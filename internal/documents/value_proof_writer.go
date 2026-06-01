package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func WriteValueProof(outDir string, summary ValueProofSummary, items []SourceMeaningPreviewItem) error {
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
	root, err := filepath.Abs(filepath.Join(outDir, ValueProofDirName))
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
	expected := map[string]bool{"value-summary.json": true, "local-value-packet.md": true, "pr-safe-summary.md": true}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "value-summary.json", summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "local-value-packet.md", []byte(valueProofLocalMarkdown(summary, items))); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "pr-safe-summary.md", []byte(valueProofPRSafeMarkdown(summary))); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func valueProofLocalMarkdown(summary ValueProofSummary, items []SourceMeaningPreviewItem) string {
	var b strings.Builder
	b.WriteString("# Mindline mixed-source value proof\n\n")
	b.WriteString("> Local proof packet only. This is not a destination write, human approval, or no-human autonomy claim.\n\n")
	b.WriteString("## What was read\n\n")
	b.WriteString(fmt.Sprintf("- Corpus: `%s`\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Sources accounted: %d of %d (%.2f)\n", summary.AccountedSourceCount, summary.SourceCount, summary.SourceAccountingRatio))
	b.WriteString(fmt.Sprintf("- Source states: %d processed, %d skipped, %d excluded, %d blocked\n\n", summary.ProcessedSourceCount, summary.SkippedSourceCount, summary.ExcludedSourceCount, summary.BlockedSourceCount))
	b.WriteString("## What Mindline understood\n\n")
	b.WriteString(fmt.Sprintf("- Graph evidence-ready atoms: %d of %d (%.2f)\n", summary.EvidenceReadyAtomCount, summary.AtomCount, summary.EvidenceReadyAtomRatio))
	b.WriteString(fmt.Sprintf("- Reviewable atoms with evidence or explicit blocker: %d of %d (%.2f)\n", summary.EvidenceOrBlockerCount, summary.AtomCount, summary.EvidenceOrBlockerRatio))
	b.WriteString(fmt.Sprintf("- Corpus graph relations: %d\n", summary.RelationCount))
	writeValueProofRelationCounts(&b, summary.RelationTypeCounts)
	b.WriteString("\n## Where to inspect the real evidence\n\n")
	b.WriteString(fmt.Sprintf("- Source meaning report: `%s`\n", filepath.ToSlash(filepath.Join(SourceMeaningPreviewDirName, "meaning-report.md"))))
	for _, source := range sortedValueProofSources(summary.Sources) {
		b.WriteString(fmt.Sprintf("- `%s`: state=%s atoms=%d relations=%d evidence preview `%s`\n", source.SourceID, source.State, source.AtomCount, source.RelationCount, source.PreviewPath))
	}
	b.WriteString("\n## Evidence highlights\n\n")
	writeValueProofEvidenceHighlights(&b, items)
	b.WriteString("\n## Claim status\n\n")
	writeValueProofClaimStatus(&b, summary)
	b.WriteString("\n## Proof artifacts\n\n")
	b.WriteString(fmt.Sprintf("- Pressure summary: `%s`\n", summary.PressureSummaryPath))
	b.WriteString(fmt.Sprintf("- Graph summary: `%s`\n", summary.GraphSummaryPath))
	b.WriteString(fmt.Sprintf("- Meaning summary: `%s`\n", summary.MeaningSummaryPath))
	b.WriteString("\n## Blockers and next targets\n\n")
	writeValueProofList(&b, "Blockers", summary.Blockers)
	writeValueProofList(&b, "Next improvement targets", summary.NextImprovementTargets)
	return b.String()
}

func writeValueProofEvidenceHighlights(b *strings.Builder, items []SourceMeaningPreviewItem) {
	wrote := false
	sorted := append([]SourceMeaningPreviewItem{}, items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].SourceID < sorted[j].SourceID })
	for _, item := range sorted {
		if len(item.Atoms) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("### %s\n\n", markdownTitle(item.SourceLabel, item.SourceID)))
		for _, atom := range item.Atoms {
			wrote = true
			b.WriteString(fmt.Sprintf("- `%s` %s, status=%s, confidence=%s, lines=%d-%d\n", atom.AtomID, atom.CandidateKind, atom.ReviewStatus, atom.Confidence, atom.LineStart, atom.LineEnd))
			if strings.TrimSpace(atom.Summary) != "" {
				b.WriteString("  - Summary: " + truncateForMarkdown(atom.Summary, 220) + "\n")
			}
			if strings.TrimSpace(atom.Excerpt) != "" {
				b.WriteString("  - Evidence: " + truncateForMarkdown(atom.Excerpt, 260) + "\n")
			} else if len(atom.Missingness) > 0 {
				b.WriteString("  - Blocker: " + missingnessList(atom.Missingness) + "\n")
			} else {
				b.WriteString("  - Blocker: missing_evidence\n")
			}
		}
		b.WriteString("\n")
	}
	if !wrote {
		b.WriteString("- No evidence-backed atoms surfaced.\n")
	}
}

