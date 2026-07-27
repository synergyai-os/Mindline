# Plan: Trusted Slack-to-Product-Brain Activation and Gated Drain

**Status:** draft for role-panel review  
**Phase:** Plan  
**Version:** draft-3  
**Date:** 2026-07-14  
**Branch:** `codex/slack-strategic-routing`  
**Signed Shape:** `dbe4c6163374e3cb9540ea692b9bb19281f137b76544a07d5858a76f455b706b`  
**Signed Spec:** `b16ec7eb62618427b8b74b9b65d58435442a4212b35584ad81bbcadaeeee6f29`  
**Foundation:** WP-45 commit `449008a`; instruction projection commit `fb4afc2`

## Delivery rule

Each slice must leave tests green, preserve v0.1 compatibility, append evidence to the successor work package, and stop on a privacy, authority, approval, destination-identity, accounting, or replay defect. No slice may infer product success from command exit alone.

Slices 1–5 accept synthetic and sentinel fixtures only. They must reject Randy/private Slack payloads, any non-synthetic source item, real source/destination credentials, and real destination transports. Those entry points remain unreachable until Slice 6 seals a valid receipt for the current commit and configuration.

## Slice 0 — Chain handoff and executable contract

1. Capture a successor work package with exact Chain edges: successor `implements` DEC-414; successor `governed_by` STD-21 and STD-22; DEC-415 `governs` the successor's scoped projection exception; successor `informed_by` the pinned WP-45 foundation; and TEN-26 `constrains` the successor. Bind the signed Shape/Spec/Plan hashes.
2. Record Product, Architecture, Delivery, Chain Steward, Delivery Quality, and Risk sign-off.
3. Preserve the explicit global authority-domain/setup limitations and WP-45 review/key-cleanup obligations.
4. Add an acceptance-to-test/evidence ledger before implementation.

Exit: successor WP is active/building with exact artifacts and no unresolved handoff defect.

## Slice 1 — Pure activation core and private authority

Implement test-first:

- `internal/orchestration/types.go`, `aggregate.go`, `commands.go`, `sampling.go`, `readiness.go`, `service.go`;
- `internal/runjournal/events.go`, `store.go`, `projection.go`, `lease.go`;
- bounded reads in `internal/privateio`;
- fake source/retrieval/processor/destination ports.

Tests:

- aggregate rebuild and illegal transition rejection;
- configuration/component drift requires fork;
- deterministic `min(3, canonical_count)` per stratum with no refill/reroll;
- source-record/occurrence/canonical referential accounting;
- append/fsync/hash chain, CAS, lease, tamper, mode/symlink/unknown-file, size cap, crash rebuild;
- missing/N/A readiness evidence fails closed;
- credentials and secret-like content cannot enter events/projections.

Exit: pure Configure -> Inventory -> Prove -> Review -> Drain state machines work with fakes and no network/UI/destination writes.

## Slice 2 — Connections, occurrence-complete external import, and processing baseline

Implement:

- `internal/integrations/session_registry.go` with opaque leases, TTL/revoke/cancel and identity pinning;
- `internal/acquisition/types.go` and `internal/acquisition/slack/external_import.go`;
- occurrence-complete manifest v1 strict decoder and canonical-only legacy rejection/readiness state;
- `internal/retrieval/types.go`, registry, imported-evidence adapter, retrieval completeness states;
- `internal/processing/evidence_matcher.go` and immutable operator-review records;
- `internal/processing/routingcompat/compile.go` mapping exact four v0.1 inputs into `routing.CompileGraph`.

Tests:

- import size/schema/trailing/unknown fields/fingerprint/count/orphan/duplicate/reverse-link invariants;
- one source occurrence retained per native identity/timestamp and duplicate canonical mapping;
- lease expiry/revoke/reconnect/wrong identity and sentinel non-persistence;
- deterministic evidence matcher, manual/blocked states, unresolved evidence refs, prompt-injection fixtures;
- WP-45 routing compatibility goldens and unmappable-role/no-outbox behavior.

