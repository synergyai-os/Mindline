package personalmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	ContextPacketSchemaVersion   = "mindline-agent-context-packet/v0.2"
	CompactPacketSchemaVersion   = "mindline-agent-context-packet/v0.3"
	HydratedCaptureSchemaVersion = "mindline-hydrated-capture/v0.1"
	MaximumLensRequestRunes      = 64 << 10
	MaximumRetrievalContentBytes = 64 << 20
	MaximumCitationEvidenceRefs  = 32
	MaximumCompactResourceStates = 32
	MaximumCompactSnippetRunes   = 500
	MaximumCompactIndexDocuments = MaximumRecords + MaximumResources
	MaximumCompactOwnerLinks     = 1_000_000
	DefaultSearchLimit           = 10
	MaximumSearchLimit           = 100

	CompactAbstentionPolicySchemaVersion      = "mindline-compact-abstention-policy/v0.7"
	DefaultCompactMinimumSemanticCosine       = 0.60
	DefaultCompactMinimumSemanticMargin       = 0.03
	DefaultCompactMinimumSemanticOnlyCosine   = 0.64
	DefaultCompactMinimumSemanticOnlyMargin   = 0.04
	DefaultCompactMinimumSemanticLexicalCover = 0.35
	DefaultCompactMinimumLexicalIDFCoverage   = 0.80
	DefaultCompactMaximumLexicalDocumentRatio = 0.01
	DefaultCompactMinimumLexicalWinnerMargin  = 0.15
	DefaultCompactMinimumLexicalMatchedTerms  = 3
	DefaultCompactMinimumOrderedPhraseTerms   = 3
	CompactSemanticCalibrationIdentity        = "ollama/embeddinggemma:latest/retrieval-input-v0.2|query-prompt=search-result-v0.1|document-prompt=title-none-v0.1|document-projection=record-source+unique-current-resource+authorization-evidence-alias-v0.10|distinct-resource-evidence-margin=v0.1|chunk-runes=2000|chunk-overlap=200|max-chunks=8"
	compactLexicalEvidenceRule                = "rank1_ordered_phrase_or_rare_idf_coverage"
	compactStopwordPolicy                     = "mindline-english-stopwords/v0.2"
	compactRankingIdentity                    = "bm25-original-query|authorization-meaningful-query|candidate-pool=100|documents=record-source+unique-current-resource+authorization-evidence-alias-v0.10|raw-hit-identity=fail-closed-v0.1|authorization=existing-v0.7-then-calibrated-corroborated-resource-v0.1|owner-expansion=post-authorization-v0.1|feedback-alias=observed-owner-mean-v0.1|relevance-lookup-chunk=100000|rrf-k=60|lexical-weight=2|semantic-weight=1|lens-rerank-only"
	compactChunkingIdentity                   = "document-projection=record-source+unique-current-resource+authorization-evidence-alias-v0.10|chunk-runes=2000|chunk-overlap=200|max-chunks=8"
	compactResourceDocumentPrefix             = "compact-resource:"
	maximumCompactDocumentIDRunes             = 128

	IndexEvidenceKindRecordSource   = "record_source"
	IndexEvidenceKindUniqueResource = "unique_resource"
)

// RetrieverPort is the agent-facing canonical Mindline contract.
type RetrieverPort interface {
	Search(SearchRequest) (ContextPacket, error)
	SearchCompact(SearchRequest) (CompactContextPacket, error)
	Get(string) (HydratedCapture, error)
}

// RetrievalBackendPort owns ranking only. It never owns canonical hydration,
// citations, authority labels, or context-packet assembly.
type RetrievalBackendPort interface {
	MethodID() string
	Rank(SearchRequest, []IndexDocument) ([]RankedHit, error)
}

type RetrievalDiagnosticsPort interface {
	RetrievalDiagnostics() (state, degradedReason string)
}

// CompactSemanticCalibrationPort identifies a backend whose semantic scores
// are calibrated for compact answer authorization. Unknown providers may rank,
// but their scores cannot grant permission to answer.
type CompactSemanticCalibrationPort interface {
	CompactSemanticCalibrationID() string
}

type IndexDocument struct {
	DocumentID string
	Text       string
	// FeedbackAliases are canonical item identities whose recorded relevance
	// applies to this retrieval-only document. An empty set means DocumentID.
	FeedbackAliases []string
	// AuthorizationEvidenceAliases identify the independent evidence represented
	// by this retrieval document. They are ranking-only and never packet output.
	AuthorizationEvidenceAliases []string
	AuthorizationEvidenceKind    string
}

type RankedHit struct {
	DocumentID   string
	Score        float64
	MatchedTerms []string
	Components   map[string]float64
}

type ContextRetriever struct {
	repository RepositoryPort
	backend    RetrievalBackendPort
}

// CompactIndexSnapshot exposes the source-neutral, retrieval-only projection
// used by compact search so the local service can prepare derived indexes
// outside the request path. It never exposes this projection over the API.
type CompactIndexSnapshot struct {
	LibraryFingerprint string
	Documents          []IndexDocument
}

type LexicalBM25Backend struct{}

func (LexicalBM25Backend) MethodID() string {
	return "mindline_lexical_bm25_baseline/v0.1"
}

func NewRetriever(repository RepositoryPort, backend RetrievalBackendPort) ContextRetriever {
	return ContextRetriever{repository: repository, backend: backend}
}

func NewLexicalRetriever(repository RepositoryPort) ContextRetriever {
	return NewRetriever(repository, LexicalBM25Backend{})
}

func (retriever ContextRetriever) PrepareCompactIndex() (CompactIndexSnapshot, error) {
	projection, err := retriever.prepareCompactProjection()
	if err != nil {
		return CompactIndexSnapshot{}, err
	}
	documents := make([]IndexDocument, len(projection.indexDocuments))
	copy(documents, projection.indexDocuments)
	return CompactIndexSnapshot{
		LibraryFingerprint: projection.library.Fingerprint,
		Documents:          documents,
	}, nil
}

type evidenceDocument struct {
	id                string
	record            CaptureRecord
	versionState      string
	resources         []ResourceContext
	resourceRevisions []ResourceRevision
	contents          map[string]ExtractedContentArtifact
	revisionContents  map[string]ExtractedContentArtifact
	searchText        string
}

type compactRetrievalProjection struct {
	library              Library
	indexDocuments       []IndexDocument
	recordsByID          map[string]CaptureRecord
	resourcesByID        map[string]ResourceContext
	ownersByDocumentID   map[string][]string
	resourceByDocumentID map[string]string
	contentCache         map[string]ExtractedContentArtifact
	hydratedBytes        int
}

func (retriever ContextRetriever) Search(request SearchRequest) (ContextPacket, error) {
	if retriever.backend == nil {
		return ContextPacket{}, errors.New("personal evidence retrieval backend is unavailable")
	}
	library, documents, err := retriever.prepareDocuments()
	if err != nil {
		return ContextPacket{}, err
	}
	indexDocuments := make([]IndexDocument, 0, len(documents))
	byID := make(map[string]evidenceDocument, len(documents))
	for _, document := range documents {
		indexDocuments = append(indexDocuments, IndexDocument{DocumentID: document.id, Text: document.searchText})
		byID[document.id] = document
	}
	hits, err := retriever.backend.Rank(request, indexDocuments)
	if err != nil {
		return ContextPacket{}, err
	}
	method := strings.TrimSpace(retriever.backend.MethodID())
	if method == "" {
		return ContextPacket{}, errors.New("personal evidence retrieval backend has no method identity")
	}
	packet := assembleContextPacket(request, library, hits, byID, method)
	if diagnostics, ok := retriever.backend.(RetrievalDiagnosticsPort); ok {
		packet.RetrievalState, packet.DegradedReason = diagnostics.RetrievalDiagnostics()
	}
	return packet, nil
}

