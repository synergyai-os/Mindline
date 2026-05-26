package observability

import (
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestAutonomyReadinessSafeEventsUseAllowlistedMetadataOnlyFields(t *testing.T) {
	report := documents.AutonomyReadinessReport{
		SchemaVersion:   documents.AutonomyReadinessReportSchemaVersion,
		SuiteID:         "suite-private-source-name",
		Threshold:       0.98,
		Accuracy:        0.5,
		HeldOut:         true,
		ThresholdStatus: documents.AutonomyReadinessNotEligible,
		Counts: documents.AutonomyReadinessCounts{
			CandidateCount:           2,
			JudgedCount:              2,
			RejectedCount:            1,
			EvalCountedCount:         2,
			EvidenceReadyCount:       2,
			HumanReviewRequiredCount: 1,
			ModelErrorCount:          1,
			FalsePositiveCount:       1,
			FalseNegativeCount:       0,
			EvalCountedAcceptedCount: 1,
		},
		KRs: map[string]documents.AutonomyReadinessKR{
			"KEY-3": {Passed: false},
			"KEY-4": {Passed: true},
			"KEY-5": {Passed: true},
			"KEY-6": {Passed: true},
			"KEY-7": {Passed: true},
		},
		Slices: documents.AutonomyReadinessSlices{
			ByFailureReason: map[string]map[string]int{
				"unexpected_candidate": {"reject": 1},
			},
			ByEvidenceReadinessReason: map[string]int{
				"missing_source_excerpt": 1,
			},
			ByProviderModel: map[string]int{
				"openai/gpt-5.2": 1,
			},
		},
		Improvement: []documents.AutonomyReadinessImprovement{{Code: "below_threshold", Summary: "Private raw text must not export"}},
	}

	events := AutonomyReadinessSafeEvents(report)
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	event := events[0]
	event.DistinctID = "mindline-test"
	if err := ValidateSafeEvent(event); err != nil {
		t.Fatalf("event should validate: %v", err)
	}
	if event.Properties["recommendation"] != "below_threshold" ||
		event.Properties["provider"] != "openai" ||
		event.Properties["model"] != "gpt-5.2" ||
		event.Properties["metadata_only"] != true ||
		event.Properties["false_positive_count"] != 1 ||
		event.Properties["eval_counted_accepted_count"] != 1 {
		t.Fatalf("missing expected metadata fields: %+v", event.Properties)
	}
	for key, value := range event.Properties {
		if strings.Contains(strings.ToLower(key), "summary") || strings.Contains(strings.ToLower(key), "source_text") {
			t.Fatalf("unsafe key exported: %s", key)
		}
		if s, ok := value.(string); ok && strings.Contains(s, "Private raw text") {
			t.Fatalf("unsafe value exported: %s=%s", key, s)
		}
	}
}

func TestAutonomyReadinessProjectionRejectsPoisonedProperties(t *testing.T) {
	event := AutonomyReadinessSafeEvents(documents.AutonomyReadinessReport{
		SuiteID:         "suite-demo",
		ThresholdStatus: documents.AutonomyReadinessNotEligible,
		KRs: map[string]documents.AutonomyReadinessKR{
			"KEY-3": {}, "KEY-4": {}, "KEY-5": {}, "KEY-6": {}, "KEY-7": {},
		},
	})[0]
	event.DistinctID = "mindline-test"
	event.Properties["source_excerpt"] = "private"
	if err := ValidateSafeEvent(event); err == nil || !strings.Contains(err.Error(), "unsafe PostHog property") {
		t.Fatalf("expected unsafe key rejection, got %v", err)
	}
}
