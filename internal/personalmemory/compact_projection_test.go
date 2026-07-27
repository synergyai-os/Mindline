package personalmemory

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type compactProjectionBackend struct {
	documents     []IndexDocument
	hits          []RankedHit
	calibrationID string
}

func (*compactProjectionBackend) MethodID() string {
	return "compact-projection-fixture/v0.10"
}

func (backend *compactProjectionBackend) Rank(
	_ SearchRequest,
	documents []IndexDocument,
) ([]RankedHit, error) {
	backend.documents = append([]IndexDocument(nil), documents...)
	return append([]RankedHit(nil), backend.hits...), nil
}

func (backend *compactProjectionBackend) CompactSemanticCalibrationID() string {
	return backend.calibrationID
}

func authorizedProjectionHit(documentID string, terms ...string) RankedHit {
	return RankedHit{
		DocumentID: documentID,
		Score:      1,
		MatchedTerms: append(
			[]string(nil), terms...,
		),
		Components: lexicalAuthorizationComponents(
			len(terms), len(terms), 1, 1, 0.001, 1,
		),
	}
}

func TestCompactProjectionIndexesLongLinkedPageIndependently(t *testing.T) {
	resourceID := "resource-long-page"
	documentID, err := compactResourceDocumentID(resourceID)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("background material ", 1200) +
		"portable memory retrieves precise lessons"
	reference := ContentArtifactRef{
		ArtifactID: "artifact-long-page",
		ByteLength: len([]byte(body)),
		RuneCount:  len([]rune(body)),
	}
	repository := &compactRepository{
		contents: map[string]string{reference.ArtifactID: body},
		library: Library{
			SchemaVersion: LibrarySchemaVersion,
			Revision:      11,
			Fingerprint:   strings.Repeat("1", 64),
			Records: []CaptureRecord{{
				RecordID:    "record-long-page",
				SourceRef:   "slack://fixture/long-page",
				RawText:     strings.Repeat("unrelated saved message ", 800),
				ResourceIDs: []string{resourceID},
				ContentHash: strings.Repeat("2", 64),
			}},
			Resources: []ResourceContext{{
				ResourceID:     resourceID,
				Metadata:       ResourceMetadata{Title: "A long linked page"},
				Content:        &reference,
				ContentHash:    strings.Repeat("3", 64),
				AuthorityClass: AuthorityClass,
			}},
		},
	}
	backend := &compactProjectionBackend{hits: []RankedHit{
		authorizedProjectionHit(documentID, "portable", "memory", "retrieves"),
	}}
	packet, err := NewRetriever(repository, backend).SearchCompact(SearchRequest{
		Query: "portable memory retrieves",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(backend.documents) != 2 {
		t.Fatalf("compact projection documents=%+v", backend.documents)
	}
	documents := map[string]string{}
	for _, document := range backend.documents {
		documents[document.DocumentID] = document.Text
	}
	if strings.Contains(documents["record-long-page"], "precise lessons") ||
		!strings.Contains(documents[documentID], "precise lessons") {
		t.Fatalf("linked content was not independently projected: %+v", backend.documents)
	}
	if packet.AnswerState != "answered" || len(packet.Citations) != 1 ||
		packet.Citations[0].RecordID != "record-long-page" {
		t.Fatalf("resource hit did not map to its canonical record: %+v", packet)
	}
}

func TestCompactProjectionDeduplicatesSharedResourceAndExpandsDeterministicOwners(t *testing.T) {
	resourceID := "resource-shared"
	documentID, err := compactResourceDocumentID(resourceID)
	if err != nil {
		t.Fatal(err)
	}
	records := []CaptureRecord{
		{
			RecordID: "record-b", SourceRef: "slack://fixture/b",
			RawText: "second save", ResourceIDs: []string{resourceID},
			ContentHash: strings.Repeat("b", 64),
		},
		{
			RecordID: "record-a", SourceRef: "slack://fixture/a",
			RawText: "first save", ResourceIDs: []string{resourceID},
			ContentHash: strings.Repeat("a", 64),
		},
	}
	run := func(records []CaptureRecord) (CompactContextPacket, []IndexDocument) {
		t.Helper()
		repository := &compactRepository{library: Library{
			SchemaVersion: LibrarySchemaVersion,
			Revision:      4,
			Fingerprint:   strings.Repeat("4", 64),
			Records:       records,
			Resources: []ResourceContext{{
				ResourceID: resourceID,
				Metadata:   ResourceMetadata{Title: "Shared portable memory lesson"},
				ContentHash: strings.Repeat(
					"c", 64,
				),
				AuthorityClass: AuthorityClass,
			}},
		}}
		hit := authorizedProjectionHit(documentID, "shared", "portable", "memory")
		hit.Components["projection_marker"] = 7
		backend := &compactProjectionBackend{hits: []RankedHit{hit}}
		packet, err := NewRetriever(repository, backend).SearchCompact(SearchRequest{
			Query: "shared portable memory",
			Limit: 2,
		})
		if err != nil {
			t.Fatal(err)
		}
		return packet, backend.documents
	}
	first, firstDocuments := run(records)
	second, secondDocuments := run([]CaptureRecord{records[1], records[0]})
	if !reflect.DeepEqual(firstDocuments, secondDocuments) {
		t.Fatalf("compact projection order changed: first=%+v second=%+v",
			firstDocuments, secondDocuments)
	}
	resourceDocuments := 0
	for _, document := range firstDocuments {
		if document.DocumentID == documentID {
			resourceDocuments++
			if !reflect.DeepEqual(
				document.FeedbackAliases,
				[]string{"record-a", "record-b"},
			) {
				t.Fatalf("shared resource feedback aliases=%v",
					document.FeedbackAliases)
			}
		}
	}
	if resourceDocuments != 1 {
		t.Fatalf("shared resource projected %d times: %+v",
			resourceDocuments, firstDocuments)
	}
	if len(first.Citations) != 2 ||
		first.Citations[0].RecordID != "record-a" ||
		first.Citations[1].RecordID != "record-b" ||
		first.Citations[0].ComponentScores["projection_marker"] != 7 {
		t.Fatalf("resource owners were not expanded deterministically: %+v", first)
	}
	if !reflect.DeepEqual(first.Citations, second.Citations) {
		t.Fatalf("owner expansion was not replay deterministic: first=%+v second=%+v",
			first.Citations, second.Citations)
	}
}

func TestCompactProjectionNeverIndexesGenericOrSupersededResourceEvidence(t *testing.T) {
	parentID := "resource-parent"
	genericURL := "https://example.invalid/generic-child"
	genericID := stableResourceID(genericURL)
	revisionURL := "https://example.invalid/revision-child"
	revisionID := stableResourceID(revisionURL)
	backend := &compactProjectionBackend{hits: []RankedHit{
		authorizedProjectionHit("record-parent", "current", "saved", "message"),
	}}
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion,
		Revision:      9,
		Fingerprint:   strings.Repeat("9", 64),
		Records: []CaptureRecord{{
			RecordID: "record-parent", RawText: "current saved message",
			SourceRef: "slack://fixture/parent", ResourceIDs: []string{parentID},
			ContentHash: strings.Repeat("8", 64),
		}},
		Resources: []ResourceContext{
			{
				ResourceID: parentID,
				Metadata:   ResourceMetadata{Title: "Current resource"},
				Excerpts: []ResourceExcerpt{
					{ExcerptID: "related-generic", Text: "GENERIC_EXCERPT", Locator: genericURL},
					{ExcerptID: "curated-current", Text: "curated current excerpt", Locator: "body"},
				},
				RelatedURLs: []RelatedResource{{
					URL: genericURL, SemanticallyRelevant: true,
					DiscoveryEvidenceRef: "related-generic",
				}},
				ContentHash: strings.Repeat("7", 64),
			},
			{ResourceID: genericID, Metadata: ResourceMetadata{Title: "GENERIC_CHILD"}},
			{ResourceID: revisionID, Metadata: ResourceMetadata{Title: "REVISION_CHILD"}},
		},
		ResourceRevisions: []ResourceRevision{{
			RevisionID: "resource-revision-parent",
			Resource: ResourceContext{
				ResourceID: parentID,
				Metadata:   ResourceMetadata{Title: "SUPERSEDED_METADATA"},
				RelatedURLs: []RelatedResource{{
					URL: revisionURL, SemanticallyRelevant: true,
					DiscoveryEvidenceRef: "curated-revision",
				}},
			},
		}},
	}}
	packet, err := NewRetriever(repository, backend).SearchCompact(SearchRequest{
		Query: "current saved message",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	indexed := map[string]string{}
	for _, document := range backend.documents {
		indexed[document.DocumentID] = document.Text
	}
	parentDocumentID, err := compactResourceDocumentID(parentID)
	if err != nil {
		t.Fatal(err)
	}
	genericDocumentID, err := compactResourceDocumentID(genericID)
	if err != nil {
		t.Fatal(err)
	}
	revisionDocumentID, err := compactResourceDocumentID(revisionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := indexed[genericDocumentID]; exists {
		t.Fatalf("generic outbound resource entered the compact index: %+v", backend.documents)
	}
	if _, exists := indexed[revisionDocumentID]; exists {
		t.Fatalf("superseded resource relation entered the compact index: %+v", backend.documents)
	}
	parentText := indexed[parentDocumentID]
	for _, forbidden := range []string{
		"GENERIC_EXCERPT", genericURL, "SUPERSEDED_METADATA", revisionURL,
	} {
		if strings.Contains(parentText, forbidden) {
			t.Fatalf("compact resource document exposed %q: %q", forbidden, parentText)
		}
	}
	if !strings.Contains(parentText, "curated current excerpt") ||
		packet.AnswerState != "answered" {
		t.Fatalf("current resource projection was damaged: text=%q packet=%+v",
			parentText, packet)
	}
}

func TestCompactResourceHitSupportsExactCanonicalGet(t *testing.T) {
	resourceID := "resource-exact-get"
	documentID, err := compactResourceDocumentID(resourceID)
	if err != nil {
		t.Fatal(err)
	}
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion,
		Revision:      2,
		Fingerprint:   strings.Repeat("e", 64),
		Records: []CaptureRecord{{
			RecordID: "record-exact-get", SourceRef: "slack://fixture/exact-get",
			RawText: "saved external link", ResourceIDs: []string{resourceID},
			ContentHash: strings.Repeat("d", 64),
		}},
		Resources: []ResourceContext{{
			ResourceID:  resourceID,
			Metadata:    ResourceMetadata{Title: "Exact canonical retrieval"},
			ContentHash: strings.Repeat("c", 64),
		}},
	}}
	backend := &compactProjectionBackend{hits: []RankedHit{
		authorizedProjectionHit(documentID, "exact", "canonical", "retrieval"),
	}}
	retriever := NewRetriever(repository, backend)
	packet, err := retriever.SearchCompact(SearchRequest{
		Query: "exact canonical retrieval",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Citations) != 1 ||
		packet.Citations[0].RecordID != "record-exact-get" {
		t.Fatalf("resource hit did not return a canonical record citation: %+v", packet)
	}
	hydrated, err := retriever.Get(packet.Citations[0].RecordID)
	if err != nil {
		t.Fatal(err)
	}
	if hydrated.RecordID != "record-exact-get" ||
		hydrated.Record.RecordID != "record-exact-get" {
		t.Fatalf("canonical get returned the wrong record: %+v", hydrated)
	}
}

func TestCompactProjectionAbstainsForAbsentTopicWithoutHydratingUnselectedRecords(t *testing.T) {
	referenceA := ContentArtifactRef{ArtifactID: "artifact-a", ByteLength: 20}
	referenceB := ContentArtifactRef{ArtifactID: "artifact-b", ByteLength: 20}
	resourceA := "resource-selected"
	resourceB := "resource-unselected"
	documentA, err := compactResourceDocumentID(resourceA)
	if err != nil {
		t.Fatal(err)
	}
	repository := &compactRepository{
		contents: map[string]string{
			referenceA.ArtifactID: "selected private body",
			referenceB.ArtifactID: "UNSELECTED_PRIVATE_BODY",
		},
		library: Library{
			SchemaVersion: LibrarySchemaVersion,
			Revision:      6,
			Fingerprint:   strings.Repeat("6", 64),
			Records: []CaptureRecord{
				{
					RecordID: "record-selected", SourceRef: "slack://fixture/selected",
					RawText: "selected save", ResourceIDs: []string{resourceA},
					ContentHash: strings.Repeat("5", 64),
				},
				{
					RecordID: "record-unselected", SourceRef: "slack://fixture/unselected",
					RawText: "UNSELECTED_SOURCE", ResourceIDs: []string{resourceB},
					ContentHash: strings.Repeat("4", 64),
				},
			},
			Resources: []ResourceContext{
				{ResourceID: resourceA, Content: &referenceA},
				{ResourceID: resourceB, Content: &referenceB},
			},
		},
	}
	backend := &compactProjectionBackend{hits: []RankedHit{
		authorizedProjectionHit(documentA, "selected", "private", "body"),
	}}
	packet, err := NewRetriever(repository, backend).SearchCompact(SearchRequest{
		Query: "selected private body",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.contentLoads != 2 {
		t.Fatalf("unique index resources loaded %d times; selected hydration reloaded content",
			repository.contentLoads)
	}
	data, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Citations) != 1 ||
		packet.Citations[0].RecordID != "record-selected" ||
		strings.Contains(string(data), "record-unselected") ||
		strings.Contains(string(data), "UNSELECTED_SOURCE") ||
		strings.Contains(string(data), resourceB) {
		t.Fatalf("unselected canonical record leaked into output: %s", data)
	}

	absentBackend := &compactProjectionBackend{hits: []RankedHit{{
		DocumentID: documentA,
		Score:      1,
		Components: map[string]float64{
			"semantic_cosine": 0,
		},
	}}}
	absent, err := NewRetriever(repository, absentBackend).SearchCompact(SearchRequest{
		Query: "absent topic",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if absent.AnswerState != "abstained" ||
		absent.AbstentionReason != "no_retrieval_candidates" ||
		len(absent.Citations) != 0 {
		t.Fatalf("absent-topic guard changed: %+v", absent)
	}
}

func TestCompactProjectionFailsClosedBeforeUnknownHitCanAuthorizeKnownCandidate(t *testing.T) {
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion,
		Revision:      3,
		Fingerprint:   strings.Repeat("3", 64),
		Records: []CaptureRecord{{
			RecordID:    "record-known",
			SourceRef:   "slack://fixture/known",
			RawText:     "unrelated retained source",
			ContentHash: strings.Repeat("2", 64),
		}},
	}}
	backend := &compactProjectionBackend{hits: []RankedHit{
		authorizedProjectionHit(
			"stale-or-foreign-document",
			"portable", "quantum", "orchards",
		),
		{
			DocumentID: "record-known",
			Score:      0.5,
			Components: map[string]float64{"semantic_cosine": 0},
		},
	}}
	packet, err := NewRetriever(repository, backend).SearchCompact(SearchRequest{
		Query: "portable quantum orchards",
		Limit: 1,
	})
	if err == nil ||
		!strings.Contains(err.Error(), "unknown compact document identity") {
		t.Fatalf("unknown authorizing hit did not fail closed: packet=%+v err=%v",
			packet, err)
	}
	if packet.AnswerState == "answered" || len(packet.Citations) != 0 {
		t.Fatalf("unknown authorizing hit leaked a known citation: %+v", packet)
	}
}

func corroboratedResourceHit(
	documentID string,
	cosine, rawMargin, distinctMargin, lexicalCoverage float64,
) RankedHit {
	return RankedHit{
		DocumentID: documentID,
		Score:      1,
		Components: map[string]float64{
			"semantic_rank":                     1,
			"semantic_cosine":                   cosine,
			"semantic_margin":                   rawMargin,
			"semantic_distinct_evidence_valid":  1,
			"semantic_distinct_evidence_margin": distinctMargin,
			"lexical_idf_coverage":              lexicalCoverage,
		},
	}
}

func TestCompactCorroboratedResourceRecoveryIgnoresOnlySameResourceSourceSibling(t *testing.T) {
	resourceID := "resource-recovery"
	resourceDocumentID, err := compactResourceDocumentID(resourceID)
	if err != nil {
		t.Fatal(err)
	}
	records := []CaptureRecord{
		{
			RecordID: "record-owner", SourceRef: "slack://fixture/owner",
			RawText: "source sibling", ResourceIDs: []string{resourceID},
			ContentHash: strings.Repeat("a", 64),
		},
		{
			RecordID: "record-competitor", SourceRef: "slack://fixture/competitor",
			RawText:     "different retained evidence",
			ContentHash: strings.Repeat("b", 64),
		},
	}
	run := func(records []CaptureRecord) (CompactContextPacket, []IndexDocument) {
		t.Helper()
		backend := &compactProjectionBackend{
			calibrationID: CompactSemanticCalibrationIdentity,
			hits: []RankedHit{
				corroboratedResourceHit(
					resourceDocumentID, 0.62, 0.01, 0.06, 0.40,
				),
				{
					DocumentID: "record-owner", Score: 0.99,
					Components: map[string]float64{
						"semantic_rank": 2, "semantic_cosine": 0.61,
						"semantic_margin": 0.01,
					},
				},
				{
					DocumentID: "record-competitor", Score: 0.80,
					Components: map[string]float64{
						"semantic_rank": 3, "semantic_cosine": 0.56,
						"semantic_margin": 0.01,
					},
				},
			},
		}
		repository := &compactRepository{library: Library{
			SchemaVersion: LibrarySchemaVersion,
			Revision:      12,
			Fingerprint:   strings.Repeat("1", 64),
			Records:       records,
			Resources: []ResourceContext{{
				ResourceID: resourceID,
				Metadata: ResourceMetadata{
					Title: "Corroborated resource recovery",
				},
				ContentHash: strings.Repeat("c", 64),
			}},
		}}
		packet, err := NewRetriever(repository, backend).SearchCompact(SearchRequest{
			Query: "corroborated resource recovery",
			Limit: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		return packet, backend.documents
	}
	first, firstDocuments := run(records)
	second, secondDocuments := run([]CaptureRecord{records[1], records[0]})
	if first.AnswerState != "answered" ||
		len(first.Citations) != 1 ||
		first.Citations[0].RecordID != "record-owner" {
		t.Fatalf("same-resource source sibling was not recovered: %+v", first)
	}
	if !reflect.DeepEqual(first.Citations, second.Citations) ||
		!reflect.DeepEqual(firstDocuments, secondDocuments) {
		t.Fatalf("corroborated recovery was not replay deterministic")
	}
	documentByID := map[string]IndexDocument{}
	for _, document := range firstDocuments {
		documentByID[document.DocumentID] = document
	}
	resourceDocument := documentByID[resourceDocumentID]
	sourceDocument := documentByID["record-owner"]
	if resourceDocument.AuthorizationEvidenceKind != IndexEvidenceKindUniqueResource ||
		!reflect.DeepEqual(
			resourceDocument.AuthorizationEvidenceAliases,
			[]string{resourceID},
		) ||
		sourceDocument.AuthorizationEvidenceKind != IndexEvidenceKindRecordSource ||
		!reflect.DeepEqual(
			sourceDocument.AuthorizationEvidenceAliases,
			[]string{resourceID},
		) {
		t.Fatalf("authorization evidence aliases were not projected: resource=%+v source=%+v",
			resourceDocument, sourceDocument)
	}
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"compact-resource:",
		"authorization_evidence_aliases",
		"authorization_evidence_kind",
	} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("internal authorization evidence leaked as %q: %s",
				forbidden, data)
		}
	}
}

func TestCompactCorroboratedResourceRecoveryPreservesAbstentionGuards(t *testing.T) {
	resourceID := "resource-recovery-guard"
	resourceDocumentID, err := compactResourceDocumentID(resourceID)
	if err != nil {
		t.Fatal(err)
	}
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion,
		Revision:      13,
		Fingerprint:   strings.Repeat("2", 64),
		Records: []CaptureRecord{
			{
				RecordID: "record-winner", SourceRef: "slack://fixture/winner",
				RawText: "source sibling", ResourceIDs: []string{resourceID},
				ContentHash: strings.Repeat("d", 64),
			},
			{
				RecordID: "record-runner", SourceRef: "slack://fixture/runner",
				RawText:     "different owner runner",
				ContentHash: strings.Repeat("e", 64),
			},
		},
		Resources: []ResourceContext{{
			ResourceID:  resourceID,
			Metadata:    ResourceMetadata{Title: "Recovery guard"},
			ContentHash: strings.Repeat("f", 64),
		}},
	}}
	for _, test := range []struct {
		name          string
		calibrationID string
		hit           RankedHit
	}{
		{
			name:          "different-owner near runner",
			calibrationID: CompactSemanticCalibrationIdentity,
			hit: corroboratedResourceHit(
				resourceDocumentID, 0.62, 0.01, 0.01, 0.40,
			),
		},
		{
			name:          "lexical corroboration below floor",
			calibrationID: CompactSemanticCalibrationIdentity,
			hit: corroboratedResourceHit(
				resourceDocumentID, 0.62, 0.01, 0.06,
				DefaultCompactMinimumSemanticLexicalCover-0.000001,
			),
		},
		{
			name:          "semantic only remains blocked",
			calibrationID: CompactSemanticCalibrationIdentity,
			hit: corroboratedResourceHit(
				resourceDocumentID, 0.66, 0.01, 0.06, 0,
			),
		},
		{
			name:          "stale calibration remains blocked",
			calibrationID: CompactSemanticCalibrationIdentity + "|stale",
			hit: corroboratedResourceHit(
				resourceDocumentID, 0.62, 0.01, 0.06, 0.40,
			),
		},
		{
			name:          "absent near-neighbor remains blocked",
			calibrationID: CompactSemanticCalibrationIdentity,
			hit: corroboratedResourceHit(
				resourceDocumentID, 0.61, 0.01,
				DefaultCompactMinimumSemanticMargin-0.000001, 0.40,
			),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend := &compactProjectionBackend{
				calibrationID: test.calibrationID,
				hits: []RankedHit{
					test.hit,
					{
						DocumentID: "record-winner", Score: 0.99,
						Components: map[string]float64{
							"semantic_rank": 2, "semantic_cosine": 0.61,
							"semantic_margin": 0.01,
						},
					},
					{
						DocumentID: "record-runner", Score: 0.98,
						Components: map[string]float64{
							"semantic_rank": 3, "semantic_cosine": 0.61,
							"semantic_margin": 0.01,
						},
					},
				},
			}
			packet, err := NewRetriever(repository, backend).SearchCompact(
				SearchRequest{Query: "absent conceptual topic", Limit: 3},
			)
			if err != nil {
				t.Fatal(err)
			}
			if packet.AnswerState != "abstained" ||
				len(packet.Citations) != 0 {
				t.Fatalf("recovery guard answered: %+v", packet)
			}
		})
	}
}
