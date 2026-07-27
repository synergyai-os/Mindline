package assurance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	ProofRunnerVersion             = "mindline-proof-runner/v1"
	ControllerBootstrapSchema      = "mindline-proof-controller-bootstrap/v1"
	maximumControllerArtifactBytes = 16 << 20
)

var controllerArtifactNames = []string{
	"controller-bootstrap-evidence.json",
	"proof-runner-build.stdout",
	"proof-runner-build.stderr",
	"proof-runner-buildinfo.json",
	"proof-runner.sha256",
	"proof-runner-version.stdout",
}

var controllerBase32Pattern = regexp.MustCompile(`^[a-z2-7]+$`)

type ControllerBootstrapEvidence struct {
	SchemaVersion                  string   `json:"schema_version"`
	ControllerInvocationID         string   `json:"controller_invocation_id"`
	ControllerInvocationGeneration string   `json:"controller_invocation_generation"`
	ControllerBootstrapRoot        string   `json:"controller_bootstrap_root"`
	FrozenCommit                   string   `json:"frozen_commit"`
	FrozenTree                     string   `json:"frozen_tree"`
	ManifestSHA256                 string   `json:"manifest_sha256"`
	BuildTool                      string   `json:"build_tool"`
	BuildArgv                      []string `json:"build_argv"`
	BuildWorkingDirectory          string   `json:"build_working_directory"`
	BuildShellFalse                bool     `json:"build_shell_false"`
	BuildExit                      int      `json:"build_exit"`
	BuildStdoutSHA256              string   `json:"build_stdout_sha256"`
	BuildStderrSHA256              string   `json:"build_stderr_sha256"`
	RunnerBuildinfoSHA256          string   `json:"runner_buildinfo_sha256"`
	RunnerSHA256                   string   `json:"runner_sha256"`
	VersionArgv                    []string `json:"version_argv"`
	VersionStdoutExact             string   `json:"version_stdout_exact"`
	InvokeTool                     string   `json:"invoke_tool"`
	InvokeArgv                     []string `json:"invoke_argv"`
	InvokeWorkingDirectory         string   `json:"invoke_working_directory"`
	InvokeShellFalse               bool     `json:"invoke_shell_false"`
	StateInvocationReady           bool     `json:"state_invocation_ready"`
}

type ValidatedControllerBootstrap struct {
	Record     ControllerBootstrapEvidence
	RecordPath string
	Root       string
	Artifacts  map[string][]byte
}

// RunProofRunner implements the deliberately small, exact CLI surface.
func RunProofRunner(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--version" {
		_, _ = io.WriteString(stdout, ProofRunnerVersion+"\n")
		return 0
	}
	if len(args) == 5 && args[0] == "run" && args[1] == "--manifest" && args[3] == "--controller-bootstrap" {
		if err := runProofBootstrap(args[2], args[4], stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, "mindline-proof-runner:", err)
			return 1
		}
		return 0
	}
	if len(args) == 2 && args[0] == "group" {
		if err := runProofGroup(args[1], stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, "mindline-proof-runner:", err)
			return 1
		}
		return 0
	}
	_, _ = fmt.Fprintln(stderr, "usage: mindline-proof-runner --version | run --manifest <absolute-path> --controller-bootstrap <absolute-path> | group <signed-group-id>")
	return 2
}

