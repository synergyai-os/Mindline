package destinations

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateDestinationOperation(t *testing.T) {
	valid := []Operation{
		validOperation(OperationCreateNote, VisibilityPublish, "30-resources/source.md", "Title", "Body", nil),
		validOperation(OperationAttentionPreview, VisibilityAttention, "00-inbox/clarify.md", "Clarify", "Body", []string{"needs_clarification"}),
		validOperation(OperationBackgroundRecord, VisibilityBackground, "40-archives/background/source.json", "Background", "Body", nil),
		validOperation(OperationSkip, VisibilitySkip, "", "", "", []string{"secret_like"}),
		validOperation(OperationBlocked, VisibilityBlocked, "", "Blocked", "", []string{"needs_enrichment"}),
	}
	for _, operation := range valid {
		t.Run(string(operation.OperationType), func(t *testing.T) {
			if err := ValidateOperation(operation); err != nil {
				t.Fatalf("expected valid operation, got %v", err)
			}
		})
	}

	invalid := []struct {
		name      string
		operation Operation
		want      string
	}{
		{name: "missing schema", operation: mutate(valid[0], func(o *Operation) { o.SchemaVersion = "" }), want: "schema_version"},
		{name: "missing operation id", operation: mutate(valid[0], func(o *Operation) { o.OperationID = "" }), want: "operation_id"},
		{name: "wrong write mode", operation: mutate(valid[0], func(o *Operation) { o.WriteMode = "live" }), want: "write_mode"},
		{name: "skip with locator", operation: mutate(valid[3], func(o *Operation) { o.PlannedLocator = "00-inbox/nope.md" }), want: "planned_locator"},
		{name: "skip without blocker", operation: mutate(valid[3], func(o *Operation) { o.Blockers = nil }), want: "blockers"},
		{name: "blocked with body", operation: mutate(valid[4], func(o *Operation) { o.Body = "leaky body" }), want: "body"},
		{name: "create without locator", operation: mutate(valid[0], func(o *Operation) { o.PlannedLocator = "" }), want: "planned_locator"},
		{name: "create with blocker", operation: mutate(valid[0], func(o *Operation) { o.Blockers = []string{"unexpected"} }), want: "blockers"},
		{name: "attention without body", operation: mutate(valid[1], func(o *Operation) { o.Body = "" }), want: "body"},
		{name: "background without title", operation: mutate(valid[2], func(o *Operation) { o.Title = "" }), want: "title"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOperation(tc.operation)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestOperationID(t *testing.T) {
	first := GenerateOperationID("tolaria", "a/b", OperationCreateNote)
	second := GenerateOperationID("tolaria", "a:b", OperationCreateNote)
	if first == second {
		t.Fatalf("expected fingerprint to prevent sanitized collision")
	}
	for _, id := range []string{first, second} {
		if strings.ContainsAny(id, "/: ") {
			t.Fatalf("operation id is not filename safe: %q", id)
		}
	}
}

func TestPrivacySafeOperationID(t *testing.T) {
	id := GenerateOperationID("tolaria", "xoxb-super-secret-private.example/files-pri", OperationCreateNote)
	if !strings.HasPrefix(id, "tolaria-operation-") {
		t.Fatalf("expected neutral visible base, got %q", id)
	}
	for _, leaked := range []string{"xoxb", "super-secret", "private.example", "files-pri"} {
		if strings.Contains(id, leaked) {
			t.Fatalf("operation id leaked %q: %s", leaked, id)
		}
	}

	blocked := validOperation(OperationCreateNote, VisibilityPublish, "30-resources/source.md", "Title", "Body", nil)
	blocked.OperationID = id
	blocked = ResolveConflicts([]Operation{
		validOperation(OperationCreateNote, VisibilityPublish, "30-resources/source.md", "Winner", "Body", nil),
		blocked,
	})[1]
	if blocked.OperationID != id {
		t.Fatalf("operation id changed after conflict conversion: got %q want %q", blocked.OperationID, id)
	}
}

func TestResolveConflicts(t *testing.T) {
	winner := validOperation(OperationCreateNote, VisibilityPublish, "30-resources/source.md", "Winner", "Body", nil)
	winner.OperationID = "winner"
	winner.IdempotencyKey = "same-key"
	loser := validOperation(OperationCreateNote, VisibilityPublish, "30-resources/source.md", "Loser", "Body", nil)
	loser.OperationID = "loser"
	loser.IdempotencyKey = "same-key"

	resolved := ResolveConflicts([]Operation{winner, loser})
	if resolved[0].OperationType != OperationCreateNote {
		t.Fatalf("winner changed: %#v", resolved[0])
	}
	got := resolved[1]
	if got.OperationType != OperationBlocked || got.VisibilityLane != VisibilityBlocked {
		t.Fatalf("loser not blocked: %#v", got)
	}
	if got.OperationID != "loser" {
		t.Fatalf("operation id changed: %q", got.OperationID)
	}
	if got.PlannedLocator != "" || got.Body != "" {
		t.Fatalf("blocked conflict kept locator/body: %#v", got)
	}
	assertContains(t, got.Blockers, "conflict:planned_locator")
	assertContains(t, got.Blockers, "conflict:idempotency_key")

	conflicts, ok := got.Metadata["conflicts"].([]Conflict)
	if !ok {
		t.Fatalf("expected typed conflicts metadata, got %#v", got.Metadata["conflicts"])
	}
	if len(conflicts) != 2 {
		t.Fatalf("expected two conflicts, got %#v", conflicts)
	}
	if conflicts[0].Field != "planned_locator" || conflicts[1].Field != "idempotency_key" {
		t.Fatalf("conflicts are not ordered by field: %#v", conflicts)
	}
	if conflicts[0].ConflictingOperationID != "winner" || conflicts[1].ConflictingOperationID != "winner" {
		t.Fatalf("missing winning operation ids: %#v", conflicts)
	}
}

func TestResolveConflictsSanitizesSecretValues(t *testing.T) {
	winner := validOperation(OperationCreateNote, VisibilityPublish, "30-resources/super-secret.md", "Winner", "Body", nil)
	winner.OperationID = "winner"
	winner.IdempotencyKey = "api_key=super-secret"
	loser := validOperation(OperationCreateNote, VisibilityPublish, "30-resources/super-secret.md", "Loser", "Body", nil)
	loser.OperationID = "loser"
	loser.IdempotencyKey = "api_key=super-secret"

	got := ResolveConflicts([]Operation{winner, loser})[1]
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal operation: %v", err)
	}
	for _, leaked := range []string{"api_key=super-secret", "30-resources/super-secret.md"} {
		if strings.Contains(string(encoded), leaked) {
			t.Fatalf("conflict metadata leaked %q:\n%s", leaked, encoded)
		}
	}
	if !strings.Contains(string(encoded), "fingerprint:") {
		t.Fatalf("expected fingerprinted conflict value, got %s", encoded)
	}
}

func validOperation(operationType OperationType, lane VisibilityLane, locator, title, body string, blockers []string) Operation {
	return Operation{
		SchemaVersion:        "destination-operation/v0.1",
		OperationID:          "operation-id",
		DestinationAdapterID: "tolaria",
		SourceCandidateID:    "candidate-1",
		SourceRecordID:       "record-1",
		IdempotencyKey:       "source:key",
		OperationType:        operationType,
		WriteMode:            WriteModeDryRun,
		VisibilityLane:       lane,
		PlannedLocator:       locator,
		Title:                title,
		Body:                 body,
		Metadata:             map[string]any{},
		Blockers:             blockers,
		AuthorityIDs:         []string{"WP-5", "DEC-12"},
	}
}

func mutate(operation Operation, fn func(*Operation)) Operation {
	fn(&operation)
	return operation
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("expected %q in %#v", want, values)
}
