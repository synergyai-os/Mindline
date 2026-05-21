# WP-9 Product Brain Workspace Profile and Proposal Adapter Implementation Plan

Plan version: `MINDLINE-WP9-PLAN-V3`
Date: 2026-05-21
Status: Signed for Plan Ready in Product Brain via `DEC-21` and shaped `WP-9`. Delivery authority still requires the implementation preflight in this plan.

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:test-driven-development` before implementation, then `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a local-only Product Brain workspace profile contract and proposal adapter so Mindline can produce safe, deterministic Product Brain write proposals from WP-8 run/review output without assuming any workspace's collections.

**Architecture:** Add `internal/productbrain` as a proposal-only adapter layer over existing WP-8 output. The adapter parses a local workspace profile fixture, resolves semantic intents against profile metadata and mappings, then writes deterministic proposal JSON/previews under `--out`. It must not import Product-OS code or call live APIs.

**Tech Stack:** Go, standard library JSON/path/hash/time APIs, existing pipeline run/review artifacts, existing artifact writer patterns, local fixtures.

---

## Files and Responsibilities

- Create `internal/productbrain/profile.go`: profile structs, validation, fingerprinting, supported schema version.
- Create `internal/productbrain/profile_test.go`: unsupported version, unknown collection/field mapping, required field, platform-only, custom profile coverage.
- Create `internal/productbrain/proposal.go`: proposal structs, deterministic IDs, `externalRef`, `idempotencyKey`, actor/provenance, status model.
- Create `internal/productbrain/proposal_test.go`: deterministic identity, `externalRef` vs `idempotencyKey`, safe IDs, ready/blocked cases.
- Create `internal/productbrain/resolver.go`: semantic intent resolution against profile mappings and collection metadata.
- Create `internal/productbrain/resolver_test.go`: default profile, custom workspace profile, missing mapping, missing required field, platform-only refusal.
- Create `internal/productbrain/writer.go`: safe proposal summary, item JSON, and markdown preview writer under `productbrain-proposals/**`.
- Create `internal/productbrain/writer_test.go`: containment, no-leak scan, deterministic output, existing-output behavior aligned with current artifact writer standards.
- Modify `internal/cli/runner.go` or equivalent CLI command registration: add local command for proposal generation.
- Create or modify CLI tests for `mindline product-brain propose <run-dir> --profile <profile.json> --out <dir>`.
- Add fixtures under `testdata/productbrain/profiles/default-governance.json` and `testdata/productbrain/profiles/custom-workspace.json`.
- Add proposal input fixture using existing WP-8 run/review output style under `testdata/productbrain/runs/`.
- Modify `README.md`: document profile schema, command usage, proposal artifact layout, and dry-run/no-live boundaries.

## Task 0: Preflight

- [ ] **Step 1: Verify Product Brain profile**

Run:

```bash
pb profile list
```

Expected: JSON contains `"activeSource":"local"` and `"active":"randy-s-pkm"`.

- [ ] **Step 2: Verify signed delivery authority**

Run:

```bash
pb get WP-9
```

Expected:

- `WP-9` exists and names the Product Brain workspace profile/proposal adapter work;
- `WP-9` is shaped, not shipped;
- `WP-9` references the signed spec and plan files;
- exclusions include no live Product Brain writes, no Product-OS edits, no live APIs, and no hardcoded Randy workspace collections.

If this is not true, stop and reconcile Product Brain before code edits.

- [ ] **Step 3: Verify branch base and WP-8 artifacts**

Run:

```bash
git branch --show-current
test -f internal/pipeline/runner.go
test -f internal/pipeline/runs/ledger.go
test -f internal/pipeline/runs/ledger_test.go
```

Expected: all file checks pass. If WP-8 files are missing, stop and reconcile base.

- [ ] **Step 4: Run baseline tests**

Run:

```bash
go test -count=1 ./...
```

