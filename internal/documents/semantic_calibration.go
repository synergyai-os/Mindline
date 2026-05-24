package documents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultSemanticCalibrationThreshold = 0.98
const (
	maxSemanticCalibrationExcerptRanges        = 3
	maxSemanticCalibrationExcerptLines         = 6
	maxSemanticCalibrationExcerptCharsPerRange = 1200
	maxSemanticCalibrationExcerptCharsPerItem  = 4000
)

var semanticCalibrationFailureClasses = []SemanticCalibrationFailureClass{
	SemanticCalibrationFailureAccepted,
	SemanticCalibrationFailureFalsePositive,
	SemanticCalibrationFailureFalseNegative,
	SemanticCalibrationFailureMissingEvidence,
	SemanticCalibrationFailureRelationError,
	SemanticCalibrationFailureSourceScopeError,
	SemanticCalibrationFailureBlockedPrivate,
	SemanticCalibrationFailureDuplicate,
	SemanticCalibrationFailureNeedsReviewAmbiguity,
	SemanticCalibrationFailureOther,
}

func CalibrateSemanticAcceptance(inputDir, outDir string, options SemanticCalibrationOptions) (SemanticCalibrationSummary, error) {
	root, err := resolveSemanticAcceptanceRoot(inputDir)
	if err != nil {
		return SemanticCalibrationSummary{}, err
	}
	acceptance, items, expected, err := readSemanticAcceptanceBundle(root)
	if err != nil {
		return SemanticCalibrationSummary{}, err
	}
	source, err := loadSemanticCalibrationSource(options)
	if err != nil {
		return SemanticCalibrationSummary{}, err
	}
	threshold := options.Threshold
	if threshold < defaultSemanticCalibrationThreshold {
		threshold = defaultSemanticCalibrationThreshold
	}
	summary := BuildSemanticCalibrationSummary(acceptance, items, expected, threshold, options.HeldOut, source)
	if err := WriteSemanticCalibration(outDir, summary); err != nil {
		return SemanticCalibrationSummary{}, err
	}
	return summary, nil
}

func NextSemanticCalibrationReviewPage(inputDir string) (SemanticCalibrationPage, error) {
	root, err := resolveSemanticCalibrationRoot(inputDir)
	if err != nil {
		return SemanticCalibrationPage{}, err
	}
	summary, err := readSemanticCalibrationSummary(root)
	if err != nil {
		return SemanticCalibrationPage{}, err
	}
	if err := validateSemanticCalibrationSummaryPaths(root, summary); err != nil {
		return SemanticCalibrationPage{}, err
	}
	cursor, err := readSemanticCalibrationCursor(root, summary)
	if err != nil {
		return SemanticCalibrationPage{}, err
	}
	if err := validateSemanticCalibrationCursor(cursor, summary); err != nil {
		return SemanticCalibrationPage{}, err
	}
	if cursor.NextIndex >= len(summary.ReviewItems) {
		cursor.NextIndex = len(summary.ReviewItems)
		cursor.ProcessedCount = len(summary.ReviewItems)
		cursor.RemainingCount = 0
		cursor.Exhausted = true
		if err := writeJSON(root, "cursor.json", cursor); err != nil {
			return SemanticCalibrationPage{}, ArtifactWriteError{Err: err}
		}
		return SemanticCalibrationPage{
			SchemaVersion: SemanticCalibrationPageSchemaVersion,
			Done:          true,
			Cursor:        cursor,
		}, nil
	}
	itemSummary := summary.ReviewItems[cursor.NextIndex]
	itemPath, err := containedSemanticCalibrationPath(root, itemSummary.ItemPath)
	if err != nil {
		return SemanticCalibrationPage{}, err
	}
	var item SemanticCalibrationReviewItem
	if err := readJSONFile(itemPath, &item); err != nil {
		return SemanticCalibrationPage{}, fmt.Errorf("read calibration review item: %w", err)
	}
	if item.SchemaVersion != SemanticCalibrationReviewItemSchemaVersion &&
		item.SchemaVersion != SemanticCalibrationReviewItemPreviousSchemaVersion &&
		item.SchemaVersion != SemanticCalibrationReviewItemLegacySchemaVersion {
		return SemanticCalibrationPage{}, fmt.Errorf("unsupported semantic calibration review item schema version: %s", item.SchemaVersion)
	}
	if item.ReviewItemID != itemSummary.ReviewItemID {
		return SemanticCalibrationPage{}, fmt.Errorf("calibration review item id mismatch: %s", itemSummary.ReviewItemID)
	}
	if item.SchemaVersion == SemanticCalibrationReviewItemLegacySchemaVersion || item.SchemaVersion == SemanticCalibrationReviewItemPreviousSchemaVersion {
		normalizeLegacySemanticCalibrationReviewItem(&item)
	}
	if err := ValidateSemanticCalibrationReviewItem(item); err != nil {
		return SemanticCalibrationPage{}, err
	}
	cursor.NextIndex++
	cursor.ProcessedCount = cursor.NextIndex
	cursor.RemainingCount = len(summary.ReviewItems) - cursor.NextIndex
	cursor.Exhausted = cursor.RemainingCount == 0
	if err := writeJSON(root, "cursor.json", cursor); err != nil {
		return SemanticCalibrationPage{}, ArtifactWriteError{Err: err}
	}
	return SemanticCalibrationPage{
		SchemaVersion: SemanticCalibrationPageSchemaVersion,
		Done:          false,
		Cursor:        cursor,
		Item:          &item,
		ReviewContext: semanticCalibrationReviewContext(item),
		PageMarkdown:  semanticCalibrationPageMarkdown(item),
	}, nil
}

