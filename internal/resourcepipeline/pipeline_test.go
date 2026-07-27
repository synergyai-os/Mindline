package resourcepipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

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
