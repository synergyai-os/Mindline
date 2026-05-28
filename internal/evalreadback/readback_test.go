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

func TestBuildRemovesStaleComparisonWhenBaselineOmitted(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	out := filepath.Join(root, "out")
	writePressure(t, baseline, 0.4, 0.7)
	writePressure(t, current, 0.9, 0.2)

	if _, err := Build(current, out, Options{BaselineRoot: baseline}); err != nil {
		t.Fatalf("baseline build: %v", err)
	}
	comparisonPath := filepath.Join(out, DirName, "comparison-summary.json")
	if _, err := os.Stat(comparisonPath); err != nil {
		t.Fatalf("expected comparison output: %v", err)
	}

	summary, err := Build(current, out, Options{})
	if err != nil {
		t.Fatalf("current-only build: %v", err)
	}
	if summary.Comparison != nil {
		t.Fatalf("expected no comparison without baseline, got %+v", summary.Comparison)
	}
	if _, err := os.Stat(comparisonPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale comparison output removed, err=%v", err)
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

func TestBuildAcceptsCorpusPressureArtifactDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "corpus-pressure")
	writeFixture(t, filepath.Join(root, "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"source_count":               2,
		"evidence_ready_atom_ratio":  1,
		"review_burden_ratio":        0,
		"corpus_fingerprint":         "artifact-root-corpus",
		"command_config_fingerprint": "artifact-root-config",
		"guardrails":                 completeGuardrails(),
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build artifact directory: %v", err)
	}
	if summary.ArtifactCount != 1 {
		t.Fatalf("expected one artifact, got %d", summary.ArtifactCount)
	}
	if summary.ArtifactTypeCounts["corpus_pressure_summary"] != 1 {
		t.Fatalf("expected corpus pressure summary detected, got %+v", summary.ArtifactTypeCounts)
	}
}

func TestBuildHonorsHeldOutArtifactOutsideTestdata(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "corpus-acceptance", "benchmark-summary.json"), map[string]any{
		"schema_version":             "corpus-acceptance-summary/v0.1",
		"source_count":               2,
		"evidence_ready_atom_ratio":  1,
		"review_burden_ratio":        0,
		"corpus_fingerprint":         "held-out-corpus",
		"command_config_fingerprint": "held-out-config",
		"held_out":                   true,
		"guardrails":                 map[string]any{"destination_writes": 0, "hosted_telemetry_exports": 0, "hosted_inference_calls": 0},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.SampleStatus != "held_out" {
		t.Fatalf("expected held_out sample status, got %s", summary.SampleStatus)
	}
	if summary.GeneralizationStatus != "generalizable" {
		t.Fatalf("expected held-out artifact to support bounded generalization, got %s", summary.GeneralizationStatus)
	}
	if gateStatus(summary, "generalization_claim") != "pass" {
		t.Fatalf("expected generalization claim pass, got %+v", summary.ClaimGates)
	}
}

func TestBuildAcceptsCorpusAcceptanceGuardrailsAsCompleteSideEffectEvidence(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "corpus-acceptance", "benchmark-summary.json"), map[string]any{
		"schema_version":             "corpus-acceptance-summary/v0.1",
		"suite_kind":                 "held_out",
		"corpus_fingerprint":         "held-out-corpus",
		"command_config_fingerprint": "held-out-config",
		"threshold":                  0.98,
		"accuracy":                   1.0,
		"eval_count":                 50,
		"held_out":                   true,
		"suite_valid":                true,
		"dec64_eligible":             true,
		"guardrails":                 map[string]any{"destination_writes": 0, "hosted_telemetry_exports": 0, "hosted_inference_calls": 0},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gateStatus(summary, "side_effect_claim") != "pass" {
		t.Fatalf("expected corpus acceptance guardrails to complete side-effect evidence, got %+v", summary.ClaimGates)
	}
	if gateStatus(summary, "dec64_no_human_claim") != "pass" {
		t.Fatalf("expected corpus acceptance threshold proof to pass DEC-64 gate, got %+v", summary.ClaimGates)
	}
}

func TestBuildBlocksDEC64ClaimWhenReadbackEvidenceUnsafe(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "corpus-acceptance", "benchmark-summary.json"), map[string]any{
		"schema_version":             "corpus-acceptance-summary/v0.1",
		"suite_kind":                 "held_out",
		"corpus_fingerprint":         "held-out-corpus",
		"command_config_fingerprint": "held-out-config",
		"threshold":                  0.98,
		"accuracy":                   1.0,
		"eval_count":                 50,
		"held_out":                   true,
		"suite_valid":                true,
		"dec64_eligible":             true,
		"guardrails":                 map[string]any{"destination_writes": 0, "hosted_telemetry_exports": 0, "hosted_inference_calls": 0},
	})
	writeRaw(t, filepath.Join(root, "trace", "trace-summary.json"), `{"schema_version":"mindline-trace-summary/v0.1","input_path":"/private/tmp/source.json"}`)

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gateStatus(summary, "privacy_safe_readback") != "fail" {
		t.Fatalf("expected privacy gate to fail: %+v", summary.ClaimGates)
	}
	if gateStatus(summary, "side_effect_claim") != "pass" {
		t.Fatalf("expected side-effect evidence to remain complete: %+v", summary.ClaimGates)
	}
	if gateStatus(summary, "dec64_no_human_claim") == "pass" {
		t.Fatalf("expected unsafe evidence to block DEC-64 claim: %+v", summary.ClaimGates)
	}
	if gate := gateByName(summary, "dec64_no_human_claim"); !containsString(gate.ReasonCodes, "unsafe_or_leaky") {
		t.Fatalf("expected unsafe DEC-64 block reason, got %+v", gate)
	}
}

