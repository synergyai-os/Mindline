package documents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type semanticSourceText map[string]map[int]string

func SemanticPath(inputPath, outDir string) (SemanticSummary, error) {
	structureRoot, sourceText, err := prepareSemanticStructure(inputPath, outDir)
	if err != nil {
		return SemanticSummary{}, err
	}
	structureSummary, nodes, err := readStructureArtifacts(structureRoot)
	if err != nil {
		return SemanticSummary{}, err
	}
	nodeIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, node.NodeID)
	}
	runID := SemanticRunID(structureSummary.RunID, append(nodeIDs, semanticSourceFingerprint(sourceText)))
	observations := ExtractSemanticObservations(runID, nodes, sourceText)
	candidates, relations := ConsolidateSemanticCandidates(runID, observations)
	if err := WriteSemantic(outDir, runID, structureSummary.SourceCount, observations, candidates, relations); err != nil {
		return SemanticSummary{}, err
	}
	return BuildSemanticSummary(runID, structureSummary.SourceCount, observations, candidates, relations), nil
}

func semanticSourceFingerprint(sourceText semanticSourceText) string {
	if sourceText == nil {
		return "source-text:none"
	}
	var sources []string
	for sourceID := range sourceText {
		sources = append(sources, sourceID)
	}
	sort.Strings(sources)
	var parts []string
	for _, sourceID := range sources {
		var lines []int
		for line := range sourceText[sourceID] {
			lines = append(lines, line)
		}
		sort.Ints(lines)
		for _, line := range lines {
			parts = append(parts, fmt.Sprintf("%s:%d:%s", sourceID, line, sourceText[sourceID][line]))
		}
	}
	return "source-text:" + contentHash(strings.Join(parts, "\n"))
}

func prepareSemanticStructure(inputPath, outDir string) (string, semanticSourceText, error) {
	if strings.TrimSpace(outDir) == "" {
		return "", nil, fmt.Errorf("missing required --out")
	}
	if isStructureRoot(inputPath) {
		sourceText, err := readSegmentSourceText(filepath.Join(filepath.Dir(inputPath), "document-segments"))
		if err != nil {
			return "", nil, err
		}
		return inputPath, sourceText, nil
	}
	if isStructureRoot(filepath.Join(inputPath, "document-structure")) {
		sourceText, err := readSegmentSourceText(filepath.Join(inputPath, "document-segments"))
		if err != nil {
			return "", nil, err
		}
		return filepath.Join(inputPath, "document-structure"), sourceText, nil
	}
	sourceText, err := readSemanticSourceText(inputPath)
	if err != nil {
		return "", nil, err
	}
	if _, err := StructurePath(inputPath, outDir); err != nil {
		return "", nil, err
	}
	return filepath.Join(outDir, "document-structure"), sourceText, nil
}

func readSegmentSourceText(root string) (semanticSourceText, error) {
	data, err := os.ReadFile(filepath.Join(root, "segment-summary.json"))
	if err != nil {
		return nil, fmt.Errorf("read sibling document-segments: %w", err)
	}
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}
	out := semanticSourceText{}
	for _, item := range summary.Segments {
		segmentData, err := os.ReadFile(filepath.Join(root, item.SegmentPath))
		if err != nil {
			return nil, err
		}
		var segment Segment
		if err := json.Unmarshal(segmentData, &segment); err != nil {
			return nil, err
		}
		if out[segment.SourceDocumentID] == nil {
			out[segment.SourceDocumentID] = map[int]string{}
		}
		for line := segment.Evidence.LineStart; line <= segment.Evidence.LineEnd; line++ {
			out[segment.SourceDocumentID][line] = segment.Summary
		}
	}
	return out, nil
}

func isStructureRoot(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(path, "structure-summary.json"))
	return err == nil && !info.IsDir()
}

func readSemanticSourceText(inputPath string) (semanticSourceText, error) {
	paths, err := markdownPaths(inputPath)
	if err != nil {
		return nil, err
	}
	sourceIDs, err := sourceDocumentIDs(inputPath, paths)
	if err != nil {
		return nil, err
	}
	out := semanticSourceText{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		lines := map[int]string{}
		for i, line := range strings.Split(string(data), "\n") {
			lines[i+1] = line
		}
		out[sourceIDs[path]] = lines
	}
	return out, nil
}

