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
	options, err := parseAgentOptions(args)
	allowed := []string{"actor", "idempotency-key", "reason"}
	if !reversal {
		allowed = append(allowed, "run", "lens", "record", "disposition")
	} else {
		allowed = append(allowed, "judgment")
	}
	if err != nil || len(options.positionals) != 0 || !onlyAgentKeys(options.values, allowed...) ||
		options.values["actor"] == "" || options.values["idempotency-key"] == "" {
		return agentUsage(stderr)
	}
	input := agentstate.JudgmentRequest{
		IdempotencyKey: options.values["idempotency-key"],
		Actor:          options.values["actor"], Reason: options.values["reason"],
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
