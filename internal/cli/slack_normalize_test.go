package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlackNormalizePrintsDeterministicEnvelopeToStdoutByDefault(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("slack.json", []byte(slackExportJSON()))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"slack", "normalize", "slack.json"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d got %d stderr=%s", ExitOK, code, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if fs.WriteCountExcept("slack.json") != 0 {
		t.Fatalf("expected no writes by default")
	}

	var envelope SlackNormalizeEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if envelope.AdapterID != "slack" || envelope.CandidateCount != 2 {
		t.Fatalf("bad envelope header: %#v", envelope)
	}
	if len(envelope.Candidates) != 2 || envelope.Candidates[0].Candidate == nil || envelope.Candidates[0].Path != "" {
		t.Fatalf("expected candidate bodies in stdout: %#v", envelope.Candidates)
	}
	if envelope.Candidates[0].Candidate.ExternalID != "DSELF:1710000000.000001" {
		t.Fatalf("expected old-to-new output, got %s", envelope.Candidates[0].Candidate.ExternalID)
	}
	if envelope.Checkpoint.BatchOrder != "old_to_new" || envelope.Checkpoint.CandidateCount != 2 {
		t.Fatalf("bad checkpoint: %#v", envelope.Checkpoint)
	}
}

func TestSlackNormalizeWritesCandidatesAndCheckpointWithOut(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("slack.json", []byte(slackExportJSON()))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"slack", "normalize", "slack.json", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d got %d stderr=%s", ExitOK, code, stderr.String())
	}
	var envelope SlackNormalizeEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if envelope.Candidates[0].Candidate != nil || envelope.Candidates[0].Path == "" {
		t.Fatalf("expected paths only with --out: %#v", envelope.Candidates[0])
	}
	checkpointPath := cleanPath("dry-run/slack-checkpoint.json")
	if !fs.Exists(checkpointPath) {
		t.Fatalf("missing checkpoint file")
	}
	for _, candidate := range envelope.Candidates {
		if !fs.Exists(candidate.Path) {
			t.Fatalf("missing candidate file %s", candidate.Path)
		}
		if !strings.HasPrefix(candidate.Path, cleanPath("dry-run")+string(filepath.Separator)) {
			t.Fatalf("candidate escaped out dir: %s", candidate.Path)
		}
	}
}

func TestSlackNormalizeDoesNotEmitForbiddenPrivateOrSecretStrings(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("slack.json", []byte(slackUnsafeExportJSON()))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"slack", "normalize", "slack.json", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("expected exit %d got %d stderr=%s", ExitOK, code, stderr.String())
	}
	output := stdout.String()
	for _, path := range fs.Paths() {
		output += "\n" + path
		if strings.HasPrefix(path, cleanPath("dry-run")+string(filepath.Separator)) {
			output += "\n" + string(fs.MustReadFile(path))
		}
	}
	for _, forbidden := range []string{privateFileMarker(), secretValue(), botToken(), liveKey(), publicFileURL()} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("forbidden value leaked %q in output:\n%s", forbidden, output)
		}
	}
	var envelope SlackNormalizeEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	for _, item := range envelope.Candidates {
		if strings.Contains(item.Path, "workspace.slack.com") {
			t.Fatalf("permalink leaked in generated path: %s", item.Path)
		}
	}
}

