package recallproof

import (
	"errors"
	"fmt"
)

const (
	AuthorityReceiptSchema = "mindline-recall-pre-live-receipt/v0.1"
	LifecycleReceiptSchema = "mindline-recall-lifecycle-receipt/v0.1"
)

// AdoptionReceipt is the structural, per-unit reconciliation result. It never
// carries native identities, content, URLs, or provider values.
type AdoptionReceipt struct {
	DeliveredNative          int `json:"delivered_native"`
	CanonicalDeclared        int `json:"canonical_declared"`
	StructuralExcluded       int `json:"structural_excluded"`
	CanonicalReceiptDeclared int `json:"canonical_receipt_declared"`
}

func (receipt AdoptionReceipt) Validate() error {
	if receipt.DeliveredNative < 0 || receipt.CanonicalDeclared < 0 || receipt.StructuralExcluded < 0 || receipt.CanonicalReceiptDeclared < 0 {
		return errors.New("adoption receipt counts must be non-negative")
	}
	if receipt.DeliveredNative != receipt.CanonicalDeclared+receipt.StructuralExcluded {
		return errors.New("adoption receipt equation does not reconcile")
	}
	if receipt.CanonicalReceiptDeclared != receipt.CanonicalDeclared {
		return errors.New("canonical import receipt does not match canonical declared")
	}
	return nil
}

// AuthorityReceipt binds the reusable proof and live configuration to the
// exact source tree approved before private connector acquisition.
type AuthorityReceipt struct {
	SchemaVersion            string            `json:"schema_version"`
	Binding                  TreeConfigBinding `json:"binding"`
	ReusableProofFingerprint string            `json:"reusable_proof_fingerprint"`
}

func (receipt AuthorityReceipt) Validate() error {
	if receipt.SchemaVersion != AuthorityReceiptSchema {
		return fmt.Errorf("unsupported authority receipt schema: %s", receipt.SchemaVersion)
	}
	if err := receipt.Binding.Validate(); err != nil {
		return err
	}
	if !fingerprintPattern.MatchString(receipt.ReusableProofFingerprint) {
		return errors.New("authority receipt proof fingerprint must be a sha256 commitment")
	}
	return nil
}

func RequireAuthorityReceiptBinding(receipt AuthorityReceipt, binding TreeConfigBinding) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if err := binding.Validate(); err != nil {
		return err
	}
	if receipt.Binding != binding {
		return errors.New("tree/config binding differs from pre-live authority receipt")
	}
	return nil
}

func AuthorityReceiptFromPreLive(receipt PhaseReceipt) (AuthorityReceipt, error) {
	if err := receipt.Validate(); err != nil {
		return AuthorityReceipt{}, err
	}
	if receipt.Phase != "pre_live" {
		return AuthorityReceipt{}, errors.New("authority receipt requires a pre-live phase")
	}
	fingerprint, err := DeterministicArtifactFingerprint(receipt.Artifact)
	if err != nil {
		return AuthorityReceipt{}, err
	}
	authority := AuthorityReceipt{
		SchemaVersion: AuthorityReceiptSchema, Binding: receipt.Binding,
		ReusableProofFingerprint: fingerprint,
	}
	return authority, authority.Validate()
}

// LifecycleGates is deliberately structural: a passing receipt proves only
// that named executable gates passed, never a broader private-data claim.
type LifecycleGates struct {
	WholeEnvelopeValidated            bool `json:"whole_envelope_validated"`
	StrictFramesValidatedWhole        bool `json:"strict_frames_validated_whole"`
	ConflictFailsBeforeImport         bool `json:"conflict_fails_before_import"`
	ConflictPreservesPriorFingerprint bool `json:"conflict_preserves_prior_fingerprint"`
	ReplayHasNoDuplicateEffects       bool `json:"replay_has_no_duplicate_effects"`
	RestartMatchesUninterruptedOracle bool `json:"restart_matches_uninterrupted_oracle"`
	RevisionAndTombstonePreserved     bool `json:"revision_and_tombstone_preserved"`
	CapacityPreservesPriorFingerprint bool `json:"capacity_preserves_prior_fingerprint"`
	TerminalResourcesAndQueueRebuild  bool `json:"terminal_resources_and_queue_rebuild"`
	LegacyAndCompactCompatible        bool `json:"legacy_and_compact_compatible"`
	RollbackPreservesCanonicalState   bool `json:"rollback_preserves_canonical_state"`
	PrivacyOutputStructuralOnly       bool `json:"privacy_output_structural_only"`
}

func (gates LifecycleGates) Validate() error {
	if !gates.WholeEnvelopeValidated || !gates.StrictFramesValidatedWhole || !gates.ConflictFailsBeforeImport || !gates.ConflictPreservesPriorFingerprint || !gates.ReplayHasNoDuplicateEffects || !gates.RestartMatchesUninterruptedOracle || !gates.RevisionAndTombstonePreserved || !gates.CapacityPreservesPriorFingerprint || !gates.TerminalResourcesAndQueueRebuild || !gates.LegacyAndCompactCompatible || !gates.RollbackPreservesCanonicalState || !gates.PrivacyOutputStructuralOnly {
		return errors.New("one or more required lifecycle gates did not pass")
	}
	return nil
}

