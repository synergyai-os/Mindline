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
			if len(concept.SourceEvidence) < 2 {
				t.Fatalf("expected source evidence groups: %+v", concept.SourceEvidence)
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

func TestBuildCorpusConceptIndexBlocksIncoherentLinkOnlyRelationNeighborhood(t *testing.T) {
	pressure := CorpusPressureSummary{
		SchemaVersion:            CorpusPressureSummarySchemaVersion,
		CorpusID:                 "corpus-pr46-bad-concept",
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
		AtomCount:         5,
		RelationCount:     5,
		ReplayFingerprint: "graph-fp",
	}
	atoms := []CorpusGraphAtom{
		conceptTestKindAtom("mail-newsletter", "gmail-newsletter", "gmail", "Notification that newsletter needs more articles before sending"),
		conceptTestKindAtom("slack-link-1", "slack-linkedin", "slack", "https://www.linkedin.com/posts/victor-ronchin_every-time-a-new-operator-opens-claude-from-share-7465059065443000320-u8sr"),
		conceptTestKindAtom("slack-link-2", "slack-linkedin", "slack", "- https://www.linkedin.com/posts/victor-ronchin_every-time-a-new-operator-opens-claude-from-share-7465059065443000320-u8sr"),
		conceptTestKindAtom("mail-meeting-1", "gmail-meeting", "gmail", "Snippet: Meeting summary covering workflow improvements and operational updates"),
		conceptTestKindAtom("mail-meeting-2", "gmail-meeting", "gmail", "Meeting summary covering workflow improvements and operational updates"),
	}
	relations := []CorpusGraphRelation{
		conceptTestRelation("rel-news-link", pressure.CorpusID, "mail-newsletter", "slack-link-1"),
		conceptTestRelation("rel-news-link-2", pressure.CorpusID, "mail-newsletter", "slack-link-2"),
		conceptTestRelation("rel-link-meeting-1", pressure.CorpusID, "slack-link-1", "mail-meeting-1"),
		conceptTestRelation("rel-link-meeting", pressure.CorpusID, "slack-link-2", "mail-meeting-1"),
		conceptTestRelation("rel-link-meeting-2", pressure.CorpusID, "slack-link-1", "mail-meeting-2"),
	}

	build := buildCorpusConceptIndex(pressure, graph, atoms, relations, DefaultCorpusConceptsMax)
	var relationConcept *CorpusConcept
	for i := range build.Index.Concepts {
		if strings.HasPrefix(build.Index.Concepts[i].ConceptKey, "relation\x00cross_source\x00") {
			relationConcept = &build.Index.Concepts[i]
			break
		}
	}
	if relationConcept == nil {
		t.Fatalf("expected relation concept: %+v", build.Index.Concepts)
	}
	if relationConcept.Section != CorpusConceptSectionBlocked || relationConcept.ReviewStatus != ReviewStatusBlocked {
		t.Fatalf("expected incoherent relation concept to be blocked, got section=%s status=%s reasons=%v", relationConcept.Section, relationConcept.ReviewStatus, relationConcept.ReasonCodes)
	}
	for _, want := range []string{"weak_cross_source_coherence", "link_only_evidence_requires_enrichment", "duplicate_source_atom_support"} {
		if !containsCorpusConceptString(relationConcept.ReasonCodes, want) {
			t.Fatalf("expected reason %s in %+v", want, relationConcept.ReasonCodes)
		}
	}
	if len(relationConcept.SourceEvidence) != 3 {
		t.Fatalf("expected source evidence collapsed to 3 source groups, got %+v", relationConcept.SourceEvidence)
	}
	for _, source := range relationConcept.SourceEvidence {
		if source.SourceID == "slack-linkedin" && !source.LinkOnly {
			t.Fatalf("expected slack LinkedIn source to be link-only: %+v", source)
		}
		if source.SourceID == "gmail-meeting" && source.DuplicateAtomCount == 0 {
			t.Fatalf("expected duplicate atoms from same Gmail source to be flagged: %+v", source)
		}
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
	return conceptTestKindAtom(id, sourceID, "markdown", title)
}

func conceptTestKindAtom(id, sourceID, sourceKind, title string) CorpusGraphAtom {
	return CorpusGraphAtom{
		SchemaVersion:    CorpusGraphAtomSchemaVersion,
		AtomID:           id,
		CorpusID:         "corpus-concepts-test",
		SourceID:         sourceID,
		SourceKind:       sourceKind,
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

func conceptTestRelation(id, corpusID, fromAtomID, toAtomID string) CorpusGraphRelation {
	return CorpusGraphRelation{
		SchemaVersion: CorpusGraphRelationSchemaVersion,
		RelationID:    id,
		CorpusID:      corpusID,
		RelationType:  CorpusRelationSameTopicAs,
		FromAtomID:    fromAtomID,
		ToAtomID:      toAtomID,
		ReviewStatus:  ReviewStatusReady,
	}
}
