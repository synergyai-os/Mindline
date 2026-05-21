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

func TestPipelineDryRunRequiresFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"pipeline", "dry-run", fixturePipelineInput(t, "pipeline-text-only.json")}, &stdout, &stderr)
	if code != ExitProcess {
		t.Fatalf("expected exit %d, got %d", ExitProcess, code)
	}
	if !strings.Contains(stderr.String(), "missing required --out") {
		t.Fatalf("expected missing out error, got %q", stderr.String())
	}
}

func TestPipelineDryRunStdoutMatchesSummaryFile(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"pipeline", "dry-run", fixturePipelineInput(t, "pipeline-text-only.json"), "--method", "basb-para-code", "--destination", "tolaria", "--out", out}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(out, "pipeline-summary.json"))
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var got, want any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("decode file: %v", err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("stdout and file differ\nstdout=%s\nfile=%s", stdout.String(), data)
	}
}

func fixturePipelineInput(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("cannot resolve caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "pipeline", "inputs", name)
}
