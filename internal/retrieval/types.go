package retrieval

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/synergyai-os/Mindline/internal/routing"
)

const (
	ArtifactSchema                     = "mindline-retrieval-artifact/v0.1"
	SensitiveRedactedMissingnessReason = "secret_bearing_url_withheld"
)

type State string

const (
	StateComplete     State = "complete"
	StatePartial      State = "partial"
	StateInaccessible State = "inaccessible"
	StateFailed       State = "failed"
	StateNotAttempted State = "not_attempted"
)

type EvidenceOrigin string

const (
	OriginImportedReplay   EvidenceOrigin = "imported_replay"
	OriginLiveRetrieval    EvidenceOrigin = "live_retrieval"
	OriginSyntheticFixture EvidenceOrigin = "synthetic_fixture"
	OriginSourcePolicy     EvidenceOrigin = "source_policy"
)

type AccessClass string

const (
	AccessPublic        AccessClass = "public"
	AccessPrivate       AccessClass = "private"
	AccessAuthenticated AccessClass = "authenticated"
	AccessUnsupported   AccessClass = "unsupported"
)

type Request struct {
	CanonicalItemID  string
	CanonicalURL     string
	Strategy         string
	Format           string
	MaximumBodyBytes int64
}

type Artifact struct {
	SchemaVersion   string          `json:"schema_version"`
	CanonicalItemID string          `json:"canonical_item_id"`
	CanonicalURL    string          `json:"canonical_url"`
	Strategy        string          `json:"strategy"`
	Format          string          `json:"format"`
	State           State           `json:"state"`
	Origin          EvidenceOrigin  `json:"origin"`
	Access          AccessClass     `json:"access"`
	RetrievedAt     string          `json:"retrieved_at,omitempty"`
	Metadata        PublicMetadata  `json:"public_metadata"`
	Excerpts        []PublicExcerpt `json:"public_excerpts"`
	RelatedURLs     []RelatedURL    `json:"related_urls"`
	Missingness     []string        `json:"missingness"`
	SecretLike      bool            `json:"secret_like"`
}

type PublicMetadata struct {
	Title       string `json:"title,omitempty"`
	Author      string `json:"author,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

type PublicExcerpt struct {
	ExcerptID string `json:"excerpt_id"`
	Text      string `json:"text"`
	Locator   string `json:"locator"`
}

type RelatedURL struct {
	URL                  string `json:"url"`
	Relation             string `json:"relation"`
	DiscoveryEvidenceRef string `json:"discovery_evidence_ref"`
	SemanticallyRelevant bool   `json:"semantically_relevant"`
}

type Retriever interface {
	Retrieve(context.Context, Request) (Artifact, error)
}

func ValidateArtifact(artifact Artifact) error {
	if artifact.SchemaVersion != ArtifactSchema || strings.TrimSpace(artifact.CanonicalItemID) == "" || strings.TrimSpace(artifact.Strategy) == "" || strings.TrimSpace(artifact.Format) == "" {
		return errors.New("invalid retrieval artifact identity")
	}
	redactedManual := artifact.CanonicalURL == "" && artifact.SecretLike && artifact.State == StateNotAttempted && artifact.Access == AccessUnsupported && artifact.Origin == OriginSourcePolicy
	if artifact.Origin == OriginSourcePolicy && !redactedManual {
		return errors.New("source-policy retrieval must be content-free and sensitive-redacted")
	}
	if redactedManual && (artifact.Strategy != "manual_support" || artifact.Format != "sensitive_redacted" || artifact.RetrievedAt != "" || artifact.Metadata != (PublicMetadata{}) || len(artifact.Excerpts) != 0 || len(artifact.RelatedURLs) != 0 || len(artifact.Missingness) != 1 || artifact.Missingness[0] != SensitiveRedactedMissingnessReason) {
		return errors.New("sensitive-redacted retrieval must use the exact content-free envelope")
	}
	if !redactedManual {
		parsed, err := url.Parse(artifact.CanonicalURL)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("invalid retrieval canonical URL")
		}
	}
	switch artifact.State {
	case StateComplete, StatePartial, StateInaccessible, StateFailed, StateNotAttempted:
	default:
		return errors.New("invalid retrieval state")
	}
	switch artifact.Origin {
	case OriginImportedReplay, OriginLiveRetrieval, OriginSyntheticFixture, OriginSourcePolicy:
	default:
		return errors.New("invalid retrieval evidence origin")
	}
	switch artifact.Access {
	case AccessPublic, AccessPrivate, AccessAuthenticated, AccessUnsupported:
	default:
		return errors.New("invalid retrieval access class")
	}
	if artifact.RetrievedAt != "" {
		if _, err := time.Parse(time.RFC3339, artifact.RetrievedAt); err != nil {
			return errors.New("invalid retrieval timestamp")
		}
	}
	excerptIDs := map[string]bool{}
	total := 0
	for _, excerpt := range artifact.Excerpts {
		length := utf8.RuneCountInString(excerpt.Text)
		if strings.TrimSpace(excerpt.ExcerptID) == "" || excerptIDs[excerpt.ExcerptID] || strings.TrimSpace(excerpt.Locator) == "" || length == 0 || length > 1000 {
			return errors.New("invalid public excerpt")
		}
		total += length
		excerptIDs[excerpt.ExcerptID] = true
	}
	if total > 4000 {
		return errors.New("public excerpt budget exceeded")
	}
	if artifact.State == StateInaccessible && (len(artifact.Excerpts) != 0 || len(artifact.Missingness) == 0 || artifact.Metadata != (PublicMetadata{}) || len(artifact.RelatedURLs) != 0) {
		return errors.New("inaccessible retrieval must be explicit and unevidenced")
	}
	if artifact.State == StateComplete && len(artifact.Excerpts) == 0 {
		return errors.New("complete retrieval requires public evidence")
	}
	for _, related := range artifact.RelatedURLs {
		safeURL, storageState, err := routing.PrepareURLForStorage(related.URL)
		if err != nil || storageState == routing.URLStorageSensitiveRedacted || safeURL != related.URL || related.Relation != "source_links_to" || !excerptIDs[related.DiscoveryEvidenceRef] {
			return errors.New("invalid related retrieval evidence")
		}
	}
	return nil
}

func MissingArtifact(request Request, state State, access AccessClass, origin EvidenceOrigin, reason string) Artifact {
	return Artifact{
		SchemaVersion: ArtifactSchema, CanonicalItemID: request.CanonicalItemID, CanonicalURL: request.CanonicalURL,
		Strategy: request.Strategy, Format: request.Format, State: state, Access: access, Origin: origin, Missingness: []string{fmt.Sprintf("%s", reason)},
	}
}
