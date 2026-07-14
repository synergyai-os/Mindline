package productbrain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	DeliveryRunSchema     = "productbrain-delivery-run/v0.1"
	DeliveryHistorySchema = "productbrain-delivery-history/v0.1"
	DeliveryStateSchema   = "productbrain-delivery-state/v0.1"
	DeliverySummarySchema = "productbrain-delivery-summary/v0.1"
)

type journalWriter func(string, DeliveryRun) error

type DeliveryOptions struct {
	Now           func() time.Time
	journalWriter journalWriter
}
type DeliveryOperationResult struct {
	OperationID              string `json:"operation_id"`
	Kind                     string `json:"kind"`
	State                    string `json:"state"`
	Attempts                 int    `json:"attempts"`
	MutationResponseReceived bool   `json:"mutation_response_received,omitempty"`
	MutationObserved         bool   `json:"mutation_observed"`
	Acknowledged             bool   `json:"acknowledged"`
	SafeCategory             string `json:"safe_category,omitempty"`
	EntryDocID               string `json:"entry_doc_id,omitempty"`
	RemoteObjectID           string `json:"remote_object_id,omitempty"`
	ReadbackFingerprint      string `json:"readback_fingerprint,omitempty"`
	DraftVerified            bool   `json:"draft_verified,omitempty"`
	ActorVerified            bool   `json:"actor_verified,omitempty"`
	AttributionVerified      bool   `json:"attribution_verified,omitempty"`
}
type DeliveryRun struct {
	SchemaVersion                 string                    `json:"schema_version"`
	Fingerprint                   string                    `json:"fingerprint"`
	Sequence                      int                       `json:"sequence"`
	InvocationID                  string                    `json:"invocation_id"`
	OutboxFingerprint             string                    `json:"outbox_fingerprint"`
	ProfileFingerprint            string                    `json:"profile_fingerprint"`
	PreflightFingerprint          string                    `json:"preflight_fingerprint"`
	PreflightSnapshotRef          string                    `json:"preflight_snapshot_ref"`
	PreflightMutationCalls        int                       `json:"preflight_mutation_calls"`
	ExternalPreconditionsRepeated bool                      `json:"external_preconditions_repeated"`
	StartedAt                     string                    `json:"started_at"`
	EndedAt                       string                    `json:"ended_at,omitempty"`
	Outcome                       string                    `json:"outcome"`
	EntriesCreated                int                       `json:"entries_created_this_run"`
	RelationsCreated              int                       `json:"relations_created_this_run"`
	Operations                    []DeliveryOperationResult `json:"operations"`
}
type DeliveryHistory struct {
	SchemaVersion      string        `json:"schema_version"`
	Fingerprint        string        `json:"fingerprint"`
	OutboxFingerprint  string        `json:"outbox_fingerprint"`
	ProfileFingerprint string        `json:"profile_fingerprint"`
	RunRefs            []string      `json:"run_refs"`
	Runs               []DeliveryRun `json:"runs"`
}
type DeliveryState struct {
	SchemaVersion     string                    `json:"schema_version"`
	Fingerprint       string                    `json:"fingerprint"`
	OutboxFingerprint string                    `json:"outbox_fingerprint"`
	LatestSequence    int                       `json:"latest_sequence"`
	Operations        []DeliveryOperationResult `json:"operations"`
}
type DeliverySummary struct {
	SchemaVersion               string   `json:"schema_version"`
	Fingerprint                 string   `json:"fingerprint"`
	OutboxFingerprint           string   `json:"outbox_fingerprint"`
	ProfileFingerprint          string   `json:"profile_fingerprint"`
	PreflightLineageVerified    bool     `json:"preflight_lineage_verified"`
	RunCount                    int      `json:"run_count"`
	CompletedRunCount           int      `json:"completed_run_count"`
	InterruptedRunCount         int      `json:"interrupted_run_count"`
	FailedRunCount              int      `json:"failed_run_count"`
	ExpectedOperationCount      int      `json:"expected_operation_count"`
	EntriesAcknowledged         int      `json:"entries_acknowledged"`
	RelationsAcknowledged       int      `json:"relations_acknowledged"`
	Blocked                     int      `json:"blocked"`
	Mismatches                  int      `json:"mismatches"`
	FirstRunEntryMutations      int      `json:"first_run_entry_mutations"`
	FirstRunRelationMutations   int      `json:"first_run_relation_mutations"`
	LatestRunEntryMutations     int      `json:"latest_run_entry_mutations"`
	LatestRunRelationMutations  int      `json:"latest_run_relation_mutations"`
	ReplayZeroMutation          bool     `json:"replay_zero_mutation"`
	DraftOnly                   bool     `json:"draft_only"`
	EntryActorVerified          bool     `json:"entry_actor_verified"`
	RelationAttributionVerified bool     `json:"relation_attribution_verified"`
	PrivacyFindingCount         int      `json:"privacy_finding_count"`
	OperatorJudged              bool     `json:"operator_judged"`
	HeldOut                     bool     `json:"held_out"`
	Generalizable               bool     `json:"generalizable"`
	AutonomyClaim               bool     `json:"autonomy_claim"`
	DestinationWrites           int      `json:"destination_writes"`
	ProductBrainWrites          int      `json:"product_brain_writes"`
	RunRefs                     []string `json:"run_refs"`
}

