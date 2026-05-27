# WP-32 Plan - Source Meaning Preview and Routing Proof

## Goal

Deliver a local, read-only source meaning preview layer over corpus-pressure output so Randy can judge what Mindline read and how it would route meaning without destination writes.

## Sequence

1. Define data contract
   - Add meaning-preview schema constants and summary/item types.
   - Define routing hint, missingness reason, and guardrail fields.
   - Keep destination wording as routing hints, not proposed writes.

2. Build reader / summarizer
   - Locate `corpus-pressure/pressure-summary.json` from either the corpus-pressure root or its parent.
   - Read graph summary/atom/relation artifacts referenced by the pressure output.
   - Build one preview item per processed source.
   - Include source state, candidate counts, atoms, evidence excerpts, relation context, missingness, and routing hints.

3. Build writer
   - Write `source-meaning-preview/meaning-summary.json`.
   - Write `source-meaning-preview/meaning-report.md`.
   - Write `source-meaning-preview/sources/<source-id>.md`.
   - Reject unexpected files and symlink/protected-root escapes.

4. Add CLI
   - Add `mindline documents meaning-preview <corpus-pressure-out-or-parent> --out <dir>`.
   - Validate `--out` with destination-root protection before reading input.
   - Emit JSON summary to stdout.

5. Test
   - Unit-test routing hints and missingness:
     - reference-only/link-only -> `needs_enrichment` or `reference_only`;
     - decision/action/risk/issue/capability -> expected coarse routing;
     - blocked/no-candidate sources stay blocked/no-op.
   - Unit-test duplicate/relation context in previews.
   - CLI-test synthetic corpus-pressure fixture into meaning-preview.
   - Protected-root regression.

6. Self-review as Randy
   - Run a synthetic example and inspect Markdown previews.
   - Run private `/private/tmp` Slack smoke through intake -> pressure -> preview.
   - Judge at least 10 previews from Markdown alone.
   - Iterate if the preview still feels too technical or ambiguous.

7. Verify and publish
   - `go test ./...`
   - `git diff --check`
   - PB audit for WP-32.
   - Leak scan for private runtime identifiers/content.
   - Commit, push, and open PR only after self-review passes.

## Acceptance checks

- Every processed source has a source preview.
- Every preview shows evidence or explicit missingness.
- Every eligible atom has non-applyable routing hints with `write_eligible: false`.
- Summary/report show all write guardrails as zero.
- Product Brain and Tolaria are never called.
- Randy can understand the proof without JSON spelunking.

## Risks

- Routing hints may be mistaken for destination authority.
  - Mitigation: every artifact labels itself preview/calibration only and `write_eligible: false`.

- Weak semantics may look better because routing gives them destination language.
  - Mitigation: first-class `needs_enrichment` and `reference_only` counters.

- Private Slack evidence could leak.
  - Mitigation: committed fixtures synthetic; private runtime only under `/private/tmp`; leak scan before PR.

- Scope could drift into real destination adapters.
  - Mitigation: no destination schemas, profiles, apply payloads, auth, DB, or writes.
