# WP-42 - Guided Corpus Acceptance Label Recording

## Outcome

Add the next methodology surface after PR 36: a source-neutral local CLI workflow that lets a human inspect the next corpus acceptance labeling case and record a structured label without hand-editing JSON.

This moves real private processed inputs from a labeling packet into human-owned label records that `corpus-acceptance-label-apply` can consume. It does not claim held-out accuracy, generalization, formal autonomy-threshold readiness, destination-write readiness, or no-human operation.

## Problem

PR 35 produced real-data labeling packets. PR 36 added `corpus-acceptance-label-apply`, so valid label records can become answer-key artifacts. The remaining workflow gap is human operation: a labeler still has to inspect packet JSON, copy internal IDs, and edit records manually.

That is not good enough for the methodology. A generated scaffold can fake progress, and a redacted report that still requires raw IDs pushes the operator back into private packet spelunking.

## Direction

Add two commands:

`mindline documents corpus-acceptance-label-next <corpus-acceptance-labeling-out-or-parent> --records <label-records.json> --out <dir>`

`mindline documents corpus-acceptance-label-record <corpus-acceptance-labeling-out-or-parent> --records <label-records.json> --map <label-next-map.json> --case-ref <ref> --decision expected_present|expected_absent|uncertain|abstain [--candidate-ref <ref>] [--expected-outcome <id>] [--expected-kind <kind>] [--required-evidence-ref <ref>]... [--labeler <id>] [--independence-attestation not_generated_from_evaluated_run] [--note <safe text>]`

`label-next` reads the existing labeling packet and current records, selects the next unrecorded item in deterministic packet order, and writes:

- `corpus-acceptance-label-next/label-next-summary.json`
- `corpus-acceptance-label-next/label-next-map.json`
- `corpus-acceptance-label-next/label-next-report.md`

The stdout contract is explicit: stdout returns progress plus either the next redacted item or a queue-empty state. The report/stdout expose only redacted refs, candidate kind/status/confidence bucket, counts, blockers, and claim boundaries. Raw source IDs, candidate IDs, evidence node IDs, Slack IDs, URLs, local/private paths, raw source text, authors, notes, and governance IDs must not appear in stdout or Markdown reports.

`label-next-map.json` is local-only runtime output. It may contain raw packet refs needed for `label-record`, but it is never printed in stdout or Markdown reports.

If the records path is missing, `label-next` may bootstrap an empty records envelope with schema metadata, non-independent provenance, defaults, and zero records. It must not create generated placeholder record items.

`label-record` resolves redacted refs through the local map, validates resolved raw refs against the current packet, and appends or updates exactly one human-owned `CorpusAcceptanceLabelRecordItem`. It writes atomically so a failed write leaves the previous records file parseable and unchanged.

## Product Model Fit

- Eligibility: EXTEND.
- Pattern extended: corpus acceptance labeling and benchmark flow from WP-30, WP-40, and WP-41.
- Product object: corpus acceptance label record lifecycle.
- Why not bespoke: the workflow is source-neutral and destination-neutral. It consumes packet cases/candidates and existing label-record schema, not Slack, Gmail, Tolaria, Product Brain, or any one private corpus.
- Out of scope: browser UI, Slack or Gmail setup, LLM labels, automatic semantic choices, destination writes, benchmark threshold changes, held-out accuracy claims, generalization claims, and no-human operation claims.

## Key Results

- KR1: A real private corpus acceptance labeling packet can produce a next-case report and then one human label record through redacted refs without hand-editing JSON.
- KR2: `label-next` deterministic queueing uses packet source order and candidate order. Already recorded valid items are skipped; invalid existing records do not hide cases.
- KR3: `label-record` appends or updates a deterministic record ID for the same case/candidate/decision target and rejects unsafe values, stale maps, unknown refs, and mismatched packet refs.
- KR4: Generated or empty records envelopes remain proof-blocked. Unedited generated output must produce `BenchmarkReady=false` and `HeldOutReady=false` when passed through `corpus-acceptance-label-apply`.
- KR5: Human-recorded `expected_present` and `expected_absent` records round-trip structurally through `corpus-acceptance-label-apply`.
- KR6: `uncertain` and `abstain` records are supported, do not count as eval outcomes, and do not inflate readiness.
- KR7: Stdout and Markdown reports are redacted. Raw IDs and private refs may exist only in local JSON artifacts required for resolution and benchmark compatibility.
- KR8: No network calls, hosted inference, Slack API calls, destination writes, Product Brain writes, Tolaria writes, threshold changes, automatic labels, or no-human claims occur at runtime.

## Behavior Impact

Operators get a real local labeling surface:

- `label-next` tells them what to review next using redacted refs and progress state.
- `label-record` records the decision through structured flags and validates it against the current packet.
- `label-apply` can then turn the human-owned records into answer-key artifacts under existing gates.

This closes the practical gap between "real processed inputs produced a packet" and "a human can record real labels into a consumable output surface with the right methodology."

## Guardrails

- No generated placeholder record items.
- No independent provenance from generated scaffolds or packet origin.
- File-level independence may become `not_generated_from_evaluated_run` only through explicit human attestation and only when no generated placeholder records exist.
- `uncertain` and `abstain` never count as eval outcomes.
- Local maps may contain raw refs, but stdout/reports must not.
- No destination writes.
- No Product Brain writes from Mindline runtime.
- No Tolaria writes.
- No hosted inference.
- No hosted telemetry exports.
- No automatic labels.
- No held-out, generalization, formal autonomy, destination-write, or no-human claims.

## Acceptance

- Unit tests cover missing-record bootstrap, deterministic refs and record IDs, empty packet, zero candidates, invalid maps, unknown refs, unsafe values, invalid existing records, uncertain/abstain accounting, generated proof-blocking through label apply, and human-recorded round trip through label apply.
- CLI tests cover usage, redacted stdout contracts, direct and parent labeling paths, unrelated flag rejection, and apply compatibility.
- Writer tests cover report redaction and local map separation.
- A real private Slack runtime packet can produce a local next-case report, local map, one local human record, and label-apply output without writing destinations or leaking private data in stdout/report surfaces.
- `go test ./...` and `git diff --check` pass.
- LOOP reviewers sign off on Shape/Spec/Plan and final delivery.
