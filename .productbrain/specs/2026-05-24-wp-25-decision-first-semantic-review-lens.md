# WP-25 Spec: Decision-First Semantic Review Lens

Status: signed off for delivery
Date: 2026-05-24
Owner: Randy/Codex
System domain: review-learning

## Product Model Fit

Path: extend an existing canonical pattern.

WP-25 extends the existing local semantic judgment harness from WP-18, WP-20, WP-21, and WP-22. It does not create a permanent review product, dashboard, destination writer, or model-improvement loop. The work belongs in WS-2 because reliable human labels are a prerequisite for WP-23's held-out autonomy-readiness evidence report.

## Diagnosis

The current semantic review UI exposes the judgment bundle, but the reviewer still has to decode raw relation plumbing, evidence IDs, readiness tags, and failure-reason mechanics before making a decision. On real `temp/` artifacts this makes the label quality depend on reviewer patience and system knowledge, not only candidate correctness.

That blocks WP-23. A held-out evidence report is only useful if each human judgment records a clear decision against the candidate, its strongest evidence, and the failure taxonomy. If the reviewer cannot quickly answer "should this candidate count?", the resulting metrics are contaminated.

## Authority

- Strategy: STR-3 requires autonomy-readiness before destination writes.
- Initiative: INI-1 targets semantic autonomy readiness.
- Workstream: WS-2 covers evidence and evaluation foundation and excludes permanent UI/dashboard work.
- Decision: DEC-64 requires at least 98% measured held-out accuracy before no-human claims.
- Standards: STD-17 provider-agnostic measured trust, STD-18 inline source excerpts, STD-19 loopback review API safety.
- Tension: TEN-6 shows that pagination alone is not reviewable when the preview lacks the information needed for adjudication.

## Outcome

A local one-item-at-a-time review lens lets a reviewer make a high-confidence judgment without first parsing raw artifact internals. The first viewport must explain the review task, show candidate meaning, show the strongest evidence, show progress, and keep decision controls aligned to the active candidate.

## In Scope

- Reframe the local review page around the decision question: "Should this candidate count as a correct semantic extraction?"
- Keep the one-item pagination invariant and visible total/judged/remaining progress.
- Show candidate title, kind, confidence, source, review status, evidence readiness, and summary in a decision-oriented order.
- Show a concise default evidence set: inline source excerpts first, limited by default, with counts for hidden evidence and relations.
- Collapse raw relation cards, evidence IDs, relation IDs, blockers, and readiness internals behind explicit detail sections.
- Make non-accept decisions use contextual compatible failure reasons only.
- Keep all existing JSON artifacts, readiness data, relation context, and raw details available for audit.
- Preserve loopback-only UI safety: token, same-origin/Host checks, JSON content type, serialized writes, no read-path mutation.
- Verify against real local `temp/*.md` semantic judgment artifacts without committing private temp content.

## Out of Scope

- No destination writes, Tolaria writes, Product Brain apply transport, auto-accept, auto-commit, or no-human claim.
- No permanent dashboard, auth system, multi-user review product, or hosted review service.
- No provider/model/prompt tuning and no provider-specific architecture changes.
- No WP-23 report implementation beyond ensuring this UI can produce reliable labels for it.
- No external LLM judge packets containing private source excerpts.

## Functional Contract

1. The page still serves exactly one unjudged candidate at a time.
2. The review screen's first viewport prioritizes review task, candidate summary, evidence highlights, progress, and decision controls.
3. Evidence highlights show inline excerpts when available; unavailable excerpts are shown as a decision-relevant warning, not as silent absence.
4. Raw relation/evidence plumbing is collapsed by default and remains inspectable.
5. Relation context is summarized as counts and relationship-type chips by default; detailed endpoint cards are behind a details control.
6. Decision controls require compatible failure reasons for reject, unclear, duplicate, and wrong-kind. Accept records no failure reason.
7. The guide mode explains the mental model: the reviewer evaluates extraction correctness, not final knowledge-writing approval.
8. Existing judgment persistence, summary recomputation, failure taxonomy counts, and evidence-readiness counts continue to work.

## Done When

1. The default review HTML contains a decision question before any raw relation detail text, and tests assert that `Review task`, `Should this candidate count`, `Evidence highlights`, `Raw details`, and `Save decision` are present.
2. The default review view renders at most five source excerpts outside collapsed details, shows the hidden-excerpt count when more exist, and renders zero expanded raw relation cards before the raw-details disclosure is opened.
3. Detail sections disclose hidden evidence ranges, relation IDs, relation endpoint cards, blockers, and readiness internals behind collapsed `<details>` controls.
4. Non-accept decisions expose only compatible failure reasons for the selected decision and persist the selected reason through `/api/judgments`.
5. Automated tests cover the HTML/JS contract in `internal/cli/documents_decompose_test.go`, the API write contract, one-item pagination, and read-path non-mutation.
6. Real `temp/*.md` verification records only private-content-safe evidence: source filenames, candidate counts, served page status, visible excerpt counts, collapsed-detail status, progress state, and compatible-reason behavior. It must not copy source excerpts into committed artifacts or external judge packets.
7. `go test ./...`, `git diff --check`, and `pb audit WP-25` pass before PR.

## Guardrails

- Private `temp/` content may be used for local verification only.
- Screenshots or final summaries must avoid exposing private source excerpts unless Randy explicitly asks for them.
- The work may improve human review ergonomics, but it must not imply the semantic engine is accurate enough for autonomous operation.
