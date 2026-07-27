# WP-48 complete Slack recall — signed implementation Plan

Date: 2026-07-27  
Status: signed in DEC-429
Authority: DEC-427, DEC-428, WP-48  
Signed Spec:
`.productbrain/specs/2026-07-27-wp-48-complete-slack-recall.md`

## Delivery stance

Build the smallest coherent end-to-end recall slice. The stream-aligned
Mindline team owns the founder outcome. Ingestion control, private I/O, local
agent access, and evaluation are reusable platform capabilities. Slack
translation and public resource retrieval remain explicit adapters or
complicated subsystems behind narrow ports.

No increment may redefine the signed Spec. A material change to source scope,
retention, privacy, canonical authority, API compatibility, budgets, claim
limits, or destination policy returns to Spec.

Implementation uses the clean worktree
`/private/tmp/mindline-full-slack-drain` on
`codex/mindline-full-slack-drain`. The dirty `5216fb3` worktree remains
untouched. PR #47 is the prerequisite and must be merged or present as an
equivalent clean base before successor delivery.

Durable element ownership:

- FEAT-26: complete source reconciliation controller;
- FEAT-27: bounded terminal resource processing;
- FEAT-28: compatible compact local-agent access;
- FEAT-29: complete-recall evaluation and lifecycle proof.

## Increment 0 — prerequisite and frozen baseline

Outcome: delivery starts from one known PR #47 tree without mixing earlier
worktree changes.

Actions:

1. Recheck PR #47 reviews, threads, merge state, and available checks.
2. Run its focused agent/service/install/recovery/privacy tests and the current
   repository quality gates on `c5a5617`.
3. Merge PR #47 only if the exact reviewed head remains clean.
4. Confirm this successor branch contains the same prerequisite commit or
   rebase/cherry-pick only when needed; never copy files from the dirty root.
5. Record the prerequisite commit, Go/tool versions, test results, and known
   unrelated failures structurally.
6. Add fixture/eval manifest schemas before changing ranking behavior.

Files:

- `.productbrain/specs/2026-07-27-wp-48-complete-slack-recall-shape.md`
- `.productbrain/specs/2026-07-27-wp-48-complete-slack-recall.md`
- `.productbrain/plans/2026-07-27-wp-48-complete-slack-recall-plan.md`
- `internal/assurance/manifests/wp48-complete-recall-v1.json`
- `testdata/ingestioncontroller/`
- `testdata/resourcequeue/`
- `testdata/agent-eval/`

Gates:

- PR #47 exact-head review/check state is clean;
- prerequisite focused tests pass;
- no private fixture or path is added;
- `git diff --check` passes.

## Increment 1 — source-neutral ingestion controller

Outcome: deterministic strict frames can be adopted, canonically
receipt-checked, read back, replayed, and resumed without persisting raw Slack
frames.

Test first:

- 11-identity overlapping fixture with exact denominator;
- whole-frame strict validation plus per-unit adoption equality
  `delivered_native = canonical_declared + structural_excluded`;
- canonical import receipt equality to `canonical_declared`, including repeated
  overlap identities;
- zero user-authored exclusion;
- edit revision and delete tombstone;
- final-owner union/overlap/gap/parent/reply/thread closure;
- overlapping occurrences of one native key must have identical structural
  author class and disposition; conflict fails incomplete before import and
  preserves the prior canonical fingerprint;
- truncation, missing/duplicate ordinal, descriptor/total/end mismatch,
  trailing data, 32-MiB unit, 128-MiB aggregate, and 100,000-message breach all
  fail incomplete before first import and preserve the prior canonical
  fingerprint;
- process failure before and after validate/import/readback/ledger advance;
- interrupted output equals uninterrupted oracle;
- exact replay adds zero canonical effects;
- unsafe ledger root, symlink, containment, mode, atomic-recovery, and
  false-complete rejection;
- repository lock-held admission accepts an under-48-MiB import, rejects an
  over-cap direct/controller/concurrent import before persistence, and leaves
  the prior canonical fingerprint unchanged;
- controller/core/service schemas reject credential and raw cursor fields.

