package recalleval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// OwnerManifest is deliberately owner-only runtime input. It may contain a
// query, but must never be committed or emitted by a proof command. Expected
// records are commitments rather than canonical IDs.
type OwnerManifest struct {
	SchemaVersion         string      `json:"schema_version"`
	LibraryFingerprint    string      `json:"library_fingerprint"`
	Baseline              RunBinding  `json:"baseline"`
	ReviewerFingerprint   string      `json:"reviewer_fingerprint"`
	LabelsFrozenBeforeRun bool        `json:"labels_frozen_before_run"`
	Cases                 []OwnerCase `json:"cases"`
	Fingerprint           string      `json:"fingerprint,omitempty"`
}

type OwnerCase struct {
	CaseID                       string   `json:"case_id"`
	Kind                         string   `json:"kind"`
	Query                        string   `json:"query"`
	ExpectedCanonicalCommitments []string `json:"expected_canonical_commitments"`
}

// RunBinding prevents a baseline/candidate comparison across a different
// source tree or frozen runtime configuration.
type RunBinding struct {
	BuildFingerprint         string `json:"build_fingerprint"`
	TreeFingerprint          string `json:"tree_fingerprint"`
	ConfigurationFingerprint string `json:"configuration_fingerprint"`
}

func (binding RunBinding) Validate() error {
	if !isFingerprint(binding.BuildFingerprint) || !isFingerprint(binding.TreeFingerprint) || !isFingerprint(binding.ConfigurationFingerprint) {
		return errors.New("run binding requires build, tree, and configuration commitments")
	}
	return nil
}

// CompactSearchPort and CanonicalEvidencePort are separate on purpose. A
// compact packet is never trusted as proof that a citation remains canonical.
type CompactSearchPort interface {
	SearchCompact(context.Context, string) (CompactSearchResult, error)
}

type CanonicalEvidencePort interface {
	GetCanonicalEvidence(context.Context, string) (CanonicalEvidence, error)
}

var ErrFrozenLibraryBinding = errors.New("frozen library binding unavailable")

type CompactSearchResult struct {
	Citations                 []CompactCitation `json:"citations"`
	UnselectedHydratedContent bool              `json:"unselected_hydrated_content"`
	LibraryFingerprint        string            `json:"library_fingerprint"`
}

type CompactCitation struct {
	RecordID string `json:"record_id"`
}

// CanonicalEvidence is local-only hydration evidence. Run returns its
// commitment, never this value or its record identity.
type CanonicalEvidence struct {
	RecordID           string   `json:"record_id"`
	SourceCommitment   string   `json:"source_commitment"`
	AuthorityClass     string   `json:"authority_class"`
	Current            bool     `json:"current"`
	ContentHash        string   `json:"content_hash"`
	Missingness        []string `json:"missingness"`
	ResourceStates     []string `json:"resource_states"`
	LibraryFingerprint string   `json:"library_fingerprint"`
}

