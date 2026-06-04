package evalreadback

const (
	SummarySchemaVersion    = "mindline-eval-readback-summary/v0.1"
	ComparisonSchemaVersion = "mindline-eval-readback-comparison/v0.1"
	DirName                 = "eval-readback"
)

type Options struct {
	BaselineRoot   string
	ProtectedRoots []string
}

const ReadbackSummaryFile = "readback-summary.json"

type Summary struct {
	SchemaVersion        string             `json:"schema_version"`
	RunID                string             `json:"run_id"`
	InputRootLabel       string             `json:"input_root_label"`
	BaselineRootLabel    string             `json:"baseline_root_label,omitempty"`
	ArtifactCount        int                `json:"artifact_count"`
	ArtifactTypeCounts   map[string]int     `json:"artifact_type_counts"`
	SampleStatus         string             `json:"sample_status"`
	GeneralizationStatus string             `json:"generalization_status"`
	ImprovementStatus    string             `json:"improvement_status"`
	SemanticReadiness    SemanticReadiness  `json:"semantic_readiness"`
	ClaimGates           []ClaimGate        `json:"claim_gates"`
	Guardrails           Guardrails         `json:"guardrails"`
	TopImprovementTarget ImprovementTarget  `json:"top_improvement_target"`
	RerunInstructions    []string           `json:"rerun_instructions"`
	ReplayBaseline       ReplayBaseline     `json:"replay_baseline"`
	SafeArtifactRefs     []string           `json:"safe_artifact_refs"`
	Artifacts            []ArtifactEvidence `json:"artifacts"`
	BaselineArtifactRefs []string           `json:"baseline_artifact_refs,omitempty"`
	BaselineArtifacts    []ArtifactEvidence `json:"baseline_artifacts,omitempty"`
	Comparison           *ComparisonSummary `json:"comparison,omitempty"`
}

type ArtifactEvidence struct {
	Type          string             `json:"type"`
	SchemaVersion string             `json:"schema_version,omitempty"`
	Ref           string             `json:"ref"`
	Status        string             `json:"status"`
	ReasonCodes   []string           `json:"reason_codes,omitempty"`
	Metrics       map[string]float64 `json:"metrics,omitempty"`
	Flags         map[string]bool    `json:"flags,omitempty"`
	Fingerprints  map[string]string  `json:"fingerprints,omitempty"`
}

type ClaimGate struct {
	Gate         string   `json:"gate"`
	Status       string   `json:"status"`
	ReasonCodes  []string `json:"reason_codes,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	ClaimImpact  string   `json:"claim_impact"`
}

type Guardrails struct {
	NetworkFetches            int  `json:"network_fetches"`
	HostedTelemetryExports    int  `json:"hosted_telemetry_exports"`
	HostedInferenceCalls      int  `json:"hosted_inference_calls"`
	BrowserCalls              int  `json:"browser_calls"`
	SlackAPICalls             int  `json:"slack_api_calls"`
	DestinationWrites         int  `json:"destination_writes"`
	ProductBrainWrites        int  `json:"product_brain_writes"`
	TolariaWrites             int  `json:"tolaria_writes"`
	AutoAccepts               int  `json:"auto_accepts"`
	NoHumanClaims             bool `json:"no_human_claims"`
	CommittedPrivateArtifacts int  `json:"committed_private_artifacts"`
}

type ImprovementTarget struct {
	Code         string   `json:"code"`
	Rationale    string   `json:"rationale"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

type SemanticReadiness struct {
	Status                     string   `json:"status"`
	ReasonCodes                []string `json:"reason_codes,omitempty"`
	ProcessedSourceCount       int      `json:"processed_source_count"`
	DocumentSegmentCount       int      `json:"document_segment_count"`
	SemanticObservationCount   int      `json:"semantic_observation_count"`
	SemanticCandidateCount     int      `json:"semantic_candidate_count"`
	ReferenceCandidateCount    int      `json:"reference_candidate_count"`
	OneCandidateSourceCount    int      `json:"one_candidate_source_count"`
	ReferenceOnlySourceCount   int      `json:"reference_only_source_count"`
	CandidatePerSourceRatio    float64  `json:"candidate_per_processed_source_ratio"`
	ObservationPerSegmentRatio float64  `json:"observation_per_segment_ratio"`
	ReferenceCandidateRatio    float64  `json:"reference_candidate_ratio"`
}

type ReplayBaseline struct {
	Status                   string   `json:"status"`
	ReasonCodes              []string `json:"reason_codes,omitempty"`
	CorpusFingerprint        string   `json:"corpus_fingerprint,omitempty"`
	CommandConfigFingerprint string   `json:"command_config_fingerprint,omitempty"`
	ArtifactTypes            []string `json:"artifact_types"`
	SafeArtifactRefs         []string `json:"safe_artifact_refs"`
	RerunInstruction         string   `json:"rerun_instruction"`
}

type ComparisonSummary struct {
	SchemaVersion string             `json:"schema_version"`
	Status        string             `json:"status"`
	ReasonCodes   []string           `json:"reason_codes,omitempty"`
	MetricDeltas  map[string]float64 `json:"metric_deltas,omitempty"`
	BaselineLabel string             `json:"baseline_label"`
	CurrentLabel  string             `json:"current_label"`
}
