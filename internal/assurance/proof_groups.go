package assurance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	ProofGroupEvidenceSchema   = "mindline-proof-group-evidence/v1"
	ProofRunnerBuildInfoSchema = "mindline-proof-runner-buildinfo/v1"
	ProofProcessRegistrySchema = "mindline-proof-process-registry/v1"
)

type ProofArtifactEvidence struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type ProofGroupEvidence struct {
	SchemaVersion string                  `json:"schema_version"`
	ID            string                  `json:"id"`
	Phase         string                  `json:"phase"`
	Argv          []string                `json:"argv"`
	Outcome       string                  `json:"outcome"`
	StartedAt     string                  `json:"started_at"`
	CompletedAt   string                  `json:"completed_at"`
	Artifacts     []ProofArtifactEvidence `json:"artifacts"`
	Details       json.RawMessage         `json:"details"`
	Exports       map[string]string       `json:"exports,omitempty"`
}

type ProofRunnerToolIdentity struct {
	Path          string   `json:"path"`
	SHA256        string   `json:"sha256"`
	VersionArgv   []string `json:"version_argv"`
	VersionOutput string   `json:"version_output"`
}

type ProofRunnerBuildInfo struct {
	SchemaVersion string                             `json:"schema_version"`
	FrozenCommit  string                             `json:"frozen_commit"`
	FrozenTree    string                             `json:"frozen_tree"`
	GoVersion     string                             `json:"go_version"`
	Tools         map[string]ProofRunnerToolIdentity `json:"tools"`
	BrowserBundle string                             `json:"browser_bundle"`
	Chromium      string                             `json:"chromium"`
}

type proofProcessRecord struct {
	PID       int    `json:"pid"`
	PGID      int    `json:"pgid"`
	State     string `json:"state"`
	StartedAt string `json:"started_at"`
}

type proofProcessRegistry struct {
	SchemaVersion     string                        `json:"schema_version"`
	AttemptID         string                        `json:"attempt_id"`
	AttemptGeneration string                        `json:"attempt_generation"`
	Exports           map[string]proofProcessRecord `json:"exports"`
}

func executeRunnerOwnedGroup(attempt *ProofAttempt, manifest WP46Manifest, group ManifestGroup) ([]string, map[string]any, error) {
	switch group.ID {
	case "validate_controller_bootstrap":
		return validateImportedController(attempt)
	case "resolve_execution_bindings":
		return resolveProofBindings(attempt)
	case "verify_toolchain":
		return verifyProofToolchain(attempt)
	case "prepare_proof_roots":
		return verifyProofRoots(attempt)
	case "verify_baseline_safe_command":
		return exerciseBaselineCommand(attempt, false)
	case "git_diff_check":
		return runStrictGitDiffCheck(attempt)
	case "seal_pre_live_checkpoint":
		return sealPreLiveCheckpoint(attempt, manifest)
	case "stop_server_graceful":
		return stopTrackedServer(attempt, "initial_server_process_group", false, "server-graceful-stop.json")
	case "stop_server_crash":
		return stopTrackedServer(attempt, "graceful_restart_server_process_group", true, "server-crash-stop.json")
	case "stop_server_for_rollback":
		return stopTrackedServer(attempt, "crash_restart_server_process_group", false, "server-pre-rollback-stop.json")
	case "exercise_baseline_populated_owner_environment":
		return exerciseBaselineCommand(attempt, true)
	case "contain_server_process":
		return containTrackedServers(attempt)
	case "resolve_private_run_binding":
		return resolvePrivateRunBinding(attempt)
	case "close_attempt_and_finalize_evidence":
		return nil, nil, closeProofAttempt(attempt, manifest)
	default:
		return nil, nil, fmt.Errorf("runner-owned group %q has no fail-closed implementation", group.ID)
	}
}

