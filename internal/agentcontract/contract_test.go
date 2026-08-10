package agentcontract

import (
	"strings"
	"testing"
)

func TestSharedWorkflowUsesOneExecutableScopedRouteAndNoContextSelection(t *testing.T) {
	workflow := NewWorkflow("/Applications/Mindline Agent/mindline", "/private/config path/config.json")
	commands := []string{workflow.RegistrationToken, workflow.Register, workflow.Discover, workflow.Search, workflow.Get,
		workflow.FeedbackToken, workflow.Feedback, workflow.FeedbackReverse}
	for _, command := range commands {
		if !strings.HasPrefix(command, "'/Applications/Mindline Agent/mindline' agent ") {
			t.Fatalf("workflow is not executable through the same binary: %q", command)
		}
	}
	for _, command := range []string{workflow.Discover, workflow.Search, workflow.Get, workflow.Feedback, workflow.FeedbackReverse} {
		if !strings.Contains(command, "--scope <scope> --lens <lens> --agent <actor>") ||
			!strings.Contains(command, "--config '/private/config path/config.json'") {
			t.Fatalf("workflow lost binding or config propagation: %q", command)
		}
	}
	if !strings.Contains(workflow.Register, "--name <agent-name> --retry-token <token>") ||
		!strings.Contains(workflow.Register, "--config '/private/config path/config.json'") {
		t.Fatalf("registration lost retry or config contract: %q", workflow.Register)
	}
	if strings.Contains(strings.Join(commands, "\n"), "scope-list") ||
		strings.Contains(strings.Join(commands, "\n"), "actor-list") {
		t.Fatal("shared agent workflow invites context selection")
	}
	help := HelpText("/Applications/Mindline Agent/mindline")
	if !strings.Contains(help, "owner must supply the complete scope and lens") ||
		!strings.Contains(help, "Never list, choose, infer") {
		t.Fatalf("help lost owner-supplied binding rule: %q", help)
	}
	for _, command := range commands {
		if !strings.Contains(help, strings.TrimSuffix(command, " --config '/private/config path/config.json'")) &&
			(command == workflow.Discover || command == workflow.Search || command == workflow.Get) {
			t.Fatalf("help drifted from shared workflow: %q", command)
		}
	}
}
