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
	ClaimGates           []ClaimGate        `json:"claim_gates"`
	Guardrails           Guardrails         `json:"guardrails"`
	TopImprovementTarget ImprovementTarget  `json:"top_improvement_target"`
	RerunInstructions    []string           `json:"rerun_instructions"`
	SafeArtifactRefs     []string           `json:"safe_artifact_refs"`
	Artifacts            []ArtifactEvidence `json:"artifacts"`
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

type ComparisonSummary struct {
	SchemaVersion string             `json:"schema_version"`
	Status        string             `json:"status"`
	ReasonCodes   []string           `json:"reason_codes,omitempty"`
	MetricDeltas  map[string]float64 `json:"metric_deltas,omitempty"`
	BaselineLabel string             `json:"baseline_label"`
	CurrentLabel  string             `json:"current_label"`
}
