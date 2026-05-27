package documents

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const corpusAcceptanceDirName = "corpus-acceptance"
const corpusAcceptanceIndependentProvenance = "not_generated_from_evaluated_run"
const CorpusAcceptanceDEC64MinEvalCount = 50
const CorpusAcceptanceDEC64MinThreshold = 0.98

func BuildCorpusAcceptanceBenchmark(pressurePath, answerKeyPath, outDir string, options CorpusAcceptanceBenchmarkOptions) (CorpusAcceptanceBenchmarkSummary, error) {
	if options.Threshold == 0 {
		options.Threshold = CorpusAcceptanceDEC64MinThreshold
	}
	if options.Threshold <= 0 || math.IsNaN(options.Threshold) || math.IsInf(options.Threshold, 0) {
		return CorpusAcceptanceBenchmarkSummary{}, fmt.Errorf("invalid threshold")
	}
	pressureRoot, summary, err := readCorpusAcceptancePressureSummary(pressurePath)
	if err != nil {
		return CorpusAcceptanceBenchmarkSummary{}, err
	}
	answerKey, err := readCorpusAcceptanceAnswerKey(answerKeyPath)
	if err != nil {
		return CorpusAcceptanceBenchmarkSummary{}, err
	}
	benchmark := evaluateCorpusAcceptance(pressureRoot, summary, answerKey, options)
	if err := WriteCorpusAcceptanceBenchmark(outDir, benchmark); err != nil {
		return CorpusAcceptanceBenchmarkSummary{}, err
	}
	return benchmark, nil
}

func readCorpusAcceptancePressureSummary(path string) (string, CorpusPressureSummary, error) {
	if strings.TrimSpace(path) == "" {
		return "", CorpusPressureSummary{}, fmt.Errorf("missing corpus pressure path")
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return "", CorpusPressureSummary{}, err
	}
	if err := rejectSymlinkAncestors(root); err != nil {
		return "", CorpusPressureSummary{}, err
	}
	summaryPath := filepath.Join(root, CorpusPressureDirName, "pressure-summary.json")
	if filepath.Base(root) == CorpusPressureDirName {
		summaryPath = filepath.Join(root, "pressure-summary.json")
		root = filepath.Dir(root)
	}
	if err := rejectIfSymlink(summaryPath); err != nil {
		return "", CorpusPressureSummary{}, err
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return "", CorpusPressureSummary{}, fmt.Errorf("read corpus pressure summary: %w", err)
	}
	var summary CorpusPressureSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return "", CorpusPressureSummary{}, fmt.Errorf("decode corpus pressure summary: %w", err)
	}
	if summary.SchemaVersion != CorpusPressureSummarySchemaVersion {
		return "", CorpusPressureSummary{}, fmt.Errorf("unsupported corpus pressure summary schema version: %s", summary.SchemaVersion)
	}
	return root, summary, nil
}

func readCorpusAcceptanceAnswerKey(path string) (CorpusAcceptanceAnswerKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CorpusAcceptanceAnswerKey{}, fmt.Errorf("read corpus acceptance answer key: %w", err)
	}
	var answerKey CorpusAcceptanceAnswerKey
	if err := json.Unmarshal(data, &answerKey); err != nil {
		return CorpusAcceptanceAnswerKey{}, fmt.Errorf("decode corpus acceptance answer key: %w", err)
	}
	return answerKey, nil
}

