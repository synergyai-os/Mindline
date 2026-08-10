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
	if canonical == nil || validateDraftManifestStructure(draft) != nil {
		return OwnerManifest{}, errors.New("retrieval evaluation draft is incomplete")
	}
	manifest := OwnerManifest{
		SchemaVersion: ManifestSchemaVersion, LibraryFingerprint: draft.LibraryFingerprint,
		Baseline: draft.Baseline, ReviewerFingerprint: draft.ReviewerFingerprint,
		LabelsFrozenBeforeRun: true,
	}
	for _, item := range draft.Cases {
		sealed := OwnerCase{CaseID: item.CaseID, Kind: item.Kind, Query: item.Query}
		for _, recordID := range item.ExpectedRecordIDs {
			evidence, err := canonical.GetCanonicalEvidence(ctx, recordID)
			if err != nil {
				return OwnerManifest{}, err
			}
			if evidence.LibraryFingerprint != draft.LibraryFingerprint {
				return OwnerManifest{}, errors.New("canonical evidence changed the frozen library")
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

// validateDraftManifestStructure rejects the complete private label shape
// before any canonical evidence is hydrated. That makes invalid denominators
// side-effect free and prevents duplicate labels from being counted twice.
func validateDraftManifestStructure(draft DraftManifest) error {
	if draft.SchemaVersion != DraftManifestSchemaVersion ||
		!isFingerprint(draft.LibraryFingerprint) || !isFingerprint(draft.ReviewerFingerprint) ||
		draft.Baseline.Validate() != nil || len(draft.Cases) < 20 {
		return errors.New("retrieval evaluation draft is incomplete")
	}
	caseIDs := make(map[string]struct{}, len(draft.Cases))
	answerable, noAnswer := 0, 0
	for _, item := range draft.Cases {
		if !caseIDPattern.MatchString(item.CaseID) || item.Query == "" {
			return errors.New("retrieval evaluation draft contains an invalid case")
		}
		if _, duplicate := caseIDs[item.CaseID]; duplicate {
			return errors.New("retrieval evaluation draft repeats a case")
		}
		caseIDs[item.CaseID] = struct{}{}
		switch item.Kind {
		case CaseAnswerable:
			answerable++
			if len(item.ExpectedRecordIDs) == 0 {
				return errors.New("answerable draft case has no expected records")
			}
		case CaseNoAnswer:
			noAnswer++
			if len(item.ExpectedRecordIDs) != 0 {
				return errors.New("no-answer draft case has expected records")
			}
		default:
			return errors.New("retrieval evaluation draft has unsupported case kind")
		}
		expected := make(map[string]struct{}, len(item.ExpectedRecordIDs))
		for _, recordID := range item.ExpectedRecordIDs {
			if recordID == "" {
				return errors.New("retrieval evaluation draft has empty expected record")
			}
			if _, duplicate := expected[recordID]; duplicate {
				return errors.New("retrieval evaluation draft repeats an expected record")
			}
			expected[recordID] = struct{}{}
		}
	}
	if answerable < 12 || noAnswer < 8 {
		return errors.New("retrieval evaluation draft requires 12 answerable and 8 no-answer cases")
	}
	return nil
}
