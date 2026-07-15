package activationapp

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/controlui"
	"github.com/synergyai-os/Mindline/internal/integrations"
	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/processing"
	"github.com/synergyai-os/Mindline/internal/processing/routingcompat"
	"github.com/synergyai-os/Mindline/internal/productbrain"
	"github.com/synergyai-os/Mindline/internal/retrieval"
	"github.com/synergyai-os/Mindline/internal/routing"
)

func (app *App) validatePersistedState(state persistedState) error {
	if state.SchemaVersion != StateSchemaVersion {
		return errors.New("activation state requires rebuild after STD-20")
	}
	if state.KnownSource != nil {
		if err := validateConnectionSnapshot(*state.KnownSource, "slack", integrations.ConnectionSlackWebAPI); err != nil {
			return err
		}
	}
	if state.KnownDestination != nil {
		if err := validateConnectionSnapshot(*state.KnownDestination, "product_brain", integrations.ConnectionProductBrain); err != nil {
			return err
		}
	}
	if state.SlackDrainWindow != nil {
		if err := validateSlackDrainWindow(*state.SlackDrainWindow); err != nil {
			return err
		}
		if state.KnownSource == nil || state.KnownSource.Identity.WorkspaceID != state.SlackDrainWindow.WorkspaceID || state.KnownSource.Identity.ChannelID != state.SlackDrainWindow.ChannelID {
			return errors.New("persisted Slack drain window identity mismatch")
		}
	}
	seenAuthorizations := map[string]bool{}
	for _, fingerprint := range state.PreLiveAuthorizations {
		if fingerprint == "" || seenAuthorizations[fingerprint] {
			return errors.New("invalid pre-live authorization history")
		}
		seenAuthorizations[fingerprint] = true
	}
	if state.PreLiveReceipt != "" && !seenAuthorizations[state.PreLiveReceipt] {
		return errors.New("current pre-live authorization is not in append-only history")
	}
	if state.Inventory != nil {
		if err := acquisition.ValidateInventory(*state.Inventory); err != nil {
			return err
		}
	}
	if state.SourceScope != nil {
		if err := acquisition.ValidateSourceScope(*state.SourceScope); err != nil {
			return err
		}
	}
	if state.Strategy != nil {
		if err := processing.ValidateStrategy(*state.Strategy); err != nil {
			return err
		}
	}
	if state.DrainPolicy != nil {
		if err := validateDrainPolicy(*state.DrainPolicy); err != nil {
			return err
		}
	}
	if state.Plan != nil {
		if err := orchestration.ValidateRunPlan(*state.Plan); err != nil {
			return err
		}
		if state.Inventory == nil || state.Strategy == nil || state.DrainPolicy == nil || state.Plan.StrategyFingerprint != state.Strategy.Fingerprint {
			return errors.New("persisted run plan binding mismatch")
		}
		policy := state.DrainPolicy
		if state.Plan.Budgets.MaximumNetworkRequests != policy.MaximumNetworkRequests || state.Plan.Budgets.MaximumWallTimeSeconds != policy.MaximumWallTimeSeconds || state.Plan.Budgets.MaximumCostMicrounits != policy.MaximumCostMicrounits || state.Plan.Budgets.MaximumRetryAttempts != policy.MaximumRetryAttempts || state.Plan.Budgets.ManualSupportTolerance != policy.ManualSupportTolerance {
			return errors.New("persisted run plan drain policy mismatch")
		}
	}
	if state.Sample != nil {
		if state.Inventory == nil {
			return errors.New("persisted sample lacks inventory")
		}
		inventory, err := toOrchestrationInventory(*state.Inventory)
		if err != nil {
			return err
		}
		if err := orchestration.ValidateSampleManifest(inventory, *state.Sample); err != nil {
			return err
		}
		if state.Plan == nil || state.Plan.InventoryFingerprint != inventory.Fingerprint {
			return errors.New("persisted inventory binding mismatch")
		}
	}
	if state.Queue != nil {
		if state.Inventory == nil || state.Sample == nil {
			return errors.New("persisted queue lacks frozen inventory authority")
		}
		if state.Route == nil {
			if err := validateQueueProjection(*state.Queue, *state.Inventory, *state.Sample); err != nil {
				return err
			}
		} else if err := validateReviewedQueueProjection(*state.Queue, *state.Inventory, *state.Sample); err != nil {
			return err
		}
	}
	if state.ProofInventory != nil {
		if err := acquisition.ValidateInventory(*state.ProofInventory); err != nil {
			return err
		}
	}
	for _, artifact := range state.Retrieval {
		if err := retrieval.ValidateArtifact(artifact); err != nil {
			return err
		}
	}
	proposals := map[string]processing.Proposal{}
	for _, proposal := range state.Proposals {
		if proposals[proposal.CanonicalItemID].CanonicalItemID != "" {
			return errors.New("duplicate persisted proposal")
		}
		if err := processing.ValidateProposal(proposal); err != nil {
			return err
		}
		proposals[proposal.CanonicalItemID] = proposal
	}
	reviewed := map[string]bool{}
	for _, review := range state.Reviews {
		proposal, ok := proposals[review.CanonicalItemID]
		if !ok || reviewed[review.CanonicalItemID] {
			return errors.New("persisted review coverage mismatch")
		}
		if err := processing.ValidateOperatorReview(review, proposal); err != nil {
			return err
		}
		reviewed[review.CanonicalItemID] = true
	}
	if state.Route != nil {
		if err := routing.ValidateResult(*state.Route); err != nil {
			return err
		}
		if state.ProofInventory == nil || state.Strategy == nil || len(state.Reviews) != len(state.Proposals) {
			return errors.New("persisted route lacks complete review authority")
		}
		recompiled, err := routingcompat.CompileReviewed(routingcompat.Input{Inventory: *state.ProofInventory, Retrieval: state.Retrieval, Strategy: *state.Strategy, Proposals: state.Proposals, Reviews: state.Reviews})
		if err != nil || !canonicalEqual(recompiled, *state.Route) {
			return errors.New("persisted route does not match reviewed authority")
		}
	}
	if state.Outbox != nil {
		if err := productbrain.ValidateOutbox(*state.Outbox); err != nil {
			return err
		}
		if state.Preview != nil {
			if err := validatePersistedPreview(*state.Preview, *state.Outbox, state.Preflight); err != nil {
				return err
			}
		}
		profile := productbrain.DeliveryProfileFromSnapshot(state.Outbox.ProfileSnapshot)
		if state.Preflight != nil {
			if err := productbrain.ValidatePreflight(*state.Preflight, *state.Outbox, profile); err != nil {
				return err
			}
		}
	}
	if state.Approval != nil {
		data, _ := json.Marshal(state.Approval)
		if _, err := productbrain.DecodeDeliveryApproval(data); err != nil {
			return err
		}
	}
	if state.HumanEvidence != nil {
		data, _ := json.Marshal(state.HumanEvidence)
		if _, err := productbrain.DecodeHumanInitiationEvidence(data); err != nil {
			return err
		}
	}
	if state.Delivery != nil {
		authoritative, err := productbrain.ReadApprovedDeliveryReceipt(filepath.Join(app.runtimeRoot, "product-brain"))
		if err != nil || !canonicalEqual(state.Delivery, &authoritative) {
			return errors.New("persisted delivery receipt is not authoritative")
		}
	}
	if state.Cancellation != nil {
		authoritative, err := productbrain.ReadApprovedCancellation(filepath.Join(app.runtimeRoot, "product-brain"))
		if err != nil || !canonicalEqual(state.Cancellation, &authoritative) {
			return errors.New("persisted cancellation receipt is not authoritative")
		}
	}
	if state.ZeroDelivery != nil {
		fingerprint := state.ZeroDelivery.Fingerprint
		copy := *state.ZeroDelivery
		copy.Fingerprint = ""
		if fingerprint == "" || fingerprint != acquisition.Fingerprint(copy) || copy.Status != "zero_draft_activation_recorded" {
			return errors.New("invalid zero-delivery receipt")
		}
	}
	if state.FounderReview != nil {
		fingerprint := state.FounderReview.Fingerprint
		copy := *state.FounderReview
		copy.Fingerprint = ""
		if fingerprint == "" || fingerprint != acquisition.Fingerprint(copy) {
			return errors.New("invalid founder review")
		}
	}
	if state.RecoveryProof != nil {
		fingerprint := state.RecoveryProof.Fingerprint
		copyProof := *state.RecoveryProof
		copyProof.Fingerprint = ""
		if fingerprint == "" || fingerprint != acquisition.Fingerprint(copyProof) || copyProof.SchemaVersion != RecoveryProofSchema || copyProof.RunID != state.RunID || state.Inventory == nil || state.Sample == nil || state.Queue == nil || copyProof.InventoryFingerprint != state.Inventory.Fingerprint || copyProof.SampleFingerprint != state.Sample.Fingerprint || copyProof.QueueFingerprint != state.Queue.Fingerprint {
			return errors.New("invalid run recovery proof")
		}
	}
	if state.DrainConfirmation != nil {
		fingerprint := state.DrainConfirmation.Fingerprint
		copyConfirmation := *state.DrainConfirmation
		copyConfirmation.Fingerprint = ""
		if fingerprint == "" || fingerprint != acquisition.Fingerprint(copyConfirmation) || copyConfirmation.SchemaVersion != DrainConfirmationSchema || state.Plan == nil || state.Queue == nil || copyConfirmation.RunPlanFingerprint != state.Plan.Fingerprint || copyConfirmation.QueueFingerprint != state.Queue.Fingerprint || strings.TrimSpace(copyConfirmation.SessionFingerprint) == "" || strings.TrimSpace(copyConfirmation.NonceFingerprint) == "" {
			return errors.New("invalid human drain confirmation")
		}
	}
	if state.DeliveryResume != nil {
		fingerprint := state.DeliveryResume.Fingerprint
		copyConsent := *state.DeliveryResume
		copyConsent.Fingerprint = ""
		if fingerprint == "" || fingerprint != acquisition.Fingerprint(copyConsent) || copyConsent.SchemaVersion != DeliveryResumeSchema || state.Approval == nil || copyConsent.BatchFingerprint != state.Approval.BatchFingerprint || copyConsent.ApprovalFingerprint != state.Approval.Fingerprint || strings.TrimSpace(copyConsent.SessionFingerprint) == "" || strings.TrimSpace(copyConsent.NonceFingerprint) == "" {
			return errors.New("invalid human delivery resume")
		}
	}
	return nil
}

