package orchestration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	RunPlanSchemaVersion        = "mindline-activation-run-plan/v0.1"
	InventorySchemaVersion      = "mindline-activation-inventory/v0.2"
	StrategySchemaVersion       = "mindline-activation-strategy/v0.1"
	BatchApprovalSchemaVersion  = "mindline-activation-batch-approval/v0.1"
	SampleManifestSchemaVersion = "mindline-activation-sample/v0.1"
	EventSchemaVersion          = "mindline-activation-event/v0.1"

	RunModeProof                = "proof"
	RunModeExperimentalDrain    = "experimental_drain"
	PrivacyPolicySyntheticOnly  = "synthetic_only"
	PrivacyPolicyPrivateRuntime = "private_runtime_authorized"
)

var (
	ErrInvalidRunPlan     = errors.New("invalid run plan")
	ErrInvalidInventory   = errors.New("invalid inventory snapshot")
	ErrConfigurationDrift = errors.New("configuration drift requires a new run")
	ErrVersionConflict    = errors.New("run version conflict")
	ErrIllegalTransition  = errors.New("illegal run transition")
	ErrInvalidEvent       = errors.New("invalid activation event")
	ErrSampleChanged      = errors.New("sealed sample changed")
	ErrSampleBudget       = errors.New("proof sample budget exceeded")
	ErrInvalidReadiness   = errors.New("invalid readiness evidence")
	ErrMissingEventStore  = errors.New("missing activation event store")
)

type SessionRef string
type RunID string
type ExpectedVersion uint64

type SourceScope struct {
	ConnectorKind     string `json:"connector_kind"`
	WorkspaceID       string `json:"workspace_id"`
	ChannelID         string `json:"channel_id"`
	LowerInclusive    string `json:"lower_inclusive"`
	UpperInclusive    string `json:"upper_inclusive"`
	IncludeThreads    bool   `json:"include_threads"`
	IncludeReplies    bool   `json:"include_replies"`
	AttachmentPolicy  string `json:"attachment_policy"`
	PrivateFilePolicy string `json:"private_file_policy"`
	EditDeletePolicy  string `json:"edit_delete_policy"`
	AdapterVersion    string `json:"adapter_version"`
	Fingerprint       string `json:"fingerprint"`
}

type InventorySnapshot struct {
	SchemaVersion   string          `json:"schema_version"`
	Fingerprint     string          `json:"fingerprint"`
	SourceIdentity  string          `json:"source_identity"`
	Watermark       string          `json:"watermark"`
	OccurrenceCount int             `json:"occurrence_count"`
	CanonicalCount  int             `json:"canonical_count"`
	SourceRecords   []SourceRecord  `json:"source_records"`
	URLOccurrences  []URLOccurrence `json:"url_occurrences"`
	CanonicalItems  []InventoryItem `json:"canonical_items"`
	Strata          []StratumCount  `json:"strata"`
	Completeness    []EvidenceCheck `json:"completeness"`
}

type SourceRecord struct {
	SourceRecordID     string   `json:"source_record_id"`
	NativeMessageID    string   `json:"native_message_id"`
	NativeTimestamp    string   `json:"native_timestamp"`
	ContentFingerprint string   `json:"content_fingerprint"`
	URLOccurrenceIDs   []string `json:"url_occurrence_ids"`
	EditDeleteState    string   `json:"edit_delete_state,omitempty"`
	ThreadParentID     string   `json:"thread_parent_id,omitempty"`
}

type URLOccurrence struct {
	URLOccurrenceID   string `json:"url_occurrence_id"`
	SourceRecordID    string `json:"source_record_id"`
	SourceOrdinal     int    `json:"source_ordinal"`
	ObservedURL       string `json:"observed_url,omitempty"`
	CanonicalItemID   string `json:"canonical_item_id"`
	SanitizationState string `json:"sanitization_state,omitempty"`
}

