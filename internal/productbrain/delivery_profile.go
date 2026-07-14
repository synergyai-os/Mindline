package productbrain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const DeliveryProfileSchema = "productbrain-delivery-profile/v0.1"

type DeliveryProfile struct {
	SchemaVersion    string                    `json:"schema_version"`
	ProfileID        string                    `json:"profile_id"`
	Workspace        DeliveryWorkspace         `json:"workspace"`
	Transport        DeliveryTransportProfile  `json:"transport"`
	Credential       DeliveryCredentialProfile `json:"credential"`
	RoleMappings     map[string]RoleMapping    `json:"role_mappings"`
	RelationMappings map[string]string         `json:"relation_mappings"`
	DraftOnly        bool                      `json:"draft_only"`
	ReviewPolicy     *DeliveryReviewPolicy     `json:"review_policy,omitempty"`
}

type DeliveryWorkspace struct {
	ExpectedID   string `json:"expected_id"`
	ExpectedSlug string `json:"expected_slug"`
}
type DeliveryTransportProfile struct {
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	APIPath string `json:"api_path"`
}
type DeliveryCredentialProfile struct {
	Provider      string `json:"provider"`
	Name          string `json:"name"`
	ExpectedKeyID string `json:"expected_key_id"`
}
type RoleMapping struct {
	CollectionSlug string `json:"collection_slug"`
	IDPrefix       string `json:"id_prefix"`
}

type DeliveryReviewPolicy struct {
	CredentialLifecycle     string `json:"credential_lifecycle"`
	PrivateRuntimeLifecycle string `json:"private_runtime_lifecycle"`
}

func LoadDeliveryProfile(path string) (DeliveryProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DeliveryProfile{}, err
	}
	if err := rejectDeliveryProfileSecretMaterial(data); err != nil {
		return DeliveryProfile{}, err
	}
	var profile DeliveryProfile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return DeliveryProfile{}, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return DeliveryProfile{}, err
	}
	if err := ValidateDeliveryProfile(profile); err != nil {
		return DeliveryProfile{}, err
	}
	return profile, nil
}

func rejectDeliveryProfileSecretMaterial(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	if profileNodeContainsSecret(raw, "") {
		return errors.New("Product Brain delivery profile contains forbidden secret material")
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("delivery profile contains trailing JSON")
		}
		return err
	}
	return nil
}

func profileNodeContainsSecret(value any, parent string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
			if parent == "credential" && normalized != "provider" && normalized != "name" && normalized != "expected_key_id" {
				return true
			}
			if normalized != "expected_key_id" && (strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") || strings.Contains(normalized, "token") || normalized == "api_key" || normalized == "apikey" || normalized == "access_key" || normalized == "private_key" || normalized == "authorization") {
				return true
			}
			if profileNodeContainsSecret(child, normalized) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if profileNodeContainsSecret(child, parent) {
				return true
			}
		}
	case string:
		lower := strings.ToLower(strings.TrimSpace(typed))
		for _, marker := range []string{"pb_sk_", "xoxb-", "xoxp-", "xoxa-", "xoxr-", "xapp-", "bearer "} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	return false
}

func ValidateDeliveryProfile(p DeliveryProfile) error {
	if p.SchemaVersion != DeliveryProfileSchema || strings.TrimSpace(p.ProfileID) == "" {
		return errors.New("invalid Product Brain delivery profile")
	}
	if p.Workspace.ExpectedID == "" || p.Workspace.ExpectedSlug == "" || p.Credential.ExpectedKeyID == "" {
		return errors.New("incomplete Product Brain delivery identity")
	}
	if p.Transport.Kind != "aki" || p.Transport.APIPath != "/api/aki" {
		return errors.New("unsupported Product Brain transport profile")
	}
	if p.Credential.Provider != "environment" || p.Credential.Name != "MINDLINE_PRODUCT_BRAIN_API_KEY" {
		return errors.New("unsupported Product Brain credential provider")
	}
	if !p.DraftOnly {
		return errors.New("Product Brain delivery profile must be draft-only")
	}
	if p.ReviewPolicy != nil {
		if p.ReviewPolicy.CredentialLifecycle != "persistent" && p.ReviewPolicy.CredentialLifecycle != "retire_after_review" {
			return errors.New("unsupported Product Brain credential review lifecycle")
		}
		if p.ReviewPolicy.PrivateRuntimeLifecycle != "retain" && p.ReviewPolicy.PrivateRuntimeLifecycle != "cleanup_after_review" {
			return errors.New("unsupported Product Brain private runtime review lifecycle")
		}
	}
	allowedRoles := map[string]bool{"external_entity": true, "evidence_backed_finding": true, "unresolved_tension": true}
	if len(p.RoleMappings) != len(allowedRoles) {
		return errors.New("unsupported Product Brain role mapping")
	}
	for role, mapping := range p.RoleMappings {
		if !allowedRoles[role] || mapping.CollectionSlug == "" || mapping.IDPrefix == "" {
			return fmt.Errorf("invalid role mapping")
		}
	}
	if len(p.RelationMappings) != 1 || p.RelationMappings["related_to"] != "related_to" {
		return errors.New("unsupported Product Brain relation mapping")
	}
	return nil
}

// DeliveryProfileFromSnapshot reconstructs the runtime profile only after the
// enclosing outbox has passed ValidateOutbox. That boundary permits empty
// transport fields solely for the exact immutable delivered v0.1 fingerprint.
func DeliveryProfileFromSnapshot(snapshot DeliveryProfileSnapshot) DeliveryProfile {
	kind, apiPath := snapshot.TransportKind, snapshot.TransportAPIPath
	if kind == "" {
		kind = "aki"
	}
	if apiPath == "" {
		apiPath = "/api/aki"
	}
	return DeliveryProfile{
		SchemaVersion: DeliveryProfileSchema,
		ProfileID:     snapshot.ProfileID,
		Workspace: DeliveryWorkspace{
			ExpectedID:   snapshot.ExpectedWorkspaceID,
			ExpectedSlug: snapshot.ExpectedWorkspaceSlug,
		},
		Transport: DeliveryTransportProfile{
			Kind:    kind,
			BaseURL: snapshot.ExpectedOrigin,
			APIPath: apiPath,
		},
		Credential: DeliveryCredentialProfile{
			Provider:      "environment",
			Name:          "MINDLINE_PRODUCT_BRAIN_API_KEY",
			ExpectedKeyID: snapshot.ExpectedKeyID,
		},
		RoleMappings:     cloneRoleMappings(snapshot.RoleMappings),
		RelationMappings: cloneStringMap(snapshot.RelationMappings),
		DraftOnly:        snapshot.DraftOnly,
		ReviewPolicy:     cloneReviewPolicy(snapshot.ReviewPolicy),
	}
}
