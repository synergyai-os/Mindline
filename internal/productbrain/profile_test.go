package productbrain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileValidationAcceptsDefaultAndCustomFixtures(t *testing.T) {
	for _, name := range []string{"default-governance.json", "custom-workspace.json"} {
		t.Run(name, func(t *testing.T) {
			profile := loadProfileFixture(t, name)
			if err := ValidateProfile(profile); err != nil {
				t.Fatalf("ValidateProfile() error = %v", err)
			}
			if got := ProfileFingerprint(profile); !strings.HasPrefix(got, "sha256:") {
				t.Fatalf("fingerprint = %q, want sha256 prefix", got)
			}
		})
	}
}

func TestProfileValidationRejectsInvalidShape(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Profile)
		want string
	}{
		{
			name: "unsupported schema",
			edit: func(profile *Profile) { profile.SchemaVersion = "productbrain-workspace-profile/v9" },
			want: "unsupported profile schema",
		},
		{
			name: "missing workspace identity",
			edit: func(profile *Profile) { profile.Workspace.Slug = "" },
			want: "workspace identity",
		},
		{
			name: "no collections",
			edit: func(profile *Profile) { profile.Collections = nil },
			want: "at least one collection",
		},
		{
			name: "unknown mapped collection",
			edit: func(profile *Profile) { profile.IntentMappings[0].CollectionSlug = "missing" },
			want: "unknown collection",
		},
		{
			name: "unknown mapped field",
			edit: func(profile *Profile) { profile.IntentMappings[0].FieldMap["rationale"] = "missing" },
			want: "unknown field",
		},
		{
			name: "duplicate intent mapping",
			edit: func(profile *Profile) {
				profile.IntentMappings = append(profile.IntentMappings, profile.IntentMappings[0])
			},
			want: "duplicate intent mapping",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile := validProfileForTest()
			tc.edit(&profile)
			err := ValidateProfile(profile)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateProfile() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestPlatformOnlyCollectionIsAvailableForResolverRefusal(t *testing.T) {
	profile := validProfileForTest()
	profile.Collections[0].PlatformOnly = true
	if err := ValidateProfile(profile); err != nil {
		t.Fatalf("platform-only is a resolver constraint, ValidateProfile() error = %v", err)
	}
}

func loadProfileFixture(t *testing.T, name string) Profile {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "productbrain", "profiles", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile fixture: %v", err)
	}
	profile, err := ParseProfile(data)
	if err != nil {
		t.Fatalf("ParseProfile() error = %v", err)
	}
	return profile
}

func validProfileForTest() Profile {
	return Profile{
		SchemaVersion: SupportedProfileSchemaVersion,
		Workspace: Workspace{
			ExternalID: "workspace-default",
			Slug:       "default-governance",
			Name:       "Default Governance Workspace",
		},
		KernelContract: KernelContract{
			SupportsWriteEntry:          true,
			SupportsUpsertByExternalRef: true,
			SupportsExternalRef:         true,
			SupportsIdempotencyKey:      true,
			SupportsActorAuthority:      true,
			SupportsProvenance:          true,
		},
		Collections: []Collection{
			{
				Slug:                  "decisions",
				Name:                  "Decisions",
				Purpose:               "Significant decisions with rationale and context",
				ValidWorkflowStatuses: []string{"pending", "active"},
				DefaultWorkflowStatus: "pending",
				Fields: []Field{
					{Key: "rationale", Label: "Rationale", Type: "text", Required: true, SemanticRole: "rationale"},
				},
			},
		},
		IntentMappings: []IntentMapping{
			{Intent: IntentDurableDecision, CollectionSlug: "decisions", FieldMap: map[string]string{"rationale": "rationale"}},
		},
	}
}
