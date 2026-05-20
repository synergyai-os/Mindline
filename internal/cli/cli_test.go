package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProcessPrintsDeterministicEnvelopeToStdoutByDefault(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("candidate.json", []byte(publishCandidate("candidate-publish", "slack:publish")))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"process", "candidate.json"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if fs.WriteCountExcept("candidate.json") != 0 {
		t.Fatalf("expected no artifact files by default")
	}
	expected := `{
  "state": "dry_run_published",
  "record_id": "candidate-publish",
  "source_candidate_id": "candidate-publish",
  "idempotency_key": "slack:publish",
  "safety": {
    "private_provenance": false,
    "redaction_required": false,
    "secret_like": false
  },
  "artifact_count": 1,
  "artifacts": [
    {
      "kind": "dry_run_publish",
      "path": "",
      "body": "---\ntype: Source\nstatus: dry_run\ndomain: Tolaria PKM OS\ntopics:\n  - knowledge-management\n  - code-workflow\nsource_adapter: slack\nsource_url: https://public.example/source\nconfidence: high\nprocessing_status: dry_run_published\nvisibility: publish\nschema_version: v0.1\ncandidate_id: candidate-publish\n---\n\n# CODE workflow source\n\n## Snapshot\nA useful source about CODE workflow.\n\n## Source Content\n- Source: https://public.example/source\n- Captured from: slack\n- Author: Randy\n\n## Key Details\n- A useful source about CODE workflow.\n\n## Relevance\nClassified under Tolaria PKM OS with high confidence.\n\n## Signals\n- knowledge-management\n- code-workflow\n\n## Related Sources\n- https://public.example/source\n\n## Next Action\nNo immediate action. Keep as processed source reference.\n"
    }
  ],
  "authority_ids": [
    "DEC-4",
    "DEC-3",
    "DEC-2",
    "DEC-1",
    "FEAT-1",
    "STD-1",
    "STD-7",
    "STD-10",
    "STD-11",
    "STD-12",
    "FEAT-4",
    "WP-1"
  ]
}
`
	if stdout.String() != expected {
		t.Fatalf("unexpected deterministic stdout\n--- got ---\n%s\n--- want ---\n%s", stdout.String(), expected)
	}
	if !strings.HasSuffix(stdout.String(), "\n") {
		t.Fatalf("expected trailing newline")
	}
	if strings.HasSuffix(strings.TrimSuffix(stdout.String(), "\n"), "\n") {
		t.Fatalf("expected exactly one trailing newline")
	}
}

func TestProcessWritesArtifactsOnlyWithExplicitOut(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("candidate.json", []byte(publishCandidate("candidate-publish", "slack:publish")))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"process", "candidate.json", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	artifactPath := cleanPath("dry-run/candidate-publish-publish.md")
	if !fs.Exists(artifactPath) {
		t.Fatalf("expected artifact %q to be written", artifactPath)
	}
	if !strings.Contains(string(fs.MustReadFile(artifactPath)), "## Snapshot") {
		t.Fatalf("expected artifact body to be written")
	}
	var envelope ResultEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if envelope.Artifacts[0].Path != artifactPath {
		t.Fatalf("expected path %q, got %q", artifactPath, envelope.Artifacts[0].Path)
	}
	if envelope.Artifacts[0].Body != "" {
		t.Fatalf("expected stdout artifact body to be empty with --out")
	}
	if string(fs.MustReadFile(artifactPath)) != expectedPublishMarkdown() {
		t.Fatalf("artifact file did not exactly match engine body\n--- got ---\n%s\n--- want ---\n%s", string(fs.MustReadFile(artifactPath)), expectedPublishMarkdown())
	}
}

func TestUsageErrorsReturnExitOne(t *testing.T) {
	cases := [][]string{
		{},
		{"unknown"},
		{"process"},
		{"process", "candidate.json", "--bad"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := NewRunner(NewMemoryFS()).Run(args, &stdout, &stderr)
			if code != ExitUsage {
				t.Fatalf("expected exit %d, got %d", ExitUsage, code)
			}
			if stdout.String() != "" {
				t.Fatalf("expected empty stdout, got %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), "usage: mindline process <candidate.json> [--out <dir>]") {
				t.Fatalf("expected usage stderr, got %q", stderr.String())
			}
		})
	}
}

func TestUnreadableInputReturnsExitOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewMemoryFS()).Run([]string{"process", "missing.json"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected exit %d, got %d", ExitUsage, code)
	}
	if !strings.Contains(stderr.String(), "read candidate:") {
		t.Fatalf("expected read candidate stderr, got %q", stderr.String())
	}
}

func TestInvalidCandidateReturnsExitTwoAndWritesNoArtifacts(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("bad.json", []byte(`{"schema_version":"v0.1"}`))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"process", "bad.json", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitProcess {
		t.Fatalf("expected exit %d, got %d", ExitProcess, code)
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "process candidate:") {
		t.Fatalf("expected process candidate stderr, got %q", stderr.String())
	}
	if fs.Exists(cleanPath("dry-run")) {
		t.Fatalf("expected invalid candidate to write no output dir/artifacts")
	}
}

func TestInvalidOutReturnsExitOne(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("candidate.json", []byte(publishCandidate("candidate-publish", "slack:publish")))
	fs.WriteFile("not-dir", []byte("file"))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"process", "candidate.json", "--out", "not-dir"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected exit %d, got %d", ExitUsage, code)
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid --out:") {
		t.Fatalf("expected invalid out stderr, got %q", stderr.String())
	}
}

func TestUnwritableOutReturnsInvalidOut(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("candidate.json", []byte(publishCandidate("candidate-publish", "slack:publish")))
	fs.FailCanWriteUnder(cleanPath("dry-run"))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"process", "candidate.json", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected exit %d, got %d", ExitUsage, code)
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid --out:") {
		t.Fatalf("expected invalid out stderr, got %q", stderr.String())
	}
}

func TestExistingUnwritableOutReturnsInvalidOut(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("candidate.json", []byte(publishCandidate("candidate-publish", "slack:publish")))
	if err := fs.MkdirAll("dry-run", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fs.FailCanWriteUnder(cleanPath("dry-run"))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"process", "candidate.json", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected exit %d, got %d", ExitUsage, code)
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid --out:") {
		t.Fatalf("expected invalid out stderr, got %q", stderr.String())
	}
}

func TestEmptyOutReturnsInvalidOut(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("candidate.json", []byte(publishCandidate("candidate-publish", "slack:publish")))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"process", "candidate.json", "--out", ""}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected exit %d, got %d", ExitUsage, code)
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid --out:") {
		t.Fatalf("expected invalid out stderr, got %q", stderr.String())
	}
}

func TestArtifactWriteFailureReturnsExitThree(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("candidate.json", []byte(publishCandidate("candidate-publish", "slack:publish")))
	fs.FailWritesUnder(cleanPath("dry-run"))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"process", "candidate.json", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitArtifactWrite {
		t.Fatalf("expected exit %d, got %d", ExitArtifactWrite, code)
	}
	if stdout.String() != "" {
		t.Fatalf("expected no success envelope, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "write artifact:") {
		t.Fatalf("expected write artifact stderr, got %q", stderr.String())
	}
}

func TestStateEnvelopesAreDeterministicForNonPublishOutcomes(t *testing.T) {
	cases := []struct {
		name      string
		candidate string
		state     string
		count     int
	}{
		{name: "attention", candidate: candidateWithVisibility("candidate-attention", "attention"), state: "attention_ready", count: 1},
		{name: "background", candidate: candidateWithVisibility("candidate-background", "background"), state: "background_ready", count: 0},
		{name: "needs-enrichment", candidate: candidateWithEnrichment("candidate-enrichment", "incomplete", "publish"), state: "needs_enrichment", count: 0},
		{name: "skipped", candidate: candidateWithSafety("candidate-skipped", "secret_like", "publish"), state: "skipped", count: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := NewMemoryFS()
			fs.WriteFile("candidate.json", []byte(tc.candidate))
			var stdout, stderr bytes.Buffer
			code := NewRunner(fs).Run([]string{"process", "candidate.json"}, &stdout, &stderr)
			if code != ExitOK {
				t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
			}
			var envelope ResultEnvelope
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("decode stdout: %v", err)
			}
			if envelope.State != tc.state {
				t.Fatalf("expected state %q, got %q", tc.state, envelope.State)
			}
			if envelope.ArtifactCount != tc.count {
				t.Fatalf("expected artifact count %d, got %d", tc.count, envelope.ArtifactCount)
			}
		})
	}
}

