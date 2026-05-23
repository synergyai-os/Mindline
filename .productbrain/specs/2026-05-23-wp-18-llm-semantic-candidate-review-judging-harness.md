# MINDLINE-WP18-SPEC-V3 - LLM Semantic Candidate Review And Judging Harness

## Status

Spec remediation for PR #13. Delivery is not acceptable until this spec and the paired plan are signed off, captured on Chain, implemented, verified, and pushed to the existing PR branch.

## Chain Authority

- `WP-18` - LLM semantic candidate review and judging harness.
- `DEC-79` - WP-18 diagnose/spec/plan sign-off: destination mapping deferred; review/judging harness is next.
- `TEN-8` - PR #13 CLI-only review was rejected because the human review job requires a local UI in this PR.
- `WP-17` - Provider-agnostic LLM semantic classifier.
- `INS-13` - real temp corpus produces grounded multi-candidate LLM semantic output for source Markdown.
- `STD-18` - semantic previews must include inline evidence excerpts.
- `STD-17` / `ARCH-1` - LLM provider agnostic architecture; OpenAI is only the first provider.
- `DEC-64` - no-human steady state requires >=98% measured held-out accuracy.
- `TEN-6` - pagination without self-contained review context is not reviewable.

## Plain-English Goal

WP-18 makes generated semantic candidates reviewable by a human without making the human chase files.

The reviewer should not open raw JSON, chase candidate IDs, inspect every artifact manually, or lose sight of how much work remains. Mindline should present exactly one current candidate at a time in a local UI, while keeping the batch-level context visible: total candidates, judged count, remaining count, current source/run, and judgment distribution. Those decisions become local calibration evidence and a batch quality report.

## Problem

WP-17 can generate plausible LLM semantic candidates over real Markdown inputs, but the output is still difficult to evaluate. WP-18 V2 made a machine-readable judgment bundle and CLI pagination, but Randy rejected the PR because CLI/Markdown review still fails the user job: human review needs a friendly local surface that keeps the reviewer oriented while showing only one decision item. Without that UI, Mindline still cannot measure precision, review burden, failure modes, or progress toward the >=98% held-out no-human bar in a way a user can actually operate.

## In Scope

1. Add a destination-neutral semantic judgment artifact model over existing `semantic-candidates/` runs.
2. Add a CLI command that initializes a local judgment bundle from a semantic run.
3. Add one-item pagination that returns exactly one unjudged candidate per call, with title, kind, confidence, summary, source document, progress, evidence nodes/ranges, and inline evidence excerpts.
4. Add a CLI command that records a judgment for one candidate.
5. Add a local browser review UI command over the same bundle, not a separate persistence model.
6. The UI must show exactly one current candidate item at a time and must not expose a scrollable list of candidate bodies.
7. The UI must keep overall review context visible: total, judged, remaining, accepted/rejected/unclear/duplicate/wrong-kind counts, source/run context, progress, and completion state.
8. UI choices must persist through the same local judgment JSON/report/cursor model used by `judge-record`.
9. Supported judgment choices: `accept`, `reject`, `unclear`, `duplicate`, `wrong-kind`.
10. Persist judgments locally as machine-readable JSON tied to run id, candidate id, source document id, candidate kind, confidence, reviewer id, model/provider metadata when available, choice, note, and timestamp.
11. Support resume by tracking judged candidates and returning the next unjudged candidate until exhausted.
12. Generate a batch quality report with candidate count, judged count, remaining count, accept/reject/unclear/duplicate/wrong-kind counts, precision estimate, review burden, blocked/skipped coverage, and failure-mode counts.
13. Work with deterministic and LLM-generated semantic candidate bundles.
14. Verify against all eligible direct Markdown files in `temp/`, while keeping private temp-derived generated outputs and judgments uncommitted.

## Out Of Scope

- No Product Brain writes.
- No Tolaria writes.
- No destination policy mapping.
- No Product Brain proposal generation or apply transport.
- No auto-accept.
- No no-human claim unless DEC-64 held-out >=98% criteria are met.
- No provider-specific classifier changes.
- No committed private temp source, temp-derived run bundles, judgment bundles, or raw provider responses.
- No separate UI state or persistence; the UI is a projection over the existing local bundle and APIs.
- No multi-item candidate body review surface. The reviewer may see aggregate counts and IDs, but only one current candidate body is visible at a time.

## CLI Contract

Initialize a review bundle:

```sh
mindline documents judge <semantic-run-dir> --out <dir> [--source-root <dir> --source <relative.md>]
```

Return the next review page:

```sh
mindline documents judge-next <semantic-judgment-dir-or-parent>
```

Record a judgment:

```sh
mindline documents judge-record <semantic-judgment-dir-or-parent> --candidate <candidate-id> --choice accept|reject|unclear|duplicate|wrong-kind [--note <text>] [--reviewer <id>]
```

Serve the local review UI:

```sh
mindline documents judge-serve <semantic-judgment-dir-or-parent> [--addr 127.0.0.1:8787] [--reviewer <id>]
```

