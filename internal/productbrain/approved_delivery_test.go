package productbrain

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type oneTimeHumanVerifier struct {
	expected string
	used     map[string]bool
	calls    int
}

func (v *oneTimeHumanVerifier) VerifyAndConsume(_ context.Context, evidence HumanInitiationEvidence) error {
	v.calls++
	if evidence.Fingerprint != v.expected || v.used[evidence.ReviewNonceFingerprint] {
		return errors.New("invalid or replayed human initiation")
	}
	v.used[evidence.ReviewNonceFingerprint] = true
	return nil
}

type approvedFakeTransport struct {
	*memoryPBTransport
	entryCalls    map[string]int
	relationCalls map[string]int
	mutationErr   error
}

type blockingApprovedTransport struct {
	*approvedFakeTransport
	started chan struct{}
	release chan struct{}
}

func (transport *blockingApprovedTransport) CreateEntry(ctx context.Context, request CreateEntryRequest) (CreateEntryResult, error) {
	select {
	case transport.started <- struct{}{}:
	default:
	}
	select {
	case <-transport.release:
	case <-ctx.Done():
		return CreateEntryResult{}, ctx.Err()
	}
	return transport.approvedFakeTransport.CreateEntry(ctx, request)
}

func newApprovedFakeTransport(profile DeliveryProfile) *approvedFakeTransport {
	return &approvedFakeTransport{memoryPBTransport: newMemoryPBTransport(profile), entryCalls: map[string]int{}, relationCalls: map[string]int{}}
}

func (f *approvedFakeTransport) CreateEntry(ctx context.Context, request CreateEntryRequest) (CreateEntryResult, error) {
	f.entryCalls[request.EntryID]++
	if f.mutationErr != nil {
		return CreateEntryResult{}, f.mutationErr
	}
	return f.memoryPBTransport.CreateEntry(ctx, request)
}

func (f *approvedFakeTransport) CreateEntryRelation(ctx context.Context, request CreateRelationRequest) (CreateRelationResult, error) {
	key := request.FromEntryID + "|" + request.Type + "|" + request.ToEntryID
	f.relationCalls[key]++
	if f.mutationErr != nil {
		return CreateRelationResult{}, f.mutationErr
	}
	return f.memoryPBTransport.CreateEntryRelation(ctx, request)
}

func approvedFixture(t *testing.T, now time.Time, maximumAttempts int) ApprovedBatch {
	t.Helper()
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	preflight := testPreflight(outbox, profile)
	privacyFingerprint := DeliveryPrivacyFingerprint(outbox)
	batchFingerprint := DeliveryBatchFingerprint(outbox, preflight, privacyFingerprint)
	operations := OrderedDeliveryOperationFingerprints(outbox)
	if maximumAttempts == 0 {
		maximumAttempts = len(operations) * 2
	}
	expiresAt := now.Add(time.Hour).UTC().Format(time.RFC3339Nano)
	evidence := SealHumanInitiationEvidence(HumanInitiationEvidence{
		SchemaVersion: DeliveryApprovalHumanSchemaForTest(), SessionFingerprint: hashText("session"), ReviewNonceFingerprint: hashText("nonce"), PreviewEvidenceFingerprint: hashText("preview"), BatchFingerprint: batchFingerprint, DestinationWorkspaceID: profile.Workspace.ExpectedID, DestinationKeyID: profile.Credential.ExpectedKeyID, OrderedOperationFingerprints: append([]string{}, operations...), MaximumDestinationWrites: len(operations), MaximumMutationAttempts: maximumAttempts, IssuedAt: now.Add(-time.Minute).UTC().Format(time.RFC3339Nano), ExpiresAt: expiresAt, HumanGesture: true, ServerDerived: true,
	})
	approval := SealDeliveryApproval(DeliveryApproval{
		SchemaVersion: DeliveryApprovalSchema, BatchFingerprint: batchFingerprint, OutboxFingerprint: outbox.Fingerprint, PreflightFingerprint: preflight.Fingerprint, PrivacyFingerprint: privacyFingerprint, DestinationWorkspaceID: profile.Workspace.ExpectedID, DestinationKeyID: profile.Credential.ExpectedKeyID, OrderedOperationFingerprints: append([]string{}, operations...), MaximumDestinationWrites: len(operations), MaximumMutationAttempts: maximumAttempts, HumanInitiationEvidenceFingerprint: evidence.Fingerprint, ApprovedAt: now.UTC().Format(time.RFC3339Nano), ExpiresAt: expiresAt,
	})
	return ApprovedBatch{BatchFingerprint: batchFingerprint, Outbox: outbox, Profile: profile, Preflight: preflight, PrivacyFingerprint: privacyFingerprint, Approval: approval, HumanInitiationEvidence: evidence}
}

