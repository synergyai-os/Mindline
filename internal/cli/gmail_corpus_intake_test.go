package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gmailadapter "github.com/synergyai-os/Mindline/internal/adapters/gmail"
	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestGmailCorpusIntakeCLIWritesPressureCompatibleManifest(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "gmail.json")
	out := filepath.Join(root, "intake")
	writeCLITestFile(t, input, []byte(`{
  "source": {"account": "private-account", "mailbox": "all-mail", "adapter_id": "gmail"},
  "responses": [
    {"id": "m1", "from_": "sender@example.com", "subject": "Research", "body": "Save https://example.com/research", "email_ts": "2026-06-04T07:00:00Z"},
    {"id": "m2", "from_": "sender@example.com", "subject": "", "body": "", "email_ts": "2026-06-04T08:00:00Z"}
  ]
}`))

	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"gmail", "corpus-intake", input, "--out", out}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d got %d stderr=%s", ExitOK, code, stderr.String())
	}
	var summary gmailadapter.CorpusIntakeSummary
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

func TestGmailCorpusIntakeCLIUsesRunnerFileSystemForWrites(t *testing.T) {
	realCWD := t.TempDir()
	t.Chdir(realCWD)
	fs := NewMemoryFS()
	if err := fs.WriteFile("gmail.json", []byte(`{
  "source": {"account": "private-account", "mailbox": "all-mail", "adapter_id": "gmail"},
  "messages": [
    {"id": "m1", "from_": "sender@example.com", "subject": "Research", "body": "Save https://example.com/research", "email_ts": "2026-06-04T07:00:00Z"}
  ]
}`)); err != nil {
		t.Fatalf("write memory input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"gmail", "corpus-intake", "gmail.json", "--out", "intake"}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if !fs.Exists(cleanPath("intake/corpus-pressure-manifest.json")) {
		t.Fatalf("expected manifest written through runner filesystem; paths=%v", fs.Paths())
	}
	if !fs.Exists(cleanPath("intake/gmail-corpus-intake/intake-summary.json")) {
		t.Fatalf("expected summary written through runner filesystem; paths=%v", fs.Paths())
	}
	if _, err := os.Stat(filepath.Join(realCWD, "intake", "corpus-pressure-manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("corpus intake wrote through real filesystem, stat err=%v", err)
	}
}

func TestGmailCorpusIntakeRejectsProtectedDestinationRoot(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("gmail.json", []byte(`{
  "source": {"account": "private-account", "mailbox": "all-mail", "adapter_id": "gmail"},
  "messages": [
    {"id": "m1", "from_": "sender@example.com", "subject": "Research", "body": "Save https://example.com/research", "email_ts": "2026-06-04T07:00:00Z"}
  ]
}`))
	fs.MkdirAll("tolaria", 0o755)

	var stdout, stderr bytes.Buffer
	code := NewRunnerWithProtectedRoots(fs, []string{"tolaria"}).Run([]string{"gmail", "corpus-intake", "gmail.json", "--out", "tolaria"}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "protected output root") {
		t.Fatalf("expected protected root error, got %q", stderr.String())
	}
	if fs.Exists(cleanPath("tolaria/gmail-corpus-intake")) || fs.Exists(cleanPath("tolaria/corpus-pressure-manifest.json")) {
		t.Fatalf("protected destination root was written")
	}
}