func TestNonPublishOutcomesWriteNoFilesEvenWithOut(t *testing.T) {
	cases := []struct {
		name      string
		candidate string
	}{
		{name: "background", candidate: candidateWithVisibility("candidate-background", "background")},
		{name: "needs-enrichment", candidate: candidateWithEnrichment("candidate-enrichment", "incomplete", "publish")},
		{name: "skipped", candidate: candidateWithSafety("candidate-skipped", "secret_like", "publish")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := NewMemoryFS()
			fs.WriteFile("candidate.json", []byte(tc.candidate))
			var stdout, stderr bytes.Buffer
			code := NewRunner(fs).Run([]string{"process", "candidate.json", "--out", "dry-run"}, &stdout, &stderr)
			if code != ExitOK {
				t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
			}
			if fs.WriteCountExcept("candidate.json") != 0 {
				t.Fatalf("expected no artifact files for %s with --out; files=%v", tc.name, fs.Paths())
			}
		})
	}
}

func TestPrivateProvenanceDoesNotLeakThroughCLI(t *testing.T) {
	fs := NewMemoryFS()
	input := publishCandidate("../secret/private-candidate", "slack:private")
	input = strings.Replace(input, `"permalink": {"value": "https://public.example/source", "visibility": "public"}`, `"permalink": {"value": "https://private.example/source", "visibility": "private"}`, 1)
	input = strings.Replace(input, `"author": {"value": "Randy", "visibility": "public"}`, `"author": {"value": "Randy Private", "visibility": "private"}`, 1)
	fs.WriteFile("candidate.json", []byte(input))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"process", "candidate.json", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	for _, leaked := range []string{"https://private.example/source", "Randy Private"} {
		if strings.Contains(stdout.String(), leaked) {
			t.Fatalf("stdout leaked %q:\n%s", leaked, stdout.String())
		}
	}
	if fs.WriteCountExcept("candidate.json") != 0 {
		t.Fatalf("expected private provenance publish block to write no artifacts")
	}
}

func TestAnyPrivateProvenanceFieldBlocksPublishThroughCLI(t *testing.T) {
	fs := NewMemoryFS()
	input := publishCandidate("candidate-raw-private", "slack:raw-private")
	input = strings.Replace(input, `"raw_locator": {"value": "slack://D123/msg-1", "visibility": "public"}`, `"raw_locator": {"value": "slack://D123/private", "visibility": "private"}`, 1)
	fs.WriteFile("candidate.json", []byte(input))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"process", "candidate.json", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if strings.Contains(stdout.String(), "slack://D123/private") {
		t.Fatalf("stdout leaked private raw locator:\n%s", stdout.String())
	}
	var envelope ResultEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if envelope.State != "background_ready" {
		t.Fatalf("expected private provenance to block publish into background_ready, got %q", envelope.State)
	}
	if fs.WriteCountExcept("candidate.json") != 0 {
		t.Fatalf("expected private provenance publish block to write no artifacts")
	}
}

func TestOutPathContainmentSanitizesCandidateIDs(t *testing.T) {
	cases := []string{
		"../secret/private-candidate",
		"/tmp/private-candidate",
		"C:\\\\tmp\\\\private-candidate",
	}
	for _, candidateID := range cases {
		t.Run(candidateID, func(t *testing.T) {
			fs := NewMemoryFS()
			fs.WriteFile("candidate.json", []byte(publishCandidate(candidateID, "slack:path:"+candidateID)))

			var stdout, stderr bytes.Buffer
			code := NewRunner(fs).Run([]string{"process", "candidate.json", "--out", "dry-run"}, &stdout, &stderr)

			if code != ExitOK {
				t.Fatalf("expected exit %d, got %d stderr=%s", ExitOK, code, stderr.String())
			}
			var artifactPaths []string
			for _, path := range fs.Paths() {
				if path == "candidate.json" || path == "dry-run" {
					continue
				}
				artifactPaths = append(artifactPaths, path)
				if filepath.Dir(path) != "dry-run" {
					t.Fatalf("path escaped output dir: %q", path)
				}
				if strings.Contains(path, ".."+pathSeparator()) || strings.HasPrefix(path, pathSeparator()) {
					t.Fatalf("path escaped containment: %q", path)
				}
				for _, r := range filepath.Base(path) {
					if !isAllowedFilenameRune(r) {
						t.Fatalf("filename contains disallowed rune %q in %q", r, path)
					}
				}
			}
			if len(artifactPaths) != 1 {
				t.Fatalf("expected one artifact path, got %v", artifactPaths)
			}
		})
	}
}