func (retriever ContextRetriever) SearchCompact(request SearchRequest) (CompactContextPacket, error) {
	if retriever.backend == nil {
		return CompactContextPacket{}, errors.New("personal evidence retrieval backend is unavailable")
	}
	method := strings.TrimSpace(retriever.backend.MethodID())
	if method == "" {
		return CompactContextPacket{}, errors.New("personal evidence retrieval backend has no method identity")
	}
	policy := DefaultCompactAbstentionPolicy()
	callerLimit := request.Limit
	if callerLimit <= 0 {
		callerLimit = DefaultSearchLimit
	}
	if callerLimit > MaximumSearchLimit {
		return CompactContextPacket{}, errors.New("search limit exceeds 100")
	}
	queryTerms := meaningfulQueryTerms(request.Query)
	if len(queryTerms) == 0 {
		library, err := retriever.repository.Load()
		if err != nil {
			return CompactContextPacket{}, err
		}
		packet := assembleCompactContextPacket(request, library, nil, nil, method, policy)
		packet.AnswerState = "abstained"
		packet.AbstentionReason = "query_has_no_meaningful_terms"
		return packet, nil
	}
	projection, err := retriever.prepareCompactProjection()
	if err != nil {
		return CompactContextPacket{}, err
	}
	rankingRequest := request
	rankingRequest.LexicalQuery = strings.Join(queryTerms, " ")
	rankingRequest.Limit = MaximumSearchLimit
	rawHits, err := retriever.backend.Rank(rankingRequest, projection.indexDocuments)
	if err != nil {
		return CompactContextPacket{}, err
	}
	// Rank may switch a hybrid backend into an explicit degraded mode. Read the
	// method afterwards so the packet describes the retrieval that actually ran.
	method = strings.TrimSpace(retriever.backend.MethodID())
	if method == "" {
		return CompactContextPacket{}, errors.New("personal evidence retrieval backend has no method identity")
	}
	if err := validateCompactRawHitIdentities(rawHits, projection); err != nil {
		return CompactContextPacket{}, err
	}
	calibrationID := ""
	if calibrated, ok := retriever.backend.(CompactSemanticCalibrationPort); ok {
		calibrationID = strings.TrimSpace(calibrated.CompactSemanticCalibrationID())
	}
	rawHits = usableCompactHits(rawHits, policy, calibrationID, projection)
	hits, selectedResources, err := expandCompactHits(rawHits, projection, callerLimit)
	if err != nil {
		return CompactContextPacket{}, err
	}
	documents, err := retriever.hydrateCompactRecords(
		hits, selectedResources, &projection,
	)
	if err != nil {
		return CompactContextPacket{}, err
	}
	packet := assembleCompactContextPacket(
		request, projection.library, hits, documents, method, policy,
	)
	if diagnostics, ok := retriever.backend.(RetrievalDiagnosticsPort); ok {
		packet.RetrievalState, packet.DegradedReason = diagnostics.RetrievalDiagnostics()
	}
	return packet, nil
}

func (retriever ContextRetriever) Get(recordID string) (HydratedCapture, error) {
	library, err := retriever.repository.Load()
	if err != nil {
		return HydratedCapture{}, err
	}
	resourcesByID := make(map[string]ResourceContext, len(library.Resources))
	for _, resource := range library.Resources {
		resourcesByID[resource.ResourceID] = resource
	}
	resourceRevisions := groupResourceRevisions(library.ResourceRevisions)
	var record CaptureRecord
	versionState := ""
	if current, exists := findCurrentRecord(library.Records, recordID); exists {
		record = current
		versionState = "current"
	} else if revision, exists := findRevision(library.Revisions, recordID); exists {
		record = revision.Record
		versionState = "superseded"
	} else {
		return HydratedCapture{}, errors.New("personal evidence record not found")
	}
	hydratedBytes := 0
	document, err := retriever.prepareDocument(recordID, record, versionState, resourcesByID, resourceRevisions, &hydratedBytes)
	if err != nil {
		return HydratedCapture{}, err
	}
	contents := make([]ExtractedContentArtifact, 0, len(document.contents))
	for _, resource := range document.resources {
		if content, exists := document.contents[resource.ResourceID]; exists {
			contents = append(contents, content)
		}
	}
	return HydratedCapture{
		SchemaVersion: HydratedCaptureSchemaVersion,
		RecordID:      document.id, VersionState: document.versionState,
		Record: document.record, Resources: document.resources,
		ResourceRevisions: document.resourceRevisions, Contents: append(contents, orderedRevisionContents(document)...),
	}, nil
}

func (retriever ContextRetriever) ReviewLenses(batch LensBatch, limit int) (LensReviewPacket, error) {
	if retriever.backend == nil {
		return LensReviewPacket{}, errors.New("personal evidence retrieval backend is unavailable")
	}
	if err := validateLensBatch(batch); err != nil {
		return LensReviewPacket{}, err
	}
	before, err := retriever.repository.Load()
	if err != nil {
		return LensReviewPacket{}, err
	}
	_, documents, err := retriever.prepareDocuments()
	if err != nil {
		return LensReviewPacket{}, err
	}
	indexDocuments := make([]IndexDocument, 0, len(documents))
	byID := make(map[string]evidenceDocument, len(documents))
	for _, document := range documents {
		indexDocuments = append(indexDocuments, IndexDocument{DocumentID: document.id, Text: document.searchText})
		byID[document.id] = document
	}
	projections := make([]LensProjection, 0, len(batch.Lenses))
	for _, lens := range batch.Lenses {
		request := SearchRequest{Query: lens.Query, Limit: limit}
		hits, err := retriever.backend.Rank(request, indexDocuments)
		if err != nil {
			return LensReviewPacket{}, err
		}
		method := strings.TrimSpace(retriever.backend.MethodID())
		if method == "" {
			return LensReviewPacket{}, errors.New("personal evidence retrieval backend has no method identity")
		}
		packet := assembleContextPacket(request, before, hits, byID, method)
		projections = append(projections, LensProjection{
			Lens: lens, LibraryFingerprint: before.Fingerprint,
			RetainedCount: len(before.Records), Matches: packet.Citations,
		})
	}
	after, err := retriever.repository.Load()
	if err != nil {
		return LensReviewPacket{}, err
	}
	return LensReviewPacket{
		SchemaVersion: LensReviewSchemaVersion, AuthorityClass: AuthorityClass,
		RetainedBefore: len(before.Records), RetainedAfter: len(after.Records),
		FingerprintBefore: before.Fingerprint, FingerprintAfter: after.Fingerprint,
		RetentionUnchanged: len(before.Records) == len(after.Records) && before.Fingerprint == after.Fingerprint,
		LensCount:          len(batch.Lenses), Projections: projections,
	}, nil
}

func (retriever ContextRetriever) prepareDocuments() (Library, []evidenceDocument, error) {
	library, err := retriever.repository.Load()
	if err != nil {
		return Library{}, nil, err
	}
	resourcesByID := make(map[string]ResourceContext, len(library.Resources))
	for _, resource := range library.Resources {
		resourcesByID[resource.ResourceID] = resource
	}
	resourceRevisions := groupResourceRevisions(library.ResourceRevisions)
	hydratedBytes := 0
	documents := make([]evidenceDocument, 0, len(library.Records))
	for _, record := range library.Records {
		document, err := retriever.prepareDocument(record.RecordID, record, "current", resourcesByID, resourceRevisions, &hydratedBytes)
		if err != nil {
			return Library{}, nil, err
		}
		documents = append(documents, document)
	}
	for _, revision := range library.Revisions {
		document, err := retriever.prepareDocument(revision.RevisionID, revision.Record, "superseded", resourcesByID, resourceRevisions, &hydratedBytes)
		if err != nil {
			return Library{}, nil, err
		}
		documents = append(documents, document)
	}
	return library, documents, nil
}

