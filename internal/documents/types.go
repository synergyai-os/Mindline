package documents

const (
	SegmentSummarySchemaVersion                    = "document-segment-summary/v0.1"
	SegmentSchemaVersion                           = "document-segment/v0.1"
	StructureSummarySchemaVersion                  = "document-structure-summary/v0.1"
	StructureNodeSchemaVersion                     = "document-structure-node/v0.1"
	SemanticSummarySchemaVersion                   = "semantic-candidate-summary/v0.1"
	SemanticObservationSchemaVersion               = "semantic-observation/v0.1"
	SemanticCandidateSchemaVersion                 = "semantic-candidate/v0.1"
	SemanticRelationSchemaVersion                  = "semantic-relation/v0.1"
	SemanticAcceptanceSummarySchemaVersion         = "semantic-acceptance-summary/v0.1"
	SemanticAcceptanceAnswerKeySchemaVersion       = "semantic-acceptance-answer-key/v0.1"
	SemanticAcceptanceExpectedOutcomeSchemaVersion = "semantic-acceptance-expected-outcome/v0.1"
	SemanticAcceptanceItemSchemaVersion            = "semantic-acceptance-item/v0.1"
	SourceKindMarkdown                             = "markdown"
	EvidenceKindLocation                           = "location"
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

type StructureNodeType string

const (
	StructureNodeTypeDocument       StructureNodeType = "document"
	StructureNodeTypeSection        StructureNodeType = "section"
	StructureNodeTypeTable          StructureNodeType = "table"
	StructureNodeTypeTableRow       StructureNodeType = "table_row"
	StructureNodeTypeCapability     StructureNodeType = "capability"
	StructureNodeTypeTranscriptTurn StructureNodeType = "transcript_turn"
	StructureNodeTypeAudience       StructureNodeType = "audience"
	StructureNodeTypeWorkflow       StructureNodeType = "workflow"
	StructureNodeTypeRequirement    StructureNodeType = "requirement"
	StructureNodeTypeUnknown        StructureNodeType = "unknown"
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

type StructureSummary struct {
	SchemaVersion    string                    `json:"schema_version"`
	RunID            string                    `json:"run_id"`
	SourceCount      int                       `json:"source_count"`
	NodeCount        int                       `json:"node_count"`
	NeedsReviewCount int                       `json:"needs_review_count"`
	BlockedCount     int                       `json:"blocked_count"`
	NodeTypeCounts   map[StructureNodeType]int `json:"node_type_counts"`
	RootNodeIDs      []string                  `json:"root_node_ids"`
	Nodes            []StructureSummaryNode    `json:"nodes"`
}

type StructureSummaryNode struct {
	NodeID           string            `json:"node_id"`
	SourceDocumentID string            `json:"source_document_id"`
	NodeType         StructureNodeType `json:"node_type"`
	ReviewStatus     ReviewStatus      `json:"review_status"`
	Confidence       Confidence        `json:"confidence"`
	NodePath         string            `json:"node_path"`
	PreviewPath      string            `json:"preview_path"`
}

type StructureNode struct {
	SchemaVersion     string            `json:"schema_version"`
	NodeID            string            `json:"node_id"`
	RunID             string            `json:"run_id"`
	SourceDocumentID  string            `json:"source_document_id"`
	NodeType          StructureNodeType `json:"node_type"`
	ReviewStatus      ReviewStatus      `json:"review_status"`
	Confidence        Confidence        `json:"confidence"`
	Title             string            `json:"title"`
	Summary           string            `json:"summary"`
	ParentNodeID      string            `json:"parent_node_id"`
	ChildNodeIDs      []string          `json:"child_node_ids"`
	RelatedSegmentIDs []string          `json:"related_segment_ids"`
	Evidence          StructureEvidence `json:"evidence"`
	Blockers          []Blocker         `json:"blockers"`
	NodePath          string            `json:"-"`
}

type StructureEvidence struct {
	SourceKind        string   `json:"source_kind"`
	SourceDocumentID  string   `json:"source_document_id"`
	HeadingPath       []string `json:"heading_path"`
	LineStart         int      `json:"line_start"`
	LineEnd           int      `json:"line_end"`
	ContentHash       string   `json:"content_hash"`
	RelatedSegmentIDs []string `json:"related_segment_ids"`
}

type SemanticCandidateKind string

const (
	SemanticCandidateKindTopic       SemanticCandidateKind = "topic_candidate"
	SemanticCandidateKindDecision    SemanticCandidateKind = "decision_candidate"
	SemanticCandidateKindAction      SemanticCandidateKind = "action_candidate"
	SemanticCandidateKindIssue       SemanticCandidateKind = "issue_candidate"
	SemanticCandidateKindQuestion    SemanticCandidateKind = "question_candidate"
	SemanticCandidateKindRequirement SemanticCandidateKind = "requirement_candidate"
	SemanticCandidateKindCapability  SemanticCandidateKind = "capability_candidate"
	SemanticCandidateKindDependency  SemanticCandidateKind = "dependency_candidate"
	SemanticCandidateKindRisk        SemanticCandidateKind = "risk_candidate"
	SemanticCandidateKindReference   SemanticCandidateKind = "reference_candidate"
	SemanticCandidateKindUnknown     SemanticCandidateKind = "unknown_candidate"
)

type SemanticObservationKind string

const (
	SemanticObservationKindAgendaFrame          SemanticObservationKind = "agenda_frame"
	SemanticObservationKindClaim                SemanticObservationKind = "claim"
	SemanticObservationKindQuestion             SemanticObservationKind = "question"
	SemanticObservationKindProposal             SemanticObservationKind = "proposal"
	SemanticObservationKindObjection            SemanticObservationKind = "objection"
	SemanticObservationKindDecisionSignal       SemanticObservationKind = "decision_signal"
	SemanticObservationKindActionSignal         SemanticObservationKind = "action_signal"
	SemanticObservationKindOwnerSignal          SemanticObservationKind = "owner_signal"
	SemanticObservationKindDeadlineSignal       SemanticObservationKind = "deadline_signal"
	SemanticObservationKindRecapSignal          SemanticObservationKind = "recap_signal"
	SemanticObservationKindCapabilityStatement  SemanticObservationKind = "capability_statement"
	SemanticObservationKindRequirementStatement SemanticObservationKind = "requirement_statement"
	SemanticObservationKindDependencyStatement  SemanticObservationKind = "dependency_statement"
	SemanticObservationKindRiskStatement        SemanticObservationKind = "risk_statement"
	SemanticObservationKindReferenceStatement   SemanticObservationKind = "reference_statement"
	SemanticObservationKindUnknown              SemanticObservationKind = "unknown_observation"
)

type SemanticRelationshipType string

const (
	SemanticRelationshipSupports         SemanticRelationshipType = "supports"
	SemanticRelationshipRefines          SemanticRelationshipType = "refines"
	SemanticRelationshipContradicts      SemanticRelationshipType = "contradicts"
	SemanticRelationshipAnswers          SemanticRelationshipType = "answers"
	SemanticRelationshipSupersedes       SemanticRelationshipType = "supersedes"
	SemanticRelationshipSummarizes       SemanticRelationshipType = "summarizes"
	SemanticRelationshipSameTopicAs      SemanticRelationshipType = "same_topic_as"
	SemanticRelationshipDependsOn        SemanticRelationshipType = "depends_on"
	SemanticRelationshipAssignsAction    SemanticRelationshipType = "assigns_action"
	SemanticRelationshipMentionsOwner    SemanticRelationshipType = "mentions_owner"
	SemanticRelationshipMentionsDeadline SemanticRelationshipType = "mentions_deadline"
	SemanticRelationshipDerivedFrom      SemanticRelationshipType = "derived_from"
)

type SemanticRelationEndpointType string

const (
	SemanticRelationEndpointStructureNode SemanticRelationEndpointType = "structure_node"
	SemanticRelationEndpointObservation   SemanticRelationEndpointType = "observation"
	SemanticRelationEndpointCandidate     SemanticRelationEndpointType = "candidate"
)

type SemanticDestinationStatus string

const SemanticDestinationUnresolved SemanticDestinationStatus = "unresolved"

type SemanticSummary struct {
	SchemaVersion          string                           `json:"schema_version"`
	RunID                  string                           `json:"run_id"`
	SourceCount            int                              `json:"source_count"`
	ObservationCount       int                              `json:"observation_count"`
	CandidateCount         int                              `json:"candidate_count"`
	RelationCount          int                              `json:"relation_count"`
	NeedsReviewCount       int                              `json:"needs_review_count"`
	BlockedCount           int                              `json:"blocked_count"`
	CandidateKindCounts    map[SemanticCandidateKind]int    `json:"candidate_kind_counts"`
	ObservationKindCounts  map[SemanticObservationKind]int  `json:"observation_kind_counts"`
	RelationshipTypeCounts map[SemanticRelationshipType]int `json:"relationship_type_counts"`
	Candidates             []SemanticSummaryCandidate       `json:"candidates"`
}

type SemanticSummaryCandidate struct {
	CandidateID   string                `json:"candidate_id"`
	CandidateKind SemanticCandidateKind `json:"candidate_kind"`
	ReviewStatus  ReviewStatus          `json:"review_status"`
	Confidence    Confidence            `json:"confidence"`
	CandidatePath string                `json:"candidate_path"`
	PreviewPath   string                `json:"preview_path"`
}

type SemanticEvidenceRange struct {
	StructureNodeID string `json:"structure_node_id"`
	LineStart       int    `json:"line_start"`
	LineEnd         int    `json:"line_end"`
}

type SemanticObservation struct {
	SchemaVersion    string                  `json:"schema_version"`
	ObservationID    string                  `json:"observation_id"`
	RunID            string                  `json:"run_id"`
	SourceDocumentID string                  `json:"source_document_id"`
	ObservationKind  SemanticObservationKind `json:"observation_kind"`
	ReviewStatus     ReviewStatus            `json:"review_status"`
	Confidence       Confidence              `json:"confidence"`
	Title            string                  `json:"title"`
	Summary          string                  `json:"summary"`
	EvidenceNodes    []string                `json:"evidence_nodes"`
	EvidenceRanges   []SemanticEvidenceRange `json:"evidence_ranges"`
	ContentHash      string                  `json:"content_hash"`
	Blockers         []Blocker               `json:"blockers"`
}

type SemanticCandidate struct {
	SchemaVersion     string                    `json:"schema_version"`
	CandidateID       string                    `json:"candidate_id"`
	RunID             string                    `json:"run_id"`
	SourceDocumentID  string                    `json:"source_document_id,omitempty"`
	CandidateKind     SemanticCandidateKind     `json:"candidate_kind"`
	ReviewStatus      ReviewStatus              `json:"review_status"`
	Confidence        Confidence                `json:"confidence"`
	Title             string                    `json:"title"`
	Summary           string                    `json:"summary"`
	EvidenceNodes     []string                  `json:"evidence_nodes"`
	EvidenceRanges    []SemanticEvidenceRange   `json:"evidence_ranges"`
	ObservationIDs    []string                  `json:"observation_ids"`
	RelationIDs       []string                  `json:"relation_ids"`
	DestinationStatus SemanticDestinationStatus `json:"destination_status"`
	Blockers          []Blocker                 `json:"blockers"`
}

type SemanticRelation struct {
	SchemaVersion    string                       `json:"schema_version"`
	RelationID       string                       `json:"relation_id"`
	RunID            string                       `json:"run_id"`
	RelationshipType SemanticRelationshipType     `json:"relationship_type"`
	FromID           string                       `json:"from_id"`
	FromType         SemanticRelationEndpointType `json:"from_type"`
	ToID             string                       `json:"to_id"`
	ToType           SemanticRelationEndpointType `json:"to_type"`
	EvidenceNodes    []string                     `json:"evidence_nodes"`
	Confidence       Confidence                   `json:"confidence"`
	ReviewStatus     ReviewStatus                 `json:"review_status"`
	Blockers         []Blocker                    `json:"blockers"`
}

type SemanticExpectedOutcomeState string

const (
	ExpectedOutcomePresent SemanticExpectedOutcomeState = "expected_present"
	ExpectedOutcomeAbsent  SemanticExpectedOutcomeState = "expected_absent"
)

type SemanticAcceptanceState string

const (
	SemanticAcceptanceAccepted    SemanticAcceptanceState = "accepted"
	SemanticAcceptanceRejected    SemanticAcceptanceState = "rejected"
	SemanticAcceptanceNeedsReview SemanticAcceptanceState = "needs_review"
	SemanticAcceptanceNeedsSplit  SemanticAcceptanceState = "needs_split"
	SemanticAcceptanceNeedsMerge  SemanticAcceptanceState = "needs_merge"
	SemanticAcceptanceBlocked     SemanticAcceptanceState = "blocked"
)

type SemanticAcceptanceReason string

const (
	SemanticAcceptanceReasonCorrect                SemanticAcceptanceReason = "correct"
	SemanticAcceptanceReasonWrongKind              SemanticAcceptanceReason = "wrong_kind"
	SemanticAcceptanceReasonUnsupportedEvidence    SemanticAcceptanceReason = "unsupported_evidence"
	SemanticAcceptanceReasonMissingEvidence        SemanticAcceptanceReason = "missing_evidence"
	SemanticAcceptanceReasonUnsafeOrPrivate        SemanticAcceptanceReason = "unsafe_or_private"
	SemanticAcceptanceReasonDuplicate              SemanticAcceptanceReason = "duplicate"
	SemanticAcceptanceReasonTooBroad               SemanticAcceptanceReason = "too_broad"
	SemanticAcceptanceReasonTooNarrow              SemanticAcceptanceReason = "too_narrow"
	SemanticAcceptanceReasonStaleOrContradicted    SemanticAcceptanceReason = "stale_or_contradicted"
	SemanticAcceptanceReasonAmbiguous              SemanticAcceptanceReason = "ambiguous"
	SemanticAcceptanceReasonMissingExpectedOutcome SemanticAcceptanceReason = "missing_expected_outcome"
	SemanticAcceptanceReasonUnexpectedCandidate    SemanticAcceptanceReason = "unexpected_candidate"
)

type SemanticAcceptanceAnswerKey struct {
	SchemaVersion    string                    `json:"schema_version"`
	AnswerKeyID      string                    `json:"answer_key_id"`
	SourceDocumentID string                    `json:"source_document_id"`
	ExpectedOutcomes []SemanticExpectedOutcome `json:"expected_outcomes"`
}

type SemanticExpectedOutcome struct {
	SchemaVersion          string                       `json:"schema_version,omitempty"`
	ExpectedOutcomeID      string                       `json:"expected_outcome_id"`
	ExpectedState          SemanticExpectedOutcomeState `json:"expected_state"`
	ExpectedKind           SemanticCandidateKind        `json:"expected_kind"`
	RequiredEvidence       []string                     `json:"required_evidence"`
	AcceptableAlternates   []string                     `json:"acceptable_evidence_alternates"`
	TitleSignals           []string                     `json:"title_signals"`
	SummarySignals         []string                     `json:"summary_signals"`
	RelationRequirements   []SemanticRelationshipType   `json:"relation_requirements"`
	MinimumConfidenceFloor Confidence                   `json:"minimum_confidence_floor"`
	Notes                  string                       `json:"notes"`
}

type SemanticAcceptanceSummary struct {
	SchemaVersion                     string                          `json:"schema_version"`
	RunID                             string                          `json:"run_id"`
	AnswerKeyID                       string                          `json:"answer_key_id"`
	CandidateCount                    int                             `json:"candidate_count"`
	ExpectedPresentCount              int                             `json:"expected_present_count"`
	ExpectedAbsentCount               int                             `json:"expected_absent_count"`
	MatchedExpectedCount              int                             `json:"matched_expected_count"`
	MissedExpectedCount               int                             `json:"missed_expected_count"`
	UnexpectedCandidateCount          int                             `json:"unexpected_candidate_count"`
	FalsePositiveCount                int                             `json:"false_positive_count"`
	FalseNegativeCount                int                             `json:"false_negative_count"`
	DuplicateCount                    int                             `json:"duplicate_count"`
	EvidenceMissingCount              int                             `json:"evidence_missing_count"`
	AcceptedCount                     int                             `json:"accepted_count"`
	RejectedCount                     int                             `json:"rejected_count"`
	NeedsReviewCount                  int                             `json:"needs_review_count"`
	BlockedCount                      int                             `json:"blocked_count"`
	ReviewBurdenCount                 int                             `json:"review_burden_count"`
	PrecisionLikeMatchRate            float64                         `json:"precision_like_match_rate"`
	RecallLikeExpectedOutcomeCoverage float64                         `json:"recall_like_expected_outcome_coverage"`
	QualityStatement                  string                          `json:"quality_statement"`
	ExpectedOutcomes                  []SemanticExpectedOutcomeResult `json:"expected_outcomes"`
	Candidates                        []SemanticAcceptanceItemSummary `json:"candidates"`
	Items                             []SemanticAcceptanceItem        `json:"-"`
}

type SemanticExpectedOutcomeResult struct {
	SchemaVersion      string                       `json:"schema_version"`
	ExpectedOutcomeID  string                       `json:"expected_outcome_id"`
	ExpectedState      SemanticExpectedOutcomeState `json:"expected_state"`
	ExpectedKind       SemanticCandidateKind        `json:"expected_kind"`
	AcceptanceState    SemanticAcceptanceState      `json:"acceptance_state"`
	Reason             SemanticAcceptanceReason     `json:"reason"`
	MatchedCandidateID string                       `json:"matched_candidate_id,omitempty"`
	ExpectedPath       string                       `json:"expected_path"`
}

type SemanticAcceptanceItemSummary struct {
	CandidateID     string                   `json:"candidate_id"`
	CandidateKind   SemanticCandidateKind    `json:"candidate_kind"`
	AcceptanceState SemanticAcceptanceState  `json:"acceptance_state"`
	Reason          SemanticAcceptanceReason `json:"reason"`
	ItemPath        string                   `json:"item_path"`
	PreviewPath     string                   `json:"preview_path"`
}

type SemanticAcceptanceItem struct {
	SchemaVersion     string                   `json:"schema_version"`
	CandidateID       string                   `json:"candidate_id"`
	RunID             string                   `json:"run_id"`
	SourceDocumentID  string                   `json:"source_document_id"`
	CandidateKind     SemanticCandidateKind    `json:"candidate_kind"`
	ReviewStatus      ReviewStatus             `json:"review_status"`
	Confidence        Confidence               `json:"confidence"`
	Title             string                   `json:"title"`
	Summary           string                   `json:"summary"`
	EvidenceNodes     []string                 `json:"evidence_nodes"`
	EvidenceRanges    []SemanticEvidenceRange  `json:"evidence_ranges"`
	RelationIDs       []string                 `json:"relation_ids"`
	AcceptanceState   SemanticAcceptanceState  `json:"acceptance_state"`
	Reason            SemanticAcceptanceReason `json:"reason"`
	ExpectedOutcomeID string                   `json:"expected_outcome_id,omitempty"`
	Blockers          []Blocker                `json:"blockers"`
}