func evaluateCorpusAcceptance(root string, pressure CorpusPressureSummary, answerKey CorpusAcceptanceAnswerKey, options CorpusAcceptanceBenchmarkOptions) CorpusAcceptanceBenchmarkSummary {
	benchmark := CorpusAcceptanceBenchmarkSummary{
		SchemaVersion:             CorpusAcceptanceSummarySchemaVersion,
		SuiteID:                   answerKey.SuiteID,
		SuiteKind:                 answerKey.SuiteKind,
		CorpusID:                  pressure.CorpusID,
		CorpusFingerprint:         pressure.CorpusFingerprint,
		CommandConfigFingerprint:  pressure.CommandConfigFingerprint,
		PressureReplayFingerprint: pressure.ReplayFingerprint,
		Threshold:                 options.Threshold,
		HeldOut:                   options.HeldOut,
		Guardrails:                pressure.Guardrails,
		NextImprovementTargets:    []string{},
	}
	graphSummary, graphOK := readCorpusAcceptanceGraphSummary(root, pressure.GraphSummaryPath)
	sourceByID := map[string]CorpusPressureSourceResult{}
	for _, source := range pressure.Sources {
		sourceByID[source.SourceID] = source
	}
	benchmark.SuiteValidityBlockers = validateCorpusAcceptanceAnswerKey(answerKey, pressure, options)
	for _, sourceKey := range answerKey.Sources {
		sourceResult := CorpusAcceptanceSourceSummary{SourceID: sourceKey.SourceID}
		source, ok := sourceByID[sourceKey.SourceID]
		if ok {
			sourceResult.SourceContentHash = source.SourceContentHash
		}
		benchmark.EvalCount += len(sourceKey.ExpectedOutcomes)
		for _, outcome := range sourceKey.ExpectedOutcomes {
			if outcome.ExpectedState == ExpectedOutcomePresent {
				benchmark.ExpectedPresentCount++
			}
			if outcome.ExpectedState == ExpectedOutcomeAbsent {
				benchmark.ExpectedAbsentCount++
			}
		}
		if !ok {
			sourceResult.Blockers = append(sourceResult.Blockers, "missing_pressure_source")
			sourceResult.EvalCount = len(sourceKey.ExpectedOutcomes)
			sourceResult.FalseNegativeCount = countExpectedPresent(sourceKey.ExpectedOutcomes)
			sourceResult.Accuracy = 0
			benchmark.UnjudgedCount += len(sourceKey.ExpectedOutcomes)
			benchmark.FalseNegativeCount += sourceResult.FalseNegativeCount
			benchmark.MissedExpectedCount += sourceResult.FalseNegativeCount
			benchmark.Sources = append(benchmark.Sources, sourceResult)
			continue
		}
		if source.State != CorpusPressureSourceProcessed {
			sourceResult.Blockers = append(sourceResult.Blockers, "source_not_processed:"+string(source.State))
			sourceResult.EvalCount = len(sourceKey.ExpectedOutcomes)
			sourceResult.FalseNegativeCount = countExpectedPresent(sourceKey.ExpectedOutcomes)
			benchmark.UnjudgedCount += len(sourceKey.ExpectedOutcomes)
			benchmark.FalseNegativeCount += sourceResult.FalseNegativeCount
			benchmark.MissedExpectedCount += sourceResult.FalseNegativeCount
			if source.ReasonCode == CorpusPressureReasonSemanticError {
				benchmark.ModelErrorCount += len(sourceKey.ExpectedOutcomes)
			}
			benchmark.Sources = append(benchmark.Sources, sourceResult)
			continue
		}
		sourceBenchmark := evaluateCorpusAcceptanceSource(root, source, sourceKey)
		applyCorpusAcceptanceSource(&benchmark, &sourceResult, sourceBenchmark)
		benchmark.Sources = append(benchmark.Sources, sourceResult)
	}
	if graphOK {
		benchmark.DuplicateCount += graphSummary.RelationTypeCounts[CorpusRelationPossibleDuplicate]
		benchmark.ContradictionCount += graphSummary.RelationTypeCounts[CorpusRelationContradicts]
		benchmark.FalsePositiveCount += graphSummary.RelationMetrics.FalsePositiveCount
		benchmark.FalseNegativeCount += graphSummary.RelationMetrics.FalseNegativeCount
	}
	if requiresCorpusRelationCoverage(answerKey) && !graphOK {
		benchmark.SuiteValidityBlockers = append(benchmark.SuiteValidityBlockers, "graph_summary_missing")
	}
	if requiresCorpusRelationCoverage(answerKey) && graphOK && graphSummary.RelationMetrics.EvalCountedRelationCount == 0 && graphSummary.RelationMetrics.FalseNegativeCount == 0 {
		benchmark.SuiteValidityBlockers = append(benchmark.SuiteValidityBlockers, "relation_answer_key_missing")
	}
	benchmark.SuiteValid = len(benchmark.SuiteValidityBlockers) == 0
	benchmark.Accuracy = ratio(benchmark.MatchedExpectedCount, benchmark.EvalCount)
	benchmark.FalsePositiveRate = ratio(benchmark.FalsePositiveCount, benchmark.CandidateCount)
	benchmark.FalseNegativeRate = ratio(benchmark.FalseNegativeCount, benchmark.EvalCount)
	benchmark.ReviewBurdenRate = ratio(benchmark.ReviewBurdenCount, benchmark.CandidateCount+benchmark.FalseNegativeCount)
	benchmark.NextImprovementTargets = corpusAcceptanceTargets(benchmark)
	benchmark.EligibilityBlockers = corpusAcceptanceEligibilityBlockers(benchmark)
	benchmark.DEC64Eligible = len(benchmark.EligibilityBlockers) == 0
	sort.SliceStable(benchmark.Sources, func(i, j int) bool {
		return benchmark.Sources[i].SourceID < benchmark.Sources[j].SourceID
	})
	return benchmark
}

