package personalmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/routing"
)

func placeholderResource(canonicalURL string) ResourceContext {
	resource := ResourceContext{
		ResourceID:     stableResourceID(canonicalURL),
		CanonicalURL:   canonicalURL,
		State:          "not_attempted",
		AccessClass:    "unsupported",
		Missingness:    []string{"enrichment_not_attempted"},
		AuthorityClass: AuthorityClass,
	}
	resource.ContentHash = fingerprintResource(resource)
	return resource
}

func redactedResource(canonicalURL string) ResourceContext {
	resource := ResourceContext{
		ResourceID:     stableResourceID(canonicalURL),
		CanonicalURL:   canonicalURL,
		State:          "inaccessible",
		AccessClass:    "unsupported",
		Missingness:    []string{"secret_like_content_redacted"},
		AuthorityClass: AuthorityClass,
	}
	resource.ContentHash = fingerprintResource(resource)
	return resource
}

func resourceFromContentOnly(canonicalURL string, content ContentArtifactRef, contentMissingness []string, contentAccess string) ResourceContext {
	state := "complete"
	missingness := []string{"source_metadata_not_provided", "source_excerpts_not_provided"}
	missingness = append(missingness, contentMissingness...)
	if content.Completeness == "partial" {
		state = "partial"
		missingness = append(missingness, "full_content_partial")
	}
	resource := ResourceContext{
		ResourceID: stableResourceID(canonicalURL), CanonicalURL: canonicalURL,
		State: state, AccessClass: contentAccess, Content: &content,
		Missingness: uniqueSorted(missingness), AuthorityClass: AuthorityClass,
	}
	resource.ContentHash = fingerprintResource(resource)
	return resource
}

func resourceFromImportedEvidence(input acquisition.ImportedEvidence, content *ContentArtifactRef, contentMissingness []string, contentAccess string) (ResourceContext, error) {
	safeURL, storageState, err := routing.PrepareURLForStorage(input.CanonicalURL)
	if err != nil || storageState == routing.URLStorageSensitiveRedacted || safeURL == "" {
		return ResourceContext{}, errors.New("personal evidence enrichment URL is unsafe")
	}
	canonicalURL, err := routing.CanonicalizeURL(safeURL)
	if err != nil || canonicalURL != input.CanonicalURL {
		return ResourceContext{}, errors.New("personal evidence enrichment URL is not canonical")
	}
	access := strings.TrimSpace(input.AccessClass)
	if access == "" {
		access = "public"
	}
	if content != nil && access != contentAccess {
		return ResourceContext{}, errors.New("personal evidence content access classification does not match its resource")
	}
	secretInputs := []string{
		input.CanonicalItemID,
		input.Metadata.Title, input.Metadata.Author, input.Metadata.PublishedAt,
		strings.Join(input.Missingness, " "),
	}
	for _, excerpt := range input.Excerpts {
		secretInputs = append(secretInputs, excerpt.ExcerptID, excerpt.Text, excerpt.Locator)
	}
	for _, related := range input.RelatedURLs {
		secretInputs = append(secretInputs, related.URL, related.Relation, related.DiscoveryEvidenceRef)
	}
	if input.SecretLike || importedEvidenceContainsSecret(secretInputs...) {
		resource := ResourceContext{
			ResourceID:     stableResourceID(canonicalURL),
			CanonicalURL:   canonicalURL,
			State:          "inaccessible",
			AccessClass:    "unsupported",
			Missingness:    []string{"secret_like_content_redacted"},
			AuthorityClass: AuthorityClass,
		}
		resource.ContentHash = fingerprintResource(resource)
		return resource, nil
	}
	state := strings.TrimSpace(input.State)
	missingness := append([]string(nil), input.Missingness...)
	missingness = append(missingness, contentMissingness...)
	title, titleRedacted := sanitizeTextURLs(strings.TrimSpace(input.Metadata.Title))
	author, authorRedacted := sanitizeTextURLs(strings.TrimSpace(input.Metadata.Author))
	publishedAt, publishedRedacted := sanitizeTextURLs(strings.TrimSpace(input.Metadata.PublishedAt))
	if titleRedacted || authorRedacted || publishedRedacted {
		missingness = append(missingness, "metadata_sensitive_url_redacted")
	}
	if state == "complete" && content == nil {
		state = "partial"
		missingness = append(missingness, "full_content_not_provided")
	}
	if content != nil && content.Completeness == "partial" && state == "complete" {
		state = "partial"
		missingness = append(missingness, "full_content_partial")
	}
	resource := ResourceContext{
		ResourceID:   stableResourceID(canonicalURL),
		CanonicalURL: canonicalURL,
		State:        state,
		AccessClass:  access,
		RetrievedAt:  strings.TrimSpace(input.RetrievedAt),
		Metadata: ResourceMetadata{
			Title:       title,
			Author:      author,
			PublishedAt: publishedAt,
		},
		Missingness:    uniqueSorted(missingness),
		Content:        content,
		AuthorityClass: AuthorityClass,
	}
	if resource.RetrievedAt != "" {
		if _, err := time.Parse(time.RFC3339, resource.RetrievedAt); err != nil {
			return ResourceContext{}, errors.New("personal evidence enrichment timestamp is invalid")
		}
	}
	for _, excerpt := range input.Excerpts {
		text, textRedacted := sanitizeTextURLs(strings.TrimSpace(excerpt.Text))
		locator, locatorRedacted := sanitizeTextURLs(strings.TrimSpace(excerpt.Locator))
		if textRedacted || locatorRedacted {
			resource.Missingness = append(resource.Missingness, "excerpt_sensitive_url_redacted")
		}
		resource.Excerpts = append(resource.Excerpts, ResourceExcerpt{
			ExcerptID: strings.TrimSpace(excerpt.ExcerptID),
			Text:      text,
			Locator:   locator,
		})
	}
	resource.Missingness = uniqueSorted(resource.Missingness)
	for _, related := range input.RelatedURLs {
		safeRelated, state, err := routing.PrepareURLForStorage(related.URL)
		if err != nil || state == routing.URLStorageSensitiveRedacted || safeRelated == "" {
			return ResourceContext{}, errors.New("personal evidence related URL is unsafe")
		}
		canonicalRelated, err := routing.CanonicalizeURL(safeRelated)
		if err != nil {
			return ResourceContext{}, errors.New("personal evidence related URL is invalid")
		}
		resource.RelatedURLs = append(resource.RelatedURLs, RelatedResource{
			URL:                  canonicalRelated,
			Relation:             strings.TrimSpace(related.Relation),
			DiscoveryEvidenceRef: strings.TrimSpace(related.DiscoveryEvidenceRef),
			SemanticallyRelevant: related.SemanticallyRelevant,
		})
	}
	sort.Slice(resource.Excerpts, func(i, j int) bool {
		return resource.Excerpts[i].ExcerptID < resource.Excerpts[j].ExcerptID
	})
	sort.Slice(resource.RelatedURLs, func(i, j int) bool {
		if resource.RelatedURLs[i].URL == resource.RelatedURLs[j].URL {
			return resource.RelatedURLs[i].DiscoveryEvidenceRef < resource.RelatedURLs[j].DiscoveryEvidenceRef
		}
		return resource.RelatedURLs[i].URL < resource.RelatedURLs[j].URL
	})
	resource.ContentHash = fingerprintResource(resource)
	if err := validateResource(resource); err != nil {
		return ResourceContext{}, err
	}
	return resource, nil
}

