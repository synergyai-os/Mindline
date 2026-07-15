package orchestration

import (
	"encoding/json"
	"fmt"
	"time"
)

type CommandKind string

const (
	CommandConfigure         CommandKind = "configure"
	CommandStartInventory    CommandKind = "start_inventory"
	CommandFreezeInventory   CommandKind = "freeze_inventory"
	CommandSelectProof       CommandKind = "select_proof"
	CommandStartProof        CommandKind = "start_proof"
	CommandCompleteProof     CommandKind = "complete_proof"
	CommandConfirmDrain      CommandKind = "confirm_experimental_drain"
	CommandStartDrain        CommandKind = "start_drain_processing"
	CommandSealQueue         CommandKind = "seal_queue"
	CommandPause             CommandKind = "pause"
	CommandResume            CommandKind = "resume"
	CommandCancel            CommandKind = "cancel"
	CommandRequireCredential CommandKind = "require_credential"
	CommandRestoreCredential CommandKind = "restore_credential"
)

type Command struct {
	Kind            CommandKind
	ExpectedVersion uint64
	Plan            *RunPlan
	Now             time.Time
}

func HandleCommand(aggregate Aggregate, command Command) (Event, error) {
	if aggregate.RunID == "" || command.ExpectedVersion != aggregate.Version {
		return Event{}, ErrVersionConflict
	}
	if command.Plan == nil || ValidateRunPlan(*command.Plan) != nil {
		return Event{}, ErrInvalidRunPlan
	}
	if aggregate.PlanFingerprint != "" && (command.Plan.Fingerprint != aggregate.PlanFingerprint || !equalStringMaps(command.Plan.ComponentVersions, aggregate.ComponentVersions)) {
		return Event{}, ErrConfigurationDrift
	}
	target, err := commandTarget(aggregate, command.Kind)
	if err != nil {
		return Event{}, err
	}
	if !aggregate.transitionAllowed(target) {
		return Event{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, aggregate.State, target)
	}
	now := command.Now
	if now.IsZero() {
		return Event{}, ErrInvalidEvent
	}
	payload := TransitionPayload{From: aggregate.State, To: target, PlanFingerprint: command.Plan.Fingerprint, ComponentVersions: cloneStringsMap(command.Plan.ComponentVersions)}
	if command.Kind == CommandConfigure {
		plan := *command.Plan
		plan.ComponentVersions = cloneStringsMap(command.Plan.ComponentVersions)
		payload.RunPlan = &plan
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	event := Event{SchemaVersion: EventSchemaVersion, RunID: aggregate.RunID, Sequence: aggregate.Version + 1, Type: EventRunTransition, OccurredAt: now.UTC().Format(time.RFC3339Nano), Payload: raw}
	if _, err := DecodeTransitionPayload(event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func commandTarget(aggregate Aggregate, kind CommandKind) (RunState, error) {
	switch kind {
	case CommandConfigure:
		return StateConfigured, nil
	case CommandStartInventory:
		return StateInventorying, nil
	case CommandFreezeInventory:
		return StateInventoryFrozen, nil
	case CommandSelectProof:
		return StateProofSelected, nil
	case CommandStartProof:
		return StateProofProcessing, nil
	case CommandCompleteProof:
		return StateProofComplete, nil
	case CommandConfirmDrain:
		return StateDrainConfirmed, nil
	case CommandStartDrain:
		return StateDrainProcessing, nil
	case CommandSealQueue:
		return StateQueueSealed, nil
	case CommandPause:
		return StatePaused, nil
	case CommandResume:
		if aggregate.State != StatePaused || aggregate.ResumeState == "" {
			return "", ErrIllegalTransition
		}
		return aggregate.ResumeState, nil
	case CommandCancel:
		return StateCancelled, nil
	case CommandRequireCredential:
		return StateCredentialRequired, nil
	case CommandRestoreCredential:
		if aggregate.State != StateCredentialRequired || aggregate.ResumeState == "" {
			return "", ErrIllegalTransition
		}
		return aggregate.ResumeState, nil
	default:
		return "", ErrIllegalTransition
	}
}
