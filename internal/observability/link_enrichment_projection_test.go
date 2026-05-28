package observability

import (
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestLinkEnrichmentSafeEventsUsePostHogEvaluationMetadata(t *testing.T) {
	event := LinkEnrichmentSafeEvents(linkEnrichmentProjectionFixture(), "telemetry-salt")[0]
	if event.Event != "$ai_evaluation" {
		t.Fatalf("expected PostHog evaluation event, got %s", event.Event)
	}
	if err := ValidateSafeEvent(event); err != nil {
		t.Fatalf("event should validate: %v", err)
	}
	if event.Properties["$ai_evaluation_name"] != LinkEnrichmentEvaluationName ||
		event.Properties["$ai_evaluation_result"] != true ||
		event.Properties["$ai_evaluation_reasoning"] != "passed_link_enrichment_kr" ||
		event.Properties["metadata_only"] != true ||
		event.Properties["missing_link_reduction_ratio"] != 1.0 ||
		event.Properties["needs_enrichment_reduction_ratio"] != 1.0 ||
		event.Properties["safety_network_fetches"] != 0 {
		t.Fatalf("missing expected eval metadata: %+v", event.Properties)
	}
	for key, value := range event.Properties {
		if strings.Contains(strings.ToLower(key), "source_text") || strings.Contains(strings.ToLower(key), "prompt") {
			t.Fatalf("unsafe key exported: %s", key)
		}
		if s, ok := value.(string); ok && strings.Contains(s, "https://") {
			t.Fatalf("unsafe URL value exported: %s=%s", key, s)
		}
	}
}

func TestLinkEnrichmentSafeEventsFailKRWhenGuardrailRegresses(t *testing.T) {
	summary := linkEnrichmentProjectionFixture()
	summary.Comparison.Guardrails.NetworkFetches = 1
	event := LinkEnrichmentSafeEvents(summary, "telemetry-salt")[0]
	if event.Properties["$ai_evaluation_result"] != false ||
		event.Properties["$ai_evaluation_reasoning"] != "guardrail_violation" {
		t.Fatalf("expected failed guardrail evaluation: %+v", event.Properties)
	}
}

func linkEnrichmentProjectionFixture() documents.LinkEnrichmentLoopSummary {
	return documents.LinkEnrichmentLoopSummary{
		CorpusID: "corpus-private-name",
		RequestSummary: documents.LinkArtifactRequestSummary{
			SourceCount:            2,
			URLMentionCount:        2,
			UniqueURLCount:         2,
			AccountedURLCount:      2,
			AlreadyArtifactedCount: 2,
			SuppliedArtifactCount:  2,
			MatchedArtifactCount:   2,
			URLAccountingCoverage:  1,
			ArtifactMatchCoverage:  1,
		},
		Comparison: documents.LinkEnrichmentComparisonSummary{
			Verdict:                       documents.LinkEnrichmentVerdictImproved,
			Comparable:                    true,
			MissingLinkReductionRatio:     1,
			NeedsEnrichmentReductionRatio: 1,
			BaselineMissingnessCounts: map[documents.SourceMeaningPreviewMissingness]int{
				documents.SourceMeaningMissingnessMissingLinkEnrichment: 2,
			},
			EnrichedMissingnessCounts: map[documents.SourceMeaningPreviewMissingness]int{
				documents.SourceMeaningMissingnessNone: 2,
			},
			BaselineRoutingCounts: map[documents.SourceMeaningPreviewRoutingHint]int{
				documents.SourceMeaningRoutingNeedsEnrichment: 2,
			},
			EnrichedRoutingCounts: map[documents.SourceMeaningPreviewRoutingHint]int{
				documents.SourceMeaningRoutingTolariaCandidate: 2,
			},
		},
	}
}