func validateCorpusAcceptanceAnswerKey(answerKey CorpusAcceptanceAnswerKey, pressure CorpusPressureSummary, options CorpusAcceptanceBenchmarkOptions) []string {
	var blockers []string
	if answerKey.SchemaVersion != CorpusAcceptanceAnswerKeySchemaVersion {
		blockers = append(blockers, "unsupported_schema_version")
	}
	if strings.TrimSpace(answerKey.SuiteID) == "" || sanitizeID(answerKey.SuiteID) != answerKey.SuiteID {
		blockers = append(blockers, "unsafe_suite_id")
	}
	if answerKey.SuiteKind != CorpusAcceptanceSuiteHeldOut && answerKey.SuiteKind != CorpusAcceptanceSuiteCalibration {
		blockers = append(blockers, "unsupported_suite_kind")
	}
	if strings.TrimSpace(answerKey.Provenance.Labeler) == "" {
		blockers = append(blockers, "missing_labeler")
	}
	if answerKey.Provenance.Independence != corpusAcceptanceIndependentProvenance {
		blockers = append(blockers, "answer_key_not_independent")
	}
	if answerKey.CorpusID != pressure.CorpusID {
		blockers = append(blockers, "corpus_id_mismatch")
	}
	if answerKey.CorpusFingerprint != pressure.CorpusFingerprint {
		blockers = append(blockers, "corpus_fingerprint_mismatch")
	}
	if answerKey.CommandConfigFingerprint != "" && answerKey.CommandConfigFingerprint != pressure.CommandConfigFingerprint {
		blockers = append(blockers, "command_config_fingerprint_mismatch")
	}
	if answerKey.MinEvalCount <= 0 {
		blockers = append(blockers, "missing_min_eval_count")
	}
	if answerKey.SuiteKind == CorpusAcceptanceSuiteHeldOut && answerKey.MinEvalCount < CorpusAcceptanceDEC64MinEvalCount {
		blockers = append(blockers, "below_dec64_min_eval_count")
	}
	if options.HeldOut && options.Threshold < CorpusAcceptanceDEC64MinThreshold {
		blockers = append(blockers, "below_dec64_threshold")
	}
	if options.HeldOut && answerKey.SuiteKind != CorpusAcceptanceSuiteHeldOut {
		blockers = append(blockers, "held_out_option_requires_held_out_suite")
	}
	evalCount := 0
	seenSources := map[string]bool{}
	kinds := map[SemanticCandidateKind]bool{}
	relations := map[SemanticRelationshipType]bool{}
	failures := map[SemanticAcceptanceReason]bool{}
	for _, source := range answerKey.Sources {
		if strings.TrimSpace(source.SourceID) == "" || sanitizeID(source.SourceID) != source.SourceID {
			blockers = append(blockers, "unsafe_source_id")
		}
		if seenSources[source.SourceID] {
			blockers = append(blockers, "duplicate_source_id")
		}
		seenSources[source.SourceID] = true
		if source.SourceDocumentID != "" && (containsUnsafeMarker(source.SourceDocumentID) || containsGovernanceID(source.SourceDocumentID)) {
			blockers = append(blockers, "unsafe_source_document_id")
		}
		sourceAnswerKey := SemanticAcceptanceAnswerKey{
			SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
			AnswerKeyID:      answerKey.SuiteID + "-" + source.SourceID,
			SourceDocumentID: firstNonBlankCorpusString(source.SourceDocumentID, source.SourceID),
			ExpectedOutcomes: source.ExpectedOutcomes,
		}
		if err := ValidateSemanticAcceptanceAnswerKey(sourceAnswerKey); err != nil {
			blockers = append(blockers, "invalid_expected_outcomes:"+source.SourceID)
		}
		for _, outcome := range source.ExpectedOutcomes {
			evalCount++
			kinds[outcome.ExpectedKind] = true
			for _, relation := range outcome.RelationRequirements {
				relations[relation] = true
			}
			if outcome.ExpectedState == ExpectedOutcomeAbsent {
				failures[SemanticAcceptanceReasonUnexpectedCandidate] = true
			}
			if outcome.ExpectedState == ExpectedOutcomePresent {
				failures[SemanticAcceptanceReasonMissingExpectedOutcome] = true
				failures[SemanticAcceptanceReasonWrongKind] = true
			}
		}
	}
	if evalCount < answerKey.MinEvalCount {
		blockers = append(blockers, "below_min_eval_count")
	}
	if answerKey.CoverageRequirements.MinSourceCount > 0 && len(answerKey.Sources) < answerKey.CoverageRequirements.MinSourceCount {
		blockers = append(blockers, "below_min_source_count")
	}
	for _, kind := range answerKey.CoverageRequirements.CandidateKinds {
		if !kinds[kind] {
			blockers = append(blockers, "missing_candidate_kind_coverage:"+string(kind))
		}
	}
	for _, relation := range answerKey.CoverageRequirements.RelationTypes {
		if !relations[relation] {
			blockers = append(blockers, "missing_relation_coverage:"+string(relation))
		}
	}
	for _, failure := range answerKey.CoverageRequirements.FailureModes {
		if !failures[failure] {
			blockers = append(blockers, "missing_failure_mode_coverage:"+string(failure))
		}
	}
	if containsUnsafeCorpusAcceptanceAnswerKeyMarker(answerKey) {
		blockers = append(blockers, "answer_key_contains_private_marker")
	}
	return uniqueStrings(blockers)
}

