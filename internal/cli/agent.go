package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/localservice"
)

func (r Runner) runAgent(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	switch args[0] {
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
	return encodePersonalMemoryJSON(stdout, stderr, capabilities)
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

func supportsSearchFormat(formats []string, expected string) bool {
	for _, format := range formats {
		if format == expected {
			return true
		}
	}
	return false
}

func (r Runner) runAgentGet(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 1 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	capture, err := client.Get(context.Background(), options.positionals[0])
	if err != nil {
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
	if _, err := client.Status(context.Background()); err == nil {
		return client, nil
	}
	if _, restartErr := localservice.Restart(configPath); restartErr != nil {
		return nil, fmt.Errorf("local agent service is unavailable; restart failed: %w", restartErr)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Status(context.Background()); err == nil {
			return client, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, errors.New("local agent service is unavailable after one restart attempt")
}

func agentUsage(stderr io.Writer) int {
	fmt.Fprint(stderr, usage)
	return ExitUsage
}

func agentFailure(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "mindline agent: %v\n", err)
	return ExitProcess
}
