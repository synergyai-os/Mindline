package documents

import (
	"fmt"
	"strings"
)

func ValidateSegment(segment Segment) error {
	if segment.SchemaVersion != SegmentSchemaVersion {
		return fmt.Errorf("unsupported segment schema version: %s", segment.SchemaVersion)
	}
	if strings.TrimSpace(segment.SegmentID) == "" {
		return fmt.Errorf("missing segment id")
	}
	if sanitizeID(segment.SegmentID) != segment.SegmentID {
		return fmt.Errorf("unsafe segment id: %s", segment.SegmentID)
	}
	if strings.TrimSpace(segment.RunID) == "" {
		return fmt.Errorf("missing run id")
	}
	if strings.TrimSpace(segment.SourceDocumentID) == "" {
		return fmt.Errorf("missing source document id")
	}
	if segment.SourceKind != SourceKindMarkdown {
		return fmt.Errorf("unsupported source kind: %s", segment.SourceKind)
	}
	if !validSemanticType(segment.SemanticType) {
		return fmt.Errorf("unsupported semantic type: %s", segment.SemanticType)
	}
	if !validReviewStatus(segment.ReviewStatus) {
		return fmt.Errorf("unsupported review status: %s", segment.ReviewStatus)
	}
	if !validConfidence(segment.Confidence) {
		return fmt.Errorf("unsupported confidence: %s", segment.Confidence)
	}
	if segment.SemanticType == SemanticTypeUnknown && segment.ReviewStatus != ReviewStatusNeedsReview {
		return fmt.Errorf("unknown segments must need review")
	}
	if segment.Confidence == ConfidenceLow && segment.ReviewStatus == ReviewStatusReady {
		return fmt.Errorf("low confidence segments cannot be ready")
	}
	if segment.ReviewStatus == ReviewStatusReady && (strings.TrimSpace(segment.Title) == "" || strings.TrimSpace(segment.Summary) == "") {
		return fmt.Errorf("ready segments require title and summary")
	}
	if segment.ReviewStatus == ReviewStatusReady && strings.TrimSpace(segment.Evidence.ContentHash) == "" {
		return fmt.Errorf("ready segments require content hash")
	}
	if segment.ReviewStatus == ReviewStatusReady && segment.Evidence.LineStart <= 0 {
		return fmt.Errorf("ready segments require provenance")
	}
	return nil
}

func RejectDuplicateSegmentIDs(segments []Segment) error {
	seen := map[string]bool{}
	for _, segment := range segments {
		if strings.TrimSpace(segment.SegmentID) == "" {
			return fmt.Errorf("missing segment id")
		}
		if seen[segment.SegmentID] {
			return fmt.Errorf("duplicate segment id: %s", segment.SegmentID)
		}
		seen[segment.SegmentID] = true
	}
	return nil
}

func ClassifyUnsafeMarkers(segment Segment) Segment {
	body := segment.Title + "\n" + segment.Summary + "\n" + strings.Join(segment.Evidence.HeadingPath, "\n") + "\n" + segment.SourceDocumentID
	if containsUnsafeMarker(body) {
		segment.ReviewStatus = ReviewStatusBlocked
		segment.Confidence = ConfidenceLow
		if containsUnsafeMarker(segment.SourceDocumentID) {
			segment.SourceDocumentID = redactedDocumentID(segment.SourceDocumentID)
		}
		segment.Title = "Unsafe content redacted"
		segment.Summary = "Segment content was redacted because it contains an unsafe marker."
		for i := range segment.Evidence.HeadingPath {
			segment.Evidence.HeadingPath[i] = "Unsafe heading redacted"
		}
		segment.Blockers = append(segment.Blockers, Blocker{
			Code:    "unsafe_private_marker",
			Message: "Segment contains an unsafe or private marker.",
		})
	}
	return segment
}

func containsUnsafeMarker(value string) bool {
	body := strings.ToLower(value)
	return strings.Contains(body, "private_content") || strings.Contains(body, "secret") || strings.Contains(body, "token")
}

func validSemanticType(value SemanticType) bool {
	switch value {
	case SemanticTypeSourceNote, SemanticTypeMeetingNote, SemanticTypeDecision, SemanticTypeTension, SemanticTypeAction, SemanticTypeCommitment, SemanticTypeStandard, SemanticTypeInsight, SemanticTypeWorkItem, SemanticTypeReference, SemanticTypeUnknown:
		return true
	default:
		return false
	}
}

func validReviewStatus(value ReviewStatus) bool {
	switch value {
	case ReviewStatusReady, ReviewStatusNeedsReview, ReviewStatusBlocked, ReviewStatusSkipped:
		return true
	default:
		return false
	}
}

func validConfidence(value Confidence) bool {
	switch value {
	case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		return true
	default:
		return false
	}
}
