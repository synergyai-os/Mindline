package productbrain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	DeliveryApprovalSchema              = "productbrain-delivery-approval/v0.1"
	HumanInitiationEvidenceSchema       = "productbrain-human-initiation-evidence/v0.1"
	ApprovedDeliveryAuthoritySchema     = "productbrain-approved-delivery-authority/v0.2"
	ApprovedDeliveryRunSchema           = "productbrain-approved-delivery-run/v0.2"
	ApprovedDeliveryStateSchema         = "productbrain-approved-delivery-state/v0.2"
	ApprovedDeliveryHistorySchema       = "productbrain-approved-delivery-history/v0.2"
	ApprovedDeliveryCancellationSchema  = "productbrain-approved-delivery-cancellation/v0.2"
	ApprovedDeliveryReceiptSchema       = "productbrain-approved-delivery-receipt/v0.2"
	approvedDeliveryAuthorityDir        = "approved-delivery-v0.2"
	maximumApprovedAttemptsPerOperation = 4
)

var (
	ErrApprovedDeliveryCancelled = errors.New("approved_delivery_cancelled")
	ErrApprovedDeliveryAmbiguous = errors.New("approved_delivery_ambiguous")
	ErrApprovalExpired           = errors.New("approval_expired")
	ErrAttemptBudgetExhausted    = errors.New("mutation_attempt_budget_exhausted")
	ErrWriteBudgetExhausted      = errors.New("destination_write_budget_exhausted")
)

// HumanInitiationEvidence is produced by the authenticated browser ceremony.
// Product Brain accepts it only when a verifier confirms and consumes the
// server-side one-time capability; the booleans and hashes are not authority by
// themselves.
type HumanInitiationEvidence struct {
	SchemaVersion                string   `json:"schema_version"`
	Fingerprint                  string   `json:"fingerprint"`
	SessionFingerprint           string   `json:"session_fingerprint"`
	ReviewNonceFingerprint       string   `json:"review_nonce_fingerprint"`
	PreviewEvidenceFingerprint   string   `json:"preview_evidence_fingerprint"`
	BatchFingerprint             string   `json:"batch_fingerprint"`
	DestinationWorkspaceID       string   `json:"destination_workspace_id"`
	DestinationKeyID             string   `json:"destination_key_id"`
	OrderedOperationFingerprints []string `json:"ordered_operation_fingerprints"`
	MaximumDestinationWrites     int      `json:"maximum_destination_writes"`
	MaximumMutationAttempts      int      `json:"maximum_mutation_attempts"`
	IssuedAt                     string   `json:"issued_at"`
	ExpiresAt                    string   `json:"expires_at"`
	HumanGesture                 bool     `json:"human_gesture"`
	ServerDerived                bool     `json:"server_derived"`
}