type InventoryItem struct {
	CanonicalItemID     string   `json:"canonical_item_id"`
	CanonicalURL        string   `json:"canonical_url"`
	RetrievalStrategyID string   `json:"retrieval_strategy_id"`
	FormatVariant       string   `json:"format_variant"`
	OccurrenceIDs       []string `json:"occurrence_ids"`
	AccessState         string   `json:"access_state,omitempty"`
	CompletenessStatus  string   `json:"completeness_status,omitempty"`
}

type StratumCount struct {
	RetrievalStrategyID string `json:"retrieval_strategy_id"`
	FormatVariant       string `json:"format_variant"`
	CanonicalCount      int    `json:"canonical_count"`
}

type EvidenceCheck struct {
	Name                string `json:"name"`
	Status              string `json:"status"`
	EvidenceFingerprint string `json:"evidence_fingerprint,omitempty"`
}

type StrategySnapshot struct {
	SchemaVersion    string   `json:"schema_version"`
	StrategyID       string   `json:"strategy_id"`
	Version          string   `json:"version"`
	Fingerprint      string   `json:"fingerprint"`
	ContextLenses    string   `json:"context_lenses"`
	RoutingPolicy    string   `json:"routing_policy"`
	SignificantTerms []string `json:"significant_terms"`
	IncludeTerms     []string `json:"include_terms"`
	ExcludeTerms     []string `json:"exclude_terms"`
}

type RunBudgets struct {
	MaximumItems           int   `json:"maximum_items"`
	MaximumBytes           int64 `json:"maximum_bytes"`
	MaximumAttempts        int   `json:"maximum_attempts"`
	MaximumNetworkRequests int   `json:"maximum_network_requests"`
	MaximumWallTimeSeconds int   `json:"maximum_wall_time_seconds"`
	MaximumCostMicrounits  int64 `json:"maximum_cost_microunits"`
	MaximumRetryAttempts   int   `json:"maximum_retry_attempts"`
	ManualSupportTolerance int   `json:"manual_support_tolerance"`
}

type RunPlan struct {
	SchemaVersion          string            `json:"schema_version"`
	Fingerprint            string            `json:"fingerprint"`
	SourceScopeFingerprint string            `json:"source_scope_fingerprint"`
	InventoryFingerprint   string            `json:"inventory_fingerprint"`
	StrategyFingerprint    string            `json:"strategy_fingerprint"`
	ComponentVersions      map[string]string `json:"component_versions"`
	PrivacyPolicy          string            `json:"privacy_policy"`
	Mode                   string            `json:"mode"`
	IdempotencyNamespace   string            `json:"idempotency_namespace"`
	Budgets                RunBudgets        `json:"budgets"`
}

type BatchApproval struct {
	SchemaVersion                      string   `json:"schema_version"`
	Fingerprint                        string   `json:"fingerprint"`
	BatchFingerprint                   string   `json:"batch_fingerprint"`
	OutboxFingerprint                  string   `json:"outbox_fingerprint"`
	PreflightFingerprint               string   `json:"preflight_fingerprint"`
	PrivacyFingerprint                 string   `json:"privacy_fingerprint"`
	DestinationWorkspaceID             string   `json:"destination_workspace_id"`
	DestinationKeyID                   string   `json:"destination_key_id"`
	OrderedOperationFingerprints       []string `json:"ordered_operation_fingerprints"`
	MaximumDestinationWrites           int      `json:"maximum_destination_writes"`
	MaximumMutationAttempts            int      `json:"maximum_mutation_attempts"`
	HumanInitiationEvidenceFingerprint string   `json:"human_initiation_evidence_fingerprint"`
	ApprovedAt                         string   `json:"approved_at"`
	ExpiresAt                          string   `json:"expires_at"`
}

// The application ports are deliberately defined at the consuming boundary.
// Provider/source/destination packages implement them without shaping the core.
type SourceInventory interface {
	Probe(ctx context.Context, session SessionRef, scope SourceScope) (SourceCapability, error)
	Freeze(ctx context.Context, session SessionRef, scope SourceScope) (InventorySnapshot, error)
}

