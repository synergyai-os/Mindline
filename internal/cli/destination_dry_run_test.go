package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDestinationDryRunWritesTolariaOperationLayout(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("result.json", []byte(destinationInput("dry_run_published", "candidate-publish", "slack:publish", "dry_run_publish", expectedPublishMarkdown(), false, false, false)))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"destination", "dry-run", "result.json", "--adapter", "tolaria", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	var summary DestinationDryRunSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if summary.DestinationAdapterID != "tolaria" {
		t.Fatalf("expected tolaria adapter, got %q", summary.DestinationAdapterID)
	}
	if summary.WriteMode != "dry_run" {
		t.Fatalf("expected dry_run write mode, got %q", summary.WriteMode)
	}
	if summary.OperationCount != 1 {
		t.Fatalf("expected one operation, got %d", summary.OperationCount)
	}
	if summary.BlockedCount != 0 {
		t.Fatalf("expected zero blocked operations, got %d", summary.BlockedCount)
	}
	if len(summary.Operations) != 1 {
		t.Fatalf("expected one summary operation, got %d", len(summary.Operations))
	}
	item := summary.Operations[0]
	if item.OperationType != "create_note" || item.VisibilityLane != "publish" || item.Blocked {
		t.Fatalf("unexpected operation summary: %+v", item)
	}
	if item.OperationJSONPath == "" || item.PreviewPath == "" {
		t.Fatalf("expected operation and preview paths: %+v", item)
	}
	if !fs.Exists(item.OperationJSONPath) {
		t.Fatalf("expected operation JSON file %q", item.OperationJSONPath)
	}
	if !fs.Exists(item.PreviewPath) {
		t.Fatalf("expected preview file %q", item.PreviewPath)
	}
	if !strings.Contains(string(fs.MustReadFile(item.PreviewPath)), "source_candidate_id: candidate-publish") {
		t.Fatalf("expected neutral preview body")
	}
	if string(fs.MustReadFile("dry-run/destination-summary.json")) != stdout.String() {
		t.Fatalf("summary file and stdout differ")
	}
}

func TestDestinationDryRunRequiresOutAndTolariaAdapter(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("result.json", []byte(destinationInput("dry_run_published", "candidate-publish", "slack:publish", "dry_run_publish", expectedPublishMarkdown(), false, false, false)))

	cases := [][]string{
		{"destination", "dry-run", "result.json", "--adapter", "tolaria"},
		{"destination", "dry-run", "result.json", "--out", "dry-run"},
		{"destination", "dry-run", "result.json", "--adapter", "notion", "--out", "dry-run"},
		{"destination", "write", "result.json", "--adapter", "tolaria", "--out", "dry-run"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := NewRunner(fs).Run(args, &stdout, &stderr)
			if code != ExitUsage {
				t.Fatalf("expected exit %d, got %d", ExitUsage, code)
			}
			if stdout.String() != "" {
				t.Fatalf("expected empty stdout, got %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), "usage: mindline destination dry-run") {
				t.Fatalf("expected destination usage, got %q", stderr.String())
			}
		})
	}
}

func TestDestinationDryRunWritesNoPreviewForBackgroundSkipOrBlocked(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantType  string
		wantLane  string
		wantBlock bool
	}{
		{
			name:     "background",
			input:    destinationInput("background_ready", "candidate-background", "slack:background", "processing_record", "background trace", false, false, false),
			wantType: "background_record",
			wantLane: "background",
		},
		{
			name:      "skipped",
			input:     destinationInput("skipped", "candidate-skipped", "slack:skipped", "processing_record", "skipped trace", false, false, true),
			wantType:  "skip",
			wantLane:  "skip",
			wantBlock: true,
		},
		{
			name:      "needs_enrichment",
			input:     destinationInput("needs_enrichment", "candidate-needs-enrichment", "slack:needs-enrichment", "processing_record", "enrichment trace", false, false, false),
			wantType:  "blocked",
			wantLane:  "blocked",
			wantBlock: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := NewMemoryFS()
			fs.WriteFile("result.json", []byte(tc.input))
			var stdout, stderr bytes.Buffer
			code := NewRunner(fs).Run([]string{"destination", "dry-run", "result.json", "--adapter", "tolaria", "--out", "dry-run"}, &stdout, &stderr)
			if code != ExitOK {
				t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
			}
			var summary DestinationDryRunSummary
			if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
				t.Fatalf("decode stdout: %v", err)
			}
			item := summary.Operations[0]
			if item.OperationType != tc.wantType || item.VisibilityLane != tc.wantLane || item.Blocked != tc.wantBlock {
				t.Fatalf("unexpected operation summary: %+v", item)
			}
			if item.PreviewPath != "" {
				t.Fatalf("expected no preview path, got %q", item.PreviewPath)
			}
			if strings.Contains(strings.Join(fs.Paths(), "\n"), "/previews/") {
				t.Fatalf("expected no preview files, got paths %v", fs.Paths())
			}
		})
	}
}

