package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentcontract"
	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func (r Runner) runAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help":
		return r.writeAgentHelp(stdout)
	case "discover":
		return r.runAgentDiscover(args[1:], stdout, stderr)
	case "feedback-token":
		return r.runAgentFeedbackToken(args[1:], stdout, stderr)
	case "connection-handle":
		return r.runAgentConnectionHandle(args[1:], stdout, stderr)
	case "connection-bind":
		return r.runAgentConnectionBind(args[1:], stdout, stderr)
	case "connection-archive":
		return r.runAgentConnectionArchive(args[1:], stdout, stderr)
	case "build-binding":
		return r.runAgentBuildBinding(args[1:], stdout, stderr)
	case "registration-token":
		return r.runAgentRegistrationToken(args[1:], stdout, stderr)
	case "register":
		return r.runAgentRegister(args[1:], stdout, stderr)
	case "install":
		return r.runAgentInstall(args[1:], stdout, stderr)
	case "restart":
		return r.runAgentRestart(args[1:], stdout, stderr)
	case "rollback":
		return r.runAgentRollback(args[1:], stdout, stderr)
	case "uninstall":
		return r.runAgentUninstall(args[1:], stdout, stderr)
	case "status":
		return r.runAgentStatus(args[1:], stdout, stderr)
	case "capabilities":
		return r.runAgentCapabilities(args[1:], stdout, stderr)
	case "search":
		return r.runAgentSearch(args[1:], stdout, stderr)
	case "get":
		return r.runAgentGet(args[1:], stdout, stderr)
	case "scope-list":
		return r.runAgentScopeList(args[1:], stdout, stderr)
	case "scope-put":
		return r.runAgentScopePut(args[1:], stdout, stderr)
	case "scope-archive":
		return r.runAgentScopeArchive(args[1:], stdout, stderr)
	case "lens-list":
		return r.runAgentLensList(args[1:], stdout, stderr)
	case "lens-put":
		return r.runAgentLensPut(args[1:], stdout, stderr)
	case "lens-archive":
		return r.runAgentLensArchive(args[1:], stdout, stderr)
	case "lens-delete":
		return r.runAgentLensDelete(args[1:], stdout, stderr)
	case "actor-list":
		return r.runAgentActorList(args[1:], stdout, stderr)
	case "actor-put":
		return r.runAgentActorPut(args[1:], stdout, stderr)
	case "actor-archive":
		return r.runAgentActorArchive(args[1:], stdout, stderr)
	case "feedback":
		return r.runAgentFeedback(args[1:], stdout, stderr, false)
	case "feedback-reverse":
		return r.runAgentFeedback(args[1:], stdout, stderr, true)
	case "service-run":
		return r.runAgentService(args[1:], stdout, stderr)
	default:
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
}

func (r Runner) runAgentBuildBinding(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		return agentUsage(stderr)
	}
	return encodePersonalMemoryJSON(stdout, stderr, localservice.BuildBindingFor(r.agentExecutable))
}

func (r Runner) runAgentStatus(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	status, err := client.Status(context.Background())
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, status)
}

func (r Runner) runAgentCapabilities(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	capabilities, err := client.Capabilities(context.Background())
	if err != nil {
		return agentFailure(stderr, err)
	}
	if r.agentNamespace == "agent-only" {
		capabilities = r.agentOnlyCapabilities(capabilities)
	}
	return encodePersonalMemoryJSON(stdout, stderr, capabilities)
}

func (r Runner) agentOnlyCapabilities(capabilities localservice.Capabilities) localservice.Capabilities {
	workflow := r.agentWorkflow("")
	return localservice.Capabilities{
		SchemaVersion:                capabilities.SchemaVersion,
		SearchFormats:                []string{personalmemory.ScopedCompactPacketSchemaVersion},
		CompactAbstentionPolicy:      capabilities.CompactAbstentionPolicy,
		ExplicitHydrationCommand:     workflow.Get,
		FeedbackRetryToken:           capabilities.FeedbackRetryToken,
		Features:                     capabilities.Features,
		ScopedSearchEndpoint:         capabilities.ScopedSearchEndpoint,
		ScopedFeedbackEndpoint:       capabilities.ScopedFeedbackEndpoint,
		ScopedHydrationEndpoint:      capabilities.ScopedHydrationEndpoint,
		AgentRegistrationEndpoint:    capabilities.AgentRegistrationEndpoint,
		RecommendedAgentRoute:        capabilities.RecommendedAgentRoute,
		IdentityAssurance:            capabilities.IdentityAssurance,
		HostileProcessAuthentication: capabilities.HostileProcessAuthentication,
		OwnerMutationEnforcement:     capabilities.OwnerMutationEnforcement,
		FeedbackTokenCommand:         workflow.FeedbackToken,
		RegistrationTokenCommand:     workflow.RegistrationToken,
		ProjectConnectionEndpoint:    capabilities.ProjectConnectionEndpoint,
	}
}

