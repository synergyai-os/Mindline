package localservice

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeBindingHashesExactExecutableConfigAndEmbeddedTree(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-binding-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(config.RuntimeRoot, "config.json")
	if err := SaveConfig(configPath, config); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "mindline")
	if err := os.WriteFile(executable, []byte("exact-candidate-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	previousTree := sourceTreeFingerprint
	sourceTreeFingerprint = "sha256:" + strings.Repeat("b", 64)
	t.Cleanup(func() { sourceTreeFingerprint = previousTree })

	binding := runtimeBindingFor(executable, configPath, config)
	if binding.State != "ready" || binding.TreeFingerprint != sourceTreeFingerprint {
		t.Fatalf("exact runtime binding not ready: %+v", binding)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if binding.BuildFingerprint != "sha256:"+hex.EncodeToString(digest[:]) {
		t.Fatalf("runtime bound the wrong executable: %+v", binding)
	}
	if err := os.WriteFile(configPath, append([]byte(" "), []byte("tampered")...), 0o600); err != nil {
		t.Fatal(err)
	}
	tampered := runtimeBindingFor(executable, configPath, config)
	if tampered.State != "ready" || tampered.ConfigurationFingerprint == binding.ConfigurationFingerprint {
		t.Fatalf("runtime config bytes were not rebound: before=%+v after=%+v", binding, tampered)
	}
	sourceTreeFingerprint = ""
	if unavailable := runtimeBindingFor(executable, configPath, config); unavailable.State != "unavailable" {
		t.Fatalf("ordinary build claimed an audited tree: %+v", unavailable)
	}
}

func TestEvaluationLeaseBlocksLifecycleReplacementForCompleteRun(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mindline-eval-lease-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	config, err := ConfigFromRoots(filepath.Join(root, "runtime"), filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(filepath.Join(config.RuntimeRoot, "config.json"), config); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireLifecycleLock(config.RuntimeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireEvaluationLease(config.SocketPath)
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle, err := acquireLifecycleLock(config.RuntimeRoot); err == nil {
		_ = lifecycle.Close()
		t.Fatal("lifecycle replacement acquired the complete-evaluation lease")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	lifecycle, err := acquireLifecycleLock(config.RuntimeRoot)
	if err != nil {
		t.Fatalf("released evaluation lease still blocked lifecycle: %v", err)
	}
	_ = lifecycle.Close()
}
