# MINDLINE-NEXT-SLICE-DIAGNOSIS-SPEC-V5: Semantic Candidate Acceptance Loop

Date: 2026-05-22
Status: Draft for LOOP Diagnose + Spec review
Stop mode: Full Delivery after existing `WP-15` is reconciled to this signed spec

## Chain Authority

- Product: `PROD-1` Mindline.
- Work package: existing `WP-15 Semantic candidate acceptance loop`, captured as a roadmap draft before V4 sign-off. This spec governs and reconciles that existing entry; no second WP-15 should be created.
- Product direction: `DOMAIN-1` says Product Brain is a future destination and authority consumer, not the Mindline product. Mindline remains the ingestion engine with source, method, processor, ledger, and destination boundaries.
- Recent diagnosis: `DEC-54` says the next big product step is a destination-neutral semantic candidate acceptance loop, with `WP-14` as the immediate enabling PR.
- Delivered prerequisite: `WP-14` / `DEC-56` shipped source-native Product Brain proposal identity and replay hardening.
- Delivered semantic substrate: `WP-13` / `DEC-50` shipped destination-neutral semantic observations, relations, and candidates.
- Quality correction: `DEC-57` says semantic relations must be built from classified candidates to avoid endpoint/status drift.
- Standards: `STD-11` Product Brain SSOT, `STD-12` private provenance blocks publish, `STD-16` proposal adapters fail closed and remain bundle-contained.

## Diagnosis

The third-party integration acceptance review does not make live Product Brain integration the next Mindline PR. Product Brain's live kernel write path remains unbuilt and out of Mindline's control. Mindline's one blocking integration defect, run-scoped `externalRef.id`, has already been addressed by `WP-14`.

The real remaining product risk is that Mindline still cannot prove semantic candidate quality. `WP-13` created the evidence model, but it did not create an acceptance loop where humans or fixtures can adjudicate candidates, record accepted/rejected/split/merged outcomes, and report precision or review burden. The acqui-tech evaluation explicitly says runtime value changes only when Mindline ships a live slice with calibrated classification measured on held-out captures. The next PR should therefore make candidate quality measurable before any destination adapter or live transport expands.

The real private review artifacts under ignored `temp/` sharpen this diagnosis:

- `temp/wp12-meeting-transcript-1-before-after.md` shows transcript structure improved from 188 flat segments to 93 structure nodes: one document plus 92 `transcript_turn` nodes.
- `temp/wp13-meeting-transcript-1-semantic-v2/semantic-candidates/semantic-summary.json` shows the semantic layer produced 50 observations, 1 `action_candidate`, and 26 needs-review items.
- The private transcript semantic preview shows the lone transcript action candidate is low-confidence, needs-review, and stitched from unrelated evidence.
- `temp/wp13-notion-doc-1-semantic-v2/semantic-candidates/semantic-summary.json` shows the process document produced 53 capability observations but collapsed them into one medium-confidence `capability_candidate`.
- `temp/` proves WP11/WP12/WP13 behavior directly. It does not contain a real WP14 Product Brain proposal replay artifact, so WP15 must not pretend `temp/` validates proposal replay.

These artifacts are private evidence. They may guide the acceptance harness and ignored local review runs, but committed fixtures must stay synthetic and must not contain recognizable private labels or raw private content.

## Product Model Fit

Eligibility path: `EXTEND`.

This extends the canonical Mindline local dry-run/review pattern. It does not create a Product Brain-specific integration path. The product object is a destination-neutral `semantic-acceptance` artifact family that evaluates semantic candidates before any workspace policy or destination adapter applies them.

This is not bespoke because the same acceptance loop is needed for Markdown documents, transcripts, Slack captures, future source adapters, and future destinations. It turns "semantic candidates exist" into "semantic candidates can be judged, improved, and compared across runs."

## Recommended Next PR

Work package: `WP-15 Semantic candidate acceptance loop`.

Because `WP-15` already exists in Product Brain, delivery authority requires updating that existing entry from this signed V5 spec and plan, then running audit/gates. The existing entry is not evidence of prior delivery authority; it is the durable work package to reconcile.

The PR should add a local-only command and artifact bundle:

```text
mindline documents accept <semantic-run-dir> --answer-key <answer-key.json> --out <dir>
```

It should read `semantic-candidates/`, apply a deterministic capture-level answer key, and write:

