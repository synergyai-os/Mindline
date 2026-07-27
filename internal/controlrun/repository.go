package controlrun

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

type Repository struct {
	mu                sync.Mutex
	root              string
	controlDir        string
	currentPath       string
	backupPath        string
	lockPath          string
	entropy           io.Reader
	faults            privateio.FaultInjector
	defaultGeneration string
}

func NewRepository(root string, options Options) (*Repository, error) {
	if strings.TrimSpace(root) == "" || !filepath.IsAbs(root) {
		return nil, errors.New("control run selection root is invalid")
	}
	entropy := options.Entropy
	if entropy == nil {
		entropy = rand.Reader
	}
	defaultGeneration, err := randomGeneration(entropy)
	if err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	controlDir := filepath.Join(root, "control")
	return &Repository{
		root: root, controlDir: controlDir,
		currentPath: filepath.Join(controlDir, "selected-run.json"),
		backupPath:  filepath.Join(controlDir, "selected-run.backup.json"),
		lockPath:    filepath.Join(controlDir, "settings.lock"),
		entropy:     entropy, faults: options.Faults, defaultGeneration: defaultGeneration,
	}, nil
}

func NewDefaultRepository(options Options) (*Repository, error) {
	root, err := privateio.DefaultControlPlaneRoot()
	if err != nil {
		return nil, err
	}
	return NewRepository(root, options)
}

func (repository *Repository) Root() string { return repository.root }

func (repository *Repository) Load(ctx context.Context) (Snapshot, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if _, err := os.Lstat(repository.currentPath); os.IsNotExist(err) {
		return repository.defaultSnapshot(), nil
	}
	lock, err := repository.lock()
	if err != nil {
		return Snapshot{}, err
	}
	defer lock.Close()
	return repository.loadUnlocked()
}

func (repository *Repository) CompareAndSwap(ctx context.Context, expected Revision, explicitRunID string) (Snapshot, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := repository.prepare(); err != nil {
		return Snapshot{}, err
	}
	lock, err := repository.lock()
	if err != nil {
		return Snapshot{}, err
	}
	defer lock.Close()
	current, err := repository.loadUnlocked()
	if err != nil {
		return Snapshot{}, err
	}
	if current.State == StateRecoveryRequired {
		return current, ErrRecoveryRequired
	}
	if current.Document.Revision() != expected || current.Document.Version == math.MaxUint64 {
		return current, ErrConflict
	}
	if err := validateReservedRun(repository.root, explicitRunID); err != nil {
		return current, err
	}
	generation, err := randomGeneration(repository.entropy)
	if err != nil {
		return current, err
	}
	document := sealDocument(current.Document.Version+1, generation, explicitRunID)
	next, err := canonicalDocumentBytes(document)
	if err != nil {
		return current, ErrInvalid
	}
	var prior []byte
	if current.Document.Version > 0 {
		prior, err = canonicalDocumentBytes(current.Document)
		if err != nil {
			return current, ErrInvalid
		}
	}
	validate := func(data []byte) error { _, err := parseDocument(data); return err }
	if err := privateio.AtomicReplaceWithBackup(repository.root, repository.currentPath, repository.backupPath, next, prior, MaxDocumentBytes, validate, repository.faults); err != nil {
		return current, err
	}
	persisted, err := repository.loadUnlocked()
	if err != nil || persisted.State == StateRecoveryRequired || persisted.Document.Fingerprint != document.Fingerprint {
		return Snapshot{}, privateio.ErrAtomicValidationFailed
	}
	return persisted, nil
}

func (repository *Repository) prepare() error {
	if err := privateio.PrepareDir(repository.root); err != nil {
		return errors.New("control run selection storage unavailable")
	}
	if err := privateio.PrepareDir(repository.controlDir); err != nil {
		return errors.New("control run selection storage unavailable")
	}
	return nil
}

func (repository *Repository) lock() (*privateio.AdvisoryLock, error) {
	lock, err := privateio.AcquireAdvisoryLock(repository.root, repository.lockPath)
	if errors.Is(err, privateio.ErrLockBusy) {
		return nil, ErrLockBusy
	}
	if err != nil {
		return nil, errors.New("control run selection storage unavailable")
	}
	return lock, nil
}

func (repository *Repository) defaultSnapshot() Snapshot {
	document := Document{SchemaVersion: SchemaVersion, Version: 0, Generation: repository.defaultGeneration, SelectedRunID: ""}
	document.Fingerprint = fingerprintDocument(document)
	return Snapshot{State: StateNone, Document: document}
}

func (repository *Repository) loadUnlocked() (Snapshot, error) {
	data, err := privateio.ReadFileBounded(repository.root, repository.currentPath, MaxDocumentBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return repository.defaultSnapshot(), nil
	}
	if err != nil {
		problem := repository.problemUnlocked("unsafe_or_unreadable", nil)
		return Snapshot{State: StateRecoveryRequired, Problem: &problem}, nil
	}
	document, err := parseDocument(data)
	if err != nil {
		code := "corrupt"
		if errors.Is(err, ErrUnsupported) {
			code = "unsupported_schema"
		}
		problem := repository.problemUnlocked(code, data)
		problem.ReadableRevision = readableRevision(data)
		return Snapshot{State: StateRecoveryRequired, Problem: &problem}, nil
	}
	state := StateSelected
	if document.SelectedRunID == "" {
		state = StateNone
	}
	return Snapshot{State: state, Document: document}, nil
}

func (repository *Repository) problemUnlocked(code string, current []byte) Problem {
	backup, _ := privateio.ReadFileBounded(repository.root, repository.backupPath, MaxDocumentBytes)
	_, backupErr := parseDocument(backup)
	problem := Problem{Code: code, BackupAvailable: len(backup) > 0 && backupErr == nil}
	problem.Fingerprint = problemFingerprint(code, current, backup)
	return problem
}
