package productbrain

import "testing"

func TestResolveDefaultAndCustomProfiles(t *testing.T) {
	input := ResolveInput{
		RunID:             "run-0123456789abcdef",
		ReviewItemID:      "review-decision",
		SourceCandidateID: "slack-DSELF-1710000000000001",
		SafeTitle:         "Choose workspace profile bridge",
		SafeContext:       "Decision: use Product Brain workspace profiles before live writes.",
		Intent:            IntentDurableDecision,
	}

	defaultProposal := Resolve(input, loadProfileFixture(t, "default-governance.json"))
	if defaultProposal.Status != ProposalStatusReady {
		t.Fatalf("default proposal status = %s blockers=%v", defaultProposal.Status, defaultProposal.Blockers)
	}
	if defaultProposal.Operation == nil || defaultProposal.Operation.TargetCollectionSlug != "decisions" {
		t.Fatalf("default target = %+v", defaultProposal.Operation)
	}
	if defaultProposal.Operation.Data["rationale"] == "" {
		t.Fatalf("expected rationale field to be populated")
	}

	customProposal := Resolve(input, loadProfileFixture(t, "custom-workspace.json"))
	if customProposal.Status != ProposalStatusReady {
		t.Fatalf("custom proposal status = %s blockers=%v", customProposal.Status, customProposal.Blockers)
	}
	if customProposal.Operation == nil || customProposal.Operation.TargetCollectionSlug != "choices" {
		t.Fatalf("custom target = %+v", customProposal.Operation)
	}
	if customProposal.Operation.Data["why"] == "" {
		t.Fatalf("expected custom why field to be populated")
	}
}

func TestResolveBlocksUnsupportedCases(t *testing.T) {
	cases := []struct {
		name    string
		input   ResolveInput
		profile Profile
		want    string
	}{
		{
			name:    "missing mapping",
			input:   ResolveInput{RunID: "run-1", ReviewItemID: "review-1", SourceCandidateID: "candidate-1", SafeTitle: "Note", SafeContext: "Context", Intent: IntentOpenTension},
			profile: loadProfileFixture(t, "default-governance.json"),
			want:    "missing_intent_mapping",
		},
		{
			name:  "ambiguous mapping",
			input: ResolveInput{RunID: "run-1", ReviewItemID: "review-1", SourceCandidateID: "candidate-1", SafeTitle: "Note", SafeContext: "Context", Intent: IntentDurableDecision},
			profile: func() Profile {
				profile := loadProfileFixture(t, "default-governance.json")
				profile.IntentMappings = append(profile.IntentMappings, profile.IntentMappings[0])
				return profile
			}(),
			want: "ambiguous_intent_mapping",
		},
		{
			name:    "missing required field",
			input:   ResolveInput{RunID: "run-1", ReviewItemID: "review-1", SourceCandidateID: "candidate-1", SafeTitle: "Note", Intent: IntentDurableDecision},
			profile: loadProfileFixture(t, "default-governance.json"),
			want:    "missing_required_field",
		},
		{
			name:  "platform only collection",
			input: ResolveInput{RunID: "run-1", ReviewItemID: "review-1", SourceCandidateID: "candidate-1", SafeTitle: "Note", SafeContext: "Context", Intent: IntentDurableDecision},
			profile: func() Profile {
				profile := loadProfileFixture(t, "default-governance.json")
				profile.Collections[0].PlatformOnly = true
				return profile
			}(),
			want: "platform_only_collection",
		},
		{
			name:  "unsupported kernel contract",
			input: ResolveInput{RunID: "run-1", ReviewItemID: "review-1", SourceCandidateID: "candidate-1", SafeTitle: "Note", SafeContext: "Context", Intent: IntentDurableDecision},
			profile: func() Profile {
				profile := loadProfileFixture(t, "default-governance.json")
				profile.KernelContract.SupportsExternalRef = false
				return profile
			}(),
			want: "unsupported_kernel_contract",
		},
		{
			name:    "no write",
			input:   ResolveInput{RunID: "run-1", ReviewItemID: "review-1", SourceCandidateID: "candidate-1", SafeTitle: "Note", SafeContext: "Context", Intent: IntentNoProductBrainWrite},
			profile: loadProfileFixture(t, "default-governance.json"),
			want:    "no_product_brain_write",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.input, tc.profile)
			if got.Status != ProposalStatusBlocked && got.Status != ProposalStatusSkipped {
				t.Fatalf("status = %s, want blocked/skipped", got.Status)
			}
			if len(got.Blockers) == 0 || got.Blockers[0].Code != tc.want {
				t.Fatalf("blockers = %+v, want first code %q", got.Blockers, tc.want)
			}
		})
	}
}

func TestResolveBlocksUnsupportedFieldRole(t *testing.T) {
	profile := loadProfileFixture(t, "default-governance.json")
	profile.IntentMappings[0].FieldMap["unsupported_role"] = "rationale"

	got := Resolve(ResolveInput{
		RunID:             "run-1",
		ReviewItemID:      "review-1",
		SourceCandidateID: "candidate-1",
		SafeTitle:         "Choose workspace profile bridge",
		SafeContext:       "Decision: use Product Brain workspace profiles before live writes.",
		Intent:            IntentDurableDecision,
	}, profile)
	if got.Status != ProposalStatusBlocked {
		t.Fatalf("status = %s, want blocked", got.Status)
	}
	if len(got.Blockers) == 0 || got.Blockers[0].Code != "unsupported_field_role" {
		t.Fatalf("blockers = %+v, want unsupported_field_role", got.Blockers)
	}
}
