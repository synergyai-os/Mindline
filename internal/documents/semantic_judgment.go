package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func JudgeSemanticCandidates(semanticRunDir, outDir string, options SemanticJudgmentOptions) (SemanticJudgmentSummary, error) {
	semanticSummary, candidates, _, err := readSemanticAcceptanceInput(semanticRunDir)
	if err != nil {
		return SemanticJudgmentSummary{}, err
	}
	source, err := loadSemanticCalibrationSource(SemanticCalibrationOptions{SourceRoot: options.SourceRoot, SourcePath: options.SourcePath})
	if err != nil {
		return SemanticJudgmentSummary{}, err
	}
	items := semanticJudgmentCandidates(candidates, source, nil)
	summary := BuildSemanticJudgmentSummary(semanticSummary.RunID, semanticSummary.SourceCount, items, nil)
	if err := WriteSemanticJudgment(outDir, summary); err != nil {
		return SemanticJudgmentSummary{}, err
	}
	return summary, nil
}

func NextSemanticJudgmentPage(inputDir string) (SemanticJudgmentPage, error) {
	root, err := resolveSemanticJudgmentRoot(inputDir)
	if err != nil {
		return SemanticJudgmentPage{}, err
	}
	summary, err := readSemanticJudgmentSummary(root)
	if err != nil {
		return SemanticJudgmentPage{}, err
	}
	items, judgments, err := readSemanticJudgmentBundle(root, summary)
	if err != nil {
		return SemanticJudgmentPage{}, err
	}
	judged := semanticJudgmentsByCandidate(judgments)
	nextIndex := len(items)
	var next *SemanticJudgmentCandidate
	for i := range items {
		if judged[items[i].CandidateID] != nil {
			continue
		}
		nextIndex = i
		item := items[i]
		next = &item
		break
	}
	cursor := SemanticJudgmentCursor{
		SchemaVersion:  SemanticJudgmentCursorSchemaVersion,
		RunID:          summary.RunID,
		NextIndex:      nextIndex,
		TotalCount:     len(items),
		JudgedCount:    len(judgments),
		RemainingCount: len(items) - len(judgments),
		Exhausted:      next == nil,
	}
	if err := writeJSON(root, "cursor.json", cursor); err != nil {
		return SemanticJudgmentPage{}, ArtifactWriteError{Err: err}
	}
	if next == nil {
		return SemanticJudgmentPage{
			SchemaVersion: SemanticJudgmentPageSchemaVersion,
			Done:          true,
			Cursor:        cursor,
		}, nil
	}
	return SemanticJudgmentPage{
		SchemaVersion: SemanticJudgmentPageSchemaVersion,
		Done:          false,
		Cursor:        cursor,
		Item:          next,
		PageMarkdown:  semanticJudgmentPageMarkdown(*next, cursor),
	}, nil
}

func RecordSemanticJudgment(inputDir string, input SemanticJudgmentRecordInput) (SemanticJudgmentSummary, error) {
	root, err := resolveSemanticJudgmentRoot(inputDir)
	if err != nil {
		return SemanticJudgmentSummary{}, err
	}
	if !validSemanticJudgmentChoice(input.Choice) {
		return SemanticJudgmentSummary{}, fmt.Errorf("unsupported semantic judgment choice: %s", input.Choice)
	}
	summary, err := readSemanticJudgmentSummary(root)
	if err != nil {
		return SemanticJudgmentSummary{}, err
	}
	items, judgments, err := readSemanticJudgmentBundle(root, summary)
	if err != nil {
		return SemanticJudgmentSummary{}, err
	}
	if semanticJudgmentsByCandidate(judgments)[input.CandidateID] != nil {
		return SemanticJudgmentSummary{}, fmt.Errorf("semantic judgment already exists: %s", input.CandidateID)
	}
	var target *SemanticJudgmentCandidate
	for i := range items {
		if items[i].CandidateID == input.CandidateID {
			target = &items[i]
			break
		}
	}
	if target == nil {
		return SemanticJudgmentSummary{}, fmt.Errorf("unknown semantic judgment candidate: %s", input.CandidateID)
	}
	recordedAt := input.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}
	record := SemanticJudgmentRecord{
		SchemaVersion:    SemanticJudgmentRecordSchemaVersion,
		RunID:            target.RunID,
		CandidateID:      target.CandidateID,
		SourceDocumentID: target.SourceDocumentID,
		CandidateKind:    target.CandidateKind,
		Confidence:       target.Confidence,
		Choice:           input.Choice,
		Note:             strings.TrimSpace(input.Note),
		ReviewerID:       strings.TrimSpace(input.ReviewerID),
		RecordedAt:       recordedAt.UTC().Format(time.RFC3339),
	}
	if err := ValidateSemanticJudgmentRecord(record); err != nil {
		return SemanticJudgmentSummary{}, err
	}
	judgments = append(judgments, record)
	judgedByCandidate := semanticJudgmentsByCandidate(judgments)
	for i := range items {
		items[i].Judgment = judgedByCandidate[items[i].CandidateID]
	}
	updated := BuildSemanticJudgmentSummary(summary.RunID, summary.SourceCount, items, judgments)
	if err := WriteSemanticJudgmentRoot(root, updated); err != nil {
		return SemanticJudgmentSummary{}, err
	}
	return updated, nil
}

