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
	expected := map[string]bool{"pressure-summary.json": true, "pressure-report.md": true}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "pressure-summary.json", summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "pressure-report.md", []byte(corpusPressureMarkdown(summary, graph))); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
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
	b.WriteString(fmt.Sprintf("- Semantic candidates: %d\n", summary.SemanticCandidateCount))
	b.WriteString(fmt.Sprintf("- Graph atoms: %d\n", summary.GraphAtomCount))
	b.WriteString(fmt.Sprintf("- Graph relations: %d\n", summary.GraphRelationCount))
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
