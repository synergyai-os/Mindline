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

func TestDocumentsAccept(t *testing.T) {
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	answerKey := filepath.Join(t.TempDir(), "answer-key.json")
	if err := os.WriteFile(answerKey, []byte(`{
  "schema_version": "semantic-acceptance-answer-key/v0.1",
  "answer_key_id": "ak-cli",
  "source_document_id": "doc-transcript-consolidated-action",
  "expected_outcomes": [
    {
      "expected_outcome_id": "exp-action",
      "expected_state": "expected_present",
      "expected_kind": "action_candidate",
      "required_evidence": ["node-262592341686a94b"],
      "title_signals": ["checklist"],
      "summary_signals": ["prepare"],
      "minimum_confidence_floor": "low"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write answer key: %v", err)
	}

	acceptOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "accept", semanticOut,
		"--answer-key", answerKey,
		"--out", acceptOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected accept exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	var summary struct {
		SchemaVersion             string  `json:"schema_version"`
		MatchedExpectedCount      int     `json:"matched_expected_count"`
		PrecisionLikeMatchRate    float64 `json:"precision_like_match_rate"`
		QualityStatement          string  `json:"quality_statement"`
		RecallLikeOutcomeCoverage float64 `json:"recall_like_expected_outcome_coverage"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if summary.SchemaVersion != "semantic-acceptance-summary/v0.1" {
		t.Fatalf("unexpected schema: %s", summary.SchemaVersion)
	}
	if summary.MatchedExpectedCount != 1 || summary.PrecisionLikeMatchRate == 0 || summary.RecallLikeOutcomeCoverage == 0 {
		t.Fatalf("unexpected acceptance summary: %+v", summary)
	}
	if !strings.Contains(summary.QualityStatement, "not calibrated") {
		t.Fatalf("expected not-calibrated quality statement: %+v", summary)
	}
	if _, err := os.Stat(filepath.Join(acceptOut, "semantic-acceptance", "acceptance-summary.json")); err != nil {
		t.Fatalf("expected acceptance summary artifact: %v", err)
	}
}

func TestDocumentsCalibrateAndCalibrateNext(t *testing.T) {
	acceptOut := documentsAcceptanceFixture(t)
	calibrateOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "calibrate", filepath.Join(acceptOut, "semantic-acceptance"),
		"--out", calibrateOut,
		"--threshold", "0.98",
		"--held-out",
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected calibrate exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	var summary struct {
		SchemaVersion   string `json:"schema_version"`
		ThresholdStatus string `json:"threshold_status"`
		NoHumanEligible bool   `json:"no_human_eligible"`
		ReviewItemCount int    `json:"review_item_count"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode calibrate stdout: %v\n%s", err, stdout.String())
	}
	if summary.SchemaVersion != "semantic-calibration-summary/v0.1" || summary.ReviewItemCount == 0 {
		t.Fatalf("unexpected calibration summary: %+v", summary)
	}
	if _, err := os.Stat(filepath.Join(calibrateOut, "semantic-calibration", "calibration-summary.json")); err != nil {
		t.Fatalf("expected calibration summary artifact: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "calibrate-next", filepath.Join(calibrateOut, "semantic-calibration"),
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected calibrate-next exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	var page struct {
		SchemaVersion string           `json:"schema_version"`
		Done          bool             `json:"done"`
		Item          *json.RawMessage `json:"item,omitempty"`
		Cursor        struct {
			ProcessedCount int `json:"processed_count"`
			RemainingCount int `json:"remaining_count"`
		} `json:"cursor"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &page); err != nil {
		t.Fatalf("decode calibrate-next stdout: %v\n%s", err, stdout.String())
	}
	if page.SchemaVersion != "semantic-calibration-page/v0.1" || page.Done || page.Item == nil || page.Cursor.ProcessedCount != 1 {
		t.Fatalf("calibrate-next must return one item page: %+v", page)
	}
}

func TestDocumentsAcceptRejectsDestinationAndProfileFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "accept", t.TempDir(),
		"--answer-key", filepath.Join(t.TempDir(), "answer-key.json"),
		"--profile", documentsFixture(t, "..", "productbrain", "profiles", "default-governance.json"),
		"--out", t.TempDir(),
	}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage exit for --profile, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage: mindline documents accept") {
		t.Fatalf("expected documents accept usage, got %q", stderr.String())
	}
}

func TestDocumentsCalibrateRejectsDestinationAndProfileFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "calibrate", t.TempDir(),
		"--profile", documentsFixture(t, "..", "productbrain", "profiles", "default-governance.json"),
		"--out", t.TempDir(),
	}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage exit for --profile, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage: mindline documents calibrate") {
		t.Fatalf("expected documents calibrate usage, got %q", stderr.String())
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

func documentsAcceptanceFixture(t *testing.T) string {
	t.Helper()
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	answerKey := filepath.Join(t.TempDir(), "answer-key.json")
	if err := os.WriteFile(answerKey, []byte(`{
  "schema_version": "semantic-acceptance-answer-key/v0.1",
  "answer_key_id": "ak-cli-calibration",
  "source_document_id": "doc-transcript-consolidated-action",
  "expected_outcomes": [
    {
      "expected_outcome_id": "exp-action",
      "expected_state": "expected_present",
      "expected_kind": "action_candidate",
      "required_evidence": ["node-262592341686a94b"],
      "title_signals": ["checklist"],
      "summary_signals": ["prepare"],
      "minimum_confidence_floor": "low"
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write answer key: %v", err)
	}
	acceptOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "accept", semanticOut,
		"--answer-key", answerKey,
		"--out", acceptOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected accept exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	return acceptOut
}
