package processing

import (
	"errors"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
)

func RecordOperatorReview(proposal Proposal, input OperatorReviewInput) (OperatorReviewRecord, error) {
	if err := ValidateProposal(proposal); err != nil {
		return OperatorReviewRecord{}, err
	}
	if strings.TrimSpace(input.ReviewerID) == "" || strings.TrimSpace(input.Rationale) == "" {
		return OperatorReviewRecord{}, errors.New("operator review identity and rationale are required")
	}
	if proposal.RequiresManualReview {
		if input.ManualSupportOutcome != "queued_for_manual_processing" && input.ManualSupportOutcome != "confirmed_unavailable" {
			return OperatorReviewRecord{}, errors.New("manual-support outcome is required")
		}
	} else if input.ManualSupportOutcome != "not_required" {
		return OperatorReviewRecord{}, errors.New("manual-support outcome must be not_required")
	}
	if _, err := time.Parse(time.RFC3339, input.ReviewedAt); err != nil {
		return OperatorReviewRecord{}, errors.New("invalid operator review timestamp")
	}
	judgment := Judgment{}
	switch input.Decision {
	case ReviewAccept:
		judgment = cloneJudgment(proposal.Judgment)
	case ReviewRevise:
		judgment = cloneJudgment(input.Judgment)
	case ReviewReject:
	default:
		return OperatorReviewRecord{}, errors.New("invalid operator review decision")
	}
	if input.Decision != ReviewReject {
		if err := validateJudgment(judgment, proposal.AllowedEvidenceRefs, len(proposal.Judgment.LensResults)); err != nil {
			return OperatorReviewRecord{}, err
		}
		if proposal.RequiresManualReview && judgment.Disposition == "promote" {
			return OperatorReviewRecord{}, errors.New("manual retrieval cannot be promoted without new evidence")
		}
	}
	record := OperatorReviewRecord{
		SchemaVersion: OperatorReviewSchema, CanonicalItemID: proposal.CanonicalItemID, ProposalFingerprint: proposal.Fingerprint,
		StrategyFingerprint: proposal.StrategyFingerprint, Decision: input.Decision, ReviewerID: input.ReviewerID, ReviewedAt: input.ReviewedAt,
		Rationale: input.Rationale, Judgment: judgment, ManualSupportOutcome: input.ManualSupportOutcome,
	}
	record.Fingerprint = acquisition.Fingerprint(record)
	return record, nil
}

func ValidateOperatorReview(record OperatorReviewRecord, proposal Proposal) error {
	if record.SchemaVersion != OperatorReviewSchema || record.Fingerprint == "" || record.Fingerprint != acquisition.Fingerprint(record) {
		return errors.New("operator review fingerprint mismatch")
	}
	if record.CanonicalItemID != proposal.CanonicalItemID || record.ProposalFingerprint != proposal.Fingerprint || record.StrategyFingerprint != proposal.StrategyFingerprint {
		return errors.New("operator review authority mismatch")
	}
	if strings.TrimSpace(record.ReviewerID) == "" || strings.TrimSpace(record.Rationale) == "" {
		return errors.New("invalid operator review attribution")
	}
	if proposal.RequiresManualReview {
		if record.ManualSupportOutcome != "queued_for_manual_processing" && record.ManualSupportOutcome != "confirmed_unavailable" {
			return errors.New("manual-support outcome mismatch")
		}
	} else if record.ManualSupportOutcome != "not_required" {
		return errors.New("manual-support outcome mismatch")
	}
	if _, err := time.Parse(time.RFC3339, record.ReviewedAt); err != nil {
		return errors.New("invalid operator review timestamp")
	}
	if record.Decision == ReviewReject {
		if !isZeroJudgment(record.Judgment) {
			return errors.New("rejected review cannot carry a judgment")
		}
		return nil
	}
	if record.Decision != ReviewAccept && record.Decision != ReviewRevise {
		return errors.New("invalid operator review decision")
	}
	if proposal.RequiresManualReview && record.Judgment.Disposition == "promote" {
		return errors.New("manual retrieval cannot be promoted without new evidence")
	}
	return validateJudgment(record.Judgment, proposal.AllowedEvidenceRefs, len(proposal.Judgment.LensResults))
}

func isZeroJudgment(judgment Judgment) bool {
	return len(judgment.LensResults) == 0 && judgment.SemanticAssessment.PrimaryRole == "" && judgment.SemanticAssessment.Summary == "" && judgment.SemanticAssessment.Confidence == 0 && len(judgment.SemanticAssessment.EvidenceRefs) == 0 && len(judgment.SemanticAssessment.Missingness) == 0 && judgment.Disposition == "" && judgment.DispositionRationale == "" && len(judgment.SemanticNodes) == 0 && len(judgment.SemanticEdges) == 0
}

