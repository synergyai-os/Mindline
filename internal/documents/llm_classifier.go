package documents

import (
	"fmt"
	"strings"
)

type llmSemanticResponse struct {
	Candidates []llmSemanticCandidate `json:"candidates"`
}

type llmSemanticCandidate struct {
	Kind          string   `json:"kind"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	Confidence    string   `json:"confidence"`
	EvidenceNodes []string `json:"evidence_nodes"`
}

type LLMSemanticRequest struct {
	SourceDocumentID string            `json:"source_document_id"`
	Nodes            []LLMSemanticNode `json:"nodes"`
}

type LLMSemanticNode struct {
	NodeID string `json:"node_id"`
	Text   string `json:"text"`
}

type LLMSemanticProvider interface {
	Classify(request LLMSemanticRequest) (llmSemanticResponse, error)
}

func BuildLLMSemanticPrompt(request LLMSemanticRequest) string {
	var builder strings.Builder
	builder.WriteString("You are classifying Mindline document structure nodes into semantic candidates.\n")
	builder.WriteString("Return JSON only with this shape: {\"candidates\":[{\"kind\":\"action_candidate|decision_candidate|issue_candidate|question_candidate|requirement_candidate|capability_candidate|dependency_candidate|risk_candidate|reference_candidate|topic_candidate\",\"title\":\"...\",\"summary\":\"...\",\"confidence\":\"low|medium|high\",\"evidence_nodes\":[\"node-id\"]}]}.\n")
	builder.WriteString("Return the smallest useful set of candidates: usually 5-15, at most 20. Keep each title under 14 words and each summary under 35 words.\n")
	builder.WriteString("Use only the supplied evidence node IDs. Do not invent evidence. Do not include destination names, Product Brain fields, Tolaria paths, or provider metadata.\n")
	builder.WriteString("Source document: ")
	builder.WriteString(request.SourceDocumentID)
	builder.WriteString("\nNodes:\n")
	for _, node := range request.Nodes {
		builder.WriteString("- ")
		builder.WriteString(node.NodeID)
		builder.WriteString(": ")
		builder.WriteString(strings.Join(strings.Fields(node.Text), " "))
		builder.WriteString("\n")
	}
	return builder.String()
}

func buildLLMSemanticRequest(nodes []StructureNode, sourceText semanticSourceText) LLMSemanticRequest {
	request := LLMSemanticRequest{}
	for _, node := range nodes {
		if request.SourceDocumentID == "" {
			request.SourceDocumentID = node.SourceDocumentID
		}
		if node.ReviewStatus == ReviewStatusBlocked {
			continue
		}
		text := semanticNodeText(node, sourceText)
		if strings.TrimSpace(text) == "" {
			continue
		}
		request.Nodes = append(request.Nodes, LLMSemanticNode{
			NodeID: node.NodeID,
			Text:   text,
		})
	}
	return request
}

func buildLLMSemanticArtifacts(runID string, nodes []StructureNode, response llmSemanticResponse) ([]SemanticCandidate, []SemanticRelation, error) {
	request := buildLLMSemanticRequest(nodes, nil)
	observations, candidates, relations, err := buildLLMSemanticObservationsAndArtifacts(runID, nodes, request, response)
	if err != nil {
		return nil, nil, err
	}
	_ = observations
	return candidates, relations, nil
}

func buildLLMSemanticObservationsAndArtifacts(runID string, nodes []StructureNode, request LLMSemanticRequest, response llmSemanticResponse) ([]SemanticObservation, []SemanticCandidate, []SemanticRelation, error) {
	nodesByID := map[string]StructureNode{}
	for _, node := range nodes {
		nodesByID[node.NodeID] = node
	}
	textByNodeID := map[string]string{}
	for _, node := range request.Nodes {
		textByNodeID[node.NodeID] = node.Text
	}
	var observations []SemanticObservation
	var candidates []SemanticCandidate
	var relations []SemanticRelation
	for index, item := range response.Candidates {
		kind := SemanticCandidateKind(strings.TrimSpace(item.Kind))
		if !validSemanticCandidateKind(kind) {
			return nil, nil, nil, fmt.Errorf("unsupported LLM candidate kind: %s", item.Kind)
		}
		confidence := Confidence(strings.TrimSpace(item.Confidence))
		if confidence == "" {
			confidence = ConfidenceMedium
		}
		if confidence != ConfidenceLow && confidence != ConfidenceMedium && confidence != ConfidenceHigh {
			return nil, nil, nil, fmt.Errorf("unsupported LLM confidence: %s", item.Confidence)
		}
		reviewStatus := ReviewStatusReady
		if confidence == ConfidenceLow {
			reviewStatus = ReviewStatusNeedsReview
		}
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Summary) == "" {
			return nil, nil, nil, fmt.Errorf("LLM candidate missing title or summary")
		}
		if len(item.EvidenceNodes) == 0 {
			return nil, nil, nil, fmt.Errorf("LLM candidate missing evidence nodes")
		}
		var candidateObservations []SemanticObservation
		seenEvidenceNodes := map[string]bool{}
		discriminator := llmCandidateDiscriminator(index)
		for _, nodeID := range item.EvidenceNodes {
			node, ok := resolveLLMEvidenceNode(nodesByID, nodeID)
			if !ok {
				return nil, nil, nil, fmt.Errorf("unknown LLM evidence node: %s", nodeID)
			}
			if seenEvidenceNodes[node.NodeID] {
				continue
			}
			seenEvidenceNodes[node.NodeID] = true
			observation := SemanticObservation{
				SchemaVersion:    SemanticObservationSchemaVersion,
				ObservationID:    SemanticObservationID(runID, node.NodeID, observationKindForCandidateKind(kind), item.Title+"\x00"+discriminator),
				RunID:            runID,
				SourceDocumentID: node.SourceDocumentID,
				ObservationKind:  observationKindForCandidateKind(kind),
				ReviewStatus:     reviewStatus,
				Confidence:       confidence,
				Title:            semanticSummaryText(item.Title),
				Summary:          semanticSummaryText(item.Summary),
				EvidenceNodes:    []string{node.NodeID},
				EvidenceRanges: []SemanticEvidenceRange{{
					StructureNodeID: node.NodeID,
					LineStart:       node.Evidence.LineStart,
					LineEnd:         node.Evidence.LineEnd,
				}},
				ContentHash: "sha256:" + contentHash(strings.Join([]string{runID, node.NodeID, item.Kind, item.Title, item.Summary, fmt.Sprintf("%d", index)}, "\n")),
				Blockers:    []Blocker{},
			}
			observation = ClassifyUnsafeSemanticObservation(observation)
			observations = append(observations, observation)
			candidateObservations = append(candidateObservations, observation)
		}
		if len(candidateObservations) == 0 {
			continue
		}
		candidate := newSemanticCandidate(runID, candidateObservations[0].SourceDocumentID, kind, reviewStatus, confidence, item.Title, item.Summary, candidateObservations)
		candidate.CandidateID = SemanticCandidateID(runID, kind, candidate.SourceDocumentID, item.Title+"\x00"+discriminator, candidate.EvidenceNodes)
		candidate.EvidenceExcerpts = llmEvidenceExcerpts(candidate.EvidenceNodes, textByNodeID)
		for _, observation := range candidateObservations {
			relation := newSemanticRelation(runID, SemanticRelationshipDerivedFrom, candidate.CandidateID, SemanticRelationEndpointCandidate, observation.ObservationID, SemanticRelationEndpointObservation, observation.EvidenceNodes, candidate.ReviewStatus)
			candidate.RelationIDs = append(candidate.RelationIDs, relation.RelationID)
			relations = append(relations, relation)
		}
		candidates = append(candidates, ClassifyUnsafeSemanticCandidate(candidate))
	}
	return orderSemanticObservations(observations), orderSemanticCandidates(candidates), orderSemanticRelations(relations), nil
}

func llmCandidateDiscriminator(index int) string {
	return fmt.Sprintf("llm-candidate-index:%d", index)
}

func llmEvidenceExcerpts(evidenceNodes []string, textByNodeID map[string]string) []SemanticEvidenceExcerpt {
	excerpts := []SemanticEvidenceExcerpt{}
	for _, nodeID := range evidenceNodes {
		text := semanticSummaryText(textByNodeID[nodeID])
		if text == "" {
			continue
		}
		excerpts = append(excerpts, SemanticEvidenceExcerpt{
			StructureNodeID: nodeID,
			Text:            text,
		})
	}
	return excerpts
}

func resolveLLMEvidenceNode(nodesByID map[string]StructureNode, rawNodeID string) (StructureNode, bool) {
	nodeID := strings.TrimSpace(rawNodeID)
	if node, ok := nodesByID[nodeID]; ok {
		return node, true
	}
	if nodeID == "" || strings.HasPrefix(nodeID, "node-") {
		return StructureNode{}, false
	}
	matchID := "node-" + nodeID
	node, ok := nodesByID[matchID]
	return node, ok
}

func observationKindForCandidateKind(kind SemanticCandidateKind) SemanticObservationKind {
	switch kind {
	case SemanticCandidateKindDecision:
		return SemanticObservationKindDecisionSignal
	case SemanticCandidateKindAction:
		return SemanticObservationKindActionSignal
	case SemanticCandidateKindIssue:
		return SemanticObservationKindObjection
	case SemanticCandidateKindQuestion:
		return SemanticObservationKindQuestion
	case SemanticCandidateKindRequirement:
		return SemanticObservationKindRequirementStatement
	case SemanticCandidateKindCapability:
		return SemanticObservationKindCapabilityStatement
	case SemanticCandidateKindDependency:
		return SemanticObservationKindDependencyStatement
	case SemanticCandidateKindRisk:
		return SemanticObservationKindRiskStatement
	case SemanticCandidateKindReference:
		return SemanticObservationKindReferenceStatement
	default:
		return SemanticObservationKindClaim
	}
}
