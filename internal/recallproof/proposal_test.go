package recallproof

import "testing"

func TestWP48ProofGroupProposalsAreOrderedAndBound(t *testing.T) {
	groups := WP48ProofGroupProposals()
	if err := ValidateGroupProposals(groups); err != nil {
		t.Fatalf("ValidateGroupProposals: %v", err)
	}
	if groups[len(groups)-1].ID != "wp48_final_revalidation" {
		t.Fatalf("final group = %s", groups[len(groups)-1].ID)
	}
}

func TestRequireExactBindingRejectsDrift(t *testing.T) {
	binding := testBinding()
	if err := RequireExactBinding(binding, binding); err != nil {
		t.Fatalf("RequireExactBinding identical: %v", err)
	}
	drift := binding
	drift.BinaryFingerprint = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := RequireExactBinding(binding, drift); err == nil {
		t.Fatal("RequireExactBinding accepted binary drift")
	}
}

func testBinding() TreeConfigBinding {
	return TreeConfigBinding{
		SchemaVersion:                BindingSchemaVersion,
		Commit:                       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TreeFingerprint:              "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BinaryFingerprint:            "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AssuranceManifestFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		LiveConfigurationFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
}
