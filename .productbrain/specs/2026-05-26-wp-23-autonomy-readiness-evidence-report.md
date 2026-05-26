# WP-23 Autonomy Readiness Evidence Report

## Status

Signed implementation target for the next PR. This materializes existing Chain authority `WP-23`; it does not create a new `WP-28`.

## Authority

- `DEC-64`: no-human semantic ingestion requires held-out accuracy >=98%.
- `STR-2` / `STR-3`: autonomous semantic ingestion before destination writes.
- `PRI-1` and `BR-1`: privacy-by-design and hosted metadata-only observability.
- `TEN-12`: hosted LLM observability can leak private source material.
- `STD-17` and `ARCH-1`: provider-agnostic measurement before trust.
- `WP-21`, `WP-22`, `WP-25`, `WP-26`, `WP-27`: evidence readiness, taxonomy, review lens, agent triage, and trace spine.
- `KEY-3`, `KEY-4`, `KEY-5`, `KEY-6`, `KEY-7`: threshold, taxonomy, slicing, safety, and evidence eligibility.

## Problem

Mindline can now produce semantic candidates, judgment bundles, evidence-readiness counts, failure taxonomy, agent review proposals, and privacy-safe trace summaries. But the system still lacks one canonical go/no-go artifact that answers:

- Are we eligible for the DEC-64 no-human claim?
- What failed, by source type, candidate kind, failure reason, evidence readiness, relation presence, provider/model, and run status?
- What should Codex improve next without Randy manually reading every artifact?

PostHog can help with trend visibility and deterministic metadata checks, but it cannot be the semantic source of truth for private content. PostHog LLM-as-judge paths require input/output and are out of scope for private Mindline traces.

## Outcome

Mindline writes a canonical local autonomy-readiness report from semantic, judgment, and trace artifacts. The report is JSON-first and has a markdown projection. It states `eligible` or `not_eligible` for DEC-64, reports required KRs, ranks the top improvement targets, and optionally projects safe metadata to PostHog through a dedicated allowlisted mapper.

After this ships, Codex can autonomously run the approved corpus, read the local report, identify the highest-leverage failure slices, propose the next experiment, and rerun the loop. Randy remains the authority for held-out labels, privacy/product tradeoffs, destination writes, and any no-human readiness claim.

PR #20 revision requirement: this PR must prove the report is not merely descriptive. It must run a bounded autonomous improvement loop on every `temp/*.md` file, make only general system fixes that are not tuned to the temp corpus, rerun after each fix, and stop early only when either the pre-DEC-64 KR is achieved or the remaining blocker is no longer addressable without ground-truth labels.

## Scope

Implement:

1. `documents readiness-report <semantic-judgment-dir-or-parent> --out <dir> [--threshold 0.98] [--held-out]`
2. Local canonical report under `autonomy-readiness/readiness-report.json`.
3. Markdown projection under `autonomy-readiness/readiness-report.md`.
4. Report schema version, evaluator version, suite/run IDs, input artifact schema versions, threshold status, KRs, slices, counters, and top improvement targets.
5. Optional metadata-only PostHog projection generated only from a dedicated mapper.
6. Tests proving local truth does not depend on PostHog availability.

## Canonical Report Contract

The JSON report must include:

- `schema_version`
- `evaluator_version`
- `suite_id`
- `source_run_ids`
- `held_out`
- `threshold`
- `threshold_status`: `eligible` or `not_eligible`
- `accuracy`, computed as `eval_counted_accepted / (eval_counted_accepted + eval_counted_false_positive + eval_counted_false_negative)` over counted held-out evidence only
- aggregate counts: candidates, judged, remaining, accepted, rejected, unclear, duplicate, wrong-kind, false-positive, false-negative, eval-counted, evidence-ready, evidence-excluded, human-review-required, machine-triaged, model-error
- safety counters: destination writes, auto-accepts, no-human claims, committed private artifacts
- KR status for `KEY-3`, `KEY-4`, `KEY-5`, `KEY-6`, `KEY-7`
- slices by source document, source type, candidate kind, confidence, review status, relation presence, relation type, failure reason, evidence readiness reason, provider/model, and run status when present
- top improvement targets, using closed-vocabulary recommendation codes plus local artifact references
- PostHog projection status: disabled, sent, failed, or blocked unsafe

`threshold_status` may be `eligible` only when:

- `held_out=true`
- `accuracy >= threshold`
- safety counters are all zero
- evidence eligibility is 100% for eval-counted candidates
- taxonomy coverage is 100% for non-accept failures
- no remaining/human-required/model-error blockers affect counted evidence

Otherwise it must be `not_eligible`.

## PostHog Projection Contract

The hosted projection is not the canonical report. It must be generated through a dedicated mapper, not by flattening or reusing the canonical report struct.

Allowed hosted fields:

- event name from an allowlist
- projection schema version
- feature/workflow/command
- salted trace/run/suite IDs
- provider/model only when a real LLM was used
- aggregate counts
- enum-keyed taxonomy counts
- enum-keyed evidence-readiness counts
- threshold status
- KR pass/fail booleans
- metadata-only and redaction flags
- optional latency/token/cost/error metadata when already safe

Forbidden hosted fields:

- `$ai_input`, `$ai_output_choices`, `$ai_input_state`, prompts, completions, tool arguments/results
- source text, excerpts, evidence snippets, transcript turns, document text
- candidate title, summary, body, preview, or free-text rationale
- human reviewer notes
- local paths, Dropbox paths, filenames derived from private source names
- permalinks, URLs, raw source-native IDs
- secrets, salts, API keys, tokens, env values, headers, request/response bodies
- raw provider errors that may quote private content

Local report writing must succeed even if optional hosted export fails. Safety-validation failures in the projection must fail closed before network and mark the projection as blocked.

## KRs

Aggressive but honest delivery KRs:

1. `KEY-3`: report emits DEC-64 `eligible` only at held-out >=98%; real temp verification should remain `not_eligible` unless that is truly proven.
2. `KEY-4`: 100% of non-accept failures in counted evidence carry stable failure taxonomy.
3. `KEY-5`: all required slices exist.
4. `KEY-6`: destination writes, auto-accepts, no-human claims, and committed private artifacts remain zero.
5. `KEY-7`: 100% of eval-counted candidates are evidence-ready.
6. Every failed report lists top improvement targets with local artifact references and closed-vocabulary recommendation codes, including `no_candidates` when a run extracts nothing countable.
7. 0 hosted telemetry events contain forbidden private fields or unsafe values.
8. Pre-DEC-64 self-improvement loop over all `temp/*.md` must show concrete movement: all markdown files complete end to end without crash; direct source-like files produce evidence-ready candidates; intentionally skipped non-source/review artifacts are reported as skipped, not as extraction failures; model errors reach 0; review burden materially decreases from the initial LLM-backed baseline; `threshold_status` remains `not_eligible` unless authoritative held-out judgments exist.

## Anti-Goals

- No Product Brain writes.
- No Tolaria writes.
- No destination apply path.
- No auto-accept.
- No no-human claim below DEC-64 proof.
- No prompt/model tuning.
- No PostHog-as-canonical eval store.
- No hosted LLM-as-judge over private traces.
- No committed private temp artifacts.
- No temp-corpus-specific shortcuts, filename allowlists, candidate title patches, or prompt tweaks retained without measured aggregate gain.

## Verification

Required before PR:

- Unit tests for report eligibility logic.
- Unit tests for required slices and KR statuses.
- Unit tests proving PostHog projection omits poisoned canonical fields.
- Golden/explicit test for allowed hosted keys.
- Negative tests for forbidden keys and unsafe values.
- Test that deterministic runs do not inherit ambient provider/model metadata.
- Test that local report is written when optional network export fails.
- Test that unsafe projection fails before network.
- CLI tests for disabled default and enabled metadata export.
- Real `temp/*.md` smoke through semantics, judgment, and readiness report.
- Bounded self-improvement evidence:
  - Baseline: `/private/tmp/mindline-loop-baseline` produced 69 candidates, 69 evidence-ready/eval-counted, 25 human-review-required, 25 review burden, 25 model errors, and 6 candidate-producing source-like files.
  - Final accepted loop: `/private/tmp/mindline-loop-final2` completed all 8 markdown files, produced 71 candidates, 71 evidence-ready/eval-counted, 10 human-review-required, 10 review burden, 0 model errors, 61 machine-triaged proposal-only candidates, 6 candidate-producing source-like files, and 2 intentionally skipped review-artifact files with empty improvement targets.
  - Stop reason: after five attempted loops, the remaining blocker is `no_judged_eval_outcomes` / `remaining_judgments`, not a local report crash or schema error. Machine triage is deliberately proposal-only and cannot satisfy DEC-64.
- `go test ./...`
- `git diff --check`
- `pb audit WP-23`
