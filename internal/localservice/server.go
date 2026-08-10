package localservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentcontract"
	"github.com/synergyai-os/Mindline/internal/agentretrieval"
	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/embedding"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const maximumRequestBytes = 1 << 20

type Server struct {
	config     Config
	repository *personalmemory.FileRepository
	state      *agentstate.Store
	embedder   embedding.Port
	lock       *privateio.AdvisoryLock
	httpServer *http.Server
	listener   net.Listener
	now        func() time.Time
	recovery   string

	semanticMu         sync.Mutex
	semanticIndex      SemanticIndexStatus
	semanticCancel     context.CancelFunc
	semanticDone       chan struct{}
	semanticGeneration uint64
	semanticClosing    bool
}

func NewServer(config Config, now func() time.Time, httpClient *http.Client) (*Server, error) {
	if err := config.Prepare(); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	lock, err := privateio.AcquireAdvisoryLock(config.RuntimeRoot, pathFor(config.RuntimeRoot, "service.lock"))
	if errors.Is(err, privateio.ErrLockBusy) {
		return nil, errors.New("local agent service is already running")
	}
	if err != nil {
		return nil, errors.New("acquire local agent service ownership")
	}
	fail := func(err error) (*Server, error) {
		_ = lock.Close()
		return nil, err
	}
	repository, err := personalmemory.NewFileRepository(config.MemoryRoot, now)
	if err != nil {
		return fail(err)
	}
	state, recovery, err := agentstate.OpenRecovering(config.StatePath, now)
	if err != nil {
		return fail(err)
	}
	embedder, err := embedding.NewOllama(config.OllamaURL, config.EmbeddingModel, httpClient)
	if err != nil {
		_ = state.Close()
		return fail(err)
	}
	return &Server{
		config: config, repository: repository, state: state,
		embedder: embedder, lock: lock, now: now, recovery: recovery,
		semanticIndex: SemanticIndexStatus{State: "stale"},
	}, nil
}

