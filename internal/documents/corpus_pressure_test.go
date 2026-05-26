package documents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusPressureBuildsReadableReportAndReplay(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "documents", "semantic")
	outA := t.TempDir()
	outB := t.TempDir()
	outC := t.TempDir()
	summaryA, _, err := BuildCorpusPressure(input, outA, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure A: %v", err)
	}
	summaryB, _, err := BuildCorpusPressure(input, outB, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure B: %v", err)
	}
	summaryC, _, err := BuildCorpusPressure(input, outC, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure C: %v", err)
	}
	if summaryA.SourceCount != 3 || summaryA.ProcessedSourceCount != 3 || summaryA.SkippedSourceCount != 0 || summaryA.BlockedSourceCount != 0 {
		t.Fatalf("unexpected source accounting: %+v", summaryA)
	}
	if summaryA.SemanticCandidateCount == 0 || summaryA.GraphAtomCount == 0 {
		t.Fatalf("expected semantic candidates and graph atoms: %+v", summaryA)
	}
	if summaryA.ReplayFingerprint != summaryB.ReplayFingerprint || summaryA.ReplayFingerprint != summaryC.ReplayFingerprint {
		t.Fatalf("pressure replay changed: %s %s %s", summaryA.ReplayFingerprint, summaryB.ReplayFingerprint, summaryC.ReplayFingerprint)
	}
	reportData, err := os.ReadFile(filepath.Join(outA, CorpusPressureDirName, "pressure-report.md"))
	if err != nil {
		t.Fatalf("read pressure report: %v", err)
	}
	report := string(reportData)
	for _, want := range []string{
		"## Corpus answer",
		"## Source accounting",
		"## Extracted candidates by source",
		"## Connected clusters",
		"## Duplicate candidates",
		"## Contradiction candidates",
		"## Evidence/readiness failures",
		"## Next improvement targets",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
	if summaryA.EvidenceReadyAtomCount < summaryA.GraphAtomCount && !strings.Contains(report, "evidence_incomplete_atom") {
		t.Fatalf("report must name evidence-incomplete atoms when readiness fails:\n%s", report)
	}
}

func TestCorpusPressureManifestRejectsEscapingSource(t *testing.T) {
	root := t.TempDir()
	manifest := `{
  "schema_version": "corpus-pressure-manifest/v0.1",
  "corpus_id": "bad-corpus",
  "sources": [{"source_id":"bad","source_kind":"markdown","path":"../outside.md"}]
}`
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := BuildCorpusPressure(manifestPath, t.TempDir(), CorpusPressureOptions{}); err == nil || !strings.Contains(err.Error(), "escaped") {
		t.Fatalf("expected containment error, got %v", err)
	}
}

func TestCorpusPressureDirectoryRejectsSymlinkSourceEscape(t *testing.T) {
	root := t.TempDir()
	in := filepath.Join(root, "in")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(in, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("# Outside\nsecret outside root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(in, "leak.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, _, err := BuildCorpusPressure(in, t.TempDir(), CorpusPressureOptions{}); err == nil || !strings.Contains(err.Error(), "escaped") {
		t.Fatalf("expected symlink containment error, got %v", err)
	}
}

func TestCorpusPressureRejectsOutputSourceSymlinkEscape(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "documents", "semantic")
	out := t.TempDir()
	escaped := filepath.Join(t.TempDir(), "escaped")
	if err := os.MkdirAll(filepath.Join(out, "sources"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(escaped, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escaped, filepath.Join(out, "sources", "process-capability-evidence")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{}); err != nil {
		t.Fatalf("build corpus pressure should account for blocked source and continue: %v", err)
	}
	if _, err := os.Stat(filepath.Join(escaped, "source.md")); err == nil {
		t.Fatalf("source copy escaped output root through symlink")
	}
}

func TestCorpusPressureRejectsPressureReportSymlinkEscape(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "documents", "semantic")
	out := t.TempDir()
	escaped := filepath.Join(t.TempDir(), "escaped-pressure")
	if err := os.MkdirAll(escaped, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escaped, filepath.Join(out, CorpusPressureDirName)); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{}); err == nil || !strings.Contains(err.Error(), "escaped") {
		t.Fatalf("expected pressure report symlink escape error, got %v", err)
	}
	for _, file := range []string{"pressure-summary.json", "pressure-report.md"} {
		if _, err := os.Stat(filepath.Join(escaped, file)); err == nil {
			t.Fatalf("pressure artifact escaped output root through symlink: %s", file)
		}
	}
}
