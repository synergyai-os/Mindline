package documents

import (
	"sort"
	"strings"
)

func semanticFailureReasons() []SemanticFailureReason {
	return []SemanticFailureReason{
		SemanticFailureWrongKind,
		SemanticFailureUnsupportedEvidence,
		SemanticFailureMissingEvidence,
		SemanticFailureUnsafeOrPrivate,
		SemanticFailureDuplicate,
		SemanticFailureTooBroad,
		SemanticFailureTooNarrow,
		SemanticFailureStaleOrContradicted,
		SemanticFailureAmbiguous,
		SemanticFailureMissingExpectedOutcome,
		SemanticFailureUnexpectedCandidate,
		SemanticFailureRelationError,
		SemanticFailureSourceScopeError,
		SemanticFailureOther,
	}
}

func validSemanticFailureReason(reason SemanticFailureReason) bool {
	for _, valid := range semanticFailureReasons() {
		if reason == valid {
			return true
		}
	}
	return false
}

func emptySemanticFailureReasonCounts() map[SemanticFailureReason]int {
	counts := map[SemanticFailureReason]int{}
	for _, reason := range semanticFailureReasons() {
		counts[reason] = 0
	}
	return counts
}

func cloneSemanticFailureReasons(reasons []SemanticFailureReason) []SemanticFailureReason {
	if len(reasons) == 0 {
		return nil
	}
	out := append([]SemanticFailureReason(nil), reasons...)
	sort.SliceStable(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func semanticFailureReasonForAcceptanceReason(reason SemanticAcceptanceReason) (SemanticFailureReason, bool, bool) {
	switch reason {
	case SemanticAcceptanceReasonCorrect:
		return "", false, false
	case SemanticAcceptanceReasonWrongKind:
		return SemanticFailureWrongKind, false, true
	case SemanticAcceptanceReasonUnsupportedEvidence:
		return SemanticFailureUnsupportedEvidence, false, true
	case SemanticAcceptanceReasonMissingEvidence:
		return SemanticFailureMissingEvidence, false, true
	case SemanticAcceptanceReasonUnsafeOrPrivate:
		return SemanticFailureUnsafeOrPrivate, false, true
	case SemanticAcceptanceReasonDuplicate:
		return SemanticFailureDuplicate, false, true
	case SemanticAcceptanceReasonTooBroad:
		return SemanticFailureTooBroad, false, true
	case SemanticAcceptanceReasonTooNarrow:
		return SemanticFailureTooNarrow, false, true
	case SemanticAcceptanceReasonStaleOrContradicted:
		return SemanticFailureStaleOrContradicted, false, true
	case SemanticAcceptanceReasonAmbiguous:
		return SemanticFailureAmbiguous, false, true
	case SemanticAcceptanceReasonMissingExpectedOutcome:
		return SemanticFailureMissingExpectedOutcome, false, true
	case SemanticAcceptanceReasonUnexpectedCandidate:
		return SemanticFailureUnexpectedCandidate, false, true
	default:
		return SemanticFailureOther, true, true
	}
}

func semanticFailureReasonForCalibrationClass(class SemanticCalibrationFailureClass) (SemanticFailureReason, bool, bool) {
	switch class {
	case SemanticCalibrationFailureAccepted:
		return "", true, false
	case SemanticCalibrationFailureFalsePositive:
		return SemanticFailureUnexpectedCandidate, true, true
	case SemanticCalibrationFailureFalseNegative:
		return SemanticFailureMissingExpectedOutcome, true, true
	case SemanticCalibrationFailureMissingEvidence:
		return SemanticFailureMissingEvidence, true, true
	case SemanticCalibrationFailureRelationError:
		return SemanticFailureRelationError, true, true
	case SemanticCalibrationFailureSourceScopeError:
		return SemanticFailureSourceScopeError, true, true
	case SemanticCalibrationFailureBlockedPrivate:
		return SemanticFailureUnsafeOrPrivate, true, true
	case SemanticCalibrationFailureDuplicate:
		return SemanticFailureDuplicate, true, true
	case SemanticCalibrationFailureNeedsReviewAmbiguity:
		return SemanticFailureAmbiguous, true, true
	case SemanticCalibrationFailureOther:
		return SemanticFailureOther, true, true
	default:
		return SemanticFailureOther, true, true
	}
}

func semanticFailureReasonForCalibrationItem(reason SemanticAcceptanceReason, class SemanticCalibrationFailureClass) (SemanticFailureReason, bool, bool) {
	if mapped, inferred, ok := semanticFailureReasonForAcceptanceReason(reason); ok {
		return mapped, inferred, true
	}
	return semanticFailureReasonForCalibrationClass(class)
}

func defaultSemanticFailureReasonForChoice(choice SemanticJudgmentChoice) (SemanticFailureReason, bool) {
	switch choice {
	case SemanticJudgmentChoiceReject:
		return SemanticFailureUnexpectedCandidate, true
	case SemanticJudgmentChoiceUnclear:
		return SemanticFailureAmbiguous, true
	case SemanticJudgmentChoiceDuplicate:
		return SemanticFailureDuplicate, true
	case SemanticJudgmentChoiceWrongKind:
		return SemanticFailureWrongKind, true
	default:
		return "", false
	}
}

func semanticJudgmentChoiceAllowsFailureReason(choice SemanticJudgmentChoice, reason SemanticFailureReason) bool {
	if choice == SemanticJudgmentChoiceAccept {
		return reason == ""
	}
	if !validSemanticFailureReason(reason) {
		return false
	}
	switch choice {
	case SemanticJudgmentChoiceReject:
		switch reason {
		case SemanticFailureUnexpectedCandidate,
			SemanticFailureUnsupportedEvidence,
			SemanticFailureMissingEvidence,
			SemanticFailureTooBroad,
			SemanticFailureTooNarrow,
			SemanticFailureStaleOrContradicted,
			SemanticFailureUnsafeOrPrivate,
			SemanticFailureRelationError,
			SemanticFailureSourceScopeError,
			SemanticFailureOther:
			return true
		}
	case SemanticJudgmentChoiceUnclear:
		switch reason {
		case SemanticFailureAmbiguous,
			SemanticFailureMissingEvidence,
			SemanticFailureUnsupportedEvidence,
			SemanticFailureRelationError,
			SemanticFailureSourceScopeError,
			SemanticFailureOther:
			return true
		}
	case SemanticJudgmentChoiceDuplicate:
		return reason == SemanticFailureDuplicate
	case SemanticJudgmentChoiceWrongKind:
		return reason == SemanticFailureWrongKind
	}
	return false
}

func semanticFailureReasonText(reasons []SemanticFailureReason) string {
	ordered := cloneSemanticFailureReasons(reasons)
	parts := make([]string, 0, len(ordered))
	for _, reason := range ordered {
		parts = append(parts, string(reason))
	}
	return strings.Join(parts, ", ")
}