func validateImportedController(attempt *ProofAttempt) ([]string, map[string]any, error) {
	root := filepath.Join(attempt.Root, "controller-bootstrap")
	artifacts := make(map[string][]byte, len(controllerArtifactNames))
	for _, name := range controllerArtifactNames {
		data, err := readPrivateRegular(filepath.Join(root, name), maximumControllerArtifactBytes)
		if err != nil {
			return nil, nil, err
		}
		artifacts[name] = data
	}
	var record ControllerBootstrapEvidence
	if err := decodeStrict(artifacts["controller-bootstrap-evidence.json"], &record); err != nil {
		return nil, nil, err
	}
	if record.SchemaVersion != ControllerBootstrapSchema || !record.StateInvocationReady ||
		record.ManifestSHA256 != WP46ManifestSHA256 || record.FrozenCommit != attempt.State.FrozenCommit {
		return nil, nil, errors.New("imported controller record does not bind this attempt")
	}
	checks := map[string]string{
		"proof-runner-build.stdout":   record.BuildStdoutSHA256,
		"proof-runner-build.stderr":   record.BuildStderrSHA256,
		"proof-runner-buildinfo.json": record.RunnerBuildinfoSHA256,
	}
	for name, expected := range checks {
		if sha256Hex(artifacts[name]) != expected {
			return nil, nil, fmt.Errorf("imported controller artifact %s changed", name)
		}
	}
	if strings.TrimSpace(string(artifacts["proof-runner.sha256"])) != record.RunnerSHA256 ||
		string(artifacts["proof-runner-version.stdout"]) != ProofRunnerVersion+"\n" {
		return nil, nil, errors.New("imported proof runner identity changed")
	}
	buildInfo, err := parseRunnerBuildInfo(artifacts["proof-runner-buildinfo.json"])
	if err != nil || buildInfo.FrozenCommit != record.FrozenCommit || buildInfo.FrozenTree != record.FrozenTree {
		return nil, nil, errors.New("proof runner build info does not bind the frozen source")
	}
	validation := map[string]any{
		"schema_version": "mindline-controller-bootstrap-validation/v1", "attempt_id": attempt.State.AttemptID,
		"controller_invocation_id": record.ControllerInvocationID, "manifest_sha256": record.ManifestSHA256,
		"frozen_commit": record.FrozenCommit, "frozen_tree": record.FrozenTree, "imported_artifact_bytes_exact": true,
		"actual_run_argv": record.InvokeArgv, "unknown_fields": 0, "secret_values": 0,
	}
	path, err := writeAttemptArtifact(attempt, "controller-bootstrap-validation.json", validation)
	return []string{path}, validation, err
}

func resolveProofBindings(attempt *ProofAttempt) ([]string, map[string]any, error) {
	repositoryRoot, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	nonAttempt := map[string]any{
		"schema_version": "mindline-resolved-bindings/v1", "REPOSITORY_ROOT": repositoryRoot,
		"FROZEN_COMMIT": attempt.State.FrozenCommit, "MINDLINE_CONTROL_ROOT": attempt.ControlRoot,
		"MINDLINE_ATTEMPT_STATE": attempt.StatePath,
	}
	attemptBindings := map[string]any{
		"schema_version": "mindline-resolved-attempt-bindings/v1", "PROOF_ATTEMPT_ID": attempt.State.AttemptID,
		"PROOF_ATTEMPT_GENERATION": attempt.State.AttemptGeneration, "MINDLINE_PROOF_ROOT": attempt.Root,
		"MINDLINE_ATTEMPT_LEDGER":    attempt.LedgerPath,
		"MINDLINE_PRECHANGE_ROOT":    filepath.Join(attempt.Root, "prepare", "prechange-worktree"),
		"MINDLINE_PRECHANGE_BINARY":  filepath.Join(attempt.Root, "bin", "mindline-prechange"),
		"MINDLINE_FROZEN_BINARY":     filepath.Join(attempt.Root, "bin", "mindline-frozen"),
		"MINDLINE_OPERATOR_LAUNCHER": filepath.Join(attempt.Root, "bin", "mindline-operator-launcher"),
	}
	deferred := map[string]any{"schema_version": "mindline-deferred-bindings/v1", "unresolved": []string{"PRIVATE_RUN_ROOT"}}
	values := []struct {
		name  string
		value any
	}{
		{"resolved-non-attempt-bindings.json", nonAttempt}, {"resolved-attempt-bindings.json", attemptBindings},
		{"declared-deferred-bindings.json", deferred},
	}
	paths := make([]string, 0, len(values))
	for _, value := range values {
		path, err := writeAttemptArtifact(attempt, value.name, value.value)
		if err != nil {
			return nil, nil, err
		}
		paths = append(paths, path)
	}
	return paths, map[string]any{"non_attempt_bindings_resolved": true, "fresh_attempt_bindings_resolved": true, "deferred_bindings_unresolved": []string{"PRIVATE_RUN_ROOT"}}, nil
}

