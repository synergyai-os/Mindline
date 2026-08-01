# Plan: WP-49 Evidence Deepening and Scoped Agent Recall

**Status:** candidate for role-panel review  
**Phase:** Plan  
**Version:** draft-1  
**Date:** 2026-08-01  
**Product:** Mindline  
**Shape authority:** DEC-434; commit `41c939d`  
**Spec authority:** DEC-435; commit `eb75b3f9f431e1cb87233853c671087381ee6dbc`; SHA-256 `79ee2d836e9185dfb52ad21e94866d105e2eb104317912ba207e8d0202e61c0e`  
**Work package:** WP-49  
**Feature:** FEAT-30  
**Clean candidate baseline:** commit `eb75b3f9f431e1cb87233853c671087381ee6dbc`; tree `e8076423d08deb37228cc7465a074b4a2f0c6b66`  
**Implementation authority:** none until this exact Plan passes two unchanged five-role reviews, is committed, receives signed Chain authority, and WP-49's handoff audit passes again

## 1. Delivery outcome

Deliver the smallest complete successor to the current Mindline Slack recall
prototype that proves two user outcomes together:

1. selected saved resources gain independently validated primary evidence, so a
   fresh agent can answer from the user's memory with exact citations or abstain
   honestly; and
2. one shared evidence library can be ranked differently for each user-owned
   project, lens, and stable agent without copying evidence or leaking one
   agent's judgments into another agent's view.

The same query and authorized library snapshot must expose the same base
candidate/citation set to every context. Context affects only ordering. Owner
feedback is shared only inside its exact scope/lens; agent feedback is visible
only to that exact scope/lens/agent. The complete pairwise isolation matrix must
pass live and after restart.

No default install changes before a contained fresh-agent proof and Randy's
bound `useful` verdict. Product Brain remains delivery authority and is never a
runtime dependency or destination in this Plan.

## 2. Plan challenge and reconciliation

The first sequencing challenge is to avoid building a large crawler, second
database, or bespoke Randy-only pipeline before the outcome can be measured.
This Plan resolves that by:

- freezing private transition, retrieval, and context-isolation cases before
  candidate product code exists;
- building only three public acquisition capabilities:
  `readable_body/v1`, `primary_post/v1`, and `transcript/v1`;
- retaining one canonical FileRepository and one shared evidence artifact per
  resource/capability while keeping context and feedback as reversible derived
  state;
- using the current ranking formula and weights exactly, with no tuning during
  the intervention;
- implementing one bounded public fault oracle for installation rather than
  treating quarantine as a universal success path;
- proving the candidate in a separate contained runtime before touching the
  working local install; and
- stopping automatically after a failed transition gate, failed private
  evaluator, invalid outside-agent output, `not_useful` verdict, or install
  smoke failure.

Rejected as outside authority: broad crawling, authenticated browser use,
comments acquisition, Slack OAuth/UI, Product Brain runtime transport, LocalDB
code, learned ranking, cross-agent aggregation, agent-created scopes/lenses,
and production/generalization claims.

## 3. Team topology and ownership

| Capability owner | Owns | Does not own |
| --- | --- | --- |
| Stream-aligned recall team | end-to-end Slack-memory-to-agent outcome, composition, operator CLI, staged proof | provider policy internals or Product Brain runtime |
| Platform: personal evidence | canonical library/evidence repository, coherent snapshots, recovery, stable-source reconciliation | provider extraction, ranking, UI |
| Complicated subsystem: public evidence | capability adapters, independent readiness validation, bounded request accounting | canonical promotion or user relevance |
| Platform: agent context | scopes, lenses, actors, contextual runs, append-only judgments, disclosure ledger | evidence readiness or hostile-user authentication |
| Complicated subsystem: assurance/recovery | manifests, private evaluator, disclosure controller, bootstrap/install fault oracle | product strategy or private semantic labels |
| Chain Steward and Delivery Quality | phase authority, unchanged-tree reviews, evidence ledger, closure truth | implementation shortcuts |

Dependency direction is fixed:

```text
cmd/cli/localservice composition
        │
        ├── resourcedeepening ──> resourceadapters ──> resourcefetch
        │          │                      │
        │          └──> resourcebudget <──┘
        │
        ├── personalmemory ──> evidencereadiness
        │          │
        │          └── immutable EvidenceReadSnapshot
        │                         │
        ├── agentretrieval <──────┘
        │          └──> agentstate contextual projection
        │
        └── recal/eval + recallproof + founderreview + localservice install
```

`resourcefetch` depends only on the package-neutral request-budget port.
Provider adapters cannot import FileRepository or set readiness. Retrieval,
evaluation, and staged proof receive only an immutable bounded read snapshot,
never the broad repository or a mutation port.

## 4. Change-control boundary

This Plan may refine file names, private helper types, test grouping, and
within-slice implementation order only when the signed contracts and gates are
unchanged. Renewed Spec and Chain authority is mandatory for any change to:

- user outcome, scope, exclusions, or claim boundary;
- private/public proof separation or manifest cases;
- schema, identity, commitment, readiness, ranking, feedback, or citation
  semantics;
