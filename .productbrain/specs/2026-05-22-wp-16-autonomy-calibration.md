# WP-16 Autonomy Calibration From Acceptance Batches

Artifact: MINDLINE-WP16A-SPEC-V7
Status: PR #11 remediation draft for LOOP sign-off
Work package: WP-16
Date: 2026-05-22

## WP-16A Review Remediation

PR #11 did not satisfy the human-review job. It implemented one-item pagination mechanically, but the item page did not contain enough real content for a reviewer to understand the candidate, compare it to the expected outcome, inspect evidence, or choose an adjudication. The remediation is part of WP-16 and must land on the existing PR, not a new PR.

The standard is now higher: a non-exhausted `calibrate-next` response must be a self-contained review page. A human reviewer should not need to manually open candidate JSON, acceptance JSON, structure artifacts, or source files just to understand the item being reviewed.

## Escalation Loop Ledger

### Loop 1 - Minimum Comprehension

Diagnosis: the current calibration preview is generated from `SemanticCalibrationReviewItemSummary`, so it drops title, summary, confidence, evidence ranges, blockers, and expected-outcome context already present in the full item JSON.

Spec raise: every one-item page must include candidate title, kind, confidence, acceptance state, failure class, reason, expected outcome id when present, blocker messages, and evidence line references.

Plan raise: tests must fail if the preview/page contains only metadata and no candidate content.

### Loop 2 - Source Grounding

Diagnosis: candidate summaries can be stitched or low-quality, especially for transcripts. A human cannot judge semantic quality from a weak generated summary alone. Loop 1 reviewer blockers also showed that source excerpt handling is unsafe without explicit containment and hard excerpt caps.

Spec raise: calibration may accept source Markdown context only through an explicit `--source-root <dir>` plus `--source <relative-markdown-file>` pair. When available, it must include bounded source excerpts for evidence ranges in the review page. Missing source context must be explicit, not silent. Excerpts are capped at 3 ranges, 6 lines per range, 1,200 characters per range, and 4,000 characters total per item.

Plan raise: real temp verification must prove transcript and Notion examples include source excerpts or an explicit source-context-missing notice. Tests must cover symlinked source, traversal, non-Markdown source, oversized range clamping, and private/governance marker rejection from source excerpts.

### Loop 3 - Adjudication-Ready Page

Diagnosis: even content-rich evidence is incomplete if the user is not shown the actual decision they are being asked to make. Loop 1 reviewer blockers also showed that false-negative and expected-outcome comparison pages could remain metadata-only. Loop 2 reviewers showed that schema evolution and Chain authority must be explicit before implementation.

Spec raise: each page must use a new page schema, include expected-outcome context, and include an adjudication prompt with the allowed decisions and what each means for calibration: accept, reject false positive, mark missing evidence, mark ambiguous, mark wrong kind/scope, or block private/governance.

Plan raise: CLI JSON must carry a renderable Markdown page and structured `review_context` under `semantic-calibration-page/v0.2` so terminal users and future UI clients consume the same self-contained item. False-negative tests must fail unless the page explains that no candidate matched and shows expected-outcome details. Newly generated WP-16A bundles must include real expected-outcome details. Legacy `v0.1` calibration bundles may be read, but missing fields must render as explicit `unavailable` notices and mark the page as legacy/not fully adjudication-ready.

## Product Decision

Mindline's destination-neutral semantic layer must move toward autonomous ingestion, not a permanent human review queue. Human adjudication is allowed only as a temporary calibration and trust-building mechanism. A batch may be promoted toward no-human steady state only when held-out acceptance evidence proves at least 98% measured accuracy and no private-marker or containment regression exists.

## Chain Authority

- WP-16: Autonomy calibration from acceptance batches.
- DEC-64: Mindline semantic automation target is no human in steady state.
- DEC-65: WP-16 adjudication uses one-item CLI pagination.
- TEN-6: WP-16 PR #11 pagination is mechanically correct but not reviewable.
- DEC-69: WP-16A V5 raises PR #11 remediation standard for review pages.
- DEC-70: WP-16A V6 authorizes PR #11 remediation after three spec-plan loops.
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
- Self-contained one-item review pages that include real candidate content, source/evidence context, and explicit adjudication choices.
- Optional source Markdown context for calibration review pages, with bounded evidence excerpts derived from evidence line ranges.
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

### Human Review Page Contract

For every non-exhausted page, `calibrate-next` must return a self-contained review page using `semantic-calibration-page/v0.2`:

- `item`: the structured calibration review item.
- `review_context`: structured context for UI clients.
- `page_markdown`: terminal-ready Markdown for the exact item.

`review_context` and `page_markdown` must include:

- source document id and source file path when supplied
- candidate id, expected outcome id, kind, title, confidence, review status
- generated summary or explicit `no summary` notice
- acceptance state, failure class, reason, and blocker messages
- evidence ranges with line numbers
- bounded source excerpts for evidence ranges when source Markdown is supplied
- explicit adjudication choices and their calibration meaning

When source Markdown is not supplied, the page must say `source excerpts unavailable` and still show candidate summary plus evidence line references. It must never silently imply evidence was inspected.

### Schema Evolution

WP-16A must not silently mutate the meaning of existing `v0.1` schemas.

