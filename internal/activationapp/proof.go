package activationapp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/processing"
	"github.com/synergyai-os/Mindline/internal/processing/routingcompat"
	"github.com/synergyai-os/Mindline/internal/retrieval"
	"github.com/synergyai-os/Mindline/internal/routing"
)

func (app *App) freezeLocked(ctx context.Context) (any, error) {
	if app.state.Inventory == nil || app.state.Strategy == nil || app.state.DrainPolicy == nil {
		return nil, errors.New("inventory and strategy are required")
	}
	if err := acquisition.ValidateInventory(*app.state.Inventory); err != nil {
		return nil, err
	}
	if err := validateExhaustiveInventory(*app.state.Inventory); err != nil {
		return nil, err
	}
	if err := processing.ValidateStrategy(*app.state.Strategy); err != nil {
		return nil, err
	}
	if app.state.Sample != nil && app.state.Plan != nil && app.state.Queue != nil {
		return map[string]any{"frozen": true, "sample_fingerprint": app.state.Sample.Fingerprint, "selected_items": len(app.state.Sample.SelectedItemIDs)}, nil
	}
	inventory, err := toOrchestrationInventory(*app.state.Inventory)
	if err != nil {
		return nil, err
	}
	sample, err := orchestration.SelectProofSample(inventory, "sha256-stratum-order/v0.1")
	if err != nil {
		return nil, err
	}
	privacyPolicy := orchestration.PrivacyPolicyPrivateRuntime
	if app.synthetic {
		privacyPolicy = orchestration.PrivacyPolicySyntheticOnly
	}
	sourceScopeFingerprint := acquisition.Fingerprint(struct {
		SourceIdentity string `json:"source_identity"`
		Watermark      string `json:"watermark"`
	}{app.state.Inventory.SourceIdentity, app.state.Inventory.Watermark})
	if app.state.SourceScope != nil {
		sourceScopeFingerprint = app.state.SourceScope.Fingerprint
	}
	acquisitionVersion := "external_slack_inventory/v2"
	if app.state.SourceScope != nil && strings.TrimSpace(app.state.SourceScope.AdapterVersion) != "" {
		acquisitionVersion = app.state.SourceScope.AdapterVersion
	}
	plan := orchestration.RunPlan{
		SchemaVersion:          orchestration.RunPlanSchemaVersion,
		SourceScopeFingerprint: sourceScopeFingerprint,
		InventoryFingerprint:   inventory.Fingerprint, StrategyFingerprint: app.state.Strategy.Fingerprint,
		ComponentVersions: map[string]string{
			"acquisition": acquisitionVersion, "retrieval": "imported_evidence/v0.1",
			"processing": processing.EvidenceMatcherVersion, "routing": "strict-compiler/v0.1",
			"product_brain": "approved-delivery/v0.2", "control_ui": "local-browser/v0.1",
		},
		PrivacyPolicy: privacyPolicy, Mode: orchestration.RunModeProof,
		IdempotencyNamespace: acquisition.Fingerprint(struct{ RunID, Inventory string }{string(app.state.RunID), inventory.Fingerprint}),
		Budgets: orchestration.RunBudgets{
			MaximumItems: len(sample.SelectedItemIDs), MaximumBytes: 64 << 20, MaximumAttempts: maximum(1, len(sample.SelectedItemIDs)*3),
			MaximumNetworkRequests: app.state.DrainPolicy.MaximumNetworkRequests, MaximumWallTimeSeconds: app.state.DrainPolicy.MaximumWallTimeSeconds,
			MaximumCostMicrounits: app.state.DrainPolicy.MaximumCostMicrounits, MaximumRetryAttempts: app.state.DrainPolicy.MaximumRetryAttempts,
			ManualSupportTolerance: app.state.DrainPolicy.ManualSupportTolerance,
		},
	}
	if err := orchestration.SealRunPlan(&plan); err != nil {
		return nil, err
	}
	aggregate, err := app.service.Get(ctx, app.state.RunID)
	if err != nil {
		return nil, err
	}
	steps := []struct {
		from orchestration.RunState
		kind orchestration.CommandKind
	}{
		{"", orchestration.CommandConfigure},
		{orchestration.StateConfigured, orchestration.CommandStartInventory},
		{orchestration.StateInventorying, orchestration.CommandFreezeInventory},
		{orchestration.StateInventoryFrozen, orchestration.CommandSelectProof},
	}
	for _, step := range steps {
		if aggregate.State != step.from {
			continue
		}
		aggregate, err = app.service.Execute(ctx, app.state.RunID, orchestration.Command{Kind: step.kind, ExpectedVersion: aggregate.Version, Plan: &plan})
		if err != nil {
			return nil, err
		}
	}
	if aggregate.State != orchestration.StateProofSelected {
		return nil, fmt.Errorf("inventory freeze stopped in %s", aggregate.State)
	}
	app.state.Plan = &plan
	app.state.Sample = &sample
	queue, err := buildQueueProjection(*app.state.Inventory, sample)
	if err != nil {
		return nil, err
	}
	app.state.Queue = &queue
	if err := app.commitAuthorityLocked(ctx, "queue", queue.Fingerprint); err != nil {
		return nil, err
	}
	return map[string]any{
		"frozen": true, "inventory_fingerprint": inventory.Fingerprint, "sample_fingerprint": sample.Fingerprint,
		"selected_items": len(sample.SelectedItemIDs), "unselected_items": len(app.state.Inventory.CanonicalItems) - len(sample.SelectedItemIDs),
	}, nil
}

