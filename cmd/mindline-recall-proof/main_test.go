package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestProofInputRejectsOversizedStdinAndFiles(t *testing.T) {
	if _, err := readBounded(bytes.NewReader(bytes.Repeat([]byte("x"), (64<<10)+1)), 64<<10); err == nil {
		t.Fatal("oversized proof stdin was accepted")
	}
	root := t.TempDir()
	path := filepath.Join(root, "oversized.json")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate((64 << 10) + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedRegular(path, 64<<10); err == nil {
		t.Fatal("oversized proof file was accepted")
	}
	symlink := filepath.Join(root, "symlink.json")
	if err := os.Symlink(path, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedRegular(symlink, 64<<10); err == nil {
		t.Fatal("symlink proof file was accepted")
	}
}