- trust, privacy, disclosure, network, credential, or destination behavior;
- phase order, staged-before-install order, recovery preference, or quarantine
  conditions;
- founder-verdict authority; or
- Product Brain's non-runtime boundary.

Any reviewed Plan byte change restarts the full five-role review sequence.
Any defect after a frozen candidate tree invalidates every downstream candidate
hash, result, staged proof, verdict, reviewer pass, and install receipt.

## 5. Slice 0 — authority tooling before product code

### Work

1. Add strict public schemas and canonical commitment code for:
   - evidence-transition manifest;
   - private held-out retrieval manifest;
   - context-isolation manifest and its nested feedback/oracle rows;
   - public installation fault-oracle manifest; and
   - pre-build authority receipt.
2. Add one owner-local manifest tool with separate `create`, `validate`, and
   `seal` operations. `create` accepts explicitly named private inputs;
   `validate` is read-only; `seal` binds the exact clean baseline and refuses a
   dirty tree, late file, changed fingerprint, insufficient split/cardinality,
   or non-ancestor baseline.
3. Make private manifest paths explicit operator inputs and keep them beneath
   the owner-only proof/control root. No latest-directory discovery, globbing,
   environment fallback, or repository path is accepted.
4. Generate the public installation fault oracle from the closed Spec state
   enums. Validation rejects a missing or extra `before_`/`after_` fault point,
   missing state combination, duplicate row, always-quarantine implementation,
   or row/count/size excess.
5. Add structural leak scanning and typed public projections that cannot hold
   private case IDs, resource IDs, queries, labels, expected sources, actors,
   contexts, orders, paths, URLs, or excerpts.
6. Independently review the authority tooling before using it. Record the
   reviewed commit/tree and tool binary hash in an owner-private tooling receipt.
7. Using only the reviewed tool, freeze:
   - the exact six transition cases;
   - at least 20 held-out cases with at least 12 answerable and 8 no-answer;
   - 4–32 context cases with 2–8 contexts, 2–8 agents, full checkpoint rows,
     at least one different-order pair, and the full live/restart pairwise
     isolation matrix; and
   - the complete non-private installation fault oracle.
8. Seal and read back the pre-build authority receipt against baseline commit
   `eb75b3f9f431e1cb87233853c671087381ee6dbc` and tree
   `e8076423d08deb37228cc7465a074b4a2f0c6b66` before the first candidate
   product-code change.

### Likely files

- Add `internal/assurance/evidence_manifest_types.go`.
- Add `internal/assurance/evidence_manifest_validation.go`.
- Add `internal/assurance/evidence_manifest_commitments.go`.
- Add `internal/assurance/evidence_manifest_test.go`.
- Add `internal/assurance/install_fault_oracle.go` and tests.
- Add thin `cmd/mindline-evidence-authority/main.go` plus command tests.

Private manifests and receipts do not enter the repository. The public
fault-oracle schema and reusable fixtures may enter `internal/assurance` only
after leak scans prove they contain no private values.

### Gate

- Public mutation tests cover every included, excluded, nullable, sorted, and
  capped field in all five authority types.
- Tool review passes on one unchanged commit/tree through Systems Architecture,
  Delivery Quality, and Risk/Safety at minimum.
- Pre-build receipt binds exact fingerprints/counts/reviewer commitments and
  states `candidate_boundary_state=not_started`.
- Repository history and candidate worktree contain zero private manifest
  bytes, raw source values, or paths.
- Product Brain receives only structural fingerprints, counts, baseline
  commit/tree, and claim limits.
- No production package, CLI behavior, ranking behavior, evidence repository,
  or default install changes in this slice.

## 6. Slice 1 — canonical evidence and coherent reads

### Work

1. Extend `personalmemory.FileRepository` with the additive v0.1 evidence
   sidecar, owner-only directories, strict schemas, deterministic sorting,
   content-addressed candidate envelopes/artifacts, and the recovery journal.
2. Implement shared canonical commitment primitives once in a small internal
   package; packages expose typed projection functions rather than ad-hoc map
   hashing.
3. Add `evidencereadiness` with immutable capability contracts, deterministic
   validation, inaccessible mapping, quality computation, and repository-owned
   attestation assembly.
4. Implement provider-neutral bootstrap from an explicit bounded batch.
   Bootstrap can create only `unverified` heads and is exact-replay safe.
5. Implement staged candidate journaling and the single atomic
   promote/reject/inaccessible transaction with stronger-only semantics,
   predecessor retention, exact replay receipts, and the 16-revision cap.
6. Implement current/backup/recovery-journal replacement and every crash
   recovery branch before any acquisition composition.
7. Implement stable-source reconciliation with unchanged-control projection,
   at most 256 exact external receipts, deterministic interrupted-receipt
   recreation, and evidence reads blocked until reconciliation completes.
8. Implement `OpenEvidenceSnapshot` as the only retrieval/evaluation read port:
   two-phase coherent read, frozen byte/memory/time caps, at most two retries,
   immutable source documents and ready evidence, and index invalidation on
   either library or catalog fingerprint change.

### Likely files

