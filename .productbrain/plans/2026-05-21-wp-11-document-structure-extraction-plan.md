# MINDLINE-WP11-PLAN-V2: Document Structure and Capability Model Extraction

Date: 2026-05-21
Status: Plan for LOOP review
Stop mode: Plan Ready

## Authority

- Work package: `WP-11` Document structure and capability model extraction.
- Signed spec: `MINDLINE-WP11-SPEC-V5`.
- Spec sign-off decision: `DEC-29`.
- Prior layer: `WP-10` destination-neutral `document-segments/v0.1`.
- Product boundary: `DOMAIN-1` says Product Brain is a future downstream consumer, not the product or schema owner.
- Safety standards: `STD-12`, `STD-13`, `STD-16`.

## Implementation Precondition

WP-11 implementation must start from a branch that contains the merged WP-10 document decomposition code and tests. If WP-10 is not yet on `main`, create the implementation branch stacked on the merged WP-10 branch or wait for WP-10 to land.

Do not stage, commit, or copy raw files from `temp/`. Use only synthetic fixtures created for this work package.

## Build Sequence

### 1. Synthetic Fixtures First

Create synthetic Markdown fixtures under `testdata/documents/structure/`:
- `process-capability-table.md`
- `audience-capability-list.md`
- `mixed-structure.md`
- `malformed-ambiguous.md`
- `unsafe-marker.md`

Create expected golden output under `testdata/documents/expected/structure/` after the red test defines the desired contract.

Fixture rules:
- Structurally resemble real process/capability documents.
- Use synthetic labels only.
- Include headings, tables, repeated row patterns, audience sections, mixed prose/bullets, malformed table/list cases, and explicit unsafe-marker cases.
- Do not include Product Brain, Tolaria, Notion, Obsidian, real customer/project labels, private source names, or recognizable user examples.

### 2. Red Tests for Structure Contract

Add focused tests in the existing document package, following WP-10 naming and helper patterns.

Required test coverage:
- CLI accepts one Markdown file and a directory.
- `document-structure/structure-summary.json` is written.
- `nodes/*.json` and `previews/*.md` are written.
- summary and node schemas match `document-structure-summary/v0.1` and `document-structure-node/v0.1`.
- directory traversal is lexical by normalized relative path.
- `root_node_ids`, summary `nodes[]`, `child_node_ids`, and `related_segment_ids` use stable ordering.
- `run_id` and `node_id` are deterministic and do not use wall-clock time, random values, absolute local paths, filesystem discovery order, or Chain/Product Brain IDs.
- parent-child relationships preserve document, section, table, row, capability, audience, workflow, requirement, and unknown structure where applicable.
- structure nodes link back to WP-10 segment IDs and source line ranges.
- ambiguous or malformed inputs produce `needs_review` or `unknown`, not confident guesses.

### 3. Structure Model and Extractor

Introduce internal document-structure types near the existing WP-10 document code.

Model requirements:
- `DocumentStructureSummary`
- `DocumentStructureNode`
- `DocumentStructureEvidence`
- review status enum: `ready`, `needs_review`, `blocked`, `skipped`
- node type enum: `document`, `section`, `table`, `table_row`, `capability`, `audience`, `workflow`, `requirement`, `unknown`

Extractor requirements:
- consume Markdown and/or WP-10 `DocumentSegment` output without changing the WP-10 persisted segment contract.
- create one document root per source document.
- create section nodes from Markdown headings.
- create table and table-row nodes from Markdown tables.
- derive capability/audience/workflow/requirement nodes only from deterministic structural patterns in synthetic fixtures.
- mark uncertain structure as `needs_review` or `unknown`.
- preserve source evidence with document ID, heading path, line start/end, content hash, and related segment IDs.

### 4. Safe Structure Writer

Implement a local-only writer for `document-structure/` artifacts.

Writer requirements:
- write only under explicit `--out`.
- reject traversal, symlinked root, symlinked parents, symlinked generated paths, stale unexpected generated files, and duplicate IDs.
- finalize/redact nodes before expected-file calculation, summary construction, validation, JSON writes, and preview writes.
- build `node_path` only from finalized/redacted fields.
- rebuild `structure-summary.json` from finalized nodes.
- reject caller-supplied or prebuilt summaries that contain unsafe/private markers or governance/destination leakage.
- ensure persisted summary, nodes, and previews contain no Chain/Product Brain authority IDs.

