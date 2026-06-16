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
	summary, err := BuildSummary(inputRoot, options)
	if err != nil {
		return Summary{}, err
	}
	if err := Write(outRoot, summary, options.ProtectedRoots); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func BuildSummary(inputRoot string, options Options) (Summary, error) {
	model, err := buildModel(inputRoot)
	if err != nil {
		return Summary{}, err
	}
	summary := summarize(model)
	if strings.TrimSpace(options.BaselineRoot) != "" {
		baseline, err := buildBaselineModel(options.BaselineRoot)
		if err != nil {
			return Summary{}, fmt.Errorf("read baseline: %w", err)
		}
		comparison := compareModels(baseline, model)
		summary.BaselineRootLabel = baseline.rootLabel
		summary.BaselineArtifactRefs = prefixedArtifactRefs("baseline", artifactRefs(baseline.artifacts))
		summary.BaselineArtifacts = baseline.artifacts
		summary.Comparison = &comparison
		summary.ImprovementStatus = comparison.Status
		rebuildClaimGates(&summary)
	}
	return summary, nil
}

func ApplyBaseline(summary Summary, baselineRoot string) (Summary, error) {
	summary = RefreshSummary(summary)
	if strings.TrimSpace(baselineRoot) == "" {
		return summary, nil
	}
	baseline, err := buildBaselineModel(baselineRoot)
	if err != nil {
		return Summary{}, fmt.Errorf("read baseline: %w", err)
	}
	current := modelFromSummary(summary)
	comparison := compareModels(baseline, current)
	summary.BaselineRootLabel = baseline.rootLabel
	summary.BaselineArtifactRefs = prefixedArtifactRefs("baseline", artifactRefs(baseline.artifacts))
	summary.BaselineArtifacts = baseline.artifacts
	summary.Comparison = &comparison
	summary.ImprovementStatus = comparison.Status
	rebuildClaimGates(&summary)
	return summary, nil
}

func ApplyBaselineSummary(summary Summary, baselineSummary Summary) Summary {
	summary = RefreshSummary(summary)
	baselineSummary = RefreshSummary(baselineSummary)
	current := modelFromSummary(summary)
	baseline := modelFromSummary(baselineSummary)
	comparison := compareModels(baseline, current)
	summary.BaselineRootLabel = baseline.rootLabel
	summary.BaselineArtifactRefs = prefixedArtifactRefs("baseline", artifactRefs(baseline.artifacts))
	summary.BaselineArtifacts = baseline.artifacts
	summary.Comparison = &comparison
	summary.ImprovementStatus = comparison.Status
	rebuildClaimGates(&summary)
	return summary
}

func RefreshSummary(summary Summary) Summary {
	current := modelFromSummary(summary)
	refreshed := summarize(current)
	if len(summary.BaselineArtifacts) == 0 {
		if summary.Comparison != nil {
			refreshed.BaselineRootLabel = summary.BaselineRootLabel
			refreshed.BaselineArtifactRefs = append([]string{}, summary.BaselineArtifactRefs...)
			refreshed.BaselineArtifacts = append([]ArtifactEvidence{}, summary.BaselineArtifacts...)
			comparison := *summary.Comparison
			refreshed.Comparison = &comparison
			refreshed.ImprovementStatus = summary.ImprovementStatus
			rebuildClaimGates(&refreshed)
		}
		return refreshed
	}
	baseline := modelFromArtifacts(summary.BaselineRootLabel, summary.SampleStatus, summary.BaselineArtifacts)
	comparison := compareModels(baseline, current)
	refreshed.BaselineRootLabel = summary.BaselineRootLabel
	refreshed.BaselineArtifactRefs = prefixedArtifactRefs("baseline", artifactRefs(baseline.artifacts))
	refreshed.BaselineArtifacts = baseline.artifacts
	refreshed.Comparison = &comparison
	refreshed.ImprovementStatus = comparison.Status
	rebuildClaimGates(&refreshed)
	return refreshed
}

func modelFromSummary(summary Summary) readbackModel {
	model := modelFromArtifacts(summary.InputRootLabel, summary.SampleStatus, summary.Artifacts)
	if summary.ReplayBaseline.Status == "blocked" {
		model.flags["replay_baseline_blocked"] = true
		model.replayBaselineReasonCodes = append([]string{}, summary.ReplayBaseline.ReasonCodes...)
	}
	return model
}

func modelFromArtifacts(rootLabel, sampleStatus string, artifacts []ArtifactEvidence) readbackModel {
	model := readbackModel{
		rootLabel:     rootLabel,
		sampleStatus:  sampleStatus,
		metrics:       map[string]float64{},
		flags:         map[string]bool{},
		fingerprints:  map[string]string{},
		artifactTypes: map[string]bool{},
		artifacts:     append([]ArtifactEvidence{}, artifacts...),
	}
	for _, artifact := range model.artifacts {
		mergeModelEvidence(&model, artifact)
	}
	return model
}

func Write(outRoot string, summary Summary, protectedRoots []string) error {
	return writeSummary(outRoot, summary, protectedRoots)
}

func LoadSummary(path string) (Summary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Summary{}, err
	}
	if containsDeniedString(string(data)) {
		return Summary{}, errors.New("readback summary contains unsafe private or secret pattern")
	}
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return Summary{}, err
	}
	if summary.SchemaVersion != SummarySchemaVersion {
		return Summary{}, fmt.Errorf("unsupported readback summary schema: %s", summary.SchemaVersion)
	}
	return summary, nil
}

func ValidateOutputPath(root, candidate string, protectedRoots []string) error {
	return rejectSymlinkEscape(root, candidate, protectedRoots)
}

type readbackModel struct {
	root                      string
	rootLabel                 string
	sampleStatus              string
	artifacts                 []ArtifactEvidence
	guardrails                Guardrails
	metrics                   map[string]float64
	flags                     map[string]bool
	fingerprints              map[string]string
	artifactTypes             map[string]bool
	replayBaselineReasonCodes []string
	segmentSummaryCount       int
	segmentSummarySegments    int
}

func buildBaselineModel(inputRoot string) (readbackModel, error) {
	summary, _, err := LoadSummaryFromRoot(inputRoot)
	if err == nil {
		summary = RefreshSummary(summary)
		return modelFromSummary(summary), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return readbackModel{}, err
	}
	return buildModel(inputRoot)
}