func (app *App) startProofLocked(ctx context.Context) (any, error) {
	if app.state.Inventory == nil || app.state.Strategy == nil || app.state.Sample == nil || app.state.Plan == nil {
		return nil, errors.New("frozen inventory and strategy required")
	}
	aggregate, err := app.service.Get(ctx, app.state.RunID)
	if err != nil {
		return nil, err
	}
	if aggregate.State == orchestration.StateProofComplete || aggregate.State == orchestration.StateDrainConfirmed {
		return map[string]any{"completed": true, "processed_items": len(app.state.Proposals)}, nil
	}
	if aggregate.State == orchestration.StateProofSelected {
		aggregate, err = app.service.Execute(ctx, app.state.RunID, orchestration.Command{Kind: orchestration.CommandStartProof, ExpectedVersion: aggregate.Version, Plan: app.state.Plan})
		if err != nil {
			return nil, err
		}
	}
	if aggregate.State != orchestration.StateProofProcessing {
		return nil, fmt.Errorf("proof cannot start from %s", aggregate.State)
	}
	proofInventory, err := selectProofInventory(*app.state.Inventory, app.state.Sample.SelectedItemIDs)
	if err != nil {
		return nil, err
	}
	adapter, err := retrieval.NewImportedEvidenceAdapter(app.state.Evidence)
	if err != nil {
		return nil, err
	}
	registry := retrieval.NewRegistry()
	strategies := map[string]bool{}
	for _, item := range proofInventory.CanonicalItems {
		if !strategies[item.RetrievalStrategy] {
			if err := registry.Register(item.RetrievalStrategy, adapter); err != nil {
				return nil, err
			}
			strategies[item.RetrievalStrategy] = true
		}
	}
	artifacts := make([]retrieval.Artifact, 0, len(proofInventory.CanonicalItems))
	proposals := make([]processing.Proposal, 0, len(proofInventory.CanonicalItems))
	for _, item := range proofInventory.CanonicalItems {
		request := retrieval.Request{CanonicalItemID: item.CanonicalItemID, CanonicalURL: item.CanonicalURL, Strategy: item.RetrievalStrategy, Format: item.Format, MaximumBodyBytes: 4 << 20}
		var artifact retrieval.Artifact
		if item.AccessState == routing.URLStorageSensitiveRedacted {
			artifact = retrieval.MissingArtifact(request, retrieval.StateNotAttempted, retrieval.AccessUnsupported, retrieval.OriginSourcePolicy, retrieval.SensitiveRedactedMissingnessReason)
			artifact.SecretLike = true
		} else {
			artifact, err = registry.Retrieve(ctx, request)
			if err != nil {
				return nil, err
			}
		}
		processed, err := (processing.EvidenceMatcher{}).Process(processing.Request{Item: item, Retrieval: artifact, Strategy: *app.state.Strategy})
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
		proposals = append(proposals, processed.Proposal)
	}
	app.state.ProofInventory = &proofInventory
	app.state.Retrieval = artifacts
	app.state.Proposals = proposals
	app.state.Reviews = nil
	for _, proposal := range proposals {
		if !proposal.RequiresManualReview {
			continue
		}
		review, reviewErr := processing.RecordOperatorReview(proposal, processing.OperatorReviewInput{
			Decision: processing.ReviewAccept, ReviewerID: "mindline-manual-support-router/v0.1", ReviewedAt: app.now().UTC().Format(time.RFC3339),
			Rationale:            "Mindline routed this evidence-empty or inaccessible item to manual support without a founder semantic judgment.",
			ManualSupportOutcome: "queued_for_manual_processing",
		})
		if reviewErr != nil {
			return nil, reviewErr
		}
		app.state.Reviews = append(app.state.Reviews, review)
	}
	app.state.Route = nil
	if len(proposals) == 0 {
		route, compileErr := routingcompat.CompileReviewed(routingcompat.Input{Inventory: proofInventory, Retrieval: artifacts, Strategy: *app.state.Strategy, Proposals: proposals, Reviews: nil})
		if compileErr != nil {
			return nil, compileErr
		}
		app.state.Route = &route
		if err := app.markQueueReviewedLocked(); err != nil {
			return nil, err
		}
	}
	app.resetDestinationArtifactsLocked()
	if err := app.commitAuthorityLocked(ctx, "processing", acquisition.Fingerprint(struct{ Retrieval, Proposals any }{app.state.Retrieval, app.state.Proposals})); err != nil {
		return nil, err
	}
	if len(proposals) == 0 {
		if err := app.recordAuthorityLocked(ctx, "queue", app.state.Queue.Fingerprint); err != nil {
			return nil, err
		}
		aggregate, err = app.service.Get(ctx, app.state.RunID)
		if err != nil {
			return nil, err
		}
		aggregate, err = app.service.Execute(ctx, app.state.RunID, orchestration.Command{Kind: orchestration.CommandCompleteProof, ExpectedVersion: aggregate.Version, Plan: app.state.Plan})
		if err != nil {
			return nil, err
		}
		return map[string]any{"completed": true, "state": aggregate.State, "processed_items": 0, "awaiting_operator_review": 0}, nil
	}
	if len(app.state.Reviews) == len(proposals) {
		if err := app.completeProofReviewsLocked(ctx); err != nil {
			return nil, err
		}
		return map[string]any{"completed": true, "state": orchestration.StateProofComplete, "processed_items": len(proposals), "awaiting_operator_review": 0, "manual_support_items": len(proposals), "proposed_promotions": 0}, nil
	}
	if len(app.state.Reviews) > 0 {
		if err := app.commitAuthorityLocked(ctx, "review", acquisition.Fingerprint(app.state.Reviews)); err != nil {
			return nil, err
		}
	}
	manual, promoted := 0, 0
	for _, proposal := range proposals {
		if proposal.RequiresManualReview {
			manual++
		}
		if proposal.Judgment.Disposition == "promote" {
			promoted++
		}
	}
	return map[string]any{"completed": false, "state": aggregate.State, "processed_items": len(proposals), "awaiting_operator_review": len(proposals) - len(app.state.Reviews), "manual_support_items": manual, "proposed_promotions": promoted}, nil
}

