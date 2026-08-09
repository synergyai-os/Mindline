package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/localservice"
)

func TestAgentHelpIsBoundedAndSuccessful(t *testing.T) {
	for _, command := range [][]string{{"agent", "--help"}, {"agent", "help"}} {
		var stdout, stderr bytes.Buffer
		if code := NewRunner(NewMemoryFS()).Run(command, &stdout, &stderr); code != ExitOK {
			t.Fatalf("command=%v code=%d stderr=%s", command, code, stderr.String())
		}
		text := stdout.String()
		if !strings.Contains(text, "agent discover") || !strings.Contains(text, "feedback-token") ||
			!strings.Contains(text, "owner/debug") || strings.Contains(text, "actor-put") ||
			strings.Count(text, "\n") > 30 || stderr.Len() != 0 {
			t.Fatalf("unbounded or incomplete help stdout=%q stderr=%q", text, stderr.String())
		}
	}
}

func TestAgentFeedbackTokenHasAtLeast128RandomBits(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := NewRunner(NewMemoryFS()).Run([]string{"agent", "feedback-token"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var result map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(result["retry_token"])
	if err != nil || len(decoded) < 16 || result["owner"] != "caller" ||
		result["reuse"] != "identical_retry_only" {
		t.Fatalf("result=%v bytes=%d err=%v", result, len(decoded), err)
	}
}

func TestScopedFeedbackFailureUsesClosedRepairSafeSchema(t *testing.T) {
	secretValue := "private-record-id"
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewMemoryFS()).Run([]string{"agent", "feedback", "--run", "run",
		"--scope", "project", "--lens", "product", "--record", secretValue,
		"--actor", "agent", "--disposition", "used", "--retry-token", "retry"},
		&stdout, &stderr)
	if code != ExitProcess || stdout.Len() != 0 || strings.Contains(stderr.String(), secretValue) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var failure map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil || len(failure) != 5 ||
		failure["schema_version"] != "mindline-agent-error/v0.1" ||
		failure["error_code"] != "incomplete_binding" {
		t.Fatalf("failure=%v err=%v", failure, err)
	}
}

func TestScopedGetFailureUsesClosedRepairSafeSchema(t *testing.T) {
	privateRecord := "private-record-id"
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewMemoryFS()).Run([]string{"agent", "get", privateRecord,
		"--run", "private-run", "--scope", "private-scope", "--lens", "private-lens"},
		&stdout, &stderr)
	if code != ExitProcess || stdout.Len() != 0 || strings.Contains(stderr.String(), privateRecord) ||
		strings.Contains(stderr.String(), "private-run") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var failure map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil || len(failure) != 5 ||
		failure["schema_version"] != "mindline-agent-error/v0.1" ||
		failure["error_code"] != "incomplete_binding" {
		t.Fatalf("failure=%v err=%v", failure, err)
	}
}

func TestMalformedScopedCommandsNeverExposeGlobalUsage(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		operation string
	}{
		{name: "get missing record", operation: "scoped_get", args: []string{"agent", "get", "--run", "run", "--scope", "scope", "--lens", "lens", "--agent", "agent"}},
		{name: "get unknown flag", operation: "scoped_get", args: []string{"agent", "get", "record", "--run", "run", "--scope", "scope", "--lens", "lens", "--agent", "agent", "--unknown", "value"}},
		{name: "get parse failure", operation: "scoped_get", args: []string{"agent", "get", "record", "--scope", "scope", "--agent"}},
		{name: "feedback missing actor", operation: "feedback", args: []string{"agent", "feedback", "--run", "run", "--scope", "scope", "--lens", "lens", "--agent", "agent", "--record", "record", "--disposition", "used", "--retry-token", "token"}},
		{name: "feedback unknown flag", operation: "feedback", args: []string{"agent", "feedback", "--run", "run", "--scope", "scope", "--lens", "lens", "--agent", "agent", "--record", "record", "--actor", "agent", "--disposition", "used", "--retry-token", "token", "--unknown", "value"}},
		{name: "feedback parse failure", operation: "feedback", args: []string{"agent", "feedback", "--scope", "scope", "--agent", "agent", "--reason"}},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := NewRunner(NewMemoryFS()).Run(item.args, &stdout, &stderr)
			if code != ExitProcess || stdout.Len() != 0 || strings.Contains(stderr.String(), "usage:") ||
				strings.Contains(stderr.String(), "scope-put") {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			var failure map[string]any
			if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil || len(failure) != 5 ||
				failure["operation"] != item.operation || failure["error_code"] != "invalid_scoped_command" {
				t.Fatalf("failure=%v err=%v", failure, err)
			}
		})
	}
}