func validateConnectionSnapshot(snapshot integrations.ConnectionSnapshot, provider string, kind integrations.ConnectionKind) error {
	identity := snapshot.Identity
	if snapshot.ConnectionID == "" || snapshot.Kind != kind || identity.Provider != provider || identity.WorkspaceID == "" || identity.CapabilityVersion == "" ||
		snapshot.CreatedAt.IsZero() || snapshot.LastUsedAt.Before(snapshot.CreatedAt) || snapshot.IdleExpiresAt.Before(snapshot.LastUsedAt) || snapshot.AbsoluteExpiresAt.Before(snapshot.CreatedAt) {
		return errors.New("persisted integration identity is invalid")
	}
	if kind == integrations.ConnectionSlackWebAPI && (identity.ChannelID == "" || identity.CapabilityVersion != acquisitionslack.WebAPIAdapterVersion) {
		return errors.New("persisted Slack integration identity is invalid")
	}
	return nil
}

func validateDrainPolicy(policy DrainPolicy) error {
	fingerprint := policy.Fingerprint
	policy.Fingerprint = ""
	if policy.SchemaVersion != DrainPolicySchema || fingerprint == "" || fingerprint != acquisition.Fingerprint(policy) {
		return errors.New("drain policy fingerprint mismatch")
	}
	if policy.MaximumNetworkRequests <= 0 || policy.MaximumNetworkRequests > 1_000_000 ||
		policy.MaximumWallTimeSeconds < 60 || policy.MaximumWallTimeSeconds > 86_400 ||
		policy.MaximumCostMicrounits < 0 || policy.MaximumCostMicrounits > 1_000_000_000_000 ||
		policy.MaximumRetryAttempts < 0 || policy.MaximumRetryAttempts > 100_000 ||
		policy.ManualSupportTolerance < 0 || policy.ManualSupportTolerance > 250_000 {
		return errors.New("drain policy is outside product safety bounds")
	}
	return nil
}

