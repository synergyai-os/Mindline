package agentstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (store *Store) initializeScoped(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS scoped_meta (
			key TEXT PRIMARY KEY NOT NULL,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS scopes (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL,
			purpose TEXT NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('active', 'archived')),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS scoped_lenses (
			scope_id TEXT NOT NULL REFERENCES scopes(id),
			id TEXT NOT NULL,
			name TEXT NOT NULL,
			query TEXT NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('active', 'archived')),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(scope_id, id)
		)`,
		`CREATE TABLE IF NOT EXISTS agent_actors (
			id TEXT PRIMARY KEY NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('active', 'archived')),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS scoped_retrieval_runs (
			run_id TEXT PRIMARY KEY NOT NULL,
			query TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			lens_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			retrieval_method TEXT NOT NULL,
			library_fingerprint TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(scope_id, lens_id) REFERENCES scoped_lenses(scope_id, id),
			FOREIGN KEY(agent_id) REFERENCES agent_actors(id)
		)`,
		`CREATE TABLE IF NOT EXISTS scoped_retrieval_candidates (
			run_id TEXT NOT NULL REFERENCES scoped_retrieval_runs(run_id) ON DELETE CASCADE,
			rank INTEGER NOT NULL,
			record_id TEXT NOT NULL,
			final_score REAL NOT NULL,
			components_json BLOB NOT NULL,
			PRIMARY KEY(run_id, record_id),
			UNIQUE(run_id, rank)
		)`,
		`CREATE TABLE IF NOT EXISTS scoped_judgments (
			judgment_id TEXT PRIMARY KEY NOT NULL,
			idempotency_key TEXT UNIQUE NOT NULL,
			run_id TEXT NOT NULL,
			scope_id TEXT NOT NULL,
			lens_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			record_id TEXT NOT NULL,
			actor TEXT NOT NULL CHECK(actor IN ('owner', 'agent')),
			disposition TEXT NOT NULL,
			reason TEXT NOT NULL,
			reverses_judgment_id TEXT NOT NULL,
			effect REAL NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS scoped_judgments_one_reversal
			ON scoped_judgments(reverses_judgment_id) WHERE reverses_judgment_id <> ''`,
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize scoped agent state: %w", err)
		}
	}
	if err := store.restoreScopedSidecarIfNeeded(ctx); err != nil {
		return err
	}
	if err := store.projectLegacyState(ctx); err != nil {
		return err
	}
	_, err := store.db.ExecContext(ctx, `INSERT INTO scoped_meta(key, value)
		VALUES('schema_version', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		ScopedSchemaVersion,
	)
	if err != nil {
		return errors.New("initialize scoped agent state schema")
	}
	return nil
}

func (store *Store) projectLegacyState(ctx context.Context) error {
	now := store.now().UTC().Format(time.RFC3339Nano)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("project legacy agent state")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO scopes(id, name, purpose, status, created_at, updated_at)
		VALUES(?, 'Owner root', 'Legacy owner-created lenses', 'active', ?, ?)
		ON CONFLICT(id) DO NOTHING`, OwnerRootScopeID, now, now); err != nil {
		return errors.New("project legacy agent state")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_actors(id, name, status, created_at, updated_at)
		VALUES(?, 'Legacy agent', 'archived', ?, ?)
		ON CONFLICT(id) DO NOTHING`, LegacyAgentActorID, now, now); err != nil {
		return errors.New("project legacy agent state")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_lenses(
		scope_id, id, name, query, status, created_at, updated_at
	) SELECT ?, id, name, query, 'active', created_at, updated_at FROM lenses
	WHERE 1=1
	ON CONFLICT(scope_id, id) DO UPDATE SET
		name=excluded.name, query=excluded.query,
		created_at=excluded.created_at, updated_at=excluded.updated_at`, OwnerRootScopeID); err != nil {
		return errors.New("project legacy lenses")
	}
	// Legacy runs remain compatibility-only. Copy only runs that support a
	// historical judgment so the scoped projection preserves the exact audit
	// identity without exposing unlensed legacy search as scoped recall.
	if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_retrieval_runs(
		run_id, query, scope_id, lens_id, agent_id, retrieval_method,
		library_fingerprint, created_at
	) SELECT DISTINCT r.run_id, r.query, ?, r.lens_id, ?, r.retrieval_method,
		r.library_fingerprint, r.created_at
	FROM retrieval_runs r JOIN judgments j ON j.run_id=r.run_id
	JOIN scoped_lenses l ON l.scope_id=? AND l.id=r.lens_id
	WHERE 1=1
	ON CONFLICT(run_id) DO NOTHING`, OwnerRootScopeID, LegacyAgentActorID, OwnerRootScopeID); err != nil {
		return errors.New("project legacy retrieval runs")
	}
	// A legacy corruption recovery snapshot intentionally retains judgments but
	// not their source retrieval rows. Rebuild the smallest auditable trace for
	// those historical judgments so the scoped projection never contains orphan
	// feedback and owner learning survives recovery.
	if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_retrieval_runs(
		run_id, query, scope_id, lens_id, agent_id, retrieval_method,
		library_fingerprint, created_at
	) SELECT j.run_id, 'Recovered legacy feedback', ?, j.lens_id, ?,
		'legacy-recovery', 'legacy-recovery', MIN(j.created_at)
	FROM judgments j JOIN scoped_lenses l ON l.scope_id=? AND l.id=j.lens_id
	GROUP BY j.run_id, j.lens_id
	ON CONFLICT(run_id) DO NOTHING`, OwnerRootScopeID, LegacyAgentActorID, OwnerRootScopeID); err != nil {
		return errors.New("project recovered legacy retrieval runs")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_retrieval_candidates(
		run_id, rank, record_id, final_score, components_json
	) SELECT c.run_id, c.rank, c.record_id, c.final_score, c.components_json
	FROM retrieval_candidates c JOIN scoped_retrieval_runs r ON r.run_id=c.run_id
	WHERE 1=1
	ON CONFLICT(run_id, record_id) DO NOTHING`); err != nil {
		return errors.New("project legacy retrieval candidates")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_retrieval_candidates(
		run_id, rank, record_id, final_score, components_json
	) SELECT missing.run_id,
		COALESCE((SELECT MAX(existing.rank) FROM scoped_retrieval_candidates existing
			WHERE existing.run_id=missing.run_id), 0) +
		ROW_NUMBER() OVER(PARTITION BY missing.run_id ORDER BY missing.record_id),
		missing.record_id, 0, '{}'
	FROM (SELECT DISTINCT j.run_id, j.record_id FROM judgments j
		JOIN scoped_retrieval_runs r ON r.run_id=j.run_id
		LEFT JOIN scoped_retrieval_candidates c ON c.run_id=j.run_id AND c.record_id=j.record_id
		WHERE c.record_id IS NULL) AS missing
	WHERE 1=1
	ON CONFLICT(run_id, record_id) DO NOTHING`); err != nil {
		return errors.New("project recovered legacy retrieval candidates")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_judgments(
		judgment_id, idempotency_key, run_id, scope_id, lens_id, agent_id,
		record_id, actor, disposition, reason, reverses_judgment_id, effect, created_at
	) SELECT judgment_id, idempotency_key, run_id, ?, lens_id,
		CASE WHEN actor='agent' THEN ? ELSE '' END,
		record_id, CASE WHEN actor='agent' THEN 'agent' ELSE 'owner' END,
		disposition, reason, reverses_judgment_id, effect, created_at
	FROM judgments
	WHERE 1=1
	ON CONFLICT(judgment_id) DO NOTHING`, OwnerRootScopeID, LegacyAgentActorID); err != nil {
		return errors.New("project legacy judgments")
	}
	return tx.Commit()
}

