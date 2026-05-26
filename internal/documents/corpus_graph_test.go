package documents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusGraphBuildsRelationsAndMetrics(t *testing.T) {
	root := writeCorpusGraphFixture(t, "run-a")
	summary, atoms, relations, reviews, err := BuildCorpusGraph(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatalf("build corpus graph: %v", err)
	}
	if summary.AtomCount != 8 {
		t.Fatalf("atom count = %d, want 8", summary.AtomCount)
	}
	for _, relationType := range []CorpusRelationType{CorpusRelationPossibleDuplicate, CorpusRelationContradicts, CorpusRelationSupersedes, CorpusRelationSameTopicAs} {
		if summary.RelationTypeCounts[relationType] == 0 {
			t.Fatalf("missing relation type %s in %#v", relationType, summary.RelationTypeCounts)
		}
	}
	if summary.RelationMetrics.Precision < 0.95 {
		t.Fatalf("precision = %.2f, want >= .95", summary.RelationMetrics.Precision)
	}
	if summary.RelationMetrics.Recall < 0.90 {
		t.Fatalf("recall = %.2f, want >= .90", summary.RelationMetrics.Recall)
	}
	if summary.ReviewBurdenRatio > 0.20 {
		t.Fatalf("review burden = %.2f, want <= .20", summary.ReviewBurdenRatio)
	}
	if len(atoms) == 0 || len(relations) == 0 || len(reviews) != len(relations) {
		t.Fatalf("expected atoms, relations, and one review per relation")
	}
	for _, atom := range atoms {
		if atom.SourceLabel == "" || atom.SourceDocumentID == "" || atom.LineStart == 0 || atom.LineEnd == 0 || strings.TrimSpace(atom.Excerpt) == "" {
			t.Fatalf("atom missing evidence: %#v", atom)
		}
	}
}

func TestCorpusGraphReplayIgnoresSemanticRunScopedIDs(t *testing.T) {
	rootA := writeCorpusGraphFixture(t, "run-a")
	rootB := writeCorpusGraphFixture(t, "run-b")
	summaryA, atomsA, relationsA, _, err := BuildCorpusGraph(filepath.Join(rootA, "manifest.json"))
	if err != nil {
		t.Fatalf("build corpus graph A: %v", err)
	}
	summaryB, atomsB, relationsB, _, err := BuildCorpusGraph(filepath.Join(rootB, "manifest.json"))
	if err != nil {
		t.Fatalf("build corpus graph B: %v", err)
	}
	if summaryA.ReplayFingerprint != summaryB.ReplayFingerprint {
		t.Fatalf("fingerprint changed with run-scoped IDs: %s != %s", summaryA.ReplayFingerprint, summaryB.ReplayFingerprint)
	}
	for i := range atomsA {
		if atomsA[i].AtomID != atomsB[i].AtomID {
			t.Fatalf("atom ID changed at %d: %s != %s", i, atomsA[i].AtomID, atomsB[i].AtomID)
		}
		if atomsA[i].Provenance.SemanticCandidateID == atomsB[i].Provenance.SemanticCandidateID {
			t.Fatalf("test fixture did not vary semantic candidate IDs")
		}
	}
	for i := range relationsA {
		if relationsA[i].RelationID != relationsB[i].RelationID {
			t.Fatalf("relation ID changed at %d: %s != %s", i, relationsA[i].RelationID, relationsB[i].RelationID)
		}
	}
}

func TestCorpusGraphSupersedesMetricsAreDirectional(t *testing.T) {
	relation := CorpusGraphRelation{
		RelationID:   "rel-directed",
		RelationType: CorpusRelationSupersedes,
		FromAtomID:   "atom-new",
		ToAtomID:     "atom-old",
		ReviewStatus: ReviewStatusReady,
		Evidence: []CorpusGraphRelationEvidence{
			{AtomID: "atom-new", SourceID: "source", SourceLabel: "source.md", SourceDocumentID: "doc", LineStart: 1, LineEnd: 1, Excerpt: "new", ContentHash: "hash-new"},
			{AtomID: "atom-old", SourceID: "source", SourceLabel: "source.md", SourceDocumentID: "doc", LineStart: 2, LineEnd: 2, Excerpt: "old", ContentHash: "hash-old"},
		},
	}
	atoms := []CorpusGraphAtom{
		{AtomID: "atom-new", Title: "Use new checklist"},
		{AtomID: "atom-old", Title: "Use legacy checklist"},
	}
	metrics := evaluateCorpusRelations([]CorpusGraphRelation{relation}, CorpusGraphAnswerKey{
		SchemaVersion: CorpusGraphAnswerKeySchemaVersion,
		Relations: []CorpusGraphAnswerRelation{{
			RelationType: CorpusRelationSupersedes,
			FromTitle:    "Use legacy checklist",
			ToTitle:      "Use new checklist",
		}},
	}, atoms)
	if metrics.TruePositiveCount != 0 || metrics.FalsePositiveCount != 1 || metrics.FalseNegativeCount != 1 {
		t.Fatalf("reversed supersedes should be FP/FN, got %+v", metrics)
	}
}