func validatePersistedPreview(preview controlui.BatchPreview, outbox productbrain.Outbox, preflight *productbrain.PreflightArtifact) error {
	if preview.OutboxFingerprint != outbox.Fingerprint || preview.PrivacyFingerprint != productbrain.DeliveryPrivacyFingerprint(outbox) || preview.DestinationWorkspaceID != outbox.ProfileSnapshot.ExpectedWorkspaceID || preview.DestinationKeyID != outbox.ProfileSnapshot.ExpectedKeyID || preview.MaximumDestinationWrites != len(outbox.Operations) || preview.MaximumMutationAttempts < len(outbox.Operations) || preview.OperationCount != len(outbox.Operations) || preview.PrivacyFindingCount != len(outbox.PrivacyFindings) || !preview.DraftOnly || !preview.OperatorJudged || !canonicalEqual(preview.OperationFingerprints, productbrain.OrderedDeliveryOperationFingerprints(outbox)) {
		return errors.New("persisted batch preview authority mismatch")
	}
	if _, err := time.Parse(time.RFC3339Nano, preview.ExpiresAt); err != nil {
		return errors.New("persisted batch preview expiry mismatch")
	}
	if len(preview.Operations) != len(outbox.Operations) {
		return errors.New("persisted batch operation preview mismatch")
	}
	types, entries, relations := map[string]int{}, 0, 0
	for index, operation := range outbox.Operations {
		actual := preview.Operations[index]
		if actual.OperationID != operation.OperationID || actual.Kind != operation.Kind || actual.PayloadFingerprint != operation.PayloadFingerprint || !canonicalEqual(actual.Dependencies, operation.Dependencies) {
			return errors.New("persisted batch operation preview mismatch")
		}
		if operation.Entry != nil {
			entries++
			types[operation.Entry.CollectionSlug]++
			if actual.CollectionSlug != operation.Entry.CollectionSlug || actual.EntryID != operation.Entry.EntryID || actual.Name != operation.Entry.Name || actual.SourceRef != operation.Entry.SourceRef || !canonicalEqual(actual.Data, operation.Entry.Data) {
				return errors.New("persisted entry preview mismatch")
			}
		} else if operation.Relation != nil {
			relations++
			if actual.RelationIdentity != operation.Relation.RelationIdentity || actual.RelationType != operation.Relation.Type || actual.FromEntryID != operation.Relation.FromEntryID || actual.ToEntryID != operation.Relation.ToEntryID {
				return errors.New("persisted relation preview mismatch")
			}
		}
	}
	if preview.EntryOperationCount != entries || preview.RelationOperationCount != relations || !canonicalEqual(preview.TypeDistribution, types) {
		return errors.New("persisted batch distribution mismatch")
	}
	if len(outbox.Operations) == 0 {
		expected := strings.TrimPrefix(acquisition.Fingerprint(struct{ Outbox, Workspace, Key string }{outbox.Fingerprint, preview.DestinationWorkspaceID, preview.DestinationKeyID}), "sha256:")
		if preflight != nil || preview.PreflightFingerprint != "" || preview.BatchFingerprint != expected {
			return errors.New("persisted zero batch authority mismatch")
		}
	} else {
		if preflight == nil || preview.PreflightFingerprint != preflight.Fingerprint || preview.BatchFingerprint != productbrain.DeliveryBatchFingerprint(outbox, *preflight, preview.PrivacyFingerprint) {
			return errors.New("persisted exact batch authority mismatch")
		}
		if len(preview.PreflightGates) != len(preflight.Gates) {
			return errors.New("persisted preflight preview mismatch")
		}
		for index, gate := range preflight.Gates {
			if preview.PreflightGates[index] != (controlui.BatchGatePreview{Name: gate.Name, Verdict: gate.Verdict, Actual: gate.Actual}) {
				return errors.New("persisted preflight preview mismatch")
			}
		}
	}
	return nil
}

func canonicalEqual(left, right any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}