func (r Runner) runAgentSearch(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) == 0 ||
		!onlyAgentKeys(options.values, "scope", "lens", "agent", "limit", "format") {
		return agentUsage(stderr)
	}
	limit := 10
	if value := options.values["limit"]; value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			return agentUsage(stderr)
		}
	}
	format := options.values["format"]
	scoped := options.values["scope"] != "" || options.values["agent"] != "" ||
		format == "compact-scoped-v0.4"
	if scoped {
		if format != "compact-scoped-v0.4" || options.values["scope"] == "" ||
			options.values["lens"] == "" || options.values["agent"] == "" {
			return agentUsage(stderr)
		}
		client, err := agentClient(options.configPath)
		if err != nil {
			return agentFailure(stderr, err)
		}
		capabilities, err := client.Capabilities(context.Background())
		if err != nil {
			return agentFailure(stderr, err)
		}
		if !supportsSearchFormat(capabilities.Features, localservice.ScopedRecallCapability) {
			return agentFailure(stderr, errors.New("local agent service does not support scoped recall v0.4"))
		}
		packet, err := client.SearchScoped(context.Background(), localservice.ScopedSearchInput{
			Query: strings.Join(options.positionals, " "), ScopeID: options.values["scope"],
			LensID: options.values["lens"], AgentID: options.values["agent"], Limit: limit,
		})
		if err != nil {
			return agentFailure(stderr, err)
		}
		if r.agentNamespace == "agent-only" && packet.AuditState != "recorded" {
			return writeAgentContractError(
				stderr, "scoped_search", "audit_receipt_unavailable", false, "upgrade_mindline",
			)
		}
		packet.NextActions = r.scopedNextActions(packet, options.configPath)
		return encodePersonalMemoryJSON(stdout, stderr, packet)
	}
	if format != "" && format != "legacy-v0.2" && format != "compact-v0.3" {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	input := localservice.SearchInput{
		Query: strings.Join(options.positionals, " "), LensID: options.values["lens"], Limit: limit,
	}
	if format == "legacy-v0.2" {
		packet, err := client.Search(context.Background(), input)
		if err != nil {
			return agentFailure(stderr, err)
		}
		return encodePersonalMemoryJSON(stdout, stderr, packet)
	}
	capabilities, capabilityErr := client.Capabilities(context.Background())
	if capabilityErr == nil && supportsSearchFormat(
		capabilities.SearchFormats, "mindline-agent-context-packet/v0.3",
	) {
		packet, compactErr := client.SearchCompact(context.Background(), input)
		if compactErr != nil {
			return agentFailure(stderr, compactErr)
		}
		return encodePersonalMemoryJSON(stdout, stderr, packet)
	}
	packet, err := client.Search(context.Background(), input)
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, packet)
}

func (r Runner) scopedNextActions(
	packet personalmemory.CompactContextPacket, configPath string,
) *personalmemory.AgentNextActions {
	workflow := r.agentWorkflow(configPath)
	bind := func(command string) string {
		return strings.NewReplacer(
			"<run>", agentcontract.ShellQuote(packet.RunID),
			"<scope>", agentcontract.ShellQuote(packet.ScopeID),
			"<lens>", agentcontract.ShellQuote(packet.LensID),
			"<actor>", agentcontract.ShellQuote(packet.AgentID),
		).Replace(command)
	}
	actions := &personalmemory.AgentNextActions{
		State: "select_citations", NewQueryRule: "different_query_same_binding_only",
		ForbiddenFallbacks: []string{"memory search", "memory get", "unscoped agent search", "unscoped agent get"},
	}
	if packet.AnswerState == "abstained" {
		actions.State = "stop_or_new_query_same_binding"
		actions.AbstentionTerminal = true
		return actions
	}
	actions.HydrateSelectedCommand = bind(workflow.Get)
	actions.FeedbackTokenCommand = workflow.FeedbackToken
	actions.FeedbackCommand = bind(workflow.Feedback)
	return actions
}

func supportsSearchFormat(formats []string, expected string) bool {
	for _, format := range formats {
		if format == expected {
			return true
		}
	}
	return false
}

