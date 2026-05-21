package processors

import (
	"net/url"
	"strings"

	"github.com/synergyai-os/Mindline/internal/pipeline/methods"
)

type Input struct {
	Text              string
	URLs              []URLUnit
	PrivateProvenance bool
	SecretLike        bool
}

type URLUnit struct {
	URL  string `json:"url"`
	Kind string `json:"kind"`
}

type PlanResult struct {
	SchemaVersion string   `json:"schema_version"`
	Steps         []Step   `json:"steps"`
	Blockers      []string `json:"blockers,omitempty"`
	AuthorityIDs  []string `json:"authority_ids"`
}

type Step struct {
	StepID      string `json:"step_id"`
	ContentID   string `json:"content_id"`
	ContentKind string `json:"content_kind"`
	ProcessorID string `json:"processor_id"`
	Requirement string `json:"requirement"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
}

func Plan(input Input, profile methods.Profile, authorityIDs []string) PlanResult {
	result := PlanResult{SchemaVersion: "processor-plan/v0.1", AuthorityIDs: append([]string(nil), authorityIDs...)}
	if input.SecretLike {
		result.Blockers = []string{"secret_like_content_detected"}
		result.Steps = []Step{{StepID: "step-1", ContentID: "candidate", ContentKind: "secret_like", ProcessorID: "secret_skip", Requirement: "blocked", Status: "blocked", Reason: "secret_like_content_detected"}}
		return result
	}
	if input.PrivateProvenance {
		result.Blockers = []string{"private_provenance_requires_review"}
		result.Steps = []Step{{StepID: "step-1", ContentID: "candidate", ContentKind: "private_provenance", ProcessorID: "private_provenance_block", Requirement: "blocked", Status: "blocked", Reason: "private_provenance_requires_review"}}
		return result
	}
	if len(input.URLs) == 0 {
		result.Steps = []Step{{StepID: "step-1", ContentID: "text-1", ContentKind: "text", ProcessorID: profile.ProcessorPolicy["text"].RequiredProcessor, Requirement: "required", Status: "planned"}}
		return result
	}
	var missing []string
	for index, unit := range input.URLs {
		kind := unit.Kind
		if strings.TrimSpace(kind) == "" {
			kind = DetectURLKind(unit.URL)
		}
		policy, ok := profile.ProcessorPolicy[kind]
		if !ok {
			kind = "unknown"
			policy = profile.ProcessorPolicy[kind]
		}
		if kind == "unknown" {
			missing = append(missing, policy.MissingArtifactReason)
			continue
		}
		result.Steps = append(result.Steps, Step{
			StepID:      stepID(len(result.Steps) + 1),
			ContentID:   "url-" + itoa(index+1),
			ContentKind: kind,
			ProcessorID: policy.RequiredProcessor,
			Requirement: "required",
			Status:      "planned",
		})
		if policy.MissingArtifactReason != "" {
			missing = append(missing, policy.MissingArtifactReason)
		}
	}
	if len(missing) > 0 {
		result.Blockers = missing
		reason := missing[0]
		if len(missing) > 1 {
			reason = "missing_required_local_artifacts"
		}
		result.Steps = append(result.Steps, Step{
			StepID:      stepID(len(result.Steps) + 1),
			ContentID:   "candidate",
			ContentKind: "manual",
			ProcessorID: "manual_processing_required",
			Requirement: "blocked",
			Status:      "blocked",
			Reason:      reason,
		})
	}
	return result
}

func DetectURLKind(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return "unknown"
	}
	host := strings.ToLower(parsed.Host)
	path := strings.ToLower(parsed.Path)
	switch {
	case strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be"):
		return "youtube_url"
	case strings.Contains(host, "linkedin.com"):
		return "linkedin_url"
	case strings.HasSuffix(path, ".pdf"):
		return "pdf_url"
	case parsed.Scheme == "http" || parsed.Scheme == "https":
		return "web_url"
	default:
		return "unknown"
	}
}

func stepID(value int) string {
	return "step-" + itoa(value)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
