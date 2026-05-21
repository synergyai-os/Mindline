# WP-10 Markdown Document Decomposition and Destination-Neutral Proposal Evaluation Spec

Spec version: `MINDLINE-WP10-SPEC-V1`
Date: 2026-05-21
Status: Draft for LOOP Spec sign-off.

## Product Brain Authority

- Product: `PROD-1` - Mindline.
- Product direction: `DOMAIN-1` - Product Brain is a destination/authority consumer, not the Mindline product.
- Signed shape: `MINDLINE-WP10-SHAPE-V2`.
- Prior deliveries: `WP-8` local run ledger/review queue and `WP-9` Product Brain workspace profile/proposal adapter.
- Evidence: `INS-9` - raw Markdown examples need decomposition before Product Brain proposal generation.
- Method boundary: `DEC-15` - core is method-neutral; BASB/PARA/CODE is a profile, not core architecture.
- Destination boundary: `WP-5`, `DEC-12`, `DEC-13` - destinations consume dry-run/proposal contracts and do not own source classification.
- Safety and containment standards: `STD-12`, `STD-13`, `STD-14`, `STD-15`, `STD-16`.

## Problem

WP-9 safely turns review queue items into Product Brain proposals, but real Markdown exports are not already-curated review items. The local examples in `temp/` contain mixed source material: transcripts, Slack-like threads, Notion strategy docs, capability lists, will/won't-do tables, risks, actions, decisions, and references.

When each Markdown file is treated as one review item, Mindline either skips the whole file or creates one broad destination proposal. That preserves safety, but it loses the actual knowledge structure. If the fix is built inside the Product Brain adapter, Mindline becomes Product Brain-shaped and breaks its own architecture.

## Product Model Fit

Eligibility verdict: `EXTEND`.

WP-10 extends the canonical Mindline pipeline:

1. source adapters normalize captures with provenance;
2. processors and method profiles enrich and classify without live actions;
3. run ledger and review queue preserve inspectable local state;
4. destination/proposal adapters consume safe review artifacts.

The reusable product object is `DocumentSegment`, a destination-neutral representation of a smaller knowledge candidate extracted from a larger document. Product Brain, Notion, Obsidian, Tolaria, local-folder, and future API adapters may map `DocumentSegment` differently, but none of them define the core segment schema.

## Scope

In scope:

- a versioned `document-segments/v0.1` artifact contract;
- a local Markdown decomposition command or pipeline mode that reads Markdown files and writes local output under explicit `--out`;
- deterministic segmentation for headings, tables, bullet lists, transcript turns, and mixed prose sections;
- an explicit semantic type set independent of any destination collection names;
- confidence and review status assignment;
- provenance metadata by source document ID and location range, not raw private source text;
- sanitized fixture inputs and expected outputs for transcript, mixed thread, and Notion/strategy patterns;
- local evaluation report showing segment counts, type counts, review counts, and downstream Product Brain proposal interpretation;
- optional downstream Product Brain proposal evaluation from segments using WP-9, while preserving Product Brain as a consumer.

Out of scope:

- live writes to Product Brain, Notion, Obsidian, Tolaria, Slack, browser, or any network/API;
- LLM extraction/classification;
- chain-aware duplicate detection against live Product Brain;
- applying or approving proposals;
- changing Product Brain kernel behavior;
- hardcoding the current Product Brain workspace collections as core types;
- committing raw files from `temp/` or raw-derived artifacts;
- perfect semantic extraction from every sentence.

## Raw Input and Fixture Safety

Raw files in `temp/` are private local examples. The repository must ignore `temp/`. Implementation must not stage or commit `temp/` or any raw-derived output.

Committed fixtures must be sanitized and intentionally authored under:

```text
testdata/documents/markdown/
```

Expected golden outputs must be sanitized and intentionally authored under:

```text
testdata/documents/expected/
```

Fixture rules:

- use synthetic role labels such as `Speaker A`, `Product Lead`, or `Support Lead`;
- no personal identifiers;
- no full local filesystem paths;
- no private Slack/thread content;
- no raw URLs except documented synthetic examples in test fixtures;
- no long raw transcript excerpts copied from private examples;
- provenance fixture data uses stable document IDs and locations, not private source text.

Verification must include:

```bash
rg "Randy|Nina|Barbara|Lucas|Geert|Klaas|Morgana|ZDHC|Saprolab|/Users/|Young Human Club|slack|http://|https://" testdata/documents internal/documents
```

Expected result: no matches unless the match is an explicit allowlisted synthetic string documented in the test.

## Document Segment Contract

The first implementation slice writes:

```text
<out>/document-segments/segment-summary.json
<out>/document-segments/segments/<segment_id>.json
<out>/document-segments/previews/<segment_id>.md
```

`segment-summary.json`:

```json
{
  "schema_version": "document-segment-summary/v0.1",
  "run_id": "run-doc-0123456789abcdef",
  "source_count": 3,
  "segment_count": 12,
  "needs_review_count": 4,
  "type_counts": {
    "decision": 2,
    "action": 3,
    "reference": 4,
    "unknown": 3
  },
  "segments": [
    {
      "segment_id": "seg-0123456789abcdef",
      "source_document_id": "doc-transcript-demo",
      "semantic_type": "action",
      "review_status": "ready",
      "confidence": "medium",
      "segment_path": "segments/seg-0123456789abcdef.json",
      "preview_path": "previews/seg-0123456789abcdef.md"
    }
  ],
  "authority_ids": ["PROD-1", "DOMAIN-1", "DEC-15", "WP-8", "WP-9", "WP-10"]
}
```

Segment item:

```json
{
  "schema_version": "document-segment/v0.1",
  "segment_id": "seg-0123456789abcdef",
  "run_id": "run-doc-0123456789abcdef",
  "source_document_id": "doc-transcript-demo",
  "source_kind": "markdown",
  "semantic_type": "action",
  "review_status": "ready",
  "confidence": "medium",
  "title": "Prepare support handover session",
  "summary": "Support Lead should schedule a handover session for second-line support topics.",
  "evidence": {
    "kind": "location",
    "heading_path": ["Support readiness"],
    "line_start": 42,
    "line_end": 47,
    "content_hash": "sha256:<hex>"
  },
  "blockers": [],
  "authority_ids": ["PROD-1", "DOMAIN-1", "DEC-15", "WP-8", "WP-9", "WP-10"]
}
```

## Allowed Semantic Types

The core semantic type set is fixed for WP-10:

- `source_note` - raw or lightly processed source context worth retaining;
- `meeting_note` - meeting-level summary/context;
- `decision` - a settled choice or decision candidate;
- `tension` - unresolved risk, blocker, contradiction, or concern;
- `action` - task or next action not yet confirmed as a commitment;
- `commitment` - explicit owner-backed commitment;
- `standard` - reusable rule, convention, or operating principle;
- `insight` - reusable learning or pattern;
- `work_item` - bounded future work or project package candidate;
- `reference` - structured reference material such as capabilities, facts, timelines, or lists;
- `unknown` - segment requires review before durable classification.

These are semantic labels, not Product Brain collection names. Destination adapters may map them to collections, pages, files, folders, databases, properties, or no output.

## Review Status and Confidence

Allowed `review_status` values:

- `ready` - deterministic rules found enough structure for downstream proposal evaluation;
- `needs_review` - plausible segment, but type, title, summary, or safety classification is uncertain;
- `blocked` - unsafe, unsupported, or incomplete segment that must not flow to downstream proposals;
- `skipped` - not useful as durable knowledge.

Allowed `confidence` values:

- `high` - strong structural signal and required fields are populated;
- `medium` - sufficient structure but human review may improve type or wording;
- `low` - weak signal; default review status should be `needs_review` or `blocked`.

Rules:

- Any segment with `semantic_type=unknown` must have `review_status=needs_review`.
- Any missing title or summary must become `needs_review` unless the segment is `skipped`.
- Any unsafe/private content marker must become `blocked`.
- A segment can only become `ready` when provenance, title, summary, semantic type, confidence, and safety checks all pass.