type DeliveryApproval struct {
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

type ApprovedBatch struct {
	BatchFingerprint        string
	Outbox                  Outbox
	Profile                 DeliveryProfile
	Preflight               PreflightArtifact
	PrivacyFingerprint      string
	Approval                DeliveryApproval
	HumanInitiationEvidence HumanInitiationEvidence
}

type ApprovedDeliveryAuthority struct {
	SchemaVersion           string                  `json:"schema_version"`
	Fingerprint             string                  `json:"fingerprint"`
	Approval                DeliveryApproval        `json:"approval"`
	HumanInitiationEvidence HumanInitiationEvidence `json:"human_initiation_evidence"`
}

type HumanInitiationVerifier interface {
	VerifyAndConsume(context.Context, HumanInitiationEvidence) error
}

type ApprovalRef struct {
	ApprovalFingerprint string `json:"approval_fingerprint"`
	BatchFingerprint    string `json:"batch_fingerprint"`
}

type CancellationReceipt struct {
	SchemaVersion       string `json:"schema_version"`
	Fingerprint         string `json:"fingerprint"`
	ApprovalFingerprint string `json:"approval_fingerprint"`
	BatchFingerprint    string `json:"batch_fingerprint"`
	CancelledAt         string `json:"cancelled_at"`
}

type ApprovedMutationAttempt struct {
	AttemptNumber        int    `json:"attempt_number"`
	AttemptID            string `json:"attempt_id"`
	RunSequence          int    `json:"run_sequence"`
	OperationID          string `json:"operation_id"`
	OperationFingerprint string `json:"operation_fingerprint"`
	ReservedAt           string `json:"reserved_at"`
	Outcome              string `json:"outcome"`
	ResponseReceived     bool   `json:"response_received"`
	MayHaveCommitted     bool   `json:"may_have_committed"`
	ReadbackFingerprint  string `json:"readback_fingerprint,omitempty"`
}

type ApprovedOperationState struct {
	OperationID          string                    `json:"operation_id"`
	Kind                 string                    `json:"kind"`
	OperationFingerprint string                    `json:"operation_fingerprint"`
	EntryID              string                    `json:"entry_id,omitempty"`
	State                string                    `json:"state"`
	UniqueWriteReserved  bool                      `json:"unique_write_reserved"`
	Attempts             []ApprovedMutationAttempt `json:"attempts"`
	EntryDocID           string                    `json:"entry_doc_id,omitempty"`
	RemoteObjectID       string                    `json:"remote_object_id,omitempty"`
	ReadbackFingerprint  string                    `json:"readback_fingerprint,omitempty"`
	SafeCategory         string                    `json:"safe_category,omitempty"`
}

type ApprovedDeliveryState struct {
	SchemaVersion                      string                   `json:"schema_version"`
	Fingerprint                        string                   `json:"fingerprint"`
	ApprovalFingerprint                string                   `json:"approval_fingerprint"`
	HumanInitiationEvidenceFingerprint string                   `json:"human_initiation_evidence_fingerprint"`
	BatchFingerprint                   string                   `json:"batch_fingerprint"`
	OutboxFingerprint                  string                   `json:"outbox_fingerprint"`
	ProfileFingerprint                 string                   `json:"profile_fingerprint"`
	PreflightFingerprint               string                   `json:"preflight_fingerprint"`
	PrivacyFingerprint                 string                   `json:"privacy_fingerprint"`
	DestinationWorkspaceID             string                   `json:"destination_workspace_id"`
	DestinationKeyID                   string                   `json:"destination_key_id"`
	OrderedOperationFingerprints       []string                 `json:"ordered_operation_fingerprints"`
	MaximumDestinationWrites           int                      `json:"maximum_destination_writes"`
	MaximumMutationAttempts            int                      `json:"maximum_mutation_attempts"`
	UniqueWriteReservations            int                      `json:"unique_write_reservations"`
	MutationAttempts                   int                      `json:"mutation_attempts"`
	LatestRunSequence                  int                      `json:"latest_run_sequence"`
	CancellationFingerprint            string                   `json:"cancellation_fingerprint,omitempty"`
	Status                             string                   `json:"status"`
	Operations                         []ApprovedOperationState `json:"operations"`
}

type ApprovedDeliveryRun struct {
	SchemaVersion       string   `json:"schema_version"`
	Fingerprint         string   `json:"fingerprint"`
	Sequence            int      `json:"sequence"`
	InvocationID        string   `json:"invocation_id"`
	ApprovalFingerprint string   `json:"approval_fingerprint"`
	BatchFingerprint    string   `json:"batch_fingerprint"`
	StartedAt           string   `json:"started_at"`
	EndedAt             string   `json:"ended_at,omitempty"`
	Outcome             string   `json:"outcome"`
	AttemptIDs          []string `json:"attempt_ids"`
}

type ApprovedDeliveryHistory struct {
	SchemaVersion       string                `json:"schema_version"`
	Fingerprint         string                `json:"fingerprint"`
	ApprovalFingerprint string                `json:"approval_fingerprint"`
	BatchFingerprint    string                `json:"batch_fingerprint"`
	RunRefs             []string              `json:"run_refs"`
	Runs                []ApprovedDeliveryRun `json:"runs"`
}

type ApprovedDeliveryReceipt struct {
	SchemaVersion           string   `json:"schema_version"`
	Fingerprint             string   `json:"fingerprint"`
	ApprovalFingerprint     string   `json:"approval_fingerprint"`
	BatchFingerprint        string   `json:"batch_fingerprint"`
	StateFingerprint        string   `json:"state_fingerprint"`
	HistoryFingerprint      string   `json:"history_fingerprint,omitempty"`
	Status                  string   `json:"status"`
	AcknowledgedOperations  int      `json:"acknowledged_operations"`
	UniqueWriteReservations int      `json:"unique_write_reservations"`
	MutationAttempts        int      `json:"mutation_attempts"`
	RemoteObjectIDs         []string `json:"remote_object_ids"`
	CancellationFingerprint string   `json:"cancellation_fingerprint,omitempty"`
}

type ApprovedDeliveryOptions struct {
	Now                     func() time.Time
	HumanInitiationVerifier HumanInitiationVerifier
	afterAuthoritySealed    func() error
	beforeAttemptOrdering   func() error
	afterAttemptReserved    func(ApprovedMutationAttempt) error
	afterMutation           func(ApprovedMutationAttempt) error
}

func DeliveryPrivacyFingerprint(outbox Outbox) string {
	return hashValue(struct {
		OutboxFingerprint string           `json:"outbox_fingerprint"`
		Findings          []PrivacyFinding `json:"privacy_findings"`
	}{outbox.Fingerprint, outbox.PrivacyFindings})
}

func OrderedDeliveryOperationFingerprints(outbox Outbox) []string {
	values := make([]string, 0, len(outbox.Operations))
	for _, operation := range outbox.Operations {
		values = append(values, operation.PayloadFingerprint)
	}
	return values
}

func DeliveryBatchFingerprint(outbox Outbox, preflight PreflightArtifact, privacyFingerprint string) string {
	return hashValue(struct {
		OutboxFingerprint            string   `json:"outbox_fingerprint"`
		PreflightFingerprint         string   `json:"preflight_fingerprint"`
		PrivacyFingerprint           string   `json:"privacy_fingerprint"`
		DestinationWorkspaceID       string   `json:"destination_workspace_id"`
		DestinationKeyID             string   `json:"destination_key_id"`
		OrderedOperationFingerprints []string `json:"ordered_operation_fingerprints"`
	}{outbox.Fingerprint, preflight.Fingerprint, privacyFingerprint, outbox.ProfileSnapshot.ExpectedWorkspaceID, outbox.ProfileSnapshot.ExpectedKeyID, OrderedDeliveryOperationFingerprints(outbox)})
}

func SealHumanInitiationEvidence(value HumanInitiationEvidence) HumanInitiationEvidence {
	value.Fingerprint = hashValue(value)
	return value
}

func SealDeliveryApproval(value DeliveryApproval) DeliveryApproval {
	value.Fingerprint = hashValue(value)
	return value
}

func DecodeDeliveryApproval(data []byte) (DeliveryApproval, error) {
	var value DeliveryApproval
	if err := decodeStrictApprovedJSON(data, &value); err != nil {
		return DeliveryApproval{}, err
	}
	if value.SchemaVersion != DeliveryApprovalSchema || value.Fingerprint != hashValue(value) {
		return DeliveryApproval{}, errors.New("invalid delivery approval")
	}
	return value, nil
}

func DecodeHumanInitiationEvidence(data []byte) (HumanInitiationEvidence, error) {
	var value HumanInitiationEvidence
	if err := decodeStrictApprovedJSON(data, &value); err != nil {
		return HumanInitiationEvidence{}, err
	}
	if value.SchemaVersion != HumanInitiationEvidenceSchema || value.Fingerprint != hashValue(value) {
		return HumanInitiationEvidence{}, errors.New("invalid human initiation evidence")
	}
	return value, nil
}

func decodeStrictApprovedJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("invalid trailing approved-delivery data")
	}
	return nil
}

