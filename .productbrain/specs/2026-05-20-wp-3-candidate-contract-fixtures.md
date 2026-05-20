# WP-3 Spec: Mindline Candidate Contract and Fixture Pack

Output version: `MINDLINE-WP3-SPEC-V2`

## Authority

- Product: `PROD-1` Mindline.
- Current work package: `WP-3` Mindline normalized candidate fixture pack and adapter contract.
- Prior delivery: `WP-2` CLI dry-run runner, dependent on `WP-1` core gates.
- Roadmap decision: `DEC-6` contract fixtures before Slack dry-run before destination boundary.
- Architecture decisions: `DEC-2`, `DEC-3`, `DEC-4`, `DEC-5`.
- Standards: `STD-5`, `STD-6`, `STD-7`, `STD-11`, `STD-12`.

## Product Model Fit

Verdict: PASS.

Mindline is a headless knowledge-processing engine. A public normalized candidate contract and fixture pack is the next product-shaped layer because it makes the source adapter boundary testable before Slack or any destination adapter can define the schema by accident. This is open-source infrastructure, not Randy-specific automation: future adapters for Slack, YouTube, LinkedIn, GitHub, email, browser capture, or local files should all target the same source-candidate contract.

## Impact Pack

Users affected:

- Randy and agents building Mindline now.
- Future source-adapter authors.
- Future destination-adapter authors who rely on stable core states.
- Reviewers who need concrete fixture behavior rather than prose-only architecture.

Upstream effects:

- Source adapters gain a conformance target.
- Slack adapter work can start from known fixtures instead of inventing shape.

Downstream effects:

- Destination adapter work can trust that core states and artifact expectations are stable.
- README/docs can point contributors to runnable examples.

Governance:

- PB remains source of truth before/during/after work (`STD-11`).
- Private provenance visibility remains publish-blocking (`STD-12`).
- External enrichment/save intent is represented in contract examples (`STD-5`).
- Slack-specific skip/checkpoint expectations are represented as source-adapter cases without making Slack the portable schema owner (`STD-6`, `STD-7`).

Regression map:

- Do not introduce live Slack, Tolaria, network, auth, database, or provider dependencies.
- Do not put destination surface assumptions into the source-candidate contract.
- Do not create fixtures that pass only because test code special-cases fixture paths.
- Do not weaken existing core validation, CLI exit codes, or privacy gates.

Outcome:

- A contributor can read the contract, inspect fixtures, run one command/test, and know whether a normalized candidate is compatible with Mindline.

## Problem

WP-2 made it possible to process a candidate JSON file, but the repository still lacks public examples, contract documentation, and a conformance harness. Without WP-3, the first real adapter could accidentally become the schema authority. That would make Mindline less portable and harder to open-source.

## Direction

Add the first public source-candidate contract surface:

1. Contract documentation that describes the normalized candidate schema, fields, enums, safety model, provenance visibility, idempotency, adapter responsibilities, and out-of-scope destination behavior.
2. A fixture pack under `examples/candidates/` covering all required routes and guardrails.
3. A conformance harness, exposed through tests and a simple command, that runs valid fixtures through the existing CLI/core and verifies deterministic outcomes.

## In Scope

- Public docs for normalized source candidates.
- Public JSON fixtures in `examples/candidates/`.
- A machine-readable fixture manifest if useful for deterministic verification.
- Tests that validate fixture parseability, expected CLI/core states, and output determinism.
- A command or documented `go test` target that acts as the conformance harness.
- README update pointing to contract docs and fixtures.
- Small core/CLI test helper refactors only if needed to avoid duplication.

## Out Of Scope

- Live Slack access.
- Slack connector auth.
- Tolaria writes or Tolaria-specific frontmatter.
- Notion, Obsidian, Mem, or other destination-specific assumptions.
- Network fetching or enrichment.
- LLM classification.
- Database/provider/auth choices.
- Batch processing beyond fixture conformance.
- Changing WP-2 CLI command shape.

## Contract Requirements

The source candidate contract must document:

- `schema_version` current value: `v0.1`.
- Required identity fields: `candidate_id`, `adapter_id`, `external_id`, `captured_at`, `idempotency_key`.
- Required provenance object with visibility-wrapped fields:
  - `permalink`
  - `native_timestamp`
  - `author`
  - `raw_locator`
- Visibility enum: `public`, `private`.
- Required content object:
  - `text`
  - `urls`
  - `attachments`
  - `source_title`
- Enrichment enum: `not_required`, `complete`, `incomplete`, `failed`.
- Classification object:
  - `type`
  - `domain`
  - `topics`
  - `confidence`
  - `needs_clarification`
  - `clarification_reason`
- Safety object:
  - `redaction_required`
  - `secret_like`
  - `empty_content`
  - `private_provenance`
- Desired visibility enum: `background`, `attention`, `publish`, `clarify`.
- State expectations:
  - publish-ready -> `dry_run_published`
  - attention -> `attention_ready`
  - clarify -> `attention_ready`
  - background -> `background_ready`
  - incomplete/failed enrichment -> `needs_enrichment`, unless clarify intent creates attention preview
  - empty or secret-like -> `skipped`
  - private provenance publish -> `background_ready`
  - invalid schema -> CLI exit `2`

The contract must explicitly state that source candidates must not include destination paths, Tolaria frontmatter, Notion page IDs, Obsidian folders, Mem-specific metadata, or final write instructions.

## Fixture Requirements

Create fixtures under `examples/candidates/` with stable names:

- `publish-ready.json`
- `attention-needed.json`
- `clarify-needed.json`
- `background-only.json`
- `needs-enrichment.json`
- `enrichment-clarify.json`
- `skipped-secret-like.json`
- `skipped-empty-content.json`
- `private-provenance.json`
- `redaction-required-attention.json`
- `invalid-schema-version.json`
- `invalid-missing-required.json`
- `save-intent-linked-page.json`
- `slack-empty-artifact.json`
- `path-safety-edge.json`

Valid fixtures must be accepted by the CLI and produce deterministic expected states. Invalid fixtures must fail with exit `2` and `process candidate:` on stderr.

Add `examples/candidates/manifest.json` as the machine-readable conformance source. Each manifest case must include:

- `file`
- `valid`
- `exit_code`
- `expected_state`
- `artifact_count`
- `stderr_contains`
- `assertions`

The manifest must encode this exact expected behavior:

| Fixture | Valid | Exit | State | Artifacts | Required assertions |
| --- | --- | ---: | --- | ---: | --- |
| `publish-ready.json` | yes | 0 | `dry_run_published` | 1 | stdout deterministic; artifact kind `dry_run_publish` |
| `attention-needed.json` | yes | 0 | `attention_ready` | 1 | artifact kind `attention_preview`; no processed-source publish markdown |
| `clarify-needed.json` | yes | 0 | `attention_ready` | 1 | artifact body contains clarification reason |
| `background-only.json` | yes | 0 | `background_ready` | 0 | no artifact with or without `--out` |
| `needs-enrichment.json` | yes | 0 | `needs_enrichment` | 0 | no artifact with or without `--out` |
| `enrichment-clarify.json` | yes | 0 | `attention_ready` | 1 | artifact body contains enrichment blocker |
| `skipped-secret-like.json` | yes | 0 | `skipped` | 0 | no artifact and no secret-like value in stdout |
| `skipped-empty-content.json` | yes | 0 | `skipped` | 0 | empty text accepted only because `safety.empty_content` is true |
| `private-provenance.json` | yes | 0 | `background_ready` | 0 | private values absent from stdout and no artifact files with `--out` |
| `redaction-required-attention.json` | yes | 0 | `attention_ready` | 1 | private/body values redacted; artifact contains `[redacted]` |
| `invalid-schema-version.json` | no | 2 | `error` | 0 | stderr contains `process candidate:` and `unsupported schema_version` |
| `invalid-missing-required.json` | no | 2 | `error` | 0 | stderr contains `process candidate:` and `missing required field` |
| `save-intent-linked-page.json` | yes | 0 | `dry_run_published` | 1 | content preserves captured source plus outbound URL; no network fetch required |
| `slack-empty-artifact.json` | yes | 0 | `skipped` | 0 | adapter-like Slack empty capture is skipped safely |
| `path-safety-edge.json` | yes | 0 | `dry_run_published` | 1 | `--out` path remains inside output directory with sanitized filename |

Fixture contents should be realistic enough to teach adapter authors:

- `save-intent-linked-page.json` should show a captured post/link where the adapter preserves both the captured source and outbound URL context without fetching the network.
- `slack-empty-artifact.json` should encode an empty Slack-like capture as skipped, proving `STD-7` without requiring live Slack.
- `private-provenance.json` must include a private provenance field that would be unsafe to publish and must not leak through publish artifacts.
- `path-safety-edge.json` must include a candidate id that could be dangerous if used as a filename, proving the CLI remains safe with `--out`.

## Conformance Harness

The conformance harness must:

- Run from repo-local tests with `go test -count=1 ./...`.
- Verify every fixture file is valid JSON.
- Verify valid fixtures produce the expected state.
- Verify invalid fixtures return exit `2`.
- Verify deterministic stdout by running at least one valid fixture twice and comparing exact output.
- Verify `--out` path containment for the path-safety fixture.
- Verify private provenance fixture does not write publish artifacts or leak private values.
- Avoid network, Slack, Tolaria, auth, DB, and provider dependencies.

A separate CLI subcommand is not required for WP-3 unless it materially simplifies usage. A documented `go test` conformance target is sufficient.

## Acceptance Criteria

1. Candidate contract documentation exists and is public repo content.
2. Fixtures exist under `examples/candidates/` and cover the required cases.
3. Fixture expected behavior is encoded in tests or manifest, not only prose.
4. `go test -count=1 ./...` verifies fixture conformance.
5. Valid fixtures produce deterministic CLI/core states.
6. Invalid fixtures return exit `2` with `process candidate:` stderr.
7. Private provenance fixture proves `STD-12` no-publish/no-leak behavior.
8. Save-intent fixture represents `STD-5` without network fetching.
9. Slack skip fixture represents `STD-7` without live Slack.
10. Path-safety fixture proves `--out` containment remains safe.
11. Docs state that candidates contain no destination-specific write instructions.
12. README explains how to run the fixture conformance check.

## Implementation Shape

- Prefer `docs/candidate-contract.md` for the public contract.
- Prefer `examples/candidates/` for fixtures.
- Prefer `internal/fixtures` or `internal/cli` tests for conformance only if it keeps package boundaries clean.
- Do not move source-candidate contract logic into Slack or Tolaria packages.
- Do not add external dependencies unless the spec is updated and reviewers sign off.

## Review Requirements

LOOP reviewers must sign off before delivery authority:

- Chain Steward
- Domain/User Job Reviewer
- Systems Architect
- Delivery Quality Reviewer
- Risk/Safety Reviewer
