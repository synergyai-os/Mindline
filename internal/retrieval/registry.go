package retrieval

import (
	"context"
	"errors"
	"strings"
	"sync"
)

type Registry struct {
	mu         sync.RWMutex
	retrievers map[string]Retriever
}

func NewRegistry() *Registry { return &Registry{retrievers: map[string]Retriever{}} }

func (registry *Registry) Register(strategy string, retriever Retriever) error {
	strategy = strings.TrimSpace(strategy)
	if strategy == "" || retriever == nil {
		return errors.New("invalid retrieval strategy registration")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.retrievers[strategy]; exists {
		return errors.New("retrieval strategy already registered")
	}
	registry.retrievers[strategy] = retriever
	return nil
}

func (registry *Registry) Retrieve(ctx context.Context, request Request) (Artifact, error) {
	registry.mu.RLock()
	retriever := registry.retrievers[request.Strategy]
	registry.mu.RUnlock()
	if retriever == nil {
		return MissingArtifact(request, StateNotAttempted, AccessUnsupported, OriginSyntheticFixture, "unsupported retrieval strategy"), nil
	}
	artifact, err := retriever.Retrieve(ctx, request)
	if err != nil {
		return Artifact{}, err
	}
	if artifact.CanonicalItemID != request.CanonicalItemID || artifact.CanonicalURL != request.CanonicalURL || artifact.Strategy != request.Strategy || artifact.Format != request.Format {
		return Artifact{}, errors.New("retrieval artifact request identity mismatch")
	}
	if artifact.Origin == OriginLiveRetrieval {
		return Artifact{}, ErrLiveTransportDisabled
	}
	if err := ValidateArtifact(artifact); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}