func LoadSummaryFromRoot(input string) (Summary, string, error) {
	info, err := os.Stat(input)
	if err != nil {
		return Summary{}, "", err
	}
	candidates := []string{}
	if !info.IsDir() {
		if filepath.Base(input) == ReadbackSummaryFile {
			summary, err := LoadSummary(input)
			return summary, filepath.ToSlash(filepath.Base(input)), err
		}
		return Summary{}, "", os.ErrNotExist
	}
	for _, rel := range []string{
		ReadbackSummaryFile,
		filepath.Join(DirName, ReadbackSummaryFile),
		filepath.Join("readback", DirName, ReadbackSummaryFile),
		filepath.Join("eval-proof", "readback", DirName, ReadbackSummaryFile),
		filepath.Join("eval-loop-decision", "readback", DirName, ReadbackSummaryFile),
		filepath.Join("eval-loop-decision", DirName, "readback", DirName, ReadbackSummaryFile),
	} {
		candidates = append(candidates, filepath.Join(input, rel))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			summary, err := LoadSummary(candidate)
			if err != nil {
				return Summary{}, "", err
			}
			rel, err := filepath.Rel(input, candidate)
			if err != nil {
				return summary, filepath.ToSlash(candidate), nil
			}
			return summary, filepath.ToSlash(rel), nil
		}
	}
	return Summary{}, "", os.ErrNotExist
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
		if d.Type()&os.ModeSymlink != 0 {
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
		artifactType := artifactTypeFor(root, ref)
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

func artifactTypeFor(root, ref string) string {
	if artifactType := artifactTypeForRef(ref); artifactType != "" {
		return artifactType
	}
	prefix := artifactRootPrefix(root)
	if prefix == "" {
		return ""
	}
	return artifactTypeForRef(filepath.ToSlash(filepath.Join(prefix, ref)))
}

func artifactRootPrefix(root string) string {
	base := filepath.Base(root)
	switch base {
	case "trace", "corpus-pressure", "corpus-graph", "corpus-pressure-loop", "corpus-acceptance", "autonomy-readiness", "link-enrichment", "value-proof", "source-meaning-packet", "corpus-concepts":
		return base
	case "comparison", "requests", "posthog":
		if filepath.Base(filepath.Dir(root)) == "link-enrichment" {
			return filepath.ToSlash(filepath.Join("link-enrichment", base))
		}
	}
	return ""
}

func artifactTypeForRef(ref string) string {
	switch {
	case strings.HasSuffix(ref, "trace/trace-summary.json"):
		return "generic_trace_summary"
	case strings.HasSuffix(ref, "corpus-pressure/pressure-summary.json"):
		return "corpus_pressure_summary"
	case strings.HasSuffix(ref, "corpus-pressure/eval-input.json"):
		return "corpus_pressure_eval_input"
	case strings.HasSuffix(ref, "corpus-pressure/trace-summary.json"):
		return "corpus_pressure_trace_summary"
	case strings.HasSuffix(ref, "corpus-graph/graph-summary.json"):
		return "corpus_graph_summary"
	case strings.HasSuffix(ref, "document-segments/segment-summary.json"):
		return "document_segment_summary"
	case strings.HasSuffix(ref, "semantic-candidates/semantic-summary.json"):
		return "semantic_candidate_summary"
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
	case strings.HasSuffix(ref, "value-proof/value-summary.json"):
		return "value_proof_summary"
	case strings.HasSuffix(ref, "source-meaning-packet/meaning-summary.json"):
		return "source_meaning_packet_summary"
	case strings.HasSuffix(ref, "corpus-concepts/concept-summary.json"):
		return "corpus_concept_summary"
	case strings.HasSuffix(ref, "corpus-concepts/review-records.json"):
		return "corpus_concept_review_records"
	default:
		return ""
	}
}

func readArtifact(path, ref, artifactType string) (ArtifactEvidence, error) {
	if containsDeniedRefString(ref) {
		return ArtifactEvidence{
			Type: artifactType, Ref: sanitizedArtifactRef(ref), Status: "unsafe_or_leaky", ReasonCodes: []string{"unsafe_artifact_ref"},
		}, nil
	}
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
		"corpus_graph_summary":               "corpus-graph-summary/v0.1",
		"document_segment_summary":           "document-segment-summary/v0.1",
		"semantic_candidate_summary":         "semantic-candidate-summary/v0.1",
		"corpus_pressure_loop_summary":       "corpus-pressure-loop-summary/v0.1",
		"corpus_acceptance_benchmark":        "corpus-acceptance-summary/v0.1",
		"autonomy_readiness_report":          "autonomy-readiness-report/v0.1",
		"link_enrichment_loop_summary":       "link-enrichment-loop-summary/v0.1",
		"link_enrichment_comparison_summary": "link-enrichment-comparison/v0.1",
		"link_artifact_requests":             "local-link-artifact-requests/v0.1",
		"link_enrichment_eval_projection":    "mindline-link-enrichment-eval-projection/v0.1",
		"value_proof_summary":                "mindline-value-proof/v0.1",
		"source_meaning_packet_summary":      "source-meaning-packet/v0.1",
		"corpus_concept_summary":             "corpus-concepts/v0.1",
		"corpus_concept_review_records":      "corpus-concept-review-records/v0.1",
	}
	return strings.TrimSpace(schemaVersion) == expected[artifactType]
}

func extractEvidence(raw map[string]any, artifact *ArtifactEvidence) {
	for _, key := range []string{
		"source_count", "candidate_count", "semantic_candidate_count", "observation_count", "evidence_ready_atom_count",
		"processed_source_count", "semantic_observation_count", "document_segment_count", "segment_count",
		"reference_candidate_count", "one_candidate_source_count", "reference_only_source_count",
		"accounted_source_count", "atom_count", "evidence_or_blocker_atom_count", "relation_count",
		"review_burden_count", "missing_link_reduction_ratio", "needs_enrichment_reduction_ratio",
		"missing_link_enrichment_reduction_ratio",
		"url_accounting_coverage", "artifact_match_coverage", "model_error_count",
		"human_review_required_count",
		"eval_counted_count", "evidence_ready_count", "eval_counted_human_review_required_count",
		"eval_counted_model_error_count",
		"review_group_count", "ready_group_count", "needs_review_group_count", "blocked_group_count",
		"proposal_count", "evidence_reference_count", "evidence_or_blocker_group_count",
		"generated_review_group_count", "generated_ready_group_count", "generated_needs_review_group_count",
		"generated_blocked_group_count", "generated_proposal_count", "generated_evidence_reference_count",
		"generated_evidence_or_blocker_group_count", "generated_review_burden_count",
		"concept_count", "generated_concept_count", "cross_source_concept_count", "local_concept_count",
		"needs_review_concept_count", "blocked_concept_count", "concept_review_burden_count",
		"concept_review_count", "cleanup_triage_count", "enrichment_backlog_count", "blocked_diagnostic_count",
		"cross_source_evidence_reference_count", "cross_source_kind_pair_count", "max_concept_count",
		"omitted_concept_count",
		"scale_skipped_source_count", "max_processed_sources", "max_source_bytes", "max_source_segments", "max_source_candidates",
		"max_graph_pair_comparisons", "max_graph_relations", "max_packet_review_groups",
		"graph_pair_comparison_count", "graph_pair_comparison_limit", "graph_relation_candidate_limit",
		"pair_comparison_count", "pair_comparison_limit", "relation_candidate_limit",
		"max_review_group_count", "omitted_atom_count",
		"threshold", "accuracy", "eval_count",
	} {
		if value, ok := numberValue(raw[key]); ok {
			artifact.Metrics[key] = value
		}
	}
	for _, key := range []string{"processed_source_ratio", "source_accounting_ratio", "evidence_ready_atom_ratio", "evidence_or_blocker_ratio", "review_burden_ratio", "candidate_per_processed_source_ratio", "observation_per_segment_ratio", "reference_candidate_ratio", "atom_compression_ratio", "relation_review_compression_ratio", "evidence_or_blocker_group_ratio", "generated_atom_compression_ratio", "generated_relation_review_compression_ratio", "generated_evidence_or_blocker_group_ratio", "generated_review_burden_ratio", "concept_review_burden_ratio", "atom_coverage_ratio", "cross_source_atom_ratio"} {
		if value, ok := numberValue(raw[key]); ok {
			artifact.Metrics[key] = value
		}
	}
	extractSemanticReadinessEvidence(raw, artifact)
	if status := stringValue(raw["scale_status"]); status != "" {
		artifact.Flags[status] = true
		if status == "scale_partial" {
			artifact.ReasonCodes = appendUnique(artifact.ReasonCodes, "scale_partial")
		}
	}
	if reasons, ok := raw["scale_reason_codes"].([]any); ok {
		for _, item := range reasons {
			if reason := stringValue(item); reason != "" {
				artifact.ReasonCodes = appendUnique(artifact.ReasonCodes, reason)
			}
		}
	}
	if budget, ok := raw["scale_budget"].(map[string]any); ok {
		extractEvidence(budget, artifact)
	}
	for _, key := range []string{"ready_for_50_file_pressure", "held_out", "non_generalizable_runtime", "comparable", "dec64_eligible", "no_human_eligible", "suite_valid"} {
		if value, ok := boolValue(raw[key]); ok {
			artifact.Flags[key] = value
		}
	}
	if stringValue(raw["threshold_status"]) == "eligible" {
		artifact.Flags["threshold_eligible"] = true
	}
	for _, key := range []string{"corpus_fingerprint", "command_config_fingerprint", "replay_fingerprint", "graph_replay_fingerprint"} {
		if value := stringValue(raw[key]); value != "" {
			artifact.Fingerprints[key] = value
		}
	}
	for _, key := range []string{"baseline_corpus_fingerprint", "enriched_corpus_fingerprint", "baseline_config_fingerprint", "enriched_config_fingerprint"} {
		if value := stringValue(raw[key]); value != "" {
			artifact.Fingerprints[key] = value
		}
	}
	if value := stringValue(raw["baseline_corpus_fingerprint"]); value != "" {
		artifact.Fingerprints["corpus_fingerprint"] = value
	}
	if value := stringValue(raw["baseline_config_fingerprint"]); value != "" {
		artifact.Fingerprints["command_config_fingerprint"] = value
	}
	if value := stringValue(raw["pressure_replay_fingerprint"]); value != "" {
		artifact.Fingerprints["pressure_replay_fingerprint"] = value
	}
	if guardrails, ok := raw["guardrails"].(map[string]any); ok {
		extractGuardrails(guardrails, artifact)
	}
	if safetyCounters, ok := raw["safety_counters"].(map[string]any); ok {
		extractSafetyCounters(safetyCounters, artifact)
	}
	if comparison, ok := raw["comparison"].(map[string]any); ok {
		extractEvidence(comparison, artifact)
	}
	if requestSummary, ok := raw["request_summary"].(map[string]any); ok {
		extractEvidence(requestSummary, artifact)
	}
	if summary, ok := raw["summary"].(map[string]any); ok {
		extractEvidence(summary, artifact)
	}
	if counts, ok := raw["counts"].(map[string]any); ok {
		extractEvidence(counts, artifact)
	}
	extractCorpusConceptReviewProgressEvidence(raw, artifact)
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
				"safety_browser_calls", "safety_slack_api_calls",
				"safety_destination_writes", "safety_product_brain_writes", "safety_tolaria_writes", "safety_auto_accepts",
				"safety_committed_private_artifacts",
			} {
				if value, ok := numberValue(props[key]); ok {
					artifact.Metrics[key] = value
				}
			}
			if value, ok := numberValue(props["safety_no_human_claims"]); ok {
				artifact.Metrics["safety_no_human_claims"] = value
			} else if value, ok := boolValue(props["safety_no_human_claims"]); ok {
				artifact.Flags["safety_no_human_claims"] = value
			}
		}
	}
}

