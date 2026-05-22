# WP-16 Autonomy Calibration From Acceptance Batches

Artifact: MINDLINE-WP16-SPEC-V3
Status: Draft for LOOP sign-off
Work package: WP-16
Date: 2026-05-22

## Product Decision

Mindline's destination-neutral semantic layer must move toward autonomous ingestion, not a permanent human review queue. Human adjudication is allowed only as a temporary calibration and trust-building mechanism. A batch may be promoted toward no-human steady state only when held-out acceptance evidence proves at least 98% measured accuracy and no private-marker or containment regression exists.

## Chain Authority

- WP-16: Autonomy calibration from acceptance batches.
- DEC-64: Mindline semantic automation target is no human in steady state.
- DEC-65: WP-16 adjudication uses one-item CLI pagination.
- WP-15: Semantic candidate acceptance loop is the upstream dependency.
- STD-11: Product Brain SSOT must stay current.
- STD-12: Private provenance visibility is authoritative for publish blocking.
- STD-16: Proposal adapters must fail closed and remain bundle-contained.

## Problem

WP-15 made semantic candidate quality measurable, but the next step can easily become a human review product. That would contradict the target behavior: Mindline should eventually ingest without Randy or another human clearing routine items. WP-16 must convert acceptance results into calibration evidence, failure classes, and trust thresholds so human intervention shrinks over time rather than becoming the workflow.

## In Scope

- A calibration artifact set derived from WP-15 `semantic-acceptance` outputs.
- A batch-level calibration summary with measured accuracy, false-positive count, false-negative count, needs-review count, review-burden rate, threshold status, and no-human eligibility.
- A machine-readable failure taxonomy covering false positives, false negatives, missing evidence, relation errors, source-scope errors, blocked/private cases, duplicates, and needs-review ambiguity.
- A temporary adjudication queue represented as calibration evidence, not destination policy or live writes.
- CLI pagination that returns exactly one adjudication item per command response, persists cursor/progress, and resumes until the batch is exhausted.
- Tests proving below-threshold batches fail closed and above-threshold held-out fixtures can pass without human review.
- Local verification against every Markdown corpus file under `temp/`: meeting transcripts, Notion/process documents, mixed Markdown, and prior before/after review Markdown artifacts.

## Out of Scope

- Product Brain live writes, Tolaria writes, or destination policy mapping.
- Claiming real semantic understanding quality without externally supplied held-out answer keys.
- Training or replacing the semantic classifier.
- Persisting private real `temp/` source material or derived answer keys into the repository.
- Treating human review as a steady-state product workflow.

## Contract

### Calibration Inputs

The first implementation consumes a WP-15 `semantic-acceptance` directory containing:

- `acceptance-summary.json`
- per-candidate acceptance item JSON files
- expected outcome JSON files

The command may accept either the `semantic-acceptance` directory itself or its parent output directory, but it must not traverse outside that bundle.

Input containment applies to the directory and every artifact loaded from it: `acceptance-summary.json`, expected-outcome files, acceptance item files, cursor files, and path fields inside those artifacts. Symlinked input artifacts, symlinked ancestor directories under the supplied bundle, absolute paths, `..` traversal, and lexical paths that resolve outside the supplied bundle must be rejected.

### Calibration Outputs

The command writes `semantic-calibration/` under `--out` with:

- `calibration-summary.json`
- `review-items/*.json`
- `cursor.json`
- `reports/calibration-report.md`

Outputs must use stable schema versions, deterministic ordering, bundle-contained paths, and the same private/governance-marker guardrails as WP-15.

### Accuracy Rule

Measured accuracy is:

```text
accepted_count / scored_count
```

where `scored_count` is the sum of exactly-one primary WP-16 class counts for every non-blocked scored item:

```text
accepted + false_positive + false_negative + missing_evidence + relation_error + source_scope_error + duplicate + needs_review_ambiguity + other
```

`blocked_private` is excluded from the numerator and denominator because it is a safety gate, not a classifier-quality score. Any `blocked_private` item prevents no-human eligibility even when the numeric threshold is met.

Threshold status:

- `trusted` only when measured accuracy is `>= 0.98`, held-out evidence is marked true, blocked/private count is zero, and containment/private-marker validation passes.
- `not_trusted` otherwise.

The report must state whether the evidence is held-out. Generated or self-derived answer keys may exercise the pipeline but cannot prove real semantic accuracy.

### Failure Classes

Each review item must carry one primary failure class:

- `accepted`
- `false_positive`
- `false_negative`
- `missing_evidence`
- `relation_error`
- `source_scope_error`
- `blocked_private`
- `duplicate`
- `needs_review_ambiguity`
- `other`

The summary must include counts for every class, including zero counts.

The primary class is assigned by this deterministic precedence table:

| WP-15 evidence | WP-16 primary class |
|---|---|
| Candidate item `acceptance_state=blocked` or `reason=unsafe_or_private` | `blocked_private` |
| Expected outcome result `reason=missing_expected_outcome` with no matched candidate | `false_negative` |
| Candidate item `acceptance_state=accepted` and `reason=correct` | `accepted` |
| Candidate item `reason=duplicate` | `duplicate` |
| Candidate item `reason=missing_evidence` or `reason=unsupported_evidence` | `missing_evidence` |
| Candidate item `reason=unexpected_candidate` | `false_positive` |
| Candidate item `acceptance_state=needs_review`, `needs_split`, or `needs_merge`, or `reason=ambiguous`, `too_broad`, `too_narrow`, or `stale_or_contradicted` | `needs_review_ambiguity` |
| Candidate or expected outcome carries explicit future mismatch metadata `mismatch_type=relation` | `relation_error` |
| Candidate or expected outcome carries explicit future mismatch metadata `mismatch_type=source_scope` | `source_scope_error` |
| Anything else | `other` |

`relation_error` and `source_scope_error` must be present in the taxonomy and summary counts now, but WP-16 must report zero for them unless future WP-15 acceptance artifacts carry explicit mismatch metadata. WP-16 must not infer those classes from weak post-hoc signals.

### CLI

Add:

```text
mindline documents calibrate <semantic-acceptance-dir-or-parent> --out <dir> [--threshold 0.98] [--held-out]
mindline documents calibrate-next <semantic-calibration-dir-or-parent>
```

`calibrate` writes the calibration artifact set and returns the batch summary as JSON.

`calibrate-next` returns exactly one page object as JSON:

- When an item remains: `done=false`, one `item`, and updated cursor/progress.
- When exhausted: `done=true`, no item, and final cursor/progress.

It must persist cursor progress after each successful item page so interruption and resume are deterministic.

### Temp Corpus Verification

The local verification run must process every Markdown file directly under `temp/`:

- `meeting-transcript-1.md`
- `meeting-transcript-2.md`
- `mixed-doc.md`
- `notion-doc-1.md`
- `notion-doc-2.md`
- `notion-doc-3.md`
- `wp11-notion-doc-1-before-after.md`
- `wp12-meeting-transcript-1-before-after.md`

Non-corpus files such as `.DS_Store`, scripts, and previously generated JSON outputs are excluded from corpus pass/fail. Verification may write real-corpus outputs only to `/private/tmp` or another non-repository scratch path.

## Acceptance Criteria

1. Given WP-15 acceptance output, `documents calibrate` emits a batch-level calibration summary with accuracy, FP/FN counts, review-burden rate, threshold status, held-out status, and no-human eligibility.
2. The calibration report explicitly states that human adjudication is temporary calibration evidence, not the steady-state workflow.
3. No batch is marked `trusted` unless held-out evidence is true, measured accuracy is at least 0.98, blocked/private count is zero, and containment/private-marker validation passes.
4. Failure classes are machine-readable and every review item has exactly one primary class.
5. `documents calibrate-next` returns one and only one item per non-exhausted response, persists cursor/progress, and resumes until exhausted.
6. Tests prove below-threshold batches fail closed and above-threshold held-out fixtures can pass without requiring human review.
7. Tests prove private/governance markers and symlinked output parents are rejected.
8. Tests prove malicious or symlinked calibration input artifacts are rejected before reading outside the supplied `semantic-acceptance` or `semantic-calibration` bundle.
9. Real temp Markdown corpus verification completes without crash, without repository writes, and without claiming semantic trust unless held-out answer-key evidence exists.

## Evidence Expectations

- Unit tests for calibration math, failure taxonomy, trust gating, private-marker rejection, deterministic output, and pagination cursor behavior.
- CLI tests for `documents calibrate` and `documents calibrate-next`.
- Full `go test ./...`.
- Local real-corpus verification against every Markdown file listed above, with aggregate artifact counts and trust-gate status reported without quoting private content.

## LOOP Sign-Off Panel

- Chain Steward
- Domain/User Job Reviewer
- Systems Architect
- Delivery Quality Reviewer
- Risk/Safety Reviewer
