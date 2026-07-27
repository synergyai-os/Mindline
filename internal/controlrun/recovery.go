package controlrun

import (
	"context"
	"math"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

func (repository *Repository) Recovery(ctx context.Context) (Problem, error) {
	snapshot, err := repository.Load(ctx)
	if err != nil {
		return Problem{}, err
	}
	if snapshot.State != StateRecoveryRequired || snapshot.Problem == nil {
		return Problem{}, ErrNoRecovery
	}
	return *snapshot.Problem, nil
}

// Recover changes only the non-authorizing pointer. An empty explicitRunID
// means clear; a non-empty value must name an already reserved explicit run.
func (repository *Repository) Recover(ctx context.Context, problemFingerprintValue string, expected *Revision, acknowledgement, explicitRunID string) (Snapshot, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if acknowledgement != RecoveryAcknowledgement {
		return Snapshot{}, ErrInvalid
	}
	if err := repository.prepare(); err != nil {
		return Snapshot{}, err
	}
	lock, err := repository.lock()
	if err != nil {
		return Snapshot{}, err
	}
	defer lock.Close()
	current, err := repository.loadUnlocked()
	if err != nil {
		return Snapshot{}, err
	}
	if current.State != StateRecoveryRequired || current.Problem == nil {
		return current, ErrNoRecovery
	}
	if current.Problem.Fingerprint != problemFingerprintValue {
		return current, ErrConflict
	}
	if current.Problem.ReadableRevision != nil && (expected == nil || *expected != *current.Problem.ReadableRevision) {
		return current, ErrConflict
	}
	if err := validateReservedRun(repository.root, explicitRunID); err != nil {
		return current, err
	}
	backupRaw, _ := privateio.ReadFileBounded(repository.root, repository.backupPath, MaxDocumentBytes)
	backup, backupErr := parseDocument(backupRaw)
	version := uint64(1)
	if backupErr == nil {
		if backup.Version == math.MaxUint64 {
			return current, ErrInvalid
		}
		version = backup.Version + 1
	}
	generation, err := randomGeneration(repository.entropy)
	if err != nil {
		return current, err
	}
	document := sealDocument(version, generation, explicitRunID)
	next, err := canonicalDocumentBytes(document)
	if err != nil {
		return current, ErrInvalid
	}
	validate := func(data []byte) error { _, err := parseDocument(data); return err }
	if err := privateio.AtomicReplaceWithBackup(repository.root, repository.currentPath, repository.backupPath, next, nil, MaxDocumentBytes, validate, repository.faults); err != nil {
		return current, err
	}
	persisted, err := repository.loadUnlocked()
	if err != nil || persisted.State == StateRecoveryRequired || persisted.Document.Fingerprint != document.Fingerprint {
		return Snapshot{}, privateio.ErrAtomicValidationFailed
	}
	return persisted, nil
}
