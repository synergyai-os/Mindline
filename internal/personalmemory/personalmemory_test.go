package personalmemory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
)

func TestPersonalEvidenceLibraryPersistsEverySlackRecordAndReplaysIdempotently(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	now := func() time.Time { return time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC) }
	repository, err := NewFileRepository(root, now)
	if err != nil {
		t.Fatal(err)
	}
	batch := fixtureBatch()
	captureBatch := captureBatchForTest(t, batch)
	first, err := repository.Import(captureBatch)
	if err != nil {
		t.Fatal(err)
	}
	if first.InsertedRecords != 4 || first.TotalRecords != 4 || first.DeclaredRecords != 4 {
		t.Fatalf("first import lost the denominator: %+v", first)
	}
	before, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	replay, err := repository.Import(captureBatch)
	if err != nil {
		t.Fatal(err)
	}
	after, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	if replay.InsertedRecords != 0 || replay.UpdatedRecords != 0 || replay.UnchangedRecords != 4 || before != after {
		t.Fatalf("replay mutated the library: before=%+v replay=%+v after=%+v", before, replay, after)
	}
	restarted, err := NewFileRepository(root, now)
	if err != nil {
		t.Fatal(err)
	}
	library, err := restarted.Load()
	if err != nil || len(library.Records) != 4 {
		t.Fatalf("restart lost evidence: records=%d err=%v", len(library.Records), err)
	}
	if len(library.Resources) != 3 {
		t.Fatalf("restart lost resource placeholders: resources=%d", len(library.Resources))
	}
}

func TestImportWithinBudgetRejectsBeforeCanonicalPersistence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	repository, err := NewFileRepository(root, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	batch := captureBatchForTest(t, fixtureBatch())
	if _, err := repository.ImportWithinBudget(batch, 1); err == nil {
		t.Fatal("expected lock-held admission rejection")
	}
	status, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.RecordCount != 0 || status.Fingerprint != EmptyLibrary().Fingerprint {
		t.Fatalf("rejected admission mutated canonical state: %+v", status)
	}
	if _, err := repository.ImportWithinBudget(batch, MaximumCaptureLibraryBytes); err != nil {
		t.Fatalf("expected admitted import: %v", err)
	}
}

func TestStatusCacheInvalidatesAfterLibraryMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	repository, err := NewFileRepository(root, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	before, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	if before.RecordCount != 0 {
		t.Fatalf("unexpected initial status: %+v", before)
	}

	receipt, err := repository.Import(captureBatchForTest(t, fixtureBatch()))
	if err != nil {
		t.Fatal(err)
	}
	after, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	if after.RecordCount != receipt.TotalRecords || after.Fingerprint == before.Fingerprint || after.Revision <= before.Revision {
		t.Fatalf("status cache survived a committed library mutation: before=%+v after=%+v receipt=%+v", before, after, receipt)
	}
}

