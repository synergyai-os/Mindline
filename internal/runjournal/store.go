package runjournal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	defaultMaximumJournalBytes int64 = 16 << 20
	defaultMaximumEventBytes   int64 = 64 << 10
)

var safeRunID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type StoreOptions struct {
	MaximumJournalBytes int64
	MaximumEventBytes   int64
}

type Store struct {
	root                string
	maximumJournalBytes int64
	maximumEventBytes   int64
}

func NewStore(root string, options StoreOptions) (*Store, error) {
	if err := privateio.PrepareDir(root); err != nil {
		return nil, err
	}
	if options.MaximumJournalBytes <= 0 {
		options.MaximumJournalBytes = defaultMaximumJournalBytes
	}
	if options.MaximumEventBytes <= 0 {
		options.MaximumEventBytes = defaultMaximumEventBytes
	}
	if options.MaximumEventBytes > options.MaximumJournalBytes {
		return nil, errors.New("event limit exceeds journal limit")
	}
	return &Store{root: root, maximumJournalBytes: options.MaximumJournalBytes, maximumEventBytes: options.MaximumEventBytes}, nil
}

func (store *Store) Load(ctx context.Context, runID orchestration.RunID) ([]orchestration.Event, error) {
	runDir, err := store.existingRunDir(runID)
	if err != nil {
		if os.IsNotExist(err) {
			return []orchestration.Event{}, nil
		}
		return nil, err
	}
	var loaded journal
	err = withRunLock(ctx, store.root, runDir, func() error {
		if err := validateRunDirectory(store.root, runDir); err != nil {
			return err
		}
		value, err := store.loadJournalUnlocked(runID, runDir)
		if err != nil {
			return err
		}
		loaded = value
		return nil
	})
	if err != nil {
		return nil, err
	}
	return append([]orchestration.Event(nil), loaded.Events...), nil
}

func (store *Store) Append(ctx context.Context, runID orchestration.RunID, expected orchestration.ExpectedVersion, events ...orchestration.Event) error {
	if len(events) == 0 {
		return errors.New("no activation events to append")
	}
	runDir, err := store.prepareRunDir(runID)
	if err != nil {
		return err
	}
	return withRunLock(ctx, store.root, runDir, func() error {
		if err := validateRunDirectory(store.root, runDir); err != nil {
			return err
		}
		value, err := store.loadJournalUnlocked(runID, runDir)
		if err != nil {
			return err
		}
		if uint64(expected) != uint64(len(value.Events)) {
			return orchestration.ErrVersionConflict
		}
		previous := genesisHash
		if len(value.Events) > 0 {
			previous = value.Events[len(value.Events)-1].EventHash
		}
		for index, event := range events {
			if event.RunID != runID || int64(len(event.Payload)) > store.maximumEventBytes {
				if int64(len(event.Payload)) > store.maximumEventBytes {
					return ErrEventTooLarge
				}
				return ErrJournalCorrupt
			}
			sealed, err := sealEvent(event, uint64(len(value.Events)+index+1), previous)
			if err != nil {
				return err
			}
			value.Events = append(value.Events, sealed)
			previous = sealed.EventHash
		}
		sealJournal(&value)
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if int64(len(data)) > store.maximumJournalBytes {
			return ErrJournalTooLarge
		}
		return privateio.WriteFile(filepath.Join(runDir, journalFilename), data, false)
	})
}

func (store *Store) loadJournalUnlocked(runID orchestration.RunID, runDir string) (journal, error) {
	path := filepath.Join(runDir, journalFilename)
	data, err := privateio.ReadFileBounded(store.root, path, store.maximumJournalBytes)
	if os.IsNotExist(err) {
		value := journal{SchemaVersion: JournalSchemaVersion, RunID: runID, Events: []orchestration.Event{}}
		sealJournal(&value)
		return value, nil
	}
	if err != nil {
		if errors.Is(err, privateio.ErrReadLimitExceeded) {
			return journal{}, ErrJournalTooLarge
		}
		return journal{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value journal
	if err := decoder.Decode(&value); err != nil {
		return journal{}, fmt.Errorf("%w: %v", ErrJournalCorrupt, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return journal{}, ErrJournalCorrupt
	}
	if err := validateJournal(value, runID, store.maximumEventBytes); err != nil {
		return journal{}, err
	}
	return value, nil
}

func (store *Store) prepareRunDir(runID orchestration.RunID) (string, error) {
	if !validRunID(runID) {
		return "", ErrInvalidRunID
	}
	runsDir := filepath.Join(store.root, "runs")
	if err := privateio.PrepareDir(runsDir); err != nil {
		return "", err
	}
	runDir := filepath.Join(runsDir, string(runID))
	if err := privateio.PrepareDir(runDir); err != nil {
		return "", err
	}
	return runDir, nil
}

func (store *Store) existingRunDir(runID orchestration.RunID) (string, error) {
	if !validRunID(runID) {
		return "", ErrInvalidRunID
	}
	runDir := filepath.Join(store.root, "runs", string(runID))
	info, err := os.Lstat(runDir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", ErrJournalCorrupt
	}
	return runDir, nil
}

func validRunID(runID orchestration.RunID) bool {
	return safeRunID.MatchString(string(runID)) && runID != "." && runID != ".."
}

func validateRunDirectory(root, runDir string) error {
	if err := privateio.ValidateContained(root, runDir); err != nil {
		return err
	}
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return err
	}
	allowed := map[string]bool{journalFilename: true, projectionFilename: true, leaseFilename: true, runLockFilename: true}
	for _, entry := range entries {
		if !allowed[entry.Name()] || entry.IsDir() {
			return ErrUnknownRunFile
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != privateio.FileMode {
			return ErrJournalCorrupt
		}
	}
	return nil
}

func withRunLock(ctx context.Context, root, runDir string, action func() error) error {
	if err := privateio.ValidateContained(root, runDir); err != nil {
		return err
	}
	path := filepath.Join(runDir, runLockFilename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, privateio.FileMode)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(privateio.FileMode); err != nil {
		return err
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	if err := privateio.ValidateContained(root, runDir, path); err != nil {
		return err
	}
	return action()
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
