# WP-50 Plan: Bounded Corpus Scale Gates

## Scope

Add default scale budgets to corpus pressure, corpus graph, source meaning packet, and eval readback evidence. Use the same `temp/pb-docs` corpus as the private runtime comparison target.

## Implementation Plan

1. Add `CorpusPressureScaleBudget` with safe defaults:
   - max processed sources defaults to 50;
   - max source bytes defaults to a conservative local proof size;
   - max per-source segments/semantic candidates defaults keep one large source from dominating artifacts;
   - graph pair comparisons and relation candidates are bounded;
   - packet review groups are bounded.
2. Apply source budget before per-source semantic extraction:
   - process only the first budgeted sources in stable source order;
   - skip oversized source files before copying/running semantic extraction;
   - after document segmentation, skip semantic extraction for sources that exceed per-source segment/candidate budget and mark them as scale-blocked;
   - append skipped source results for the rest;
   - use stable reason codes `scale_source_limit`, `scale_source_size_limit`, and `scale_segment_limit`;
   - expose scale status, limits, and skipped counts in pressure summary/eval/trace.
3. Add `CorpusGraphOptions`:
   - bounded pair comparison count;
   - bounded relation candidate count;
   - graph summary fields for scale status/reasons/limits.
4. Add packet scale status:
   - limit emitted review groups;
   - expose omitted atom count and reason codes;
   - keep packets write-ineligible and destination-neutral.
5. Extend readback metric/flag extraction:
   - scale status flags;
   - source/graph/packet budget metrics;
   - scale reason codes as readback evidence where possible.
6. Add focused tests:
   - corpus pressure skips over-budget sources but writes final summaries;
   - graph generation stops at pair/relation budget and still writes a graph summary;
   - source meaning packet reports partial packet coverage when capped;
   - readback extracts scale metrics and flags.
7. Verify:
   - focused Go tests for documents/readback/CLI;
   - full `go test ./...`;
   - WP49 50-item proof still processes 50/50;
   - PB-doc private runtime completes with summaries and packet;
   - readback/proof-gate safety artifacts are private-safe.

## Acceptance Criteria

- `go test ./...` passes.
- Private PB-doc run with shipped defaults completes and writes final corpus pressure and meaning packet summaries.
- PB-doc readback reports scale-bounded status and zero prohibited side effects.
- PB-doc readback chooses scale capacity as the top improvement target when scale budgets are hit.
- 50-item proof still processes all 50 sources and remains comparable.
- Product Brain captures the learning and delivery proof.

## Risks

- Default max sources may make large-corpus proof explicitly partial. That is acceptable for WP50 because partial, bounded truth is the desired behavior until streaming/full-corpus scale work exists.
- Relation caps may reduce relation recall on very dense small corpora. Tests should preserve current small fixture behavior and expose cap status when it applies.
- Packet caps can hide some atoms from review. The summary must make omitted atom count visible.

## Reviewer Sign-Off Targets

- Product/Domain reviewer: confirms the user behavior is honest and useful.
- Systems reviewer: confirms budgets sit at the right pipeline layers and remain source/destination agnostic.
- Eval/Safety reviewer: confirms proof/readback semantics block overclaiming and expose private-safe evidence.
