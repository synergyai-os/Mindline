package agentstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	recoverySchemaVersion       = "mindline-agent-recovery/v0.1"
	recoveryMarkerSchemaVersion = "mindline-agent-recovery-marker/v0.2"
	maximumRecoveryBytes        = 128 << 20
	maximumRecoveryMarkerBytes  = 64 << 10
)

type recoverySnapshot struct {
	SchemaVersion string     `json:"schema_version"`
	Lenses        []Lens     `json:"lenses"`
	Judgments     []Judgment `json:"judgments"`
}

type recoveryMarker struct {
	SchemaVersion              string `json:"schema_version"`
	QuarantineBase             string `json:"quarantine_base"`
	StagePath                  string `json:"stage_path"`
	SnapshotPresent            bool   `json:"snapshot_present"`
	ProjectConnectionsAdopted  bool   `json:"project_connections_adopted"`
	ProjectSnapshotPresent     bool   `json:"project_snapshot_present"`
	ProjectSnapshotFingerprint string `json:"project_snapshot_fingerprint"`
}

type recoveryHooks struct {
	rename        func(string, string) error
	beforeRestore func() error
}

func recoveryPath(databasePath string) string {
	return databasePath + ".recovery.json"
}

func recoveryMarkerPath(databasePath string) string {
	return databasePath + ".recovery-in-progress.json"
}

func recoveryMarkerExists(databasePath string) bool {
	_, err := os.Lstat(recoveryMarkerPath(databasePath))
	return err == nil || !os.IsNotExist(err)
}

func readRecoverySnapshot(databasePath string) (recoverySnapshot, bool, error) {
	path := recoveryPath(databasePath)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return recoverySnapshot{}, false, nil
	} else if err != nil {
		return recoverySnapshot{}, false, errors.New("read agent recovery snapshot")
	}
	var snapshot recoverySnapshot
	if err := privateio.ReadJSONStrictBounded(
		filepath.Dir(path), path, maximumRecoveryBytes, &snapshot,
	); err != nil || validateRecoverySnapshot(snapshot) != nil {
		return recoverySnapshot{}, false, errors.New("read agent recovery snapshot")
	}
	return snapshot, true, nil
}

func validateRecoverySnapshot(snapshot recoverySnapshot) error {
	if snapshot.SchemaVersion != recoverySchemaVersion {
		return errors.New("invalid agent recovery snapshot")
	}
	lensIDs := make(map[string]bool, len(snapshot.Lenses))
	for _, lens := range snapshot.Lenses {
		if !validBounded(lens.ID, 256) || !validBounded(lens.Name, 1024) ||
			!validBounded(lens.Query, maximumTextRunes) ||
			!validBounded(lens.CreatedAt, 256) || !validBounded(lens.UpdatedAt, 256) ||
			containsSecretLikeAny(lens.ID, lens.Name, lens.Query, lens.CreatedAt, lens.UpdatedAt) ||
			lensIDs[lens.ID] {
			return errors.New("invalid agent recovery snapshot")
		}
		lensIDs[lens.ID] = true
	}
	judgmentIDs := make(map[string]bool, len(snapshot.Judgments))
	idempotencyKeys := make(map[string]bool, len(snapshot.Judgments))
	originals := make(map[string]Judgment, len(snapshot.Judgments))
	for _, judgment := range snapshot.Judgments {
		if judgment.JudgmentID != stableID("judgment", judgment.IdempotencyKey) ||
			judgment.Replayed || judgmentIDs[judgment.JudgmentID] ||
			idempotencyKeys[judgment.IdempotencyKey] {
			return errors.New("invalid agent recovery snapshot")
		}
		judgmentIDs[judgment.JudgmentID] = true
		idempotencyKeys[judgment.IdempotencyKey] = true
		if judgment.ReversesID == "" {
			if err := validateRecoveredOriginal(judgment); err != nil {
				return err
			}
			originals[judgment.JudgmentID] = judgment
		}
	}
	reversed := make(map[string]bool, len(snapshot.Judgments))
	for _, judgment := range snapshot.Judgments {
		if judgment.ReversesID == "" {
			continue
		}
		original, exists := originals[judgment.ReversesID]
		if !exists || reversed[judgment.ReversesID] ||
			validateRecoveredReversal(judgment, original) != nil {
			return errors.New("invalid agent recovery snapshot")
		}
		reversed[judgment.ReversesID] = true
	}
	return nil
}