Expected: PASS before WP-9 edits.

## Task 1: Product Brain Workspace Profile Contract

- [ ] **Step 1: Write failing profile validation tests**

Create `internal/productbrain/profile_test.go` with table tests for:

- valid default governance profile;
- valid custom workspace profile;
- unsupported `schema_version`;
- mapping to unknown collection;
- field map to unknown field;
- missing workspace identity;
- no collections;
- platform-only collection target behavior is exposed to resolver.

Run:

```bash
go test ./internal/productbrain -run TestProfile -count=1
```

Expected: FAIL because package/types do not exist.

- [ ] **Step 2: Implement profile structs and validation**

Create `internal/productbrain/profile.go` with:

- `SupportedProfileSchemaVersion = "productbrain-workspace-profile/v0.1"`;
- workspace identity;
- kernel contract flags;
- collection metadata;
- field metadata;
- intent mappings;
- `ValidateProfile(Profile) error`;
- `ProfileFingerprint(Profile) string` using canonical JSON ordering where practical.

- [ ] **Step 3: Add profile fixtures**

Create:

```text
testdata/productbrain/profiles/default-governance.json
testdata/productbrain/profiles/custom-workspace.json
```

The custom fixture must not use `decisions`, `standards`, `tensions`, or `work-packages` as the successful target collection slugs. It should prove the adapter follows profile mappings, not Randy's defaults.

## Task 2: Proposal Identity and Contract

- [ ] **Step 1: Write failing proposal identity tests**

Test:

- proposal IDs are deterministic;
- proposal IDs and paths are path-safe;
- `externalRef.source == "mindline"`;
- `externalRef.id` represents source/object identity;
- `idempotencyKey` represents proposal/retry identity and is not equal to `externalRef.id`;
- `idempotencyKey` contains the exact schema-qualified suffix `productbrain-proposal/v0.1`;
- actor is `{kind:"integration", authority:"mindline"}`;
- provenance surface is `integration` and capture path is `integration:mindline`;
- authority IDs are `["PROD-1","DOMAIN-1","DEC-15","WP-8","WP-9"]`.

Run:

```bash
go test ./internal/productbrain -run TestProposalIdentity -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 2: Implement proposal structs and deterministic identity**

Create `internal/productbrain/proposal.go` with:

- summary schema;
- proposal schema;
- operation schema;
- blocker schema;
- `BuildProposalID(runID, reviewItemID, intent, targetCollectionSlug string) string`;
- `BuildExternalRef(runID, reviewItemID, intent string) ExternalRef`;
- `BuildIdempotencyKey(runID, proposalID string) string`, returning `mindline:proposal:<run_id>:<proposal_id>:productbrain-proposal/v0.1`.

## Task 3: Intent Resolver

- [ ] **Step 1: Write failing resolver tests**

Cover:

- default profile maps `durable_decision` to its configured target;
- custom profile maps the same intent to a differently named collection/field;
- missing intent mapping returns `blocked`;
- missing required target field returns `blocked`;
- `platform_only` target returns `blocked`;
- low-confidence or `no_product_brain_write` intent returns no ready proposal.

Run:

```bash
go test ./internal/productbrain -run TestResolve -count=1
```

Expected: FAIL before resolver exists.

- [ ] **Step 2: Implement resolver**

Create `internal/productbrain/resolver.go`:

- read WP-8 review item safe title/summary/blockers as candidate input;
- assign semantic intent using a narrow deterministic rule set or explicit test fixture intent field;
- resolve target collection through profile `intent_mappings`;
- populate target fields only from safe review/ledger fields;
- return blocked proposals when confidence, required fields, or governance constraints fail.

Do not add LLM calls. Do not use live Product Brain APIs.

## Task 4: Proposal Writer

- [ ] **Step 1: Write failing writer tests**

Cover:

- writes `productbrain-proposals/proposal-summary.json`;
- writes one JSON item per proposal under `productbrain-proposals/proposals/`;
- writes one markdown preview per proposal under `productbrain-proposals/previews/`;
- output paths are relative and contained under `--out`;
- existing output behavior matches the current artifact writer safety pattern;
- no raw URLs, private sentinels, token-like strings, `..`, or path separators from native IDs leak into JSON, previews, stdout, or stderr.

- [ ] **Step 2: Implement writer**

Create `internal/productbrain/writer.go` using existing safe path and artifact writer patterns where possible. Keep all proposal writes under the explicit output directory.

## Task 5: CLI Command

- [ ] **Step 1: Write failing CLI test**

Expected command shape:

```bash
mindline product-brain propose <run-dir> --profile <profile.json> --out <dir>
```

The command must:

- require `run-dir`, `--profile`, and `--out`;
- fail if `run-dir` lacks WP-8 ledger/review artifacts;
- fail if the profile is invalid;
- print a concise summary with ready/blocked counts and proposal summary path;
- never print private/secret source content.

- [ ] **Step 2: Wire command**

Add CLI routing in the existing command runner without changing `pipeline dry-run` behavior.

## Task 6: Documentation and Static Guards

- [ ] **Step 1: Update README**

Document:

- profile schema purpose;
- fixture examples;
- command usage;
- output layout;
- no-live/no-write boundary;
- how future live Product Brain application should use `externalRef`, `idempotencyKey`, actor authority, and provenance.

- [ ] **Step 2: Run no-live/no-import scans**

Run targeted scans such as:

```bash
rg "Product-OS|convex|pb profile|pb capture|http://|https://" internal/productbrain internal/cli README.md testdata/productbrain
```

Expected: no implementation dependency or live call. Fixture URLs must be absent or explicitly safe examples outside generated outputs.

## Task 7: Verification

- [ ] Run:

```bash
go test -count=1 ./...
```

Expected: PASS.

- [ ] Run default fixture command:

```bash
mindline product-brain propose testdata/productbrain/runs/reviewable --profile testdata/productbrain/profiles/default-governance.json --out /tmp/mindline-wp9-default
```

Expected: writes proposal summary with at least one ready and one blocked proposal.

- [ ] Run custom fixture command:

```bash
mindline product-brain propose testdata/productbrain/runs/reviewable --profile testdata/productbrain/profiles/custom-workspace.json --out /tmp/mindline-wp9-custom
```

Expected: ready proposal targets the custom collection slug and custom field keys from the profile.

- [ ] Run no-leak scans over both output directories:

```bash
rg "PRIVATE_DM_SENTINEL_DO_NOT_WRITE|sk-test|https?://|\\.\\.|/private|token" /tmp/mindline-wp9-default /tmp/mindline-wp9-custom
```

Expected: no matches.

## Task 8: Product Brain Closeout

- [ ] Capture implementation result in Product Brain only after verification passes.
- [ ] Update `WP-9` to `shipped` and `validated-staging` only after the code lands and verification evidence is recorded.
- [ ] Capture any reusable integration/kernel lessons as `DEC`, `STD`, `INS`, or `TEN` entries instead of leaving them only in PR text.

## Implementation Constraints

- No live Product Brain writes in WP-9 runtime code.
- No Product-OS imports or code edits.
- No hardcoded Randy collection slugs as successful targets outside fixtures/tests.
- No LLM/network calls.
- No private/secret source content in paths, JSON, previews, stdout, or stderr.
- Use profile metadata and explicit mappings as the adapter authority.
- Keep proposal artifacts deterministic.

## LOOP Gate Evidence

Implementation planning gate: PASS for Plan Ready. The spec is signed, captured/linked by `DEC-21`, `WP-9` is materialized from the signed spec, and expert reviewer sign-off passed for V3.

Delivery authority gate: NOT APPLICABLE for this run. Stop mode is Plan Ready; implementation must not begin in this loop.
