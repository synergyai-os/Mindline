package agentretrieval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/embedding"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

const (
	embeddingChunkRunes    = 2_000
	embeddingChunkOverlap  = 200
	maximumEmbeddingChunks = 8
	maximumSemanticRanks   = 100
	maximumRelevanceIDs    = 100_000
	lexicalRRFWeight       = 2.0
	semanticRRFWeight      = 1.0
)

type HybridBackend struct {
	context              context.Context
	state                *agentstate.Store
	embedder             embedding.Port
	forcedDegradedReason string

	mu             sync.Mutex
	method         string
	retrievalState string
	degradedReason string
}

type semanticChunk struct {
	id       string
	recordID string
	text     string
}

type IndexProgress struct {
	Completed int
	Target    int
}

type distinctEvidenceAuthorization struct {
	winnerID string
	top1     float64
	runner   float64
	margin   float64
}

func NewHybridBackend(ctx context.Context, state *agentstate.Store, embedder embedding.Port) *HybridBackend {
	if ctx == nil {
		ctx = context.Background()
	}
	return &HybridBackend{
		context: ctx, state: state, embedder: embedder,
		method: "mindline_hybrid_local/v0.10", retrievalState: "hybrid",
	}
}

func NewDegradedBackend(
	ctx context.Context,
	state *agentstate.Store,
	reason string,
) *HybridBackend {
	backend := NewHybridBackend(ctx, state, nil)
	backend.forcedDegradedReason = strings.TrimSpace(reason)
	if backend.forcedDegradedReason == "" {
		backend.forcedDegradedReason = "semantic retrieval is not ready"
	}
	return backend
}

func (backend *HybridBackend) MethodID() string {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.method
}

func (backend *HybridBackend) RetrievalDiagnostics() (string, string) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.retrievalState, backend.degradedReason
}

func (backend *HybridBackend) CompactSemanticCalibrationID() string {
	if backend.embedder == nil ||
		backend.embedder.ModelID() != "ollama/embeddinggemma:latest/retrieval-input-v0.2" {
		return ""
	}
	return personalmemory.CompactSemanticCalibrationIdentity
}