func TestEditedSlackCapturePreservesSearchableImmutableRevision(t *testing.T) {
	repository := populatedRepository(t)
	updated := fixtureBatch()
	updated.Messages[0].Text = "updated signal https://www.linkedin.com/posts/example_knowledgegraphs-semanticweb"
	batch := captureBatchForTest(t, updated)
	receipt, err := repository.Import(batch)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.UpdatedRecords != 1 {
		t.Fatalf("updated capture was not recognized: %+v", receipt)
	}
	status, err := repository.Status()
	if err != nil || status.RecordCount != 4 || status.HistoricalRevisionCount != 1 {
		t.Fatalf("immutable history accounting failed: %+v err=%v", status, err)
	}
	packet, err := NewLexicalRetriever(repository).Search(SearchRequest{Query: "knowledgegraphs semanticweb", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var superseded bool
	for _, citation := range packet.Citations {
		if citation.VersionState == "superseded" {
			superseded = true
		}
	}
	if !superseded {
		t.Fatalf("prior saved context is no longer searchable: %+v", packet.Citations)
	}
}

func TestSlackRecordPreservesURLToResourceIdentityForMultipleLinks(t *testing.T) {
	batch := fixtureBatch()
	batch.Messages = []acquisitionslack.NativeMessage{{
		NativeMessageID: "1785049859.590279",
		Timestamp:       "1785049859.590279",
		Text:            "https://z.example/resource https://a.example/resource",
	}}
	batch.DeclaredSourceRecords = 1
	captureBatch := captureBatchForTest(t, batch)
	records := captureBatch.Records
	if len(records) != 1 || len(records[0].URLs) != 2 || len(records[0].ResourceIDs) != 2 {
		t.Fatalf("multi-link record was not retained: %+v", records)
	}
	for index, canonicalURL := range records[0].URLs {
		if records[0].ResourceIDs[index] != stableResourceID(canonicalURL) {
			t.Fatalf("resource identity detached from URL at index %d", index)
		}
	}
}

func TestPersonalEvidenceSearchReturnsOneCitedRecordAndFullContext(t *testing.T) {
	repository := populatedRepository(t)
	retriever := NewLexicalRetriever(repository)
	packet, err := retriever.Search(SearchRequest{Query: "company brain", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Citations) != 1 || len(packet.Records) != 1 {
		t.Fatalf("expected one record-level citation, got %+v", packet)
	}
	citation := packet.Citations[0]
	if citation.AuthorityClass != AuthorityClass || !strings.Contains(citation.Snippet, "company-brain") || citation.SourceRef == "" {
		t.Fatalf("citation lost provenance or authority: %+v", citation)
	}
	record, err := retriever.Get(citation.RecordID)
	if err != nil || record.Record.RawText != packet.Records[0].RawText {
		t.Fatalf("full-context retrieval failed: record=%+v err=%v", record, err)
	}
}

func TestPersonalEvidenceEnrichmentBecomesDurableSearchableContextAndReplays(t *testing.T) {
	repository := populatedRepository(t)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	var canonicalURL string
	for _, resource := range library.Resources {
		if strings.Contains(resource.CanonicalURL, "company-brain") {
			canonicalURL = resource.CanonicalURL
			break
		}
	}
	if canonicalURL == "" {
		t.Fatal("company-brain resource placeholder is missing")
	}
	evidence := acquisition.ImportedEvidence{
		CanonicalItemID: "activation-item-not-authoritative",
		CanonicalURL:    canonicalURL,
		State:           "complete",
		RetrievedAt:     "2026-07-26T09:00:00Z",
		AccessClass:     "public",
		Metadata: acquisition.ImportedMetadata{
			Title:  "Building a company brain",
			Author: "Example author",
		},
		Excerpts: []acquisition.ImportedExcerpt{{
			ExcerptID: "excerpt-1",
			Text:      "A company brain combines durable source evidence with cited agent recall.",
			Locator:   "post",
		}},
	}
	enrichment := EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Resources:     []acquisition.ImportedEvidence{evidence},
		Contents: []ExtractedContent{{
			CanonicalURL: canonicalURL, MediaType: "text/plain", Completeness: "full",
			Text:        "A company brain combines durable source evidence with cited agent recall. This complete extracted body remains available after restart.",
			AccessClass: "public",
		}},
	}
	first, err := repository.MergeEnrichment(enrichment)
	if err != nil {
		t.Fatal(err)
	}
	if first.UpdatedResources != 1 || first.TotalResources != len(library.Resources) {
		t.Fatalf("enrichment did not replace the placeholder: %+v", first)
	}
	replay, err := repository.MergeEnrichment(enrichment)
	if err != nil {
		t.Fatal(err)
	}
	if replay.UnchangedResources != 1 || replay.InsertedResources != 0 || replay.UpdatedResources != 0 {
		t.Fatalf("enrichment replay was not idempotent: %+v", replay)
	}
	restarted, err := NewFileRepository(repository.root, nil)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := NewLexicalRetriever(restarted).Search(SearchRequest{Query: "durable cited agent recall", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Records) != 1 || len(packet.Resources) != 1 ||
		packet.Resources[0].Metadata.Title != "Building a company brain" ||
		packet.Resources[0].AuthorityClass != AuthorityClass ||
		len(packet.Citations[0].EvidenceRefs) == 0 {
		t.Fatalf("enriched context was not durably recalled: %+v", packet)
	}
	hydrated, err := NewLexicalRetriever(restarted).Get(packet.Citations[0].RecordID)
	if err != nil || len(hydrated.Contents) != 1 || !strings.Contains(hydrated.Contents[0].Text, "complete extracted body") {
		t.Fatalf("full extracted context was not hydrated after restart: %+v err=%v", hydrated, err)
	}
}

func TestEnrichmentPrunesCrashOrphanBeforeApplyingStorageBudget(t *testing.T) {
	repository := populatedRepository(t)
	if err := os.Mkdir(repository.contentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	orphanPath := filepath.Join(repository.contentDir, "content-"+strings.Repeat("a", 64)+".txt")
	if err := os.WriteFile(orphanPath, []byte("orphaned crash artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	target := library.Resources[0].CanonicalURL
	if _, err := repository.MergeEnrichment(EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Contents: []ExtractedContent{{
			CanonicalURL: target, MediaType: "text/plain", Completeness: "full",
			Text: "durable replacement content", AccessClass: "public",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatalf("unreferenced crash artifact survived enrichment: err=%v", err)
	}
}

func TestRelatedResourceCanBecomeFirstClassSearchableEvidence(t *testing.T) {
	repository := populatedRepository(t)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	parent := library.Resources[0].CanonicalURL
	child := "https://example.com/follow-up"
	first := EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalItemID: "parent", CanonicalURL: parent, State: "partial",
			RetrievedAt: "2026-07-26T09:00:00Z", AccessClass: "public",
			Excerpts: []acquisition.ImportedExcerpt{{
				ExcerptID: "parent-link", Text: "The source points to a deeper follow-up.", Locator: "page",
			}},
			RelatedURLs: []acquisition.ImportedRelated{{
				URL: child, Relation: "source_links_to", DiscoveryEvidenceRef: "parent-link", SemanticallyRelevant: true,
			}},
		}},
	}
	if _, err := repository.MergeEnrichment(first); err != nil {
		t.Fatal(err)
	}
	second := EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalItemID: "child", CanonicalURL: child, State: "complete",
			RetrievedAt: "2026-07-26T09:01:00Z", AccessClass: "public",
			Excerpts: []acquisition.ImportedExcerpt{{
				ExcerptID: "child-proof", Text: "Follow-up evidence explains competency-driven semantic retrieval.", Locator: "article",
			}},
		}},
		Contents: []ExtractedContent{{
			CanonicalURL: child, MediaType: "text/plain", Completeness: "full",
			Text: "Full follow-up context explains competency-driven semantic retrieval in detail.",
		}},
	}
	if _, err := repository.MergeEnrichment(second); err != nil {
		t.Fatal(err)
	}
	packet, err := NewLexicalRetriever(repository).Search(SearchRequest{Query: "competency-driven semantic retrieval", Limit: 3})
	if err != nil || len(packet.Citations) != 1 || len(packet.Citations[0].EvidenceRefs) == 0 {
		t.Fatalf("related evidence did not become searchable with exact citation: %+v err=%v", packet, err)
	}
}

func TestSecretLikeEnrichmentBecomesExplicitRedactionWithoutDigestOrBody(t *testing.T) {
	repository := populatedRepository(t)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	target := library.Resources[0].CanonicalURL
	secret := "password=synthetic-sensitive-value"
	batch := EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalItemID: "secret", CanonicalURL: target, State: "complete",
			RetrievedAt: "2026-07-26T09:00:00Z", AccessClass: "public",
			Excerpts: []acquisition.ImportedExcerpt{{
				ExcerptID: "secret-proof", Text: "unsafe " + secret, Locator: "page",
			}},
		}},
	}
	if _, err := repository.MergeEnrichment(batch); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(filepath.Join(repository.root, libraryFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persisted), secret) {
		t.Fatal("secret-like enrichment crossed durable storage")
	}
	loaded, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, resource := range loaded.Resources {
		if resource.CanonicalURL == target {
			found = resource.State == "inaccessible" &&
				len(resource.Missingness) == 1 &&
				resource.Missingness[0] == "secret_like_content_redacted" &&
				resource.Content == nil
		}
	}
	if !found {
		t.Fatalf("secret redaction was not explicit: %+v", loaded.Resources)
	}
}

