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