type SourceCapability struct {
	ContributorID       string `json:"contributor_id"`
	Version             string `json:"version"`
	IdentityFingerprint string `json:"identity_fingerprint"`
}

type RetrievalRequest struct {
	CanonicalItemID string `json:"canonical_item_id"`
}
type RetrievalResult struct {
	EvidenceFingerprint string `json:"evidence_fingerprint"`
}
type ProcessingRequest struct {
	CanonicalItemID string `json:"canonical_item_id"`
}
type ProcessingResult struct {
	EvidenceFingerprint string `json:"evidence_fingerprint"`
}
type BatchCandidate struct {
	Fingerprint string `json:"fingerprint"`
}
type PreflightReceipt struct {
	Fingerprint string `json:"fingerprint"`
}
type ApprovedBatch struct {
	ApprovalFingerprint string `json:"approval_fingerprint"`
}
type DeliveryReceiptRef struct {
	Fingerprint string `json:"fingerprint"`
}
type DeliveryIntentRef struct {
	Fingerprint string `json:"fingerprint"`
}
type ApprovalRef struct {
	Fingerprint string `json:"fingerprint"`
}
type CancellationReceipt struct {
	Fingerprint string `json:"fingerprint"`
}

type Retriever interface {
	Retrieve(ctx context.Context, request RetrievalRequest) (RetrievalResult, error)
}

type Processor interface {
	Process(ctx context.Context, request ProcessingRequest) (ProcessingResult, error)
}

type DestinationDelivery interface {
	Preflight(ctx context.Context, batch BatchCandidate) (PreflightReceipt, error)
	DeliverApproved(ctx context.Context, batch ApprovedBatch) (DeliveryReceiptRef, error)
	Reconcile(ctx context.Context, intent DeliveryIntentRef) (DeliveryReceiptRef, error)
	CancelApproved(ctx context.Context, approval ApprovalRef) (CancellationReceipt, error)
}

func Fingerprint(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func SealRunPlan(plan *RunPlan) error {
	if plan == nil {
		return ErrInvalidRunPlan
	}
	plan.Fingerprint = ""
	if err := validateRunPlanFields(*plan); err != nil {
		return err
	}
	plan.Fingerprint = Fingerprint(*plan)
	return nil
}

func ValidateRunPlan(plan RunPlan) error {
	fingerprint := plan.Fingerprint
	if fingerprint == "" {
		return ErrInvalidRunPlan
	}
	plan.Fingerprint = ""
	if err := validateRunPlanFields(plan); err != nil {
		return err
	}
	if fingerprint != Fingerprint(plan) {
		return fmt.Errorf("%w: fingerprint mismatch", ErrInvalidRunPlan)
	}
	return nil
}

func validateRunPlanFields(plan RunPlan) error {
	if plan.SchemaVersion != RunPlanSchemaVersion || strings.TrimSpace(plan.SourceScopeFingerprint) == "" || strings.TrimSpace(plan.InventoryFingerprint) == "" || strings.TrimSpace(plan.StrategyFingerprint) == "" || plan.PrivacyPolicy != PrivacyPolicySyntheticOnly && plan.PrivacyPolicy != PrivacyPolicyPrivateRuntime || strings.TrimSpace(plan.IdempotencyNamespace) == "" {
		return ErrInvalidRunPlan
	}
	if plan.Mode != RunModeProof && plan.Mode != RunModeExperimentalDrain {
		return ErrInvalidRunPlan
	}
	if len(plan.ComponentVersions) == 0 || plan.Budgets.MaximumItems < 0 || plan.Budgets.MaximumBytes <= 0 || plan.Budgets.MaximumAttempts <= 0 || plan.Budgets.MaximumNetworkRequests <= 0 || plan.Budgets.MaximumWallTimeSeconds <= 0 || plan.Budgets.MaximumCostMicrounits < 0 || plan.Budgets.MaximumRetryAttempts < 0 || plan.Budgets.ManualSupportTolerance < 0 {
		return ErrInvalidRunPlan
	}
	for name, version := range plan.ComponentVersions {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
			return ErrInvalidRunPlan
		}
	}
	return nil
}

