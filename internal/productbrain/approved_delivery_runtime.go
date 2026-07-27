package productbrain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

func DeliverApproved(ctx context.Context, batch ApprovedBatch, transport ProductBrainTransport, outDir string, options ApprovedDeliveryOptions) (ApprovedDeliveryReceipt, error) {
	return runApprovedDelivery(ctx, batch, transport, outDir, options, true)
}

// ReconcileApproved is intentionally read-only. It can acknowledge a mutation
// that was observed after a crash or ambiguous response, but it never reserves
// an attempt or invokes a mutation method.
func ReconcileApproved(ctx context.Context, batch ApprovedBatch, transport ProductBrainTransport, outDir string, options ApprovedDeliveryOptions) (ApprovedDeliveryReceipt, error) {
	return runApprovedDelivery(ctx, batch, transport, outDir, options, false)
}

func runApprovedDelivery(ctx context.Context, batch ApprovedBatch, transport ProductBrainTransport, outDir string, options ApprovedDeliveryOptions, allowMutation bool) (ApprovedDeliveryReceipt, error) {
	if transport == nil {
		return ApprovedDeliveryReceipt{}, errors.New("missing Product Brain transport")
	}
	if err := validateApprovedBatch(batch); err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	if err := privateio.PrepareDir(outDir); err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	dir := approvedAuthorityDir(outDir)
	if err := privateio.PrepareDir(dir); err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	release, err := acquireDeliveryLock(dir)
	if err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	defer release()
	now := options.Now
	if now == nil {
		now = time.Now
	}
	state, exists, err := loadApprovedState(dir)
	if err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	authorityPreexisting := exists
	if !exists {
		authority, authorityExists, authorityErr := loadApprovedAuthority(dir)
		if authorityErr != nil {
			return ApprovedDeliveryReceipt{}, authorityErr
		}
		if authorityExists {
			authorityPreexisting = true
			expected := ApprovedDeliveryAuthority{SchemaVersion: ApprovedDeliveryAuthoritySchema, Approval: batch.Approval, HumanInitiationEvidence: batch.HumanInitiationEvidence}
			expected.Fingerprint = hashValue(expected)
			if !canonicalEqual(authority, expected) {
				return ApprovedDeliveryReceipt{}, errors.New("immutable approval snapshot mismatch")
			}
		} else {
			if !allowMutation {
				return ApprovedDeliveryReceipt{}, os.ErrNotExist
			}
			if err := approvalValidAt(batch.Approval, now()); err != nil {
				return ApprovedDeliveryReceipt{}, err
			}
			if options.HumanInitiationVerifier == nil {
				return ApprovedDeliveryReceipt{}, errors.New("human initiation verifier missing")
			}
			if err := options.HumanInitiationVerifier.VerifyAndConsume(ctx, batch.HumanInitiationEvidence); err != nil {
				return ApprovedDeliveryReceipt{}, errors.New("human initiation rejected")
			}
			if err := sealApprovedAuthority(dir, batch); err != nil {
				return ApprovedDeliveryReceipt{}, err
			}
			if options.afterAuthoritySealed != nil {
				if hookErr := options.afterAuthoritySealed(); hookErr != nil {
					return ApprovedDeliveryReceipt{}, hookErr
				}
			}
		}
		// Rebuilding local state from an exact immutable authority snapshot is
		// crash recovery, not permission to perform a destination mutation.
		state, err = initializeApprovedState(dir, batch)
		if err != nil {
			return ApprovedDeliveryReceipt{}, err
		}
	} else {
		if err := validateStateBinding(state, batch); err != nil {
			return ApprovedDeliveryReceipt{}, err
		}
		if err := validateApprovedSnapshots(dir, batch, state); err != nil {
			return ApprovedDeliveryReceipt{}, err
		}
	}
	if allowMutation && authorityPreexisting && state.Status != "completed" && state.Status != "cancelled" {
		if options.HumanInitiationVerifier == nil || options.HumanInitiationVerifier.VerifyAndConsume(ctx, batch.HumanInitiationEvidence) != nil {
			return ApprovedDeliveryReceipt{}, errors.New("human resume initiation rejected")
		}
	}
	if err := refreshApprovedCancellation(dir, &state); err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	history, run, err := startApprovedRun(dir, &state, now())
	if err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	finish := func(outcome string, cause error) (ApprovedDeliveryReceipt, error) {
		if writeErr := writeApprovedState(dir, &state); writeErr != nil {
			return ApprovedDeliveryReceipt{}, writeErr
		}
		if sealErr := finishApprovedRun(dir, state, &history, &run, outcome, now()); sealErr != nil {
			return ApprovedDeliveryReceipt{}, sealErr
		}
		receipt, receiptErr := buildApprovedReceipt(dir, state, history)
		if receiptErr != nil {
			return ApprovedDeliveryReceipt{}, receiptErr
		}
		return receipt, cause
	}
	cancelled := state.CancellationFingerprint != ""
	if cancelled && state.Status == "completed" {
		return finish("completed", nil)
	}
	needsReconciliation := false
	for _, operation := range state.Operations {
		if len(operation.Attempts) > 0 && operation.State != "acknowledged" {
			needsReconciliation = true
			break
		}
	}
	if cancelled && !needsReconciliation {
		state.Status = "cancelled"
		return finish("cancelled", ErrApprovedDeliveryCancelled)
	}
	if err := validateApprovedLivePreconditions(ctx, batch, transport); err != nil {
		return finish("failed", err)
	}
	for index := range batch.Outbox.Operations {
		operation := batch.Outbox.Operations[index]
		persisted := &state.Operations[index]
		if (cancelled || !allowMutation) && len(persisted.Attempts) == 0 {
			continue
		}
		observed, reconcileErr := reconcileApprovedOperation(ctx, transport, operation, &state, persisted)
		if reconcileErr != nil {
			if persisted.State == "blocked" {
				state.Status = "blocked"
				return finish("failed", reconcileErr)
			}
			if len(persisted.Attempts) > 0 {
				persisted.State = "ambiguous"
				state.Status = "ambiguous"
			}
			return finish("ambiguous", ErrApprovedDeliveryAmbiguous)
		}
		if observed {
			if len(persisted.Attempts) > 0 {
				markApprovedAttemptAcknowledged(&state, index, persisted.ReadbackFingerprint)
			}
			if err := writeApprovedState(dir, &state); err != nil {
				return ApprovedDeliveryReceipt{}, err
			}
			continue
		}
		if cancelled {
			continue
		}
		if !allowMutation {
			if len(persisted.Attempts) > 0 {
				state.Status = "ambiguous"
				return finish("reconciled", ErrApprovedDeliveryAmbiguous)
			}
			continue
		}
		if options.beforeAttemptOrdering != nil {
			if hookErr := options.beforeAttemptOrdering(); hookErr != nil {
				return ApprovedDeliveryReceipt{}, hookErr
			}
		}
		orderingRelease, orderingErr := acquireApprovedOrderingLock(ctx, dir)
		if orderingErr != nil {
			return finish("failed", orderingErr)
		}
		if err := refreshApprovedCancellation(dir, &state); err != nil {
			orderingRelease()
			return finish("failed", err)
		}
		if state.CancellationFingerprint != "" {
			orderingRelease()
			state.Status = "cancelled"
			return finish("cancelled", ErrApprovedDeliveryCancelled)
		}
		// Approval expiry is checked immediately before every new durable
		// mutation reservation, including resumed authorities. Completed replay
		// and read-only reconciliation do not pass this boundary.
		if err := approvalValidAt(batch.Approval, now()); err != nil {
			orderingRelease()
			return finish("failed", err)
		}
		attempt, reserveErr := reserveApprovedAttempt(dir, &state, index, run.Sequence, now())
		orderingRelease()
		if reserveErr != nil {
			state.Status = "blocked"
			persisted.State = "blocked"
			return finish("failed", reserveErr)
		}
		// The durable state write in reserveApprovedAttempt is the authority
		// boundary. Any failure after this point consumes the attempt.
		if options.afterAttemptReserved != nil {
			if hookErr := options.afterAttemptReserved(attempt); hookErr != nil {
				return ApprovedDeliveryReceipt{}, hookErr
			}
		}
		mutationErr := mutateApprovedOperation(ctx, transport, operation)
		if options.afterMutation != nil {
			if hookErr := options.afterMutation(attempt); hookErr != nil {
				return ApprovedDeliveryReceipt{}, hookErr
			}
		}
		recordApprovedMutationResult(&state, index, mutationErr)
		if err := writeApprovedState(dir, &state); err != nil {
			return ApprovedDeliveryReceipt{}, err
		}
		observed, reconcileErr = reconcileApprovedOperation(ctx, transport, operation, &state, persisted)
		if reconcileErr != nil {
			if persisted.State == "blocked" {
				state.Status = "blocked"
				return finish("failed", reconcileErr)
			}
			markApprovedAttemptAmbiguous(&state, index)
			state.Status = "ambiguous"
			return finish("ambiguous", ErrApprovedDeliveryAmbiguous)
		}
		if observed {
			markApprovedAttemptAcknowledged(&state, index, persisted.ReadbackFingerprint)
			if err := writeApprovedState(dir, &state); err != nil {
				return ApprovedDeliveryReceipt{}, err
			}
			continue
		}
		markApprovedAttemptNotObserved(&state, index)
		if mutationErr != nil {
			state.Status = "approved"
			return finish("failed", mutationErr)
		}
		state.Status = "ambiguous"
		return finish("ambiguous", ErrApprovedDeliveryAmbiguous)
	}
	allAcknowledged := true
	for _, operation := range state.Operations {
		if operation.State != "acknowledged" {
			allAcknowledged = false
			break
		}
	}
	if allAcknowledged {
		if cancelled {
			state.Status = "cancelled"
			return finish("cancelled", ErrApprovedDeliveryCancelled)
		}
		state.Status = "completed"
		outcome := "completed"
		if !allowMutation {
			outcome = "reconciled"
		}
		return finish(outcome, nil)
	}
	if cancelled {
		state.Status = "cancelled"
		return finish("cancelled", ErrApprovedDeliveryCancelled)
	}
	state.Status = "approved"
	return finish("reconciled", nil)
}