func (store *Store) buildRecoverySnapshot(ctx context.Context) (recoverySnapshot, error) {
	snapshot := recoverySnapshot{
		SchemaVersion: recoverySchemaVersion,
		Lenses:        []Lens{},
		Judgments:     []Judgment{},
	}
	lenses, err := store.ListLenses(ctx)
	if err != nil {
		return recoverySnapshot{}, errors.New("build agent recovery snapshot")
	}
	snapshot.Lenses = lenses
	rows, err := store.db.QueryContext(ctx, `SELECT judgment_id, idempotency_key, run_id,
		lens_id, record_id, actor, disposition, reason, reverses_judgment_id, effect, created_at
		FROM judgments ORDER BY created_at, judgment_id`)
	if err != nil {
		return recoverySnapshot{}, errors.New("build agent recovery snapshot")
	}
	for rows.Next() {
		judgment, scanErr := scanJudgment(rows)
		if scanErr != nil {
			rows.Close()
			return recoverySnapshot{}, errors.New("build agent recovery snapshot")
		}
		snapshot.Judgments = append(snapshot.Judgments, judgment)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return recoverySnapshot{}, errors.New("build agent recovery snapshot")
	}
	if err := rows.Close(); err != nil {
		return recoverySnapshot{}, errors.New("build agent recovery snapshot")
	}
	if err := validateRecoverySnapshot(snapshot); err != nil {
		return recoverySnapshot{}, errors.New("build agent recovery snapshot")
	}
	return snapshot, nil
}

func (store *Store) writeRecoverySnapshot(ctx context.Context) error {
	snapshot, err := store.buildRecoverySnapshot(ctx)
	if err != nil {
		return err
	}
	if err := privateio.WriteJSON(recoveryPath(store.path), snapshot); err != nil {
		return errors.New("write agent recovery snapshot")
	}
	if err := store.writeScopedRecoverySnapshot(ctx); err != nil {
		return err
	}
	return nil
}

func OpenRecovering(path string, now Clock) (*Store, string, error) {
	return openRecovering(path, now, recoveryHooks{rename: os.Rename})
}

func openRecovering(path string, now Clock, hooks recoveryHooks) (*Store, string, error) {
	if !filepath.IsAbs(path) {
		return nil, "", errors.New("agent state path must be absolute")
	}
	path = filepath.Clean(path)
	if hooks.rename == nil {
		hooks.rename = os.Rename
	}
	store, err := Open(path, now)
	if err == nil {
		return store, "", nil
	}
	switch {
	case errors.Is(err, ErrRecoveryInProgress):
		marker, readErr := readRecoveryMarker(path)
		if readErr != nil {
			return nil, "", readErr
		}
		return resumeRecovery(path, marker, now, hooks)
	case errors.Is(err, ErrCorrupt):
		snapshot, present, snapshotErr := readRecoverySnapshot(path)
		if snapshotErr != nil {
			return nil, "", snapshotErr
		}
		scopedSnapshot, scopedPresent, scopedErr := readScopedRecoverySnapshot(path)
		if scopedErr != nil {
			return nil, "", scopedErr
		}
		projectSnapshot, projectPresent, projectErr := readProjectConnectionRecoverySnapshot(path)
		if projectErr != nil {
			return nil, "", projectErr
		}
		projectAdopted, adoptionErr := readProjectConnectionAdoptionMarker(path)
		if adoptionErr != nil {
			return nil, "", adoptionErr
		}
		if projectPresent && !projectAdopted {
			if err := ensureProjectConnectionAdoptionMarker(path); err != nil {
				return nil, "", err
			}
			projectAdopted = true
		}
		if projectAdopted && !projectPresent {
			return nil, "", errors.New("project connection recovery snapshot unavailable")
		}
		marker, markerErr := createRecoveryMarker(
			path, present, projectAdopted, projectSnapshot, projectPresent, now,
		)
		if markerErr != nil {
			return nil, "", markerErr
		}
		if present && validateRecoverySnapshot(snapshot) != nil {
			return nil, "", errors.New("start agent state recovery")
		}
		return resumeRecoveryWithSnapshot(
			path, marker, snapshot, present, scopedSnapshot, scopedPresent,
			projectSnapshot, projectPresent, now, hooks,
		)
	default:
		return nil, "", err
	}
}

