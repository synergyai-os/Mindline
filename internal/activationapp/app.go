package activationapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/controlrun"
	"github.com/synergyai-os/Mindline/internal/controlsettings"
	"github.com/synergyai-os/Mindline/internal/controlui"
	"github.com/synergyai-os/Mindline/internal/integrations"
	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/processing"
	"github.com/synergyai-os/Mindline/internal/productbrain"
	"github.com/synergyai-os/Mindline/internal/retrieval"
	"github.com/synergyai-os/Mindline/internal/runjournal"
)

const (
	stateFilename                     = "activation-state.json"
	maximumActivationStateBytes int64 = 128 << 20
)

type App struct {
	mu                   sync.Mutex
	controlRoot          string
	runtimeRoot          string
	now                  func() time.Time
	synthetic            bool
	preLiveReady         bool
	commit               string
	configuration        string
	receipt              *assurance.Receipt
	connector            DestinationConnector
	sourceConnector      SlackSourceConnector
	registry             *integrations.Registry
	settings             controlsettings.RepositoryPort
	runSelection         controlrun.RepositoryPort
	runEntropy           io.Reader
	runBlockerCode       string
	connection           *DestinationConnection
	sourceConnection     *SlackSourceConnection
	profile              *productbrain.DeliveryProfile
	journal              *runjournal.Store
	service              *orchestration.ActivationService
	leaseManager         *runjournal.LeaseManager
	lease                runjournal.Lease
	leaseMu              sync.Mutex
	leaseStop            chan struct{}
	leaseDone            chan struct{}
	leaseErr             error
	loadedPersistedState bool
	state                persistedState
	deliveryInFlight     bool
	deliveryCancel       context.CancelFunc
	connectionGeneration uint64
	sourceGeneration     uint64
	previewGeneration    uint64
	humanAuthority       *controlui.HumanAuthority
}

func DefaultConfigurationFingerprint() string {
	return acquisition.Fingerprint(struct {
		Contract              string `json:"contract"`
		MaximumPerStratum     int    `json:"maximum_per_stratum"`
		InventorySchema       string `json:"inventory_schema"`
		StrategySchema        string `json:"strategy_schema"`
		ProductBrainOrigin    string `json:"product_brain_origin"`
		ProductBrainAuthority string `json:"product_brain_authority"`
	}{"trusted-slack-activation/v0.1", orchestration.MaximumProofItemsPerStratum, acquisitionslack.ExternalInventorySchema, processing.StrategySchema, productbrain.ProductionGatewayOrigin, productbrain.ApprovedDeliveryStateSchema})
}

