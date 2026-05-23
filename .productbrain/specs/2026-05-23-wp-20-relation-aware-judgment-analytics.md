# MINDLINE-WP20-SPEC-V2 - Relation-Aware Judgment Analytics

## Status

Spec V2 after LOOP review blockers. Delivery is not authorized until this spec, the paired plan, Product Brain materialization, and audit reconciliation are signed off.

## Chain Authority

- `WP-18` - shipped local semantic judgment harness and one-item review UI.
- `DEC-84` - PR #13 merged and local `main` synced.
- `DEC-83` / `STD-19` - loopback review write APIs must remain host-allowlisted, token-protected, same-origin, JSON-only, write-locked, and read-only for page/state reads.
- `STD-18` - review pages must include inline evidence excerpts, not only ids.
- `STD-17` / `ARCH-1` - LLM-backed semantics must stay provider-agnostic and measured before trust.
- `DEC-64` - no-human steady state requires held-out measured accuracy >= 98%; this work cannot claim no-human readiness.
- `DOMAIN-1` - Product Brain is a future destination/authority consumer, not the Mindline product.
- `WP-19` - Product Brain apply-time client remains later and gated on PB kernel readiness.

## Diagnose

WP-18 made candidate review possible, but the relation surface is still too low-level. A reviewer sees raw `relation_ids`, not what the relation means, what it connects, whether it is blocking, or how relation-heavy items are performing across a batch. That leaves the human mental model weak: relation-heavy candidates can be marked unclear because the UI does not explain whether a relation is `derived_from`, `contradicts`, `supersedes`, `same_topic_as`, or another review-relevant relationship.

The immediate blocker is not candidate generation, destination mapping, or Product Brain apply. It is that the existing judgment artifact model throws away loaded semantic relation records and keeps only ids. That prevents the UI and report from answering: "Can I judge this item from here?" and "What should we improve next?"

## Plain-English Goal

Make the existing one-item judgment harness relation-aware and analytically useful.

The reviewer still sees one candidate body at a time, but the item must explain its relation context in plain review terms. The batch report must also show where quality issues cluster by candidate kind, confidence, review status, source document, relation presence, relation type, and judgment choice.

## Product Model Fit

Verdict: `EXTEND`.

WP-20 extends the existing `semantic-judgment` artifact and local UI pattern from WP-18. It does not create a new review surface, new persistence model, destination mapper, or classifier pipeline. The product object is the local semantic judgment bundle. The source of truth remains `semantic-judgment/judgment-summary.json`, candidate item JSON, judgment JSON, cursor, and report artifacts.

Why this is not bespoke: relation-aware context applies to every semantic candidate run with relations, including transcripts, Notion/process docs, Slack-derived captures, and future source adapters. Batch analytics become calibration evidence for later model improvement and no-human trust gates.

## In Scope

1. Load semantic relation records when initializing a `semantic-judgment` bundle.
2. Add additive relation review context to each `SemanticJudgmentCandidate`.
3. Relation context must include relation id, relationship type, endpoint ids/types, confidence, review status, evidence nodes, blockers, and a short review hint.
4. Render relation context in `judge-next` Markdown pages.
5. Render relation context in the local review UI without showing multiple candidate bodies.
6. Preserve raw relation ids for compatibility, but make them secondary to readable context.
7. Add batch analytics to the judgment summary/report:
   - judgment counts by candidate kind;
   - judgment counts by confidence;
   - judgment counts by review status;
   - judgment counts by source document;
   - judgment counts by relation presence;
   - judgment counts by relation type.
8. Update analytics after every CLI or UI judgment.
9. Keep all changes destination-neutral and provider-agnostic.
10. Verify against committed fixtures and all eligible direct `temp/*.md` files without committing private temp outputs or judgments.
11. Use a judgment-owned tolerant relation loading path for `documents judge`; do not loosen the shared acceptance/calibration reader.

## Out Of Scope

