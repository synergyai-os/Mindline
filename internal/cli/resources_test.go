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
	for _, command := range []string{"resources-run", "resources-continue", "resources-reconcile", "resources-status", "resources-proof"} {
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

func TestResourceRetryRequiresApprovedReasonAndEmptyReplayIsStructural(t *testing.T) {
	root := filepath.Join(t.TempDir(), "personal-memory")
	runner := NewRunner(NewOSFileSystem())
	var stdout, stderr bytes.Buffer
	if code := runner.Run([]string{"memory", "resources-retry", "--root", root}, &stdout, &stderr); code != ExitUsage {
		t.Fatalf("missing retry reason exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"memory", "resources-retry", "--reason", "access_denied", "--root", root}, &stdout, &stderr); code != ExitUsage {
		t.Fatalf("permanent retry reason exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"memory", "resources-retry", "--reason", "unreachable", "--root", root}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("empty retry exit=%d stderr=%q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), root) || strings.Contains(stdout.String(), "http") {
		t.Fatalf("retry leaked private structure: %q", stdout.String())
	}
	var status struct {
		Generation     int            `json:"generation"`
		GenerationKind string         `json:"generation_kind"`
		TerminalCounts map[string]int `json:"terminal_counts"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil ||
		status.Generation != 0 || status.GenerationKind != "" || len(status.TerminalCounts) != 0 {
		t.Fatalf("empty retry status=%+v err=%v", status, err)
	}
}

func TestResourceReconcileReturnsNonterminalStructuralStatusWithoutNetwork(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-memory")
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	privateURL := "https://private.example.test/pending"
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "workspace", SourceContainerID: "self",
		ExternalID: "private-pending", OccurredAt: "2026-07-27T12:00:00Z",
		SourceRef: "slack://workspace/self/private-pending", RawText: privateURL,
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
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run(
		[]string{"memory", "resources-reconcile", "--root", root},
		&stdout, &stderr,
	)
	if code != ExitOK {
		t.Fatalf("reconcile exit=%d stderr=%q", code, stderr.String())
	}
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	queue, err := resourcequeue.NewStore(resourceQueueRoot(root), resourcequeue.LiveProfile())
	if err != nil {
		t.Fatal(err)
	}
	derived, err := queue.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(derived.Items) != 1 || derived.Items[0].State != resourcequeue.StateQueued {
		t.Fatalf("reconcile did not return a truthful nonterminal queue: %+v", derived)
	}
	for _, privateValue := range []string{root, privateURL, library.Resources[0].ResourceID, "private-pending", "resource-queue"} {
		if strings.Contains(stdout.String(), privateValue) || strings.Contains(stderr.String(), privateValue) {
			t.Fatalf("reconcile leaked private value %q: stdout=%q stderr=%q", privateValue, stdout.String(), stderr.String())
		}
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

func TestSiblingMemoryRootsKeepIndependentResourceQueues(t *testing.T) {
	parent := t.TempDir()
	rootA, rootB := filepath.Join(parent, "memory-a"), filepath.Join(parent, "memory-b")
	resourceA := importResourceForQueueTest(t, rootA, "https://example.test/shared")
	resourceB := importResourceForQueueTest(t, rootB, "https://example.test/shared")
	if resourceA != resourceB {
		t.Fatal("fixture did not produce the same canonical resource identity")
	}
	if resourceQueueRoot(rootA) == resourceQueueRoot(rootB) {
		t.Fatal("sibling memory roots share a resource queue")
	}
	storeA, err := resourcequeue.NewStore(resourceQueueRoot(rootA), resourcequeue.LiveProfile())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.Rebuild([]resourcequeue.RebuildItem{{ResourceID: resourceA, State: resourcequeue.StateComplete}}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := NewRunner(NewOSFileSystem()).Run([]string{"memory", "resources-reconcile", "--root", rootB}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("reconcile B exit=%d stderr=%q", code, stderr.String())
	}
	storeB, err := resourcequeue.NewStore(resourceQueueRoot(rootB), resourcequeue.LiveProfile())
	if err != nil {
		t.Fatal(err)
	}
	queueA, err := storeA.Load()
	if err != nil {
		t.Fatal(err)
	}
	queueB, err := storeB.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(queueA.Items) != 1 || queueA.Items[0].State != resourcequeue.StateComplete ||
		len(queueB.Items) != 1 || queueB.Items[0].State != resourcequeue.StateQueued ||
		queueB.Items[0].Attempts != 0 || queueB.Counters.Attempts != 0 {
		t.Fatalf("sibling queue state crossed roots: A=%+v B=%+v", queueA, queueB)
	}
	for _, privateValue := range []string{rootA, rootB, resourceA} {
		if strings.Contains(stdout.String(), privateValue) || strings.Contains(stderr.String(), privateValue) {
			t.Fatalf("resource reconcile leaked private value %q", privateValue)
		}
	}
}

func importResourceForQueueTest(t *testing.T, root, url string) string {
	t.Helper()
	repository, err := personalmemory.NewFileRepository(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "workspace", SourceContainerID: "self",
		ExternalID: "shared", OccurredAt: "2026-08-10T10:00:00Z",
		SourceRef: "slack://workspace/self/shared", RawText: url,
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
		t.Fatalf("fixture resources=%d err=%v", len(library.Resources), err)
	}
	return library.Resources[0].ResourceID
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
	queue, err := resourcequeue.NewStore(resourceQueueRoot(root), resourcequeue.LiveProfile())
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
