package recallproof

import (
	"context"
	"errors"

	"github.com/synergyai-os/Mindline/internal/founderreview"
)

// DurableFounderReviewAdapter prevents a caller from asserting a founder
// outcome directly. It loads the immutable owner-only record and requires its
// proof run, structural proof, and cited-record commitments to match the final
// proof being closed.
type DurableFounderReviewAdapter struct {
	Repository                         founderreview.RepositoryPort
	Binding                            TreeConfigBinding
	ExpectedProofRunID                 string
	ExpectedStructuralProofFingerprint string
	ExpectedCitedRecordsFingerprint    string
}

func (adapter DurableFounderReviewAdapter) FounderReviewReceipt() (FounderReviewReceipt, error) {
	if adapter.Repository == nil {
		return FounderReviewReceipt{}, errors.New("durable founder review repository is required")
	}
	if err := adapter.Binding.Validate(); err != nil {
		return FounderReviewReceipt{}, err
	}
	record, err := adapter.Repository.Load(context.Background())
	if err != nil {
		return FounderReviewReceipt{}, err
	}
	if record.ProofRunID != adapter.ExpectedProofRunID ||
		record.StructuralProofFingerprint != adapter.ExpectedStructuralProofFingerprint ||
		record.CitedRecordsFingerprint != adapter.ExpectedCitedRecordsFingerprint {
		return FounderReviewReceipt{}, errors.New("durable founder review does not bind the final proof")
	}
	receipt := record.Receipt()
	return FounderReviewReceipt{
		SchemaVersion: FounderReviewReceiptSchema,
		Binding:       adapter.Binding, Outcome: string(receipt.Verdict),
		ReviewFingerprint: receipt.EventID,
	}, nil
}
