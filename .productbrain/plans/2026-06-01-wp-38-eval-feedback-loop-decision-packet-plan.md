# WP-38 Implementation Plan — Eval Feedback Loop Decision Packet

## Implementation Slices

1. Add `internal/evalloopdecision`
   - Define packet/report schema.
   - Load current evidence through `evalreadback.Build` or existing readback summary.
   - Load optional baseline through readback comparison.
   - Optionally read existing WP-36 proof packet and WP-37 value-proof summary when present.
   - Select exactly one top improvement target.
   - Compute improvement state: `improved`, `not_improved`, `inconclusive`, `not_comparable`, or `blocked_missing_baseline`.
   - Copy safety/generalization/DEC-64 claim status from readback/proof gates.

2. Add safe writers
   - Write `decision-packet.json`, `decision-report.md`, and `chain-capture-draft.md`.
   - Reuse readback output validation and denied-pattern checks.
   - Keep refs relative or hashed; no absolute private paths.

3. Add CLI
   - `mindline eval loop-decision <current> --out <dir> [--baseline <baseline>]`.
   - Reject destination/profile/provider flags.
   - Return exit 0 for safe decision-support packets even when improvement/generalization/DEC-64 are blocked; return nonzero only for invalid input, unsafe output, or write failure.

4. Add tests
   - Current-only fixture produces blocked improvement and one target.
   - Comparable baseline/current fixture produces improved state.
   - Non-comparable or missing baseline does not claim improvement.
   - Unsafe output patterns are rejected.
   - CLI usage and output files.

5. Runtime verification
   - Run WP-37 fixture through `documents value-proof`.
   - Run `eval loop-decision` current-only.
   - Run `eval readback` and `eval proof-gate --claim safety` over the decision output if supported, or leak-scan the decision output directly.
   - Run `go test ./internal/evalloopdecision ./internal/cli`.
   - Run `go test ./...`.
   - Run `git diff --check`.
   - Run `pb audit WP-38 --phase handoff --verbose`.

## Acceptance

The work is complete only when:

- WP-38 is captured on Chain with relations to WP-27, WP-35, WP-36, WP-37, FLO-1, DEC-250, DEC-64, PRI-1, BR-1, STD-17, and TEN-23.
- The command emits one prioritized product-general improvement target.
- The command refuses to turn missing/non-comparable baseline into an improvement claim.
- The decision report is plain English and agent-actionable.
- Generalization and DEC-64 remain blocked without held-out proof.
- All verification commands pass.
- LOOP reviewers sign off on the final implementation.
