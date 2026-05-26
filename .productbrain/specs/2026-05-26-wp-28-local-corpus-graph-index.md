# WP-28 Local Corpus Graph Index

## Status

Signed implementation target for the next PR after PR #20 / WP-23. This creates the local connected-corpus substrate required before 50-file pressure, Tolaria draft writes, hosted auth, or any persistent hosted database.

## Authority

- `DEC-153`: add a local read-only corpus graph/index before hosted auth or hosted DB.
- `DEC-154`: hosted auth/DB remains blocked until privacy/security gates pass; duplicate and contradicting atoms remain reviewable evidence-backed relation candidates, not auto-resolved.
- `STR-3`: autonomy-readiness before destination writes.
- `DEC-64`: no-human semantic ingestion requires held-out accuracy >=98%.
- `PRI-1` and `BR-1`: privacy-by-design and traceability-by-design; hosted observability is metadata-only.
- `STD-17` and `ARCH-1`: provider-agnostic measurement before trust.
- `WP-23`: canonical autonomy-readiness report and local eval control plane.
- `TEN-4`: future writes require durable source-object identity and cross-run replay proof.
- `INS-12` / `INS-13`: real temp Markdown produces grounded candidates, while review artifacts may be intentionally skipped.

## Problem

Mindline can process one Markdown source into structure, semantic candidates, judgment artifacts, traces, and readiness reports. That is not enough for a second-brain ingestion system. Real value appears when many files are connected: the same atom appears in multiple places, older notes get superseded, claims conflict, and a user needs to understand what the corpus says without manually reconciling every file.

The system also cannot safely jump to hosted auth/DB or Tolaria writes. We first need a local rebuildable graph that proves identity, evidence, relation candidates, duplicate detection, contradiction/supersession surfacing, and aggregate eval metrics.

## Outcome

Mindline can build a local, read-only corpus graph from a manifest of Markdown files and existing semantic artifacts. The graph turns multi-file source evidence into reviewable atoms and relation candidates, then writes aggregate metrics that explain:

- what atoms were extracted;
- which atoms may be duplicates;
- which atoms may contradict or supersede one another;
- which atoms share a topic;
- which relation candidates are reviewable and why;
- how much evidence is ready;
- how much review burden remains;
- whether the graph is deterministic across reruns.

The graph is local-only and rebuildable. It does not write to Tolaria, Product Brain, a hosted DB, or a hosted eval store.

## Scope

Implement:

1. `mindline documents corpus-graph <manifest.json> --out <dir>`
2. A manifest contract that points to source Markdown files and/or semantic run directories.
3. A local graph artifact directory:
   - `corpus-graph/graph-summary.json`
   - `corpus-graph/atoms/<atom-id>.json`
   - `corpus-graph/relations/<relation-id>.json`
   - `corpus-graph/review-items/<relation-id>.json`
   - `corpus-graph/graph-report.md`
4. Deterministic source IDs, atom IDs, and relation IDs derived from source-native identity and content/evidence, never from run IDs or output paths.
5. Evidence-backed atoms with source document ID, source path label, line span, excerpt, candidate IDs, observation IDs, confidence, and review status.
6. Relation candidates with closed relation types:
   - `possible_duplicate`
   - `contradicts`
   - `supersedes`
   - `same_topic_as`
7. Relation review state:
   - `ready`
   - `needs_review`
   - `blocked`
8. Aggregate graph metrics:
   - source count
   - semantic run count
   - atom count
   - relation count by type
   - relation count by review status
   - evidence-ready atom count
   - evidence-ready relation count
   - blocked count
   - duplicate cluster count
   - review burden count and ratio
   - deterministic replay fingerprint
9. Tests proving deterministic replay, evidence enforcement, relation generation, and local-only behavior.

## Relation Semantics

All relation outputs are candidates. WP-28 must never auto-merge, auto-delete, auto-resolve, auto-commit, or silently prefer one atom over another.

- `possible_duplicate`: two atoms have materially similar normalized title/summary/evidence meaning and should be reviewed as possible duplicate knowledge.
- `contradicts`: two atoms assert incompatible facts or states about the same subject. Deterministic detection may start with explicit contradiction markers in the fixture corpus; LLM-assisted contradiction detection is out of scope unless provider-agnostic and optional.
- `supersedes`: one atom appears to replace or update another. Deterministic detection may start with explicit supersession markers in the fixture corpus or newer/older source metadata.
- `same_topic_as`: two atoms share a topic or subject but are not necessarily duplicates or conflicts.

Relation candidates must include:

- relation ID
- relation type
- source atom ID
- target atom ID
- confidence
- review status
- reason code from a closed vocabulary
- evidence references for both sides
- no raw hosted telemetry payload

## Durable Identity Contract

Graph IDs must be stable across semantic reruns. Existing semantic artifact IDs are useful provenance, but they are not durable graph identity because current semantic candidate, observation, relation, and structure node IDs may include run-scoped identity.

Allowed graph ID seed fields:

- `corpus_id`
- manifest `source_id`
- source document/path label from the manifest
- candidate kind
- normalized title
- normalized summary
- source-native line spans
- excerpt/content hash
- relation type
- relation reason code
- participating atom IDs for relation IDs

