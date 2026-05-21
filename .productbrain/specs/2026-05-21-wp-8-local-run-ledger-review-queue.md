# WP-8 Local Run Ledger and Review Queue Spec

Spec version: `MINDLINE-WP8-SPEC-V4`
Date: 2026-05-21
Status: Draft for Randy + LOOP review. Not delivery authority until signed in Product Brain.

## Product Brain Authority

- Product: `PROD-1` - Mindline.
- Prior delivery: `WP-7` - local pipeline runner and method routing boundary, shipped and validated-staging.
- Pipeline authority: `DEC-17` - local pipeline runner spec V12 and plan V5 signed off.
- Method boundary: `DEC-15` - Mindline core is method-neutral; BASB/PARA/CODE is first method profile, not core architecture.
- Architecture: `DEC-2`, `DEC-3`, `DEC-4` - Product Brain governs the system; Mindline is a headless Go core with JSON adapter contracts.
- Standards: `STD-10`, `STD-11`, `STD-12`, `STD-7`, `STD-5`.
- Product pressure: `INS-7` - visible Tolaria Inbox count exposes background CODE work as user-facing noise.
- User-facing note quality pressure: `INS-6`, `STD-8`, `STD-9`.

Delivery precondition: PR #3 (`codex/wp-7-local-pipeline-runner`) must be merged into `main`, or Product Brain must capture an explicit branch-base decision to build WP-8 on top of that PR branch. WP-8 depends on `internal/pipeline`, `pipeline dry-run`, method profiles, processor plans, destination summaries, and the artifact writer introduced by WP-7.

Runtime `authority_ids` in WP-8-owned ledger and review artifacts must carry the direct authority set in this order: `PROD-1`, `DEC-17`, `DEC-15`, `WP-8`. Older source/destination decisions remain inherited context for implementation, but they are not the primary authority list copied through WP-8 run artifacts.

## Problem

Mindline can now run a local dry-run pipeline, but each run is still just a one-off artifact tree. That is not enough for catching up Slack history or operating a second-brain engine safely.

The user job is not "produce files." The job is: capture freely, let Mindline process in the background, then see only the items that need attention or are ready for use. To support that job, Mindline needs durable local memory of what happened in each run:

1. which inputs were processed;
2. what state each item reached;
3. which items are blocked, skipped, background-only, or ready for review;
4. which items require Randy's decision or clarification;
5. which outputs can be retried without duplicating prior work;
6. which outputs must never surface because of private provenance or secret-like content.

Without a run ledger and review queue, the next live-source slice would either reprocess the same captures blindly or push raw processing inventory into visible surfaces again. That would repeat the Tolaria noise problem.

## Selected Approach

Create a local-only run ledger and review queue on top of WP-7 pipeline output.

The ledger records every dry-run execution as deterministic JSON files under the chosen `--out` directory. The review queue is derived from item state and blockers, not hand-authored by adapters. This keeps the system headless and portable while giving a future UI, CLI, or destination adapter a stable inspection surface.

WP-8 remains dry-run only:

- no live Slack fetch;
- no live enrichment;
- no live Tolaria writes;
- no database/provider/auth coupling;
- no daemon or background scheduler;
- no permanent global state outside the explicit output directory.

## Product Model Fit

Eligibility verdict: `EXTEND`.

WP-8 extends the canonical pipeline pattern created by WP-7. It does not create a second processing model. The product object is a `run`, containing item outcomes and review actions derived from the pipeline result, processor plan, and destination plan.

Why this is not bespoke:

- future source adapters can produce candidates that flow into the same run ledger;
- future destination adapters can consume the same review queue states;
- future UIs can read the same files without depending on Tolaria;
- retry/idempotency behavior belongs to the engine, not to one Slack batch or one note surface.

## Scope

In scope:

- versioned local run ledger schema;
- per-run manifest;
- per-item state index;
- review queue derived from pipeline item outcomes;
- deterministic run id generation;
- idempotency and duplicate-run behavior for local dry-runs;
- CLI command or flag to produce ledger/review artifacts as part of pipeline dry-run;
- tests proving private/secret data does not leak into ledger or review queue;
- README documentation for how to inspect a run.

Out of scope:

- live Slack API access;
- live Tolaria writes;
- real YouTube/web/PDF/LinkedIn enrichment processors;
- persistent database storage;
- auth providers;
- scheduling/daemon mode;
- UI/dashboard;
- changing the method-profile architecture;
- changing destination write authorization.

## Concepts

### Run

A run is one execution of the local pipeline against one pipeline input file.

Run id:

- deterministic by default for dry-run fixtures;
- derived from input path, input file content hash, method id, destination id, and Mindline run schema version;
- formatted as `run-<16 hex chars>`;
- must not include private source text, private URLs, or secret-like content.

### Ledger

The ledger is the durable local record of the run. It is written under:

```text
<out>/ledger/run-manifest.json
<out>/ledger/items/<record_id>.json
<out>/ledger/index.json
```

The ledger is not a destination adapter. It is engine-local dry-run evidence.

All item identity used in ledger filenames and path segments must be opaque and path-safe. `record_id`, `source_candidate_id`, generated artifact filenames, and generated relative path fields must never contain raw URLs, author names, DM text, source text, tokens, path separators, `..`, private sentinel content, or secret-like content. Unsafe native ids must be transformed to deterministic safe ids before any ledger or queue path is built.

### Review Queue

The review queue is the subset of run items that need human or future processor attention. It is written under:

```text
<out>/review-queue/review-queue.json
<out>/review-queue/items/<record_id>.json
```

The queue is derived from states and blockers. It must not include background-only items unless they require attention.

The review queue uses the same path-safe identity invariant as the ledger. Queue filenames and `review_item_path` values must be generated from safe ids only.

## State Model

WP-8 item states:

- `published_preview` - safe, unblocked item with a destination dry-run preview.
- `background` - processed but not visible/actionable.
- `needs_enrichment` - processor plan says required local artifacts are missing.
- `needs_clarification` - save intent, classification, or routing needs human input.
- `blocked` - safety, unsupported state, conflict, or destination blocker prevents progress.
- `skipped` - intentionally skipped, for example empty or secret-like content.

Review queue inclusion:

- include: `needs_enrichment`, `needs_clarification`, `blocked`;
- include `skipped` only when the reason is reviewable and not secret-like;
- exclude: `published_preview`, `background`, non-reviewable `skipped`.

Private and secret-like items:

- private provenance may appear only as fingerprints and blocker labels;
- private provenance alone is not a review-queue reason. A private-provenance item maps to `background` and `review_required: false` unless it also has a distinct user-actionable blocker such as missing enrichment, clarification, conflict, unsupported state, or an explicit safe-redaction decision;
- secret-like content must not appear in ledger, review queue, stdout, stderr, or previews;
- secret-like skipped items must not produce human-readable body content.

## Run Manifest Contract

`ledger/run-manifest.json`:

```json
{
  "schema_version": "run-ledger/v0.1",
  "run_id": "run-0123456789abcdef",
  "run_mode": "dry_run",
  "pipeline_summary_path": "pipeline-summary.json",
  "method_id": "basb-para-code",
  "destination_id": "tolaria",
  "input_fingerprint": "sha256:<hex>",
  "started_at": "2026-05-21T00:00:00Z",
  "completed_at": "2026-05-21T00:00:00Z",
  "item_count": 3,
  "review_queue_count": 2,
  "states": {
    "published_preview": 1,
    "background": 0,
    "needs_enrichment": 1,
    "needs_clarification": 0,
    "blocked": 1,
    "skipped": 0
  },
  "authority_ids": ["PROD-1", "DEC-17", "DEC-15", "WP-8"]
}
```

Time fields may be injected for deterministic tests. Production dry-run may use current UTC time, but tests must not depend on wall-clock time.

## Ledger Item Contract

`ledger/items/<record_id>.json`:

```json
{
  "schema_version": "run-ledger-item/v0.1",
  "run_id": "run-0123456789abcdef",
  "record_id": "pipeline-text-only",
  "source_candidate_id": "pipeline-text-only",
  "state": "published_preview",
  "review_required": false,
  "blockers": [],
  "pipeline_result_path": "results/pipeline-text-only.json",
  "processor_plan_path": "processors/pipeline-text-only.json",
  "destination_summary_path": "destinations/pipeline-text-only/destination-summary.json",
  "preview_paths": ["destinations/pipeline-text-only/previews/tolaria-pipeline-text-only-create-note-04725ffeb88f99b6.md"],
  "safe_title": "Mindline pipeline text-only fixture",
  "safe_summary": "Safe publish preview generated.",
  "authority_ids": ["PROD-1", "DEC-17", "DEC-15", "WP-8"]
}
```

`safe_title` and `safe_summary` must be redacted or generic for private/secret-like inputs.

`record_id`, `source_candidate_id`, `pipeline_result_path`, `processor_plan_path`, `destination_summary_path`, and `preview_paths` must be generated from safe ids or existing safe artifact paths. They must not serialize private/native ids when those ids contain URLs, source text, path traversal, private sentinel content, or secret-like content.

## Ledger Index Contract

`ledger/index.json` is the stable lookup surface for future CLIs, UIs, and adapters:

```json
{
  "schema_version": "run-ledger-index/v0.1",
  "run_id": "run-0123456789abcdef",
  "item_count": 3,
  "review_required_count": 2,
  "items": [
    {
      "record_id": "pipeline-text-only",
      "source_candidate_id": "pipeline-text-only",
      "state": "published_preview",
      "review_required": false,
      "ledger_item_path": "items/pipeline-text-only.json"
    },
    {
      "record_id": "pipeline-youtube-url",
      "source_candidate_id": "pipeline-youtube-url",
      "state": "needs_enrichment",
      "review_required": true,
      "ledger_item_path": "items/pipeline-youtube-url.json"
    }
  ],
  "authority_ids": ["PROD-1", "DEC-17", "DEC-15", "WP-8"]
}
```

Index item ordering must match pipeline item ordering. The index must not include raw source body, private URLs, private author names, path traversal fragments, or secret-like content. `ledger_item_path` values must be clean relative paths under `items/` and must be generated from safe ids.

## Review Queue Contract

`review-queue/review-queue.json`:

```json
{
  "schema_version": "review-queue/v0.1",
  "run_id": "run-0123456789abcdef",
  "queue_count": 2,
  "items": [
    {
      "record_id": "pipeline-youtube-url",
      "state": "needs_enrichment",
      "reason": "missing_local_youtube_transcript",
      "review_item_path": "items/pipeline-youtube-url.json"
    }
  ],
  "authority_ids": ["PROD-1", "DEC-17", "DEC-15", "WP-8"]
}
```

`review-queue/items/<record_id>.json`:

```json
{
  "schema_version": "review-queue-item/v0.1",
  "run_id": "run-0123456789abcdef",
  "record_id": "pipeline-youtube-url",
  "state": "needs_enrichment",
  "priority": "normal",
  "reason": "missing_local_youtube_transcript",
  "suggested_next_action": "Provide or run a local YouTube transcript processor before destination write.",
  "safe_title": "YouTube capture needs transcript",
  "safe_context": "A YouTube URL was captured, but no local transcript artifact was provided.",
  "links": {
    "pipeline_result_path": "results/pipeline-youtube-url.json",
    "processor_plan_path": "processors/pipeline-youtube-url.json",
    "destination_summary_path": "destinations/pipeline-youtube-url/destination-summary.json"
  },
  "authority_ids": ["PROD-1", "DEC-17", "DEC-15", "WP-8"]
}
```

`record_id`, `review_item_path`, and all queue item links must be safe relative references. Queue contracts must never expose raw native ids that contain private text, raw URLs, path separators, `..`, private sentinel content, or secret-like content.

## CLI Contract

WP-8 should extend the existing command without adding a second runner:

```bash
mindline pipeline dry-run <pipeline-input.json> --method basb-para-code --destination tolaria --out <dir>
```

By default after WP-8, this command writes:

- existing WP-7 pipeline artifacts;
- `ledger/**`;
- `review-queue/**`.

No new command is required in WP-8 unless implementation discovers a strong reason to split inspection. A separate `mindline runs inspect <out>` command is deferred to a later work package.

## Idempotency and Retry

Running the same input into a fresh output directory must produce the same run id and the same logical ledger/review queue content, excluding injected or current time fields.

Running into an existing output directory must be safe:

- if the existing manifest has the same `run_id`, the command may overwrite deterministic dry-run artifacts;
- if the existing manifest has a different `run_id`, the command must refuse unless a future explicit overwrite flag exists;
- WP-8 does not add overwrite flags.

## Acceptance Criteria

1. `go test -count=1 ./...` passes.
2. Existing WP-7 pipeline dry-run tests still pass.
3. Text-only fixture writes a ledger item with `state: published_preview` and does not enter the review queue.
4. YouTube/missing-artifact fixture writes `state: needs_enrichment` and enters the review queue.
5. Private provenance fixture writes only fingerprints/generic context, maps to `background` unless it has a distinct user-actionable blocker, and does not enter the review queue for private provenance alone.
6. Secret-like fixture does not leak secret-like content into ledger, review queue, stdout, stderr, previews, summaries, or generated file/path names.
7. Unsafe native ids containing private URLs, sentinels, path separators, or `..` are transformed to deterministic safe ids before ledger/review filenames and path references are produced.
8. Re-running the same input into the same output directory is deterministic and safe when `run_id` matches.
9. Running a different input into an output directory with an existing different `run_id` fails before writing new ledger/review queue files and leaves existing ledger/review files unchanged.
10. Static scans prove no live Slack/network/browser/LLM/auth/database/Tolaria write behavior.
11. README documents how to inspect the run ledger and review queue.

## Required Static Proof

Final verification must include:

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

- test and dry-run commands exit `0`;
- all `rg` content scans and the `find ... | rg` path-name check return no matches and exit `1`.

## Review Decisions in This Draft

1. `published_preview` items are excluded from the review queue because review queue means human attention required.
2. Secret-like skipped items are excluded from the review queue unless a future package adds a generic aggregate counter; WP-8 must not produce per-item human-readable secret context.
3. Ledger remains JSON under `--out`; persistent database storage is deferred until live capture volume or UI needs prove it is necessary.
