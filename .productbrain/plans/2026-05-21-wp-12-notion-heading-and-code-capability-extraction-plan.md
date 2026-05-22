# MINDLINE-WP12-PLAN-V4: Real input structure benchmark for process docs and transcripts

Date: 2026-05-21
Spec: `.productbrain/specs/2026-05-21-wp-12-notion-heading-and-code-capability-extraction.md`
Target PR: #7, branch `codex/wp-11-document-structure`

## Delivery Sequence

1. Add synthetic no-H1 Notion/process-doc and transcript fixtures.
   - Include intro text.
   - Include a pre-heading table.
   - Include sibling `###` sections.
   - Include `PL-*` and `P-*` capability-code bullets.
   - Include a table-row title such as `PL-23 - Contacts and relationships`.
   - Include negative code cases: `ABC-1 - Not a capability`, `PL-1` without title, and prose mentioning `PL-1`.
   - Include timestamp/speaker transcript turns with utterance text.

2. Add failing structure assertions.
   - Document root is filename-derived when no H1 exists.
   - Sibling `###` headings are siblings.
   - Pre-heading table is attached to document root.
   - Code-prefixed bullets produce capability nodes.
   - Code-prefixed table-row titles produce child capability nodes under the table row.
   - Negative code cases remain untyped.
   - Transcript-shaped input produces `transcript_turn` nodes.
   - Transcript turns preserve timestamp/speaker title, line range, and related segment IDs.

3. Normalize heading levels inside the Markdown parser.
   - Keep H1 documents unchanged.
   - For no-H1 documents, normalize heading depth while attaching skipped parent levels to the nearest existing shallower ancestor or document root.
   - Preserve source line numbers.

4. Improve document root title behavior.
   - Use first H1 when present.
   - Use filename stem when no H1 exists.
   - Keep root node path deterministic.

5. Add code-prefixed capability recognition.
   - Recognize only `PL-<digits>`, optional `PL-<digits>–<digits>` ranges, and `P-<uppercase letter><digits>` followed by ` - `, ` — `, or `: ` and readable text.
   - Apply table-row recognition only to the row title cell.
   - Strip Markdown emphasis for capability titles.
   - Avoid broad/governance/private code inference.

6. Add transcript turn structure extraction.
   - Add `transcript_turn` to the document-structure-node/v0.1 enum as an explicit WP-12 additive schema extension.
   - Detect timestamp/speaker lines.
   - Create turn nodes under document root.
   - Keep the behavior structural, not interpretive.
   - Mark malformed empty turns as `needs_review`.

7. Regenerate synthetic golden structure output.

8. Update the ignored private comparison artifacts.
   - Rerun WP-10 `documents decompose` for `temp/notion-doc-1.md`.
   - Rerun WP-12 structure extraction for the same file.
   - Rerun WP-10 `documents decompose` for `temp/meeting-transcript-1.md`.
   - Rerun WP-12 structure extraction for the same file.
   - Update `temp/wp11-notion-doc-1-before-after.md` and `temp/wp12-meeting-transcript-1-before-after.md` with before/after counts, structural summaries, and local-only source context needed for review. These files remain ignored and must not be committed.

9. Verify.
   - `go test -count=1 ./...`
   - CLI generation plus golden diff.
   - Destination/live/private/governance/null scans.
   - `git status --short --ignored temp` confirms temp remains ignored.

10. LOOP review.
   - Chain Steward.
   - Domain/User Job Reviewer.
   - Systems Architect.
   - Delivery Quality Reviewer.
   - Risk/Safety Reviewer.
   - Rerun full panel if any blocker changes output.

11. Commit and push to PR #7.

## Files Expected To Change

- `internal/documents/decompose.go`
- `internal/documents/structure.go`
- `internal/documents/documents_test.go`
- `internal/documents/types.go`
- `testdata/documents/structure/*`
- `testdata/documents/expected/structure/*`
- `.productbrain/specs/2026-05-21-wp-12-notion-heading-and-code-capability-extraction.md`
- `.productbrain/plans/2026-05-21-wp-12-notion-heading-and-code-capability-extraction-plan.md`

Ignored evidence, not committed:

- `temp/wp11-notion-doc-1-before-after.md`
- `temp/wp12-meeting-transcript-1-before-after.md`
- `/tmp/mindline-wp11-real-doc-before`
- `/tmp/mindline-wp11-real-doc-after`
- `/tmp/mindline-wp12-transcript-before`
- `/tmp/mindline-wp12-transcript-after`

## Exclusions

- No committed private source material.
- No destination/proposal adapter changes.
- No live integrations.
- No LLM classification.
- No Product Brain work-item state transition until review proof exists.