func (store *Store) PutScope(ctx context.Context, scope Scope) (Scope, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	scope.ID = strings.TrimSpace(scope.ID)
	scope.Name = strings.TrimSpace(scope.Name)
	scope.Purpose = strings.TrimSpace(scope.Purpose)
	if !validBounded(scope.ID, 256) || !validBounded(scope.Name, 1024) ||
		!validBounded(scope.Purpose, maximumTextRunes) || scope.ID == OwnerRootScopeID {
		return Scope{}, errors.New("invalid scope")
	}
	now := store.now().UTC().Format(time.RFC3339Nano)
	_, err := store.db.ExecContext(ctx, `INSERT INTO scopes(id, name, purpose, status, created_at, updated_at)
		VALUES(?, ?, ?, 'active', ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, purpose=excluded.purpose,
		updated_at=excluded.updated_at`, scope.ID, scope.Name, scope.Purpose, now, now)
	if err != nil {
		return Scope{}, errors.New("save scope")
	}
	saved, err := store.getScope(ctx, scope.ID)
	if err == nil {
		err = store.writeRecoverySnapshot(ctx)
	}
	return saved, err
}

func (store *Store) GetScope(ctx context.Context, id string) (Scope, error) {
	return store.getScope(ctx, strings.TrimSpace(id))
}

