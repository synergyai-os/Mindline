package documents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func AcceptSemantic(semanticRunDir, answerKeyPath, outDir string) (SemanticAcceptanceSummary, error) {
	summary, candidates, relations, err := readSemanticAcceptanceInput(semanticRunDir)
	if err != nil {
		return SemanticAcceptanceSummary{}, err
	}
	answerKey, err := readSemanticAcceptanceAnswerKey(answerKeyPath)
	if err != nil {
		return SemanticAcceptanceSummary{}, err
	}
	if err := ValidateSemanticAcceptanceAnswerKey(answerKey); err != nil {
		return SemanticAcceptanceSummary{}, err
	}
	acceptance := EvaluateSemanticAcceptance(summary.RunID, answerKey, candidates, relations)
	if err := WriteSemanticAcceptance(outDir, acceptance); err != nil {
		return SemanticAcceptanceSummary{}, err
	}
	return acceptance, nil
}

func readSemanticAcceptanceInput(runDir string) (SemanticSummary, []SemanticCandidate, []SemanticRelation, error) {
	root := runDir
	if !isSemanticRoot(root) {
		root = filepath.Join(runDir, "semantic-candidates")
	}
	summary, candidates, err := readSemanticSummaryAndCandidates(root)
	if err != nil {
		return SemanticSummary{}, nil, nil, err
	}
	relations, err := readSemanticAcceptanceRelations(root, candidates)
	if err != nil {
		return SemanticSummary{}, nil, nil, err
	}
	return summary, orderSemanticCandidates(candidates), relations, nil
}

func readSemanticSummaryAndCandidates(root string) (SemanticSummary, []SemanticCandidate, error) {
	summaryPath, err := containedSemanticAcceptancePath(root, "semantic-summary.json")
	if err != nil {
		return SemanticSummary{}, nil, err
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return SemanticSummary{}, nil, fmt.Errorf("read semantic summary: %w", err)
	}
	var summary SemanticSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return SemanticSummary{}, nil, fmt.Errorf("decode semantic summary: %w", err)
	}
	candidates := make([]SemanticCandidate, 0, len(summary.Candidates))
	for _, item := range summary.Candidates {
		if item.CandidatePath != SemanticCandidateJSONPath(item.CandidateID) {
			return SemanticSummary{}, nil, fmt.Errorf("unexpected semantic candidate path for %s: %s", item.CandidateID, item.CandidatePath)
		}
		candidatePath, err := containedSemanticAcceptancePath(root, item.CandidatePath)
		if err != nil {
			return SemanticSummary{}, nil, err
		}
		candidateData, err := os.ReadFile(candidatePath)
		if err != nil {
			return SemanticSummary{}, nil, fmt.Errorf("read semantic candidate: %w", err)
		}
		var candidate SemanticCandidate
		if err := json.Unmarshal(candidateData, &candidate); err != nil {
			return SemanticSummary{}, nil, fmt.Errorf("decode semantic candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	candidates = finalizeSemanticCandidates(candidates)
	for _, candidate := range candidates {
		if err := ValidateSemanticCandidate(candidate); err != nil {
			return SemanticSummary{}, nil, fmt.Errorf("invalid semantic candidate: %w", err)
		}
	}
	return summary, orderSemanticCandidates(candidates), nil
}

func readSemanticAcceptanceRelations(root string, candidates []SemanticCandidate) ([]SemanticRelation, error) {
	seen := map[string]bool{}
	var relations []SemanticRelation
	for _, candidate := range candidates {
		for _, relationID := range candidate.RelationIDs {
			if seen[relationID] {
				continue
			}
			seen[relationID] = true
			relationPath, err := containedSemanticAcceptancePath(root, SemanticRelationJSONPath(relationID))
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(relationPath)
			if err != nil {
				return nil, fmt.Errorf("read semantic relation: %w", err)
			}
			var relation SemanticRelation
			if err := json.Unmarshal(data, &relation); err != nil {
				return nil, fmt.Errorf("decode semantic relation: %w", err)
			}
			if err := ValidateSemanticRelation(relation); err != nil {
				return nil, fmt.Errorf("invalid semantic relation: %w", err)
			}
			relations = append(relations, relation)
		}
	}
	return orderSemanticRelations(relations), nil
}

func containedSemanticAcceptancePath(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("unsafe semantic artifact path: %s", relative)
	}
	cleanRelative := filepath.Clean(relative)
	if cleanRelative == "." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) || cleanRelative == ".." {
		return "", fmt.Errorf("unsafe semantic artifact path: %s", relative)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, cleanRelative))
	if err != nil {
		return "", err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("semantic artifact path escapes root: %s", relative)
	}
	if err := rejectSymlinkAncestors(targetAbs); err != nil {
		return "", err
	}
	if err := rejectIfSymlink(targetAbs); err != nil {
		return "", err
	}
	return targetAbs, nil
}