func firstNonBlankCorpusString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func evaluateCorpusAcceptanceSource(root string, source CorpusPressureSourceResult, sourceKey CorpusAcceptanceAnswerKeySource) SemanticAcceptanceSummary {
	semanticRoot, err := containedCorpusAcceptancePath(root, source.SemanticRunDir)
	if err != nil {
		return unevaluatedSourceAcceptance(source.SemanticRunID, sourceKey)
	}
	_, candidates, relations, err := readSemanticAcceptanceInput(semanticRoot)
	if err != nil {
		return unevaluatedSourceAcceptance(source.SemanticRunID, sourceKey)
	}
	sourceDocumentID := sourceKey.SourceDocumentID
	if strings.TrimSpace(sourceDocumentID) == "" {
		sourceDocumentID = corpusAcceptanceArtifactSourceDocumentID(candidates, source.SourceID)
	}
	answerKey := SemanticAcceptanceAnswerKey{
		SchemaVersion:    SemanticAcceptanceAnswerKeySchemaVersion,
		AnswerKeyID:      sourceKey.SourceID,
		SourceDocumentID: sourceDocumentID,
		ExpectedOutcomes: sourceKey.ExpectedOutcomes,
	}
	acceptance := EvaluateSemanticAcceptance(source.SemanticRunID, answerKey, candidates, relations)
	acceptance.WrongKindCount = countWrongKindMatches(answerKey.SourceDocumentID, sourceKey.ExpectedOutcomes, candidates)
	return acceptance
}

func corpusAcceptanceArtifactSourceDocumentID(candidates []SemanticCandidate, fallback string) string {
	sourceDocumentID := ""
	for _, candidate := range candidates {
		candidateSourceID := candidateSourceDocumentID(candidate)
		if strings.TrimSpace(candidateSourceID) == "" {
			continue
		}
		if sourceDocumentID == "" {
			sourceDocumentID = candidateSourceID
			continue
		}
		if sourceDocumentID != candidateSourceID {
			return fallback
		}
	}
	if sourceDocumentID != "" {
		return sourceDocumentID
	}
	return fallback
}

func unevaluatedSourceAcceptance(runID string, sourceKey CorpusAcceptanceAnswerKeySource) SemanticAcceptanceSummary {
	evalCount := len(sourceKey.ExpectedOutcomes)
	missed := countExpectedPresent(sourceKey.ExpectedOutcomes)
	return SemanticAcceptanceSummary{
		SchemaVersion:        SemanticAcceptanceSummarySchemaVersion,
		RunID:                runID,
		AnswerKeyID:          sourceKey.SourceID,
		ExpectedPresentCount: missed,
		ExpectedAbsentCount:  evalCount - missed,
		MissedExpectedCount:  missed,
		FalseNegativeCount:   missed,
		NeedsReviewCount:     evalCount,
		ReviewBurdenCount:    evalCount,
		QualityStatement:     "Source could not be evaluated.",
	}
}

