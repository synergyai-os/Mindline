package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/recalleval"
)

type fixedStatusPort struct {
	status localservice.Status
	err    error
}

func (port fixedStatusPort) Status(context.Context) (localservice.Status, error) {
	return port.status, port.err
}

func TestVerifyServiceBindingRequiresExactLiveRuntime(t *testing.T) {
	binding := recalleval.RunBinding{
		BuildFingerprint:         "sha256:" + string(bytes.Repeat([]byte("a"), 64)),
		TreeFingerprint:          "sha256:" + string(bytes.Repeat([]byte("b"), 64)),
		ConfigurationFingerprint: "sha256:" + string(bytes.Repeat([]byte("c"), 64)),
	}
	matching := localservice.Status{RuntimeBinding: localservice.RuntimeBinding{
		State: "ready", BuildFingerprint: binding.BuildFingerprint,
		TreeFingerprint:          binding.TreeFingerprint,
		ConfigurationFingerprint: binding.ConfigurationFingerprint,
	}}
	if err := verifyServiceBinding(context.Background(), fixedStatusPort{status: matching}, binding); err != nil {
		t.Fatalf("matching live runtime rejected: %v", err)
	}
	matching.RuntimeBinding.TreeFingerprint = "sha256:" + string(bytes.Repeat([]byte("d"), 64))
	if err := verifyServiceBinding(context.Background(), fixedStatusPort{status: matching}, binding); err == nil {
		t.Fatal("mismatched live runtime accepted")
	}
	matching.RuntimeBinding.State = "unavailable"
	if err := verifyServiceBinding(context.Background(), fixedStatusPort{status: matching}, binding); err == nil {
		t.Fatal("unattested live runtime accepted")
	}
	if err := verifyServiceBinding(context.Background(), fixedStatusPort{err: errors.New("offline")}, binding); err == nil {
		t.Fatal("unavailable live service accepted")
	}
}

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
