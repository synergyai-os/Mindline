package resourcequeue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func TestTerminalMappingIsCanonicalAndFixed(t *testing.T) {
	tests := []struct{ state, reason, want string }{
		{StateComplete, "", "complete"}, {StatePartial, "", "partial"},
		{StateBlocked, "sensitive_or_ambiguous", "inaccessible"},
		{StateBlocked, "access_denied", "inaccessible"},
		{StateBlocked, "manual_processing_required", "inaccessible"},
		{StateBlocked, "unreachable", "failed"}, {StateBlocked, "budget_exhausted", "failed"},
	}
	for _, test := range tests {
		got, missingness, err := CanonicalState(test.state, test.reason)
		if err != nil || got != test.want {
			t.Fatalf("%s:%s = %q, %v", test.state, test.reason, got, err)
		}
		if test.state == StateBlocked && (len(missingness) != 1 || missingness[0] != "resource_blocked:"+test.reason) {
			t.Fatalf("blocked mapping missingness = %#v", missingness)
		}
	}
}

func TestStoreRestartAndCapacityTerminalAreDeterministic(t *testing.T) {
	profile := FixtureProfile()
	store, err := NewStore(t.TempDir()+"/queue", profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue([]string{"resource-c", "resource-a", "resource-b", "resource-d"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"resource-a", "resource-b", "resource-c"} {
		item, found, err := store.ClaimNext()
		if err != nil || !found || item.ResourceID != want || item.State != StateProcessing {
			t.Fatalf("claim = %#v, %v, %v", item, found, err)
		}
		if exhausted, err := store.Consume(item.ResourceID, Usage{}); err != nil || exhausted {
			t.Fatalf("settle = exhausted:%v err:%v", exhausted, err)
		}
		if err := store.Finish(item.ResourceID, StatePartial, ""); err != nil {
			t.Fatal(err)
		}
	}
	item, found, err := store.ClaimNext()
	if err != nil || !found || item.ResourceID != "resource-d" || item.State != StateBlocked || item.Reason != "budget_exhausted" {
		t.Fatalf("capacity terminal = %#v, %v, %v", item, found, err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(t.TempDir()+"/different", profile); err != nil {
		t.Fatal(err)
	}
	if before.Fingerprint == "" || before.Counters.ProcessedResources != 3 {
		t.Fatalf("unexpected stored queue %#v", before)
	}
}

func TestUsageCapLeavesItemForBudgetTerminal(t *testing.T) {
	profile := FixtureProfile()
	profile.Name = "fixture-byte-cap"
	profile.MaxDownloadedBytes = 10
	profile = SealProfile(profile)
	store, err := NewStore(t.TempDir()+"/queue", profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue([]string{"resource-a"}); err != nil {
		t.Fatal(err)
	}
	item, found, err := store.ClaimNext()
	if err != nil || !found {
		t.Fatalf("claim = %#v, %v, %v", item, found, err)
	}
	exhausted, err := store.Consume(item.ResourceID, Usage{DownloadedBytes: 11})
	if err != nil || !exhausted {
		t.Fatalf("consume = exhausted:%v err:%v", exhausted, err)
	}
	queue, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if queue.Counters.DownloadedBytes != profile.MaxDownloadedBytes ||
		queue.Items[0].State != StateProcessing ||
		queue.Items[0].ReservedRequests != 0 {
		t.Fatalf("overflow was not durably saturated before terminal mapping: %#v", queue)
	}
}

func TestRestartRecoversLeaseWithoutResettingAttemptsOrJobIdentity(t *testing.T) {
	root := t.TempDir() + "/queue"
	profile := FixtureProfile()
	store, err := NewStore(root, profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue([]string{"resource-a"}); err != nil {
		t.Fatal(err)
	}
	first, found, err := store.ClaimNext()
	if err != nil || !found || first.Attempts != 1 || first.JobID != JobIdentity(profile, "resource-a") {
		t.Fatalf("first claim = %#v, %v, %v", first, found, err)
	}
	restarted, err := NewStore(root, profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Recover(); err != nil {
		t.Fatal(err)
	}
	second, found, err := restarted.ClaimNext()
	if err != nil || !found || second.State != StateProcessing || second.Attempts != 2 || second.JobID != first.JobID {
		t.Fatalf("recovered claim = %#v, %v, %v", second, found, err)
	}
}

func TestRetriesDoNotConsumeUniqueResourceCapacityAndSettleExactRequests(t *testing.T) {
	profile := FixtureProfile()
	profile.Name = "fixture-retry-counts"
	profile.MaxRequests = 2
	profile = SealProfile(profile)
	store, err := NewStore(t.TempDir()+"/queue", profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue([]string{"resource-a", "resource-b"}); err != nil {
		t.Fatal(err)
	}
	first, found, err := store.ClaimNext()
	if err != nil || !found {
		t.Fatalf("first claim = %#v %v %v", first, found, err)
	}
	if exhausted, err := store.Consume(first.ResourceID, Usage{Requests: 1}); err != nil || exhausted {
		t.Fatalf("first consume = %v %v", exhausted, err)
	}
	if err := store.Requeue(first.ResourceID); err != nil {
		t.Fatal(err)
	}
	second, found, err := store.ClaimNext()
	if err != nil || !found || second.ResourceID != first.ResourceID || second.Attempts != 2 {
		t.Fatalf("retry claim = %#v %v %v", second, found, err)
	}
	if exhausted, err := store.Consume(second.ResourceID, Usage{Requests: 1}); err != nil || exhausted {
		t.Fatalf("second consume = %v %v", exhausted, err)
	}
	if err := store.Finish(second.ResourceID, StatePartial, ""); err != nil {
		t.Fatal(err)
	}
	blocked, found, err := store.ClaimNext()
	if err != nil || !found || blocked.State != StateBlocked || blocked.Reason != "budget_exhausted" {
		t.Fatalf("request-cap terminal = %#v %v %v", blocked, found, err)
	}
	queue, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if queue.Counters.ProcessedResources != 1 || queue.Counters.Attempts != 2 || queue.Counters.Requests != 2 || queue.Counters.ReservedRequests != 0 {
		t.Fatalf("retry counters = %#v", queue.Counters)
	}
}

func TestNamedFixtureProfilesAreSealedAndDistinct(t *testing.T) {
	profiles := FixtureProfiles()
	want := []string{"fixture-resource-count", "fixture-request-count", "fixture-download-bytes", "fixture-decoded-bytes", "fixture-extracted-bytes", "fixture-runtime-storage", "fixture-attempt-count", "fixture-wall-time"}
	seen := map[string]bool{}
	for _, name := range want {
		profile, ok := profiles[name]
		if !ok || ValidateProfile(profile) != nil || seen[profile.Fingerprint] {
			t.Fatalf("fixture profile %s = %#v", name, profile)
		}
		seen[profile.Fingerprint] = true
	}
}

func TestNamedFixtureProfilesExhaustOnlyTheirFrozenBudget(t *testing.T) {
	for name, profile := range FixtureProfiles() {
		t.Run(name, func(t *testing.T) {
			store, err := NewStore(t.TempDir()+"/queue", profile)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Enqueue([]string{"resource-a", "resource-b"}); err != nil {
				t.Fatal(err)
			}
			first, found, err := store.ClaimNext()
			if err != nil || !found {
				t.Fatalf("first claim = %#v %v %v", first, found, err)
			}
			switch name {
			case "fixture-resource-count":
				if exhausted, err := store.Consume(first.ResourceID, Usage{}); err != nil || exhausted {
					t.Fatalf("settle=%v %v", exhausted, err)
				}
				if err := store.Finish(first.ResourceID, StatePartial, ""); err != nil {
					t.Fatal(err)
				}
				second, found, err := store.ClaimNext()
				if err != nil || !found || second.State != StateBlocked || second.Reason != "budget_exhausted" {
					t.Fatalf("resource cap=%#v %v %v", second, found, err)
				}
			case "fixture-request-count":
				if exhausted, err := store.Consume(first.ResourceID, Usage{Requests: 1}); err != nil || exhausted {
					t.Fatalf("settle=%v %v", exhausted, err)
				}
				if err := store.Finish(first.ResourceID, StatePartial, ""); err != nil {
					t.Fatal(err)
				}
				second, found, err := store.ClaimNext()
				if err != nil || !found || second.State != StateBlocked || second.Reason != "budget_exhausted" {
					t.Fatalf("request cap=%#v %v %v", second, found, err)
				}
			case "fixture-attempt-count":
				if exhausted, err := store.Consume(first.ResourceID, Usage{}); err != nil || exhausted {
					t.Fatalf("settle=%v %v", exhausted, err)
				}
				if err := store.Requeue(first.ResourceID); err != nil {
					t.Fatal(err)
				}
				second, found, err := store.ClaimNext()
				if err != nil || !found || second.State != StateBlocked || second.Reason != "budget_exhausted" {
					t.Fatalf("attempt cap=%#v %v %v", second, found, err)
				}
			default:
				usage := Usage{}
				switch name {
				case "fixture-download-bytes":
					usage.DownloadedBytes = profile.MaxDownloadedBytes + 1
				case "fixture-decoded-bytes":
					usage.DecodedBytes = profile.MaxDecodedBytes + 1
				case "fixture-extracted-bytes":
					usage.ExtractedBytes = profile.MaxExtractedBytes + 1
				case "fixture-runtime-storage":
					usage.RuntimeStorageBytes = profile.MaxRuntimeStorageBytes + 1
				case "fixture-wall-time":
					usage.WallSeconds = profile.MaxRunWallSeconds + 1
				}
				exhausted, err := store.Consume(first.ResourceID, usage)
				if err != nil || !exhausted {
					t.Fatalf("%s usage exhaustion=%v err=%v", name, exhausted, err)
				}
			}
		})
	}
}

func TestRetryEligibilityAndFrozenBackoffs(t *testing.T) {
	if retryEligible(FetchResult{HTTPStatus: 400}) || retryEligible(FetchResult{HTTPStatus: 403}) || retryEligible(FetchResult{}) {
		t.Fatal("non-transient result became retryable")
	}
	if !retryEligible(FetchResult{TransientNetwork: true}) || !retryEligible(FetchResult{HTTPStatus: 429}) || !retryEligible(FetchResult{HTTPStatus: 503}) {
		t.Fatal("allowed retry was rejected")
	}
	store, err := NewStore(t.TempDir()+"/queue", FixtureProfile())
	if err != nil {
		t.Fatal(err)
	}
	delays := []time.Duration{}
	runner := Runner{Store: store, Now: func() time.Time { return time.Unix(0, 0) }, Sleep: func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil }}
	if err := runner.sleepRetry(context.Background(), 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := runner.sleepRetry(context.Background(), 2, 0); err != nil {
		t.Fatal(err)
	}
	if err := runner.sleepRetry(context.Background(), 2, 100); err != nil {
		t.Fatal(err)
	}
	if len(delays) != 3 || delays[0] != time.Second || delays[1] != 3*time.Second || delays[2] != 60*time.Second {
		t.Fatalf("retry delays = %#v", delays)
	}
}

type blockedFetcher struct{}

func (blockedFetcher) Fetch(context.Context, Target) (FetchResult, error) {
	return FetchResult{BlockedReason: "access_denied"}, nil
}

type retryRepository struct {
	library personalmemory.Library
	merges  int
}

func (repository *retryRepository) Load() (personalmemory.Library, error) {
	return repository.library, nil
}

func (repository *retryRepository) MergeEnrichment(personalmemory.EnrichmentBatch) (personalmemory.EnrichmentReceipt, error) {
	repository.merges++
	return personalmemory.EnrichmentReceipt{}, nil
}

type transientThenPartialFetcher struct{ calls int }

func (fetcher *transientThenPartialFetcher) Fetch(context.Context, Target) (FetchResult, error) {
	fetcher.calls++
	if fetcher.calls < 3 {
		return FetchResult{
			BlockedReason:    "unreachable",
			TransientNetwork: true,
			Usage:            Usage{Requests: 1, WallSeconds: 1},
		}, nil
	}
	return FetchResult{State: StatePartial, Usage: Usage{Requests: 1, WallSeconds: 1}}, nil
}

func TestNormalRetryableFetchResultRetriesBeforeTerminalMapping(t *testing.T) {
	profile := FixtureProfile()
	store, err := NewStore(t.TempDir()+"/queue", profile)
	if err != nil {
		t.Fatal(err)
	}
	resourceID := "resource-retry"
	if _, err := store.Enqueue([]string{resourceID}); err != nil {
		t.Fatal(err)
	}
	repository := &retryRepository{library: personalmemory.Library{
		Resources: []personalmemory.ResourceContext{{
			ResourceID: resourceID, CanonicalURL: "https://example.com/retry",
		}},
	}}
	fetcher := &transientThenPartialFetcher{}
	delays := 0
	runner := Runner{
		Store: store, Repository: repository, Fetcher: fetcher,
		Sleep: func(context.Context, time.Duration) error { delays++; return nil },
	}
	if err := runner.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	queue, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if fetcher.calls != 3 || delays != 2 || repository.merges != 1 ||
		len(queue.Items) != 1 || queue.Items[0].State != StatePartial ||
		queue.Items[0].Attempts != 3 || queue.Counters.Requests != 3 {
		t.Fatalf("retry lifecycle did not settle exactly once: calls=%d delays=%d merges=%d queue=%+v",
			fetcher.calls, delays, repository.merges, queue)
	}
}

type invalidFetchedContentRepository struct {
	library       personalmemory.Library
	mergeAttempts int
	fallbackState string
}

func (repository *invalidFetchedContentRepository) Load() (personalmemory.Library, error) {
	return repository.library, nil
}

func (repository *invalidFetchedContentRepository) MergeEnrichment(batch personalmemory.EnrichmentBatch) (personalmemory.EnrichmentReceipt, error) {
	repository.mergeAttempts++
	if len(batch.Contents) != 0 {
		return personalmemory.EnrichmentReceipt{}, errors.New("untrusted fetched content rejected")
	}
	if len(batch.Resources) == 1 {
		repository.fallbackState = batch.Resources[0].State
	}
	return personalmemory.EnrichmentReceipt{}, nil
}

type completeContentFetcher struct{}

func (completeContentFetcher) Fetch(context.Context, Target) (FetchResult, error) {
	return FetchResult{
		State:   StateComplete,
		Content: &personalmemory.ExtractedContent{},
		Usage:   Usage{Requests: 1},
	}, nil
}

func TestRejectedFetchedPayloadBecomesManualTerminalAndDrainContinues(t *testing.T) {
	store, err := NewStore(t.TempDir()+"/queue", FixtureProfile())
	if err != nil {
		t.Fatal(err)
	}
	resourceID := "resource-untrusted"
	if _, err := store.Enqueue([]string{resourceID}); err != nil {
		t.Fatal(err)
	}
	repository := &invalidFetchedContentRepository{library: personalmemory.Library{
		Resources: []personalmemory.ResourceContext{{
			ResourceID: resourceID, CanonicalURL: "https://example.com/untrusted",
		}},
	}}
	runner := Runner{Store: store, Repository: repository, Fetcher: completeContentFetcher{}}
	if err := runner.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	queue, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if repository.mergeAttempts != 2 || repository.fallbackState != "inaccessible" ||
		len(queue.Items) != 1 || queue.Items[0].State != StateBlocked ||
		queue.Items[0].Reason != "manual_processing_required" {
		t.Fatalf("unsafe payload did not settle as a manual terminal: merges=%d fallback=%q queue=%+v",
			repository.mergeAttempts, repository.fallbackState, queue)
	}
}

type unavailableRepository struct {
	library personalmemory.Library
}

func (repository *unavailableRepository) Load() (personalmemory.Library, error) {
	return repository.library, nil
}

func (*unavailableRepository) MergeEnrichment(personalmemory.EnrichmentBatch) (personalmemory.EnrichmentReceipt, error) {
	return personalmemory.EnrichmentReceipt{}, errors.New("storage unavailable")
}

func TestFetchedPayloadFallbackDoesNotHideRepositoryFailure(t *testing.T) {
	store, err := NewStore(t.TempDir()+"/queue", FixtureProfile())
	if err != nil {
		t.Fatal(err)
	}
	resourceID := "resource-storage-failure"
	if _, err := store.Enqueue([]string{resourceID}); err != nil {
		t.Fatal(err)
	}
	repository := &unavailableRepository{library: personalmemory.Library{
		Resources: []personalmemory.ResourceContext{{
			ResourceID: resourceID, CanonicalURL: "https://example.com/storage-failure",
		}},
	}}
	runner := Runner{Store: store, Repository: repository, Fetcher: completeContentFetcher{}}
	if err := runner.Drain(context.Background()); err == nil {
		t.Fatal("repository failure was hidden by the payload fallback")
	}
	queue, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Items) != 1 || queue.Items[0].State != StateProcessing {
		t.Fatalf("failed infrastructure item was incorrectly terminalized: %+v", queue)
	}
}

func TestQueueRebuildDoesNotChangeCanonicalBlockedReadback(t *testing.T) {
	root := t.TempDir()
	repository, err := personalmemory.NewFileRepository(root+"/library", func() time.Time {
		return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "workspace", SourceContainerID: "self",
		ExternalID: "message-1", OccurredAt: "2026-07-27T12:00:00Z",
		SourceRef: "slack://workspace/self/message-1", RawText: "https://example.com/article",
		EditDeleteState: "original", Missingness: []string{"permalink_unavailable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "slack:workspace:self", LowerInclusive: "1", UpperInclusive: "1", Watermark: "1",
		DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	initial, err := repository.Load()
	if err != nil || len(initial.Resources) != 1 {
		t.Fatalf("initial library = %#v, %v", initial, err)
	}

	profile := FixtureProfile()
	store, err := NewStore(root+"/queue", profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue([]string{initial.Resources[0].ResourceID}); err != nil {
		t.Fatal(err)
	}
	runner := Runner{Store: store, Repository: repository, Fetcher: blockedFetcher{}}
	if processed, err := runner.ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("first process = %v, %v", processed, err)
	}
	afterFirst, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	assertCanonicalBlocked(t, afterFirst, initial.Resources[0].ResourceID)

	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := NewStore(root+"/queue", profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rebuilt.Enqueue([]string{initial.Resources[0].ResourceID}); err != nil {
		t.Fatal(err)
	}
	if processed, err := (Runner{Store: rebuilt, Repository: repository, Fetcher: blockedFetcher{}}).ProcessNext(context.Background()); err != nil || !processed {
		t.Fatalf("rebuilt process = %v, %v", processed, err)
	}
	afterRebuild, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	assertCanonicalBlocked(t, afterRebuild, initial.Resources[0].ResourceID)
	if afterFirst.Fingerprint != afterRebuild.Fingerprint {
		t.Fatalf("derived queue rebuild changed canonical fingerprint: %s != %s", afterFirst.Fingerprint, afterRebuild.Fingerprint)
	}
}

func assertCanonicalBlocked(t *testing.T, library personalmemory.Library, resourceID string) {
	t.Helper()
	for _, resource := range library.Resources {
		if resource.ResourceID == resourceID {
			if resource.State != "inaccessible" || len(resource.Excerpts) != 0 || len(resource.Missingness) != 1 || resource.Missingness[0] != "resource_blocked:access_denied" {
				t.Fatalf("unexpected canonical blocked resource: %#v", resource)
			}
			return
		}
	}
	t.Fatalf("resource %s absent from canonical library", resourceID)
}
