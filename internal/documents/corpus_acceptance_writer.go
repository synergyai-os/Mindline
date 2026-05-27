package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteCorpusAcceptanceBenchmark(outDir string, summary CorpusAcceptanceBenchmarkSummary) error {
	if strings.TrimSpace(outDir) == "" {
		return ArtifactWriteError{Err: fmt.Errorf("missing required --out")}
	}
	if err := ValidateCorpusAcceptanceBenchmarkSummary(summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return ArtifactWriteError{Err: err}
	}
	root, err := filepath.Abs(filepath.Join(outDir, corpusAcceptanceDirName))
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
	expected := map[string]bool{"benchmark-summary.json": true, "benchmark-report.md": true}
	for _, source := range summary.Sources {
		expected[filepath.ToSlash(filepath.Join("sources", source.SourceID, "acceptance-summary.json"))] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "benchmark-summary.json", summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "benchmark-report.md", []byte(corpusAcceptanceMarkdown(summary))); err != nil {
		return ArtifactWriteError{Err: err}
	}
	for _, source := range summary.Sources {
		path := filepath.ToSlash(filepath.Join("sources", source.SourceID, "acceptance-summary.json"))
		if err := writeJSON(realRoot, path, source); err != nil {
			return ArtifactWriteError{Err: err}
		}
	}
	return nil
}

func ValidateCorpusAcceptanceBenchmarkSummary(summary CorpusAcceptanceBenchmarkSummary) error {
	if summary.SchemaVersion != CorpusAcceptanceSummarySchemaVersion {
		return fmt.Errorf("unsupported corpus acceptance summary schema version: %s", summary.SchemaVersion)
	}
	body := strings.Join([]string{
		summary.SuiteID,
		string(summary.SuiteKind),
		summary.CorpusID,
		summary.CorpusFingerprint,
		summary.CommandConfigFingerprint,
		summary.PressureReplayFingerprint,
		strings.Join(summary.SuiteValidityBlockers, "\n"),
		strings.Join(summary.EligibilityBlockers, "\n"),
		strings.Join(summary.NextImprovementTargets, "\n"),
	}, "\n")
	for _, source := range summary.Sources {
		if filepath.IsAbs(source.AcceptanceSummaryPath) {
			return fmt.Errorf("corpus acceptance summary contains absolute artifact path")
		}
		body += "\n" + source.SourceID + "\n" + source.SourceContentHash + "\n" + source.AcceptanceSummaryPath
		body += "\n" + strings.Join(source.Blockers, "\n")
	}
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		return fmt.Errorf("corpus acceptance summary contains private marker")
	}
	return nil
}

func corpusAcceptanceMarkdown(summary CorpusAcceptanceBenchmarkSummary) string {
	var b strings.Builder
	b.WriteString("# Corpus acceptance benchmark\n\n")
	if summary.DEC64Eligible {
		b.WriteString("The held-out corpus benchmark is eligible for DEC-64 under the configured threshold.\n\n")
	} else {
		b.WriteString("The held-out corpus benchmark is not eligible for DEC-64. Inspect suite validity and eligibility blockers.\n\n")
	}
	b.WriteString(fmt.Sprintf("- Suite: %s (%s)\n", summary.SuiteID, summary.SuiteKind))
	b.WriteString(fmt.Sprintf("- Corpus: %s\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Held out: %t\n", summary.HeldOut))
	b.WriteString(fmt.Sprintf("- Suite valid: %t\n", summary.SuiteValid))
	b.WriteString(fmt.Sprintf("- DEC-64 eligible: %t\n", summary.DEC64Eligible))
	b.WriteString(fmt.Sprintf("- Accuracy: %.4f threshold %.4f\n", summary.Accuracy, summary.Threshold))
	b.WriteString(fmt.Sprintf("- Eval count: %d\n", summary.EvalCount))
	b.WriteString(fmt.Sprintf("- Matched: %d\n", summary.MatchedExpectedCount))
	b.WriteString(fmt.Sprintf("- False positives: %d\n", summary.FalsePositiveCount))
	b.WriteString(fmt.Sprintf("- False negatives: %d\n", summary.FalseNegativeCount))
	b.WriteString(fmt.Sprintf("- Wrong kind: %d\n", summary.WrongKindCount))
	b.WriteString(fmt.Sprintf("- Human review required: %d\n", summary.HumanReviewRequiredCount))
	b.WriteString(fmt.Sprintf("- Review burden: %d\n\n", summary.ReviewBurdenCount))
	if len(summary.SuiteValidityBlockers) > 0 {
		b.WriteString("## Suite validity blockers\n\n")
		for _, blocker := range summary.SuiteValidityBlockers {
			b.WriteString("- " + blocker + "\n")
		}
		b.WriteString("\n")
	}
	if len(summary.EligibilityBlockers) > 0 {
		b.WriteString("## Eligibility blockers\n\n")
		for _, blocker := range summary.EligibilityBlockers {
			b.WriteString("- " + blocker + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Sources\n\n")
	for _, source := range summary.Sources {
		b.WriteString(fmt.Sprintf("- `%s`: accuracy=%.4f eval=%d candidates=%d fp=%d fn=%d wrong_kind=%d review=%d\n", source.SourceID, source.Accuracy, source.EvalCount, source.CandidateCount, source.FalsePositiveCount, source.FalseNegativeCount, source.WrongKindCount, source.HumanReviewRequiredCount))
	}
	if len(summary.NextImprovementTargets) > 0 {
		b.WriteString("\n## Next improvement targets\n\n")
		for _, target := range summary.NextImprovementTargets {
			b.WriteString("- " + target + "\n")
		}
	}
	return b.String()
}