func verifyProofToolchain(attempt *ProofAttempt) ([]string, map[string]any, error) {
	data, err := readPrivateRegular(filepath.Join(attempt.Root, "controller-bootstrap", "proof-runner-buildinfo.json"), maximumControllerArtifactBytes)
	if err != nil {
		return nil, nil, err
	}
	buildInfo, err := parseRunnerBuildInfo(data)
	if err != nil {
		return nil, nil, err
	}
	if buildInfo.BrowserBundle != "26.707.71524" || buildInfo.Chromium != "150.0.7871.115" {
		return nil, nil, errors.New("controller did not bind the signed browser identity")
	}
	if !strings.Contains(buildInfo.GoVersion, fmt.Sprintf("go version %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)) {
		return nil, nil, errors.New("controller build toolchain differs from the proof runner toolchain")
	}
	type toolSpec struct {
		argv              []string
		needle, exactPath string
	}
	required := map[string]toolSpec{
		"go":          {[]string{"version"}, fmt.Sprintf("go version %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH), ""},
		"git":         {[]string{"--version"}, "git version 2.50.1 (Apple Git-155)", "/usr/bin/git"},
		"govulncheck": {[]string{"-version"}, "govulncheck@v1.1.4", ""},
		"gosec":       {[]string{"-version"}, "Version: 2.28.0", ""},
		"gitleaks":    {[]string{"version"}, "8.30.1", ""},
		"pb":          {[]string{"--version"}, "0.1.0-beta.1584", ""},
	}
	verified := make(map[string]any, len(buildInfo.Tools)+1)
	if len(buildInfo.Tools) != len(required) {
		return nil, nil, errors.New("controller tool identity set is incomplete")
	}
	for name, spec := range required {
		identity, exists := buildInfo.Tools[name]
		if !exists || !equalStrings(identity.VersionArgv, spec.argv) || !strings.Contains(identity.VersionOutput, spec.needle) || spec.exactPath != "" && identity.Path != spec.exactPath {
			return nil, nil, fmt.Errorf("tool %s does not match the signed identity", name)
		}
		if !filepath.IsAbs(identity.Path) || !validHexSHA256(identity.SHA256) || len(identity.VersionArgv) == 0 {
			return nil, nil, fmt.Errorf("tool %s build identity is incomplete", name)
		}
		binary, err := readRegularFile(identity.Path, 512<<20)
		if err != nil || sha256Hex(binary) != identity.SHA256 {
			return nil, nil, fmt.Errorf("tool %s binary fingerprint changed", name)
		}
		command := exec.Command(identity.Path, identity.VersionArgv...)
		command.Env = minimalOwnerEnvironment()
		output, err := command.CombinedOutput()
		if err != nil || strings.TrimSpace(string(output)) != strings.TrimSpace(identity.VersionOutput) {
			return nil, nil, fmt.Errorf("tool %s version identity changed", name)
		}
		verified[name] = map[string]any{"path": identity.Path, "sha256": identity.SHA256, "version_output": strings.TrimSpace(string(output))}
	}
	verified["codex_in_app_browser"] = map[string]any{"bundle": buildInfo.BrowserBundle, "chromium": buildInfo.Chromium}
	record := map[string]any{"schema_version": "mindline-tool-identities/v1", "tools": verified}
	path, err := writeAttemptArtifact(attempt, "tool-identities.json", record)
	return []string{path}, record, err
}

func verifyProofRoots(attempt *ProofAttempt) ([]string, map[string]any, error) {
	for _, path := range []string{attempt.ControlRoot, filepath.Join(attempt.ControlRoot, "assurance"), attempt.Root} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
			return nil, nil, fmt.Errorf("proof root is unsafe: %s", path)
		}
	}
	fingerprint, err := fingerprintTree(attempt.ControlRoot, map[string]bool{attempt.Root: true})
	if err != nil {
		return nil, nil, err
	}
	fingerprintPath := filepath.Join(attempt.Root, "control-root-before.sha256")
	if err := writeNoReplaceSynced(fingerprintPath, []byte(fingerprint+"\n"), 0o600); err != nil {
		return nil, nil, err
	}
	record := map[string]any{"schema_version": "mindline-proof-root-verification/v1", "root_mode": "0700", "files_mode": "0600", "no_symlink": true, "control_root_fingerprint": fingerprint}
	path, err := writeAttemptArtifact(attempt, "proof-root-verification.json", record)
	return []string{fingerprintPath, path}, record, err
}

func exerciseBaselineCommand(attempt *ProofAttempt, populated bool) ([]string, map[string]any, error) {
	binary := filepath.Join(attempt.Root, "bin", "mindline-prechange")
	if _, err := validateRegularExecutable(binary); err != nil {
		return nil, nil, err
	}
	if populated {
		for _, required := range []string{filepath.Join(attempt.ControlRoot, "control", "settings.json"), filepath.Join(attempt.ControlRoot, "control", "selected-run.json")} {
			if _, err := readPrivateRegular(required, 1<<20); err != nil {
				return nil, nil, errors.New("populated baseline proof requires settings and v0.4 run selection")
			}
		}
	}
	before, err := fingerprintTree(attempt.ControlRoot, map[string]bool{attempt.Root: true})
	if err != nil {
		return nil, nil, err
	}
	result, err := runObservedCommand(binary, []string{"activation", "config-fingerprint"}, minimalOwnerEnvironment(), filepath.Dir(binary))
	if err != nil {
		return nil, nil, err
	}
	after, err := fingerprintTree(attempt.ControlRoot, map[string]bool{attempt.Root: true})
	if err != nil {
		return nil, nil, err
	}
	if before != after || result.ChildEvents != 0 || result.BrowserEvents != 0 || strings.TrimSpace(result.Stdout) == "" {
		return nil, nil, errors.New("baseline safe command changed durable state or spawned a process")
	}
	prefix := "baseline-safe-command"
	if populated {
		prefix = "baseline-populated-owner"
	}
	record := map[string]any{"schema_version": "mindline-baseline-command-proof/v1", "argv": []string{"activation", "config-fingerprint"}, "exit": 0, "stdout": strings.TrimSpace(result.Stdout), "child_process_events": result.ChildEvents, "browser_processes_created": result.BrowserEvents, "before_sha256": before, "after_sha256": after}
	paths := []string{}
	for _, artifact := range []struct {
		name  string
		value any
	}{
		{prefix + "-invocation.json", record}, {prefix + "-process-events.json", map[string]any{"child_process_events": result.ChildEvents, "browser_processes_created": result.BrowserEvents}},
	} {
		path, err := writeAttemptArtifact(attempt, artifact.name, artifact.value)
		if err != nil {
			return nil, nil, err
		}
		paths = append(paths, path)
	}
	beforeName := "baseline-preflight-owner-root-before.sha256"
	afterName := "baseline-preflight-owner-root-after.sha256"
	if populated {
		beforeName = "populated-owner-root-before.sha256"
		afterName = "populated-owner-root-after.sha256"
	}
	for _, value := range []struct{ name, fingerprint string }{{beforeName, before}, {afterName, after}} {
		path := filepath.Join(attempt.Root, value.name)
		if err := writeNoReplaceSynced(path, []byte(value.fingerprint+"\n"), 0o600); err != nil {
			return nil, nil, err
		}
		paths = append(paths, path)
	}
	return paths, record, nil
}

func runStrictGitDiffCheck(attempt *ProofAttempt) ([]string, map[string]any, error) {
	repositoryRoot, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	type invocation struct {
		name string
		args []string
	}
	commands := []invocation{{"git-diff-check", []string{"diff", "--check", "HEAD"}}, {"git-status-porcelain-v2", []string{"status", "--porcelain=v2", "--untracked-files=all"}}, {"git-tree", []string{"rev-parse", "HEAD^{tree}"}}}
	outputs := map[string][]byte{}
	paths := []string{}
	for _, invocation := range commands {
		command := exec.Command("/usr/bin/git", invocation.args...)
		command.Dir = repositoryRoot
		command.Env = minimalOwnerEnvironment()
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		if err := command.Run(); err != nil {
			return nil, nil, fmt.Errorf("%s failed: %s", invocation.name, strings.TrimSpace(stderr.String()))
		}
		outputs[invocation.name] = stdout.Bytes()
		for _, stream := range []struct {
			suffix string
			data   []byte
		}{{"stdout", stdout.Bytes()}, {"stderr", stderr.Bytes()}} {
			if invocation.name == "git-tree" {
				continue
			}
			path := filepath.Join(attempt.Root, invocation.name+"."+stream.suffix)
			if err := writeNoReplaceSynced(path, stream.data, 0o600); err != nil {
				return nil, nil, err
			}
			paths = append(paths, path)
		}
	}
	if len(outputs["git-diff-check"]) != 0 || len(outputs["git-status-porcelain-v2"]) != 0 {
		return nil, nil, errors.New("frozen source tree is dirty")
	}
	commit, err := gitValue(repositoryRoot, "rev-parse", "HEAD")
	if err != nil || commit != attempt.State.FrozenCommit {
		return nil, nil, errors.New("frozen commit changed")
	}
	var controller ControllerBootstrapEvidence
	data, err := readPrivateRegular(filepath.Join(attempt.Root, "controller-bootstrap", "controller-bootstrap-evidence.json"), 1<<20)
	if err != nil || decodeStrict(data, &controller) != nil {
		return nil, nil, errors.New("controller tree binding unavailable")
	}
	tree := strings.TrimSpace(string(outputs["git-tree"]))
	if tree != controller.FrozenTree {
		return nil, nil, errors.New("frozen tree changed")
	}
	record := map[string]any{"schema_version": "mindline-git-tree-binding/v1", "head_commit": commit, "head_tree": tree, "status_bytes": 0, "untracked_entries": 0}
	path, err := writeAttemptArtifact(attempt, "git-tree-binding.json", record)
	if err != nil {
		return nil, nil, err
	}
	paths = append(paths, path)
	return paths, record, nil
}

func sealPreLiveCheckpoint(attempt *ProofAttempt, manifest WP46Manifest) ([]string, map[string]any, error) {
	if attempt.State.State != "preparing" {
		return nil, nil, errors.New("checkpoint requires preparing attempt")
	}
	checks := make(map[string]string, len(manifest.ReceiptCheckMap))
	for _, mapping := range manifest.ReceiptCheckMap {
		evidence, err := loadPassingGroupEvidence(attempt.Root, mapping.Group)
		if err != nil {
			return nil, nil, err
		}
		data, _ := json.Marshal(evidence)
		checks[mapping.ReceiptCheck] = sha256Hex(data)
	}
	bindings, err := checkpointBindings(attempt)
	if err != nil {
		return nil, nil, err
	}
	checkpoint := map[string]any{"schema_version": "mindline-pre-live-evidence-checkpoint/v1", "attempt_id": attempt.State.AttemptID, "attempt_generation": attempt.State.AttemptGeneration, "manifest_sha256": WP46ManifestSHA256, "source_commit": attempt.State.FrozenCommit, "receipt_check_evidence": checks, "bindings": bindings}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, nil, err
	}
	data = append(data, '\n')
	fixedPath := filepath.Join(attempt.ControlRoot, "assurance", "pre-live-checkpoint.json")
	if err := writeNoReplaceSynced(fixedPath, data, 0o600); err != nil {
		return nil, nil, err
	}
	attemptPath := filepath.Join(attempt.Root, "pre-live-evidence-checkpoint.json")
	if err := writeNoReplaceSynced(attemptPath, data, 0o600); err != nil {
		return nil, nil, err
	}
	if err := syncDirectory(filepath.Dir(fixedPath)); err != nil {
		return nil, nil, err
	}
	if err := TransitionProofAttempt(attempt, "checkpoint_sealed", "", time.Now()); err != nil {
		return nil, nil, err
	}
	return []string{attemptPath}, map[string]any{"checkpoint_sha256": sha256Hex(data), "fixed_path": fixedPath, "attempt_status_after": "checkpoint_sealed"}, nil
}

