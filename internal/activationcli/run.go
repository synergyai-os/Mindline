package activationcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	neturl "net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/synergyai-os/Mindline/internal/activationapp"
	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/controlui"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const Usage = "usage: mindline activation config-fingerprint\nusage: mindline activation gate-receipt --runtime <private-dir>\nusage: mindline activation serve --runtime <private-dir> --receipt <receipt.json> --open\n"

var readBuildInfo = debug.ReadBuildInfo
var runGatePlan = assurance.RunFixedGate
var verifySourceBinding = assurance.VerifySourceBinding

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, Usage)
		return 1
	}
	switch args[0] {
	case "config-fingerprint":
		if len(args) != 1 {
			fmt.Fprint(stderr, Usage)
			return 1
		}
		fmt.Fprintln(stdout, activationapp.DefaultConfigurationFingerprint())
		return 0
	case "gate-receipt":
		return runGateReceipt(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	default:
		fmt.Fprint(stderr, Usage)
		return 1
	}
}

func runGateReceipt(args []string, stdout, stderr io.Writer) int {
	runtimeRoot, ok := parseOnePath(args, "--runtime")
	if !ok {
		fmt.Fprint(stderr, Usage)
		return 1
	}
	revision, err := cleanBuildRevision()
	if err != nil {
		fmt.Fprintf(stderr, "pre-live receipt blocked: %v\n", err)
		return 2
	}
	if err := privateio.PrepareDir(runtimeRoot); err != nil {
		fmt.Fprintf(stderr, "pre-live receipt blocked: %v\n", err)
		return 2
	}
	workdir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pre-live receipt blocked: %v\n", err)
		return 2
	}
	sourceBinding, err := verifySourceBinding(workdir, revision)
	if err != nil {
		fmt.Fprintf(stderr, "pre-live receipt blocked: %v\n", err)
		return 2
	}
	checks, err := runGatePlan(workdir, revision, runtimeRoot)
	if err != nil {
		fmt.Fprintf(stderr, "pre-live receipt blocked: %v\n", err)
		return 2
	}
	finalSourceBinding, err := verifySourceBinding(workdir, revision)
	if err != nil {
		fmt.Fprintf(stderr, "pre-live receipt blocked: post-check source verification failed: %v\n", err)
		return 2
	}
	if finalSourceBinding != sourceBinding {
		fmt.Fprintln(stderr, "pre-live receipt blocked: source binding changed during checks")
		return 2
	}
	receipt, err := assurance.Build(revision, activationapp.DefaultConfigurationFingerprint(), finalSourceBinding, time.Now().UTC(), checks)
	if err != nil {
		fmt.Fprintf(stderr, "pre-live receipt blocked: %v\n", err)
		return 2
	}
	path := filepath.Join(runtimeRoot, "pre-live-receipt.json")
	if err := assurance.Write(runtimeRoot, path, receipt); err != nil {
		fmt.Fprintf(stderr, "pre-live receipt blocked: %v\n", err)
		return 2
	}
	_ = json.NewEncoder(stdout).Encode(map[string]any{"receipt_path": path, "receipt_fingerprint": receipt.Fingerprint, "commit": revision})
	return 0
}

func parseOnePath(args []string, flag string) (string, bool) {
	if len(args) != 2 || args[0] != flag || strings.TrimSpace(args[1]) == "" || !filepath.IsAbs(args[1]) {
		return "", false
	}
	return filepath.Clean(args[1]), true
}

func runServe(args []string, stdout, stderr io.Writer) int {
	runtimeRoot, receiptPath, ok := parseServePaths(args)
	if !ok {
		fmt.Fprint(stderr, Usage)
		return 1
	}
	revision, err := cleanBuildRevision()
	if err != nil {
		fmt.Fprintf(stderr, "activation blocked: %v\n", err)
		return 2
	}
	receipt, err := assurance.Load(runtimeRoot, receiptPath)
	if err != nil {
		fmt.Fprintf(stderr, "activation blocked: %v\n", err)
		return 2
	}
	app, err := activationapp.New(activationapp.Options{
		RuntimeRoot: runtimeRoot, Commit: revision, ConfigurationFingerprint: activationapp.DefaultConfigurationFingerprint(),
		PreLiveReceipt: &receipt, ReceiptMaxAge: 30 * time.Minute,
	})
	if err != nil {
		fmt.Fprintf(stderr, "activation blocked: %v\n", err)
		return 2
	}
	defer app.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	running, err := controlui.Launch(ctx, app, runtimeRoot, systemBrowserOpener{})
	if err != nil {
		fmt.Fprintf(stderr, "activation blocked: %v\n", err)
		return 2
	}
	defer running.Shutdown(context.Background())
	_ = json.NewEncoder(stdout).Encode(map[string]any{"safe_url": running.SafeDisplayURL(), "runtime_root": runtimeRoot})
	<-ctx.Done()
	return 0
}

func parseServePaths(args []string) (string, string, bool) {
	if len(args) == 5 && args[4] == "--open" {
		return parseTwoPaths(args[:4], "--runtime", "--receipt")
	}
	if len(args) == 5 && args[0] == "--open" {
		return parseTwoPaths(args[1:], "--runtime", "--receipt")
	}
	return "", "", false
}

type systemBrowserOpener struct{}

func (systemBrowserOpener) Open(url string) error {
	target, err := neturl.Parse(url)
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.User != nil || target.Path != "/" || target.RawQuery != "" || target.Port() == "" || target.Host != net.JoinHostPort("127.0.0.1", target.Port()) {
		return errors.New("refusing unsafe browser bootstrap target")
	}
	fragment, err := neturl.ParseQuery(target.Fragment)
	if err != nil || len(fragment) != 1 || len(fragment["bootstrap"]) != 1 || len(fragment.Get("bootstrap")) < 32 {
		return errors.New("refusing unsafe browser bootstrap target")
	}
	return exec.Command("/usr/bin/open", url).Run()
}

func parseTwoPaths(args []string, firstFlag, secondFlag string) (string, string, bool) {
	if len(args) != 4 {
		return "", "", false
	}
	values := map[string]string{}
	for index := 0; index < len(args); index += 2 {
		if args[index] != firstFlag && args[index] != secondFlag || strings.TrimSpace(args[index+1]) == "" {
			return "", "", false
		}
		values[args[index]] = args[index+1]
	}
	first, second := values[firstFlag], values[secondFlag]
	if first == "" || second == "" || !filepath.IsAbs(first) || !filepath.IsAbs(second) {
		return "", "", false
	}
	return filepath.Clean(first), filepath.Clean(second), true
}

func cleanBuildRevision() (string, error) {
	info, ok := readBuildInfo()
	if !ok || info == nil {
		return "", errors.New("VCS build identity unavailable")
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	revision := strings.TrimSpace(settings["vcs.revision"])
	if revision == "" {
		return "", errors.New("VCS revision unavailable")
	}
	if settings["vcs.modified"] != "false" {
		return "", errors.New("working tree or build inputs are modified")
	}
	return revision, nil
}
