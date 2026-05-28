package evalreadback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Build(inputRoot, outRoot string, options Options) (Summary, error) {
	model, err := buildModel(inputRoot)
	if err != nil {
		return Summary{}, err
	}
	summary := summarize(model)
	if strings.TrimSpace(options.BaselineRoot) != "" {
		baseline, err := buildModel(options.BaselineRoot)
		if err != nil {
			return Summary{}, fmt.Errorf("read baseline: %w", err)
		}
		comparison := compareModels(baseline, model)
		summary.BaselineRootLabel = baseline.rootLabel
		summary.Comparison = &comparison
		summary.ImprovementStatus = comparison.Status
		rebuildClaimGates(&summary)
	}
	if err := writeSummary(outRoot, summary, options.ProtectedRoots); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

type readbackModel struct {
	root          string
	rootLabel     string
	sampleStatus  string
	artifacts     []ArtifactEvidence
	guardrails    Guardrails
	metrics       map[string]float64
	flags         map[string]bool
	fingerprints  map[string]string
	artifactTypes map[string]bool
}

func buildModel(inputRoot string) (readbackModel, error) {
	if strings.TrimSpace(inputRoot) == "" {
		return readbackModel{}, errors.New("missing input root")
	}
	root, err := filepath.Abs(inputRoot)
	if err != nil {
		return readbackModel{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return readbackModel{}, err
	}
	if !info.IsDir() {
		return readbackModel{}, fmt.Errorf("input root must be a directory: %s", inputRoot)
	}
	model := readbackModel{
		root:          root,
		rootLabel:     safeRootLabel(root),
		sampleStatus:  sampleStatusFor(root),
		metrics:       map[string]float64{},
		flags:         map[string]bool{},
		fingerprints:  map[string]string{},
		artifactTypes: map[string]bool{},
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		ref := filepath.ToSlash(rel)
		artifactType := artifactTypeFor(ref)
		if artifactType == "" {
			return nil
		}
		artifact, err := readArtifact(path, ref, artifactType)
		if err != nil {
			return err
		}
		model.artifacts = append(model.artifacts, artifact)
		mergeModelEvidence(&model, artifact)
		return nil
	})
	if err != nil {
		return readbackModel{}, err
	}
	sort.Slice(model.artifacts, func(i, j int) bool { return model.artifacts[i].Ref < model.artifacts[j].Ref })
	if len(model.artifacts) == 0 {
		return readbackModel{}, errors.New("no supported eval/trace artifacts found")
	}
	return model, nil
}

func artifactTypeFor(ref string) string {
	switch {
	case strings.HasSuffix(ref, "trace/trace-summary.json"):
		return "generic_trace_summary"
	case strings.HasSuffix(ref, "corpus-pressure/pressure-summary.json"):
		return "corpus_pressure_summary"
	case strings.HasSuffix(ref, "corpus-pressure/eval-input.json"):
		return "corpus_pressure_eval_input"
	case strings.HasSuffix(ref, "corpus-pressure/trace-summary.json"):
		return "corpus_pressure_trace_summary"
	case strings.HasSuffix(ref, "corpus-pressure-loop/loop-summary.json"):
		return "corpus_pressure_loop_summary"
	case strings.HasSuffix(ref, "corpus-acceptance/benchmark-summary.json"):
		return "corpus_acceptance_benchmark"
	case strings.HasSuffix(ref, "autonomy-readiness/readiness-report.json"):
		return "autonomy_readiness_report"
	case strings.HasSuffix(ref, "link-enrichment/loop-summary.json"):
		return "link_enrichment_loop_summary"
	case strings.HasSuffix(ref, "link-enrichment/comparison/comparison-summary.json"):
		return "link_enrichment_comparison_summary"
	case strings.HasSuffix(ref, "link-enrichment/requests/link-artifact-requests.json"):
		return "link_artifact_requests"
	case strings.HasSuffix(ref, "link-enrichment/posthog/eval-projection.json"):
		return "link_enrichment_eval_projection"
	default:
		return ""
	}
}

func readArtifact(path, ref, artifactType string) (ArtifactEvidence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ArtifactEvidence{}, err
	}
	if containsDeniedString(string(data)) {
		return ArtifactEvidence{
			Type: artifactType, Ref: ref, Status: "unsafe_or_leaky", ReasonCodes: []string{"raw_private_or_secret_pattern"},
		}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return ArtifactEvidence{}, fmt.Errorf("decode %s: %w", ref, err)
	}
	artifact := ArtifactEvidence{
		Type:          artifactType,
		SchemaVersion: stringValue(raw["schema_version"]),
		Ref:           ref,
		Status:        "detected",
		Metrics:       map[string]float64{},
		Flags:         map[string]bool{},
		Fingerprints:  map[string]string{},
	}
	if !supportedSchema(artifact.Type, artifact.SchemaVersion) {
		artifact.Status = "unsupported_schema"
		artifact.ReasonCodes = []string{"unsupported_schema_version"}
		artifact.Metrics = nil
		artifact.Flags = nil
		artifact.Fingerprints = nil
		return artifact, nil
	}
	extractEvidence(raw, &artifact)
	return artifact, nil
}