func createRecoveryMarker(
	databasePath string,
	snapshotPresent bool,
	projectAdopted bool,
	projectSnapshot projectConnectionRecoverySnapshot,
	projectSnapshotPresent bool,
	now Clock,
) (recoveryMarker, error) {
	if now == nil {
		now = time.Now
	}
	projectFingerprint, err := projectConnectionSnapshotFingerprint(
		projectSnapshot, projectSnapshotPresent,
	)
	if err != nil {
		return recoveryMarker{}, err
	}
	timestamp := now().UTC().Format("20060102T150405.000000000Z")
	marker := recoveryMarker{
		SchemaVersion:              recoveryMarkerSchemaVersion,
		QuarantineBase:             databasePath + ".corrupt-" + timestamp,
		StagePath:                  databasePath + ".recovery-stage",
		SnapshotPresent:            snapshotPresent,
		ProjectConnectionsAdopted:  projectAdopted,
		ProjectSnapshotPresent:     projectSnapshotPresent,
		ProjectSnapshotFingerprint: projectFingerprint,
	}
	if err := validateRecoveryMarker(databasePath, marker); err != nil {
		return recoveryMarker{}, err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Lstat(marker.QuarantineBase + suffix); err == nil {
			return recoveryMarker{}, errors.New("start agent state recovery")
		} else if !os.IsNotExist(err) {
			return recoveryMarker{}, errors.New("start agent state recovery")
		}
	}
	if err := privateio.WriteJSONNoReplace(recoveryMarkerPath(databasePath), marker); err != nil {
		return recoveryMarker{}, errors.New("start agent state recovery")
	}
	return marker, nil
}

func readRecoveryMarker(databasePath string) (recoveryMarker, error) {
	var marker recoveryMarker
	path := recoveryMarkerPath(databasePath)
	if err := privateio.ReadJSONStrictBounded(
		filepath.Dir(path), path, maximumRecoveryMarkerBytes, &marker,
	); err != nil || validateRecoveryMarker(databasePath, marker) != nil {
		return recoveryMarker{}, errors.New("read agent recovery marker")
	}
	return marker, nil
}

func validateRecoveryMarker(databasePath string, marker recoveryMarker) error {
	databasePath = filepath.Clean(databasePath)
	root := filepath.Dir(databasePath)
	if marker.SchemaVersion != recoveryMarkerSchemaVersion ||
		marker.StagePath != databasePath+".recovery-stage" ||
		!strings.HasPrefix(marker.QuarantineBase, databasePath+".corrupt-") ||
		filepath.Dir(marker.QuarantineBase) != root ||
		marker.ProjectConnectionsAdopted != marker.ProjectSnapshotPresent ||
		!validProjectSnapshotFingerprint(marker.ProjectSnapshotFingerprint, marker.ProjectSnapshotPresent) {
		return errors.New("invalid agent recovery marker")
	}
	return privateio.ValidateContained(
		root, databasePath, marker.StagePath, marker.QuarantineBase,
		recoveryMarkerPath(databasePath), recoveryPath(databasePath),
		scopedRecoveryPath(databasePath),
		projectConnectionRecoveryPath(databasePath),
		projectConnectionAdoptionPath(databasePath),
	)
}

