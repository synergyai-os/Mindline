package recallproof

import (
	"encoding/json"
	"testing"
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

func TestPhaseReceiptSupportsEverySignedPhaseWithoutPrivateOutput(t *testing.T) {
	for _, phase := range []string{"pre_live", "live", "eval", "outside_agent", "final"} {
		receipt := PhaseReceipt{SchemaVersion: PhaseReceiptSchema, Phase: phase, Binding: testBinding(), Artifact: StructuralArtifact{SchemaVersion: "proof_v1", Build: "wp48", State: "pass", Counts: map[string]int{"records": 1}, Fingerprints: map[string]string{"tree": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Tests: map[string]bool{"fixture": true}}}
		artifact, err := receipt.DecisionArtifact()
		if err != nil || artifact.State != "pass" {
			t.Fatalf("phase %s: %+v, %v", phase, artifact, err)
		}
	}
}

type acceptingVerifier struct{}

func (acceptingVerifier) VerifyStructuralReceipt(SignedStructuralReceipt) error { return nil }

type founderPort struct{ receipt FounderReviewReceipt }

func (port founderPort) FounderReviewReceipt() (FounderReviewReceipt, error) {
	return port.receipt, nil
}
