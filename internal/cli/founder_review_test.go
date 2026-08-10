package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/founderreview"
)

func TestFounderReviewCLIRecordsOnlyStructuralDurableReceipt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "personal-memory")
	input := founderReviewInput{
		ProofRunID:                 strings.Repeat("A", 43),
		StructuralProofFingerprint: "sha256:" + strings.Repeat("a", 64),
		CitedRecordsFingerprint:    "sha256:" + strings.Repeat("b", 64),
		Verdict:                    founderreview.VerdictUseful,
		RetryToken:                 strings.Repeat("r", 16),
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	runner := NewRunnerWithInput(NewOSFileSystem(), bytes.NewReader(body))
	if code := runner.Run(
		[]string{"memory", "founder-review", "-", "--root", root},
		&stdout, &stderr,
	); code != ExitOK {
		t.Fatalf("review exit=%d stderr=%q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), input.RetryToken) ||
		strings.Contains(stdout.String(), root) ||
		!strings.Contains(stdout.String(), `"resolution": "user_value_closed"`) {
		t.Fatalf("founder review output is not structural: %s", stdout.String())
	}
}

func TestSiblingMemoryRootsKeepIndependentFounderReviews(t *testing.T) {
	parent := t.TempDir()
	for index, root := range []string{filepath.Join(parent, "memory-a"), filepath.Join(parent, "memory-b")} {
		input := founderReviewInput{
			ProofRunID:                 strings.Repeat(string(rune('A'+index)), 43),
			StructuralProofFingerprint: "sha256:" + strings.Repeat(string(rune('a'+index)), 64),
			CitedRecordsFingerprint:    "sha256:" + strings.Repeat(string(rune('c'+index)), 64),
			Verdict:                    founderreview.VerdictUseful,
			RetryToken:                 strings.Repeat(string(rune('r'+index)), 16),
		}
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if code := NewRunnerWithInput(NewOSFileSystem(), bytes.NewReader(body)).Run(
			[]string{"memory", "founder-review", "-", "--root", root}, &stdout, &stderr,
		); code != ExitOK {
			t.Fatalf("review %d exit=%d stderr=%q", index, code, stderr.String())
		}
	}
	if personalMemoryRuntimeRoot(filepath.Join(parent, "memory-a"), "founder-review-runtimes") ==
		personalMemoryRuntimeRoot(filepath.Join(parent, "memory-b"), "founder-review-runtimes") {
		t.Fatal("sibling memory roots share a founder review repository")
	}
}
