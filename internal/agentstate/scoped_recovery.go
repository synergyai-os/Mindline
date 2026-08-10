package agentstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/synergyai-os/Mindline/internal/contentguard"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	scopedRecoverySchemaVersion = "mindline-agent-scoped-recovery/v0.1"
	maximumScopedRecoveryBytes  = 128 << 20
)

type scopedRecoverySnapshot struct {
	SchemaVersion string                 `json:"schema_version"`
	Scopes        []Scope                `json:"scopes"`
	Lenses        []ScopedLens           `json:"lenses"`
	Actors        []AgentActor           `json:"actors"`
	Runs          []ScopedRetrievalTrace `json:"runs"`
	Judgments     []ScopedJudgment       `json:"judgments"`
}

func scopedRecoveryPath(databasePath string) string {
	return databasePath + ".scoped-recovery.json"
}

// restoreScopedSidecarIfNeeded is deliberately insert-only. It runs only when
// the scoped schema marker is absent (first upgrade or re-upgrade after a prior
// binary rebuilt the legacy database), and never replaces live scoped rows.
func (store *Store) restoreScopedSidecarIfNeeded(ctx context.Context) (bool, error) {
	var version string
	err := store.db.QueryRowContext(ctx,
		`SELECT value FROM scoped_meta WHERE key='schema_version'`).Scan(&version)
	if err == nil {
		if version != ScopedSchemaVersion {
			return false, errors.New("unsupported scoped agent state schema")
		}
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, errors.New("read scoped agent state schema")
	}
	snapshot, present, err := readScopedRecoverySnapshot(store.path)
	if err != nil || !present {
		return false, err
	}
	return true, store.restoreScopedRecoverySnapshot(ctx, snapshot)
}

func readScopedRecoverySnapshot(databasePath string) (scopedRecoverySnapshot, bool, error) {
	path := scopedRecoveryPath(databasePath)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return scopedRecoverySnapshot{}, false, nil
	} else if err != nil {
		return scopedRecoverySnapshot{}, false, errors.New("read scoped agent recovery snapshot")
	}
	var snapshot scopedRecoverySnapshot
	if err := privateio.ReadJSONStrictBounded(filepath.Dir(path), path, maximumScopedRecoveryBytes, &snapshot); err != nil ||
		validateScopedRecoverySnapshot(snapshot) != nil {
		return scopedRecoverySnapshot{}, false, errors.New("read scoped agent recovery snapshot")
	}
	return snapshot, true, nil
}

func (store *Store) writeScopedRecoverySnapshot(ctx context.Context) error {
	snapshot, err := store.buildScopedRecoverySnapshot(ctx)
	if err != nil {
		return err
	}
	if err := writeScopedRecoverySnapshotFile(scopedRecoveryPath(store.path), snapshot, store.scopedRecoveryLimit()); err != nil {
		return errors.New("write scoped agent recovery snapshot")
	}
	return nil
}

func (store *Store) scopedRecoveryLimit() int64 {
	if store.scopedRecoveryByteLimit > 0 {
		return store.scopedRecoveryByteLimit
	}
	return maximumScopedRecoveryBytes
}

// preflightScopedRetrievalRecovery proves that the exact post-commit scoped
// recovery projection still fits before the database mutation begins. The
// mutation mutex held by the caller keeps this projection stable until commit.
func (store *Store) preflightScopedRetrievalRecovery(ctx context.Context, trace ScopedRetrievalTrace) error {
	snapshot, err := store.buildScopedRecoverySnapshot(ctx)
	if err != nil {
		return err
	}
	trace.Candidates = append([]ScopedCandidateTrace(nil), trace.Candidates...)
	sort.Slice(trace.Candidates, func(i, j int) bool {
		if trace.Candidates[i].Rank == trace.Candidates[j].Rank {
			return trace.Candidates[i].RecordID < trace.Candidates[j].RecordID
		}
		return trace.Candidates[i].Rank < trace.Candidates[j].Rank
	})
	snapshot.Runs = append(snapshot.Runs, trace)
	sort.Slice(snapshot.Runs, func(i, j int) bool {
		if snapshot.Runs[i].CreatedAt == snapshot.Runs[j].CreatedAt {
			return snapshot.Runs[i].RunID < snapshot.Runs[j].RunID
		}
		return snapshot.Runs[i].CreatedAt < snapshot.Runs[j].CreatedAt
	})
	if err := validateScopedRecoverySnapshot(snapshot); err != nil {
		return errors.New("preflight scoped agent recovery snapshot")
	}
	if _, err := encodeScopedRecoverySnapshot(snapshot, store.scopedRecoveryLimit()); err != nil {
		return errors.New("preflight scoped agent recovery snapshot")
	}
	return nil
}