func TestCorpusGraphWritesArtifacts(t *testing.T) {
	root := writeCorpusGraphFixture(t, "run-a")
	summary, atoms, relations, reviews, err := BuildCorpusGraph(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatalf("build corpus graph: %v", err)
	}
	out := t.TempDir()
	if err := WriteCorpusGraph(out, summary, atoms, relations, reviews); err != nil {
		t.Fatalf("write corpus graph: %v", err)
	}
	for _, relative := range []string{"graph-summary.json", "graph-report.md"} {
		if _, err := os.Stat(filepath.Join(out, CorpusGraphDirName, relative)); err != nil {
			t.Fatalf("missing %s: %v", relative, err)
		}
	}
}

func TestCorpusGraphSkipsMissingSemanticArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.md"), []byte("# Source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(root, "manifest.json"), CorpusGraphManifest{
		SchemaVersion: CorpusGraphManifestSchemaVersion,
		CorpusID:      "corpus-missing",
		Sources: []CorpusGraphManifestSource{{
			SourceID:       "source-missing",
			SourceKind:     SourceKindMarkdown,
			Path:           "source.md",
			SemanticRunDir: "missing-semantic",
		}},
	})
	summary, atoms, relations, _, err := BuildCorpusGraph(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatalf("missing semantic artifacts should skip, not fail: %v", err)
	}
	if summary.SkippedSourceCount != 1 || len(summary.Blockers) != 1 || len(atoms) != 0 || len(relations) != 0 {
		t.Fatalf("unexpected missing artifact summary: %+v atoms=%d relations=%d", summary, len(atoms), len(relations))
	}
}

func TestCorpusGraphRejectsEscapedSemanticCandidatePath(t *testing.T) {
	root := t.TempDir()
	semanticRoot := filepath.Join(root, "semantic", "semantic-candidates")
	if err := os.MkdirAll(semanticRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.md"), []byte("# Source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(semanticRoot, "semantic-summary.json"), SemanticSummary{
		SchemaVersion:  SemanticSummarySchemaVersion,
		RunID:          "run-escaped",
		SourceCount:    1,
		CandidateCount: 1,
		Candidates: []SemanticSummaryCandidate{{
			CandidateID:   "cand-escaped",
			CandidateKind: SemanticCandidateKindAction,
			ReviewStatus:  ReviewStatusReady,
			Confidence:    ConfidenceHigh,
			CandidatePath: "../outside.json",
		}},
	})
	writeTestJSON(t, filepath.Join(root, "manifest.json"), CorpusGraphManifest{
		SchemaVersion: CorpusGraphManifestSchemaVersion,
		CorpusID:      "corpus-escaped",
		Sources: []CorpusGraphManifestSource{{
			SourceID:       "source-escaped",
			SourceKind:     SourceKindMarkdown,
			Path:           "source.md",
			SemanticRunDir: "semantic/semantic-candidates",
		}},
	})
	_, _, _, _, err := BuildCorpusGraph(filepath.Join(root, "manifest.json"))
	if err == nil || !strings.Contains(err.Error(), "escaped semantic run directory") {
		t.Fatalf("expected escaped artifact path error, got %v", err)
	}
}

