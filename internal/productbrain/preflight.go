package productbrain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const PreflightSchema = "productbrain-preflight/v0.1"

type PreflightGate struct {
	Name    string `json:"name"`
	Verdict string `json:"verdict"`
	Actual  string `json:"actual,omitempty"`
}
type PreflightArtifact struct {
	SchemaVersion       string                  `json:"schema_version"`
	Fingerprint         string                  `json:"fingerprint"`
	OutboxFingerprint   string                  `json:"outbox_fingerprint"`
	ProfileFingerprint  string                  `json:"profile_fingerprint"`
	ExpectedOrigin      string                  `json:"expected_origin"`
	Workspace           WorkspaceCapability     `json:"workspace"`
	CollectionContracts []CollectionContractRef `json:"collection_contracts"`
	Gates               []PreflightGate         `json:"gates"`
	MutationCalls       int                     `json:"mutation_calls"`
	Verdict             string                  `json:"verdict"`
}

type CollectionContractRef struct {
	Slug        string `json:"slug"`
	Fingerprint string `json:"fingerprint"`
}

func BuildPreflight(ctx context.Context, outbox Outbox, profile DeliveryProfile, transport ProductBrainTransport) (PreflightArtifact, error) {
	if err := ValidateOutbox(outbox); err != nil {
		return PreflightArtifact{}, err
	}
	if err := ValidateDeliveryProfile(profile); err != nil || outbox.ProfileFingerprint != hashValue(profile) {
		return PreflightArtifact{}, errors.New("outbox_state_mismatch")
	}
	if transport == nil {
		return PreflightArtifact{}, errors.New("missing Product Brain transport")
	}
	secretScanner, ok := transport.(RuntimeSecretScanner)
	if !ok {
		return PreflightArtifact{}, errors.New("capability_missing")
	}
	artifact := PreflightArtifact{SchemaVersion: PreflightSchema, OutboxFingerprint: outbox.Fingerprint, ProfileFingerprint: outbox.ProfileFingerprint, ExpectedOrigin: profile.Transport.BaseURL, MutationCalls: 0, Verdict: "pass"}
	checks := []struct {
		name, actual string
		pass         bool
	}{{"trusted_origin", profile.Transport.BaseURL, profile.Transport.BaseURL == ProductionGatewayOrigin}, {"runtime_secret_scan", "zero findings", len(secretScanner.RuntimeSecretFindings(outbox)) == 0}}
	capability, resolveErr := transport.ResolveWorkspace(ctx)
	if resolveErr != nil {
		return PreflightArtifact{}, resolveErr
	}
	artifact.Workspace = capability
	checks = append(checks, struct {
		name, actual string
		pass         bool
	}{"workspace_id", capability.ID, capability.ID == profile.Workspace.ExpectedID}, struct {
		name, actual string
		pass         bool
	}{"workspace_slug", capability.Slug, capability.Slug == profile.Workspace.ExpectedSlug}, struct {
		name, actual string
		pass         bool
	}{"governance_mode", capability.GovernanceMode, capability.GovernanceMode == "open" || capability.GovernanceMode == "consensus" || capability.GovernanceMode == "role"}, struct {
		name, actual string
		pass         bool
	}{"key_scope", capability.KeyScope, capability.KeyScope == "readwrite"}, struct {
		name, actual string
		pass         bool
	}{"key_id", capability.KeyID, capability.KeyID == profile.Credential.ExpectedKeyID})
	for _, slug := range outboxCollectionSlugs(outbox) {
		contract, contractErr := transport.GetCollectionFields(ctx, slug)
		if contractErr != nil {
			return PreflightArtifact{}, contractErr
		}
		contractErr = validateOutboxCollectionContract(outbox, slug, contract)
		contractFingerprint := hashValue(contract)
		artifact.CollectionContracts = append(artifact.CollectionContracts, CollectionContractRef{Slug: slug, Fingerprint: contractFingerprint})
		checks = append(checks, struct {
			name, actual string
			pass         bool
		}{"collection_contract:" + slug, contractFingerprint, contractErr == nil})
	}
	for _, check := range checks {
		verdict := "pass"
		if !check.pass {
			verdict = "fail"
			artifact.Verdict = "fail"
		}
		artifact.Gates = append(artifact.Gates, PreflightGate{Name: check.name, Verdict: verdict, Actual: check.actual})
	}
	artifact.Fingerprint = hashValue(artifact)
	if artifact.Verdict != "pass" {
		return artifact, errors.New("workspace_mismatch")
	}
	return artifact, nil
}
func WritePreflight(outDir string, artifact PreflightArtifact) error {
	if err := privateio.PrepareDir(outDir); err != nil {
		return err
	}
	return privateio.WriteJSON(filepath.Join(outDir, "preflight.json"), artifact)
}
func LoadPreflight(path string) (PreflightArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PreflightArtifact{}, err
	}
	return DecodePreflight(data)
}

