// Package resourcequeue owns bounded, restart-safe *derived* resource work.
// It deliberately persists resource IDs and fixed reason codes only: URLs and
// fetched material remain canonical repository concerns.
package resourcequeue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

const (
	SchemaVersion       = "mindline-resource-queue/v0.1"
	BudgetSchemaVersion = "mindline-resource-budget/v0.1"

	StateQueued     = "queued"
	StateProcessing = "processing"
	StateComplete   = "complete"
	StatePartial    = "partial"
	StateBlocked    = "blocked"

	ReasonBudgetExhausted   = "budget_exhausted"
	ReasonRunBudgetDeferred = "run_budget_deferred"

	BudgetDimensionWire           = "wire"
	BudgetDimensionDecoded        = "decoded"
	BudgetDimensionExtracted      = "extracted"
	BudgetDimensionRuntimeStorage = "runtime_storage"

	// MaximumQueueItems matches the canonical personal-memory resource
	// denominator. MaxResources remains a per-generation processing budget.
	MaximumQueueItems       = 250_000
	MaximumRunWallSeconds   = 24 * 60 * 60
	maximumResourceIDBytes  = 64
	maximumProfileNameBytes = 64
)

var fixedBlockedReasons = map[string]bool{
	"sensitive_or_ambiguous":     true,
	"unsupported_scheme":         true,
	"unsupported_mime":           true,
	"access_denied":              true,
	"unreachable":                true,
	"rate_limited":               true,
	"unsafe_network_target":      true,
	ReasonBudgetExhausted:        true,
	ReasonRunBudgetDeferred:      true,
	"extractor_unsupported":      true,
	"manual_processing_required": true,
}

// BudgetProfile is versioned and fingerprinted with every queue. Fixture
// profiles are intentionally explicit so lowering a cap can never alter the
// live profile by accident.
type BudgetProfile struct {
	SchemaVersion          string      `json:"schema_version"`
	Name                   string      `json:"name"`
	MaxResources           int         `json:"max_resources"`
	MaxRequests            int         `json:"max_requests"`
	MaxDownloadedBytes     int64       `json:"max_downloaded_bytes"`
	MaxDecodedBytes        int64       `json:"max_decoded_bytes"`
	MaxExtractedBytes      int64       `json:"max_extracted_bytes"`
	MaxRuntimeStorageBytes int64       `json:"max_runtime_storage_bytes"`
	MaxAttemptsPerResource int         `json:"max_attempts_per_resource"`
	MaxRunWallSeconds      int64       `json:"max_run_wall_seconds"`
	GlobalConcurrency      int         `json:"global_concurrency"`
	PerHostConcurrency     int         `json:"per_host_concurrency"`
	FetchPolicy            FetchPolicy `json:"fetch_policy"`
	Fingerprint            string      `json:"fingerprint"`
}

// FetchPolicy freezes every numeric per-response and retry control used by
// the queue/fetch boundary. It is included in BudgetProfile.Fingerprint.
type FetchPolicy struct {
	RequestTimeoutSeconds  int64 `json:"request_timeout_seconds"`
	MaxRedirects           int   `json:"max_redirects"`
	MaxWireBytes           int64 `json:"max_wire_bytes"`
	MaxDecodedBytes        int64 `json:"max_decoded_bytes"`
	MaxExtractedBytes      int64 `json:"max_extracted_bytes"`
	RetryBackoffOneSeconds int64 `json:"retry_backoff_one_seconds"`
	RetryBackoffTwoSeconds int64 `json:"retry_backoff_two_seconds"`
	MaxRetryAfterSeconds   int64 `json:"max_retry_after_seconds"`
}

func LiveProfile() BudgetProfile {
	return SealProfile(BudgetProfile{
		SchemaVersion: BudgetSchemaVersion, Name: "live",
		MaxResources: 500, MaxRequests: 1000,
		MaxDownloadedBytes: 256 << 20, MaxDecodedBytes: 64 << 20, MaxExtractedBytes: 64 << 20,
		MaxRuntimeStorageBytes: 512 << 20, MaxAttemptsPerResource: 3,
		MaxRunWallSeconds: 45 * 60, GlobalConcurrency: 4, PerHostConcurrency: 1,
		FetchPolicy: FetchPolicy{RequestTimeoutSeconds: 20, MaxRedirects: 3, MaxWireBytes: 5 << 20, MaxDecodedBytes: 2 << 20, MaxExtractedBytes: 512 << 10, RetryBackoffOneSeconds: 1, RetryBackoffTwoSeconds: 3, MaxRetryAfterSeconds: 60},
	})
}

// FixtureProfile is a valid deterministic baseline; callers may derive a
// named profile and seal it to test one cap at a time.
func FixtureProfile() BudgetProfile {
	p := LiveProfile()
	p.Name = "fixture-primary"
	p.MaxResources = 3
	return SealProfile(p)
}

