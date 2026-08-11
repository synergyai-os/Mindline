package agentstate

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	maximumProjectConnections        = 256
	maximumProjectConnectionBytes    = 256 << 10
	maximumProjectAdoptionBytes      = 4 << 10
	projectConnectionRecoverySuffix  = ".project-connections-recovery.json"
	projectConnectionAdoptionSuffix  = ".project-connections-adopted.json"
	projectConnectionAdoptionVersion = "mindline-project-connection-adoption/v0.1"
)

var (
	ErrProjectConnectionNotFound       = errors.New("project connection not found")
	ErrProjectConnectionConflict       = errors.New("project connection conflicts with existing binding")
	ErrProjectConnectionArchived       = errors.New("project connection is archived")
	ErrProjectConnectionOutcomeUnknown = errors.New("project connection outcome requires identical retry")
)

type projectConnectionRecoverySnapshot struct {
	SchemaVersion string              `json:"schema_version"`
	Connections   []ProjectConnection `json:"connections"`
}

type projectConnectionAdoption struct {
	SchemaVersion string `json:"schema_version"`
}

func projectConnectionRecoveryPath(databasePath string) string {
	return databasePath + projectConnectionRecoverySuffix
}

func projectConnectionAdoptionPath(databasePath string) string {
	return databasePath + projectConnectionAdoptionSuffix
}

func (store *Store) initializeProjectConnections(ctx context.Context) error {
	metaExists, err := store.projectConnectionTableExists(ctx, "project_connection_meta")
	if err != nil {
		return err
	}
	connectionsExist, err := store.projectConnectionTableExists(ctx, "project_connections")
	if err != nil {
		return err
	}
	if metaExists {
		if !connectionsExist {
			return fmt.Errorf("%w: incomplete project connection schema", ErrCorrupt)
		}
		var version string
		if err := store.db.QueryRowContext(ctx,
			`SELECT value FROM project_connection_meta WHERE key='schema_version'`).Scan(&version); err != nil {
			return errors.New("read project connection schema")
		}
		if version != ProjectConnectionSchemaVersion {
			return errors.New("unsupported project connection schema")
		}
		expected, present, err := readProjectConnectionRecoverySnapshot(store.path)
		if err != nil {
			return err
		}
		actual, buildErr := store.buildProjectConnectionRecoverySnapshot(ctx)
		if buildErr != nil || !present || !reflect.DeepEqual(actual, expected) {
			return fmt.Errorf("%w: project connections differ from acknowledged recovery", ErrCorrupt)
		}
		return ensureProjectConnectionAdoptionMarker(store.path)
	}
	if connectionsExist {
		var count int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_connections`).Scan(&count); err != nil || count != 0 {
			return errors.New("restore project connections into non-empty state")
		}
	}
	snapshot, present, err := readProjectConnectionRecoverySnapshot(store.path)
	if err != nil {
		return err
	}
	if !present {
		snapshot = projectConnectionRecoverySnapshot{
			SchemaVersion: ProjectConnectionSchemaVersion,
			Connections:   []ProjectConnection{},
		}
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("initialize project connections")
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS project_connection_meta (
			key TEXT PRIMARY KEY NOT NULL,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS project_connections (
			digest TEXT PRIMARY KEY NOT NULL,
			scope_id TEXT NOT NULL,
			lens_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('active', 'archived')),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(scope_id, lens_id) REFERENCES scoped_lenses(scope_id, id),
			FOREIGN KEY(agent_id) REFERENCES agent_actors(id)
		)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return errors.New("initialize project connections")
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_connections`).Scan(&count); err != nil || count != 0 {
		return errors.New("restore project connections into non-empty state")
	}
	for _, connection := range snapshot.Connections {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_connections(
			digest, scope_id, lens_id, agent_id, status, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?)`, connection.Digest, connection.ScopeID,
			connection.LensID, connection.AgentID, connection.Status,
			connection.CreatedAt, connection.UpdatedAt); err != nil {
			return errors.New("restore project connection recovery snapshot")
		}
	}
	if store.projectConnectionInitHook != nil {
		if err := store.projectConnectionInitHook(); err != nil {
			return errors.New("initialize project connection schema")
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_connection_meta(key, value)
		VALUES('schema_version', ?)`, ProjectConnectionSchemaVersion); err != nil {
		return errors.New("initialize project connection schema")
	}
	if err := tx.Commit(); err != nil {
		return errors.New("initialize project connection schema")
	}
	if !present {
		if err := store.writeProjectConnectionRecoverySnapshot(ctx); err != nil {
			return err
		}
	}
	return ensureProjectConnectionAdoptionMarker(store.path)
}