func semanticJudgmentCandidates(candidates []SemanticCandidate, source semanticCalibrationSourceContext, judgments []SemanticJudgmentRecord) []SemanticJudgmentCandidate {
	judged := semanticJudgmentsByCandidate(judgments)
	out := make([]SemanticJudgmentCandidate, 0, len(candidates))
	for _, candidate := range orderSemanticCandidates(candidates) {
		item := SemanticJudgmentCandidate{
			SchemaVersion:    SemanticJudgmentCandidateSchemaVersion,
			CandidateID:      candidate.CandidateID,
			RunID:            candidate.RunID,
			SourceDocumentID: candidateSourceDocumentID(candidate),
			CandidateKind:    candidate.CandidateKind,
			ReviewStatus:     candidate.ReviewStatus,
			Confidence:       candidate.Confidence,
			Title:            candidate.Title,
			Summary:          candidate.Summary,
			EvidenceNodes:    cloneStringList(candidate.EvidenceNodes),
			EvidenceRanges:   cloneSemanticEvidenceRanges(candidate.EvidenceRanges),
			EvidenceExcerpts: semanticCalibrationEvidenceExcerpts(source, candidate.EvidenceRanges),
			RelationIDs:      cloneStringList(candidate.RelationIDs),
			Blockers:         cloneBlockerList(candidate.Blockers),
			Judgment:         judged[candidate.CandidateID],
		}
		out = append(out, item)
	}
	return out
}

func BuildSemanticJudgmentSummary(runID string, sourceCount int, items []SemanticJudgmentCandidate, judgments []SemanticJudgmentRecord) SemanticJudgmentSummary {
	judged := semanticJudgmentsByCandidate(judgments)
	counts := map[SemanticJudgmentChoice]int{
		SemanticJudgmentChoiceReject:    0,
		SemanticJudgmentChoiceUnclear:   0,
		SemanticJudgmentChoiceDuplicate: 0,
		SemanticJudgmentChoiceWrongKind: 0,
	}
	accepted := 0
	rejected := 0
	unclear := 0
	duplicate := 0
	wrongKind := 0
	blocked := 0
	skipped := 0
	summaries := make([]SemanticJudgmentCandidateSummary, 0, len(items))
	for _, item := range items {
		if item.ReviewStatus == ReviewStatusBlocked {
			blocked++
		}
		if item.ReviewStatus == ReviewStatusSkipped {
			skipped++
		}
		judgment := judged[item.CandidateID]
		choice := SemanticJudgmentChoice("")
		judgmentPath := ""
		if judgment != nil {
			choice = judgment.Choice
			judgmentPath = SemanticJudgmentRecordJSONPath(item.CandidateID)
			switch judgment.Choice {
			case SemanticJudgmentChoiceAccept:
				accepted++
			case SemanticJudgmentChoiceReject:
				rejected++
				counts[SemanticJudgmentChoiceReject]++
			case SemanticJudgmentChoiceUnclear:
				unclear++
				counts[SemanticJudgmentChoiceUnclear]++
			case SemanticJudgmentChoiceDuplicate:
				duplicate++
				counts[SemanticJudgmentChoiceDuplicate]++
			case SemanticJudgmentChoiceWrongKind:
				wrongKind++
				counts[SemanticJudgmentChoiceWrongKind]++
			}
		}
		summaries = append(summaries, SemanticJudgmentCandidateSummary{
			CandidateID:      item.CandidateID,
			CandidateKind:    item.CandidateKind,
			ReviewStatus:     item.ReviewStatus,
			Confidence:       item.Confidence,
			JudgmentChoice:   choice,
			CandidatePath:    SemanticJudgmentCandidateJSONPath(item.CandidateID),
			PagePath:         SemanticJudgmentPagePath(item.CandidateID),
			JudgmentPath:     judgmentPath,
			SourceDocumentID: item.SourceDocumentID,
		})
	}
	sort.SliceStable(summaries, func(i, j int) bool { return summaries[i].CandidateID < summaries[j].CandidateID })
	judgedCount := len(judgments)
	remaining := len(items) - judgedCount
	reviewBurden := remaining + rejected + unclear + duplicate + wrongKind
	return SemanticJudgmentSummary{
		SchemaVersion:     SemanticJudgmentSummarySchemaVersion,
		RunID:             runID,
		SourceCount:       sourceCount,
		CandidateCount:    len(items),
		JudgedCount:       judgedCount,
		RemainingCount:    remaining,
		AcceptedCount:     accepted,
		RejectedCount:     rejected,
		UnclearCount:      unclear,
		DuplicateCount:    duplicate,
		WrongKindCount:    wrongKind,
		BlockedCount:      blocked,
		SkippedCount:      skipped,
		ReviewBurdenCount: reviewBurden,
		PrecisionEstimate: ratio(accepted, judgedCount),
		FailureModeCounts: counts,
		QualityStatement:  "Judgments are calibration evidence only; no-human readiness still requires held-out >=98% accuracy.",
		CursorPath:        "cursor.json",
		ReportPath:        "reports/judgment-report.md",
		Candidates:        summaries,
		Items:             items,
		Judgments:         orderSemanticJudgmentRecords(judgments),
	}
}

