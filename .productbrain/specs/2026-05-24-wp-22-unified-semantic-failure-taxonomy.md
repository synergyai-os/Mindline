# WP-22 Spec - Unified Semantic Failure Taxonomy

Status: Draft for LOOP reviewer sign-off
Date: 2026-05-24
Work package: WP-22
Authority: PROD-1, DOMAIN-1, INI-1, WS-2, KEY-4, KEY-6, STD-17, DEC-64, STD-18, STD-19

## Diagnosis

WP-18 through WP-21 made semantic review inspectable: one item at a time, source excerpts, relation context, grouped analytics, and an evidence-readiness gate. The remaining blocker is comparability. The system currently has at least three overlapping failure languages:

- Human judgment choices: `reject`, `unclear`, `duplicate`, `wrong-kind`.
- Acceptance reasons: `unsupported_evidence`, `missing_evidence`, `too_broad`, `unexpected_candidate`, and related codes.
- Calibration rollups: `false_positive`, `false_negative`, `missing_evidence`, `needs_review_ambiguity`, and related classes.

Those are useful at different layers, but they are not one stable taxonomy. As a result, a reviewer decision, a calibration item, and a future LLM-as-judge decision cannot be compared without bespoke interpretation. That weakens KEY-4 because taxonomy coverage is not measurable, and it weakens KEY-6 because no-human readiness can be argued from imprecise labels instead of strict failure evidence.

## Product Decision

WP-22 establishes a single canonical `SemanticFailureReason` enum for semantic evaluation. Existing review choices remain user-action states, and existing calibration failure classes remain compatibility rollups, but neither is the canonical taxonomy.

The canonical reason code is the shared language across:

- Human review records.
- Human review summaries and reports.
- Calibration review items and reports.
- Future LLM judge outputs.

This PR must not add a new LLM judge implementation. It creates the provider-agnostic contract that an LLM judge would have to emit later.

## In Scope

1. Define canonical semantic failure reason codes in code and docs.
2. Require every non-accept human judgment record to carry exactly one primary `failure_reason`.
3. Allow optional `secondary_failure_reasons` for additional diagnosis without changing the primary aggregation.
4. Validate choice/reason compatibility:
   - `accept` has no failure reason.
   - `duplicate` uses `duplicate`.
   - `wrong-kind` uses `wrong_kind`.
   - `unclear` uses ambiguity or missing-context reasons.
   - `reject` uses a concrete failure reason, not a vague review action.
5. Add reason counts and reason groupings to judgment summaries and reports.
6. Add reason selection and taxonomy guidance to the judgment CLI/UI without changing the one-item-at-a-time review model.
7. Add canonical reason counts to calibration summaries and reports while retaining existing failure class fields as compatibility rollups.
8. Preserve the safety floor: zero destination writes, zero Product Brain apply transport, and no no-human claim unless DEC-64 proof is met.
9. Verify against real `temp/*.md` semantic judgment bundles and targeted tests.

## Out of Scope

- New LLM judge calls or model scoring.
- Provider-specific tuning.
- Destination writes to Tolaria, Product Brain, or any external system.
- Product Brain kernel/app integration.
- Replacing the review UI with a larger workflow product.
- Changing the 98% held-out autonomy threshold.

## Canonical Reason Codes

The canonical enum is:

- `wrong_kind`
- `unsupported_evidence`
- `missing_evidence`
- `unsafe_or_private`
- `duplicate`
- `too_broad`
- `too_narrow`
- `stale_or_contradicted`
- `ambiguous`
- `missing_expected_outcome`
- `unexpected_candidate`
- `relation_error`
- `source_scope_error`
- `other`

`correct` and `accepted` are not failure reasons. Accepted judgments are counted through `choice=accept`, not through the failure taxonomy.

## Compatibility Mapping

Existing `SemanticAcceptanceReason` values map into `SemanticFailureReason` as follows:

| Existing acceptance reason | Canonical failure reason | Inferred? | Notes |
| --- | --- | --- | --- |
| `correct` | none | false | Accepted output; not a failure. |
| `wrong_kind` | `wrong_kind` | false | Same semantic meaning. |
| `unsupported_evidence` | `unsupported_evidence` | false | Same semantic meaning. |
| `missing_evidence` | `missing_evidence` | false | Same semantic meaning. |
| `unsafe_or_private` | `unsafe_or_private` | false | Same semantic meaning and safety rollup. |
| `duplicate` | `duplicate` | false | Same semantic meaning. |
| `too_broad` | `too_broad` | false | Same semantic meaning. |
| `too_narrow` | `too_narrow` | false | Same semantic meaning. |
| `stale_or_contradicted` | `stale_or_contradicted` | false | Same semantic meaning. |
| `ambiguous` | `ambiguous` | false | Same semantic meaning. |
| `missing_expected_outcome` | `missing_expected_outcome` | false | Same semantic meaning. |
| `unexpected_candidate` | `unexpected_candidate` | false | Same semantic meaning. |
| unknown/empty legacy value | `other` | true | Only for legacy read compatibility; newly written artifacts must reject unknown values. |

