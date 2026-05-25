# WP-26 Spec: Agent-Assisted Semantic Review Triage

Status: draft for LOOP review
Date: 2026-05-25
Owner: Randy/Codex
System domain: review-learning

## Product Model Fit

Path: extend an existing canonical pattern.

WP-26 extends the semantic judgment harness from WP-18, WP-20, WP-21, WP-22, and WP-25. WP-25 must be delivered and locally validated before WP-26 implementation starts, because WP-26 assumes the decision-first review lens exists. It uses the provider-agnostic LLM port established by WP-17, but applies it to review triage rather than semantic candidate generation.

This belongs primarily and only to WS-2 because it produces evaluation evidence and lowers review burden. WS-1 is a future beneficiary and provider-boundary constraint, not the owning workstream for this slice. WP-26 does not perform provider/model comparison, destination writes, no-human approval, Product Brain apply behavior, or a hosted review product.

## Diagnosis

WP-25's delivered review lens makes the human review UI understandable, but it still makes Randy manually adjudicate every candidate. That is useful for calibration, but it does not match the product direction: Mindline should move toward autonomous semantic ingestion where humans only see genuinely hard or risky cases.

The Chain does not allow us to jump straight to no-human operation. DEC-64 requires held-out >=98% accuracy and safety guardrails before no-human claims. The next real slice is therefore not "auto-accept." It is an agent-assisted review proposal layer that lets a model pre-label candidates, identify low-risk cases, and flag only the candidates that still require human judgment. The model's output is evidence for calibration, not final system truth.

## Authority

- Strategy: STR-3 requires autonomy-readiness before destination writes.
- Initiative: INI-1 targets semantic autonomy readiness.
- Workstream: WS-2 covers evidence and evaluation foundation and excludes permanent UI/dashboard work. WP-26 is part of WS-2.
- Workstream constraint: WS-1 forbids provider lock-in and may benefit later, but WP-26 is not part of WS-1 and does not run provider/model comparison.
- Decision: DEC-64 requires >=98% measured held-out accuracy before no-human claims.
- Architecture: ARCH-1 requires interchangeable model providers behind a Mindline LLM provider port.
- Standards: STD-17 requires provider-agnostic LLM classifiers measured before trust; STD-18 requires inline source excerpts; STD-19 constrains loopback review safety.
- Prior work: WP-17 provider-agnostic LLM classifier, WP-18 judgment harness, WP-21 evidence readiness, WP-22 failure taxonomy, WP-25 decision-first review lens.

## Outcome

The judgment bundle can include an agent review proposal for each semantic candidate:

- proposed judgment choice;
- compatible failure reason when applicable;
- confidence;
- short rationale;
- risk/review reason codes;
- `human_review_required`.

The UI and CLI then make the queue concrete: high-confidence, evidence-ready proposals can be marked as machine-triaged, while risky/low-confidence proposals remain visible for human review. "Machine-triaged" means proposal-only, unjudged, non-authoritative, and still auditable in the backlog. It cannot satisfy review completion, autonomy metrics, or DEC-64 evidence without later human judgment or held-out proof.

The success shape is narrower than autonomous approval: the manual review queue size decreases, and machine-triaged candidates remain inspectable with their proposal, rationale, and risk codes.

## In Scope

- Add a provider-neutral semantic review proposal contract separate from semantic candidate generation.
- Use OpenAI as the first concrete provider through the existing provider configuration path.
- Add CLI support to build a semantic judgment bundle with agent review proposals.
- Store bounded proposal metadata in local judgment artifacts without committing private `temp/` content, prompts, raw source excerpts, or raw model responses.
- Show the proposal, confidence, rationale, risk reasons, and human-review requirement in the review UI.
- Preserve one-item-at-a-time review for candidates that require human judgment, while keeping machine-triaged candidates unjudged and auditable.
- Keep human decisions explicit: a proposal must not create a `judgment-record` by itself.
- Add deterministic fake-provider tests for proposal generation, validation, queue behavior, and UI rendering.
- Verify against every direct `temp/*.md` input locally and summarize only private-content-safe counts and filenames.

## Out of Scope

