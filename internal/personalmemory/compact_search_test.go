package personalmemory

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

type compactRepository struct {
	library      Library
	contentLoads int
	contents     map[string]string
}

type compactNoiseBackend struct {
	hit RankedHit
}

type compactHitsBackend struct {
	hits          []RankedHit
	calibrationID string
}

type compactQueryCaptureBackend struct {
	request SearchRequest
	hits    []RankedHit
}

func (backend compactNoiseBackend) MethodID() string {
	return "compact-noise-fixture/v0.1"
}

func (backend compactNoiseBackend) Rank(SearchRequest, []IndexDocument) ([]RankedHit, error) {
	return []RankedHit{backend.hit}, nil
}

func (backend compactHitsBackend) MethodID() string {
	return "compact-hits-fixture/v0.1"
}

func (backend compactHitsBackend) Rank(SearchRequest, []IndexDocument) ([]RankedHit, error) {
	return append([]RankedHit(nil), backend.hits...), nil
}

func (backend compactHitsBackend) CompactSemanticCalibrationID() string {
	return backend.calibrationID
}

func (backend *compactQueryCaptureBackend) MethodID() string {
	return "compact-query-capture/v0.1"
}

func (backend *compactQueryCaptureBackend) Rank(
	request SearchRequest,
	_ []IndexDocument,
) ([]RankedHit, error) {
	backend.request = request
	return append([]RankedHit(nil), backend.hits...), nil
}

func (repository *compactRepository) Load() (Library, error) {
	return repository.library, nil
}

func (*compactRepository) Import(CaptureBatch) (ImportReceipt, error) {
	return ImportReceipt{}, nil
}

func (*compactRepository) MergeEnrichment(EnrichmentBatch) (EnrichmentReceipt, error) {
	return EnrichmentReceipt{}, nil
}

func (repository *compactRepository) LoadContent(
	reference ContentArtifactRef,
) (ExtractedContentArtifact, error) {
	repository.contentLoads++
	return ExtractedContentArtifact{
		Reference: reference,
		Text:      repository.contents[reference.ArtifactID],
	}, nil
}