func readStructureArtifacts(root string) (StructureSummary, []StructureNode, error) {
	data, err := os.ReadFile(filepath.Join(root, "structure-summary.json"))
	if err != nil {
		return StructureSummary{}, nil, err
	}
	var summary StructureSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return StructureSummary{}, nil, err
	}
	nodes := make([]StructureNode, 0, len(summary.Nodes))
	for _, item := range summary.Nodes {
		nodeData, err := os.ReadFile(filepath.Join(root, StructureNodeJSONPath(item.NodeID)))
		if err != nil {
			return StructureSummary{}, nil, err
		}
		var node StructureNode
		if err := json.Unmarshal(nodeData, &node); err != nil {
			return StructureSummary{}, nil, err
		}
		nodes = append(nodes, node)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		left, right := nodes[i], nodes[j]
		return strings.Join([]string{left.SourceDocumentID, fmt.Sprintf("%06d", left.Evidence.LineStart), left.NodeID}, "\x00") < strings.Join([]string{right.SourceDocumentID, fmt.Sprintf("%06d", right.Evidence.LineStart), right.NodeID}, "\x00")
	})
	return summary, nodes, nil
}

func ExtractSemanticObservations(runID string, nodes []StructureNode, sourceText semanticSourceText) []SemanticObservation {
	var observations []SemanticObservation
	for _, node := range nodes {
		if node.ReviewStatus == ReviewStatusBlocked {
			continue
		}
		text := semanticNodeText(node, sourceText)
		for _, kind := range semanticObservationKinds(node, text) {
			observations = append(observations, newSemanticObservation(runID, node, kind, text))
		}
	}
	return orderSemanticObservations(finalizeSemanticObservations(observations))
}

