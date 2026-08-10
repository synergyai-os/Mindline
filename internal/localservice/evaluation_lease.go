package localservice

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

// EvaluationLease prevents install, restart, rollback, and uninstall from
// replacing the runtime while a bound retrieval evaluation is in progress.
type EvaluationLease struct {
	lock *privateio.AdvisoryLock
}

func AcquireEvaluationLease(socketPath string) (*EvaluationLease, error) {
	socketPath = filepath.Clean(socketPath)
	if !filepath.IsAbs(socketPath) || filepath.Base(socketPath) != "mindline.sock" {
		return nil, errors.New("evaluation lifecycle lease unavailable")
	}
	runtimeRoot := filepath.Dir(socketPath)
	rootInfo, err := os.Lstat(runtimeRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode().Perm() != privateio.DirMode {
		return nil, errors.New("evaluation lifecycle lease unavailable")
	}
	lockPath := filepath.Join(runtimeRoot, "lifecycle.lock")
	lockInfo, err := os.Lstat(lockPath)
	if err != nil || !lockInfo.Mode().IsRegular() || lockInfo.Mode()&os.ModeSymlink != 0 || lockInfo.Mode().Perm() != privateio.FileMode {
		return nil, errors.New("evaluation lifecycle lease unavailable")
	}
	lock, err := privateio.AcquireAdvisoryLock(runtimeRoot, lockPath)
	if errors.Is(err, privateio.ErrLockBusy) {
		return nil, errors.New("evaluation lifecycle operation busy")
	}
	if err != nil {
		return nil, errors.New("evaluation lifecycle lease unavailable")
	}
	return &EvaluationLease{lock: lock}, nil
}

func (lease *EvaluationLease) Close() error {
	if lease == nil || lease.lock == nil {
		return nil
	}
	lock := lease.lock
	lease.lock = nil
	return lock.Close()
}
