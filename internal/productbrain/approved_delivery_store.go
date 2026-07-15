package productbrain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	maximumApprovedStateBytes        int64 = 64 << 20
	maximumApprovedAuthorityBytes    int64 = 64 << 20
	maximumApprovedCancellationBytes int64 = 64 << 10
	maximumApprovedHistoryBytes      int64 = 64 << 20
	maximumApprovedRunBytes          int64 = 16 << 20
	maximumApprovedReceiptBytes      int64 = 16 << 20
)

func approvedAuthorityDir(outDir string) string {
	return filepath.Join(outDir, approvedDeliveryAuthorityDir)
}

func approvedStatePath(dir string) string        { return filepath.Join(dir, "state.json") }
func approvedHistoryPath(dir string) string      { return filepath.Join(dir, "history.json") }
func approvedActivePath(dir string) string       { return filepath.Join(dir, ".active.json") }
func approvedAuthorityPath(dir string) string    { return filepath.Join(dir, "authority.json") }
func approvedCancellationPath(dir string) string { return filepath.Join(dir, "cancellation.json") }
func approvedReceiptPath(dir string) string      { return filepath.Join(dir, "receipt.json") }

func writeApprovedState(dir string, state *ApprovedDeliveryState) error {
	state.Fingerprint = hashValue(*state)
	return privateio.WriteJSON(approvedStatePath(dir), *state)
}

func loadApprovedState(dir string) (ApprovedDeliveryState, bool, error) {
	var state ApprovedDeliveryState
	if err := privateio.ReadJSONStrictBounded(dir, approvedStatePath(dir), maximumApprovedStateBytes, &state); err != nil {
		if os.IsNotExist(err) {
			return ApprovedDeliveryState{}, false, nil
		}
		return ApprovedDeliveryState{}, false, err
	}
	if err := validateApprovedState(state); err != nil {
		return ApprovedDeliveryState{}, false, err
	}
	return state, true, nil
}

func validateApprovedState(state ApprovedDeliveryState) error {
	if state.SchemaVersion != ApprovedDeliveryStateSchema || state.Fingerprint != hashValue(state) || len(state.Operations) == 0 || len(state.OrderedOperationFingerprints) != len(state.Operations) || state.MaximumDestinationWrites != len(state.Operations) || state.MaximumMutationAttempts < len(state.Operations) || state.MaximumMutationAttempts > len(state.Operations)*maximumApprovedAttemptsPerOperation || state.UniqueWriteReservations < 0 || state.MutationAttempts < 0 || state.UniqueWriteReservations > state.MaximumDestinationWrites || state.MutationAttempts > state.MaximumMutationAttempts {
		return errors.New("approved delivery state mismatch")
	}
	switch state.Status {
	case "approved", "delivering", "ambiguous", "completed", "cancelled", "blocked":
	default:
		return errors.New("approved delivery state mismatch")
	}
	seenOperations := map[string]bool{}
	attemptNumbers := map[int]bool{}
	writeReservations, attempts := 0, 0
	for index, operation := range state.Operations {
		if operation.OperationID == "" || seenOperations[operation.OperationID] || operation.OperationFingerprint != state.OrderedOperationFingerprints[index] || operation.Kind != "entry" && operation.Kind != "relation" || operation.Kind == "entry" && operation.EntryID == "" || operation.Kind == "relation" && operation.EntryID != "" {
			return errors.New("approved delivery operation mismatch")
		}
		seenOperations[operation.OperationID] = true
		if operation.UniqueWriteReserved {
			writeReservations++
		}
		switch operation.State {
		case "pending", "reserved", "not_observed", "ambiguous", "acknowledged", "blocked":
		default:
			return errors.New("approved delivery operation mismatch")
		}
		if operation.State == "acknowledged" && (operation.ReadbackFingerprint == "" || operation.RemoteObjectID == "") {
			return errors.New("approved delivery operation mismatch")
		}
		for _, attempt := range operation.Attempts {
			attempts++
			if attempt.AttemptNumber <= 0 || attempt.AttemptID == "" || attempt.RunSequence <= 0 || attempt.OperationID != operation.OperationID || attempt.OperationFingerprint != operation.OperationFingerprint || attemptNumbers[attempt.AttemptNumber] {
				return errors.New("approved delivery attempt mismatch")
			}
			attemptNumbers[attempt.AttemptNumber] = true
			expectedID := hashText(fmt.Sprintf("%s|%s|%d", state.ApprovalFingerprint, operation.OperationID, attempt.AttemptNumber))
			if attempt.AttemptID != expectedID {
				return errors.New("approved delivery attempt mismatch")
			}
			switch attempt.Outcome {
			case "reserved", "not_observed", "ambiguous", "acknowledged", "rejected":
			default:
				return errors.New("approved delivery attempt mismatch")
			}
		}
	}
	for number := 1; number <= attempts; number++ {
		if !attemptNumbers[number] {
			return errors.New("approved delivery attempt sequence mismatch")
		}
	}
	if writeReservations != state.UniqueWriteReservations || attempts != state.MutationAttempts {
		return errors.New("approved delivery budget mismatch")
	}
	return nil
}

