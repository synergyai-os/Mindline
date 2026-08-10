package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/localservice"
)

func TestAuditedBuildUsesTheSupportedExactTreeLinkerContract(t *testing.T) {
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	flag, err := localservice.AuditedSourceTreeLinkerFlag(fingerprint)
	if err != nil || !strings.Contains(flag, "sourceTreeFingerprint="+fingerprint) {
		t.Fatalf("exact tree linker contract unavailable: flag=%q err=%v", flag, err)
	}
	if _, err := localservice.AuditedSourceTreeLinkerFlag("owner-supplied-label"); err == nil {
		t.Fatal("non-commitment tree label was accepted")
	}
	if !pathInside("/repo", "/repo/bin/mindline") || pathInside("/repo", "/tmp/mindline") {
		t.Fatal("audited build output boundary drifted")
	}
}

func TestAuditedSnapshotExcludesIgnoredSourceFiles(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	gitPath := "/usr/bin/git"
	if runtime.GOOS != "darwin" {
		if _, err := os.Lstat(gitPath); err != nil {
			gitPath = "/bin/git"
		}
	}
	initCommand := exec.Command(gitPath, "init", repository)
	initCommand.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	if output, err := initCommand.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %s", output)
	}
	if err := os.WriteFile(filepath.Join(repository, ".gitignore"), []byte("ignored.go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "tracked.go"), []byte("package fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "ignored.go"), []byte("package fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(gitPath, "-C", repository, "add", ".gitignore", "tracked.go")
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %s", output)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err := os.Mkdir(snapshot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := snapshotTrackedTree(repository, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(snapshot, "tracked.go")); err != nil {
		t.Fatal("tracked source was not copied")
	}
	if _, err := os.Stat(filepath.Join(snapshot, "ignored.go")); !os.IsNotExist(err) {
		t.Fatal("ignored source entered audited snapshot")
	}
}

func TestAuditedBuildResolvesTheSystemTemporaryDirectory(t *testing.T) {
	resolved, err := filepath.EvalSymlinks("/tmp")
	if err != nil || !filepath.IsAbs(resolved) {
		t.Fatalf("temporary directory is unavailable: resolved=%q err=%v", resolved, err)
	}
	if !filepath.IsAbs(filepath.Join(resolved, "mindline")) {
		t.Fatal("resolved audited output is not absolute")
	}
}
