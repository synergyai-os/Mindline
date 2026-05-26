package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const postHogTimeout = 10 * time.Second
const propEvalHumanReviewRequired = "eval_counted_human_review_required_count"

var allowedProperties = map[string]bool{
	"event_schema":                       true,
	"feature":                            true,
	"trace_mode":                         true,
	"provider":                           true,
	"model":                              true,
	"command":                            true,
	"status":                             true,
	"input_redacted":                     true,
	"output_redacted":                    true,
	"source_redacted":                    true,
	"blocked_field_count":                true,
	"secret_detected":                    true,
	"failure_reason":                     true,
	"confidence_bucket":                  true,
	"validation_result":                  true,
	"evaluation_result":                  true,
	"latency_ms":                         true,
	"input_tokens":                       true,
	"output_tokens":                      true,
	"estimated_cost_cents":               true,
	"run_id":                             true,
	"workflow":                           true,
	"recommendation":                     true,
	"source_count":                       true,
	"observation_count":                  true,
	"candidate_count":                    true,
	"relation_count":                     true,
	"needs_review_count":                 true,
	"blocked_count":                      true,
	"skipped_count":                      true,
	"judged_count":                       true,
	"remaining_count":                    true,
	"agent_reviewed_count":               true,
	"human_review_required_count":        true,
	"machine_triaged_count":              true,
	"accepted_count":                     true,
	"rejected_count":                     true,
	"unclear_count":                      true,
	"duplicate_count":                    true,
	"wrong_kind_count":                   true,
	"false_positive_count":               true,
	"false_negative_count":               true,
	"eval_counted_accepted_count":        true,
	"eval_counted_false_positive_count":  true,
	"eval_counted_false_negative_count":  true,
	"eval_counted_unclear_count":         true,
	"eval_counted_remaining_count":       true,
	"evidence_ready_count":               true,
	"eval_counted_count":                 true,
	"evidence_excluded_count":            true,
	"review_burden_count":                true,
	propEvalHumanReviewRequired:          true,
	"semantic_no_candidates":             true,
	"semantic_needs_review":              true,
	"judgment_human_review":              true,
	"judgment_model_errors":              true,
	"eval_counted_model_error_count":     true,
	"trace_id":                           true,
	"$ai_trace_id":                       true,
	"$ai_provider":                       true,
	"$ai_model":                          true,
	"$ai_is_error":                       true,
	"metadata_only":                      true,
	"projection_schema":                  true,
	"threshold":                          true,
	"accuracy":                           true,
	"held_out":                           true,
	"safety_destination_writes":          true,
	"safety_auto_accepts":                true,
	"safety_no_human_claims":             true,
	"safety_committed_private_artifacts": true,
	"kr_key_3":                           true,
	"kr_key_4":                           true,
	"kr_key_5":                           true,
	"kr_key_6":                           true,
	"kr_key_7":                           true,
}

var allowedDynamicProperties = map[string]bool{
	"failure_reason_count.ambiguous":                               true,
	"failure_reason_count.duplicate":                               true,
	"failure_reason_count.missing_evidence":                        true,
	"failure_reason_count.missing_expected_outcome":                true,
	"failure_reason_count.other":                                   true,
	"failure_reason_count.relation_error":                          true,
	"failure_reason_count.source_scope_error":                      true,
	"failure_reason_count.stale_or_contradicted":                   true,
	"failure_reason_count.too_broad":                               true,
	"failure_reason_count.too_narrow":                              true,
	"failure_reason_count.unexpected_candidate":                    true,
	"failure_reason_count.unsafe_or_private":                       true,
	"failure_reason_count.unsupported_evidence":                    true,
	"failure_reason_count.wrong_kind":                              true,
	"evidence_readiness_reason_count.blocked_or_skipped":           true,
	"evidence_readiness_reason_count.candidate_blockers":           true,
	"evidence_readiness_reason_count.invalid_relation_context":     true,
	"evidence_readiness_reason_count.missing_candidate_content":    true,
	"evidence_readiness_reason_count.missing_evidence_nodes":       true,
	"evidence_readiness_reason_count.missing_evidence_ranges":      true,
	"evidence_readiness_reason_count.missing_relation_context":     true,
	"evidence_readiness_reason_count.missing_source_excerpt":       true,
	"evidence_readiness_reason_count.private_or_governance_marker": true,
}