func BuildSemanticCalibrationSummary(acceptance SemanticAcceptanceSummary, items []SemanticAcceptanceItem, expected []SemanticExpectedOutcomeResult, threshold float64, heldOut bool, sources ...semanticCalibrationSourceContext) SemanticCalibrationSummary {
	source := semanticCalibrationSourceContext{}
	if len(sources) > 0 {
		source = sources[0]
	}
	reviewItems := semanticCalibrationReviewItems(acceptance.RunID, items, expected, source)
	counts := emptySemanticCalibrationFailureClassCounts()
	reasonCounts := emptySemanticFailureReasonCounts()
	for _, item := range reviewItems {
		counts[item.FailureClass]++
		if item.FailureReason != "" {
			reasonCounts[item.FailureReason]++
		}
	}
	scored := 0
	for class, count := range counts {
		if class == SemanticCalibrationFailureBlockedPrivate {
			continue
		}
		scored += count
	}
	accepted := counts[SemanticCalibrationFailureAccepted]
	measured := ratio(accepted, scored)
	status := SemanticCalibrationThresholdNotTrusted
	noHuman := false
	if heldOut && scored > 0 && measured >= threshold && counts[SemanticCalibrationFailureBlockedPrivate] == 0 {
		status = SemanticCalibrationThresholdTrusted
		noHuman = true
	}
	itemSummaries := make([]SemanticCalibrationReviewItemSummary, 0, len(reviewItems))
	for _, item := range reviewItems {
		itemSummaries = append(itemSummaries, SemanticCalibrationReviewItemSummary{
			ReviewItemID:    item.ReviewItemID,
			ItemPath:        SemanticCalibrationReviewItemJSONPath(item.ReviewItemID),
			PreviewPath:     SemanticCalibrationReviewPreviewPath(item.ReviewItemID),
			FailureClass:    item.FailureClass,
			FailureReason:   item.FailureReason,
			FailureInferred: item.FailureInferred,
			AcceptanceState: item.AcceptanceState,
			Reason:          item.Reason,
		})
	}
	return SemanticCalibrationSummary{
		SchemaVersion:       SemanticCalibrationSummarySchemaVersion,
		RunID:               acceptance.RunID,
		AnswerKeyID:         acceptance.AnswerKeyID,
		Threshold:           threshold,
		HeldOut:             heldOut,
		ThresholdStatus:     status,
		NoHumanEligible:     noHuman,
		MeasuredAccuracy:    measured,
		ScoredCount:         scored,
		AcceptedCount:       accepted,
		FalsePositiveCount:  counts[SemanticCalibrationFailureFalsePositive],
		FalseNegativeCount:  counts[SemanticCalibrationFailureFalseNegative],
		NeedsReviewCount:    counts[SemanticCalibrationFailureNeedsReviewAmbiguity],
		ReviewBurdenCount:   scored - accepted,
		ReviewBurdenRate:    ratio(scored-accepted, scored),
		BlockedPrivateCount: counts[SemanticCalibrationFailureBlockedPrivate],
		ReviewItemCount:     len(reviewItems),
		FailureClassCounts:  counts,
		FailureReasonCounts: reasonCounts,
		QualityStatement:    semanticCalibrationQualityStatement(heldOut),
		CursorPath:          "cursor.json",
		ReportPath:          "reports/calibration-report.md",
		ReviewItems:         itemSummaries,
		Items:               reviewItems,
	}
}

func WriteSemanticCalibration(outDir string, summary SemanticCalibrationSummary) error {
	if err := writeSemanticCalibration(outDir, summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func writeSemanticCalibration(outDir string, summary SemanticCalibrationSummary) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("missing required --out")
	}
	if err := ValidateSemanticCalibrationSummary(summary); err != nil {
		return err
	}
	root, err := filepath.Abs(filepath.Join(outDir, "semantic-calibration"))
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
		"calibration-summary.json":      true,
		"cursor.json":                   true,
		"reports/calibration-report.md": true,
	}
	for _, item := range summary.ReviewItems {
		expectedFiles[item.ItemPath] = true
		expectedFiles[item.PreviewPath] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expectedFiles); err != nil {
		return err
	}
	if err := writeJSON(realRoot, "calibration-summary.json", summary); err != nil {
		return err
	}
	cursor := SemanticCalibrationCursor{
		SchemaVersion:  SemanticCalibrationCursorSchemaVersion,
		RunID:          summary.RunID,
		NextIndex:      0,
		TotalCount:     summary.ReviewItemCount,
		ProcessedCount: 0,
		RemainingCount: summary.ReviewItemCount,
		Exhausted:      summary.ReviewItemCount == 0,
	}
	if err := writeJSON(realRoot, "cursor.json", cursor); err != nil {
		return err
	}
	itemsByID := map[string]SemanticCalibrationReviewItem{}
	for _, item := range summary.Items {
		itemsByID[item.ReviewItemID] = item
	}
	for _, item := range summary.ReviewItems {
		full, ok := itemsByID[item.ReviewItemID]
		if !ok {
			return fmt.Errorf("missing calibration review item: %s", item.ReviewItemID)
		}
		if err := writeJSON(realRoot, item.ItemPath, full); err != nil {
			return err
		}
		if err := writeFile(realRoot, item.PreviewPath, []byte(semanticCalibrationPreviewMarkdown(full))); err != nil {
			return err
		}
	}
	if err := writeFile(realRoot, "reports/calibration-report.md", []byte(semanticCalibrationReportMarkdown(summary))); err != nil {
		return err
	}
	return nil
}

func ValidateSemanticCalibrationSummary(summary SemanticCalibrationSummary) error {
	if summary.SchemaVersion != SemanticCalibrationSummarySchemaVersion && summary.SchemaVersion != SemanticCalibrationSummaryLegacySchemaVersion {
		return fmt.Errorf("unsupported semantic calibration summary schema version: %s", summary.SchemaVersion)
	}
	if summary.FailureClassCounts == nil {
		return fmt.Errorf("missing semantic calibration failure class counts")
	}
	if summary.SchemaVersion == SemanticCalibrationSummarySchemaVersion {
		if summary.FailureReasonCounts == nil {
			return fmt.Errorf("missing semantic calibration failure reason counts")
		}
		for reason := range summary.FailureReasonCounts {
			if !validSemanticFailureReason(reason) {
				return fmt.Errorf("unsupported semantic calibration failure reason count: %s", reason)
			}
		}
	}
	body := summary.RunID + "\n" + summary.AnswerKeyID + "\n" + summary.QualityStatement
	for _, item := range summary.ReviewItems {
		body += "\n" + item.ReviewItemID + "\n" + item.ItemPath + "\n" + item.PreviewPath
	}
	for _, item := range summary.Items {
		body += "\n" + item.RunID + "\n" + item.ReviewItemID + "\n" + item.CandidateID + "\n" + item.ExpectedOutcomeID
		body += "\n" + item.SourceDocumentID + "\n" + item.Title + "\n" + item.Summary + "\n" + strings.Join(item.EvidenceNodes, "\n")
		body += "\n" + strings.Join(item.RelationIDs, "\n")
		body += "\n" + semanticCalibrationExpectedOutcomeBody(item.ExpectedOutcome)
		body += "\n" + string(item.FailureReason)
		for _, excerpt := range item.EvidenceExcerpts {
			body += "\n" + excerpt.SourceLabel + "\n" + excerpt.StructureNodeID + "\n" + excerpt.Text + "\n" + excerpt.UnavailableReason
		}
		for _, blocker := range item.Blockers {
			body += "\n" + blocker.Code + "\n" + blocker.Message
		}
		for _, evidenceRange := range item.EvidenceRanges {
			body += "\n" + evidenceRange.StructureNodeID
		}
	}
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		return fmt.Errorf("semantic calibration output contains private marker")
	}
	return nil
}

