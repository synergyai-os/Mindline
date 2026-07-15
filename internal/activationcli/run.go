package activationcli

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"strconv"
	"strings"
	"syscall"
	"time"

	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/activationapp"
	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/controlui"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const Usage = "usage: mindline activation config-fingerprint\nusage: mindline activation gate-receipt --runtime <private-dir>\nusage: mindline activation build-slack-manifest --runtime <private-dir> --receipt <receipt.json> --out <manifest.json> --payload-bytes <n> --payload-sha256 <hex>\nusage: mindline activation serve --runtime <private-dir> --receipt <receipt.json> --open\n"

const (
	maximumNativeBatchSize = int64(64 << 20)
)

var readBuildInfo = debug.ReadBuildInfo
var runGatePlan = assurance.RunFixedGate
var verifySourceBinding = assurance.VerifySourceBinding

func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), stdout, stderr)
}

func RunWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
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
	case "build-slack-manifest":
		return runBuildSlackManifest(args[1:], stdin, stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	default:
		fmt.Fprint(stderr, Usage)
		return 1
	}
}

type manifestBuildSummary struct {
	ManifestPath       string `json:"manifest_path"`
	ContentFingerprint string `json:"content_fingerprint"`
	SourceRecords      int    `json:"source_records"`
	URLOccurrences     int    `json:"url_occurrences"`
	CanonicalItems     int    `json:"canonical_items"`
}

func runBuildSlackManifest(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	runtimeRoot, receiptPath, outPath, payloadBytes, payloadDigest, ok := parseBuildSlackManifestArgs(args)
	if !ok {
		fmt.Fprint(stderr, Usage)
		return 1
	}
	revision, err := cleanBuildRevision()
	if err != nil {
		fmt.Fprintf(stderr, "Slack manifest blocked: %v\n", err)
		return 2
	}
	receipt, err := assurance.Load(runtimeRoot, receiptPath)
	if err != nil {
		fmt.Fprintf(stderr, "Slack manifest blocked: %v\n", err)
		return 2
	}
	configuration := activationapp.DefaultConfigurationFingerprint()
	if err := assurance.Validate(receipt, revision, configuration, time.Now().UTC(), 30*time.Minute); err != nil {
		fmt.Fprintf(stderr, "Slack manifest blocked: %v\n", err)
		return 2
	}
	if err := validateManifestOutputPath(runtimeRoot, outPath); err != nil {
		fmt.Fprintf(stderr, "Slack manifest blocked: %v\n", err)
		return 2
	}
	batch, err := decodeNativeSlackBatch(stdin, payloadBytes, payloadDigest)
	if err != nil {
		fmt.Fprintf(stderr, "Slack manifest blocked: %v\n", err)
		return 2
	}
	manifest, err := acquisitionslack.BuildAuthorizedExternalManifestFromNativeBatch(batch, receipt, revision, configuration, time.Now().UTC(), 30*time.Minute)
	if err != nil {
		fmt.Fprintf(stderr, "Slack manifest blocked: %v\n", err)
		return 2
	}
	manifestPayload, err := json.Marshal(manifest)
	if err != nil || int64(len(manifestPayload)+1) > acquisitionslack.DefaultMaximumBytes {
		fmt.Fprintln(stderr, "Slack manifest blocked: normalized manifest exceeds import budget")
		return 2
	}
	if err := privateio.WriteFile(outPath, append(manifestPayload, '\n'), true); err != nil {
		fmt.Fprintf(stderr, "Slack manifest blocked: %v\n", err)
		return 2
	}
	_ = json.NewEncoder(stdout).Encode(manifestBuildSummary{
		ManifestPath: outPath, ContentFingerprint: manifest.ContentFingerprint,
		SourceRecords: len(manifest.SourceRecords), URLOccurrences: len(manifest.URLOccurrences), CanonicalItems: len(manifest.CanonicalItems),
	})
	return 0
}

// The bridge accepts one length- and SHA-bound JSON frame so connector
// orchestration can keep
// native content off argv, environment variables, shell history, and temporary
// raw-export files. Bytes outside the declared frame are never interpreted as
// source records; the declared denominator and pagination evidence are checked
// independently below.
func decodeNativeSlackBatch(input io.Reader, expectedBytes int64, expectedDigest string) (acquisitionslack.NativeBatch, error) {
	if input == nil {
		return acquisitionslack.NativeBatch{}, errors.New("native Slack batch is missing")
	}
	if expectedBytes <= 0 || expectedBytes > maximumNativeBatchSize || len(expectedDigest) != sha256.Size*2 {
		return acquisitionslack.NativeBatch{}, errors.New("native Slack frame binding is invalid")
	}
	payload := make([]byte, expectedBytes)
	if _, err := io.ReadFull(input, payload); err != nil {
		return acquisitionslack.NativeBatch{}, errors.New("native Slack frame is incomplete")
	}
	digest := sha256.Sum256(payload)
	if fmt.Sprintf("%x", digest[:]) != strings.ToLower(expectedDigest) {
		return acquisitionslack.NativeBatch{}, errors.New("native Slack frame fingerprint mismatch")
	}
	if int64(len(payload)) > maximumNativeBatchSize {
		return acquisitionslack.NativeBatch{}, errors.New("native Slack batch exceeds size limit")
	}
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return acquisitionslack.NativeBatch{}, errors.New("native Slack batch is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var batch acquisitionslack.NativeBatch
	if err := decoder.Decode(&batch); err != nil {
		return acquisitionslack.NativeBatch{}, errors.New("native Slack batch is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return acquisitionslack.NativeBatch{}, errors.New("native Slack batch contains trailing data")
	}
	return batch, nil
}

func validateManifestOutputPath(root, outPath string) error {
	if err := privateio.ValidateContained(root, outPath); err != nil {
		return err
	}
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(outPath))
	if err != nil || relative == "." || relative == "" || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("manifest output must be a strict runtime descendant")
	}
	return nil
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

func parseBuildSlackManifestArgs(args []string) (string, string, string, int64, string, bool) {
	if len(args) != 10 {
		return "", "", "", 0, "", false
	}
	allowed := map[string]bool{"--runtime": true, "--receipt": true, "--out": true, "--payload-bytes": true, "--payload-sha256": true}
	values := map[string]string{}
	for index := 0; index < len(args); index += 2 {
		flag := args[index]
		if !allowed[flag] || strings.TrimSpace(args[index+1]) == "" {
			return "", "", "", 0, "", false
		}
		if values[flag] != "" {
			return "", "", "", 0, "", false
		}
		values[flag] = args[index+1]
	}
	runtimeRoot, receiptPath, outPath := values["--runtime"], values["--receipt"], values["--out"]
	payloadBytes, err := strconv.ParseInt(values["--payload-bytes"], 10, 64)
	payloadDigest := strings.ToLower(values["--payload-sha256"])
	if runtimeRoot == "" || receiptPath == "" || outPath == "" || !filepath.IsAbs(runtimeRoot) || !filepath.IsAbs(receiptPath) || !filepath.IsAbs(outPath) ||
		err != nil || payloadBytes <= 0 || payloadBytes > maximumNativeBatchSize || len(payloadDigest) != sha256.Size*2 {
		return "", "", "", 0, "", false
	}
	for _, character := range payloadDigest {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return "", "", "", 0, "", false
		}
	}
	return filepath.Clean(runtimeRoot), filepath.Clean(receiptPath), filepath.Clean(outPath), payloadBytes, payloadDigest, true
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
