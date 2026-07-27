package resourcequeue

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

// Repository is the narrow canonical port used by the derived queue.
type Repository interface {
	Load() (personalmemory.Library, error)
	MergeEnrichment(personalmemory.EnrichmentBatch) (personalmemory.EnrichmentReceipt, error)
}

type Target struct {
	ResourceID, CanonicalURL string
	// Remaining is structural only. Fetch adapters enforce their own per-response
	// limits and use this allowance to avoid crossing aggregate queue caps.
	Remaining Usage
}

// Fetcher is a port only. Policy, transport, and providers belong to
// resourcefetch; this package neither accepts nor stores provider credentials.
type Fetcher interface {
	Fetch(context.Context, Target) (FetchResult, error)
}

type FetchResult struct {
	State         string
	BlockedReason string
	Retryable     bool
	Evidence      acquisition.ImportedEvidence
	Content       *personalmemory.ExtractedContent
	Usage         Usage
	// Retry eligibility is derived only from a transient transport failure, 429,
	// or 5xx. Adapters must not label arbitrary failures retryable.
	TransientNetwork  bool
	HTTPStatus        int
	RetryAfterSeconds int64
}

type Runner struct {
	Store      *Store
	Repository Repository
	Fetcher    Fetcher
	Sleep      func(context.Context, time.Duration) error
	Now        func() time.Time
}

func (runner Runner) Recover() error {
	if runner.Store == nil {
		return errors.New("resource queue store is required")
	}
	_, err := runner.Store.Recover()
	return err
}

// Drain processes deterministic queued work until no item is immediately
// runnable. Retryable work remains queued for a later explicitly bounded run.
func (runner Runner) Drain(ctx context.Context) error {
	if err := runner.Recover(); err != nil {
		return err
	}
	for {
		settled, err := runner.settleBudgetRemainder()
		if err != nil {
			return err
		}
		if settled {
			continue
		}
		processed, err := runner.ProcessNext(ctx)
		if err != nil {
			return err
		}
		if !processed {
			return nil
		}
	}
}

func (runner Runner) settleBudgetRemainder() (bool, error) {
	resourceIDs, found, err := runner.Store.BudgetRemainder()
	if err != nil || !found {
		return false, err
	}
	library, err := runner.Repository.Load()
	if err != nil {
		return false, err
	}
	urls := make(map[string]string, len(library.Resources))
	for _, resource := range library.Resources {
		urls[resource.ResourceID] = resource.CanonicalURL
	}
	resources := make([]acquisition.ImportedEvidence, 0, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		canonicalURL, exists := urls[resourceID]
		if !exists {
			return false, errors.New("queued resource is absent from canonical library")
		}
		resources = append(resources, acquisition.ImportedEvidence{
			CanonicalURL: canonicalURL,
			State:        "failed",
			AccessClass:  "public",
			Missingness:  []string{"resource_blocked:budget_exhausted"},
		})
	}
	if _, err := runner.Repository.MergeEnrichment(personalmemory.EnrichmentBatch{
		SchemaVersion: personalmemory.EnrichmentBatchSchemaVersion,
		Resources:     resources,
	}); err != nil {
		return false, err
	}
	if err := runner.Store.TerminalizeBudgetRemainder(resourceIDs); err != nil {
		return false, err
	}
	return true, nil
}

func (runner Runner) ProcessNext(ctx context.Context) (bool, error) {
	if runner.Store == nil || runner.Repository == nil || runner.Fetcher == nil {
		return false, errors.New("resource queue runner is incomplete")
	}
	item, found, err := runner.Store.ClaimNext()
	if err != nil || !found {
		return found, err
	}
	target, err := runner.target(item.ResourceID)
	if err != nil {
		return true, err
	}
	if item.State == StateBlocked {
		return true, runner.mergeAndFinish(target, item, FetchResult{})
	}
	if target.Remaining.Requests < 1 {
		return true, errors.New("resource queue item has no request reservation")
	}

	result, fetchErr := runner.Fetcher.Fetch(ctx, target)
	if exhausted, err := runner.Store.Consume(item.ResourceID, result.Usage); err != nil {
		return true, err
	} else if exhausted {
		item.State, item.Reason = StateBlocked, "budget_exhausted"
		return true, runner.mergeAndFinish(target, item, FetchResult{})
	}
	if retryEligible(result) && item.Attempts < runner.profileAttempts() {
		if err := runner.sleepRetry(ctx, item.Attempts, result.RetryAfterSeconds); err != nil {
			return true, err
		}
		return true, runner.Store.Requeue(item.ResourceID)
	}
	if fetchErr != nil {
		reason := result.BlockedReason
		if !IsBlockedReason(reason) {
			reason = "unreachable"
		}
		item.State, item.Reason = StateBlocked, reason
		return true, runner.mergeAndFinish(target, item, FetchResult{})
	}
	if result.BlockedReason != "" {
		if blockedHasPayload(result) {
			return true, errors.New("blocked resource fetch included content")
		}
		item.State, item.Reason = StateBlocked, approvedBlockedReason(result.BlockedReason)
		return true, runner.mergeAndFinish(target, item, FetchResult{})
	}
	if result.State != StateComplete && result.State != StatePartial {
		item.State, item.Reason = StateBlocked, "budget_exhausted"
		return true, runner.mergeAndFinish(target, item, FetchResult{})
	}
	if result.State == StateComplete && result.Content == nil {
		return true, errors.New("complete resource fetch requires content")
	}
	item.State, item.Reason = result.State, ""
	return true, runner.mergeFetchedOrBlock(target, item, result)
}

func (runner Runner) profileAttempts() int {
	queue, err := runner.Store.Load()
	if err != nil {
		return 0
	}
	return queue.Profile.MaxAttemptsPerResource
}

