package assurance

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	ProofAttemptSchema = "mindline-proof-attempt/v1"
	NamespaceSchema    = "mindline-proof-namespace/v1"
)

var fullGitCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type ProofAttemptState struct {
	SchemaVersion                  string `json:"schema_version"`
	AttemptID                      string `json:"attempt_id"`
	AttemptGeneration              string `json:"attempt_generation"`
	FrozenCommit                   string `json:"frozen_commit"`
	ManifestSHA256                 string `json:"manifest_sha256"`
	ProofRoot                      string `json:"proof_root"`
	NamespaceMarkerSHA256          string `json:"namespace_marker_sha256"`
	State                          string `json:"state"`
	UpdatedAt                      string `json:"updated_at"`
	FinalEvidenceIndexSHA256       string `json:"final_evidence_index_sha256,omitempty"`
	ImmutableRevalidationSetSHA256 string `json:"immutable_revalidation_set_sha256,omitempty"`
}

type ProofAttempt struct {
	ControlRoot string
	Root        string
	StatePath   string
	LedgerPath  string
	State       ProofAttemptState
}

type AttemptOptions struct {
	ControlRoot string
	Port        int
	Random      io.Reader
	Now         func() time.Time
}

type namespaceMarker struct {
	SchemaVersion     string `json:"schema_version"`
	AttemptID         string `json:"attempt_id"`
	AttemptGeneration string `json:"attempt_generation"`
	FrozenCommit      string `json:"frozen_commit"`
	ManifestSHA256    string `json:"manifest_sha256"`
}

func DefaultControlRoot() (string, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configRoot, "Mindline", "control-plane"), nil
}

// BeginWP46ProofAttempt exclusively probes the fixed address before it makes
// durable assurance mutations, then publishes a fresh no-replace namespace.
func BeginWP46ProofAttempt(frozenCommit, manifestSHA256 string, options AttemptOptions) (ProofAttempt, error) {
	if !fullGitCommitPattern.MatchString(frozenCommit) {
		return ProofAttempt{}, errors.New("proof attempt requires a full lowercase Git commit")
	}
	if !validHexSHA256(manifestSHA256) {
		return ProofAttempt{}, errors.New("proof attempt requires a manifest SHA-256")
	}
	controlRoot := options.ControlRoot
	if controlRoot == "" {
		var err error
		controlRoot, err = DefaultControlRoot()
		if err != nil {
			return ProofAttempt{}, err
		}
	}
	if !filepath.IsAbs(controlRoot) {
		return ProofAttempt{}, errors.New("control root must be absolute")
	}
	port := options.Port
	if port == 0 {
		port = 9876
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return ProofAttempt{}, fmt.Errorf("fixed proof port is not exclusively available: %w", err)
	}
	defer listener.Close()

	random := options.Random
	if random == nil {
		random = rand.Reader
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	attemptID, err := randomBase32(random, 16)
	if err != nil {
		return ProofAttempt{}, err
	}
	generation, err := randomBase32(random, 32)
	if err != nil {
		return ProofAttempt{}, err
	}

	assuranceRoot := filepath.Join(controlRoot, "assurance")
	commitRoot := filepath.Join(assuranceRoot, "proof", "WP-46", frozenCommit)
	attemptsRoot := filepath.Join(commitRoot, "attempts")
	for _, directory := range []string{controlRoot, assuranceRoot, filepath.Join(assuranceRoot, "proof"), filepath.Join(assuranceRoot, "proof", "WP-46"), commitRoot, attemptsRoot} {
		if err := ensurePrivateDirectory(directory); err != nil {
			return ProofAttempt{}, err
		}
	}
	proofRoot := filepath.Join(attemptsRoot, attemptID+"."+generation)
	if err := os.Mkdir(proofRoot, 0o700); err != nil {
		return ProofAttempt{}, fmt.Errorf("create no-replace proof attempt: %w", err)
	}
	marker := namespaceMarker{NamespaceSchema, attemptID, generation, frozenCommit, manifestSHA256}
	markerBytes, _ := json.Marshal(marker)
	markerBytes = append(markerBytes, '\n')
	markerPath := filepath.Join(proofRoot, "namespace-marker.json")
	if err := writeNoReplaceSynced(markerPath, markerBytes, 0o600); err != nil {
		return ProofAttempt{}, err
	}
	if err := syncDirectory(proofRoot); err != nil {
		return ProofAttempt{}, err
	}
	markerHash := sha256.Sum256(markerBytes)
	state := ProofAttemptState{
		SchemaVersion: ProofAttemptSchema, AttemptID: attemptID, AttemptGeneration: generation,
		FrozenCommit: frozenCommit, ManifestSHA256: manifestSHA256, ProofRoot: proofRoot,
		NamespaceMarkerSHA256: hex.EncodeToString(markerHash[:]), State: "preparing",
		UpdatedAt: now().UTC().Format(time.RFC3339Nano),
	}
	statePath := filepath.Join(assuranceRoot, "current-proof-attempt.json")
	if err := writeJSONAtomicSynced(statePath, state, 0o600); err != nil {
		return ProofAttempt{}, err
	}
	ledgerPath := filepath.Join(proofRoot, "attempt-ledger.jsonl")
	if err := writeNoReplaceSynced(ledgerPath, nil, 0o600); err != nil {
		return ProofAttempt{}, err
	}
	return ProofAttempt{ControlRoot: controlRoot, Root: proofRoot, StatePath: statePath, LedgerPath: ledgerPath, State: state}, nil
}

func TransitionProofAttempt(attempt *ProofAttempt, next, finalIndexSHA256 string, now time.Time) error {
	if attempt == nil || attempt.StatePath == "" {
		return errors.New("proof attempt is unavailable")
	}
	allowed := map[string]map[string]bool{
		"preparing":         {"checkpoint_sealed": true, "failed": true},
		"checkpoint_sealed": {"receipt_minted": true, "failed": true},
		"receipt_minted":    {"live_proof": true, "closing": true, "failed": true},
		"live_proof":        {"closing": true, "failed": true},
		"closing":           {"succeeded": true, "failed": true},
	}
	if !allowed[attempt.State.State][next] {
		return fmt.Errorf("invalid proof attempt transition %s -> %s", attempt.State.State, next)
	}
	if next == "succeeded" && !validHexSHA256(finalIndexSHA256) {
		return errors.New("success requires the durable final index SHA-256")
	}
	attempt.State.State = next
	attempt.State.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	attempt.State.FinalEvidenceIndexSHA256 = finalIndexSHA256
	if err := writeJSONAtomicSynced(attempt.StatePath, attempt.State, 0o600); err != nil {
		return err
	}
	return nil
}

func ensurePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("private directory is unsafe: %s", path)
	}
	return nil
}

func randomBase32(reader io.Reader, bytesCount int) (string, error) {
	buffer := make([]byte, bytesCount)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buffer)), nil
}

func validHexSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func writeJSONAtomicSynced(path string, value any, mode os.FileMode) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"."+fmt.Sprint(time.Now().UnixNano())+".tmp")
	if err := writeNoReplaceSynced(temporary, data, mode); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func writeNoReplaceSynced(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil {
		return err
	}
	return closeErr
}
