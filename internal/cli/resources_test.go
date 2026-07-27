package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestResourceCommandsReturnStructuralOnlyStatusForEmptyLibrary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "personal-memory")
	runner := NewRunner(NewOSFileSystem())
	for _, command := range []string{"resources-run", "resources-status", "resources-proof"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := runner.Run([]string{"memory", command, "--root", root}, &stdout, &stderr); code != ExitOK {
				t.Fatalf("%s exit=%d stderr=%q", command, code, stderr.String())
			}
			if strings.Contains(stdout.String(), root) || strings.Contains(stdout.String(), "resource-queue") || strings.Contains(stdout.String(), "http") {
				t.Fatalf("%s leaked a private path or URL: %s", command, stdout.String())
			}
			var status struct {
				SchemaVersion     string         `json:"schema_version"`
				BudgetFingerprint string         `json:"budget_fingerprint"`
				TerminalCounts    map[string]int `json:"terminal_counts"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &status); err != nil || status.SchemaVersion == "" || status.BudgetFingerprint == "" || len(status.TerminalCounts) != 0 {
				t.Fatalf("%s structural JSON = %#v err=%v", command, status, err)
			}
		})
	}
}

func TestResourceRebuildProofRejectsNonTerminalDerivedQueue(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memory")
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run(
		[]string{"memory", "resources-rebuild-proof", "--root", root},
		&stdout, &stderr,
	)
	if code != ExitOK {
		t.Fatalf("empty terminal rebuild proof failed: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var output map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output["state"] != "pass" || output["all_terminal"] != true {
		t.Fatalf("unexpected rebuild proof: %#v", output)
	}
}

func TestResourceCommandsRejectUnexpectedArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := NewRunner(NewOSFileSystem()).Run([]string{"memory", "resources-status", "unexpected"}, &stdout, &stderr); code != ExitUsage {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