func refreshApprovedCancellation(dir string, state *ApprovedDeliveryState) error {
	receipt, exists, err := loadCancellation(dir)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if receipt.ApprovalFingerprint != state.ApprovalFingerprint || receipt.BatchFingerprint != state.BatchFingerprint {
		return errors.New("cancellation binding mismatch")
	}
	state.CancellationFingerprint = receipt.Fingerprint
	if state.Status != "completed" {
		state.Status = "cancelled"
	}
	return nil
}

func validateApprovedLivePreconditions(ctx context.Context, batch ApprovedBatch, transport ProductBrainTransport) error {
	capability, err := transport.ResolveWorkspace(ctx)
	if err != nil {
		return err
	}
	if err := checkCapability(capability, batch.Profile); err != nil {
		return err
	}
	scanner, ok := transport.(RuntimeSecretScanner)
	if !ok || len(scanner.RuntimeSecretFindings(batch.Outbox)) > 0 {
		return errors.New("unsafe_outbound_value")
	}
	return validateLiveCollectionContracts(ctx, transport, batch.Outbox, batch.Preflight)
}

func reserveApprovedAttempt(dir string, state *ApprovedDeliveryState, operationIndex, runSequence int, now time.Time) (ApprovedMutationAttempt, error) {
	operation := &state.Operations[operationIndex]
	if !operation.UniqueWriteReserved {
		if state.UniqueWriteReservations >= state.MaximumDestinationWrites {
			return ApprovedMutationAttempt{}, ErrWriteBudgetExhausted
		}
		operation.UniqueWriteReserved = true
		state.UniqueWriteReservations++
	}
	if state.MutationAttempts >= state.MaximumMutationAttempts {
		return ApprovedMutationAttempt{}, ErrAttemptBudgetExhausted
	}
	number := state.MutationAttempts + 1
	attempt := ApprovedMutationAttempt{AttemptNumber: number, AttemptID: hashText(fmt.Sprintf("%s|%s|%d", state.ApprovalFingerprint, operation.OperationID, number)), RunSequence: runSequence, OperationID: operation.OperationID, OperationFingerprint: operation.OperationFingerprint, ReservedAt: now.UTC().Format(time.RFC3339Nano), Outcome: "reserved"}
	operation.Attempts = append(operation.Attempts, attempt)
	operation.State = "reserved"
	state.MutationAttempts++
	state.Status = "delivering"
	if err := writeApprovedState(dir, state); err != nil {
		return ApprovedMutationAttempt{}, err
	}
	return attempt, nil
}

