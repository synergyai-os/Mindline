package agentretrieval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/embedding"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

const maximumEmbeddingRunes = 16 << 10

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
		method: "mindline_hybrid_local/v0.1", retrievalState: "hybrid",
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
	lexicalHits, err := (personalmemory.LexicalBM25Backend{}).Rank(
		personalmemory.SearchRequest{Query: query, Limit: lexicalLimit}, documents,
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
	semanticScores, semanticErr := backend.semanticScores(strings.TrimSpace(query+"\n"+lensQuery), documents)
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
	lexicalRaw := make(map[string]float64, len(lexicalHits))
	matchedTerms := make(map[string][]string, len(lexicalHits))
	for index, hit := range lexicalHits {
		lexicalRanks[hit.DocumentID] = index + 1
		lexicalRaw[hit.DocumentID] = hit.Score
		matchedTerms[hit.DocumentID] = hit.MatchedTerms
	}
	semanticRanks := rankScores(semanticScores)
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
		base := lexicalRRF + semanticRRF
		if semanticErr != nil {
			base = lexicalRRF
		}
		if base > maximumBase {
			maximumBase = base
		}
		candidates = append(candidates, scored{
			id: document.DocumentID,
			components: map[string]float64{
				"lexical_raw":     lexicalRaw[document.DocumentID],
				"lexical_rrf":     lexicalRRF,
				"semantic_cosine": semanticScores[document.DocumentID],
				"semantic_rrf":    semanticRRF,
				"lens_feedback":   relevance[document.DocumentID],
			},
		})
	}
	hits := make([]personalmemory.RankedHit, 0, len(candidates))
	for _, candidate := range candidates {
		base := candidate.components["lexical_rrf"]
		if semanticErr == nil {
			base += candidate.components["semantic_rrf"]
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
	vectors := make(map[string][]float64, len(documents))
	missing := []personalmemory.IndexDocument{}
	for _, document := range documents {
		fingerprint := textFingerprint(document.Text)
		vector, exists, err := backend.state.LoadEmbedding(
			backend.context, document.DocumentID, fingerprint, backend.embedder.ModelID(),
		)
		if err != nil {
			return nil, err
		}
		if exists {
			vectors[document.DocumentID] = vector
			continue
		}
		missing = append(missing, document)
	}
	for start := 0; start < len(missing); start += 32 {
		end := start + 32
		if end > len(missing) {
			end = len(missing)
		}
		inputs := make([]string, 0, end-start)
		for _, document := range missing[start:end] {
			inputs = append(inputs, truncateRunes(document.Text, maximumEmbeddingRunes))
		}
		embedded, err := backend.embedder.Embed(backend.context, inputs)
		if err != nil {
			return nil, err
		}
		for index, vector := range embedded {
			document := missing[start+index]
			vectors[document.DocumentID] = vector
			if err := backend.state.SaveEmbedding(backend.context, agentstate.Embedding{
				DocumentID: document.DocumentID, DocumentFingerprint: textFingerprint(document.Text),
				Model: backend.embedder.ModelID(), Vector: vector,
			}); err != nil {
				return nil, err
			}
		}
	}
	queryVectors, err := backend.embedder.Embed(backend.context, []string{truncateRunes(query, maximumEmbeddingRunes)})
	if err != nil {
		return nil, err
	}
	for documentID, vector := range vectors {
		score, err := embedding.Cosine(queryVectors[0], vector)
		if err != nil {
			return nil, err
		}
		scores[documentID] = score
	}
	return scores, nil
}

func (backend *HybridBackend) setMode(semanticErr error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if semanticErr == nil {
		backend.method = "mindline_hybrid_local/v0.1"
		backend.retrievalState = "hybrid"
		backend.degradedReason = ""
		return
	}
	backend.method = "mindline_lexical_degraded/v0.1"
	backend.retrievalState = "degraded"
	backend.degradedReason = semanticErr.Error()
}

func rankScores(scores map[string]float64) map[string]int {
	type item struct {
		id    string
		score float64
	}
	items := make([]item, 0, len(scores))
	for id, score := range scores {
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
		ranks[item.id] = index + 1
	}
	return ranks
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