func supportedSchema(artifactType, schemaVersion string) bool {
	expected := map[string]string{
		"generic_trace_summary":              "mindline-trace-summary/v0.1",
		"corpus_pressure_summary":            "corpus-pressure-summary/v0.1",
		"corpus_pressure_eval_input":         "corpus-pressure-eval-input/v0.1",
		"corpus_pressure_trace_summary":      "corpus-pressure-trace-summary/v0.1",
		"corpus_pressure_loop_summary":       "corpus-pressure-loop-summary/v0.1",
		"corpus_acceptance_benchmark":        "corpus-acceptance-summary/v0.1",
		"autonomy_readiness_report":          "autonomy-readiness-report/v0.1",
		"link_enrichment_loop_summary":       "link-enrichment-loop-summary/v0.1",
		"link_enrichment_comparison_summary": "link-enrichment-comparison/v0.1",
		"link_artifact_requests":             "local-link-artifact-requests/v0.1",
		"link_enrichment_eval_projection":    "mindline-link-enrichment-eval-projection/v0.1",
	}
	return strings.TrimSpace(schemaVersion) == expected[artifactType]
}

func extractEvidence(raw map[string]any, artifact *ArtifactEvidence) {
	for _, key := range []string{
		"source_count", "candidate_count", "semantic_candidate_count", "evidence_ready_atom_count",
		"review_burden_count", "missing_link_reduction_ratio", "needs_enrichment_reduction_ratio",
		"missing_link_enrichment_reduction_ratio",
		"url_accounting_coverage", "artifact_match_coverage", "model_error_count",
		"human_review_required_count",
	} {
		if value, ok := numberValue(raw[key]); ok {
			artifact.Metrics[key] = value
		}
	}
	for _, key := range []string{"processed_source_ratio", "evidence_ready_atom_ratio", "review_burden_ratio"} {
		if value, ok := numberValue(raw[key]); ok {
			artifact.Metrics[key] = value
		}
	}
	for _, key := range []string{"ready_for_50_file_pressure", "held_out", "non_generalizable_runtime", "comparable"} {
		if value, ok := boolValue(raw[key]); ok {
			artifact.Flags[key] = value
		}
	}
	for _, key := range []string{"corpus_fingerprint", "command_config_fingerprint", "replay_fingerprint", "graph_replay_fingerprint"} {
		if value := stringValue(raw[key]); value != "" {
			artifact.Fingerprints[key] = value
		}
	}
	if guardrails, ok := raw["guardrails"].(map[string]any); ok {
		extractGuardrails(guardrails, artifact)
	}
	if comparison, ok := raw["comparison"].(map[string]any); ok {
		extractEvidence(comparison, artifact)
	}
	if requestSummary, ok := raw["request_summary"].(map[string]any); ok {
		extractEvidence(requestSummary, artifact)
	}
	if events, ok := raw["events"].([]any); ok {
		for _, event := range events {
			item, ok := event.(map[string]any)
			if !ok {
				continue
			}
			props, ok := item["properties"].(map[string]any)
			if !ok {
				props, ok = item["Properties"].(map[string]any)
			}
			if !ok {
				continue
			}
			for _, key := range []string{"$ai_evaluation_result", "non_generalizable_runtime", "metadata_only"} {
				if value, ok := boolValue(props[key]); ok {
					artifact.Flags[key] = value
				}
			}
			for _, key := range []string{
				"missing_link_reduction_ratio", "needs_enrichment_reduction_ratio",
				"missing_link_enrichment_reduction_ratio",
				"url_accounting_coverage", "artifact_match_coverage",
				"safety_network_fetches", "safety_hosted_telemetry_exports", "safety_hosted_inference_calls",
				"safety_destination_writes",
			} {
				if value, ok := numberValue(props[key]); ok {
					artifact.Metrics[key] = value
				}
			}
		}
	}
}