func (store *Store) getScope(ctx context.Context, id string) (Scope, error) {
	var scope Scope
	err := store.db.QueryRowContext(ctx, `SELECT id, name, purpose, status, created_at, updated_at
		FROM scopes WHERE id=?`, id).Scan(&scope.ID, &scope.Name, &scope.Purpose,
		&scope.Status, &scope.CreatedAt, &scope.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Scope{}, errors.New("scope not found")
	}
	if err != nil {
		return Scope{}, errors.New("read scope")
	}
	return scope, nil
}

func (store *Store) ListScopes(ctx context.Context) ([]Scope, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT id, name, purpose, status, created_at, updated_at
		FROM scopes ORDER BY id`)
	if err != nil {
		return nil, errors.New("list scopes")
	}
	defer rows.Close()
	result := []Scope{}
	for rows.Next() {
		var scope Scope
		if err := rows.Scan(&scope.ID, &scope.Name, &scope.Purpose, &scope.Status,
			&scope.CreatedAt, &scope.UpdatedAt); err != nil {
			return nil, errors.New("list scopes")
		}
		result = append(result, scope)
	}
	return result, rows.Err()
}

func (store *Store) ArchiveScope(ctx context.Context, id string) (Scope, error) {
	return store.archiveScopeObject(ctx, "scopes", strings.TrimSpace(id), "", "scope")
}

func (store *Store) PutScopedLens(ctx context.Context, lens ScopedLens) (ScopedLens, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	lens.ScopeID = strings.TrimSpace(lens.ScopeID)
	lens.ID = strings.TrimSpace(lens.ID)
	lens.Name = strings.TrimSpace(lens.Name)
	lens.Query = strings.TrimSpace(lens.Query)
	if !validBounded(lens.ScopeID, 256) || !validBounded(lens.ID, 256) ||
		!validBounded(lens.Name, 1024) || !validBounded(lens.Query, maximumTextRunes) {
		return ScopedLens{}, errors.New("invalid scoped lens")
	}
	scope, err := store.getScope(ctx, lens.ScopeID)
	if err != nil || scope.Status != StatusActive {
		return ScopedLens{}, errors.New("active scope not found")
	}
	now := store.now().UTC().Format(time.RFC3339Nano)
	_, err = store.db.ExecContext(ctx, `INSERT INTO scoped_lenses(
		scope_id, id, name, query, status, created_at, updated_at
	) VALUES(?, ?, ?, ?, 'active', ?, ?)
	ON CONFLICT(scope_id, id) DO UPDATE SET name=excluded.name, query=excluded.query,
		updated_at=excluded.updated_at`, lens.ScopeID, lens.ID, lens.Name, lens.Query, now, now)
	if err != nil {
		return ScopedLens{}, errors.New("save scoped lens")
	}
	saved, err := store.getScopedLens(ctx, lens.ScopeID, lens.ID)
	if err == nil {
		err = store.writeRecoverySnapshot(ctx)
	}
	return saved, err
}

func (store *Store) GetScopedLens(ctx context.Context, scopeID, lensID string) (ScopedLens, error) {
	return store.getScopedLens(ctx, strings.TrimSpace(scopeID), strings.TrimSpace(lensID))
}

func (store *Store) getScopedLens(ctx context.Context, scopeID, lensID string) (ScopedLens, error) {
	var lens ScopedLens
	err := store.db.QueryRowContext(ctx, `SELECT scope_id, id, name, query, status, created_at, updated_at
		FROM scoped_lenses WHERE scope_id=? AND id=?`, scopeID, lensID).Scan(
		&lens.ScopeID, &lens.ID, &lens.Name, &lens.Query, &lens.Status,
		&lens.CreatedAt, &lens.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ScopedLens{}, errors.New("scoped lens not found")
	}
	if err != nil {
		return ScopedLens{}, errors.New("read scoped lens")
	}
	return lens, nil
}

func (store *Store) ListScopedLenses(ctx context.Context, scopeID string) ([]ScopedLens, error) {
	scopeID = strings.TrimSpace(scopeID)
	query := `SELECT scope_id, id, name, query, status, created_at, updated_at FROM scoped_lenses`
	args := []any{}
	if scopeID != "" {
		query += ` WHERE scope_id=?`
		args = append(args, scopeID)
	}
	query += ` ORDER BY scope_id, id`
	rows, err := store.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.New("list scoped lenses")
	}
	defer rows.Close()
	result := []ScopedLens{}
	for rows.Next() {
		var lens ScopedLens
		if err := rows.Scan(&lens.ScopeID, &lens.ID, &lens.Name, &lens.Query,
			&lens.Status, &lens.CreatedAt, &lens.UpdatedAt); err != nil {
			return nil, errors.New("list scoped lenses")
		}
		result = append(result, lens)
	}
	return result, rows.Err()
}

func (store *Store) ArchiveScopedLens(ctx context.Context, scopeID, lensID string) (ScopedLens, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	scopeID, lensID = strings.TrimSpace(scopeID), strings.TrimSpace(lensID)
	if !validBounded(scopeID, 256) || !validBounded(lensID, 256) {
		return ScopedLens{}, errors.New("invalid scoped lens")
	}
	now := store.now().UTC().Format(time.RFC3339Nano)
	result, err := store.db.ExecContext(ctx, `UPDATE scoped_lenses SET status='archived', updated_at=?
		WHERE scope_id=? AND id=?`, now, scopeID, lensID)
	if err != nil {
		return ScopedLens{}, errors.New("archive scoped lens")
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return ScopedLens{}, errors.New("scoped lens not found")
	}
	saved, err := store.getScopedLens(ctx, scopeID, lensID)
	if err == nil {
		err = store.writeRecoverySnapshot(ctx)
	}
	return saved, err
}

func (store *Store) PutAgentActor(ctx context.Context, actor AgentActor) (AgentActor, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	actor.ID, actor.Name = strings.TrimSpace(actor.ID), strings.TrimSpace(actor.Name)
	if !validBounded(actor.ID, 256) || !validBounded(actor.Name, 1024) || actor.ID == LegacyAgentActorID {
		return AgentActor{}, errors.New("invalid agent actor")
	}
	now := store.now().UTC().Format(time.RFC3339Nano)
	_, err := store.db.ExecContext(ctx, `INSERT INTO agent_actors(id, name, status, created_at, updated_at)
		VALUES(?, ?, 'active', ?, ?) ON CONFLICT(id) DO UPDATE SET
		name=excluded.name, updated_at=excluded.updated_at`, actor.ID, actor.Name, now, now)
	if err != nil {
		return AgentActor{}, errors.New("save agent actor")
	}
	saved, err := store.getAgentActor(ctx, actor.ID)
	if err == nil {
		err = store.writeRecoverySnapshot(ctx)
	}
	return saved, err
}

func (store *Store) GetAgentActor(ctx context.Context, id string) (AgentActor, error) {
	return store.getAgentActor(ctx, strings.TrimSpace(id))
}

func (store *Store) getAgentActor(ctx context.Context, id string) (AgentActor, error) {
	var actor AgentActor
	err := store.db.QueryRowContext(ctx, `SELECT id, name, status, created_at, updated_at
		FROM agent_actors WHERE id=?`, id).Scan(&actor.ID, &actor.Name, &actor.Status,
		&actor.CreatedAt, &actor.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentActor{}, errors.New("agent actor not found")
	}
	if err != nil {
		return AgentActor{}, errors.New("read agent actor")
	}
	return actor, nil
}

func (store *Store) ListAgentActors(ctx context.Context) ([]AgentActor, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT id, name, status, created_at, updated_at
		FROM agent_actors ORDER BY id`)
	if err != nil {
		return nil, errors.New("list agent actors")
	}
	defer rows.Close()
	result := []AgentActor{}
	for rows.Next() {
		var actor AgentActor
		if err := rows.Scan(&actor.ID, &actor.Name, &actor.Status, &actor.CreatedAt,
			&actor.UpdatedAt); err != nil {
			return nil, errors.New("list agent actors")
		}
		result = append(result, actor)
	}
	return result, rows.Err()
}

