# Plan: WP-46 Stable Local Control Plane

**Status:** draft for role-panel review  
**Phase:** Plan  
**Version:** draft-12  
**Date:** 2026-07-15  
**Product:** Mindline  
**Signed Shape:** `e4e1c6ae6ebab7f52efc24bfa1d4368de3d4570fbdf853ff5a39141c045e0f6e`  
**Signed Spec:** `d94f926b07286b374212f905b1988babeaa646eeccfb9e4fcead11cc40de5070`  
**Proof manifest:** `internal/assurance/manifests/wp46-stable-control-v1.json`, SHA-256 `e7264dda3b6ed3978f6ac880d49be4ac798ce930af433728fbc6e35fe3a152a9`  
**Rollback baseline:** `17d33cb8845d2eab744cc605c310f54bc20fdb01`  
**Authority:** WP-46 shaped materialization; DEC-414, DEC-418, DEC-419, DEC-420; STD-19, STD-20, STD-21, STD-22; PRI-1; BR-1  
**Implementation authority:** none until this exact Plan passes two clean five-role reviews, is captured/materialized in WP-46, and the second handoff audit passes

## 1. Delivery outcome

Deliver one coherent replacement path, not a parallel launcher:

```text
Codex launcher pipe ──> mindline activation serve ──> 127.0.0.1:9876
                                  │
                                  ├─ locked/pairing/session boundary
                                  ├─ durable non-secret settings
                                  ├─ explicit immutable proof selection
                                  └─ gated in-memory Slack/Product Brain leases
```

Randy sees one stable link and exact saved settings. He remains Founder / Product Taste Maker; the Product Team makes and executes the implementation decisions inside the signed boundary. No database, framework migration, new provider, or processing redesign enters this Plan.

## 2. Pre-Plan challenge and reconciliation

The Architecture, Risk/Safety, and Delivery Quality challenges tightened the initial sequence:

- Accept: only three new reusable capability packages—`controlsettings`, `controlrun`, and `operatorchannel`—with existing packages extended at their current ownership boundaries. The thin `cmd/mindline-operator-launcher` and `cmd/mindline-proof-runner` composition artifacts contain no additional business capability; they invoke the existing operator-channel and assurance ports through exact source-controlled commands.
- Accept: persistence lands first as dormant, non-authorizing libraries. It is not wired into production startup until the fixed listener can bind before any durable mutation.
- Accept: executable/source/manifest-bound receipt migration finishes before receipt age is removed; event revocation finishes before lease TTLs are removed.
- Accept: the exact source-controlled proof manifest is embedded and bound into the receipt/evidence index; the rollback baseline and browser bundle/engine are pinned now.
- Accept: every defect invalidates the binary, receipt, evidence index, and reviewer passes.
- Reject: adding a database, general storage framework, full dependency-direction rewrite, SPA framework, full lens manager, keychain/OAuth/daemon, or legacy-runtime migration. Each is outside the signed outcome.
- Preserve: the single streaming pairing response from the signed Spec. A two-route browser-claimant protocol was considered but would invent a second redemption surface and diverge from the signed Shape.

The first ten Plan review attempts found and this draft corrects the remaining execution defects. Both the Codex-owned operator launcher and proof controller are named, built, versioned, hashed, source-controlled executables. The manifest now freezes the outer `go build` and `run --manifest --controller-bootstrap` controller commands, and every runner-owned group invokes the same exact binary with argv `group <signed-group-id>`; no abstract operation remains. Before each invocation, the outer driver generates a fresh non-authorizing invocation id/generation and creates one unique owner-only no-replace controller-bootstrap bundle containing the exact build outputs, build info, hash, version output, manifest/commit/tree binding, and invocation-ready record. A crash before that record is sealed leaves an inert uniquely named root that is never discovered, reused, or mutated; the retry uses a new identity/root. Runner bootstrap accepts only the exact record path in argv, validates the bundle and actual argv, imports it byte-for-byte into the fresh attempt, and a named required pre-closure group proves the import before the explicit checkpoint dependency. That record flows through the checkpoint, receipt, final revalidation, acceptance item 16, and final index; the authoritative `succeeded` marker is the controller invocation's terminal-success evidence, so closure requires no circular post-success write. Bootstrap and attempt recovery remain attempt-isolated and crash-safe; active launches retain their payload-free liveness channel; the browser creates provider-free v0.4 evidence before rollback; and final success has the single `succeeded` commit with no later required write. Existing readback and safety-gate paths are frozen exactly. The review sequence restarts from Product on this exact draft and manifest.

## 3. Team ownership and stable APIs

