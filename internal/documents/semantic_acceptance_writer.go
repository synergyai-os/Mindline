package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteSemanticAcceptance(outDir string, summary SemanticAcceptanceSummary) error {
	if err := writeSemanticAcceptance(outDir, summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func writeSemanticAcceptance(outDir string, summary SemanticAcceptanceSummary) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("missing required --out")
	}
	if err := ValidateSemanticAcceptanceSummary(summary); err != nil {
		return err
	}
	root, err := filepath.Abs(filepath.Join(outDir, "semantic-acceptance"))
	if err != nil {
		return err
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return err
	}
	if err := rejectIfSymlink(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	expectedFiles := map[string]bool{
		"acceptance-summary.json":   true,
		"reports/quality-report.md": true,
	}
	for _, outcome := range summary.ExpectedOutcomes {
		expectedFiles[outcome.ExpectedPath] = true
	}
	for _, item := range summary.Candidates {
		expectedFiles[item.ItemPath] = true
		expectedFiles[item.PreviewPath] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expectedFiles); err != nil {
		return err
	}
	if err := writeJSON(realRoot, "acceptance-summary.json", summary); err != nil {
		return err
	}
	for _, outcome := range summary.ExpectedOutcomes {
		if err := writeJSON(realRoot, outcome.ExpectedPath, outcome); err != nil {
			return err
		}
	}
	itemsByID := map[string]SemanticAcceptanceItem{}
	for _, item := range summary.Items {
		itemsByID[item.CandidateID] = item
	}
	for _, item := range summary.Candidates {
		full, ok := itemsByID[item.CandidateID]
		if !ok {
			full = SemanticAcceptanceItem{
				SchemaVersion:   SemanticAcceptanceItemSchemaVersion,
				CandidateID:     item.CandidateID,
				CandidateKind:   item.CandidateKind,
				AcceptanceState: item.AcceptanceState,
				Reason:          item.Reason,
				EvidenceNodes:   []string{},
				EvidenceRanges:  []SemanticEvidenceRange{},
				RelationIDs:     []string{},
				Blockers:        []Blocker{},
			}
		}
		if err := writeJSON(realRoot, item.ItemPath, full); err != nil {
			return err
		}
		if err := writeFile(realRoot, item.PreviewPath, []byte(semanticAcceptancePreviewMarkdown(item))); err != nil {
			return err
		}
	}
	if err := writeFile(realRoot, "reports/quality-report.md", []byte(semanticAcceptanceReportMarkdown(summary))); err != nil {
		return err
	}
	return nil
}

func ValidateSemanticAcceptanceSummary(summary SemanticAcceptanceSummary) error {
	if summary.SchemaVersion != SemanticAcceptanceSummarySchemaVersion {
		return fmt.Errorf("unsupported semantic acceptance summary schema version: %s", summary.SchemaVersion)
	}
	body := summary.RunID + "\n" + summary.AnswerKeyID + "\n" + summary.QualityStatement
	for _, outcome := range summary.ExpectedOutcomes {
		body += "\n" + outcome.ExpectedOutcomeID + "\n" + outcome.MatchedCandidateID
	}
	for _, item := range summary.Candidates {
		body += "\n" + item.CandidateID
	}
	for _, item := range summary.Items {
		body += "\n" + item.RunID + "\n" + item.CandidateID + "\n" + item.Title + "\n" + item.Summary + "\n" + strings.Join(item.EvidenceNodes, "\n")
		body += "\n" + item.SourceDocumentID + "\n" + strings.Join(item.RelationIDs, "\n")
		for _, blocker := range item.Blockers {
			body += "\n" + blocker.Code + "\n" + blocker.Message
		}
		for _, evidenceRange := range item.EvidenceRanges {
			body += "\n" + evidenceRange.StructureNodeID
		}
	}
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		return fmt.Errorf("semantic acceptance output contains private marker")
	}
	return nil
}

func semanticAcceptancePreviewMarkdown(item SemanticAcceptanceItemSummary) string {
	var b strings.Builder
	b.WriteString("# Semantic acceptance item\n\n")
	b.WriteString("- Candidate: " + item.CandidateID + "\n")
	b.WriteString("- Kind: " + string(item.CandidateKind) + "\n")
	b.WriteString("- Acceptance state: " + string(item.AcceptanceState) + "\n")
	b.WriteString("- Reason: " + string(item.Reason) + "\n")
	return b.String()
}

func semanticAcceptanceReportMarkdown(summary SemanticAcceptanceSummary) string {
	var b strings.Builder
	b.WriteString("# Semantic Acceptance Quality Report\n\n")
	b.WriteString(summary.QualityStatement + "\n\n")
	b.WriteString(fmt.Sprintf("- Candidates: %d\n", summary.CandidateCount))
	b.WriteString(fmt.Sprintf("- Matched expected outcomes: %d\n", summary.MatchedExpectedCount))
	b.WriteString(fmt.Sprintf("- Missed expected outcomes: %d\n", summary.MissedExpectedCount))
	b.WriteString(fmt.Sprintf("- Unexpected candidates: %d\n", summary.UnexpectedCandidateCount))
	b.WriteString(fmt.Sprintf("- False positives: %d\n", summary.FalsePositiveCount))
	b.WriteString(fmt.Sprintf("- False negatives: %d\n", summary.FalseNegativeCount))
	b.WriteString(fmt.Sprintf("- Review burden: %d\n", summary.ReviewBurdenCount))
	b.WriteString(fmt.Sprintf("- Duplicate candidates: %d\n", summary.DuplicateCount))
	b.WriteString(fmt.Sprintf("- Evidence-missing count: %d\n", summary.EvidenceMissingCount))
	b.WriteString(fmt.Sprintf("- Precision-like match rate: %.2f\n", summary.PrecisionLikeMatchRate))
	b.WriteString(fmt.Sprintf("- Recall-like expected-outcome coverage: %.2f\n", summary.RecallLikeExpectedOutcomeCoverage))
	return b.String()
}
