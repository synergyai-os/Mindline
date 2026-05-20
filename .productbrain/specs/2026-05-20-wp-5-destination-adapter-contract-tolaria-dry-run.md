# WP-5 Destination Adapter Contract and Tolaria Dry-Run Boundary Spec

Spec version: `MINDLINE-WP5-SPEC-V7`
Date: 2026-05-20
Status: Signed in Product Brain as `DEC-12`; Git artifact captured after PR #1 merge verification.
Stop mode: Spec + Plan only. Delivery requires the signed WP-5 plan, TDD, LOOP delivery review, and Product Brain close-out.

## Product Brain Authority

- Work package: `WP-5` - Mindline destination adapter contract and Tolaria dry-run boundary.
- Product: `PROD-1` - Mindline.
- Sequence decision: `DEC-6` - contract fixtures before Slack dry-run before destination boundary.
- Prior delivery dependency: `DEC-10` / PR #1 - WP-4 Slack dry-run source adapter. Merge verified on `main` at `4ccc6b2`, including remediation commit `a8dbda5`.
- Architecture decisions: `DEC-1`, `DEC-2`, `DEC-3`, `DEC-4`, `DEC-5`.
- Standards: `STD-10`, `STD-11`, `STD-12`, plus `STD-8` and `STD-9` for Tolaria note output quality.
- Existing core surface on `main`: `internal/sbos` validation/routing engine, candidate fixtures, CLI dry-run runner, and Slack dry-run normalization from WP-2/WP-3/WP-4.

## Problem

Mindline can normalize and dry-run process source candidates, but it does not yet have a destination boundary. Without a destination adapter contract, Tolaria can accidentally become the hidden output model, source adapters can leak destination assumptions, and live writes could happen before idempotency, conflict handling, privacy, and explicit write authorization are settled.

Randy needs Mindline to prove where useful knowledge would go without creating visible Tolaria noise or writing to the vault. The first destination should be Tolaria because it is Randy's current PKM surface, but the contract must be portable enough for Obsidian, Notion, Mem, local folders, or custom destinations later.

## Selected Approach

Define a destination adapter contract before adding any live write behavior. The contract has three stages:

1. `plan` - decide the intended destination operation from an SBOS result/artifact without touching the destination.
2. `dry_run` - render deterministic local artifacts that show exactly what would be written and why.
3. `write` - reserved for future work and blocked unless explicit authorization is present.

WP-5 should implement only the contract and Tolaria dry-run boundary. It may produce local dry-run files for inspection, but it must not write into the Tolaria vault.

Alternatives rejected:

- Tolaria-specific renderer first: faster, but makes Tolaria the hidden model and weakens open-source portability.
- Generic destination interface plus live Tolaria write: too risky because authorization, idempotency, conflict behavior, and Inbox visibility are not proven.
- Delay destinations until live Slack is complete: leaves source work ungrounded in output behavior and postpones the most important CODE noise problem.

## In Scope

- A versioned destination operation contract for candidate-processing results and artifacts.
- A Tolaria adapter dry-run implementation that maps SBOS outcomes into planned destination operations.
- Local fixture coverage for publish, attention, background, clarification, private provenance, redaction, conflict, and idempotency cases.
- A CLI dry-run command that accepts local input and writes preview artifacts only to an explicit output directory.
- Explicit operation metadata: destination adapter id, operation id, source candidate id, idempotency key, planned locator, visibility lane, write mode, reason, blockers, and authority ids.
- Tolaria visibility rules:
  - `publish` can plan a durable note only when provenance is public or safely redacted and enrichment is complete.
  - `attention` can plan a visible Inbox note only when Randy must act or clarify.
  - `background` must not create visible Inbox output.
  - private provenance must block publish and either produce no output or an attention-safe redacted preview depending on the SBOS state.
- Conflict behavior in dry-run: detect same planned path/idempotency key within the dry-run batch and emit conflict metadata instead of overwriting.
- README documentation explaining that destination dry-run is local-only and does not write to Tolaria.

## Out of Scope

- No live Tolaria vault writes.
- No Notion, Obsidian, Mem, database, cloud storage, sync, or API destinations.
- No auth, user accounts, provider abstraction, OAuth, or remote service integration.
- No live Slack API or Slack backlog processing.
- No automatic promotion from source candidates into Product Brain.
- No hidden checkpoint or state store beyond explicit dry-run artifacts.
- No UI.
- No broad refactor of the SBOS engine unless required to create a small, testable destination boundary.

## Contract Requirements

### Destination Operation

Every planned operation must include:

- `schema_version`
- `operation_id`
- `destination_adapter_id`
- `source_candidate_id`
- `source_record_id`
- `idempotency_key`
- `operation_type`: `create_note`, `attention_preview`, `background_record`, `skip`, or `blocked`
- `write_mode`: `dry_run` for WP-5
- `visibility_lane`: `publish`, `attention`, `background`, `skip`, or `blocked`
- `planned_locator`
- `title`
- `body`
- `metadata`
- `blockers`
- `authority_ids`