func semanticCalibrationReviewItems(runID string, items []SemanticAcceptanceItem, expected []SemanticExpectedOutcomeResult, source semanticCalibrationSourceContext) []SemanticCalibrationReviewItem {
	out := make([]SemanticCalibrationReviewItem, 0, len(items)+len(expected))
	expectedByID := map[string]SemanticExpectedOutcomeResult{}
	for _, outcome := range expected {
		expectedByID[outcome.ExpectedOutcomeID] = outcome
	}
	for _, item := range items {
		failureClass := semanticCalibrationFailureClassForItem(item)
		failureReason, inferred, _ := semanticFailureReasonForCalibrationItem(item.Reason, failureClass)
		expectedContext := semanticCalibrationExpectedOutcomeContext(expectedByID[item.ExpectedOutcomeID], item.CandidateID)
		out = append(out, SemanticCalibrationReviewItem{
			SchemaVersion:     SemanticCalibrationReviewItemSchemaVersion,
			ReviewItemID:      "review-" + item.CandidateID,
			RunID:             item.RunID,
			SourceDocumentID:  item.SourceDocumentID,
			CandidateID:       item.CandidateID,
			ExpectedOutcomeID: item.ExpectedOutcomeID,
			ExpectedOutcome:   expectedContext,
			CandidateKind:     item.CandidateKind,
			ReviewStatus:      item.ReviewStatus,
			Confidence:        item.Confidence,
			Title:             item.Title,
			Summary:           item.Summary,
			EvidenceNodes:     cloneStringList(item.EvidenceNodes),
			EvidenceRanges:    cloneSemanticEvidenceRanges(item.EvidenceRanges),
			EvidenceExcerpts:  semanticCalibrationEvidenceExcerpts(source, item.EvidenceRanges),
			RelationIDs:       cloneStringList(item.RelationIDs),
			AcceptanceState:   item.AcceptanceState,
			Reason:            item.Reason,
			FailureClass:      failureClass,
			FailureReason:     failureReason,
			FailureInferred:   inferred,
			NeedsAdjudication: failureClass != SemanticCalibrationFailureAccepted,
			Blockers:          cloneBlockerList(item.Blockers),
		})
	}
	for _, outcome := range expected {
		if outcome.Reason != SemanticAcceptanceReasonMissingExpectedOutcome || outcome.MatchedCandidateID != "" {
			continue
		}
		out = append(out, SemanticCalibrationReviewItem{
			SchemaVersion:     SemanticCalibrationReviewItemSchemaVersion,
			ReviewItemID:      "review-missed-" + outcome.ExpectedOutcomeID,
			RunID:             runID,
			ExpectedOutcomeID: outcome.ExpectedOutcomeID,
			ExpectedOutcome:   semanticCalibrationExpectedOutcomeContext(outcome, ""),
			CandidateKind:     outcome.ExpectedKind,
			EvidenceNodes:     []string{},
			EvidenceRanges:    []SemanticEvidenceRange{},
			EvidenceExcerpts:  semanticCalibrationEvidenceExcerpts(source, nil),
			RelationIDs:       []string{},
			AcceptanceState:   outcome.AcceptanceState,
			Reason:            outcome.Reason,
			FailureClass:      SemanticCalibrationFailureFalseNegative,
			FailureReason:     SemanticFailureMissingExpectedOutcome,
			FailureInferred:   false,
			NeedsAdjudication: true,
			Blockers:          []Blocker{},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ReviewItemID < out[j].ReviewItemID
	})
	for i := range out {
		out[i].ItemIndex = i
	}
	return out
}

func semanticCalibrationFailureClassForItem(item SemanticAcceptanceItem) SemanticCalibrationFailureClass {
	if item.AcceptanceState == SemanticAcceptanceBlocked || item.Reason == SemanticAcceptanceReasonUnsafeOrPrivate {
		return SemanticCalibrationFailureBlockedPrivate
	}
	if item.AcceptanceState == SemanticAcceptanceAccepted && item.Reason == SemanticAcceptanceReasonCorrect {
		return SemanticCalibrationFailureAccepted
	}
	switch item.Reason {
	case SemanticAcceptanceReasonDuplicate:
		return SemanticCalibrationFailureDuplicate
	case SemanticAcceptanceReasonMissingEvidence, SemanticAcceptanceReasonUnsupportedEvidence:
		return SemanticCalibrationFailureMissingEvidence
	case SemanticAcceptanceReasonUnexpectedCandidate:
		return SemanticCalibrationFailureFalsePositive
	case SemanticAcceptanceReasonAmbiguous, SemanticAcceptanceReasonTooBroad, SemanticAcceptanceReasonTooNarrow, SemanticAcceptanceReasonStaleOrContradicted:
		return SemanticCalibrationFailureNeedsReviewAmbiguity
	}
	switch item.AcceptanceState {
	case SemanticAcceptanceNeedsReview, SemanticAcceptanceNeedsMerge, SemanticAcceptanceNeedsSplit:
		return SemanticCalibrationFailureNeedsReviewAmbiguity
	}
	return SemanticCalibrationFailureOther
}

func emptySemanticCalibrationFailureClassCounts() map[SemanticCalibrationFailureClass]int {
	counts := map[SemanticCalibrationFailureClass]int{}
	for _, class := range semanticCalibrationFailureClasses {
		counts[class] = 0
	}
	return counts
}

type semanticCalibrationSourceContext struct {
	Label string
	Lines []string
}

func loadSemanticCalibrationSource(options SemanticCalibrationOptions) (semanticCalibrationSourceContext, error) {
	if strings.TrimSpace(options.SourceRoot) == "" && strings.TrimSpace(options.SourcePath) == "" {
		return semanticCalibrationSourceContext{}, nil
	}
	if strings.TrimSpace(options.SourceRoot) == "" || strings.TrimSpace(options.SourcePath) == "" {
		return semanticCalibrationSourceContext{}, fmt.Errorf("source context requires --source-root and --source")
	}
	if filepath.IsAbs(options.SourcePath) {
		return semanticCalibrationSourceContext{}, fmt.Errorf("source path must be relative")
	}
	cleanSource := filepath.Clean(options.SourcePath)
	if cleanSource == "." || cleanSource == ".." || strings.HasPrefix(cleanSource, ".."+string(filepath.Separator)) {
		return semanticCalibrationSourceContext{}, fmt.Errorf("source path escapes source root")
	}
	if strings.ToLower(filepath.Ext(cleanSource)) != ".md" {
		return semanticCalibrationSourceContext{}, fmt.Errorf("source path must be markdown")
	}
	root, err := filepath.Abs(options.SourceRoot)
	if err != nil {
		return semanticCalibrationSourceContext{}, err
	}
	if err := rejectSymlinkAncestors(root); err != nil {
		return semanticCalibrationSourceContext{}, err
	}
	if err := rejectIfSymlink(root); err != nil {
		return semanticCalibrationSourceContext{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return semanticCalibrationSourceContext{}, err
	}
	if !info.IsDir() {
		return semanticCalibrationSourceContext{}, fmt.Errorf("source root is not a directory")
	}
	target := filepath.Join(root, cleanSource)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return semanticCalibrationSourceContext{}, err
	}
	if !isInside(root, targetAbs) {
		return semanticCalibrationSourceContext{}, fmt.Errorf("source path escapes source root")
	}
	if err := rejectSymlinkAncestors(targetAbs); err != nil {
		return semanticCalibrationSourceContext{}, err
	}
	if err := rejectIfSymlink(targetAbs); err != nil {
		return semanticCalibrationSourceContext{}, err
	}
	sourceInfo, err := os.Stat(targetAbs)
	if err != nil {
		return semanticCalibrationSourceContext{}, err
	}
	if sourceInfo.IsDir() {
		return semanticCalibrationSourceContext{}, fmt.Errorf("source path must be a file")
	}
	data, err := os.ReadFile(targetAbs)
	if err != nil {
		return semanticCalibrationSourceContext{}, err
	}
	return semanticCalibrationSourceContext{
		Label: filepath.ToSlash(cleanSource),
		Lines: semanticCalibrationSourceLines(string(data)),
	}, nil
}

func semanticCalibrationSourceLines(text string) []string {
	lines := strings.Split(text, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func semanticCalibrationExpectedOutcomeContext(outcome SemanticExpectedOutcomeResult, matchedCandidateID string) SemanticCalibrationExpectedOutcomeContext {
	if outcome.ExpectedOutcomeID == "" {
		return SemanticCalibrationExpectedOutcomeContext{
			LegacyContext: true,
			Completeness:  "unavailable",
		}
	}
	legacy := outcome.SchemaVersion == SemanticAcceptanceExpectedOutcomeLegacySchemaVersion
	completeness := "complete"
	if legacy {
		completeness = "legacy_unavailable"
	}
	if matchedCandidateID == "" {
		matchedCandidateID = outcome.MatchedCandidateID
	}
	return SemanticCalibrationExpectedOutcomeContext{
		ExpectedOutcomeID:      outcome.ExpectedOutcomeID,
		ExpectedState:          outcome.ExpectedState,
		ExpectedKind:           outcome.ExpectedKind,
		MatchedCandidateID:     matchedCandidateID,
		RequiredEvidence:       cloneStringList(outcome.RequiredEvidence),
		AcceptableAlternates:   cloneStringList(outcome.AcceptableAlternates),
		TitleSignals:           cloneStringList(outcome.TitleSignals),
		SummarySignals:         cloneStringList(outcome.SummarySignals),
		RelationRequirements:   append([]SemanticRelationshipType(nil), outcome.RelationRequirements...),
		MinimumConfidenceFloor: outcome.MinimumConfidenceFloor,
		Notes:                  outcome.Notes,
		LegacyContext:          legacy,
		Completeness:           completeness,
	}
}

func semanticExpectedOutcomeHasRichContext(outcome SemanticExpectedOutcomeResult) bool {
	return hasNonBlankString(outcome.RequiredEvidence) ||
		hasNonBlankString(outcome.AcceptableAlternates) ||
		hasNonBlankString(outcome.TitleSignals) ||
		hasNonBlankString(outcome.SummarySignals) ||
		len(outcome.RelationRequirements) > 0 ||
		outcome.MinimumConfidenceFloor != "" ||
		strings.TrimSpace(outcome.Notes) != ""
}

func semanticExpectedOutcomeNeedsCalibrationContext(outcome SemanticExpectedOutcomeResult, referencedByItem bool) bool {
	if outcome.ExpectedState == ExpectedOutcomePresent {
		return true
	}
	return referencedByItem
}

func validateRichSemanticExpectedOutcomeResult(outcome SemanticExpectedOutcomeResult) error {
	missing := []string{}
	if !hasNonBlankString(outcome.RequiredEvidence) {
		missing = append(missing, "required_evidence")
	}
	if !hasNonBlankString(outcome.AcceptableAlternates) {
		missing = append(missing, "acceptable_evidence_alternates")
	}
	if !hasNonBlankString(outcome.TitleSignals) {
		missing = append(missing, "title_signals")
	}
	if !hasNonBlankString(outcome.SummarySignals) {
		missing = append(missing, "summary_signals")
	}
	if len(outcome.RelationRequirements) == 0 {
		missing = append(missing, "relation_requirements")
	}
	if outcome.MinimumConfidenceFloor == "" {
		missing = append(missing, "minimum_confidence_floor")
	}
	if strings.TrimSpace(outcome.Notes) == "" {
		missing = append(missing, "notes")
	}
	if len(missing) > 0 {
		return fmt.Errorf("semantic acceptance expected outcome lacks rich review context: %s missing %s", outcome.ExpectedOutcomeID, strings.Join(missing, ", "))
	}
	return nil
}

func semanticCalibrationExpectedOutcomeBody(outcome SemanticCalibrationExpectedOutcomeContext) string {
	var b strings.Builder
	b.WriteString(outcome.ExpectedOutcomeID)
	b.WriteString("\n" + string(outcome.ExpectedState))
	b.WriteString("\n" + string(outcome.ExpectedKind))
	b.WriteString("\n" + outcome.MatchedCandidateID)
	b.WriteString("\n" + strings.Join(outcome.RequiredEvidence, "\n"))
	b.WriteString("\n" + strings.Join(outcome.AcceptableAlternates, "\n"))
	b.WriteString("\n" + strings.Join(outcome.TitleSignals, "\n"))
	b.WriteString("\n" + strings.Join(outcome.SummarySignals, "\n"))
	for _, requirement := range outcome.RelationRequirements {
		b.WriteString("\n" + string(requirement))
	}
	b.WriteString("\n" + string(outcome.MinimumConfidenceFloor))
	b.WriteString("\n" + outcome.Notes)
	b.WriteString("\n" + outcome.Completeness)
	return b.String()
}

func semanticCalibrationEvidenceExcerpts(source semanticCalibrationSourceContext, ranges []SemanticEvidenceRange) []SemanticCalibrationEvidenceExcerpt {
	if len(ranges) == 0 {
		return []SemanticCalibrationEvidenceExcerpt{semanticCalibrationUnavailableExcerpt(source.Label, "", "no evidence ranges")}
	}
	if len(source.Lines) == 0 {
		return []SemanticCalibrationEvidenceExcerpt{semanticCalibrationUnavailableExcerpt(source.Label, ranges[0].StructureNodeID, "source excerpts unavailable")}
	}
	limit := len(ranges)
	if limit > maxSemanticCalibrationExcerptRanges {
		limit = maxSemanticCalibrationExcerptRanges
	}
	out := make([]SemanticCalibrationEvidenceExcerpt, 0, limit)
	totalChars := 0
	for i := 0; i < limit && totalChars < maxSemanticCalibrationExcerptCharsPerItem; i++ {
		excerpt := semanticCalibrationEvidenceExcerpt(source, ranges[i], maxSemanticCalibrationExcerptCharsPerItem-totalChars)
		totalChars += len(excerpt.Text)
		out = append(out, excerpt)
	}
	if len(out) == 0 {
		return []SemanticCalibrationEvidenceExcerpt{semanticCalibrationUnavailableExcerpt(source.Label, "", "source excerpts unavailable")}
	}
	return out
}

func semanticCalibrationEvidenceExcerpt(source semanticCalibrationSourceContext, evidenceRange SemanticEvidenceRange, remainingChars int) SemanticCalibrationEvidenceExcerpt {
	lineCount := len(source.Lines)
	start := evidenceRange.LineStart
	end := evidenceRange.LineEnd
	clamped := false
	if start < 1 {
		start = 1
		clamped = true
	}
	if end < start {
		end = start
		clamped = true
	}
	if lineCount == 0 {
		return semanticCalibrationUnavailableExcerpt(source.Label, evidenceRange.StructureNodeID, "source excerpts unavailable")
	}
	if start > lineCount {
		start = lineCount
		end = lineCount
		clamped = true
	}
	if end > lineCount {
		end = lineCount
		clamped = true
	}
	if end-start+1 > maxSemanticCalibrationExcerptLines {
		end = start + maxSemanticCalibrationExcerptLines - 1
		clamped = true
	}
	text := strings.Join(source.Lines[start-1:end], "\n")
	if len(text) > maxSemanticCalibrationExcerptCharsPerRange {
		text = text[:maxSemanticCalibrationExcerptCharsPerRange]
		clamped = true
	}
	if remainingChars > 0 && len(text) > remainingChars {
		text = text[:remainingChars]
		clamped = true
	}
	return SemanticCalibrationEvidenceExcerpt{
		SourceLabel:     source.Label,
		StructureNodeID: evidenceRange.StructureNodeID,
		LineStart:       start,
		LineEnd:         end,
		Text:            text,
		Clamped:         clamped,
	}
}

func semanticCalibrationUnavailableExcerpt(sourceLabel, nodeID, reason string) SemanticCalibrationEvidenceExcerpt {
	return SemanticCalibrationEvidenceExcerpt{
		SourceLabel:       sourceLabel,
		StructureNodeID:   nodeID,
		Unavailable:       true,
		UnavailableReason: reason,
	}
}

func readSemanticAcceptanceBundle(root string) (SemanticAcceptanceSummary, []SemanticAcceptanceItem, []SemanticExpectedOutcomeResult, error) {
	summaryPath, err := containedSemanticCalibrationPath(root, "acceptance-summary.json")
	if err != nil {
		return SemanticAcceptanceSummary{}, nil, nil, err
	}
	var summary SemanticAcceptanceSummary
	if err := readJSONFile(summaryPath, &summary); err != nil {
		return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("read acceptance summary: %w", err)
	}
	if err := ValidateSemanticAcceptanceSummary(summary); err != nil {
		return SemanticAcceptanceSummary{}, nil, nil, err
	}
	for _, itemSummary := range summary.Candidates {
		if _, err := containedSemanticCalibrationPath(root, itemSummary.ItemPath); err != nil {
			return SemanticAcceptanceSummary{}, nil, nil, err
		}
		if _, err := containedSemanticCalibrationPath(root, itemSummary.PreviewPath); err != nil {
			return SemanticAcceptanceSummary{}, nil, nil, err
		}
	}
	for _, expectedSummary := range summary.ExpectedOutcomes {
		if _, err := containedSemanticCalibrationPath(root, expectedSummary.ExpectedPath); err != nil {
			return SemanticAcceptanceSummary{}, nil, nil, err
		}
	}
	items := make([]SemanticAcceptanceItem, 0, len(summary.Candidates))
	for _, itemSummary := range summary.Candidates {
		itemPath, err := containedSemanticCalibrationPath(root, itemSummary.ItemPath)
		if err != nil {
			return SemanticAcceptanceSummary{}, nil, nil, err
		}
		var item SemanticAcceptanceItem
		if err := readJSONFile(itemPath, &item); err != nil {
			return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("read acceptance item: %w", err)
		}
		if item.SchemaVersion != SemanticAcceptanceItemSchemaVersion {
			return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("unsupported semantic acceptance item schema version: %s", item.SchemaVersion)
		}
		if item.CandidateID != itemSummary.CandidateID {
			return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("acceptance item id mismatch: %s", itemSummary.CandidateID)
		}
		if item.CandidateKind != itemSummary.CandidateKind || item.AcceptanceState != itemSummary.AcceptanceState || item.Reason != itemSummary.Reason {
			return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("acceptance item summary mismatch: %s", itemSummary.CandidateID)
		}
		items = append(items, item)
	}
	expected := make([]SemanticExpectedOutcomeResult, 0, len(summary.ExpectedOutcomes))
	referencedExpectedOutcomes := semanticCalibrationReferencedExpectedOutcomes(items)
	for _, expectedSummary := range summary.ExpectedOutcomes {
		expectedPath, err := containedSemanticCalibrationPath(root, expectedSummary.ExpectedPath)
		if err != nil {
			return SemanticAcceptanceSummary{}, nil, nil, err
		}
		var outcome SemanticExpectedOutcomeResult
		if err := readJSONFile(expectedPath, &outcome); err != nil {
			return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("read expected outcome: %w", err)
		}
		if outcome.SchemaVersion != SemanticAcceptanceExpectedOutcomeSchemaVersion && outcome.SchemaVersion != SemanticAcceptanceExpectedOutcomeLegacySchemaVersion {
			return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("unsupported semantic acceptance expected outcome schema version: %s", outcome.SchemaVersion)
		}
		if outcome.SchemaVersion == SemanticAcceptanceExpectedOutcomeSchemaVersion && semanticExpectedOutcomeNeedsCalibrationContext(outcome, referencedExpectedOutcomes[outcome.ExpectedOutcomeID]) {
			if err := validateRichSemanticExpectedOutcomeResult(outcome); err != nil {
				return SemanticAcceptanceSummary{}, nil, nil, err
			}
		}
		if _, err := containedSemanticCalibrationPath(root, outcome.ExpectedPath); err != nil {
			return SemanticAcceptanceSummary{}, nil, nil, err
		}
		if filepath.Clean(outcome.ExpectedPath) != filepath.Clean(expectedSummary.ExpectedPath) {
			return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("expected outcome path mismatch: %s", outcome.ExpectedOutcomeID)
		}
		if outcome.ExpectedOutcomeID != expectedSummary.ExpectedOutcomeID ||
			outcome.ExpectedState != expectedSummary.ExpectedState ||
			outcome.ExpectedKind != expectedSummary.ExpectedKind ||
			outcome.AcceptanceState != expectedSummary.AcceptanceState ||
			outcome.Reason != expectedSummary.Reason ||
			outcome.MatchedCandidateID != expectedSummary.MatchedCandidateID {
			return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("expected outcome summary mismatch: %s", expectedSummary.ExpectedOutcomeID)
		}
		expected = append(expected, outcome)
	}
	if err := validateSemanticCalibrationExpectedOutcomeReferences(items, expected); err != nil {
		return SemanticAcceptanceSummary{}, nil, nil, err
	}
	return summary, items, expected, nil
}

func semanticCalibrationReferencedExpectedOutcomes(items []SemanticAcceptanceItem) map[string]bool {
	referenced := map[string]bool{}
	for _, item := range items {
		if strings.TrimSpace(item.ExpectedOutcomeID) != "" {
			referenced[item.ExpectedOutcomeID] = true
		}
	}
	return referenced
}

func validateSemanticCalibrationExpectedOutcomeReferences(items []SemanticAcceptanceItem, expected []SemanticExpectedOutcomeResult) error {
	expectedByID := map[string]bool{}
	for _, outcome := range expected {
		expectedByID[outcome.ExpectedOutcomeID] = true
	}
	for _, item := range items {
		if strings.TrimSpace(item.ExpectedOutcomeID) == "" {
			continue
		}
		if !expectedByID[item.ExpectedOutcomeID] {
			return fmt.Errorf("acceptance item references missing expected outcome: %s", item.ExpectedOutcomeID)
		}
	}
	return nil
}

func readSemanticCalibrationSummary(root string) (SemanticCalibrationSummary, error) {
	summaryPath, err := containedSemanticCalibrationPath(root, "calibration-summary.json")
	if err != nil {
		return SemanticCalibrationSummary{}, err
	}
	var summary SemanticCalibrationSummary
	if err := readJSONFile(summaryPath, &summary); err != nil {
		return SemanticCalibrationSummary{}, fmt.Errorf("read calibration summary: %w", err)
	}
	if err := ValidateSemanticCalibrationSummary(summary); err != nil {
		return SemanticCalibrationSummary{}, err
	}
	return summary, nil
}

func readSemanticCalibrationCursor(root string, summary SemanticCalibrationSummary) (SemanticCalibrationCursor, error) {
	cursorPath, err := containedSemanticCalibrationPath(root, summary.CursorPath)
	if err != nil {
		return SemanticCalibrationCursor{}, err
	}
	var cursor SemanticCalibrationCursor
	if err := readJSONFile(cursorPath, &cursor); err != nil {
		return SemanticCalibrationCursor{}, fmt.Errorf("read calibration cursor: %w", err)
	}
	if cursor.SchemaVersion != SemanticCalibrationCursorSchemaVersion {
		return SemanticCalibrationCursor{}, fmt.Errorf("unsupported semantic calibration cursor schema version: %s", cursor.SchemaVersion)
	}
	return cursor, nil
}

func validateSemanticCalibrationCursor(cursor SemanticCalibrationCursor, summary SemanticCalibrationSummary) error {
	if cursor.RunID != summary.RunID {
		return fmt.Errorf("calibration cursor run id mismatch")
	}
	if cursor.TotalCount != len(summary.ReviewItems) {
		return fmt.Errorf("calibration cursor total does not match review item count")
	}
	if cursor.NextIndex < 0 || cursor.NextIndex > cursor.TotalCount {
		return fmt.Errorf("invalid calibration cursor next index")
	}
	if cursor.ProcessedCount != cursor.NextIndex {
		return fmt.Errorf("invalid calibration cursor processed count")
	}
	if cursor.RemainingCount != cursor.TotalCount-cursor.NextIndex {
		return fmt.Errorf("invalid calibration cursor remaining count")
	}
	if cursor.Exhausted != (cursor.NextIndex == cursor.TotalCount) {
		return fmt.Errorf("invalid calibration cursor exhausted state")
	}
	return nil
}

func validateSemanticCalibrationSummaryPaths(root string, summary SemanticCalibrationSummary) error {
	if _, err := containedSemanticCalibrationPath(root, summary.CursorPath); err != nil {
		return err
	}
	if _, err := containedSemanticCalibrationPath(root, summary.ReportPath); err != nil {
		return err
	}
	for _, item := range summary.ReviewItems {
		if _, err := containedSemanticCalibrationPath(root, item.ItemPath); err != nil {
			return err
		}
		if _, err := containedSemanticCalibrationPath(root, item.PreviewPath); err != nil {
			return err
		}
	}
	return nil
}

func ValidateSemanticCalibrationReviewItem(item SemanticCalibrationReviewItem) error {
	if item.SchemaVersion != SemanticCalibrationReviewItemSchemaVersion &&
		item.SchemaVersion != SemanticCalibrationReviewItemPreviousSchemaVersion &&
		item.SchemaVersion != SemanticCalibrationReviewItemLegacySchemaVersion {
		return fmt.Errorf("unsupported semantic calibration review item schema version: %s", item.SchemaVersion)
	}
	if item.SchemaVersion == SemanticCalibrationReviewItemSchemaVersion {
		if item.FailureReason != "" && !validSemanticFailureReason(item.FailureReason) {
			return fmt.Errorf("unsupported semantic calibration failure reason: %s", item.FailureReason)
		}
		if item.NeedsAdjudication && item.FailureReason == "" {
			return fmt.Errorf("semantic calibration adjudication item requires failure reason: %s", item.ReviewItemID)
		}
	}
	body := item.RunID + "\n" + item.ReviewItemID + "\n" + item.CandidateID + "\n" + item.ExpectedOutcomeID
	body += "\n" + item.SourceDocumentID + "\n" + item.Title + "\n" + item.Summary + "\n" + strings.Join(item.EvidenceNodes, "\n")
	body += "\n" + strings.Join(item.RelationIDs, "\n")
	body += "\n" + semanticCalibrationExpectedOutcomeBody(item.ExpectedOutcome)
	for _, excerpt := range item.EvidenceExcerpts {
		body += "\n" + excerpt.SourceLabel + "\n" + excerpt.StructureNodeID + "\n" + excerpt.Text + "\n" + excerpt.UnavailableReason
	}
	for _, blocker := range item.Blockers {
		body += "\n" + blocker.Code + "\n" + blocker.Message
	}
	for _, evidenceRange := range item.EvidenceRanges {
		body += "\n" + evidenceRange.StructureNodeID
	}
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		return fmt.Errorf("semantic calibration review item contains private marker")
	}
	return nil
}

func normalizeLegacySemanticCalibrationReviewItem(item *SemanticCalibrationReviewItem) {
	if item.ExpectedOutcome.ExpectedOutcomeID == "" && item.ExpectedOutcomeID != "" {
		item.ExpectedOutcome = SemanticCalibrationExpectedOutcomeContext{
			ExpectedOutcomeID: item.ExpectedOutcomeID,
			LegacyContext:     true,
			Completeness:      "legacy_unavailable",
		}
	}
	if len(item.EvidenceExcerpts) == 0 {
		item.EvidenceExcerpts = semanticCalibrationEvidenceExcerpts(semanticCalibrationSourceContext{}, item.EvidenceRanges)
	}
	if item.FailureReason == "" {
		reason, inferred, _ := semanticFailureReasonForCalibrationItem(item.Reason, item.FailureClass)
		item.FailureReason = reason
		item.FailureInferred = inferred
	}
	item.SchemaVersion = SemanticCalibrationReviewItemSchemaVersion
}

func resolveSemanticAcceptanceRoot(path string) (string, error) {
	return resolveNamedArtifactRoot(path, "semantic-acceptance")
}

func resolveSemanticCalibrationRoot(path string) (string, error) {
	return resolveNamedArtifactRoot(path, "semantic-calibration")
}

func resolveNamedArtifactRoot(path, name string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("missing %s path", name)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	candidate := abs
	if filepath.Base(candidate) != name {
		candidate = filepath.Join(abs, name)
	}
	if err := rejectSymlinkAncestors(candidate); err != nil {
		return "", err
	}
	if err := rejectIfSymlink(candidate); err != nil {
		return "", err
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", candidate)
	}
	realRoot, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	return realRoot, nil
}

func containedSemanticCalibrationPath(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("unsafe semantic calibration artifact path: %s", relative)
	}
	cleanRelative := filepath.Clean(relative)
	if cleanRelative == "." || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe semantic calibration artifact path: %s", relative)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, cleanRelative))
	if err != nil {
		return "", err
	}
	if targetAbs == rootAbs || !isInside(rootAbs, targetAbs) {
		return "", fmt.Errorf("semantic calibration artifact path escapes root: %s", relative)
	}
	if err := rejectSymlinkAncestors(targetAbs); err != nil {
		return "", err
	}
	if err := rejectIfSymlink(targetAbs); err != nil {
		return "", err
	}
	return targetAbs, nil
}

func readJSONFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return err
	}
	return nil
}