func (runner Runner) target(resourceID string) (Target, error) {
	library, err := runner.Repository.Load()
	if err != nil {
		return Target{}, err
	}
	queue, err := runner.Store.Load()
	if err != nil {
		return Target{}, err
	}
	reservedRequests := 0
	for _, item := range queue.Items {
		if item.ResourceID == resourceID && item.State == StateProcessing {
			reservedRequests = item.ReservedRequests
			break
		}
	}
	remaining := Usage{
		Requests:            reservedRequests,
		DownloadedBytes:     queue.Profile.MaxDownloadedBytes - queue.Counters.DownloadedBytes,
		DecodedBytes:        queue.Profile.MaxDecodedBytes - queue.Counters.DecodedBytes,
		ExtractedBytes:      queue.Profile.MaxExtractedBytes - queue.Counters.ExtractedBytes,
		RuntimeStorageBytes: queue.Profile.MaxRuntimeStorageBytes - queue.Counters.RuntimeStorageBytes,
		WallSeconds:         queue.Profile.MaxRunWallSeconds - queue.Counters.WallSeconds,
	}
	for _, resource := range library.Resources {
		if resource.ResourceID == resourceID {
			return Target{ResourceID: resourceID, CanonicalURL: resource.CanonicalURL, Remaining: remaining}, nil
		}
	}
	return Target{}, errors.New("queued resource is absent from canonical library")
}

func retryEligible(result FetchResult) bool {
	return result.TransientNetwork || result.HTTPStatus == 429 || (result.HTTPStatus >= 500 && result.HTTPStatus <= 599)
}

func (runner Runner) sleepRetry(ctx context.Context, attempt int, retryAfter int64) error {
	queue, err := runner.Store.Load()
	if err != nil {
		return err
	}
	delay := queue.Profile.FetchPolicy.RetryBackoffOneSeconds
	if attempt > 1 {
		delay = queue.Profile.FetchPolicy.RetryBackoffTwoSeconds
	}
	if retryAfter > 0 && retryAfter < delay {
		delay = retryAfter
	}
	if retryAfter > delay {
		delay = retryAfter
	}
	if delay > queue.Profile.FetchPolicy.MaxRetryAfterSeconds {
		delay = queue.Profile.FetchPolicy.MaxRetryAfterSeconds
	}
	if runner.Now != nil {
		_ = runner.Now()
	}
	if runner.Sleep != nil {
		return runner.Sleep(ctx, time.Duration(delay)*time.Second)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(delay) * time.Second):
		return nil
	}
}

func approvedBlockedReason(reason string) string {
	if IsBlockedReason(reason) {
		return reason
	}
	// Unknown adapter outcomes and per-response oversize are never persisted as
	// novel reasons. Both consume the bounded fetch envelope.
	return "budget_exhausted"
}

func blockedHasPayload(result FetchResult) bool {
	if result.Content != nil {
		return true
	}
	evidence := result.Evidence
	return evidence.CanonicalItemID != "" || evidence.Metadata.Title != "" || evidence.Metadata.Author != "" || evidence.Metadata.PublishedAt != "" || len(evidence.Excerpts) != 0 || len(evidence.RelatedURLs) != 0
}

// A fetched public page is still untrusted input. If its payload cannot cross
// the canonical validation boundary, discard that payload and record a fixed
// manual-processing terminal instead of stopping every later resource. A
// genuine repository/storage failure also rejects the payload-free fallback,
// so infrastructure failures continue to fail closed.
func (runner Runner) mergeFetchedOrBlock(target Target, item Item, result FetchResult) error {
	if err := runner.mergeCanonical(target, item, result); err != nil {
		item.State, item.Reason = StateBlocked, "manual_processing_required"
		return runner.mergeAndFinish(target, item, FetchResult{})
	}
	return runner.finishQueueItem(item)
}

func (runner Runner) mergeAndFinish(target Target, item Item, result FetchResult) error {
	if err := runner.mergeCanonical(target, item, result); err != nil {
		return err
	}
	return runner.finishQueueItem(item)
}

func (runner Runner) mergeCanonical(target Target, item Item, result FetchResult) error {
	canonicalState, missingness, err := CanonicalState(item.State, item.Reason)
	if err != nil {
		return err
	}
	evidence := result.Evidence
	evidence.CanonicalURL = target.CanonicalURL
	evidence.State = canonicalState
	if evidence.AccessClass == "" {
		if canonicalState == "inaccessible" {
			evidence.AccessClass = "unsupported"
		} else {
			evidence.AccessClass = "public"
		}
	}
	if item.State == StateBlocked {
		evidence.Missingness = append(evidence.Missingness, missingness...)
	}
	batch := personalmemory.EnrichmentBatch{SchemaVersion: personalmemory.EnrichmentBatchSchemaVersion, Resources: []acquisition.ImportedEvidence{evidence}}
	if result.Content != nil {
		content := *result.Content
		content.CanonicalURL = target.CanonicalURL
		batch.Contents = []personalmemory.ExtractedContent{content}
	}
	_, err = runner.Repository.MergeEnrichment(batch)
	return err
}

func (runner Runner) finishQueueItem(item Item) error {
	// A capacity terminal item was never marked processing; persist it directly
	// after canonical readback has succeeded.
	if item.State == StateBlocked && item.Attempts == 0 {
		return nil
	}
	return runner.Store.Finish(item.ResourceID, item.State, item.Reason)
}

// CurrentMissingness is deliberately canonical-derived helper data for compact
// and explicit-get callers. It contains no queue consultation.
func CurrentMissingness(resource personalmemory.ResourceContext) []string {
	values := append([]string(nil), resource.Missingness...)
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
	return values
}
