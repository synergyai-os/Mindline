package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func TestAgentOnlySurfaceExposesGovernedRoutesAndRejectsOwnerRoutes(t *testing.T) {
	runner := NewRunner(NewMemoryFS())
	runner.agentExecutable = "/opt/mindline"
	var help, helpError bytes.Buffer
	if code := runner.Run([]string{"agent-only", "help"}, &help, &helpError); code != ExitOK {
		t.Fatalf("help code=%d stderr=%s", code, helpError.String())
	}
	for _, expected := range []string{
		"'/opt/mindline' agent-only registration-token",
		"'/opt/mindline' agent-only discover",
		"'/opt/mindline' agent-only search",
		"'/opt/mindline' agent-only get",
		"'/opt/mindline' agent-only feedback",
	} {
		if !strings.Contains(help.String(), expected) {
			t.Fatalf("agent-only help missing %q: %s", expected, help.String())
		}
	}
	for _, forbidden := range []string{"scope-list", "lens-list", "actor-list", "agent install"} {
		if strings.Contains(help.String(), forbidden) {
			t.Fatalf("agent-only help exposed %q: %s", forbidden, help.String())
		}
	}

	blocked := [][]string{
		{"agent-only", "scope-list"},
		{"agent-only", "search", "query"},
		{"agent-only", "get", "record"},
		{"agent-only", "install"},
		{"agent-only", "feedback", "--run", "run", "--scope", "scope", "--lens", "lens", "--agent", "agent", "--record", "record", "--actor", "owner", "--disposition", "used", "--retry-token", "token"},
	}
	for _, args := range blocked {
		var stdout, stderr bytes.Buffer
		if code := runner.Run(args, &stdout, &stderr); code != ExitProcess {
			t.Fatalf("args=%v code=%d stderr=%s", args, code, stderr.String())
		}
		var failure agentContractError
		if json.Unmarshal(stderr.Bytes(), &failure) != nil || failure.ErrorCode != "route_not_available" ||
			strings.Contains(stderr.String(), "usage:") || stdout.Len() != 0 {
			t.Fatalf("args=%v stdout=%s stderr=%s", args, stdout.String(), stderr.String())
		}
	}
}

func TestAgentOnlyCapabilitiesExposeOnlyScopedNamespacedRoute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
			SchemaVersion:             localservice.CapabilitiesSchemaVersion,
			SearchFormats:             []string{"mindline-agent-context-packet/v0.2", "mindline-agent-context-packet/v0.3"},
			CompactSearchEndpoint:     "/v1/search/compact",
			ExplicitHydrationCommand:  "mindline agent get <record>",
			FeedbackRetryToken:        true,
			Features:                  []string{localservice.ScopedRecallCapability},
			ScopedSearchEndpoint:      "/v1/scoped/search/compact",
			ScopedFeedbackEndpoint:    "/v1/scoped/judgments",
			ScopedHydrationEndpoint:   localservice.ScopedHydrationEndpoint,
			AgentRegistrationEndpoint: "/v1/scoped/actors/register",
			RecommendedAgentRoute:     localservice.RecommendedAgentRoute,
			OwnerDebugRouteClass:      localservice.OwnerDebugRouteClass,
			IdentityAssurance:         "declared_local_actor",
			OwnerMutationEnforcement:  "cooperative",
			FeedbackTokenCommand:      "agent feedback-token",
			RegistrationTokenCommand:  "agent registration-token",
		})
	})
	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	runner := NewRunner(NewOSFileSystem())
	runner.agentExecutable = "/opt/mindline"
	var stdout, stderr bytes.Buffer
	if code := runner.Run([]string{"agent-only", "capabilities", "--config", configPath}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var capabilities localservice.Capabilities
	if err := json.Unmarshal(stdout.Bytes(), &capabilities); err != nil {
		t.Fatal(err)
	}
	serialized := stdout.String()
	if capabilities.CompactSearchEndpoint != "" || capabilities.OwnerDebugRouteClass != "" ||
		len(capabilities.SearchFormats) != 1 || capabilities.SearchFormats[0] != personalmemory.ScopedCompactPacketSchemaVersion ||
		!strings.Contains(capabilities.ExplicitHydrationCommand, "agent-only get") ||
		!strings.Contains(capabilities.FeedbackTokenCommand, "agent-only feedback-token") ||
		!strings.Contains(capabilities.RegistrationTokenCommand, "agent-only registration-token") ||
		strings.Contains(serialized, "/v1/search/compact") || strings.Contains(serialized, " agent get") ||
		strings.Contains(serialized, "owner_debug_ungated") {
		t.Fatalf("capabilities=%+v serialized=%s", capabilities, serialized)
	}
}