Implement:

- `internal/ingestioncontroller/types.go`
  - `mindline_ingestion_run/v0.1`;
  - lifecycle, structural counts, unit states, receipts, commitments;
- `internal/ingestioncontroller/controller.go`
  - validate intact frame;
  - classify each delivered identity retain/withhold/objective-exclude;
  - call repository-owned lock-held 48 MiB admission;
  - normalize/import every non-excluded identity, including overlap repeats;
  - seal adoption equation and compare canonical declared count/receipt;
  - fresh canonical readback;
  - exact in-memory final union;
- `internal/ingestioncontroller/ledger.go`
  - `mindline_ingestion_ledger/v0.1`;
  - owner-only atomic state and recovery;
  - structural field allowlist;
- `internal/adapters/slack/run_frame.go`
  - Slack-native identity, author-class, parent/reply, and disposition facts;
  - no credentials/cursors in the core contract;
- `internal/cli/ingestion.go`
  - bounded closed-envelope stream from a credential-owning connector;
  - `run-apply`, `run-status`, and `run-proof`;
- `internal/personalmemory/repository.go`
  - `ImportWithinBudget` computes and serializes the exact next library while
    holding the canonical lock, rejects before persistence above 48 MiB, and is
    the shared path for controller and direct imports in this slice;
- focused tests beside each package plus CLI framing tests.

Closed-envelope protocol:

- `begin` frame: schema, random run ID, private source scope, configuration
  fingerprint, exact ordered unit count, aggregate message-count ceiling, and
  aggregate byte ceiling;
- exactly one `unit` frame per declared ordinal, binding one complete
  descriptor to one strict NativeBatch v1;
- `end` frame: exact observed unit/message/byte totals and an ephemeral
  full-envelope commitment;
- maximum 32 MiB per unit frame, 128 MiB aggregate raw envelope, and 100,000
  native messages; exceeding a cap fails incomplete;
- the controller buffers and validates the complete bounded envelope in
  process memory, rejects missing/duplicate ordinals, mismatched
  descriptors/totals, truncation, trailing data, or a missing end marker, and
  computes the exact union before the first import;
- raw-frame and ephemeral-envelope commitments are never persisted or emitted;
  the ledger receives only post-sanitization aggregate commitments allowed by
  the Spec.

Design constraints:

- every NativeBatch v1 frame is validated whole and unchanged;
- structural exclusion is the only pre-canonical filter; final overlap
  ownership is reconciliation only;
- raw frames and individual native identities are never persisted in the
  structural ledger;
- restart reacquires from the frozen scope beginning;
- canonical evidence remains useful while an interrupted run is incomplete;
- the controller never owns Slack credentials or destination behavior.

Gate:

`go test ./internal/ingestioncontroller ./internal/adapters/slack ./internal/cli`
plus the fixture proof group must pass twice on the unchanged tree.

## Increment 2 — terminal resource queue and safe public fetch

Outcome: every safe canonical resource in the run becomes complete, partial, or
structurally blocked within frozen budgets while the Slack capture remains
searchable.

Test first:

- primary four-safe-resource oracle;
- separate fingerprinted fixture profiles for resource, request, wire,
  decoded, extracted, storage, attempt, and wall-time exhaustion;
- counter/config persistence before and after restart;
- stale-processing recovery without counter reset;
- deterministic terminal reason;
- concurrency never exceeds global/per-host limits;
- sensitive URL variants yield identical durable identity/fingerprint;
- no request reaches loopback/private/link-local/metadata/IPv4/IPv6/localhost,
  mixed-answer, rebound, unsafe redirect, downgrade, proxy, userinfo,
  cookie/auth-header, MIME, or decompression-bomb sinks;
- zero source text, URL, hostname, path, cursor, header, token, provider body,
  or raw error in queue/ledger/log/proof surfaces;
- terminal queue state cannot delete or mutate source capture;
- table-driven proof maps every terminal queue state into the exact canonical
  `ResourceContext` state and fixed current missingness with no fabricated
  content, deletes/rebuilds the derived queue, and verifies unchanged canonical
  fingerprint plus identical compact/get readback without queue access.

