package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const CorpusPressureDirName = "corpus-pressure"

func WriteCorpusPressure(outDir string, summary CorpusPressureSummary, graph CorpusGraphSummary) error {
	if strings.TrimSpace(outDir) == "" {
		return ArtifactWriteError{Err: fmt.Errorf("missing required --out")}
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return ArtifactWriteError{Err: err}
	}
	root, err := filepath.Abs(filepath.Join(outDir, CorpusPressureDirName))
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectIfSymlink(root); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ArtifactWriteError{Err: err}
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	expected := map[string]bool{"pressure-summary.json": true, "pressure-report.md": true, "eval-input.json": true, "trace-summary.json": true}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "pressure-summary.json", summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "pressure-report.md", []byte(corpusPressureMarkdown(summary, graph))); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "eval-input.json", corpusPressureEvalInput(summary)); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "trace-summary.json", CorpusPressureTraceSummaryFor(summary, CorpusPressureSourceCounters{})); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func WriteCorpusPressureLoop(outDir string, summary CorpusPressureLoopSummary) error {
	if strings.TrimSpace(outDir) == "" {
		return ArtifactWriteError{Err: fmt.Errorf("missing required --out")}
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return ArtifactWriteError{Err: err}
	}
	root, err := filepath.Abs(filepath.Join(outDir, CorpusPressureLoopDirName))
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectIfSymlink(root); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ArtifactWriteError{Err: err}
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	expected := map[string]bool{"loop-summary.json": true, "loop-report.md": true}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "loop-summary.json", summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "loop-report.md", []byte(corpusPressureLoopMarkdown(summary))); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func corpusPressureLoopMarkdown(summary CorpusPressureLoopSummary) string {
	var b strings.Builder
	b.WriteString("# Corpus pressure loop report\n\n")
	b.WriteString(fmt.Sprintf("- Corpus: %s\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Runs: %d of %d\n", summary.RunCount, summary.MaxRuns))
	b.WriteString(fmt.Sprintf("- Stop reason: %s\n", summary.StopReason))
	b.WriteString(fmt.Sprintf("- KR passed: %t\n", summary.KRPassed))
	b.WriteString(fmt.Sprintf("- Build fingerprint: %s\n", summary.BuildFingerprint))
	b.WriteString(fmt.Sprintf("- Config fingerprint: %s\n\n", summary.CommandConfigFingerprint))
	b.WriteString("## Iterations\n\n")
	for _, item := range summary.Iterations {
		b.WriteString(fmt.Sprintf("- %02d: kr=%t processed=%.2f evidence=%.2f blocked=%d skipped=%d excluded=%d fingerprint=%s summary `%s`\n", item.Iteration, item.KRPassed, item.ProcessedSourceRatio, item.EvidenceReadyAtomRatio, item.SourceCounters.Blocked, item.SourceCounters.Skipped, item.SourceCounters.Excluded, item.PressureFingerprint, item.PressureSummaryPath))
	}
	return b.String()
}

func corpusPressureEvalInput(summary CorpusPressureSummary) CorpusPressureEvalInput {
	return CorpusPressureEvalInput{
		SchemaVersion:             CorpusPressureEvalInputSchemaVersion,
		CorpusID:                  summary.CorpusID,
		CommandConfigFingerprint:  summary.CommandConfigFingerprint,
		CorpusFingerprint:         summary.CorpusFingerprint,
		PressureSummaryPath:       filepath.ToSlash(filepath.Join(CorpusPressureDirName, "pressure-summary.json")),
		GraphSummaryPath:          summary.GraphSummaryPath,
		SourceCounters:            corpusPressureSourceCounters(summary),
		ProcessedSourceRatio:      summary.ProcessedSourceRatio,
		EvidenceReadyAtomRatio:    summary.EvidenceReadyAtomRatio,
		ReviewBurdenRatio:         summary.ReviewBurdenRatio,
		ReadyForFiftyFilePressure: summary.ReadyForFiftyFilePressure,
		Guardrails:                summary.Guardrails,
		NextImprovementTargets:    append([]string{}, summary.NextImprovementTargets...),
	}
}

