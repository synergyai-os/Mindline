package activationcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/activationapp"
	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/controlui"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/routing"
)

func TestWP46_FixedPortCollisionPrecedesMutationAndNeverOpensBrowser(t *testing.T) {
	root := filepath.Join(t.TempDir(), "must-not-exist")
	initialized := false
	dependencies := serveDependencies{
		resolveRoot:   func() (string, error) { return root, nil },
		buildRevision: func() (string, error) { return "clean-revision", nil },
		verifyChannel: func(*os.File) (controlui.PairingConfirmer, error) { return nil, nil },
		listen: func(network, address string) (net.Listener, error) {
			if network != "tcp4" || address != fixedControlAddress {
				t.Fatalf("wrong listener contract: %s %s", network, address)
			}
			return nil, errors.New("synthetic fixed-port collision")
		},
		initialize: func(string, string, string, controlui.PairingConfirmer) (*serveRuntime, error) {
			initialized = true
			return nil, errors.New("must not initialize")
		},
		serveContext: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
		httpServer:   controlui.HTTPServer,
	}
	var stdout, stderr bytes.Buffer
	code := runServe(nil, nil, &stdout, &stderr, dependencies)
	if code == 0 || stdout.Len() != 0 || stderr.String() != "port_occupied\n" {
		t.Fatalf("collision contract failed: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if initialized {
		t.Fatal("fixed-port collision reached durable initialization")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("collision mutated stable root: %v", err)
	}
	if strings.Contains(Usage, "--open") || strings.Contains(Usage, "--receipt") && strings.Contains(Usage, "activation serve --") {
		t.Fatalf("legacy browser opener or receipt-path serve mode remains in usage: %s", Usage)
	}
}

func TestActivationServePrintsOnlyStableURLAfterTCP4Bind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bound := false
	initializedAfterBind := false
	dependencies := serveDependencies{
		resolveRoot:   func() (string, error) { return t.TempDir(), nil },
		buildRevision: func() (string, error) { return "clean-revision", nil },
		verifyChannel: func(*os.File) (controlui.PairingConfirmer, error) { return nil, nil },
		listen: func(network, address string) (net.Listener, error) {
			if network != "tcp4" || address != fixedControlAddress {
				t.Fatalf("wrong listener contract: %s %s", network, address)
			}
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			bound = err == nil
			return listener, err
		},
		initialize: func(string, string, string, controlui.PairingConfirmer) (*serveRuntime, error) {
			initializedAfterBind = bound
			return &serveRuntime{handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), close: func() {}}, nil
		},
		serveContext: func() (context.Context, context.CancelFunc) { return ctx, func() {} },
		httpServer:   controlui.HTTPServer,
	}
	var stdout, stderr bytes.Buffer
	if code := runServe(nil, nil, &stdout, &stderr, dependencies); code != 0 {
		t.Fatalf("serve failed: code=%d stderr=%q", code, stderr.String())
	}
	if !initializedAfterBind || stdout.String() != fixedControlURL+"\n" || stderr.Len() != 0 {
		t.Fatalf("stable startup contract failed: after_bind=%v stdout=%q stderr=%q", initializedAfterBind, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runServe([]string{"--open"}, nil, &stdout, &stderr, dependencies); code == 0 || stdout.Len() != 0 {
		t.Fatalf("legacy serve flag accepted: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestActivationInitializerStartsSafeShellWithoutGateReceipt(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "missing"},
		{name: "invalid", setup: func(t *testing.T, root string) {
			assuranceRoot := filepath.Join(root, "assurance")
			if err := privateio.PrepareDir(assuranceRoot); err != nil {
				t.Fatal(err)
			}
			if err := privateio.WriteFile(filepath.Join(assuranceRoot, "pre-live-receipt.json"), []byte("not a valid receipt\n"), true); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if test.setup != nil {
				test.setup(t, root)
			}
			dependencies := productionServeDependencies()
			runtime, err := dependencies.initialize(root, "clean-revision", activationapp.DefaultConfigurationFingerprint(), nil)
			if err != nil {
				t.Fatalf("%s gate must not prevent safe local shell startup: %v", test.name, err)
			}
			if runtime == nil || runtime.handler == nil || runtime.close == nil {
				t.Fatalf("%s gate did not produce a safe serving runtime", test.name)
			}
			state, err := runtime.app.State(context.Background())
			if err != nil {
				runtime.close()
				t.Fatalf("read safe shell state: %v", err)
			}
			view, ok := state.(activationapp.View)
			if !ok || view.RunSelection.State != "none" || view.RunSelection.SelectedRunID != "" {
				runtime.close()
				t.Fatalf("fresh stable root selected a legacy run: %#v", state)
			}
			runtime.close()
		})
	}
}

func TestGateReceiptUsesCleanBuildIdentityAndCompleteChecks(t *testing.T) {
	original := readBuildInfo
	originalGate := runGatePlan
	originalBinding := verifySourceBinding
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "commit-test"}, {Key: "vcs.modified", Value: "false"}}}, true
	}
	root := t.TempDir()
	useControlRoot(t, root)
	runGatePlan = func(workdir, revision, runtimeRoot string) ([]assurance.Check, error) {
		if workdir == "" || revision != "commit-test" || runtimeRoot != root {
			t.Fatalf("gate received wrong binding: workdir=%q revision=%q runtime=%q", workdir, revision, runtimeRoot)
		}
		return passingChecks(), nil
	}
	bindingCalls := 0
	verifySourceBinding = func(string, string) (string, error) {
		bindingCalls++
		return "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil
	}
	defer func() { readBuildInfo = original; runGatePlan = originalGate; verifySourceBinding = originalBinding }()
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"gate-receipt"}, &stdout, &stderr); code != 0 {
		t.Fatalf("gate receipt failed: code=%d stderr=%s", code, stderr.String())
	}
	receipt, err := assurance.Load(root, filepath.Join(root, "assurance", "pre-live-receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Commit != "commit-test" || !receipt.PrivateDataAuthorized || !receipt.RealCredentialAuthorized || !receipt.RealTransportAuthorized {
		t.Fatalf("wrong receipt: %+v", receipt)
	}
	if bindingCalls != 2 {
		t.Fatalf("expected source verification before and after checks, got %d", bindingCalls)
	}
}

