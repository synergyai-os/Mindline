package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const SourceMeaningPreviewDirName = "source-meaning-preview"

func WriteSourceMeaningPreview(outDir string, summary SourceMeaningPreviewSummary, items []SourceMeaningPreviewItem) error {
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
	root, err := filepath.Abs(filepath.Join(outDir, SourceMeaningPreviewDirName))
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
	expected := map[string]bool{"meaning-summary.json": true, "meaning-report.md": true}
	for _, item := range items {
		expected[SourceMeaningItemPreviewPath(item.SourceID)] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "meaning-summary.json", summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "meaning-report.md", []byte(sourceMeaningReportMarkdown(summary))); err != nil {
		return ArtifactWriteError{Err: err}
	}
	for _, item := range items {
		if err := writeFile(realRoot, SourceMeaningItemPreviewPath(item.SourceID), []byte(sourceMeaningItemMarkdown(item))); err != nil {
			return ArtifactWriteError{Err: err}
		}
	}
	return nil
}

func SourceMeaningItemPreviewPath(sourceID string) string {
	return filepath.ToSlash(filepath.Join("sources", sanitizeID(sourceID)+".md"))
}

func sourceMeaningReportMarkdown(summary SourceMeaningPreviewSummary) string {
	var b strings.Builder
	b.WriteString("# Source meaning preview report\n\n")
	b.WriteString("> Preview/calibration only. Nothing in this report is saved, accepted, destination-ready, or executable as a write.\n\n")
	b.WriteString("## Review answer\n\n")
	b.WriteString(fmt.Sprintf("- Corpus: %s\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Sources previewed: %d of %d processed\n", summary.PreviewedSourceCount, summary.ProcessedSourceCount))
	b.WriteString(fmt.Sprintf("- Source states: %d processed, %d skipped, %d excluded, %d blocked, %d total\n", summary.ProcessedSourceCount, summary.SkippedSourceCount, summary.ExcludedSourceCount, summary.BlockedSourceCount, summary.SourceCount))
	b.WriteString(fmt.Sprintf("- Atoms: %d\n", summary.AtomCount))
	b.WriteString(fmt.Sprintf("- Relations: %d\n", summary.RelationCount))
	b.WriteString(fmt.Sprintf("- Preview coverage: %.2f\n", summary.PreviewCoverageRatio))
	b.WriteString(fmt.Sprintf("- Evidence coverage: %.2f\n", summary.EvidenceCoverageRatio))
	b.WriteString(fmt.Sprintf("- Routing coverage: %.2f\n\n", summary.RoutingCoverageRatio))

	b.WriteString("## Guardrails\n\n")
	b.WriteString(fmt.Sprintf("- Destination writes: %d\n", summary.Guardrails.DestinationWrites))
	b.WriteString(fmt.Sprintf("- Product Brain writes: %d\n", summary.Guardrails.ProductBrainWrites))
	b.WriteString(fmt.Sprintf("- Tolaria writes: %d\n", summary.Guardrails.TolariaWrites))
	b.WriteString(fmt.Sprintf("- Hosted inference calls: %d\n", summary.Guardrails.HostedInferenceCalls))
	b.WriteString(fmt.Sprintf("- Hosted telemetry exports: %d\n\n", summary.Guardrails.HostedTelemetryExports))

	b.WriteString("## Routing hints\n\n")
	writeMeaningCounts(&b, meaningRouteCounts(summary.RoutingHintCounts))
	b.WriteString("\n## Missingness\n\n")
	writeMeaningCounts(&b, meaningMissingnessCounts(summary.MissingnessCounts))
	b.WriteString("\n## Candidate kinds\n\n")
	writeMeaningCounts(&b, meaningCandidateKindCounts(summary.CandidateKindCounts))
	b.WriteString("\n## Sources\n\n")
	for _, item := range summary.Items {
		b.WriteString(fmt.Sprintf("- `%s` %s: %s, atoms=%d, relations=%d, routes=%s, missingness=%s, preview `%s`\n", item.SourceID, item.SourceLabel, item.State, item.AtomCount, item.RelationCount, routeHintList(item.RoutingHints), missingnessList(item.Missingness), item.PreviewPath))
	}
	if len(summary.Items) == 0 {
		b.WriteString("- No sources available for preview.\n")
	}
	return b.String()
}

