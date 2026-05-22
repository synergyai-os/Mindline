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
	threshold := options.Threshold
	if threshold < defaultSemanticCalibrationThreshold {
		threshold = defaultSemanticCalibrationThreshold
	}
	summary := BuildSemanticCalibrationSummary(acceptance, items, expected, threshold, options.HeldOut)
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
	if item.SchemaVersion != SemanticCalibrationReviewItemSchemaVersion {
		return SemanticCalibrationPage{}, fmt.Errorf("unsupported semantic calibration review item schema version: %s", item.SchemaVersion)
	}
	if item.ReviewItemID != itemSummary.ReviewItemID {
		return SemanticCalibrationPage{}, fmt.Errorf("calibration review item id mismatch: %s", itemSummary.ReviewItemID)
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
	}, nil
}

func BuildSemanticCalibrationSummary(acceptance SemanticAcceptanceSummary, items []SemanticAcceptanceItem, expected []SemanticExpectedOutcomeResult, threshold float64, heldOut bool) SemanticCalibrationSummary {
	reviewItems := semanticCalibrationReviewItems(acceptance.RunID, items, expected)
	counts := emptySemanticCalibrationFailureClassCounts()
	for _, item := range reviewItems {
		counts[item.FailureClass]++
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
		if err := writeFile(realRoot, item.PreviewPath, []byte(semanticCalibrationPreviewMarkdown(item))); err != nil {
			return err
		}
	}
	if err := writeFile(realRoot, "reports/calibration-report.md", []byte(semanticCalibrationReportMarkdown(summary))); err != nil {
		return err
	}
	return nil
}

func ValidateSemanticCalibrationSummary(summary SemanticCalibrationSummary) error {
	if summary.SchemaVersion != SemanticCalibrationSummarySchemaVersion {
		return fmt.Errorf("unsupported semantic calibration summary schema version: %s", summary.SchemaVersion)
	}
	if summary.FailureClassCounts == nil {
		return fmt.Errorf("missing semantic calibration failure class counts")
	}
	body := summary.RunID + "\n" + summary.AnswerKeyID + "\n" + summary.QualityStatement
	for _, item := range summary.ReviewItems {
		body += "\n" + item.ReviewItemID + "\n" + item.ItemPath + "\n" + item.PreviewPath
	}
	for _, item := range summary.Items {
		body += "\n" + item.RunID + "\n" + item.ReviewItemID + "\n" + item.CandidateID + "\n" + item.ExpectedOutcomeID
		body += "\n" + item.SourceDocumentID + "\n" + item.Title + "\n" + item.Summary + "\n" + strings.Join(item.EvidenceNodes, "\n")
		body += "\n" + strings.Join(item.RelationIDs, "\n")
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

func semanticCalibrationReviewItems(runID string, items []SemanticAcceptanceItem, expected []SemanticExpectedOutcomeResult) []SemanticCalibrationReviewItem {
	out := make([]SemanticCalibrationReviewItem, 0, len(items)+len(expected))
	for _, item := range items {
		failureClass := semanticCalibrationFailureClassForItem(item)
		out = append(out, SemanticCalibrationReviewItem{
			SchemaVersion:     SemanticCalibrationReviewItemSchemaVersion,
			ReviewItemID:      "review-" + item.CandidateID,
			RunID:             item.RunID,
			SourceDocumentID:  item.SourceDocumentID,
			CandidateID:       item.CandidateID,
			ExpectedOutcomeID: item.ExpectedOutcomeID,
			CandidateKind:     item.CandidateKind,
			ReviewStatus:      item.ReviewStatus,
			Confidence:        item.Confidence,
			Title:             item.Title,
			Summary:           item.Summary,
			EvidenceNodes:     cloneStringList(item.EvidenceNodes),
			EvidenceRanges:    cloneSemanticEvidenceRanges(item.EvidenceRanges),
			RelationIDs:       cloneStringList(item.RelationIDs),
			AcceptanceState:   item.AcceptanceState,
			Reason:            item.Reason,
			FailureClass:      failureClass,
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
			CandidateKind:     outcome.ExpectedKind,
			EvidenceNodes:     []string{},
			EvidenceRanges:    []SemanticEvidenceRange{},
			RelationIDs:       []string{},
			AcceptanceState:   outcome.AcceptanceState,
			Reason:            outcome.Reason,
			FailureClass:      SemanticCalibrationFailureFalseNegative,
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
	for _, expectedSummary := range summary.ExpectedOutcomes {
		expectedPath, err := containedSemanticCalibrationPath(root, expectedSummary.ExpectedPath)
		if err != nil {
			return SemanticAcceptanceSummary{}, nil, nil, err
		}
		var outcome SemanticExpectedOutcomeResult
		if err := readJSONFile(expectedPath, &outcome); err != nil {
			return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("read expected outcome: %w", err)
		}
		if outcome.SchemaVersion != SemanticAcceptanceExpectedOutcomeSchemaVersion {
			return SemanticAcceptanceSummary{}, nil, nil, fmt.Errorf("unsupported semantic acceptance expected outcome schema version: %s", outcome.SchemaVersion)
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
	return summary, items, expected, nil
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
	if item.SchemaVersion != SemanticCalibrationReviewItemSchemaVersion {
		return fmt.Errorf("unsupported semantic calibration review item schema version: %s", item.SchemaVersion)
	}
	body := item.RunID + "\n" + item.ReviewItemID + "\n" + item.CandidateID + "\n" + item.ExpectedOutcomeID
	body += "\n" + item.SourceDocumentID + "\n" + item.Title + "\n" + item.Summary + "\n" + strings.Join(item.EvidenceNodes, "\n")
	body += "\n" + strings.Join(item.RelationIDs, "\n")
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

func semanticCalibrationPreviewMarkdown(item SemanticCalibrationReviewItemSummary) string {
	var b strings.Builder
	b.WriteString("# Semantic calibration item\n\n")
	b.WriteString("- Review item: " + item.ReviewItemID + "\n")
	b.WriteString("- Failure class: " + string(item.FailureClass) + "\n")
	b.WriteString("- Acceptance state: " + string(item.AcceptanceState) + "\n")
	b.WriteString("- Reason: " + string(item.Reason) + "\n")
	return b.String()
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
	return b.String()
}

func semanticCalibrationQualityStatement(heldOut bool) string {
	if heldOut {
		return "This calibration uses held-out acceptance evidence for the measured trust gate."
	}
	return "This calibration is a mechanical pipeline evaluation only; generated or self-derived labels cannot prove semantic accuracy."
}
