package cli

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentcontract"
	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

type agentContractError struct {
	SchemaVersion string `json:"schema_version"`
	ErrorCode     string `json:"error_code"`
	Operation     string `json:"operation"`
	Retryable     bool   `json:"retryable"`
	RepairAction  string `json:"repair_action"`
}

func writeAgentContractError(
	stderr io.Writer, operation, code string, retryable bool, repairAction string,
) int {
	_ = json.NewEncoder(stderr).Encode(agentContractError{
		SchemaVersion: "mindline-agent-error/v0.1", ErrorCode: code,
		Operation: operation, Retryable: retryable, RepairAction: repairAction,
	})
	return ExitProcess
}

type discoveryBinding struct {
	ScopeID   string `json:"scope_id"`
	LensID    string `json:"lens_id"`
	AgentID   string `json:"agent_id"`
	ScopeName string `json:"scope_name"`
	LensName  string `json:"lens_name"`
	AgentName string `json:"agent_name"`
}

type discoveryContract struct {
	SchemaVersion  string            `json:"schema_version"`
	DiscoveryState string            `json:"discovery_state"`
	ApprovedRoute  string            `json:"approved_route"`
	Config         map[string]string `json:"config"`
	Binding        discoveryBinding  `json:"binding"`
	Trust          map[string]any    `json:"trust"`
	Workflow       map[string]string `json:"workflow"`
	Policy         map[string]any    `json:"policy"`
	Feedback       map[string]any    `json:"feedback"`
}

func (r Runner) runAgentDiscover(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 ||
		!onlyAgentKeys(options.values, "scope", "lens", "agent") ||
		options.values["scope"] == "" || options.values["lens"] == "" ||
		options.values["agent"] == "" {
		return writeAgentContractError(stderr, "discover", "incomplete_binding", false, "request_owner_binding")
	}
	timeout := r.agentDiscoveryTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client, err := readOnlyAgentClient(ctx, options.configPath)
	if err != nil {
		return writeAgentContractError(stderr, "discover", "service_unavailable", true, "retry_service")
	}
	capabilities, err := client.Capabilities(ctx)
	if err != nil {
		return writeAgentContractError(stderr, "discover", "service_unavailable", true, "retry_service")
	}
	if !supportsSearchFormat(capabilities.Features, localservice.DiscoveryCapability) {
		return writeAgentContractError(stderr, "discover", "capability_unavailable", false, "upgrade_mindline")
	}
	scopes, err := client.ListScopes(ctx)
	if err != nil {
		return writeAgentContractError(stderr, "discover", "service_unavailable", true, "retry_service")
	}
	actors, err := client.ListActors(ctx)
	if err != nil {
		return writeAgentContractError(stderr, "discover", "service_unavailable", true, "retry_service")
	}
	lenses, err := client.ListScopedLenses(ctx, options.values["scope"])
	if err != nil {
		return writeAgentContractError(stderr, "discover", "service_unavailable", true, "retry_service")
	}
	scope, ok := findScope(scopes, options.values["scope"])
	if !ok {
		return writeAgentContractError(stderr, "discover", "binding_not_found", false, "request_owner_binding")
	}
	if scope.Status != agentstate.StatusActive {
		return writeAgentContractError(stderr, "discover", "binding_archived", false, "request_owner_binding")
	}
	lens, ok := findScopedLens(lenses, options.values["lens"])
	if !ok {
		return writeAgentContractError(stderr, "discover", "binding_not_found", false, "request_owner_binding")
	}
	if lens.Status != agentstate.StatusActive {
		return writeAgentContractError(stderr, "discover", "binding_archived", false, "request_owner_binding")
	}
	actor, ok := findActor(actors, options.values["agent"])
	if !ok {
		return writeAgentContractError(stderr, "discover", "binding_not_found", false, "request_owner_binding")
	}
	if actor.Status != agentstate.StatusActive {
		return writeAgentContractError(stderr, "discover", "binding_archived", false, "request_owner_binding")
	}
	configMode, configPath := "default", ""
	if strings.TrimSpace(options.configPath) != "" {
		configMode = "explicit"
		configPath = "<same-as-discovery>"
	}
	workflow := agentcontract.NewWorkflow(r.agentExecutable, configPath)
	contract := discoveryContract{
		SchemaVersion: "mindline-agent-discovery/v0.1", DiscoveryState: "ready",
		ApprovedRoute: localservice.RecommendedAgentRoute,
		Config:        map[string]string{"mode": configMode, "propagation": "reuse_discovery_argument_for_every_service_command"},
		Binding: discoveryBinding{ScopeID: scope.ID, LensID: lens.ID, AgentID: actor.ID,
			ScopeName: scope.Name, LensName: lens.Name, AgentName: actor.Name},
		Trust: map[string]any{"identity_assurance": agentcontract.IdentityAssurance,
			"hostile_process_authentication": false, "owner_mutation_enforcement": agentcontract.MutationEnforcement},
		Workflow: map[string]string{
			"search_format":            "compact-scoped-v0.4",
			"search_command":           workflow.Search,
			"get_command":              workflow.Get,
			"feedback_token_command":   workflow.FeedbackToken,
			"feedback_command":         workflow.Feedback,
			"feedback_reverse_command": workflow.FeedbackReverse,
		},
		Policy: map[string]any{"abstention_is_terminal": true, "selective_hydration_only": true,
			"memory_fallback_allowed": false, "authority_class": personalmemory.AuthorityClass},
		Feedback: map[string]any{"retry_token_owner": "caller", "retry_token_minimum_random_bits": 128,
			"identical_retry_replays": true, "conflicting_reuse_rejected": true,
			"agent_effect_scope": "exact_scope_lens_agent", "new_event_requires_new_token": true},
	}
	return encodePersonalMemoryJSON(stdout, stderr, contract)
}

func findScope(items []agentstate.Scope, id string) (agentstate.Scope, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return agentstate.Scope{}, false
}

func findScopedLens(items []agentstate.ScopedLens, id string) (agentstate.ScopedLens, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return agentstate.ScopedLens{}, false
}

func findActor(items []agentstate.AgentActor, id string) (agentstate.AgentActor, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return agentstate.AgentActor{}, false
}

func (r Runner) runAgentFeedbackToken(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		return agentUsage(stderr)
	}
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return writeAgentContractError(stderr, "feedback_token", "token_generation_failed", true, "retry")
	}
	return encodePersonalMemoryJSON(stdout, stderr, map[string]string{
		"schema_version": "mindline-feedback-token/v0.1",
		"retry_token":    base64.RawURLEncoding.EncodeToString(value),
		"owner":          "caller", "reuse": "identical_retry_only",
	})
}

func (r Runner) writeAgentHelp(stdout io.Writer) int {
	_, _ = io.WriteString(stdout, agentcontract.HelpText(r.agentExecutable))
	return ExitOK
}
