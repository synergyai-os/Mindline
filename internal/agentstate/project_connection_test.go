package agentstate

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

func TestProjectConnectionLifecycleIsStableAndIsolated(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	seedScopedContexts(t, store, ctx, now)
	digest := strings.Repeat("a", 64)
	binding := ScopedContext{ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a"}
	connection, err := store.BindProjectConnection(ctx, digest, binding)
	if err != nil || connection.Status != StatusActive || connection.Replayed {
		t.Fatalf("connection=%+v err=%v", connection, err)
	}
	replay, err := store.BindProjectConnection(ctx, digest, binding)
	if err != nil || !replay.Replayed || replay.CreatedAt != connection.CreatedAt {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	if err := os.Remove(projectConnectionRecoveryPath(path)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, actor, err := store.ResolveProjectConnection(ctx, digest); err != nil || actor.ID != binding.AgentID {
		t.Fatalf("normal resolution read the recovery sidecar: actor=%+v err=%v", actor, err)
	}
	if _, err := store.BindProjectConnection(ctx, digest, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-b",
	}); !errors.Is(err, ErrProjectConnectionConflict) {
		t.Fatalf("conflicting bind err=%v", err)
	}
	_, scope, lens, actor, err := store.ResolveProjectConnection(ctx, digest)
	if err != nil || scope.ID != binding.ScopeID || lens.ID != binding.LensID || actor.ID != binding.AgentID {
		t.Fatalf("resolved scope=%+v lens=%+v actor=%+v err=%v", scope, lens, actor, err)
	}
	archived, err := store.ArchiveProjectConnection(ctx, digest)
	if err != nil || archived.Status != StatusArchived || archived.Replayed {
		t.Fatalf("archived=%+v err=%v", archived, err)
	}
	archiveReplay, err := store.ArchiveProjectConnection(ctx, digest)
	if err != nil || !archiveReplay.Replayed {
		t.Fatalf("archive replay=%+v err=%v", archiveReplay, err)
	}
	if _, _, _, _, err := store.ResolveProjectConnection(ctx, digest); !errors.Is(err, ErrProjectConnectionArchived) {
		t.Fatalf("archived resolve err=%v", err)
	}
	if _, err := store.BindProjectConnection(ctx, digest, binding); !errors.Is(err, ErrProjectConnectionArchived) {
		t.Fatalf("archived rebind err=%v", err)
	}
	status, err := store.Status(ctx)
	if err != nil || status.ProjectConnectionCount != 1 || status.ActiveConnectionCount != 0 || status.ArchivedConnectionCount != 1 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	data, err := os.ReadFile(projectConnectionRecoveryPath(path))
	if err != nil || strings.Contains(string(data), "mlc1_") {
		t.Fatalf("recovery snapshot leaked plaintext handle or failed: err=%v data=%s", err, data)
	}
}

func TestProjectConnectionCapacityRejectsWithoutMutation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 11, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	seedScopedContexts(t, store, ctx, now)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := now.Format(time.RFC3339Nano)
	for index := 0; index < maximumProjectConnections; index++ {
		digest := fmt.Sprintf("%064x", index)
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_connections(
			digest, scope_id, lens_id, agent_id, status, created_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?)`, digest, "scope-a", "lens-one", "agent-a",
			StatusActive, timestamp, timestamp); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.writeProjectConnectionRecoverySnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(projectConnectionRecoveryPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BindProjectConnection(ctx, strings.Repeat("f", 64), ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}); err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("over-capacity bind err=%v", err)
	}
	after, err := os.ReadFile(projectConnectionRecoveryPath(path))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("over-capacity bind mutated recovery projection: err=%v", err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_connections`).Scan(&count); err != nil || count != maximumProjectConnections {
		t.Fatalf("connection count=%d err=%v", count, err)
	}
}

func TestProjectConnectionRecoveryRejectsInvalidTimestampAndSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "agent.sqlite")
	invalid := projectConnectionRecoverySnapshot{
		SchemaVersion: ProjectConnectionSchemaVersion,
		Connections: []ProjectConnection{{
			Digest: strings.Repeat("d", 64), ScopeID: "scope-a", LensID: "lens-one",
			AgentID: "agent-a", Status: StatusActive,
			CreatedAt: "not-a-time", UpdatedAt: "2026-08-11T11:00:00Z",
		}},
	}
	data, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if err := privateio.WriteFile(projectConnectionRecoveryPath(path), data, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readProjectConnectionRecoverySnapshot(path); err == nil {
		t.Fatal("invalid project connection timestamp was accepted")
	}
	if err := os.Remove(projectConnectionRecoveryPath(path)); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target.json")
	if err := privateio.WriteFile(target, []byte("{}"), false); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, projectConnectionRecoveryPath(path)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readProjectConnectionRecoverySnapshot(path); err == nil {
		t.Fatal("symlinked project connection snapshot was accepted")
	}
}

