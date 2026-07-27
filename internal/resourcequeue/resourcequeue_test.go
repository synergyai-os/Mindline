package resourcequeue

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
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
		{StateBlocked, ReasonRunBudgetDeferred, "failed"},
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

func TestQueueGenerationIsBackwardCompatibleAndNonnegative(t *testing.T) {
	profile := FixtureProfile()
	legacy := Empty(profile)
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"generation"`) || strings.Contains(string(payload), `"legacy_budget_migration_complete"`) {
		t.Fatalf("zero-value continuation fields changed the legacy queue envelope: %s", payload)
	}
	store, err := NewStore(t.TempDir()+"/queue", profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Generation != 0 {
		t.Fatalf("legacy queue did not load as generation zero: %+v err=%v", loaded, err)
	}
	loaded.Generation = -1
	loaded = Seal(loaded)
	if Validate(loaded) == nil {
		t.Fatal("negative queue generation was accepted")
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
	if err != nil || !found || item.ResourceID != "resource-d" || item.State != StateBlocked || item.Reason != ReasonRunBudgetDeferred {
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
	if err != nil || !found || blocked.State != StateBlocked || blocked.Reason != ReasonRunBudgetDeferred {
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
				if err != nil || !found || second.State != StateBlocked || second.Reason != ReasonRunBudgetDeferred {
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
				if err != nil || !found || second.State != StateBlocked || second.Reason != ReasonRunBudgetDeferred {
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

type partialFetcher struct{}

func (partialFetcher) Fetch(context.Context, Target) (FetchResult, error) {
	return FetchResult{State: StatePartial, Usage: Usage{Requests: 1}}, nil
}

type batchRecordingRepository struct {
	library    personalmemory.Library
	batchSizes []int
}

func (repository *batchRecordingRepository) Load() (personalmemory.Library, error) {
	return repository.library, nil
}

func (repository *batchRecordingRepository) MergeEnrichment(batch personalmemory.EnrichmentBatch) (personalmemory.EnrichmentReceipt, error) {
	repository.batchSizes = append(repository.batchSizes, len(batch.Resources))
	return personalmemory.EnrichmentReceipt{}, nil
}

func TestGlobalBudgetRemainderSettlesInOneCanonicalBatch(t *testing.T) {
	profile := FixtureProfile()
	profile.MaxResources = 1
	profile = SealProfile(profile)
	store, err := NewStore(t.TempDir()+"/queue", profile)
	if err != nil {
		t.Fatal(err)
	}
	resourceIDs := []string{"resource-a", "resource-b", "resource-c"}
	if _, err := store.Enqueue(resourceIDs); err != nil {
		t.Fatal(err)
	}
	repository := &batchRecordingRepository{library: personalmemory.Library{
		Resources: []personalmemory.ResourceContext{
			{ResourceID: resourceIDs[0], CanonicalURL: "https://example.com/a"},
			{ResourceID: resourceIDs[1], CanonicalURL: "https://example.com/b"},
			{ResourceID: resourceIDs[2], CanonicalURL: "https://example.com/c"},
		},
	}}
	runner := Runner{Store: store, Repository: repository, Fetcher: partialFetcher{}}
	if err := runner.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	queue, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.batchSizes) != 2 || repository.batchSizes[0] != 1 || repository.batchSizes[1] != 2 {
		t.Fatalf("budget remainder was not one canonical batch: %#v", repository.batchSizes)
	}
	if queue.Items[0].State != StatePartial ||
		queue.Items[1].State != StateBlocked || queue.Items[1].Reason != ReasonRunBudgetDeferred ||
		queue.Items[2].State != StateBlocked || queue.Items[2].Reason != ReasonRunBudgetDeferred {
		t.Fatalf("budget remainder did not settle terminally: %+v", queue)
	}
}

type fixedUsageFetcher struct{ usage Usage }

func (fetcher fixedUsageFetcher) Fetch(context.Context, Target) (FetchResult, error) {
	return FetchResult{State: StatePartial, Usage: fetcher.usage}, nil
}

func TestEveryGlobalBudgetDimensionDefersAndContinuesExactlyOneGeneration(t *testing.T) {
	tests := []struct {
		name  string
		limit func(*BudgetProfile)
		usage Usage
	}{
		{name: "resources", limit: func(profile *BudgetProfile) { profile.MaxResources = 1 }},
		{name: "requests", limit: func(profile *BudgetProfile) { profile.MaxRequests = 1 }, usage: Usage{Requests: 2}},
		{name: "downloaded", limit: func(profile *BudgetProfile) { profile.MaxDownloadedBytes = 1 }, usage: Usage{DownloadedBytes: 2}},
		{name: "decoded", limit: func(profile *BudgetProfile) { profile.MaxDecodedBytes = 1 }, usage: Usage{DecodedBytes: 2}},
		{name: "extracted", limit: func(profile *BudgetProfile) { profile.MaxExtractedBytes = 1 }, usage: Usage{ExtractedBytes: 2}},
		{name: "runtime-storage", limit: func(profile *BudgetProfile) { profile.MaxRuntimeStorageBytes = 1 }, usage: Usage{RuntimeStorageBytes: 2}},
		{name: "wall", limit: func(profile *BudgetProfile) { profile.MaxRunWallSeconds = 1 }, usage: Usage{WallSeconds: 2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := FixtureProfile()
			profile.Name = "fixture-generation-" + test.name
			profile.MaxResources = 10
			test.limit(&profile)
			profile = SealProfile(profile)
			store, err := NewStore(t.TempDir()+"/queue", profile)
			if err != nil {
				t.Fatal(err)
			}
			resourceIDs := []string{"resource-a", "resource-b", "resource-c"}
			if _, err := store.Enqueue(resourceIDs); err != nil {
				t.Fatal(err)
			}
			repository := &batchRecordingRepository{library: personalmemory.Library{
				Resources: []personalmemory.ResourceContext{
					{ResourceID: resourceIDs[0], CanonicalURL: "https://example.com/a"},
					{ResourceID: resourceIDs[1], CanonicalURL: "https://example.com/b"},
					{ResourceID: resourceIDs[2], CanonicalURL: "https://example.com/c"},
				},
			}}
			if err := (Runner{Store: store, Repository: repository, Fetcher: fixedUsageFetcher{usage: test.usage}}).Drain(context.Background()); err != nil {
				t.Fatal(err)
			}
			before, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			beforeDeferred, beforePartial := queueOutcomeCounts(before)
			if beforeDeferred == 0 {
				t.Fatalf("%s cap did not preserve deferred work: %+v", test.name, before)
			}
			if test.name == "resources" {
				if beforePartial != 1 || beforeDeferred != 2 {
					t.Fatalf("resource cap outcomes = partial:%d deferred:%d", beforePartial, beforeDeferred)
				}
			} else if beforePartial != 0 || beforeDeferred != 3 {
				t.Fatalf("%s aggregate overage did not defer current and remainder: partial:%d deferred:%d", test.name, beforePartial, beforeDeferred)
			}
			if _, started, err := store.StartNextGeneration(); err != nil || !started {
				t.Fatalf("start %s continuation = %v %v", test.name, started, err)
			}
			if err := (Runner{Store: store, Repository: repository, Fetcher: fixedUsageFetcher{}}).Drain(context.Background()); err != nil {
				t.Fatal(err)
			}
			after, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			afterDeferred, afterPartial := queueOutcomeCounts(after)
			if after.Generation != 1 || afterPartial <= beforePartial || afterDeferred >= beforeDeferred {
				t.Fatalf("%s continuation did not progress exactly one generation: before=%+v after=%+v", test.name, before, after)
			}
			if after.Counters.ProcessedResources > profile.MaxResources ||
				after.Counters.Requests > profile.MaxRequests ||
				after.Counters.DownloadedBytes > profile.MaxDownloadedBytes ||
				after.Counters.DecodedBytes > profile.MaxDecodedBytes ||
				after.Counters.ExtractedBytes > profile.MaxExtractedBytes ||
				after.Counters.RuntimeStorageBytes > profile.MaxRuntimeStorageBytes ||
				after.Counters.WallSeconds > profile.MaxRunWallSeconds {
				t.Fatalf("%s continuation crossed frozen profile: %+v", test.name, after.Counters)
			}
		})
	}
}

func TestAggregateCapCrashDefersRecoveredAttemptAndRemainderWithinNextGeneration(t *testing.T) {
	profile := FixtureProfile()
	profile.Name = "fixture-decoded-crash-continuation"
	profile.MaxResources = 10
	profile.MaxDecodedBytes = 1
	profile = SealProfile(profile)
	store, err := NewStore(t.TempDir()+"/queue", profile)
	if err != nil {
		t.Fatal(err)
	}
	resourceIDs := []string{"resource-a", "resource-b"}
	if _, err := store.Enqueue(resourceIDs); err != nil {
		t.Fatal(err)
	}
	claimed, found, err := store.ClaimNext()
	if err != nil || !found || claimed.Attempts != 1 {
		t.Fatalf("crash claim = %+v %v %v", claimed, found, err)
	}
	if exhausted, err := store.Consume(claimed.ResourceID, Usage{DecodedBytes: 1}); err != nil || exhausted {
		t.Fatalf("exact cap consume = exhausted:%v err:%v", exhausted, err)
	}
	repository := &batchRecordingRepository{library: personalmemory.Library{
		Resources: []personalmemory.ResourceContext{
			{ResourceID: resourceIDs[0], CanonicalURL: "https://example.com/a"},
			{ResourceID: resourceIDs[1], CanonicalURL: "https://example.com/b"},
		},
	}}
	if err := (Runner{Store: store, Repository: repository, Fetcher: fixedUsageFetcher{}}).Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	crashed, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	deferred, partial := queueOutcomeCounts(crashed)
	if deferred != 2 || partial != 0 || crashed.Counters.DecodedBytes != profile.MaxDecodedBytes {
		t.Fatalf("crash recovery escaped exhausted generation: %+v", crashed)
	}
	if _, started, err := store.StartNextGeneration(); err != nil || !started {
		t.Fatalf("start recovered generation = %v %v", started, err)
	}
	if err := (Runner{Store: store, Repository: repository, Fetcher: fixedUsageFetcher{}}).Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	resumed, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	deferred, partial = queueOutcomeCounts(resumed)
	if resumed.Generation != 1 || deferred != 0 || partial != 2 ||
		resumed.Counters.DecodedBytes > profile.MaxDecodedBytes {
		t.Fatalf("recovered generation did not finish safely: %+v", resumed)
	}
	for _, item := range resumed.Items {
		if item.ResourceID == claimed.ResourceID && item.Attempts != 2 {
			t.Fatalf("recovered attempt was not bounded and retained: %+v", item)
		}
	}
}

func queueOutcomeCounts(queue Queue) (deferred, partial int) {
	for _, item := range queue.Items {
		if item.State == StateBlocked && item.Reason == ReasonRunBudgetDeferred {
			deferred++
		}
		if item.State == StatePartial {
			partial++
		}
	}
	return deferred, partial
}

func TestStartNextGenerationMigratesOnlyNeverAttemptedLegacyRemainder(t *testing.T) {
	profile := FixtureProfile()
	store, err := NewStore(t.TempDir()+"/queue", profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue([]string{"legacy-remainder", "attempted-budget", "deferred", "other-terminal"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.update(func(queue *Queue) error {
		queue.Counters = Counters{
			ProcessedResources: 3, Requests: 4, Attempts: 5,
			DownloadedBytes: 6, DecodedBytes: 7, ExtractedBytes: 8,
			RuntimeStorageBytes: 9, WallSeconds: 10,
		}
		for index := range queue.Items {
			item := &queue.Items[index]
			item.State = StateBlocked
			switch item.ResourceID {
			case "legacy-remainder":
				item.Reason = ReasonBudgetExhausted
			case "attempted-budget":
				item.Reason, item.Attempts = ReasonBudgetExhausted, 1
			case "deferred":
				item.Reason = ReasonRunBudgetDeferred
			case "other-terminal":
				item.Reason = "unreachable"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	queue, started, err := store.StartNextGeneration()
	if err != nil || !started {
		t.Fatalf("start generation = started:%v err:%v", started, err)
	}
	if queue.Generation != 1 || queue.Counters != (Counters{}) || !queue.LegacyBudgetMigrationComplete {
		t.Fatalf("generation did not reset exactly: %+v", queue)
	}
	states := map[string]string{}
	for _, item := range queue.Items {
		states[item.ResourceID] = item.State + ":" + item.Reason
	}
	if states["legacy-remainder"] != StateQueued+":" || states["deferred"] != StateQueued+":" ||
		states["attempted-budget"] != StateBlocked+":"+ReasonBudgetExhausted ||
		states["other-terminal"] != StateBlocked+":unreachable" {
		t.Fatalf("legacy migration selected the wrong items: %#v", states)
	}
	replayed, started, err := store.StartNextGeneration()
	if err != nil || started || replayed.Generation != 1 || replayed.Fingerprint != queue.Fingerprint {
		t.Fatalf("queued generation replay was not a no-op: started:%v queue:%+v err:%v", started, replayed, err)
	}
}

func TestStartNextGenerationIsNoOpWithoutDeferredWork(t *testing.T) {
	store, err := NewStore(t.TempDir()+"/queue", FixtureProfile())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue([]string{"attempted-budget", "terminal"}); err != nil {
		t.Fatal(err)
	}
	before, err := store.update(func(queue *Queue) error {
		queue.Items[0].State, queue.Items[0].Reason, queue.Items[0].Attempts = StateBlocked, ReasonBudgetExhausted, 1
		queue.Items[1].State, queue.Items[1].Reason = StateBlocked, "unreachable"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	after, started, err := store.StartNextGeneration()
	if err != nil || started || after.Generation != 0 || after.Fingerprint != before.Fingerprint {
		t.Fatalf("terminal no-op changed queue: started:%v before:%+v after:%+v err:%v", started, before, after, err)
	}
}

func TestRetryGenerationSelectsOnlyApprovedTransientReasonAndReplaysSafely(t *testing.T) {
	store, err := NewStore(t.TempDir()+"/queue", FixtureProfile())
	if err != nil {
		t.Fatal(err)
	}
	items := []RebuildItem{
		{ResourceID: "unreachable", State: StateBlocked, Reason: "unreachable"},
		{ResourceID: "rate", State: StateBlocked, Reason: "rate_limited"},
		{ResourceID: "access", State: StateBlocked, Reason: "access_denied"},
		{ResourceID: "unsafe", State: StateBlocked, Reason: "unsafe_network_target"},
		{ResourceID: "manual", State: StateBlocked, Reason: "manual_processing_required"},
		{ResourceID: "unsupported", State: StateBlocked, Reason: "unsupported_mime"},
	}
	if _, err := store.Rebuild(items); err != nil {
		t.Fatal(err)
	}
	if _, err := store.update(func(queue *Queue) error {
		queue.Counters = Counters{ProcessedResources: 4, Requests: 5}
		for index := range queue.Items {
			queue.Items[index].Attempts = 3
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	queue, started, err := store.StartRetryGeneration("unreachable")
	if err != nil || !started || queue.Generation != 1 ||
		queue.GenerationKind != "retry:unreachable" || queue.Counters != (Counters{}) {
		t.Fatalf("retry generation = started:%v queue:%+v err:%v", started, queue, err)
	}
	for _, item := range queue.Items {
		if item.ResourceID == "unreachable" {
			if item.State != StateQueued || item.Reason != "" || item.Attempts != 0 {
				t.Fatalf("approved retry was not reset: %+v", item)
			}
			continue
		}
		if item.State != StateBlocked || item.Attempts != 3 {
			t.Fatalf("non-selected terminal changed: %+v", item)
		}
	}
	if _, _, err := store.StartRetryGeneration("unreachable"); err == nil {
		t.Fatal("active retry generation was not refused")
	}
	completed, err := store.update(func(queue *Queue) error {
		for index := range queue.Items {
			if queue.Items[index].ResourceID == "unreachable" {
				queue.Items[index].State = StateBlocked
				queue.Items[index].Reason = "unreachable"
				queue.Items[index].Attempts = 1
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, started, err := store.StartRetryGeneration("unreachable")
	if err != nil || started || replayed.Generation != 1 || replayed.Fingerprint != completed.Fingerprint {
		t.Fatalf("completed retry replay changed queue: started:%v queue:%+v err=%v", started, replayed, err)
	}
	for _, reason := range []string{"access_denied", "unsafe_network_target", "sensitive_or_ambiguous", "manual_processing_required", "unsupported_mime"} {
		if _, _, err := store.StartRetryGeneration(reason); err == nil {
			t.Fatalf("permanent reason %q became retryable", reason)
		}
	}
}

func TestSyncMembershipPrunesLegacyJobsWithoutResettingRetainedRunState(t *testing.T) {
	store, err := NewStore(t.TempDir()+"/queue", FixtureProfile())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rebuild([]RebuildItem{
		{ResourceID: "keep", State: StateBlocked, Reason: ReasonRunBudgetDeferred},
		{ResourceID: "remove-deferred", State: StateBlocked, Reason: ReasonRunBudgetDeferred},
		{ResourceID: "remove-processing", State: StateQueued},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := store.update(func(queue *Queue) error {
		queue.Generation = 4
		queue.GenerationKind = "continuation"
		queue.Counters = Counters{
			ProcessedResources: 2, Requests: 7, Attempts: 3,
			ReservedRequests: 4, DownloadedBytes: 11, DecodedBytes: 12,
			ExtractedBytes: 13, RuntimeStorageBytes: 14, WallSeconds: 15,
		}
		for index := range queue.Items {
			switch queue.Items[index].ResourceID {
			case "keep":
				queue.Items[index].Attempts = 2
			case "remove-processing":
				queue.Items[index].State = StateProcessing
				queue.Items[index].Attempts = 1
				queue.Items[index].ReservedRequests = 4
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.SyncMembership([]string{"keep", "new"})
	if err != nil {
		t.Fatal(err)
	}
	expectedCounters := before.Counters
	expectedCounters.ReservedRequests = 0
	if after.Generation != before.Generation ||
		after.GenerationKind != before.GenerationKind ||
		after.Counters != expectedCounters || len(after.Items) != 2 {
		t.Fatalf("membership sync reset run state: before=%+v after=%+v", before, after)
	}
	for _, item := range after.Items {
		switch item.ResourceID {
		case "keep":
			if item.State != StateBlocked || item.Reason != ReasonRunBudgetDeferred ||
				item.Attempts != 2 {
				t.Fatalf("retained item changed: %+v", item)
			}
		case "new":
			if item.State != StateQueued || item.Attempts != 0 {
				t.Fatalf("new processable item was not queued: %+v", item)
			}
		default:
			t.Fatalf("removed legacy item survived: %+v", item)
		}
	}
}

type cancelingUsageFetcher struct{ cancel context.CancelFunc }

func (fetcher cancelingUsageFetcher) Fetch(context.Context, Target) (FetchResult, error) {
	fetcher.cancel()
	return FetchResult{Usage: Usage{Requests: 1, DecodedBytes: 1}}, context.Canceled
}

func TestCanceledFetchSettlesUsageAndRequeuesWithoutCanonicalFailure(t *testing.T) {
	store, err := NewStore(t.TempDir()+"/queue", FixtureProfile())
	if err != nil {
		t.Fatal(err)
	}
	resourceID := "resource-canceled"
	if _, err := store.Enqueue([]string{resourceID}); err != nil {
		t.Fatal(err)
	}
	repository := &batchRecordingRepository{library: personalmemory.Library{
		Resources: []personalmemory.ResourceContext{{
			ResourceID: resourceID, CanonicalURL: "https://example.com/canceled",
		}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	err = (Runner{Store: store, Repository: repository, Fetcher: cancelingUsageFetcher{cancel: cancel}}).Drain(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled drain err=%v", err)
	}
	queue, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Items) != 1 || queue.Items[0].State != StateQueued ||
		queue.Items[0].Reason != "" || queue.Items[0].Attempts != 1 ||
		queue.Counters.Requests != 1 || queue.Counters.DecodedBytes != 1 ||
		len(repository.batchSizes) != 0 {
		t.Fatalf("canceled fetch persisted failure or lost usage: queue=%+v merges=%v", queue, repository.batchSizes)
	}
	if err := (Runner{Store: store, Repository: repository, Fetcher: fixedUsageFetcher{}}).Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	queue, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if queue.Items[0].State != StatePartial || queue.Items[0].Attempts != 2 || len(repository.batchSizes) != 1 {
		t.Fatalf("recovered canceled fetch did not finish once: queue=%+v merges=%v", queue, repository.batchSizes)
	}
}

func TestPreCanceledDrainDoesNotClaimWork(t *testing.T) {
	store, err := NewStore(t.TempDir()+"/queue", FixtureProfile())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue([]string{"resource-a"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = (Runner{Store: store, Repository: &batchRecordingRepository{}, Fetcher: fixedUsageFetcher{}}).Drain(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled drain err=%v", err)
	}
	queue, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if queue.Items[0].State != StateQueued || queue.Items[0].Attempts != 0 || queue.Counters != (Counters{}) {
		t.Fatalf("pre-canceled drain claimed work: %+v", queue)
	}
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
