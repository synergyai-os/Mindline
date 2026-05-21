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

func TestProductBrainProposeWritesSummaryAndCustomTargets(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{
		"product-brain", "propose", productBrainRunFixture(t),
		"--profile", productBrainProfileFixture(t, "custom-workspace.json"),
		"--out", out,
	}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	var summary struct {
		ProposalCount int `json:"proposal_count"`
		BlockedCount  int `json:"blocked_count"`
		Proposals     []struct {
			Status               string `json:"status"`
			TargetCollectionSlug string `json:"target_collection_slug"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if summary.ProposalCount != 2 || summary.BlockedCount != 1 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	if summary.Proposals[0].TargetCollectionSlug != "choices" {
		t.Fatalf("expected custom choices target, got %+v", summary.Proposals[0])
	}
	if _, err := os.Stat(filepath.Join(out, "productbrain-proposals", "proposal-summary.json")); err != nil {
		t.Fatalf("expected proposal summary: %v", err)
	}
}

func TestProductBrainProposeRejectsMissingArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"product-brain", "propose", productBrainRunFixture(t)}, &stdout, &stderr)
	if code != ExitUsage {
		t.Fatalf("expected exit %d, got %d", ExitUsage, code)
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout")
	}
	if !strings.Contains(stderr.String(), "usage: mindline product-brain propose") {
		t.Fatalf("expected product-brain usage, got %q", stderr.String())
	}
}

func productBrainRunFixture(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("cannot resolve caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "productbrain", "runs", "reviewable")
}

func productBrainProfileFixture(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("cannot resolve caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "productbrain", "profiles", name)
}
