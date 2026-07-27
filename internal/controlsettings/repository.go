package controlsettings

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
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

type Repository struct {
	mu                sync.Mutex
	root              string
	controlDir        string
	currentPath       string
	backupPath        string
	lockPath          string
	now               func() time.Time
	entropy           io.Reader
	faults            privateio.FaultInjector
	validators        map[string]AdapterValidator
	defaultGeneration string
}

func NewRepository(root string, options Options) (*Repository, error) {
	if strings.TrimSpace(root) == "" || !filepath.IsAbs(root) {
		return nil, errors.New("control settings root is invalid")
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	entropy := options.Entropy
	if entropy == nil {
		entropy = rand.Reader
	}
	defaultGeneration, err := randomGeneration(entropy)
	if err != nil {
		return nil, err
	}
	validators := make(map[string]AdapterValidator, len(options.Validators))
	for kind, validator := range options.Validators {
		if strings.TrimSpace(kind) == "" || validator == nil {
			return nil, ErrInvalid
		}
		validators[kind] = validator
	}
	root = filepath.Clean(root)
	controlDir := filepath.Join(root, "control")
	return &Repository{
		root: root, controlDir: controlDir,
		currentPath: filepath.Join(controlDir, "settings.json"),
		backupPath:  filepath.Join(controlDir, "settings.backup.json"),
		lockPath:    filepath.Join(controlDir, "settings.lock"),
		now:         now, entropy: entropy, faults: options.Faults,
		validators: validators, defaultGeneration: defaultGeneration,
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
		return repository.defaultSnapshot()
	}
	lock, err := repository.lock()
	if err != nil {
		return Snapshot{}, err
	}
	defer lock.Close()
	return repository.loadUnlocked()
}

func (repository *Repository) Save(ctx context.Context, expected Revision, draft Draft) (Snapshot, error) {
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
	if current.Document.Revision() != expected {
		return current, ErrConflict
	}
	if current.Document.Version == math.MaxUint64 {
		return current, ErrConflict
	}
	canonicalDraft, err := canonicalizeDraft(draft, repository.validators)
	if err != nil {
		return current, ErrInvalid
	}
	generation, err := randomGeneration(repository.entropy)
	if err != nil {
		return current, err
	}
	document, _ := sealDocument(current.Document.Version+1, generation, repository.now(), canonicalDraft)
	next, err := canonicalDocumentBytes(document)
	if err != nil {
		return current, ErrInvalid
	}
	var prior []byte
	if current.State == StateSaved {
		prior, err = canonicalDocumentBytes(current.Document)
		if err != nil {
			return current, ErrInvalid
		}
	}
	validate := func(data []byte) error {
		_, err := parseDocument(data, repository.validators)
		return err
	}
	if err := privateio.AtomicReplaceWithBackup(repository.root, repository.currentPath, repository.backupPath, next, prior, MaxDocumentBytes, validate, repository.faults); err != nil {
		return current, err
	}
	persisted, err := repository.loadUnlocked()
	if err != nil || persisted.State != StateSaved || persisted.Document.Fingerprint != document.Fingerprint {
		return Snapshot{}, privateio.ErrAtomicValidationFailed
	}
	return persisted, nil
}

func (repository *Repository) prepare() error {
	if err := privateio.PrepareDir(repository.root); err != nil {
		return errors.New("control settings storage unavailable")
	}
	if err := privateio.PrepareDir(repository.controlDir); err != nil {
		return errors.New("control settings storage unavailable")
	}
	return nil
}

func (repository *Repository) lock() (*privateio.AdvisoryLock, error) {
	lock, err := privateio.AcquireAdvisoryLock(repository.root, repository.lockPath)
	if errors.Is(err, privateio.ErrLockBusy) {
		return nil, ErrLockBusy
	}
	if err != nil {
		return nil, errors.New("control settings storage unavailable")
	}
	return lock, nil
}

func (repository *Repository) defaultSnapshot() (Snapshot, error) {
	draft, err := canonicalizeDraft(DefaultDraft(), repository.validators)
	if err != nil {
		return Snapshot{}, ErrInvalid
	}
	document := Document{
		SchemaVersion: SchemaVersion, Version: 0, Generation: repository.defaultGeneration,
		SavedAt: "", Draft: draft,
	}
	document.Fingerprint = fingerprintDocument(document)
	return Snapshot{State: StateDefaults, Document: document}, nil
}

func (repository *Repository) loadUnlocked() (Snapshot, error) {
	data, err := privateio.ReadFileBounded(repository.root, repository.currentPath, MaxDocumentBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return repository.defaultSnapshot()
	}
	if err != nil {
		problem := repository.problemUnlocked("unsafe_or_unreadable", nil)
		return Snapshot{State: StateRecoveryRequired, Problem: &problem}, nil
	}
	document, err := parseDocument(data, repository.validators)
	if err != nil {
		code := "corrupt"
		if errors.Is(err, ErrUnsupported) {
			code = "unsupported_schema"
		}
		problem := repository.problemUnlocked(code, data)
		return Snapshot{State: StateRecoveryRequired, Problem: &problem}, nil
	}
	return Snapshot{State: StateSaved, Document: document}, nil
}

func (repository *Repository) problemUnlocked(code string, current []byte) Problem {
	backup, _ := privateio.ReadFileBounded(repository.root, repository.backupPath, MaxDocumentBytes)
	_, backupErr := parseDocument(backup, repository.validators)
	available := len(backup) > 0 && backupErr == nil
	problem := Problem{Code: code, BackupAvailable: available}
	problem.Fingerprint = problemFingerprint(code, current, backup)
	return problem
}