func TestAgentDiscoverValidatesExactBindingAndPropagatesExplicitConfig(t *testing.T) {
	mutatingRequests := 0
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			mutatingRequests++
		}
		writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
			SchemaVersion: localservice.CapabilitiesSchemaVersion,
			Features:      []string{localservice.ScopedRecallCapability, localservice.DiscoveryCapability},
		})
	})
	mux.HandleFunc("GET /v1/scoped/scopes", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			mutatingRequests++
		}
		writeLegacyAgentEnvelope(t, writer, []agentstate.Scope{
			{ID: "project", Name: "Project", Status: agentstate.StatusActive},
			{ID: "other", Name: "Other", Status: agentstate.StatusActive},
			{ID: "archived-scope", Name: "Archived scope", Status: agentstate.StatusArchived},
		})
	})
	mux.HandleFunc("GET /v1/scoped/scopes/project/lenses", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			mutatingRequests++
		}
		writeLegacyAgentEnvelope(t, writer, []agentstate.ScopedLens{
			{ScopeID: "project", ID: "product", Name: "Product", Status: agentstate.StatusActive},
			{ScopeID: "project", ID: "archived-lens", Name: "Archived lens", Status: agentstate.StatusArchived},
		})
	})
	mux.HandleFunc("GET /v1/scoped/scopes/other/lenses", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			mutatingRequests++
		}
		writeLegacyAgentEnvelope(t, writer, []agentstate.ScopedLens{{ScopeID: "other", ID: "other-product", Name: "Other product", Status: agentstate.StatusActive}})
	})
	mux.HandleFunc("GET /v1/scoped/scopes/archived-scope/lenses", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			mutatingRequests++
		}
		writeLegacyAgentEnvelope(t, writer, []agentstate.ScopedLens{{ScopeID: "archived-scope", ID: "product", Name: "Product", Status: agentstate.StatusActive}})
	})
	mux.HandleFunc("GET /v1/scoped/actors", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			mutatingRequests++
		}
		writeLegacyAgentEnvelope(t, writer, []agentstate.AgentActor{
			{ID: "external", Name: "External", Status: agentstate.StatusActive},
			{ID: "archived", Name: "Archived", Status: agentstate.StatusArchived},
		})
	})
	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	runner := NewRunner(NewOSFileSystem())
	runner.agentExecutable = "/opt/mindline"
	output := runScopedAgentCLI(t, runner, configPath,
		"discover", "--scope", "project", "--lens", "product", "--agent", "external")
	var contract discoveryContract
	if err := json.Unmarshal(output, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.DiscoveryState != "ready" || contract.Binding.AgentID != "external" ||
		contract.Config["mode"] != "explicit" ||
		!strings.Contains(contract.Workflow["search_command"], "<same-as-discovery>") ||
		!strings.HasPrefix(contract.Workflow["search_command"], "'/opt/mindline' agent search") ||
		strings.Contains(string(output), configPath) || contract.Policy["authority_class"] != "personal_evidence_non_authoritative" {
		t.Fatalf("contract=%+v output=%s", contract, output)
	}
	otherOutput := runScopedAgentCLI(t, runner, configPath,
		"discover", "--scope", "other", "--lens", "other-product", "--agent", "external")
	var other discoveryContract
	if err := json.Unmarshal(otherOutput, &other); err != nil || other.Binding.ScopeID != "other" ||
		other.Binding.LensID != "other-product" {
		t.Fatalf("second plausible context=%+v err=%v", other, err)
	}

	negative := []struct {
		name, code string
		args       []string
	}{
		{name: "partial", code: "incomplete_binding", args: []string{"--scope", "project", "--lens", "product"}},
		{name: "cross scope", code: "binding_not_found", args: []string{"--scope", "project", "--lens", "other-product", "--agent", "external"}},
		{name: "unknown actor", code: "binding_not_found", args: []string{"--scope", "project", "--lens", "product", "--agent", "unknown"}},
		{name: "archived scope", code: "binding_archived", args: []string{"--scope", "archived-scope", "--lens", "product", "--agent", "external"}},
		{name: "archived lens", code: "binding_archived", args: []string{"--scope", "project", "--lens", "archived-lens", "--agent", "external"}},
		{name: "archived actor", code: "binding_archived", args: []string{"--scope", "project", "--lens", "product", "--agent", "archived"}},
	}
	for _, item := range negative {
		t.Run(item.name, func(t *testing.T) {
			args := append([]string{"agent", "discover"}, item.args...)
			args = append(args, "--config", configPath)
			var stdout, stderr bytes.Buffer
			code := runner.Run(args, &stdout, &stderr)
			if code != ExitProcess || stdout.Len() != 0 {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			var failure agentContractError
			if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil ||
				failure.ErrorCode != item.code || strings.Contains(stderr.String(), "unknown") {
				t.Fatalf("failure=%+v err=%v", failure, err)
			}
		})
	}
	if mutatingRequests != 0 {
		t.Fatalf("discovery issued %d mutating requests", mutatingRequests)
	}
}

func TestAgentDiscoverExplicitConfigNeverFallsBackToSecondDefaultRuntime(t *testing.T) {
	home := ""
	for _, suffix := range []string{"h", "i", "j", "k", "l"} {
		candidate := "/private/tmp/" + suffix
		if err := os.Mkdir(candidate, 0o700); err == nil {
			home = candidate
			break
		}
	}
	if home == "" {
		t.Fatal("no short private temporary home available")
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	defaultConfig, err := localservice.DefaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	_, closeDefault := startScopedAgentCLITestServerWithConfig(t, discoveryMux(t, "default-agent"), defaultConfig)
	defer closeDefault()

	explicitMux := discoveryMux(t, "explicit-agent")
	explicitConfig, closeExplicit := startScopedAgentCLITestServer(t, explicitMux)
	defer closeExplicit()
	runner := NewRunner(NewOSFileSystem())
	runner.agentExecutable = "/opt/mindline"
	runScopedAgentCLI(t, runner, explicitConfig,
		"discover", "--scope", "project", "--lens", "product", "--agent", "explicit-agent")

	var stdout, stderr bytes.Buffer
	code := runner.Run([]string{"agent", "discover", "--scope", "project", "--lens", "product",
		"--agent", "default-agent", "--config", explicitConfig}, &stdout, &stderr)
	if code != ExitProcess || stdout.Len() != 0 {
		t.Fatalf("explicit runtime silently fell back: code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var failure agentContractError
	if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil || failure.ErrorCode != "binding_not_found" {
		t.Fatalf("failure=%+v err=%v", failure, err)
	}
}

func TestAgentDiscoverPreservesRealServiceStateFingerprint(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-discovery-state-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := localservice.ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
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
	if _, err := client.PutScope(context.Background(), agentstate.Scope{ID: "project", Name: "Project", Purpose: "Recall"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutScopedLens(context.Background(), agentstate.ScopedLens{ScopeID: "project", ID: "product", Name: "Product", Query: "strategy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutActor(context.Background(), agentstate.AgentActor{ID: "external", Name: "External"}); err != nil {
		t.Fatal(err)
	}
	before := discoveryStateFingerprint(t, client)
	runner := NewRunner(NewOSFileSystem())
	runner.agentExecutable = "/opt/mindline"
	runScopedAgentCLI(t, runner, configPath,
		"discover", "--scope", "project", "--lens", "product", "--agent", "external")
	after := discoveryStateFingerprint(t, client)
	if !bytes.Equal(before, after) {
		t.Fatal("discovery changed the service state fingerprint")
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

func discoveryStateFingerprint(t *testing.T, client *localservice.Client) []byte {
	t.Helper()
	ctx := context.Background()
	status, err := client.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := client.ListScopes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lenses, err := client.ListScopedLenses(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	actors, err := client.ListActors(ctx)
	if err != nil {
		t.Fatal(err)
	}
	value := struct {
		MemoryRevision    uint64                  `json:"memory_revision"`
		MemoryFingerprint string                  `json:"memory_fingerprint"`
		RetrievalRuns     int                     `json:"retrieval_runs"`
		Judgments         int                     `json:"judgments"`
		Scopes            []agentstate.Scope      `json:"scopes"`
		Lenses            []agentstate.ScopedLens `json:"lenses"`
		Actors            []agentstate.AgentActor `json:"actors"`
	}{
		MemoryRevision: status.Memory.Revision, MemoryFingerprint: status.Memory.Fingerprint,
		RetrievalRuns: status.State.RetrievalRunCount, Judgments: status.State.JudgmentCount,
		Scopes: scopes, Lenses: lenses, Actors: actors,
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func discoveryMux(t *testing.T, actorID string) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
			SchemaVersion: localservice.CapabilitiesSchemaVersion,
			Features:      []string{localservice.ScopedRecallCapability, localservice.DiscoveryCapability},
		})
	})
	mux.HandleFunc("GET /v1/scoped/scopes", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, []agentstate.Scope{{ID: "project", Name: "Project", Status: agentstate.StatusActive}})
	})
	mux.HandleFunc("GET /v1/scoped/scopes/project/lenses", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, []agentstate.ScopedLens{{ScopeID: "project", ID: "product", Name: "Product", Status: agentstate.StatusActive}})
	})
	mux.HandleFunc("GET /v1/scoped/actors", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, []agentstate.AgentActor{{ID: actorID, Name: actorID, Status: agentstate.StatusActive}})
	})
	return mux
}
