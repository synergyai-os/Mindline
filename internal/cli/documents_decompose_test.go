package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDocumentsDecompose(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "decompose", documentsFixture(t, "markdown"),
		"--out", out,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	var summary struct {
		SchemaVersion string `json:"schema_version"`
		SegmentCount  int    `json:"segment_count"`
		Segments      []struct {
			SegmentPath string `json:"segment_path"`
			PreviewPath string `json:"preview_path"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if summary.SchemaVersion != "document-segment-summary/v0.1" {
		t.Fatalf("unexpected schema: %s", summary.SchemaVersion)
	}
	if summary.SegmentCount == 0 || len(summary.Segments) != summary.SegmentCount {
		t.Fatalf("unexpected segment count: %+v", summary)
	}
	if _, err := os.Stat(filepath.Join(out, "document-segments", "segment-summary.json")); err != nil {
		t.Fatalf("expected summary artifact: %v", err)
	}
	for _, item := range summary.Segments {
		if _, err := os.Stat(filepath.Join(out, "document-segments", item.SegmentPath)); err != nil {
			t.Fatalf("expected segment artifact %s: %v", item.SegmentPath, err)
		}
		if _, err := os.Stat(filepath.Join(out, "document-segments", item.PreviewPath)); err != nil {
			t.Fatalf("expected preview artifact %s: %v", item.PreviewPath, err)
		}
	}
}

func TestDocumentsStructure(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "structure", documentsFixture(t, "structure"),
		"--out", out,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	var summary struct {
		SchemaVersion string `json:"schema_version"`
		NodeCount     int    `json:"node_count"`
		Nodes         []struct {
			NodePath    string `json:"node_path"`
			PreviewPath string `json:"preview_path"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if summary.SchemaVersion != "document-structure-summary/v0.1" {
		t.Fatalf("unexpected schema: %s", summary.SchemaVersion)
	}
	if summary.NodeCount == 0 || len(summary.Nodes) != summary.NodeCount {
		t.Fatalf("unexpected node count: %+v", summary)
	}
	if _, err := os.Stat(filepath.Join(out, "document-structure", "structure-summary.json")); err != nil {
		t.Fatalf("expected summary artifact: %v", err)
	}
	for _, item := range summary.Nodes {
		if item.NodePath == "" {
			t.Fatalf("expected node path in summary item: %+v", item)
		}
		if _, err := os.Stat(filepath.Join(out, "document-structure", item.PreviewPath)); err != nil {
			t.Fatalf("expected preview artifact %s: %v", item.PreviewPath, err)
		}
	}
}

func TestDocumentsStructureDoesNotReadProductBrainProfile(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "structure", documentsFixture(t, "structure", "mixed-structure.md"),
		"--profile", documentsFixture(t, "..", "productbrain", "profiles", "default-governance.json"),
		"--out", out,
	}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage exit for --profile, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage: mindline documents structure") {
		t.Fatalf("expected documents structure usage, got %q", stderr.String())
	}
}

func TestDocumentsSemantics(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", out,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	var summary struct {
		SchemaVersion    string `json:"schema_version"`
		ObservationCount int    `json:"observation_count"`
		CandidateCount   int    `json:"candidate_count"`
		RelationCount    int    `json:"relation_count"`
		Candidates       []struct {
			CandidatePath string `json:"candidate_path"`
			PreviewPath   string `json:"preview_path"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if summary.SchemaVersion != "semantic-candidate-summary/v0.1" {
		t.Fatalf("unexpected schema: %s", summary.SchemaVersion)
	}
	if summary.ObservationCount == 0 || summary.CandidateCount == 0 || summary.RelationCount == 0 {
		t.Fatalf("unexpected semantic counts: %+v", summary)
	}
	if _, err := os.Stat(filepath.Join(out, "document-structure", "structure-summary.json")); err != nil {
		t.Fatalf("expected document structure beside semantic output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "semantic-candidates", "semantic-summary.json")); err != nil {
		t.Fatalf("expected semantic summary artifact: %v", err)
	}
	for _, item := range summary.Candidates {
		if _, err := os.Stat(filepath.Join(out, "semantic-candidates", item.CandidatePath)); err != nil {
			t.Fatalf("expected candidate artifact %s: %v", item.CandidatePath, err)
		}
		if _, err := os.Stat(filepath.Join(out, "semantic-candidates", item.PreviewPath)); err != nil {
			t.Fatalf("expected candidate preview %s: %v", item.PreviewPath, err)
		}
	}
}

func TestDocumentsSemanticsRejectsDestinationAndProfileFlags(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--profile", documentsFixture(t, "..", "productbrain", "profiles", "default-governance.json"),
		"--out", out,
	}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage exit for --profile, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage: mindline documents semantics") {
		t.Fatalf("expected documents semantics usage, got %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--destination", "tolaria",
		"--out", out,
	}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage exit for --destination, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestDocumentsStructureReportsWriteFailuresAsArtifactWrite(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(outFile, []byte("occupied"), 0o644); err != nil {
		t.Fatalf("write out file: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "structure", documentsFixture(t, "structure", "mixed-structure.md"),
		"--out", outFile,
	}, &stdout, &stderr)
	if code != ExitArtifactWrite {
		t.Fatalf("expected artifact write exit, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "write document structure") {
		t.Fatalf("expected write error context, got %q", stderr.String())
	}
}

func TestDocumentsDecomposeDoesNotReadProductBrainProfile(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "decompose", documentsFixture(t, "markdown", "transcript-decision-action.md"),
		"--profile", documentsFixture(t, "..", "productbrain", "profiles", "default-governance.json"),
		"--out", out,
	}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage exit for --profile, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage: mindline documents decompose") {
		t.Fatalf("expected documents usage, got %q", stderr.String())
	}
}

func TestDocumentsDecomposeDoesNotEmitProductBrainProposals(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "decompose", documentsFixture(t, "markdown", "mixed-thread-capture.md"),
		"--out", out,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "productbrain-proposals")); !os.IsNotExist(err) {
		t.Fatalf("documents command must not emit productbrain-proposals, err=%v", err)
	}
	if strings.Contains(strings.ToLower(stdout.String()), "productbrain") {
		t.Fatalf("documents stdout contains productbrain coupling: %s", stdout.String())
	}
}

func TestDocumentsDecomposeReportsWriteFailuresAsArtifactWrite(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(outFile, []byte("occupied"), 0o644); err != nil {
		t.Fatalf("write out file: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "decompose", documentsFixture(t, "markdown", "mixed-thread-capture.md"),
		"--out", outFile,
	}, &stdout, &stderr)
	if code != ExitArtifactWrite {
		t.Fatalf("expected artifact write exit, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "write document segments") {
		t.Fatalf("expected write error context, got %q", stderr.String())
	}
}

func documentsFixture(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("cannot resolve caller")
	}
	all := append([]string{filepath.Dir(file), "..", "..", "testdata", "documents"}, parts...)
	return filepath.Join(all...)
}
