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
	return SemanticPathWithOptions(inputPath, outDir, SemanticOptions{Classifier: SemanticClassifierDeterministic})
}

func SemanticPathWithOptions(inputPath, outDir string, options SemanticOptions) (SemanticSummary, error) {
	if options.Classifier == "" {
		options.Classifier = SemanticClassifierDeterministic
	}
	var provider LLMSemanticProvider
	if options.Classifier == SemanticClassifierLLM {
		var err error
		provider, err = semanticLLMProvider(options)
		if err != nil {
			return SemanticSummary{}, err
		}
	}
	structureRoot, sourceText, err := prepareSemanticStructure(inputPath, outDir)
	if err != nil {
		return SemanticSummary{}, err
	}
	structureSummary, nodes, err := readStructureArtifacts(structureRoot)
	if err != nil {
		return SemanticSummary{}, err
	}
	segments, err := readSemanticSegments(filepath.Join(filepath.Dir(structureRoot), "document-segments"))
	if err != nil {
		return SemanticSummary{}, err
	}
	nodeIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, node.NodeID)
	}
	runIDInputs := append(nodeIDs, semanticSourceFingerprint(sourceText))
	if options.Classifier == SemanticClassifierLLM {
		runIDInputs = append(runIDInputs, "classifier:llm", "provider:"+options.LLMProvider, "model:"+options.LLMModel)
	}
	runID := SemanticRunID(structureSummary.RunID, runIDInputs)
	observations := ExtractSemanticObservations(runID, nodes, sourceText)
	observations = append(observations, ExtractSegmentSemanticObservations(runID, nodes, segments)...)
	observations = orderSemanticObservations(finalizeSemanticObservations(observations))
	candidates, relations := ConsolidateSemanticCandidates(runID, observations)
	if options.Classifier == SemanticClassifierLLM {
		request := buildLLMSemanticRequest(nodes, sourceText)
		if len(request.Nodes) == 0 {
			skippedReason := "all structure nodes are blocked or empty; no semantic candidates expected"
			if err := WriteSemanticWithSkippedReason(outDir, runID, structureSummary.SourceCount, []SemanticObservation{}, []SemanticCandidate{}, []SemanticRelation{}, skippedReason); err != nil {
				return SemanticSummary{}, err
			}
			return BuildSemanticSummaryWithSkippedReason(runID, structureSummary.SourceCount, []SemanticObservation{}, []SemanticCandidate{}, []SemanticRelation{}, skippedReason), nil
		}
		response, err := provider.Classify(request)
		if err != nil {
			return SemanticSummary{}, err
		}
		observations, candidates, relations, err = buildLLMSemanticObservationsAndArtifacts(runID, nodes, request, response)
		if err != nil {
			return SemanticSummary{}, err
		}
	}
	if options.ReferenceFallback && len(candidates) == 0 && len(nodes) > 0 {
		observations = referenceFallbackObservations(runID, nodes, sourceText)
		candidates, relations = ConsolidateSemanticCandidates(runID, observations)
	}
	if err := WriteSemantic(outDir, runID, structureSummary.SourceCount, observations, candidates, relations); err != nil {
		return SemanticSummary{}, err
	}
	return BuildSemanticSummary(runID, structureSummary.SourceCount, observations, candidates, relations), nil
}

func referenceFallbackObservations(runID string, nodes []StructureNode, sourceText semanticSourceText) []SemanticObservation {
	for _, node := range nodes {
		if node.ReviewStatus == ReviewStatusBlocked {
			continue
		}
		text := semanticNodeText(node, sourceText)
		if strings.TrimSpace(text) == "" {
			continue
		}
		return []SemanticObservation{newSemanticObservation(runID, node, SemanticObservationKindReferenceStatement, text)}
	}
	return nil
}

