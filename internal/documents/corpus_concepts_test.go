package documents

import (
	"os"
	"path/filepath"
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
		}
		if concept.WriteEligible {
			t.Fatalf("concepts must remain write-ineligible: %+v", concept)
		}
	}
	if !foundMixed {
		t.Fatalf("expected mixed source-kind coverage: %+v", build.Index.Concepts)
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
		LineStart:        1,
		LineEnd:          2,
		ContentHash:      "hash-" + id,
	}
}
