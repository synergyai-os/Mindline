package recalleval

import (
	"context"
	"errors"
)

const DraftManifestSchemaVersion = "mindline-retrieval-eval-draft/v0.1"

// DraftManifest is owner-only reviewer input created before either evaluated
// run. Record IDs are converted to canonical commitments before sealing.
type DraftManifest struct {
	SchemaVersion       string      `json:"schema_version"`
	LibraryFingerprint  string      `json:"library_fingerprint"`
	Baseline            RunBinding  `json:"baseline"`
	ReviewerFingerprint string      `json:"reviewer_fingerprint"`
	Cases               []DraftCase `json:"cases"`
}

type DraftCase struct {
	CaseID            string   `json:"case_id"`
	Kind              string   `json:"kind"`
	Query             string   `json:"query"`
	ExpectedRecordIDs []string `json:"expected_record_ids"`
}

func SealOwnerManifest(ctx context.Context, draft DraftManifest, canonical CanonicalEvidencePort) (OwnerManifest, error) {
	if draft.SchemaVersion != DraftManifestSchemaVersion || canonical == nil {
		return OwnerManifest{}, errors.New("retrieval evaluation draft is incomplete")
	}
	manifest := OwnerManifest{
		SchemaVersion: ManifestSchemaVersion, LibraryFingerprint: draft.LibraryFingerprint,
		Baseline: draft.Baseline, ReviewerFingerprint: draft.ReviewerFingerprint,
		LabelsFrozenBeforeRun: true,
	}
	for _, item := range draft.Cases {
		sealed := OwnerCase{CaseID: item.CaseID, Kind: item.Kind, Query: item.Query}
		if item.Kind == CaseNoAnswer && len(item.ExpectedRecordIDs) != 0 {
			return OwnerManifest{}, errors.New("no-answer draft case has expected records")
		}
		for _, recordID := range item.ExpectedRecordIDs {
			evidence, err := canonical.GetCanonicalEvidence(ctx, recordID)
			if err != nil {
				return OwnerManifest{}, err
			}
			commitment, err := CanonicalEvidenceCommitment(evidence)
			if err != nil {
				return OwnerManifest{}, err
			}
			sealed.ExpectedCanonicalCommitments = append(sealed.ExpectedCanonicalCommitments, commitment)
		}
		manifest.Cases = append(manifest.Cases, sealed)
	}
	if err := validateOwnerManifestStructure(manifest); err != nil {
		return OwnerManifest{}, err
	}
	fingerprint, err := OwnerManifestFingerprint(manifest)
	if err != nil {
		return OwnerManifest{}, err
	}
	manifest.Fingerprint = fingerprint
	return manifest, manifest.Validate()
}