// preflightScopedJudgmentRecovery proves that the exact post-commit judgment
// projection fits before the durable database mutation begins. The caller
// holds mutationMu, so the projection cannot drift between this check and the
// insert.
func (store *Store) preflightScopedJudgmentRecovery(ctx context.Context, judgment ScopedJudgment) error {
	snapshot, err := store.buildScopedRecoverySnapshot(ctx)
	if err != nil {
		return err
	}
	snapshot.Judgments = append(snapshot.Judgments, judgment)
	sort.Slice(snapshot.Judgments, func(i, j int) bool {
		if snapshot.Judgments[i].CreatedAt == snapshot.Judgments[j].CreatedAt {
			return snapshot.Judgments[i].JudgmentID < snapshot.Judgments[j].JudgmentID
		}
		return snapshot.Judgments[i].CreatedAt < snapshot.Judgments[j].CreatedAt
	})
	if err := validateScopedRecoverySnapshot(snapshot); err != nil {
		return errors.New("preflight scoped agent recovery snapshot")
	}
	if _, err := encodeScopedRecoverySnapshot(snapshot, store.scopedRecoveryLimit()); err != nil {
		return errors.New("preflight scoped agent recovery snapshot")
	}
	return nil
}

func (store *Store) preflightScopeRecovery(ctx context.Context, proposed Scope) (Scope, error) {
	snapshot, err := store.buildScopedRecoverySnapshot(ctx)
	if err != nil {
		return Scope{}, err
	}
	found := false
	for index := range snapshot.Scopes {
		if snapshot.Scopes[index].ID != proposed.ID {
			continue
		}
		proposed.CreatedAt = snapshot.Scopes[index].CreatedAt
		if proposed.Status == "" {
			proposed.Status = snapshot.Scopes[index].Status
		}
		snapshot.Scopes[index] = proposed
		found = true
		break
	}
	if !found {
		proposed.Status = StatusActive
		proposed.CreatedAt = proposed.UpdatedAt
		snapshot.Scopes = append(snapshot.Scopes, proposed)
	}
	sort.Slice(snapshot.Scopes, func(i, j int) bool { return snapshot.Scopes[i].ID < snapshot.Scopes[j].ID })
	if err := validateAndEncodeScopedRecoveryProjection(snapshot, store.scopedRecoveryLimit()); err != nil {
		return Scope{}, err
	}
	return proposed, nil
}

func (store *Store) preflightScopedLensRecovery(ctx context.Context, proposed ScopedLens) (ScopedLens, error) {
	snapshot, err := store.buildScopedRecoverySnapshot(ctx)
	if err != nil {
		return ScopedLens{}, err
	}
	found := false
	for index := range snapshot.Lenses {
		if snapshot.Lenses[index].ScopeID != proposed.ScopeID || snapshot.Lenses[index].ID != proposed.ID {
			continue
		}
		proposed.CreatedAt = snapshot.Lenses[index].CreatedAt
		if proposed.Status == "" {
			proposed.Status = snapshot.Lenses[index].Status
		}
		snapshot.Lenses[index] = proposed
		found = true
		break
	}
	if !found {
		proposed.Status = StatusActive
		proposed.CreatedAt = proposed.UpdatedAt
		snapshot.Lenses = append(snapshot.Lenses, proposed)
	}
	sort.Slice(snapshot.Lenses, func(i, j int) bool {
		if snapshot.Lenses[i].ScopeID == snapshot.Lenses[j].ScopeID {
			return snapshot.Lenses[i].ID < snapshot.Lenses[j].ID
		}
		return snapshot.Lenses[i].ScopeID < snapshot.Lenses[j].ScopeID
	})
	if err := validateAndEncodeScopedRecoveryProjection(snapshot, store.scopedRecoveryLimit()); err != nil {
		return ScopedLens{}, err
	}
	return proposed, nil
}