func isSemanticRoot(path string) bool {
	info, err := os.Stat(filepath.Join(path, "semantic-summary.json"))
	return err == nil && !info.IsDir()
}

func readSemanticAcceptanceAnswerKey(path string) (SemanticAcceptanceAnswerKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SemanticAcceptanceAnswerKey{}, fmt.Errorf("read answer key: %w", err)
	}
	var answerKey SemanticAcceptanceAnswerKey
	if err := json.Unmarshal(data, &answerKey); err != nil {
		return SemanticAcceptanceAnswerKey{}, fmt.Errorf("decode answer key: %w", err)
	}
	return answerKey, nil
}

func ValidateSemanticAcceptanceAnswerKey(answerKey SemanticAcceptanceAnswerKey) error {
	if answerKey.SchemaVersion != SemanticAcceptanceAnswerKeySchemaVersion {
		return fmt.Errorf("unsupported semantic acceptance answer key schema version: %s", answerKey.SchemaVersion)
	}
	if strings.TrimSpace(answerKey.AnswerKeyID) == "" || sanitizeID(answerKey.AnswerKeyID) != answerKey.AnswerKeyID {
		return fmt.Errorf("unsafe semantic acceptance answer key id: %s", answerKey.AnswerKeyID)
	}
	if containsUnsafeMarker(answerKey.AnswerKeyID) || containsUnsafeMarker(answerKey.SourceDocumentID) || containsGovernanceID(answerKey.AnswerKeyID) || containsGovernanceID(answerKey.SourceDocumentID) {
		return fmt.Errorf("semantic acceptance answer key contains private marker")
	}
	seen := map[string]bool{}
	for _, outcome := range answerKey.ExpectedOutcomes {
		if strings.TrimSpace(outcome.ExpectedOutcomeID) == "" || sanitizeID(outcome.ExpectedOutcomeID) != outcome.ExpectedOutcomeID {
			return fmt.Errorf("unsafe expected outcome id: %s", outcome.ExpectedOutcomeID)
		}
		if seen[outcome.ExpectedOutcomeID] {
			return fmt.Errorf("duplicate expected outcome id: %s", outcome.ExpectedOutcomeID)
		}
		seen[outcome.ExpectedOutcomeID] = true
		if outcome.ExpectedState != ExpectedOutcomePresent && outcome.ExpectedState != ExpectedOutcomeAbsent {
			return fmt.Errorf("unsupported expected outcome state: %s", outcome.ExpectedState)
		}
		if !validSemanticCandidateKind(outcome.ExpectedKind) {
			return fmt.Errorf("unsupported expected outcome kind: %s", outcome.ExpectedKind)
		}
		if !validConfidence(outcome.MinimumConfidenceFloor) {
			return fmt.Errorf("unsupported minimum confidence floor: %s", outcome.MinimumConfidenceFloor)
		}
		if outcome.ExpectedState == ExpectedOutcomePresent && !hasNonBlankString(outcome.RequiredEvidence) {
			return fmt.Errorf("expected-present outcome requires evidence: %s", outcome.ExpectedOutcomeID)
		}
		if containsUnsafeOutcomeMarker(outcome) {
			return fmt.Errorf("expected outcome contains private marker: %s", outcome.ExpectedOutcomeID)
		}
		for _, requirement := range outcome.RelationRequirements {
			if !validSemanticRelationshipType(requirement) {
				return fmt.Errorf("unsupported relation requirement: %s", requirement)
			}
		}
	}
	return nil
}

