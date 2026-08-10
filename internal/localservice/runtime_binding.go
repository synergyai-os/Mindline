package localservice

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

var runtimeFingerprintPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

const sourceTreeLinkerSymbol = "github.com/synergyai-os/Mindline/internal/localservice.sourceTreeFingerprint"

// sourceTreeFingerprint is set only by the audited candidate/baseline build.
// Ordinary builds remain usable, but cannot claim an evaluation binding.
var sourceTreeFingerprint string

// AuditedSourceTreeLinkerFlag is the single supported linker contract for a
// service build that may participate in retrieval evaluation.
func AuditedSourceTreeLinkerFlag(treeFingerprint string) (string, error) {
	treeFingerprint = strings.TrimSpace(treeFingerprint)
	if !runtimeFingerprintPattern.MatchString(treeFingerprint) {
		return "", errors.New("audited source tree fingerprint is invalid")
	}
	return "-X=" + sourceTreeLinkerSymbol + "=" + treeFingerprint, nil
}

type RuntimeBinding struct {
	State                    string `json:"state"`
	BuildFingerprint         string `json:"build_fingerprint,omitempty"`
	TreeFingerprint          string `json:"tree_fingerprint,omitempty"`
	ConfigurationFingerprint string `json:"configuration_fingerprint,omitempty"`
}

const BuildBindingSchemaVersion = "mindline-agent-build-binding/v0.1"

// BuildBinding is the self-reported identity of one exact Mindline binary.
// It deliberately excludes runtime configuration so an audited builder can
// verify the binary before it is installed or allowed to read user state.
type BuildBinding struct {
	SchemaVersion    string `json:"schema_version"`
	State            string `json:"state"`
	BuildFingerprint string `json:"build_fingerprint,omitempty"`
	TreeFingerprint  string `json:"tree_fingerprint,omitempty"`
}

// BuildBindingFor returns only structural fingerprints derived from the
// running binary and the source-tree commitment embedded by the supported
// audited build path.
func BuildBindingFor(executable string) BuildBinding {
	binding := BuildBinding{SchemaVersion: BuildBindingSchemaVersion, State: "unavailable"}
	executable = filepath.Clean(strings.TrimSpace(executable))
	if !filepath.IsAbs(executable) {
		return binding
	}
	buildFingerprint, err := regularFileFingerprint(executable, maximumBinaryBytes)
	if err != nil {
		return binding
	}
	binding.BuildFingerprint = buildFingerprint
	binding.TreeFingerprint = strings.TrimSpace(sourceTreeFingerprint)
	if runtimeFingerprintPattern.MatchString(binding.BuildFingerprint) &&
		runtimeFingerprintPattern.MatchString(binding.TreeFingerprint) {
		binding.State = "ready"
	}
	return binding
}

func runtimeBindingFor(executable, configPath string, config Config) RuntimeBinding {
	binding := RuntimeBinding{State: "unavailable"}
	executable = filepath.Clean(strings.TrimSpace(executable))
	configPath = filepath.Clean(strings.TrimSpace(configPath))
	if !filepath.IsAbs(executable) || !filepath.IsAbs(configPath) ||
		configPath != filepath.Join(config.RuntimeRoot, "config.json") {
		return binding
	}
	buildFingerprint, err := regularFileFingerprint(executable, maximumBinaryBytes)
	if err != nil {
		return binding
	}
	configBytes, err := privateio.ReadFileBounded(config.RuntimeRoot, configPath, 64<<10)
	if err != nil {
		return binding
	}
	configurationDigest := sha256.Sum256(configBytes)
	binding.BuildFingerprint = buildFingerprint
	binding.ConfigurationFingerprint = "sha256:" + hex.EncodeToString(configurationDigest[:])
	binding.TreeFingerprint = strings.TrimSpace(sourceTreeFingerprint)
	if runtimeFingerprintPattern.MatchString(binding.BuildFingerprint) &&
		runtimeFingerprintPattern.MatchString(binding.TreeFingerprint) &&
		runtimeFingerprintPattern.MatchString(binding.ConfigurationFingerprint) {
		binding.State = "ready"
	}
	return binding
}

func regularFileFingerprint(path string, maximum int64) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() < 1 || info.Size() > maximum {
		return "", errors.New("runtime executable unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("runtime executable unavailable")
	}
	defer file.Close()
	digest := sha256.New()
	written, err := io.Copy(digest, io.LimitReader(file, maximum+1))
	if err != nil || written != info.Size() || written > maximum {
		return "", errors.New("runtime executable unavailable")
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}
