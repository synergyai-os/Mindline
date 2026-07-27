package localservice

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	rollbackManifestSchemaVersion = "mindline-local-agent-rollback/v0.1"
	maximumSkillBytes             = 1 << 20
)

type rollbackManifest struct {
	SchemaVersion    string `json:"schema_version"`
	BinaryBackupPath string `json:"binary_backup_path"`
	BinarySHA256     string `json:"binary_sha256"`
	SkillBackupPath  string `json:"skill_backup_path"`
	SkillSHA256      string `json:"skill_sha256"`
	BackedUpAt       string `json:"backed_up_at"`
}

func rollbackRoot(config Config) string {
	return filepath.Join(config.RuntimeRoot, "rollback")
}

func rollbackManifestPath(config Config) string {
	return filepath.Join(rollbackRoot(config), "manifest.json")
}

func rollbackBinaryPath(config Config) string {
	return filepath.Join(rollbackRoot(config), "mindline.previous")
}

func rollbackSkillPath(config Config) string {
	return filepath.Join(rollbackRoot(config), "SKILL.previous.md")
}

func prepareRollbackBackup(config Config, binaryPath, skillPath string) error {
	binary, binaryPresent, err := readArtifact(binaryPath, 0o700, maximumBinaryBytes)
	if err != nil {
		return errors.New("read prior installed binary")
	}
	skill, skillPresent, err := readArtifact(skillPath, privateio.FileMode, maximumSkillBytes)
	if err != nil {
		return errors.New("read prior installed skill")
	}
	if !binaryPresent && !skillPresent {
		return nil
	}
	if !binaryPresent || !skillPresent {
		return errors.New("prior local agent installation is incomplete")
	}
	if err := privateio.PrepareDir(rollbackRoot(config)); err != nil {
		return errors.New("prepare local agent rollback")
	}
	if err := privateio.WriteFile(rollbackBinaryPath(config), binary, false); err != nil {
		return errors.New("save prior installed binary")
	}
	if err := privateio.WriteFile(rollbackSkillPath(config), skill, false); err != nil {
		return errors.New("save prior installed skill")
	}
	manifest := rollbackManifest{
		SchemaVersion:    rollbackManifestSchemaVersion,
		BinaryBackupPath: rollbackBinaryPath(config), BinarySHA256: sha256Hex(binary),
		SkillBackupPath: rollbackSkillPath(config), SkillSHA256: sha256Hex(skill),
		BackedUpAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := privateio.WriteJSON(rollbackManifestPath(config), manifest); err != nil {
		return errors.New("write local agent rollback manifest")
	}
	return nil
}

func Rollback(configPath string) (InstallReceipt, error) {
	if strings.TrimSpace(configPath) == "" {
		var err error
		configPath, err = DefaultConfigPath()
		if err != nil {
			return InstallReceipt{}, err
		}
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		return InstallReceipt{}, err
	}
	receipt, err := readValidatedInstallReceipt(config, configPath)
	if err != nil {
		return InstallReceipt{}, err
	}
	manifest, _, skill, err := readRollbackArtifacts(config)
	if err != nil {
		return InstallReceipt{}, err
	}
	if runtime.GOOS != "darwin" || receipt.LaunchAgentPath == "" {
		return InstallReceipt{}, errors.New("automatic service rollback is not supported")
	}
	if err := stopUserService(receipt.LaunchAgentPath); err != nil {
		return InstallReceipt{}, err
	}
	libraryBefore, stateBefore, err := lifecycleFingerprints(config)
	if err != nil {
		return InstallReceipt{}, err
	}
	// Restore the legacy skill first. A successor binary can still run that
	// skill if the process stops here, while a legacy binary cannot understand
	// every successor skill command.
	if err := privateio.WriteFile(receipt.SkillPath, skill, false); err != nil {
		return InstallReceipt{}, errors.New("restore prior installed skill")
	}
	if err := copyExecutable(manifest.BinaryBackupPath, receipt.InstalledBinary); err != nil {
		return InstallReceipt{}, errors.New("restore prior installed binary")
	}
	restoredBinary, present, err := readArtifact(
		receipt.InstalledBinary, 0o700, maximumBinaryBytes,
	)
	if err != nil || !present || sha256Hex(restoredBinary) != manifest.BinarySHA256 {
		return InstallReceipt{}, errors.New("verify restored installed binary")
	}
	restoredSkill, present, err := readArtifact(
		receipt.SkillPath, privateio.FileMode, maximumSkillBytes,
	)
	if err != nil || !present || sha256Hex(restoredSkill) != manifest.SkillSHA256 {
		return InstallReceipt{}, errors.New("verify restored installed skill")
	}
	if err := restartUserService(receipt.LaunchAgentPath); err != nil {
		return InstallReceipt{}, err
	}
	libraryAfter, stateAfter, err := lifecycleFingerprints(config)
	if err != nil {
		return InstallReceipt{}, err
	}
	if libraryBefore != libraryAfter || stateBefore != stateAfter {
		return InstallReceipt{}, errors.New("rollback changed canonical or durable agent state")
	}
	receipt.ServiceState = "rolled_back"
	return receipt, nil
}

func readValidatedInstallReceipt(config Config, configPath string) (InstallReceipt, error) {
	var receipt InstallReceipt
	if err := privateio.ReadJSONStrictBounded(
		config.RuntimeRoot, filepath.Join(config.RuntimeRoot, "install.json"), 64<<10, &receipt,
	); err != nil || receipt.SchemaVersion != InstallReceiptSchemaVersion {
		return InstallReceipt{}, errors.New("read install receipt")
	}
	if err := validateInstallReceipt(config, configPath, receipt); err != nil {
		return InstallReceipt{}, err
	}
	return receipt, nil
}

func readRollbackArtifacts(config Config) (rollbackManifest, []byte, []byte, error) {
	var manifest rollbackManifest
	if err := privateio.ReadJSONStrictBounded(
		rollbackRoot(config), rollbackManifestPath(config), 64<<10, &manifest,
	); err != nil || validateRollbackManifest(config, manifest) != nil {
		return rollbackManifest{}, nil, nil, errors.New("read local agent rollback manifest")
	}
	binary, err := privateio.ReadFileBounded(
		rollbackRoot(config), manifest.BinaryBackupPath, maximumBinaryBytes,
	)
	if err != nil || sha256Hex(binary) != manifest.BinarySHA256 {
		return rollbackManifest{}, nil, nil, errors.New("verify prior installed binary")
	}
	skill, err := privateio.ReadFileBounded(
		rollbackRoot(config), manifest.SkillBackupPath, maximumSkillBytes,
	)
	if err != nil || sha256Hex(skill) != manifest.SkillSHA256 {
		return rollbackManifest{}, nil, nil, errors.New("verify prior installed skill")
	}
	return manifest, binary, skill, nil
}

func validateRollbackManifest(config Config, manifest rollbackManifest) error {
	if manifest.SchemaVersion != rollbackManifestSchemaVersion ||
		manifest.BinaryBackupPath != rollbackBinaryPath(config) ||
		manifest.SkillBackupPath != rollbackSkillPath(config) ||
		len(manifest.BinarySHA256) != 64 || len(manifest.SkillSHA256) != 64 ||
		strings.TrimSpace(manifest.BackedUpAt) == "" {
		return errors.New("invalid local agent rollback manifest")
	}
	_, binaryErr := hex.DecodeString(manifest.BinarySHA256)
	_, skillErr := hex.DecodeString(manifest.SkillSHA256)
	if binaryErr != nil || skillErr != nil {
		return errors.New("invalid local agent rollback manifest")
	}
	return privateio.ValidateContained(
		config.RuntimeRoot, manifest.BinaryBackupPath, manifest.SkillBackupPath,
		rollbackManifestPath(config),
	)
}

func lifecycleFingerprints(config Config) (string, string, error) {
	repository, err := personalmemory.NewFileRepository(config.MemoryRoot, nil)
	if err != nil {
		return "", "", errors.New("read canonical library fingerprint")
	}
	status, err := repository.Status()
	if err != nil {
		return "", "", errors.New("read canonical library fingerprint")
	}
	durableFingerprint, err := agentstate.DurableFingerprint(config.StatePath)
	if err != nil {
		return "", "", errors.New("read durable agent state fingerprint")
	}
	return status.Fingerprint, durableFingerprint, nil
}

func readArtifact(path string, mode fs.FileMode, maximum int64) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != mode {
		return nil, false, errors.New("unsafe installed artifact")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	if info.Size() < 1 || info.Size() > maximum {
		return nil, false, errors.New("installed artifact exceeds limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, false, errors.New("read installed artifact")
	}
	return data, true, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