func stopTrackedServer(attempt *ProofAttempt, export string, crash bool, artifact string) ([]string, map[string]any, error) {
	registry, path, err := loadProcessRegistry(attempt)
	if err != nil {
		return nil, nil, err
	}
	record, exists := registry.Exports[export]
	if !exists || record.State != "running" || record.PGID <= 0 {
		return nil, nil, fmt.Errorf("process export %s is unavailable", export)
	}
	if err := stopProcessGroup(record.PGID, crash, 5*time.Second); err != nil {
		return nil, nil, err
	}
	if portAcceptingConnections() {
		return nil, nil, errors.New("fixed listener remains reachable after stop")
	}
	record.State = "stopped"
	registry.Exports[export] = record
	if err := writeJSONAtomicSynced(path, registry, 0o600); err != nil {
		return nil, nil, err
	}
	result := map[string]any{"schema_version": "mindline-server-stop-proof/v1", "export": export, "pgid": record.PGID, "signal": map[bool]string{true: "SIGKILL", false: "SIGTERM"}[crash], "process_group_absent": true, "listener_closed": true}
	artifactPath, err := writeAttemptArtifact(attempt, artifact, result)
	return []string{artifactPath}, result, err
}

func containTrackedServers(attempt *ProofAttempt) ([]string, map[string]any, error) {
	registry, path, err := loadProcessRegistry(attempt)
	if errors.Is(err, os.ErrNotExist) {
		if portAcceptingConnections() {
			return nil, nil, errors.New("untracked fixed listener is reachable")
		}
		result := map[string]any{"schema_version": "mindline-server-containment/v1", "tracked_processes": 0, "server_process_absent": true, "listener_closed": true, "further_provider_use_observed_zero": true}
		artifact, writeErr := writeAttemptArtifact(attempt, "post-private-server-containment.json", result)
		return []string{artifact}, result, writeErr
	}
	if err != nil {
		return nil, nil, err
	}
	stopped := 0
	for name, record := range registry.Exports {
		if record.State != "running" {
			continue
		}
		if err := stopProcessGroup(record.PGID, false, 5*time.Second); err != nil {
			return nil, nil, err
		}
		record.State = "stopped"
		registry.Exports[name] = record
		stopped++
	}
	if err := writeJSONAtomicSynced(path, registry, 0o600); err != nil {
		return nil, nil, err
	}
	if portAcceptingConnections() {
		return nil, nil, errors.New("fixed listener remains reachable after containment")
	}
	result := map[string]any{"schema_version": "mindline-server-containment/v1", "tracked_processes_stopped": stopped, "server_process_absent": true, "launcher_process_absent": true, "process_group_absent": true, "listener_closed": true, "further_provider_use_observed_zero": true}
	artifact, err := writeAttemptArtifact(attempt, "post-private-server-containment.json", result)
	return []string{artifact}, result, err
}

