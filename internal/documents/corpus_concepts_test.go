package documents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCorpusConceptIndexGroupsCrossSourceConcepts(t *testing.T) {
	pressure := CorpusPressureSummary{
		SchemaVersion:            CorpusPressureSummarySchemaVersion,
		CorpusID:                 "corpus-concepts-test",
		SourceCount:              3,
		ProcessedSourceCount:     3,
		ScaleStatus:              "scale_complete",
		CorpusFingerprint:        "corpus-fp",
		CommandConfigFingerprint: "config-fp",
		ReplayFingerprint:        "pressure-fp",
	}
	graph := CorpusGraphSummary{
		SchemaVersion:     CorpusGraphSummarySchemaVersion,
		CorpusID:          pressure.CorpusID,
		SourceCount:       3,
		AtomCount:         3,
		RelationCount:     250,
		ReplayFingerprint: "graph-fp",
	}
	atoms := []CorpusGraphAtom{
		conceptTestAtom("a1", "gmail-alpha", "Source methodology requires bounded concept review"),
		conceptTestAtom("a2", "slack-alpha", "Bounded concept review should replace relation flood"),
		conceptTestAtom("a3", "gmail-beta", "Local accountancy reminder"),
	}
	relations := []CorpusGraphRelation{{
		SchemaVersion: CorpusGraphRelationSchemaVersion,
		RelationID:    "rel-a1-a2",
		CorpusID:      pressure.CorpusID,
		RelationType:  CorpusRelationSameTopicAs,
		FromAtomID:    "a1",
		ToAtomID:      "a2",
		ReviewStatus:  ReviewStatusReady,
	}}

	build := buildCorpusConceptIndex(pressure, graph, atoms, relations, DefaultCorpusConceptsMax)
	if build.Summary.ConceptCount == 0 {
		t.Fatalf("expected concept groups")
	}
	if build.Summary.CrossSourceConceptCount == 0 {
		t.Fatalf("expected cross-source concept: %+v", build.Summary)
	}
	if build.Summary.RelationReviewCompressionRatio <= 0.99 {
		t.Fatalf("expected relation review compression, got %.4f", build.Summary.RelationReviewCompressionRatio)
	}
	foundMixed := false
	for _, concept := range build.Index.Concepts {
		if concept.Section == CorpusConceptSectionCrossSource && concept.SourceKindCoverage["gmail"] > 0 && concept.SourceKindCoverage["slack"] > 0 {
			foundMixed = true
			if strings.Contains(strings.ToLower(concept.Title), "relation neighborhood") {
				t.Fatalf("expected reviewer title, got generic machine label: %s", concept.Title)
			}
			if strings.TrimSpace(concept.GroupingRationale) == "" || strings.TrimSpace(concept.ReviewPrompt) == "" {
				t.Fatalf("expected review rationale and prompt: %+v", concept)
			}
			if len(concept.RepresentativeEvidence) < 2 {
				t.Fatalf("expected representative evidence previews: %+v", concept.RepresentativeEvidence)
			}
			for _, preview := range concept.RepresentativeEvidence {
				if strings.TrimSpace(preview.Excerpt) == "" || strings.TrimSpace(preview.Title) == "" {
					t.Fatalf("expected readable evidence preview: %+v", preview)
				}
			}
		}
		if concept.WriteEligible {
			t.Fatalf("concepts must remain write-ineligible: %+v", concept)
		}
	}
	if !foundMixed {
		t.Fatalf("expected mixed source-kind coverage: %+v", build.Index.Concepts)
	}
}