func TestPersonalEvidenceRedactsSecretsAndSensitiveURLsBeforePersistence(t *testing.T) {
	repository := populatedRepository(t)
	data, err := os.ReadFile(filepath.Join(repository.root, libraryFileName))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(data))
	for _, forbidden := range []string{"xoxb-synthetic-secret", "token=synthetic-secret", "utm_source=slack"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("private library persisted forbidden material %q", forbidden)
		}
	}
	if !strings.Contains(lower, "secret_like_content_redacted") || !strings.Contains(lower, "non-semantic") && !strings.Contains(lower, "linkedin.com") {
		t.Fatal("redaction or safe source evidence is missing")
	}
	info, err := os.Stat(repository.root)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("root is not owner-only: mode=%v err=%v", info.Mode().Perm(), err)
	}
	fileInfo, err := os.Stat(filepath.Join(repository.root, libraryFileName))
	if err != nil || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("library is not owner-only: mode=%v err=%v", fileInfo.Mode().Perm(), err)
	}
}

func TestRepositoryRefusesToAdoptExistingNonPrivateDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shared-directory")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileRepository(root, nil); err == nil {
		t.Fatal("repository silently adopted and changed an existing non-private directory")
	}
	info, err := os.Stat(root)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("repository changed caller-owned directory permissions: mode=%v err=%v", info.Mode().Perm(), err)
	}
}

