package privateio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileBoundedAcceptsExactLimitAndRejectsOversize(t *testing.T) {
	root, err := CreateRuntimeRoot(t.TempDir(), "bounded-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "input.bin")
	if err := os.WriteFile(path, []byte("12345"), FileMode); err != nil {
		t.Fatal(err)
	}
	data, err := ReadFileBounded(root, path, 5)
	if err != nil || string(data) != "12345" {
		t.Fatalf("exact limit read = %q, %v", data, err)
	}
	if _, err := ReadFileBounded(root, path, 4); !errors.Is(err, ErrReadLimitExceeded) {
		t.Fatalf("oversize error = %v", err)
	}
	if _, err := ReadFileBounded(root, path, 0); !errors.Is(err, ErrInvalidReadLimit) {
		t.Fatalf("invalid limit error = %v", err)
	}
}

func TestReadJSONStrictBoundedKeepsClosedSchema(t *testing.T) {
	root, err := CreateRuntimeRoot(t.TempDir(), "bounded-json-")
	if err != nil {
		t.Fatal(err)
	}
	type record struct {
		Name string `json:"name"`
	}
	path := filepath.Join(root, "record.json")
	if err := os.WriteFile(path, []byte(`{"name":"fixture"}`), FileMode); err != nil {
		t.Fatal(err)
	}
	var value record
	if err := ReadJSONStrictBounded(root, path, 64, &value); err != nil || value.Name != "fixture" {
		t.Fatalf("bounded strict read = %+v, %v", value, err)
	}
	if err := os.WriteFile(path, []byte(`{"name":"fixture","secret":"blocked"}`), FileMode); err != nil {
		t.Fatal(err)
	}
	if err := ReadJSONStrictBounded(root, path, 64, &value); err == nil {
		t.Fatal("unknown field was accepted")
	}
}
