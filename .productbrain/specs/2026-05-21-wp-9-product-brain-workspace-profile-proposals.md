# WP-9 Product Brain Workspace Profile and Proposal Adapter Spec

Spec version: `MINDLINE-WP9-SPEC-V3`
Date: 2026-05-21
Status: Signed for Plan Ready in Product Brain via `DEC-21` and materialized as shaped `WP-9`.

## Product Brain Authority

- Product: `PROD-1` - Mindline.
- Product direction: `DOMAIN-1` - Product Brain is a future destination/authority consumer, not the Mindline product.
- Prior delivery: `WP-8` - local run ledger and review queue, shipped and verified.
- Method boundary: `DEC-15` - Mindline core is method-neutral; user/workspace method behavior must stay pluggable.
- Pipeline authority: `DEC-17`, `DEC-18`, `DEC-19`, `DEC-20` - WP-7/WP-8 pipeline, ledger, queue, and remediation authority.
- Architecture: `DEC-2`, `DEC-3`, `DEC-4` - Product Brain governs the system; Mindline is a headless engine with JSON adapter contracts.
- Standards: `STD-10`, `STD-11`, `STD-12`, `STD-15`.
- External design evidence, read-only: Product-OS kernel refactor docs in the local Product-OS checkout under `docs/system/kernel-refactor/`, especially kernel-owned write primitives, workspace-owned collections, `externalRef`, idempotency, actor authority, provenance, and domain events.

Runtime `authority_ids` in WP-9-owned proposal artifacts must carry the direct authority set in this order: `PROD-1`, `DOMAIN-1`, `DEC-15`, `WP-8`, `WP-9`. Product-OS kernel docs are evidence for compatibility, not Chain authority for Mindline.

## Problem

Mindline can now process local runs and derive a review queue, but it still has no safe bridge from reviewable outputs into Product Brain.

The tempting shortcut is to hardcode Randy's current Product Brain collections, such as decisions, standards, tensions, and work packages. That would be wrong. Each Product Brain workspace can define its own collections, fields, workflow statuses, instructions, quality criteria, and governance rules. A Mindline adapter that writes to "our collections" would bake one workspace into the engine and would fail the moment another workspace renames, edits, removes, or specializes those collections.

The user job is: connect Mindline to a Product Brain workspace without letting Mindline become Product Brain, and without assuming that the target workspace looks like Randy's current system-default workspace.

## Selected Approach

Create a local-only Product Brain workspace profile contract and proposal adapter.

WP-9 does not write to Product Brain. It reads WP-8 run ledger/review queue artifacts, reads a local Product Brain workspace profile JSON fixture, and writes deterministic Product Brain proposal artifacts under an explicit `--out` directory. The adapter proposes what a Product Brain kernel client would do later; it does not call Product-OS, Convex, the `pb` CLI, MCP, Slack, Tolaria, or any live API.

This makes the hard design question concrete before live integration:

1. discover or receive the target workspace's collections, fields, workflow statuses, guidance, quality criteria, and write affordances;
2. map Mindline review items to semantic write intents;
3. resolve those intents against the workspace profile, not against hardcoded collection slugs;
4. emit proposals only when required fields, governance rules, and safety constraints can be satisfied;
5. block ambiguous or unsupported cases with explainable review reasons.

## Product Model Fit

Eligibility verdict: `EXTEND`.

WP-9 extends the existing Mindline adapter contract:

- source adapters normalize external captures;
- method profiles govern processing policy;
- processors plan enrichment without live actions;
- WP-8 records run/review state;
- destination/proposal adapters consume safe run output.

The new product object is a `ProductBrainWorkspaceProfile`, paired with `ProductBrainProposal` artifacts. This is not a bespoke Randy workflow because the profile explicitly models workspace-specific collections and instructions as data.

## Scope

In scope:

- versioned `productbrain-workspace-profile/v0.1` JSON contract;
- parser and validation for workspace profile fixtures;
- two target workspace profile fixtures: a default governance-like workspace and a custom workspace with renamed/different collections and fields;
- semantic intent model for Product Brain proposals;
- resolver that maps intents through profile metadata and explicit profile mappings;
- deterministic proposal artifacts from WP-8 run ledger/review queue inputs;
- blocked proposal artifacts when required fields, collection mapping, governance, platform-only rules, or profile version constraints cannot be satisfied;
- explicit separation of `externalRef` identity and `idempotencyKey` retry/application identity;
- private/secret-safe proposal output;
- CLI entrypoint for local dry-run proposal generation;
- README documentation for profile authoring and proposal inspection.

Out of scope:

- live Product Brain writes;
- live Product-OS or Convex calls;
- direct Product-OS code changes;
- fetching live workspace profile data;
- changing Product Brain kernel schema or collection behavior;
- changing Tolaria destination behavior;
- live Slack fetching or live enrichment;
- UI/dashboard;
- auth/provider integration;
- background scheduler or daemon;
- automatic application of proposals.

## Product Brain Workspace Profile Contract

The adapter consumes one JSON file:

```json
{
  "schema_version": "productbrain-workspace-profile/v0.1",
  "workspace": {
    "external_id": "workspace-default",
    "slug": "default-governance",
    "name": "Default Governance Workspace"
  },
  "kernel_contract": {
    "supports_write_entry": true,
    "supports_upsert_by_external_ref": true,
    "supports_external_ref": true,
    "supports_idempotency_key": true,
    "supports_actor_authority": true,
    "supports_provenance": true
  },
  "collections": [
    {
      "slug": "decisions",
      "name": "Decisions",
      "purpose": "Significant decisions with rationale and context",
      "governed": true,
      "platform_only": false,
      "valid_workflow_statuses": ["pending", "active"],
      "default_workflow_status": "pending",
      "classification_signals": ["decision", "rationale", "settled choice"],
      "usage_guidance": "Use for durable choices with rationale.",
      "quality_criteria": ["has rationale", "has authority"],
      "fields": [
        {
          "key": "rationale",
          "label": "Rationale",
          "type": "text",
          "required": true,
          "semantic_role": "rationale",
          "writing_guidance": "Explain why the choice was made."
        }
      ]
    }
  ],
  "intent_mappings": [
    {
      "intent": "durable_decision",
      "collection_slug": "decisions",
      "field_map": {
        "rationale": "rationale"
      }
    }
  ]
}
```

Profile validation must fail closed when:

- `schema_version` is unsupported;
- workspace identity is missing;
- no collections are present;
- a mapping targets an unknown collection;
- a field map targets an unknown field;
- a required mapped target field is missing from the target collection;
- a target collection is `platform_only`;
- the profile claims live-write affordances that are missing from proposal metadata needed for future kernel clients.

Runtime proposal resolution must fail closed when a required target field exists in the profile but cannot be populated from safe review/ledger input.

## Semantic Intent Model

Mindline classifies review items into proposal intents before workspace resolution:

- `durable_decision` - a settled choice with rationale;
- `operating_standard` - a reusable rule or convention;
- `open_tension` - a blocker, risk, or unresolved friction;
- `implementation_work` - bounded future work;
- `reusable_insight` - durable learning or pattern;
- `reference_note` - context worth preserving but not governance truth;
- `no_product_brain_write` - item should not become Product Brain state.

The intent is not the write target. The profile resolves the target. If the profile cannot resolve the target confidently, the proposal status is `blocked`.

## Proposal Artifact Contract

WP-9 writes:

```text
<out>/productbrain-proposals/proposal-summary.json
<out>/productbrain-proposals/proposals/<proposal_id>.json
<out>/productbrain-proposals/previews/<proposal_id>.md
```

`proposal-summary.json`:

```json
{
  "schema_version": "productbrain-proposal-summary/v0.1",
  "run_id": "run-0123456789abcdef",
  "workspace_profile": {
    "schema_version": "productbrain-workspace-profile/v0.1",
    "workspace_slug": "default-governance",
    "fingerprint": "sha256:<hex>"
  },
  "proposal_count": 2,
  "blocked_count": 1,
  "proposals": [
    {
      "proposal_id": "pbp-0123456789abcdef",
      "status": "ready",
      "intent": "durable_decision",
      "target_collection_slug": "decisions",
      "proposal_path": "proposals/pbp-0123456789abcdef.json",
      "preview_path": "previews/pbp-0123456789abcdef.md"
    }
  ],
  "authority_ids": ["PROD-1", "DOMAIN-1", "DEC-15", "WP-8", "WP-9"]
}
```