func (retriever ContextRetriever) prepareCompactProjection() (compactRetrievalProjection, error) {
	library, err := retriever.repository.Load()
	if err != nil {
		return compactRetrievalProjection{}, err
	}
	if len(library.Records) > MaximumRecords || len(library.Resources) > MaximumResources {
		return compactRetrievalProjection{}, errors.New("personal evidence library exceeds compact retrieval bounds")
	}
	resourcesByID := make(map[string]ResourceContext, len(library.Resources))
	for _, resource := range library.Resources {
		if strings.TrimSpace(resource.ResourceID) == "" {
			return compactRetrievalProjection{}, errors.New("personal evidence resource has no stable identity")
		}
		if _, exists := resourcesByID[resource.ResourceID]; exists {
			return compactRetrievalProjection{}, errors.New("personal evidence resource identity is duplicated")
		}
		resourcesByID[resource.ResourceID] = resource
	}
	projection := compactRetrievalProjection{
		library:              library,
		indexDocuments:       make([]IndexDocument, 0, len(library.Records)+len(library.Resources)),
		recordsByID:          make(map[string]CaptureRecord, len(library.Records)),
		resourcesByID:        resourcesByID,
		ownersByDocumentID:   make(map[string][]string, len(library.Records)+len(library.Resources)),
		resourceByDocumentID: make(map[string]string, len(library.Resources)),
		contentCache:         map[string]ExtractedContentArtifact{},
	}
	records := append([]CaptureRecord(nil), library.Records...)
	sort.Slice(records, func(i, j int) bool {
		return records[i].RecordID < records[j].RecordID
	})
	ownersByResourceID := map[string][]string{}
	ownerLinks := 0
	for _, record := range records {
		if err := validateCompactDocumentID(record.RecordID); err != nil {
			return compactRetrievalProjection{}, err
		}
		if _, exists := projection.recordsByID[record.RecordID]; exists {
			return compactRetrievalProjection{}, errors.New("personal evidence record identity is duplicated")
		}
		projection.recordsByID[record.RecordID] = record
		projection.ownersByDocumentID[record.RecordID] = []string{record.RecordID}
		resources := currentResourceBundleForRecord(record, resourcesByID)
		authorizationAliases := make([]string, 0, len(resources))
		for _, resource := range resources {
			authorizationAliases = append(authorizationAliases, resource.ResourceID)
		}
		authorizationAliases = uniqueSorted(authorizationAliases)
		if len(authorizationAliases) == 0 {
			authorizationAliases = []string{record.RecordID}
		}
		projection.indexDocuments = append(projection.indexDocuments, IndexDocument{
			DocumentID:                   record.RecordID,
			Text:                         strings.TrimSpace(record.RawText),
			FeedbackAliases:              []string{record.RecordID},
			AuthorizationEvidenceAliases: authorizationAliases,
			AuthorizationEvidenceKind:    IndexEvidenceKindRecordSource,
		})
		for _, resource := range resources {
			ownersByResourceID[resource.ResourceID] = append(
				ownersByResourceID[resource.ResourceID], record.RecordID,
			)
			ownerLinks++
			if ownerLinks > MaximumCompactOwnerLinks {
				return compactRetrievalProjection{}, errors.New("personal evidence compact owner map exceeds its execution budget")
			}
		}
	}
	resourceIDs := make([]string, 0, len(ownersByResourceID))
	for resourceID := range ownersByResourceID {
		resourceIDs = append(resourceIDs, resourceID)
	}
	sort.Strings(resourceIDs)
	for _, resourceID := range resourceIDs {
		resource := resourcesByID[resourceID]
		documentID, err := compactResourceDocumentID(resourceID)
		if err != nil {
			return compactRetrievalProjection{}, err
		}
		owners := uniqueSorted(ownersByResourceID[resourceID])
		if len(owners) == 0 {
			return compactRetrievalProjection{}, errors.New("personal evidence resource index document has no current owner")
		}
		if _, exists := projection.ownersByDocumentID[documentID]; exists {
			return compactRetrievalProjection{}, errors.New("personal evidence compact document identity is duplicated")
		}
		var content ExtractedContentArtifact
		if resource.Content == nil {
			content = ExtractedContentArtifact{}
		} else {
			content, err = retriever.loadCompactRetrievalContent(
				*resource.Content, &projection.hydratedBytes, projection.contentCache,
			)
			if err != nil {
				return compactRetrievalProjection{}, err
			}
		}
		projection.indexDocuments = append(projection.indexDocuments, IndexDocument{
			DocumentID:                   documentID,
			Text:                         compactResourceSearchText(resource, content),
			FeedbackAliases:              append([]string(nil), owners...),
			AuthorizationEvidenceAliases: []string{resourceID},
			AuthorizationEvidenceKind:    IndexEvidenceKindUniqueResource,
		})
		projection.ownersByDocumentID[documentID] = owners
		projection.resourceByDocumentID[documentID] = resourceID
	}
	if len(projection.indexDocuments) > MaximumCompactIndexDocuments {
		return compactRetrievalProjection{}, errors.New("personal evidence compact index exceeds its document budget")
	}
	return projection, nil
}

func validateCompactRawHitIdentities(
	rawHits []RankedHit,
	projection compactRetrievalProjection,
) error {
	for _, hit := range rawHits {
		owners, exists := projection.ownersByDocumentID[hit.DocumentID]
		if !exists {
			return errors.New("personal evidence backend returned an unknown compact document identity")
		}
		if len(owners) == 0 {
			return errors.New("personal evidence backend returned an ownerless compact document identity")
		}
		if resourceID := projection.resourceByDocumentID[hit.DocumentID]; resourceID == "" {
			if len(owners) != 1 || owners[0] != hit.DocumentID {
				return errors.New("personal evidence record document does not map to itself")
			}
		}
	}
	return nil
}

func expandCompactHits(
	rawHits []RankedHit,
	projection compactRetrievalProjection,
	limit int,
) ([]RankedHit, map[string]string, error) {
	hits := make([]RankedHit, 0, min(limit, len(rawHits)))
	selectedResources := map[string]string{}
	seenRecords := map[string]bool{}
	for _, rawHit := range rawHits {
		owners, exists := projection.ownersByDocumentID[rawHit.DocumentID]
		if !exists {
			continue
		}
		resourceID := projection.resourceByDocumentID[rawHit.DocumentID]
		if resourceID != "" && len(owners) == 0 {
			return nil, nil, errors.New("personal evidence resource hit has no current owner")
		}
		for _, recordID := range owners {
			if seenRecords[recordID] {
				continue
			}
			if _, exists := projection.recordsByID[recordID]; !exists {
				return nil, nil, errors.New("personal evidence resource owner is not a current record")
			}
			seenRecords[recordID] = true
			hit := rawHit
			hit.DocumentID = recordID
			hit.MatchedTerms = append([]string(nil), rawHit.MatchedTerms...)
			hit.Components = copyScores(rawHit.Components)
			hits = append(hits, hit)
			if resourceID != "" {
				selectedResources[recordID] = resourceID
			}
			if len(hits) == limit {
				return hits, selectedResources, nil
			}
		}
	}
	return hits, selectedResources, nil
}

func (retriever ContextRetriever) hydrateCompactRecords(
	hits []RankedHit,
	selectedResources map[string]string,
	projection *compactRetrievalProjection,
) (map[string]evidenceDocument, error) {
	documents := make(map[string]evidenceDocument, len(hits))
	for _, hit := range hits {
		record, exists := projection.recordsByID[hit.DocumentID]
		if !exists {
			return nil, errors.New("personal evidence compact hit is not a current record")
		}
		document, err := retriever.prepareCompactRecordDocument(
			record, projection.resourcesByID, &projection.hydratedBytes, projection.contentCache,
		)
		if err != nil {
			return nil, err
		}
		if resourceID := selectedResources[record.RecordID]; resourceID != "" {
			document.resources, err = resourceFirst(document.resources, resourceID)
			if err != nil {
				return nil, err
			}
		}
		documents[record.RecordID] = document
	}
	return documents, nil
}