// FixtureProfiles exposes the named, independently fingerprinted one-cap
// profiles required by the reusable proof. Fake clocks/sizes make callers'
// tests deterministic; these values never affect LiveProfile.
func FixtureProfiles() map[string]BudgetProfile {
	profiles := map[string]BudgetProfile{}
	for name := range map[string]bool{"resource-count": true, "request-count": true, "download-bytes": true, "decoded-bytes": true, "extracted-bytes": true, "runtime-storage": true, "attempt-count": true, "wall-time": true} {
		profile := FixtureProfile()
		profile.Name = "fixture-" + name
		switch name {
		case "resource-count":
			profile.MaxResources = 1
		case "request-count":
			profile.MaxRequests = 1
		case "download-bytes":
			profile.MaxDownloadedBytes = 1
		case "decoded-bytes":
			profile.MaxDecodedBytes = 1
		case "extracted-bytes":
			profile.MaxExtractedBytes = 1
		case "runtime-storage":
			profile.MaxRuntimeStorageBytes = 1
		case "attempt-count":
			profile.MaxAttemptsPerResource = 1
		case "wall-time":
			profile.MaxRunWallSeconds = 1
		}
		profiles[profile.Name] = SealProfile(profile)
	}
	return profiles
}

func SealProfile(profile BudgetProfile) BudgetProfile {
	profile.SchemaVersion = BudgetSchemaVersion
	profile.Fingerprint = ""
	profile.Fingerprint = fingerprint(profile)
	return profile
}

func ValidateProfile(profile BudgetProfile) error {
	if profile.SchemaVersion != BudgetSchemaVersion || !validQueueText(profile.Name, maximumProfileNameBytes) ||
		profile.MaxResources < 1 || profile.MaxRequests < 1 ||
		profile.MaxDownloadedBytes < 1 || profile.MaxDecodedBytes < 1 || profile.MaxExtractedBytes < 1 || profile.MaxRuntimeStorageBytes < 1 ||
		profile.MaxAttemptsPerResource < 1 || profile.MaxRunWallSeconds < 1 ||
		profile.MaxRunWallSeconds > MaximumRunWallSeconds ||
		profile.GlobalConcurrency < 1 || profile.PerHostConcurrency < 1 ||
		profile.PerHostConcurrency > profile.GlobalConcurrency ||
		profile.FetchPolicy.RequestTimeoutSeconds < 1 || profile.FetchPolicy.MaxRedirects < 0 ||
		profile.FetchPolicy.MaxWireBytes < 1 || profile.FetchPolicy.MaxDecodedBytes < 1 || profile.FetchPolicy.MaxExtractedBytes < 1 ||
		profile.FetchPolicy.RetryBackoffOneSeconds < 1 || profile.FetchPolicy.RetryBackoffTwoSeconds < 1 || profile.FetchPolicy.MaxRetryAfterSeconds < 1 {
		return errors.New("invalid resource budget profile")
	}
	copy := profile
	copy.Fingerprint = ""
	if profile.Fingerprint == "" || profile.Fingerprint != fingerprint(copy) {
		return errors.New("resource budget profile fingerprint mismatch")
	}
	return nil
}

type Counters struct {
	ProcessedResources  int   `json:"processed_resources"`
	Requests            int   `json:"requests"`
	Attempts            int   `json:"attempts"`
	ReservedRequests    int   `json:"reserved_requests"`
	DownloadedBytes     int64 `json:"downloaded_bytes"`
	DecodedBytes        int64 `json:"decoded_bytes"`
	ExtractedBytes      int64 `json:"extracted_bytes"`
	RuntimeStorageBytes int64 `json:"runtime_storage_bytes"`
	WallSeconds         int64 `json:"wall_seconds"`
}

type Item struct {
	// JobID is deterministic from the sealed run profile and canonical resource
	// ID. It is safe to persist, unlike a URL or provider cursor.
	JobID            string `json:"job_id"`
	ResourceID       string `json:"resource_id"`
	State            string `json:"state"`
	Reason           string `json:"reason,omitempty"`
	Attempts         int    `json:"attempts"`
	ReservedRequests int    `json:"reserved_requests"`
}

// RebuildItem is the complete source-neutral state needed to reconstruct one
// derived queue item from canonical resource evidence. Attempts and consumed
// counters are deliberately not authority and reset during a rebuild.
type RebuildItem struct {
	ResourceID string
	State      string
	Reason     string
}

type Queue struct {
	SchemaVersion string        `json:"schema_version"`
	Profile       BudgetProfile `json:"profile"`
	// Generation is zero for the original bounded run. Omitempty preserves the
	// fingerprint of queues written before generational continuation existed.
	Generation int `json:"generation,omitempty"`
	// GenerationKind lets a canceled operator retry resume idempotently without
	// treating an unrelated reconciled queue as the same operation.
	GenerationKind string `json:"generation_kind,omitempty"`
	// GenerationClosed prevents any later claim after a narrowed decoded,
	// extracted, or storage envelope proved unable to fit the next resource.
	// Omitempty preserves fingerprints written before this boundary existed.
	GenerationClosed bool `json:"generation_closed,omitempty"`
	// Rebuilds cannot safely distinguish a historical global remainder from a
	// per-resource budget failure. This derived marker prevents an ambiguous
	// rebuilt budget_exhausted item from entering the one-time legacy migration.
	LegacyBudgetMigrationComplete bool     `json:"legacy_budget_migration_complete,omitempty"`
	Counters                      Counters `json:"counters"`
	Items                         []Item   `json:"items"`
	Fingerprint                   string   `json:"fingerprint"`
}