func (store *Store) preflightAgentActorRecovery(ctx context.Context, proposed AgentActor) (AgentActor, error) {
	snapshot, err := store.buildScopedRecoverySnapshot(ctx)
	if err != nil {
		return AgentActor{}, err
	}
	found := false
	for index := range snapshot.Actors {
		if snapshot.Actors[index].ID != proposed.ID {
			continue
		}
		proposed.CreatedAt = snapshot.Actors[index].CreatedAt
		if proposed.Status == "" {
			proposed.Status = snapshot.Actors[index].Status
		}
		snapshot.Actors[index] = proposed
		found = true
		break
	}
	if !found {
		proposed.Status = StatusActive
		proposed.CreatedAt = proposed.UpdatedAt
		snapshot.Actors = append(snapshot.Actors, proposed)
	}
	sort.Slice(snapshot.Actors, func(i, j int) bool { return snapshot.Actors[i].ID < snapshot.Actors[j].ID })
	if err := validateAndEncodeScopedRecoveryProjection(snapshot, store.scopedRecoveryLimit()); err != nil {
		return AgentActor{}, err
	}
	return proposed, nil
}

func validateAndEncodeScopedRecoveryProjection(snapshot scopedRecoverySnapshot, maximum int64) error {
	if err := validateScopedRecoverySnapshot(snapshot); err != nil {
		return errors.New("preflight scoped agent recovery snapshot")
	}
	if _, err := encodeScopedRecoverySnapshot(snapshot, maximum); err != nil {
		return errors.New("preflight scoped agent recovery snapshot")
	}
	return nil
}

func writeScopedRecoverySnapshotFile(path string, snapshot scopedRecoverySnapshot, maximum int64) error {
	data, err := encodeScopedRecoverySnapshot(snapshot, maximum)
	if err != nil {
		return err
	}
	return privateio.WriteFile(path, data, false)
}

func encodeScopedRecoverySnapshot(snapshot scopedRecoverySnapshot, maximum int64) ([]byte, error) {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if maximum < 1 || int64(len(data)) > maximum {
		return nil, errors.New("scoped agent recovery snapshot exceeds limit")
	}
	return data, nil
}

func (store *Store) buildScopedRecoverySnapshot(ctx context.Context) (scopedRecoverySnapshot, error) {
	snapshot := scopedRecoverySnapshot{
		SchemaVersion: scopedRecoverySchemaVersion,
		Scopes:        []Scope{}, Lenses: []ScopedLens{}, Actors: []AgentActor{},
		Runs: []ScopedRetrievalTrace{}, Judgments: []ScopedJudgment{},
	}
	var err error
	if snapshot.Scopes, err = store.ListScopes(ctx); err != nil {
		return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
	}
	if snapshot.Lenses, err = store.ListScopedLenses(ctx, ""); err != nil {
		return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
	}
	if snapshot.Actors, err = store.ListAgentActors(ctx); err != nil {
		return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
	}
	rows, err := store.db.QueryContext(ctx, `SELECT run_id, query, scope_id, lens_id,
		agent_id, retrieval_method, library_fingerprint, created_at
		FROM scoped_retrieval_runs ORDER BY created_at, run_id`)
	if err != nil {
		return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
	}
	for rows.Next() {
		var run ScopedRetrievalTrace
		if err := rows.Scan(&run.RunID, &run.Query, &run.ScopeID, &run.LensID,
			&run.AgentID, &run.RetrievalMethod, &run.LibraryFingerprint, &run.CreatedAt); err != nil {
			rows.Close()
			return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
		}
		run.Candidates = []ScopedCandidateTrace{}
		snapshot.Runs = append(snapshot.Runs, run)
	}
	if err := rows.Close(); err != nil {
		return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
	}
	for index := range snapshot.Runs {
		candidateRows, err := store.db.QueryContext(ctx, `SELECT record_id, rank,
			final_score, components_json FROM scoped_retrieval_candidates
			WHERE run_id=? ORDER BY rank, record_id`, snapshot.Runs[index].RunID)
		if err != nil {
			return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
		}
		for candidateRows.Next() {
			var candidate ScopedCandidateTrace
			var components []byte
			if err := candidateRows.Scan(&candidate.RecordID, &candidate.Rank,
				&candidate.FinalScore, &components); err != nil ||
				json.Unmarshal(components, &candidate.ComponentScore) != nil {
				candidateRows.Close()
				return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
			}
			snapshot.Runs[index].Candidates = append(snapshot.Runs[index].Candidates, candidate)
		}
		if err := candidateRows.Close(); err != nil {
			return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
		}
	}
	rows, err = store.db.QueryContext(ctx, `SELECT judgment_id, idempotency_key,
		run_id, scope_id, lens_id, agent_id, record_id, actor, disposition, reason,
		reverses_judgment_id, effect, created_at FROM scoped_judgments
		ORDER BY created_at, judgment_id`)
	if err != nil {
		return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
	}
	for rows.Next() {
		judgment, err := scanScopedJudgment(rows)
		if err != nil {
			rows.Close()
			return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
		}
		snapshot.Judgments = append(snapshot.Judgments, judgment)
	}
	if err := rows.Close(); err != nil {
		return scopedRecoverySnapshot{}, errors.New("build scoped agent recovery snapshot")
	}
	if err := validateScopedRecoverySnapshot(snapshot); err != nil {
		return scopedRecoverySnapshot{}, fmt.Errorf("build scoped agent recovery snapshot: %w", err)
	}
	return snapshot, nil
}