func TestExactOlderBatchReplayCannotRollBackCurrentCapture(t *testing.T) {
	repository := populatedRepository(t)
	original := captureBatchForTest(t, fixtureBatch())
	updatedNative := fixtureBatch()
	updatedNative.Messages[0].Text = "updated durable meaning https://example.com/current"
	updated := captureBatchForTest(t, updatedNative)
	if _, err := repository.Import(updated); err != nil {
		t.Fatal(err)
	}
	replay, err := repository.Import(original)
	if err != nil || replay.UnchangedRecords != len(original.Records) || replay.UpdatedRecords != 0 {
		t.Fatalf("older exact replay was not recognized: %+v err=%v", replay, err)
	}
	library, err := repository.Load()
	if err != nil || !strings.Contains(library.Records[0].RawText, "updated durable meaning") {
		t.Fatalf("older replay rolled the current capture backward: %+v err=%v", library.Records[0], err)
	}
}

func TestNewCoverageWindowPersistsEvenWhenAllRecordsAreUnchanged(t *testing.T) {
	repository := populatedRepository(t)
	base := captureBatchForTest(t, fixtureBatch())
	expanded, err := NewCaptureBatch(CaptureBatchInput{
		SourceIdentity: base.SourceIdentity, LowerInclusive: base.LowerInclusive,
		UpperInclusive: "1785049999.999999", Watermark: "1785049999.999999",
		DeclaredRecords: base.DeclaredRecords, Records: base.Records,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := repository.Import(expanded)
	if err != nil || receipt.UnchangedRecords != len(base.Records) {
		t.Fatalf("unchanged expanded coverage failed: %+v err=%v", receipt, err)
	}
	library, err := repository.Load()
	if err != nil || len(library.Imports) != 2 || library.Imports[1].Watermark != expanded.Watermark {
		t.Fatalf("new coverage evidence was not persisted: %+v err=%v", library.Imports, err)
	}
}

func TestUnreachableIncomingRelatedCycleCannotAuthorizeItself(t *testing.T) {
	repository := populatedRepository(t)
	firstURL := "https://unrelated.example/one"
	secondURL := "https://unrelated.example/two"
	batch := EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{
			{
				CanonicalItemID: "unrelated-one", CanonicalURL: firstURL, State: "partial", AccessClass: "public",
				Excerpts:    []acquisition.ImportedExcerpt{{ExcerptID: "link-one", Text: "points elsewhere", Locator: "page"}},
				RelatedURLs: []acquisition.ImportedRelated{{URL: secondURL, Relation: "source_links_to", DiscoveryEvidenceRef: "link-one", SemanticallyRelevant: true}},
			},
			{
				CanonicalItemID: "unrelated-two", CanonicalURL: secondURL, State: "partial", AccessClass: "public",
				Excerpts:    []acquisition.ImportedExcerpt{{ExcerptID: "link-two", Text: "points back", Locator: "page"}},
				RelatedURLs: []acquisition.ImportedRelated{{URL: firstURL, Relation: "source_links_to", DiscoveryEvidenceRef: "link-two", SemanticallyRelevant: true}},
			},
		},
	}
	if _, err := repository.MergeEnrichment(batch); err == nil {
		t.Fatal("unreachable incoming related-resource cycle authorized itself")
	}
}

func TestResourceReEnrichmentPreservesSearchableHydratedHistory(t *testing.T) {
	repository := populatedRepository(t)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	target := library.Resources[0].CanonicalURL
	enrich := func(marker string) {
		t.Helper()
		_, err := repository.MergeEnrichment(EnrichmentBatch{
			SchemaVersion: EnrichmentBatchSchemaVersion,
			Resources: []acquisition.ImportedEvidence{{
				CanonicalItemID: "resource-history", CanonicalURL: target,
				State: "complete", AccessClass: "public",
				Excerpts: []acquisition.ImportedExcerpt{{ExcerptID: "history", Text: marker + " evidence", Locator: "page"}},
			}},
			Contents: []ExtractedContent{{
				CanonicalURL: target, MediaType: "text/plain", Completeness: "full",
				Text: marker + " complete extracted body", AccessClass: "public",
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	enrich("legacy-zephyr")
	enrich("current-nova")
	packet, err := NewLexicalRetriever(repository).Search(SearchRequest{Query: "legacy zephyr", Limit: 3})
	if err != nil || len(packet.Citations) != 1 || len(packet.ResourceRevisions) == 0 {
		t.Fatalf("historical resource was not searchable: %+v err=%v", packet, err)
	}
	var historicalReference bool
	for _, reference := range packet.Citations[0].EvidenceRefs {
		if reference.ResourceVersionState == "superseded" && reference.ResourceRevisionID != "" {
			historicalReference = true
		}
	}
	if !historicalReference {
		t.Fatalf("historical search lacked exact versioned evidence: %+v", packet.Citations[0].EvidenceRefs)
	}
	hydrated, err := NewLexicalRetriever(repository).Get(packet.Citations[0].RecordID)
	if err != nil || len(hydrated.ResourceRevisions) == 0 || len(hydrated.Contents) < 2 {
		t.Fatalf("historical resource body was not hydratable: %+v err=%v", hydrated, err)
	}
}

func TestEverySearchableResourceFieldCanProduceAnExactEvidenceReference(t *testing.T) {
	repository := populatedRepository(t)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	target := library.Resources[0].CanonicalURL
	if _, err := repository.MergeEnrichment(EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalItemID: "field-citations", CanonicalURL: target,
			State: "partial", AccessClass: "public",
			Metadata:    acquisition.ImportedMetadata{PublishedAt: "2026-07-26T00:00:00Z"},
			Missingness: []string{"statistics_not_independently_verified"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		query   string
		locator string
	}{
		{query: "2026", locator: "public_metadata"},
		{query: "statistics independently verified", locator: "resource_missingness"},
	} {
		packet, err := NewLexicalRetriever(repository).Search(SearchRequest{Query: test.query, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, citation := range packet.Citations {
			for _, reference := range citation.EvidenceRefs {
				if reference.ResourceID == stableResourceID(target) && reference.Locator == test.locator {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("searchable field %q had no exact %s evidence reference: %+v", test.query, test.locator, packet)
		}
	}
}

func TestContentAccessClassificationMustMatchResource(t *testing.T) {
	repository := populatedRepository(t)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	target := library.Resources[0].CanonicalURL
	_, err = repository.MergeEnrichment(EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalItemID: "access-mismatch", CanonicalURL: target,
			State: "complete", AccessClass: "public",
		}},
		Contents: []ExtractedContent{{
			CanonicalURL: target, MediaType: "text/plain", Completeness: "full",
			Text: "private body", AccessClass: "private",
		}},
	})
	if err == nil {
		t.Fatal("mismatched full-content access classification was accepted")
	}
}

func TestExtractedContentWithUnsafeURLPersistsOnlyContentFreeRedaction(t *testing.T) {
	repository := populatedRepository(t)
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	target := library.Resources[0].CanonicalURL
	unsafeValue := "opaque-private-share-value"
	if _, err := repository.MergeEnrichment(EnrichmentBatch{
		SchemaVersion: EnrichmentBatchSchemaVersion,
		Contents: []ExtractedContent{{
			CanonicalURL: target, MediaType: "text/plain", Completeness: "full",
			Text: "follow https://example.com/private?share=" + unsafeValue, AccessClass: "public",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	var matched ResourceContext
	for _, resource := range loaded.Resources {
		if resource.CanonicalURL == target {
			matched = resource
		}
	}
	if matched.Content == nil || !testContainsString(matched.Missingness, "content_sensitive_url_redacted") {
		t.Fatalf("unsafe content URL missingness was not explicit: %+v", matched)
	}
	content, err := repository.LoadContent(*matched.Content)
	if err != nil || strings.Contains(content.Text, unsafeValue) ||
		!strings.Contains(content.Text, "[mindline-sensitive-url-redacted]") {
		t.Fatalf("unsafe URL value crossed content storage: %+v err=%v", content, err)
	}
}

func TestCaptureFingerprintIsIndependentFromEveryLexicalURLValue(t *testing.T) {
	build := func(url string) CaptureRecord {
		record, err := NewCaptureRecord(CaptureRecordInput{
			SourceAdapter: "synthetic", SourceScopeID: "scope", SourceContainerID: "container",
			ExternalID: "record", OccurredAt: "2026-07-26T08:00:00Z",
			SourceRef: "synthetic://scope/container/record", RawText: "same meaning " + url,
		})
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	first := build("https://one.example/source")
	second := build("https://two.example/other")
	if first.SourceContentFingerprint != second.SourceContentFingerprint {
		t.Fatal("lexical URL value influenced the persisted source fingerprint")
	}
	if first.ContentHash == second.ContentHash {
		t.Fatal("canonical evidence identity failed to account for different retained resources")
	}
}

func TestCaptureConstructorRedactsUnsafeAuthorAndBatchRejectsSecretProvenance(t *testing.T) {
	record, err := NewCaptureRecord(CaptureRecordInput{
		SourceAdapter: "synthetic", SourceScopeID: "scope", SourceContainerID: "container",
		ExternalID: "record", OccurredAt: "2026-07-26T08:00:00Z",
		SourceRef: "synthetic://scope/container/record", RawText: "safe capture",
		AuthorName: "password=synthetic-private-value",
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.AuthorName != "" || !testContainsString(record.Missingness, "author_metadata_redacted") {
		t.Fatalf("unsafe author metadata was retained: %+v", record)
	}
	if _, err := NewCaptureBatch(CaptureBatchInput{
		SourceIdentity: "pb_sk_synthetic-private-value",
		LowerInclusive: "1", UpperInclusive: "1", Watermark: "1",
		DeclaredRecords: 1, Records: []CaptureRecord{record},
	}); err == nil {
		t.Fatal("secret-like batch provenance crossed the repository trust boundary")
	}
}

func TestContextPacketReportsTheActualReplaceableBackendMethod(t *testing.T) {
	repository := populatedRepository(t)
	retriever := NewRetriever(repository, fixedTestBackend{method: "candidate-localdb-derived-index/v0.1"})
	packet, err := retriever.Search(SearchRequest{Query: "company", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if packet.RetrievalMethod != "candidate-localdb-derived-index/v0.1" {
		t.Fatalf("context packet falsely reported another backend: %+v", packet)
	}
}

func TestRetrievalHydrationIsTargetedForGetAndAggregateBoundedForSearch(t *testing.T) {
	library := EmptyLibrary()
	for index := 0; index < 17; index++ {
		resourceID := "resource-budget-" + string(rune('a'+index))
		recordID := "capture-budget-" + string(rune('a'+index))
		reference := ContentArtifactRef{
			ArtifactID: "content-" + strings.Repeat(string(rune('a'+index)), 64),
			SHA256:     strings.Repeat(string(rune('a'+index)), 64),
			ByteLength: 4 << 20, RuneCount: 1, MediaType: "text/plain",
			Completeness: "full", StorageClass: "owner_only_content_addressed_file",
		}
		library.Records = append(library.Records, CaptureRecord{
			RecordID: recordID, RawText: "budget record", ResourceIDs: []string{resourceID},
		})
		library.Resources = append(library.Resources, ResourceContext{
			ResourceID: resourceID, CanonicalURL: "https://example.com/" + recordID,
			Content: &reference,
		})
	}
	repository := &hydrationBudgetRepository{library: library}
	retriever := NewLexicalRetriever(repository)
	if _, err := retriever.Get(library.Records[0].RecordID); err != nil || repository.loads != 1 {
		t.Fatalf("targeted get hydrated unrelated evidence: loads=%d err=%v", repository.loads, err)
	}
	repository.loads = 0
	if _, err := retriever.Search(SearchRequest{Query: "budget", Limit: 3}); err == nil ||
		!strings.Contains(err.Error(), "hydration budget") ||
		repository.loads != MaximumRetrievalContentBytes/(4<<20) {
		t.Fatalf("aggregate hydration budget was not enforced: loads=%d err=%v", repository.loads, err)
	}
}

func TestCaptureBatchFingerprintDoesNotDependOnWithheldSecretValue(t *testing.T) {
	first := fixtureBatch()
	second := fixtureBatch()
	first.Messages[3].Text = "credential password=synthetic-first-value"
	second.Messages[3].Text = "credential password=synthetic-second-value"
	firstBatch := captureBatchForTest(t, first)
	secondBatch := captureBatchForTest(t, second)
	if captureBatchFingerprint(firstBatch) != captureBatchFingerprint(secondBatch) {
		t.Fatal("persisted batch identity depends on withheld secret material")
	}
}

func TestLensesAreDerivedAndCannotChangeRetention(t *testing.T) {
	repository := populatedRepository(t)
	before, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	retriever := NewLexicalRetriever(repository)
	packet, err := retriever.ReviewLenses(LensBatch{
		SchemaVersion: LensBatchSchemaVersion,
		Lenses: []Lens{
			{ID: "product-brain", Name: "Building Product Brain", Query: "company brain"},
			{ID: "org-design", Name: "AI organization design", Query: "AI futures"},
			{ID: "content", Name: "Content inspiration", Query: "context engineering"},
		},
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	after, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	if before != after || len(packet.Projections) != 3 || !packet.RetentionUnchanged {
		t.Fatalf("lenses changed retention: before=%+v after=%+v packet=%+v", before, after, packet)
	}
	for _, projection := range packet.Projections {
		if projection.RetainedCount != 4 || projection.LibraryFingerprint != before.Fingerprint {
			t.Fatalf("lens is not bound to the complete retained denominator: %+v", projection)
		}
	}
}

func populatedRepository(t *testing.T) *FileRepository {
	t.Helper()
	root := filepath.Join(t.TempDir(), "library")
	repository, err := NewFileRepository(root, func() time.Time {
		return time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	batch := captureBatchForTest(t, fixtureBatch())
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	return repository
}

func fixtureBatch() acquisitionslack.NativeBatch {
	return acquisitionslack.NativeBatch{
		SchemaVersion: acquisitionslack.NativeBatchSchema,
		WorkspaceID:   "T-fixture", ChannelID: "D-fixture",
		LowerInclusive: "1784756437.515429",
		UpperInclusive: "1785049859.590279",
		Watermark:      "1785049859.590279",
		IncludeThreads: true, IncludeReplies: true,
		PaginationExhausted: true, ThreadPaginationExhausted: true,
		DeclaredSourceRecords: 4,
		Messages: []acquisitionslack.NativeMessage{
			{
				NativeMessageID: "1784756437.515429", Timestamp: "1784756437.515429",
				AuthorID: "U1", AuthorName: "Randy",
				Text: "https://www.linkedin.com/posts/example_knowledgegraphs-semanticweb?utm_source=slack",
			},
			{
				NativeMessageID: "1784902473.012269", Timestamp: "1784902473.012269",
				AuthorID: "U1", AuthorName: "Randy",
				Text: "https://www.linkedin.com/posts/example_company-brain-share",
			},
			{
				NativeMessageID: "1784988423.680879", Timestamp: "1784988423.680879",
				AuthorID: "U1", AuthorName: "Randy",
				Text: "https://github.com/example/advanced-context-engineering-for-coding-agents",
			},
			{
				NativeMessageID: "1785049859.590279", Timestamp: "1785049859.590279",
				AuthorID: "U1", AuthorName: "Randy",
				Text: "credential xoxb-synthetic-secret and https://example.com/?token=synthetic-secret",
			},
		},
	}
}

func captureBatchForTest(t *testing.T, batch acquisitionslack.NativeBatch) CaptureBatch {
	t.Helper()
	records := make([]CaptureRecord, 0, len(batch.Messages))
	for _, message := range batch.Messages {
		occurredAt, err := acquisition.NativeTimestampToRFC3339(message.Timestamp)
		if err != nil {
			t.Fatal(err)
		}
		sourceRef := "slack://" + batch.WorkspaceID + "/" + batch.ChannelID + "/" + message.NativeMessageID
		missingness := []string{"permalink_unavailable"}
		if message.Permalink != "" {
			sourceRef = message.Permalink
			missingness = nil
		}
		record, err := NewCaptureRecord(CaptureRecordInput{
			SourceAdapter: "slack", SourceScopeID: batch.WorkspaceID,
			SourceContainerID: batch.ChannelID, ExternalID: message.NativeMessageID,
			OccurredAt: occurredAt, AuthorID: message.AuthorID, AuthorName: message.AuthorName,
			SourceRef: sourceRef, RawText: message.Text, ThreadParentID: message.ThreadParentID,
			AttachmentCount: message.AttachmentCount, PrivateFileCount: message.PrivateFileCount,
			EditDeleteState: message.EditDeleteState, Missingness: missingness,
		})
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	capture, err := NewCaptureBatch(CaptureBatchInput{
		SourceIdentity: "slack:" + batch.WorkspaceID + ":" + batch.ChannelID,
		LowerInclusive: batch.LowerInclusive, UpperInclusive: batch.UpperInclusive,
		Watermark: batch.Watermark, DeclaredRecords: batch.DeclaredSourceRecords,
		Records: records,
	})
	if err != nil {
		t.Fatal(err)
	}
	return capture
}

func testContainsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

type fixedTestBackend struct {
	method string
}

type hydrationBudgetRepository struct {
	library Library
	loads   int
}

func (repository *hydrationBudgetRepository) Load() (Library, error) {
	return repository.library, nil
}

func (*hydrationBudgetRepository) Import(CaptureBatch) (ImportReceipt, error) {
	return ImportReceipt{}, nil
}

func (*hydrationBudgetRepository) MergeEnrichment(EnrichmentBatch) (EnrichmentReceipt, error) {
	return EnrichmentReceipt{}, nil
}

func (repository *hydrationBudgetRepository) LoadContent(reference ContentArtifactRef) (ExtractedContentArtifact, error) {
	repository.loads++
	return ExtractedContentArtifact{Reference: reference, Text: "budget"}, nil
}

func (backend fixedTestBackend) MethodID() string {
	return backend.method
}

func (fixedTestBackend) Rank(_ SearchRequest, documents []IndexDocument) ([]RankedHit, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	return []RankedHit{{
		DocumentID: documents[0].DocumentID, Score: 1, MatchedTerms: []string{"company"},
	}}, nil
}