// Kept as a function so fixture construction cannot accidentally use the
// approval schema for human evidence.
func DeliveryApprovalHumanSchemaForTest() string { return HumanInitiationEvidenceSchema }

func approvedVerifier(batch ApprovedBatch) *oneTimeHumanVerifier {
	return &oneTimeHumanVerifier{expected: batch.HumanInitiationEvidence.Fingerprint, used: map[string]bool{}}
}

func TestDeliverApprovedSealsV02AuthorityAndReplaysWithoutMutation(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	batch := approvedFixture(t, now, 0)
	verifier := approvedVerifier(batch)
	transport := newApprovedFakeTransport(batch.Profile)
	dir := t.TempDir()
	receipt, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{Now: func() time.Time { return now }, HumanInitiationVerifier: verifier})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "completed" || receipt.AcknowledgedOperations != len(batch.Outbox.Operations) || receipt.UniqueWriteReservations != len(batch.Outbox.Operations) || receipt.MutationAttempts != len(batch.Outbox.Operations) || verifier.calls != 1 {
		t.Fatalf("unexpected approved receipt: %+v verifier=%d", receipt, verifier.calls)
	}
	beforeEntries, beforeRelations := totalApprovedMutationCalls(transport)
	replay, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{Now: func() time.Time { return now.Add(time.Minute) }})
	if err != nil {
		t.Fatal(err)
	}
	afterEntries, afterRelations := totalApprovedMutationCalls(transport)
	if replay.Status != "completed" || beforeEntries != afterEntries || beforeRelations != afterRelations || replay.MutationAttempts != receipt.MutationAttempts {
		t.Fatalf("replay mutated destination: before=%d/%d after=%d/%d receipt=%+v", beforeEntries, beforeRelations, afterEntries, afterRelations, replay)
	}
	readback, err := ReadApprovedDeliveryReceipt(dir)
	if err != nil || !canonicalEqual(readback, replay) {
		t.Fatalf("sealed receipt did not read back exactly: %+v err=%v", readback, err)
	}
	if _, err := CancelApproved(context.Background(), ApprovalRef{ApprovalFingerprint: batch.Approval.Fingerprint, BatchFingerprint: batch.BatchFingerprint}, dir, func() time.Time { return now.Add(2 * time.Minute) }); err != nil {
		t.Fatal(err)
	}
	completedAfterCancel, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{Now: func() time.Time { return now.Add(3 * time.Minute) }})
	if err != nil || completedAfterCancel.Status != "completed" {
		t.Fatalf("post-completion cancellation rewrote the completed outcome: %+v err=%v", completedAfterCancel, err)
	}
	legacy, err := loadDeliveryHistory(dir, batch.Outbox.Fingerprint, hashValue(batch.Profile))
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.Runs) != 0 {
		t.Fatalf("v0.2 delivery contaminated legacy history: %+v", legacy)
	}
	history, err := ReadApprovedDeliveryHistory(dir)
	if err != nil || len(history.Runs) != 3 || history.Runs[0].SchemaVersion != ApprovedDeliveryRunSchema {
		t.Fatalf("unexpected v0.2 history: %+v err=%v", history, err)
	}
}

