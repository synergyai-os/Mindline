# MINDLINE-WP12-SPEC-V4: Real input structure benchmark for process docs and transcripts

Date: 2026-05-21
Work package: WP-12.
Parent surface: WP-11 document structure extraction in PR #7.

## Problem

WP-11 gives Mindline a destination-neutral `document-structure/` artifact layer, but real private source tests exposed two quality gaps:

1. Notion-style exports can start with prose and tables, then use `###` headings without a preceding H1/H2. The current parser treats absolute heading depth as real hierarchy, so sibling `###` headings can become nested under the first `###` heading.
2. Real capability lists often use code-prefixed bullets such as `PL-1`, `PL-23`, `P-S1`, or `P-B4`. WP-11 only recognizes explicit `Capability:` or `cap-` style labels, so these real capabilities stay invisible as typed capability nodes.
3. Real meeting transcripts can contain timestamp/speaker turns without Markdown headings. WP-11 emits only a single document node, which hides the transcript's basic conversational structure even though WP-10 finds many flat segments and action candidates.

This makes the output better than WP-10 flat segments, but still not good enough for recognizable process/capability documents.

## Scope

In scope:

- Normalize Markdown heading hierarchy per source document before section parsing.
- If a document has no H1, treat the shallowest heading level present as the top structure level for that document.
- Keep existing H1/H2/H3 behavior unchanged for ordinary Markdown with a proper H1.
- Preserve pre-heading prose and tables under the document root instead of making the first lower-level heading the root.
- Use the filename-derived title as the document root when no H1 exists.
- Recognize code-prefixed capability bullets and table-row titles as `capability` nodes when they match stable product/platform capability code patterns.
- Detect timestamp/speaker transcript turns and emit deterministic `transcript_turn` structure nodes under the document root.
- Keep transcript turn output structural only: no invented topics, summaries, decisions, or AI interpretation.
- Add synthetic fixtures that represent the private Notion/process-doc shape without committing private content.
- Update ignored real-file comparisons after the loop for both private benchmark sources:
  - `temp/notion-doc-1.md`
  - `temp/meeting-transcript-1.md`

Out of scope:

- No Product Brain proposal generation.
- No destination mapping to Product Brain, Tolaria, Notion, Obsidian, or other tools.
- No LLM/AI interpretation or ontology solving.
- No raw private temp file content committed to the repo.
- No semantic rewrite of WP-10 segment classification.
- No broad extraction of arbitrary uppercase codes as capabilities unless they fit the documented capability-code pattern.
- No transcript topic modeling, speaker identity resolution, or action-owner assignment.

## Contract

### Heading normalization

For every parsed Markdown document:

- If at least one H1 exists, preserve normal Markdown heading semantics.
- If no H1 exists, compute `base_heading_level = minimum heading level in the source`.
- Normalize every heading level as `normalized_level = heading_level - base_heading_level + 1`.
- Use normalized heading levels for `heading_path` and parent/child section structure.
- When exported Markdown skips an available parent level, attach the heading to the nearest existing ancestor at a shallower source heading level; if no such ancestor exists, attach it to the document root. Do not invent placeholder parents.
- Keep source line numbers unchanged.
- The document root title is:
  - first H1 text when an H1 exists;
  - otherwise the filename stem, sanitized only for paths/IDs, while the title remains readable.

### Capability code recognition

Recognize a list item or table-row title as a `capability` when it begins with a capability code followed by a separator and readable text.

Exact accepted matcher:

- Strip Markdown emphasis markers from the candidate title.
- Candidate title must start with either:
  - `PL-` followed by one or more digits, optionally followed by an en-dash range and one or more digits; or
  - `P-` followed by one uppercase letter and one or more digits.
- The code must be followed by ` - `, ` — `, or `: `.
- At least one readable non-code word must follow the separator.
- Anything else remains untyped unless another existing WP-11 typed-node rule applies.

Accepted code examples:

- `PL-1 - Access and central entry`
- `PL-23 - Contacts and relationships`
- `PL-10–12 — Rulebook stewardship`
- `P-S1 - Maintain chemical inventory`
- `P-S1 — Maintain chemical inventory`
- `P-B4 - Participate in Brands to Zero`

Rejected examples:

- prose that merely mentions `PL-1`
- `ABC-1 - Not a capability`
- `PL-1` with no title
- `P-S - Missing number`
- dates, issue references, or IDs without a separator and readable title
- private/governance identifiers already blocked by WP-11 safety rules

The capability node title should remove Markdown emphasis and keep the code plus readable capability text.

