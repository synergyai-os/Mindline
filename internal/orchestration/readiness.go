package orchestration

import (
	"sort"
	"strings"
)

type ReadinessStage string

const (
	StageInventory         ReadinessStage = "READY_TO_INVENTORY"
	StageProcess           ReadinessStage = "READY_TO_PROCESS"
	StageExperimentalDrain ReadinessStage = "READY_TO_EXPERIMENTAL_DRAIN"
	StageDeliver           ReadinessStage = "READY_TO_DELIVER"
)

type CheckStatus string

const (
	CheckPass    CheckStatus = "pass"
	CheckFail    CheckStatus = "fail"
	CheckPending CheckStatus = "pending"
	CheckNA      CheckStatus = "n/a"
)

type ReadinessVerdictValue string

const (
	VerdictReady       ReadinessVerdictValue = "READY"
	VerdictConditional ReadinessVerdictValue = "CONDITIONAL"
	VerdictBlocked     ReadinessVerdictValue = "BLOCKED"
)

type ReadinessCheck struct {
	Name                string      `json:"name"`
	Status              CheckStatus `json:"status"`
	EvidenceFingerprint string      `json:"evidence_fingerprint,omitempty"`
	NARationale         string      `json:"na_rationale,omitempty"`
	ContractAllowsNA    bool        `json:"contract_allows_na,omitempty"`
}

type ReadinessContribution struct {
	ContributorID  string           `json:"contributor_id"`
	Version        string           `json:"version"`
	RequiredChecks []string         `json:"required_checks"`
	Checks         []ReadinessCheck `json:"checks"`
}

type ReadinessVerdict struct {
	Stage               ReadinessStage        `json:"stage"`
	Verdict             ReadinessVerdictValue `json:"verdict"`
	EvidenceFingerprint string                `json:"evidence_fingerprint"`
	Blockers            []string              `json:"blockers,omitempty"`
	Conditions          []string              `json:"conditions,omitempty"`
}

func EvaluateReadiness(stage ReadinessStage, contributions ...ReadinessContribution) ReadinessVerdict {
	verdict := ReadinessVerdict{Stage: stage, Verdict: VerdictReady}
	if !validReadinessStage(stage) || len(contributions) == 0 {
		verdict.Verdict = VerdictBlocked
		verdict.Blockers = append(verdict.Blockers, "readiness_evidence:missing")
		verdict.EvidenceFingerprint = Fingerprint(verdict)
		return verdict
	}
	seenContributors := map[string]bool{}
	for _, contribution := range contributions {
		prefix := strings.TrimSpace(contribution.ContributorID)
		if prefix == "" || contribution.Version == "" || seenContributors[prefix] {
			verdict.Blockers = append(verdict.Blockers, prefix+":invalid_contributor")
			continue
		}
		seenContributors[prefix] = true
		required := map[string]bool{}
		for _, name := range contribution.RequiredChecks {
			if name == "" || required[name] {
				verdict.Blockers = append(verdict.Blockers, prefix+":"+name+":invalid_required_set")
			}
			required[name] = true
		}
		checks := map[string]ReadinessCheck{}
		duplicates := map[string]bool{}
		for _, check := range contribution.Checks {
			if check.Name == "" || checks[check.Name].Name != "" {
				duplicates[check.Name] = true
				continue
			}
			checks[check.Name] = check
		}
		for name := range checks {
			if !required[name] {
				verdict.Blockers = append(verdict.Blockers, prefix+":"+name+":undeclared")
			}
		}
		for _, name := range contribution.RequiredChecks {
			check, exists := checks[name]
			if !exists {
				verdict.Blockers = append(verdict.Blockers, prefix+":"+name+":missing")
				continue
			}
			if duplicates[name] {
				verdict.Blockers = append(verdict.Blockers, prefix+":"+name+":duplicate")
				continue
			}
			switch check.Status {
			case CheckPass:
				if check.EvidenceFingerprint == "" {
					verdict.Blockers = append(verdict.Blockers, prefix+":"+name+":missing_evidence")
				}
			case CheckFail:
				verdict.Blockers = append(verdict.Blockers, prefix+":"+name+":failed")
			case CheckPending:
				verdict.Conditions = append(verdict.Conditions, prefix+":"+name+":pending")
			case CheckNA:
				if !check.ContractAllowsNA || strings.TrimSpace(check.NARationale) == "" {
					verdict.Blockers = append(verdict.Blockers, prefix+":"+name+":na_not_authorized")
				}
			default:
				verdict.Blockers = append(verdict.Blockers, prefix+":"+name+":invalid_status")
			}
		}
	}
	sort.Strings(verdict.Blockers)
	sort.Strings(verdict.Conditions)
	if len(verdict.Blockers) > 0 {
		verdict.Verdict = VerdictBlocked
	} else if len(verdict.Conditions) > 0 {
		verdict.Verdict = VerdictConditional
	}
	verdict.EvidenceFingerprint = Fingerprint(verdict)
	return verdict
}

func validReadinessStage(stage ReadinessStage) bool {
	switch stage {
	case StageInventory, StageProcess, StageExperimentalDrain, StageDeliver:
		return true
	default:
		return false
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
