# WP-16 Autonomy Calibration Implementation Plan

Artifact: MINDLINE-WP16-PLAN-V3
Status: Draft for LOOP sign-off
Work package: WP-16
Spec: MINDLINE-WP16-SPEC-V3
Date: 2026-05-22

## Phase 1: Test Contracts First

Add tests that define the WP-16 behavior before implementation:

- Calibration marks below-threshold batches `not_trusted`.
- Held-out batches at or above 0.98 with no blocked/private items become `trusted`.
- Non-held-out batches never become `trusted`, even with perfect fixture scores.
- Failure taxonomy includes all required classes and assigns one class per review item using the SPEC V3 precedence table.
- Scored count is the sum of primary class counts for non-blocked items, so false negatives are not double-counted as missing evidence.
- Pagination returns exactly one item, advances cursor, resumes from disk, and returns `done=true` only after exhaustion.
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

Keep the model destination-neutral and source-agnostic.

## Phase 3: Calibration Engine

Add `internal/documents/semantic_calibration.go` and writer helpers:

- Read WP-15 acceptance outputs from a contained bundle path.
- Rehydrate acceptance items and expected outcome results.
- Compute scored count, measured accuracy, review-burden rate, threshold status, no-human eligibility, and failure-class counts.
- Generate one calibration review item for each acceptance item and each missed expected outcome that lacks a candidate.
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
- Parse optional `--threshold` and `--held-out`.
- Return JSON summaries/pages using existing stdout conventions.

## Phase 6: Real Temp Corpus Verification

Run the implemented workflow against every Markdown file directly under `temp/`:

1. Generate semantic outputs to `/private/tmp/mindline-wp16-temp-corpus`.
2. Generate temporary mechanical answer keys only for pipeline exercise when authoritative held-out labels are absent.
3. Run acceptance and calibration for each file.
4. Drain `calibrate-next` until exhausted for every calibration output.
5. Record aggregate pass/fail evidence without committing private temp content or derived private answer keys.

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