Exit: an occurrence-complete synthetic external-export fixture freezes, selects, processes, and produces reviewable destination-neutral results entirely through application services.

## Slice 3 — Product Brain approved-delivery v0.2

Implement:

- ephemeral provider lookup on every AKI call; no cached secret; fixed no-proxy transport; revocation-aware contexts;
- `DeliveryApproval/v0.1` and approved `Run/State/History/v0.2` with dual v0.1 readers;
- exact one-time human-initiation evidence validation;
- unique-write and cumulative-attempt reservation with fsync before send;
- durable `CancelApproved`, cancellation/attempt ordering, crash/reconcile/readback/replay;
- activation destination adapter that projects verified non-secret identity into the unchanged v0.1 profile/outbox/preflight contract and calls only `DeliverApproved`.

Tests:

- legacy profile/artifacts/CLI unchanged;
- key lease revoke prevents the next call and sentinel does not escape;
- approval drift/expiry/nonce/session/batch replay/actor injection/automatic approval rejection;
- attempt budget atomically consumed before every socket write;
- cancel-before-reservation forbids send, reserve-before-cancel permits only that attempt;
- crashes at intent/reservation/socket/response/journal/readback/receipt reconcile without duplicate mutation;
- zero-mutation replay and v0.1/v0.2 history isolation.

Exit: fake and test Product Brain transports prove exact draft-only approved delivery authority and recovery.

## Slice 4 — Hardened browser vertical

Implement:

- `internal/controlui/server.go`, strict command handlers, multipart import quarantine, projections, embedded external JS/CSS/templates;
- one unpredictable loopback listener, exact Host/peer/Origin, no CORS/cookie, in-memory custom-header session and CSRF capabilities;
- bounded `http.Server`, headers/CSP/no-store, safe errors;
- Connections, Strategy, Prove/Review, exact batch preview/approval, Founder Review, and Drain panels;
- `mindline activation serve --open` as a thin composition root over `ActivationService` that opens the authenticated browser bootstrap automatically. During founder proof Codex/operator starts it; Randy performs no shell or JSON orchestration. A future packaged launcher calls the same composition root.

Tests:

- Host/origin/peer/session/CSRF/content-type/body/header/trailing/timeouts/CORS/GET mutation;
- hostile listener on another port receives no capability;
- multipart part/count/size/quarantine cleanup and atomic adoption;
- source XSS/prompt strings render with `textContent`, never HTML;
- one-time review nonce and human gesture; no CLI/processor approval route;
- browser-visible zero-draft activation and founder-review state.

Exit: browser Configure -> Upload -> Strategy -> Freeze -> Prove -> Review -> Preview works with fake Product Brain and requires no JSON editing, project file path, or shell from the user.

## Slice 5 — Slack Web API and safe public retrieval

Implement:

- fixed-origin Slack Web API adapter with exact read-only methods/scopes, pagination/checkpoints, threads/replies, edits/deletes/files policy, budgets, 429/revoke states;
- central retrieval broker with public-IP resolution/pinning, peer and redirect revalidation, ports 80/443, no proxy/auth/cookies, HTTPS downgrade block, content/decompression/time budgets;
- provider/format strategy registry for the observed LinkedIn, YouTube, Spotify, GitHub, article/Substack, PDF/document, generic, authenticated/private, partial/blocked/rot scenarios;
- synthetic fixtures cover every observed spike scenario shape; any private imported spike evidence remains disabled until the pre-live gate and is always replay-labelled, while live retrieval is separately labelled.

Tests:

- multi-page Slack fixtures, cursor cycles, duplicates, threads, deleted/edited messages, file policy, 429 storm, revoke, restart;
- private/special/mapped IPs, mixed DNS, rebinding, redirects, downgrade, env proxy, slow/large/decompression fixtures;
- observed scenario registry accounting and unsupported/manual behavior.

