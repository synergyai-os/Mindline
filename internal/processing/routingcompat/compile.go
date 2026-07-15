package routingcompat

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/processing"
	"github.com/synergyai-os/Mindline/internal/retrieval"
	"github.com/synergyai-os/Mindline/internal/routing"
)

type Input struct {
	Inventory acquisition.InventorySnapshot
	Retrieval []retrieval.Artifact
	Strategy  processing.StrategySnapshot
	Proposals []processing.Proposal
	Reviews   []processing.OperatorReviewRecord
}

// CompileReviewed is the only activation compatibility path into the strict
// v0.1 routing compiler. It transforms the four validated inputs and delegates
// final authority to routing.CompileGraph; it never constructs routing.Result.
func CompileReviewed(input Input) (routing.Result, error) {
	if err := acquisition.ValidateInventory(input.Inventory); err != nil {
		return routing.Result{}, err
	}
	if err := processing.ValidateStrategy(input.Strategy); err != nil {
		return routing.Result{}, err
	}
	artifactsByItem, err := validateRetrieval(input.Inventory, input.Retrieval)
	if err != nil {
		return routing.Result{}, err
	}
	proposalsByItem, reviewsByItem, judgedAt, err := validateReviews(input.Inventory, input.Strategy, input.Proposals, input.Reviews)
	if err != nil {
		return routing.Result{}, err
	}
	routableInventory := destinationRoutableInventory(input.Inventory)

	graph, err := compileGraph(routableInventory, artifactsByItem)
	if err != nil {
		return routing.Result{}, err
	}
	linkArtifacts := compileArtifacts(routableInventory, artifactsByItem)
	profile := compileProfile(input.Strategy)
	judgments := compileJudgments(routableInventory, profile, proposalsByItem, reviewsByItem, judgedAt)
	return routing.CompileGraph(graph, linkArtifacts, profile, judgments)
}

// destinationRoutableInventory excludes content-free sensitive-redacted items
// only after their retrieval and operator-review coverage has been validated.
// They remain authoritative in the activation inventory and review ledger but
// cannot enter the strict URL graph or any destination outbox.
func destinationRoutableInventory(inventory acquisition.InventorySnapshot) acquisition.InventorySnapshot {
	allowedItems := map[string]bool{}
	allowedOccurrences := map[string]bool{}
	result := acquisition.InventorySnapshot{SourceIdentity: inventory.SourceIdentity, Watermark: inventory.Watermark}
	for _, item := range inventory.CanonicalItems {
		if item.AccessState == "sensitive_redacted" {
			continue
		}
		result.CanonicalItems = append(result.CanonicalItems, item)
		allowedItems[item.CanonicalItemID] = true
		for _, occurrenceID := range item.URLOccurrenceIDs {
			allowedOccurrences[occurrenceID] = true
		}
	}
	for _, occurrence := range inventory.URLOccurrences {
		if allowedItems[occurrence.CanonicalItemID] && allowedOccurrences[occurrence.URLOccurrenceID] {
			result.URLOccurrences = append(result.URLOccurrences, occurrence)
		}
	}
	for _, record := range inventory.SourceRecords {
		copyRecord := record
		copyRecord.URLOccurrenceIDs = nil
		for _, occurrenceID := range record.URLOccurrenceIDs {
			if allowedOccurrences[occurrenceID] {
				copyRecord.URLOccurrenceIDs = append(copyRecord.URLOccurrenceIDs, occurrenceID)
			}
		}
		if len(copyRecord.URLOccurrenceIDs) > 0 {
			result.SourceRecords = append(result.SourceRecords, copyRecord)
		}
	}
	return result
}

