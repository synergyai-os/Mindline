package resourcequeue

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

const (
	queueFileName  = "resource-queue.json"
	backupFileName = "resource-queue.backup.json"
	queueLockName  = "resource-queue.lock"
	maxQueueBytes  = 4 << 20
)

// Store serializes all queue transitions under an owner-only advisory lock.
// Its root is separate from canonical evidence, so it can be deleted and
// rebuilt without changing the canonical fingerprint.
type Store struct {
	root, path, backup, lock string
	profile                  BudgetProfile
	faultInjector            privateio.FaultInjector
}

func NewStore(root string, profile BudgetProfile) (*Store, error) {
	if !filepath.IsAbs(root) || ValidateProfile(profile) != nil {
		return nil, errors.New("resource queue storage unavailable")
	}
	root = filepath.Clean(root)
	if err := privateio.PrepareDir(root); err != nil {
		return nil, errors.New("resource queue storage unavailable")
	}
	store := &Store{root: root, path: filepath.Join(root, queueFileName), backup: filepath.Join(root, backupFileName), lock: filepath.Join(root, queueLockName), profile: profile}
	if err := privateio.ValidateContained(root, store.path, store.backup, store.lock); err != nil {
		return nil, errors.New("resource queue storage unavailable")
	}
	return store, nil
}

func (store *Store) Load() (Queue, error) {
	var queue Queue
	err := privateio.ReadJSONStrictBounded(store.root, store.path, maxQueueBytes, &queue)
	if errors.Is(err, fs.ErrNotExist) {
		return Empty(store.profile), nil
	}
	if err != nil || Validate(queue) != nil || queue.Profile.Fingerprint != store.profile.Fingerprint {
		return Queue{}, errors.New("resource queue unavailable")
	}
	return queue, nil
}

func (store *Store) update(change func(*Queue) error) (Queue, error) {
	lock, err := privateio.AcquireAdvisoryLock(store.root, store.lock)
	if err != nil {
		return Queue{}, errors.New("resource queue busy")
	}
	defer lock.Close()
	queue, err := store.Load()
	if err != nil {
		return Queue{}, err
	}
	if err := change(&queue); err != nil {
		return Queue{}, err
	}
	queue = Seal(queue)
	next, err := privateio.CanonicalJSONBytes(queue)
	if err != nil {
		return Queue{}, errors.New("resource queue serialization failed")
	}
	var prior []byte
	if data, readErr := privateio.ReadFileBounded(store.root, store.path, maxQueueBytes); readErr == nil {
		prior = data
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return Queue{}, errors.New("resource queue unavailable")
	}
	if err := privateio.AtomicReplaceWithBackup(store.root, store.path, store.backup, next, prior, maxQueueBytes, func(data []byte) error {
		var candidate Queue
		if err := privateio.DecodeJSONStrict(data, &candidate); err != nil {
			return err
		}
		return Validate(candidate)
	}, store.faultInjector); err != nil {
		return Queue{}, errors.New("resource queue persistence failed")
	}
	return queue, nil
}

func (store *Store) Enqueue(resourceIDs []string) (Queue, error) {
	return store.update(func(queue *Queue) error {
		known := map[string]bool{}
		for _, item := range queue.Items {
			known[item.ResourceID] = true
		}
		for _, resourceID := range resourceIDs {
			if resourceID == "" {
				return errors.New("resource queue resource ID is empty")
			}
			if !known[resourceID] {
				queue.Items = append(queue.Items, Item{JobID: JobIdentity(queue.Profile, resourceID), ResourceID: resourceID, State: StateQueued})
				known[resourceID] = true
			}
		}
		return nil
	})
}

// SyncMembership atomically makes the derived queue contain exactly the
// currently processable resource IDs. Matching jobs retain their state,
// attempts, generation, and consumed counters. Removed in-flight jobs release
// only their unconsumed request reservation; consumed budget is never refunded.
func (store *Store) SyncMembership(resourceIDs []string) (Queue, error) {
	return store.updateMembership(resourceIDs, true)
}

