package documents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceMeaningPreviewBuildsReadableRoutingProof(t *testing.T) {
	root := t.TempDir()
	pressureOut := filepath.Join(root, "pressure")
	summary, _, err := BuildCorpusPressure(fixturePath(t, "markdown"), pressureOut, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	if summary.ProcessedSourceCount == 0 {
		t.Fatalf("expected processed sources")
	}

	previewOut := filepath.Join(root, "meaning")
	meaning, items, err := BuildSourceMeaningPreview(pressureOut, previewOut)
	if err != nil {
		t.Fatalf("build source meaning preview: %v", err)
	}

	if meaning.PreviewedSourceCount != summary.ProcessedSourceCount {
		t.Fatalf("expected previewed source count %d, got %d", summary.ProcessedSourceCount, meaning.PreviewedSourceCount)
	}
	if meaning.SkippedSourceCount != summary.SkippedSourceCount || meaning.BlockedSourceCount != summary.BlockedSourceCount || meaning.ExcludedSourceCount != summary.ExcludedSourceCount {
		t.Fatalf("expected source state counts to match pressure summary: %+v vs %+v", meaning, summary)
	}
	if meaning.PreviewCoverageRatio != 1 || meaning.EvidenceCoverageRatio != 1 || meaning.RoutingCoverageRatio != 1 {
		t.Fatalf("expected full coverage, got preview=%f evidence=%f routing=%f", meaning.PreviewCoverageRatio, meaning.EvidenceCoverageRatio, meaning.RoutingCoverageRatio)
	}
	if meaning.Guardrails.DestinationWrites != 0 || meaning.Guardrails.ProductBrainWrites != 0 || meaning.Guardrails.TolariaWrites != 0 {
		t.Fatalf("expected zero write guardrails: %+v", meaning.Guardrails)
	}
	if meaning.RoutingHintCounts[SourceMeaningRoutingProductBrainCandidate] == 0 {
		t.Fatalf("expected at least one Product Brain routing hint")
	}
	if meaning.RoutingHintCounts[SourceMeaningRoutingTolariaCandidate] == 0 {
		t.Fatalf("expected at least one Tolaria routing hint")
	}
	if len(items) != len(summary.Sources) {
		t.Fatalf("expected one item per source")
	}

	report := mustReadString(t, filepath.Join(previewOut, SourceMeaningPreviewDirName, "meaning-report.md"))
	if !strings.Contains(report, "Preview/calibration only") || !strings.Contains(report, "Destination writes: 0") {
		t.Fatalf("report missing review/safety language:\n%s", report)
	}
	sourcePreview := mustReadString(t, filepath.Join(previewOut, SourceMeaningPreviewDirName, "sources", "mixed-thread-capture.md"))
	if !strings.Contains(sourcePreview, "Destination writes: `0`") || !strings.Contains(sourcePreview, "Product Brain writes: `0`") || !strings.Contains(sourcePreview, "Tolaria writes: `0`") {
		t.Fatalf("source preview missing per-source guardrails:\n%s", sourcePreview)
	}
}

func TestSourceMeaningPreviewRedactsUnsafeEvidence(t *testing.T) {
	root := t.TempDir()
	pressureOut := filepath.Join(root, "pressure")
	if _, _, err := BuildCorpusPressure(fixturePath(t, "markdown"), pressureOut, CorpusPressureOptions{}); err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}

	previewOut := filepath.Join(root, "meaning")
	meaning, _, err := BuildSourceMeaningPreview(pressureOut, previewOut)
	if err != nil {
		t.Fatalf("build source meaning preview: %v", err)
	}
	if meaning.MissingnessCounts[SourceMeaningMissingnessBlockedSource] == 0 {
		t.Fatalf("expected unsafe evidence to be counted as blocked source: %+v", meaning.MissingnessCounts)
	}
	if meaning.RoutingHintCounts[SourceMeaningRoutingBlocked] == 0 {
		t.Fatalf("expected unsafe evidence to force blocked routing: %+v", meaning.RoutingHintCounts)
	}

	rooted := filepath.Join(previewOut, SourceMeaningPreviewDirName)
	entries, err := os.ReadDir(filepath.Join(rooted, "sources"))
	if err != nil {
		t.Fatalf("read source previews: %v", err)
	}
	all := mustReadString(t, filepath.Join(rooted, "meaning-report.md"))
	for _, entry := range entries {
		all += "\n" + mustReadString(t, filepath.Join(rooted, "sources", entry.Name()))
	}
	if strings.Contains(all, "PRIVATE_CONTENT") {
		t.Fatalf("preview leaked unsafe private marker:\n%s", all)
	}
	if !strings.Contains(all, "[redacted unsafe/private evidence excerpt]") {
		t.Fatalf("expected redacted evidence marker:\n%s", all)
	}
}

func TestReadSourceMeaningGraphDeduplicatesSameSourceRelations(t *testing.T) {
	root := t.TempDir()
	graphDir := filepath.Join(root, "corpus-graph")
	writeDocumentsTestJSON(t, filepath.Join(graphDir, "graph-summary.json"), CorpusGraphSummary{
		SchemaVersion: CorpusGraphSummarySchemaVersion,
		Atoms: []CorpusGraphSummaryAtom{
			{AtomID: "atom-a", SourceID: "source-1", AtomPath: "atoms/atom-a.json"},
			{AtomID: "atom-b", SourceID: "source-1", AtomPath: "atoms/atom-b.json"},
		},
		Relations: []CorpusGraphSummaryRelation{
			{RelationID: "rel-same-source", FromAtomID: "atom-a", ToAtomID: "atom-b", RelationPath: "relations/rel-same-source.json"},
		},
	})
	writeDocumentsTestJSON(t, filepath.Join(graphDir, "atoms", "atom-a.json"), CorpusGraphAtom{AtomID: "atom-a", SourceID: "source-1"})
	writeDocumentsTestJSON(t, filepath.Join(graphDir, "atoms", "atom-b.json"), CorpusGraphAtom{AtomID: "atom-b", SourceID: "source-1"})
	writeDocumentsTestJSON(t, filepath.Join(graphDir, "relations", "rel-same-source.json"), CorpusGraphRelation{
		RelationID: "rel-same-source",
		FromAtomID: "atom-a",
		ToAtomID:   "atom-b",
	})

	_, _, relationsBySource, err := readSourceMeaningGraph(root, "corpus-graph/graph-summary.json")
	if err != nil {
		t.Fatalf("read source meaning graph: %v", err)
	}
	if got := len(relationsBySource["source-1"]); got != 1 {
		t.Fatalf("same-source relation count = %d, want 1", got)
	}
}

func mustReadString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