func CanonicalEvidenceCommitment(evidence CanonicalEvidence) (string, error) {
	if evidence.RecordID == "" || !isFingerprint(evidence.SourceCommitment) || evidence.AuthorityClass == "" || !evidence.Current || !isFingerprint(evidence.ContentHash) || !isFingerprint(evidence.LibraryFingerprint) {
		return "", errors.New("incomplete current canonical evidence")
	}
	canonical := struct {
		RecordID         string   `json:"record_id"`
		SourceCommitment string   `json:"source_commitment"`
		AuthorityClass   string   `json:"authority_class"`
		Current          bool     `json:"current"`
		ContentHash      string   `json:"content_hash"`
		Missingness      []string `json:"missingness"`
		ResourceStates   []string `json:"resource_states"`
	}{
		RecordID: evidence.RecordID, SourceCommitment: evidence.SourceCommitment,
		AuthorityClass: evidence.AuthorityClass, Current: evidence.Current,
		ContentHash:    evidence.ContentHash,
		Missingness:    append([]string(nil), evidence.Missingness...),
		ResourceStates: append([]string(nil), evidence.ResourceStates...),
	}
	sort.Strings(canonical.Missingness)
	sort.Strings(canonical.ResourceStates)
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

type RunResult struct {
	Evaluation Evaluation `json:"evaluation"`
	Binding    RunBinding `json:"binding"`
}

func (manifest OwnerManifest) Validate() error {
	if err := validateOwnerManifestStructure(manifest); err != nil {
		return err
	}
	if manifest.Fingerprint != "" {
		fingerprint, err := ownerManifestFingerprintUnchecked(manifest)
		if err != nil || fingerprint != manifest.Fingerprint {
			return errors.New("owner manifest fingerprint does not match structural content")
		}
	}
	return nil
}

func validateOwnerManifestStructure(manifest OwnerManifest) error {
	if manifest.SchemaVersion != ManifestSchemaVersion || !isFingerprint(manifest.LibraryFingerprint) || !isFingerprint(manifest.ReviewerFingerprint) || !manifest.LabelsFrozenBeforeRun {
		return errors.New("owner manifest has invalid structural bindings")
	}
	if err := manifest.Baseline.Validate(); err != nil {
		return err
	}
	if len(manifest.Cases) < 20 {
		return errors.New("owner manifest requires at least 20 cases")
	}
	answerable, noAnswer := 0, 0
	seen := map[string]struct{}{}
	for _, item := range manifest.Cases {
		if !caseIDPattern.MatchString(item.CaseID) || item.Query == "" {
			return errors.New("owner manifest contains an invalid case")
		}
		if _, exists := seen[item.CaseID]; exists {
			return errors.New("owner manifest repeats a case")
		}
		seen[item.CaseID] = struct{}{}
		switch item.Kind {
		case CaseAnswerable:
			answerable++
			if len(item.ExpectedCanonicalCommitments) == 0 {
				return errors.New("answerable case has no expected commitment")
			}
		case CaseNoAnswer:
			noAnswer++
			if len(item.ExpectedCanonicalCommitments) != 0 {
				return errors.New("no-answer case has expected commitment")
			}
		default:
			return errors.New("owner manifest has unsupported case kind")
		}
		for _, commitment := range item.ExpectedCanonicalCommitments {
			if !isFingerprint(commitment) {
				return errors.New("owner manifest contains a non-commitment expected record")
			}
		}
	}
	if answerable < 12 || noAnswer < 8 {
		return errors.New("owner manifest requires 12 answerable and 8 no-answer cases")
	}
	return nil
}

func OwnerManifestFingerprint(manifest OwnerManifest) (string, error) {
	if err := validateOwnerManifestStructure(manifest); err != nil {
		return "", err
	}
	return ownerManifestFingerprintUnchecked(manifest)
}

func ownerManifestFingerprintUnchecked(manifest OwnerManifest) (string, error) {
	canonical := manifest
	canonical.Fingerprint = ""
	canonical.Cases = append([]OwnerCase(nil), manifest.Cases...)
	sort.Slice(canonical.Cases, func(i, j int) bool { return canonical.Cases[i].CaseID < canonical.Cases[j].CaseID })
	for index := range canonical.Cases {
		sort.Strings(canonical.Cases[index].ExpectedCanonicalCommitments)
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

// Run executes compact search then explicitly hydrates every selected citation
// through canonical evidence. Citation validity is computed here, never
// accepted from a caller.
func Run(ctx context.Context, manifest OwnerManifest, binding RunBinding, compact CompactSearchPort, canonical CanonicalEvidencePort) (RunResult, error) {
	if err := manifest.Validate(); err != nil {
		return RunResult{}, err
	}
	if err := binding.Validate(); err != nil {
		return RunResult{}, err
	}
	structural, err := structuralManifest(manifest)
	if err != nil {
		return RunResult{}, err
	}
	manifestFingerprint, err := ManifestFingerprint(structural)
	if err != nil {
		return RunResult{}, err
	}
	result := Evaluation{SchemaVersion: ResultSchemaVersion, LibraryFingerprint: manifest.LibraryFingerprint, ManifestFingerprint: manifestFingerprint, BuildFingerprint: binding.BuildFingerprint}
	for _, item := range manifest.Cases {
		packet, err := compact.SearchCompact(ctx, item.Query)
		if err != nil {
			return RunResult{}, fmt.Errorf("compact search %s: %w", item.CaseID, err)
		}
		if packet.LibraryFingerprint != manifest.LibraryFingerprint {
			return RunResult{}, fmt.Errorf("compact search %s changed the frozen library", item.CaseID)
		}
		caseResult := CaseResult{CaseID: item.CaseID}
		for _, compactCitation := range packet.Citations {
			evidence, err := canonical.GetCanonicalEvidence(ctx, compactCitation.RecordID)
			if err != nil {
				if errors.Is(err, ErrFrozenLibraryBinding) {
					return RunResult{}, fmt.Errorf("canonical evidence %s: %w", item.CaseID, err)
				}
				caseResult.Citations = append(caseResult.Citations, Citation{
					RecordFingerprint: invalidCitationCommitment(compactCitation.RecordID),
					Valid:             false,
				})
				continue
			}
			if evidence.LibraryFingerprint != manifest.LibraryFingerprint {
				return RunResult{}, fmt.Errorf("canonical evidence %s changed the frozen library", item.CaseID)
			}
			commitment, err := CanonicalEvidenceCommitment(evidence)
			if err != nil {
				caseResult.Citations = append(caseResult.Citations, Citation{
					RecordFingerprint: invalidCitationCommitment(compactCitation.RecordID),
					Valid:             false,
				})
				continue
			}
			caseResult.Citations = append(caseResult.Citations, Citation{RecordFingerprint: commitment, Valid: true})
		}
		result.Cases = append(result.Cases, caseResult)
		result.UnselectedHydratedContent = result.UnselectedHydratedContent || packet.UnselectedHydratedContent
	}
	return RunResult{Evaluation: result, Binding: binding}, nil
}

func invalidCitationCommitment(recordID string) string {
	digest := sha256.Sum256([]byte("invalid-canonical-citation\x00" + recordID))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func CompareRuns(manifest OwnerManifest, baseline, candidate RunResult) (ThresholdResult, error) {
	if err := manifest.Validate(); err != nil {
		return ThresholdResult{}, err
	}
	if err := baseline.Binding.Validate(); err != nil {
		return ThresholdResult{}, err
	}
	if err := candidate.Binding.Validate(); err != nil {
		return ThresholdResult{}, err
	}
	if baseline.Binding != manifest.Baseline ||
		baseline.Binding.ConfigurationFingerprint != candidate.Binding.ConfigurationFingerprint {
		return ThresholdResult{}, errors.New("baseline and candidate must bind the frozen manifest and runtime configuration")
	}
	if baseline.Evaluation.BuildFingerprint != baseline.Binding.BuildFingerprint ||
		candidate.Evaluation.BuildFingerprint != candidate.Binding.BuildFingerprint {
		return ThresholdResult{}, errors.New("evaluation build does not match its run binding")
	}
	structural, err := structuralManifest(manifest)
	if err != nil {
		return ThresholdResult{}, err
	}
	structural.Fingerprint, err = ManifestFingerprint(structural)
	if err != nil {
		return ThresholdResult{}, err
	}
	return Compare(structural, baseline.Evaluation, candidate.Evaluation)
}

func structuralManifest(manifest OwnerManifest) (Manifest, error) {
	result := Manifest{SchemaVersion: ManifestSchemaVersion, LibraryFingerprint: manifest.LibraryFingerprint, BaselineBuild: manifest.Baseline.BuildFingerprint, ReviewerFingerprint: manifest.ReviewerFingerprint, LabelsFrozenBeforeRun: manifest.LabelsFrozenBeforeRun}
	for _, item := range manifest.Cases {
		query := sha256.Sum256([]byte(item.Query))
		result.Cases = append(result.Cases, Case{CaseID: item.CaseID, Kind: item.Kind, QueryFingerprint: "sha256:" + hex.EncodeToString(query[:]), ExpectedRecordFingerprints: append([]string(nil), item.ExpectedCanonicalCommitments...)})
	}
	if err := ValidateManifest(result); err != nil {
		return Manifest{}, err
	}
	return result, nil
}
