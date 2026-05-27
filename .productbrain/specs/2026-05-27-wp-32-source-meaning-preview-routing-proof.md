# WP-32 Source Meaning Preview and Routing Proof

Status: signed target for the next PR after WP-31.

## Chain authority

- `STR-2`: Mindline is a headless, provider-agnostic semantic ingestion engine. Tolaria and Product Brain are downstream destinations, not the product.
- `STR-3`: autonomy-readiness before destination writes.
- `DEC-64`: no-human steady state requires held-out >=98% accuracy and safety guardrails; review is calibration, not the product.
- `WP-31` / `DEC-214`: read-only Slack corpus intake is merged.
- `INS-18`: Slack link-only intake mechanically works, but currently creates shallow `reference_candidate` output and duplicate-clustering pressure.
- `STD-18`: semantic previews must include inline evidence excerpts.
- `KEY-6`: destination writes, auto-accept/no-human claims, and committed private artifacts stay at zero before DEC-64 proof.

## Problem

WP-31 lets Slack captures become corpus-pressure input, but the result is still too technical to review. Randy cannot judge from the output alone what source was read, what Mindline thinks it means, what evidence supports that reading, what context is missing, and whether Tolaria or Product Brain would care.

The current corpus-pressure report proves machinery. It does not yet prove reviewer-legible source meaning.

## Outcome

Given an existing corpus-pressure output, Mindline emits a local source-meaning preview bundle that lets Randy review the product result without opening internal JSON:

- source snapshot and source identity;
- extracted meaning candidates / atoms;
- inline evidence excerpt or explicit missingness;
- duplicate / relation context;
- coarse non-applyable routing hints for Tolaria, Product Brain, both, no-op, needs enrichment, or blocked;
- summary counters that make shallow/reference-only output and duplicate pressure visible.

Every artifact must say this is preview/calibration only. It is not saved, accepted, destination-ready, or executable as a write.

## Command contract

```sh
mindline documents meaning-preview <corpus-pressure-out-or-parent> --out <dir>
```

The command reads existing corpus-pressure and corpus-graph artifacts. It does not rerun semantics, call an LLM, fetch links, call Slack, call Product Brain, write Tolaria, write Product Brain, use auth, use a DB, or export hosted telemetry.

## Artifact contract

Output directory:

```text
<out>/source-meaning-preview/
  meaning-summary.json
  meaning-report.md
  sources/<source-id>.md
```

`meaning-summary.json` includes:

- schema version;
- corpus id;
- source count and previewed source count;
- atom count;
- processed / skipped / blocked source counts;
- meaning bucket counts;
- missingness reason counts;
- routing hint counts;
- duplicate / relation counts;
- guardrails with `destination_writes`, `product_brain_writes`, and `tolaria_writes`, all zero;
- preview paths.

Per-source preview includes:

1. Review banner: preview/calibration only, no writes.
2. Source snapshot: source id, label, kind, state, candidate count, source path.
3. Extracted meaning: title, kind, confidence, review status, summary.
4. Evidence: inline excerpt, or explicit missingness reason.
5. Relation context: duplicate/same-topic/contradiction notes when present.
6. Routing hints: Tolaria / Product Brain / both / no-op / needs enrichment / blocked, with reason codes and `write_eligible: false`.
7. Guardrails: all write counters remain zero.

## Routing hint taxonomy

Routing hints are diagnostics, not destination payloads.

- `tolaria_candidate`: useful personal PKM source/signal/task/resource candidate.
- `product_brain_candidate`: likely reusable system/product governance candidate such as decision, tension, standard, insight, work package, or architecture note.
- `both_candidate`: may matter to both PKM and Product Brain.
- `no_op`: not useful enough for either destination.
- `needs_enrichment`: a link-only or under-contextualized source needs more context before semantic trust.
- `blocked`: unsafe, private, malformed, or otherwise not eligible for routing.

Reason codes must explain the hint, such as `reference_only`, `link_only_source`, `missing_link_enrichment`, `decision_candidate`, `action_candidate`, `duplicate_context`, `blocked_source`, or `no_semantic_candidates`.

## Requirements

1. 100% of processed corpus-pressure sources produce a per-source meaning preview.
2. 100% of previewed sources include inline evidence excerpts or explicit missingness.
3. 100% of eligible atoms receive non-applyable routing hints with reason codes and `write_eligible: false`.
4. Link-only / shallow outputs are counted and labeled `needs_enrichment` or `reference_only`; they cannot masquerade as strong semantic understanding.
5. Duplicate / same-topic / contradiction context is visible when corpus graph relations exist.
6. The command rejects protected destination roots before reading input or writing artifacts.
7. Guardrails report zero destination writes, Product Brain writes, Tolaria writes, hosted telemetry exports, and hosted inference calls.
8. Committed fixtures remain synthetic. Private runtime proof may use `/private/tmp` only.

## Exclusions

- No Tolaria writes.
- No Product Brain writes.
- No destination apply payloads.
- No Product Brain profile lookup or schema mapping.
- No Tolaria Markdown note rendering.
- No auth/login/DB.
- No hosted telemetry export.
- No LLM calls or provider tuning.
- No live link fetching or browser enrichment.
- No permanent UI.
- No no-human, auto-accept, production-ready, or DEC-64 eligibility claim.

## Verification

- Focused unit tests for summary building, routing hints, missingness, duplicate context, and protected-root rejection.
- CLI test that reads a synthetic corpus-pressure output and writes source-meaning-preview artifacts.
- `go test ./...`
- `git diff --check`
- Private runtime smoke after implementation:
  - Slack corpus-intake on private data under `/private/tmp`;
  - corpus-pressure over the manifest;
  - meaning-preview over corpus-pressure output;
  - manual/Randy-proxy review of at least 10 previews from Markdown alone;
  - leak scan proving no private Slack identifiers/content entered committed files.

## Review standard

Before asking Randy to review, the implementer must review as Randy first:

- Can I understand the original source from the preview?
- Can I see what the system extracted?
- Can I see the evidence or missingness?
- Can I see why Tolaria or Product Brain would or would not care?
- Can I judge without opening raw JSON?

If the answer is no, iterate before requesting human review.
