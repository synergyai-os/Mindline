package localservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

var rollbackReadinessCheck = waitForRollbackReadiness

type rollbackManifest struct {
	SchemaVersion    string `json:"schema_version"`
	BinaryBackupPath string `json:"binary_backup_path"`
	BinarySHA256     string `json:"binary_sha256"`
	SkillBackupPath  string `json:"skill_backup_path"`
	SkillSHA256      string `json:"skill_sha256"`
	BackedUpAt       string `json:"backed_up_at"`
}

type installArtifactSnapshot struct {
	path       string
	mode       fs.FileMode
	maximum    int64
	present    bool
	sha256     string
	backupPath string
}

type installTransaction struct {
	artifacts       map[string]installArtifactSnapshot
	mutationOrder   []string
	backupRoot      string
	launchPath      string
	priorRunning    bool
	serviceTouched  bool
	restoreFailed   bool
	removeOnFailure []string
}

func beginInstallTransaction(config Config, receipt InstallReceipt, priorRunning bool) (*installTransaction, error) {
	if err := privateio.PrepareDir(config.RuntimeRoot); err != nil {
		return nil, errors.New("prepare install transaction")
	}
	backupRoot, err := os.MkdirTemp(config.RuntimeRoot, ".install-transaction-")
	if err != nil {
		return nil, errors.New("prepare install transaction")
	}
	if err := os.Chmod(backupRoot, privateio.DirMode); err != nil {
		_ = os.RemoveAll(backupRoot)
		return nil, errors.New("secure install transaction")
	}
	transaction := &installTransaction{
		artifacts:  make(map[string]installArtifactSnapshot),
		backupRoot: backupRoot, launchPath: receipt.LaunchAgentPath, priorRunning: priorRunning,
	}
	for _, auxiliary := range []string{
		config.SocketPath,
		filepath.Join(config.RuntimeRoot, "service.stdout.log"),
		filepath.Join(config.RuntimeRoot, "service.stderr.log"),
	} {
		if _, err := os.Lstat(auxiliary); os.IsNotExist(err) {
			transaction.removeOnFailure = append(transaction.removeOnFailure, auxiliary)
		} else if err != nil {
			transaction.close()
			return nil, errors.New("inventory prior install")
		}
	}
	specs := []installArtifactSnapshot{
		{path: receipt.ConfigPath, mode: privateio.FileMode, maximum: 64 << 10},
		{path: filepath.Join(config.RuntimeRoot, "install.json"), mode: privateio.FileMode, maximum: 64 << 10},
		{path: receipt.InstalledBinary, mode: 0o700, maximum: maximumBinaryBytes},
		{path: receipt.SkillPath, mode: privateio.FileMode, maximum: maximumSkillBytes},
		{path: receipt.LaunchAgentPath, mode: privateio.FileMode, maximum: 1 << 20},
		{path: rollbackManifestPath(config), mode: privateio.FileMode, maximum: 64 << 10},
		{path: rollbackBinaryPath(config), mode: privateio.FileMode, maximum: maximumBinaryBytes},
		{path: rollbackSkillPath(config), mode: privateio.FileMode, maximum: maximumSkillBytes},
	}
	for index, spec := range specs {
		if strings.TrimSpace(spec.path) == "" {
			continue
		}
		data, present, readErr := readArtifact(spec.path, spec.mode, spec.maximum)
		if readErr != nil {
			transaction.close()
			return nil, errors.New("inventory prior install")
		}
		spec.present = present
		if present {
			spec.sha256 = sha256Hex(data)
			spec.backupPath = filepath.Join(backupRoot, fmt.Sprintf("artifact-%02d", index))
			if err := privateio.WriteFile(spec.backupPath, data, true); err != nil {
				transaction.close()
				return nil, errors.New("back up prior install")
			}
			backedUp, err := privateio.ReadFileBounded(backupRoot, spec.backupPath, spec.maximum)
			if err != nil || sha256Hex(backedUp) != spec.sha256 {
				transaction.close()
				return nil, errors.New("verify prior install backup")
			}
		}
		transaction.artifacts[spec.path] = spec
	}
	return transaction, nil
}