| Team capability | Owns | Must not own |
|---|---|---|
| Stream-aligned activation team | end-to-end founder return workflow, app projections, UI actions, lifecycle proof | new provider-specific core schema |
| Platform: local control state | private atomic primitives, settings repository, run-selection pointer | run/delivery authority |
| Complicated subsystem: assurance/session security | executable/source/manifest receipt, operator pipe, browser session, lease revocation | product strategy or destination semantics |
| Source adapter: Slack | strict Slack default envelope and live source connector | generic settings repository |
| Destination adapter: Product Brain | existing exact draft approval/readback/replay | browser pairing or app authentication |
| Chain Steward / Delivery Quality | phase authority, manifest, evidence, unchanged-tree review | implementation shortcuts or hidden scope |

Stable new ports:

```text
controlsettings.Repository:
  Load(ctx) -> Snapshot
  Save(ctx, expected {version,generation}, Draft) -> Snapshot
  Recovery(ctx) -> Problem
  Recover(ctx, problemFingerprint, acknowledgement, action) -> Snapshot

controlsettings.AdapterValidator:
  ValidateDefaults(schemaVersion, strictRawJSON) -> canonicalStrictRawJSON

controlrun.Repository:
  Load(ctx) -> SelectionSnapshot
  CompareAndSwap(ctx, expected {version,generation}, explicitRunID) -> SelectionSnapshot
  Recover(ctx, problemFingerprint, expected?, acknowledgement, clear|explicitRunID) -> SelectionSnapshot

operatorchannel.Verifier:
  Verify(*os.File) -> verified read channel

operatorchannel.Launcher:
  Run(serverExecutable, verifiedAnonymousOperatorInput, optionalVerifiedProofController, stdout, stderr, signals) -> exit
  (requires a second Codex-owned anonymous single-writer input pipe;
   constructs the downstream anonymous pipe; starts exactly serverExecutable activation serve;
   during an active proof passes the payload-free controller read end to launcher and server;
   retains no controller writer in either child; never opens a browser; forwards only bounded frames)
```

Settings and selection revisions use `{version,generation}` everywhere, including HTTP requests and settings-derived proof snapshots. Neither object can authorize provider access, source scope, approval, or destination mutation.

## 4. Slice 0 — freeze proof harness and authority

### Work

- Keep the proof manifest at `internal/assurance/manifests/wp46-stable-control-v1.json` byte-stable after Plan sign-off.
- Add an embedded-manifest loader and strict validator in `internal/assurance` during implementation. It rejects unknown fields, missing/duplicate check IDs, unrecognized placeholders, shell execution, missing required tests, or a manifest whose embedded bytes do not match the signed SHA.
- Add the assurance engine in `internal/assurance` and a thin source-controlled `cmd/mindline-proof-runner` executable with exact version output `mindline-proof-runner/v1`. Before any attempt, Codex generates a fresh random invocation id/generation, creates its unique no-replace owner-only controller-bootstrap root, executes the manifest's exact outer build command into that root, captures its exact stdout/stderr, clean frozen-commit build info/hash, and exact version output, then atomically seals and fsyncs an invocation-ready record with the exact `run --manifest --controller-bootstrap <record>` command. A partial root is inert history and is never auto-discovered; same-commit retry always uses a fresh root. The invoked runner requires its actual argv to match, validates only the explicitly named bundle before mutation, imports it byte-for-byte with no-replace into the fresh attempt, and proves it through the named `validate_controller_bootstrap` group. Every other runner-owned manifest group is a direct subprocess invocation of that same binary with exact argv `group <group-id>`; the group id selects only the signed embedded behavior. No shell or free-form operation dispatch is accepted.
- The runner writes owner-only JSONL plus the final evidence index containing manifest, runner, commit, server binary, launcher, receipt, configuration, tool, browser, and run-evidence fingerprints. It never writes captured provider credentials or private source bodies.
- Before any manifest group, the proof-runner bootstrap resolves the repository, frozen commit, stable control root, commit proof root, fixed checkpoint/receipt, attempt state/lock/process registry/history, and pinned tool root. It acquires the fixed attempt lock, requires no prior tracked group and an exclusive TCP4 port-9876 bind, and holds that check listener while strictly recovering any abandoned `preparing|checkpoint_sealed|receipt_minted|live_proof|closing` state. It then generates a fresh random non-authorizing attempt id/generation, derives `<commit-proof-root>/attempts/<attempt-id>.<generation>`, creates it owner-only with no-replace, writes/fsyncs an owner-only namespace marker, and fsyncs the directory. Only after that durable namespace exists does it atomically publish `preparing` with the namespace-marker fingerprint and derive/open this attempt's ledger, pre-change worktree/binary, frozen binary, launcher, and artifacts. A crash before publication leaves an inert orphan root with no current attempt/receipt authority; it is never auto-adopted. Prior attempt roots are immutable history and never a binding candidate. `PRIVATE_RUN_ROOT` remains deferred to its successful producer. A missing, premature, ambiguous, symlinked, out-of-root, reused, or secret-valued binding fails before its consumer.
- Implement the exact durable attempt states `preparing → checkpoint_sealed → receipt_minted → live_proof → closing → succeeded`, with `failed` reachable from every non-succeeded state. `succeeded` is the single authoritative success commit and contains the already-durable final-index fingerprint. During `receipt_minted|live_proof`, live authority additionally requires the held attempt lock and verified controller-liveness channel. An abandoned or mismatching state may start only the safe shell after bind-time quarantine; it cannot admit a provider credential or transport.
- Encode the only legal phase order as `prepare → pre_live → receipt → browser → private_live → containment → post_live → finalize`. A group may depend only on completed earlier/same-phase groups; the runner rejects a cycle, forward dependency, missing dependency, unclosed group, or artifact claimed before it exists.
- Implement four closed scheduling policies only: normal groups run after successful dependencies; containment groups run at phase exit/after attempt even when browser or private proof failed; attempt closure always runs at finalization and quarantines the fixed checkpoint/receipt on any failure; baseline cleanup always runs when its worktree was materialized. Failed attempts end with no fixed live-authority receipt and write a value-free failure ledger; only an all-pass run may retain its exact matching receipt and create the success evidence index.
- Encode a machine-readable acceptance map for all signed Spec acceptance items 1–16. Every item names closed manifest groups and structured predicates; free-form prose cannot satisfy an acceptance item.
- Stage exact scanner identities before final proof: govulncheck v1.1.4, gosec v2.28.0, gitleaks 8.30.1. Tool bootstrapping is proof infrastructure and does not enter runtime dependencies.

