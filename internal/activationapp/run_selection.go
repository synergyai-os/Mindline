package activationapp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/controlrun"
	"github.com/synergyai-os/Mindline/internal/controlsettings"
	"github.com/synergyai-os/Mindline/internal/controlui"
	"github.com/synergyai-os/Mindline/internal/integrations"
	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/runjournal"
)

var errExplicitRunRequired = errors.New("an explicit compatible run is required")

func newStableControlApp(options Options) (*App, error) {
	controlRoot := strings.TrimSpace(options.ControlRoot)
	if controlRoot == "" {
		controlRoot = strings.TrimSpace(options.RuntimeRoot)
	}
	if controlRoot == "" || !filepath.IsAbs(controlRoot) {
		return nil, errors.New("missing stable control root")
	}
	if err := privateio.PrepareDir(controlRoot); err != nil {
		return nil, err
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	configuration := strings.TrimSpace(options.ConfigurationFingerprint)
	if configuration == "" {
		configuration = DefaultConfigurationFingerprint()
	}
	preLiveReady := false
	if options.SyntheticOnly {
		preLiveReady = true
	} else if options.PreLiveReceipt != nil && strings.TrimSpace(options.Commit) != "" {
		preLiveReady = assurance.Validate(*options.PreLiveReceipt, strings.TrimSpace(options.Commit), configuration) == nil
	}
	connector := options.Connector
	if connector == nil && !options.SyntheticOnly {
		connector = productionConnector{}
	}
	sourceConnector := options.SourceConnector
	if sourceConnector == nil {
		sourceConnector = productionSlackSourceConnector{}
	}
	settings := options.SettingsRepository
	if settings == nil {
		created, err := controlsettings.NewRepository(controlRoot, controlSettingsOptions(now))
		if err != nil {
			return nil, err
		}
		settings = created
	}
	runSelection := options.RunRepository
	if runSelection == nil {
		created, err := controlrun.NewRepository(controlRoot, controlrun.Options{})
		if err != nil {
			return nil, err
		}
		runSelection = created
	}
	humanAuthority, err := controlui.NewHumanAuthority()
	if err != nil {
		return nil, err
	}
	entropy := options.RunEntropy
	if entropy == nil {
		entropy = rand.Reader
	}
	app := &App{
		controlRoot: controlRoot, now: now, synthetic: options.SyntheticOnly,
		preLiveReady: preLiveReady, commit: strings.TrimSpace(options.Commit), configuration: configuration,
		receipt:   options.PreLiveReceipt,
		connector: connector, sourceConnector: sourceConnector,
		registry: integrations.NewSessionRegistry(integrations.RegistryOptions{}),
		settings: settings, runSelection: runSelection, runEntropy: entropy,
		humanAuthority: humanAuthority,
	}
	selection, err := runSelection.Load(context.Background())
	if err != nil {
		app.registry.Shutdown()
		return nil, err
	}
	if selection.State == controlrun.StateRecoveryRequired {
		app.runBlockerCode = "selection_recovery_required"
		if selection.Problem != nil {
			app.runBlockerCode = "selection_" + selection.Problem.Code
		}
		return app, nil
	}
	if selection.Document.SelectedRunID == "" {
		return app, nil
	}
	if err := app.activateRunLocked(selection.Document.SelectedRunID); err != nil {
		app.deactivateRunLocked()
		app.runBlockerCode = classifySelectedRunError(err)
	}
	return app, nil
}

func classifySelectedRunError(err error) string {
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, controlrun.ErrRunNotFound) {
		return "selected_run_unavailable"
	}
	return "selected_run_incompatible"
}