func (backend *HybridBackend) Rank(request personalmemory.SearchRequest, documents []personalmemory.IndexDocument) ([]personalmemory.RankedHit, error) {
	if backend.state == nil {
		return nil, errors.New("hybrid retrieval state is unavailable")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, errors.New("search query is empty")
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		return nil, errors.New("search limit exceeds 100")
	}

	lexicalLimit := len(documents)
	if lexicalLimit > 100 {
		lexicalLimit = 100
	}
	lexicalQuery := strings.TrimSpace(request.LexicalQuery)
	if lexicalQuery == "" {
		lexicalQuery = query
	}
	lexicalHits, err := (personalmemory.LexicalBM25Backend{}).Rank(
		personalmemory.SearchRequest{
			Query: query, LexicalQuery: lexicalQuery, Limit: lexicalLimit,
		},
		documents,
	)
	if err != nil {
		return nil, err
	}
	contextQuery := strings.TrimSpace(strings.Join([]string{
		request.ScopePurpose, request.LensQuery,
	}, "\n"))
	scoped := strings.TrimSpace(request.ScopeID) != "" ||
		strings.TrimSpace(request.AgentID) != ""
	if !scoped && strings.TrimSpace(request.LensID) != "" {
		lens, err := backend.state.GetLens(backend.context, request.LensID)
		if err != nil {
			return nil, err
		}
		contextQuery = lens.Query
	}
	var authorizationSemanticScores map[string]float64
	var semanticErr error
	if backend.forcedDegradedReason != "" {
		authorizationSemanticScores = map[string]float64{}
		semanticErr = errors.New(backend.forcedDegradedReason)
	} else {
		authorizationSemanticScores, semanticErr = backend.semanticScores(query, documents)
	}
	rankingSemanticScores := authorizationSemanticScores
	if semanticErr == nil && contextQuery != "" {
		rankingSemanticScores, semanticErr = backend.semanticScores(
			strings.TrimSpace(query+"\n"+contextQuery), documents,
		)
	}
	backend.setMode(semanticErr)

	relevance, err := backend.feedbackRelevanceForRequest(request, documents)
	if err != nil {
		return nil, err
	}
	lexicalRanks := make(map[string]int, len(lexicalHits))
	lexicalComponents := make(map[string]map[string]float64, len(lexicalHits))
	matchedTerms := make(map[string][]string, len(lexicalHits))
	for index, hit := range lexicalHits {
		lexicalRanks[hit.DocumentID] = index + 1
		lexicalComponents[hit.DocumentID] = hit.Components
		matchedTerms[hit.DocumentID] = hit.MatchedTerms
	}
	semanticRanks := rankScores(rankingSemanticScores, maximumSemanticRanks)
	authorizationSemanticRanks := rankScores(
		authorizationSemanticScores, maximumSemanticRanks,
	)
	semanticTop1, semanticTop2, semanticMargin := topScoreMargin(
		authorizationSemanticScores,
	)
	distinctEvidence, distinctEvidenceOK := distinctEvidenceSemanticAuthorization(
		authorizationSemanticScores, documents,
	)
	type scored struct {
		id         string
		score      float64
		components map[string]float64
	}
	candidates := []scored{}
	maximumBase := 0.0
	maximumAuthorizationBase := 0.0
	for _, document := range documents {
		lexicalRRF := reciprocalRank(lexicalRanks[document.DocumentID])
		semanticRRF := reciprocalRank(semanticRanks[document.DocumentID])
		authorizationSemanticRRF := reciprocalRank(
			authorizationSemanticRanks[document.DocumentID],
		)
		base := lexicalRRFWeight*lexicalRRF + semanticRRFWeight*semanticRRF
		authorizationBase := lexicalRRFWeight*lexicalRRF +
			semanticRRFWeight*authorizationSemanticRRF
		if semanticErr != nil {
			base = lexicalRRFWeight * lexicalRRF
			authorizationBase = base
		}
		if base > maximumBase {
			maximumBase = base
		}
		if authorizationBase > maximumAuthorizationBase {
			maximumAuthorizationBase = authorizationBase
		}
		components := map[string]float64{
			"lexical_raw":                    lexicalComponents[document.DocumentID]["lexical_raw"],
			"lexical_rank":                   float64(lexicalRanks[document.DocumentID]),
			"lexical_rrf":                    lexicalRRF,
			"lexical_query_terms":            lexicalComponents[document.DocumentID]["lexical_query_terms"],
			"lexical_matched_terms":          lexicalComponents[document.DocumentID]["lexical_matched_terms"],
			"lexical_idf_coverage":           lexicalComponents[document.DocumentID]["lexical_idf_coverage"],
			"lexical_rarest_document_ratio":  lexicalComponents[document.DocumentID]["lexical_rarest_document_ratio"],
			"lexical_exact_ordered_phrase":   lexicalComponents[document.DocumentID]["lexical_exact_ordered_phrase"],
			"lexical_winner_relative_margin": lexicalComponents[document.DocumentID]["lexical_winner_relative_margin"],
			"semantic_cosine":                authorizationSemanticScores[document.DocumentID],
			"semantic_rank":                  float64(authorizationSemanticRanks[document.DocumentID]),
			"semantic_top1":                  semanticTop1,
			"semantic_top2":                  semanticTop2,
			"semantic_margin":                semanticMargin,
			"semantic_ranking_cosine":        rankingSemanticScores[document.DocumentID],
			"semantic_rrf":                   semanticRRF,
			"semantic_authorization_rrf":     authorizationSemanticRRF,
			"lens_feedback":                  relevance[document.DocumentID],
			"authorization_base_raw":         authorizationBase,
		}
		if distinctEvidenceOK && document.DocumentID == distinctEvidence.winnerID {
			components["semantic_distinct_evidence_valid"] = 1
			components["semantic_distinct_evidence_top1"] = distinctEvidence.top1
			components["semantic_distinct_evidence_runner"] = distinctEvidence.runner
			components["semantic_distinct_evidence_margin"] = distinctEvidence.margin
		}
		candidates = append(candidates, scored{
			id: document.DocumentID, components: components,
		})
	}
	if scoped {
		authorizedLimit := request.QueryAuthorizedLimit
		if authorizedLimit <= 0 {
			authorizedLimit = limit
		}
		sort.Slice(candidates, func(i, j int) bool {
			left := candidates[i].components["authorization_base_raw"]
			right := candidates[j].components["authorization_base_raw"]
			if left == right {
				return candidates[i].id < candidates[j].id
			}
			return left > right
		})
		if len(candidates) > authorizedLimit {
			candidates = candidates[:authorizedLimit]
		}
	}
	hits := make([]personalmemory.RankedHit, 0, len(candidates))
	for _, candidate := range candidates {
		base := lexicalRRFWeight * candidate.components["lexical_rrf"]
		if semanticErr == nil {
			base += semanticRRFWeight * candidate.components["semantic_rrf"]
		}
		if base == 0 {
			continue
		}
		if maximumBase > 0 {
			base /= maximumBase
		}
		candidate.components["hybrid_base"] = base
		if maximumAuthorizationBase > 0 {
			candidate.components["authorization_base"] =
				candidate.components["authorization_base_raw"] / maximumAuthorizationBase
		}
		candidate.score = base + candidate.components["lens_feedback"]
		candidate.components["final"] = candidate.score
		hits = append(hits, personalmemory.RankedHit{
			DocumentID: candidate.id, Score: candidate.score,
			MatchedTerms: matchedTerms[candidate.id], Components: candidate.components,
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].DocumentID < hits[j].DocumentID
		}
		return hits[i].Score > hits[j].Score
	})
	if !scoped && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (backend *HybridBackend) semanticScores(query string, documents []personalmemory.IndexDocument) (map[string]float64, error) {
	if backend.embedder == nil {
		return nil, errors.New("semantic provider is not configured")
	}
	scores := make(map[string]float64, len(documents))
	chunks := semanticChunks(documents)
	vectors, err := backend.prepareDocumentEmbeddings(chunks, nil)
	if err != nil {
		return nil, err
	}
	queryVector, err := backend.embedQuery(truncateRunes(query, embeddingChunkRunes))
	if err != nil {
		return nil, err
	}
	for _, chunk := range chunks {
		vector := vectors[chunk.id]
		score, err := embedding.Cosine(queryVector, vector)
		if err != nil {
			return nil, err
		}
		if current, exists := scores[chunk.recordID]; !exists || score > current {
			scores[chunk.recordID] = score
		}
	}
	return scores, nil
}

