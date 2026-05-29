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
	writeProofPressure(t, root, 1, 0, "sk-test-secret-do-not-leak", completeProofGuardrails())
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
	if packet.Verdict != VerdictPass || packet.ReadbackSummaryRef != "input/readback-summary.json" {
		t.Fatalf("unexpected proof from readback: %+v", packet)
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
	denied := []string{"/private/tmp/", "/Users/", "Young Human Club Dropbox", "slack.com/archives/", "sk-", "OPENAI_API_KEY"}
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