func CorpusPressureTraceSummaryFor(summary CorpusPressureSummary, deltas CorpusPressureSourceCounters) CorpusPressureTraceSummary {
	return CorpusPressureTraceSummary{
		SchemaVersion:            CorpusPressureTraceSchemaVersion,
		CorpusID:                 summary.CorpusID,
		Stages:                   corpusPressureTraceStages(summary),
		SourceCounters:           corpusPressureSourceCounters(summary),
		SourceDeltas:             deltas,
		ProcessedSourceRatio:     summary.ProcessedSourceRatio,
		EvidenceReadyAtomRatio:   summary.EvidenceReadyAtomRatio,
		CommandConfigFingerprint: summary.CommandConfigFingerprint,
		CorpusFingerprint:        summary.CorpusFingerprint,
		PressureFingerprint:      summary.ReplayFingerprint,
		GraphReplayFingerprint:   summary.GraphReplayFingerprint,
		Guardrails:               summary.Guardrails,
		ArtifactPaths: map[string]string{
			"pressure_summary": filepath.ToSlash(filepath.Join(CorpusPressureDirName, "pressure-summary.json")),
			"pressure_report":  filepath.ToSlash(filepath.Join(CorpusPressureDirName, "pressure-report.md")),
			"graph_summary":    summary.GraphSummaryPath,
			"graph_manifest":   summary.GraphManifestPath,
		},
	}
}

func corpusPressureTraceStages(summary CorpusPressureSummary) []CorpusPressureTraceStage {
	stages := []CorpusPressureTraceStage{
		{Name: "source_accounting", Status: "pass", Count: summary.SourceCount},
		{Name: "semantic_extraction", Status: "pass", Count: summary.SemanticCandidateCount},
		{Name: "corpus_graph", Status: "pass", Count: summary.GraphAtomCount},
		{Name: "pressure_readiness", Status: "pass"},
	}
	if corpusPressureGraphFailed(summary) {
		stages[2].Status = "failed"
	}
	if !summary.ReadyForFiftyFilePressure {
		stages[len(stages)-1].Status = "needs_improvement"
	}
	if summary.BlockedSourceCount > 0 || summary.UnexplainedExclusionCount > 0 {
		stages[len(stages)-1].Status = "blocked"
	}
	return stages
}

func corpusPressureGraphFailed(summary CorpusPressureSummary) bool {
	for _, blocker := range summary.Blockers {
		if strings.HasPrefix(blocker, "corpus graph failed:") {
			return true
		}
	}
	return false
}

func corpusPressureSourceCounters(summary CorpusPressureSummary) CorpusPressureSourceCounters {
	return CorpusPressureSourceCounters{
		Total:       summary.SourceCount,
		Eligible:    summary.EligibleSourceCount,
		Processed:   summary.ProcessedSourceCount,
		Skipped:     summary.SkippedSourceCount,
		Excluded:    summary.ExcludedSourceCount,
		Blocked:     summary.BlockedSourceCount,
		Unexplained: summary.UnexplainedExclusionCount,
	}
}

