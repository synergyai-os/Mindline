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

- WP-46 implements DEC-414 and DEC-416 and is governed by STD-20, STD-21, STD-22, PRI-1, and BR-1.
- DEC-416 makes `external_slack_inventory/v2`, normalized and orchestration inventory `v0.2`, and activation state `v0.3` a deliberate no-migration boundary. Pre-STD-20 projections require quarantine and a native-source rebuild.
- DEC-415 governs only the evidence-backed setup-projection exception.
- TEN-26 and DEC-64 constrain WP-46.
- WP-45 commit `449008a` is the pinned foundation; WP-45 remains separately `building` pending Randy review, temporary-key retirement, and private-root cleanup.
- Product Brain global authority-domain cutover remains `activeSource: legacy`; global setup remains incomplete; neither is claimed as ready.
- WP-46 is active/verified and `pb audit WP-46 --phase handoff --verbose` passes 18/18.

Outcome coupling:

- KEY-11: trusted activation completion; target one founder run with 100% occurrence/sample accounting; verification scheduled 2026-08-13.
- KEY-12: one private-runtime finding was blocked before browser activation; zero outbound exposure is confirmed; health remains `at_risk` until a rebuilt manifest and fresh full gate pass.

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
| Occurrence accounting | source-record/occurrence/canonical invariant tests and full live denominator | full native denominator reached 1,090 source records and 1,058 extracted URL occurrences; the superseded manifest is non-authoritative; content-free rebuilt denominator pending |
| Session credentials | lease/revoke/reconnect plus success/error/restart sentinel scans | automated lease/revoke/restart and sentinel proof passed; live disconnect pending |
| Retrieval safety | Slack pagination and SSRF/rebinding/redirect/budget adversarial suites | automated broker, Slack pagination/restart, content-free checkpoint, and durable-budget suites passed; live scenario proof pending |
| Processor isolation | prompt-injection, closed schema, evidence-ref, no-authority tests | automated proof passed, including explicit manual-retrieval non-promotion |
| Routing compatibility | WP-45 golden transformation through `routing.CompileGraph` | automated compatibility proof passed |
| Product Brain approval | one-time human evidence, v0.2 attempt/cancel/crash/readback/replay tests | automated proof passed, including per-attempt expiry and atomic cancel/reserve ordering; live destination proof pending |
| Hardened UI | Host/peer/Origin/session/CSRF/multipart/XSS/timeouts/hostile-port tests and browser smoke | automated server/adversarial proof passed; current-commit browser smoke pending |
| Pre-live gate | commit/config-bound tests, race, vet, govulncheck, gosec, gitleaks, sentinel, browser, crash/replay receipt | first clean-commit gate passed; private-runtime rescan then correctly blocked one secret-bearing URL occurrence; remediation and fresh gate pending |
| Founder proof | browser Configure -> Prove -> Review; conditional exact draft delivery; usefulness/burden review | pending |
| Remainder boundary | full frozen inventory selected/unselected-unprocessed projection; no remainder retrieval/delivery | automated capped-selection/queue/readiness proof passed; live full-denominator projection pending |
| Eval/claims | readback and proof gates; blocked safety/generalization/improvement/DEC-64 claims | pending |
| Close | post-live rerun, two clean reviews, PB reconciliation, key lifecycle evidence | pending |

Current remediation-tree verification (`WP-46-redaction-v10`; source-diff SHA-256 `e9ab94026287856a0a4101e5ca90ab646d6de5711d4dcc6d1049a762371aa805`):

- Focused acquisition, routing, orchestration, retrieval, processing, activation, UI, and Product Brain suites: pass.
- `go test ./...`: pass on Go 1.26.5.
- `go vet ./...`: pass on Go 1.26.5.
- `go test -race ./...`: pass on Go 1.26.5.
- `govulncheck ./...`: pass; no vulnerabilities found.
- `gosec` high-severity/high-confidence policy: pass.
- Clean immutable commit and its fixed clean-HEAD/history/runtime scanner receipt: pending.
- `git diff --check`: pass.
- Final `WP-46-redaction-v10` panel: Domain/User Job Product SIGN-OFF; Systems Architect SIGN-OFF; Delivery Quality/Integration SIGN-OFF; Risk/Safety SIGN-OFF; Chain Steward SIGN-OFF. Every role reproduced or accepted the same source-diff SHA-256 above; no source change followed review.
- Fresh pinned scanner receipt and founder browser evidence remain pending.

## Fail-closed private-runtime finding

- The complete native Slack handoff proved 1,090 native source records and 1,058 extracted URL occurrences across the exhausted source window.
- A fresh fixed-gate runtime scan blocked one secret-bearing URL query value before the Product Brain key was entered and before any destination operation existed.
- The superseded private manifests and receipt are non-authoritative and must remain outside the active runtime surface; no occurrence may be deleted to obtain a pass.
- STD-20 v3 now requires a deny-by-default in-memory URL decision. Every lexical HTTP(S) token reaches the policy. Userinfo, fragments, secret-shaped host/path/value components, malformed or ambiguous serialization, encoded query-key aliases, and non-allowlisted parameters produce a content-free occurrence keyed only by source record plus ordinal. Related URLs cross the same policy. They retain denominator and manual-review accounting while carrying no URL, URL-derived digest, retrieval authority, routing graph node, or destination authority.
- Source content fingerprints replace every lexical URL with its ordinal placeholder before hashing. Changing only a withheld URL changes no persisted source fingerprint, occurrence identity, or item identity.
- Regression proof covers lexical denominator preservation, ambiguous and encoded query parsing, exact provider route/key/cardinality, host/path/fragment/value markers, unsafe related links, complete redacted accounting, v2 import and pre-STD-20 rebuild rejection, content-free recovery/UI source references, no retrieval attempt, rejected promotion, and zero Product Brain operations.
- The Chain records the finding on WP-46, KEY-12, FEAT-21, and FEAT-25. STD-20 v3 and DEC-416 encode the final policy and schema boundary; WP-46 is related `governed_by` STD-20 and `implements` DEC-416. Runtime validation remains pending.

## Defect history retained in the contract

- Activation success and value observation are separate; zero drafts are truthful success without value.
- Samples are sealed before retrieval and never rerolled/refilled.
- External inventory has separate source records, URL occurrences, and canonical items.
- Product Brain approved delivery uses v0.2 durable state; v0.1 stays isolated and read-only for activation.
- Session/CSRF capabilities stay in JS memory, not loopback-host cookies.
- External inventory v2, normalized/orchestration inventory v0.2, and activation state v0.3 are a deliberate no-migration schema boundary; old manifests, mutable state, and recovered authority projections fail explicitly with a native-rebuild requirement.
- Mutation attempts are durably reserved before every send and cancellation is enforced inside Product Brain authority.
- Approval expiry is revalidated immediately before every new reservation; cancellation creation and cancellation-check/reservation share a short authority lock that is released before destination I/O.
- Human approval uses one-time server-derived initiation evidence; no actor string, CLI, processor, or agent can approve.
- Private source data, real credentials, and real transports remain unreachable until the pre-live receipt passes.
- Slack restart checkpoints contain only exact scope, non-secret fingerprints, and counts. Raw messages, URLs, response bodies, credentials, and provider cursors are never checkpointed; cumulative fail-safe budgets survive restart and progress clears only after inventory adoption.
- Secret-bearing or structurally ambiguous URLs remain counted as content-free `sensitive_redacted` manual items. They are excluded from retrieval and the strict routing graph after complete review coverage is validated, so even an attempted promote decision cannot create a destination operation.
