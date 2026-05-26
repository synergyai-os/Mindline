# WP-29 Local Corpus Pressure Runner

## Status

Signed target for the next PR after PR #21 / WP-28. This slice turns the existing per-file semantic commands and WP-28 corpus graph into one repeatable local corpus pressure loop.

## Authority

- `DEC-164`: PR #21 / WP-28 merged to main at merge commit `7f8d240`.
- `WP-28`: local corpus graph/index substrate exists, but it consumes already-prepared semantic artifacts.
- `DEC-153`: add local corpus graph/index before hosted auth or hosted DB.
- `DEC-154`: hosted auth/DB remains blocked; duplicate and contradicting atoms stay reviewable evidence-backed relation candidates, not auto-resolved.
- `STR-3`: autonomy-readiness before destination writes.
- `DEC-64`: no-human semantic ingestion requires held-out accuracy >=98% and safety guardrails.
- `PRI-1` and `BR-1`: privacy-by-design and traceability-by-design; hosted observability is metadata-only.
- `STD-17` and `ARCH-1`: provider-agnostic measurement before trust.
- `WP-23` and `WP-27`: local readiness/eval and privacy-safe traceability are canonical feedback-loop surfaces.

## Diagnosis

WP-28 proved that Mindline can build a local graph from semantic artifacts, but the larger product loop is still too manual. A 50-file pressure run currently requires an agent or human to run per-file semantics, assemble a graph manifest, run corpus graph, inspect scattered artifacts, and infer what improved or failed.

That is the real blocker before loading larger private corpora or doing destination writes. The system needs one reproducible local run envelope that accounts for every source, runs or reuses semantic artifacts, builds the graph, and writes a single readiness report with failures and next-improvement targets.

## Outcome

Mindline can run a local corpus pressure loop over a directory or manifest of Markdown sources and produce one auditable output directory that answers:

- which files were considered;
- which files produced semantic candidates;
- which files were skipped or blocked and why;
- which graph atoms and relation candidates were produced;
- whether evidence, privacy, and deterministic replay gates passed;
- whether the corpus is ready for larger 50-file pressure;
- what the next improvement target is.

The runner is local-first and read-only. It does not write to Tolaria, Product Brain, a hosted DB, or any destination. Hosted LLM inference is never used by default and is allowed only through explicit classifier/provider options.

## Scope

Implement:

1. `mindline documents corpus-pressure <markdown-dir-or-manifest> --out <dir>`
2. Input discovery for Markdown directories and a small JSON manifest form.
3. A stable local run directory with:
   - per-source structure/semantic artifacts;
   - generated `corpus-graph-manifest.json`;
   - WP-28 `corpus-graph/` artifacts;
   - `corpus-pressure/pressure-summary.json`;
   - `corpus-pressure/pressure-report.md`.
4. Per-source accounting:
   - discovered source count;
   - processed source count;
   - skipped source count;
   - blocked source count;
   - semantic candidate count;
   - skip/block reason codes from a closed vocabulary.
5. Graph/readiness accounting:
   - graph atom count;
   - relation count by type and status;
   - evidence-ready atom/relation counts;
   - review burden count and ratio;
   - ready-for-50-file-pressure boolean;
   - blockers and next-improvement targets.
6. Deterministic replay fingerprint for the pressure run.
7. Default offline/local behavior using deterministic semantics unless the user explicitly selects `--classifier llm --llm-provider <provider> --llm-model <model>`.
8. A human/agent-facing report contract that answers the corpus question without requiring JSON archaeology.
9. Tests for manifest/directory containment, source accounting, no-hosted-by-default behavior, no hosted telemetry export by default, no destination writes, graph composition, deterministic replay, and temp smoke.

## Input Contract

Directory input:

```text
mindline documents corpus-pressure temp --out /private/tmp/mindline-pressure
```

The runner recursively discovers `*.md` files under the input directory, excluding generated output directories and hidden tool/cache directories.

Manifest input:

```json
{
  "schema_version": "corpus-pressure-manifest/v0.1",
  "corpus_id": "local-pressure-corpus",
  "sources": [
    {
      "source_id": "meeting-transcript-1",
      "source_kind": "markdown",
      "path": "meeting-transcript-1.md"
    }
  ]
}
```

Requirements:

- source paths resolve relative to the input root or manifest directory;
- paths must not escape through `..` or symlink traversal;
- source IDs are stable and duplicate source IDs fail closed;
- generated corpus graph manifests use the same stable source IDs;
- private source excerpts remain local to artifacts and never enter hosted telemetry.

## Readiness Semantics

The runner must not hide failures to make metrics look better.

Source states:

- `processed`: source produced semantic summary and candidate artifacts.
- `skipped`: source was intentionally ignored with a non-error reason, such as unsupported generated artifact or no extractable semantic candidates.
- `blocked`: source failed a safety, containment, schema, provider, or evidence gate.

