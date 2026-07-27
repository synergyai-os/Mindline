package slack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	slackBudgetSchema               = "slack_http_budget/v1"
	maximumSlackBudgetBytes   int64 = 4 << 20
	maximumSlackResponseBytes       = int64(2 << 20)
)

type SlackBudgetScope struct {
	WorkspaceID string `json:"workspace_id"`
	ChannelID   string `json:"channel_id"`
	Oldest      string `json:"oldest"`
	Latest      string `json:"latest"`
}

type slackBudgetReservation struct {
	MaximumItems int   `json:"maximum_items"`
	MaximumBytes int64 `json:"maximum_bytes"`
}

type SlackBudgetUsage struct {
	SchemaVersion     string                            `json:"schema_version"`
	Fingerprint       string                            `json:"fingerprint"`
	Scope             SlackBudgetScope                  `json:"scope"`
	BudgetFingerprint string                            `json:"budget_fingerprint"`
	StartedAt         string                            `json:"started_at"`
	Sequence          int                               `json:"sequence"`
	Requests          int                               `json:"requests"`
	Items             int                               `json:"items"`
	Bytes             int64                             `json:"bytes"`
	Retries           int                               `json:"retries"`
	CostMicrounits    int64                             `json:"cost_microunits"`
	Pending           map[string]slackBudgetReservation `json:"pending_reservations,omitempty"`
}

type slackBudgetStore interface {
	Deadline(context.Context) (time.Time, error)
	ReserveRequest(context.Context, time.Time, int, int64) (string, error)
	SettleRequest(context.Context, string, int, int64) error
	ReserveRetry(context.Context, time.Time) error
}

func sealSlackBudgetUsage(usage SlackBudgetUsage) SlackBudgetUsage {
	usage.SchemaVersion = slackBudgetSchema
	usage.Fingerprint = ""
	usage.Fingerprint = acquisition.Fingerprint(usage)
	return usage
}

func validateSlackBudgetUsage(usage SlackBudgetUsage, scope SlackBudgetScope, budgets SlackHTTPBudgets) error {
	fingerprint := usage.Fingerprint
	usage.Fingerprint = ""
	startedAt, timeErr := time.Parse(time.RFC3339Nano, usage.StartedAt)
	if usage.SchemaVersion != slackBudgetSchema || fingerprint == "" || fingerprint != acquisition.Fingerprint(usage) || usage.Scope != scope || usage.BudgetFingerprint != acquisition.Fingerprint(budgets) || timeErr != nil || startedAt.IsZero() || usage.Sequence < 0 || usage.Requests < 0 || usage.Items < 0 || usage.Bytes < 0 || usage.Retries < 0 || usage.CostMicrounits < 0 || usage.Requests > budgets.MaximumRequests || usage.Items > budgets.MaximumItems || usage.Bytes > budgets.MaximumBytes || usage.Retries > budgets.MaximumRetries || usage.CostMicrounits > budgets.MaximumCostMicrounits {
		return errors.New("Slack durable budget authority mismatch")
	}
	pendingItems, pendingBytes := 0, int64(0)
	for id, reservation := range usage.Pending {
		if len(id) != 64 || reservation.MaximumItems < 0 || reservation.MaximumBytes <= 0 {
			return errors.New("Slack durable budget reservation mismatch")
		}
		pendingItems += reservation.MaximumItems
		pendingBytes += reservation.MaximumBytes
	}
	if pendingItems > usage.Items || pendingBytes > usage.Bytes || len(usage.Pending) > usage.Sequence {
		return errors.New("Slack durable budget reservation mismatch")
	}
	return nil
}

func newSlackBudgetUsage(scope SlackBudgetScope, budgets SlackHTTPBudgets, startedAt time.Time) SlackBudgetUsage {
	return sealSlackBudgetUsage(SlackBudgetUsage{
		Scope: scope, BudgetFingerprint: acquisition.Fingerprint(budgets),
		StartedAt: startedAt.UTC().Format(time.RFC3339Nano), Pending: map[string]slackBudgetReservation{},
	})
}