func (server *Server) Serve() error {
	if err := removeStaleSocket(server.config); err != nil {
		return err
	}
	listener, err := net.Listen("unix", server.config.SocketPath)
	if err != nil {
		return errors.New("listen on local agent socket")
	}
	if err := os.Chmod(server.config.SocketPath, privateio.FileMode); err != nil {
		listener.Close()
		return errors.New("secure local agent socket")
	}
	server.listener = listener
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capabilities", server.handleCapabilities)
	mux.HandleFunc("GET /v1/status", server.handleStatus)
	mux.HandleFunc("POST /v1/search", server.handleSearch)
	mux.HandleFunc("POST /v1/search/compact", server.handleSearchCompact)
	mux.HandleFunc("GET /v1/captures/{recordID}", server.handleGet)
	mux.HandleFunc("GET /v1/lenses", server.handleListLenses)
	mux.HandleFunc("PUT /v1/lenses/{lensID}", server.handlePutLens)
	mux.HandleFunc("DELETE /v1/lenses/{lensID}", server.handleDeleteLens)
	mux.HandleFunc("POST /v1/judgments", server.handleJudgment)
	mux.HandleFunc("GET /v1/scoped/scopes", server.handleListScopes)
	mux.HandleFunc("PUT /v1/scoped/scopes/{scopeID}", server.handlePutScope)
	mux.HandleFunc("POST /v1/scoped/scopes/{scopeID}/archive", server.handleArchiveScope)
	mux.HandleFunc("GET /v1/scoped/scopes/{scopeID}/lenses", server.handleListScopedLenses)
	mux.HandleFunc("GET /v1/scoped/lenses", server.handleListScopedLenses)
	mux.HandleFunc("PUT /v1/scoped/scopes/{scopeID}/lenses/{lensID}", server.handlePutScopedLens)
	mux.HandleFunc("POST /v1/scoped/scopes/{scopeID}/lenses/{lensID}/archive", server.handleArchiveScopedLens)
	mux.HandleFunc("GET /v1/scoped/actors", server.handleListActors)
	mux.HandleFunc("POST /v1/scoped/actors/register", server.handleRegisterActor)
	mux.HandleFunc("PUT /v1/scoped/actors/{actorID}", server.handlePutActor)
	mux.HandleFunc("POST /v1/scoped/actors/{actorID}/archive", server.handleArchiveActor)
	mux.HandleFunc("POST /v1/scoped/search/compact", server.handleSearchScoped)
	mux.HandleFunc("POST /v1/scoped/get", server.handleGetScoped)
	mux.HandleFunc("POST /v1/scoped/judgments", server.handleScopedJudgment)
	server.httpServer = &http.Server{
		Handler: mux, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 2 * time.Minute,
	}
	server.ensureSemanticIndex()
	err = server.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (server *Server) Close(ctx context.Context) error {
	var first error
	semanticDone := server.beginSemanticShutdown()
	if server.httpServer != nil {
		if err := server.httpServer.Shutdown(ctx); err != nil {
			first = err
		}
	}
	if server.listener != nil {
		_ = server.listener.Close()
	}
	if semanticDone != nil {
		select {
		case <-semanticDone:
		case <-ctx.Done():
			if first == nil {
				first = errors.New("stop semantic index worker")
			}
		}
	}
	_ = os.Remove(server.config.SocketPath)
	if err := server.state.Close(); first == nil {
		first = err
	}
	if err := server.lock.Close(); first == nil {
		first = err
	}
	return first
}

func (server *Server) handleCapabilities(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, Capabilities{
		SchemaVersion:                CapabilitiesSchemaVersion,
		SearchFormats:                []string{"mindline-agent-context-packet/v0.2", "mindline-agent-context-packet/v0.3"},
		CompactSearchEndpoint:        "/v1/search/compact",
		CompactAbstentionPolicy:      personalmemory.DefaultCompactAbstentionPolicy(),
		ExplicitHydrationCommand:     agentcontract.NewWorkflow("mindline", "").Get,
		FeedbackRetryToken:           true,
		Features:                     []string{ScopedRecallCapability, DiscoveryCapability, AgentRegistrationCapability},
		ScopedSearchEndpoint:         "/v1/scoped/search/compact",
		ScopedFeedbackEndpoint:       "/v1/scoped/judgments",
		ScopedHydrationEndpoint:      ScopedHydrationEndpoint,
		AgentRegistrationEndpoint:    "/v1/scoped/actors/register",
		RecommendedAgentRoute:        RecommendedAgentRoute,
		OwnerDebugRouteClass:         OwnerDebugRouteClass,
		IdentityAssurance:            agentcontract.IdentityAssurance,
		HostileProcessAuthentication: false,
		OwnerMutationEnforcement:     agentcontract.MutationEnforcement,
		FeedbackTokenCommand:         agentcontract.FeedbackTokenCommand,
		RegistrationTokenCommand:     agentcontract.RegistrationTokenCommand,
	})
}

func (server *Server) handleStatus(writer http.ResponseWriter, _ *http.Request) {
	memory, err := server.repository.Status()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	state, err := server.state.Status(context.Background())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	if server.recovery != "" {
		state.RecoveryState = "database_rebuilt; user_state_restored_from_private_snapshot; derived_index_rebuilds_on_use; prior_database_quarantined"
	}
	writeJSON(writer, http.StatusOK, Status{
		SchemaVersion: APISchemaVersion, ServiceState: "ready",
		Memory: memory, State: projectAgentStateStatus(state),
		SemanticIndex: server.semanticIndexStatus(memory.Fingerprint, state.IndexedFingerprint),
	})
}

func projectAgentStateStatus(state agentstate.Status) PublicAgentStateStatus {
	return PublicAgentStateStatus{
		SchemaVersion: state.SchemaVersion, LensCount: state.LensCount,
		RetrievalRunCount: state.RetrievalRunCount, JudgmentCount: state.JudgmentCount,
		EmbeddingCount: state.EmbeddingCount, IndexedFingerprint: state.IndexedFingerprint,
		RecoveryState: state.RecoveryState,
	}
}

