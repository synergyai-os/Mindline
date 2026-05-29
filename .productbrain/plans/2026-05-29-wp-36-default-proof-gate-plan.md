# WP-36 Default Eval Proof Gate Plan

**Spec:** `.productbrain/specs/2026-05-29-wp-36-default-proof-gate.md`
**Stop mode:** Full Delivery
**Plan status:** signed-plan-candidate V2
**Implementation rule:** make proof executable; do not create a new evaluator, hosted dependency, destination path, or corpus-specific shortcut.

## 1. Chain Authority

1. Review draft scope with LOOP reviewers before Chain capture.
2. Capture WP-36 as a work package.
3. Link WP-36 to:
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
   - WP-35;
   - DEC-296.
4. Run `pb audit WP-36 --phase handoff --verbose`.
5. Reconcile any material audit warnings before implementation.

## 2. Code Shape

Add a policy layer beside readback:

```text
internal/evalproof/
  types.go
  proof.go
  writer.go
  proof_test.go
```

Primary API:

```go
type Options struct {
    BaselineRoot string
    Policy string
    ProtectedRoots []string
}

func Build(inputRoot, outRoot string, options Options) (Summary, error)
```

Responsibilities:

1. Validate selected policy.
2. Build WP-35 readback into `eval-proof/readback`, or load an existing readback summary.
3. Evaluate mandatory gates against `evalreadback.Summary`.
4. Emit pass/fail/blocked verdict and suggested exit code.
5. Write JSON, Markdown, and Chain draft.
6. Keep all outputs safe for PR and Chain capture.

## 3. Claim Profiles

`safety` mandatory gates:

- `artifact_presence`: pass.
- `privacy_safe_readback`: pass.
- `side_effect_claim`: pass.
- `unsupported_schema`: no unsupported artifacts.

`improvement` mandatory gates:

- all `safety` gates;
- `improvement_claim`: pass;
- hard blocker: missing `--baseline` emits `blocked` with `missing_baseline`.

`generalization` mandatory gates:

- all `safety` gates;
- `generalization_claim`: pass.

`dec64` mandatory gates:

- all `safety` gates;
- `dec64_no_human_claim`: pass, with supporting readback evidence for held-out labels, threshold >=98%, no-human eligibility, absence of non-generalizable evidence, and zero destination-write/autonomy side-effect counters.

`next_target` remains diagnostic and is reported but not mandatory proof.

## 4. CLI

Add usage:

```bash
mindline eval proof-gate <run-or-readback-dir> --out <dir> --claim safety|improvement|generalization|dec64 [--baseline <run-or-artifact-dir>]
```

CLI behavior:

- no default claim; the operator must name the intended claim;
- print proof packet JSON to stdout;
- return `ExitOK` only for `verdict=pass`;
- return `ExitProcess` for gate fail/block;
- return `ExitUsage` for malformed args;
- always include deterministic `claim`, `verdict`, `exit_code`, and stable failure codes in proof packets for fail/block results produced after valid args;
- enforce existing output-root protection.

## 5. Tests

Focused tests:

1. `safety` passes on clean fixture output without baseline.
2. `improvement` without baseline fails with `missing_baseline`.
3. Not-comparable baseline fails.
4. Unchanged and regressed comparisons fail.
5. Unsafe artifact fails.
6. Unsupported schema fails.
7. Side-effect counter regression/failure fails.
8. `generalization` fails when generalization is blocked.
9. `dec64` fails when DEC-64/no-human is blocked.
10. Generated JSON, Markdown, Chain draft, and nested readback artifacts exist.
11. Proof outputs pass denied-pattern leak scan.
12. CLI exit codes match policy verdicts.
13. Existing `eval readback` remains a reporting command and returns success with blocked claims.
14. Existing `eval-readback/readback-summary.json` input produces the same verdict as artifact-dir input.

## 6. Runtime Proof

Synthetic pass:

```bash
go run ./cmd/mindline eval proof-gate testdata/eval-readback/current --baseline testdata/eval-readback/baseline --out /private/tmp/mindline-wp36-proof-improvement --claim improvement
```

Synthetic stricter-policy block:

```bash
go run ./cmd/mindline eval proof-gate testdata/eval-readback/current --baseline testdata/eval-readback/baseline --out /private/tmp/mindline-wp36-proof-generalization --claim generalization
```

Synthetic missing baseline failure:

```bash
go run ./cmd/mindline eval proof-gate testdata/eval-readback/current --out /private/tmp/mindline-wp36-proof-missing-baseline --claim improvement
```

## 7. Review Proof

Before PR readiness:

1. `go test ./internal/evalproof ./internal/evalreadback ./internal/cli`.
2. `go test ./...`.
3. `git diff --check`.
4. Leak scan over `/private/tmp/mindline-wp36-proof-*`.
5. `pb audit WP-36 --phase handoff --verbose`.
6. LOOP delivery/review sign-off from product, eval architecture, and privacy/operability lenses.

## 8. Non-Goals

- Do not add CI enforcement in this PR.
- Do not query PostHog.
- Do not write Product Brain automatically.
- Do not create a database or login.
- Do not change extraction/classification prompts.
- Do not add destination write behavior.
