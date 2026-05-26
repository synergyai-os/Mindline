package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const AutonomyReadinessDirName = "autonomy-readiness"

func WriteAutonomyReadinessReport(outDir string, report AutonomyReadinessReport) error {
	if err := writeAutonomyReadinessReport(outDir, report); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func writeAutonomyReadinessReport(outDir string, report AutonomyReadinessReport) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("missing required --out")
	}
	if report.SchemaVersion != AutonomyReadinessReportSchemaVersion {
		return fmt.Errorf("unsupported autonomy readiness report schema version: %s", report.SchemaVersion)
	}
	root, err := filepath.Abs(filepath.Join(outDir, AutonomyReadinessDirName))
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
		"readiness-report.json": true,
		"readiness-report.md":   true,
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expectedFiles); err != nil {
		return err
	}
	if err := writeJSON(realRoot, "readiness-report.json", report); err != nil {
		return err
	}
	return writeFile(realRoot, "readiness-report.md", []byte(autonomyReadinessMarkdown(report)))
}

func autonomyReadinessMarkdown(report AutonomyReadinessReport) string {
	var b strings.Builder
	b.WriteString("# Autonomy readiness report\n\n")
	b.WriteString(fmt.Sprintf("- Threshold status: %s\n", report.ThresholdStatus))
	b.WriteString(fmt.Sprintf("- Held out: %t\n", report.HeldOut))
	b.WriteString(fmt.Sprintf("- Accuracy: %.4f\n", report.Accuracy))
	b.WriteString(fmt.Sprintf("- Threshold: %.4f\n", report.Threshold))
	b.WriteString(fmt.Sprintf("- Candidates: %d\n", report.Counts.CandidateCount))
	b.WriteString(fmt.Sprintf("- Eval-counted: %d\n", report.Counts.EvalCountedCount))
	b.WriteString(fmt.Sprintf("- Eval-counted accepted: %d\n", report.Counts.EvalCountedAcceptedCount))
	b.WriteString(fmt.Sprintf("- Eval-counted false positives: %d\n", report.Counts.EvalCountedFalsePositiveCount))
	b.WriteString(fmt.Sprintf("- Eval-counted false negatives: %d\n", report.Counts.EvalCountedFalseNegativeCount))
	b.WriteString(fmt.Sprintf("- Evidence-ready: %d\n", report.Counts.EvidenceReadyCount))
	b.WriteString(fmt.Sprintf("- Human review required: %d\n", report.Counts.HumanReviewRequiredCount))
	b.WriteString(fmt.Sprintf("- Model errors: %d\n", report.Counts.ModelErrorCount))
	b.WriteString(fmt.Sprintf("- PostHog projection: %s\n\n", report.Projection.Status))

	b.WriteString("## KRs\n\n")
	for _, key := range []string{"KEY-3", "KEY-4", "KEY-5", "KEY-6", "KEY-7"} {
		item := report.KRs[key]
		b.WriteString(fmt.Sprintf("- %s: %s (%s; target %s)\n", key, item.Status, item.Current, item.Target))
	}
	b.WriteString("\n")

	if len(report.Blockers) > 0 {
		b.WriteString("## Blockers\n\n")
		for _, blocker := range report.Blockers {
			b.WriteString(fmt.Sprintf("- %s\n", blocker))
		}
		b.WriteString("\n")
	}

	if len(report.Improvement) > 0 {
		b.WriteString("## Top improvement targets\n\n")
		for _, item := range report.Improvement {
			b.WriteString(fmt.Sprintf("- %s (%d): %s\n", item.Code, item.Count, item.Summary))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Safety counters\n\n")
	b.WriteString(fmt.Sprintf("- Destination writes: %d\n", report.SafetyCounters.DestinationWrites))
	b.WriteString(fmt.Sprintf("- Auto-accepts: %d\n", report.SafetyCounters.AutoAccepts))
	b.WriteString(fmt.Sprintf("- No-human claims: %d\n", report.SafetyCounters.NoHumanClaims))
	b.WriteString(fmt.Sprintf("- Committed private artifacts: %d\n", report.SafetyCounters.CommittedPrivateArtifacts))
	return b.String()
}
