package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunScansTrackedSourceAndExplicitProofDirectory(t *testing.T) {
	root := newGitRepository(t)
	writeFile(t, filepath.Join(root, "source.txt"), "safe source\n")
	git(t, root, "add", "source.txt")
	proof := filepath.Join(root, "generated-proof")
	writeFile(t, filepath.Join(proof, "receipt.json"), `{"state":"pass"}`)

	var output bytes.Buffer
	err := run([]string{"--root", root, "--self-test", "--proof-dir", proof}, &output, detectCredential)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var got receipt
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	if got.SchemaVersion != contractVersion || got.State != "clean" || got.SelfTest != "passed" ||
		got.TrackedFileCount != 1 || got.ProofFileCount != 1 || got.FindingCount != 0 {
		t.Fatalf("receipt = %#v", got)
	}
}

func TestRunBlocksWithoutLeakingTrackedFinding(t *testing.T) {
	root := newGitRepository(t)
	secret := string(selfTestSentinel())
	path := filepath.Join(root, "private.txt")
	writeFile(t, path, secret)
	git(t, root, "add", "private.txt")

	var output bytes.Buffer
	err := run([]string{"--root", root, "--self-test"}, &output, detectCredential)
	if !errors.Is(err, errBlocked) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(output.String(), secret) || strings.Contains(output.String(), path) || strings.Contains(output.String(), "private.txt") {
		t.Fatalf("receipt leaked finding metadata: %q", output.String())
	}
	var got receipt
	if json.Unmarshal(output.Bytes(), &got) != nil || got.State != "blocked" || got.FindingCount != 1 {
		t.Fatalf("receipt = %#v (%q)", got, output.String())
	}
}

func TestRunBlocksFindingInExplicitProofDirectory(t *testing.T) {
	root := newGitRepository(t)
	writeFile(t, filepath.Join(root, "source.txt"), "safe\n")
	git(t, root, "add", "source.txt")
	proof := filepath.Join(root, "proof")
	writeFile(t, filepath.Join(proof, "generated.json"), string(selfTestSentinel()))

	var output bytes.Buffer
	err := run([]string{"--root", root, "--self-test", "--proof-dir", proof}, &output, detectCredential)
	if !errors.Is(err, errBlocked) {
		t.Fatalf("error = %v", err)
	}
	var got receipt
	if json.Unmarshal(output.Bytes(), &got) != nil || got.ProofFileCount != 1 || got.FindingCount != 1 {
		t.Fatalf("receipt = %#v", got)
	}
}

func TestDetectorFindsQuotedCredentialField(t *testing.T) {
	value := "aB3dE5fG7hJ9kL2mN4pQ6rS8"
	body := []byte(`{"openai_api_key":"` + value + `"}`)
	if !detectCredential(body) {
		t.Fatal("quoted credential field was not detected")
	}
}

func TestSelfTestRejectsNoOpDetector(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"--root", t.TempDir(), "--self-test"}, &output, func([]byte) bool { return false })
	if !errors.Is(err, errScannerFailed) || output.Len() != 0 {
		t.Fatalf("error = %v, output = %q", err, output.String())
	}
}

func TestRunRequiresRootAndSelfTest(t *testing.T) {
	for _, args := range [][]string{{"--self-test"}, {"--root", t.TempDir()}} {
		if err := run(args, &bytes.Buffer{}, detectCredential); !errors.Is(err, errInvalidInput) {
			t.Fatalf("args %v: error = %v", args, err)
		}
	}
}

func newGitRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "--quiet")
	return root
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git command failed: %v (%s)", err, output)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
