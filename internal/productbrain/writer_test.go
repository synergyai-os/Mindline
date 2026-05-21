package productbrain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriterWritesSummaryProposalsAndPreviews(t *testing.T) {
	out := t.TempDir()
	ready := NewProposal(ProposalInput{
		RunID:                "run-0123456789abcdef",
		SourceReviewItemID:   "review-item",
		Intent:               IntentDurableDecision,
		Status:               ProposalStatusReady,
		TargetCollectionSlug: "decisions",
		EntryName:            "Example decision",
		WorkflowStatus:       "pending",
		Data:                 map[string]string{"rationale": "Safe rationale."},
	})
	blocked := NewProposal(ProposalInput{
		RunID:              "run-0123456789abcdef",
		SourceReviewItemID: "review-blocked",
		Intent:             IntentOpenTension,
		Status:             ProposalStatusBlocked,
		Blockers:           []Blocker{{Code: "missing_intent_mapping", Message: "No mapping."}},
	})

	summary, err := Write(out, WriteInput{
		RunID:        "run-0123456789abcdef",
		Profile:      loadProfileFixture(t, "default-governance.json"),
		Proposals:    []Proposal{ready, blocked},
		AuthorityIDs: WP9AuthorityIDs,
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if summary.ProposalCount != 2 || summary.BlockedCount != 1 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	for _, relative := range []string{
		"productbrain-proposals/proposal-summary.json",
		filepath.ToSlash(filepath.Join("productbrain-proposals", "proposals", ready.ProposalID+".json")),
		filepath.ToSlash(filepath.Join("productbrain-proposals", "previews", ready.ProposalID+".md")),
		filepath.ToSlash(filepath.Join("productbrain-proposals", "proposals", blocked.ProposalID+".json")),
		filepath.ToSlash(filepath.Join("productbrain-proposals", "previews", blocked.ProposalID+".md")),
	} {
		if _, err := os.Stat(filepath.Join(out, relative)); err != nil {
			t.Fatalf("expected %s: %v", relative, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(out, "productbrain-proposals", "proposal-summary.json"))
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var decoded Summary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if decoded.Proposals[0].ProposalPath != "proposals/"+ready.ProposalID+".json" {
		t.Fatalf("unexpected proposal path: %+v", decoded.Proposals[0])
	}
	if decoded.Proposals[1].PreviewPath == "" {
		t.Fatalf("blocked proposal should still have preview path: %+v", decoded.Proposals[1])
	}
}

func TestWriterRejectsLeaksAndEscapes(t *testing.T) {
	out := t.TempDir()
	proposal := NewProposal(ProposalInput{
		RunID:                "run-0123456789abcdef",
		SourceReviewItemID:   "../PRIVATE_DM_SENTINEL_DO_NOT_WRITE",
		Intent:               IntentDurableDecision,
		Status:               ProposalStatusReady,
		TargetCollectionSlug: "decisions",
		EntryName:            "https" + "://private.example/source",
		WorkflowStatus:       "pending",
		Data:                 map[string]string{"rationale": "sk-test-secret-do-not-leak"},
	})
	proposal.Operation.EntryName = "https" + "://private.example/source"
	proposal.Operation.Data["rationale"] = "sk-test-secret-do-not-leak"
	err := WriteProposals(out, []Proposal{proposal}, loadProfileFixture(t, "default-governance.json"))
	if err == nil || !strings.Contains(err.Error(), "private or secret") {
		t.Fatalf("WriteProposals() error = %v, want private or secret refusal", err)
	}
}

func TestWriterRejectsSymlinkedOutputParents(t *testing.T) {
	out := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(out, "productbrain-proposals")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	proposal := NewProposal(ProposalInput{
		RunID:                "run-0123456789abcdef",
		SourceReviewItemID:   "review-item",
		Intent:               IntentDurableDecision,
		Status:               ProposalStatusReady,
		TargetCollectionSlug: "decisions",
		EntryName:            "Example decision",
		WorkflowStatus:       "pending",
		Data:                 map[string]string{"rationale": "Safe rationale."},
	})
	err := WriteProposals(out, []Proposal{proposal}, loadProfileFixture(t, "default-governance.json"))
	if err == nil || !strings.Contains(err.Error(), "escaped output directory") {
		t.Fatalf("WriteProposals() error = %v, want escaped output directory refusal", err)
	}
}
