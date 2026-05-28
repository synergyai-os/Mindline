package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
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
	if _, err := NewOSFileSystem().Stat(filepath.Join(out, documents.LinkEnrichmentDirName, "posthog", "eval-projection.json")); err != nil {
		t.Fatalf("missing PostHog eval projection: %v", err)
	}
}

func TestDocumentsLinkEnrichmentLoopCLIExportsPostHogEvaluation(t *testing.T) {
	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "true")
	t.Setenv("MINDLINE_LLM_TRACE_MODE", "metadata")
	t.Setenv("MINDLINE_TELEMETRY_SALT", "salt")
	t.Setenv("POSTHOG_PROJECT_API_KEY", "phc-test")
	t.Setenv("POSTHOG_HOST", "https://eu.i.posthog.com")
	root := t.TempDir()
	sourcePath := writeCLILinkLoopFile(t, root, "source.md", "# Link\n\nhttps://example.com/research\n")
	manifestPath := writeCLILinkLoopManifest(t, root, sourcePath)
	artifactsPath := writeCLILinkLoopArtifacts(t, root)
	out := filepath.Join(root, "out")
	var capturedBody string
	runner := NewRunnerWithPostHogTransport(NewOSFileSystem(), httpRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://eu.i.posthog.com/capture/" {
			t.Fatalf("unexpected PostHog URL: %s", req.URL.String())
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		capturedBody = string(body)
		if containsUnsafePostHogBody(capturedBody) {
			t.Fatalf("PostHog body contains unsafe content: %s", capturedBody)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	}))

	var stdout, stderr bytes.Buffer
	code := runner.Run([]string{"documents", "link-enrichment-loop", manifestPath, "--artifacts", artifactsPath, "--out", out}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ok, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if capturedBody == "" {
		t.Fatalf("expected PostHog capture request")
	}
	var payload struct {
		Event      string         `json:"event"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal([]byte(capturedBody), &payload); err != nil {
		t.Fatalf("decode PostHog payload: %v\n%s", err, capturedBody)
	}
	if payload.Event != "$ai_evaluation" ||
		payload.Properties["$ai_evaluation_name"] != "mindline.link_enrichment.covered_missingness_reduction" ||
		payload.Properties["$ai_evaluation_result"] != true ||
		payload.Properties["metadata_only"] != true {
		t.Fatalf("unexpected PostHog evaluation payload: %+v", payload)
	}
	projection := readCLILinkLoopString(t, filepath.Join(out, documents.LinkEnrichmentDirName, "posthog", "eval-projection.json"))
	if !strings.Contains(projection, `"status": "sent"`) || !strings.Contains(projection, `"$ai_evaluation"`) {
		t.Fatalf("expected sent PostHog eval projection:\n%s", projection)
	}
}

func TestDocumentsLinkEnrichmentLoopCLIWritesFailedEvaluationWhenKRsMiss(t *testing.T) {
	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "false")
	root := t.TempDir()
	sourcePath := writeCLILinkLoopFile(t, root, "source.md", "# Links\n\nhttps://example.com/research\n\nhttps://example.com/missing\n")
	manifestPath := writeCLILinkLoopManifest(t, root, sourcePath)
	artifactsPath := writeCLILinkLoopArtifactsWithExtraStale(t, root)
	out := filepath.Join(root, "out")

	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"documents", "link-enrichment-loop", manifestPath, "--artifacts", artifactsPath, "--out", out}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected ok, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	projection := readCLILinkLoopString(t, filepath.Join(out, documents.LinkEnrichmentDirName, "posthog", "eval-projection.json"))
	if !strings.Contains(projection, `"$ai_evaluation_result": false`) ||
		!strings.Contains(projection, "incomplete_artifact_match") ||
		!strings.Contains(projection, `"artifact_match_coverage": 0.5`) {
		t.Fatalf("expected failed KR evaluation projection:\n%s", projection)
	}
}

func TestDocumentsLinkEnrichmentLoopCLIBlocksRawTraceModeBeforeExport(t *testing.T) {
	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "true")
	t.Setenv("MINDLINE_LLM_TRACE_MODE", "raw")
	t.Setenv("MINDLINE_TELEMETRY_SALT", "salt")
	t.Setenv("POSTHOG_PROJECT_API_KEY", "phc-test")
	t.Setenv("POSTHOG_HOST", "https://eu.i.posthog.com")
	root := t.TempDir()
	sourcePath := writeCLILinkLoopFile(t, root, "source.md", "# Link\n\nhttps://example.com/research\n")
	manifestPath := writeCLILinkLoopManifest(t, root, sourcePath)
	artifactsPath := writeCLILinkLoopArtifacts(t, root)
	out := filepath.Join(root, "out")

	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"documents", "link-enrichment-loop", manifestPath, "--artifacts", artifactsPath, "--out", out}, &stdout, &stderr)
	if code != ExitArtifactWrite {
		t.Fatalf("expected artifact-write failure, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	projection := readCLILinkLoopString(t, filepath.Join(out, documents.LinkEnrichmentDirName, "posthog", "eval-projection.json"))
	if !strings.Contains(projection, `"status": "blocked"`) ||
		!strings.Contains(projection, `"error_class": "config_error"`) ||
		!strings.Contains(stderr.String(), "unsupported LLM trace mode: raw") {
		t.Fatalf("expected blocked raw trace projection, stderr=%s projection=\n%s", stderr.String(), projection)
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

func readCLILinkLoopString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
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

func writeCLILinkLoopArtifactsWithExtraStale(t *testing.T, root string) string {
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
    },
    {
      "url": "https://example.com/stale",
      "title": "Stale source",
      "description": "This artifact should be counted as stale.",
      "excerpt": "Stale artifacts must fail the evaluation coverage KR."
    }
  ]
}
`
	if err := NewOSFileSystem().WriteFile(path, []byte(body)); err != nil {
		t.Fatalf("write artifacts: %v", err)
	}
	return path
}