func TestAuthoritySealedBeforeStateCrashRecoversReadOnly(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	batch := approvedFixture(t, now, 0)
	dir := t.TempDir()
	transport := newApprovedFakeTransport(batch.Profile)
	crash := errors.New("crash after immutable authority")
	if _, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{
		Now: func() time.Time { return now }, HumanInitiationVerifier: approvedVerifier(batch),
		afterAuthoritySealed: func() error { return crash },
	}); !errors.Is(err, crash) {
		t.Fatalf("expected simulated crash, got %v", err)
	}
	entries, relations := totalApprovedMutationCalls(transport)
	if entries != 0 || relations != 0 {
		t.Fatal("mutation occurred before state recovery")
	}
	receipt, err := ReconcileApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{Now: func() time.Time { return now.Add(time.Second) }})
	if err != nil {
		t.Fatalf("read-only recovery failed: %v", err)
	}
	entries, relations = totalApprovedMutationCalls(transport)
	if receipt.Fingerprint == "" || entries != 0 || relations != 0 {
		t.Fatal("read-only recovery did not preserve a zero-mutation authority state")
	}
	if _, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{
		Now: func() time.Time { return now.Add(2 * time.Second) }, HumanInitiationVerifier: approvedVerifier(batch),
	}); err != nil {
		t.Fatalf("recovered authority could not continue: %v", err)
	}
}

func TestDeliverApprovedRejectsDriftExpiryActorInjectionAndNonceReplay(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	batch := approvedFixture(t, now, 0)
	transport := newApprovedFakeTransport(batch.Profile)
	drifted := batch
	drifted.Approval.PreflightFingerprint = hashText("drift")
	drifted.Approval = SealDeliveryApproval(drifted.Approval)
	if _, err := DeliverApproved(context.Background(), drifted, transport, t.TempDir(), ApprovedDeliveryOptions{Now: func() time.Time { return now }, HumanInitiationVerifier: approvedVerifier(drifted)}); err == nil {
		t.Fatal("approval/preflight drift was accepted")
	}
	overBudget := batch
	overBudget.HumanInitiationEvidence.MaximumDestinationWrites++
	overBudget.HumanInitiationEvidence = SealHumanInitiationEvidence(overBudget.HumanInitiationEvidence)
	overBudget.Approval.MaximumDestinationWrites++
	overBudget.Approval.HumanInitiationEvidenceFingerprint = overBudget.HumanInitiationEvidence.Fingerprint
	overBudget.Approval = SealDeliveryApproval(overBudget.Approval)
	if _, err := DeliverApproved(context.Background(), overBudget, transport, t.TempDir(), ApprovedDeliveryOptions{Now: func() time.Time { return now }, HumanInitiationVerifier: approvedVerifier(overBudget)}); err == nil {
		t.Fatal("destination-write budget larger than the exact batch was accepted")
	}
	if _, err := DeliverApproved(context.Background(), batch, transport, t.TempDir(), ApprovedDeliveryOptions{Now: func() time.Time { return now.Add(2 * time.Hour) }, HumanInitiationVerifier: approvedVerifier(batch)}); !errors.Is(err, ErrApprovalExpired) {
		t.Fatalf("expired approval was accepted: %v", err)
	}
	encoded, _ := json.Marshal(batch.Approval)
	var injected map[string]any
	_ = json.Unmarshal(encoded, &injected)
	injected["actor"] = "agent-forged"
	encoded, _ = json.Marshal(injected)
	if _, err := DecodeDeliveryApproval(encoded); err == nil {
		t.Fatal("actor injection was accepted by strict approval decoder")
	}
	verifier := approvedVerifier(batch)
	crash := errors.New("synthetic crash")
	if _, err := DeliverApproved(context.Background(), batch, transport, t.TempDir(), ApprovedDeliveryOptions{Now: func() time.Time { return now }, HumanInitiationVerifier: verifier, afterAttemptReserved: func(ApprovedMutationAttempt) error { return crash }}); !errors.Is(err, crash) {
		t.Fatalf("expected synthetic crash, got %v", err)
	}
	if _, err := DeliverApproved(context.Background(), batch, transport, t.TempDir(), ApprovedDeliveryOptions{Now: func() time.Time { return now }, HumanInitiationVerifier: verifier}); err == nil || err.Error() != "human initiation rejected" {
		t.Fatalf("replayed review nonce was accepted: %v", err)
	}
}