- Add `internal/evidencecommitment/` with focused projection and mutation tests.
- Add `internal/evidencereadiness/{types,contracts,validator,attestation}.go` and
  tests.
- Add focused `internal/personalmemory/evidence_*.go` files for schema,
  repository, promotion, recovery, reconciliation, snapshot, and tests.
- Modify existing personal-memory composition only after repository tests pass.

No single implementation file may become the new repository god file. Split
schema, persistence, transition, recovery, and read-model ownership when a file
approaches the repository file-size quality limit.

### Gate

- Every valid and invalid readiness/artifact/envelope/attestation state row is
  executable.
- Exact replay adds zero head, artifact, envelope, revision, receipt, or catalog
  effect.
- All quality/readiness transitions, equal-quality rules, truncation downgrade,
  policy/generation changes, and the 17th predecessor failure pass.
- Envelope raw SHA and domain commitment cannot substitute for one another;
  any block/order/null/usage/digest/commitment/size mutation fails.
- Every current/mirror/recovery and stable-source reconciliation crash point
  resolves only to the exact acknowledged old or new authority.
- Prior v0.4 binaries remain able to read the unchanged legacy library; rollback
  touches no evidence sidecar byte.
- Snapshot mixed revisions, caps plus one, mutation-bearing dependencies, and
  stale index reuse fail closed with zero evidence disclosure.

## 7. Slice 2 — bounded acquisition, canonical budgets, and queue recovery

### Work

1. Add package-neutral `resourcebudget.RequestBudgetLedgerPort` and structural
   attempt/request reservation types.
2. Extend `resourcefetch.SafePublicFetcher` so every primary, redirect, caption,
   embed, and secondary request must reserve and read back a fresh canonical
   request ordinal before send, then settle or remain ambiguously consumed.
3. Add provider-neutral `resourceadapters` interface and three implementations:
   - semantic/readability public body;
   - public primary-post body independent of comments; and
   - public caption/transcript.
4. Reapply STD-20 classification and every fetch control to every secondary
   target. Never inherit cookies, authorization, proxy state, browser state, or
   arbitrary headers.
5. Add `resourcedeepening` canonical run/allocation/attempt/request/outcome
   orchestration backed by FileRepository. The queue is a projection only.
6. Implement the exact `mindline-deepening-request/v0.1` schema and delivery
   path: `resource_id`, `capability`, `extractor_policy_fingerprint`, fixed
   `refresh_generation=0`, nullable `base_artifact_id`, and the canonical
   request fingerprint. Retrieval may emit at most one non-persisted structural
   intent after query-only selection and may not adopt it. A narrow
   application-orchestration port reloads canonical state, validates budget,
   satisfaction, policy, generation, base artifact, and duplicate identity,
   then asks FileRepository to atomically create the generation-zero allocation
   before projecting derived queue work. Exact replay and already-satisfied
   evidence are no-ops. Search, get, evaluation, and feedback can neither
   allocate nor enqueue work.
7. Implement exact logical-job, attempt, request, artifact, envelope, refresh,
   and retry identities. Only local `resources deepen --refresh --retry-id`
   allocates an explicit refresh generation.
8. Project/rebuild queue work from open canonical allocations. Terminal
   blocked, inaccessible, no-op, and stronger-current outcomes never refetch
   after queue loss.
9. Implement the exact founder progressive sequence and frozen aggregate caps:
   six transitions, stop/gate, mandatory matched cases, deterministic backfill,
   then seal. Cap exhaustion terminalizes remaining selected work and blocks the
   claim across restart.
10. Delay cleanup of run-owned rejected candidate envelopes/artifacts only until
    the bounded structural run projection in Slice 4 is durably serialized and
    read back. They remain owner-only, inside the shared 32-MiB evidence/staging
    cap, and cannot authorize retrieval. After that readback, cleanup follows
    the signed contained-path/hash rules; its absence proof is part of the final
    safety input. A crash cannot extend the run deadline or storage cap.

### Likely files

- Add `internal/resourcebudget/{types,port}.go` and tests.
- Add `internal/resourceadapters/{types,readable_body,primary_post,transcript}.go`
  and fixture tests.
- Add `internal/resourcedeepening/{types,repository_adapter,orchestrator,queue_projection,recovery}.go`
  and tests.
- Modify `internal/resourcefetch` for injected reservation/settlement.
- Modify `internal/resourcequeue` only as a derived projection adapter.
- Extend `internal/agentretrieval` with the structural intent projection only;
  its package receives no allocation or queue mutation port.
- Extend `internal/cli/resources.go` with bounded deepen/status/recover/refresh
  commands; no URL or secret argument is accepted.

### Gate

- Public fixtures distinguish useful bodies/posts/transcripts from shells,
  metadata, comments absence, and transcript absence exactly as the Spec says.
- Rejected secondary targets perform zero requests and emit only a fixed reason.
- Every request observed by the network has one prior canonical reservation;
  crashes and ambiguity never exceed request/attempt/byte/time/storage caps.
- Queue deletion before and after every allocation/outcome reconstructs exact
  open work and never repeats terminal work.
- Refresh retry replay returns one generation across concurrency/restart;
  malformed, oversized, non-v4, wrong-surface, and exhausted requests fail.
