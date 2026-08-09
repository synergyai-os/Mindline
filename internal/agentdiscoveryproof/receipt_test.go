package agentdiscoveryproof

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrivateCommitmentIsKeyedAndReceiptRejectsPlainHashAndUnknownFields(t *testing.T) {
	key := make([]byte, 32)
	for index := range key {
		key[index] = byte(index + 1)
	}
	commitment, err := CommitPrivate(key, "answerable_question", "a guessable question")
	if err != nil || !strings.HasPrefix(commitment.Value, "hmac-sha256:") {
		t.Fatalf("commitment=%+v err=%v", commitment, err)
	}
	plain := sha256.Sum256([]byte("a guessable question"))
	if strings.TrimPrefix(commitment.Value, "hmac-sha256:") == hex.EncodeToString(plain[:]) {
		t.Fatal("private commitment was an unkeyed deterministic hash")
	}
	receipt := validReceipt(t, key)
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err != nil {
		t.Fatal(err)
	}
	receipt.PrivateCommitments[0].Value = "sha256:" + hex.EncodeToString(plain[:])
	data, _ = json.Marshal(receipt)
	if _, err := Decode(data); err == nil {
		t.Fatal("plain private hash was accepted")
	}
	data = append(data[:len(data)-1], []byte(`,"query":"private"}`)...)
	if _, err := Decode(data); err == nil {
		t.Fatal("unknown private field was accepted")
	}
}

func TestReceiptRejectsIncompleteVacuousAndOverCeilingProof(t *testing.T) {
	key := make([]byte, 32)
	for index := range key {
		key[index] = byte(index + 1)
	}
	tests := []struct {
		name   string
		mutate func(*Receipt)
	}{
		{name: "missing case", mutate: func(value *Receipt) { value.Cases = value.Cases[1:] }},
		{name: "zero count", mutate: func(value *Receipt) { value.Cases[0].Count = 0 }},
		{name: "latency over ceiling", mutate: func(value *Receipt) {
			for index := range value.Cases {
				if value.Cases[index].ID == "latency_agent_help" {
					value.Cases[index].P95MS, value.Cases[index].MaximumMS = 251, 251
				}
			}
		}},
		{name: "missing binding", mutate: func(value *Receipt) { value.PrivateCommitments = value.PrivateCommitments[1:] }},
		{name: "wrong spec", mutate: func(value *Receipt) { value.SpecSHA256 = strings.Repeat("b", 64) }},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			receipt := validReceipt(t, key)
			item.mutate(&receipt)
			data, _ := json.Marshal(receipt)
			if _, err := Decode(data); err == nil {
				t.Fatal("invalid proof receipt was accepted")
			}
		})
	}
}

func validReceipt(t *testing.T, key []byte) Receipt {
	t.Helper()
	sha := strings.Repeat("a", 64)
	receipt := Receipt{SchemaVersion: SchemaVersion, TreeSHA256: sha, BinarySHA256: sha,
		SpecSHA256: SignedSpecSHA256, PlanSHA256: SignedPlanSHA256,
		LatencyManifestSHA256: sha, BaselineCommit: RequiredBaselineCommit}
	for id, ceiling := range requiredCases {
		item := Case{ID: id, State: "pass", Count: 1}
		if ceiling > 0 {
			item.Count, item.ColdMS, item.P50MS, item.P95MS, item.MaximumMS = 21, 1, 1, 2, 3
		}
		receipt.Cases = append(receipt.Cases, item)
	}
	for kind := range requiredCommitmentKinds {
		commitment, err := CommitPrivate(key, kind, "private-"+kind)
		if err != nil {
			t.Fatal(err)
		}
		receipt.PrivateCommitments = append(receipt.PrivateCommitments, commitment)
	}
	return receipt
}
