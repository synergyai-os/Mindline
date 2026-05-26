package observability

import (
	"sort"
	"strings"

	"github.com/synergyai-os/Mindline/internal/documents"
)

const AutonomyReadinessProjectionSchemaVersion = "mindline-autonomy-readiness-projection/v0.1"

func AutonomyReadinessSafeEvents(report documents.AutonomyReadinessReport, telemetrySalt string) []SafeEvent {
	props := map[string]any{
		"event_schema":                       AutonomyReadinessProjectionSchemaVersion,
		"feature":                            "mindline.autonomy_readiness",
		"workflow":                           "autonomy_readiness",
		"command":                            "documents readiness-report",
		"status":                             string(report.ThresholdStatus),
		"validation_result":                  string(report.ThresholdStatus),
		"evaluation_result":                  string(report.ThresholdStatus),
		"input_redacted":                     true,
		"output_redacted":                    true,
		"source_redacted":                    true,
		"metadata_only":                      true,
		"projection_schema":                  AutonomyReadinessProjectionSchemaVersion,
		"threshold":                          report.Threshold,
		"accuracy":                           report.Accuracy,
		"held_out":                           report.HeldOut,
		"run_id":                             safeRunID(report.SuiteID, telemetrySalt),
		"candidate_count":                    report.Counts.CandidateCount,
		"judged_count":                       report.Counts.JudgedCount,
		"remaining_count":                    report.Counts.RemainingCount,
		"accepted_count":                     report.Counts.AcceptedCount,
		"rejected_count":                     report.Counts.RejectedCount,
		"unclear_count":                      report.Counts.UnclearCount,
		"duplicate_count":                    report.Counts.DuplicateCount,
		"wrong_kind_count":                   report.Counts.WrongKindCount,
		"false_positive_count":               report.Counts.FalsePositiveCount,
		"false_negative_count":               report.Counts.FalseNegativeCount,
		"eval_counted_accepted_count":        report.Counts.EvalCountedAcceptedCount,
		"eval_counted_false_positive_count":  report.Counts.EvalCountedFalsePositiveCount,
		"eval_counted_false_negative_count":  report.Counts.EvalCountedFalseNegativeCount,
		"eval_counted_remaining_count":       report.Counts.EvalCountedRemainingCount,
		"evidence_ready_count":               report.Counts.EvidenceReadyCount,
		"eval_counted_count":                 report.Counts.EvalCountedCount,
		"evidence_excluded_count":            report.Counts.EvidenceExcludedCount,
		"review_burden_count":                report.Counts.ReviewBurdenCount,
		"human_review_required_count":        report.Counts.HumanReviewRequiredCount,
		propEvalHumanReviewRequired:          report.Counts.EvalCountedHumanReviewCount,
		"machine_triaged_count":              report.Counts.MachineTriagedCount,
		"agent_reviewed_count":               report.Counts.AgentReviewedCount,
		"judgment_model_errors":              report.Counts.ModelErrorCount,
		"eval_counted_model_error_count":     report.Counts.EvalCountedModelErrorCount,
		"safety_destination_writes":          report.SafetyCounters.DestinationWrites,
		"safety_auto_accepts":                report.SafetyCounters.AutoAccepts,
		"safety_no_human_claims":             report.SafetyCounters.NoHumanClaims,
		"safety_committed_private_artifacts": report.SafetyCounters.CommittedPrivateArtifacts,
		"kr_key_3":                           report.KRs["KEY-3"].Passed,
		"kr_key_4":                           report.KRs["KEY-4"].Passed,
		"kr_key_5":                           report.KRs["KEY-5"].Passed,
		"kr_key_6":                           report.KRs["KEY-6"].Passed,
		"kr_key_7":                           report.KRs["KEY-7"].Passed,
	}
	if len(report.Improvement) > 0 {
		props["recommendation"] = report.Improvement[0].Code
	}
	provider, model := firstProviderModel(report.Slices.ByProviderModel)
	if provider != "" {
		props["provider"] = provider
		props["$ai_provider"] = provider
	}
	if model != "" {
		props["model"] = model
		props["$ai_model"] = model
	}
	addSafeCounts(props, "failure_reason_count.", failureReasonCounts(report.Slices.ByFailureReason))
	addSafeCounts(props, "evidence_readiness_reason_count.", report.Slices.ByEvidenceReadinessReason)
	return []SafeEvent{{
		Event:      "$ai_feedback",
		TraceID:    safeTraceID(report.SuiteID, telemetrySalt),
		Properties: props,
	}}
}

func safeRunID(suiteID, telemetrySalt string) string {
	return "run-" + contentHash("autonomy_readiness.run:"+strings.TrimSpace(telemetrySalt)+":"+strings.TrimSpace(suiteID))
}

func safeTraceID(suiteID, telemetrySalt string) string {
	return "trace-" + contentHash("autonomy_readiness.trace:"+strings.TrimSpace(telemetrySalt)+":"+strings.TrimSpace(suiteID))
}

func firstProviderModel(counts map[string]int) (string, string) {
	values := make([]string, 0, len(counts))
	for value := range counts {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || trimmed == "deterministic_or_unknown" || !strings.Contains(trimmed, "/") {
			continue
		}
		values = append(values, trimmed)
	}
	sort.Strings(values)
	for _, value := range values {
		provider, model, _ := strings.Cut(value, "/")
		if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
			continue
		}
		return strings.TrimSpace(provider), strings.TrimSpace(model)
	}
	return "", ""
}

func addSafeCounts(props map[string]any, prefix string, counts map[string]int) {
	for key, count := range counts {
		if strings.TrimSpace(key) == "" {
			continue
		}
		props[prefix+key] = count
	}
}

func failureReasonCounts(slices map[string]map[string]int) map[string]int {
	out := map[string]int{}
	for reason, choices := range slices {
		total := 0
		for _, count := range choices {
			total += count
		}
		out[reason] = total
	}
	return out
}
