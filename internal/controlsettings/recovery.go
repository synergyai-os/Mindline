package controlsettings

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

func (repository *Repository) Recover(ctx context.Context, problemFingerprintValue, acknowledgement string, action RecoveryAction) (Snapshot, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if acknowledgement != RecoveryAcknowledgement || (action != RecoveryRestoreBackup && action != RecoveryReplaceDefaults) {
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
	backupRaw, _ := privateio.ReadFileBounded(repository.root, repository.backupPath, MaxDocumentBytes)
	backup, backupErr := parseDocument(backupRaw, repository.validators)
	if action == RecoveryRestoreBackup && backupErr != nil {
		return current, ErrInvalid
	}
	draft := DefaultDraft()
	version := uint64(1)
	if backupErr == nil {
		if backup.Version == math.MaxUint64 {
			return current, ErrInvalid
		}
		version = backup.Version + 1
		if action == RecoveryRestoreBackup {
			draft = backup.Draft
		}
	}
	draft, err = canonicalizeDraft(draft, repository.validators)
	if err != nil {
		return current, ErrInvalid
	}
	generation, err := randomGeneration(repository.entropy)
	if err != nil {
		return current, err
	}
	document, _ := sealDocument(version, generation, repository.now(), draft)
	next, err := canonicalDocumentBytes(document)
	if err != nil {
		return current, ErrInvalid
	}
	validate := func(data []byte) error {
		_, err := parseDocument(data, repository.validators)
		return err
	}
	// Recovery never copies corrupt current bytes over the last-valid backup.
	if err := privateio.AtomicReplaceWithBackup(repository.root, repository.currentPath, repository.backupPath, next, nil, MaxDocumentBytes, validate, repository.faults); err != nil {
		return current, err
	}
	persisted, err := repository.loadUnlocked()
	if err != nil || persisted.State != StateSaved || persisted.Document.Fingerprint != document.Fingerprint {
		return Snapshot{}, privateio.ErrAtomicValidationFailed
	}
	return persisted, nil
}