func (app *App) activateRunLocked(runID string) error {
	if app.runSelection == nil || app.controlRoot == "" || app.service != nil {
		return errors.New("run activation unavailable")
	}
	if err := controlrun.ValidateRunID(runID); err != nil {
		return err
	}
	runRoot := filepath.Join(app.controlRoot, "runs", runID)
	info, err := os.Lstat(runRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != privateio.DirMode || privateio.ValidateContained(app.controlRoot, runRoot) != nil {
		return controlrun.ErrRunNotFound
	}
	journal, err := runjournal.NewStore(filepath.Join(runRoot, "run-journal"), runjournal.StoreOptions{})
	if err != nil {
		return err
	}
	leaseManager, err := runjournal.NewLeaseManager(filepath.Join(runRoot, "run-journal"), app.now)
	if err != nil {
		return err
	}
	lease, err := leaseManager.Acquire(context.Background(), orchestration.RunID(runID), fmt.Sprintf("activation-%d-%d", os.Getpid(), app.now().UTC().UnixNano()), 30*time.Second)
	if err != nil {
		return err
	}
	app.runtimeRoot = runRoot
	app.journal = journal
	app.service = orchestration.NewActivationService(journal, app.now)
	app.leaseManager, app.lease = leaseManager, lease
	app.leaseErr = nil
	app.loadedPersistedState = false
	app.state = persistedState{SchemaVersion: StateSchemaVersion, RunID: orchestration.RunID(runID), BuildCommit: app.commit, Configuration: app.configuration}
	if app.receipt != nil {
		app.state.PreLiveReceipt = app.receipt.Fingerprint
		app.state.PreLiveAuthorizations = []string{app.receipt.Fingerprint}
	}
	app.startLeaseRenewal()
	if err := app.loadState(); err != nil {
		return err
	}
	if err := app.reconcileRunProjection(context.Background()); err != nil {
		return err
	}
	if !app.synthetic && app.preLiveReady && app.receipt != nil {
		if err := app.commitAuthorityLocked(context.Background(), "pre_live_authorization", app.receipt.Fingerprint); err != nil {
			return err
		}
	}
	app.runBlockerCode = ""
	return nil
}

func (app *App) deactivateRunLocked() {
	app.stopLeaseRenewal()
	if app.leaseManager != nil && app.lease.Fingerprint != "" {
		_ = app.leaseManager.Release(context.Background(), app.lease)
	}
	if app.deliveryCancel != nil {
		app.deliveryCancel()
	}
	if app.connection != nil && app.connection.Disconnect != nil {
		_ = app.connection.Disconnect()
	}
	if app.sourceConnection != nil && app.sourceConnection.Disconnect != nil {
		_ = app.sourceConnection.Disconnect()
	}
	if app.registry != nil {
		app.registry.Shutdown()
	}
	app.registry = integrations.NewSessionRegistry(integrations.RegistryOptions{})
	app.connection = nil
	app.sourceConnection = nil
	app.profile = nil
	app.deliveryCancel = nil
	app.deliveryInFlight = false
	app.journal = nil
	app.service = nil
	app.leaseManager = nil
	app.lease = runjournal.Lease{}
	app.leaseErr = nil
	app.runtimeRoot = ""
	app.loadedPersistedState = false
	app.state = persistedState{}
}

func (app *App) canLeaveActiveRunLocked() bool {
	if app.service == nil {
		return true
	}
	if app.deliveryInFlight {
		return false
	}
	if app.state.Approval == nil {
		return true
	}
	return app.state.Delivery != nil || app.state.ZeroDelivery != nil || app.state.Cancellation != nil
}

func (app *App) restorePriorRunLocked(runID string) {
	if runID == "" {
		return
	}
	if err := app.activateRunLocked(runID); err != nil {
		app.deactivateRunLocked()
		app.runBlockerCode = classifySelectedRunError(err)
	}
}

// CreateRun reserves a new immutable run and snapshots one exact saved settings
// revision before changing the non-authorizing selection pointer. It performs
// no source or destination provider operation.
func (app *App) CreateRun(ctx context.Context, expectedSelection controlrun.Revision, settingsRevision controlsettings.Revision, settingsFingerprint string) (controlrun.Snapshot, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.runSelection == nil || app.controlRoot == "" || !app.canLeaveActiveRunLocked() {
		return controlrun.Snapshot{}, errors.New("new proof is blocked")
	}
	if !app.transportAuthorizedLocked() {
		return controlrun.Snapshot{}, errors.New("live gate required")
	}
	selection, err := app.runSelection.Load(ctx)
	if err != nil {
		return controlrun.Snapshot{}, err
	}
	if selection.State == controlrun.StateRecoveryRequired {
		return selection, controlrun.ErrRecoveryRequired
	}
	if selection.Document.Revision() != expectedSelection {
		return selection, controlrun.ErrConflict
	}
	priorRunID := selection.Document.SelectedRunID
	if app.service != nil && string(app.state.RunID) != priorRunID {
		return controlrun.Snapshot{}, errors.New("active run selection mismatch")
	}
	settings, err := app.settings.Load(ctx)
	if err != nil {
		return controlrun.Snapshot{}, err
	}
	if settings.State != controlsettings.StateSaved || settings.Document.Revision() != settingsRevision || settings.Document.Fingerprint != settingsFingerprint {
		return controlrun.Snapshot{}, controlsettings.ErrConflict
	}
	runID, err := controlrun.NewRunID(app.now(), app.runEntropy)
	if err != nil {
		return controlrun.Snapshot{}, err
	}
	if _, err := controlrun.ReserveRun(app.controlRoot, runID); err != nil {
		return controlrun.Snapshot{}, err
	}
	if app.service != nil {
		app.deactivateRunLocked()
	}
	if err := app.activateRunLocked(runID); err != nil {
		app.deactivateRunLocked()
		app.restorePriorRunLocked(priorRunID)
		return controlrun.Snapshot{}, err
	}
	if _, err := app.applySettingsLocked(ctx, useSettingsPayload{
		SettingsVersion: settingsRevision.Version, SettingsGeneration: settingsRevision.Generation,
		SettingsFingerprint: settingsFingerprint,
	}); err != nil {
		app.deactivateRunLocked()
		app.restorePriorRunLocked(priorRunID)
		return controlrun.Snapshot{}, err
	}
	updated, err := app.runSelection.CompareAndSwap(ctx, expectedSelection, runID)
	if err != nil {
		app.deactivateRunLocked()
		app.restorePriorRunLocked(priorRunID)
		return updated, err
	}
	return updated, nil
}

func (app *App) SelectRun(ctx context.Context, expected controlrun.Revision, runID string) (controlrun.Snapshot, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.runSelection == nil || !app.canLeaveActiveRunLocked() {
		return controlrun.Snapshot{}, errors.New("run selection is blocked")
	}
	if app.service != nil && string(app.state.RunID) == runID {
		return controlrun.Snapshot{}, errors.New("run is already selected")
	}
	updated, err := app.runSelection.CompareAndSwap(ctx, expected, runID)
	if err != nil {
		return updated, err
	}
	if app.service != nil {
		app.deactivateRunLocked()
	}
	if err := app.activateRunLocked(runID); err != nil {
		app.deactivateRunLocked()
		app.runBlockerCode = classifySelectedRunError(err)
		return updated, nil
	}
	return updated, nil
}

func (app *App) RecoverRunSelection(ctx context.Context, problemFingerprint string, expected *controlrun.Revision, acknowledgement, runID string) (controlrun.Snapshot, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.runSelection == nil || app.deliveryInFlight || app.service != nil {
		return controlrun.Snapshot{}, errors.New("run selection recovery is blocked")
	}
	updated, err := app.runSelection.Recover(ctx, problemFingerprint, expected, acknowledgement, runID)
	if err != nil {
		return updated, err
	}
	app.runBlockerCode = ""
	if runID == "" {
		return updated, nil
	}
	if err := app.activateRunLocked(runID); err != nil {
		app.deactivateRunLocked()
		app.runBlockerCode = classifySelectedRunError(err)
	}
	return updated, nil
}

func (app *App) runSelectionView(snapshot controlrun.Snapshot) RunSelectionView {
	view := RunSelectionView{
		Version: snapshot.Document.Version, Generation: snapshot.Document.Generation,
		SelectedRunID: snapshot.Document.SelectedRunID, SafePriorRun: snapshot.Document.SelectedRunID,
	}
	switch {
	case snapshot.State == controlrun.StateRecoveryRequired:
		view.State = "blocked"
		if snapshot.Problem != nil {
			view.ReasonCode = snapshot.Problem.Code
			view.ProblemFingerprint = snapshot.Problem.Fingerprint
			view.BackupAvailable = snapshot.Problem.BackupAvailable
			if snapshot.Problem.ReadableRevision != nil {
				view.ReadableVersion = snapshot.Problem.ReadableRevision.Version
				view.ReadableGeneration = snapshot.Problem.ReadableRevision.Generation
			}
		}
	case snapshot.Document.SelectedRunID == "":
		view.State = "none"
	case app.service != nil && string(app.state.RunID) == snapshot.Document.SelectedRunID:
		view.State = "compatible_selected"
	case app.runBlockerCode == "selected_run_incompatible":
		view.State = "incompatible_preserved"
		view.ReasonCode = app.runBlockerCode
	default:
		view.State = "blocked"
		view.ReasonCode = app.runBlockerCode
	}
	return view
}
