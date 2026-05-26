package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/documents"
	"github.com/synergyai-os/Mindline/internal/observability"
)

type httpRoundTripper func(*http.Request) (*http.Response, error)

func (fn httpRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

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
	if _, err := os.Stat(filepath.Join(out, "trace", "trace-summary.json")); err != nil {
		t.Fatalf("expected semantic trace summary artifact: %v", err)
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

func TestDocumentsSemanticsExportsMetadataTraceWhenEnabled(t *testing.T) {
	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "true")
	t.Setenv("MINDLINE_LLM_TRACE_MODE", "metadata")
	t.Setenv("MINDLINE_TELEMETRY_SALT", "salt")
	t.Setenv("POSTHOG_PROJECT_API_KEY", "phc-test")
	t.Setenv("POSTHOG_HOST", "https://eu.i.posthog.com")
	t.Setenv("MINDLINE_LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_MODEL", "gpt-test")
	out := t.TempDir()
	var capturedBodies []string
	runner := NewRunnerWithPostHogTransport(NewOSFileSystem(), httpRoundTripper(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		capturedBodies = append(capturedBodies, string(body))
		if containsUnsafePostHogBody(string(body)) {
			t.Fatalf("PostHog body contains unsafe content: %s", string(body))
		}
		if strings.Contains(string(body), "gpt-test") || strings.Contains(string(body), "$ai_model") || strings.Contains(string(body), "$ai_provider") {
			t.Fatalf("deterministic semantic run must not export ambient LLM metadata: %s", string(body))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	}))
	var stdout, stderr bytes.Buffer

	code := runner.Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", out,
	}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if len(capturedBodies) != 1 {
		t.Fatalf("expected one PostHog event, got %d", len(capturedBodies))
	}
	if _, err := os.Stat(filepath.Join(out, "trace", "trace-summary.json")); err != nil {
		t.Fatalf("expected trace summary artifact: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "trace", "trace-summary.json"))
	if err != nil {
		t.Fatalf("read trace summary: %v", err)
	}
	if strings.Contains(string(data), "gpt-test") || strings.Contains(string(data), "openai") {
		t.Fatalf("deterministic semantic trace must not include ambient LLM metadata: %s", string(data))
	}
}

func TestDocumentsSemanticsContinuesWhenPostHogExportFails(t *testing.T) {
	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "true")
	t.Setenv("MINDLINE_LLM_TRACE_MODE", "metadata")
	t.Setenv("MINDLINE_TELEMETRY_SALT", "salt")
	t.Setenv("POSTHOG_PROJECT_API_KEY", "phc-test")
	t.Setenv("POSTHOG_HOST", "https://eu.i.posthog.com")
	out := t.TempDir()
	runner := NewRunnerWithPostHogTransport(NewOSFileSystem(), httpRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
	}))
	var stdout, stderr bytes.Buffer

	code := runner.Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", out,
	}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected semantic command to ignore PostHog failure, got %d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "trace", "trace-summary.json")); err != nil {
		t.Fatalf("expected trace summary artifact despite PostHog failure: %v", err)
	}
	if !strings.Contains(stderr.String(), "posthog capture:") {
		t.Fatalf("expected PostHog warning on stderr, got %q", stderr.String())
	}
}

func TestWriteAndExportTraceFailsOnUnsafeTelemetryEvent(t *testing.T) {
	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "true")
	t.Setenv("MINDLINE_LLM_TRACE_MODE", "metadata")
	t.Setenv("MINDLINE_TELEMETRY_SALT", "salt")
	t.Setenv("POSTHOG_PROJECT_API_KEY", "phc-test")
	t.Setenv("POSTHOG_HOST", "https://eu.i.posthog.com")
	out := t.TempDir()
	called := false
	runner := NewRunnerWithPostHogTransport(NewOSFileSystem(), httpRoundTripper(func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	}))
	summary := observability.NewTraceSummary("semantic", "documents semantics", "run-safe", "sk-test", "", "ok", "review", nil, nil, []string{"$ai_generation"})
	var stderr bytes.Buffer

	err := runner.writeAndExportTrace(out, summary, &stderr)

	if err == nil || !strings.Contains(err.Error(), "unsafe PostHog property value") {
		t.Fatalf("expected unsafe telemetry event to fail closed, got %v", err)
	}
	if called {
		t.Fatalf("unsafe telemetry event must fail before network")
	}
	if strings.Contains(stderr.String(), "posthog capture:") {
		t.Fatalf("safety validation failure must not be downgraded to warning: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "trace", "trace-summary.json")); err != nil {
		t.Fatalf("expected local trace artifact before export validation failure: %v", err)
	}
}

