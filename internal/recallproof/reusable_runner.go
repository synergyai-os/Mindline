package recallproof

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
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
	FailureCode     string
}

type OSDirectExecutor struct{}

func (OSDirectExecutor) Run(ctx context.Context, directory, tool string, argv []string) CommandResult {
	executable, identity, err := approvedProofExecutable(tool)
	if err != nil {
		return CommandResult{ExitCode: -1, FailureCode: "tool_identity_unavailable"}
	}
	commandContext, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	command := exec.CommandContext(commandContext, executable, argv...)
	command.Dir = directory
	command.Env = proofEnvironment(directory, tool, executable)
	assurance.ConfigureBoundedProcess(command)
	stdout, stderr := &proofOutputBuffer{}, &proofOutputBuffer{}
	command.Stdout, command.Stderr = stdout, stderr
	err = command.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ToolFingerprint: identity}
	after, afterErr := executableFingerprint(executable)
	if stdout.exceeded || stderr.exceeded {
		result.ExitCode = -1
		result.FailureCode = "output_limit_exceeded"
		return result
	}
	if afterErr != nil || after != identity {
		result.ExitCode = -1
		result.FailureCode = "tool_identity_changed"
		return result
	}
	if err == nil {
		return result
	}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
		result.FailureCode = "command_failed"
		return result
	}
	result.ExitCode = -1
	if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
		result.FailureCode = "command_timed_out"
	} else {
		result.FailureCode = "command_execution_unavailable"
	}
	return result
}

func approvedProofExecutable(tool string) (string, string, error) {
	var path string
	switch tool {
	case "go":
		var err error
		path, err = approvedGoExecutable()
		if err != nil {
			return "", "", err
		}
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

func approvedGoExecutable() (string, error) {
	version := strings.TrimPrefix(runtime.Version(), "go")
	home, _ := os.UserHomeDir()
	roots := []string{
		runtime.GOROOT(), os.Getenv("GOROOT"),
		filepath.Join(home, "go", "pkg", "mod", "golang.org", "toolchain@v0.0.1-go"+version+"."+runtime.GOOS+"-"+runtime.GOARCH),
		filepath.Join(home, ".proto", "tools", "go", version),
		"/usr/local/go", "/opt/homebrew/opt/go/libexec", "/usr/local/opt/go/libexec",
	}
	seen := map[string]struct{}{}
	for _, root := range roots {
		if root == "" || !filepath.IsAbs(root) {
			continue
		}
		candidate, err := filepath.EvalSymlinks(filepath.Join(filepath.Clean(root), "bin", "go"))
		if err != nil || !filepath.IsAbs(candidate) {
			continue
		}
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			continue
		}
		build, err := buildinfo.ReadFile(candidate)
		if err != nil || build.GoVersion != runtime.Version() {
			continue
		}
		return candidate, nil
	}
	return "", errors.New("approved Go toolchain is unavailable")
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

func proofEnvironment(directory, tool, executable string) []string {
	home, _ := os.UserHomeDir()
	digest := sha256.Sum256([]byte(filepath.Clean(directory)))
	cache := filepath.Join(os.TempDir(), "mindline-wp48-proof-"+hex.EncodeToString(digest[:8]))
	_ = os.MkdirAll(cache, 0o700)
	pathParts := []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"}
	goRoot := ""
	if tool == "go" {
		goRoot = filepath.Dir(filepath.Dir(executable))
		pathParts = append([]string{filepath.Join(goRoot, "bin")}, pathParts...)
	}
	environment := []string{
		"HOME=" + home, "TMPDIR=" + os.TempDir(), "PATH=" + strings.Join(pathParts, string(os.PathListSeparator)),
		"GOCACHE=" + cache,
		"GOTOOLCHAIN=local", "GOENV=off", "GOFLAGS=", "GOWORK=off", "GOPROXY=off",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"LANG=C", "LC_ALL=C", "NO_PROXY=127.0.0.1,localhost",
	}
	if goRoot != "" {
		environment = append(environment, "GOROOT="+goRoot)
	}
	return environment
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
			return StructuralArtifact{}, fmt.Errorf("WP-48 proof group failed: %s (%s, exit %d)", group.ID, result.FailureCode, result.ExitCode)
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
	value := tool + "\x00" + result.ToolFingerprint + "\x00" + strings.Join(argv, "\x00") + fmt.Sprintf("\x00%d\x00%s\x00", result.ExitCode, result.FailureCode)
	value += string(result.Stdout) + "\x00" + string(result.Stderr)
	return shaCommitment([]byte(value))
}

func shaCommitment(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}