func TestGateReceiptRejectsSourceBindingChangeDuringChecks(t *testing.T) {
	original := readBuildInfo
	originalGate := runGatePlan
	originalBinding := verifySourceBinding
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "commit-test"}, {Key: "vcs.modified", Value: "false"}}}, true
	}
	runGatePlan = func(string, string, string) ([]assurance.Check, error) { return passingChecks(), nil }
	useControlRoot(t, t.TempDir())
	bindingCalls := 0
	verifySourceBinding = func(string, string) (string, error) {
		bindingCalls++
		if bindingCalls == 1 {
			return "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil
		}
		return "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", nil
	}
	defer func() { readBuildInfo = original; runGatePlan = originalGate; verifySourceBinding = originalBinding }()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"gate-receipt"}, &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "source binding changed") {
		t.Fatalf("source mutation minted authority: code=%d stderr=%s", code, stderr.String())
	}
}

func TestGateReceiptRejectsModifiedBuild(t *testing.T) {
	original := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "commit-test"}, {Key: "vcs.modified", Value: "true"}}}, true
	}
	defer func() { readBuildInfo = original }()
	useControlRoot(t, t.TempDir())
	var stdout, stderr bytes.Buffer
	code := Run([]string{"gate-receipt"}, &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "modified") {
		t.Fatalf("modified build was not rejected: code=%d stderr=%s", code, stderr.String())
	}
}

func TestGateReceiptRejectsCallerSuppliedCheckClaims(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"gate-receipt", "--checks", "/tmp/caller-claims.json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("caller-supplied check claims minted pre-live authority")
	}
}

func useControlRoot(t *testing.T, root string) {
	t.Helper()
	original := resolveControlRoot
	resolveControlRoot = func() (string, error) { return root, nil }
	t.Cleanup(func() { resolveControlRoot = original })
}