func (transaction *installTransaction) mutate(stage, path string, operation func() error) error {
	if _, exists := transaction.artifacts[path]; !exists {
		return errors.New("install mutation is outside the transaction")
	}
	transaction.mutationOrder = append(transaction.mutationOrder, path)
	if err := operation(); err != nil {
		return err
	}
	if err := installFaultHook(stage); err != nil {
		return err
	}
	return nil
}

func (transaction *installTransaction) restore() error {
	var first error
	if transaction.serviceTouched && transaction.launchPath != "" {
		if err := stopUserService(transaction.launchPath); err != nil {
			first = errors.New("stop candidate local agent service")
		}
	}
	for index := len(transaction.mutationOrder) - 1; index >= 0; index-- {
		snapshot := transaction.artifacts[transaction.mutationOrder[index]]
		if err := restoreInstallArtifact(snapshot); err != nil && first == nil {
			first = err
		}
	}
	for _, path := range transaction.removeOnFailure {
		if err := removeFailedFirstInstallArtifact(path); err != nil && first == nil {
			first = err
		}
	}
	if transaction.serviceTouched && transaction.priorRunning && transaction.launchPath != "" {
		if err := restartUserService(transaction.launchPath); err != nil && first == nil {
			first = errors.New("restart prior local agent service")
		}
	}
	transaction.restoreFailed = first != nil
	return first
}

func removeFailedFirstInstallArtifact(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("remove failed first install artifact")
	}
	if err := os.Remove(path); err != nil {
		return errors.New("remove failed first install artifact")
	}
	return nil
}

