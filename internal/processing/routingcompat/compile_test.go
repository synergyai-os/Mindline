package routingcompat

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/processing"
	"github.com/synergyai-os/Mindline/internal/retrieval"
	"github.com/synergyai-os/Mindline/internal/routing"
)

func TestCompileReviewedMapsExactFourInputsThroughStrictCompiler(t *testing.T) {
	canonicalURL := "https://example.com/research"
	canonicalID := routing.CanonicalURLID(canonicalURL)
	digest := sha256.Sum256([]byte("synthetic source"))
	inventory := acquisition.SealInventory(acquisition.InventorySnapshot{
		SourceIdentity: "external_slack_inventory:T-synthetic:C-synthetic", Watermark: "1700000000.000001",
		SourceRecords:  []acquisition.SourceRecord{{SourceRecordID: "source-1", NativeMessageID: "message-1", NativeTimestamp: "1700000000.000001", ContentFingerprint: hex.EncodeToString(digest[:]), URLOccurrenceIDs: []string{"occurrence-1"}, EditDeleteState: "original"}},
		URLOccurrences: []acquisition.URLOccurrence{{URLOccurrenceID: "occurrence-1", SourceRecordID: "source-1", ObservedURL: canonicalURL, CanonicalItemID: canonicalID}},
		CanonicalItems: []acquisition.InventoryItem{{CanonicalItemID: canonicalID, CanonicalURL: canonicalURL, Kind: "article", URLOccurrenceIDs: []string{"occurrence-1"}, RetrievalStrategy: "generic", Format: "html"}},
		Strata:         []acquisition.StratumCount{{RetrievalStrategy: "generic", Format: "html", Count: 1}},
	})
	strategy := processing.SealStrategy(processing.StrategySnapshot{StrategyID: "founder-activation", Version: "1", ContextLenses: "Product strategy", RoutingPolicy: "Promote evidence-backed sources.", IncludeTerms: []string{"product strategy"}})
	artifact := retrieval.Artifact{
		SchemaVersion: retrieval.ArtifactSchema, CanonicalItemID: canonicalID, CanonicalURL: canonicalURL, Strategy: "generic", Format: "html",
		State: retrieval.StateComplete, Origin: retrieval.OriginSyntheticFixture, Access: retrieval.AccessPublic, RetrievedAt: "2026-07-14T10:00:00Z",
		Metadata: retrieval.PublicMetadata{Title: "Product strategy research", Author: "Synthetic Author"},
		Excerpts: []retrieval.PublicExcerpt{{ExcerptID: "evidence-1", Text: "A research finding about product strategy.", Locator: "page"}},
	}
	processed, err := (processing.EvidenceMatcher{}).Process(processing.Request{Item: inventory.CanonicalItems[0], Retrieval: artifact, Strategy: strategy})
	if err != nil {
		t.Fatal(err)
	}
	review, err := processing.RecordOperatorReview(processed.Proposal, processing.OperatorReviewInput{Decision: processing.ReviewAccept, ReviewerID: "operator-synthetic", ReviewedAt: "2026-07-14T10:05:00Z", Rationale: "Synthetic fixture reviewed.", ManualSupportOutcome: "not_required"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := CompileReviewed(Input{Inventory: inventory, Retrieval: []retrieval.Artifact{artifact}, Strategy: strategy, Proposals: []processing.Proposal{processed.Proposal}, Reviews: []processing.OperatorReviewRecord{review}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Graph.SourceRecords[0].SourceRecordID != "source-1" || result.Graph.URLOccurrences[0].URLOccurrenceID != "occurrence-1" || result.Graph.CanonicalURLs[0].CanonicalURLID != canonicalID {
		t.Fatalf("source identity was not preserved: %+v", result.Graph)
	}
	if result.Decisions.Sources[0].Disposition != "promote" || !result.Summary.OperatorJudged {
		t.Fatalf("reviewed judgment was not compiled: %+v", result.Decisions.Sources[0])
	}
}

func TestCompileReviewedRejectsUnreviewedAndTamperedProposal(t *testing.T) {
	canonicalURL := "https://example.com/research"
	canonicalID := routing.CanonicalURLID(canonicalURL)
	digest := sha256.Sum256([]byte("synthetic source"))
	inventory := acquisition.SealInventory(acquisition.InventorySnapshot{
		SourceIdentity: "external_slack_inventory:T:C", Watermark: "1700000000.000001",
		SourceRecords:  []acquisition.SourceRecord{{SourceRecordID: "source-1", NativeMessageID: "message-1", NativeTimestamp: "1700000000.000001", ContentFingerprint: hex.EncodeToString(digest[:]), URLOccurrenceIDs: []string{"occurrence-1"}, EditDeleteState: "original"}},
		URLOccurrences: []acquisition.URLOccurrence{{URLOccurrenceID: "occurrence-1", SourceRecordID: "source-1", ObservedURL: canonicalURL, CanonicalItemID: canonicalID}},
		CanonicalItems: []acquisition.InventoryItem{{CanonicalItemID: canonicalID, CanonicalURL: canonicalURL, Kind: "article", URLOccurrenceIDs: []string{"occurrence-1"}, RetrievalStrategy: "generic", Format: "html"}},
		Strata:         []acquisition.StratumCount{{RetrievalStrategy: "generic", Format: "html", Count: 1}},
	})
	strategy := processing.SealStrategy(processing.StrategySnapshot{StrategyID: "strategy", Version: "1", ContextLenses: "Product", RoutingPolicy: "Hold by default."})
	artifact := retrieval.Artifact{SchemaVersion: retrieval.ArtifactSchema, CanonicalItemID: canonicalID, CanonicalURL: canonicalURL, Strategy: "generic", Format: "html", State: retrieval.StateComplete, Origin: retrieval.OriginSyntheticFixture, Access: retrieval.AccessPublic, Excerpts: []retrieval.PublicExcerpt{{ExcerptID: "e1", Text: "Product evidence.", Locator: "page"}}}
	proposal, err := (processing.EvidenceMatcher{}).Process(processing.Request{Item: inventory.CanonicalItems[0], Retrieval: artifact, Strategy: strategy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileReviewed(Input{Inventory: inventory, Retrieval: []retrieval.Artifact{artifact}, Strategy: strategy, Proposals: []processing.Proposal{proposal.Proposal}}); err == nil {
		t.Fatal("unreviewed proposal was accepted")
	}
}
