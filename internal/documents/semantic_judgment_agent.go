package documents

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type LLMSemanticReviewer interface {
	ReviewSemanticJudgment(request LLMSemanticReviewRequest) (llmSemanticReviewResponse, error)
}

type LLMSemanticReviewRequest struct {
	Candidate SemanticJudgmentCandidate `json:"candidate"`
}

type llmSemanticReviewResponse struct {
	Choice              string   `json:"choice"`
	FailureReason       string   `json:"failure_reason,omitempty"`
	Confidence          string   `json:"confidence"`
	HumanReviewRequired bool     `json:"human_review_required"`
	ReviewReasonCodes   []string `json:"review_reason_codes"`
	Rationale           string   `json:"rationale"`
}

func BuildLLMSemanticReviewPrompt(request LLMSemanticReviewRequest) string {
	item := request.Candidate
	var b strings.Builder
	b.WriteString("You are reviewing one Mindline semantic candidate for extraction correctness.\n")
	b.WriteString("Return JSON only with this shape: {\"choice\":\"accept|reject|unclear|duplicate|wrong-kind\",\"failure_reason\":\"...\",\"confidence\":\"low|medium|high\",\"human_review_required\":true|false,\"review_reason_codes\":[\"...\"],\"rationale\":\"...\"}.\n")
	b.WriteString("This is not a destination approval. Judge only whether the extraction should count as correct. Use human_review_required=false only when evidence is clear, the choice is high confidence, and no safety/risk issue exists.\n")
	b.WriteString("Use a failure_reason compatible with the choice. Accept uses no failure_reason. Keep rationale under 30 words and do not copy long source excerpts.\n")
	b.WriteString("Candidate:\n")
	b.WriteString("- id: " + item.CandidateID + "\n")
	b.WriteString("- kind: " + string(item.CandidateKind) + "\n")
	b.WriteString("- confidence: " + string(item.Confidence) + "\n")
	b.WriteString("- review_status: " + string(item.ReviewStatus) + "\n")
	b.WriteString("- title: " + strings.Join(strings.Fields(item.Title), " ") + "\n")
	b.WriteString("- summary: " + strings.Join(strings.Fields(item.Summary), " ") + "\n")
	b.WriteString("- evidence_readiness: " + string(item.EvidenceReadiness.Status) + "\n")
	if len(item.EvidenceReadiness.ReasonCodes) > 0 {
		b.WriteString("- readiness_reasons: " + semanticEvidenceReadinessReasonText(item.EvidenceReadiness.ReasonCodes) + "\n")
	}
	if len(item.Blockers) > 0 {
		b.WriteString("Blockers:\n")
		for _, blocker := range item.Blockers {
			b.WriteString("- " + blocker.Code + ": " + blocker.Message + "\n")
		}
	}
	b.WriteString("Evidence excerpts:\n")
	excerptCount := 0
	for _, excerpt := range item.EvidenceExcerpts {
		if excerpt.Unavailable || strings.TrimSpace(excerpt.Text) == "" {
			continue
		}
		excerptCount++
		b.WriteString("- " + excerpt.SourceLabel + " lines ")
		b.WriteString(fmt.Sprintf("%d-%d: ", excerpt.LineStart, excerpt.LineEnd))
		b.WriteString(strings.Join(strings.Fields(trimSemanticText(excerpt.Text, 900)), " "))
		b.WriteString("\n")
		if excerptCount >= 5 {
			break
		}
	}
	if excerptCount == 0 {
		b.WriteString("- none available\n")
	}
	b.WriteString("Relation context:\n")
	if len(item.RelationContext) == 0 {
		b.WriteString("- none loaded\n")
	} else {
		for _, relation := range item.RelationContext {
			b.WriteString("- " + string(relation.RelationshipType) + " " + relation.RelationID)
			b.WriteString("; other: " + relation.OtherEndpoint.Label)
			if relation.OtherEndpoint.Summary != "" {
				b.WriteString("; " + relation.OtherEndpoint.Summary)
			}
			b.WriteString("; hint: " + relation.ReviewHint + "\n")
		}
	}
	return b.String()
}