func semanticNodeText(node StructureNode, sourceText semanticSourceText) string {
	if sourceText != nil {
		var parts []string
		for line := node.Evidence.LineStart; line <= node.Evidence.LineEnd; line++ {
			if text := strings.TrimSpace(sourceText[node.SourceDocumentID][line]); text != "" && !strings.HasPrefix(text, "#") {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	return strings.TrimSpace(node.Title + " " + node.Summary)
}

func semanticObservationKinds(node StructureNode, text string) []SemanticObservationKind {
	lower := strings.ToLower(text)
	kinds := []SemanticObservationKind{}
	switch node.NodeType {
	case StructureNodeTypeCapability:
		kinds = append(kinds, SemanticObservationKindCapabilityStatement)
	case StructureNodeTypeRequirement:
		kinds = append(kinds, SemanticObservationKindRequirementStatement)
	case StructureNodeTypeTranscriptTurn:
		if strings.Contains(lower, "transcript turn by ") {
			return nil
		}
		if strings.Contains(lower, "today we need") || strings.Contains(lower, "agenda") {
			kinds = append(kinds, SemanticObservationKindAgendaFrame)
		}
		if strings.Contains(lower, "?") || strings.Contains(lower, "question:") {
			kinds = append(kinds, SemanticObservationKindQuestion)
		}
		if strings.Contains(lower, "proposal:") {
			kinds = append(kinds, SemanticObservationKindProposal)
		}
		if strings.Contains(lower, "objection:") || strings.Contains(lower, "not ready") || strings.Contains(lower, "blocked") {
			kinds = append(kinds, SemanticObservationKindObjection)
		}
		if strings.Contains(lower, "decision:") || strings.Contains(lower, "decide") {
			kinds = append(kinds, SemanticObservationKindDecisionSignal)
		}
		if strings.Contains(lower, "recap:") {
			kinds = append(kinds, SemanticObservationKindRecapSignal)
		}
		if strings.Contains(lower, "will ") || strings.Contains(lower, "action:") || strings.Contains(lower, "prepare") {
			kinds = append(kinds, SemanticObservationKindActionSignal)
		}
		if strings.Contains(lower, " by ") || strings.Contains(lower, "friday") {
			kinds = append(kinds, SemanticObservationKindDeadlineSignal)
		}
	default:
		if strings.Contains(lower, "requirement:") {
			kinds = append(kinds, SemanticObservationKindRequirementStatement)
		}
		if strings.Contains(lower, "dependency:") || strings.Contains(lower, "depends on") {
			kinds = append(kinds, SemanticObservationKindDependencyStatement)
		}
		if strings.Contains(lower, "risk:") || strings.Contains(lower, "blocks review") {
			kinds = append(kinds, SemanticObservationKindRiskStatement)
		}
	}
	if len(kinds) == 0 && node.NodeType == StructureNodeTypeUnknown {
		kinds = append(kinds, SemanticObservationKindUnknown)
	}
	return dedupeObservationKinds(kinds)
}

func newSemanticObservation(runID string, node StructureNode, kind SemanticObservationKind, text string) SemanticObservation {
	title := semanticTitle(kind, text, node.Title)
	observation := SemanticObservation{
		SchemaVersion:    SemanticObservationSchemaVersion,
		RunID:            runID,
		SourceDocumentID: node.SourceDocumentID,
		ObservationKind:  kind,
		ReviewStatus:     ReviewStatusReady,
		Confidence:       ConfidenceMedium,
		Title:            title,
		Summary:          semanticSummaryText(text),
		EvidenceNodes:    []string{node.NodeID},
		EvidenceRanges: []SemanticEvidenceRange{{
			StructureNodeID: node.NodeID,
			LineStart:       node.Evidence.LineStart,
			LineEnd:         node.Evidence.LineEnd,
		}},
		ContentHash: "sha256:" + contentHash(strings.Join([]string{node.NodeID, string(kind), text}, "\n")),
		Blockers:    []Blocker{},
	}
	if kind == SemanticObservationKindUnknown || kind == SemanticObservationKindObjection {
		observation.ReviewStatus = ReviewStatusNeedsReview
		observation.Confidence = ConfidenceLow
	}
	observation.ObservationID = SemanticObservationID(runID, node.NodeID, kind, title)
	return ClassifyUnsafeSemanticObservation(observation)
}

func ConsolidateSemanticCandidates(runID string, observations []SemanticObservation) ([]SemanticCandidate, []SemanticRelation) {
	bySource := map[string][]SemanticObservation{}
	for _, observation := range observations {
		bySource[observation.SourceDocumentID] = append(bySource[observation.SourceDocumentID], observation)
	}
	var candidates []SemanticCandidate
	var relations []SemanticRelation
	for sourceID, items := range bySource {
		cands := candidatesForSource(runID, sourceID, items)
		for i := range cands {
			candidate := ClassifyUnsafeSemanticCandidate(cands[i])
			for _, observationID := range candidate.ObservationIDs {
				observation, ok := findObservation(items, observationID)
				if !ok {
					continue
				}
				relation := newSemanticRelation(runID, SemanticRelationshipDerivedFrom, candidate.CandidateID, SemanticRelationEndpointCandidate, observation.ObservationID, SemanticRelationEndpointObservation, observation.EvidenceNodes, candidate.ReviewStatus)
				candidate.RelationIDs = append(candidate.RelationIDs, relation.RelationID)
				relations = append(relations, relation)
			}
			if hasObservationKind(items, SemanticObservationKindObjection) {
				if proposal, ok := firstObservation(items, SemanticObservationKindProposal); ok {
					if objection, ok := firstObservation(items, SemanticObservationKindObjection); ok {
						relations = append(relations, newSemanticRelation(runID, SemanticRelationshipContradicts, objection.ObservationID, SemanticRelationEndpointObservation, proposal.ObservationID, SemanticRelationEndpointObservation, mergeUniqueStrings(objection.EvidenceNodes, proposal.EvidenceNodes), ReviewStatusNeedsReview))
						candidate.RelationIDs = append(candidate.RelationIDs, relations[len(relations)-1].RelationID)
					}
				}
			}
			candidates = append(candidates, candidate)
		}
	}
	return orderSemanticCandidates(candidates), orderSemanticRelations(finalizeSemanticRelations(relations))
}

func candidatesForSource(runID, sourceID string, observations []SemanticObservation) []SemanticCandidate {
	if hasObservationKind(observations, SemanticObservationKindObjection) {
		return []SemanticCandidate{newSemanticCandidate(runID, sourceID, SemanticCandidateKindIssue, ReviewStatusNeedsReview, ConfidenceLow, "Import remains under review", observationSummary(observations), observations)}
	}
	var out []SemanticCandidate
	actionObs := filterObservations(observations, SemanticObservationKindActionSignal, SemanticObservationKindRecapSignal, SemanticObservationKindDecisionSignal, SemanticObservationKindProposal)
	if len(actionObs) >= 2 {
		status := ReviewStatusNeedsReview
		confidence := ConfidenceLow
		if hasObservationKind(observations, SemanticObservationKindRecapSignal) && hasObservationKind(observations, SemanticObservationKindActionSignal) && (hasObservationKind(observations, SemanticObservationKindDecisionSignal) || hasObservationKind(observations, SemanticObservationKindProposal)) {
			status = ReviewStatusReady
			confidence = ConfidenceMedium
		}
		out = append(out, newSemanticCandidate(runID, sourceID, SemanticCandidateKindAction, status, confidence, "Prepare the checklist", observationSummary(actionObs), actionObs))
	}
	capabilityObs := filterObservations(observations, SemanticObservationKindCapabilityStatement, SemanticObservationKindRequirementStatement, SemanticObservationKindDependencyStatement, SemanticObservationKindRiskStatement)
	if len(capabilityObs) > 0 {
		out = append(out, newSemanticCandidate(runID, sourceID, SemanticCandidateKindCapability, ReviewStatusReady, ConfidenceMedium, capabilityCandidateTitle(capabilityObs), observationSummary(capabilityObs), capabilityObs))
	}
	if len(out) == 0 {
		questionObs := filterObservations(observations, SemanticObservationKindQuestion)
		if len(questionObs) > 0 {
			out = append(out, newSemanticCandidate(runID, sourceID, SemanticCandidateKindQuestion, ReviewStatusNeedsReview, ConfidenceLow, "Open question needs review", observationSummary(questionObs), questionObs))
		}
	}
	return out
}

func newSemanticCandidate(runID, sourceID string, kind SemanticCandidateKind, status ReviewStatus, confidence Confidence, title, summary string, observations []SemanticObservation) SemanticCandidate {
	evidenceNodes := semanticEvidenceNodes(observations)
	candidate := SemanticCandidate{
		SchemaVersion:     SemanticCandidateSchemaVersion,
		RunID:             runID,
		SourceDocumentID:  sourceID,
		CandidateKind:     kind,
		ReviewStatus:      status,
		Confidence:        confidence,
		Title:             title,
		Summary:           summary,
		EvidenceNodes:     evidenceNodes,
		EvidenceRanges:    semanticEvidenceRanges(observations),
		ObservationIDs:    semanticObservationIDs(observations),
		RelationIDs:       []string{},
		DestinationStatus: SemanticDestinationUnresolved,
		Blockers:          []Blocker{},
	}
	if candidate.ReviewStatus == ReviewStatusNeedsReview {
		candidate.Blockers = append(candidate.Blockers, Blocker{Code: "semantic_review_required", Message: "Candidate requires review because evidence is weak, contradicted, or ambiguous."})
	}
	candidate.CandidateID = SemanticCandidateID(runID, kind, sourceID, title, evidenceNodes)
	return candidate
}

func newSemanticRelation(runID string, relationshipType SemanticRelationshipType, fromID string, fromType SemanticRelationEndpointType, toID string, toType SemanticRelationEndpointType, evidenceNodes []string, status ReviewStatus) SemanticRelation {
	relation := SemanticRelation{
		SchemaVersion:    SemanticRelationSchemaVersion,
		RunID:            runID,
		RelationshipType: relationshipType,
		FromID:           fromID,
		FromType:         fromType,
		ToID:             toID,
		ToType:           toType,
		EvidenceNodes:    cloneStringList(evidenceNodes),
		Confidence:       ConfidenceMedium,
		ReviewStatus:     status,
		Blockers:         []Blocker{},
	}
	if status == ReviewStatusNeedsReview {
		relation.Confidence = ConfidenceLow
	}
	relation.RelationID = SemanticRelationID(runID, relationshipType, fromID, toID)
	return ClassifyUnsafeSemanticRelation(relation)
}

func BuildSemanticSummary(runID string, sourceCount int, observations []SemanticObservation, candidates []SemanticCandidate, relations []SemanticRelation) SemanticSummary {
	summary := SemanticSummary{
		SchemaVersion:          SemanticSummarySchemaVersion,
		RunID:                  runID,
		SourceCount:            sourceCount,
		CandidateKindCounts:    map[SemanticCandidateKind]int{},
		ObservationKindCounts:  map[SemanticObservationKind]int{},
		RelationshipTypeCounts: map[SemanticRelationshipType]int{},
	}
	for _, observation := range observations {
		summary.ObservationCount++
		if observation.ReviewStatus == ReviewStatusNeedsReview {
			summary.NeedsReviewCount++
		}
		if observation.ReviewStatus == ReviewStatusBlocked {
			summary.BlockedCount++
		}
		summary.ObservationKindCounts[observation.ObservationKind]++
	}
	for _, candidate := range candidates {
		summary.CandidateCount++
		if candidate.ReviewStatus == ReviewStatusNeedsReview {
			summary.NeedsReviewCount++
		}
		if candidate.ReviewStatus == ReviewStatusBlocked {
			summary.BlockedCount++
		}
		summary.CandidateKindCounts[candidate.CandidateKind]++
		summary.Candidates = append(summary.Candidates, SemanticSummaryCandidate{
			CandidateID:   candidate.CandidateID,
			CandidateKind: candidate.CandidateKind,
			ReviewStatus:  candidate.ReviewStatus,
			Confidence:    candidate.Confidence,
			CandidatePath: SemanticCandidateJSONPath(candidate.CandidateID),
			PreviewPath:   SemanticPreviewPath(candidate.CandidateID),
		})
	}
	for _, relation := range relations {
		summary.RelationCount++
		if relation.ReviewStatus == ReviewStatusNeedsReview {
			summary.NeedsReviewCount++
		}
		if relation.ReviewStatus == ReviewStatusBlocked {
			summary.BlockedCount++
		}
		summary.RelationshipTypeCounts[relation.RelationshipType]++
	}
	return summary
}

func semanticTitle(kind SemanticObservationKind, text, fallback string) string {
	clean := semanticSummaryText(text)
	if clean == "" {
		clean = fallback
	}
	prefix := strings.ReplaceAll(string(kind), "_", " ")
	return trimSemanticText(prefix+": "+clean, 96)
}

func semanticSummaryText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(stripMarkdownEmphasis(text))), " ")
	text = strings.TrimPrefix(text, "- ")
	return trimSemanticText(text, 160)
}

