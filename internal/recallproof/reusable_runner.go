package recallproof

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/assurance"
)

// DirectExecutor is intentionally shell-free. Stdout and stderr are held only
// long enough to derive a commitment; callers never receive their contents.
type DirectExecutor interface {
	Run(context.Context, string, string, []string) CommandResult
}

type CommandResult struct {
	ExitCode        int
	Stdout          []byte
	Stderr          []byte
	ToolFingerprint string
}

type OSDirectExecutor struct{}

func (OSDirectExecutor) Run(ctx context.Context, directory, tool string, argv []string) CommandResult {
	executable, identity, err := approvedProofExecutable(tool)
	if err != nil {
		return CommandResult{ExitCode: -1}
	}
	commandContext, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	command := exec.CommandContext(commandContext, executable, argv...)
	command.Dir = directory
	command.Env = proofEnvironment(directory)
	assurance.ConfigureBoundedProcess(command)
	stdout, stderr := &proofOutputBuffer{}, &proofOutputBuffer{}
	command.Stdout, command.Stderr = stdout, stderr
	err = command.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ToolFingerprint: identity}
	after, afterErr := executableFingerprint(executable)
	if stdout.exceeded || stderr.exceeded || afterErr != nil || after != identity {
		result.ExitCode = -1
		return result
	}
	if err == nil {
		return result
	}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
		return result
	}
	result.ExitCode = -1
	return result
}