- Intent emission persists nothing. Orchestration adoption creates exactly one
  canonical generation-zero allocation before one derived enqueue, and exact
  replay/already-ready evidence changes nothing.
- Search/get/evaluation/feedback still make zero network calls, zero allocation
  writes, and zero queue mutations; feedback cannot authorize refresh.
- Transition processing stops before backfill/evaluation unless at least three
  exact frozen cases become independently ready.

## 8. Slice 3 — shared evidence with scoped multi-agent relevance

### Work

1. Add owner-managed `ContextScope`, `ContextLens`, and `ContextAgentActor` to
   `agentstate.Store`, including archive state, exact field/page/database caps,
   strict migrations, fingerprints, and readback.
2. Migrate existing lenses to `owner_root_scope` without changing lens IDs,
   text, timestamps, reversal links, idempotency, event counts, effects, or
   legacy output bytes. Map historical generic-agent judgments only to the
   reserved `legacy_agent_actor`.
3. Add owner CLI operations `scope-put|list|archive`, `lens-put|list|archive`,
   and `actor-put|list|archive`, plus owner-only binding of one configured active
   actor to each installed/local agent integration. Agent mode may list/select
   an active scope and lens but can only read its owner-configured actor; it
   cannot select or impersonate another actor, or create, rename, merge,
   activate, archive, or rebind any context object.
4. Split retrieval into two deterministic stages:
   - query-only authorization produces at most 100 complete base candidate and
     citation commitments; then
   - scope/lens/agent context reranks only that frozen set before top-k.
5. Persist `ContextualRetrievalRun` with exact query/snapshot/scorer/base-set
   commitments and one active scope/lens/actor.
6. Persist append-only `ContextualJudgment` rows only for records emitted by the
   bound run and exact context partition. Recompute effects; never trust input.
7. Apply the frozen formula only:
   - owner `used|dismissed` = `+1.0|-1.0` in exact scope/lens for all agents;
   - current agent `used|dismissed` = `+0.25|-0.25` in exact
     scope/lens/agent;
   - multiply raw feedback by `0.1`; clamp to `[-0.3,+0.3]`.
8. Make reversals exact negatives inside the original partition. Preserve
   conflicts as events; do not mutate evidence or infer truth.
9. Execute every row of the frozen context-isolation manifest live and after
   store close/open plus process restart.

### Likely files

- Extend `internal/agentstate` with `scopes.go`, `actors.go`,
  `context_migration.go`, `contextual_runs.go`, and focused tests.
- Refactor `internal/agentretrieval/hybrid.go` into bounded query-only and
  contextual-rerank collaborators while preserving the legacy adapter.
- Extend `internal/cli/agent.go`, `agent_args.go`, and `agent_feedback.go` with
  context selection/owner management boundaries.
- Add context-isolation evaluator support under `internal/recalleval` without
  importing private manifest rows into public result types.

### Gate

- Identical query/snapshot/output version/base budget yields byte-identical base
  candidate and citation commitments in every context.
- At least one precommitted context pair yields its two different exact final
  orders without introducing or deleting a candidate.
- After every agent event, only that exact actor/context view may change; after
  every owner event, all agents in only that exact scope/lens may change.
- The complete `N × (N-1)` source-agent/observer-agent noninterference matrix
  passes at every checkpoint, live and after restart.
- Cross-scope, cross-lens, cross-agent, wrong-run, wrong-record, substitution,
  duplicate, and effect-tamper writes fail before state change.
- A run or judgment naming any actor other than the integration's
  owner-configured active actor fails before evidence read or agent-state write,
  even when the other actor is active.
- Legacy output stays byte-compatible; no new agent receives legacy generic
  feedback.
- Context operations change no stable source, evidence head, readiness,
  artifact, retention, or evidence-catalog fingerprint.

## 9. Slice 4 — intervention evaluator and reusable proof

### Work

1. Add v0.2 evidence-intervention evaluation to `recalleval` with separately
   typed private authority/result and public result.
2. Before queue/network/stage/catalog mutation, create and read back private
   intervention authority binding exact candidate tree/binary/evaluator,
   baseline catalog, stable source, frozen manifests, run profile, validators,
   ranker, runtime config, run ID, and complete allowed catalog-change scopes.
3. Evaluate before/after with the same candidate binary. Reject any removed,
   unmatched, duplicate-owned, excess, wrong-kind, or wrong-scope catalog row.
4. Re-run readiness validation for every cited current artifact and validate
   the exact resource/artifact/envelope/capability/attestation/head citation
   commitment.
5. Compute baseline and candidate metric projections separately from exact
   per-case integer inputs. No aggregate boolean can override row arithmetic.
6. Execute all context-isolation oracle rows before and after restart and keep
   private identities out of the public type.
7. Derive the public result only after private result readback and lexical leak
   scan against all private values.
