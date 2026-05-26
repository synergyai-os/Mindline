package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	AutonomyReadinessReportSchemaVersion = "autonomy-readiness-report/v0.1"
	AutonomyReadinessEvaluatorVersion    = "wp-23/v0.1"
	AutonomyReadinessDefaultThreshold    = 0.98
)

type AutonomyReadinessOptions struct {
	Threshold float64
	HeldOut   bool
}

type AutonomyReadinessThresholdStatus string

const (
	AutonomyReadinessEligible    AutonomyReadinessThresholdStatus = "eligible"
	AutonomyReadinessNotEligible AutonomyReadinessThresholdStatus = "not_eligible"
)

type AutonomyReadinessProjectionStatus string

const (
	AutonomyReadinessProjectionDisabled AutonomyReadinessProjectionStatus = "disabled"
	AutonomyReadinessProjectionSent     AutonomyReadinessProjectionStatus = "sent"
	AutonomyReadinessProjectionFailed   AutonomyReadinessProjectionStatus = "failed"
	AutonomyReadinessProjectionBlocked  AutonomyReadinessProjectionStatus = "blocked_unsafe"
)

type AutonomyReadinessReport struct {
	SchemaVersion    string                            `json:"schema_version"`
	EvaluatorVersion string                            `json:"evaluator_version"`
	SuiteID          string                            `json:"suite_id"`
	SourceRunIDs     []string                          `json:"source_run_ids"`
	HeldOut          bool                              `json:"held_out"`
	Threshold        float64                           `json:"threshold"`
	ThresholdStatus  AutonomyReadinessThresholdStatus  `json:"threshold_status"`
	Accuracy         float64                           `json:"accuracy"`
	Counts           AutonomyReadinessCounts           `json:"counts"`
	SafetyCounters   AutonomyReadinessSafetyCounters   `json:"safety_counters"`
	KRs              map[string]AutonomyReadinessKR    `json:"krs"`
	Slices           AutonomyReadinessSlices           `json:"slices"`
	Improvement      []AutonomyReadinessImprovement    `json:"top_improvement_targets"`
	Blockers         []string                          `json:"blockers"`
	Projection       AutonomyReadinessProjectionReport `json:"posthog_projection"`
}

type AutonomyReadinessCounts struct {
	SourceCount                   int `json:"source_count"`
	CandidateCount                int `json:"candidate_count"`
	JudgedCount                   int `json:"judged_count"`
	RemainingCount                int `json:"remaining_count"`
	AcceptedCount                 int `json:"accepted_count"`
	RejectedCount                 int `json:"rejected_count"`
	UnclearCount                  int `json:"unclear_count"`
	DuplicateCount                int `json:"duplicate_count"`
	WrongKindCount                int `json:"wrong_kind_count"`
	FalsePositiveCount            int `json:"false_positive_count"`
	FalseNegativeCount            int `json:"false_negative_count"`
	EvalCountedAcceptedCount      int `json:"eval_counted_accepted_count"`
	EvalCountedFalsePositiveCount int `json:"eval_counted_false_positive_count"`
	EvalCountedFalseNegativeCount int `json:"eval_counted_false_negative_count"`
	EvalCountedUnclearCount       int `json:"eval_counted_unclear_count"`
	EvalCountedRemainingCount     int `json:"eval_counted_remaining_count"`
	BlockedCount                  int `json:"blocked_count"`
	SkippedCount                  int `json:"skipped_count"`
	EvalCountedCount              int `json:"eval_counted_count"`
	EvidenceReadyCount            int `json:"evidence_ready_count"`
	EvidenceExcludedCount         int `json:"evidence_excluded_count"`
	HumanReviewRequiredCount      int `json:"human_review_required_count"`
	EvalCountedHumanReviewCount   int `json:"eval_counted_human_review_required_count"`
	MachineTriagedCount           int `json:"machine_triaged_count"`
	AgentReviewedCount            int `json:"agent_reviewed_count"`
	ReviewBurdenCount             int `json:"review_burden_count"`
	ModelErrorCount               int `json:"model_error_count"`
	EvalCountedModelErrorCount    int `json:"eval_counted_model_error_count"`
}