Implement:

- `internal/resourcequeue/types.go`
  - lifecycle and `mindline-resource-budget/v0.1`;
- `internal/resourcequeue/store.go`
  - owner-only atomic derived queue, counters, recovery;
- `internal/resourcequeue/runner.go`
  - deterministic ordering, leases, retries, terminal reasons, restart;
- `internal/resourcequeue/canonical.go`
  - map queue `complete|partial|blocked:<reason>` into existing canonical
    `complete|partial|inaccessible|failed` plus current
    `resource_blocked:<reason>` missingness;
- `internal/resourcefetch/policy.go`
  - STD-20 before identity/fetch/log;
  - scheme/port/host/header/proxy/MIME checks;
- `internal/resourcefetch/dialer.go`
  - all-answer DNS validation, pinned public address, peer verification, TLS
    hostname preservation, redirect revalidation;
- `internal/resourcefetch/extract.go`
  - separately bounded wire/decoded/extracted content;
  - provider-neutral public metadata/text;
- `internal/resourcefetch/providers.go`
  - HTML/article/Substack, GitHub, YouTube, Spotify/media, LinkedIn, and generic
    public HTTP profiles without changing canonical schema;
- `internal/cli/resources.go`
  - run/status/proof commands using structural output only.

Reuse:

- `routing.PrepareURLForStorage`;
- `personalmemory.MergeEnrichment`;
- `privateio` atomic/mode/containment primitives;
- no browser cookies, Slack session, Product Brain key, or destination adapter;
- compact search/get read canonical current resource state and missingness,
  never rebuildable queue state.

Gates:

- focused queue/fetch tests pass with the race detector;
- every fixture resource is terminal;
- every queue-to-canonical mapping and queue-rebuild-independent compact/get
  readback gate passes;
- privacy allowlist scanner reports zero forbidden fields;
- the resource proof group passes twice unchanged.

## Increment 3 — additive compact agent access and feedback repair

Outcome: fresh agents receive small citations by default and hydrate only
selected evidence without breaking existing PR #47 clients.

Test first:

- byte/field-equivalent legacy
  `mindline-agent-context-packet/v0.2` on `/v1/search`;
- capability negotiation;
- compact `mindline-agent-context-packet/v0.3` has citations/diagnostics but no
  records, resources, revisions, content, database path, or runtime path;
- explicit get is the only hydration path;
- current citations do not inherit historical placeholder missingness;
- same feedback retry token and intent replays;
- changed intent with the same token conflicts;
- new run/event can create a new judgment without global-key conflict;
- installed skill uses compact when available and falls back to v0.2;
- upgrade/rollback preserves canonical and durable-state fingerprints and
  leaves legacy status/search/get usable.

Implement:

- `internal/personalmemory/types.go`
  - compact packet and current resource summaries;
- `internal/personalmemory/search.go`
  - `SearchCompact` without unselected full hydration;
  - current-only missingness;
- `internal/localservice/types.go`, `server.go`, `client.go`
  - capabilities and compact endpoint/client;
  - public status projection without database path;
- `internal/agentstate/feedback.go`
  - derived v1 retry identity;
- `internal/cli/agent.go`, `agent_args.go`, `agent_feedback.go`
  - `--format compact-v0.3`, capability fallback, `--retry-token`;
- `internal/cli/agent_install.go` and `internal/localservice/install.go`
  - atomic prior binary/skill hash backup and state-safe rollback;
- installed `mindline` skill
  - compact-first, explicit get, citation, untrusted-source, abstention, and
    bounded feedback workflow.

Compatibility rule:

- do not change legacy `/v1/search` or canonical/durable state schema;
- rollback restores executable/skill artifacts only.

Gates:

- legacy/compact/Get/feedback/install/rollback focused tests pass;
- clean-home lifecycle proof passes;
- canonical and durable-state fingerprints are unchanged across migration.

## Increment 4 — integrated reusable proof and pre-live authority

Outcome: the exact delivery tree proves the complete reusable lifecycle before
private Slack data crosses the connector boundary.

Actions:

