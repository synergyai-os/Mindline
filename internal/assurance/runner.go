package assurance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

const FixedGateRunnerVersion = "mindline-fixed-pre-live-runner/v0.2"

type gateSurface uint8

const (
	sourceCheckout gateSurface = iota
	cleanHEADExport
	runtimeSnapshot
)

type gateCommand struct {
	name          string
	executable    string
	versionArgs   []string
	versionNeedle string
	args          []string
	surface       gateSurface
}

// gosec has no severity above HIGH. Requiring both HIGH severity and HIGH
// confidence is the explicit pre-live policy: legacy LOW/MEDIUM findings remain
// visible outside this authority gate, while a verified HIGH finding or any
// scanner error blocks authority.
var gosecPreLivePolicy = []string{"-quiet", "-fmt=json", "-severity=high", "-confidence=high", "-exclude-generated", "./..."}

var fixedGateCommands = []gateCommand{
	{name: "go_test", executable: "go", versionArgs: []string{"version"}, args: []string{"test", "-count=1", "./..."}},
	{name: "targeted_race", executable: "go", versionArgs: []string{"version"}, args: []string{"test", "-race", "-count=1", "./internal/acquisition/slack", "./internal/activationapp", "./internal/controlui", "./internal/integrations", "./internal/orchestration", "./internal/processing", "./internal/productbrain", "./internal/retrieval", "./internal/privateio", "./internal/runjournal"}},
	{name: "go_vet", executable: "go", versionArgs: []string{"version"}, args: []string{"vet", "./..."}},
	{name: "git_diff_check", executable: "git", versionArgs: []string{"--version"}, args: []string{"diff", "--check", "HEAD"}},
	{name: "govulncheck", executable: "govulncheck", versionArgs: []string{"-version"}, versionNeedle: "govulncheck@v1.1.4", args: []string{"./..."}},
	{name: "gosec", executable: "gosec", versionArgs: []string{"-version"}, versionNeedle: "Version: 2.28.0", args: gosecPreLivePolicy},
	{name: "gitleaks_clean_head", executable: "gitleaks", versionArgs: []string{"version"}, versionNeedle: "8.30.1", args: []string{"dir", "--redact", "--no-banner", "."}, surface: cleanHEADExport},
	{name: "gitleaks_history", executable: "gitleaks", versionArgs: []string{"version"}, versionNeedle: "8.30.1", args: []string{"git", "--redact", "--no-banner", "."}},
	{name: "gitleaks_runtime_surface", executable: "gitleaks", versionArgs: []string{"version"}, versionNeedle: "8.30.1", args: []string{"dir", "--redact", "--no-banner", "."}, surface: runtimeSnapshot},
	{name: "sentinel_surface_scan", executable: "go", versionArgs: []string{"version"}, args: []string{"test", "-count=1", "./internal/acquisition/...", "./internal/activationapp", "./internal/controlui", "./internal/integrations", "./internal/privateio", "./internal/processing", "./internal/productbrain", "./internal/retrieval", "./internal/runjournal", "-run", "Secret|Privacy|Sentinel|Credential|Bootstrap|Session|CSRF"}},
	{name: "hardened_browser", executable: "go", versionArgs: []string{"version"}, args: []string{"test", "-count=1", "./internal/controlui", "./internal/activationapp"}},
	{name: "productbrain_crash_replay", executable: "go", versionArgs: []string{"version"}, args: []string{"test", "-count=1", "./internal/productbrain", "-run", "Test(DeliverApprovedSealsV02AuthorityAndReplaysWithoutMutation|ApprovedAttemptIsDurableBeforeSendAndCrashReconcilesWithoutDuplicate|CancellationAfterReservedMutationAllowsOnlyReconciliation)$"}},
	{name: "activation_journal_recovery", executable: "go", versionArgs: []string{"version"}, args: []string{"test", "-count=1", "./internal/activationapp", "./internal/runjournal", "-run", "Test(RunJournalRestoresMissingActivationProjectionFromImmutableSnapshot|PersistedProjectionWithoutJournalFailsClosed|StoreAppendLoadCASProjectionAndTamper)$"}},
	{name: "legacy_compatibility", executable: "go", versionArgs: []string{"version"}, args: []string{"test", "-count=1", "./internal/cli", "./internal/evalreadback", "./internal/evalproof"}},
}

type commandExecutor func(string, []string, string) ([]byte, error)

type gateRunContext struct {
	sourceRoot             string
	cleanHEADRoot          string
	cleanHEADBinding       string
	runtimeSnapshotRoot    string
	runtimeSnapshotBinding string
}