func TestSlackNormalizeUsageAndInputErrors(t *testing.T) {
	cases := [][]string{
		{"slack"},
		{"slack", "bad"},
		{"slack", "normalize"},
		{"slack", "normalize", "input.json", "--bad"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := NewRunner(NewMemoryFS()).Run(args, &stdout, &stderr)
			if code != ExitUsage {
				t.Fatalf("expected usage exit, got %d", code)
			}
			if stdout.String() != "" || !strings.Contains(stderr.String(), "usage:") {
				t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}

	var emptyOutStdout, emptyOutStderr bytes.Buffer
	code := NewRunner(NewMemoryFS()).Run([]string{"slack", "normalize", "input.json", "--out", ""}, &emptyOutStdout, &emptyOutStderr)
	if code != ExitUsage || !strings.Contains(emptyOutStderr.String(), "invalid --out") {
		t.Fatalf("expected invalid out usage, got code=%d stderr=%q", code, emptyOutStderr.String())
	}

	fs := NewMemoryFS()
	fs.WriteFile("bad.json", []byte(`{"source":{},"messages":[{}]}`))
	var stdout, stderr bytes.Buffer
	code = NewRunner(fs).Run([]string{"slack", "normalize", "bad.json"}, &stdout, &stderr)
	if code != ExitProcess {
		t.Fatalf("expected process exit, got %d stderr=%s", code, stderr.String())
	}
}

func TestSlackNormalizeRejectsInvalidNormalizedCandidate(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("bad-visibility.json", []byte(`{
	  "source": {"workspace": "synergyai-os", "channel_id": "DSELF", "adapter_id": "slack"},
	  "messages": [
	    {
	      "ts": "1710000000.000001",
	      "user": "U123",
	      "author_name": "Randy",
	      "text": "publish this",
	      "capture_metadata": {
	        "desired_visibility_hint": "public"
	      }
	    }
	  ]
	}`))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"slack", "normalize", "bad-visibility.json"}, &stdout, &stderr)

	if code != ExitProcess {
		t.Fatalf("expected process exit, got %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid desired_visibility") {
		t.Fatalf("expected candidate validation error, got %q", stderr.String())
	}
}

func TestSlackNormalizeWriteFailureReturnsArtifactWrite(t *testing.T) {
	fs := NewMemoryFS()
	fs.WriteFile("slack.json", []byte(slackExportJSON()))
	fs.FailWritesUnder(cleanPath("dry-run"))

	var stdout, stderr bytes.Buffer
	code := NewRunner(fs).Run([]string{"slack", "normalize", "slack.json", "--out", "dry-run"}, &stdout, &stderr)

	if code != ExitArtifactWrite {
		t.Fatalf("expected artifact write exit, got %d stderr=%s", code, stderr.String())
	}
}

func slackExportJSON() string {
	return `{
  "source": {"workspace": "synergyai-os", "channel_id": "DSELF", "channel_name": "self-dm", "adapter_id": "slack"},
  "messages": [
    {"ts": "1710000001.000001", "user": "U123", "author_name": "Randy", "text": "second", "permalink": "https://workspace.slack.com/archives/DSELF/p1710000001000001"},
    {"ts": "1710000000.000001", "user": "U123", "author_name": "Randy", "text": "first", "permalink": "https://workspace.slack.com/archives/DSELF/p1710000000000001"}
  ]
}`
}

func slackUnsafeExportJSON() string {
	return `{
  "source": {"workspace": "synergyai-os", "channel_id": "DSELF", "channel_name": "self-dm", "adapter_id": "slack"},
  "messages": [
    {"ts": "1710000000.000001", "user": "U123", "author_name": "Randy", "text": "password=` + secretValue() + ` ` + botToken() + ` api_key=` + liveKey() + `"},
    {"ts": "1710000001.000001", "user": "U123", "author_name": "Randy", "text": "Review https://example.com/page", "files": [{"id": "F123", "title": "Design PDF", "url_private": "` + privateFileURL() + `", "url_public": "` + publicFileURL() + `"}]}
  ]
}`
}

func secretValue() string {
	return "super-" + "secret-value"
}

func botToken() string {
	return "xoxb-" + "1234567890-abcdef"
}

func liveKey() string {
	return "sk_live" + "_secret"
}

func privateFileURL() string {
	return "https://files.slack.com/files-" + "pri/T/F123/design.pdf"
}

func privateFileMarker() string {
	return "files-" + "pri"
}

func publicFileURL() string {
	return "https://files.example/public/" + "design.pdf"
}