func (store *Store) restoreScopedRecoverySnapshot(ctx context.Context, snapshot scopedRecoverySnapshot) error {
	if validateScopedRecoverySnapshot(snapshot) != nil {
		return errors.New("restore scoped agent recovery snapshot")
	}
	// Marker absence plus non-empty scoped tables is an interrupted migration,
	// not permission to overwrite live data. Fail closed and preserve it.
	for _, table := range []string{"scopes", "scoped_lenses", "agent_actors",
		"scoped_retrieval_runs", "scoped_retrieval_candidates", "scoped_judgments"} {
		var count int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			return errors.New("restore scoped agent recovery snapshot into non-empty state")
		}
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("restore scoped agent recovery snapshot")
	}
	defer tx.Rollback()
	for _, scope := range snapshot.Scopes {
		if !validBounded(scope.ID, 256) || !validBounded(scope.Name, 1024) ||
			!validBounded(scope.Purpose, maximumTextRunes) || !validScopedStatus(scope.Status) {
			return errors.New("restore scoped agent recovery snapshot")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO scopes VALUES(?, ?, ?, ?, ?, ?)`,
			scope.ID, scope.Name, scope.Purpose, scope.Status, scope.CreatedAt, scope.UpdatedAt); err != nil {
			return errors.New("restore scoped agent recovery snapshot")
		}
	}
	for _, lens := range snapshot.Lenses {
		if !validBounded(lens.ScopeID, 256) || !validBounded(lens.ID, 256) ||
			!validBounded(lens.Name, 1024) || !validBounded(lens.Query, maximumTextRunes) ||
			!validScopedStatus(lens.Status) {
			return errors.New("restore scoped agent recovery snapshot")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_lenses VALUES(?, ?, ?, ?, ?, ?, ?)`,
			lens.ScopeID, lens.ID, lens.Name, lens.Query, lens.Status, lens.CreatedAt, lens.UpdatedAt); err != nil {
			return errors.New("restore scoped agent recovery snapshot")
		}
	}
	for _, actor := range snapshot.Actors {
		if !validBounded(actor.ID, 256) || !validBounded(actor.Name, 1024) ||
			!validScopedStatus(actor.Status) {
			return errors.New("restore scoped agent recovery snapshot")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_actors VALUES(?, ?, ?, ?, ?)`,
			actor.ID, actor.Name, actor.Status, actor.CreatedAt, actor.UpdatedAt); err != nil {
			return errors.New("restore scoped agent recovery snapshot")
		}
	}
	for _, run := range snapshot.Runs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_retrieval_runs VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			run.RunID, run.Query, run.ScopeID, run.LensID, run.AgentID,
			run.RetrievalMethod, run.LibraryFingerprint, run.CreatedAt); err != nil {
			return errors.New("restore scoped agent recovery snapshot")
		}
		for _, candidate := range run.Candidates {
			components, err := json.Marshal(candidate.ComponentScore)
			if err != nil {
				return errors.New("restore scoped agent recovery snapshot")
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_retrieval_candidates VALUES(?, ?, ?, ?, ?)`,
				run.RunID, candidate.Rank, candidate.RecordID, candidate.FinalScore, components); err != nil {
				return errors.New("restore scoped agent recovery snapshot")
			}
		}
	}
	for _, judgment := range snapshot.Judgments {
		if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_judgments VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			judgment.JudgmentID, judgment.IdempotencyKey, judgment.RunID,
			judgment.ScopeID, judgment.LensID, judgment.AgentID, judgment.RecordID,
			judgment.Actor, judgment.Disposition, judgment.Reason, judgment.ReversesID,
			judgment.Effect, judgment.CreatedAt); err != nil {
			return errors.New("restore scoped agent recovery snapshot")
		}
	}
	return tx.Commit()
}