func (retriever ContextRetriever) prepareCompactRecordDocument(
	record CaptureRecord,
	resourcesByID map[string]ResourceContext,
	hydratedBytes *int,
	contentCache map[string]ExtractedContentArtifact,
) (evidenceDocument, error) {
	resources := currentResourceBundleForRecord(record, resourcesByID)
	contents := map[string]ExtractedContentArtifact{}
	for _, resource := range resources {
		if resource.Content == nil {
			continue
		}
		content, err := retriever.loadCompactRetrievalContent(
			*resource.Content, hydratedBytes, contentCache,
		)
		if err != nil {
			return evidenceDocument{}, err
		}
		contents[resource.ResourceID] = content
	}
	return evidenceDocument{
		id: record.RecordID, record: record, versionState: "current", resources: resources,
		contents: contents, revisionContents: map[string]ExtractedContentArtifact{},
	}, nil
}

func (retriever ContextRetriever) loadCompactRetrievalContent(
	reference ContentArtifactRef,
	hydratedBytes *int,
	cache map[string]ExtractedContentArtifact,
) (ExtractedContentArtifact, error) {
	key := reference.ArtifactID + "\x00" + reference.SHA256
	if content, exists := cache[key]; exists {
		return content, nil
	}
	content, err := retriever.loadRetrievalContent(reference, hydratedBytes)
	if err != nil {
		return ExtractedContentArtifact{}, err
	}
	cache[key] = content
	return content, nil
}

func (retriever ContextRetriever) prepareDocument(id string, record CaptureRecord, state string, resourcesByID map[string]ResourceContext, revisionsByResourceID map[string][]ResourceRevision, hydratedBytes *int) (evidenceDocument, error) {
	resources, resourceRevisions := resourceBundleForRecord(record, resourcesByID, revisionsByResourceID)
	contents := map[string]ExtractedContentArtifact{}
	revisionContents := map[string]ExtractedContentArtifact{}
	for _, resource := range resources {
		if resource.Content == nil {
			continue
		}
		content, err := retriever.loadRetrievalContent(*resource.Content, hydratedBytes)
		if err != nil {
			return evidenceDocument{}, err
		}
		contents[resource.ResourceID] = content
	}
	for _, revision := range resourceRevisions {
		if revision.Resource.Content == nil {
			continue
		}
		content, err := retriever.loadRetrievalContent(*revision.Resource.Content, hydratedBytes)
		if err != nil {
			return evidenceDocument{}, err
		}
		revisionContents[revision.RevisionID] = content
	}
	return evidenceDocument{
		id: id, record: record, versionState: state, resources: resources,
		resourceRevisions: resourceRevisions, contents: contents, revisionContents: revisionContents,
		searchText: searchableText(record, resources, resourceRevisions, contents, revisionContents),
	}, nil
}

func (retriever ContextRetriever) loadRetrievalContent(
	reference ContentArtifactRef,
	hydratedBytes *int,
) (ExtractedContentArtifact, error) {
	if reference.ByteLength > MaximumRetrievalContentBytes-*hydratedBytes {
		return ExtractedContentArtifact{}, errors.New("personal evidence retrieval exceeds its hydration budget")
	}
	content, err := retriever.repository.LoadContent(reference)
	if err != nil {
		return ExtractedContentArtifact{}, err
	}
	*hydratedBytes += content.Reference.ByteLength
	return content, nil
}

func findCurrentRecord(records []CaptureRecord, recordID string) (CaptureRecord, bool) {
	for _, record := range records {
		if record.RecordID == recordID {
			return record, true
		}
	}
	return CaptureRecord{}, false
}

func findRevision(revisions []CaptureRevision, revisionID string) (CaptureRevision, bool) {
	for _, revision := range revisions {
		if revision.RevisionID == revisionID {
			return revision, true
		}
	}
	return CaptureRevision{}, false
}