func TestAgentOnlySearchReturnsRecordedAuditAndExactNextActions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
			SchemaVersion: localservice.CapabilitiesSchemaVersion,
			Features:      []string{localservice.ScopedRecallCapability},
		})
	})
	mux.HandleFunc("POST /v1/scoped/search/compact", func(writer http.ResponseWriter, request *http.Request) {
		writeLegacyAgentEnvelope(t, writer, personalmemory.CompactContextPacket{
			SchemaVersion: personalmemory.ScopedCompactPacketSchemaVersion,
			RunID:         "run-1", Query: "team accountability", ScopeID: "project-1",
			LensID: "governance", AgentID: "agent-1", AuditState: "recorded",
			AnswerState: "answered", Citations: []personalmemory.CompactCitation{{RecordID: "record-1"}},
		})
	})
	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	runner := NewRunner(NewOSFileSystem())
	runner.agentExecutable = "/opt/mindline"
	var stdout, stderr bytes.Buffer
	code := runner.Run([]string{
		"agent-only", "search", "team", "accountability", "--scope", "project-1",
		"--lens", "governance", "--agent", "agent-1", "--format", "compact-scoped-v0.4",
		"--config", configPath,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var packet personalmemory.CompactContextPacket
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil {
		t.Fatal(err)
	}
	if packet.AuditState != "recorded" || packet.NextActions == nil ||
		packet.NextActions.State != "select_citations" || packet.NextActions.AbstentionTerminal ||
		!strings.Contains(packet.NextActions.HydrateSelectedCommand, "agent-only get <record>") ||
		!strings.Contains(packet.NextActions.HydrateSelectedCommand, "'run-1'") ||
		!strings.Contains(packet.NextActions.FeedbackCommand, "agent-only feedback") ||
		!strings.Contains(packet.NextActions.FeedbackCommand, "'agent-1'") {
		t.Fatalf("packet=%+v", packet)
	}
}

func TestAgentOnlySearchRejectsServiceWithoutRecordedAuditReceipt(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
			SchemaVersion: localservice.CapabilitiesSchemaVersion,
			Features:      []string{localservice.ScopedRecallCapability},
		})
	})
	mux.HandleFunc("POST /v1/scoped/search/compact", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, personalmemory.CompactContextPacket{
			SchemaVersion: personalmemory.ScopedCompactPacketSchemaVersion,
			RunID:         "run-1", ScopeID: "project-1", LensID: "governance", AgentID: "agent-1",
			AnswerState: "abstained", Citations: []personalmemory.CompactCitation{},
		})
	})
	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"agent-only", "search", "missing", "--scope", "project-1", "--lens", "governance",
		"--agent", "agent-1", "--format", "compact-scoped-v0.4", "--config", configPath,
	}, &stdout, &stderr)
	if code != ExitProcess || !strings.Contains(stderr.String(), "audit_receipt_unavailable") || stdout.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestAgentOnlyAbstentionIsRecordedAndHasNoHydrationEscape(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capabilities", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, localservice.Capabilities{
			SchemaVersion: localservice.CapabilitiesSchemaVersion,
			Features:      []string{localservice.ScopedRecallCapability},
		})
	})
	mux.HandleFunc("POST /v1/scoped/search/compact", func(writer http.ResponseWriter, _ *http.Request) {
		writeLegacyAgentEnvelope(t, writer, personalmemory.CompactContextPacket{
			SchemaVersion: personalmemory.ScopedCompactPacketSchemaVersion,
			RunID:         "run-empty", Query: "absent topic", ScopeID: "project-1", LensID: "governance",
			AgentID: "agent-1", AuditState: "recorded", AnswerState: "abstained",
			AbstentionReason: "no_retrieval_candidates",
			AbstentionDiagnostics: &personalmemory.AbstentionDiagnostics{
				Classification: "below_evidence_threshold", RankedCandidateCount: 10,
			},
			Citations: []personalmemory.CompactCitation{},
		})
	})
	configPath, closeServer := startScopedAgentCLITestServer(t, mux)
	defer closeServer()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"agent-only", "search", "absent", "topic", "--scope", "project-1", "--lens", "governance",
		"--agent", "agent-1", "--format", "compact-scoped-v0.4", "--config", configPath,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var packet personalmemory.CompactContextPacket
	if err := json.Unmarshal(stdout.Bytes(), &packet); err != nil {
		t.Fatal(err)
	}
	if packet.AuditState != "recorded" || packet.AnswerState != "abstained" ||
		packet.NextActions == nil || !packet.NextActions.AbstentionTerminal ||
		packet.NextActions.State != "stop_or_new_query_same_binding" ||
		packet.NextActions.HydrateSelectedCommand != "" || packet.NextActions.FeedbackCommand != "" ||
		packet.NextActions.NewQueryRule != "different_query_same_binding_only" {
		t.Fatalf("packet=%+v", packet)
	}
}