func extractCorpusConceptReviewProgressEvidence(raw map[string]any, artifact *ArtifactEvidence) {
	progressByKind := map[string]map[string]float64{}
	hasProgress := false
	if progress, ok := raw["review_work_kind_progress"].(map[string]any); ok {
		hasProgress = true
		for kind, item := range progress {
			bucket, ok := item.(map[string]any)
			if !ok {
				continue
			}
			metrics := progressByKind[kind]
			if metrics == nil {
				metrics = map[string]float64{}
				progressByKind[kind] = metrics
			}
			for _, key := range []string{"total_count", "reviewed_count", "remaining_count"} {
				if value, ok := numberValue(bucket[key]); ok {
					metrics[key] = value
				}
			}
			if choices, ok := bucket["choice_counts"].(map[string]any); ok {
				for choice, rawCount := range choices {
					if value, ok := numberValue(rawCount); ok {
						metrics["choice_"+choice+"_count"] = value
					}
				}
			}
		}
	}
	if records, ok := raw["records"].([]any); ok && !hasProgress {
		seen := map[string]bool{}
		for _, item := range records {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			conceptID := stringValue(record["concept_id"])
			if conceptID == "" || seen[conceptID] {
				continue
			}
			seen[conceptID] = true
			kind := stringValue(record["review_work_kind"])
			if kind == "" {
				kind = "concept_review"
			}
			metrics := progressByKind[kind]
			if metrics == nil {
				metrics = map[string]float64{}
				progressByKind[kind] = metrics
			}
			metrics["reviewed_count"]++
			if choice := stringValue(record["choice"]); choice != "" {
				metrics["choice_"+choice+"_count"]++
			}
		}
	}
	for kind, metrics := range progressByKind {
		safeKind := strings.ReplaceAll(kind, "-", "_")
		for key, value := range metrics {
			artifact.Metrics[safeKind+"_"+key] = value
		}
	}
}

func extractGuardrails(guardrails map[string]any, artifact *ArtifactEvidence) {
	for _, key := range []string{"network_fetches", "hosted_telemetry_exports", "hosted_inference_calls", "browser_calls", "slack_api_calls", "destination_writes", "product_brain_writes", "tolaria_writes", "auto_accepts", "committed_private_artifacts"} {
		if value, ok := numberValue(guardrails[key]); ok {
			artifact.Metrics["guardrail_"+key] = value
		}
	}
	if value, ok := numberValue(guardrails["no_human_claims"]); ok {
		artifact.Metrics["guardrail_no_human_claims"] = value
	} else if value, ok := boolValue(guardrails["no_human_claims"]); ok {
		artifact.Flags["guardrail_no_human_claims"] = value
	}
}

func extractSemanticReadinessEvidence(raw map[string]any, artifact *ArtifactEvidence) {
	if status := stringValue(raw["semantic_readiness_status"]); status != "" {
		artifact.Flags["semantic_readiness_"+status] = true
	}
	if sources, ok := raw["sources"].([]any); ok {
		processed := 0
		oneCandidate := 0
		referenceOnly := 0
		referenceCandidates := 0
		for _, item := range sources {
			source, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if stringValue(source["state"]) != "processed" {
				continue
			}
			processed++
			candidates := intNumberValue(source["candidate_count"])
			if candidates == 1 {
				oneCandidate++
			}
			if counts, ok := source["candidate_kind_counts"].(map[string]any); ok {
				reference := intNumberValue(counts["reference_candidate"])
				referenceCandidates += reference
				if candidates > 0 && reference == candidates {
					referenceOnly++
				}
			}
		}
		if processed > 0 {
			artifact.Metrics["processed_source_count"] = float64(processed)
			artifact.Metrics["one_candidate_source_count"] = float64(oneCandidate)
			artifact.Metrics["reference_only_source_count"] = float64(referenceOnly)
		}
		if referenceCandidates > 0 {
			artifact.Metrics["reference_candidate_count"] = float64(referenceCandidates)
		}
	}
	if counts, ok := raw["candidate_kind_counts"].(map[string]any); ok {
		if reference := intNumberValue(counts["reference_candidate"]); reference > 0 {
			artifact.Metrics["reference_candidate_count"] = float64(reference)
		}
	}
}

func intNumberValue(value any) int {
	if number, ok := numberValue(value); ok {
		return int(number)
	}
	return 0
}

