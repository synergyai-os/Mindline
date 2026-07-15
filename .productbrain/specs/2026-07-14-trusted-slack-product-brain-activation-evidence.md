# Evidence Ledger: WP-46 Trusted Slack Activation and Gated Product Brain Drain

**Date:** 2026-07-14  
**Work package:** WP-46  
**Branch:** `codex/slack-strategic-routing`  
**Claim boundary:** private, deterministic-sample, operator-assisted founder proof; no full-remainder quality, held-out, improvement, generalization, production, or no-human claim.

## Signed authority

| Artifact | Version | SHA-256 | Exact-hash reviewers |
|---|---:|---|---|
| Activation Shape | draft-7 | `dbe4c6163374e3cb9540ea692b9bb19281f137b76544a07d5858a76f455b706b` | Product PASS; Architecture/Chain PASS; Risk/Safety PASS |
| Activation Spec | draft-4 | `b16ec7eb62618427b8b74b9b65d58435442a4212b35584ad81bbcadaeeee6f29` | Product PASS; Architecture/Chain PASS; Risk/Safety PASS |
| Activation Plan | draft-3 | `c7d290eefb30eb8dd9ffe7cbdfd15f88e1ee2e4070e74a9ba8c99616a852e59a` | Product PASS; Architecture/Chain PASS; Risk/Safety PASS |

Chain authority and limitations:

- WP-46 implements DEC-414 and is governed by STD-21, STD-22, PRI-1, and BR-1.
- DEC-415 governs only the evidence-backed setup-projection exception.
- TEN-26 and DEC-64 constrain WP-46.
- WP-45 commit `449008a` is the pinned foundation; WP-45 remains separately `building` pending Randy review, temporary-key retirement, and private-root cleanup.
- Product Brain global authority-domain cutover remains `activeSource: legacy`; global setup remains incomplete; neither is claimed as ready.
- WP-46 is active/verified and `pb audit WP-46 --phase handoff --verbose` passes 18/18.

Outcome coupling:

- KEY-11: trusted activation completion; target one founder run with 100% occurrence/sample accounting; verification scheduled 2026-08-13.
- KEY-12: zero critical private-activation safety/authority violations.

Feature elements:

- FEAT-20 activation core and durable run journal.
- FEAT-21 session connections and occurrence-complete Slack inventory.
- FEAT-22 retrieval, processing, and routing compatibility.
- FEAT-23 Product Brain approved-delivery authority v0.2.
- FEAT-24 hardened activation browser and founder review.
- FEAT-25 pre-live DevSecOps and bounded founder proof.

## Baseline

- WP-45 foundation commit: `449008a`.
- Instruction-projection commit: `fb4afc2`.
- Bounded foundation gates previously passed: full Go tests, vet, diff check, targeted race, two clean reviewer rounds.
- Source-access spike inventory observed 1,043 URL occurrences, 1,029 canonical URLs, 44 structural formats and 52 representative adjudication scenarios. The existing sanitized canonical-only inventory is not occurrence-complete and cannot satisfy WP-46 inventory readiness.
- All implementation slices before the commit-bound pre-live receipt are synthetic/sentinel-only.

## Acceptance-to-evidence ledger

| Contract | Required executable evidence | State |
|---|---|---|
| Pure run authority | aggregate, transition, drift, deterministic sample, readiness tests | automated local proof passed; clean-commit gate pending |
| Occurrence accounting | source-record/occurrence/canonical invariant tests and full live denominator | synthetic and import invariants passed; full live denominator pending |
| Session credentials | lease/revoke/reconnect plus success/error/restart sentinel scans | automated lease/revoke/restart and sentinel proof passed; live disconnect pending |
| Retrieval safety | Slack pagination and SSRF/rebinding/redirect/budget adversarial suites | automated broker, Slack pagination/restart, content-free checkpoint, and durable-budget suites passed; live scenario proof pending |
| Processor isolation | prompt-injection, closed schema, evidence-ref, no-authority tests | automated proof passed, including explicit manual-retrieval non-promotion |
| Routing compatibility | WP-45 golden transformation through `routing.CompileGraph` | automated compatibility proof passed |
| Product Brain approval | one-time human evidence, v0.2 attempt/cancel/crash/readback/replay tests | automated proof passed, including per-attempt expiry and atomic cancel/reserve ordering; live destination proof pending |
| Hardened UI | Host/peer/Origin/session/CSRF/multipart/XSS/timeouts/hostile-port tests and browser smoke | automated server/adversarial proof passed; current-commit browser smoke pending |
| Pre-live gate | commit/config-bound tests, race, vet, govulncheck, gosec, gitleaks, sentinel, browser, crash/replay receipt | pending |
| Founder proof | browser Configure -> Prove -> Review; conditional exact draft delivery; usefulness/burden review | pending |
| Remainder boundary | full frozen inventory selected/unselected-unprocessed projection; no remainder retrieval/delivery | automated capped-selection/queue/readiness proof passed; live full-denominator projection pending |
| Eval/claims | readback and proof gates; blocked safety/generalization/improvement/DEC-64 claims | pending |
| Close | post-live rerun, two clean reviews, PB reconciliation, key lifecycle evidence | pending |

Current uncommitted-tree verification:

- `go test ./...`: pass.
- `go vet ./...`: pass.
- `go test -race ./...`: pass.
- `git diff --check`: pass.
- Product/UX, Architecture/Chain authority, and Risk/Safety defect-driven re-reviews: pass; no implementation blocker.
- Clean immutable commit, pinned scanner receipt, and founder browser evidence remain pending.

## Defect history retained in the contract

- Activation success and value observation are separate; zero drafts are truthful success without value.
- Samples are sealed before retrieval and never rerolled/refilled.
- External inventory has separate source records, URL occurrences, and canonical items.
- Product Brain approved delivery uses v0.2 durable state; v0.1 stays isolated and read-only for activation.
- Session/CSRF capabilities stay in JS memory, not loopback-host cookies.
- Mutation attempts are durably reserved before every send and cancellation is enforced inside Product Brain authority.
- Approval expiry is revalidated immediately before every new reservation; cancellation creation and cancellation-check/reservation share a short authority lock that is released before destination I/O.
- Human approval uses one-time server-derived initiation evidence; no actor string, CLI, processor, or agent can approve.
- Private source data, real credentials, and real transports remain unreachable until the pre-live receipt passes.
- Slack restart checkpoints contain only exact scope, non-secret fingerprints, and counts. Raw messages, URLs, response bodies, credentials, and provider cursors are never checkpointed; cumulative fail-safe budgets survive restart and progress clears only after inventory adoption.
