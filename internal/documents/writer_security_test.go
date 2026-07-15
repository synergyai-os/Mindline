package documents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileInRootOverwritesRegularFileAndRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	regular := filepath.Join(root, "artifact.md")
	if err := os.WriteFile(regular, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileInRoot(root, "artifact.md", []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(regular); err != nil || string(data) != "new" {
		t.Fatalf("atomic regular-file replacement failed: data=%q err=%v", data, err)
	}
	info, err := os.Stat(regular)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("atomic replacement widened existing permissions: mode=%v", info.Mode().Perm())
	}
	sealed := filepath.Join(root, "sealed.md")
	if err := os.WriteFile(sealed, []byte("sealed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sealed, 0); err != nil {
		t.Fatal(err)
	}
	if err := writeFileInRoot(root, "sealed.md", []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	sealedInfo, err := os.Stat(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if sealedInfo.Mode().Perm() != 0 {
		t.Fatalf("atomic replacement widened mode-000 permissions: mode=%v", sealedInfo.Mode().Perm())
	}

	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.md")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "linked.md")); err != nil {
		t.Fatal(err)
	}
	if err := writeFileInRoot(root, "linked.md", []byte("escaped"), 0o644); err == nil {
		t.Fatal("final-component symlink was accepted")
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := writeFileInRoot(root, filepath.Join("escape", "created.md"), []byte("escaped"), 0o644); err == nil {
		t.Fatal("parent symlink escaped the root")
	}
	if data, err := os.ReadFile(outsideFile); err != nil || string(data) != "outside" {
		t.Fatalf("outside target changed: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "created.md")); !os.IsNotExist(err) {
		t.Fatalf("root-scoped write created an outside artifact: %v", err)
	}
}

func TestOpenBoundRootRejectsRootSymlinkAndPathSwap(t *testing.T) {
	outside := t.TempDir()
	symlink := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := openBoundRoot(symlink); err == nil {
		t.Fatal("symlink root was accepted")
	}

	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	original := filepath.Join(parent, "original")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := openBoundRootWith(root, func(path string) (*os.Root, error) {
		if err := os.Rename(path, original); err != nil {
			return nil, err
		}
		if err := os.Symlink(outside, path); err != nil {
			return nil, err
		}
		return os.OpenRoot(path)
	})
	if err == nil {
		t.Fatal("root-path swap was accepted")
	}
}