- No Product Brain writes.
- No Tolaria writes.
- No destination policy mapping.
- No Product Brain proposal generation or apply transport.
- No auto-accept or no-human readiness claim.
- No provider-specific classifier changes.
- No prompt/classifier tuning.
- No change to semantic candidate generation.
- No answer-key or held-out calibration threshold changes.
- No committed private temp source, temp-derived run bundles, judgment bundles, or raw provider responses.
- No multi-item candidate body review surface.
- No full related candidate bodies or multi-item review. Compact endpoint labels are in scope when they can be derived from already loaded candidates or observations without showing a second candidate body.

## Data Contract

The existing `semantic-judgment-* /v0.1` schemas may remain additive-compatible. New JSON fields must be optional for older bundles and must not break existing readers.

Candidate item additions:

```json
{
  "relation_context": [
    {
      "relation_id": "rel-example",
      "relationship_type": "contradicts",
      "from_id": "cand-a",
      "from_type": "candidate",
      "to_id": "cand-b",
      "to_type": "candidate",
      "confidence": "medium",
      "review_status": "needs_review",
      "evidence_nodes": ["node-1"],
      "blockers": [],
      "other_endpoint": {
        "endpoint_id": "cand-b",
        "endpoint_type": "candidate",
        "role": "to",
        "label": "Gateway launch is already resolved",
        "summary": "The related candidate says the launch blocker was resolved in the same source.",
        "unavailable": false
      },
      "review_hint": "This candidate conflicts with another semantic object; inspect whether it is stale, resolved, or should be marked unclear/reject."
    }
  ]
}
```

Summary additions:

```json
{
  "judgment_by_candidate_kind": {
    "action_candidate": { "accept": 2, "reject": 1 }
  },
  "judgment_by_confidence": {
    "high": { "accept": 3 }
  },
  "judgment_by_review_status": {
    "ready": { "accept": 3 }
  },
  "judgment_by_source_document": {
    "doc-meeting-transcript-1": { "unclear": 1 }
  },
  "judgment_by_relation_presence": {
    "with_relations": { "unclear": 1 },
    "without_relations": { "accept": 1 }
  },
  "judgment_by_relation_type": {
    "contradicts": { "unclear": 1 }
  }
}
```

Counts include only judged candidates. Unjudged candidates remain represented by `remaining_count`.

For `judgment_by_relation_type`, a judged candidate contributes once to each distinct relation type present in its relation context. A candidate with two `derived_from` relations and one `contradicts` relation increments `derived_from` once and `contradicts` once for that judgment choice.

## UI Contract

The UI remains an operational review tool.

- It shows exactly one current candidate body.
- It keeps total/judged/remaining and judgment distribution visible.
- It renders relation context as readable relation cards, not just raw id tags.
- Each relation card must include relationship type, review hint, and bounded other-endpoint context when available: endpoint role relative to the current candidate, endpoint type/id, and a one-line label/summary from already loaded candidate or observation metadata.
- When compact endpoint context is unavailable, the card must say `endpoint context unavailable` rather than making the reviewer chase raw JSON.
- It must make relation-heavy uncertainty legible: contradiction, supersession, duplicate/same-topic, dependency, assignment, deadline, owner, and evidence derivation have different review implications.
- It must preserve STD-19 protections: loopback Host validation, token-protected writes, same-origin JSON POSTs, write lock serialization, and read-only state/page endpoints.

## Review Hints

Minimum hints:

- `derived_from`: "This is the evidence link from the candidate back to the source observation or structure."
- `contradicts`: "This candidate conflicts with another semantic object; inspect whether it is stale, resolved, or should be marked unclear/reject."
- `supersedes`: "This candidate may replace an older semantic object; check whether the older item should not be accepted as current."
- `same_topic_as`: "This candidate overlaps another semantic object; use duplicate when it repeats the same useful object."
- `depends_on`: "This candidate depends on another object; check whether it is actionable or incomplete without that dependency."
- `assigns_action`: "This relation suggests ownership or assignment; check whether the action candidate has the right owner/scope."
- `mentions_owner`: "This relation contributes owner context; check whether the candidate uses it correctly."
- `mentions_deadline`: "This relation contributes deadline context; check whether the candidate uses it correctly."
- Other known relation types get a generic "Use this relation to decide whether the candidate is supported, duplicated, stale, or incomplete."