Proposal item:

```json
{
  "schema_version": "productbrain-proposal/v0.1",
  "proposal_id": "pbp-0123456789abcdef",
  "run_id": "run-0123456789abcdef",
  "source_review_item_id": "pipeline-text-only",
  "status": "ready",
  "intent": "durable_decision",
  "confidence": "high",
  "operation": {
    "kind": "upsert_entry_by_external_ref",
    "target_collection_slug": "decisions",
    "entry_name": "Example decision",
    "workflow_status": "pending",
    "data": {
      "rationale": "Safe rationale derived from the review item."
    }
  },
  "externalRef": {
    "source": "mindline",
    "id": "run-0123456789abcdef:pipeline-text-only:durable_decision"
  },
  "idempotencyKey": "mindline:proposal:run-0123456789abcdef:pbp-0123456789abcdef:productbrain-proposal/v0.1",
  "actor": {
    "kind": "integration",
    "authority": "mindline"
  },
  "provenance": {
    "surface": "integration",
    "capture_path": "integration:mindline",
    "source_run_id": "run-0123456789abcdef"
  },
  "blockers": [],
  "authority_ids": ["PROD-1", "DOMAIN-1", "DEC-15", "WP-8", "WP-9"]
}
```

Blocked proposals must include a safe `blockers[]` array and must omit any target `data` values that would leak private source content, secret-like content, raw URLs, path traversal fragments, tokens, or private author names.

## Acceptance Criteria

1. A local command reads a WP-8 run output directory plus a profile JSON and writes `productbrain-proposals/**` under an explicit `--out` directory.
2. The default governance fixture can produce at least one ready proposal without hardcoding target collection slugs outside profile data.
3. The custom workspace fixture proves renamed/different collections and fields work through profile metadata and `intent_mappings`.
4. Missing required fields produce blocked proposals with reviewable reasons.
5. `platform_only` target collections are refused.
6. Unsupported profile versions fail before proposal generation.
7. Proposal IDs, output paths, external references, and idempotency keys are deterministic and path-safe.
8. Tests prove `externalRef` and `idempotencyKey` are distinct and stable for their separate purposes, and that `idempotencyKey` contains the exact schema-qualified suffix `productbrain-proposal/v0.1`.
9. Private/secret sentinel values, raw URLs, path traversal fragments, and token-like strings do not appear in proposal JSON, previews, stdout, or stderr.
10. The implementation imports no Product-OS code and calls no live Product Brain, Convex, `pb`, Slack, Tolaria, or network APIs.
11. `go test -count=1 ./...` passes.

## Risks

- Profile overfitting: avoid by testing both default-like and custom workspace profiles.
- Premature live integration: block by keeping WP-9 proposal-only and scanning for live API/CLI calls.
- Semantic intent hardcoding: intent is only an intermediate; target collection/fields come from the profile.
- External identity confusion: keep `externalRef` for source/object identity and `idempotencyKey` for retry/application identity.
- Product-OS drift: treat the current kernel docs as compatibility evidence, not as a compile-time dependency.

## LOOP Gate Evidence

Adapter gate: PASS. Current PB profile is local `randy-s-pkm`; work-package lifecycle is known; artifact roots are `.productbrain/specs/` and `.productbrain/plans/`.

Brainstorm authority gate: PASS for Spec. The problem, scope, direction, outcome, Chain authority, constraints, stop mode, and verification expectations are explicit in this spec.

Product Model Fit gate: PASS with `EXTEND`. WP-9 extends the Mindline adapter/run-review pattern and introduces reusable profile/proposal objects.

Impact discovery gate: PASS for planning. Upstream is WP-8 run ledger/review queue; downstream is future Product Brain kernel client/application; trust boundary is proposal-only.

Spec authority gate: PASS. Expert reviewers signed V3, `DEC-21` captured the signed spec/plan, and `WP-9` was materialized from the signed spec.
