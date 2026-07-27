package orchestration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type RunState string

const (
	StateConfigured         RunState = "configured"
	StateInventorying       RunState = "inventorying"
	StateInventoryFrozen    RunState = "inventory_frozen"
	StateProofSelected      RunState = "proof_selected"
	StateProofProcessing    RunState = "proof_processing"
	StateProofComplete      RunState = "proof_complete"
	StateDrainConfirmed     RunState = "drain_confirmed"
	StateDrainProcessing    RunState = "drain_processing"
	StateQueueSealed        RunState = "queue_sealed"
	StatePaused             RunState = "paused"
	StateCredentialRequired RunState = "credential_required"
	StateCancelled          RunState = "cancelled"
)

type EventType string

const (
	EventRunTransition      EventType = "run_transition"
	EventAuthorityReference EventType = "authority_reference"
)

type Event struct {
	SchemaVersion string          `json:"schema_version"`
	RunID         RunID           `json:"run_id"`
	Sequence      uint64          `json:"sequence"`
	EventID       string          `json:"event_id,omitempty"`
	Type          EventType       `json:"type"`
	OccurredAt    string          `json:"occurred_at"`
	PreviousHash  string          `json:"previous_hash,omitempty"`
	Payload       json.RawMessage `json:"payload"`
	PayloadHash   string          `json:"payload_hash,omitempty"`
	EventHash     string          `json:"event_hash,omitempty"`
}

type TransitionPayload struct {
	From              RunState          `json:"from,omitempty"`
	To                RunState          `json:"to"`
	PlanFingerprint   string            `json:"plan_fingerprint"`
	ComponentVersions map[string]string `json:"component_versions"`
	RunPlan           *RunPlan          `json:"run_plan,omitempty"`
}

type AuthorityReferencePayload struct {
	Domain                string `json:"domain"`
	ArtifactFingerprint   string `json:"artifact_fingerprint"`
	ProjectionFingerprint string `json:"projection_fingerprint"`
}

type Aggregate struct {
	RunID                         RunID
	Version                       uint64
	State                         RunState
	ResumeState                   RunState
	PlanFingerprint               string
	ComponentVersions             map[string]string
	AuthorityReferences           map[string]string
	AuthorityProjectionReferences map[string]string
	LatestAuthorityProjection     string
}

func (aggregate *Aggregate) Apply(event Event) error {
	if aggregate == nil || aggregate.RunID == "" || event.RunID != aggregate.RunID || event.Sequence != aggregate.Version+1 {
		return ErrInvalidEvent
	}
	if event.Type == EventAuthorityReference {
		payload, err := DecodeAuthorityReferencePayload(event)
		if err != nil {
			return err
		}
		if aggregate.AuthorityReferences == nil {
			aggregate.AuthorityReferences = map[string]string{}
		}
		if aggregate.AuthorityProjectionReferences == nil {
			aggregate.AuthorityProjectionReferences = map[string]string{}
		}
		aggregate.AuthorityReferences[payload.Domain] = payload.ArtifactFingerprint
		aggregate.AuthorityProjectionReferences[payload.Domain] = payload.ProjectionFingerprint
		aggregate.LatestAuthorityProjection = payload.ProjectionFingerprint
		aggregate.Version = event.Sequence
		return nil
	}
	payload, err := DecodeTransitionPayload(event)
	if err != nil {
		return err
	}
	if payload.From != aggregate.State || !aggregate.transitionAllowed(payload.To) {
		return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, aggregate.State, payload.To)
	}
	if aggregate.PlanFingerprint == "" {
		if payload.RunPlan == nil || ValidateRunPlan(*payload.RunPlan) != nil || payload.RunPlan.Fingerprint != payload.PlanFingerprint {
			return ErrInvalidEvent
		}
		aggregate.PlanFingerprint = payload.RunPlan.Fingerprint
		aggregate.ComponentVersions = cloneStringsMap(payload.RunPlan.ComponentVersions)
	} else if payload.RunPlan != nil || payload.PlanFingerprint != aggregate.PlanFingerprint || !equalStringMaps(payload.ComponentVersions, aggregate.ComponentVersions) {
		return ErrConfigurationDrift
	}
	previous := aggregate.State
	if payload.To == StatePaused || payload.To == StateCredentialRequired {
		aggregate.ResumeState = previous
	} else if previous == StatePaused || previous == StateCredentialRequired {
		if payload.To != aggregate.ResumeState {
			return ErrIllegalTransition
		}
		aggregate.ResumeState = ""
	}
	if payload.To == StateCancelled {
		aggregate.ResumeState = ""
	}
	aggregate.State = payload.To
	aggregate.Version = event.Sequence
	return nil
}