func (store *Store) projectConnectionTableExists(ctx context.Context, name string) (bool, error) {
	var count int
	if err := store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&count); err != nil {
		return false, errors.New("inspect project connection schema")
	}
	return count == 1, nil
}

func (store *Store) BindProjectConnection(
	ctx context.Context, digest string, binding ScopedContext,
) (ProjectConnection, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	if !validProjectConnectionDigest(digest) {
		return ProjectConnection{}, errors.New("invalid project connection")
	}
	scope, lens, actor, err := store.resolveScopedContext(ctx, binding)
	if err != nil {
		return ProjectConnection{}, err
	}
	binding = ScopedContext{ScopeID: scope.ID, LensID: lens.ID, AgentID: actor.ID}
	acknowledged, current, pending, err := store.projectConnectionRecoveryState(ctx)
	if err != nil {
		return ProjectConnection{}, err
	}
	if pending {
		existing, found := projectConnectionInSnapshot(current, digest)
		if !found || existing.Status != StatusActive || existing.ScopeID != binding.ScopeID ||
			existing.LensID != binding.LensID || existing.AgentID != binding.AgentID ||
			!reflect.DeepEqual(projectConnectionSnapshotWith(acknowledged, existing), current) {
			return ProjectConnection{}, ErrProjectConnectionOutcomeUnknown
		}
		if err := store.writeProjectConnectionRecoverySnapshot(ctx); err != nil {
			return ProjectConnection{}, ErrProjectConnectionOutcomeUnknown
		}
		existing.Replayed = true
		return existing, nil
	}
	if existing, found, err := store.projectConnectionByDigest(ctx, digest); err != nil {
		return ProjectConnection{}, err
	} else if found {
		if existing.Status == StatusArchived {
			return ProjectConnection{}, ErrProjectConnectionArchived
		}
		if existing.ScopeID != binding.ScopeID || existing.LensID != binding.LensID || existing.AgentID != binding.AgentID {
			return ProjectConnection{}, ErrProjectConnectionConflict
		}
		if err := store.writeProjectConnectionRecoverySnapshot(ctx); err != nil {
			return ProjectConnection{}, ErrProjectConnectionOutcomeUnknown
		}
		existing.Replayed = true
		return existing, nil
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_connections`).Scan(&count); err != nil {
		return ProjectConnection{}, errors.New("count project connections")
	}
	if count >= maximumProjectConnections {
		return ProjectConnection{}, errors.New("project connection capacity reached")
	}
	now := store.now().UTC().Format(timeFormat)
	connection := ProjectConnection{
		Digest: digest, ScopeID: binding.ScopeID, LensID: binding.LensID, AgentID: binding.AgentID,
		Status: StatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.preflightProjectConnectionRecovery(ctx, connection); err != nil {
		return ProjectConnection{}, err
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO project_connections(
		digest, scope_id, lens_id, agent_id, status, created_at, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?)`, connection.Digest, connection.ScopeID,
		connection.LensID, connection.AgentID, connection.Status,
		connection.CreatedAt, connection.UpdatedAt)
	if err != nil {
		return ProjectConnection{}, errors.New("save project connection")
	}
	if err := store.writeProjectConnectionRecoverySnapshot(ctx); err != nil {
		return ProjectConnection{}, ErrProjectConnectionOutcomeUnknown
	}
	return connection, nil
}

