package privateio

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

var (
	ErrAtomicWriteFailed      = errors.New("private atomic write failed")
	ErrAtomicValidationFailed = errors.New("private atomic validation failed")
)

// AtomicReplaceWithBackup atomically replaces currentPath and, when prior is
// non-empty, first persists the last-valid canonical bytes to backupPath. Both
// targets are owner-only regular files below root. Success includes a bounded
// no-follow reread and caller-supplied validation of the exact persisted bytes.
func AtomicReplaceWithBackup(root, currentPath, backupPath string, next, prior []byte, maxBytes int64, validate func([]byte) error, inject FaultInjector) error {
	if maxBytes <= 0 || int64(len(next)) > maxBytes || validate == nil {
		return ErrAtomicValidationFailed
	}
	if err := validate(next); err != nil {
		return ErrAtomicValidationFailed
	}
	if err := ValidateContained(root, currentPath, backupPath); err != nil {
		return ErrAtomicWriteFailed
	}
	if len(prior) > 0 {
		if int64(len(prior)) > maxBytes || validate(prior) != nil {
			return ErrAtomicValidationFailed
		}
		if inject.at(FaultBeforeBackupWrite) != nil {
			return ErrAtomicWriteFailed
		}
		if err := atomicReplaceFile(backupPath, prior, inject, true); err != nil {
			return err
		}
	}
	if inject.at(FaultBeforeCurrentWrite) != nil {
		return ErrAtomicWriteFailed
	}
	if err := atomicReplaceFile(currentPath, next, inject, false); err != nil {
		return err
	}
	if inject.at(FaultBeforeReread) != nil {
		return ErrAtomicWriteFailed
	}
	persisted, err := ReadFileBounded(root, currentPath, maxBytes)
	if err != nil || !bytes.Equal(persisted, next) || validate(persisted) != nil {
		return ErrAtomicValidationFailed
	}
	if inject.at(FaultAfterReread) != nil {
		return ErrAtomicWriteFailed
	}
	return nil
}

func atomicReplaceFile(path string, data []byte, inject FaultInjector, backup bool) error {
	dir := filepath.Dir(path)
	if err := PrepareDir(dir); err != nil {
		return ErrAtomicWriteFailed
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != FileMode {
			return ErrAtomicWriteFailed
		}
	} else if !os.IsNotExist(err) {
		return ErrAtomicWriteFailed
	}
	temp, err := os.CreateTemp(dir, ".mindline-atomic-*")
	if err != nil {
		return ErrAtomicWriteFailed
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(FileMode); err != nil {
		temp.Close()
		return ErrAtomicWriteFailed
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return ErrAtomicWriteFailed
	}
	if inject.at(map[bool]FaultPoint{true: FaultAfterBackupWrite, false: FaultAfterCurrentWrite}[backup]) != nil {
		temp.Close()
		return ErrAtomicWriteFailed
	}
	if inject.at(map[bool]FaultPoint{true: FaultBeforeBackupSync, false: FaultBeforeCurrentSync}[backup]) != nil {
		temp.Close()
		return ErrAtomicWriteFailed
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return ErrAtomicWriteFailed
	}
	if inject.at(map[bool]FaultPoint{true: FaultAfterBackupSync, false: FaultAfterCurrentSync}[backup]) != nil {
		temp.Close()
		return ErrAtomicWriteFailed
	}
	if err := temp.Close(); err != nil {
		return ErrAtomicWriteFailed
	}
	if inject.at(map[bool]FaultPoint{true: FaultBeforeBackupRename, false: FaultBeforeCurrentRename}[backup]) != nil {
		return ErrAtomicWriteFailed
	}
	if err := os.Rename(tempName, path); err != nil {
		return ErrAtomicWriteFailed
	}
	if inject.at(map[bool]FaultPoint{true: FaultAfterBackupRename, false: FaultAfterCurrentRename}[backup]) != nil {
		return ErrAtomicWriteFailed
	}
	if inject.at(map[bool]FaultPoint{true: FaultBeforeBackupDirSync, false: FaultBeforeCurrentDirSync}[backup]) != nil {
		return ErrAtomicWriteFailed
	}
	directory, err := os.Open(dir)
	if err != nil {
		return ErrAtomicWriteFailed
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil || closeErr != nil {
		return ErrAtomicWriteFailed
	}
	if inject.at(map[bool]FaultPoint{true: FaultAfterBackupDirSync, false: FaultAfterCurrentDirSync}[backup]) != nil {
		return ErrAtomicWriteFailed
	}
	return nil
}

// CanonicalJSONBytes produces the stable, newline-terminated encoding used by
// the control repositories.
func CanonicalJSONBytes(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
