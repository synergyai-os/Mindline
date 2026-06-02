package evalproof

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/evalreadback"
)

func TestImprovementProofPassesWithComparableBaseline(t *testing.T) {
	out := t.TempDir()
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), out, Options{
		BaselineRoot: filepath.Join("..", "..", "testdata", "eval-readback", "baseline"),
		Claim:        ClaimImprovement,
	})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictPass || packet.ExitCode != 0 {
		t.Fatalf("expected pass, got %+v", packet)
	}
	if gateVerdict(packet, "improvement_claim") != VerdictPass {
		t.Fatalf("expected improvement gate pass: %+v", packet.MandatoryGates)
	}
	for _, rel := range []string{"proof-packet.json", "proof-report.md", "chain-capture-draft.md", filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)} {
		if _, err := os.Stat(filepath.Join(out, DirName, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	assertProofOutputSafe(t, filepath.Join(out, DirName))
}

func TestImprovementProofUsesReadbackOutputAsBaseline(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeProofPressure(t, baseline, 0.4, 0.7, "same", completeProofGuardrails())
	writeProofPressure(t, current, 0.9, 0.2, "same", completeProofGuardrails())

	baselineReadback := filepath.Join(root, "baseline-readback")
	if _, err := evalreadback.Build(baseline, baselineReadback, evalreadback.Options{}); err != nil {
		t.Fatalf("baseline readback: %v", err)
	}

	packet, err := Build(current, filepath.Join(root, "proof"), Options{
		BaselineRoot: baselineReadback,
		Claim:        ClaimImprovement,
	})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictPass || gateVerdict(packet, "improvement_claim") != VerdictPass {
		t.Fatalf("expected improvement proof to pass with readback baseline, got %+v", packet)
	}
}

func TestImprovementProofBlocksReadbackBaselineThatIsNotReplayReady(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeProofJSON(t, filepath.Join(baseline, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":            "corpus-pressure-summary/v0.1",
		"evidence_ready_atom_ratio": 0.4,
		"review_burden_ratio":       0.7,
		"corpus_fingerprint":        "same",
		"guardrails":                completeProofGuardrails(),
	})
	writeProofPressure(t, current, 0.9, 0.2, "same", completeProofGuardrails())

	baselineReadback := filepath.Join(root, "baseline-readback")
	if _, err := evalreadback.Build(baseline, baselineReadback, evalreadback.Options{}); err != nil {
		t.Fatalf("baseline readback: %v", err)
	}

	packet, err := Build(current, filepath.Join(root, "proof"), Options{
		BaselineRoot: baselineReadback,
		Claim:        ClaimImprovement,
	})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked ||
		!gateHasReason(packet, "improvement_claim", "replay_baseline_blocked") ||
		!gateHasReason(packet, "improvement_claim", "missing_command_config_fingerprint") {
		t.Fatalf("expected blocked replay baseline reasons, got %+v", packet.MandatoryGates)
	}
}

func TestSafetyProofPassesWithoutBaseline(t *testing.T) {
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), t.TempDir(), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictPass {
		t.Fatalf("expected safety pass without baseline, got %+v", packet)
	}
	if gateVerdict(packet, "improvement_claim") != "" {
		t.Fatalf("safety claim should not require improvement gate: %+v", packet.MandatoryGates)
	}
}

func TestProofPacketJSONIncludesRequiredEmptyFields(t *testing.T) {
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), t.TempDir(), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	data, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal packet: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal packet: %v", err)
	}
	for _, key := range []string{"baseline_root_label", "blocked_claims", "failed_claims", "permitted_claims"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("proof packet missing required field %q: %s", key, string(data))
		}
	}
	for _, key := range []string{"blocked_claims", "failed_claims", "permitted_claims"} {
		if _, ok := raw[key].([]any); !ok {
			t.Fatalf("proof packet field %q must serialize as an array, got %#v in %s", key, raw[key], string(data))
		}
	}
}

func TestImprovementProofBlocksWithoutBaseline(t *testing.T) {
	packet, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), t.TempDir(), Options{Claim: ClaimImprovement})
	if err != nil {
		t.Fatalf("build proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked || packet.ExitCode == 0 {
		t.Fatalf("expected blocked nonzero proof, got %+v", packet)
	}
	if !gateHasReason(packet, "improvement_claim", "missing_baseline") {
		t.Fatalf("expected missing_baseline, got %+v", packet.MandatoryGates)
	}
}