func resolvePrivateRunBinding(attempt *ProofAttempt) ([]string, map[string]any, error) {
	producer, err := loadPassingGroupEvidence(attempt.Root, "private_capped_founder_proof")
	if err != nil {
		return nil, nil, err
	}
	runRoot := filepath.Clean(producer.Exports["selected_run_root"])
	runsRoot := filepath.Join(attempt.ControlRoot, "runs")
	relative, err := filepath.Rel(runsRoot, runRoot)
	if err != nil || relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, nil, errors.New("private run export escapes the control root")
	}
	info, err := os.Lstat(runRoot)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("private run export is not an immutable owner-only run")
	}
	selection, err := readPrivateRegular(filepath.Join(attempt.ControlRoot, "control", "selected-run.json"), 1<<20)
	if err != nil {
		return nil, nil, err
	}
	if !bytes.Contains(selection, []byte(`"selected_run_id":"`+filepath.Base(runRoot)+`"`)) {
		return nil, nil, errors.New("private run is not the explicit selected run")
	}
	record := map[string]any{"schema_version": "mindline-private-run-binding/v1", "binding": "PRIVATE_RUN_ROOT", "path": runRoot, "run_id": filepath.Base(runRoot), "producer_group": producer.ID, "producer_evidence_sha256": sha256Hex(mustJSON(producer))}
	path, err := writeAttemptArtifact(attempt, "private-run-binding.json", record)
	return []string{path}, record, err
}

