package routing

const (
	LensProfileSchemaVersion = "context-lens-profile/v0.1"
	LinkArtifactsSchema      = "routing-link-artifacts/v0.1"
	JudgmentsSchema          = "routing-judgments/v0.1"
	SourceGraphSchema        = "strategic-source-graph/v0.1"
	DecisionsSchema          = "strategic-route-decisions/v0.1"
	SummarySchema            = "mindline-strategic-routing-summary/v0.1"
)

type LensProfile struct {
	SchemaVersion  string `json:"schema_version"`
	ProfileID      string `json:"profile_id"`
	ProfileVersion string `json:"profile_version"`
	Lenses         []Lens `json:"lenses"`
}

type Lens struct {
	LensID   string   `json:"lens_id"`
	Name     string   `json:"name"`
	Question string   `json:"question"`
	Include  []string `json:"include"`
	Exclude  []string `json:"exclude"`
}

type LinkArtifacts struct {
	SchemaVersion string         `json:"schema_version"`
	Items         []LinkArtifact `json:"items"`
}

type LinkArtifact struct {
	CanonicalURL   string          `json:"canonical_url"`
	RetrievedAt    string          `json:"retrieved_at"`
	State          string          `json:"state"`
	PublicMetadata PublicMetadata  `json:"public_metadata"`
	PublicExcerpts []PublicExcerpt `json:"public_excerpts"`
	RelatedURLs    []RelatedURL    `json:"related_urls"`
	Missingness    []string        `json:"missingness"`
}

type PublicMetadata struct {
	Title       string `json:"title,omitempty"`
	Author      string `json:"author,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

type PublicExcerpt struct {
	ExcerptID string `json:"excerpt_id"`
	Text      string `json:"text"`
	Locator   string `json:"locator"`
}

type RelatedURL struct {
	URL                  string `json:"url"`
	Relation             string `json:"relation"`
	DiscoveryEvidenceRef string `json:"discovery_evidence_ref"`
	SemanticallyRelevant bool   `json:"semantically_relevant"`
}

type Judgments struct {
	SchemaVersion  string           `json:"schema_version"`
	JudgmentMethod string           `json:"judgment_method"`
	JudgedAt       string           `json:"judged_at"`
	ProfileID      string           `json:"profile_id"`
	ProfileVersion string           `json:"profile_version"`
	Sources        []SourceJudgment `json:"sources"`
}

type SourceJudgment struct {
	CanonicalURLID       string             `json:"canonical_url_id"`
	LensResults          []LensResult       `json:"lens_results"`
	SemanticAssessment   SemanticAssessment `json:"semantic_assessment"`
	Disposition          string             `json:"disposition"`
	DispositionRationale string             `json:"disposition_rationale"`
	SemanticNodes        []SemanticNode     `json:"semantic_nodes"`
	SemanticEdges        []SemanticEdge     `json:"semantic_edges"`
}

type LensResult struct {
	LensID       string   `json:"lens_id"`
	Result       string   `json:"result"`
	Confidence   float64  `json:"confidence"`
	Rationale    string   `json:"rationale"`
	EvidenceRefs []string `json:"evidence_refs"`
	Missingness  []string `json:"missingness"`
}

type SemanticAssessment struct {
	PrimaryRole  string   `json:"primary_role"`
	Summary      string   `json:"summary"`
	Confidence   float64  `json:"confidence"`
	EvidenceRefs []string `json:"evidence_refs"`
	Missingness  []string `json:"missingness"`
}

type SemanticNode struct {
	SemanticNodeID string         `json:"semantic_node_id"`
	Role           string         `json:"role"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Confidence     float64        `json:"confidence"`
	LensRefs       []string       `json:"lens_refs"`
	EvidenceRefs   []string       `json:"evidence_refs"`
	Attributes     map[string]any `json:"attributes"`
}