Forbidden graph ID seed fields:

- semantic `run_id`
- semantic candidate IDs
- semantic observation IDs
- semantic relation IDs
- structure node IDs
- output directories
- generated artifact paths
- temp paths
- timestamps
- random values
- provider request IDs

Graph artifacts may preserve semantic candidate IDs, observation IDs, structure node IDs, and semantic relation IDs as provenance fields. The graph replay fingerprint must use durable graph IDs, counts, relation types, review states, evidence coordinates, and content hashes, not run-scoped semantic artifact IDs.

## Metric Formulas

Relation answer-key metrics are computed only for relation types represented in the answer key.

- `eval_counted_relation`: a relation candidate with `review_status=ready`, evidence-ready source and target atoms, and a relation type included in the answer key.
- `true_positive`: an eval-counted relation whose unordered atom pair and relation type match the answer key.
- `false_positive`: an eval-counted relation whose unordered atom pair and relation type are absent from the answer key.
- `false_negative`: an answer-key relation whose unordered atom pair and relation type has no matching eval-counted relation.
- `precision = true_positive / (true_positive + false_positive)`.
- `recall = true_positive / (true_positive + false_negative)`.
- `same_topic_as` participates in precision and recall only when the answer key includes `same_topic_as`; otherwise it is reported as review context but excluded from seeded recall.
- Blocked and `needs_review` relation candidates do not count toward precision/recall, but they do count toward review burden.
- `review_burden_ratio = relation_candidates_with_review_status_needs_review_or_blocked / total_relation_candidates`.

## Manifest Contract

The manifest is JSON and local-only:

```json
{
  "schema_version": "corpus-graph-manifest/v0.1",
  "corpus_id": "local-eval-corpus",
  "sources": [
    {
      "source_id": "source-native-id",
      "source_kind": "markdown",
      "path": "relative/path.md",
      "semantic_run_dir": "optional/semantic/run"
    }
  ]
}
```

Requirements:

- Paths are resolved relative to the manifest directory unless absolute paths are explicitly allowed by CLI input validation.
- Paths must not escape the manifest root by `..` or symlink traversal.
- Source IDs are required and stable.
- The graph builder may consume existing semantic run dirs or deterministic/local semantic artifacts only. If semantic artifacts are missing, the default behavior is to skip or block the source locally with an explicit reason. WP-28 must not call a hosted model/provider during corpus graph generation.
- Private source text is local-only.

## KRs

Aggressive delivery KRs:

1. **Deterministic graph:** the same manifest run three times produces identical source IDs, atom IDs, relation IDs, aggregate counts, and replay fingerprint.
2. **Evidence floor:** 100% of counted atoms and relation candidates include source file label, source document ID, line span, and excerpt. Evidence-less items are blocked and excluded from readiness counts.
3. **No run-scoped identity:** durable graph IDs contain no run ID, output path, temp path, timestamp, or random component.
4. **Connected-corpus relation coverage:** seeded fixture corpus emits at least one `possible_duplicate`, one `contradicts`, one `supersedes`, and one `same_topic_as` relation candidate.
5. **Relation precision gate:** fixture relation precision is >=95% against the answer key for eval-counted relation candidates.
6. **Seeded recall gate:** seeded duplicate/contradiction/supersession recall is >=90% against the answer key.
7. **Review burden ceiling:** review burden ratio is <=20% for high-confidence deterministic relations in the fixture corpus; uncertain relations remain reviewable instead of being auto-resolved.
8. **Privacy/locality:** 0 Product Brain writes, 0 Tolaria writes, 0 hosted DB writes, 0 hosted raw-text telemetry events.
9. **Scale preflight:** current `temp/*.md` source-like files can be represented as a graph without crash; intentionally skipped review artifacts are reported as skipped, not failures.
10. **50-file readiness signal:** report states whether the graph is ready for a later 50-file read-only pressure run and lists blockers if not.

## Anti-Goals

- No hosted auth/login.
- No hosted database.
- No Tolaria writes.
- No Product Brain writes.
- No destination proposal apply path.
- No auto-merge or auto-resolution.
- No hidden permanent local database.
- No provider-specific LLM dependency.
- No hosted inference over private source text.
- No tuning to private `temp/` filenames or content.
- No no-human readiness claim.

## Verification

Required before PR:

- Unit tests for manifest containment and schema validation.
- Unit tests for deterministic source, atom, relation IDs and replay fingerprint.
- Unit tests for relation candidate generation and answer-key metrics.
- Unit tests that evidence-less atoms/relations are blocked and not counted as evidence-ready.
- Unit tests proving no run IDs/output paths/timestamps enter durable IDs.
- CLI integration test for `documents corpus-graph`.
- Fixture corpus with seeded duplicate, contradiction, supersession, and same-topic relations.
- Three-run deterministic replay comparison.
- Real `temp/*.md` smoke run.
- Verification that corpus graph and temp smoke execute without hosted model/provider calls, and without raw private text in stdout, hosted telemetry, Product Brain, Tolaria, or destination writes.
- `go test ./...`
- `git diff --check`
- `pb audit WP-28` or closest available PB audit command.