func initializeApprovedState(dir string, batch ApprovedBatch) (ApprovedDeliveryState, error) {
	state := ApprovedDeliveryState{
		SchemaVersion:                      ApprovedDeliveryStateSchema,
		ApprovalFingerprint:                batch.Approval.Fingerprint,
		HumanInitiationEvidenceFingerprint: batch.HumanInitiationEvidence.Fingerprint,
		BatchFingerprint:                   batch.BatchFingerprint,
		OutboxFingerprint:                  batch.Outbox.Fingerprint,
		ProfileFingerprint:                 hashValue(batch.Profile),
		PreflightFingerprint:               batch.Preflight.Fingerprint,
		PrivacyFingerprint:                 batch.PrivacyFingerprint,
		DestinationWorkspaceID:             batch.Profile.Workspace.ExpectedID,
		DestinationKeyID:                   batch.Profile.Credential.ExpectedKeyID,
		OrderedOperationFingerprints:       append([]string{}, batch.Approval.OrderedOperationFingerprints...),
		MaximumDestinationWrites:           batch.Approval.MaximumDestinationWrites,
		MaximumMutationAttempts:            batch.Approval.MaximumMutationAttempts,
		Status:                             "approved",
	}
	for _, operation := range batch.Outbox.Operations {
		entryID := ""
		if operation.Entry != nil {
			entryID = operation.Entry.EntryID
		}
		state.Operations = append(state.Operations, ApprovedOperationState{OperationID: operation.OperationID, Kind: operation.Kind, OperationFingerprint: operation.PayloadFingerprint, EntryID: entryID, State: "pending", Attempts: []ApprovedMutationAttempt{}})
	}
	if cancellation, exists, err := loadCancellation(dir); err != nil {
		return ApprovedDeliveryState{}, err
	} else if exists {
		if cancellation.ApprovalFingerprint != state.ApprovalFingerprint || cancellation.BatchFingerprint != state.BatchFingerprint {
			return ApprovedDeliveryState{}, errors.New("cancellation binding mismatch")
		}
		state.CancellationFingerprint = cancellation.Fingerprint
		state.Status = "cancelled"
	}
	if err := writeApprovedState(dir, &state); err != nil {
		return ApprovedDeliveryState{}, err
	}
	return state, nil
}

func loadApprovedAuthority(dir string) (ApprovedDeliveryAuthority, bool, error) {
	var authority ApprovedDeliveryAuthority
	if err := privateio.ReadJSONStrictBounded(dir, approvedAuthorityPath(dir), maximumApprovedAuthorityBytes, &authority); err != nil {
		if os.IsNotExist(err) {
			return ApprovedDeliveryAuthority{}, false, nil
		}
		return ApprovedDeliveryAuthority{}, false, err
	}
	if authority.SchemaVersion != ApprovedDeliveryAuthoritySchema || authority.Fingerprint != hashValue(authority) || authority.Approval.Fingerprint != hashValue(authority.Approval) || authority.HumanInitiationEvidence.Fingerprint != hashValue(authority.HumanInitiationEvidence) {
		return ApprovedDeliveryAuthority{}, false, errors.New("approved delivery authority mismatch")
	}
	return authority, true, nil
}

func sealApprovedAuthority(dir string, batch ApprovedBatch) error {
	authority := ApprovedDeliveryAuthority{SchemaVersion: ApprovedDeliveryAuthoritySchema, Approval: batch.Approval, HumanInitiationEvidence: batch.HumanInitiationEvidence}
	authority.Fingerprint = hashValue(authority)
	if existing, exists, err := loadApprovedAuthority(dir); err != nil {
		return err
	} else if exists {
		if !canonicalEqual(existing, authority) {
			return errors.New("immutable approval snapshot mismatch")
		}
		return nil
	}
	return privateio.WriteJSONNoReplace(approvedAuthorityPath(dir), authority)
}