func (server *Server) handleSearch(writer http.ResponseWriter, request *http.Request) {
	var input SearchInput
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	runID, err := randomID("retrieval")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	backend := server.retrievalBackend(request.Context())
	packet, err := personalmemory.NewRetriever(server.repository, backend).Search(personalmemory.SearchRequest{
		Query: input.Query, Limit: input.Limit, RunID: runID, LensID: input.LensID,
	})
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	trace := agentstate.RetrievalTrace{
		RunID: runID, Query: packet.Query, LensID: packet.LensID,
		RetrievalMethod:    packet.RetrievalMethod,
		LibraryFingerprint: packet.LibraryFingerprint,
		CreatedAt:          server.now().UTC().Format(time.RFC3339Nano),
		Candidates:         make([]agentstate.CandidateTrace, 0, len(packet.Citations)),
	}
	for rank, citation := range packet.Citations {
		trace.Candidates = append(trace.Candidates, agentstate.CandidateTrace{
			RecordID: citation.RecordID, Rank: rank + 1, FinalScore: citation.Score,
			ComponentScore: citation.ComponentScores,
		})
	}
	if err := server.state.SaveRetrieval(request.Context(), trace); err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeJSON(writer, http.StatusOK, packet)
}

func (server *Server) handleSearchCompact(writer http.ResponseWriter, request *http.Request) {
	var input SearchInput
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	runID, err := randomID("retrieval")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	backend := server.retrievalBackend(request.Context())
	packet, err := personalmemory.NewRetriever(server.repository, backend).SearchCompact(
		personalmemory.SearchRequest{
			Query: input.Query, Limit: input.Limit, RunID: runID, LensID: input.LensID,
		},
	)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	trace := agentstate.RetrievalTrace{
		RunID: runID, Query: packet.Query, LensID: packet.LensID,
		RetrievalMethod: packet.RetrievalMethod, LibraryFingerprint: packet.LibraryFingerprint,
		CreatedAt:  server.now().UTC().Format(time.RFC3339Nano),
		Candidates: make([]agentstate.CandidateTrace, 0, len(packet.Citations)),
	}
	for rank, citation := range packet.Citations {
		trace.Candidates = append(trace.Candidates, agentstate.CandidateTrace{
			RecordID: citation.RecordID, Rank: rank + 1, FinalScore: citation.Score,
			ComponentScore: citation.ComponentScores,
		})
	}
	if err := server.state.SaveRetrieval(request.Context(), trace); err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeJSON(writer, http.StatusOK, packet)
}

func (server *Server) retrievalBackend(ctx context.Context) *agentretrieval.HybridBackend {
	server.ensureSemanticIndex()
	status := server.semanticIndexSnapshot()
	if status.State == "ready" {
		return agentretrieval.NewHybridBackend(ctx, server.state, server.embedder)
	}
	reason := status.Reason
	if reason == "" {
		reason = "semantic index is not ready; lexical retrieval is available"
	}
	return agentretrieval.NewDegradedBackend(ctx, server.state, reason)
}

func (server *Server) ensureSemanticIndex() {
	server.semanticMu.Lock()
	closing := server.semanticClosing
	server.semanticMu.Unlock()
	if closing {
		return
	}
	memory, err := server.repository.Status()
	if err != nil {
		server.setSemanticUnavailable("semantic index cannot read the current library")
		return
	}
	state, err := server.state.Status(context.Background())
	if err != nil {
		server.setSemanticUnavailable("semantic index state is unavailable")
		return
	}
	server.semanticMu.Lock()
	defer server.semanticMu.Unlock()
	if server.semanticClosing {
		return
	}
	if server.semanticIndex.State == "building" &&
		server.semanticIndex.LibraryFingerprint == memory.Fingerprint {
		return
	}
	// Recheck the cache once per service lifetime even when an older service
	// recorded this fingerprint. Prior versions could record it after a
	// degraded request without proving every current document was embedded.
	if server.semanticIndex.State == "ready" &&
		state.IndexedFingerprint == memory.Fingerprint {
		return
	}
	if server.semanticCancel != nil {
		server.semanticCancel()
	}
	server.semanticGeneration++
	generation := server.semanticGeneration
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	server.semanticCancel = cancel
	server.semanticDone = done
	server.semanticIndex = SemanticIndexStatus{
		State: "building", LibraryFingerprint: memory.Fingerprint,
		IndexedFingerprint: state.IndexedFingerprint,
		Reason:             "semantic index is building; lexical retrieval is available",
	}
	go server.prepareSemanticIndex(ctx, generation, memory.Fingerprint, done)
}

