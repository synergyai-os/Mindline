# WP-8 Local Run Ledger and Review Queue Implementation Plan

Plan version: `MINDLINE-WP8-PLAN-V4`
Date: 2026-05-21
Status: Draft for Randy + LOOP review. Not delivery authority until signed in Product Brain.

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:test-driven-development` before implementation, then `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend Mindline's local `pipeline dry-run` so every run produces a deterministic local run ledger and derived review queue.

**Architecture:** Add ledger and review queue generation inside `internal/pipeline` as a post-processing layer over WP-7 pipeline summary, result, processor plan, and destination output. Keep it local-file and dry-run only. The pipeline remains the orchestrator; ledger/review code must not call live APIs, destination writes, databases, auth providers, or Tolaria vault paths.

**Tech Stack:** Go, standard library JSON/path/hash/time APIs, existing `internal/pipeline`, existing CLI runner, existing artifact writer, existing testdata pipeline fixtures.

**Delivery Preconditions:** PR #3 (`codex/wp-7-local-pipeline-runner`) must be merged into `main`, or Product Brain must capture an explicit branch-base decision to build on top of PR #3. Verify `internal/pipeline/runner.go`, `internal/pipeline/artifacts/writer.go`, and `testdata/pipeline/inputs/pipeline-text-only.json` exist before implementation.

---

## Files and Responsibilities

- Create `internal/pipeline/runs/ledger.go`: versioned run manifest, ledger item, review queue, review item structs and deterministic builders.
- Create `internal/pipeline/runs/ledger_test.go`: state mapping, review inclusion, run id determinism, private/secret redaction, existing-output behavior.
- Modify `internal/pipeline/artifacts/writer.go`: write ledger and review queue artifacts under `ledger/**` and `review-queue/**`.
- Modify `internal/pipeline/artifacts/writer_test.go`: golden file checks for ledger/review output and no-leak scans.
- Modify `internal/pipeline/runner.go`: build run ledger after pipeline item processing and before artifact write.
- Modify `internal/pipeline/runner_test.go`: fixture-level tests for text-only, missing enrichment, private provenance, secret-like, and rerun/idempotency behavior.
- Modify `internal/cli/pipeline_dry_run_test.go`: stdout/file parity must include ledger paths and review queue counts.
- Create `testdata/pipeline/inputs/pipeline-youtube-url.json` and `testdata/pipeline/candidates/pipeline-youtube-url.json` for the missing-enrichment queue fixture.
- Add expected output fixtures under `testdata/pipeline/expected/wp8/**` if the existing test style uses golden fixtures.
- Modify `README.md`: document local run ledger and review queue inspection.

## Task 0: Preflight

- [ ] **Step 1: Verify Product Brain profile**

Run:

```bash
pb profile list
```

Expected: JSON contains `"activeSource":"local"` and `"active":"randy-s-pkm"`.

- [ ] **Step 2: Verify branch base**

Run:

```bash
git branch --show-current
test -f internal/pipeline/runner.go
test -f internal/pipeline/artifacts/writer.go
test -f testdata/pipeline/inputs/pipeline-text-only.json
```

Expected: all file checks pass. If they fail, PR #3 is not merged or the branch is not based on PR #3; stop and reconcile base.

- [ ] **Step 3: Verify delivery authority**

Run:

```bash
pb get WP-8
```

Expected:

- `WP-8` exists and names the local run ledger/review queue work package;
- `WP-8` is related to `PROD-1`, `DEC-17`, and the signed WP-8 spec/plan decision or capture record;
- the signed spec and signed plan paths match the files being implemented;
- status is an implementation-ready pre-delivery state, not `shipped`.

If Product Brain does not prove that WP-8 is the governing delivery authority, stop and reconcile PB before code edits.

- [ ] **Step 4: Run baseline tests**

Run:

```bash
go test -count=1 ./...
```

Expected: PASS before WP-8 edits.

## Task 1: Run Ledger Domain Model

- [ ] **Step 1: Write failing ledger state tests**

Create `internal/pipeline/runs/ledger_test.go`:

```go
package runs

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStateFromPipelineItem(t *testing.T) {
	cases := []struct {
		name        string
		input       ItemInput
		wantState   string
		wantReview  bool
		wantReason  string
	}{
		{
			name: "published preview",
			input: ItemInput{
				RecordID: "pipeline-text-only",
				PipelineState: "dry_run_published",
				PreviewPaths: []string{"destinations/pipeline-text-only/previews/op.md"},
			},
			wantState: "published_preview",
		},
		{
			name: "missing enrichment",
			input: ItemInput{
				RecordID: "pipeline-youtube-url",
				Blockers: []string{"missing_local_youtube_transcript"},
			},
			wantState: "needs_enrichment",
			wantReview: true,
			wantReason: "missing_local_youtube_transcript",
		},
		{
			name: "private provenance",
			input: ItemInput{
				RecordID: "fingerprint:abc123",
				PrivateProvenance: true,
			},
			wantState: "background",
			wantReview: false,
			wantReason: "",
		},
		{
			name: "secret skip",
			input: ItemInput{
				RecordID: "fingerprint:def456",
				PipelineState: "skipped",
				Blockers: []string{"secret_like_content_detected"},
				SecretLike: true,
			},
			wantState: "skipped",
			wantReview: false,
			wantReason: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildLedgerItem("run-abc", tc.input, []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"})
			if got.State != tc.wantState {
				t.Fatalf("state got %q want %q", got.State, tc.wantState)
			}
			if got.ReviewRequired != tc.wantReview {
				t.Fatalf("review got %v want %v", got.ReviewRequired, tc.wantReview)
			}
			if got.ReviewReason != tc.wantReason {
				t.Fatalf("reason got %q want %q", got.ReviewReason, tc.wantReason)
			}
		})
	}
}

func TestPathSafeIDsRedactUnsafeNativeIDs(t *testing.T) {
	cases := []string{
		"https://linkedin.com/private/PRIVATE_DM_SENTINEL_DO_NOT_WRITE",
		"sk-test-secret-do-not-leak",
		"../outside",
		`folder\\outside`,
		"author-name/private-dm-text",
	}
	for _, native := range cases {
		t.Run(native, func(t *testing.T) {
			got := BuildLedgerItem("run-abc", ItemInput{
				RecordID: native,
				SourceCandidateID: native,
				PrivateProvenance: true,
			}, []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"})
			for _, value := range []string{got.RecordID, got.SourceCandidateID} {
				assertSafeIdentifier(t, value)
			}
			for _, value := range []string{got.PipelineResultPath, got.ProcessorPlanPath, got.DestinationSummaryPath} {
				assertSafeRelativePath(t, value)
			}
		})
	}
}

func assertSafeIdentifier(t *testing.T, value string) {
	t.Helper()
	for _, forbidden := range []string{"PRIVATE_DM_SENTINEL_DO_NOT_WRITE", "sk-test-secret-do-not-leak", "http://", "https://", "..", "/", `\\`} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("unsafe id/path %q contains %q", value, forbidden)
		}
	}
}

func assertSafeRelativePath(t *testing.T, value string) {
	t.Helper()
	for _, forbidden := range []string{"PRIVATE_DM_SENTINEL_DO_NOT_WRITE", "sk-test-secret-do-not-leak", "http://", "https://", "..", `\\`} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("unsafe path %q contains %q", value, forbidden)
		}
	}
	if filepath.IsAbs(value) {
		t.Fatalf("path must be relative: %q", value)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/pipeline/runs -run TestStateFromPipelineItem -count=1
```

Expected: FAIL because package/types do not exist.

- [ ] **Step 3: Implement minimal ledger structs and state mapping**

Create `internal/pipeline/runs/ledger.go` with:

```go
package runs

type ItemInput struct {
	RecordID          string
	SourceCandidateID string
	PipelineState     string
	Blockers          []string
	PreviewPaths      []string
	PrivateProvenance bool
	SecretLike        bool
	RedactionRequired bool
	SafeTitle         string
}

type LedgerItem struct {
	SchemaVersion         string   `json:"schema_version"`
	RunID                 string   `json:"run_id"`
	RecordID              string   `json:"record_id"`
	SourceCandidateID     string   `json:"source_candidate_id"`
	State                 string   `json:"state"`
	ReviewRequired        bool     `json:"review_required"`
	ReviewReason          string   `json:"-"`
	Blockers              []string `json:"blockers"`
	PipelineResultPath    string   `json:"pipeline_result_path"`
	ProcessorPlanPath     string   `json:"processor_plan_path"`
	DestinationSummaryPath string   `json:"destination_summary_path"`
	PreviewPaths          []string `json:"preview_paths,omitempty"`
	SafeTitle             string   `json:"safe_title"`
	SafeSummary           string   `json:"safe_summary"`
	AuthorityIDs          []string `json:"authority_ids"`
}
```