func readSemanticJudgmentSummary(root string) (SemanticJudgmentSummary, error) {
	path, err := containedSemanticJudgmentPath(root, "judgment-summary.json")
	if err != nil {
		return SemanticJudgmentSummary{}, err
	}
	var summary SemanticJudgmentSummary
	if err := readJSONFile(path, &summary); err != nil {
		return SemanticJudgmentSummary{}, fmt.Errorf("read semantic judgment summary: %w", err)
	}
	if err := ValidateSemanticJudgmentSummary(summary); err != nil {
		return SemanticJudgmentSummary{}, err
	}
	return summary, nil
}

func ReadSemanticJudgmentSummary(inputDir string) (SemanticJudgmentSummary, error) {
	root, err := resolveSemanticJudgmentRoot(inputDir)
	if err != nil {
		return SemanticJudgmentSummary{}, err
	}
	return readSemanticJudgmentSummary(root)
}

func readSemanticJudgmentBundle(root string, summary SemanticJudgmentSummary) ([]SemanticJudgmentCandidate, []SemanticJudgmentRecord, error) {
	items := make([]SemanticJudgmentCandidate, 0, len(summary.Candidates))
	judgments := make([]SemanticJudgmentRecord, 0)
	for _, itemSummary := range summary.Candidates {
		itemPath, err := containedSemanticJudgmentPath(root, itemSummary.CandidatePath)
		if err != nil {
			return nil, nil, err
		}
		var item SemanticJudgmentCandidate
		if err := readJSONFile(itemPath, &item); err != nil {
			return nil, nil, fmt.Errorf("read semantic judgment candidate: %w", err)
		}
		if item.SchemaVersion != SemanticJudgmentCandidateSchemaVersion {
			return nil, nil, fmt.Errorf("unsupported semantic judgment candidate schema version: %s", item.SchemaVersion)
		}
		if err := ValidateSemanticJudgmentCandidate(item); err != nil {
			return nil, nil, err
		}
		if item.CandidateID != itemSummary.CandidateID {
			return nil, nil, fmt.Errorf("semantic judgment candidate id mismatch: %s", itemSummary.CandidateID)
		}
		items = append(items, item)
		if itemSummary.JudgmentPath == "" {
			continue
		}
		judgmentPath, err := containedSemanticJudgmentPath(root, itemSummary.JudgmentPath)
		if err != nil {
			return nil, nil, err
		}
		var judgment SemanticJudgmentRecord
		if err := readJSONFile(judgmentPath, &judgment); err != nil {
			return nil, nil, fmt.Errorf("read semantic judgment record: %w", err)
		}
		if err := ValidateSemanticJudgmentRecord(judgment); err != nil {
			return nil, nil, err
		}
		if judgment.CandidateID != itemSummary.CandidateID || judgment.Choice != itemSummary.JudgmentChoice {
			return nil, nil, fmt.Errorf("semantic judgment summary mismatch: %s", itemSummary.CandidateID)
		}
		judgments = append(judgments, judgment)
	}
	return items, orderSemanticJudgmentRecords(judgments), nil
}

