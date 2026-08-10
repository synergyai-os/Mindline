package agentstate

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestFeedbackRetryTokenReplaysIntentAndRejectsTokenReuse(t *testing.T) {
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	store, err := Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), func() time.Time {
		return now
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.PutLens(ctx, Lens{ID: "product", Name: "Product", Query: "product"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRetrieval(ctx, RetrievalTrace{
		RunID: "run-one", Query: "product", LensID: "product",
		RetrievalMethod: "test", LibraryFingerprint: "fingerprint",
		CreatedAt: now.Format(time.RFC3339),
		Candidates: []CandidateTrace{{
			RecordID: "record-one", Rank: 1, FinalScore: 1,
			ComponentScore: map[string]float64{"final": 1},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	request := JudgmentRequest{
		RetryToken: "one-intended-event", RunID: "run-one", LensID: "product",
		RecordID: "record-one", Actor: "agent", Disposition: "used",
		Reason: "used in answer",
	}
	first, err := store.ApplyJudgment(ctx, request)
	if err != nil || first.IdempotencyKey == "" || first.Replayed {
		t.Fatalf("first feedback=%+v err=%v", first, err)
	}
	replay, err := store.ApplyJudgment(ctx, request)
	if err != nil || !replay.Replayed || replay.JudgmentID != first.JudgmentID {
		t.Fatalf("feedback replay=%+v err=%v", replay, err)
	}
	changed := request
	changed.Disposition = "dismissed"
	if _, err := store.ApplyJudgment(ctx, changed); err == nil {
		t.Fatal("same retry token accepted a changed intent")
	}
	next := changed
	next.RetryToken = "a-new-intended-event"
	if _, err := store.ApplyJudgment(ctx, next); err != nil {
		t.Fatalf("new event token was rejected: %v", err)
	}
}

func TestFeedbackRetryIdentityIsDeterministicAndLegacyKeyRemainsAccepted(t *testing.T) {
	request := JudgmentRequest{
		RetryToken: "retry-token", RunID: "run", LensID: "lens", RecordID: "record",
		Actor: "agent", Disposition: "used", Reason: "reason",
	}
	first, err := FeedbackIdempotencyKey(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FeedbackIdempotencyKey(request)
	if err != nil || first != second {
		t.Fatalf("retry identity first=%q second=%q err=%v", first, second, err)
	}
	if _, err := FeedbackIdempotencyKey(JudgmentRequest{
		IdempotencyKey: "legacy", RetryToken: "retry-token", RunID: "run",
		LensID: "lens", RecordID: "record", Actor: "agent", Disposition: "used",
	}); err == nil {
		t.Fatal("ambiguous explicit and derived identity was accepted")
	}
}
