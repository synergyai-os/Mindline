package pipeline

import (
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
