package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestCorpusConceptUIServesReviewStateAndRecordsDecision(t *testing.T) {
	root := t.TempDir()
	concept := documents.CorpusConcept{
		SchemaVersion:     documents.CorpusConceptsSchemaVersion,
		ConceptID:         "concept-ui-test",
		CorpusID:          "corpus-ui-test",
		Title:             "Cross-source topic concept: review, evidence",
		ReviewPrompt:      "Decide whether these snippets describe one coherent concept.",
		GroupingRationale: "Grouped from cross-source relations across 2 sources.",
		CandidateMeaning:  "Possible corpus concept: review and evidence describe the same source-backed decision workflow.",
		AcceptMeaning:     "Accept means these sources independently support one accepted corpus concept for later proposal work.",
		DecisionRubric: []documents.CorpusConceptDecisionCriterion{
			{Choice: documents.CorpusConceptReviewAccept, Label: "Accept", Criterion: "Use only when the candidate meaning is supported by all readable source contributions."},
			{Choice: documents.CorpusConceptReviewSplitNeeded, Label: "Split", Criterion: "Use when the sources contain different meanings."},
			{Choice: documents.CorpusConceptReviewNeedsSourceContext, Label: "Need context", Criterion: "Use when the excerpts are insufficient to judge."},
		},
		Section:                documents.CorpusConceptSectionCrossSource,
		CandidateKind:          documents.SemanticCandidateKindTopic,
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
		SourceEvidence: []documents.CorpusConceptSourceEvidence{{
			SourceID:            "gmail-source-ui",
			SourceKind:          "gmail",
			SourceRef:           "gmail:source-ui",
			AtomCount:           1,
			ReviewableAtomCount: 1,
			Contribution:        "Gmail supports the candidate by saying readable evidence is needed before review decisions.",
			Evidence: []documents.CorpusConceptEvidencePreview{{
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
			CandidateMeaning:       concept.CandidateMeaning,
			AcceptMeaning:          concept.AcceptMeaning,
			DecisionRubric:         concept.DecisionRubric,
			Section:                concept.Section,
			CandidateKind:          concept.CandidateKind,
			AtomCount:              concept.AtomCount,
			SourceCount:            concept.SourceCount,
			EvidenceReferenceCount: concept.EvidenceReferenceCount,
			SourceKindCoverage:     concept.SourceKindCoverage,
			ReviewStatus:           concept.ReviewStatus,
			RepresentativeEvidence: len(concept.RepresentativeEvidence),
			SourceEvidence:         len(concept.SourceEvidence),
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
	if !strings.Contains(rec.Body.String(), "Copy concept") || !strings.Contains(rec.Body.String(), "copy-fallback-text") || !strings.Contains(rec.Body.String(), "Mindline concept review packet") || !strings.Contains(rec.Body.String(), "Source Evidence") {
		t.Fatalf("expected concept copy affordance in UI")
	}
	for _, want := range []string{"Candidate Meaning", "Accept means", "Decision rubric", "Candidate meaning:", "Accept means:"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("expected review contract marker %q in UI", want)
		}
	}

	state := getCorpusConceptUIState(t, handler)
	if state.Progress.TotalConceptCount != 1 || state.Progress.ReviewedConceptCount != 0 {
		t.Fatalf("unexpected initial progress: %+v", state.Progress)
	}
	if got := state.Index.Concepts[0].RepresentativeEvidence[0].Excerpt; got == "" {
		t.Fatalf("expected representative evidence excerpt")
	}
	if got := state.Index.Concepts[0].SourceEvidence[0].Evidence[0].Excerpt; got == "" {
		t.Fatalf("expected source evidence excerpt")
	}
	if got := state.Index.Concepts[0].CandidateMeaning; got == "" {
		t.Fatalf("expected candidate meaning in API state")
	}
	if got := state.Index.Concepts[0].SourceEvidence[0].Contribution; got == "" {
		t.Fatalf("expected interpreted source contribution in API state")
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

func TestCorpusConceptUISeparatesWorkKindProgressAndRejectsInvalidChoice(t *testing.T) {
	root := t.TempDir()
	conceptReview := documents.CorpusConcept{
		SchemaVersion:  documents.CorpusConceptsSchemaVersion,
		ConceptID:      "concept-review-ui",
		CorpusID:       "corpus-ui-work-kind",
		Title:          "Cross-source topic concept: review",
		ReviewPrompt:   "Decide whether these Gmail and Slack snippets describe one coherent concept.",
		Section:        documents.CorpusConceptSectionCrossSource,
		ReviewStatus:   documents.ReviewStatusNeedsReview,
		ReviewWorkKind: documents.CorpusConceptReviewWorkConceptReview,
		SourceCount:    2,
		AtomCount:      2,
	}
	cleanup := documents.CorpusConcept{
		SchemaVersion:  documents.CorpusConceptsSchemaVersion,
		ConceptID:      "cleanup-workspace-ui",
		CorpusID:       "corpus-ui-work-kind",
		Title:          "local topic: workspace",
		ReviewPrompt:   "Use this as extraction cleanup feedback, not as accepted knowledge.",
		Section:        documents.CorpusConceptSectionLocal,
		ReviewStatus:   documents.ReviewStatusNeedsReview,
		ReviewWorkKind: documents.CorpusConceptReviewWorkCleanupTriage,
		SourceCount:    1,
		AtomCount:      2,
		ReasonCodes:    []string{"single_source_concept", "duplicate_source_atom_support"},
	}
	enrichment := documents.CorpusConcept{
		SchemaVersion:  documents.CorpusConceptsSchemaVersion,
		ConceptID:      "enrichment-link-ui",
		CorpusID:       "corpus-ui-work-kind",
		Title:          "Unread source link",
		ReviewPrompt:   "Decide whether this source needs more context.",
		Section:        documents.CorpusConceptSectionNeedsReview,
		ReviewStatus:   documents.ReviewStatusNeedsReview,
		ReviewWorkKind: documents.CorpusConceptReviewWorkEnrichmentBacklog,
		SourceCount:    1,
		AtomCount:      1,
	}
	blocked := documents.CorpusConcept{
		SchemaVersion:  documents.CorpusConceptsSchemaVersion,
		ConceptID:      "blocked-diagnostic-ui",
		CorpusID:       "corpus-ui-work-kind",
		Title:          "Blocked evidence",
		ReviewPrompt:   "Inspect the blocker before concept review.",
		Section:        documents.CorpusConceptSectionBlocked,
		ReviewStatus:   documents.ReviewStatusBlocked,
		ReviewWorkKind: documents.CorpusConceptReviewWorkBlockedDiagnostic,
		SourceCount:    1,
		AtomCount:      1,
	}
	summary := documents.CorpusConceptSummary{
		SchemaVersion: documents.CorpusConceptsSchemaVersion,
		CorpusID:      "corpus-ui-work-kind",
		ConceptCount:  4,
		ReviewWorkKindCounts: map[documents.CorpusConceptReviewWorkKind]int{
			documents.CorpusConceptReviewWorkConceptReview:     1,
			documents.CorpusConceptReviewWorkCleanupTriage:     1,
			documents.CorpusConceptReviewWorkEnrichmentBacklog: 1,
			documents.CorpusConceptReviewWorkBlockedDiagnostic: 1,
		},
	}
	index := documents.CorpusConceptIndex{
		SchemaVersion: documents.CorpusConceptsSchemaVersion,
		CorpusID:      "corpus-ui-work-kind",
		Concepts:      []documents.CorpusConcept{conceptReview, cleanup, enrichment, blocked},
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
	for _, marker := range []string{"function activeWorkProgress(progress)", "progress.work_kind_counts", "Reviewed · ", "remaining in ", "bucket.total_count"} {
		if !strings.Contains(rec.Body.String(), marker) {
			t.Fatalf("expected lane-specific progress marker %q", marker)
		}
	}

	state := getCorpusConceptUIState(t, handler)
	if state.Progress.TotalConceptCount != 1 {
		t.Fatalf("expected default progress to count only concept review: %+v", state.Progress)
	}
	if got := state.Progress.WorkKindCounts[documents.CorpusConceptReviewWorkCleanupTriage].TotalCount; got != 1 {
		t.Fatalf("expected cleanup progress count, got %+v", state.Progress.WorkKindCounts)
	}
	for _, kind := range []documents.CorpusConceptReviewWorkKind{documents.CorpusConceptReviewWorkEnrichmentBacklog, documents.CorpusConceptReviewWorkBlockedDiagnostic} {
		if got := state.Progress.WorkKindCounts[kind].TotalCount; got != 1 {
			t.Fatalf("expected %s progress count, got %+v", kind, state.Progress.WorkKindCounts)
		}
	}
	if state.Index.Concepts[1].ReviewWorkKind != documents.CorpusConceptReviewWorkCleanupTriage {
		t.Fatalf("expected cleanup work kind in API state: %+v", state.Index.Concepts[1])
	}

	payload := `{"concept_id":"cleanup-workspace-ui","review_work_kind":"cleanup_triage","choice":"accept"}`
	req = httptest.NewRequest(http.MethodPost, "/api/reviews", strings.NewReader(payload))
	req.Host = judgmentUITestHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", "test-token")
	req.Header.Set("Origin", "http://"+judgmentUITestHost)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid cleanup accept status 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	payload = `{"concept_id":"cleanup-workspace-ui","review_work_kind":"cleanup_triage","choice":"rename_needed"}`
	req = httptest.NewRequest(http.MethodPost, "/api/reviews", strings.NewReader(payload))
	req.Host = judgmentUITestHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", "test-token")
	req.Header.Set("Origin", "http://"+judgmentUITestHost)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected valid cleanup status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	updated := decodeCorpusConceptUIState(t, rec.Body)
	if got := updated.ReviewRecords.Records[0].ReviewWorkKind; got != documents.CorpusConceptReviewWorkCleanupTriage {
		t.Fatalf("expected review record work kind, got %s", got)
	}
}

func TestCorpusConceptUIDoesNotFallbackZeroConceptReviewProgressToAllConcepts(t *testing.T) {
	root := t.TempDir()
	cleanup := documents.CorpusConcept{
		SchemaVersion:  documents.CorpusConceptsSchemaVersion,
		ConceptID:      "cleanup-workspace-ui",
		CorpusID:       "corpus-ui-cleanup-only",
		Title:          "local topic: workspace",
		ReviewPrompt:   "Use this as extraction cleanup feedback, not as accepted knowledge.",
		Section:        documents.CorpusConceptSectionLocal,
		ReviewStatus:   documents.ReviewStatusNeedsReview,
		ReviewWorkKind: documents.CorpusConceptReviewWorkCleanupTriage,
		SourceCount:    1,
		AtomCount:      2,
		ReasonCodes:    []string{"single_source_concept", "duplicate_source_atom_support"},
	}
	summary := documents.CorpusConceptSummary{
		SchemaVersion:      documents.CorpusConceptsSchemaVersion,
		CorpusID:           "corpus-ui-cleanup-only",
		ConceptCount:       1,
		CleanupTriageCount: 1,
		ReviewWorkKindCounts: map[documents.CorpusConceptReviewWorkKind]int{
			documents.CorpusConceptReviewWorkCleanupTriage: 1,
		},
	}
	index := documents.CorpusConceptIndex{
		SchemaVersion: documents.CorpusConceptsSchemaVersion,
		CorpusID:      "corpus-ui-cleanup-only",
		Concepts:      []documents.CorpusConcept{cleanup},
	}
	if err := documents.WriteCorpusConceptIndex(root, summary, index); err != nil {
		t.Fatalf("write concept index: %v", err)
	}
	handler := newCorpusConceptUIHandlerWithToken(filepath.Join(root, documents.CorpusConceptsDirName), "test-token", []string{judgmentUITestHost})
	state := getCorpusConceptUIState(t, handler)
	if state.Progress.TotalConceptCount != 0 || state.Progress.RemainingConceptCount != 0 {
		t.Fatalf("expected zero default concept-review progress for cleanup-only queue: %+v", state.Progress)
	}
	if got := state.Progress.WorkKindCounts[documents.CorpusConceptReviewWorkCleanupTriage].TotalCount; got != 1 {
		t.Fatalf("expected cleanup work-kind progress to retain cleanup item, got %+v", state.Progress.WorkKindCounts)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = judgmentUITestHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected html status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `lane === "concept_review" ? numberOr(progress.total_concept_count, 0) : 0`) {
		t.Fatalf("expected UI to preserve zero concept-review totals without || fallback")
	}
}

func TestCorpusConceptUIWriteSafetyAndStateReadOnly(t *testing.T) {
	root := t.TempDir()
	concept := documents.CorpusConcept{
		SchemaVersion:  documents.CorpusConceptsSchemaVersion,
		ConceptID:      "concept-review-ui",
		CorpusID:       "corpus-ui-safety",
		Title:          "Cross-source topic concept: review",
		ReviewPrompt:   "Decide whether these snippets describe one coherent concept.",
		Section:        documents.CorpusConceptSectionCrossSource,
		ReviewStatus:   documents.ReviewStatusNeedsReview,
		ReviewWorkKind: documents.CorpusConceptReviewWorkConceptReview,
		SourceCount:    2,
		AtomCount:      2,
	}
	summary := documents.CorpusConceptSummary{
		SchemaVersion: documents.CorpusConceptsSchemaVersion,
		CorpusID:      "corpus-ui-safety",
		ConceptCount:  1,
		ReviewWorkKindCounts: map[documents.CorpusConceptReviewWorkKind]int{
			documents.CorpusConceptReviewWorkConceptReview: 1,
		},
	}
	index := documents.CorpusConceptIndex{
		SchemaVersion: documents.CorpusConceptsSchemaVersion,
		CorpusID:      "corpus-ui-safety",
		Concepts:      []documents.CorpusConcept{concept},
	}
	if err := documents.WriteCorpusConceptIndex(root, summary, index); err != nil {
		t.Fatalf("write concept index: %v", err)
	}
	handler := newCorpusConceptUIHandlerWithToken(filepath.Join(root, documents.CorpusConceptsDirName), "test-token", []string{judgmentUITestHost})

	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.Host = judgmentUITestHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected state status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := documents.ReadCorpusConceptReviewRecords(root); err != nil {
		t.Fatalf("read empty records: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, documents.CorpusConceptsDirName, "review-records.json")); !os.IsNotExist(err) {
		t.Fatalf("expected /api/state to be read-only, stat err=%v", err)
	}

	payload := `{"concept_id":"concept-review-ui","review_work_kind":"concept_review","choice":"accept"}`
	req = httptest.NewRequest(http.MethodPost, "/api/reviews", strings.NewReader(payload))
	req.Host = "127.0.0.1:9999"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", "test-token")
	req.Header.Set("Origin", "http://127.0.0.1:9999")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected unconfigured loopback host status 403, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/reviews", strings.NewReader(payload))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", "test-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected non-loopback host status 403, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/reviews", strings.NewReader(payload))
	req.Host = judgmentUITestHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", "test-token")
	req.Header.Set("Origin", "http://127.0.0.1:9999")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin status 403, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/reviews", strings.NewReader("not-json"))
	req.Host = judgmentUITestHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", "test-token")
	req.Header.Set("Origin", "http://"+judgmentUITestHost)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid JSON status 400, got %d", rec.Code)
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