func (backend *HybridBackend) PrepareIndex(
	documents []personalmemory.IndexDocument,
	report func(IndexProgress),
) error {
	if backend.state == nil {
		return errors.New("hybrid retrieval state is unavailable")
	}
	if backend.embedder == nil {
		return errors.New("semantic provider is not configured")
	}
	_, err := backend.prepareDocumentEmbeddings(semanticChunks(documents), report)
	return err
}

func semanticChunks(documents []personalmemory.IndexDocument) []semanticChunk {
	chunks := []semanticChunk{}
	for _, document := range documents {
		for index, text := range embeddingChunks(document.Text) {
			chunks = append(chunks, semanticChunk{
				id:       document.DocumentID + "\x00chunk:" + stringID(index),
				recordID: document.DocumentID,
				text:     text,
			})
		}
	}
	return chunks
}

func (backend *HybridBackend) prepareDocumentEmbeddings(
	chunks []semanticChunk,
	report func(IndexProgress),
) (map[string][]float64, error) {
	vectors := make(map[string][]float64, len(chunks))
	missing := []semanticChunk{}
	modelID := backend.embedder.ModelID()
	for _, chunk := range chunks {
		fingerprint := textFingerprint(chunk.text)
		vector, exists, err := backend.state.LoadEmbedding(
			backend.context, chunk.id, fingerprint, modelID,
		)
		if err != nil {
			return nil, err
		}
		if exists {
			vectors[chunk.id] = vector
			continue
		}
		missing = append(missing, chunk)
	}
	completed := len(chunks) - len(missing)
	if report != nil {
		report(IndexProgress{Completed: completed, Target: len(chunks)})
	}
	for start := 0; start < len(missing); start += 32 {
		end := start + 32
		if end > len(missing) {
			end = len(missing)
		}
		inputs := make([]string, 0, end-start)
		for _, chunk := range missing[start:end] {
			inputs = append(inputs, chunk.text)
		}
		embedded, err := backend.embedDocuments(inputs)
		if err != nil {
			return nil, err
		}
		if len(embedded) != len(inputs) {
			return nil, errors.New("semantic provider returned an incomplete embedding batch")
		}
		for index, vector := range embedded {
			chunk := missing[start+index]
			vectors[chunk.id] = vector
			if err := backend.state.SaveEmbedding(backend.context, agentstate.Embedding{
				DocumentID: chunk.id, DocumentFingerprint: textFingerprint(chunk.text),
				Model: modelID, Vector: vector,
			}); err != nil {
				return nil, err
			}
			completed++
		}
		if report != nil {
			report(IndexProgress{Completed: completed, Target: len(chunks)})
		}
	}
	return vectors, nil
}

