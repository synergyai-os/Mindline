package privateio

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// DefaultControlPlaneRoot is the stable local control root. It contains no
// process id, commit, random suffix, or run identity.
func DefaultControlPlaneRoot() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("local configuration directory unavailable")
	}
	return ControlPlaneRootFromConfigDir(configDir)
}

func ControlPlaneRootFromConfigDir(configDir string) (string, error) {
	if strings.TrimSpace(configDir) == "" || !filepath.IsAbs(configDir) {
		return "", errors.New("local configuration directory unavailable")
	}
	return filepath.Join(filepath.Clean(configDir), "Mindline", "control-plane"), nil
}