func semanticLLMProvider(options SemanticOptions) (LLMSemanticProvider, error) {
	if options.LLMClient != nil {
		return options.LLMClient, nil
	}
	if options.LLMProvider != "openai" {
		return nil, fmt.Errorf("unsupported LLM provider: %s", options.LLMProvider)
	}
	if strings.TrimSpace(options.LLMModel) == "" {
		return nil, fmt.Errorf("missing OpenAI model")
	}
	if strings.TrimSpace(options.LLMAPIKey) == "" {
		return nil, fmt.Errorf("missing OpenAI API key")
	}
	return NewOpenAIProvider(options.LLMAPIKey, options.LLMModel, nil), nil
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

func readSemanticSegments(root string) ([]Segment, error) {
	data, err := os.ReadFile(filepath.Join(root, "segment-summary.json"))
	if err != nil {
		return nil, fmt.Errorf("read sibling document-segments: %w", err)
	}
	var summary Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}
	segments := make([]Segment, 0, len(summary.Segments))
	for _, item := range summary.Segments {
		segmentData, err := os.ReadFile(filepath.Join(root, item.SegmentPath))
		if err != nil {
			return nil, err
		}
		var segment Segment
		if err := json.Unmarshal(segmentData, &segment); err != nil {
			return nil, err
		}
		segments = append(segments, segment)
	}
	sort.SliceStable(segments, func(i, j int) bool {
		left, right := segments[i], segments[j]
		return strings.Join([]string{left.SourceDocumentID, fmt.Sprintf("%06d", left.Evidence.LineStart), left.SegmentID}, "\x00") < strings.Join([]string{right.SourceDocumentID, fmt.Sprintf("%06d", right.Evidence.LineStart), right.SegmentID}, "\x00")
	})
	return segments, nil
}

func ExtractSemanticObservations(runID string, nodes []StructureNode, sourceText semanticSourceText) []SemanticObservation {
	var observations []SemanticObservation
	for _, node := range nodes {
		if node.ReviewStatus == ReviewStatusBlocked {
			observations = append(observations, newBlockedSemanticObservation(runID, node))
			continue
		}
		text := semanticNodeText(node, sourceText)
		for _, kind := range semanticObservationKinds(node, text) {
			observations = append(observations, newSemanticObservation(runID, node, kind, text))
		}
	}
	return orderSemanticObservations(finalizeSemanticObservations(observations))
}

func ExtractSegmentSemanticObservations(runID string, nodes []StructureNode, segments []Segment) []SemanticObservation {
	nodeBySegment := semanticNodeBySegment(nodes)
	blockedSources := blockedSemanticSources(nodes, segments)
	var observations []SemanticObservation
	for _, segment := range segments {
		node, ok := nodeBySegment[segment.SegmentID]
		if !ok {
			continue
		}
		if segment.ReviewStatus == ReviewStatusBlocked {
			observations = append(observations, newBlockedSegmentSemanticObservation(runID, node, segment))
			continue
		}
		if blockedSources[segment.SourceDocumentID] {
			continue
		}
		text := semanticSegmentText(segment)
		for _, kind := range semanticSegmentObservationKinds(segment, text) {
			observations = append(observations, newSegmentSemanticObservation(runID, node, segment, kind, text))
		}
	}
	return orderSemanticObservations(finalizeSemanticObservations(observations))
}

func semanticNodeBySegment(nodes []StructureNode) map[string]StructureNode {
	out := map[string]StructureNode{}
	ordered := append([]StructureNode(nil), nodes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		leftSpan := left.Evidence.LineEnd - left.Evidence.LineStart
		rightSpan := right.Evidence.LineEnd - right.Evidence.LineStart
		if leftSpan != rightSpan {
			return leftSpan < rightSpan
		}
		return strings.Join([]string{left.SourceDocumentID, fmt.Sprintf("%06d", left.Evidence.LineStart), left.NodeID}, "\x00") < strings.Join([]string{right.SourceDocumentID, fmt.Sprintf("%06d", right.Evidence.LineStart), right.NodeID}, "\x00")
	})
	for _, node := range ordered {
		if node.ReviewStatus == ReviewStatusBlocked {
			continue
		}
		for _, segmentID := range node.RelatedSegmentIDs {
			if _, exists := out[segmentID]; exists {
				continue
			}
			out[segmentID] = node
		}
	}
	return out
}