func approvedProofExecutable(tool string) (string, string, error) {
	var path string
	switch tool {
	case "go":
		path = filepath.Join(runtime.GOROOT(), "bin", "go")
	case "git":
		if runtime.GOOS == "darwin" {
			path = "/usr/bin/git"
		} else if _, err := os.Stat("/usr/bin/git"); err == nil {
			path = "/usr/bin/git"
		} else {
			path = "/bin/git"
		}
	default:
		return "", "", errors.New("proof executable is not allowlisted")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !filepath.IsAbs(resolved) {
		return "", "", errors.New("proof executable identity is unavailable")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return "", "", errors.New("proof executable identity is unavailable")
	}
	identity, err := executableFingerprint(resolved)
	return resolved, identity, err
}

func executableFingerprint(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func proofEnvironment(directory string) []string {
	home, _ := os.UserHomeDir()
	digest := sha256.Sum256([]byte(filepath.Clean(directory)))
	cache := filepath.Join(os.TempDir(), "mindline-wp48-proof-"+hex.EncodeToString(digest[:8]))
	_ = os.MkdirAll(cache, 0o700)
	path := strings.Join([]string{filepath.Join(runtime.GOROOT(), "bin"), "/usr/bin", "/bin", "/usr/sbin", "/sbin"}, string(os.PathListSeparator))
	return []string{
		"HOME=" + home, "TMPDIR=" + os.TempDir(), "PATH=" + path,
		"GOROOT=" + runtime.GOROOT(), "GOCACHE=" + cache,
		"GOTOOLCHAIN=local", "GOENV=off", "GOFLAGS=", "GOWORK=off", "GOPROXY=off",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"LANG=C", "LC_ALL=C", "NO_PROXY=127.0.0.1,localhost",
	}
}

type proofOutputBuffer struct {
	bytes.Buffer
	exceeded bool
}

func (buffer *proofOutputBuffer) Write(value []byte) (int, error) {
	const maximum = 8 << 20
	original := len(value)
	remaining := maximum - buffer.Len()
	if remaining <= 0 {
		buffer.exceeded = true
		return original, nil
	}
	if len(value) > remaining {
		value = value[:remaining]
		buffer.exceeded = true
	}
	_, _ = buffer.Buffer.Write(value)
	return original, nil
}

type ReusableProofRunner struct{ Executor DirectExecutor }

// RunPreLive binds the supplied root to the exact commit, source tree, and
// clean state before executing every embedded-manifest pre-live group.
func (runner ReusableProofRunner) RunPreLive(ctx context.Context, repositoryRoot string, binding TreeConfigBinding) (StructuralArtifact, error) {
	if runner.Executor == nil {
		return StructuralArtifact{}, errors.New("direct proof executor is required")
	}
	if err := binding.Validate(); err != nil {
		return StructuralArtifact{}, err
	}
	manifestPath := filepath.Join(repositoryRoot, "internal", "assurance", "manifests", "wp48-complete-recall-v1.json")
	manifest, err := assurance.LoadSignedWP48Manifest(manifestPath)
	if err != nil {
		return StructuralArtifact{}, err
	}
	if err := runner.verifyRepository(ctx, repositoryRoot, binding); err != nil {
		return StructuralArtifact{}, err
	}
	fingerprints := map[string]string{}
	tests := map[string]bool{}
	count := 0
	for _, group := range manifest.Groups {
		if group.Phase != "pre_live" {
			continue
		}
		for _, dependency := range group.DependsOn {
			if passed, exists := tests[dependency]; exists && !passed {
				return StructuralArtifact{}, errors.New("proof dependency did not pass")
			}
		}
		result := runner.Executor.Run(ctx, repositoryRoot, group.Tool, group.Argv)
		fingerprints[group.ID] = commandCommitment(group.Tool, group.Argv, result)
		if result.ExitCode != 0 {
			return StructuralArtifact{}, fmt.Errorf("WP-48 proof group failed: %s (exit %d)", group.ID, result.ExitCode)
		}
		tests[group.ID] = true
		count++
	}
	if count == 0 {
		return StructuralArtifact{}, errors.New("embedded WP-48 manifest has no pre-live groups")
	}
	if err := runner.verifyRepository(ctx, repositoryRoot, binding); err != nil {
		return StructuralArtifact{}, err
	}
	return StructuralArtifact{SchemaVersion: "mindline-reusable-proof/v0.1", Build: "wp48", State: "pass", Counts: map[string]int{"executed_pre_live_groups": count}, Fingerprints: fingerprints, Tests: tests}, nil
}

func (runner ReusableProofRunner) verifyRepository(ctx context.Context, root string, binding TreeConfigBinding) error {
	topLevel := runner.Executor.Run(ctx, root, "git", []string{"rev-parse", "--show-toplevel"})
	if topLevel.ExitCode != 0 || strings.TrimSpace(string(topLevel.Stdout)) != root {
		return errors.New("repository root does not exactly match supplied binding root")
	}
	head := runner.Executor.Run(ctx, root, "git", []string{"rev-parse", "HEAD"})
	if head.ExitCode != 0 || strings.TrimSpace(string(head.Stdout)) != binding.Commit {
		return errors.New("repository HEAD does not match supplied binding")
	}
	status := runner.Executor.Run(ctx, root, "git", []string{"status", "--porcelain=v1", "--untracked-files=all"})
	if status.ExitCode != 0 || len(strings.TrimSpace(string(status.Stdout))) != 0 {
		return errors.New("repository is not clean")
	}
	tree := runner.Executor.Run(ctx, root, "git", []string{"ls-tree", "-r", "--full-tree", "HEAD"})
	if tree.ExitCode != 0 || shaCommitment(tree.Stdout) != binding.TreeFingerprint {
		return errors.New("repository tree does not match supplied binding")
	}
	binaryFingerprint, err := currentExecutableFingerprint()
	if err != nil || binaryFingerprint != binding.BinaryFingerprint {
		return errors.New("running proof binary does not match supplied binding")
	}
	if "sha256:"+assurance.WP48ManifestFingerprint() != binding.AssuranceManifestFingerprint {
		return errors.New("embedded assurance manifest does not match supplied binding")
	}
	if LiveConfigurationFingerprint() != binding.LiveConfigurationFingerprint {
		return errors.New("live configuration does not match supplied binding")
	}
	return nil
}

func currentExecutableFingerprint() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func commandCommitment(tool string, argv []string, result CommandResult) string {
	value := tool + "\x00" + result.ToolFingerprint + "\x00" + strings.Join(argv, "\x00") + fmt.Sprintf("\x00%d\x00", result.ExitCode)
	value += string(result.Stdout) + "\x00" + string(result.Stderr)
	return shaCommitment([]byte(value))
}

func shaCommitment(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}