func mutateApprovedOperation(ctx context.Context, transport ProductBrainTransport, operation OutboxOperation) error {
	switch operation.Kind {
	case "entry":
		expected := operation.Entry
		_, err := transport.CreateEntry(ctx, CreateEntryRequest{CollectionSlug: expected.CollectionSlug, EntryID: expected.EntryID, Name: expected.Name, Data: expected.Data, SourceRef: expected.SourceRef, SourceExcerpt: expected.SourceExcerpt, CreatedBy: expected.CreatedBy, ForceDraft: expected.ForceDraft})
		return err
	case "relation":
		expected := operation.Relation
		_, err := transport.CreateEntryRelation(ctx, CreateRelationRequest{FromEntryID: expected.FromEntryID, ToEntryID: expected.ToEntryID, Type: expected.Type, Metadata: expected.Metadata, IfMissing: expected.IfMissing})
		return err
	default:
		return errors.New("invalid approved delivery operation")
	}
}

func recordApprovedMutationResult(state *ApprovedDeliveryState, operationIndex int, mutationErr error) {
	attempt := latestApprovedAttempt(state, operationIndex)
	if attempt == nil {
		return
	}
	if mutationErr == nil {
		attempt.ResponseReceived = true
		attempt.MayHaveCommitted = true
		attempt.Outcome = "ambiguous"
		state.Operations[operationIndex].State = "ambiguous"
		return
	}
	var transportErr *TransportError
	if errors.As(mutationErr, &transportErr) {
		attempt.MayHaveCommitted = transportErr.MayHaveCommitted
	}
	if attempt.MayHaveCommitted {
		attempt.Outcome = "ambiguous"
		state.Operations[operationIndex].State = "ambiguous"
	} else {
		attempt.Outcome = "rejected"
		state.Operations[operationIndex].State = "not_observed"
	}
}

