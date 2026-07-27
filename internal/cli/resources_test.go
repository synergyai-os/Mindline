package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/resourcequeue"
)

func TestResourceCommandsReturnStructuralOnlyStatusForEmptyLibrary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "personal-memory")
	runner := NewRunner(NewOSFileSystem())
	for _, command := range []string{"resources-run", "resources-continue", "resources-status", "resources-proof"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := runner.Run([]string{"memory", command, "--root", root}, &stdout, &stderr); code != ExitOK {
				t.Fatalf("%s exit=%d stderr=%q", command, code, stderr.String())
			}
			if strings.Contains(stdout.String(), root) || strings.Contains(stdout.String(), "resource-queue") || strings.Contains(stdout.String(), "http") {
				t.Fatalf("%s leaked a private path or URL: %s", command, stdout.String())
			}
			var status struct {
				SchemaVersion     string         `json:"schema_version"`
				BudgetFingerprint string         `json:"budget_fingerprint"`
				Generation        int            `json:"generation"`
				DeferredCount     int            `json:"deferred_count"`
				TerminalCounts    map[string]int `json:"terminal_counts"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &status); err != nil || status.SchemaVersion == "" || status.BudgetFingerprint == "" || status.Generation != 0 || status.DeferredCount != 0 || len(status.TerminalCounts) != 0 {
				t.Fatalf("%s structural JSON = %#v err=%v", command, status, err)
			}
		})
	}
}

func TestResourceRebuildProofRejectsNonTerminalDerivedQueue(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memory")
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run(
		[]string{"memory", "resources-rebuild-proof", "--root", root},
		&stdout, &stderr,
	)
	if code != ExitOK {
		t.Fatalf("empty terminal rebuild proof failed: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var output map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output["state"] != "pass" || output["all_terminal"] != true {
		t.Fatalf("unexpected rebuild proof: %#v", output)
	}
}

func TestResourceCommandsRejectUnexpectedArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := NewRunner(NewOSFileSystem()).Run([]string{"memory", "resources-status", "unexpected"}, &stdout, &stderr); code != ExitUsage {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestResourceContinueReturnsTerminalStructuralStatusWithoutPrivateValues(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-memory")
	repository, err := personalmemory.NewFileRepository(root, func() time.Time {
		return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	privateURL := "https://private.example.test/saved-item"
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "workspace", SourceContainerID: "self",
		ExternalID: "private-message", OccurredAt: "2026-07-27T12:00:00Z",
		SourceRef: "slack://workspace/self/private-message", RawText: privateURL,
		EditDeleteState: "original", Missingness: []string{"permalink_unavailable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "slack:workspace:self", LowerInclusive: "1", UpperInclusive: "1",
		Watermark: "1", DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	library, err := repository.Load()
	if err != nil || len(library.Resources) != 1 {
		t.Fatalf("library resources=%d err=%v", len(library.Resources), err)
	}
	queue, err := resourcequeue.NewStore(filepath.Join(filepath.Dir(root), "resource-queue"), resourcequeue.LiveProfile())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Rebuild([]resourcequeue.RebuildItem{{
		ResourceID: library.Resources[0].ResourceID,
		State:      resourcequeue.StateBlocked,
		Reason:     "unreachable",
	}}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run(
		[]string{"memory", "resources-continue", "--root", root},
		&stdout, &stderr,
	)
	if code != ExitOK {
		t.Fatalf("continue exit=%d stderr=%q", code, stderr.String())
	}
	for _, privateValue := range []string{root, privateURL, library.Resources[0].ResourceID, "private-message", "resource-queue"} {
		if strings.Contains(stdout.String(), privateValue) || strings.Contains(stderr.String(), privateValue) {
			t.Fatalf("continue leaked private value %q: stdout=%q stderr=%q", privateValue, stdout.String(), stderr.String())
		}
	}
	var status struct {
		Generation     int            `json:"generation"`
		DeferredCount  int            `json:"deferred_count"`
		TerminalCounts map[string]int `json:"terminal_counts"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil ||
		status.Generation != 0 || status.DeferredCount != 0 ||
		status.TerminalCounts[resourcequeue.StateBlocked] != 1 {
		t.Fatalf("terminal structural status=%+v err=%v", status, err)
	}
}
