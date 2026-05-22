# WP-16 Autonomy Calibration Implementation Plan

Artifact: MINDLINE-WP16A-PLAN-V7
Status: PR #11 remediation draft for LOOP sign-off
Work package: WP-16
Spec: MINDLINE-WP16A-SPEC-V7
Date: 2026-05-22

## Remediation Premise

PR #11 is not approved because it satisfies one-item pagination mechanically but fails the human-review job. This plan extends the existing WP-16 PR with WP-16A: self-contained review pages that a human can understand without chasing candidate JSON, acceptance JSON, structure artifacts, or source files.

## Three-Loop Standard Raise

### Loop 1 - Minimum Comprehension

- Add failing tests proving the current preview/page is unacceptable when it lacks candidate title, summary, kind, confidence, failure class, reason, and evidence references.
- Change preview generation to use the full `SemanticCalibrationReviewItem`, not only `SemanticCalibrationReviewItemSummary`.

### Loop 2 - Source Grounding

- Add optional `--source-root <dir> --source <relative-markdown-file>` parsing for `documents calibrate`.
- Read only the explicitly supplied source Markdown file under the explicit source root.
- Extract bounded evidence excerpts from existing evidence line ranges.
- Persist excerpts in the review item/page only after private/governance-marker validation.
- Make source absence explicit with `source_excerpts_unavailable`.
- Add source tests for symlinked source, traversal, absolute source path rejection, non-Markdown source rejection, oversized evidence range clamping, and private/governance marker rejection from excerpts.

### Loop 3 - Adjudication-Ready Page

- Add `review_context` and `page_markdown` to `SemanticCalibrationPage`.
- Add expected-outcome detail fields to review context/items for newly generated outputs; allow unavailable notices only for legacy inputs.
- Bump page output schema to `semantic-calibration-page/v0.2`.
- Bump review item schema to `semantic-calibration-review-item/v0.2` if expected-outcome/source context is persisted in review item artifacts.
- Keep legacy `v0.1` review-item reading compatible with explicit `unavailable` notices in the returned `v0.2` page.
- Include explicit adjudication choices and their calibration meaning.
- Add false-negative page tests proving missed expected outcomes are reviewable and explicitly say no candidate matched.
- Update real temp verification to assert every generated non-empty review page contains real candidate content and either evidence excerpts or an explicit source-unavailable notice.

## Phase 1: Test Contracts First

Add tests that define the WP-16 behavior before implementation:

- Calibration marks below-threshold batches `not_trusted`.
- Held-out batches at or above 0.98 with no blocked/private items become `trusted`.
- Non-held-out batches never become `trusted`, even with perfect fixture scores.
- Failure taxonomy includes all required classes and assigns one class per review item using the SPEC V7 precedence table.
- Scored count is the sum of primary class counts for non-blocked items, so false negatives are not double-counted as missing evidence.
- Pagination returns exactly one item, advances cursor, resumes from disk, and returns `done=true` only after exhaustion.
- `calibrate-next` page output includes title, summary, confidence, failure class, acceptance state, evidence references, and adjudication choices.
- When `documents calibrate --source-root <dir> --source <relative-markdown-file>` is supplied, review pages include bounded source excerpts from evidence line ranges.
- When no source is supplied, review pages explicitly say source excerpts are unavailable.
- New matched expected-outcome pages include real expected state, expected kind, matched candidate id, required evidence, alternates, title/summary signals, relation requirements, confidence floor, and notes.
- New false-negative pages include real expected-outcome details, an explicit no-candidate-matched explanation, evidence context when available, and adjudication choices.
- Legacy expected-outcome pages with missing fields include unavailable notices and a not-fully-adjudication-ready marker.
- Private/governance markers are rejected from calibration outputs.
- Symlinked output parents are rejected.
- Malicious or symlinked input artifacts are rejected for `semantic-acceptance` and `semantic-calibration` bundles, including `acceptance-summary.json`, expected-outcome files, item files, cursor files, and path fields loaded from those artifacts.
- CLI usage covers `documents calibrate` and `documents calibrate-next`.

## Phase 2: Data Model

Extend `internal/documents/types.go` and `internal/documents/ids.go` with:

- `SemanticCalibrationSummary`
- `SemanticCalibrationReviewItem`
- `SemanticCalibrationCursor`
- `SemanticCalibrationPage`
- failure-class constants
- schema-version constants
- stable path helpers for review items and previews
- `SemanticCalibrationReviewContext`
- `SemanticCalibrationEvidenceExcerpt`
- `SemanticCalibrationAdjudicationChoice`
- `page_markdown` on `SemanticCalibrationPage`
- expected-outcome context fields on calibration review items/pages
- schema-version constants for `semantic-calibration-page/v0.2` and any persisted review-item `v0.2` change
- `semantic-acceptance-expected-outcome/v0.2` fields or compatible persisted answer-key snapshot fields for new acceptance outputs

Keep the model destination-neutral and source-agnostic.

## Phase 3: Calibration Engine

Add `internal/documents/semantic_calibration.go` and writer helpers:

- Read WP-15 acceptance outputs from a contained bundle path.
- Rehydrate acceptance items and expected outcome results.
- Compute scored count, measured accuracy, review-burden rate, threshold status, no-human eligibility, and failure-class counts.
- Generate one calibration review item for each acceptance item and each missed expected outcome that lacks a candidate.
- Build content-rich review pages from full review items.
- Optionally attach bounded source excerpts from an explicitly supplied Markdown source file.
- Clamp source excerpts to 3 ranges, 6 lines per range, 1,200 characters per range, and 4,000 characters total per item.
- Store only safe relative source labels, never absolute local source paths.
- Preserve compatibility reading existing `v0.1` review items while emitting `v0.2` pages.
- Preserve answer-key expected-outcome details when building new acceptance expected-outcome result artifacts so calibration pages do not rely on unavailable placeholders.
- Validate private/governance-marker safety before writing.
- Validate input containment before reading each summary, expected-outcome, item, cursor, or path-referenced artifact.
- Write `semantic-calibration/` artifacts deterministically.

## Phase 4: Paginated Cursor

Implement `NextSemanticCalibrationReviewPage`:

- Resolve `semantic-calibration` from either the directory itself or its parent.
- Load `calibration-summary.json` and `cursor.json`.
- Return a page containing exactly one item when available.
- Persist cursor progress after each successful page.
- Return `done=true` with no item when exhausted.
- Reject cursor/item paths that escape the calibration bundle.

## Phase 5: CLI

Update `internal/cli/runner.go`:

- Extend usage text.
- Add `documents calibrate`.
- Add `documents calibrate-next`.
- Parse optional `--threshold`, `--held-out`, `--source-root <dir>`, and `--source <relative-markdown-file>`.
- Return JSON summaries/pages using existing stdout conventions.
- Assert `calibrate-next` stdout uses `semantic-calibration-page/v0.2`.

## Phase 6: Real Temp Corpus Verification

Run the implemented workflow against every Markdown file directly under `temp/`:

1. Generate semantic outputs to `/private/tmp/mindline-wp16-temp-corpus`.
2. Generate temporary mechanical answer keys only for pipeline exercise when authoritative held-out labels are absent.
3. Run acceptance and calibration for each file.
4. Drain `calibrate-next` until exhausted for every calibration output.
5. Record aggregate pass/fail evidence and content-rich page checks without committing private temp content or derived private answer keys.

The verification result must clearly distinguish mechanical pipeline pass from real semantic-accuracy proof.

## Phase 7: Review and Chain Close

- Run full `go test ./...`.
- Run `pb audit WP-16 --phase shaping --verbose`, then update WP-16 status/proof fields according to local lifecycle rules.
- Capture final decision/proof in PB.
- Rerun the LOOP review panel for final implementation output.
- Close the PB session after durable truth is updated.

## Workstream Ownership

Single orchestrator implementation is preferred for this slice because the same types, writer, and CLI parser are tightly coupled. Subagents are used for phase review/sign-off, not parallel file edits.

## Non-Negotiables

- No destination writes.
- No private `temp/` content committed to git.
- No trust claim from self-derived labels.
- One CLI review page means one item maximum.
- Human adjudication remains temporary calibration evidence.