func valueProofPRSafeMarkdown(summary ValueProofSummary) string {
	var b strings.Builder
	b.WriteString("# WP-37 PR-safe value proof summary\n\n")
	b.WriteString("This summary intentionally excludes raw source text, private paths, prompts, completions, and destination payloads.\n\n")
	b.WriteString(fmt.Sprintf("- Schema: `%s`\n", summary.SchemaVersion))
	b.WriteString(fmt.Sprintf("- Corpus id: `%s`\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Sources accounted: %d of %d (%.2f)\n", summary.AccountedSourceCount, summary.SourceCount, summary.SourceAccountingRatio))
	b.WriteString(fmt.Sprintf("- Graph evidence-ready atoms: %d of %d (%.2f)\n", summary.EvidenceReadyAtomCount, summary.AtomCount, summary.EvidenceReadyAtomRatio))
	b.WriteString(fmt.Sprintf("- Reviewable atoms with evidence or explicit blocker: %d of %d (%.2f)\n", summary.EvidenceOrBlockerCount, summary.AtomCount, summary.EvidenceOrBlockerRatio))
	b.WriteString(fmt.Sprintf("- Graph-backed relations: %d\n", summary.RelationCount))
	writeValueProofRelationCounts(&b, summary.RelationTypeCounts)
	b.WriteString("\n## Default side-effect counters\n\n")
	b.WriteString(fmt.Sprintf("- Hosted inference calls: %d\n", summary.Guardrails.HostedInferenceCalls))
	b.WriteString(fmt.Sprintf("- Hosted telemetry exports: %d\n", summary.Guardrails.HostedTelemetryExports))
	b.WriteString(fmt.Sprintf("- Network fetches: %d\n", summary.Guardrails.NetworkFetches))
	b.WriteString(fmt.Sprintf("- Browser calls: %d\n", summary.Guardrails.BrowserCalls))
	b.WriteString(fmt.Sprintf("- Slack API calls: %d\n", summary.Guardrails.SlackAPICalls))
	b.WriteString(fmt.Sprintf("- Destination writes: %d\n", summary.Guardrails.DestinationWrites))
	b.WriteString(fmt.Sprintf("- Product Brain writes: %d\n", summary.Guardrails.ProductBrainWrites))
	b.WriteString(fmt.Sprintf("- Tolaria writes: %d\n", summary.Guardrails.TolariaWrites))
	b.WriteString(fmt.Sprintf("- Auto-accepts: %d\n", summary.Guardrails.AutoAccepts))
	b.WriteString("\n## Claim status\n\n")
	writeValueProofClaimStatus(&b, summary)
	b.WriteString("\n## Source accounting\n\n")
	for _, source := range sortedValueProofSources(summary.Sources) {
		b.WriteString(fmt.Sprintf("- `%s`: kind=%s state=%s reason=%s atoms=%d relations=%d\n", source.SourceID, source.SourceKind, source.State, source.ReasonCode, source.AtomCount, source.RelationCount))
	}
	return b.String()
}

func writeValueProofRelationCounts(b *strings.Builder, counts map[CorpusRelationType]int) {
	b.WriteString("- Relation type counts:\n")
	if len(counts) == 0 {
		b.WriteString("  - none: 0\n")
		return
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteString(fmt.Sprintf("  - %s: %d\n", key, counts[CorpusRelationType(key)]))
	}
}

func writeValueProofClaimStatus(b *strings.Builder, summary ValueProofSummary) {
	b.WriteString(fmt.Sprintf("- Safety: `%s`\n", summary.ClaimStatuses.Safety))
	b.WriteString(fmt.Sprintf("- Improvement: `%s`\n", summary.ClaimStatuses.Improvement))
	b.WriteString(fmt.Sprintf("- Generalization: `%s`\n", summary.ClaimStatuses.Generalization))
	b.WriteString(fmt.Sprintf("- DEC-64/no-human: `%s`\n", summary.ClaimStatuses.DEC64))
}

func writeValueProofList(b *strings.Builder, title string, values []string) {
	b.WriteString("### " + title + "\n\n")
	if len(values) == 0 {
		b.WriteString("- None.\n\n")
		return
	}
	for _, value := range values {
		b.WriteString("- " + value + "\n")
	}
	b.WriteString("\n")
}