func validateApprovedSnapshots(dir string, batch ApprovedBatch, state ApprovedDeliveryState) error {
	authority, exists, err := loadApprovedAuthority(dir)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("approved delivery authority missing")
	}
	approval, evidence := authority.Approval, authority.HumanInitiationEvidence
	if approval.Fingerprint != hashValue(approval) || evidence.Fingerprint != hashValue(evidence) || !canonicalEqual(approval, batch.Approval) || !canonicalEqual(evidence, batch.HumanInitiationEvidence) || state.ApprovalFingerprint != approval.Fingerprint || state.HumanInitiationEvidenceFingerprint != evidence.Fingerprint {
		return errors.New("immutable approval snapshot mismatch")
	}
	return nil
}

func validateStateBinding(state ApprovedDeliveryState, batch ApprovedBatch) error {
	if state.ApprovalFingerprint != batch.Approval.Fingerprint || state.HumanInitiationEvidenceFingerprint != batch.HumanInitiationEvidence.Fingerprint || state.BatchFingerprint != batch.BatchFingerprint || state.OutboxFingerprint != batch.Outbox.Fingerprint || state.ProfileFingerprint != hashValue(batch.Profile) || state.PreflightFingerprint != batch.Preflight.Fingerprint || state.PrivacyFingerprint != batch.PrivacyFingerprint || state.DestinationWorkspaceID != batch.Profile.Workspace.ExpectedID || state.DestinationKeyID != batch.Profile.Credential.ExpectedKeyID || !equalStrings(state.OrderedOperationFingerprints, batch.Approval.OrderedOperationFingerprints) || state.MaximumDestinationWrites != batch.Approval.MaximumDestinationWrites || state.MaximumMutationAttempts != batch.Approval.MaximumMutationAttempts || len(state.Operations) != len(batch.Outbox.Operations) {
		return errors.New("approved delivery binding mismatch")
	}
	for index, operation := range batch.Outbox.Operations {
		persisted := state.Operations[index]
		expectedEntryID := ""
		if operation.Entry != nil {
			expectedEntryID = operation.Entry.EntryID
		}
		if persisted.OperationID != operation.OperationID || persisted.Kind != operation.Kind || persisted.OperationFingerprint != operation.PayloadFingerprint || persisted.EntryID != expectedEntryID {
			return errors.New("approved delivery operation drift")
		}
	}
	return nil
}

func loadCancellation(dir string) (CancellationReceipt, bool, error) {
	var receipt CancellationReceipt
	if err := privateio.ReadJSONStrictBounded(dir, approvedCancellationPath(dir), maximumApprovedCancellationBytes, &receipt); err != nil {
		if os.IsNotExist(err) {
			return CancellationReceipt{}, false, nil
		}
		return CancellationReceipt{}, false, err
	}
	if receipt.SchemaVersion != ApprovedDeliveryCancellationSchema || receipt.Fingerprint != hashValue(receipt) || receipt.ApprovalFingerprint == "" || receipt.BatchFingerprint == "" {
		return CancellationReceipt{}, false, errors.New("invalid approved delivery cancellation")
	}
	if _, err := time.Parse(time.RFC3339Nano, receipt.CancelledAt); err != nil {
		return CancellationReceipt{}, false, errors.New("invalid approved delivery cancellation")
	}
	return receipt, true, nil
}

func ReadApprovedCancellation(outDir string) (CancellationReceipt, error) {
	receipt, exists, err := loadCancellation(approvedAuthorityDir(outDir))
	if err != nil {
		return CancellationReceipt{}, err
	}
	if !exists {
		return CancellationReceipt{}, os.ErrNotExist
	}
	return receipt, nil
}