- No destination writes, Tolaria writes, Product Brain apply transport, Product Brain proposals, or auto-commit behavior.
- No no-human readiness claim and no autonomous approval metric beyond local proposal counts.
- No provider-specific architecture beyond the first OpenAI adapter implementation.
- No prompt/model benchmarking suite, provider tuning matrix, or WP-23 held-out evidence report.
- No permanent hosted review dashboard or multi-user review product.
- No external judge packet containing private source excerpts.

## Functional Contract

1. `documents judge` can opt into an agent reviewer with provider/model flags.
2. The reviewer interface is provider-neutral; OpenAI is only the first implementation.
3. The model receives the same decision context a human sees: candidate title, kind, confidence, summary, evidence readiness, source excerpts, blockers, and relation context.
4. The model must return structured JSON only: choice, failure reason, confidence, rationale, human-review flag, and review reason codes.
5. Validation rejects unsupported choices, incompatible failure reasons, missing rationale, invalid confidence, and empty review reason codes when `human_review_required=true`.
6. Agent proposals never create human judgment records and never increment accepted/rejected/judged counts.
7. The next-item queue should prioritize candidates that require human review when proposal data is present. Machine-triaged candidates remain unjudged in artifacts and reports for audit; they are not accepted, rejected, excluded, or counted as reviewed.
8. The UI shows the agent proposal before manual decision controls and clearly distinguishes "agent proposal" from "saved judgment."
9. Existing judgment persistence, failure taxonomy, evidence readiness, loopback token, Host checks, and serialized writes continue to work.
10. Unsafe/private/governance-marker candidates must not be sent to the LLM. They receive a local proposal/error state with `human_review_required=true`.
11. `agent_review_proposal` is optional and additive. Existing judgment bundles and candidate artifacts without proposals remain valid.
12. `SemanticJudgmentRecord` schema and count semantics are unchanged. Proposal paths cannot create or mutate judgment records, accepted/rejected/judged counts, Tolaria writes, PB writes, or destination artifacts without explicit human action.
13. Provider metadata in canonical judgment artifacts is opaque and provider-neutral. OpenAI-specific response fields, prompt shape, raw provider payloads, and parser assumptions must not enter canonical artifacts.

## Done When

1. The spec/plan and Chain WP-26 entry pass LOOP spec/plan sign-off and `pb audit WP-26`.
2. `documents judge` supports agent review flags without provider lock-in.
3. Judgment candidate artifacts include validated optional agent review proposals.
4. Summary/report output includes agent-reviewed, human-review-required, and machine-triaged counts, and states that machine-triaged does not mean judged.
5. `judge-next` and the UI route to human-required unjudged candidates when proposals exist, while preserving audit access to machine-triaged candidates.
6. UI tests assert the proposal is visible, labelled as non-final, and does not save a judgment without explicit user action.
7. Unit tests use fake providers and cover validation, incompatible reasons, low-confidence human-required routing, evidence-readiness risk routing, no auto-recording, and no destination/PB/Tolaria artifact creation.
8. Backward-compatibility tests prove pre-WP-26 artifacts without proposal metadata still parse, report, and route correctly.
9. Local verification runs on every direct `temp/*.md` file using the configured provider and records private-content-safe counts only.
10. `go test ./...`, `git diff --check`, and LOOP delivery review sign-off pass before PR.

## Guardrails

- Private `temp/` content may be sent to an LLM only by explicit local CLI invocation using Randy's configured key and only after the pre-model safety gate passes. Tests must not require network or real keys.
- Prompts, raw source excerpts, full provider responses, and private `temp/` content must not be written to committed artifacts, reports, test fixtures, or review packets.
- Agent proposals are calibration artifacts, not authority. They do not satisfy DEC-64 by themselves.
- `human_review_required=false` means "not prioritized for manual review." It does not mean accepted, rejected, approved, saved, destination-ready, queue-complete, or DEC-64 evidence by itself.
- Machine-triaged proposals do not make the queue complete and do not count as human judgments.
- Provider names, models, prompts, and response parsing must stay behind the provider boundary.
- The UI may reduce human review work, but must not hide the existence of machine-triaged candidates from summary/report evidence.