func New(options Options) (*App, error) {
	if strings.TrimSpace(options.ControlRoot) != "" || options.RunRepository != nil {
		return newStableControlApp(options)
	}
	if strings.TrimSpace(options.RuntimeRoot) == "" {
		return nil, errors.New("missing private runtime root")
	}
	if err := privateio.PrepareDir(options.RuntimeRoot); err != nil {
		return nil, err
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	runID := options.RunID
	if runID == "" {
		runID = defaultRunID
	}
	configuration := strings.TrimSpace(options.ConfigurationFingerprint)
	if configuration == "" {
		configuration = DefaultConfigurationFingerprint()
	}
	preLiveReady := false
	if !options.SyntheticOnly && options.PreLiveReceipt != nil {
		if strings.TrimSpace(options.Commit) == "" {
			return nil, errors.New("pre-live gate commit binding required")
		}
		if err := assurance.Validate(*options.PreLiveReceipt, options.Commit, configuration); err != nil {
			return nil, err
		}
		preLiveReady = true
	}
	connector := options.Connector
	if connector == nil {
		if options.SyntheticOnly {
			return nil, errors.New("synthetic activation requires an injected destination connector")
		}
		connector = productionConnector{}
	}
	sourceConnector := options.SourceConnector
	if sourceConnector == nil {
		sourceConnector = productionSlackSourceConnector{}
	}
	settings := options.SettingsRepository
	if settings == nil {
		createdSettings, settingsErr := controlsettings.NewRepository(options.RuntimeRoot, controlSettingsOptions(now))
		if settingsErr != nil {
			return nil, settingsErr
		}
		settings = createdSettings
	}
	journal, err := runjournal.NewStore(filepath.Join(options.RuntimeRoot, "run-journal"), runjournal.StoreOptions{})
	if err != nil {
		return nil, err
	}
	humanAuthority, err := controlui.NewHumanAuthority()
	if err != nil {
		return nil, err
	}
	app := &App{
		runtimeRoot: options.RuntimeRoot, now: now, synthetic: options.SyntheticOnly, preLiveReady: preLiveReady,
		commit: strings.TrimSpace(options.Commit), configuration: configuration, receipt: options.PreLiveReceipt,
		connector: connector, sourceConnector: sourceConnector, registry: integrations.NewSessionRegistry(integrations.RegistryOptions{}), settings: settings, journal: journal,
		humanAuthority: humanAuthority,
		state:          persistedState{SchemaVersion: StateSchemaVersion, RunID: runID, BuildCommit: strings.TrimSpace(options.Commit), Configuration: configuration},
	}
	leaseManager, err := runjournal.NewLeaseManager(filepath.Join(options.RuntimeRoot, "run-journal"), now)
	if err != nil {
		app.registry.Shutdown()
		return nil, err
	}
	lease, err := leaseManager.Acquire(context.Background(), runID, fmt.Sprintf("activation-%d-%d", os.Getpid(), now().UTC().UnixNano()), 30*time.Second)
	if err != nil {
		app.registry.Shutdown()
		return nil, err
	}
	app.leaseManager, app.lease = leaseManager, lease
	app.startLeaseRenewal()
	if options.PreLiveReceipt != nil {
		app.state.PreLiveReceipt = options.PreLiveReceipt.Fingerprint
		app.state.PreLiveAuthorizations = []string{options.PreLiveReceipt.Fingerprint}
	}
	app.service = orchestration.NewActivationService(journal, now)
	if err := app.loadState(); err != nil {
		app.stopLeaseRenewal()
		_ = app.leaseManager.Release(context.Background(), app.lease)
		app.registry.Shutdown()
		return nil, err
	}
	if err := app.reconcileRunProjection(context.Background()); err != nil {
		app.stopLeaseRenewal()
		_ = app.leaseManager.Release(context.Background(), app.lease)
		app.registry.Shutdown()
		return nil, err
	}
	if !options.SyntheticOnly && preLiveReady && options.PreLiveReceipt != nil {
		if err := app.commitAuthorityLocked(context.Background(), "pre_live_authorization", options.PreLiveReceipt.Fingerprint); err != nil {
			app.stopLeaseRenewal()
			_ = app.leaseManager.Release(context.Background(), app.lease)
			app.registry.Shutdown()
			return nil, err
		}
	}
	return app, nil
}

func (app *App) reconcileRunProjection(ctx context.Context) error {
	aggregate, err := app.service.Get(ctx, app.state.RunID)
	if err != nil {
		return err
	}
	if app.loadedPersistedState && aggregate.Version == 0 {
		return errors.New("activation projection exists without its authoritative run journal")
	}
	if aggregate.LatestAuthorityProjection != "" && app.state.Fingerprint != aggregate.LatestAuthorityProjection {
		path := filepath.Join(app.runtimeRoot, "activation-authority", strings.TrimPrefix(aggregate.LatestAuthorityProjection, "sha256:")+".json")
		var restored persistedState
		if err := privateio.ReadJSONStrictBounded(app.runtimeRoot, path, maximumActivationStateBytes, &restored); err != nil || restored.Fingerprint != aggregate.LatestAuthorityProjection {
			return errors.New("run journal authority projection is unavailable")
		}
		if err := app.validatePersistedState(restored); err != nil {
			return err
		}
		app.state = restored
		if err := privateio.WriteJSON(filepath.Join(app.runtimeRoot, stateFilename), app.state); err != nil {
			return err
		}
	}
	current := app.currentAuthorityFingerprints()
	for domain, fingerprint := range aggregate.AuthorityReferences {
		if expected := current[domain]; expected != "" && expected != fingerprint {
			if domain == "pre_live_authorization" {
				continue
			}
			return errors.New("run journal authority reference conflicts with activation state")
		}
	}
	if aggregate.State == orchestration.StateProofProcessing && app.state.Route != nil && app.state.Plan != nil && len(app.state.Reviews) == len(app.state.Proposals) {
		_, err = app.service.Execute(ctx, app.state.RunID, orchestration.Command{Kind: orchestration.CommandCompleteProof, ExpectedVersion: aggregate.Version, Plan: app.state.Plan})
		return err
	}
	if (aggregate.State == orchestration.StateProofComplete || aggregate.State == orchestration.StateDrainConfirmed) && app.state.Route == nil {
		return errors.New("run journal is ahead of the activation projection")
	}
	return nil
}

func (app *App) currentAuthorityFingerprints() map[string]string {
	result := map[string]string{}
	if app.state.KnownSource != nil {
		result["source_connection"] = acquisition.Fingerprint(*app.state.KnownSource)
	}
	if app.state.SlackDrainWindow != nil {
		result["source_window"] = app.state.SlackDrainWindow.Fingerprint
	}
	if app.state.PreLiveReceipt != "" {
		result["pre_live_authorization"] = app.state.PreLiveReceipt
	}
	if app.state.Inventory != nil {
		result["inventory"] = app.state.Inventory.Fingerprint
	}
	if app.state.Strategy != nil {
		result["strategy"] = app.state.Strategy.Fingerprint
	}
	if app.state.KnownDestination != nil {
		result["destination"] = acquisition.Fingerprint(*app.state.KnownDestination)
	}
	if app.state.Queue != nil {
		result["queue"] = app.state.Queue.Fingerprint
	}
	if app.state.ProofInventory != nil {
		result["processing"] = acquisition.Fingerprint(struct{ Retrieval, Proposals any }{app.state.Retrieval, app.state.Proposals})
	}
	if len(app.state.Reviews) > 0 {
		result["review"] = acquisition.Fingerprint(app.state.Reviews)
	}
	if app.state.Outbox != nil {
		result["outbox"] = app.state.Outbox.Fingerprint
	}
	if app.state.Approval != nil {
		result["approval"] = app.state.Approval.Fingerprint
	}
	if app.state.Cancellation != nil {
		result["cancellation"] = app.state.Cancellation.Fingerprint
	}
	if app.state.Delivery != nil {
		result["delivery"] = app.state.Delivery.Fingerprint
	} else if app.state.ZeroDelivery != nil {
		result["delivery"] = app.state.ZeroDelivery.Fingerprint
	}
	if app.state.FounderReview != nil {
		result["founder_review"] = app.state.FounderReview.Fingerprint
	}
	if app.state.RecoveryProof != nil {
		result["recovery"] = app.state.RecoveryProof.Fingerprint
	}
	if app.state.DrainConfirmation != nil {
		result["drain_confirmation"] = app.state.DrainConfirmation.Fingerprint
	}
	if app.state.DeliveryResume != nil {
		result["delivery_resume"] = app.state.DeliveryResume.Fingerprint
	}
	return result
}

func (app *App) Close() {
	app.stopLeaseRenewal()
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.deliveryCancel != nil {
		app.deliveryCancel()
	}
	if app.connection != nil && app.connection.Disconnect != nil {
		_ = app.connection.Disconnect()
	}
	if app.sourceConnection != nil && app.sourceConnection.Disconnect != nil {
		_ = app.sourceConnection.Disconnect()
	}
	app.connection = nil
	app.sourceConnection = nil
	app.profile = nil
	app.registry.Shutdown()
	if app.leaseManager != nil && app.lease.Fingerprint != "" {
		_ = app.leaseManager.Release(context.Background(), app.lease)
		app.lease = runjournal.Lease{}
	}
}

func (app *App) State(ctx context.Context) (any, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.viewLocked(ctx)
}

// ControlUIAuthority exposes verification/sealing authority only to the
// loopback control server. Its sealing methods are package-private, so generic
// callers cannot mint a browser gesture, and App never accepts injected
// verifier callbacks.
func (app *App) ControlUIAuthority() *controlui.HumanAuthority { return app.humanAuthority }

func (app *App) ImportExternalInventory(ctx context.Context, path, displayName string) (any, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.service == nil || app.state.RunID == "" {
		return nil, errExplicitRunRequired
	}
	displayName = filepath.Base(strings.TrimSpace(displayName))
	if displayName == "" || displayName == "." || len(displayName) > 255 {
		return nil, errors.New("invalid inventory display name")
	}
	if !app.synthetic && !app.preLiveReady {
		return nil, errors.New("pre-live gate required")
	}
	aggregate, err := app.service.Get(ctx, app.state.RunID)
	if err != nil {
		return nil, err
	}
	if aggregate.State != "" && aggregate.State != orchestration.StateConfigured && aggregate.State != orchestration.StateInventorying {
		return nil, errors.New("inventory is already frozen")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	var result acquisitionslack.ImportResult
	if app.synthetic {
		result, err = acquisitionslack.DecodeExternalInventory(file, acquisitionslack.DefaultMaximumBytes)
	} else if app.receipt != nil {
		result, err = acquisitionslack.DecodeAuthorizedExternalInventory(file, acquisitionslack.DefaultMaximumBytes, *app.receipt, app.commit, app.configuration)
	} else {
		err = errors.New("pre-live gate required")
	}
	if err != nil {
		return nil, err
	}
	if app.synthetic && !syntheticInventory(result.Snapshot) {
		return nil, errors.New("synthetic activation rejected non-synthetic source identity")
	}
	if findings := productbrain.ScanPublicArtifact(result.ImportedEvidence, ""); len(findings) > 0 {
		return nil, errors.New("imported evidence quarantined by secret scanner")
	}
	app.state.Inventory = &result.Snapshot
	scope := result.SourceScope
	app.state.SourceScope = &scope
	app.state.SourceDataClass = result.DataClass
	accounting := ImportAccounting{
		FileName:  displayName,
		FileBytes: fileInfo.Size(), Declared: result.DeclaredCounts, Observed: result.ObservedCounts,
		OmissionCount:  inventoryCountDifference(result.DeclaredCounts, result.ObservedCounts),
		DuplicateCount: result.ObservedCounts.URLOccurrences - result.ObservedCounts.CanonicalItems,
	}
	app.state.ImportAccounting = &accounting
	app.state.Evidence = append([]acquisition.ImportedEvidence(nil), result.ImportedEvidence...)
	app.resetAfterInventoryLocked()
	if err := app.commitAuthorityLocked(ctx, "inventory", result.Snapshot.Fingerprint); err != nil {
		return nil, err
	}
	return map[string]any{
		"accepted": true, "source_records": len(result.Snapshot.SourceRecords),
		"url_occurrences": result.Snapshot.OccurrenceCount, "canonical_items": result.Snapshot.CanonicalCount,
		"inventory_fingerprint": result.Snapshot.Fingerprint,
		"file_bytes":            accounting.FileBytes, "declared_counts": accounting.Declared, "observed_counts": accounting.Observed,
		"omission_count": accounting.OmissionCount, "duplicate_occurrences": accounting.DuplicateCount,
	}, nil
}

func inventoryCountDifference(left, right acquisition.InventoryCounts) int {
	abs := func(value int) int {
		if value < 0 {
			return -value
		}
		return value
	}
	return abs(left.SourceRecords-right.SourceRecords) + abs(left.URLOccurrences-right.URLOccurrences) + abs(left.CanonicalItems-right.CanonicalItems)
}

func (app *App) connectDestination(ctx context.Context, payload any) (any, error) {
	credential, ok := payload.([]byte)
	if !ok || len(credential) == 0 {
		return nil, errors.New("destination connection blocked")
	}
	app.mu.Lock()
	if app.service == nil || app.state.RunID == "" || app.connector == nil || app.deliveryInFlight || !app.transportAuthorizedLocked() {
		app.mu.Unlock()
		return nil, errors.New("destination connection blocked")
	}
	generation := app.connectionGeneration
	stateFingerprint := app.state.Fingerprint
	var expectedIdentity *integrations.VerifiedIdentity
	if app.state.KnownDestination != nil {
		copyIdentity := app.state.KnownDestination.Identity
		expectedIdentity = &copyIdentity
	}
	app.mu.Unlock()

	connection, err := app.connector.Connect(ctx, app.registry, credential)
	if err != nil {
		return nil, err
	}
	closeCandidate := func() {
		if connection != nil && connection.Disconnect != nil {
			_ = connection.Disconnect()
		}
	}
	if connection == nil || connection.Transport == nil {
		closeCandidate()
		return nil, errors.New("capability_missing")
	}
	profile := deliveryProfile(connection.Capability)
	if err := productbrain.ValidateDeliveryProfile(profile); err != nil {
		closeCandidate()
		return nil, err
	}
	if expectedIdentity != nil && !canonicalEqual(*expectedIdentity, connection.Snapshot.Identity) {
		closeCandidate()
		return nil, errors.New("destination identity changed; start a new activation run")
	}

	var reconciled productbrain.ApprovedDeliveryReceipt
	clearApproval := false
	app.mu.Lock()
	if app.state.Approval != nil && app.state.HumanEvidence != nil && app.state.Outbox != nil && app.state.Preflight != nil {
		batch := productbrain.ApprovedBatch{BatchFingerprint: app.state.Approval.BatchFingerprint, Outbox: *app.state.Outbox, Profile: profile, Preflight: *app.state.Preflight, PrivacyFingerprint: app.state.Approval.PrivacyFingerprint, Approval: *app.state.Approval, HumanInitiationEvidence: *app.state.HumanEvidence}
		app.mu.Unlock()
		reconciled, err = productbrain.ReconcileApproved(ctx, batch, connection.Transport, filepath.Join(app.runtimeRoot, "product-brain"), productbrain.ApprovedDeliveryOptions{Now: app.now})
		clearApproval = os.IsNotExist(err)
		if err != nil && !clearApproval && !errors.Is(err, productbrain.ErrApprovedDeliveryAmbiguous) && !errors.Is(err, productbrain.ErrApprovedDeliveryCancelled) {
			closeCandidate()
			return nil, err
		}
	} else {
		app.mu.Unlock()
	}

	app.mu.Lock()
	defer app.mu.Unlock()
	if app.deliveryInFlight || app.connectionGeneration != generation || app.state.Fingerprint != stateFingerprint || !app.transportAuthorizedLocked() {
		closeCandidate()
		return nil, errors.New("destination connection state changed during verification")
	}
	if app.connection != nil && app.connection.Disconnect != nil {
		_ = app.connection.Disconnect()
	}
	app.connection, app.profile = connection, &profile
	app.connectionGeneration++
	app.state.KnownDestination = &connection.Snapshot
	if clearApproval {
		app.state.Preview, app.state.Approval, app.state.HumanEvidence = nil, nil, nil
	}
	if err := app.commitAuthorityLocked(ctx, "destination", acquisition.Fingerprint(connection.Snapshot)); err != nil {
		return nil, err
	}
	if reconciled.Fingerprint != "" {
		app.state.Delivery = &reconciled
		if err := app.commitAuthorityLocked(ctx, "delivery", reconciled.Fingerprint); err != nil {
			return nil, err
		}
	}
	return map[string]any{"connected": true, "identity": connection.Snapshot.Identity, "session_only": true}, nil
}

func (app *App) Execute(ctx context.Context, command controlui.Command) (any, error) {
	if command.Kind == "connect_destination" || command.Kind == "resume_delivery" {
		app.mu.Lock()
		active := app.service != nil && app.state.RunID != ""
		app.mu.Unlock()
		if !active {
			return nil, errExplicitRunRequired
		}
	}
	if command.Kind == "connect_destination" {
		return app.connectDestination(ctx, command.Payload)
	}
	if command.Kind == "resume_delivery" {
		return app.resumeApprovedDelivery(ctx, command)
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.service == nil || app.state.RunID == "" {
		return nil, errExplicitRunRequired
	}
	if command.Kind == "review_item" || command.Kind == "founder_review" || command.Kind == "confirm_experimental_drain" {
		if !app.verifyHumanActionLocked(command) {
			return nil, errors.New("human browser action rejected")
		}
	}
	switch command.Kind {
	case "use_settings_for_proof":
		var payload useSettingsPayload
		if err := decodePayload(command.Payload, &payload); err != nil {
			return nil, err
		}
		return app.applySettingsLocked(ctx, payload)
	case "save_strategy":
		var payload saveStrategyPayload
		if err := decodePayload(command.Payload, &payload); err != nil || strings.TrimSpace(payload.ContextLenses) == "" || strings.TrimSpace(payload.RoutingPolicy) == "" {
			return nil, errors.New("invalid strategy")
		}
		policy := DrainPolicy{
			SchemaVersion:          DrainPolicySchema,
			MaximumNetworkRequests: payload.MaximumNetworkRequests, MaximumWallTimeSeconds: payload.MaximumWallTimeSeconds,
			MaximumCostMicrounits: payload.MaximumCostMicrounits, MaximumRetryAttempts: payload.MaximumRetryAttempts,
			ManualSupportTolerance: payload.ManualSupportTolerance,
		}
		policy.Fingerprint = acquisition.Fingerprint(policy)
		if err := validateDrainPolicy(policy); err != nil {
			return nil, err
		}
		if app.state.Sample != nil {
			return nil, errors.New("frozen strategy cannot be changed")
		}
		contentFingerprint := acquisition.Fingerprint(payload)
		strategy := processing.SealStrategy(processing.StrategySnapshot{
			StrategyID: "founder-activation", Version: strings.TrimPrefix(contentFingerprint, "sha256:")[:16],
			ContextLenses: payload.ContextLenses, RoutingPolicy: payload.RoutingPolicy,
			OperatorIdentity: "founder-browser-session", CreatedAt: app.now().UTC().Format(time.RFC3339),
		})
		if err := processing.ValidateStrategy(strategy); err != nil {
			return nil, err
		}
		app.state.Strategy = &strategy
		app.state.DrainPolicy = &policy
		app.resetAfterStrategyLocked()
		if err := app.commitAuthorityLocked(ctx, "strategy", strategy.Fingerprint); err != nil {
			return nil, err
		}
		return map[string]any{"saved": true, "strategy_fingerprint": strategy.Fingerprint, "drain_policy_fingerprint": policy.Fingerprint, "version": strategy.Version}, nil
	case "freeze_inventory":
		return app.freezeLocked(ctx)
	case "start_proof":
		return app.startProofLocked(ctx)
	case "review_item":
		var payload reviewItemPayload
		if err := decodePayload(command.Payload, &payload); err != nil {
			return nil, err
		}
		return app.reviewItemLocked(ctx, payload)
	case "founder_review":
		var payload founderReviewPayload
		if err := decodePayload(command.Payload, &payload); err != nil {
			return nil, err
		}
		return app.recordFounderReviewLocked(ctx, payload)
	case "confirm_experimental_drain":
		aggregate, err := app.service.Get(ctx, app.state.RunID)
		if err != nil {
			return nil, err
		}
		verdict := app.readinessLocked(aggregate)
		if app.state.Plan == nil || app.state.Queue == nil || verdict.Verdict != orchestration.VerdictConditional || len(verdict.Conditions) != 1 || !strings.HasSuffix(verdict.Conditions[0], ":explicit_experimental_drain_confirmation:pending") {
			return nil, errors.New("experimental drain readiness is blocked")
		}
		if aggregate.State != orchestration.StateDrainConfirmed {
			aggregate, err = app.service.Execute(ctx, app.state.RunID, orchestration.Command{Kind: orchestration.CommandConfirmDrain, ExpectedVersion: aggregate.Version, Plan: app.state.Plan})
			if err != nil {
				return nil, err
			}
		}
		confirmation := DrainConfirmation{
			SchemaVersion: DrainConfirmationSchema, RunPlanFingerprint: app.state.Plan.Fingerprint, QueueFingerprint: app.state.Queue.Fingerprint,
			SessionFingerprint: command.HumanAction.SessionFingerprint, NonceFingerprint: command.HumanAction.NonceFingerprint,
			ConfirmedAt: command.HumanAction.GestureRecordedAt,
		}
		confirmation.Fingerprint = acquisition.Fingerprint(confirmation)
		app.state.DrainConfirmation = &confirmation
		if err := app.commitAuthorityLocked(ctx, "drain_confirmation", confirmation.Fingerprint); err != nil {
			return nil, err
		}
		aggregate, err = app.service.Get(ctx, app.state.RunID)
		if err != nil {
			return nil, err
		}
		readiness := app.readinessLocked(aggregate)
		if err := app.recordAuthorityLocked(ctx, "readiness", readiness.EvidenceFingerprint); err != nil {
			return nil, err
		}
		return map[string]any{"ready": true, "state": aggregate.State, "readiness": readiness, "remainder_processed": false}, nil
	case "cancel":
		var payload cancelPayload
		if err := decodePayload(command.Payload, &payload); err != nil || app.state.Approval == nil || payload.ApprovalFingerprint != app.state.Approval.Fingerprint {
			return nil, errors.New("approval mismatch")
		}
		receipt, err := productbrain.CancelApproved(ctx, productbrain.ApprovalRef{ApprovalFingerprint: app.state.Approval.Fingerprint, BatchFingerprint: app.state.Approval.BatchFingerprint}, filepath.Join(app.runtimeRoot, "product-brain"), app.now)
		if err != nil {
			return nil, err
		}
		app.state.Cancellation = &receipt
		if err := app.commitAuthorityLocked(ctx, "cancellation", receipt.Fingerprint); err != nil {
			return nil, err
		}
		if app.deliveryCancel != nil {
			app.deliveryCancel()
		}
		return receipt, nil
	case "disconnect":
		if app.deliveryCancel != nil {
			app.deliveryCancel()
		}
		if app.connection != nil && app.connection.Disconnect != nil {
			_ = app.connection.Disconnect()
		}
		app.connection = nil
		app.profile = nil
		app.connectionGeneration++
		return map[string]any{"disconnected": true}, nil
	default:
		return nil, fmt.Errorf("unsupported activation command %q", command.Kind)
	}
}

func (app *App) verifyHumanActionLocked(command controlui.Command) bool {
	if command.HumanAction == nil || app.humanAuthority == nil || command.HumanAction.Kind != command.Kind || command.HumanAction.PayloadFingerprint != controlui.FingerprintPayload(command.Payload) {
		return false
	}
	recordedAt, err := time.Parse(time.RFC3339Nano, command.HumanAction.GestureRecordedAt)
	if err != nil || recordedAt.After(app.now().UTC().Add(5*time.Second)) || app.now().UTC().Sub(recordedAt) > 2*time.Minute {
		return false
	}
	return app.humanAuthority.VerifyAndConsumeAction(*command.HumanAction)
}

func (app *App) recordAuthorityLocked(ctx context.Context, domain, fingerprint string) error {
	if err := app.renewLeaseLocked(ctx); err != nil {
		return err
	}
	if app.state.Fingerprint == "" {
		app.sealStateLocked()
	}
	_, err := app.service.RecordAuthority(ctx, app.state.RunID, domain, fingerprint, app.state.Fingerprint)
	return err
}

func (app *App) commitAuthorityLocked(ctx context.Context, domain, fingerprint string) error {
	if err := app.renewLeaseLocked(ctx); err != nil {
		return err
	}
	app.sealStateLocked()
	dir := filepath.Join(app.runtimeRoot, "activation-authority")
	if err := privateio.PrepareDir(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, strings.TrimPrefix(app.state.Fingerprint, "sha256:")+".json")
	if err := privateio.WriteJSONNoReplace(path, app.state); err != nil {
		var existing persistedState
		if readErr := privateio.ReadJSONStrictBounded(app.runtimeRoot, path, maximumActivationStateBytes, &existing); readErr != nil || !canonicalEqual(existing, app.state) {
			return err
		}
	}
	if _, err := app.service.RecordAuthority(ctx, app.state.RunID, domain, fingerprint, app.state.Fingerprint); err != nil {
		return err
	}
	return privateio.WriteJSON(filepath.Join(app.runtimeRoot, stateFilename), app.state)
}

func (app *App) renewLeaseLocked(ctx context.Context) error {
	return app.renewLease(ctx)
}

func (app *App) renewLease(ctx context.Context) error {
	app.leaseMu.Lock()
	defer app.leaseMu.Unlock()
	if app.leaseErr != nil {
		return app.leaseErr
	}
	if app.leaseManager == nil || app.lease.Fingerprint == "" {
		return errors.New("activation run lease unavailable")
	}
	renewed, err := app.leaseManager.Renew(ctx, app.lease, 30*time.Second)
	if err != nil {
		app.leaseErr = err
		return err
	}
	app.lease = renewed
	return nil
}

func (app *App) startLeaseRenewal() {
	stop := make(chan struct{})
	done := make(chan struct{})
	app.leaseMu.Lock()
	app.leaseStop = stop
	app.leaseDone = done
	app.leaseMu.Unlock()
	go func(stop <-chan struct{}, done chan<- struct{}) {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = app.renewLease(context.Background())
			}
		}
	}(stop, done)
}

func (app *App) stopLeaseRenewal() {
	app.leaseMu.Lock()
	stop, done := app.leaseStop, app.leaseDone
	app.leaseStop, app.leaseDone = nil, nil
	app.leaseMu.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
}

func (app *App) loadState() error {
	path := filepath.Join(app.runtimeRoot, stateFilename)
	var state persistedState
	if err := privateio.ReadJSONStrictBounded(app.runtimeRoot, path, maximumActivationStateBytes, &state); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	fingerprint := state.Fingerprint
	state.Fingerprint = ""
	if state.SchemaVersion != StateSchemaVersion {
		return errors.New("activation state requires rebuild after STD-20")
	}
	if state.RunID == "" || fingerprint == "" || fingerprint != acquisition.Fingerprint(state) {
		return errors.New("activation state fingerprint mismatch")
	}
	if state.RunID != app.state.RunID || state.BuildCommit != app.commit || state.Configuration != app.configuration {
		return errors.New("activation state build or configuration binding mismatch")
	}
	expectedReceipt := ""
	if app.receipt != nil {
		expectedReceipt = app.receipt.Fingerprint
	}
	if expectedReceipt == "" && state.PreLiveReceipt != "" {
		return errors.New("activation state pre-live receipt binding mismatch")
	}
	state.Fingerprint = fingerprint
	if err := app.validatePersistedState(state); err != nil {
		return err
	}
	app.state = state
	app.loadedPersistedState = true
	if expectedReceipt != "" {
		app.state.PreLiveReceipt = expectedReceipt
		found := false
		for _, fingerprint := range app.state.PreLiveAuthorizations {
			if fingerprint == expectedReceipt {
				found = true
			}
		}
		if !found {
			app.state.PreLiveAuthorizations = append(app.state.PreLiveAuthorizations, expectedReceipt)
		}
	}
	return nil
}

func (app *App) persistLocked() error {
	app.sealStateLocked()
	return privateio.WriteJSON(filepath.Join(app.runtimeRoot, stateFilename), app.state)
}

func (app *App) sealStateLocked() {
	app.state.SchemaVersion = StateSchemaVersion
	app.state.BuildCommit = app.commit
	app.state.Configuration = app.configuration
	app.state.PreLiveReceipt = ""
	if app.receipt != nil {
		app.state.PreLiveReceipt = app.receipt.Fingerprint
		found := false
		for _, fingerprint := range app.state.PreLiveAuthorizations {
			if fingerprint == app.receipt.Fingerprint {
				found = true
			}
		}
		if !found {
			app.state.PreLiveAuthorizations = append(app.state.PreLiveAuthorizations, app.receipt.Fingerprint)
		}
	}
	app.state.Fingerprint = ""
	app.state.Fingerprint = acquisition.Fingerprint(app.state)
}

func (app *App) transportAuthorizedLocked() bool {
	if app.synthetic {
		return true
	}
	return app.preLiveReady && app.receipt != nil && assurance.Validate(*app.receipt, app.commit, app.configuration) == nil
}

func (app *App) resetAfterInventoryLocked() {
	app.state.Plan = nil
	app.state.Sample = nil
	app.state.Queue = nil
	app.state.ProofInventory = nil
	app.state.Retrieval = nil
	app.state.Proposals = nil
	app.state.Reviews = nil
	app.state.Route = nil
	app.resetDestinationArtifactsLocked()
}

func (app *App) resetAfterStrategyLocked() {
	app.state.Plan = nil
	app.state.Sample = nil
	app.state.Queue = nil
	app.state.ProofInventory = nil
	app.state.Retrieval = nil
	app.state.Proposals = nil
	app.state.Reviews = nil
	app.state.Route = nil
	app.resetDestinationArtifactsLocked()
}

func (app *App) resetDestinationArtifactsLocked() {
	app.state.Outbox = nil
	app.state.Preflight = nil
	app.state.Preview = nil
	app.state.Approval = nil
	app.state.HumanEvidence = nil
	app.state.Delivery = nil
	app.state.Cancellation = nil
	app.state.ZeroDelivery = nil
	app.state.FounderReview = nil
	app.state.RecoveryProof = nil
	app.state.DrainConfirmation = nil
	app.state.DeliveryResume = nil
}

func (app *App) viewLocked(ctx context.Context) (View, error) {
	settings, err := app.settings.Load(ctx)
	if err != nil {
		return View{}, err
	}
	mode := "live"
	if app.synthetic {
		mode = "synthetic"
	}
	view := View{SchemaVersion: StateSchemaVersion, Mode: mode, PreLiveReady: app.transportAuthorizedLocked(), Settings: settings}
	view.ActiveStrategy.State = "absent"
	view.Connections.SessionOnly = true
	if app.runSelection != nil {
		selection, err := app.runSelection.Load(ctx)
		if err != nil {
			return View{}, err
		}
		view.RunSelection = app.runSelectionView(selection)
		if app.service == nil {
			return view, nil
		}
	}
	if app.service == nil {
		return View{}, errExplicitRunRequired
	}
	aggregate, err := app.service.Get(ctx, app.state.RunID)
	if err != nil {
		return View{}, err
	}
	view.Run = RunView{RunID: app.state.RunID, State: aggregate.State, Version: aggregate.Version}
	view.DeliveryInFlight = app.deliveryInFlight
	view.Connections.SourceImported = app.state.Inventory != nil
	view.Connections.SourceConnected = app.sourceConnection != nil
	view.Connections.DestinationConnected = app.connection != nil
	if app.connection != nil {
		identity := app.connection.Snapshot.Identity
		view.Connections.DestinationIdentity = &identity
	}
	if app.sourceConnection != nil {
		identity := app.sourceConnection.Snapshot.Identity
		view.Connections.SourceIdentity = &identity
	} else if app.state.KnownSource != nil {
		identity := app.state.KnownSource.Identity
		view.Connections.SourceIdentity = &identity
	}
	if app.state.Strategy != nil {
		view.Strategy = StrategyView{Configured: true, Fingerprint: app.state.Strategy.Fingerprint, ContextLenses: app.state.Strategy.ContextLenses, RoutingPolicy: app.state.Strategy.RoutingPolicy}
		view.ActiveStrategy = ActiveStrategyView{State: "open", SettingsVersion: app.state.SettingsVersion, SettingsGeneration: app.state.SettingsGeneration, Fingerprint: app.state.Strategy.Fingerprint, ExactLenses: processing.ContextLenses(*app.state.Strategy), RoutingPolicy: app.state.Strategy.RoutingPolicy}
		if app.state.Sample != nil {
			view.ActiveStrategy.State = "sealed"
		}
		if app.state.DrainPolicy != nil {
			copyPolicy := *app.state.DrainPolicy
			view.Strategy.DrainPolicy = &copyPolicy
		}
	}
	if app.state.Inventory != nil {
		view.Inventory.SourceRecords = len(app.state.Inventory.SourceRecords)
		view.Inventory.URLOccurrences = app.state.Inventory.OccurrenceCount
		view.Inventory.CanonicalItems = app.state.Inventory.CanonicalCount
		view.Inventory.SourceIdentity = app.state.Inventory.SourceIdentity
		view.Inventory.Watermark = app.state.Inventory.Watermark
		view.Inventory.DataClass = app.state.SourceDataClass
		view.Inventory.DuplicateCount = app.state.Inventory.OccurrenceCount - app.state.Inventory.CanonicalCount
		view.Inventory.Completeness = append([]acquisition.EvidenceCheck(nil), app.state.Inventory.Completeness...)
		if app.state.ImportAccounting != nil {
			view.Inventory.FileName = app.state.ImportAccounting.FileName
			view.Inventory.FileBytes = app.state.ImportAccounting.FileBytes
			view.Inventory.DeclaredCounts = app.state.ImportAccounting.Declared
			view.Inventory.ObservedCounts = app.state.ImportAccounting.Observed
			view.Inventory.OmissionCount = app.state.ImportAccounting.OmissionCount
		}
	}
	if app.state.Sample != nil {
		view.Inventory.Frozen = true
		view.Inventory.SelectedItems = len(app.state.Sample.SelectedItemIDs)
		view.Inventory.UnselectedItems = view.Inventory.CanonicalItems - view.Inventory.SelectedItems
		view.Inventory.Strata = append([]orchestration.StratumSample(nil), app.state.Sample.Strata...)
	}
	if app.state.Queue != nil {
		view.Inventory.QueueFingerprint = app.state.Queue.Fingerprint
	}
	view.Proof.Started = app.state.ProofInventory != nil
	view.Proof.Completed = aggregate.State == orchestration.StateProofComplete || aggregate.State == orchestration.StateDrainConfirmed || aggregate.State == orchestration.StateDrainProcessing || aggregate.State == orchestration.StateQueueSealed
	artifacts := map[string]retrieval.Artifact{}
	for _, artifact := range app.state.Retrieval {
		artifacts[artifact.CanonicalItemID] = artifact
	}
	proposals := map[string]processing.Proposal{}
	for _, proposal := range app.state.Proposals {
		proposals[proposal.CanonicalItemID] = proposal
	}
	reviews := map[string]processing.OperatorReviewRecord{}
	for _, review := range app.state.Reviews {
		reviews[review.CanonicalItemID] = review
	}
	sourceRecords := map[string]acquisition.SourceRecord{}
	occurrences := map[string]acquisition.URLOccurrence{}
	if app.state.ProofInventory != nil {
		for _, record := range app.state.ProofInventory.SourceRecords {
			sourceRecords[record.SourceRecordID] = record
		}
		for _, occurrence := range app.state.ProofInventory.URLOccurrences {
			occurrences[occurrence.URLOccurrenceID] = occurrence
		}
	}
	if app.state.ProofInventory != nil {
		for _, item := range app.state.ProofInventory.CanonicalItems {
			artifact, proposal, review := artifacts[item.CanonicalItemID], proposals[item.CanonicalItemID], reviews[item.CanonicalItemID]
			entry := ProofItemView{
				CanonicalItemID: item.CanonicalItemID, CanonicalURL: item.CanonicalURL, Kind: item.Kind,
				RetrievalStrategy: item.RetrievalStrategy, Format: item.Format, RetrievalState: string(artifact.State),
				EvidenceOrigin: string(artifact.Origin), RequiresManualReview: proposal.RequiresManualReview,
				Title: artifact.Metadata.Title, Author: artifact.Metadata.Author, Missingness: append([]string(nil), artifact.Missingness...),
				LensResults:  append([]processing.LensResult(nil), proposal.Judgment.LensResults...),
				ProposedRole: proposal.Judgment.SemanticAssessment.PrimaryRole, ProposedDisposition: proposal.Judgment.Disposition,
				ProposedSummary:   proposal.Judgment.SemanticAssessment.Summary,
				ProposedRationale: proposal.Judgment.DispositionRationale, ReasonCodes: append([]string(nil), proposal.ReasonCodes...),
				DestinationMapping: destinationMapping(proposal.Judgment.SemanticAssessment.PrimaryRole), ReviewStatus: "awaiting_operator_review",
			}
			entry.Missingness = append(entry.Missingness, proposal.Judgment.SemanticAssessment.Missingness...)
			seenOccurrences := map[string]bool{}
			for _, occurrenceID := range item.URLOccurrenceIDs {
				occurrence := occurrences[occurrenceID]
				record := sourceRecords[occurrence.SourceRecordID]
				if record.SourceRecordID != "" && !seenOccurrences[occurrenceID] {
					entry.SourceReferences = append(entry.SourceReferences, SourceReferenceView{NativeMessageID: record.NativeMessageID, NativeTimestamp: record.NativeTimestamp, URLOrdinal: occurrence.SourceOrdinal})
					seenOccurrences[occurrenceID] = true
				}
			}
			for _, excerpt := range artifact.Excerpts {
				entry.Excerpts = append(entry.Excerpts, EvidenceExcerptView{Text: excerpt.Text, Locator: excerpt.Locator})
			}
			if review.Fingerprint != "" {
				entry.Role = review.Judgment.SemanticAssessment.PrimaryRole
				entry.Disposition = review.Judgment.Disposition
				entry.ReviewFingerprint = review.Fingerprint
				entry.ReviewStatus = "reviewed"
				view.Proof.ReviewedCount++
			} else {
				entry.Role = proposal.Judgment.SemanticAssessment.PrimaryRole
				entry.Disposition = proposal.Judgment.Disposition
				view.Proof.AwaitingReviewCount++
			}
			if proposal.RequiresManualReview {
				view.Proof.ManualSupportCount++
			}
			if review.Judgment.Disposition == "promote" {
				view.Proof.PromoteCount++
			}
			view.Proof.Items = append(view.Proof.Items, entry)
		}
		view.Proof.ItemCount = len(view.Proof.Items)
	}
	if app.state.Outbox != nil {
		view.Destination.OperationCount = len(app.state.Outbox.Operations)
	} else if app.state.Route != nil && app.profile != nil {
		if outbox, _, compileErr := productbrain.CompileOutbox(*app.state.Route, *app.profile); compileErr == nil {
			view.Destination.OperationCount = len(outbox.Operations)
		}
	}
	view.Destination.BatchPreview = app.state.Preview
	if app.state.Approval != nil {
		view.Destination.ApprovalFingerprint = app.state.Approval.Fingerprint
	}
	if app.state.Cancellation != nil {
		view.Destination.CancellationFingerprint = app.state.Cancellation.Fingerprint
	}
	if app.state.Delivery != nil {
		view.Destination.DeliveryStatus = app.state.Delivery.Status
		view.Destination.ReceiptFingerprint = app.state.Delivery.Fingerprint
		view.Destination.DraftIDs = append([]string(nil), app.state.Delivery.RemoteObjectIDs...)
	} else if app.state.ZeroDelivery != nil {
		view.Destination.DeliveryStatus = app.state.ZeroDelivery.Status
		view.Destination.ReceiptFingerprint = app.state.ZeroDelivery.Fingerprint
	}
	view.Founder.ReviewRecorded = app.state.FounderReview != nil
	view.Founder.TrustedActivationCompletion = app.trustedActivationLocked()
	view.Founder.TrustedValueObserved = app.state.FounderReview != nil && app.state.FounderReview.ValueVerdict == "useful" && !app.state.FounderReview.ZeroDraft && len(app.state.FounderReview.UsefulDraftIDs) > 0 && strings.TrimSpace(app.state.FounderReview.UsefulnessReason) != ""
	view.Drain.Verdict = app.readinessLocked(aggregate)
	view.Drain.Stages = app.readinessStagesLocked(aggregate)
	view.Drain.AuthorizationSentences = readinessAuthorizationSentences()
	view.Drain.FullInventoryQueued = app.queueValidLocked()
	view.Drain.ExperimentalDrainAuthorized = view.Drain.Verdict.Verdict == orchestration.VerdictReady
	view.Drain.RequiresExplicitConfirmation = view.Drain.Verdict.Verdict == orchestration.VerdictConditional
	view.Drain.ProcessedRemainder = false
	return view, nil
}

func destinationMapping(role string) string {
	switch role {
	case "external_entity":
		return "Product Brain / landscape draft"
	case "evidence_backed_finding":
		return "Product Brain / insights draft"
	case "unresolved_tension":
		return "Product Brain / tensions draft"
	default:
		return "unmapped; no destination write"
	}
}

func (app *App) queueValidLocked() bool {
	if app.state.Queue == nil || app.state.Inventory == nil || app.state.Sample == nil {
		return false
	}
	if app.state.Route != nil {
		return validateReviewedQueueProjection(*app.state.Queue, *app.state.Inventory, *app.state.Sample) == nil
	}
	return validateQueueProjection(*app.state.Queue, *app.state.Inventory, *app.state.Sample) == nil
}

func (app *App) readinessLocked(aggregate orchestration.Aggregate) orchestration.ReadinessVerdict {
	check := func(name string, passed bool, evidence string) orchestration.ReadinessCheck {
		status := orchestration.CheckFail
		if passed {
			status = orchestration.CheckPass
		}
		return orchestration.ReadinessCheck{Name: name, Status: status, EvidenceFingerprint: evidence}
	}
	queueEvidence := ""
	if app.state.Queue != nil {
		queueEvidence = app.state.Queue.Fingerprint
	}
	reviewEvidence := acquisition.Fingerprint(app.state.Reviews)
	deliveryEvidence := ""
	if app.state.FounderReview != nil {
		deliveryEvidence = app.state.FounderReview.Fingerprint
	}
	manualCount, manualAssessed := app.manualSupportOutcomeStateLocked()
	proofComplete := app.state.Route != nil && len(app.state.Reviews) == len(app.state.Proposals) && (len(app.state.Proposals) > 0 || app.state.Sample != nil && len(app.state.Sample.SelectedItemIDs) == 0)
	strataPassed, strataEvidence := app.observedStrataOutcomeLocked()
	privacyPassed := app.outboxPrivacyPassedLocked()
	recoveryPassed := app.recoveryProofValidLocked(aggregate)
	resourcesPassed, resourceEvidence, projectedManual := app.projectedDrainResourcesLocked(manualCount)
	manualWithinTolerance := app.state.Plan != nil && projectedManual <= app.state.Plan.Budgets.ManualSupportTolerance
	securityEvidence := app.configuration
	if app.receipt != nil {
		securityEvidence = app.receipt.Fingerprint
	}
	confirmation := orchestration.ReadinessCheck{Name: "explicit_experimental_drain_confirmation", Status: orchestration.CheckPending}
	if aggregate.State == orchestration.StateDrainConfirmed && app.drainConfirmationValidLocked(aggregate) {
		confirmation.Status = orchestration.CheckPass
		confirmation.EvidenceFingerprint = app.state.DrainConfirmation.Fingerprint
	}
	return orchestration.EvaluateReadiness(orchestration.StageExperimentalDrain,
		orchestration.ReadinessContribution{ContributorID: "frozen_queue", Version: QueueProjectionSchema, RequiredChecks: []string{"exhaustive_queue_accounting"}, Checks: []orchestration.ReadinessCheck{check("exhaustive_queue_accounting", app.queueValidLocked(), queueEvidence)}},
		orchestration.ReadinessContribution{ContributorID: "operator_review", Version: processing.OperatorReviewSchema, RequiredChecks: []string{"proof_complete", "manual_support_assessed", "observed_strata_outcomes"}, Checks: []orchestration.ReadinessCheck{
			check("proof_complete", proofComplete, reviewEvidence),
			check("manual_support_assessed", manualAssessed, acquisition.Fingerprint(struct {
				Count    int
				Assessed bool
			}{manualCount, manualAssessed})),
			check("observed_strata_outcomes", strataPassed, strataEvidence),
		}},
		orchestration.ReadinessContribution{ContributorID: "activation_safety", Version: "pre-live/v0.1", RequiredChecks: []string{"pre_live_authority", "privacy_security_clear", "recovery_proven", "resource_projection_within_budget", "manual_burden_within_tolerance", "trusted_activation_completion", "explicit_experimental_drain_confirmation"}, Checks: []orchestration.ReadinessCheck{
			check("pre_live_authority", app.transportAuthorizedLocked(), securityEvidence),
			check("privacy_security_clear", privacyPassed, acquisition.Fingerprint(struct{ Passed bool }{privacyPassed})),
			check("recovery_proven", recoveryPassed, acquisition.Fingerprint(struct{ Passed bool }{recoveryPassed})),
			check("resource_projection_within_budget", resourcesPassed, resourceEvidence),
			check("manual_burden_within_tolerance", manualWithinTolerance, acquisition.Fingerprint(struct{ Projected, Tolerance int }{projectedManual, func() int {
				if app.state.Plan == nil {
					return 0
				}
				return app.state.Plan.Budgets.ManualSupportTolerance
			}()})),
			check("trusted_activation_completion", app.trustedActivationLocked(), deliveryEvidence),
			confirmation,
		}},
	)
}

func (app *App) readinessStagesLocked(aggregate orchestration.Aggregate) []orchestration.ReadinessVerdict {
	check := func(name string, passed bool, evidence string) orchestration.ReadinessCheck {
		status := orchestration.CheckFail
		if passed {
			status = orchestration.CheckPass
		}
		return orchestration.ReadinessCheck{Name: name, Status: status, EvidenceFingerprint: evidence}
	}
	inventoryPresent := app.state.Inventory != nil && app.state.SourceScope != nil && app.state.ImportAccounting != nil
	inventoryEvidence := ""
	if app.state.Inventory != nil {
		inventoryEvidence = app.state.Inventory.Fingerprint
	}
	sourceVersion := acquisitionslack.ExternalInventorySchema
	connectionIdentity := inventoryPresent
	immutableScope := inventoryPresent && app.state.SourceScope.Fingerprint != ""
	acquisitionBudgets := inventoryPresent && app.state.ImportAccounting.FileBytes <= acquisitionslack.DefaultMaximumBytes
	cancellation := orchestration.ReadinessCheck{Name: "cancellation_contract", Status: orchestration.CheckNA, ContractAllowsNA: true, NARationale: "bounded external-file import has no remote pagination to cancel"}
	if app.state.SourceScope != nil && app.state.SourceScope.ConnectorKind == "slack_web_api" {
		sourceVersion = acquisitionslack.WebAPIAdapterVersion
		connectionIdentity = inventoryPresent && app.state.KnownSource != nil && app.state.KnownSource.Identity.WorkspaceID == app.state.SourceScope.WorkspaceID && app.state.KnownSource.Identity.ChannelID == app.state.SourceScope.ChannelID
		immutableScope = immutableScope && app.state.SlackDrainWindow != nil && app.state.SlackDrainWindow.WorkspaceID == app.state.SourceScope.WorkspaceID && app.state.SlackDrainWindow.ChannelID == app.state.SourceScope.ChannelID && app.state.SlackDrainWindow.Oldest == app.state.SourceScope.LowerInclusive && app.state.SlackDrainWindow.Latest == app.state.SourceScope.UpperInclusive
		acquisitionBudgets = acquisitionBudgets && app.state.DrainPolicy != nil
		cancelPassed := app.state.SlackDrainWindow != nil && app.receiptHasChecks("targeted_race", "activation_journal_recovery")
		cancellation = check("cancellation_contract", cancelPassed, acquisition.Fingerprint(struct {
			Window  *SlackDrainWindow
			Budgets *DrainPolicy
		}{app.state.SlackDrainWindow, app.state.DrainPolicy}))
	}
	inventory := orchestration.EvaluateReadiness(orchestration.StageInventory, orchestration.ReadinessContribution{
		ContributorID: "source_inventory", Version: sourceVersion,
		RequiredChecks: []string{"connection_identity", "immutable_scope_integrity", "acquisition_budgets", "cancellation_contract", "private_root"},
		Checks: []orchestration.ReadinessCheck{
			check("connection_identity", connectionIdentity, inventoryEvidence), check("immutable_scope_integrity", immutableScope, func() string {
				if app.state.SourceScope == nil {
					return ""
				}
				return app.state.SourceScope.Fingerprint
			}()),
			check("acquisition_budgets", acquisitionBudgets, acquisition.Fingerprint(struct {
				Import *ImportAccounting
				Policy *DrainPolicy
			}{app.state.ImportAccounting, app.state.DrainPolicy})),
			cancellation,
			check("private_root", app.transportAuthorizedLocked(), app.configuration),
		},
	})
	process := orchestration.EvaluateReadiness(orchestration.StageProcess, orchestration.ReadinessContribution{
		ContributorID: "frozen_proof", Version: QueueProjectionSchema,
		RequiredChecks: []string{"exhaustive_reconciled_inventory", "sealed_selection_accounting", "processor_versions", "manual_capacity_pinned"},
		Checks: []orchestration.ReadinessCheck{
			check("exhaustive_reconciled_inventory", app.queueValidLocked(), func() string {
				if app.state.Queue == nil {
					return ""
				}
				return app.state.Queue.Fingerprint
			}()),
			check("sealed_selection_accounting", app.state.Sample != nil && app.state.Plan != nil, func() string {
				if app.state.Sample == nil {
					return ""
				}
				return app.state.Sample.Fingerprint
			}()),
			check("processor_versions", app.state.Plan != nil && app.state.Plan.ComponentVersions["processing"] == processing.EvidenceMatcherVersion, func() string {
				if app.state.Plan == nil {
					return ""
				}
				return app.state.Plan.Fingerprint
			}()),
			check("manual_capacity_pinned", app.state.Plan != nil && app.state.Plan.Budgets.ManualSupportTolerance >= 0, func() string {
				if app.state.Plan == nil {
					return ""
				}
				return app.state.Plan.Fingerprint
			}()),
		},
	})
	preflightPassed := app.state.Preview != nil && app.state.Outbox != nil && (len(app.state.Outbox.Operations) == 0 || app.state.Preflight != nil)
	deliver := orchestration.EvaluateReadiness(orchestration.StageDeliver, orchestration.ReadinessContribution{
		ContributorID: "product_brain_batch", Version: productbrain.ApprovedDeliveryStateSchema,
		RequiredChecks: []string{"destination_identity", "mapping_schema_capacity", "live_preflight", "privacy_clear", "exact_batch_approval"},
		Checks: []orchestration.ReadinessCheck{
			check("destination_identity", app.connection != nil && app.profile != nil && app.state.KnownDestination != nil, func() string {
				if app.state.KnownDestination == nil {
					return ""
				}
				return acquisition.Fingerprint(app.state.KnownDestination.Identity)
			}()),
			check("mapping_schema_capacity", app.state.Outbox != nil && app.state.Preview != nil, func() string {
				if app.state.Outbox == nil {
					return ""
				}
				return app.state.Outbox.Fingerprint
			}()),
			check("live_preflight", preflightPassed, func() string {
				if app.state.Preflight == nil {
					if app.state.Preview != nil {
						return acquisition.Fingerprint(*app.state.Preview)
					}
					return ""
				}
				return app.state.Preflight.Fingerprint
			}()),
			check("privacy_clear", app.outboxPrivacyPassedLocked(), func() string {
				if app.state.Preview == nil {
					return ""
				}
				return app.state.Preview.PrivacyFingerprint
			}()),
			check("exact_batch_approval", app.state.Approval != nil, func() string {
				if app.state.Approval == nil {
					return ""
				}
				return app.state.Approval.Fingerprint
			}()),
		},
	})
	return []orchestration.ReadinessVerdict{inventory, process, deliver, app.readinessLocked(aggregate)}
}

func readinessAuthorizationSentences() map[orchestration.ReadinessStage]string {
	return map[orchestration.ReadinessStage]string{
		orchestration.StageInventory:         "inventory may start; processing and destination writes are unauthorized.",
		orchestration.StageProcess:           "the frozen capped proof may start; destination writes are unauthorized.",
		orchestration.StageExperimentalDrain: "exhaustive processing to this frozen watermark may start after confirmation; Product Brain delivery is unauthorized.",
		orchestration.StageDeliver:           "only the displayed batch fingerprint, destination, and budgets are authorized.",
	}
}

func (app *App) manualSupportOutcomeStateLocked() (int, bool) {
	reviews := map[string]processing.OperatorReviewRecord{}
	for _, review := range app.state.Reviews {
		reviews[review.CanonicalItemID] = review
	}
	count := 0
	for _, proposal := range app.state.Proposals {
		if !proposal.RequiresManualReview {
			continue
		}
		count++
		outcome := reviews[proposal.CanonicalItemID].ManualSupportOutcome
		if outcome != "queued_for_manual_processing" && outcome != "confirmed_unavailable" {
			return count, false
		}
	}
	return count, true
}

func (app *App) observedStrataOutcomeLocked() (bool, string) {
	if app.state.Sample == nil || app.state.ProofInventory == nil || len(app.state.Reviews) != len(app.state.Proposals) {
		return false, ""
	}
	items := map[string]acquisition.InventoryItem{}
	for _, item := range app.state.ProofInventory.CanonicalItems {
		items[item.CanonicalItemID] = item
	}
	counts := map[string]int{}
	for _, id := range app.state.Sample.SelectedItemIDs {
		item, ok := items[id]
		if !ok {
			return false, ""
		}
		counts[item.RetrievalStrategy+"\x00"+item.Format]++
	}
	for _, stratum := range app.state.Sample.Strata {
		if counts[stratum.RetrievalStrategyID+"\x00"+stratum.FormatVariant] != len(stratum.SelectedItemIDs) {
			return false, ""
		}
	}
	return true, acquisition.Fingerprint(struct {
		Sample string
		Counts map[string]int
	}{app.state.Sample.Fingerprint, counts})
}

func (app *App) outboxPrivacyPassedLocked() bool {
	return app.state.Outbox != nil && len(app.state.Outbox.PrivacyFindings) == 0 && app.state.Preview != nil && app.state.Preview.PrivacyFindingCount == 0
}

func (app *App) receiptHasChecks(names ...string) bool {
	if app.receipt == nil {
		return false
	}
	checks := map[string]bool{}
	for _, check := range app.receipt.Checks {
		checks[check.Name] = check.Outcome == "pass"
	}
	for _, name := range names {
		if !checks[name] {
			return false
		}
	}
	return true
}

func (app *App) projectedDrainResourcesLocked(manualSelected int) (bool, string, int) {
	if app.state.Plan == nil || app.state.Inventory == nil || app.state.Sample == nil {
		return false, "", 0
	}
	items, selected := len(app.state.Inventory.CanonicalItems), len(app.state.Sample.SelectedItemIDs)
	projectedManual := 0
	if selected > 0 {
		projectedManual = (manualSelected*items + selected - 1) / selected
	}
	projection := struct {
		Network, WallSeconds, Retries int
		CostMicrounits                int64
		Manual                        int
	}{items * 4, items * 15, items, int64(items) * 500, projectedManual}
	budget := app.state.Plan.Budgets
	passed := projection.Network <= budget.MaximumNetworkRequests && projection.WallSeconds <= budget.MaximumWallTimeSeconds && projection.Retries <= budget.MaximumRetryAttempts && projection.CostMicrounits <= budget.MaximumCostMicrounits
	return passed, acquisition.Fingerprint(struct {
		Projection any
		Budget     orchestration.RunBudgets
	}{projection, budget}), projectedManual
}

func (app *App) trustedActivationLocked() bool {
	if app.state.FounderReview == nil || app.state.Inventory == nil || app.state.Sample == nil {
		return false
	}
	if app.state.ZeroDelivery != nil {
		return app.state.ZeroDelivery.Status == "zero_draft_activation_recorded" && app.state.FounderReview.ZeroDraft
	}
	return app.state.Delivery != nil && app.state.Delivery.Status == "completed" && !app.state.FounderReview.ZeroDraft
}

func (app *App) drainConfirmationValidLocked(aggregate orchestration.Aggregate) bool {
	confirmation := app.state.DrainConfirmation
	if confirmation == nil || app.state.Plan == nil || app.state.Queue == nil {
		return false
	}
	fingerprint := confirmation.Fingerprint
	copyConfirmation := *confirmation
	copyConfirmation.Fingerprint = ""
	return confirmation.SchemaVersion == DrainConfirmationSchema && fingerprint != "" && fingerprint == acquisition.Fingerprint(copyConfirmation) &&
		confirmation.RunPlanFingerprint == app.state.Plan.Fingerprint && confirmation.QueueFingerprint == app.state.Queue.Fingerprint &&
		strings.TrimSpace(confirmation.SessionFingerprint) != "" && strings.TrimSpace(confirmation.NonceFingerprint) != "" &&
		aggregate.AuthorityReferences["drain_confirmation"] == fingerprint
}

func (app *App) recordFounderReviewLocked(ctx context.Context, payload founderReviewPayload) (any, error) {
	if strings.TrimSpace(payload.UsefulnessReason) == "" || strings.TrimSpace(payload.CredentialBurden) == "" || strings.TrimSpace(payload.ManualSupportBurden) == "" || strings.TrimSpace(payload.ApprovalBurden) == "" {
		return nil, errors.New("founder burden review incomplete")
	}
	startedAt, startErr := time.Parse(time.RFC3339Nano, payload.DiscoveryMetrics.StartedAt)
	submittedAt, submitErr := time.Parse(time.RFC3339Nano, payload.DiscoveryMetrics.SubmittedAt)
	if startErr != nil || submitErr != nil || submittedAt.Before(startedAt) || payload.DiscoveryMetrics.ElapsedMilliseconds < 0 || payload.DiscoveryMetrics.Errors < 0 || payload.DiscoveryMetrics.Retries < 0 || payload.DiscoveryMetrics.Backtracks < 0 || payload.DiscoveryMetrics.HelpRequests < 0 {
		return nil, errors.New("founder discovery metrics are invalid")
	}
	expectedReceipt := ""
	if app.state.Delivery != nil {
		expectedReceipt = app.state.Delivery.Fingerprint
	} else if app.state.ZeroDelivery != nil {
		expectedReceipt = app.state.ZeroDelivery.Fingerprint
	}
	if expectedReceipt == "" || payload.ZeroDraft != (app.state.ZeroDelivery != nil) {
		return nil, errors.New("delivery receipt required")
	}
	if payload.ZeroDraft {
		if payload.ReceiptFingerprint != expectedReceipt || len(payload.UsefulDraftIDs) != 0 || payload.ValueVerdict != "zero_draft" {
			return nil, errors.New("zero draft review cannot claim delivered value")
		}
	} else if payload.ReceiptFingerprint != expectedReceipt {
		return nil, errors.New("founder review receipt mismatch")
	}
	sort.Strings(payload.UsefulDraftIDs)
	if !payload.ZeroDraft {
		if payload.ValueVerdict != "useful" && payload.ValueVerdict != "not_useful" {
			return nil, errors.New("founder value verdict is required")
		}
		acknowledged := map[string]bool{}
		for _, id := range app.state.Delivery.RemoteObjectIDs {
			acknowledged[id] = true
		}
		for index, id := range payload.UsefulDraftIDs {
			if strings.TrimSpace(id) == "" || !acknowledged[id] || index > 0 && payload.UsefulDraftIDs[index-1] == id {
				return nil, errors.New("founder value must reference unique acknowledged Product Brain drafts")
			}
		}
		if payload.ValueVerdict == "useful" && (len(payload.UsefulDraftIDs) == 0 || strings.TrimSpace(payload.UsefulnessReason) == "") {
			return nil, errors.New("useful verdict requires selected acknowledged drafts and a reason")
		}
		if payload.ValueVerdict == "not_useful" && len(payload.UsefulDraftIDs) != 0 {
			return nil, errors.New("not-useful verdict cannot select useful drafts")
		}
	}
	valueObserved := !payload.ZeroDraft && payload.ValueVerdict == "useful" && len(payload.UsefulDraftIDs) > 0 && strings.TrimSpace(payload.UsefulnessReason) != ""
	if valueObserved != (payload.DiscoveryMetrics.TimeToTrustedValueMillis != nil) {
		return nil, errors.New("time-to-trusted-value metric does not match the founder verdict")
	}
	review := FounderReview{
		SchemaVersion: FounderReviewSchema, ReceiptFingerprint: payload.ReceiptFingerprint, UsefulDraftIDs: append([]string(nil), payload.UsefulDraftIDs...),
		ValueVerdict:     payload.ValueVerdict,
		UsefulnessReason: strings.TrimSpace(payload.UsefulnessReason), CredentialBurden: strings.TrimSpace(payload.CredentialBurden),
		ManualSupportBurden: strings.TrimSpace(payload.ManualSupportBurden), ApprovalBurden: strings.TrimSpace(payload.ApprovalBurden),
		ZeroDraft: payload.ZeroDraft, ReviewedAt: app.now().UTC().Format(time.RFC3339Nano), DiscoveryMetrics: payload.DiscoveryMetrics,
	}
	review.Fingerprint = acquisition.Fingerprint(review)
	app.state.FounderReview = &review
	if err := app.commitAuthorityLocked(ctx, "founder_review", review.Fingerprint); err != nil {
		return nil, err
	}
	if err := app.proveRecoveryLocked(ctx); err != nil {
		return nil, err
	}
	return review, nil
}

func decodePayload(value, target any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func syntheticInventory(snapshot acquisition.InventorySnapshot) bool {
	identity := strings.ToLower(snapshot.SourceIdentity)
	return strings.Contains(identity, "synthetic") || strings.Contains(identity, "sentinel")
}

var _ controlui.Application = (*App)(nil)
