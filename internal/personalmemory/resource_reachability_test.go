package personalmemory

import (
	"strings"
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
	if !GenericExtractorReferenceExcerpt(ResourceExcerpt{ExcerptID: " related-legacy "}) {
		t.Fatal("generic extractor provenance excerpt was not recognized")
	}
	if GenericExtractorReferenceExcerpt(ResourceExcerpt{ExcerptID: "curated-proof"}) {
		t.Fatal("curated excerpt was classified as generic extractor provenance")
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
	genericTargets := GenericExtractorReferenceTargetIDs(library)
	if len(genericTargets) != 1 ||
		genericTargets[0] != stableResourceID(urls["generic"]) {
		t.Fatalf("legacy generic target denominator = %v", genericTargets)
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

func TestGenericReferenceIsHydratableButCannotInfluenceSearch(t *testing.T) {
	repository := populatedRepository(t)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	record := library.Records[0]
	parent := ""
	for _, resource := range library.Resources {
		if resource.ResourceID == record.ResourceIDs[0] {
			parent = resource.CanonicalURL
			break
		}
	}
	if parent == "" {
		t.Fatal("record resource was not retained")
	}
	genericURL := "https://example.com/zxqgenericoutboundmarker"
	if _, err := repository.MergeEnrichment(EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalURL: parent, State: "partial", AccessClass: "public",
			Excerpts: []acquisition.ImportedExcerpt{
				{
					ExcerptID: "related-generic-proof", Text: genericURL,
					Locator: "discovered_outbound_link",
				},
				{
					ExcerptID: "curated-proof", Text: "zxqcuratedpreservedmarker",
					Locator: "curated_summary",
				},
			},
			RelatedURLs: []acquisition.ImportedRelated{{
				URL: genericURL, Relation: "source_links_to",
				DiscoveryEvidenceRef: "related-generic-proof", SemanticallyRelevant: false,
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	generic, err := NewLexicalRetriever(repository).Search(SearchRequest{
		Query: "zxqgenericoutboundmarker", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(generic.Citations) != 0 || len(generic.Resources) != 0 {
		t.Fatalf("generic outbound reference influenced retrieval: %+v", generic)
	}

	curated, err := NewLexicalRetriever(repository).Search(SearchRequest{
		Query: "zxqcuratedpreservedmarker", Limit: 10,
	})
	if err != nil || len(curated.Citations) == 0 {
		t.Fatalf("curated excerpt stopped being searchable: %+v err=%v", curated, err)
	}
	foundCuratedReference := false
	for _, citation := range curated.Citations {
		for _, reference := range citation.EvidenceRefs {
			if reference.ExcerptID == "curated-proof" {
				foundCuratedReference = true
			}
			if strings.HasPrefix(reference.ExcerptID, GenericExtractorEvidencePrefix) {
				t.Fatalf("search emitted generic extractor evidence: %+v", reference)
			}
		}
	}
	if !foundCuratedReference {
		t.Fatalf("curated search lacked its exact evidence reference: %+v", curated)
	}

	hydrated, err := NewLexicalRetriever(repository).Get(record.RecordID)
	if err != nil {
		t.Fatal(err)
	}
	foundRelation := false
	foundGenericExcerpt := false
	for _, resource := range hydrated.Resources {
		for _, related := range resource.RelatedURLs {
			if related.URL == genericURL &&
				related.DiscoveryEvidenceRef == "related-generic-proof" {
				foundRelation = true
			}
		}
		for _, excerpt := range resource.Excerpts {
			if excerpt.ExcerptID == "related-generic-proof" &&
				excerpt.Text == genericURL {
				foundGenericExcerpt = true
			}
		}
	}
	if !foundRelation || !foundGenericExcerpt {
		t.Fatalf("explicit hydration lost generic reference provenance: %+v", hydrated.Resources)
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