func sourceMeaningItemMarkdown(item SourceMeaningPreviewItem) string {
	var b strings.Builder
	b.WriteString("# " + markdownTitle(item.SourceLabel, item.SourceID) + "\n\n")
	b.WriteString("> Preview/calibration only. This is not saved, accepted, destination-ready, or executable as a write.\n\n")
	b.WriteString("## Source snapshot\n\n")
	b.WriteString(fmt.Sprintf("- Source id: `%s`\n", item.SourceID))
	b.WriteString(fmt.Sprintf("- Source kind: `%s`\n", item.SourceKind))
	b.WriteString(fmt.Sprintf("- State: `%s`\n", item.State))
	b.WriteString(fmt.Sprintf("- Reason: `%s`\n", item.ReasonCode))
	b.WriteString("- Destination writes: `0`\n")
	b.WriteString("- Product Brain writes: `0`\n")
	b.WriteString("- Tolaria writes: `0`\n")
	if item.SourcePath != "" {
		b.WriteString(fmt.Sprintf("- Source artifact: `%s`\n", item.SourcePath))
	}
	b.WriteString(fmt.Sprintf("- Atoms: %d\n", item.AtomCount))
	b.WriteString(fmt.Sprintf("- Relations: %d\n\n", item.RelationCount))

	b.WriteString("## What Mindline thinks this means\n\n")
	if len(item.Atoms) == 0 {
		b.WriteString("- No semantic atom was extracted.\n")
	} else {
		for _, atom := range item.Atoms {
			b.WriteString(fmt.Sprintf("### %s\n\n", markdownTitle(atom.Title, atom.AtomID)))
			b.WriteString(fmt.Sprintf("- Kind: `%s`\n", atom.CandidateKind))
			b.WriteString(fmt.Sprintf("- Confidence: `%s`\n", atom.Confidence))
			b.WriteString(fmt.Sprintf("- Review status: `%s`\n", atom.ReviewStatus))
			b.WriteString(fmt.Sprintf("- Lines: %d-%d\n", atom.LineStart, atom.LineEnd))
			if strings.TrimSpace(atom.Summary) != "" {
				b.WriteString("\n" + atom.Summary + "\n")
			}
			b.WriteString("\nEvidence:\n\n")
			if strings.TrimSpace(atom.Excerpt) == "" {
				b.WriteString("- Missing evidence excerpt.\n")
			} else {
				b.WriteString("> " + strings.ReplaceAll(strings.TrimSpace(atom.Excerpt), "\n", "\n> ") + "\n")
			}
			b.WriteString("\nRouting hints:\n\n")
			for _, route := range atom.RoutingHints {
				b.WriteString(fmt.Sprintf("- `%s` write_eligible=%t reasons=%s\n", route.Hint, route.WriteEligible, strings.Join(route.ReasonCodes, ",")))
			}
			if len(atom.Missingness) > 0 {
				b.WriteString("\nMissingness:\n\n")
				for _, missing := range atom.Missingness {
					b.WriteString(fmt.Sprintf("- `%s`\n", missing))
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Relation context\n\n")
	if len(item.Relations) == 0 {
		b.WriteString("- No duplicate, same-topic, or contradiction context found.\n")
	} else {
		for _, relation := range item.Relations {
			b.WriteString(fmt.Sprintf("- `%s` %s, status=%s, confidence=%s, other_source=`%s`, reason=`%s`\n", relation.RelationID, relation.RelationType, relation.ReviewStatus, relation.Confidence, relation.OtherSource, relation.ReasonCode))
			for _, evidence := range relation.Evidence {
				b.WriteString("  - " + truncateForMarkdown(evidence, 220) + "\n")
			}
		}
	}
	b.WriteString("\n## Source-level routing\n\n")
	for _, route := range item.RoutingHints {
		b.WriteString(fmt.Sprintf("- `%s` write_eligible=%t reasons=%s\n", route.Hint, route.WriteEligible, strings.Join(route.ReasonCodes, ",")))
	}
	b.WriteString("\n## Source-level missingness\n\n")
	for _, missing := range item.Missingness {
		b.WriteString(fmt.Sprintf("- `%s`\n", missing))
	}
	if len(item.Missingness) == 0 {
		b.WriteString("- `none`\n")
	}
	return b.String()
}

func markdownTitle(title, fallback string) string {
	if strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	return fallback
}

func truncateForMarkdown(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	if limit < 4 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func meaningRouteCounts(counts map[SourceMeaningPreviewRoutingHint]int) map[string]int {
	out := map[string]int{}
	for key, value := range counts {
		out[string(key)] = value
	}
	return out
}

func meaningMissingnessCounts(counts map[SourceMeaningPreviewMissingness]int) map[string]int {
	out := map[string]int{}
	for key, value := range counts {
		out[string(key)] = value
	}
	return out
}

func meaningCandidateKindCounts(counts map[SemanticCandidateKind]int) map[string]int {
	out := map[string]int{}
	for key, value := range counts {
		out[string(key)] = value
	}
	return out
}

func writeMeaningCounts(b *strings.Builder, counts map[string]int) {
	if len(counts) == 0 {
		b.WriteString("- None.\n")
		return
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteString(fmt.Sprintf("- `%s`: %d\n", key, counts[key]))
	}
}

func routeHintList(values []SourceMeaningPreviewRoutingHint) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, string(value))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func missingnessList(values []SourceMeaningPreviewMissingness) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, string(value))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