func (store *Store) ResolveProjectConnection(
	ctx context.Context, digest string,
) (ProjectConnection, Scope, ScopedLens, AgentActor, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	if !validProjectConnectionDigest(digest) {
		return ProjectConnection{}, Scope{}, ScopedLens{}, AgentActor{}, errors.New("invalid project connection")
	}
	_, _, pending, err := store.projectConnectionRecoveryState(ctx)
	if err != nil {
		return ProjectConnection{}, Scope{}, ScopedLens{}, AgentActor{}, err
	}
	if pending {
		return ProjectConnection{}, Scope{}, ScopedLens{}, AgentActor{}, ErrProjectConnectionOutcomeUnknown
	}
	connection, found, err := store.projectConnectionByDigest(ctx, digest)
	if err != nil {
		return ProjectConnection{}, Scope{}, ScopedLens{}, AgentActor{}, err
	}
	if !found {
		return ProjectConnection{}, Scope{}, ScopedLens{}, AgentActor{}, ErrProjectConnectionNotFound
	}
	if connection.Status != StatusActive {
		return ProjectConnection{}, Scope{}, ScopedLens{}, AgentActor{}, ErrProjectConnectionArchived
	}
	scope, lens, actor, err := store.resolveScopedContext(ctx, ScopedContext{
		ScopeID: connection.ScopeID, LensID: connection.LensID, AgentID: connection.AgentID,
	})
	if err != nil {
		return ProjectConnection{}, Scope{}, ScopedLens{}, AgentActor{}, errors.New("project connection target is unavailable")
	}
	return connection, scope, lens, actor, nil
}

func (store *Store) ArchiveProjectConnection(ctx context.Context, digest string) (ProjectConnection, error) {
	store.mutationMu.Lock()
	defer store.mutationMu.Unlock()
	if !validProjectConnectionDigest(digest) {
		return ProjectConnection{}, errors.New("invalid project connection")
	}
	acknowledged, current, pending, err := store.projectConnectionRecoveryState(ctx)
	if err != nil {
		return ProjectConnection{}, err
	}
	if pending {
		existing, found := projectConnectionInSnapshot(current, digest)
		if !found || existing.Status != StatusArchived ||
			!reflect.DeepEqual(projectConnectionSnapshotWith(acknowledged, existing), current) {
			return ProjectConnection{}, ErrProjectConnectionOutcomeUnknown
		}
		if err := store.writeProjectConnectionRecoverySnapshot(ctx); err != nil {
			return ProjectConnection{}, ErrProjectConnectionOutcomeUnknown
		}
		existing.Replayed = true
		return existing, nil
	}
	connection, found, err := store.projectConnectionByDigest(ctx, digest)
	if err != nil {
		return ProjectConnection{}, err
	}
	if !found {
		return ProjectConnection{}, ErrProjectConnectionNotFound
	}
	if connection.Status == StatusArchived {
		if err := store.writeProjectConnectionRecoverySnapshot(ctx); err != nil {
			return ProjectConnection{}, ErrProjectConnectionOutcomeUnknown
		}
		connection.Replayed = true
		return connection, nil
	}
	connection.Status = StatusArchived
	connection.UpdatedAt = store.now().UTC().Format(timeFormat)
	if err := store.preflightProjectConnectionRecovery(ctx, connection); err != nil {
		return ProjectConnection{}, err
	}
	result, err := store.db.ExecContext(ctx, `UPDATE project_connections SET status=?, updated_at=?
		WHERE digest=? AND status=?`, StatusArchived, connection.UpdatedAt, digest, StatusActive)
	if err != nil {
		return ProjectConnection{}, errors.New("archive project connection")
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return ProjectConnection{}, errors.New("archive project connection")
	}
	if err := store.writeProjectConnectionRecoverySnapshot(ctx); err != nil {
		return ProjectConnection{}, ErrProjectConnectionOutcomeUnknown
	}
	return connection, nil
}