func semanticCalibrationReviewContext(item SemanticCalibrationReviewItem) *SemanticCalibrationReviewContext {
	return &SemanticCalibrationReviewContext{
		ReviewItemID:        item.ReviewItemID,
		SourceDocumentID:    item.SourceDocumentID,
		SourceLabel:         firstSemanticCalibrationSourceLabel(item.EvidenceExcerpts),
		CandidateID:         item.CandidateID,
		ExpectedOutcome:     item.ExpectedOutcome,
		FailureClass:        item.FailureClass,
		FailureReason:       item.FailureReason,
		FailureInferred:     item.FailureInferred,
		AcceptanceState:     item.AcceptanceState,
		Reason:              item.Reason,
		EvidenceExcerpts:    append([]SemanticCalibrationEvidenceExcerpt(nil), item.EvidenceExcerpts...),
		AdjudicationChoices: semanticCalibrationAdjudicationChoices(),
	}
}

func firstSemanticCalibrationSourceLabel(excerpts []SemanticCalibrationEvidenceExcerpt) string {
	for _, excerpt := range excerpts {
		if strings.TrimSpace(excerpt.SourceLabel) != "" {
			return excerpt.SourceLabel
		}
	}
	return ""
}

func semanticCalibrationAdjudicationChoices() []SemanticCalibrationAdjudicationChoice {
	return []SemanticCalibrationAdjudicationChoice{
		{Choice: "accept", Meaning: "candidate is correct calibration evidence"},
		{Choice: "reject_false_positive", Meaning: "candidate should not have been emitted"},
		{Choice: "missing_evidence", Meaning: "candidate may be right but lacks required evidence"},
		{Choice: "ambiguous", Meaning: "candidate cannot be judged from available evidence"},
		{Choice: "wrong_kind_or_scope", Meaning: "candidate kind or source scope is wrong"},
		{Choice: "block_private_governance", Meaning: "candidate or evidence contains private/governance material"},
	}
}

