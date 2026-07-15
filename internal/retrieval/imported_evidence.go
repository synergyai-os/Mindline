package retrieval

import (
	"context"
	"errors"

	"github.com/synergyai-os/Mindline/internal/acquisition"
)

type ImportedEvidenceAdapter struct {
	byCanonicalItem map[string]acquisition.ImportedEvidence
}

func NewImportedEvidenceAdapter(evidence []acquisition.ImportedEvidence) (*ImportedEvidenceAdapter, error) {
	adapter := &ImportedEvidenceAdapter{byCanonicalItem: map[string]acquisition.ImportedEvidence{}}
	for _, item := range evidence {
		if item.CanonicalItemID == "" || adapter.byCanonicalItem[item.CanonicalItemID].CanonicalItemID != "" {
			return nil, errors.New("invalid or duplicate imported evidence identity")
		}
		adapter.byCanonicalItem[item.CanonicalItemID] = item
	}
	return adapter, nil
}

func (adapter *ImportedEvidenceAdapter) Retrieve(_ context.Context, request Request) (Artifact, error) {
	item, exists := adapter.byCanonicalItem[request.CanonicalItemID]
	if !exists {
		return MissingArtifact(request, StateNotAttempted, AccessUnsupported, OriginImportedReplay, "no imported evidence"), nil
	}
	if item.CanonicalURL != request.CanonicalURL {
		return Artifact{}, errors.New("imported evidence canonical identity mismatch")
	}
	access := AccessClass(item.AccessClass)
	if access == "" {
		access = AccessPublic
	}
	artifact := Artifact{
		SchemaVersion: ArtifactSchema, CanonicalItemID: item.CanonicalItemID, CanonicalURL: item.CanonicalURL,
		Strategy: request.Strategy, Format: request.Format, State: State(item.State), Origin: OriginImportedReplay, Access: access,
		RetrievedAt: item.RetrievedAt,
		Metadata:    PublicMetadata{Title: item.Metadata.Title, Author: item.Metadata.Author, PublishedAt: item.Metadata.PublishedAt},
		Missingness: append([]string(nil), item.Missingness...),
		SecretLike:  item.SecretLike,
	}
	for _, excerpt := range item.Excerpts {
		artifact.Excerpts = append(artifact.Excerpts, PublicExcerpt{ExcerptID: excerpt.ExcerptID, Text: excerpt.Text, Locator: excerpt.Locator})
	}
	for _, related := range item.RelatedURLs {
		artifact.RelatedURLs = append(artifact.RelatedURLs, RelatedURL{URL: related.URL, Relation: related.Relation, DiscoveryEvidenceRef: related.DiscoveryEvidenceRef, SemanticallyRelevant: related.SemanticallyRelevant})
	}
	if err := ValidateArtifact(artifact); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}