func semanticJudgmentsByCandidate(judgments []SemanticJudgmentRecord) map[string]*SemanticJudgmentRecord {
	out := map[string]*SemanticJudgmentRecord{}
	for i := range judgments {
		judgment := judgments[i]
		out[judgment.CandidateID] = &judgment
	}
	return out
}

func validSemanticJudgmentChoice(choice SemanticJudgmentChoice) bool {
	switch choice {
	case SemanticJudgmentChoiceAccept, SemanticJudgmentChoiceReject, SemanticJudgmentChoiceUnclear, SemanticJudgmentChoiceDuplicate, SemanticJudgmentChoiceWrongKind:
		return true
	default:
		return false
	}
}

func orderSemanticJudgmentRecords(judgments []SemanticJudgmentRecord) []SemanticJudgmentRecord {
	out := append([]SemanticJudgmentRecord(nil), judgments...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CandidateID < out[j].CandidateID })
	return out
}

func resolveSemanticJudgmentRoot(path string) (string, error) {
	return resolveNamedArtifactRoot(path, "semantic-judgment")
}

func containedSemanticJudgmentPath(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("unsafe semantic judgment artifact path: %s", relative)
	}
	cleanRelative := filepath.Clean(relative)
	if cleanRelative == "." || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe semantic judgment artifact path: %s", relative)
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
		return "", fmt.Errorf("semantic judgment artifact path escapes root: %s", relative)
	}
	if err := rejectSymlinkAncestors(targetAbs); err != nil {
		return "", err
	}
	if err := rejectIfSymlink(targetAbs); err != nil {
		return "", err
	}
	return targetAbs, nil
}

func ValidateSemanticJudgmentSummary(summary SemanticJudgmentSummary) error {
	if summary.SchemaVersion != SemanticJudgmentSummarySchemaVersion {
		return fmt.Errorf("unsupported semantic judgment summary schema version: %s", summary.SchemaVersion)
	}
	if summary.FailureModeCounts == nil {
		return fmt.Errorf("missing semantic judgment failure mode counts")
	}
	body := summary.RunID + "\n" + summary.QualityStatement
	for _, item := range summary.Candidates {
		body += "\n" + item.CandidateID + "\n" + item.CandidatePath + "\n" + item.PagePath + "\n" + item.JudgmentPath + "\n" + item.SourceDocumentID
	}
	for _, item := range summary.Items {
		body += "\n" + semanticJudgmentCandidateBody(item)
	}
	for _, judgment := range summary.Judgments {
		body += "\n" + semanticJudgmentRecordBody(judgment)
	}
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		return fmt.Errorf("semantic judgment output contains private marker")
	}
	return nil
}

func ValidateSemanticJudgmentCandidate(item SemanticJudgmentCandidate) error {
	if item.SchemaVersion != SemanticJudgmentCandidateSchemaVersion {
		return fmt.Errorf("unsupported semantic judgment candidate schema version: %s", item.SchemaVersion)
	}
	if strings.TrimSpace(item.CandidateID) == "" || sanitizeID(item.CandidateID) != item.CandidateID {
		return fmt.Errorf("unsafe semantic judgment candidate id: %s", item.CandidateID)
	}
	if !validSemanticCandidateKind(item.CandidateKind) {
		return fmt.Errorf("unsupported semantic judgment candidate kind: %s", item.CandidateKind)
	}
	if !validConfidence(item.Confidence) {
		return fmt.Errorf("unsupported semantic judgment confidence: %s", item.Confidence)
	}
	if item.ReviewStatus != ReviewStatusReady && item.ReviewStatus != ReviewStatusNeedsReview && item.ReviewStatus != ReviewStatusBlocked && item.ReviewStatus != ReviewStatusSkipped {
		return fmt.Errorf("unsupported semantic judgment review status: %s", item.ReviewStatus)
	}
	body := semanticJudgmentCandidateBody(item)
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		return fmt.Errorf("semantic judgment candidate contains private marker")
	}
	return nil
}

func ValidateSemanticJudgmentRecord(record SemanticJudgmentRecord) error {
	if record.SchemaVersion != SemanticJudgmentRecordSchemaVersion {
		return fmt.Errorf("unsupported semantic judgment record schema version: %s", record.SchemaVersion)
	}
	if strings.TrimSpace(record.CandidateID) == "" || sanitizeID(record.CandidateID) != record.CandidateID {
		return fmt.Errorf("unsafe semantic judgment record candidate id: %s", record.CandidateID)
	}
	if !validSemanticJudgmentChoice(record.Choice) {
		return fmt.Errorf("unsupported semantic judgment choice: %s", record.Choice)
	}
	if strings.TrimSpace(record.RecordedAt) == "" {
		return fmt.Errorf("missing semantic judgment recorded_at")
	}
	body := semanticJudgmentRecordBody(record)
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		return fmt.Errorf("semantic judgment record contains private marker")
	}
	return nil
}

