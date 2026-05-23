# MINDLINE-WP20-PLAN-V2 - Relation-Aware Judgment Analytics

## Goal

Implement WP-20 by extending the existing semantic judgment bundle with readable relation context and grouped judgment analytics, without changing candidate generation, destination writes, or no-human trust gates.

## Architecture

The semantic run reader already loads candidates and relations. `JudgeSemanticCandidates` will pass those relation records into judgment candidate construction instead of discarding them. The judgment bundle remains the single persistence model; CLI pages, UI state, and reports render the same enriched candidate and summary data.

## Task 1 - Add Relation Context Types And Analytics Fields

Files:

- Modify `internal/documents/types.go`
- Modify validation helpers in `internal/documents/semantic_judgment.go`

Steps:

1. Add `SemanticJudgmentRelationContext` with relation id, relationship type, endpoints, confidence, review status, evidence nodes, blockers, and review hint.
2. Add `SemanticJudgmentEndpointContext` with endpoint id, endpoint type, role relative to the current candidate, label, summary, unavailable, and unavailable reason.
3. Add `RelationContext []SemanticJudgmentRelationContext` to `SemanticJudgmentCandidate`.
4. Add grouped analytics maps to `SemanticJudgmentSummary`:
   - `JudgmentByCandidateKind map[SemanticCandidateKind]map[SemanticJudgmentChoice]int`
   - `JudgmentByConfidence map[Confidence]map[SemanticJudgmentChoice]int`
   - `JudgmentByReviewStatus map[ReviewStatus]map[SemanticJudgmentChoice]int`
   - `JudgmentBySourceDocument map[string]map[SemanticJudgmentChoice]int`
   - `JudgmentByRelationPresence map[string]map[SemanticJudgmentChoice]int`
   - `JudgmentByRelationType map[SemanticRelationshipType]map[SemanticJudgmentChoice]int`
5. Update validation so every relation-context and endpoint-context field participates in private/governance marker scans.
6. Keep fields additive-compatible; do not rename existing fields.

Tests:

- Add a focused unit test that building a judgment summary with judged candidates populates grouped analytics.
- Add a validation test that unsafe relation context or unsafe endpoint context is rejected.

## Task 2 - Build Relation Context From Semantic Runs

Files:

- Modify `internal/documents/semantic_judgment.go`
- Use existing relation loading from semantic input.

Steps:

1. Add a judgment-owned semantic input loader for `documents judge`.
2. Keep summary and candidate loading strict.
3. Load available relation records for candidate `relation_ids` into a relation lookup.
4. Do not change `readSemanticAcceptanceInput` behavior used by acceptance/calibration.
5. For each candidate relation id, attach a relation context record when the relation exists.
6. Preserve missing relation ids in the existing `relation_ids` field; do not fabricate relation context for missing files.
7. Build bounded other-endpoint context from already loaded candidates or observations when available. Do not render a full related candidate body.
8. If endpoint context is unavailable, store an explicit unavailable reason.
9. Generate deterministic review hints by relationship type.

Tests:

- Add a test that a fixture semantic run with relation files creates judgment candidates with relation context.
- Assert missing relation records do not crash `documents judge`; raw ids remain visible and relation context is not fabricated.
- Assert the strict acceptance/calibration shared input still fails on missing relation files.
- Assert relation context includes compact other-endpoint context when the related endpoint is an already loaded candidate or observation.

## Task 3 - Render Relation Context In CLI Pages And Reports

Files:

- Modify `internal/documents/semantic_judgment.go`
- Modify `internal/documents/semantic_judgment_writer.go`

Steps:

1. Add a `## Relation context` section to `semanticJudgmentPageMarkdown`.
2. For each relation context item, render relation type, id, endpoints, other endpoint label/summary or unavailable reason, confidence, review status, evidence nodes, blockers, and review hint.
3. Keep the existing `Relation ids` fallback when relation context is empty or incomplete.
4. Extend `semanticJudgmentReportMarkdown` with grouped analytics sections.
5. Keep report wording explicit that analytics are calibration evidence only.

Tests:

- Add/extend tests asserting `judge-next` page markdown contains relation type and review hint.
- Add/extend tests asserting `judge-next` page markdown contains other-endpoint label/summary or explicit unavailable reason.
- Add/extend tests asserting `judgment-report.md` contains grouped analytics after a judgment is recorded.

## Task 4 - Render Relation Context In The Local UI

Files:

- Modify `internal/cli/semantic_judgment_ui.go`
- Modify `internal/cli/documents_decompose_test.go`

Steps:

1. Add relation card styles in the existing inline UI template.
2. Render `item.relation_context` as relation cards in the current candidate body.
3. Include compact other-endpoint context or explicit unavailable reason in each card.
4. Keep the one-current-candidate body rule; do not render related candidate bodies.
5. Keep raw relation ids available as secondary metadata.
6. Add lightweight analytics context in the UI state if already present in `summary`; no new endpoint.
7. Preserve existing STD-19 protections and tests.

Tests:

- Extend `TestDocumentsJudgeServeStateAndRecord` to assert the UI HTML supports relation context and `/api/state` includes relation context for the current fixture item.
- Assert `/api/state` exposes compact endpoint context or unavailable endpoint context without exposing a second candidate body.
- Existing host/token/same-origin/non-JSON tests must keep passing.

## Task 5 - Product Brain Materialization And Audit

Files:

- Product Brain entries only through `pb` commands.

Steps:

1. Capture the signed spec/plan decision.
2. Create or update the durable WP-20 work package from the signed spec.
3. Relate WP-20 to WP-18, STD-19, STD-18, STD-17, ARCH-1, DEC-64, DOMAIN-1, and WP-19 as appropriate.
4. Run available PB audit/gate commands.
5. Reconcile any audit blockers without changing signed meaning.

## Task 6 - Verification On Fixtures And Temp Corpus

Commands:

```sh
go test -count=1 ./internal/documents ./internal/cli
go test -count=1 ./...
git diff --check
```

Temp verification:

1. For every direct `temp/*.md`, run `documents semantics` with LLM classifier if API credentials are available; otherwise use committed fixture validation and report API-key blocker truthfully.
2. Initialize a judgment bundle.
3. Verify `/api/state` or `judge-next` exposes relation context when relation-producing runs exist.
4. Record at least one smoke judgment for candidate-producing runs.
5. Capture only aggregate counts; do not commit private temp-derived outputs.

## Task 7 - Review And PR Readiness

Steps:

1. Run implementation review with at least Chain/Spec compliance and Delivery Quality perspectives.
2. Run three independent zero-context LLM judges using sanitized evidence only.
3. Fix blockers and rerun affected/full panels as required by LOOP.
4. Capture closeout decisions/insights in PB.
5. Commit, push, and open a ready-for-review PR only after verification passes.

## Exclusions Rechecked

- No destination writes.
- No Product Brain apply client.
- No classifier/prompt tuning.
- No no-human claim.
- No private temp artifacts committed.
- No alternate UI persistence.
