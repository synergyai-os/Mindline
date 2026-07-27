package assurance

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanHEADExportExcludesIgnoredWorkspaceFilesAndCleansUp(t *testing.T) {
	repository := t.TempDir()
	writeTestFile(t, filepath.Join(repository, "go.mod"), "module "+expectedModulePath+"\n\ngo 1.25\n")
	writeTestFile(t, filepath.Join(repository, ".gitignore"), ".env.local\n")
	writeTestFile(t, filepath.Join(repository, "tracked.txt"), "tracked source\n")
	runTestGit(t, repository, "init")
	runTestGit(t, repository, "add", "go.mod", ".gitignore", "tracked.txt")
	runTestGit(t, repository, "-c", "user.name=Mindline Test", "-c", "user.email=mindline@example.invalid", "commit", "-m", "fixture")
	revision := strings.TrimSpace(runTestGit(t, repository, "rev-parse", "HEAD"))
	writeTestFile(t, filepath.Join(repository, ".env.local"), "IGNORED_PRIVATE_SENTINEL=present\n")

	surface, err := createCleanHEADExport(repository, revision)
	if err != nil {
		t.Fatal(err)
	}
	root := surface.root
	info, err := os.Stat(root)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("clean export root is not private: info=%v err=%v", info, err)
	}
	tracked, err := os.ReadFile(filepath.Join(root, "tracked.txt"))
	if err != nil || string(tracked) != "tracked source\n" {
		t.Fatalf("clean export did not contain exact tracked source: %q err=%v", tracked, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".env.local")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ignored workspace secret entered clean HEAD export: %v", err)
	}
	if !validSHA256Fingerprint(surface.binding) {
		t.Fatalf("clean export binding is invalid: %q", surface.binding)
	}
	if err := surface.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private clean export was not removed: %v", err)
	}
}

func TestRuntimeSnapshotIncludesIgnoredPrivateSurfaceAndIsBounded(t *testing.T) {
	runtimeRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(runtimeRoot, "queue"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(runtimeRoot, "queue", ".env.local"), "PRIVATE_RUNTIME_SENTINEL=present\n")
	surface, err := createRuntimeSnapshot(runtimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer surface.cleanup()
	value, err := os.ReadFile(filepath.Join(surface.root, "queue", ".env.local"))
	if err != nil || string(value) != "PRIVATE_RUNTIME_SENTINEL=present\n" {
		t.Fatalf("runtime snapshot omitted private ignored content: %q err=%v", value, err)
	}
	if !validSHA256Fingerprint(surface.binding) {
		t.Fatalf("runtime snapshot binding is invalid: %q", surface.binding)
	}
}

func TestRuntimeSnapshotRejectsSymlinkedSurfaceEntries(t *testing.T) {
	runtimeRoot := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside")
	writeTestFile(t, target, "outside\n")
	if err := os.Symlink(target, filepath.Join(runtimeRoot, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := createRuntimeSnapshot(runtimeRoot); err == nil {
		t.Fatal("symlinked runtime surface was accepted")
	}
}

func TestCleanHEADArchivePathRejectsTraversal(t *testing.T) {
	if _, err := containedArchivePath(t.TempDir(), "../escape"); err == nil {
		t.Fatal("archive traversal path was accepted")
	}
}

func writeTestFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runTestGit(t *testing.T, workdir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", workdir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, output)
	}
	return string(output)
}
