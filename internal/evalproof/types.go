package evalproof

import "github.com/synergyai-os/Mindline/internal/evalreadback"

const (
	PacketSchemaVersion = "mindline-eval-proof-packet/v0.1"
	DirName             = "eval-proof"

	ClaimSafety         = "safety"
	ClaimImprovement    = "improvement"
	ClaimGeneralization = "generalization"
	ClaimDEC64          = "dec64"

	VerdictPass    = "pass"
	VerdictFail    = "fail"
	VerdictBlocked = "blocked"
)

type Options struct {
	BaselineRoot   string
	Claim          string
	ProtectedRoots []string
}

type Packet struct {
	SchemaVersion        string                         `json:"schema_version"`
	RunID                string                         `json:"run_id"`
	Claim                string                         `json:"claim"`
	Verdict              string                         `json:"verdict"`
	ExitCode             int                            `json:"exit_code"`
	InputRootLabel       string                         `json:"input_root_label"`
	BaselineRootLabel    string                         `json:"baseline_root_label,omitempty"`
	ReadbackSummaryRef   string                         `json:"readback_summary_ref"`
	EvalProjection       EvalProjection                 `json:"eval_projection"`
	MandatoryGates       []GateResult                   `json:"mandatory_gates"`
	BlockedClaims        []ClaimResult                  `json:"blocked_claims,omitempty"`
	FailedClaims         []ClaimResult                  `json:"failed_claims,omitempty"`
	PermittedClaims      []ClaimResult                  `json:"permitted_claims,omitempty"`
	GeneralizationLimit  string                         `json:"generalization_limit"`
	TopImprovementTarget evalreadback.ImprovementTarget `json:"top_improvement_target"`
	RerunInstructions    []string                       `json:"rerun_instructions"`
	SafeArtifactRefs     []string                       `json:"safe_artifact_refs"`
}

type EvalProjection struct {
	IntendedUsers              string   `json:"intended_users"`
	InputSourceTypes           []string `json:"input_source_types"`
	OutputDestinationSurfaces  []string `json:"output_destination_surfaces"`
	WorkspaceAssumptions       string   `json:"workspace_assumptions"`
	ProviderModelAssumptions   string   `json:"provider_model_assumptions"`
	PrivacyBoundary            string   `json:"privacy_boundary"`
	SampleStatus               string   `json:"sample_status"`
	HeldOutGeneralizationClaim string   `json:"held_out_generalization_claim"`
	KRThresholds               []string `json:"kr_thresholds"`
	Guardrails                 string   `json:"guardrails"`
}

type GateResult struct {
	Gate             string   `json:"gate"`
	RequiredStatuses []string `json:"required_statuses"`
	ActualStatus     string   `json:"actual_status"`
	Verdict          string   `json:"verdict"`
	ReasonCodes      []string `json:"reason_codes,omitempty"`
	ClaimImpact      string   `json:"claim_impact"`
}

type ClaimResult struct {
	Claim       string   `json:"claim"`
	Status      string   `json:"status"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
}