var forbiddenPropertyMarkers = []string{
	"prompt",
	"completion",
	"source_text",
	"source_excerpt",
	"candidate_summary",
	"raw_input",
	"raw_output",
	"file_path",
	"temp_path",
	"permalink",
	"secret",
	"api_key",
	"token",
}

type SafeEvent struct {
	Event      string
	DistinctID string
	TraceID    string
	Properties map[string]any
}

type PostHogExporter struct {
	config Config
	client *http.Client
}

type SafeEventValidationError struct {
	err error
}

func (err SafeEventValidationError) Error() string {
	return err.err.Error()
}

func (err SafeEventValidationError) Unwrap() error {
	return err.err
}

func IsSafeEventValidationError(err error) bool {
	var validationErr SafeEventValidationError
	return errors.As(err, &validationErr)
}

func NewPostHogExporter(config Config, transport http.RoundTripper) PostHogExporter {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return PostHogExporter{
		config: config,
		client: &http.Client{
			Transport: transport,
			Timeout:   postHogTimeout,
		},
	}
}

func (exporter PostHogExporter) Capture(event SafeEvent) error {
	if !exporter.config.Enabled {
		return nil
	}
	properties := map[string]any{}
	for key, value := range event.Properties {
		properties[key] = value
	}
	properties["trace_id"] = event.TraceID
	properties["$ai_trace_id"] = event.TraceID
	properties["trace_mode"] = TraceModeMetadata
	finalEvent := event
	if distinctID := pseudonymousDistinctID(exporter.config.TelemetrySalt); distinctID != "" {
		finalEvent.DistinctID = distinctID
	}
	finalEvent.Properties = properties
	if err := ValidateSafeEvent(finalEvent); err != nil {
		return SafeEventValidationError{err: err}
	}

	payload := map[string]any{
		"api_key":     exporter.config.PostHogKey,
		"event":       finalEvent.Event,
		"distinct_id": finalEvent.DistinctID,
		"properties":  properties,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, exporter.config.PostHogHost+"/capture/", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := exporter.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 256))
		return fmt.Errorf("PostHog capture status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func pseudonymousDistinctID(telemetrySalt string) string {
	trimmed := strings.TrimSpace(telemetrySalt)
	if trimmed == "" {
		return ""
	}
	return "mindline-" + contentHash("mindline.posthog.distinct_id:"+trimmed)
}

func ValidateSafeEvent(event SafeEvent) error {
	if strings.TrimSpace(event.Event) == "" {
		return fmt.Errorf("missing event name")
	}
	if looksUnsafeValue(event.Event) {
		return fmt.Errorf("unsafe event name")
	}
	if strings.TrimSpace(event.DistinctID) == "" {
		return fmt.Errorf("missing distinct id")
	}
	if looksUnsafeValue(event.DistinctID) {
		return fmt.Errorf("unsafe distinct id")
	}
	if strings.TrimSpace(event.TraceID) == "" {
		return fmt.Errorf("missing trace id")
	}
	if looksUnsafeValue(event.TraceID) {
		return fmt.Errorf("unsafe trace id")
	}
	for key, value := range event.Properties {
		allowedDynamic := allowedDynamicProperties[key]
		if !allowedProperties[key] && !allowedDynamic {
			return fmt.Errorf("unsafe PostHog property: %s", key)
		}
		if err := validateSafePropertyValue(key, value); err != nil {
			return err
		}
	}
	return nil
}

func validateSafePropertyValue(key string, value any) error {
	switch typed := value.(type) {
	case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return nil
	case string:
		if looksUnsafeValue(typed) {
			return fmt.Errorf("unsafe PostHog property value: %s", key)
		}
		return nil
	default:
		return fmt.Errorf("unsupported PostHog property value: %s", key)
	}
}

func looksUnsafeValue(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "sk-") ||
		strings.Contains(lower, "-----begin") ||
		strings.Contains(lower, "/users/") ||
		strings.Contains(lower, "young human club dropbox") ||
		strings.Contains(lower, "source excerpt") ||
		strings.Contains(lower, "prompt:") ||
		strings.Contains(lower, "completion:")
}
