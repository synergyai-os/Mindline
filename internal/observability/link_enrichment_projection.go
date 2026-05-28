package observability

import (
	"strings"

	"github.com/synergyai-os/Mindline/internal/documents"
)

const LinkEnrichmentProjectionSchemaVersion = "mindline-link-enrichment-eval-projection/v0.1"

const LinkEnrichmentEvaluationName = "mindline.link_enrichment.covered_missingness_reduction"

type LinkEnrichmentProjection struct {
	SchemaVersion string      `json:"schema_version"`
	Status        string      `json:"status"`
	ErrorClass    string      `json:"error_class,omitempty"`
	Events        []SafeEvent `json:"events"`
}

func LinkEnrichmentSafeEvents(summary documents.LinkEnrichmentLoopSummary, telemetrySalt string) []SafeEvent {
	comparison := summary.Comparison
	requests := summary.RequestSummary
	passed := linkEnrichmentEvalPassed(summary)
	props := map[string]any{
		"event_schema":                       LinkEnrichmentProjectionSchemaVersion,
		"feature":                            "mindline.link_enrichment_loop",
		"workflow":                           "link_enrichment_loop",
		"command":                            "documents link-enrichment-loop",
		"status":                             string(comparison.Verdict),
		"validation_result":                  linkEnrichmentValidationResult(passed),
		"evaluation_result":                  linkEnrichmentValidationResult(passed),
		"$ai_evaluation_name":                LinkEnrichmentEvaluationName,
		"$ai_evaluation_result":              passed,
		"$ai_evaluation_reasoning":           linkEnrichmentReasoning(summary),
		"input_redacted":                     true,
		"output_redacted":                    true,
		"source_redacted":                    true,
		"metadata_only":                      true,
		"projection_schema":                  LinkEnrichmentProjectionSchemaVersion,
		"run_id":                             safeLinkEnrichmentRunID(summary.CorpusID, telemetrySalt),
		"source_count":                       requests.SourceCount,
		"url_mention_count":                  requests.URLMentionCount,
		"unique_url_count":                   requests.UniqueURLCount,
		"accounted_url_count":                requests.AccountedURLCount,
		"requestable_count":                  requests.RequestableCount,
		"already_artifacted_count":           requests.AlreadyArtifactedCount,
		"unsupported_count":                  requests.UnsupportedCount,
		"blocked_private_or_secret_count":    requests.BlockedPrivateCount,
		"blocked_by_policy_count":            requests.BlockedPolicyCount,
		"supplied_artifact_count":            requests.SuppliedArtifactCount,
		"matched_artifact_count":             requests.MatchedArtifactCount,
		"stale_artifact_count":               requests.StaleArtifactCount,
		"non_generalizable_runtime":          requests.NonGeneralizableRuntime,
		"url_accounting_coverage":            requests.URLAccountingCoverage,
		"artifact_match_coverage":            requests.ArtifactMatchCoverage,
		"comparable":                         comparison.Comparable,
		"baseline_needs_enrichment_count":    comparison.BaselineRoutingCounts[documents.SourceMeaningRoutingNeedsEnrichment],
		"enriched_needs_enrichment_count":    comparison.EnrichedRoutingCounts[documents.SourceMeaningRoutingNeedsEnrichment],
		"baseline_missing_link_count":        comparison.BaselineMissingnessCounts[documents.SourceMeaningMissingnessMissingLinkEnrichment],
		"enriched_missing_link_count":        comparison.EnrichedMissingnessCounts[documents.SourceMeaningMissingnessMissingLinkEnrichment],
		"missing_link_reduction_ratio":       comparison.MissingLinkReductionRatio,
		"needs_enrichment_reduction_ratio":   comparison.NeedsEnrichmentReductionRatio,
		"safety_hosted_inference_calls":      comparison.Guardrails.HostedInferenceCalls,
		"safety_hosted_telemetry_exports":    comparison.Guardrails.HostedTelemetryExports,
		"safety_network_fetches":             comparison.Guardrails.NetworkFetches,
		"safety_browser_calls":               comparison.Guardrails.BrowserCalls,
		"safety_slack_api_calls":             comparison.Guardrails.SlackAPICalls,
		"safety_destination_writes":          comparison.Guardrails.DestinationWrites,
		"safety_auto_accepts":                0,
		"safety_no_human_claims":             true,
		"safety_committed_private_artifacts": 0,
		"kr_key_6":                           linkEnrichmentSafetyPassed(summary),
		"kr_key_7":                           linkEnrichmentEvidencePassed(summary),
	}
	return []SafeEvent{{
		Event:      "$ai_evaluation",
		DistinctID: "mindline-link-enrichment-" + contentHash("link_enrichment.distinct:"+strings.TrimSpace(telemetrySalt)),
		TraceID:    safeLinkEnrichmentTraceID(summary.CorpusID, telemetrySalt),
		Properties: props,
	}}
}

