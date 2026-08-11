package cli

import (
	"bytes"
	"io"
)

// runAgentOnly is the cooperative least-privilege surface for outside agents.
// It intentionally does not claim to sandbox a hostile local process; it makes
// the governed route the only route exposed by this command namespace.
func (r Runner) runAgentOnly(args []string, stdout, stderr io.Writer) int {
	if !agentOnlyRouteAllowed(args) {
		return writeAgentContractError(
			stderr, "agent_only", "route_not_available", false, "use_agent_only_help",
		)
	}
	r.agentNamespace = "agent-only"
	var commandOutput, commandError bytes.Buffer
	code := r.runAgent(args, &commandOutput, &commandError)
	if code == ExitUsage {
		return writeAgentContractError(
			stderr, "agent_only", "invalid_governed_command", false, "use_agent_only_help",
		)
	}
	_, _ = io.Copy(stdout, &commandOutput)
	_, _ = io.Copy(stderr, &commandError)
	return code
}

func agentOnlyRouteAllowed(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "help", "--help", "status", "capabilities", "registration-token",
		"register", "discover", "feedback-token":
		return true
	case "search":
		return hasCompleteAgentOnlyBinding(args[1:], false) &&
			hasAgentOptionValue(args[1:], "format", "compact-scoped-v0.4")
	case "get":
		return hasCompleteAgentOnlyBinding(args[1:], true)
	case "feedback":
		return hasCompleteAgentOnlyBinding(args[1:], true) &&
			hasAgentOptionValue(args[1:], "actor", "agent")
	case "feedback-reverse":
		return hasAgentOptions(args[1:], "scope", "lens", "agent", "judgment", "idempotency-key") &&
			hasAgentOptionValue(args[1:], "actor", "agent")
	default:
		return false
	}
}

func hasCompleteAgentOnlyBinding(args []string, requireRun bool) bool {
	required := []string{"scope", "lens", "agent"}
	if requireRun {
		required = append(required, "run")
	}
	return hasAgentOptions(args, required...)
}

func hasAgentOptions(args []string, names ...string) bool {
	options, err := parseAgentOptions(args)
	if err != nil {
		return false
	}
	for _, name := range names {
		if options.values[name] == "" {
			return false
		}
	}
	return true
}