type AutonomyReadinessSafetyCounters struct {
	DestinationWrites         int `json:"destination_writes"`
	AutoAccepts               int `json:"auto_accepts"`
	NoHumanClaims             int `json:"no_human_claims"`
	CommittedPrivateArtifacts int `json:"committed_private_artifacts"`
}

type AutonomyReadinessKR struct {
	Status  string `json:"status"`
	Passed  bool   `json:"passed"`
	Current string `json:"current"`
	Target  string `json:"target"`
}

type AutonomyReadinessSlices struct {
	BySourceDocument          map[string]map[string]int `json:"by_source_document"`
	BySourceType              map[string]map[string]int `json:"by_source_type"`
	ByCandidateKind           map[string]map[string]int `json:"by_candidate_kind"`
	ByConfidence              map[string]map[string]int `json:"by_confidence"`
	ByReviewStatus            map[string]map[string]int `json:"by_review_status"`
	ByRelationPresence        map[string]map[string]int `json:"by_relation_presence"`
	ByRelationType            map[string]map[string]int `json:"by_relation_type"`
	ByFailureReason           map[string]map[string]int `json:"by_failure_reason"`
	ByEvidenceReadinessReason map[string]int            `json:"by_evidence_readiness_reason"`
	ByProviderModel           map[string]int            `json:"by_provider_model"`
	ByRunStatus               map[string]int            `json:"by_run_status"`
}

type AutonomyReadinessImprovement struct {
	Code    string   `json:"code"`
	Count   int      `json:"count"`
	Summary string   `json:"summary"`
	Refs    []string `json:"local_artifact_refs,omitempty"`
}

type AutonomyReadinessProjectionReport struct {
	Status        AutonomyReadinessProjectionStatus `json:"status"`
	SchemaVersion string                            `json:"schema_version,omitempty"`
	ErrorClass    string                            `json:"error_class,omitempty"`
}

