# WP-41 Plan - Corpus Acceptance Label Recording

## Sequence

1. Add label record domain types.
   - Schema: `corpus-acceptance-label-records/v0.1`.
   - Decisions: `expected_present`, `expected_absent`, `uncertain`, `abstain`.
   - Metadata: suite ID/kind, labeler, independence, min eval count, coverage requirements, notes.
   - Per-record fields: case ID, candidate ID, expected outcome ID, expected kind, source document ID, required evidence, title/summary signals, relation requirements, confidence floor, notes.

2. Add label apply builder/writer.
   - Input: labeling packet directory or parent plus records JSON.
   - Output root: `corpus-acceptance-label-recording/`.
   - Artifacts: `answer-key.json`, `label-recording-summary.json`, `label-recording-report.md`.
   - Validate records against packet case IDs, candidates, source IDs, source document IDs, and evidence nodes.
   - Include abstain/uncertain counts in summary but exclude them from expected outcomes.

3. Add CLI command.
   - `mindline documents corpus-acceptance-label-apply <labeling-dir-or-parent> --records <records.json> --out <dir>`.
   - Emit redacted summary to stdout.
   - Reject destination/profile/provider/classifier/held-out/threshold flags.

4. Preserve acceptance strictness.
   - Do not change corpus acceptance benchmark validation or formal autonomy thresholds.
   - Add hard tests proving non-independent records stay blocked when benchmarked.
   - Keep tiny held-out suites ineligible.

5. Verify with local tests and real private runtime data.
   - Focused document tests.
   - Focused CLI tests.
   - Real Slack packet + local records -> answer-key artifact.
   - Report/stdout privacy scan.
   - `go test ./...`.
   - `git diff --check`.

6. Capture Chain truth and open PR.
   - Capture outcome, real evidence, claim boundaries, guardrail status, and next blocked claim.
   - Push branch and open PR.

## Risk Controls

- Local JSON artifacts may include exact source/candidate/evidence refs for benchmark matching; stdout and markdown reports must not.
- Independence is labeler-provided metadata. The command validates shape and safety, but does not certify that a labeler was genuinely independent.
- Real private Slack labels used for runtime verification are local-only and do not become committed fixtures.
- The work produces answer keys, not destination outputs.

## Expected Next Work After This PR

Once this lands, the next high-leverage slice is a review/recording UI or guided `label-next` workflow so humans can fill records efficiently from the packet without hand-editing JSON.
