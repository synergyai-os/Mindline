package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
)

func TestDocumentsEnrichSourcesCLIWritesPressureCompatibleBundle(t *testing.T) {
	root := t.TempDir()
	sourceRel := filepath.ToSlash(filepath.Join("fixtures", "source-1.md"))
	sourcePath := filepath.Join(root, filepath.FromSlash(sourceRel))
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source fixture: %v", err)
	}
	writeCLITestFile(t, sourcePath, []byte("# Link\n\nhttps://example.com/research\n"))
	manifestPath := filepath.Join(root, "corpus-pressure-manifest.json")
	writeCLITestJSON(t, manifestPath, documents.CorpusPressureManifest{
		SchemaVersion: documents.CorpusPressureManifestSchemaVersion,
		CorpusID:      "corpus-cli-enrich",
		Sources: []documents.CorpusPressureManifestSource{{
			SourceID:   "source-1",
			SourceKind: documents.SourceKindMarkdown,
			Path:       sourceRel,
		}},
	})
	artifactsPath := filepath.Join(root, "artifacts.json")
	writeCLITestJSON(t, artifactsPath, documents.LocalSourceEnrichmentArtifactManifest{
		SchemaVersion: documents.LocalSourceEnrichmentArtifactsSchemaVersion,
		Artifacts: []documents.LocalSourceEnrichmentArtifact{{
			URL:     "https://example.com/research",
			Title:   "Enriched source",
			Excerpt: "Requirement: Review the enriched source before routing.",
		}},
	})

	out := filepath.Join(root, "enriched")
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"documents", "enrich-sources", manifestPath, "--artifacts", artifactsPath, "--out", out}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	var summary documents.SourceEnrichmentSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if summary.EnrichedURLCount != 1 || summary.URLAccountingCoverage != 1 {
		t.Fatalf("bad summary: %+v", summary)
	}
	source := mustReadCLIFile(t, filepath.Join(out, "sources", "source-1", "source.md"))
	if !strings.Contains(source, "## Enriched Sources") || !strings.Contains(source, "Retrieval mode: `local_artifact`") {
		t.Fatalf("missing enriched source section:\n%s", source)
	}
}

func TestDocumentsEnrichSourcesCLIRequiresArtifactsAndOut(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"documents", "enrich-sources", "manifest.json", "--out", "out"}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d", code)
	}
	if stdout.String() != "" || !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("expected usage output, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