func (app *App) reviewItemLocked(ctx context.Context, payload reviewItemPayload) (any, error) {
	if app.state.ProofInventory == nil || len(app.state.Proposals) == 0 || app.state.Preview != nil {
		return nil, errors.New("review window is not open")
	}
	index := -1
	for candidate := range app.state.Proposals {
		if app.state.Proposals[candidate].CanonicalItemID == payload.ItemID {
			index = candidate
			break
		}
	}
	if index < 0 || strings.TrimSpace(payload.Rationale) == "" {
		return nil, errors.New("review item not found")
	}
	for _, existing := range app.state.Reviews {
		if existing.CanonicalItemID == payload.ItemID {
			return nil, errors.New("item review is immutable once recorded")
		}
	}
	proposal := app.state.Proposals[index]
	judgment := proposal.Judgment
	decision := processing.ReviewDecision(payload.Decision)
	if decision != processing.ReviewAccept && decision != processing.ReviewRevise {
		return nil, errors.New("review must explicitly accept or revise the proposal")
	}
	if decision == processing.ReviewAccept {
		if payload.Role != judgment.SemanticAssessment.PrimaryRole || payload.Disposition != judgment.Disposition {
			return nil, errors.New("accepted review must match the proposed judgment")
		}
	} else {
		allowedRoles := map[string]bool{"external_entity": true, "evidence_backed_finding": true, "unresolved_tension": true, "reference_resource": true, "unknown": true}
		allowedDispositions := map[string]bool{"promote": true, "hold": true, "clarify": true, "monitor": true, "archive": true}
		if !allowedRoles[payload.Role] || !allowedDispositions[payload.Disposition] || len(proposal.AllowedEvidenceRefs) == 0 {
			return nil, errors.New("requested review revision is not evidence-supported")
		}
		judgment.SemanticAssessment.PrimaryRole = payload.Role
		judgment.Disposition = payload.Disposition
		judgment.DispositionRationale = strings.TrimSpace(payload.Rationale)
		if payload.Disposition == "promote" {
			if payload.Role == "reference_resource" {
				return nil, errors.New("reference resources are not mapped to Product Brain")
			}
			name := judgment.SemanticAssessment.Summary
			if len([]rune(name)) > 120 {
				name = string([]rune(name)[:120])
			}
			judgment.SemanticNodes = []processing.SemanticNode{{
				SemanticNodeID: "reviewed-" + strings.TrimPrefix(acquisition.Fingerprint(struct{ Item, Role string }{payload.ItemID, payload.Role}), "sha256:")[:20],
				Role:           payload.Role, Name: name, Description: judgment.SemanticAssessment.Summary, Confidence: judgment.SemanticAssessment.Confidence,
				EvidenceRefs: append([]string(nil), judgment.SemanticAssessment.EvidenceRefs...), Attributes: map[string]any{"review_source": "founder_browser"},
			}}
			for _, lens := range judgment.LensResults {
				if lens.Result == "matched" {
					judgment.SemanticNodes[0].LensRefs = append(judgment.SemanticNodes[0].LensRefs, lens.LensID)
				}
			}
		} else {
			judgment.SemanticNodes = nil
			judgment.SemanticEdges = nil
		}
	}
	review, err := processing.RecordOperatorReview(proposal, processing.OperatorReviewInput{
		Decision: decision, ReviewerID: "founder-browser-session", ReviewedAt: app.now().UTC().Format(time.RFC3339),
		Rationale: strings.TrimSpace(payload.Rationale), Judgment: judgment, ManualSupportOutcome: payload.ManualSupportOutcome,
	})
	if err != nil {
		return nil, err
	}
	app.state.Reviews = append(app.state.Reviews, review)
	complete := len(app.state.Reviews) == len(app.state.Proposals)
	if complete {
		if err := app.completeProofReviewsLocked(ctx); err != nil {
			return nil, err
		}
	}
	if !complete {
		app.resetDestinationArtifactsLocked()
	}
	if !complete {
		if err := app.commitAuthorityLocked(ctx, "review", acquisition.Fingerprint(app.state.Reviews)); err != nil {
			return nil, err
		}
	}
	return map[string]any{"reviewed": true, "review_fingerprint": review.Fingerprint, "proof_complete": complete, "remaining_reviews": len(app.state.Proposals) - len(app.state.Reviews)}, nil
}