Implement `BuildSafeID(native string) string` and use it inside `BuildLedgerItem(runID string, input ItemInput, authorityIDs []string) LedgerItem`.

`ReviewReason` is an internal bridge field for deriving the review queue. It must not serialize into ledger item JSON.

Safe id and artifact path rules:

- safe ids are deterministic and opaque;
- safe ids contain only lowercase letters, numbers, and hyphens;
- unsafe native ids are transformed to `item-<16 hex chars>`;
- unsafe means empty, raw URL, author/private text marker, secret-like marker, path separator, `..`, whitespace-heavy source text, or any value containing `PRIVATE_DM_SENTINEL_DO_NOT_WRITE` or `sk-test-secret-do-not-leak`;
- if a human fixture id is already safe, such as `pipeline-text-only`, preserve it for readable test fixtures.
- serialized artifact references are clean `<out>`-relative paths with no `..`, no absolute prefix, no raw URL, and no raw/native unsafe id material.
- examples: `pipeline-summary.json`, `results/<safe-id>.json`, `processors/<safe-id>.json`, `destinations/<safe-id>/destination-summary.json`, and `destinations/<safe-id>/previews/<safe-file>.md`.

State rules:

- secret-like skipped -> `skipped`, `ReviewRequired=false`;
- blocker containing `missing_local_` -> `needs_enrichment`, `ReviewRequired=true`;
- blocker containing `clarification` or `ambiguous` -> `needs_clarification`, `ReviewRequired=true`;
- `private_provenance` without another blocker -> `background`, `ReviewRequired=false`;
- any other blocker -> `blocked`, `ReviewRequired=true`;
- preview path present and no blockers -> `published_preview`;
- otherwise no blockers -> `background`.

- [ ] **Step 4: Run test to verify pass**

Run:

```bash
go test ./internal/pipeline/runs -run TestStateFromPipelineItem -count=1
```

Expected: PASS.

- [ ] **Step 5: Confirm path-safe id coverage**

Run:

```bash
go test ./internal/pipeline/runs -run TestPathSafeIDsRedactUnsafeNativeIDs -count=1
```

Expected: PASS. No ledger id/path field contains sentinels, URL markers, path separators, or traversal fragments.

## Task 2: Deterministic Run Manifest

- [ ] **Step 1: Write failing run id and manifest tests**

Append to `internal/pipeline/runs/ledger_test.go`:

```go
func TestRunIDIsDeterministicAndDoesNotUsePrivateContent(t *testing.T) {
	a := RunIdentityInput{
		InputPath: "/repo/testdata/pipeline/inputs/pipeline-private-provenance.json",
		InputBytes: []byte(`{"source":"fingerprint-only"}`),
		MethodID: "basb-para-code",
		DestinationID: "tolaria",
	}
	b := a
	gotA := BuildRunID(a)
	gotB := BuildRunID(b)
	if gotA != gotB {
		t.Fatalf("run id not deterministic: %q %q", gotA, gotB)
	}
	if gotA == "" || len(gotA) != len("run-0123456789abcdef") {
		t.Fatalf("unexpected run id %q", gotA)
	}
}

func TestManifestCountsStates(t *testing.T) {
	items := []LedgerItem{
		{State: "published_preview"},
		{State: "needs_enrichment", ReviewRequired: true},
		{State: "blocked", ReviewRequired: true},
	}
	manifest := BuildManifest(ManifestInput{
		RunID: "run-abc",
		RunMode: "dry_run",
		MethodID: "basb-para-code",
		DestinationID: "tolaria",
		InputFingerprint: "sha256:test",
		Items: items,
		AuthorityIDs: []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"},
	})
	if manifest.ItemCount != 3 || manifest.ReviewQueueCount != 2 {
		t.Fatalf("unexpected counts: %+v", manifest)
	}
	if manifest.States["published_preview"] != 1 || manifest.States["needs_enrichment"] != 1 || manifest.States["blocked"] != 1 {
		t.Fatalf("unexpected state counts: %+v", manifest.States)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/pipeline/runs -run 'TestRunID|TestManifest' -count=1
```

