# WP-4 Slack Self-DM Dry-Run Implementation Plan V5

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the signed WP-4 Slack self-DM dry-run source adapter from `MINDLINE-WP4-SPEC-V7`.

**Architecture:** Add a local-only Slack adapter package that reads Slack-like export JSON, normalizes messages old-to-new into existing `sbos.Candidate` objects, and emits explicit checkpoint metadata. Wire it into the existing CLI as `mindline slack normalize <slack-export.json> [--out <dir>]` without live Slack, network, Tolaria, database, auth, or destination behavior.

**Tech Stack:** Go standard library, existing `internal/sbos` candidate contract, existing `internal/cli` runner and filesystem abstraction.

---

## Signed Authority

- Spec: `.productbrain/specs/2026-05-20-wp-4-slack-self-dm-dry-run.md`
- Signed version: `MINDLINE-WP4-SPEC-V7`
- PB proof: `DEC-7`
- Work package: `WP-4`

## File Structure

- Create: `internal/adapters/slack/types.go`
  - Slack source payload structs, checkpoint structs, normalize result structs.
- Create: `internal/adapters/slack/normalize.go`
  - Pure normalization logic: sorting, candidate mapping, URL extraction, secret detection, private file sentinel handling.
- Create: `internal/adapters/slack/normalize_test.go`
  - Adapter-level TDD tests for candidate mapping, routes through `sbos`, safety, and checkpoint metadata.
- Create: `internal/cli/slack_normalize_test.go`
  - CLI TDD tests for stdout, `--out`, errors, containment, and no raw private/secret output.
- Modify: `internal/cli/runner.go`
  - Dispatch `slack normalize`, parse args, encode deterministic envelope, write candidate/checkpoint files.
- Create: `examples/slack/*.json`
  - Required fixture files from the signed spec.
- Modify: `README.md`
  - Add one short dry-run Slack normalize example after implementation passes.

## Task 1: Add Slack Adapter RED Tests

**Files:**
- Create: `internal/adapters/slack/normalize_test.go`
- Create: `internal/adapters/slack/types.go`
- Create: `internal/adapters/slack/normalize.go`

- [ ] **Step 1: Write failing adapter tests**

Write tests that call the desired API before implementation:

```go
func TestNormalizeSortsMessagesOldToNewAndBuildsCheckpoint(t *testing.T) {
	payload := slack.Payload{
		Source: slack.Source{Workspace: "synergyai-os", ChannelID: "DSELF", ChannelName: "self-dm"},
		Messages: []slack.Message{
			{TS: "1710000002.000001", User: "U1", AuthorName: "Randy", Text: "third", Permalink: "https://slack.example/3"},
			{TS: "1710000000.000001", User: "U1", AuthorName: "Randy", Text: "first", Permalink: "https://slack.example/1"},
			{TS: "1710000001.000001", User: "U1", AuthorName: "Randy", Text: "second", Permalink: "https://slack.example/2"},
		},
	}
	result, err := slack.Normalize(payload)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	got := []string{result.Candidates[0].ExternalID, result.Candidates[1].ExternalID, result.Candidates[2].ExternalID}
	want := []string{"DSELF:1710000000.000001", "DSELF:1710000001.000001", "DSELF:1710000002.000001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order got %#v want %#v", got, want)
	}
	if result.Checkpoint.BatchOrder != "old_to_new" || result.Checkpoint.FirstTS != "1710000000.000001" || result.Checkpoint.LastTS != "1710000002.000001" {
		t.Fatalf("bad checkpoint: %#v", result.Checkpoint)
	}
}
```

Also add tests for:

- empty content candidate has `safety.empty_content: true` and `sbos.NewEngine().ProcessCandidate` returns `skipped`
- secret strings `password=`, `api_key=`, `xoxb-`, `xoxp-`, `bearer `, `sk_live_` are redacted and route to `skipped`
- missing permalink uses `slack://missing-permalink/<channel>/<normalized_ts>`
- all provenance fields default private
- explicit public assertion makes all provenance fields public and URL candidates reach `needs_enrichment`
- `url_private` is replaced by `slack-file-private://F123` and raw private URL is absent
- `url_public` is absent when public provenance assertion is empty and present only when public provenance assertion is non-empty
- private Slack permalinks remain in candidate provenance with `visibility: private`; publish is blocked by the core unless public provenance is asserted
- ambiguous metadata maps to `desired_visibility: clarify`, `classification.needs_clarification: true`, non-empty `classification.clarification_reason`, and attention/clarification route

- [ ] **Step 2: Run RED**

Run:

```bash
env GOCACHE="$PWD/.cache/go-build" go test -count=1 ./internal/adapters/slack
```