func linkEnrichmentEvalPassed(summary documents.LinkEnrichmentLoopSummary) bool {
	return summary.Comparison.Verdict == documents.LinkEnrichmentVerdictImproved &&
		summary.Comparison.Comparable &&
		summary.RequestSummary.URLAccountingCoverage == 1 &&
		summary.RequestSummary.ArtifactMatchCoverage == 1 &&
		summary.Comparison.MissingLinkReductionRatio >= 0.98 &&
		summary.Comparison.NeedsEnrichmentReductionRatio >= 0.98 &&
		linkEnrichmentSafetyPassed(summary)
}

func linkEnrichmentSafetyPassed(summary documents.LinkEnrichmentLoopSummary) bool {
	guardrails := summary.Comparison.Guardrails
	return guardrails.HostedInferenceCalls == 0 &&
		guardrails.HostedTelemetryExports == 0 &&
		guardrails.NetworkFetches == 0 &&
		guardrails.BrowserCalls == 0 &&
		guardrails.SlackAPICalls == 0 &&
		guardrails.DestinationWrites == 0 &&
		guardrails.ProductBrainWrites == 0 &&
		guardrails.TolariaWrites == 0
}

func linkEnrichmentEvidencePassed(summary documents.LinkEnrichmentLoopSummary) bool {
	return summary.RequestSummary.URLAccountingCoverage == 1 &&
		summary.RequestSummary.ArtifactMatchCoverage == 1 &&
		summary.Comparison.MissingLinkReductionRatio >= 0.98 &&
		summary.Comparison.NeedsEnrichmentReductionRatio >= 0.98
}

func linkEnrichmentValidationResult(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}

func linkEnrichmentReasoning(summary documents.LinkEnrichmentLoopSummary) string {
	reasons := []string{}
	if !summary.Comparison.Comparable {
		reasons = append(reasons, "not_comparable")
	}
	if summary.RequestSummary.URLAccountingCoverage < 1 {
		reasons = append(reasons, "incomplete_url_accounting")
	}
	if summary.RequestSummary.ArtifactMatchCoverage < 1 {
		reasons = append(reasons, "incomplete_artifact_match")
	}
	if summary.Comparison.MissingLinkReductionRatio < 0.98 {
		reasons = append(reasons, "missing_link_reduction_below_kr")
	}
	if summary.Comparison.NeedsEnrichmentReductionRatio < 0.98 {
		reasons = append(reasons, "needs_enrichment_reduction_below_kr")
	}
	if !linkEnrichmentSafetyPassed(summary) {
		reasons = append(reasons, "guardrail_violation")
	}
	if len(reasons) == 0 {
		return "passed_link_enrichment_kr"
	}
	return strings.Join(reasons, ",")
}

func safeLinkEnrichmentRunID(corpusID, telemetrySalt string) string {
	return "run-" + contentHash("link_enrichment.run:"+strings.TrimSpace(telemetrySalt)+":"+strings.TrimSpace(corpusID))
}

func safeLinkEnrichmentTraceID(corpusID, telemetrySalt string) string {
	return "trace-" + contentHash("link_enrichment.trace:"+strings.TrimSpace(telemetrySalt)+":"+strings.TrimSpace(corpusID))
}
