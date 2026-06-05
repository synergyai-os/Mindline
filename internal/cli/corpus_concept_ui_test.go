package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestCorpusConceptUIServesReviewStateAndRecordsDecision(t *testing.T) {
	root := t.TempDir()
	concept := documents.CorpusConcept{
		SchemaVersion:          documents.CorpusConceptsSchemaVersion,
		ConceptID:              "concept-ui-test",
		CorpusID:               "corpus-ui-test",
		Title:                  "Cross-source topic concept: review, evidence",
		ReviewPrompt:           "Decide whether these snippets describe one coherent concept.",
		GroupingRationale:      "Grouped from cross-source relations across 2 sources.",
		Section:                documents.CorpusConceptSectionCrossSource,
		CandidateKind:          documents.SemanticCandidateKindTopic,
		RoutingHint:            documents.SourceMeaningRoutingTolariaCandidate,
		ReviewStatus:           documents.ReviewStatusNeedsReview,
		AtomCount:              2,
		SourceCount:            2,
		EvidenceReferenceCount: 2,
		SourceKindCoverage:     map[string]int{"gmail": 1, "slack": 1},
		RepresentativeEvidence: []documents.CorpusConceptEvidencePreview{{
			EvidenceRefID: "evref-ui",
			AtomID:        "atom-ui",
			SourceID:      "gmail-source-ui",
			SourceKind:    "gmail",
			SourceRef:     "gmail:source-ui",
			LineStart:     1,
			LineEnd:       2,
			ContentHash:   "hash-ui",
			Title:         "Readable evidence title",
			Summary:       "Readable evidence summary",
			Excerpt:       "Readable private-local excerpt for review.",
		}},
	}
	summary := documents.CorpusConceptSummary{
		SchemaVersion:           documents.CorpusConceptsSchemaVersion,
		CorpusID:                "corpus-ui-test",
		SourceCount:             2,
		ProcessedSourceCount:    2,
		ConceptCount:            1,
		CrossSourceConceptCount: 1,
		Concepts: []documents.CorpusConceptListItem{{
			ConceptID:              concept.ConceptID,
			Title:                  concept.Title,
			ReviewPrompt:           concept.ReviewPrompt,
			GroupingRationale:      concept.GroupingRationale,
			Section:                concept.Section,
			CandidateKind:          concept.CandidateKind,
			RoutingHint:            concept.RoutingHint,
			AtomCount:              concept.AtomCount,
			SourceCount:            concept.SourceCount,
			EvidenceReferenceCount: concept.EvidenceReferenceCount,
			SourceKindCoverage:     concept.SourceKindCoverage,
			ReviewStatus:           concept.ReviewStatus,
			RepresentativeEvidence: len(concept.RepresentativeEvidence),
			ConceptPath:            filepath.ToSlash(filepath.Join(documents.CorpusConceptsDirName, documents.CorpusConceptPath(concept.ConceptID))),
		}},
	}
	index := documents.CorpusConceptIndex{
		SchemaVersion: documents.CorpusConceptsSchemaVersion,
		CorpusID:      "corpus-ui-test",
		Concepts:      []documents.CorpusConcept{concept},
	}
	if err := documents.WriteCorpusConceptIndex(root, summary, index); err != nil {
		t.Fatalf("write concept index: %v", err)
	}

	handler := newCorpusConceptUIHandlerWithToken(filepath.Join(root, documents.CorpusConceptsDirName), "test-token", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = judgmentUITestHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected html status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `name="mindline-review-token" content="test-token"`) {
		t.Fatalf("expected review token meta tag")
	}
	if !strings.Contains(rec.Body.String(), "Copy concept") || !strings.Contains(rec.Body.String(), "Mindline concept review packet") {
		t.Fatalf("expected concept copy affordance in UI")
	}

	state := getCorpusConceptUIState(t, handler)
	if state.Progress.TotalConceptCount != 1 || state.Progress.ReviewedConceptCount != 0 {
		t.Fatalf("unexpected initial progress: %+v", state.Progress)
	}
	if got := state.Index.Concepts[0].RepresentativeEvidence[0].Excerpt; got == "" {
		t.Fatalf("expected representative evidence excerpt")
	}

	payload := `{"concept_id":"concept-ui-test","choice":"rename_needed","note":"needs clearer title"}`
	req = httptest.NewRequest(http.MethodPost, "/api/reviews", strings.NewReader(payload))
	req.Host = judgmentUITestHost
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected tokenless review status 403, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/reviews", strings.NewReader(payload))
	req.Host = judgmentUITestHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", "test-token")
	req.Header.Set("Origin", "http://"+judgmentUITestHost)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected review status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	updated := decodeCorpusConceptUIState(t, rec.Body)
	if updated.Progress.ReviewedConceptCount != 1 || updated.Progress.ChoiceCounts[documents.CorpusConceptReviewRenameNeeded] != 1 {
		t.Fatalf("expected updated review progress: %+v", updated.Progress)
	}
	if len(updated.ReviewRecords.Records) != 1 || updated.ReviewRecords.Records[0].Note != "needs clearer title" {
		t.Fatalf("expected persisted review record: %+v", updated.ReviewRecords)
	}
}

func getCorpusConceptUIState(t *testing.T, handler http.Handler) corpusConceptUIState {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.Host = judgmentUITestHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected state status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	return decodeCorpusConceptUIState(t, rec.Body)
}

func decodeCorpusConceptUIState(t *testing.T, body io.Reader) corpusConceptUIState {
	t.Helper()
	var state corpusConceptUIState
	if err := json.NewDecoder(body).Decode(&state); err != nil {
		t.Fatalf("decode concept UI state: %v", err)
	}
	return state
}
