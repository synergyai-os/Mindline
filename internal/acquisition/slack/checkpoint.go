package slack

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	webAPICheckpointSchema       = "slack_web_api_checkpoint/v2"
	maximumCheckpointBytes int64 = 64 << 10
)

type WebAPICheckpointScope struct {
	WorkspaceID string `json:"workspace_id"`
	ChannelID   string `json:"channel_id"`
	Oldest      string `json:"oldest"`
	Latest      string `json:"latest"`
}

func (scope WebAPICheckpointScope) fingerprint() string {
	return acquisition.Fingerprint(scope)
}

type WebAPICheckpoint struct {
	SchemaVersion     string                `json:"schema_version"`
	Fingerprint       string                `json:"fingerprint"`
	Scope             WebAPICheckpointScope `json:"scope"`
	Phase             string                `json:"phase"`
	Attempt           int                   `json:"attempt"`
	CursorFingerprint string                `json:"cursor_fingerprint"`
	ThreadFingerprint string                `json:"thread_fingerprint,omitempty"`
	HistoryRecords    int                   `json:"history_records"`
	ReplyRecords      int                   `json:"reply_records"`
	CompletedThreads  int                   `json:"completed_threads"`
	AttemptPages      int                   `json:"attempt_pages"`
	CompletedPages    int                   `json:"completed_pages"`
}

func sealWebAPICheckpoint(checkpoint WebAPICheckpoint) WebAPICheckpoint {
	checkpoint.SchemaVersion = webAPICheckpointSchema
	checkpoint.Fingerprint = ""
	checkpoint.Fingerprint = acquisition.Fingerprint(checkpoint)
	return checkpoint
}

func validateWebAPICheckpoint(checkpoint WebAPICheckpoint, scope WebAPICheckpointScope) error {
	fingerprint := checkpoint.Fingerprint
	checkpoint.Fingerprint = ""
	if checkpoint.SchemaVersion != webAPICheckpointSchema || fingerprint == "" || fingerprint != acquisition.Fingerprint(checkpoint) || checkpoint.Scope != scope {
		return errors.New("Slack Web API checkpoint authority mismatch")
	}
	if checkpoint.Phase != "history" && checkpoint.Phase != "replies" && checkpoint.Phase != "complete" || checkpoint.Attempt <= 0 || len(checkpoint.CursorFingerprint) != 64 || checkpoint.ThreadFingerprint != "" && len(checkpoint.ThreadFingerprint) != 64 || checkpoint.HistoryRecords < 0 || checkpoint.ReplyRecords < 0 || checkpoint.CompletedThreads < 0 || checkpoint.AttemptPages < 0 || checkpoint.CompletedPages < checkpoint.AttemptPages {
		return errors.New("Slack Web API checkpoint state is invalid")
	}
	return nil
}

type WebAPICheckpointStore interface {
	Load(context.Context, WebAPICheckpointScope) (WebAPICheckpoint, bool, error)
	Save(context.Context, WebAPICheckpoint) error
	Clear(context.Context, WebAPICheckpointScope) error
}

type memoryWebAPICheckpointStore struct {
	mu    sync.Mutex
	items map[string]WebAPICheckpoint
}

func newMemoryWebAPICheckpointStore() *memoryWebAPICheckpointStore {
	return &memoryWebAPICheckpointStore{items: map[string]WebAPICheckpoint{}}
}

func cloneWebAPICheckpoint(checkpoint WebAPICheckpoint) WebAPICheckpoint {
	encoded, _ := json.Marshal(checkpoint)
	var result WebAPICheckpoint
	_ = json.Unmarshal(encoded, &result)
	return result
}

func (store *memoryWebAPICheckpointStore) Load(_ context.Context, scope WebAPICheckpointScope) (WebAPICheckpoint, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	checkpoint, found := store.items[scope.fingerprint()]
	return cloneWebAPICheckpoint(checkpoint), found, nil
}

func (store *memoryWebAPICheckpointStore) Save(_ context.Context, checkpoint WebAPICheckpoint) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.items[checkpoint.Scope.fingerprint()] = cloneWebAPICheckpoint(checkpoint)
	return nil
}

func (store *memoryWebAPICheckpointStore) Clear(_ context.Context, scope WebAPICheckpointScope) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.items, scope.fingerprint())
	return nil
}

type FileWebAPICheckpointStore struct {
	root string
}

func NewFileWebAPICheckpointStore(root string) (*FileWebAPICheckpointStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("missing Slack checkpoint root")
	}
	root = filepath.Clean(root)
	if err := privateio.PrepareDir(root); err != nil {
		return nil, err
	}
	return &FileWebAPICheckpointStore{root: root}, nil
}

func (store *FileWebAPICheckpointStore) path(scope WebAPICheckpointScope) string {
	return filepath.Join(store.root, strings.TrimPrefix(scope.fingerprint(), "sha256:")+".json")
}

func (store *FileWebAPICheckpointStore) Load(_ context.Context, scope WebAPICheckpointScope) (WebAPICheckpoint, bool, error) {
	var checkpoint WebAPICheckpoint
	err := privateio.ReadJSONStrictBounded(store.root, store.path(scope), maximumCheckpointBytes, &checkpoint)
	if os.IsNotExist(err) {
		return WebAPICheckpoint{}, false, nil
	}
	if err != nil {
		return WebAPICheckpoint{}, false, err
	}
	if err := validateWebAPICheckpoint(checkpoint, scope); err != nil {
		return WebAPICheckpoint{}, false, err
	}
	return checkpoint, true, nil
}

func (store *FileWebAPICheckpointStore) Save(_ context.Context, checkpoint WebAPICheckpoint) error {
	checkpoint = sealWebAPICheckpoint(checkpoint)
	if err := validateWebAPICheckpoint(checkpoint, checkpoint.Scope); err != nil {
		return err
	}
	return privateio.WriteJSON(store.path(checkpoint.Scope), checkpoint)
}

func (store *FileWebAPICheckpointStore) Clear(_ context.Context, scope WebAPICheckpointScope) error {
	path := store.path(scope)
	if err := privateio.ValidateContained(store.root, path); err != nil {
		return err
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