func reserveSlackRequest(usage *SlackBudgetUsage, budgets SlackHTTPBudgets, now time.Time, maximumItems int, maximumBytes int64) (string, error) {
	startedAt, err := time.Parse(time.RFC3339Nano, usage.StartedAt)
	if err != nil || now.Before(startedAt) || !now.Before(startedAt.Add(budgets.MaximumWallTime)) || maximumItems < 0 || maximumBytes <= 0 || usage.Requests+1 > budgets.MaximumRequests || usage.Items+maximumItems > budgets.MaximumItems || usage.Bytes+maximumBytes > budgets.MaximumBytes || usage.CostMicrounits+1 > budgets.MaximumCostMicrounits {
		return "", ErrSlackBudgetExceeded
	}
	usage.Sequence++
	id := acquisition.Fingerprint(struct {
		Scope    SlackBudgetScope `json:"scope"`
		Sequence int              `json:"sequence"`
	}{Scope: usage.Scope, Sequence: usage.Sequence})
	if usage.Pending == nil {
		usage.Pending = map[string]slackBudgetReservation{}
	}
	usage.Pending[id] = slackBudgetReservation{MaximumItems: maximumItems, MaximumBytes: maximumBytes}
	usage.Requests++
	usage.Items += maximumItems
	usage.Bytes += maximumBytes
	usage.CostMicrounits++
	return id, nil
}

func settleSlackRequest(usage *SlackBudgetUsage, id string, actualItems int, actualBytes int64) error {
	reservation, found := usage.Pending[id]
	if !found || actualItems < 0 || actualBytes < 0 || actualItems > reservation.MaximumItems || actualBytes > reservation.MaximumBytes {
		return errors.New("Slack durable budget settlement mismatch")
	}
	usage.Items -= reservation.MaximumItems - actualItems
	usage.Bytes -= reservation.MaximumBytes - actualBytes
	delete(usage.Pending, id)
	return nil
}

func reserveSlackRetry(usage *SlackBudgetUsage, budgets SlackHTTPBudgets, now time.Time) error {
	startedAt, err := time.Parse(time.RFC3339Nano, usage.StartedAt)
	if err != nil || now.Before(startedAt) || !now.Before(startedAt.Add(budgets.MaximumWallTime)) || usage.Retries+1 > budgets.MaximumRetries {
		return ErrSlackRateLimited
	}
	usage.Retries++
	return nil
}

type memorySlackBudgetStore struct {
	mu      sync.Mutex
	budgets SlackHTTPBudgets
	usage   SlackBudgetUsage
}

func newMemorySlackBudgetStore(budgets SlackHTTPBudgets, startedAt time.Time) *memorySlackBudgetStore {
	scope := SlackBudgetScope{WorkspaceID: "ephemeral", ChannelID: "ephemeral", Oldest: "ephemeral", Latest: "ephemeral"}
	return &memorySlackBudgetStore{budgets: budgets, usage: newSlackBudgetUsage(scope, budgets, startedAt)}
}

func (store *memorySlackBudgetStore) ReserveRequest(_ context.Context, now time.Time, maximumItems int, maximumBytes int64) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	id, err := reserveSlackRequest(&store.usage, store.budgets, now, maximumItems, maximumBytes)
	store.usage = sealSlackBudgetUsage(store.usage)
	return id, err
}

func (store *memorySlackBudgetStore) Deadline(_ context.Context) (time.Time, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	startedAt, err := time.Parse(time.RFC3339Nano, store.usage.StartedAt)
	if err != nil {
		return time.Time{}, errors.New("Slack durable budget authority mismatch")
	}
	return startedAt.Add(store.budgets.MaximumWallTime), nil
}

func (store *memorySlackBudgetStore) SettleRequest(_ context.Context, id string, actualItems int, actualBytes int64) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	err := settleSlackRequest(&store.usage, id, actualItems, actualBytes)
	store.usage = sealSlackBudgetUsage(store.usage)
	return err
}

func (store *memorySlackBudgetStore) ReserveRetry(_ context.Context, now time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	err := reserveSlackRetry(&store.usage, store.budgets, now)
	store.usage = sealSlackBudgetUsage(store.usage)
	return err
}

type FileSlackBudgetStore struct {
	root    string
	path    string
	lock    string
	scope   SlackBudgetScope
	budgets SlackHTTPBudgets
}