func semanticCalibrationPageMarkdown(item SemanticCalibrationReviewItem) string {
	var b strings.Builder
	b.WriteString("# Semantic calibration item\n\n")
	b.WriteString("- Review item: " + item.ReviewItemID + "\n")
	if item.SourceDocumentID != "" {
		b.WriteString("- Source document: " + item.SourceDocumentID + "\n")
	}
	if item.CandidateID != "" {
		b.WriteString("- Candidate: " + item.CandidateID + "\n")
	}
	if item.Title != "" {
		b.WriteString("- Title: " + item.Title + "\n")
	} else {
		b.WriteString("- Title: no title\n")
	}
	b.WriteString("- Kind: " + string(item.CandidateKind) + "\n")
	b.WriteString("- Confidence: " + string(item.Confidence) + "\n")
	b.WriteString("- Review status: " + string(item.ReviewStatus) + "\n")
	b.WriteString("- Failure class: " + string(item.FailureClass) + "\n")
	if item.FailureReason != "" {
		b.WriteString("- Failure reason: " + string(item.FailureReason) + "\n")
		b.WriteString(fmt.Sprintf("- Failure reason inferred: %t\n", item.FailureInferred))
	}
	b.WriteString("- Acceptance state: " + string(item.AcceptanceState) + "\n")
	b.WriteString("- Reason: " + string(item.Reason) + "\n")
	if item.Summary != "" {
		b.WriteString("\n## Candidate summary\n\n")
		b.WriteString(item.Summary + "\n")
	} else {
		b.WriteString("\n## Candidate summary\n\nno summary\n")
	}
	b.WriteString("\n## Expected outcome\n\n")
	b.WriteString(semanticCalibrationExpectedOutcomeMarkdown(item.ExpectedOutcome, item.FailureClass))
	b.WriteString("\n## Evidence\n\n")
	if len(item.EvidenceRanges) == 0 {
		b.WriteString("- Evidence ranges: unavailable\n")
	} else {
		for _, evidenceRange := range item.EvidenceRanges {
			b.WriteString(fmt.Sprintf("- %s lines %d-%d\n", evidenceRange.StructureNodeID, evidenceRange.LineStart, evidenceRange.LineEnd))
		}
	}
	b.WriteString("\n## Source excerpts\n\n")
	for _, excerpt := range item.EvidenceExcerpts {
		if excerpt.Unavailable {
			reason := excerpt.UnavailableReason
			if reason == "" {
				reason = "source excerpts unavailable"
			}
			b.WriteString("- " + reason + "\n")
			continue
		}
		b.WriteString(fmt.Sprintf("### %s lines %d-%d\n\n", excerpt.SourceLabel, excerpt.LineStart, excerpt.LineEnd))
		b.WriteString(excerpt.Text + "\n")
		if excerpt.Clamped {
			b.WriteString("\n(clamped)\n")
		}
	}
	if len(item.Blockers) > 0 {
		b.WriteString("\n## Blockers\n\n")
		for _, blocker := range item.Blockers {
			b.WriteString("- " + blocker.Code + ": " + blocker.Message + "\n")
		}
	}
	b.WriteString("\n## Adjudication choices\n\n")
	for _, choice := range semanticCalibrationAdjudicationChoices() {
		b.WriteString("- " + choice.Choice + ": " + choice.Meaning + "\n")
	}
	return b.String()
}

