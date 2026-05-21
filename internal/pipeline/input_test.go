package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseInputAcceptsValidCandidateInput(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "inputs"), 0o755); err != nil {
		t.Fatalf("mkdir inputs: %v", err)
	}
	inputPath := filepath.Join(root, "inputs", "pipeline-text-only.json")
	if err := os.WriteFile(inputPath, []byte(`{
	  "schema_version": "pipeline-input/v0.1",
	  "run_mode": "dry_run",
	  "source": {"kind": "candidate", "path": "../candidates/pipeline-text-only.json"},
	  "method": {"id": "basb-para-code"},
	  "destination": {"id": "tolaria"},
	  "authority_ids": ["DEC-15", "DEC-6", "DEC-12", "DEC-13"]
	}`), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	parsed, err := ParseInputFile(inputPath, ParseOptions{})
	if err != nil {
		t.Fatalf("parse input: %v", err)
	}
	if parsed.RunMode != RunModeDryRun {
		t.Fatalf("expected dry_run, got %q", parsed.RunMode)
	}
	if parsed.Source.Kind != SourceCandidate {
		t.Fatalf("expected candidate source, got %q", parsed.Source.Kind)
	}
	if parsed.BundleRoot != root {
		t.Fatalf("expected bundle root %q, got %q", root, parsed.BundleRoot)
	}
	wantIDs := []string{"DEC-15", "DEC-6", "DEC-12", "DEC-13"}
	if len(parsed.AuthorityIDs) != len(wantIDs) {
		t.Fatalf("expected authority ids %v, got %v", wantIDs, parsed.AuthorityIDs)
	}
	for i := range wantIDs {
		if parsed.AuthorityIDs[i] != wantIDs[i] {
			t.Fatalf("expected authority ids %v, got %v", wantIDs, parsed.AuthorityIDs)
		}
	}
}

func TestParseInputRejectsInvalidAuthorityIDsBeforeOutput(t *testing.T) {
	cases := []struct {
		name string
		ids  string
		want string
	}{
		{"missing", ``, "authority_ids are required"},
		{"emptyList", `"authority_ids": []`, "authority_ids are required"},
		{"emptyID", `"authority_ids": [""]`, "authority_id must not be empty"},
		{"droppedWP6", `"authority_ids": ["WP-6", "DEC-15"]`, "dropped or unauthorized authority_id: WP-6"},
		{"unknown", `"authority_ids": ["DEC-999"]`, "unknown authority_id: DEC-999"},
		{"duplicate", `"authority_ids": ["DEC-15", "DEC-15"]`, "duplicate authority_id: DEC-15"},
		{"malformed", `"authority_ids": ["not an id"]`, "malformed authority_id: not an id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseInputBytes([]byte(validInputWithAuthorityFragment(tc.ids)), "/repo/testdata/pipeline/inputs/pipeline-invalid.json", ParseOptions{})
			if err == nil {
				t.Fatalf("expected error")
			}
			if got := err.Error(); !strings.Contains(got, tc.want) {
				t.Fatalf("expected error containing %q, got %q", tc.want, got)
			}
		})
	}
}

func validInputWithAuthorityFragment(fragment string) string {
	body := `{
	  "schema_version": "pipeline-input/v0.1",
	  "run_mode": "dry_run",
	  "source": {"kind": "candidate", "path": "../candidates/pipeline-text-only.json"},
	  "method": {"id": "basb-para-code"},
	  "destination": {"id": "tolaria"}`
	if fragment != "" {
		body += ",\n	  " + fragment
	}
	return body + "\n	}"
}
