package evalreadback

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDetectsArtifactsAndBlocksPrivateGeneralization(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"corpus_id":                  "corpus-a",
		"source_count":               2,
		"evidence_ready_atom_ratio":  0.5,
		"review_burden_ratio":        0.5,
		"corpus_fingerprint":         "same",
		"command_config_fingerprint": "same-config",
		"guardrails":                 map[string]any{"destination_writes": 0, "hosted_inference_calls": 0},
	})
	writeFixture(t, filepath.Join(root, "link-enrichment", "posthog", "eval-projection.json"), map[string]any{
		"schema_version": "mindline-link-enrichment-eval-projection/v0.1",
		"events": []any{map[string]any{"properties": map[string]any{
			"$ai_evaluation_result":     false,
			"non_generalizable_runtime": true,
			"metadata_only":             true,
			"artifact_match_coverage":   0.5,
		}}},
	})

	out := filepath.Join(root, "out")
	summary, err := Build(root, out, Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ArtifactCount != 2 {
		t.Fatalf("expected 2 artifacts, got %d", summary.ArtifactCount)
	}
	if summary.GeneralizationStatus != "non_generalizable" {
		t.Fatalf("expected non-generalizable, got %s", summary.GeneralizationStatus)
	}
	if summary.TopImprovementTarget.Code != "needs_held_out_labels" {
		t.Fatalf("unexpected target: %+v", summary.TopImprovementTarget)
	}
	chainDraft := readString(t, filepath.Join(out, DirName, "chain-capture-draft.md"))
	if strings.Contains(chainDraft, root) || strings.Contains(chainDraft, "/private/tmp/") {
		t.Fatalf("chain draft leaked root path: %s", chainDraft)
	}
}

func TestBuildComparesBaselineCurrent(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressure(t, baseline, 0.4, 0.7)
	writePressure(t, current, 0.9, 0.2)

	out := filepath.Join(root, "out")
	summary, err := Build(current, out, Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "improved" {
		t.Fatalf("expected improved, got %s comparison=%+v", summary.ImprovementStatus, summary.Comparison)
	}
	if summary.Comparison == nil || summary.Comparison.MetricDeltas["evidence_ready_atom_ratio"] <= 0 {
		t.Fatalf("missing positive delta: %+v", summary.Comparison)
	}
}

func TestBuildCommittedFixtureDetectsMultipleArtifactTypes(t *testing.T) {
	out := t.TempDir()
	summary, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), out, Options{
		BaselineRoot: filepath.Join("..", "..", "testdata", "eval-readback", "baseline"),
	})
	if err != nil {
		t.Fatalf("build committed fixture: %v", err)
	}
	if summary.ArtifactCount < 5 {
		t.Fatalf("expected at least five artifacts, got %d", summary.ArtifactCount)
	}
	if summary.ImprovementStatus != "improved" {
		t.Fatalf("expected fixture improvement, got %s", summary.ImprovementStatus)
	}
	if summary.GeneralizationStatus != "generalizable" {
		t.Fatalf("expected fixture generalizable, got %s", summary.GeneralizationStatus)
	}
}

func TestBuildSupportsActualWriterSchemas(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "trace", "trace-summary.json"), map[string]any{
		"schema_version":             "mindline-trace-summary/v0.1",
		"source_count":               2,
		"processed_source_ratio":     1,
		"review_burden_ratio":        0,
		"command_config_fingerprint": "trace-config",
	})
	writeFixture(t, filepath.Join(root, "corpus-acceptance", "benchmark-summary.json"), map[string]any{
		"schema_version":             "corpus-acceptance-summary/v0.1",
		"source_count":               2,
		"evidence_ready_atom_ratio":  1,
		"review_burden_ratio":        0,
		"corpus_fingerprint":         "acceptance-corpus",
		"command_config_fingerprint": "acceptance-config",
		"held_out":                   true,
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsupported_schema" {
			t.Fatalf("actual writer schema should be supported: %+v", artifact)
		}
	}
	if summary.ArtifactTypeCounts["generic_trace_summary"] != 1 {
		t.Fatalf("expected trace summary detected, got %+v", summary.ArtifactTypeCounts)
	}
	if summary.ArtifactTypeCounts["corpus_acceptance_benchmark"] != 1 {
		t.Fatalf("expected corpus acceptance summary detected, got %+v", summary.ArtifactTypeCounts)
	}
}