func TestApprovedAttemptIsDurableBeforeSendAndCrashReconcilesWithoutDuplicate(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	batch := approvedFixture(t, now, 0)
	transport := newApprovedFakeTransport(batch.Profile)
	crash := errors.New("synthetic crash after reservation")
	dir := t.TempDir()
	if _, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{Now: func() time.Time { return now }, HumanInitiationVerifier: approvedVerifier(batch), afterAttemptReserved: func(ApprovedMutationAttempt) error { return crash }}); !errors.Is(err, crash) {
		t.Fatalf("expected reservation crash, got %v", err)
	}
	state, err := ReadApprovedDeliveryState(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, relations := totalApprovedMutationCalls(transport)
	if state.MutationAttempts != 1 || state.Operations[0].Attempts[0].Outcome != "reserved" || entries+relations != 0 {
		t.Fatalf("attempt was not durably reserved before send: state=%+v calls=%d", state, entries+relations)
	}

	batch2 := approvedFixture(t, now, 0)
	transport2 := newApprovedFakeTransport(batch2.Profile)
	dir2 := t.TempDir()
	crashAfterSend := errors.New("synthetic crash after mutation")
	if _, err := DeliverApproved(context.Background(), batch2, transport2, dir2, ApprovedDeliveryOptions{Now: func() time.Time { return now }, HumanInitiationVerifier: approvedVerifier(batch2), afterMutation: func(ApprovedMutationAttempt) error { return crashAfterSend }}); !errors.Is(err, crashAfterSend) {
		t.Fatalf("expected post-send crash, got %v", err)
	}
	firstEntryID := batch2.Outbox.Operations[0].Entry.EntryID
	if transport2.entryCalls[firstEntryID] != 1 {
		t.Fatalf("first mutation did not occur exactly once: %v", transport2.entryCalls)
	}
	receipt, err := DeliverApproved(context.Background(), batch2, transport2, dir2, ApprovedDeliveryOptions{
		Now: func() time.Time { return now.Add(time.Minute) }, HumanInitiationVerifier: approvedVerifier(batch2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "completed" || transport2.entryCalls[firstEntryID] != 1 {
		t.Fatalf("crash recovery duplicated the mutation: receipt=%+v calls=%v", receipt, transport2.entryCalls)
	}
	history, err := ReadApprovedDeliveryHistory(dir2)
	if err != nil || len(history.Runs) != 2 || history.Runs[0].Outcome != "interrupted" {
		t.Fatalf("crash was not preserved in v0.2 history: %+v err=%v", history, err)
	}
}

func TestApprovedAttemptBudgetAndCancellationOrdering(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	batch := approvedFixture(t, now, 0)
	batch = resealApprovedAttempts(batch, len(batch.Outbox.Operations))
	transport := newApprovedFakeTransport(batch.Profile)
	transport.mutationErr = &TransportError{Category: "validation_failed"}
	dir := t.TempDir()
	for attempt := 0; attempt < batch.Approval.MaximumMutationAttempts; attempt++ {
		options := ApprovedDeliveryOptions{
			Now:                     func() time.Time { return now.Add(time.Duration(attempt) * time.Minute) },
			HumanInitiationVerifier: approvedVerifier(batch),
		}
		if _, err := DeliverApproved(context.Background(), batch, transport, dir, options); err == nil {
			t.Fatal("synthetic rejected mutation unexpectedly succeeded")
		}
	}
	if _, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{
		Now: func() time.Time { return now.Add(30 * time.Minute) }, HumanInitiationVerifier: approvedVerifier(batch),
	}); !errors.Is(err, ErrAttemptBudgetExhausted) {
		t.Fatalf("attempt budget was not enforced: %v", err)
	}
	state, _ := ReadApprovedDeliveryState(dir)
	if state.MutationAttempts != batch.Approval.MaximumMutationAttempts {
		t.Fatalf("attempt budget accounting drifted: %+v", state)
	}

	cancelBatch := approvedFixture(t, now, 0)
	cancelTransport := newApprovedFakeTransport(cancelBatch.Profile)
	cancelDir := t.TempDir()
	ref := ApprovalRef{ApprovalFingerprint: cancelBatch.Approval.Fingerprint, BatchFingerprint: cancelBatch.BatchFingerprint}
	first, err := CancelApproved(context.Background(), ref, cancelDir, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	second, err := CancelApproved(context.Background(), ref, cancelDir, func() time.Time { return now.Add(time.Minute) })
	if err != nil || second.Fingerprint != first.Fingerprint {
		t.Fatalf("cancellation was not idempotent: first=%+v second=%+v err=%v", first, second, err)
	}
	if _, err := DeliverApproved(context.Background(), cancelBatch, cancelTransport, cancelDir, ApprovedDeliveryOptions{Now: func() time.Time { return now }, HumanInitiationVerifier: approvedVerifier(cancelBatch)}); !errors.Is(err, ErrApprovedDeliveryCancelled) {
		t.Fatalf("cancel-before-reservation did not block send: %v", err)
	}
	entries, relations := totalApprovedMutationCalls(cancelTransport)
	if entries+relations != 0 {
		t.Fatalf("cancelled batch mutated destination: %d", entries+relations)
	}
}

func TestExpiredResumeCannotReserveOrSendAnotherMutation(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	batch := approvedFixture(t, now, 0)
	transport := newApprovedFakeTransport(batch.Profile)
	transport.mutationErr = &TransportError{Category: "validation_failed"}
	dir := t.TempDir()
	if _, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{
		Now: func() time.Time { return now }, HumanInitiationVerifier: approvedVerifier(batch),
	}); err == nil {
		t.Fatal("rejected first mutation unexpectedly succeeded")
	}
	beforeEntries, beforeRelations := totalApprovedMutationCalls(transport)
	if beforeEntries+beforeRelations != 1 {
		t.Fatalf("fixture did not consume exactly one mutation attempt: %d", beforeEntries+beforeRelations)
	}
	if _, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{
		Now: func() time.Time { return now.Add(2 * time.Hour) }, HumanInitiationVerifier: approvedVerifier(batch),
	}); !errors.Is(err, ErrApprovalExpired) {
		t.Fatalf("expired resume was not blocked: %v", err)
	}
	afterEntries, afterRelations := totalApprovedMutationCalls(transport)
	state, err := ReadApprovedDeliveryState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if beforeEntries != afterEntries || beforeRelations != afterRelations || state.MutationAttempts != 1 {
		t.Fatalf("expired resume reached a mutation boundary: before=%d/%d after=%d/%d state=%+v", beforeEntries, beforeRelations, afterEntries, afterRelations, state)
	}
}

func TestCancellationDurableImmediatelyBeforeAttemptOrderingPreventsReservation(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	batch := approvedFixture(t, now, 0)
	transport := newApprovedFakeTransport(batch.Profile)
	dir := t.TempDir()
	ref := ApprovalRef{ApprovalFingerprint: batch.Approval.Fingerprint, BatchFingerprint: batch.BatchFingerprint}
	called := false
	_, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{
		Now: func() time.Time { return now }, HumanInitiationVerifier: approvedVerifier(batch),
		beforeAttemptOrdering: func() error {
			called = true
			_, cancelErr := CancelApproved(context.Background(), ref, dir, func() time.Time { return now.Add(time.Nanosecond) })
			return cancelErr
		},
	})
	if !errors.Is(err, ErrApprovedDeliveryCancelled) || !called {
		t.Fatalf("cancel-first ordering did not stop delivery: called=%v err=%v", called, err)
	}
	entries, relations := totalApprovedMutationCalls(transport)
	state, stateErr := ReadApprovedDeliveryState(dir)
	if stateErr != nil {
		t.Fatal(stateErr)
	}
	if entries+relations != 0 || state.MutationAttempts != 0 || state.UniqueWriteReservations != 0 {
		t.Fatalf("durable cancellation allowed a reservation or send: calls=%d state=%+v", entries+relations, state)
	}
}