The contract must be destination-neutral. `planned_locator` is an opaque destination-relative locator owned by the destination adapter. It may be a file-like path for Tolaria, a page id for Notion, a database key for a future store, or another adapter-defined locator. The neutral contract must not require Markdown, file extensions, PARA folders, Tolaria frontmatter, or `STD-9` note sections. Tolaria-specific path, Markdown, and body-shape rules belong inside the Tolaria adapter package/tests, not the SBOS candidate schema or source adapter packages.

Operation id invariant:

- Initial `operation_id` is derived before conflict conversion from `<destination_adapter_id>-<source_candidate_id>-<initial_operation_type>`.
- Unsafe filename characters are replaced with `-`.
- Every `operation_id` must append a stable non-reversible fingerprint of the unsanitized tuple.
- The visible base portion of `operation_id` must itself be privacy-safe.
- If the unsanitized tuple contains or may contain private provenance or secret-like material, `operation_id` must use a neutral label plus fingerprint, producing `<destination_adapter_id>-operation-<fingerprint>`, not a sanitized raw tuple.
- If the tuple is safe to display, `operation_id` may use `<sanitized-base>-<fingerprint>`.
- The fingerprint must be deterministic across runs and long enough to make sanitized filename collisions practically impossible for dry-run fixtures.
- Operation ids do not change if a later conflict converts `operation_type` to `blocked`; this keeps artifact names stable and traceable to the initial plan.
- A fixture must cover two source candidate ids that sanitize to the same base and prove both operation JSON files are written without overwrite and appear deterministically in the summary.
- A fixture must cover a source candidate id containing private/secret-like material and prove `operation_id`, filenames, summary paths, stdout, and operation JSON do not contain the raw value.

Neutral per-operation invariants:

- `create_note`
  - `planned_locator` is required and must be destination-relative or adapter-local.
  - `title` is required.
  - `body` is required.
  - `blockers` must be empty.
- `attention_preview`
  - `planned_locator` is required and must be destination-relative or adapter-local.
  - `title` is required.
  - `body` is required and redacted when needed.
  - `blockers` may include clarification, redaction, private provenance, or enrichment reasons.
- `background_record`
  - `planned_locator` is required and must be destination-relative or adapter-local.
  - `title` is required.
  - `body` is required and must be a compact processing record.
  - `blockers` must be empty unless background routing was caused by privacy or redaction.
- `skip`
  - `planned_locator` must be an empty string.
  - `title` must be an empty string.
  - `body` must be an empty string.
  - `blockers` must contain at least one reason.
- `blocked`
  - `planned_locator` must be an empty string.
  - `title` may be present for diagnostics or empty.
  - `body` must be an empty string.
  - `blockers` must contain at least one reason.

Conflict invariants:

- Conflict detection runs after all initial operations are planned and before artifacts are written.
- Duplicate non-empty `planned_locator` values within one dry-run batch are conflicts.
- Duplicate non-empty `idempotency_key` values within one dry-run batch are conflicts.
- The first operation for a conflicting value remains unchanged.
- Every later operation that conflicts with an earlier operation is converted to `operation_type: blocked`, `visibility_lane: blocked`, `planned_locator: ""`, `body: ""`, and no preview artifact is written for it.
- Conflicting operations must include all applicable stable blocker codes:
  - `conflict:planned_locator` for locator conflicts.
  - `conflict:idempotency_key` for idempotency conflicts.
- Conflicting operations must include `metadata.conflicts`, an array ordered by field name in this exact order: `planned_locator`, then `idempotency_key`.
- Each `metadata.conflicts[]` item must include:
  - `field`: `planned_locator` or `idempotency_key`;
  - `value`: the conflicting value;
  - `conflicting_operation_id`: the earlier operation id that won the dry-run plan.
- If a later operation conflicts on both `planned_locator` and `idempotency_key`, it must include both blocker codes and two `metadata.conflicts[]` items in the required order.
- `blocked_count` includes conflict-blocked operations.
- Summary operation entries must set `blocked: true` for conflict-blocked operations.

Global emitted-artifact safety invariant:

- No raw private provenance value or secret-like value may appear in any emitted dry-run surface:
  - operation JSON;
  - summary JSON;
  - stdout;
  - filenames or paths;
  - `operation_id`;
  - `idempotency_key`;
  - `planned_locator`;
  - `title`;
  - `body`;
  - `metadata`;
  - conflict diagnostics.
- When a conflicting value may contain private provenance or secret-like material, `metadata.conflicts[].value` must use a sanitized display value or a stable non-reversible fingerprint instead of the raw value.
- Artifact scans must cover every generated file and stdout, not only publish previews.

### Tolaria Dry-Run Rules

The Tolaria dry-run adapter must map SBOS outputs as follows:

