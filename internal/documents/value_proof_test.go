package documents

import (
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