func extractGuardrails(guardrails map[string]any, artifact *ArtifactEvidence) {
	for _, key := range []string{"network_fetches", "hosted_telemetry_exports", "hosted_inference_calls", "destination_writes", "product_brain_writes", "tolaria_writes"} {
		if value, ok := numberValue(guardrails[key]); ok {
			artifact.Metrics["guardrail_"+key] = value
		}
	}
}

func mergeModelEvidence(model *readbackModel, artifact ArtifactEvidence) {
	model.artifactTypes[artifact.Type] = true
	if artifact.Status == "unsafe_or_leaky" {
		model.flags["unsafe_or_leaky"] = true
	}
	for key, value := range artifact.Metrics {
		model.metrics[key] = value
		switch key {
		case "guardrail_network_fetches", "safety_network_fetches":
			model.guardrails.NetworkFetches += int(value)
		case "guardrail_hosted_telemetry_exports", "safety_hosted_telemetry_exports":
			model.guardrails.HostedTelemetryExports += int(value)
		case "guardrail_hosted_inference_calls", "safety_hosted_inference_calls":
			model.guardrails.HostedInferenceCalls += int(value)
		case "guardrail_destination_writes", "safety_destination_writes":
			model.guardrails.DestinationWrites += int(value)
		case "guardrail_product_brain_writes":
			model.guardrails.ProductBrainWrites += int(value)
		case "guardrail_tolaria_writes":
			model.guardrails.TolariaWrites += int(value)
		}
	}
	for key, value := range artifact.Flags {
		model.flags[key] = value
	}
	for key, value := range artifact.Fingerprints {
		if _, exists := model.fingerprints[key]; !exists {
			model.fingerprints[key] = value
		}
	}
}

func summarize(model readbackModel) Summary {
	typeCounts := map[string]int{}
	refs := []string{}
	for _, artifact := range model.artifacts {
		typeCounts[artifact.Type]++
		refs = append(refs, artifact.Ref)
	}
	sampleStatus := model.sampleStatus
	if sampleStatus == "unknown" && model.flags["held_out"] {
		sampleStatus = "held_out"
	}
	sort.Strings(refs)
	summary := Summary{
		SchemaVersion:      SummarySchemaVersion,
		RunID:              stableID(model.rootLabel, refs),
		InputRootLabel:     model.rootLabel,
		ArtifactCount:      len(model.artifacts),
		ArtifactTypeCounts: typeCounts,
		SampleStatus:       sampleStatus,
		ImprovementStatus:  "not_evaluated",
		Guardrails:         model.guardrails,
		SafeArtifactRefs:   refs,
		Artifacts:          model.artifacts,
	}
	if model.flags["non_generalizable_runtime"] || sampleStatus == "private_runtime" || sampleStatus == "temp_runtime" || sampleStatus == "unknown" {
		summary.GeneralizationStatus = "non_generalizable"
	} else {
		summary.GeneralizationStatus = "generalizable"
	}
	summary.TopImprovementTarget = chooseTarget(model, summary.GeneralizationStatus)
	summary.RerunInstructions = []string{"rerun the source command after addressing " + summary.TopImprovementTarget.Code + ", then run eval readback with --baseline pointing to this run"}
	rebuildClaimGates(&summary)
	return summary
}

