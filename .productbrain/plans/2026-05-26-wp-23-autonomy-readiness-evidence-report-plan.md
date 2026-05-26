# WP-23 Autonomy Readiness Evidence Report Plan

## Intent

Deliver the local canonical eval control plane that makes DEC-64 measurable. The implementation must use WP-23 authority, not duplicate WP-28.

## Sequence

1. **Types and schema**
   - Add autonomy-readiness report types under `internal/documents`.
   - Include schema version, evaluator version, threshold, held-out flag, eligibility status, aggregate counts, KR statuses, slices, safety counters, projection status, and top improvement targets.

2. **Input loading**
   - Accept a `semantic-judgment` directory or parent directory.
   - Reuse existing containment patterns for symlink and traversal rejection.
   - Load `judgment-summary.json`, candidate summaries, trace summaries when present, and safe local metadata.

3. **Metrics and eligibility**
   - Compute aggregate counts from `SemanticJudgmentSummary`.
   - Compute accuracy only from counted held-out judgments.
   - Emit `eligible` only when DEC-64 conditions pass.
   - Emit `not_eligible` with blockers otherwise.

4. **Slices and KRs**
   - Produce required slices by source document, source type, candidate kind, confidence, review status, relation presence, relation type, failure reason, evidence-readiness reason, provider/model, and run status where data exists.
   - Emit KR statuses for `KEY-3`, `KEY-4`, `KEY-5`, `KEY-6`, `KEY-7`.

5. **Improvement targets**
   - Generate top targets from closed-vocabulary rules:
     - evidence readiness failures
     - missing taxonomy
     - no candidates extracted
     - model errors
     - high review burden
     - wrong kind / duplicate / unclear concentration
     - not held-out
     - below threshold
   - Include only local artifact references, not source excerpts or private text.

6. **Writer**
   - Write `autonomy-readiness/readiness-report.json`.
   - Write `autonomy-readiness/readiness-report.md`.
   - Use containment-safe output helpers equivalent to existing document writers.

7. **PostHog projection**
   - Add a dedicated projection mapper from the canonical report to safe events.
   - Do not flatten the report struct.
   - Deny-by-default allowed keys.
   - Local report still writes when optional network export fails.
   - Unsafe projection validation fails before network and marks/returns blocked.

8. **CLI**
   - Add usage:
     `mindline documents readiness-report <semantic-judgment-dir-or-parent> --out <dir> [--threshold 0.98] [--held-out]`
   - Print JSON report to stdout.

9. **Tests**
   - Add table-driven eligibility tests.
   - Add slice/KR tests.
   - Add projection allowlist and poisoned-field tests.
   - Add CLI integration tests for disabled telemetry, enabled telemetry, network failure, unsafe projection, and output containment.

10. **Real temp verification**
   - Run semantics on every `temp/*.md`.
   - Run judgment against candidate-producing outputs with source context.
   - Run readiness report.
   - Verify report remains `not_eligible` unless held-out DEC-64 proof truly exists.
   - Verify top improvement targets are actionable and privacy-safe.

11. **Review and close**
   - Run `go test ./...`.
   - Run `git diff --check`.
   - Run `pb audit WP-23`.
   - Run LOOP review panel against final implementation.
   - Capture final Chain decision with what is true and what remains blocked.

## Files Expected

- `internal/documents/autonomy_readiness.go`
- `internal/documents/autonomy_readiness_writer.go`
- `internal/documents/autonomy_readiness_test.go`
- `internal/observability/readiness_projection.go`
- `internal/observability/readiness_projection_test.go`
- `internal/cli/runner.go`
- `internal/cli/documents_decompose_test.go`
- `.productbrain/specs/2026-05-26-wp-23-autonomy-readiness-evidence-report.md`
- `.productbrain/plans/2026-05-26-wp-23-autonomy-readiness-evidence-report-plan.md`

## Stop Conditions

Stop and report if:

- local report eligibility cannot be computed from existing artifacts without inventing ground truth;
- PostHog projection requires raw/private content;
- PB audit fails after reconciliation;
- tests reveal existing judgment artifacts are insufficient for a meaningful report and the fix would expand beyond WP-23.
