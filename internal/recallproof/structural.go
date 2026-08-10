package recallproof

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"

	"github.com/synergyai-os/Mindline/internal/assurance"
)

// StructuralArtifact is the only exported proof shape. It has no free-form
// text, paths, source identity, URL, query, or raw error field.
type StructuralArtifact struct {
	SchemaVersion string            `json:"schema_version"`
	Build         string            `json:"build"`
	State         string            `json:"state"`
	Counts        map[string]int    `json:"counts"`
	Fingerprints  map[string]string `json:"fingerprints"`
	Tests         map[string]bool   `json:"tests"`
}

var structuralKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func DecodeStructuralArtifact(data []byte) (StructuralArtifact, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var artifact StructuralArtifact
	if err := decoder.Decode(&artifact); err != nil {
		return StructuralArtifact{}, fmt.Errorf("strict structural artifact decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return StructuralArtifact{}, errors.New("structural artifact has trailing JSON")
	}
	if err := artifact.Validate(); err != nil {
		return StructuralArtifact{}, err
	}
	return artifact, nil
}

func (artifact StructuralArtifact) Validate() error {
	if artifact.SchemaVersion == "" || !structuralKey.MatchString(artifact.Build) || !structuralKey.MatchString(artifact.State) {
		return errors.New("structural artifact has invalid fixed fields")
	}
	if len(artifact.Counts) == 0 || len(artifact.Fingerprints) == 0 || len(artifact.Tests) == 0 {
		return errors.New("structural artifact lacks required proof fields")
	}
	for key, count := range artifact.Counts {
		if !structuralKey.MatchString(key) || count < 0 {
			return errors.New("structural artifact contains invalid count")
		}
	}
	for key, fingerprint := range artifact.Fingerprints {
		if !structuralKey.MatchString(key) || !fingerprintPattern.MatchString(fingerprint) {
			return errors.New("structural artifact contains invalid fingerprint")
		}
	}
	for key, passed := range artifact.Tests {
		if !structuralKey.MatchString(key) {
			return errors.New("structural artifact contains invalid test name")
		}
		if artifact.State == "pass" && !passed {
			return errors.New("passing structural artifact contains a failed test")
		}
	}
	return nil
}

// SignedStructuralReceipt is validated by the owner-controlled verifier. The
// payload stays structural; a verifier can use a platform signing mechanism
// without exposing its signing key to this package.
type SignedStructuralReceipt struct {
	SchemaVersion       string            `json:"schema_version"`
	Kind                string            `json:"kind"`
	Binding             TreeConfigBinding `json:"binding"`
	ArtifactFingerprint string            `json:"artifact_fingerprint"`
	SignerFingerprint   string            `json:"signer_fingerprint"`
	Signature           string            `json:"signature"`
}

func (receipt SignedStructuralReceipt) Validate() error {
	if receipt.SchemaVersion != "mindline-signed-structural-receipt/v0.1" || !structuralKey.MatchString(receipt.Kind) || receipt.Signature == "" || !fingerprintPattern.MatchString(receipt.ArtifactFingerprint) || !fingerprintPattern.MatchString(receipt.SignerFingerprint) {
		return errors.New("invalid signed structural receipt")
	}
	return receipt.Binding.Validate()
}

type ReceiptVerifier interface {
	VerifyStructuralReceipt(SignedStructuralReceipt) error
}

const PhaseReceiptSchema = "mindline-recall-phase-receipt/v0.1"

// PhaseReceipt is a fail-closed structural handoff. Only phases with a typed,
// exact artifact contract are accepted; later proof phases remain unavailable
// until their producers and schemas exist.
type PhaseReceipt struct {
	SchemaVersion string             `json:"schema_version"`
	Phase         string             `json:"phase"`
	Binding       TreeConfigBinding  `json:"binding"`
	Artifact      StructuralArtifact `json:"artifact"`
}

func (receipt PhaseReceipt) Validate() error {
	if receipt.SchemaVersion != PhaseReceiptSchema {
		return errors.New("unsupported phase receipt schema")
	}
	if receipt.Phase != "pre_live" {
		return errors.New("unsupported proof phase")
	}
	if err := receipt.Binding.Validate(); err != nil {
		return err
	}
	if err := receipt.Artifact.Validate(); err != nil {
		return err
	}
	if receipt.Artifact.State != "pass" {
		return errors.New("phase receipt is not passing")
	}
	return validatePreLiveArtifact(receipt.Artifact)
}

func validatePreLiveArtifact(artifact StructuralArtifact) error {
	if artifact.SchemaVersion != "mindline-reusable-proof/v0.1" ||
		artifact.Build != "wp48" || artifact.State != "pass" ||
		len(artifact.Counts) != 1 {
		return errors.New("pre-live artifact does not match the reusable proof contract")
	}
	manifest, err := assurance.ParseWP48Manifest(assurance.EmbeddedWP48Manifest())
	if err != nil {
		return errors.New("pre-live proof manifest is unavailable")
	}
	expected := make(map[string]struct{})
	for _, group := range manifest.Groups {
		if group.Phase == "pre_live" {
			expected[group.ID] = struct{}{}
		}
	}
	if artifact.Counts["executed_pre_live_groups"] != len(expected) ||
		len(artifact.Tests) != len(expected) || len(artifact.Fingerprints) != len(expected) {
		return errors.New("pre-live artifact does not cover the exact proof manifest")
	}
	for id := range expected {
		if !artifact.Tests[id] || artifact.Fingerprints[id] == "" {
			return errors.New("pre-live artifact omits a required proof group")
		}
	}
	for id := range artifact.Tests {
		if _, exists := expected[id]; !exists {
			return errors.New("pre-live artifact contains an unapproved proof group")
		}
	}
	for id := range artifact.Fingerprints {
		if _, exists := expected[id]; !exists {
			return errors.New("pre-live artifact contains an unapproved proof fingerprint")
		}
	}
	return nil
}

// DecisionArtifact creates a stable public-safe result after phase validation.
func (receipt PhaseReceipt) DecisionArtifact() (StructuralArtifact, error) {
	if err := receipt.Validate(); err != nil {
		return StructuralArtifact{}, err
	}
	fingerprint, err := DeterministicArtifactFingerprint(receipt.Artifact)
	if err != nil {
		return StructuralArtifact{}, err
	}
	return StructuralArtifact{SchemaVersion: "mindline-recall-proof-command/v0.1", Build: "wp48", State: "pass", Counts: map[string]int{"phases_validated": 1}, Fingerprints: map[string]string{"artifact": fingerprint, "tree": receipt.Binding.TreeFingerprint, "config": receipt.Binding.LiveConfigurationFingerprint}, Tests: map[string]bool{"phase_receipt_valid": true}}, nil
}

type LifecycleRunner struct{ Verifier ReceiptVerifier }

var requiredLifecycleReceipts = []string{"envelope", "adoption", "conflict", "replay", "restart", "revision", "capacity", "resources", "api", "rollback", "privacy"}

// Verify checks signed receipts rather than caller-populated lifecycle flags
// and returns a deterministic commitment-only decision receipt.
func (runner LifecycleRunner) Verify(binding TreeConfigBinding, receipts []SignedStructuralReceipt) (StructuralArtifact, error) {
	if runner.Verifier == nil {
		return StructuralArtifact{}, errors.New("lifecycle receipt verifier is required")
	}
	if err := binding.Validate(); err != nil {
		return StructuralArtifact{}, err
	}
	seen := map[string]struct{}{}
	fingerprints := map[string]string{}
	for _, receipt := range receipts {
		if err := receipt.Validate(); err != nil {
			return StructuralArtifact{}, err
		}
		if receipt.Binding != binding {
			return StructuralArtifact{}, errors.New("receipt binding differs from lifecycle binding")
		}
		if _, exists := seen[receipt.Kind]; exists {
			return StructuralArtifact{}, errors.New("duplicate lifecycle receipt kind")
		}
		if err := runner.Verifier.VerifyStructuralReceipt(receipt); err != nil {
			return StructuralArtifact{}, err
		}
		seen[receipt.Kind] = struct{}{}
		fingerprints[receipt.Kind] = receipt.ArtifactFingerprint
	}
	for _, kind := range requiredLifecycleReceipts {
		if _, exists := seen[kind]; !exists {
			return StructuralArtifact{}, fmt.Errorf("missing required lifecycle receipt: %s", kind)
		}
	}
	return StructuralArtifact{SchemaVersion: "mindline-recall-proof-decision/v0.1", Build: "wp48", State: "pass", Counts: map[string]int{"signed_receipts": len(receipts)}, Fingerprints: fingerprints, Tests: map[string]bool{"all_required_receipts_verified": true}}, nil
}

const FounderReviewReceiptSchema = "mindline-founder-review-receipt/v0.1"

// FounderReviewReceipt carries only a bounded outcome and a commitment to the
// owner-only review record; cited evidence cannot enter this receipt.
type FounderReviewReceipt struct {
	SchemaVersion     string            `json:"schema_version"`
	Binding           TreeConfigBinding `json:"binding"`
	Outcome           string            `json:"outcome"`
	ReviewFingerprint string            `json:"review_fingerprint"`
}

func (receipt FounderReviewReceipt) Validate() error {
	if receipt.SchemaVersion != FounderReviewReceiptSchema || !fingerprintPattern.MatchString(receipt.ReviewFingerprint) {
		return errors.New("invalid founder review receipt")
	}
	if err := receipt.Binding.Validate(); err != nil {
		return err
	}
	switch receipt.Outcome {
	case "useful", "not_useful", "declined":
		return nil
	default:
		return errors.New("invalid founder outcome")
	}
}

// FounderReviewPort keeps taste judgment outside raw evidence. Only useful
// closes the user-value outcome; other values remain explicit non-closure.
type FounderReviewPort interface {
	FounderReviewReceipt() (FounderReviewReceipt, error)
}

func RequireUsefulFounderOutcome(port FounderReviewPort, binding TreeConfigBinding) error {
	if port == nil {
		return errors.New("founder review port is required")
	}
	receipt, err := port.FounderReviewReceipt()
	if err != nil {
		return err
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	if receipt.Binding != binding {
		return errors.New("founder review receipt binding differs from final binding")
	}
	if receipt.Outcome != "useful" {
		return errors.New("founder outcome does not close user value")
	}
	return nil
}

func DeterministicArtifactFingerprint(artifact StructuralArtifact) (string, error) {
	if err := artifact.Validate(); err != nil {
		return "", err
	}
	canonical := artifact
	canonical.Counts, canonical.Fingerprints, canonical.Tests = sortedCounts(artifact.Counts), sortedFingerprints(artifact.Fingerprints), sortedTests(artifact.Tests)
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func sortedCounts(values map[string]int) map[string]int {
	keys := keysOf(values)
	result := make(map[string]int, len(values))
	for _, key := range keys {
		result[key] = values[key]
	}
	return result
}
func sortedFingerprints(values map[string]string) map[string]string {
	keys := keysOf(values)
	result := make(map[string]string, len(values))
	for _, key := range keys {
		result[key] = values[key]
	}
	return result
}
func sortedTests(values map[string]bool) map[string]bool {
	keys := keysOf(values)
	result := make(map[string]bool, len(values))
	for _, key := range keys {
		result[key] = values[key]
	}
	return result
}
func keysOf[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