func runProofBootstrap(manifestPath, controllerPath string, stdout io.Writer) error {
	if !filepath.IsAbs(manifestPath) || !filepath.IsAbs(controllerPath) {
		return errors.New("run requires absolute manifest and controller-bootstrap paths")
	}
	manifest, err := LoadSignedWP46Manifest(manifestPath)
	if err != nil {
		return err
	}
	repositoryRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(manifestPath))))
	repositoryRoot, err = filepath.Abs(repositoryRoot)
	if err != nil {
		return err
	}
	runnerPath, err := os.Executable()
	if err != nil {
		return err
	}
	runnerPath, err = filepath.EvalSymlinks(runnerPath)
	if err != nil {
		return err
	}
	actualArgv := []string{"run", "--manifest", manifestPath, "--controller-bootstrap", controllerPath}
	validated, err := ValidateControllerBootstrap(controllerPath, manifestPath, repositoryRoot, runnerPath, actualArgv)
	if err != nil {
		return err
	}
	attempt, err := BeginWP46ProofAttempt(validated.Record.FrozenCommit, WP46ManifestSHA256, AttemptOptions{})
	if err != nil {
		return err
	}
	if err := importControllerBootstrap(attempt.Root, validated); err != nil {
		return err
	}
	bootstrap := struct {
		SchemaVersion             string   `json:"schema_version"`
		ManifestID                string   `json:"manifest_id"`
		ManifestSHA256            string   `json:"manifest_sha256"`
		ControllerInvocationID    string   `json:"controller_invocation_id"`
		ControllerBootstrapSHA256 string   `json:"controller_bootstrap_sha256"`
		ActualRunnerArgv          []string `json:"actual_runner_argv"`
		AttemptID                 string   `json:"attempt_id"`
		AttemptGeneration         string   `json:"attempt_generation"`
	}{WP46ManifestSchema, manifest.ID, WP46ManifestSHA256, validated.Record.ControllerInvocationID,
		sha256Hex(validated.Artifacts["controller-bootstrap-evidence.json"]), actualArgv,
		attempt.State.AttemptID, attempt.State.AttemptGeneration}
	if err := writeJSONNoReplaceSynced(filepath.Join(attempt.Root, "bootstrap-evidence.json"), bootstrap, 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "proof attempt prepared: %s\n", attempt.Root)
	return nil
}