// DecodePreflight applies the same strict structural contract at runtime and
// proof boundaries. Binding to a particular outbox/profile is checked by
// ValidatePreflight after decoding.
func DecodePreflight(data []byte) (PreflightArtifact, error) {
	var artifact PreflightArtifact
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&artifact); err != nil {
		return PreflightArtifact{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return PreflightArtifact{}, errors.New("invalid trailing preflight data")
	}
	if artifact.SchemaVersion != PreflightSchema || artifact.Fingerprint != hashValue(artifact) || artifact.Verdict != "pass" || artifact.MutationCalls != 0 || validatePreflightGateSet(artifact) != nil {
		return PreflightArtifact{}, errors.New("invalid preflight artifact")
	}
	for _, gate := range artifact.Gates {
		if gate.Verdict != "pass" {
			return PreflightArtifact{}, errors.New("failed preflight artifact")
		}
	}
	return artifact, nil
}
func ValidatePreflight(artifact PreflightArtifact, outbox Outbox, profile DeliveryProfile) error {
	if err := ValidateOutbox(outbox); err != nil {
		return err
	}
	if err := ValidateDeliveryProfile(profile); err != nil || artifact.SchemaVersion != PreflightSchema || artifact.Fingerprint != hashValue(artifact) || validatePreflightGateSet(artifact) != nil {
		return errors.New("invalid preflight artifact")
	}
	if artifact.OutboxFingerprint != outbox.Fingerprint || artifact.ProfileFingerprint != hashValue(profile) || artifact.ExpectedOrigin != profile.Transport.BaseURL || artifact.Workspace.ID != profile.Workspace.ExpectedID || artifact.Workspace.Slug != profile.Workspace.ExpectedSlug || artifact.Workspace.KeyID != profile.Credential.ExpectedKeyID || artifact.Workspace.KeyScope != "readwrite" || !validGovernanceMode(artifact.Workspace.GovernanceMode) {
		return errors.New("outbox_state_mismatch")
	}
	if artifact.MutationCalls != 0 || artifact.Verdict != "pass" {
		return errors.New("capability_missing")
	}
	expectedSlugs := outboxCollectionSlugs(outbox)
	if len(artifact.CollectionContracts) != len(expectedSlugs) {
		return errors.New("collection_contract_mismatch")
	}
	for index, slug := range expectedSlugs {
		if artifact.CollectionContracts[index].Slug != slug || artifact.CollectionContracts[index].Fingerprint == "" {
			return errors.New("collection_contract_mismatch")
		}
	}
	return nil
}

func validateLiveCollectionContracts(ctx context.Context, transport ProductBrainTransport, outbox Outbox, artifact PreflightArtifact) error {
	expected := map[string]string{}
	for _, ref := range artifact.CollectionContracts {
		expected[ref.Slug] = ref.Fingerprint
	}
	for _, slug := range outboxCollectionSlugs(outbox) {
		contract, err := transport.GetCollectionFields(ctx, slug)
		if err != nil {
			return err
		}
		if err := validateOutboxCollectionContract(outbox, slug, contract); err != nil || expected[slug] == "" || expected[slug] != hashValue(contract) {
			return errors.New("collection_contract_mismatch")
		}
	}
	return nil
}

func outboxCollectionSlugs(outbox Outbox) []string {
	seen := map[string]bool{}
	var slugs []string
	for _, operation := range outbox.Operations {
		if operation.Entry != nil && !seen[operation.Entry.CollectionSlug] {
			seen[operation.Entry.CollectionSlug] = true
			slugs = append(slugs, operation.Entry.CollectionSlug)
		}
	}
	sort.Strings(slugs)
	return slugs
}

