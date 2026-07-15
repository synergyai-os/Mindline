package controlui

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

type HumanAction struct {
	Kind               string
	PayloadFingerprint string
	SessionFingerprint string
	GestureRecordedAt  string
	NonceFingerprint   string
	seal               [32]byte
}

type HumanAuthority struct {
	mu       sync.Mutex
	key      [32]byte
	consumed map[string]bool
}

func NewHumanAuthority() (*HumanAuthority, error) {
	authority := &HumanAuthority{consumed: map[string]bool{}}
	if _, err := rand.Read(authority.key[:]); err != nil {
		return nil, err
	}
	return authority, nil
}

func FingerprintPayload(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (authority *HumanAuthority) sealAction(kind string, payload any, session string, now time.Time) (HumanAction, error) {
	if authority == nil || kind == "" || session == "" {
		return HumanAction{}, errors.New("human action authority unavailable")
	}
	nonce, err := randomToken(32)
	if err != nil {
		return HumanAction{}, err
	}
	sessionHash := sha256.Sum256([]byte(session))
	action := HumanAction{
		Kind: kind, PayloadFingerprint: FingerprintPayload(payload), SessionFingerprint: hex.EncodeToString(sessionHash[:]),
		GestureRecordedAt: now.UTC().Format(time.RFC3339Nano), NonceFingerprint: FingerprintPayload(nonce),
	}
	action.seal = authority.actionMAC(action)
	return action, nil
}

func (authority *HumanAuthority) VerifyAndConsumeAction(action HumanAction) bool {
	if authority == nil || action.Kind == "" || action.PayloadFingerprint == "" || action.SessionFingerprint == "" || action.NonceFingerprint == "" {
		return false
	}
	expected := authority.actionMAC(action)
	if subtle.ConstantTimeCompare(expected[:], action.seal[:]) != 1 {
		return false
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.consumed[action.NonceFingerprint] {
		return false
	}
	authority.consumed[action.NonceFingerprint] = true
	return true
}

func (authority *HumanAuthority) actionMAC(action HumanAction) [32]byte {
	mac := hmac.New(sha256.New, authority.key[:])
	_, _ = mac.Write([]byte(action.Kind))
	_, _ = mac.Write([]byte(action.PayloadFingerprint))
	_, _ = mac.Write([]byte(action.SessionFingerprint))
	_, _ = mac.Write([]byte(action.GestureRecordedAt))
	_, _ = mac.Write([]byte(action.NonceFingerprint))
	var result [32]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func (authority *HumanAuthority) sealApproval(preview BatchPreview, session string, now time.Time) HumanApproval {
	gesture := now.UTC().Format(time.RFC3339Nano)
	previewBytes, _ := json.Marshal(preview)
	sessionHash := sha256.Sum256([]byte(session))
	mac := hmac.New(sha256.New, authority.key[:])
	_, _ = mac.Write(previewBytes)
	_, _ = mac.Write(sessionHash[:])
	_, _ = mac.Write([]byte(gesture))
	var seal [32]byte
	copy(seal[:], mac.Sum(nil))
	return HumanApproval{Preview: preview, InitiationFingerprint: hex.EncodeToString(seal[:]), SessionFingerprint: hex.EncodeToString(sessionHash[:]), GestureRecordedAt: gesture, seal: seal}
}

func (authority *HumanAuthority) VerifyAndConsumeApproval(approval HumanApproval) bool {
	if authority == nil {
		return false
	}
	previewBytes, _ := json.Marshal(approval.Preview)
	mac := hmac.New(sha256.New, authority.key[:])
	_, _ = mac.Write(previewBytes)
	sessionHash, err := hex.DecodeString(approval.SessionFingerprint)
	if err != nil || len(sessionHash) != sha256.Size {
		return false
	}
	_, _ = mac.Write(sessionHash)
	_, _ = mac.Write([]byte(approval.GestureRecordedAt))
	expected := mac.Sum(nil)
	if subtle.ConstantTimeCompare(expected, approval.seal[:]) != 1 || approval.InitiationFingerprint != hex.EncodeToString(approval.seal[:]) {
		return false
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.consumed[approval.InitiationFingerprint] {
		return false
	}
	authority.consumed[approval.InitiationFingerprint] = true
	return true
}