func validateApprovedBatch(batch ApprovedBatch) error {
	if err := ValidateOutbox(batch.Outbox); err != nil {
		return err
	}
	if err := ValidateDeliveryProfile(batch.Profile); err != nil || batch.Outbox.ProfileFingerprint != hashValue(batch.Profile) {
		return errors.New("outbox_state_mismatch")
	}
	if err := ValidatePreflight(batch.Preflight, batch.Outbox, batch.Profile); err != nil {
		return err
	}
	privacyFingerprint := DeliveryPrivacyFingerprint(batch.Outbox)
	operationFingerprints := OrderedDeliveryOperationFingerprints(batch.Outbox)
	expectedBatchFingerprint := DeliveryBatchFingerprint(batch.Outbox, batch.Preflight, privacyFingerprint)
	approval, evidence := batch.Approval, batch.HumanInitiationEvidence
	if batch.PrivacyFingerprint != privacyFingerprint || batch.BatchFingerprint != expectedBatchFingerprint || approval.SchemaVersion != DeliveryApprovalSchema || approval.Fingerprint != hashValue(approval) || evidence.SchemaVersion != HumanInitiationEvidenceSchema || evidence.Fingerprint != hashValue(evidence) {
		return errors.New("approval_fingerprint_mismatch")
	}
	if approval.BatchFingerprint != expectedBatchFingerprint || approval.OutboxFingerprint != batch.Outbox.Fingerprint || approval.PreflightFingerprint != batch.Preflight.Fingerprint || approval.PrivacyFingerprint != privacyFingerprint || approval.DestinationWorkspaceID != batch.Profile.Workspace.ExpectedID || approval.DestinationKeyID != batch.Profile.Credential.ExpectedKeyID || approval.HumanInitiationEvidenceFingerprint != evidence.Fingerprint {
		return errors.New("approval_binding_mismatch")
	}
	if !equalStrings(approval.OrderedOperationFingerprints, operationFingerprints) || approval.MaximumDestinationWrites != len(batch.Outbox.Operations) || approval.MaximumMutationAttempts < len(batch.Outbox.Operations) || approval.MaximumMutationAttempts > len(batch.Outbox.Operations)*maximumApprovedAttemptsPerOperation {
		return errors.New("approval_budget_mismatch")
	}
	if evidence.BatchFingerprint != expectedBatchFingerprint || evidence.DestinationWorkspaceID != approval.DestinationWorkspaceID || evidence.DestinationKeyID != approval.DestinationKeyID || !equalStrings(evidence.OrderedOperationFingerprints, operationFingerprints) || evidence.MaximumDestinationWrites != approval.MaximumDestinationWrites || evidence.MaximumMutationAttempts != approval.MaximumMutationAttempts || evidence.ExpiresAt != approval.ExpiresAt {
		return errors.New("human_initiation_binding_mismatch")
	}
	if strings.TrimSpace(evidence.SessionFingerprint) == "" || len(evidence.ReviewNonceFingerprint) != 64 || strings.TrimSpace(evidence.PreviewEvidenceFingerprint) == "" || !evidence.HumanGesture || !evidence.ServerDerived {
		return errors.New("invalid human initiation evidence")
	}
	approvedAt, err := time.Parse(time.RFC3339Nano, approval.ApprovedAt)
	if err != nil {
		return errors.New("invalid approval time")
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, evidence.IssuedAt)
	if err != nil {
		return errors.New("invalid human initiation time")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, approval.ExpiresAt)
	if err != nil || issuedAt.After(approvedAt) || approvedAt.After(expiresAt) || approvedAt.IsZero() || expiresAt.IsZero() {
		return errors.New("invalid approval time")
	}
	return nil
}

func approvalValidAt(approval DeliveryApproval, now time.Time) error {
	approvedAt, err := time.Parse(time.RFC3339Nano, approval.ApprovedAt)
	if err != nil || now.UTC().Before(approvedAt.UTC()) {
		return errors.New("approval_not_active")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, approval.ExpiresAt)
	if err != nil || !now.UTC().Before(expiresAt.UTC()) {
		return ErrApprovalExpired
	}
	return nil
}
