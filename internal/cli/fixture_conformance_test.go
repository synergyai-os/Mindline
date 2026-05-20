package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureManifest struct {
	Cases []fixtureCase `json:"cases"`
}

type fixtureCase struct {
	File             string   `json:"file"`
	Valid            bool     `json:"valid"`
	ExitCode         int      `json:"exit_code"`
	ExpectedState    string   `json:"expected_state"`
	ArtifactCount    int      `json:"artifact_count"`
	StderrContains   string   `json:"stderr_contains"`
	Assertions       []string `json:"assertions"`
	PrivateValues    []string `json:"private_values"`
	ExpectedBodyText []string `json:"expected_body_text"`
}

func TestCandidateFixtureConformance(t *testing.T) {
	root := repoRoot(t)
	assertContractDocsExist(t, root)
	manifest := loadFixtureManifest(t, root)
	if len(manifest.Cases) == 0 {
		t.Fatalf("expected fixture manifest cases")
	}

	seen := map[string]bool{}
	for _, tc := range manifest.Cases {
		t.Run(tc.File, func(t *testing.T) {
			if seen[tc.File] {
				t.Fatalf("duplicate fixture %q", tc.File)
			}
			seen[tc.File] = true

			path := filepath.Join(root, "examples", "candidates", tc.File)
			input, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var raw any
			if err := json.Unmarshal(input, &raw); err != nil {
				t.Fatalf("fixture is not valid JSON: %v", err)
			}

			var stdout, stderr bytes.Buffer
			code := NewRunner(NewOSFileSystem()).Run([]string{"process", path}, &stdout, &stderr)
			if code != tc.ExitCode {
				t.Fatalf("expected exit %d, got %d stderr=%s stdout=%s", tc.ExitCode, code, stderr.String(), stdout.String())
			}
			if tc.StderrContains != "" && !strings.Contains(stderr.String(), tc.StderrContains) {
				t.Fatalf("expected stderr to contain %q, got %q", tc.StderrContains, stderr.String())
			}
			if !tc.Valid {
				if !strings.Contains(stderr.String(), "process candidate:") {
					t.Fatalf("expected invalid fixture stderr to contain process candidate prefix, got %q", stderr.String())
				}
				if stdout.String() != "" {
					t.Fatalf("expected invalid fixture to emit no stdout, got %q", stdout.String())
				}
				return
			}
			if stderr.String() != "" {
				t.Fatalf("expected empty stderr, got %q", stderr.String())
			}

			var envelope ResultEnvelope
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("decode stdout envelope: %v\n%s", err, stdout.String())
			}
			if envelope.State != tc.ExpectedState {
				t.Fatalf("expected state %q, got %q", tc.ExpectedState, envelope.State)
			}
			if envelope.ArtifactCount != tc.ArtifactCount {
				t.Fatalf("expected artifact count %d, got %d", tc.ArtifactCount, envelope.ArtifactCount)
			}
			assertFixtureAssertions(t, root, tc, stdout.String(), envelope)
		})
	}
}

func TestCandidateFixtureStdoutIsDeterministic(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "examples", "candidates", "publish-ready.json")

	var firstOut, firstErr, secondOut, secondErr bytes.Buffer
	firstCode := NewRunner(NewOSFileSystem()).Run([]string{"process", path}, &firstOut, &firstErr)
	secondCode := NewRunner(NewOSFileSystem()).Run([]string{"process", path}, &secondOut, &secondErr)

	if firstCode != ExitOK || secondCode != ExitOK {
		t.Fatalf("expected both runs to pass, got %d/%d stderr=%s%s", firstCode, secondCode, firstErr.String(), secondErr.String())
	}
	if firstOut.String() != secondOut.String() {
		t.Fatalf("expected deterministic stdout\n--- first ---\n%s\n--- second ---\n%s", firstOut.String(), secondOut.String())
	}
}

