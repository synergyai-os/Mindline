package agentstate

import (
	"bytes"
	"context"
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

func TestReupgradePreservesScopedStateAndQuarantinesLegacyAgentWrites(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedScopedContexts(t, store, ctx, now)
	if _, err := store.ApplyScopedJudgment(ctx, ScopedJudgmentRequest{
		RetryToken: "pre-rollback-retry-123456", RunID: "run-scope-a-agent-a",
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
		RecordID: "record-one", Actor: FeedbackAgent, Disposition: "used",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	created := now.Add(time.Minute).Format(time.RFC3339Nano)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO lenses(id,name,query,created_at,updated_at) VALUES(?,?,?,?,?)`, []any{"legacy-lens", "Legacy", "legacy query", created, created}},
		{`INSERT INTO retrieval_runs(run_id,query,lens_id,retrieval_method,library_fingerprint,created_at) VALUES(?,?,?,?,?,?)`, []any{"legacy-run", "legacy", "legacy-lens", "legacy", "fingerprint", created}},
		{`INSERT INTO retrieval_candidates(run_id,rank,record_id,final_score,components_json) VALUES(?,?,?,?,?)`, []any{"legacy-run", 1, "legacy-record", 1.0, []byte(`{"final":1}`)}},
	}
	for _, statement := range statements {
		if _, err := legacy.Exec(statement.query, statement.args...); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	for _, judgment := range []struct {
		key, actor string
		effect     float64
	}{
		{key: "legacy-owner", actor: "user", effect: 1},
		{key: "legacy-agent", actor: "agent", effect: 0.25},
	} {
		if _, err := legacy.Exec(`INSERT INTO judgments(
			judgment_id,idempotency_key,run_id,lens_id,record_id,actor,disposition,
			reason,reverses_judgment_id,effect,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, stableID("judgment", judgment.key), judgment.key,
			"legacy-run", "legacy-lens", "legacy-record", judgment.actor, "used", "", "",
			judgment.effect, created); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path, func() time.Time { return now.Add(2 * time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertScopedRelevance(t, reopened, ctx, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}, 0.025)
	if _, err := reopened.GetScopedLens(ctx, OwnerRootScopeID, "legacy-lens"); err == nil {
		t.Fatal("post-upgrade legacy lens entered scoped recall")
	}
	var scopedRows, legacyRows int
	if err := reopened.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scoped_judgments WHERE run_id='legacy-run'`,
	).Scan(&scopedRows); err != nil || scopedRows != 0 {
		t.Fatalf("post-upgrade scoped judgments=%d err=%v", scopedRows, err)
	}
	if err := reopened.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM judgments WHERE run_id='legacy-run'`,
	).Scan(&legacyRows); err != nil || legacyRows != 2 {
		t.Fatalf("quarantined legacy judgments=%d err=%v", legacyRows, err)
	}
	legacyActor, err := reopened.GetAgentActor(ctx, LegacyAgentActorID)
	if err != nil || legacyActor.Status != StatusArchived {
		t.Fatalf("legacy actor=%+v err=%v", legacyActor, err)
	}
}

func TestLegacyOwnerReversalOfAgentFeedbackStaysQuarantined(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 8, 9, 30, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), privateio.DirMode); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE lenses(id TEXT PRIMARY KEY, name TEXT, query TEXT, created_at TEXT, updated_at TEXT)`,
		`CREATE TABLE retrieval_runs(run_id TEXT PRIMARY KEY, query TEXT, lens_id TEXT, retrieval_method TEXT, library_fingerprint TEXT, created_at TEXT)`,
		`CREATE TABLE retrieval_candidates(run_id TEXT, rank INTEGER, record_id TEXT, final_score REAL, components_json BLOB, PRIMARY KEY(run_id,record_id))`,
		`CREATE TABLE judgments(judgment_id TEXT PRIMARY KEY, idempotency_key TEXT UNIQUE, run_id TEXT, lens_id TEXT, record_id TEXT, actor TEXT, disposition TEXT, reason TEXT, reverses_judgment_id TEXT, effect REAL, created_at TEXT)`,
	} {
		if _, err := legacy.Exec(statement); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	created := now.Format(time.RFC3339Nano)
	originalID := stableID("judgment", "legacy-agent-original")
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO lenses VALUES(?,?,?,?,?)`, []any{"legacy-lens", "Legacy", "legacy", created, created}},
		{`INSERT INTO retrieval_runs VALUES(?,?,?,?,?,?)`, []any{"legacy-run", "legacy", "legacy-lens", "test", "fingerprint", created}},
		{`INSERT INTO retrieval_candidates VALUES(?,?,?,?,?)`, []any{"legacy-run", 1, "legacy-record", 1.0, []byte(`{"final":1}`)}},
		{`INSERT INTO judgments VALUES(?,?,?,?,?,?,?,?,?,?,?)`, []any{originalID, "legacy-agent-original", "legacy-run", "legacy-lens", "legacy-record", "agent", "used", "", "", 0.25, created}},
		{`INSERT INTO judgments VALUES(?,?,?,?,?,?,?,?,?,?,?)`, []any{stableID("judgment", "legacy-owner-reversal"), "legacy-owner-reversal", "legacy-run", "legacy-lens", "legacy-record", "user", "reversed", "", originalID, -0.25, created}},
	} {
		if _, err := legacy.Exec(statement.query, statement.args...); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, privateio.FileMode); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	ctx := context.Background()
	if _, err := reopened.PutAgentActor(ctx, AgentActor{ID: "fresh-agent", Name: "Fresh"}); err != nil {
		t.Fatal(err)
	}
	assertScopedRecordRelevance(t, reopened, ctx, ScopedContext{
		ScopeID: OwnerRootScopeID, LensID: "legacy-lens", AgentID: "fresh-agent",
	}, "legacy-record", 0)
	var quarantined int
	if err := reopened.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scoped_judgments
		WHERE run_id='legacy-run' AND actor='agent' AND agent_id=?`, LegacyAgentActorID).Scan(&quarantined); err != nil || quarantined != 2 {
		t.Fatalf("quarantined judgments=%d err=%v", quarantined, err)
	}
}

func TestScopedFeedbackIsDirectionalAndPartitioned(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	store, err := Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	seedScopedContexts(t, store, ctx, now)

	owner, err := store.ApplyScopedJudgment(ctx, ScopedJudgmentRequest{
		RetryToken: "owner-retry-token-123456789", RunID: "run-scope-a-agent-a",
		ScopeID: "scope-a", LensID: "lens-one", RecordID: "record-one",
		Actor: FeedbackOwner, Disposition: "used",
	})
	if err != nil || owner.Effect != 1 || owner.AgentID != "" {
		t.Fatalf("owner judgment=%+v err=%v", owner, err)
	}
	agentARequest := ScopedJudgmentRequest{
		RetryToken: "agent-a-retry-token-123456", RunID: "run-scope-a-agent-a",
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
		RecordID: "record-one", Actor: FeedbackAgent, Disposition: "used",
	}
	agentA, err := store.ApplyScopedJudgment(ctx, agentARequest)
	if err != nil || agentA.Effect != 0.25 {
		t.Fatalf("agent A judgment=%+v err=%v", agentA, err)
	}
	replay, err := store.ApplyScopedJudgment(ctx, agentARequest)
	if err != nil || !replay.Replayed || replay.JudgmentID != agentA.JudgmentID {
		t.Fatalf("agent A replay=%+v err=%v", replay, err)
	}
	conflict := agentARequest
	conflict.Disposition = "dismissed"
	if _, err := store.ApplyScopedJudgment(ctx, conflict); err == nil {
		t.Fatal("conflicting retry-token reuse was accepted")
	}
	if _, err := store.ApplyScopedJudgment(ctx, ScopedJudgmentRequest{
		RetryToken: "agent-b-retry-token-123456", RunID: "run-scope-a-agent-b",
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-b",
		RecordID: "record-one", Actor: FeedbackAgent, Disposition: "dismissed",
	}); err != nil {
		t.Fatal(err)
	}

	assertScopedRelevance(t, store, ctx, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}, 0.125)
	assertScopedRelevance(t, store, ctx, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-b",
	}, 0.075)
	assertScopedRelevance(t, store, ctx, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-two", AgentID: "agent-a",
	}, 0)
	assertScopedRelevance(t, store, ctx, ScopedContext{
		ScopeID: "scope-b", LensID: "lens-one", AgentID: "agent-a",
	}, 0)

	if _, err := store.ArchiveAgentActor(ctx, "agent-a"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.ResolveScopedContext(ctx, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}); err == nil {
		t.Fatal("archived actor remained selectable")
	}
}

func TestScopedAgentReversalPersistsAndRejectsSecondOrCrossContextUse(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 8, 10, 30, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedScopedContexts(t, store, ctx, now)
	original, err := store.ApplyScopedJudgment(ctx, ScopedJudgmentRequest{
		RetryToken: "reversal-original-retry-123456", RunID: "run-scope-a-agent-a",
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
		RecordID: "record-one", Actor: FeedbackAgent, Disposition: "used",
	})
	if err != nil {
		t.Fatal(err)
	}
	reversalRequest := ScopedJudgmentRequest{
		IdempotencyKey: "reversal-event-key-123456", ScopeID: "scope-a",
		LensID: "lens-one", AgentID: "agent-a", Actor: FeedbackAgent,
		ReversesID: original.JudgmentID,
	}
	reversal, err := store.ApplyScopedJudgment(ctx, reversalRequest)
	if err != nil || reversal.Effect != -0.25 || reversal.ReversesID != original.JudgmentID {
		t.Fatalf("reversal=%+v err=%v", reversal, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replay, err := reopened.ApplyScopedJudgment(ctx, reversalRequest)
	if err != nil || !replay.Replayed || replay.JudgmentID != reversal.JudgmentID {
		t.Fatalf("reversal replay=%+v err=%v", replay, err)
	}
	second := reversalRequest
	second.IdempotencyKey = "second-reversal-event-key-123456"
	if _, err := reopened.ApplyScopedJudgment(ctx, second); err == nil {
		t.Fatal("second reversal was accepted")
	}
	crossContext := reversalRequest
	crossContext.IdempotencyKey = "cross-context-reversal-key-123456"
	crossContext.AgentID = "agent-b"
	if _, err := reopened.ApplyScopedJudgment(ctx, crossContext); err == nil {
		t.Fatal("cross-context reversal was accepted")
	}
	assertScopedRelevance(t, reopened, ctx, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}, 0)
}

func TestCorruptRecoveryRestoresScopedState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 8, 11, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedScopedContexts(t, store, ctx, now)
	request := ScopedJudgmentRequest{
		RetryToken: "recover-retry-token-123456", RunID: "run-scope-a-agent-a",
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
		RecordID: "record-one", Actor: FeedbackAgent, Disposition: "used",
	}
	original, err := store.ApplyScopedJudgment(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}

	recovered, quarantine, err := OpenRecovering(path, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if quarantine == "" {
		t.Fatal("expected corrupt database quarantine")
	}
	if _, _, _, err := recovered.ResolveScopedContext(ctx, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}); err != nil {
		t.Fatalf("resolve recovered context: %v", err)
	}
	assertScopedRelevance(t, recovered, ctx, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}, 0.025)
	replay, err := recovered.ApplyScopedJudgment(ctx, request)
	if err != nil || !replay.Replayed || replay.JudgmentID != original.JudgmentID {
		t.Fatalf("recovered replay=%+v err=%v", replay, err)
	}
}

func TestCorruptRecoveryRejectsInvalidScopedSidecar(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 8, 11, 30, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	seedScopedContexts(t, store, context.Background(), now)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot, present, err := readScopedRecoverySnapshot(path)
	if err != nil || !present || len(snapshot.Runs) == 0 {
		t.Fatalf("snapshot present=%t runs=%d err=%v", present, len(snapshot.Runs), err)
	}
	snapshot.Runs[0].AgentID = "missing-agent"
	if err := privateio.WriteJSON(scopedRecoveryPath(path), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenRecovering(path, func() time.Time { return now.Add(time.Second) }); err == nil {
		t.Fatal("invalid scoped recovery sidecar was accepted")
	}
	if matches, _ := filepath.Glob(path + ".corrupt-*"); len(matches) != 0 {
		t.Fatal("database was quarantined before invalid scoped recovery sidecar failed closed")
	}
}

func TestOversizedScopedRecoveryDoesNotReplaceReadableSnapshot(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 8, 11, 45, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	seedScopedContexts(t, store, context.Background(), now)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	sidecar := scopedRecoveryPath(path)
	before, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, present, err := readScopedRecoverySnapshot(path)
	if err != nil || !present {
		t.Fatalf("snapshot present=%v err=%v", present, err)
	}
	if err := writeScopedRecoverySnapshotFile(sidecar, snapshot, 1); err == nil {
		t.Fatal("oversized scoped recovery snapshot replaced the readable copy")
	}
	after, err := os.ReadFile(sidecar)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("oversized write changed readable snapshot: err=%v", err)
	}
}

func TestScopedRetrievalPreflightsRecoveryCapacityBeforeCommit(t *testing.T) {
	now := time.Date(2026, 8, 8, 11, 50, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.PutScope(ctx, Scope{ID: "capacity-scope", Name: "Capacity scope", Purpose: "capacity proof"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutScopedLens(ctx, ScopedLens{ScopeID: "capacity-scope", ID: "capacity-lens", Name: "Capacity lens", Query: "capacity"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutAgentActor(ctx, AgentActor{ID: "capacity-agent", Name: "Capacity agent"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(scopedRecoveryPath(path))
	if err != nil {
		t.Fatal(err)
	}
	store.scopedRecoveryByteLimit = int64(len(before) + 8)
	trace := ScopedRetrievalTrace{
		RunID: "capacity-run", Query: strings.Repeat("q", 512),
		ScopeID: "capacity-scope", LensID: "capacity-lens", AgentID: "capacity-agent",
		RetrievalMethod: "test", LibraryFingerprint: "fingerprint",
		CreatedAt: now.Format(time.RFC3339Nano),
		Candidates: []ScopedCandidateTrace{{
			RecordID: "capacity-record", Rank: 1, FinalScore: 1,
			ComponentScore: map[string]float64{"final": 1},
		}},
	}
	if err := store.SaveScopedRetrieval(ctx, trace); err == nil {
		t.Fatal("oversized scoped retrieval recovery projection was committed")
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scoped_retrieval_runs WHERE run_id=?`, trace.RunID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected retrieval reached database: count=%d err=%v", count, err)
	}
	after, err := os.ReadFile(scopedRecoveryPath(path))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected retrieval changed recovery snapshot: err=%v", err)
	}
	store.scopedRecoveryByteLimit = 0
	if err := store.SaveScopedRetrieval(ctx, trace); err != nil {
		t.Fatalf("exact retry collided after preflight rejection: %v", err)
	}
}