func trimSemanticText(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max])
}

func observationSummary(observations []SemanticObservation) string {
	parts := make([]string, 0, len(observations))
	for _, observation := range observations {
		if observation.Summary != "" {
			parts = append(parts, observation.Summary)
		}
	}
	return trimSemanticText(strings.Join(parts, " "), 240)
}

func capabilityCandidateTitle(observations []SemanticObservation) string {
	for _, observation := range observations {
		if observation.ObservationKind == SemanticObservationKindCapabilityStatement {
			return trimSemanticText(strings.TrimPrefix(observation.Title, "capability statement: "), 96)
		}
	}
	return "Capability evidence"
}

func semanticEvidenceNodes(observations []SemanticObservation) []string {
	var out []string
	for _, observation := range observations {
		out = mergeUniqueStrings(out, observation.EvidenceNodes)
	}
	return out
}

func semanticEvidenceRanges(observations []SemanticObservation) []SemanticEvidenceRange {
	var out []SemanticEvidenceRange
	seen := map[string]bool{}
	for _, observation := range observations {
		for _, evidenceRange := range observation.EvidenceRanges {
			key := fmt.Sprintf("%s:%d:%d", evidenceRange.StructureNodeID, evidenceRange.LineStart, evidenceRange.LineEnd)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, evidenceRange)
		}
	}
	return out
}

