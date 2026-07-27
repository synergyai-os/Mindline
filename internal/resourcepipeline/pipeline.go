// Package resourcepipeline binds one sealed resource-queue run to the public
// fetch boundary. It deliberately exposes only structural state; canonical
// evidence remains owned by personalmemory.
package resourcepipeline

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/resourcefetch"
	"github.com/synergyai-os/Mindline/internal/resourcequeue"
	"github.com/synergyai-os/Mindline/internal/routing"
)

// FetchPort makes the network boundary replaceable in a network-free proof.
// The policy is constructed freshly from the sealed profile and dynamically
// narrowed to the reserved request allowance for this target.
type FetchPort interface {
	Fetch(context.Context, string, resourcefetch.FrozenPolicy) resourcefetch.Result
}

type liveFetchPort struct{ dependencies resourcefetch.Dependencies }

func (port liveFetchPort) Fetch(ctx context.Context, url string, policy resourcefetch.FrozenPolicy) resourcefetch.Result {
	fetcher, err := resourcefetch.NewWithPolicy(port.dependencies, policy)
	if err != nil {
		return resourcefetch.Result{State: "blocked", Reason: resourcefetch.ReasonBudgetExhausted, PolicyFingerprint: policy.Fingerprint}
	}
	return fetcher.Fetch(ctx, url)
}

// ProfilePolicy is the single profile-to-fetch-policy projection. Its
// fingerprint includes the complete sealed budget fingerprint, preventing a
// different request timeout, redirect count, or byte limit from being used.
func ProfilePolicy(profile resourcequeue.BudgetProfile) (resourcefetch.FrozenPolicy, error) {
	if err := resourcequeue.ValidateProfile(profile); err != nil {
		return resourcefetch.FrozenPolicy{}, err
	}
	policy := profile.FetchPolicy
	return resourcefetch.FrozenPolicy{
		Fingerprint:              "resourcequeue-fetch/" + profile.Fingerprint,
		RequestTimeout:           time.Duration(policy.RequestTimeoutSeconds) * time.Second,
		MaximumRedirects:         policy.MaxRedirects,
		MaximumWireBytes:         policy.MaxWireBytes,
		MaximumDecodedBytes:      policy.MaxDecodedBytes,
		MaximumExtractedBytes:    int(policy.MaxExtractedBytes),
		MaximumRetryAfterSeconds: int(policy.MaxRetryAfterSeconds),
	}, nil
}

type Pipeline struct {
	Store      *resourcequeue.Store
	Repository resourcequeue.Repository
	Profile    resourcequeue.BudgetProfile
	Port       FetchPort
	Sleep      func(context.Context, time.Duration) error
	Now        func() time.Time
}

func New(root string, repository resourcequeue.Repository, profile resourcequeue.BudgetProfile, port FetchPort) (*Pipeline, error) {
	if repository == nil || port == nil {
		return nil, errors.New("resource pipeline is incomplete")
	}
	store, err := resourcequeue.NewStore(root, profile)
	if err != nil {
		return nil, err
	}
	return &Pipeline{Store: store, Repository: repository, Profile: profile, Port: port}, nil
}

func NewLive(root string, repository resourcequeue.Repository, profile resourcequeue.BudgetProfile, dependencies resourcefetch.Dependencies) (*Pipeline, error) {
	return New(root, repository, profile, liveFetchPort{dependencies: dependencies})
}

// EnqueueCurrent adds each safe canonical current resource once. Queue-level
// deterministic IDs make repeated calls idempotent; no URL is persisted here.
func (pipeline *Pipeline) EnqueueCurrent() error {
	if pipeline == nil || pipeline.Store == nil || pipeline.Repository == nil {
		return errors.New("resource pipeline is incomplete")
	}
	library, err := pipeline.Repository.Load()
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(library.Resources))
	for _, resource := range library.Resources {
		safe, state, err := routing.PrepareURLForStorage(resource.CanonicalURL)
		if err != nil || state == routing.URLStorageSensitiveRedacted || safe != resource.CanonicalURL {
			continue
		}
		ids = append(ids, resource.ResourceID)
	}
	sort.Strings(ids)
	_, err = pipeline.Store.Enqueue(ids)
	return err
}