func (store *Store) ArchiveAgentActor(ctx context.Context, id string) (AgentActor, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	id = strings.TrimSpace(id)
	if !validBounded(id, 256) || id == LegacyAgentActorID {
		return AgentActor{}, errors.New("invalid agent actor")
	}
	now := store.now().UTC().Format(time.RFC3339Nano)
	result, err := store.db.ExecContext(ctx, `UPDATE agent_actors SET status='archived', updated_at=? WHERE id=?`, now, id)
	if err != nil {
		return AgentActor{}, errors.New("archive agent actor")
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return AgentActor{}, errors.New("agent actor not found")
	}
	saved, err := store.getAgentActor(ctx, id)
	if err == nil {
		err = store.writeRecoverySnapshot(ctx)
	}
	return saved, err
}

func (store *Store) archiveScopeObject(ctx context.Context, table, id, parentID, object string) (Scope, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	if table != "scopes" || parentID != "" || !validBounded(id, 256) || id == OwnerRootScopeID {
		return Scope{}, fmt.Errorf("invalid %s", object)
	}
	now := store.now().UTC().Format(time.RFC3339Nano)
	result, err := store.db.ExecContext(ctx, `UPDATE scopes SET status='archived', updated_at=? WHERE id=?`, now, id)
	if err != nil {
		return Scope{}, fmt.Errorf("archive %s", object)
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return Scope{}, fmt.Errorf("%s not found", object)
	}
	saved, err := store.getScope(ctx, id)
	if err == nil {
		err = store.writeRecoverySnapshot(ctx)
	}
	return saved, err
}