func TestDestinationDryRunDoesNotLeakSecretMaterial(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("result.json", []byte(destinationInput("dry_run_published", "super-secret/private-candidate", "xoxb-super-secret-token", "dry_run_publish", expectedPublishMarkdown(), false, true, true)))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"destination", "dry-run", "result.json", "--adapter", "tolaria", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	for _, leaked := range []string{"super-secret/private-candidate", "xoxb-super-secret-token"} {
		if strings.Contains(stdout.String(), leaked) {
			t.Fatalf("stdout leaked %q:\n%s", leaked, stdout.String())
		}
		for _, path := range fs.Paths() {
			if path == "result.json" {
				continue
			}
			if strings.Contains(path, leaked) {
				t.Fatalf("path leaked %q: %s", leaked, path)
			}
			if strings.Contains(string(fs.MustReadFileIfFile(path)), leaked) {
				t.Fatalf("file %q leaked %q", path, leaked)
			}
		}
	}
}

func TestDestinationDryRunDoesNotLeakPrivateFlaggedOrdinaryIdentifiers(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("result.json", []byte(destinationInput("dry_run_published", "private-client-note-42", "client-followup-42", "dry_run_publish", expectedPublishMarkdown(), true, false, false)))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"destination", "dry-run", "result.json", "--adapter", "tolaria", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	for _, leaked := range []string{"private-client-note-42", "client-followup-42"} {
		if strings.Contains(stdout.String(), leaked) {
			t.Fatalf("stdout leaked %q:\n%s", leaked, stdout.String())
		}
		for _, path := range fs.Paths() {
			if path == "result.json" {
				continue
			}
			if strings.Contains(path, leaked) {
				t.Fatalf("path leaked %q: %s", leaked, path)
			}
			if strings.Contains(string(fs.MustReadFileIfFile(path)), leaked) {
				t.Fatalf("file %q leaked %q", path, leaked)
			}
		}
	}
}

func TestDestinationDryRunWritesRedactedAttentionPreview(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("result.json", []byte(destinationInput("attention_ready", "candidate-attention", "client-followup-42", "attention_preview", "Redaction required\n- Source: [redacted]\n", true, false, false)))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"destination", "dry-run", "result.json", "--adapter", "tolaria", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	var summary DestinationDryRunSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	item := summary.Operations[0]
	if item.OperationType != "attention_preview" || item.PreviewPath == "" || item.Blocked {
		t.Fatalf("expected attention preview with preview path, got %+v", item)
	}
	if !strings.Contains(strings.Join(fs.Paths(), "\n"), "/previews/") {
		t.Fatalf("expected preview file, got paths %v", fs.Paths())
	}
	for _, path := range fs.Paths() {
		if path == "result.json" {
			continue
		}
		for _, leaked := range []string{"candidate-attention", "client-followup-42"} {
			if strings.Contains(string(fs.MustReadFileIfFile(path)), leaked) || strings.Contains(path, leaked) {
				t.Fatalf("private identifier %q leaked in %q", leaked, path)
			}
		}
	}
}