func SealInventorySnapshot(snapshot *InventorySnapshot) error {
	if snapshot == nil {
		return ErrInvalidInventory
	}
	snapshot.Fingerprint = ""
	canonicalizeInventory(snapshot)
	if err := validateInventoryFields(*snapshot); err != nil {
		return err
	}
	snapshot.OccurrenceCount = len(snapshot.URLOccurrences)
	snapshot.CanonicalCount = len(snapshot.CanonicalItems)
	snapshot.Strata = inventoryStrata(snapshot.CanonicalItems)
	snapshot.Fingerprint = Fingerprint(*snapshot)
	return nil
}

func ValidateInventorySnapshot(snapshot InventorySnapshot) error {
	fingerprint := snapshot.Fingerprint
	if fingerprint == "" {
		return ErrInvalidInventory
	}
	snapshot.Fingerprint = ""
	canonicalizeInventory(&snapshot)
	if err := validateInventoryFields(snapshot); err != nil {
		return err
	}
	if snapshot.OccurrenceCount != len(snapshot.URLOccurrences) || snapshot.CanonicalCount != len(snapshot.CanonicalItems) || !equalStrata(snapshot.Strata, inventoryStrata(snapshot.CanonicalItems)) {
		return ErrInvalidInventory
	}
	if fingerprint != Fingerprint(snapshot) {
		return fmt.Errorf("%w: fingerprint mismatch", ErrInvalidInventory)
	}
	return nil
}

func canonicalizeInventory(snapshot *InventorySnapshot) {
	for index := range snapshot.SourceRecords {
		sort.Strings(snapshot.SourceRecords[index].URLOccurrenceIDs)
	}
	for index := range snapshot.CanonicalItems {
		sort.Strings(snapshot.CanonicalItems[index].OccurrenceIDs)
	}
	sort.Slice(snapshot.SourceRecords, func(i, j int) bool {
		return snapshot.SourceRecords[i].SourceRecordID < snapshot.SourceRecords[j].SourceRecordID
	})
	sort.Slice(snapshot.URLOccurrences, func(i, j int) bool {
		return snapshot.URLOccurrences[i].URLOccurrenceID < snapshot.URLOccurrences[j].URLOccurrenceID
	})
	sort.Slice(snapshot.CanonicalItems, func(i, j int) bool {
		return snapshot.CanonicalItems[i].CanonicalItemID < snapshot.CanonicalItems[j].CanonicalItemID
	})
	sort.Slice(snapshot.Completeness, func(i, j int) bool { return snapshot.Completeness[i].Name < snapshot.Completeness[j].Name })
}