func TestBuildCurrentOnlyDoesNotPassImprovementClaim(t *testing.T) {
	out := t.TempDir()
	summary, err := Build(filepath.Join("..", "..", "testdata", "eval-readback", "current"), out, Options{})
	if err != nil {
		t.Fatalf("build committed fixture: %v", err)
	}
	if summary.ImprovementStatus != "not_evaluated" {
		t.Fatalf("current-only run must not claim improvement, got %s", summary.ImprovementStatus)
	}
	var improvementGate ClaimGate
	for _, gate := range summary.ClaimGates {
		if gate.Gate == "improvement_claim" {
			improvementGate = gate
			break
		}
	}
	if improvementGate.Status != "blocked" || !containsString(improvementGate.ReasonCodes, "missing_baseline") {
		t.Fatalf("expected missing_baseline blocked improvement gate, got %+v", improvementGate)
	}
}

func TestBuildUnsafeArtifactBlocksImprovementClaim(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressure(t, baseline, 0.4, 0.7)
	writePressure(t, current, 0.9, 0.2)
	writeRaw(t, filepath.Join(current, "link-enrichment", "loop-summary.json"), `{"schema_version":"link-enrichment-loop-summary/v0.1","input_path":"/private/tmp/source.json"}`)

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "improved" {
		t.Fatalf("expected metrics to remain improved, got %s", summary.ImprovementStatus)
	}
	if gateStatus(summary, "privacy_safe_readback") != "fail" {
		t.Fatalf("expected privacy gate to fail: %+v", summary.ClaimGates)
	}
	if gateStatus(summary, "improvement_claim") != "blocked" {
		t.Fatalf("expected improvement claim blocked by unsafe evidence: %+v", summary.ClaimGates)
	}
}

func TestBuildGuardrailRegressionBlocksImprovementClaim(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressureWithGuardrails(t, baseline, 0.4, 0.7, map[string]any{"destination_writes": 0, "hosted_inference_calls": 0})
	writePressureWithGuardrails(t, current, 0.9, 0.2, map[string]any{"destination_writes": 1, "hosted_inference_calls": 0})

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "regressed" {
		t.Fatalf("expected guardrail regression to dominate quality improvement, got %s comparison=%+v", summary.ImprovementStatus, summary.Comparison)
	}
	if gateStatus(summary, "improvement_claim") != "fail" {
		t.Fatalf("expected improvement claim fail on guardrail regression: %+v", summary.ClaimGates)
	}
	if summary.Comparison == nil || !containsString(summary.Comparison.ReasonCodes, "guardrail_regression") {
		t.Fatalf("expected guardrail_regression reason, got %+v", summary.Comparison)
	}
}

func TestBuildRecordsUnsupportedSchemaWithoutMetrics(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":            "corpus-pressure-summary/v9.9",
		"evidence_ready_atom_ratio": 1,
		"corpus_fingerprint":        "should-not-copy",
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(summary.Artifacts) != 1 {
		t.Fatalf("expected one artifact, got %d", len(summary.Artifacts))
	}
	artifact := summary.Artifacts[0]
	if artifact.Status != "unsupported_schema" || !containsString(artifact.ReasonCodes, "unsupported_schema_version") {
		t.Fatalf("expected unsupported schema artifact, got %+v", artifact)
	}
	if len(artifact.Metrics) != 0 || len(artifact.Fingerprints) != 0 {
		t.Fatalf("unsupported schema should not extract metrics/fingerprints, got %+v", artifact)
	}
}

func TestBuildBlocksNotComparableFingerprint(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressureWithFingerprint(t, baseline, 0.4, 0.7, "a")
	writePressureWithFingerprint(t, current, 0.9, 0.2, "b")

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "not_comparable" {
		t.Fatalf("expected not_comparable, got %s", summary.ImprovementStatus)
	}
}

