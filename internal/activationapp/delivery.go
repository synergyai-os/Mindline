package activationapp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/controlui"
	"github.com/synergyai-os/Mindline/internal/productbrain"
)

func (app *App) PreviewBatch(ctx context.Context, maximumDestinationWrites, maximumMutationAttempts int) (controlui.BatchPreview, error) {
	app.mu.Lock()
	if app.deliveryInFlight {
		app.mu.Unlock()
		return controlui.BatchPreview{}, errors.New("delivery already in flight")
	}
	if app.state.Route == nil || app.connection == nil || app.profile == nil || !app.transportAuthorizedLocked() {
		app.mu.Unlock()
		return controlui.BatchPreview{}, errors.New("proof, destination, and pre-live authority are required")
	}
	if maximumDestinationWrites <= 0 || maximumMutationAttempts < maximumDestinationWrites || maximumMutationAttempts > 10_000 {
		app.mu.Unlock()
		return controlui.BatchPreview{}, errors.New("invalid destination budgets")
	}
	route := *app.state.Route
	routeFingerprint := acquisition.Fingerprint(route)
	profile := *app.profile
	connection := app.connection
	generation := app.connectionGeneration
	app.previewGeneration++
	previewGeneration := app.previewGeneration
	outbox, summary, err := productbrain.CompileOutbox(route, profile)
	if err != nil {
		app.mu.Unlock()
		return controlui.BatchPreview{}, err
	}
	if len(outbox.Operations) > maximumDestinationWrites {
		app.mu.Unlock()
		return controlui.BatchPreview{}, errors.New("destination write ceiling is below the exact batch size")
	}
	operationFingerprints := productbrain.OrderedDeliveryOperationFingerprints(outbox)
	preview := controlui.BatchPreview{
		OutboxFingerprint: outbox.Fingerprint, PrivacyFingerprint: productbrain.DeliveryPrivacyFingerprint(outbox),
		DestinationWorkspaceID: profile.Workspace.ExpectedID, DestinationKeyID: profile.Credential.ExpectedKeyID,
		OperationFingerprints: operationFingerprints, MaximumDestinationWrites: len(outbox.Operations),
		MaximumMutationAttempts: maximum(maximumMutationAttempts, len(outbox.Operations)), ExpiresAt: app.now().UTC().Add(5 * time.Minute).Format(time.RFC3339Nano),
		OperationCount: len(outbox.Operations), EntryOperationCount: summary.EntryOperationCount,
		RelationOperationCount: summary.RelationOperationCount, PrivacyFindingCount: len(outbox.PrivacyFindings),
		DraftOnly: outbox.ProfileSnapshot.DraftOnly, OperatorJudged: outbox.OperatorJudged, TypeDistribution: map[string]int{},
	}
	app.mu.Unlock()
	for _, operation := range outbox.Operations {
		item := controlui.BatchOperationPreview{OperationID: operation.OperationID, Kind: operation.Kind, PayloadFingerprint: operation.PayloadFingerprint, Dependencies: append([]string(nil), operation.Dependencies...)}
		if operation.Entry != nil {
			item.CollectionSlug, item.EntryID, item.Name, item.SourceRef = operation.Entry.CollectionSlug, operation.Entry.EntryID, operation.Entry.Name, operation.Entry.SourceRef
			item.Data = cloneAnyMap(operation.Entry.Data)
			preview.TypeDistribution[operation.Entry.CollectionSlug]++
		} else if operation.Relation != nil {
			item.RelationIdentity, item.RelationType = operation.Relation.RelationIdentity, operation.Relation.Type
			item.FromEntryID, item.ToEntryID = operation.Relation.FromEntryID, operation.Relation.ToEntryID
		}
		preview.Operations = append(preview.Operations, item)
	}
	var preflightArtifact *productbrain.PreflightArtifact
	if len(outbox.Operations) == 0 {
		preview.BatchFingerprint = strings.TrimPrefix(acquisition.Fingerprint(struct {
			Outbox, Workspace, Key string
		}{outbox.Fingerprint, preview.DestinationWorkspaceID, preview.DestinationKeyID}), "sha256:")
	} else {
		preflight, err := productbrain.BuildPreflight(ctx, outbox, profile, connection.Transport)
		if err != nil {
			return controlui.BatchPreview{}, err
		}
		preview.PreflightFingerprint = preflight.Fingerprint
		for _, gate := range preflight.Gates {
			preview.PreflightGates = append(preview.PreflightGates, controlui.BatchGatePreview{Name: gate.Name, Verdict: gate.Verdict, Actual: gate.Actual})
		}
		privacy := productbrain.DeliveryPrivacyFingerprint(outbox)
		preview.BatchFingerprint = productbrain.DeliveryBatchFingerprint(outbox, preflight, privacy)
		preflightArtifact = &preflight
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.deliveryInFlight || app.previewGeneration != previewGeneration || app.connectionGeneration != generation || app.connection != connection || app.state.Route == nil || acquisition.Fingerprint(*app.state.Route) != routeFingerprint || !app.transportAuthorizedLocked() {
		return controlui.BatchPreview{}, errors.New("batch authority changed during preview")
	}
	app.state.Outbox = &outbox
	app.state.Preflight = preflightArtifact
	app.state.Approval = nil
	app.state.HumanEvidence = nil
	app.state.Delivery = nil
	app.state.ZeroDelivery = nil
	app.state.FounderReview = nil
	app.state.Preview = &preview
	if err := app.commitAuthorityLocked(ctx, "outbox", outbox.Fingerprint); err != nil {
		return controlui.BatchPreview{}, err
	}
	return preview, nil
}

func cloneAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (app *App) ApproveBatch(ctx context.Context, approval controlui.HumanApproval) (any, error) {
	return app.approveBatch(ctx, approval, false)
}

func (app *App) approveBatch(ctx context.Context, approval controlui.HumanApproval, testAuthorityBypass bool) (any, error) {
	app.mu.Lock()
	if app.state.Preview == nil || app.state.Outbox == nil || app.connection == nil || app.profile == nil || !app.transportAuthorizedLocked() || !testAuthorityBypass && !approval.ValidFor(*app.state.Preview, app.humanAuthority) {
		app.mu.Unlock()
		return nil, errors.New("human initiation rejected")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, approval.Preview.ExpiresAt)
	if err != nil || !app.now().UTC().Before(expiresAt.UTC()) {
		app.mu.Unlock()
		return nil, productbrain.ErrApprovalExpired
	}
	if len(app.state.Outbox.Operations) == 0 {
		receipt := ZeroDeliveryReceipt{
			SchemaVersion: ZeroDeliveryReceiptSchema, BatchFingerprint: app.state.Preview.BatchFingerprint,
			Status: "zero_draft_activation_recorded", RecordedAt: app.now().UTC().Format(time.RFC3339Nano),
		}
		receipt.Fingerprint = acquisition.Fingerprint(receipt)
		app.state.ZeroDelivery = &receipt
		if err := app.commitAuthorityLocked(ctx, "delivery", receipt.Fingerprint); err != nil {
			app.mu.Unlock()
			return nil, err
		}
		app.mu.Unlock()
		return receipt, nil
	}
	if app.state.Preflight == nil {
		app.mu.Unlock()
		return nil, errors.New("preflight authority missing")
	}
	if app.deliveryInFlight {
		app.mu.Unlock()
		return nil, errors.New("delivery already in flight")
	}
	evidence := productbrain.SealHumanInitiationEvidence(productbrain.HumanInitiationEvidence{
		SchemaVersion: productbrain.HumanInitiationEvidenceSchema, SessionFingerprint: approval.SessionFingerprint,
		ReviewNonceFingerprint: approval.InitiationFingerprint, PreviewEvidenceFingerprint: acquisition.Fingerprint(approval.Preview),
		BatchFingerprint: approval.Preview.BatchFingerprint, DestinationWorkspaceID: approval.Preview.DestinationWorkspaceID,
		DestinationKeyID: approval.Preview.DestinationKeyID, OrderedOperationFingerprints: append([]string(nil), approval.Preview.OperationFingerprints...),
		MaximumDestinationWrites: approval.Preview.MaximumDestinationWrites, MaximumMutationAttempts: approval.Preview.MaximumMutationAttempts,
		IssuedAt: approval.GestureRecordedAt, ExpiresAt: approval.Preview.ExpiresAt, HumanGesture: true, ServerDerived: true,
	})
	deliveryApproval := productbrain.SealDeliveryApproval(productbrain.DeliveryApproval{
		SchemaVersion: productbrain.DeliveryApprovalSchema, BatchFingerprint: approval.Preview.BatchFingerprint,
		OutboxFingerprint: app.state.Outbox.Fingerprint, PreflightFingerprint: app.state.Preflight.Fingerprint,
		PrivacyFingerprint: productbrain.DeliveryPrivacyFingerprint(*app.state.Outbox), DestinationWorkspaceID: approval.Preview.DestinationWorkspaceID,
		DestinationKeyID: approval.Preview.DestinationKeyID, OrderedOperationFingerprints: append([]string(nil), approval.Preview.OperationFingerprints...),
		MaximumDestinationWrites: approval.Preview.MaximumDestinationWrites, MaximumMutationAttempts: approval.Preview.MaximumMutationAttempts,
		HumanInitiationEvidenceFingerprint: evidence.Fingerprint, ApprovedAt: approval.GestureRecordedAt, ExpiresAt: approval.Preview.ExpiresAt,
	})
	app.state.HumanEvidence = &evidence
	app.state.Approval = &deliveryApproval
	if err := app.commitAuthorityLocked(ctx, "approval", deliveryApproval.Fingerprint); err != nil {
		app.mu.Unlock()
		return nil, err
	}
	verifier := &oneTimeHumanVerifier{fingerprint: evidence.Fingerprint}
	batch := productbrain.ApprovedBatch{
		BatchFingerprint: approval.Preview.BatchFingerprint, Outbox: *app.state.Outbox, Profile: *app.profile,
		Preflight: *app.state.Preflight, PrivacyFingerprint: productbrain.DeliveryPrivacyFingerprint(*app.state.Outbox),
		Approval: deliveryApproval, HumanInitiationEvidence: evidence,
	}
	transport := app.connection.Transport
	deadline := expiresAt.UTC()
	if !app.synthetic && app.receipt != nil {
		generatedAt, parseErr := time.Parse(time.RFC3339Nano, app.receipt.GeneratedAt)
		if parseErr != nil {
			app.mu.Unlock()
			return nil, errors.New("pre-live gate timestamp rejected")
		}
		gateDeadline := generatedAt.UTC().Add(app.receiptMaxAge)
		if gateDeadline.Before(deadline) {
			deadline = gateDeadline
		}
	}
	deliveryCtx, cancel := context.WithDeadline(ctx, deadline)
	app.deliveryInFlight = true
	app.deliveryCancel = cancel
	app.mu.Unlock()

	receipt, deliveryErr := productbrain.DeliverApproved(deliveryCtx, batch, transport, filepath.Join(app.runtimeRoot, "product-brain"), productbrain.ApprovedDeliveryOptions{Now: app.now, HumanInitiationVerifier: verifier})
	cancel()
	app.mu.Lock()
	app.deliveryInFlight = false
	app.deliveryCancel = nil
	if receipt.Fingerprint != "" {
		app.state.Delivery = &receipt
	}
	if receipt.Fingerprint != "" {
		if journalErr := app.commitAuthorityLocked(context.Background(), "delivery", receipt.Fingerprint); journalErr != nil {
			app.mu.Unlock()
			return nil, journalErr
		}
	}
	app.mu.Unlock()
	if deliveryErr != nil {
		return receipt, deliveryErr
	}
	return receipt, nil
}

type oneTimeHumanVerifier struct {
	mu          sync.Mutex
	fingerprint string
	consumed    bool
}

func (app *App) resumeApprovedDelivery(ctx context.Context, command controlui.Command) (any, error) {
	app.mu.Lock()
	if !app.verifyHumanActionLocked(command) || app.deliveryInFlight || app.state.Approval == nil || app.state.HumanEvidence == nil || app.state.Outbox == nil || app.state.Preflight == nil || app.connection == nil || app.profile == nil || !app.transportAuthorizedLocked() {
		app.mu.Unlock()
		return nil, errors.New("human delivery resume rejected")
	}
	if app.state.Delivery != nil && app.state.Delivery.Status == "completed" {
		receipt := *app.state.Delivery
		app.mu.Unlock()
		return receipt, nil
	}
	batch := productbrain.ApprovedBatch{
		BatchFingerprint: app.state.Approval.BatchFingerprint, Outbox: *app.state.Outbox, Profile: *app.profile,
		Preflight: *app.state.Preflight, PrivacyFingerprint: app.state.Approval.PrivacyFingerprint,
		Approval: *app.state.Approval, HumanInitiationEvidence: *app.state.HumanEvidence,
	}
	consent := DeliveryResumeConsent{
		SchemaVersion: DeliveryResumeSchema, BatchFingerprint: batch.BatchFingerprint, ApprovalFingerprint: batch.Approval.Fingerprint,
		SessionFingerprint: command.HumanAction.SessionFingerprint, NonceFingerprint: command.HumanAction.NonceFingerprint,
		ResumedAt: command.HumanAction.GestureRecordedAt,
	}
	consent.Fingerprint = acquisition.Fingerprint(consent)
	app.state.DeliveryResume = &consent
	if err := app.commitAuthorityLocked(ctx, "delivery_resume", consent.Fingerprint); err != nil {
		app.mu.Unlock()
		return nil, err
	}
	transport := app.connection.Transport
	verifier := &oneTimeHumanVerifier{fingerprint: batch.HumanInitiationEvidence.Fingerprint}
	deadline := app.now().UTC().Add(5 * time.Minute)
	if !app.synthetic && app.receipt != nil {
		generatedAt, err := time.Parse(time.RFC3339Nano, app.receipt.GeneratedAt)
		if err != nil {
			app.mu.Unlock()
			return nil, errors.New("pre-live gate timestamp rejected")
		}
		gateDeadline := generatedAt.UTC().Add(app.receiptMaxAge)
		if gateDeadline.Before(deadline) {
			deadline = gateDeadline
		}
	}
	deliveryCtx, cancel := context.WithDeadline(ctx, deadline)
	app.deliveryInFlight, app.deliveryCancel = true, cancel
	app.mu.Unlock()

	receipt, deliveryErr := productbrain.DeliverApproved(deliveryCtx, batch, transport, filepath.Join(app.runtimeRoot, "product-brain"), productbrain.ApprovedDeliveryOptions{Now: app.now, HumanInitiationVerifier: verifier})
	cancel()
	app.mu.Lock()
	app.deliveryInFlight, app.deliveryCancel = false, nil
	if receipt.Fingerprint != "" {
		app.state.Delivery = &receipt
		if err := app.commitAuthorityLocked(context.Background(), "delivery", receipt.Fingerprint); err != nil {
			app.mu.Unlock()
			return nil, err
		}
	}
	app.mu.Unlock()
	if deliveryErr != nil {
		return receipt, deliveryErr
	}
	return receipt, nil
}

func (verifier *oneTimeHumanVerifier) VerifyAndConsume(_ context.Context, evidence productbrain.HumanInitiationEvidence) error {
	verifier.mu.Lock()
	defer verifier.mu.Unlock()
	if verifier.consumed || verifier.fingerprint == "" || evidence.Fingerprint != verifier.fingerprint {
		return errors.New("human initiation capability unavailable")
	}
	verifier.consumed = true
	return nil
}

var _ productbrain.HumanInitiationVerifier = (*oneTimeHumanVerifier)(nil)