- `ArtifactDryRunPublish` -> `create_note` dry-run operation if no privacy/redaction blocker exists.
- `ArtifactAttentionPreview` -> `attention_preview` dry-run operation.
- `StateBackgroundReady` -> `background_record` dry-run operation with no visible Inbox note. This preserves a deterministic processing trace while respecting `STD-10`.
- `StateSkipped` -> `skip` operation with reason.
- `StateNeedsEnrichment` -> `blocked` operation with enrichment blocker.
- private provenance or redaction blockers -> no publish note; at most a redacted attention preview when SBOS already produced one.

Tolaria paths must be deterministic and preview-only. They should encode intended workflow lane without writing to `/Users/randyhereman/Young Human Club Dropbox/02. Areas/PKM - Tolaria`.

Tolaria adapter-specific invariants:

- Tolaria `create_note` dry-run operations must set `planned_locator` to a relative Markdown path ending in `.md`.
- Tolaria publish preview bodies must include the `STD-9` sections: Snapshot, Source Content, Key Details, Relevance, Signals, Related Sources, and Next Action.
- Tolaria attention preview operations must set `planned_locator` under `00-inbox/`, must be redacted when needed, and must not pretend to be a fully processed `STD-9` source note unless all publish requirements are satisfied.
- Tolaria background record operations must set `planned_locator` under `40-archives/background/` and must not create visible Inbox output.
- Tolaria `background_record` operations write only `operations/<operation_id>.json`; they must not write `previews/<operation_id>.md`, and their summary `preview_path` must be an empty string.
- Tolaria `skip` and `blocked` operations must not produce Markdown preview files.

### CLI

The CLI shape is:

```text
mindline destination dry-run <sbos-result.json> --adapter tolaria --out <dir>
```

The command must:

- require `--out`;
- reject missing or unsafe output directories using the same path-safety standard as previous dry-run commands;
- write operation JSON files and any preview Markdown into the output directory only;
- use this output layout:
  - `operations/<operation_id>.json` for every planned operation;
  - `previews/<operation_id>.md` only for Tolaria operations with non-empty Markdown preview bodies;
  - `destination-summary.json` for the deterministic run summary;
- derive `operation_id` using the operation id invariant above;
- print the same deterministic summary JSON that is written to `destination-summary.json`;
- include summary fields in this order: `destination_adapter_id`, `write_mode`, `operation_count`, `blocked_count`, `operations`, `authority_ids`;
- include each summary operation as `operation_id`, `operation_type`, `visibility_lane`, `operation_json_path`, `preview_path`, `blocked`;
- never write to the Tolaria vault;
- never require network, auth, Slack, or PB runtime access.

## Acceptance Criteria

1. A destination operation contract exists and is covered by validation tests.
2. Tolaria dry-run maps publish, attention, background, skipped, enrichment-blocked, private-provenance, redaction, idempotency, conflict, sanitized operation-id collision, and private/secret operation-id cases deterministically.
3. CLI dry-run requires `--out` and writes only under the explicit output directory.
4. Static boundary checks prove no Tolaria vault writes, no live destination API calls, no Slack API calls, and no auth/provider coupling.
5. Generated artifact and stdout scans prove private provenance and secret-looking content are not leaked into any emitted dry-run surface, including operation JSON, summary JSON, filenames, locators, titles, metadata, conflict diagnostics, and preview Markdown.
6. README explains the destination dry-run contract and its no-write boundary.
7. Tolaria publish preview fixtures include the `STD-9` body sections and use `STD-8` progressive-enrichment judgment: useful now, deeper only when reused.
8. Delivery depends on the merged WP-4 candidate/CLI contracts now present on `main`; if those contracts change after `4ccc6b2`, reconcile before implementation.

## Verification Expectations

- TDD red/green for destination operation validation.
- TDD red/green for Tolaria dry-run mapping.
- TDD red/green for CLI behavior.
- `go test -count=1 ./...`.
- `go test -json ./...`.
- Static grep over implementation surfaces for forbidden live-write/network/auth/Tolaria path patterns.
- Artifact scans over generated dry-run output and stdout for private/secret fixture strings.

## Risks and Guardrails

- Risk: Tolaria becomes the destination model. Guardrail: keep destination operation schema portable and put Tolaria-specific behavior behind an adapter package.
- Risk: visible Tolaria noise returns. Guardrail: background records produce no visible Inbox note; attention output is reserved for action/clarification.
- Risk: live write sneaks in. Guardrail: WP-5 only supports `write_mode: dry_run`; live write is a future package requiring explicit authorization.
- Risk: merged WP-4 candidate output changes after this verification. Guardrail: reconcile current `main` before implementation and revise the input parser boundary if candidate/result shape changes.

## Phase Output

This spec is complete when the LOOP Spec panel signs off on `MINDLINE-WP5-SPEC-V7` or a later revised version and the final signed version is captured in Product Brain.
