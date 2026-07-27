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
	"time"

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
	server.httpServer = &http.Server{
		Handler: mux, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 2 * time.Minute,
	}
	err = server.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (server *Server) Close(ctx context.Context) error {
	var first error
	if server.httpServer != nil {
		if err := server.httpServer.Shutdown(ctx); err != nil {
			first = err
		}
	}
	if server.listener != nil {
		_ = server.listener.Close()
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
		SchemaVersion:            CapabilitiesSchemaVersion,
		SearchFormats:            []string{"mindline-agent-context-packet/v0.2", "mindline-agent-context-packet/v0.3"},
		CompactSearchEndpoint:    "/v1/search/compact",
		CompactAbstentionPolicy:  personalmemory.DefaultCompactAbstentionPolicy(),
		ExplicitHydrationCommand: "agent get",
		FeedbackRetryToken:       true,
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
	backend := agentretrieval.NewHybridBackend(request.Context(), server.state, server.embedder)
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
	if err := server.state.SetIndexedFingerprint(request.Context(), packet.LibraryFingerprint); err != nil {
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
	backend := agentretrieval.NewHybridBackend(request.Context(), server.state, server.embedder)
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
	if err := server.state.SetIndexedFingerprint(request.Context(), packet.LibraryFingerprint); err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeJSON(writer, http.StatusOK, packet)
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
