package localservice

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
)

func TestServerIsSingleWriterAndPersistsLensesAcrossRestart(t *testing.T) {
	t.Parallel()
	ollama := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/embed" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"embeddings":[[1,0]]}`))
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
