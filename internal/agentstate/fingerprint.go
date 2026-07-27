package agentstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
)

// DurableFingerprint identifies only non-derived agent state: user-created
// lenses and append-only judgments. It is read-only and treats a missing
// snapshot as the canonical empty durable state.
func DurableFingerprint(databasePath string) (string, error) {
	if !filepath.IsAbs(databasePath) {
		return "", errors.New("agent state path must be absolute")
	}
	snapshot, present, err := readRecoverySnapshot(filepath.Clean(databasePath))
	if err != nil {
		return "", err
	}
	if !present {
		snapshot = recoverySnapshot{
			SchemaVersion: recoverySchemaVersion,
			Lenses:        []Lens{},
			Judgments:     []Judgment{},
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", errors.New("fingerprint durable agent state")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