func ValidateControllerBootstrap(recordPath, manifestPath, repositoryRoot, runnerPath string, actualArgv []string) (ValidatedControllerBootstrap, error) {
	for _, path := range []string{recordPath, manifestPath, repositoryRoot, runnerPath} {
		if !filepath.IsAbs(path) {
			return ValidatedControllerBootstrap{}, errors.New("controller bootstrap paths must be absolute")
		}
	}
	if filepath.Base(recordPath) != "controller-bootstrap-evidence.json" {
		return ValidatedControllerBootstrap{}, errors.New("controller bootstrap record has an unexpected name")
	}
	root := filepath.Dir(recordPath)
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm()&0o077 != 0 {
		return ValidatedControllerBootstrap{}, errors.New("controller bootstrap root is not owner-only")
	}
	artifacts := make(map[string][]byte, len(controllerArtifactNames))
	for _, name := range controllerArtifactNames {
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maximumControllerArtifactBytes {
			return ValidatedControllerBootstrap{}, fmt.Errorf("controller artifact %q is unsafe", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return ValidatedControllerBootstrap{}, err
		}
		artifacts[name] = data
	}
	decoder := json.NewDecoder(bytes.NewReader(artifacts["controller-bootstrap-evidence.json"]))
	decoder.DisallowUnknownFields()
	var record ControllerBootstrapEvidence
	if err := decoder.Decode(&record); err != nil {
		return ValidatedControllerBootstrap{}, fmt.Errorf("strict controller record decode: %w", err)
	}
	if record.SchemaVersion != ControllerBootstrapSchema || !record.StateInvocationReady ||
		record.ControllerBootstrapRoot != root || record.ManifestSHA256 != WP46ManifestSHA256 ||
		!fullGitCommitPattern.MatchString(record.FrozenCommit) || !fullGitCommitPattern.MatchString(record.FrozenTree) ||
		!record.BuildShellFalse || !record.InvokeShellFalse || record.BuildExit != 0 {
		return ValidatedControllerBootstrap{}, errors.New("controller bootstrap binding mismatch")
	}
	if len(record.ControllerInvocationID) != 26 || len(record.ControllerInvocationGeneration) != 52 ||
		!controllerBase32Pattern.MatchString(record.ControllerInvocationID) || !controllerBase32Pattern.MatchString(record.ControllerInvocationGeneration) {
		return ValidatedControllerBootstrap{}, errors.New("controller invocation identity is invalid")
	}
	wantRunner := filepath.Join(root, "mindline-proof-runner")
	wantBuildArgv := []string{"build", "-trimpath", "-o", wantRunner, "./cmd/mindline-proof-runner"}
	wantInvokeArgv := []string{"run", "--manifest", manifestPath, "--controller-bootstrap", recordPath}
	if record.BuildTool != "go" || !equalStrings(record.BuildArgv, wantBuildArgv) || record.BuildWorkingDirectory != repositoryRoot ||
		record.InvokeTool != wantRunner || !equalStrings(record.InvokeArgv, wantInvokeArgv) || !equalStrings(actualArgv, wantInvokeArgv) ||
		record.InvokeWorkingDirectory != repositoryRoot || !equalStrings(record.VersionArgv, []string{"--version"}) ||
		record.VersionStdoutExact != ProofRunnerVersion+"\n" || string(artifacts["proof-runner-version.stdout"]) != ProofRunnerVersion+"\n" {
		return ValidatedControllerBootstrap{}, errors.New("controller build, version, or invoke argv is inexact")
	}
	resolvedRunner, err := filepath.EvalSymlinks(wantRunner)
	if err != nil {
		return ValidatedControllerBootstrap{}, err
	}
	if resolvedRunner != runnerPath {
		return ValidatedControllerBootstrap{}, errors.New("running proof controller is not the recorded binary")
	}
	runnerInfo, err := os.Lstat(wantRunner)
	if err != nil || !runnerInfo.Mode().IsRegular() || runnerInfo.Mode()&0o111 == 0 || runnerInfo.Mode()&os.ModeSymlink != 0 {
		return ValidatedControllerBootstrap{}, errors.New("recorded proof runner is not a regular executable")
	}
	hashChecks := map[string]string{
		"proof-runner-build.stdout":   record.BuildStdoutSHA256,
		"proof-runner-build.stderr":   record.BuildStderrSHA256,
		"proof-runner-buildinfo.json": record.RunnerBuildinfoSHA256,
	}
	for name, expected := range hashChecks {
		if sha256Hex(artifacts[name]) != expected {
			return ValidatedControllerBootstrap{}, fmt.Errorf("controller artifact %q fingerprint mismatch", name)
		}
	}
	runnerBytes, err := os.ReadFile(wantRunner)
	if err != nil {
		return ValidatedControllerBootstrap{}, err
	}
	if sha256Hex(runnerBytes) != record.RunnerSHA256 || strings.TrimSpace(string(artifacts["proof-runner.sha256"])) != record.RunnerSHA256 {
		return ValidatedControllerBootstrap{}, errors.New("proof runner fingerprint mismatch")
	}
	buildInfo, err := parseRunnerBuildInfo(artifacts["proof-runner-buildinfo.json"])
	if err != nil || buildInfo.FrozenCommit != record.FrozenCommit || buildInfo.FrozenTree != record.FrozenTree {
		return ValidatedControllerBootstrap{}, errors.New("proof runner build info binding mismatch")
	}
	commit, err := gitValue(repositoryRoot, "rev-parse", "HEAD")
	if err != nil || commit != record.FrozenCommit {
		return ValidatedControllerBootstrap{}, errors.New("controller frozen commit mismatch")
	}
	tree, err := gitValue(repositoryRoot, "rev-parse", "HEAD^{tree}")
	if err != nil || tree != record.FrozenTree {
		return ValidatedControllerBootstrap{}, errors.New("controller frozen tree mismatch")
	}
	return ValidatedControllerBootstrap{Record: record, RecordPath: recordPath, Root: root, Artifacts: artifacts}, nil
}

func importControllerBootstrap(attemptRoot string, validated ValidatedControllerBootstrap) error {
	destination := filepath.Join(attemptRoot, "controller-bootstrap")
	if err := os.Mkdir(destination, 0o700); err != nil {
		return err
	}
	for _, name := range controllerArtifactNames {
		if err := writeNoReplaceSynced(filepath.Join(destination, name), validated.Artifacts[name], 0o600); err != nil {
			return err
		}
	}
	return syncDirectory(destination)
}

func runProofGroup(groupID string, stdout io.Writer) error {
	manifest, err := ParseWP46Manifest(embeddedWP46Manifest)
	if err != nil {
		return err
	}
	var selected ManifestGroup
	for _, group := range manifest.Groups {
		if group.ID == groupID {
			selected = group
			break
		}
	}
	if selected.ID == "" || selected.Tool != "${MINDLINE_PROOF_RUNNER}" || !equalStrings(selected.Argv, []string{"group", groupID}) {
		return fmt.Errorf("group %q is not a signed runner-owned group", groupID)
	}
	controlRoot, err := DefaultControlRoot()
	if err != nil {
		return err
	}
	attempt, err := loadCurrentProofAttempt(controlRoot)
	if err != nil {
		return err
	}
	for _, dependency := range selected.DependsOn {
		if _, err := loadPassingGroupEvidence(attempt.Root, dependency); err != nil {
			return fmt.Errorf("group %q dependency %q has no passing evidence: %w", groupID, dependency, err)
		}
	}
	started := time.Now().UTC()
	artifacts, details, err := executeRunnerOwnedGroup(&attempt, manifest, selected)
	if err != nil {
		return err
	}
	if groupID == "close_attempt_and_finalize_evidence" {
		_, _ = fmt.Fprintln(stdout, groupID+": pass")
		return nil
	}
	groupsRoot := filepath.Join(attempt.Root, "groups")
	if err := ensurePrivateDirectory(groupsRoot); err != nil {
		return err
	}
	record, err := buildPassingGroupEvidence(attempt.Root, selected, started, time.Now().UTC(), artifacts, details)
	if err != nil {
		return err
	}
	if err := writeJSONNoReplaceSynced(filepath.Join(groupsRoot, groupID+".json"), record, 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, groupID+": pass")
	return nil
}

func loadCurrentProofAttempt(controlRoot string) (ProofAttempt, error) {
	path := filepath.Join(controlRoot, "assurance", "current-proof-attempt.json")
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 1<<20 {
		return ProofAttempt{}, errors.New("current proof attempt is unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state ProofAttemptState
	if err := decoder.Decode(&state); err != nil {
		return ProofAttempt{}, err
	}
	if state.SchemaVersion != ProofAttemptSchema || state.State == "succeeded" || state.State == "failed" ||
		!filepath.IsAbs(state.ProofRoot) || !strings.HasPrefix(state.ProofRoot, filepath.Join(controlRoot, "assurance", "proof", "WP-46")+string(filepath.Separator)) {
		return ProofAttempt{}, errors.New("current proof attempt is not active")
	}
	return ProofAttempt{ControlRoot: controlRoot, Root: state.ProofRoot, StatePath: path, LedgerPath: filepath.Join(state.ProofRoot, "attempt-ledger.jsonl"), State: state}, nil
}

func gitValue(repositoryRoot string, args ...string) (string, error) {
	git := "/usr/bin/git"
	if runtime.GOOS == "windows" {
		var err error
		git, err = exec.LookPath("git")
		if err != nil {
			return "", err
		}
	}
	command := exec.Command(git, args...)
	command.Dir = repositoryRoot
	command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + os.Getenv("HOME"), "LANG=C", "LC_ALL=C"}
	output, err := command.Output()
	return strings.TrimSpace(string(output)), err
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func writeJSONNoReplaceSynced(path string, value any, mode os.FileMode) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeNoReplaceSynced(path, append(data, '\n'), mode)
}