func TestRecordCorpusConceptReviewPersistsProgress(t *testing.T) {
	root := t.TempDir()
	pressure := CorpusPressureSummary{
		SchemaVersion:            CorpusPressureSummarySchemaVersion,
		CorpusID:                 "corpus-wp46-review-test",
		SourceCount:              2,
		ProcessedSourceCount:     2,
		ScaleStatus:              "scale_complete",
		CorpusFingerprint:        "corpus-fp",
		CommandConfigFingerprint: "config-fp",
		ReplayFingerprint:        "pressure-fp",
	}
	graph := CorpusGraphSummary{
		SchemaVersion:     CorpusGraphSummarySchemaVersion,
		CorpusID:          pressure.CorpusID,
		SourceCount:       2,
		AtomCount:         2,
		RelationCount:     12,
		ReplayFingerprint: "graph-fp",
	}
	atoms := []CorpusGraphAtom{
		conceptTestAtom("a1", "gmail-alpha", "Review surface needs readable evidence"),
		conceptTestAtom("a2", "slack-alpha", "Readable evidence supports review decisions"),
	}
	relations := []CorpusGraphRelation{{
		SchemaVersion: CorpusGraphRelationSchemaVersion,
		RelationID:    "rel-a1-a2",
		CorpusID:      pressure.CorpusID,
		RelationType:  CorpusRelationSameTopicAs,
		FromAtomID:    "a1",
		ToAtomID:      "a2",
		ReviewStatus:  ReviewStatusReady,
	}}
	build := buildCorpusConceptIndex(pressure, graph, atoms, relations, DefaultCorpusConceptsMax)
	if err := WriteCorpusConceptIndex(root, build.Summary, build.Index); err != nil {
		t.Fatalf("write concept index: %v", err)
	}
	conceptID := build.Index.Concepts[0].ConceptID
	records, err := RecordCorpusConceptReview(root, CorpusConceptReviewRecordInput{
		ConceptID:  conceptID,
		Choice:     CorpusConceptReviewSplitNeeded,
		Note:       "too broad",
		ReviewerID: "reviewer-test",
	})
	if err != nil {
		t.Fatalf("record concept review: %v", err)
	}
	if len(records.Records) != 1 || records.Records[0].Choice != CorpusConceptReviewSplitNeeded {
		t.Fatalf("expected persisted review record: %+v", records)
	}
	readBack, err := ReadCorpusConceptReviewRecords(root)
	if err != nil {
		t.Fatalf("read review records: %v", err)
	}
	progress := BuildCorpusConceptReviewProgress(build.Index, readBack)
	if progress.ReviewedConceptCount != 1 || progress.RemainingConceptCount != len(build.Index.Concepts)-1 || progress.ChoiceCounts[CorpusConceptReviewSplitNeeded] != 1 {
		t.Fatalf("unexpected review progress: %+v", progress)
	}
}

func TestWriteCorpusConceptIndexRejectsUnexpectedFiles(t *testing.T) {
	root := t.TempDir()
	summary := CorpusConceptSummary{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: "corpus-a"}
	index := CorpusConceptIndex{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: "corpus-a"}
	if err := WriteCorpusConceptIndex(root, summary, index); err != nil {
		t.Fatalf("write concept index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, CorpusConceptsDirName, "stale.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	if err := WriteCorpusConceptIndex(root, summary, index); err == nil {
		t.Fatalf("expected stale file rejection")
	}
}

func TestWriteCorpusConceptIndexAllowsReviewRecords(t *testing.T) {
	root := t.TempDir()
	summary := CorpusConceptSummary{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: "corpus-a"}
	index := CorpusConceptIndex{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: "corpus-a"}
	if err := WriteCorpusConceptIndex(root, summary, index); err != nil {
		t.Fatalf("write concept index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, CorpusConceptsDirName, "review-records.json"), []byte(`{"schema_version":"corpus-concept-review-records/v0.1","corpus_id":"corpus-a","records":[]}`), 0o644); err != nil {
		t.Fatalf("write review records: %v", err)
	}
	if err := WriteCorpusConceptIndex(root, summary, index); err != nil {
		t.Fatalf("expected review records to be allowed, got %v", err)
	}
}

func conceptTestAtom(id, sourceID, title string) CorpusGraphAtom {
	return CorpusGraphAtom{
		SchemaVersion:    CorpusGraphAtomSchemaVersion,
		AtomID:           id,
		CorpusID:         "corpus-concepts-test",
		SourceID:         sourceID,
		SourceKind:       "markdown",
		SourceDocumentID: sourceID,
		CandidateKind:    SemanticCandidateKindTopic,
		ReviewStatus:     ReviewStatusReady,
		Confidence:       ConfidenceMedium,
		Title:            title,
		Summary:          title,
		Excerpt:          title + " excerpt",
		LineStart:        1,
		LineEnd:          2,
		ContentHash:      "hash-" + id,
	}
}