func TestBuildAcceptsCorpusPressureGuardrailsAsCompleteSideEffectEvidence(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"corpus_id":                  "corpus-a",
		"source_count":               2,
		"evidence_ready_atom_ratio":  1,
		"review_burden_ratio":        0,
		"corpus_fingerprint":         "corpus-a",
		"command_config_fingerprint": "config-a",
		"guardrails":                 map[string]any{"destination_writes": 0, "hosted_telemetry_exports": 0, "hosted_inference_calls": 0},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gateStatus(summary, "side_effect_claim") != "pass" {
		t.Fatalf("expected corpus pressure guardrails to complete side-effect evidence, got %+v", summary.ClaimGates)
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

func TestBuildSanitizesPrivateArtifactRefs(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "Users", "randy", "slack.com", "archives", "C123", "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"corpus_id":                  "corpus-a",
		"source_count":               2,
		"evidence_ready_atom_ratio":  0.9,
		"review_burden_ratio":        0.1,
		"corpus_fingerprint":         "same",
		"command_config_fingerprint": "same-config",
		"guardrails":                 completeGuardrails(),
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gateStatus(summary, "privacy_safe_readback") != "fail" {
		t.Fatalf("expected private ref to fail privacy gate: %+v", summary.ClaimGates)
	}
	serialized, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if strings.Contains(string(serialized), "Users/") || strings.Contains(string(serialized), "slack.com/archives") {
		t.Fatalf("summary leaked private artifact ref: %s", serialized)
	}
	chainDraft := readString(t, filepath.Join(root, "out", DirName, "chain-capture-draft.md"))
	if strings.Contains(chainDraft, "Users/") || strings.Contains(chainDraft, "slack.com/archives") {
		t.Fatalf("chain draft leaked private artifact ref: %s", chainDraft)
	}
}

func TestBuildBaselineUnsafeArtifactBlocksProofClaims(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressure(t, baseline, 0.4, 0.7)
	writePressure(t, current, 0.9, 0.2)
	writeRaw(t, filepath.Join(baseline, "link-enrichment", "loop-summary.json"), `{"schema_version":"link-enrichment-loop-summary/v0.1","input_path":"/private/tmp/source.json"}`)

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "improved" {
		t.Fatalf("expected metrics to remain improved, got %s", summary.ImprovementStatus)
	}
	if gateStatus(summary, "privacy_safe_readback") != "fail" {
		t.Fatalf("expected baseline unsafe artifact to fail privacy gate: %+v", summary.ClaimGates)
	}
	if gateStatus(summary, "improvement_claim") != "blocked" {
		t.Fatalf("expected baseline unsafe artifact to block improvement claim: %+v", summary.ClaimGates)
	}
	if len(summary.BaselineArtifacts) == 0 {
		t.Fatalf("expected baseline artifacts to be retained as proof evidence")
	}
}

func TestBuildBaselineUnsupportedArtifactBlocksImprovementClaim(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressure(t, baseline, 0.4, 0.7)
	writePressure(t, current, 0.9, 0.2)
	writeFixture(t, filepath.Join(baseline, "link-enrichment", "loop-summary.json"), map[string]any{
		"schema_version": "link-enrichment-loop-summary/v9",
	})

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "improved" {
		t.Fatalf("expected metrics to remain improved, got %s", summary.ImprovementStatus)
	}
	if gateStatus(summary, "privacy_safe_readback") != "pass" {
		t.Fatalf("expected unsupported baseline artifact not to fail privacy gate: %+v", summary.ClaimGates)
	}
	if gateStatus(summary, "improvement_claim") != "blocked" {
		t.Fatalf("expected baseline unsupported artifact to block improvement claim: %+v", summary.ClaimGates)
	}
	if !containsString(gateByName(summary, "improvement_claim").ReasonCodes, "unsupported_schema") {
		t.Fatalf("expected unsupported_schema reason: %+v", summary.ClaimGates)
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

func TestBuildDoesNotDoubleCountDuplicatedCorpusPressureGuardrails(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	guardrails := completeGuardrails()
	guardrails["hosted_inference_calls"] = 1
	writePressureWithGuardrails(t, baseline, 0.4, 0.7, guardrails)
	for _, fixture := range []struct {
		path          string
		schemaVersion string
	}{
		{path: "corpus-pressure/pressure-summary.json", schemaVersion: "corpus-pressure-summary/v0.1"},
		{path: "corpus-pressure/eval-input.json", schemaVersion: "corpus-pressure-eval-input/v0.1"},
		{path: "corpus-pressure/trace-summary.json", schemaVersion: "corpus-pressure-trace-summary/v0.1"},
	} {
		writeFixture(t, filepath.Join(current, fixture.path), map[string]any{
			"schema_version":             fixture.schemaVersion,
			"corpus_id":                  "corpus-a",
			"source_count":               2,
			"evidence_ready_atom_ratio":  0.9,
			"review_burden_ratio":        0.2,
			"corpus_fingerprint":         "same",
			"command_config_fingerprint": "same-config",
			"guardrails":                 guardrails,
		})
	}

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.Guardrails.HostedInferenceCalls != 1 {
		t.Fatalf("expected duplicated run-level guardrail to be counted once, got %+v", summary.Guardrails)
	}
	if summary.ImprovementStatus != "improved" {
		t.Fatalf("expected duplicated guardrail artifacts not to create regression, got %s comparison=%+v", summary.ImprovementStatus, summary.Comparison)
	}
	if summary.Comparison != nil && containsString(summary.Comparison.ReasonCodes, "guardrail_regression") {
		t.Fatalf("expected no guardrail_regression from duplicated guardrails, got %+v", summary.Comparison)
	}
}

func TestBuildHostedTelemetryBlocksSideEffectClaim(t *testing.T) {
	root := t.TempDir()
	writePressureWithGuardrails(t, root, 0.9, 0.2, map[string]any{"destination_writes": 0, "hosted_telemetry_exports": 1, "hosted_inference_calls": 0})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.Guardrails.HostedTelemetryExports != 1 {
		t.Fatalf("expected hosted telemetry counter, got %+v", summary.Guardrails)
	}
	if gateStatus(summary, "side_effect_claim") != "fail" {
		t.Fatalf("expected side effect claim to fail on hosted telemetry, got %+v", summary.ClaimGates)
	}
}

func TestBuildBrowserSlackGuardrailsBlockSideEffectClaim(t *testing.T) {
	root := t.TempDir()
	guardrails := completeGuardrails()
	guardrails["browser_calls"] = 1
	guardrails["slack_api_calls"] = 1
	writeFixture(t, filepath.Join(root, "link-enrichment", "loop-summary.json"), map[string]any{
		"schema_version": "link-enrichment-loop-summary/v0.1",
		"guardrails":     guardrails,
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := summary.Artifacts[0].Metrics["guardrail_browser_calls"]; got != 1 {
		t.Fatalf("expected browser guardrail metric, got metrics=%+v", summary.Artifacts[0].Metrics)
	}
	if got := summary.Artifacts[0].Metrics["guardrail_slack_api_calls"]; got != 1 {
		t.Fatalf("expected Slack guardrail metric, got metrics=%+v", summary.Artifacts[0].Metrics)
	}
	if gateStatus(summary, "side_effect_claim") != "fail" {
		t.Fatalf("expected side effect claim to fail on browser/Slack calls, got %+v", summary.ClaimGates)
	}
}

func TestBuildReadsPostHogSafetyNoHumanFlag(t *testing.T) {
	root := t.TempDir()
	writePostHogLinkEnrichmentSafetyEvent(t, root, true)

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.Guardrails.NoHumanClaims {
		t.Fatalf("expected PostHog projection flag not to count as no-human claim guardrail, got %+v", summary.Guardrails)
	}
	if !summary.Artifacts[0].Flags["safety_no_human_claims"] {
		t.Fatalf("expected PostHog projection flag to remain available as evidence, got %+v", summary.Artifacts[0].Flags)
	}
	if gateStatus(summary, "side_effect_claim") != "pass" {
		t.Fatalf("expected side effect claim to pass on projection flag alone, got %+v", summary.ClaimGates)
	}
}

func TestBuildComparesIgnoresPostHogSafetyNoHumanProjectionFlag(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressure(t, baseline, 0.4, 0.7)
	writePressure(t, current, 0.9, 0.2)
	writePostHogLinkEnrichmentSafetyEvent(t, baseline, false)
	writePostHogLinkEnrichmentSafetyEvent(t, current, true)

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "improved" {
		t.Fatalf("expected projection flag not to create guardrail regression, got %s comparison=%+v", summary.ImprovementStatus, summary.Comparison)
	}
	if summary.Comparison != nil && containsString(summary.Comparison.ReasonCodes, "guardrail_regression") {
		t.Fatalf("expected no guardrail_regression reason from projection flag, got %+v", summary.Comparison)
	}
}

func TestBuildReadsLinkArtifactRequestSummaryMetrics(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "link-enrichment", "requests", "link-artifact-requests.json"), map[string]any{
		"schema_version": "local-link-artifact-requests/v0.1",
		"summary": map[string]any{
			"source_count":              3,
			"url_accounting_coverage":   0.75,
			"artifact_match_coverage":   0.5,
			"non_generalizable_runtime": true,
		},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	metrics := summary.Artifacts[0].Metrics
	if metrics["url_accounting_coverage"] != 0.75 {
		t.Fatalf("expected nested URL accounting coverage, got metrics=%+v", metrics)
	}
	if metrics["artifact_match_coverage"] != 0.5 {
		t.Fatalf("expected nested artifact match coverage, got metrics=%+v", metrics)
	}
	if !summary.Artifacts[0].Flags["non_generalizable_runtime"] {
		t.Fatalf("expected nested non-generalizable flag, got flags=%+v", summary.Artifacts[0].Flags)
	}
	if summary.TopImprovementTarget.Code != "needs_held_out_labels" {
		t.Fatalf("expected nested flag to drive non-generalizable target, got %+v", summary.TopImprovementTarget)
	}
}

func TestBuildPreservesNonGeneralizableRuntimeAcrossArtifacts(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"corpus_id":                  "corpus-a",
		"source_count":               2,
		"evidence_ready_atom_ratio":  0.9,
		"review_burden_ratio":        0.1,
		"corpus_fingerprint":         "same",
		"command_config_fingerprint": "same-config",
		"held_out":                   true,
		"non_generalizable_runtime":  true,
		"guardrails":                 completeGuardrails(),
	})
	writeFixture(t, filepath.Join(root, "link-enrichment", "requests", "link-artifact-requests.json"), map[string]any{
		"schema_version": "local-link-artifact-requests/v0.1",
		"summary": map[string]any{
			"source_count":                 2,
			"url_accounting_coverage":      1,
			"artifact_match_coverage":      1,
			"non_generalizable_runtime":    false,
			"missing_link_reduction_ratio": 0.5,
		},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.GeneralizationStatus != "non_generalizable" {
		t.Fatalf("expected non-generalizable blocker to be sticky, got %s", summary.GeneralizationStatus)
	}
}

func TestBuildReadsLinkEnrichmentComparisonFingerprints(t *testing.T) {
	root := t.TempDir()
	writeLinkEnrichmentComparison(t, root, "same-corpus", "same-config", 0.4)

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	fingerprints := summary.Artifacts[0].Fingerprints
	if fingerprints["corpus_fingerprint"] != "same-corpus" {
		t.Fatalf("expected baseline corpus fingerprint normalized, got %+v", fingerprints)
	}
	if fingerprints["command_config_fingerprint"] != "same-config" {
		t.Fatalf("expected baseline config fingerprint normalized, got %+v", fingerprints)
	}
	if fingerprints["enriched_corpus_fingerprint"] != "same-corpus" || fingerprints["baseline_corpus_fingerprint"] != "same-corpus" {
		t.Fatalf("expected raw link-enrichment fingerprints preserved, got %+v", fingerprints)
	}
}

func TestBuildComparesMissingLinkEnrichmentReductionRatio(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeLinkEnrichmentComparison(t, baseline, "same-corpus", "same-config", 0.2)
	writeLinkEnrichmentComparison(t, current, "same-corpus", "same-config", 0.6)

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "improved" {
		t.Fatalf("expected missing-link enrichment improvement, got %s comparison=%+v", summary.ImprovementStatus, summary.Comparison)
	}
	if summary.Comparison == nil || summary.Comparison.MetricDeltas["missing_link_enrichment_reduction_ratio"] <= 0 {
		t.Fatalf("expected missing-link enrichment delta, got %+v", summary.Comparison)
	}
}

func TestBuildComparesLinkEnrichmentOnBaselineFingerprints(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeLinkEnrichmentComparisonWithFingerprints(t, baseline, "source-corpus", "baseline-enriched", "source-config", "baseline-enriched-config", 0.2)
	writeLinkEnrichmentComparisonWithFingerprints(t, current, "source-corpus", "current-enriched", "source-config", "current-enriched-config", 0.6)

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "improved" {
		t.Fatalf("expected baseline fingerprint match to compare despite enriched fingerprint changes, got %s comparison=%+v", summary.ImprovementStatus, summary.Comparison)
	}
	if summary.Comparison == nil || summary.Comparison.MetricDeltas["missing_link_enrichment_reduction_ratio"] <= 0 {
		t.Fatalf("expected missing-link enrichment delta, got %+v", summary.Comparison)
	}
}

func TestBuildBlocksComparisonWhenArtifactDeclaresNotComparable(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writeLinkEnrichmentComparison(t, baseline, "same-corpus", "same-config", 0.2)
	writeFixture(t, filepath.Join(current, "link-enrichment", "comparison", "comparison-summary.json"), map[string]any{
		"schema_version":                          "link-enrichment-comparison/v0.1",
		"comparable":                              false,
		"reason_codes":                            []any{"not_comparable"},
		"enriched_corpus_fingerprint":             "same-corpus",
		"enriched_config_fingerprint":             "same-config",
		"missing_link_enrichment_reduction_ratio": 0.8,
		"needs_enrichment_reduction_ratio":        0.8,
		"guardrails":                              completeGuardrails(),
	})

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "not_comparable" {
		t.Fatalf("expected artifact non-comparability to block improvement, got %s comparison=%+v", summary.ImprovementStatus, summary.Comparison)
	}
	if summary.Comparison == nil || !containsString(summary.Comparison.ReasonCodes, "artifact_not_comparable") {
		t.Fatalf("expected artifact_not_comparable reason, got %+v", summary.Comparison)
	}
	if gateStatus(summary, "improvement_claim") != "blocked" {
		t.Fatalf("expected improvement claim blocked, got %+v", summary.ClaimGates)
	}
}

func TestBuildReadsAutonomyReadinessCountMetrics(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "autonomy-readiness", "readiness-report.json"), map[string]any{
		"schema_version":          "autonomy-readiness-report/v0.1",
		"held_out":                true,
		"threshold":               0.98,
		"threshold_status":        "eligible",
		"accuracy":                0.97,
		"safety_counters":         map[string]any{"destination_writes": 0, "auto_accepts": 0, "no_human_claims": 0, "committed_private_artifacts": 0},
		"counts":                  map[string]any{"eval_counted_count": 100, "evidence_ready_count": 92, "human_review_required_count": 8, "model_error_count": 2},
		"top_improvement_targets": []any{},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	metrics := summary.Artifacts[0].Metrics
	for key, expected := range map[string]float64{
		"eval_counted_count":          100,
		"evidence_ready_count":        92,
		"human_review_required_count": 8,
		"model_error_count":           2,
	} {
		if metrics[key] != expected {
			t.Fatalf("expected nested %s=%v, got metrics=%+v", key, expected, metrics)
		}
	}
}

func TestBuildBlocksSideEffectClaimWithoutGuardrailEvidence(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "trace", "trace-summary.json"), map[string]any{
		"schema_version":            "mindline-trace-summary/v0.1",
		"source_count":              1,
		"evidence_ready_atom_ratio": 1,
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gateStatus(summary, "side_effect_claim") != "blocked" {
		t.Fatalf("expected side effect claim blocked without guardrail evidence, got %+v", summary.ClaimGates)
	}
	if gate := gateByName(summary, "side_effect_claim"); !containsString(gate.ReasonCodes, "missing_side_effect_evidence") {
		t.Fatalf("expected missing_side_effect_evidence reason, got %+v", gate)
	}
}

func TestBuildReadsAutonomySafetyCountersForSideEffectClaim(t *testing.T) {
	root := t.TempDir()
	writePressureWithGuardrails(t, root, 0.9, 0.2, map[string]any{"destination_writes": 0, "hosted_inference_calls": 0})
	writeFixture(t, filepath.Join(root, "autonomy-readiness", "readiness-report.json"), map[string]any{
		"schema_version":          "autonomy-readiness-report/v0.1",
		"held_out":                true,
		"threshold":               0.98,
		"threshold_status":        "eligible",
		"accuracy":                1,
		"safety_counters":         map[string]any{"destination_writes": 1, "auto_accepts": 1, "no_human_claims": 1, "committed_private_artifacts": 1},
		"counts":                  map[string]any{"eval_counted_count": 100, "evidence_ready_count": 100},
		"top_improvement_targets": []any{},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.Guardrails.DestinationWrites != 1 || summary.Guardrails.AutoAccepts != 1 || !summary.Guardrails.NoHumanClaims || summary.Guardrails.CommittedPrivateArtifacts != 1 {
		t.Fatalf("expected autonomy safety counters extracted, got %+v", summary.Guardrails)
	}
	if gateStatus(summary, "side_effect_claim") != "fail" {
		t.Fatalf("expected side effect claim failure from autonomy safety counters, got %+v", summary.ClaimGates)
	}
	if gate := gateByName(summary, "dec64_no_human_claim"); gate.Status != "blocked" {
		t.Fatalf("expected DEC-64 gate blocked when autonomy safety counters are nonzero, got %+v", gate)
	}
}

func TestBuildBlocksSideEffectClaimWithIncompleteAutonomySafetyCounters(t *testing.T) {
	root := t.TempDir()
	writePressureWithGuardrails(t, root, 0.9, 0.2, map[string]any{"destination_writes": 0, "hosted_inference_calls": 0})
	writeFixture(t, filepath.Join(root, "autonomy-readiness", "readiness-report.json"), map[string]any{
		"schema_version":          "autonomy-readiness-report/v0.1",
		"held_out":                true,
		"threshold":               0.98,
		"threshold_status":        "eligible",
		"accuracy":                1,
		"safety_counters":         map[string]any{"destination_writes": 0},
		"counts":                  map[string]any{"eval_counted_count": 100, "evidence_ready_count": 100},
		"top_improvement_targets": []any{},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gate := gateByName(summary, "side_effect_claim"); gate.Status != "blocked" || !containsString(gate.ReasonCodes, "missing_side_effect_evidence") {
		t.Fatalf("expected side effect claim blocked without complete safety counters, got %+v", gate)
	}
	if gate := gateByName(summary, "dec64_no_human_claim"); gate.Status != "blocked" {
		t.Fatalf("expected DEC-64 gate blocked without complete safety evidence, got %+v", gate)
	}
}

func TestBuildPassesDEC64GateWithHeldOutAcceptanceProof(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "corpus-acceptance", "benchmark-summary.json"), map[string]any{
		"schema_version":             "corpus-acceptance-summary/v0.1",
		"suite_kind":                 "held_out",
		"held_out":                   true,
		"suite_valid":                true,
		"dec64_eligible":             true,
		"threshold":                  0.98,
		"accuracy":                   1,
		"eval_count":                 100,
		"corpus_fingerprint":         "acceptance-corpus",
		"command_config_fingerprint": "acceptance-config",
		"guardrails":                 completeGuardrails(),
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gate := gateByName(summary, "dec64_no_human_claim"); gate.Status != "pass" {
		t.Fatalf("expected DEC-64 gate pass from held-out acceptance proof, got %+v", gate)
	}
}

func TestBuildPassesDEC64GateWithEligibleAutonomyReadinessProof(t *testing.T) {
	root := t.TempDir()
	writePressureWithGuardrails(t, root, 0.9, 0.2, map[string]any{})
	writeFixture(t, filepath.Join(root, "autonomy-readiness", "readiness-report.json"), map[string]any{
		"schema_version":          "autonomy-readiness-report/v0.1",
		"held_out":                true,
		"threshold":               0.98,
		"threshold_status":        "eligible",
		"accuracy":                1,
		"safety_counters":         map[string]any{"destination_writes": 0, "auto_accepts": 0, "no_human_claims": 0, "committed_private_artifacts": 0},
		"counts":                  map[string]any{"eval_counted_count": 100, "evidence_ready_count": 100},
		"top_improvement_targets": []any{},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gate := gateByName(summary, "dec64_no_human_claim"); gate.Status != "pass" {
		t.Fatalf("expected DEC-64 gate pass from eligible autonomy readiness proof, got %+v", gate)
	}
}

func TestBuildPassesDEC64GateWithStandaloneAutonomyReadinessProof(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "autonomy-readiness", "readiness-report.json"), map[string]any{
		"schema_version":          "autonomy-readiness-report/v0.1",
		"held_out":                true,
		"threshold":               0.98,
		"threshold_status":        "eligible",
		"accuracy":                1,
		"safety_counters":         map[string]any{"destination_writes": 0, "auto_accepts": 0, "no_human_claims": 0, "committed_private_artifacts": 0},
		"counts":                  map[string]any{"eval_counted_count": 100, "evidence_ready_count": 100},
		"top_improvement_targets": []any{},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gate := gateByName(summary, "side_effect_claim"); gate.Status != "pass" {
		t.Fatalf("expected side-effect gate pass from standalone autonomy safety counters, got %+v", gate)
	}
	if gate := gateByName(summary, "dec64_no_human_claim"); gate.Status != "pass" {
		t.Fatalf("expected DEC-64 gate pass from standalone autonomy readiness proof, got %+v", gate)
	}
}

func TestBuildBlocksDEC64GateForUnscopedEligibilityFlag(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"source_count":               2,
		"evidence_ready_atom_ratio":  1,
		"review_burden_ratio":        0,
		"held_out":                   true,
		"dec64_eligible":             true,
		"threshold":                  0.98,
		"accuracy":                   1,
		"corpus_fingerprint":         "pressure-corpus",
		"command_config_fingerprint": "pressure-config",
		"guardrails":                 completeGuardrails(),
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gate := gateByName(summary, "dec64_no_human_claim"); gate.Status != "blocked" {
		t.Fatalf("expected DEC-64 gate blocked for unscoped eligibility flag, got %+v", gate)
	}
}

func TestBuildBlocksDEC64GateForInvalidAcceptanceProof(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "corpus-acceptance", "benchmark-summary.json"), map[string]any{
		"schema_version":             "corpus-acceptance-summary/v0.1",
		"suite_kind":                 "held_out",
		"held_out":                   true,
		"suite_valid":                false,
		"dec64_eligible":             true,
		"threshold":                  0.98,
		"accuracy":                   1,
		"eval_count":                 100,
		"corpus_fingerprint":         "acceptance-corpus",
		"command_config_fingerprint": "acceptance-config",
		"guardrails":                 map[string]any{"destination_writes": 0, "hosted_inference_calls": 0},
	})

	summary, err := Build(root, filepath.Join(root, "out"), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if gate := gateByName(summary, "dec64_no_human_claim"); gate.Status != "blocked" {
		t.Fatalf("expected DEC-64 gate blocked for invalid acceptance proof, got %+v", gate)
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

func TestBuildBlocksFingerprintlessSameArtifactComparison(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressureWithoutFingerprint(t, baseline, 0.4, 0.7)
	writePressureWithoutFingerprint(t, current, 0.9, 0.2)

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "not_comparable" {
		t.Fatalf("expected not_comparable, got %s", summary.ImprovementStatus)
	}
	if summary.Comparison == nil || !containsString(summary.Comparison.ReasonCodes, "missing_fingerprints") {
		t.Fatalf("expected missing_fingerprints reason, got %+v", summary.Comparison)
	}
}

func TestBuildBlocksCommandOnlyFingerprintComparison(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressureWithCommandFingerprintOnly(t, baseline, 0.4, 0.7, "same-config")
	writePressureWithCommandFingerprintOnly(t, current, 0.9, 0.2, "same-config")

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "not_comparable" {
		t.Fatalf("expected not_comparable, got %s", summary.ImprovementStatus)
	}
	if summary.Comparison == nil || !containsString(summary.Comparison.ReasonCodes, "missing_fingerprints") {
		t.Fatalf("expected missing_fingerprints reason, got %+v", summary.Comparison)
	}
}

func TestBuildBlocksConflictingCorpusFingerprintsWithinRun(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressureWithFingerprint(t, baseline, 0.4, 0.7, "same")
	writePressureWithFingerprint(t, current, 0.9, 0.2, "same")
	writeFixture(t, filepath.Join(current, "corpus-pressure", "trace-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-trace-summary/v0.1",
		"processed_source_ratio":     1,
		"evidence_ready_atom_ratio":  0.9,
		"corpus_fingerprint":         "different",
		"command_config_fingerprint": "same-config",
		"guardrails":                 completeGuardrails(),
	})

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "not_comparable" {
		t.Fatalf("expected not_comparable, got %s", summary.ImprovementStatus)
	}
	if summary.Comparison == nil || !containsString(summary.Comparison.ReasonCodes, "conflicting_fingerprints") {
		t.Fatalf("expected conflicting_fingerprints reason, got %+v", summary.Comparison)
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

func TestBuildAutonomySafetyRegressionBlocksImprovementClaim(t *testing.T) {
	root := t.TempDir()
	baseline := filepath.Join(root, "baseline")
	current := filepath.Join(root, "current")
	writePressureWithGuardrails(t, baseline, 0.4, 0.7, map[string]any{"destination_writes": 0, "hosted_inference_calls": 0})
	writePressureWithGuardrails(t, current, 0.9, 0.2, map[string]any{"destination_writes": 0, "hosted_inference_calls": 0})
	writeFixture(t, filepath.Join(baseline, "autonomy-readiness", "readiness-report.json"), map[string]any{
		"schema_version":          "autonomy-readiness-report/v0.1",
		"held_out":                true,
		"threshold":               0.98,
		"threshold_status":        "eligible",
		"accuracy":                1,
		"safety_counters":         map[string]any{"destination_writes": 0, "auto_accepts": 0, "no_human_claims": 0, "committed_private_artifacts": 0},
		"counts":                  map[string]any{"eval_counted_count": 100, "evidence_ready_count": 100},
		"top_improvement_targets": []any{},
	})
	writeFixture(t, filepath.Join(current, "autonomy-readiness", "readiness-report.json"), map[string]any{
		"schema_version":          "autonomy-readiness-report/v0.1",
		"held_out":                true,
		"threshold":               0.98,
		"threshold_status":        "eligible",
		"accuracy":                1,
		"safety_counters":         map[string]any{"destination_writes": 0, "auto_accepts": 1, "no_human_claims": 1, "committed_private_artifacts": 1},
		"counts":                  map[string]any{"eval_counted_count": 100, "evidence_ready_count": 100},
		"top_improvement_targets": []any{},
	})

	summary, err := Build(current, filepath.Join(root, "out"), Options{BaselineRoot: baseline})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.ImprovementStatus != "regressed" {
		t.Fatalf("expected autonomy safety regression to dominate quality improvement, got %s comparison=%+v", summary.ImprovementStatus, summary.Comparison)
	}
	if summary.Comparison == nil || !containsString(summary.Comparison.ReasonCodes, "guardrail_regression") {
		t.Fatalf("expected guardrail_regression reason, got %+v", summary.Comparison)
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

func TestBuildRejectsSymlinkEscapeOutputFile(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "current")
	writePressure(t, current, 0.9, 0.2)
	out := filepath.Join(root, "out")
	readbackDir := filepath.Join(out, DirName)
	if err := os.MkdirAll(readbackDir, 0o755); err != nil {
		t.Fatalf("mkdir readback dir: %v", err)
	}
	escaped := filepath.Join(root, "escaped")
	if err := os.MkdirAll(escaped, 0o755); err != nil {
		t.Fatalf("mkdir escaped: %v", err)
	}
	escapedFile := filepath.Join(escaped, "readback-summary.json")
	if err := os.WriteFile(escapedFile, []byte("original"), 0o644); err != nil {
		t.Fatalf("write escaped file: %v", err)
	}
	if err := os.Symlink(escapedFile, filepath.Join(readbackDir, "readback-summary.json")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := Build(current, out, Options{ProtectedRoots: []string{escaped}})
	if err == nil || !strings.Contains(err.Error(), "escapes output root") {
		t.Fatalf("expected symlinked output file rejection, got %v", err)
	}
	if got := readString(t, escapedFile); got != "original" {
		t.Fatalf("escaped output file was overwritten: %q", got)
	}
}

func TestBuildRejectsProtectedOutputRootBeforeCreatingDirs(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "current")
	writePressure(t, current, 0.9, 0.2)
	protected := filepath.Join(root, "protected")
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatalf("mkdir protected: %v", err)
	}
	out := filepath.Join(protected, "out")

	_, err := Build(current, out, Options{ProtectedRoots: []string{protected}})
	if err == nil || !strings.Contains(err.Error(), "protected output root") {
		t.Fatalf("expected protected output root rejection, got %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("protected output directory should not be created, err=%v", err)
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
	allGuardrails := completeGuardrails()
	for key, value := range guardrails {
		allGuardrails[key] = value
	}
	writeFixture(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"corpus_id":                  "corpus-a",
		"source_count":               2,
		"evidence_ready_atom_ratio":  evidenceReady,
		"review_burden_ratio":        reviewBurden,
		"corpus_fingerprint":         "same",
		"command_config_fingerprint": "same-config",
		"guardrails":                 allGuardrails,
	})
}

func completeGuardrails() map[string]any {
	return map[string]any{
		"network_fetches":          0,
		"hosted_telemetry_exports": 0,
		"hosted_inference_calls":   0,
		"browser_calls":            0,
		"slack_api_calls":          0,
		"destination_writes":       0,
		"product_brain_writes":     0,
		"tolaria_writes":           0,
	}
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

func writePressureWithCommandFingerprintOnly(t *testing.T, root string, evidenceReady, reviewBurden float64, commandFingerprint string) {
	t.Helper()
	writeFixture(t, filepath.Join(root, "corpus-pressure", "pressure-summary.json"), map[string]any{
		"schema_version":             "corpus-pressure-summary/v0.1",
		"corpus_id":                  "corpus-a",
		"source_count":               2,
		"evidence_ready_atom_ratio":  evidenceReady,
		"review_burden_ratio":        reviewBurden,
		"command_config_fingerprint": commandFingerprint,
		"guardrails":                 completeGuardrails(),
	})
}

func writeLinkEnrichmentComparison(t *testing.T, root, corpusFingerprint, configFingerprint string, missingLinkReduction float64) {
	t.Helper()
	writeLinkEnrichmentComparisonWithFingerprints(t, root, corpusFingerprint, corpusFingerprint, configFingerprint, configFingerprint, missingLinkReduction)
}

func writeLinkEnrichmentComparisonWithFingerprints(t *testing.T, root, baselineCorpusFingerprint, enrichedCorpusFingerprint, baselineConfigFingerprint, enrichedConfigFingerprint string, missingLinkReduction float64) {
	t.Helper()
	writeFixture(t, filepath.Join(root, "link-enrichment", "comparison", "comparison-summary.json"), map[string]any{
		"schema_version":                          "link-enrichment-comparison/v0.1",
		"comparable":                              true,
		"baseline_corpus_fingerprint":             baselineCorpusFingerprint,
		"enriched_corpus_fingerprint":             enrichedCorpusFingerprint,
		"baseline_config_fingerprint":             baselineConfigFingerprint,
		"enriched_config_fingerprint":             enrichedConfigFingerprint,
		"missing_link_enrichment_reduction_ratio": missingLinkReduction,
		"needs_enrichment_reduction_ratio":        missingLinkReduction,
		"guardrails":                              completeGuardrails(),
	})
}

func writePostHogLinkEnrichmentSafetyEvent(t *testing.T, root string, noHumanClaims bool) {
	t.Helper()
	writeFixture(t, filepath.Join(root, "link-enrichment", "posthog", "eval-projection.json"), map[string]any{
		"schema_version": "mindline-link-enrichment-eval-projection/v0.1",
		"events": []any{map[string]any{"properties": map[string]any{
			"$ai_evaluation_result":              true,
			"metadata_only":                      true,
			"safety_network_fetches":             0,
			"safety_hosted_telemetry_exports":    0,
			"safety_hosted_inference_calls":      0,
			"safety_browser_calls":               0,
			"safety_slack_api_calls":             0,
			"safety_destination_writes":          0,
			"safety_product_brain_writes":        0,
			"safety_tolaria_writes":              0,
			"safety_auto_accepts":                0,
			"safety_no_human_claims":             noHumanClaims,
			"safety_committed_private_artifacts": 0,
		}}},
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

func gateByName(summary Summary, gateName string) ClaimGate {
	for _, gate := range summary.ClaimGates {
		if gate.Gate == gateName {
			return gate
		}
	}
	return ClaimGate{}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}