### Files

- Add `internal/assurance/manifest.go`, `manifest_test.go`, `proof_runner.go`, `proof_runner_test.go`, `attempt.go`, and `attempt_test.go`, plus the thin `cmd/mindline-proof-runner/main.go`.
- Modify `internal/assurance/runner.go` to derive its fixed gate plan from or verify equivalence with the embedded manifest.

### Gate

- Manifest JSON parses and hashes to the signed value; the controller build/version/run commands and every runner group argv are exact and complete; runner version/bytes/build/source/manifest identity is pinned. The invocation-unique external controller-bootstrap bundle is immutable, owner-only, no-replace, imported exactly once into the attempt, and its named validation group is in the pre-closure set, an explicit checkpoint predecessor, and acceptance item 16. Its identity, fingerprint, and actual invocation argv are bound by checkpoint, receipt, final revalidation, and final index; only the durable `succeeded` marker proves terminal invocation success. Injected crashes across identity/root creation, artifact capture, record sealing/fsync, and pre/post exec prove partial roots remain inert, no partial bytes are imported, and a new same-commit invocation succeeds without overwrite or latest-selection. Every attempt artifact has one exact attempt-scoped path; bootstrap resolves commit-scoped bindings first and fresh-attempt bindings only after it creates the no-replace attempt root, before group evidence. Deferred bindings have one declared producer and one producer-completion edge before use; the pre-closure required set excludes the closer and equals every other manifest group; the phase graph, crash finalizers, group exports, artifacts, receipt-check map, and acceptance 1–16 map validate as closed. Same-commit retry tests prove zero prior-attempt or prior-controller artifact consumption or collision.
- Every automated command is argv-based; no shell interpolation.
- Zero/missing/duplicate/skipped named `TestWP46_` events fail the manifest group.
- No private/live source, provider credential, Product Brain mutation, or browser navigation occurs in this slice.

## 5. Slice 1 — dormant non-authorizing persistence

### Work

1. Extend `privateio` with:
   - owner-only nonblocking advisory lock held on a regular `0600` descriptor;
   - no-follow bounded strict read;
   - last-valid backup, temp fsync, atomic rename, directory fsync, and reread validation;
   - injected fault points for every write boundary; and
   - canonical error categories that never expose file contents or secret-shaped input.
2. Add `controlsettings`:
   - exact v1 schema/defaults from the Spec;
   - ordered lens list, routing policy, all five existing processing ceilings;
   - adapter-default discriminated envelopes and validator registry;
   - random 256-bit generation plus display version/fingerprint/save time;
   - strict 64 KiB limit, CAS, backup, adoption, explicit recovery, and conflict projection; and
   - secret-shaped field/value rejection without reflection.
3. Put Slack `slack_web_api` default validation in `internal/acquisition/slack`; composition registers it. Core settings never imports Slack.
4. Add `controlrun`:
   - explicit selection schema, generation CAS, backup, recovery, no-latest behavior;
   - random run ID construction and no-replace directory reservation helper; and
   - no activation-state mutation or authority behavior.

### Files

- Modify `internal/privateio/privateio.go`; add `atomic.go`, `lock_darwin.go`, `faults.go` and tests.
- Add `internal/controlsettings/{types,repository,validation,defaults,recovery}_test.go` and implementations.
- Add `internal/controlrun/{types,repository,validation,recovery}_test.go` and implementations.
- Add `internal/acquisition/slack/control_defaults.go` and tests.

### Gate