func TestCancellationAfterReservedMutationAllowsOnlyReconciliation(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	batch := approvedFixture(t, now, 0)
	transport := newApprovedFakeTransport(batch.Profile)
	dir := t.TempDir()
	crash := errors.New("crash after remote commit")
	if _, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{Now: func() time.Time { return now }, HumanInitiationVerifier: approvedVerifier(batch), afterMutation: func(ApprovedMutationAttempt) error { return crash }}); !errors.Is(err, crash) {
		t.Fatalf("expected synthetic crash, got %v", err)
	}
	beforeEntries, beforeRelations := totalApprovedMutationCalls(transport)
	ref := ApprovalRef{ApprovalFingerprint: batch.Approval.Fingerprint, BatchFingerprint: batch.BatchFingerprint}
	if _, err := CancelApproved(context.Background(), ref, dir, func() time.Time { return now.Add(time.Second) }); err != nil {
		t.Fatal(err)
	}
	receipt, err := ReconcileApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{Now: func() time.Time { return now.Add(time.Minute) }})
	if !errors.Is(err, ErrApprovedDeliveryCancelled) || receipt.Status != "cancelled" || receipt.AcknowledgedOperations != 1 {
		t.Fatalf("cancelled ambiguous attempt was not reconciled: receipt=%+v err=%v", receipt, err)
	}
	afterEntries, afterRelations := totalApprovedMutationCalls(transport)
	if beforeEntries != afterEntries || beforeRelations != afterRelations {
		t.Fatalf("cancellation allowed a new send: before=%d/%d after=%d/%d", beforeEntries, beforeRelations, afterEntries, afterRelations)
	}
}

