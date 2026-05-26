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
			CandidateCount:              2,
			JudgedCount:                 2,
			RemainingCount:              1,
			RejectedCount:               1,
			EvalCountedCount:            2,
			EvidenceReadyCount:          2,
			HumanReviewRequiredCount:    1,
			EvalCountedHumanReviewCount: 0,
			ModelErrorCount:             1,
			EvalCountedModelErrorCount:  0,
			FalsePositiveCount:          1,
			FalseNegativeCount:          0,
			EvalCountedAcceptedCount:    1,
			EvalCountedRemainingCount:   0,
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

	events := AutonomyReadinessSafeEvents(report, "telemetry-salt")
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
		event.Properties["eval_counted_accepted_count"] != 1 ||
		event.Properties["remaining_count"] != 1 ||
		event.Properties["eval_counted_remaining_count"] != 0 ||
		event.Properties["human_review_required_count"] != 1 ||
		event.Properties["eval_counted_human_review_required_count"] != 0 ||
		event.Properties["judgment_model_errors"] != 1 ||
		event.Properties["eval_counted_model_error_count"] != 0 {
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
	}, "telemetry-salt")[0]
	event.DistinctID = "mindline-test"
	event.Properties["source_excerpt"] = "private"
	if err := ValidateSafeEvent(event); err == nil || !strings.Contains(err.Error(), "unsafe PostHog property") {
		t.Fatalf("expected unsafe key rejection, got %v", err)
	}
}

func TestAutonomyReadinessSafeEventsSaltHostedIdentifiers(t *testing.T) {
	report := documents.AutonomyReadinessReport{
		SuiteID:         "private-suite-name",
		ThresholdStatus: documents.AutonomyReadinessNotEligible,
		KRs: map[string]documents.AutonomyReadinessKR{
			"KEY-3": {}, "KEY-4": {}, "KEY-5": {}, "KEY-6": {}, "KEY-7": {},
		},
	}

	first := AutonomyReadinessSafeEvents(report, "salt-one")[0]
	firstAgain := AutonomyReadinessSafeEvents(report, "salt-one")[0]
	second := AutonomyReadinessSafeEvents(report, "salt-two")[0]

	if first.Properties["run_id"] != firstAgain.Properties["run_id"] || first.TraceID != firstAgain.TraceID {
		t.Fatalf("expected stable salted identifiers for the same salt")
	}
	if first.Properties["run_id"] == second.Properties["run_id"] {
		t.Fatalf("expected run_id to vary by telemetry salt, got %q", first.Properties["run_id"])
	}
	if first.TraceID == second.TraceID {
		t.Fatalf("expected TraceID to vary by telemetry salt, got %q", first.TraceID)
	}
	if strings.Contains(first.Properties["run_id"].(string), report.SuiteID) || strings.Contains(first.TraceID, report.SuiteID) {
		t.Fatalf("expected hashed identifiers to omit raw suite id")
	}
}

func TestAutonomyReadinessSafeEventsChooseProviderModelDeterministically(t *testing.T) {
	report := documents.AutonomyReadinessReport{
		SuiteID:         "suite-demo",
		ThresholdStatus: documents.AutonomyReadinessNotEligible,
		KRs: map[string]documents.AutonomyReadinessKR{
			"KEY-3": {}, "KEY-4": {}, "KEY-5": {}, "KEY-6": {}, "KEY-7": {},
		},
		Slices: documents.AutonomyReadinessSlices{
			ByProviderModel: map[string]int{
				"openai/gpt-5.2":              2,
				"anthropic/claude-3-5-sonnet": 2,
				"deterministic_or_unknown":    10,
			},
		},
	}

	for i := 0; i < 25; i++ {
		event := AutonomyReadinessSafeEvents(report, "telemetry-salt")[0]
		if event.Properties["provider"] != "anthropic" || event.Properties["model"] != "claude-3-5-sonnet" {
			t.Fatalf("expected deterministic provider/model selection, got %q/%q", event.Properties["provider"], event.Properties["model"])
		}
	}
}