func (LexicalBM25Backend) Rank(request SearchRequest, documents []IndexDocument) ([]RankedHit, error) {
	rankingTerms := uniqueSorted(tokenize(request.Query))
	authorizationTerms := meaningfulQueryTerms(request.Query)
	if strings.TrimSpace(request.LexicalQuery) != "" {
		authorizationTerms = uniqueSorted(tokenize(request.LexicalQuery))
	}
	orderedAuthorizationTerms := uniqueInOrder(meaningfulQueryTermsInOrder(request.Query))
	if len(rankingTerms) == 0 {
		return nil, errors.New("search query is empty")
	}
	limit := request.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaximumSearchLimit {
		return nil, errors.New("search limit exceeds 100")
	}
	type prepared struct {
		document IndexDocument
		terms    []string
		counts   map[string]int
	}
	preparedDocuments := make([]prepared, 0, len(documents))
	documentFrequency := map[string]int{}
	frequencyTerms := uniqueSorted(append(
		append([]string(nil), rankingTerms...), authorizationTerms...,
	))
	totalLength := 0
	for _, document := range documents {
		terms := tokenize(document.Text)
		counts := termCounts(terms)
		preparedDocuments = append(preparedDocuments, prepared{document: document, terms: terms, counts: counts})
		totalLength += len(terms)
		for _, term := range frequencyTerms {
			if counts[term] > 0 {
				documentFrequency[term]++
			}
		}
	}
	averageLength := 1.0
	if len(preparedDocuments) > 0 {
		averageLength = float64(totalLength) / float64(len(preparedDocuments))
	}
	hits := []RankedHit{}
	for _, document := range preparedDocuments {
		score := 0.0
		matchedAuthorizationTerms := []string{}
		rarestDocumentRatio := 1.0
		for _, term := range rankingTerms {
			tf := document.counts[term]
			if tf == 0 {
				continue
			}
			idf := math.Log(1 + (float64(len(preparedDocuments)-documentFrequency[term])+0.5)/(float64(documentFrequency[term])+0.5))
			numerator := float64(tf) * 2.2
			denominator := float64(tf) + 1.2*(0.25+0.75*float64(len(document.terms))/averageLength)
			score += idf * numerator / denominator
		}
		if score == 0 {
			continue
		}
		totalAuthorizationIDF := 0.0
		matchedAuthorizationIDF := 0.0
		for _, term := range authorizationTerms {
			df := documentFrequency[term]
			idf := math.Log(1 + (float64(len(preparedDocuments)-df)+0.5)/(float64(df)+0.5))
			totalAuthorizationIDF += idf
			if document.counts[term] == 0 {
				continue
			}
			matchedAuthorizationTerms = append(matchedAuthorizationTerms, term)
			matchedAuthorizationIDF += idf
			ratio := float64(df) / float64(len(preparedDocuments))
			if ratio < rarestDocumentRatio {
				rarestDocumentRatio = ratio
			}
		}
		idfCoverage := 0.0
		if totalAuthorizationIDF > 0 {
			idfCoverage = matchedAuthorizationIDF / totalAuthorizationIDF
		}
		exactPhrase := len(orderedAuthorizationTerms) >= 2 &&
			containsTermSequence(document.terms, orderedAuthorizationTerms)
		hits = append(hits, RankedHit{
			DocumentID: document.document.DocumentID, Score: score,
			MatchedTerms: uniqueSorted(matchedAuthorizationTerms),
			Components: map[string]float64{
				"lexical_raw":                    score,
				"lexical_query_terms":            float64(len(authorizationTerms)),
				"lexical_matched_terms":          float64(len(matchedAuthorizationTerms)),
				"lexical_idf_coverage":           idfCoverage,
				"lexical_rarest_document_ratio":  rarestDocumentRatio,
				"lexical_exact_ordered_phrase":   boolScore(exactPhrase),
				"lexical_winner_relative_margin": 0,
			},
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].DocumentID < hits[j].DocumentID
		}
		return hits[i].Score > hits[j].Score
	})
	winnerMargin := 1.0
	if len(hits) > 1 && hits[0].Score > 0 {
		winnerMargin = (hits[0].Score - hits[1].Score) / hits[0].Score
		if winnerMargin < 0 {
			winnerMargin = 0
		}
	}
	for index := range hits {
		hits[index].Components["lexical_rank"] = float64(index + 1)
		hits[index].Components["lexical_winner_relative_margin"] = winnerMargin
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func assembleContextPacket(request SearchRequest, library Library, hits []RankedHit, documents map[string]evidenceDocument, retrievalMethod string) ContextPacket {
	packet := ContextPacket{
		SchemaVersion: ContextPacketSchemaVersion, RunID: request.RunID,
		Query: strings.TrimSpace(request.Query), LensID: request.LensID,
		RetrievalMethod: retrievalMethod, AuthorityClass: AuthorityClass,
		LibraryRevision: library.Revision, LibraryFingerprint: library.Fingerprint,
		Citations: []Citation{}, Records: []CaptureRecord{}, Resources: []ResourceContext{},
		ResourceRevisions: []ResourceRevision{},
	}
	includedResources := map[string]bool{}
	includedResourceRevisions := map[string]bool{}
	for _, hit := range hits {
		document, exists := documents[hit.DocumentID]
		if !exists {
			continue
		}
		author := document.record.AuthorName
		if author == "" {
			author = document.record.AuthorID
		}
		evidenceRefs := evidenceReferences(document, hit.MatchedTerms)
		sourceSnippet := snippet(document.record.RawText, 500)
		if len(evidenceRefs) > 0 && strings.TrimSpace(evidenceRefs[0].MatchedSnippet) != "" {
			sourceSnippet = evidenceRefs[0].MatchedSnippet
		}
		packet.Citations = append(packet.Citations, Citation{
			RecordID: hit.DocumentID, LogicalRecordID: document.record.RecordID,
			VersionState: document.versionState, SourceRef: document.record.SourceRef,
			OccurredAt: document.record.OccurredAt, Author: author, Snippet: sourceSnippet,
			MatchedTerms: hit.MatchedTerms, Score: hit.Score, ContentHash: document.record.ContentHash,
			ComponentScores: copyScores(hit.Components),
			ContextState:    document.record.ContextState, Missingness: citationMissingness(document),
			ResourceIDs: append([]string(nil), document.record.ResourceIDs...), EvidenceRefs: evidenceRefs,
			AuthorityClass: AuthorityClass,
		})
		packet.Records = append(packet.Records, document.record)
		for _, resource := range document.resources {
			if includedResources[resource.ResourceID] {
				continue
			}
			includedResources[resource.ResourceID] = true
			packet.Resources = append(packet.Resources, resource)
		}
		for _, revision := range document.resourceRevisions {
			if includedResourceRevisions[revision.RevisionID] {
				continue
			}
			includedResourceRevisions[revision.RevisionID] = true
			packet.ResourceRevisions = append(packet.ResourceRevisions, revision)
		}
	}
	return packet
}

func assembleCompactContextPacket(
	request SearchRequest,
	library Library,
	hits []RankedHit,
	documents map[string]evidenceDocument,
	retrievalMethod string,
	policy CompactAbstentionPolicy,
) CompactContextPacket {
	packet := CompactContextPacket{
		SchemaVersion: CompactPacketSchemaVersion, RunID: request.RunID,
		Query: strings.TrimSpace(request.Query), LensID: request.LensID,
		RetrievalMethod: retrievalMethod, AuthorityClass: AuthorityClass,
		AbstentionPolicyFingerprint: policy.Fingerprint,
		LibraryRevision:             library.Revision, LibraryFingerprint: library.Fingerprint,
		AnswerState: "abstained", AbstentionReason: "no_retrieval_candidates",
		Citations: []CompactCitation{},
	}
	for _, hit := range hits {
		document, exists := documents[hit.DocumentID]
		if !exists {
			continue
		}
		packet.Citations = append(packet.Citations, compactCitation(document, hit))
	}
	if len(packet.Citations) > 0 {
		packet.AnswerState = "answered"
		packet.AbstentionReason = ""
	}
	return packet
}

func compactCitation(document evidenceDocument, hit RankedHit) CompactCitation {
	author := document.record.AuthorName
	if author == "" {
		author = document.record.AuthorID
	}
	references := evidenceReferences(document, hit.MatchedTerms)
	compactReferences := make([]CompactEvidenceReference, 0, len(references))
	for _, reference := range references {
		compactReferences = append(compactReferences, CompactEvidenceReference{
			ResourceID: reference.ResourceID, ResourceHash: reference.ResourceHash,
			ResourceVersionState: reference.ResourceVersionState,
			ResourceRevisionID:   reference.ResourceRevisionID,
			ExcerptID:            reference.ExcerptID, ArtifactID: reference.ArtifactID,
			Locator: reference.Locator,
			MatchedSnippet: boundedCompactSnippet(
				reference.MatchedSnippet, MaximumCompactSnippetRunes,
			),
		})
	}
	states := make([]ResourceStateSummary, 0, len(document.resources))
	for _, resource := range document.resources {
		if len(states) == MaximumCompactResourceStates {
			break
		}
		states = append(states, ResourceStateSummary{
			ResourceID: resource.ResourceID, State: resource.State,
			AccessClass: resource.AccessClass, ContentHash: resource.ContentHash,
			Missingness:    append([]string(nil), resource.Missingness...),
			AuthorityClass: resource.AuthorityClass,
		})
	}
	sourceSnippet := boundedCompactSnippet(document.record.RawText, MaximumCompactSnippetRunes)
	if len(compactReferences) > 0 &&
		strings.TrimSpace(compactReferences[0].MatchedSnippet) != "" {
		sourceSnippet = compactReferences[0].MatchedSnippet
	}
	return CompactCitation{
		RecordID: hit.DocumentID, LogicalRecordID: document.record.RecordID,
		VersionState: document.versionState, SourceRef: document.record.SourceRef,
		OccurredAt: document.record.OccurredAt, Author: author, Snippet: sourceSnippet,
		MatchedTerms: append([]string(nil), hit.MatchedTerms...), Score: hit.Score,
		ComponentScores: copyScores(hit.Components), ContentHash: document.record.ContentHash,
		ContextState: document.record.ContextState, Missingness: citationMissingness(document),
		EvidenceRefs: compactReferences, ResourceStates: states,
		ResourceStatesTruncated: len(document.resources) > len(states),
		AuthorityClass:          AuthorityClass,
	}
}

func boundedCompactSnippet(value string, maximum int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if maximum <= 0 {
		return ""
	}
	if len(runes) <= maximum {
		return value
	}
	if maximum == 1 {
		return "…"
	}
	return strings.TrimSpace(string(runes[:maximum-1])) + "…"
}

// DefaultCompactAbstentionPolicy binds every ranking and authorization
// assumption used by compact retrieval. Any change requires a new version.
func DefaultCompactAbstentionPolicy() CompactAbstentionPolicy {
	identity := strings.Join([]string{
		CompactAbstentionPolicySchemaVersion,
		"minimum_semantic_cosine=" + strconv.FormatFloat(
			DefaultCompactMinimumSemanticCosine, 'f', 6, 64,
		),
		"minimum_semantic_margin=" + strconv.FormatFloat(
			DefaultCompactMinimumSemanticMargin, 'f', 6, 64,
		),
		"minimum_semantic_only_cosine=" + strconv.FormatFloat(
			DefaultCompactMinimumSemanticOnlyCosine, 'f', 6, 64,
		),
		"minimum_semantic_only_margin=" + strconv.FormatFloat(
			DefaultCompactMinimumSemanticOnlyMargin, 'f', 6, 64,
		),
		"minimum_semantic_lexical_coverage=" + strconv.FormatFloat(
			DefaultCompactMinimumSemanticLexicalCover, 'f', 6, 64,
		),
		"minimum_lexical_idf_coverage=" + strconv.FormatFloat(
			DefaultCompactMinimumLexicalIDFCoverage, 'f', 6, 64,
		),
		"maximum_lexical_document_ratio=" + strconv.FormatFloat(
			DefaultCompactMaximumLexicalDocumentRatio, 'f', 6, 64,
		),
		"minimum_lexical_winner_margin=" + strconv.FormatFloat(
			DefaultCompactMinimumLexicalWinnerMargin, 'f', 6, 64,
		),
		"minimum_lexical_matched_terms=" + strconv.Itoa(
			DefaultCompactMinimumLexicalMatchedTerms,
		),
		"minimum_ordered_phrase_terms=" + strconv.Itoa(
			DefaultCompactMinimumOrderedPhraseTerms,
		),
		"lexical_evidence_rule=" + compactLexicalEvidenceRule,
		"stopword_policy=" + compactStopwordPolicy,
		"semantic_calibration_identity=" + CompactSemanticCalibrationIdentity,
		"ranking_identity=" + compactRankingIdentity,
		"chunking_identity=" + compactChunkingIdentity,
	}, "\n")
	sum := sha256.Sum256([]byte(identity))
	return CompactAbstentionPolicy{
		SchemaVersion:                  CompactAbstentionPolicySchemaVersion,
		MinimumSemanticCosine:          DefaultCompactMinimumSemanticCosine,
		MinimumSemanticMargin:          DefaultCompactMinimumSemanticMargin,
		MinimumSemanticOnlyCosine:      DefaultCompactMinimumSemanticOnlyCosine,
		MinimumSemanticOnlyMargin:      DefaultCompactMinimumSemanticOnlyMargin,
		MinimumSemanticLexicalCoverage: DefaultCompactMinimumSemanticLexicalCover,
		MinimumLexicalIDFCoverage:      DefaultCompactMinimumLexicalIDFCoverage,
		MaximumLexicalDocumentRatio:    DefaultCompactMaximumLexicalDocumentRatio,
		MinimumLexicalWinnerMargin:     DefaultCompactMinimumLexicalWinnerMargin,
		MinimumLexicalMatchedTerms:     DefaultCompactMinimumLexicalMatchedTerms,
		MinimumOrderedPhraseTerms:      DefaultCompactMinimumOrderedPhraseTerms,
		LexicalEvidenceRule:            compactLexicalEvidenceRule,
		StopwordPolicy:                 compactStopwordPolicy,
		SemanticCalibrationIdentity:    CompactSemanticCalibrationIdentity,
		RankingIdentity:                compactRankingIdentity,
		ChunkingIdentity:               compactChunkingIdentity,
		Fingerprint:                    hex.EncodeToString(sum[:]),
	}
}

func usableCompactHits(
	hits []RankedHit,
	policy CompactAbstentionPolicy,
	calibrationID string,
	projection compactRetrievalProjection,
) []RankedHit {
	valid := make([]RankedHit, 0, len(hits))
	seen := map[string]bool{}
	for _, hit := range hits {
		if strings.TrimSpace(hit.DocumentID) == "" || seen[hit.DocumentID] ||
			math.IsNaN(hit.Score) || math.IsInf(hit.Score, 0) || hit.Score <= 0 {
			continue
		}
		seen[hit.DocumentID] = true
		valid = append(valid, hit)
	}
	if !compactQueryAuthorized(valid, policy, calibrationID) &&
		!compactCorroboratedResourceAuthorized(
			valid, policy, calibrationID, projection,
		) {
		return nil
	}
	return valid
}

func compactCorroboratedResourceAuthorized(
	hits []RankedHit,
	policy CompactAbstentionPolicy,
	calibrationID string,
	projection compactRetrievalProjection,
) bool {
	if calibrationID != policy.SemanticCalibrationIdentity {
		return false
	}
	var winner RankedHit
	winnerCount := 0
	for _, hit := range hits {
		rank, ok := finiteComponent(hit, "semantic_rank")
		if !ok || rank != 1 {
			continue
		}
		winner = hit
		winnerCount++
	}
	if winnerCount != 1 {
		return false
	}
	resourceID, isResource := projection.resourceByDocumentID[winner.DocumentID]
	if !isResource || strings.TrimSpace(resourceID) == "" {
		return false
	}
	indexDocument, exists := compactIndexDocumentByID(
		projection.indexDocuments, winner.DocumentID,
	)
	if !exists ||
		indexDocument.AuthorizationEvidenceKind != IndexEvidenceKindUniqueResource ||
		len(indexDocument.AuthorizationEvidenceAliases) != 1 ||
		indexDocument.AuthorizationEvidenceAliases[0] != resourceID {
		return false
	}
	cosine, cosineOK := finiteComponent(winner, "semantic_cosine")
	margin, marginOK := finiteComponent(
		winner, "semantic_distinct_evidence_margin",
	)
	coverage, coverageOK := finiteComponent(winner, "lexical_idf_coverage")
	valid, validOK := finiteComponent(
		winner, "semantic_distinct_evidence_valid",
	)
	return cosineOK && marginOK && coverageOK && validOK && valid == 1 &&
		cosine >= policy.MinimumSemanticCosine &&
		margin >= policy.MinimumSemanticMargin &&
		coverage >= policy.MinimumSemanticLexicalCoverage
}

func compactIndexDocumentByID(
	documents []IndexDocument,
	documentID string,
) (IndexDocument, bool) {
	var matched IndexDocument
	count := 0
	for _, document := range documents {
		if document.DocumentID != documentID {
			continue
		}
		matched = document
		count++
	}
	return matched, count == 1
}

func compactQueryAuthorized(
	hits []RankedHit,
	policy CompactAbstentionPolicy,
	calibrationID string,
) bool {
	for _, hit := range hits {
		rank, rankOK := finiteComponent(hit, "lexical_rank")
		if !rankOK || rank != 1 {
			continue
		}
		queryTerms, queryOK := finiteComponent(hit, "lexical_query_terms")
		matchedTerms, matchedOK := finiteComponent(hit, "lexical_matched_terms")
		idfCoverage, coverageOK := finiteComponent(hit, "lexical_idf_coverage")
		rarestRatio, rarityOK := finiteComponent(hit, "lexical_rarest_document_ratio")
		winnerMargin, marginOK := finiteComponent(hit, "lexical_winner_relative_margin")
		exactPhrase, phraseOK := finiteComponent(hit, "lexical_exact_ordered_phrase")
		if !queryOK || !matchedOK || !coverageOK || !rarityOK || !marginOK ||
			!phraseOK || queryTerms < 2 {
			break
		}
		if exactPhrase >= 1 &&
			(queryTerms >= float64(policy.MinimumOrderedPhraseTerms) ||
				(queryTerms >= 2 && rarestRatio <= policy.MaximumLexicalDocumentRatio)) {
			return true
		}
		if matchedTerms >= float64(policy.MinimumLexicalMatchedTerms) &&
			idfCoverage >= policy.MinimumLexicalIDFCoverage &&
			rarestRatio <= policy.MaximumLexicalDocumentRatio &&
			winnerMargin >= policy.MinimumLexicalWinnerMargin {
			return true
		}
		break
	}
	if calibrationID != policy.SemanticCalibrationIdentity {
		return false
	}
	for _, hit := range hits {
		semanticRank, rankOK := finiteComponent(hit, "semantic_rank")
		cosine, cosineOK := finiteComponent(hit, "semantic_cosine")
		margin, marginOK := finiteComponent(hit, "semantic_margin")
		if !rankOK || !cosineOK || !marginOK || semanticRank != 1 {
			continue
		}
		if cosine >= policy.MinimumSemanticOnlyCosine &&
			margin >= policy.MinimumSemanticOnlyMargin {
			return true
		}
		lexicalCoverage, lexicalOK := finiteComponent(hit, "lexical_idf_coverage")
		return lexicalOK &&
			cosine >= policy.MinimumSemanticCosine &&
			margin >= policy.MinimumSemanticMargin &&
			lexicalCoverage >= policy.MinimumSemanticLexicalCoverage
	}
	return false
}

func finiteComponent(hit RankedHit, name string) (float64, bool) {
	value, exists := hit.Components[name]
	return value, exists && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func meaningfulQueryTerms(query string) []string {
	terms := uniqueSorted(meaningfulQueryTermsInOrder(query))
	return terms
}

func meaningfulQueryTermsInOrder(query string) []string {
	stopwords := map[string]bool{
		"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
		"be": true, "by": true, "do": true, "for": true, "from": true, "how": true,
		"i": true, "in": true, "is": true, "it": true, "me": true, "my": true,
		"of": true, "on": true, "or": true, "that": true, "the": true, "this": true,
		"to": true, "was": true, "were": true, "what": true, "when": true,
		"where": true, "which": true, "who": true, "why": true, "with": true,
		"can": true, "could": true, "get": true, "gets": true, "got": true,
		"idea": true, "ideas": true, "know": true, "make": true, "makes": true,
		"should": true, "thing": true, "things": true, "use": true, "used": true,
		"using": true, "way": true, "ways": true, "work": true, "works": true,
		"working": true, "would": true,
	}
	terms := tokenize(query)
	filtered := make([]string, 0, len(terms))
	for _, term := range terms {
		if stopwords[term] || len([]rune(term)) < 2 {
			continue
		}
		filtered = append(filtered, term)
	}
	return filtered
}

func uniqueInOrder(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func containsTermSequence(documentTerms, queryTerms []string) bool {
	if len(queryTerms) == 0 || len(queryTerms) > len(documentTerms) {
		return false
	}
	for start := 0; start+len(queryTerms) <= len(documentTerms); start++ {
		matched := true
		for offset := range queryTerms {
			if documentTerms[start+offset] != queryTerms[offset] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func containsAnyAdjacentPair(documentTerms, queryTerms []string) bool {
	for index := 0; index+1 < len(queryTerms); index++ {
		if containsTermSequence(documentTerms, queryTerms[index:index+2]) {
			return true
		}
	}
	return false
}

func boolScore(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func copyScores(scores map[string]float64) map[string]float64 {
	if len(scores) == 0 {
		return nil
	}
	copy := make(map[string]float64, len(scores))
	for name, value := range scores {
		copy[name] = value
	}
	return copy
}

func evidenceReferences(document evidenceDocument, matchedTerms []string) []EvidenceReference {
	references := []EvidenceReference{}
	if len(matchedTerms) == 0 {
		for _, resource := range document.resources {
			content, _ := document.contents[resource.ResourceID]
			references = append(references, semanticResourceReference(resource, content, "current", ""))
			if len(references) == MaximumCitationEvidenceRefs {
				return references
			}
		}
		for _, revision := range document.resourceRevisions {
			content, _ := document.revisionContents[revision.RevisionID]
			references = append(references, semanticResourceReference(
				revision.Resource, content, "superseded", revision.RevisionID,
			))
			if len(references) == MaximumCitationEvidenceRefs {
				return references
			}
		}
		return references
	}
	for _, resource := range document.resources {
		content, _ := document.contents[resource.ResourceID]
		references = append(references, referencesForResource(resource, content, matchedTerms, "current", "")...)
		if len(references) >= MaximumCitationEvidenceRefs {
			return references[:MaximumCitationEvidenceRefs]
		}
	}
	for _, revision := range document.resourceRevisions {
		content, _ := document.revisionContents[revision.RevisionID]
		references = append(references, referencesForResource(
			revision.Resource, content, matchedTerms, "superseded", revision.RevisionID,
		)...)
		if len(references) >= MaximumCitationEvidenceRefs {
			return references[:MaximumCitationEvidenceRefs]
		}
	}
	return references
}

func referencesForResource(resource ResourceContext, content ExtractedContentArtifact, matchedTerms []string, versionState, revisionID string) []EvidenceReference {
	references := []EvidenceReference{}
	base := func() EvidenceReference {
		return EvidenceReference{
			ResourceID: resource.ResourceID, CanonicalURL: resource.CanonicalURL,
			ResourceHash: resource.ContentHash, ResourceVersionState: versionState,
			ResourceRevisionID: revisionID,
		}
	}
	for _, excerpt := range resource.Excerpts {
		if GenericExtractorReferenceExcerpt(excerpt) {
			continue
		}
		if !containsAnyTerm(excerpt.Text+" "+excerpt.Locator, matchedTerms) {
			continue
		}
		reference := base()
		reference.ExcerptID = excerpt.ExcerptID
		reference.Locator = excerpt.Locator
		reference.MatchedSnippet = snippet(excerpt.Text, 500)
		references = append(references, reference)
	}
	if content.Reference.ArtifactID != "" && containsAnyTerm(content.Text, matchedTerms) {
		reference := base()
		reference.ArtifactID = content.Reference.ArtifactID
		reference.Locator = "extracted_content"
		reference.MatchedSnippet = matchedWindow(content.Text, matchedTerms, 500)
		references = append(references, reference)
	}
	metadata := strings.TrimSpace(strings.Join([]string{
		resource.Metadata.Title, resource.Metadata.Author, resource.Metadata.PublishedAt,
	}, " — "))
	if strings.TrimSpace(metadata) != "" && containsAnyTerm(metadata, matchedTerms) {
		reference := base()
		reference.Locator = "public_metadata"
		reference.MatchedSnippet = snippet(metadata, 500)
		references = append(references, reference)
	}
	missingness := strings.Join(resource.Missingness, ", ")
	if strings.TrimSpace(missingness) != "" && containsAnyTerm(missingness, matchedTerms) {
		reference := base()
		reference.Locator = "resource_missingness"
		reference.MatchedSnippet = snippet(missingness, 500)
		references = append(references, reference)
	}
	return references
}

func semanticResourceReference(resource ResourceContext, content ExtractedContentArtifact, versionState, revisionID string) EvidenceReference {
	reference := EvidenceReference{
		ResourceID: resource.ResourceID, CanonicalURL: resource.CanonicalURL,
		ResourceHash: resource.ContentHash, ResourceVersionState: versionState,
		ResourceRevisionID: revisionID, Locator: "semantic_record_match",
	}
	if content.Reference.ArtifactID != "" {
		reference.ArtifactID = content.Reference.ArtifactID
	}
	return reference
}

func citationMissingness(document evidenceDocument) []string {
	values := append([]string(nil), document.record.Missingness...)
	for _, resource := range document.resources {
		values = append(values, resource.Missingness...)
	}
	for _, revision := range document.resourceRevisions {
		values = append(values, revision.Resource.Missingness...)
	}
	return uniqueSorted(values)
}

func searchableText(record CaptureRecord, resources []ResourceContext, revisions []ResourceRevision, contents map[string]ExtractedContentArtifact, revisionContents map[string]ExtractedContentArtifact) string {
	parts := []string{
		record.RawText, strings.Join(record.URLs, " "), record.AuthorName,
		record.AuthorID, record.SourceRef, strings.Join(record.Missingness, " "),
	}
	for _, resource := range resources {
		parts = append(parts, searchableResourceText(resource, contents[resource.ResourceID])...)
	}
	for _, revision := range revisions {
		parts = append(parts, searchableResourceText(revision.Resource, revisionContents[revision.RevisionID])...)
	}
	return strings.Join(parts, "\n")
}

func searchableResourceText(resource ResourceContext, content ExtractedContentArtifact) []string {
	parts := []string{
		resource.Metadata.Title, resource.Metadata.Author,
		resource.Metadata.PublishedAt, strings.Join(resource.Missingness, " "),
	}
	for _, excerpt := range resource.Excerpts {
		if GenericExtractorReferenceExcerpt(excerpt) {
			continue
		}
		parts = append(parts, excerpt.Text, excerpt.Locator)
	}
	if content.Reference.ArtifactID != "" {
		parts = append(parts, content.Text)
	}
	return parts
}

func compactResourceSearchText(resource ResourceContext, content ExtractedContentArtifact) string {
	parts := []string{
		strings.TrimSpace(resource.Metadata.Title),
		strings.TrimSpace(resource.Metadata.Author),
		strings.TrimSpace(resource.Metadata.PublishedAt),
	}
	for _, excerpt := range resource.Excerpts {
		if GenericExtractorReferenceExcerpt(excerpt) {
			continue
		}
		parts = append(parts, strings.TrimSpace(excerpt.Text), strings.TrimSpace(excerpt.Locator))
	}
	if content.Reference.ArtifactID != "" {
		parts = append(parts, strings.TrimSpace(content.Text))
	}
	nonempty := parts[:0]
	for _, part := range parts {
		if part != "" {
			nonempty = append(nonempty, part)
		}
	}
	return strings.Join(nonempty, "\n")
}

func compactResourceDocumentID(resourceID string) (string, error) {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return "", errors.New("personal evidence resource has no stable identity")
	}
	documentID := compactResourceDocumentPrefix + resourceID
	if err := validateCompactDocumentID(documentID); err != nil {
		return "", err
	}
	return documentID, nil
}

func validateCompactDocumentID(documentID string) error {
	if strings.TrimSpace(documentID) == "" {
		return errors.New("personal evidence compact document has no stable identity")
	}
	if len([]rune(documentID)) > maximumCompactDocumentIDRunes {
		return errors.New("personal evidence compact document identity exceeds its execution budget")
	}
	return nil
}

func currentResourceBundleForRecord(
	record CaptureRecord,
	resourcesByID map[string]ResourceContext,
) []ResourceContext {
	resources := []ResourceContext{}
	seen := map[string]bool{}
	queue := uniqueSorted(record.ResourceIDs)
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
		resources = append(resources, resource)
		relatedIDs := []string{}
		for _, related := range resource.RelatedURLs {
			if FollowableRelatedResource(related) {
				relatedIDs = append(relatedIDs, stableResourceID(related.URL))
			}
		}
		queue = append(queue, uniqueSorted(relatedIDs)...)
	}
	return resources
}

func resourceFirst(resources []ResourceContext, resourceID string) ([]ResourceContext, error) {
	index := -1
	for position, resource := range resources {
		if resource.ResourceID == resourceID {
			index = position
			break
		}
	}
	if index < 0 {
		return nil, errors.New("personal evidence resource hit is not reachable from its current owner")
	}
	if index == 0 {
		return resources, nil
	}
	ordered := make([]ResourceContext, 0, len(resources))
	ordered = append(ordered, resources[index])
	ordered = append(ordered, resources[:index]...)
	ordered = append(ordered, resources[index+1:]...)
	return ordered, nil
}

func resourceBundleForRecord(record CaptureRecord, resourcesByID map[string]ResourceContext, revisionsByResourceID map[string][]ResourceRevision) ([]ResourceContext, []ResourceRevision) {
	resources := []ResourceContext{}
	revisions := []ResourceRevision{}
	seen := map[string]bool{}
	seenRevisions := map[string]bool{}
	queue := append([]string(nil), record.ResourceIDs...)
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
		resources = append(resources, resource)
		for _, related := range resource.RelatedURLs {
			if FollowableRelatedResource(related) {
				queue = append(queue, stableResourceID(related.URL))
			}
		}
		for _, revision := range revisionsByResourceID[resourceID] {
			if seenRevisions[revision.RevisionID] {
				continue
			}
			seenRevisions[revision.RevisionID] = true
			revisions = append(revisions, revision)
			for _, related := range revision.Resource.RelatedURLs {
				if FollowableRelatedResource(related) {
					queue = append(queue, stableResourceID(related.URL))
				}
			}
		}
	}
	return resources, revisions
}

func groupResourceRevisions(revisions []ResourceRevision) map[string][]ResourceRevision {
	grouped := map[string][]ResourceRevision{}
	for _, revision := range revisions {
		resourceID := revision.Resource.ResourceID
		grouped[resourceID] = append(grouped[resourceID], revision)
	}
	return grouped
}

func orderedRevisionContents(document evidenceDocument) []ExtractedContentArtifact {
	contents := make([]ExtractedContentArtifact, 0, len(document.revisionContents))
	for _, revision := range document.resourceRevisions {
		if content, exists := document.revisionContents[revision.RevisionID]; exists {
			contents = append(contents, content)
		}
	}
	return contents
}

func validateLensBatch(batch LensBatch) error {
	if batch.SchemaVersion != LensBatchSchemaVersion || len(batch.Lenses) == 0 {
		return errors.New("invalid personal memory lens batch")
	}
	seen := map[string]bool{}
	totalRunes := 0
	for _, lens := range batch.Lenses {
		if strings.TrimSpace(lens.ID) == "" || strings.TrimSpace(lens.Name) == "" || strings.TrimSpace(lens.Query) == "" || seen[lens.ID] {
			return errors.New("invalid personal memory lens")
		}
		seen[lens.ID] = true
		totalRunes += len([]rune(lens.ID)) + len([]rune(lens.Name)) + len([]rune(lens.Query))
	}
	if totalRunes > MaximumLensRequestRunes {
		return errors.New("personal memory lens request exceeds execution budget; submit smaller batches")
	}
	return nil
}

func containsAnyTerm(value string, terms []string) bool {
	counts := termCounts(tokenize(value))
	for _, term := range terms {
		if counts[term] > 0 {
			return true
		}
	}
	return false
}

func matchedWindow(value string, terms []string, maximum int) string {
	lower := strings.ToLower(value)
	start := -1
	for _, term := range terms {
		if index := strings.Index(lower, strings.ToLower(term)); index >= 0 && (start < 0 || index < start) {
			start = index
		}
	}
	runes := []rune(value)
	if start < 0 || len(runes) <= maximum {
		return snippet(value, maximum)
	}
	runeStart := len([]rune(value[:start]))
	windowStart := runeStart - maximum/4
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := windowStart + maximum
	if windowEnd > len(runes) {
		windowEnd = len(runes)
	}
	prefix := ""
	suffix := ""
	if windowStart > 0 {
		prefix = "…"
	}
	if windowEnd < len(runes) {
		suffix = "…"
	}
	return prefix + strings.TrimSpace(string(runes[windowStart:windowEnd])) + suffix
}

func tokenize(value string) []string {
	terms := []string{}
	var token strings.Builder
	flush := func() {
		if token.Len() == 0 {
			return
		}
		value := token.String()
		if len([]rune(value)) >= 2 {
			terms = append(terms, value)
		}
		token.Reset()
	}
	for _, character := range strings.ToLower(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			token.WriteRune(character)
		} else {
			flush()
		}
	}
	flush()
	return terms
}

func termCounts(terms []string) map[string]int {
	counts := make(map[string]int, len(terms))
	for _, term := range terms {
		counts[term]++
	}
	return counts
}

func snippet(value string, maximum int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return strings.TrimSpace(string(runes[:maximum])) + "…"
}
