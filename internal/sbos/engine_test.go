package sbos

import (
	"strings"
	"testing"
)

func TestProcessCandidateRejectsMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			name: "missing candidate id",
			input: `{
				"schema_version": "v0.1",
				"adapter_id": "slack",
				"external_id": "msg-1",
				"captured_at": "2026-05-20T10:00:00Z",
				"idempotency_key": "slack:msg-1"
			}`,
		},
		{
			name:  "missing provenance",
			input: strings.Replace(publishCandidate("missing-provenance", "slack:missing-provenance"), `"provenance": {`, `"missing_provenance": {`, 1),
		},
		{
			name:  "missing classification type",
			input: strings.Replace(publishCandidate("missing-type", "slack:missing-type"), `"type": "Source"`, `"type": ""`, 1),
		},
		{
			name:  "missing content text",
			input: strings.Replace(publishCandidate("missing-content", "slack:missing-content"), `"text": "A useful source about CODE workflow."`, `"text": ""`, 1),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewEngine().ProcessCandidate([]byte(tc.input))

			if err == nil {
				t.Fatalf("expected validation error")
			}
			if result.State != StateError {
				t.Fatalf("expected error state, got %q", result.State)
			}
			if len(result.Artifacts) != 0 {
				t.Fatalf("expected no artifacts, got %d", len(result.Artifacts))
			}
		})
	}
}

func TestProcessCandidateRejectsInvalidEnumValues(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid schema version",
			input: strings.Replace(publishCandidate("bad-schema", "slack:bad-schema"), `"schema_version": "v0.1"`, `"schema_version": "v9"`, 1),
		},
		{
			name:  "invalid enrichment status",
			input: strings.Replace(publishCandidate("bad-enrichment", "slack:bad-enrichment"), `"enrichment_status": "complete"`, `"enrichment_status": "maybe"`, 1),
		},
		{
			name:  "invalid desired visibility",
			input: strings.Replace(publishCandidate("bad-visibility", "slack:bad-visibility"), `"desired_visibility": "publish"`, `"desired_visibility": "visible"`, 1),
		},
		{
			name:  "invalid provenance visibility",
			input: strings.Replace(publishCandidate("bad-provenance-visibility", "slack:bad-provenance-visibility"), `"visibility": "public"`, `"visibility": "team-only"`, 1),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NewEngine().ProcessCandidate([]byte(tc.input))

			if err == nil {
				t.Fatalf("expected validation error")
			}
			if result.State != StateError {
				t.Fatalf("expected error state, got %q", result.State)
			}
			if len(result.Artifacts) != 0 {
				t.Fatalf("expected no artifacts, got %d", len(result.Artifacts))
			}
		})
	}
}

func TestProcessCandidateDoesNotDuplicateIdempotencyKeys(t *testing.T) {
	engine := NewEngine()
	input := publishCandidate("candidate-1", "slack:msg-1")

	first, err := engine.ProcessCandidate([]byte(input))
	if err != nil {
		t.Fatalf("first process: %v", err)
	}
	second, err := engine.ProcessCandidate([]byte(input))
	if err != nil {
		t.Fatalf("second process: %v", err)
	}

	if first.RecordID != second.RecordID {
		t.Fatalf("expected duplicate to return existing record id, got %q and %q", first.RecordID, second.RecordID)
	}
	if engine.RecordCount() != 1 {
		t.Fatalf("expected one stored record, got %d", engine.RecordCount())
	}
	if len(second.Artifacts) != 0 {
		t.Fatalf("expected duplicate processing to emit no new artifact, got %d", len(second.Artifacts))
	}
}

func TestEngineUsesInjectedCandidateStore(t *testing.T) {
	store := NewMemoryCandidateStore()
	engine := NewEngineWithStore(store)

	_, err := engine.ProcessCandidate([]byte(publishCandidate("candidate-store", "slack:store")))
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if store.Count() != 1 {
		t.Fatalf("expected injected store to contain one record, got %d", store.Count())
	}
}

func TestProcessCandidateSkipsEmptyOrSecretLikeContent(t *testing.T) {
	cases := []struct {
		name  string
		field string
	}{
		{name: "empty content", field: "empty_content"},
		{name: "secret-like content", field: "secret_like"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := candidateWithSafety("candidate-"+tc.name, tc.field, `"desired_visibility": "publish"`)

			result, err := NewEngine().ProcessCandidate([]byte(input))

			if err != nil {
				t.Fatalf("process: %v", err)
			}
			if result.State != StateSkipped {
				t.Fatalf("expected skipped state, got %q", result.State)
			}
			if len(result.Artifacts) != 0 {
				t.Fatalf("expected no artifacts, got %d", len(result.Artifacts))
			}
			if strings.Contains(result.StoredRecord.RawContent, "secret") {
				t.Fatalf("expected stored record to be redacted/minimal, got %q", result.StoredRecord.RawContent)
			}
		})
	}
}