func TestProjectConnectionSurvivesCorruptionRecoveryAndReupgrade(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	ctx := context.Background()
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	seedScopedContexts(t, store, ctx, now)
	digest := strings.Repeat("b", 64)
	archivedDigest := strings.Repeat("c", 64)
	binding := ScopedContext{ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a"}
	if _, err := store.BindProjectConnection(ctx, digest, binding); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BindProjectConnection(ctx, archivedDigest, binding); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArchiveProjectConnection(ctx, archivedDigest); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`DROP TABLE project_connections; DROP TABLE project_connection_meta`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	reupgraded, err := Open(path, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, actor, err := reupgraded.ResolveProjectConnection(ctx, digest); err != nil || actor.ID != binding.AgentID {
		t.Fatalf("reupgrade actor=%+v err=%v", actor, err)
	}
	if _, _, _, _, err := reupgraded.ResolveProjectConnection(ctx, archivedDigest); !errors.Is(err, ErrProjectConnectionArchived) {
		t.Fatalf("reupgrade lost archived tombstone: %v", err)
	}
	if err := reupgraded.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("corrupt-agent-state"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	interrupted := false
	if _, _, err := openRecovering(path, func() time.Time { return now.Add(2 * time.Minute) }, recoveryHooks{
		rename: os.Rename,
		beforeRestore: func() error {
			if !interrupted {
				interrupted = true
				return errors.New("injected recovery interruption")
			}
			return nil
		},
	}); err == nil {
		t.Fatal("injected recovery interruption was accepted")
	}
	recovered, quarantine, err := OpenRecovering(path, func() time.Time { return now.Add(3 * time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if quarantine == "" {
		t.Fatal("corrupt database was not quarantined")
	}
	if _, _, _, actor, err := recovered.ResolveProjectConnection(ctx, digest); err != nil || actor.ID != binding.AgentID {
		t.Fatalf("recovered actor=%+v err=%v", actor, err)
	}
	if _, _, _, _, err := recovered.ResolveProjectConnection(ctx, archivedDigest); !errors.Is(err, ErrProjectConnectionArchived) {
		t.Fatalf("recovery reactivated archived tombstone: %v", err)
	}
}

func TestProjectConnectionPostCommitFailureRequiresRetryAndRestartKeepsAcknowledgedState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	ctx := context.Background()
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	seedScopedContexts(t, store, ctx, now)
	digest := strings.Repeat("9", 64)
	secondDigest := strings.Repeat("7", 64)
	thirdDigest := strings.Repeat("6", 64)
	binding := ScopedContext{ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a"}
	before, err := os.ReadFile(projectConnectionRecoveryPath(path))
	if err != nil {
		t.Fatal(err)
	}
	store.projectConnectionWriteHook = func() error { return errors.New("injected post-commit failure") }
	if _, err := store.BindProjectConnection(ctx, digest, binding); !errors.Is(err, ErrProjectConnectionOutcomeUnknown) {
		t.Fatalf("bind outcome err=%v", err)
	}
	afterFailure, err := os.ReadFile(projectConnectionRecoveryPath(path))
	if err != nil || !bytes.Equal(before, afterFailure) {
		t.Fatalf("failed bind changed acknowledged recovery: err=%v", err)
	}
	if _, _, _, _, err := store.ResolveProjectConnection(ctx, digest); !errors.Is(err, ErrProjectConnectionOutcomeUnknown) {
		t.Fatalf("unacknowledged bind resolved before identical retry: %v", err)
	}
	if _, err := store.BindProjectConnection(ctx, secondDigest, binding); !errors.Is(err, ErrProjectConnectionOutcomeUnknown) {
		t.Fatalf("different bind was allowed while an outcome was pending: %v", err)
	}
	if _, found, err := store.projectConnectionByDigest(ctx, secondDigest); err != nil || found {
		t.Fatalf("different bind mutated state: found=%v err=%v", found, err)
	}
	store.projectConnectionWriteHook = nil
	replayed, err := store.BindProjectConnection(ctx, digest, binding)
	if err != nil || !replayed.Replayed {
		t.Fatalf("identical bind retry failed: connection=%+v err=%v", replayed, err)
	}
	if _, _, _, actor, err := store.ResolveProjectConnection(ctx, digest); err != nil || actor.ID != binding.AgentID {
		t.Fatalf("acknowledged bind did not resolve: actor=%+v err=%v", actor, err)
	}
	store.projectConnectionWriteHook = func() error { return errors.New("injected second bind failure") }
	if _, err := store.BindProjectConnection(ctx, secondDigest, binding); !errors.Is(err, ErrProjectConnectionOutcomeUnknown) {
		t.Fatalf("second bind outcome err=%v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, quarantine, err := OpenRecovering(path, func() time.Time { return now.Add(time.Minute) })
	if err != nil || quarantine == "" {
		t.Fatalf("restart recovery quarantine=%q err=%v", quarantine, err)
	}
	if _, _, _, actor, err := recovered.ResolveProjectConnection(ctx, digest); err != nil || actor.ID != binding.AgentID {
		t.Fatalf("restart lost acknowledged bind: actor=%+v err=%v", actor, err)
	}
	if _, _, _, _, err := recovered.ResolveProjectConnection(ctx, secondDigest); !errors.Is(err, ErrProjectConnectionNotFound) {
		t.Fatalf("restart promoted unacknowledged bind: %v", err)
	}
	recovered.projectConnectionWriteHook = func() error { return errors.New("injected archive failure") }
	if _, err := recovered.ArchiveProjectConnection(ctx, digest); !errors.Is(err, ErrProjectConnectionOutcomeUnknown) {
		t.Fatalf("archive outcome err=%v", err)
	}
	if _, _, _, _, err := recovered.ResolveProjectConnection(ctx, digest); !errors.Is(err, ErrProjectConnectionOutcomeUnknown) {
		t.Fatalf("unacknowledged archive resolved before repair: %v", err)
	}
	if _, err := recovered.BindProjectConnection(ctx, thirdDigest, binding); !errors.Is(err, ErrProjectConnectionOutcomeUnknown) {
		t.Fatalf("different mutation was allowed while archive outcome was pending: %v", err)
	}
	if _, found, err := recovered.projectConnectionByDigest(ctx, thirdDigest); err != nil || found {
		t.Fatalf("different mutation changed state: found=%v err=%v", found, err)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, quarantine, err = OpenRecovering(path, func() time.Time { return now.Add(2 * time.Minute) })
	if err != nil || quarantine == "" {
		t.Fatalf("archive restart quarantine=%q err=%v", quarantine, err)
	}
	if _, _, _, actor, err := recovered.ResolveProjectConnection(ctx, digest); err != nil || actor.ID != binding.AgentID {
		t.Fatalf("restart promoted unacknowledged archive: actor=%+v err=%v", actor, err)
	}
	if _, err := recovered.ArchiveProjectConnection(ctx, digest); err != nil {
		t.Fatalf("identical archive retry failed: %v", err)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, func() time.Time { return now.Add(3 * time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, _, _, _, err := reopened.ResolveProjectConnection(ctx, digest); !errors.Is(err, ErrProjectConnectionArchived) {
		t.Fatalf("acknowledged archive did not survive restart: %v", err)
	}
	status, err := reopened.Status(ctx)
	if err != nil || status.AgentActorCount != 3 || status.ProjectConnectionCount != 1 ||
		status.ActiveConnectionCount != 0 || status.ArchivedConnectionCount != 1 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestProjectConnectionMissingRecoverySnapshotFailsClosedWithoutMutation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	ctx := context.Background()
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	seedScopedContexts(t, store, ctx, now)
	if _, err := store.BindProjectConnection(ctx, strings.Repeat("4", 64), ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(projectConnectionRecoveryPath(path)); err != nil {
		t.Fatal(err)
	}
	if adopted, err := readProjectConnectionAdoptionMarker(path); err != nil || !adopted {
		t.Fatalf("project connection adoption was not durably marked: adopted=%v err=%v", adopted, err)
	}
	if recovered, quarantine, err := OpenRecovering(path, func() time.Time { return now.Add(time.Minute) }); err == nil {
		if recovered != nil {
			recovered.Close()
		}
		t.Fatalf("missing recovery snapshot was accepted with quarantine %q", quarantine)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed-closed recovery mutated database: err=%v", err)
	}
	if _, err := os.Lstat(recoveryMarkerPath(path)); !os.IsNotExist(err) {
		t.Fatalf("failed-closed recovery created a marker: %v", err)
	}
	if matches, err := filepath.Glob(path + ".corrupt-*"); err != nil || len(matches) != 0 {
		t.Fatalf("failed-closed recovery quarantined state: matches=%v err=%v", matches, err)
	}
	if _, err := os.Lstat(projectConnectionRecoveryPath(path)); !os.IsNotExist(err) {
		t.Fatalf("failed-closed recovery recreated missing snapshot: %v", err)
	}
}

func TestProjectConnectionRecoveryResumeRejectsMissingBoundSnapshot(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 15, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	ctx := context.Background()
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	seedScopedContexts(t, store, ctx, now)
	if _, err := store.BindProjectConnection(ctx, strings.Repeat("3", 64), ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt-agent-state"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openRecovering(path, func() time.Time { return now.Add(time.Minute) }, recoveryHooks{
		rename: os.Rename,
		beforeRestore: func() error {
			return errors.New("injected recovery interruption")
		},
	}); err == nil {
		t.Fatal("injected recovery interruption was accepted")
	}
	marker, err := readRecoveryMarker(path)
	if err != nil {
		t.Fatal(err)
	}
	binding, present, err := readProjectRecoveryBinding(path, marker)
	if err != nil || !present || !binding.ProjectConnectionsAdopted || !binding.ProjectSnapshotPresent ||
		binding.ProjectSnapshotFingerprint == "" {
		t.Fatalf("binding=%+v present=%v err=%v", binding, present, err)
	}
	var legacyMarker struct {
		SchemaVersion   string `json:"schema_version"`
		QuarantineBase  string `json:"quarantine_base"`
		StagePath       string `json:"stage_path"`
		SnapshotPresent bool   `json:"snapshot_present"`
	}
	if err := privateio.ReadJSONStrictBounded(filepath.Dir(path), recoveryMarkerPath(path),
		maximumRecoveryMarkerBytes, &legacyMarker); err != nil ||
		legacyMarker.SchemaVersion != "mindline-agent-recovery-marker/v0.1" {
		t.Fatalf("previous binary cannot read recovery marker: marker=%+v err=%v", legacyMarker, err)
	}
	quarantineBefore, err := os.ReadFile(marker.QuarantineBase)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(projectConnectionRecoveryPath(path)); err != nil {
		t.Fatal(err)
	}
	if recovered, _, err := OpenRecovering(path, func() time.Time { return now.Add(2 * time.Minute) }); err == nil {
		if recovered != nil {
			recovered.Close()
		}
		t.Fatal("resume accepted a missing marker-bound project snapshot")
	}
	quarantineAfter, err := os.ReadFile(marker.QuarantineBase)
	if err != nil || !bytes.Equal(quarantineBefore, quarantineAfter) {
		t.Fatalf("failed resume changed quarantine: err=%v", err)
	}
	if _, err := os.Lstat(recoveryMarkerPath(path)); err != nil {
		t.Fatalf("failed resume removed recovery marker: %v", err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("failed resume promoted a canonical database: %v", err)
	}
}

func TestProjectConnectionNewBinaryResumesProvenPreWP56V01RecoveryMarker(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "state")
	if err := privateio.PrepareDir(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "agent.sqlite")
	if err := os.WriteFile(path, []byte("pre-wp56-corrupt-agent-state"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	if err := privateio.WriteJSON(recoveryPath(path), recoverySnapshot{
		SchemaVersion: recoverySchemaVersion,
		Lenses:        []Lens{},
		Judgments:     []Judgment{},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openRecovering(path, func() time.Time { return now.Add(time.Minute) }, recoveryHooks{
		rename: os.Rename,
		beforeRestore: func() error {
			return errors.New("injected legacy recovery interruption")
		},
	}); err == nil {
		t.Fatal("injected recovery interruption was accepted")
	}
	if err := os.Remove(projectRecoveryBindingPath(path)); err != nil {
		t.Fatal(err)
	}
	if adopted, err := readProjectConnectionAdoptionMarker(path); err != nil || adopted {
		t.Fatalf("test state was not proven pre-WP56: adopted=%v err=%v", adopted, err)
	}
	if _, present, err := readProjectConnectionRecoverySnapshot(path); err != nil || present {
		t.Fatalf("test state unexpectedly had a project snapshot: present=%v err=%v", present, err)
	}
	recovered, quarantine, err := OpenRecovering(path, func() time.Time { return now.Add(2 * time.Minute) })
	if err != nil || quarantine == "" {
		t.Fatalf("legacy marker resume quarantine=%q err=%v", quarantine, err)
	}
	defer recovered.Close()
	if adopted, err := readProjectConnectionAdoptionMarker(path); err != nil || !adopted {
		t.Fatalf("legacy marker resume did not adopt project connections: adopted=%v err=%v", adopted, err)
	}
}

func TestProjectConnectionReupgradeRestoreAndSchemaMarkerAreAtomic(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 13, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	seedScopedContexts(t, store, ctx, now)
	digest := strings.Repeat("8", 64)
	if _, err := store.BindProjectConnection(ctx, digest, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE project_connections`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE project_connection_meta`); err != nil {
		t.Fatal(err)
	}
	store.projectConnectionInitHook = func() error { return errors.New("injected initialization interruption") }
	if err := store.initializeProjectConnections(ctx); err == nil {
		t.Fatal("interrupted project connection initialization succeeded")
	}
	for _, table := range []string{"project_connections", "project_connection_meta"} {
		exists, err := store.projectConnectionTableExists(ctx, table)
		if err != nil || exists {
			t.Fatalf("transaction left partial table %s: exists=%v err=%v", table, exists, err)
		}
	}
	store.projectConnectionInitHook = nil
	if err := store.initializeProjectConnections(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, _, actor, err := store.ResolveProjectConnection(ctx, digest); err != nil || actor.ID != "agent-a" {
		t.Fatalf("atomic retry actor=%+v err=%v", actor, err)
	}
}
