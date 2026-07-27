package personalmemory

import (
	"sort"
	"strings"
)

// GenericExtractorEvidencePrefix identifies references emitted by the generic
// cross-host extractor. It carries bounded provenance, never semantic approval.
const GenericExtractorEvidencePrefix = "related-"

// FollowableRelatedResource is the canonical boundary between bounded
// reference evidence and an explicitly curated semantic relation. Historical
// resourcefetch links used related-* evidence IDs and must remain unclassified
// references even if an older adapter incorrectly set the relevance flag.
func FollowableRelatedResource(related RelatedResource) bool {
	return related.SemanticallyRelevant &&
		!strings.HasPrefix(strings.TrimSpace(related.DiscoveryEvidenceRef), GenericExtractorEvidencePrefix)
}

// ProcessableResourceIDs returns the complete resource graph reachable from
// retained current or historical captures through followable relations only.
// Canonical orphan evidence remains retained but cannot silently enter search
// or network processing.
func ProcessableResourceIDs(library Library) []string {
	resourcesByID := make(map[string]ResourceContext, len(library.Resources))
	for _, resource := range library.Resources {
		resourcesByID[resource.ResourceID] = resource
	}
	revisionsByID := make(map[string][]ResourceRevision, len(library.ResourceRevisions))
	for _, revision := range library.ResourceRevisions {
		revisionsByID[revision.Resource.ResourceID] = append(revisionsByID[revision.Resource.ResourceID], revision)
	}
	queue := []string{}
	for _, record := range library.Records {
		queue = append(queue, record.ResourceIDs...)
	}
	for _, revision := range library.Revisions {
		queue = append(queue, revision.Record.ResourceIDs...)
	}
	seen := map[string]bool{}
	for len(queue) > 0 {
		resourceID := queue[0]
		queue = queue[1:]
		if seen[resourceID] {
			continue
		}
		resource, exists := resourcesByID[resourceID]
		if !exists {
			continue
		}
		seen[resourceID] = true
		for _, related := range resource.RelatedURLs {
			if FollowableRelatedResource(related) {
				queue = append(queue, stableResourceID(related.URL))
			}
		}
		for _, revision := range revisionsByID[resourceID] {
			for _, related := range revision.Resource.RelatedURLs {
				if FollowableRelatedResource(related) {
					queue = append(queue, stableResourceID(related.URL))
				}
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for resourceID := range seen {
		ids = append(ids, resourceID)
	}
	sort.Strings(ids)
	return ids
}