### 5. CLI Command

Add:

```text
mindline documents structure <markdown-path-or-dir> --out <dir>
```

CLI requirements:
- follows existing document command style and exit-code classes.
- accepts one Markdown file or a directory.
- writes artifacts under `<out>/document-structure/`.
- prints deterministic summary JSON to stdout.
- rejects Product Brain profile/proposal flags on this path.
- does not introduce live network, auth, browser, provider, daemon, UI, LLM, Slack, Tolaria, Notion, Obsidian, or Product Brain write surfaces.

### 6. Golden Output and Boundary Scans

Generate expected golden artifacts only from synthetic fixtures.

Golden output must include:
- summary JSON
- node JSON files
- preview Markdown files

Boundary scans must prove:
- no destination hints in generated output.
- no live integration terms in `internal/documents`.
- no unsafe/private/governance markers in generated output.
- private `temp/` files remain untracked or ignored and unstaged.

### 7. Final Verification

Run and report:

```bash
go test -count=1 ./...
go run ./cmd/mindline documents structure testdata/documents/structure --out /tmp/mindline-wp11-structure-verify
diff -ru testdata/documents/expected/structure /tmp/mindline-wp11-structure-verify/document-structure
rg -n "Product Brain|Tolaria|Notion|Obsidian|proposal|profile|collection" /tmp/mindline-wp11-structure-verify/document-structure
rg -n "https?://|\\b(api|auth|token|browser|provider|daemon|llm|slack)\\b" internal/documents
rg -n "UNSAFE|PRIVATE|SECRET|REDACT_ME|authority_ids|authority_id" /tmp/mindline-wp11-structure-verify/document-structure
rg -n "\\b(PROD|DOMAIN|WP|DEC|STD|INS)-[0-9]+\\b" /tmp/mindline-wp11-structure-verify/document-structure
git status --short --ignored temp
```

Expected results:
- tests, CLI command, and golden diff pass.
- all three `rg` scans produce no generated-output leakage matches, except the live-integration scan may include comments or fixture names explicitly proving absence.
- the Chain/PB governance ID scan produces no generated-output matches.
- `temp/` is not staged and raw private files are not committed.

## PR Shape

Implementation PR should include:
- synthetic fixtures and expected golden artifacts.
- internal structure model/extractor/writer.
- CLI command.
- tests for extraction, determinism, writer safety, CLI behavior, and boundary scans.
- PR description that names `WP-11`, signed spec `MINDLINE-WP11-SPEC-V5`, and verification command results.

Do not include:
- raw private examples.
- Product Brain proposal generation.
- live Product Brain writes.
- destination adapter mapping.
- Tolaria, Notion, Obsidian, Slack, browser, network, auth, provider, daemon, UI, or LLM integration.
- ontology or semantic classification beyond deterministic structural extraction.

## Review Gates

Before requesting staff+ review:
- Confirm WP-10 code is present on the implementation branch.
- Confirm generated output is deterministic across two consecutive runs into clean temp directories.
- Confirm no private temp files are staged.
- Confirm `git diff --cached` contains only WP-11 implementation, fixtures, expected artifacts, and tests.
- Confirm Product Brain state reflects implementation outcome before closing the PB session.

## Rollback / Abort Gate

Stop implementation and do not request staff+ review if any containment, privacy, determinism, WP-10 regression, or CLI behavior gate fails.

Rollback path:
- revert the WP-11 implementation slice or PR branch changes.
- remove generated local `document-structure/` artifacts and temp verification outputs.
- preserve WP-10 `document-segments/v0.1` behavior unchanged.
- keep raw private `temp/` files untracked or ignored and unstaged.
- capture the blocker in Product Brain before closing the session or reshaping the work.

## Follow-Up Boundary

The likely next work package remains `WP-12`: a local review/apply contract for `DocumentSegment` and `DocumentStructure` artifacts. WP-11 must not implement that review/apply layer.