func (app *App) completeProofReviewsLocked(ctx context.Context) error {
	route, err := routingcompat.CompileReviewed(routingcompat.Input{Inventory: *app.state.ProofInventory, Retrieval: app.state.Retrieval, Strategy: *app.state.Strategy, Proposals: app.state.Proposals, Reviews: app.state.Reviews})
	if err != nil {
		return err
	}
	app.state.Route = &route
	if err := app.markQueueReviewedLocked(); err != nil {
		return err
	}
	app.resetDestinationArtifactsLocked()
	if err := app.commitAuthorityLocked(ctx, "queue", app.state.Queue.Fingerprint); err != nil {
		return err
	}
	if err := app.commitAuthorityLocked(ctx, "review", acquisition.Fingerprint(app.state.Reviews)); err != nil {
		return err
	}
	aggregate, err := app.service.Get(ctx, app.state.RunID)
	if err != nil {
		return err
	}
	if aggregate.State != orchestration.StateProofProcessing {
		return fmt.Errorf("proof review cannot complete from %s", aggregate.State)
	}
	_, err = app.service.Execute(ctx, app.state.RunID, orchestration.Command{Kind: orchestration.CommandCompleteProof, ExpectedVersion: aggregate.Version, Plan: app.state.Plan})
	return err
}

