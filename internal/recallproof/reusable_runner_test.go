package recallproof

import (
	"context"
	"encoding/json"
	"os"
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

func TestReusableProofRunnerRejectsTreeDriftDuringProof(t *testing.T) {
	root, binding, executor := reusableRunnerFixture(t)
	drift := &driftingDirectExecutor{base: executor}
	if _, err := (ReusableProofRunner{Executor: drift}).RunPreLive(context.Background(), root, binding); err == nil {
		t.Fatal("tree drift during proof produced a pass artifact")
	}
}

func TestOSDirectExecutorIgnoresAmbientPathAndRejectsUnknownTools(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"go", "git"} {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\necho attacker-path-tool\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	result := (OSDirectExecutor{}).Run(context.Background(), filepath.Clean(filepath.Join("..", "..")), "git", []string{"--version"})
	if result.ExitCode != 0 || result.ToolFingerprint == "" || strings.Contains(string(result.Stdout), "attacker-path-tool") {
		t.Fatalf("ambient PATH selected proof tool: %+v", result)
	}
	if result := (OSDirectExecutor{}).Run(context.Background(), ".", "sh", []string{"-c", "true"}); result.ExitCode != -1 {
		t.Fatalf("unknown proof tool accepted: %+v", result)
	}
}

type fakeDirectExecutor struct{ results map[string]CommandResult }

func (executor *fakeDirectExecutor) Run(_ context.Context, _ string, tool string, argv []string) CommandResult {
	if result, exists := executor.results[tool+"\x00"+strings.Join(argv, "\x00")]; exists {
		return result
	}
	return CommandResult{}
}

type driftingDirectExecutor struct {
	base     *fakeDirectExecutor
	groupRan bool
}

func (executor *driftingDirectExecutor) Run(ctx context.Context, root, tool string, argv []string) CommandResult {
	if executor.groupRan && tool == "git" && strings.Join(argv, "\x00") == "status\x00--porcelain=v1\x00--untracked-files=all" {
		return CommandResult{Stdout: []byte(" M internal/recallproof/reusable_runner.go\n")}
	}
	result := executor.base.Run(ctx, root, tool, argv)
	if tool != "git" || (len(argv) > 0 && argv[0] != "rev-parse" && argv[0] != "status" && argv[0] != "ls-tree") {
		executor.groupRan = true
	}
	return result
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