func extractSafetyCounters(safetyCounters map[string]any, artifact *ArtifactEvidence) {
	for _, key := range []string{"destination_writes", "auto_accepts", "no_human_claims", "committed_private_artifacts"} {
		if value, ok := numberValue(safetyCounters[key]); ok {
			artifact.Metrics["safety_"+key] = value
		}
	}
}

func mergeModelEvidence(model *readbackModel, artifact ArtifactEvidence) {
	model.artifactTypes[artifact.Type] = true
	if artifact.Status == "unsafe_or_leaky" {
		model.flags["unsafe_or_leaky"] = true
	}
	if artifact.Type == "document_segment_summary" {
		model.segmentSummaryCount++
		if value, ok := artifact.Metrics["segment_count"]; ok {
			model.segmentSummarySegments += int(value)
			model.metrics["document_segment_count_from_summaries"] = float64(model.segmentSummarySegments)
		}
	}
	if artifact.Type == "semantic_candidate_summary" {
		sourceCount, hasSourceCount := artifact.Metrics["source_count"]
		candidateCount, hasCandidateCount := artifact.Metrics["candidate_count"]
		referenceCandidateCount, hasReferenceCandidateCount := artifact.Metrics["reference_candidate_count"]
		if hasSourceCount {
			model.metrics["processed_source_count_from_summaries"] += sourceCount
			if hasCandidateCount && candidateCount == sourceCount {
				model.metrics["one_candidate_source_count_from_summaries"] += sourceCount
			}
			if hasCandidateCount && hasReferenceCandidateCount && referenceCandidateCount == candidateCount {
				model.metrics["reference_only_source_count_from_summaries"] += sourceCount
			}
		}
		if value, ok := artifact.Metrics["observation_count"]; ok {
			model.metrics["semantic_observation_count_from_summaries"] += value
		}
		if value, ok := artifact.Metrics["candidate_count"]; ok {
			model.metrics["semantic_candidate_count_from_summaries"] += value
		}
		if value, ok := artifact.Metrics["reference_candidate_count"]; ok {
			model.metrics["reference_candidate_count_from_summaries"] += value
		}
	}
	for key, value := range artifact.Metrics {
		if !mergeArtifactMetricIntoModel(artifact.Type, key) {
			continue
		}
		model.metrics[key] = value
		switch key {
		case "guardrail_network_fetches", "safety_network_fetches":
			model.guardrails.NetworkFetches = maxInt(model.guardrails.NetworkFetches, int(value))
		case "guardrail_hosted_telemetry_exports", "safety_hosted_telemetry_exports":
			model.guardrails.HostedTelemetryExports = maxInt(model.guardrails.HostedTelemetryExports, int(value))
		case "guardrail_hosted_inference_calls", "safety_hosted_inference_calls":
			model.guardrails.HostedInferenceCalls = maxInt(model.guardrails.HostedInferenceCalls, int(value))
		case "guardrail_browser_calls", "safety_browser_calls":
			model.guardrails.BrowserCalls = maxInt(model.guardrails.BrowserCalls, int(value))
		case "guardrail_slack_api_calls", "safety_slack_api_calls":
			model.guardrails.SlackAPICalls = maxInt(model.guardrails.SlackAPICalls, int(value))
		case "guardrail_destination_writes", "safety_destination_writes":
			model.guardrails.DestinationWrites = maxInt(model.guardrails.DestinationWrites, int(value))
		case "guardrail_product_brain_writes", "safety_product_brain_writes":
			model.guardrails.ProductBrainWrites = maxInt(model.guardrails.ProductBrainWrites, int(value))
		case "guardrail_tolaria_writes", "safety_tolaria_writes":
			model.guardrails.TolariaWrites = maxInt(model.guardrails.TolariaWrites, int(value))
		case "guardrail_auto_accepts", "safety_auto_accepts":
			model.guardrails.AutoAccepts = maxInt(model.guardrails.AutoAccepts, int(value))
		case "guardrail_no_human_claims", "safety_no_human_claims":
			if value > 0 {
				model.guardrails.NoHumanClaims = true
			}
		case "guardrail_committed_private_artifacts", "safety_committed_private_artifacts":
			model.guardrails.CommittedPrivateArtifacts = maxInt(model.guardrails.CommittedPrivateArtifacts, int(value))
		}
	}
	for key, value := range artifact.Flags {
		if key == "comparable" && !value {
			model.flags["artifact_not_comparable"] = true
		}
		if value {
			model.flags[key] = true
		} else if _, exists := model.flags[key]; !exists {
			model.flags[key] = false
		}
		if key == "guardrail_no_human_claims" && value {
			model.guardrails.NoHumanClaims = true
		}
	}
	for key, value := range artifact.Fingerprints {
		if existing, exists := model.fingerprints[key]; exists {
			if existing != value {
				model.flags["conflicting_"+key] = true
			}
		} else {
			model.fingerprints[key] = value
		}
	}
}

func mergeArtifactMetricIntoModel(artifactType, metric string) bool {
	if artifactType == "source_meaning_packet_summary" {
		switch metric {
		case "review_burden_count", "review_burden_ratio":
			return false
		}
	}
	return true
}

func summarize(model readbackModel) Summary {
	typeCounts := map[string]int{}
	for _, artifact := range model.artifacts {
		typeCounts[artifact.Type]++
	}
	refs := artifactRefs(model.artifacts)
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
	summary.SemanticReadiness = semanticReadinessForModel(model)
	summary.TopImprovementTarget = chooseTarget(model, summary.GeneralizationStatus)
	summary.RerunInstructions = []string{"rerun the source command after addressing " + summary.TopImprovementTarget.Code + ", then run eval readback with --baseline pointing to this run"}
	summary.ReplayBaseline = replayBaselineForSummary(summary)
	rebuildClaimGates(&summary)
	return summary
}

