package productbrain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/pipeline/runs"
)

func TestProposeRejectsSymlinkedReviewItemEscape(t *testing.T) {
	runDir := t.TempDir()
	mustMkdir(t, filepath.Join(runDir, "ledger"))
	mustMkdir(t, filepath.Join(runDir, "review-queue", "items"))

	writeTestJSON(t, filepath.Join(runDir, "ledger", "run-manifest.json"), runs.Manifest{
		SchemaVersion: runs.RunLedgerSchemaVersion,
		RunID:         "run-symlink-escape",
	})
	writeTestJSON(t, filepath.Join(runDir, "review-queue", "review-queue.json"), runs.ReviewQueue{
		SchemaVersion: runs.ReviewQueueSchemaVersion,
		RunID:         "run-symlink-escape",
		QueueCount:    1,
		Items: []runs.ReviewQueueEntry{
			{RecordID: "escaped", ReviewItemPath: "items/escaped.json"},
		},
	})

	outsideDir := t.TempDir()
	outsideItem := filepath.Join(outsideDir, "escaped.json")
	writeTestJSON(t, outsideItem, runs.ReviewQueueItem{
		SchemaVersion: runs.ReviewItemSchemaVersion,
		RunID:         "run-symlink-escape",
		RecordID:      "escaped",
		SafeTitle:     "Escaped item",
		SafeContext:   "Decision: this content came from outside the run bundle.",
	})
	if err := os.Symlink(outsideItem, filepath.Join(runDir, "review-queue", "items", "escaped.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := Propose(runDir, profileFixturePath(t, "default-governance.json"), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "escaped run directory") {
		t.Fatalf("Propose() error = %v, want escaped run directory", err)
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func profileFixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "productbrain", "profiles", name)
}