func TestBuildSlackManifestBridgesNativeBatchWithoutLeakingContent(t *testing.T) {
	original := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "commit-test"}, {Key: "vcs.modified", Value: "false"}}}, true
	}
	defer func() { readBuildInfo = original }()

	root := t.TempDir()
	useControlRoot(t, root)
	if err := privateio.PrepareDir(root); err != nil {
		t.Fatal(err)
	}
	assuranceRoot := filepath.Join(root, "assurance")
	if err := privateio.PrepareDir(assuranceRoot); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	receipt, err := assurance.Build(
		"commit-test",
		activationapp.DefaultConfigurationFingerprint(),
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		now,
		passingChecks(),
	)
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(assuranceRoot, "pre-live-receipt.json")
	if err := assurance.Write(root, receiptPath, receipt); err != nil {
		t.Fatal(err)
	}
	secretSentinel := "connector-private-sentinel"
	batch := acquisitionslack.NativeBatch{
		SchemaVersion: acquisitionslack.NativeBatchSchema, WorkspaceID: "T1", ChannelID: "D1",
		LowerInclusive: "1.000001", UpperInclusive: "2.000001", Watermark: "2.000001",
		IncludeThreads: true, IncludeReplies: true, PaginationExhausted: true, ThreadPaginationExhausted: true,
		DeclaredSourceRecords: 2,
		Messages: []acquisitionslack.NativeMessage{
			{NativeMessageID: "1.000001", Timestamp: "1.000001", Text: secretSentinel + " https://example.com/post"},
			{NativeMessageID: "2.000001", Timestamp: "2.000001", Text: "duplicate https://example.com/post"},
		},
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	outPath := filepath.Join(root, "slack-manifest.json")
	var stdout, stderr bytes.Buffer
	code := RunWithInput([]string{
		"build-slack-manifest", "--out", outPath,
		"--payload-bytes", fmt.Sprint(len(payload)), "--payload-sha256", fmt.Sprintf("%x", digest[:]),
	}, bytes.NewReader(payload), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("native bridge failed: code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), secretSentinel) {
		t.Fatal("native message content leaked to command output")
	}
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != privateio.FileMode {
		t.Fatalf("private manifest mode=%v", info.Mode().Perm())
	}
	data, err := privateio.ReadFile(root, outPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest acquisitionslack.ExternalManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.SourceRecords) != 2 || len(manifest.URLOccurrences) != 2 || len(manifest.CanonicalItems) != 1 {
		t.Fatalf("occurrence denominator changed: records=%d occurrences=%d canonical=%d", len(manifest.SourceRecords), len(manifest.URLOccurrences), len(manifest.CanonicalItems))
	}
	if strings.Contains(string(data), secretSentinel) {
		t.Fatal("raw native message content was persisted in the normalized manifest")
	}
}

func TestBuildSlackManifestRejectsUnknownInputAndEscapingOutput(t *testing.T) {
	unknown := []byte(`{"schema_version":"mindline_native_slack_batch/v1","unknown":true}`)
	unknownDigest := sha256.Sum256(unknown)
	if _, err := decodeNativeSlackBatch(bytes.NewReader(unknown), int64(len(unknown)), fmt.Sprintf("%x", unknownDigest[:])); err == nil {
		t.Fatal("unknown native-batch field was accepted")
	}
	if _, _, _, ok := parseBuildSlackManifestArgs([]string{
		"--out", "relative.json",
		"--payload-bytes", "1", "--payload-sha256", strings.Repeat("a", 64),
	}); ok {
		t.Fatal("relative manifest output was accepted")
	}
	root := t.TempDir()
	parent := filepath.Dir(root)
	before, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateManifestOutputPath(root, root); err == nil {
		t.Fatal("runtime root was accepted as a manifest file")
	}
	after, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != after.Mode().Perm() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("rejected root output mutated parent metadata")
	}
	trailing := append(append([]byte{}, unknown...), []byte(`{"second":true}`)...)
	trailingDigest := sha256.Sum256(trailing)
	if _, err := decodeNativeSlackBatch(bytes.NewReader(trailing), int64(len(trailing)), fmt.Sprintf("%x", trailingDigest[:])); err == nil {
		t.Fatal("multiple objects inside one declared frame were accepted")
	}
}

