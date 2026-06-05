# WP-50 Bounded Corpus Scale Gates

## Chain Authority

- INS-25: PB-doc scale needs bounded graph and packet gates.
- DEC-389: PR 44 merged into main on 2026-06-05.
- DEC-386: WP49 delivered source meaning review packets.
- DEC-153: Mindline should use local corpus graph/index proof before hosted auth/database work.
- WP-28, WP-29, WP-32, WP-37: corpus graph, pressure, acceptance, and mixed-source proof surfaces.
- PRI-1, BR-1, STD-17, DEC-64: local/private-safe proof, no destination writes, no no-human claims without held-out gates.

## Problem

The PR44/WP49 code can turn a small 50-source run into a meaning packet, but the same shipped command against `temp/pb-docs` did not complete. It reached 471 source directories, 471 segment summaries, 299 semantic summaries, roughly 8.6GB, and roughly 993,016 files before the process had to be stopped. No final `corpus-pressure/pressure-summary.json` or `source-meaning-packet/meaning-summary.json` existed.

This is a product failure because command success/failure did not leave a durable answer. A large private corpus must become bounded evidence, not an unbounded local artifact tree.

## Outcome

Mindline corpus pressure becomes scale-aware by default. When a corpus exceeds the current local budget, the run completes as a bounded, honest artifact packet:

- processed sources stay inside the configured budget;
- oversized sources are skipped before artifact-heavy semantic extraction;
- per-source semantic extraction can be stopped after segmentation when segment/candidate budgets would make the source unsafe for this local run;
- unprocessed sources are explicitly marked with stable scale reason codes;
- corpus graph generation uses bounded pair/relation budgets;
- source meaning packets expose whether their packet is complete or scale-partial;
- eval readback can see scale status and improvement targets;
- safety proof still shows zero network, hosted telemetry, hosted inference, destination writes, Product Brain writes, Tolaria writes, auto-accepts, and no-human claims.

## Non-Outcome

WP50 does not claim full PB-doc understanding, canonical cross-file deduplication, destination writes, autonomous acceptance, hosted indexing, or no-human readiness. If the PB-doc run is scale-partial, that is the correct answer for this PR.

## Key Results

1. Running `mindline documents corpus-pressure temp/pb-docs --out <dir>` with shipped defaults completes and writes `corpus-pressure/pressure-summary.json`, `eval-input.json`, `trace-summary.json`, and `corpus-graph/graph-summary.json`.
2. Running `mindline documents meaning-packet <same-run> --out <dir>` completes and writes `source-meaning-packet/meaning-summary.json`.
3. The PB-doc run reports stable scale status/reasons, including source count, source byte, segment/candidate, graph, or packet budget status when a budget applies.
4. The same 50-item fixture/proof path from WP49 still processes all 50 sources by default and remains comparable.
5. `eval readback` detects the new scale metrics/flags without leaking private content.
6. Tests cover source-budget skipping, graph pair/relation budget stopping, packet scale status, and readback evidence extraction.

## Measurable Behavior Difference

Before WP50: a too-large corpus could run indefinitely or generate hundreds of thousands of files without final summaries.

After WP50: the same corpus produces a completed, private-safe artifact packet that says "we processed this bounded slice, skipped the rest because of scale budget, and broad/full-corpus understanding is blocked until the next scale capability lands."

## Guardrails

- Defaults must be safe; users should not need to remember scale flags for the basic command.
- Existing small fixtures and the 50-source proof must not be artificially truncated.
- Scale-blocked is not a hard process failure when the bounded evidence packet is written successfully.
- All new fields are additive and schema-compatible.
- Do not commit private PB-doc artifacts or source excerpts.

## Re-Challenge And Reconciliation

First target: cap relation writes after graph generation.

Rejected as too weak. The PB-doc failure showed artifact growth before final graph and packet summaries existed. A second pass that only capped source count was also too weak because 50 very large files could still run full semantic extraction and blow the artifact budget.

The sharper WP50 target is to budget earlier at corpus-pressure source processing with count and source-size limits, then stop unsafe sources after segmentation when segment/candidate budgets would exceed the local proof envelope. It also budgets graph relation generation and packet review groups. That produces the user-visible behavior Mindline needs: a finished, comparable, honest answer even when full-corpus processing is not yet safe.