## Relation Loading Boundary

`documents accept` and calibration must continue to use the existing strict semantic input path: a candidate that references a missing relation file is malformed and should fail there.

`documents judge` is a review surface and may tolerate missing relation files because the reviewer still needs to inspect the candidate and can see that relation context is incomplete. WP-20 must therefore add a judgment-owned tolerant relation loading path or wrapper:

- candidate and summary loading remain strict;
- available relation files are loaded and validated;
- missing relation files do not abort `documents judge`;
- missing relation ids remain visible in `relation_ids`;
- no fabricated `relation_context` is emitted for missing relation records;
- the candidate page/UI explicitly renders unavailable relation context when a relation id has no loaded relation record;
- acceptance/calibration behavior is unchanged and covered by regression tests.

## Acceptance

WP-20 is complete when:

1. Spec and plan are captured or linked on Chain and the durable work package exactly materializes them.
2. Product Brain audit/gate reconciliation has no blocking mismatch.
3. `documents judge` initializes relation-aware judgment candidates from semantic runs that include relation files.
4. `documents judge-next` Markdown includes readable relation context for candidates with relations.
5. `documents judge-serve` UI renders relation context and preserves exactly-one-current-candidate body behavior.
6. `judgment-summary.json` and `reports/judgment-report.md` include relation-aware and grouped judgment analytics.
7. Recording a CLI or UI judgment recomputes the grouped analytics deterministically except for judgment timestamp.
8. Missing relation-file behavior is split correctly: `documents judge` remains readable with raw ids and unavailable relation context, while acceptance/calibration strict input still fails on malformed relation references.
9. Older judgment bundles without relation context remain readable or fail with a clear compatibility error; no panic.
10. Validation covers safety: private/governance markers in relation context are rejected, traversal/symlink containment still holds, and STD-19 review API protections remain tested.
11. Focused and full Go tests pass.
12. Real `temp/*.md` verification runs over all eligible direct Markdown source files and proves relation context plus analytics are present where relation-producing runs exist.
13. Implementation review and three independent zero-context LLM judges sign off on the final evidence before PR submission.

## Guardrails

- Relation-aware review improves judgment quality; it does not prove candidate quality.
- Judgment analytics are calibration evidence; held-out calibration remains the only path to no-human eligibility.
- Destination mapping remains deferred.
- Product Brain apply remains `WP-19` and later-gated.
- External judge packets must exclude real temp source text, raw provider responses, and private temp-derived artifacts; use committed fixtures, diffs, test output, and aggregate temp counts.

## LOOP Diagnose/Shape Sign-Off

Output version: proposed direction v0, 2026-05-23.

- Chain Steward: `SIGN-OFF`; WP-18 is shipped, WP-19 remains later/gated, next work should be post-WP-18 relation-aware judgment analytics.
- Domain/User Job Reviewer: `SIGN-OFF`; relation ids without meaning leave reviewers unable to judge relation-heavy items confidently.
- Systems Architect / Delivery Quality Reviewer: `SIGN-OFF`; coherent smallest useful boundary is additive relation context plus grouped analytics in the existing judgment harness.

## LOOP Spec/Plan V1 Review Blockers Applied In V2

- Domain/User Job `BLOCKER`: raw endpoint ids were insufficient for the reviewer mental model. V2 requires bounded other-endpoint context or explicit unavailable context while preserving one-current-candidate body.
- Systems/Delivery `BLOCKER`: V1 mixed tolerant missing-relation behavior with the strict acceptance reader. V2 requires a judgment-owned tolerant relation loading path and explicit regression that acceptance/calibration remain strict.