func (server *Server) beginSemanticShutdown() <-chan struct{} {
	server.semanticMu.Lock()
	defer server.semanticMu.Unlock()
	server.semanticClosing = true
	if server.semanticCancel != nil {
		server.semanticCancel()
	}
	return server.semanticDone
}

func (server *Server) prepareSemanticIndex(
	ctx context.Context,
	generation uint64,
	expectedFingerprint string,
	done chan struct{},
) {
	defer close(done)
	snapshot, err := personalmemory.NewLexicalRetriever(server.repository).PrepareCompactIndex()
	if err != nil {
		server.finishSemanticIndex(generation, "unavailable", expectedFingerprint,
			"semantic index preparation failed; lexical retrieval is available")
		return
	}
	if snapshot.LibraryFingerprint != expectedFingerprint {
		server.finishSemanticIndex(generation, "stale", snapshot.LibraryFingerprint,
			"the library changed while semantic indexing started; lexical retrieval is available")
		return
	}
	backend := agentretrieval.NewHybridBackend(ctx, server.state, server.embedder)
	err = backend.PrepareIndex(snapshot.Documents, func(progress agentretrieval.IndexProgress) {
		server.updateSemanticProgress(generation, progress)
	})
	if err != nil {
		if ctx.Err() != nil {
			server.finishSemanticIndex(generation, "stale", expectedFingerprint,
				"semantic indexing was interrupted; lexical retrieval is available")
			return
		}
		server.finishSemanticIndex(generation, "unavailable", expectedFingerprint,
			"semantic index provider is unavailable; lexical retrieval is available")
		return
	}
	// A fully cached document index does not otherwise contact the local model.
	// Warm and verify the query path before advertising semantic readiness so
	// the first real agent request does not silently fall back to lexical-only.
	var warmErr error
	if retrievalEmbedder, ok := server.embedder.(embedding.RetrievalPort); ok {
		_, warmErr = retrievalEmbedder.EmbedQuery(ctx, "mindline semantic readiness")
	} else {
		_, warmErr = server.embedder.Embed(ctx, []string{"mindline semantic readiness"})
	}
	if warmErr != nil {
		if ctx.Err() != nil {
			server.finishSemanticIndex(generation, "stale", expectedFingerprint,
				"semantic indexing was interrupted; lexical retrieval is available")
			return
		}
		server.finishSemanticIndex(generation, "unavailable", expectedFingerprint,
			"semantic query provider is unavailable; lexical retrieval is available")
		return
	}
	current, err := server.repository.Status()
	if err != nil || current.Fingerprint != expectedFingerprint {
		server.finishSemanticIndex(generation, "stale", expectedFingerprint,
			"the library changed during semantic indexing; lexical retrieval is available")
		return
	}
	if err := server.state.SetIndexedFingerprint(ctx, expectedFingerprint); err != nil {
		server.finishSemanticIndex(generation, "unavailable", expectedFingerprint,
			"semantic index readiness could not be recorded; lexical retrieval is available")
		return
	}
	server.finishSemanticIndex(generation, "ready", expectedFingerprint, "")
}

func (server *Server) updateSemanticProgress(
	generation uint64,
	progress agentretrieval.IndexProgress,
) {
	server.semanticMu.Lock()
	defer server.semanticMu.Unlock()
	if generation != server.semanticGeneration || server.semanticIndex.State != "building" {
		return
	}
	server.semanticIndex.Completed = progress.Completed
	server.semanticIndex.Target = progress.Target
}

func (server *Server) finishSemanticIndex(
	generation uint64,
	state string,
	fingerprint string,
	reason string,
) {
	server.semanticMu.Lock()
	defer server.semanticMu.Unlock()
	if generation != server.semanticGeneration {
		return
	}
	server.semanticIndex.State = state
	server.semanticIndex.LibraryFingerprint = fingerprint
	server.semanticIndex.Reason = reason
	if state == "ready" {
		server.semanticIndex.IndexedFingerprint = fingerprint
		server.semanticIndex.Completed = server.semanticIndex.Target
	}
	server.semanticCancel = nil
}

