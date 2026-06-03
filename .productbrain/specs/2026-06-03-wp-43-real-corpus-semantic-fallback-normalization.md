# WP-43 Real-Corpus Semantic Fallback Normalization

Date: 2026-06-03
Status: signed for implementation
Product: Mindline
Layer: document artifact finalization

## Outcome

Messy real Markdown corpus-pressure runs must end in reviewable and fully accounted local artifacts/reports, not invalid fallback states or source-level blocked dead ends.

This work fixes observed deterministic fallback failures from the real `temp/pb-docs` corpus. The fix is source-neutral and destination-neutral: any source that enters the document artifact layer should obey the same readiness contract.

## Baseline Evidence

Baseline command:

```bash
go run ./cmd/mindline documents corpus-pressure temp/pb-docs --out /private/tmp/mindline-wp43-pb-docs-pressure
```

Baseline readback:

```bash
go run ./cmd/mindline eval readback /private/tmp/mindline-wp43-pb-docs-pressure --out /private/tmp/mindline-wp43-pb-docs-readback
```

Baseline result:

- `source_count`: 471
- `eligible_source_count`: 451
- `processed_source_count`: 441
- `excluded_source_count`: 20
- `blocked_source_count`: 10
- `processed_source_ratio`: 0.9778270509977827
- `evidence_ready_atom_ratio`: 0.9956043956043956
- `review_burden_ratio`: 0.004776772609612254
- `ready_for_50_file_pressure`: false
- side-effect guardrails: all zero
- readback sample boundary: `private_runtime`, `non_generalizable`

Observed blocked failure classes:

- unknown segments emitted in invalid ready states;
- unknown structure nodes emitted in invalid ready states;
- ready segments missing title or summary;
- all-structure-blocked sources excluded from the eligible denominator.

## Scope

Implement only in the document artifact finalization path:

- `finalizeSegments`
- `finalizeStructureNodes`
- constructors only if required to keep IDs/evidence stable

Do not relax `ValidateSegment` or `ValidateStructureNode`.

Do not special-case `temp/pb-docs`, Product Brain documents, Slack, Gmail, or any source path.

Do not change corpus-pressure accounting except as a downstream result of valid artifact states.

## Validity Contract

Invalid artifact states:

- unknown semantic type or unknown structure node with `review_status=ready`;
- low-confidence artifact with `review_status=ready`;
- ready artifact missing title;
- ready artifact missing summary;
- ready artifact missing required provenance/evidence.

Required fallback behavior:

- unknown or incomplete fallback artifacts become `needs_review`;
- confidence becomes `low`;
- artifact has an explicit blocker code and reason text;
- existing provenance/evidence is preserved;
- summary and review-burden counts include the artifact;
- unsafe marker classification keeps precedence and remains blocked/redacted;
- strict validation errors remain strict when an artifact cannot be made schema-valid without inventing evidence.

Exclusion behavior:

- exclusions must remain closed, counted, and inspectable;
- `unexplained_exclusion_count` must remain `0`;
- exclusions must not hide formerly invalid eligible sources or improve metrics by denominator laundering.

## KRs

After delivery:

1. On the same private `temp/pb-docs` deterministic corpus-pressure run, `blocked_source_count` moves from `10` to `0`.
2. Synthetic fixtures and output scans show `ready_unknown_segment_count == 0`.
3. Synthetic fixtures and output scans show `ready_unknown_structure_node_count == 0`.
4. Synthetic fixtures and output scans show `ready_empty_title_or_summary_count == 0`.
5. Fallback items are valid `needs_review`, `low` confidence, with blocker/reason text and preserved evidence/provenance when present.
6. Side-effect guardrails remain zero for:
   - `network_fetches`
   - `hosted_inference_calls`
   - `hosted_telemetry_exports`
   - `browser_calls`
   - `slack_api_calls`
   - `destination_writes`
   - `product_brain_writes`
   - `tolaria_writes`
   - `auto_accepts`
   - `no_human_claims`
   - `committed_private_artifacts`
7. Private rerun `evidence_ready_atom_ratio` does not regress below baseline `0.9956043956043956`.
8. Readback compares current private runtime proof against the baseline and still marks broad claims as non-generalizable.

## Required Tests

Write failing tests before implementation for:

- unknown-ready segment finalization;
- low-confidence-ready segment finalization;
- ready segment missing title;
- ready segment missing summary;
- unsafe blocked precedence for segments;
- unknown-ready structure node finalization;
- low-confidence-ready structure node finalization;
- ready structure node missing title;
- ready structure node missing summary;
- unsafe blocked precedence for structure nodes;
- writer summaries counting finalized `needs_review` artifacts;
- synthetic corpus-pressure regression proving fallback classes no longer become source-level semantic write blockers while review burden remains visible.

## Proof Plan

After implementation:

```bash
go test ./internal/documents
go test ./...
git diff --check
go run ./cmd/mindline documents corpus-pressure temp/pb-docs --out /private/tmp/mindline-wp43-pb-docs-current
go run ./cmd/mindline eval readback /private/tmp/mindline-wp43-pb-docs-current --out /private/tmp/mindline-wp43-pb-docs-current-readback --baseline /private/tmp/mindline-wp43-pb-docs-readback
go run ./cmd/mindline eval proof-gate /private/tmp/mindline-wp43-pb-docs-current-readback --out /private/tmp/mindline-wp43-pb-docs-current-safety-gate --claim safety
go run ./cmd/mindline eval proof-gate /private/tmp/mindline-wp43-pb-docs-current-readback --out /private/tmp/mindline-wp43-pb-docs-current-improvement-gate --claim improvement --baseline /private/tmp/mindline-wp43-pb-docs-readback
```

If a proof-gate command rejects the artifact shape, capture that result as a proof limitation and use readback plus metrics for the supported claim boundary.

Before PR:

- verify no private corpus body, Slack/Gmail body, runtime output, readback excerpt, or `/private/tmp` artifact is committed;
- inspect `git status`, staged diff, and artifact paths;
- PB/PR summaries may report only counts, states, reason codes, commands, and proof boundaries.

## Exclusions

No Slack connector.
No Gmail connector.
No live source access.
No auth/API client.
No browser fetching.
No destination writes.
No Product Brain/Tolaria writes from Mindline runtime.
No LLM/provider tuning.
No committed private content.
No autonomous acceptance.
No held-out/generalization claim.
No DEC-64/no-human claim.
No destination-readiness claim.

## LOOP Sign-Off

Spec and plan v3 signed off by:

- Chain/Product authority reviewer
- Domain/User Job reviewer
- Systems Architect reviewer
- Delivery Quality reviewer
- Risk/Safety reviewer from v2 safety scope

Implementation may start only after this artifact and the matching plan are captured or linked in Product Brain.