func rebuildClaimGates(summary *Summary) {
	gates := []ClaimGate{
		{Gate: "artifact_presence", Status: "pass", EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "readback has local evidence to inspect"},
	}
	unsafe := hasUnsafeArtifact(summary)
	unsupported := hasUnsupportedArtifact(summary)
	if unsafe {
		gates = append(gates, ClaimGate{Gate: "privacy_safe_readback", Status: "fail", ReasonCodes: []string{"unsafe_or_leaky"}, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks improvement and Chain proof claims until unsafe artifacts are removed or redacted"})
	} else {
		gates = append(gates, ClaimGate{Gate: "privacy_safe_readback", Status: "pass", ClaimImpact: "readback output did not detect unsafe supported artifacts"})
	}
	if summary.GeneralizationStatus == "generalizable" {
		gates = append(gates, ClaimGate{Gate: "generalization_claim", Status: "pass", ClaimImpact: "sample can support its bounded generalization claim"})
	} else {
		gates = append(gates, ClaimGate{Gate: "generalization_claim", Status: "blocked", ReasonCodes: []string{"sample_bound_or_non_held_out"}, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks broad product, DEC-64, or no-human claims"})
	}
	improvementStatus := summary.ImprovementStatus
	switch improvementStatus {
	case "improved":
		switch {
		case unsafe:
			gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"unsafe_or_leaky"}, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks improvement claim until readback evidence is privacy-safe"})
		case unsupported:
			gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"unsupported_schema"}, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks improvement claim until supported-looking artifacts use known schemas"})
		default:
			gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "pass", ClaimImpact: "current run improved against a comparable baseline"})
		}
	case "unchanged", "regressed":
		gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "fail", ReasonCodes: []string{improvementStatus}, ClaimImpact: "blocks improvement claim"})
	case "not_comparable":
		gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"not_comparable"}, ClaimImpact: "blocks improvement claim"})
	default:
		gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"missing_baseline"}, ClaimImpact: "blocks improvement claim until comparable baseline is supplied"})
	}
	gates = append(gates, ClaimGate{Gate: "dec64_no_human_claim", Status: "blocked", ReasonCodes: []string{"held_out_threshold_not_proven"}, ClaimImpact: "blocks no-human autonomy readiness claim"})
	if summary.Guardrails.NetworkFetches == 0 && summary.Guardrails.HostedTelemetryExports == 0 && summary.Guardrails.HostedInferenceCalls == 0 && summary.Guardrails.DestinationWrites == 0 && summary.Guardrails.ProductBrainWrites == 0 && summary.Guardrails.TolariaWrites == 0 {
		gates = append(gates, ClaimGate{Gate: "side_effect_claim", Status: "pass", ClaimImpact: "readback found no prohibited side-effect counters"})
	} else {
		gates = append(gates, ClaimGate{Gate: "side_effect_claim", Status: "fail", ReasonCodes: []string{"guardrail_counter_nonzero"}, ClaimImpact: "blocks safety claim"})
	}
	if summary.TopImprovementTarget.Code != "" {
		gates = append(gates, ClaimGate{Gate: "next_target", Status: "pass", EvidenceRefs: summary.TopImprovementTarget.EvidenceRefs, ClaimImpact: "next improvement target is explicit"})
	}
	summary.ClaimGates = gates
}

func hasUnsafeArtifact(summary *Summary) bool {
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsafe_or_leaky" {
			return true
		}
	}
	return false
}

func hasUnsupportedArtifact(summary *Summary) bool {
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsupported_schema" {
			return true
		}
	}
	return false
}