func semanticJudgmentCandidateBody(item SemanticJudgmentCandidate) string {
	var b strings.Builder
	b.WriteString(item.RunID + "\n" + item.CandidateID + "\n" + item.SourceDocumentID)
	b.WriteString("\n" + string(item.CandidateKind) + "\n" + string(item.ReviewStatus) + "\n" + string(item.Confidence))
	b.WriteString("\n" + item.Title + "\n" + item.Summary + "\n" + strings.Join(item.EvidenceNodes, "\n") + "\n" + strings.Join(item.RelationIDs, "\n"))
	for _, evidenceRange := range item.EvidenceRanges {
		b.WriteString("\n" + evidenceRange.StructureNodeID)
	}
	for _, excerpt := range item.EvidenceExcerpts {
		b.WriteString("\n" + excerpt.SourceLabel + "\n" + excerpt.StructureNodeID + "\n" + excerpt.Text + "\n" + excerpt.UnavailableReason)
	}
	for _, blocker := range item.Blockers {
		b.WriteString("\n" + blocker.Code + "\n" + blocker.Message)
	}
	if item.Judgment != nil {
		b.WriteString("\n" + semanticJudgmentRecordBody(*item.Judgment))
	}
	return b.String()
}

func semanticJudgmentRecordBody(record SemanticJudgmentRecord) string {
	return strings.Join([]string{
		record.RunID,
		record.CandidateID,
		record.SourceDocumentID,
		string(record.CandidateKind),
		string(record.Confidence),
		string(record.Choice),
		record.Note,
		record.ReviewerID,
		record.RecordedAt,
	}, "\n")
}

func semanticJudgmentPageMarkdown(item SemanticJudgmentCandidate, cursor SemanticJudgmentCursor) string {
	var b strings.Builder
	b.WriteString("# Semantic judgment item\n\n")
	b.WriteString(fmt.Sprintf("- Progress: %d/%d judged, %d remaining\n", cursor.JudgedCount, cursor.TotalCount, cursor.RemainingCount))
	b.WriteString("- Candidate: " + item.CandidateID + "\n")
	if item.SourceDocumentID != "" {
		b.WriteString("- Source document: " + item.SourceDocumentID + "\n")
	}
	b.WriteString("- Title: " + fallbackText(item.Title, "no title") + "\n")
	b.WriteString("- Kind: " + string(item.CandidateKind) + "\n")
	b.WriteString("- Confidence: " + string(item.Confidence) + "\n")
	b.WriteString("- Review status: " + string(item.ReviewStatus) + "\n")
	b.WriteString("\n## Candidate summary\n\n")
	b.WriteString(fallbackText(item.Summary, "no summary") + "\n")
	b.WriteString("\n## Evidence ranges\n\n")
	if len(item.EvidenceRanges) == 0 {
		b.WriteString("- Evidence ranges unavailable\n")
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
	}
	b.WriteString("\n## Adjudication choices\n\n")
	b.WriteString("- accept - useful, correct, and evidence-supported\n")
	b.WriteString("- reject - should not have been emitted\n")
	b.WriteString("- unclear - cannot be judged from available context\n")
	b.WriteString("- duplicate - repeats another candidate\n")
	b.WriteString("- wrong-kind - useful content but wrong candidate type or scope\n")
	return b.String()
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func SemanticJudgmentCandidateJSONPath(candidateID string) string {
	return filepath.ToSlash(filepath.Join("candidates", sanitizeID(candidateID)+".json"))
}

func SemanticJudgmentPagePath(candidateID string) string {
	return filepath.ToSlash(filepath.Join("pages", sanitizeID(candidateID)+".md"))
}

func SemanticJudgmentRecordJSONPath(candidateID string) string {
	return filepath.ToSlash(filepath.Join("judgments", sanitizeID(candidateID)+".json"))
}

func semanticJudgmentRoot(outDir string) (string, error) {
	if strings.TrimSpace(outDir) == "" {
		return "", fmt.Errorf("missing required --out")
	}
	return filepath.Abs(filepath.Join(outDir, "semantic-judgment"))
}

func ensureSemanticJudgmentRoot(root string) (string, error) {
	if err := rejectSymlinkAncestors(root); err != nil {
		return "", err
	}
	if err := rejectIfSymlink(root); err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(root)
}