func compileGraph(inventory acquisition.InventorySnapshot, artifacts map[string]retrieval.Artifact) (routing.SourceGraph, error) {
	adapterKind := inventory.SourceIdentity
	if separator := strings.IndexByte(adapterKind, ':'); separator >= 0 {
		adapterKind = adapterKind[:separator]
	}
	graph := routing.SourceGraph{SchemaVersion: routing.SourceGraphSchema, Adapter: routing.AdapterRef{Kind: adapterKind, Version: "v1"}}
	for _, record := range inventory.SourceRecords {
		occurredAt, err := acquisition.NativeTimestampToRFC3339(record.NativeTimestamp)
		if err != nil {
			return routing.SourceGraph{}, err
		}
		graph.SourceRecords = append(graph.SourceRecords, routing.SourceRecord{
			SourceRecordID: record.SourceRecordID, SourceKind: "message", OccurredAt: occurredAt,
			RawProvenanceRef: "source-record://" + record.SourceRecordID + "#sha256=" + record.ContentFingerprint,
			URLOccurrenceIDs: append([]string(nil), record.URLOccurrenceIDs...),
		})
	}
	for _, occurrence := range inventory.URLOccurrences {
		graph.URLOccurrences = append(graph.URLOccurrences, routing.URLOccurrence{
			URLOccurrenceID: occurrence.URLOccurrenceID, SourceRecordID: occurrence.SourceRecordID,
			ObservedURL: occurrence.ObservedURL, CanonicalURLID: occurrence.CanonicalItemID,
		})
		graph.Edges = append(graph.Edges, routing.GraphEdge{
			EdgeID: stableID("edge-", occurrence.SourceRecordID, occurrence.URLOccurrenceID, occurrence.CanonicalItemID),
			Type:   "source_record_contains_url", From: occurrence.SourceRecordID, To: occurrence.CanonicalItemID, EvidenceRefs: []string{occurrence.URLOccurrenceID},
		})
	}
	for _, item := range inventory.CanonicalItems {
		if item.CanonicalItemID != routing.CanonicalURLID(item.CanonicalURL) {
			return routing.SourceGraph{}, fmt.Errorf("canonical item %s is incompatible with strict v0.1 identity", item.CanonicalItemID)
		}
		artifact := artifacts[item.CanonicalItemID]
		graph.CanonicalURLs = append(graph.CanonicalURLs, routing.CanonicalURL{
			CanonicalURLID: item.CanonicalItemID, CanonicalURL: item.CanonicalURL, Kind: routingKind(item.Kind),
			Depth: 0, Discovery: "source_occurrence", EnrichmentState: string(artifact.State), Missingness: append([]string(nil), artifact.Missingness...),
		})
	}
	return graph, nil
}

func compileArtifacts(inventory acquisition.InventorySnapshot, artifacts map[string]retrieval.Artifact) routing.LinkArtifacts {
	result := routing.LinkArtifacts{SchemaVersion: routing.LinkArtifactsSchema}
	for _, item := range inventory.CanonicalItems {
		artifact := artifacts[item.CanonicalItemID]
		compiled := routing.LinkArtifact{
			CanonicalURL: artifact.CanonicalURL, RetrievedAt: artifact.RetrievedAt, State: string(artifact.State),
			PublicMetadata: routing.PublicMetadata{Title: artifact.Metadata.Title, Author: artifact.Metadata.Author, PublishedAt: artifact.Metadata.PublishedAt},
			Missingness:    append([]string(nil), artifact.Missingness...),
		}
		for _, excerpt := range artifact.Excerpts {
			compiled.PublicExcerpts = append(compiled.PublicExcerpts, routing.PublicExcerpt{ExcerptID: excerpt.ExcerptID, Text: excerpt.Text, Locator: excerpt.Locator})
		}
		for _, related := range artifact.RelatedURLs {
			compiled.RelatedURLs = append(compiled.RelatedURLs, routing.RelatedURL{URL: related.URL, Relation: related.Relation, DiscoveryEvidenceRef: related.DiscoveryEvidenceRef, SemanticallyRelevant: related.SemanticallyRelevant})
		}
		result.Items = append(result.Items, compiled)
	}
	return result
}

