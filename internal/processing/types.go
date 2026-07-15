package processing

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/retrieval"
)

const (
	StrategySchema           = "mindline-strategy-snapshot/v0.1"
	ProposalSchema           = "mindline-processing-proposal/v0.1"
	OperatorReviewSchema     = "mindline-operator-review/v0.1"
	EvidenceMatcherVersion   = "evidence_matcher/v0.1"
	StrategyTokenizerVersion = "strategy-tokenizer/v0.1"
)

type StrategySnapshot struct {
	SchemaVersion    string   `json:"schema_version"`
	StrategyID       string   `json:"strategy_id"`
	Version          string   `json:"version"`
	Fingerprint      string   `json:"fingerprint"`
	ContextLenses    string   `json:"context_lenses"`
	RoutingPolicy    string   `json:"routing_policy"`
	SignificantTerms []string `json:"significant_terms"`
	IncludeTerms     []string `json:"include_terms"`
	ExcludeTerms     []string `json:"exclude_terms"`
	OperatorIdentity string   `json:"operator_identity,omitempty"`
	CreatedAt        string   `json:"created_at,omitempty"`
}

type Request struct {
	Item      acquisition.InventoryItem
	Retrieval retrieval.Artifact
	Strategy  StrategySnapshot
}

type Result struct {
	Proposal Proposal
}

type Proposal struct {
	SchemaVersion        string   `json:"schema_version"`
	Fingerprint          string   `json:"fingerprint"`
	CanonicalItemID      string   `json:"canonical_item_id"`
	StrategyFingerprint  string   `json:"strategy_fingerprint"`
	ProcessorVersion     string   `json:"processor_version"`
	TokenizerVersion     string   `json:"tokenizer_version"`
	EvidenceOrigin       string   `json:"evidence_origin"`
	AllowedEvidenceRefs  []string `json:"allowed_evidence_refs"`
	IncludeMatches       []string `json:"include_matches"`
	ExcludeMatches       []string `json:"exclude_matches"`
	RequiresManualReview bool     `json:"requires_manual_review"`
	ReasonCodes          []string `json:"reason_codes"`
	Judgment             Judgment `json:"proposed_judgment"`
}

type Judgment struct {
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

type ReviewDecision string

const (
	ReviewAccept ReviewDecision = "accept"
	ReviewRevise ReviewDecision = "revise"
	ReviewReject ReviewDecision = "reject"
)

type OperatorReviewInput struct {
	Decision             ReviewDecision
	ReviewerID           string
	ReviewedAt           string
	Judgment             Judgment
	Rationale            string
	ManualSupportOutcome string
}

type OperatorReviewRecord struct {
	SchemaVersion        string         `json:"schema_version"`
	Fingerprint          string         `json:"fingerprint"`
	CanonicalItemID      string         `json:"canonical_item_id"`
	ProposalFingerprint  string         `json:"proposal_fingerprint"`
	StrategyFingerprint  string         `json:"strategy_fingerprint"`
	Decision             ReviewDecision `json:"decision"`
	ReviewerID           string         `json:"reviewer_id"`
	ReviewedAt           string         `json:"reviewed_at"`
	Rationale            string         `json:"rationale"`
	Judgment             Judgment       `json:"judgment"`
	ManualSupportOutcome string         `json:"manual_support_outcome"`
}

func SealStrategy(strategy StrategySnapshot) StrategySnapshot {
	strategy.SchemaVersion = StrategySchema
	strategy.SignificantTerms = NormalizeTerms(strategy.SignificantTerms)
	strategy.IncludeTerms = NormalizeTerms(strategy.IncludeTerms)
	strategy.ExcludeTerms = NormalizeTerms(strategy.ExcludeTerms)
	strategy.Fingerprint = ""
	strategy.Fingerprint = acquisition.Fingerprint(strategy)
	return strategy
}

func ValidateStrategy(strategy StrategySnapshot) error {
	if strategy.SchemaVersion != StrategySchema || strings.TrimSpace(strategy.StrategyID) == "" || strings.TrimSpace(strategy.Version) == "" || strings.TrimSpace(strategy.ContextLenses) == "" || strings.TrimSpace(strategy.RoutingPolicy) == "" {
		return errors.New("invalid strategy identity or content")
	}
	if strategy.Fingerprint == "" || strategy.Fingerprint != acquisition.Fingerprint(strategy) {
		return errors.New("strategy fingerprint mismatch")
	}
	if !sameStrings(strategy.SignificantTerms, NormalizeTerms(strategy.SignificantTerms)) || !sameStrings(strategy.IncludeTerms, NormalizeTerms(strategy.IncludeTerms)) || !sameStrings(strategy.ExcludeTerms, NormalizeTerms(strategy.ExcludeTerms)) {
		return errors.New("strategy terms are not normalized")
	}
	if strategy.CreatedAt != "" {
		if _, err := time.Parse(time.RFC3339, strategy.CreatedAt); err != nil {
			return errors.New("invalid strategy creation time")
		}
	}
	if len(ContextLenses(strategy)) == 0 || len(ContextLenses(strategy)) > 8 {
		return errors.New("invalid context lens count")
	}
	return nil
}

func ContextLenses(strategy StrategySnapshot) []string {
	var lenses []string
	for _, line := range strings.Split(strategy.ContextLenses, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "-*0123456789. "))
		if line != "" {
			lenses = append(lenses, line)
		}
	}
	return lenses
}

func ContextLensID(lens string) string { return stableSlug(lens) }

func NormalizeTerms(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