func TestProcessCandidateBlocksPrivateOrRedactedPublishArtifacts(t *testing.T) {
	cases := []struct {
		name  string
		field string
	}{
		{name: "redaction required", field: "redaction_required"},
		{name: "private provenance", field: "private_provenance"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := candidateWithSafety("candidate-"+tc.name, tc.field, `"desired_visibility": "publish"`)

			result, err := NewEngine().ProcessCandidate([]byte(input))

			if err != nil {
				t.Fatalf("process: %v", err)
			}
			if result.State != StateBackgroundReady {
				t.Fatalf("expected background_ready state, got %q", result.State)
			}
			if len(result.Artifacts) != 0 {
				t.Fatalf("expected no publish artifact, got %d", len(result.Artifacts))
			}
		})
	}
}

func TestProcessCandidateDerivesPrivateProvenanceBlockFromFieldVisibility(t *testing.T) {
	input := publishCandidate("candidate-private-derived", "slack:private-derived")
	input = strings.Replace(input, `"raw_locator": {"value": "slack://D123/msg-1", "visibility": "public"}`, `"raw_locator": {"value": "slack://D123/private", "visibility": "private"}`, 1)

	result, err := NewEngine().ProcessCandidate([]byte(input))

	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if result.State != StateBackgroundReady {
		t.Fatalf("expected background_ready state, got %q", result.State)
	}
	if len(result.Artifacts) != 0 {
		t.Fatalf("expected private provenance to block publish artifact, got %d", len(result.Artifacts))
	}
}

func TestProcessCandidateRedactsPrivateFieldsInAttentionPreview(t *testing.T) {
	input := candidateWithSafety("candidate-redacted-attention", "redaction_required", `"desired_visibility": "attention"`)

	result, err := NewEngine().ProcessCandidate([]byte(input))

	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if result.State != StateAttentionReady {
		t.Fatalf("expected attention_ready state, got %q", result.State)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("expected one attention artifact, got %d", len(result.Artifacts))
	}
	body := result.Artifacts[0].Body
	for _, leaked := range []string{"candidate-redacted-attention", "https://private.example/source", "Randy Private", "secret body"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("expected private value %q to be redacted from artifact:\n%s", leaked, body)
		}
	}
	if !strings.Contains(body, "[redacted]") {
		t.Fatalf("expected redacted marker in artifact:\n%s", body)
	}
}

func TestProcessCandidateBlocksIncompleteEnrichmentFromPublishing(t *testing.T) {
	for _, enrichmentStatus := range []string{"incomplete", "failed"} {
		t.Run(enrichmentStatus, func(t *testing.T) {
			input := candidateWithEnrichment("candidate-"+enrichmentStatus, enrichmentStatus, "publish")

			result, err := NewEngine().ProcessCandidate([]byte(input))

			if err != nil {
				t.Fatalf("process: %v", err)
			}
			if result.State != StateNeedsEnrichment {
				t.Fatalf("expected needs_enrichment state, got %q", result.State)
			}
			if len(result.Artifacts) != 0 {
				t.Fatalf("expected no artifacts, got %d", len(result.Artifacts))
			}
		})
	}
}

func TestProcessCandidateClarifyIntentAlwaysCreatesClarificationPreview(t *testing.T) {
	input := candidateWithVisibility("candidate-clarify", "clarify", false)

	result, err := NewEngine().ProcessCandidate([]byte(input))

	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if result.State != StateAttentionReady {
		t.Fatalf("expected attention_ready state, got %q", result.State)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("expected one clarification preview, got %d", len(result.Artifacts))
	}
	if strings.Contains(result.Artifacts[0].Body, "## Snapshot") {
		t.Fatalf("clarification preview must not fake a processed source note:\n%s", result.Artifacts[0].Body)
	}
	if !strings.Contains(result.Artifacts[0].Body, "Clarification needed") {
		t.Fatalf("expected clarification language, got:\n%s", result.Artifacts[0].Body)
	}
}

func TestProcessCandidateBackgroundCreatesNoVisibleArtifact(t *testing.T) {
	input := candidateWithVisibility("candidate-background", "background", false)

	result, err := NewEngine().ProcessCandidate([]byte(input))

	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if result.State != StateBackgroundReady {
		t.Fatalf("expected background_ready state, got %q", result.State)
	}
	if len(result.Artifacts) != 0 {
		t.Fatalf("expected no artifacts, got %d", len(result.Artifacts))
	}
}

func TestProcessCandidateAttentionCreatesOnlyAttentionPreview(t *testing.T) {
	input := candidateWithVisibility("candidate-attention", "attention", false)

	result, err := NewEngine().ProcessCandidate([]byte(input))

	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if result.State != StateAttentionReady {
		t.Fatalf("expected attention_ready state, got %q", result.State)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("expected one artifact, got %d", len(result.Artifacts))
	}
	if result.Artifacts[0].Kind != ArtifactAttentionPreview {
		t.Fatalf("expected attention preview, got %q", result.Artifacts[0].Kind)
	}
	if strings.Contains(result.Artifacts[0].Body, "## Snapshot") {
		t.Fatalf("attention preview must not fake a processed source note:\n%s", result.Artifacts[0].Body)
	}
}

