package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentcontract"
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
			!strings.Contains(text, "registration-token") || !strings.Contains(text, "agent register") ||
			!strings.Contains(text, "owner must supply the complete scope and lens") ||
			!strings.Contains(text, "Never list, choose, infer") ||
			!strings.Contains(text, "owner/debug") || strings.Contains(text, "actor-put") ||
			strings.Count(text, "\n") > 30 || stderr.Len() != 0 {
			t.Fatalf("unbounded or incomplete help stdout=%q stderr=%q", text, stderr.String())
		}
	}
}

func TestAgentProjectConnectionHandleAndConnectedDiscovery(t *testing.T) {
	handle := "mlc1_" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	expectedDigest, err := projectConnectionDigest(handle)
	if err != nil {
		t.Fatal(err)
	}
	var receivedDigest string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
			SchemaVersion: localservice.CapabilitiesSchemaVersion,
			Features: []string{
				localservice.ScopedRecallCapability,
				localservice.DiscoveryCapability,
				localservice.ProjectConnectionCapability,
			},
		})
	})
	mux.HandleFunc("POST /v1/scoped/connections/resolve", func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), handle) {
			t.Fatal("plaintext connection handle crossed the service boundary")
		}
		var input localservice.ProjectConnectionDigestInput
		if err := json.Unmarshal(body, &input); err != nil {
			t.Fatal(err)
		}
		receivedDigest = input.Digest
		writeLegacyAgentEnvelope(t, writer, localservice.ProjectConnectionResolution{
			SchemaVersion: agentstate.ProjectConnectionSchemaVersion,
			State:         "ready", ScopeID: "project", ScopeName: "Project",
			LensID: "product", LensName: "Product", AgentID: "external", AgentName: "External",
		})
	})
	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	runner := NewRunner(NewOSFileSystem())
	runner.agentExecutable = "/opt/mindline"
	var stdout, stderr bytes.Buffer
	if code := runner.Run([]string{"agent-only", "discover", "--connection", handle,
		"--config", configPath}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var contract discoveryContract
	if err := json.Unmarshal(stdout.Bytes(), &contract); err != nil ||
		contract.DiscoveryState != "ready" || contract.Binding.ScopeID != "project" ||
		contract.Binding.LensID != "product" || contract.Binding.AgentID != "external" ||
		!strings.Contains(contract.Workflow["search_command"], "agent-only search") ||
		receivedDigest != expectedDigest || strings.Contains(stdout.String(), handle) ||
		strings.Contains(stdout.String(), expectedDigest) || strings.Contains(stdout.String(), configPath) {
		t.Fatalf("contract=%+v digest=%q err=%v output=%s", contract, receivedDigest, err, stdout.String())
	}

	for _, invalid := range []string{
		" " + handle, handle + " ", "https://example.com/" + handle,
		"../../" + handle, "pb_sk_" + strings.Repeat("a", 64), "mlc1_short",
	} {
		stdout.Reset()
		stderr.Reset()
		code := runner.Run([]string{"agent-only", "discover", "--connection", invalid,
			"--config", configPath}, &stdout, &stderr)
		var failure agentContractError
		if code != ExitProcess || stdout.Len() != 0 ||
			json.Unmarshal(stderr.Bytes(), &failure) != nil || failure.ErrorCode != "invalid_connection" ||
			strings.Contains(stderr.String(), invalid) {
			t.Fatalf("invalid=%q code=%d stdout=%s stderr=%s", invalid, code, stdout.String(), stderr.String())
		}
	}
}

