//go:build darwin || linux

package privateio

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

var ErrLockBusy = errors.New("private advisory lock is busy")

// AdvisoryLock is held for the lifetime of an owner-only regular descriptor.
type AdvisoryLock struct {
	file *os.File
}

func AcquireAdvisoryLock(root, path string) (*AdvisoryLock, error) {
	if err := ValidateContained(root, path); err != nil {
		return nil, errors.New("private advisory lock unavailable")
	}
	if err := PrepareDir(filepath.Dir(path)); err != nil {
		return nil, errors.New("private advisory lock unavailable")
	}
	if err := ValidateContained(root, path); err != nil {
		return nil, errors.New("private advisory lock unavailable")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, FileMode)
	if err != nil {
		return nil, errors.New("private advisory lock unavailable")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != FileMode {
		file.Close()
		return nil, errors.New("private advisory lock unavailable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		file.Close()
		return nil, errors.New("private advisory lock unavailable")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrLockBusy
		}
		return nil, errors.New("private advisory lock unavailable")
	}
	return &AdvisoryLock{file: file}, nil
}

func (lock *AdvisoryLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	file := lock.file
	lock.file = nil
	unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	closeErr := file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
