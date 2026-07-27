package runjournal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

func TestStoreConcurrentCASAllowsOneWriter(t *testing.T) {
	root, err := privateio.CreateRuntimeRoot(t.TempDir(), "journal-cas-")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	runID := orchestration.RunID("synthetic-cas")
	plan := journalTestPlan(t)
	event, err := orchestration.HandleCommand(orchestration.Aggregate{RunID: runID}, orchestration.Command{Kind: orchestration.CommandConfigure, ExpectedVersion: 0, Plan: &plan, Now: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- store.Append(context.Background(), runID, 0, event) }()
	}
	var successes, conflicts int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, orchestration.ErrVersionConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected concurrent append error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestStoreAppendLoadCASProjectionAndTamper(t *testing.T) {
	root, err := privateio.CreateRuntimeRoot(t.TempDir(), "journal-")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, StoreOptions{MaximumJournalBytes: 1 << 20, MaximumEventBytes: 32 << 10})
	if err != nil {
		t.Fatal(err)
	}
	runID := orchestration.RunID("synthetic-run")
	plan := journalTestPlan(t)
	aggregate := orchestration.Aggregate{RunID: runID}
	configured, err := orchestration.HandleCommand(aggregate, orchestration.Command{Kind: orchestration.CommandConfigure, ExpectedVersion: 0, Plan: &plan, Now: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), runID, 0, configured); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), runID, 0, configured); !errors.Is(err, orchestration.ErrVersionConflict) {
		t.Fatalf("CAS error = %v", err)
	}
	events, err := store.Load(context.Background(), runID)
	if err != nil || len(events) != 1 || events[0].EventHash == "" {
		t.Fatalf("loaded events = %+v, %v", events, err)
	}
	projection, err := store.RebuildProjection(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.State != orchestration.StateConfigured || projection.Version != 1 || projection.JournalFingerprint == "" {
		t.Fatalf("unexpected projection: %+v", projection)
	}

	journalPath := filepath.Join(root, "runs", string(runID), journalFilename)
	data, err := privateio.ReadFileBounded(root, journalPath, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), `"to": "configured"`, `"to": "queue_sealed"`, 1)
	if tampered == string(data) {
		t.Fatal("fixture did not locate transition payload")
	}
	if err := privateio.WriteFile(journalPath, []byte(tampered), false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background(), runID); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("tamper error = %v", err)
	}
}

func TestStoreRejectsUnknownAndUnsafeFiles(t *testing.T) {
	root, err := privateio.CreateRuntimeRoot(t.TempDir(), "journal-files-")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, StoreOptions{})
	if err != nil {
		t.Fatal(err)
	}
	runID := orchestration.RunID("synthetic-files")
	if err := privateio.PrepareDir(filepath.Join(root, "runs", string(runID))); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(root, "runs", string(runID), "unexpected.json")
	if err := os.WriteFile(unknown, []byte("{}"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(context.Background(), runID); !errors.Is(err, ErrUnknownRunFile) {
		t.Fatalf("unknown file error = %v", err)
	}
}

func TestLeaseAcquireRenewExpiryAndRelease(t *testing.T) {
	root, err := privateio.CreateRuntimeRoot(t.TempDir(), "journal-lease-")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	manager, err := NewLeaseManager(root, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	runID := orchestration.RunID("synthetic-lease")
	lease, err := manager.Acquire(context.Background(), runID, "worker-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Acquire(context.Background(), runID, "worker-b", time.Minute); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("second acquire error = %v", err)
	}
	now = now.Add(30 * time.Second)
	lease, err = manager.Renew(context.Background(), lease, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Validate(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if err := manager.Validate(context.Background(), lease); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired validation = %v", err)
	}
	replacement, err := manager.Acquire(context.Background(), runID, "worker-b", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Token == lease.Token {
		t.Fatal("expired lease token was reused")
	}
	if err := manager.Release(context.Background(), replacement); err != nil {
		t.Fatal(err)
	}
	if err := manager.Validate(context.Background(), replacement); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("released validation = %v", err)
	}
}

func journalTestPlan(t *testing.T) orchestration.RunPlan {
	t.Helper()
	plan := orchestration.RunPlan{
		SchemaVersion:          orchestration.RunPlanSchemaVersion,
		SourceScopeFingerprint: "sha256:scope",
		InventoryFingerprint:   "sha256:inventory",
		StrategyFingerprint:    "sha256:strategy",
		ComponentVersions:      map[string]string{"source": "synthetic/v1"},
		PrivacyPolicy:          orchestration.PrivacyPolicySyntheticOnly,
		Mode:                   orchestration.RunModeProof,
		IdempotencyNamespace:   "synthetic:test",
		Budgets:                orchestration.RunBudgets{MaximumItems: 10, MaximumBytes: 1 << 20, MaximumAttempts: 2, MaximumNetworkRequests: 10, MaximumWallTimeSeconds: 300, MaximumCostMicrounits: 1000, MaximumRetryAttempts: 2, ManualSupportTolerance: 25},
	}
	if err := orchestration.SealRunPlan(&plan); err != nil {
		t.Fatal(err)
	}
	return plan
}