func DecodeAuthorityReferencePayload(event Event) (AuthorityReferencePayload, error) {
	if event.SchemaVersion != EventSchemaVersion || event.Type != EventAuthorityReference || event.RunID == "" || event.Sequence == 0 || event.OccurredAt == "" {
		return AuthorityReferencePayload{}, ErrInvalidEvent
	}
	if _, err := time.Parse(time.RFC3339Nano, event.OccurredAt); err != nil {
		return AuthorityReferencePayload{}, ErrInvalidEvent
	}
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.DisallowUnknownFields()
	var payload AuthorityReferencePayload
	if err := decoder.Decode(&payload); err != nil {
		return AuthorityReferencePayload{}, ErrInvalidEvent
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || strings.TrimSpace(payload.Domain) == "" || strings.TrimSpace(payload.ArtifactFingerprint) == "" || strings.TrimSpace(payload.ProjectionFingerprint) == "" {
		return AuthorityReferencePayload{}, ErrInvalidEvent
	}
	allowed := map[string]bool{"pre_live_authorization": true, "source_connection": true, "source_window": true, "inventory": true, "strategy": true, "destination": true, "queue": true, "processing": true, "review": true, "outbox": true, "approval": true, "cancellation": true, "delivery": true, "delivery_resume": true, "founder_review": true, "recovery": true, "drain_confirmation": true, "readiness": true}
	if !allowed[payload.Domain] {
		return AuthorityReferencePayload{}, ErrInvalidEvent
	}
	return payload, nil
}

func (aggregate Aggregate) transitionAllowed(target RunState) bool {
	if aggregate.State == StateCancelled || aggregate.State == StateQueueSealed {
		return false
	}
	if target == StateCancelled {
		return aggregate.State != ""
	}
	if target == StatePaused || target == StateCredentialRequired {
		return aggregate.State != "" && aggregate.State != StatePaused && aggregate.State != StateCredentialRequired
	}
	if aggregate.State == StatePaused || aggregate.State == StateCredentialRequired {
		return target == aggregate.ResumeState
	}
	expected := map[RunState]RunState{
		"":                   StateConfigured,
		StateConfigured:      StateInventorying,
		StateInventorying:    StateInventoryFrozen,
		StateInventoryFrozen: StateProofSelected,
		StateProofSelected:   StateProofProcessing,
		StateProofProcessing: StateProofComplete,
		StateProofComplete:   StateDrainConfirmed,
		StateDrainConfirmed:  StateDrainProcessing,
		StateDrainProcessing: StateQueueSealed,
	}
	return expected[aggregate.State] == target
}

func Rebuild(runID RunID, events []Event) (Aggregate, error) {
	aggregate := Aggregate{RunID: runID}
	for _, event := range events {
		if err := aggregate.Apply(event); err != nil {
			return Aggregate{}, err
		}
	}
	return aggregate, nil
}

func DecodeTransitionPayload(event Event) (TransitionPayload, error) {
	if event.SchemaVersion != EventSchemaVersion || event.Type != EventRunTransition || event.RunID == "" || event.Sequence == 0 || event.OccurredAt == "" {
		return TransitionPayload{}, ErrInvalidEvent
	}
	if _, err := time.Parse(time.RFC3339Nano, event.OccurredAt); err != nil {
		return TransitionPayload{}, ErrInvalidEvent
	}
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.DisallowUnknownFields()
	var payload TransitionPayload
	if err := decoder.Decode(&payload); err != nil {
		return TransitionPayload{}, ErrInvalidEvent
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || payload.To == "" || payload.PlanFingerprint == "" || len(payload.ComponentVersions) == 0 {
		return TransitionPayload{}, ErrInvalidEvent
	}
	return payload, nil
}

func equalStringMaps(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
