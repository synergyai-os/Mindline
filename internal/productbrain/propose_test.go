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

func TestProposeRejectsReviewItemMissingSourceCandidateID(t *testing.T) {
	runDir := t.TempDir()
	mustMkdir(t, filepath.Join(runDir, "ledger"))
	mustMkdir(t, filepath.Join(runDir, "review-queue", "items"))

	writeTestJSON(t, filepath.Join(runDir, "ledger", "run-manifest.json"), runs.Manifest{
		SchemaVersion: runs.RunLedgerSchemaVersion,
		RunID:         "run-missing-source",
	})
	writeTestJSON(t, filepath.Join(runDir, "review-queue", "review-queue.json"), runs.ReviewQueue{
		SchemaVersion: runs.ReviewQueueSchemaVersion,
		RunID:         "run-missing-source",
		QueueCount:    1,
		Items: []runs.ReviewQueueEntry{
			{RecordID: "missing-source", ReviewItemPath: "items/missing-source.json"},
		},
	})
	writeTestJSON(t, filepath.Join(runDir, "review-queue", "items", "missing-source.json"), runs.ReviewQueueItem{
		SchemaVersion: runs.ReviewItemSchemaVersion,
		RunID:         "run-missing-source",
		RecordID:      "missing-source",
		SafeTitle:     "Choose workspace profile bridge",
		SafeContext:   "Decision: use Product Brain workspace profiles before live writes.",
	})

	_, err := Propose(runDir, profileFixturePath(t, "default-governance.json"), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "missing source_candidate_id") {
		t.Fatalf("Propose() error = %v, want missing source_candidate_id", err)
	}
}

func TestProposeKeepsExternalRefStableAcrossFreshRuns(t *testing.T) {
	first := writeProposalReplayRun(t, "run-first", "review-decision-a", "slack-DSELF-1710000000000001")
	second := writeProposalReplayRun(t, "run-second", "review-decision-b", "slack-DSELF-1710000000000001")

	firstOut := t.TempDir()
	firstSummary, err := Propose(first, profileFixturePath(t, "default-governance.json"), firstOut)
	if err != nil {
		t.Fatalf("first Propose() error = %v", err)
	}
	secondOut := t.TempDir()
	secondSummary, err := Propose(second, profileFixturePath(t, "default-governance.json"), secondOut)
	if err != nil {
		t.Fatalf("second Propose() error = %v", err)
	}

	firstProposal := readSingleProposal(t, firstSummary, firstOut)
	secondProposal := readSingleProposal(t, secondSummary, secondOut)
	if firstProposal.ExternalRef != secondProposal.ExternalRef {
		t.Fatalf("external refs differ across fresh runs: %+v != %+v", firstProposal.ExternalRef, secondProposal.ExternalRef)
	}
	if strings.Contains(firstProposal.ExternalRef.ID, "run-first") || strings.Contains(firstProposal.ExternalRef.ID, "review-decision") {
		t.Fatalf("external ref leaked run/review identity: %+v", firstProposal.ExternalRef)
	}
	if firstProposal.IdempotencyKey == secondProposal.IdempotencyKey {
		t.Fatalf("idempotency keys must differ across fresh runs")
	}
}

func writeProposalReplayRun(t *testing.T, runID string, reviewID string, sourceCandidateID string) string {
	t.Helper()
	runDir := t.TempDir()
	mustMkdir(t, filepath.Join(runDir, "ledger"))
	mustMkdir(t, filepath.Join(runDir, "review-queue", "items"))
	writeTestJSON(t, filepath.Join(runDir, "ledger", "run-manifest.json"), runs.Manifest{
		SchemaVersion: runs.RunLedgerSchemaVersion,
		RunID:         runID,
	})
	writeTestJSON(t, filepath.Join(runDir, "review-queue", "review-queue.json"), runs.ReviewQueue{
		SchemaVersion: runs.ReviewQueueSchemaVersion,
		RunID:         runID,
		QueueCount:    1,
		Items: []runs.ReviewQueueEntry{
			{RecordID: reviewID, ReviewItemPath: "items/" + reviewID + ".json"},
		},
	})
	writeTestJSON(t, filepath.Join(runDir, "review-queue", "items", reviewID+".json"), runs.ReviewQueueItem{
		SchemaVersion:     runs.ReviewItemSchemaVersion,
		RunID:             runID,
		RecordID:          reviewID,
		SourceCandidateID: sourceCandidateID,
		SafeTitle:         "Choose workspace profile bridge",
		SafeContext:       "Decision: use Product Brain workspace profiles before live writes.",
	})
	return runDir
}

func readSingleProposal(t *testing.T, summary Summary, outDir string) Proposal {
	t.Helper()
	if len(summary.Proposals) != 1 {
		t.Fatalf("proposal count = %d, want 1", len(summary.Proposals))
	}
	data, err := os.ReadFile(filepath.Join(outDir, "productbrain-proposals", summary.Proposals[0].ProposalPath))
	if err != nil {
		t.Fatalf("read proposal: %v", err)
	}
	var proposal Proposal
	if err := json.Unmarshal(data, &proposal); err != nil {
		t.Fatalf("decode proposal: %v", err)
	}
	return proposal
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
