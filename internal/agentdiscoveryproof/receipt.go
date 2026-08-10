package agentdiscoveryproof

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const (
	SchemaVersion          = "mindline-agent-discovery-proof/v0.1"
	SignedSpecSHA256       = "009c98ca9dd975bd0472fe7127ff79fb8533493601ea593b601b527631726d0d"
	SignedPlanSHA256       = "7b7b481ede61596929a78e9ac16170aef6365ccbcd0ed50992d0eb1b7bde147a"
	RequiredBaselineCommit = "723b7b319627a4fd4f508e0745bfd002fa2d0398"
)

type Receipt struct {
	SchemaVersion         string              `json:"schema_version"`
	TreeSHA256            string              `json:"tree_sha256"`
	BinarySHA256          string              `json:"binary_sha256"`
	SpecSHA256            string              `json:"spec_sha256"`
	PlanSHA256            string              `json:"plan_sha256"`
	LatencyManifestSHA256 string              `json:"latency_manifest_sha256"`
	BaselineCommit        string              `json:"baseline_commit"`
	Cases                 []Case              `json:"cases"`
	PrivateCommitments    []PrivateCommitment `json:"private_commitments"`
}

type Case struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Count     int    `json:"count"`
	ColdMS    int    `json:"cold_ms"`
	P50MS     int    `json:"p50_ms"`
	P95MS     int    `json:"p95_ms"`
	MaximumMS int    `json:"maximum_ms"`
}

type PrivateCommitment struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type ExpectedArtifacts struct {
	TreeSHA256            string
	BinarySHA256          string
	LatencyManifestSHA256 string
}

func CommitPrivate(key []byte, kind, value string) (PrivateCommitment, error) {
	kind = strings.TrimSpace(kind)
	if len(key) != 32 || kind == "" || strings.TrimSpace(value) == "" {
		return PrivateCommitment{}, errors.New("invalid private commitment input")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("mindline-proof-private-v1\x00" + kind + "\x00" + value))
	return PrivateCommitment{Kind: kind, Value: "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil))}, nil
}

func Decode(data []byte, expected ExpectedArtifacts) (Receipt, error) {
	var receipt Receipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, errors.New("invalid discovery proof receipt")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Receipt{}, errors.New("discovery proof receipt has trailing data")
	}
	if err := receipt.Validate(expected); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func (receipt Receipt) Validate(expected ExpectedArtifacts) error {
	if !isSHA(expected.TreeSHA256) || !isSHA(expected.BinarySHA256) ||
		!isSHA(expected.LatencyManifestSHA256) ||
		receipt.SchemaVersion != SchemaVersion || receipt.TreeSHA256 != expected.TreeSHA256 ||
		receipt.BinarySHA256 != expected.BinarySHA256 || receipt.SpecSHA256 != SignedSpecSHA256 ||
		receipt.PlanSHA256 != SignedPlanSHA256 || !isSHA(receipt.LatencyManifestSHA256) ||
		receipt.LatencyManifestSHA256 != expected.LatencyManifestSHA256 ||
		receipt.BaselineCommit != RequiredBaselineCommit ||
		len(receipt.Cases) != len(requiredCases) ||
		len(receipt.PrivateCommitments) != len(requiredCommitmentKinds) {
		return errors.New("invalid discovery proof receipt")
	}
	seenCases := map[string]bool{}
	for _, item := range receipt.Cases {
		ceiling, required := requiredCases[item.ID]
		if !required || item.State != "pass" || seenCases[item.ID] || item.Count <= 0 ||
			item.ColdMS < 0 || item.P50MS < 0 || item.P95MS < item.P50MS ||
			item.MaximumMS < item.P95MS {
			return errors.New("invalid discovery proof case")
		}
		if ceiling > 0 && (item.Count != 21 || item.P95MS > ceiling) {
			return errors.New("invalid discovery proof latency case")
		}
		seenCases[item.ID] = true
	}
	if len(seenCases) != len(requiredCases) {
		return errors.New("incomplete discovery proof cases")
	}
	seenCommitments := map[string]bool{}
	for _, commitment := range receipt.PrivateCommitments {
		if !allowedCommitmentKind(commitment.Kind) || seenCommitments[commitment.Kind] ||
			!strings.HasPrefix(commitment.Value, "hmac-sha256:") ||
			!isSHA(strings.TrimPrefix(commitment.Value, "hmac-sha256:")) {
			return errors.New("invalid private proof commitment")
		}
		seenCommitments[commitment.Kind] = true
	}
	if len(seenCommitments) != len(requiredCommitmentKinds) {
		return errors.New("incomplete private proof commitments")
	}
	return nil
}

var requiredCases = map[string]int{
	"help": 0, "discovery": 0, "config_separation": 0,
	"scoped_get": 0, "hydration_negatives": 0, "feedback_lifecycle": 0,
	"abstention_diagnostics": 0, "closed_errors": 0, "compatibility": 0,
	"install_rollback": 0, "secret_scan": 0, "blind_answerable": 0, "blind_absent": 0,
	"latency_agent_help": 250, "latency_feedback_token": 250,
	"latency_agent_status":    3000,
	"latency_discovery_ready": 3000, "latency_discovery_invalid": 3000,
	"latency_scoped_get": 5000, "latency_typed_scoped_get_error": 5000,
	"latency_scoped_feedback": 5000, "latency_typed_feedback_error": 5000,
	"latency_feedback_reverse": 5000,
	"latency_scoped_search":    25000,
}

var requiredCommitmentKinds = map[string]bool{
	"answerable_question": true, "absent_question": true, "scope": true,
	"lens": true, "agent": true, "private_transcript": true,
	"config": true, "library": true, "machine": true,
}

func allowedCommitmentKind(value string) bool {
	return requiredCommitmentKinds[value]
}

func isSHA(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