The commands must be local-only and deterministic except for the timestamp on newly recorded judgments.

`judge-serve` may run a local HTTP server, but it must bind only to loopback/local addresses by default, serve no remote assets, and use the same bundle APIs as the CLI. Non-loopback bind requests must fail closed, including `0.0.0.0`, `::`, LAN IPs, and hostnames that do not resolve exclusively to loopback. Its JSON API is an implementation detail, but tests must prove it returns one current item plus aggregate context, persists posted judgments, accepts loopback binds, and rejects non-loopback binds.

## Artifact Contract

The writer creates:

```text
<out>/semantic-judgment/judgment-summary.json
<out>/semantic-judgment/cursor.json
<out>/semantic-judgment/candidates/<candidate_id>.json
<out>/semantic-judgment/pages/<candidate_id>.md
<out>/semantic-judgment/judgments/<candidate_id>.json
<out>/semantic-judgment/reports/judgment-report.md
```

`judgments/<candidate_id>.json` is created only after a judgment is recorded.

The summary and report must update after each recorded judgment.

The UI must not create an alternate data store. Any UI judgment must result in the same `judgments/<candidate_id>.json`, updated `judgment-summary.json`, updated `cursor.json`, and updated report that `judge-record` would produce.

## UI Contract

The local UI is an operational review tool, not a landing page. It must support these jobs:

1. Start from a semantic-judgment bundle and understand the batch: source/run, total candidates, judged count, remaining count, progress, and judgment distribution.
2. Review one candidate at a time: title, kind, confidence, review status, summary, source document id, evidence ranges, inline excerpts or unavailable reasons, blockers, and relation ids.
3. Decide quickly: choose `accept`, `reject`, `unclear`, `duplicate`, or `wrong-kind`, optionally add a note, and submit.
4. Continue safely: after submit, the UI advances to the next unjudged candidate; when exhausted, it shows completion and final counts.
5. Resume safely: refreshing or restarting the server reads the bundle and continues from the next unjudged candidate.

Non-negotiable UI rules:

- exactly one candidate body is visible at a time;
- overall review context remains visible while deciding;
- persisted state remains local and bundle-contained;
- no external network assets or provider calls are needed to review;
- serving the UI is loopback-only; non-loopback bind addresses fail closed;
- the UI cannot claim no-human readiness or auto-accept quality.

## Evidence Excerpts

When `--source-root` and `--source` are supplied, pages include capped source excerpts derived from candidate evidence ranges. The same containment rules as WP-16 apply:

- source path must be relative;
- source path must be Markdown;
- source path must remain under source root;
- symlink parents are rejected;
- excerpts are capped by range, line count, and total character budget.

When source text is not supplied, the page must still be useful and explicit by showing unavailable excerpt reasons rather than silently omitting evidence.

## Acceptance

WP-18 is complete when:

1. The spec and plan are captured on Chain and `WP-18` is updated from shaped to delivery/review-ready state.
2. `documents judge`, `documents judge-next`, `documents judge-record`, and `documents judge-serve` exist and are documented in CLI usage.
3. A reviewer can initialize from a semantic run, open the local UI, review exactly one candidate at a time, record judgments, resume, and reach exhaustion.
4. The UI shows overall batch context while keeping only one current candidate body visible.
5. Each current item is self-contained enough to judge: candidate metadata, candidate summary, evidence ranges, inline excerpts or explicit unavailable reasons, progress, and allowed choices.
6. UI-submitted judgments and CLI-submitted judgments use the same local JSON/report/cursor model and never mutate the source semantic-candidate run.
7. The report includes counts, precision estimate, review burden, blocked/skipped coverage, and failure-mode counts.
8. The implementation rejects path traversal, symlink escape, malformed choices, unknown candidates, duplicate conflicting judgments unless overwrite is explicit in the future, and private/governance markers in committed fixture outputs.
9. UI/API tests prove one-item state, aggregate context, judgment persistence, bad-choice rejection, loopback-only serving, non-loopback bind rejection, and post-submit advancement/completion.
10. Full Go tests pass.
11. Temp verification runs on all eligible direct `temp/*.md` inputs and proves the harness can initialize, expose UI/API state, and record at least one local smoke judgment for candidate-producing runs; privacy-blocked/skipped runs are counted separately.
12. Three independent zero-context LLM judges sign off on the final implementation evidence before PR update.

## Guardrails

- The review harness measures candidate quality; it does not make candidate quality true.
- Human or LLM judgments are calibration evidence, not source-of-truth Product Brain entries.
- Any claim of no-human readiness remains blocked until held-out measured accuracy is >=98%.
- Destination mapping remains the next later step after measured review quality exists.
- External/LLM judge packets must exclude real temp source text, temp-derived excerpts, raw provider responses, and temp-derived judgment artifacts. Judges may receive sanitized fixture excerpts, code diffs, test output, CLI examples from committed fixtures, and aggregate temp counts/results only.