// PruneMembership atomically removes jobs outside the current processable set
// without adopting newly discovered work. Continue uses this before advancing
// a terminal generation so new work cannot prevent the fresh budget opening.
func (store *Store) PruneMembership(resourceIDs []string) (Queue, error) {
	return store.updateMembership(resourceIDs, false)
}

func (store *Store) updateMembership(resourceIDs []string, addMissing bool) (Queue, error) {
	desired := make(map[string]bool, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		if resourceID == "" || desired[resourceID] {
			return Queue{}, errors.New("invalid resource queue membership")
		}
		desired[resourceID] = true
	}
	return store.update(func(queue *Queue) error {
		retained := make([]Item, 0, len(desired))
		known := make(map[string]bool, len(desired))
		active := false
		for _, item := range queue.Items {
			if !desired[item.ResourceID] {
				if item.State == StateProcessing {
					if item.ReservedRequests > queue.Counters.ReservedRequests {
						return errors.New("resource queue reservation accounting is invalid")
					}
					queue.Counters.ReservedRequests -= item.ReservedRequests
				}
				continue
			}
			retained = append(retained, item)
			known[item.ResourceID] = true
			if item.State == StateQueued || item.State == StateProcessing {
				active = true
			}
		}
		missing := false
		if addMissing {
			for _, resourceID := range resourceIDs {
				if !known[resourceID] {
					missing = true
					break
				}
			}
		}
		if missing && !active {
			legacyDeferredAllowed := !queue.LegacyBudgetMigrationComplete
			queue.Generation++
			queue.GenerationKind = "continuation"
			queue.GenerationClosed = false
			queue.Counters = Counters{}
			queue.LegacyBudgetMigrationComplete = true
			for index := range retained {
				item := &retained[index]
				if item.State == StateBlocked &&
					(item.Reason == ReasonRunBudgetDeferred ||
						(legacyDeferredAllowed &&
							item.Reason == ReasonBudgetExhausted &&
							item.Attempts == 0)) {
					item.State = StateQueued
					item.Reason = ""
					item.Attempts = 0
					item.ReservedRequests = 0
				}
			}
		}
		for _, resourceID := range resourceIDs {
			if !addMissing || known[resourceID] {
				continue
			}
			retained = append(retained, Item{
				JobID: JobIdentity(queue.Profile, resourceID), ResourceID: resourceID,
				State: StateQueued,
			})
		}
		if missing && active && strings.HasPrefix(queue.GenerationKind, "retry:") {
			// A retry generation identifies only its selected transient
			// terminals. Newly discovered direct work is unrelated and makes
			// retry resumption ambiguous; Continue may still adopt the mixed
			// active queue as ordinary work.
			queue.GenerationKind = ""
		}
		queue.Items = retained
		return nil
	})
}

// Rebuild replaces the complete derived queue from canonical resource state.
// It cannot import URLs, content, attempts, or prior budget counters.
func (store *Store) Rebuild(items []RebuildItem) (Queue, error) {
	return store.update(func(queue *Queue) error {
		queue.Counters = Counters{}
		queue.GenerationKind = ""
		queue.GenerationClosed = false
		// Canonical run_budget_deferred is sufficient to preserve future
		// continuation. A rebuild deliberately opts out of the ambiguous
		// budget_exhausted legacy migration because canonical state does not
		// retain the old derived attempt count.
		queue.LegacyBudgetMigrationComplete = true
		queue.Items = make([]Item, 0, len(items))
		seen := map[string]bool{}
		for _, input := range items {
			if input.ResourceID == "" || seen[input.ResourceID] {
				return errors.New("invalid resource queue rebuild item")
			}
			seen[input.ResourceID] = true
			switch input.State {
			case StateQueued:
				if input.Reason != "" {
					return errors.New("queued rebuild item has reason")
				}
			case StateComplete, StatePartial:
				if input.Reason != "" {
					return errors.New("successful rebuild item has reason")
				}
			case StateBlocked:
				if !IsBlockedReason(input.Reason) {
					return errors.New("blocked rebuild item has invalid reason")
				}
			default:
				return errors.New("invalid resource queue rebuild state")
			}
			queue.Items = append(queue.Items, Item{
				JobID:      JobIdentity(queue.Profile, input.ResourceID),
				ResourceID: input.ResourceID, State: input.State, Reason: input.Reason,
			})
		}
		return nil
	})
}

