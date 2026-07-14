package documents

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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
		conceptTestKindAtom("a1", "source-alpha", "gmail", "Source methodology requires bounded concept review"),
		conceptTestKindAtom("a2", "source-beta", "slack", "Bounded concept review should replace relation flood"),
		conceptTestKindAtom("a3", "source-gamma", "gmail", "Local accountancy reminder"),
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
			if concept.ReviewWorkKind != CorpusConceptReviewWorkConceptReview {
				t.Fatalf("expected supported cross-source item in concept review, got %s", concept.ReviewWorkKind)
			}
			if strings.Contains(strings.ToLower(concept.Title), "relation neighborhood") {
				t.Fatalf("expected reviewer title, got generic machine label: %s", concept.Title)
			}
			if strings.TrimSpace(concept.GroupingRationale) == "" || strings.TrimSpace(concept.ReviewPrompt) == "" {
				t.Fatalf("expected review rationale and prompt: %+v", concept)
			}
			if strings.TrimSpace(concept.CandidateMeaning) == "" {
				t.Fatalf("expected candidate meaning for human review: %+v", concept)
			}
			if strings.TrimSpace(concept.AcceptMeaning) == "" || !strings.Contains(strings.ToLower(concept.AcceptMeaning), "accepted corpus concept") {
				t.Fatalf("expected explicit accept consequence: %+v", concept.AcceptMeaning)
			}
			if len(concept.DecisionRubric) < 5 {
				t.Fatalf("expected decision rubric for reviewer choices: %+v", concept.DecisionRubric)
			}
			if len(concept.RepresentativeEvidence) < 2 {
				t.Fatalf("expected representative evidence previews: %+v", concept.RepresentativeEvidence)
			}
			if len(concept.SourceEvidence) < 2 {
				t.Fatalf("expected source evidence groups: %+v", concept.SourceEvidence)
			}
			for _, source := range concept.SourceEvidence {
				if strings.TrimSpace(source.Contribution) == "" {
					t.Fatalf("expected interpreted source contribution: %+v", source)
				}
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
	if build.Summary.ConceptReviewBurdenCount != build.Summary.ConceptReviewCount || build.Summary.ConceptReviewBurdenCount == 0 {
		t.Fatalf("concept review burden must count only concept-review work: %+v", build.Summary)
	}
	reviewPacket := corpusConceptReviewMarkdown(build.Summary)
	for _, want := range []string{"candidate=", "accept=", "rubric=Accept:", "contribution="} {
		if !strings.Contains(reviewPacket, want) {
			t.Fatalf("expected review packet to include %q, got:\n%s", want, reviewPacket)
		}
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
	if relationConcept.ReviewWorkKind != CorpusConceptReviewWorkBlockedDiagnostic {
		t.Fatalf("blocked link-only relation must take blocked precedence, got %s", relationConcept.ReviewWorkKind)
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

func TestBuildCorpusConceptIndexBlocksCrossSourceWhenReadableSupportIsOneKindAndOutlier(t *testing.T) {
	pressure := CorpusPressureSummary{
		SchemaVersion:            CorpusPressureSummarySchemaVersion,
		CorpusID:                 "corpus-pr46-modelo-concept",
		SourceCount:              5,
		ProcessedSourceCount:     5,
		ScaleStatus:              "scale_complete",
		CorpusFingerprint:        "corpus-fp",
		CommandConfigFingerprint: "config-fp",
		ReplayFingerprint:        "pressure-fp",
	}
	graph := CorpusGraphSummary{
		SchemaVersion:     CorpusGraphSummarySchemaVersion,
		CorpusID:          pressure.CorpusID,
		SourceCount:       5,
		AtomCount:         5,
		RelationCount:     5,
		ReplayFingerprint: "graph-fp",
	}
	atoms := []CorpusGraphAtom{
		conceptTestKindAtom("modelo-request", "gmail-modelo-request", "gmail", "Request for Modelo 190 payslips and Modelo 111s"),
		conceptTestKindAtom("modelo-confirm", "gmail-modelo-confirm", "gmail", "Confirmation that Modelo 190 was reviewed and correct"),
		conceptTestKindAtom("funding-digest", "gmail-funding-digest", "gmail", "Digest about fresh funding rounds and AI offers"),
		conceptTestKindAtom("slack-link-motion", "slack-motion-link", "slack", "https://www.linkedin.com/posts/adish-jain_today-were-launching-motion-the-frontier-ugcPost"),
		conceptTestKindAtom("slack-link-perspective", "slack-perspective-link", "slack", "https://www.linkedin.com/posts/helensandersonhsa_what-is-your-perspective-on-the-bringing-ugcPost"),
	}
	relations := []CorpusGraphRelation{
		conceptTestRelation("rel-modelo-link", pressure.CorpusID, "modelo-request", "slack-link-motion"),
		conceptTestRelation("rel-link-confirm", pressure.CorpusID, "slack-link-motion", "modelo-confirm"),
		conceptTestRelation("rel-link-funding", pressure.CorpusID, "slack-link-motion", "funding-digest"),
		conceptTestRelation("rel-funding-link", pressure.CorpusID, "funding-digest", "slack-link-perspective"),
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
		t.Fatalf("expected Modelo relation concept to be blocked, got section=%s status=%s reasons=%v", relationConcept.Section, relationConcept.ReviewStatus, relationConcept.ReasonCodes)
	}
	if relationConcept.ReviewWorkKind != CorpusConceptReviewWorkBlockedDiagnostic {
		t.Fatalf("blocked outlier relation must be diagnostic, got %s", relationConcept.ReviewWorkKind)
	}
	for _, want := range []string{"insufficient_readable_source_kind_support", "readable_source_outlier", "link_only_evidence_requires_enrichment"} {
		if !containsCorpusConceptString(relationConcept.ReasonCodes, want) {
			t.Fatalf("expected reason %s in %+v", want, relationConcept.ReasonCodes)
		}
	}
	outlierFound := false
	for _, source := range relationConcept.SourceEvidence {
		if source.SourceID == "gmail-funding-digest" {
			outlierFound = containsCorpusConceptString(source.Flags, "readable_source_outlier")
		}
		if strings.HasPrefix(source.SourceID, "slack-") && source.ReviewableAtomCount != 0 {
			t.Fatalf("expected Slack links to provide no readable support: %+v", source)
		}
	}
	if !outlierFound {
		t.Fatalf("expected funding digest source to be marked as readable outlier: %+v", relationConcept.SourceEvidence)
	}
}

func TestBuildCorpusConceptIndexBlocksGenericSameKindActionTermBucket(t *testing.T) {
	pressure := CorpusPressureSummary{
		SchemaVersion:            CorpusPressureSummarySchemaVersion,
		CorpusID:                 "corpus-pr46-prepare-concept",
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
		RelationCount:     3,
		ReplayFingerprint: "graph-fp",
	}
	atoms := []CorpusGraphAtom{
		conceptTestActionAtom("founder-time", "gmail-founder-time", "Prepare the checklist", "Founder operations note about uninterrupted time to solve business problems."),
		conceptTestActionAtom("invoice-wait", "gmail-invoice-wait", "Prepare the checklist", "Reply asking whether June invoice should wait for updated amount."),
		conceptTestActionAtom("payment-proof", "gmail-payment-proof", "Prepare the checklist", "Request to send proof of payment so system can be updated."),
	}

	build := buildCorpusConceptIndex(pressure, graph, atoms, nil, DefaultCorpusConceptsMax)
	var prepareConcept *CorpusConcept
	for i := range build.Index.Concepts {
		if build.Index.Concepts[i].ConceptKey == "term\x00action_candidate\x00prepare" {
			prepareConcept = &build.Index.Concepts[i]
			break
		}
	}
	if prepareConcept == nil {
		t.Fatalf("expected prepare term concept: %+v", build.Index.Concepts)
	}
	if prepareConcept.Section != CorpusConceptSectionBlocked || prepareConcept.ReviewStatus != ReviewStatusBlocked {
		t.Fatalf("expected generic prepare concept to be blocked, got section=%s status=%s reasons=%v", prepareConcept.Section, prepareConcept.ReviewStatus, prepareConcept.ReasonCodes)
	}
	if !containsCorpusConceptString(prepareConcept.ReasonCodes, "generic_term_bucket_requires_cleanup") {
		t.Fatalf("expected generic term reason in %+v", prepareConcept.ReasonCodes)
	}
	if prepareConcept.ReviewWorkKind != CorpusConceptReviewWorkBlockedDiagnostic {
		t.Fatalf("structurally blocked generic bucket must be diagnostic, got %s", prepareConcept.ReviewWorkKind)
	}
}

func TestBuildCorpusConceptIndexRoutesSingleSourceLocalBucketToCleanupTriage(t *testing.T) {
	pressure := CorpusPressureSummary{
		SchemaVersion:            CorpusPressureSummarySchemaVersion,
		CorpusID:                 "corpus-pr46-workspace-concept",
		SourceCount:              1,
		ProcessedSourceCount:     1,
		ScaleStatus:              "scale_complete",
		CorpusFingerprint:        "corpus-fp",
		CommandConfigFingerprint: "config-fp",
		ReplayFingerprint:        "pressure-fp",
	}
	graph := CorpusGraphSummary{
		SchemaVersion:     CorpusGraphSummarySchemaVersion,
		CorpusID:          pressure.CorpusID,
		SourceCount:       1,
		AtomCount:         2,
		RelationCount:     1,
		ReplayFingerprint: "graph-fp",
	}
	atoms := []CorpusGraphAtom{
		conceptTestKindAtom("workspace-layout", "gmail-workspace", "gmail", "Workspace setup and layout notes"),
		conceptTestKindAtom("workspace-tools", "gmail-workspace", "gmail", "Workspace tools and folder structure"),
	}

	build := buildCorpusConceptIndex(pressure, graph, atoms, nil, DefaultCorpusConceptsMax)
	var workspaceConcept *CorpusConcept
	for i := range build.Index.Concepts {
		if strings.Contains(build.Index.Concepts[i].ConceptKey, "\x00workspace") {
			workspaceConcept = &build.Index.Concepts[i]
			break
		}
	}
	if workspaceConcept == nil {
		t.Fatalf("expected workspace concept: %+v", build.Index.Concepts)
	}
	if workspaceConcept.ReviewWorkKind != CorpusConceptReviewWorkCleanupTriage {
		t.Fatalf("expected workspace concept to be cleanup triage, got %s concept=%+v", workspaceConcept.ReviewWorkKind, workspaceConcept)
	}
	if workspaceConcept.ReviewWorkKind == CorpusConceptReviewWorkConceptReview {
		t.Fatalf("single-source workspace bucket must not be normal concept review: %+v", workspaceConcept)
	}
	if !containsCorpusConceptString(workspaceConcept.ReasonCodes, "single_source_concept") {
		t.Fatalf("expected single-source reason: %+v", workspaceConcept.ReasonCodes)
	}
	if build.Summary.ConceptReviewBurdenCount != 0 {
		t.Fatalf("single-source cleanup must not count as concept-review burden: %+v", build.Summary)
	}
}

func TestCorpusConceptSourceKindUsesExplicitProvenance(t *testing.T) {
	atom := conceptTestKindAtom("source-provenance", "slack-shaped-legacy-id", "notion", "Explicit source provenance")
	if got := sourceKindForConcept(atom); got != "notion" {
		t.Fatalf("source kind must come from explicit provenance, got %q", got)
	}
	if got := corpusConceptSourceRef(atom.SourceID, sourceKindForConcept(atom)); !strings.HasPrefix(got, "notion:") {
		t.Fatalf("source ref must use explicit source kind, got %q", got)
	}
}

func TestCorpusConceptReviewWorkKindProgressAndChoiceValidation(t *testing.T) {
	index := CorpusConceptIndex{
		SchemaVersion: CorpusConceptsSchemaVersion,
		CorpusID:      "corpus-progress",
		Concepts: []CorpusConcept{
			{ConceptID: "concept-review", CorpusID: "corpus-progress", Title: "Reviewable concept", ReviewWorkKind: CorpusConceptReviewWorkConceptReview},
			{ConceptID: "cleanup", CorpusID: "corpus-progress", Title: "Cleanup item", ReviewWorkKind: CorpusConceptReviewWorkCleanupTriage},
			{ConceptID: "enrichment", CorpusID: "corpus-progress", Title: "Enrichment item", ReviewWorkKind: CorpusConceptReviewWorkEnrichmentBacklog},
			{ConceptID: "blocked", CorpusID: "corpus-progress", Title: "Blocked item", ReviewWorkKind: CorpusConceptReviewWorkBlockedDiagnostic},
		},
	}
	records := CorpusConceptReviewRecords{
		SchemaVersion: CorpusConceptReviewRecordsSchemaVersion,
		CorpusID:      "corpus-progress",
		Records: []CorpusConceptReviewRecord{{
			SchemaVersion:  CorpusConceptReviewRecordsSchemaVersion,
			CorpusID:       "corpus-progress",
			ConceptID:      "cleanup",
			ConceptTitle:   "Cleanup item",
			ReviewWorkKind: CorpusConceptReviewWorkCleanupTriage,
			Choice:         CorpusConceptReviewRenameNeeded,
			RecordedAt:     "2026-06-16T10:00:00Z",
		}},
	}

	progress := BuildCorpusConceptReviewProgress(index, records)
	if progress.TotalConceptCount != 1 || progress.RemainingConceptCount != 1 {
		t.Fatalf("default concept-review progress should count only concept_review work: %+v", progress)
	}
	cleanup := progress.WorkKindCounts[CorpusConceptReviewWorkCleanupTriage]
	if cleanup.TotalCount != 1 || cleanup.ReviewedCount != 1 || cleanup.ChoiceCounts[CorpusConceptReviewRenameNeeded] != 1 {
		t.Fatalf("expected cleanup progress bucket, got %+v in %+v", cleanup, progress.WorkKindCounts)
	}
	if allowedCorpusConceptReviewChoice(CorpusConceptReviewWorkCleanupTriage, CorpusConceptReviewAccept) {
		t.Fatalf("cleanup triage must reject accept")
	}
	if !allowedCorpusConceptReviewChoice(CorpusConceptReviewWorkEnrichmentBacklog, CorpusConceptReviewNeedsSourceContext) {
		t.Fatalf("enrichment backlog should allow needs source context")
	}
}

func TestRecordCorpusConceptReviewRejectsInvalidAndMismatchedWorkKind(t *testing.T) {
	root := t.TempDir()
	concept := CorpusConcept{
		SchemaVersion:  CorpusConceptsSchemaVersion,
		ConceptID:      "cleanup-concept",
		CorpusID:       "corpus-record-kind",
		Title:          "local topic: workspace",
		ReviewWorkKind: CorpusConceptReviewWorkCleanupTriage,
		Section:        CorpusConceptSectionLocal,
		CandidateKind:  SemanticCandidateKindTopic,
		ReviewStatus:   ReviewStatusNeedsReview,
	}
	summary := CorpusConceptSummary{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: concept.CorpusID, ConceptCount: 1}
	index := CorpusConceptIndex{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: concept.CorpusID, Concepts: []CorpusConcept{concept}}
	if err := WriteCorpusConceptIndex(root, summary, index); err != nil {
		t.Fatalf("write concept index: %v", err)
	}

	if _, err := RecordCorpusConceptReview(root, CorpusConceptReviewRecordInput{
		ConceptID:      concept.ConceptID,
		ReviewWorkKind: CorpusConceptReviewWorkCleanupTriage,
		Choice:         CorpusConceptReviewAccept,
	}); err == nil {
		t.Fatalf("expected cleanup accept to be rejected")
	}
	if _, err := RecordCorpusConceptReview(root, CorpusConceptReviewRecordInput{
		ConceptID:      concept.ConceptID,
		ReviewWorkKind: CorpusConceptReviewWorkConceptReview,
		Choice:         CorpusConceptReviewRejectNoisy,
	}); err == nil {
		t.Fatalf("expected mismatched work kind to be rejected")
	}
	if _, err := RecordCorpusConceptReview(root, CorpusConceptReviewRecordInput{
		ConceptID:      concept.ConceptID,
		ReviewWorkKind: CorpusConceptReviewWorkCleanupTriage,
		Choice:         CorpusConceptReviewRenameNeeded,
	}); err != nil {
		t.Fatalf("expected valid cleanup choice, got %v", err)
	}
	records, err := RecordCorpusConceptReview(root, CorpusConceptReviewRecordInput{
		ConceptID: concept.ConceptID,
		Choice:    CorpusConceptReviewRenameNeeded,
	})
	if err != nil {
		t.Fatalf("expected omitted input work kind to infer concept work kind, got %v", err)
	}
	cleanup := records.ReviewWorkKindProgress[CorpusConceptReviewWorkCleanupTriage]
	if cleanup.TotalCount != 1 || cleanup.ReviewedCount != 1 || cleanup.ChoiceCounts[CorpusConceptReviewRenameNeeded] != 1 {
		t.Fatalf("expected persisted cleanup review progress, got %+v", records.ReviewWorkKindProgress)
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
		conceptTestKindAtom("a1", "source-alpha", "gmail", "Review surface needs readable evidence"),
		conceptTestKindAtom("a2", "source-beta", "slack", "Readable evidence supports review decisions"),
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
	workKind := records.Records[0].ReviewWorkKind
	progressBucket := records.ReviewWorkKindProgress[workKind]
	if progressBucket.TotalCount == 0 || progressBucket.ReviewedCount != 1 || progressBucket.ChoiceCounts[CorpusConceptReviewSplitNeeded] != 1 {
		t.Fatalf("expected persisted work-kind progress: workKind=%s records=%+v", workKind, records)
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

func TestRecordCorpusConceptReviewSerializesConcurrentWrites(t *testing.T) {
	root := t.TempDir()
	index := CorpusConceptIndex{
		SchemaVersion: CorpusConceptsSchemaVersion,
		CorpusID:      "corpus-concurrent-review",
		Concepts: []CorpusConcept{
			{SchemaVersion: CorpusConceptsSchemaVersion, ConceptID: "concept-alpha", CorpusID: "corpus-concurrent-review", Title: "Alpha", ReviewWorkKind: CorpusConceptReviewWorkConceptReview},
			{SchemaVersion: CorpusConceptsSchemaVersion, ConceptID: "concept-beta", CorpusID: "corpus-concurrent-review", Title: "Beta", ReviewWorkKind: CorpusConceptReviewWorkConceptReview},
		},
	}
	summary := CorpusConceptSummary{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: index.CorpusID, ConceptCount: len(index.Concepts)}
	if err := WriteCorpusConceptIndex(root, summary, index); err != nil {
		t.Fatalf("write concept index: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(ordinal int) {
			defer wg.Done()
			conceptID := index.Concepts[ordinal%len(index.Concepts)].ConceptID
			_, err := RecordCorpusConceptReview(root, CorpusConceptReviewRecordInput{
				ConceptID: conceptID,
				Choice:    CorpusConceptReviewSplitNeeded,
				Note:      "concurrent review",
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent review write failed: %v", err)
	}
	records, err := ReadCorpusConceptReviewRecords(root)
	if err != nil {
		t.Fatalf("read concurrent review records: %v", err)
	}
	if len(records.Records) != 2 {
		t.Fatalf("expected one durable record per concept, got %+v", records.Records)
	}
	bucket := records.ReviewWorkKindProgress[CorpusConceptReviewWorkConceptReview]
	if bucket.TotalCount != 2 || bucket.ReviewedCount != 2 || bucket.RemainingCount != 0 {
		t.Fatalf("unexpected concurrent progress: %+v", bucket)
	}
}

func TestReadCorpusConceptReviewRecordsRejectsStaleReviewContract(t *testing.T) {
	root := t.TempDir()
	concept := CorpusConcept{
		SchemaVersion:  CorpusConceptsSchemaVersion,
		ConceptID:      "concept-stale-contract",
		CorpusID:       "corpus-stale-contract",
		Title:          "Reviewable concept",
		ReviewWorkKind: CorpusConceptReviewWorkConceptReview,
	}
	index := CorpusConceptIndex{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: concept.CorpusID, Concepts: []CorpusConcept{concept}}
	summary := CorpusConceptSummary{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: concept.CorpusID, ConceptCount: 1}
	if err := WriteCorpusConceptIndex(root, summary, index); err != nil {
		t.Fatalf("write concept index: %v", err)
	}
	if _, err := RecordCorpusConceptReview(root, CorpusConceptReviewRecordInput{ConceptID: concept.ConceptID, Choice: CorpusConceptReviewAccept}); err != nil {
		t.Fatalf("record initial review: %v", err)
	}
	index.Concepts[0].ReviewWorkKind = CorpusConceptReviewWorkCleanupTriage
	index.Concepts[0].ReviewPrompt = "Route this to cleanup instead of concept acceptance."
	if err := WriteCorpusConceptIndex(root, summary, index); err != nil {
		t.Fatalf("regenerate concept index: %v", err)
	}
	if _, err := ReadCorpusConceptReviewRecords(root); err == nil || !strings.Contains(err.Error(), "contract fingerprint mismatch") {
		t.Fatalf("expected stale review contract rejection, got %v", err)
	}
}

func TestRecordCorpusConceptReviewRejectsSymlinkedRoot(t *testing.T) {
	targetRoot := t.TempDir()
	concept := CorpusConcept{
		SchemaVersion:  CorpusConceptsSchemaVersion,
		ConceptID:      "concept-symlink-root",
		CorpusID:       "corpus-symlink-root",
		Title:          "Reviewable concept",
		ReviewWorkKind: CorpusConceptReviewWorkConceptReview,
	}
	index := CorpusConceptIndex{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: concept.CorpusID, Concepts: []CorpusConcept{concept}}
	summary := CorpusConceptSummary{SchemaVersion: CorpusConceptsSchemaVersion, CorpusID: concept.CorpusID, ConceptCount: 1}
	if err := WriteCorpusConceptIndex(targetRoot, summary, index); err != nil {
		t.Fatalf("write target concept index: %v", err)
	}
	linkParent := filepath.Join(t.TempDir(), "linked-output")
	if err := os.MkdirAll(linkParent, 0o755); err != nil {
		t.Fatalf("mkdir link parent: %v", err)
	}
	if err := os.Symlink(filepath.Join(targetRoot, CorpusConceptsDirName), filepath.Join(linkParent, CorpusConceptsDirName)); err != nil {
		t.Fatalf("create corpus concept symlink: %v", err)
	}
	if _, err := RecordCorpusConceptReview(linkParent, CorpusConceptReviewRecordInput{ConceptID: concept.ConceptID, Choice: CorpusConceptReviewAccept}); err == nil {
		t.Fatalf("expected symlinked concept root to be rejected")
	}
	if _, err := os.Stat(filepath.Join(targetRoot, CorpusConceptsDirName, "review-records.json")); !os.IsNotExist(err) {
		t.Fatalf("symlink rejection must not write target records, stat err=%v", err)
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
	records := CorpusConceptReviewRecords{
		SchemaVersion:             CorpusConceptReviewRecordsSchemaVersion,
		CorpusID:                  index.CorpusID,
		ReviewContractFingerprint: CorpusConceptReviewContractFingerprint(index),
		Records:                   []CorpusConceptReviewRecord{},
	}
	records.ReviewWorkKindProgress = BuildCorpusConceptReviewProgress(index, records).WorkKindCounts
	if err := writeJSON(filepath.Join(root, CorpusConceptsDirName), "review-records.json", records); err != nil {
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

func conceptTestActionAtom(id, sourceID, title, excerpt string) CorpusGraphAtom {
	atom := conceptTestKindAtom(id, sourceID, "gmail", title)
	atom.CandidateKind = SemanticCandidateKindAction
	atom.Summary = excerpt
	atom.Excerpt = excerpt
	return atom
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
