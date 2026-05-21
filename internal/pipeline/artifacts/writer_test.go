package artifacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/pipeline/runs"
)

func TestWriterWritesTextOnlyOutput(t *testing.T) {
	out := t.TempDir()
	output := Output{
		SchemaVersion: "pipeline-summary/v0.1",
		RunMode:       "dry_run",
		MethodID:      "basb-para-code",
		DestinationID: "tolaria",
		ItemCount:     1,
		Items: []Item{{
			CandidateID:        "pipeline-text-only",
			State:              "dry_run_published",
			Result:             map[string]any{"state": "dry_run_published"},
			ProcessorPlan:      map[string]any{"schema_version": "processor-plan/v0.1"},
			DestinationSummary: map[string]any{"operation_count": float64(1)},
			OperationFiles:     []OperationFile{{Path: "destinations/pipeline-text-only/operations/op.json", Body: map[string]any{"operation_id": "op"}}},
			PreviewFiles:       []PreviewFile{{Path: "destinations/pipeline-text-only/previews/op.md", Body: "# Processed source pipeline-text-only\n\n## Snapshot\n\nMindline should keep raw capture, method policy, and destination preview separate.\n"}},
		}},
		AuthorityIDs: []string{"DEC-15", "DEC-6", "DEC-12", "DEC-13"},
	}

	if err := Write(out, output, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, relative := range []string{
		"pipeline-summary.json",
		"results/pipeline-text-only.json",
		"processors/pipeline-text-only.json",
		"destinations/pipeline-text-only/destination-summary.json",
		"destinations/pipeline-text-only/operations/op.json",
		"destinations/pipeline-text-only/previews/op.md",
	} {
		if _, err := os.Stat(filepath.Join(out, relative)); err != nil {
			t.Fatalf("expected %s: %v", relative, err)
		}
	}
}

func TestWriterWritesLedgerAndReviewQueue(t *testing.T) {
	out := t.TempDir()
	output := goldenTextOnlyPipelineOutputWithLedger()
	err := Write(out, output, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
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
}

func goldenTextOnlyPipelineOutputWithLedger() Output {
	authorityIDs := []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"}
	item := runs.BuildLedgerItem("run-abc", runs.ItemInput{
		RecordID:          "pipeline-text-only",
		SourceCandidateID: "pipeline-text-only",
		PipelineState:     "dry_run_published",
		PreviewPaths:      []string{"destinations/pipeline-text-only/previews/op.md"},
		SafeTitle:         "Mindline pipeline text-only fixture",
	}, authorityIDs)
	return Output{
		SchemaVersion: "pipeline-summary/v0.1",
		RunMode:       "dry_run",
		MethodID:      "basb-para-code",
		DestinationID: "tolaria",
		ItemCount:     1,
		Items: []Item{{
			CandidateID:        "pipeline-text-only",
			State:              "dry_run_published",
			Result:             map[string]any{"state": "dry_run_published"},
			ProcessorPlan:      map[string]any{"schema_version": "processor-plan/v0.1"},
			DestinationSummary: map[string]any{"operation_count": float64(1)},
			PreviewFiles:       []PreviewFile{{Path: "destinations/pipeline-text-only/previews/op.md", Body: "# Processed source pipeline-text-only\n"}},
		}},
		AuthorityIDs: authorityIDs,
		RunManifest: runs.BuildManifest(runs.ManifestInput{
			RunID:            "run-abc",
			RunMode:          "dry_run",
			MethodID:         "basb-para-code",
			DestinationID:    "tolaria",
			InputFingerprint: "sha256:test",
			Items:            []runs.LedgerItem{item},
			AuthorityIDs:     authorityIDs,
			Now:              "2026-05-21T00:00:00Z",
		}),
		LedgerItems: []runs.LedgerItem{item},
		LedgerIndex: runs.BuildIndex("run-abc", []runs.LedgerItem{item}, authorityIDs),
		ReviewQueue: runs.BuildReviewQueue("run-abc", []runs.LedgerItem{item}, authorityIDs),
	}
}

func TestWriterRejectsProtectedTolariaOutputAndSentinels(t *testing.T) {
	protected := filepath.Join(t.TempDir(), "PKM - Tolaria")
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatalf("mkdir protected: %v", err)
	}
	err := Write(protected, Output{Items: []Item{{CandidateID: "x"}}}, []string{protected})
	if err == nil || !strings.Contains(err.Error(), "protected Tolaria vault") {
		t.Fatalf("expected protected output error, got %v", err)
	}

	out := t.TempDir()
	err = Write(out, Output{Items: []Item{{CandidateID: "private", Result: "PRIVATE_DM_SENTINEL_DO_NOT_WRITE"}}}, nil)
	if err == nil || !strings.Contains(err.Error(), "sentinel") {
		t.Fatalf("expected sentinel rejection, got %v", err)
	}
}

func TestWriterRejectsSymlinkedOutputSubdirectory(t *testing.T) {
	out := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(out, "results")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	output := Output{Items: []Item{{
		CandidateID:        "pipeline-text-only",
		Result:             map[string]any{"state": "dry_run_published"},
		ProcessorPlan:      map[string]any{"schema_version": "processor-plan/v0.1"},
		DestinationSummary: map[string]any{"operation_count": 1},
	}}}

	err := Write(out, output, nil)
	if err == nil || !strings.Contains(err.Error(), "output path escaped output directory") {
		t.Fatalf("expected symlink escape rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "pipeline-text-only.json")); err == nil {
		t.Fatalf("writer followed symlinked output directory")
	}
}

func TestWriterAssignsUniquePathsForCollidingCandidateSlugs(t *testing.T) {
	out := t.TempDir()
	output := Output{Items: []Item{
		{
			CandidateID:        "candidate-a",
			Result:             map[string]any{"candidate": "first"},
			ProcessorPlan:      map[string]any{"candidate": "first"},
			DestinationSummary: map[string]any{"candidate": "first"},
		},
		{
			CandidateID:        "candidate-a",
			Result:             map[string]any{"candidate": "second"},
			ProcessorPlan:      map[string]any{"candidate": "second"},
			DestinationSummary: map[string]any{"candidate": "second"},
		},
	}}

	if err := Write(out, output, nil); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "results", "candidate-a.json")); err != nil {
		t.Fatalf("expected first result path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "results", "candidate-a-2.json")); err != nil {
		t.Fatalf("expected collision result path: %v", err)
	}
}
