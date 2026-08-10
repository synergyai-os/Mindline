package localservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func TestSearchStaysUsefulWhileSemanticIndexBuildsInBackground(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	ollama := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/embed" {
			http.NotFound(writer, request)
			return
		}
		var input struct {
			Inputs []string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode embedding request: %v", err)
			return
		}
		startedOnce.Do(func() { close(started) })
		select {
		case <-release:
		case <-request.Context().Done():
			return
		}
		vectors := make([][]float64, len(input.Inputs))
		for index := range vectors {
			vectors[index] = []float64{1, 0}
		}
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(map[string]any{"embeddings": vectors}); err != nil {
			t.Errorf("encode embedding response: %v", err)
		}
	}))
	defer ollama.Close()

	root, err := os.MkdirTemp("/tmp", "mindline-semantic-index-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	config.OllamaURL = ollama.URL
	seedSemanticIndexMemory(t, config.MemoryRoot)
	server, err := NewServer(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve() }()
	client := NewClient(config.SocketPath)
	waitForService(t, client)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("semantic background indexing did not start")
	}
	status, err := client.Status(context.Background())
	if err != nil || status.ServiceState != "ready" || status.SemanticIndex.State != "building" ||
		status.SemanticIndex.Completed != 0 || status.SemanticIndex.Target != 1 {
		t.Fatalf("status=%+v err=%v", status, err)
	}

	searchContext, cancelSearch := context.WithTimeout(context.Background(), time.Second)
	packet, err := client.SearchCompact(searchContext, SearchInput{
		Query: "product brain citations", Limit: 3,
	})
	cancelSearch()
	if err != nil || packet.RetrievalState != "degraded" ||
		packet.RetrievalMethod != "mindline_lexical_degraded/v0.2" ||
		!strings.Contains(packet.DegradedReason, "building") || len(packet.Citations) != 1 {
		t.Fatalf("packet=%+v err=%v", packet, err)
	}

	close(release)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, err = client.Status(context.Background())
		if err == nil && status.SemanticIndex.State == "ready" &&
			status.SemanticIndex.IndexedFingerprint == status.Memory.Fingerprint {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || status.SemanticIndex.State != "ready" ||
		status.SemanticIndex.Completed != status.SemanticIndex.Target || status.SemanticIndex.Target != 1 {
		t.Fatalf("semantic index did not become ready: status=%+v err=%v", status, err)
	}
	packet, err = client.SearchCompact(context.Background(), SearchInput{
		Query: "product brain citations", Limit: 3,
	})
	if err != nil || packet.RetrievalState != "hybrid" ||
		packet.RetrievalMethod != "mindline_hybrid_local/v0.17" {
		t.Fatalf("hybrid packet=%+v err=%v", packet, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestCloseCancelsBackgroundSemanticIndex(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	ollama := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		startedOnce.Do(func() { close(started) })
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	defer ollama.Close()
	root, err := os.MkdirTemp("/tmp", "mindline-semantic-cancel-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	config.OllamaURL = ollama.URL
	seedSemanticIndexMemory(t, config.MemoryRoot)
	server, err := NewServer(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve() }()
	waitForService(t, NewClient(config.SocketPath))
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("semantic background indexing did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatal(err)
	}
	close(release)
	ollama.CloseClientConnections()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestSemanticIndexCannotRearmAfterShutdownBegins(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-semantic-closing-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	seedSemanticIndexMemory(t, config.MemoryRoot)
	server, err := NewServer(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	server.beginSemanticShutdown()

	const attempts = 16
	var wait sync.WaitGroup
	wait.Add(attempts)
	for index := 0; index < attempts; index++ {
		go func() {
			defer wait.Done()
			server.ensureSemanticIndex()
		}()
	}
	wait.Wait()

	server.semanticMu.Lock()
	generation := server.semanticGeneration
	worker := server.semanticDone
	server.semanticMu.Unlock()
	if generation != 0 || worker != nil {
		t.Fatalf("semantic worker rearmed during shutdown: generation=%d", generation)
	}
	if err := server.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func seedSemanticIndexMemory(t *testing.T, root string) {
	t.Helper()
	repository, err := personalmemory.NewFileRepository(root, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "self", SourceContainerID: "dm",
		ExternalID: "1", OccurredAt: "2026-08-08T08:00:00Z",
		SourceRef: "slack://self/dm/1",
		RawText:   "product brain citations make local agent recall useful",
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "slack-self", LowerInclusive: "1", UpperInclusive: "1",
		Watermark: "1", DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
}

func TestServerIsSingleWriterAndPersistsLensesAcrossRestart(t *testing.T) {
	t.Parallel()
	ollama := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/embed" {
			http.NotFound(writer, request)
			return
		}
		var input struct {
			Inputs []string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode embedding request: %v", err)
			return
		}
		vectors := make([][]float64, len(input.Inputs))
		for index := range vectors {
			vectors[index] = []float64{1, 0}
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"embeddings": vectors})
	}))
	defer ollama.Close()
	root, err := os.MkdirTemp("/tmp", "mindline-svc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	config.OllamaURL = ollama.URL
	server, err := NewServer(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewServer(config, nil, nil); err == nil {
		t.Fatal("second writer unexpectedly acquired service ownership")
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve() }()
	time.Sleep(20 * time.Millisecond)
	select {
	case serveErr := <-result:
		t.Fatalf("serve failed: %v", serveErr)
	default:
	}
	client := NewClient(config.SocketPath)
	waitForService(t, client)
	for index := 0; index < 4; index++ {
		id := fmt.Sprintf("lens-%d", index)
		if _, err := client.PutLens(context.Background(), agentstate.Lens{
			ID: id, Name: id, Query: "product context",
		}); err != nil {
			t.Fatal(err)
		}
	}
	packet, err := client.Search(context.Background(), SearchInput{
		Query: "product context", LensID: "lens-0", Limit: 3,
	})
	if err != nil || packet.RetrievalState != "hybrid" || packet.RunID == "" {
		t.Fatalf("packet=%+v err=%v", packet, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := server.Close(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-result; err != nil {
		t.Fatal(err)
	}

	restarted, err := NewServer(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result = make(chan error, 1)
	go func() { result <- restarted.Serve() }()
	time.Sleep(20 * time.Millisecond)
	select {
	case serveErr := <-result:
		t.Fatalf("restart serve failed: %v", serveErr)
	default:
	}
	waitForService(t, client)
	lenses, err := client.ListLenses(context.Background())
	if err != nil || len(lenses) != 4 {
		t.Fatalf("lenses=%+v err=%v", lenses, err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	if err := restarted.Close(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func waitForService(t *testing.T, client *Client) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if _, err := client.Status(context.Background()); err == nil {
			return
		} else {
			lastErr = err
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("local service did not become ready: %v", lastErr)
}
