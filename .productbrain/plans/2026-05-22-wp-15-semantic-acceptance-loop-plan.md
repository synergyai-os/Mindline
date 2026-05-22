# MINDLINE-WP15-PLAN-V5: Semantic Candidate Acceptance Loop And Evaluation Report

Date: 2026-05-22
Status: Draft for LOOP Plan review
Spec: `.productbrain/specs/2026-05-22-next-mindline-slice-diagnosis-and-spec.md`
Stop mode: Full Delivery after existing `WP-15` is reconciled to the signed spec

## Goal

Deliver the next real Mindline PR: a local, destination-neutral acceptance loop that makes semantic candidate quality measurable before destination adapters, live Product Brain transport, or acquisition claims expand.

## Delivery Sequence

1. Reconcile Chain authority before implementation.
   - Confirm `WP-13` and `WP-14` are shipped/validated.
   - Confirm `DEC-54`, `DEC-56`, and `DEC-57` remain the current decision set.
   - Reconcile the existing `WP-15` entry from the signed V5 spec and plan; do not create a second WP-15.

2. Add capture-level answer-key schema fixtures.
   - Create held-out synthetic semantic fixture inputs under `testdata/documents/semantic-acceptance/`.
   - Create answer-key fixtures that define expected semantic outcomes independent of generated candidate IDs.
   - Each expected outcome must include expected state, expected kind, required evidence IDs/ranges or acceptable alternates, title/summary signals, relation requirements, and minimum confidence floor.
   - Include expected-present, expected-absent, accepted, rejected, needs-split, needs-merge, blocked/private, duplicate, missing-evidence, false-negative, and unexpected-candidate cases.
   - Candidate-specific answer rows are allowed only as optional manual-review overrides; they are not the primary quality harness.

3. Add private evidence guardrails.
   - Treat `temp/wp12-meeting-transcript-1-before-after.md`, `temp/wp13-meeting-transcript-1-semantic-v2/`, `temp/wp11-notion-doc-1-before-after.md`, and `temp/wp13-notion-doc-1-semantic-v2/` as ignored local evidence only.
   - Use the real transcript result as the motivating private case: 93 structure nodes, 50 observations, one low-confidence needs-review action candidate, and messy unrelated evidence.
   - Use the real process-document result as the motivating private case: 53 capability observations collapsed into one medium-confidence capability candidate.
   - Do not commit private source material, real participant names, private generated reports, or recognizable labels.
   - Do not claim `temp/` proves WP14 proposal replay; no real Product Brain proposal artifacts were found there.

4. Add failing acceptance schema tests.
   - Validate allowed states and reasons.
   - Reject duplicate expected outcome IDs.
   - Reject expected-present outcomes without evidence requirements.
   - Reject candidate override rows for unknown candidate IDs unless explicitly marked as stale/manual-review evidence.
   - Reject accepted decisions without evidence references or matched expected outcomes.
   - Reject private/unsafe markers in previews and reports.

5. Add destination-neutral acceptance types.
   - Keep types in `internal/documents` or another destination-neutral package.
   - Do not import `internal/productbrain` or destination packages.
   - Preserve source document, candidate, relation, evidence, confidence, review status, blocker, and acceptance decision fields.

6. Implement acceptance evaluator.
   - Read a persisted `semantic-candidates/` run directory.
   - Read a capture-level answer-key JSON file.
   - Match generated candidates to expected outcomes by expected kind, source document, evidence ranges, title/summary signals, relation requirements, and blockers.
   - Produce per-candidate acceptance items.
   - Produce per-expected-outcome evaluation items for found, missed, and expected-absent outcomes.
   - Mark stale/missing candidates, missed expected outcomes, unexpected candidates, and unsupported evidence with explicit reasons.
   - Never mutate source semantic artifacts.

7. Implement writer and report.
   - Write `semantic-acceptance/acceptance-summary.json`.
   - Write per-expected-outcome JSON under `semantic-acceptance/expected-outcomes/`.
   - Write per-item JSON under `semantic-acceptance/items/`.
   - Write per-item previews under `semantic-acceptance/previews/`.
   - Write `semantic-acceptance/reports/quality-report.md`.
   - Ensure every generated file is referenced or intentionally allowlisted.
   - Report review burden, precision-like match rate, recall-like expected-outcome coverage, false positives, false negatives, blocked count, and evidence-missing count.
   - State explicitly that this is not calibrated classifier quality yet.

8. Add CLI command.
   - Add `mindline documents accept <semantic-run-dir> --answer-key <answer-key.json> --out <dir>`.
   - Reject destination/profile/proposal/live-write flags.
   - Print `acceptance-summary.json` to stdout.

9. Add deterministic and regression tests.
   - Golden compare summary, items, previews, and report.
   - Repeated run diff proves determinism.
   - Missing semantic directory, stale answer key, duplicate expected outcomes, unknown candidate override, missed expected outcome, unexpected candidate, traversal, symlink parent, unsupported state, and private-marker cases fail closed.

10. Verify.
   - `go test -count=1 ./internal/documents ./internal/cli`
   - `go test -count=1 ./...`
   - CLI generation against held-out synthetic fixtures.
   - deterministic rerun/diff.
   - leakage scan for Product Brain live hooks, destination hints, Chain IDs, private markers, unsafe marker leakage, and `: null`.
   - live-write scan proving no Product Brain, network, Convex, or destination apply path was added.
   - optional ignored local run against the private `temp/` semantic outputs, with only non-private aggregate findings captured in Chain.

11. LOOP review.
   - Chain Steward: Chain authority, WP-13/WP-14 fit, no premature work-package materialization.
   - Domain/User Job: acceptance loop actually measures candidate quality and review burden.
   - Systems Architect: destination-neutral boundaries and artifact contracts.
   - Delivery Quality: testability, deterministic reports, maintainable schemas.
   - Risk/Safety: private data handling, no live writes, no overclaiming calibrated quality.

12. Chain closeout after sign-off.
   - Capture signed spec/plan proof.
   - Update existing `WP-15` from the signed spec only.
   - Run applicable Product Brain audit/gates.
   - Reconcile audit without changing signed meaning; rerun review if meaning changes.

## Expected Implementation Files

- `internal/documents/types.go`
- `internal/documents/semantic_acceptance.go`
- `internal/documents/semantic_acceptance_writer.go`
- `internal/documents/documents_test.go`
- `internal/cli/runner.go`
- `internal/cli/documents_decompose_test.go`
- `testdata/documents/semantic-acceptance/*`
- `testdata/documents/expected/semantic-acceptance/*`

## Exclusions

- No Product Brain proposal generation.
- No live writes.
- No app-integration transport.
- No workspace profile application.
- No destination adapter changes.
- No LLM/provider integration in this slice.
- No committed private benchmark source material or raw-derived private output.
- No final work-package creation before LOOP and human-owner sign-off.