// Consume records only structurally bounded fetch usage. A violating adapter
// receives exhausted=true; no fetched text or metadata may then be merged.
func (store *Store) Consume(resourceID string, usage Usage) (exhausted bool, err error) {
	if err := validateUsage(usage); err != nil {
		return false, err
	}
	_, err = store.update(func(queue *Queue) error {
		var settleErr error
		exhausted, settleErr = consumeUsage(queue, resourceID, usage)
		return settleErr
	})
	return exhausted, err
}

// ConsumeAndCloseGeneration commits reported usage, releases the current
// reservation, requeues that exact item, and closes the generation in one
// durable transition. A crash can therefore expose either the untouched claim
// or the fully settled close, never consumed counters without the close marker.
func (store *Store) ConsumeAndCloseGeneration(resourceID string, usage Usage) (exhausted bool, err error) {
	if err := validateUsage(usage); err != nil {
		return false, err
	}
	_, err = store.update(func(queue *Queue) error {
		if queue.GenerationClosed {
			return errors.New("resource queue generation is already closed")
		}
		var settleErr error
		exhausted, settleErr = consumeUsage(queue, resourceID, usage)
		if settleErr != nil {
			return settleErr
		}
		for index := range queue.Items {
			item := &queue.Items[index]
			if item.ResourceID != resourceID || item.State != StateProcessing {
				continue
			}
			if item.ReservedRequests != 0 {
				return errors.New("resource queue request reservation is not settled")
			}
			item.State = StateQueued
			queue.GenerationClosed = true
			return nil
		}
		return errors.New("resource queue item is not processing")
	})
	return exhausted, err
}

func validateUsage(usage Usage) error {
	if usage.Requests < 0 || usage.DownloadedBytes < 0 || usage.DecodedBytes < 0 ||
		usage.ExtractedBytes < 0 || usage.RuntimeStorageBytes < 0 ||
		usage.WallSeconds < 0 {
		return errors.New("resource usage is invalid")
	}
	return nil
}

func consumeUsage(queue *Queue, resourceID string, usage Usage) (bool, error) {
	exhausted := false
	for index := range queue.Items {
		item := &queue.Items[index]
		if item.ResourceID != resourceID || item.State != StateProcessing {
			continue
		}
		if usage.Requests > item.ReservedRequests ||
			usage.Requests > queue.Profile.FetchPolicy.MaxRedirects+1 {
			exhausted = true
			queue.Counters.ReservedRequests -= item.ReservedRequests
			item.ReservedRequests = 0
			queue.Counters.Requests = queue.Profile.MaxRequests
			return exhausted, nil
		}
		nextDownload := queue.Counters.DownloadedBytes + usage.DownloadedBytes
		nextDecoded := queue.Counters.DecodedBytes + usage.DecodedBytes
		nextExtracted := queue.Counters.ExtractedBytes + usage.ExtractedBytes
		nextStorage := queue.Counters.RuntimeStorageBytes + usage.RuntimeStorageBytes
		nextWall := queue.Counters.WallSeconds + usage.WallSeconds
		if nextDownload > queue.Profile.MaxDownloadedBytes ||
			nextDecoded > queue.Profile.MaxDecodedBytes ||
			nextExtracted > queue.Profile.MaxExtractedBytes ||
			nextStorage > queue.Profile.MaxRuntimeStorageBytes ||
			nextWall > queue.Profile.MaxRunWallSeconds {
			exhausted = true
			queue.Counters.ReservedRequests -= item.ReservedRequests
			queue.Counters.Requests = minimumInt(
				queue.Profile.MaxRequests, queue.Counters.Requests+usage.Requests,
			)
			queue.Counters.DownloadedBytes = minimumInt64(queue.Profile.MaxDownloadedBytes, nextDownload)
			queue.Counters.DecodedBytes = minimumInt64(queue.Profile.MaxDecodedBytes, nextDecoded)
			queue.Counters.ExtractedBytes = minimumInt64(queue.Profile.MaxExtractedBytes, nextExtracted)
			queue.Counters.RuntimeStorageBytes = minimumInt64(queue.Profile.MaxRuntimeStorageBytes, nextStorage)
			queue.Counters.WallSeconds = minimumInt64(queue.Profile.MaxRunWallSeconds, nextWall)
			item.ReservedRequests = 0
			return exhausted, nil
		}
		queue.Counters.ReservedRequests -= item.ReservedRequests
		queue.Counters.Requests += usage.Requests
		item.ReservedRequests = 0
		queue.Counters.DownloadedBytes = nextDownload
		queue.Counters.DecodedBytes = nextDecoded
		queue.Counters.ExtractedBytes = nextExtracted
		queue.Counters.RuntimeStorageBytes = nextStorage
		queue.Counters.WallSeconds = nextWall
		return exhausted, nil
	}
	return false, errors.New("resource queue item is not processing")
}

func minimumInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func minimumInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func (store *Store) Recover() (Queue, error) {
	return store.update(func(queue *Queue) error {
		for index := range queue.Items {
			if queue.Items[index].State == StateProcessing {
				queue.Counters.ReservedRequests -= queue.Items[index].ReservedRequests
				queue.Items[index].ReservedRequests = 0
				queue.Items[index].State = StateQueued
			}
		}
		return nil
	})
}

// ClaimNext is deterministic by resource ID. Capacity and attempt exhaustion
// become a durable blocked terminal state rather than deleting a capture.
func (store *Store) ClaimNext() (Item, bool, error) {
	var selected Item
	var found bool
	_, err := store.update(func(queue *Queue) error {
		sort.Slice(queue.Items, func(i, j int) bool { return queue.Items[i].ResourceID < queue.Items[j].ResourceID })
		for index := range queue.Items {
			item := &queue.Items[index]
			if item.State != StateQueued {
				continue
			}
			if item.Attempts >= queue.Profile.MaxAttemptsPerResource {
				item.State, item.Reason = StateBlocked, ReasonBudgetExhausted
				selected, found = *item, true
				return nil
			}
			if budgetDefersItem(*queue, *item) {
				item.State, item.Reason = StateBlocked, ReasonRunBudgetDeferred
				selected, found = *item, true
				return nil
			}
			item.State = StateProcessing
			if item.Attempts == 0 {
				queue.Counters.ProcessedResources++
			}
			item.Attempts++
			queue.Counters.Attempts++
			item.ReservedRequests = queue.Profile.FetchPolicy.MaxRedirects + 1
			remaining := queue.Profile.MaxRequests - queue.Counters.Requests - queue.Counters.ReservedRequests
			if item.ReservedRequests > remaining {
				item.ReservedRequests = remaining
			}
			queue.Counters.ReservedRequests += item.ReservedRequests
			selected, found = *item, true
			return nil
		}
		return nil
	})
	return selected, found, err
}

// BudgetRemainder returns all still-queued jobs once a global frozen budget is
// exhausted. It is read-only so the canonical repository can settle the whole
// remainder atomically before the derived queue is advanced.
func (store *Store) BudgetRemainder() ([]string, bool, error) {
	queue, err := store.Load()
	if err != nil {
		return nil, false, err
	}
	if !globalBudgetExhausted(queue) {
		return nil, false, nil
	}
	resourceIDs := make([]string, 0)
	for _, item := range queue.Items {
		if item.State == StateQueued && budgetDefersItem(queue, item) {
			resourceIDs = append(resourceIDs, item.ResourceID)
		}
	}
	sort.Strings(resourceIDs)
	return resourceIDs, len(resourceIDs) != 0, nil
}