func TestDestinationDryRunRejectsProtectedOutputRootAndSymlink(t *testing.T) {
	base := t.TempDir()
	vault := filepath.Join(base, "protected-vault")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(vault, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	inputPath := filepath.Join(base, "result.json")
	if err := os.WriteFile(inputPath, []byte(destinationInput("dry_run_published", "candidate-publish", "slack:publish", "dry_run_publish", expectedPublishMarkdown(), false, false, false)), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	symlinkOut := filepath.Join(outside, "vault-link")
	if err := os.Symlink(vault, symlinkOut); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	for _, outDir := range []string{vault, filepath.Join(vault, "dry-run"), symlinkOut} {
		t.Run(outDir, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := NewRunnerWithProtectedRoots(NewOSFileSystem(), []string{vault}).Run([]string{"destination", "dry-run", inputPath, "--adapter", "tolaria", "--out", outDir}, &stdout, &stderr)
			if code != ExitUsage {
				t.Fatalf("expected exit %d, got %d stderr=%s", ExitUsage, code, stderr.String())
			}
			if stdout.String() != "" {
				t.Fatalf("expected empty stdout, got %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), "protected output root") {
				t.Fatalf("expected protected root error, got %q", stderr.String())
			}
			if _, err := os.Stat(filepath.Join(vault, "dry-run")); err == nil {
				t.Fatalf("protected dry-run directory was created before rejection")
			}
		})
	}
}

func TestConfiguredProtectedRootsDefaultsWhenEnvUnset(t *testing.T) {
	t.Setenv(protectedRootsEnv, "")

	roots := configuredProtectedRoots()

	if len(roots) == 0 {
		t.Fatalf("expected default protected roots when %s is unset", protectedRootsEnv)
	}
	for _, root := range roots {
		if root == defaultTolariaProtectedRoot {
			return
		}
	}
	t.Fatalf("expected default Tolaria protected root %q in %+v", defaultTolariaProtectedRoot, roots)
}

func TestDestinationDryRunResolvesArtifactPathsFromInputDirectory(t *testing.T) {
	base := t.TempDir()
	inputDir := filepath.Join(base, "input")
	outDir := filepath.Join(base, "out")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("mkdir input: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "publish.md"), []byte(expectedPublishMarkdown()), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	inputPath := filepath.Join(inputDir, "result.json")
	if err := os.WriteFile(inputPath, []byte(destinationInputWithPathArtifact("dry_run_published", "candidate-publish", "slack:publish", "dry_run_publish", "publish.md")), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := NewRunnerWithProtectedRoots(NewOSFileSystem(), nil).Run([]string{"destination", "dry-run", inputPath, "--adapter", "tolaria", "--out", outDir}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"operation_type": "create_note"`) {
		t.Fatalf("expected create note summary, got %s", stdout.String())
	}
}

func TestDestinationDryRunRejectsFinalFileSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outDir := filepath.Join(base, "out")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(filepath.Join(outDir, "operations"), 0o755); err != nil {
		t.Fatalf("mkdir operations: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "previews"), 0o755); err != nil {
		t.Fatalf("mkdir previews: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	inputPath := filepath.Join(base, "result.json")
	if err := os.WriteFile(inputPath, []byte(destinationInput("dry_run_published", "candidate-publish", "slack:publish", "dry_run_publish", expectedPublishMarkdown(), false, false, false)), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	operationID := "tolaria-candidate-publish-create-note-aa3572c5145cb574"
	if err := os.Symlink(filepath.Join(outside, "operation.json"), filepath.Join(outDir, "operations", operationID+".json")); err != nil {
		t.Fatalf("operation symlink: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := NewRunnerWithProtectedRoots(NewOSFileSystem(), []string{outside}).Run([]string{"destination", "dry-run", inputPath, "--adapter", "tolaria", "--out", outDir}, &stdout, &stderr)

	if code != ExitArtifactWrite {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitArtifactWrite, code, stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "escaped output directory") && !strings.Contains(stderr.String(), "protected output root") {
		t.Fatalf("expected symlink containment error, got %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(outside, "operation.json")); err == nil {
		t.Fatalf("outside symlink target was written")
	}
}

func TestDestinationDryRunAcceptsProcessOutResultEnvelope(t *testing.T) {
	base := t.TempDir()
	outDir := filepath.Join(base, "out")
	if err := os.WriteFile(filepath.Join(base, "publish.md"), []byte(expectedPublishMarkdown()), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	inputPath := filepath.Join(base, "result.json")
	input := `{
  "state": "dry_run_published",
  "record_id": "candidate-publish",
  "source_candidate_id": "candidate-publish",
  "idempotency_key": "slack:publish",
  "safety": {"private_provenance": false, "redaction_required": false, "secret_like": false},
  "artifact_count": 1,
  "artifacts": [{"kind": "dry_run_publish", "path": "publish.md", "body": ""}],
  "authority_ids": ["WP-5", "DEC-12"]
}`
	if err := os.WriteFile(inputPath, []byte(input), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := NewRunnerWithProtectedRoots(NewOSFileSystem(), nil).Run([]string{"destination", "dry-run", inputPath, "--adapter", "tolaria", "--out", outDir}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"operation_type": "create_note"`) {
		t.Fatalf("expected create note summary, got %s", stdout.String())
	}
}

func TestDestinationDryRunAcceptsProcessOutNoArtifactStates(t *testing.T) {
	base := t.TempDir()
	inputPath := filepath.Join(base, "result.json")
	input := `{
  "state": "skipped",
  "record_id": "candidate-skipped",
  "source_candidate_id": "candidate-skipped",
  "idempotency_key": "slack:skipped",
  "safety": {"private_provenance": false, "redaction_required": false, "secret_like": true},
  "artifact_count": 0,
  "artifacts": [],
  "authority_ids": ["WP-5", "DEC-12"]
}`
	if err := os.WriteFile(inputPath, []byte(input), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := NewRunnerWithProtectedRoots(NewOSFileSystem(), nil).Run([]string{"destination", "dry-run", inputPath, "--adapter", "tolaria", "--out", filepath.Join(base, "out")}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"operation_type": "skip"`) {
		t.Fatalf("expected skip summary, got %s", stdout.String())
	}
}

func TestDestinationDryRunPreservesProcessOutSafetyAndIdempotency(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("candidate.json", []byte(candidateWithSafety("private-client-note-42", "redaction_required", "attention")))

	var processStdout, processStderr bytes.Buffer
	processCode := NewRunner(fs).Run([]string{"process", "candidate.json"}, &processStdout, &processStderr)
	if processCode != ExitOK {
		t.Fatalf("process expected exit %d, got %d stderr=%s", ExitOK, processCode, processStderr.String())
	}
	fs.WriteFile("result.json", processStdout.Bytes())

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"destination", "dry-run", "result.json", "--adapter", "tolaria", "--out", "dry-run"}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("destination expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	for _, leaked := range []string{"private-client-note-42", "slack:private-client-note-42"} {
		if strings.Contains(stdout.String(), leaked) {
			t.Fatalf("stdout leaked %q:\n%s", leaked, stdout.String())
		}
		for _, path := range fs.Paths() {
			if path == "candidate.json" || path == "result.json" {
				continue
			}
			if strings.Contains(path, leaked) || strings.Contains(string(fs.MustReadFileIfFile(path)), leaked) {
				t.Fatalf("destination output leaked %q in %q", leaked, path)
			}
		}
	}
	if !strings.Contains(stdout.String(), `"operation_type": "attention_preview"`) {
		t.Fatalf("expected attention preview summary, got %s", stdout.String())
	}
}

func destinationInput(state, candidateID, idempotencyKey, artifactKind, artifactBody string, privateProvenance, redactionRequired, secretLike bool) string {
	artifactJSON := ""
	if artifactKind != "" {
		body, _ := json.Marshal(artifactBody)
		artifactJSON = `{"kind": "` + artifactKind + `", "body": ` + string(body) + `}`
	}
	return `{
  "schema_version": "destination-input/v0.1",
  "result": {
    "state": "` + state + `",
    "record_id": "` + candidateID + `",
    "source_candidate_id": "` + candidateID + `",
    "idempotency_key": "` + idempotencyKey + `",
    "authority_ids": ["WP-5", "DEC-12"],
    "artifacts": [` + artifactJSON + `],
    "safety": {
      "private_provenance": ` + boolString(privateProvenance) + `,
      "redaction_required": ` + boolString(redactionRequired) + `,
      "secret_like": ` + boolString(secretLike) + `
    }
  }
}`
}

func destinationInputWithPathArtifact(state, candidateID, idempotencyKey, artifactKind, artifactPath string) string {
	return `{
  "schema_version": "destination-input/v0.1",
  "result": {
    "state": "` + state + `",
    "record_id": "` + candidateID + `",
    "source_candidate_id": "` + candidateID + `",
    "idempotency_key": "` + idempotencyKey + `",
    "authority_ids": ["WP-5", "DEC-12"],
    "artifacts": [{"kind": "` + artifactKind + `", "path": "` + artifactPath + `"}],
    "safety": {
      "private_provenance": false,
      "redaction_required": false,
      "secret_like": false
    }
  }
}`
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