func CancelApproved(ctx context.Context, ref ApprovalRef, outDir string, now func() time.Time) (CancellationReceipt, error) {
	if err := ctx.Err(); err != nil {
		return CancellationReceipt{}, err
	}
	if len(ref.ApprovalFingerprint) != 64 || len(ref.BatchFingerprint) != 64 {
		return CancellationReceipt{}, errors.New("invalid approval reference")
	}
	dir := approvedAuthorityDir(outDir)
	if err := privateio.PrepareDir(dir); err != nil {
		return CancellationReceipt{}, err
	}
	release, err := acquireApprovedOrderingLock(ctx, dir)
	if err != nil {
		return CancellationReceipt{}, err
	}
	defer release()
	if existing, exists, err := loadCancellation(dir); err != nil {
		return CancellationReceipt{}, err
	} else if exists {
		if existing.ApprovalFingerprint != ref.ApprovalFingerprint || existing.BatchFingerprint != ref.BatchFingerprint {
			return CancellationReceipt{}, errors.New("cancellation binding mismatch")
		}
		return existing, nil
	}
	state, exists, err := loadApprovedState(dir)
	if err != nil {
		return CancellationReceipt{}, err
	}
	if exists && (state.ApprovalFingerprint != ref.ApprovalFingerprint || state.BatchFingerprint != ref.BatchFingerprint) {
		return CancellationReceipt{}, errors.New("cancellation binding mismatch")
	}
	if now == nil {
		now = time.Now
	}
	receipt := CancellationReceipt{SchemaVersion: ApprovedDeliveryCancellationSchema, ApprovalFingerprint: ref.ApprovalFingerprint, BatchFingerprint: ref.BatchFingerprint, CancelledAt: now().UTC().Format(time.RFC3339Nano)}
	receipt.Fingerprint = hashValue(receipt)
	if err := privateio.WriteJSONNoReplace(approvedCancellationPath(dir), receipt); err != nil {
		if existing, found, loadErr := loadCancellation(dir); loadErr == nil && found && existing.ApprovalFingerprint == ref.ApprovalFingerprint && existing.BatchFingerprint == ref.BatchFingerprint {
			return existing, nil
		}
		return CancellationReceipt{}, err
	}
	return receipt, nil
}

func loadApprovedHistory(dir, approvalFingerprint, batchFingerprint string) (ApprovedDeliveryHistory, error) {
	history := ApprovedDeliveryHistory{SchemaVersion: ApprovedDeliveryHistorySchema, ApprovalFingerprint: approvalFingerprint, BatchFingerprint: batchFingerprint, RunRefs: []string{}, Runs: []ApprovedDeliveryRun{}}
	runsDir := filepath.Join(dir, "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return history, nil
		}
		return ApprovedDeliveryHistory{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			return ApprovedDeliveryHistory{}, errors.New("approved delivery run store contains unexpected artifact")
		}
		var run ApprovedDeliveryRun
		if err := privateio.ReadJSONStrictBounded(dir, filepath.Join(runsDir, entry.Name()), maximumApprovedRunBytes, &run); err != nil {
			return ApprovedDeliveryHistory{}, err
		}
		sequence := len(history.Runs) + 1
		name := fmt.Sprintf("%06d-%s.json", sequence, run.InvocationID)
		if entry.Name() != name || run.SchemaVersion != ApprovedDeliveryRunSchema || run.Sequence != sequence || run.Fingerprint != hashValue(run) || run.ApprovalFingerprint != approvalFingerprint || run.BatchFingerprint != batchFingerprint || !validApprovedRunOutcome(run.Outcome) {
			return ApprovedDeliveryHistory{}, errors.New("approved delivery history mismatch")
		}
		history.RunRefs = append(history.RunRefs, filepath.ToSlash(filepath.Join("runs", name)))
		history.Runs = append(history.Runs, run)
	}
	if len(history.Runs) > 0 {
		history.Fingerprint = hashValue(history)
	}
	var projected ApprovedDeliveryHistory
	if err := privateio.ReadJSONStrictBounded(dir, approvedHistoryPath(dir), maximumApprovedHistoryBytes, &projected); err == nil {
		if projected.Fingerprint != hashValue(projected) || projected.ApprovalFingerprint != history.ApprovalFingerprint || projected.BatchFingerprint != history.BatchFingerprint || len(projected.Runs) > len(history.Runs) || len(projected.RunRefs) != len(projected.Runs) {
			return ApprovedDeliveryHistory{}, errors.New("approved delivery history projection mismatch")
		}
		for index := range projected.Runs {
			if projected.RunRefs[index] != history.RunRefs[index] || !canonicalEqual(projected.Runs[index], history.Runs[index]) {
				return ApprovedDeliveryHistory{}, errors.New("approved delivery history projection mismatch")
			}
		}
		if len(projected.Runs) < len(history.Runs) {
			if err := privateio.WriteJSON(approvedHistoryPath(dir), history); err != nil {
				return ApprovedDeliveryHistory{}, err
			}
		}
	} else if !os.IsNotExist(err) {
		return ApprovedDeliveryHistory{}, err
	}
	return history, nil
}

