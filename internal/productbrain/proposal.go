package productbrain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	ProposalSchemaVersion        = "productbrain-proposal/v0.1"
	ProposalSummarySchemaVersion = "productbrain-proposal-summary/v0.1"
)

var WP9AuthorityIDs = []string{"PROD-1", "DOMAIN-1", "DEC-15", "WP-8", "WP-9"}

type ProposalStatus string

const (
	ProposalStatusReady   ProposalStatus = "ready"
	ProposalStatusBlocked ProposalStatus = "blocked"
	ProposalStatusSkipped ProposalStatus = "skipped"
)

type Summary struct {
	SchemaVersion    string           `json:"schema_version"`
	RunID            string           `json:"run_id"`
	WorkspaceProfile WorkspaceProfile `json:"workspace_profile"`
	ProposalCount    int              `json:"proposal_count"`
	BlockedCount     int              `json:"blocked_count"`
	Proposals        []SummaryItem    `json:"proposals"`
	AuthorityIDs     []string         `json:"authority_ids"`
}

type WorkspaceProfile struct {
	SchemaVersion string `json:"schema_version"`
	WorkspaceSlug string `json:"workspace_slug"`
	Fingerprint   string `json:"fingerprint"`
}

type SummaryItem struct {
	ProposalID           string         `json:"proposal_id"`
	Status               ProposalStatus `json:"status"`
	Intent               Intent         `json:"intent"`
	TargetCollectionSlug string         `json:"target_collection_slug,omitempty"`
	ProposalPath         string         `json:"proposal_path"`
	PreviewPath          string         `json:"preview_path,omitempty"`
}

type Proposal struct {
	SchemaVersion      string         `json:"schema_version"`
	ProposalID         string         `json:"proposal_id"`
	RunID              string         `json:"run_id"`
	SourceReviewItemID string         `json:"source_review_item_id"`
	Status             ProposalStatus `json:"status"`
	Intent             Intent         `json:"intent"`
	Confidence         string         `json:"confidence"`
	Operation          *Operation     `json:"operation,omitempty"`
	ExternalRef        ExternalRef    `json:"externalRef"`
	IdempotencyKey     string         `json:"idempotencyKey"`
	Actor              Actor          `json:"actor"`
	Provenance         Provenance     `json:"provenance"`
	Blockers           []Blocker      `json:"blockers"`
	AuthorityIDs       []string       `json:"authority_ids"`
}

type Operation struct {
	Kind                 string            `json:"kind"`
	TargetCollectionSlug string            `json:"target_collection_slug"`
	EntryName            string            `json:"entry_name"`
	WorkflowStatus       string            `json:"workflow_status"`
	Data                 map[string]string `json:"data"`
}

type ExternalRef struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

type Actor struct {
	Kind      string `json:"kind"`
	Authority string `json:"authority"`
}

type Provenance struct {
	Surface     string `json:"surface"`
	CapturePath string `json:"capture_path"`
	SourceRunID string `json:"source_run_id"`
}

type Blocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ProposalInput struct {
	RunID                string
	SourceReviewItemID   string
	SourceCandidateID    string
	Intent               Intent
	Status               ProposalStatus
	TargetCollectionSlug string
	EntryName            string
	WorkflowStatus       string
	Data                 map[string]string
	Blockers             []Blocker
}

func NewProposal(input ProposalInput) Proposal {
	proposalID := BuildProposalID(input.RunID, input.SourceReviewItemID, input.Intent, input.TargetCollectionSlug)
	status := input.Status
	if status == "" {
		status = ProposalStatusReady
	}
	confidence := "high"
	if status != ProposalStatusReady {
		confidence = "low"
	}
	proposal := Proposal{
		SchemaVersion:      ProposalSchemaVersion,
		ProposalID:         proposalID,
		RunID:              input.RunID,
		SourceReviewItemID: safeID(input.SourceReviewItemID),
		Status:             status,
		Intent:             input.Intent,
		Confidence:         confidence,
		ExternalRef:        BuildExternalRef(input.SourceCandidateID, input.Intent),
		IdempotencyKey:     BuildIdempotencyKey(input.RunID, proposalID),
		Actor:              Actor{Kind: "integration", Authority: "mindline"},
		Provenance:         Provenance{Surface: "integration", CapturePath: "integration:mindline", SourceRunID: input.RunID},
		Blockers:           append([]Blocker(nil), input.Blockers...),
		AuthorityIDs:       append([]string(nil), WP9AuthorityIDs...),
	}
	if status == ProposalStatusReady {
		proposal.Operation = &Operation{
			Kind:                 "upsert_entry_by_external_ref",
			TargetCollectionSlug: input.TargetCollectionSlug,
			EntryName:            safeText(input.EntryName),
			WorkflowStatus:       input.WorkflowStatus,
			Data:                 safeStringMap(input.Data),
		}
	}
	return proposal
}

func BuildProposalID(runID string, reviewItemID string, intent Intent, targetCollectionSlug string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		ProposalSchemaVersion,
		runID,
		reviewItemID,
		string(intent),
		targetCollectionSlug,
	}, "\x00")))
	return "pbp-" + hex.EncodeToString(sum[:])[:16]
}

func BuildExternalRef(sourceCandidateID string, intent Intent) ExternalRef {
	if strings.TrimSpace(sourceCandidateID) == "" {
		sourceCandidateID = "unknown-source"
	}
	sourceID := safeID(sourceCandidateID)
	if strings.TrimSpace(sourceID) == "" {
		sourceID = "unknown-source"
	}
	return ExternalRef{
		Source: "mindline",
		ID:     fmt.Sprintf("mindline:%s:%s", sourceID, intent),
	}
}

func BuildIdempotencyKey(runID string, proposalID string) string {
	return fmt.Sprintf("mindline:proposal:%s:%s:%s", safeID(runID), safeID(proposalID), ProposalSchemaVersion)
}

func safeID(value string) string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" || strings.Contains(cleaned, "..") || strings.ContainsAny(cleaned, `/\`) || containsUnsafe(cleaned) {
		sum := sha256.Sum256([]byte(cleaned))
		return "item-" + hex.EncodeToString(sum[:])[:16]
	}
	for _, r := range cleaned {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		sum := sha256.Sum256([]byte(cleaned))
		return "item-" + hex.EncodeToString(sum[:])[:16]
	}
	return cleaned
}

func safeText(value string) string {
	if containsUnsafe(value) {
		return "Redacted proposal"
	}
	return strings.TrimSpace(value)
}

func safeStringMap(input map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range input {
		out[key] = safeText(value)
	}
	return out
}

func containsUnsafe(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"private_dm_sentinel_do_not_write",
		"sk-test-secret-do-not-leak",
		"http" + "://",
		"https" + "://",
		"/private",
		"token",
		"..",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