- Defaults and exact round trip; 30-day injected clocks; cross-build reload.
- Version+generation ABA race and stale conflict; one winner.
- Permissions, ownership, symlink/non-regular replacement, lock contention, oversize, unknown/duplicate/trailing fields.
- Fault at every backup/temp/fsync/rename/directory-fsync/reread boundary has the deterministic Spec outcome.
- Current+backup corrupt/unsupported cases require explicit problem-fingerprint recovery.
- Selection never infers latest and never modifies run evidence.
- Secret sentinel absent from settings, selection, backup, lock error, and test logs.
- The libraries remain unwired from production `activationapp.New` and cannot authorize a transport.

## 6. Slice 2 — executable-bound assurance and run integrity

### Work

1. Upgrade the pre-live receipt schema to bind:
   - exact executable SHA-256;
   - exact proof-runner schema version, executable SHA-256, clean build commit, and source tree;
   - signed embedded proof-manifest SHA-256;
   - clean commit/source binding;
   - configuration and gate-plan fingerprints;
   - runner/tool identities and complete check evidence; and
   - informational generated time without age authority.
2. Add injected executable/source/config/manifest binding providers and constant-time fingerprint comparison.
3. Every proof attempt starts with the fixed checkpoint and receipt absent or atomically quarantined and a fresh attempt id/generation in `current-proof-attempt.json`. The exact pinned proof-runner completes every automated pre-live group and atomically seals their value-free evidence at exactly `<root>/assurance/pre-live-checkpoint.json`, binding its own schema/version/hash/build/source identity plus that attempt. It then invokes `activation gate-receipt` with no arguments. That command resolves the default stable root, requires the current attempt state and exact matching already-sealed checkpoint, never changes the checkpoint, and atomically mints only `<root>/assurance/pre-live-receipt.json`; tests inject a private root provider. The exact server binary validates the matching attempt/checkpoint and mints the receipt that the same binary later serves. The receipt binds the attempt, checkpoint, proof runner, executable/source/configuration/manifest/operator-launcher/gate/check/tool identities and explicitly does not bind browser, private-run, containment, or final-index evidence. A mint failure cannot expose the quarantined prior receipt; any later group failure triggers finalization that stops tracked processes, quarantines this attempt's fixed checkpoint/receipt, and proves current live authority absent.
4. Gate startup also validates the exact attempt state. `succeeded` accepts its exact receipt/final-index binding for ordinary later launches. `receipt_minted|live_proof` accepts only while the fixed attempt lock is held by the verified proof controller and the payload-free controller-liveness descriptor is valid; controller EOF cancels the live context/process group. After binding port 9876, any abandoned active/closing state or mismatching receipt is atomically marked failed and quarantined before the safe shell loads, so restart cannot reuse a stale active receipt.
5. Split `RunIntegrity` from `LiveTransportAuthority`:
   - valid v0.4 state/journal/schema/executable/commit/config enables read-only projection;
   - receipt/source/gate readiness controls credentials and live transport;
   - missing/old/torn/wrong receipt never prevents the locked/settings shell; and
   - new receipts require restart.
6. Add v0.4 state that persists verified identities but no connection IDs, session refs, secrets, or lease deadlines. v0.3 remains byte-identical read-only evidence and is never migrated.
7. Recheck live binding at startup, credential acceptance, and immediately before every Slack/retrieval/Product Brain socket or mutation reservation. Drift cancels queued work; ambiguous Product Brain I/O enters existing reconciliation only.
8. Remove only receipt-age authority and its receipt-age parameters after all callers use the exact binding. Approval-preview expiry, pairing expiry, HTTP deadlines, Slack processing budgets, and delivery reconciliation deadlines remain.

### Files

- Modify `internal/assurance/{receipt,runner}.go` and tests.
- Modify `internal/activationcli/run.go` gate path and tests.
- Modify `internal/activationapp/{types,app,state_validation,source,delivery,recovery}.go` and tests.
- Modify the Slack external-import authorization call sites that currently consume receipt age.

### Gate

- Old receipt schema, absent/duplicate/wrong-path receipt, and every executable/manifest/commit/source/config/plan/check/tool drift fail live authority.
- Missing gate still exposes only safe settings and compatible read-only proof state.
- Receipt stays valid after 30 injected days when every binding is unchanged.
- Approval-preview expiry, pairing expiry, HTTP deadlines, Slack processing wall budget, and delivery reconciliation deadlines remain intact; only receipt-age authority disappears.
- v0.3 directories and evidence hashes remain byte-for-byte unchanged.

## 7. Slice 3 — process-lifetime provider leases

### Work