func validApprovedRunOutcome(outcome string) bool {
	switch outcome {
	case "completed", "failed", "ambiguous", "interrupted", "cancelled", "reconciled":
		return true
	default:
		return false
	}
}

func approvedAttemptIDsForRun(state ApprovedDeliveryState, sequence int) []string {
	var values []string
	for _, operation := range state.Operations {
		for _, attempt := range operation.Attempts {
			if attempt.RunSequence == sequence {
				values = append(values, attempt.AttemptID)
			}
		}
	}
	return values
}

func sealInterruptedApprovedRun(dir string, state ApprovedDeliveryState, history *ApprovedDeliveryHistory, ended time.Time) error {
	var run ApprovedDeliveryRun
	if err := privateio.ReadJSONStrictBounded(dir, approvedActivePath(dir), maximumApprovedRunBytes, &run); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if run.SchemaVersion != ApprovedDeliveryRunSchema || run.Fingerprint != hashValue(run) || run.Sequence != len(history.Runs)+1 || run.ApprovalFingerprint != state.ApprovalFingerprint || run.BatchFingerprint != state.BatchFingerprint || run.Outcome != "running" {
		if run.SchemaVersion == ApprovedDeliveryRunSchema && run.Fingerprint == hashValue(run) && run.Sequence > 0 && run.Sequence <= len(history.Runs) && run.ApprovalFingerprint == state.ApprovalFingerprint && run.BatchFingerprint == state.BatchFingerprint && run.Outcome == "running" && history.Runs[run.Sequence-1].InvocationID == run.InvocationID {
			return os.Remove(approvedActivePath(dir))
		}
		return errors.New("active approved delivery run mismatch")
	}
	run.AttemptIDs = approvedAttemptIDsForRun(state, run.Sequence)
	run.Outcome = "interrupted"
	run.EndedAt = ended.UTC().Format(time.RFC3339Nano)
	return sealApprovedRun(dir, history, run)
}

func startApprovedRun(dir string, state *ApprovedDeliveryState, now time.Time) (ApprovedDeliveryHistory, ApprovedDeliveryRun, error) {
	history, err := loadApprovedHistory(dir, state.ApprovalFingerprint, state.BatchFingerprint)
	if err != nil {
		return ApprovedDeliveryHistory{}, ApprovedDeliveryRun{}, err
	}
	if err := sealInterruptedApprovedRun(dir, *state, &history, now); err != nil {
		return ApprovedDeliveryHistory{}, ApprovedDeliveryRun{}, err
	}
	sequence := len(history.Runs) + 1
	run := ApprovedDeliveryRun{SchemaVersion: ApprovedDeliveryRunSchema, Sequence: sequence, InvocationID: hashText(fmt.Sprintf("%s|%d|%d", state.ApprovalFingerprint, sequence, now.UnixNano()))[:20], ApprovalFingerprint: state.ApprovalFingerprint, BatchFingerprint: state.BatchFingerprint, StartedAt: now.UTC().Format(time.RFC3339Nano), Outcome: "running", AttemptIDs: []string{}}
	run.Fingerprint = hashValue(run)
	if err := privateio.WriteJSON(approvedActivePath(dir), run); err != nil {
		return ApprovedDeliveryHistory{}, ApprovedDeliveryRun{}, err
	}
	state.LatestRunSequence = sequence
	if state.Status != "cancelled" && state.Status != "completed" {
		state.Status = "delivering"
	}
	if err := writeApprovedState(dir, state); err != nil {
		return ApprovedDeliveryHistory{}, ApprovedDeliveryRun{}, err
	}
	return history, run, nil
}