func semanticCalibrationPreviewMarkdown(item SemanticCalibrationReviewItem) string {
	return semanticCalibrationPageMarkdown(item)
}

func semanticCalibrationExpectedOutcomeMarkdown(outcome SemanticCalibrationExpectedOutcomeContext, failureClass SemanticCalibrationFailureClass) string {
	var b strings.Builder
	if outcome.ExpectedOutcomeID == "" {
		b.WriteString("- Expected outcome: unavailable\n")
		return b.String()
	}
	b.WriteString("- Expected outcome: " + outcome.ExpectedOutcomeID + "\n")
	b.WriteString("- Expected state: " + string(outcome.ExpectedState) + "\n")
	b.WriteString("- Expected kind: " + string(outcome.ExpectedKind) + "\n")
	if outcome.MatchedCandidateID != "" {
		b.WriteString("- Matched candidate: " + outcome.MatchedCandidateID + "\n")
	} else if failureClass == SemanticCalibrationFailureFalseNegative {
		b.WriteString("- No candidate matched this expected outcome.\n")
	}
	writeListOrUnavailable(&b, "Required evidence", outcome.RequiredEvidence)
	writeListOrUnavailable(&b, "Acceptable alternates", outcome.AcceptableAlternates)
	writeListOrUnavailable(&b, "Title signals", outcome.TitleSignals)
	writeListOrUnavailable(&b, "Summary signals", outcome.SummarySignals)
	if len(outcome.RelationRequirements) == 0 {
		b.WriteString("- Relation requirements: unavailable\n")
	} else {
		parts := make([]string, 0, len(outcome.RelationRequirements))
		for _, requirement := range outcome.RelationRequirements {
			parts = append(parts, string(requirement))
		}
		b.WriteString("- Relation requirements: " + strings.Join(parts, ", ") + "\n")
	}
	if outcome.MinimumConfidenceFloor == "" {
		b.WriteString("- Minimum confidence floor: unavailable\n")
	} else {
		b.WriteString("- Minimum confidence floor: " + string(outcome.MinimumConfidenceFloor) + "\n")
	}
	if outcome.Notes == "" {
		b.WriteString("- Notes: unavailable\n")
	} else {
		b.WriteString("- Notes: " + outcome.Notes + "\n")
	}
	if outcome.LegacyContext {
		b.WriteString("- Legacy context: true\n")
		b.WriteString("- Legacy calibration input lacks full expected-outcome context; this page is not fully adjudication-ready.\n")
	}
	return b.String()
}