8. Add one typed immutable `resourcedeepening.VerifyRunStructure` boundary.
   It alone understands canonical run accounting and receives the exact
   owner-private before/after catalogs plus a read-only repository
   artifact/envelope resolver. It emits a bounded structural projection;
   `evalreadback` and `evalproof` do not traverse catalog rows or duplicate
   these formulas.

   The verifier selects one closed graph:
   `DeepeningRun(run_id) → every-and-only DeepeningAllocation(run_id) →
   matching JobAttempt and JobOutcomeReceipt → matching RequestExecution`,
   together with every-and-only matching `EvidenceStage`,
   `EvidencePromotionReceipt`, immutable candidate envelope, and immutable
   final artifact. Every allocation has exactly one terminal outcome; every
   attempt, request, stage, and promotion belongs to one selected allocation.
   Missing, orphan, duplicate, cross-run, extra-reachable, wrong-state,
   wrong-byte-length, missing-file, or commitment-mismatched evidence fails.

   Counter formulas are exact:

   - `allocated_resources` is the cardinality of distinct `resource_id` values
     across selected allocations, not the allocation-row count;
   - `attempts` and `request_executions` are the selected row counts;
   - reserved wire, decoded, and wall values are integer sums over every
     selected request reservation;
   - settled wire, decoded, and wall values are integer sums of the non-null
     actuals for `state=settled`; ambiguous requests require null actuals,
     remain fully represented only in reserved totals, and are separately
     counted in the projection;
   - `extracted_bytes` is the sum of `usage.extracted_bytes` from exactly one
     strict immutable candidate envelope per unique run-owned stage; an
     artifact-free inaccessible outcome contributes zero, and non-zero
     extraction without a journaled envelope is invalid;
   - `evidence_storage_bytes` is the sum, once per unique content-addressed ID,
     of candidate-envelope bytes plus final-artifact bytes for adopted stages,
     excluding any ID already reachable from the bound before-catalog; artifact
     creation must agree with the matching promotion receipt; and
   - `staging_storage_bytes` is the conservative consumed sum, once per unique
     content-addressed ID, of candidate-envelope plus final-artifact bytes for
     run-owned `staged|rejected` rows. Bounded later cleanup never refunds this
     run budget.

   Every recomputed field must equal the signed `DeepeningRun` counter. The
   verifier additionally requires evidence plus staging storage within the one
   shared cap, complete terminal outcome closure, exact run deadline/profile,
   and exact equality between the loaded catalog fingerprint and the private
   result's after-catalog fingerprint. The structural projection is serialized
   once by `evalreadback` before rejected-stage cleanup and is immutable input
   to the later final safety readback.
9. Extend `evalreadback` with strict recognition of the public v0.1
   intervention result, exact integer-PPM baseline/candidate projections,
   transition/context/nonregression status, claim limits, and comparable
   baseline binding. Unknown versions, missing artifacts, arithmetic mismatch,
   private fields, or inconsistent aggregate fingerprints fail closed.
10. Extend `evalproof` and the CLI with a WP-49-scoped evidence-intervention
   projection while preserving every generic proof-gate meaning. Authorized
   bounded public acquisition passes safety only when each observed request is
   covered by the signed run profile, canonical reservation/settlement, and
   exact `resourcedeepening` structural projection. The fixed public
   intervention result is never treated as if it contained request counters,
   and generic proof packages never reimplement canonical catalog semantics.

   The scoped safety projection may also permit exactly one hosted inference
   surface: the signed `codex_task` fresh-agent proof. That allowance exists
   only when the authorization receipt, staged manifest, immutable grant terms,
   issuance, complete use ledger, terminalization, valid task-output receipt,
   complete teardown receipt, and founder verdict all resolve to the same
   task/host/run/context/candidate/source/evidence commitments and remain within
   the one-shot disclosure limits. Before that complete chain exists, hosted
   agent disclosure is `pending`, never reported as zero or passed. Every
   unbound hosted inference or export remains prohibited; hosted telemetry,
   destination writes, unreserved/over-budget public requests, private exports,
   and every other existing prohibited side effect remain zero. Other artifact
   schemas cannot use either scoped allowance.
11. Freeze these exact command templates in the final proof manifest, replacing
    each bracketed binding with one owner-private absolute artifact root and
    recording the exact argv in the evidence ledger:

```text
mindline eval readback [candidate-run-root] --baseline [baseline-run-root] --out [readback-out]
mindline eval proof-gate [readback-out] --baseline [baseline-run-root] --out [improvement-proof-out] --claim improvement
mindline eval readback [final-structural-proof-root] --baseline [baseline-run-root] --out [final-readback-out]
mindline eval proof-gate [final-readback-out] --out [safety-proof-out] --claim safety
```

### Likely files

- Add focused `internal/recalleval/evidence_{types,authority,diff,metrics,context,public}.go`
  and tests.
- Extend `cmd/mindline-recall-eval` with one exact v0.2 intervention surface.
- Add reusable non-private capability, lifecycle, privacy, and fault fixtures
  under current package testdata conventions.
- Extend `internal/evalreadback`, `internal/evalproof`, and
  `internal/cli/eval_readback_test.go` for the strict public schema and scoped
  improvement/safety command paths.
- Add `internal/resourcedeepening/structural_proof.go` and focused mutation,
  closure, counter, artifact, cleanup-order, and cap tests; generic evaluation
  packages consume only its bounded projection.

### Gate