func compareModels(baseline, current readbackModel) ComparisonSummary {
	comparison := ComparisonSummary{
		SchemaVersion: ComparisonSchemaVersion,
		Status:        "not_comparable",
		BaselineLabel: baseline.rootLabel,
		CurrentLabel:  current.rootLabel,
		MetricDeltas:  map[string]float64{},
	}
	comparable, reasons := comparableModels(baseline, current)
	if !comparable {
		comparison.ReasonCodes = reasons
		return comparison
	}
	comparison.ReasonCodes = reasons
	improved, regressed := false, false
	for _, metric := range []string{"evidence_ready_atom_ratio", "processed_source_ratio", "missing_link_reduction_ratio", "needs_enrichment_reduction_ratio", "url_accounting_coverage", "artifact_match_coverage"} {
		before, bok := baseline.metrics[metric]
		after, aok := current.metrics[metric]
		if !bok || !aok {
			continue
		}
		delta := after - before
		comparison.MetricDeltas[metric] = delta
		if delta > 0 {
			improved = true
		}
		if delta < 0 {
			regressed = true
		}
	}
	for _, metric := range []string{"review_burden_ratio", "review_burden_count", "human_review_required_count", "model_error_count"} {
		before, bok := baseline.metrics[metric]
		after, aok := current.metrics[metric]
		if !bok || !aok {
			continue
		}
		delta := before - after
		comparison.MetricDeltas[metric+"_reduction"] = delta
		if delta > 0 {
			improved = true
		}
		if delta < 0 {
			regressed = true
		}
	}
	for _, guardrail := range []struct {
		name   string
		before int
		after  int
	}{
		{name: "network_fetches", before: baseline.guardrails.NetworkFetches, after: current.guardrails.NetworkFetches},
		{name: "hosted_telemetry_exports", before: baseline.guardrails.HostedTelemetryExports, after: current.guardrails.HostedTelemetryExports},
		{name: "hosted_inference_calls", before: baseline.guardrails.HostedInferenceCalls, after: current.guardrails.HostedInferenceCalls},
		{name: "destination_writes", before: baseline.guardrails.DestinationWrites, after: current.guardrails.DestinationWrites},
		{name: "product_brain_writes", before: baseline.guardrails.ProductBrainWrites, after: current.guardrails.ProductBrainWrites},
		{name: "tolaria_writes", before: baseline.guardrails.TolariaWrites, after: current.guardrails.TolariaWrites},
	} {
		delta := float64(guardrail.before - guardrail.after)
		if delta != 0 {
			comparison.MetricDeltas["guardrail_"+guardrail.name+"_reduction"] = delta
		}
		if guardrail.after > guardrail.before {
			regressed = true
			comparison.ReasonCodes = append(comparison.ReasonCodes, "guardrail_regression")
		}
	}
	switch {
	case regressed:
		comparison.Status = "regressed"
	case improved:
		comparison.Status = "improved"
	default:
		comparison.Status = "unchanged"
	}
	return comparison
}

func comparableModels(a, b readbackModel) (bool, []string) {
	checked := false
	for _, key := range []string{"corpus_fingerprint", "command_config_fingerprint"} {
		av, aok := a.fingerprints[key]
		bv, bok := b.fingerprints[key]
		if aok && bok {
			checked = true
			if av != bv {
				return false, []string{"fingerprint_mismatch"}
			}
		} else if aok != bok {
			return false, []string{"one_sided_fingerprint"}
		}
	}
	if checked {
		return true, nil
	}
	for artifactType := range a.artifactTypes {
		if b.artifactTypes[artifactType] {
			return false, []string{"missing_fingerprints"}
		}
	}
	return false, []string{"artifact_domain_mismatch"}
}