func TestAgentProjectConnectionHandleHasExactOpaqueGrammar(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := NewRunner(NewMemoryFS()).Run([]string{"agent", "connection-handle"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var receipt struct {
		SchemaVersion string `json:"schema_version"`
		Connection    string `json:"connection"`
		Secret        bool   `json:"secret"`
		Owner         string `json:"owner"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil ||
		receipt.SchemaVersion != "mindline-project-connection-handle/v0.1" ||
		len(receipt.Connection) != 48 || !strings.HasPrefix(receipt.Connection, "mlc1_") ||
		receipt.Secret || receipt.Owner != "caller" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
	if _, err := projectConnectionDigest(receipt.Connection); err != nil {
		t.Fatal(err)
	}
}

func TestAgentProjectConnectionUnknownOutcomeRequiresExactRetry(t *testing.T) {
	handle := "mlc1_" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{6}, 32))
	mux := http.NewServeMux()
	writeUnknown := func(writer http.ResponseWriter) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"schema_version": localservice.APISchemaVersion,
			"error":          "project connection outcome requires identical retry",
		})
	}
	mux.HandleFunc("POST /v1/scoped/connections/bind", func(writer http.ResponseWriter, _ *http.Request) {
		writeUnknown(writer)
	})
	mux.HandleFunc("POST /v1/scoped/connections/archive", func(writer http.ResponseWriter, _ *http.Request) {
		writeUnknown(writer)
	})
	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	runner := NewRunner(NewOSFileSystem())
	for _, test := range []struct {
		name, operation, repair string
		args                    []string
	}{
		{name: "bind", operation: "connection_bind", repair: "retry_same_connection_bind", args: []string{
			"agent", "connection-bind", "--connection", handle,
			"--scope", "project", "--lens", "product", "--agent", "agent-a", "--config", configPath,
		}},
		{name: "archive", operation: "connection_archive", repair: "retry_same_connection_archive", args: []string{
			"agent", "connection-archive", "--connection", handle, "--config", configPath,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := runner.Run(test.args, &stdout, &stderr); code != ExitProcess || stdout.Len() != 0 {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			var failure agentContractError
			if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil ||
				failure.ErrorCode != "connection_outcome_unknown" || failure.Operation != test.operation ||
				!failure.Retryable || failure.RepairAction != test.repair || strings.Contains(stderr.String(), handle) {
				t.Fatalf("failure=%+v err=%v stderr=%s", failure, err, stderr.String())
			}
		})
	}
}

func TestAgentProjectConnectionDroppedResponseRequiresExactRetry(t *testing.T) {
	handle := "mlc1_" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))
	mux := http.NewServeMux()
	dropResponse := func(writer http.ResponseWriter) {
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			t.Fatal("test server does not support connection hijacking")
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		_ = connection.Close()
	}
	mux.HandleFunc("POST /v1/scoped/connections/bind", func(writer http.ResponseWriter, _ *http.Request) {
		dropResponse(writer)
	})
	mux.HandleFunc("POST /v1/scoped/connections/archive", func(writer http.ResponseWriter, _ *http.Request) {
		dropResponse(writer)
	})
	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	runner := NewRunner(NewOSFileSystem())
	for _, test := range []struct {
		operation, repair string
		args              []string
	}{
		{operation: "connection_bind", repair: "retry_same_connection_bind", args: []string{
			"agent", "connection-bind", "--connection", handle,
			"--scope", "project", "--lens", "product", "--agent", "agent-a", "--config", configPath,
		}},
		{operation: "connection_archive", repair: "retry_same_connection_archive", args: []string{
			"agent", "connection-archive", "--connection", handle, "--config", configPath,
		}},
	} {
		var stdout, stderr bytes.Buffer
		if code := runner.Run(test.args, &stdout, &stderr); code != ExitProcess || stdout.Len() != 0 {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
		var failure agentContractError
		if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil ||
			failure.ErrorCode != "connection_outcome_unknown" || failure.Operation != test.operation ||
			!failure.Retryable || failure.RepairAction != test.repair || strings.Contains(stderr.String(), handle) {
			t.Fatalf("failure=%+v err=%v stderr=%s", failure, err, stderr.String())
		}
	}
}

func TestAgentRegisterUsageUsesSharedAgentNamePlaceholder(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewMemoryFS()).Run([]string{"agent", "register"}, &stdout, &stderr)
	if code != ExitProcess || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "invalid_registration") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(usage, "agent register --name <agent-name>") ||
		strings.Contains(usage, "agent register --name <name>") {
		t.Fatalf("agent registration usage drifted: %s", usage)
	}
}

func TestAgentRegistrationFailurePreservesRetryIdentityUnlessConflictIsConfirmed(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 24))
	for _, test := range []struct {
		name         string
		status       int
		errorCode    string
		retryable    bool
		repairAction string
	}{
		{name: "rejected input", status: http.StatusBadRequest, errorCode: "registration_rejected", repairAction: "correct_registration_input"},
		{name: "confirmed conflict", status: http.StatusConflict, errorCode: "registration_conflict", repairAction: "create_registration_token"},
		{name: "ambiguous server failure", status: http.StatusInternalServerError, errorCode: "registration_outcome_unknown", retryable: true, repairAction: "retry_same_registration"},
	} {
		t.Run(test.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, _ *http.Request) {
				writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
					SchemaVersion: localservice.CapabilitiesSchemaVersion,
					Features:      []string{localservice.AgentRegistrationCapability},
				})
			})
			mux.HandleFunc("POST /v1/scoped/actors/register", func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(test.status)
				_ = json.NewEncoder(writer).Encode(map[string]string{
					"schema_version": localservice.APISchemaVersion,
					"error":          "registration failed",
				})
			})
			configPath, closeServer := startScopedAgentCLITestServer(t, mux)
			defer closeServer()
			var stdout, stderr bytes.Buffer
			code := NewRunner(NewOSFileSystem()).Run([]string{"agent", "register", "--name", "Fresh agent",
				"--retry-token", token, "--config", configPath}, &stdout, &stderr)
			if code != ExitProcess || stdout.Len() != 0 {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			var failure agentContractError
			if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil ||
				failure.ErrorCode != test.errorCode || failure.Retryable != test.retryable ||
				failure.RepairAction != test.repairAction || strings.Contains(stderr.String(), token) {
				t.Fatalf("failure=%+v err=%v", failure, err)
			}
		})
	}
}

func TestAgentRegistrationUsesOneBoundedDeadline(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{4}, 24))
	for _, hangAt := range []string{"capabilities", "registration"} {
		t.Run(hangAt, func(t *testing.T) {
			waitPastDeadline := func(request *http.Request) {
				timer := time.NewTimer(250 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-request.Context().Done():
				case <-timer.C:
				}
			}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, request *http.Request) {
				if hangAt == "capabilities" {
					waitPastDeadline(request)
					return
				}
				writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
					SchemaVersion: localservice.CapabilitiesSchemaVersion,
					Features:      []string{localservice.AgentRegistrationCapability},
				})
			})
			mux.HandleFunc("POST /v1/scoped/actors/register", func(_ http.ResponseWriter, request *http.Request) {
				waitPastDeadline(request)
			})
			configPath, closeServer := startScopedAgentCLITestServer(t, mux)
			defer closeServer()
			runner := NewRunner(NewOSFileSystem())
			runner.agentRegistrationTimeout = 75 * time.Millisecond
			var stdout, stderr bytes.Buffer
			started := time.Now()
			code := runner.Run([]string{"agent", "register", "--name", "Fresh agent",
				"--retry-token", token, "--config", configPath}, &stdout, &stderr)
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("registration exceeded bounded deadline: %s", elapsed)
			}
			var failure agentContractError
			expectedCode := "registration_outcome_unknown"
			if hangAt == "capabilities" {
				expectedCode = "service_unavailable"
			}
			if code != ExitProcess || stdout.Len() != 0 ||
				json.Unmarshal(stderr.Bytes(), &failure) != nil ||
				failure.ErrorCode != expectedCode || !failure.Retryable ||
				failure.RepairAction != "retry_same_registration" ||
				strings.Contains(stderr.String(), token) {
				t.Fatalf("code=%d stdout=%s failure=%+v stderr=%s", code, stdout.String(), failure, stderr.String())
			}
		})
	}
}

func TestAgentRegistrationDeadlineIncludesServiceRecovery(t *testing.T) {
	token := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{5}, 24))
	root, err := os.MkdirTemp("/tmp", "mindline-agent-registration-recovery-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	config, err := localservice.ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(config.RuntimeRoot, "config.json")
	if err := localservice.SaveConfig(configPath, config); err != nil {
		t.Fatal(err)
	}
	originalRestart := restartAgentServiceWithin
	restartAgentServiceWithin = func(ctx context.Context, _ string) (localservice.InstallReceipt, error) {
		<-ctx.Done()
		return localservice.InstallReceipt{}, ctx.Err()
	}
	defer func() { restartAgentServiceWithin = originalRestart }()
	runner := NewRunner(NewOSFileSystem())
	runner.agentRegistrationTimeout = 75 * time.Millisecond
	var stdout, stderr bytes.Buffer
	started := time.Now()
	code := runner.Run([]string{"agent", "register", "--name", "Fresh agent",
		"--retry-token", token, "--config", configPath}, &stdout, &stderr)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("registration recovery exceeded bounded deadline: %s", elapsed)
	}
	var failure agentContractError
	if code != ExitProcess || stdout.Len() != 0 || json.Unmarshal(stderr.Bytes(), &failure) != nil ||
		failure.ErrorCode != "service_unavailable" || !failure.Retryable ||
		failure.RepairAction != "retry_service" || strings.Contains(stderr.String(), token) {
		t.Fatalf("code=%d stdout=%s failure=%+v stderr=%s", code, stdout.String(), failure, stderr.String())
	}
}

func TestAgentRegistrationCreatesOpaqueIdentityWithoutSendingOrReturningToken(t *testing.T) {
	runner := NewRunner(NewOSFileSystem())
	runner.agentExecutable = "/opt/mindline"
	var tokenOutput, tokenError bytes.Buffer
	if code := runner.Run([]string{"agent", "registration-token"}, &tokenOutput, &tokenError); code != ExitOK {
		t.Fatalf("token code=%d stderr=%s", code, tokenError.String())
	}
	var token map[string]string
	if err := json.Unmarshal(tokenOutput.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	seenBody := ""
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
			SchemaVersion: localservice.CapabilitiesSchemaVersion,
			Features:      []string{localservice.AgentRegistrationCapability},
		})
	})
	mux.HandleFunc("POST /v1/scoped/actors/register", func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		seenBody = string(body)
		var input localservice.AgentRegistrationInput
		if err := json.Unmarshal(body, &input); err != nil ||
			!strings.HasPrefix(input.AgentID, "agent-") || input.Name != "Fresh Cursor agent" {
			t.Fatalf("registration input=%+v err=%v", input, err)
		}
		writeLegacyAgentEnvelope(t, writer, agentstate.AgentActor{
			ID: input.AgentID, Name: input.Name, Status: agentstate.StatusActive,
		})
	})
	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	var stdout, stderr bytes.Buffer
	code := runner.Run([]string{"agent-only", "register", "--name", "Fresh Cursor agent",
		"--retry-token", token["retry_token"], "--config", configPath}, &stdout, &stderr)
	if code != ExitOK || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var receipt agentRegistrationReceipt
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.RegistrationState != "ready" || receipt.AgentID == "" ||
		receipt.AgentName != "Fresh Cursor agent" || receipt.AgentStatus != agentstate.StatusActive ||
		receipt.RetryTokenPersisted || !receipt.ExactReplayOnly ||
		!strings.Contains(receipt.NextDiscoveryCommand, agentcontract.ShellQuote(receipt.AgentID)) ||
		!strings.Contains(receipt.NextDiscoveryCommand, "agent-only discover") ||
		!strings.Contains(receipt.NextDiscoveryCommand, "<same-as-registration>") ||
		strings.Contains(stdout.String(), token["retry_token"]) ||
		strings.Contains(seenBody, token["retry_token"]) || strings.Contains(stdout.String(), configPath) {
		t.Fatalf("receipt=%+v body=%s output=%s", receipt, seenBody, stdout.String())
	}
}

func TestRegisteredAgentIDIsDeterministicAndRequiresRandomToken(t *testing.T) {
	one := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 24))
	two := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 24))
	first, err := registeredAgentID(one)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := registeredAgentID(one)
	if err != nil || replay != first {
		t.Fatalf("replay=%q first=%q err=%v", replay, first, err)
	}
	other, err := registeredAgentID(two)
	if err != nil || other == first {
		t.Fatalf("other=%q first=%q err=%v", other, first, err)
	}
	if _, err := registeredAgentID("short"); err == nil {
		t.Fatal("short registration token was accepted")
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

func TestAgentBuildBindingIsReadOnlyAndStructural(t *testing.T) {
	runner := NewRunner(NewMemoryFS())
	var stdout, stderr bytes.Buffer
	if code := runner.Run([]string{"agent", "build-binding"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var binding localservice.BuildBinding
	if err := json.Unmarshal(stdout.Bytes(), &binding); err != nil {
		t.Fatal(err)
	}
	if binding.SchemaVersion != localservice.BuildBindingSchemaVersion ||
		binding.BuildFingerprint == "" || binding.State != "unavailable" ||
		binding.TreeFingerprint != "" || stderr.Len() != 0 {
		t.Fatalf("unexpected ordinary-build binding: %+v stderr=%q", binding, stderr.String())
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

func TestScopedFeedbackReverseFailuresKeepReverseOperation(t *testing.T) {
	tests := [][]string{
		{"agent", "feedback-reverse", "--scope", "project", "--lens", "product", "--actor", "agent", "--judgment", "judgment", "--idempotency-key", "key"},
		{"agent", "feedback-reverse", "--scope", "project", "--lens", "product", "--agent", "outside", "--actor", "agent", "--judgment", "judgment"},
		{"agent", "feedback-reverse", "--scope", "project", "--lens", "product", "--agent", "outside", "--actor", "agent", "--judgment", "judgment", "--idempotency-key", "key"},
	}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		code := NewRunner(NewMemoryFS()).Run(args, &stdout, &stderr)
		var failure agentContractError
		if code != ExitProcess || stdout.Len() != 0 || json.Unmarshal(stderr.Bytes(), &failure) != nil ||
			failure.Operation != "feedback_reverse" || strings.Contains(stderr.String(), "usage:") {
			t.Fatalf("args=%v code=%d stdout=%s stderr=%s failure=%+v", args, code, stdout.String(), stderr.String(), failure)
		}
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
	var agentOnlyOutput, agentOnlyError bytes.Buffer
	if code := runner.Run([]string{"agent-only", "discover", "--scope", "project", "--lens", "product",
		"--agent", "external", "--config", configPath}, &agentOnlyOutput, &agentOnlyError); code != ExitOK {
		t.Fatalf("agent-only discover code=%d stderr=%s", code, agentOnlyError.String())
	}
	var agentOnlyContract discoveryContract
	if err := json.Unmarshal(agentOnlyOutput.Bytes(), &agentOnlyContract); err != nil ||
		!strings.HasPrefix(agentOnlyContract.Workflow["search_command"], "'/opt/mindline' agent-only search") ||
		strings.Contains(agentOnlyOutput.String(), " agent search") {
		t.Fatalf("agent-only contract=%+v err=%v", agentOnlyContract, err)
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

func TestAgentDiscoverDoesNotRestartAnUnavailableService(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-discover-readonly-")
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
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"agent", "discover",
		"--scope", "project", "--lens", "product", "--agent", "outside-agent",
		"--config", configPath}, &stdout, &stderr)
	if code != ExitProcess || stdout.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var failure agentContractError
	if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil ||
		failure.ErrorCode != "service_unavailable" || !failure.Retryable ||
		failure.RepairAction != "retry_service" {
		t.Fatalf("failure=%+v err=%v", failure, err)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(configPath)
	if err != nil || !bytes.Equal(before, after) || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatalf("read-only discovery mutated runtime configuration: err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(config.RuntimeRoot, "install.json")); !os.IsNotExist(err) {
		t.Fatalf("read-only discovery created an install receipt: %v", err)
	}
}

func TestAgentDiscoverUsesOneBoundedDeadlineForHungService(t *testing.T) {
	for _, hangAt := range []string{"status", "capabilities"} {
		t.Run(hangAt, func(t *testing.T) {
			configPath, closeServer := startHangingDiscoveryServer(t, hangAt)
			defer closeServer()
			runner := NewRunner(NewOSFileSystem())
			runner.agentDiscoveryTimeout = 75 * time.Millisecond
			var stdout, stderr bytes.Buffer
			started := time.Now()
			code := runner.Run([]string{"agent", "discover", "--scope", "project",
				"--lens", "product", "--agent", "outside", "--config", configPath}, &stdout, &stderr)
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("discovery exceeded bounded deadline: %s", elapsed)
			}
			var failure agentContractError
			if code != ExitProcess || stdout.Len() != 0 ||
				json.Unmarshal(stderr.Bytes(), &failure) != nil ||
				failure.Operation != "discover" || failure.ErrorCode != "service_unavailable" ||
				!failure.Retryable {
				t.Fatalf("code=%d stdout=%s failure=%+v stderr=%s", code, stdout.String(), failure, stderr.String())
			}
		})
	}
}

func TestOrdinaryAgentReadinessUsesCallerDeadlineForHungService(t *testing.T) {
	configPath, closeServer := startHangingDiscoveryServer(t, "status")
	defer closeServer()
	config, err := localservice.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = waitForAgentReady(ctx, localservice.NewClient(config.SocketPath))
	if elapsed := time.Since(started); err == nil || elapsed > time.Second {
		t.Fatalf("ordinary readiness did not honor its deadline: elapsed=%s err=%v", elapsed, err)
	}
}

func startHangingDiscoveryServer(t *testing.T, hangAt string) (string, func()) {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "mindline-discovery-deadline-")
	if err != nil {
		t.Fatal(err)
	}
	config, err := localservice.ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
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
	mux.HandleFunc("GET /v1/status", func(writer http.ResponseWriter, request *http.Request) {
		if hangAt == "status" {
			<-request.Context().Done()
			return
		}
		writeLegacyAgentEnvelope(t, writer, localservice.Status{SchemaVersion: localservice.APISchemaVersion, ServiceState: "ready"})
	})
	mux.HandleFunc("GET /v1/capabilities", func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	})
	server := &http.Server{Handler: mux}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	return configPath, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		<-result
		_ = os.RemoveAll(root)
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
		ScopedRuns        int                     `json:"scoped_runs"`
		ScopedJudgments   int                     `json:"scoped_judgments"`
		Scopes            []agentstate.Scope      `json:"scopes"`
		Lenses            []agentstate.ScopedLens `json:"lenses"`
		Actors            []agentstate.AgentActor `json:"actors"`
	}{
		MemoryRevision: status.Memory.Revision, MemoryFingerprint: status.Memory.Fingerprint,
		ScopedRuns: status.State.ScopedRetrievalRunCount, ScopedJudgments: status.State.ScopedJudgmentCount,
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