- At least 3 of 6 exact transition cases become ready.
- At least 12 answerable and 8 no-answer held-out cases pass:
  recall `>= max(0.75, baseline)`, precision
  `>= max(0.15, baseline-0.05)`, citation completeness `1.00`, no-answer false
  positives `0`, and exact six-case nonregression.
- Stable-source denominator and Slack-source behavior are byte-identical before
  and after.
- Every context/agent isolation row passes live and after restart.
- The public result is at most 64 KiB, contains aggregates/fingerprints/fixed
  reasons/claim limits only, and passes type-level plus lexical leakage tests.
- Ranking/scorer fingerprint remains unchanged; no threshold or weight is tuned
  from candidate results.
- Both exact readback and proof-gate command families pass with bound comparable
  baseline/current evidence. Missing/swapped baseline, wrong output root,
  arithmetic drift, unauthorized request, over-budget acquisition, hosted
  task without the complete one-shot receipt chain, any other hosted side
  effect, destination mutation, or use of a scoped allowance by another schema
  blocks or fails the named claim.
- Structural proof tests independently mutate every run counter and each graph
  edge; distinguish distinct resources from allocation rows; cover settled and
  ambiguous requests; vary extraction separately from envelope/artifact byte
  lengths; exercise shared storage exact-cap/cap-plus-one and duplicate
  content-addressed IDs; and prove cleanup cannot precede projection readback or
  refund a consumed run budget.

## 10. Slice 5 — contained fresh-agent disclosure and honest output

### Work

1. Extend `agentstate.Store` with immutable authorization/grant/issuance terms,
   hash-chained one-shot use state/events/transitions, task terminalization, and
   exact uniqueness/readback rules.
2. Add a narrow staged disclosure authorizer in front of every staged
   response-bearing route. Hand-authored files, config, another socket, or a
   stable actor ID alone never authorize private access.
3. Add owner-only authorization CLI bound to the active goal, exact fresh task
   and host identity, existing scope/lens/actor, expiry, candidate bits,
   behavior fingerprints, memory/evidence snapshot, and pre-minted run ID.
4. Construct a separate staged runtime/config/socket/agent-state database. Seed
   it only through the canonical SQLite backup API while the source remains
   readable, then fingerprint and read back the complete logical snapshot. A
   backup error, concurrent-snapshot ambiguity, or source/seed fingerprint
   mismatch fails before manifest issuance or staged service start. The staged
   runtime uses the exact candidate binary and behavior-equivalent rendered
   skill, shares canonical evidence read-only, and changes no default deployment
   or canonical feedback during the proof.
5. Permit exactly one compact search, zero or one citation-bound selected get,
   and zero or one feedback event within every response and cumulative cap.
   Reserve before private read; serialize once; settle/read back before releasing
   the exact byte slice. Ambiguous reservations remain consumed.
6. Capture at most 32,769 bytes from the task surface by the fixed one-hour or
   expiry deadline. Strictly validate the closed `answered|abstained` terminal
   envelope and terminalize the grant atomically before receipt creation.
7. For answers, require one selected get and citations for every claim. For
   abstention, require exact equality with repository-observed missingness, or
   the one ready-evidence semantic abstention bound to the selected citation and
   exact output. No free-text abstention is accepted.
8. Arm lifecycle teardown before the first staged side effect. Crash-resume
   output capture, feedback sealing/no-feedback, service stop, socket absence,
   runtime removal, private-envelope cleanup, and unchanged-default readback.
9. Store valid task output and optional feedback only in owner-private encrypted
   envelopes until Randy's verdict. Structural receipts contain commitments
   only.

### Likely files

- Extend `internal/agentstate` with focused `disclosure_*.go` files and tests.
- Add `internal/recallproof/staged_{manifest,grant,controller,output,teardown}.go`
  and tests.
- Extend `internal/localservice` with staged-only authorization middleware and
  distinct staged composition; leave default behavior untouched.
- Extend the installed skill template through its existing generation path;
  do not hand-edit a separately behaving staged skill.
- Add operator-only CLI surfaces under `internal/cli` and thin command wiring.

### Gate

- Wrong/expired/revoked/exhausted/task/host/run/scope/lens/actor/query/operation/
  tree/binary/skill/config/socket/source/evidence binding fails before private
  read.
- Exact-cap responses succeed; cap plus one releases no body. Full search plus
  get still permits zero-private-byte feedback; every later operation is denied.
- Reservation/completion crashes, duplicate request, partial socket write,
  restart, and corrupt state never replay private bytes or restore capacity.
- Answered output contains no uncited claim. Observed and semantic abstention
  paths reject every invented, omitted, extra, substituted, mixed, or unbound
  reason/citation.
- Every lifecycle fault from pre-staging through complete teardown either
  removes the staged runtime and proves the defaults unchanged, or enters
  explicit quarantine that permanently blocks verdict/install and makes no
  unchanged-default or completion claim.
- Raw query, answer, body, snippet, citation text, label, identity, URL, and
  path are absent from logs, Product Brain, telemetry, git, and public proof.

## 11. Slice 6 — founder verdict, feedback disposition, and exact installation

### Work

