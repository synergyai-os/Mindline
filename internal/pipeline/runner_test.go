package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerGoldenTextOnlyFixture(t *testing.T) {
	out := t.TempDir()
	summary, err := Run(filepath.Join("..", "..", "testdata", "pipeline", "inputs", "pipeline-text-only.json"), out, RunOptions{})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	if summary.ItemCount != 1 || summary.BlockedCount != 0 {
		t.Fatalf("unexpected counts: %+v", summary)
	}
	preview := filepath.Join(out, "destinations", "pipeline-text-only", "previews")
	entries, err := os.ReadDir(preview)
	if err != nil {
		t.Fatalf("read preview dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one preview, got %d", len(entries))
	}
	body, err := os.ReadFile(filepath.Join(preview, entries[0].Name()))
	if err != nil {
		t.Fatalf("read preview: %v", err)
	}
	if !strings.Contains(string(body), "## Snapshot") {
		t.Fatalf("expected method-shaped preview:\n%s", body)
	}
	if strings.Contains(string(body), "PRIVATE_DM_SENTINEL_DO_NOT_WRITE") {
		t.Fatalf("private sentinel leaked")
	}
}

func TestRunnerWritesLedgerAndReviewQueueForTextOnlyFixture(t *testing.T) {
	out := t.TempDir()
	summary, err := Run(filepath.Join("..", "..", "testdata", "pipeline", "inputs", "pipeline-text-only.json"), out, RunOptions{})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	for _, relative := range []string{
		"ledger/run-manifest.json",
		"ledger/index.json",
		"ledger/items/pipeline-text-only.json",
		"review-queue/review-queue.json",
	} {
		if _, err := os.Stat(filepath.Join(out, relative)); err != nil {
			t.Fatalf("expected %s: %v", relative, err)
		}
	}
	if summary.RunManifest.ReviewQueueCount != 0 || summary.ReviewQueue.QueueCount != 0 {
		t.Fatalf("text-only publish should not require review: manifest=%+v queue=%+v", summary.RunManifest, summary.ReviewQueue)
	}
	data, err := os.ReadFile(filepath.Join(out, "ledger", "items", "pipeline-text-only.json"))
	if err != nil {
		t.Fatalf("read ledger item: %v", err)
	}
	var ledger struct {
		State          string `json:"state"`
		ReviewRequired bool   `json:"review_required"`
	}
	if err := json.Unmarshal(data, &ledger); err != nil {
		t.Fatalf("decode ledger item: %v", err)
	}
	if ledger.State != "published_preview" || ledger.ReviewRequired {
		t.Fatalf("unexpected ledger item: %+v", ledger)
	}
}

func TestRunnerQueuesMissingEnrichment(t *testing.T) {
	out := t.TempDir()
	summary, err := Run(filepath.Join("..", "..", "testdata", "pipeline", "inputs", "pipeline-youtube-url.json"), out, RunOptions{})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	if summary.RunManifest.ReviewQueueCount != 1 || summary.ReviewQueue.QueueCount != 1 {
		t.Fatalf("expected one review item: manifest=%+v queue=%+v", summary.RunManifest, summary.ReviewQueue)
	}
	queueItem := filepath.Join(out, "review-queue", "items", "pipeline-youtube-url.json")
	data, err := os.ReadFile(queueItem)
	if err != nil {
		t.Fatalf("read queue item: %v", err)
	}
	var item struct {
		State               string            `json:"state"`
		Reason              string            `json:"reason"`
		SuggestedNextAction string            `json:"suggested_next_action"`
		Links               map[string]string `json:"links"`
	}
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatalf("decode queue item: %v", err)
	}
	if item.State != "needs_enrichment" || item.Reason != "missing_local_youtube_transcript" {
		t.Fatalf("unexpected queue item: %+v", item)
	}
	for key, value := range item.Links {
		if strings.Contains(value, "..") || filepath.IsAbs(value) {
			t.Fatalf("unsafe queue link %s=%q", key, value)
		}
	}
}

func TestRunnerKeepsPrivateProvenanceOutOfReviewQueue(t *testing.T) {
	out := t.TempDir()
	summary, err := Run(filepath.Join("..", "..", "testdata", "pipeline", "inputs", "pipeline-private-provenance.json"), out, RunOptions{})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	if summary.ReviewQueue.QueueCount != 0 {
		t.Fatalf("private provenance alone should not require review: %+v", summary.ReviewQueue)
	}
	data, err := os.ReadFile(filepath.Join(out, "ledger", "items", "pipeline-private-provenance.json"))
	if err != nil {
		t.Fatalf("read ledger item: %v", err)
	}
	var item struct {
		State          string `json:"state"`
		ReviewRequired bool   `json:"review_required"`
	}
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatalf("decode ledger item: %v", err)
	}
	if item.State != "background" || item.ReviewRequired {
		t.Fatalf("unexpected private ledger state: %+v", item)
	}
}

func TestRunnerRefusesDifferentRunInExistingOutputDirectory(t *testing.T) {
	out := t.TempDir()
	firstInput := filepath.Join("..", "..", "testdata", "pipeline", "inputs", "pipeline-text-only.json")
	first, err := Run(firstInput, out, RunOptions{})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	manifestPath := filepath.Join(out, "ledger", "run-manifest.json")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest before: %v", err)
	}
	secondInput := filepath.Join("..", "..", "testdata", "pipeline", "inputs", "pipeline-youtube-url.json")
	_, err = Run(secondInput, out, RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "existing output directory contains a different run") {
		t.Fatalf("expected different-run refusal, got %v", err)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("manifest mutated after refused run")
	}
	again, err := Run(firstInput, out, RunOptions{})
	if err != nil {
		t.Fatalf("same run should be idempotent: %v", err)
	}
	if again.RunManifest.RunID != first.RunManifest.RunID {
		t.Fatalf("same input changed run id: %q != %q", again.RunManifest.RunID, first.RunManifest.RunID)
	}
}

func TestRunnerBlocksPrivateAndSecretFixtures(t *testing.T) {
	for _, name := range []string{"pipeline-private-provenance.json", "pipeline-secret-like.json"} {
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			summary, err := Run(filepath.Join("..", "..", "testdata", "pipeline", "inputs", name), out, RunOptions{})
			if err != nil {
				t.Fatalf("run pipeline: %v", err)
			}
			if summary.BlockedCount != 1 {
				t.Fatalf("expected blocked count 1, got %d", summary.BlockedCount)
			}
			if err := filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				data, readErr := os.ReadFile(path)
				if readErr != nil {
					return readErr
				}
				for _, sentinel := range []string{"PRIVATE_DM_SENTINEL_DO_NOT_WRITE", "sk-test-secret-do-not-leak"} {
					if strings.Contains(string(data), sentinel) {
						t.Fatalf("sentinel leaked in %s", path)
					}
				}
				return nil
			}); err != nil {
				t.Fatalf("walk output: %v", err)
			}
		})
	}
}

func TestRunnerRefusesWhenDestinationPrerequisiteMissing(t *testing.T) {
	runner := Runner{DestinationAvailable: func(adapterID string) bool { return false }}
	_, err := runner.Run(filepath.Join("..", "..", "testdata", "pipeline", "inputs", "pipeline-text-only.json"), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "WP-5 destination dry-run support is required before pipeline delivery") {
		t.Fatalf("unexpected error: %v", err)
	}
}
