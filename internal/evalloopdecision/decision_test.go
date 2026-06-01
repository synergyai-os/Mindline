package evalloopdecision

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/synergyai-os/Mindline/internal/evalproof"
	"github.com/synergyai-os/Mindline/internal/evalreadback"
)

func TestDecisionCurrentOnlyBlocksImprovementAndNamesOneTarget(t *testing.T) {
	out := t.TempDir()
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), out, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if packet.ImprovementState != ImprovementBlockedMissingBaseline {
		t.Fatalf("expected blocked missing baseline, got %s", packet.ImprovementState)
	}
	if packet.TopImprovementTarget.Code == "" {
		t.Fatalf("expected one top improvement target")
	}
	if packet.TopImprovementTarget.Code != "establish_comparable_baseline" {
		t.Fatalf("expected comparable baseline target, got %s", packet.TopImprovementTarget.Code)
	}
	if packet.RerunInstruction == "" {
		t.Fatalf("expected rerun instruction")
	}
	if packet.ClaimStatuses.DEC64 == "pass" {
		t.Fatalf("DEC-64 must remain blocked without held-out proof")
	}
	for _, rel := range []string{"decision-packet.json", "decision-report.md", "chain-capture-draft.md"} {
		if _, err := os.Stat(filepath.Join(out, DirName, rel)); err != nil {
			t.Fatalf("expected %s: %v", rel, err)
		}
	}
}

func TestDecisionReportsImprovedForComparableBaseline(t *testing.T) {
	out := t.TempDir()
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), out, Options{
		BaselineRoot: filepath.Join("..", "..", "testdata", "eval-readback", "baseline"),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if packet.ImprovementState != ImprovementImproved {
		t.Fatalf("expected improved, got %s", packet.ImprovementState)
	}
	if packet.Comparison == nil || packet.Comparison.Status != "improved" {
		t.Fatalf("expected improved comparison, got %#v", packet.Comparison)
	}
}

func TestDecisionReadsProofPacketInput(t *testing.T) {
	root := t.TempDir()
	proofOut := filepath.Join(root, "proof")
	if _, err := evalproof.Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), proofOut, evalproof.Options{Claim: evalproof.ClaimSafety}); err != nil {
		t.Fatalf("proof build: %v", err)
	}
	packet, err := Build(filepath.Join(proofOut, evalproof.DirName), filepath.Join(root, "decision"), Options{})
	if err != nil {
		t.Fatalf("decision build: %v", err)
	}
	if packet.ImprovementState != ImprovementBlockedMissingBaseline {
		t.Fatalf("expected missing baseline from proof input, got %s", packet.ImprovementState)
	}
	if packet.ReadbackSummaryRef == "" {
		t.Fatalf("expected readback summary ref")
	}
}

func TestDecisionAcceptsReadbackSummaryBaseline(t *testing.T) {
	root := t.TempDir()
	baselineReadback := filepath.Join(root, "baseline-readback")
	if _, err := evalreadback.Build(filepath.Join("..", "..", "testdata", "eval-readback", "baseline"), baselineReadback, evalreadback.Options{}); err != nil {
		t.Fatalf("baseline readback: %v", err)
	}
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), filepath.Join(root, "decision"), Options{
		BaselineRoot: filepath.Join(baselineReadback, evalreadback.DirName),
	})
	if err != nil {
		t.Fatalf("decision build: %v", err)
	}
	if packet.Comparison == nil || packet.ImprovementState != ImprovementImproved {
		t.Fatalf("expected comparison from readback baseline, got state=%s comparison=%#v", packet.ImprovementState, packet.Comparison)
	}
}

func TestDecisionAcceptsProofPacketBaseline(t *testing.T) {
	root := t.TempDir()
	baselineProof := filepath.Join(root, "baseline-proof")
	if _, err := evalproof.Build(filepath.Join("..", "..", "testdata", "eval-readback", "baseline"), baselineProof, evalproof.Options{Claim: evalproof.ClaimSafety}); err != nil {
		t.Fatalf("baseline proof: %v", err)
	}
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), filepath.Join(root, "decision"), Options{
		BaselineRoot: filepath.Join(baselineProof, evalproof.DirName),
	})
	if err != nil {
		t.Fatalf("decision build: %v", err)
	}
	if packet.Comparison == nil || packet.ImprovementState != ImprovementImproved {
		t.Fatalf("expected comparison from proof baseline, got state=%s comparison=%#v", packet.ImprovementState, packet.Comparison)
	}
}

func TestDecisionDoesNotClaimImprovementWhenReadbackBlocksImprovementClaim(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeDecisionPressure(t, baseline, 0.4, 0.7)
	writeDecisionPressure(t, current, 0.9, 0.2)
	writeRawDecision(t, filepath.Join(current, "link-enrichment", "loop-summary.json"), `{"schema_version":"link-enrichment-loop-summary/v0.1","input_path":"/private/tmp/source.json"}`)

	packet, err := Build(current, filepath.Join(root, "decision"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("decision build: %v", err)
	}
	if packet.ImprovementState == ImprovementImproved || packet.ClaimStatuses.Improvement == ImprovementImproved {
		t.Fatalf("must not claim improvement when readback blocks improvement claim: %#v", packet)
	}
	if packet.ImprovementState != ImprovementInconclusive {
		t.Fatalf("expected inconclusive blocked claim, got %s", packet.ImprovementState)
	}
}

func TestDecisionRejectsUnsafeOutput(t *testing.T) {
	out := t.TempDir()
	packet := Packet{
		SchemaVersion: PacketSchemaVersion,
		RunID:         "run-test",
		TopImprovementTarget: evalreadback.ImprovementTarget{
			Code:      "unsafe",
			Rationale: "/Users/private/source.md",
		},
	}
	if err := Write(out, packet, nil); err == nil {
		t.Fatalf("expected unsafe output rejection")
	}
}

func writeDecisionPressure(t *testing.T, root string, evidenceReady, reviewBurden float64) {
	t.Helper()
	target := filepath.Join(root, "corpus-pressure", "pressure-summary.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeRawDecision(t, target, `{
		"schema_version": "corpus-pressure-summary/v0.1",
		"corpus_id": "corpus-decision",
		"source_count": 1,
		"processed_source_count": 1,
		"evidence_ready_atom_ratio": `+floatString(evidenceReady)+`,
		"review_burden_ratio": `+floatString(reviewBurden)+`,
		"corpus_fingerprint": "same",
		"command_config_fingerprint": "same-config",
		"guardrails": {
			"network_fetches": 0,
			"hosted_telemetry_exports": 0,
			"hosted_inference_calls": 0,
			"browser_calls": 0,
			"slack_api_calls": 0,
			"destination_writes": 0,
			"product_brain_writes": 0,
			"tolaria_writes": 0,
			"auto_accepts": 0,
			"no_human_claims": 0,
			"committed_private_artifacts": 0
		}
	}`)
}

func writeRawDecision(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func floatString(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
