# WP-42 Plan - Guided Corpus Acceptance Label Recording

## Sequence

1. Add label workflow domain types.
   - Summary/output type for `label-next`.
   - Local map type for redacted refs to raw packet refs.
   - Queue item type with redacted case/candidate/evidence refs.
   - Keep using existing `CorpusAcceptanceLabelRecords` and `CorpusAcceptanceLabelRecordItem`.

2. Add `label-next` builder and writer.
   - Read direct or parent `corpus-acceptance-labeling` paths.
   - Read existing records when present; bootstrap an empty non-independent records envelope when missing.
   - Validate existing records before using them to skip queue items.
   - Select next item in packet source order and candidate order.
   - Write summary JSON, local map JSON, and redacted report.
   - Return stdout with progress plus either next redacted item or queue-empty state.

3. Add `label-record` mutation path.
   - Resolve `--case-ref`, `--candidate-ref`, and `--required-evidence-ref` through the local map.
   - Revalidate resolved refs against the current packet.
   - Append or update one deterministic record ID for the same target.
   - Support `expected_present`, `expected_absent`, `uncertain`, and `abstain`.
   - Set or retain file-level provenance only through explicit human attestation.
   - Write records atomically.

4. Add CLI commands.
   - `mindline documents corpus-acceptance-label-next <labeling-dir-or-parent> --records <records.json> --out <dir>`.
   - `mindline documents corpus-acceptance-label-record <labeling-dir-or-parent> --records <records.json> --map <label-next-map.json> --case-ref <ref> --decision expected_present|expected_absent|uncertain|abstain ...`.
   - Reject destination, profile, provider, classifier, threshold, held-out, and answer-key flags.

5. Preserve proof boundaries.
   - No generated placeholder record items.
   - Unedited empty/generated records remain blocked through `corpus-acceptance-label-apply`.
   - Uncertain/abstain remain non-eval.
   - No benchmark threshold or formal readiness changes.

6. Test and verify.
   - Focused documents tests for builder, writer, queue, map resolution, mutation, and proof-blocking.
   - Focused CLI tests for usage, stdout, direct/parent path, redaction, unrelated flag rejection, and apply round trip.
   - Redaction scans for stdout/report surfaces.
   - Real private Slack packet smoke: `label-next` -> `label-record` -> `label-apply`.
   - `go test ./...`.
   - `git diff --check`.

7. Capture and publish.
   - Capture Shape/Spec/Plan sign-off and final delivery evidence in Product Brain.
   - Push branch and open PR.

## Risk Controls

- Raw refs stay in local JSON artifacts only and are excluded from stdout/report Markdown.
- `label-record` validates map refs against the current packet before writing.
- Missing records bootstrap creates an empty records envelope, not generated decisions.
- Global records independence is treated as suite/file-level because that is the current schema. It changes only with explicit human attestation and no generated placeholder records.
- Real private runtime outputs stay local and are not committed.
- The work produces human-recorded label records, not destination outputs or autonomous acceptance.

## Expected Next Work After This PR

Once this lands, the next high-leverage slice is either an interactive local review surface around the same label record contract or a larger independently labeled real held-out packet run that can begin measuring real acceptance accuracy under WP-30 gates.
