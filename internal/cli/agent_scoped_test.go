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

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func TestAgentCLIScopedLifecycleCommands(t *testing.T) {
	mux := http.NewServeMux()
	scopePuts := make(chan agentstate.Scope, 1)
	lensPuts := make(chan agentstate.ScopedLens, 1)
	actorPuts := make(chan agentstate.AgentActor, 1)

	mux.HandleFunc("PUT /v1/scoped/scopes/project", func(writer http.ResponseWriter, request *http.Request) {
		var scope agentstate.Scope
		if err := json.NewDecoder(request.Body).Decode(&scope); err != nil {
			t.Errorf("decode scope: %v", err)
		}
		scope.Status = agentstate.StatusActive
		scopePuts <- scope
		writeLegacyAgentEnvelope(t, writer, scope)
	})
	mux.HandleFunc("GET /v1/scoped/scopes", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, []agentstate.Scope{{ID: "project"}})
	})
	mux.HandleFunc("POST /v1/scoped/scopes/project/archive", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, agentstate.Scope{ID: "project", Status: agentstate.StatusArchived})
	})
	mux.HandleFunc("PUT /v1/scoped/scopes/project/lenses/product", func(writer http.ResponseWriter, request *http.Request) {
		var lens agentstate.ScopedLens
		if err := json.NewDecoder(request.Body).Decode(&lens); err != nil {
			t.Errorf("decode lens: %v", err)
		}
		lens.Status = agentstate.StatusActive
		lensPuts <- lens
		writeLegacyAgentEnvelope(t, writer, lens)
	})
	mux.HandleFunc("GET /v1/scoped/scopes/project/lenses", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, []agentstate.ScopedLens{{ScopeID: "project", ID: "product"}})
	})
	mux.HandleFunc("GET /v1/scoped/lenses", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, []agentstate.ScopedLens{{ScopeID: "project", ID: "product"}})
	})
	mux.HandleFunc("POST /v1/scoped/scopes/project/lenses/product/archive", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, agentstate.ScopedLens{
			ScopeID: "project", ID: "product", Status: agentstate.StatusArchived,
		})
	})
	mux.HandleFunc("PUT /v1/scoped/actors/codex", func(writer http.ResponseWriter, request *http.Request) {
		var actor agentstate.AgentActor
		if err := json.NewDecoder(request.Body).Decode(&actor); err != nil {
			t.Errorf("decode actor: %v", err)
		}
		actor.Status = agentstate.StatusActive
		actorPuts <- actor
		writeLegacyAgentEnvelope(t, writer, actor)
	})
	mux.HandleFunc("GET /v1/scoped/actors", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, []agentstate.AgentActor{{ID: "codex"}})
	})
	mux.HandleFunc("POST /v1/scoped/actors/codex/archive", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, agentstate.AgentActor{ID: "codex", Status: agentstate.StatusArchived})
	})

	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	runner := NewRunner(NewOSFileSystem())
	runScopedAgentCLI(t, runner, configPath, "scope-put", "project", "--name", "Project", "--purpose", "Recall")
	if scope := <-scopePuts; scope.ID != "project" || scope.Name != "Project" || scope.Purpose != "Recall" {
		t.Fatalf("scope put=%+v", scope)
	}
	runScopedAgentCLI(t, runner, configPath, "scope-list")
	runScopedAgentCLI(t, runner, configPath, "scope-archive", "project")
	runScopedAgentCLI(t, runner, configPath, "lens-put", "product", "--scope", "project", "--name", "Product", "--query", "strategy")
	if lens := <-lensPuts; lens.ScopeID != "project" || lens.ID != "product" || lens.Query != "strategy" {
		t.Fatalf("lens put=%+v", lens)
	}
	runScopedAgentCLI(t, runner, configPath, "lens-list", "--scope", "project")
	runScopedAgentCLI(t, runner, configPath, "lens-list")
	runScopedAgentCLI(t, runner, configPath, "lens-archive", "product", "--scope", "project")
	runScopedAgentCLI(t, runner, configPath, "actor-put", "codex", "--name", "Codex")
	if actor := <-actorPuts; actor.ID != "codex" || actor.Name != "Codex" {
		t.Fatalf("actor put=%+v", actor)
	}
	runScopedAgentCLI(t, runner, configPath, "actor-list")
	runScopedAgentCLI(t, runner, configPath, "actor-archive", "codex")
}

