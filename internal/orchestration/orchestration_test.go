package orchestration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestAggregateRejectsIllegalTransitionAndConfigurationDrift(t *testing.T) {
	plan := testPlan(t)
	runID := RunID("synthetic-run-1")
	aggregate := Aggregate{RunID: runID}
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	configured := mustHandle(t, aggregate, Command{Kind: CommandConfigure, ExpectedVersion: 0, Plan: &plan, Now: now})
	if err := aggregate.Apply(configured); err != nil {
		t.Fatal(err)
	}
	if aggregate.State != StateConfigured || aggregate.Version != 1 || aggregate.PlanFingerprint != plan.Fingerprint {
		t.Fatalf("unexpected configured aggregate: %+v", aggregate)
	}

	if _, err := HandleCommand(aggregate, Command{Kind: CommandStartProof, ExpectedVersion: 1, Plan: &plan, Now: now}); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("illegal transition error = %v", err)
	}
	changed := plan
	changed.ComponentVersions = cloneStringsMap(plan.ComponentVersions)
	changed.ComponentVersions["processor"] = "synthetic/v0.2"
	if err := SealRunPlan(&changed); err != nil {
		t.Fatal(err)
	}
	if _, err := HandleCommand(aggregate, Command{Kind: CommandStartInventory, ExpectedVersion: 1, Plan: &changed, Now: now}); !errors.Is(err, ErrConfigurationDrift) {
		t.Fatalf("configuration drift error = %v", err)
	}
	if _, err := HandleCommand(aggregate, Command{Kind: CommandStartInventory, ExpectedVersion: 0, Plan: &plan, Now: now}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("version conflict error = %v", err)
	}
}

func TestAggregateRebuildsLegalFlowWithSuspension(t *testing.T) {
	plan := testPlan(t)
	runID := RunID("synthetic-run-2")
	aggregate := Aggregate{RunID: runID}
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	commands := []CommandKind{
		CommandConfigure,
		CommandStartInventory,
		CommandFreezeInventory,
		CommandSelectProof,
		CommandStartProof,
		CommandPause,
		CommandResume,
		CommandRequireCredential,
		CommandRestoreCredential,
		CommandCompleteProof,
		CommandConfirmDrain,
		CommandStartDrain,
		CommandSealQueue,
	}
	events := make([]Event, 0, len(commands))
	for _, kind := range commands {
		command := Command{Kind: kind, ExpectedVersion: aggregate.Version, Plan: &plan, Now: now.Add(time.Duration(aggregate.Version) * time.Second)}
		event := mustHandle(t, aggregate, command)
		if err := aggregate.Apply(event); err != nil {
			t.Fatalf("apply %s: %v", kind, err)
		}
		events = append(events, event)
	}
	if aggregate.State != StateQueueSealed || aggregate.ResumeState != "" {
		t.Fatalf("unexpected terminal state: %+v", aggregate)
	}
	rebuilt, err := Rebuild(runID, events)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.State != aggregate.State || rebuilt.Version != aggregate.Version || rebuilt.PlanFingerprint != aggregate.PlanFingerprint {
		t.Fatalf("rebuilt aggregate mismatch: got %+v want %+v", rebuilt, aggregate)
	}
}

func TestSelectProofSampleIsDeterministicCappedAndNotReplaceable(t *testing.T) {
	inventory := testInventory(t)
	manifest, err := SelectProofSample(inventory, "sample-order/v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Strata) != 2 || len(manifest.SelectedItemIDs) != 5 {
		t.Fatalf("unexpected sample: %+v", manifest)
	}
	for _, stratum := range manifest.Strata {
		if len(stratum.SelectedItemIDs) > 3 {
			t.Fatalf("stratum exceeded cap: %+v", stratum)
		}
		if len(stratum.SelectedItemIDs)+len(stratum.UnselectedItemIDs) != stratum.CanonicalCount {
			t.Fatalf("stratum accounting mismatch: %+v", stratum)
		}
	}

	reordered := inventory
	reordered.CanonicalItems = append([]InventoryItem(nil), inventory.CanonicalItems...)
	for left, right := 0, len(reordered.CanonicalItems)-1; left < right; left, right = left+1, right-1 {
		reordered.CanonicalItems[left], reordered.CanonicalItems[right] = reordered.CanonicalItems[right], reordered.CanonicalItems[left]
	}
	reordered.Fingerprint = ""
	if err := SealInventorySnapshot(&reordered); err != nil {
		t.Fatal(err)
	}
	again, err := SelectProofSample(reordered, "sample-order/v1")
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(manifest.SelectedItemIDs, again.SelectedItemIDs) {
		t.Fatalf("selection changed with input order: %v != %v", manifest.SelectedItemIDs, again.SelectedItemIDs)
	}

	tampered := manifest
	tampered.SelectedItemIDs = append([]string(nil), manifest.SelectedItemIDs...)
	tampered.SelectedItemIDs[0] = manifest.Strata[0].UnselectedItemIDs[0]
	tampered.Fingerprint = ""
	tampered.Fingerprint = Fingerprint(tampered)
	if err := ValidateSampleManifest(inventory, tampered); !errors.Is(err, ErrSampleChanged) {
		t.Fatalf("replacement error = %v", err)
	}
}

