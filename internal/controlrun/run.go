package controlrun

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

// ReserveRun creates exactly one owner-only immutable run identity. It never
// replaces, discovers, or selects any other run.
func ReserveRun(root, runID string) (string, error) {
	if err := ValidateRunID(runID); err != nil || !filepath.IsAbs(root) {
		return "", ErrInvalid
	}
	if err := privateio.PrepareDir(root); err != nil {
		return "", errors.New("run storage unavailable")
	}
	runsDir := filepath.Join(root, "runs")
	if err := privateio.PrepareDir(runsDir); err != nil {
		return "", errors.New("run storage unavailable")
	}
	runPath := filepath.Join(runsDir, runID)
	if err := privateio.ValidateContained(root, runPath); err != nil {
		return "", errors.New("run storage unavailable")
	}
	if err := os.Mkdir(runPath, privateio.DirMode); err != nil {
		if os.IsExist(err) {
			return "", os.ErrExist
		}
		return "", errors.New("run storage unavailable")
	}
	if err := privateio.ValidateContained(root, runPath); err != nil {
		return "", errors.New("run storage unavailable")
	}
	return runPath, nil
}

func validateReservedRun(root, runID string) error {
	if runID == "" {
		return nil
	}
	if err := ValidateRunID(runID); err != nil {
		return ErrInvalid
	}
	path := filepath.Join(root, "runs", runID)
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != privateio.DirMode {
		return ErrRunNotFound
	}
	if err := privateio.ValidateContained(root, path); err != nil {
		return ErrRunNotFound
	}
	return nil
}
