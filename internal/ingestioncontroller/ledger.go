package ingestioncontroller

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const maximumLedgerBytes = int64(1 << 20)

type LedgerStore struct {
	root   string
	path   string
	backup string
	lock   string
	apply  string
}

func NewLedgerStore(root string) (*LedgerStore, error) {
	if !filepath.IsAbs(root) || strings.TrimSpace(root) == "" {
		return nil, errors.New("ingestion ledger root must be absolute")
	}
	root = filepath.Clean(root)
	if err := privateio.PrepareDir(root); err != nil {
		return nil, errors.New("ingestion ledger storage unavailable")
	}
	store := &LedgerStore{root: root, path: filepath.Join(root, "ingestion-ledger.json"), backup: filepath.Join(root, "ingestion-ledger.backup.json"), lock: filepath.Join(root, "ingestion-ledger.lock"), apply: filepath.Join(root, "ingestion-apply.lock")}
	if err := privateio.ValidateContained(root, store.path, store.backup, store.lock, store.apply); err != nil {
		return nil, errors.New("ingestion ledger storage unavailable")
	}
	return store, nil
}

func (store *LedgerStore) AcquireApplyLock() (*privateio.AdvisoryLock, error) {
	lock, err := privateio.AcquireAdvisoryLock(store.root, store.apply)
	if err != nil {
		return nil, errors.New("ingestion apply busy")
	}
	return lock, nil
}

func (store *LedgerStore) Load() (Ledger, error) {
	var ledger Ledger
	err := privateio.ReadJSONStrictBounded(store.root, store.path, maximumLedgerBytes, &ledger)
	if errors.Is(err, fs.ErrNotExist) {
		return Ledger{}, nil
	}
	if err != nil || !validLedger(ledger) {
		return Ledger{}, errors.New("ingestion ledger unavailable")
	}
	return ledger, nil
}

func (store *LedgerStore) Save(ledger Ledger) error {
	if !validLedger(ledger) {
		return errors.New("invalid ingestion ledger")
	}
	lock, err := privateio.AcquireAdvisoryLock(store.root, store.lock)
	if err != nil {
		return errors.New("ingestion ledger busy")
	}
	defer lock.Close()
	next, err := privateio.CanonicalJSONBytes(ledger)
	if err != nil {
		return errors.New("ingestion ledger unavailable")
	}
	var prior []byte
	if existing, err := privateio.ReadFileBounded(store.root, store.path, maximumLedgerBytes); err == nil {
		prior = existing
	} else if !errors.Is(err, fs.ErrNotExist) {
		return errors.New("ingestion ledger unavailable")
	}
	validate := func(bytes []byte) error {
		var decoded Ledger
		if err := privateio.DecodeJSONStrict(bytes, &decoded); err != nil || !validLedger(decoded) {
			return errors.New("invalid ingestion ledger")
		}
		return nil
	}
	if err := privateio.AtomicReplaceWithBackup(store.root, store.path, store.backup, next, prior, maximumLedgerBytes, validate, nil); err != nil {
		return errors.New("ingestion ledger unavailable")
	}
	return nil
}

func validLedger(ledger Ledger) bool {
	if ledger.SchemaVersion != LedgerSchemaVersion || strings.TrimSpace(ledger.RunID) == "" ||
		(ledger.State != "incomplete" && ledger.State != "recovering" && ledger.State != "failed" && ledger.State != "complete") ||
		strings.TrimSpace(ledger.SourceAdapter) == "" || strings.TrimSpace(ledger.SourceScope) == "" ||
		strings.TrimSpace(ledger.ConfigurationFingerprint) == "" || ledger.DeliveredCount < 0 || ledger.CanonicalDeclaredCount < 0 ||
		ledger.StructuralExcludedCount < 0 || ledger.DeliveredCount != ledger.CanonicalDeclaredCount+ledger.StructuralExcludedCount ||
		ledger.OwnedCount < 0 || ledger.RetainedCount < 0 || ledger.WithheldCount < 0 || ledger.OwnedCount != ledger.RetainedCount+ledger.WithheldCount+ledger.StructuralExcludedCount ||
		ledger.OverlapCount < 0 || ledger.GapCount < 0 || ledger.ThreadCount < 0 || ledger.CanonicalBeforeCount < 0 || ledger.CanonicalAfterCount < 0 {
		return false
	}
	return len(ledger.AggregateCommitment) == 64 && len(ledger.CanonicalBeforeFingerprint) == 64 && len(ledger.CanonicalAfterFingerprint) == 64
}