func validateExhaustiveInventory(snapshot acquisition.InventorySnapshot) error {
	required := map[string]int{
		"source_record_denominator":           len(snapshot.SourceRecords),
		"url_occurrence_denominator":          len(snapshot.URLOccurrences),
		"canonical_item_denominator":          len(snapshot.CanonicalItems),
		"bidirectional_occurrence_accounting": len(snapshot.URLOccurrences),
	}
	optional := map[string]int{"sensitive_redacted_url_occurrences": 0, "non_semantic_url_sanitizations": 0}
	for _, occurrence := range snapshot.URLOccurrences {
		switch occurrence.SanitizationState {
		case routing.URLStorageSensitiveRedacted:
			optional["sensitive_redacted_url_occurrences"]++
		case routing.URLStorageNonSemanticComponentsRemoved:
			optional["non_semantic_url_sanitizations"]++
		}
	}
	seen := map[string]bool{}
	for _, check := range snapshot.Completeness {
		expected, ok := required[check.Check]
		if !ok {
			expected, ok = optional[check.Check]
		}
		if !ok || seen[check.Check] || check.Status != "pass" || check.Count != expected {
			return errors.New("inventory completeness evidence rejected")
		}
		seen[check.Check] = true
	}
	for name := range required {
		if !seen[name] {
			return errors.New("inventory completeness evidence is incomplete")
		}
	}
	for name := range optional {
		if !seen[name] {
			return errors.New("inventory sanitization evidence is incomplete")
		}
	}
	if len(seen) != len(required)+len(optional) {
		return errors.New("inventory completeness evidence is incomplete")
	}
	return nil
}

func buildQueueProjection(snapshot acquisition.InventorySnapshot, sample orchestration.SampleManifest) (QueueProjection, error) {
	selected := map[string]bool{}
	for _, id := range sample.SelectedItemIDs {
		selected[id] = true
	}
	queue := QueueProjection{SchemaVersion: QueueProjectionSchema, InventoryFingerprint: snapshot.Fingerprint, SampleFingerprint: sample.Fingerprint}
	for _, item := range snapshot.CanonicalItems {
		state := "unselected_unprocessed"
		if selected[item.CanonicalItemID] {
			state = "selected_pending_review"
			queue.SelectedCount++
		} else {
			queue.UnselectedCount++
		}
		queue.Items = append(queue.Items, QueueItem{CanonicalItemID: item.CanonicalItemID, Stratum: item.RetrievalStrategy + "|" + item.Format, State: state})
	}
	sort.Slice(queue.Items, func(i, j int) bool { return queue.Items[i].CanonicalItemID < queue.Items[j].CanonicalItemID })
	queue.Fingerprint = acquisition.Fingerprint(queue)
	if err := validateQueueProjection(queue, snapshot, sample); err != nil {
		return QueueProjection{}, err
	}
	return queue, nil
}