func RunFixedGate(workdir, revision, runtimeRoot string) ([]Check, error) {
	cleanExport, err := createCleanHEADExport(workdir, revision)
	if err != nil {
		return nil, err
	}
	runtimeExport, err := createRuntimeSnapshot(runtimeRoot)
	if err != nil {
		if cleanupErr := cleanExport.cleanup(); cleanupErr != nil {
			return nil, fmt.Errorf("runtime snapshot failed: %v; clean HEAD cleanup failed: %w", err, cleanupErr)
		}
		return nil, err
	}
	context := gateRunContext{
		sourceRoot: workdir, cleanHEADRoot: cleanExport.root, cleanHEADBinding: cleanExport.binding,
		runtimeSnapshotRoot: runtimeExport.root, runtimeSnapshotBinding: runtimeExport.binding,
	}
	checks, runErr := runFixedGate(context, executeBounded)
	runtimeCleanupErr := runtimeExport.cleanup()
	cleanCleanupErr := cleanExport.cleanup()
	if runErr != nil {
		return nil, runErr
	}
	if runtimeCleanupErr != nil || cleanCleanupErr != nil {
		return nil, fmt.Errorf("pre-live private scan surface cleanup failed: runtime=%v clean_head=%v", runtimeCleanupErr, cleanCleanupErr)
	}
	return checks, nil
}

