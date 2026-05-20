package tolaria

import (
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/destinations"
)

func TestPlanMapsInputStatesToTolariaOperations(t *testing.T) {
	cases := []struct {
		name          string
		input         destinations.InputResult
		operationType destinations.OperationType
		lane          destinations.VisibilityLane
		locatorPart   string
		bodyPart      string
		blocked       bool
	}{
		{
			name:          "publish",
			input:         inputResult("dry_run_published", "dry_run_publish", publishBody(), destinations.InputSafety{}),
			operationType: destinations.OperationCreateNote,
			lane:          destinations.VisibilityPublish,
			locatorPart:   "30-resources/",
			bodyPart:      "## Snapshot",
		},
		{
			name:          "attention",
			input:         inputResult("attention_ready", "attention_preview", "Clarification needed", destinations.InputSafety{}),
			operationType: destinations.OperationAttentionPreview,
			lane:          destinations.VisibilityAttention,
			locatorPart:   "00-inbox/",
			bodyPart:      "Clarification needed",
			blocked:       true,
		},
		{
			name:          "background",
			input:         inputResult("background_ready", "", "", destinations.InputSafety{}),
			operationType: destinations.OperationBackgroundRecord,
			lane:          destinations.VisibilityBackground,
			locatorPart:   "40-archives/background/",
			bodyPart:      "background_ready",
		},
		{
			name:          "skipped",
			input:         inputResult("skipped", "", "", destinations.InputSafety{SecretLike: true}),
			operationType: destinations.OperationSkip,
			lane:          destinations.VisibilitySkip,
			blocked:       true,
		},
		{
			name:          "needs enrichment",
			input:         inputResult("needs_enrichment", "", "", destinations.InputSafety{}),
			operationType: destinations.OperationBlocked,
			lane:          destinations.VisibilityBlocked,
			blocked:       true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			operations, err := Plan(tc.input)
			if err != nil {
				t.Fatalf("plan: %v", err)
			}
			if len(operations) != 1 {
				t.Fatalf("expected one operation, got %#v", operations)
			}
			got := operations[0]
			if got.OperationType != tc.operationType || got.VisibilityLane != tc.lane {
				t.Fatalf("unexpected operation: %#v", got)
			}
			if tc.locatorPart != "" && !strings.Contains(got.PlannedLocator, tc.locatorPart) {
				t.Fatalf("planned locator %q missing %q", got.PlannedLocator, tc.locatorPart)
			}
			if tc.bodyPart != "" && !strings.Contains(got.Body, tc.bodyPart) {
				t.Fatalf("body missing %q:\n%s", tc.bodyPart, got.Body)
			}
			if tc.blocked && len(got.Blockers) == 0 {
				t.Fatalf("expected blockers for %#v", got)
			}
			if err := destinations.ValidateOperation(got); err != nil {
				t.Fatalf("operation failed validation: %v\n%#v", err, got)
			}
		})
	}
}

func TestPlanBlocksPrivateOrRedactedPublish(t *testing.T) {
	for _, safety := range []destinations.InputSafety{
		{PrivateProvenance: true},
		{RedactionRequired: true},
	} {
		operations, err := Plan(inputResult("dry_run_published", "dry_run_publish", publishBody(), safety))
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if operations[0].OperationType == destinations.OperationCreateNote {
			t.Fatalf("private/redacted publish produced create_note: %#v", operations[0])
		}
	}
}

func TestPlanPublishBodyKeepsSourceNoteSections(t *testing.T) {
	operations, err := Plan(inputResult("dry_run_published", "dry_run_publish", publishBody(), destinations.InputSafety{}))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	for _, section := range []string{"## Snapshot", "## Source Content", "## Key Details", "## Relevance", "## Signals", "## Related Sources", "## Next Action"} {
		if !strings.Contains(operations[0].Body, section) {
			t.Fatalf("publish body missing %q:\n%s", section, operations[0].Body)
		}
	}
}

func TestPlanAttentionDoesNotPretendToBeProcessedSource(t *testing.T) {
	operations, err := Plan(inputResult("attention_ready", "attention_preview", "Need clarification", destinations.InputSafety{}))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if strings.Contains(operations[0].Body, "## Snapshot") {
		t.Fatalf("attention preview pretended to be processed source:\n%s", operations[0].Body)
	}
}

func TestPlanResolvesDuplicateLocatorsAndIdempotency(t *testing.T) {
	first := inputResult("dry_run_published", "dry_run_publish", publishBody(), destinations.InputSafety{})
	first.RecordID = "same"
	first.SourceCandidateID = "same"
	first.IdempotencyKey = "same-key"
	second := first
	second.SourceCandidateID = "same"
	second.IdempotencyKey = "same-key"

	operations, err := PlanBatch([]destinations.InputResult{first, second})
	if err != nil {
		t.Fatalf("plan batch: %v", err)
	}
	if operations[0].OperationType != destinations.OperationCreateNote {
		t.Fatalf("first operation changed: %#v", operations[0])
	}
	if operations[1].OperationType != destinations.OperationBlocked || operations[1].VisibilityLane != destinations.VisibilityBlocked {
		t.Fatalf("second operation not conflict-blocked: %#v", operations[1])
	}
}

func inputResult(state, artifactKind, body string, safety destinations.InputSafety) destinations.InputResult {
	var artifacts []destinations.InputArtifact
	if artifactKind != "" {
		artifacts = []destinations.InputArtifact{{Kind: artifactKind, Body: body}}
	}
	return destinations.InputResult{
		State:             state,
		RecordID:          "record-1",
		SourceCandidateID: "candidate-1",
		IdempotencyKey:    "slack:candidate-1",
		AuthorityIDs:      []string{"WP-5", "DEC-12"},
		Artifacts:         artifacts,
		Safety:            safety,
	}
}

func publishBody() string {
	return "# CODE workflow source\n\n## Snapshot\nUseful source.\n\n## Source Content\n- Source\n\n## Key Details\n- Detail\n\n## Relevance\nRelevant.\n\n## Signals\n- signal\n\n## Related Sources\n- source\n\n## Next Action\nKeep."
}