func semanticJudgmentReviewer(options SemanticJudgmentOptions) (LLMSemanticReviewer, error) {
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

func attachSemanticAgentReviews(items []SemanticJudgmentCandidate, options SemanticJudgmentOptions) ([]SemanticJudgmentCandidate, error) {
	if options.Reviewer == SemanticJudgmentReviewerNone {
		return items, nil
	}
	if options.Reviewer != SemanticJudgmentReviewerLLM {
		return nil, fmt.Errorf("unsupported semantic judgment reviewer: %s", options.Reviewer)
	}
	var reviewer LLMSemanticReviewer
	out := append([]SemanticJudgmentCandidate(nil), items...)
	for i := range out {
		proposal, ok, err := localSemanticAgentReviewProposal(out[i], options)
		if err != nil {
			return nil, err
		}
		if ok {
			out[i].AgentReview = &proposal
			continue
		}
		if reviewer == nil {
			reviewer, err = semanticJudgmentReviewer(options)
			if err != nil {
				return nil, err
			}
		}
		response, err := reviewer.ReviewSemanticJudgment(LLMSemanticReviewRequest{Candidate: out[i]})
		if err != nil {
			proposal = semanticAgentReviewErrorProposal(SemanticAgentReviewReasonModelError, "model review failed safely; human review required", options)
			out[i].AgentReview = &proposal
			continue
		}
		proposal, err = semanticAgentReviewProposalFromResponse(response, options)
		if err != nil {
			proposal = semanticAgentReviewErrorProposal(SemanticAgentReviewReasonModelError, "model output could not be safely normalized; human review required", options)
			out[i].AgentReview = &proposal
			continue
		}
		proposal = forceSemanticAgentReviewRisk(out[i], proposal)
		out[i].AgentReview = &proposal
	}
	return out, nil
}

func localSemanticAgentReviewProposal(item SemanticJudgmentCandidate, options SemanticJudgmentOptions) (SemanticAgentReviewProposal, bool, error) {
	if semanticEvidenceReadinessUnsafe(item) {
		return semanticAgentReviewErrorProposal(SemanticAgentReviewReasonUnsafeOrPrivate, "candidate blocked before external review because it contains unsafe, private, or governance markers", options), true, nil
	}
	return SemanticAgentReviewProposal{}, false, nil
}

func semanticAgentReviewErrorProposal(reason SemanticAgentReviewReasonCode, message string, options SemanticJudgmentOptions) SemanticAgentReviewProposal {
	return SemanticAgentReviewProposal{
		SchemaVersion:       SemanticAgentReviewProposalSchemaVersion,
		Provider:            strings.TrimSpace(options.LLMProvider),
		Model:               strings.TrimSpace(options.LLMModel),
		Choice:              SemanticJudgmentChoiceUnclear,
		FailureReason:       SemanticFailureAmbiguous,
		Confidence:          ConfidenceLow,
		HumanReviewRequired: true,
		ReviewReasonCodes:   []SemanticAgentReviewReasonCode{reason},
		Rationale:           trimSemanticText(strings.Join(strings.Fields(message), " "), 180),
		Error:               reason.String(),
	}
}

func semanticAgentReviewProposalFromResponse(response llmSemanticReviewResponse, options SemanticJudgmentOptions) (SemanticAgentReviewProposal, error) {
	proposal := SemanticAgentReviewProposal{
		SchemaVersion:       SemanticAgentReviewProposalSchemaVersion,
		Provider:            strings.TrimSpace(options.LLMProvider),
		Model:               strings.TrimSpace(options.LLMModel),
		Choice:              SemanticJudgmentChoice(strings.TrimSpace(response.Choice)),
		FailureReason:       SemanticFailureReason(strings.TrimSpace(response.FailureReason)),
		Confidence:          Confidence(strings.TrimSpace(response.Confidence)),
		HumanReviewRequired: response.HumanReviewRequired,
		Rationale:           trimSemanticText(strings.Join(strings.Fields(response.Rationale), " "), 240),
	}
	for _, reason := range response.ReviewReasonCodes {
		proposal.ReviewReasonCodes = append(proposal.ReviewReasonCodes, SemanticAgentReviewReasonCode(strings.TrimSpace(reason)))
	}
	if proposal.Confidence == "" {
		proposal.Confidence = ConfidenceLow
		proposal.HumanReviewRequired = true
	}
	if err := ValidateSemanticAgentReviewProposal(proposal); err != nil {
		return SemanticAgentReviewProposal{}, err
	}
	return proposal, nil
}

func forceSemanticAgentReviewRisk(item SemanticJudgmentCandidate, proposal SemanticAgentReviewProposal) SemanticAgentReviewProposal {
	reasons := map[SemanticAgentReviewReasonCode]bool{}
	for _, reason := range proposal.ReviewReasonCodes {
		reasons[reason] = true
	}
	if proposal.Confidence != ConfidenceHigh {
		proposal.HumanReviewRequired = true
		reasons[SemanticAgentReviewReasonLowConfidence] = true
	}
	if item.EvidenceReadiness.Status != SemanticEvidenceReadinessPass {
		proposal.HumanReviewRequired = true
		reasons[SemanticAgentReviewReasonEvidenceNotReady] = true
		for _, readinessReason := range item.EvidenceReadiness.ReasonCodes {
			if readinessReason == SemanticEvidenceReadinessPrivateOrGovernanceMarker {
				reasons[SemanticAgentReviewReasonUnsafeOrPrivate] = true
			}
			if readinessReason == SemanticEvidenceReadinessInvalidRelationContext || readinessReason == SemanticEvidenceReadinessMissingRelationContext {
				reasons[SemanticAgentReviewReasonInvalidRelationContext] = true
			}
		}
	}
	if len(item.Blockers) > 0 {
		proposal.HumanReviewRequired = true
		reasons[SemanticAgentReviewReasonBlockersPresent] = true
	}
	if proposal.Choice == SemanticJudgmentChoiceUnclear {
		proposal.HumanReviewRequired = true
		reasons[SemanticAgentReviewReasonModelUncertain] = true
	}
	if !proposal.HumanReviewRequired {
		reasons[SemanticAgentReviewReasonMachineTriaged] = true
	} else {
		delete(reasons, SemanticAgentReviewReasonMachineTriaged)
		if len(reasons) == 0 {
			reasons[SemanticAgentReviewReasonModelUncertain] = true
		}
	}
	proposal.ReviewReasonCodes = orderSemanticAgentReviewReasonCodes(reasons)
	return proposal
}

func ValidateSemanticAgentReviewProposal(proposal SemanticAgentReviewProposal) error {
	if proposal.SchemaVersion != SemanticAgentReviewProposalSchemaVersion {
		return fmt.Errorf("unsupported semantic agent review proposal schema version: %s", proposal.SchemaVersion)
	}
	if strings.TrimSpace(proposal.Provider) == "" {
		return fmt.Errorf("missing semantic agent review provider")
	}
	if strings.TrimSpace(proposal.Model) == "" {
		return fmt.Errorf("missing semantic agent review model")
	}
	if !validSemanticJudgmentChoice(proposal.Choice) {
		return fmt.Errorf("unsupported semantic agent review choice: %s", proposal.Choice)
	}
	if proposal.Confidence != ConfidenceLow && proposal.Confidence != ConfidenceMedium && proposal.Confidence != ConfidenceHigh {
		return fmt.Errorf("unsupported semantic agent review confidence: %s", proposal.Confidence)
	}
	record := SemanticJudgmentRecord{SchemaVersion: SemanticJudgmentRecordSchemaVersion, CandidateID: "candidate", Choice: proposal.Choice, FailureReason: proposal.FailureReason, RecordedAt: "1970-01-01T00:00:00Z"}
	if err := validateSemanticJudgmentRecordFailureReason(record); err != nil {
		return err
	}
	if strings.TrimSpace(proposal.Rationale) == "" {
		return fmt.Errorf("missing semantic agent review rationale")
	}
	if proposal.HumanReviewRequired && len(proposal.ReviewReasonCodes) == 0 {
		return fmt.Errorf("semantic agent review requiring human review needs reason codes")
	}
	for _, reason := range proposal.ReviewReasonCodes {
		if !validSemanticAgentReviewReasonCode(reason) {
			return fmt.Errorf("unsupported semantic agent review reason code: %s", reason)
		}
	}
	body, _ := json.Marshal(proposal)
	if containsUnsafeMarker(string(body)) || containsGovernanceID(string(body)) {
		return fmt.Errorf("semantic agent review proposal contains private marker")
	}
	return nil
}

func validSemanticAgentReviewReasonCode(reason SemanticAgentReviewReasonCode) bool {
	for _, valid := range semanticAgentReviewReasonCodes() {
		if reason == valid {
			return true
		}
	}
	return false
}

func semanticAgentReviewReasonCodes() []SemanticAgentReviewReasonCode {
	return []SemanticAgentReviewReasonCode{
		SemanticAgentReviewReasonLowConfidence,
		SemanticAgentReviewReasonEvidenceNotReady,
		SemanticAgentReviewReasonBlockersPresent,
		SemanticAgentReviewReasonUnsafeOrPrivate,
		SemanticAgentReviewReasonInvalidRelationContext,
		SemanticAgentReviewReasonModelUncertain,
		SemanticAgentReviewReasonModelError,
		SemanticAgentReviewReasonMachineTriaged,
	}
}

func orderSemanticAgentReviewReasonCodes(reasons map[SemanticAgentReviewReasonCode]bool) []SemanticAgentReviewReasonCode {
	out := make([]SemanticAgentReviewReasonCode, 0, len(reasons))
	for reason := range reasons {
		out = append(out, reason)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (reason SemanticAgentReviewReasonCode) String() string {
	return string(reason)
}