func sealApprovedRun(dir string, history *ApprovedDeliveryHistory, run ApprovedDeliveryRun) error {
	if run.Sequence != len(history.Runs)+1 || run.Outcome == "running" || run.ApprovalFingerprint != history.ApprovalFingerprint || run.BatchFingerprint != history.BatchFingerprint {
		return errors.New("approved delivery run sequence mismatch")
	}
	run.Fingerprint = hashValue(run)
	runsDir := filepath.Join(dir, "runs")
	if err := privateio.PrepareDir(runsDir); err != nil {
		return err
	}
	name := fmt.Sprintf("%06d-%s.json", run.Sequence, run.InvocationID)
	if err := privateio.WriteJSONNoReplace(filepath.Join(runsDir, name), run); err != nil {
		return err
	}
	history.RunRefs = append(history.RunRefs, filepath.ToSlash(filepath.Join("runs", name)))
	history.Runs = append(history.Runs, run)
	history.Fingerprint = hashValue(*history)
	if err := privateio.WriteJSON(approvedHistoryPath(dir), *history); err != nil {
		return err
	}
	if err := os.Remove(approvedActivePath(dir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func finishApprovedRun(dir string, state ApprovedDeliveryState, history *ApprovedDeliveryHistory, run *ApprovedDeliveryRun, outcome string, now time.Time) error {
	run.AttemptIDs = approvedAttemptIDsForRun(state, run.Sequence)
	run.Outcome = outcome
	run.EndedAt = now.UTC().Format(time.RFC3339Nano)
	return sealApprovedRun(dir, history, *run)
}

func buildApprovedReceipt(dir string, state ApprovedDeliveryState, history ApprovedDeliveryHistory) (ApprovedDeliveryReceipt, error) {
	receipt := approvedReceiptValue(state, history)
	if err := privateio.WriteJSON(approvedReceiptPath(dir), receipt); err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	return receipt, nil
}

func approvedReceiptValue(state ApprovedDeliveryState, history ApprovedDeliveryHistory) ApprovedDeliveryReceipt {
	receipt := ApprovedDeliveryReceipt{SchemaVersion: ApprovedDeliveryReceiptSchema, ApprovalFingerprint: state.ApprovalFingerprint, BatchFingerprint: state.BatchFingerprint, StateFingerprint: state.Fingerprint, HistoryFingerprint: history.Fingerprint, Status: state.Status, UniqueWriteReservations: state.UniqueWriteReservations, MutationAttempts: state.MutationAttempts, CancellationFingerprint: state.CancellationFingerprint}
	for _, operation := range state.Operations {
		if operation.State == "acknowledged" {
			receipt.AcknowledgedOperations++
			if strings.TrimSpace(operation.RemoteObjectID) != "" {
				receipt.RemoteObjectIDs = append(receipt.RemoteObjectIDs, operation.RemoteObjectID)
			}
		}
	}
	sort.Strings(receipt.RemoteObjectIDs)
	receipt.Fingerprint = hashValue(receipt)
	return receipt
}

func ReadApprovedDeliveryState(outDir string) (ApprovedDeliveryState, error) {
	dir := approvedAuthorityDir(outDir)
	state, exists, err := loadApprovedState(dir)
	if err != nil {
		return ApprovedDeliveryState{}, err
	}
	if !exists {
		return ApprovedDeliveryState{}, os.ErrNotExist
	}
	return state, nil
}

func ReadApprovedDeliveryHistory(outDir string) (ApprovedDeliveryHistory, error) {
	state, err := ReadApprovedDeliveryState(outDir)
	if err != nil {
		return ApprovedDeliveryHistory{}, err
	}
	return loadApprovedHistory(approvedAuthorityDir(outDir), state.ApprovalFingerprint, state.BatchFingerprint)
}

func ReadApprovedDeliveryReceipt(outDir string) (ApprovedDeliveryReceipt, error) {
	dir := approvedAuthorityDir(outDir)
	var receipt ApprovedDeliveryReceipt
	if err := privateio.ReadJSONStrictBounded(dir, approvedReceiptPath(dir), maximumApprovedReceiptBytes, &receipt); err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	if receipt.SchemaVersion != ApprovedDeliveryReceiptSchema || receipt.Fingerprint != hashValue(receipt) {
		return ApprovedDeliveryReceipt{}, errors.New("approved delivery receipt mismatch")
	}
	state, exists, err := loadApprovedState(dir)
	if err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	if !exists {
		return ApprovedDeliveryReceipt{}, errors.New("approved delivery receipt mismatch")
	}
	history, err := loadApprovedHistory(dir, state.ApprovalFingerprint, state.BatchFingerprint)
	if err != nil {
		return ApprovedDeliveryReceipt{}, err
	}
	if !canonicalEqual(receipt, approvedReceiptValue(state, history)) {
		return ApprovedDeliveryReceipt{}, errors.New("approved delivery receipt is stale")
	}
	return receipt, nil
}
