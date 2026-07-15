package activationcli

import (
	"bytes"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/assurance"
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

func passingChecks() []assurance.Check {
	checks := make([]assurance.Check, 0, len(assurance.RequiredChecks))
	for _, name := range assurance.RequiredChecks {
		checks = append(checks, assurance.Check{Name: name, ToolVersion: "fixed-test-runner/v1", Outcome: "pass", EvidenceFingerprint: "sha256:" + name})
	}
	return checks
}