Exit: Slack Web API passes synthetic full-connector proof; retrieval registry recognizes every observed scenario and safely handles supported/manual/blocked outcomes.

## Slice 6 — Fail-closed pre-live DevSecOps gate

Before any private Slack import, real Product Brain key entry, or real transport is reachable, run and seal a pre-live gate receipt for:

- full tests, targeted race, vet, pinned govulncheck/gosec/gitleaks including repository history;
- credential sentinel corpus across responses/errors/logs/files/journals/queue/artifacts/telemetry/browser storage/git;
- hardened browser/adversarial smoke including hostile local listener and multipart quarantine;
- complete Product Brain v0.2 approval, attempt, cancellation, crash/reconcile/readback/replay matrix;
- compatibility/readback of WP-45 and legacy CLI.

The real-key UI controls and real Product Brain destination composition root remain disabled unless the current commit/configuration has a valid gate receipt. Tool unavailability, incomplete coverage, or any unresolved high/critical/verified-secret finding blocks live proof.

Exit: an immutable, commit-bound pre-live receipt authorizes only the bounded private founder proof.

## Slice 7 — Queue denominator and live capped founder proof

1. Derive an occurrence-complete external export of the full selected Slack conversation and import it through the browser surface.
2. Paste the temporary Product Brain key only into the browser password field; verify returned disposable workspace/key identity and collection contracts.
3. Enter the two founder strategy anchors: Product Brain landscape and AI-dominant organizational/team design.
4. Freeze the full denominator; process the deterministic capped sample with no reroll/refill; operator-review every selected item.
5. If reviewed promote candidates produce a non-empty exact batch, Randy performs the browser approval ceremony, then Mindline delivers drafts, exact-readback, replays, and records usefulness/burden founder review. If there is no candidate batch, record the truthful zero-draft founder review, set `trusted_value_observed=false`, and do not reroll, refill, or invoke destination delivery.
6. Rebuild the full frozen inventory projection with selected and unselected/unprocessed remainder accounting. Do not retrieve, process, advance, or deliver the remainder. Report `READY_TO_EXPERIMENTAL_DRAIN`, `CONDITIONAL`, or `BLOCKED` separately; later remainder processing would require the recorded readiness evidence and `ConfirmExperimentalDrain`, outside this proof run.
7. Revoke/disconnect the temporary key and prove further calls fail; retain or clean the private run root according to Randy review policy.

Exit: `trusted_activation_completion` is truthfully decided; `trusted_value_observed` is separately decided; up to three per observed stratum are proven; Product Brain shows only exact-approved drafts when a batch exists; the full frozen inventory is visible and accounted for without remainder processing; no broad claim is made.

## Slice 8 — Post-live DevSecOps, eval, documentation, and Chain close

Run and record:

- rerun full tests, targeted race, vet, pinned govulncheck/gosec/gitleaks, post-live credential sentinel scans, diff check;
- rerun hardened browser smoke and crash/replay matrix against the final tree and reconcile the live v0.2 journal;
- eval readback and applicable proof gates with claim rejections;
- compatibility/readback of WP-45 and legacy CLI;
- README/operator instructions for Connections -> Strategy -> Prove -> Drain;
- Product Model Fit, Impact, lifecycle proof, evidence ledger, two clean defect-driven review rounds;
- Product Brain reconciliation of decisions, standards, tensions, learnings, successor WP, WP-45 boundary, and actual outcome.

Exit: all applicable acceptance evidence passes, remaining limitations are explicit, temporary credential lifecycle is complete, and the successor WP is ready for Randy review rather than falsely closed.

## Rollback

Disable `activation serve`; preserve the owner-only activation/Product Brain v0.2 journals for readback; leave legacy CLI and v0.1 artifacts untouched. Never reverse destination mutations automatically. Revoke the session credential and reconcile any ambiguous Product Brain operation before retry or cleanup.