func semanticObservationIDs(observations []SemanticObservation) []string {
	out := make([]string, 0, len(observations))
	for _, observation := range observations {
		out = append(out, observation.ObservationID)
	}
	sort.Strings(out)
	return out
}

func filterObservations(observations []SemanticObservation, kinds ...SemanticObservationKind) []SemanticObservation {
	allowed := map[SemanticObservationKind]bool{}
	for _, kind := range kinds {
		allowed[kind] = true
	}
	var out []SemanticObservation
	for _, observation := range observations {
		if allowed[observation.ObservationKind] {
			out = append(out, observation)
		}
	}
	return orderSemanticObservations(out)
}

func hasObservationKind(observations []SemanticObservation, kind SemanticObservationKind) bool {
	_, ok := firstObservation(observations, kind)
	return ok
}

func firstObservation(observations []SemanticObservation, kind SemanticObservationKind) (SemanticObservation, bool) {
	for _, observation := range observations {
		if observation.ObservationKind == kind {
			return observation, true
		}
	}
	return SemanticObservation{}, false
}

func findObservation(observations []SemanticObservation, id string) (SemanticObservation, bool) {
	for _, observation := range observations {
		if observation.ObservationID == id {
			return observation, true
		}
	}
	return SemanticObservation{}, false
}

func mergeUniqueStrings(left []string, right []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range append(append([]string(nil), left...), right...) {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func dedupeObservationKinds(kinds []SemanticObservationKind) []SemanticObservationKind {
	seen := map[SemanticObservationKind]bool{}
	var out []SemanticObservationKind
	for _, kind := range kinds {
		if seen[kind] {
			continue
		}
		seen[kind] = true
		out = append(out, kind)
	}
	return out
}
