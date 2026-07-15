package productbrain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

type deliveryLock struct {
	PID      int    `json:"pid"`
	Hostname string `json:"hostname"`
}

// acquireApprovedOrderingLock serializes only cancellation creation and the
// cancellation-check/attempt-reservation transaction. It is deliberately not
// held during destination I/O, so cancellation authority remains writable
// while a previously reserved mutation is in flight.
func acquireApprovedOrderingLock(ctx context.Context, dir string) (func(), error) {
	path := filepath.Join(dir, ".approved-ordering.lock")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, privateio.FileMode)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(privateio.FileMode); err != nil {
		file.Close()
		return nil, err
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func acquireDeliveryLock(dir string) (func(), error) {
	path := filepath.Join(dir, ".delivery.lock")
	host, _ := os.Hostname()
	lock := deliveryLock{PID: os.Getpid(), Hostname: host}
	data, _ := json.Marshal(lock)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateio.FileMode)
	if err == nil {
		if _, writeErr := file.Write(data); writeErr != nil {
			file.Close()
			os.Remove(path)
			return nil, writeErr
		}
		file.Sync()
		file.Close()
		return func() { _ = os.Remove(path) }, nil
	}
	if !os.IsExist(err) {
		return nil, err
	}
	var existing deliveryLock
	if readErr := privateio.ReadJSONStrict(dir, path, &existing); readErr != nil {
		return nil, errors.New("delivery_locked")
	}
	if existing.Hostname != host || existing.PID <= 0 {
		return nil, errors.New("delivery_locked")
	}
	processErr := syscall.Kill(existing.PID, 0)
	if processErr == nil || processErr == syscall.EPERM {
		return nil, errors.New("delivery_locked")
	}
	if processErr != syscall.ESRCH {
		return nil, errors.New("delivery_locked")
	}
	if err := os.Remove(path); err != nil {
		return nil, errors.New("delivery_locked")
	}
	return acquireDeliveryLock(dir)
}
func validateDeliveryBinding(dir string, outbox Outbox, profile DeliveryProfile) error {
	path := filepath.Join(dir, "delivery-binding.json")
	binding := map[string]any{"outbox_fingerprint": outbox.Fingerprint, "profile_fingerprint": hashValue(profile)}
	var existing map[string]any
	if err := privateio.ReadJSONStrict(dir, path, &existing); err == nil {
		if !canonicalEqual(existing, binding) {
			return errors.New("outbox_state_mismatch")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return privateio.WriteJSONNoReplace(path, binding)
}
func snapshotPreflight(dir string, artifact PreflightArtifact) (string, error) {
	subdir := filepath.Join(dir, "preflight-snapshots")
	if err := privateio.PrepareDir(subdir); err != nil {
		return "", err
	}
	name := artifact.Fingerprint + ".json"
	path := filepath.Join(subdir, name)
	var loaded PreflightArtifact
	if err := privateio.ReadJSONStrict(dir, path, &loaded); err == nil {
		if loaded.Fingerprint != artifact.Fingerprint || !canonicalEqual(loaded, artifact) {
			return "", errors.New("preflight snapshot mismatch")
		}
	} else if os.IsNotExist(err) {
		if err := privateio.WriteJSONNoReplace(path, artifact); err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("preflight-snapshots", name)), nil
}
func writeActiveJournal(dir string, run DeliveryRun) error {
	run.Fingerprint = hashValue(run)
	return privateio.WriteJSON(filepath.Join(dir, ".delivery-active.json"), run)
}
func loadDeliveryHistory(dir, outboxFP, profileFP string) (DeliveryHistory, error) {
	path := filepath.Join(dir, "delivery-history.json")
	projected, projectionExists, err := readProjectedHistory(dir, path, outboxFP, profileFP)
	if err != nil {
		return DeliveryHistory{}, err
	}
	reconstructed, err := reconstructDeliveryHistory(dir, outboxFP, profileFP)
	if err != nil {
		return DeliveryHistory{}, err
	}
	if projectionExists {
		if len(projected.Runs) > len(reconstructed.Runs) {
			return DeliveryHistory{}, errors.New("delivery history authority mismatch")
		}
		for i := range projected.Runs {
			if projected.RunRefs[i] != reconstructed.RunRefs[i] || !canonicalEqual(projected.Runs[i], reconstructed.Runs[i]) {
				return DeliveryHistory{}, errors.New("delivery history authority mismatch")
			}
		}
	}
	if len(reconstructed.Runs) > 0 && (!projectionExists || !canonicalEqual(projected, reconstructed)) {
		if err := privateio.WriteJSON(path, reconstructed); err != nil {
			return DeliveryHistory{}, err
		}
	}
	return reconstructed, nil
}

func readProjectedHistory(root, path, outboxFP, profileFP string) (DeliveryHistory, bool, error) {
	var history DeliveryHistory
	if err := privateio.ReadJSONStrict(root, path, &history); err != nil {
		if os.IsNotExist(err) {
			return DeliveryHistory{}, false, nil
		}
		return DeliveryHistory{}, false, err
	}
	if history.SchemaVersion != DeliveryHistorySchema || history.Fingerprint != hashValue(history) || history.OutboxFingerprint != outboxFP || history.ProfileFingerprint != profileFP || len(history.RunRefs) != len(history.Runs) {
		return DeliveryHistory{}, false, errors.New("delivery history mismatch")
	}
	for i, run := range history.Runs {
		if run.SchemaVersion != DeliveryRunSchema || run.Sequence != i+1 || run.Fingerprint != hashValue(run) || validateDeliveryRunOperations(run) != nil {
			return DeliveryHistory{}, false, errors.New("delivery history authority mismatch")
		}
	}
	return history, true, nil
}

func reconstructDeliveryHistory(dir, outboxFP, profileFP string) (DeliveryHistory, error) {
	history := DeliveryHistory{SchemaVersion: DeliveryHistorySchema, OutboxFingerprint: outboxFP, ProfileFingerprint: profileFP, RunRefs: []string{}, Runs: []DeliveryRun{}}
	runsDir := filepath.Join(dir, "delivery-runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return history, nil
		}
		return DeliveryHistory{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return DeliveryHistory{}, errors.New("delivery run store contains unexpected artifact")
		}
		var run DeliveryRun
		if err := privateio.ReadJSONStrict(dir, filepath.Join(runsDir, entry.Name()), &run); err != nil {
			return DeliveryHistory{}, err
		}
		expectedSequence := len(history.Runs) + 1
		expectedName := fmt.Sprintf("%06d-%s.json", expectedSequence, run.InvocationID)
		if entry.Name() != expectedName || run.SchemaVersion != DeliveryRunSchema || run.Sequence != expectedSequence || run.Fingerprint != hashValue(run) || run.OutboxFingerprint != outboxFP || run.ProfileFingerprint != profileFP || run.Outcome == "running" || validateDeliveryRunOperations(run) != nil {
			return DeliveryHistory{}, errors.New("delivery run authority mismatch")
		}
		history.RunRefs = append(history.RunRefs, filepath.ToSlash(filepath.Join("delivery-runs", entry.Name())))
		history.Runs = append(history.Runs, run)
	}
	if len(history.Runs) > 0 {
		history.Fingerprint = hashValue(history)
	}
	return history, nil
}
func sealInterruptedJournal(dir string, history *DeliveryHistory, ended time.Time) error {
	path := filepath.Join(dir, ".delivery-active.json")
	var run DeliveryRun
	if err := privateio.ReadJSONStrict(dir, path, &run); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if run.Fingerprint != hashValue(run) || run.OutboxFingerprint != history.OutboxFingerprint || run.ProfileFingerprint != history.ProfileFingerprint {
		return errors.New("active delivery journal mismatch")
	}
	if run.Sequence <= len(history.Runs) {
		sealed := history.Runs[run.Sequence-1]
		if sealed.InvocationID != run.InvocationID || !canonicalEqual(sealed, run) {
			return errors.New("active delivery journal conflicts with sealed authority")
		}
		return os.Remove(path)
	}
	if run.Sequence != len(history.Runs)+1 {
		return errors.New("active delivery journal sequence mismatch")
	}
	run.Outcome = "interrupted"
	run.EndedAt = ended.UTC().Format(time.RFC3339Nano)
	run.Fingerprint = hashValue(run)
	sealed, err := sealRun(dir, *history, run)
	if err != nil {
		return err
	}
	*history = sealed
	return nil
}
func sealRun(dir string, history DeliveryHistory, run DeliveryRun) (DeliveryHistory, error) {
	if run.Sequence != len(history.Runs)+1 || run.OutboxFingerprint != history.OutboxFingerprint || run.ProfileFingerprint != history.ProfileFingerprint || run.Outcome == "running" || validateDeliveryRunOperations(run) != nil {
		return DeliveryHistory{}, errors.New("delivery run sequence mismatch")
	}
	run.Fingerprint = hashValue(run)
	runsDir := filepath.Join(dir, "delivery-runs")
	if err := privateio.PrepareDir(runsDir); err != nil {
		return DeliveryHistory{}, err
	}
	name := fmt.Sprintf("%06d-%s.json", run.Sequence, run.InvocationID)
	if err := privateio.WriteJSONNoReplace(filepath.Join(runsDir, name), run); err != nil {
		return DeliveryHistory{}, err
	}
	history.RunRefs = append(history.RunRefs, filepath.ToSlash(filepath.Join("delivery-runs", name)))
	history.Runs = append(history.Runs, run)
	history.Fingerprint = hashValue(history)
	if err := privateio.WriteJSON(filepath.Join(dir, "delivery-history.json"), history); err != nil {
		return DeliveryHistory{}, err
	}
	if err := os.Remove(filepath.Join(dir, ".delivery-active.json")); err != nil && !os.IsNotExist(err) {
		return DeliveryHistory{}, err
	}
	return history, nil
}

func validateDeliveryRunOperations(run DeliveryRun) error {
	seen := map[string]bool{}
	entryMutations := 0
	relationMutations := 0
	for _, operation := range run.Operations {
		if operation.OperationID == "" || seen[operation.OperationID] || operation.Kind != "entry" && operation.Kind != "relation" || operation.Attempts < 0 {
			return errors.New("delivery operation authority mismatch")
		}
		seen[operation.OperationID] = true
		switch operation.State {
		case "pending":
			if operation.Attempts != 0 || operation.MutationResponseReceived || operation.Acknowledged || operation.MutationObserved || operation.SafeCategory != "" {
				return errors.New("delivery operation authority mismatch")
			}
		case "sending":
			if operation.Attempts < 1 || operation.MutationResponseReceived || operation.Acknowledged || operation.MutationObserved || operation.SafeCategory != "" {
				return errors.New("delivery operation authority mismatch")
			}
		case "reconciling":
			if operation.Attempts < 1 || operation.Acknowledged || operation.MutationObserved || operation.SafeCategory != "" {
				return errors.New("delivery operation authority mismatch")
			}
		case "blocked":
			if operation.Attempts < 1 || operation.Acknowledged || !ValidSafeDeliveryCategory(operation.SafeCategory) || operation.MutationObserved != (operation.SafeCategory == "readback_mismatch") || operation.MutationResponseReceived && operation.SafeCategory != "readback_mismatch" && operation.SafeCategory != "ambiguous_outcome" {
				return errors.New("delivery operation authority mismatch")
			}
		case "acknowledged":
			if operation.Attempts < 1 || !operation.Acknowledged || operation.SafeCategory != "" {
				return errors.New("delivery operation authority mismatch")
			}
		default:
			return errors.New("delivery operation authority mismatch")
		}
		if operation.MutationResponseReceived || operation.MutationObserved {
			if operation.Kind == "entry" {
				entryMutations++
			} else {
				relationMutations++
			}
		}
	}
	if run.EntriesCreated != entryMutations || run.RelationsCreated != relationMutations {
		return errors.New("delivery mutation counter mismatch")
	}
	return nil
}
func rebuildDeliveryProjections(dir string, outbox Outbox, profile DeliveryProfile, history DeliveryHistory) (DeliverySummary, error) {
	latest := map[string]DeliveryOperationResult{}
	for _, run := range history.Runs {
		for _, op := range run.Operations {
			prior, ok := latest[op.OperationID]
			if !ok || op.Acknowledged || !prior.Acknowledged {
				latest[op.OperationID] = op
			}
		}
	}
	operations := make([]DeliveryOperationResult, 0, len(latest))
	for _, op := range latest {
		operations = append(operations, op)
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].OperationID < operations[j].OperationID })
	state := DeliveryState{SchemaVersion: DeliveryStateSchema, OutboxFingerprint: history.OutboxFingerprint, LatestSequence: len(history.Runs), Operations: operations}
	state.Fingerprint = hashValue(state)
	if err := privateio.WriteJSON(filepath.Join(dir, "delivery-state.json"), state); err != nil {
		return DeliverySummary{}, err
	}
	summary := summarizeDelivery(outbox, history, operations)
	if profile.ProfileID != "" {
		summary.ProfileFingerprint = hashValue(profile)
	}
	summary.Fingerprint = hashValue(summary)
	if err := privateio.WriteJSON(filepath.Join(dir, "delivery-summary.json"), summary); err != nil {
		return DeliverySummary{}, err
	}
	if err := privateio.WriteFile(filepath.Join(dir, "mindline-review-packet.md"), []byte(deliveryReviewPacket(outbox, history, summary)), false); err != nil {
		return DeliverySummary{}, err
	}
	return summary, nil
}
func summarizeDelivery(outbox Outbox, history DeliveryHistory, operations []DeliveryOperationResult) DeliverySummary {
	summary := DeliverySummary{SchemaVersion: DeliverySummarySchema, OutboxFingerprint: history.OutboxFingerprint, ProfileFingerprint: history.ProfileFingerprint, PreflightLineageVerified: true, RunCount: len(history.Runs), ExpectedOperationCount: len(outbox.Operations), DraftOnly: true, EntryActorVerified: true, RelationAttributionVerified: true, PrivacyFindingCount: len(outbox.PrivacyFindings), OperatorJudged: outbox.OperatorJudged, HeldOut: outbox.HeldOut, Generalizable: outbox.Generalizable, AutonomyClaim: outbox.AutonomyClaim, RunRefs: append([]string{}, history.RunRefs...)}
	entrySeen, relationSeen := false, false
	for index, run := range history.Runs {
		if run.Outcome == "completed" {
			summary.CompletedRunCount++
		} else if run.Outcome == "interrupted" {
			summary.InterruptedRunCount++
		} else if run.Outcome == "failed" {
			summary.FailedRunCount++
		}
		safePreconditionFailure := run.Outcome == "failed" && !run.ExternalPreconditionsRepeated && run.EntriesCreated == 0 && run.RelationsCreated == 0 && !deliveryRunObservedMutation(run)
		if run.Outcome == "failed" && (run.EntriesCreated != 0 || run.RelationsCreated != 0 || deliveryRunObservedMutation(run)) || run.PreflightFingerprint == "" || run.PreflightSnapshotRef == "" || run.PreflightMutationCalls != 0 || !run.ExternalPreconditionsRepeated && !safePreconditionFailure {
			summary.PreflightLineageVerified = false
		}
		summary.DestinationWrites += run.EntriesCreated + run.RelationsCreated
		summary.ProductBrainWrites += run.EntriesCreated + run.RelationsCreated
		if len(history.Runs) == 1 || index < len(history.Runs)-1 {
			summary.FirstRunEntryMutations += run.EntriesCreated
			summary.FirstRunRelationMutations += run.RelationsCreated
		}
		for _, op := range run.Operations {
			if op.State == "blocked" {
				summary.Blocked++
			}
			if op.SafeCategory == "readback_mismatch" {
				summary.Mismatches++
			}
		}
	}
	if len(history.Runs) > 0 {
		latest := history.Runs[len(history.Runs)-1]
		summary.LatestRunEntryMutations = latest.EntriesCreated
		summary.LatestRunRelationMutations = latest.RelationsCreated
		if len(history.Runs) >= 2 && latest.Outcome == "completed" && latest.EntriesCreated == 0 && latest.RelationsCreated == 0 {
			allAck := true
			for _, op := range latest.Operations {
				if !op.Acknowledged {
					allAck = false
				}
			}
			summary.ReplayZeroMutation = allAck
		}
	}
	for _, op := range operations {
		if op.Acknowledged {
			if op.Kind == "entry" {
				entrySeen = true
				summary.EntriesAcknowledged++
				if !op.DraftVerified {
					summary.DraftOnly = false
				}
				if !op.ActorVerified {
					summary.EntryActorVerified = false
				}
			} else {
				relationSeen = true
				summary.RelationsAcknowledged++
				if !op.AttributionVerified {
					summary.RelationAttributionVerified = false
				}
			}
		}
	}
	if !entrySeen {
		summary.DraftOnly = false
		summary.EntryActorVerified = false
	}
	if !relationSeen {
		summary.RelationAttributionVerified = false
	}
	return summary
}

func deliveryRunObservedMutation(run DeliveryRun) bool {
	for _, operation := range run.Operations {
		if operation.MutationObserved {
			return true
		}
	}
	return false
}
func safeSequenceFromName(name string) int {
	value, _ := strconv.Atoi(strings.SplitN(name, "-", 2)[0])
	return value
}

var _ = safeSequenceFromName
var _ = time.Now
