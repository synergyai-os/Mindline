package agentstate

import (
	"context"
	"database/sql"
	"math"
	"os"
	"path/filepath"
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
	if _, err := reopened.PutAgentActor(ctx, AgentActor{ID: "fresh-agent", Name: "Fresh agent"}); err != nil {
		t.Fatal(err)
	}
	var ownerRows, legacyAgentRows int
	if err := reopened.db.QueryRowContext(ctx, `SELECT
		SUM(CASE WHEN actor='owner' AND agent_id='' THEN 1 ELSE 0 END),
		SUM(CASE WHEN actor='agent' AND agent_id=? THEN 1 ELSE 0 END)
		FROM scoped_judgments WHERE scope_id=? AND lens_id=?`,
		LegacyAgentActorID, OwnerRootScopeID, "legacy-lens",
	).Scan(&ownerRows, &legacyAgentRows); err != nil || ownerRows != 1 || legacyAgentRows != 1 {
		t.Fatalf("legacy projection owner_rows=%d agent_rows=%d err=%v", ownerRows, legacyAgentRows, err)
	}
	assertScopedRecordRelevance(t, reopened, ctx, ScopedContext{
		ScopeID: OwnerRootScopeID, LensID: "legacy-lens", AgentID: "fresh-agent",
	}, "legacy-record", 0.1)
	legacyActor, err := reopened.GetAgentActor(ctx, LegacyAgentActorID)
	if err != nil || legacyActor.Status != StatusArchived {
		t.Fatalf("legacy actor=%+v err=%v", legacyActor, err)
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
