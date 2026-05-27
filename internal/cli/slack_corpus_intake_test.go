package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	slackadapter "github.com/synergyai-os/Mindline/internal/adapters/slack"
	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestSlackCorpusIntakeCLIWritesPressureCompatibleManifest(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "slack.json")
	out := filepath.Join(root, "intake")
	writeCLITestFile(t, input, []byte(`{
  "source": {"workspace": "synthetic", "channel_id": "DTEST", "channel_name": "self-dm", "adapter_id": "slack"},
  "messages": [
    {"ts": "1710000001.000001", "user": "U123", "author_name": "Randy", "text": "Save https://example.com/research"},
    {"ts": "1710000000.000001", "user": "U123", "author_name": "Randy", "text": ""}
  ]
}`))

	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"slack", "corpus-intake", input, "--out", out}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d got %d stderr=%s", ExitOK, code, stderr.String())
	}
	var summary slackadapter.CorpusIntakeSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v\n%s", err, stdout.String())
	}
	if summary.ProcessedCount != 1 || summary.SkippedCount != 1 || summary.ManifestPath != "corpus-pressure-manifest.json" {
		t.Fatalf("bad summary: %#v", summary)
	}

	pressureOut := filepath.Join(root, "pressure")
	var pressureStdout, pressureStderr bytes.Buffer
	code = NewRunner(NewOSFileSystem()).Run([]string{"documents", "corpus-pressure", filepath.Join(out, "corpus-pressure-manifest.json"), "--out", pressureOut}, &pressureStdout, &pressureStderr)
	if code != ExitOK {
		t.Fatalf("expected corpus-pressure exit %d got %d stderr=%s", ExitOK, code, pressureStderr.String())
	}
	var pressure documents.CorpusPressureSummary
	if err := json.Unmarshal(pressureStdout.Bytes(), &pressure); err != nil {
		t.Fatalf("decode pressure summary: %v\n%s", err, pressureStdout.String())
	}
	if pressure.SourceCount != 1 || pressure.Guardrails.DestinationWrites != 0 {
		t.Fatalf("bad pressure summary: %#v", pressure)
	}
}

func TestSlackCorpusIntakeCLIUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"slack", "corpus-intake", "input.json"}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage, got %d", code)
	}
}

func TestSlackCorpusIntakeRejectsProtectedDestinationRoot(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("slack.json", []byte(`{
  "source": {"workspace": "synthetic", "channel_id": "DTEST", "channel_name": "self-dm", "adapter_id": "slack"},
  "messages": [
    {"ts": "1710000000.000001", "user": "U123", "author_name": "Randy", "text": "Save https://example.com/research"}
  ]
}`))
	fs.MkdirAll("tolaria", 0o755)

	var stdout, stderr bytes.Buffer
	code := NewRunnerWithProtectedRoots(fs, []string{"tolaria"}).Run([]string{"slack", "corpus-intake", "slack.json", "--out", "tolaria"}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "protected output root") {
		t.Fatalf("expected protected root error, got %q", stderr.String())
	}
	if fs.Exists(cleanPath("tolaria/slack-corpus-intake")) || fs.Exists(cleanPath("tolaria/corpus-pressure-manifest.json")) {
		t.Fatalf("protected destination root was written")
	}
}

func writeCLITestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
