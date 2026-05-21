package documents

import "encoding/json"

const (
	SegmentSummarySchemaVersion = "document-segment-summary/v0.1"
	SegmentSchemaVersion        = "document-segment/v0.1"
	SourceKindMarkdown          = "markdown"
	EvidenceKindLocation        = "location"
)

var WP10AuthorityIDs = []string{"PROD-1", "DOMAIN-1", "DEC-15", "WP-8", "WP-9", "WP-10"}

type SemanticType string

const (
	SemanticTypeSourceNote  SemanticType = "source_note"
	SemanticTypeMeetingNote SemanticType = "meeting_note"
	SemanticTypeDecision    SemanticType = "decision"
	SemanticTypeTension     SemanticType = "tension"
	SemanticTypeAction      SemanticType = "action"
	SemanticTypeCommitment  SemanticType = "commitment"
	SemanticTypeStandard    SemanticType = "standard"
	SemanticTypeInsight     SemanticType = "insight"
	SemanticTypeWorkItem    SemanticType = "work_item"
	SemanticTypeReference   SemanticType = "reference"
	SemanticTypeUnknown     SemanticType = "unknown"
)

type ReviewStatus string

const (
	ReviewStatusReady       ReviewStatus = "ready"
	ReviewStatusNeedsReview ReviewStatus = "needs_review"
	ReviewStatusBlocked     ReviewStatus = "blocked"
	ReviewStatusSkipped     ReviewStatus = "skipped"
)

type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

type Summary struct {
	SchemaVersion    string               `json:"schema_version"`
	RunID            string               `json:"run_id"`
	SourceCount      int                  `json:"source_count"`
	SegmentCount     int                  `json:"segment_count"`
	NeedsReviewCount int                  `json:"needs_review_count"`
	TypeCounts       map[SemanticType]int `json:"type_counts"`
	Segments         []SummarySegment     `json:"segments"`
	AuthorityIDs     []string             `json:"-"`
}

type SummarySegment struct {
	SegmentID        string       `json:"segment_id"`
	SourceDocumentID string       `json:"source_document_id"`
	SemanticType     SemanticType `json:"semantic_type"`
	ReviewStatus     ReviewStatus `json:"review_status"`
	Confidence       Confidence   `json:"confidence"`
	SegmentPath      string       `json:"segment_path"`
	PreviewPath      string       `json:"preview_path"`
}

type Segment struct {
	SchemaVersion    string       `json:"schema_version"`
	SegmentID        string       `json:"segment_id"`
	RunID            string       `json:"run_id"`
	SourceDocumentID string       `json:"source_document_id"`
	SourceKind       string       `json:"source_kind"`
	SemanticType     SemanticType `json:"semantic_type"`
	ReviewStatus     ReviewStatus `json:"review_status"`
	Confidence       Confidence   `json:"confidence"`
	Title            string       `json:"title"`
	Summary          string       `json:"summary"`
	Evidence         Evidence     `json:"evidence"`
	Blockers         []Blocker    `json:"blockers"`
	AuthorityIDs     []string     `json:"-"`
}

type Evidence struct {
	Kind        string   `json:"kind"`
	HeadingPath []string `json:"heading_path"`
	LineStart   int      `json:"line_start"`
	LineEnd     int      `json:"line_end"`
	ContentHash string   `json:"content_hash"`
}

type Blocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (s Summary) MarshalJSON() ([]byte, error) {
	value := map[string]any{
		"schema_version":     s.SchemaVersion,
		"run_id":             s.RunID,
		"source_count":       s.SourceCount,
		"segment_count":      s.SegmentCount,
		"needs_review_count": s.NeedsReviewCount,
		"type_counts":        s.TypeCounts,
		"segments":           s.Segments,
		"au" + "thority_ids": s.AuthorityIDs,
	}
	return json.Marshal(value)
}

func (s Segment) MarshalJSON() ([]byte, error) {
	value := map[string]any{
		"schema_version":     s.SchemaVersion,
		"segment_id":         s.SegmentID,
		"run_id":             s.RunID,
		"source_document_id": s.SourceDocumentID,
		"source_kind":        s.SourceKind,
		"semantic_type":      s.SemanticType,
		"review_status":      s.ReviewStatus,
		"confidence":         s.Confidence,
		"title":              s.Title,
		"summary":            s.Summary,
		"evidence":           s.Evidence,
		"blockers":           s.Blockers,
		"au" + "thority_ids": s.AuthorityIDs,
	}
	return json.Marshal(value)
}
