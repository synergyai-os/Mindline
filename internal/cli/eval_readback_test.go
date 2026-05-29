package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalReadbackCLI(t *testing.T) {
	root := t.TempDir()
	writeCLIReadbackPressure(t, filepath.Join(root, "baseline"), 0.2, 0.8, "same")
	writeCLIReadbackPressure(t, filepath.Join(root, "current"), 0.8, 0.3, "same")
	out := filepath.Join(root, "out")

	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"eval", "readback", filepath.Join(root, "current"), "--baseline", filepath.Join(root, "baseline"), "--out", out}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ok, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schema_version": "mindline-eval-readback-summary/v0.1"`) ||
		!strings.Contains(stdout.String(), `"improvement_status": "improved"`) {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	for _, rel := range []string{"readback-summary.json", "readback-report.md", "chain-capture-draft.md", "comparison-summary.json"} {
		if _, err := os.Stat(filepath.Join(out, "eval-readback", rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	chainDraft := readCLIEvalReadbackString(t, filepath.Join(out, "eval-readback", "chain-capture-draft.md"))
	if strings.Contains(chainDraft, root) || strings.Contains(chainDraft, "/private/tmp/") {
		t.Fatalf("chain draft leaked private path: %s", chainDraft)
	}
	report := readCLIEvalReadbackString(t, filepath.Join(out, "eval-readback", "readback-report.md"))
	if !strings.Contains(report, "Metric deltas") || !strings.Contains(report, "evidence_ready_atom_ratio") {
		t.Fatalf("report should explain improvement deltas:\n%s", report)
	}
}

func TestEvalReadbackCLIUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"eval", "readback"}, &stdout, &stderr)
	if code != ExitUsage || !strings.Contains(stderr.String(), "mindline eval readback") {
		t.Fatalf("expected usage, code=%d stderr=%s", code, stderr.String())
	}
}

func TestEvalReadbackCLINoArtifacts(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"eval", "readback", root, "--out", filepath.Join(root, "out")}, &stdout, &stderr)
	if code != ExitProcess || !strings.Contains(stderr.String(), "no supported eval/trace artifacts") {
		t.Fatalf("expected process error, code=%d stderr=%s", code, stderr.String())
	}
}

func TestEvalReadbackCLIRejectsProtectedOutputRoot(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "protected")
	writeCLIReadbackPressure(t, filepath.Join(root, "current"), 0.8, 0.3, "same")
	runner := NewRunnerWithProtectedRoots(NewOSFileSystem(), []string{protected})

	var stdout, stderr bytes.Buffer
	code := runner.Run([]string{"eval", "readback", filepath.Join(root, "current"), "--out", filepath.Join(protected, "eval-out")}, &stdout, &stderr)
	if code != ExitUsage || !strings.Contains(stderr.String(), "protected output root") {
		t.Fatalf("expected protected root usage failure, code=%d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(protected, "eval-out", "eval-readback")); !os.IsNotExist(err) {
		t.Fatalf("readback output should not be created in protected root, err=%v", err)
	}
}

func TestEvalProofGateCLI(t *testing.T) {
	root := t.TempDir()
	writeCLIReadbackPressure(t, filepath.Join(root, "baseline"), 0.2, 0.8, "same")
	writeCLIReadbackPressure(t, filepath.Join(root, "current"), 0.8, 0.3, "same")
	out := filepath.Join(root, "out")

	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"eval", "proof-gate", filepath.Join(root, "current"), "--baseline", filepath.Join(root, "baseline"), "--out", out, "--claim", "improvement"}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ok, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schema_version": "mindline-eval-proof-packet/v0.1"`) ||
		!strings.Contains(stdout.String(), `"claim": "improvement"`) ||
		!strings.Contains(stdout.String(), `"verdict": "pass"`) {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	for _, rel := range []string{"proof-packet.json", "proof-report.md", "chain-capture-draft.md"} {
		if _, err := os.Stat(filepath.Join(out, "eval-proof", rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
}

func TestEvalProofGateCLIBlockedClaimReturnsProcessErrorWithPacket(t *testing.T) {
	root := t.TempDir()
	writeCLIReadbackPressure(t, filepath.Join(root, "current"), 0.8, 0.3, "same")

	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"eval", "proof-gate", filepath.Join(root, "current"), "--out", filepath.Join(root, "out"), "--claim", "improvement"}, &stdout, &stderr)
	if code != ExitProcess {
		t.Fatalf("expected process error, got %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"verdict": "blocked"`) || !strings.Contains(stdout.String(), `"missing_baseline"`) {
		t.Fatalf("expected blocked proof packet, stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func TestEvalProofGateCLIUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"eval", "proof-gate"}, &stdout, &stderr)
	if code != ExitUsage || !strings.Contains(stderr.String(), "mindline eval proof-gate") {
		t.Fatalf("expected usage, code=%d stderr=%s", code, stderr.String())
	}
}

func writeCLIReadbackPressure(t *testing.T, root string, evidenceReady, reviewBurden float64, fingerprint string) {
	t.Helper()
	target := filepath.Join(root, "corpus-pressure", "pressure-summary.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	payload := map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"corpus_id":                  "corpus-cli",
		"evidence_ready_atom_ratio":  evidenceReady,
		"review_burden_ratio":        reviewBurden,
		"corpus_fingerprint":         fingerprint,
		"command_config_fingerprint": "same-config",
		"guardrails": map[string]any{
			"network_fetches":             0,
			"hosted_telemetry_exports":    0,
			"hosted_inference_calls":      0,
			"browser_calls":               0,
			"slack_api_calls":             0,
			"destination_writes":          0,
			"product_brain_writes":        0,
			"tolaria_writes":              0,
			"auto_accepts":                0,
			"no_human_claims":             0,
			"committed_private_artifacts": 0,
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(target, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readCLIEvalReadbackString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}