func replayBaselineForSummary(summary Summary) ReplayBaseline {
	artifactTypes := make([]string, 0, len(summary.ArtifactTypeCounts))
	for artifactType := range summary.ArtifactTypeCounts {
		artifactTypes = append(artifactTypes, artifactType)
	}
	sort.Strings(artifactTypes)
	baseline := ReplayBaseline{
		Status:           "ready",
		ArtifactTypes:    artifactTypes,
		SafeArtifactRefs: append([]string{}, summary.SafeArtifactRefs...),
		RerunInstruction: "rerun the same source command with the same corpus and command configuration, then use this readback output as --baseline for eval proof-gate or eval loop-decision",
	}
	corpus := map[string]bool{}
	config := map[string]bool{}
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsupported_schema" {
			baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "unsupported_schema")
		}
		if artifact.Status == "unsafe_or_leaky" {
			baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "unsafe_or_leaky")
		}
		if value := artifact.Fingerprints["corpus_fingerprint"]; value != "" {
			corpus[value] = true
		}
		if value := artifact.Fingerprints["command_config_fingerprint"]; value != "" {
			config[value] = true
		}
	}
	switch len(corpus) {
	case 0:
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "missing_corpus_fingerprint")
	case 1:
		for value := range corpus {
			baseline.CorpusFingerprint = value
		}
	default:
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "conflicting_corpus_fingerprints")
	}
	switch len(config) {
	case 0:
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "missing_command_config_fingerprint")
	case 1:
		for value := range config {
			baseline.CommandConfigFingerprint = value
		}
	default:
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "conflicting_command_config_fingerprints")
	}
	hasSupportedArtifact := false
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "detected" {
			hasSupportedArtifact = true
			break
		}
	}
	if !hasSupportedArtifact {
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "missing_supported_artifacts")
	}
	if !hasSideEffectEvidence(&summary) {
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "missing_side_effect_evidence")
	}
	if hasSideEffectCounter(&summary) {
		baseline.ReasonCodes = appendUnique(baseline.ReasonCodes, "side_effect_counter_nonzero")
	}
	if len(baseline.ReasonCodes) > 0 {
		baseline.Status = "blocked"
	}
	return baseline
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func rebuildClaimGates(summary *Summary) {
	gates := []ClaimGate{
		{Gate: "artifact_presence", Status: "pass", EvidenceRefs: proofArtifactRefs(summary), ClaimImpact: "readback has local evidence to inspect"},
	}
	unsafe := hasUnsafeArtifact(summary)
	unsupported := hasUnsupportedArtifact(summary)
	if unsafe {
		gates = append(gates, ClaimGate{Gate: "privacy_safe_readback", Status: "fail", ReasonCodes: []string{"unsafe_or_leaky"}, EvidenceRefs: unsafeArtifactRefs(summary), ClaimImpact: "blocks improvement and Chain proof claims until unsafe artifacts are removed or redacted"})
	} else {
		gates = append(gates, ClaimGate{Gate: "privacy_safe_readback", Status: "pass", ClaimImpact: "readback output did not detect unsafe supported artifacts"})
	}
	if summary.GeneralizationStatus == "generalizable" {
		gates = append(gates, ClaimGate{Gate: "generalization_claim", Status: "pass", ClaimImpact: "sample can support its bounded generalization claim"})
	} else {
		gates = append(gates, ClaimGate{Gate: "generalization_claim", Status: "blocked", ReasonCodes: []string{"sample_bound_or_non_held_out"}, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks broad product, DEC-64, or no-human claims"})
	}
	if summary.SemanticReadiness.Status == "blocked" {
		gates = append(gates, ClaimGate{Gate: "semantic_readiness", Status: "blocked", ReasonCodes: append([]string{"semantic_readiness_blocked"}, summary.SemanticReadiness.ReasonCodes...), EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "source intake may have succeeded, but semantic value is not proven"})
	} else if summary.SemanticReadiness.Status == "ready" {
		gates = append(gates, ClaimGate{Gate: "semantic_readiness", Status: "pass", EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "semantic-density counters did not detect reference-only collapse"})
	} else {
		gates = append(gates, ClaimGate{Gate: "semantic_readiness", Status: "not_evaluated", ReasonCodes: summary.SemanticReadiness.ReasonCodes, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "semantic readiness was not evaluated for this artifact shape or sample size"})
	}
	improvementStatus := summary.ImprovementStatus
	if summary.SemanticReadiness.Status == "blocked" {
		gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: append([]string{"semantic_readiness_blocked"}, summary.SemanticReadiness.ReasonCodes...), EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks improvement claim until semantic extraction value is proven"})
	} else if summary.SemanticReadiness.Status == "not_evaluated" && !stringListContains(summary.SemanticReadiness.ReasonCodes, "insufficient_processed_sources") {
		gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: append([]string{"semantic_readiness_not_evaluated"}, summary.SemanticReadiness.ReasonCodes...), EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks improvement claim until semantic-density evidence is present or reconstructable"})
	} else if summaryHasFlag(summary, "scale_partial") {
		gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"scale_partial"}, EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "blocks improvement claim until scale capacity limits are resolved"})
	} else {
		switch improvementStatus {
		case "improved":
			switch {
			case unsafe:
				gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"unsafe_or_leaky"}, EvidenceRefs: unsafeArtifactRefs(summary), ClaimImpact: "blocks improvement claim until readback evidence is privacy-safe"})
			case unsupported:
				gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"unsupported_schema"}, EvidenceRefs: unsupportedArtifactRefs(summary), ClaimImpact: "blocks improvement claim until supported-looking artifacts use known schemas"})
			default:
				gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "pass", ClaimImpact: "current run improved against a comparable baseline"})
			}
		case "unchanged", "regressed":
			gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "fail", ReasonCodes: []string{improvementStatus}, ClaimImpact: "blocks improvement claim"})
		case "not_comparable":
			reasons := []string{"not_comparable"}
			if summary.Comparison != nil {
				reasons = append(reasons, summary.Comparison.ReasonCodes...)
			}
			gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: reasons, ClaimImpact: "blocks improvement claim"})
		default:
			gates = append(gates, ClaimGate{Gate: "improvement_claim", Status: "blocked", ReasonCodes: []string{"missing_baseline"}, ClaimImpact: "blocks improvement claim until comparable baseline is supplied"})
		}
	}
	scalePartial := summaryHasFlag(summary, "scale_partial")
	if hasDEC64ThresholdProof(summary) && hasSideEffectEvidence(summary) && !hasSideEffectCounter(summary) && !unsafe && !unsupported && !scalePartial {
		gates = append(gates, ClaimGate{Gate: "dec64_no_human_claim", Status: "pass", EvidenceRefs: firstRefs(summary.SafeArtifactRefs), ClaimImpact: "held-out threshold proof supports bounded no-human readiness claim"})
	} else {
		gates = append(gates, ClaimGate{Gate: "dec64_no_human_claim", Status: "blocked", ReasonCodes: dec64BlockedReasonCodes(summary, unsafe, unsupported), ClaimImpact: "blocks no-human autonomy readiness claim"})
	}
	if hasSideEffectCounter(summary) {
		gates = append(gates, ClaimGate{Gate: "side_effect_claim", Status: "fail", ReasonCodes: []string{"guardrail_counter_nonzero"}, ClaimImpact: "blocks safety claim"})
	} else if !hasSideEffectEvidence(summary) {
		gates = append(gates, ClaimGate{Gate: "side_effect_claim", Status: "blocked", ReasonCodes: []string{"missing_side_effect_evidence"}, ClaimImpact: "blocks safety claim until artifacts expose guardrail counters"})
	} else {
		gates = append(gates, ClaimGate{Gate: "side_effect_claim", Status: "pass", ClaimImpact: "readback found no prohibited side-effect counters"})
	}
	if summary.TopImprovementTarget.Code != "" {
		gates = append(gates, ClaimGate{Gate: "next_target", Status: "pass", EvidenceRefs: summary.TopImprovementTarget.EvidenceRefs, ClaimImpact: "next improvement target is explicit"})
	}
	summary.ClaimGates = gates
}

func summaryHasFlag(summary *Summary, flag string) bool {
	for _, artifact := range summary.Artifacts {
		if artifact.Flags[flag] {
			return true
		}
	}
	return false
}

func dec64BlockedReasonCodes(summary *Summary, unsafe bool, unsupported bool) []string {
	reasonCodes := []string{}
	if !hasDEC64ThresholdProof(summary) {
		reasonCodes = append(reasonCodes, "held_out_threshold_not_proven")
	}
	if !hasSideEffectEvidence(summary) {
		reasonCodes = append(reasonCodes, "missing_side_effect_evidence")
	}
	if hasSideEffectCounter(summary) {
		reasonCodes = append(reasonCodes, "guardrail_counter_nonzero")
	}
	if summaryHasFlag(summary, "scale_partial") {
		reasonCodes = append(reasonCodes, "scale_partial")
	}
	if unsafe {
		reasonCodes = append(reasonCodes, "unsafe_or_leaky")
	}
	if unsupported {
		reasonCodes = append(reasonCodes, "unsupported_schema")
	}
	if len(reasonCodes) == 0 {
		return []string{"held_out_threshold_not_proven"}
	}
	return reasonCodes
}

func hasUnsafeArtifact(summary *Summary) bool {
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsafe_or_leaky" {
			return true
		}
	}
	for _, artifact := range summary.BaselineArtifacts {
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
	for _, artifact := range summary.BaselineArtifacts {
		if artifact.Status == "unsupported_schema" {
			return true
		}
	}
	return false
}