// ReconciliationSummary keeps the denominator executable without retaining
// native identities. Overlap may be non-zero, but it must never remove an
// otherwise delivered identity from adoption.
type ReconciliationSummary struct {
	UniqueNative         int `json:"unique_native"`
	Retained             int `json:"retained"`
	Excluded             int `json:"excluded"`
	Withheld             int `json:"withheld"`
	UserAuthoredExcluded int `json:"user_authored_excluded"`
	Overlap              int `json:"overlap"`
	Gap                  int `json:"gap"`
	UnresolvedThread     int `json:"unresolved_thread"`
}

func (summary ReconciliationSummary) Validate() error {
	for _, count := range []int{summary.UniqueNative, summary.Retained, summary.Excluded, summary.Withheld, summary.UserAuthoredExcluded, summary.Overlap, summary.Gap, summary.UnresolvedThread} {
		if count < 0 {
			return errors.New("reconciliation counts must be non-negative")
		}
	}
	if summary.UniqueNative != summary.Retained+summary.Excluded+summary.Withheld {
		return errors.New("native denominator does not reconcile")
	}
	if summary.UserAuthoredExcluded != 0 {
		return errors.New("user-authored identities may not be excluded")
	}
	if summary.Gap != 0 || summary.UnresolvedThread != 0 {
		return errors.New("reconciliation contains an unresolved gap or thread")
	}
	return nil
}

// PreservedFingerprint proves that a rejected conflict or capacity transition
// made no canonical mutation. It contains commitments only.
type PreservedFingerprint struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

func (proof PreservedFingerprint) Validate() error {
	if !fingerprintPattern.MatchString(proof.Before) || !fingerprintPattern.MatchString(proof.After) {
		return errors.New("preserved fingerprint proof requires sha256 commitments")
	}
	if proof.Before != proof.After {
		return errors.New("rejected transition changed the canonical fingerprint")
	}
	return nil
}

// ZeroEffects is the exact replay result for the completed frozen source.
type ZeroEffects struct {
	Records   int `json:"records"`
	Revisions int `json:"revisions"`
	Resources int `json:"resources"`
	Feedback  int `json:"feedback"`
}

func (effects ZeroEffects) Validate() error {
	if effects.Records != 0 || effects.Revisions != 0 || effects.Resources != 0 || effects.Feedback != 0 {
		return errors.New("replay introduced a duplicate effect")
	}
	return nil
}

// QueueRebuildReadback requires the derived queue to be disposable without
// changing canonical state or its compact/explicit-get consumer surfaces.
type QueueRebuildReadback struct {
	CanonicalBefore string `json:"canonical_before"`
	CanonicalAfter  string `json:"canonical_after"`
	CompactBefore   string `json:"compact_before"`
	CompactAfter    string `json:"compact_after"`
	GetBefore       string `json:"get_before"`
	GetAfter        string `json:"get_after"`
	AllTerminal     bool   `json:"all_terminal"`
}

func (readback QueueRebuildReadback) Validate() error {
	for _, pair := range [][2]string{{readback.CanonicalBefore, readback.CanonicalAfter}, {readback.CompactBefore, readback.CompactAfter}, {readback.GetBefore, readback.GetAfter}} {
		if !fingerprintPattern.MatchString(pair[0]) || !fingerprintPattern.MatchString(pair[1]) || pair[0] != pair[1] {
			return errors.New("queue rebuild changed canonical or consumer readback")
		}
	}
	if !readback.AllTerminal {
		return errors.New("resource queue contains a non-terminal resource")
	}
	return nil
}

// LifecycleReceipt composes all reusable proof requirements into a
// content-free artifact that can be bound to the pre-live authority receipt.
type LifecycleReceipt struct {
	SchemaVersion       string                `json:"schema_version"`
	Binding             TreeConfigBinding     `json:"binding"`
	AdoptionReceipts    []AdoptionReceipt     `json:"adoption_receipts"`
	Reconciliation      ReconciliationSummary `json:"reconciliation"`
	ConflictFingerprint PreservedFingerprint  `json:"conflict_fingerprint"`
	CapacityFingerprint PreservedFingerprint  `json:"capacity_fingerprint"`
	ReplayEffects       ZeroEffects           `json:"replay_effects"`
	QueueRebuild        QueueRebuildReadback  `json:"queue_rebuild"`
	Gates               LifecycleGates        `json:"gates"`
}

func (receipt LifecycleReceipt) Validate() error {
	if receipt.SchemaVersion != LifecycleReceiptSchema {
		return fmt.Errorf("unsupported lifecycle receipt schema: %s", receipt.SchemaVersion)
	}
	if err := receipt.Binding.Validate(); err != nil {
		return err
	}
	if len(receipt.AdoptionReceipts) == 0 {
		return errors.New("lifecycle receipt requires at least one adoption receipt")
	}
	for _, adoption := range receipt.AdoptionReceipts {
		if err := adoption.Validate(); err != nil {
			return err
		}
	}
	if err := receipt.Reconciliation.Validate(); err != nil {
		return err
	}
	if err := receipt.ConflictFingerprint.Validate(); err != nil {
		return err
	}
	if err := receipt.CapacityFingerprint.Validate(); err != nil {
		return err
	}
	if err := receipt.ReplayEffects.Validate(); err != nil {
		return err
	}
	if err := receipt.QueueRebuild.Validate(); err != nil {
		return err
	}
	return receipt.Gates.Validate()
}