func TestSelectProofSampleAllowsTruthfulEmptyDenominator(t *testing.T) {
	inventory := InventorySnapshot{SchemaVersion: InventorySchemaVersion, SourceIdentity: "synthetic-empty", Watermark: "synthetic-watermark"}
	if err := SealInventorySnapshot(&inventory); err != nil {
		t.Fatal(err)
	}
	manifest, err := SelectProofSample(inventory, "sample-order/v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.SelectedItemIDs) != 0 || len(manifest.Strata) != 0 {
		t.Fatalf("empty inventory manufactured a sample: %+v", manifest)
	}
}

func TestSelectProofSampleEnforcesIndependentStrataAndTotalCaps(t *testing.T) {
	tooManyStrata := testInventoryWithStrata(t, MaximumProofStrata+1, 1)
	if _, err := SelectProofSample(tooManyStrata, "sample-order/v1"); !errors.Is(err, ErrSampleBudget) {
		t.Fatalf("strata cap did not fail closed: %v", err)
	}

	strataForTotalOverflow := MaximumProofSelectedItems/MaximumProofItemsPerStratum + 1
	if strataForTotalOverflow >= MaximumProofStrata {
		t.Fatal("test constants do not exercise an independent total cap")
	}
	tooManySelected := testInventoryWithStrata(t, strataForTotalOverflow, MaximumProofItemsPerStratum)
	if _, err := SelectProofSample(tooManySelected, "sample-order/v1"); !errors.Is(err, ErrSampleBudget) {
		t.Fatalf("total selected-items cap did not fail closed: %v", err)
	}
}

func TestReadinessFailsClosedOnMissingAndUnauthorizedNA(t *testing.T) {
	contribution := ReadinessContribution{
		ContributorID: "synthetic-source",
		Version:       "v1",
		RequiredChecks: []string{
			"identity",
			"scope",
		},
		Checks: []ReadinessCheck{{Name: "identity", Status: CheckPass, EvidenceFingerprint: "sha256:identity"}},
	}
	verdict := EvaluateReadiness(StageInventory, contribution)
	if verdict.Verdict != VerdictBlocked || !containsString(verdict.Blockers, "synthetic-source:scope:missing") {
		t.Fatalf("missing readiness check did not block: %+v", verdict)
	}

	contribution.Checks = append(contribution.Checks, ReadinessCheck{Name: "scope", Status: CheckNA, NARationale: "fixture has no remote scope"})
	verdict = EvaluateReadiness(StageInventory, contribution)
	if verdict.Verdict != VerdictBlocked || !containsString(verdict.Blockers, "synthetic-source:scope:na_not_authorized") {
		t.Fatalf("unauthorized N/A did not block: %+v", verdict)
	}

	contribution.Checks[1].ContractAllowsNA = true
	verdict = EvaluateReadiness(StageInventory, contribution)
	if verdict.Verdict != VerdictReady {
		t.Fatalf("authorized N/A did not pass: %+v", verdict)
	}
}

func TestActivationServiceUsesEventStoreCAS(t *testing.T) {
	store := &memoryEventStore{}
	service := NewActivationService(store, func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) })
	plan := testPlan(t)
	aggregate, err := service.Execute(context.Background(), RunID("synthetic-service"), Command{Kind: CommandConfigure, ExpectedVersion: 0, Plan: &plan})
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.State != StateConfigured || len(store.events) != 1 {
		t.Fatalf("unexpected service result: %+v events=%d", aggregate, len(store.events))
	}
}

type memoryEventStore struct{ events []Event }

func (s *memoryEventStore) Load(context.Context, RunID) ([]Event, error) {
	return append([]Event(nil), s.events...), nil
}

func (s *memoryEventStore) Append(_ context.Context, _ RunID, expected ExpectedVersion, events ...Event) error {
	if uint64(expected) != uint64(len(s.events)) {
		return ErrVersionConflict
	}
	s.events = append(s.events, events...)
	return nil
}

func mustHandle(t *testing.T, aggregate Aggregate, command Command) Event {
	t.Helper()
	event, err := HandleCommand(aggregate, command)
	if err != nil {
		t.Fatalf("handle %s: %v", command.Kind, err)
	}
	return event
}

