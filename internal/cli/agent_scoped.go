package cli

import (
	"context"
	"io"

	"github.com/synergyai-os/Mindline/internal/agentstate"
)

func (r Runner) runAgentScopeList(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	scopes, err := client.ListScopes(context.Background())
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, scopes)
}

func (r Runner) runAgentScopePut(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 1 ||
		!onlyAgentKeys(options.values, "name", "purpose") ||
		options.values["name"] == "" || options.values["purpose"] == "" {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	scope, err := client.PutScope(context.Background(), agentstate.Scope{
		ID: options.positionals[0], Name: options.values["name"], Purpose: options.values["purpose"],
	})
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, scope)
}

func (r Runner) runAgentScopeArchive(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 1 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	scope, err := client.ArchiveScope(context.Background(), options.positionals[0])
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, scope)
}

func (r Runner) runAgentLensArchive(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 1 ||
		!onlyAgentKeys(options.values, "scope") || options.values["scope"] == "" {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	lens, err := client.ArchiveScopedLens(
		context.Background(), options.values["scope"], options.positionals[0],
	)
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, lens)
}

func (r Runner) runAgentActorList(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	actors, err := client.ListActors(context.Background())
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, actors)
}

func (r Runner) runAgentActorPut(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 1 ||
		!onlyAgentKeys(options.values, "name") || options.values["name"] == "" {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	actor, err := client.PutActor(context.Background(), agentstate.AgentActor{
		ID: options.positionals[0], Name: options.values["name"],
	})
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, actor)
}

func (r Runner) runAgentActorArchive(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 1 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	actor, err := client.ArchiveActor(context.Background(), options.positionals[0])
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, actor)
}
