# WP-26 Plan: Agent-Assisted Semantic Review Triage

Status: draft for LOOP review
Date: 2026-05-25

## Implementation Sequence

1. Product Brain and LOOP authority.
   - Run spec/plan reviewer sign-off before implementation.
   - Confirm WP-25 is delivered and locally validated before implementation starts.
   - Capture WP-26 on Chain after sign-off and link it to INI-1, WS-2, DEC-64, ARCH-1, STD-17, STD-18, STD-19, and prior WPs. Treat WS-1 only as a provider-boundary constraint/future beneficiary, not as the owning workstream.
   - Run `pb audit WP-26` and fix authority gaps before delivery work.

2. Add the provider-neutral review proposal contract.
   - Add semantic agent-review types to `internal/documents/types.go`.
   - Keep the contract separate from semantic candidate generation.
   - Represent `choice`, `failure_reason`, `confidence`, `rationale`, `human_review_required`, `review_reason_codes`, provider, and model.
   - Treat provider/model as opaque provider-neutral audit metadata only; do not persist OpenAI-specific response fields, prompt shape, raw provider payloads, or parser assumptions in canonical judgment artifacts.
   - Make proposal metadata optional and additive so existing judgment bundles without proposals remain valid.
   - Validate choices through the existing judgment/failure taxonomy.

3. Add review proposal generation.
   - Add an `LLMSemanticReviewer` interface and prompt builder.
   - Reuse the OpenAI provider as the first implementation while keeping the call behind the provider boundary.
   - Extend `SemanticJudgmentOptions` and `JudgeSemanticCandidates` so `documents judge` can optionally attach proposals.
   - Add a pre-model safety gate: unsafe/private/governance-marker candidates are never sent to the LLM and receive a local `human_review_required=true` proposal/error state.
   - Do not persist prompts, raw source excerpts, full provider responses, or private `temp/` content to committed artifacts, reports, test fixtures, or review packets.
   - Ensure low-confidence, failed evidence readiness, blockers, invalid relation context, and unsupported model output force `human_review_required=true`.

4. Preserve judgment authority and queue behavior.
   - Proposals must not create `SemanticJudgmentRecord` files.
   - Preserve existing `SemanticJudgmentRecord` schema and accepted/rejected/judged count semantics.
   - Add summary/report counts for agent-reviewed, human-review-required, and machine-triaged candidates.
   - Define machine-triaged as proposal-only, unjudged, non-authoritative, and still auditable. It cannot satisfy review completion, autonomy metrics, DEC-64 evidence, destination readiness, PB writes, Tolaria writes, or destination artifact creation.
   - Define `human_review_required=false` as "not prioritized for manual review," not accepted/rejected/approved/saved.
   - Update `judge-next` so unjudged human-required candidates are served first when proposals exist.
   - Keep machine-triaged candidates inspectable in candidate JSON and reports.

5. Update the local review UI.
   - Show agent proposal, confidence, human-review flag, rationale, and review reason codes above manual controls.
   - Label the proposal as non-final.
   - Preserve one-item-at-a-time review, guide mode, evidence highlights, raw details, compatible reasons, token/Host safety, and serialized writes.

6. Test and verify.
   - Add fake-provider tests for proposal attachment, validation, no auto-recording, queue routing, and summary counts.
   - Add backward-compatibility tests proving pre-WP-26 artifacts still parse, report, and route correctly when proposal metadata is absent.
   - Add tests proving proposal paths cannot create or mutate `SemanticJudgmentRecord`, accepted/rejected/judged counts, Tolaria writes, PB writes, or destination artifacts without explicit human action.
   - Add UI/API tests for visible proposal labels and explicit save requirement.
   - Run focused tests and `go test ./...`.
   - Run local verification on every direct `temp/*.md` file with the configured provider and summarize only filenames and counts.
   - Run LOOP delivery review with at least three independent judges before PR.

## Review Panel

- Chain Steward: verifies WP-26 is governed by STR-3, INI-1, WS-2, DEC-64, ARCH-1, STD-17, STD-18, and STD-19, with WS-1 treated only as a provider-boundary constraint.
- Product Strategy Reviewer: verifies this is an autonomy-readiness slice, not distraction into permanent human UX.
- Systems Architect: verifies provider-agnostic boundaries and that OpenAI is only one implementation.
- Review UX Reviewer: verifies the user job is "only show me what still needs judgment" while preserving audit visibility.
- Risk/Safety Reviewer: verifies private data, no-human claims, and model-output authority are constrained.
- Delivery Quality Reviewer: verifies tests, schema changes, and verification evidence are scoped and fail-able.

## Validation Commands

```sh
pb audit WP-26
go test ./...
git diff --check
```

## Expected Evidence

- Spec and plan artifacts under `.productbrain/`.
- WP-26 Chain entry linked to WS-2 as owning workstream and the relevant strategy, architecture, standards, and work packages.
- Fake-provider test coverage for proposal generation and queue behavior.
- UI/API test coverage for visible agent proposal and no implicit judgment save.
- Local all-`temp/*.md` verification with private-content-safe counts.
- LOOP spec/plan and delivery reviewer sign-off before ready PR.
