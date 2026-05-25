package observability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const postHogTimeout = 10 * time.Second

var allowedProperties = map[string]bool{
	"event_schema":         true,
	"feature":              true,
	"trace_mode":           true,
	"provider":             true,
	"model":                true,
	"command":              true,
	"status":               true,
	"input_redacted":       true,
	"output_redacted":      true,
	"source_redacted":      true,
	"blocked_field_count":  true,
	"secret_detected":      true,
	"failure_reason":       true,
	"confidence_bucket":    true,
	"validation_result":    true,
	"evaluation_result":    true,
	"latency_ms":           true,
	"input_tokens":         true,
	"output_tokens":        true,
	"estimated_cost_cents": true,
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
	if err := ValidateSafeEvent(event); err != nil {
		return err
	}
	properties := map[string]any{}
	for key, value := range event.Properties {
		properties[key] = value
	}
	properties["trace_id"] = event.TraceID
	properties["trace_mode"] = TraceModeMetadata

	payload := map[string]any{
		"api_key":     exporter.config.PostHogKey,
		"event":       event.Event,
		"distinct_id": event.DistinctID,
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
		if !allowedProperties[key] {
			return fmt.Errorf("unsafe PostHog property: %s", key)
		}
		for _, marker := range forbiddenPropertyMarkers {
			if strings.Contains(strings.ToLower(key), marker) {
				return fmt.Errorf("unsafe PostHog property: %s", key)
			}
		}
		if text, ok := value.(string); ok {
			if looksUnsafeValue(text) {
				return fmt.Errorf("unsafe PostHog property value: %s", key)
			}
		}
	}
	return nil
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
