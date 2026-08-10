package agentretrieval

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

type fakeEmbedder struct{}

type scopedOrderingEmbedder struct{}

type contextFailureEmbedder struct{}

type contextEvictionEmbedder struct{}

type projectionTestRepository struct {
	library personalmemory.Library
}

func (repository *projectionTestRepository) Load() (personalmemory.Library, error) {
	return repository.library, nil
}

func (*projectionTestRepository) Import(
	personalmemory.CaptureBatch,
) (personalmemory.ImportReceipt, error) {
	return personalmemory.ImportReceipt{}, nil
}

func (*projectionTestRepository) MergeEnrichment(
	personalmemory.EnrichmentBatch,
) (personalmemory.EnrichmentReceipt, error) {
	return personalmemory.EnrichmentReceipt{}, nil
}

func (*projectionTestRepository) LoadContent(
	personalmemory.ContentArtifactRef,
) (personalmemory.ExtractedContentArtifact, error) {
	return personalmemory.ExtractedContentArtifact{}, nil
}

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

func (scopedOrderingEmbedder) ModelID() string { return "fake/scoped-ordering-v1" }

func (scopedOrderingEmbedder) Embed(_ context.Context, inputs []string) ([][]float64, error) {
	result := make([][]float64, 0, len(inputs))
	for _, input := range inputs {
		result = append(result, scopedOrderingVector(input))
	}
	return result, nil
}

func (scopedOrderingEmbedder) EmbedDocuments(
	_ context.Context,
	inputs []string,
) ([][]float64, error) {
	result := make([][]float64, 0, len(inputs))
	for _, input := range inputs {
		result = append(result, scopedOrderingVector(input))
	}
	return result, nil
}

func (scopedOrderingEmbedder) EmbedQuery(
	_ context.Context,
	input string,
) ([]float64, error) {
	input = strings.ToLower(input)
	if strings.Contains(input, "prefer-beta") {
		return []float64{0, 1, 1, 1}, nil
	}
	if strings.Contains(input, "prefer-alpha") {
		return []float64{1, 0, 0, 0}, nil
	}
	return []float64{1, 1, 0, 0}, nil
}

func scopedOrderingVector(input string) []float64 {
	switch {
	case strings.Contains(input, "alpha"):
		return []float64{1, 0, 0, 0}
	case strings.Contains(input, "beta"):
		return []float64{0, 1, 0, 0}
	case strings.Contains(input, "gamma"):
		return []float64{0, 0, 1, 0}
	default:
		return []float64{0, 0, 0, 1}
	}
}

func (contextFailureEmbedder) ModelID() string { return "fake/context-failure-v1" }
func (contextFailureEmbedder) Embed(context.Context, []string) ([][]float64, error) {
	return nil, errors.New("generic embedding path should not be used")
}
func (contextFailureEmbedder) EmbedDocuments(_ context.Context, inputs []string) ([][]float64, error) {
	result := make([][]float64, 0, len(inputs))
	for _, input := range inputs {
		switch {
		case strings.Contains(input, "doc-b"):
			result = append(result, []float64{1, 0})
		case strings.Contains(input, "doc-a"):
			result = append(result, []float64{-1, 0})
		default:
			result = append(result, []float64{0, 1})
		}
	}
	return result, nil
}
func (contextFailureEmbedder) EmbedQuery(_ context.Context, input string) ([]float64, error) {
	if strings.Contains(input, "context-fails") {
		return nil, errors.New("context embedding failed")
	}
	return []float64{1, 0}, nil
}

