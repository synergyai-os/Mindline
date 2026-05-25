# WP-25 Plan: Decision-First Semantic Review Lens

Status: signed off for delivery
Date: 2026-05-24

## Implementation Sequence

1. Tighten UI tests around the decision-first contract.
   - Assert the review page includes review-task framing, evidence highlights, collapsed raw details, progress, and contextual failure reason controls.
   - Assert `Review task`, `Should this candidate count`, `Evidence highlights`, `Raw details`, and `Save decision` exist in the served HTML/JS contract.
   - Assert raw relation cards are not expanded by default and visible excerpt output is capped before details are opened.
   - Assert the API still persists compatible reject/unclear/duplicate/wrong-kind reasons and rejects unsafe/incompatible writes through existing validation.
   - Focus test coverage in `internal/cli/documents_decompose_test.go` for the HTML/JS/API contract and existing `internal/documents/documents_test.go` for one-item pagination and read-path non-mutation regressions.

2. Rework the local review page in `internal/cli/semantic_judgment_ui.go`.
   - Keep existing endpoints and artifact schema.
   - Add a decision-first layout with the decision question and candidate essentials before raw details.
   - Limit visible evidence excerpts by default and show counts for hidden excerpts/relations.
   - Move raw relation context, relation IDs, evidence ranges, blockers, and readiness reasons into collapsed detail sections.
   - Change the decision flow so the active decision controls the visible compatible failure reasons before saving.

3. Preserve the document judgment contract in `internal/documents`.
   - Avoid schema churn unless the UI cannot meet the spec without it.
   - Keep JSON artifacts canonical and markdown/read-only projections intact.
   - Keep one-item pagination and read-path non-mutation behavior.

4. Verify with fixtures and real private inputs.
   - Run focused Go tests for semantic judgment UI and document judgment behavior.
   - Run `go test ./...`.
   - Generate/serve judgment bundles from representative `temp/*.md` files locally.
   - Inspect the local browser page for `notion-doc-1.md` and one transcript source using objective checks: decision question visible; progress state visible; no more than five evidence excerpts visible by default; raw relation/details collapsed; hidden counts visible; selecting reject/unclear/duplicate/wrong-kind filters reasons to compatible options before save.
   - Retain only private-content-safe verification evidence: filenames, candidate counts, served status, visible excerpt counts, collapsed-detail status, progress state, and compatible-reason behavior. Do not copy source excerpts into committed artifacts, PR descriptions, or external judge packets.

5. Product Brain and PR readiness.
   - Capture WP-25 on Chain after spec/plan sign-off.
   - Run `pb audit WP-25` and reconcile any warnings that affect authority.
   - Run LOOP delivery review sign-off on the final diff and verification evidence.
   - Commit, push, and open a ready-for-review PR.

## Review Panel

- Chain Steward: verifies WP-25 fits STR-3, INI-1, WS-2, DEC-64, STD-17, STD-18, STD-19, and does not become a permanent dashboard.
- Domain/User Job Reviewer: verifies the reviewer mental model and one-item task are understandable on real artifacts.
- Systems Architect: verifies schema stability, local-only loopback safety, and no destination/model coupling.
- Delivery Quality Reviewer: verifies tests and implementation are scoped, maintainable, and fail-able.
- Risk/Safety Reviewer: verifies private temp content does not leak into committed artifacts or external review packets.

## Validation Commands

```sh
go test ./...
git diff --check
pb audit WP-25
```

## Expected Evidence

- Spec and plan artifacts under `.productbrain/`.
- WP-25 Chain entry linked to INI-1 and WS-2, governed by DEC-64, STD-17, STD-18, and STD-19.
- Focused and full test output.
- Local browser verification against private `temp/` artifacts without committing or externally transmitting private content.
- LOOP reviewer sign-off for spec/plan and final delivery.