1. Register WP-48 proof groups in the assurance manifest.
2. Run failure injection immediately before and after closed-envelope receive,
   strict-frame validation, disposition/adoption sealing, final
   union/reconciliation, canonical import, canonical readback, resource
   enqueue, resource processing, enrichment merge, and ledger advancement.
   The integrated group also deletes/rebuilds the derived queue and re-runs
   canonical fingerprint plus compact/get state/missingness readback.
   It also runs every malformed/over-cap closed-envelope case and asserts zero
   import effect plus the unchanged prior canonical fingerprint.
3. Run `go test ./...`, race-sensitive focused tests, build, vet, file-size,
   diff, credential/secret, private-surface, and owner-mode gates.
4. Run legacy upgrade/rollback on a clean temporary home.
5. Run two unchanged-tree reviewer passes from Product, Architecture, Delivery
   Quality, Risk/Safety, and Chain Steward.
6. Capture defects and corrections on Chain; any correction restarts the clean
   pass.
7. Create a pre-live authority receipt containing the exact `HEAD`, clean-tree
   source fingerprint, built binary hash, assurance-manifest fingerprint, and
   live configuration fingerprint.

Proof output follows the Spec allowlist. Synthetic fixture sentinels remain
synthetic and may not resemble private source values.

The connector/controller synthetic integration uses distinct source-text, URL,
token, and cursor sentinels and captures outer tool/model result, controller
stdout/stderr, connector stdout/stderr, logs, temporary paths, ledger, receipt,
and proof. Every surface must contain zero sentinel matches.

Gate:

- every reusable proof group passes on the exact pre-live commit;
- reviewers sign the exact tree twice;
- no uncommitted implementation change exists before private acquisition.

## Increment 5 — private Codex-operated Slack drain

Outcome: one frozen self-DM scope becomes a complete canonical recall library.

Connector procedure:

1. Use the connected Slack connector to identify the self-DM, freeze the latest
   inclusive watermark, and use the earliest available boundary.
2. Page channel history to exhaustion. For every discovered threaded parent,
   page replies to exhaustion.
3. Keep Slack token/session and raw cursors inside the connector only.
4. Build strict occurrence-complete NativeBatch v1 frames in connector memory.
   Prefer one complete frame when beneath 20,000 messages/50,000 URL
   occurrences/32 MiB; otherwise use exhausted overlapping time windows.
5. Immediately before connector use, verify `HEAD`, clean-tree source
   fingerprint, binary hash, assurance-manifest fingerprint, live
   configuration fingerprint, and pre-live receipt all match Increment 4.
6. Pass frames through an already-open owner-local opaque pipe, Unix socket, or
   PTY stdin into the controller. Raw bytes may not appear in command
   arguments, environment, files, logs, stdout/stderr, outer model/tool-result
   payloads, or generated instructions. The controller returns only its
   structural receipt. If this transport is unavailable, fail closed before
   acquisition.
7. On interruption, reacquire from the frozen scope beginning and replay.
8. Stop fail-closed if exhaustion, thread closure, strict frame validity,
   identity union, capacity, receipt, or readback is ambiguous.

Then:

- run safe resource processing under the frozen live profile;
- record structural evidence that every live import used repository-owned,
  lock-held 48-MiB admission;
- restart the service/controller and resume terminal processing;
- exact-replay the frozen source;
- delete/rebuild the derived resource queue and verify unchanged canonical
  fingerprint plus identical compact/get current resource state and
  missingness without queue access;
- create/delete a temporary lens and verify the canonical denominator and
  fingerprint do not change;
- scan every non-runtime artifact and output against the privacy allowlist.

Private runtime may contain canonical evidence. Exported proof contains only
aggregate structural evidence.

Gate:

- run state is `complete`;
- the live structural receipt repeats the exact pre-live tree, binary,
  assurance-manifest, and configuration binding;
- live repository-admission and queue-rebuild-independent canonical/compact/get
  readback gates pass;
- exact denominator, per-unit adoption equation, canonical receipt/readback,
  terminal-resource, restart/replay, retention, owner-mode, and privacy gates
  pass;
