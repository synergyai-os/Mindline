package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestDocumentsLinkEnrichmentLoopCLI(t *testing.T) {
	root := t.TempDir()
	sourcePath := writeCLILinkLoopFile(t, root, "source.md", "# Link\n\nhttps://example.com/research\n")
	manifestPath := writeCLILinkLoopManifest(t, root, sourcePath)
	artifactsPath := writeCLILinkLoopArtifacts(t, root)
	out := filepath.Join(root, "out")

	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"documents", "link-enrichment-loop", manifestPath, "--artifacts", artifactsPath, "--out", out}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ok, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schema_version": "link-enrichment-loop-summary/v0.1"`) || !strings.Contains(stdout.String(), `"verdict": "improved"`) {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}
	if _, err := NewOSFileSystem().Stat(filepath.Join(out, documents.LinkEnrichmentDirName, "comparison", "comparison-report.md")); err != nil {
		t.Fatalf("missing comparison report: %v", err)
	}
}

func TestDocumentsLinkEnrichmentLoopCLIUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"documents", "link-enrichment-loop"}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage, got %d", code)
	}
	if !strings.Contains(stderr.String(), "link-enrichment-loop") {
		t.Fatalf("usage should mention command, got:\n%s", stderr.String())
	}
}

func writeCLILinkLoopFile(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := NewOSFileSystem().WriteFile(path, []byte(body)); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return path
}

func writeCLILinkLoopManifest(t *testing.T, root, sourcePath string) string {
	t.Helper()
	manifestPath := filepath.Join(root, "corpus-pressure-manifest.json")
	body := `{
  "schema_version": "corpus-pressure-manifest/v0.1",
  "corpus_id": "corpus-cli-link-loop",
  "sources": [
    {"source_id": "source-1", "source_kind": "markdown", "path": "` + filepath.Base(sourcePath) + `"}
  ]
}
`
	if err := NewOSFileSystem().WriteFile(manifestPath, []byte(body)); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return manifestPath
}

func writeCLILinkLoopArtifacts(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "artifacts.json")
	body := `{
  "schema_version": "local-source-enrichment-artifacts/v0.1",
  "artifacts": [
    {
      "url": "https://example.com/research",
      "title": "Research source",
      "description": "Requirement: review source evidence before routing.",
      "excerpt": "Requirement: local artifacts should make link captures reviewable."
    }
  ]
}
`
	if err := NewOSFileSystem().WriteFile(path, []byte(body)); err != nil {
		t.Fatalf("write artifacts: %v", err)
	}
	return path
}
