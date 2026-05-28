package documents

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildLinkEnrichmentLoopReducesLinkMissingnessWithLocalArtifacts(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "slack-source-1", "# Slack capture\n\nhttps://example.com/research\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-link-loop-test", CorpusPressureManifestSource{
		SourceID:   "slack-source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:         "https://example.com/research",
		Title:       "Research source",
		SourceName:  "Example Research",
		Description: "Requirement: review source evidence before routing.",
		Excerpt:     "Requirement: local artifacts should make link captures reviewable before destination routing.",
	})

	out := filepath.Join(root, "loop")
	summary, err := BuildLinkEnrichmentLoop(manifestPath, artifactsPath, out, LinkEnrichmentLoopOptions{})
	if err != nil {
		t.Fatalf("BuildLinkEnrichmentLoop: %v", err)
	}
	if summary.RequestSummary.URLMentionCount != 1 || summary.RequestSummary.AlreadyArtifactedCount != 1 || summary.RequestSummary.URLAccountingCoverage != 1 {
		t.Fatalf("bad request accounting: %+v", summary.RequestSummary)
	}
	if summary.RequestSummary.NonGeneralizableRuntime {
		t.Fatalf("ordinary fixture input should not be marked non-generalizable runtime")
	}
	if summary.Comparison.Verdict != LinkEnrichmentVerdictImproved {
		t.Fatalf("expected improved verdict: %+v", summary.Comparison)
	}
	if !summary.Comparison.Comparable || summary.Comparison.BaselineSourceSetFingerprint != summary.Comparison.EnrichedSourceSetFingerprint {
		t.Fatalf("expected explicit same-source-set comparability: %+v", summary.Comparison)
	}
	if !containsLinkLoopString(summary.Comparison.ComparableBasis, "local_enrichment_transform") {
		t.Fatalf("expected local enrichment comparable basis: %+v", summary.Comparison.ComparableBasis)
	}
	if summary.Comparison.MissingLinkReductionRatio != 1 || summary.Comparison.NeedsEnrichmentReductionRatio != 1 {
		t.Fatalf("expected full missingness reduction: %+v", summary.Comparison)
	}
	report := mustReadString(t, filepath.Join(out, LinkEnrichmentDirName, "comparison", "comparison-report.md"))
	for _, expected := range []string{"# Link enrichment comparison report", "missing_link_enrichment", "1 -> 0", "needs_enrichment"} {
		if !strings.Contains(report, expected) {
			t.Fatalf("missing %q in comparison report:\n%s", expected, report)
		}
	}
	preview := mustReadString(t, filepath.Join(out, "enriched-meaning", SourceMeaningPreviewDirName, "sources", "slack-source-1.md"))
	if !strings.Contains(preview, "Requirement: local artifacts should make link captures reviewable") {
		t.Fatalf("expected enriched evidence in preview:\n%s", preview)
	}
}

func containsLinkLoopString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestBuildLinkEnrichmentLoopContinuesFromCorpusPressureOutput(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "slack-source-1", "# Slack capture\n\nhttps://example.com/research\n")
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-link-loop-test", CorpusPressureManifestSource{
		SourceID:   "slack-source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root, LocalSourceEnrichmentArtifact{
		URL:         "https://example.com/research",
		Title:       "Research source",
		SourceName:  "Example Research",
		Description: "Requirement: continue from corpus pressure output.",
		Excerpt:     "Requirement: existing corpus pressure output should be enough to run link enrichment.",
	})
	pressureOut := filepath.Join(root, "pressure")
	if _, _, err := BuildCorpusPressure(manifestPath, pressureOut, CorpusPressureOptions{}); err != nil {
		t.Fatalf("BuildCorpusPressure: %v", err)
	}

	out := filepath.Join(root, "loop")
	summary, err := BuildLinkEnrichmentLoop(pressureOut, artifactsPath, out, LinkEnrichmentLoopOptions{})
	if err != nil {
		t.Fatalf("BuildLinkEnrichmentLoop: %v", err)
	}
	if !strings.Contains(summary.ResolvedManifestPath, "generated-input") {
		t.Fatalf("expected generated manifest path when continuing from pressure output: %+v", summary)
	}
	if summary.Comparison.MissingLinkReductionRatio != 1 || summary.Comparison.NeedsEnrichmentReductionRatio != 1 {
		t.Fatalf("expected full missingness reduction from pressure output: %+v", summary.Comparison)
	}
	_ = mustReadString(t, filepath.Join(out, LinkEnrichmentDirName, "generated-input", "sources", "slack-source-1", "source.md"))
}

