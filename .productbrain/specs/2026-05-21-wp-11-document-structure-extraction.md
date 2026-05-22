# MINDLINE-WP11-SPEC-V5: Document Structure and Capability Model Extraction

Date: 2026-05-21
Status: Spec for LOOP review
Stop mode: Plan Ready

## Chain Authority

- Product: `PROD-1` Mindline.
- Domain: `DOMAIN-1` says Product Brain is a future destination/authority consumer, not the product; Mindline remains the ingestion engine with source, method, processor, ledger, and destination boundaries.
- Work package: `WP-11` Document structure and capability model extraction.
- Prior work: `WP-10` shipped destination-neutral `document-segments/v0.1`.
- Signed delivery proof: `DEC-28` WP-10 Delivery SIGN-OFF V7.
- Prior insight: `INS-9` says raw Markdown examples need decomposition before Product Brain proposal generation.
- Governing safety standards:
  - `STD-12` private provenance visibility is authoritative for publish blocking.
  - `STD-13` dry-run artifacts must reject symlinked `--out` parents and disambiguate colliding output.
  - `STD-16` proposal adapters must fail closed and remain bundle-contained.

## Problem

WP-10 safely decomposes Markdown into destination-neutral segments, but process/capability documents still become flat line fragments. A representative real process/capability document produced 89 local segments, mostly `source_note`, with no durable representation of the document's parent-child structure.

This prevents downstream review and destination adapters from understanding the document as a process/capability model. Sending flat segments directly to Product Brain, Tolaria, Notion, Obsidian, or any future destination would force each adapter to rediscover structure independently and would risk making one destination the hidden schema owner.

## Product Model Fit Proof

Eligibility path: `EXTEND`.

WP-11 extends the WP-10 `DocumentSegment` contract with a new destination-neutral `DocumentStructure` layer. It does not replace segments and does not implement any destination mapping.

Durable user job:
- Turn a structured Markdown document into an inspectable local model that preserves hierarchy, typed nodes, and provenance before review or destination evaluation.

Product object:
- `DocumentStructure`: a local artifact set that groups one or more `DocumentSegment` artifacts into typed structural nodes and relationships.

Allowed states:
- `ready`: structure node is confidently derived from source structure.
- `needs_review`: structure node or relationship is plausible but ambiguous.
- `blocked`: unsafe/private marker or provenance issue prevents writing readable content.
- `skipped`: source material is intentionally omitted from structure extraction.

Why this is not bespoke:
- Process docs, capability maps, strategy docs, operating models, and requirements matrices all need hierarchy preservation before downstream routing.
- The output is local and destination-neutral, so Product Brain is one future consumer rather than the schema owner.

## Impact Pack

### Use Cases

Primary happy path:
- A Markdown process/capability document is first decomposed with WP-10, then converted into `document-structure/v0.1` artifacts that preserve section, table, row, and audience hierarchy.

Alternate paths:
- A mostly prose document produces section/prose nodes with lower structural confidence.
- A malformed table produces `needs_review` nodes rather than guessed hierarchy.
- Unsafe marker content is blocked/redacted before any structure JSON or preview is written.
- An empty or unsupported document fails with a clear process error and no partial structure writes.

### Upstream and Downstream

Upstream:
- Markdown files.
- WP-10 `document-segments/v0.1` artifacts.

Downstream:
- Future review/apply queue.
- Future destination adapter evaluation for Product Brain, Tolaria, Notion, Obsidian, and local file outputs.

Out of scope downstream:
- No live writes.
- No Product Brain proposal artifacts.
- No destination-specific fields or collection names.

### Architecture Contract

Artifact root:

```text
<out>/document-structure/
  structure-summary.json
  nodes/<structure_node_id>.json
  previews/<structure_node_id>.md
```

The `document-structure` writer must be local-only, deterministic, and at least as strict as the WP-10 writer for:
- traversal rejection
- symlinked root/parent/generated-path rejection
- stale unexpected generated file rejection
- duplicate ID rejection
- unsafe marker redaction
- source ID and heading/provenance redaction
- summary rebuilt from finalized nodes before write

### Regression Surface

What can break:
- WP-10 document segment output.
- CLI dry-run behavior.
- output containment and privacy guarantees.
- deterministic golden fixtures.
- downstream adapter boundary assumptions.

Detection:
- focused `internal/documents` tests.
- CLI tests for structure command behavior.
- golden expected output comparison.
- static boundary scans for destination names and live integration surfaces.
- generated-output scans for unsafe/private marker leakage.
- `go test -count=1 ./...`.

## Required Contract

### CLI

Add a local command:

```text
mindline documents structure <markdown-path-or-dir> --out <dir>
```

The command may internally run WP-10 decomposition first, or may share parsing helpers, but the persisted structure artifacts must be separate from `document-segments`.

Required behavior:
- accepts one Markdown file or a directory of Markdown files
- writes only under explicit `--out`
- prints structure summary JSON to stdout
- returns existing artifact/write/process exit-code classes consistently with current CLI patterns
- rejects Product Brain profile/proposal flags for this command path

### Summary Schema

`structure-summary.json` must include:

- `schema_version`: `document-structure-summary/v0.1`
- `run_id`
- `source_count`
- `node_count`
- `needs_review_count`
- `blocked_count`
- `node_type_counts`
- `root_node_ids`
- `nodes[]` with:
  - `node_id`
  - `source_document_id`
  - `node_type`
  - `review_status`
  - `confidence`
  - `node_path`
  - `preview_path`

### Node Schema

Each node JSON must include:

- `schema_version`: `document-structure-node/v0.1`
- `node_id`
- `run_id`
- `source_document_id`
- `node_type`
- `review_status`
- `confidence`
- `title`
- `summary`
- `parent_node_id`
- `child_node_ids`
- `related_segment_ids`
- `evidence`
- `blockers`

Node types:
- `document`
- `section`
- `table`
- `table_row`
- `capability`
- `audience`
- `workflow`
- `requirement`
- `unknown`

Evidence must include:
- source kind
- source document ID
- heading path
- line start/end
- content hash
- related segment IDs

### Structure Extraction Rules

The implementation must derive:
- document root node per source document
- section nodes from Markdown heading hierarchy
- table nodes for Markdown table blocks
- table row nodes under the table node
- capability-like nodes from stable synthetic fixture patterns, such as identifier-like prefixes, bold capability titles, and repeated table/list rows
- audience-like nodes from heading/list context in sanitized fixtures
- parent-child relationships from Markdown structure, not destination mapping

Ambiguous or malformed structure must become `needs_review` or `unknown`, not a confident guess.

### Determinism Rules

Directory input traversal must be lexical and stable by normalized relative path.

Generated output must use deterministic ordering:
- `root_node_ids` sorted by source document order, then node path
- summary `nodes[]` sorted by source document order, node path, node type, and node ID
- each `child_node_ids` list sorted by source order and node path
- each `related_segment_ids` list sorted by source order and segment order

Generated node IDs must be derived from stable source document identity, node type, normalized structural path, source line range, and content hash. IDs must not depend on map iteration, filesystem discovery order, wall-clock time, random values, absolute local paths, or Product Brain/Chain IDs.

Generated `run_id` values must be derived only from normalized input identity/order and source content hashes. `run_id` must not depend on wall-clock time, random values, absolute local paths, filesystem discovery order, or Product Brain/Chain IDs.

### Safety Rules

The structure writer is the final safety boundary.

It must finalize/redact nodes before summary construction, validation, expected file calculation, JSON writes, and preview writes.

Unsafe/private marker detection must cover:
- title
- summary
- node path
- heading path
- source document ID
- related segment IDs when derived from unsafe source IDs
- caller-supplied nodes
- prebuilt summaries

Summary `node_path` values must be built only from finalized/redacted node fields after title and heading redaction, and before expected file calculation, summary validation, JSON writes, and preview writes.

Raw private documents and recognizable labels from real user examples must not be committed. Fixtures must be synthetic but structurally representative.

### Destination-Neutral Rules

Core structure artifacts must not contain:
- Product Brain proposal fields
- Notion/Obsidian/Tolaria destination hints
- Product Brain profile references
- destination collection names
- live API/auth/network/browser/provider references
- Chain authority IDs or governance metadata inside persisted output artifacts

Chain authority may govern the implementation work package, tests, and spec/plan artifacts, but it must not be serialized into `document-structure/v0.1` summary, node, or preview outputs.

## Acceptance Criteria

1. `mindline documents structure <path-or-dir> --out <dir>` writes `document-structure/` artifacts and prints a deterministic summary JSON.
2. Sanitized fixtures cover at least:
   - process/capability table
   - audience-specific capability list
   - mixed headings, bullets, prose
   - malformed or ambiguous structure requiring review
   - unsafe/private marker redaction
3. Golden tests compare `structure-summary.json`, node JSON, and previews.
4. Structure nodes preserve parent-child relationships and link back to WP-10 segment IDs/source line ranges.
5. No Product Brain/Tolaria/Notion/Obsidian destination hints appear in structure output.
6. No live network/auth/browser/provider/destination code is introduced in `internal/documents`.
7. Structure writer rejects traversal, symlink escape, stale unexpected generated files, duplicate IDs, and unsafe prebuilt summaries.
8. Raw private temp files remain ignored and uncommitted.
9. `go test -count=1 ./...` passes.

## Required Verification Commands

The implementation PR must run and report these commands from the repository root:

```bash
go test -count=1 ./...
go run ./cmd/mindline documents structure testdata/documents/structure --out /tmp/mindline-wp11-structure-verify
diff -ru testdata/documents/expected/structure /tmp/mindline-wp11-structure-verify/document-structure
rg -n "Product Brain|Tolaria|Notion|Obsidian|proposal|profile|collection" /tmp/mindline-wp11-structure-verify/document-structure
rg -n "https?://|\\b(api|auth|token|browser|provider|daemon|llm|slack)\\b" internal/documents
rg -n "UNSAFE|PRIVATE|SECRET|REDACT_ME|authority_ids|authority_id" /tmp/mindline-wp11-structure-verify/document-structure
git status --short --ignored temp
```

Expected verification results:
- The first three commands pass.
- The destination-hint scan finds no matches in generated structure output.
- The live-integration scan finds no matches in `internal/documents`, except comments or test fixture names explicitly proving absence of those integrations.
- The unsafe/private/governance scan finds no matches in generated structure output.
- The `git status` check shows raw private temp files remain untracked or ignored and are not staged.

## Exclusions

- No Product Brain proposal generation.
- No live Product Brain writes.
- No Tolaria, Notion, Obsidian, Slack, browser, network, auth, provider, daemon, UI, or LLM integration.
- No raw private document content or recognizable private labels in committed fixtures/spec examples.
- No review/apply queue.
- No destination adapter mapping from `DocumentStructure` to Product Brain entries.
- No attempt to solve ontology or semantic classification beyond deterministic structural extraction.

## Follow-Up

Likely next package:
- `WP-12`: local review/apply contract for `DocumentSegment` and `DocumentStructure` artifacts.
