# WP-45 Plan

## 1. RED Tests

- Add package tests for seed-mode apply without `seed-private-map.json` failing before `answer-key.json` is written.
- Add package tests for stale or mismatched seed private maps failing closed.
- Add package tests for seed-map-backed apply output under workspace, `.productbrain`, and `testdata` roots being rejected after symlink resolution.
- Add package tests for successful seed apply reporting aggregate fields and `artifact_confidentiality=local_private_rehydrated`.
- Add CLI end-to-end test for `labeling --seed -> label-next -> label-record -> label-apply -> corpus-acceptance`.
- Add leak assertions for stdout, stderr, report Markdown, durable JSON summary, and failure/error messages.

## 2. Summary Schema

- Extend `CorpusAcceptanceLabelRecordingSummary` and CLI output summary with seed handoff status fields.
- Keep non-seed recordings backward compatible with zero/default values and `artifact_confidentiality=non_seed_local`.
- Treat `local_private_rehydrated` as the confidentiality class for the entire seed-map-backed apply output tree.

## 3. Seed Apply Validation

- Detect seed mode from the labeling packet claim boundary and seed packet identity.
- Require a valid private map for seed-mode apply.
- Fail before write when the map is absent, mismatched, stale, or invalid.
- Count translated sources, expected outcomes, and evidence refs while building the answer key.

## 4. Private-Local Output Protection

- Add reusable resolved-path rejection for local-private rehydrated apply output.
- Reject current workspace descendants, `.productbrain`, `testdata`, durable report roots, and other git-visible/durable roots.
- Run rejection before `os.MkdirAll` or any answer-key write.

## 5. Report And CLI Output

- Add redacted aggregate handoff fields to stdout and `label-recording-report.md`.
- Do not print source IDs, candidate IDs, evidence node IDs, source paths, temp paths, source excerpts, private-map entries, PB IDs, Slack/Gmail IDs, or destination locators.
- Preserve existing claim boundaries and readiness blockers.

## 6. Verification

Run focused tests first:

```bash
go test ./internal/documents -run 'CorpusAcceptanceLabelRecording|CorpusAcceptanceLabelingSeed'
go test ./internal/cli -run 'DocumentsCorpusAcceptanceLabel'
```

Run full verification:

```bash
go test ./...
git diff --check
```

Run real private proof under `/private/tmp`:

```bash
go run ./cmd/mindline documents corpus-pressure temp/pb-docs --out /private/tmp/mindline-wp45-real-pressure
go run ./cmd/mindline documents corpus-acceptance-labeling /private/tmp/mindline-wp45-real-pressure --out /private/tmp/mindline-wp45-real-seed --seed --max-cases 50
go run ./cmd/mindline documents corpus-acceptance-label-next /private/tmp/mindline-wp45-real-seed --records /private/tmp/mindline-wp45-real-records.json --out /private/tmp/mindline-wp45-real-next
go run ./cmd/mindline documents corpus-acceptance-label-record /private/tmp/mindline-wp45-real-seed --records /private/tmp/mindline-wp45-real-records.json --map /private/tmp/mindline-wp45-real-next/corpus-acceptance-label-next/label-next-map.json --case-ref case-001 --candidate-ref candidate-001 --decision expected_present --required-evidence-ref evidence-001 --labeler codex_runtime_probe --note local-non-independent-handoff-probe
go run ./cmd/mindline documents corpus-acceptance-label-apply /private/tmp/mindline-wp45-real-seed --records /private/tmp/mindline-wp45-real-records.json --out /private/tmp/mindline-wp45-real-recording
go run ./cmd/mindline documents corpus-acceptance /private/tmp/mindline-wp45-real-pressure --answer-key /private/tmp/mindline-wp45-real-recording/corpus-acceptance-label-recording/answer-key.json --out /private/tmp/mindline-wp45-real-acceptance --held-out
```

Expected real proof:

- corpus pressure succeeds with zero side-effect guardrails;
- seed queue succeeds;
- label apply reports seed private-map status and translation counts;
- corpus acceptance consumes the answer key and counts the applied label;
- suite/DEC64 readiness remains blocked for expected min-count and independence reasons;
- stdout/report/PB proof stays aggregate-only.

## 7. PB Closeout

- Capture spec/plan authority, final proof, blocked claims, and real-data limitation in PB.
- Capture any durable standard or decision if implementation reveals a reusable privacy/output-root rule.
- Request LOOP review before opening the PR.