func TestBuildBlocksOneSidedFingerprint(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressureWithFingerprint(t, baseline, 0.4, 0.7, "same")
	writePressureWithoutFingerprint(t, current, 0.9, 0.2)

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "not_comparable" {
		t.Fatalf("expected not_comparable, got %s", summary.ImprovementStatus)
	}
	if summary.Comparison == nil || !containsString(summary.Comparison.ReasonCodes, "one_sided_fingerprint") {
		t.Fatalf("expected one_sided_fingerprint reason, got %+v", summary.Comparison)
	}
}

func TestBuildBlocksMixedFingerprintlessArtifactDomains(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeFixture(t, filepath.Join(baseline, "trace", "trace-summary.json"), map[string]any{
		"schema_version":            "generic-trace-summary/v0.1",
		"evidence_ready_atom_ratio": 0.2,
	})
	writeFixture(t, filepath.Join(current, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":            "corpus-pressure-summary/v0.1",
		"evidence_ready_atom_ratio": 0.9,
	})

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "not_comparable" {
		t.Fatalf("expected not_comparable, got %s", summary.ImprovementStatus)
	}
	if summary.Comparison == nil || !containsString(summary.Comparison.ReasonCodes, "artifact_domain_mismatch") {
		t.Fatalf("expected artifact_domain_mismatch reason, got %+v", summary.Comparison)
	}
}

func TestBuildRejectsSymlinkEscapeOutput(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "current")
	writePressure(t, current, 0.9, 0.2)
	out := filepath.Join(root, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}
	escaped := filepath.Join(root, "escaped")
	if err := os.MkdirAll(escaped, 0o755); err != nil {
		t.Fatalf("mkdir escaped: %v", err)
	}
	if err := os.Symlink(escaped, filepath.Join(out, DirName)); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := Build(current, out, Options{ProtectedRoots: []string{escaped}})
	if err == nil || !strings.Contains(err.Error(), "escapes output root") {
		t.Fatalf("expected symlink escape rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(escaped, "readback-summary.json")); !os.IsNotExist(err) {
		t.Fatalf("escaped output should not be written, err=%v", err)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestBuildRejectsNoArtifacts(t *testing.T) {
	root := t.TempDir()
	_, err := Build(root, filepath.Join(root, "out"), Options{})
	if err == nil || !strings.Contains(err.Error(), "no supported eval/trace artifacts") {
		t.Fatalf("expected no artifacts error, got %v", err)
	}
}

func writePressure(t *testing.T, root string, evidenceReady, reviewBurden float64) {
	t.Helper()
	writePressureWithFingerprint(t, root, evidenceReady, reviewBurden, "same")
}

func writePressureWithFingerprint(t *testing.T, root string, evidenceReady, reviewBurden float64, fingerprint string) {
	t.Helper()
	writeFixture(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"corpus_id":                  "corpus-a",
		"source_count":               2,
		"evidence_ready_atom_ratio":  evidenceReady,
		"review_burden_ratio":        reviewBurden,
		"corpus_fingerprint":         fingerprint,
		"command_config_fingerprint": "same-config",
		"guardrails":                 map[string]any{"destination_writes": 0, "hosted_inference_calls": 0},
	})
}

func writePressureWithGuardrails(t *testing.T, root string, evidenceReady, reviewBurden float64, guardrails map[string]any) {
	t.Helper()
	writeFixture(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"corpus_id":                  "corpus-a",
		"source_count":               2,
		"evidence_ready_atom_ratio":  evidenceReady,
		"review_burden_ratio":        reviewBurden,
		"corpus_fingerprint":         "same",
		"command_config_fingerprint": "same-config",
		"guardrails":                 guardrails,
	})
}

func writePressureWithoutFingerprint(t *testing.T, root string, evidenceReady, reviewBurden float64) {
	t.Helper()
	writeFixture(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":            "corpus-pressure-summary/v0.1",
		"corpus_id":                 "corpus-a",
		"source_count":              2,
		"evidence_ready_atom_ratio": evidenceReady,
		"review_burden_ratio":       reviewBurden,
		"guardrails":                map[string]any{"destination_writes": 0, "hosted_inference_calls": 0},
	})
}

func writeFixture(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func writeRaw(t *testing.T, path string, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(value+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func gateStatus(summary Summary, gateName string) string {
	for _, gate := range summary.ClaimGates {
		if gate.Gate == gateName {
			return gate.Status
		}
	}
	return ""
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}