func TestCompactSearchIndexesContentPrivatelyAndOmitsHistoricalMissingnessAndPaths(t *testing.T) {
	resourceID := "resource-current"
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion, Revision: 7, Fingerprint: strings.Repeat("a", 64),
		Records: []CaptureRecord{{
			RecordID: "record-current", SourceRef: "slack://fixture/current",
			OccurredAt: "2026-07-27T10:00:00Z", RawText: "durable contextual agent memory",
			ResourceIDs: []string{resourceID}, Missingness: []string{"current_capture_gap"},
			ContextState: "partial", ContentHash: strings.Repeat("b", 64),
		}},
		Resources: []ResourceContext{{
			ResourceID: resourceID, State: "partial", AccessClass: "public",
			Metadata:    ResourceMetadata{Title: "Agent memory"},
			Missingness: []string{"current_resource_gap"}, ContentHash: strings.Repeat("c", 64),
			Content: &ContentArtifactRef{
				ArtifactID: "content-artifact", ByteLength: 128,
			},
			AuthorityClass: AuthorityClass,
		}},
		ResourceRevisions: []ResourceRevision{{
			RevisionID: "resource-revision",
			Resource: ResourceContext{
				ResourceID: resourceID, Missingness: []string{"historical_gap"},
			},
		}},
	}}
	packet, err := NewLexicalRetriever(repository).SearchCompact(SearchRequest{
		Query: "contextual agent memory", Limit: 3, RunID: "run-compact",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.contentLoads != 1 {
		t.Fatalf("compact search loaded %d current content artifacts for ranking", repository.contentLoads)
	}
	if packet.SchemaVersion != CompactPacketSchemaVersion ||
		packet.AnswerState != "answered" || len(packet.Citations) != 1 {
		t.Fatalf("compact packet=%+v", packet)
	}
	citation := packet.Citations[0]
	if !containsString(citation.Missingness, "current_capture_gap") ||
		!containsString(citation.Missingness, "current_resource_gap") ||
		containsString(citation.Missingness, "historical_gap") {
		t.Fatalf("compact current missingness=%v", citation.Missingness)
	}
	evidenceIdentities := map[string]bool{}
	for _, reference := range citation.EvidenceRefs {
		identity := strings.Join([]string{
			reference.ResourceID, reference.ResourceVersionState,
			reference.ResourceRevisionID, reference.ExcerptID,
			reference.ArtifactID, reference.Locator,
		}, "\x00")
		if evidenceIdentities[identity] {
			t.Fatalf("compact citation repeated evidence reference: %+v", reference)
		}
		evidenceIdentities[identity] = true
	}
	data, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"records"`, `"resources"`, `"resource_revisions"`, `"raw_text"`,
		`"canonical_url"`, `"database_path"`, `"runtime_path"`, "/private/",
	} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("compact packet exposed %q: %s", forbidden, data)
		}
	}
}

func TestCompactSearchIndexesOnlyCurrentRecords(t *testing.T) {
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion, Revision: 2,
		Fingerprint: strings.Repeat("7", 64),
		Records: []CaptureRecord{{
			RecordID: "record-current", SourceRef: "slack://fixture/current",
			RawText: "current portable lesson", ContentHash: strings.Repeat("8", 64),
		}},
		Revisions: []CaptureRevision{{
			RevisionID: "revision-superseded",
			Record: CaptureRecord{
				RecordID: "record-current", SourceRef: "slack://fixture/current",
				RawText: "obsolete quartz folklore", ContentHash: strings.Repeat("9", 64),
			},
		}},
	}}
	packet, err := NewLexicalRetriever(repository).SearchCompact(SearchRequest{
		Query: "obsolete quartz folklore", Limit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if packet.AnswerState != "abstained" || len(packet.Citations) != 0 {
		t.Fatalf("compact search returned a superseded revision: %+v", packet)
	}
}

func TestCompactChangesDoNotAlterLegacyV02PacketShape(t *testing.T) {
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion, Revision: 1,
		Fingerprint: strings.Repeat("5", 64),
		Records: []CaptureRecord{{
			RecordID: "record-legacy", SourceRef: "slack://fixture/legacy",
			RawText: "legacy compatible recall", ContentHash: strings.Repeat("6", 64),
		}},
	}}
	packet, err := NewLexicalRetriever(repository).Search(SearchRequest{
		Query: "legacy compatible recall", Limit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	if packet.SchemaVersion != ContextPacketSchemaVersion ||
		strings.Contains(string(data), `"answer_state"`) ||
		strings.Contains(string(data), `"abstention_policy_fingerprint"`) {
		t.Fatalf("legacy v0.2 packet shape changed: %s", data)
	}
}

func TestCompactSearchFindsExtractedLessonWithoutReturningFullBody(t *testing.T) {
	tail := "PRIVATE_FULL_BODY_TAIL_MUST_NOT_BE_RETURNED"
	content := "portable contextual systems make saved lessons useful " +
		strings.Repeat("bounded private context ", 40) + tail
	reference := ContentArtifactRef{
		ArtifactID: "saved-lesson", ByteLength: len([]byte(content)),
		RuneCount: len([]rune(content)),
	}
	repository := &compactRepository{
		contents: map[string]string{reference.ArtifactID: content},
		library: Library{
			SchemaVersion: LibrarySchemaVersion, Revision: 1,
			Fingerprint: strings.Repeat("f", 64),
			Records: []CaptureRecord{{
				RecordID: "record-lesson", SourceRef: "slack://fixture/lesson",
				RawText: "https://example.invalid/saved", ResourceIDs: []string{"resource-lesson"},
				ContentHash: strings.Repeat("1", 64),
			}},
			Resources: []ResourceContext{{
				ResourceID: "resource-lesson", State: "complete",
				Content: &reference, ContentHash: strings.Repeat("2", 64),
				AuthorityClass: AuthorityClass,
			}},
		},
	}
	packet, err := NewLexicalRetriever(repository).SearchCompact(SearchRequest{
		Query: "portable contextual systems", Limit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repository.contentLoads != 1 || packet.AnswerState != "answered" ||
		len(packet.Citations) != 1 {
		t.Fatalf("extracted lesson was not privately indexed: loads=%d packet=%+v",
			repository.contentLoads, packet)
	}
	citation := packet.Citations[0]
	if len([]rune(citation.Snippet)) > 500 {
		t.Fatalf("compact citation snippet exceeded bound: %d", len([]rune(citation.Snippet)))
	}
	foundArtifactReference := false
	for _, reference := range citation.EvidenceRefs {
		if len([]rune(reference.MatchedSnippet)) > 500 {
			t.Fatalf("compact evidence snippet exceeded bound: %d",
				len([]rune(reference.MatchedSnippet)))
		}
		foundArtifactReference = foundArtifactReference ||
			reference.ArtifactID == "saved-lesson"
	}
	if !foundArtifactReference {
		t.Fatalf("compact citation omitted extracted-content evidence: %+v", citation.EvidenceRefs)
	}
	data, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), content) || strings.Contains(string(data), tail) {
		t.Fatalf("compact response returned unbounded extracted body: %s", data)
	}
}

func TestCompactSearchAbstainsForStopwordOnlyAndNoiseResults(t *testing.T) {
	repository := &compactRepository{library: EmptyLibrary()}
	packet, err := NewLexicalRetriever(repository).SearchCompact(SearchRequest{
		Query: "what is this and how", Limit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if packet.AnswerState != "abstained" ||
		packet.AbstentionReason != "query_has_no_meaningful_terms" ||
		len(packet.Citations) != 0 {
		t.Fatalf("stopword query did not abstain: %+v", packet)
	}
}

func TestCompactSearchPreservesSemanticQueryAndSeparatesLexicalTerms(t *testing.T) {
	backend := &compactQueryCaptureBackend{}
	repository := &compactRepository{library: EmptyLibrary()}
	_, err := NewRetriever(repository, backend).SearchCompact(SearchRequest{
		Query: "How should contextual agents work?", Limit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if backend.request.Query != "How should contextual agents work?" ||
		backend.request.LexicalQuery != "agents contextual" ||
		backend.request.Limit != MaximumSearchLimit {
		t.Fatalf("compact ranking query was damaged: %+v", backend.request)
	}
}

func TestCompactSearchDiscardsUnsubstantiatedRankerNoise(t *testing.T) {
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion, Revision: 1,
		Fingerprint: strings.Repeat("d", 64),
		Records: []CaptureRecord{{
			RecordID: "record-noise", SourceRef: "slack://fixture/noise",
			RawText: "unrelated retained item", ContentHash: strings.Repeat("e", 64),
		}},
	}}
	for _, hit := range []RankedHit{
		{DocumentID: "record-noise", Score: 1},
		{
			DocumentID: "record-noise", Score: 1,
			Components: map[string]float64{"semantic_cosine": 0},
		},
	} {
		packet, err := NewRetriever(repository, compactNoiseBackend{hit: hit}).SearchCompact(
			SearchRequest{Query: "quantum orchards", Limit: 3},
		)
		if err != nil {
			t.Fatal(err)
		}
		if packet.AnswerState != "abstained" ||
			packet.AbstentionReason != "no_retrieval_candidates" ||
			len(packet.Citations) != 0 {
			t.Fatalf("unsubstantiated ranker noise was returned: %+v", packet)
		}
	}
}

func TestCompactLexicalAuthorizationBoundaries(t *testing.T) {
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion, Revision: 1,
		Fingerprint: strings.Repeat("1", 64),
		Records: []CaptureRecord{{
			RecordID: "record-lexical", SourceRef: "slack://fixture/lexical",
			RawText: "retained context", ContentHash: strings.Repeat("2", 64),
		}},
	}}
	for _, test := range []struct {
		name       string
		query      string
		components map[string]float64
		expected   string
	}{
		{
			name: "ordered three term phrase", query: "portable quantum orchards",
			components: lexicalAuthorizationComponents(3, 3, 1, 1, 0.01, 0),
			expected:   "answered",
		},
		{
			name: "rare ordered two term phrase", query: "portable quantum",
			components: lexicalAuthorizationComponents(2, 2, 1, 1, 0.01, 0),
			expected:   "answered",
		},
		{
			name: "common ordered two term phrase", query: "portable quantum",
			components: lexicalAuthorizationComponents(2, 2, 1, 1, 0.010001, 0),
			expected:   "abstained",
		},
		{
			name: "adjacent pair alone", query: "portable quantum orchards",
			components: lexicalAuthorizationComponents(3, 2, 0.67, 0, 0.001, 1),
			expected:   "abstained",
		},
		{
			name: "single term", query: "portable",
			components: lexicalAuthorizationComponents(1, 1, 1, 0, 0.001, 1),
			expected:   "abstained",
		},
		{
			name: "coverage and margin boundary", query: "portable quantum orchard systems",
			components: lexicalAuthorizationComponents(4, 3, 0.80, 0, 0.01, 0.15),
			expected:   "answered",
		},
		{
			name: "below margin", query: "portable quantum orchard systems",
			components: lexicalAuthorizationComponents(4, 3, 0.80, 0, 0.01, 0.149999),
			expected:   "abstained",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet, err := NewRetriever(repository, compactNoiseBackend{hit: RankedHit{
				DocumentID: "record-lexical", Score: 1, Components: test.components,
			}}).SearchCompact(SearchRequest{Query: test.query, Limit: 3})
			if err != nil {
				t.Fatal(err)
			}
			if packet.AnswerState != test.expected {
				t.Fatalf("lexical evidence result=%+v", packet)
			}
		})
	}
}

func lexicalAuthorizationComponents(
	queryTerms, matchedTerms int,
	idfCoverage, exactPhrase, rarestRatio, winnerMargin float64,
) map[string]float64 {
	return map[string]float64{
		"lexical_raw": 1, "lexical_rank": 1,
		"lexical_query_terms": float64(queryTerms), "lexical_matched_terms": float64(matchedTerms),
		"lexical_idf_coverage": idfCoverage, "lexical_exact_ordered_phrase": exactPhrase,
		"lexical_rarest_document_ratio":  rarestRatio,
		"lexical_winner_relative_margin": winnerMargin,
	}
}

func TestLexicalBM25ScoresOriginalQueryAndCarriesFilteredAuthorization(t *testing.T) {
	documents := make([]IndexDocument, 0, 101)
	for index := 0; index < 100; index++ {
		documents = append(documents, IndexDocument{
			DocumentID: "common-" + string(rune('Ā'+index)),
			Text:       "how filler",
		})
	}
	documents = append(documents, IndexDocument{
		DocumentID: "rare", Text: "contextual agent memory",
	})
	hits, err := (LexicalBM25Backend{}).Rank(SearchRequest{
		Query: "how contextual agent memory", LexicalQuery: "agent contextual memory", Limit: 100,
	}, documents)
	if err != nil {
		t.Fatal(err)
	}
	foundOriginalOnlyMatch := false
	for _, hit := range hits {
		if strings.HasPrefix(hit.DocumentID, "common-") && hit.Score > 0 &&
			hit.Components["lexical_matched_terms"] == 0 {
			foundOriginalOnlyMatch = true
		}
	}
	if !foundOriginalOnlyMatch {
		t.Fatalf("original query was not scored separately: %+v", hits)
	}
}

func TestCompactSemanticAbstentionThresholdIsFrozenAndBoundToPacket(t *testing.T) {
	policy := DefaultCompactAbstentionPolicy()
	if policy != DefaultCompactAbstentionPolicy() ||
		policy.SchemaVersion != CompactAbstentionPolicySchemaVersion ||
		policy.MinimumSemanticCosine != DefaultCompactMinimumSemanticCosine ||
		policy.MinimumSemanticMargin != DefaultCompactMinimumSemanticMargin ||
		policy.MinimumSemanticOnlyCosine != DefaultCompactMinimumSemanticOnlyCosine ||
		policy.MinimumSemanticOnlyMargin != DefaultCompactMinimumSemanticOnlyMargin ||
		policy.MinimumSemanticLexicalCoverage != DefaultCompactMinimumSemanticLexicalCover ||
		policy.Fingerprint != "b9766ff76024ab3080e70c4128d7a4165c9c7f34c65819aa863b96389028a7a4" {
		t.Fatalf("compact abstention policy is not deterministic: %+v", policy)
	}
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion, Revision: 1,
		Fingerprint: strings.Repeat("3", 64),
		Records: []CaptureRecord{{
			RecordID: "record-semantic", SourceRef: "slack://fixture/semantic",
			RawText: "retained context", ContentHash: strings.Repeat("4", 64),
		}},
	}}
	for _, test := range []struct {
		name     string
		cosine   float64
		expected string
	}{
		{name: "just below", cosine: DefaultCompactMinimumSemanticOnlyCosine - 0.000001, expected: "abstained"},
		{name: "at threshold", cosine: DefaultCompactMinimumSemanticOnlyCosine, expected: "answered"},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet, err := NewRetriever(repository, compactHitsBackend{
				calibrationID: CompactSemanticCalibrationIdentity,
				hits: []RankedHit{{
					DocumentID: "record-semantic", Score: 1,
					Components: map[string]float64{
						"semantic_cosine": test.cosine, "semantic_rank": 1,
						"semantic_margin": DefaultCompactMinimumSemanticOnlyMargin,
					},
				}},
			}).SearchCompact(SearchRequest{Query: "semantic threshold", Limit: 3})
			if err != nil {
				t.Fatal(err)
			}
			if packet.AnswerState != test.expected ||
				packet.AbstentionPolicyFingerprint != policy.Fingerprint {
				t.Fatalf("threshold result=%+v policy=%+v", packet, policy)
			}
		})
	}
}

func TestCompactSemanticAnswerRequiresWinnerMargin(t *testing.T) {
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion, Revision: 1,
		Fingerprint: strings.Repeat("a", 64),
		Records: []CaptureRecord{
			{
				RecordID: "record-first", SourceRef: "slack://fixture/first",
				RawText: "first context", ContentHash: strings.Repeat("b", 64),
			},
			{
				RecordID: "record-second", SourceRef: "slack://fixture/second",
				RawText: "second context", ContentHash: strings.Repeat("c", 64),
			},
		},
	}}
	packet, err := NewRetriever(repository, compactHitsBackend{
		calibrationID: CompactSemanticCalibrationIdentity,
		hits: []RankedHit{
			{
				DocumentID: "record-first", Score: 1,
				Components: map[string]float64{
					"semantic_cosine": 0.80, "semantic_rank": 1, "semantic_margin": 0.01,
				},
			},
			{
				DocumentID: "record-second", Score: 0.9,
				Components: map[string]float64{
					"semantic_cosine": 0.79, "semantic_rank": 2, "semantic_margin": 0.01,
				},
			},
		}}).SearchCompact(SearchRequest{Query: "semantic ambiguity", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if packet.AnswerState != "abstained" || len(packet.Citations) != 0 {
		t.Fatalf("flat semantic ranking did not abstain: %+v", packet)
	}
}

func TestCompactSemanticV07ExpandsCalibratedAuthorizationWithoutWeakeningAbsentGuards(t *testing.T) {
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion, Revision: 1,
		Fingerprint: strings.Repeat("7", 64),
		Records: []CaptureRecord{
			{
				RecordID: "record-first", SourceRef: "slack://fixture/v07-first",
				RawText: "first retained context", ContentHash: strings.Repeat("8", 64),
			},
			{
				RecordID: "record-neighbor", SourceRef: "slack://fixture/v07-neighbor",
				RawText: "near-neighbor retained context", ContentHash: strings.Repeat("9", 64),
			},
		},
	}}
	semanticOnlyExpanded := RankedHit{
		DocumentID: "record-first", Score: 1,
		Components: map[string]float64{
			"semantic_cosine": 0.66, "semantic_rank": 1, "semantic_margin": 0.05,
		},
	}
	corroboratedExpanded := RankedHit{
		DocumentID: "record-first", Score: 1,
		Components: map[string]float64{
			"semantic_cosine": 0.62, "semantic_rank": 1, "semantic_margin": 0.035,
			"lexical_idf_coverage": 0.40,
		},
	}
	for _, test := range []struct {
		name          string
		calibrationID string
		hits          []RankedHit
		expected      string
	}{
		{
			name:          "semantic-only band below v0.6 is authorized",
			calibrationID: CompactSemanticCalibrationIdentity,
			hits:          []RankedHit{semanticOnlyExpanded},
			expected:      "answered",
		},
		{
			name:          "semantic plus lexical band below v0.6 is authorized",
			calibrationID: CompactSemanticCalibrationIdentity,
			hits:          []RankedHit{corroboratedExpanded},
			expected:      "answered",
		},
		{
			name:          "unknown calibration remains forbidden",
			calibrationID: CompactSemanticCalibrationIdentity + "|stale-policy=v0.6",
			hits:          []RankedHit{semanticOnlyExpanded},
			expected:      "abstained",
		},
		{
			name:          "near-neighbor below semantic-only margin abstains",
			calibrationID: CompactSemanticCalibrationIdentity,
			hits: []RankedHit{
				{
					DocumentID: "record-first", Score: 1,
					Components: map[string]float64{
						"semantic_cosine": 0.80, "semantic_rank": 1,
						"semantic_margin": DefaultCompactMinimumSemanticOnlyMargin - 0.000001,
					},
				},
				{
					DocumentID: "record-neighbor", Score: 0.99,
					Components: map[string]float64{
						"semantic_cosine": 0.80 - DefaultCompactMinimumSemanticOnlyMargin + 0.000001,
						"semantic_rank":   2,
						"semantic_margin": DefaultCompactMinimumSemanticOnlyMargin - 0.000001,
					},
				},
			},
			expected: "abstained",
		},
		{
			name:          "low-margin corroborated neighbor abstains",
			calibrationID: CompactSemanticCalibrationIdentity,
			hits: []RankedHit{{
				DocumentID: "record-first", Score: 1,
				Components: map[string]float64{
					"semantic_cosine": 0.80, "semantic_rank": 1,
					"semantic_margin":      DefaultCompactMinimumSemanticMargin - 0.000001,
					"lexical_idf_coverage": 1,
				},
			}},
			expected: "abstained",
		},
		{
			name:          "low-coverage corroboration abstains",
			calibrationID: CompactSemanticCalibrationIdentity,
			hits: []RankedHit{{
				DocumentID: "record-first", Score: 1,
				Components: map[string]float64{
					"semantic_cosine": DefaultCompactMinimumSemanticCosine,
					"semantic_rank":   1, "semantic_margin": DefaultCompactMinimumSemanticMargin,
					"lexical_idf_coverage": DefaultCompactMinimumSemanticLexicalCover - 0.000001,
				},
			}},
			expected: "abstained",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet, err := NewRetriever(repository, compactHitsBackend{
				calibrationID: test.calibrationID, hits: test.hits,
			}).SearchCompact(SearchRequest{Query: "v07 calibration boundary", Limit: 3})
			if err != nil {
				t.Fatal(err)
			}
			if packet.AnswerState != test.expected {
				t.Fatalf("v0.7 authorization result=%+v", packet)
			}
			if test.expected == "abstained" && len(packet.Citations) != 0 {
				t.Fatalf("v0.7 abstention leaked citations: %+v", packet)
			}
		})
	}
}

func TestCompactSearchAuthorizesFromFullPoolThenReturnsCallerLimit(t *testing.T) {
	records := make([]CaptureRecord, 0, MaximumSearchLimit)
	hits := make([]RankedHit, 0, MaximumSearchLimit)
	for index := 0; index < MaximumSearchLimit; index++ {
		id := "record-" + strconv.Itoa(index)
		records = append(records, CaptureRecord{
			RecordID: id, SourceRef: "slack://fixture/" + id,
			RawText: "retained context", ContentHash: strings.Repeat("d", 64),
		})
		hit := RankedHit{DocumentID: id, Score: float64(MaximumSearchLimit - index)}
		if index == 75 {
			hit.Components = lexicalAuthorizationComponents(3, 3, 1, 1, 1, 0)
		}
		hits = append(hits, hit)
	}
	backend := &compactQueryCaptureBackend{hits: hits}
	packet, err := NewRetriever(&compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion, Revision: 1,
		Fingerprint: strings.Repeat("e", 64), Records: records,
	}}, backend).SearchCompact(SearchRequest{
		Query: "portable quantum orchards", Limit: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if backend.request.Limit != MaximumSearchLimit ||
		packet.AnswerState != "answered" || len(packet.Citations) != 3 ||
		packet.Citations[0].RecordID != "record-0" {
		t.Fatalf("compact pool/backfill contract failed: request=%+v packet=%+v",
			backend.request, packet)
	}
}

func TestCompactSemanticAuthorizationRequiresCalibrationAndSupportsCorroboration(t *testing.T) {
	repository := &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion, Revision: 1,
		Fingerprint: strings.Repeat("f", 64),
		Records: []CaptureRecord{{
			RecordID: "record-semantic", SourceRef: "slack://fixture/semantic",
			RawText: "retained context", ContentHash: strings.Repeat("1", 64),
		}},
	}}
	hit := RankedHit{
		DocumentID: "record-semantic", Score: 1,
		Components: map[string]float64{
			"semantic_cosine": DefaultCompactMinimumSemanticCosine,
			"semantic_rank":   1, "semantic_margin": DefaultCompactMinimumSemanticMargin,
			"lexical_idf_coverage": DefaultCompactMinimumSemanticLexicalCover,
		},
	}
	for _, test := range []struct {
		name, calibration, expected string
	}{
		{name: "missing calibration", expected: "abstained"},
		{name: "exact calibration", calibration: CompactSemanticCalibrationIdentity, expected: "answered"},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet, err := NewRetriever(repository, compactHitsBackend{
				hits: []RankedHit{hit}, calibrationID: test.calibration,
			}).SearchCompact(SearchRequest{Query: "semantic corroboration", Limit: 3})
			if err != nil {
				t.Fatal(err)
			}
			if packet.AnswerState != test.expected {
				t.Fatalf("semantic calibration result=%+v", packet)
			}
		})
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
