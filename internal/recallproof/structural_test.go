package recallproof

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/assurance"
)

func TestDecodeStructuralArtifactRejectsPrivateOrUnknownFields(t *testing.T) {
	valid := []byte(`{"schema_version":"proof_v1","build":"wp48","state":"pass","counts":{"records":1},"fingerprints":{"tree":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"tests":{"fixture":true}}`)
	if _, err := DecodeStructuralArtifact(valid); err != nil {
		t.Fatalf("DecodeStructuralArtifact: %v", err)
	}
	private := []byte(`{"schema_version":"proof_v1","build":"wp48","state":"pass","counts":{"records":1},"fingerprints":{"tree":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"tests":{"fixture":true},"query":"private"}`)
	if _, err := DecodeStructuralArtifact(private); err == nil {
		t.Fatal("DecodeStructuralArtifact accepted private field")
	}
}

func TestDecodeStructuralArtifactRejectsFailedTestInPassingArtifact(t *testing.T) {
	data := []byte(`{"schema_version":"proof_v1","build":"wp48","state":"pass","counts":{"records":1},"fingerprints":{"tree":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"tests":{"fixture":false}}`)
	if _, err := DecodeStructuralArtifact(data); err == nil {
		t.Fatal("passing structural artifact accepted a failed test")
	}
}

func TestLifecycleRunnerRequiresVerifiedReceiptsAndUsefulFounderOutcome(t *testing.T) {
	binding := testBinding()
	receipts := make([]SignedStructuralReceipt, 0, len(requiredLifecycleReceipts))
	for _, kind := range requiredLifecycleReceipts {
		receipts = append(receipts, SignedStructuralReceipt{SchemaVersion: "mindline-signed-structural-receipt/v0.1", Kind: kind, Binding: binding, ArtifactFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SignerFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Signature: "owner-verified"})
	}
	artifact, err := (LifecycleRunner{Verifier: acceptingVerifier{}}).Verify(binding, receipts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DeterministicArtifactFingerprint(artifact); err != nil {
		t.Fatal(err)
	}
	if err := RequireUsefulFounderOutcome(founderPort{receipt: FounderReviewReceipt{SchemaVersion: FounderReviewReceiptSchema, Binding: binding, Outcome: "useful", ReviewFingerprint: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}}, binding); err != nil {
		t.Fatal(err)
	}
	if err := RequireUsefulFounderOutcome(founderPort{receipt: FounderReviewReceipt{SchemaVersion: FounderReviewReceiptSchema, Binding: binding, Outcome: "declined", ReviewFingerprint: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}}, binding); err == nil {
		t.Fatal("declined founder outcome closed the proof")
	}

	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeStructuralArtifact(data); err != nil {
		t.Fatal(err)
	}
}

func TestPhaseReceiptAcceptsOnlyExactPreLiveContract(t *testing.T) {
	receipt := PhaseReceipt{SchemaVersion: PhaseReceiptSchema, Phase: "pre_live", Binding: testBinding(), Artifact: validPreLiveArtifact(t)}
	artifact, err := receipt.DecisionArtifact()
	if err != nil || artifact.State != "pass" {
		t.Fatalf("exact pre-live: %+v, %v", artifact, err)
	}
	for _, phase := range []string{"live", "eval", "outside_agent", "final"} {
		receipt.Phase = phase
		if _, err := receipt.DecisionArtifact(); err == nil {
			t.Fatalf("unsupported phase %s accepted reusable pre-live artifact", phase)
		}
	}
	receipt.Phase = "pre_live"
	receipt.Artifact.Tests["unapproved"] = true
	receipt.Artifact.Fingerprints["unapproved"] = "sha256:" + strings.Repeat("b", 64)
	if _, err := receipt.DecisionArtifact(); err == nil {
		t.Fatal("pre-live artifact with extra proof group was accepted")
	}
}

func validPreLiveArtifact(t *testing.T) StructuralArtifact {
	t.Helper()
	manifest, err := assurance.ParseWP48Manifest(assurance.EmbeddedWP48Manifest())
	if err != nil {
		t.Fatal(err)
	}
	fingerprints := map[string]string{}
	tests := map[string]bool{}
	for _, group := range manifest.Groups {
		if group.Phase == "pre_live" {
			fingerprints[group.ID] = "sha256:" + strings.Repeat("a", 64)
			tests[group.ID] = true
		}
	}
	return StructuralArtifact{
		SchemaVersion: "mindline-reusable-proof/v0.1", Build: "wp48", State: "pass",
		Counts:       map[string]int{"executed_pre_live_groups": len(tests)},
		Fingerprints: fingerprints, Tests: tests,
	}
}

type acceptingVerifier struct{}

func (acceptingVerifier) VerifyStructuralReceipt(SignedStructuralReceipt) error { return nil }

type founderPort struct{ receipt FounderReviewReceipt }

func (port founderPort) FounderReviewReceipt() (FounderReviewReceipt, error) {
	return port.receipt, nil
}