// Usage is reported by a fetch adapter as structural counters only. The queue
// rejects values that would cross its frozen aggregate caps before canonical
// enrichment can be committed.
type Usage struct {
	Requests            int
	DownloadedBytes     int64
	DecodedBytes        int64
	ExtractedBytes      int64
	RuntimeStorageBytes int64
	WallSeconds         int64
}

func JobIdentity(profile BudgetProfile, resourceID string) string {
	sum := sha256.Sum256([]byte(profile.Fingerprint + "\x00" + resourceID))
	return "resource-job-" + hex.EncodeToString(sum[:])[:24]
}

func Empty(profile BudgetProfile) Queue {
	q := Queue{SchemaVersion: SchemaVersion, Profile: profile, Items: []Item{}}
	return Seal(q)
}

func Seal(queue Queue) Queue {
	queue.SchemaVersion = SchemaVersion
	sort.Slice(queue.Items, func(i, j int) bool { return queue.Items[i].ResourceID < queue.Items[j].ResourceID })
	queue.Fingerprint = ""
	queue.Fingerprint = fingerprint(queue)
	return queue
}

func Validate(queue Queue) error {
	if queue.SchemaVersion != SchemaVersion || ValidateProfile(queue.Profile) != nil || queue.Generation < 0 ||
		len(queue.Items) > MaximumQueueItems ||
		!validGenerationKind(queue.GenerationKind) ||
		queue.Counters.ProcessedResources < 0 || queue.Counters.Requests < 0 || queue.Counters.Attempts < 0 || queue.Counters.ReservedRequests < 0 ||
		queue.Counters.DownloadedBytes < 0 || queue.Counters.DecodedBytes < 0 || queue.Counters.ExtractedBytes < 0 ||
		queue.Counters.RuntimeStorageBytes < 0 || queue.Counters.WallSeconds < 0 {
		return errors.New("invalid resource queue")
	}
	seen := map[string]bool{}
	for _, item := range queue.Items {
		if !validQueueText(item.ResourceID, maximumResourceIDBytes) || item.JobID != JobIdentity(queue.Profile, item.ResourceID) || seen[item.ResourceID] || item.Attempts < 0 || item.ReservedRequests < 0 {
			return errors.New("invalid resource queue item")
		}
		seen[item.ResourceID] = true
		switch item.State {
		case StateQueued, StateProcessing, StateComplete, StatePartial:
			if item.Reason != "" {
				return errors.New("non-blocked resource queue item has reason")
			}
		case StateBlocked:
			if !fixedBlockedReasons[item.Reason] {
				return errors.New("invalid blocked resource reason")
			}
		default:
			return errors.New("invalid resource queue state")
		}
		if item.State != StateProcessing && item.ReservedRequests != 0 {
			return errors.New("non-processing resource queue item reserves requests")
		}
	}
	copy := queue
	copy.Fingerprint = ""
	if queue.Fingerprint == "" || queue.Fingerprint != fingerprint(copy) {
		return errors.New("resource queue fingerprint mismatch")
	}
	return nil
}

func validQueueText(value string, maximumBytes int) bool {
	if value == "" || len(value) > maximumBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range []byte(value) {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') &&
			character != '-' && character != '_' && character != '.' && character != ':' {
			return false
		}
	}
	return true
}

func validGenerationKind(kind string) bool {
	return kind == "" || kind == "continuation" ||
		kind == "retry:unreachable" || kind == "retry:rate_limited"
}

func IsRetryableTerminalReason(reason string) bool {
	return reason == "unreachable" || reason == "rate_limited"
}

func IsBlockedReason(reason string) bool { return fixedBlockedReasons[reason] }

// CanonicalState maps operational terminal state to the existing durable
// ResourceContext state contract. The queue itself is never a read source.
func CanonicalState(state, reason string) (string, []string, error) {
	switch state {
	case StateComplete:
		return "complete", nil, nil
	case StatePartial:
		return "partial", nil, nil
	case StateBlocked:
		if !IsBlockedReason(reason) {
			return "", nil, errors.New("invalid blocked resource reason")
		}
		canonical := "failed"
		switch reason {
		case "sensitive_or_ambiguous", "access_denied", "manual_processing_required":
			canonical = "inaccessible"
		}
		return canonical, []string{"resource_blocked:" + reason}, nil
	default:
		return "", nil, errors.New("resource queue item is not terminal")
	}
}

func fingerprint(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