Existing `SemanticCalibrationFailureClass` values map into canonical reasons only when no more specific acceptance reason is available:

| Existing calibration class | Canonical fallback reason | Inferred? | Notes |
| --- | --- | --- | --- |
| `accepted` | none | true | Accepted output; not a failure. |
| `false_positive` | `unexpected_candidate` | true | Rollup fallback. Prefer item acceptance reason when present. |
| `false_negative` | `missing_expected_outcome` | true | Rollup fallback. |
| `missing_evidence` | `missing_evidence` | true | Rollup fallback; may collapse unsupported vs missing evidence. |
| `relation_error` | `relation_error` | true | Rollup fallback. |
| `source_scope_error` | `source_scope_error` | true | Rollup fallback. |
| `blocked_private` | `unsafe_or_private` | true | Safety fallback. |
| `duplicate` | `duplicate` | true | Rollup fallback. |
| `needs_review_ambiguity` | `ambiguous` | true | Rollup fallback; may hide too-broad/too-narrow/stale detail. |
| `other` | `other` | true | Rollup fallback. |
| unknown/empty legacy value | `other` | true | Only for legacy read compatibility; newly written artifacts must reject unknown values. |

Mapping rule: if an artifact has both an acceptance reason and a calibration failure class, the acceptance reason wins because it carries more specific diagnosis. The calibration class is retained as a rollup field, not used as the primary taxonomy unless the specific reason is unavailable.

## Choice Compatibility

New human judgment records must satisfy this table:

| Judgment choice | Required primary failure reason |
| --- | --- |
| `accept` | none |
| `reject` | one of `unexpected_candidate`, `unsupported_evidence`, `missing_evidence`, `too_broad`, `too_narrow`, `stale_or_contradicted`, `unsafe_or_private`, `relation_error`, `source_scope_error`, `other` |
| `unclear` | one of `ambiguous`, `missing_evidence`, `unsupported_evidence`, `relation_error`, `source_scope_error`, `other` |
| `duplicate` | exactly `duplicate` |
| `wrong-kind` | exactly `wrong_kind` |

Secondary reasons may use any canonical failure reason except the primary reason. Secondary reasons are optional and never determine the primary aggregation.

## Schema Contract

New judgment artifacts bump schema versions because reason codes become part of the review contract:

- `semantic-judgment-summary/v0.3`
- `semantic-judgment-candidate/v0.3`
- `semantic-judgment-record/v0.2`
- `semantic-judgment-page/v0.3`

New calibration artifacts bump schema versions when canonical failure reason fields are emitted:

- `semantic-calibration-summary/v0.2`
- `semantic-calibration-review-item/v0.3`
- `semantic-calibration-page/v0.3`

Compatibility behavior:

- Legacy judgment records without a reason are read as legacy and normalized with deterministic defaults only to preserve old bundles:
  - `reject` -> `unexpected_candidate`
  - `unclear` -> `ambiguous`
  - `duplicate` -> `duplicate`
  - `wrong-kind` -> `wrong_kind`
  - `accept` -> no failure reason
- Newly written records must never omit the primary reason for non-accept choices.
- Legacy calibration failure classes remain readable, but new reports aggregate by canonical failure reason first.

## Acceptance Criteria

1. A non-accept `judge-record` call without `--reason` fails.
2. An `accept` `judge-record` call with `--reason` fails.
3. An incompatible reason/choice pair fails, for example `--choice duplicate --reason missing_evidence`.
4. A valid non-accept judgment writes exactly one primary reason and optional secondary reasons.
5. Judgment summaries aggregate `failure_reason_counts`.
6. Judgment reports include failure taxonomy counts and choice/reason guidance.
7. The review UI records a primary reason for every non-accept decision while keeping max one candidate visible at a time.
8. Calibration summaries expose `failure_reason_counts` and calibration reports prioritize canonical reason counts before old rollups.
9. Real `temp/*.md` verification can generate semantic judgment bundles and record at least one accepted and one non-accepted judgment with reason aggregation visible.
10. `go test ./...` passes.
11. `pb audit WP-22` has no failed gates after the WP record is updated.
12. UI proof shows that `judge-serve` sends a persisted `failure_reason` for a non-accept decision and still renders only one candidate item in the active review page.

## Guardrails

- The taxonomy is evaluation evidence, not permission to write autonomously.
- Evidence readiness remains a separate eligibility gate. Failure reasons explain semantic quality after a candidate is reviewable; readiness reasons explain why a candidate is excluded from evaluation.
- Review UI changes must be minimal and must not hide source excerpts or relation context.
- Provider-agnostic architecture is preserved: reason codes are plain artifact fields, not OpenAI/Claude/Gemini/OpenRouter concepts.
- Real `temp/*.md` source text, source excerpts, candidate excerpts, judgment pages, and generated markdown reports from real temp verification are local-only. They must not be sent to any LLM, external reviewer, or external service. External review packets may include only sanitized summaries, schema diffs, commands run, aggregate counts, and non-sensitive synthetic examples.