```text
<out>/semantic-acceptance/
  acceptance-summary.json
  expected-outcomes/<expected_outcome_id>.json
  items/<candidate_id>.json
  previews/<candidate_id>.md
  reports/quality-report.md
```

The answer key must not be centered on generated `candidate_id` values. It must define expected semantic outcomes for the held-out capture/run independently of the candidate generator, then match generated candidates to expected outcomes by kind, source document, evidence ranges, title/summary signals, and relation evidence. Candidate-level adjudication is allowed as a secondary review layer, but the quality claim depends on expected outcomes.

## Answer-Key Contract

The held-out answer key must include:

- `answer_key_id`
- `source_document_id`
- `expected_outcomes[]`
- optional `expected_absent[]`
- optional candidate-specific override decisions for manual review cases

Each expected outcome must include:

- `expected_outcome_id`
- `expected_state`: `expected_present` or `expected_absent`
- `expected_kind`
- `required_evidence[]`
- `acceptable_evidence_alternates[]`
- `title_signals[]`
- `summary_signals[]`
- `relation_requirements[]`
- `minimum_confidence_floor`
- `notes`

Evaluation must report generated candidates that match expected outcomes, expected outcomes that were missed, generated candidates that should have been absent, and generated candidates that cannot be matched to any expected outcome.

## Acceptance States

Allowed acceptance states for the first slice:

- `accepted`
- `rejected`
- `needs_review`
- `needs_split`
- `needs_merge`
- `blocked`

Allowed decision reasons for the first slice:

- `correct`
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

## Scope

In scope:

- Define answer-key and acceptance artifact schemas.
- Add synthetic held-out semantic fixtures that are not the same examples used to tune `WP-13`.
- Add ignored local comparison/evidence support for `temp/`-style private transcript and process-document runs, without committing private source content or derived private outputs.
- Add deterministic acceptance evaluation from candidate artifacts plus capture-level expected outcomes.
- Report candidate count, accepted/rejected/needs-review counts, expected-present count, expected-absent count, matched expected count, missed expected count, unexpected candidate count, precision-like match rate, recall-like expected-outcome coverage, review burden, blocked count, duplicate count, and evidence-missing count.
- Preserve candidate IDs, source document IDs, evidence ranges, relation IDs, confidence, review status, and blockers in acceptance items.
- Add quality-report Markdown for human inspection.
- Add regression tests for deterministic output, stale candidate IDs, missing evidence, private marker blocking, duplicate answer rows, and schema validation.

Out of scope:

- No live Product Brain writes.
- No Product Brain proposal generation from semantic candidates.
- No claim that `temp/` validates WP14 proposal replay.
- No app-integration transport.
- No kernel capability probing.
- No Slack daemon or live source fetching.
- No LLM/provider integration.
- No UI.
- No claim of final calibrated classification quality beyond the measured deterministic fixtures in this slice.

## Outcome Proof

The PR is successful when Mindline can answer, from artifacts alone:

1. Which expected semantic outcomes were found and which were missed.
2. Which generated semantic candidates were accepted, rejected, blocked, unexpected, or sent back for review.
3. Why each candidate and expected outcome received that state.
4. Which evidence node/range supported or failed the candidate/outcome match.
5. Whether a repeated run produces the same acceptance report.
6. Whether held-out fixtures expose false positives, false negatives, and review burden instead of hiding them behind `ready`/`high` labels.
7. That the report uses "precision-like" and "recall-like" language and explicitly says this is not calibrated classifier quality yet.

## Roadmap

1. `WP-15`: semantic acceptance loop and evaluation report.
2. `WP-16`: semantic review queue generation from acceptance results, still destination-neutral.
3. `WP-17`: measured classifier improvement slice, potentially LLM-assisted, judged against the `WP-15` answer-key harness.
4. `WP-18`: destination policy mapping from accepted semantic candidates to local proposal intents, with Product Brain remaining one adapter.
5. `WP-19`: Product Brain app-integration apply-time client shape, only after PB kernel affordances and transport contract are ready.

## Gates

- Brainstorm Authority: PASS if reviewers accept that classification quality, not live integration, is the next true blocker.
- Product Model Fit: EXTEND.
- Impact Discovery: PASS for planning if upstream/downstream contracts and exclusions above are preserved.
- Spec Authority: BLOCKED until LOOP reviewers sign off on this exact V5 or a revised version and the signed result is captured on Chain.
- Delivery Authority: BLOCKED until existing `WP-15` is updated from the signed spec and audit/gates pass.