func validScopedStatus(status string) bool {
	return status == StatusActive || status == StatusArchived
}

func validateScopedRecoverySnapshot(snapshot scopedRecoverySnapshot) error {
	if snapshot.SchemaVersion != scopedRecoverySchemaVersion {
		return errors.New("invalid scoped agent recovery snapshot schema")
	}
	scopes := make(map[string]bool, len(snapshot.Scopes))
	for _, scope := range snapshot.Scopes {
		if !validBounded(scope.ID, 256) || !validBounded(scope.Name, 1024) ||
			!validBounded(scope.Purpose, maximumTextRunes) || !validScopedStatus(scope.Status) ||
			!validBounded(scope.CreatedAt, 256) || !validBounded(scope.UpdatedAt, 256) ||
			containsSecretLikeAny(scope.ID, scope.Name, scope.Purpose, scope.Status,
				scope.CreatedAt, scope.UpdatedAt) || scopes[scope.ID] {
			return errors.New("invalid scoped agent recovery snapshot scope")
		}
		scopes[scope.ID] = true
	}
	lenses := make(map[string]bool, len(snapshot.Lenses))
	for _, lens := range snapshot.Lenses {
		key := lens.ScopeID + "\x00" + lens.ID
		if !scopes[lens.ScopeID] || !validBounded(lens.ID, 256) ||
			!validBounded(lens.Name, 1024) || !validBounded(lens.Query, maximumTextRunes) ||
			!validScopedStatus(lens.Status) || !validBounded(lens.CreatedAt, 256) ||
			!validBounded(lens.UpdatedAt, 256) ||
			containsSecretLikeAny(lens.ScopeID, lens.ID, lens.Name, lens.Query,
				lens.Status, lens.CreatedAt, lens.UpdatedAt) || lenses[key] {
			return errors.New("invalid scoped agent recovery snapshot lens")
		}
		lenses[key] = true
	}
	actors := make(map[string]bool, len(snapshot.Actors))
	for _, actor := range snapshot.Actors {
		if !validBounded(actor.ID, 256) || !validBounded(actor.Name, 1024) ||
			!validScopedStatus(actor.Status) || !validBounded(actor.CreatedAt, 256) ||
			!validBounded(actor.UpdatedAt, 256) ||
			containsSecretLikeAny(actor.ID, actor.Name, actor.Status, actor.CreatedAt, actor.UpdatedAt) ||
			actors[actor.ID] {
			return errors.New("invalid scoped agent recovery snapshot actor")
		}
		actors[actor.ID] = true
	}
	type runContext struct{ scopeID, lensID, agentID string }
	runs := make(map[string]runContext, len(snapshot.Runs))
	candidates := make(map[string]bool)
	for _, run := range snapshot.Runs {
		if !validBounded(run.RunID, 256) || !validBounded(run.Query, maximumTextRunes) ||
			!lenses[run.ScopeID+"\x00"+run.LensID] || !actors[run.AgentID] ||
			!validBounded(run.RetrievalMethod, 1024) ||
			!validBounded(run.LibraryFingerprint, 256) || !validBounded(run.CreatedAt, 256) ||
			containsSecretLikeAny(run.RunID, run.Query, run.ScopeID, run.LensID,
				run.AgentID, run.RetrievalMethod, run.LibraryFingerprint, run.CreatedAt) ||
			len(run.Candidates) > 100 {
			return errors.New("invalid scoped agent recovery snapshot run")
		}
		if _, exists := runs[run.RunID]; exists {
			return errors.New("invalid scoped agent recovery snapshot duplicate run")
		}
		runs[run.RunID] = runContext{run.ScopeID, run.LensID, run.AgentID}
		seenRecords, seenRanks := map[string]bool{}, map[int]bool{}
		for _, candidate := range run.Candidates {
			if !validBounded(candidate.RecordID, 1024) || candidate.Rank < 1 ||
				containsSecretLikeAny(candidate.RecordID) ||
				seenRecords[candidate.RecordID] || seenRanks[candidate.Rank] ||
				!finite(candidate.FinalScore) || len(candidate.ComponentScore) > 1000 {
				return errors.New("invalid scoped agent recovery snapshot candidate")
			}
			for name, score := range candidate.ComponentScore {
				if !validBounded(name, 256) || contentguard.ContainsSecretLike(name) || !finite(score) {
					return errors.New("invalid scoped agent recovery snapshot candidate component")
				}
			}
			seenRecords[candidate.RecordID], seenRanks[candidate.Rank] = true, true
			candidates[run.RunID+"\x00"+candidate.RecordID] = true
		}
	}
	judgmentIDs := make(map[string]bool, len(snapshot.Judgments))
	idempotencyKeys := make(map[string]bool, len(snapshot.Judgments))
	originals := make(map[string]ScopedJudgment, len(snapshot.Judgments))
	for _, judgment := range snapshot.Judgments {
		run, runExists := runs[judgment.RunID]
		validActor := judgment.Actor == FeedbackOwner && judgment.AgentID == "" ||
			judgment.Actor == FeedbackAgent && judgment.AgentID == run.agentID
		switch {
		case !validBounded(judgment.JudgmentID, 256), !validBounded(judgment.IdempotencyKey, 1024):
			return errors.New("invalid scoped agent recovery snapshot judgment identity")
		case containsSecretLikeAny(judgment.JudgmentID, judgment.IdempotencyKey,
			judgment.RunID, judgment.ScopeID, judgment.LensID, judgment.AgentID,
			judgment.RecordID, judgment.Reason, judgment.ReversesID, judgment.CreatedAt):
			return errors.New("invalid scoped agent recovery snapshot judgment privacy")
		case judgmentIDs[judgment.JudgmentID], idempotencyKeys[judgment.IdempotencyKey]:
			return errors.New("invalid scoped agent recovery snapshot duplicate judgment")
		case !runExists:
			return fmt.Errorf("invalid scoped agent recovery snapshot judgment run (%d runs)", len(runs))
		case run.scopeID != judgment.ScopeID:
			return errors.New("invalid scoped agent recovery snapshot judgment scope")
		case run.lensID != judgment.LensID:
			return errors.New("invalid scoped agent recovery snapshot judgment lens")
		case !validActor:
			return errors.New("invalid scoped agent recovery snapshot judgment actor")
		case !candidates[judgment.RunID+"\x00"+judgment.RecordID]:
			return errors.New("invalid scoped agent recovery snapshot judgment candidate")
		case !validOptional(judgment.Reason, 4096), !validBounded(judgment.CreatedAt, 256),
			judgment.Replayed, !finite(judgment.Effect):
			return errors.New("invalid scoped agent recovery snapshot judgment metadata")
		}
		judgmentIDs[judgment.JudgmentID], idempotencyKeys[judgment.IdempotencyKey] = true, true
		if judgment.ReversesID == "" {
			effect, err := scopedJudgmentEffect(judgment.Actor, judgment.Disposition)
			if err != nil || judgment.Effect != effect {
				return errors.New("invalid scoped agent recovery snapshot judgment effect")
			}
			originals[judgment.JudgmentID] = judgment
		}
	}
	reversed := make(map[string]bool, len(snapshot.Judgments))
	for _, judgment := range snapshot.Judgments {
		if judgment.ReversesID == "" {
			continue
		}
		original, exists := originals[judgment.ReversesID]
		if !exists || reversed[judgment.ReversesID] || judgment.Disposition != "reversed" ||
			judgment.RunID != original.RunID || judgment.ScopeID != original.ScopeID ||
			judgment.LensID != original.LensID || judgment.AgentID != original.AgentID ||
			judgment.RecordID != original.RecordID || judgment.Actor != original.Actor ||
			judgment.Effect != -original.Effect {
			return errors.New("invalid scoped agent recovery snapshot reversal")
		}
		reversed[judgment.ReversesID] = true
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
