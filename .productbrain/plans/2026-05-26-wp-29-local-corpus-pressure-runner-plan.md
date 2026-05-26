# WP-29 Local Corpus Pressure Runner Plan

## Intent

Compose the existing document structure, semantic extraction, readiness/reporting, telemetry safety, and WP-28 corpus graph pieces into one local corpus pressure loop. The PR should make repeated corpus evaluation boring and reproducible, not add a new UI or destination behavior.

2026-05-26 addendum: raise the PR to an eval-loop foundation. The loop must generate local metadata-only eval/trace artifacts, run up to 20 pressure iterations, record deltas and stop reasons, and avoid fake progress by separating processed, skipped, excluded, and blocked sources.

## Sequence

1. **Types and contracts**
   - Add corpus pressure schema constants and structs under `internal/documents`.
   - Define manifest, source record, source result, summary metrics, blocker, and replay fingerprint types.
   - Define closed skip/block reason codes.

2. **Input loading**
   - Support either Markdown directory input or `corpus-pressure-manifest/v0.1`.
   - Resolve source paths relative to the directory or manifest.
   - Reject duplicate source IDs, unsupported source kinds, path traversal, and symlink escapes.
   - Exclude generated output/cache/tool directories from directory discovery.

3. **Per-source run envelope**
   - For each source, create a stable per-source artifact directory.
   - Run or reuse structure/semantic generation through existing document APIs.
   - Default to deterministic/local classifier.
   - Pass LLM classifier/provider/model only when explicit CLI options are present.
   - Record processed, skipped, or blocked with reason codes instead of silently dropping files.

4. **Graph composition**
   - Generate a WP-28 `corpus-graph-manifest.json` from processed semantic run directories.
   - Invoke the existing corpus graph builder/writer.
   - Preserve graph summary path and graph replay fingerprint in the pressure summary.

5. **Pressure metrics and report**
   - Compute source accounting, candidate counts, graph counts, evidence readiness, review burden, readiness boolean, blockers, and next-improvement targets.
   - Write `pressure-report.md` as the human/agent-facing corpus answer, with sections for corpus answer, source accounting, extracted candidates by source, connected clusters, duplicate candidates, contradiction candidates, evidence/readiness failures, and prioritized next improvement targets.
   - Write:
     - `corpus-pressure/pressure-summary.json`
     - `corpus-pressure/pressure-report.md`
   - Keep raw private excerpts local to semantic/graph artifacts only.

6. **Eval and trace artifacts**
   - Write `corpus-pressure/eval-input.json` with metadata-only evaluation inputs, guardrail counters, summary paths, fingerprints, and readiness counters.
   - Write `corpus-pressure/trace-summary.json` with metadata-only stage execution status, processed/skipped/excluded/blocked counters, processed/skipped/excluded/blocked deltas when previous metrics are available, fingerprints, and no-hosted/no-telemetry/no-destination counters.
   - Ensure neither artifact contains raw source excerpts, prompts, completions, or reconstructable private text.

7. **Loop harness**
   - Add `mindline documents corpus-pressure-loop <markdown-dir-or-manifest> --out <dir> [--max-runs <n>]`.
   - Cap `--max-runs` at 20.
   - Store each iteration under `iterations/<n>/`.
   - Write `corpus-pressure-loop/loop-summary.json` and `loop-report.md`.
   - Record build/git fingerprint, command config fingerprint, corpus fingerprint, pressure fingerprint, metrics, deltas, KR pass/fail, and stop reason.
   - Stop honestly as `same_binary_same_inputs` or `no_change_detected` when the executable/config/input and metrics cannot move.

8. **Raised pressure gates**
   - Implement KR evaluation with separate counters for processed, skipped, excluded, and blocked sources.
   - Require 100% source accounting, >=95% processed source ratio, 0 blocked sources, 0 unexplained exclusions, >=90% evidence-ready atoms on counted processed atoms, deterministic replay, and no hosted/default destination behavior.
   - Do not let skipped/excluded sources count as evidence-ready, accuracy-improving, or loop-improving.

9. **CLI**
   - Add:
     `mindline documents corpus-pressure <markdown-dir-or-manifest> --out <dir> [--classifier deterministic|llm --llm-provider <provider> --llm-model <model>]`
   - Add:
     `mindline documents corpus-pressure-loop <markdown-dir-or-manifest> --out <dir> [--max-runs <n>] [--classifier deterministic|llm --llm-provider <provider> --llm-model <model>]`
   - Print `pressure-summary.json` to stdout.
   - Print `loop-summary.json` to stdout for the loop command.
   - Keep usage and errors explicit.

10. **Tests**
   - Directory discovery and manifest containment.
   - Duplicate source IDs and unsupported kinds fail closed.
   - Default offline/no-hosted behavior.
   - Telemetry-enabled default run with a fake PostHog/network transport or equivalent fail-on-call harness proving zero hosted telemetry/network export.
   - CLI regression proving corpus-pressure produces only pressure, semantic, and graph artifacts, with no destination adapter invocation and no Tolaria/Product Brain/destination artifacts.
   - Explicit LLM option plumbing without provider lock-in.
   - Graph composition and output paths.
   - Three-run deterministic replay.
   - No bespoke temp filename assumptions.
   - CLI integration.
   - Eval/trace artifact schemas and privacy redaction.
   - Loop summary schema, stop reasons, max-run cap, KR pass/fail.
   - Guardrail test proving unchanged executable/input/config does not claim improvement.
   - Raised KR denominator tests proving skipped/excluded sources do not count as processed or evidence-ready.
   - Trace-summary tests proving processed/skipped/excluded/blocked counters and deltas exist.

11. **Real temp smoke and loop**
   - Run default corpus pressure over `temp/` into `/private/tmp`.
   - Verify all `temp/*.md` are accounted for.
   - Verify no hosted model/provider call by default.
   - Verify no destination writes.
   - Capture summary metrics and whether `ready_for_50_file_pressure` is true or false.
   - Run the loop command over `temp/` into `/private/tmp`.
   - Stop only because KRs pass, same binary/input/config cannot improve further, or 20 iterations are exhausted.

12. **Review and close**
   - Run `go test ./...`.
   - Run `git diff --check`.
   - Run PB audit/reconciliation.
   - Run LOOP reviewer panel on final implementation and proof.
   - Capture final Chain truth before PR.

## Expected Files

- `internal/documents/corpus_pressure.go`
- `internal/documents/corpus_pressure_writer.go`
- `internal/documents/corpus_pressure_test.go`
- `internal/documents/corpus_pressure_loop.go`
- `internal/cli/runner.go`
- `internal/cli/documents_decompose_test.go` or a focused CLI test file
- `testdata/documents/corpus-pressure/...`
- `.productbrain/specs/2026-05-26-wp-29-local-corpus-pressure-runner.md`
- `.productbrain/plans/2026-05-26-wp-29-local-corpus-pressure-runner-plan.md`

## Stop Conditions

Stop and capture a blocker if:

- existing document APIs cannot be composed without duplicating semantic extraction logic;
- graph composition requires hosted inference or destination writes;
- source accounting cannot be made deterministic;
- loop artifacts would need raw private text;
- unchanged binary/input/config would require pretending the system improved;
- temp smoke requires committing private artifacts;
- privacy-safe no-hosted default cannot be proven with tests;
- PB audit says WP-29 conflicts with active strategy or workstream authority.
