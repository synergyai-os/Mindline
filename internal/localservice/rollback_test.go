package localservice

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

func TestRollbackReadinessBoundsUnresponsiveStatusRequest(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-ready-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	socketPath := filepath.Join(root, "hung.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	release := make(chan struct{})
	defer close(release)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		<-release
	}()
	started := time.Now()
	if err := waitForRollbackReadinessWithin(Config{SocketPath: socketPath}, 100*time.Millisecond); err == nil {
		t.Fatal("unresponsive rollback status was accepted")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("rollback readiness exceeded its request deadline: %v", elapsed)
	}
}

func TestUpgradeBacksUpAndRollbackRestoresOnlyBinaryAndSkill(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd proof is Darwin-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := os.MkdirTemp("/tmp", "mindline-rollback-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	first, err := Install(InstallOptions{
		Config: config, ConfigPath: filepath.Join(config.RuntimeRoot, "config.json"),
		SourceBinary: executable, Start: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	priorBinary, err := os.ReadFile(first.InstalledBinary)
	if err != nil {
		t.Fatal(err)
	}
	priorSkill := []byte("legacy-compatible-skill\n")
	if err := privateio.WriteFile(first.SkillPath, priorSkill, false); err != nil {
		t.Fatal(err)
	}
	successor := filepath.Join(root, "successor")
	if err := os.WriteFile(successor, []byte("successor-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	successorReceipt, err := Install(InstallOptions{
		Config: config, ConfigPath: filepath.Join(config.RuntimeRoot, "config.json"),
		SourceBinary: successor, Start: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceMarker := filepath.Join(config.MemoryRoot, "evidence-marker")
	if err := os.WriteFile(evidenceMarker, []byte("canonical"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	state, err := agentstate.Open(config.StatePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	stateMarker := config.StatePath + ".recovery.json"
	originalStop := stopUserService
	originalRestart := restartUserService
	originalInspect := inspectUserService
	originalReadiness := rollbackReadinessCheck
	readinessChecks := 0
	var stateAfterStop []byte
	stopUserService = func(string) error {
		store, err := agentstate.Open(config.StatePath, nil)
		if err != nil {
			return err
		}
		if _, err := store.PutLens(context.Background(), agentstate.Lens{ID: "during-stop", Name: "During stop", Query: "stable rollback baseline"}); err != nil {
			_ = store.Close()
			return err
		}
		if err := store.Close(); err != nil {
			return err
		}
		stateAfterStop, err = os.ReadFile(stateMarker)
		return err
	}
	restartUserService = func(string) error { return nil }
	inspectUserService = func(string) (bool, error) { return true, nil }
	rollbackReadinessCheck = func(Config) error { readinessChecks++; return nil }
	t.Cleanup(func() {
		stopUserService = originalStop
		restartUserService = originalRestart
		inspectUserService = originalInspect
		rollbackReadinessCheck = originalReadiness
	})
	receipt, err := Rollback(first.ConfigPath)
	if err != nil || receipt.ServiceState != "rolled_back" {
		t.Fatalf("rollback receipt=%+v err=%v", receipt, err)
	}
	if readinessChecks != 1 {
		t.Fatalf("rollback committed without one readiness check: %d", readinessChecks)
	}
	persisted, err := readValidatedInstallReceipt(config, first.ConfigPath)
	if err != nil || persisted != receipt || persisted.ServiceState != "rolled_back" {
		t.Fatalf("rollback receipt was not durably committed: persisted=%+v returned=%+v err=%v", persisted, receipt, err)
	}
	if persisted == successorReceipt {
		t.Fatal("rollback left the successor install receipt unchanged")
	}
	if restored, err := os.ReadFile(first.InstalledBinary); err != nil ||
		!bytes.Equal(restored, priorBinary) {
		t.Fatalf("binary was not restored: err=%v", err)
	}
	if restored, err := os.ReadFile(first.SkillPath); err != nil ||
		!bytes.Equal(restored, priorSkill) {
		t.Fatalf("skill was not restored: %q err=%v", restored, err)
	}
	if value, err := os.ReadFile(evidenceMarker); err != nil || string(value) != "canonical" {
		t.Fatalf("rollback changed canonical evidence: %q err=%v", value, err)
	}
	if value, err := os.ReadFile(stateMarker); err != nil || !bytes.Equal(value, stateAfterStop) {
		t.Fatalf("rollback changed durable state: %q err=%v", value, err)
	}
}

func TestSkillFreeUpgradeAndRollbackPreserveDeliberatelyAbsentHelperSkill(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd proof is Darwin-specific")
	}
	t.Setenv("HOME", t.TempDir())
	root, err := os.MkdirTemp("/tmp", "mindline-skill-free-rollback-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	first, err := Install(InstallOptions{
		Config: config, ConfigPath: filepath.Join(config.RuntimeRoot, "config.json"),
		SourceBinary: executable, Start: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Create a complete upgrade first so the skill-free upgrade must also clear
	// an older rollback skill rather than accidentally reusing it.
	if _, err := Install(InstallOptions{
		Config: config, ConfigPath: first.ConfigPath, SourceBinary: executable, Start: false,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(first.SkillPath); err != nil {
		t.Fatal(err)
	}
	priorBinary, err := os.ReadFile(first.InstalledBinary)
	if err != nil {
		t.Fatal(err)
	}
	successor := filepath.Join(root, "successor")
	if err := os.WriteFile(successor, []byte("skill-free-successor"), 0o700); err != nil {
		t.Fatal(err)
	}
	successorReceipt, err := Install(InstallOptions{
		Config: config, ConfigPath: first.ConfigPath, SourceBinary: successor, Start: false,
	})
	if err != nil || successorReceipt.SkillState != "absent_preserved" {
		t.Fatalf("receipt=%+v err=%v", successorReceipt, err)
	}
	if _, err := os.Lstat(first.SkillPath); !os.IsNotExist(err) {
		t.Fatalf("skill-free upgrade recreated helper skill: %v", err)
	}
	manifest, _, skill, err := readRollbackArtifacts(config)
	if err != nil || rollbackManifestHasSkill(manifest) || len(skill) != 0 {
		t.Fatalf("manifest=%+v skill=%q err=%v", manifest, skill, err)
	}
	if _, err := os.Lstat(rollbackSkillPath(config)); !os.IsNotExist(err) {
		t.Fatalf("stale rollback skill survived: %v", err)
	}

	originalStop, originalRestart := stopUserService, restartUserService
	originalInspect, originalReadiness := inspectUserService, rollbackReadinessCheck
	stopUserService = func(string) error { return nil }
	restartUserService = func(string) error { return nil }
	inspectUserService = func(string) (bool, error) { return false, nil }
	rollbackReadinessCheck = func(Config) error { return nil }
	t.Cleanup(func() {
		stopUserService, restartUserService = originalStop, originalRestart
		inspectUserService, rollbackReadinessCheck = originalInspect, originalReadiness
	})
	receipt, err := Rollback(first.ConfigPath)
	if err != nil || receipt.ServiceState != "rolled_back" || receipt.SkillState != "absent_preserved" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
	restored, err := os.ReadFile(first.InstalledBinary)
	if err != nil || !bytes.Equal(restored, priorBinary) {
		t.Fatalf("binary was not restored: err=%v", err)
	}
	if _, err := os.Lstat(first.SkillPath); !os.IsNotExist(err) {
		t.Fatalf("rollback recreated deliberately absent helper skill: %v", err)
	}
}

func TestRollbackFailureAfterStopRestoresSuccessorInstallAndService(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd proof is Darwin-specific")
	}
	t.Setenv("HOME", t.TempDir())
	root, err := os.MkdirTemp("/tmp", "mindline-rollback-compensation-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	first, err := Install(InstallOptions{Config: config, ConfigPath: filepath.Join(config.RuntimeRoot, "config.json"), SourceBinary: executable, Start: false})
	if err != nil {
		t.Fatal(err)
	}
	if err := privateio.WriteFile(first.SkillPath, []byte("prior-skill\n"), false); err != nil {
		t.Fatal(err)
	}
	successorSource := filepath.Join(root, "successor")
	if err := os.WriteFile(successorSource, []byte("successor-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(InstallOptions{Config: config, ConfigPath: first.ConfigPath, SourceBinary: successorSource, Start: false}); err != nil {
		t.Fatal(err)
	}
	successorBinary, err := os.ReadFile(first.InstalledBinary)
	if err != nil {
		t.Fatal(err)
	}
	successorSkill, err := os.ReadFile(first.SkillPath)
	if err != nil {
		t.Fatal(err)
	}
	state, err := agentstate.Open(config.StatePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	originalStop, originalRestart := stopUserService, restartUserService
	originalInspect, originalFault := inspectUserService, installFaultHook
	originalReadiness := rollbackReadinessCheck
	stops, restarts := 0, 0
	stopUserService = func(string) error { stops++; return nil }
	restartUserService = func(string) error { restarts++; return nil }
	inspectUserService = func(string) (bool, error) { return true, nil }
	rollbackReadinessCheck = func(Config) error { return nil }
	installFaultHook = func(stage string) error {
		if stage == "rollback-active-binary" {
			return errors.New("injected rollback failure")
		}
		return nil
	}
	t.Cleanup(func() {
		stopUserService, restartUserService = originalStop, originalRestart
		inspectUserService, installFaultHook = originalInspect, originalFault
		rollbackReadinessCheck = originalReadiness
	})
	if _, err := Rollback(first.ConfigPath); err == nil {
		t.Fatal("injected rollback failure was accepted")
	}
	if restored, err := os.ReadFile(first.InstalledBinary); err != nil || !bytes.Equal(restored, successorBinary) {
		t.Fatalf("successor binary was not restored: err=%v", err)
	}
	if restored, err := os.ReadFile(first.SkillPath); err != nil || !bytes.Equal(restored, successorSkill) {
		t.Fatalf("successor skill was not restored: err=%v", err)
	}
	if stops < 2 || restarts != 1 {
		t.Fatalf("rollback compensation did not restore the running service: stops=%d restarts=%d", stops, restarts)
	}

	installFaultHook = func(string) error { return nil }
	stops, restarts = 0, 0
	stopUserService = func(string) error {
		stops++
		if stops == 1 {
			return errors.New("stop outcome unknown after termination")
		}
		return nil
	}
	if _, err := Rollback(first.ConfigPath); err == nil {
		t.Fatal("uncertain initial stop failure was accepted")
	}
	if restored, err := os.ReadFile(first.InstalledBinary); err != nil || !bytes.Equal(restored, successorBinary) {
		t.Fatalf("uncertain stop lost successor binary: err=%v", err)
	}
	if restored, err := os.ReadFile(first.SkillPath); err != nil || !bytes.Equal(restored, successorSkill) {
		t.Fatalf("uncertain stop lost successor skill: err=%v", err)
	}
	if stops != 2 || restarts != 1 {
		t.Fatalf("uncertain stop was not compensated: stops=%d restarts=%d", stops, restarts)
	}

	stops, restarts = 0, 0
	stopUserService = func(string) error { stops++; return nil }
	rollbackReadinessCheck = func(Config) error { return errors.New("restored service exited") }
	if _, err := Rollback(first.ConfigPath); err == nil {
		t.Fatal("rollback without service readiness was accepted")
	}
	if restored, err := os.ReadFile(first.InstalledBinary); err != nil || !bytes.Equal(restored, successorBinary) {
		t.Fatalf("readiness failure lost successor binary: err=%v", err)
	}
	if restored, err := os.ReadFile(first.SkillPath); err != nil || !bytes.Equal(restored, successorSkill) {
		t.Fatalf("readiness failure lost successor skill: err=%v", err)
	}
	if stops != 2 || restarts != 2 {
		t.Fatalf("readiness failure was not compensated: stops=%d restarts=%d", stops, restarts)
	}
}

func TestRollbackRejectsTamperedBackupBeforeStoppingService(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd proof is Darwin-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := os.MkdirTemp("/tmp", "mindline-rollback-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	first, err := Install(InstallOptions{
		Config: config, ConfigPath: filepath.Join(config.RuntimeRoot, "config.json"),
		SourceBinary: executable, Start: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Install(InstallOptions{
		Config: config, ConfigPath: filepath.Join(config.RuntimeRoot, "config.json"),
		SourceBinary: executable, Start: false,
	}); err != nil {
		t.Fatal(err)
	}
	if err := privateio.WriteFile(rollbackBinaryPath(config), []byte("tampered"), false); err != nil {
		t.Fatal(err)
	}
	stopped := false
	originalStop := stopUserService
	stopUserService = func(string) error {
		stopped = true
		return nil
	}
	t.Cleanup(func() { stopUserService = originalStop })
	if _, err := Rollback(first.ConfigPath); err == nil {
		t.Fatal("tampered rollback backup was accepted")
	}
	if stopped {
		t.Fatal("service stopped before rollback backup verification")
	}
}
