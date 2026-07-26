package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOSFileSystemReadFileBoundedRejectsOversizeAndSymlinkInputs(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "input.json")
	if err := os.WriteFile(target, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := NewOSFileSystem()
	if _, err := fs.ReadFileBounded(target, 4); err == nil {
		t.Fatal("bounded file read accepted an oversized input")
	}
	link := filepath.Join(root, "input-link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFileBounded(link, 32); err == nil {
		t.Fatal("bounded file read followed a caller-controlled symlink")
	}
}