func assertContractDocsExist(t *testing.T, root string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "docs", "candidate-contract.md"))
	if err != nil {
		t.Fatalf("read candidate contract docs: %v", err)
	}
	for _, required := range []string{"schema_version", "provenance", "desired_visibility", "Candidates are not destination instructions", "go test -count=1 ./..."} {
		if !strings.Contains(string(data), required) {
			t.Fatalf("candidate contract docs missing %q", required)
		}
	}
}

func loadFixtureManifest(t *testing.T, root string) fixtureManifest {
	t.Helper()
	path := filepath.Join(root, "examples", "candidates", "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest fixtureManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return manifest
}

func assertFixtureAssertions(t *testing.T, root string, tc fixtureCase, stdout string, envelope ResultEnvelope) {
	t.Helper()
	for _, value := range tc.PrivateValues {
		if strings.Contains(stdout, value) {
			t.Fatalf("stdout leaked private value %q:\n%s", value, stdout)
		}
	}
	for _, text := range tc.ExpectedBodyText {
		if !strings.Contains(stdout, text) {
			t.Fatalf("stdout missing expected text %q:\n%s", text, stdout)
		}
	}
	for _, assertion := range tc.Assertions {
		switch assertion {
		case "dry_run_publish_artifact":
			if len(envelope.Artifacts) != 1 || envelope.Artifacts[0].Kind != string("dry_run_publish") {
				t.Fatalf("expected one dry_run_publish artifact, got %#v", envelope.Artifacts)
			}
		case "attention_preview_artifact":
			if len(envelope.Artifacts) != 1 || envelope.Artifacts[0].Kind != string("attention_preview") {
				t.Fatalf("expected one attention_preview artifact, got %#v", envelope.Artifacts)
			}
		case "no_publish_markdown":
			if strings.Contains(stdout, "## Snapshot") {
				t.Fatalf("expected no publish markdown in stdout:\n%s", stdout)
			}
		case "no_artifact_with_out":
			assertNoArtifactWithOut(t, root, tc.File)
		case "private_values_absent_with_out":
			assertPrivateValuesAbsentWithOut(t, root, tc)
		case "path_contained_with_out":
			assertPathContainedWithOut(t, root, tc.File)
		default:
			t.Fatalf("unknown fixture assertion %q", assertion)
		}
	}
}

func assertNoArtifactWithOut(t *testing.T, root, file string) {
	t.Helper()
	outDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"process", filepath.Join(root, "examples", "candidates", file), "--out", outDir}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("process with --out: code=%d stderr=%s", code, stderr.String())
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no artifact files, got %d", len(entries))
	}
}

func assertPrivateValuesAbsentWithOut(t *testing.T, root string, tc fixtureCase) {
	t.Helper()
	outDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"process", filepath.Join(root, "examples", "candidates", tc.File), "--out", outDir}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("process with --out: code=%d stderr=%s", code, stderr.String())
	}
	for _, value := range tc.PrivateValues {
		if strings.Contains(stdout.String(), value) {
			t.Fatalf("--out stdout leaked private value %q:\n%s", value, stdout.String())
		}
	}
}

func assertPathContainedWithOut(t *testing.T, root, file string) {
	t.Helper()
	outDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := NewRunner(NewOSFileSystem()).Run([]string{"process", filepath.Join(root, "examples", "candidates", file), "--out", outDir}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("process with --out: code=%d stderr=%s", code, stderr.String())
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one artifact file, got %d", len(entries))
	}
	artifactPath := filepath.Join(outDir, entries[0].Name())
	if filepath.Dir(artifactPath) != outDir {
		t.Fatalf("artifact escaped output dir: %s", artifactPath)
	}
	if strings.Contains(entries[0].Name(), "..") || strings.ContainsAny(entries[0].Name(), `/\`) {
		t.Fatalf("artifact filename is not sanitized: %q", entries[0].Name())
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatalf("could not find repo root from cwd")
		}
		wd = parent
	}
}