func validateOutboxCollectionContract(outbox Outbox, slug string, contract CollectionCapability) error {
	if !contract.Found || contract.Slug != slug {
		return errors.New("collection_contract_mismatch")
	}
	fields := map[string]CollectionFieldCapability{}
	for _, field := range contract.Fields {
		if strings.TrimSpace(field.Key) == "" || fields[field.Key].Key != "" || !supportedCollectionFieldType(field.Type) || !validCollectionFieldOptions(field) {
			return errors.New("collection_contract_mismatch")
		}
		fields[field.Key] = field
	}
	for _, operation := range outbox.Operations {
		if operation.Entry == nil || operation.Entry.CollectionSlug != slug {
			continue
		}
		for key, value := range operation.Entry.Data {
			field, ok := fields[key]
			if !ok || !collectionFieldValueValid(field, value) {
				return errors.New("collection_contract_mismatch")
			}
		}
		for _, field := range contract.Fields {
			if !field.Required {
				continue
			}
			value, ok := operation.Entry.Data[field.Key]
			if !ok || !collectionFieldValueValid(field, value) || (field.Type == "string" || field.Type == "text") && strings.TrimSpace(fmt.Sprint(value)) == "" {
				return errors.New("collection_contract_mismatch")
			}
		}
	}
	return nil
}

func supportedCollectionFieldType(fieldType string) bool {
	switch fieldType {
	case "select", "string", "text", "date", "number", "boolean", "person":
		return true
	default:
		return false
	}
}

func validCollectionFieldOptions(field CollectionFieldCapability) bool {
	if field.Type != "select" {
		return len(field.Options) == 0
	}
	if len(field.Options) == 0 {
		return false
	}
	for index, option := range field.Options {
		if strings.TrimSpace(option) == "" || index > 0 && field.Options[index-1] >= option {
			return false
		}
	}
	return true
}

func validatePreflightGateSet(artifact PreflightArtifact) error {
	if artifact.ExpectedOrigin != ProductionGatewayOrigin || artifact.Workspace.ID == "" || artifact.Workspace.Slug == "" || artifact.Workspace.KeyID == "" || artifact.Workspace.KeyScope != "readwrite" || !validGovernanceMode(artifact.Workspace.GovernanceMode) {
		return errors.New("invalid preflight authority values")
	}
	expected := map[string]string{
		"trusted_origin":      artifact.ExpectedOrigin,
		"runtime_secret_scan": "zero findings",
		"workspace_id":        artifact.Workspace.ID,
		"workspace_slug":      artifact.Workspace.Slug,
		"governance_mode":     artifact.Workspace.GovernanceMode,
		"key_scope":           artifact.Workspace.KeyScope,
		"key_id":              artifact.Workspace.KeyID,
	}
	seenContracts := map[string]bool{}
	lastSlug := ""
	for _, contract := range artifact.CollectionContracts {
		if contract.Slug == "" || contract.Fingerprint == "" || seenContracts[contract.Slug] || lastSlug >= contract.Slug && lastSlug != "" {
			return errors.New("invalid collection contract refs")
		}
		seenContracts[contract.Slug] = true
		lastSlug = contract.Slug
		expected["collection_contract:"+contract.Slug] = contract.Fingerprint
	}
	if len(seenContracts) == 0 || len(artifact.Gates) != len(expected) {
		return errors.New("invalid preflight gates")
	}
	seen := map[string]bool{}
	for _, gate := range artifact.Gates {
		actual, exists := expected[gate.Name]
		if !exists || seen[gate.Name] || gate.Verdict != "pass" || gate.Actual != actual {
			return errors.New("invalid preflight gates")
		}
		seen[gate.Name] = true
	}
	return nil
}

func validGovernanceMode(value string) bool {
	return value == "open" || value == "consensus" || value == "role"
}

func collectionFieldValueValid(field CollectionFieldCapability, value any) bool {
	switch field.Type {
	case "select":
		text, ok := value.(string)
		if !ok {
			return false
		}
		for _, option := range field.Options {
			if text == option {
				return true
			}
		}
		return false
	case "string", "text", "date", "person":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		if ok {
			return true
		}
		_, ok = value.(int)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	default:
		return false
	}
}

var _ = fmt.Sprintf
var _ = strings.TrimSpace
