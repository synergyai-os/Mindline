# WP-21 Evidence Readiness Gate Implementation Plan

Artifact: MINDLINE-WP21-PLAN-V2
Status: Draft for LOOP sign-off, V2
Work package: WP-21
Spec: MINDLINE-WP21-SPEC-V2
Date: 2026-05-24

## Delivery Premise

WP-21 is a measurement-foundation slice. It must make semantic judgment artifacts say which candidates are eligible for autonomy-readiness evaluation. It must not implement WP-22 taxonomy, WP-23 held-out reports, model improvement, destination writes, or no-human promotion.

## Phase 1: Red Tests

Add focused tests in `internal/documents/documents_test.go`:

1. A fully grounded candidate with source excerpt and loaded relation context passes readiness and is eval-counted.
2. A candidate judged without source context fails readiness with `missing_source_excerpt` and is not eval-counted.
3. A candidate with zero relation ids/context fails readiness with `missing_relation_context`.
4. A candidate whose relation file is missing fails readiness with `missing_relation_context`.
5. A candidate whose relation does not reference the current candidate fails readiness with `invalid_relation_context`.
6. A candidate with blockers or blocked/skipped review status fails readiness.
7. `BuildSemanticJudgmentSummary` aggregates readiness counts and reason counts.
8. `NextSemanticJudgmentPage` Markdown includes readiness status, eval-counted state, and reason codes.
9. `NextSemanticJudgmentPage` item JSON includes readiness fields.
10. `judgment-report.md` includes readiness counts and reason-code counts.
11. Review UI markup includes readiness status and excluded reason codes.
12. Validation fails closed if private/governance markers appear in candidate body, source excerpts, relation context, or blockers.

These are not optional surface checks. If a UI test cannot run browser-side without overbuilding, add a focused renderer/HTML test over the generated UI document instead.

## Phase 2: Data Model

Update `internal/documents/types.go`:

- Add `SemanticEvidenceReadinessStatus`.
- Add reason-code constants.
- Add `SemanticEvidenceReadiness` struct.
- Add readiness fields to `SemanticJudgmentCandidate` and `SemanticJudgmentCandidateSummary`.
- Add readiness counts to `SemanticJudgmentSummary`.

Bump readiness-required artifact schema versions:

- `semantic-judgment-summary/v0.2`
- `semantic-judgment-candidate/v0.2`
- `semantic-judgment-page/v0.2`

Support legacy `v0.1` reads only as fail-closed/non-eval-counted compatibility. Do not make `v0.1` mean both "no readiness contract" and "readiness required."

## Phase 3: Readiness Computation

Update `internal/documents/semantic_judgment.go`:

- Compute readiness when creating each `SemanticJudgmentCandidate`.
- Treat unavailable excerpts as missing source evidence.
- Treat zero relation ids/context as missing relation context.
- Treat unloaded relation ids as missing relation context.
- Treat unrelated relation endpoints or unavailable other endpoint context as invalid relation context.
- Treat candidate blockers, blocked/skipped status, empty title/summary, missing evidence nodes, and missing ranges as failures.
- Clone/sort reason codes deterministically.

Update `BuildSemanticJudgmentSummary` to aggregate readiness counts and reason-code counts.

## Phase 4: Validation and Writers

Update validation in `semantic_judgment.go`:

- Validate readiness status and reason-code enum.
- Reject `eval_counted=true` unless readiness status is pass and reason list is empty.
- Keep private/governance marker checks over readiness fields.
- Add tests or validation coverage proving candidate body, source excerpts, relation context, and blockers cannot carry private/governance markers into written readiness artifacts.
- Add tests proving legacy or absent readiness is never counted as eval-ready.

Update `internal/documents/semantic_judgment_writer.go`:

- Include readiness in report Markdown.
- Ensure persisted summaries and candidate files validate before write.

## Phase 5: CLI / Page / UI Surfacing

Update `semanticJudgmentPageMarkdown`:

- Show evidence readiness status.
- Show eval-counted yes/no.
- Show reason codes for excluded items.

Update `internal/cli/semantic_judgment_ui.go`:

- Show readiness status near the candidate metadata.
- Show reason codes when excluded.
- Keep max-one-item review rule and existing progress display.

## Phase 6: Real Temp Verification

For each direct Markdown file under `temp/`:

1. Run `go run ./cmd/mindline documents semantics <file> --out /private/tmp/mindline-wp21-real/<basename> --classifier llm --llm-provider openai --llm-model gpt-5.2` when an API key is available; otherwise use deterministic classifier and record the limitation.
2. Run `go run ./cmd/mindline documents judge /private/tmp/mindline-wp21-real/<basename> --out /private/tmp/mindline-wp21-judge/<basename> --source-root temp --source <basename>.md`.
3. Inspect `semantic-judgment/judgment-summary.json` for candidate count, eval-counted count, excluded count, and reason counts.
4. Apply this pass rule per file: the bundle has zero candidates, or at least one eval-counted candidate, or every candidate is excluded with non-empty readiness reason counts.
5. Run `judge-next` on at least one bundle with candidates and confirm the item JSON and page Markdown expose readiness.
6. Open `judge-serve` for at least one bundle with candidates and confirm the UI shows readiness status and reason codes.

Do not commit generated `/private/tmp` artifacts or private source excerpts.

## Phase 7: Verification

Run:

```sh
go test ./...
git diff --check
pb audit WP-21
```

Also scan the diff for forbidden behavior:

- Mindline-generated Product Brain/Tolaria destination writes,
- auto-accept/no-human claims,
- provider-specific schema coupling,
- committed `temp/` output.

## Phase 8: LOOP Review and Chain Closeout

Run the selected LOOP panel on the implemented output:

- Chain Steward
- Domain/User Job Reviewer
- Systems Architect
- Delivery Quality Reviewer
- Risk/Safety Reviewer

After sign-off, perform only Product Brain governance closeout, not Mindline destination writeback:

- update WP-21 validation/status according to aggregate evidence,
- capture a DEC with spec/plan/delivery proof,
- link/cite the PR,
- submit the PR ready for review.

Governance closeout is limited to aggregate proof, command evidence, PR links, reviewer verdicts, and WP status. It must not include private `temp/` source excerpts, generated private judgment artifacts, or semantic candidate payloads. It does not authorize Product Brain as a Mindline destination and does not satisfy DEC-64 autonomy proof.

## File Scope

Expected implementation files:

- `internal/documents/types.go`
- `internal/documents/semantic_judgment.go`
- `internal/documents/semantic_judgment_writer.go`
- `internal/documents/documents_test.go`
- `internal/cli/semantic_judgment_ui.go`

Forbidden scope:

- Product Brain proposal/apply code,
- destination adapters,
- model/provider adapter behavior except verification command use,
- committed private `temp/` artifacts.