func (r Runner) runAgentGet(args []string, stdout, stderr io.Writer) int {
	scopedIntent := hasAgentOption(args, "run", "scope", "lens", "agent")
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 1 ||
		!onlyAgentKeys(options.values, "run", "scope", "lens", "agent") {
		if scopedIntent {
			return writeAgentContractError(stderr, "scoped_get", "invalid_scoped_command", false, "use_discovery_template")
		}
		return agentUsage(stderr)
	}
	scoped := options.values["run"] != "" || options.values["scope"] != "" ||
		options.values["lens"] != "" || options.values["agent"] != ""
	if scoped && (options.values["run"] == "" || options.values["scope"] == "" ||
		options.values["lens"] == "" || options.values["agent"] == "") {
		return writeAgentContractError(stderr, "scoped_get", "incomplete_binding", false, "request_owner_binding")
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		if scoped {
			return writeAgentContractError(stderr, "scoped_get", "service_unavailable", true, "retry_service")
		}
		return agentFailure(stderr, err)
	}
	var capture personalmemory.HydratedCapture
	if scoped {
		capture, err = client.GetScoped(context.Background(), localservice.ScopedGetInput{
			RunID: options.values["run"], ScopeID: options.values["scope"],
			LensID: options.values["lens"], AgentID: options.values["agent"],
			RecordID: options.positionals[0],
		})
	} else {
		capture, err = client.Get(context.Background(), options.positionals[0])
		capture.RouteClass = localservice.OwnerDebugRouteClass
		capture.AgentRecallApproved = false
	}
	if err != nil {
		if scoped {
			return writeAgentContractError(stderr, "scoped_get", "scoped_hydration_rejected", false, "rerun_scoped_search")
		}
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, capture)
}

func (r Runner) runAgentLensList(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 || !onlyAgentKeys(options.values, "scope") {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	lenses, err := client.ListScopedLenses(context.Background(), options.values["scope"])
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, lenses)
}

func (r Runner) runAgentLensPut(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 1 ||
		!onlyAgentKeys(options.values, "scope", "name", "query") ||
		options.values["scope"] == "" || options.values["name"] == "" ||
		options.values["query"] == "" {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	lens, err := client.PutScopedLens(context.Background(), agentstate.ScopedLens{
		ScopeID: options.values["scope"], ID: options.positionals[0],
		Name: options.values["name"], Query: options.values["query"],
	})
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, lens)
}

func (r Runner) runAgentLensDelete(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 1 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	result, err := client.DeleteLens(context.Background(), options.positionals[0])
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, result)
}

func agentClient(configPath string) (*localservice.Client, error) {
	config, err := localservice.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	client := localservice.NewClient(config.SocketPath)
	initialContext, cancelInitial := context.WithTimeout(context.Background(), time.Second)
	_, initialErr := client.Status(initialContext)
	cancelInitial()
	if initialErr == nil {
		return client, nil
	}
	if _, restartErr := localservice.Restart(configPath); restartErr != nil {
		return nil, fmt.Errorf("local agent service is unavailable; restart failed: %w", restartErr)
	}
	recoveryContext, cancelRecovery := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRecovery()
	if err := waitForAgentReady(recoveryContext, client); err == nil {
		return client, nil
	}
	return nil, errors.New("local agent service is unavailable after one restart attempt")
}

var restartAgentServiceWithin = localservice.RestartContext

func agentClientWithin(ctx context.Context, configPath string) (*localservice.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	config, err := localservice.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	client := localservice.NewClient(config.SocketPath)
	initialContext, cancelInitial := context.WithTimeout(ctx, time.Second)
	_, initialErr := client.Status(initialContext)
	cancelInitial()
	if initialErr == nil {
		return client, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, restartErr := restartAgentServiceWithin(ctx, configPath); restartErr != nil {
		return nil, fmt.Errorf("local agent service is unavailable; restart failed: %w", restartErr)
	}
	if err := waitForAgentReady(ctx, client); err == nil {
		return client, nil
	}
	return nil, errors.New("local agent service is unavailable after one restart attempt")
}

func waitForAgentReady(ctx context.Context, client *localservice.Client) error {
	for {
		if _, err := client.Status(ctx); err == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func readOnlyAgentClient(ctx context.Context, configPath string) (*localservice.Client, error) {
	config, err := localservice.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	client := localservice.NewClient(config.SocketPath)
	if _, err := client.Status(ctx); err != nil {
		return nil, errors.New("local agent service is unavailable")
	}
	return client, nil
}

func agentUsage(stderr io.Writer) int {
	fmt.Fprint(stderr, usage)
	return ExitUsage
}

func agentFailure(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "mindline agent: %v\n", err)
	return ExitProcess
}