func distinctEvidenceSemanticAuthorization(
	scores map[string]float64,
	documents []personalmemory.IndexDocument,
) (distinctEvidenceAuthorization, bool) {
	if len(scores) == 0 || len(documents) == 0 {
		return distinctEvidenceAuthorization{}, false
	}
	documentsByID := make(map[string]personalmemory.IndexDocument, len(documents))
	aliasesByID := make(map[string][]string, len(documents))
	resourceDocumentsByAlias := map[string]string{}
	for _, document := range documents {
		documentID := strings.TrimSpace(document.DocumentID)
		if documentID == "" {
			return distinctEvidenceAuthorization{}, false
		}
		if _, exists := documentsByID[documentID]; exists {
			return distinctEvidenceAuthorization{}, false
		}
		aliases, valid := strictAuthorizationEvidenceAliases(document)
		if !valid {
			return distinctEvidenceAuthorization{}, false
		}
		switch document.AuthorizationEvidenceKind {
		case personalmemory.IndexEvidenceKindRecordSource:
		case personalmemory.IndexEvidenceKindUniqueResource:
			if len(aliases) != 1 {
				return distinctEvidenceAuthorization{}, false
			}
			if _, exists := resourceDocumentsByAlias[aliases[0]]; exists {
				return distinctEvidenceAuthorization{}, false
			}
			resourceDocumentsByAlias[aliases[0]] = documentID
		default:
			return distinctEvidenceAuthorization{}, false
		}
		documentsByID[documentID] = document
		aliasesByID[documentID] = aliases
	}
	winnerID := ""
	top1 := math.Inf(-1)
	for documentID, score := range scores {
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return distinctEvidenceAuthorization{}, false
		}
		if _, exists := documentsByID[documentID]; !exists {
			return distinctEvidenceAuthorization{}, false
		}
		if winnerID == "" || score > top1 ||
			(score == top1 && documentID < winnerID) {
			winnerID = documentID
			top1 = score
		}
	}
	winner, exists := documentsByID[winnerID]
	if !exists ||
		winner.AuthorizationEvidenceKind != personalmemory.IndexEvidenceKindUniqueResource {
		return distinctEvidenceAuthorization{}, false
	}
	winnerAliases := aliasesByID[winnerID]
	if len(winnerAliases) != 1 {
		return distinctEvidenceAuthorization{}, false
	}
	winnerAlias := winnerAliases[0]
	runner := math.Inf(-1)
	for documentID, score := range scores {
		if documentID == winnerID {
			continue
		}
		document := documentsByID[documentID]
		aliases := aliasesByID[documentID]
		if document.AuthorizationEvidenceKind == personalmemory.IndexEvidenceKindRecordSource &&
			len(aliases) == 1 && aliases[0] == winnerAlias {
			continue
		}
		if score > runner {
			runner = score
		}
	}
	if math.IsInf(runner, -1) {
		return distinctEvidenceAuthorization{
			winnerID: winnerID, top1: top1, runner: 0, margin: 1,
		}, true
	}
	return distinctEvidenceAuthorization{
		winnerID: winnerID, top1: top1, runner: runner, margin: top1 - runner,
	}, true
}