func validateInventoryFields(snapshot InventorySnapshot) error {
	if snapshot.SchemaVersion != InventorySchemaVersion || strings.TrimSpace(snapshot.SourceIdentity) == "" || strings.TrimSpace(snapshot.Watermark) == "" {
		return ErrInvalidInventory
	}
	records := map[string]SourceRecord{}
	recordOccurrenceRefs := map[string]int{}
	for _, record := range snapshot.SourceRecords {
		if record.SourceRecordID == "" || record.NativeMessageID == "" || record.NativeTimestamp == "" || record.ContentFingerprint == "" {
			return ErrInvalidInventory
		}
		if _, exists := records[record.SourceRecordID]; exists {
			return ErrInvalidInventory
		}
		records[record.SourceRecordID] = record
		seen := map[string]bool{}
		for _, occurrenceID := range record.URLOccurrenceIDs {
			if occurrenceID == "" || seen[occurrenceID] {
				return ErrInvalidInventory
			}
			seen[occurrenceID] = true
			recordOccurrenceRefs[occurrenceID]++
		}
	}
	items := map[string]InventoryItem{}
	itemOccurrenceRefs := map[string]int{}
	for _, item := range snapshot.CanonicalItems {
		if item.CanonicalItemID == "" || item.RetrievalStrategyID == "" || item.FormatVariant == "" || len(item.OccurrenceIDs) == 0 {
			return ErrInvalidInventory
		}
		if item.AccessState == "sensitive_redacted" {
			if item.CanonicalURL != "" || item.RetrievalStrategyID != "manual_support" || item.FormatVariant != "sensitive_redacted" {
				return ErrInvalidInventory
			}
		} else if item.AccessState != "" || item.CanonicalURL == "" {
			return ErrInvalidInventory
		}
		if _, exists := items[item.CanonicalItemID]; exists {
			return ErrInvalidInventory
		}
		items[item.CanonicalItemID] = item
		seen := map[string]bool{}
		for _, occurrenceID := range item.OccurrenceIDs {
			if occurrenceID == "" || seen[occurrenceID] {
				return ErrInvalidInventory
			}
			seen[occurrenceID] = true
			itemOccurrenceRefs[occurrenceID]++
		}
	}
	occurrences := map[string]URLOccurrence{}
	sourceOrdinals := map[string]bool{}
	for _, occurrence := range snapshot.URLOccurrences {
		ordinalKey := fmt.Sprintf("%s\x00%d", occurrence.SourceRecordID, occurrence.SourceOrdinal)
		if occurrence.SourceOrdinal < 0 || sourceOrdinals[ordinalKey] || occurrence.URLOccurrenceID == "" || occurrence.SourceRecordID == "" || occurrence.CanonicalItemID == "" {
			return ErrInvalidInventory
		}
		item := items[occurrence.CanonicalItemID]
		sensitiveOccurrence := occurrence.SanitizationState == "sensitive_redacted"
		sensitiveItem := item.AccessState == "sensitive_redacted"
		if sensitiveOccurrence != sensitiveItem {
			return ErrInvalidInventory
		}
		if sensitiveOccurrence {
			if occurrence.ObservedURL != "" {
				return ErrInvalidInventory
			}
		} else if occurrence.ObservedURL == "" || occurrence.SanitizationState != "" && occurrence.SanitizationState != "non_semantic_components_removed" {
			return ErrInvalidInventory
		}
		if _, exists := occurrences[occurrence.URLOccurrenceID]; exists {
			return ErrInvalidInventory
		}
		if _, exists := records[occurrence.SourceRecordID]; !exists {
			return ErrInvalidInventory
		}
		if _, exists := items[occurrence.CanonicalItemID]; !exists {
			return ErrInvalidInventory
		}
		sourceOrdinals[ordinalKey] = true
		occurrences[occurrence.URLOccurrenceID] = occurrence
	}
	for occurrenceID := range occurrences {
		if recordOccurrenceRefs[occurrenceID] != 1 || itemOccurrenceRefs[occurrenceID] != 1 {
			return ErrInvalidInventory
		}
	}
	if len(recordOccurrenceRefs) != len(occurrences) || len(itemOccurrenceRefs) != len(occurrences) {
		return ErrInvalidInventory
	}
	return nil
}

func inventoryStrata(items []InventoryItem) []StratumCount {
	counts := map[string]int{}
	parts := map[string][2]string{}
	for _, item := range items {
		key := item.RetrievalStrategyID + "\x00" + item.FormatVariant
		counts[key]++
		parts[key] = [2]string{item.RetrievalStrategyID, item.FormatVariant}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]StratumCount, 0, len(keys))
	for _, key := range keys {
		result = append(result, StratumCount{RetrievalStrategyID: parts[key][0], FormatVariant: parts[key][1], CanonicalCount: counts[key]})
	}
	return result
}

func equalStrata(left, right []StratumCount) bool {
	leftData, _ := json.Marshal(left)
	rightData, _ := json.Marshal(right)
	return string(leftData) == string(rightData)
}

func cloneStringsMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