1. Present the exact captured cited answer or evidence-bound abstention to Randy
   through the owner-only verdict surface. Record `useful|not_useful` only
   against the exact task/output/citation/grant/context/candidate commitments.
2. If and only if verdict is useful and staged feedback settled, decrypt and
   promote that exact feedback once through canonical `agentstate` idempotency.
   Useful-without-feedback and not-useful change no canonical feedback.
3. Crash-resume deletion/readback of feedback and task-output envelope keys and
   ciphertext. Failed feedback promotion rolls canonical agent state back,
   deletes both envelopes, and blocks installation.
4. Build and install the immutable recovery bootstrap before candidate
   replacement. Atomically hand off OS autostart/default-listener ownership from
   exact prior descriptor to bootstrap, with explicit prior-preserved,
   prior-restored, bootstrap-ready, and no-service quarantine outcomes.
5. Snapshot exact prior binary, rendered skill, config, target-service
   descriptor, and agent-state backup beneath owner-only rollback authority.
6. Replace target components one at a time under the install lock and append the
   exact before/after journal state around each atomic side effect.
7. Start the candidate only as a sealed bootstrap child. Run no-private-output
   smoke while default forwarding remains blocked. Forward only after durable
   `smoke_passed`, terminal `installed`, and installation-receipt readback.
8. On any nonterminal restart with valid prior authority, restore and start the
   exact prior sealed child before forwarding. Use a smoke-passed candidate only
   when the prior is unprovable and candidate authority is exact. Quarantine
   only the enumerated unprovable-authority rows.

### Likely files

- Extend `internal/founderreview` with exact usefulness-verdict and disposition
  repositories.
- Add focused `internal/localservice/bootstrap_*.go` and
  `installation_recovery_*.go` files and a small immutable bootstrap command.
- Refactor existing install/rollback helpers behind the bootstrap ownership
  boundary; do not preserve a direct target-autostart path.
- Extend `internal/recallproof` with installation receipts and public fault
  evaluation.

### Gate

- `not_useful`, missing/invalid output, incomplete teardown, unreadable ledger,
  wrong/stale candidate, failed feedback promotion, or incomplete disposition
  cannot begin installation.
- Feedback promotion is exact-once and changes only the bound context-derived
  judgment; every non-feedback row remains byte-identical.
- Every feedback/output envelope disposition state crash-resumes and reads back
  key and ciphertext absence.
- The immutable bootstrap is the sole post-handoff autostart/default-listener
  owner. Direct target binding and preterminal private responses are impossible.
- Every public fault-oracle row passes: no-fault candidate install, valid-prior
  rollback before smoke, provable smoke-passed candidate recovery when prior is
  unprovable, terminal receipt recreation, both handoff descriptor sides,
  post-ready/pre-snapshot exact-prior service, and quarantine only where neither
  side is provable.
- Failed smoke restores exact prior deployment/service/agent state or produces
  an honest no-service quarantine receipt; no mixed deployment serves.

## 12. Slice 7 — full founder-private proof and fresh external readback

### Work

Run the exact signed sequence on the already drained founder-private Slack
library without exporting private material:

1. verify the immutable pre-build receipt and unchanged candidate ancestry;
2. record exact source/evidence/agent/default-deployment baselines;
3. create/read back private intervention authority before mutation;
4. process the six transition cases and stop at the transition gate;
5. if it passes, process all mandatory currently matched evidence cases and the
   deterministic capped backfill;
6. seal the after-catalog, run private intervention evaluation, and derive the
   public aggregate result;
7. close/reopen stores and restart processes for replay, recovery, stable
   denominator, duplicate, privacy, and complete N-agent isolation proofs;
8. start the contained candidate and authorize one fresh outside Codex task;
9. capture and validate its cited answer or evidence-bound abstention;
10. complete staged teardown, collect Randy's usefulness verdict, and dispose
    of private envelopes;
11. rerun final structural readback and the scoped safety proof over the exact
    canonical public-request ledger plus the complete one-task disclosure,
    teardown, verdict, and envelope-disposition receipts;
12. if and only if useful and that safety proof passes, install through the
    immutable bootstrap and pass the
    bound post-install no-private-output smoke; otherwise preserve the current
    default install. The already completed contained outside-agent task is the
    sole fresh-agent proof in this slice and its grant cannot be reused after
    teardown or installation;
13. perform final unchanged-tree five-role review and reconcile Product Brain
    with exact passed, failed, deferred, and non-generalizable claims.

### Gate

- Stable Slack denominator, duplicate replay, restart, queue reconstruction,
  privacy scans, readiness/citation validation, metric thresholds, pairwise
  context isolation, staged lifecycle, default preservation, and fault oracle
  all pass on exact bound artifacts.
- Outside agent follows only the exact rendered skill and needs no repository
  knowledge, bespoke prompt, manual file, Slack credential, or Product Brain
  key.
- Randy sees an answer with usable citations or an honest bounded abstention and
  can make the only subjective decision: `useful` or `not_useful`.
- A public report states exact aggregate result, limits, and what remains
  unproven. No private identity or content appears.
