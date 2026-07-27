package agentretrieval

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

type fakeEmbedder struct{}

func (fakeEmbedder) ModelID() string { return "fake/v1" }

func (fakeEmbedder) Embed(_ context.Context, inputs []string) ([][]float64, error) {
	result := make([][]float64, 0, len(inputs))
	for _, input := range inputs {
		if strings.Contains(strings.ToLower(input), "authority") ||
			strings.Contains(strings.ToLower(input), "product brain") {
			result = append(result, []float64{1, 0})
		} else {
			result = append(result, []float64{0, 1})
		}
	}
	return result, nil
}

type failingEmbedder struct{}

func (failingEmbedder) ModelID() string { return "offline/v1" }
func (failingEmbedder) Embed(context.Context, []string) ([][]float64, error) {
	return nil, errors.New("semantic provider offline")
}

func TestHybridBackendUsesSemanticLensAndReversibleFeedback(t *testing.T) {
	t.Parallel()
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	ctx := context.Background()
	if _, err := state.PutLens(ctx, agentstate.Lens{
		ID: "product-brain", Name: "Product Brain",
		Query: "product brain authority and evidence",
	}); err != nil {
		t.Fatal(err)
	}
	documents := []personalmemory.IndexDocument{
		{DocumentID: "relevant", Text: "authority graph provenance"},
		{DocumentID: "noise", Text: "team collaboration rooms"},
	}
	request := personalmemory.SearchRequest{
		Query: "How should it work?", LensID: "product-brain", Limit: 2,
	}
	hits, err := NewHybridBackend(ctx, state, fakeEmbedder{}).Rank(request, documents)
	if err != nil || len(hits) != 2 || hits[0].DocumentID != "relevant" {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
	if hits[0].Components["semantic_cosine"] <= hits[1].Components["semantic_cosine"] {
		t.Fatalf("semantic scores not reflected: %+v", hits)
	}
	if err := state.SaveRetrieval(ctx, agentstate.RetrievalTrace{
		RunID: "run", Query: request.Query, LensID: request.LensID,
		RetrievalMethod: "test", LibraryFingerprint: "fingerprint", CreatedAt: "now",
		Candidates: []agentstate.CandidateTrace{
			{RecordID: hits[0].DocumentID, Rank: 1, FinalScore: hits[0].Score, ComponentScore: hits[0].Components},
			{RecordID: hits[1].DocumentID, Rank: 2, FinalScore: hits[1].Score, ComponentScore: hits[1].Components},
		},
	}); err != nil {
		t.Fatal(err)
	}
	judgment, err := state.ApplyJudgment(ctx, agentstate.JudgmentRequest{
		IdempotencyKey: "dismiss", RunID: "run", LensID: request.LensID,
		RecordID: "relevant", Actor: "user", Disposition: "dismissed",
	})
	if err != nil {
		t.Fatal(err)
	}
	hits, err = NewHybridBackend(ctx, state, fakeEmbedder{}).Rank(request, documents)
	if err != nil || componentFor(hits, "relevant", "lens_feedback") != -0.1 ||
		hits[0].DocumentID != "noise" {
		t.Fatalf("feedback not applied: %+v err=%v", hits, err)
	}
	if _, err := state.ApplyJudgment(ctx, agentstate.JudgmentRequest{
		IdempotencyKey: "reverse", Actor: "user", ReversesID: judgment.JudgmentID,
	}); err != nil {
		t.Fatal(err)
	}
	hits, err = NewHybridBackend(ctx, state, fakeEmbedder{}).Rank(request, documents)
	if err != nil || componentFor(hits, "relevant", "lens_feedback") != 0 ||
		hits[0].DocumentID != "relevant" {
		t.Fatalf("feedback not reversed: %+v err=%v", hits, err)
	}
}

func TestHybridBackendReportsLexicalDegradation(t *testing.T) {
	t.Parallel()
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	backend := NewHybridBackend(context.Background(), state, failingEmbedder{})
	hits, err := backend.Rank(personalmemory.SearchRequest{
		Query: "provenance", Limit: 3,
	}, []personalmemory.IndexDocument{
		{DocumentID: "match", Text: "source provenance and citations"},
		{DocumentID: "noise", Text: "team rooms"},
	})
	if err != nil || len(hits) != 1 || hits[0].DocumentID != "match" {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
	stateName, reason := backend.RetrievalDiagnostics()
	if backend.MethodID() != "mindline_lexical_degraded/v0.1" ||
		stateName != "degraded" || reason != "semantic provider offline" {
		t.Fatalf("method=%s state=%s reason=%s", backend.MethodID(), stateName, reason)
	}
}

func componentFor(hits []personalmemory.RankedHit, documentID, component string) float64 {
	for _, hit := range hits {
		if hit.DocumentID == documentID {
			return hit.Components[component]
		}
	}
	return 999
}