## Deterministic Decomposition Rules

The first implementation slice is deterministic and local-only:

- headings create section boundaries;
- Markdown tables create one candidate per meaningful row, plus an optional table-level `reference` segment;
- transcript timestamps or speaker turns create turn groups;
- consecutive bullets under a heading create list item candidates;
- prose sections create one `source_note`, `meeting_note`, `reference`, or `unknown` segment depending on structural signals;
- duplicate segment IDs within a run are refused before writing artifacts.

No LLM calls are allowed. If deterministic rules cannot classify confidently, emit `unknown` / `needs_review`.

## Destination Adapter Mapping

Core document segments must not contain destination-specific hints, surfaces, collection names, database names, folder paths, Product Brain intents, Notion properties, Obsidian paths, Tolaria locations, or write instructions.

Adapters map `semantic_type`, `review_status`, `confidence`, provenance, and summary fields downstream. For Product Brain evaluation, the Product Brain adapter may translate semantic types to WP-9 intents in a separate Product Brain proposal artifact:

- `decision` -> `durable_decision`
- `standard` -> `operating_standard`
- `tension` -> `open_tension`
- `work_item` or `action` -> `implementation_work`
- `insight` -> `reusable_insight`
- `reference`, `meeting_note`, `source_note` -> `reference_note`
- `unknown`, `blocked`, `skipped` -> no ready Product Brain proposal

The core must not import `internal/productbrain` to perform decomposition. Any Product Brain evaluation must be a separate downstream step after document segments exist.

Destination-specific mapping artifacts, if any, are out of the core `document-segment/v0.1` schema. They must be generated only by explicit downstream adapter commands.

## CLI / Local Evaluation

The implementation should add a local command or pipeline mode with this shape:

```bash
mindline documents decompose <markdown-path-or-dir> --out <dir>
```

If Product Brain evaluation is included in the same PR, it must be explicit and downstream, for example:

```bash
mindline documents decompose <path> --out <dir>
mindline product-brain propose <segment-run-dir> --profile <profile.json> --out <dir>
```

The default command must not read Product Brain profiles or emit Product Brain proposals.

## Acceptance Criteria

1. `temp/` is ignored by Git and no plan step stages or commits it.
2. Sanitized fixture inputs exist under `testdata/documents/markdown/` with these exact files:
   - `transcript-decision-action.md`
   - `mixed-thread-capture.md`
   - `strategy-capability-table.md`
3. Expected fixture outputs exist under `testdata/documents/expected/` with these exact golden trees:
   - `transcript-decision-action/document-segments/`
   - `mixed-thread-capture/document-segments/`
   - `strategy-capability-table/document-segments/`
4. Golden outputs prove representative decomposition counts:
   - `transcript-decision-action.md` emits at least 6 segments, including at least 1 `decision`, 2 `action` or `commitment`, 1 `tension`, and 1 `meeting_note`.
   - `mixed-thread-capture.md` emits at least 7 segments, including at least 1 `source_note`, 1 `reference`, 1 `action`, 1 `insight`, and 1 `unknown` / `needs_review`.
   - `strategy-capability-table.md` emits at least 7 segments, including at least 2 `standard` or `decision`, 2 `reference`, 1 `work_item`, and one table-level `reference` segment.
