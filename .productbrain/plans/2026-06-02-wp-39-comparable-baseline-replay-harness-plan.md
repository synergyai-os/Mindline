# WP-39 Implementation Plan - Comparable Baseline Replay Harness

## Delivery Slices

1. Readback replay-baseline contract
   - Add replay-baseline types to `internal/evalreadback`.
   - Build readiness from supported artifacts, fingerprints, schemas, privacy, and side-effect gates.
   - Write the contract into `readback-summary.json`, `readback-report.md`, and `chain-capture-draft.md`.

2. Baseline summary loading
   - Add a public helper that resolves a readback summary from a direct file, an `eval-readback/` directory, a parent output directory, or a nested readback output.
   - Use that helper when applying baselines so proof-gate can consume prior readback output as baseline evidence.
   - Keep raw artifact-directory baselines working.

3. Proof-gate and loop-decision compatibility
   - Ensure `eval proof-gate --baseline <readback-output>` evaluates improvement against the baseline summary.
   - Ensure replay-blocked baselines block improvement with explicit reason codes.
   - Ensure loop-decision keeps persisting refreshed baseline-applied readback summaries.

4. CLI/runtime proof
   - Keep existing command shapes; no new command is required.
   - Runtime sequence: `documents value-proof` baseline/current fixture outputs, `eval readback` on baseline, `eval proof-gate` current with baseline readback, and `eval loop-decision` current with baseline readback.

## TDD Plan

1. RED: add readback test for `replay_baseline.status=ready` only when both corpus and command-config fingerprints exist.
2. GREEN: implement replay-baseline readiness.
3. RED: add readback test for missing command-config fingerprint blocking replay readiness.
4. GREEN: implement reason codes and report projection.
5. RED: add proof-gate test showing `--baseline <readback-output>` is consumed as baseline and passes improvement for a comparable current run.
6. GREEN: add readback-summary baseline resolution in evalreadback/evalproof.
7. RED: add proof-gate or loop-decision test showing replay-blocked baseline blocks improvement.
8. GREEN: carry replay-baseline blocked state into comparison/proof gates.

## Verification

Run, in order:

1. Focused package tests: `go test ./internal/evalreadback ./internal/evalproof ./internal/evalloopdecision ./internal/cli`.
2. Full tests: `go test ./...`.
3. Diff hygiene: `git diff --check`.
4. Runtime proof over committed value-proof fixture and readback baseline output.
5. Generated-output leak scan for denied private strings.
6. `pb audit WP-39 --phase handoff --verbose` or the closest supported audit command.
7. LOOP delivery/review sign-off.

## Stop/Abort Conditions

Stop and capture a blocker if:

- PB profile is not local `randy-s-pkm`.
- A required readback/proof contract change would weaken existing WP-35/WP-36/WP-38 proof behavior.
- Comparable baseline proof requires exposing raw private paths, source text, prompts, completions, or destination payloads.
- `eval proof-gate` cannot consume readback baselines without a larger redesign.
