package runjournal

import (
	"context"
	"path/filepath"

	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

type RunProjection struct {
	SchemaVersion                 string                 `json:"schema_version"`
	Fingerprint                   string                 `json:"fingerprint"`
	RunID                         orchestration.RunID    `json:"run_id"`
	JournalFingerprint            string                 `json:"journal_fingerprint"`
	Version                       uint64                 `json:"version"`
	State                         orchestration.RunState `json:"state"`
	ResumeState                   orchestration.RunState `json:"resume_state,omitempty"`
	PlanFingerprint               string                 `json:"plan_fingerprint"`
	ComponentVersions             map[string]string      `json:"component_versions"`
	AuthorityReferences           map[string]string      `json:"authority_references,omitempty"`
	AuthorityProjectionReferences map[string]string      `json:"authority_projection_references,omitempty"`
	LatestAuthorityProjection     string                 `json:"latest_authority_projection,omitempty"`
}

func (store *Store) RebuildProjection(ctx context.Context, runID orchestration.RunID) (RunProjection, error) {
	runDir, err := store.existingRunDir(runID)
	if err != nil {
		return RunProjection{}, err
	}
	var projection RunProjection
	if err := withRunLock(ctx, store.root, runDir, func() error {
		if err := validateRunDirectory(store.root, runDir); err != nil {
			return err
		}
		value, err := store.loadJournalUnlocked(runID, runDir)
		if err != nil {
			return err
		}
		aggregate, err := orchestration.Rebuild(runID, value.Events)
		if err != nil {
			return err
		}
		projection = RunProjection{
			SchemaVersion:                 ProjectionSchemaVersion,
			RunID:                         runID,
			JournalFingerprint:            value.Fingerprint,
			Version:                       aggregate.Version,
			State:                         aggregate.State,
			ResumeState:                   aggregate.ResumeState,
			PlanFingerprint:               aggregate.PlanFingerprint,
			ComponentVersions:             copyStringMap(aggregate.ComponentVersions),
			AuthorityReferences:           copyStringMap(aggregate.AuthorityReferences),
			AuthorityProjectionReferences: copyStringMap(aggregate.AuthorityProjectionReferences),
			LatestAuthorityProjection:     aggregate.LatestAuthorityProjection,
		}
		projection.Fingerprint = orchestration.Fingerprint(projection)
		return privateio.WriteJSON(filepath.Join(runDir, projectionFilename), projection)
	}); err != nil {
		return RunProjection{}, err
	}
	return projection, nil
}

func copyStringMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
