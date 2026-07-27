package recalleval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureManifestValidatesAndFingerprints(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "agent-eval", "manifest-v0.1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("ValidateManifest: %v", err)
	}
	fingerprint, err := ManifestFingerprint(manifest)
	if err != nil || fingerprint != manifest.Fingerprint {
		t.Fatalf("fingerprint = %q, %v", fingerprint, err)
	}
}

func TestCompareAppliesHeldOutThresholds(t *testing.T) {
	manifest := testManifest(t)
	fingerprint, err := ManifestFingerprint(manifest)
	if err != nil {
		t.Fatal(err)
	}
	baseline := matchingEvaluation(manifest, fingerprint, "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	candidate := matchingEvaluation(manifest, fingerprint, "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	result, err := Compare(manifest, baseline, candidate)
	if err != nil || !result.Passed {
		t.Fatalf("Compare = %+v, %v", result, err)
	}
	candidate.Cases[12].Citations = []Citation{{RecordFingerprint: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", Valid: true}}
	result, err = Compare(manifest, baseline, candidate)
	if err != nil || result.Passed || !contains(result.ReasonCodes, "no_answer_false_positive") {
		t.Fatalf("expected no-answer rejection, got %+v, %v", result, err)
	}
}

func TestScoreRejectsManifestBindingDrift(t *testing.T) {
	manifest := testManifest(t)
	fingerprint, err := ManifestFingerprint(manifest)
	if err != nil {
		t.Fatal(err)
	}
	evaluation := matchingEvaluation(manifest, fingerprint, "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	evaluation.LibraryFingerprint = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if _, err := Score(manifest, evaluation); err == nil {
		t.Fatal("Score accepted library binding drift")
	}
}

func TestCompareRejectsBaselineOrCompactBindingDrift(t *testing.T) {
	manifest := testManifest(t)
	fingerprint, err := ManifestFingerprint(manifest)
	if err != nil {
		t.Fatal(err)
	}
	baseline := matchingEvaluation(manifest, fingerprint, "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	candidate := matchingEvaluation(manifest, fingerprint, "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	baseline.BuildFingerprint = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if _, err := Compare(manifest, baseline, candidate); err == nil {
		t.Fatal("Compare accepted a baseline build that differs from the manifest")
	}

	baseline = matchingEvaluation(manifest, fingerprint, manifest.BaselineBuild)
	candidate.UnselectedHydratedContent = true
	result, err := Compare(manifest, baseline, candidate)
	if err != nil || result.Passed || !contains(result.ReasonCodes, "compact_output_contains_unselected_hydrated_content") {
		t.Fatalf("expected compact hydration rejection, got %+v, %v", result, err)
	}
}

func testManifest(t *testing.T) Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "agent-eval", "manifest-v0.1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Fingerprint = ""
	return manifest
}

func matchingEvaluation(manifest Manifest, fingerprint, build string) Evaluation {
	result := Evaluation{SchemaVersion: ResultSchemaVersion, LibraryFingerprint: manifest.LibraryFingerprint, ManifestFingerprint: fingerprint, BuildFingerprint: build}
	for _, item := range manifest.Cases {
		caseResult := CaseResult{CaseID: item.CaseID}
		if item.Kind == CaseAnswerable {
			caseResult.Citations = []Citation{{RecordFingerprint: item.ExpectedRecordFingerprints[0], Valid: true}}
		}
		result.Cases = append(result.Cases, caseResult)
	}
	return result
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