// TerminalizeBudgetRemainder advances only the exact queued identities that
// were already committed canonically as run-budget-deferred. A crash between the
// canonical write and this derived transition safely replays the same
// idempotent canonical batch on restart.
func (store *Store) TerminalizeBudgetRemainder(resourceIDs []string) error {
	if len(resourceIDs) == 0 {
		return nil
	}
	_, err := store.update(func(queue *Queue) error {
		if !globalBudgetExhausted(*queue) {
			return errors.New("resource queue budget is not exhausted")
		}
		expected := make(map[string]bool, len(resourceIDs))
		for _, resourceID := range resourceIDs {
			if resourceID == "" || expected[resourceID] {
				return errors.New("invalid resource queue budget remainder")
			}
			expected[resourceID] = true
		}
		for index := range queue.Items {
			item := &queue.Items[index]
			if !expected[item.ResourceID] {
				continue
			}
			if item.State != StateQueued {
				return errors.New("resource queue budget remainder changed")
			}
			if !budgetDefersItem(*queue, *item) {
				return errors.New("resource queue item is not budget deferred")
			}
			item.State, item.Reason = StateBlocked, ReasonRunBudgetDeferred
			delete(expected, item.ResourceID)
		}
		if len(expected) != 0 {
			return errors.New("resource queue budget remainder is incomplete")
		}
		return nil
	})
	return err
}

// StartNextGeneration atomically opens at most one new bounded run. It only
// acts after the current queue is terminal, resets every aggregate counter,
// increments generation exactly once, and requeues canonical deferred work.
//
// Queues persisted before run_budget_deferred existed receive one conservative
// migration: budget_exhausted items are eligible only when their derived
// attempt count proves they were never fetched. Rebuild marks that migration
// complete because canonical budget_exhausted alone is ambiguous.
func (store *Store) StartNextGeneration() (Queue, bool, error) {
	started := false
	queue, err := store.update(func(queue *Queue) error {
		for _, item := range queue.Items {
			if item.State == StateQueued || item.State == StateProcessing {
				return nil
			}
		}
		eligible := make([]int, 0)
		for index, item := range queue.Items {
			if item.State != StateBlocked {
				continue
			}
			if item.Reason == ReasonRunBudgetDeferred ||
				(!queue.LegacyBudgetMigrationComplete && item.Reason == ReasonBudgetExhausted && item.Attempts == 0) {
				eligible = append(eligible, index)
			}
		}
		if len(eligible) == 0 {
			return nil
		}
		queue.Generation++
		queue.GenerationKind = "continuation"
		queue.GenerationClosed = false
		queue.Counters = Counters{}
		queue.LegacyBudgetMigrationComplete = true
		for _, index := range eligible {
			queue.Items[index].State = StateQueued
			queue.Items[index].Reason = ""
			queue.Items[index].ReservedRequests = 0
		}
		started = true
		return nil
	})
	return queue, started, err
}

// StartRetryGeneration atomically opens one operator-authorized retry
// generation for a fixed transient terminal reason. It refuses active work,
// resets the selected resources' attempt budgets, and leaves every permanent
// terminal untouched.
func (store *Store) StartRetryGeneration(reason string) (Queue, bool, error) {
	if !IsRetryableTerminalReason(reason) {
		return Queue{}, false, errors.New("resource retry reason is not approved")
	}
	started := false
	queue, err := store.update(func(queue *Queue) error {
		for _, item := range queue.Items {
			if item.State == StateQueued || item.State == StateProcessing {
				return errors.New("resource queue is not terminal")
			}
		}
		kind := "retry:" + reason
		if queue.GenerationKind == kind {
			return nil
		}
		eligible := make([]int, 0)
		for index, item := range queue.Items {
			if item.State == StateBlocked && item.Reason == reason {
				eligible = append(eligible, index)
			}
		}
		if len(eligible) == 0 {
			return nil
		}
		queue.Generation++
		queue.GenerationKind = kind
		queue.GenerationClosed = false
		queue.Counters = Counters{}
		queue.LegacyBudgetMigrationComplete = true
		for _, index := range eligible {
			queue.Items[index].State = StateQueued
			queue.Items[index].Reason = ""
			queue.Items[index].Attempts = 0
			queue.Items[index].ReservedRequests = 0
		}
		started = true
		return nil
	})
	return queue, started, err
}