func TestIngestSlackDurablyStoresCaptureAndEnrichmentBeforePublishingManifest(t *testing.T) {
	original := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "commit-test"}, {Key: "vcs.modified", Value: "false"}}}, true
	}
	defer func() { readBuildInfo = original }()
	root := t.TempDir()
	useControlRoot(t, root)
	if err := privateio.PrepareDir(root); err != nil {
		t.Fatal(err)
	}
	assuranceRoot := filepath.Join(root, "assurance")
	if err := privateio.PrepareDir(assuranceRoot); err != nil {
		t.Fatal(err)
	}
	receipt, err := assurance.Build(
		"commit-test",
		activationapp.DefaultConfigurationFingerprint(),
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		time.Now().UTC(),
		passingChecks(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := assurance.Write(root, filepath.Join(assuranceRoot, "pre-live-receipt.json"), receipt); err != nil {
		t.Fatal(err)
	}
	canonicalURL := "https://github.com/example/project"
	envelope := slackIngestEnvelope{
		SchemaVersion: slackIngestEnvelopeSchema,
		NativeBatch: acquisitionslack.NativeBatch{
			SchemaVersion: acquisitionslack.NativeBatchSchema,
			WorkspaceID:   "T1", ChannelID: "D1",
			LowerInclusive: "1.000001", UpperInclusive: "1.000001", Watermark: "1.000001",
			IncludeThreads: true, IncludeReplies: true, PaginationExhausted: true, ThreadPaginationExhausted: true,
			DeclaredSourceRecords: 1,
			Messages: []acquisitionslack.NativeMessage{{
				NativeMessageID: "1.000001", Timestamp: "1.000001",
				AuthorName: "Randy", Text: canonicalURL,
			}},
		},
		Resources: []acquisition.ImportedEvidence{{
			CanonicalItemID: routing.CanonicalURLID(canonicalURL),
			CanonicalURL:    canonicalURL,
			State:           "complete",
			RetrievedAt:     "2026-07-26T10:00:00Z",
			AccessClass:     "public",
			Metadata:        acquisition.ImportedMetadata{Title: "Agent-first memory"},
			Excerpts: []acquisition.ImportedExcerpt{{
				ExcerptID: "excerpt-1", Text: "Durable context remains available after restart.", Locator: "page",
			}},
		}},
		Contents: []personalmemory.ExtractedContent{{
			CanonicalURL: canonicalURL, MediaType: "text/plain", Completeness: "full",
			Text:        "Durable context remains available after restart. The complete extracted body is retained separately from activation.",
			AccessClass: "public",
		}},
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	outPath := filepath.Join(root, "slack-manifest.json")
	var stdout, stderr bytes.Buffer
	code := RunWithInput([]string{
		"ingest-slack", "--out", outPath,
		"--payload-bytes", fmt.Sprint(len(payload)), "--payload-sha256", fmt.Sprintf("%x", digest[:]),
	}, bytes.NewReader(payload), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Slack ingest failed: code=%d stderr=%s", code, stderr.String())
	}
	repository, err := personalmemory.NewFileRepository(filepath.Join(root, "personal-memory"), nil)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := personalmemory.NewLexicalRetriever(repository).Search(personalmemory.SearchRequest{
		Query: "durable context restart", Limit: 3,
	})
	if err != nil || len(packet.Records) != 1 || len(packet.Resources) != 1 {
		t.Fatalf("durable enriched recall failed: packet=%+v err=%v", packet, err)
	}
	replayStdout := bytes.Buffer{}
	replayStderr := bytes.Buffer{}
	if code := RunWithInput([]string{
		"ingest-slack", "--out", outPath,
		"--payload-bytes", fmt.Sprint(len(payload)), "--payload-sha256", fmt.Sprintf("%x", digest[:]),
	}, bytes.NewReader(payload), &replayStdout, &replayStderr); code != 0 {
		t.Fatalf("Slack ingest replay failed: code=%d stderr=%s", code, replayStderr.String())
	}
	var summary manifestBuildSummary
	if err := json.Unmarshal(replayStdout.Bytes(), &summary); err != nil ||
		summary.Memory == nil || summary.Memory.UnchangedRecords != 1 ||
		summary.Enrichment == nil || summary.Enrichment.UnchangedResources != 1 ||
		!summary.ManifestReused || summary.ManifestCreated {
		t.Fatalf("Slack ingest replay duplicated state: %+v err=%v", summary, err)
	}
}

func TestIngestSlackRetainsPersonalEvidenceWhenActivationGateIsUnavailable(t *testing.T) {
	original := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "commit-test"}, {Key: "vcs.modified", Value: "false"}}}, true
	}
	defer func() { readBuildInfo = original }()
	root := t.TempDir()
	useControlRoot(t, root)
	envelope := slackIngestEnvelope{
		SchemaVersion: slackIngestEnvelopeSchema,
		NativeBatch: acquisitionslack.NativeBatch{
			SchemaVersion: acquisitionslack.NativeBatchSchema,
			WorkspaceID:   "T-retain", ChannelID: "D-retain",
			LowerInclusive: "1.000001", UpperInclusive: "1.000001", Watermark: "1.000001",
			IncludeThreads: true, IncludeReplies: true, PaginationExhausted: true, ThreadPaginationExhausted: true,
			DeclaredSourceRecords: 1,
			Messages: []acquisitionslack.NativeMessage{{
				NativeMessageID: "1.000001", Timestamp: "1.000001", Text: "https://example.com/retained",
			}},
		},
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	var stdout, stderr bytes.Buffer
	code := RunWithInput([]string{
		"ingest-slack", "--out", filepath.Join(root, "manifest.json"),
		"--payload-bytes", fmt.Sprint(len(payload)), "--payload-sha256", fmt.Sprintf("%x", digest[:]),
	}, bytes.NewReader(payload), &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "personal evidence retained") {
		t.Fatalf("missing downstream-only failure: code=%d stderr=%s", code, stderr.String())
	}
	repository, err := personalmemory.NewFileRepository(filepath.Join(root, "personal-memory"), nil)
	if err != nil {
		t.Fatal(err)
	}
	status, err := repository.Status()
	if err != nil || status.RecordCount != 1 {
		t.Fatalf("stale activation gate blocked personal retention: %+v err=%v", status, err)
	}
}

func passingChecks() []assurance.Check {
	checks := make([]assurance.Check, 0, len(assurance.RequiredChecks))
	for _, name := range assurance.RequiredChecks {
		checks = append(checks, assurance.Check{Name: name, ToolVersion: "fixed-test-runner/v1", Outcome: "pass", EvidenceFingerprint: "sha256:" + name})
	}
	return checks
}
