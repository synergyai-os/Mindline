# WP-30 Held-Out Corpus Acceptance Benchmark Plan

## Status

Implementation plan for the signed WP-30 spec. Do not broaden into prompt tuning, UI, hosted auth, hosted DB, destination writes, or answer-key generation.

## Delivery Shape

Add a separate corpus benchmark evaluator rather than modifying WP-29 pressure readiness.

Primary API:

```go
func BuildCorpusAcceptanceBenchmark(pressurePath, answerKeyPath, outDir string, options CorpusAcceptanceBenchmarkOptions) (CorpusAcceptanceBenchmarkSummary, error)
```

Primary CLI:

```text
mindline documents corpus-acceptance <corpus-pressure-out-or-parent> --answer-key <corpus-answer-key.json> --out <dir> [--threshold 0.98] [--held-out]
```

## Steps

1. Add corpus acceptance types in `internal/documents/types.go`:
   - answer key schema;
   - suite provenance;
   - coverage requirements;
   - source expected outcomes;
   - benchmark summary;
   - per-source and per-outcome result summaries;
   - eligibility blockers.

2. Add evaluator in `internal/documents/corpus_acceptance.go`:
   - resolve pressure root or `corpus-pressure/` subdir;
   - read and validate `pressure-summary.json`;
   - read and validate answer key;
   - enforce suite validity and fingerprint/provenance gates;
   - load per-source semantic artifacts using existing acceptance readers;
   - reuse `EvaluateSemanticAcceptance` source-by-source where possible;
   - aggregate corpus-level metrics and blockers;
   - compute DEC-64 eligibility separately from accuracy.

3. Add writer in `internal/documents/corpus_acceptance_writer.go`:
   - write `corpus-acceptance/benchmark-summary.json`;
   - write `corpus-acceptance/benchmark-report.md`;
   - keep top-level outputs metadata-only.

4. Add CLI routing in `internal/cli/runner.go`:
   - usage line;
   - `runDocumentsCorpusAcceptance`;
   - argument parser;
   - JSON stdout.

5. Add tests:
   - validation and suite validity;
   - root resolution and path containment;
   - source aggregation;
   - false-positive/false-negative/wrong-kind/model-error/human-review-required blockers;
   - calibration suite cannot pass eligibility;
   - privacy-safe output assertions;
   - CLI regression.

6. Add a small fixture answer key under `testdata/documents/` only if needed for tests. It must be generic fixture data, not private `temp/` content.

7. Run real smoke:
   - produce corpus-pressure output from local `temp/*.md` into `/private/tmp`;
   - run corpus-acceptance with a deliberately small/calibration answer key or fixture as appropriate;
   - verify the benchmark either reports metrics or blocks suite validity honestly;
   - verify no hosted telemetry or destination writes.

8. Capture Chain truth:
   - `WP-30` work package;
   - sign-off decision;
   - relation links to `DEC-177`, `DEC-64`, `STR-3`, `INI-1`, `PRI-1`, `BR-1`, `STD-17`, `ARCH-1`, `WP-23`, `WP-28`, `WP-29`, and `INS-11`.

## Review Gates

Before commit/PR:

- Chain Steward: confirms no authority overclaim and Product Brain relations are present.
- Systems Architect: confirms evaluator is separate from pressure and does not rerun extraction.
- Product/Domain: confirms outcome answers "is this semantically correct enough to trust?"
- Risk/Safety/Eval: confirms suite validity, privacy, and false-trust blockers.

## Expected PR Value

After this PR, Mindline has the missing measurement layer between pressure runs and no-human claims. A developer or agent can run a corpus, evaluate it against independent held-out expectations, and know exactly why autonomy remains blocked or what quality failure class to improve next.