func TestScopedFeedbackRetryRepairsFailedRecoverySnapshot(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedScopedContexts(t, store, ctx, now)
	sidecar := scopedRecoveryPath(path)
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sidecar, privateio.DirMode); err != nil {
		t.Fatal(err)
	}
	request := ScopedJudgmentRequest{
		RetryToken: "repair-retry-token-123456", RunID: "run-scope-a-agent-a",
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
		RecordID: "record-one", Actor: FeedbackAgent, Disposition: "used",
	}
	if _, err := store.ApplyScopedJudgment(ctx, request); err == nil {
		t.Fatal("initial sidecar refresh failure was acknowledged")
	}
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	replay, err := store.ApplyScopedJudgment(ctx, request)
	if err != nil || !replay.Replayed {
		t.Fatalf("repair retry=%+v err=%v", replay, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	recovered, _, err := OpenRecovering(path, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	assertScopedRelevance(t, recovered, ctx, ScopedContext{
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
	}, 0.025)
}

func TestScopedFeedbackRejectsCredentialShapedReasonBeforePersistence(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	seedScopedContexts(t, store, ctx, now)
	request := ScopedJudgmentRequest{
		RetryToken: "secret-reason-retry-token-123456", RunID: "run-scope-a-agent-a",
		ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a",
		RecordID: "record-one", Actor: FeedbackAgent, Disposition: "used",
		Reason: "debug api_key=synthetic-private-credential",
	}
	if _, err := store.ApplyScopedJudgment(ctx, request); err == nil {
		t.Fatal("credential-shaped feedback reason was accepted")
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scoped_judgments`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected credential reason reached durable judgments: count=%d err=%v", count, err)
	}
	if snapshot, present, err := readScopedRecoverySnapshot(path); err != nil || !present || len(snapshot.Judgments) != 0 {
		t.Fatalf("rejected credential reason reached recovery snapshot: present=%v judgments=%d err=%v", present, len(snapshot.Judgments), err)
	}
}

func seedScopedContexts(t *testing.T, store *Store, ctx context.Context, now time.Time) {
	t.Helper()
	for _, scope := range []Scope{
		{ID: "scope-a", Name: "Scope A", Purpose: "architecture"},
		{ID: "scope-b", Name: "Scope B", Purpose: "team design"},
	} {
		if _, err := store.PutScope(ctx, scope); err != nil {
			t.Fatal(err)
		}
	}
	for _, lens := range []ScopedLens{
		{ScopeID: "scope-a", ID: "lens-one", Name: "Lens one", Query: "reliability"},
		{ScopeID: "scope-a", ID: "lens-two", Name: "Lens two", Query: "storytelling"},
		{ScopeID: "scope-b", ID: "lens-one", Name: "Lens one", Query: "reliability"},
	} {
		if _, err := store.PutScopedLens(ctx, lens); err != nil {
			t.Fatal(err)
		}
	}
	for _, actor := range []AgentActor{{ID: "agent-a", Name: "Agent A"}, {ID: "agent-b", Name: "Agent B"}} {
		if _, err := store.PutAgentActor(ctx, actor); err != nil {
			t.Fatal(err)
		}
	}
	for _, trace := range []ScopedRetrievalTrace{
		{RunID: "run-scope-a-agent-a", ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-a"},
		{RunID: "run-scope-a-agent-b", ScopeID: "scope-a", LensID: "lens-one", AgentID: "agent-b"},
		{RunID: "run-scope-a-lens-two", ScopeID: "scope-a", LensID: "lens-two", AgentID: "agent-a"},
		{RunID: "run-scope-b-agent-a", ScopeID: "scope-b", LensID: "lens-one", AgentID: "agent-a"},
	} {
		trace.Query = "same query"
		trace.RetrievalMethod = "test"
		trace.LibraryFingerprint = "fingerprint"
		trace.CreatedAt = now.Format(time.RFC3339Nano)
		trace.Candidates = []ScopedCandidateTrace{{
			RecordID: "record-one", Rank: 1, FinalScore: 1,
			ComponentScore: map[string]float64{"final": 1},
		}}
		if err := store.SaveScopedRetrieval(ctx, trace); err != nil {
			t.Fatal(err)
		}
	}
}

func assertScopedRelevance(
	t *testing.T,
	store *Store,
	ctx context.Context,
	context ScopedContext,
	want float64,
) {
	t.Helper()
	assertScopedRecordRelevance(t, store, ctx, context, "record-one", want)
}

func assertScopedRecordRelevance(
	t *testing.T,
	store *Store,
	ctx context.Context,
	context ScopedContext,
	recordID string,
	want float64,
) {
	t.Helper()
	got, err := store.ScopedRelevance(ctx, context, []string{recordID})
	if err != nil || math.Abs(got[recordID]-want) > 1e-12 {
		t.Fatalf("context=%+v relevance=%v want=%v err=%v", context, got, want, err)
	}
}
