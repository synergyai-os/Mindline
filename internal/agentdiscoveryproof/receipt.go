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

const SchemaVersion = "mindline-agent-discovery-proof/v0.1"

type Receipt struct {
	SchemaVersion      string              `json:"schema_version"`
	TreeSHA256         string              `json:"tree_sha256"`
	BinarySHA256       string              `json:"binary_sha256"`
	SpecSHA256         string              `json:"spec_sha256"`
	PlanSHA256         string              `json:"plan_sha256"`
	Cases              []Case              `json:"cases"`
	PrivateCommitments []PrivateCommitment `json:"private_commitments"`
}

type Case struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Count     int    `json:"count"`
	P50MS     int    `json:"p50_ms"`
	P95MS     int    `json:"p95_ms"`
	MaximumMS int    `json:"maximum_ms"`
}

type PrivateCommitment struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
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

func Decode(data []byte) (Receipt, error) {
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
	if err := receipt.Validate(); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func (receipt Receipt) Validate() error {
	if receipt.SchemaVersion != SchemaVersion || !isSHA(receipt.TreeSHA256) ||
		!isSHA(receipt.BinarySHA256) || !isSHA(receipt.SpecSHA256) || !isSHA(receipt.PlanSHA256) ||
		len(receipt.Cases) == 0 || len(receipt.PrivateCommitments) == 0 {
		return errors.New("invalid discovery proof receipt")
	}
	seenCases := map[string]bool{}
	for _, item := range receipt.Cases {
		if !allowedCase(item.ID) || item.State != "pass" || seenCases[item.ID] ||
			item.Count < 0 || item.P50MS < 0 || item.P95MS < item.P50MS ||
			item.MaximumMS < item.P95MS {
			return errors.New("invalid discovery proof case")
		}
		seenCases[item.ID] = true
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
	return nil
}

func allowedCase(value string) bool {
	switch value {
	case "help", "discovery", "scoped_get", "feedback", "blind_answerable",
		"blind_absent", "rollback", "secret_scan", "latency":
		return true
	default:
		return false
	}
}

func allowedCommitmentKind(value string) bool {
	switch value {
	case "answerable_question", "absent_question", "scope", "lens", "agent", "private_transcript":
		return true
	default:
		return false
	}
}

func isSHA(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