func TestProofPacketEmittedWhenArtifactsMissing(t *testing.T) {
	root := t.TempDir()
	packet, err := Build(root, filepath.Join(t.TempDir(), "proof"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("missing artifacts should produce proof packet, got error: %v", err)
	}
	if packet.Verdict != VerdictFail || !gateHasReason(packet, "artifact_presence", "missing_proof") {
		t.Fatalf("expected missing proof packet, got %+v", packet)
	}
}

func TestImprovementProofPreservesNotComparableReason(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeProofPressure(t, baseline, 0.2, 0.8, "baseline-fingerprint", completeProofGuardrails())
	writeProofPressure(t, current, 0.8, 0.3, "current-fingerprint", completeProofGuardrails())

	packet, err := Build(current, filepath.Join(root, "proof"), Options{Claim: ClaimImprovement, BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build not-comparable proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked || !gateHasReason(packet, "improvement_claim", "not_comparable") {
		t.Fatalf("expected not_comparable, got %+v", packet.MandatoryGates)
	}
}

func TestReadbackDeniesSecretLikeFingerprint(t *testing.T) {
	root := t.TempDir()
	writeProofPressure(t, root, 1, 0, "sk-proj-secret-do-not-leak", completeProofGuardrails())
	packet, err := Build(root, filepath.Join(t.TempDir(), "proof"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("secret-like input should be converted into failed proof, not leak through error: %v", err)
	}
	if packet.Verdict != VerdictFail || !gateHasReason(packet, "privacy_safe_readback", "unsafe_output") {
		t.Fatalf("expected unsafe output failure, got %+v", packet.MandatoryGates)
	}
}

func TestSafetyProofBlocksIncompleteSideEffectEvidence(t *testing.T) {
	root := t.TempDir()
	writeProofPressure(t, root, 1, 0, "same", map[string]any{
		"hosted_telemetry_exports": 0,
		"hosted_inference_calls":   0,
		"destination_writes":       0,
	})
	packet, err := Build(root, filepath.Join(t.TempDir(), "proof"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build incomplete guardrail proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked || !gateHasReason(packet, "side_effect_claim", "missing_side_effect_evidence") {
		t.Fatalf("expected missing side-effect evidence, got %+v", packet.MandatoryGates)
	}
}

func TestGeneralizationAndDEC64FailWhenClaimsBlocked(t *testing.T) {
	root := t.TempDir()
	writeProofPressure(t, root, 1, 0, "same", completeProofGuardrails())
	packet, err := Build(root, filepath.Join(t.TempDir(), "generalization"), Options{Claim: ClaimGeneralization})
	if err != nil {
		t.Fatalf("build generalization proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked || !gateHasReason(packet, "generalization_claim", "non_generalizable") {
		t.Fatalf("expected generalization blocked for private runtime, got %+v", packet)
	}
	packet, err = Build(root, filepath.Join(t.TempDir(), "dec64"), Options{Claim: ClaimDEC64})
	if err != nil {
		t.Fatalf("build dec64 proof: %v", err)
	}
	if packet.Verdict != VerdictBlocked || gateVerdict(packet, "dec64_no_human_claim") != VerdictBlocked {
		t.Fatalf("expected dec64 blocked, got %+v", packet)
	}
}

func TestProofFailsUnsupportedSchemaAndSideEffects(t *testing.T) {
	root := t.TempDir()
	writeProofJSON(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version": "corpus-pressure-summary/v9",
	})
	packet, err := Build(root, filepath.Join(t.TempDir(), "unsupported"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build unsupported proof: %v", err)
	}
	if packet.Verdict != VerdictFail || !gateHasReason(packet, "schema_supported", "unsupported_artifact") {
		t.Fatalf("expected unsupported schema fail, got %+v", packet.MandatoryGates)
	}

	sideEffectRoot := t.TempDir()
	guardrails := completeProofGuardrails()
	guardrails["destination_writes"] = 1
	writeProofPressure(t, sideEffectRoot, 1, 0, "same", guardrails)
	packet, err = Build(sideEffectRoot, filepath.Join(t.TempDir(), "side-effect"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build side-effect proof: %v", err)
	}
	if packet.Verdict != VerdictFail || !gateHasReason(packet, "side_effect_claim", "guardrail_failed") {
		t.Fatalf("expected side-effect fail, got %+v", packet.MandatoryGates)
	}
}

func TestProofLoadsExistingReadbackSummary(t *testing.T) {
	root := t.TempDir()
	readbackOut := filepath.Join(root, "readback")
	if _, err := evalreadback.Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), readbackOut, evalreadback.Options{
		BaselineRoot: filepath.Join("..", "..", "testdata", "eval-readback", "baseline"),
	}); err != nil {
		t.Fatalf("build readback: %v", err)
	}
	packet, err := Build(filepath.Join(readbackOut, evalreadback.DirName, evalreadback.ReadbackSummaryFile), filepath.Join(root, "proof"), Options{Claim: ClaimImprovement})
	if err != nil {
		t.Fatalf("build proof from readback: %v", err)
	}
	expectedRef := filepath.ToSlash(filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile))
	if packet.Verdict != VerdictPass || packet.ReadbackSummaryRef != expectedRef {
		t.Fatalf("unexpected proof from readback: %+v", packet)
	}
	if _, err := os.Stat(filepath.Join(root, "proof", DirName, "readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)); err != nil {
		t.Fatalf("expected proof-local readback summary copy: %v", err)
	}
}

func TestProofPreservesNestedExistingReadbackSummaryRef(t *testing.T) {
	root := t.TempDir()
	readbackOut := filepath.Join(root, "run")
	if _, err := evalreadback.Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), readbackOut, evalreadback.Options{
		BaselineRoot: filepath.Join("..", "..", "testdata", "eval-readback", "baseline"),
	}); err != nil {
		t.Fatalf("build readback: %v", err)
	}
	packet, err := Build(readbackOut, filepath.Join(root, "proof"), Options{Claim: ClaimImprovement})
	if err != nil {
		t.Fatalf("build proof from nested readback: %v", err)
	}
	expectedRef := filepath.ToSlash(filepath.Join("readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile))
	if packet.Verdict != VerdictPass || packet.ReadbackSummaryRef != expectedRef {
		t.Fatalf("unexpected nested proof ref: %+v", packet)
	}
	if _, err := os.Stat(filepath.Join(root, "proof", DirName, "readback", evalreadback.DirName, evalreadback.ReadbackSummaryFile)); err != nil {
		t.Fatalf("expected nested proof-local readback summary copy: %v", err)
	}
}

func TestProofAppliesBaselineToExistingReadbackSummary(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	readbackOut := filepath.Join(root, "readback")
	writeProofPressure(t, baseline, 0.2, 0.8, "same", completeProofGuardrails())
	writeProofPressure(t, current, 0.8, 0.3, "same", completeProofGuardrails())
	if _, err := evalreadback.Build(current, readbackOut, evalreadback.Options{}); err != nil {
		t.Fatalf("build readback without baseline: %v", err)
	}

	packet, err := Build(filepath.Join(readbackOut, evalreadback.DirName, evalreadback.ReadbackSummaryFile), filepath.Join(root, "proof"), Options{
		Claim:        ClaimImprovement,
		BaselineRoot: baseline,
	})
	if err != nil {
		t.Fatalf("build proof from readback with supplied baseline: %v", err)
	}
	if packet.Verdict != VerdictPass || gateVerdict(packet, "improvement_claim") != VerdictPass {
		t.Fatalf("expected supplied baseline to produce improvement proof, got %+v", packet)
	}
}

func TestProofReevaluatesCachedReadbackSummary(t *testing.T) {
	root := t.TempDir()
	run := filepath.Join(root, "run")
	writeProofPressure(t, run, 1, 0, "same", map[string]any{
		"hosted_telemetry_exports": 0,
		"hosted_inference_calls":   0,
		"destination_writes":       0,
	})
	summary, err := evalreadback.BuildSummary(run, evalreadback.Options{})
	if err != nil {
		t.Fatalf("build stale source summary: %v", err)
	}
	for i := range summary.ClaimGates {
		if summary.ClaimGates[i].Gate == "side_effect_claim" {
			summary.ClaimGates[i].Status = "pass"
			summary.ClaimGates[i].ReasonCodes = nil
		}
	}
	writeProofSummary(t, filepath.Join(run, evalreadback.DirName, evalreadback.ReadbackSummaryFile), summary)

	packet, err := Build(run, filepath.Join(root, "proof"), Options{Claim: ClaimSafety})
	if err != nil {
		t.Fatalf("build proof from stale readback: %v", err)
	}
	if packet.Verdict != VerdictBlocked || !gateHasReason(packet, "side_effect_claim", "missing_side_effect_evidence") {
		t.Fatalf("expected cached gates to be re-evaluated, got %+v", packet.MandatoryGates)
	}
}

func gateVerdict(packet Packet, gate string) string {
	for _, result := range packet.MandatoryGates {
		if result.Gate == gate {
			return result.Verdict
		}
	}
	return ""
}

func gateHasReason(packet Packet, gate string, reason string) bool {
	for _, result := range packet.MandatoryGates {
		if result.Gate != gate {
			continue
		}
		for _, actual := range result.ReasonCodes {
			if actual == reason {
				return true
			}
		}
	}
	return false
}

func writeProofPressure(t *testing.T, root string, evidenceReady, reviewBurden float64, fingerprint string, guardrails map[string]any) {
	t.Helper()
	writeProofJSON(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"evidence_ready_atom_ratio":  evidenceReady,
		"review_burden_ratio":        reviewBurden,
		"corpus_fingerprint":         fingerprint,
		"command_config_fingerprint": "same-config",
		"guardrails":                 guardrails,
	})
}

func writeProofJSON(t *testing.T, path string, payload map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func writeProofSummary(t *testing.T, path string, summary evalreadback.Summary) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
}

func completeProofGuardrails() map[string]any {
	return map[string]any{
		"network_fetches":             0,
		"hosted_telemetry_exports":    0,
		"hosted_inference_calls":      0,
		"browser_calls":               0,
		"slack_api_calls":             0,
		"destination_writes":          0,
		"product_brain_writes":        0,
		"tolaria_writes":              0,
		"auto_accepts":                0,
		"no_human_claims":             0,
		"committed_private_artifacts": 0,
	}
}

func assertProofOutputSafe(t *testing.T, root string) {
	t.Helper()
	denied := []string{"/private/tmp/", "/Users/", "Young Human Club Dropbox", "slack.com/archives/", "sk-proj-", "OPENAI_API_KEY"}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, pattern := range denied {
			if strings.Contains(string(data), pattern) {
				t.Fatalf("%s leaked denied pattern %q", path, pattern)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk proof output: %v", err)
	}
}