func TestProcessCandidatePublishCreatesDeterministicMarkdown(t *testing.T) {
	input := publishCandidate("candidate-publish", "slack:publish")

	result, err := NewEngine().ProcessCandidate([]byte(input))

	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if result.State != StateDryRunPublished {
		t.Fatalf("expected dry_run_published state, got %q", result.State)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("expected one artifact, got %d", len(result.Artifacts))
	}
	if result.Artifacts[0].Kind != ArtifactDryRunPublish {
		t.Fatalf("expected dry run publish artifact, got %q", result.Artifacts[0].Kind)
	}
	assertEqualString(t, result.Artifacts[0].Body, expectedPublishMarkdown())
}

func TestStateMachineRejectsInvalidTransitions(t *testing.T) {
	machine := NewStateMachine()

	if err := machine.Transition(StateIngested, StateDryRunPublished); err == nil {
		t.Fatalf("expected invalid transition error")
	}
	if err := machine.Transition(StatePublishReady, StateDryRunPublished); err != nil {
		t.Fatalf("expected valid transition, got %v", err)
	}
}

func TestProcessCandidateCarriesPBAuthorityMetadata(t *testing.T) {
	result, err := NewEngine().ProcessCandidate([]byte(publishCandidate("candidate-authority", "slack:authority")))
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	for _, required := range []string{"DEC-4", "DEC-3", "DEC-2", "DEC-1", "FEAT-1", "STD-1", "STD-7", "STD-10", "STD-11", "STD-12", "FEAT-4", "WP-1"} {
		if !contains(result.AuthorityIDs, required) {
			t.Fatalf("missing PB authority id %q in %#v", required, result.AuthorityIDs)
		}
	}
}

func publishCandidate(candidateID, idempotencyKey string) string {
	return `{
		"schema_version": "v0.1",
		"candidate_id": "` + candidateID + `",
		"adapter_id": "slack",
		"external_id": "msg-1",
		"captured_at": "2026-05-20T10:00:00Z",
		"provenance": {
			"permalink": {"value": "https://public.example/source", "visibility": "public"},
			"native_timestamp": {"value": "2026-05-20T10:00:00Z", "visibility": "public"},
			"author": {"value": "Randy", "visibility": "public"},
			"raw_locator": {"value": "slack://D123/msg-1", "visibility": "public"}
		},
		"content": {
			"text": "A useful source about CODE workflow.",
			"urls": ["https://public.example/source"],
			"attachments": [],
			"source_title": "CODE workflow source"
		},
		"enrichment_status": "complete",
		"classification": {
			"type": "Source",
			"domain": "Tolaria PKM OS",
			"topics": ["knowledge-management", "code-workflow"],
			"confidence": "high",
			"needs_clarification": false,
			"clarification_reason": ""
		},
		"safety": {
			"redaction_required": false,
			"secret_like": false,
			"empty_content": false,
			"private_provenance": false
		},
		"desired_visibility": "publish",
		"idempotency_key": "` + idempotencyKey + `"
	}`
}

func candidateWithVisibility(candidateID, visibility string, needsClarification bool) string {
	input := publishCandidate(candidateID, "slack:"+candidateID)
	input = strings.Replace(input, `"desired_visibility": "publish"`, `"desired_visibility": "`+visibility+`"`, 1)
	input = strings.Replace(input, `"needs_clarification": false`, `"needs_clarification": `+boolLiteral(needsClarification), 1)
	return input
}

func candidateWithEnrichment(candidateID, enrichmentStatus, visibility string) string {
	input := candidateWithVisibility(candidateID, visibility, false)
	return strings.Replace(input, `"enrichment_status": "complete"`, `"enrichment_status": "`+enrichmentStatus+`"`, 1)
}

func candidateWithSafety(candidateID, safetyField, visibilityMutation string) string {
	input := publishCandidate(candidateID, "slack:"+candidateID)
	input = strings.Replace(input, `"`+safetyField+`": false`, `"`+safetyField+`": true`, 1)
	input = strings.Replace(input, `"desired_visibility": "publish"`, visibilityMutation, 1)
	input = strings.Replace(input, `"permalink": {"value": "https://public.example/source", "visibility": "public"}`, `"permalink": {"value": "https://private.example/source", "visibility": "private"}`, 1)
	input = strings.Replace(input, `"author": {"value": "Randy", "visibility": "public"}`, `"author": {"value": "Randy Private", "visibility": "private"}`, 1)
	input = strings.Replace(input, `"text": "A useful source about CODE workflow."`, `"text": "secret body"`, 1)
	return input
}

func expectedPublishMarkdown() string {
	return `source_candidate_id: candidate-publish
state: dry_run_published
title: CODE workflow source
type: Source
domain: Tolaria PKM OS
confidence: high
source_adapter: slack
text: A useful source about CODE workflow.
urls:
- https://public.example/source
`
}

func assertEqualString(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("unexpected string\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func boolLiteral(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