func globalBudgetExhausted(queue Queue) bool {
	return queue.GenerationClosed ||
		queue.Counters.ProcessedResources >= queue.Profile.MaxResources ||
		queue.Counters.Requests+queue.Counters.ReservedRequests >= queue.Profile.MaxRequests ||
		queue.Counters.DownloadedBytes >= queue.Profile.MaxDownloadedBytes ||
		queue.Counters.DecodedBytes >= queue.Profile.MaxDecodedBytes ||
		queue.Counters.ExtractedBytes >= queue.Profile.MaxExtractedBytes ||
		queue.Counters.RuntimeStorageBytes >= queue.Profile.MaxRuntimeStorageBytes ||
		queue.Counters.WallSeconds >= queue.Profile.MaxRunWallSeconds
}

func budgetDefersItem(queue Queue, item Item) bool {
	if queue.GenerationClosed ||
		queue.Counters.Requests+queue.Counters.ReservedRequests >= queue.Profile.MaxRequests ||
		queue.Counters.DownloadedBytes >= queue.Profile.MaxDownloadedBytes ||
		queue.Counters.DecodedBytes >= queue.Profile.MaxDecodedBytes ||
		queue.Counters.ExtractedBytes >= queue.Profile.MaxExtractedBytes ||
		queue.Counters.RuntimeStorageBytes >= queue.Profile.MaxRuntimeStorageBytes ||
		queue.Counters.WallSeconds >= queue.Profile.MaxRunWallSeconds {
		return true
	}
	return queue.Counters.ProcessedResources >= queue.Profile.MaxResources && item.Attempts == 0
}

func (store *Store) Requeue(resourceID string) error {
	_, err := store.update(func(queue *Queue) error {
		for index := range queue.Items {
			if queue.Items[index].ResourceID == resourceID && queue.Items[index].State == StateProcessing {
				queue.Counters.ReservedRequests -= queue.Items[index].ReservedRequests
				queue.Items[index].ReservedRequests = 0
				queue.Items[index].State = StateQueued
				return nil
			}
		}
		return errors.New("resource queue item is not processing")
	})
	return err
}

func (store *Store) Finish(resourceID, state, reason string) error {
	if _, _, err := CanonicalState(state, reason); err != nil {
		return err
	}
	_, err := store.update(func(queue *Queue) error {
		for index := range queue.Items {
			if queue.Items[index].ResourceID == resourceID && queue.Items[index].State == StateProcessing {
				if queue.Items[index].ReservedRequests != 0 {
					return errors.New("resource queue request reservation was not settled")
				}
				queue.Items[index].State, queue.Items[index].Reason = state, reason
				return nil
			}
		}
		return errors.New("resource queue item is not processing")
	})
	return err
}

// Delete removes only derived queue state. Canonical evidence is intentionally
// untouched; callers use this to prove queue rebuild readback invariance.
func (store *Store) Delete() error {
	if err := privateio.ValidateContained(store.root, store.path, store.backup, store.lock); err != nil {
		return errors.New("resource queue unavailable")
	}
	lock, err := privateio.AcquireAdvisoryLock(store.root, store.lock)
	if err != nil {
		return errors.New("resource queue busy")
	}
	defer lock.Close()
	// Keep the lock inode: deleting an open advisory-lock file would allow a
	// second writer to create and lock a different inode. The queue and its
	// backup are the complete derived state and are safe to remove under this
	// stable lock.
	for _, path := range []string{store.path, store.backup} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return errors.New("resource queue deletion failed")
		}
	}
	return nil
}
