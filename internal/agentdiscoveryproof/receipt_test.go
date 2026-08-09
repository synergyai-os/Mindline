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
	sha := strings.Repeat("a", 64)
	receipt := Receipt{SchemaVersion: SchemaVersion, TreeSHA256: sha, BinarySHA256: sha,
		SpecSHA256: sha, PlanSHA256: sha,
		Cases:              []Case{{ID: "help", State: "pass", Count: 1, P50MS: 1, P95MS: 2, MaximumMS: 3}},
		PrivateCommitments: []PrivateCommitment{commitment}}
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