func (store *Store) ListProjectConnections(ctx context.Context) ([]ProjectConnection, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT digest, scope_id, lens_id, agent_id,
		status, created_at, updated_at FROM project_connections ORDER BY digest`)
	if err != nil {
		return nil, errors.New("list project connections")
	}
	defer rows.Close()
	result := []ProjectConnection{}
	for rows.Next() {
		var connection ProjectConnection
		if err := rows.Scan(&connection.Digest, &connection.ScopeID, &connection.LensID,
			&connection.AgentID, &connection.Status, &connection.CreatedAt,
			&connection.UpdatedAt); err != nil {
			return nil, errors.New("list project connections")
		}
		result = append(result, connection)
	}
	return result, rows.Err()
}

func (store *Store) projectConnectionByDigest(ctx context.Context, digest string) (ProjectConnection, bool, error) {
	var connection ProjectConnection
	err := store.db.QueryRowContext(ctx, `SELECT digest, scope_id, lens_id, agent_id,
		status, created_at, updated_at FROM project_connections WHERE digest=?`, digest).Scan(
		&connection.Digest, &connection.ScopeID, &connection.LensID, &connection.AgentID,
		&connection.Status, &connection.CreatedAt, &connection.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectConnection{}, false, nil
	}
	if err != nil {
		return ProjectConnection{}, false, errors.New("read project connection")
	}
	return connection, true, nil
}

func validProjectConnectionDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func projectConnectionSnapshotWith(
	snapshot projectConnectionRecoverySnapshot, proposed ProjectConnection,
) projectConnectionRecoverySnapshot {
	found := false
	for index := range snapshot.Connections {
		if snapshot.Connections[index].Digest == proposed.Digest {
			proposed.CreatedAt = snapshot.Connections[index].CreatedAt
			snapshot.Connections[index] = proposed
			found = true
			break
		}
	}
	if !found {
		snapshot.Connections = append(snapshot.Connections, proposed)
	}
	sort.Slice(snapshot.Connections, func(i, j int) bool {
		return snapshot.Connections[i].Digest < snapshot.Connections[j].Digest
	})
	return snapshot
}

func (store *Store) projectConnectionRecoveryState(
	ctx context.Context,
) (projectConnectionRecoverySnapshot, projectConnectionRecoverySnapshot, bool, error) {
	acknowledged, present, err := readProjectConnectionRecoverySnapshot(store.path)
	if err != nil || !present {
		return projectConnectionRecoverySnapshot{}, projectConnectionRecoverySnapshot{}, false,
			errors.New("read acknowledged project connections")
	}
	current, err := store.buildProjectConnectionRecoverySnapshot(ctx)
	if err != nil {
		return projectConnectionRecoverySnapshot{}, projectConnectionRecoverySnapshot{}, false, err
	}
	return acknowledged, current, !reflect.DeepEqual(acknowledged, current), nil
}

func projectConnectionInSnapshot(
	snapshot projectConnectionRecoverySnapshot, digest string,
) (ProjectConnection, bool) {
	for _, connection := range snapshot.Connections {
		if connection.Digest == digest {
			return connection, true
		}
	}
	return ProjectConnection{}, false
}

func (store *Store) preflightProjectConnectionRecovery(ctx context.Context, proposed ProjectConnection) error {
	snapshot, err := store.buildProjectConnectionRecoverySnapshot(ctx)
	if err != nil {
		return err
	}
	snapshot = projectConnectionSnapshotWith(snapshot, proposed)
	if _, err := encodeProjectConnectionRecoverySnapshot(snapshot); err != nil {
		return errors.New("preflight project connection recovery snapshot")
	}
	return nil
}

func (store *Store) buildProjectConnectionRecoverySnapshot(ctx context.Context) (projectConnectionRecoverySnapshot, error) {
	connections, err := store.ListProjectConnections(ctx)
	if err != nil {
		return projectConnectionRecoverySnapshot{}, errors.New("build project connection recovery snapshot")
	}
	snapshot := projectConnectionRecoverySnapshot{
		SchemaVersion: ProjectConnectionSchemaVersion, Connections: connections,
	}
	if err := validateProjectConnectionRecoverySnapshot(snapshot); err != nil {
		return projectConnectionRecoverySnapshot{}, errors.New("build project connection recovery snapshot")
	}
	return snapshot, nil
}

func (store *Store) writeProjectConnectionRecoverySnapshot(ctx context.Context) error {
	snapshot, err := store.buildProjectConnectionRecoverySnapshot(ctx)
	if err != nil {
		return err
	}
	data, err := encodeProjectConnectionRecoverySnapshot(snapshot)
	if err != nil {
		return err
	}
	if store.projectConnectionWriteHook != nil {
		if err := store.projectConnectionWriteHook(); err != nil {
			return errors.New("write project connection recovery snapshot")
		}
	}
	if err := privateio.WriteFile(projectConnectionRecoveryPath(store.path), data, false); err != nil {
		return errors.New("write project connection recovery snapshot")
	}
	written, present, err := readProjectConnectionRecoverySnapshot(store.path)
	if err != nil || !present || !reflect.DeepEqual(written, snapshot) {
		return errors.New("verify project connection recovery snapshot")
	}
	return nil
}

func readProjectConnectionRecoverySnapshot(databasePath string) (projectConnectionRecoverySnapshot, bool, error) {
	path := projectConnectionRecoveryPath(databasePath)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return projectConnectionRecoverySnapshot{}, false, nil
	} else if err != nil {
		return projectConnectionRecoverySnapshot{}, false, errors.New("read project connection recovery snapshot")
	}
	var snapshot projectConnectionRecoverySnapshot
	if err := privateio.ReadJSONStrictBounded(filepath.Dir(path), path,
		maximumProjectConnectionBytes, &snapshot); err != nil ||
		validateProjectConnectionRecoverySnapshot(snapshot) != nil {
		return projectConnectionRecoverySnapshot{}, false, errors.New("read project connection recovery snapshot")
	}
	return snapshot, true, nil
}

func readProjectConnectionAdoptionMarker(databasePath string) (bool, error) {
	path := projectConnectionAdoptionPath(databasePath)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, errors.New("read project connection adoption marker")
	}
	var marker projectConnectionAdoption
	if err := privateio.ReadJSONStrictBounded(filepath.Dir(path), path,
		maximumProjectAdoptionBytes, &marker); err != nil ||
		marker.SchemaVersion != projectConnectionAdoptionVersion {
		return false, errors.New("read project connection adoption marker")
	}
	return true, nil
}

func ensureProjectConnectionAdoptionMarker(databasePath string) error {
	present, err := readProjectConnectionAdoptionMarker(databasePath)
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	if err := privateio.WriteJSONNoReplace(projectConnectionAdoptionPath(databasePath),
		projectConnectionAdoption{SchemaVersion: projectConnectionAdoptionVersion}); err != nil {
		if present, readErr := readProjectConnectionAdoptionMarker(databasePath); readErr != nil || !present {
			return errors.New("write project connection adoption marker")
		}
	}
	return nil
}

func encodeProjectConnectionRecoverySnapshot(snapshot projectConnectionRecoverySnapshot) ([]byte, error) {
	if err := validateProjectConnectionRecoverySnapshot(snapshot); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, errors.New("encode project connection recovery snapshot")
	}
	data = append(data, '\n')
	if len(data) > maximumProjectConnectionBytes {
		return nil, errors.New("project connection recovery snapshot exceeds limit")
	}
	return data, nil
}

func validateProjectConnectionRecoverySnapshot(snapshot projectConnectionRecoverySnapshot) error {
	if snapshot.SchemaVersion != ProjectConnectionSchemaVersion ||
		len(snapshot.Connections) > maximumProjectConnections {
		return errors.New("invalid project connection recovery snapshot")
	}
	seen := make(map[string]bool, len(snapshot.Connections))
	for _, connection := range snapshot.Connections {
		if !validProjectConnectionDigest(connection.Digest) || seen[connection.Digest] ||
			!validBounded(connection.ScopeID, 256) || !validBounded(connection.LensID, 256) ||
			!validBounded(connection.AgentID, 256) || !validScopedStatus(connection.Status) ||
			!validProjectConnectionTime(connection.CreatedAt) ||
			!validProjectConnectionTime(connection.UpdatedAt) ||
			connection.Replayed || containsSecretLikeAny(connection.ScopeID, connection.LensID,
			connection.AgentID, connection.Status, connection.CreatedAt, connection.UpdatedAt) {
			return errors.New("invalid project connection recovery snapshot")
		}
		seen[connection.Digest] = true
	}
	return nil
}

func validProjectConnectionTime(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && value == parsed.Format(time.RFC3339Nano)
}

func projectConnectionRecoverySnapshotMatches(
	ctx context.Context, store *Store, expected projectConnectionRecoverySnapshot, present bool,
) error {
	if !present {
		expected = projectConnectionRecoverySnapshot{
			SchemaVersion: ProjectConnectionSchemaVersion, Connections: []ProjectConnection{},
		}
	}
	actual, err := store.buildProjectConnectionRecoverySnapshot(ctx)
	if err != nil || !reflect.DeepEqual(actual, expected) {
		return errors.New("verify project connection recovery snapshot")
	}
	return nil
}

const timeFormat = time.RFC3339Nano
