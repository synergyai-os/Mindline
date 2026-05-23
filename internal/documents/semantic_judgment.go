package documents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func JudgeSemanticCandidates(semanticRunDir, outDir string, options SemanticJudgmentOptions) (SemanticJudgmentSummary, error) {
	semanticSummary, candidates, relations, observations, err := readSemanticJudgmentInput(semanticRunDir)
	if err != nil {
		return SemanticJudgmentSummary{}, err
	}
	source, err := loadSemanticCalibrationSource(SemanticCalibrationOptions{SourceRoot: options.SourceRoot, SourcePath: options.SourcePath})
	if err != nil {
		return SemanticJudgmentSummary{}, err
	}
	items := semanticJudgmentCandidates(candidates, relations, observations, source, nil)
	summary := BuildSemanticJudgmentSummary(semanticSummary.RunID, semanticSummary.SourceCount, items, nil)
	if err := WriteSemanticJudgment(outDir, summary); err != nil {
		return SemanticJudgmentSummary{}, err
	}
	return summary, nil
}

func readSemanticJudgmentInput(runDir string) (SemanticSummary, []SemanticCandidate, []SemanticRelation, []SemanticObservation, error) {
	root := runDir
	if !isSemanticRoot(root) {
		root = filepath.Join(runDir, "semantic-candidates")
	}
	summary, candidates, err := readSemanticSummaryAndCandidates(root)
	if err != nil {
		return SemanticSummary{}, nil, nil, nil, err
	}
	relations, err := readSemanticJudgmentRelations(root, candidates)
	if err != nil {
		return SemanticSummary{}, nil, nil, nil, err
	}
	observations, err := readSemanticJudgmentObservations(root, candidates)
	if err != nil {
		return SemanticSummary{}, nil, nil, nil, err
	}
	return summary, candidates, relations, observations, nil
}

func readSemanticJudgmentRelations(root string, candidates []SemanticCandidate) ([]SemanticRelation, error) {
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
			if os.IsNotExist(err) {
				continue
			}
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

func readSemanticJudgmentObservations(root string, candidates []SemanticCandidate) ([]SemanticObservation, error) {
	seen := map[string]bool{}
	var observations []SemanticObservation
	for _, candidate := range candidates {
		for _, observationID := range candidate.ObservationIDs {
			if seen[observationID] {
				continue
			}
			seen[observationID] = true
			observationPath, err := containedSemanticAcceptancePath(root, SemanticObservationJSONPath(observationID))
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(observationPath)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("read semantic observation: %w", err)
			}
			var observation SemanticObservation
			if err := json.Unmarshal(data, &observation); err != nil {
				return nil, fmt.Errorf("decode semantic observation: %w", err)
			}
			if err := ValidateSemanticObservation(observation); err != nil {
				return nil, fmt.Errorf("invalid semantic observation: %w", err)
			}
			observations = append(observations, observation)
		}
	}
	return orderSemanticObservations(observations), nil
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
	var updated SemanticJudgmentSummary
	err = withSemanticJudgmentBundleLock(root, func() error {
		summary, err := readSemanticJudgmentSummary(root)
		if err != nil {
			return err
		}
		items, judgments, err := readSemanticJudgmentBundle(root, summary)
		if err != nil {
			return err
		}
		if semanticJudgmentsByCandidate(judgments)[input.CandidateID] != nil {
			return fmt.Errorf("semantic judgment already exists: %s", input.CandidateID)
		}
		var target *SemanticJudgmentCandidate
		for i := range items {
			if items[i].CandidateID == input.CandidateID {
				target = &items[i]
				break
			}
		}
		if target == nil {
			return fmt.Errorf("unknown semantic judgment candidate: %s", input.CandidateID)
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
			return err
		}
		judgments = append(judgments, record)
		judgedByCandidate := semanticJudgmentsByCandidate(judgments)
		for i := range items {
			items[i].Judgment = judgedByCandidate[items[i].CandidateID]
		}
		updated = BuildSemanticJudgmentSummary(summary.RunID, summary.SourceCount, items, judgments)
		if err := writeSemanticJudgmentRoot(root, updated); err != nil {
			return ArtifactWriteError{Err: err}
		}
		return nil
	})
	if err != nil {
		return SemanticJudgmentSummary{}, err
	}
	return updated, nil
}