func TestCorpusGraphRejectsInvalidAnswerKey(t *testing.T) {
	root := writeCorpusGraphFixture(t, "run-a")
	if err := os.WriteFile(filepath.Join(root, "answer-key.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err := BuildCorpusGraph(filepath.Join(root, "manifest.json"))
	if err == nil {
		t.Fatalf("expected invalid answer key error")
	}
}

func writeCorpusGraphFixture(t *testing.T, runID string) string {
	t.Helper()
	root := t.TempDir()
	semanticDir := filepath.Join(root, "semantic-a")
	candidates := []SemanticCandidate{
		corpusCandidate(runID, "dup-a", SemanticCandidateKindAction, "Publish launch note", "Publish the launch note for the gateway.", 1),
		corpusCandidate(runID, "dup-b", SemanticCandidateKindAction, "Publish launch note", "Publish the launch note for the gateway.", 5),
		corpusCandidate(runID, "contra-a", SemanticCandidateKindDecision, "Enable feature X", "Feature X is enabled for the July pilot.", 9),
		corpusCandidate(runID, "contra-b", SemanticCandidateKindDecision, "Disable feature X", "contradicts: Enable feature X; Feature X is disabled for the July pilot.", 13),
		corpusCandidate(runID, "super-a", SemanticCandidateKindRequirement, "Use legacy checklist", "Use the legacy checklist for review.", 17),
		corpusCandidate(runID, "super-b", SemanticCandidateKindRequirement, "Use new checklist", "supersedes: Use legacy checklist; Use the new checklist for review.", 21),
		corpusCandidate(runID, "topic-a", SemanticCandidateKindReference, "Gateway training plan", "Gateway training plan covers onboarding users.", 25),
		corpusCandidate(runID, "topic-b", SemanticCandidateKindReference, "Gateway training FAQ", "Gateway training FAQ covers onboarding questions.", 29),
	}
	if err := WriteSemantic(semanticDir, runID, 1, nil, candidates, nil); err != nil {
		t.Fatalf("write semantic fixture: %v", err)
	}
	manifest := CorpusGraphManifest{
		SchemaVersion: CorpusGraphManifestSchemaVersion,
		CorpusID:      "corpus-fixture",
		Sources: []CorpusGraphManifestSource{{
			SourceID:       "source-a",
			SourceKind:     SourceKindMarkdown,
			Path:           "source-a.md",
			SemanticRunDir: "semantic-a/semantic-candidates",
		}},
		AnswerKeyPath: "answer-key.json",
	}
	writeTestJSON(t, filepath.Join(root, "manifest.json"), manifest)
	if err := os.WriteFile(filepath.Join(root, "source-a.md"), []byte("# Source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestJSON(t, filepath.Join(root, "answer-key.json"), CorpusGraphAnswerKey{
		SchemaVersion: CorpusGraphAnswerKeySchemaVersion,
		Relations: []CorpusGraphAnswerRelation{
			{RelationType: CorpusRelationPossibleDuplicate, FromTitle: "Publish launch note", ToTitle: "Publish launch note"},
			{RelationType: CorpusRelationContradicts, FromTitle: "Enable feature X", ToTitle: "Disable feature X"},
			{RelationType: CorpusRelationSupersedes, FromTitle: "Use new checklist", ToTitle: "Use legacy checklist"},
		},
	})
	return root
}

func corpusCandidate(runID, seed string, kind SemanticCandidateKind, title, summary string, line int) SemanticCandidate {
	return SemanticCandidate{
		SchemaVersion:     SemanticCandidateSchemaVersion,
		CandidateID:       "cand-" + sanitizeID(seed+"-"+runID),
		RunID:             runID,
		SourceDocumentID:  "doc-source-a",
		CandidateKind:     kind,
		ReviewStatus:      ReviewStatusReady,
		Confidence:        ConfidenceHigh,
		Title:             title,
		Summary:           summary,
		EvidenceNodes:     []string{"node-" + sanitizeID(seed+"-"+runID)},
		EvidenceRanges:    []SemanticEvidenceRange{{StructureNodeID: "node-" + sanitizeID(seed+"-"+runID), LineStart: line, LineEnd: line}},
		EvidenceExcerpts:  []SemanticEvidenceExcerpt{{StructureNodeID: "node-" + sanitizeID(seed+"-"+runID), Text: title + " evidence."}},
		ObservationIDs:    []string{"obs-" + sanitizeID(seed+"-"+runID)},
		RelationIDs:       []string{"rel-" + sanitizeID(seed+"-"+runID)},
		DestinationStatus: SemanticDestinationUnresolved,
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
