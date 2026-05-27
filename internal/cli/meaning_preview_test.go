package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestDocumentsMeaningPreviewCLIWritesPreviewBundle(t *testing.T) {
	root := t.TempDir()
	pressureOut := filepath.Join(root, "pressure")
	if _, _, err := documents.BuildCorpusPressure(filepath.Join(repoRoot(t), "testdata", "documents", "markdown"), pressureOut, documents.CorpusPressureOptions{}); err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}

	previewOut := filepath.Join(root, "meaning")
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"documents", "meaning-preview", pressureOut, "--out", previewOut}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	var summary documents.SourceMeaningPreviewSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if summary.SchemaVersion != documents.SourceMeaningPreviewSchemaVersion {
		t.Fatalf("unexpected schema version %s", summary.SchemaVersion)
	}
	if summary.PreviewedSourceCount != 3 || summary.PreviewCoverageRatio != 1 {
		t.Fatalf("unexpected preview coverage: %+v", summary)
	}
	if summary.Guardrails.DestinationWrites != 0 || summary.Guardrails.ProductBrainWrites != 0 || summary.Guardrails.TolariaWrites != 0 {
		t.Fatalf("expected zero write guardrails: %+v", summary.Guardrails)
	}
	report := mustReadCLIFile(t, filepath.Join(previewOut, documents.SourceMeaningPreviewDirName, "meaning-report.md"))
	if !strings.Contains(report, "Preview/calibration only") || !strings.Contains(report, "Product Brain writes: 0") {
		t.Fatalf("report missing review guardrails:\n%s", report)
	}
}

func TestDocumentsMeaningPreviewCLIRejectsProtectedRoot(t *testing.T) {
	root := t.TempDir()
	pressureOut := filepath.Join(root, "pressure")
	if _, _, err := documents.BuildCorpusPressure(filepath.Join(repoRoot(t), "testdata", "documents", "markdown"), pressureOut, documents.CorpusPressureOptions{}); err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	protected := filepath.Join(root, "vault")
	out := filepath.Join(protected, "meaning")

	var stdout, stderr bytes.Buffer
	code := NewRunnerWithProtectedRoots(NewOSFileSystem(), []string{protected}).Run([]string{"documents", "meaning-preview", pressureOut, "--out", out}, &stdout, &stderr)
	if code != ExitArtifactWrite {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitArtifactWrite, code, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "protected output root") {
		t.Fatalf("expected protected-root stderr, got %q", stderr.String())
	}
}

func mustReadCLIFile(t *testing.T, path string) string {
	t.Helper()
	data, err := NewOSFileSystem().ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
