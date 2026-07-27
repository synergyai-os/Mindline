package personalmemory

import (
	"testing"

	"github.com/synergyai-os/Mindline/internal/acquisition"
)

func TestFollowableRelationsRejectLegacyGenericExtractorEvidence(t *testing.T) {
	if FollowableRelatedResource(RelatedResource{SemanticallyRelevant: false, DiscoveryEvidenceRef: "curated-proof"}) {
		t.Fatal("unapproved relation became followable")
	}
	if FollowableRelatedResource(RelatedResource{SemanticallyRelevant: true, DiscoveryEvidenceRef: "related-legacy"}) {
		t.Fatal("legacy generic extractor relation became followable")
	}
	if !FollowableRelatedResource(RelatedResource{SemanticallyRelevant: true, DiscoveryEvidenceRef: "curated-proof"}) {
		t.Fatal("explicit curated relation was not followable")
	}
}

func TestProcessableResourcesStartAtRetainedRecordsAndFollowOnlyCuratedRelations(t *testing.T) {
	urls := map[string]string{
		"parent":     "https://example.com/parent",
		"curated":    "https://example.com/curated",
		"generic":    "https://example.com/generic",
		"unapproved": "https://example.com/unapproved",
		"historical": "https://example.com/historical",
		"orphan":     "https://example.com/orphan",
	}
	resource := func(name string, related ...RelatedResource) ResourceContext {
		return ResourceContext{ResourceID: stableResourceID(urls[name]), CanonicalURL: urls[name], RelatedURLs: related}
	}
	parent := resource("parent",
		RelatedResource{URL: urls["curated"], DiscoveryEvidenceRef: "curated-proof", SemanticallyRelevant: true},
		RelatedResource{URL: urls["generic"], DiscoveryEvidenceRef: "related-legacy", SemanticallyRelevant: true},
		RelatedResource{URL: urls["unapproved"], DiscoveryEvidenceRef: "unapproved-proof", SemanticallyRelevant: false},
	)
	library := Library{
		Records: []CaptureRecord{{ResourceIDs: []string{parent.ResourceID}}},
		Resources: []ResourceContext{
			parent, resource("curated"), resource("generic"), resource("unapproved"),
			resource("historical"), resource("orphan"),
		},
		ResourceRevisions: []ResourceRevision{{Resource: ResourceContext{
			ResourceID: parent.ResourceID,
			RelatedURLs: []RelatedResource{{
				URL: urls["historical"], DiscoveryEvidenceRef: "historical-curated-proof", SemanticallyRelevant: true,
			}},
		}}},
	}
	got := map[string]bool{}
	for _, resourceID := range ProcessableResourceIDs(library) {
		got[resourceID] = true
	}
	for _, name := range []string{"parent", "curated", "historical"} {
		if !got[stableResourceID(urls[name])] {
			t.Fatalf("processable graph lost %s", name)
		}
	}
	for _, name := range []string{"generic", "unapproved", "orphan"} {
		if got[stableResourceID(urls[name])] {
			t.Fatalf("unprocessable graph admitted %s", name)
		}
	}
}

func TestGenericReferenceIsRetainedWithoutCreatingPlaceholder(t *testing.T) {
	repository := populatedRepository(t)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	parent := library.Resources[0].CanonicalURL
	child := "https://example.com/generic-reference"
	if _, err := repository.MergeEnrichment(EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalURL: parent, State: "partial", AccessClass: "public",
			Excerpts: []acquisition.ImportedExcerpt{{
				ExcerptID: "related-synthetic", Text: child, Locator: "discovered_outbound_link",
			}},
			RelatedURLs: []acquisition.ImportedRelated{{
				URL: child, Relation: "source_links_to",
				DiscoveryEvidenceRef: "related-synthetic", SemanticallyRelevant: true,
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	library, err = repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range library.Resources {
		if resource.CanonicalURL == child {
			t.Fatal("generic reference created a first-class placeholder")
		}
	}
	found := false
	for _, resource := range library.Resources {
		if resource.CanonicalURL == parent && len(resource.RelatedURLs) == 1 {
			found = resource.RelatedURLs[0].URL == child
		}
	}
	if !found {
		t.Fatal("generic reference provenance was not retained")
	}
}

func TestExistingOrphanPlaceholderCanBecomeReferenceOnlyTerminal(t *testing.T) {
	repository := populatedRepository(t)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	orphanURL := "https://example.com/legacy-generic-placeholder"
	orphan := placeholderResource(orphanURL)
	library.Resources = append(library.Resources, orphan)
	library = sealLibrary(library)
	if err := repository.persistLibrary(library); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.MergeEnrichment(EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalURL: orphanURL, State: "inaccessible", AccessClass: "unsupported",
			Missingness: []string{"resource_blocked:manual_processing_required"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	library, err = repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range library.Resources {
		if resource.CanonicalURL != orphanURL {
			continue
		}
		if resource.State != "inaccessible" ||
			len(resource.Missingness) != 1 ||
			resource.Missingness[0] != "resource_blocked:manual_processing_required" {
			t.Fatalf("orphan placeholder terminal = %+v", resource)
		}
		if len(library.ResourceRevisions) == 0 {
			t.Fatal("reference-only terminalization did not retain prior evidence history")
		}
		return
	}
	t.Fatal("reference-only orphan disappeared")
}
