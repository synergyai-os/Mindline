// Package resourcepipeline binds one sealed resource-queue run to the public
// fetch boundary. It deliberately exposes only structural state; canonical
// evidence remains owned by personalmemory.
package resourcepipeline

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
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

// EnqueueCurrent atomically synchronizes the derived queue to the complete set
// of safe, currently processable resources. Legacy overbroad jobs are removed
// before any fetch can claim them; matching job state and consumed budgets are
// preserved.
func (pipeline *Pipeline) EnqueueCurrent() error {
	return pipeline.updateCurrentMembership(true)
}

// PruneCurrent removes unsafe or unprocessable derived jobs without adopting
// newly discovered work. This preserves Continue's ability to open the next
// bounded generation before useful new work enters the queue.
func (pipeline *Pipeline) PruneCurrent() error {
	return pipeline.updateCurrentMembership(false)
}

func (pipeline *Pipeline) updateCurrentMembership(addMissing bool) error {
	if pipeline == nil || pipeline.Store == nil || pipeline.Repository == nil {
		return errors.New("resource pipeline is incomplete")
	}
	library, err := pipeline.Repository.Load()
	if err != nil {
		return err
	}
	resources := processableResources(library)
	ids := make([]string, 0, len(resources))
	for _, resource := range resources {
		safe, state, err := routing.PrepareURLForStorage(resource.CanonicalURL)
		if err != nil || state == routing.URLStorageSensitiveRedacted || safe != resource.CanonicalURL {
			continue
		}
		ids = append(ids, resource.ResourceID)
	}
	sort.Strings(ids)
	if addMissing {
		_, err = pipeline.Store.SyncMembership(ids)
	} else {
		_, err = pipeline.Store.PruneMembership(ids)
	}
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
	resources := processableResources(library)
	items := make([]resourcequeue.RebuildItem, 0, len(resources))
	for _, resource := range resources {
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

func processableResources(library personalmemory.Library) []personalmemory.ResourceContext {
	byID := make(map[string]personalmemory.ResourceContext, len(library.Resources))
	for _, resource := range library.Resources {
		byID[resource.ResourceID] = resource
	}
	ids := personalmemory.ProcessableResourceIDs(library)
	resources := make([]personalmemory.ResourceContext, 0, len(ids))
	for _, resourceID := range ids {
		if resource, exists := byID[resourceID]; exists {
			resources = append(resources, resource)
		}
	}
	return resources
}

// Reconcile disposes overbroad derived work without network access. Existing
// unapproved reference-only placeholders become explicit fixed terminals in
// canonical evidence; acquired evidence is retained unchanged. The rebuilt
// queue contains only resources reachable from retained captures through
// explicitly followable relations.
func (pipeline *Pipeline) Reconcile(ctx context.Context) error {
	if pipeline == nil || pipeline.Store == nil || pipeline.Repository == nil {
		return errors.New("resource pipeline is incomplete")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	library, err := pipeline.Repository.Load()
	if err != nil {
		return err
	}
	processable := map[string]bool{}
	for _, resourceID := range personalmemory.ProcessableResourceIDs(library) {
		processable[resourceID] = true
	}
	genericTargets := map[string]bool{}
	for _, resourceID := range personalmemory.GenericExtractorReferenceTargetIDs(library) {
		genericTargets[resourceID] = true
	}
	referenceOnly := make([]acquisition.ImportedEvidence, 0)
	for _, resource := range library.Resources {
		if processable[resource.ResourceID] ||
			!genericTargets[resource.ResourceID] ||
			resource.State != "not_attempted" {
			continue
		}
		referenceOnly = append(referenceOnly, acquisition.ImportedEvidence{
			CanonicalURL: resource.CanonicalURL,
			State:        "inaccessible",
			AccessClass:  "unsupported",
			Missingness:  []string{"resource_blocked:manual_processing_required"},
		})
	}
	if len(referenceOnly) != 0 {
		if _, err := pipeline.Repository.MergeEnrichment(personalmemory.EnrichmentBatch{
			SchemaVersion: personalmemory.EnrichmentBatchSchemaVersion,
			Resources:     referenceOnly,
		}); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return pipeline.RebuildCurrent()
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
	if pipeline == nil || pipeline.Store == nil {
		return errors.New("resource pipeline is incomplete")
	}
	lock, err := pipeline.Store.AcquireOperationLock()
	if err != nil {
		return err
	}
	defer lock.Close()
	return pipeline.runner().Recover()
}

func (pipeline *Pipeline) Run(ctx context.Context) error {
	if pipeline == nil || pipeline.Store == nil || pipeline.Repository == nil {
		return errors.New("resource pipeline is incomplete")
	}
	lock, err := pipeline.Store.AcquireOperationLock()
	if err != nil {
		return err
	}
	defer lock.Close()
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
	lock, err := pipeline.Store.AcquireOperationLock()
	if err != nil {
		return err
	}
	defer lock.Close()
	queue, err := pipeline.Store.Load()
	if err != nil {
		return err
	}
	if len(queue.Items) == 0 {
		if err := pipeline.RebuildCurrent(); err != nil {
			return err
		}
	} else if err := pipeline.PruneCurrent(); err != nil {
		return err
	}
	runner := pipeline.runner()
	if err := runner.Recover(); err != nil {
		return err
	}
	if _, _, err := pipeline.Store.StartNextGeneration(); err != nil {
		return err
	}
	// Open the next generation before adopting resources discovered by the
	// prior one. Otherwise a terminal generation whose counters are already at
	// a cap immediately defers those discoveries and requires a second
	// continuation command before any useful work can resume.
	if err := pipeline.EnqueueCurrent(); err != nil {
		return err
	}
	return runner.Drain(ctx)
}

// Retry opens or resumes one bounded operator-authorized transient retry
// generation. Replaying the same completed retry is a no-op; unrelated active
// work is refused rather than adopted implicitly.
func (pipeline *Pipeline) Retry(ctx context.Context, reason string) error {
	if pipeline == nil || pipeline.Store == nil || pipeline.Repository == nil ||
		!resourcequeue.IsRetryableTerminalReason(reason) {
		return errors.New("resource retry is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lock, err := pipeline.Store.AcquireOperationLock()
	if err != nil {
		return err
	}
	defer lock.Close()
	runner := pipeline.runner()
	if err := runner.Recover(); err != nil {
		return err
	}
	queue, err := pipeline.Store.Load()
	if err != nil {
		return err
	}
	active := false
	for _, item := range queue.Items {
		if item.State == resourcequeue.StateQueued || item.State == resourcequeue.StateProcessing {
			active = true
			break
		}
	}
	kind := "retry:" + reason
	if active {
		if queue.GenerationKind != kind {
			return errors.New("resource queue has unrelated active work")
		}
		return runner.Drain(ctx)
	}
	if _, _, err := pipeline.Store.StartRetryGeneration(reason); err != nil {
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
		return resourcequeue.FetchResult{State: "blocked", BlockedReason: resourcequeue.ReasonRunBudgetDeferred}, nil
	}
	if target.Remaining.DownloadedBytes < 1 || target.Remaining.DecodedBytes < 1 ||
		target.Remaining.ExtractedBytes < 1 || target.Remaining.RuntimeStorageBytes < 1 ||
		target.Remaining.WallSeconds < 1 {
		return resourcequeue.FetchResult{State: "blocked", BlockedReason: resourcequeue.ReasonRunBudgetDeferred}, nil
	}
	narrowed := false
	decodedCapacityNarrowed := false
	extractedCapacityDimension := ""
	maxRedirects := target.Remaining.Requests - 1
	if maxRedirects < policy.MaximumRedirects {
		policy.MaximumRedirects = maxRedirects
		narrowed = true
	}
	if target.Remaining.DownloadedBytes < policy.MaximumWireBytes {
		policy.MaximumWireBytes = target.Remaining.DownloadedBytes
		narrowed = true
	}
	if target.Remaining.DecodedBytes < policy.MaximumDecodedBytes {
		policy.MaximumDecodedBytes = target.Remaining.DecodedBytes
		narrowed = true
		decodedCapacityNarrowed = true
	}
	if int64(policy.MaximumExtractedBytes) > target.Remaining.ExtractedBytes {
		policy.MaximumExtractedBytes = int(target.Remaining.ExtractedBytes)
		narrowed = true
		extractedCapacityDimension = resourcequeue.BudgetDimensionExtracted
	}
	if int64(policy.MaximumExtractedBytes) > target.Remaining.RuntimeStorageBytes {
		policy.MaximumExtractedBytes = int(target.Remaining.RuntimeStorageBytes)
		narrowed = true
		extractedCapacityDimension = resourcequeue.BudgetDimensionRuntimeStorage
	}
	if wallLimit := time.Duration(target.Remaining.WallSeconds) * time.Second; wallLimit < policy.RequestTimeout {
		policy.RequestTimeout = wallLimit
		narrowed = true
	}
	// The narrowed policy is bound to both the sealed profile and target
	// allowance. The port must echo it exactly or the result is rejected.
	policy.Fingerprint += "/requests-" + strconv.Itoa(target.Remaining.Requests) +
		"/wire-" + strconv.FormatInt(policy.MaximumWireBytes, 10) +
		"/decoded-" + strconv.FormatInt(policy.MaximumDecodedBytes, 10) +
		"/extracted-" + strconv.Itoa(policy.MaximumExtractedBytes) +
		"/wall-nanos-" + strconv.FormatInt(int64(policy.RequestTimeout), 10)
	result := fetcher.port.Fetch(ctx, target.CanonicalURL, policy)
	if result.PolicyFingerprint != policy.Fingerprint {
		return resourcequeue.FetchResult{}, errors.New("resource fetch policy fingerprint mismatch")
	}
	adapted := resourcequeue.FromResourcefetchResult(result)
	if narrowed && adapted.BlockedReason == resourcequeue.ReasonBudgetExhausted {
		adapted.BlockedReason = resourcequeue.ReasonRunBudgetDeferred
		switch adapted.ExhaustedBudgetDimension {
		case resourcequeue.BudgetDimensionDecoded:
			adapted.CloseGeneration = decodedCapacityNarrowed
		case resourcequeue.BudgetDimensionExtracted:
			if extractedCapacityDimension != "" {
				adapted.ExhaustedBudgetDimension = extractedCapacityDimension
				adapted.CloseGeneration = true
			}
		}
	}
	if adapted.Usage.Requests > target.Remaining.Requests {
		return resourcequeue.FetchResult{State: "blocked", BlockedReason: resourcequeue.ReasonRunBudgetDeferred}, nil
	}
	return adapted, nil
}

type Status struct {
	SchemaVersion                            string                 `json:"schema_version"`
	BudgetFingerprint                        string                 `json:"budget_fingerprint"`
	QueueFingerprint                         string                 `json:"queue_fingerprint"`
	Generation                               int                    `json:"generation"`
	GenerationKind                           string                 `json:"generation_kind,omitempty"`
	GenerationClosed                         bool                   `json:"generation_closed,omitempty"`
	CanonicalResourceCount                   int                    `json:"canonical_resource_count"`
	ProcessableResourceCount                 int                    `json:"processable_resource_count"`
	QueueResourceCount                       int                    `json:"queue_resource_count"`
	LegacyGenericReferenceCount              int                    `json:"legacy_generic_reference_count"`
	UnresolvedUnprocessableNotAttemptedCount int                    `json:"unresolved_unprocessable_not_attempted_count"`
	AcquiredUnprocessableCount               int                    `json:"acquired_unprocessable_count"`
	DeferredCount                            int                    `json:"deferred_count"`
	Counters                                 resourcequeue.Counters `json:"counters"`
	TerminalCounts                           map[string]int         `json:"terminal_counts"`
	ReasonCounts                             map[string]int         `json:"reason_counts"`
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
	library, err := pipeline.Repository.Load()
	if err != nil {
		return Status{}, err
	}
	processable := map[string]bool{}
	for _, resourceID := range personalmemory.ProcessableResourceIDs(library) {
		processable[resourceID] = true
	}
	genericTargets := map[string]bool{}
	for _, resourceID := range personalmemory.GenericExtractorReferenceTargetIDs(library) {
		genericTargets[resourceID] = true
	}
	status := Status{
		SchemaVersion:     "mindline-resource-pipeline-status/v0.2",
		BudgetFingerprint: queue.Profile.Fingerprint, QueueFingerprint: queue.Fingerprint,
		Generation: queue.Generation, GenerationKind: queue.GenerationKind,
		GenerationClosed:         queue.GenerationClosed,
		CanonicalResourceCount:   len(library.Resources),
		ProcessableResourceCount: len(processable), QueueResourceCount: len(queue.Items),
		Counters: queue.Counters, TerminalCounts: map[string]int{}, ReasonCounts: map[string]int{},
	}
	for _, resource := range library.Resources {
		if processable[resource.ResourceID] {
			continue
		}
		if genericTargets[resource.ResourceID] {
			status.LegacyGenericReferenceCount++
		} else if resource.State == "not_attempted" {
			status.UnresolvedUnprocessableNotAttemptedCount++
		} else {
			status.AcquiredUnprocessableCount++
		}
	}
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

type CanonicalReadbackPair struct {
	Before CanonicalReadback
	After  CanonicalReadback
}

func (pipeline *Pipeline) DeleteAndRebuild(read func() (CanonicalReadback, error)) (CanonicalReadbackPair, error) {
	if read == nil {
		return CanonicalReadbackPair{}, errors.New("canonical readback is required")
	}
	lock, err := pipeline.Store.AcquireOperationLock()
	if err != nil {
		return CanonicalReadbackPair{}, err
	}
	defer lock.Close()
	before, err := read()
	if err != nil {
		return CanonicalReadbackPair{}, err
	}
	if err := pipeline.Store.Delete(); err != nil {
		return CanonicalReadbackPair{}, err
	}
	if err := pipeline.RebuildCurrent(); err != nil {
		return CanonicalReadbackPair{}, err
	}
	after, err := read()
	if err != nil {
		return CanonicalReadbackPair{}, err
	}
	if before != after {
		return CanonicalReadbackPair{}, errors.New("derived queue rebuild changed canonical readback")
	}
	return CanonicalReadbackPair{Before: before, After: after}, nil
}

// Compile-time narrow dependency check; FileRepository satisfies the port.
var _ resourcequeue.Repository = (*personalmemory.FileRepository)(nil)
