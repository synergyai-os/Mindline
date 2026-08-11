package agentstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
)

// DurableFingerprint identifies all non-rebuildable legacy, scoped, and
// project-connection state.
// It is read-only and treats missing snapshots as canonical empty state.
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
	scoped, scopedPresent, err := readScopedRecoverySnapshot(filepath.Clean(databasePath))
	if err != nil {
		return "", err
	}
	if !scopedPresent {
		scoped = scopedRecoverySnapshot{
			SchemaVersion: scopedRecoverySchemaVersion,
			Scopes:        []Scope{}, Lenses: []ScopedLens{}, Actors: []AgentActor{},
			Runs: []ScopedRetrievalTrace{}, Judgments: []ScopedJudgment{},
		}
	}
	projectConnections, connectionsPresent, err := readProjectConnectionRecoverySnapshot(filepath.Clean(databasePath))
	if err != nil {
		return "", err
	}
	if !connectionsPresent {
		projectConnections = projectConnectionRecoverySnapshot{
			SchemaVersion: ProjectConnectionSchemaVersion,
			Connections:   []ProjectConnection{},
		}
	}
	data, err := json.Marshal(struct {
		Legacy             recoverySnapshot                  `json:"legacy"`
		Scoped             scopedRecoverySnapshot            `json:"scoped"`
		ProjectConnections projectConnectionRecoverySnapshot `json:"project_connections"`
	}{Legacy: snapshot, Scoped: scoped, ProjectConnections: projectConnections})
	if err != nil {
		return "", errors.New("fingerprint durable agent state")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
