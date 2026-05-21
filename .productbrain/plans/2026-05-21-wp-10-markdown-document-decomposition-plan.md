# WP-10 Markdown Document Decomposition and Destination-Neutral Proposal Evaluation Plan

Plan version: `MINDLINE-WP10-PLAN-V1`
Date: 2026-05-21
Status: Draft for LOOP Plan sign-off.

## Authority

- Work package: `WP-10` - Markdown document decomposition and destination-neutral proposal evaluation.
- Signed spec: `MINDLINE-WP10-SPEC-V1`.
- Spec sign-off decision: `DEC-26`.
- Evidence: `INS-9` - raw Markdown examples need decomposition before Product Brain proposal generation.
- Product boundary: `PROD-1`, `DOMAIN-1`.
- Architecture guardrail: Product Brain is a downstream destination/authority consumer, not the core schema owner.

## Delivery Contract

Build only the local deterministic document decomposition slice:

- sanitized Markdown fixtures and expected golden outputs;
- versioned `document-segment-summary/v0.1` and `document-segment/v0.1` structs and validation;
- deterministic Markdown segmenter for headings, tables, bullets, transcript turns, and prose;
- local artifact writer under explicit `--out`;
- `mindline documents decompose <markdown-path-or-dir> --out <dir>`;
- verification proving no raw temp leak, no live/provider behavior, no destination-specific core fields, and no Product Brain coupling in the default document command.

Do not implement Product Brain apply/review, Notion/Obsidian/Tolaria adapters, LLM extraction, live provider reads, auth, network calls, or background ingestion.

## Phase 0 - Preflight

1. Confirm `pb profile list` reports `activeSource: local` and `active: randy-s-pkm` before any PB updates.
2. Confirm `git status --short --branch` and keep unrelated changes untouched.
3. Confirm `temp/` is ignored and not staged:

```bash
git check-ignore temp
git status --short --ignored temp
```

4. Read the signed spec before editing:

```bash
sed -n '1,340p' .productbrain/specs/2026-05-21-wp-10-markdown-document-decomposition.md
```

## Phase 1 - TDD Fixture Pack

Write tests first for fixture presence, no-leak rules, and golden shape.

Create sanitized authored inputs:

```text
testdata/documents/markdown/transcript-decision-action.md
testdata/documents/markdown/mixed-thread-capture.md
testdata/documents/markdown/strategy-capability-table.md
```

Create expected golden trees:

```text
testdata/documents/expected/transcript-decision-action/document-segments/
testdata/documents/expected/mixed-thread-capture/document-segments/
testdata/documents/expected/strategy-capability-table/document-segments/
```

Required tests:

- `TestGoldenMarkdownDecomposition`
- `TestGeneratedOutputMatchesGoldenFixtures`
- `TestDocumentsDecomposeWritesCompleteArtifactTree`

The fixture tests must assert the minimum segment/type counts from the spec. Golden comparison should normalize fields that are intentionally run-specific before comparing.

## Phase 2 - Segment Contract

Write validation tests before implementation:

- `TestUnsupportedSchema`
- `TestReviewStatusConfidence`
- `TestUnsafePrivateContentMarkerBlocksSegment`
- `TestDocumentSegmentHasNoDestinationHints`
- `TestDocumentsPackageDoesNotImportProductBrain`
- `TestSegmentID`
- `TestSegmentPath`
- `TestDuplicateSegmentID`

Implement an `internal/documents` package owning only destination-neutral types, validation, IDs, and path-safe artifact names. `DocumentSegment` must not include `destination_hints`, `surface`, destination collection names, destination paths, Product Brain intents, Notion properties, Obsidian paths, or Tolaria locations.

Any unsafe or private content marker must fail closed to `blocked`. A segment may become `ready` only after safety checks, provenance, title, summary, semantic type, and confidence all pass.

## Phase 3 - Deterministic Segmenter

Implement deterministic segmentation rules in small test-backed slices:

1. Headings create section boundaries and `heading_path`.
2. Transcript speaker turns or timestamps create turn groups.
3. Markdown tables create one candidate per meaningful row plus one table-level `reference` segment when useful.
4. Consecutive bullets create list item candidates.
5. Prose sections create `meeting_note`, `source_note`, `reference`, or `unknown` based on structural signals.
6. Weak or ambiguous signals emit `unknown` with `needs_review`, never a confident destination proposal.

No LLM, network, auth, provider, browser, shell execution, or live Product Brain calls are allowed.

## Phase 4 - Writer and Containment

Write containment tests first:

- `TestWriterRejectsTraversal`
- `TestWriterRejectsSymlink`
- `TestDuplicateSegmentID`
- `TestDocumentsDecomposeWritesCompleteArtifactTree`

Implement artifact writing under:

```text
<out>/document-segments/segment-summary.json
<out>/document-segments/segments/<segment_id>.json
<out>/document-segments/previews/<segment_id>.md
```

