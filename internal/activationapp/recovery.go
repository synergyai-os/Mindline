package activationapp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/runjournal"
)

// proveRecoveryLocked independently reconstructs the current run from the
// immutable journal and its referenced authority snapshot. It proves this
// run's exact frozen inventory/sample/queue, rather than treating a generic
// recovery unit test or the mutable in-memory projection as runtime evidence.
func (app *App) proveRecoveryLocked(ctx context.Context) error {
	if app.state.Inventory == nil || app.state.Sample == nil || app.state.Queue == nil {
		return errors.New("frozen queue recovery proof is unavailable")
	}
	journalRoot := filepath.Join(app.runtimeRoot, "run-journal")
	independentStore, err := runjournal.NewStore(journalRoot, runjournal.StoreOptions{})
	if err != nil {
		return err
	}
	independentService := orchestration.NewActivationService(independentStore, app.now)
	rebuilt, err := independentService.Get(ctx, app.state.RunID)
	if err != nil {
		return fmt.Errorf("independent run journal reconstruction failed: %w", err)
	}
	if rebuilt.LatestAuthorityProjection == "" {
		return errors.New("run journal reconstruction failed")
	}
	path := filepath.Join(app.runtimeRoot, "activation-authority", strings.TrimPrefix(rebuilt.LatestAuthorityProjection, "sha256:")+".json")
	var recovered persistedState
	if err := privateio.ReadJSONStrictBounded(app.runtimeRoot, path, maximumActivationStateBytes, &recovered); err != nil {
		return err
	}
	if recovered.Fingerprint != rebuilt.LatestAuthorityProjection || recovered.Inventory == nil || recovered.Sample == nil || recovered.Queue == nil {
		return errors.New("recovered authority projection is incomplete")
	}
	if recovered.Inventory.Fingerprint != app.state.Inventory.Fingerprint || recovered.Sample.Fingerprint != app.state.Sample.Fingerprint || recovered.Queue.Fingerprint != app.state.Queue.Fingerprint {
		return errors.New("recovered frozen queue authority drifted")
	}
	if err := acquisition.ValidateInventory(*recovered.Inventory); err != nil {
		return fmt.Errorf("recovered inventory validation failed: %w", err)
	}
	orchestrationInventory, err := toOrchestrationInventory(*recovered.Inventory)
	if err != nil {
		return err
	}
	if err := orchestration.ValidateSampleManifest(orchestrationInventory, *recovered.Sample); err != nil {
		return fmt.Errorf("recovered sample validation failed: %w", err)
	}
	if err := validateReviewedQueueProjection(*recovered.Queue, *recovered.Inventory, *recovered.Sample); err != nil {
		return fmt.Errorf("recovered queue validation failed: %w", err)
	}
	proof := RecoveryProof{
		SchemaVersion: RecoveryProofSchema, RunID: app.state.RunID, JournalVersion: rebuilt.Version,
		RecoveredAuthorityProjection: rebuilt.LatestAuthorityProjection,
		InventoryFingerprint:         app.state.Inventory.Fingerprint, SampleFingerprint: app.state.Sample.Fingerprint,
		QueueFingerprint: app.state.Queue.Fingerprint, ProvenAt: app.now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
	proof.Fingerprint = acquisition.Fingerprint(proof)
	app.state.RecoveryProof = &proof
	return app.commitAuthorityLocked(ctx, "recovery", proof.Fingerprint)
}

func (app *App) recoveryProofValidLocked(aggregate orchestration.Aggregate) bool {
	proof := app.state.RecoveryProof
	if proof == nil || app.state.Inventory == nil || app.state.Sample == nil || app.state.Queue == nil {
		return false
	}
	fingerprint := proof.Fingerprint
	copyProof := *proof
	copyProof.Fingerprint = ""
	return proof.SchemaVersion == RecoveryProofSchema && fingerprint != "" && fingerprint == acquisition.Fingerprint(copyProof) &&
		proof.RunID == app.state.RunID && proof.InventoryFingerprint == app.state.Inventory.Fingerprint &&
		proof.SampleFingerprint == app.state.Sample.Fingerprint && proof.QueueFingerprint == app.state.Queue.Fingerprint &&
		aggregate.AuthorityReferences["recovery"] == fingerprint && aggregate.AuthorityProjectionReferences["recovery"] != ""
}