func latestApprovedAttempt(state *ApprovedDeliveryState, operationIndex int) *ApprovedMutationAttempt {
	attempts := state.Operations[operationIndex].Attempts
	if len(attempts) == 0 {
		return nil
	}
	return &state.Operations[operationIndex].Attempts[len(attempts)-1]
}

func markApprovedAttemptAcknowledged(state *ApprovedDeliveryState, operationIndex int, readbackFingerprint string) {
	if attempt := latestApprovedAttempt(state, operationIndex); attempt != nil {
		attempt.Outcome = "acknowledged"
		attempt.MayHaveCommitted = true
		attempt.ReadbackFingerprint = readbackFingerprint
	}
}

func markApprovedAttemptAmbiguous(state *ApprovedDeliveryState, operationIndex int) {
	if attempt := latestApprovedAttempt(state, operationIndex); attempt != nil {
		attempt.Outcome = "ambiguous"
		attempt.MayHaveCommitted = true
	}
	state.Operations[operationIndex].State = "ambiguous"
}

func markApprovedAttemptNotObserved(state *ApprovedDeliveryState, operationIndex int) {
	if attempt := latestApprovedAttempt(state, operationIndex); attempt != nil {
		attempt.Outcome = "not_observed"
	}
	state.Operations[operationIndex].State = "not_observed"
}

func reconcileApprovedOperation(ctx context.Context, transport ProductBrainTransport, operation OutboxOperation, state *ApprovedDeliveryState, persisted *ApprovedOperationState) (bool, error) {
	switch operation.Kind {
	case "entry":
		found, err := transport.GetEntry(ctx, operation.Entry.EntryID)
		if err != nil {
			return false, err
		}
		if found.Found {
			if err := compareEntry(found, *operation.Entry); err != nil {
				persisted.State = "blocked"
				persisted.SafeCategory = "readback_mismatch"
				state.Status = "blocked"
				return false, err
			}
			persisted.State = "acknowledged"
			persisted.EntryDocID = found.DocID
			persisted.RemoteObjectID = found.EntryID
			persisted.ReadbackFingerprint = hashValue(found)
			persisted.SafeCategory = ""
			return true, nil
		}
		matches, err := transport.SearchEntries(ctx, operation.Entry.Name, operation.Entry.CollectionSlug)
		if err != nil {
			return false, err
		}
		for _, match := range matches {
			if match.EntryID != operation.Entry.EntryID {
				persisted.State = "blocked"
				persisted.SafeCategory = "destination_name_conflict"
				state.Status = "blocked"
				return false, errors.New("destination_name_conflict")
			}
		}
	case "relation":
		from := approvedEntryStateByEntryID(state, operation.Relation.FromEntryID)
		to := approvedEntryStateByEntryID(state, operation.Relation.ToEntryID)
		if from == nil || to == nil || from.State != "acknowledged" || to.State != "acknowledged" {
			return false, errors.New("dependency_not_acknowledged")
		}
		relations, err := transport.ListEntryRelations(ctx, operation.Relation.FromEntryID)
		if err != nil {
			return false, err
		}
		matched, relationID, conflict := findRelation(relations, from.EntryDocID, to.EntryDocID, *operation.Relation)
		if conflict {
			persisted.State = "blocked"
			persisted.SafeCategory = "readback_mismatch"
			state.Status = "blocked"
			return false, errors.New("readback_mismatch")
		}
		if matched {
			persisted.State = "acknowledged"
			persisted.RemoteObjectID = relationID
			persisted.ReadbackFingerprint = hashValue(map[string]any{"relation_id": relationID, "identity": operation.Relation.RelationIdentity, "metadata": operation.Relation.Metadata})
			persisted.SafeCategory = ""
			return true, nil
		}
	default:
		return false, errors.New("invalid approved delivery operation")
	}
	if len(persisted.Attempts) > 0 {
		persisted.State = "not_observed"
	} else {
		persisted.State = "pending"
	}
	return false, nil
}

func approvedEntryStateByEntryID(state *ApprovedDeliveryState, entryID string) *ApprovedOperationState {
	for index := range state.Operations {
		operation := &state.Operations[index]
		if operation.Kind == "entry" && operation.EntryID == entryID {
			return operation
		}
	}
	return nil
}