- `SemanticCalibrationPageSchemaVersion` becomes `semantic-calibration-page/v0.2` because `review_context` and `page_markdown` are required response fields.
- `SemanticCalibrationReviewItemSchemaVersion` becomes `semantic-calibration-review-item/v0.2` if expected-outcome/source excerpt context is persisted inside review-item JSON.
- Existing `semantic-calibration-summary/v0.1` may remain unchanged unless summary fields change.
- `calibrate-next` may read existing `semantic-calibration-review-item/v0.1` artifacts, but the returned page must be `semantic-calibration-page/v0.2` with missing context rendered as `unavailable`.
- `semantic-acceptance-expected-outcome/v0.2` may be introduced to persist the answer-key snapshot fields needed by WP-16A review pages.
- New WP-16A calibration generated from new acceptance outputs must use rich expected-outcome context; `unavailable` is allowed only for legacy `v0.1` acceptance/review artifacts.

`calibrate` accepts optional source context flags:

```text
--source-root <dir> --source <relative-markdown-file>
```

The source file is local-only review context. Its content may be copied into calibration artifacts only as bounded excerpts tied to existing evidence ranges and only after private/governance-marker validation.

Source containment contract:

- `--source` is valid only when `--source-root` is also supplied.
- `--source-root` may be absolute or relative, but must resolve to a non-symlinked directory.
- `--source` must be a relative `.md` path under `--source-root`; absolute paths and `..` traversal are rejected.
- the resolved source file and every ancestor between root and file must be non-symlinked.
- directories, non-Markdown files, unreadable files, and files containing private/governance markers in selected excerpts are rejected.
- calibration artifacts store only the caller-supplied relative source label, never the absolute source path.
- WP-16A must not discover arbitrary nearby files or infer source context from the acceptance bundle.

Excerpt caps:

- at most 3 evidence ranges are rendered per review item.
- at most 6 source lines are rendered per range.
- at most 1,200 characters are rendered per range after line clamping.
- at most 4,000 source-excerpt characters are rendered per review item.
- ranges outside the source file are clamped to the file bounds and marked `clamped=true`.
- if all excerpts are unavailable after validation, the page must show `source excerpts unavailable`.

### Expected Outcome Context

Every review page must include an expected-outcome section.

For matched candidate pages, include:

- expected outcome id
- expected state
- expected kind
- matched candidate id
- required evidence ids
- acceptable evidence alternates
- title signals
- summary signals
- relation requirements
- minimum confidence floor
- notes

For false-negative pages, include the same expected-outcome fields plus an explicit `No candidate matched this expected outcome.` statement. If a field is absent from the WP-15 artifact, render `unavailable` rather than omitting the section.

New WP-16A-generated calibration pages must include real expected-outcome details, not only `unavailable` placeholders. To support that, WP-16A may extend WP-15 acceptance expected-outcome result artifacts to persist an answer-key snapshot:

- required evidence ids
- acceptable evidence alternates
- title signals
- summary signals
- relation requirements
- minimum confidence floor
- notes

If older `v0.1` artifacts lack those fields, the review page must show explicit unavailable notices, mark `legacy_context=true`, and state `Legacy calibration input lacks full expected-outcome context; this page is not fully adjudication-ready.`

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
5a. Each non-exhausted `calibrate-next` response is understandable without opening another artifact: it includes candidate content, calibration result, evidence line references, source excerpts when supplied, and adjudication choices.
5b. Matched and missed expected-outcome pages generated by WP-16A include real expected-outcome details; false-negative pages explicitly say no candidate matched.
5b-legacy. Legacy `v0.1` inputs with missing expected-outcome details render explicit unavailable notices and mark the page legacy/not fully adjudication-ready.
5c. Source excerpts are rendered only from `--source-root` contained Markdown files and obey the range, line, and character caps.
5d. `calibrate-next` returns `semantic-calibration-page/v0.2`; legacy `v0.1` review-item inputs render with explicit unavailable notices rather than silent omission.
6. Tests prove below-threshold batches fail closed and above-threshold held-out fixtures can pass without requiring human review.
7. Tests prove private/governance markers and symlinked output parents are rejected.
8. Tests prove malicious or symlinked calibration input artifacts and source-context inputs are rejected before reading outside the supplied `semantic-acceptance`, `semantic-calibration`, or `--source-root` bundle.
9. Real temp Markdown corpus verification completes without crash, without repository writes, and without claiming semantic trust unless held-out answer-key evidence exists.

## Evidence Expectations

- Unit tests for calibration math, failure taxonomy, trust gating, private-marker rejection, deterministic output, and pagination cursor behavior.
- CLI tests for `documents calibrate` and `documents calibrate-next`.
- CLI tests proving `calibrate-next` emits content-rich `page_markdown` and `review_context`, not a metadata-only page.
- Tests for matched and missed expected-outcome review pages.
- Tests for source root containment, symlinked source rejection, traversal rejection, non-Markdown source rejection, excerpt clamping, and source-derived private/governance marker rejection.
- Tests asserting schema version behavior: new `v0.2` page output, persisted expected-outcome answer-key context for new outputs, and readable legacy `v0.1` items with unavailable notices plus legacy/not-fully-adjudication-ready marker.
- Full `go test ./...`.
- Local real-corpus verification against every Markdown file listed above, with aggregate artifact counts, trust-gate status, and one safe excerpt-presence/comprehension proof per generated review item without committing private content.

## LOOP Sign-Off Panel

- Chain Steward
- Domain/User Job Reviewer
- Systems Architect
- Delivery Quality Reviewer
- Risk/Safety Reviewer
