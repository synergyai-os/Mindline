# WP-35 Eval Readback Claim Gate Plan

**Spec:** `.productbrain/specs/2026-05-28-wp-35-eval-readback-claim-gate.md`
**Stop mode:** Full Delivery
**Plan status:** signed-plan V2
**Implementation rule:** read existing artifacts and gate claims; do not create another evaluator, dashboard, hosted dependency, or destination path.

## 1. Chain Authority

1. Capture WP-35 as a work package after spec sign-off.
2. Link WP-35 to:
   - STR-3;
   - WS-2;
   - PRI-1;
   - BR-1;
   - STD-17;
   - DEC-250;
   - DEC-251;
   - DEC-252;
   - FLO-1;
   - TEN-23;
   - WP-23;
   - WP-27;
   - WP-29;
   - WP-30;
   - WP-34.
3. Run Product Brain audit after capture and before delivery authority.
4. Keep PR 28 separate; this PR must not depend on unmerged AGENTS.md edits.

## 2. Code Shape

Add a new package:

```text
internal/evalreadback/
  types.go
  reader.go
  diagnose.go
  compare.go
  writer.go
  readback_test.go
```

Primary API:

```go
type Options struct {
    BaselineRoot string
}

func Build(inputRoot, outRoot string, options Options) (Summary, error)
```

Responsibilities:

1. Resolve and contain input, baseline, and output roots.
2. Walk the input root and detect known artifact paths by relative path and schema version where possible.
3. Read only metadata fields needed for gates and deltas.
4. Convert input, baseline, output, and proof locations into safe root labels plus relative refs; never serialize private absolute paths.
5. Build claim gates.
6. Compare baseline/current when supplied.
7. Select one top improvement target.
8. Write JSON, Markdown, and Chain capture draft.

## 3. Artifact Detection

Use a closed registry of known artifact matchers:

- generic trace summary;
- corpus-pressure summary;
- corpus-pressure eval input;
- corpus-pressure trace summary;
- corpus-pressure loop summary;
- corpus acceptance benchmark summary;
- autonomy-readiness report;
- link-enrichment loop summary;
- link-enrichment comparison summary;
- link-artifact request pack;
- link-enrichment PostHog eval projection.

Matchers must return:

- artifact type;
- schema version if present;
- relative path;
- safe counters;
- safe booleans;
- known fingerprint fields;
- sample/generalization flags;
- safety/guardrail counters;
- reason codes.

Unknown files are ignored unless they look like a supported artifact with an unsupported schema version; then they are recorded as `unsupported_schema` without copying content.

## 4. Claim Gates

Implement gate functions:

1. `artifact_presence`: pass when at least one known artifact is detected.
2. `privacy_safe_readback`: fail if a generated readback field contains denied patterns.
3. `generalization_claim`: block private/temp/unknown/non-held-out results.
4. `improvement_claim`: require baseline/current comparability and positive supported deltas.
5. `dec64_no_human_claim`: block without held-out acceptance threshold proof.
6. `side_effect_claim`: pass only when artifact guardrails show zero network/hosted/destination writes, or when no side-effect evidence exists and the claim remains not evaluated.
7. `next_target`: pass only when one generalized improvement target and rerun instruction exist.

## 5. Comparison

When `--baseline` is supplied:

1. Build a readback model for the baseline without writing nested artifacts.
2. Compare compatible metrics:
   - candidate count;
   - evidence-ready/eval-counted count;
   - human-review-required/review-burden count;
   - model errors;
   - missing link enrichment;
   - needs enrichment;
   - artifact coverage;
   - failure taxonomy counts;
   - KR/eval projection result.
3. Require matching corpus/config fingerprints when both sides expose them.
4. If fingerprints are absent, mark comparison `not_comparable`; shared artifact type alone is not enough to claim improvement.
5. Write `eval-readback/comparison-summary.json`.

## 6. CLI Shape

Add top-level usage:

```bash
mindline eval readback <run-or-artifact-dir> --out <dir> [--baseline <run-or-artifact-dir>]
```

CLI requirements:

- print summary JSON to stdout;
- protect output roots using existing filesystem guard patterns;
- return usage error on missing args;
- return process error when no supported artifact is found;
- never call PostHog, Slack, browser, Product Brain, Tolaria, or network code.

## 7. Tests

Focused tests:

1. Detects all supported fixture artifact types.
2. Emits JSON, Markdown, and Chain draft.
3. Blocks generalization for private runtime and temp runtime.
4. Blocks no-human/DEC-64 claim without held-out benchmark proof.
5. Blocks improvement without baseline.
6. Reports improved/unchanged/regressed/not-comparable when baseline exists.
7. Produces one top improvement target for failed/blocked runs.
8. Rejects unsupported schema safely.
9. Generated readback artifacts pass denied-pattern leak scan.
10. CLI success, usage, no-artifact failure, and baseline comparison.
11. No network/hosted/destination side effects via fake transports or static command path proof.
12. Chain draft path sanitizer strips `/private/tmp`, Dropbox, home-directory paths, Slack permalinks, and raw private URLs.

## 8. Runtime Proof

Synthetic proof:

```bash
go run ./cmd/mindline eval readback testdata/eval-readback/current --baseline testdata/eval-readback/baseline --out /private/tmp/mindline-wp35-synthetic-readback
```

WP-34 artifact proof:

```bash
go run ./cmd/mindline eval readback /private/tmp/mindline-wp34-synthetic-v4 --baseline /private/tmp/mindline-wp34-synthetic --out /private/tmp/mindline-wp35-wp34-readback
```

Private runtime proof, when available:

```bash
go run ./cmd/mindline eval readback /private/tmp/mindline-wp34-posthog-live-v4 --out /private/tmp/mindline-wp35-private-readback
```

Private runtime proof must remain uncommitted and marked non-generalizable.

## 9. Review Proof

Before PR readiness:

1. `go test ./internal/evalreadback ./internal/cli`.
2. `go test ./...`.
3. `git diff --check`.
4. Generated-output leak scan over WP-35 runtime outputs, including proof that Chain drafts contain only safe labels and relative refs.
5. `pb audit WP-35 --phase handoff --verbose` pass or warn-only with reconciliation.
6. LOOP delivery/review subagents sign off on final output version.
