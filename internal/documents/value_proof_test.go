package documents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValueProofBuildsMixedSourcePacket(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "value-proof-run")
	summary, err := BuildValueProof(filepath.Join("..", "..", "testdata", "documents", "value-proof", "corpus-pressure-manifest.json"), out, ValueProofOptions{})
	if err != nil {
		t.Fatalf("build value proof: %v", err)
	}
	if summary.SchemaVersion != ValueProofSchemaVersion {
		t.Fatalf("unexpected schema version %s", summary.SchemaVersion)
	}
	if summary.SourceCount != 4 || summary.AccountedSourceCount != 4 || summary.SourceAccountingRatio != 1 {
		t.Fatalf("unexpected source accounting: %+v", summary)
	}
	if summary.AtomCount == 0 || summary.EvidenceOrBlockerCount != summary.AtomCount || summary.EvidenceOrBlockerRatio != 1 {
		t.Fatalf("expected every atom to have evidence or blocker: %+v", summary)
	}
	if summary.RelationCount == 0 {
		t.Fatalf("expected graph-backed relation context")
	}
	if summary.GraphSummaryPath != filepath.ToSlash(filepath.Join(CorpusGraphDirName, "graph-summary.json")) {
		t.Fatalf("expected corpus graph summary ref, got %s", summary.GraphSummaryPath)
	}
	if summary.Guardrails.DestinationWrites != 0 || summary.Guardrails.ProductBrainWrites != 0 || summary.Guardrails.TolariaWrites != 0 || summary.Guardrails.HostedTelemetryExports != 0 || summary.Guardrails.HostedInferenceCalls != 0 {
		t.Fatalf("expected zero side effects: %+v", summary.Guardrails)
	}
	if summary.ClaimStatuses.Improvement != "blocked_missing_comparable_baseline" || summary.ClaimStatuses.DEC64 == "" {
		t.Fatalf("expected blocked non-safety claims: %+v", summary.ClaimStatuses)
	}

	localPacket := mustReadString(t, filepath.Join(out, ValueProofDirName, "local-value-packet.md"))
	if !strings.Contains(localPacket, "Evidence highlights") || !strings.Contains(localPacket, "Evidence:") || !strings.Contains(localPacket, SourceMeaningPreviewDirName) {
		t.Fatalf("local packet does not point to evidence previews:\n%s", localPacket)
	}
	prSafe := mustReadString(t, filepath.Join(out, ValueProofDirName, "pr-safe-summary.md"))
	if strings.Contains(prSafe, "Action:") || strings.Contains(prSafe, "Decision:") || strings.Contains(prSafe, "Young Human Club Dropbox") || strings.Contains(prSafe, "/private/tmp") {
		t.Fatalf("PR-safe summary leaked raw/private content:\n%s", prSafe)
	}
}

func TestValueProofSkipsGeneratedArtifactsWhenOutputOverlapsDirectoryInput(t *testing.T) {
	root := t.TempDir()
	if err := copyValueProofFixture(root); err != nil {
		t.Fatal(err)
	}
	summary, err := BuildValueProof(root, root, ValueProofOptions{})
	if err != nil {
		t.Fatalf("first build value proof: %v", err)
	}
	if summary.SourceCount != 4 {
		t.Fatalf("expected original source count, got %d", summary.SourceCount)
	}
	summary, err = BuildValueProof(root, root, ValueProofOptions{})
	if err != nil {
		t.Fatalf("second build value proof: %v", err)
	}
	if summary.SourceCount != 4 {
		t.Fatalf("generated artifact Markdown was re-ingested, source count=%d", summary.SourceCount)
	}
}

func TestValueProofReadsGeneratedPressureWhenOutputBasenameIsCorpusPressure(t *testing.T) {
	out := filepath.Join(t.TempDir(), CorpusPressureDirName)
	if _, err := BuildValueProof(filepath.Join("..", "..", "testdata", "documents", "value-proof", "corpus-pressure-manifest.json"), out, ValueProofOptions{}); err != nil {
		t.Fatalf("build value proof with corpus-pressure output basename: %v", err)
	}
}

func TestValueProofCountsEvidenceOrBlockerAtomsWithoutFloatTruncation(t *testing.T) {
	pressure := CorpusPressureSummary{SourceCount: 1, ProcessedSourceCount: 1}
	graph := CorpusGraphSummary{AtomCount: 22}
	meaning := SourceMeaningPreviewSummary{EvidenceCoverageRatio: 15.0 / 22.0}
	summary := buildValueProofSummary(pressure, graph, meaning)
	if summary.EvidenceOrBlockerCount != 15 {
		t.Fatalf("expected integer evidence count 15, got %d", summary.EvidenceOrBlockerCount)
	}
}

func TestValueProofPRSafeSummaryRedactsSourceIdentifiers(t *testing.T) {
	summary := ValueProofSummary{
		SchemaVersion:         ValueProofSchemaVersion,
		CorpusID:              "corpus-private",
		SourceCount:           1,
		AccountedSourceCount:  1,
		SourceAccountingRatio: 1,
		Sources: []ValueProofSourceSummary{{
			SourceID:      "customer-alpha/slack-product-plan",
			SourceKind:    SourceKindMarkdown,
			State:         CorpusPressureSourceProcessed,
			ReasonCode:    CorpusPressureReasonNone,
			AtomCount:     2,
			RelationCount: 1,
		}},
	}
	markdown := valueProofPRSafeMarkdown(summary)
	if strings.Contains(markdown, "customer-alpha") || strings.Contains(markdown, "slack-product-plan") {
		t.Fatalf("PR-safe summary leaked source identifier:\n%s", markdown)
	}
	if !strings.Contains(markdown, "source_ref=") {
		t.Fatalf("PR-safe summary missing stable redacted source ref:\n%s", markdown)
	}
}

func copyValueProofFixture(target string) error {
	sourceRoot := filepath.Join("..", "..", "testdata", "documents", "value-proof")
	return filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
}
