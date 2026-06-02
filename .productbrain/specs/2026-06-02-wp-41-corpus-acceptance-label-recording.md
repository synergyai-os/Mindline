# WP-41 - Corpus Acceptance Label Recording

## Outcome

Add the next methodology surface after PR 35: turn a real corpus acceptance labeling packet plus human-owned label records into a corpus acceptance answer key artifact.

This closes the gap between "real inputs produced a labeling packet" and "real outputs can be measured by the existing corpus acceptance benchmark." It does not claim held-out accuracy, generalization, formal autonomy-threshold readiness, destination-write readiness, or no-human operation.

## Problem

PR 35 produced a local labeling packet and a non-independent answer-key template. That is the correct handoff, but it still requires a manual, error-prone edit of the answer-key JSON. The current system has no typed record format, no validation against the packet's cases/candidates/evidence refs, no abstain/uncertain accounting, and no redacted report that shows whether a recorded label set is benchmark-ready.

## Direction

Add a recording/apply command:

`mindline documents corpus-acceptance-label-apply <labeling-dir-or-parent> --records <records.json> --out <dir>`

The command reads the local labeling packet, reads human-owned label records, validates them against packet case IDs, candidate IDs, source IDs, source document IDs, and evidence node IDs, then writes:

- `corpus-acceptance-label-recording/answer-key.json`
- `corpus-acceptance-label-recording/label-recording-summary.json`
- `corpus-acceptance-label-recording/label-recording-report.md`

The generated answer key may use independent provenance only when the input records explicitly provide independent provenance. This command must not infer independence from the packet or from Codex-generated data.

## Product Model Fit

- Eligibility: EXTEND.
- Pattern extended: corpus acceptance labeling/benchmark flow from WP-30 and WP-40.
- Product object: corpus acceptance answer key lifecycle.
- Why not bespoke: the command is source-neutral and destination-neutral. It consumes packet cases/candidates rather than Slack-specific fields, and it feeds the existing corpus acceptance benchmark.
- Out of scope: interactive UI, Gmail setup, destination writes, automatic labeling, LLM labeling, and benchmark threshold changes.

## Key Results

- KR1: A new label-record schema captures labeler, independence, suite kind, min eval count, coverage, and per-case label decisions.
- KR2: Label records validate against the labeling packet; unknown case IDs, candidate IDs, evidence refs, source IDs, or unsafe values block artifact generation.
- KR3: `expected_present` and `expected_absent` records become `SemanticExpectedOutcome` entries in `answer-key.json`; `uncertain` and `abstain` records are counted but do not become expected outcomes.
- KR4: Generated output reports benchmark readiness without overclaiming. It shows eval count, abstain count, uncertain count, held-out readiness, and blocking reasons.
- KR5: PR-facing stdout/report surfaces use aggregate counts and redacted case IDs only; they must not expose raw source text, URLs, Slack identifiers, private paths, source IDs, candidate IDs, evidence node IDs, or governance IDs.
- KR6: A real private Slack runtime packet can be processed with local operator label records into a local answer-key artifact while keeping all side-effect guardrails at zero.
- KR7: Existing corpus acceptance remains strict. Tiny held-out suites still fail formal autonomy eligibility, and generated/non-independent records cannot produce valid held-out proof.

## Behavior Impact

Operators get a typed path for turning real private cases into an answer key. The system can now separate three states:

- packet ready for labeling;
- labels recorded but not enough or not independent enough for held-out claims;
- answer key ready to benchmark under existing acceptance gates.

This is the first operational surface where real processed inputs can land in a real measurement artifact.

## Guardrails

- No destination writes.
- No Product Brain writes from Mindline runtime.
- No Tolaria writes.
- No hosted inference.
- No telemetry exports.
- No automatic labels.
- No no-human claims.
- No private content in committed tests or PR-facing reports.

## Acceptance

- Unit tests cover valid apply, unknown case/candidate/evidence blockers, abstain/uncertain accounting, and non-independent provenance blocking.
- CLI tests cover argument parsing, redacted stdout, and artifact output.
- A real private Slack runtime packet is processed with local label records and produces a local answer-key artifact plus redacted report.
- `go test ./...` and `git diff --check` pass.
- LOOP reviewers sign off on Shape/Spec/Plan before delivery and on final delivery before close.