func strictAuthorizationEvidenceAliases(
	document personalmemory.IndexDocument,
) ([]string, bool) {
	if len(document.AuthorizationEvidenceAliases) == 0 {
		return nil, false
	}
	seen := map[string]bool{}
	aliases := make([]string, 0, len(document.AuthorizationEvidenceAliases))
	for _, alias := range document.AuthorizationEvidenceAliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || seen[alias] {
			return nil, false
		}
		seen[alias] = true
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases, true
}

func (backend *HybridBackend) feedbackRelevance(
	lensID string,
	documents []personalmemory.IndexDocument,
) (map[string]float64, error) {
	return backend.feedbackRelevanceForRequest(
		personalmemory.SearchRequest{LensID: lensID}, documents,
	)
}

func (backend *HybridBackend) feedbackRelevanceForRequest(
	request personalmemory.SearchRequest,
	documents []personalmemory.IndexDocument,
) (map[string]float64, error) {
	documentRelevance := make(map[string]float64, len(documents))
	if strings.TrimSpace(request.LensID) == "" || len(documents) == 0 {
		return documentRelevance, nil
	}
	aliasesByDocument := make(map[string][]string, len(documents))
	aliasSet := map[string]bool{}
	for _, document := range documents {
		aliases := sortedUniqueFeedbackAliases(document)
		aliasesByDocument[document.DocumentID] = aliases
		for _, alias := range aliases {
			aliasSet[alias] = true
		}
	}
	aliases := make([]string, 0, len(aliasSet))
	for alias := range aliasSet {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	relevanceByAlias := make(map[string]float64, len(aliases))
	for start := 0; start < len(aliases); start += maximumRelevanceIDs {
		end := start + maximumRelevanceIDs
		if end > len(aliases) {
			end = len(aliases)
		}
		var values map[string]float64
		var err error
		if strings.TrimSpace(request.ScopeID) != "" || strings.TrimSpace(request.AgentID) != "" {
			values, err = backend.state.ScopedRelevance(
				backend.context,
				agentstate.ScopedContext{
					ScopeID: request.ScopeID, LensID: request.LensID, AgentID: request.AgentID,
				},
				aliases[start:end],
			)
		} else {
			values, err = backend.state.Relevance(
				backend.context, request.LensID, aliases[start:end],
			)
		}
		if err != nil {
			return nil, err
		}
		for alias, value := range values {
			relevanceByAlias[alias] = value
		}
	}
	for _, document := range documents {
		total := 0.0
		observed := 0
		for _, alias := range aliasesByDocument[document.DocumentID] {
			value, exists := relevanceByAlias[alias]
			if !exists {
				continue
			}
			total += value
			observed++
		}
		if observed > 0 {
			documentRelevance[document.DocumentID] = total / float64(observed)
		}
	}
	return documentRelevance, nil
}

func sortedUniqueFeedbackAliases(document personalmemory.IndexDocument) []string {
	values := document.FeedbackAliases
	if len(values) == 0 {
		values = []string{document.DocumentID}
	}
	seen := map[string]bool{}
	aliases := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		aliases = append(aliases, value)
	}
	if len(aliases) == 0 && strings.TrimSpace(document.DocumentID) != "" {
		aliases = append(aliases, strings.TrimSpace(document.DocumentID))
	}
	sort.Strings(aliases)
	return aliases
}

