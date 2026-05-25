package observability

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	TraceModeMetadata = "metadata"
)

type Config struct {
	Enabled       bool
	TraceMode     string
	TelemetrySalt string
	PostHogKey    string
	PostHogHost   string
}

func ConfigFromValues(values map[string]string) (Config, error) {
	config := Config{
		Enabled:       parseBool(values["MINDLINE_TELEMETRY_ENABLED"]),
		TraceMode:     valueOrDefault(values["MINDLINE_LLM_TRACE_MODE"], TraceModeMetadata),
		TelemetrySalt: strings.TrimSpace(values["MINDLINE_TELEMETRY_SALT"]),
		PostHogKey:    strings.TrimSpace(firstNonEmpty(values["POSTHOG_PROJECT_API_KEY"], values["POSTHOG_API_KEY"])),
		PostHogHost:   strings.TrimRight(strings.TrimSpace(values["POSTHOG_HOST"]), "/"),
	}
	if config.TraceMode != TraceModeMetadata {
		return config, fmt.Errorf("unsupported LLM trace mode: %s", config.TraceMode)
	}
	if !config.Enabled {
		return config, nil
	}
	if config.PostHogKey == "" {
		return config, errors.New("missing PostHog project API key")
	}
	if config.PostHogHost == "" {
		return config, errors.New("missing PostHog host")
	}
	parsed, err := url.Parse(config.PostHogHost)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return config, fmt.Errorf("invalid PostHog host: %s", config.PostHogHost)
	}
	if config.TelemetrySalt == "" {
		return config, errors.New("missing telemetry salt")
	}
	return config, nil
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func valueOrDefault(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
