package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

const TraceSummarySchemaVersion = "mindline-trace-summary/v0.1"

type TraceSummary struct {
	SchemaVersion  string         `json:"schema_version"`
	TraceID        string         `json:"trace_id"`
	RunID          string         `json:"run_id"`
	Command        string         `json:"command"`
	Workflow       string         `json:"workflow"`
	Provider       string         `json:"provider,omitempty"`
	Model          string         `json:"model,omitempty"`
	Status         string         `json:"status"`
	Counts         map[string]int `json:"counts"`
	Labels         map[string]int `json:"labels,omitempty"`
	Recommendation string         `json:"recommendation"`
	PostHogEvents  []string       `json:"posthog_events"`
	Privacy        TracePrivacy   `json:"privacy"`
}

type TracePrivacy struct {
	TraceMode      string `json:"trace_mode"`
	InputRedacted  bool   `json:"input_redacted"`
	OutputRedacted bool   `json:"output_redacted"`
	SourceRedacted bool   `json:"source_redacted"`
	MetadataOnly   bool   `json:"metadata_only"`
}

func (summary TraceSummary) SafeEvents() []SafeEvent {
	baseProperties := map[string]any{
		"event_schema":    TraceSummarySchemaVersion,
		"feature":         "mindline.trace_eval",
		"command":         summary.Command,
		"status":          summary.Status,
		"input_redacted":  summary.Privacy.InputRedacted,
		"output_redacted": summary.Privacy.OutputRedacted,
		"source_redacted": summary.Privacy.SourceRedacted,
		"trace_mode":      TraceModeMetadata,
		"run_id":          summary.RunID,
		"workflow":        summary.Workflow,
		"recommendation":  summary.Recommendation,
	}
	if strings.TrimSpace(summary.Provider) != "" {
		baseProperties["provider"] = summary.Provider
		baseProperties["$ai_provider"] = summary.Provider
	}
	if strings.TrimSpace(summary.Model) != "" {
		baseProperties["model"] = summary.Model
		baseProperties["$ai_model"] = summary.Model
	}
	for key, count := range summary.Counts {
		baseProperties[key] = count
	}
	for key, count := range summary.Labels {
		baseProperties[key] = count
	}
	events := make([]SafeEvent, 0, len(summary.PostHogEvents))
	for _, eventName := range summary.PostHogEvents {
		events = append(events, SafeEvent{
			Event:      eventName,
			TraceID:    summary.TraceID,
			Properties: cloneProperties(baseProperties),
		})
	}
	return events
}

func NewTraceSummary(workflow, command, runID, provider, model, status, recommendation string, counts map[string]int, labels map[string]int, events []string) TraceSummary {
	return TraceSummary{
		SchemaVersion:  TraceSummarySchemaVersion,
		TraceID:        stableTraceID(workflow, runID),
		RunID:          strings.TrimSpace(runID),
		Command:        strings.TrimSpace(command),
		Workflow:       strings.TrimSpace(workflow),
		Provider:       strings.TrimSpace(provider),
		Model:          strings.TrimSpace(model),
		Status:         strings.TrimSpace(status),
		Counts:         cloneCounts(counts),
		Labels:         cloneCounts(labels),
		Recommendation: strings.TrimSpace(recommendation),
		PostHogEvents:  append([]string(nil), events...),
		Privacy: TracePrivacy{
			TraceMode:      TraceModeMetadata,
			InputRedacted:  true,
			OutputRedacted: true,
			SourceRedacted: true,
			MetadataOnly:   true,
		},
	}
}

func cloneCounts(input map[string]int) map[string]int {
	out := map[string]int{}
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneProperties(input map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		out[key] = value
	}
	return out
}

func stableTraceID(workflow, runID string) string {
	parts := []string{strings.TrimSpace(workflow), strings.TrimSpace(runID)}
	sort.Strings(parts)
	return "trace-" + contentHash(strings.Join(parts, ":"))
}

func contentHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}
