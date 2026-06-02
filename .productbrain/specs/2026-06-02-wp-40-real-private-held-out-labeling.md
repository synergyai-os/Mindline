# WP-40 - Real Private Held-Out Labeling Packet

## Outcome

Turn a real corpus-pressure run into a privacy-contained labeling packet and answer-key template that a human can label independently, so Mindline can move from "real inputs process cleanly" to "real outputs are measured against the right methodology."

This is a methodology and surface-readiness slice. It does not tune extraction, write to destinations, claim no-human operation, or claim DEC-64/generalization readiness.

## Real Evidence

The selecting run used a local private Slack DM slice with 10 real source messages. The artifacts are local runtime artifacts only and are not committed.

- Source type: Slack DM, private runtime.
- Processed sources: 10 of 10.
- Semantic candidates: 10.
- Evidence-ready atom ratio: 1.00.
- Review burden ratio: 0.00.
- Graph relations: 45 possible-duplicate relation candidates. This is labeling coverage for relation review, not an extractor-quality defect claim in this PR.
- Corpus fingerprint: `corpus-9b18db9afe8092a8`.
- Command config fingerprint: `config-80204d9bb0b931e4`.
- Replay fingerprint: `pressure-88cf733bb72493f1`.
- Readback status: replay baseline ready, generalization blocked, improvement blocked without baseline, top target `needs_held_out_labels`.

## Raised Standard

First-round target would have been "make the run pass." That is too weak. The product-general target is:

Mindline must expose a repeatable handoff from real private pressure artifacts into an independently labelable answer-key workflow, while making it impossible for generated or run-derived labels to satisfy held-out or DEC-64 gates.

## Key Results

- KR1: `mindline documents corpus-acceptance-labeling <corpus-pressure-out-or-parent> --out <dir>` writes a local labeling packet for every processed source in a pressure run.
- KR2: The local JSON packet includes methodology references needed for labeling: corpus fingerprints, replay fingerprints, local source IDs, redacted case IDs, source content hashes, source states, candidate IDs, candidate kinds, review statuses, evidence node IDs, relation coverage counts, and label instructions.
- KR3: The command writes an answer-key template that is explicitly labeler-owned, requires human fields, supports uncertain/abstain handling through notes and expected-state edits, and is intentionally not independent until edited by a human labeler.
- KR4: The generated template cannot make `documents corpus-acceptance --held-out` valid without human labeler edits and must fail with `answer_key_not_independent`.
- KR5: The report is PR-safe: it uses redacted deterministic case IDs and includes no raw private source text, raw URLs, Slack channel/message IDs, Slack permalinks, absolute private paths, or destination write claims.
- KR6: Real Slack runtime verification produces a 10-source packet from the private DM run and keeps destination writes, Product Brain writes, Tolaria writes, hosted inference, telemetry exports, auto-accepts, and no-human claims at zero.
- KR7: Readback/proof language remains honest: this PR can claim local real-data labeling readiness, not held-out accuracy, generalization, DEC-64, autonomous writes, or output-surface correctness.

## Behavior Impact

Users get a concrete next surface after real data processing: a labeling packet they can use to produce a real answer key. Operators stop relying on synthetic-only fixtures or command success as product proof.

The benchmark remains strict. A generated packet/template is a bridge to human labeling, not proof. Once independently labeled, the existing corpus acceptance benchmark can measure real source behavior against explicit expected outcomes.

The report surface should speak in case IDs such as `case-001` and aggregate counts. Exact source IDs may exist in local JSON artifacts because acceptance needs exact source matching, but they are not PR-facing and should not appear in markdown reports or final delivery language.

## Guardrails

- No destination writes.
- No Product Brain adapter writes from Mindline runtime.
- No Tolaria writes.
- No hosted inference unless explicitly selected elsewhere.
- No telemetry exports.
- No Gmail setup in this slice; Slack private runtime data is sufficient.
- No committed private artifacts.
- No claim that the private Slack slice generalizes.

## Exclusions

- Gmail source adapter setup.
- Destination adapter write plans.
- Autonomous no-human operation.
- Extractor tuning to reduce possible duplicate relations.
- Full held-out suite completion.

## Acceptance

- Unit tests cover packet/template construction and prove generated templates remain invalid for held-out acceptance.
- A hard negative test proves the generated `answer-key-template.json` fails held-out acceptance with `answer_key_not_independent`.
- CLI tests cover argument parsing and output routing.
- Real private Slack runtime verification writes a packet from the existing pressure run.
- The generated report passes private-content scanning.
- `go test ./...` and `git diff --check` pass.
- LOOP reviewer sign-off confirms the slice is decision-driven and claim-bounded.