func resumeRecovery(
	databasePath string,
	marker recoveryMarker,
	now Clock,
	hooks recoveryHooks,
) (*Store, string, error) {
	snapshot, present, err := readRecoverySnapshot(databasePath)
	if err != nil {
		return nil, "", err
	}
	if marker.SnapshotPresent != present {
		return nil, "", errors.New("resume agent state recovery: recovery snapshot changed")
	}
	scopedSnapshot, scopedPresent, err := readScopedRecoverySnapshot(databasePath)
	if err != nil {
		return nil, "", err
	}
	projectSnapshot, projectPresent, err := readProjectConnectionRecoverySnapshot(databasePath)
	if err != nil {
		return nil, "", err
	}
	if err := verifyRecoveryMarkerProjectSnapshot(marker, projectSnapshot, projectPresent); err != nil {
		return nil, "", err
	}
	return resumeRecoveryWithSnapshot(
		databasePath, marker, snapshot, present, scopedSnapshot, scopedPresent,
		projectSnapshot, projectPresent, now, hooks,
	)
}

func projectConnectionSnapshotFingerprint(
	snapshot projectConnectionRecoverySnapshot, present bool,
) (string, error) {
	if !present {
		return "", nil
	}
	data, err := encodeProjectConnectionRecoverySnapshot(snapshot)
	if err != nil {
		return "", errors.New("fingerprint project connection recovery snapshot")
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validProjectSnapshotFingerprint(value string, present bool) bool {
	if !present {
		return value == ""
	}
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func verifyRecoveryMarkerProjectSnapshot(
	marker recoveryMarker,
	snapshot projectConnectionRecoverySnapshot,
	present bool,
) error {
	if marker.ProjectConnectionsAdopted {
		fingerprint, err := projectConnectionSnapshotFingerprint(snapshot, present)
		if err != nil || !present || fingerprint != marker.ProjectSnapshotFingerprint {
			return errors.New("resume agent state recovery: project connection recovery snapshot changed")
		}
		return nil
	}
	if !present {
		return nil
	}
	empty := projectConnectionRecoverySnapshot{
		SchemaVersion: ProjectConnectionSchemaVersion,
		Connections:   []ProjectConnection{},
	}
	if !reflect.DeepEqual(snapshot, empty) {
		return errors.New("resume agent state recovery: project connection recovery snapshot changed")
	}
	return nil
}

func resumeRecoveryWithSnapshot(
	databasePath string,
	marker recoveryMarker,
	snapshot recoverySnapshot,
	snapshotPresent bool,
	scopedSnapshot scopedRecoverySnapshot,
	scopedSnapshotPresent bool,
	projectSnapshot projectConnectionRecoverySnapshot,
	projectSnapshotPresent bool,
	now Clock,
	hooks recoveryHooks,
) (*Store, string, error) {
	if store, state, err := inspectRecoveryCanonical(
		databasePath, snapshot, snapshotPresent, scopedSnapshot, scopedSnapshotPresent,
		projectSnapshot, projectSnapshotPresent, now,
	); err != nil {
		return nil, "", err
	} else if state == "complete" {
		if err := finalizeRecovery(databasePath, store); err != nil {
			_ = store.Close()
			return nil, "", err
		}
		return store, marker.QuarantineBase, nil
	} else if state == "corrupt" || state == "missing" {
		if err := quarantineDatabase(databasePath, marker.QuarantineBase, hooks.rename); err != nil {
			return nil, "", err
		}
	}
	if err := removeRecoveryStage(marker.StagePath); err != nil {
		return nil, "", err
	}
	if err := removeScopedRecoveryStage(marker.StagePath); err != nil {
		return nil, "", err
	}
	if err := removeProjectConnectionRecoveryStage(marker.StagePath); err != nil {
		return nil, "", err
	}
	if err := removeProjectConnectionAdoptionStage(marker.StagePath); err != nil {
		return nil, "", err
	}
	if scopedSnapshotPresent {
		if err := privateio.WriteJSON(scopedRecoveryPath(marker.StagePath), scopedSnapshot); err != nil {
			return nil, "", errors.New("prepare scoped agent state recovery stage")
		}
	}
	if projectSnapshotPresent {
		if err := privateio.WriteJSON(projectConnectionRecoveryPath(marker.StagePath), projectSnapshot); err != nil {
			return nil, "", errors.New("prepare project connection recovery stage")
		}
	}
	stage, err := openStore(marker.StagePath, now, false)
	if err != nil {
		return nil, "", errors.New("prepare agent state recovery stage")
	}
	if hooks.beforeRestore != nil {
		if err := hooks.beforeRestore(); err != nil {
			_ = stage.Close()
			return nil, "", errors.New("restore agent state recovery stage")
		}
	}
	// Prove the scoped sidecar itself was restored before legacy ingress is
	// projected into the reserved root/legacy context below.
	if err := scopedRecoverySnapshotMatches(
		context.Background(), stage, scopedSnapshot, scopedSnapshotPresent,
	); err != nil {
		_ = stage.Close()
		return nil, "", err
	}
	if err := projectConnectionRecoverySnapshotMatches(
		context.Background(), stage, projectSnapshot, projectSnapshotPresent,
	); err != nil {
		_ = stage.Close()
		return nil, "", err
	}
	if snapshotPresent {
		if err := stage.restoreRecoverySnapshot(context.Background(), snapshot); err != nil {
			_ = stage.Close()
			return nil, "", err
		}
	}
	if !scopedSnapshotPresent {
		if err := stage.projectLegacyState(context.Background()); err != nil {
			_ = stage.Close()
			return nil, "", errors.New("project legacy state during recovery")
		}
	}
	scopedSnapshot, err = stage.buildScopedRecoverySnapshot(context.Background())
	if err != nil {
		_ = stage.Close()
		return nil, "", fmt.Errorf("reconcile scoped agent state recovery snapshot: %w", err)
	}
	scopedSnapshotPresent = true
	projectSnapshot, err = stage.buildProjectConnectionRecoverySnapshot(context.Background())
	if err != nil {
		_ = stage.Close()
		return nil, "", fmt.Errorf("reconcile project connection recovery snapshot: %w", err)
	}
	projectSnapshotPresent = true
	if err := recoverySnapshotMatches(context.Background(), stage, snapshot, snapshotPresent); err != nil {
		_ = stage.Close()
		return nil, "", err
	}
	if err := scopedRecoverySnapshotMatches(
		context.Background(), stage, scopedSnapshot, scopedSnapshotPresent,
	); err != nil {
		_ = stage.Close()
		return nil, "", err
	}
	if err := projectConnectionRecoverySnapshotMatches(
		context.Background(), stage, projectSnapshot, projectSnapshotPresent,
	); err != nil {
		_ = stage.Close()
		return nil, "", err
	}
	if err := stage.Close(); err != nil {
		return nil, "", errors.New("close agent state recovery stage")
	}
	if err := removeScopedRecoveryStage(marker.StagePath); err != nil {
		return nil, "", err
	}
	if err := removeProjectConnectionRecoveryStage(marker.StagePath); err != nil {
		return nil, "", err
	}
	if err := removeProjectConnectionAdoptionStage(marker.StagePath); err != nil {
		return nil, "", err
	}
	// Persist the reconciled projection before promotion so an interruption
	// after the database rename resumes against the exact promoted state.
	if err := privateio.WriteJSON(scopedRecoveryPath(databasePath), scopedSnapshot); err != nil {
		return nil, "", errors.New("persist reconciled scoped recovery snapshot")
	}
	if err := privateio.WriteJSON(projectConnectionRecoveryPath(databasePath), projectSnapshot); err != nil {
		return nil, "", errors.New("persist reconciled project connection recovery snapshot")
	}
	if err := hooks.rename(marker.StagePath, databasePath); err != nil {
		return nil, "", errors.New("promote agent state recovery stage")
	}
	if err := syncDirectory(filepath.Dir(databasePath)); err != nil {
		return nil, "", errors.New("sync recovered agent state")
	}
	recovered, err := openStore(databasePath, now, false)
	if err != nil {
		return nil, "", errors.New("verify recovered agent state")
	}
	if err := recoverySnapshotMatches(context.Background(), recovered, snapshot, snapshotPresent); err != nil {
		_ = recovered.Close()
		return nil, "", err
	}
	if err := scopedRecoverySnapshotMatches(
		context.Background(), recovered, scopedSnapshot, scopedSnapshotPresent,
	); err != nil {
		_ = recovered.Close()
		return nil, "", err
	}
	if err := projectConnectionRecoverySnapshotMatches(
		context.Background(), recovered, projectSnapshot, projectSnapshotPresent,
	); err != nil {
		_ = recovered.Close()
		return nil, "", err
	}
	if err := finalizeRecovery(databasePath, recovered); err != nil {
		_ = recovered.Close()
		return nil, "", err
	}
	return recovered, marker.QuarantineBase, nil
}

func inspectRecoveryCanonical(
	databasePath string,
	snapshot recoverySnapshot,
	snapshotPresent bool,
	scopedSnapshot scopedRecoverySnapshot,
	scopedSnapshotPresent bool,
	projectSnapshot projectConnectionRecoverySnapshot,
	projectSnapshotPresent bool,
	now Clock,
) (*Store, string, error) {
	if _, err := os.Lstat(databasePath); os.IsNotExist(err) {
		return nil, "missing", nil
	} else if err != nil {
		return nil, "", errors.New("inspect agent state during recovery")
	}
	store, err := openStore(databasePath, now, false)
	if errors.Is(err, ErrCorrupt) {
		return nil, "corrupt", nil
	}
	if err != nil {
		return nil, "", err
	}
	if err := recoverySnapshotMatches(context.Background(), store, snapshot, snapshotPresent); err != nil {
		_ = store.Close()
		return nil, "", errors.New("resume agent state recovery: canonical state does not match recovery snapshot")
	}
	if err := scopedRecoverySnapshotMatches(
		context.Background(), store, scopedSnapshot, scopedSnapshotPresent,
	); err != nil {
		_ = store.Close()
		return nil, "", errors.New("resume agent state recovery: canonical scoped state does not match recovery snapshot")
	}
	if err := projectConnectionRecoverySnapshotMatches(
		context.Background(), store, projectSnapshot, projectSnapshotPresent,
	); err != nil {
		_ = store.Close()
		return nil, "", errors.New("resume agent state recovery: canonical project connections do not match recovery snapshot")
	}
	return store, "complete", nil
}

func recoverySnapshotMatches(
	ctx context.Context,
	store *Store,
	expected recoverySnapshot,
	snapshotPresent bool,
) error {
	actual, err := store.buildRecoverySnapshot(ctx)
	if err != nil {
		return errors.New("verify agent state recovery snapshot")
	}
	if !snapshotPresent {
		expected = recoverySnapshot{
			SchemaVersion: recoverySchemaVersion,
			Lenses:        []Lens{},
			Judgments:     []Judgment{},
		}
	}
	if !reflect.DeepEqual(actual, expected) {
		return errors.New("verify agent state recovery snapshot")
	}
	return nil
}

func scopedRecoverySnapshotMatches(
	ctx context.Context,
	store *Store,
	expected scopedRecoverySnapshot,
	present bool,
) error {
	if !present {
		return nil
	}
	actual, err := store.buildScopedRecoverySnapshot(ctx)
	if err != nil {
		return errors.New("verify scoped agent state recovery snapshot")
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf(
			"verify scoped agent state recovery snapshot: scopes=%t lenses=%t(%d/%d) actors=%t runs=%t judgments=%t(%d/%d)",
			reflect.DeepEqual(actual.Scopes, expected.Scopes),
			reflect.DeepEqual(actual.Lenses, expected.Lenses),
			len(actual.Lenses), len(expected.Lenses),
			reflect.DeepEqual(actual.Actors, expected.Actors),
			reflect.DeepEqual(actual.Runs, expected.Runs),
			reflect.DeepEqual(actual.Judgments, expected.Judgments),
			len(actual.Judgments), len(expected.Judgments),
		)
	}
	return nil
}

func quarantineDatabase(databasePath, quarantineBase string, rename func(string, string) error) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		source := databasePath + suffix
		destination := quarantineBase + suffix
		sourceInfo, sourceErr := os.Lstat(source)
		destinationInfo, destinationErr := os.Lstat(destination)
		sourceExists := sourceErr == nil
		destinationExists := destinationErr == nil
		if sourceErr != nil && !os.IsNotExist(sourceErr) ||
			destinationErr != nil && !os.IsNotExist(destinationErr) ||
			sourceExists && destinationExists {
			return errors.New("quarantine corrupt agent state")
		}
		if destinationExists {
			if !destinationInfo.Mode().IsRegular() ||
				destinationInfo.Mode().Perm() != privateio.FileMode {
				return errors.New("quarantine corrupt agent state")
			}
			continue
		}
		if !sourceExists {
			continue
		}
		if !sourceInfo.Mode().IsRegular() || sourceInfo.Mode().Perm() != privateio.FileMode {
			return errors.New("quarantine corrupt agent state")
		}
		if err := rename(source, destination); err != nil {
			return errors.New("quarantine corrupt agent state")
		}
		if err := syncDirectory(filepath.Dir(databasePath)); err != nil {
			return errors.New("quarantine corrupt agent state")
		}
	}
	return nil
}

func removeRecoveryStage(stagePath string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := stagePath + suffix
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != privateio.FileMode {
			return errors.New("clear agent state recovery stage")
		}
		if err := os.Remove(path); err != nil {
			return errors.New("clear agent state recovery stage")
		}
	}
	if err := syncDirectory(filepath.Dir(stagePath)); err != nil {
		return errors.New("clear agent state recovery stage")
	}
	return nil
}

