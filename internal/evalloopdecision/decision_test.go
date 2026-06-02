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

func TestDecisionReadsProofPacketWithoutReadbackRef(t *testing.T) {
	root := t.TempDir()
	emptyInput := filepath.Join(root, "empty")
	if err := os.MkdirAll(emptyInput, 0o755); err != nil {
		t.Fatalf("mkdir empty input: %v", err)
	}
	proofOut := filepath.Join(root, "proof")
	proofPacket, err := evalproof.Build(emptyInput, proofOut, evalproof.Options{Claim: evalproof.ClaimSafety})
	if err != nil {
		t.Fatalf("proof build: %v", err)
	}
	if proofPacket.ReadbackSummaryRef != "" {
		t.Fatalf("expected missing-proof packet without readback ref, got %s", proofPacket.ReadbackSummaryRef)
	}

	decisionOut := filepath.Join(root, "decision")
	packet, err := Build(filepath.Join(proofOut, evalproof.DirName), decisionOut, Options{})
	if err != nil {
		t.Fatalf("decision build: %v", err)
	}
	if packet.TopImprovementTarget.Code != "missing_proof" {
		t.Fatalf("expected missing proof target, got %+v", packet.TopImprovementTarget)
	}
	if packet.ClaimStatuses.Safety != SafetyBlocked {
		t.Fatalf("expected safety blocked from missing side-effect evidence, got %s", packet.ClaimStatuses.Safety)
	}
	if packet.RerunInstruction != "run the relevant Mindline eval command to produce local trace/eval artifacts, then rerun eval proof-gate" {
		t.Fatalf("expected missing-proof rerun instruction, got %s", packet.RerunInstruction)
	}
	if packet.ReadbackSummaryRef != filepath.ToSlash(filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)) {
		t.Fatalf("expected persisted synthetic readback ref, got %s", packet.ReadbackSummaryRef)
	}
	if _, err := os.Stat(filepath.Join(decisionOut, DirName, "readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)); err != nil {
		t.Fatalf("expected persisted synthetic readback summary: %v", err)
	}
}

func TestDecisionReadsProofPacketGeneratedFromExistingReadback(t *testing.T) {
	root := t.TempDir()
	readbackOut := filepath.Join(root, "readback")
	if _, err := evalreadback.Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), readbackOut, evalreadback.Options{}); err != nil {
		t.Fatalf("readback build: %v", err)
	}
	proofOut := filepath.Join(root, "proof")
	proofPacket, err := evalproof.Build(filepath.Join(readbackOut, evalreadback.DirName), proofOut, evalproof.Options{Claim: evalproof.ClaimSafety})
	if err != nil {
		t.Fatalf("proof build: %v", err)
	}
	if proofPacket.ReadbackSummaryRef != filepath.ToSlash(filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)) {
		t.Fatalf("expected proof packet to persist readback summary ref, got %s", proofPacket.ReadbackSummaryRef)
	}
	packet, err := Build(filepath.Join(proofOut, evalproof.DirName), filepath.Join(root, "decision"), Options{})
	if err != nil {
		t.Fatalf("decision build: %v", err)
	}
	if packet.ReadbackSummaryRef == "" {
		t.Fatalf("expected readback summary ref")
	}
}