func withSemanticJudgmentBundleLock(root string, fn func() error) error {
	lockDir := filepath.Join(root, ".semantic-judgment.lock")
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := os.Mkdir(lockDir, 0o700)
		if err == nil {
			defer os.Remove(lockDir)
			return fn()
		}
		if !os.IsExist(err) {
			return err
		}
		info, statErr := os.Lstat(lockDir)
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("semantic judgment lock path is unsafe")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for semantic judgment lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func semanticJudgmentCandidates(candidates []SemanticCandidate, relations []SemanticRelation, observations []SemanticObservation, source semanticCalibrationSourceContext, judgments []SemanticJudgmentRecord) []SemanticJudgmentCandidate {
	judged := semanticJudgmentsByCandidate(judgments)
	relationByID := semanticRelationsByID(relations)
	candidateByID := semanticCandidatesByID(candidates)
	observationByID := semanticObservationsByID(observations)
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
			RelationContext:  semanticJudgmentRelationContext(candidate, relationByID, candidateByID, observationByID),
			Blockers:         cloneBlockerList(candidate.Blockers),
			Judgment:         judged[candidate.CandidateID],
		}
		out = append(out, item)
	}
	return out
}

func semanticJudgmentRelationContext(candidate SemanticCandidate, relationByID map[string]SemanticRelation, candidateByID map[string]SemanticCandidate, observationByID map[string]SemanticObservation) []SemanticJudgmentRelationContext {
	context := make([]SemanticJudgmentRelationContext, 0, len(candidate.RelationIDs))
	for _, relationID := range candidate.RelationIDs {
		relation, ok := relationByID[relationID]
		if !ok {
			continue
		}
		context = append(context, SemanticJudgmentRelationContext{
			RelationID:       relation.RelationID,
			RelationshipType: relation.RelationshipType,
			FromID:           relation.FromID,
			FromType:         relation.FromType,
			ToID:             relation.ToID,
			ToType:           relation.ToType,
			Confidence:       relation.Confidence,
			ReviewStatus:     relation.ReviewStatus,
			EvidenceNodes:    cloneStringList(relation.EvidenceNodes),
			Blockers:         cloneBlockerList(relation.Blockers),
			OtherEndpoint:    semanticJudgmentOtherEndpoint(candidate.CandidateID, relation, candidateByID, observationByID),
			ReviewHint:       semanticJudgmentRelationHint(relation.RelationshipType),
		})
	}
	return context
}

func semanticJudgmentOtherEndpoint(candidateID string, relation SemanticRelation, candidateByID map[string]SemanticCandidate, observationByID map[string]SemanticObservation) SemanticJudgmentEndpointContext {
	endpointID := relation.ToID
	endpointType := relation.ToType
	role := "to"
	if relation.ToID == candidateID {
		endpointID = relation.FromID
		endpointType = relation.FromType
		role = "from"
	}
	context := SemanticJudgmentEndpointContext{EndpointID: endpointID, EndpointType: endpointType, Role: role}
	switch endpointType {
	case SemanticRelationEndpointCandidate:
		if candidate, ok := candidateByID[endpointID]; ok {
			context.Label = semanticJudgmentEndpointLabel(candidate.Title)
			context.Summary = semanticJudgmentEndpointSummary(candidate.Summary)
			return context
		}
	case SemanticRelationEndpointObservation:
		if observation, ok := observationByID[endpointID]; ok {
			context.Label = semanticJudgmentEndpointLabel(observation.Title)
			context.Summary = semanticJudgmentEndpointSummary(observation.Summary)
			return context
		}
	}
	context.Unavailable = true
	context.UnavailableReason = "endpoint context unavailable"
	return context
}

