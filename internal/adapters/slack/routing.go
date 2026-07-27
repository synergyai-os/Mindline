package slack

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/routing"
)

// CompileRouting converts Slack-native records into Mindline's source-neutral
// graph contract before handing semantic compilation to the routing layer.
func CompileRouting(payload Payload, artifacts routing.LinkArtifacts, profile routing.LensProfile, judgments routing.Judgments) (routing.Result, error) {
	graph, err := buildRoutingGraph(payload, artifacts)
	if err != nil {
		return routing.Result{}, err
	}
	return routing.CompileGraph(graph, artifacts, profile, judgments)
}

func buildRoutingGraph(payload Payload, artifacts routing.LinkArtifacts) (routing.SourceGraph, error) {
	channelID := strings.TrimSpace(payload.Source.ChannelID)
	if channelID == "" {
		return routing.SourceGraph{}, errors.New("missing source.channel_id")
	}
	artifactByCanonical := map[string]routing.LinkArtifact{}
	for _, artifact := range artifacts.Items {
		safeURL, storageState, err := routing.PrepareURLForStorage(artifact.CanonicalURL)
		if err != nil || storageState == routing.URLStorageSensitiveRedacted {
			return routing.SourceGraph{}, errors.New("unsafe enrichment canonical URL")
		}
		canonical, err := routing.CanonicalizeURL(safeURL)
		if err != nil {
			return routing.SourceGraph{}, errors.New("invalid enrichment canonical URL")
		}
		identity := routingCanonicalIdentity(canonical)
		if _, exists := artifactByCanonical[identity]; exists {
			return routing.SourceGraph{}, errors.New("duplicate enrichment canonical URL")
		}
		artifact.CanonicalURL = canonical
		artifactByCanonical[identity] = artifact
	}

	graph := routing.SourceGraph{SchemaVersion: routing.SourceGraphSchema, Adapter: routing.AdapterRef{Kind: "slack", Version: "v0.1"}}
	canonicalByIdentity := map[string]routing.CanonicalURL{}
	for index, message := range payload.Messages {
		occurredAt, err := routingOccurredAt(message)
		if err != nil {
			return routing.SourceGraph{}, err
		}
		urls := routing.URLPattern.FindAllString(message.Text, -1)
		if len(urls) != 1 {
			return routing.SourceGraph{}, fmt.Errorf("Slack routing record %d must contain exactly one URL", index+1)
		}
		observed, storageState, err := routing.PrepareURLForStorage(urls[0])
		if err != nil || storageState == routing.URLStorageSensitiveRedacted {
			return routing.SourceGraph{}, fmt.Errorf("unsafe Slack routing URL at record %d", index+1)
		}
		canonical, err := routing.CanonicalizeURL(observed)
		if err != nil {
			return routing.SourceGraph{}, fmt.Errorf("invalid Slack routing URL at record %d", index+1)
		}
		identity := routingCanonicalIdentity(canonical)
		artifact, ok := artifactByCanonical[identity]
		if !ok {
			return routing.SourceGraph{}, fmt.Errorf("missing enrichment artifact for Slack routing record %d", index+1)
		}
		canonical = artifact.CanonicalURL
		canonicalID := routing.CanonicalURLID(canonical)
		recordID := routingStableID("src-", channelID, message.TS, fmt.Sprint(index+1))
		occurrenceID := routingStableID("occ-", recordID, observed, fmt.Sprint(index+1))
		graph.SourceRecords = append(graph.SourceRecords, routing.SourceRecord{
			SourceRecordID: recordID, SourceKind: "message", OccurredAt: occurredAt,
			RawProvenanceRef: fmt.Sprintf("adapter-local://slack/capture-%03d", index+1),
			URLOccurrenceIDs: []string{occurrenceID},
		})
		graph.URLOccurrences = append(graph.URLOccurrences, routing.URLOccurrence{URLOccurrenceID: occurrenceID, SourceRecordID: recordID, ObservedURL: observed, CanonicalURLID: canonicalID})
		graph.Edges = append(graph.Edges, routing.GraphEdge{EdgeID: routingStableID("edge-", recordID, occurrenceID, canonicalID), Type: "source_record_contains_url", From: recordID, To: canonicalID, EvidenceRefs: []string{occurrenceID}})
		if _, exists := canonicalByIdentity[identity]; !exists {
			canonicalByIdentity[identity] = routing.CanonicalURL{CanonicalURLID: canonicalID, CanonicalURL: canonical, Kind: routing.URLKind(canonical), Depth: 0, Discovery: "source_occurrence", EnrichmentState: artifact.State, Missingness: append([]string{}, artifact.Missingness...)}
			graph.CanonicalURLs = append(graph.CanonicalURLs, canonicalByIdentity[identity])
		}
	}

	primaryCount := len(graph.CanonicalURLs)
	for index := 0; index < primaryCount; index++ {
		parent := graph.CanonicalURLs[index]
		artifact := artifactByCanonical[routingCanonicalIdentity(parent.CanonicalURL)]
		for relatedIndex, related := range artifact.RelatedURLs {
			if !related.SemanticallyRelevant {
				continue
			}
			if related.Relation != "source_links_to" || !slackArtifactHasEvidence(artifact, related.DiscoveryEvidenceRef) {
				return routing.SourceGraph{}, errors.New("invalid related URL evidence")
			}
			safeRelated, storageState, err := routing.PrepareURLForStorage(related.URL)
			if err != nil || storageState == routing.URLStorageSensitiveRedacted {
				return routing.SourceGraph{}, errors.New("unsafe related URL")
			}
			canonical, err := routing.CanonicalizeURL(safeRelated)
			if err != nil {
				return routing.SourceGraph{}, errors.New("invalid related URL")
			}
			identity := routingCanonicalIdentity(canonical)
			childArtifact, ok := artifactByCanonical[identity]
			if !ok {
				return routing.SourceGraph{}, errors.New("missing enrichment artifact for related URL")
			}
			if existing, exists := canonicalByIdentity[identity]; exists {
				if existing.Depth != 1 || existing.ParentCanonicalURLID != parent.CanonicalURLID {
					return routing.SourceGraph{}, errors.New("related URL has ambiguous graph identity")
				}
				continue
			}
			canonical = childArtifact.CanonicalURL
			child := routing.CanonicalURL{CanonicalURLID: routing.CanonicalURLID(canonical), CanonicalURL: canonical, Kind: routing.URLKind(canonical), Depth: 1, ParentCanonicalURLID: parent.CanonicalURLID, Discovery: "enrichment_related_url", EnrichmentState: childArtifact.State, Missingness: append([]string{}, childArtifact.Missingness...)}
			canonicalByIdentity[identity] = child
			graph.CanonicalURLs = append(graph.CanonicalURLs, child)
			graph.Edges = append(graph.Edges, routing.GraphEdge{EdgeID: routingStableID("edge-", parent.CanonicalURLID, child.CanonicalURLID, fmt.Sprint(relatedIndex+1)), Type: "source_links_to", From: parent.CanonicalURLID, To: child.CanonicalURLID, EvidenceRefs: []string{related.DiscoveryEvidenceRef}})
		}
	}
	return graph, nil
}