func testPlan(t *testing.T) RunPlan {
	t.Helper()
	plan := RunPlan{
		SchemaVersion:          RunPlanSchemaVersion,
		SourceScopeFingerprint: "sha256:scope",
		InventoryFingerprint:   "sha256:inventory",
		StrategyFingerprint:    "sha256:strategy",
		ComponentVersions: map[string]string{
			"source":    "synthetic/v1",
			"processor": "synthetic/v0.1",
		},
		PrivacyPolicy:        PrivacyPolicySyntheticOnly,
		Mode:                 RunModeProof,
		IdempotencyNamespace: "synthetic:test",
		Budgets:              RunBudgets{MaximumItems: 100, MaximumBytes: 1 << 20, MaximumAttempts: 2, MaximumNetworkRequests: 100, MaximumWallTimeSeconds: 300, MaximumCostMicrounits: 1000, MaximumRetryAttempts: 2, ManualSupportTolerance: 25},
	}
	if err := SealRunPlan(&plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func testInventory(t *testing.T) InventorySnapshot {
	t.Helper()
	items := []InventoryItem{
		{CanonicalItemID: "a-1", CanonicalURL: "https://example.test/a1", RetrievalStrategyID: "article", FormatVariant: "html", OccurrenceIDs: []string{"occ-a1"}},
		{CanonicalItemID: "a-2", CanonicalURL: "https://example.test/a2", RetrievalStrategyID: "article", FormatVariant: "html", OccurrenceIDs: []string{"occ-a2"}},
		{CanonicalItemID: "a-3", CanonicalURL: "https://example.test/a3", RetrievalStrategyID: "article", FormatVariant: "html", OccurrenceIDs: []string{"occ-a3"}},
		{CanonicalItemID: "a-4", CanonicalURL: "https://example.test/a4", RetrievalStrategyID: "article", FormatVariant: "html", OccurrenceIDs: []string{"occ-a4"}},
		{CanonicalItemID: "a-5", CanonicalURL: "https://example.test/a5", RetrievalStrategyID: "article", FormatVariant: "html", OccurrenceIDs: []string{"occ-a5"}},
		{CanonicalItemID: "b-1", CanonicalURL: "https://example.test/b1", RetrievalStrategyID: "metadata", FormatVariant: "audio", OccurrenceIDs: []string{"occ-b1"}},
		{CanonicalItemID: "b-2", CanonicalURL: "https://example.test/b2", RetrievalStrategyID: "metadata", FormatVariant: "audio", OccurrenceIDs: []string{"occ-b2"}},
	}
	snapshot := InventorySnapshot{SchemaVersion: InventorySchemaVersion, SourceIdentity: "synthetic-workspace", Watermark: "synthetic-watermark", CanonicalItems: items}
	for index, item := range items {
		recordID := "src-" + item.CanonicalItemID
		occurrenceID := item.OccurrenceIDs[0]
		snapshot.SourceRecords = append(snapshot.SourceRecords, SourceRecord{SourceRecordID: recordID, NativeMessageID: recordID, NativeTimestamp: "2026-07-14T12:00:00Z", ContentFingerprint: "sha256:" + item.CanonicalItemID, URLOccurrenceIDs: []string{occurrenceID}})
		snapshot.URLOccurrences = append(snapshot.URLOccurrences, URLOccurrence{URLOccurrenceID: occurrenceID, SourceRecordID: recordID, ObservedURL: item.CanonicalURL, CanonicalItemID: item.CanonicalItemID})
		_ = index
	}
	if err := SealInventorySnapshot(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func testInventoryWithStrata(t *testing.T, strata, itemsPerStratum int) InventorySnapshot {
	t.Helper()
	snapshot := InventorySnapshot{SchemaVersion: InventorySchemaVersion, SourceIdentity: "synthetic-capped-workspace", Watermark: "synthetic-watermark"}
	for stratum := 0; stratum < strata; stratum++ {
		for itemIndex := 0; itemIndex < itemsPerStratum; itemIndex++ {
			itemID := fmt.Sprintf("item-%03d-%02d", stratum, itemIndex)
			occurrenceID := "occ-" + itemID
			recordID := "src-" + itemID
			canonicalURL := "https://example.test/" + itemID
			snapshot.CanonicalItems = append(snapshot.CanonicalItems, InventoryItem{
				CanonicalItemID: itemID, CanonicalURL: canonicalURL, RetrievalStrategyID: fmt.Sprintf("strategy-%03d", stratum), FormatVariant: "html", OccurrenceIDs: []string{occurrenceID},
			})
			snapshot.SourceRecords = append(snapshot.SourceRecords, SourceRecord{
				SourceRecordID: recordID, NativeMessageID: recordID, NativeTimestamp: "2026-07-14T12:00:00Z", ContentFingerprint: "sha256:" + itemID, URLOccurrenceIDs: []string{occurrenceID},
			})
			snapshot.URLOccurrences = append(snapshot.URLOccurrences, URLOccurrence{
				URLOccurrenceID: occurrenceID, SourceRecordID: recordID, ObservedURL: canonicalURL, CanonicalItemID: itemID,
			})
		}
	}
	if err := SealInventorySnapshot(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