func expectedPublishMarkdown() string {
	return `---
type: Source
status: dry_run
domain: Tolaria PKM OS
topics:
  - knowledge-management
  - code-workflow
source_adapter: slack
source_url: https://public.example/source
confidence: high
processing_status: dry_run_published
visibility: publish
schema_version: v0.1
candidate_id: candidate-publish
---

# CODE workflow source

## Snapshot
A useful source about CODE workflow.

## Source Content
- Source: https://public.example/source
- Captured from: slack
- Author: Randy

## Key Details
- A useful source about CODE workflow.

## Relevance
Classified under Tolaria PKM OS with high confidence.

## Signals
- knowledge-management
- code-workflow

## Related Sources
- https://public.example/source

## Next Action
No immediate action. Keep as processed source reference.
`
}

func isAllowedFilenameRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '-', r == '_', r == '.':
		return true
	default:
		return false
	}
}

func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	var gotValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("decode got JSON: %v\n%s", err, got)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode want JSON: %v\n%s", err, want)
	}
	gotCanonical, _ := json.MarshalIndent(gotValue, "", "  ")
	wantCanonical, _ := json.MarshalIndent(wantValue, "", "  ")
	if string(gotCanonical) != string(wantCanonical) {
		t.Fatalf("unexpected JSON\n--- got ---\n%s\n--- want ---\n%s", gotCanonical, wantCanonical)
	}
}

func cleanPath(path string) string {
	return filepath.Clean(path)
}

func pathSeparator() string {
	if runtime.GOOS == "windows" {
		return `\`
	}
	return "/"
}

func publishCandidate(candidateID, idempotencyKey string) string {
	return `{
		"schema_version": "v0.1",
		"candidate_id": "` + candidateID + `",
		"adapter_id": "slack",
		"external_id": "msg-1",
		"captured_at": "2026-05-20T10:00:00Z",
		"provenance": {
			"permalink": {"value": "https://public.example/source", "visibility": "public"},
			"native_timestamp": {"value": "2026-05-20T10:00:00Z", "visibility": "public"},
			"author": {"value": "Randy", "visibility": "public"},
			"raw_locator": {"value": "slack://D123/msg-1", "visibility": "public"}
		},
		"content": {
			"text": "A useful source about CODE workflow.",
			"urls": ["https://public.example/source"],
			"attachments": [],
			"source_title": "CODE workflow source"
		},
		"enrichment_status": "complete",
		"classification": {
			"type": "Source",
			"domain": "Tolaria PKM OS",
			"topics": ["knowledge-management", "code-workflow"],
			"confidence": "high",
			"needs_clarification": false,
			"clarification_reason": ""
		},
		"safety": {
			"redaction_required": false,
			"secret_like": false,
			"empty_content": false,
			"private_provenance": false
		},
		"desired_visibility": "publish",
		"idempotency_key": "` + idempotencyKey + `"
	}`
}

func candidateWithVisibility(candidateID, visibility string) string {
	return strings.Replace(publishCandidate(candidateID, "slack:"+candidateID), `"desired_visibility": "publish"`, `"desired_visibility": "`+visibility+`"`, 1)
}

func candidateWithEnrichment(candidateID, enrichmentStatus, visibility string) string {
	input := candidateWithVisibility(candidateID, visibility)
	return strings.Replace(input, `"enrichment_status": "complete"`, `"enrichment_status": "`+enrichmentStatus+`"`, 1)
}

func candidateWithSafety(candidateID, safetyField, visibility string) string {
	input := candidateWithVisibility(candidateID, visibility)
	return strings.Replace(input, `"`+safetyField+`": false`, `"`+safetyField+`": true`, 1)
}