Expected: FAIL because `internal/adapters/slack` and `Normalize` do not exist.

## Task 2: Implement Minimal Slack Adapter

**Files:**
- Create/modify: `internal/adapters/slack/types.go`
- Create/modify: `internal/adapters/slack/normalize.go`

- [ ] **Step 1: Add types and `Normalize` implementation**

Implement:

```go
package slack

type Payload struct {
	Source   Source    `json:"source"`
	Messages []Message `json:"messages"`
}

type Source struct {
	Workspace   string `json:"workspace"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	AdapterID   string `json:"adapter_id"`
}

type Message struct {
	TS              string          `json:"ts"`
	User            string          `json:"user"`
	AuthorName      string          `json:"author_name"`
	Text            string          `json:"text"`
	Permalink       string          `json:"permalink"`
	Files           []File          `json:"files"`
	Attachments     []Attachment    `json:"attachments"`
	CapturedAt      string          `json:"captured_at"`
	CaptureMetadata CaptureMetadata `json:"capture_metadata"`
}
```

Keep implementation pure: no filesystem, network, Tolaria path, or environment reads.

- [ ] **Step 2: Run GREEN**

Run:

```bash
env GOCACHE="$PWD/.cache/go-build" go test -count=1 ./internal/adapters/slack
```

Expected: PASS.

## Task 3: Add Required Fixture Files

**Files:**
- Create: `examples/slack/reverse-ordered-batch.json`
- Create: `examples/slack/url-file-attachment.json`
- Create: `examples/slack/empty-content.json`
- Create: `examples/slack/secret-redaction.json`
- Create: `examples/slack/ambiguous-metadata.json`
- Create: `examples/slack/missing-permalink-publish.json`
- Create: `examples/slack/private-default-publish.json`
- Create: `examples/slack/public-provenance-enrichment.json`

- [ ] **Step 1: Write fixture files exactly matching the signed spec**

Use `apply_patch` to add JSON fixtures. Keep secret fixture values only inside `examples/slack/secret-redaction.json`.

- [ ] **Step 2: Add fixture-driven adapter test**

Add a table-driven test that loads every `examples/slack/*.json`, normalizes it, marshals every candidate, confirms `sbos.NewEngine().ProcessCandidate` accepts each candidate, and asserts the fixture-specific route/key fields:

- `reverse-ordered-batch.json`: candidate order is ascending Slack timestamp.
- `url-file-attachment.json`: text and attachment URLs are present; `https://files.slack.com/files-pri/T/F123/design.pdf` is absent; `https://files.example/public/design.pdf` is absent because public assertion is empty; `slack-file-private://F123` is present; `safety.private_provenance` is true.
- `empty-content.json`: route is `skipped`.
- `secret-redaction.json`: route is `skipped`; raw secret fixture strings are absent from marshaled candidates.
- `ambiguous-metadata.json`: route is `attention_ready`; `desired_visibility` is `clarify`; `needs_clarification` is true; `classification.clarification_reason` is non-empty.
- `missing-permalink-publish.json`: route is `background_ready`; permalink sentinel is present; raw empty permalink is impossible.
- `private-default-publish.json`: route is `background_ready`; all provenance visibilities are private.
- `public-provenance-enrichment.json`: route is `needs_enrichment`; provenance visibilities are public; eligible URL is present.

- [ ] **Step 3: Run fixture tests**

Run:

```bash
env GOCACHE="$PWD/.cache/go-build" go test -count=1 ./internal/adapters/slack ./internal/sbos
```

Expected: PASS.

## Task 4: Add CLI RED Tests

**Files:**
- Create: `internal/cli/slack_normalize_test.go`
- Modify later: `internal/cli/runner.go`

- [ ] **Step 1: Write failing CLI tests**

Cover:

- `mindline slack normalize input.json` prints deterministic envelope and writes no files
- `mindline slack normalize input.json --out dry-run` writes candidate JSON files plus `slack-checkpoint.json`
- invalid args return `ExitUsage`
- invalid Slack payload returns `ExitProcess`
- write failure returns `ExitArtifactWrite`
- stdout and written files contain no raw `url_private` or secret fixture strings
- stdout, all `--out` files, checkpoint JSON, candidate JSON, and generated file paths contain no raw `url_private`, non-asserted `url_public`, or secret fixture strings; present Slack permalinks may appear only inside candidate provenance with `visibility: private`

- [ ] **Step 2: Run RED**

Run:

```bash
env GOCACHE="$PWD/.cache/go-build" go test -count=1 ./internal/cli
```

Expected: FAIL because CLI dispatch does not support `slack normalize`.

## Task 5: Implement CLI Command

**Files:**
- Modify: `internal/cli/runner.go`

- [ ] **Step 1: Dispatch commands**

Change `Run` to dispatch:

- `process <candidate.json> [--out <dir>]`
- `slack normalize <slack-export.json> [--out <dir>]`

Keep existing `process` behavior byte-for-byte unless tests require only usage text additions.

- [ ] **Step 2: Add Slack normalize envelope**

Emit deterministic JSON with:

```json
{
  "adapter_id": "slack",
  "candidate_count": 1,
  "candidates": [],
  "checkpoint": {},
  "authority_ids": ["WP-4", "WP-3", "WP-2", "FEAT-1", "STD-5", "STD-6", "STD-7", "STD-11", "STD-12", "DEC-6"]
}
```

Default stdout includes candidate bodies. With `--out`, stdout includes paths and omits bodies.

- [ ] **Step 3: Write output files safely**

Use existing `validateOutDir`, `sanitizeFilenameBase`, `isInside`, and `displayPath`. Candidate files use `<candidate_id>.json`; checkpoint file is `slack-checkpoint.json`.

- [ ] **Step 4: Run GREEN**

Run:

```bash
env GOCACHE="$PWD/.cache/go-build" go test -count=1 ./internal/cli
```

Expected: PASS.

## Task 6: End-to-End Verification, Boundary Proof, and Docs

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add README example**

Add a short example:

```bash
mindline slack normalize examples/slack/reverse-ordered-batch.json
mindline slack normalize examples/slack/reverse-ordered-batch.json --out dry-run
```

State that this is local dry-run normalization only: no live Slack, no Tolaria, no destination writes.

- [ ] **Step 2: Run full verification**

Run:

```bash
env GOCACHE="$PWD/.cache/go-build" go test -count=1 ./...
```

Expected: PASS.

- [ ] **Step 3: Run JSON verification**

Run:

```bash
env GOCACHE="$PWD/.cache/go-build" go test -json ./...
```

Expected: all packages pass, no failures.

- [ ] **Step 4: Artifact safety scan**

Run CLI `--out` against `examples/slack/url-file-attachment.json`, `examples/slack/secret-redaction.json`, and `examples/slack/private-default-publish.json` into a temporary output directory, then scan stdout, written files, checkpoint files, and paths for forbidden raw values:

```bash
rg "files-pri|super-secret-value|xoxb-1234567890-abcdef|sk_live_secret|https://files.example/public/design.pdf" <tmp-output-dir>
```

Expected: no matches for raw private file URLs, non-asserted public file URLs, or secret fixture strings. Do not include Slack message permalinks in this blanket forbidden scan because signed candidate mapping preserves present permalinks in private provenance.

Add a structural assertion in CLI tests for generated candidates from `private-default-publish.json`: any `https://workspace.slack.com/archives` value appears only at `provenance.permalink.value`, with `provenance.permalink.visibility: private`, and the core route is `background_ready`.

- [ ] **Step 5: Static boundary grep**

Run:

```bash
rg "net/http|http\\.Get|http\\.Post|oauth|token|SLACK|slack\\.com/api|conversations\\.history|chat\\.getPermalink|PKM - Tolaria|Tolaria/" internal cmd
```

Expected: no matches. README may mention “no Tolaria writes” as boundary documentation; implementation code must not reference Tolaria paths or live Slack/network/auth surfaces.

- [ ] **Step 6: Secret/private string source grep**

Run:

```bash
rg "files-pri|super-secret-value|xoxb-1234567890-abcdef|sk_live_secret" internal cmd README.md
```

Expected: no matches. Fixture-only secret/private inputs may appear under `examples/slack/*.json`, not under `internal`, `cmd`, or `README.md`.

## Task 7: PB SSOT and Lifecycle Proof

**Files:**
- No code files.

- [ ] **Step 1: Capture delivery proof in PB**

After verification passes, update Product Brain from the control workspace. Capture a concise entry linked to `WP-4` and `DEC-7` that records:

- delivered scope
- verification commands and pass/fail result
- safety proof outcome
- any deferred work or blockers
- WP-4 lifecycle/status disposition

Do not mark WP-4 shipped unless all tests, safety scans, review sign-off, and lifecycle proof pass.

- [ ] **Step 2: Close or update WP-4 lifecycle**

Use the existing PB lifecycle fields for `WP-4` to record the final status and validation status. If PB command support is insufficient, capture a blocker/tension instead of inventing a lifecycle state.

## Phase Exit

Do not start implementation until this plan is signed off by:

- Chain Steward
- Domain/User Job Reviewer
- Systems Architect
- Delivery Quality Reviewer
- Risk/Safety Reviewer

If any reviewer blocks, revise the plan version and rerun the full panel.