- Change registration to `LeaseOptions{Kind, Secret, Identity}`; remove idle and absolute TTLs.
- Remove expiry fields and `ConnectionID` from projections persisted by v0.4. Persist only adapter kind plus stable `VerifiedIdentity`.
- Add provider-neutral `ErrCredentialRejected` and live-authority revocation hooks.
- On disconnect, shutdown, provider rejection/revocation, identity mismatch/drift, or executable/source/config/gate drift: atomically revoke, zero secret bytes, cancel the lease context, and reject every later use.
- Only after all revocation tests pass, delete old idle/absolute expiry behavior and update Slack/Product Brain connectors.

### Files

- Modify `internal/integrations/session_registry.go` and tests.
- Modify `internal/activationapp/{slack_source_connection,productbrain_connection,types,app}.go` and tests.

### Gate

- An injected 30-day time advance does not expire an unchanged running process.
- Every defined event immediately revokes/cancels/zeroes.
- Terminal provider auth errors normalize to `ErrCredentialRejected`; transient and ambiguous transport errors do not destroy evidence or trigger blind retry.
- Credentials and `SessionRef` are absent from settings, selection, run state, receipt, journal, errors, logs, URL, argv, environment, browser storage, and telemetry sentinels.

## 8. Slice 4 — fixed listener, operator pipe, and browser session

### Work

1. Add `operatorchannel`:
   - Darwin verifier accepts only read-only `S_IFIFO` with `Nlink == 0` and rejects TTY/file/device/socket/named FIFO/`/dev/null`/unverifiable input;
   - exact `MINDLINE_PAIR_V1 <22-base64url>\n` parser, 64-byte bound, close-on-exec, and terminal malformed/EOF state;
   - launcher-construction tests prove no writer descriptor reaches the server or descendants; and
   - the launcher first positively verifies that its own stdin is the read end of a Codex-exec-owned anonymous `S_IFIFO`, `Nlink == 0`, close-on-exec, single-writer channel—not a PTY/file/path—and rejects otherwise;
   - a bounded launcher implementation creates the downstream pipe, owns its only writer, starts exactly the supplied verified executable with child argv `activation serve`, forwards operator confirmation frames without logging them, propagates graceful termination, and never contains a browser-open path; and
   - during an active proof the runner creates a separate anonymous payload-free controller-liveness pipe, retains its only writer, and passes the read end to both launcher and server; controller EOF cancels the server and group even if the launcher or proof runner crashes. After a committed successful proof, the ordinary launcher creates the equivalent launcher-lifetime pipe so launcher death still cancels the server; and
   - construction/adversarial tests prove an unrelated same-UID process has no pathname or writer capability and cannot inject a successful confirmation into either pairing channel or forge a live proof controller.
2. Add the source-controlled `mindline-operator-launcher` composition executable. It accepts exactly one absolute verified Mindline executable path, adds no server flags, and uses `operatorchannel.Launcher`; it is a Codex/operator artifact, not a second server mode. Codex starts it through a plain-pipe (non-PTY) controlled process session every time the PoC server is run.
3. Refactor CLI inputs:
   - production `Runner` retains `*os.File` for the operator channel separately from generic native-source input;
   - the exact frozen `${MINDLINE_FROZEN_BINARY} activation serve` command accepts no flags and is launched with a launcher-owned anonymous unidirectional stdin pipe; Codex exclusively controls the launcher process/input while the server and its descendants inherit no writer;
   - verify channel and compute pure identities, bind TCP4 `127.0.0.1:9876`, then and only then create/load the stable root;
   - bind failure emits `port_occupied`, no success URL, no probe/kill/fallback, and zero durable mutation;
   - success prints exactly `http://127.0.0.1:9876/\n` and never invokes `/usr/bin/open`.
4. Make `controlui.Launch` accept the already-bound listener and remove opener, random port, bootstrap fragment, and caller receipt path.
5. Replace bootstrap with the single streaming `POST /api/session/pair` protocol, one pending challenge, five-minute monotonic expiry, single-use confirmation, rate/cooldown controls, cancellation/replacement safety, and one active session.
6. Store only versioned session/CSRF keys in exact-origin `sessionStorage`; implement lock, stale/restart clearing, private DOM clearing, hash removal, and server instance binding.
7. Apply route-wide Host/TCP4 peer/query/cookie/Authorization/preflight/no-store/CSP/referrer/permissions limits; every API requires `X-Mindline-Origin`, paired reads require `X-Mindline-Session`, and mutations additionally require exact Origin and `X-Mindline-CSRF`.

### Files

- Add `internal/operatorchannel/{verify_darwin,frame,channel,launcher}.go` and tests plus `cmd/mindline-operator-launcher/main.go`.
- Modify `internal/cli/runner.go`, `internal/activationcli/run.go`, `internal/controlui/{launch,server}.go` and tests.
- Split control UI implementation into `pairing.go`, `session.go`, `security.go`, and `errors.go` if needed to keep files reviewable; this is internal package organization, not a fourth new capability package.

### Gate