func closeProofAttempt(attempt *ProofAttempt, manifest WP46Manifest) error {
	for _, id := range manifest.PreclosureRequiredGroups {
		if _, err := loadPassingGroupEvidence(attempt.Root, id); err != nil {
			return failProofAttempt(attempt, fmt.Errorf("preclosure group %s: %w", id, err))
		}
	}
	repositoryRoot, err := os.Getwd()
	if err != nil {
		return failProofAttempt(attempt, err)
	}
	commit, err := gitValue(repositoryRoot, "rev-parse", "HEAD")
	if err != nil || commit != attempt.State.FrozenCommit {
		return failProofAttempt(attempt, errors.New("final commit revalidation failed"))
	}
	status := exec.Command("/usr/bin/git", "status", "--porcelain=v2", "--untracked-files=all")
	status.Dir = repositoryRoot
	status.Env = minimalOwnerEnvironment()
	output, err := status.Output()
	if err != nil || len(output) != 0 {
		return failProofAttempt(attempt, errors.New("final source tree is not clean"))
	}
	first, err := fingerprintTree(attempt.Root, map[string]bool{filepath.Join(attempt.Root, "proof-evidence-index.json"): true, filepath.Join(attempt.Root, "proof-failure-ledger.json"): true})
	if err != nil {
		return failProofAttempt(attempt, err)
	}
	second, err := fingerprintTree(attempt.Root, map[string]bool{filepath.Join(attempt.Root, "proof-evidence-index.json"): true, filepath.Join(attempt.Root, "proof-failure-ledger.json"): true})
	if err != nil || first != second {
		return failProofAttempt(attempt, errors.New("immutable proof set changed during final revalidation"))
	}
	bindings, err := checkpointBindings(attempt)
	if err != nil {
		return failProofAttempt(attempt, err)
	}
	index := map[string]any{"schema_version": "mindline-proof-evidence-index/v1", "attempt_id": attempt.State.AttemptID, "attempt_generation": attempt.State.AttemptGeneration, "manifest_sha256": WP46ManifestSHA256, "source_commit": commit, "immutable_revalidation_set_fingerprint": first, "bindings": bindings}
	indexBytes := mustJSON(index)
	indexHash := sha256Hex(indexBytes)
	attempt.State.ImmutableRevalidationSetSHA256 = first
	if err := TransitionProofAttempt(attempt, "closing", "", time.Now()); err != nil {
		return failProofAttempt(attempt, err)
	}
	indexPath := filepath.Join(attempt.Root, "proof-evidence-index.json")
	if err := writeNoReplaceSynced(indexPath, indexBytes, 0o600); err != nil {
		return failProofAttempt(attempt, err)
	}
	if err := syncDirectory(attempt.Root); err != nil {
		return failProofAttempt(attempt, err)
	}
	post, err := fingerprintTree(attempt.Root, map[string]bool{indexPath: true, filepath.Join(attempt.Root, "proof-failure-ledger.json"): true})
	if err != nil || post != first {
		return failProofAttempt(attempt, errors.New("post-index immutable proof set changed"))
	}
	return TransitionProofAttempt(attempt, "succeeded", indexHash, time.Now())
}

