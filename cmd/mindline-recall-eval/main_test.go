package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRejectsUnknownOrUnboundedInvocation(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"seal", "socket", "out"}, bytes.NewBufferString(`{"unknown":true}`), &output); err == nil {
		t.Fatal("unknown draft fields were accepted")
	}
	if err := run([]string{"compare"}, bytes.NewReader(nil), &output); err == nil {
		t.Fatal("invalid invocation was accepted")
	}
}

func TestReadStrictEnforcesOwnerOnlyBoundWhileReading(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	exact := filepath.Join(root, "exact.json")
	data := append([]byte(`{}`), bytes.Repeat([]byte(" "), maximumOwnerEvalBytes-2)...)
	if err := os.WriteFile(exact, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := readStrict(exact, &value); err != nil {
		t.Fatalf("exact limit rejected: %v", err)
	}
	oversized := filepath.Join(root, "oversized.json")
	file, err := os.OpenFile(oversized, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maximumOwnerEvalBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := readStrict(oversized, &value); err == nil {
		t.Fatal("oversized owner input was accepted")
	}
	symlink := filepath.Join(root, "symlink.json")
	if err := os.Symlink(exact, symlink); err != nil {
		t.Fatal(err)
	}
	if err := readStrict(symlink, &value); err == nil {
		t.Fatal("symlink owner input was accepted")
	}
}