- Product Brain records final commit/tree, receipt fingerprints, aggregate
  metrics, reviewer outcomes, founder verdict, installation state, rollback or
  quarantine state, and remaining obligations.

## 13. Verification command families

Exact command lines are frozen in the authority tooling and final proof
manifest after implementation, but the required families are fixed now:

```text
go test -count=1 ./internal/assurance/...
go test -count=1 ./internal/personalmemory/... ./internal/evidencereadiness/...
go test -count=1 ./internal/resourcebudget/... ./internal/resourcefetch/... ./internal/resourceadapters/... ./internal/resourcedeepening/...
go test -count=1 ./internal/agentstate/... ./internal/agentretrieval/...
go test -count=1 ./internal/recalleval/... ./internal/evalreadback/... ./internal/evalproof/... ./internal/recallproof/... ./internal/founderreview/... ./internal/localservice/...
go test -race -count=1 ./internal/personalmemory/... ./internal/resourcebudget/... ./internal/resourcefetch/... ./internal/resourcedeepening/... ./internal/agentstate/... ./internal/recallproof/... ./internal/localservice/...
go test -count=1 ./...
go vet ./...
git diff --check eb75b3f9f431e1cb87233853c671087381ee6dbc...[candidate-commit]
```

The signed proof manifest replaces `[candidate-commit]` with the one exact
reviewed candidate commit before execution; a worktree-only or moving-HEAD
range is invalid evidence.

Required adversarial families include canonical mutation, strict decode and cap
plus one, symlink/containment/permission, SSRF/DNS/redirect/rebinding/peer-pin,
secret/URL redaction, crash-before/after every durable side effect, concurrency,
queue deletion/rebuild, stale/substituted commitment, context and actor
cross-leakage, disclosure replay, response-byte exactness, staged teardown,
bootstrap handoff, install/rollback/quarantine, and private-to-public leakage.

Named product claims use `mindline eval readback` and the applicable
`mindline eval proof-gate`; process exit zero alone is not outcome proof.

## 14. Evidence ledger and review policy

For every slice, record:

- authority decision and exact Plan/Spec commit;
- pre-change and post-change commit/tree;
- commands, structured results, and artifact fingerprints;
- defects found, corrections, and invalidated prior evidence;
- privacy/secret/path scans;
- reviewer role, reviewed tree, verdict, and timestamp;
- claim boundary and non-generalizable status; and
- rollback/recovery result.

High-risk authority, disclosure, privacy, recovery, evaluation, and installation
changes require the full five-role panel: Product/Domain User, Systems
Architecture, Delivery Quality, Risk/Safety, and Chain Steward. A defect restarts
the required unchanged-tree clean-pass sequence. No reviewer may sign a tree
different from the evidence tree.

## 15. Stop conditions and rollback

Stop automatically and preserve the working default install when any of these
occurs:

- authority receipt/manifests are absent, late, changed, private-leaking, or not
  ancestral to the candidate;
- fewer than three of six transitions become independently ready;
- stable source or Slack behavior changes;
- retrieval, citation, no-answer, nonregression, or context-isolation gate fails;
- resource/run/disclosure/storage/time budget exhausts;
- private state or evidence reaches logs, telemetry, Product Brain, git, public
  proof, or another agent/task;
- fresh output is missing, oversized, invalid, uncited, or unbound;
- staged teardown/default-state readback is incomplete;
- Randy records `not_useful`;
- feedback promotion cannot be proven and rolled back cleanly; or
- bootstrap/install authority cannot prove exact prior or exact smoke-passed
  candidate.

Before installation, rollback means discard/quarantine only the contained
candidate runtime and preserve the default unchanged. After bootstrap handoff,
recovery follows the exact public fault oracle: restore exact prior when
provable, complete exact candidate only after durable smoke authority, otherwise
serve zero private bytes under explicit quarantine.

## 16. Definition of done

WP-49 is done only when all of the following are true on one exact reviewed
tree:

1. the four pre-build manifests and authority receipt predate candidate code;
2. at least 3 of 6 exact insufficient resources become independently ready;
3. held-out recall, precision, citation, no-answer, and six-case nonregression
   gates pass;
4. the shared base candidate/citation set is identical across contexts while a
   precommitted pair ranks differently;
5. the complete `N × (N-1)` feedback isolation matrix passes live and after
   restart;
6. one fresh outside agent returns a validated cited answer or exact honest
   abstention through the bounded staged integration;
7. Randy records `useful`; if he records `not_useful`, the implementation is a
   valid learning result but the goal and installation remain incomplete;
8. optional settled feedback is adopted exactly once only after useful verdict,
   and every private envelope is disposed with readback;
9. the exact candidate installs through the immutable bootstrap and passes
   post-install smoke; a restored prior or quarantine is an honest recovery
   result but leaves WP-49 and the active goal incomplete;
10. the installed target is the exact binary, skill template, and behavior
    policy already proven by the contained fresh outside agent; post-install
    smoke proves that exact deployment binding without issuing a second grant;
11. all tests, proof gates, privacy scans, and two unchanged-tree final review
    rounds pass; and
12. Product Brain states the exact result, limits, installation state, and
    remaining gaps without a production, generalization, autonomy, or no-human
    claim.