type autonomyReadinessTraceSummary struct {
	RunID    string `json:"run_id"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Status   string `json:"status"`
}

func BuildAutonomyReadinessReport(inputDir string, options AutonomyReadinessOptions) (AutonomyReadinessReport, error) {
	root, err := resolveSemanticJudgmentRoot(inputDir)
	if err != nil {
		return AutonomyReadinessReport{}, err
	}
	summary, err := readSemanticJudgmentSummary(root)
	if err != nil {
		return AutonomyReadinessReport{}, err
	}
	items, judgments, err := readSemanticJudgmentBundle(root, summary)
	if err != nil {
		return AutonomyReadinessReport{}, err
	}
	if options.Threshold == 0 {
		options.Threshold = AutonomyReadinessDefaultThreshold
	}
	if math.IsNaN(options.Threshold) || math.IsInf(options.Threshold, 0) || options.Threshold <= 0 || options.Threshold > 1 {
		return AutonomyReadinessReport{}, fmt.Errorf("threshold must be >0 and <=1")
	}
	trace := readAutonomyReadinessTrace(root)
	counts := autonomyCounts(summary, items, judgments)
	safetyCounters, err := autonomySafetyCounters(root)
	if err != nil {
		return AutonomyReadinessReport{}, err
	}
	report := AutonomyReadinessReport{
		SchemaVersion:    AutonomyReadinessReportSchemaVersion,
		EvaluatorVersion: AutonomyReadinessEvaluatorVersion,
		SuiteID:          autonomySuiteID(summary),
		SourceRunIDs:     autonomySourceRunIDs(summary, trace),
		HeldOut:          options.HeldOut,
		Threshold:        options.Threshold,
		Accuracy:         autonomyAccuracy(counts),
		Counts:           counts,
		SafetyCounters:   safetyCounters,
		Slices:           autonomySlices(summary, items, trace),
		Projection: AutonomyReadinessProjectionReport{
			Status: AutonomyReadinessProjectionDisabled,
		},
	}
	report.Blockers = autonomyBlockers(report, summary)
	report.ThresholdStatus = AutonomyReadinessNotEligible
	if len(report.Blockers) == 0 {
		report.ThresholdStatus = AutonomyReadinessEligible
	}
	report.KRs = autonomyKRs(report, summary)
	report.Improvement = autonomyImprovementTargets(report, summary)
	return report, nil
}

func WithAutonomyReadinessProjection(report AutonomyReadinessReport, projection AutonomyReadinessProjectionReport) AutonomyReadinessReport {
	report.Projection = projection
	return report
}

func autonomyCounts(summary SemanticJudgmentSummary, items []SemanticJudgmentCandidate, judgments []SemanticJudgmentRecord) AutonomyReadinessCounts {
	evalCounts := autonomyEvalCountedOutcomeCounts(items, judgments)
	return AutonomyReadinessCounts{
		SourceCount:                   summary.SourceCount,
		CandidateCount:                summary.CandidateCount,
		JudgedCount:                   summary.JudgedCount,
		RemainingCount:                summary.RemainingCount,
		AcceptedCount:                 summary.AcceptedCount,
		RejectedCount:                 summary.RejectedCount,
		UnclearCount:                  summary.UnclearCount,
		DuplicateCount:                summary.DuplicateCount,
		WrongKindCount:                summary.WrongKindCount,
		FalsePositiveCount:            autonomyFalsePositiveCount(summary),
		FalseNegativeCount:            autonomyFalseNegativeCount(summary),
		EvalCountedAcceptedCount:      evalCounts.accepted,
		EvalCountedFalsePositiveCount: evalCounts.falsePositive,
		EvalCountedFalseNegativeCount: evalCounts.falseNegative,
		EvalCountedUnclearCount:       evalCounts.unclear,
		EvalCountedRemainingCount:     autonomyEvalCountedRemainingCount(summary),
		BlockedCount:                  summary.BlockedCount,
		SkippedCount:                  summary.SkippedCount,
		EvalCountedCount:              summary.EvalCountedCount,
		EvidenceReadyCount:            summary.EvidenceReadyCount,
		EvidenceExcludedCount:         summary.EvidenceExcludedCount,
		HumanReviewRequiredCount:      summary.HumanReviewRequiredCount,
		EvalCountedHumanReviewCount:   autonomyEvalCountedHumanReviewRequiredCount(summary),
		MachineTriagedCount:           summary.MachineTriagedCount,
		AgentReviewedCount:            summary.AgentReviewedCount,
		ReviewBurdenCount:             summary.ReviewBurdenCount,
		ModelErrorCount:               autonomyModelErrorCount(items),
		EvalCountedModelErrorCount:    autonomyEvalCountedModelErrorCount(items),
	}
}

func autonomyAccuracy(counts AutonomyReadinessCounts) float64 {
	denominator := autonomyAccuracyDenominator(counts)
	if denominator == 0 {
		return 0
	}
	return float64(counts.EvalCountedAcceptedCount) / float64(denominator)
}

func autonomySlices(summary SemanticJudgmentSummary, items []SemanticJudgmentCandidate, trace autonomyReadinessTraceSummary) AutonomyReadinessSlices {
	return AutonomyReadinessSlices{
		BySourceDocument:          stringChoiceMap(summary.JudgmentBySourceDocument),
		BySourceType:              autonomySourceTypeSlice(summary.Candidates, items),
		ByCandidateKind:           candidateKindChoiceMap(summary.JudgmentByCandidateKind),
		ByConfidence:              confidenceChoiceMap(summary.JudgmentByConfidence),
		ByReviewStatus:            reviewStatusChoiceMap(summary.JudgmentByReviewStatus),
		ByRelationPresence:        stringChoiceMap(summary.JudgmentByRelationPresence),
		ByRelationType:            relationTypeChoiceMap(summary.JudgmentByRelationType),
		ByFailureReason:           failureReasonChoiceMap(summary.JudgmentByFailureReason),
		ByEvidenceReadinessReason: readinessReasonCounts(summary.EvidenceReadinessReasonCounts),
		ByProviderModel:           autonomyProviderModelSlice(trace),
		ByRunStatus:               autonomyRunStatusSlice(trace),
	}
}

func autonomyBlockers(report AutonomyReadinessReport, summary SemanticJudgmentSummary) []string {
	var blockers []string
	if !report.HeldOut {
		blockers = append(blockers, "not_held_out")
	}
	if autonomyAccuracyDenominator(report.Counts) == 0 {
		blockers = append(blockers, "no_judged_eval_outcomes")
	} else if report.Accuracy < report.Threshold {
		blockers = append(blockers, "below_threshold")
	}
	if report.Counts.EvalCountedCount != report.Counts.EvidenceReadyCount {
		blockers = append(blockers, "evidence_readiness_below_100_percent")
	}
	if nonAcceptCount(summary) > taxonomyCount(summary) {
		blockers = append(blockers, "failure_taxonomy_incomplete")
	}
	if report.Counts.EvalCountedRemainingCount > 0 {
		blockers = append(blockers, "remaining_judgments")
	}
	if report.Counts.EvalCountedHumanReviewCount > 0 {
		blockers = append(blockers, "human_review_required")
	}
	if report.Counts.EvalCountedModelErrorCount > 0 {
		blockers = append(blockers, "model_errors")
	}
	if report.SafetyCounters.DestinationWrites != 0 ||
		report.SafetyCounters.AutoAccepts != 0 ||
		report.SafetyCounters.NoHumanClaims != 0 ||
		report.SafetyCounters.CommittedPrivateArtifacts != 0 {
		blockers = append(blockers, "safety_counter_nonzero")
	}
	sort.Strings(blockers)
	return blockers
}

func autonomyKRs(report AutonomyReadinessReport, summary SemanticJudgmentSummary) map[string]AutonomyReadinessKR {
	taxonomyPassed := nonAcceptCount(summary) == taxonomyCount(summary)
	evidencePassed := report.Counts.EvalCountedCount == report.Counts.EvidenceReadyCount
	slicesPassed := report.Slices.BySourceDocument != nil &&
		report.Slices.BySourceType != nil &&
		report.Slices.ByCandidateKind != nil &&
		report.Slices.ByRelationPresence != nil &&
		report.Slices.ByFailureReason != nil &&
		report.Slices.ByEvidenceReadinessReason != nil &&
		report.Slices.ByProviderModel != nil &&
		report.Slices.ByRunStatus != nil
	safetyPassed := report.SafetyCounters.DestinationWrites == 0 &&
		report.SafetyCounters.AutoAccepts == 0 &&
		report.SafetyCounters.NoHumanClaims == 0 &&
		report.SafetyCounters.CommittedPrivateArtifacts == 0
	return map[string]AutonomyReadinessKR{
		"KEY-3": kr(report.HeldOut && report.Accuracy >= report.Threshold, fmt.Sprintf("%.4f held_out=%t", report.Accuracy, report.HeldOut), fmt.Sprintf(">=%.2f held_out=true", report.Threshold)),
		"KEY-4": kr(taxonomyPassed, fmt.Sprintf("%d/%d", taxonomyCount(summary), nonAcceptCount(summary)), "100% non-accept failures classified"),
		"KEY-5": kr(slicesPassed, "required slices present", "all required slices present"),
		"KEY-6": kr(safetyPassed, "0 writes/auto-accept/no-human/private-artifacts", "0 writes/auto-accept/no-human/private-artifacts"),
		"KEY-7": kr(evidencePassed, fmt.Sprintf("%d/%d", report.Counts.EvidenceReadyCount, report.Counts.EvalCountedCount), "100% eval-counted evidence-ready"),
	}
}

func kr(passed bool, current, target string) AutonomyReadinessKR {
	status := "fail"
	if passed {
		status = "pass"
	}
	return AutonomyReadinessKR{Status: status, Passed: passed, Current: current, Target: target}
}

func autonomyImprovementTargets(report AutonomyReadinessReport, summary SemanticJudgmentSummary) []AutonomyReadinessImprovement {
	var targets []AutonomyReadinessImprovement
	add := func(code string, count int, text string) {
		if count <= 0 {
			return
		}
		targets = append(targets, AutonomyReadinessImprovement{
			Code:    code,
			Count:   count,
			Summary: text,
			Refs:    autonomyRefs(summary),
		})
	}
	if !report.HeldOut {
		add("not_held_out", 1, "Promote reviewed examples into a declared held-out suite before making autonomy claims.")
	}
	denominator := autonomyAccuracyDenominator(report.Counts)
	if denominator == 0 && summary.SkippedReason == "" {
		add("no_judged_eval_outcomes", 1, "Create authoritative judgments for eval-counted candidates before readiness can be assessed.")
	} else if denominator > 0 && report.Accuracy < report.Threshold {
		add("below_threshold", autonomyBelowThresholdGapCount(report), "Improve extraction and judgment quality before DEC-64 eligibility.")
	}
	if report.Counts.CandidateCount == 0 && summary.SkippedReason == "" {
		add("no_candidates", 1, "Fix extraction/classification coverage for sources that produce no semantic candidates.")
	}
	add("evidence_readiness", report.Counts.EvidenceExcludedCount, "Make excluded candidates evidence-ready or remove them from counted evaluation.")
	add("taxonomy_coverage", nonAcceptCount(summary)-taxonomyCount(summary), "Add stable failure reasons for every non-accept judgment.")
	add("human_review_required", report.Counts.EvalCountedHumanReviewCount, "Reduce the eval-counted candidate set that requires human review.")
	add("model_errors", report.Counts.EvalCountedModelErrorCount, "Fix model-error paths before trusting eval-counted agent review.")
	add("review_burden", report.Counts.ReviewBurdenCount, "Lower the remaining review burden through better confidence and evidence gates.")
	add("wrong_kind", report.Counts.WrongKindCount, "Tighten candidate-kind classification.")
	add("duplicate", report.Counts.DuplicateCount, "Improve deduplication before readiness claims.")
	add("unclear", report.Counts.UnclearCount, "Improve evidence/context packaging for ambiguous candidates.")
	add("false_positive", report.Counts.FalsePositiveCount, "Reduce incorrect extracted candidates before readiness claims.")
	add("false_negative", report.Counts.FalseNegativeCount, "Recover expected outcomes that the extractor missed.")
	add("remaining_judgments", report.Counts.EvalCountedRemainingCount, "Finish the eval-counted judgment queue or exclude unjudged items from the held-out suite.")
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].Count == targets[j].Count {
			return targets[i].Code < targets[j].Code
		}
		return targets[i].Count > targets[j].Count
	})
	if len(targets) > 5 {
		targets = autonomyLimitImprovementTargets(targets, 5, "below_threshold")
	}
	if targets == nil {
		return []AutonomyReadinessImprovement{}
	}
	return targets
}

func autonomyLimitImprovementTargets(targets []AutonomyReadinessImprovement, limit int, requiredCode string) []AutonomyReadinessImprovement {
	if limit <= 0 || len(targets) <= limit {
		return targets
	}
	limited := append([]AutonomyReadinessImprovement(nil), targets[:limit]...)
	if improvementTargetsContain(limited, requiredCode) {
		return limited
	}
	for _, target := range targets[limit:] {
		if target.Code == requiredCode {
			limited[len(limited)-1] = target
			return limited
		}
	}
	return limited
}

func improvementTargetsContain(targets []AutonomyReadinessImprovement, code string) bool {
	for _, target := range targets {
		if target.Code == code {
			return true
		}
	}
	return false
}

func autonomyBelowThresholdGapCount(report AutonomyReadinessReport) int {
	if report.Accuracy >= report.Threshold {
		return 0
	}
	count := int(math.Ceil((report.Threshold - report.Accuracy) * 10000))
	if count < 1 {
		return 1
	}
	return count
}

func autonomyAccuracyDenominator(counts AutonomyReadinessCounts) int {
	return counts.EvalCountedAcceptedCount + counts.EvalCountedFalsePositiveCount + counts.EvalCountedFalseNegativeCount + counts.EvalCountedUnclearCount
}

func autonomySourceTypeSlice(summaries []SemanticJudgmentCandidateSummary, items []SemanticJudgmentCandidate) map[string]map[string]int {
	out := map[string]map[string]int{}
	itemByID := map[string]SemanticJudgmentCandidate{}
	for _, item := range items {
		itemByID[item.CandidateID] = item
	}
	for _, summary := range summaries {
		sourceType := inferAutonomySourceType(summary, itemByID[summary.CandidateID])
		choice := summary.JudgmentChoice
		if choice == "" {
			choice = "unjudged"
		}
		incrementChoice(out, sourceType, string(choice))
	}
	return out
}

func inferAutonomySourceType(summary SemanticJudgmentCandidateSummary, item SemanticJudgmentCandidate) string {
	probe := strings.ToLower(summary.SourceDocumentID + " " + item.SourceDocumentID)
	for _, excerpt := range item.EvidenceExcerpts {
		probe += " " + strings.ToLower(excerpt.SourceLabel)
	}
	switch {
	case strings.Contains(probe, "transcript"):
		return "transcript"
	case strings.Contains(probe, "notion"):
		return "notion_doc"
	case strings.Contains(probe, "slack"):
		return "message_thread"
	case strings.TrimSpace(probe) == "":
		return "unknown"
	default:
		return "markdown_document"
	}
}

func autonomyProviderModelSlice(trace autonomyReadinessTraceSummary) map[string]int {
	key := "deterministic_or_unknown"
	if strings.TrimSpace(trace.Provider) != "" || strings.TrimSpace(trace.Model) != "" {
		key = strings.TrimSpace(trace.Provider) + "/" + strings.TrimSpace(trace.Model)
	}
	return map[string]int{key: 1}
}

func autonomyRunStatusSlice(trace autonomyReadinessTraceSummary) map[string]int {
	status := strings.TrimSpace(trace.Status)
	if status == "" {
		status = "unknown"
	}
	return map[string]int{status: 1}
}

func readAutonomyReadinessTrace(root string) autonomyReadinessTraceSummary {
	path := filepath.Join(filepath.Dir(root), "trace", "trace-summary.json")
	var trace autonomyReadinessTraceSummary
	_ = readJSONFile(path, &trace)
	return trace
}

func autonomySuiteID(summary SemanticJudgmentSummary) string {
	return "suite-" + shortStableID(summary.RunID)
}

func shortStableID(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:16]
}

func autonomySourceRunIDs(summary SemanticJudgmentSummary, trace autonomyReadinessTraceSummary) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range []string{summary.RunID, trace.RunID} {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func autonomyRefs(summary SemanticJudgmentSummary) []string {
	var refs []string
	if summary.ReportPath != "" {
		refs = append(refs, summary.ReportPath)
	}
	if summary.CursorPath != "" {
		refs = append(refs, summary.CursorPath)
	}
	sort.Strings(refs)
	return refs
}

func nonAcceptCount(summary SemanticJudgmentSummary) int {
	return summary.RejectedCount + summary.UnclearCount + summary.DuplicateCount + summary.WrongKindCount
}

func taxonomyCount(summary SemanticJudgmentSummary) int {
	total := 0
	for _, count := range summary.FailureReasonCounts {
		total += count
	}
	return total
}

func autonomyFalsePositiveCount(summary SemanticJudgmentSummary) int {
	if len(summary.Candidates) > 0 {
		count := 0
		for _, candidate := range summary.Candidates {
			if autonomyHasFailureReason(candidate.FailureReason, candidate.SecondaryFailureReasons, SemanticFailureMissingExpectedOutcome) {
				continue
			}
			switch candidate.JudgmentChoice {
			case SemanticJudgmentChoiceReject, SemanticJudgmentChoiceDuplicate, SemanticJudgmentChoiceWrongKind:
				count++
			}
		}
		return count
	}
	return summary.RejectedCount + summary.DuplicateCount + summary.WrongKindCount
}

func autonomyFalseNegativeCount(summary SemanticJudgmentSummary) int {
	if len(summary.Candidates) > 0 {
		count := 0
		for _, candidate := range summary.Candidates {
			if autonomyHasFailureReason(candidate.FailureReason, candidate.SecondaryFailureReasons, SemanticFailureMissingExpectedOutcome) {
				count++
			}
		}
		return count
	}
	return summary.FailureReasonCounts[SemanticFailureMissingExpectedOutcome]
}

func autonomyEvalCountedRemainingCount(summary SemanticJudgmentSummary) int {
	if len(summary.Candidates) == 0 {
		return summary.RemainingCount
	}
	count := 0
	for _, candidate := range summary.Candidates {
		if candidate.EvalCounted && candidate.JudgmentChoice == "" {
			count++
		}
	}
	return count
}

func autonomyEvalCountedHumanReviewRequiredCount(summary SemanticJudgmentSummary) int {
	count := 0
	for _, candidate := range summary.Candidates {
		if !candidate.EvalCounted || candidate.HumanReviewRequired == nil || !*candidate.HumanReviewRequired {
			continue
		}
		count++
	}
	return count
}

type autonomyEvalOutcomeCounts struct {
	accepted      int
	falsePositive int
	falseNegative int
	unclear       int
}

func autonomyEvalCountedOutcomeCounts(items []SemanticJudgmentCandidate, judgments []SemanticJudgmentRecord) autonomyEvalOutcomeCounts {
	evalCounted := map[string]bool{}
	for _, item := range items {
		evalCounted[item.CandidateID] = item.EvidenceReadiness.EvalCounted
	}
	var counts autonomyEvalOutcomeCounts
	for _, judgment := range judgments {
		if !evalCounted[judgment.CandidateID] {
			continue
		}
		if autonomyHasFailureReason(judgment.FailureReason, judgment.SecondaryReasons, SemanticFailureMissingExpectedOutcome) {
			counts.falseNegative++
			continue
		}
		switch judgment.Choice {
		case SemanticJudgmentChoiceAccept:
			counts.accepted++
		case SemanticJudgmentChoiceUnclear:
			counts.unclear++
		case SemanticJudgmentChoiceReject, SemanticJudgmentChoiceDuplicate, SemanticJudgmentChoiceWrongKind:
			counts.falsePositive++
		}
	}
	return counts
}

func autonomyHasFailureReason(primary SemanticFailureReason, secondary []SemanticFailureReason, target SemanticFailureReason) bool {
	if primary == target {
		return true
	}
	for _, reason := range secondary {
		if reason == target {
			return true
		}
	}
	return false
}

func autonomyModelErrorCount(items []SemanticJudgmentCandidate) int {
	count := 0
	for _, item := range items {
		if item.AgentReview == nil {
			continue
		}
		for _, reason := range item.AgentReview.ReviewReasonCodes {
			if reason == SemanticAgentReviewReasonModelError {
				count++
				break
			}
		}
	}
	return count
}

func autonomyEvalCountedModelErrorCount(items []SemanticJudgmentCandidate) int {
	count := 0
	for _, item := range items {
		readiness := item.EvidenceReadiness
		if readiness.Status == "" {
			readiness = semanticLegacyEvidenceReadiness(item)
		}
		if !readiness.EvalCounted || item.AgentReview == nil {
			continue
		}
		for _, reason := range item.AgentReview.ReviewReasonCodes {
			if reason == SemanticAgentReviewReasonModelError {
				count++
				break
			}
		}
	}
	return count
}

func autonomySafetyCounters(root string) (AutonomyReadinessSafetyCounters, error) {
	artifactRoot := filepath.Dir(root)
	var counters AutonomyReadinessSafetyCounters
	err := filepath.WalkDir(artifactRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("scan safety artifact %s: %w", path, err)
		}
		if entry.IsDir() {
			if entry.Name() == AutonomyReadinessDirName {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || filepath.Ext(path) != ".json" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("scan safety artifact %s: %w", path, err)
		}
		var value any
		if err := json.Unmarshal(body, &value); err != nil {
			return fmt.Errorf("scan safety artifact %s: %w", path, err)
		}
		counters = addSafetyCounters(counters, autonomySafetyCountersFromArtifact(value))
		return nil
	})
	return counters, err
}

func addSafetyCounters(left, right AutonomyReadinessSafetyCounters) AutonomyReadinessSafetyCounters {
	return AutonomyReadinessSafetyCounters{
		DestinationWrites:         left.DestinationWrites + right.DestinationWrites,
		AutoAccepts:               left.AutoAccepts + right.AutoAccepts,
		NoHumanClaims:             left.NoHumanClaims + right.NoHumanClaims,
		CommittedPrivateArtifacts: left.CommittedPrivateArtifacts + right.CommittedPrivateArtifacts,
	}
}

func autonomySafetyCountersFromArtifact(value any) AutonomyReadinessSafetyCounters {
	counters := AutonomyReadinessSafetyCounters{
		DestinationWrites: autonomyDestinationWriteCount(value),
		AutoAccepts:       autonomyAutoAcceptCount(value),
		NoHumanClaims:     autonomyNoHumanClaimCount(value),
	}
	if autonomyHasPrivateSafetyMarker(value) {
		counters.CommittedPrivateArtifacts = 1
	}
	return counters
}

func autonomyDestinationWriteCount(value any) int {
	object, ok := value.(map[string]any)
	if !ok {
		return 0
	}
	if stringField(object, "schema_version") != "destination-operation/v0.1" {
		return 0
	}
	switch stringField(object, "operation_type") {
	case "create_note", "attention_preview", "background_record":
		return 1
	default:
		return 0
	}
}

func autonomyAutoAcceptCount(value any) int {
	return countJSONValue(value, func(key string, value any) int {
		switch key {
		case "auto_accept", "auto_accepted":
			if boolField(value) {
				return 1
			}
		case "auto_accept_count", "auto_accepted_count", "auto_accepts":
			return nonNegativeIntField(value)
		}
		return 0
	})
}

func autonomyNoHumanClaimCount(value any) int {
	return countJSONValue(value, func(key string, value any) int {
		if key == "no_human_eligible" && boolField(value) {
			return 1
		}
		return 0
	})
}

func autonomyHasPrivateSafetyMarker(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if safety, ok := typed["safety"].(map[string]any); ok {
			if boolField(safety["private_provenance"]) ||
				boolField(safety["redaction_required"]) ||
				boolField(safety["secret_like"]) {
				return true
			}
		}
		for _, child := range typed {
			if autonomyHasPrivateSafetyMarker(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if autonomyHasPrivateSafetyMarker(child) {
				return true
			}
		}
	}
	return false
}

func countJSONValue(value any, match func(string, any) int) int {
	switch typed := value.(type) {
	case map[string]any:
		count := 0
		for key, child := range typed {
			count += match(key, child)
			count += countJSONValue(child, match)
		}
		return count
	case []any:
		count := 0
		for _, child := range typed {
			count += countJSONValue(child, match)
		}
		return count
	default:
		return 0
	}
}

func stringField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return strings.TrimSpace(value)
}

func boolField(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func nonNegativeIntField(value any) int {
	number, ok := value.(float64)
	if !ok || number <= 0 {
		return 0
	}
	return int(math.Floor(number))
}

func incrementChoice(out map[string]map[string]int, key, choice string) {
	if key == "" {
		key = "unknown"
	}
	if choice == "" {
		choice = "none"
	}
	if out[key] == nil {
		out[key] = map[string]int{}
	}
	out[key][choice]++
}

func stringChoiceMap(input map[string]map[SemanticJudgmentChoice]int) map[string]map[string]int {
	out := map[string]map[string]int{}
	for key, choices := range input {
		out[key] = semanticChoiceCounts(choices)
	}
	return out
}

func candidateKindChoiceMap(input map[SemanticCandidateKind]map[SemanticJudgmentChoice]int) map[string]map[string]int {
	out := map[string]map[string]int{}
	for key, choices := range input {
		out[string(key)] = semanticChoiceCounts(choices)
	}
	return out
}

func confidenceChoiceMap(input map[Confidence]map[SemanticJudgmentChoice]int) map[string]map[string]int {
	out := map[string]map[string]int{}
	for key, choices := range input {
		out[string(key)] = semanticChoiceCounts(choices)
	}
	return out
}

func reviewStatusChoiceMap(input map[ReviewStatus]map[SemanticJudgmentChoice]int) map[string]map[string]int {
	out := map[string]map[string]int{}
	for key, choices := range input {
		out[string(key)] = semanticChoiceCounts(choices)
	}
	return out
}

func relationTypeChoiceMap(input map[SemanticRelationshipType]map[SemanticJudgmentChoice]int) map[string]map[string]int {
	out := map[string]map[string]int{}
	for key, choices := range input {
		out[string(key)] = semanticChoiceCounts(choices)
	}
	return out
}

func failureReasonChoiceMap(input map[SemanticFailureReason]map[SemanticJudgmentChoice]int) map[string]map[string]int {
	out := map[string]map[string]int{}
	for key, choices := range input {
		out[string(key)] = semanticChoiceCounts(choices)
	}
	return out
}

func semanticChoiceCounts(input map[SemanticJudgmentChoice]int) map[string]int {
	out := map[string]int{}
	for key, value := range input {
		out[string(key)] = value
	}
	return out
}

func readinessReasonCounts(input map[SemanticEvidenceReadinessReason]int) map[string]int {
	out := map[string]int{}
	for key, value := range input {
		out[string(key)] = value
	}
	return out
}
