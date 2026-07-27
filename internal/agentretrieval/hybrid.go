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
	lexicalRRFWeight       = 2.0
	semanticRRFWeight      = 1.0
)

type HybridBackend struct {
	context  context.Context
	state    *agentstate.Store
	embedder embedding.Port

	mu             sync.Mutex
	method         string
	retrievalState string
	degradedReason string
}

func NewHybridBackend(ctx context.Context, state *agentstate.Store, embedder embedding.Port) *HybridBackend {
	if ctx == nil {
		ctx = context.Background()
	}
	return &HybridBackend{
		context: ctx, state: state, embedder: embedder,
		method: "mindline_hybrid_local/v0.6", retrievalState: "hybrid",
	}
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
	lensQuery := ""
	if strings.TrimSpace(request.LensID) != "" {
		lens, err := backend.state.GetLens(backend.context, request.LensID)
		if err != nil {
			return nil, err
		}
		lensQuery = lens.Query
	}
	authorizationSemanticScores, semanticErr := backend.semanticScores(query, documents)
	rankingSemanticScores := authorizationSemanticScores
	if semanticErr == nil && strings.TrimSpace(lensQuery) != "" {
		rankingSemanticScores, semanticErr = backend.semanticScores(
			strings.TrimSpace(query+"\n"+lensQuery), documents,
		)
	}
	backend.setMode(semanticErr)

	documentIDs := make([]string, 0, len(documents))
	for _, document := range documents {
		documentIDs = append(documentIDs, document.DocumentID)
	}
	relevance, err := backend.state.Relevance(backend.context, request.LensID, documentIDs)
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
	type scored struct {
		id         string
		score      float64
		components map[string]float64
	}
	candidates := []scored{}
	maximumBase := 0.0
	for _, document := range documents {
		lexicalRRF := reciprocalRank(lexicalRanks[document.DocumentID])
		semanticRRF := reciprocalRank(semanticRanks[document.DocumentID])
		base := lexicalRRFWeight*lexicalRRF + semanticRRFWeight*semanticRRF
		if semanticErr != nil {
			base = lexicalRRFWeight * lexicalRRF
		}
		if base > maximumBase {
			maximumBase = base
		}
		candidates = append(candidates, scored{
			id: document.DocumentID,
			components: map[string]float64{
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
				"lens_feedback":                  relevance[document.DocumentID],
			},
		})
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
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (backend *HybridBackend) semanticScores(query string, documents []personalmemory.IndexDocument) (map[string]float64, error) {
	if backend.embedder == nil {
		return nil, errors.New("semantic provider is not configured")
	}
	scores := make(map[string]float64, len(documents))
	type semanticChunk struct {
		id       string
		recordID string
		text     string
	}
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
		for index, vector := range embedded {
			chunk := missing[start+index]
			vectors[chunk.id] = vector
			if err := backend.state.SaveEmbedding(backend.context, agentstate.Embedding{
				DocumentID: chunk.id, DocumentFingerprint: textFingerprint(chunk.text),
				Model: modelID, Vector: vector,
			}); err != nil {
				return nil, err
			}
		}
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
		backend.method = "mindline_hybrid_local/v0.6"
		backend.retrievalState = "hybrid"
		backend.degradedReason = ""
		return
	}
	backend.method = "mindline_lexical_degraded/v0.1"
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