// RebuildCurrent derives operational queue state only from canonical resource
// evidence. Terminal outcomes remain terminal; resources never attempted are
// queued. No network call or private URL is persisted by the queue.
func (pipeline *Pipeline) RebuildCurrent() error {
	if pipeline == nil || pipeline.Store == nil || pipeline.Repository == nil {
		return errors.New("resource pipeline is incomplete")
	}
	library, err := pipeline.Repository.Load()
	if err != nil {
		return err
	}
	items := make([]resourcequeue.RebuildItem, 0, len(library.Resources))
	for _, resource := range library.Resources {
		safe, storageState, err := routing.PrepareURLForStorage(resource.CanonicalURL)
		if err != nil || storageState == routing.URLStorageSensitiveRedacted || safe != resource.CanonicalURL {
			continue
		}
		state, reason := derivedQueueState(resource)
		items = append(items, resourcequeue.RebuildItem{ResourceID: resource.ResourceID, State: state, Reason: reason})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ResourceID < items[j].ResourceID })
	_, err = pipeline.Store.Rebuild(items)
	return err
}

func derivedQueueState(resource personalmemory.ResourceContext) (state, reason string) {
	switch resource.State {
	case "complete":
		return resourcequeue.StateComplete, ""
	case "partial":
		return resourcequeue.StatePartial, ""
	case "failed", "inaccessible":
		for _, missing := range resource.Missingness {
			const prefix = "resource_blocked:"
			if strings.HasPrefix(missing, prefix) && resourcequeue.IsBlockedReason(strings.TrimPrefix(missing, prefix)) {
				return resourcequeue.StateBlocked, strings.TrimPrefix(missing, prefix)
			}
		}
		return resourcequeue.StateBlocked, "manual_processing_required"
	default:
		return resourcequeue.StateQueued, ""
	}
}

func (pipeline *Pipeline) Recover(ctx context.Context) error {
	if pipeline == nil {
		return errors.New("resource pipeline is incomplete")
	}
	return pipeline.runner().Recover()
}

func (pipeline *Pipeline) Run(ctx context.Context) error {
	if pipeline == nil || pipeline.Store == nil || pipeline.Repository == nil {
		return errors.New("resource pipeline is incomplete")
	}
	queue, err := pipeline.Store.Load()
	if err != nil {
		return err
	}
	if len(queue.Items) == 0 {
		err = pipeline.RebuildCurrent()
	} else {
		err = pipeline.EnqueueCurrent()
	}
	if err != nil {
		return err
	}
	return pipeline.runner().Drain(ctx)
}

// Continue resumes an interrupted generation or starts exactly one next
// generation after the prior queue is terminal. Each generation uses the same
// sealed profile and is drained only to its own bounded terminal state.
func (pipeline *Pipeline) Continue(ctx context.Context) error {
	if pipeline == nil || pipeline.Store == nil || pipeline.Repository == nil {
		return errors.New("resource pipeline is incomplete")
	}
	queue, err := pipeline.Store.Load()
	if err != nil {
		return err
	}
	if len(queue.Items) == 0 {
		err = pipeline.RebuildCurrent()
	} else {
		err = pipeline.EnqueueCurrent()
	}
	if err != nil {
		return err
	}
	runner := pipeline.runner()
	if err := runner.Recover(); err != nil {
		return err
	}
	if _, _, err := pipeline.Store.StartNextGeneration(); err != nil {
		return err
	}
	return runner.Drain(ctx)
}

func (pipeline *Pipeline) runner() resourcequeue.Runner {
	return resourcequeue.Runner{Store: pipeline.Store, Repository: pipeline.Repository, Fetcher: queueFetcher{profile: pipeline.Profile, port: pipeline.Port}, Sleep: pipeline.Sleep, Now: pipeline.Now}
}

type queueFetcher struct {
	profile resourcequeue.BudgetProfile
	port    FetchPort
}

