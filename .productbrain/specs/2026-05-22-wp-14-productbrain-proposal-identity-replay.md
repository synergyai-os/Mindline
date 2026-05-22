# MINDLINE-WP14-SPEC-V1: Product Brain Proposal Identity And Replay

## Authority

- Work package: `WP-14 Product Brain proposal identity acceptance fixes`.
- Diagnosis: `DEC-54` says the next big product step is a destination-neutral semantic candidate acceptance loop, and the immediate enabling PR is source-native identity/replay hardening.
- Constraint: `TEN-4` says Product Brain proposals currently risk duplicate PB entries because `externalRef.id` embeds run-scoped identity.
- Rule: `INS-10` says `externalRef.id` must be a pure function of source-native object identity while `runID` belongs in `idempotencyKey`.
- Product direction: `DOMAIN-1` says Product Brain is a future destination/authority consumer, not Mindline's product.

## Problem

Mindline Product Brain proposal artifacts currently use run-scoped identity for `externalRef.id`. The present shape derives the external reference from `runID`, review queue item ID, and intent. That is wrong for future `upsertByExternalRef` semantics because the same source object can be reprocessed in a fresh run and appear as a different PB object.

This is not a live-write bug yet because Product Brain proposals are dry-run artifacts. It is still blocking because later acceptance/evaluation, replay, batch retry, and app-integration work cannot be trusted while object identity is run-scoped.

## Product Model Fit

Eligibility: `EXTEND`.

WP-14 extends the existing source candidate, run ledger/review queue, and Product Brain proposal contracts. It does not create live Product Brain integration and does not make Product Brain the Mindline product path.

The reusable system behavior is: source-object identity and run/event idempotency are separate. Source-object identity is stable across runs; idempotency is scoped to a produced proposal event.

## Scope

In scope:

- Carry source-native candidate identity from ledger items into review queue items.
- Build Product Brain proposal `externalRef.id` from durable source identity plus semantic intent, not `runID` or review queue item path identity.
- Keep `idempotencyKey` source-prefixed and run/proposal scoped.
- Add fresh-run replay tests proving the same source object maps to the same `externalRef.id` across different run IDs while producing different idempotency keys.
- Make unknown profile semantic roles fail closed instead of silently defaulting to context.
- Document proposal artifacts as app-integration apply-time inputs, not trusted live writes or file-drop authority.
- Add or update golden fixtures needed for the changed review queue/proposal schema.

Out of scope:

- Live Product Brain writes.
- Product Brain app-integration transport implementation.
- Kernel capability probing.
- Monotonic source version enforcement beyond preserving available source-native identity fields.
- Partial-batch apply machinery.
- Semantic candidate quality/classification harness.
- Live Slack fetching or daemon behavior.
- Broad source identity remodel across all document/semantic artifacts unless required for this proposal path.

## Contract

### Source Identity

Review queue items must include a stable source identity field derived from the ledger item's `source_candidate_id`. For Slack, this ultimately comes from source-native channel/timestamp identity. Unsafe source identities remain sanitized before persistence.

### Product Brain ExternalRef

Ready and blocked/skipped Product Brain proposal artifacts must include:

- `externalRef.source = "mindline"`.
- `externalRef.id = "mindline:<safe-source-candidate-id>:<intent>"` or an equivalent safe deterministic encoding that excludes `runID`, queue item path identity, and transient output paths.

The intent remains part of the id because the same source object may produce different durable PB object classes.

### Product Brain Idempotency

`idempotencyKey` remains source-prefixed and event/run scoped. It must differ across distinct proposal production events when `runID` changes, even if `externalRef.id` is stable.

### Unknown Role Handling

Profile field-map roles must be explicit. Supported roles for this slice are:

- `title`
- `name`
- `summary`
- `rationale`

Any unknown role blocks the proposal with `unsupported_field_role` rather than silently mapping to context.

### Apply-Time Posture

Proposal artifacts remain dry-run artifacts. They are not trusted write authority. A future Product Brain app-integration client/gateway must re-stamp actor/provenance and apply `upsertByExternalRef` through authenticated transport.

## Acceptance

1. `externalRef.id` excludes `runID`.
2. `externalRef.id` excludes review queue item/path identity.
3. The same source candidate identity and intent produce the same `externalRef.id` across different runs.
4. Different runs for the same source candidate and intent produce different source-prefixed `idempotencyKey` values.
5. Review queue items persist `source_candidate_id` for review-required items.
6. Product Brain proposal generation consumes `source_candidate_id` from review queue items.
7. Unknown field-map roles produce a blocked proposal with code `unsupported_field_role`.
8. Existing proposal writer containment and redaction behavior remains intact.
9. Existing pipeline, Product Brain, and CLI tests pass.
10. No live Product Brain write path is added.

## Verification

Required commands:

```sh
go test -count=1 ./internal/pipeline/runs ./internal/productbrain ./internal/cli
go test -count=1 ./...
rg -n "upsertByExternalRef\\(|writeEntry\\(|convex|net/http|http://|https://" internal/productbrain internal/pipeline/runs
```

For the `rg` command, no matches in implementation code for live write/network behavior is the expected result. Existing test strings must be inspected if they appear.
