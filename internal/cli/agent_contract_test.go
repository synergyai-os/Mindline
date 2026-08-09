package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

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

func TestAgentDiscoverValidatesExactBindingAndPropagatesExplicitConfig(t *testing.T) {
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
		writeLegacyAgentEnvelope(t, writer, []agentstate.AgentActor{{ID: "external", Name: "External", Status: agentstate.StatusActive}})
	})
	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	output := runScopedAgentCLI(t, NewRunner(NewOSFileSystem()), configPath,
		"discover", "--scope", "project", "--lens", "product", "--agent", "external")
	var contract discoveryContract
	if err := json.Unmarshal(output, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.DiscoveryState != "ready" || contract.Binding.AgentID != "external" ||
		contract.Config["mode"] != "explicit" ||
		!strings.Contains(contract.Workflow["search_command"], "<same-as-discovery>") ||
		strings.Contains(string(output), configPath) || contract.Policy["authority_class"] != "personal_evidence_non_authoritative" {
		t.Fatalf("contract=%+v output=%s", contract, output)
	}

	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"agent", "discover", "--scope", "project",
		"--lens", "product", "--agent", "unknown", "--config", configPath}, &stdout, &stderr)
	if code != ExitProcess || stdout.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var failure agentContractError
	if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil ||
		failure.ErrorCode != "binding_not_found" || strings.Contains(stderr.String(), "unknown") {
		t.Fatalf("failure=%+v err=%v", failure, err)
	}
}
