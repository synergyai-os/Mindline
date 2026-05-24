package documents

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func WriteSemanticJudgment(outDir string, summary SemanticJudgmentSummary) error {
	root, err := semanticJudgmentRoot(outDir)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return WriteSemanticJudgmentRoot(root, summary)
}

func WriteSemanticJudgmentRoot(root string, summary SemanticJudgmentSummary) error {
	if err := writeSemanticJudgmentRoot(root, summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func writeSemanticJudgmentRoot(root string, summary SemanticJudgmentSummary) error {
	if err := ValidateSemanticJudgmentSummary(summary); err != nil {
		return err
	}
	realRoot, err := ensureSemanticJudgmentRoot(root)
	if err != nil {
		return err
	}
	expectedFiles := map[string]bool{
		"judgment-summary.json":      true,
		"cursor.json":                true,
		"reports/judgment-report.md": true,
	}
	itemsByID := map[string]SemanticJudgmentCandidate{}
	for _, item := range summary.Items {
		itemsByID[item.CandidateID] = item
	}
	for _, item := range summary.Candidates {
		expectedFiles[item.CandidatePath] = true
		expectedFiles[item.PagePath] = true
		if item.JudgmentPath != "" {
			expectedFiles[item.JudgmentPath] = true
		}
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expectedFiles); err != nil {
		return err
	}
	if err := writeJSON(realRoot, "judgment-summary.json", summary); err != nil {
		return err
	}
	cursor := SemanticJudgmentCursor{
		SchemaVersion:  SemanticJudgmentCursorSchemaVersion,
		RunID:          summary.RunID,
		NextIndex:      semanticJudgmentNextIndex(summary),
		TotalCount:     summary.CandidateCount,
		JudgedCount:    summary.JudgedCount,
		RemainingCount: summary.RemainingCount,
		Exhausted:      summary.RemainingCount == 0,
	}
	if err := writeJSON(realRoot, "cursor.json", cursor); err != nil {
		return err
	}
	for _, itemSummary := range summary.Candidates {
		item, ok := itemsByID[itemSummary.CandidateID]
		if !ok {
			return fmt.Errorf("missing semantic judgment candidate: %s", itemSummary.CandidateID)
		}
		if err := writeJSON(realRoot, itemSummary.CandidatePath, item); err != nil {
			return err
		}
		if err := writeFile(realRoot, itemSummary.PagePath, []byte(semanticJudgmentPageMarkdown(item, cursor))); err != nil {
			return err
		}
	}
	for _, judgment := range summary.Judgments {
		if err := writeJSON(realRoot, SemanticJudgmentRecordJSONPath(judgment.CandidateID), judgment); err != nil {
			return err
		}
	}
	if err := writeFile(realRoot, "reports/judgment-report.md", []byte(semanticJudgmentReportMarkdown(summary))); err != nil {
		return err
	}
	return nil
}

func semanticJudgmentNextIndex(summary SemanticJudgmentSummary) int {
	for i, item := range summary.Candidates {
		if item.JudgmentPath == "" {
			return i
		}
	}
	return len(summary.Candidates)
}

func semanticJudgmentReportMarkdown(summary SemanticJudgmentSummary) string {
	var b strings.Builder
	b.WriteString("# Semantic Judgment Quality Report\n\n")
	b.WriteString(summary.QualityStatement + "\n\n")
	b.WriteString(fmt.Sprintf("- Candidates: %d\n", summary.CandidateCount))
	b.WriteString(fmt.Sprintf("- Judged: %d\n", summary.JudgedCount))
	b.WriteString(fmt.Sprintf("- Remaining: %d\n", summary.RemainingCount))
	b.WriteString(fmt.Sprintf("- Accepted: %d\n", summary.AcceptedCount))
	b.WriteString(fmt.Sprintf("- Rejected: %d\n", summary.RejectedCount))
	b.WriteString(fmt.Sprintf("- Unclear: %d\n", summary.UnclearCount))
	b.WriteString(fmt.Sprintf("- Duplicate: %d\n", summary.DuplicateCount))
	b.WriteString(fmt.Sprintf("- Wrong kind: %d\n", summary.WrongKindCount))
	b.WriteString(fmt.Sprintf("- Blocked coverage: %d\n", summary.BlockedCount))
	b.WriteString(fmt.Sprintf("- Skipped coverage: %d\n", summary.SkippedCount))
	b.WriteString(fmt.Sprintf("- Evidence ready: %d\n", summary.EvidenceReadyCount))
	b.WriteString(fmt.Sprintf("- Eval counted: %d\n", summary.EvalCountedCount))
	b.WriteString(fmt.Sprintf("- Evidence excluded: %d\n", summary.EvidenceExcludedCount))
	b.WriteString(fmt.Sprintf("- Review burden: %d\n", summary.ReviewBurdenCount))
	b.WriteString(fmt.Sprintf("- Precision estimate: %.2f\n", summary.PrecisionEstimate))
	b.WriteString("\n## Evidence readiness\n\n")
	for _, reason := range semanticEvidenceReadinessReasons() {
		b.WriteString(fmt.Sprintf("- %s: %d\n", reason, summary.EvidenceReadinessReasonCounts[reason]))
	}
	b.WriteString("\n## Failure modes\n\n")
	for _, choice := range []SemanticJudgmentChoice{
		SemanticJudgmentChoiceReject,
		SemanticJudgmentChoiceUnclear,
		SemanticJudgmentChoiceDuplicate,
		SemanticJudgmentChoiceWrongKind,
	} {
		b.WriteString(fmt.Sprintf("- %s: %d\n", choice, summary.FailureModeCounts[choice]))
	}
	b.WriteString("\n## Grouped judgment analytics\n\n")
	b.WriteString("These grouped counts are calibration evidence only; they do not prove no-human readiness.\n\n")
	writeSemanticJudgmentGroupSection(&b, "By candidate kind", summary.JudgmentByCandidateKind)
	writeSemanticJudgmentGroupSection(&b, "By confidence", summary.JudgmentByConfidence)
	writeSemanticJudgmentGroupSection(&b, "By review status", summary.JudgmentByReviewStatus)
	writeSemanticJudgmentGroupSection(&b, "By source document", summary.JudgmentBySourceDocument)
	writeSemanticJudgmentGroupSection(&b, "By relation presence", summary.JudgmentByRelationPresence)
	writeSemanticJudgmentGroupSection(&b, "By relation type", summary.JudgmentByRelationType)
	return b.String()
}

func writeSemanticJudgmentGroupSection[K ~string](b *strings.Builder, title string, groups map[K]map[SemanticJudgmentChoice]int) {
	b.WriteString("### " + title + "\n\n")
	if len(groups) == 0 {
		b.WriteString("- No judged candidates\n\n")
		return
	}
	keys := make([]string, 0, len(groups))
	byString := map[string]map[SemanticJudgmentChoice]int{}
	for key, counts := range groups {
		stringKey := string(key)
		keys = append(keys, stringKey)
		byString[stringKey] = counts
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteString("- " + key)
		for _, choice := range []SemanticJudgmentChoice{
			SemanticJudgmentChoiceAccept,
			SemanticJudgmentChoiceReject,
			SemanticJudgmentChoiceUnclear,
			SemanticJudgmentChoiceDuplicate,
			SemanticJudgmentChoiceWrongKind,
		} {
			if count := byString[key][choice]; count > 0 {
				b.WriteString(fmt.Sprintf(" %s=%d", choice, count))
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}
