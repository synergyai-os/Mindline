package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteCorpusAcceptanceLabelRecording(outDir string, summary CorpusAcceptanceLabelRecordingSummary, answerKey CorpusAcceptanceAnswerKey) error {
	if strings.TrimSpace(outDir) == "" {
		return ArtifactWriteError{Err: fmt.Errorf("missing required --out")}
	}
	if err := ValidateCorpusAcceptanceLabelRecordingSummary(summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	report := corpusAcceptanceLabelRecordingMarkdown(summary)
	if containsUnsafeMarker(report) || containsGovernanceID(report) || containsPrivateReportMarker(report) {
		return ArtifactWriteError{Err: fmt.Errorf("corpus acceptance label recording report contains private marker")}
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return ArtifactWriteError{Err: err}
	}
	root, err := filepath.Abs(filepath.Join(outDir, corpusAcceptanceLabelRecordingDirName))
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectIfSymlink(root); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if summary.ArtifactConfidentiality == corpusAcceptanceArtifactLocalPrivateRehydrated {
		if err := rejectCorpusAcceptanceLocalPrivateRehydratedOutput(root); err != nil {
			return ArtifactWriteError{Err: err}
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ArtifactWriteError{Err: err}
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	expected := map[string]bool{"answer-key.json": true, "label-recording-summary.json": true, "label-recording-report.md": true}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "answer-key.json", answerKey); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "label-recording-summary.json", summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "label-recording-report.md", []byte(report)); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func ValidateCorpusAcceptanceLabelRecordingSummary(summary CorpusAcceptanceLabelRecordingSummary) error {
	if summary.SchemaVersion != CorpusAcceptanceLabelRecordingSummarySchemaVersion {
		return fmt.Errorf("unsupported corpus acceptance label recording summary schema version: %s", summary.SchemaVersion)
	}
	if strings.TrimSpace(summary.SuiteID) == "" || sanitizeID(summary.SuiteID) != summary.SuiteID {
		return fmt.Errorf("unsafe suite id")
	}
	if !validCorpusAcceptanceArtifactConfidentiality(summary.ArtifactConfidentiality) {
		return fmt.Errorf("invalid artifact confidentiality")
	}
	body := strings.Join([]string{
		summary.SuiteID,
		string(summary.SuiteKind),
		summary.LabelingStatus,
		summary.CorpusID,
		summary.CorpusFingerprint,
		summary.CommandConfigFingerprint,
		summary.Independence,
		strings.Join(summary.Blockers, "\n"),
		summary.AnswerKeyPath,
		summary.ReportPath,
		summary.SeedPrivateMapStatus,
		summary.ArtifactConfidentiality,
		strings.Join(summary.ClaimBoundaries, "\n"),
	}, "\n")
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		return fmt.Errorf("corpus acceptance label recording summary contains private marker")
	}
	return nil
}

func validCorpusAcceptanceArtifactConfidentiality(value string) bool {
	switch value {
	case corpusAcceptanceArtifactLocalPrivateRehydrated,
		corpusAcceptanceArtifactPrivateSafeRedacted,
		corpusAcceptanceArtifactNonSeedLocal,
		corpusAcceptanceArtifactBlocked:
		return true
	default:
		return false
	}
}

func rejectCorpusAcceptanceLocalPrivateRehydratedOutput(realRoot string) error {
	clean, err := realPathForPossiblyMissing(realRoot)
	if err != nil {
		return err
	}
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		switch part {
		case ".productbrain", "testdata", ".git":
			return fmt.Errorf("local private rehydrated output must be outside durable workspace artifacts")
		}
	}
	gitVisible, err := hasGitVisibleAncestor(clean)
	if err != nil {
		return err
	}
	if gitVisible {
		return fmt.Errorf("local private rehydrated output must be outside durable workspace artifacts")
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	absWD, err := filepath.Abs(wd)
	if err != nil {
		return err
	}
	realWD, err := filepath.EvalSymlinks(absWD)
	if err != nil {
		return err
	}
	if sameOrChildPath(realWD, clean) {
		return fmt.Errorf("local private rehydrated output must be outside durable workspace artifacts")
	}
	return nil
}

func hasGitVisibleAncestor(path string) (bool, error) {
	current := filepath.Clean(path)
	for {
		info, err := os.Lstat(filepath.Join(current, ".git"))
		if err == nil {
			return info.Mode().IsDir() || info.Mode().IsRegular(), nil
		}
		if !os.IsNotExist(err) {
			return false, err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false, nil
		}
		current = parent
	}
}

func corpusAcceptanceLabelRecordingMarkdown(summary CorpusAcceptanceLabelRecordingSummary) string {
	var b strings.Builder
	b.WriteString("# Corpus acceptance label recording\n\n")
	b.WriteString("Label records were converted into a corpus acceptance answer key artifact. This is not held-out accuracy proof, generalization proof, formal autonomy-threshold proof, destination-write readiness, or no-human operation.\n\n")
	b.WriteString(fmt.Sprintf("- Labeling status: %s\n", summary.LabelingStatus))
	b.WriteString(fmt.Sprintf("- Suite kind: %s\n", summary.SuiteKind))
	b.WriteString(fmt.Sprintf("- Records: %d\n", summary.RecordCount))
	b.WriteString(fmt.Sprintf("- Eval outcomes: %d\n", summary.EvalCount))
	b.WriteString(fmt.Sprintf("- Expected present: %d\n", summary.ExpectedPresentCount))
	b.WriteString(fmt.Sprintf("- Expected absent: %d\n", summary.ExpectedAbsentCount))
	b.WriteString(fmt.Sprintf("- Uncertain: %d\n", summary.UncertainCount))
	b.WriteString(fmt.Sprintf("- Abstain: %d\n", summary.AbstainCount))
	b.WriteString(fmt.Sprintf("- Seed mode: %t\n", summary.SeedMode))
	b.WriteString(fmt.Sprintf("- Seed private map status: %s\n", summary.SeedPrivateMapStatus))
	b.WriteString(fmt.Sprintf("- Original corpus compatible: %t\n", summary.OriginalCorpusCompatible))
	b.WriteString(fmt.Sprintf("- Translated sources: %d\n", summary.TranslatedSourceCount))
	b.WriteString(fmt.Sprintf("- Translated expected outcomes: %d\n", summary.TranslatedExpectedOutcomeCount))
	b.WriteString(fmt.Sprintf("- Translated evidence refs: %d\n", summary.TranslatedEvidenceRefCount))
	b.WriteString(fmt.Sprintf("- Artifact confidentiality: %s\n", summary.ArtifactConfidentiality))
	b.WriteString(fmt.Sprintf("- Held-out ready: %t\n", summary.HeldOutReady))
	b.WriteString(fmt.Sprintf("- Benchmark ready: %t\n\n", summary.BenchmarkReady))
	if len(summary.Blockers) > 0 {
		b.WriteString("## Blockers\n\n")
		for _, blocker := range summary.Blockers {
			b.WriteString("- " + blocker + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Guardrails\n\n")
	b.WriteString(fmt.Sprintf("- Destination writes: %d\n", summary.Guardrails.DestinationWrites))
	b.WriteString(fmt.Sprintf("- Product Brain writes: %d\n", summary.Guardrails.ProductBrainWrites))
	b.WriteString(fmt.Sprintf("- Tolaria writes: %d\n", summary.Guardrails.TolariaWrites))
	b.WriteString(fmt.Sprintf("- Hosted inference calls: %d\n", summary.Guardrails.HostedInferenceCalls))
	b.WriteString(fmt.Sprintf("- Hosted telemetry exports: %d\n", summary.Guardrails.HostedTelemetryExports))
	b.WriteString(fmt.Sprintf("- Auto accepts: %d\n", summary.Guardrails.AutoAccepts))
	b.WriteString(fmt.Sprintf("- No-human claims: %d\n\n", summary.Guardrails.NoHumanClaims))
	b.WriteString("## Claim boundaries\n\n")
	for _, boundary := range summary.ClaimBoundaries {
		b.WriteString("- " + boundary + "\n")
	}
	return b.String()
}