func (store *Store) ResolveScopedContext(ctx context.Context, input ScopedContext) (Scope, ScopedLens, AgentActor, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	return store.resolveScopedContext(ctx, input)
}

func (store *Store) resolveScopedContext(ctx context.Context, input ScopedContext) (Scope, ScopedLens, AgentActor, error) {
	input.ScopeID = strings.TrimSpace(input.ScopeID)
	input.LensID = strings.TrimSpace(input.LensID)
	input.AgentID = strings.TrimSpace(input.AgentID)
	if !validBounded(input.ScopeID, 256) || !validBounded(input.LensID, 256) ||
		!validBounded(input.AgentID, 256) {
		return Scope{}, ScopedLens{}, AgentActor{}, errors.New("incomplete scoped context")
	}
	var scope Scope
	var lens ScopedLens
	var actor AgentActor
	err := store.db.QueryRowContext(ctx, `SELECT
		s.id, s.name, s.purpose, s.status, s.created_at, s.updated_at,
		l.scope_id, l.id, l.name, l.query, l.status, l.created_at, l.updated_at,
		a.id, a.name, a.status, a.created_at, a.updated_at
	FROM scopes s JOIN scoped_lenses l ON l.scope_id=s.id
	JOIN agent_actors a ON a.id=?
	WHERE s.id=? AND l.id=?`, input.AgentID, input.ScopeID, input.LensID).Scan(
		&scope.ID, &scope.Name, &scope.Purpose, &scope.Status, &scope.CreatedAt, &scope.UpdatedAt,
		&lens.ScopeID, &lens.ID, &lens.Name, &lens.Query, &lens.Status, &lens.CreatedAt, &lens.UpdatedAt,
		&actor.ID, &actor.Name, &actor.Status, &actor.CreatedAt, &actor.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Scope{}, ScopedLens{}, AgentActor{}, errors.New("scoped context not found")
	}
	if err != nil {
		return Scope{}, ScopedLens{}, AgentActor{}, errors.New("resolve scoped context")
	}
	if scope.Status != StatusActive || lens.Status != StatusActive || actor.Status != StatusActive {
		return Scope{}, ScopedLens{}, AgentActor{}, errors.New("scoped context is archived")
	}
	return scope, lens, actor, nil
}