- Fixed port/no-open/collision-before-mutation proof. The exact built and hashed operator launcher is exercised directly; PTY/file/path input to either layer is rejected. Every successful start/restart uses a fresh Codex-owned anonymous launcher-input pipe plus a fresh launcher-owned anonymous server-input pipe. Active-proof launches add a fresh payload-free controller pipe whose writer exists only in the runner. Injected controller/launcher/runner crash points prove both processes stop, the lock releases, the active receipt cannot be reused, and the next bind quarantines it before live authority.
- Pipe verifier, writer-leak, exact frame, partial/oversize/CRLF/NUL/extra/wrong/replay/EOF/post-EOF/concurrency matrix.
- Pair request exact-response binding; ordinary expiry permits one-action new code without operator-channel failure.
- `localhost`, IPv6, trailing dot, alternate port, DNS-rebinding Host, non-loopback peer, query/cookie/Auth, CORS/preflight, Origin/header/session/CSRF/body/content-type cases fail closed.
- Refresh/lock/restart and exact two-key browser storage contract; strict CSP/inert rendering/no private locked response.
- Provider credential fields and live routes remain hidden or blocked until Slice 5 authority integration.

## 9. Slice 5 — application state, UI, and authority cutover

### Work

1. Compose the stable repositories after successful bind. Register Slack adapter defaults and discover the exact receipt.
2. Add explicit application projections for session, gate, settings, active strategy, run selection, source/destination connection, and recovery enums from the Spec.
3. Add routes for:
   - paired state;
   - settings save/adoption/recovery;
   - use exact settings revision for proof;
   - run create/select/selection recovery;
   - lock; and
   - existing source/import/proof/review/delivery actions behind the new authority boundary.
4. New proof creation validates gate, selection CAS, settings CAS, terminal delivery state, then creates v0.4 evidence before selecting it. Selection failure leaves inert unselected evidence only.
5. The browser lifecycle explicitly creates and selects one provider-free v0.4 proof immediately after saving settings and before the first restart. It snapshots the exact active strategy and proves zero source items, credentials, provider network attempts, approvals, or destination mutations. That immutable run is the state exercised by graceful/crash restart and populated-root rollback; a fresh root no longer assumes v0.4 evidence already exists.
6. Replace HTML-owned values with server-owned hydration. Preserve exact current three lens texts, routing policy, and five ceilings. Track baseline/dirty state; never overwrite edits during refresh; expose explicit conflict diff/rebase, adoption, corrupt/unsupported recovery, compatible/incompatible prior proof, and next-proof divergence.
7. Keep visual styling; rewrite workflow copy around “Choose what matters → review a small proof → send only the drafts you approve.” Add plain actionable errors, changed/retry/correlation/focus data, on-demand credential fields, exact identity mismatch, and “reconnect never replays.”
8. Implement keyboard, focus, live-region, field-error association, no-color-only, 200% reflow, and reduced-motion behavior. Remove the duplicate manual-support field.
9. After all replacement tests pass, delete the old production bootstrap/random-port/opener/caller-receipt path. No dual authority remains.

### Files

- Modify `internal/activationapp/{types,app,source,proof,delivery,recovery,state_validation}.go` and tests.
- Modify `internal/controlui/server.go`, split files, and `assets/{index.html,app.js,style.css}` plus tests.
- Modify `internal/activationcli/run.go`, `internal/cli/runner.go`, CLI usage and tests.

### Gate

- Exact settings/default/adoption/hydration/save-time/version/generation behavior.
- Dirty refresh and conflict never lose edits; acknowledged rebase uses exact current CAS.
- Settings changes never mutate sealed strategy; incompatible prior proof is explicit and untouched.
- Missing Slack/Product Brain permits settings/review but blocks only actions that need that adapter.
- Reconnect identity mismatch changes nothing and never repeats preview/resume/approval/send.
- Existing exact approval, write locks, budgets, cancellation, readback, replay, and ambiguous-I/O reconciliation regressions pass unchanged.
- Accessibility contract passes deterministic DOM/state tests; final manual proof remains mandatory.

## 10. Slice 6 — frozen executable and proof

### Closed proof sequence

