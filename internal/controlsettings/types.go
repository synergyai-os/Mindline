package controlsettings

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	SchemaVersion                 = "mindline.control-settings/v1"
	MaxDocumentBytes        int64 = 64 * 1024
	RecoveryAcknowledgement       = "I understand this replaces the persisted Mindline control settings."
)

var (
	ErrConflict         = errors.New("control settings revision conflict")
	ErrInvalid          = errors.New("control settings are invalid")
	ErrRecoveryRequired = errors.New("control settings recovery required")
	ErrNoRecovery       = errors.New("control settings recovery is not required")
	ErrUnsupported      = errors.New("control settings schema is unsupported")
	ErrLockBusy         = privateio.ErrLockBusy
)

type Revision struct {
	Version    uint64 `json:"version"`
	Generation string `json:"generation"`
}

type DrainPolicy struct {
	MaximumNetworkRequests int   `json:"maximum_network_requests"`
	MaximumWallTimeSeconds int   `json:"maximum_wall_time_seconds"`
	MaximumCostMicrounits  int64 `json:"maximum_cost_microunits"`
	MaximumRetryAttempts   int   `json:"maximum_retry_attempts"`
	ManualSupportTolerance int   `json:"manual_support_tolerance"`
}

type AdapterDefault struct {
	Slot          string          `json:"slot"`
	AdapterKind   string          `json:"adapter_kind"`
	SchemaVersion string          `json:"schema_version"`
	Values        json.RawMessage `json:"values"`
}

// ExpectedIdentity intentionally contains only the stable non-secret fields
// projected by a verified provider identity. It grants no live authority.
type ExpectedIdentity struct {
	AdapterKind       string `json:"adapter_kind"`
	WorkspaceID       string `json:"workspace_id"`
	ChannelID         string `json:"channel_id,omitempty"`
	KeyID             string `json:"key_id,omitempty"`
	CapabilityVersion string `json:"capability_version"`
}

type Draft struct {
	ContextLenses               []string          `json:"context_lenses"`
	RoutingPolicy               string            `json:"routing_policy"`
	DrainPolicy                 DrainPolicy       `json:"drain_policy"`
	AdapterDefaults             []AdapterDefault  `json:"adapter_defaults"`
	ExpectedSourceIdentity      *ExpectedIdentity `json:"expected_source_identity"`
	ExpectedDestinationIdentity *ExpectedIdentity `json:"expected_destination_identity"`
}

type Document struct {
	SchemaVersion string `json:"schema_version"`
	Version       uint64 `json:"version"`
	Generation    string `json:"generation"`
	SavedAt       string `json:"saved_at"`
	Fingerprint   string `json:"fingerprint"`
	Draft         Draft  `json:"draft"`
}

func (document Document) Revision() Revision {
	return Revision{Version: document.Version, Generation: document.Generation}
}

type State string

const (
	StateDefaults         State = "defaults"
	StateSaved            State = "saved"
	StateRecoveryRequired State = "recovery_required"
)

type Snapshot struct {
	State    State    `json:"state"`
	Document Document `json:"document"`
	Problem  *Problem `json:"problem,omitempty"`
}

type Problem struct {
	Fingerprint      string    `json:"fingerprint"`
	Code             string    `json:"code"`
	BackupAvailable  bool      `json:"backup_available"`
	ReadableRevision *Revision `json:"readable_revision,omitempty"`
}

type RecoveryAction string

const (
	RecoveryRestoreBackup   RecoveryAction = "restore_backup"
	RecoveryReplaceDefaults RecoveryAction = "replace_defaults"
)

type AdapterValidator interface {
	ValidateDefaults(schemaVersion string, strictRawJSON json.RawMessage) (json.RawMessage, error)
}

type AdapterValidatorFunc func(string, json.RawMessage) (json.RawMessage, error)

func (function AdapterValidatorFunc) ValidateDefaults(schemaVersion string, raw json.RawMessage) (json.RawMessage, error) {
	return function(schemaVersion, raw)
}

type Options struct {
	Now        func() time.Time
	Entropy    io.Reader
	Faults     privateio.FaultInjector
	Validators map[string]AdapterValidator
}

type RepositoryPort interface {
	Load(context.Context) (Snapshot, error)
	Save(context.Context, Revision, Draft) (Snapshot, error)
	Recovery(context.Context) (Problem, error)
	Recover(context.Context, string, string, RecoveryAction) (Snapshot, error)
}