func Deliver(ctx context.Context, outbox Outbox, profile DeliveryProfile, preflight PreflightArtifact, transport ProductBrainTransport, outDir string, options DeliveryOptions) (DeliverySummary, error) {
	if transport == nil {
		return DeliverySummary{}, errors.New("missing Product Brain transport")
	}
	if err := ValidateOutbox(outbox); err != nil {
		return DeliverySummary{}, err
	}
	if err := ValidatePreflight(preflight, outbox, profile); err != nil {
		return DeliverySummary{}, err
	}
	if err := privateio.PrepareDir(outDir); err != nil {
		return DeliverySummary{}, err
	}
	release, err := acquireDeliveryLock(outDir)
	if err != nil {
		return DeliverySummary{}, err
	}
	defer release()
	if err := validateDeliveryBinding(outDir, outbox, profile); err != nil {
		return DeliverySummary{}, err
	}
	snapshotRef, err := snapshotPreflight(outDir, preflight)
	if err != nil {
		return DeliverySummary{}, err
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	writer := options.journalWriter
	if writer == nil {
		writer = writeActiveJournal
	}
	history, err := loadDeliveryHistory(outDir, outbox.Fingerprint, hashValue(profile))
	if err != nil {
		return DeliverySummary{}, err
	}
	if err := sealInterruptedJournal(outDir, &history, now()); err != nil {
		return DeliverySummary{}, err
	}
	sequence := len(history.Runs) + 1
	started := now().UTC()
	run := DeliveryRun{SchemaVersion: DeliveryRunSchema, Sequence: sequence, InvocationID: hashText(fmt.Sprintf("%d|%d|%d", os.Getpid(), sequence, started.UnixNano()))[:20], OutboxFingerprint: outbox.Fingerprint, ProfileFingerprint: hashValue(profile), PreflightFingerprint: preflight.Fingerprint, PreflightSnapshotRef: snapshotRef, PreflightMutationCalls: 0, StartedAt: started.Format(time.RFC3339Nano), Outcome: "running"}
	for _, op := range outbox.Operations {
		run.Operations = append(run.Operations, DeliveryOperationResult{OperationID: op.OperationID, Kind: op.Kind, State: "pending"})
	}
	if err := persistActiveJournal(writer, outDir, run); err != nil {
		return DeliverySummary{}, err
	}
	capability, err := transport.ResolveWorkspace(ctx)
	if err != nil {
		return finishDeliveryFailure(outDir, outbox, profile, history, run, now(), "capability_missing", err, writer)
	}
	if err := checkCapability(capability, profile); err != nil {
		return finishDeliveryFailure(outDir, outbox, profile, history, run, now(), "workspace_mismatch", err, writer)
	}
	secretScanner, ok := transport.(RuntimeSecretScanner)
	if !ok {
		return finishDeliveryFailure(outDir, outbox, profile, history, run, now(), "capability_missing", errors.New("runtime secret scanner missing"), writer)
	}
	if len(secretScanner.RuntimeSecretFindings(outbox)) > 0 {
		return finishDeliveryFailure(outDir, outbox, profile, history, run, now(), "unsafe_outbound_value", errors.New("runtime secret scan failed"), writer)
	}
	if err := validateLiveCollectionContracts(ctx, transport, outbox, preflight); err != nil {
		return finishDeliveryFailure(outDir, outbox, profile, history, run, now(), "collection_contract_mismatch", err, writer)
	}
	run.ExternalPreconditionsRepeated = true
	if err := persistActiveJournal(writer, outDir, run); err != nil {
		return DeliverySummary{}, err
	}
	entryReadbacks := map[string]EntryReadback{}
	for index, op := range outbox.Operations {
		if op.Kind != "entry" {
			continue
		}
		readback, created, err := deliverEntry(ctx, transport, *op.Entry, &run, index, outDir, writer)
		if created {
			run.EntriesCreated++
		}
		if err != nil {
			var persistenceErr *journalPersistenceError
			if errors.As(err, &persistenceErr) {
				return DeliverySummary{}, err
			}
			return finishDeliveryFailure(outDir, outbox, profile, history, run, now(), safeCategory(err), err, writer)
		}
		entryReadbacks[op.Entry.EntryID] = readback
		if err := persistActiveJournal(writer, outDir, run); err != nil {
			return DeliverySummary{}, err
		}
	}
	for index, op := range outbox.Operations {
		if op.Kind != "relation" {
			continue
		}
		created, err := deliverRelation(ctx, transport, *op.Relation, outbox.ProfileSnapshot.ExpectedKeyID, entryReadbacks, &run, index, outDir, writer)
		if created {
			run.RelationsCreated++
		}
		if err != nil {
			var persistenceErr *journalPersistenceError
			if errors.As(err, &persistenceErr) {
				return DeliverySummary{}, err
			}
			return finishDeliveryFailure(outDir, outbox, profile, history, run, now(), safeCategory(err), err, writer)
		}
		if err := persistActiveJournal(writer, outDir, run); err != nil {
			return DeliverySummary{}, err
		}
	}
	run.Outcome = "completed"
	run.EndedAt = now().UTC().Format(time.RFC3339Nano)
	if err := persistActiveJournal(writer, outDir, run); err != nil {
		return DeliverySummary{}, err
	}
	history, err = sealRun(outDir, history, run)
	if err != nil {
		return DeliverySummary{}, err
	}
	summary, err := rebuildDeliveryProjections(outDir, outbox, profile, history)
	if err != nil {
		return DeliverySummary{}, err
	}
	return summary, nil
}

func deliverEntry(ctx context.Context, transport ProductBrainTransport, expected EntryOperation, run *DeliveryRun, index int, outDir string, writer journalWriter) (EntryReadback, bool, error) {
	result := &run.Operations[index]
	result.State = "reconciling"
	result.Attempts++
	if err := persistActiveJournal(writer, outDir, *run); err != nil {
		return EntryReadback{}, false, err
	}
	found, err := transport.GetEntry(ctx, expected.EntryID)
	if err != nil {
		return EntryReadback{}, false, err
	}
	if found.Found {
		if err := compareEntry(found, expected); err != nil {
			return EntryReadback{}, false, err
		}
		ackEntry(result, found)
		return found, false, nil
	}
	matches, err := transport.SearchEntries(ctx, expected.Name, expected.CollectionSlug)
	if err != nil {
		return EntryReadback{}, false, err
	}
	for _, match := range matches {
		if match.EntryID != expected.EntryID {
			return EntryReadback{}, false, errors.New("destination_name_conflict")
		}
	}
	result.State = "sending"
	if err := persistActiveJournal(writer, outDir, *run); err != nil {
		return EntryReadback{}, false, err
	}
	_, createErr := transport.CreateEntry(ctx, CreateEntryRequest{CollectionSlug: expected.CollectionSlug, EntryID: expected.EntryID, Name: expected.Name, Data: expected.Data, SourceRef: expected.SourceRef, SourceExcerpt: expected.SourceExcerpt, CreatedBy: expected.CreatedBy, ForceDraft: expected.ForceDraft})
	result.MutationResponseReceived = createErr == nil
	result.State = "reconciling"
	if err := persistActiveJournal(writer, outDir, *run); err != nil {
		return EntryReadback{}, createErr == nil, err
	}
	found, readErr := transport.GetEntry(ctx, expected.EntryID)
	if readErr != nil {
		return EntryReadback{}, createErr == nil, errors.New("ambiguous_outcome")
	}
	if !found.Found {
		if createErr != nil {
			return EntryReadback{}, false, createErr
		}
		return EntryReadback{}, true, errors.New("ambiguous_outcome")
	}
	result.MutationObserved = true
	if err := compareEntry(found, expected); err != nil {
		return EntryReadback{}, true, err
	}
	ackEntry(result, found)
	return found, true, nil
}
func deliverRelation(ctx context.Context, transport ProductBrainTransport, expected RelationOperation, expectedKeyID string, entries map[string]EntryReadback, run *DeliveryRun, index int, outDir string, writer journalWriter) (bool, error) {
	from, to := entries[expected.FromEntryID], entries[expected.ToEntryID]
	if !from.Found || !to.Found {
		return false, errors.New("dependency_not_acknowledged")
	}
	result := &run.Operations[index]
	result.State = "reconciling"
	result.Attempts++
	if err := persistActiveJournal(writer, outDir, *run); err != nil {
		return false, err
	}
	relations, err := transport.ListEntryRelations(ctx, expected.FromEntryID)
	if err != nil {
		return false, err
	}
	if matched, relationID, conflict := findRelation(relations, from.DocID, to.DocID, expected); conflict {
		return false, errors.New("readback_mismatch")
	} else if matched {
		ackRelation(result, relationID, expected, expectedKeyID)
		return false, nil
	}
	result.State = "sending"
	if err := persistActiveJournal(writer, outDir, *run); err != nil {
		return false, err
	}
	createResult, createErr := transport.CreateEntryRelation(ctx, CreateRelationRequest{FromEntryID: expected.FromEntryID, ToEntryID: expected.ToEntryID, Type: expected.Type, Metadata: expected.Metadata, IfMissing: expected.IfMissing})
	result.MutationResponseReceived = createErr == nil
	result.State = "reconciling"
	if err := persistActiveJournal(writer, outDir, *run); err != nil {
		return createErr == nil, err
	}
	relations, readErr := transport.ListEntryRelations(ctx, expected.FromEntryID)
	if readErr != nil {
		return createErr == nil, errors.New("ambiguous_outcome")
	}
	matched, relationID, conflict := findRelation(relations, from.DocID, to.DocID, expected)
	if conflict {
		result.MutationObserved = true
		return true, errors.New("readback_mismatch")
	}
	if !matched {
		if createErr == nil && relationIDExists(relations, createResult.RelationID) {
			result.MutationObserved = true
			return true, errors.New("readback_mismatch")
		}
		if createErr != nil {
			return false, createErr
		}
		return true, errors.New("ambiguous_outcome")
	}
	result.MutationObserved = true
	ackRelation(result, relationID, expected, expectedKeyID)
	return true, nil
}

func relationIDExists(relations []RelationReadback, relationID string) bool {
	if strings.TrimSpace(relationID) == "" {
		return false
	}
	for _, relation := range relations {
		if relation.RelationID == relationID {
			return true
		}
	}
	return false
}

func compareEntry(actual EntryReadback, expected EntryOperation) error {
	if !actual.Found || actual.EntryID != expected.EntryID || actual.CollectionSlug != expected.CollectionSlug || actual.Name != expected.Name || actual.Status != "draft" || actual.SourceRef != expected.SourceRef || actual.SourceExcerpt != expected.SourceExcerpt || actual.CreatedBy != expected.CreatedBy || !canonicalEqual(actual.Data, expected.Data) {
		return errors.New("readback_mismatch")
	}
	return nil
}
func findRelation(values []RelationReadback, fromDoc, toDoc string, expected RelationOperation) (bool, string, bool) {
	identityCount := 0
	matchCount := 0
	matchID := ""
	metadataConflict := false
	for _, relation := range values {
		if relation.FromDocID == fromDoc && relation.ToDocID == toDoc && relation.Type == expected.Type {
			identityCount++
			if canonicalEqual(relation.Metadata, expected.Metadata) {
				matchCount++
				matchID = relation.RelationID
			} else {
				metadataConflict = true
			}
		}
	}
	if identityCount == 1 && matchCount == 1 && !metadataConflict {
		return true, matchID, false
	}
	return false, "", identityCount > 0
}
func canonicalEqual(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}
func ackEntry(result *DeliveryOperationResult, readback EntryReadback) {
	result.State = "acknowledged"
	result.Acknowledged = true
	result.EntryDocID = readback.DocID
	result.RemoteObjectID = readback.EntryID
	result.ReadbackFingerprint = hashValue(readback)
	result.DraftVerified = readback.Status == "draft"
	result.ActorVerified = readback.CreatedBy == ExpectedCreatedBy
}
func ackRelation(result *DeliveryOperationResult, relationID string, expected RelationOperation, expectedKeyID string) {
	result.State = "acknowledged"
	result.Acknowledged = true
	result.RemoteObjectID = relationID
	result.ReadbackFingerprint = hashValue(map[string]any{"relation_id": relationID, "identity": expected.RelationIdentity, "metadata": expected.Metadata})
	result.AttributionVerified = validRelationMetadata(expected.Metadata, expectedKeyID)
}
func safeCategory(err error) string {
	if err == nil {
		return ""
	}
	var transportErr *TransportError
	if errors.As(err, &transportErr) {
		return normalizeTransportCategory(transportErr.Category, transportErr.MayHaveCommitted)
	}
	if ValidSafeDeliveryCategory(err.Error()) {
		return err.Error()
	}
	return "local_state_failure"
}

type journalPersistenceError struct{ cause error }

func (e *journalPersistenceError) Error() string { return "local_state_failure" }
func (e *journalPersistenceError) Unwrap() error { return e.cause }

func persistActiveJournal(writer journalWriter, outDir string, run DeliveryRun) error {
	if err := writer(outDir, run); err != nil {
		return &journalPersistenceError{cause: err}
	}
	return nil
}
func checkCapability(cap WorkspaceCapability, p DeliveryProfile) error {
	if cap.ID != p.Workspace.ExpectedID || cap.Slug != p.Workspace.ExpectedSlug || cap.KeyScope != "readwrite" || cap.KeyID != p.Credential.ExpectedKeyID {
		return errors.New("workspace_mismatch")
	}
	switch cap.GovernanceMode {
	case "open", "consensus", "role":
		return nil
	}
	return errors.New("capability_missing")
}

func finishDeliveryFailure(outDir string, outbox Outbox, profile DeliveryProfile, history DeliveryHistory, run DeliveryRun, ended time.Time, category string, cause error, writer journalWriter) (DeliverySummary, error) {
	category = normalizeTransportCategory(category, false)
	run.Outcome = "failed"
	run.EndedAt = ended.UTC().Format(time.RFC3339Nano)
	for i := range run.Operations {
		if run.Operations[i].State == "sending" || run.Operations[i].State == "reconciling" {
			run.Operations[i].State = "blocked"
			run.Operations[i].SafeCategory = category
			break
		}
	}
	if err := persistActiveJournal(writer, outDir, run); err != nil {
		return DeliverySummary{}, err
	}
	sealed, sealErr := sealRun(outDir, history, run)
	if sealErr != nil {
		return DeliverySummary{}, sealErr
	}
	summary, projectionErr := rebuildDeliveryProjections(outDir, outbox, profile, sealed)
	if projectionErr != nil {
		return DeliverySummary{}, errors.New("local_state_failure")
	}
	_ = cause
	return summary, errors.New(category)
}

func sortedOperations(values []DeliveryOperationResult) []DeliveryOperationResult {
	out := append([]DeliveryOperationResult{}, values...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].OperationID < out[j].OperationID })
	return out
}

var _ = strings.TrimSpace