func ValidateProposal(proposal Proposal) error {
	if proposal.SchemaVersion != ProposalSchema || proposal.CanonicalItemID == "" || proposal.StrategyFingerprint == "" || proposal.ProcessorVersion != EvidenceMatcherVersion || proposal.TokenizerVersion != StrategyTokenizerVersion {
		return errors.New("invalid processing proposal identity")
	}
	if proposal.Fingerprint == "" || proposal.Fingerprint != acquisition.Fingerprint(proposal) {
		return errors.New("processing proposal fingerprint mismatch")
	}
	if proposal.RequiresManualReview && proposal.Judgment.Disposition == "promote" {
		return errors.New("manual retrieval cannot be promoted without new evidence")
	}
	return validateJudgment(proposal.Judgment, proposal.AllowedEvidenceRefs, len(proposal.Judgment.LensResults))
}

func validateJudgment(judgment Judgment, allowedEvidence []string, expectedLenses int) error {
	if len(judgment.LensResults) != expectedLenses || expectedLenses == 0 {
		return errors.New("invalid reviewed lens coverage")
	}
	allowed := map[string]bool{}
	for _, ref := range allowedEvidence {
		if ref == "" || allowed[ref] {
			return errors.New("invalid allowed evidence set")
		}
		allowed[ref] = true
	}
	lenses := map[string]bool{}
	for _, result := range judgment.LensResults {
		if result.LensID == "" || lenses[result.LensID] || result.Confidence < 0 || result.Confidence > 1 || strings.TrimSpace(result.Rationale) == "" || !refsAllowed(result.EvidenceRefs, allowed) {
			return errors.New("invalid reviewed lens result")
		}
		switch result.Result {
		case "matched", "not_matched":
		case "unknown":
			if len(result.Missingness) == 0 {
				return errors.New("unknown reviewed lens requires missingness")
			}
		default:
			return errors.New("invalid reviewed lens result")
		}
		lenses[result.LensID] = true
	}
	roles := map[string]bool{"external_entity": true, "evidence_backed_finding": true, "unresolved_tension": true, "reference_resource": true, "action": true, "unknown": true}
	assessment := judgment.SemanticAssessment
	if !roles[assessment.PrimaryRole] || assessment.Confidence < 0 || assessment.Confidence > 1 || !refsAllowed(assessment.EvidenceRefs, allowed) {
		return errors.New("invalid reviewed semantic assessment")
	}
	if assessment.PrimaryRole == "unknown" {
		if len(assessment.Missingness) == 0 || assessment.Summary != "" || len(assessment.EvidenceRefs) != 0 {
			return errors.New("unknown reviewed assessment must not invent meaning")
		}
	} else if strings.TrimSpace(assessment.Summary) == "" || len(assessment.EvidenceRefs) == 0 {
		return errors.New("reviewed assessment requires evidence")
	}
	switch judgment.Disposition {
	case "promote", "hold", "monitor", "archive", "clarify":
	default:
		return errors.New("invalid reviewed disposition")
	}
	if strings.TrimSpace(judgment.DispositionRationale) == "" {
		return errors.New("reviewed disposition requires rationale")
	}
	if judgment.Disposition == "promote" && len(judgment.SemanticNodes) == 0 {
		return errors.New("promote review requires semantic nodes")
	}
	for _, node := range judgment.SemanticNodes {
		if node.SemanticNodeID == "" || !roles[node.Role] || node.Role == "unknown" || node.Name == "" || node.Description == "" || node.Confidence < 0 || node.Confidence > 1 || len(node.EvidenceRefs) == 0 || !refsAllowed(node.EvidenceRefs, allowed) {
			return errors.New("invalid reviewed semantic node")
		}
	}
	return nil
}

func refsAllowed(refs []string, allowed map[string]bool) bool {
	seen := map[string]bool{}
	for _, ref := range refs {
		if !allowed[ref] || seen[ref] {
			return false
		}
		seen[ref] = true
	}
	return true
}

func cloneJudgment(judgment Judgment) Judgment {
	clone := judgment
	clone.LensResults = append([]LensResult(nil), judgment.LensResults...)
	for index := range clone.LensResults {
		clone.LensResults[index].EvidenceRefs = append([]string(nil), clone.LensResults[index].EvidenceRefs...)
		clone.LensResults[index].Missingness = append([]string(nil), clone.LensResults[index].Missingness...)
	}
	clone.SemanticAssessment.EvidenceRefs = append([]string(nil), judgment.SemanticAssessment.EvidenceRefs...)
	clone.SemanticAssessment.Missingness = append([]string(nil), judgment.SemanticAssessment.Missingness...)
	clone.SemanticNodes = append([]SemanticNode(nil), judgment.SemanticNodes...)
	for index := range clone.SemanticNodes {
		clone.SemanticNodes[index].LensRefs = append([]string(nil), clone.SemanticNodes[index].LensRefs...)
		clone.SemanticNodes[index].EvidenceRefs = append([]string(nil), clone.SemanticNodes[index].EvidenceRefs...)
		if clone.SemanticNodes[index].Attributes != nil {
			attributes := map[string]any{}
			for key, value := range clone.SemanticNodes[index].Attributes {
				attributes[key] = value
			}
			clone.SemanticNodes[index].Attributes = attributes
		}
	}
	clone.SemanticEdges = append([]SemanticEdge(nil), judgment.SemanticEdges...)
	return clone
}