func hasNonBlankString(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func containsUnsafeOutcomeMarker(outcome SemanticExpectedOutcome) bool {
	parts := []string{outcome.ExpectedOutcomeID, outcome.Notes}
	parts = append(parts, outcome.RequiredEvidence...)
	parts = append(parts, outcome.AcceptableAlternates...)
	parts = append(parts, outcome.TitleSignals...)
	parts = append(parts, outcome.SummarySignals...)
	body := strings.Join(parts, "\n")
	return containsUnsafeMarker(body) || containsGovernanceID(body)
}

func EvaluateSemanticAcceptance(runID string, answerKey SemanticAcceptanceAnswerKey, candidates []SemanticCandidate, relations []SemanticRelation) SemanticAcceptanceSummary {
	candidates = orderSemanticCandidates(candidates)
	candidates = filterSemanticCandidatesBySource(candidates, answerKey.SourceDocumentID)
	relationsByCandidate := relationTypesByCandidate(relations)
	usedCandidates := map[string]string{}
	results := make([]SemanticExpectedOutcomeResult, 0, len(answerKey.ExpectedOutcomes))
	expectedPresent := 0
	expectedAbsent := 0
	matched := 0
	missed := 0
	for _, outcome := range answerKey.ExpectedOutcomes {
		result := SemanticExpectedOutcomeResult{
			SchemaVersion:          SemanticAcceptanceExpectedOutcomeSchemaVersion,
			ExpectedOutcomeID:      outcome.ExpectedOutcomeID,
			ExpectedState:          outcome.ExpectedState,
			ExpectedKind:           outcome.ExpectedKind,
			RequiredEvidence:       cloneStringList(outcome.RequiredEvidence),
			AcceptableAlternates:   cloneStringList(outcome.AcceptableAlternates),
			TitleSignals:           cloneStringList(outcome.TitleSignals),
			SummarySignals:         cloneStringList(outcome.SummarySignals),
			RelationRequirements:   append([]SemanticRelationshipType(nil), outcome.RelationRequirements...),
			MinimumConfidenceFloor: outcome.MinimumConfidenceFloor,
			Notes:                  outcome.Notes,
			ExpectedPath:           SemanticAcceptanceExpectedOutcomeJSONPath(outcome.ExpectedOutcomeID),
		}
		switch outcome.ExpectedState {
		case ExpectedOutcomePresent:
			expectedPresent++
			if candidate, ok := matchExpectedOutcome(answerKey.SourceDocumentID, outcome, candidates, relationsByCandidate, usedCandidates); ok {
				usedCandidates[candidate.CandidateID] = outcome.ExpectedOutcomeID
				result.AcceptanceState = SemanticAcceptanceAccepted
				result.Reason = SemanticAcceptanceReasonCorrect
				result.MatchedCandidateID = candidate.CandidateID
				matched++
			} else {
				result.AcceptanceState = SemanticAcceptanceRejected
				result.Reason = SemanticAcceptanceReasonMissingExpectedOutcome
				missed++
			}
		case ExpectedOutcomeAbsent:
			expectedAbsent++
			if candidate, ok := matchExpectedOutcome(answerKey.SourceDocumentID, outcome, candidates, relationsByCandidate, usedCandidates); ok {
				usedCandidates[candidate.CandidateID] = outcome.ExpectedOutcomeID
				result.AcceptanceState = SemanticAcceptanceRejected
				result.Reason = SemanticAcceptanceReasonUnexpectedCandidate
				result.MatchedCandidateID = candidate.CandidateID
			} else {
				result.AcceptanceState = SemanticAcceptanceAccepted
				result.Reason = SemanticAcceptanceReasonCorrect
			}
		}
		results = append(results, result)
	}
	items := make([]SemanticAcceptanceItem, 0, len(candidates))
	for _, candidate := range candidates {
		state := SemanticAcceptanceRejected
		reason := SemanticAcceptanceReasonUnexpectedCandidate
		expectedID := ""
		if matchedExpected, ok := usedCandidates[candidate.CandidateID]; ok {
			expectedID = matchedExpected
			state = SemanticAcceptanceAccepted
			reason = SemanticAcceptanceReasonCorrect
			for _, result := range results {
				if result.MatchedCandidateID == candidate.CandidateID && result.ExpectedState == ExpectedOutcomeAbsent {
					state = SemanticAcceptanceRejected
					reason = SemanticAcceptanceReasonUnexpectedCandidate
					break
				}
			}
		}
		if isUnsafeBlockedSemanticCandidate(candidate) {
			state = SemanticAcceptanceBlocked
			reason = SemanticAcceptanceReasonUnsafeOrPrivate
		} else if candidate.ReviewStatus == ReviewStatusNeedsReview && state != SemanticAcceptanceRejected {
			state = SemanticAcceptanceNeedsReview
			reason = SemanticAcceptanceReasonAmbiguous
		}
		items = append(items, SemanticAcceptanceItem{
			SchemaVersion:     SemanticAcceptanceItemSchemaVersion,
			CandidateID:       candidate.CandidateID,
			RunID:             candidate.RunID,
			SourceDocumentID:  candidateSourceDocumentID(candidate),
			CandidateKind:     candidate.CandidateKind,
			ReviewStatus:      candidate.ReviewStatus,
			Confidence:        candidate.Confidence,
			Title:             candidate.Title,
			Summary:           candidate.Summary,
			EvidenceNodes:     cloneStringList(candidate.EvidenceNodes),
			EvidenceRanges:    cloneSemanticEvidenceRanges(candidate.EvidenceRanges),
			RelationIDs:       cloneStringList(candidate.RelationIDs),
			AcceptanceState:   state,
			Reason:            reason,
			ExpectedOutcomeID: expectedID,
			Blockers:          cloneBlockerList(candidate.Blockers),
		})
	}
	return BuildSemanticAcceptanceSummary(runID, answerKey, results, items, expectedPresent, expectedAbsent, matched, missed)
}

func filterSemanticCandidatesBySource(candidates []SemanticCandidate, sourceDocumentID string) []SemanticCandidate {
	out := make([]SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidateSourceDocumentID(candidate) != sourceDocumentID {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func matchExpectedOutcome(sourceDocumentID string, outcome SemanticExpectedOutcome, candidates []SemanticCandidate, relationsByCandidate map[string]map[SemanticRelationshipType]bool, used map[string]string) (SemanticCandidate, bool) {
	for _, candidate := range candidates {
		if used[candidate.CandidateID] != "" || candidate.CandidateKind != outcome.ExpectedKind {
			continue
		}
		if isUnsafeBlockedSemanticCandidate(candidate) {
			continue
		}
		if candidateSourceDocumentID(candidate) != sourceDocumentID {
			continue
		}
		if !confidenceAtLeast(candidate.Confidence, outcome.MinimumConfidenceFloor) {
			continue
		}
		if !containsAllSignals(candidate.Title, outcome.TitleSignals) || !containsAllSignals(candidate.Summary, outcome.SummarySignals) {
			continue
		}
		if !containsRequiredEvidenceSet(candidate.EvidenceNodes, outcome.RequiredEvidence, outcome.AcceptableAlternates) {
			continue
		}
		if !hasRequiredEvidenceRanges(candidate, outcome.RequiredEvidence, outcome.AcceptableAlternates) {
			continue
		}
		if !containsRequiredRelations(relationsByCandidate[candidate.CandidateID], outcome.RelationRequirements) {
			continue
		}
		return candidate, true
	}
	return SemanticCandidate{}, false
}

func isUnsafeBlockedSemanticCandidate(candidate SemanticCandidate) bool {
	return candidate.ReviewStatus == ReviewStatusBlocked
}

func candidateSourceDocumentID(candidate SemanticCandidate) string {
	if strings.TrimSpace(candidate.SourceDocumentID) != "" {
		return candidate.SourceDocumentID
	}
	if len(candidate.EvidenceRanges) == 0 {
		return ""
	}
	sourceID := ""
	for _, evidenceRange := range candidate.EvidenceRanges {
		parts := strings.SplitN(evidenceRange.StructureNodeID, "/", 2)
		if len(parts) != 2 {
			return ""
		}
		if sourceID == "" {
			sourceID = parts[0]
			continue
		}
		if sourceID != parts[0] {
			return ""
		}
	}
	return sourceID
}

func relationTypesByCandidate(relations []SemanticRelation) map[string]map[SemanticRelationshipType]bool {
	out := map[string]map[SemanticRelationshipType]bool{}
	for _, relation := range relations {
		if relation.ReviewStatus == ReviewStatusBlocked {
			continue
		}
		for _, endpoint := range []struct {
			id   string
			kind SemanticRelationEndpointType
		}{
			{id: relation.FromID, kind: relation.FromType},
			{id: relation.ToID, kind: relation.ToType},
		} {
			if endpoint.kind != SemanticRelationEndpointCandidate {
				continue
			}
			if out[endpoint.id] == nil {
				out[endpoint.id] = map[SemanticRelationshipType]bool{}
			}
			out[endpoint.id][relation.RelationshipType] = true
		}
	}
	return out
}

func containsAllSignals(body string, signals []string) bool {
	lower := strings.ToLower(body)
	for _, signal := range signals {
		if strings.TrimSpace(signal) == "" {
			continue
		}
		if !strings.Contains(lower, strings.ToLower(signal)) {
			return false
		}
	}
	return true
}

func containsRequiredEvidenceSet(evidenceNodes, required, alternates []string) bool {
	if len(required) == 0 && len(alternates) == 0 {
		return true
	}
	evidence := map[string]bool{}
	for _, node := range evidenceNodes {
		evidence[node] = true
	}
	for _, node := range required {
		if strings.TrimSpace(node) == "" {
			continue
		}
		if !evidence[node] {
			return false
		}
	}
	if len(required) > 0 {
		return true
	}
	for _, node := range alternates {
		if strings.TrimSpace(node) == "" {
			continue
		}
		if evidence[node] {
			return true
		}
	}
	return false
}

func hasRequiredEvidenceRanges(candidate SemanticCandidate, required, alternates []string) bool {
	allowed := map[string]bool{}
	for _, node := range required {
		if strings.TrimSpace(node) != "" {
			allowed[node] = true
		}
	}
	for _, node := range alternates {
		if strings.TrimSpace(node) != "" {
			allowed[node] = true
		}
	}
	if len(allowed) == 0 {
		return true
	}
	covered := map[string]bool{}
	for _, evidenceRange := range candidate.EvidenceRanges {
		if allowed[evidenceRange.StructureNodeID] {
			covered[evidenceRange.StructureNodeID] = true
		}
	}
	for _, node := range required {
		if strings.TrimSpace(node) == "" {
			continue
		}
		if !covered[node] {
			return false
		}
	}
	if len(required) > 0 {
		return true
	}
	for _, node := range alternates {
		if strings.TrimSpace(node) == "" {
			continue
		}
		if covered[node] {
			return true
		}
	}
	return false
}

func containsRequiredRelations(actual map[SemanticRelationshipType]bool, required []SemanticRelationshipType) bool {
	for _, relation := range required {
		if !actual[relation] {
			return false
		}
	}
	return true
}

func confidenceAtLeast(actual, floor Confidence) bool {
	rank := map[Confidence]int{ConfidenceLow: 1, ConfidenceMedium: 2, ConfidenceHigh: 3}
	return rank[actual] >= rank[floor]
}

func BuildSemanticAcceptanceSummary(runID string, answerKey SemanticAcceptanceAnswerKey, expected []SemanticExpectedOutcomeResult, items []SemanticAcceptanceItem, expectedPresent, expectedAbsent, matched, missed int) SemanticAcceptanceSummary {
	itemSummaries := make([]SemanticAcceptanceItemSummary, 0, len(items))
	accepted := 0
	rejected := 0
	needsReview := 0
	blocked := 0
	unexpected := 0
	duplicate := 0
	evidenceMissing := 0
	for _, item := range items {
		switch item.AcceptanceState {
		case SemanticAcceptanceAccepted:
			accepted++
		case SemanticAcceptanceRejected:
			rejected++
		case SemanticAcceptanceNeedsReview, SemanticAcceptanceNeedsMerge, SemanticAcceptanceNeedsSplit:
			needsReview++
		case SemanticAcceptanceBlocked:
			blocked++
		}
		if item.AcceptanceState == SemanticAcceptanceRejected && item.Reason == SemanticAcceptanceReasonUnexpectedCandidate {
			unexpected++
		}
		if item.Reason == SemanticAcceptanceReasonDuplicate {
			duplicate++
		}
		if item.Reason == SemanticAcceptanceReasonMissingEvidence || item.Reason == SemanticAcceptanceReasonUnsupportedEvidence {
			evidenceMissing++
		}
		itemSummaries = append(itemSummaries, SemanticAcceptanceItemSummary{
			CandidateID:     item.CandidateID,
			CandidateKind:   item.CandidateKind,
			AcceptanceState: item.AcceptanceState,
			Reason:          item.Reason,
			ItemPath:        SemanticAcceptanceItemJSONPath(item.CandidateID),
			PreviewPath:     SemanticAcceptancePreviewPath(item.CandidateID),
		})
	}
	sort.SliceStable(expected, func(i, j int) bool {
		return expected[i].ExpectedOutcomeID < expected[j].ExpectedOutcomeID
	})
	sort.SliceStable(itemSummaries, func(i, j int) bool {
		return itemSummaries[i].CandidateID < itemSummaries[j].CandidateID
	})
	return SemanticAcceptanceSummary{
		SchemaVersion:                     SemanticAcceptanceSummarySchemaVersion,
		RunID:                             runID,
		AnswerKeyID:                       answerKey.AnswerKeyID,
		CandidateCount:                    len(items),
		ExpectedPresentCount:              expectedPresent,
		ExpectedAbsentCount:               expectedAbsent,
		MatchedExpectedCount:              matched,
		MissedExpectedCount:               missed,
		UnexpectedCandidateCount:          unexpected,
		AcceptedCount:                     accepted,
		RejectedCount:                     rejected,
		NeedsReviewCount:                  needsReview,
		BlockedCount:                      blocked,
		ReviewBurdenCount:                 len(items) - accepted - blocked + missed,
		PrecisionLikeMatchRate:            ratio(matched, len(items)-blocked),
		RecallLikeExpectedOutcomeCoverage: ratio(matched, expectedPresent),
		FalsePositiveCount:                unexpected,
		FalseNegativeCount:                missed,
		DuplicateCount:                    duplicate,
		EvidenceMissingCount:              evidenceMissing + missed,
		QualityStatement:                  "This is a deterministic fixture evaluation, not calibrated classifier quality yet.",
		ExpectedOutcomes:                  expected,
		Candidates:                        itemSummaries,
		Items:                             items,
	}
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func cloneSemanticEvidenceRanges(values []SemanticEvidenceRange) []SemanticEvidenceRange {
	if len(values) == 0 {
		return []SemanticEvidenceRange{}
	}
	out := make([]SemanticEvidenceRange, len(values))
	copy(out, values)
	return out
}