Ready for 50-file pressure is true only when:

- every discovered Markdown source is accounted for as processed, skipped, or blocked;
- blocked source count is zero;
- processed source count is greater than zero;
- graph summary exists and has a non-empty replay fingerprint;
- evidence-ready atom ratio is at least 0.90 for counted atoms;
- review burden ratio is at most 0.20 for relation candidates;
- no hosted provider call occurred unless explicit LLM options were supplied;
- no destination write occurred.

If not ready, the report must list blocker reason codes and the next improvement targets.

## Pressure Report Contract

`corpus-pressure/pressure-report.md` is not only a metrics dump. It is the primary human/agent-facing answer for the corpus run. It must include these sections:

1. **Corpus answer**: one short summary of what the run found and whether it is ready for larger pressure.
2. **Source accounting**: processed, skipped, and blocked sources with source IDs, labels, reason codes, and artifact paths.
3. **Extracted candidates by source**: candidate counts by source and candidate kind, with local artifact paths to inspect details.
4. **Connected clusters**: same-topic or connected groups with atom IDs, source IDs, relation types, statuses, and local artifact paths.
5. **Duplicate candidates**: possible duplicate relation candidates with confidence, status, atom IDs, source IDs, and local artifact paths.
6. **Contradiction candidates**: contradiction relation candidates with confidence, status, atom IDs, source IDs, and local artifact paths.
7. **Evidence/readiness failures**: blocked or evidence-incomplete sources, atoms, and relations with closed reason codes.
8. **Next improvement targets**: prioritized categories explaining what should improve next, such as extraction coverage, evidence completeness, duplicate precision, contradiction coverage, review burden, or safety containment.

The report may include titles, source IDs, relation types, statuses, counts, reason codes, and local artifact paths. Raw private excerpts remain local to semantic and graph artifacts and are not required in this top-level report.

## KRs

Aggressive delivery KRs:

1. **One-command corpus loop:** one CLI command creates per-source semantic artifacts, graph artifacts, and a pressure report.
2. **Complete source accounting:** 100% of discovered `*.md` files are represented in `pressure-summary.json` as processed, skipped, or blocked.
3. **Deterministic replay:** three runs over the same fixture corpus produce the same source IDs, generated graph manifest, graph replay fingerprint, and pressure replay fingerprint.
4. **No hosted by default:** default corpus-pressure runs make zero hosted LLM calls and zero hosted telemetry exports.
5. **Explicit hosted opt-in:** LLM classifier options are passed only when explicitly requested and remain provider-agnostic.
6. **Graph composition:** the runner invokes/reuses WP-28 corpus graph generation and writes graph artifacts for every processed semantic run.
7. **Evidence floor:** counted graph atoms and relation candidates inherit WP-28 evidence requirements; missing evidence is blocked or excluded from readiness counts.
8. **Temp smoke:** all local `temp/*.md` files are accounted for in a real smoke run without crash, destination writes, or hosted inference by default.
9. **No bespoke temp logic:** tests prove the runner works on fixture corpora without relying on private `temp/` filenames or content.
10. **Actionable improvement target:** when `ready_for_50_file_pressure=false`, the report names concrete blocker reason codes and next target categories.
11. **Readable corpus answer:** `pressure-report.md` includes source accounting, extracted candidate summaries, connected clusters, duplicate/contradiction candidates, evidence failures, and next improvement targets without requiring direct JSON inspection.

## Anti-Goals

- No UI or human review surface changes.
- No hosted auth/login.
- No hosted database.
- No Tolaria writes.
- No Product Brain writes.
- No destination proposal apply path.
- No auto-accept, auto-merge, auto-resolution, or authority claims.
- No provider-specific core architecture.
- No hidden permanent local database.
- No tuning to private `temp/` filenames or content.
- No claim that DEC-64 no-human readiness has been met.

## Verification

Required before PR:

- Unit tests for directory and manifest containment.
- Unit tests for source discovery/accounting and skip/block reason vocabulary.
- Unit tests for deterministic replay over three runs.
- Unit tests proving default runs do not initialize hosted providers.
- Unit or CLI regression proving default runs perform zero hosted telemetry/network exports even when telemetry environment variables are enabled, using a fake PostHog/network transport or equivalent fail-on-call harness.
- Unit tests proving explicit LLM options remain provider-agnostic and opt-in.
- Committed CLI regression proving corpus-pressure produces only pressure, semantic, and graph artifacts, and does not invoke destination adapters or write Tolaria/Product Brain/destination artifacts.
- CLI integration test for `documents corpus-pressure`.
- Fixture corpus proving graph composition works without private data.
- Real `temp/*.md` smoke run into `/private/tmp`.
- `go test ./...`
- `git diff --check`
- `pb audit WP-29 --phase shaping --verbose` or closest available audit.