func prepareExtractedContent(input ExtractedContent) (string, *ContentArtifactRef, []byte, []string, string, bool, error) {
	safeURL, state, err := routing.PrepareURLForStorage(input.CanonicalURL)
	if err != nil || state == routing.URLStorageSensitiveRedacted || safeURL == "" {
		return "", nil, nil, nil, "", false, errors.New("personal evidence content URL is unsafe")
	}
	canonicalURL, err := routing.CanonicalizeURL(safeURL)
	if err != nil || canonicalURL != input.CanonicalURL {
		return "", nil, nil, nil, "", false, errors.New("personal evidence content URL is not canonical")
	}
	text := strings.TrimSpace(input.Text)
	if text == "" || len([]byte(text)) > MaximumExtractedContentBytes {
		return "", nil, nil, nil, "", false, errors.New("personal evidence extracted content exceeds its bounded contract")
	}
	if importedEvidenceContainsSecret(text, input.MediaType, strings.Join(input.Missingness, " ")) {
		return canonicalURL, nil, nil, nil, "", true, nil
	}
	if len(input.Missingness) > 100 {
		return "", nil, nil, nil, "", false, errors.New("personal evidence content missingness exceeds its bounded contract")
	}
	contentMissingness := make([]string, 0, len(input.Missingness)+1)
	for _, value := range input.Missingness {
		value = strings.TrimSpace(value)
		if value == "" || len([]rune(value)) > 256 || containsUnsafeURL(value) {
			return "", nil, nil, nil, "", false, errors.New("personal evidence content missingness is invalid")
		}
		contentMissingness = append(contentMissingness, value)
	}
	var urlRedacted bool
	text, urlRedacted = sanitizeTextURLs(text)
	if urlRedacted {
		contentMissingness = append(contentMissingness, "content_sensitive_url_redacted")
	}
	switch input.Completeness {
	case "full", "partial":
	default:
		return "", nil, nil, nil, "", false, errors.New("personal evidence content completeness is invalid")
	}
	mediaType := strings.TrimSpace(input.MediaType)
	if mediaType == "" {
		return "", nil, nil, nil, "", false, errors.New("personal evidence content media type is missing")
	}
	access := strings.TrimSpace(input.AccessClass)
	if access == "" {
		access = "public"
	}
	switch access {
	case "public", "private", "authenticated":
	default:
		return "", nil, nil, nil, "", false, errors.New("personal evidence content access classification is invalid")
	}
	payload := []byte(text)
	digest := sha256.Sum256(payload)
	sha := hex.EncodeToString(digest[:])
	reference := &ContentArtifactRef{
		ArtifactID: "content-" + sha, SHA256: sha,
		ByteLength: len(payload), RuneCount: len([]rune(text)),
		MediaType: mediaType, Completeness: input.Completeness,
		StorageClass: "owner_only_content_addressed_file",
	}
	if err := validateContentReference(*reference); err != nil {
		return "", nil, nil, nil, "", false, err
	}
	return canonicalURL, reference, payload, uniqueSorted(contentMissingness), access, false, nil
}

func fingerprintResource(resource ResourceContext) string {
	copy := resource
	copy.ContentHash = ""
	return fingerprintValue(copy)
}