func (backend *HybridBackend) embedDocuments(inputs []string) ([][]float64, error) {
	if retriever, ok := backend.embedder.(embedding.RetrievalPort); ok {
		return retriever.EmbedDocuments(backend.context, inputs)
	}
	return backend.embedder.Embed(backend.context, inputs)
}

func (backend *HybridBackend) embedQuery(input string) ([]float64, error) {
	if retriever, ok := backend.embedder.(embedding.RetrievalPort); ok {
		return retriever.EmbedQuery(backend.context, input)
	}
	vectors, err := backend.embedder.Embed(backend.context, []string{input})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func embeddingChunks(value string) []string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) == 0 {
		return nil
	}
	if len(runes) <= embeddingChunkRunes {
		return []string{string(runes)}
	}
	chunks := make([]string, 0, maximumEmbeddingChunks)
	coveredRunes := embeddingChunkRunes +
		(maximumEmbeddingChunks-1)*(embeddingChunkRunes-embeddingChunkOverlap)
	for index := 0; index < maximumEmbeddingChunks; index++ {
		start := index * (embeddingChunkRunes - embeddingChunkOverlap)
		if len(runes) > coveredRunes {
			start = index * (len(runes) - embeddingChunkRunes) /
				(maximumEmbeddingChunks - 1)
		}
		end := start + embeddingChunkRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end >= len(runes) {
			break
		}
	}
	return chunks
}

func stringID(value int) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if value < len(digits) {
		return string(digits[value])
	}
	return "x"
}

func (backend *HybridBackend) setMode(semanticErr error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if semanticErr == nil {
		backend.method = "mindline_hybrid_local/v0.10"
		backend.retrievalState = "hybrid"
		backend.degradedReason = ""
		return
	}
	backend.method = "mindline_lexical_degraded/v0.2"
	backend.retrievalState = "degraded"
	backend.degradedReason = semanticErr.Error()
}

func rankScores(scores map[string]float64, limit int) map[string]int {
	type item struct {
		id    string
		score float64
	}
	items := make([]item, 0, len(scores))
	for id, score := range scores {
		if math.IsNaN(score) || math.IsInf(score, 0) {
			continue
		}
		items = append(items, item{id: id, score: score})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].id < items[j].id
		}
		return items[i].score > items[j].score
	})
	ranks := make(map[string]int, len(items))
	for index, item := range items {
		if limit > 0 && index >= limit {
			break
		}
		ranks[item.id] = index + 1
	}
	return ranks
}

func topScoreMargin(scores map[string]float64) (float64, float64, float64) {
	top1 := math.Inf(-1)
	top2 := math.Inf(-1)
	for _, score := range scores {
		if math.IsNaN(score) || math.IsInf(score, 0) {
			continue
		}
		if score > top1 {
			top2 = top1
			top1 = score
		} else if score > top2 {
			top2 = score
		}
	}
	if math.IsInf(top1, -1) {
		return 0, 0, 0
	}
	if math.IsInf(top2, -1) {
		return top1, 0, 1
	}
	return top1, top2, top1 - top2
}

func reciprocalRank(rank int) float64 {
	if rank <= 0 {
		return 0
	}
	return 1 / float64(60+rank)
}

func textFingerprint(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func truncateRunes(value string, maximum int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maximum {
		return string(runes)
	}
	return string(runes[:maximum])
}
