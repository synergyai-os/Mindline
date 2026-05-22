# MINDLINE-WP14-PLAN-V1: Product Brain Proposal Identity And Replay

## Goal

Deliver `MINDLINE-WP14-SPEC-V1`: stable source-native Product Brain proposal identity, replay proof, explicit role handling, and no live-write expansion.

## Sequence

1. Add red tests for proposal identity.
   - Update `internal/productbrain/proposal_test.go`.
   - Prove `BuildExternalRef` or its replacement excludes `runID`.
   - Prove same source candidate + intent gives same externalRef across two run IDs.
   - Prove idempotency keys differ across the same replay runs.

2. Add red tests for review queue source identity.
   - Update `internal/pipeline/runs/ledger_test.go`.
   - Prove `BuildReviewQueueItem` and `BuildReviewQueueItems` persist `source_candidate_id`.
   - Prove unsafe source candidate IDs remain sanitized.

3. Add red tests for full proposal generation replay.
   - Update `internal/productbrain/propose_test.go`.
   - Build two synthetic run directories with different run IDs and the same review source candidate.
   - Run `Propose` on both.
   - Assert proposal external refs match and idempotency keys differ.

4. Add red test for unknown role fail-closed behavior.
   - Update `internal/productbrain/resolver_test.go`.
   - Add an unknown field-map role to a profile.
   - Assert blocked proposal with `unsupported_field_role`.

5. Implement the smallest contract changes.
   - Add `SourceCandidateID` to `runs.ReviewQueueItem`.
   - Populate it from `LedgerItem.SourceCandidateID`.
   - Add `SourceCandidateID` to `productbrain.ResolveInput` and `ProposalInput`.
   - Build `ExternalRef` from source candidate ID and intent.
   - Keep `BuildIdempotencyKey(runID, proposalID)` run/proposal scoped.
   - Make `valueForRole` return `(value, ok)` or equivalent to fail unknown roles.

6. Update fixtures affected by the review queue schema.
   - Update `testdata/productbrain/runs/reviewable/review-queue/items/*.json`.
   - Update any expected artifact fixture that serializes review queue items.

7. Run targeted verification.
   - `go test -count=1 ./internal/pipeline/runs ./internal/productbrain ./internal/cli`

8. Run full verification and live-write scan.
   - `go test -count=1 ./...`
   - `rg -n "upsertByExternalRef\\(|writeEntry\\(|convex|net/http|http://|https://" internal/productbrain internal/pipeline/runs`
   - Inspect any matches and confirm they are not live write/network implementation.

9. Run LOOP delivery review.
   - Chain Steward: authority, Product Brain boundary, DEC-54/WP-14 fit.
   - Domain/User Job: identity hardening as prerequisite to semantic acceptance loop.
   - Systems Architect: source/run/proposal identity split.
   - Delivery Quality: tests, fixture scope, maintainability.
   - Risk/Safety: replay/idempotency, no live writes, redaction boundary.

10. Close Product Brain.
   - Capture delivery evidence.
   - Mark WP-14 shipped/validated only if verification and reviewers pass.
   - Record follow-up that the next larger step is the semantic candidate acceptance/evaluation harness.

## Risks

- The slice could expand into a broad identity remodel. Keep it limited to the review/proposal path unless tests prove upstream data is missing.
- The replay test could accidentally prove same-run determinism instead of cross-run behavior. Use distinct run IDs explicitly.
- Blocking unknown roles may affect profiles; keep supported roles explicit and tests clear.
- Do not add Product Brain live transport, kernel calls, or network code.