1. Finish implementation and focused tests without private source, real credentials, live transports, or browser navigation. Stage the exact pinned scanner executables.
2. Commit the frozen tree locally and require it to be clean. The outer Codex driver resolves the pinned owner-controlled tool root, generates a fresh random invocation id/generation, creates its unique no-replace controller-bootstrap root, executes exactly `go build -trimpath -o ${MINDLINE_PROOF_RUNNER} ./cmd/mindline-proof-runner`, captures build stdout/stderr plus build info/hash, captures exact `mindline-proof-runner/v1\n`, and atomically seals/fsyncs the invocation-ready evidence record. It then invokes exactly `${MINDLINE_PROOF_RUNNER} run --manifest ${REPOSITORY_ROOT}/internal/assurance/manifests/wp46-stable-control-v1.json --controller-bootstrap ${MINDLINE_CONTROLLER_BOOTSTRAP_ROOT}/controller-bootstrap-evidence.json`. These outer operations and artifacts are part of the signed manifest contract. A crash before exec leaves only an inert unique root; a retry generates another identity and never reads or overwrites the partial one.
3. The pinned runner bootstraps the new attempt: resolve static bindings; validate the external bundle against manifest/commit/tree/binary and its own actual invocation argv; hold a collision-check listener while quarantining prior fixed authority; create the fresh attempt identity/root/ledger; import the bundle byte-for-byte with no-replace; run `validate_controller_bootstrap`; then verify its own identity, Go 1.26.5, git 2.50.1, Product Brain CLI 0.1.0-beta.1584, the three scanners, and the pinned Codex Browser bundle/Chromium identity.
4. Materialize rollback baseline `17d33cb8845d2eab744cc605c310f54bc20fdb01` in a detached clean worktree. Build its `mindline` binary, the frozen-tree `mindline` binary, and the frozen-tree `mindline-operator-launcher` with Go 1.26.5 `-trimpath`; verify all source/build bindings before any contract group.
5. Preflight the only permitted old-binary command as `${MINDLINE_PRECHANGE_BINARY} activation config-fingerprint`. A Darwin process observer must report zero child/browser processes and no owner-root mutation. This proves the command is safe to use later; it is not yet the rollback proof. The old `activation serve --open` path is never invoked.
6. Execute every runner-owned group as exact `${MINDLINE_PROOF_RUNNER} group <signed-group-id>` plus the manifest's other exact commands: the closed `TestWP46_` inventory (including deterministic 30-day, graceful/crash restart, old-binary byte-identity, roll-forward, collision, pairing, settings, selection, gate, launcher, runner, lease, and accessibility lifecycle), full regression, race, vet, clean-tree/untracked check, scanners, and sentinels.
7. The exact runner seals one immutable fixed pre-live checkpoint bound to the imported controller-bootstrap evidence fingerprint, actual invocation argv, its own identity, and the fresh attempt. The exact frozen binary validates that already-sealed matching checkpoint and mints the fixed matching pre-live receipt with the same controller bindings without modifying the checkpoint. No prior receipt remains current; no browser/private/containment/final evidence is required or permitted in this receipt.
8. Through its non-PTY plain-pipe controller, start `${MINDLINE_OPERATOR_LAUNCHER} ${MINDLINE_FROZEN_BINARY}` in a dedicated process group. The group contains exactly the launcher and `${MINDLINE_FROZEN_BINARY} activation serve`; the runner records both PIDs, the PGID, and the server PID that owns TCP4 `127.0.0.1:9876`. Codex alone owns the upstream anonymous-pipe writer; the launcher alone owns the downstream writer; the proof runner alone owns the payload-free controller-liveness writer whose read end reaches both children. Startup must print only `http://127.0.0.1:9876/\n`; it never opens a browser.
9. Perform initial browser save/refresh/lock, then explicitly create/select one local provider-free v0.4 proof using those exact settings and prove zero provider/network/mutation activity. The proof runner sends SIGTERM to the exact two-member process group, proves both processes and the listener gone, then starts a new group with three fresh pipes; Randy re-pairs and reads back exact settings plus the compatible selected v0.4 proof. It sends SIGKILL to that entire second process group—not only the launcher—proves both processes and listener gone, starts a third group with three fresh pipes, and Randy re-pairs and completes crash/accessibility readback.
10. After the third frozen-server browser readback proves settings and v0.4 state survived both graceful and crash restart, stop that exact process group. Snapshot the populated stable root, run only `${MINDLINE_PRECHANGE_BINARY} activation config-fingerprint` in that same owner environment under the child/browser-process observer, and prove settings, v0.4, legacy v0.3, and every stable-root byte unchanged. Then start the frozen binary through a fourth fresh launcher/process group, re-pair, and prove the exact pre-rollback settings version, generation, fingerprint, and values return while credentials and pre-pair browser capabilities are absent.
11. Complete the capped private proof. At containment phase exit—even if any browser/private predicate failed—the browser controller attempts explicit provider disconnect, clears authority/storage/private DOM, the runner stops any live launcher/server with bounded SIGTERM then SIGKILL fallback, proves listener/process/lease/zeroization/no-further-use state, and the post-private runtime scan always runs. Only an all-pass containment result can continue to success proof.
12. Resolve `PRIVATE_RUN_ROOT` from the successful capped-proof export, then run readback and require exactly `readback/eval-readback/readback-summary.json`; run the safety proof gate and require exactly `proof-gate/eval-proof/proof-packet.json`; then run the Chain handoff audit. At finalization—even after any failure—baseline cleanup first runs whenever materialized. The closer evaluates the exact pre-closure set containing every other manifest group and excluding itself. Its immutable revalidation set includes the imported controller-bootstrap bundle/validation and actual run argv alongside the exact proof-runner version/bytes/build/source binding, clean HEAD/tree/status, source/embedded manifest, server/launcher, configuration/gate, checkpoint/receipt, namespace marker, and all 43 pre-closure records/artifacts. After double revalidation and durable index creation, `succeeded` is both the only success commit and the controller invocation's terminal-success evidence; no required write follows. Earlier drift/crash/failure closes controller liveness, stops processes, quarantines authority, and writes only failure evidence.