The writer must reject traversal, symlink escape, duplicate segment IDs, unsupported schema versions, and unreferenced unexpected files. Preview Markdown must be generated from sanitized segment title/summary/provenance metadata, not copied raw private source text.

## Phase 5 - CLI Boundary

Write CLI tests first:

- `TestDocumentsDecompose`
- `TestDocumentsDecomposeDoesNotReadProductBrainProfile`
- `TestDocumentsDecomposeDoesNotEmitProductBrainProposals`

Implement:

```bash
mindline documents decompose <markdown-path-or-dir> --out <dir>
```

The default command must only read local Markdown inputs and write local document segment artifacts under `--out`. It must not read Product Brain profiles, invoke Product Brain proposal code, create `productbrain-proposals/`, or import `internal/productbrain` from `internal/documents`.

## Phase 6 - Verification

Run the required verification exactly, interpreting no-match scans as success. Because `rg` exits with code 1 when there are no matches, either run scans manually and record "no matches" or wrap them in a small verification script that treats exit code 1 as pass and exit codes above 1 as failure.

```bash
go test -count=1 ./...
go test ./internal/documents -count=1
go test ./internal/cli -run TestDocumentsDecompose -count=1
go test ./internal/documents -run 'TestGoldenMarkdownDecomposition|TestDocumentsDecomposeWritesCompleteArtifactTree|TestSegmentID|TestSegmentPath|TestDuplicateSegmentID|TestWriterRejectsTraversal|TestWriterRejectsSymlink|TestUnsupportedSchema|TestReviewStatusConfidence|TestUnsafePrivateContentMarkerBlocksSegment|TestDocumentSegmentHasNoDestinationHints|TestDocumentsPackageDoesNotImportProductBrain' -count=1
go test ./internal/cli -run 'TestDocumentsDecomposeDoesNotReadProductBrainProfile|TestDocumentsDecomposeDoesNotEmitProductBrainProposals' -count=1
go run ./cmd/mindline documents decompose testdata/documents/markdown --out /tmp/mindline-wp10-documents
go test ./internal/documents -run TestGeneratedOutputMatchesGoldenFixtures -count=1
rg "Randy|Nina|Barbara|Lucas|Geert|Klaas|Morgana|ZDHC|Saprolab|/Users/|Young Human Club|slack|http://|https://" testdata/documents internal/documents /tmp/mindline-wp10-documents
rg "destination_hints|surface|productbrain|notion|obsidian|tolaria" /tmp/mindline-wp10-documents/document-segments
rg "internal/productbrain|productbrain\\.|product-brain|Product-OS|convex|pb profile|pb capture|notion|obsidian|tolaria|slack|browser|auth|oauth|net/http|os/exec|exec.Command|http://|https://" internal/documents
rg "Product-OS|convex|pb profile|pb capture|notion|obsidian|tolaria|slack|browser|auth|oauth|net/http|os/exec|exec.Command|http://|https://" internal/cli
```

Expected:

- all tests pass;
- decomposition writes the complete expected artifact tree;
- generated output matches golden fixtures after allowed normalization;
- no-leak scans have no matches except explicit synthetic allowlist strings;
- no-live scans have no WP-10 implementation matches;
- any pre-existing `internal/cli` matches are reviewed and proven outside `documents decompose`.

## Phase 7 - Product Brain Closeout

After implementation and review, update Product Brain:

- mark `WP-10` shipped only after verification passes and review signs off;
- capture delivery sign-off decision with exact verification evidence;
- preserve any follow-up work as separate WPs, especially live PB apply/review, LLM extraction, and Notion/Obsidian/Tolaria adapters.

## Plan Ready Closeout

This current LOOP run stops at Plan Ready. After selected LOOP plan reviewers sign off on `MINDLINE-WP10-PLAN-V1`:

1. persist this plan artifact in the repository;
2. capture or link the plan sign-off in Product Brain;
3. update `WP-10` only to the appropriate plan-ready state;
4. report the ready implementation authority to Randy;
5. do not start implementation in this LOOP run.

Implementation begins only in a later delivery run that explicitly picks up `WP-10`, `DEC-26`, this signed plan, and the verification gates above. Phase 7 remains the post-delivery closeout rule for that later run.

## Implementation Order

1. Fixture tests and sanitized fixture files.
2. Contract validation tests and types.
3. Segment ID/path/provenance helpers.
4. Segmenter tests and deterministic segmenter.
5. Writer containment tests and artifact writer.
6. CLI boundary tests and CLI wiring.
7. Golden comparison and no-leak/no-live verification.
8. PB delivery evidence and review closeout.

## Non-Negotiables

- Do not stage or commit `temp/`.
- Do not put Product Brain hints into core segment artifacts.
- Do not make Product Brain required for document decomposition.
- Do not introduce live writes, network, auth, browser, provider, or shell execution.
- Do not rely on raw private example text for committed fixtures or previews.