- otherwise stop with a structural blocker and no completeness claim.

## Increment 6 — same-library retrieval evaluation

Outcome: ranking/abstention claims are based on the full frozen library, not the
old eight-item sample.

Actions:

1. Freeze the complete canonical fingerprint.
2. Assign an independent reviewer who can inspect canonical private evidence
   but cannot inspect evaluated-run output.
3. Create the owner-only v0.1 evaluation manifest with at least 12 answerable
   and 8 no-answer cases and exact expected current IDs.
4. Run the verified PR #47 baseline binary in an isolated derived-state root
   against that exact library and manifest.
5. Run the successor against the same library and manifest without tuning its
   synthetic-fixture-frozen abstention policy before or after held-out output
   is visible.
6. Compute the Spec formulas and thresholds.
7. If any threshold fails, retain the working full recall slice but block all
   ranking-quality claims. Any improvement iteration requires a new
   independently frozen held-out cycle.

Gate:

- matching library/manifest fingerprints;
- recall@5, precision@5, citation completeness, no-answer, and compact privacy
  thresholds pass for a ranking-quality claim;
- aggregate metrics/case fingerprints only leave the private runtime.

## Increment 7 — fresh outside-agent proof and founder handoff

Outcome: someone who has no Slack/corpus/database access can actually use
Mindline.

Actions:

1. Build and install the exact successor binary and Mindline skill.
2. Start/restart the user service and verify status is structural and path-free.
3. Give a fresh Codex task only the installed Mindline skill and the founder
   questions; do not provide Slack, repository, database, record list, or
   earlier outputs.
4. Require compact search, selected-record get, cited answers, current
   missingness, explicit abstention, and bounded feedback on used/dismissed
   evidence.
5. Replay the same intended feedback event and prove no duplicate effect.
6. Upgrade/rollback once, prove legacy CLI usability and unchanged
   fingerprints, then reinstall the successor for founder review.
7. Ask Randy only for qualitative taste: are the cited answers genuinely
   useful personal recall?

Gate:

- fresh-agent run passes the exact installed surface;
- founder reviews concrete cited answers and an owner-only structural review
  record stores exactly one enum: `useful`, `not_useful`, or `declined`;
- `useful` is required to close the user-value outcome; `not_useful` returns to
  Diagnose/Shape and `declined` leaves the outcome unverified;
- no destination action or broader claim is inferred.

## Increment 8 — review, PR, and close-out

Outcome: code, Chain, proof, and visible product behavior agree.

Actions:

1. Freeze the final tree and run full quality/security/proof gates. The final
   `HEAD`, clean-tree source fingerprint, binary hash, assurance-manifest
   fingerprint, and live configuration fingerprint must equal the pre-live
   receipt and live proof. Any mismatch invalidates reusable/live/fresh-agent
   evidence and requires those proofs to rerun on the final tree.
2. Run two clean unchanged-tree reviewer passes from all five roles.
3. Resolve every actionable PR comment and CI failure; corrections restart the
   relevant clean-pass sequence.
4. Update WP-48 with exact proof, sample boundary, blocked claims, and remaining
   obligations.
5. Promote/verify only signed evidence and use `pb session close` with an
   honest summary.
6. Remove obsolete temporary worktrees only after the branch/PR is safely
   published; preserve the user's dirty original worktree.

Closure is blocked unless:

- every Spec done-when criterion has executable evidence;
- the full drain is structurally complete;
- the outside-agent proof works;
- founder qualitative review is recorded `useful`;
- no required work remains hidden in terminal output, private artifacts, or
  chat memory alone.

## Required reviewer ownership

- Product: founder journey, saved-item survival, usefulness, scope honesty.
- Architecture: canonical/derived boundaries, source neutrality, replay,
  compatibility, rollback.
- Delivery Quality: executable denominators, failure injection, comparable
  metrics, lifecycle proof.
- Risk/Safety: private data, credentials, fetch policy, proof allowlist,
  owner-only recovery.
- Chain Steward: Spec fidelity, phase gates, audit, claims, and close-out.

Any blocker stops phase exit. Any file change after a clean review restarts the
full selected panel.