func validateQueueProjection(queue QueueProjection, snapshot acquisition.InventorySnapshot, sample orchestration.SampleManifest) error {
	fingerprint := queue.Fingerprint
	queue.Fingerprint = ""
	if queue.SchemaVersion != QueueProjectionSchema || fingerprint == "" || fingerprint != acquisition.Fingerprint(queue) || queue.InventoryFingerprint != snapshot.Fingerprint || queue.SampleFingerprint != sample.Fingerprint || len(queue.Items) != len(snapshot.CanonicalItems) {
		return errors.New("frozen queue projection mismatch")
	}
	expected, err := buildQueueProjectionUnchecked(snapshot, sample)
	if err != nil || !canonicalEqual(queue.Items, expected.Items) || queue.SelectedCount != expected.SelectedCount || queue.UnselectedCount != expected.UnselectedCount {
		return errors.New("frozen queue accounting mismatch")
	}
	return nil
}

func buildQueueProjectionUnchecked(snapshot acquisition.InventorySnapshot, sample orchestration.SampleManifest) (QueueProjection, error) {
	selected := map[string]bool{}
	for _, id := range sample.SelectedItemIDs {
		selected[id] = true
	}
	queue := QueueProjection{SchemaVersion: QueueProjectionSchema, InventoryFingerprint: snapshot.Fingerprint, SampleFingerprint: sample.Fingerprint}
	for _, item := range snapshot.CanonicalItems {
		state := "unselected_unprocessed"
		if selected[item.CanonicalItemID] {
			state = "selected_pending_review"
			queue.SelectedCount++
		} else {
			queue.UnselectedCount++
		}
		queue.Items = append(queue.Items, QueueItem{CanonicalItemID: item.CanonicalItemID, Stratum: item.RetrievalStrategy + "|" + item.Format, State: state})
	}
	sort.Slice(queue.Items, func(i, j int) bool { return queue.Items[i].CanonicalItemID < queue.Items[j].CanonicalItemID })
	return queue, nil
}

func (app *App) markQueueReviewedLocked() error {
	if app.state.Queue == nil || app.state.Inventory == nil || app.state.Sample == nil {
		return errors.New("frozen queue projection missing")
	}
	for index := range app.state.Queue.Items {
		if app.state.Queue.Items[index].State == "selected_pending_review" {
			app.state.Queue.Items[index].State = "selected_reviewed"
		}
	}
	app.state.Queue.Fingerprint = ""
	app.state.Queue.Fingerprint = acquisition.Fingerprint(*app.state.Queue)
	return validateReviewedQueueProjection(*app.state.Queue, *app.state.Inventory, *app.state.Sample)
}

func validateReviewedQueueProjection(queue QueueProjection, snapshot acquisition.InventorySnapshot, sample orchestration.SampleManifest) error {
	fingerprint := queue.Fingerprint
	queue.Fingerprint = ""
	if fingerprint == "" || fingerprint != acquisition.Fingerprint(queue) || queue.InventoryFingerprint != snapshot.Fingerprint || queue.SampleFingerprint != sample.Fingerprint || len(queue.Items) != len(snapshot.CanonicalItems) {
		return errors.New("reviewed queue projection mismatch")
	}
	selected := map[string]bool{}
	for _, id := range sample.SelectedItemIDs {
		selected[id] = true
	}
	seen := map[string]bool{}
	for _, item := range queue.Items {
		if seen[item.CanonicalItemID] || selected[item.CanonicalItemID] && item.State != "selected_reviewed" || !selected[item.CanonicalItemID] && item.State != "unselected_unprocessed" {
			return errors.New("reviewed queue accounting mismatch")
		}
		seen[item.CanonicalItemID] = true
	}
	return nil
}

