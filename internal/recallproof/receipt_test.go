package recallproof

import (
	"testing"
)

func TestAdoptionReceiptRequiresSignedEquationAndCanonicalReceipt(t *testing.T) {
	receipt := AdoptionReceipt{DeliveredNative: 11, CanonicalDeclared: 10, StructuralExcluded: 1, CanonicalReceiptDeclared: 10}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	receipt.CanonicalReceiptDeclared = 9
	if err := receipt.Validate(); err == nil {
		t.Fatal("Validate accepted a mismatched canonical receipt")
	}
}

func TestLifecycleReceiptRequiresEveryReusableGate(t *testing.T) {
	binding := testBinding()
	receipt := LifecycleReceipt{
		SchemaVersion:       LifecycleReceiptSchema,
		Binding:             binding,
		AdoptionReceipts:    []AdoptionReceipt{{DeliveredNative: 11, CanonicalDeclared: 10, StructuralExcluded: 1, CanonicalReceiptDeclared: 10}},
		Reconciliation:      ReconciliationSummary{UniqueNative: 11, Retained: 9, Excluded: 1, Withheld: 1},
		ConflictFingerprint: preservedFingerprint(),
		CapacityFingerprint: preservedFingerprint(),
		ReplayEffects:       ZeroEffects{},
		QueueRebuild:        queueRebuildReadback(),
		Gates:               passingLifecycleGates(),
	}
	if err := receipt.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	receipt.Gates.ConflictPreservesPriorFingerprint = false
	if err := receipt.Validate(); err == nil {
		t.Fatal("Validate accepted a conflict proof without fingerprint preservation")
	}
}

func TestLifecycleReceiptRejectsDenominatorAndQueueRebuildDrift(t *testing.T) {
	receipt := LifecycleReceipt{
		SchemaVersion:       LifecycleReceiptSchema,
		Binding:             testBinding(),
		AdoptionReceipts:    []AdoptionReceipt{{DeliveredNative: 11, CanonicalDeclared: 10, StructuralExcluded: 1, CanonicalReceiptDeclared: 10}},
		Reconciliation:      ReconciliationSummary{UniqueNative: 11, Retained: 9, Excluded: 1, Withheld: 1},
		ConflictFingerprint: preservedFingerprint(),
		CapacityFingerprint: preservedFingerprint(),
		ReplayEffects:       ZeroEffects{},
		QueueRebuild:        queueRebuildReadback(),
		Gates:               passingLifecycleGates(),
	}
	receipt.Reconciliation.UserAuthoredExcluded = 1
	if err := receipt.Validate(); err == nil {
		t.Fatal("Validate accepted a user-authored exclusion")
	}
	receipt.Reconciliation.UserAuthoredExcluded = 0
	receipt.QueueRebuild.CompactAfter = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	if err := receipt.Validate(); err == nil {
		t.Fatal("Validate accepted queue rebuild compact readback drift")
	}
}

func TestAuthorityReceiptBindsExactTreeAndConfiguration(t *testing.T) {
	binding := testBinding()
	receipt := AuthorityReceipt{SchemaVersion: AuthorityReceiptSchema, Binding: binding, ReusableProofFingerprint: binding.TreeFingerprint}
	if err := RequireAuthorityReceiptBinding(receipt, binding); err != nil {
		t.Fatalf("RequireAuthorityReceiptBinding: %v", err)
	}
	drift := binding
	drift.LiveConfigurationFingerprint = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := RequireAuthorityReceiptBinding(receipt, drift); err == nil {
		t.Fatal("RequireAuthorityReceiptBinding accepted configuration drift")
	}
}

func TestAuthorityReceiptCanOnlyBeMintedFromPassingPreLivePhase(t *testing.T) {
	binding := testBinding()
	artifact := validPreLiveArtifact(t)
	receipt, err := AuthorityReceiptFromPreLive(PhaseReceipt{
		SchemaVersion: PhaseReceiptSchema, Phase: "pre_live", Binding: binding, Artifact: artifact,
	})
	if err != nil || receipt.Binding != binding || receipt.ReusableProofFingerprint == "" {
		t.Fatalf("mint pre-live authority = %#v, %v", receipt, err)
	}
	if _, err := AuthorityReceiptFromPreLive(PhaseReceipt{
		SchemaVersion: PhaseReceiptSchema, Phase: "live", Binding: binding, Artifact: artifact,
	}); err == nil {
		t.Fatal("live receipt minted pre-live authority")
	}
}

func passingLifecycleGates() LifecycleGates {
	return LifecycleGates{
		WholeEnvelopeValidated: true, StrictFramesValidatedWhole: true,
		ConflictFailsBeforeImport: true, ConflictPreservesPriorFingerprint: true,
		ReplayHasNoDuplicateEffects: true, RestartMatchesUninterruptedOracle: true,
		RevisionAndTombstonePreserved: true, CapacityPreservesPriorFingerprint: true,
		TerminalResourcesAndQueueRebuild: true, LegacyAndCompactCompatible: true,
		RollbackPreservesCanonicalState: true, PrivacyOutputStructuralOnly: true,
	}
}

func preservedFingerprint() PreservedFingerprint {
	return PreservedFingerprint{Before: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", After: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
}

func queueRebuildReadback() QueueRebuildReadback {
	return QueueRebuildReadback{
		CanonicalBefore: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CanonicalAfter: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CompactBefore: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", CompactAfter: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		GetBefore: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", GetAfter: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", AllTerminal: true,
	}
}