func writeListOrUnavailable(b *strings.Builder, label string, values []string) {
	if !hasNonBlankString(values) {
		b.WriteString("- " + label + ": unavailable\n")
		return
	}
	b.WriteString("- " + label + ": " + strings.Join(values, ", ") + "\n")
}

func semanticCalibrationReportMarkdown(summary SemanticCalibrationSummary) string {
	var b strings.Builder
	b.WriteString("# Semantic Calibration Report\n\n")
	b.WriteString(summary.QualityStatement + "\n\n")
	b.WriteString("Human adjudication here is temporary calibration evidence, not the steady-state workflow.\n\n")
	b.WriteString(fmt.Sprintf("- Held out: %t\n", summary.HeldOut))
	b.WriteString(fmt.Sprintf("- Threshold: %.2f\n", summary.Threshold))
	b.WriteString(fmt.Sprintf("- Threshold status: %s\n", summary.ThresholdStatus))
	b.WriteString(fmt.Sprintf("- No-human eligible: %t\n", summary.NoHumanEligible))
	b.WriteString(fmt.Sprintf("- Measured accuracy: %.2f\n", summary.MeasuredAccuracy))
	b.WriteString(fmt.Sprintf("- Scored items: %d\n", summary.ScoredCount))
	b.WriteString(fmt.Sprintf("- False positives: %d\n", summary.FalsePositiveCount))
	b.WriteString(fmt.Sprintf("- False negatives: %d\n", summary.FalseNegativeCount))
	b.WriteString(fmt.Sprintf("- Needs review: %d\n", summary.NeedsReviewCount))
	b.WriteString(fmt.Sprintf("- Review burden rate: %.2f\n", summary.ReviewBurdenRate))
	b.WriteString(fmt.Sprintf("- Blocked/private: %d\n", summary.BlockedPrivateCount))
	b.WriteString("\n## Failure reasons\n\n")
	for _, reason := range semanticFailureReasons() {
		b.WriteString(fmt.Sprintf("- %s: %d\n", reason, summary.FailureReasonCounts[reason]))
	}
	b.WriteString("\n## Compatibility rollups\n\n")
	for _, class := range semanticCalibrationFailureClasses {
		b.WriteString(fmt.Sprintf("- %s: %d\n", class, summary.FailureClassCounts[class]))
	}
	return b.String()
}

func semanticCalibrationQualityStatement(heldOut bool) string {
	if heldOut {
		return "This calibration uses held-out acceptance evidence for the measured trust gate."
	}
	return "This calibration is a mechanical pipeline evaluation only; generated or self-derived labels cannot prove semantic accuracy."
}
