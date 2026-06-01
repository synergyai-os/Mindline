package evalloopdecision

import "github.com/synergyai-os/Mindline/internal/evalreadback"

const (
	PacketSchemaVersion = "mindline-eval-loop-decision/v0.1"
	DirName             = "eval-loop-decision"

	ImprovementImproved               = "improved"
	ImprovementNotImproved            = "not_improved"
	ImprovementInconclusive           = "inconclusive"
	ImprovementNotComparable          = "not_comparable"
	ImprovementBlockedMissingBaseline = "blocked_missing_baseline"
	GeneralizationBlocked             = "blocked"
	DEC64Blocked                      = "blocked"
	SafetyPass                        = "pass"
	SafetyBlocked                     = "blocked"
)

type Options struct {
	BaselineRoot   string
	ProtectedRoots []string
}

type Packet struct {
	SchemaVersion        string                          `json:"schema_version"`
	RunID                string                          `json:"run_id"`
	DecisionKind         string                          `json:"decision_kind"`
	CurrentRootLabel     string                          `json:"current_root_label"`
	BaselineRootLabel    string                          `json:"baseline_root_label,omitempty"`
	ImprovementState     string                          `json:"improvement_state"`
	ClaimStatuses        ClaimStatuses                   `json:"claim_statuses"`
	TopImprovementTarget evalreadback.ImprovementTarget  `json:"top_improvement_target"`
	ProductGeneralTarget string                          `json:"product_general_target"`
	RerunInstruction     string                          `json:"rerun_instruction"`
	Comparison           *evalreadback.ComparisonSummary `json:"comparison,omitempty"`
	Guardrails           evalreadback.Guardrails         `json:"guardrails"`
	SafeArtifactRefs     []string                        `json:"safe_artifact_refs"`
	ReadbackSummaryRef   string                          `json:"readback_summary_ref"`
	DecisionLimits       []string                        `json:"decision_limits"`
}

type ClaimStatuses struct {
	Safety         string `json:"safety"`
	Improvement    string `json:"improvement"`
	Generalization string `json:"generalization"`
	DEC64          string `json:"dec64"`
}