func (store *Store) SaveScopedRetrieval(ctx context.Context, trace ScopedRetrievalTrace) error {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	trace.RunID = strings.TrimSpace(trace.RunID)
	trace.Query = strings.TrimSpace(trace.Query)
	trace.ScopeID = strings.TrimSpace(trace.ScopeID)
	trace.LensID = strings.TrimSpace(trace.LensID)
	trace.AgentID = strings.TrimSpace(trace.AgentID)
	if !validBounded(trace.RunID, 256) || !validBounded(trace.Query, maximumTextRunes) ||
		!validBounded(trace.RetrievalMethod, 1024) || !validBounded(trace.LibraryFingerprint, 256) ||
		!validBounded(trace.CreatedAt, 256) || len(trace.Candidates) > 100 {
		return errors.New("invalid scoped retrieval trace")
	}
	if _, _, _, err := store.resolveScopedContext(ctx, ScopedContext{
		ScopeID: trace.ScopeID, LensID: trace.LensID, AgentID: trace.AgentID,
	}); err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("start scoped retrieval trace")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_retrieval_runs(
		run_id, query, scope_id, lens_id, agent_id, retrieval_method,
		library_fingerprint, created_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, trace.RunID, trace.Query, trace.ScopeID,
		trace.LensID, trace.AgentID, trace.RetrievalMethod, trace.LibraryFingerprint,
		trace.CreatedAt); err != nil {
		return errors.New("save scoped retrieval trace")
	}
	seenRecords := map[string]bool{}
	seenRanks := map[int]bool{}
	for _, candidate := range trace.Candidates {
		candidate.RecordID = strings.TrimSpace(candidate.RecordID)
		if !validBounded(candidate.RecordID, 1024) || candidate.Rank < 1 ||
			seenRecords[candidate.RecordID] || seenRanks[candidate.Rank] {
			return errors.New("invalid scoped retrieval candidate")
		}
		seenRecords[candidate.RecordID] = true
		seenRanks[candidate.Rank] = true
		components, err := json.Marshal(candidate.ComponentScore)
		if err != nil {
			return errors.New("encode scoped retrieval trace")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_retrieval_candidates(
			run_id, rank, record_id, final_score, components_json
		) VALUES(?, ?, ?, ?, ?)`, trace.RunID, candidate.Rank, candidate.RecordID,
			candidate.FinalScore, components); err != nil {
			return errors.New("save scoped retrieval candidate")
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.New("save scoped retrieval trace")
	}
	return store.writeRecoverySnapshot(ctx)
}