func failProofAttempt(attempt *ProofAttempt, cause error) error {
	_ = os.Remove(filepath.Join(attempt.Root, "proof-evidence-index.json"))
	_ = os.Remove(filepath.Join(attempt.ControlRoot, "assurance", "pre-live-receipt.json"))
	ledger := map[string]any{"schema_version": "mindline-proof-failure-ledger/v1", "attempt_id": attempt.State.AttemptID, "error": cause.Error(), "failed_at": time.Now().UTC().Format(time.RFC3339Nano)}
	_, _ = writeAttemptArtifact(attempt, "proof-failure-ledger.json", ledger)
	_ = TransitionProofAttempt(attempt, "failed", "", time.Now())
	return cause
}

func buildPassingGroupEvidence(root string, group ManifestGroup, started, completed time.Time, artifactPaths []string, details map[string]any) (ProofGroupEvidence, error) {
	artifacts := make([]ProofArtifactEvidence, 0, len(artifactPaths))
	for _, path := range artifactPaths {
		data, err := readPrivateRegular(path, 512<<20)
		if err != nil {
			return ProofGroupEvidence{}, err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
			return ProofGroupEvidence{}, errors.New("group artifact escapes attempt root")
		}
		artifacts = append(artifacts, ProofArtifactEvidence{filepath.ToSlash(relative), sha256Hex(data), int64(len(data))})
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	detailBytes, err := json.Marshal(details)
	if err != nil {
		return ProofGroupEvidence{}, err
	}
	return ProofGroupEvidence{ProofGroupEvidenceSchema, group.ID, group.Phase, []string{"group", group.ID}, "pass", started.Format(time.RFC3339Nano), completed.Format(time.RFC3339Nano), artifacts, detailBytes, nil}, nil
}

func loadPassingGroupEvidence(root, id string) (ProofGroupEvidence, error) {
	data, err := readPrivateRegular(filepath.Join(root, "groups", id+".json"), 4<<20)
	if err != nil {
		return ProofGroupEvidence{}, err
	}
	var evidence ProofGroupEvidence
	if err := decodeStrict(data, &evidence); err != nil {
		return ProofGroupEvidence{}, err
	}
	manifest, parseErr := ParseWP46Manifest(embeddedWP46Manifest)
	if parseErr != nil {
		return ProofGroupEvidence{}, parseErr
	}
	var expectedArgv []string
	found := false
	for _, group := range manifest.Groups {
		if group.ID == id {
			expectedArgv = group.Argv
			found = true
			break
		}
	}
	if !found || evidence.SchemaVersion != ProofGroupEvidenceSchema || evidence.ID != id || evidence.Outcome != "pass" || !equalStrings(evidence.Argv, expectedArgv) {
		return ProofGroupEvidence{}, errors.New("group evidence identity mismatch")
	}
	for _, artifact := range evidence.Artifacts {
		path := filepath.Join(root, filepath.FromSlash(artifact.Path))
		data, err := readPrivateRegular(path, 512<<20)
		if err != nil || int64(len(data)) != artifact.Bytes || sha256Hex(data) != artifact.SHA256 {
			return ProofGroupEvidence{}, fmt.Errorf("group artifact %s changed", artifact.Path)
		}
	}
	return evidence, nil
}

func checkpointBindings(attempt *ProofAttempt) (map[string]string, error) {
	paths := map[string]string{"proof_runner": filepath.Join(attempt.Root, "controller-bootstrap", "proof-runner.sha256"), "controller_bootstrap": filepath.Join(attempt.Root, "controller-bootstrap", "controller-bootstrap-evidence.json"), "server_binary": filepath.Join(attempt.Root, "bin", "mindline-frozen"), "operator_launcher": filepath.Join(attempt.Root, "bin", "mindline-operator-launcher")}
	bindings := map[string]string{}
	for name, path := range paths {
		data, err := readPrivateRegular(path, 512<<20)
		if err != nil {
			return nil, err
		}
		if name == "proof_runner" {
			bindings[name] = strings.TrimSpace(string(data))
		} else {
			bindings[name] = sha256Hex(data)
		}
	}
	bindings["namespace_marker"] = attempt.State.NamespaceMarkerSHA256
	return bindings, nil
}

func loadProcessRegistry(attempt *ProofAttempt) (proofProcessRegistry, string, error) {
	path := filepath.Join(attempt.ControlRoot, "assurance", "proof-process-registry.json")
	data, err := readPrivateRegular(path, 1<<20)
	if err != nil {
		return proofProcessRegistry{}, path, err
	}
	var registry proofProcessRegistry
	if err := decodeStrict(data, &registry); err != nil {
		return proofProcessRegistry{}, path, err
	}
	if registry.SchemaVersion != ProofProcessRegistrySchema || registry.AttemptID != attempt.State.AttemptID || registry.AttemptGeneration != attempt.State.AttemptGeneration {
		return proofProcessRegistry{}, path, errors.New("process registry belongs to another attempt")
	}
	return registry, path, nil
}

func writeAttemptArtifact(attempt *ProofAttempt, relative string, value any) (string, error) {
	path := filepath.Join(attempt.Root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(attempt.Root, path)
	if err != nil || rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("attempt artifact path escapes root")
	}
	if err := ensurePrivateDirectory(filepath.Dir(path)); err != nil {
		return "", err
	}
	if err := writeJSONNoReplaceSynced(path, value, 0o600); err != nil {
		return "", err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return "", err
	}
	return path, nil
}

func readPrivateRegular(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maximum {
		return nil, errors.New("private artifact is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maximum+1))
}

func readRegularFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maximum {
		return nil, errors.New("regular file identity is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maximum+1))
}
func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("JSON contains trailing data")
	}
	return nil
}
func parseRunnerBuildInfo(data []byte) (ProofRunnerBuildInfo, error) {
	var value ProofRunnerBuildInfo
	if err := decodeStrict(data, &value); err != nil {
		return value, err
	}
	if value.SchemaVersion != ProofRunnerBuildInfoSchema || !fullGitCommitPattern.MatchString(value.FrozenCommit) || !fullGitCommitPattern.MatchString(value.FrozenTree) || value.Tools == nil {
		return value, errors.New("proof runner build info is invalid")
	}
	return value, nil
}
func validateRegularExecutable(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode()&0o111 == 0 {
		return nil, errors.New("required executable is unsafe")
	}
	return info, nil
}
func minimalOwnerEnvironment() []string {
	values := []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C", "LC_ALL=C"}
	for _, name := range []string{"HOME", "TMPDIR"} {
		if value := os.Getenv(name); value != "" {
			values = append(values, name+"="+value)
		}
	}
	return values
}
func portAcceptingConnections() bool {
	connection, err := net.DialTimeout("tcp4", "127.0.0.1:9876", 150*time.Millisecond)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}
func mustJSON(value any) []byte { data, _ := json.Marshal(value); return append(data, '\n') }

func fingerprintTree(root string, excluded map[string]bool) (string, error) {
	root = filepath.Clean(root)
	hash := sha256.New()
	files := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		path = filepath.Clean(path)
		for excludedPath := range excluded {
			if path == excludedPath || strings.HasPrefix(path, excludedPath+string(filepath.Separator)) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if path == root {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("snapshot rejects symlink")
		}
		relative, _ := filepath.Rel(root, path)
		if info.IsDir() {
			if info.Mode().Perm() != 0o700 {
				return errors.New("snapshot directory is not owner-only")
			}
			_, _ = fmt.Fprintf(hash, "D\x00%s\x00%o\n", filepath.ToSlash(relative), info.Mode().Perm())
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return errors.New("snapshot rejects unsafe file")
		}
		files++
		if files > 100000 {
			return errors.New("snapshot file limit exceeded")
		}
		data, err := readPrivateRegular(path, 512<<20)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		_, _ = fmt.Fprintf(hash, "F\x00%s\x00%o\x00%d\x00%s\n", filepath.ToSlash(relative), info.Mode().Perm(), len(data), hex.EncodeToString(digest[:]))
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
	}
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