func applyCorpusAcceptanceSource(benchmark *CorpusAcceptanceBenchmarkSummary, sourceResult *CorpusAcceptanceSourceSummary, acceptance SemanticAcceptanceSummary) {
	correctAbsent := correctExpectedAbsentCount(acceptance)
	sourceResult.EvalCount = acceptance.ExpectedPresentCount + acceptance.ExpectedAbsentCount
	sourceResult.CandidateCount = acceptance.CandidateCount
	sourceResult.MatchedExpectedCount = acceptance.MatchedExpectedCount + correctAbsent
	sourceResult.FalsePositiveCount = acceptance.FalsePositiveCount
	sourceResult.FalseNegativeCount = acceptance.FalseNegativeCount
	sourceResult.WrongKindCount = acceptance.WrongKindCount
	sourceResult.HumanReviewRequiredCount = acceptance.NeedsReviewCount
	sourceResult.AcceptanceSummaryPath = filepath.ToSlash(filepath.Join(corpusAcceptanceDirName, "sources", sourceResult.SourceID, "acceptance-summary.json"))
	sourceResult.Accuracy = ratio(sourceResult.MatchedExpectedCount, sourceResult.EvalCount)
	benchmark.CandidateCount += acceptance.CandidateCount
	benchmark.AcceptedCount += acceptance.AcceptedCount
	benchmark.MatchedExpectedCount += acceptance.MatchedExpectedCount + correctAbsent
	benchmark.MissedExpectedCount += acceptance.MissedExpectedCount
	benchmark.FalsePositiveCount += acceptance.FalsePositiveCount
	benchmark.FalseNegativeCount += acceptance.FalseNegativeCount
	benchmark.WrongKindCount += acceptance.WrongKindCount
	benchmark.DuplicateCount += acceptance.DuplicateCount
	benchmark.HumanReviewRequiredCount += acceptance.NeedsReviewCount
	if acceptance.QualityStatement == "Source could not be evaluated." {
		benchmark.UnjudgedCount += sourceResult.EvalCount
	}
	benchmark.SafetyBlockedCount += acceptance.BlockedCount
	benchmark.ReviewBurdenCount += acceptance.ReviewBurdenCount
}

func correctExpectedAbsentCount(acceptance SemanticAcceptanceSummary) int {
	count := 0
	for _, outcome := range acceptance.ExpectedOutcomes {
		if outcome.ExpectedState == ExpectedOutcomeAbsent && outcome.AcceptanceState == SemanticAcceptanceAccepted && outcome.Reason == SemanticAcceptanceReasonCorrect {
			count++
		}
	}
	return count
}

func readCorpusAcceptanceGraphSummary(root, relative string) (CorpusGraphSummary, bool) {
	if strings.TrimSpace(relative) == "" {
		return CorpusGraphSummary{}, false
	}
	path, err := containedCorpusAcceptancePath(root, relative)
	if err != nil {
		return CorpusGraphSummary{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CorpusGraphSummary{}, false
	}
	var summary CorpusGraphSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return CorpusGraphSummary{}, false
	}
	if summary.SchemaVersion != CorpusGraphSummarySchemaVersion {
		return CorpusGraphSummary{}, false
	}
	return summary, true
}

func containedCorpusAcceptancePath(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("unsafe corpus acceptance artifact path: %s", relative)
	}
	cleanRelative := filepath.Clean(filepath.FromSlash(relative))
	if cleanRelative == "." || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe corpus acceptance artifact path: %s", relative)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, cleanRelative))
	if err != nil {
		return "", err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("corpus acceptance artifact path escapes root: %s", relative)
	}
	if err := rejectSymlinkAncestors(targetAbs); err != nil {
		return "", err
	}
	if err := rejectIfSymlink(targetAbs); err != nil {
		return "", err
	}
	return targetAbs, nil
}

func requiresCorpusRelationCoverage(answerKey CorpusAcceptanceAnswerKey) bool {
	return len(answerKey.CoverageRequirements.RelationTypes) > 0
}

