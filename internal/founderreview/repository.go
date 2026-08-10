package founderreview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

var (
	runIDPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	proofHashPattern  = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	retryTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{16,256}$`)
)

type Repository struct {
	mu          sync.Mutex
	root        string
	reviewDir   string
	currentPath string
	backupPath  string
	lockPath    string
	now         func() time.Time
	entropy     io.Reader
	faults      privateio.FaultInjector
}

// NewRepository confines a single immutable founder review below root. root
// is prepared as an owner-only private runtime directory on the first Create.
func NewRepository(root string, options Options) (*Repository, error) {
	if strings.TrimSpace(root) == "" || !filepath.IsAbs(root) {
		return nil, ErrInvalid
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	entropy := options.Entropy
	if entropy == nil {
		entropy = rand.Reader
	}
	root = filepath.Clean(root)
	reviewDir := filepath.Join(root, "founder-review")
	return &Repository{
		root: root, reviewDir: reviewDir,
		currentPath: filepath.Join(reviewDir, "review.json"),
		backupPath:  filepath.Join(reviewDir, "review.backup.json"),
		lockPath:    filepath.Join(reviewDir, "review.lock"),
		now:         now, entropy: entropy, faults: options.Faults,
	}, nil
}

func (repository *Repository) Root() string { return repository.root }

// Create persists exactly one founder outcome. Replacing it would obscure the
// outcome that drove closure or a return to Diagnose/Shape, so it is refused.
func (repository *Repository) Create(ctx context.Context, request Request) (Record, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if err := validateRequest(request); err != nil {
		return Record{}, ErrInvalid
	}
	if err := repository.prepare(); err != nil {
		return Record{}, err
	}
	lock, err := repository.lock()
	if err != nil {
		return Record{}, err
	}
	defer lock.Close()
	if _, err := os.Lstat(repository.currentPath); err == nil {
		existing, err := repository.loadUnlocked()
		if err != nil {
			return Record{}, err
		}
		if existing.RetryTokenHash != retryTokenHash(request.RetryToken) {
			return Record{}, ErrAlreadyRecorded
		}
		if existing.EventID != eventID(request) {
			return Record{}, ErrRetryConflict
		}
		return existing, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Record{}, ErrUnavailable
	}
	runID, err := randomRunID(repository.entropy)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	record := Record{
		SchemaVersion:              SchemaVersion,
		RunID:                      runID,
		ProofRunID:                 request.ProofRunID,
		StructuralProofFingerprint: request.StructuralProofFingerprint,
		CitedRecordsFingerprint:    request.CitedRecordsFingerprint,
		Verdict:                    request.Verdict,
		EventID:                    eventID(request),
		RetryTokenHash:             retryTokenHash(request.RetryToken),
		RecordedAt:                 repository.now().UTC().Truncate(time.Second).Format(time.RFC3339),
	}
	if err := validateRecord(record); err != nil {
		return Record{}, ErrInvalid
	}
	next, err := privateio.CanonicalJSONBytes(record)
	if err != nil {
		return Record{}, ErrInvalid
	}
	validate := func(data []byte) error {
		_, err := parseRecord(data)
		return err
	}
	if err := privateio.AtomicReplaceWithBackup(repository.root, repository.currentPath, repository.backupPath, next, nil, MaxRecordBytes, validate, repository.faults); err != nil {
		return Record{}, ErrUnavailable
	}
	persisted, err := repository.loadUnlocked()
	if err != nil || persisted != record {
		return Record{}, ErrUnavailable
	}
	return persisted, nil
}

func (repository *Repository) Load(ctx context.Context) (Record, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if _, err := os.Lstat(repository.root); errors.Is(err, fs.ErrNotExist) {
		return Record{}, ErrNotFound
	} else if err != nil {
		return Record{}, ErrUnavailable
	}
	if _, err := os.Lstat(repository.currentPath); errors.Is(err, fs.ErrNotExist) {
		return Record{}, ErrNotFound
	} else if err != nil {
		return Record{}, ErrUnavailable
	}
	lock, err := repository.lock()
	if err != nil {
		return Record{}, err
	}
	defer lock.Close()
	return repository.loadUnlocked()
}

func (repository *Repository) prepare() error {
	if err := privateio.PrepareDir(repository.root); err != nil {
		return ErrUnavailable
	}
	if err := privateio.PrepareDir(repository.reviewDir); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (repository *Repository) lock() (*privateio.AdvisoryLock, error) {
	lock, err := privateio.AcquireAdvisoryLock(repository.root, repository.lockPath)
	if errors.Is(err, privateio.ErrLockBusy) {
		return nil, ErrLockBusy
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	return lock, nil
}

func (repository *Repository) loadUnlocked() (Record, error) {
	data, err := privateio.ReadFileBounded(repository.root, repository.currentPath, MaxRecordBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, ErrUnavailable
	}
	record, err := parseRecord(data)
	if err != nil {
		return Record{}, ErrUnavailable
	}
	return record, nil
}

func randomRunID(reader io.Reader) (string, error) {
	buffer := make([]byte, 32)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func retryTokenHash(token string) string {
	sum := sha256.Sum256([]byte("mindline-founder-review-retry/v1\x00" + token))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func eventID(request Request) string {
	value := strings.Join([]string{
		"mindline-founder-review-event/v1",
		request.ProofRunID,
		request.StructuralProofFingerprint,
		request.CitedRecordsFingerprint,
		string(request.Verdict),
		retryTokenHash(request.RetryToken),
	}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