func TestDocumentsSemanticsRejectsSymlinkedTraceOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on windows")
	}
	out := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(out, "trace")); err != nil {
		t.Fatalf("symlink trace dir: %v", err)
	}
	var stdout, stderr bytes.Buffer

	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", out,
	}, &stdout, &stderr)

	if code != ExitArtifactWrite {
		t.Fatalf("expected artifact write exit for symlinked trace dir, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "escaped output directory") {
		t.Fatalf("expected containment error, got %q", stderr.String())
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

func TestDocumentsSemanticsLLMRequiresConfiguredProviderBeforeSourceRead(t *testing.T) {
	fs := NewMemoryFS()
	fs.files["temp/private.md"] = []byte("# Private\nsource")
	runner := NewRunner(fs)
	var stdout, stderr bytes.Buffer

	code := runner.Run([]string{
		"documents", "semantics", "temp/private.md",
		"--out", "out",
		"--classifier", "llm",
		"--llm-provider", "openai",
	}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "missing OpenAI model") {
		t.Fatalf("expected missing OpenAI model before source read, got %q", stderr.String())
	}
}

func TestDocumentsSemanticsRejectsUnsupportedLLMProviderBeforeSourceRead(t *testing.T) {
	fs := NewMemoryFS()
	fs.files["temp/private.md"] = []byte("# Private\nsource")
	runner := NewRunner(fs)
	var stdout, stderr bytes.Buffer

	code := runner.Run([]string{
		"documents", "semantics", "temp/private.md",
		"--out", "out",
		"--classifier", "llm",
		"--llm-provider", "gemini",
		"--llm-model", "gemini-test",
	}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "unsupported LLM provider: gemini") {
		t.Fatalf("expected unsupported provider before source read, got %q", stderr.String())
	}
}

func TestObservabilityPostHogTestDisabledByDefault(t *testing.T) {
	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "")
	t.Setenv("MINDLINE_LLM_TRACE_MODE", "")
	t.Setenv("MINDLINE_TELEMETRY_SALT", "")
	t.Setenv("POSTHOG_PROJECT_API_KEY", "")
	t.Setenv("POSTHOG_API_KEY", "")
	t.Setenv("POSTHOG_HOST", "")
	var stdout, stderr bytes.Buffer

	code := NewRunner(NewMemoryFS()).Run([]string{"observability", "posthog-test"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	var envelope ObservabilityTestEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if envelope.State != "telemetry_disabled" || envelope.Network {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}

func TestObservabilityPostHogTestRejectsRawModeBeforeNetwork(t *testing.T) {
	fs := NewMemoryFS()
	fs.files[".env.local"] = []byte("MINDLINE_TELEMETRY_ENABLED=true\nMINDLINE_LLM_TRACE_MODE=raw\nPOSTHOG_PROJECT_API_KEY=phc-test\nPOSTHOG_HOST=https://eu.i.posthog.com\nMINDLINE_TELEMETRY_SALT=salt\n")
	var stdout, stderr bytes.Buffer

	code := NewRunner(fs).Run([]string{"observability", "posthog-test"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "unsupported LLM trace mode: raw") {
		t.Fatalf("expected raw mode rejection, got %q", stderr.String())
	}
}

func TestObservabilityPostHogTestSendsMetadataOnlyEvent(t *testing.T) {
	fs := NewMemoryFS()
	fs.files[".env.local"] = []byte("MINDLINE_TELEMETRY_ENABLED=true\nMINDLINE_LLM_TRACE_MODE=metadata\nPOSTHOG_PROJECT_API_KEY=phc-test\nPOSTHOG_HOST=https://eu.i.posthog.com\nMINDLINE_TELEMETRY_SALT=salt\n")
	var capturedBody string
	runner := NewRunnerWithPostHogTransport(fs, httpRoundTripper(func(req *http.Request) (*http.Response, error) {
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

	code := runner.Run([]string{"observability", "posthog-test"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if capturedBody == "" {
		t.Fatalf("expected PostHog request")
	}
	var envelope ObservabilityTestEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if envelope.State != "posthog_test_sent" || !envelope.Network {
		t.Fatalf("unexpected envelope: %+v", envelope)
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
      "acceptable_evidence_alternates": ["node-262592341686a94b"],
      "title_signals": ["checklist"],
      "summary_signals": ["prepare"],
      "relation_requirements": ["derived_from"],
      "minimum_confidence_floor": "low",
      "notes": "CLI expected action."
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
	if summary.SchemaVersion != "semantic-calibration-summary/v0.2" || summary.ReviewItemCount == 0 {
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
		PageMarkdown  string           `json:"page_markdown"`
		ReviewContext *json.RawMessage `json:"review_context,omitempty"`
		Cursor        struct {
			ProcessedCount int `json:"processed_count"`
			RemainingCount int `json:"remaining_count"`
		} `json:"cursor"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &page); err != nil {
		t.Fatalf("decode calibrate-next stdout: %v\n%s", err, stdout.String())
	}
	if page.SchemaVersion != "semantic-calibration-page/v0.3" || page.Done || page.Item == nil || page.ReviewContext == nil || page.PageMarkdown == "" || page.Cursor.ProcessedCount != 1 {
		t.Fatalf("calibrate-next must return one item page: %+v", page)
	}
	if !strings.Contains(page.PageMarkdown, "Adjudication choices") {
		t.Fatalf("expected content-rich calibration page markdown, got %q", page.PageMarkdown)
	}
}

func TestDocumentsJudgeJudgeNextAndJudgeRecord(t *testing.T) {
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	var summary struct {
		SchemaVersion         string `json:"schema_version"`
		CandidateCount        int    `json:"candidate_count"`
		RemainingCount        int    `json:"remaining_count"`
		EvidenceExcludedCount int    `json:"evidence_excluded_count"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode judge stdout: %v\n%s", err, stdout.String())
	}
	if summary.SchemaVersion != "semantic-judgment-summary/v0.3" || summary.CandidateCount == 0 || summary.RemainingCount != summary.CandidateCount || summary.EvidenceExcludedCount == 0 {
		t.Fatalf("unexpected judgment summary: %+v", summary)
	}
	if _, err := os.Stat(filepath.Join(judgeOut, "trace", "trace-summary.json")); err != nil {
		t.Fatalf("expected judgment trace summary artifact: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge-next", filepath.Join(judgeOut, "semantic-judgment"),
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge-next exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	var page struct {
		SchemaVersion string `json:"schema_version"`
		Done          bool   `json:"done"`
		PageMarkdown  string `json:"page_markdown"`
		Item          *struct {
			CandidateID       string `json:"candidate_id"`
			EvidenceReadiness struct {
				Status      string   `json:"status"`
				EvalCounted bool     `json:"eval_counted"`
				ReasonCodes []string `json:"reason_codes"`
			} `json:"evidence_readiness"`
		} `json:"item"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &page); err != nil {
		t.Fatalf("decode judge-next stdout: %v\n%s", err, stdout.String())
	}
	if page.SchemaVersion != "semantic-judgment-page/v0.3" || page.Done || page.Item == nil || !strings.Contains(page.PageMarkdown, "Adjudication choices") || !strings.Contains(page.PageMarkdown, "Evidence readiness") || !strings.Contains(page.PageMarkdown, "Failure reason contract") {
		t.Fatalf("unexpected judgment page: %+v", page)
	}
	if page.Item.EvidenceReadiness.Status == "" || len(page.Item.EvidenceReadiness.ReasonCodes) == 0 || page.Item.EvidenceReadiness.EvalCounted {
		t.Fatalf("expected judge-next item readiness exclusion without source context: %+v", page.Item.EvidenceReadiness)
	}
	report, err := os.ReadFile(filepath.Join(judgeOut, "semantic-judgment", "reports", "judgment-report.md"))
	if err != nil {
		t.Fatalf("read judgment report: %v", err)
	}
	if !strings.Contains(string(report), "Evidence readiness") || !strings.Contains(string(report), "Eval counted") || !strings.Contains(string(report), "missing_source_excerpt") {
		t.Fatalf("expected readiness section in judgment report:\n%s", string(report))
	}

	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge-record", filepath.Join(judgeOut, "semantic-judgment"),
		"--candidate", page.Item.CandidateID,
		"--choice", "accept",
		"--note", "Looks useful.",
		"--reviewer", "cli-test",
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge-record exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	var updated struct {
		SchemaVersion string  `json:"schema_version"`
		JudgedCount   int     `json:"judged_count"`
		AcceptedCount int     `json:"accepted_count"`
		Precision     float64 `json:"precision_estimate"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &updated); err != nil {
		t.Fatalf("decode judge-record stdout: %v\n%s", err, stdout.String())
	}
	if updated.SchemaVersion != "semantic-judgment-summary/v0.3" || updated.JudgedCount != 1 || updated.AcceptedCount != 1 || updated.Precision != 1 {
		t.Fatalf("unexpected judgment record summary: %+v", updated)
	}
}

func TestDocumentsJudgeContinuesWhenPostHogExportFails(t *testing.T) {
	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "false")
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "true")
	t.Setenv("MINDLINE_LLM_TRACE_MODE", "metadata")
	t.Setenv("MINDLINE_TELEMETRY_SALT", "salt")
	t.Setenv("POSTHOG_PROJECT_API_KEY", "phc-test")
	t.Setenv("POSTHOG_HOST", "https://eu.i.posthog.com")
	runner := NewRunnerWithPostHogTransport(NewOSFileSystem(), httpRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable")), Header: make(http.Header)}, nil
	}))
	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()

	code = runner.Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected judge command to ignore PostHog failure, got %d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(judgeOut, "trace", "trace-summary.json")); err != nil {
		t.Fatalf("expected trace summary artifact despite PostHog failure: %v", err)
	}
	if !strings.Contains(stderr.String(), "posthog capture:") {
		t.Fatalf("expected PostHog warning on stderr, got %q", stderr.String())
	}
}

func TestDocumentsJudgeExportsMetadataTraceWhenEnabled(t *testing.T) {
	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "false")
	t.Setenv("MINDLINE_LLM_PROVIDER", "openai")
	t.Setenv("OPENAI_MODEL", "gpt-test")
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	t.Setenv("MINDLINE_TELEMETRY_ENABLED", "true")
	t.Setenv("MINDLINE_LLM_TRACE_MODE", "metadata")
	t.Setenv("MINDLINE_TELEMETRY_SALT", "salt")
	t.Setenv("POSTHOG_PROJECT_API_KEY", "phc-test")
	t.Setenv("POSTHOG_HOST", "https://eu.i.posthog.com")
	var capturedBodies []string
	runner := NewRunnerWithPostHogTransport(NewOSFileSystem(), httpRoundTripper(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		capturedBodies = append(capturedBodies, string(body))
		if containsUnsafePostHogBody(string(body)) {
			t.Fatalf("PostHog body contains unsafe content: %s", string(body))
		}
		if strings.Contains(string(body), "gpt-test") || strings.Contains(string(body), "$ai_model") || strings.Contains(string(body), "$ai_provider") {
			t.Fatalf("non-agent judgment run must not export ambient LLM metadata: %s", string(body))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	}))
	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()

	code = runner.Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected judge exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if len(capturedBodies) != 2 {
		t.Fatalf("expected generation and feedback PostHog events, got %d", len(capturedBodies))
	}
	if _, err := os.Stat(filepath.Join(judgeOut, "trace", "trace-summary.json")); err != nil {
		t.Fatalf("expected trace summary artifact: %v", err)
	}
	var trace struct {
		Counts map[string]int `json:"counts"`
	}
	data, err := os.ReadFile(filepath.Join(judgeOut, "trace", "trace-summary.json"))
	if err != nil {
		t.Fatalf("read trace summary: %v", err)
	}
	if strings.Contains(string(data), "gpt-test") || strings.Contains(string(data), "openai") {
		t.Fatalf("non-agent judgment trace must not include ambient LLM metadata: %s", string(data))
	}
	if err := json.Unmarshal(data, &trace); err != nil {
		t.Fatalf("decode trace summary: %v", err)
	}
	if _, ok := trace.Counts["failure_reason_count.unexpected_candidate"]; !ok {
		t.Fatalf("expected failure taxonomy counts in trace: %+v", trace.Counts)
	}
	if _, ok := trace.Counts["evidence_readiness_reason_count.missing_source_excerpt"]; !ok {
		t.Fatalf("expected evidence readiness taxonomy counts in trace: %+v", trace.Counts)
	}
}

func TestSemanticJudgmentTraceSummaryLabelsAgentModelErrors(t *testing.T) {
	summary := documents.SemanticJudgmentSummary{
		RunID:                         "run-model-error",
		SourceCount:                   1,
		CandidateCount:                1,
		AgentReviewedCount:            1,
		HumanReviewRequiredCount:      1,
		FailureReasonCounts:           map[documents.SemanticFailureReason]int{},
		EvidenceReadinessReasonCounts: map[documents.SemanticEvidenceReadinessReason]int{},
		Items: []documents.SemanticJudgmentCandidate{
			{
				CandidateID: "candidate-1",
				AgentReview: &documents.SemanticAgentReviewProposal{
					ReviewReasonCodes: []documents.SemanticAgentReviewReasonCode{
						documents.SemanticAgentReviewReasonModelError,
					},
				},
			},
		},
	}

	trace := semanticJudgmentTraceSummary(summary, documents.SemanticJudgmentOptions{
		Reviewer:    documents.SemanticJudgmentReviewerLLM,
		LLMProvider: "openai",
		LLMModel:    "gpt-test",
	})

	if trace.Labels["judgment_model_errors"] != 1 {
		t.Fatalf("expected model error label from agent review reason codes, got %+v", trace.Labels)
	}
}

func containsUnsafePostHogBody(body string) bool {
	for _, marker := range []string{`"source_text":`, `"source_excerpt":`, `"prompt":`, `"completion":`} {
		if strings.Contains(body, marker) {
			return true
		}
	}
	return false
}

func TestDocumentsJudgeServeStateAndRecord(t *testing.T) {
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	root := filepath.Join(judgeOut, "semantic-judgment")
	handler := newSemanticJudgmentUIHandler(root, "ui-test")
	token := judgmentUIToken(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = judgmentUITestHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected ui status 200, got %d", rec.Code)
	}
	var html bytes.Buffer
	if _, err := html.ReadFrom(rec.Body); err != nil {
		t.Fatalf("read ui html: %v", err)
	}
	for _, want := range []string{"Mindline Review", "Review", "Guide", "How to review", "Decision meanings", "remaining", "current-candidate", "decision-controls", "failure-reason", "evidence", "Review task", "Should this candidate count", "Agent proposal", "not a saved judgment", "Evidence highlights", "Raw details", "Save decision", "details class=\\\"raw-details\\\"", "openRawDetails", "visibleEvidenceLimit", "selectedChoice", "isSubmitting", "failureReasonsByChoice[selectedChoice]"} {
		if !strings.Contains(html.String(), want) {
			t.Fatalf("expected UI HTML to contain %q, got %s", want, html.String())
		}
	}
	for _, want := range []string{"selectDecision(choice)", "submitSelectedChoice()", "renderFailureReasonOptions(selectedChoice)", "ensureCompatibleFailureReason(selectedChoice)"} {
		if !strings.Contains(html.String(), want) {
			t.Fatalf("expected UI HTML to preselect compatible failure reasons with %q, got %s", want, html.String())
		}
	}
	if strings.Contains(html.String(), "<details class=\"raw-details\" open") {
		t.Fatalf("expected raw details to be collapsed by default")
	}
	uiHTML := html.String()
	for _, want := range []string{
		"const visibleEvidenceLimit = 5",
		"const visibleExcerpts = allExcerpts.slice(0, visibleEvidenceLimit)",
		"const hiddenExcerptCount = Math.max(0, allExcerpts.length - visibleEvidenceLimit)",
		"hiddenExcerptCount > 0",
		"page.cursor.remaining_count",
		"Human review queue clear",
		"machine-triaged proposal-only and remain unjudged/auditable",
		"const rawExcerpts = allExcerpts.map(excerptHtml).join(\"\")",
		"const reasons = failureReasonsByChoice[selectedChoice] || failureReasonsByChoice[activeChoice] || []",
		"button.disabled = isSubmitting",
		"saveButton.disabled = isSubmitting || !currentCandidateId || !selectedChoice",
		"if (!currentCandidateId || isSubmitting) return",
		"} finally {",
	} {
		if !strings.Contains(uiHTML, want) {
			t.Fatalf("expected decision-first UI behavior contract to contain %q", want)
		}
	}
	if relationIndex := strings.Index(uiHTML, "\"<div><h3>Relation context</h3>\" + relationContext"); relationIndex == -1 {
		t.Fatalf("expected raw relation context to be rendered only through the raw-details section")
	} else {
		detailsIndex := strings.Index(uiHTML, "<details class=\\\"raw-details\\\"")
		if detailsIndex == -1 {
			t.Fatalf("expected raw details section in UI renderer")
		}
		if relationIndex < detailsIndex {
			t.Fatalf("expected relation context renderer to appear after collapsed raw details start")
		}
	}

	state := getJudgmentUIState(t, handler)
	if state.SchemaVersion != "semantic-judgment-ui-state/v0.1" {
		t.Fatalf("unexpected state schema: %+v", state)
	}
	if state.Summary.CandidateCount == 0 || state.Summary.RemainingCount != state.Summary.CandidateCount {
		t.Fatalf("expected aggregate review context in state: %+v", state.Summary)
	}
	if state.Page.Done || state.Page.Item == nil {
		t.Fatalf("expected exactly one current candidate in state: %+v", state.Page)
	}
	if len(state.Page.Item.RelationIDs) == 0 || len(state.Page.Item.Blockers) == 0 {
		t.Fatalf("expected fixture current item to exercise relations and blockers: %+v", state.Page.Item)
	}
	if len(state.Page.Item.RelationContext) == 0 {
		t.Fatalf("expected UI state to include resolved relation context: %+v", state.Page.Item)
	}
	if strings.TrimSpace(state.Page.Item.RelationContext[0].OtherEndpoint.Role) == "" {
		t.Fatalf("expected UI relation context to include other endpoint role: %+v", state.Page.Item.RelationContext[0])
	}
	firstCandidateID := state.Page.Item.CandidateID

	req = httptest.NewRequest(http.MethodPost, "/api/judgments", strings.NewReader(`{"candidate_id":"`+firstCandidateID+`","choice":"reject","failure_reason":"unsupported_evidence","note":"not supported enough"}`))
	req.Host = judgmentUITestHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected judgment status 200, got %d", rec.Code)
	}
	updated := decodeJudgmentUIState(t, rec.Body)
	if updated.Summary.JudgedCount != 1 || updated.Summary.RejectedCount != 1 || updated.Summary.FailureReasonCounts[documents.SemanticFailureUnsupportedEvidence] != 1 || updated.Summary.RemainingCount != state.Summary.CandidateCount-1 {
		t.Fatalf("expected updated aggregate context after UI judgment: %+v", updated.Summary)
	}
	if updated.Page.Item != nil && updated.Page.Item.CandidateID == firstCandidateID {
		t.Fatalf("expected UI to advance after recording judgment, still on %s", firstCandidateID)
	}
	judgmentPath := filepath.Join(root, "judgments", firstCandidateID+".json")
	if _, err := os.Stat(judgmentPath); err != nil {
		t.Fatalf("expected UI judgment artifact: %v", err)
	}
	var judgment documents.SemanticJudgmentRecord
	data, err := os.ReadFile(judgmentPath)
	if err != nil {
		t.Fatalf("read UI judgment artifact: %v", err)
	}
	if err := json.Unmarshal(data, &judgment); err != nil {
		t.Fatalf("decode UI judgment artifact: %v", err)
	}
	if judgment.FailureReason != documents.SemanticFailureUnsupportedEvidence {
		t.Fatalf("expected UI judgment to persist failure reason, got %+v", judgment)
	}
}

func TestDocumentsJudgeServeRejectsBadChoice(t *testing.T) {
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	handler := newSemanticJudgmentUIHandler(filepath.Join(judgeOut, "semantic-judgment"), "ui-test")
	token := judgmentUIToken(t, handler)

	state := getJudgmentUIState(t, handler)
	req := httptest.NewRequest(http.MethodPost, "/api/judgments", strings.NewReader(`{"candidate_id":"`+state.Page.Item.CandidateID+`","choice":"maybe"}`))
	req.Host = judgmentUITestHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad choice status 400, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/judgments", strings.NewReader(`{"candidate_id":"cand-missing","choice":"accept"}`))
	req.Host = judgmentUITestHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown candidate status 400, got %d", rec.Code)
	}
}

func TestDocumentsJudgeServeRejectsTokenlessAndCrossOriginPosts(t *testing.T) {
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	handler := newSemanticJudgmentUIHandler(filepath.Join(judgeOut, "semantic-judgment"), "ui-test")
	state := getJudgmentUIState(t, handler)

	payload := `{"candidate_id":"` + state.Page.Item.CandidateID + `","choice":"accept"}`
	req := httptest.NewRequest(http.MethodPost, "/api/judgments", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Host = judgmentUITestHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected tokenless judgment status 403, got %d", rec.Code)
	}

	token := judgmentUIToken(t, handler)
	req = httptest.NewRequest(http.MethodPost, "/api/judgments", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", token)
	req.Header.Set("Origin", "https://example.invalid")
	req.Host = judgmentUITestHost
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin judgment status 403, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/judgments", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", token)
	req.Header.Set("Origin", "http://"+judgmentUITestHost)
	req.Host = judgmentUITestHost
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected same-origin tokened judgment status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDocumentsJudgeServeRejectsNonJSONPosts(t *testing.T) {
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	handler := newSemanticJudgmentUIHandler(filepath.Join(judgeOut, "semantic-judgment"), "ui-test")
	state := getJudgmentUIState(t, handler)
	token := judgmentUIToken(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/judgments", strings.NewReader("candidate_id="+state.Page.Item.CandidateID+"&choice=accept"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Mindline-Review-Token", token)
	req.Header.Set("Origin", "http://"+judgmentUITestHost)
	req.Host = judgmentUITestHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected non-json judgment status 415, got %d", rec.Code)
	}
}

func TestDocumentsJudgeServeRejectsNonLoopbackHostEvenWithToken(t *testing.T) {
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	handler := newSemanticJudgmentUIHandler(filepath.Join(judgeOut, "semantic-judgment"), "ui-test")
	state := getJudgmentUIState(t, handler)
	token := judgmentUIToken(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/judgments", strings.NewReader(`{"candidate_id":"`+state.Page.Item.CandidateID+`","choice":"accept"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", token)
	req.Header.Set("Origin", "http://attacker.test:8787")
	req.Host = "attacker.test:8787"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected non-loopback host status 403, got %d", rec.Code)
	}
}

func TestDocumentsJudgeServeAllowsConfiguredLoopbackAliasHost(t *testing.T) {
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	aliasHost := "review.localhost:8787"
	handler := newSemanticJudgmentUIHandlerWithAllowedHosts(filepath.Join(judgeOut, "semantic-judgment"), "ui-test", []string{aliasHost})
	token := judgmentUITokenForHost(t, handler, aliasHost)
	state := getJudgmentUIStateForHost(t, handler, aliasHost)

	req := httptest.NewRequest(http.MethodPost, "/api/judgments", strings.NewReader(`{"candidate_id":"`+state.Page.Item.CandidateID+`","choice":"accept"}`))
	req.Host = aliasHost
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mindline-Review-Token", token)
	req.Header.Set("Origin", "http://"+aliasHost)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected configured alias host status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDocumentsJudgeServeRejectsUnconfiguredLocalhostAliasHost(t *testing.T) {
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	aliasHost := "review.localhost:8787"
	handler := newSemanticJudgmentUIHandler(filepath.Join(judgeOut, "semantic-judgment"), "ui-test")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = aliasHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected unconfigured localhost alias host status 403, got %d", rec.Code)
	}
}

func TestDocumentsJudgeServeRejectsConfiguredNonLocalAliasHost(t *testing.T) {
	semanticOut := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "semantics", documentsFixture(t, "semantic"),
		"--out", semanticOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected semantic generation exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}

	judgeOut := t.TempDir()
	stdout.Reset()
	stderr.Reset()
	code = NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge", semanticOut,
		"--out", judgeOut,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected judge exit %d, got %d stdout=%s stderr=%s", ExitOK, code, stdout.String(), stderr.String())
	}
	attackerHost := "attacker.test:8787"
	handler := newSemanticJudgmentUIHandlerWithAllowedHosts(filepath.Join(judgeOut, "semantic-judgment"), "ui-test", []string{attackerHost})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = attackerHost
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected configured non-local alias host status 403, got %d", rec.Code)
	}
}

func TestDocumentsJudgeServeLoopbackValidation(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8787", "localhost:8787", "[::1]:8787"} {
		t.Run("accepts_"+addr, func(t *testing.T) {
			if err := validateLoopbackAddr(addr); err != nil {
				t.Fatalf("expected loopback addr %q to pass: %v", addr, err)
			}
		})
	}
	for _, addr := range []string{"0.0.0.0:8787", "[::]:8787", "192.168.1.20:8787"} {
		t.Run("rejects_"+addr, func(t *testing.T) {
			if err := validateLoopbackAddr(addr); err == nil {
				t.Fatalf("expected non-loopback addr %q to fail", addr)
			}
		})
	}
}

