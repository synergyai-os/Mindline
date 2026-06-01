package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestDocumentsValueProofCLIWritesInspectableBundle(t *testing.T) {
	out := filepath.Join(t.TempDir(), "run")
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "value-proof",
		filepath.Join(repoRoot(t), "testdata", "documents", "value-proof", "corpus-pressure-manifest.json"),
		"--out", out,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	var summary documents.ValueProofSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if summary.SourceCount != 4 || summary.SourceAccountingRatio != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.GraphSummaryPath == "" || summary.RelationCount == 0 {
		t.Fatalf("expected graph-backed relation context: %+v", summary)
	}
	packet := mustReadCLIFile(t, filepath.Join(out, documents.ValueProofDirName, "local-value-packet.md"))
	if !strings.Contains(packet, "Local proof packet only") || !strings.Contains(packet, "Claim status") {
		t.Fatalf("packet missing review framing:\n%s", packet)
	}
}

func TestDocumentsValueProofCLIRejectsDestinationOptions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "value-proof", "testdata/documents/value-proof/corpus-pressure-manifest.json", "--destination", "tolaria", "--out", t.TempDir(),
	}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected exit %d, got %d", ExitUsage, code)
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage: mindline") {
		t.Fatalf("expected usage, got %q", stderr.String())
	}
}
