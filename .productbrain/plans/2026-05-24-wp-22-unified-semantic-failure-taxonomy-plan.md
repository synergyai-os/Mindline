# WP-22 Plan - Unified Semantic Failure Taxonomy

Status: Draft for LOOP reviewer sign-off
Date: 2026-05-24
Work package: WP-22

## Implementation Plan

1. Add the canonical taxonomy.
   - Define `SemanticFailureReason`.
   - Add validity, ordering, and choice-compatibility helpers.
   - Implement the spec mapping table for every existing `SemanticAcceptanceReason` and `SemanticCalibrationFailureClass` value.
   - Mark fallback mappings from legacy records/classes as inferred.

2. Update semantic judgment artifacts.
   - Bump judgment schema versions.
   - Add `failure_reason`, `secondary_failure_reasons`, and `failure_reason_inferred` where needed.
   - Require a primary reason for every newly written non-accept record.
   - Normalize legacy records deterministically on read.
   - Aggregate `failure_reason_counts` and `judgment_by_failure_reason`.

3. Update CLI and UI review flows.
   - Add `--reason` and repeated `--secondary-reason` to `documents judge-record`.
   - Show taxonomy guidance in `judge-next` markdown.
   - Add a small failure-reason control to `judge-serve`, preserving one candidate at a time and the existing progress/context layout.

4. Update calibration artifacts.
   - Bump calibration summary/review/page schema versions.
   - Add canonical failure reason to item summaries, review items, review context, and report output.
   - Keep `failure_class` as a rollup/compatibility field.

5. Add tests before completing implementation.
   - Record validation: missing reason, accept-with-reason, incompatible reason, valid reason.
   - Mapping coverage for every acceptance reason and calibration failure class.
   - Summary aggregation: failure reason counts and grouped reason counts.
   - Legacy record normalization.
   - Calibration reason mapping and counts.
   - CLI parsing for `--reason` and `--secondary-reason`.

6. Verify with real temp files.
   - Generate semantic judgment bundles for all `temp/*.md`.
   - Record representative accept and non-accept judgments with canonical reasons.
   - Inspect JSON and markdown reports for visible reason counts.
   - Confirm no destination write artifacts are produced.
   - Keep raw temp source excerpts, generated pages, generated candidate excerpts, and generated markdown reports local-only; do not send them to external/LLM reviewers.

7. Product Brain closeout.
   - Update WP-22 fields with the signed scope, risks, architecture, build sequence, and validation result.
   - Run `pb audit WP-22`.
   - Capture any reusable decision/insight only if implementation reveals a durable rule not already represented by WP-22/STD-17/DEC-64.

## Review Plan

Run LOOP review in two gates:

1. Spec/plan gate before implementation:
   - Chain Steward: PB authority and audit fit.
   - Domain/User Job Reviewer: human review job remains understandable.
   - Systems Architect: canonical taxonomy avoids provider and UI coupling.
   - Delivery Quality Reviewer: scope is testable in one PR.
   - Risk/Safety Reviewer: no-human and private-data guardrails remain intact.

2. Delivery gate before PR:
   - Same panel, against implementation evidence, tests, temp verification output, and `pb audit WP-22`.

## Verification Commands

```sh
go test ./...
pb audit WP-22
for f in temp/*.md; do
  name=$(basename "$f" .md)
  go run ./cmd/mindline documents semantics "$f" --out "/private/tmp/mindline-wp22-real/$name"
  go run ./cmd/mindline documents judge "/private/tmp/mindline-wp22-real/$name/semantic-candidates" --out "/private/tmp/mindline-wp22-real/$name-judgment" --source-root temp --source "$(basename "$f")"
done
```

Representative record verification will use `documents judge-record` with `--reason` against generated bundles, then inspect `judgment-summary.json` and `reports/judgment-report.md`.

Calibration verification uses a synthetic fixture, not real private temp excerpts:

```sh
go run ./cmd/mindline documents accept testdata/documents/expected/semantic/semantic-candidates --answer-key testdata/documents/fixtures/semantic-answer-key.json --out /private/tmp/mindline-wp22-calibration
go run ./cmd/mindline documents calibrate /private/tmp/mindline-wp22-calibration/semantic-acceptance --out /private/tmp/mindline-wp22-calibration-review --held-out
```

Expected calibration proof:

- `/private/tmp/mindline-wp22-calibration-review/semantic-calibration/calibration-summary.json` includes `failure_reason_counts`.
- `/private/tmp/mindline-wp22-calibration-review/semantic-calibration/reports/calibration-report.md` includes canonical failure reason counts before compatibility rollups.

UI verification:

```sh
go run ./cmd/mindline documents judge-serve /private/tmp/mindline-wp22-real/<bundle>/semantic-judgment --addr 127.0.0.1:8787 --reviewer wp22-local
```

Expected UI proof:

- Browser renders one active candidate card, plus progress/remaining context.
- Choosing a non-accept decision sends one `failure_reason` in the request body.
- The persisted judgment JSON includes that primary `failure_reason`.
- The updated `judgment-summary.json` increments the matching `failure_reason_counts` key.
- UI screenshots or external review packets must not include real temp source excerpts.

## Done Proof

WP-22 is done when:

- Tests pass.
- Real temp verification shows reason aggregation in judgment reports.
- Calibration output includes canonical reason counts.
- The review UI can record a non-accept decision with a reason.
- `pb audit WP-22` has no failed gates.
- LOOP delivery reviewers sign off.
- A ready PR is pushed.
