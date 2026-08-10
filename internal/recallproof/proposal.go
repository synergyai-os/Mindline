// Package recallproof keeps WP-48 proof bindings and proposed manifest groups
// separate from the currently signed assurance manifest. It intentionally
// emits structural commitments only.
package recallproof

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
)

const BindingSchemaVersion = "mindline-recall-proof-binding/v0.1"

type TreeConfigBinding struct {
	SchemaVersion                string `json:"schema_version"`
	Commit                       string `json:"commit"`
	TreeFingerprint              string `json:"tree_fingerprint"`
	BinaryFingerprint            string `json:"binary_fingerprint"`
	AssuranceManifestFingerprint string `json:"assurance_manifest_fingerprint"`
	LiveConfigurationFingerprint string `json:"live_configuration_fingerprint"`
}

// Validate verifies the exact structural binding required before live source
// acquisition and again at close-out. It accepts no paths, source identifiers,
// credentials, URLs, or private evidence.
func (binding TreeConfigBinding) Validate() error {
	if binding.SchemaVersion != BindingSchemaVersion {
		return fmt.Errorf("unsupported binding schema: %s", binding.SchemaVersion)
	}
	if !commitPattern.MatchString(binding.Commit) {
		return errors.New("binding commit must be a full git commit")
	}
	for _, value := range []string{binding.TreeFingerprint, binding.BinaryFingerprint, binding.AssuranceManifestFingerprint, binding.LiveConfigurationFingerprint} {
		if !fingerprintPattern.MatchString(value) {
			return errors.New("binding fingerprints must be sha256 commitments")
		}
	}
	return nil
}

func RequireExactBinding(preLive, final TreeConfigBinding) error {
	if err := preLive.Validate(); err != nil {
		return err
	}
	if err := final.Validate(); err != nil {
		return err
	}
	if preLive != final {
		return errors.New("final tree/config binding differs from pre-live authority")
	}
	return nil
}

// GroupProposal is the exact structural content to add to a future signed
// WP-48 assurance manifest. It is not executable authority by itself.
type GroupProposal struct {
	ID               string   `json:"id"`
	Phase            string   `json:"phase"`
	DependsOn        []string `json:"depends_on"`
	RunPolicy        string   `json:"run_policy"`
	Tool             string   `json:"tool"`
	Argv             []string `json:"argv"`
	Artifacts        []string `json:"artifacts"`
	RequiredEvidence []string `json:"required_evidence"`
	RequiredBindings []string `json:"required_bindings"`
}

func WP48ProofGroupProposals() []GroupProposal {
	return []GroupProposal{
		{
			ID: "wp48_reusable_lifecycle", Phase: "pre_live", RunPolicy: "exact_frozen_tree", Tool: "${MINDLINE_PROOF_RUNNER}", Argv: []string{"group", "wp48_reusable_lifecycle"},
			Artifacts:        []string{"wp48-reusable-lifecycle.json", "wp48-privacy-scan.json"},
			RequiredEvidence: []string{"synthetic_closed_envelope", "adoption_receipts", "conflict_preserves_fingerprint", "replay_restart_duplicate_revision_capacity", "terminal_resources_queue_rebuild", "legacy_compact_rollback", "structural_privacy_scan"}, RequiredBindings: bindingFields(),
		},
		{
			ID: "wp48_private_drain", Phase: "live", DependsOn: []string{"wp48_reusable_lifecycle"}, RunPolicy: "exact_pre_live_receipt", Tool: "${MINDLINE_PROOF_RUNNER}", Argv: []string{"group", "wp48_private_drain"},
			Artifacts:        []string{"wp48-live-structural-receipt.json", "wp48-live-canonical-readback.json"},
			RequiredEvidence: []string{"strict_live_envelope", "per_unit_adoption_receipts", "canonical_readback", "repository_admission", "queue_rebuild_readback", "privacy_scan"}, RequiredBindings: bindingFields(),
		},
		{
			ID: "wp48_retrieval_eval", Phase: "post_live", DependsOn: []string{"wp48_private_drain"}, RunPolicy: "same_library_manifest", Tool: "${MINDLINE_PROOF_RUNNER}", Argv: []string{"group", "wp48_retrieval_eval"},
			Artifacts:        []string{"wp48-retrieval-eval-result.json"},
			RequiredEvidence: []string{"independently_labelled_manifest", "same_library_baseline", "abstention_threshold_result", "compact_privacy_result"}, RequiredBindings: bindingFields(),
		},
		{
			ID: "wp48_fresh_agent_handoff", Phase: "post_live", DependsOn: []string{"wp48_private_drain", "wp48_retrieval_eval"}, RunPolicy: "installed_surface_only", Tool: "${MINDLINE_PROOF_RUNNER}", Argv: []string{"group", "wp48_fresh_agent_handoff"},
			Artifacts:        []string{"wp48-fresh-agent-result.json"},
			RequiredEvidence: []string{"installed_skill_run", "selected_hydration", "bounded_feedback", "founder_outcome"}, RequiredBindings: bindingFields(),
		},
		{
			ID: "wp48_final_revalidation", Phase: "close", DependsOn: []string{"wp48_fresh_agent_handoff"}, RunPolicy: "exact_final_binding", Tool: "${MINDLINE_PROOF_RUNNER}", Argv: []string{"group", "wp48_final_revalidation"},
			Artifacts:        []string{"wp48-final-binding.json", "wp48-unchanged-tree-reviews.json"},
			RequiredEvidence: []string{"final_tree_match", "unchanged_tree_reviews", "closure_readback"}, RequiredBindings: bindingFields(),
		},
	}
}

func ValidateGroupProposals(groups []GroupProposal) error {
	if len(groups) == 0 {
		return errors.New("no proof groups proposed")
	}
	seen := map[string]struct{}{}
	for index, group := range groups {
		if !groupIDPattern.MatchString(group.ID) || group.Phase == "" || group.RunPolicy == "" || group.Tool != "${MINDLINE_PROOF_RUNNER}" || len(group.Argv) != 2 || group.Argv[0] != "group" || group.Argv[1] != group.ID || len(group.Artifacts) == 0 || len(group.RequiredEvidence) == 0 || len(group.RequiredBindings) == 0 {
			return fmt.Errorf("invalid group proposal: %s", group.ID)
		}
		if _, exists := seen[group.ID]; exists {
			return fmt.Errorf("duplicate group proposal: %s", group.ID)
		}
		for _, dependency := range group.DependsOn {
			if _, exists := seen[dependency]; !exists {
				return fmt.Errorf("group %s depends on a later or unknown group %s", group.ID, dependency)
			}
		}
		if !equalSorted(group.RequiredBindings, bindingFields()) {
			return fmt.Errorf("group %s does not require the complete tree/config binding", group.ID)
		}
		seen[group.ID] = struct{}{}
		_ = index
	}
	return nil
}

func bindingFields() []string {
	return []string{"commit", "tree_fingerprint", "binary_fingerprint", "assurance_manifest_fingerprint", "live_configuration_fingerprint"}
}

func equalSorted(left, right []string) bool {
	left, right = append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
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

var (
	commitPattern      = regexp.MustCompile(`^[a-f0-9]{40}$`)
	fingerprintPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	groupIDPattern     = regexp.MustCompile(`^wp48_[a-z0-9_]+$`)
)