func semanticJudgmentEndpointLabel(value string) string {
	return trimSemanticText(strings.Join(strings.Fields(value), " "), 96)
}

func semanticJudgmentEndpointSummary(value string) string {
	return semanticSummaryText(value)
}

func semanticJudgmentRelationHint(relationship SemanticRelationshipType) string {
	switch relationship {
	case SemanticRelationshipDerivedFrom:
		return "This is the evidence link from the candidate back to the source observation or structure."
	case SemanticRelationshipContradicts:
		return "This candidate conflicts with another semantic object; inspect whether it is stale, resolved, or should be marked unclear/reject."
	case SemanticRelationshipSupersedes:
		return "This candidate may replace an older semantic object; check whether the older item should not be accepted as current."
	case SemanticRelationshipSameTopicAs:
		return "This candidate overlaps another semantic object; use duplicate when it repeats the same useful object."
	case SemanticRelationshipDependsOn:
		return "This candidate depends on another object; check whether it is actionable or incomplete without that dependency."
	case SemanticRelationshipAssignsAction:
		return "This relation suggests ownership or assignment; check whether the action candidate has the right owner/scope."
	case SemanticRelationshipMentionsOwner:
		return "This relation contributes owner context; check whether the candidate uses it correctly."
	case SemanticRelationshipMentionsDeadline:
		return "This relation contributes deadline context; check whether the candidate uses it correctly."
	default:
		return "Use this relation to decide whether the candidate is supported, duplicated, stale, or incomplete."
	}
}

func BuildSemanticJudgmentSummary(runID string, sourceCount int, items []SemanticJudgmentCandidate, judgments []SemanticJudgmentRecord) SemanticJudgmentSummary {
	judged := semanticJudgmentsByCandidate(judgments)
	counts := map[SemanticJudgmentChoice]int{
		SemanticJudgmentChoiceReject:    0,
		SemanticJudgmentChoiceUnclear:   0,
		SemanticJudgmentChoiceDuplicate: 0,
		SemanticJudgmentChoiceWrongKind: 0,
	}
	byKind := map[SemanticCandidateKind]map[SemanticJudgmentChoice]int{}
	byConfidence := map[Confidence]map[SemanticJudgmentChoice]int{}
	byReviewStatus := map[ReviewStatus]map[SemanticJudgmentChoice]int{}
	bySource := map[string]map[SemanticJudgmentChoice]int{}
	byRelationPresence := map[string]map[SemanticJudgmentChoice]int{}
	byRelationType := map[SemanticRelationshipType]map[SemanticJudgmentChoice]int{}
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
			incrementSemanticJudgmentGroup(byKind, item.CandidateKind, choice)
			incrementSemanticJudgmentGroup(byConfidence, item.Confidence, choice)
			incrementSemanticJudgmentGroup(byReviewStatus, item.ReviewStatus, choice)
			sourceKey := item.SourceDocumentID
			if sourceKey == "" {
				sourceKey = "unknown_source"
			}
			incrementSemanticJudgmentGroup(bySource, sourceKey, choice)
			presenceKey := "without_relations"
			if len(item.RelationIDs) > 0 {
				presenceKey = "with_relations"
			}
			incrementSemanticJudgmentGroup(byRelationPresence, presenceKey, choice)
			for _, relationType := range distinctSemanticJudgmentRelationTypes(item.RelationContext) {
				incrementSemanticJudgmentGroup(byRelationType, relationType, choice)
			}
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
		SchemaVersion:              SemanticJudgmentSummarySchemaVersion,
		RunID:                      runID,
		SourceCount:                sourceCount,
		CandidateCount:             len(items),
		JudgedCount:                judgedCount,
		RemainingCount:             remaining,
		AcceptedCount:              accepted,
		RejectedCount:              rejected,
		UnclearCount:               unclear,
		DuplicateCount:             duplicate,
		WrongKindCount:             wrongKind,
		BlockedCount:               blocked,
		SkippedCount:               skipped,
		ReviewBurdenCount:          reviewBurden,
		PrecisionEstimate:          ratio(accepted, judgedCount),
		FailureModeCounts:          counts,
		JudgmentByCandidateKind:    byKind,
		JudgmentByConfidence:       byConfidence,
		JudgmentByReviewStatus:     byReviewStatus,
		JudgmentBySourceDocument:   bySource,
		JudgmentByRelationPresence: byRelationPresence,
		JudgmentByRelationType:     byRelationType,
		QualityStatement:           "Judgments are calibration evidence only; no-human readiness still requires held-out >=98% accuracy.",
		CursorPath:                 "cursor.json",
		ReportPath:                 "reports/judgment-report.md",
		Candidates:                 summaries,
		Items:                      items,
		Judgments:                  orderSemanticJudgmentRecords(judgments),
	}
}