func restoreInstallArtifact(snapshot installArtifactSnapshot) error {
	if !snapshot.present {
		info, err := os.Lstat(snapshot.path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("remove failed install artifact")
		}
		if err := os.Remove(snapshot.path); err != nil {
			return errors.New("remove failed install artifact")
		}
		return nil
	}
	if snapshot.mode == 0o700 {
		if err := copyExecutable(snapshot.backupPath, snapshot.path); err != nil {
			return errors.New("restore prior install artifact")
		}
	} else {
		backup, err := privateio.ReadFileBounded(filepath.Dir(snapshot.backupPath), snapshot.backupPath, snapshot.maximum)
		if err != nil || sha256Hex(backup) != snapshot.sha256 {
			return errors.New("read prior install artifact")
		}
		if err := privateio.WriteFile(snapshot.path, backup, false); err != nil {
			return errors.New("restore prior install artifact")
		}
	}
	data, present, err := readArtifact(snapshot.path, snapshot.mode, snapshot.maximum)
	if err != nil || !present || sha256Hex(data) != snapshot.sha256 {
		return errors.New("verify restored install artifact")
	}
	return nil
}
func (transaction *installTransaction) close() {
	if transaction == nil || transaction.restoreFailed {
		return
	}
	_ = os.RemoveAll(transaction.backupRoot)
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

func prepareRollbackBackup(transaction *installTransaction, config Config, binaryPath, skillPath string) error {
	binarySnapshot, binaryKnown := transaction.artifacts[binaryPath]
	skillSnapshot, skillKnown := transaction.artifacts[skillPath]
	if !binaryKnown || !skillKnown {
		return errors.New("read prior local agent installation")
	}
	binaryPresent := binarySnapshot.present
	skillPresent := skillSnapshot.present
	if !binaryPresent && !skillPresent {
		return nil
	}
	if !binaryPresent || !skillPresent {
		return errors.New("prior local agent installation is incomplete")
	}
	binary, err := privateio.ReadFileBounded(
		transaction.backupRoot, binarySnapshot.backupPath, binarySnapshot.maximum,
	)
	if err != nil || sha256Hex(binary) != binarySnapshot.sha256 {
		return errors.New("read prior installed binary")
	}
	skill, err := privateio.ReadFileBounded(
		transaction.backupRoot, skillSnapshot.backupPath, skillSnapshot.maximum,
	)
	if err != nil || sha256Hex(skill) != skillSnapshot.sha256 {
		return errors.New("read prior installed skill")
	}
	if err := privateio.PrepareDir(rollbackRoot(config)); err != nil {
		return errors.New("prepare local agent rollback")
	}
	if err := transaction.mutate("rollback-binary", rollbackBinaryPath(config), func() error {
		return privateio.WriteFile(rollbackBinaryPath(config), binary, false)
	}); err != nil {
		return errors.New("save prior installed binary")
	}
	if err := transaction.mutate("rollback-skill", rollbackSkillPath(config), func() error {
		return privateio.WriteFile(rollbackSkillPath(config), skill, false)
	}); err != nil {
		return errors.New("save prior installed skill")
	}
	manifest := rollbackManifest{
		SchemaVersion:    rollbackManifestSchemaVersion,
		BinaryBackupPath: rollbackBinaryPath(config), BinarySHA256: sha256Hex(binary),
		SkillBackupPath: rollbackSkillPath(config), SkillSHA256: sha256Hex(skill),
		BackedUpAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := transaction.mutate("rollback-manifest", rollbackManifestPath(config), func() error {
		return privateio.WriteJSON(rollbackManifestPath(config), manifest)
	}); err != nil {
		return errors.New("write local agent rollback manifest")
	}
	return nil
}

func Rollback(configPath string) (returnReceipt InstallReceipt, returnErr error) {
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
	priorRunning, err := inspectUserService(receipt.LaunchAgentPath)
	if err != nil {
		return InstallReceipt{}, errors.New("inspect local agent service before rollback")
	}
	transaction, err := beginInstallTransaction(config, receipt, priorRunning)
	if err != nil {
		return InstallReceipt{}, err
	}
	committed := false
	defer func() {
		if !committed && returnErr != nil {
			if restoreErr := transaction.restore(); restoreErr != nil {
				returnErr = fmt.Errorf("%w; restore successor install: %v", returnErr, restoreErr)
			}
		}
		transaction.close()
	}()
	transaction.serviceTouched = true
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
	if err := transaction.mutate("rollback-active-skill", receipt.SkillPath, func() error {
		return privateio.WriteFile(receipt.SkillPath, skill, false)
	}); err != nil {
		return InstallReceipt{}, errors.New("restore prior installed skill")
	}
	if err := transaction.mutate("rollback-active-binary", receipt.InstalledBinary, func() error {
		return copyExecutable(manifest.BinaryBackupPath, receipt.InstalledBinary)
	}); err != nil {
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
	if err := installFaultHook("rollback-service-restart"); err != nil {
		return InstallReceipt{}, err
	}
	if err := rollbackReadinessCheck(config); err != nil {
		return InstallReceipt{}, errors.New("restored local agent service is not ready")
	}
	libraryAfter, stateAfter, err := lifecycleFingerprints(config)
	if err != nil {
		return InstallReceipt{}, err
	}
	if libraryBefore != libraryAfter || stateBefore != stateAfter {
		return InstallReceipt{}, errors.New("rollback changed canonical or durable agent state")
	}
	if err := installFaultHook("rollback-final-fingerprint"); err != nil {
		return InstallReceipt{}, err
	}
	receipt.ServiceState = "rolled_back"
	committed = true
	return receipt, nil
}

func waitForRollbackReadiness(config Config) error {
	return waitForRollbackReadinessWithin(config, 5*time.Second)
}

func waitForRollbackReadinessWithin(config Config, window time.Duration) error {
	client := NewClient(config.SocketPath)
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		requestContext, cancel := context.WithDeadline(context.Background(), deadline)
		status, err := client.Status(requestContext)
		cancel()
		if err == nil && status.ServiceState == "ready" {
			return nil
		}
		if remaining := time.Until(deadline); remaining > 0 {
			time.Sleep(min(50*time.Millisecond, remaining))
		}
	}
	return errors.New("local agent service readiness timed out")
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
