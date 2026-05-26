# WP-29 Local Corpus Pressure Runner Plan

## Intent

Compose the existing document structure, semantic extraction, readiness/reporting, telemetry safety, and WP-28 corpus graph pieces into one local corpus pressure loop. The PR should make repeated corpus evaluation boring and reproducible, not add a new UI or destination behavior.

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

6. **CLI**
   - Add:
     `mindline documents corpus-pressure <markdown-dir-or-manifest> --out <dir> [--classifier deterministic|llm --llm-provider <provider> --llm-model <model>]`
   - Print `pressure-summary.json` to stdout.
   - Keep usage and errors explicit.

7. **Tests**
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

8. **Real temp smoke**
   - Run default corpus pressure over `temp/` into `/private/tmp`.
   - Verify all `temp/*.md` are accounted for.
   - Verify no hosted model/provider call by default.
   - Verify no destination writes.
   - Capture summary metrics and whether `ready_for_50_file_pressure` is true or false.

9. **Review and close**
   - Run `go test ./...`.
   - Run `git diff --check`.
   - Run PB audit/reconciliation.
   - Run LOOP reviewer panel on final implementation and proof.
   - Capture final Chain truth before PR.

## Expected Files

- `internal/documents/corpus_pressure.go`
- `internal/documents/corpus_pressure_writer.go`
- `internal/documents/corpus_pressure_test.go`
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
- temp smoke requires committing private artifacts;
- privacy-safe no-hosted default cannot be proven with tests;
- PB audit says WP-29 conflicts with active strategy or workstream authority.