func chooseTarget(model readbackModel, generalization string) ImprovementTarget {
	refs := []string{}
	for _, artifact := range model.artifacts {
		refs = append(refs, artifact.Ref)
	}
	refs = firstRefs(refs)
	if model.flags["unsafe_or_leaky"] {
		return ImprovementTarget{Code: "unsafe_or_leaky", Rationale: "Readback detected a denied private or secret-looking pattern in a supported artifact.", EvidenceRefs: refs}
	}
	if generalization != "generalizable" {
		return ImprovementTarget{Code: "needs_held_out_labels", Rationale: "The run is sample-bound, private, temp, unknown, or explicitly non-generalizable.", EvidenceRefs: refs}
	}
	if value, ok := model.metrics["artifact_match_coverage"]; ok && value < 1 {
		return ImprovementTarget{Code: "needs_source_enrichment", Rationale: "Artifact coverage is incomplete, so source meaning is still missing enrichment.", EvidenceRefs: refs}
	}
	if value, ok := model.metrics["evidence_ready_atom_ratio"]; ok && value < 1 {
		return ImprovementTarget{Code: "needs_evidence_readiness", Rationale: "Not all eval-counted atoms are evidence-ready.", EvidenceRefs: refs}
	}
	return ImprovementTarget{Code: "ready_for_next_pressure_run", Rationale: "No higher-priority readback blocker was found; rerun the next pressure/eval slice with comparable baseline.", EvidenceRefs: refs}
}

func writeSummary(outRoot string, summary Summary, protectedRoots []string) error {
	if strings.TrimSpace(outRoot) == "" {
		return errors.New("missing output root")
	}
	root, err := filepath.Abs(outRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	dir := filepath.Join(root, DirName)
	if err := rejectSymlinkEscape(root, dir, protectedRoots); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := rejectSymlinkEscape(root, dir, protectedRoots); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "readback-summary.json"), summary); err != nil {
		return err
	}
	if summary.Comparison != nil {
		if err := writeJSON(filepath.Join(dir, "comparison-summary.json"), summary.Comparison); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "readback-report.md"), []byte(markdownReport(summary)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "chain-capture-draft.md"), []byte(chainDraft(summary)), 0o644); err != nil {
		return err
	}
	return nil
}

func rejectSymlinkEscape(root, dir string, protectedRoots []string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	realRoot, err = filepath.Abs(realRoot)
	if err != nil {
		return err
	}
	realDir := dir
	if _, err := os.Lstat(dir); err == nil {
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return err
		}
		realDir = resolved
	} else if os.IsNotExist(err) {
		rel, relErr := filepath.Rel(root, dir)
		if relErr != nil {
			return relErr
		}
		realDir = filepath.Join(realRoot, rel)
	} else {
		return err
	}
	realDir, err = filepath.Abs(realDir)
	if err != nil {
		return err
	}
	if !isSameOrInside(realRoot, realDir) {
		return fmt.Errorf("eval readback output escapes output root")
	}
	for _, protectedRoot := range protectedRoots {
		if strings.TrimSpace(protectedRoot) == "" {
			continue
		}
		realProtected, err := filepath.EvalSymlinks(protectedRoot)
		if err != nil {
			continue
		}
		realProtected, err = filepath.Abs(realProtected)
		if err != nil {
			continue
		}
		if isSameOrInside(realProtected, realDir) {
			return fmt.Errorf("protected output root")
		}
	}
	return nil
}

