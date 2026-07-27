package resourcepipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/resourcefetch"
	"github.com/synergyai-os/Mindline/internal/resourcequeue"
)

type fakeFetchPort struct {
	calls  int
	policy resourcefetch.FrozenPolicy
}

func (port *fakeFetchPort) Fetch(_ context.Context, _ string, policy resourcefetch.FrozenPolicy) resourcefetch.Result {
	port.calls++
	port.policy = policy
	return resourcefetch.Result{State: "complete", PolicyFingerprint: policy.Fingerprint, RequestCount: 1, MediaType: "text/plain", Text: "network-free public context", WireBytes: 27, DecodedBytes: 27, ExtractedBytes: 27}
}

func TestPipelineEnqueueRunAndDerivedQueueRebuildPreserveReadback(t *testing.T) {
	root := t.TempDir()
	repository, err := personalmemory.NewFileRepository(root+"/library", func() time.Time { return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{SourceAdapter: "slack", SourceScopeID: "workspace", SourceContainerID: "self", ExternalID: "message-1", OccurredAt: "2026-07-27T12:00:00Z", SourceRef: "slack://workspace/self/message-1", RawText: "https://example.com/article", EditDeleteState: "original", Missingness: []string{"permalink_unavailable"}})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{SourceIdentity: "slack:workspace:self", LowerInclusive: "1", UpperInclusive: "1", Watermark: "1", DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	port := &fakeFetchPort{}
	pipeline, err := New(root+"/queue", repository, resourcequeue.FixtureProfile(), port)
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if port.calls != 1 || port.policy.MaximumRedirects != 3 {
		t.Fatalf("unexpected dynamic fetch policy: calls=%d policy=%#v", port.calls, port.policy)
	}
	status, err := pipeline.StructuralStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.TerminalCounts[resourcequeue.StateComplete] != 1 || status.Counters.ProcessedResources != 1 || status.Counters.Requests != 1 || status.BudgetFingerprint == "" {
		t.Fatalf("unexpected structural status: %#v", status)
	}

	readback := func() (CanonicalReadback, error) {
		library, err := repository.Load()
		if err != nil {
			return CanonicalReadback{}, err
		}
		if len(library.Resources) != 1 || library.Resources[0].State != "complete" {
			t.Fatalf("unexpected canonical resource: %#v", library.Resources)
		}
		compact := digest(library.Resources[0].ResourceID + "\x00" + library.Resources[0].State + "\x00" + join(library.Resources[0].Missingness))
		get := digest(library.Resources[0].ContentHash + "\x00" + library.Resources[0].State + "\x00" + join(library.Resources[0].Missingness))
		return CanonicalReadback{Canonical: library.Fingerprint, Compact: compact, Get: get}, nil
	}
	if _, err := pipeline.DeleteAndRebuild(readback); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.StructuralProof(); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Store.Delete(); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if port.calls != 1 {
		t.Fatalf("rebuilt terminal queue refetched canonical resource: calls=%d", port.calls)
	}
}

func TestPipelineRejectsPolicyFingerprintMismatch(t *testing.T) {
	profile := resourcequeue.FixtureProfile()
	policy, err := ProfilePolicy(profile)
	if err != nil {
		t.Fatal(err)
	}
	if policy.RequestTimeout != 20*time.Second || policy.MaximumRedirects != 3 || policy.MaximumWireBytes != 5<<20 || policy.MaximumDecodedBytes != 2<<20 || policy.MaximumExtractedBytes != 512<<10 || policy.MaximumRetryAfterSeconds != 60 {
		t.Fatalf("profile policy mismatch: %#v", policy)
	}
	port := FetchPortFunc(func(_ context.Context, _ string, _ resourcefetch.FrozenPolicy) resourcefetch.Result {
		return resourcefetch.Result{State: "blocked", Reason: "unreachable", PolicyFingerprint: "wrong"}
	})
	adapter := queueFetcher{profile: profile, port: port}
	if _, err := adapter.Fetch(context.Background(), resourcequeue.Target{CanonicalURL: "https://example.com", Remaining: resourcequeue.Usage{
		Requests: 1, DownloadedBytes: profile.MaxDownloadedBytes,
		DecodedBytes: profile.MaxDecodedBytes, ExtractedBytes: profile.MaxExtractedBytes,
		RuntimeStorageBytes: profile.MaxRuntimeStorageBytes, WallSeconds: profile.MaxRunWallSeconds,
	}}); err == nil {
		t.Fatal("mismatched fetch policy fingerprint was accepted")
	}
}

type budgetBlockedPort struct{ calls int }

func (port *budgetBlockedPort) Fetch(_ context.Context, _ string, policy resourcefetch.FrozenPolicy) resourcefetch.Result {
	port.calls++
	return resourcefetch.Result{
		State: "blocked", Reason: resourcefetch.ReasonBudgetExhausted,
		RequestCount: 1, PolicyFingerprint: policy.Fingerprint,
	}
}

func TestQueueFetcherDistinguishesNarrowedRunBudgetFromPerResponseBudget(t *testing.T) {
	profile := resourcequeue.FixtureProfile()
	base, err := ProfilePolicy(profile)
	if err != nil {
		t.Fatal(err)
	}
	full := resourcequeue.Usage{
		Requests:            base.MaximumRedirects + 1,
		DownloadedBytes:     base.MaximumWireBytes,
		DecodedBytes:        base.MaximumDecodedBytes,
		ExtractedBytes:      int64(base.MaximumExtractedBytes),
		RuntimeStorageBytes: int64(base.MaximumExtractedBytes),
		WallSeconds:         int64(base.RequestTimeout / time.Second),
	}
	port := &budgetBlockedPort{}
	fetcher := queueFetcher{profile: profile, port: port}
	result, err := fetcher.Fetch(context.Background(), resourcequeue.Target{CanonicalURL: "https://example.com/full", Remaining: full})
	if err != nil || result.BlockedReason != resourcequeue.ReasonBudgetExhausted {
		t.Fatalf("full per-response oversize was not terminal: %+v err=%v", result, err)
	}
	tests := []struct {
		name   string
		narrow func(*resourcequeue.Usage)
	}{
		{name: "requests", narrow: func(usage *resourcequeue.Usage) { usage.Requests-- }},
		{name: "downloaded", narrow: func(usage *resourcequeue.Usage) { usage.DownloadedBytes-- }},
		{name: "decoded", narrow: func(usage *resourcequeue.Usage) { usage.DecodedBytes-- }},
		{name: "extracted", narrow: func(usage *resourcequeue.Usage) { usage.ExtractedBytes-- }},
		{name: "runtime-storage", narrow: func(usage *resourcequeue.Usage) { usage.RuntimeStorageBytes-- }},
		{name: "wall", narrow: func(usage *resourcequeue.Usage) { usage.WallSeconds-- }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			remaining := full
			test.narrow(&remaining)
			result, err := fetcher.Fetch(context.Background(), resourcequeue.Target{
				CanonicalURL: "https://example.com/narrowed", Remaining: remaining,
			})
			if err != nil || result.BlockedReason != resourcequeue.ReasonRunBudgetDeferred {
				t.Fatalf("%s narrowed budget was not resumable: %+v err=%v", test.name, result, err)
			}
		})
	}
	zero := full
	zero.DecodedBytes = 0
	before := port.calls
	result, err = fetcher.Fetch(context.Background(), resourcequeue.Target{CanonicalURL: "https://example.com/exhausted", Remaining: zero})
	if err != nil || result.BlockedReason != resourcequeue.ReasonRunBudgetDeferred || port.calls != before {
		t.Fatalf("fully exhausted run budget called the port or became terminal: %+v calls=%d/%d err=%v", result, before, port.calls, err)
	}
}

func TestPipelineContinuationAdvancesOneBoundedGenerationAndClearsCanonicalDeferredMissingness(t *testing.T) {
	root := t.TempDir()
	repository, err := personalmemory.NewFileRepository(root+"/library", func() time.Time {
		return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "workspace", SourceContainerID: "self",
		ExternalID: "message-many", OccurredAt: "2026-07-27T12:00:00Z",
		SourceRef:       "slack://workspace/self/message-many",
		RawText:         "https://example.com/one https://example.com/two https://example.com/three",
		EditDeleteState: "original", Missingness: []string{"permalink_unavailable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "slack:workspace:self", LowerInclusive: "1", UpperInclusive: "1",
		Watermark: "1", DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	profile := resourcequeue.FixtureProfile()
	profile.MaxResources = 1
	profile = resourcequeue.SealProfile(profile)
	port := &fakeFetchPort{}
	pipeline, err := New(root+"/queue", repository, profile, port)
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertGenerationState(t, pipeline, repository, 0, 1, 2, profile)

	// The derived queue is disposable: canonical run_budget_deferred is enough
	// to reconstruct continuation eligibility without retrying true terminals.
	if err := pipeline.Store.Delete(); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.RebuildCurrent(); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := pipeline.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if countQueueReason(rebuilt, resourcequeue.ReasonRunBudgetDeferred) != 2 {
		t.Fatalf("rebuild lost deferred eligibility: %+v", rebuilt)
	}
	if err := pipeline.Continue(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertGenerationState(t, pipeline, repository, 1, 2, 1, profile)
	if port.calls != 2 {
		t.Fatalf("one continuation processed more than one bounded generation: calls=%d", port.calls)
	}
	if err := pipeline.Continue(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertGenerationState(t, pipeline, repository, 2, 3, 0, profile)
	if port.calls != 3 {
		t.Fatalf("final generation call count=%d", port.calls)
	}
	terminal, err := pipeline.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	before := terminal.Fingerprint
	if err := pipeline.Continue(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := pipeline.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Generation != 2 || after.Fingerprint != before || port.calls != 3 {
		t.Fatalf("continuation without deferred work was not idempotent: before=%s after=%+v calls=%d", before, after, port.calls)
	}
}

func TestPipelineContinuationRecoversCrashedGenerationWithoutOpeningAnother(t *testing.T) {
	root := t.TempDir()
	repository, err := personalmemory.NewFileRepository(root+"/library", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "workspace", SourceContainerID: "self",
		ExternalID: "message-crash", OccurredAt: "2026-07-27T12:00:00Z",
		SourceRef:       "slack://workspace/self/message-crash",
		RawText:         "https://example.com/first https://example.com/second",
		EditDeleteState: "original", Missingness: []string{"permalink_unavailable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "slack:workspace:self", LowerInclusive: "1", UpperInclusive: "1",
		Watermark: "1", DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	profile := resourcequeue.FixtureProfile()
	profile.MaxResources = 1
	profile = resourcequeue.SealProfile(profile)
	port := &fakeFetchPort{}
	pipeline, err := New(root+"/queue", repository, profile, port)
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, started, err := pipeline.Store.StartNextGeneration(); err != nil || !started {
		t.Fatalf("start crashed generation = %v %v", started, err)
	}
	if item, found, err := pipeline.Store.ClaimNext(); err != nil || !found || item.State != resourcequeue.StateProcessing {
		t.Fatalf("crash lease = %+v %v %v", item, found, err)
	}
	if err := pipeline.Continue(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := pipeline.StructuralStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Generation != 1 || status.DeferredCount != 0 || status.Counters.ProcessedResources > profile.MaxResources || port.calls != 2 {
		t.Fatalf("crash recovery opened an extra generation or crossed cap: %+v calls=%d", status, port.calls)
	}
}

func TestContinuePrunesStaleGenericThenAdoptsCuratedWorkIntoFreshGeneration(t *testing.T) {
	const (
		parentID = "a-parent"
	)
	curatedURL := "https://example.com/curated-target"
	curatedID := "resource-" + digest(curatedURL)[:24]
	genericURL := "https://example.com/stale-generic"
	genericID := "resource-" + digest(genericURL)[:24]
	repository := &reconcileRepository{library: personalmemory.Library{
		Records: []personalmemory.CaptureRecord{{
			ResourceIDs: []string{parentID},
		}},
		Resources: []personalmemory.ResourceContext{
			{
				ResourceID: parentID, CanonicalURL: "https://example.com/parent",
				State: "complete",
			},
			{
				ResourceID: genericID, CanonicalURL: genericURL,
				State: "not_attempted",
			},
		},
	}}
	profile := resourcequeue.FixtureProfile()
	profile.MaxResources = 1
	profile = resourcequeue.SealProfile(profile)
	port := &fakeFetchPort{}
	pipeline, err := New(t.TempDir()+"/queue", repository, profile, port)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Store.Rebuild([]resourcequeue.RebuildItem{
		{ResourceID: parentID, State: resourcequeue.StateQueued},
		{ResourceID: genericID, State: resourcequeue.StateQueued},
	}); err != nil {
		t.Fatal(err)
	}
	parent, found, err := pipeline.Store.ClaimNext()
	if err != nil || !found || parent.ResourceID != parentID {
		t.Fatalf("prior generation parent claim=%+v found=%v err=%v", parent, found, err)
	}
	if exhausted, err := pipeline.Store.Consume(parentID, resourcequeue.Usage{Requests: 1}); err != nil || exhausted {
		t.Fatalf("prior generation usage exhausted=%v err=%v", exhausted, err)
	}
	if err := pipeline.Store.Finish(parentID, resourcequeue.StateComplete, ""); err != nil {
		t.Fatal(err)
	}
	stale, found, err := pipeline.Store.ClaimNext()
	if err != nil || !found || stale.ResourceID != genericID ||
		stale.State != resourcequeue.StateBlocked ||
		stale.Reason != resourcequeue.ReasonRunBudgetDeferred {
		t.Fatalf("prior generation stale generic item=%+v found=%v err=%v", stale, found, err)
	}

	repository.library.Resources[0].RelatedURLs = []personalmemory.RelatedResource{
		{
			URL: curatedURL, Relation: "source_links_to",
			DiscoveryEvidenceRef: "curated-proof", SemanticallyRelevant: true,
		},
		{
			URL: genericURL, Relation: "source_links_to",
			DiscoveryEvidenceRef: "related-legacy", SemanticallyRelevant: true,
		},
	}
	repository.library.Resources = append(
		repository.library.Resources,
		personalmemory.ResourceContext{
			ResourceID: curatedID, CanonicalURL: curatedURL, State: "not_attempted",
		},
	)

	if err := pipeline.Continue(context.Background()); err != nil {
		t.Fatal(err)
	}
	queue, err := pipeline.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	status, err := pipeline.StructuralStatus()
	if err != nil {
		t.Fatal(err)
	}
	if queue.Generation != 1 || port.calls != 1 || status.DeferredCount != 0 {
		t.Fatalf("curated work missed fresh generation: queue=%+v calls=%d", queue, port.calls)
	}
	foundCurated := false
	for _, item := range queue.Items {
		if item.ResourceID == genericID {
			t.Fatalf("stale generic job survived pruning: %+v", item)
		}
		if item.ResourceID == curatedID {
			foundCurated = item.State == resourcequeue.StateComplete
		}
	}
	if !foundCurated {
		t.Fatalf("one continuation did not produce useful curated terminal: %+v", queue.Items)
	}
	beforeReplay := queue.Fingerprint
	if err := pipeline.Continue(context.Background()); err != nil {
		t.Fatal(err)
	}
	replayed, err := pipeline.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Generation != 1 || replayed.Fingerprint != beforeReplay || port.calls != 1 {
		t.Fatalf("continuation replay changed frozen state: before=%s after=%+v calls=%d", beforeReplay, replayed, port.calls)
	}
}

type discoveringFetchPort struct {
	calls int
}

func (port *discoveringFetchPort) Fetch(_ context.Context, _ string, policy resourcefetch.FrozenPolicy) resourcefetch.Result {
	port.calls++
	result := resourcefetch.Result{
		State: "complete", PolicyFingerprint: policy.Fingerprint, RequestCount: 1,
		MediaType: "text/plain", Text: "network-free public context",
		WireBytes: 27, DecodedBytes: 27, ExtractedBytes: 27,
	}
	if port.calls == 1 {
		result.RelatedURLs = []string{"https://example.com/discovered"}
	}
	return result
}

func TestPipelineContinuationRetainsButDoesNotFollowGenericExtractorReferences(t *testing.T) {
	root := t.TempDir()
	repository, err := personalmemory.NewFileRepository(root+"/library", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "workspace", SourceContainerID: "self",
		ExternalID: "message-discovery", OccurredAt: "2026-07-27T12:00:00Z",
		SourceRef:       "slack://workspace/self/message-discovery",
		RawText:         "https://example.com/one https://example.com/two",
		EditDeleteState: "original", Missingness: []string{"permalink_unavailable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "slack:workspace:self", LowerInclusive: "1", UpperInclusive: "1",
		Watermark: "1", DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	profile := resourcequeue.FixtureProfile()
	profile.MaxResources = 1
	profile = resourcequeue.SealProfile(profile)
	port := &discoveringFetchPort{}
	pipeline, err := New(root+"/queue", repository, profile, port)
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	library, err := repository.Load()
	if err != nil || len(library.Resources) != 2 {
		t.Fatalf("generic reference created a processable placeholder: resources=%d err=%v", len(library.Resources), err)
	}
	foundReference := false
	for _, resource := range library.Resources {
		for _, related := range resource.RelatedURLs {
			if related.DiscoveryEvidenceRef != "" && !related.SemanticallyRelevant {
				foundReference = true
			}
		}
	}
	if !foundReference {
		t.Fatal("generic reference provenance was not retained on its source")
	}
	if err := pipeline.Continue(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err := pipeline.StructuralStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Generation != 1 || port.calls != 2 || status.DeferredCount != 0 {
		t.Fatalf("generic reference entered continuation work: status=%+v calls=%d", status, port.calls)
	}
}

type reconcileRepository struct {
	library personalmemory.Library
}

func (repository *reconcileRepository) Load() (personalmemory.Library, error) {
	return repository.library, nil
}

func (repository *reconcileRepository) MergeEnrichment(batch personalmemory.EnrichmentBatch) (personalmemory.EnrichmentReceipt, error) {
	for _, input := range batch.Resources {
		for index := range repository.library.Resources {
			if repository.library.Resources[index].CanonicalURL != input.CanonicalURL {
				continue
			}
			repository.library.Resources[index].State = input.State
			repository.library.Resources[index].AccessClass = input.AccessClass
			repository.library.Resources[index].Missingness = append([]string(nil), input.Missingness...)
		}
	}
	return personalmemory.EnrichmentReceipt{DeclaredResources: len(batch.Resources)}, nil
}

func TestPipelineReconcilePrunesUnfollowableQueueAndTerminalizesOnlyPlaceholder(t *testing.T) {
	const genericURL = "https://example.com/generic"
	parentID := "resource-parent"
	genericID := "resource-" + digest(genericURL)[:24]
	acquiredID := "resource-acquired"
	unresolvedID := "resource-unresolved"
	repository := &reconcileRepository{library: personalmemory.Library{
		Records: []personalmemory.CaptureRecord{{ResourceIDs: []string{parentID}}},
		Resources: []personalmemory.ResourceContext{
			{
				ResourceID: parentID, CanonicalURL: "https://example.com/parent", State: "not_attempted",
				RelatedURLs: []personalmemory.RelatedResource{{
					URL: genericURL, Relation: "source_links_to",
					DiscoveryEvidenceRef: "related-legacy", SemanticallyRelevant: true,
				}},
			},
			{ResourceID: genericID, CanonicalURL: genericURL, State: "not_attempted"},
			{ResourceID: acquiredID, CanonicalURL: "https://example.com/acquired", State: "partial"},
			{ResourceID: unresolvedID, CanonicalURL: "https://example.com/unresolved", State: "not_attempted"},
		},
	}}
	pipeline, err := New(t.TempDir()+"/queue", repository, resourcequeue.FixtureProfile(), &fakeFetchPort{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.Store.Enqueue([]string{parentID, genericID, acquiredID}); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	queue, err := pipeline.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Items) != 1 || queue.Items[0].ResourceID != parentID ||
		queue.Items[0].State != resourcequeue.StateQueued || queue.GenerationKind != "" {
		t.Fatalf("reconciled queue retained unprocessable work: %+v", queue)
	}
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range library.Resources {
		switch resource.ResourceID {
		case genericID:
			if resource.State != "inaccessible" ||
				len(resource.Missingness) != 1 ||
				resource.Missingness[0] != "resource_blocked:manual_processing_required" {
				t.Fatalf("generic placeholder did not become honest terminal: %+v", resource)
			}
		case acquiredID:
			if resource.State != "partial" {
				t.Fatalf("acquired orphan evidence changed: %+v", resource)
			}
		case unresolvedID:
			if resource.State != "not_attempted" {
				t.Fatalf("unproven orphan placeholder was terminalized: %+v", resource)
			}
		case parentID:
			if len(resource.RelatedURLs) != 1 {
				t.Fatal("parent generic provenance was discarded")
			}
		}
	}
	status, err := pipeline.StructuralStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.CanonicalResourceCount != 4 ||
		status.ProcessableResourceCount != 1 ||
		status.QueueResourceCount != 1 ||
		status.LegacyGenericReferenceCount != 1 ||
		status.UnresolvedUnprocessableNotAttemptedCount != 1 ||
		status.AcquiredUnprocessableCount != 1 {
		t.Fatalf("reconciliation denominator is not structurally visible: %+v", status)
	}
	categorized := status.ProcessableResourceCount +
		status.LegacyGenericReferenceCount +
		status.UnresolvedUnprocessableNotAttemptedCount +
		status.AcquiredUnprocessableCount
	if categorized != status.CanonicalResourceCount {
		t.Fatalf("resource denominator does not reconcile: categorized=%d status=%+v", categorized, status)
	}
}

func TestRunAndContinuePruneEveryLegacyGenericJobBeforeFetch(t *testing.T) {
	for _, command := range []string{"run", "continue"} {
		for _, legacyState := range []string{"queued", "deferred", "processing"} {
			t.Run(command+"/"+legacyState, func(t *testing.T) {
				const (
					genericID = "a-legacy-generic"
					directID  = "z-direct"
				)
				repository := &reconcileRepository{library: personalmemory.Library{
					Records: []personalmemory.CaptureRecord{{ResourceIDs: []string{directID}}},
					Resources: []personalmemory.ResourceContext{
						{
							ResourceID: directID, CanonicalURL: "https://example.com/direct",
							State: "not_attempted",
							RelatedURLs: []personalmemory.RelatedResource{{
								URL: "https://example.com/generic", Relation: "source_links_to",
								DiscoveryEvidenceRef: "related-legacy", SemanticallyRelevant: true,
							}},
						},
						{
							ResourceID: genericID, CanonicalURL: "https://example.com/generic",
							State: "not_attempted",
						},
					},
				}}
				port := &fakeFetchPort{}
				pipeline, err := New(
					t.TempDir()+"/queue", repository,
					resourcequeue.FixtureProfile(), port,
				)
				if err != nil {
					t.Fatal(err)
				}
				direct := resourcequeue.RebuildItem{
					ResourceID: directID, State: resourcequeue.StateQueued,
				}
				generic := resourcequeue.RebuildItem{
					ResourceID: genericID, State: resourcequeue.StateQueued,
				}
				if legacyState == "deferred" {
					direct.State, direct.Reason = resourcequeue.StateBlocked, resourcequeue.ReasonRunBudgetDeferred
					generic.State, generic.Reason = resourcequeue.StateBlocked, resourcequeue.ReasonRunBudgetDeferred
				}
				if _, err := pipeline.Store.Rebuild([]resourcequeue.RebuildItem{generic, direct}); err != nil {
					t.Fatal(err)
				}
				if legacyState == "processing" {
					claimed, found, err := pipeline.Store.ClaimNext()
					if err != nil || !found || claimed.ResourceID != genericID ||
						claimed.State != resourcequeue.StateProcessing {
						t.Fatalf("legacy processing seed = %+v found=%v err=%v", claimed, found, err)
					}
				}

				switch command {
				case "run":
					err = pipeline.Run(context.Background())
				case "continue":
					err = pipeline.Continue(context.Background())
				}
				if err != nil {
					t.Fatal(err)
				}
				queue, err := pipeline.Store.Load()
				if err != nil {
					t.Fatal(err)
				}
				if len(queue.Items) != 1 || queue.Items[0].ResourceID != directID ||
					queue.Counters.ReservedRequests != 0 {
					t.Fatalf("legacy generic work survived or reservation leaked: %+v", queue)
				}
				expectedCalls := 1
				expectedState := resourcequeue.StateComplete
				if command == "run" && legacyState == "deferred" {
					expectedCalls = 0
					expectedState = resourcequeue.StateBlocked
				}
				if port.calls != expectedCalls || queue.Items[0].State != expectedState {
					t.Fatalf("direct work outcome=%+v calls=%d want calls=%d state=%s", queue.Items[0], port.calls, expectedCalls, expectedState)
				}
				library, err := repository.Load()
				if err != nil {
					t.Fatal(err)
				}
				for _, resource := range library.Resources {
					if resource.ResourceID == genericID && resource.State != "not_attempted" {
						t.Fatalf("implicit queue pruning changed canonical generic evidence: %+v", resource)
					}
				}
			})
		}
	}
}

func TestPipelineRetryRunsOnceAndCompletedReplayIsIdempotent(t *testing.T) {
	root := t.TempDir()
	repository, err := personalmemory.NewFileRepository(root+"/library", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "workspace", SourceContainerID: "self",
		ExternalID: "retry-message", OccurredAt: "2026-07-27T12:00:00Z",
		SourceRef: "slack://workspace/self/retry-message",
		RawText:   "https://example.com/retry", EditDeleteState: "original",
		Missingness: []string{"permalink_unavailable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "slack:workspace:self", LowerInclusive: "1", UpperInclusive: "1",
		Watermark: "1", DeclaredRecords: 1, Records: []personalmemory.CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.MergeEnrichment(personalmemory.EnrichmentBatch{
		SchemaVersion: personalmemory.EnrichmentBatchSchemaVersion,
		Resources: []acquisition.ImportedEvidence{{
			CanonicalURL: "https://example.com/retry", State: "failed", AccessClass: "public",
			Missingness: []string{"resource_blocked:unreachable"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	port := &fakeFetchPort{}
	pipeline, err := New(root+"/queue", repository, resourcequeue.FixtureProfile(), port)
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.RebuildCurrent(); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Retry(context.Background(), "unreachable"); err != nil {
		t.Fatal(err)
	}
	status, err := pipeline.StructuralStatus()
	if err != nil {
		t.Fatal(err)
	}
	if port.calls != 1 || status.Generation != 1 || status.GenerationKind != "retry:unreachable" ||
		status.TerminalCounts[resourcequeue.StateComplete] != 1 {
		t.Fatalf("retry generation did not finish exactly once: status=%+v calls=%d", status, port.calls)
	}
	if err := pipeline.Retry(context.Background(), "unreachable"); err != nil {
		t.Fatal(err)
	}
	if port.calls != 1 {
		t.Fatalf("completed retry replay fetched again: calls=%d", port.calls)
	}
}

func TestRetryRefusesDirectWorkAddedToInterruptedRetryAndContinueRecovers(t *testing.T) {
	const (
		retryID  = "a-retry"
		directID = "z-direct"
	)
	repository := &reconcileRepository{library: personalmemory.Library{
		Records: []personalmemory.CaptureRecord{{ResourceIDs: []string{retryID}}},
		Resources: []personalmemory.ResourceContext{{
			ResourceID: retryID, CanonicalURL: "https://example.com/retry",
			State: "failed", Missingness: []string{"resource_blocked:unreachable"},
		}},
	}}
	port := &fakeFetchPort{}
	pipeline, err := New(
		t.TempDir()+"/queue", repository,
		resourcequeue.FixtureProfile(), port,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.RebuildCurrent(); err != nil {
		t.Fatal(err)
	}
	if _, started, err := pipeline.Store.StartRetryGeneration("unreachable"); err != nil || !started {
		t.Fatalf("start retry = %v err=%v", started, err)
	}
	claimed, found, err := pipeline.Store.ClaimNext()
	if err != nil || !found || claimed.ResourceID != retryID {
		t.Fatalf("interrupted retry claim=%+v found=%v err=%v", claimed, found, err)
	}

	repository.library.Records = append(
		repository.library.Records,
		personalmemory.CaptureRecord{ResourceIDs: []string{directID}},
	)
	repository.library.Resources = append(
		repository.library.Resources,
		personalmemory.ResourceContext{
			ResourceID: directID, CanonicalURL: "https://example.com/direct",
			State: "not_attempted",
		},
	)
	if err := pipeline.EnqueueCurrent(); err != nil {
		t.Fatal(err)
	}
	mixed, err := pipeline.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if mixed.GenerationKind != "" || len(mixed.Items) != 2 {
		t.Fatalf("new direct work did not invalidate retry identity: %+v", mixed)
	}
	if err := pipeline.Retry(context.Background(), "unreachable"); err == nil {
		t.Fatal("retry adopted mixed unrelated active work")
	}
	if port.calls != 0 {
		t.Fatalf("refused mixed retry called fetch port: calls=%d", port.calls)
	}
	if err := pipeline.Continue(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered, err := pipeline.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if port.calls != 2 || len(recovered.Items) != 2 {
		t.Fatalf("continue did not recover mixed ordinary work: queue=%+v calls=%d", recovered, port.calls)
	}
	for _, item := range recovered.Items {
		if item.State != resourcequeue.StateComplete {
			t.Fatalf("continued mixed item did not finish: %+v", item)
		}
	}
}

func assertGenerationState(t *testing.T, pipeline *Pipeline, repository *personalmemory.FileRepository, generation, complete, deferred int, profile resourcequeue.BudgetProfile) {
	t.Helper()
	status, err := pipeline.StructuralStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Generation != generation || status.DeferredCount != deferred ||
		status.Counters.ProcessedResources > profile.MaxResources ||
		status.Counters.Requests > profile.MaxRequests ||
		status.Counters.DownloadedBytes > profile.MaxDownloadedBytes ||
		status.Counters.DecodedBytes > profile.MaxDecodedBytes ||
		status.Counters.ExtractedBytes > profile.MaxExtractedBytes ||
		status.Counters.RuntimeStorageBytes > profile.MaxRuntimeStorageBytes ||
		status.Counters.WallSeconds > profile.MaxRunWallSeconds {
		t.Fatalf("generation escaped frozen profile: %+v", status)
	}
	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	gotComplete, gotDeferred := 0, 0
	for _, resource := range library.Resources {
		if resource.State == "complete" {
			gotComplete++
		}
		for _, missing := range resource.Missingness {
			if missing == "resource_blocked:"+resourcequeue.ReasonRunBudgetDeferred {
				gotDeferred++
			}
		}
	}
	if gotComplete != complete || gotDeferred != deferred {
		t.Fatalf("canonical continuation state = complete:%d deferred:%d want complete:%d deferred:%d", gotComplete, gotDeferred, complete, deferred)
	}
}

func countQueueReason(queue resourcequeue.Queue, reason string) int {
	count := 0
	for _, item := range queue.Items {
		if item.State == resourcequeue.StateBlocked && item.Reason == reason {
			count++
		}
	}
	return count
}

type FetchPortFunc func(context.Context, string, resourcefetch.FrozenPolicy) resourcefetch.Result

func (fn FetchPortFunc) Fetch(ctx context.Context, url string, policy resourcefetch.FrozenPolicy) resourcefetch.Result {
	return fn(ctx, url, policy)
}

func join(values []string) string {
	result := ""
	for _, value := range values {
		result += "\x00" + value
	}
	return result
}
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
