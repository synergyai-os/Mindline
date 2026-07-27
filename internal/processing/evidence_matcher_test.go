package processing

import (
	"reflect"
	"testing"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/retrieval"
)

func TestEvidenceMatcherIsDeterministicAndReviewBound(t *testing.T) {
	strategy := testStrategy()
	item := acquisition.InventoryItem{CanonicalItemID: "item-1", CanonicalURL: "https://example.com/research", Kind: "article", RetrievalStrategy: "generic", Format: "html"}
	artifact := retrieval.Artifact{
		SchemaVersion: retrieval.ArtifactSchema, CanonicalItemID: item.CanonicalItemID, CanonicalURL: item.CanonicalURL, Strategy: item.RetrievalStrategy, Format: item.Format,
		State: retrieval.StateComplete, Origin: retrieval.OriginSyntheticFixture, Access: retrieval.AccessPublic,
		Metadata: retrieval.PublicMetadata{Title: "Product Brain research", Author: "Synthetic Author"},
		Excerpts: []retrieval.PublicExcerpt{{ExcerptID: "evidence-1", Text: "A research finding about product strategy and AI organizations.", Locator: "page"}},
	}
	matcher := EvidenceMatcher{}
	first, err := matcher.Process(Request{Item: item, Retrieval: artifact, Strategy: strategy})
	if err != nil {
		t.Fatal(err)
	}
	second, err := matcher.Process(Request{Item: item, Retrieval: artifact, Strategy: strategy})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Proposal.Judgment.Disposition != "promote" || first.Proposal.Judgment.SemanticAssessment.PrimaryRole != "evidence_backed_finding" {
		t.Fatalf("deterministic evidence proposal mismatch: first=%+v second=%+v", first, second)
	}
	review, err := RecordOperatorReview(first.Proposal, OperatorReviewInput{Decision: ReviewAccept, ReviewerID: "operator-synthetic", ReviewedAt: "2026-07-14T10:00:00Z", Rationale: "Synthetic fixture judgment reviewed.", ManualSupportOutcome: "not_required"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateOperatorReview(review, first.Proposal); err != nil {
		t.Fatal(err)
	}
	tampered := review
	tampered.Judgment.Disposition = "archive"
	if err := ValidateOperatorReview(tampered, first.Proposal); err == nil {
		t.Fatal("tampered immutable review accepted")
	}
}

func TestEvidenceMatcherKeepsIncompleteAndSecretLikeSourcesManual(t *testing.T) {
	strategy := testStrategy()
	item := acquisition.InventoryItem{CanonicalItemID: "item-2", CanonicalURL: "https://example.com/private", Kind: "article", RetrievalStrategy: "generic", Format: "html"}
	artifact := retrieval.MissingArtifact(retrieval.Request{CanonicalItemID: item.CanonicalItemID, CanonicalURL: item.CanonicalURL, Strategy: item.RetrievalStrategy, Format: item.Format}, retrieval.StateInaccessible, retrieval.AccessAuthenticated, retrieval.OriginSyntheticFixture, "authentication required")
	artifact.SecretLike = true
	result, err := (EvidenceMatcher{}).Process(Request{Item: item, Retrieval: artifact, Strategy: strategy})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Proposal.RequiresManualReview || result.Proposal.Judgment.Disposition != "hold" || result.Proposal.Judgment.SemanticAssessment.PrimaryRole != "unknown" {
		t.Fatalf("unsafe evidence was not held for review: %+v", result.Proposal)
	}
	forged := result.Proposal
	forged.Judgment = Judgment{Disposition: "promote"}
	forged.Fingerprint = acquisition.Fingerprint(forged)
	if err := ValidateProposal(forged); err == nil || err.Error() != "manual retrieval cannot be promoted without new evidence" {
		t.Fatalf("manual retrieval promotion invariant was not explicit: %v", err)
	}
}

func testStrategy() StrategySnapshot {
	return SealStrategy(StrategySnapshot{
		StrategyID: "founder-activation", Version: "1", ContextLenses: "Product Brain landscape\nAI-dominant organizational design",
		RoutingPolicy: "Promote only relevant evidence-backed sources.", SignificantTerms: []string{"product", "brain", "ai", "organization"}, IncludeTerms: []string{"product strategy"}, ExcludeTerms: []string{"consumer advertising"},
	})
}
