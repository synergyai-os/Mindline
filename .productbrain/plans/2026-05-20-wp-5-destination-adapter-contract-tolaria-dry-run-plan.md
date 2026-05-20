# WP-5 Destination Adapter Contract and Tolaria Dry-Run Boundary Implementation Plan

Plan version: `MINDLINE-WP5-PLAN-V5`
Date: 2026-05-20
Status: Signed in Product Brain as `DEC-13`; Git artifact captured after PR #1 merge verification.

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:test-driven-development` before implementation, then `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the WP-5 destination adapter contract and Tolaria dry-run boundary without live Tolaria writes or destination coupling.

**Architecture:** Add a destination operation layer that consumes SBOS processing results/artifacts and produces destination-neutral dry-run operations. Add a Tolaria adapter that maps those operations to deterministic local preview artifacts under explicit `--out`, while keeping Tolaria Markdown/path rules inside the adapter. Add CLI support only after the contract and adapter mapping are covered by red/green tests.

**Tech Stack:** Go, standard library JSON/path/file APIs, existing `internal/sbos`, and merged CLI runner patterns from WP-2/WP-3/WP-4.

**Product Brain Authority:** WP-5 is the governing work item. DEC-12, `WP-5 Spec SIGN-OFF V7`, is the signed spec authority for this plan. Capturing this plan may move WP-5 only to `shaped` / `unvalidated`; it does not authorize `building`, `shipped`, or live destination writes.

**Merge Verification:** PR #1 is merged on `main` at `4ccc6b2`, including remediation commit `a8dbda5`. Before implementation, verify the current branch is still based on that `main` state and rerun the Task 0 checks if `main` has moved.

---

## Files and Responsibilities

- Create `internal/destinations/operation.go`: destination-neutral operation types, validation, operation id generation, conflict resolution, and no-leak helpers.
- Create `internal/destinations/operation_test.go`: contract validation, operation id collision, private/secret id handling, conflict semantics, and emitted-surface safety tests.
- Create `internal/destinations/input.go`: versioned destination dry-run input DTO, explicit parse options, and parser from `<sbos-result.json>` into a neutral result/artifact shape.
- Create `internal/destinations/input_test.go`: parser tests for inline artifact bodies, canonicalized artifact path references, unsupported schema versions, and missing artifacts.
- Create `internal/destinations/tolaria/dry_run.go`: Tolaria adapter mapping from SBOS result/artifact fixtures into destination operations.
- Create `internal/destinations/tolaria/dry_run_test.go`: publish, attention, background, skipped, enrichment-blocked, private provenance, redaction, idempotency, conflict, collision, and no-leak mapping tests.
- Create `examples/destinations/tolaria/*.json`: local dry-run inputs and expected-style fixture cases.
- Modify `internal/cli/runner.go`: add `destination dry-run <sbos-result.json> --adapter tolaria --out <dir>`.
- Create `internal/cli/destination_dry_run_test.go`: CLI usage, path safety, deterministic output layout, stdout/summary parity, and no-write boundary tests.
- Modify `README.md`: document destination dry-run, no-write boundary, and verification commands.

## Task 0: Reconcile Current Main Before Delivery

- [ ] **Step 1: Check main state**

Run:

```bash
git switch main
git status --short --untracked-files=no
```

Expected: clean tracked worktree.

- [ ] **Step 2: Confirm merged WP-4 candidate and CLI contracts are stable**

Use local merge/rebase evidence from `main`. If current `main` changed candidate JSON, SBOS result shape, CLI command routing, or path-safety helpers after `4ccc6b2`, stop and revise this plan.

- [ ] **Step 3: Start implementation branch only after stability**

Run:

```bash
git switch -c codex/wp-5-destination-dry-run
```

Expected: new branch from verified `main`.

## Task 1: Destination Operation Contract

- [ ] **Step 1: Write failing validation tests**

Create `internal/destinations/operation_test.go` with tests for:

- valid `create_note`, `attention_preview`, `background_record`, `skip`, and `blocked` operations;
- invalid missing required fields;
- invalid `write_mode` other than `dry_run`;
- `skip` and `blocked` requiring empty `planned_locator` and body;
- `create_note`, `attention_preview`, and `background_record` requiring non-empty `planned_locator`, title, and body.

Run:

```bash
go test ./internal/destinations -run TestValidateDestinationOperation -count=1
```

Expected: FAIL because `internal/destinations` does not exist.

- [ ] **Step 2: Implement minimal contract types**

Create `internal/destinations/operation.go` with:

- `OperationType`;
- `VisibilityLane`;
- `WriteMode`;
- `Operation`;
- `Conflict`;
- `ValidateOperation(operation Operation) error`.

Keep fields JSON-tagged exactly as the spec names: `schema_version`, `operation_id`, `destination_adapter_id`, `source_candidate_id`, `source_record_id`, `idempotency_key`, `operation_type`, `write_mode`, `visibility_lane`, `planned_locator`, `title`, `body`, `metadata`, `blockers`, `authority_ids`.

- [ ] **Step 3: Run validation tests**

Run:

```bash
go test ./internal/destinations -run TestValidateDestinationOperation -count=1
```

Expected: PASS.

## Task 2: Operation IDs, Conflicts, and No-Leak Safety

- [ ] **Step 1: Write failing operation-id tests**

Add tests proving:

- unsafe characters in safe tuples become filename-safe;
- two source ids like `a/b` and `a:b` produce different operation ids because of the fingerprint;
- private/secret-like tuple material produces `<destination_adapter_id>-operation-<fingerprint>` and does not include the raw value;
- operation id is derived before conflict conversion and remains stable after an operation becomes `blocked`.

Run:

```bash
go test ./internal/destinations -run 'TestOperationID|TestPrivacySafeOperationID' -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 2: Implement operation id generation**

Add `GenerateOperationID(destinationAdapterID, sourceCandidateID string, initialType OperationType) string`.

Rules:

- build the unsanitized tuple from destination adapter id, source candidate id, and initial operation type;
- if the tuple is private/secret-like, visible base is `<destination_adapter_id>-operation`;
- otherwise visible base is sanitized tuple;
- append deterministic non-reversible fingerprint;
- never expose the raw tuple except through the fingerprint.

- [ ] **Step 3: Write failing conflict tests**

Add tests proving:

- first operation wins for duplicate non-empty `planned_locator`;
- first operation wins for duplicate non-empty `idempotency_key`;
- a later operation conflicting on both fields gets both blocker codes;
- conflict-blocked operations set `operation_type: blocked` and `visibility_lane: blocked`;
- `metadata.conflicts` is ordered as `planned_locator`, then `idempotency_key`;
- conflict values are sanitized or fingerprinted when they include private/secret-like strings;
- `blocked_count` includes conflict-blocked operations.

Run:

```bash
go test ./internal/destinations -run TestResolveConflicts -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 4: Implement conflict resolution**

Add `ResolveConflicts(operations []Operation) []Operation`.

Rules:

- evaluate operations in deterministic slice order;
- first seen non-empty locator/idempotency key wins;
- later conflicts convert to `blocked`;
- set `visibility_lane` to `blocked`;
- do not change `operation_id`;
- clear `planned_locator` and `body`;
- suppress preview eligibility through `operation_type=blocked`;
- add all applicable blocker codes and ordered `metadata.conflicts`.

- [ ] **Step 5: Run destination contract tests**

Run:

```bash
go test ./internal/destinations -count=1
```

Expected: PASS.

## Task 3: Tolaria Dry-Run Adapter Mapping

- [ ] **Step 0: Write failing destination input parser tests**

Create `internal/destinations/input_test.go` with tests proving:

- `ParseDestinationInput([]byte, ParseOptions)` accepts a versioned local JSON envelope with inline artifact bodies;
- `ParseDestinationInput([]byte, ParseOptions)` accepts artifact path references only when canonicalized paths are inside `ParseOptions.BaseDir` or explicitly allowed fixture directories;
- artifact path references fail when `ParseOptions.BaseDir` is empty, relative to process cwd, outside the canonical input-file directory, or resolving through symlinks outside the allowed roots;
- unsupported `schema_version` fails;
- missing required state, record id, candidate id, authority ids, or artifact body/path fails;
- parsing never re-processes candidate JSON and never calls Slack, Tolaria, network, PB, auth, or provider code.

The initial DTO should be explicit and versioned:

```json
{
  "schema_version": "destination-input/v0.1",
  "result": {
    "state": "dry_run_published",
    "record_id": "candidate-publish",
    "source_candidate_id": "candidate-publish",
    "idempotency_key": "slack:publish",
    "authority_ids": ["WP-5", "DEC-12"],
    "artifacts": [
      {
        "kind": "dry_run_publish",
        "body": "## Snapshot\n..."
      }
    ],
    "safety": {
      "private_provenance": false,
      "redaction_required": false,
      "secret_like": false
    }
  }
}
```

Run:

```bash
go test ./internal/destinations -run TestParseDestinationInput -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 0.1: Implement destination input parser**

Create `internal/destinations/input.go` with:

```go
type ParseOptions struct {
    BaseDir          string
    AllowFixtureDirs []string
}

func ParseDestinationInput(input []byte, opts ParseOptions) (InputResult, error)
```

Implementation rules:

- decode only the versioned destination input DTO or the merged WP-4 result envelope after explicit reconciliation;
- do not re-run candidate processing;
- resolve artifact path references only after canonicalizing `opts.BaseDir`, each `opts.AllowFixtureDirs` entry, and the referenced artifact path;
- treat artifact paths as relative to the canonical input-file directory supplied through `opts.BaseDir`, never process cwd;
- reject artifact paths when `opts.BaseDir` is empty, the canonical artifact path is outside allowed roots, or a symlink/equivalent indirection resolves outside allowed roots;
- return a neutral `InputResult` that the Tolaria planner can consume without knowing CLI envelope details.

- [ ] **Step 0.2: Run destination input parser tests**

Run:

```bash
go test ./internal/destinations -run TestParseDestinationInput -count=1
```

Expected: PASS.

- [ ] **Step 1: Write failing Tolaria mapping tests**

Create `internal/destinations/tolaria/dry_run_test.go` with fixture-style tests proving:

- publish artifact maps to `create_note`;
- attention preview maps to `attention_preview`;
- `StateBackgroundReady` maps to `background_record`;
- `StateSkipped` maps to `skip`;
- `StateNeedsEnrichment` maps to `blocked`;
- private provenance/redaction never produces `create_note`;
- background records have no Markdown preview;
- Tolaria publish preview body contains Snapshot, Source Content, Key Details, Relevance, Signals, Related Sources, and Next Action;
- attention preview does not pretend to be a full processed note unless publish requirements are met.

Run:

```bash
go test ./internal/destinations/tolaria -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 2: Implement Tolaria dry-run mapper**

Create `internal/destinations/tolaria/dry_run.go`.

Expose a small API such as:

```go
func Plan(result destinations.InputResult) ([]destinations.Operation, error)
```

If current `main` introduces a different result artifact shape after this verification, adapt only `ParseDestinationInput` and its explicit `ParseOptions` boundary; keep the Tolaria planner consuming a neutral destination input result.

- [ ] **Step 3: Add fixture cases**

Create JSON fixtures under `examples/destinations/tolaria/` for:

- publish source;
- attention clarification;
- background-only;
- skipped secret-like;
- enrichment-blocked;
- private provenance;
- redaction;
- duplicate locator;
- duplicate idempotency key;
- dual conflict;
- sanitized operation-id collision;
- private/secret operation-id tuple.

- [ ] **Step 4: Run Tolaria tests**

Run:

```bash
go test ./internal/destinations/tolaria -count=1
```

Expected: PASS.

## Task 4: Destination CLI Dry-Run

- [ ] **Step 1: Write failing CLI tests**

Create `internal/cli/destination_dry_run_test.go` proving:

- `mindline destination dry-run <sbos-result.json> --adapter tolaria --out <dir>` succeeds;
- missing `--out` fails;
- unsupported adapter fails;
- output layout is exactly `operations/`, `previews/`, `destination-summary.json`;
- stdout JSON equals `destination-summary.json`;
- summary fields appear in spec order;
- `background_record`, `skip`, and `blocked` have empty `preview_path` and no Markdown preview file;
- `--out` inside the Tolaria vault path is rejected;
- `--out` resolving through a symlink into the Tolaria vault is rejected;
- output paths are canonicalized/resolved before any write;
- generated files and stdout do not contain private/secret fixture strings.

Run:

```bash
go test ./internal/cli -run TestDestinationDryRun -count=1
```

Expected: FAIL before CLI implementation.

- [ ] **Step 2: Implement CLI command routing**

Modify `internal/cli/runner.go`.

Implementation rules:

- reuse existing path-safety helpers where they fit the destination dry-run boundary;
- require `--out`;
- canonicalize the `<sbos-result.json>` path, derive its parent directory, and pass that directory as `ParseOptions.BaseDir` to `ParseDestinationInput`;
- pass only explicit fixture directories as `ParseOptions.AllowFixtureDirs`; do not let parser resolution depend on process cwd;
- canonicalize/resolve `--out` before writing;
- reject output paths at or under `/Users/randyhereman/Young Human Club Dropbox/02. Areas/PKM - Tolaria`, including symlinks or equivalent filesystem indirection resolving into the vault;
- write operation JSON for every operation;
- write Markdown preview only for Tolaria operations with non-empty preview bodies and allowed operation types;
- write `destination-summary.json`;
- print the same summary to stdout.

