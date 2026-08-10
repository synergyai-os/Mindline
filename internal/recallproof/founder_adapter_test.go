package recallproof

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/founderreview"
)

func TestUsefulFounderOutcomeMustComeFromMatchingDurableRecord(t *testing.T) {
	repository, err := founderreview.NewRepository(
		filepath.Join(t.TempDir(), "runtime"), founderreview.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	proofRunID := strings.Repeat("A", 43)
	proofFingerprint := "sha256:" + strings.Repeat("a", 64)
	citationsFingerprint := "sha256:" + strings.Repeat("b", 64)
	if _, err := repository.Create(context.Background(), founderreview.Request{
		ProofRunID: proofRunID, StructuralProofFingerprint: proofFingerprint,
		CitedRecordsFingerprint: citationsFingerprint,
		Verdict:                 founderreview.VerdictUseful,
		RetryToken:              strings.Repeat("r", 16),
	}); err != nil {
		t.Fatal(err)
	}
	binding := TreeConfigBinding{
		SchemaVersion:                BindingSchemaVersion,
		Commit:                       strings.Repeat("a", 40),
		TreeFingerprint:              "sha256:" + strings.Repeat("b", 64),
		BinaryFingerprint:            "sha256:" + strings.Repeat("c", 64),
		AssuranceManifestFingerprint: "sha256:" + strings.Repeat("d", 64),
		LiveConfigurationFingerprint: "sha256:" + strings.Repeat("e", 64),
	}
	adapter := DurableFounderReviewAdapter{
		Repository: repository, Binding: binding,
		ExpectedProofRunID:                 proofRunID,
		ExpectedStructuralProofFingerprint: proofFingerprint,
		ExpectedCitedRecordsFingerprint:    citationsFingerprint,
	}
	if err := RequireUsefulFounderOutcome(adapter, binding); err != nil {
		t.Fatal(err)
	}
	adapter.ExpectedCitedRecordsFingerprint = "sha256:" + strings.Repeat("c", 64)
	if err := RequireUsefulFounderOutcome(adapter, binding); err == nil {
		t.Fatal("mismatched durable founder review closed user value")
	}
}