func fixedGatePlanFingerprint() string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(FixedGateRunnerVersion))
	for _, spec := range fixedGateCommands {
		_, _ = hash.Write([]byte(fmt.Sprintf("\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d", spec.name, spec.executable, strings.Join(spec.versionArgs, "\x00"), spec.versionNeedle, strings.Join(spec.args, "\x00"), spec.surface)))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func runFixedGate(context gateRunContext, execute commandExecutor) ([]Check, error) {
	if strings.TrimSpace(context.sourceRoot) == "" || strings.TrimSpace(context.cleanHEADRoot) == "" ||
		strings.TrimSpace(context.cleanHEADBinding) == "" || strings.TrimSpace(context.runtimeSnapshotRoot) == "" ||
		strings.TrimSpace(context.runtimeSnapshotBinding) == "" || execute == nil {
		return nil, errors.New("fixed pre-live gate requires a working directory")
	}
	checks := make([]Check, 0, len(fixedGateCommands))
	for _, spec := range fixedGateCommands {
		workdir, surfaceBinding := context.forSurface(spec.surface)
		versionOutput, err := execute(spec.executable, spec.versionArgs, workdir)
		if err != nil || strings.TrimSpace(string(versionOutput)) == "" || spec.versionNeedle != "" && !strings.Contains(string(versionOutput), spec.versionNeedle) {
			return nil, fmt.Errorf("pre-live check %s could not verify its tool", spec.name)
		}
		started := time.Now().UTC()
		output, err := execute(spec.executable, spec.args, workdir)
		ended := time.Now().UTC()
		if err != nil {
			return nil, fmt.Errorf("pre-live check %s failed", spec.name)
		}
		digest := sha256.Sum256([]byte(strings.Join([]string{
			FixedGateRunnerVersion, spec.name, spec.executable, strings.Join(spec.args, "\x00"),
			strings.TrimSpace(string(versionOutput)), started.Format(time.RFC3339Nano), ended.Format(time.RFC3339Nano),
			surfaceBinding, hex.EncodeToString(hashBytes(output)),
		}, "\x00")))
		checks = append(checks, Check{Name: spec.name, ToolVersion: boundedVersion(versionOutput), Outcome: "pass", EvidenceFingerprint: "sha256:" + hex.EncodeToString(digest[:])})
	}
	return checks, nil
}

func (context gateRunContext) forSurface(surface gateSurface) (string, string) {
	switch surface {
	case cleanHEADExport:
		return context.cleanHEADRoot, context.cleanHEADBinding
	case runtimeSnapshot:
		return context.runtimeSnapshotRoot, context.runtimeSnapshotBinding
	default:
		return context.sourceRoot, "source-checkout"
	}
}

func executeBounded(name string, args []string, workdir string) ([]byte, error) {
	executable, err := exec.LookPath(name)
	if err != nil {
		return nil, err
	}
	identity, err := validateExecutableIdentity(name, executable)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = workdir
	command.Env = fixedGateEnvironment(workdir, executable)
	configureProcessGroup(command)
	command.Cancel = func() error { return killProcessGroup(command) }
	command.WaitDelay = 5 * time.Second
	var output limitedBuffer
	command.Stdout = &output
	command.Stderr = &output
	err = command.Run()
	if output.exceeded {
		return nil, errors.New("pre-live check output exceeded limit")
	}
	return append([]byte(identity+"\n"), output.Bytes()...), err
}

func validateExecutableIdentity(name, path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := file.WriteTo(hash); err != nil {
		_ = file.Close()
		return "", err
	}
	_ = file.Close()
	fingerprint := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	switch name {
	case "go":
		expected, _ := filepath.EvalSymlinks(filepath.Join(runtime.GOROOT(), "bin", "go"))
		actual, _ := filepath.EvalSymlinks(path)
		if expected == "" || actual != expected {
			return "", errors.New("pre-live Go executable is not the build toolchain")
		}
	case "git":
		actual, _ := filepath.EvalSymlinks(path)
		if runtime.GOOS == "darwin" && actual != "/usr/bin/git" {
			return "", errors.New("pre-live Git executable is not the system tool")
		}
	case "govulncheck", "gosec", "gitleaks":
		info, readErr := buildinfo.ReadFile(path)
		if readErr != nil {
			return "", errors.New("pre-live scanner build identity unavailable")
		}
		if !supportedScannerBuild(name, info) {
			return "", errors.New("pre-live scanner build identity unsupported")
		}
	default:
		return "", errors.New("pre-live executable is not allowlisted")
	}
	return name + " executable " + fingerprint, nil
}

func supportedScannerBuild(name string, info *debug.BuildInfo) bool {
	if info == nil {
		return false
	}
	switch name {
	case "govulncheck":
		return info.Path == "golang.org/x/vuln/cmd/govulncheck" && info.Main.Path == "golang.org/x/vuln" && info.Main.Version == "v1.1.4"
	case "gosec":
		return info.Path == "github.com/securego/gosec/v2/cmd/gosec" && info.Main.Path == "github.com/securego/gosec/v2" && info.Main.Version == "v2.28.0"
	case "gitleaks":
		if info.Path != "command-line-arguments" {
			return false
		}
		for _, setting := range info.Settings {
			if setting.Key == "-ldflags" && strings.Contains(setting.Value, "version.Version=8.30.1") {
				return true
			}
		}
	}
	return false
}

func fixedGateEnvironment(workdir, executable string) []string {
	digest := sha256.Sum256([]byte(filepath.Clean(workdir)))
	root := filepath.Join(os.TempDir(), "mindline-assurance-"+hex.EncodeToString(digest[:8]))
	for _, path := range []string{root, filepath.Join(root, "home"), filepath.Join(root, "tmp"), filepath.Join(root, "gocache"), filepath.Join(root, "gomodcache"), filepath.Join(root, "gopath")} {
		_ = os.MkdirAll(path, 0o700)
	}
	path := strings.Join([]string{filepath.Dir(executable), "/usr/local/bin", "/opt/homebrew/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"}, string(os.PathListSeparator))
	return []string{
		"HOME=" + filepath.Join(root, "home"), "TMPDIR=" + filepath.Join(root, "tmp"), "PATH=" + path,
		"GOCACHE=" + filepath.Join(root, "gocache"), "GOMODCACHE=" + filepath.Join(root, "gomodcache"), "GOPATH=" + filepath.Join(root, "gopath"),
		"GOPROXY=https://proxy.golang.org,direct", "GOSUMDB=sum.golang.org", "GOTOOLCHAIN=local", "GOVULNDB=https://vuln.go.dev",
		"LANG=C", "LC_ALL=C", "NO_PROXY=127.0.0.1,localhost",
	}
}

const expectedModulePath = "github.com/synergyai-os/Mindline"

func VerifySourceBinding(workdir, revision string) (string, error) {
	if !validGitRevision(revision) {
		return "", errors.New("source binding requires a full hexadecimal revision")
	}
	workdir, err := filepath.Abs(workdir)
	if err != nil {
		return "", err
	}
	runGit := func(args ...string) (string, error) {
		command := exec.Command("git", append([]string{"-C", workdir}, args...)...)
		command.Env = fixedGateEnvironment(workdir, "/usr/bin/git")
		value, commandErr := command.Output()
		return strings.TrimSpace(string(value)), commandErr
	}
	root, err := runGit("rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("source checkout root unavailable")
	}
	resolvedRoot, rootErr := filepath.EvalSymlinks(root)
	resolvedWorkdir, workdirErr := filepath.EvalSymlinks(workdir)
	if rootErr != nil || workdirErr != nil || resolvedRoot != resolvedWorkdir {
		return "", errors.New("pre-live gate must run at the repository root")
	}
	head, err := runGit("rev-parse", "HEAD")
	if err != nil || head != strings.TrimSpace(revision) {
		return "", errors.New("source checkout does not match the clean binary revision")
	}
	status, err := runGit("status", "--porcelain=v1", "--untracked-files=all")
	if err != nil || status != "" {
		return "", errors.New("source checkout contains modified or untracked inputs")
	}
	goMod, err := os.ReadFile(filepath.Join(workdir, "go.mod"))
	if err != nil || len(goMod) > 1<<20 {
		return "", errors.New("source module identity unavailable")
	}
	module := ""
	for _, line := range strings.Split(string(goMod), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			module = fields[1]
			break
		}
	}
	if module != expectedModulePath {
		return "", errors.New("source module identity mismatch")
	}
	binding := struct {
		Schema, RepositoryRoot, Revision, Module string
	}{"mindline-source-binding/v0.1", resolvedRoot, head, module}
	encoded, _ := json.Marshal(binding)
	fingerprint := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(fingerprint[:]), nil
}

type limitedBuffer struct {
	bytes.Buffer
	exceeded bool
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
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

func boundedVersion(value []byte) string {
	version := strings.TrimSpace(string(value))
	if len(version) > 256 {
		version = version[:256]
	}
	return version
}

func hashBytes(value []byte) []byte {
	digest := sha256.Sum256(value)
	return digest[:]
}