func NewFileSlackBudgetStore(root string, scope SlackBudgetScope, budgets SlackHTTPBudgets, startedAt time.Time) (*FileSlackBudgetStore, error) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(scope.WorkspaceID) == "" || strings.TrimSpace(scope.ChannelID) == "" || strings.TrimSpace(scope.Oldest) == "" || strings.TrimSpace(scope.Latest) == "" || startedAt.IsZero() {
		return nil, errors.New("invalid Slack durable budget scope")
	}
	if err := validateSlackHTTPBudgets(budgets); err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	if err := privateio.PrepareDir(root); err != nil {
		return nil, err
	}
	name := acquisition.Fingerprint(scope)
	store := &FileSlackBudgetStore{root: root, path: filepath.Join(root, name+".json"), lock: filepath.Join(root, name+".lock"), scope: scope, budgets: budgets}
	err := store.transact(context.Background(), func(usage *SlackBudgetUsage) error { return nil }, startedAt)
	return store, err
}

func (store *FileSlackBudgetStore) acquire(ctx context.Context) (func(), error) {
	file, err := os.OpenFile(store.lock, os.O_RDWR|os.O_CREATE, privateio.FileMode)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(privateio.FileMode); err != nil {
		file.Close()
		return nil, err
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (store *FileSlackBudgetStore) transact(ctx context.Context, update func(*SlackBudgetUsage) error, startedAt time.Time) error {
	release, err := store.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	var usage SlackBudgetUsage
	err = privateio.ReadJSONStrictBounded(store.root, store.path, maximumSlackBudgetBytes, &usage)
	if os.IsNotExist(err) {
		usage = newSlackBudgetUsage(store.scope, store.budgets, startedAt)
	} else if err != nil {
		return err
	} else if err := validateSlackBudgetUsage(usage, store.scope, store.budgets); err != nil {
		return err
	}
	if err := update(&usage); err != nil {
		return err
	}
	usage = sealSlackBudgetUsage(usage)
	if err := validateSlackBudgetUsage(usage, store.scope, store.budgets); err != nil {
		return err
	}
	return privateio.WriteJSON(store.path, usage)
}

func (store *FileSlackBudgetStore) ReserveRequest(ctx context.Context, now time.Time, maximumItems int, maximumBytes int64) (string, error) {
	var id string
	err := store.transact(ctx, func(usage *SlackBudgetUsage) error {
		var reserveErr error
		id, reserveErr = reserveSlackRequest(usage, store.budgets, now, maximumItems, maximumBytes)
		return reserveErr
	}, now)
	return id, err
}

func (store *FileSlackBudgetStore) Deadline(ctx context.Context) (time.Time, error) {
	var deadline time.Time
	err := store.transact(ctx, func(usage *SlackBudgetUsage) error {
		startedAt, parseErr := time.Parse(time.RFC3339Nano, usage.StartedAt)
		if parseErr != nil {
			return errors.New("Slack durable budget authority mismatch")
		}
		deadline = startedAt.Add(store.budgets.MaximumWallTime)
		return nil
	}, time.Now())
	return deadline, err
}

func (store *FileSlackBudgetStore) SettleRequest(ctx context.Context, id string, actualItems int, actualBytes int64) error {
	return store.transact(ctx, func(usage *SlackBudgetUsage) error {
		return settleSlackRequest(usage, id, actualItems, actualBytes)
	}, time.Now())
}

func (store *FileSlackBudgetStore) ReserveRetry(ctx context.Context, now time.Time) error {
	return store.transact(ctx, func(usage *SlackBudgetUsage) error {
		return reserveSlackRetry(usage, store.budgets, now)
	}, now)
}

func (store *FileSlackBudgetStore) Snapshot(ctx context.Context) (SlackBudgetUsage, error) {
	var snapshot SlackBudgetUsage
	err := store.transact(ctx, func(usage *SlackBudgetUsage) error {
		snapshot = *usage
		snapshot.Pending = make(map[string]slackBudgetReservation, len(usage.Pending))
		for key, value := range usage.Pending {
			snapshot.Pending[key] = value
		}
		return nil
	}, time.Now())
	return snapshot, err
}