func removeScopedRecoveryStage(stagePath string) error {
	path := scopedRecoveryPath(stagePath)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != privateio.FileMode {
		return errors.New("clear scoped agent state recovery stage")
	}
	if err := os.Remove(path); err != nil {
		return errors.New("clear scoped agent state recovery stage")
	}
	if err := syncDirectory(filepath.Dir(stagePath)); err != nil {
		return errors.New("clear scoped agent state recovery stage")
	}
	return nil
}

func removeProjectConnectionRecoveryStage(stagePath string) error {
	path := projectConnectionRecoveryPath(stagePath)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != privateio.FileMode {
		return errors.New("clear project connection recovery stage")
	}
	if err := os.Remove(path); err != nil {
		return errors.New("clear project connection recovery stage")
	}
	if err := syncDirectory(filepath.Dir(stagePath)); err != nil {
		return errors.New("clear project connection recovery stage")
	}
	return nil
}

func removeProjectConnectionAdoptionStage(stagePath string) error {
	path := projectConnectionAdoptionPath(stagePath)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != privateio.FileMode {
		return errors.New("clear project connection adoption stage")
	}
	if err := os.Remove(path); err != nil {
		return errors.New("clear project connection adoption stage")
	}
	if err := syncDirectory(filepath.Dir(stagePath)); err != nil {
		return errors.New("clear project connection adoption stage")
	}
	return nil
}

func finalizeRecovery(databasePath string, store *Store) error {
	if err := os.Remove(recoveryMarkerPath(databasePath)); err != nil {
		return errors.New("finalize agent state recovery")
	}
	if err := syncDirectory(filepath.Dir(databasePath)); err != nil {
		return errors.New("finalize agent state recovery")
	}
	if err := store.writeRecoverySnapshot(context.Background()); err != nil {
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
