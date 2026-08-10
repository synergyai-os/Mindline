package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func TestAgentCLIDefaultsToCompactFormatAndReportsCapabilities(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-agent-cli-v03-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := localservice.ConfigFromRoots(
		filepath.Join(root, "runtime"), filepath.Join(root, "memory"),
	)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(config.RuntimeRoot, "config.json")
	if err := localservice.SaveConfig(configPath, config); err != nil {
		t.Fatal(err)
	}
	server, err := localservice.NewServer(config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve() }()
	client := localservice.NewClient(config.SocketPath)
	waitForAgentCLIService(t, client)

	var stdout, stderr bytes.Buffer
	runner := NewRunner(NewOSFileSystem())
	if code := runner.Run([]string{
		"agent", "capabilities", "--config", configPath,
	}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("capabilities code=%d stderr=%s", code, stderr.String())
	}
	var capabilities localservice.Capabilities
	if err := json.Unmarshal(stdout.Bytes(), &capabilities); err != nil ||
		capabilities.SchemaVersion != localservice.CapabilitiesSchemaVersion {
		t.Fatalf("capabilities=%+v err=%v", capabilities, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{
		"agent", "search", "what", "is", "this", "and", "how",
		"--config", configPath,
	}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("default compact search code=%d stderr=%s", code, stderr.String())
	}
	var packet personalmemory.CompactContextPacket
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil ||
		packet.SchemaVersion != personalmemory.CompactPacketSchemaVersion ||
		packet.AnswerState != "abstained" {
		t.Fatalf("compact packet=%+v err=%v", packet, err)
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

func TestAgentCLIRejectsAmbiguousFeedbackIdentityBeforeConnecting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewMemoryFS()).Run([]string{
		"agent", "feedback", "--run", "run", "--lens", "lens", "--record", "record",
		"--actor", "agent", "--disposition", "used",
		"--idempotency-key", "legacy", "--retry-token", "event",
	}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("ambiguous feedback code=%d stderr=%s", code, stderr.String())
	}
}

func TestAgentCLIDefaultCompactRequestFallsBackToLegacyService(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-agent-cli-legacy-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := localservice.ConfigFromRoots(
		filepath.Join(root, "runtime"), filepath.Join(root, "memory"),
	)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(config.RuntimeRoot, "config.json")
	if err := localservice.SaveConfig(configPath, config); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", config.SocketPath)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, localservice.Status{
			SchemaVersion: localservice.APISchemaVersion, ServiceState: "ready",
		})
	})
	mux.HandleFunc("POST /v1/search", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, personalmemory.ContextPacket{
			SchemaVersion: personalmemory.ContextPacketSchemaVersion,
			Query:         "durable memory", RetrievalMethod: "legacy-test",
			AuthorityClass: personalmemory.AuthorityClass,
			Citations:      []personalmemory.Citation{}, Records: []personalmemory.CaptureRecord{},
			Resources:         []personalmemory.ResourceContext{},
			ResourceRevisions: []personalmemory.ResourceRevision{},
		})
	})
	server := &http.Server{Handler: mux}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"agent", "search", "durable", "memory", "--config", configPath,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("legacy fallback code=%d stderr=%s", code, stderr.String())
	}
	var packet personalmemory.ContextPacket
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil ||
		packet.SchemaVersion != personalmemory.ContextPacketSchemaVersion ||
		packet.RetrievalMethod != "legacy-test" {
		t.Fatalf("legacy fallback packet=%+v err=%v", packet, err)
	}
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}

func writeLegacyAgentEnvelope(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"schema_version": localservice.APISchemaVersion,
		"data":           value,
	}); err != nil {
		t.Fatal(err)
	}
}

func waitForAgentCLIService(t *testing.T, client *localservice.Client) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Status(context.Background()); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("local agent service did not start")
}