func (server *Server) setSemanticUnavailable(reason string) {
	server.semanticMu.Lock()
	defer server.semanticMu.Unlock()
	server.semanticIndex.State = "unavailable"
	server.semanticIndex.Reason = reason
}

func (server *Server) semanticIndexSnapshot() SemanticIndexStatus {
	server.semanticMu.Lock()
	defer server.semanticMu.Unlock()
	return server.semanticIndex
}

func (server *Server) semanticIndexStatus(
	libraryFingerprint string,
	indexedFingerprint string,
) SemanticIndexStatus {
	status := server.semanticIndexSnapshot()
	status.LibraryFingerprint = libraryFingerprint
	status.IndexedFingerprint = indexedFingerprint
	if status.State == "ready" && indexedFingerprint != libraryFingerprint {
		status.State = "stale"
		status.Reason = "semantic index is stale; lexical retrieval is available"
	}
	return status
}

func (server *Server) handleGet(writer http.ResponseWriter, request *http.Request) {
	recordID := strings.TrimSpace(request.PathValue("recordID"))
	if recordID == "" || len([]rune(recordID)) > 1024 {
		writeError(writer, http.StatusBadRequest, errors.New("invalid record id"))
		return
	}
	capture, err := personalmemory.NewLexicalRetriever(server.repository).Get(recordID)
	if err != nil {
		writeError(writer, http.StatusNotFound, err)
		return
	}
	capture.RouteClass = agentcontract.LegacyRouteClass
	capture.AgentRecallApproved = false
	writeJSON(writer, http.StatusOK, capture)
}

func (server *Server) handleListLenses(writer http.ResponseWriter, request *http.Request) {
	lenses, err := server.state.ListLenses(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeJSON(writer, http.StatusOK, lenses)
}

func (server *Server) handlePutLens(writer http.ResponseWriter, request *http.Request) {
	var lens agentstate.Lens
	if err := decodeRequest(request, &lens); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	lens.ID = strings.TrimSpace(request.PathValue("lensID"))
	saved, err := server.state.PutLens(request.Context(), lens)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, saved)
}

func (server *Server) handleDeleteLens(writer http.ResponseWriter, request *http.Request) {
	deleted, err := server.state.DeleteLens(request.Context(), request.PathValue("lensID"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, DeleteResult{Deleted: deleted})
}

func (server *Server) handleJudgment(writer http.ResponseWriter, request *http.Request) {
	var input agentstate.JudgmentRequest
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	judgment, err := server.state.ApplyJudgment(request.Context(), input)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, judgment)
}

func (server *Server) handleListScopes(writer http.ResponseWriter, request *http.Request) {
	scopes, err := server.state.ListScopes(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeJSON(writer, http.StatusOK, scopes)
}

func (server *Server) handlePutScope(writer http.ResponseWriter, request *http.Request) {
	var scope agentstate.Scope
	if err := decodeRequest(request, &scope); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	scope.ID = strings.TrimSpace(request.PathValue("scopeID"))
	saved, err := server.state.PutScope(request.Context(), scope)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, saved)
}

func (server *Server) handleArchiveScope(writer http.ResponseWriter, request *http.Request) {
	scope, err := server.state.ArchiveScope(request.Context(), request.PathValue("scopeID"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, scope)
}

func (server *Server) handleListScopedLenses(writer http.ResponseWriter, request *http.Request) {
	lenses, err := server.state.ListScopedLenses(
		request.Context(), strings.TrimSpace(request.PathValue("scopeID")),
	)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, lenses)
}

func (server *Server) handlePutScopedLens(writer http.ResponseWriter, request *http.Request) {
	var lens agentstate.ScopedLens
	if err := decodeRequest(request, &lens); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	lens.ScopeID = strings.TrimSpace(request.PathValue("scopeID"))
	lens.ID = strings.TrimSpace(request.PathValue("lensID"))
	saved, err := server.state.PutScopedLens(request.Context(), lens)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, saved)
}

func (server *Server) handleArchiveScopedLens(writer http.ResponseWriter, request *http.Request) {
	lens, err := server.state.ArchiveScopedLens(
		request.Context(), request.PathValue("scopeID"), request.PathValue("lensID"),
	)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, lens)
}