func toOrchestrationInventory(snapshot acquisition.InventorySnapshot) (orchestration.InventorySnapshot, error) {
	result := orchestration.InventorySnapshot{
		SchemaVersion: orchestration.InventorySchemaVersion, SourceIdentity: snapshot.SourceIdentity, Watermark: snapshot.Watermark,
	}
	for _, record := range snapshot.SourceRecords {
		result.SourceRecords = append(result.SourceRecords, orchestration.SourceRecord{SourceRecordID: record.SourceRecordID, NativeMessageID: record.NativeMessageID, NativeTimestamp: record.NativeTimestamp, ContentFingerprint: record.ContentFingerprint, URLOccurrenceIDs: append([]string(nil), record.URLOccurrenceIDs...), EditDeleteState: record.EditDeleteState, ThreadParentID: record.ThreadParentID})
	}
	for _, occurrence := range snapshot.URLOccurrences {
		result.URLOccurrences = append(result.URLOccurrences, orchestration.URLOccurrence{URLOccurrenceID: occurrence.URLOccurrenceID, SourceRecordID: occurrence.SourceRecordID, SourceOrdinal: occurrence.SourceOrdinal, ObservedURL: occurrence.ObservedURL, CanonicalItemID: occurrence.CanonicalItemID, SanitizationState: occurrence.SanitizationState})
	}
	for _, item := range snapshot.CanonicalItems {
		result.CanonicalItems = append(result.CanonicalItems, orchestration.InventoryItem{CanonicalItemID: item.CanonicalItemID, CanonicalURL: item.CanonicalURL, RetrievalStrategyID: item.RetrievalStrategy, FormatVariant: item.Format, OccurrenceIDs: append([]string(nil), item.URLOccurrenceIDs...), AccessState: item.AccessState})
	}
	for _, check := range snapshot.Completeness {
		result.Completeness = append(result.Completeness, orchestration.EvidenceCheck{Name: check.Check, Status: check.Status, EvidenceFingerprint: acquisition.Fingerprint(check)})
	}
	if err := orchestration.SealInventorySnapshot(&result); err != nil {
		return orchestration.InventorySnapshot{}, err
	}
	return result, nil
}

func selectProofInventory(snapshot acquisition.InventorySnapshot, selectedIDs []string) (acquisition.InventorySnapshot, error) {
	selected := map[string]bool{}
	for _, id := range selectedIDs {
		selected[id] = true
	}
	occurrences := map[string]bool{}
	result := acquisition.InventorySnapshot{SourceIdentity: snapshot.SourceIdentity, Watermark: snapshot.Watermark}
	for _, item := range snapshot.CanonicalItems {
		if selected[item.CanonicalItemID] {
			copyItem := item
			copyItem.URLOccurrenceIDs = append([]string(nil), item.URLOccurrenceIDs...)
			result.CanonicalItems = append(result.CanonicalItems, copyItem)
			for _, id := range item.URLOccurrenceIDs {
				occurrences[id] = true
			}
		}
	}
	for _, occurrence := range snapshot.URLOccurrences {
		if occurrences[occurrence.URLOccurrenceID] {
			result.URLOccurrences = append(result.URLOccurrences, occurrence)
		}
	}
	for _, record := range snapshot.SourceRecords {
		copyRecord := record
		copyRecord.URLOccurrenceIDs = nil
		for _, id := range record.URLOccurrenceIDs {
			if occurrences[id] {
				copyRecord.URLOccurrenceIDs = append(copyRecord.URLOccurrenceIDs, id)
			}
		}
		if len(copyRecord.URLOccurrenceIDs) > 0 {
			result.SourceRecords = append(result.SourceRecords, copyRecord)
		}
	}
	counts := map[string]int{}
	for _, item := range result.CanonicalItems {
		counts[item.RetrievalStrategy+"\x00"+item.Format]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.SplitN(key, "\x00", 2)
		result.Strata = append(result.Strata, acquisition.StratumCount{RetrievalStrategy: parts[0], Format: parts[1], Count: counts[key]})
	}
	result = acquisition.SealInventory(result)
	if err := acquisition.ValidateInventory(result); err != nil {
		return acquisition.InventorySnapshot{}, err
	}
	return result, nil
}

func maximum(left, right int) int {
	if left > right {
		return left
	}
	return right
}
