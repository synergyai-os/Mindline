package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestConfigFromValuesDisabledByDefault(t *testing.T) {
	config, err := ConfigFromValues(map[string]string{})
	if err != nil {
		t.Fatalf("disabled config should not fail: %v", err)
	}
	if config.Enabled {
		t.Fatalf("telemetry should be disabled by default")
	}
	if config.TraceMode != TraceModeMetadata {
		t.Fatalf("trace mode = %q", config.TraceMode)
	}
}

func TestConfigFromValuesRejectsUnsafeTraceModes(t *testing.T) {
	for _, mode := range []string{"raw", "full"} {
		t.Run(mode, func(t *testing.T) {
			_, err := ConfigFromValues(map[string]string{
				"MINDLINE_LLM_TRACE_MODE": mode,
			})
			if err == nil || !strings.Contains(err.Error(), "unsupported LLM trace mode") {
				t.Fatalf("expected unsafe mode rejection, got %v", err)
			}
		})
	}
}

func TestConfigFromValuesRequiresPostHogFieldsWhenEnabled(t *testing.T) {
	_, err := ConfigFromValues(map[string]string{
		"MINDLINE_TELEMETRY_ENABLED": "true",
		"MINDLINE_LLM_TRACE_MODE":    "metadata",
	})
	if err == nil || !strings.Contains(err.Error(), "missing PostHog project API key") {
		t.Fatalf("expected missing PostHog key, got %v", err)
	}
}

func TestValidateSafeEventRejectsUnsafeProperties(t *testing.T) {
	event := SafeEvent{
		Event:      "$ai_generation",
		DistinctID: "mindline-local",
		TraceID:    "trace-test",
		Properties: map[string]any{
			"prompt": "private prompt",
		},
	}
	if err := ValidateSafeEvent(event); err == nil {
		t.Fatalf("expected unsafe property rejection")
	}
}

func TestValidateSafeEventRejectsUnsafeValues(t *testing.T) {
	event := SafeEvent{
		Event:      "$ai_generation",
		DistinctID: "mindline-local",
		TraceID:    "trace-test",
		Properties: map[string]any{
			"failure_reason": "/Users/randyhereman/private.md",
		},
	}
	if err := ValidateSafeEvent(event); err == nil {
		t.Fatalf("expected unsafe value rejection")
	}
}

func TestValidateSafeEventRejectsNestedValues(t *testing.T) {
	event := SafeEvent{
		Event:      "$ai_generation",
		DistinctID: "mindline-local",
		TraceID:    "trace-test",
		Properties: map[string]any{
			"failure_reason": []string{"/Users/randyhereman/private.md"},
		},
	}
	if err := ValidateSafeEvent(event); err == nil || !strings.Contains(err.Error(), "unsupported PostHog property value") {
		t.Fatalf("expected nested value rejection, got %v", err)
	}
}

func TestValidateSafeEventAllowsExplicitTelemetryTokenAndSecretKeys(t *testing.T) {
	event := SafeEvent{
		Event:      "$ai_generation",
		DistinctID: "mindline-local",
		TraceID:    "trace-test",
		Properties: map[string]any{
			"input_tokens":    42,
			"output_tokens":   12,
			"secret_detected": false,
		},
	}

	if err := ValidateSafeEvent(event); err != nil {
		t.Fatalf("expected explicitly allowlisted telemetry keys to pass, got %v", err)
	}
}

func TestPostHogExporterRejectsUnsafeTraceIDBeforeNetwork(t *testing.T) {
	called := false
	exporter := NewPostHogExporter(Config{
		Enabled:       true,
		TraceMode:     TraceModeMetadata,
		TelemetrySalt: "salt",
		PostHogKey:    "phc-test",
		PostHogHost:   "https://eu.i.posthog.com",
	}, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	}))

	err := exporter.Capture(SafeEvent{
		Event:      "$ai_generation",
		DistinctID: "mindline-local",
		TraceID:    "/Users/randyhereman/private.md",
		Properties: map[string]any{
			"event_schema": "mindline.telemetry.test/v0.1",
			"feature":      "observability.posthog_test",
			"status":       "ok",
		},
	})

	if err == nil || !strings.Contains(err.Error(), "unsafe trace id") {
		t.Fatalf("expected unsafe trace id rejection, got %v", err)
	}
	if called {
		t.Fatalf("unsafe trace id must fail before network")
	}
}

func TestPostHogExporterCapturesMetadataOnlyEvent(t *testing.T) {
	var captured map[string]any
	exporter := NewPostHogExporter(Config{
		Enabled:       true,
		TraceMode:     TraceModeMetadata,
		TelemetrySalt: "salt",
		PostHogKey:    "phc-test",
		PostHogHost:   "https://eu.i.posthog.com",
	}, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/capture/" {
			t.Fatalf("expected /capture/, got %s", req.URL.Path)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if strings.Contains(string(body), "private") || strings.Contains(string(body), "prompt") {
			t.Fatalf("export body contains unsafe content: %s", string(body))
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	}))

	err := exporter.Capture(SafeEvent{
		Event:      "$ai_generation",
		DistinctID: "mindline-local",
		TraceID:    "trace-test",
		Properties: map[string]any{
			"event_schema":    "mindline.telemetry.test/v0.1",
			"feature":         "observability.posthog_test",
			"provider":        "openai",
			"model":           "gpt-test",
			"command":         "observability posthog-test",
			"status":          "ok",
			"input_redacted":  true,
			"output_redacted": true,
			"source_redacted": true,
		},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if captured["api_key"] != "phc-test" || captured["event"] != "$ai_generation" {
		t.Fatalf("unexpected payload: %+v", captured)
	}
	if captured["distinct_id"] != expectedDistinctID("salt") {
		t.Fatalf("expected salted distinct id, got %+v", captured["distinct_id"])
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func expectedDistinctID(salt string) string {
	sum := sha256.Sum256([]byte("mindline.posthog.distinct_id:" + salt))
	return "mindline-" + hex.EncodeToString(sum[:])[:16]
}