func artifactRefs(artifacts []ArtifactEvidence) []string {
	refs := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		refs = append(refs, artifact.Ref)
	}
	sort.Strings(refs)
	return refs
}

func prefixedArtifactRefs(prefix string, refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, filepath.ToSlash(filepath.Join(prefix, ref)))
	}
	sort.Strings(out)
	return out
}

func proofArtifactRefs(summary *Summary) []string {
	refs := append([]string{}, summary.SafeArtifactRefs...)
	refs = append(refs, summary.BaselineArtifactRefs...)
	return firstRefs(refs)
}

func unsafeArtifactRefs(summary *Summary) []string {
	refs := []string{}
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsafe_or_leaky" {
			refs = append(refs, artifact.Ref)
		}
	}
	for _, artifact := range summary.BaselineArtifacts {
		if artifact.Status == "unsafe_or_leaky" {
			refs = append(refs, filepath.ToSlash(filepath.Join("baseline", artifact.Ref)))
		}
	}
	return firstRefs(refs)
}

func unsupportedArtifactRefs(summary *Summary) []string {
	refs := []string{}
	for _, artifact := range summary.Artifacts {
		if artifact.Status == "unsupported_schema" {
			refs = append(refs, artifact.Ref)
		}
	}
	for _, artifact := range summary.BaselineArtifacts {
		if artifact.Status == "unsupported_schema" {
			refs = append(refs, filepath.ToSlash(filepath.Join("baseline", artifact.Ref)))
		}
	}
	return firstRefs(refs)
}

func hasSideEffectEvidence(summary *Summary) bool {
	present := map[string]bool{}
	autonomyPresent := map[string]bool{}
	hasAutonomyReport := false
	hasLinkEnrichmentSafetyArtifact := false
	for _, artifact := range summary.Artifacts {
		if artifact.Type == "autonomy_readiness_report" {
			hasAutonomyReport = true
		}
		if isLinkEnrichmentSafetyArtifact(artifact.Type) {
			hasLinkEnrichmentSafetyArtifact = true
		}
		for key := range artifact.Metrics {
			if name, ok := sideEffectMetricName(key); ok {
				present[name] = true
				if artifact.Type == "autonomy_readiness_report" {
					autonomyPresent[name] = true
				}
			}
		}
		for key := range artifact.Flags {
			if name, ok := sideEffectMetricName(key); ok {
				present[name] = true
				if artifact.Type == "autonomy_readiness_report" {
					autonomyPresent[name] = true
				}
			}
		}
	}
	hasAutonomySafetyEvidence := hasRequiredSideEffectEvidence(autonomyPresent, []string{"destination_writes", "auto_accepts", "no_human_claims", "committed_private_artifacts"})
	hasBaseEvidence := hasRequiredSideEffectEvidence(present, []string{
		"network_fetches", "hosted_telemetry_exports", "hosted_inference_calls", "browser_calls", "slack_api_calls",
		"destination_writes", "product_brain_writes", "tolaria_writes", "auto_accepts", "no_human_claims", "committed_private_artifacts",
	})
	if !hasBaseEvidence && !(hasAutonomyReport && hasAutonomySafetyEvidence) {
		return false
	}
	if hasAutonomyReport && !hasAutonomySafetyEvidence {
		return false
	}
	if hasLinkEnrichmentSafetyArtifact && !hasRequiredSideEffectEvidence(present, []string{"browser_calls", "slack_api_calls"}) {
		return false
	}
	return true
}

func hasRequiredSideEffectEvidence(present map[string]bool, required []string) bool {
	for _, key := range required {
		if !present[key] {
			return false
		}
	}
	return true
}

func isCorpusPressureSafetyArtifact(artifactType string) bool {
	switch artifactType {
	case "corpus_pressure_summary", "corpus_pressure_eval_input", "corpus_pressure_trace_summary", "corpus_pressure_loop_summary", "corpus_acceptance_benchmark":
		return true
	default:
		return false
	}
}

func isLinkEnrichmentSafetyArtifact(artifactType string) bool {
	switch artifactType {
	case "link_enrichment_loop_summary", "link_enrichment_comparison_summary", "link_enrichment_eval_projection":
		return true
	default:
		return false
	}
}

func sideEffectMetricName(metric string) (string, bool) {
	name := strings.TrimPrefix(metric, "guardrail_")
	if name == metric {
		name = strings.TrimPrefix(metric, "safety_")
	}
	if name == metric {
		return "", false
	}
	switch name {
	case "network_fetches", "hosted_telemetry_exports", "hosted_inference_calls", "browser_calls", "slack_api_calls", "destination_writes", "product_brain_writes", "tolaria_writes", "auto_accepts", "no_human_claims", "committed_private_artifacts":
		return name, true
	default:
		return "", false
	}
}

func hasSideEffectCounter(summary *Summary) bool {
	return summary.Guardrails.NetworkFetches > 0 ||
		summary.Guardrails.HostedTelemetryExports > 0 ||
		summary.Guardrails.HostedInferenceCalls > 0 ||
		summary.Guardrails.BrowserCalls > 0 ||
		summary.Guardrails.SlackAPICalls > 0 ||
		summary.Guardrails.DestinationWrites > 0 ||
		summary.Guardrails.ProductBrainWrites > 0 ||
		summary.Guardrails.TolariaWrites > 0 ||
		summary.Guardrails.AutoAccepts > 0 ||
		summary.Guardrails.NoHumanClaims ||
		summary.Guardrails.CommittedPrivateArtifacts > 0
}

func hasDEC64ThresholdProof(summary *Summary) bool {
	if summary.GeneralizationStatus != "generalizable" {
		return false
	}
	for _, artifact := range summary.Artifacts {
		if artifact.Type == "corpus_acceptance_benchmark" && artifact.Flags["held_out"] && artifact.Flags["suite_valid"] && (artifact.Flags["dec64_eligible"] || artifact.Flags["no_human_eligible"]) && hasThresholdProof(artifact) {
			return true
		}
		if artifact.Type == "autonomy_readiness_report" && artifact.Flags["held_out"] && artifact.Flags["threshold_eligible"] && hasThresholdProof(artifact) {
			return true
		}
	}
	return false
}

