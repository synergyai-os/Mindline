// Package founderreview persists the founder's single qualitative outcome for
// a completed recall proof. It stores only opaque commitments over private
// evidence, never source, query, or citation content or identifiers.
package founderreview

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	SchemaVersion        = "mindline-founder-review/v1"
	MaxRecordBytes int64 = 4 * 1024
)

var (
	ErrAlreadyRecorded = errors.New("founder review is already recorded")
	ErrRetryConflict   = errors.New("founder review retry token conflicts with recorded intent")
	ErrInvalid         = errors.New("founder review is invalid")
	ErrNotFound        = errors.New("founder review is not recorded")
	ErrUnavailable     = errors.New("founder review storage is unavailable")
	ErrLockBusy        = privateio.ErrLockBusy
)

// Verdict is the founder's sole qualitative judgment for this proof.
type Verdict string

const (
	VerdictUseful    Verdict = "useful"
	VerdictNotUseful Verdict = "not_useful"
	VerdictDeclined  Verdict = "declined"
)

// Resolution is the only product-lifecycle effect exposed by this package.
// Only ResolutionClosed may close the user-value outcome.
type Resolution string

const (
	ResolutionClosed                Resolution = "user_value_closed"
	ResolutionReturnToDiagnoseShape Resolution = "return_to_diagnose_shape"
	ResolutionUnverified            Resolution = "user_value_unverified"
)

// Resolution returns a closed user-value result only for an explicitly useful
// review. A negative judgment must return to Diagnose/Shape; a declined review
// remains deliberately unverified.
func (verdict Verdict) Resolution() Resolution {
	switch verdict {
	case VerdictUseful:
		return ResolutionClosed
	case VerdictNotUseful:
		return ResolutionReturnToDiagnoseShape
	default:
		return ResolutionUnverified
	}
}

func (verdict Verdict) ClosesUserValue() bool {
	return verdict.Resolution() == ResolutionClosed
}

// Record contains only structural proof binding and the founder's enum. The
// recorded timestamp is private runtime state and must never enter exported
// proof artifacts.
type Record struct {
	SchemaVersion              string  `json:"schema_version"`
	RunID                      string  `json:"run_id"`
	ProofRunID                 string  `json:"proof_run_id"`
	StructuralProofFingerprint string  `json:"structural_proof_fingerprint"`
	CitedRecordsFingerprint    string  `json:"cited_records_fingerprint"`
	Verdict                    Verdict `json:"verdict"`
	EventID                    string  `json:"event_id"`
	RetryTokenHash             string  `json:"retry_token_hash"`
	RecordedAt                 string  `json:"recorded_at"`
}

func (record Record) Resolution() Resolution { return record.Verdict.Resolution() }

func (record Record) ClosesUserValue() bool { return record.Verdict.ClosesUserValue() }

// Receipt is the only export-safe projection of a founder review. It binds the
// proof, proof run, and cited-record commitment but intentionally excludes the
// private timestamp, retry token hash, review-run ID, and all raw evidence.
type Receipt struct {
	SchemaVersion              string     `json:"schema_version"`
	ProofRunID                 string     `json:"proof_run_id"`
	StructuralProofFingerprint string     `json:"structural_proof_fingerprint"`
	CitedRecordsFingerprint    string     `json:"cited_records_fingerprint"`
	Verdict                    Verdict    `json:"verdict"`
	EventID                    string     `json:"event_id"`
	Resolution                 Resolution `json:"resolution"`
}

func (record Record) Receipt() Receipt {
	return Receipt{
		SchemaVersion:              record.SchemaVersion,
		ProofRunID:                 record.ProofRunID,
		StructuralProofFingerprint: record.StructuralProofFingerprint,
		CitedRecordsFingerprint:    record.CitedRecordsFingerprint,
		Verdict:                    record.Verdict,
		EventID:                    record.EventID,
		Resolution:                 record.Resolution(),
	}
}

type Request struct {
	// ProofRunID and CitedRecordsFingerprint bind the outcome to the exact
	// owner-only proof run and cited-record set without persisting those IDs.
	ProofRunID                 string
	StructuralProofFingerprint string
	CitedRecordsFingerprint    string
	Verdict                    Verdict
	// RetryToken is an opaque caller-held token. Only its SHA-256 hash is
	// persisted, so retried intent is idempotent without retaining the token.
	RetryToken string
}

type Options struct {
	Now     func() time.Time
	Entropy io.Reader
	Faults  privateio.FaultInjector
}

// RepositoryPort makes the bounded founder review independently consumable by
// the FEAT-29 proof orchestration without coupling it to a CLI or UI.
type RepositoryPort interface {
	Create(context.Context, Request) (Record, error)
	Load(context.Context) (Record, error)
}