func TestAgentCLIScopedSearchAndFeedbackUseCompleteTuple(t *testing.T) {
	mux := http.NewServeMux()
	searches := make(chan localservice.ScopedSearchInput, 1)
	judgments := make(chan agentstate.ScopedJudgmentRequest, 3)
	mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
			SchemaVersion: localservice.CapabilitiesSchemaVersion,
			Features:      []string{localservice.ScopedRecallCapability},
		})
	})
	mux.HandleFunc("POST /v1/scoped/search/compact", func(writer http.ResponseWriter, request *http.Request) {
		var input localservice.ScopedSearchInput
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode search: %v", err)
		}
		searches <- input
		writeLegacyAgentEnvelope(t, writer, personalmemory.CompactContextPacket{
			SchemaVersion: personalmemory.ScopedCompactPacketSchemaVersion,
			RunID:         "run", Query: input.Query, ScopeID: input.ScopeID, LensID: input.LensID,
			AgentID: input.AgentID, Citations: []personalmemory.CompactCitation{},
		})
	})
	mux.HandleFunc("POST /v1/scoped/judgments", func(writer http.ResponseWriter, request *http.Request) {
		var input agentstate.ScopedJudgmentRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Errorf("decode judgment: %v", err)
		}
		judgments <- input
		writeLegacyAgentEnvelope(t, writer, agentstate.ScopedJudgment{
			JudgmentID: "judgment", ScopeID: input.ScopeID, LensID: input.LensID,
			AgentID: input.AgentID, Actor: input.Actor, RecordID: input.RecordID,
		})
	})

	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	runner := NewRunner(NewOSFileSystem())
	runScopedAgentCLI(t, runner, configPath,
		"search", "project", "strategy", "--scope", "project", "--lens", "product",
		"--agent", "codex", "--format", "compact-scoped-v0.4", "--limit", "7",
	)
	if input := <-searches; input.Query != "project strategy" || input.ScopeID != "project" ||
		input.LensID != "product" || input.AgentID != "codex" || input.Limit != 7 {
		t.Fatalf("scoped search=%+v", input)
	}
	runScopedAgentCLI(t, runner, configPath,
		"feedback", "--run", "run", "--scope", "project", "--lens", "product",
		"--agent", "codex", "--record", "record", "--actor", "agent",
		"--disposition", "used", "--retry-token", "retry-agent",
	)
	agentFeedback := <-judgments
	if agentFeedback.AgentID != "codex" || agentFeedback.Actor != agentstate.FeedbackAgent ||
		agentFeedback.ScopeID != "project" || agentFeedback.LensID != "product" {
		t.Fatalf("agent feedback=%+v", agentFeedback)
	}
	runScopedAgentCLI(t, runner, configPath,
		"feedback", "--run", "run", "--scope", "project", "--lens", "product",
		"--record", "record", "--actor", "owner", "--disposition", "dismissed",
		"--retry-token", "retry-owner",
	)
	ownerFeedback := <-judgments
	if ownerFeedback.AgentID != "" || ownerFeedback.Actor != agentstate.FeedbackOwner ||
		ownerFeedback.ScopeID != "project" || ownerFeedback.LensID != "product" {
		t.Fatalf("owner feedback=%+v", ownerFeedback)
	}
	runScopedAgentCLI(t, runner, configPath,
		"feedback-reverse", "--scope", "project", "--lens", "product",
		"--agent", "codex", "--actor", "agent", "--judgment", "judgment",
		"--idempotency-key", "reverse-agent-feedback",
	)
	reversal := <-judgments
	if reversal.ReversesID != "judgment" || reversal.IdempotencyKey != "reverse-agent-feedback" ||
		reversal.AgentID != "codex" || reversal.Actor != agentstate.FeedbackAgent {
		t.Fatalf("agent reversal=%+v", reversal)
	}
}

func TestAgentCLIRejectsPartialScopedTuplesBeforeConnecting(t *testing.T) {
	tests := [][]string{
		{"agent", "search", "query", "--scope", "project", "--lens", "product", "--format", "compact-scoped-v0.4"},
		{"agent", "search", "query", "--scope", "project", "--lens", "product", "--agent", "codex"},
		{"agent", "feedback", "--run", "run", "--scope", "project", "--lens", "product", "--record", "record", "--actor", "agent", "--disposition", "used", "--retry-token", "retry"},
		{"agent", "feedback", "--run", "run", "--scope", "project", "--lens", "product", "--agent", "codex", "--record", "record", "--actor", "owner", "--disposition", "used", "--retry-token", "retry"},
		{"agent", "feedback", "--run", "run", "--lens", "product", "--record", "record", "--actor", "owner", "--disposition", "used", "--retry-token", "retry"},
		{"agent", "lens-put", "product", "--name", "Product", "--query", "strategy"},
		{"agent", "lens-archive", "product"},
	}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		expected := ExitUsage
		if len(args) > 1 && args[1] == "feedback" {
			expected = ExitProcess
		}
		if code := NewRunner(NewMemoryFS()).Run(args, &stdout, &stderr); code != expected {
			t.Fatalf("args=%v code=%d stderr=%s", args, code, stderr.String())
		}
	}
}

func startScopedAgentCLITestServer(t *testing.T, mux *http.ServeMux) (string, func()) {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "mindline-agent-cli-scoped-")
	if err != nil {
		t.Fatal(err)
	}
	config, err := localservice.ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	configPath, closeServer := startScopedAgentCLITestServerWithConfig(t, mux, config)
	return configPath, func() {
		closeServer()
		_ = os.RemoveAll(root)
	}
}

func startScopedAgentCLITestServerWithConfig(
	t *testing.T, mux *http.ServeMux, config localservice.Config,
) (string, func()) {
	t.Helper()
	configPath := filepath.Join(config.RuntimeRoot, "config.json")
	if err := localservice.SaveConfig(configPath, config); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", config.SocketPath)
	if err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("GET /v1/status", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, localservice.Status{
			SchemaVersion: localservice.APISchemaVersion, ServiceState: "ready",
		})
	})
	server := &http.Server{Handler: mux}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	return configPath, func() {
		if err := server.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := <-result; err != nil && err != http.ErrServerClosed {
			t.Fatal(err)
		}
	}
}

func runScopedAgentCLI(t *testing.T, runner Runner, configPath string, args ...string) []byte {
	t.Helper()
	args = append([]string{"agent"}, args...)
	args = append(args, "--config", configPath)
	var stdout, stderr bytes.Buffer
	if code := runner.Run(args, &stdout, &stderr); code != ExitOK {
		t.Fatalf("args=%v code=%d stderr=%s", args, code, stderr.String())
	}
	return stdout.Bytes()
}
