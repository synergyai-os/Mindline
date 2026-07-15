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
	"net/http"
	"os"
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
	"github.com/synergyai-os/Mindline/internal/operatorchannel"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const Usage = "usage: mindline activation config-fingerprint\nusage: mindline activation gate-receipt\nusage: mindline activation build-slack-manifest --out <manifest.json> --payload-bytes <n> --payload-sha256 <hex>\nusage: mindline activation serve\n"

const (
	maximumNativeBatchSize = int64(64 << 20)
	fixedControlAddress    = "127.0.0.1:9876"
	fixedControlURL        = "http://127.0.0.1:9876/"
)

var readBuildInfo = debug.ReadBuildInfo
var runGatePlan = assurance.RunFixedGate
var verifySourceBinding = assurance.VerifySourceBinding
var resolveControlRoot = privateio.DefaultControlPlaneRoot

func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithInputs(args, strings.NewReader(""), os.Stdin, stdout, stderr)
}

func RunWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return RunWithInputs(args, stdin, nil, stdout, stderr)
}

// RunWithInputs keeps the generic native-source stream separate from the
// launcher-owned operator channel. Only the latter may authorize browser
// pairing for the serving command.
func RunWithInputs(args []string, nativeInput io.Reader, operatorInput *os.File, stdout, stderr io.Writer) int {
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
		return runBuildSlackManifest(args[1:], nativeInput, stdout, stderr)
	case "serve":
		return runServe(args[1:], operatorInput, stdout, stderr, productionServeDependencies())
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
	outPath, payloadBytes, payloadDigest, ok := parseBuildSlackManifestArgs(args)
	if !ok {
		fmt.Fprint(stderr, Usage)
		return 1
	}
	runtimeRoot, err := resolveControlRoot()
	if err != nil || !filepath.IsAbs(runtimeRoot) {
		fmt.Fprintln(stderr, "Slack manifest blocked: stable control root unavailable")
		return 2
	}
	receiptPath := filepath.Join(runtimeRoot, "assurance", "pre-live-receipt.json")
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
	if err := assurance.Validate(receipt, revision, configuration); err != nil {
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
	manifest, err := acquisitionslack.BuildAuthorizedExternalManifestFromNativeBatch(batch, receipt, revision, configuration)
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
	if len(args) != 0 {
		fmt.Fprint(stderr, Usage)
		return 1
	}
	runtimeRoot, err := resolveControlRoot()
	if err != nil || !filepath.IsAbs(runtimeRoot) {
		fmt.Fprintln(stderr, "pre-live receipt blocked: stable control root unavailable")
		return 2
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
	assuranceRoot := filepath.Join(runtimeRoot, "assurance")
	if err := privateio.PrepareDir(assuranceRoot); err != nil {
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
	path := filepath.Join(assuranceRoot, "pre-live-receipt.json")
	if err := assurance.Write(runtimeRoot, path, receipt); err != nil {
		fmt.Fprintf(stderr, "pre-live receipt blocked: %v\n", err)
		return 2
	}
	_ = json.NewEncoder(stdout).Encode(map[string]any{"receipt_path": path, "receipt_fingerprint": receipt.Fingerprint, "commit": revision})
	return 0
}

type serveRuntime struct {
	handler http.Handler
	close   func()
	app     *activationapp.App
}

type serveDependencies struct {
	resolveRoot   func() (string, error)
	buildRevision func() (string, error)
	verifyChannel func(*os.File) (controlui.PairingConfirmer, error)
	listen        func(string, string) (net.Listener, error)
	initialize    func(string, string, string, controlui.PairingConfirmer) (*serveRuntime, error)
	serveContext  func() (context.Context, context.CancelFunc)
	httpServer    func(net.Listener, http.Handler) *http.Server
}

func productionServeDependencies() serveDependencies {
	return serveDependencies{
		resolveRoot:   resolveControlRoot,
		buildRevision: cleanBuildRevision,
		verifyChannel: func(input *os.File) (controlui.PairingConfirmer, error) {
			verified, err := (operatorchannel.AnonymousPipeVerifier{}).Verify(input)
			if err != nil {
				return nil, err
			}
			return controlui.NewOperatorPairingConfirmer(verified), nil
		},
		listen: net.Listen,
		initialize: func(runtimeRoot, revision, configuration string, pairing controlui.PairingConfirmer) (*serveRuntime, error) {
			receiptPath := filepath.Join(runtimeRoot, "assurance", "pre-live-receipt.json")
			var receipt *assurance.Receipt
			loadedReceipt, loadErr := assurance.Load(runtimeRoot, receiptPath)
			if loadErr == nil && assurance.Validate(loadedReceipt, revision, configuration) == nil {
				receipt = &loadedReceipt
			}
			app, err := activationapp.New(activationapp.Options{
				ControlRoot: runtimeRoot, Commit: revision, ConfigurationFingerprint: configuration,
				PreLiveReceipt: receipt,
			})
			if err != nil {
				return nil, err
			}
			server, err := controlui.New(app, controlui.Options{
				ExpectedHost: fixedControlAddress,
				Origin:       "http://" + fixedControlAddress,
				RuntimeRoot:  runtimeRoot,
				Pairing:      pairing,
			})
			if err != nil {
				app.Close()
				return nil, err
			}
			return &serveRuntime{handler: server.Handler(), close: app.Close, app: app}, nil
		},
		serveContext: func() (context.Context, context.CancelFunc) {
			return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		},
		httpServer: controlui.HTTPServer,
	}
}

func runServe(args []string, operatorInput *os.File, stdout, stderr io.Writer, dependencies serveDependencies) int {
	if len(args) != 0 {
		fmt.Fprint(stderr, Usage)
		return 1
	}
	if dependencies.resolveRoot == nil || dependencies.buildRevision == nil || dependencies.verifyChannel == nil || dependencies.listen == nil || dependencies.initialize == nil || dependencies.serveContext == nil || dependencies.httpServer == nil {
		fmt.Fprintln(stderr, "activation_unavailable")
		return 2
	}

	// Everything before Listen is read-only and side-effect free. In
	// particular, resolving the default root does not create it.
	pairing, err := dependencies.verifyChannel(operatorInput)
	if err != nil {
		fmt.Fprintln(stderr, "operator_channel_unavailable")
		return 2
	}
	runtimeRoot, err := dependencies.resolveRoot()
	if err != nil || !filepath.IsAbs(runtimeRoot) {
		fmt.Fprintln(stderr, "activation_unavailable")
		return 2
	}
	revision, err := dependencies.buildRevision()
	if err != nil {
		fmt.Fprintln(stderr, "activation_unavailable")
		return 2
	}
	configuration := activationapp.DefaultConfigurationFingerprint()

	listener, err := dependencies.listen("tcp4", fixedControlAddress)
	if err != nil {
		fmt.Fprintln(stderr, "port_occupied")
		return 2
	}
	defer listener.Close()

	// Durable initialization is intentionally downstream of the successful
	// fixed-port bind so a collision cannot migrate, recover, select, or write.
	runtime, err := dependencies.initialize(filepath.Clean(runtimeRoot), revision, configuration, pairing)
	if err != nil || runtime == nil || runtime.handler == nil || runtime.close == nil {
		fmt.Fprintln(stderr, "activation_unavailable")
		return 2
	}
	defer runtime.close()

	ctx, stop := dependencies.serveContext()
	defer stop()
	httpServer := dependencies.httpServer(listener, runtime.handler)
	if httpServer == nil {
		fmt.Fprintln(stderr, "activation_unavailable")
		return 2
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- httpServer.Serve(listener)
	}()

	// This is the only successful startup output. It is stable, contains no
	// fragment or capability, and never triggers a browser process.
	fmt.Fprint(stdout, fixedControlURL+"\n")

	code := 0
	select {
	case <-ctx.Done():
	case serveErr := <-serveErrors:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintln(stderr, "activation_unavailable")
			code = 2
		}
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil && code == 0 {
		fmt.Fprintln(stderr, "activation_unavailable")
		code = 2
	}
	return code
}

func parseBuildSlackManifestArgs(args []string) (string, int64, string, bool) {
	if len(args) != 6 {
		return "", 0, "", false
	}
	allowed := map[string]bool{"--out": true, "--payload-bytes": true, "--payload-sha256": true}
	values := map[string]string{}
	for index := 0; index < len(args); index += 2 {
		flag := args[index]
		if !allowed[flag] || strings.TrimSpace(args[index+1]) == "" {
			return "", 0, "", false
		}
		if values[flag] != "" {
			return "", 0, "", false
		}
		values[flag] = args[index+1]
	}
	outPath := values["--out"]
	payloadBytes, err := strconv.ParseInt(values["--payload-bytes"], 10, 64)
	payloadDigest := strings.ToLower(values["--payload-sha256"])
	if outPath == "" || !filepath.IsAbs(outPath) ||
		err != nil || payloadBytes <= 0 || payloadBytes > maximumNativeBatchSize || len(payloadDigest) != sha256.Size*2 {
		return "", 0, "", false
	}
	for _, character := range payloadDigest {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return "", 0, "", false
		}
	}
	return filepath.Clean(outPath), payloadBytes, payloadDigest, true
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
