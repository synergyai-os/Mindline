package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/localservice"
)

func (r Runner) runAgentFeedback(args []string, stdout, stderr io.Writer, reversal bool) int {
	scopedIntent := hasAgentOption(args, "scope", "agent") ||
		hasAgentOptionValue(args, "actor", agentstate.FeedbackOwner)
	options, err := parseAgentOptions(args)
	allowed := []string{"actor", "idempotency-key", "retry-token", "reason"}
	if !reversal {
		allowed = append(allowed, "run", "scope", "lens", "agent", "record", "disposition")
	} else {
		allowed = append(allowed, "judgment", "scope", "lens", "agent")
	}
	if err != nil || len(options.positionals) != 0 || !onlyAgentKeys(options.values, allowed...) ||
		options.values["actor"] == "" {
		if scopedIntent {
			operation := "feedback"
			if reversal {
				operation = "feedback_reverse"
			}
			return writeAgentContractError(stderr, operation, "invalid_scoped_command", false, "use_discovery_template")
		}
		return agentUsage(stderr)
	}
	explicitKey := options.values["idempotency-key"]
	retryToken := options.values["retry-token"]
	scoped := options.values["scope"] != "" || options.values["agent"] != "" ||
		options.values["actor"] == agentstate.FeedbackOwner
	if scoped {
		return r.runAgentScopedFeedback(options, stdout, stderr, reversal)
	}
	if reversal {
		if explicitKey == "" || retryToken != "" {
			return agentUsage(stderr)
		}
	} else if (explicitKey == "") == (retryToken == "") {
		return agentUsage(stderr)
	}
	if options.values["actor"] != "user" && options.values["actor"] != "agent" {
		return agentUsage(stderr)
	}
	input := agentstate.JudgmentRequest{
		IdempotencyKey: explicitKey, RetryToken: retryToken,
		Actor: options.values["actor"], Reason: options.values["reason"],
	}
	if reversal {
		input.ReversesID = options.values["judgment"]
		if input.ReversesID == "" {
			return agentUsage(stderr)
		}
	} else {
		input.RunID = options.values["run"]
		input.LensID = options.values["lens"]
		input.RecordID = options.values["record"]
		input.Disposition = options.values["disposition"]
		if input.RunID == "" || input.LensID == "" || input.RecordID == "" || input.Disposition == "" {
			return agentUsage(stderr)
		}
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	judgment, err := client.ApplyJudgment(context.Background(), input)
	if err != nil {
		return agentFailure(stderr, err)
	}
	return encodePersonalMemoryJSON(stdout, stderr, judgment)
}

func (r Runner) runAgentScopedFeedback(
	options agentOptions,
	stdout, stderr io.Writer,
	reversal bool,
) int {
	operation := "feedback"
	if reversal {
		operation = "feedback_reverse"
	}
	actor := options.values["actor"]
	agentID := options.values["agent"]
	if options.values["scope"] == "" || options.values["lens"] == "" ||
		(actor != agentstate.FeedbackOwner && actor != agentstate.FeedbackAgent) ||
		(actor == agentstate.FeedbackOwner && agentID != "") ||
		(actor == agentstate.FeedbackAgent && agentID == "") {
		return writeAgentContractError(stderr, operation, "incomplete_binding", false, "request_owner_binding")
	}
	input := agentstate.ScopedJudgmentRequest{
		ScopeID: options.values["scope"], LensID: options.values["lens"],
		AgentID: agentID, Actor: actor, Reason: options.values["reason"],
	}
	if reversal {
		if options.values["idempotency-key"] == "" || options.values["retry-token"] != "" ||
			options.values["judgment"] == "" {
			return writeAgentContractError(stderr, operation, "invalid_feedback_event", false, "create_new_event_key")
		}
		input.IdempotencyKey = options.values["idempotency-key"]
		input.ReversesID = options.values["judgment"]
	} else {
		if options.values["idempotency-key"] != "" || options.values["retry-token"] == "" ||
			options.values["run"] == "" || options.values["record"] == "" ||
			options.values["disposition"] == "" {
			return writeAgentContractError(stderr, operation, "invalid_feedback_event", false, "create_feedback_token")
		}
		input.RetryToken = options.values["retry-token"]
		input.RunID = options.values["run"]
		input.RecordID = options.values["record"]
		input.Disposition = options.values["disposition"]
	}
	client, err := agentClient(options.configPath)
	if err != nil {
		return writeAgentContractError(stderr, operation, "service_unavailable", true, "retry_service")
	}
	capabilities, err := client.Capabilities(context.Background())
	if err != nil {
		return writeAgentContractError(stderr, operation, "service_unavailable", true, "retry_service")
	}
	if !supportsSearchFormat(capabilities.Features, localservice.ScopedRecallCapability) {
		return writeAgentContractError(stderr, operation, "capability_unavailable", false, "upgrade_mindline")
	}
	judgment, err := client.ApplyScopedJudgment(context.Background(), input)
	if err != nil {
		return writeAgentContractError(stderr, operation, "feedback_rejected", false, "check_event_binding")
	}
	return encodePersonalMemoryJSON(stdout, stderr, judgment)
}

func (r Runner) runAgentService(args []string, stdout, stderr io.Writer) int {
	options, err := parseAgentOptions(args)
	if err != nil || len(options.positionals) != 0 || len(options.values) != 0 {
		return agentUsage(stderr)
	}
	config, err := localservice.LoadConfig(options.configPath)
	if err != nil {
		return agentFailure(stderr, err)
	}
	server, err := localservice.NewServer(config, nil, nil)
	if err != nil {
		return agentFailure(stderr, err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	result := make(chan error, 1)
	go func() {
		result <- server.Serve()
	}()
	select {
	case err := <-result:
		_ = server.Close(context.Background())
		if err != nil {
			return agentFailure(stderr, err)
		}
		return ExitOK
	case <-signals:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Close(ctx); err != nil {
			return agentFailure(stderr, err)
		}
		fmt.Fprintln(stdout, `{"schema_version":"mindline-local-agent-api/v0.1","data":{"service_state":"stopped"}}`)
		return ExitOK
	}
}
