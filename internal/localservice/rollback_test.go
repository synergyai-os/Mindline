package localservice

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

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
	if _, err := Install(InstallOptions{
		Config: config, ConfigPath: filepath.Join(config.RuntimeRoot, "config.json"),
		SourceBinary: successor, Start: false,
	}); err != nil {
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
	stateBefore, err := os.ReadFile(stateMarker)
	if err != nil {
		t.Fatal(err)
	}
	originalStop := stopUserService
	originalRestart := restartUserService
	stopUserService = func(string) error { return nil }
	restartUserService = func(string) error { return nil }
	t.Cleanup(func() {
		stopUserService = originalStop
		restartUserService = originalRestart
	})
	receipt, err := Rollback(first.ConfigPath)
	if err != nil || receipt.ServiceState != "rolled_back" {
		t.Fatalf("rollback receipt=%+v err=%v", receipt, err)
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
	if value, err := os.ReadFile(stateMarker); err != nil || !bytes.Equal(value, stateBefore) {
		t.Fatalf("rollback changed durable state: %q err=%v", value, err)
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