5. A golden fixture test named `TestGoldenMarkdownDecomposition` compares normalized generated `segment-summary.json`, per-segment JSON, and preview Markdown against `testdata/documents/expected/`.
6. A generated artifact tree test named `TestDocumentsDecomposeWritesCompleteArtifactTree` proves every summary segment has a matching segment JSON and preview file, and every generated file is referenced or intentionally allowlisted.
7. The segment contract is versioned and rejects unsupported schema versions.
8. Segment IDs and paths are deterministic, path-safe, and collision-checked by named tests.
9. Provenance uses stable document ID plus location/range/hash metadata and does not require private source text.
10. Allowed semantic types, review statuses, and confidence values are enforced.
11. Unknown or low-confidence segments fail closed into `needs_review`, `blocked`, or `skipped`.
12. Core `document-segment/v0.1` artifacts contain no destination-specific hint fields or destination-specific values such as `productbrain`, `notion`, `obsidian`, or `tolaria`.
13. The default document decomposition path does not import or call Product Brain adapter code.
14. Product Brain evaluation, if implemented, consumes segments downstream and remains optional/explicit.
15. Generated output is contained under explicit `--out` and rejects traversal and symlink escape patterns by named tests.
16. No-leak scan over committed fixtures and generated output directories returns no private/raw/local path matches.
17. No-live scan over WP-10 code surfaces returns no Product Brain live writes, Notion/Obsidian/Tolaria/Slack/network/browser/auth/exec calls.
18. Import/call boundary tests prove `internal/documents` does not import `internal/productbrain` and `mindline documents decompose` does not read Product Brain profiles or invoke proposal code.
19. `go test -count=1 ./...` passes.

## Required Verification Commands

```bash
go test -count=1 ./...
go test ./internal/documents -count=1
go test ./internal/cli -run TestDocumentsDecompose -count=1
go test ./internal/documents -run 'TestGoldenMarkdownDecomposition|TestDocumentsDecomposeWritesCompleteArtifactTree|TestSegmentID|TestSegmentPath|TestDuplicateSegmentID|TestWriterRejectsTraversal|TestWriterRejectsSymlink|TestUnsupportedSchema|TestReviewStatusConfidence|TestDocumentSegmentHasNoDestinationHints|TestDocumentsPackageDoesNotImportProductBrain' -count=1
go test ./internal/cli -run 'TestDocumentsDecomposeDoesNotReadProductBrainProfile|TestDocumentsDecomposeDoesNotEmitProductBrainProposals' -count=1
go run ./cmd/mindline documents decompose testdata/documents/markdown --out /tmp/mindline-wp10-documents
go test ./internal/documents -run TestGeneratedOutputMatchesGoldenFixtures -count=1
rg "Randy|Nina|Barbara|Lucas|Geert|Klaas|Morgana|ZDHC|Saprolab|/Users/|Young Human Club|slack|http://|https://" testdata/documents internal/documents /tmp/mindline-wp10-documents
rg "destination_hints|surface|productbrain|notion|obsidian|tolaria" /tmp/mindline-wp10-documents/document-segments
rg "internal/productbrain|productbrain\\.|product-brain|Product-OS|convex|pb profile|pb capture|notion|obsidian|tolaria|slack|browser|auth|oauth|net/http|os/exec|exec.Command|http://|https://" internal/documents
rg "Product-OS|convex|pb profile|pb capture|notion|obsidian|tolaria|slack|browser|auth|oauth|net/http|os/exec|exec.Command|http://|https://" internal/cli
```

Expected:

- tests pass;
- no-leak scan returns no matches except explicitly documented synthetic allowlist strings;
- no-live scan returns no matches in WP-10 implementation surfaces. The `internal/cli` scan may match existing pre-WP-10 Product Brain command routing only if the match is explicitly reviewed as outside `documents decompose`; the plan must include a command-path test proving the document command does not call Product Brain code.

The plan must include the named tests above. If implementation uses different test names, the plan must preserve the same proof coverage explicitly and update the required verification command before delivery starts.

## Exclusions and Follow-Up

Follow-up work, not WP-10:

- live Product Brain apply/review workflow;
- live duplicate detection against the Chain;
- LLM-assisted extraction;
- Notion/Obsidian/Tolaria-specific proposal adapters;
- UI review surface;
- background ingestion daemon;
- live Slack or document provider connectors.

## Plan Ready Stop

This LOOP run stops once:

1. this spec is signed off by the selected LOOP panel;
2. the signed spec is captured or linked in Product Brain;
3. the durable WP is materialized from the signed spec;
4. audit/gate reconciliation has no blocking failure;
5. the implementation plan is signed off and persisted.

No implementation starts in this LOOP run.