The pre-live receipt never binds or depends on the final evidence index. The final evidence index depends on the exact runner/attempt/receipt and every later proof stage. Any defect or source/manifest/runner/server-binary/launcher change restarts at step 2; containment, failed-attempt authority closure, and cleanup still run for the failed attempt.

### Founder browser checkpoint

Mindline is started without opening a browser. Randy clicks the fixed URL in Codex and performs the manifest's real-browser steps using Codex in-app Browser bundle `26.707.71524`, Chromium `150.0.7871.115`. Codex confirms only the exact displayed non-secret code over the existing launcher pipe. Owner-only evidence records hashes; screenshots containing private review evidence never enter the repo or Chain.

### Private capped proof

Only after the full pre-live manifest and receipt pass:

- accept explicit Slack/source scope or the occurrence-complete existing inventory;
- process up to three items per observed type/format stratum;
- review evidence/uncertainty;
- use the user-entered Product Brain credential only in process memory;
- preview and require Randy's exact batch approval;
- create drafts only through the app's Product Brain adapter, never runtime `pb`;
- read back and replay with zero created entries and zero created relations; and
- run eval readback plus `proof-gate --claim safety` and record founder usefulness.

At containment phase exit regardless of private-proof success, the app/browser controller disconnects Slack and Product Brain when present, locks and clears browser authority, and the runner stops the tracked launcher/server. Shutdown revokes/cancels any remaining process leases and zeroes credential buffers. A value-free post-stop probe must show the listener refuses connection, processes are absent, and provider-use counters remain unchanged during bounded observation; a second redacted runtime-surface gitleaks scan runs even when containment itself fails. Readback, Chain audit, and final success indexing require every containment predicate to pass.

### Review and defect loop

Two unchanged-tree delivery reviewers cite the same final evidence-index hash. Any defect, failed predicate, tree/binary/manifest/evidence change, ambiguous unreconciled Product Brain result, or reviewer disagreement first closes the attempt and removes its current live authority, then restarts build, gate, every manifest group, evidence indexing, and both clean reviews.

## 11. Rollback

- Before final cutover, the old launcher remains only in git history, never as a parallel runtime mode.
- If a pre-private gate fails, stop the new server; no live credential or transport was admitted. Fix the defect and restart the full manifest.
- If a live adapter fails before mutation, revoke the lease and preserve settings/run evidence.
- If Product Brain mutation is ambiguous, use existing durable reconciliation; never switch binaries or blindly retry.
- Only after exact settings and v0.4 evidence exist and have survived both graceful and crash restart, rollback invokes the pinned old binary's non-serving `activation config-fingerprint` command against that populated owner environment; it must leave settings, v0.4, legacy v0.3, and the entire stable root byte-identical and create zero browser process.
- Roll forward with the exact frozen new binary restores the exact saved settings version/generation/fingerprint/values, requires a new browser pair, and shows blank provider credentials plus no stale pre-pair browser capability.
- The proof runner removes its detached pre-change worktree during finalization; it retains only value-free binary/build/hash evidence needed by the final index.

## 12. Completion and Chain reconciliation

Completion requires all of the following on one frozen commit/binary/manifest/evidence index:

1. Every automated and manual proof-manifest predicate passes.
2. The pre-live receipt binds the exact attempt, commit, proof-runner version/bytes/build/source, server binary, operator launcher, configuration, manifest, source, tools, checks, and immutable pre-live checkpoint; the attempt has a unique no-replace evidence root, prior fixed authority was quarantined before it, and failed attempts leave no fixed receipt. The final committed state binds the successful index plus its immediate clean-tree/manifest/runner/server/launcher/checkpoint/receipt revalidation and rollback/roll-forward, browser, private, containment, post-live, and cleanup evidence.
3. Randy can click the stable link and see exact saved settings/proof state after refresh and restart without auto-open or a short application window.
4. The capped private run creates only exact approved Product Brain drafts; readback matches; replay creates zero mutations.
5. Founder usefulness is recorded separately from technical completion.
6. Two clean Product, Architecture, Chain, Delivery Quality, and Risk/Safety delivery reviews agree on the unchanged evidence index.
7. WP-46 and related decisions/tensions state the exact shipped behavior, remaining obligations, sample-bound claim, and next product-general improvement target.
8. `pb audit WP-46 --phase handoff --verbose` passes after Plan materialization and again at closure.

No completion claim is allowed for hosted availability, credential persistence, full-channel processing, semantic improvement, held-out generalization, production, or no-human operation.
