package controlrun

import (
	"context"
	"errors"
	"io"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	SchemaVersion                 = "mindline.control-run-selection/v1"
	MaxDocumentBytes        int64 = 16 * 1024
	RecoveryAcknowledgement       = "I understand this changes only the explicit Mindline run selection pointer."
)

var (
	ErrConflict         = errors.New("control run selection revision conflict")
	ErrInvalid          = errors.New("control run selection is invalid")
	ErrRecoveryRequired = errors.New("control run selection recovery required")
	ErrNoRecovery       = errors.New("control run selection recovery is not required")
	ErrUnsupported      = errors.New("control run selection schema is unsupported")
	ErrRunNotFound      = errors.New("explicit run is not available")
	ErrLockBusy         = privateio.ErrLockBusy
)

type Revision struct {
	Version    uint64 `json:"version"`
	Generation string `json:"generation"`
}

type Document struct {
	SchemaVersion string `json:"schema_version"`
	Version       uint64 `json:"version"`
	Generation    string `json:"generation"`
	SelectedRunID string `json:"selected_run_id"`
	Fingerprint   string `json:"fingerprint"`
}

func (document Document) Revision() Revision {
	return Revision{Version: document.Version, Generation: document.Generation}
}

type State string

const (
	StateNone             State = "none"
	StateSelected         State = "selected"
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

type Options struct {
	Entropy io.Reader
	Faults  privateio.FaultInjector
}

type RepositoryPort interface {
	Load(context.Context) (Snapshot, error)
	CompareAndSwap(context.Context, Revision, string) (Snapshot, error)
	Recovery(context.Context) (Problem, error)
	Recover(context.Context, string, *Revision, string, string) (Snapshot, error)
}