const judgmentUITestHost = "127.0.0.1:8787"

func judgmentUIToken(t *testing.T, handler http.Handler) string {
	t.Helper()
	return judgmentUITokenForHost(t, handler, judgmentUITestHost)
}

func judgmentUITokenForHost(t *testing.T, handler http.Handler, host string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected ui status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	const marker = `name="mindline-review-token" content="`
	start := strings.Index(body, marker)
	if start < 0 {
		t.Fatalf("review token marker missing from UI HTML")
	}
	start += len(marker)
	end := strings.Index(body[start:], `"`)
	if end < 0 {
		t.Fatalf("review token content missing closing quote")
	}
	return body[start : start+end]
}

func getJudgmentUIState(t *testing.T, handler http.Handler) semanticJudgmentUIState {
	t.Helper()
	return getJudgmentUIStateForHost(t, handler, judgmentUITestHost)
}

func getJudgmentUIStateForHost(t *testing.T, handler http.Handler, host string) semanticJudgmentUIState {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected state status 200, got %d", rec.Code)
	}
	return decodeJudgmentUIState(t, rec.Body)
}

func decodeJudgmentUIState(t *testing.T, body io.Reader) semanticJudgmentUIState {
	t.Helper()
	var state semanticJudgmentUIState
	if err := json.NewDecoder(body).Decode(&state); err != nil {
		t.Fatalf("decode UI state: %v", err)
	}
	return state
}

