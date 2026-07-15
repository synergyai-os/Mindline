package activationcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/activationapp"
	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

func TestSystemBrowserOpenerRejectsBootstrapURLConfusion(t *testing.T) {
	for name, target := range map[string]string{
		"userinfo":       "http://attacker@127.0.0.1:43123/#bootstrap=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"host":           "http://localhost:43123/#bootstrap=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"path":           "http://127.0.0.1:43123/other#bootstrap=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"query":          "http://127.0.0.1:43123/?next=evil#bootstrap=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"extra_fragment": "http://127.0.0.1:43123/#bootstrap=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&next=evil",
		"missing_port":   "http://127.0.0.1/#bootstrap=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	} {
		t.Run(name, func(t *testing.T) {
			if err := (systemBrowserOpener{}).Open(target); err == nil {
				t.Fatalf("unsafe browser target accepted: %s", target)
			}
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
	if code := Run([]string{"gate-receipt", "--runtime", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("gate receipt failed: code=%d stderr=%s", code, stderr.String())
	}
	receipt, err := assurance.Load(root, filepath.Join(root, "pre-live-receipt.json"))
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
	code := Run([]string{"gate-receipt", "--runtime", t.TempDir()}, &stdout, &stderr)
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
	var stdout, stderr bytes.Buffer
	code := Run([]string{"gate-receipt", "--runtime", t.TempDir()}, &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "modified") {
		t.Fatalf("modified build was not rejected: code=%d stderr=%s", code, stderr.String())
	}
}

func TestGateReceiptRejectsCallerSuppliedCheckClaims(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"gate-receipt", "--runtime", t.TempDir(), "--checks", "/tmp/caller-claims.json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("caller-supplied check claims minted pre-live authority")
	}
}

func TestBuildSlackManifestBridgesNativeBatchWithoutLeakingContent(t *testing.T) {
	original := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "commit-test"}, {Key: "vcs.modified", Value: "false"}}}, true
	}
	defer func() { readBuildInfo = original }()

	root := t.TempDir()
	if err := privateio.PrepareDir(root); err != nil {
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
	receiptPath := filepath.Join(root, "pre-live-receipt.json")
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
		"build-slack-manifest", "--runtime", root, "--receipt", receiptPath, "--out", outPath,
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
	if _, _, _, _, _, ok := parseBuildSlackManifestArgs([]string{
		"--runtime", "/tmp/runtime", "--receipt", "/tmp/receipt", "--out", "relative.json",
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

func passingChecks() []assurance.Check {
	checks := make([]assurance.Check, 0, len(assurance.RequiredChecks))
	for _, name := range assurance.RequiredChecks {
		checks = append(checks, assurance.Check{Name: name, ToolVersion: "fixed-test-runner/v1", Outcome: "pass", EvidenceFingerprint: "sha256:" + name})
	}
	return checks
}