func hasThresholdProof(artifact ArtifactEvidence) bool {
	threshold, thresholdOK := artifact.Metrics["threshold"]
	accuracy, accuracyOK := artifact.Metrics["accuracy"]
	return thresholdOK && accuracyOK && threshold >= 0.98 && accuracy >= threshold
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
	for _, metric := range []string{
		"evidence_ready_atom_ratio", "evidence_or_blocker_ratio", "source_accounting_ratio", "processed_source_ratio",
		"missing_link_reduction_ratio", "missing_link_enrichment_reduction_ratio", "needs_enrichment_reduction_ratio",
		"url_accounting_coverage", "artifact_match_coverage", "evidence_ready_count",
		"semantic_observation_count", "semantic_candidate_count", "candidate_per_processed_source_ratio", "observation_per_segment_ratio",
		"atom_compression_ratio", "relation_review_compression_ratio", "evidence_or_blocker_group_ratio",
	} {
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
	for _, metric := range []string{
		"review_burden_ratio", "review_burden_count", "human_review_required_count", "model_error_count",
		"reference_candidate_ratio", "reference_candidate_count", "reference_only_source_count", "one_candidate_source_count",
	} {
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
		{name: "browser_calls", before: baseline.guardrails.BrowserCalls, after: current.guardrails.BrowserCalls},
		{name: "slack_api_calls", before: baseline.guardrails.SlackAPICalls, after: current.guardrails.SlackAPICalls},
		{name: "destination_writes", before: baseline.guardrails.DestinationWrites, after: current.guardrails.DestinationWrites},
		{name: "product_brain_writes", before: baseline.guardrails.ProductBrainWrites, after: current.guardrails.ProductBrainWrites},
		{name: "tolaria_writes", before: baseline.guardrails.TolariaWrites, after: current.guardrails.TolariaWrites},
		{name: "auto_accepts", before: baseline.guardrails.AutoAccepts, after: current.guardrails.AutoAccepts},
		{name: "no_human_claims", before: boolInt(baseline.guardrails.NoHumanClaims), after: boolInt(current.guardrails.NoHumanClaims)},
		{name: "committed_private_artifacts", before: baseline.guardrails.CommittedPrivateArtifacts, after: current.guardrails.CommittedPrivateArtifacts},
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
	if !baseline.artifactTypes["source_meaning_packet_summary"] && current.artifactTypes["source_meaning_packet_summary"] {
		if compression, ok := current.metrics["atom_compression_ratio"]; ok && compression > 0 {
			comparison.MetricDeltas["source_meaning_packet_added"] = 1
			improved = true
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

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func boolMetric(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func maxInt(a, b int) int {
	if b > a {
		return b
	}
	return a
}

func comparableModels(a, b readbackModel) (bool, []string) {
	if a.flags["replay_baseline_blocked"] {
		return false, append([]string{"replay_baseline_blocked"}, a.replayBaselineReasonCodes...)
	}
	if a.flags["artifact_not_comparable"] || b.flags["artifact_not_comparable"] {
		return false, []string{"artifact_not_comparable"}
	}
	if a.flags["conflicting_corpus_fingerprint"] || b.flags["conflicting_corpus_fingerprint"] || a.flags["conflicting_command_config_fingerprint"] || b.flags["conflicting_command_config_fingerprint"] {
		return false, []string{"conflicting_fingerprints"}
	}
	av, aok := a.fingerprints["corpus_fingerprint"]
	bv, bok := b.fingerprints["corpus_fingerprint"]
	if aok != bok {
		return false, []string{"one_sided_fingerprint"}
	}
	if !aok {
		if sharesArtifactDomain(a, b) {
			return false, []string{"missing_fingerprints"}
		}
		return false, []string{"artifact_domain_mismatch"}
	}
	if av != bv {
		return false, []string{"fingerprint_mismatch"}
	}

	av, aok = a.fingerprints["command_config_fingerprint"]
	bv, bok = b.fingerprints["command_config_fingerprint"]
	if aok != bok {
		return false, []string{"one_sided_fingerprint"}
	}
	if aok && av != bv {
		return false, []string{"fingerprint_mismatch"}
	}
	return true, nil
}

func sharesArtifactDomain(a, b readbackModel) bool {
	for artifactType := range a.artifactTypes {
		if b.artifactTypes[artifactType] {
			return true
		}
	}
	return false
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
	if model.flags["scale_partial"] {
		return ImprovementTarget{Code: "scale_capacity", Rationale: "The run completed only as bounded partial evidence because one or more configured source, graph, or packet scale budgets were reached.", EvidenceRefs: refs}
	}
	if readiness := semanticReadinessForModel(model); readiness.Status == "blocked" {
		return ImprovementTarget{Code: "needs_semantic_density", Rationale: "Source intake may have succeeded, but semantic value is not proven because extraction collapsed into shallow reference output.", EvidenceRefs: refs}
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
	if evidenceReady, ok := model.metrics["evidence_ready_count"]; ok {
		if evalCounted, evalOK := model.metrics["eval_counted_count"]; evalOK && evidenceReady < evalCounted {
			return ImprovementTarget{Code: "needs_evidence_readiness", Rationale: "Not all eval-counted atoms are evidence-ready.", EvidenceRefs: refs}
		}
	}
	return ImprovementTarget{Code: "ready_for_next_pressure_run", Rationale: "No higher-priority readback blocker was found; rerun the next pressure/eval slice with comparable baseline.", EvidenceRefs: refs}
}

func semanticReadinessForModel(model readbackModel) SemanticReadiness {
	readiness := SemanticReadiness{
		ProcessedSourceCount:       intMetric(model, "processed_source_count", "processed_source_count_from_summaries", "source_count"),
		DocumentSegmentCount:       intMetric(model, "document_segment_count_from_summaries", "document_segment_count", "segment_count"),
		SemanticObservationCount:   intMetric(model, "semantic_observation_count", "semantic_observation_count_from_summaries", "observation_count"),
		SemanticCandidateCount:     intMetric(model, "semantic_candidate_count", "semantic_candidate_count_from_summaries", "candidate_count"),
		ReferenceCandidateCount:    intMetric(model, "reference_candidate_count_from_summaries", "reference_candidate_count"),
		OneCandidateSourceCount:    intMetric(model, "one_candidate_source_count", "one_candidate_source_count_from_summaries"),
		ReferenceOnlySourceCount:   intMetric(model, "reference_only_source_count", "reference_only_source_count_from_summaries"),
		CandidatePerSourceRatio:    model.metrics["candidate_per_processed_source_ratio"],
		ObservationPerSegmentRatio: model.metrics["observation_per_segment_ratio"],
		ReferenceCandidateRatio:    model.metrics["reference_candidate_ratio"],
	}
	if readiness.ProcessedSourceCount > 0 && readiness.CandidatePerSourceRatio == 0 {
		readiness.CandidatePerSourceRatio = float64(readiness.SemanticCandidateCount) / float64(readiness.ProcessedSourceCount)
	}
	if readiness.DocumentSegmentCount > 0 && readiness.ObservationPerSegmentRatio == 0 {
		readiness.ObservationPerSegmentRatio = float64(readiness.SemanticObservationCount) / float64(readiness.DocumentSegmentCount)
	}
	if readiness.SemanticCandidateCount > 0 && readiness.ReferenceCandidateRatio == 0 {
		readiness.ReferenceCandidateRatio = float64(readiness.ReferenceCandidateCount) / float64(readiness.SemanticCandidateCount)
	}
	if readiness.ProcessedSourceCount < 10 {
		readiness.Status = "not_evaluated"
		readiness.ReasonCodes = []string{"insufficient_processed_sources"}
		return readiness
	}
	hasReferenceCandidateCount := metricPresent(model, "reference_candidate_count_from_summaries", "reference_candidate_count")
	hasOneCandidateSourceCount := metricPresent(model, "one_candidate_source_count", "one_candidate_source_count_from_summaries")
	hasDocumentSegmentCount := metricPresent(model, "document_segment_count_from_summaries", "document_segment_count", "segment_count")
	if readiness.SemanticCandidateCount == readiness.ProcessedSourceCount &&
		hasReferenceCandidateCount &&
		hasOneCandidateSourceCount &&
		readiness.ReferenceCandidateCount == readiness.SemanticCandidateCount &&
		readiness.OneCandidateSourceCount == readiness.ProcessedSourceCount {
		readiness.ReasonCodes = append(readiness.ReasonCodes, "reference_only_one_candidate_per_source")
	}
	if readiness.DocumentSegmentCount > 0 && readiness.DocumentSegmentCount >= readiness.SemanticObservationCount*2 && readiness.ObservationPerSegmentRatio < 0.25 {
		readiness.ReasonCodes = append(readiness.ReasonCodes, "low_observation_to_segment_density")
	}
	if len(readiness.ReasonCodes) > 0 {
		readiness.Status = "blocked"
		return readiness
	}
	referenceCollapseEvaluated := readiness.SemanticCandidateCount != readiness.ProcessedSourceCount || (hasReferenceCandidateCount && hasOneCandidateSourceCount)
	lowDensityEvaluated := hasDocumentSegmentCount
	if !referenceCollapseEvaluated || !lowDensityEvaluated {
		readiness.Status = "not_evaluated"
		readiness.ReasonCodes = []string{"missing_semantic_density_counters"}
		if !referenceCollapseEvaluated {
			readiness.ReasonCodes = append(readiness.ReasonCodes, "missing_reference_collapse_counters")
		}
		if !lowDensityEvaluated {
			readiness.ReasonCodes = append(readiness.ReasonCodes, "missing_document_segment_count")
		}
		return readiness
	}
	readiness.Status = "ready"
	return readiness
}

func intMetric(model readbackModel, keys ...string) int {
	for _, key := range keys {
		if value, ok := model.metrics[key]; ok {
			return int(value)
		}
	}
	return 0
}

func metricPresent(model readbackModel, keys ...string) bool {
	for _, key := range keys {
		if _, ok := model.metrics[key]; ok {
			return true
		}
	}
	return false
}

func stringListContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func writeSummary(outRoot string, summary Summary, protectedRoots []string) error {
	if strings.TrimSpace(outRoot) == "" {
		return errors.New("missing output root")
	}
	root, err := filepath.Abs(outRoot)
	if err != nil {
		return err
	}
	if err := rejectSymlinkEscape(root, root, protectedRoots); err != nil {
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
	summaryPath := filepath.Join(dir, ReadbackSummaryFile)
	if err := rejectSymlinkEscape(root, summaryPath, protectedRoots); err != nil {
		return err
	}
	if err := writeJSON(summaryPath, summary); err != nil {
		return err
	}
	comparisonPath := filepath.Join(dir, "comparison-summary.json")
	if err := rejectSymlinkEscape(root, comparisonPath, protectedRoots); err != nil {
		return err
	}
	if summary.Comparison != nil {
		if err := writeJSON(comparisonPath, summary.Comparison); err != nil {
			return err
		}
	} else if err := removeIfExists(comparisonPath); err != nil {
		return err
	}
	reportPath := filepath.Join(dir, "readback-report.md")
	if err := rejectSymlinkEscape(root, reportPath, protectedRoots); err != nil {
		return err
	}
	if err := os.WriteFile(reportPath, []byte(markdownReport(summary)), 0o644); err != nil {
		return err
	}
	chainPath := filepath.Join(dir, "chain-capture-draft.md")
	if err := rejectSymlinkEscape(root, chainPath, protectedRoots); err != nil {
		return err
	}
	if err := os.WriteFile(chainPath, []byte(chainDraft(summary)), 0o644); err != nil {
		return err
	}
	return nil
}

func rejectSymlinkEscape(root, dir string, protectedRoots []string) error {
	realRoot, err := resolveOutputPath(root)
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

func resolveOutputPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Abs(resolved)
	}
	current := abs
	missing := []string{}
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("resolve output path: %s", path)
		}
		missing = append([]string{filepath.Base(current)}, missing...)
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			parts := append([]string{resolved}, missing...)
			return filepath.Abs(filepath.Join(parts...))
		}
		current = parent
	}
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

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
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
	b.WriteString("## Replay Baseline\n\n")
	b.WriteString(fmt.Sprintf("- Status: %s\n", summary.ReplayBaseline.Status))
	if len(summary.ReplayBaseline.ReasonCodes) > 0 {
		b.WriteString(fmt.Sprintf("- Reasons: %s\n", strings.Join(summary.ReplayBaseline.ReasonCodes, ", ")))
	}
	if summary.ReplayBaseline.CorpusFingerprint != "" {
		b.WriteString(fmt.Sprintf("- Corpus fingerprint: `%s`\n", summary.ReplayBaseline.CorpusFingerprint))
	}
	if summary.ReplayBaseline.CommandConfigFingerprint != "" {
		b.WriteString(fmt.Sprintf("- Command config fingerprint: `%s`\n", summary.ReplayBaseline.CommandConfigFingerprint))
	}
	if summary.ReplayBaseline.RerunInstruction != "" {
		b.WriteString(fmt.Sprintf("- Rerun: %s\n", summary.ReplayBaseline.RerunInstruction))
	}
	b.WriteString("\n")
	b.WriteString("## Semantic Readiness\n\n")
	b.WriteString(fmt.Sprintf("- Status: %s\n", summary.SemanticReadiness.Status))
	if len(summary.SemanticReadiness.ReasonCodes) > 0 {
		b.WriteString(fmt.Sprintf("- Reasons: %s\n", strings.Join(summary.SemanticReadiness.ReasonCodes, ", ")))
	}
	b.WriteString(fmt.Sprintf("- Density: %d observations / %d segments; %d candidates / %d processed sources; %d reference candidates\n", summary.SemanticReadiness.SemanticObservationCount, summary.SemanticReadiness.DocumentSegmentCount, summary.SemanticReadiness.SemanticCandidateCount, summary.SemanticReadiness.ProcessedSourceCount, summary.SemanticReadiness.ReferenceCandidateCount))
	if summary.SemanticReadiness.Status == "blocked" {
		b.WriteString("- Source intake may have succeeded, but semantic value is not proven.\n")
	}
	b.WriteString("\n")
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
	b.WriteString(". Replay baseline: ")
	b.WriteString(summary.ReplayBaseline.Status)
	if len(summary.ReplayBaseline.ReasonCodes) > 0 {
		b.WriteString(" (")
		b.WriteString(strings.Join(summary.ReplayBaseline.ReasonCodes, ", "))
		b.WriteString(")")
	}
	b.WriteString(". Semantic readiness: ")
	b.WriteString(summary.SemanticReadiness.Status)
	if len(summary.SemanticReadiness.ReasonCodes) > 0 {
		b.WriteString(" (")
		b.WriteString(strings.Join(summary.SemanticReadiness.ReasonCodes, ", "))
		b.WriteString(")")
	}
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
	denied := []string{"/private/tmp/", "/users/", "young human club dropbox", "slack.com/archives", "xoxb-", "xoxp-", "api_key=", "bearer ", "openai_api_key", "posthog_api_key"}
	for _, item := range denied {
		if strings.Contains(lower, item) {
			return true
		}
	}
	return containsSecretLikeSKToken(lower)
}

func containsSecretLikeSKToken(value string) bool {
	for start := 0; start < len(value); {
		idx := strings.Index(value[start:], "sk-")
		if idx < 0 {
			return false
		}
		idx += start
		if idx > 0 && isASCIIAlphaNumeric(value[idx-1]) {
			start = idx + len("sk-")
			continue
		}
		end := idx + len("sk-")
		for end < len(value) && isSecretTokenChar(value[end]) {
			end++
		}
		if end-idx >= 16 {
			return true
		}
		start = idx + len("sk-")
	}
	return false
}

func isSecretTokenChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
}

func isASCIIAlphaNumeric(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
}

func ContainsDeniedString(value string) bool {
	return containsDeniedString(value)
}

func containsDeniedRefString(ref string) bool {
	lower := strings.ToLower(filepath.ToSlash(ref))
	if strings.HasPrefix(lower, "users/") || containsDeniedString("/"+lower) {
		return true
	}
	return false
}

func sanitizedArtifactRef(ref string) string {
	return stableID("artifact-ref", []string{filepath.ToSlash(ref)}) + ".json"
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
