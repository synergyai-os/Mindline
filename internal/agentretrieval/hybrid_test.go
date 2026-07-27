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

type asymmetricFakeEmbedder struct {
	documentInputs []string
	queryInputs    []string
}

func (*asymmetricFakeEmbedder) ModelID() string { return "fake/asymmetric-v1" }
func (*asymmetricFakeEmbedder) Embed(context.Context, []string) ([][]float64, error) {
	return nil, errors.New("generic embedding path should not be used")
}
func (embedder *asymmetricFakeEmbedder) EmbedDocuments(
	_ context.Context,
	inputs []string,
) ([][]float64, error) {
	embedder.documentInputs = append(embedder.documentInputs, inputs...)
	vectors := make([][]float64, 0, len(inputs))
	for _, input := range inputs {
		if strings.Contains(input, "late-semantic-signal") {
			vectors = append(vectors, []float64{1, 0})
		} else {
			vectors = append(vectors, []float64{0, 1})
		}
	}
	return vectors, nil
}
func (embedder *asymmetricFakeEmbedder) EmbedQuery(
	_ context.Context,
	input string,
) ([]float64, error) {
	embedder.queryInputs = append(embedder.queryInputs, input)
	return []float64{1, 0}, nil
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
	if hits[0].Components["semantic_ranking_cosine"] <= hits[1].Components["semantic_ranking_cosine"] {
		t.Fatalf("lens-aware semantic ranking scores not reflected: %+v", hits)
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

func TestHybridBackendUsesAsymmetricChunkEmbeddingsForLateEvidence(t *testing.T) {
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	embedder := &asymmetricFakeEmbedder{}
	backend := NewHybridBackend(context.Background(), state, embedder)
	hits, err := backend.Rank(
		personalmemory.SearchRequest{Query: "conceptual answer", Limit: 2},
		[]personalmemory.IndexDocument{
			{
				DocumentID: "relevant",
				Text: strings.Repeat("earlier unrelated context ", 800) +
					" late-semantic-signal",
			},
			{DocumentID: "noise", Text: "short unrelated note"},
		},
	)
	if err != nil || len(hits) != 2 || hits[0].DocumentID != "relevant" {
		t.Fatalf("chunked semantic hits=%+v err=%v", hits, err)
	}
	if len(embedder.documentInputs) < 3 || len(embedder.queryInputs) != 1 ||
		!strings.Contains(embedder.queryInputs[0], "conceptual answer") {
		t.Fatalf("asymmetric inputs not used: docs=%d queries=%v",
			len(embedder.documentInputs), embedder.queryInputs)
	}
	if backend.MethodID() != "mindline_hybrid_local/v0.7" {
		t.Fatalf("semantic authorization policy change kept stale method identity: %s",
			backend.MethodID())
	}
}

func TestHybridBackendPreservesStrongLexicalEvidenceDuringFusion(t *testing.T) {
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	embedder := &asymmetricFakeEmbedder{}
	documents := []personalmemory.IndexDocument{{
		DocumentID: "relevant", Text: "late-semantic-signal",
	}}
	for index := 0; index < maximumSemanticRanks; index++ {
		documents = append(documents, personalmemory.IndexDocument{
			DocumentID: "noise-" + string(rune('Ā'+index)),
			Text:       "unrelated note",
		})
	}
	documents = append(documents, personalmemory.IndexDocument{
		DocumentID: "zzz-lexical-distractor",
		Text:       "conceptual answer",
	})
	hits, err := NewHybridBackend(context.Background(), state, embedder).Rank(
		personalmemory.SearchRequest{Query: "conceptual answer", Limit: 5},
		documents,
	)
	if err != nil || len(hits) == 0 || hits[0].DocumentID != "zzz-lexical-distractor" {
		t.Fatalf("strong lexical evidence was displaced by semantic-only evidence: hits=%+v err=%v",
			hits, err)
	}
}

func TestHybridBackendDoesNotBoostLexicalAuthorizationFeatures(t *testing.T) {
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	hits, err := NewHybridBackend(context.Background(), state, failingEmbedder{}).Rank(
		personalmemory.SearchRequest{Query: "portable quantum orchards", Limit: 2},
		[]personalmemory.IndexDocument{
			{DocumentID: "phrase", Text: "portable quantum orchards"},
			{DocumentID: "partial", Text: "portable quantum"},
		},
	)
	if err != nil || len(hits) != 2 {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
	if _, exists := hits[0].Components["lexical_evidence"]; exists ||
		hits[1].Score < 0.90 {
		t.Fatalf("lexical authorization changed fusion ranking: %+v", hits)
	}
}

type calibratedLensEmbedder struct{}

func (calibratedLensEmbedder) ModelID() string {
	return "ollama/embeddinggemma:latest/retrieval-input-v0.2"
}
func (calibratedLensEmbedder) Embed(context.Context, []string) ([][]float64, error) {
	return nil, errors.New("generic embedding path should not be used")
}
func (calibratedLensEmbedder) EmbedDocuments(
	_ context.Context, inputs []string,
) ([][]float64, error) {
	result := make([][]float64, 0, len(inputs))
	for _, input := range inputs {
		if strings.Contains(input, "authority") {
			result = append(result, []float64{1, 0})
		} else {
			result = append(result, []float64{0, 1})
		}
	}
	return result, nil
}
func (calibratedLensEmbedder) EmbedQuery(
	_ context.Context, input string,
) ([]float64, error) {
	if strings.Contains(input, "authority") {
		return []float64{1, 0}, nil
	}
	return []float64{0, 1}, nil
}

func TestHybridBackendLensReranksWithoutChangingAuthorizationScores(t *testing.T) {
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	ctx := context.Background()
	if _, err := state.PutLens(ctx, agentstate.Lens{
		ID: "authority", Name: "Authority", Query: "authority",
	}); err != nil {
		t.Fatal(err)
	}
	hits, err := NewHybridBackend(ctx, state, calibratedLensEmbedder{}).Rank(
		personalmemory.SearchRequest{
			Query: "unseen orchard concept", LensID: "authority", Limit: 3,
		},
		[]personalmemory.IndexDocument{
			{DocumentID: "lens", Text: "authority"},
			{DocumentID: "tie-a", Text: "unrelated"},
			{DocumentID: "tie-b", Text: "unrelated"},
		},
	)
	if err != nil || len(hits) != 3 || hits[0].DocumentID != "lens" {
		t.Fatalf("hits=%+v err=%v", hits, err)
	}
	if hits[0].Components["semantic_ranking_cosine"] != 1 ||
		hits[0].Components["semantic_cosine"] != 0 ||
		hits[0].Components["semantic_margin"] != 0 {
		t.Fatalf("lens leaked into authorization components: %+v", hits[0])
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