func TestDecisionPersistsBaselineAppliedExistingReadback(t *testing.T) {
	root := t.TempDir()
	currentReadback := filepath.Join(root, "current-readback")
	if _, err := evalreadback.Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), currentReadback, evalreadback.Options{}); err != nil {
		t.Fatalf("current readback: %v", err)
	}
	baselineReadback := filepath.Join(root, "baseline-readback")
	if _, err := evalreadback.Build(filepath.Join("..", "..", "testdata", "eval-readback", "baseline"), baselineReadback, evalreadback.Options{}); err != nil {
		t.Fatalf("baseline readback: %v", err)
	}

	decisionOut := filepath.Join(root, "decision")
	packet, err := Build(filepath.Join(currentReadback, evalreadback.DirName), decisionOut, Options{
		BaselineRoot: filepath.Join(baselineReadback, evalreadback.DirName),
	})
	if err != nil {
		t.Fatalf("decision build: %v", err)
	}
	expectedRef := filepath.ToSlash(filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile))
	if packet.ReadbackSummaryRef != expectedRef {
		t.Fatalf("expected persisted decision readback ref %s, got %s", expectedRef, packet.ReadbackSummaryRef)
	}
	persisted, err := evalreadback.LoadSummary(filepath.Join(decisionOut, DirName, "readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile))
	if err != nil {
		t.Fatalf("load persisted summary: %v", err)
	}
	if persisted.Comparison == nil || persisted.ImprovementStatus != "improved" {
		t.Fatalf("expected persisted baseline-applied comparison, got status=%s comparison=%+v", persisted.ImprovementStatus, persisted.Comparison)
	}
}

func TestDecisionAcceptsValueProofSummaryDirectory(t *testing.T) {
	root := t.TempDir()
	valueProofDir := filepath.Join(root, "value-proof")
	writeValueProofSummary(t, filepath.Join(valueProofDir, "value-summary.json"), 0)

	packet, err := Build(valueProofDir, filepath.Join(root, "decision"), Options{})
	if err != nil {
		t.Fatalf("decision build: %v", err)
	}
	if packet.ClaimStatuses.Safety != SafetyPass {
		t.Fatalf("expected value-proof safety pass, got %s", packet.ClaimStatuses.Safety)
	}
	if len(packet.SafeArtifactRefs) != 1 || packet.SafeArtifactRefs[0] != "value-summary.json" {
		t.Fatalf("expected value-proof summary ref, got %#v", packet.SafeArtifactRefs)
	}
}

func TestDecisionSafetyFailsWhenAnyArtifactSchemaUnsupported(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "current")
	writeValueProofSummary(t, filepath.Join(current, "value-proof", "value-summary.json"), 0)
	writeRawDecision(t, filepath.Join(current, "corpus-pressure", "trace-summary.json"), `{
		"schema_version": "corpus-pressure-trace-summary/v9",
		"corpus_fingerprint": "same",
		"command_config_fingerprint": "same-config"
	}`)

	packet, err := Build(current, filepath.Join(root, "decision"), Options{})
	if err != nil {
		t.Fatalf("decision build: %v", err)
	}
	if packet.ClaimStatuses.Safety != "fail" {
		t.Fatalf("unsupported schemas must fail safety, got %s", packet.ClaimStatuses.Safety)
	}
}

func TestDecisionPreservesFailingSafetyStatus(t *testing.T) {
	root := t.TempDir()
	valueProofDir := filepath.Join(root, "value-proof")
	writeValueProofSummary(t, filepath.Join(valueProofDir, "value-summary.json"), 1)

	packet, err := Build(valueProofDir, filepath.Join(root, "decision"), Options{})
	if err != nil {
		t.Fatalf("decision build: %v", err)
	}
	if packet.ClaimStatuses.Safety != "fail" {
		t.Fatalf("expected safety fail to be preserved, got %s", packet.ClaimStatuses.Safety)
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

func writeValueProofSummary(t *testing.T, path string, destinationWrites int) {
	t.Helper()
	writeRawDecision(t, path, `{
		"schema_version": "mindline-value-proof/v0.1",
		"corpus_id": "corpus-decision",
		"source_count": 1,
		"accounted_source_count": 1,
		"source_accounting_ratio": 1,
		"processed_source_count": 1,
		"atom_count": 2,
		"evidence_ready_atom_count": 2,
		"evidence_ready_atom_ratio": 1,
		"guardrails": {
			"network_fetches": 0,
			"hosted_telemetry_exports": 0,
			"hosted_inference_calls": 0,
			"browser_calls": 0,
			"slack_api_calls": 0,
			"destination_writes": `+strconv.Itoa(destinationWrites)+`,
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
