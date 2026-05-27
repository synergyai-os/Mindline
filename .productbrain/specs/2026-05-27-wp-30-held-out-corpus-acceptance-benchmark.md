# WP-30 Held-Out Corpus Acceptance Benchmark

## Status

Signed shaping target for the next PR after PR #22 / WP-29. This slice turns corpus pressure output into a measured semantic-correctness benchmark. It does not improve extraction quality directly; it makes correctness, false trust, and DEC-64 eligibility measurable on a held-out suite.

## Authority

- `DEC-177`: PR #22 / WP-29 merged to main at merge commit `6f069d4`.
- `WP-29`: local corpus pressure and loop artifacts exist, but pressure/readiness is not semantic accuracy.
- `DEC-64`: no-human semantic ingestion requires held-out accuracy >=98% and safety guardrails.
- `STR-3`: autonomy-readiness before destination writes.
- `INI-1`: Semantic autonomy readiness cycle.
- `PRI-1` and `BR-1`: privacy-by-design and traceability-by-design; hosted observability is metadata-only.
- `STD-17` and `ARCH-1`: provider-agnostic semantic measurement before trust.
- `WP-23`: autonomy readiness evidence report defines no-human eligibility as a gated evidence claim, not a feeling.
- `WP-28`: local corpus graph/index substrate and relation candidate metrics.
- `INS-11`: semantic quality needs capture-level expected outcomes, including false negatives and review burden, not only generated candidate IDs.

## Diagnosis

WP-29 gives Mindline a repeatable way to run a local corpus pressure loop and produce metadata-only trace/eval artifacts. That proves the system can account for files, produce semantic candidates, build a graph, and stop honestly when the same binary/input/config cannot improve.

It still cannot answer the product question Randy actually needs before loading 50 files or writing to Tolaria: "Was the semantic extraction correct?"

Pressure readiness can be high while semantic correctness is poor. A source can be processed, evidence-ready, and graph-connected while still missing the important item, producing the wrong kind, inventing an unexpected candidate, or creating duplicate/contradiction relation noise. The next PR must therefore add an independent held-out benchmark over corpus-pressure outputs.

The first shape was too weak because a 98% claim is meaningless on a tiny or contaminated suite. The signed shape therefore requires a first-class corpus answer key with suite validity gates: minimum eval count, source/kind/relation/failure-mode coverage, answer-key provenance independent from the run being evaluated, frozen corpus/manifest fingerprints, and privacy-safe outputs.

## Outcome

Mindline can evaluate an existing local corpus-pressure run against a first-class corpus answer key and produce an honest benchmark answer:

- how many expected semantic outcomes were correct;
- what was missing;
- what was unexpected;
- what had the wrong kind;
- where duplicate or contradiction relation quality failed;
- what still requires human review;
- whether the suite itself is valid enough to support a held-out trust claim;
- whether DEC-64 no-human eligibility is still blocked, and why.

The benchmark is local-first and read-only. It consumes artifacts already produced by corpus-pressure. It does not rerun semantic extraction, send hosted telemetry by default, write to Tolaria, write to Product Brain, create auth/login, create a DB, or apply destination writes.

## Scope

Implement:

1. `mindline documents corpus-acceptance <corpus-pressure-out-or-parent> --answer-key <corpus-answer-key.json> --out <dir> [--threshold 0.98] [--held-out]`.
2. A first-class corpus answer-key schema with:
   - `suite_id`;
   - `suite_kind`: `held_out` or `calibration`;
   - `provenance`: independent author/source marker, not generated from the evaluated run;
   - expected `corpus_id`, `corpus_fingerprint`, and optional command/config fingerprint;
   - `min_eval_count`;
   - source, candidate-kind, relation, and failure-mode coverage requirements;
   - source-scoped expected outcomes keyed by stable `source_id`.
3. A corpus benchmark evaluator that consumes:
   - `corpus-pressure/pressure-summary.json`;
   - per-source `semantic-candidates/` artifacts referenced by `SemanticRunDir`;
   - `corpus-graph/graph-summary.json` when relation metrics are available;
   - the local answer key.
4. A metadata-only benchmark summary with:
   - suite validity verdict;
   - accuracy and DEC-64 eligibility verdict;
   - false positive / false negative / wrong kind / duplicate / contradiction / model-error / unjudged / human-review-required counts and rates;
   - review burden count and rate;
   - safety and destination guardrail counters copied from pressure;
   - per-source result summaries using IDs, counts, hashes, reason codes, and local-relative artifact refs only.
5. A readable benchmark report that explains the verdict and next improvement target without exposing raw private content.
6. Tests proving the benchmark is not hardcoded to `temp/` and works on fixture corpora.

## Input Contract

Command:

```text
mindline documents corpus-acceptance /private/tmp/mindline-pressure --answer-key testdata/documents/corpus-answer-key.json --out /private/tmp/mindline-benchmark --held-out --threshold 0.98
```

The first argument may be:

- the corpus-pressure output root; or
- the `corpus-pressure/` subdirectory itself.

The evaluator resolves the pressure root, reads `corpus-pressure/pressure-summary.json`, and uses each processed source's `semantic_run_dir` to load source-scoped semantic artifacts.

Answer key shape:

```json
{
  "schema_version": "corpus-acceptance-answer-key/v0.1",
  "suite_id": "heldout-core-001",
  "suite_kind": "held_out",
  "provenance": {
    "labeler": "human-curated",
    "independence": "not_generated_from_evaluated_run"
  },
  "corpus_id": "corpus-demo",
  "corpus_fingerprint": "corpus-...",
  "min_eval_count": 50,
  "coverage_requirements": {
    "min_source_count": 5,
    "candidate_kinds": ["action_candidate", "capability_candidate"],
    "relation_types": ["possible_duplicate", "contradicts"],
    "failure_modes": ["missing_expected_outcome", "unexpected_candidate", "wrong_kind"]
  },
  "sources": [
    {
      "source_id": "meeting-transcript-1",
      "expected_outcomes": []
    }
  ]
}
```

Expected outcomes reuse the existing semantic acceptance matching vocabulary where possible: expected state, candidate kind, required evidence node IDs, acceptable evidence alternates, title signals, summary signals, relation requirements, and minimum confidence floor.

## Eligibility Semantics

`dec64_eligible` is true only when all are true:

- the run is explicitly evaluated as held-out;
- `suite_kind` is `held_out`;
- answer-key provenance says it was not generated from the evaluated run;
- suite validity passes minimum eval count and coverage requirements;
- corpus ID and corpus fingerprint match the pressure summary;
- accuracy is at least the requested threshold, default `0.98`;
- false positive count is zero;
- false negative count is zero;
- wrong-kind count is zero;
- model-error count is zero;
- unjudged count is zero;
- counted human-review-required count is zero;
- safety blocked/private count is zero for counted eval items;
- pressure guardrails show zero destination writes and zero hosted telemetry exports;
- calibration suites never satisfy autonomy eligibility.

The benchmark may report high accuracy without eligibility. That distinction is intentional.

## Privacy Rules

Benchmark summary and report must not include:

- raw private source text;
- source excerpts;
- prompts;
- raw model completions;
- absolute source paths;
- answer-key rationale that reconstructs private content;
- Product Brain or Tolaria destination payloads.

Allowed fields:

- suite IDs;
- source IDs;
- source content hashes;
- corpus and config fingerprints;
- counts and rates;
- candidate IDs;
- expected outcome IDs;
- closed reason codes;
- local-relative artifact paths under the output directory.

## KRs

1. **Corpus-level correctness:** a single command evaluates a corpus-pressure output against a corpus answer key and emits canonical JSON plus a readable report.
2. **No rerun contamination:** corpus-acceptance consumes existing pressure output and does not rerun semantic extraction or mutate pressure artifacts.
3. **First-class held-out validity:** answer keys include suite kind, provenance independence, min eval count, coverage requirements, corpus fingerprint, and source-scoped expected outcomes.
4. **False-trust blockers:** DEC-64 eligibility is blocked by any false positive, false negative, wrong kind, model error, unjudged item, counted human-review-required item, safety issue, hosted telemetry export, or destination write.
5. **Calibration cannot pass:** calibration suites can produce metrics but can never set `dec64_eligible=true`.
6. **Privacy-safe artifacts:** benchmark summary/report contain no raw source text, excerpts, prompts, completions, or absolute source paths.
7. **No bespoke temp logic:** fixture tests prove the evaluator is corpus/answer-key driven and does not rely on private `temp/` filenames or content.
8. **Real temp smoke:** a real local smoke run over `temp/*.md` pressure output can produce a benchmark result or an explicit suite-validity block without crash, hosted telemetry, destination writes, or private text in top-level benchmark artifacts.
9. **Provider agnostic:** the benchmark evaluates artifacts and is independent of whether semantic candidates came from deterministic or LLM classifiers.
10. **Audit-ready Chain:** WP-30 spec, plan, decision, relations, and verification evidence are captured so Product Brain can explain why this PR exists and what it does not claim.

## Anti-Goals

- No extraction-quality prompt tuning in this PR.
- No UI changes.
- No auth/login.
- No hosted database.
- No Tolaria writes.
- No Product Brain writes beyond Chain governance capture.
- No destination proposal apply path.
- No auto-accept, auto-merge, auto-resolution, or authority claims.
- No hosted eval export by default.
- No no-human readiness claim unless the benchmark gates actually pass.
- No hidden permanent local database.
- No answer-key generation from the evaluated run.
- No optimizing specifically for private `temp/` files.

## Verification

Required before PR:

- Unit tests for answer-key validation, including invalid suite kind, tiny suite, missing coverage, duplicate source IDs, generated-from-run provenance, corpus fingerprint mismatch, and calibration non-eligibility.
- Unit tests for corpus-pressure root resolution and containment.
- Unit tests for false positive, false negative, wrong-kind, human-review-required, model-error, and safety blocker accounting.
- Unit tests proving benchmark output omits private source text, excerpts, prompts, completions, and absolute paths.
- CLI regression for `documents corpus-acceptance`.
- Fixture corpus proving benchmark works without private `temp/` content.
- Real `temp/*.md` pressure + acceptance smoke into `/private/tmp`.
- `go test ./...`
- `git diff --check`
- `pb audit WP-30 --phase shaping --verbose` or closest available audit.