func blockedSemanticSources(nodes []StructureNode, segments []Segment) map[string]bool {
	out := map[string]bool{}
	for _, node := range nodes {
		if node.ReviewStatus == ReviewStatusBlocked {
			out[node.SourceDocumentID] = true
		}
	}
	for _, segment := range segments {
		if segment.ReviewStatus == ReviewStatusBlocked {
			out[segment.SourceDocumentID] = true
		}
	}
	return out
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

func semanticSegmentText(segment Segment) string {
	text := strings.TrimSpace(segment.Summary)
	if text == "" {
		text = strings.TrimSpace(segment.Title)
	}
	if text == "" || strings.EqualFold(text, segment.Title) {
		return text
	}
	return strings.TrimSpace(segment.Title + " " + text)
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

func semanticSegmentObservationKinds(segment Segment, text string) []SemanticObservationKind {
	if semanticTextIsMetadata(text) || semanticTextIsLinkOnly(text) {
		return nil
	}
	kinds := semanticTextObservationKinds(text)
	switch segment.SemanticType {
	case SemanticTypeDecision:
		kinds = append(kinds, SemanticObservationKindDecisionSignal)
	case SemanticTypeAction, SemanticTypeCommitment, SemanticTypeWorkItem:
		kinds = append(kinds, SemanticObservationKindActionSignal)
	case SemanticTypeStandard:
		kinds = append(kinds, SemanticObservationKindRequirementStatement)
	case SemanticTypeInsight:
		kinds = append(kinds, SemanticObservationKindClaim)
	case SemanticTypeTension:
		kinds = append(kinds, SemanticObservationKindRiskStatement)
	case SemanticTypeReference:
		if len(kinds) == 0 {
			kinds = append(kinds, SemanticObservationKindReferenceStatement)
		}
	}
	if len(kinds) == 0 && semanticTextIsSubstantive(text) {
		kinds = append(kinds, SemanticObservationKindClaim)
	}
	return dedupeObservationKinds(kinds)
}

func semanticTextObservationKinds(text string) []SemanticObservationKind {
	lower := strings.ToLower(text)
	var kinds []SemanticObservationKind
	if strings.Contains(lower, "?") || strings.Contains(lower, "question:") {
		kinds = append(kinds, SemanticObservationKindQuestion)
	}
	if strings.Contains(lower, "proposal:") || strings.Contains(lower, "propose ") || strings.Contains(lower, "suggest ") {
		kinds = append(kinds, SemanticObservationKindProposal)
	}
	if strings.Contains(lower, "objection:") || strings.Contains(lower, "not ready") {
		kinds = append(kinds, SemanticObservationKindObjection)
	}
	if strings.Contains(lower, "decision:") || strings.Contains(lower, "decided") || strings.Contains(lower, "decide ") {
		kinds = append(kinds, SemanticObservationKindDecisionSignal)
	}
	if strings.Contains(lower, "recap:") || strings.Contains(lower, "summary:") {
		kinds = append(kinds, SemanticObservationKindRecapSignal)
	}
	if strings.Contains(lower, "action:") || strings.Contains(lower, "todo:") || strings.Contains(lower, "follow up") || strings.Contains(lower, "need to") || strings.Contains(lower, "needs to") || strings.Contains(lower, "must ") || strings.Contains(lower, "should ") || strings.Contains(lower, "please ") || strings.Contains(lower, "next step") || strings.Contains(lower, "will ") {
		kinds = append(kinds, SemanticObservationKindActionSignal)
	}
	if strings.Contains(lower, "owner:") || strings.Contains(lower, "assigned to") {
		kinds = append(kinds, SemanticObservationKindOwnerSignal)
	}
	if strings.Contains(lower, "deadline:") || strings.Contains(lower, " by ") || strings.Contains(lower, "friday") || strings.Contains(lower, "monday") || strings.Contains(lower, "tomorrow") {
		kinds = append(kinds, SemanticObservationKindDeadlineSignal)
	}
	if strings.Contains(lower, "requirement:") || strings.Contains(lower, "requires ") || strings.Contains(lower, "required ") {
		kinds = append(kinds, SemanticObservationKindRequirementStatement)
	}
	if strings.Contains(lower, "dependency:") || strings.Contains(lower, "depends on") {
		kinds = append(kinds, SemanticObservationKindDependencyStatement)
	}
	if strings.Contains(lower, "risk:") || strings.Contains(lower, "blocked") || strings.Contains(lower, "blocker") || strings.Contains(lower, "concern") || strings.Contains(lower, "issue:") {
		kinds = append(kinds, SemanticObservationKindRiskStatement)
	}
	if strings.Contains(lower, "insight:") || strings.Contains(lower, "learned ") || strings.Contains(lower, "because ") || strings.Contains(lower, "shows that") || strings.Contains(lower, "means that") {
		kinds = append(kinds, SemanticObservationKindClaim)
	}
	return kinds
}

func semanticTextIsMetadata(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return true
	}
	for _, prefix := range []string{
		"source kind:", "source id:", "source label:", "captured at:", "author:",
		"timestamp:", "permalink:", "thread:", "files:", "urls:", "url:",
		"message id:", "external id:", "from:", "to:", "cc:", "bcc:", "date:", "subject:",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func semanticTextIsLinkOnly(text string) bool {
	trimmed := strings.Trim(strings.TrimSpace(text), "<>()[]")
	lower := strings.ToLower(trimmed)
	if !(strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")) {
		return false
	}
	return !strings.Contains(strings.TrimPrefix(strings.TrimPrefix(lower, "https://"), "http://"), " ")
}

func semanticTextIsSubstantive(text string) bool {
	if len(strings.TrimSpace(text)) < 24 {
		return false
	}
	return len(readableWordPattern.FindAllString(text, -1)) >= 4
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

func newBlockedSemanticObservation(runID string, node StructureNode) SemanticObservation {
	text := strings.TrimSpace(node.Title + " " + node.Summary)
	if text == "" {
		text = "Unsafe or blocked source evidence requires review."
	}
	observation := newSemanticObservation(runID, node, SemanticObservationKindUnknown, text)
	observation.ReviewStatus = ReviewStatusBlocked
	observation.Confidence = ConfidenceLow
	observation.Blockers = appendReviewBlocker(observation.Blockers, "blocked_source_evidence", "Semantic extraction stopped because source evidence was blocked before candidate publication.")
	return observation
}

func newSegmentSemanticObservation(runID string, node StructureNode, segment Segment, kind SemanticObservationKind, text string) SemanticObservation {
	observation := newSemanticObservation(runID, node, kind, text)
	observation.EvidenceRanges = []SemanticEvidenceRange{{
		StructureNodeID: node.NodeID,
		LineStart:       segment.Evidence.LineStart,
		LineEnd:         segment.Evidence.LineEnd,
	}}
	observation.ContentHash = "sha256:" + contentHash(strings.Join([]string{node.NodeID, segment.SegmentID, string(kind), text}, "\n"))
	observation.ObservationID = SemanticObservationID(runID, node.NodeID+"\x00"+segment.SegmentID, kind, observation.Title)
	if segment.ReviewStatus == ReviewStatusNeedsReview && observation.ReviewStatus == ReviewStatusReady {
		observation.ReviewStatus = ReviewStatusNeedsReview
		observation.Confidence = ConfidenceLow
	}
	return ClassifyUnsafeSemanticObservation(observation)
}

func newBlockedSegmentSemanticObservation(runID string, node StructureNode, segment Segment) SemanticObservation {
	text := strings.TrimSpace(segment.Title + " " + segment.Summary)
	if text == "" {
		text = "Unsafe or blocked segment evidence requires review."
	}
	observation := newSegmentSemanticObservation(runID, node, segment, SemanticObservationKindUnknown, text)
	observation.ReviewStatus = ReviewStatusBlocked
	observation.Confidence = ConfidenceLow
	observation.Blockers = appendReviewBlocker(observation.Blockers, "blocked_source_evidence", "Semantic extraction stopped because segment evidence was blocked before candidate publication.")
	return observation
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
	usedObservationIDs := map[string]bool{}
	actionObs := filterObservations(observations, SemanticObservationKindActionSignal, SemanticObservationKindRecapSignal, SemanticObservationKindDecisionSignal, SemanticObservationKindProposal)
	if len(actionObs) >= 2 {
		status := ReviewStatusReady
		confidence := ConfidenceMedium
		out = append(out, newSemanticCandidate(runID, sourceID, SemanticCandidateKindAction, status, confidence, "Prepare the checklist", actionCandidateSummary(actionObs), actionObs))
		markSemanticObservationsUsed(usedObservationIDs, actionObs)
	}
	capabilityObs := filterObservations(observations, SemanticObservationKindCapabilityStatement, SemanticObservationKindRequirementStatement, SemanticObservationKindDependencyStatement)
	if len(capabilityObs) > 0 {
		out = append(out, newSemanticCandidate(runID, sourceID, SemanticCandidateKindCapability, ReviewStatusReady, ConfidenceMedium, capabilityCandidateTitle(capabilityObs), observationSummary(capabilityObs), capabilityObs))
		markSemanticObservationsUsed(usedObservationIDs, capabilityObs)
	}
	for _, observation := range observations {
		if usedObservationIDs[observation.ObservationID] {
			continue
		}
		kind, ok := semanticCandidateKindForObservation(observation.ObservationKind)
		if !ok || kind == SemanticCandidateKindReference {
			continue
		}
		status, confidence := semanticCandidateReviewForObservation(observation)
		title := semanticCandidateTitleForObservation(kind, observation)
		out = append(out, newSemanticCandidate(runID, sourceID, kind, status, confidence, title, observation.Summary, []SemanticObservation{observation}))
	}
	referenceObs := filterObservations(observations, SemanticObservationKindReferenceStatement)
	if len(out) == 0 && len(referenceObs) > 0 {
		out = append(out, newSemanticCandidate(runID, sourceID, SemanticCandidateKindReference, ReviewStatusReady, ConfidenceMedium, "Reference evidence", observationSummary(referenceObs), referenceObs))
	}
	if len(out) == 0 {
		questionObs := filterObservations(observations, SemanticObservationKindQuestion)
		if len(questionObs) > 0 {
			out = append(out, newSemanticCandidate(runID, sourceID, SemanticCandidateKindQuestion, ReviewStatusNeedsReview, ConfidenceLow, "Open question needs review", observationSummary(questionObs), questionObs))
		}
	}
	return out
}

func markSemanticObservationsUsed(used map[string]bool, observations []SemanticObservation) {
	for _, observation := range observations {
		used[observation.ObservationID] = true
	}
}

func semanticCandidateKindForObservation(kind SemanticObservationKind) (SemanticCandidateKind, bool) {
	switch kind {
	case SemanticObservationKindClaim, SemanticObservationKindAgendaFrame, SemanticObservationKindRecapSignal:
		return SemanticCandidateKindTopic, true
	case SemanticObservationKindQuestion:
		return SemanticCandidateKindQuestion, true
	case SemanticObservationKindProposal, SemanticObservationKindActionSignal, SemanticObservationKindOwnerSignal, SemanticObservationKindDeadlineSignal:
		return SemanticCandidateKindAction, true
	case SemanticObservationKindDecisionSignal:
		return SemanticCandidateKindDecision, true
	case SemanticObservationKindCapabilityStatement:
		return SemanticCandidateKindCapability, true
	case SemanticObservationKindRequirementStatement:
		return SemanticCandidateKindRequirement, true
	case SemanticObservationKindDependencyStatement:
		return SemanticCandidateKindDependency, true
	case SemanticObservationKindRiskStatement, SemanticObservationKindObjection:
		return SemanticCandidateKindRisk, true
	case SemanticObservationKindReferenceStatement:
		return SemanticCandidateKindReference, true
	case SemanticObservationKindUnknown:
		return SemanticCandidateKindUnknown, true
	default:
		return "", false
	}
}

func semanticCandidateReviewForObservation(observation SemanticObservation) (ReviewStatus, Confidence) {
	if observation.ReviewStatus == ReviewStatusBlocked {
		return ReviewStatusBlocked, ConfidenceLow
	}
	switch observation.ObservationKind {
	case SemanticObservationKindQuestion, SemanticObservationKindRiskStatement, SemanticObservationKindObjection, SemanticObservationKindUnknown:
		return ReviewStatusNeedsReview, ConfidenceLow
	default:
		return ReviewStatusReady, ConfidenceMedium
	}
}

func semanticCandidateTitleForObservation(kind SemanticCandidateKind, observation SemanticObservation) string {
	title := strings.TrimSpace(observation.Summary)
	if title == "" {
		title = strings.TrimSpace(observation.Title)
	}
	prefix := strings.ReplaceAll(string(kind), "_", " ")
	return trimSemanticText(prefix+": "+title, 96)
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
	if candidate.ReviewStatus == ReviewStatusBlocked {
		candidate.Blockers = mergeSemanticObservationBlockers(candidate.Blockers, observations)
	}
	candidate.CandidateID = SemanticCandidateID(runID, kind, sourceID, title, evidenceNodes)
	return candidate
}

func mergeSemanticObservationBlockers(blockers []Blocker, observations []SemanticObservation) []Blocker {
	out := cloneBlockerList(blockers)
	seen := map[string]bool{}
	for _, blocker := range out {
		seen[blocker.Code+"\x00"+blocker.Message] = true
	}
	for _, observation := range observations {
		for _, blocker := range observation.Blockers {
			key := blocker.Code + "\x00" + blocker.Message
			if seen[key] {
				continue
			}
			out = append(out, blocker)
			seen[key] = true
		}
	}
	if len(out) == 0 {
		out = append(out, Blocker{Code: "blocked_semantic_candidate", Message: "Candidate is blocked because one or more supporting observations are blocked."})
	}
	return out
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
	return BuildSemanticSummaryWithSkippedReason(runID, sourceCount, observations, candidates, relations, "")
}

func BuildSemanticSummaryWithSkippedReason(runID string, sourceCount int, observations []SemanticObservation, candidates []SemanticCandidate, relations []SemanticRelation, skippedReason string) SemanticSummary {
	summary := SemanticSummary{
		SchemaVersion:          SemanticSummarySchemaVersion,
		RunID:                  runID,
		SourceCount:            sourceCount,
		SkippedReason:          strings.TrimSpace(skippedReason),
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

func actionCandidateSummary(observations []SemanticObservation) string {
	ordered := make([]SemanticObservation, 0, len(observations))
	seen := map[string]bool{}
	for _, kind := range []SemanticObservationKind{SemanticObservationKindActionSignal, SemanticObservationKindRecapSignal, SemanticObservationKindDeadlineSignal, SemanticObservationKindOwnerSignal, SemanticObservationKindDecisionSignal, SemanticObservationKindProposal} {
		for _, observation := range observations {
			if observation.ObservationKind == kind && !seen[observation.ObservationID] {
				ordered = append(ordered, observation)
				seen[observation.ObservationID] = true
			}
		}
	}
	for _, observation := range observations {
		if !seen[observation.ObservationID] {
			ordered = append(ordered, observation)
		}
	}
	return observationSummary(ordered)
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
