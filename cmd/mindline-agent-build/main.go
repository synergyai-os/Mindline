// mindline-agent-build produces the supported clean-tree service binary used
// by local retrieval evaluation. It embeds the exact Git tree commitment and
// emits only structural fingerprints.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/localservice"
)

const (
	maximumBuildOutputBytes = 4 << 20
	maximumAgentBinaryBytes = 512 << 20
)

type buildReceipt struct {
	SchemaVersion    string `json:"schema_version"`
	State            string `json:"state"`
	Commit           string `json:"commit"`
	TreeFingerprint  string `json:"tree_fingerprint"`
	BuildFingerprint string `json:"build_fingerprint"`
	Output           string `json:"output"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mindline-agent-build: operation failed")
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) != 2 || args[0] != "--out" {
		return errors.New("usage: --out <absolute-path-outside-repository>")
	}
	output := filepath.Clean(strings.TrimSpace(args[1]))
	if !filepath.IsAbs(output) {
		return errors.New("audited build output must be absolute")
	}
	root, head, treeFingerprint, err := repositoryBinding()
	if err != nil || pathInside(root, output) {
		return errors.New("audited source tree unavailable")
	}
	parent := filepath.Dir(output)
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("audited build output directory unavailable")
	}
	if outputInfo, err := os.Lstat(output); err == nil {
		if !outputInfo.Mode().IsRegular() || outputInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("audited build output is unsafe")
		}
	} else if !os.IsNotExist(err) {
		return errors.New("audited build output unavailable")
	}
	linkerFlag, err := localservice.AuditedSourceTreeLinkerFlag(treeFingerprint)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".mindline-agent-build-*")
	if err != nil {
		return errors.New("prepare audited build output")
	}
	if err := os.Chmod(stage, 0o700); err != nil {
		_ = os.RemoveAll(stage)
		return errors.New("secure audited build stage")
	}
	defer os.RemoveAll(stage)
	temporaryPath := filepath.Join(stage, "mindline")

	goPath := filepath.Join(runtime.GOROOT(), "bin", "go")
	goInfo, err := os.Lstat(goPath)
	if err != nil || !goInfo.Mode().IsRegular() || goInfo.Mode()&0o111 == 0 {
		return errors.New("audited Go toolchain unavailable")
	}
	buildContext, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	command := exec.CommandContext(buildContext, goPath,
		"build", "-trimpath", "-ldflags="+linkerFlag,
		"-o", temporaryPath, "./cmd/mindline",
	)
	command.Dir = root
	command.Env = auditedBuildEnvironment(goPath)
	outputBuffer := &boundedBuffer{maximum: maximumBuildOutputBytes}
	command.Stdout, command.Stderr = outputBuffer, outputBuffer
	if err := command.Run(); err != nil || outputBuffer.exceeded || buildContext.Err() != nil {
		return errors.New("audited Mindline service build failed")
	}
	rootAfter, headAfter, treeAfter, err := repositoryBinding()
	if err != nil || rootAfter != root || headAfter != head || treeAfter != treeFingerprint {
		return errors.New("source tree changed during audited build")
	}
	if err := verifyBuildInfo(temporaryPath, head); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o700); err != nil {
		return errors.New("secure audited build output")
	}
	if err := os.Rename(temporaryPath, output); err != nil {
		return errors.New("commit audited build output")
	}
	buildFingerprint, err := regularFingerprint(output)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(buildReceipt{
		SchemaVersion: "mindline-agent-build/v0.1", State: "ready",
		Commit: head, TreeFingerprint: treeFingerprint,
		BuildFingerprint: buildFingerprint, Output: output,
	})
}

func repositoryBinding() (string, string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", "", err
	}
	rootBytes, err := gitOutput(cwd, "rev-parse", "--show-toplevel")
	root := filepath.Clean(strings.TrimSpace(string(rootBytes)))
	if err != nil || root != filepath.Clean(cwd) || !filepath.IsAbs(root) {
		return "", "", "", errors.New("run audited build from repository root")
	}
	status, err := gitOutput(root, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil || len(bytes.TrimSpace(status)) != 0 {
		return "", "", "", errors.New("audited build requires a clean tree")
	}
	headBytes, err := gitOutput(root, "rev-parse", "HEAD")
	head := strings.TrimSpace(string(headBytes))
	if err != nil || len(head) != 40 {
		return "", "", "", errors.New("audited source commit unavailable")
	}
	tree, err := gitOutput(root, "ls-tree", "-r", "--full-tree", "HEAD")
	if err != nil {
		return "", "", "", errors.New("audited source tree unavailable")
	}
	digest := sha256.Sum256(tree)
	return root, head, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func gitOutput(root string, args ...string) ([]byte, error) {
	gitPath := "/usr/bin/git"
	if runtime.GOOS != "darwin" {
		if _, err := os.Lstat(gitPath); err != nil {
			gitPath = "/bin/git"
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, gitPath, args...)
	command.Dir = root
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	var output boundedBuffer
	output.maximum = maximumBuildOutputBytes
	command.Stdout, command.Stderr = &output, io.Discard
	err := command.Run()
	if err != nil || ctx.Err() != nil || output.exceeded {
		return nil, errors.New("audited Git operation failed")
	}
	return output.Bytes(), nil
}

func auditedBuildEnvironment(goPath string) []string {
	home, _ := os.UserHomeDir()
	temporary := os.TempDir()
	return []string{
		"HOME=" + home, "TMPDIR=" + temporary,
		"PATH=" + filepath.Dir(goPath) + ":/usr/bin:/bin:/usr/sbin:/sbin",
		"GOROOT=" + runtime.GOROOT(), "GOTOOLCHAIN=local", "GOENV=off",
		"GOFLAGS=", "GOWORK=off", "GOPROXY=off", "LANG=C", "LC_ALL=C",
	}
}

func verifyBuildInfo(path, head string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return errors.New("audited build information unavailable")
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["vcs.revision"] != head || settings["vcs.modified"] != "false" {
		return errors.New("audited build does not match the clean source commit")
	}
	return nil
}

func regularFingerprint(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 1 || info.Size() > maximumAgentBinaryBytes {
		return "", errors.New("audited build output unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("audited build output unavailable")
	}
	defer file.Close()
	digest := sha256.New()
	written, err := io.Copy(digest, io.LimitReader(file, maximumAgentBinaryBytes+1))
	if err != nil || written != info.Size() || written > maximumAgentBinaryBytes {
		return "", errors.New("audited build output unavailable")
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func pathInside(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type boundedBuffer struct {
	bytes.Buffer
	maximum  int
	exceeded bool
}

func (buffer *boundedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := buffer.maximum - buffer.Len()
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