func isSameOrInside(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func markdownReport(summary Summary) string {
	var b strings.Builder
	b.WriteString("# Eval Readback\n\n")
	b.WriteString(fmt.Sprintf("- Artifacts: %d\n", summary.ArtifactCount))
	b.WriteString(fmt.Sprintf("- Sample status: %s\n", summary.SampleStatus))
	b.WriteString(fmt.Sprintf("- Generalization: %s\n", summary.GeneralizationStatus))
	b.WriteString(fmt.Sprintf("- Improvement: %s\n\n", summary.ImprovementStatus))
	if summary.Comparison != nil {
		b.WriteString("## Comparison\n\n")
		b.WriteString(fmt.Sprintf("- Status: %s\n", summary.Comparison.Status))
		if len(summary.Comparison.ReasonCodes) > 0 {
			b.WriteString(fmt.Sprintf("- Reasons: %s\n", strings.Join(summary.Comparison.ReasonCodes, ", ")))
		}
		if len(summary.Comparison.MetricDeltas) > 0 {
			b.WriteString("- Metric deltas:\n")
			keys := make([]string, 0, len(summary.Comparison.MetricDeltas))
			for key := range summary.Comparison.MetricDeltas {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				b.WriteString(fmt.Sprintf("  - `%s`: %.4f\n", key, summary.Comparison.MetricDeltas[key]))
			}
		}
		b.WriteString("\n")
	} else {
		b.WriteString("## Comparison\n\nNo baseline was supplied, so improvement is not evaluated and the improvement claim remains blocked.\n\n")
	}
	b.WriteString("## Top improvement target\n\n")
	b.WriteString(fmt.Sprintf("`%s`: %s\n\n", summary.TopImprovementTarget.Code, summary.TopImprovementTarget.Rationale))
	b.WriteString("## Claim gates\n\n")
	for _, gate := range summary.ClaimGates {
		b.WriteString(fmt.Sprintf("- `%s`: %s — %s\n", gate.Gate, gate.Status, gate.ClaimImpact))
	}
	b.WriteString("\n## Safe artifact refs\n\n")
	for _, ref := range summary.SafeArtifactRefs {
		b.WriteString(fmt.Sprintf("- `%s`\n", ref))
	}
	return b.String()
}

func chainDraft(summary Summary) string {
	var b strings.Builder
	b.WriteString("WP-35 eval readback result: ")
	b.WriteString(summary.ImprovementStatus)
	b.WriteString(". Generalization: ")
	b.WriteString(summary.GeneralizationStatus)
	b.WriteString(". Blocked claims: ")
	blocked := []string{}
	for _, gate := range summary.ClaimGates {
		if gate.Status == "blocked" || gate.Status == "fail" {
			blocked = append(blocked, gate.Gate)
		}
	}
	b.WriteString(strings.Join(blocked, ", "))
	b.WriteString(". Next target: ")
	b.WriteString(summary.TopImprovementTarget.Code)
	b.WriteString(". Proof refs: ")
	b.WriteString(strings.Join(firstRefs(summary.SafeArtifactRefs), ", "))
	b.WriteString(".")
	return b.String()
}

func sampleStatusFor(root string) string {
	clean := filepath.ToSlash(root)
	switch {
	case strings.Contains(clean, "/private/tmp/"):
		return "private_runtime"
	case strings.Contains(clean, "/temp/") || strings.HasSuffix(clean, "/temp"):
		return "temp_runtime"
	case strings.Contains(clean, "/testdata/"):
		return "fixture"
	default:
		return "unknown"
	}
}

func safeRootLabel(root string) string {
	return stableID("root", []string{filepath.ToSlash(root)})
}

func stableID(prefix string, parts []string) string {
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(prefix + ":" + strings.Join(parts, "|")))
	return strings.Trim(prefix, "-_ ") + "-" + hex.EncodeToString(sum[:])[:12]
}

func firstRefs(refs []string) []string {
	out := append([]string(nil), refs...)
	sort.Strings(out)
	if len(out) > 3 {
		return out[:3]
	}
	return out
}

func containsDeniedString(value string) bool {
	lower := strings.ToLower(value)
	denied := []string{"/private/tmp/", "/users/", "young human club dropbox", "slack.com/archives", "xoxb-", "xoxp-", "api_key=", "bearer "}
	for _, item := range denied {
		if strings.Contains(lower, item) {
			return true
		}
	}
	return false
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	default:
		return 0, false
	}
}

func boolValue(value any) (bool, bool) {
	typed, ok := value.(bool)
	return typed, ok
}

func stringValue(value any) string {
	typed, _ := value.(string)
	return strings.TrimSpace(typed)
}