func corpusPressureMarkdown(summary CorpusPressureSummary, graph CorpusGraphSummary) string {
	var b strings.Builder
	b.WriteString("# Corpus pressure report\n\n")
	b.WriteString("## Corpus answer\n\n")
	if summary.ReadyForFiftyFilePressure {
		b.WriteString("The corpus pressure run is ready for larger 50-file pressure under the configured thresholds.\n\n")
	} else {
		b.WriteString("The corpus pressure run is not ready for larger 50-file pressure. Inspect the source accounting, evidence failures, and next improvement targets below.\n\n")
	}
	b.WriteString(fmt.Sprintf("- Corpus: %s\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Sources: %d processed, %d skipped, %d blocked, %d total\n", summary.ProcessedSourceCount, summary.SkippedSourceCount, summary.BlockedSourceCount, summary.SourceCount))
	b.WriteString(fmt.Sprintf("- Source state detail: %.2f processed ratio, %d excluded, %d unexplained exclusions\n", summary.ProcessedSourceRatio, summary.ExcludedSourceCount, summary.UnexplainedExclusionCount))
	b.WriteString(fmt.Sprintf("- Semantic candidates: %d\n", summary.SemanticCandidateCount))
	b.WriteString(fmt.Sprintf("- Graph atoms: %d\n", summary.GraphAtomCount))
	b.WriteString(fmt.Sprintf("- Graph relations: %d\n", summary.GraphRelationCount))
	b.WriteString(fmt.Sprintf("- Evidence-ready atoms: %d (%.2f)\n", summary.EvidenceReadyAtomCount, summary.EvidenceReadyAtomRatio))
	b.WriteString(fmt.Sprintf("- Review burden: %d (%.2f)\n", summary.ReviewBurdenCount, summary.ReviewBurdenRatio))
	b.WriteString(fmt.Sprintf("- Replay fingerprint: %s\n\n", summary.ReplayFingerprint))

	b.WriteString("## Source accounting\n\n")
	for _, source := range summary.Sources {
		b.WriteString(fmt.Sprintf("- `%s` %s: %s, candidates=%d, reason=%s, semantic=%s\n", source.SourceID, source.SourceLabel, source.State, source.CandidateCount, source.ReasonCode, emptyDash(source.SemanticRunDir)))
	}
	b.WriteString("\n")

	b.WriteString("## Extracted candidates by source\n\n")
	for _, source := range summary.Sources {
		if source.CandidateCount == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("- `%s`: %d candidate(s), kinds=%s, artifacts `%s`\n", source.SourceID, source.CandidateCount, candidateKindCountsText(source.CandidateKindCounts), source.SemanticRunDir))
	}
	if summary.SemanticCandidateCount == 0 {
		b.WriteString("- No semantic candidates extracted.\n")
	}
	b.WriteString("\n")

	writeRelationSection(&b, "Connected clusters", graph, CorpusRelationSameTopicAs)
	writeRelationSection(&b, "Duplicate candidates", graph, CorpusRelationPossibleDuplicate)
	writeRelationSection(&b, "Contradiction candidates", graph, CorpusRelationContradicts)

	b.WriteString("## Eval/trace artifact pointers\n\n")
	b.WriteString(fmt.Sprintf("- Eval input: `%s`\n", filepath.ToSlash(filepath.Join(CorpusPressureDirName, "eval-input.json"))))
	b.WriteString(fmt.Sprintf("- Trace summary: `%s`\n", filepath.ToSlash(filepath.Join(CorpusPressureDirName, "trace-summary.json"))))
	b.WriteString(fmt.Sprintf("- Pressure summary: `%s`\n", filepath.ToSlash(filepath.Join(CorpusPressureDirName, "pressure-summary.json"))))
	b.WriteString(fmt.Sprintf("- Graph summary: `%s`\n\n", summary.GraphSummaryPath))

	b.WriteString("## Evidence/readiness failures\n\n")
	if len(summary.Blockers) == 0 {
		wrote := false
		for _, atom := range graph.Atoms {
			if atom.ReviewStatus != ReviewStatusReady {
				b.WriteString(fmt.Sprintf("- evidence_incomplete_atom `%s`: status=%s, source=%s, artifact `%s`\n", atom.AtomID, atom.ReviewStatus, atom.SourceID, atom.AtomPath))
				wrote = true
			}
		}
		for _, relation := range graph.Relations {
			if relation.ReviewStatus == ReviewStatusBlocked || relation.ReviewStatus == ReviewStatusNeedsReview {
				b.WriteString(fmt.Sprintf("- evidence_incomplete_relation `%s`: status=%s, artifact `%s`\n", relation.RelationID, relation.ReviewStatus, relation.RelationPath))
				wrote = true
			}
		}
		if !wrote {
			b.WriteString("- No pressure blockers.\n")
		}
	} else {
		for _, blocker := range summary.Blockers {
			b.WriteString("- " + blocker + "\n")
		}
		for _, atom := range graph.Atoms {
			if atom.ReviewStatus != ReviewStatusReady {
				b.WriteString(fmt.Sprintf("- evidence_incomplete_atom `%s`: status=%s, source=%s, artifact `%s`\n", atom.AtomID, atom.ReviewStatus, atom.SourceID, atom.AtomPath))
			}
		}
		for _, relation := range graph.Relations {
			if relation.ReviewStatus == ReviewStatusBlocked || relation.ReviewStatus == ReviewStatusNeedsReview {
				b.WriteString(fmt.Sprintf("- evidence_incomplete_relation `%s`: status=%s, artifact `%s`\n", relation.RelationID, relation.ReviewStatus, relation.RelationPath))
			}
		}
	}
	b.WriteString("\n")

	b.WriteString("## Next improvement targets\n\n")
	if len(summary.NextImprovementTargets) == 0 {
		b.WriteString("- No improvement targets under current thresholds.\n")
	} else {
		targets := append([]string{}, summary.NextImprovementTargets...)
		sort.Strings(targets)
		for _, target := range targets {
			b.WriteString("- " + target + "\n")
		}
	}
	return b.String()
}

func writeRelationSection(b *strings.Builder, title string, graph CorpusGraphSummary, relationType CorpusRelationType) {
	b.WriteString("## " + title + "\n\n")
	count := 0
	for _, relation := range graph.Relations {
		if relation.RelationType != relationType {
			continue
		}
		count++
		b.WriteString(fmt.Sprintf("- `%s`: %s, confidence=%s, `%s` (%s %s) -> `%s` (%s %s), relation `%s`, review `%s`\n", relation.RelationID, relation.ReviewStatus, relation.Confidence, relation.FromAtomID, relation.FromSourceID, relation.FromSourceLabel, relation.ToAtomID, relation.ToSourceID, relation.ToSourceLabel, relation.RelationPath, relation.ReviewPath))
	}
	if count == 0 {
		b.WriteString("- None found.\n")
	}
	b.WriteString("\n")
}

func candidateKindCountsText(counts map[SemanticCandidateKind]int) string {
	if len(counts) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, counts[SemanticCandidateKind(key)]))
	}
	return strings.Join(parts, ",")
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
