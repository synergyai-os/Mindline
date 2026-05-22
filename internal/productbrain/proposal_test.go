package productbrain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProposalIdentityAndAuditFields(t *testing.T) {
	proposalID := BuildProposalID("run-0123456789abcdef", "review:item/unsafe", IntentDurableDecision, "decisions")
	if proposalID == "" || strings.ContainsAny(proposalID, `/\`) || strings.Contains(proposalID, "..") {
		t.Fatalf("unsafe proposal id: %q", proposalID)
	}
	if proposalID != BuildProposalID("run-0123456789abcdef", "review:item/unsafe", IntentDurableDecision, "decisions") {
		t.Fatalf("proposal id is not deterministic")
	}

	externalRef := BuildExternalRef("slack-DSELF-1710000000000001", IntentDurableDecision)
	if externalRef.Source != "mindline" {
		t.Fatalf("externalRef source = %q", externalRef.Source)
	}
	if strings.Contains(externalRef.ID, "run-0123456789abcdef") || strings.Contains(externalRef.ID, "review-item") {
		t.Fatalf("externalRef id contains run/review identity: %+v", externalRef)
	}
	if !strings.Contains(externalRef.ID, "slack-DSELF-1710000000000001") || !strings.Contains(externalRef.ID, "durable_decision") {
		t.Fatalf("externalRef id does not preserve source/object identity: %+v", externalRef)
	}

	idempotencyKey := BuildIdempotencyKey("run-0123456789abcdef", proposalID)
	if idempotencyKey == externalRef.ID {
		t.Fatalf("idempotency key must be distinct from externalRef id")
	}
	if !strings.HasSuffix(idempotencyKey, ":productbrain-proposal/v0.1") {
		t.Fatalf("idempotency key missing schema-qualified suffix: %q", idempotencyKey)
	}

	proposal := NewProposal(ProposalInput{
		RunID:                "run-0123456789abcdef",
		SourceReviewItemID:   "review-item",
		SourceCandidateID:    "slack-DSELF-1710000000000001",
		Intent:               IntentDurableDecision,
		Status:               ProposalStatusReady,
		TargetCollectionSlug: "decisions",
		EntryName:            "Example decision",
		WorkflowStatus:       "pending",
		Data:                 map[string]string{"rationale": "Safe rationale."},
	})
	if proposal.Actor.Kind != "integration" || proposal.Actor.Authority != "mindline" {
		t.Fatalf("unexpected actor: %+v", proposal.Actor)
	}
	if proposal.Provenance.Surface != "integration" || proposal.Provenance.CapturePath != "integration:mindline" {
		t.Fatalf("unexpected provenance: %+v", proposal.Provenance)
	}
	wantAuthority := []string{"PROD-1", "DOMAIN-1", "DEC-15", "WP-8", "WP-9"}
	if strings.Join(proposal.AuthorityIDs, ",") != strings.Join(wantAuthority, ",") {
		t.Fatalf("authority_ids = %v, want %v", proposal.AuthorityIDs, wantAuthority)
	}
}

func TestExternalRefStableAcrossRunsWhileIdempotencyKeyIsRunScoped(t *testing.T) {
	firstProposalID := BuildProposalID("run-first", "review-item-1", IntentDurableDecision, "decisions")
	secondProposalID := BuildProposalID("run-second", "review-item-2", IntentDurableDecision, "decisions")

	firstRef := BuildExternalRef("slack-DSELF-1710000000000001", IntentDurableDecision)
	secondRef := BuildExternalRef("slack-DSELF-1710000000000001", IntentDurableDecision)
	if firstRef != secondRef {
		t.Fatalf("externalRef changed across runs: %+v != %+v", firstRef, secondRef)
	}

	firstKey := BuildIdempotencyKey("run-first", firstProposalID)
	secondKey := BuildIdempotencyKey("run-second", secondProposalID)
	if firstKey == secondKey {
		t.Fatalf("idempotency key must remain run/proposal scoped")
	}
	if !strings.HasPrefix(firstKey, "mindline:proposal:") || !strings.HasPrefix(secondKey, "mindline:proposal:") {
		t.Fatalf("idempotency keys must be source-prefixed: %q %q", firstKey, secondKey)
	}
}

func TestBlockedProposalOmitsOperation(t *testing.T) {
	proposal := NewProposal(ProposalInput{
		RunID:              "run-0123456789abcdef",
		SourceReviewItemID: "review-blocked",
		SourceCandidateID:  "slack-DSELF-1710000000000001",
		Intent:             IntentOpenTension,
		Status:             ProposalStatusBlocked,
		Blockers:           []Blocker{{Code: "missing_intent_mapping", Message: "No mapping."}},
	})
	data, err := json.Marshal(proposal)
	if err != nil {
		t.Fatalf("marshal proposal: %v", err)
	}
	if strings.Contains(string(data), `"operation"`) {
		t.Fatalf("blocked proposal should omit operation, got %s", data)
	}
}
