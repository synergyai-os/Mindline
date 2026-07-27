package privateio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateRuntimeRootAndValidateContained(t *testing.T) {
	root, err := CreateRuntimeRoot(t.TempDir(), "mindline-private-")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input.json")
	if err := os.WriteFile(input, []byte("{}\n"), FileMode); err != nil {
		t.Fatal(err)
	}
	if err := ValidateContained(root, input, filepath.Join(root, "future", "output")); err != nil {
		t.Fatalf("valid private root rejected: %v", err)
	}
}

func TestValidateContainedRejectsWidenedRootOrComponent(t *testing.T) {
	root, err := CreateRuntimeRoot(t.TempDir(), "mindline-private-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateContained(root, filepath.Join(root, "input.json")); err == nil {
		t.Fatal("widened private root was accepted")
	}
	if err := os.Chmod(root, DirMode); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "widened")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateContained(root, filepath.Join(dir, "input.json")); err == nil {
		t.Fatal("widened private component was accepted")
	}
}

func TestValidateContainedRejectsUnsafeDescendantTree(t *testing.T) {
	root, err := CreateRuntimeRoot(t.TempDir(), "mindline-private-")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input")
	if err := os.Mkdir(input, DirMode); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(input, "nested.json")
	if err := os.WriteFile(nested, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateContained(root, input); err == nil {
		t.Fatal("group/world-readable descendant was accepted")
	}
	if err := os.Chmod(nested, FileMode); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("{}\n"), FileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(nested); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, nested); err != nil {
		t.Fatal(err)
	}
	if err := ValidateContained(root, input); err == nil {
		t.Fatal("symlinked descendant was accepted")
	}
}