func routingOccurredAt(message Message) (string, error) {
	if strings.TrimSpace(message.CapturedAt) != "" {
		if _, err := time.Parse(time.RFC3339, strings.TrimSpace(message.CapturedAt)); err != nil {
			return "", fmt.Errorf("invalid captured_at for Slack routing record")
		}
		return strings.TrimSpace(message.CapturedAt), nil
	}
	parts := strings.Split(message.TS, ".")
	seconds, err := time.ParseDuration(parts[0] + "s")
	if err != nil {
		return "", fmt.Errorf("invalid Slack timestamp %q", message.TS)
	}
	return time.Unix(int64(seconds.Seconds()), 0).UTC().Format(time.RFC3339), nil
}

func routingCanonicalIdentity(canonical string) string {
	parsed, err := url.Parse(canonical)
	if err == nil && strings.EqualFold(parsed.Hostname(), "github.com") {
		parsed.Path = strings.ToLower(parsed.Path)
		return parsed.String()
	}
	return canonical
}

func routingStableID(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return prefix + hex.EncodeToString(digest[:])[:20]
}

func slackArtifactHasEvidence(artifact routing.LinkArtifact, ref string) bool {
	for _, excerpt := range artifact.PublicExcerpts {
		if excerpt.ExcerptID == ref {
			return true
		}
	}
	return false
}