- [ ] **Step 3: Run CLI tests**

Run:

```bash
go test ./internal/cli -run TestDestinationDryRun -count=1
```

Expected: PASS.

## Task 5: Documentation and Boundary Verification

- [ ] **Step 1: Update README**

Modify `README.md` to document:

- destination dry-run purpose;
- command example;
- output layout;
- no live Tolaria writes;
- no network/auth/Slack/PB runtime dependency;
- merged WP-4 dependency and the current-main reconciliation requirement.

- [ ] **Step 2: Run full tests**

Run:

```bash
go test -count=1 ./...
go test -json ./...
```

Expected: PASS for every package.

- [ ] **Step 3: Run static boundary greps**

Run:

```bash
rg "net/http|http\\.Get|http\\.Post|oauth|token|SLACK|slack\\.com/api|conversations\\.history|chat\\.getPermalink|PKM - Tolaria|Tolaria/" internal cmd README.md
```

Expected: no implementation-surface matches except allowed documentation of the forbidden Tolaria vault path if the test names it explicitly. If documentation/test strings create expected matches, rerun a narrower grep against non-test Go implementation files and record both results.

- [ ] **Step 4: Run generated artifact scans**

Generate dry-run artifacts for the private/secret, conflict, background, publish, and collision fixtures. Capture stdout inside each output directory so scans cover every emitted surface.

Run:

```bash
mkdir -p /private/tmp/mindline-wp5-private-secret
go run ./cmd/mindline destination dry-run examples/destinations/tolaria/private-secret-operation-id.json --adapter tolaria --out /private/tmp/mindline-wp5-private-secret > /private/tmp/mindline-wp5-private-secret/stdout.json

mkdir -p /private/tmp/mindline-wp5-conflict
go run ./cmd/mindline destination dry-run examples/destinations/tolaria/dual-conflict.json --adapter tolaria --out /private/tmp/mindline-wp5-conflict > /private/tmp/mindline-wp5-conflict/stdout.json

mkdir -p /private/tmp/mindline-wp5-background
go run ./cmd/mindline destination dry-run examples/destinations/tolaria/background-only.json --adapter tolaria --out /private/tmp/mindline-wp5-background > /private/tmp/mindline-wp5-background/stdout.json

mkdir -p /private/tmp/mindline-wp5-publish
go run ./cmd/mindline destination dry-run examples/destinations/tolaria/publish-source.json --adapter tolaria --out /private/tmp/mindline-wp5-publish > /private/tmp/mindline-wp5-publish/stdout.json

mkdir -p /private/tmp/mindline-wp5-id-collision
go run ./cmd/mindline destination dry-run examples/destinations/tolaria/sanitized-operation-id-collision.json --adapter tolaria --out /private/tmp/mindline-wp5-id-collision > /private/tmp/mindline-wp5-id-collision/stdout.json
```

Then scan output directories and captured stdout for fixture strings:

```bash
rg "xoxb-|sk_live_|password=|api_key=|bearer |private.example|super-secret|files-pri" /private/tmp/mindline-wp5-*
```

Expected: no matches.

## Task 6: Review and PB Close-Out

- [ ] **Step 1: Run LOOP delivery review**

Use the full Review panel:

- Chain Steward;
- Domain/User Job;
- Systems Architect;
- Delivery Quality;
- Risk/Safety.

Every reviewer must sign off on the same delivery version before lifecycle transition.

- [ ] **Step 2: Capture PB delivery proof**

Capture a `decisions` entry linked to `WP-5` with:

- delivery version;
- tests run;
- static greps;
- artifact scans;
- reviewer sign-offs;
- explicit statement that no live Tolaria writes were added.

- [ ] **Step 3: Update WP-5 lifecycle only after proof**

If delivery proof passes:

```bash
pb update WP-5 --field status=shipped --field validationStatus=validated-staging --field lastValidatedAt=YYYY-MM-DD --note "WP-5 delivered under signed delivery proof."
```

If delivery is not executed in this session, do not move WP-5 beyond `shaped`.

## Plan Self-Review

- Spec coverage: V7 requirements map to Tasks 1-6.
- Placeholder scan: no `TBD`, `TODO`, or open implementation placeholders.
- Type consistency: plan uses `planned_locator`, `metadata.conflicts[]`, `operation_json_path`, and `preview_path` consistently with V7.
- Stop mode: spec/plan are signed; delivery starts only from verified current `main` with TDD and LOOP review.