type SemanticEdge struct {
	From         string   `json:"from"`
	Type         string   `json:"type"`
	To           string   `json:"to"`
	Rationale    string   `json:"rationale"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type AdapterRef struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
}

type SourceGraph struct {
	SchemaVersion  string          `json:"schema_version"`
	Fingerprint    string          `json:"fingerprint"`
	Adapter        AdapterRef      `json:"adapter"`
	SourceRecords  []SourceRecord  `json:"source_records"`
	URLOccurrences []URLOccurrence `json:"url_occurrences"`
	CanonicalURLs  []CanonicalURL  `json:"canonical_urls"`
	Edges          []GraphEdge     `json:"edges"`
}

type SourceRecord struct {
	SourceRecordID   string   `json:"source_record_id"`
	SourceKind       string   `json:"source_kind"`
	OccurredAt       string   `json:"occurred_at"`
	RawProvenanceRef string   `json:"raw_provenance_ref"`
	URLOccurrenceIDs []string `json:"url_occurrence_ids"`
}

type URLOccurrence struct {
	URLOccurrenceID string `json:"url_occurrence_id"`
	SourceRecordID  string `json:"source_record_id"`
	ObservedURL     string `json:"observed_url"`
	CanonicalURLID  string `json:"canonical_url_id"`
}

type CanonicalURL struct {
	CanonicalURLID       string   `json:"canonical_url_id"`
	CanonicalURL         string   `json:"canonical_url"`
	Kind                 string   `json:"kind"`
	Depth                int      `json:"depth"`
	ParentCanonicalURLID string   `json:"parent_canonical_url_id,omitempty"`
	Discovery            string   `json:"discovery"`
	EnrichmentState      string   `json:"enrichment_state"`
	Missingness          []string `json:"missingness"`
}

type GraphEdge struct {
	EdgeID       string   `json:"edge_id"`
	Type         string   `json:"type"`
	From         string   `json:"from"`
	To           string   `json:"to"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type RouteDecisions struct {
	SchemaVersion          string         `json:"schema_version"`
	Fingerprint            string         `json:"fingerprint"`
	SourceGraphFingerprint string         `json:"source_graph_fingerprint"`
	LensProfileFingerprint string         `json:"lens_profile_fingerprint"`
	JudgmentMethod         string         `json:"judgment_method"`
	Sources                []RoutedSource `json:"sources"`
}

type RoutedSource struct {
	CanonicalURLID       string             `json:"canonical_url_id"`
	CanonicalURL         string             `json:"canonical_url"`
	Depth                int                `json:"depth"`
	EnrichmentState      string             `json:"enrichment_state"`
	PublicMetadata       PublicMetadata     `json:"public_metadata"`
	PublicExcerpts       []PublicExcerpt    `json:"public_excerpts"`
	Missingness          []string           `json:"missingness"`
	LensResults          []LensResult       `json:"lens_results"`
	SemanticAssessment   SemanticAssessment `json:"semantic_assessment"`
	Disposition          string             `json:"disposition"`
	DispositionRationale string             `json:"disposition_rationale"`
	SemanticNodes        []SemanticNode     `json:"semantic_nodes"`
	SemanticEdges        []SemanticEdge     `json:"semantic_edges"`
}

type EvalProjection struct {
	IntendedUsers        string   `json:"intended_users"`
	InputSourceTypes     []string `json:"input_source_types"`
	OutputSurfaces       []string `json:"output_surfaces"`
	WorkspaceAssumptions string   `json:"workspace_assumptions"`
	ProviderAssumptions  string   `json:"provider_assumptions"`
	PrivacyBoundary      string   `json:"privacy_boundary"`
	SampleStatus         string   `json:"sample_status"`
	HeldOut              bool     `json:"held_out"`
	Generalizable        bool     `json:"generalizable"`
	Thresholds           []string `json:"thresholds"`
	Guardrails           []string `json:"guardrails"`
}

type RouteSummary struct {
	SchemaVersion             string         `json:"schema_version"`
	Fingerprint               string         `json:"fingerprint"`
	SourceGraphFingerprint    string         `json:"source_graph_fingerprint"`
	RouteDecisionsFingerprint string         `json:"route_decisions_fingerprint"`
	LensProfileFingerprint    string         `json:"lens_profile_fingerprint"`
	InputRecordCount          int            `json:"input_record_count"`
	URLOccurrenceCount        int            `json:"url_occurrence_count"`
	PrimaryCanonicalURLCount  int            `json:"primary_canonical_url_count"`
	DepthOneURLCount          int            `json:"depth_one_url_count"`
	CanonicalSourceCount      int            `json:"canonical_source_count"`
	DuplicateOccurrenceCount  int            `json:"duplicate_occurrence_count"`
	LensCount                 int            `json:"lens_count"`
	RequiredLensResultCount   int            `json:"required_lens_result_count"`
	LensResultCount           int            `json:"lens_result_count"`
	DispositionCounts         map[string]int `json:"disposition_counts"`
	EnrichmentStateCounts     map[string]int `json:"enrichment_state_counts"`
	SemanticNodeRoleCounts    map[string]int `json:"semantic_node_role_counts"`
	SemanticEdgeTypeCounts    map[string]int `json:"semantic_edge_type_counts"`
	ValidationFailureCount    int            `json:"validation_failure_count"`
	LocalPrivateFindings      int            `json:"local_private_handling_findings"`
	OutboundPrivacyFindings   int            `json:"outbound_privacy_findings"`
	OperatorJudged            bool           `json:"operator_judged"`
	EvalProjection            EvalProjection `json:"eval_projection"`
}

type Result struct {
	Graph     SourceGraph
	Decisions RouteDecisions
	Summary   RouteSummary
}
