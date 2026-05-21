package productbrain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const SupportedProfileSchemaVersion = "productbrain-workspace-profile/v0.1"

type Intent string

const (
	IntentDurableDecision     Intent = "durable_decision"
	IntentOperatingStandard   Intent = "operating_standard"
	IntentOpenTension         Intent = "open_tension"
	IntentImplementationWork  Intent = "implementation_work"
	IntentReusableInsight     Intent = "reusable_insight"
	IntentReferenceNote       Intent = "reference_note"
	IntentNoProductBrainWrite Intent = "no_product_brain_write"
)

type Profile struct {
	SchemaVersion  string          `json:"schema_version"`
	Workspace      Workspace       `json:"workspace"`
	KernelContract KernelContract  `json:"kernel_contract"`
	Collections    []Collection    `json:"collections"`
	IntentMappings []IntentMapping `json:"intent_mappings"`
}

type Workspace struct {
	ExternalID string `json:"external_id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
}

type KernelContract struct {
	SupportsWriteEntry          bool `json:"supports_write_entry"`
	SupportsUpsertByExternalRef bool `json:"supports_upsert_by_external_ref"`
	SupportsExternalRef         bool `json:"supports_external_ref"`
	SupportsIdempotencyKey      bool `json:"supports_idempotency_key"`
	SupportsActorAuthority      bool `json:"supports_actor_authority"`
	SupportsProvenance          bool `json:"supports_provenance"`
}

type Collection struct {
	Slug                  string   `json:"slug"`
	Name                  string   `json:"name"`
	Purpose               string   `json:"purpose"`
	Governed              bool     `json:"governed"`
	PlatformOnly          bool     `json:"platform_only"`
	ValidWorkflowStatuses []string `json:"valid_workflow_statuses"`
	DefaultWorkflowStatus string   `json:"default_workflow_status"`
	ClassificationSignals []string `json:"classification_signals"`
	UsageGuidance         string   `json:"usage_guidance"`
	QualityCriteria       []string `json:"quality_criteria"`
	Fields                []Field  `json:"fields"`
}

type Field struct {
	Key             string `json:"key"`
	Label           string `json:"label"`
	Type            string `json:"type"`
	Required        bool   `json:"required"`
	SemanticRole    string `json:"semantic_role"`
	WritingGuidance string `json:"writing_guidance"`
}

type IntentMapping struct {
	Intent         Intent            `json:"intent"`
	CollectionSlug string            `json:"collection_slug"`
	FieldMap       map[string]string `json:"field_map"`
}

func ParseProfile(data []byte) (Profile, error) {
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, err
	}
	if err := ValidateProfile(profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func ValidateProfile(profile Profile) error {
	if profile.SchemaVersion != SupportedProfileSchemaVersion {
		return fmt.Errorf("unsupported profile schema: %s", profile.SchemaVersion)
	}
	if strings.TrimSpace(profile.Workspace.ExternalID) == "" || strings.TrimSpace(profile.Workspace.Slug) == "" {
		return fmt.Errorf("workspace identity is required")
	}
	if len(profile.Collections) == 0 {
		return fmt.Errorf("profile must define at least one collection")
	}
	collections := map[string]Collection{}
	for _, collection := range profile.Collections {
		if strings.TrimSpace(collection.Slug) == "" {
			return fmt.Errorf("collection slug is required")
		}
		if len(collection.Fields) == 0 {
			return fmt.Errorf("collection %q must define fields", collection.Slug)
		}
		if _, exists := collections[collection.Slug]; exists {
			return fmt.Errorf("duplicate collection: %s", collection.Slug)
		}
		collections[collection.Slug] = collection
	}
	mappings := map[Intent]bool{}
	for _, mapping := range profile.IntentMappings {
		if strings.TrimSpace(string(mapping.Intent)) == "" {
			return fmt.Errorf("intent mapping intent is required")
		}
		if _, exists := mappings[mapping.Intent]; exists {
			return fmt.Errorf("duplicate intent mapping: %s", mapping.Intent)
		}
		mappings[mapping.Intent] = true
		collection, ok := collections[mapping.CollectionSlug]
		if !ok {
			return fmt.Errorf("intent %q maps to unknown collection %q", mapping.Intent, mapping.CollectionSlug)
		}
		fields := map[string]bool{}
		for _, field := range collection.Fields {
			fields[field.Key] = true
		}
		for role, fieldKey := range mapping.FieldMap {
			if !fields[fieldKey] {
				return fmt.Errorf("intent %q field role %q maps to unknown field %q", mapping.Intent, role, fieldKey)
			}
		}
	}
	return nil
}

func ProfileFingerprint(profile Profile) string {
	data, err := json.Marshal(profile)
	if err != nil {
		sum := sha256.Sum256([]byte(profile.Workspace.Slug))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