func incrementSemanticJudgmentGroup[K comparable](groups map[K]map[SemanticJudgmentChoice]int, key K, choice SemanticJudgmentChoice) {
	if groups[key] == nil {
		groups[key] = map[SemanticJudgmentChoice]int{}
	}
	groups[key][choice]++
}

func distinctSemanticJudgmentRelationTypes(context []SemanticJudgmentRelationContext) []SemanticRelationshipType {
	seen := map[SemanticRelationshipType]bool{}
	var out []SemanticRelationshipType
	for _, relation := range context {
		if seen[relation.RelationshipType] {
			continue
		}
		seen[relation.RelationshipType] = true
		out = append(out, relation.RelationshipType)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func semanticRelationsByID(relations []SemanticRelation) map[string]SemanticRelation {
	out := map[string]SemanticRelation{}
	for _, relation := range relations {
		out[relation.RelationID] = relation
	}
	return out
}

func semanticCandidatesByID(candidates []SemanticCandidate) map[string]SemanticCandidate {
	out := map[string]SemanticCandidate{}
	for _, candidate := range candidates {
		out[candidate.CandidateID] = candidate
	}
	return out
}

func semanticObservationsByID(observations []SemanticObservation) map[string]SemanticObservation {
	out := map[string]SemanticObservation{}
	for _, observation := range observations {
		out[observation.ObservationID] = observation
	}
	return out
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
	body += "\n" + semanticJudgmentChoiceGroupBody(summary.JudgmentByCandidateKind)
	body += "\n" + semanticJudgmentChoiceGroupBody(summary.JudgmentByConfidence)
	body += "\n" + semanticJudgmentChoiceGroupBody(summary.JudgmentByReviewStatus)
	body += "\n" + semanticJudgmentChoiceGroupBody(summary.JudgmentBySourceDocument)
	body += "\n" + semanticJudgmentChoiceGroupBody(summary.JudgmentByRelationPresence)
	body += "\n" + semanticJudgmentChoiceGroupBody(summary.JudgmentByRelationType)
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

func semanticJudgmentChoiceGroupBody[K ~string](groups map[K]map[SemanticJudgmentChoice]int) string {
	var b strings.Builder
	for key, counts := range groups {
		b.WriteString("\n" + string(key))
		for choice := range counts {
			b.WriteString("\n" + string(choice))
		}
	}
	return b.String()
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
	for _, relation := range item.RelationContext {
		b.WriteString("\n" + relation.RelationID + "\n" + string(relation.RelationshipType))
		b.WriteString("\n" + relation.FromID + "\n" + string(relation.FromType) + "\n" + relation.ToID + "\n" + string(relation.ToType))
		b.WriteString("\n" + string(relation.Confidence) + "\n" + string(relation.ReviewStatus) + "\n" + strings.Join(relation.EvidenceNodes, "\n") + "\n" + relation.ReviewHint)
		b.WriteString("\n" + relation.OtherEndpoint.EndpointID + "\n" + string(relation.OtherEndpoint.EndpointType) + "\n" + relation.OtherEndpoint.Role)
		b.WriteString("\n" + relation.OtherEndpoint.Label + "\n" + relation.OtherEndpoint.Summary + "\n" + relation.OtherEndpoint.UnavailableReason)
		for _, blocker := range relation.Blockers {
			b.WriteString("\n" + blocker.Code + "\n" + blocker.Message)
		}
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
	b.WriteString("\n## Relation context\n\n")
	if len(item.RelationContext) == 0 {
		if len(item.RelationIDs) == 0 {
			b.WriteString("- No relations\n")
		} else {
			b.WriteString("- Relation context unavailable for: " + strings.Join(item.RelationIDs, ", ") + "\n")
		}
	} else {
		for _, relation := range item.RelationContext {
			b.WriteString(fmt.Sprintf("### %s (%s)\n\n", relation.RelationID, relation.RelationshipType))
			b.WriteString(fmt.Sprintf("- From: %s `%s`\n", relation.FromType, relation.FromID))
			b.WriteString(fmt.Sprintf("- To: %s `%s`\n", relation.ToType, relation.ToID))
			b.WriteString(fmt.Sprintf("- Confidence: %s\n", relation.Confidence))
			b.WriteString(fmt.Sprintf("- Review status: %s\n", relation.ReviewStatus))
			if len(relation.EvidenceNodes) > 0 {
				b.WriteString("- Evidence nodes: " + strings.Join(relation.EvidenceNodes, ", ") + "\n")
			}
			b.WriteString("- Review hint: " + relation.ReviewHint + "\n")
			b.WriteString("- Other endpoint role: " + fallbackText(relation.OtherEndpoint.Role, "unknown") + "\n")
			b.WriteString("- Other endpoint: " + semanticJudgmentEndpointMarkdown(relation.OtherEndpoint) + "\n")
			for _, blocker := range relation.Blockers {
				b.WriteString(fmt.Sprintf("- Blocker: %s - %s\n", blocker.Code, blocker.Message))
			}
			b.WriteString("\n")
		}
		if len(item.RelationIDs) > len(item.RelationContext) {
			loaded := map[string]bool{}
			for _, relation := range item.RelationContext {
				loaded[relation.RelationID] = true
			}
			var missing []string
			for _, relationID := range item.RelationIDs {
				if !loaded[relationID] {
					missing = append(missing, relationID)
				}
			}
			if len(missing) > 0 {
				b.WriteString("- Relation context unavailable for: " + strings.Join(missing, ", ") + "\n")
			}
		}
	}
	b.WriteString("\n## Adjudication choices\n\n")
	b.WriteString("- accept - useful, correct, and evidence-supported\n")
	b.WriteString("- reject - should not have been emitted\n")
	b.WriteString("- unclear - cannot be judged from available context\n")
	b.WriteString("- duplicate - repeats another candidate\n")
	b.WriteString("- wrong-kind - useful content but wrong candidate type or scope\n")
	return b.String()
}

func semanticJudgmentEndpointMarkdown(endpoint SemanticJudgmentEndpointContext) string {
	if endpoint.Unavailable {
		reason := endpoint.UnavailableReason
		if reason == "" {
			reason = "endpoint context unavailable"
		}
		return fmt.Sprintf("%s `%s` (%s)", endpoint.EndpointType, endpoint.EndpointID, reason)
	}
	label := fallbackText(endpoint.Label, "no label")
	summary := fallbackText(endpoint.Summary, "no summary")
	return fmt.Sprintf("%s `%s` - %s; %s", endpoint.EndpointType, endpoint.EndpointID, label, summary)
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