Table-row capability recognition must apply to the row title cell, not arbitrary cells. A row such as `| PL-23 - Contacts and relationships | Shared reference |` should produce the row node plus a child `capability` node under the same table hierarchy.

### Structure schema extension

WP-12 explicitly authorizes `transcript_turn` as an additive `DocumentStructure` node type in `document-structure-node/v0.1`.

Rationale:

- PR #7 has not merged, so the v0.1 structure contract can still absorb this additive node enum before release.
- Existing WP-11 node types and JSON field names remain unchanged.
- Downstream consumers that do not understand `transcript_turn` can still read it as a typed structure node with the same common fields, provenance, parent/child contract, review status, confidence, blockers, and related segment IDs.
- No schema version bump is required for this PR because there is no released incompatible change; if PR #7 merges before WP-12 lands, this must be revisited before delivery.

### Transcript turn recognition

Recognize transcript turns when a line starts with a timestamp and speaker label:

- `00:02 - Facilitator`
- `12:44 - Product Lead`
- `1:02:03 - Speaker Name` is acceptable if present later.

For each detected turn:

- Emit a `transcript_turn` node under the document root.
- Use the timestamp plus speaker as the title.
- Use the source line range from the timestamp line through the last utterance line before the next turn.
- Preserve related WP-10 segment IDs for that line range.
- Keep confidence `medium` and review status `ready` when timestamp, speaker, and utterance text are present.
- Mark malformed turns `needs_review` only when timestamp/speaker is present but no utterance text follows before the next turn.

Transcript turn extraction must not infer decisions, commitments, owners, or agenda topics. Those remain downstream processor work.

## Acceptance

- Synthetic no-H1 fixture produces a document root from the filename, not from the first `###` heading.
- Sibling `###` headings in a no-H1 fixture become sibling sections, not nested descendants.
- Pre-heading table remains under the document root.
- Code-prefixed list items become `capability` nodes.
- Synthetic transcript fixture produces multiple `transcript_turn` nodes instead of only a document root.
- Transcript turn nodes preserve speaker, timestamp, line range, and related segment IDs.
- Existing WP-11 golden fixtures remain deterministic after regeneration.
- The private Notion/process-doc comparison shows the same source file moves from a false first-heading root toward a filename/root plus sibling section structure and typed capabilities.
- The private transcript comparison shows the same source file moves from a single structure node toward timestamp/speaker turn structure while preserving only line ranges and related WP-10 segment IDs; no action, owner, topic, decision, or commitment evidence is emitted by WP-12 structure output.
- Synthetic table-row capability fixture proves `PL-*` table row titles emit child `capability` nodes.
- Negative fixtures prove `ABC-1 - ...`, `PL-1` without title, and prose merely mentioning `PL-1` do not become capability nodes.
- Generated structure output remains destination-neutral and contains no Product Brain/Tolaria/Notion/Obsidian/profile/proposal hints, private markers, Chain governance IDs, live integration hooks, or `related_segment_ids: null`.

## Verification

Required commands:

- `go test -count=1 ./...`
- `go run ./cmd/mindline documents structure testdata/documents/structure --out /tmp/mindline-wp12-structure-verify`
- `diff -ru testdata/documents/expected/structure /tmp/mindline-wp12-structure-verify/document-structure`
- Leakage scans over generated structure output for destination names, private markers, Chain governance IDs, live integration terms, and `related_segment_ids: null`.
- Real private comparison regeneration for `temp/notion-doc-1.md`, kept ignored under `temp/`.
- Real private comparison regeneration for `temp/meeting-transcript-1.md`, kept ignored under `temp/`.

## Product Model Fit

Verdict: EXTEND existing canonical pattern.

WP-12 extends WP-11's destination-neutral document structure model. It does not create a new destination, source adapter, proposal adapter, or private workflow. It improves the generic Markdown parser and structural recognizers so the same artifact contract works for real exported process documents and transcript-shaped inputs.

## Risks

- Over-recognizing arbitrary codes as capabilities.
- Regressing normal H1-based Markdown hierarchy.
- Accidentally committing private temp examples.
- Hiding remaining semantic uncertainty by making all code bullets look confidently solved.
- Overclaiming transcript understanding by turning speaker turns into inferred topics or commitments.

Mitigations:

- Use a narrow capability-code pattern and synthetic fixtures.
- Preserve H1 behavior when H1 exists.
- Keep private comparison artifacts ignored.
- Keep confidence/review behavior and safety redaction from WP-11.
- Limit transcript extraction to timestamp/speaker turns and line-range provenance.