func countWrongKindMatches(sourceDocumentID string, outcomes []SemanticExpectedOutcome, candidates []SemanticCandidate) int {
	count := 0
	for _, outcome := range outcomes {
		if outcome.ExpectedState != ExpectedOutcomePresent {
			continue
		}
		for _, candidate := range candidates {
			if candidate.CandidateKind == outcome.ExpectedKind || candidateSourceDocumentID(candidate) != sourceDocumentID {
				continue
			}
			if containsAllSignals(candidate.Title, outcome.TitleSignals) &&
				containsAllSignals(candidate.Summary, outcome.SummarySignals) &&
				hasRequiredEvidenceRanges(candidate, outcome.RequiredEvidence, outcome.AcceptableAlternates) {
				count++
				break
			}
		}
	}
	return count
}

func countExpectedPresent(outcomes []SemanticExpectedOutcome) int {
	count := 0
	for _, outcome := range outcomes {
		if outcome.ExpectedState == ExpectedOutcomePresent {
			count++
		}
	}
	return count
}

func corpusAcceptanceEligibilityBlockers(summary CorpusAcceptanceBenchmarkSummary) []string {
	var blockers []string
	if !summary.HeldOut {
		blockers = append(blockers, "not_held_out")
	}
	if summary.SuiteKind != CorpusAcceptanceSuiteHeldOut {
		blockers = append(blockers, "suite_not_held_out")
	}
	if !summary.SuiteValid {
		blockers = append(blockers, "suite_invalid")
	}
	if summary.Accuracy < summary.Threshold {
		blockers = append(blockers, "below_accuracy_threshold")
	}
	if summary.Threshold < CorpusAcceptanceDEC64MinThreshold {
		blockers = append(blockers, "below_dec64_threshold")
	}
	if summary.EvalCount < CorpusAcceptanceDEC64MinEvalCount {
		blockers = append(blockers, "below_dec64_min_eval_count")
	}
	if summary.FalsePositiveCount > 0 {
		blockers = append(blockers, "false_positive")
	}
	if summary.FalseNegativeCount > 0 {
		blockers = append(blockers, "false_negative")
	}
	if summary.WrongKindCount > 0 {
		blockers = append(blockers, "wrong_kind")
	}
	if summary.ModelErrorCount > 0 {
		blockers = append(blockers, "model_error")
	}
	if summary.UnjudgedCount > 0 {
		blockers = append(blockers, "unjudged")
	}
	if summary.HumanReviewRequiredCount > 0 {
		blockers = append(blockers, "human_review_required")
	}
	if summary.SafetyBlockedCount > 0 {
		blockers = append(blockers, "safety_blocked")
	}
	if summary.Guardrails.HostedTelemetryExports > 0 {
		blockers = append(blockers, "hosted_telemetry_export")
	}
	if summary.Guardrails.DestinationWrites > 0 {
		blockers = append(blockers, "destination_write")
	}
	return uniqueStrings(blockers)
}

func corpusAcceptanceTargets(summary CorpusAcceptanceBenchmarkSummary) []string {
	var targets []string
	if !summary.SuiteValid {
		targets = append(targets, "suite_validity")
	}
	if summary.FalseNegativeCount > 0 {
		targets = append(targets, "recall")
	}
	if summary.FalsePositiveCount > 0 {
		targets = append(targets, "precision")
	}
	if summary.WrongKindCount > 0 {
		targets = append(targets, "classification_kind")
	}
	if summary.HumanReviewRequiredCount > 0 || summary.ReviewBurdenCount > 0 {
		targets = append(targets, "review_burden")
	}
	if summary.SafetyBlockedCount > 0 {
		targets = append(targets, "safety_containment")
	}
	return uniqueStrings(targets)
}

func containsUnsafeCorpusAcceptanceAnswerKeyMarker(answerKey CorpusAcceptanceAnswerKey) bool {
	parts := []string{answerKey.SuiteID, answerKey.CorpusID, answerKey.CorpusFingerprint, answerKey.CommandConfigFingerprint, answerKey.Provenance.Labeler, answerKey.Provenance.Independence}
	for _, source := range answerKey.Sources {
		parts = append(parts, source.SourceID, source.SourceDocumentID)
		for _, outcome := range source.ExpectedOutcomes {
			parts = append(parts, outcome.ExpectedOutcomeID, outcome.Notes)
			parts = append(parts, outcome.RequiredEvidence...)
			parts = append(parts, outcome.AcceptableAlternates...)
			parts = append(parts, outcome.TitleSignals...)
			parts = append(parts, outcome.SummarySignals...)
		}
	}
	body := strings.Join(parts, "\n")
	return containsUnsafeMarker(body) || containsGovernanceID(body)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
