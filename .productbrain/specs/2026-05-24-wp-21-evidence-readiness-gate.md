# WP-21 Evidence Completeness / Readiness Gate

Artifact: MINDLINE-WP21-SPEC-V2
Status: Draft for LOOP sign-off, V2
Work package: WP-21
Date: 2026-05-24

## Chain Authority

- WP-21: Evidence completeness/readiness gate.
- INI-1: Semantic autonomy readiness cycle.
- STR-2 / STR-3: autonomous semantic ingestion vision and autonomy-readiness before destination writes.
- KEY-7: 100% eval-counted candidates pass evidence readiness.
- KEY-6: zero destination writes, Mindline-generated PB/Tolaria destination writes, auto-accept/no-human claims, or committed private review artifacts before proof.
- DEC-64: no human in steady state only after held-out >=98% accuracy.
- STD-17: LLM-backed semantic classifiers must be provider-agnostic and measured before trust.
- STD-18: LLM semantic previews must include inline evidence excerpts.
- ARCH-1: model providers are interchangeable behind the Mindline LLM provider port.

## Diagnosis

WP-18 and WP-20 made semantic candidates reviewable by adding one-item judgment pages, source excerpts, and relation context. That still leaves a measurement gap: the judgment bundle reports candidate and review counts without saying whether each candidate had enough evidence to be eligible for autonomy metrics.

The current code can emit a judgment item with:

- title and summary,
- evidence node ids and evidence ranges,
- optional source excerpts,
- relation ids and relation context,
- blockers and review status.

But it does not compute a fail-closed readiness decision. A candidate with `source excerpts unavailable`, missing relation context, unrelated relation endpoints, missing evidence ranges, or blockers can still be present in the same evaluation surface as candidates that are actually inspectable. That weakens KEY-7 and makes WP-23 vulnerable to counting candidates whose source support cannot be audited.

## Product Decision

WP-21 adds a destination-neutral evidence readiness gate to the semantic judgment/evaluation surface. The gate answers one question per candidate:

```text
Can this candidate be counted in autonomy-readiness evaluation metrics?
```

The answer must be machine-readable and fail closed. It does not judge semantic correctness. It only decides whether the candidate has enough source-grounded evidence, relation context, schema validity, and safety to be counted by later reports.

## In Scope

- Add evidence readiness fields to semantic judgment artifacts.
- Mark each candidate as eval-counted or excluded.
- Record one or more stable evidence-readiness reason codes for excluded candidates.
- Add summary counts for ready, eval-counted, excluded, and reason-code totals.
- Surface readiness in judgment JSON, Markdown report, one-item page Markdown, and the local review UI.
- Exclude candidates with unavailable source excerpts from eval-counted metrics.
- Exclude candidates with missing or incomplete relation context from eval-counted metrics.
- Exclude blocked/skipped/private/governance unsafe candidates from eval-counted metrics.
- Preserve provider neutrality and no-destination-write/no-auto-claim guardrails.
- Verify behavior against all direct Markdown files in `temp/` without committing private generated output.

## Out of Scope

- No Mindline-generated destination writes to Product Brain or Tolaria.
- No Product Brain apply transport.
- No auto-accept or no-human claim.
- No model/prompt/provider tuning.
- No WP-22 unified semantic failure taxonomy.
- No WP-23 held-out autonomy-readiness report.
- No permanent review-product UX expansion.
- No committed private `temp/` source material or generated private judgment artifacts.

## Readiness Contract

Add a readiness object to every `SemanticJudgmentCandidate`:

```json
{
  "evidence_readiness": {
    "status": "pass|fail",
    "eval_counted": true,
    "reason_codes": [],
    "source_excerpt_count": 1,
    "relation_context_count": 1
  }
}
```

Readiness-required artifacts must use new schema versions:

- `semantic-judgment-summary/v0.2`
- `semantic-judgment-candidate/v0.2`
- `semantic-judgment-page/v0.2`

Legacy `v0.1` judgment artifacts without readiness may still be read where existing consumers allow them, but they are not eval-counted. If a command has to load a legacy bundle for review, absent readiness is treated as a legacy fail-closed state with `eval_counted=false`, not as an implicit pass.

Candidate summary entries must carry the same readiness status, `eval_counted`, and reason codes so consumers can evaluate a bundle without opening every candidate file.

The judgment summary must include:

- `evidence_ready_count`
- `eval_counted_count`
- `evidence_excluded_count`
- `evidence_readiness_reason_counts`

### Pass Conditions

A candidate passes evidence readiness only when all are true:

1. Candidate schema already validated successfully.
2. Candidate review status is not `blocked` or `skipped`.
3. Candidate has non-empty title and summary.
4. Candidate has at least one evidence node and at least one evidence range.
5. Candidate has at least one non-unavailable source excerpt generated from the supplied source Markdown context.
6. Candidate has no candidate blockers.
7. Candidate has no private/governance marker in candidate body, source excerpts, relation context, or blockers.
8. Candidate has at least one relation id.
9. Candidate has at least one loaded relation context.
10. Every relation id listed on the candidate has loaded relation context.
11. Every loaded relation context references the current candidate and has an available other endpoint context.

### Fail Reason Codes

The first slice uses a readiness-specific enum, separate from WP-22's future semantic failure taxonomy:

- `blocked_or_skipped`
- `missing_candidate_content`
- `missing_evidence_nodes`
- `missing_evidence_ranges`
- `missing_source_excerpt`
- `missing_relation_context`
- `invalid_relation_context`
- `candidate_blockers`
- `private_or_governance_marker`

Reason codes are additive. A candidate may fail for more than one reason.

### Counting Rule

`eval_counted=true` only when readiness status is `pass`.

Summary-level judgment counts may remain raw review counts for backward compatibility, but all new readiness summary fields must make the eligible population explicit. WP-23 must use `eval_counted_count`, not total candidate count, when it computes autonomy-readiness metrics.

## CLI / Artifact Surface

`mindline documents judge` remains the command that creates the judgment bundle. No new command is required for WP-21.

Generated `semantic-judgment/` artifacts must include readiness fields in:

- `judgment-summary.json`
- `candidates/*.json`
- `pages/*.md`
- `reports/judgment-report.md`

`mindline documents judge-next` must return a page object whose `item` includes readiness fields and whose `page_markdown` names:

- readiness status,
- whether the item is eval-counted,
- readiness reason codes when excluded.

`mindline documents judge-serve` must show readiness state in the local UI so a reviewer understands whether the item can affect metrics.

## Safety / Privacy

Readiness must never loosen existing safety behavior. If existing validation rejects private/governance markers, WP-21 keeps that rejection. If unsafe content appears in a constructed readiness surface, the candidate fails readiness and the writer must still fail closed rather than write unsafe artifacts.

Source excerpts remain bounded by the WP-16/WP-18 excerpt limits and may only come from explicit `--source-root` plus `--source` context. Missing source context is not an error for creating a review bundle, but it is an evidence-readiness failure.

Product Brain governance writeback for LOOP proof is separate from Mindline destination writes. Governance closeout may update WP-21 and capture a delivery decision only when it contains aggregate proof, command evidence, PR links, and reviewer verdicts; it must not contain private `temp/` source excerpts or generated private judgment artifacts. This governance writeback does not authorize Product Brain as a Mindline destination and does not satisfy DEC-64 autonomy proof.

## Acceptance Criteria

1. Judgment candidates include `evidence_readiness` with pass/fail, eval-counted, reason codes, source excerpt count, and relation context count.
2. Judgment summary includes evidence-ready, eval-counted, excluded, and readiness reason-code counts.
3. Readiness-required artifacts use `v0.2` schema versions; legacy `v0.1` artifacts without readiness are never eval-counted.
4. Candidates with unavailable source excerpts are excluded from eval-counted metrics.
5. Candidates with zero relation ids, missing relation context, or unrelated/unavailable relation endpoints are excluded from eval-counted metrics.
6. Blocked/skipped candidates and candidates with blockers are excluded from eval-counted metrics.
7. Private/governance markers in candidate body, source excerpts, relation context, or blockers fail readiness and cannot be written as valid readiness artifacts.
8. A fully grounded candidate with source excerpt and loaded relation context passes readiness and is eval-counted.
9. Judgment report, `judge-next` page Markdown, `judge-next` item JSON, and review UI surface readiness status and reason codes.
10. Full Go tests pass.
11. Real `temp/*.md` verification runs `documents semantics` and `documents judge` with source context for every direct Markdown file. Each file passes only if it has zero candidates, at least one eval-counted candidate, or all candidates are explicitly excluded with non-empty readiness reason counts.
12. No Mindline Product Brain/Tolaria destination writes, auto-accept behavior, no-human claims, committed private temp artifacts, or provider lock-in are introduced.

## Audit Notes

`pb audit WP-21` currently passes shaping gates with warnings for unfilled risks and no linked tensions. The implementation risks are captured here and may be mirrored to the WP risk field during governance closeout only as aggregate, private-content-free Product Brain proof:

- counting ineligible candidates would corrupt autonomy metrics;
- missing source context could be mistaken for inspected evidence;
- readiness reason codes could drift into WP-22 taxonomy before that work is shaped;
- private temp source excerpts must stay uncommitted and bounded.
