package recallproof

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/assurance"
)

func TestReusableProofRunnerExecutesManifestAndDoesNotExportOutput(t *testing.T) {
	root, binding, executor := reusableRunnerFixture(t)
	artifact, err := (ReusableProofRunner{Executor: executor}).RunPreLive(context.Background(), root, binding)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Counts["executed_pre_live_groups"] == 0 || len(artifact.Tests) != artifact.Counts["executed_pre_live_groups"] {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private command output") {
		t.Fatal("proof artifact exported command output")
	}
}

func TestReusableProofRunnerCannotPassFailedGroup(t *testing.T) {
	root, binding, executor := reusableRunnerFixture(t)
	executor.results["go\x00test\x00./internal/cli"] = CommandResult{ExitCode: 1, Stdout: []byte("private command output")}
	if _, err := (ReusableProofRunner{Executor: executor}).RunPreLive(context.Background(), root, binding); err == nil {
		t.Fatal("failed proof group produced pass artifact")
	}
}

func TestReusableProofRunnerRejectsUnverifiedBindingCommitments(t *testing.T) {
	root, binding, executor := reusableRunnerFixture(t)
	tests := []struct {
		name   string
		mutate func(*TreeConfigBinding)
	}{
		{name: "binary", mutate: func(value *TreeConfigBinding) { value.BinaryFingerprint = "sha256:" + strings.Repeat("1", 64) }},
		{name: "manifest", mutate: func(value *TreeConfigBinding) {
			value.AssuranceManifestFingerprint = "sha256:" + strings.Repeat("2", 64)
		}},
		{name: "configuration", mutate: func(value *TreeConfigBinding) {
			value.LiveConfigurationFingerprint = "sha256:" + strings.Repeat("3", 64)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drift := binding
			test.mutate(&drift)
			if _, err := (ReusableProofRunner{Executor: executor}).RunPreLive(context.Background(), root, drift); err == nil {
				t.Fatal("fabricated binding commitment was accepted")
			}
		})
	}
}

type fakeDirectExecutor struct{ results map[string]CommandResult }

func (executor *fakeDirectExecutor) Run(_ context.Context, _ string, tool string, argv []string) CommandResult {
	if result, exists := executor.results[tool+"\x00"+strings.Join(argv, "\x00")]; exists {
		return result
	}
	return CommandResult{}
}

func reusableRunnerFixture(t *testing.T) (string, TreeConfigBinding, *fakeDirectExecutor) {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", ".."))
	treeBytes := []byte("synthetic tracked tree\n")
	binaryFingerprint, err := currentExecutableFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	binding := TreeConfigBinding{SchemaVersion: BindingSchemaVersion, Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TreeFingerprint: shaCommitment(treeBytes), BinaryFingerprint: binaryFingerprint, AssuranceManifestFingerprint: "sha256:" + assurance.WP48ManifestFingerprint(), LiveConfigurationFingerprint: LiveConfigurationFingerprint()}
	executor := &fakeDirectExecutor{results: map[string]CommandResult{
		"git\x00rev-parse\x00--show-toplevel":                      {Stdout: []byte(root + "\n")},
		"git\x00rev-parse\x00HEAD":                                 {Stdout: []byte(binding.Commit + "\n")},
		"git\x00status\x00--porcelain=v1\x00--untracked-files=all": {},
		"git\x00ls-tree\x00-r\x00--full-tree\x00HEAD":              {Stdout: treeBytes},
	}}
	return root, binding, executor
}
