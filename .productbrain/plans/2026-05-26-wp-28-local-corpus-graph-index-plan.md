# WP-28 Local Corpus Graph Index Plan

## Intent

Build the local graph substrate Mindline needs before larger corpus pressure, destination writes, hosted auth, or hosted DB. The implementation must stay local-only, deterministic, provider-agnostic, and evidence-backed.

## Sequence

1. **Graph types**
   - Add corpus graph schema constants and types under `internal/documents`.
   - Define manifest, source, atom, relation, review item, summary, answer key, and metrics structs.

2. **Manifest loading and containment**
   - Load `corpus-graph-manifest/v0.1`.
   - Resolve paths relative to the manifest directory.
   - Reject empty source IDs, duplicate source IDs, unsupported source kinds, path traversal, and symlink escapes.

3. **Semantic artifact loading**
   - Load semantic summaries, candidates, observations, and relations from existing semantic run directories.
   - For WP-28, use existing semantic runs in tests/fixtures or deterministic/local semantic artifacts only; do not introduce provider-specific LLM work or hosted inference.
   - Record skipped or blocked sources instead of crashing when a source has no candidate-producing semantic artifacts.

4. **Atom construction**
   - Convert semantic candidates into graph atoms.
   - Derive stable atom IDs only from allowed durable seed fields: corpus ID, manifest source ID, source label, candidate kind, normalized title/summary, source-native line spans, and excerpt/content hash.
   - Exclude semantic run ID, candidate ID, observation ID, structure node ID, semantic relation ID, output dir, generated artifact path, temp path, timestamp, random value, and provider request ID from ID seeds.
   - Preserve candidate ID, observation IDs, structure node IDs, semantic relation IDs, source document ID, source label, line span, excerpt, confidence, and review status as provenance fields only.
   - Block atoms with missing evidence.

5. **Relation generation**
   - Generate deterministic relation candidates:
     - exact/near-normalized title-summary duplicate heuristics;
     - explicit fixture markers for contradiction/supersession where present;
     - shared normalized keyword/topic overlap for same-topic.
   - Relation IDs derive from corpus ID, relation type, atom IDs, and reason code.
   - Never auto-resolve relations.

6. **Metrics and answer-key evaluation**
   - Compute graph counts, relation counts by type/status, evidence readiness, blocked count, review burden, duplicate clusters, and replay fingerprint.
   - Add optional answer-key evaluation for relation precision/recall in fixtures using the spec formulas for eval-counted relations, true positives, false positives, false negatives, precision, recall, `same_topic_as` inclusion, and review burden ratio.

7. **Writer**
   - Write JSON artifacts under `corpus-graph/`.
   - Write a readable markdown report with counts, KRs, blockers, and next pressure-run readiness.
   - Keep private source excerpts local to graph artifacts only.

8. **CLI**
   - Add:
     `mindline documents corpus-graph <manifest.json> --out <dir>`
   - Print `graph-summary.json` to stdout.

9. **Fixtures**
   - Add `testdata/documents/corpus-graph/connected-corpus/`.
   - Include small Markdown and checked-in semantic artifacts or generated fixture artifacts with seeded duplicate, contradiction, supersession, and same-topic cases.
   - Add an answer key for relation metrics.

10. **Tests**
   - Manifest validation and containment.
   - Deterministic replay over three output dirs.
   - Explicit regression proving graph IDs and replay fingerprint do not change when semantic run IDs, semantic candidate IDs, semantic observation IDs, structure node IDs, generated artifact paths, or output dirs change while source-native evidence and content stay the same.
   - Evidence floor.
   - Relation metrics against answer key.
   - ID safety checks.
   - CLI integration.

11. **Real temp smoke**
   - Build a local manifest for source-like `temp/*.md`.
   - Use existing semantic artifacts or deterministic/local semantic generation only.
   - Skip/block missing semantic artifacts locally; do not call a hosted model/provider.
   - Run corpus graph into `/private/tmp`.
   - Verify no crash, no destination writes, no hosted model/provider calls, and no raw private text in stdout or hosted telemetry.

12. **Review and close**
   - Run `go test ./...`.
   - Run `git diff --check`.
   - Run PB audit/reconciliation.
   - Run LOOP reviewer panel on final implementation and proof.
   - Capture final Chain truth.

## Expected Files

- `internal/documents/corpus_graph.go`
- `internal/documents/corpus_graph_writer.go`
- `internal/documents/corpus_graph_test.go`
- `internal/cli/runner.go`
- `internal/cli/documents_decompose_test.go` or a new CLI-focused test file
- `testdata/documents/corpus-graph/...`
- `.productbrain/specs/2026-05-26-wp-28-local-corpus-graph-index.md`
- `.productbrain/plans/2026-05-26-wp-28-local-corpus-graph-index-plan.md`

## Stop Conditions

Stop and capture a blocker if:

- existing semantic artifacts cannot provide line-level evidence for atoms;
- relation metrics require hidden human labels not represented in an answer key;
- implementation needs hosted storage/auth to pass;
- temp smoke requires committing private artifacts;
- PB audit says WP-28 conflicts with active WP-27/WP-23 authority.