func (server *Server) handleListActors(writer http.ResponseWriter, request *http.Request) {
	actors, err := server.state.ListAgentActors(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeJSON(writer, http.StatusOK, actors)
}

func (server *Server) handlePutActor(writer http.ResponseWriter, request *http.Request) {
	var actor agentstate.AgentActor
	if err := decodeRequest(request, &actor); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	actor.ID = strings.TrimSpace(request.PathValue("actorID"))
	saved, err := server.state.PutAgentActor(request.Context(), actor)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, saved)
}

func (server *Server) handleRegisterActor(writer http.ResponseWriter, request *http.Request) {
	var input AgentRegistrationInput
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	actor, created, err := server.state.RegisterAgentActor(request.Context(), agentstate.AgentActor{
		ID: input.AgentID, Name: input.Name,
	})
	if err != nil {
		writeError(writer, http.StatusConflict, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(writer, status, actor)
}

func (server *Server) handleArchiveActor(writer http.ResponseWriter, request *http.Request) {
	actor, err := server.state.ArchiveAgentActor(request.Context(), request.PathValue("actorID"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, actor)
}

func (server *Server) handleSearchScoped(writer http.ResponseWriter, request *http.Request) {
	var input ScopedSearchInput
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	contextIDs := agentstate.ScopedContext{
		ScopeID: input.ScopeID, LensID: input.LensID, AgentID: input.AgentID,
	}
	scope, lens, actor, err := server.state.ResolveScopedContext(request.Context(), contextIDs)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	runID, err := randomID("retrieval")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	backend := server.retrievalBackend(request.Context())
	packet, err := personalmemory.NewRetriever(server.repository, backend).SearchCompact(
		personalmemory.SearchRequest{
			Query: input.Query, Limit: input.Limit, RunID: runID,
			ScopeID: scope.ID, ScopePurpose: scope.Purpose,
			LensID: lens.ID, LensQuery: lens.Query, AgentID: actor.ID,
		},
	)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	trace := agentstate.ScopedRetrievalTrace{
		RunID: runID, Query: packet.Query, ScopeID: packet.ScopeID,
		LensID: packet.LensID, AgentID: packet.AgentID,
		RetrievalMethod:    packet.RetrievalMethod,
		LibraryFingerprint: packet.LibraryFingerprint,
		CreatedAt:          server.now().UTC().Format(time.RFC3339Nano),
		Candidates:         make([]agentstate.ScopedCandidateTrace, 0, len(packet.Citations)),
	}
	for rank, citation := range packet.Citations {
		trace.Candidates = append(trace.Candidates, agentstate.ScopedCandidateTrace{
			RecordID: citation.RecordID, Rank: rank + 1, FinalScore: citation.Score,
			ComponentScore: citation.ComponentScores,
		})
	}
	if err := server.state.SaveScopedRetrieval(request.Context(), trace); err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeJSON(writer, http.StatusOK, packet)
}

func (server *Server) handleScopedJudgment(writer http.ResponseWriter, request *http.Request) {
	var input agentstate.ScopedJudgmentRequest
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	judgment, err := server.state.ApplyScopedJudgment(request.Context(), input)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeJSON(writer, http.StatusOK, judgment)
}

func decodeRequest(request *http.Request, target any) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(request.Body, maximumRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid request")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("invalid request")
	}
	return nil
}

func writeError(writer http.ResponseWriter, status int, err error) {
	writeEnvelope(writer, status, Envelope{SchemaVersion: APISchemaVersion, Error: err.Error()})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writeEnvelope(writer, status, Envelope{SchemaVersion: APISchemaVersion, Data: value})
}

func writeEnvelope(writer http.ResponseWriter, status int, envelope Envelope) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(envelope)
}

func removeStaleSocket(config Config) error {
	info, err := os.Lstat(config.SocketPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return errors.New("local agent socket path is unsafe")
	}
	if err := os.Remove(config.SocketPath); err != nil {
		return errors.New("remove stale local agent socket")
	}
	return nil
}

func randomID(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", errors.New("generate local agent id")
	}
	return prefix + "-" + hex.EncodeToString(value), nil
}

func pathFor(root, name string) string {
	return path.Clean(root + "/" + name)
}
