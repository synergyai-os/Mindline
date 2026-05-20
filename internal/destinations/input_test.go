package destinations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDestinationInput(t *testing.T) {
	input := `{
		"schema_version": "destination-input/v0.1",
		"result": {
			"state": "dry_run_published",
			"record_id": "record-publish",
			"source_candidate_id": "candidate-publish",
			"idempotency_key": "slack:publish",
			"authority_ids": ["WP-5", "DEC-12"],
			"artifacts": [{"kind": "dry_run_publish", "body": "## Snapshot\nUseful body"}],
			"safety": {"private_provenance": false, "redaction_required": false, "secret_like": false}
		}
	}`

	result, err := ParseDestinationInput([]byte(input), ParseOptions{})
	if err != nil {
		t.Fatalf("parse inline input: %v", err)
	}
	if result.State != "dry_run_published" || result.RecordID != "record-publish" || result.SourceCandidateID != "candidate-publish" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Body != "## Snapshot\nUseful body" {
		t.Fatalf("unexpected artifacts: %#v", result.Artifacts)
	}
}

func TestParseDestinationInputDecodesSafetyFlags(t *testing.T) {
	input := strings.Replace(destinationInputWithArtifactBody("artifact body"), `"private_provenance": false`, `"private_provenance": true`, 1)
	input = strings.Replace(input, `"redaction_required": false`, `"redaction_required": true`, 1)
	input = strings.Replace(input, `"secret_like": false`, `"secret_like": true`, 1)

	result, err := ParseDestinationInput([]byte(input), ParseOptions{})
	if err != nil {
		t.Fatalf("parse input: %v", err)
	}
	if !result.Safety.PrivateProvenance || !result.Safety.RedactionRequired || !result.Safety.SecretLike {
		t.Fatalf("expected safety flags to decode, got %+v", result.Safety)
	}
}

func TestParseDestinationInputArtifactPathReferences(t *testing.T) {
	base := t.TempDir()
	artifactPath := filepath.Join(base, "artifact.md")
	if err := os.WriteFile(artifactPath, []byte("artifact body"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	input := destinationInputWithArtifactPath("artifact.md")

	result, err := ParseDestinationInput([]byte(input), ParseOptions{BaseDir: base})
	if err != nil {
		t.Fatalf("parse path input: %v", err)
	}
	if result.Artifacts[0].Body != "artifact body" {
		t.Fatalf("unexpected artifact body %q", result.Artifacts[0].Body)
	}
}

func TestParseDestinationInputRejectsUnsafeArtifactPathReferences(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}

	cases := []struct {
		name  string
		input string
		opts  ParseOptions
		want  string
	}{
		{name: "empty base", input: destinationInputWithArtifactPath("artifact.md"), opts: ParseOptions{}, want: "base_dir"},
		{name: "outside relative", input: destinationInputWithArtifactPath("../" + filepath.Base(outside) + "/secret.md"), opts: ParseOptions{BaseDir: base}, want: "outside allowed roots"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDestinationInput([]byte(tc.input), tc.opts)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestParseDestinationInputRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := ParseDestinationInput([]byte(destinationInputWithArtifactPath("linked/secret.md")), ParseOptions{BaseDir: base})
	if err == nil {
		t.Fatalf("expected symlink escape rejection")
	}
	if !strings.Contains(err.Error(), "outside allowed roots") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseDestinationInputRejectsInvalidEnvelope(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "unsupported schema", input: strings.Replace(destinationInputWithArtifactBody("body"), "destination-input/v0.1", "destination-input/v9", 1), want: "schema_version"},
		{name: "missing state", input: strings.Replace(destinationInputWithArtifactBody("body"), `"state": "dry_run_published",`, `"state": "",`, 1), want: "state"},
		{name: "missing record", input: strings.Replace(destinationInputWithArtifactBody("body"), `"record_id": "record-publish",`, `"record_id": "",`, 1), want: "record_id"},
		{name: "missing candidate", input: strings.Replace(destinationInputWithArtifactBody("body"), `"source_candidate_id": "candidate-publish",`, `"source_candidate_id": "",`, 1), want: "source_candidate_id"},
		{name: "missing authority", input: strings.Replace(destinationInputWithArtifactBody("body"), `"authority_ids": ["WP-5", "DEC-12"],`, `"authority_ids": [],`, 1), want: "authority_ids"},
		{name: "missing artifact body and path", input: strings.Replace(destinationInputWithArtifactBody("body"), `"body": "body"`, `"body": ""`, 1), want: "artifact"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDestinationInput([]byte(tc.input), ParseOptions{})
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestParseProcessResultEnvelopeRequiresDestinationFields(t *testing.T) {
	input := `{
  "state": "dry_run_published",
  "record_id": "candidate-publish",
  "artifact_count": 1,
  "artifacts": [{"kind": "dry_run_publish", "body": "body"}],
  "authority_ids": ["WP-5", "DEC-12"]
}`
	_, err := ParseDestinationInput([]byte(input), ParseOptions{})
	if err == nil {
		t.Fatalf("expected legacy process result to be rejected")
	}
	if !strings.Contains(err.Error(), "source_candidate_id") {
		t.Fatalf("expected source_candidate_id error, got %v", err)
	}
}

func destinationInputWithArtifactPath(path string) string {
	return `{
		"schema_version": "destination-input/v0.1",
		"result": {
			"state": "dry_run_published",
			"record_id": "record-publish",
			"source_candidate_id": "candidate-publish",
			"idempotency_key": "slack:publish",
			"authority_ids": ["WP-5", "DEC-12"],
			"artifacts": [{"kind": "dry_run_publish", "path": "` + filepath.ToSlash(path) + `"}],
			"safety": {"private_provenance": false, "redaction_required": false, "secret_like": false}
		}
	}`
}

func destinationInputWithArtifactBody(body string) string {
	return `{
		"schema_version": "destination-input/v0.1",
		"result": {
			"state": "dry_run_published",
			"record_id": "record-publish",
			"source_candidate_id": "candidate-publish",
			"idempotency_key": "slack:publish",
			"authority_ids": ["WP-5", "DEC-12"],
			"artifacts": [{"kind": "dry_run_publish", "body": "` + body + `"}],
			"safety": {"private_provenance": false, "redaction_required": false, "secret_like": false}
		}
	}`
}