func compileProfile(strategy processing.StrategySnapshot) routing.LensProfile {
	profile := routing.LensProfile{SchemaVersion: routing.LensProfileSchemaVersion, ProfileID: strategy.StrategyID, ProfileVersion: strategy.Version}
	for _, lens := range processing.ContextLenses(strategy) {
		profile.Lenses = append(profile.Lenses, routing.Lens{
			LensID: processing.ContextLensID(lens), Name: lens, Question: lens,
			Include: append([]string(nil), strategy.IncludeTerms...), Exclude: append([]string(nil), strategy.ExcludeTerms...),
		})
	}
	return profile
}

func compileJudgments(inventory acquisition.InventorySnapshot, profile routing.LensProfile, proposals map[string]processing.Proposal, reviews map[string]processing.OperatorReviewRecord, judgedAt string) routing.Judgments {
	result := routing.Judgments{SchemaVersion: routing.JudgmentsSchema, JudgmentMethod: "operator_agent_review", JudgedAt: judgedAt, ProfileID: profile.ProfileID, ProfileVersion: profile.ProfileVersion}
	for _, item := range inventory.CanonicalItems {
		review := reviews[item.CanonicalItemID]
		proposal := proposals[item.CanonicalItemID]
		judgment := review.Judgment
		if review.Decision == processing.ReviewReject {
			judgment = rejectedJudgment(profile)
		}
		compiled := routing.SourceJudgment{
			CanonicalURLID: item.CanonicalItemID, Disposition: judgment.Disposition, DispositionRationale: judgment.DispositionRationale,
			SemanticAssessment: routing.SemanticAssessment{
				PrimaryRole: judgment.SemanticAssessment.PrimaryRole, Summary: judgment.SemanticAssessment.Summary, Confidence: judgment.SemanticAssessment.Confidence,
				EvidenceRefs: append([]string(nil), judgment.SemanticAssessment.EvidenceRefs...), Missingness: append([]string(nil), judgment.SemanticAssessment.Missingness...),
			},
		}
		for _, lens := range judgment.LensResults {
			compiled.LensResults = append(compiled.LensResults, routing.LensResult{LensID: lens.LensID, Result: lens.Result, Confidence: lens.Confidence, Rationale: lens.Rationale, EvidenceRefs: append([]string(nil), lens.EvidenceRefs...), Missingness: append([]string(nil), lens.Missingness...)})
		}
		for _, node := range judgment.SemanticNodes {
			compiled.SemanticNodes = append(compiled.SemanticNodes, routing.SemanticNode{SemanticNodeID: node.SemanticNodeID, Role: node.Role, Name: node.Name, Description: node.Description, Confidence: node.Confidence, LensRefs: append([]string(nil), node.LensRefs...), EvidenceRefs: append([]string(nil), node.EvidenceRefs...), Attributes: cloneAttributes(node.Attributes)})
		}
		for _, edge := range judgment.SemanticEdges {
			compiled.SemanticEdges = append(compiled.SemanticEdges, routing.SemanticEdge{From: edge.From, Type: edge.Type, To: edge.To, Rationale: edge.Rationale, EvidenceRefs: append([]string(nil), edge.EvidenceRefs...)})
		}
		_ = proposal // Proposal authority was checked before transformation.
		result.Sources = append(result.Sources, compiled)
	}
	return result
}

func validateRetrieval(inventory acquisition.InventorySnapshot, artifacts []retrieval.Artifact) (map[string]retrieval.Artifact, error) {
	items := map[string]acquisition.InventoryItem{}
	for _, item := range inventory.CanonicalItems {
		items[item.CanonicalItemID] = item
	}
	result := map[string]retrieval.Artifact{}
	for _, artifact := range artifacts {
		item, ok := items[artifact.CanonicalItemID]
		if !ok || result[artifact.CanonicalItemID].CanonicalItemID != "" || artifact.CanonicalURL != item.CanonicalURL || artifact.Strategy != item.RetrievalStrategy || artifact.Format != item.Format {
			return nil, errors.New("retrieval coverage or identity mismatch")
		}
		if err := retrieval.ValidateArtifact(artifact); err != nil {
			return nil, err
		}
		if artifact.Origin == retrieval.OriginLiveRetrieval {
			return nil, retrieval.ErrLiveTransportDisabled
		}
		result[artifact.CanonicalItemID] = artifact
	}
	if len(result) != len(items) {
		return nil, errors.New("incomplete retrieval coverage")
	}
	return result, nil
}

