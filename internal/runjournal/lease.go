package runjournal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const maximumLeaseBytes int64 = 16 << 10

type Lease struct {
	SchemaVersion string              `json:"schema_version"`
	Fingerprint   string              `json:"fingerprint"`
	RunID         orchestration.RunID `json:"run_id"`
	OwnerID       string              `json:"owner_id"`
	Token         string              `json:"token"`
	Generation    uint64              `json:"generation"`
	AcquiredAt    string              `json:"acquired_at"`
	ExpiresAt     string              `json:"expires_at"`
}

type LeaseManager struct {
	root string
	now  func() time.Time
}

func NewLeaseManager(root string, now func() time.Time) (*LeaseManager, error) {
	if err := privateio.PrepareDir(root); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &LeaseManager{root: root, now: now}, nil
}

func (manager *LeaseManager) Acquire(ctx context.Context, runID orchestration.RunID, ownerID string, ttl time.Duration) (Lease, error) {
	if !validRunID(runID) || ownerID == "" || ttl <= 0 {
		return Lease{}, ErrLeaseMismatch
	}
	store := &Store{root: manager.root}
	runDir, err := store.prepareRunDir(runID)
	if err != nil {
		return Lease{}, err
	}
	var result Lease
	err = withRunLock(ctx, manager.root, runDir, func() error {
		if err := validateRunDirectory(manager.root, runDir); err != nil {
			return err
		}
		existing, err := manager.loadLease(runDir)
		if err == nil {
			expires, parseErr := time.Parse(time.RFC3339Nano, existing.ExpiresAt)
			if parseErr != nil {
				return ErrJournalCorrupt
			}
			if manager.now().UTC().Before(expires) {
				return ErrLeaseHeld
			}
		} else if !errors.Is(err, ErrLeaseNotFound) {
			return err
		}
		token, err := newLeaseToken()
		if err != nil {
			return err
		}
		now := manager.now().UTC()
		generation := uint64(1)
		if existing.Generation > 0 {
			generation = existing.Generation + 1
		}
		result = Lease{SchemaVersion: LeaseSchemaVersion, RunID: runID, OwnerID: ownerID, Token: token, Generation: generation, AcquiredAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(ttl).Format(time.RFC3339Nano)}
		sealLease(&result)
		return privateio.WriteJSON(filepath.Join(runDir, leaseFilename), result)
	})
	return result, err
}

func (manager *LeaseManager) Renew(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error) {
	if !validRunID(lease.RunID) || ttl <= 0 {
		return Lease{}, ErrLeaseMismatch
	}
	runDir, err := (&Store{root: manager.root}).existingRunDir(lease.RunID)
	if err != nil {
		if os.IsNotExist(err) {
			return Lease{}, ErrLeaseNotFound
		}
		return Lease{}, err
	}
	var result Lease
	err = withRunLock(ctx, manager.root, runDir, func() error {
		current, err := manager.loadLease(runDir)
		if err != nil {
			return err
		}
		if !sameLease(current, lease) {
			return ErrLeaseMismatch
		}
		expires, err := time.Parse(time.RFC3339Nano, current.ExpiresAt)
		if err != nil || !manager.now().UTC().Before(expires) {
			return ErrLeaseExpired
		}
		result = current
		result.Generation++
		result.ExpiresAt = manager.now().UTC().Add(ttl).Format(time.RFC3339Nano)
		sealLease(&result)
		return privateio.WriteJSON(filepath.Join(runDir, leaseFilename), result)
	})
	return result, err
}

func (manager *LeaseManager) Validate(ctx context.Context, lease Lease) error {
	if !validRunID(lease.RunID) {
		return ErrLeaseMismatch
	}
	runDir, err := (&Store{root: manager.root}).existingRunDir(lease.RunID)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrLeaseNotFound
		}
		return err
	}
	return withRunLock(ctx, manager.root, runDir, func() error {
		current, err := manager.loadLease(runDir)
		if err != nil {
			return err
		}
		if !sameLease(current, lease) {
			return ErrLeaseMismatch
		}
		expires, err := time.Parse(time.RFC3339Nano, current.ExpiresAt)
		if err != nil {
			return ErrJournalCorrupt
		}
		if !manager.now().UTC().Before(expires) {
			return ErrLeaseExpired
		}
		return nil
	})
}

func (manager *LeaseManager) Release(ctx context.Context, lease Lease) error {
	if !validRunID(lease.RunID) {
		return ErrLeaseMismatch
	}
	runDir, err := (&Store{root: manager.root}).existingRunDir(lease.RunID)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrLeaseNotFound
		}
		return err
	}
	return withRunLock(ctx, manager.root, runDir, func() error {
		current, err := manager.loadLease(runDir)
		if err != nil {
			return err
		}
		if !sameLease(current, lease) {
			return ErrLeaseMismatch
		}
		if err := os.Remove(filepath.Join(runDir, leaseFilename)); err != nil {
			return err
		}
		return syncDirectory(runDir)
	})
}

func (manager *LeaseManager) loadLease(runDir string) (Lease, error) {
	path := filepath.Join(runDir, leaseFilename)
	var lease Lease
	if err := privateio.ReadJSONStrictBounded(manager.root, path, maximumLeaseBytes, &lease); err != nil {
		if os.IsNotExist(err) {
			return Lease{}, ErrLeaseNotFound
		}
		return Lease{}, err
	}
	fingerprint := lease.Fingerprint
	lease.Fingerprint = ""
	if lease.SchemaVersion != LeaseSchemaVersion || lease.RunID == "" || lease.OwnerID == "" || lease.Token == "" || lease.Generation == 0 || fingerprint == "" || fingerprint != orchestration.Fingerprint(lease) {
		return Lease{}, ErrJournalCorrupt
	}
	lease.Fingerprint = fingerprint
	return lease, nil
}

func sealLease(lease *Lease) {
	lease.Fingerprint = ""
	lease.Fingerprint = orchestration.Fingerprint(*lease)
}

func sameLease(left, right Lease) bool {
	return left.RunID == right.RunID && left.OwnerID == right.OwnerID && left.Token == right.Token && left.Generation == right.Generation && left.Fingerprint == right.Fingerprint
}

func newLeaseToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
