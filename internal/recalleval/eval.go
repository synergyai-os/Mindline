package recalleval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

var (
	caseIDPattern     = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,63}$`)
	commitmentPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

// ValidateManifest checks both structure and, when supplied, the manifest's
// deterministic fingerprint.
func ValidateManifest(manifest Manifest) error {
	if err := validateManifestStructure(manifest); err != nil {
		return err
	}
	if manifest.Fingerprint == "" {
		return nil
	}
	fingerprint, err := ManifestFingerprint(manifest)
	if err != nil {
		return err
	}
	if manifest.Fingerprint != fingerprint {
		return errors.New("manifest fingerprint does not match structural content")
	}
	return nil
}

func ManifestFingerprint(manifest Manifest) (string, error) {
	if err := validateManifestStructure(manifest); err != nil {
		return "", err
	}
	canonical := manifest
	canonical.Fingerprint = ""
	canonical.Cases = append([]Case(nil), manifest.Cases...)
	sort.Slice(canonical.Cases, func(i, j int) bool { return canonical.Cases[i].CaseID < canonical.Cases[j].CaseID })
	for index := range canonical.Cases {
		canonical.Cases[index].ExpectedRecordFingerprints = append([]string(nil), canonical.Cases[index].ExpectedRecordFingerprints...)
		sort.Strings(canonical.Cases[index].ExpectedRecordFingerprints)
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func validateManifestStructure(manifest Manifest) error {
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported manifest schema: %s", manifest.SchemaVersion)
	}
	if !isFingerprint(manifest.LibraryFingerprint) || !isFingerprint(manifest.BaselineBuild) || !isFingerprint(manifest.ReviewerFingerprint) {
		return errors.New("manifest binding fingerprints must be sha256 commitments")
	}
	if !manifest.LabelsFrozenBeforeRun {
		return errors.New("manifest labels must be frozen before candidate evaluation")
	}
	if len(manifest.Cases) < 20 {
		return errors.New("manifest requires at least 20 held-out cases")
	}
	seen := make(map[string]struct{}, len(manifest.Cases))
	answerable, noAnswer := 0, 0
	for _, item := range manifest.Cases {
		if !caseIDPattern.MatchString(item.CaseID) {
			return fmt.Errorf("invalid structural case id: %q", item.CaseID)
		}
		if _, exists := seen[item.CaseID]; exists {
			return fmt.Errorf("duplicate case id: %s", item.CaseID)
		}
		seen[item.CaseID] = struct{}{}
		if !isFingerprint(item.QueryFingerprint) {
			return fmt.Errorf("case %s query must be fingerprinted", item.CaseID)
		}
		switch item.Kind {
		case CaseAnswerable:
			answerable++
			if len(item.ExpectedRecordFingerprints) == 0 {
				return fmt.Errorf("answerable case %s has no expected records", item.CaseID)
			}
		case CaseNoAnswer:
			noAnswer++
			if len(item.ExpectedRecordFingerprints) != 0 {
				return fmt.Errorf("no-answer case %s has expected records", item.CaseID)
			}
		default:
			return fmt.Errorf("case %s has unsupported kind", item.CaseID)
		}
		expected := make(map[string]struct{}, len(item.ExpectedRecordFingerprints))
		for _, fingerprint := range item.ExpectedRecordFingerprints {
			if !isFingerprint(fingerprint) {
				return fmt.Errorf("case %s expected record must be fingerprinted", item.CaseID)
			}
			if _, exists := expected[fingerprint]; exists {
				return fmt.Errorf("case %s repeats expected record", item.CaseID)
			}
			expected[fingerprint] = struct{}{}
		}
	}
	if answerable < 12 || noAnswer < 8 {
		return errors.New("manifest requires at least 12 answerable and 8 no-answer cases")
	}
	return nil
}

// Score applies the explicit WP-48 formulas to one structural evaluation run.
func Score(manifest Manifest, evaluation Evaluation) (Metrics, error) {
	if err := ValidateManifest(manifest); err != nil {
		return Metrics{}, err
	}
	manifestFingerprint, err := ManifestFingerprint(manifest)
	if err != nil {
		return Metrics{}, err
	}
	if evaluation.SchemaVersion != ResultSchemaVersion || evaluation.LibraryFingerprint != manifest.LibraryFingerprint || evaluation.ManifestFingerprint != manifestFingerprint || !isFingerprint(evaluation.BuildFingerprint) {
		return Metrics{}, errors.New("evaluation binding does not match manifest")
	}
	expectedCases := make(map[string]struct{}, len(manifest.Cases))
	for _, item := range manifest.Cases {
		expectedCases[item.CaseID] = struct{}{}
	}
	byCase := make(map[string]CaseResult, len(evaluation.Cases))
	for _, result := range evaluation.Cases {
		if _, known := expectedCases[result.CaseID]; !known {
			return Metrics{}, fmt.Errorf("evaluation contains unknown case: %s", result.CaseID)
		}
		if _, exists := byCase[result.CaseID]; exists {
			return Metrics{}, fmt.Errorf("duplicate evaluation case: %s", result.CaseID)
		}
		if len(result.Citations) > 5 {
			return Metrics{}, fmt.Errorf("case %s returned more than five citations", result.CaseID)
		}
		seenCitation := map[string]struct{}{}
		for _, citation := range result.Citations {
			if !isFingerprint(citation.RecordFingerprint) {
				return Metrics{}, fmt.Errorf("case %s returned non-fingerprint citation", result.CaseID)
			}
			if _, exists := seenCitation[citation.RecordFingerprint]; exists {
				return Metrics{}, fmt.Errorf("case %s repeats citation", result.CaseID)
			}
			seenCitation[citation.RecordFingerprint] = struct{}{}
		}
		byCase[result.CaseID] = result
	}
	metrics := Metrics{}
	for _, item := range manifest.Cases {
		result, exists := byCase[item.CaseID]
		if !exists {
			return Metrics{}, fmt.Errorf("missing evaluation result for %s", item.CaseID)
		}
		citations := result.Citations
		if item.Kind == CaseNoAnswer {
			metrics.NoAnswerCases++
			if len(citations) > 0 {
				metrics.NoAnswerFalsePositiveRate++
			}
			continue
		}
		metrics.AnswerableCases++
		for _, citation := range citations {
			metrics.ReturnedCitationCount++
			if citation.Valid {
				metrics.FullyValidCitationCount++
			}
		}
		expected := make(map[string]struct{}, len(item.ExpectedRecordFingerprints))
		for _, fingerprint := range item.ExpectedRecordFingerprints {
			expected[fingerprint] = struct{}{}
		}
		matches := 0
		for _, citation := range citations {
			if _, exists := expected[citation.RecordFingerprint]; exists {
				matches++
			}
		}
		metrics.RecallAt5 += float64(matches) / float64(len(expected))
		if len(citations) > 0 {
			metrics.PrecisionAt5 += float64(matches) / float64(len(citations))
		}
	}
	if metrics.AnswerableCases == 0 || metrics.NoAnswerCases == 0 {
		return Metrics{}, errors.New("evaluation must include both answerable and no-answer cases")
	}
	metrics.RecallAt5 /= float64(metrics.AnswerableCases)
	metrics.PrecisionAt5 /= float64(metrics.AnswerableCases)
	metrics.NoAnswerFalsePositiveRate /= float64(metrics.NoAnswerCases)
	if metrics.ReturnedCitationCount > 0 {
		metrics.CitationCompleteness = float64(metrics.FullyValidCitationCount) / float64(metrics.ReturnedCitationCount)
	}
	return metrics, nil
}

// Compare validates same-library/same-manifest baseline and candidate results,
// then applies the signed retrieval thresholds.
func Compare(manifest Manifest, baseline, candidate Evaluation) (ThresholdResult, error) {
	baselineMetrics, err := Score(manifest, baseline)
	if err != nil {
		return ThresholdResult{}, fmt.Errorf("baseline: %w", err)
	}
	candidateMetrics, err := Score(manifest, candidate)
	if err != nil {
		return ThresholdResult{}, fmt.Errorf("candidate: %w", err)
	}
	if baseline.LibraryFingerprint != candidate.LibraryFingerprint || baseline.ManifestFingerprint != candidate.ManifestFingerprint {
		return ThresholdResult{}, errors.New("baseline and candidate are not comparable")
	}
	if baseline.BuildFingerprint != manifest.BaselineBuild {
		return ThresholdResult{}, errors.New("baseline build does not match the frozen manifest")
	}
	manifestFingerprint, _ := ManifestFingerprint(manifest)
	result := ThresholdResult{
		SchemaVersion: ResultSchemaVersion, LibraryFingerprint: manifest.LibraryFingerprint,
		ManifestFingerprint: manifestFingerprint, Baseline: baselineMetrics, Candidate: candidateMetrics,
		Formulas: []string{
			"recall@5=mean(|top-five current record fingerprints intersect expected fingerprints|/|expected fingerprints|)",
			"precision@5=mean(|top-five current record fingerprints intersect expected fingerprints|/|returned current fingerprints up to five|)",
			"citation_completeness=fully_valid_returned_citations/all_returned_citations",
			"no_answer_false_positive_rate=no_answer_cases_with_citations/all_no_answer_cases",
		},
	}
	minimumRecall := math.Max(0.75, baselineMetrics.RecallAt5)
	if candidateMetrics.RecallAt5 < minimumRecall {
		result.ReasonCodes = append(result.ReasonCodes, "recall_at_5_below_threshold")
	}
	if candidateMetrics.PrecisionAt5 < baselineMetrics.PrecisionAt5-0.05 {
		result.ReasonCodes = append(result.ReasonCodes, "precision_at_5_regressed")
	}
	if candidateMetrics.CitationCompleteness != 1 {
		result.ReasonCodes = append(result.ReasonCodes, "citation_completeness_incomplete")
	}
	if candidateMetrics.NoAnswerFalsePositiveRate != 0 {
		result.ReasonCodes = append(result.ReasonCodes, "no_answer_false_positive")
	}
	if candidate.UnselectedHydratedContent {
		result.ReasonCodes = append(result.ReasonCodes, "compact_output_contains_unselected_hydrated_content")
	}
	result.Passed = len(result.ReasonCodes) == 0
	return result, nil
}

func isFingerprint(value string) bool { return commitmentPattern.MatchString(strings.TrimSpace(value)) }