func validateReviews(inventory acquisition.InventorySnapshot, strategy processing.StrategySnapshot, proposals []processing.Proposal, reviews []processing.OperatorReviewRecord) (map[string]processing.Proposal, map[string]processing.OperatorReviewRecord, string, error) {
	items := map[string]bool{}
	for _, item := range inventory.CanonicalItems {
		items[item.CanonicalItemID] = true
	}
	proposalByItem := map[string]processing.Proposal{}
	for _, proposal := range proposals {
		if !items[proposal.CanonicalItemID] || proposalByItem[proposal.CanonicalItemID].CanonicalItemID != "" || proposal.StrategyFingerprint != strategy.Fingerprint {
			return nil, nil, "", errors.New("processing proposal coverage or strategy mismatch")
		}
		if err := processing.ValidateProposal(proposal); err != nil {
			return nil, nil, "", err
		}
		proposalByItem[proposal.CanonicalItemID] = proposal
	}
	reviewByItem := map[string]processing.OperatorReviewRecord{}
	var timestamps []string
	for _, review := range reviews {
		proposal, ok := proposalByItem[review.CanonicalItemID]
		if !ok || reviewByItem[review.CanonicalItemID].CanonicalItemID != "" {
			return nil, nil, "", errors.New("operator review coverage mismatch")
		}
		if err := processing.ValidateOperatorReview(review, proposal); err != nil {
			return nil, nil, "", err
		}
		reviewByItem[review.CanonicalItemID] = review
		timestamps = append(timestamps, review.ReviewedAt)
	}
	if len(proposalByItem) != len(items) || len(reviewByItem) != len(items) {
		return nil, nil, "", errors.New("every canonical item requires exactly one proposal and immutable operator review")
	}
	if len(timestamps) == 0 {
		return proposalByItem, reviewByItem, strategy.CreatedAt, nil
	}
	sort.Strings(timestamps)
	return proposalByItem, reviewByItem, timestamps[len(timestamps)-1], nil
}

func rejectedJudgment(profile routing.LensProfile) processing.Judgment {
	judgment := processing.Judgment{
		SemanticAssessment: processing.SemanticAssessment{PrimaryRole: "unknown", Confidence: 1, Missingness: []string{"operator_rejected_proposal"}},
		Disposition:        "hold", DispositionRationale: "The operator rejected the processing proposal; no destination operation is allowed.",
	}
	for _, lens := range profile.Lenses {
		judgment.LensResults = append(judgment.LensResults, processing.LensResult{LensID: lens.LensID, Result: "unknown", Confidence: 1, Rationale: "The operator rejected the processing proposal.", Missingness: []string{"operator_rejected_proposal"}})
	}
	return judgment
}

func routingKind(kind string) string {
	switch kind {
	case "github_repository", "linkedin_post", "linkedin_article", "youtube_video", "article", "pdf", "generic_web", "unknown":
		return kind
	case "github":
		return "github_repository"
	case "youtube":
		return "youtube_video"
	case "document":
		return "pdf"
	default:
		return "generic_web"
	}
}

func stableID(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + hex.EncodeToString(digest[:])[:20]
}

func cloneAttributes(attributes map[string]any) map[string]any {
	if attributes == nil {
		return nil
	}
	clone := make(map[string]any, len(attributes))
	for key, value := range attributes {
		clone[key] = value
	}
	return clone
}