func (contextEvictionEmbedder) ModelID() string {
	return "ollama/embeddinggemma:latest/retrieval-input-v0.2"
}
func (contextEvictionEmbedder) Embed(context.Context, []string) ([][]float64, error) {
	return nil, errors.New("generic embedding path should not be used")
}
func (contextEvictionEmbedder) EmbedDocuments(_ context.Context, inputs []string) ([][]float64, error) {
	result := make([][]float64, 0, len(inputs))
	for _, input := range inputs {
		if strings.Contains(input, "target-document") {
			result = append(result, []float64{1, 0})
		} else {
			result = append(result, []float64{0, 1})
		}
	}
	return result, nil
}
func (contextEvictionEmbedder) EmbedQuery(_ context.Context, input string) ([]float64, error) {
	if strings.Contains(input, "push-target-last") {
		return []float64{-1, 0}, nil
	}
	return []float64{1, 0}, nil
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

func TestScopedRankFreezesCallerBoundBeforeContextRerank(t *testing.T) {
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	ctx := context.Background()
	if _, err := state.PutScope(ctx, agentstate.Scope{
		ID: "project", Name: "Project", Purpose: "shared purpose",
	}); err != nil {
		t.Fatal(err)
	}
	for _, lens := range []agentstate.ScopedLens{
		{ScopeID: "project", ID: "alpha", Name: "Alpha", Query: "prefer-alpha"},
		{ScopeID: "project", ID: "beta", Name: "Beta", Query: "prefer-beta"},
	} {
		if _, err := state.PutScopedLens(ctx, lens); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := state.PutAgentActor(ctx, agentstate.AgentActor{
		ID: "agent-a", Name: "Agent A",
	}); err != nil {
		t.Fatal(err)
	}
	documents := []personalmemory.IndexDocument{
		{DocumentID: "a", Text: "shared evidence alpha"},
		{DocumentID: "b", Text: "shared evidence beta"},
		{DocumentID: "c", Text: "shared evidence gamma"},
		{DocumentID: "d", Text: "shared evidence delta"},
	}
	backend := NewHybridBackend(ctx, state, scopedOrderingEmbedder{})
	rank := func(lensID, lensQuery string) []personalmemory.RankedHit {
		hits, err := backend.Rank(personalmemory.SearchRequest{
			Query: "shared evidence", LexicalQuery: "shared evidence",
			Limit: 100, QueryAuthorizedLimit: 2,
			ScopeID: "project", ScopePurpose: "shared purpose",
			LensID: lensID, LensQuery: lensQuery, AgentID: "agent-a",
		}, documents)
		if err != nil {
			t.Fatal(err)
		}
		return hits
	}
	alpha := rank("alpha", "prefer-alpha")
	beta := rank("beta", "prefer-beta")
	if len(alpha) != 2 || len(beta) != 2 ||
		alpha[0].DocumentID != "a" || alpha[1].DocumentID != "b" ||
		beta[0].DocumentID != "b" || beta[1].DocumentID != "a" {
		t.Fatalf("context changed authorized membership or did not rerank: alpha=%+v beta=%+v", alpha, beta)
	}

	for _, scope := range []agentstate.Scope{
		{ID: "scope-alpha", Name: "Scope alpha", Purpose: "prefer-alpha"},
		{ID: "scope-beta", Name: "Scope beta", Purpose: "prefer-beta"},
	} {
		if _, err := state.PutScope(ctx, scope); err != nil {
			t.Fatal(err)
		}
		if _, err := state.PutScopedLens(ctx, agentstate.ScopedLens{
			ScopeID: scope.ID, ID: "neutral", Name: "Neutral", Query: "shared purpose",
		}); err != nil {
			t.Fatal(err)
		}
	}
	rankScope := func(scopeID, purpose string) []personalmemory.RankedHit {
		hits, err := backend.Rank(personalmemory.SearchRequest{
			Query: "shared evidence", LexicalQuery: "shared evidence",
			Limit: 100, QueryAuthorizedLimit: 2,
			ScopeID: scopeID, ScopePurpose: purpose,
			LensID: "neutral", LensQuery: "shared purpose", AgentID: "agent-a",
		}, documents)
		if err != nil {
			t.Fatal(err)
		}
		return hits
	}
	scopeAlpha := rankScope("scope-alpha", "prefer-alpha")
	scopeBeta := rankScope("scope-beta", "prefer-beta")
	if len(scopeAlpha) != 2 || len(scopeBeta) != 2 ||
		scopeAlpha[0].DocumentID != "a" || scopeAlpha[1].DocumentID != "b" ||
		scopeBeta[0].DocumentID != "b" || scopeBeta[1].DocumentID != "a" {
		t.Fatalf("scope changed authorized membership or did not rerank: alpha=%+v beta=%+v", scopeAlpha, scopeBeta)
	}
}

func TestScopedCombinedEmbeddingFailureFallsBackToQueryLexicalMembership(t *testing.T) {
	state, request := seededScopedRequest(t, "context-fails")
	defer state.Close()
	documents := []personalmemory.IndexDocument{
		{DocumentID: "a", Text: "term term term doc-a"},
		{DocumentID: "b", Text: "term doc-b"},
	}
	for index := 0; index < 100; index++ {
		documents = append(documents, personalmemory.IndexDocument{
			DocumentID: "filler-" + strconv.Itoa(index), Text: "filler evidence",
		})
	}
	request.Query = "term"
	request.LexicalQuery = "term"
	request.QueryAuthorizedLimit = 1
	backend := NewHybridBackend(context.Background(), state, contextFailureEmbedder{})
	hits, err := backend.Rank(request, documents)
	if err != nil || len(hits) != 1 || hits[0].DocumentID != "a" ||
		backend.MethodID() != "mindline_lexical_degraded/v0.2" {
		t.Fatalf("combined embedding failure did not fail closed to lexical membership: hits=%+v err=%v", hits, err)
	}
}

func TestScopedContextRankCapCannotDropAuthorizedSemanticOnlyItem(t *testing.T) {
	state, request := seededScopedRequest(t, "push-target-last")
	defer state.Close()
	records := []personalmemory.CaptureRecord{{
		RecordID: "target", SourceRef: "slack://proof/target", RawText: "target-document",
		ContentHash: strings.Repeat("a", 64),
	}}
	for index := 0; index < 101; index++ {
		records = append(records, personalmemory.CaptureRecord{
			RecordID:  "noise-" + strconv.Itoa(index),
			SourceRef: "slack://proof/noise-" + strconv.Itoa(index),
			RawText:   "contextual noise", ContentHash: strings.Repeat("b", 64),
		})
	}
	request.Query = "semantic-only-query"
	request.LexicalQuery = request.Query
	request.Limit = 1
	request.QueryAuthorizedLimit = 1
	request.RunID = "context-rank-cap-run"
	backend := NewHybridBackend(context.Background(), state, contextEvictionEmbedder{})
	repository := &projectionTestRepository{library: personalmemory.Library{
		SchemaVersion: personalmemory.LibrarySchemaVersion, Revision: 1,
		Fingerprint: strings.Repeat("f", 64), Records: records,
		Resources: []personalmemory.ResourceContext{},
	}}
	packet, err := personalmemory.NewRetriever(
		repository, backend,
	).SearchCompact(request)
	if err != nil || len(packet.Citations) != 1 || packet.Citations[0].RecordID != "target" {
		t.Fatalf("compact output dropped authorized item: packet=%+v err=%v", packet, err)
	}
	beforeScore := packet.Citations[0].Score
	if err := state.SaveScopedRetrieval(context.Background(), agentstate.ScopedRetrievalTrace{
		RunID: request.RunID, Query: request.Query, ScopeID: request.ScopeID,
		LensID: request.LensID, AgentID: request.AgentID,
		RetrievalMethod: packet.RetrievalMethod, LibraryFingerprint: packet.LibraryFingerprint,
		CreatedAt: "2026-08-08T12:00:00Z", Candidates: []agentstate.ScopedCandidateTrace{{
			RecordID: "target", Rank: 1, FinalScore: beforeScore,
			ComponentScore: packet.Citations[0].ComponentScores,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ApplyScopedJudgment(context.Background(), agentstate.ScopedJudgmentRequest{
		RetryToken: "context-rank-dismiss-123456", RunID: request.RunID,
		ScopeID: request.ScopeID, LensID: request.LensID, AgentID: request.AgentID,
		RecordID: "target", Actor: agentstate.FeedbackAgent, Disposition: "dismissed",
	}); err != nil {
		t.Fatal(err)
	}
	after, err := personalmemory.NewRetriever(repository, backend).SearchCompact(request)
	if err != nil || len(after.Citations) != 1 || after.Citations[0].RecordID != "target" ||
		after.Citations[0].Score >= beforeScore {
		t.Fatalf("dismissal did not lower retained citation: before=%+v after=%+v err=%v",
			packet.Citations, after.Citations, err)
	}
}

func seededScopedRequest(t *testing.T, lensQuery string) (*agentstate.Store, personalmemory.SearchRequest) {
	t.Helper()
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := state.PutScope(ctx, agentstate.Scope{
		ID: "project", Name: "Project", Purpose: "shared-purpose",
	}); err != nil {
		state.Close()
		t.Fatal(err)
	}
	if _, err := state.PutScopedLens(ctx, agentstate.ScopedLens{
		ScopeID: "project", ID: "lens", Name: "Lens", Query: lensQuery,
	}); err != nil {
		state.Close()
		t.Fatal(err)
	}
	if _, err := state.PutAgentActor(ctx, agentstate.AgentActor{ID: "agent", Name: "Agent"}); err != nil {
		state.Close()
		t.Fatal(err)
	}
	return state, personalmemory.SearchRequest{
		Limit: 100, ScopeID: "project", ScopePurpose: "shared-purpose",
		LensID: "lens", LensQuery: lensQuery, AgentID: "agent",
	}
}

func TestCompactResourceProjectionUsesCanonicalSharedOwnerFeedback(t *testing.T) {
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	ctx := context.Background()
	lens, err := state.PutLens(ctx, agentstate.Lens{
		ID: "shared-resource", Name: "Shared resource",
		Query: "portable quantum orchards",
	})
	if err != nil {
		t.Fatal(err)
	}
	sharedResourceID := "resource-a-shared"
	otherResourceID := "resource-z-other"
	repository := &projectionTestRepository{library: personalmemory.Library{
		SchemaVersion: personalmemory.LibrarySchemaVersion,
		Revision:      8,
		Fingerprint:   strings.Repeat("8", 64),
		Records: []personalmemory.CaptureRecord{
			{
				RecordID: "record-a", SourceRef: "slack://fixture/a",
				RawText:     "first unrelated save",
				ResourceIDs: []string{sharedResourceID},
				ContentHash: strings.Repeat("a", 64),
			},
			{
				RecordID: "record-a2", SourceRef: "slack://fixture/a2",
				RawText:     "second unrelated save",
				ResourceIDs: []string{sharedResourceID},
				ContentHash: strings.Repeat("b", 64),
			},
			{
				RecordID: "record-z", SourceRef: "slack://fixture/z",
				RawText:     "third unrelated save",
				ResourceIDs: []string{otherResourceID},
				ContentHash: strings.Repeat("c", 64),
			},
		},
		Resources: []personalmemory.ResourceContext{
			{
				ResourceID: sharedResourceID,
				Metadata: personalmemory.ResourceMetadata{
					Title: "portable quantum orchards",
				},
				ContentHash: strings.Repeat("d", 64),
			},
			{
				ResourceID: otherResourceID,
				Metadata: personalmemory.ResourceMetadata{
					Title: "portable quantum orchards",
				},
				ContentHash: strings.Repeat("e", 64),
			},
		},
	}}
	backend := NewHybridBackend(ctx, state, failingEmbedder{})
	retriever := personalmemory.NewRetriever(repository, backend)
	request := personalmemory.SearchRequest{
		Query:  "portable quantum orchards",
		LensID: lens.ID,
		Limit:  3,
	}
	before, err := retriever.SearchCompact(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Citations) != 3 ||
		before.Citations[0].RecordID != "record-a" ||
		before.Citations[1].RecordID != "record-a2" {
		t.Fatalf("shared resource did not expand to deterministic canonical owners: %+v",
			before)
	}
	candidates := make([]agentstate.CandidateTrace, 0, len(before.Citations))
	for rank, citation := range before.Citations {
		candidates = append(candidates, agentstate.CandidateTrace{
			RecordID:       citation.RecordID,
			Rank:           rank + 1,
			FinalScore:     citation.Score,
			ComponentScore: citation.ComponentScores,
		})
	}
	if err := state.SaveRetrieval(ctx, agentstate.RetrievalTrace{
		RunID: "shared-resource-feedback",
		Query: request.Query, LensID: request.LensID,
		RetrievalMethod:    before.RetrievalMethod,
		LibraryFingerprint: before.LibraryFingerprint,
		CreatedAt:          "2026-07-27T10:00:00Z",
		Candidates:         candidates,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ApplyJudgment(ctx, agentstate.JudgmentRequest{
		IdempotencyKey: "dismiss-shared-owner",
		RunID:          "shared-resource-feedback", LensID: request.LensID,
		RecordID: "record-a2", Actor: "user", Disposition: "dismissed",
	}); err != nil {
		t.Fatal(err)
	}
	after, err := retriever.SearchCompact(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Citations) != 3 ||
		after.Citations[0].RecordID != "record-z" ||
		after.Citations[1].ComponentScores["lens_feedback"] != -0.1 ||
		after.Citations[2].ComponentScores["lens_feedback"] != -0.1 {
		t.Fatalf("canonical shared-owner feedback did not rerank the resource: %+v",
			after)
	}
	data, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "compact-resource:") {
		t.Fatalf("retrieval-only resource identity leaked into packet: %s", data)
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
	if backend.MethodID() != "mindline_lexical_degraded/v0.2" ||
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
	if backend.MethodID() != "mindline_hybrid_local/v0.17" {
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

type corroboratedRecoveryEmbedder struct{}

func (corroboratedRecoveryEmbedder) ModelID() string {
	return "ollama/embeddinggemma:latest/retrieval-input-v0.2"
}
func (corroboratedRecoveryEmbedder) Embed(context.Context, []string) ([][]float64, error) {
	return nil, errors.New("generic embedding path should not be used")
}
func (corroboratedRecoveryEmbedder) EmbedDocuments(
	_ context.Context,
	inputs []string,
) ([][]float64, error) {
	vectors := make([][]float64, 0, len(inputs))
	for _, input := range inputs {
		cosine := 0.55
		switch {
		case strings.Contains(input, "winner-resource"):
			cosine = 0.62
		case strings.Contains(input, "sibling-source"):
			cosine = 0.61
		}
		vectors = append(vectors, []float64{
			cosine, math.Sqrt(1 - cosine*cosine),
		})
	}
	return vectors, nil
}
func (corroboratedRecoveryEmbedder) EmbedQuery(
	context.Context,
	string,
) ([]float64, error) {
	return []float64{1, 0}, nil
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

func TestHybridCompactSearchWiresCorroboratedResourceRecoveryEndToEnd(t *testing.T) {
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	resourceID := "resource-recovery-wiring"
	repository := &projectionTestRepository{library: personalmemory.Library{
		SchemaVersion: personalmemory.LibrarySchemaVersion,
		Revision:      14,
		Fingerprint:   strings.Repeat("4", 64),
		Records: []personalmemory.CaptureRecord{
			{
				RecordID: "record-owner", SourceRef: "slack://fixture/owner",
				RawText:     "sibling-source",
				ResourceIDs: []string{resourceID},
				ContentHash: strings.Repeat("a", 64),
			},
			{
				RecordID: "record-distinct", SourceRef: "slack://fixture/distinct",
				RawText:     "different-evidence",
				ContentHash: strings.Repeat("b", 64),
			},
		},
		Resources: []personalmemory.ResourceContext{{
			ResourceID: resourceID,
			Metadata: personalmemory.ResourceMetadata{
				Title: "winner-resource quasar context memory",
			},
			ContentHash: strings.Repeat("c", 64),
		}},
	}}
	backend := NewHybridBackend(
		context.Background(), state, corroboratedRecoveryEmbedder{},
	)
	packet, err := personalmemory.NewRetriever(
		repository, backend,
	).SearchCompact(personalmemory.SearchRequest{
		Query: "quasar retrieval memory",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if packet.AnswerState != "answered" ||
		len(packet.Citations) != 1 ||
		packet.Citations[0].RecordID != "record-owner" {
		t.Fatalf("hybrid recovery wiring did not return the canonical owner: %+v",
			packet)
	}
	components := packet.Citations[0].ComponentScores
	if components["semantic_margin"] >= personalmemory.DefaultCompactMinimumSemanticMargin ||
		components["semantic_distinct_evidence_margin"] < personalmemory.DefaultCompactMinimumSemanticMargin ||
		components["lexical_idf_coverage"] < personalmemory.DefaultCompactMinimumSemanticLexicalCover ||
		components["semantic_distinct_evidence_valid"] != 1 ||
		backend.MethodID() != "mindline_hybrid_local/v0.17" {
		t.Fatalf("hybrid recovery evidence was not wired conservatively: %+v method=%s",
			components, backend.MethodID())
	}
}

func TestFeedbackRelevanceChunksMoreThanOneHundredThousandCanonicalIDs(t *testing.T) {
	state, err := agentstate.Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	ctx := context.Background()
	lens, err := state.PutLens(ctx, agentstate.Lens{
		ID: "large-projection", Name: "Large projection", Query: "bounded relevance",
	})
	if err != nil {
		t.Fatal(err)
	}
	const boundaryID = "zzzz-boundary-record"
	if err := state.SaveRetrieval(ctx, agentstate.RetrievalTrace{
		RunID: "large-projection-run",
		Query: "bounded relevance", LensID: lens.ID,
		RetrievalMethod: "fixture", LibraryFingerprint: "fixture",
		CreatedAt: "2026-07-27T10:00:00Z",
		Candidates: []agentstate.CandidateTrace{{
			RecordID: boundaryID, Rank: 1, FinalScore: 1,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ApplyJudgment(ctx, agentstate.JudgmentRequest{
		IdempotencyKey: "large-projection-feedback",
		RunID:          "large-projection-run", LensID: lens.ID,
		RecordID: boundaryID, Actor: "user", Disposition: "used",
	}); err != nil {
		t.Fatal(err)
	}
	documents := make([]personalmemory.IndexDocument, 0, maximumRelevanceIDs+1)
	for index := 0; index < maximumRelevanceIDs; index++ {
		documents = append(documents, personalmemory.IndexDocument{
			DocumentID: "record-" + strconv.Itoa(index),
		})
	}
	documents = append(documents, personalmemory.IndexDocument{
		DocumentID: boundaryID,
	})
	relevance, err := NewHybridBackend(
		ctx, state, failingEmbedder{},
	).feedbackRelevance(lens.ID, documents)
	if err != nil {
		t.Fatal(err)
	}
	if relevance[boundaryID] != 0.1 {
		t.Fatalf("bounded relevance lookup lost the second-chunk judgment: %v",
			relevance[boundaryID])
	}
}

func TestDistinctEvidenceSemanticAuthorizationIgnoresOnlyExactResourceSibling(t *testing.T) {
	resourceWinner := personalmemory.IndexDocument{
		DocumentID:                   "resource-winner",
		FeedbackAliases:              []string{"record-owner"},
		AuthorizationEvidenceAliases: []string{"resource-a"},
		AuthorizationEvidenceKind:    personalmemory.IndexEvidenceKindUniqueResource,
	}
	sameResourceSibling := personalmemory.IndexDocument{
		DocumentID:                   "record-source-sibling",
		FeedbackAliases:              []string{"record-owner"},
		AuthorizationEvidenceAliases: []string{"resource-a"},
		AuthorizationEvidenceKind:    personalmemory.IndexEvidenceKindRecordSource,
	}
	distinctRecord := personalmemory.IndexDocument{
		DocumentID:                   "record-distinct",
		FeedbackAliases:              []string{"record-distinct"},
		AuthorizationEvidenceAliases: []string{"record-distinct"},
		AuthorizationEvidenceKind:    personalmemory.IndexEvidenceKindRecordSource,
	}
	baselineDocuments := []personalmemory.IndexDocument{
		resourceWinner, sameResourceSibling, distinctRecord,
	}
	baselineScores := map[string]float64{
		"resource-winner":       0.62,
		"record-source-sibling": 0.61,
		"record-distinct":       0.55,
	}
	baseline, ok := distinctEvidenceSemanticAuthorization(
		baselineScores, baselineDocuments,
	)
	if !ok || baseline.winnerID != "resource-winner" ||
		baseline.runner != 0.55 ||
		math.Abs(baseline.margin-0.07) > 0.000001 {
		t.Fatalf("same-resource source sibling was not ignored: %+v ok=%v",
			baseline, ok)
	}
	replayed, replayOK := distinctEvidenceSemanticAuthorization(
		baselineScores,
		[]personalmemory.IndexDocument{
			distinctRecord, sameResourceSibling, resourceWinner,
		},
	)
	if !replayOK || replayed != baseline {
		t.Fatalf("distinct-evidence computation was not replay deterministic: first=%+v replay=%+v",
			baseline, replayed)
	}

	for _, test := range []struct {
		name       string
		competitor personalmemory.IndexDocument
	}{
		{
			name: "distinct resource with same owner",
			competitor: personalmemory.IndexDocument{
				DocumentID:                   "resource-same-owner",
				FeedbackAliases:              []string{"record-owner"},
				AuthorizationEvidenceAliases: []string{"resource-b"},
				AuthorizationEvidenceKind:    personalmemory.IndexEvidenceKindUniqueResource,
			},
		},
		{
			name: "source overlaps winner and distinct resource",
			competitor: personalmemory.IndexDocument{
				DocumentID:                   "record-overlap",
				FeedbackAliases:              []string{"record-owner"},
				AuthorizationEvidenceAliases: []string{"resource-a", "resource-b"},
				AuthorizationEvidenceKind:    personalmemory.IndexEvidenceKindRecordSource,
			},
		},
		{
			name: "different owner resource",
			competitor: personalmemory.IndexDocument{
				DocumentID:                   "resource-different-owner",
				FeedbackAliases:              []string{"record-other"},
				AuthorizationEvidenceAliases: []string{"resource-c"},
				AuthorizationEvidenceKind:    personalmemory.IndexEvidenceKindUniqueResource,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			scores := map[string]float64{
				resourceWinner.DocumentID:      0.62,
				sameResourceSibling.DocumentID: 0.61,
				test.competitor.DocumentID:     0.61,
			}
			result, ok := distinctEvidenceSemanticAuthorization(
				scores,
				[]personalmemory.IndexDocument{
					resourceWinner, sameResourceSibling, test.competitor,
				},
			)
			if !ok || result.runner != 0.61 ||
				math.Abs(result.margin-0.01) > 0.000001 {
				t.Fatalf("distinct evidence did not remain a competitor: %+v ok=%v",
					result, ok)
			}
		})
	}
}

func TestDistinctEvidenceSemanticAuthorizationFailsClosedOnInvalidEvidence(t *testing.T) {
	validWinner := personalmemory.IndexDocument{
		DocumentID:                   "resource-winner",
		AuthorizationEvidenceAliases: []string{"resource-a"},
		AuthorizationEvidenceKind:    personalmemory.IndexEvidenceKindUniqueResource,
	}
	for _, test := range []struct {
		name      string
		documents []personalmemory.IndexDocument
		scores    map[string]float64
	}{
		{
			name: "missing alias",
			documents: []personalmemory.IndexDocument{{
				DocumentID:                "resource-winner",
				AuthorizationEvidenceKind: personalmemory.IndexEvidenceKindUniqueResource,
			}},
			scores: map[string]float64{"resource-winner": 0.62},
		},
		{
			name: "ambiguous duplicate alias",
			documents: []personalmemory.IndexDocument{{
				DocumentID:                   "resource-winner",
				AuthorizationEvidenceAliases: []string{"resource-a", "resource-a"},
				AuthorizationEvidenceKind:    personalmemory.IndexEvidenceKindUniqueResource,
			}},
			scores: map[string]float64{"resource-winner": 0.62},
		},
		{
			name: "duplicate unique resource identity",
			documents: []personalmemory.IndexDocument{
				validWinner,
				{
					DocumentID:                   "resource-duplicate",
					AuthorizationEvidenceAliases: []string{"resource-a"},
					AuthorizationEvidenceKind:    personalmemory.IndexEvidenceKindUniqueResource,
				},
			},
			scores: map[string]float64{
				"resource-winner": 0.62, "resource-duplicate": 0.61,
			},
		},
		{
			name:      "non-finite score",
			documents: []personalmemory.IndexDocument{validWinner},
			scores: map[string]float64{
				"resource-winner": math.NaN(),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if result, ok := distinctEvidenceSemanticAuthorization(
				test.scores, test.documents,
			); ok {
				t.Fatalf("invalid evidence authorized: %+v", result)
			}
		})
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