func TestCancellationAuthorityRemainsWritableDuringBlockedTransport(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	batch := approvedFixture(t, now, 0)
	transport := &blockingApprovedTransport{approvedFakeTransport: newApprovedFakeTransport(batch.Profile), started: make(chan struct{}, 1), release: make(chan struct{})}
	dir := t.TempDir()
	done := make(chan error, 1)
	go func() {
		_, err := DeliverApproved(context.Background(), batch, transport, dir, ApprovedDeliveryOptions{Now: func() time.Time { return now }, HumanInitiationVerifier: approvedVerifier(batch)})
		done <- err
	}()
	select {
	case <-transport.started:
	case <-time.After(5 * time.Second):
		t.Fatal("transport did not block")
	}
	ref := ApprovalRef{ApprovalFingerprint: batch.Approval.Fingerprint, BatchFingerprint: batch.BatchFingerprint}
	if _, err := CancelApproved(context.Background(), ref, dir, func() time.Time { return now.Add(time.Second) }); err != nil {
		t.Fatalf("cancellation was blocked by in-flight delivery: %v", err)
	}
	close(transport.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("delivery did not finish after release")
	}
	if _, err := ReadApprovedCancellation(dir); err != nil {
		t.Fatalf("durable cancellation missing: %v", err)
	}
}

func resealApprovedAttempts(batch ApprovedBatch, maximumAttempts int) ApprovedBatch {
	batch.HumanInitiationEvidence.MaximumMutationAttempts = maximumAttempts
	batch.HumanInitiationEvidence = SealHumanInitiationEvidence(batch.HumanInitiationEvidence)
	batch.Approval.MaximumMutationAttempts = maximumAttempts
	batch.Approval.HumanInitiationEvidenceFingerprint = batch.HumanInitiationEvidence.Fingerprint
	batch.Approval = SealDeliveryApproval(batch.Approval)
	return batch
}

func totalApprovedMutationCalls(transport *approvedFakeTransport) (int, int) {
	entries, relations := 0, 0
	for _, calls := range transport.entryCalls {
		entries += calls
	}
	for _, calls := range transport.relationCalls {
		relations += calls
	}
	return entries, relations
}
