# MINDLINE-WP18-SPEC-V2 - LLM Semantic Candidate Review And Judging Harness

## Status

Spec draft for LOOP sign-off. Delivery is not authorized until this spec and the paired plan are signed off and captured on Chain.

## Chain Authority

- `WP-18` - LLM semantic candidate review and judging harness.
- `DEC-79` - WP-18 diagnose/spec/plan sign-off: destination mapping deferred; review/judging harness is next.
- `WP-17` - Provider-agnostic LLM semantic classifier.
- `INS-13` - real temp corpus produces grounded multi-candidate LLM semantic output for source Markdown.
- `STD-18` - semantic previews must include inline evidence excerpts.
- `STD-17` / `ARCH-1` - LLM provider agnostic architecture; OpenAI is only the first provider.
- `DEC-64` - no-human steady state requires >=98% measured held-out accuracy.
- `TEN-6` - pagination without self-contained review context is not reviewable.

## Plain-English Goal

WP-18 makes the generated semantic candidates judgeable.

The reviewer should not open raw JSON, chase candidate IDs, or inspect every artifact manually. Mindline should present one candidate at a time with enough context to decide whether it is useful, correctly typed, evidence-supported, duplicated, unclear, or wrong. Those decisions become local calibration evidence and a batch quality report.

## Problem

WP-17 can generate plausible LLM semantic candidates over real Markdown inputs, but the output is still difficult to evaluate. Without a friendly judgment loop, Mindline cannot measure precision, review burden, failure modes, or progress toward the >=98% held-out no-human bar. Destination mapping would only route unmeasured candidate quality downstream and is therefore out of order.

## In Scope

1. Add a destination-neutral semantic judgment artifact model over existing `semantic-candidates/` runs.
2. Add a CLI command that initializes a local judgment bundle from a semantic run.
3. Add one-item pagination that returns exactly one unjudged candidate per call, with title, kind, confidence, summary, source document, progress, evidence nodes/ranges, and inline evidence excerpts.
4. Add a CLI command that records a judgment for one candidate.
5. Supported judgment choices: `accept`, `reject`, `unclear`, `duplicate`, `wrong-kind`.
6. Persist judgments locally as machine-readable JSON tied to run id, candidate id, source document id, candidate kind, confidence, reviewer id, model/provider metadata when available, choice, note, and timestamp.
7. Support resume by tracking judged candidates and returning the next unjudged candidate until exhausted.
8. Generate a batch quality report with candidate count, judged count, remaining count, accept/reject/unclear/duplicate/wrong-kind counts, precision estimate, review burden, blocked/skipped coverage, and failure-mode counts.
9. Work with deterministic and LLM-generated semantic candidate bundles.
10. Verify against all eligible direct Markdown files in `temp/`, while keeping private temp-derived generated outputs and judgments uncommitted.

## Out Of Scope

- No Product Brain writes.
- No Tolaria writes.
- No destination policy mapping.
- No Product Brain proposal generation or apply transport.
- No auto-accept.
- No no-human claim unless DEC-64 held-out >=98% criteria are met.
- No provider-specific classifier changes.
- No committed private temp source, temp-derived run bundles, judgment bundles, or raw provider responses.
- No separate UI state or persistence. A minimal UI may come later only as a thin projection over this same artifact model.

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

The commands must be local-only and deterministic except for the timestamp on newly recorded judgments.

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
2. `documents judge`, `documents judge-next`, and `documents judge-record` exist and are documented in CLI usage.
3. A reviewer can initialize from a semantic run, page through candidates one at a time, record judgments, resume, and reach exhaustion.
4. Each page is self-contained enough to judge: candidate metadata, candidate summary, evidence ranges, inline excerpts or explicit unavailable reasons, progress, and allowed choices.
5. Judgment artifacts are local JSON and never mutate the source semantic-candidate run.
6. The report includes counts, precision estimate, review burden, blocked/skipped coverage, and failure-mode counts.
7. The implementation rejects path traversal, symlink escape, malformed choices, unknown candidates, duplicate conflicting judgments unless overwrite is explicit in the future, and private/governance markers in committed fixture outputs.
8. Full Go tests pass.
9. Temp verification runs on all eligible direct `temp/*.md` inputs and proves the harness can initialize and page at least one candidate for candidate-producing runs; privacy-blocked/skipped runs are counted separately.
10. Three independent zero-context LLM judges sign off on the final implementation evidence before PR submission.

## Guardrails

- The review harness measures candidate quality; it does not make candidate quality true.
- Human or LLM judgments are calibration evidence, not source-of-truth Product Brain entries.
- Any claim of no-human readiness remains blocked until held-out measured accuracy is >=98%.
- Destination mapping remains the next later step after measured review quality exists.
- External/LLM judge packets must exclude real temp source text, temp-derived excerpts, raw provider responses, and temp-derived judgment artifacts. Judges may receive sanitized fixture excerpts, code diffs, test output, CLI examples from committed fixtures, and aggregate temp counts/results only.