Expected: FAIL with undefined types/functions.

- [ ] **Step 3: Implement run id and manifest**

Add structs and functions:

```go
type RunIdentityInput struct {
	InputPath     string
	InputBytes    []byte
	MethodID      string
	DestinationID string
}

type ManifestInput struct {
	RunID            string
	RunMode          string
	MethodID         string
	DestinationID    string
	InputFingerprint string
	Items            []LedgerItem
	AuthorityIDs     []string
	Now              string
}

type Manifest struct {
	SchemaVersion      string         `json:"schema_version"`
	RunID              string         `json:"run_id"`
	RunMode            string         `json:"run_mode"`
	PipelineSummaryPath string        `json:"pipeline_summary_path"`
	MethodID           string         `json:"method_id"`
	DestinationID      string         `json:"destination_id"`
	InputFingerprint   string         `json:"input_fingerprint"`
	StartedAt          string         `json:"started_at"`
	CompletedAt        string         `json:"completed_at"`
	ItemCount          int            `json:"item_count"`
	ReviewQueueCount   int            `json:"review_queue_count"`
	States             map[string]int `json:"states"`
	AuthorityIDs       []string       `json:"authority_ids"`
}
```

Use SHA-256 over schema version, input path, input bytes, method id, and destination id. Return first 16 hex chars prefixed with `run-`. Use `sha256:<hex>` for input fingerprint. If `Now` is empty, use current UTC RFC3339.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/pipeline/runs -count=1
```

Expected: PASS.

## Task 2.5: Ledger Index Builder

- [ ] **Step 1: Write failing ledger index tests**

Append to `internal/pipeline/runs/ledger_test.go`:

```go
func TestBuildIndexPreservesItemOrderAndReviewFlags(t *testing.T) {
	items := []LedgerItem{
		{RunID: "run-abc", RecordID: "publish", SourceCandidateID: "publish", State: "published_preview", ReviewRequired: false},
		{RunID: "run-abc", RecordID: "youtube", SourceCandidateID: "youtube", State: "needs_enrichment", ReviewRequired: true},
	}
	index := BuildIndex("run-abc", items, []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"})
	if index.ItemCount != 2 || index.ReviewRequiredCount != 1 {
		t.Fatalf("unexpected counts: %+v", index)
	}
	if index.Items[0].RecordID != "publish" || index.Items[0].LedgerItemPath != "items/publish.json" {
		t.Fatalf("unexpected first item: %+v", index.Items[0])
	}
	if !index.Items[1].ReviewRequired || index.Items[1].State != "needs_enrichment" {
		t.Fatalf("unexpected second item: %+v", index.Items[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/pipeline/runs -run TestBuildIndex -count=1
```

Expected: FAIL with undefined index types/functions.

- [ ] **Step 3: Implement typed index structs and builder**

Add:

```go
type Index struct {
	SchemaVersion      string      `json:"schema_version"`
	RunID              string      `json:"run_id"`
	ItemCount          int         `json:"item_count"`
	ReviewRequiredCount int        `json:"review_required_count"`
	Items              []IndexItem `json:"items"`
	AuthorityIDs       []string    `json:"authority_ids"`
}

type IndexItem struct {
	RecordID          string `json:"record_id"`
	SourceCandidateID string `json:"source_candidate_id"`
	State             string `json:"state"`
	ReviewRequired    bool   `json:"review_required"`
	LedgerItemPath    string `json:"ledger_item_path"`
}
```

Implement `BuildIndex(runID string, items []LedgerItem, authorityIDs []string) Index`. Preserve item order. Do not include source body, URLs, author names, or secret-like content.

Use only `LedgerItem.RecordID` values already normalized by `BuildSafeID`. `LedgerItemPath` must be `items/<safe-record-id>.json` and must pass the shared relative path validation used by the artifact writer. It is relative to `ledger/`, while all cross-artifact links inside item JSON remain clean `<out>`-relative paths.

- [ ] **Step 4: Run runs package tests**

Run:

```bash
go test ./internal/pipeline/runs -count=1
```

Expected: PASS.

## Task 3: Review Queue Builder

- [ ] **Step 1: Write failing review queue tests**

Append:

```go
func TestReviewQueueIncludesOnlyReviewRequiredItems(t *testing.T) {
	items := []LedgerItem{
		{RunID: "run-abc", RecordID: "publish", State: "published_preview", ReviewRequired: false},
		{RunID: "run-abc", RecordID: "youtube", State: "needs_enrichment", ReviewRequired: true, ReviewReason: "missing_local_youtube_transcript"},
		{RunID: "run-abc", RecordID: "secret", State: "skipped", ReviewRequired: false, ReviewReason: "secret_like_content_detected"},
	}
	queue := BuildReviewQueue("run-abc", items, []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"})
	if queue.QueueCount != 1 {
		t.Fatalf("queue count got %d want 1", queue.QueueCount)
	}
	if queue.Items[0].RecordID != "youtube" || queue.Items[0].Reason != "missing_local_youtube_transcript" {
		t.Fatalf("unexpected queue item: %+v", queue.Items[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/pipeline/runs -run TestReviewQueue -count=1
```

Expected: FAIL with undefined queue types/functions.

- [ ] **Step 3: Implement review queue structs and builder**

Add `ReviewQueue`, `ReviewQueueEntry`, and `ReviewQueueItem` structs:

```go
type ReviewQueue struct {
	SchemaVersion string             `json:"schema_version"`
	RunID         string             `json:"run_id"`
	QueueCount    int                `json:"queue_count"`
	Items         []ReviewQueueEntry `json:"items"`
	AuthorityIDs  []string           `json:"authority_ids"`
}

type ReviewQueueEntry struct {
	RecordID       string `json:"record_id"`
	State          string `json:"state"`
	Reason         string `json:"reason"`
	ReviewItemPath string `json:"review_item_path"`
}

type ReviewQueueItem struct {
	SchemaVersion      string            `json:"schema_version"`
	RunID              string            `json:"run_id"`
	RecordID           string            `json:"record_id"`
	State              string            `json:"state"`
	Priority           string            `json:"priority"`
	Reason             string            `json:"reason"`
	SuggestedNextAction string            `json:"suggested_next_action"`
	SafeTitle          string            `json:"safe_title"`
	SafeContext        string            `json:"safe_context"`
	Links              map[string]string `json:"links"`
	AuthorityIDs       []string          `json:"authority_ids"`
}
```

Implement:

```go
func BuildReviewQueue(runID string, items []LedgerItem, authorityIDs []string) ReviewQueue
func BuildReviewQueueItem(item LedgerItem, authorityIDs []string) ReviewQueueItem
```

Use only safe `LedgerItem.RecordID` values. `ReviewItemPath` must be `items/<safe-record-id>.json`. Queue item links must be clean `<out>`-relative paths only: no `..`, no absolute prefix, no raw URL, and no raw/native unsafe id material.

Suggested next actions:

- `needs_enrichment`: `Provide or run the missing local processor artifact before destination write.`
- `needs_clarification`: `Clarify save intent, classification, or destination visibility.`
- `blocked`: `Review blocker before retrying this item.`

Secret-like skipped items must not be included.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/pipeline/runs -count=1
```

Expected: PASS.

## Task 4: Artifact Writer Integration

- [ ] **Step 1: Write failing artifact writer tests**

Extend `internal/pipeline/artifacts/writer_test.go`:

```go
func TestWriterWritesLedgerAndReviewQueue(t *testing.T) {
	out := t.TempDir()
	output := goldenTextOnlyPipelineOutputWithLedger()
	err := Write(out, output, nil)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, relative := range []string{
		"ledger/run-manifest.json",
		"ledger/index.json",
		"ledger/items/pipeline-text-only.json",
		"review-queue/review-queue.json",
	} {
		if _, err := os.Stat(filepath.Join(out, relative)); err != nil {
			t.Fatalf("expected %s: %v", relative, err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/pipeline/artifacts -run TestWriterWritesLedgerAndReviewQueue -count=1
```

Expected: FAIL because output struct has no ledger/review fields or writer ignores them.

- [ ] **Step 3: Extend artifact output struct**

Modify `internal/pipeline/artifacts/writer.go`:

- add `RunLedger runs.Manifest` to `Output`;
- add `LedgerIndex runs.Index` to `Output`;
- add `LedgerItems []LedgerItemFile`;
- add `ReviewQueue runs.ReviewQueue`;
- add `ReviewQueueItems []ReviewQueueItemFile`;
- write:
  - `ledger/run-manifest.json`;
  - `ledger/index.json`;
  - `ledger/items/<record_id>.json`;
  - `review-queue/review-queue.json`;
  - `review-queue/items/<record_id>.json` for each review item.

Keep sentinel rejection over the full output object before writing.

Add a pre-write relative path validation gate before every file write:

- every generated relative path must be clean and relative;
- no generated path may contain `..`, absolute path prefixes, raw URLs, private/secret sentinels, or path separators inside generated ids;
- the final joined path must remain under the chosen `--out` directory;
- fail before writing any new ledger/review files if a path is unsafe.

Apply the same validator to serialized artifact link fields before writing JSON. Cross-artifact links must be `<out>`-relative paths such as `results/<safe-id>.json`, `processors/<safe-id>.json`, `destinations/<safe-id>/destination-summary.json`, and `destinations/<safe-id>/previews/<safe-file>.md`; do not serialize links relative to the JSON file location using `../`.

- [ ] **Step 4: Run artifact tests**

Run:

```bash
go test ./internal/pipeline/artifacts -count=1
```

Expected: PASS.

## Task 5: Pipeline Runner Integration

- [ ] **Step 1: Write failing runner tests**

Extend `internal/pipeline/runner_test.go`:

```go
func TestRunnerWritesLedgerAndReviewQueueForTextOnly(t *testing.T) {
	out := t.TempDir()
	_, err := Run(filepath.Join("..", "..", "testdata", "pipeline", "inputs", "pipeline-text-only.json"), out, RunOptions{Now: "2026-05-21T00:00:00Z"})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	assertJSONField(t, filepath.Join(out, "ledger", "run-manifest.json"), "item_count", float64(1))
	assertJSONField(t, filepath.Join(out, "review-queue", "review-queue.json"), "queue_count", float64(0))
}

func TestRunnerReviewQueueIncludesMissingEnrichment(t *testing.T) {
	out := t.TempDir()
	_, err := Run(filepath.Join("..", "..", "testdata", "pipeline", "inputs", "pipeline-youtube-url.json"), out, RunOptions{Now: "2026-05-21T00:00:00Z"})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}
	assertJSONField(t, filepath.Join(out, "review-queue", "review-queue.json"), "queue_count", float64(1))
}
```

Add the helper in the same test file if it does not already exist:

```go
func assertJSONField(t *testing.T, path string, field string, want any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if decoded[field] != want {
		t.Fatalf("%s got %#v want %#v", field, decoded[field], want)
	}
}
```

- [ ] **Step 2: Add missing YouTube fixture**

Create:

- `testdata/pipeline/inputs/pipeline-youtube-url.json`;
- `testdata/pipeline/candidates/pipeline-youtube-url.json`.

The candidate must include one YouTube URL and no transcript artifact, causing `missing_local_youtube_transcript`.

- [ ] **Step 3: Run test to verify it fails**

Run:

```bash
go test ./internal/pipeline -run 'TestRunnerWritesLedger|TestRunnerReviewQueue' -count=1
```

Expected: FAIL because runner does not build ledger/review output yet.

- [ ] **Step 4: Add RunOptions time injection**

Modify `internal/pipeline/runner.go`:

```go
type RunOptions struct {
	ProtectedRoots       []string
	DestinationAvailable func(adapterID string) bool
	Now                  string
}
```

Pass `Now` to manifest building for deterministic tests.

- [ ] **Step 5: Build ledger/review output in runner**

After item processing and before artifact write:

1. read input bytes;
2. build run id and input fingerprint;
3. convert pipeline items to `runs.ItemInput`;
4. build ledger items;
5. build manifest;
6. build review queue and review queue item files;
7. attach all ledger/review structures to artifact output.

All serialized artifact links attached here must be clean `<out>`-relative paths. Do not generate `../` links from `ledger/items` or `review-queue/items`; those links must pass the same no-`..` validator used by the artifact writer.

- [ ] **Step 6: Run runner tests**

Run:

```bash
go test ./internal/pipeline -count=1
```

Expected: PASS.

## Task 6: Existing Output Safety

- [ ] **Step 1: Write failing same-run and different-run output safety tests**

Add to `internal/pipeline/runner_test.go`:

```go
func TestRunnerAllowsSameRunInExistingOutputDirectory(t *testing.T) {
	out := t.TempDir()
	first, err := Run(textOnlyInputPath(t), out, RunOptions{Now: "2026-05-21T00:00:00Z"})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := Run(textOnlyInputPath(t), out, RunOptions{Now: "2026-05-21T00:00:00Z"})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if first.RunLedger.RunID != second.RunLedger.RunID {
		t.Fatalf("run id changed: %q %q", first.RunLedger.RunID, second.RunLedger.RunID)
	}
	assertJSONField(t, filepath.Join(out, "ledger", "run-manifest.json"), "run_id", first.RunLedger.RunID)
}

func TestRunnerRefusesDifferentRunInExistingOutputDirectory(t *testing.T) {
	out := t.TempDir()
	_, err := Run(textOnlyInputPath(t), out, RunOptions{Now: "2026-05-21T00:00:00Z"})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	beforeManifest := mustReadFile(t, filepath.Join(out, "ledger", "run-manifest.json"))
	beforeQueue := mustReadFile(t, filepath.Join(out, "review-queue", "review-queue.json"))
	_, err = Run(youtubeInputPath(t), out, RunOptions{Now: "2026-05-21T00:00:00Z"})
	if err == nil || !strings.Contains(err.Error(), "existing output directory belongs to a different run") {
		t.Fatalf("expected different-run refusal, got %v", err)
	}
	afterManifest := mustReadFile(t, filepath.Join(out, "ledger", "run-manifest.json"))
	afterQueue := mustReadFile(t, filepath.Join(out, "review-queue", "review-queue.json"))
	if string(beforeManifest) != string(afterManifest) || string(beforeQueue) != string(afterQueue) {
		t.Fatalf("different-run refusal mutated existing ledger/review files")
	}
}
```

Add `mustReadFile(t, path)` in the same test file if it does not already exist.

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/pipeline -run 'TestRunnerAllowsSameRun|TestRunnerRefusesDifferentRun' -count=1
```

Expected: FAIL because existing manifest is ignored.

- [ ] **Step 3: Implement existing manifest guard**

Before writing new artifacts:

- if `<out>/ledger/run-manifest.json` does not exist, continue;
- if it exists and `run_id` matches, continue;
- if it exists and `run_id` differs, fail before writing new ledger/review files.

Do not add overwrite flags in WP-8.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/pipeline -count=1
```

Expected: PASS.

## Task 7: CLI and README

- [ ] **Step 1: Extend CLI test expectations**

Modify `internal/cli/pipeline_dry_run_test.go` so `TestPipelineDryRunStdoutMatchesSummaryFile` also asserts:

```go
if _, err := os.Stat(filepath.Join(out, "ledger", "run-manifest.json")); err != nil {
	t.Fatalf("expected run manifest: %v", err)
}
if _, err := os.Stat(filepath.Join(out, "review-queue", "review-queue.json")); err != nil {
	t.Fatalf("expected review queue: %v", err)
}
```

- [ ] **Step 2: Run CLI tests**

Run:

```bash
go test ./internal/cli -run TestPipelineDryRun -count=1
```

Expected: PASS after runner/artifact integration.

- [ ] **Step 3: Update README**

Add a section:

```markdown
## Run Ledger and Review Queue

Every local pipeline dry-run writes a run ledger under `ledger/` and a derived review queue under `review-queue/`.

- `ledger/run-manifest.json` summarizes the run.
- `ledger/items/*.json` records each item state.
- `review-queue/review-queue.json` lists only items needing attention.
- `review-queue/items/*.json` contains safe, redacted next-action context.

The ledger and queue are local dry-run artifacts. They do not write to Tolaria, call live APIs, or persist to a database.
```

- [ ] **Step 4: Run full tests**

Run:

```bash
go test -count=1 ./...
```

Expected: PASS.

## Task 8: Final Verification

- [ ] **Step 1: Run signed verification commands**

Run:

```bash
go test -count=1 ./...
rm -rf /tmp/mindline-wp8-output /tmp/mindline-wp8-private-output /tmp/mindline-wp8-secret-output /tmp/mindline-wp8-stdout.txt /tmp/mindline-wp8-stderr.txt /tmp/mindline-wp8-private-stdout.txt /tmp/mindline-wp8-private-stderr.txt /tmp/mindline-wp8-secret-stdout.txt /tmp/mindline-wp8-secret-stderr.txt
go run ./cmd/mindline pipeline dry-run testdata/pipeline/inputs/pipeline-text-only.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp8-output > /tmp/mindline-wp8-stdout.txt 2> /tmp/mindline-wp8-stderr.txt
go run ./cmd/mindline pipeline dry-run testdata/pipeline/inputs/pipeline-private-provenance.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp8-private-output > /tmp/mindline-wp8-private-stdout.txt 2> /tmp/mindline-wp8-private-stderr.txt
go run ./cmd/mindline pipeline dry-run testdata/pipeline/inputs/pipeline-secret-like.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp8-secret-output > /tmp/mindline-wp8-secret-stdout.txt 2> /tmp/mindline-wp8-secret-stderr.txt
rg -n 'slack\.com/api|net/http|http\.Client|chromedp|playwright|puppeteer|openai|anthropic|claude|convex|supabase|mongodb|mongo\.Connect|clerk|workos|descope|oauth2' internal/pipeline internal/adapters internal/cli internal/destinations internal/sbos -g '!**/*_test.go'
rg -n 'PRIVATE_DM_SENTINEL_DO_NOT_WRITE|sk-test-secret-do-not-leak' /tmp/mindline-wp8-output /tmp/mindline-wp8-private-output /tmp/mindline-wp8-secret-output /tmp/mindline-wp8-stdout.txt /tmp/mindline-wp8-stderr.txt /tmp/mindline-wp8-private-stdout.txt /tmp/mindline-wp8-private-stderr.txt /tmp/mindline-wp8-secret-stdout.txt /tmp/mindline-wp8-secret-stderr.txt
rg -n 'https?://|\\.\\.|%2e%2e|PRIVATE_DM_SENTINEL_DO_NOT_WRITE|sk-test-secret-do-not-leak' /tmp/mindline-wp8-output/ledger /tmp/mindline-wp8-output/review-queue /tmp/mindline-wp8-private-output/ledger /tmp/mindline-wp8-private-output/review-queue /tmp/mindline-wp8-secret-output/ledger /tmp/mindline-wp8-secret-output/review-queue
find /tmp/mindline-wp8-output /tmp/mindline-wp8-private-output /tmp/mindline-wp8-secret-output -print | rg -n 'PRIVATE_DM_SENTINEL_DO_NOT_WRITE|sk-test-secret-do-not-leak|https?://|\\.\\.|%2e%2e|/DM-|/sk-'
```

Expected:

- `go test` and `go run` commands exit `0`;
- all `rg` content scans and the `find ... | rg` path-name check return no matches and exit `1`.

- [ ] **Step 2: Delivery review gate**

Before changing `WP-8` lifecycle status, request signed delivery review verdicts for the final implementation output. Capture:

- verification command results;
- reviewer verdicts;
- open risks or explicit no-open-risk statement;
- changed files and PR/commit reference.

If delivery review does not sign off, keep `WP-8` out of `shipped` and reconcile the blocker.

- [ ] **Step 3: Product Brain close-out**

Only after verification passes, delivery review signs off, and lifecycle proof is captured/linked in Product Brain:

```bash
pb session start
pb capture "WP-8 delivery evidence: local run ledger and review queue verification passed; delivery review signed off; open risks recorded."
pb relate <captured-evidence-id> informs WP-8
pb update WP-8 --field status=shipped --field validationStatus=validated-staging --note "WP-8 delivered after verification, delivery review sign-off, and linked lifecycle evidence."
pb verify WP-8
```

Expected: `WP-8` records linked delivery evidence, signed review verdicts, lifecycle proof, and verification status.