func TestLinkEnrichmentNonGeneralizableRuntimeMarksPrivateRuntimePaths(t *testing.T) {
	if !linkEnrichmentNonGeneralizableRuntime("/private/tmp/mindline-runtime/intake") {
		t.Fatalf("expected private runtime path to be non-generalizable")
	}
	if linkEnrichmentNonGeneralizableRuntime("testdata/documents/link-enrichment-loop/corpus-pressure-manifest.json") {
		t.Fatalf("expected committed fixture path to remain generalizable")
	}
}

func TestBuildLinkEnrichmentLoopReportsPartialCoverageAndStaleArtifacts(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeSourceEnrichmentFixture(t, root, "slack-source-1", strings.Join([]string{
		"# Slack captures",
		"https://example.com/a",
		"https://example.com/b",
		"https://example.com/file.zip",
	}, "\n"))
	manifestPath := writeSourceEnrichmentManifest(t, root, "corpus-link-loop-test", CorpusPressureManifestSource{
		SourceID:   "slack-source-1",
		SourceKind: SourceKindMarkdown,
		Path:       sourcePath,
	})
	artifactsPath := writeSourceEnrichmentArtifacts(t, root,
		LocalSourceEnrichmentArtifact{URL: "https://example.com/a", Title: "A", Excerpt: "Requirement: A is enriched."},
		LocalSourceEnrichmentArtifact{URL: "https://example.com/stale", Title: "Stale", Excerpt: "Not referenced."},
	)

	out := filepath.Join(root, "loop")
	summary, err := BuildLinkEnrichmentLoop(manifestPath, artifactsPath, out, LinkEnrichmentLoopOptions{})
	if err != nil {
		t.Fatalf("BuildLinkEnrichmentLoop: %v", err)
	}
	if summary.RequestSummary.URLMentionCount != 3 || summary.RequestSummary.AlreadyArtifactedCount != 1 || summary.RequestSummary.RequestableCount != 1 || summary.RequestSummary.UnsupportedCount != 1 || summary.RequestSummary.StaleArtifactCount != 1 {
		t.Fatalf("expected partial coverage and stale artifact accounting: %+v", summary.RequestSummary)
	}
	requests := mustReadString(t, filepath.Join(out, LinkEnrichmentDirName, "requests", "link-artifact-requests.md"))
	for _, expected := range []string{"already_artifacted", "requestable", "unsupported"} {
		if !strings.Contains(requests, expected) {
			t.Fatalf("missing %q in request report:\n%s", expected, requests)
		}
	}
}

func TestBuildLinkEnrichmentComparisonBlocksHostedInferenceProof(t *testing.T) {
	baselinePressure := CorpusPressureSummary{
		CorpusID:                 "corpus-test",
		CommandConfigFingerprint: "config-test",
		CorpusFingerprint:        "corpus-before",
		SourceCount:              1,
		ReviewBurdenRatio:        0,
		Sources:                  []CorpusPressureSourceResult{{SourceID: "source-1", SourceKind: SourceKindMarkdown, State: CorpusPressureSourceProcessed}},
		Guardrails:               CorpusPressureGuardrailCounters{HostedInferenceCalls: 1},
	}
	enrichedPressure := baselinePressure
	enrichedPressure.CorpusFingerprint = "corpus-after"
	baselineMeaning := SourceMeaningPreviewSummary{
		MissingnessCounts: map[SourceMeaningPreviewMissingness]int{SourceMeaningMissingnessMissingLinkEnrichment: 1},
		RoutingHintCounts: map[SourceMeaningPreviewRoutingHint]int{SourceMeaningRoutingNeedsEnrichment: 1},
	}
	enrichedMeaning := SourceMeaningPreviewSummary{
		MissingnessCounts: map[SourceMeaningPreviewMissingness]int{SourceMeaningMissingnessNone: 1},
		RoutingHintCounts: map[SourceMeaningPreviewRoutingHint]int{SourceMeaningRoutingTolariaCandidate: 1},
	}

	comparison := buildLinkEnrichmentComparison("corpus-test", LinkArtifactRequestSummary{}, baselinePressure, enrichedPressure, baselineMeaning, enrichedMeaning, SourceEnrichmentSummary{})
	if comparison.Verdict != LinkEnrichmentVerdictBlocked || !containsLinkLoopString(comparison.ReasonCodes, "guardrail_violation") {
		t.Fatalf("expected hosted inference to block local proof verdict: %+v", comparison)
	}
}