func (fetcher queueFetcher) Fetch(ctx context.Context, target resourcequeue.Target) (resourcequeue.FetchResult, error) {
	policy, err := ProfilePolicy(fetcher.profile)
	if err != nil {
		return resourcequeue.FetchResult{}, err
	}
	if target.Remaining.Requests < 1 {
		return resourcequeue.FetchResult{State: "blocked", BlockedReason: "budget_exhausted"}, nil
	}
	if target.Remaining.DownloadedBytes < 1 || target.Remaining.DecodedBytes < 1 ||
		target.Remaining.ExtractedBytes < 1 || target.Remaining.RuntimeStorageBytes < 1 ||
		target.Remaining.WallSeconds < 1 {
		return resourcequeue.FetchResult{State: "blocked", BlockedReason: "budget_exhausted"}, nil
	}
	maxRedirects := target.Remaining.Requests - 1
	if maxRedirects < policy.MaximumRedirects {
		policy.MaximumRedirects = maxRedirects
	}
	policy.MaximumWireBytes = minimumInt64(policy.MaximumWireBytes, target.Remaining.DownloadedBytes)
	policy.MaximumDecodedBytes = minimumInt64(policy.MaximumDecodedBytes, target.Remaining.DecodedBytes)
	if int64(policy.MaximumExtractedBytes) > target.Remaining.ExtractedBytes {
		policy.MaximumExtractedBytes = int(target.Remaining.ExtractedBytes)
	}
	if wallLimit := time.Duration(target.Remaining.WallSeconds) * time.Second; wallLimit < policy.RequestTimeout {
		policy.RequestTimeout = wallLimit
	}
	// The narrowed policy is bound to both the sealed profile and target
	// allowance. The port must echo it exactly or the result is rejected.
	policy.Fingerprint = policy.Fingerprint + "/requests-" + itoa(target.Remaining.Requests)
	result := fetcher.port.Fetch(ctx, target.CanonicalURL, policy)
	if result.PolicyFingerprint != policy.Fingerprint {
		return resourcequeue.FetchResult{}, errors.New("resource fetch policy fingerprint mismatch")
	}
	adapted := resourcequeue.FromResourcefetchResult(result)
	if adapted.Usage.Requests > target.Remaining.Requests {
		return resourcequeue.FetchResult{State: "blocked", BlockedReason: "budget_exhausted"}, nil
	}
	return adapted, nil
}

func minimumInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	bytes := [20]byte{}
	pos := len(bytes)
	for value > 0 {
		pos--
		bytes[pos] = byte('0' + value%10)
		value /= 10
	}
	return string(bytes[pos:])
}

type Status struct {
	SchemaVersion     string                 `json:"schema_version"`
	BudgetFingerprint string                 `json:"budget_fingerprint"`
	QueueFingerprint  string                 `json:"queue_fingerprint"`
	Generation        int                    `json:"generation"`
	DeferredCount     int                    `json:"deferred_count"`
	Counters          resourcequeue.Counters `json:"counters"`
	TerminalCounts    map[string]int         `json:"terminal_counts"`
	ReasonCounts      map[string]int         `json:"reason_counts"`
}

// StructuralStatus intentionally contains no resource IDs, URLs, paths, or
// fetched/canonical text, so it is safe for owner-only proof projection.
func (pipeline *Pipeline) StructuralStatus() (Status, error) {
	if pipeline == nil || pipeline.Store == nil {
		return Status{}, errors.New("resource pipeline is incomplete")
	}
	queue, err := pipeline.Store.Load()
	if err != nil {
		return Status{}, err
	}
	status := Status{SchemaVersion: "mindline-resource-pipeline-status/v0.2", BudgetFingerprint: queue.Profile.Fingerprint, QueueFingerprint: queue.Fingerprint, Generation: queue.Generation, Counters: queue.Counters, TerminalCounts: map[string]int{}, ReasonCounts: map[string]int{}}
	for _, item := range queue.Items {
		if item.State == resourcequeue.StateComplete || item.State == resourcequeue.StatePartial || item.State == resourcequeue.StateBlocked {
			status.TerminalCounts[item.State]++
		}
		if item.Reason != "" {
			status.ReasonCounts[item.Reason]++
		}
		if item.State == resourcequeue.StateBlocked && item.Reason == resourcequeue.ReasonRunBudgetDeferred {
			status.DeferredCount++
		}
	}
	return status, nil
}

func (pipeline *Pipeline) StructuralProof() (Status, error) { return pipeline.StructuralStatus() }

// CanonicalReadback contains already-sanitized hashes of three independent
// caller projections. It lets a host prove queue deletion/rebuild did not
// change canonical, compact, or explicit-get readback without exposing them.
type CanonicalReadback struct{ Canonical, Compact, Get string }

func (pipeline *Pipeline) DeleteAndRebuild(read func() (CanonicalReadback, error)) (CanonicalReadback, error) {
	if read == nil {
		return CanonicalReadback{}, errors.New("canonical readback is required")
	}
	before, err := read()
	if err != nil {
		return CanonicalReadback{}, err
	}
	if err := pipeline.Store.Delete(); err != nil {
		return CanonicalReadback{}, err
	}
	if err := pipeline.RebuildCurrent(); err != nil {
		return CanonicalReadback{}, err
	}
	after, err := read()
	if err != nil {
		return CanonicalReadback{}, err
	}
	if before != after {
		return CanonicalReadback{}, errors.New("derived queue rebuild changed canonical readback")
	}
	return after, nil
}

// Compile-time narrow dependency check; FileRepository satisfies the port.
var _ resourcequeue.Repository = (*personalmemory.FileRepository)(nil)