func TestDocumentsJudgeRejectsDestinationAndProfileFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"documents", "judge", t.TempDir(),
		"--profile", documentsFixture(t, "..", "productbrain", "profiles", "default-governance.json"),
		"--out", t.TempDir(),
	}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected usage exit for --profile, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage: mindline documents judge") {
		t.Fatalf("expected documents judge usage, got %q", stderr.String())
	}
}

func TestDocumentsJudgeRejectsLLMFlagsWithoutAgentReviewer(t *testing.T) {
	fs := NewMemoryFS()
	runner := NewRunner(fs)
	var stdout, stderr bytes.Buffer

	code := runner.Run([]string{
		"documents", "judge", "semantic-run",
		"--out", "out",
		"--llm-provider", "openai",
		"--llm-model", "gpt-test",
	}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "LLM flags require --agent-reviewer llm") {
		t.Fatalf("expected LLM flag validation before source read, got %q", stderr.String())
	}
}

func TestDocumentsJudgeDefersLLMAPIKeyValidationForAgentReviewer(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("MINDLINE_LLM_PROVIDER", "")
	runner := NewRunner(NewMemoryFS())

	_, _, options, parseErr, configError := runner.parseDocumentsJudgeArgs([]string{
		"judge", "semantic-run",
		"--out", "out",
		"--agent-reviewer", "llm",
		"--llm-provider", "openai",
		"--llm-model", "gpt-test",
	})

	if parseErr != parseErrorNone {
		t.Fatalf("expected parse success, got %v", parseErr)
	}
	if configError != "" {
		t.Fatalf("expected API key validation to be deferred, got %q", configError)
	}
	if options.LLMAPIKey != "" {
		t.Fatalf("expected no API key in options, got %q", options.LLMAPIKey)
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

func TestDocumentsCalibrateRejectsNonFiniteThresholds(t *testing.T) {
	for _, threshold := range []string{"NaN", "+Inf", "-Inf"} {
		t.Run(threshold, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := NewRunner(NewOSFileSystem()).Run([]string{
				"documents", "calibrate", t.TempDir(),
				"--out", t.TempDir(),
				"--threshold", threshold,
			}, &stdout, &stderr)
			if code != ExitUsage {
				t.Fatalf("expected usage exit for threshold %s, got %d stdout=%s stderr=%s", threshold, code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "usage: mindline documents calibrate") {
				t.Fatalf("expected documents calibrate usage, got %q", stderr.String())
			}
		})
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
      "acceptable_evidence_alternates": ["node-262592341686a94b"],
      "title_signals": ["checklist"],
      "summary_signals": ["prepare"],
      "relation_requirements": ["derived_from"],
      "minimum_confidence_floor": "low",
      "notes": "CLI expected action."
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
