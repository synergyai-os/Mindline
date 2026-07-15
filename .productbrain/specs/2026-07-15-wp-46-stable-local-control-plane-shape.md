# Shape: WP-46 Stable Local Control Plane

**Status:** draft for role-panel review  
**Phase:** Shape  
**Version:** draft-7  
**Date:** 2026-07-15  
**Product:** Mindline  
**Materially reshapes:** WP-46 Slice 4 and the live-proof startup contract

## Problem

The current WP-46 browser surface is technically hardened but not durably usable. It binds to a random loopback port, requires a one-time fragment that exists only in the automatically opened browser tab, keeps the resulting session only in JavaScript memory, requires `--open`, and validates a pre-live receipt against a 30-minute wall-clock window. Closing or refreshing the tab loses authorization, returning after a meeting is confusing, and restarting later requires shell choreography. The current runtime path used for the founder proof is temporary, while the persisted activation aggregate is intentionally bound to one commit and configuration. That preserves run authority but cannot serve as a durable user-preferences store.

This is a product-architecture defect, not a copy or styling defect. Randy cannot reliably return to the PoC, and the visible UI therefore has no stable place in his workflow.

## Founder and product-team operating contract

Randy is the **Founder / Product Taste Maker**. He owns purpose, taste, problem and market truth, strategic steering, boundary-changing founder choices, and the final usefulness verdict. The empowered Mindline Product Team owns translating that steering into Chain-grounded product decisions, architecture, implementation, validation, and safe delivery. The team asks Randy only for a real founder/product decision that changes signed boundaries; it does not make him the day-to-day architect or delivery manager.

This clarifies, rather than replaces, DEC-414, STD-22, ROL-2, and the configured Product Team. Capture it as a durable decision clarifying the founder/Product Team boundary, not as a duplicate operational role. Randy remains an explicit human authority for irreducible source, destination, exact-batch, boundary-change, and usefulness decisions already required by WP-46.

## Appetite and selected direction

Build the smallest stable **local control plane**, not a hosted application:

1. Bind exactly `127.0.0.1:9876` and print only `http://127.0.0.1:9876/`.
2. Never open a browser. Randy opens the safe URL from Codex or another trusted surface.
3. Keep the server alive until explicitly stopped; there is no arbitrary pre-live or browser-session deadline while an unchanged authorized build is running.
4. Use a stable owner-only data directory by default, with an explicit test/development override.
5. Separate durable user preferences from immutable run/evidence state.
6. Persist only versioned, non-secret preferences. Product Brain keys, Slack tokens, browser session capabilities, CSRF capabilities, pairing authority, approval nonces, and source content never enter the preference store.
7. Keep browser pairing provider-neutral and keep bearer pairing authority out of URLs, browser fields, terminal output, logs, files, and Codex/chat history. The fixed base page creates one short-lived, single-use **non-secret challenge** and waits on the originating request. Randy asks Codex to confirm the displayed challenge. The single `mindline activation serve` process concurrently serves HTTP and consumes a closed pairing-confirmation grammar from an inherited, unidirectional anonymous stdin pipe whose write end is held only by the Codex launcher. The pipe exists before the fixed listener binds; the server rejects a TTY, regular file, `/dev/null`, environment value, argument, pathname, UID-only socket, HTTP call, or reusable token as operator authority. Another same-UID process cannot open the already-created write end or confirm a challenge. The channel never accepts provider credentials or destination authority. Only the originating waiting browser request receives a new session/CSRF pair. On pipe EOF or malformed input, pending/new pairing fails closed with visible recovery while an already-paired session may continue; restart always requires a new launcher pipe and fresh challenge. A future packaged/native launcher may implement the same pairing-authority port with an equivalently unforgeable parent/native channel.
8. After pairing, store only random session and CSRF capabilities in `sessionStorage`, scoped to the exact origin/port, so same-tab refresh works. Clear them on explicit lock and treat them as invalid after a server restart. No user/provider credential is stored in browser storage. This intentionally extends the current memory-only browser-capability rule and requires a specific Chain decision accepting the residual same-origin XSS/crash-restore risk in exchange for refresh recovery; CSP, no third-party/inline/eval script, no raw HTML/`innerHTML`, `textContent` rendering, and sentinel browser-storage proof remain mandatory. Governance and UI claims distinguish these browser-held bearer capabilities from provider credentials and private source content rather than claiming that browser storage contains no secrets of any kind.
9. Keep exact Host, loopback peer, Origin, JSON, body, timeout, CSP, no-store, XSS, CSRF, constant-time, approval-nonce, and write-lock boundaries. Pairing enforcement is process-wide and concurrency-safe, with one pending single-use challenge, a short explicit expiry, a fixed attempt/creation ceiling and cooldown, identical failure responses, non-secret counters only, exact challenge confirmation, and rotation after expiry or use. A challenge identifier alone cannot retrieve a session, and approval of one challenge cannot pair another browser request.
10. Every port-9876 bind collision fails closed. Mindline does not infer that an existing listener is itself, probe an unauthenticated instance as trusted, choose another port, kill a process, or print a success URL after bind failure.
11. Keep the commit/configuration-bound pre-live gate for private data, real credentials, and live transports, but remove wall-clock age as the authority boundary. A receipt is valid only for its exact clean build/configuration/gate-plan/source binding and becomes invalid on any drift. Startup recomputes and constant-time compares the current clean source binding to the receipt before live authority exists. The composition root discovers the matching receipt automatically; Randy never handles a receipt path or JSON. The UI may start locked without live authority; live provider connection and transport actions fail closed until a matching receipt exists.
12. Provider credentials remain process-memory-only and use a process-lifetime lease: they become invalid on process stop, explicit disconnect, provider revocation/rejection, identity drift, or safety/gate drift—not a hidden 15/20-minute idle or two-hour absolute timer. Missing credentials never erase settings/run progress; the UI asks for re-entry only when an action needs the provider.

## Persistence decision

Do **not** add a database in this slice.

Mindline already has crash-safe, owner-only, symlink-rejecting, atomic-file primitives and an append/fsync run journal. The preference workload is one bounded, versioned single-user document with no query, join, concurrency, or multi-process requirement. A database would add dependency, migration, lock, backup, and supply-chain surface without improving this user outcome.

Create an injected settings repository port with an owner-only JSON-file adapter for the PoC. The port, schema, and migration contract—not the file format—are the durable architecture. A later SQLite, bbolt, hosted, or tenant-aware adapter may replace it. Current bbolt documentation confirms it would provide ACID transactions and an exclusive read-write file lock, but those capabilities are unnecessary for this bounded preference object and would duplicate Mindline's existing private-file guarantees.

## Data ownership

| State | Owner | Lifetime | Upgrade behavior |
|---|---|---|---|
| Non-secret preferences (strategy draft, routing draft, resource ceilings, safe source defaults and known non-secret identities) | Platform settings repository | days/weeks | versioned migration; survives run and commit changes |
| Run strategy snapshot, frozen inventory, reviews, approvals, delivery evidence | Activation aggregate + run journal | immutable run lifetime | remains commit/configuration-bound; never silently migrated |
| Product Brain key and Slack token | integration session registry | current process/session only | re-enter after restart |
| Pairing challenge | control UI pairing boundary | one short-lived, single-use non-secret challenge | exact Codex/operator confirmation travels over the inherited launcher-bound channel; only originating request receives session |
| Browser session/CSRF | control UI session boundary | current server + exact-origin browser-tab session | random capabilities only in `sessionStorage`; invalid after restart/lock |
| Product Brain mutation authority | approved-delivery history | exact approved batch | unchanged |

The settings repository stores editable drafts, not authority. `Save strategy` creates the immutable run snapshot used for selection, processing, review, and delivery. A saved preference can seed a later run but cannot mutate a frozen or approved run.

Authenticated paired responses and DOM may contain only the bounded source evidence necessary for the review job. That evidence is rendered as inert text, stays out of browser persistence/cache/history, and is removed on lock or session invalidation. Provider credentials and private source content are forbidden from preferences, locked/unauthorized responses, errors, logs, URLs, telemetry, and browser storage. The sentinel claim does not forbid the narrowly necessary evidence shown inside an authenticated review session.

## Stable local lifecycle

1. Codex/operator starts `mindline activation serve` with no browser-opening flag.
2. Mindline selects the stable private data directory, verifies or initializes preferences, discovers current run/gate state, and binds exactly port 9876. Normal startup is one supported command; Randy never supplies a runtime directory, receipt file, or bootstrap URL.
3. Any bind collision fails with an explicit port-9876 recovery error. Mindline never claims that an unauthenticated listener is an existing Mindline instance, chooses another port, or kills a process silently.
4. Randy clicks the stable `http://127.0.0.1:9876/` link in Codex. Mindline does not open it.
5. The locked page creates and displays a non-secret, expiring pairing challenge and asks Randy to have Codex confirm it. It renders no private preference, run, source, or destination state and never asks for a provider credential.
6. Codex confirms the exact displayed challenge through the already-open launcher-bound operator channel. Successful confirmation releases a new session/CSRF pair only to the originating waiting browser request, which stores the pair in exact-origin `sessionStorage` and renders saved preferences plus current run state. Product Brain and Slack credentials are requested only inside this paired session and only when their adapters are needed. The normal form is hydrated with the exact saved text, limits, version, and save time; HTML defaults never mask saved values.
7. Refresh restores the same session. A stale session after process restart returns to the locked page without losing preferences.
8. Explicit disconnect revokes the destination lease. Explicit lock also revokes the browser session. Neither action deletes preferences or run evidence.

## User-state matrix

| State | User sees | Allowed action | Recovery |
|---|---|---|---|
| Server stopped | safe URL cannot connect | start Mindline | same fixed URL works after start |
| Port occupied | explicit startup failure naming port 9876 | inspect/stop the conflicting process | retry; never infer ownership or use a surprise port |
| Locked | non-sensitive running explanation and expiring non-secret challenge | ask Codex to confirm the displayed challenge | base page never requests provider credentials or exposes private state |
| Paired, gate missing/drifted | saved non-secret settings plus visible live-action blocker | read/edit safe preferences only | regenerate gate for current clean build before entering provider credentials |
| Unlocked | saved settings and authorized run state | configure, prove, review, approve, or explicitly lock | refresh keeps session in same tab |
| Browser refreshed | restored exact-origin tab session | continue | stale/missing session returns to the useful lock screen and needs a fresh Codex-confirmed challenge, not a server restart |
| Server restarted | lock screen, saved settings retained | pair this browser through a fresh Codex-confirmed challenge; reconnect a provider only when its adapter is needed | run resumes only if its immutable binding is valid |
| Saved strategy/lenses | exact saved values and visible saved version/time; sealed-run fields are read-only | edit an open draft or start a new proof | never replaces saved values with HTML defaults |
| Missing source credential | saved source identity/scope and a reconnect explanation | reconnect only when another fetch is needed | review/read-only work stays available |
| Missing destination credential | saved target identity and a reconnect explanation; confirms nothing new was sent | reconnect only before preview/resume/send | exact identity must match before mutation |
| Settings missing | built-in editable defaults | save settings/strategy | creates versioned preferences |
| Settings corrupt/unsupported | explicit recovery error; no silent overwrite | restore backup or start a new acknowledged settings version | run evidence remains untouched |
| Recoverable action error | plain cause, what changed or did not change, retry/fix action, correlation ID | retry after fixing the named issue | preserves non-secret form input; reconciles in-flight delivery first |

Every state transition is keyboard-operable and announced without relying on color. Pairing/expiry/error status uses an appropriate live region; focus moves to the next actionable control after pair, lock, expiry, retry, or fatal recovery; controls expose programmatic names, descriptions, loading and disabled state; and errors are associated with the relevant field/action.

## Product Model Fit Proof

- **User job:** return to Mindline reliably, see prior non-secret configuration in the normal form, and continue an evidence-backed ingestion run without reconstructing application state, interpreting JSON, or racing an arbitrary browser window.
- **Product object:** local control plane plus versioned operator preferences; run evidence remains a separate immutable object.
- **Source of truth:** preferences repository for editable defaults; run journal/activation projection for run authority; Product Brain approved-delivery history for mutation authority.
- **Comparable surfaces:** existing fixed-port judgment/concept servers, private-file primitives, run journal, integration session registry, and hardened control UI.
- **Eligibility:** **EXTEND** the canonical loopback control surface and Platform capability. This is reusable for future Slack, file, Notion, transcript, Product Brain, Tolaria, Notion, and other adapters because neither fixed startup nor preference ownership depends on one source or destination.
- **Not bespoke:** browser pairing is a provider-neutral port with an inherited launcher-channel adapter for the Codex-operated PoC; Product Brain and Slack are separate replaceable adapters requested only after pairing. Durable settings, session lifecycle, and run separation do not depend on either provider.

## Impact pack

### Upstream and downstream

- CLI/composition root: default data directory, fixed bind, receipt discovery, no browser opener.
- Platform: versioned settings repository, private permissions, atomic update, backup/migration contract.
- Control UI: locked/unlocked states, explicit Lock action, session restore, unlock rate limit, exact form hydration, saved-version visibility, and actionable human feedback.
- Activation application: current run stays authoritative; immutable strategy snapshots remain distinct from saved drafts.
- Integrations: provider connection is independent from browser pairing; process-lifetime credential leases remain opaque, revocable, identity-bound, and non-persistent.
- Assurance: exact build/configuration/gate/source binding remains and is recomputed at startup; wall-clock expiry no longer pretends to protect an unchanged binary.
- Tests/docs: fixed-port collision, no auto-open, reload/restart, preference survival, stale session, storage sentinel, corrupt settings, and gate drift.

### Governance

- Upholds DEC-414 and STD-22 empowered, Chain-bounded delivery.
- Extends STD-19 to an exact-origin `sessionStorage` capability after provider-neutral non-secret challenge confirmation over an inherited launcher-bound channel; requires a new active risk decision because the existing WP-46 spec required memory-only browser capabilities.
- Upholds STD-20, PRI-1, and BR-1 by excluding provider credentials and private source content from preferences, logs, telemetry, URLs, and browser storage.
- Keeps DEC-417 content/narrative intelligence as a built-in editable preference default.
- Does not resolve TEN-28 multi-outcome extraction.
- Creates the persistence seam needed to resolve TEN-29, but does not claim unlimited first-class lens management in this slice. It saves and restores the current bounded newline-parsed lens configuration without re-encoding the eight-active-lens processing cap as a storage-format constraint.

### Regression and rollback

- The random-port fragment launcher remains recoverable from git but is not a supported parallel path; two launch/auth patterns would create ambiguous security authority.
- Existing run journal, Product Brain delivery, exact approval, readback, replay, and source/destination adapter contracts remain unchanged.
- A new preference schema never migrates or rewrites run evidence.
- On rollback, the stable settings document is retained but ignored by older builds; no credential is stranded.

## Authority transition

- `sessionStorage` bearer capability persistence is a material trust-boundary choice. It requires a new active, verified Founder/Risk decision tied to this final Shape and the later signed Spec; the current JS-memory-only WP-46 authority cannot be stretched to cover it.
- WP-46 remains paused under its prior random-port/auto-open/memory-only signed hashes while Shape and Spec are revised. The final signed Spec must materialize this Shape without weakening existing draft-only, exact-approved, budgeted, cancellable, readback, replay, privacy, or claim boundaries.
- After Spec consensus and Chain capture, amend WP-46 to the exact final Shape/Spec/Plan hashes and reconcile its audit/gates. No implementation or delivery authority exists until the new signed Spec and Plan are independently reviewed, captured, materialized into WP-46, and pass applicable Product Brain audit.
- No shape, pairing, settings, or usability change authorizes unseen source processing, blanket Product Brain delivery, non-draft writes, generalization, production, or no-human claims.

## Outcome and proof

The slice passes when Randy can leave for a meeting, return later, run one supported start action, click `http://127.0.0.1:9876/` in Codex, have Codex confirm the displayed non-secret challenge, and see the exact prior non-secret strategy/settings in the normal form without browser auto-open, a bearer pairing URL, provider-coupled unlock, receipt/JSON choreography, or a 30-minute user window.

Required proof:

- exact port and no-auto-open CLI tests;
- real browser click/challenge-confirm/pair/lock/refresh/restart evidence, including expiry, wrong-challenge, concurrent-browser, stale-session recovery, and rejection of confirmation attempts outside the inherited launcher channel;
- preference save/reload, exact form hydration, elapsed-time simulation, and process-restart tests with blank secrets;
- cross-build settings compatibility proof with run authority still fail-closed;
- zero provider-credential or private-source-content sentinel hits in preferences, files outside existing authorized run evidence, browser persistence/cache/history, locked or unauthorized responses, errors, logs, URLs, or telemetry; bounded authenticated review evidence is separately proven inert, uncached, unpersisted in the browser, and cleared on lock/session invalidation;
- Host/Origin/peer/challenge origin-binding/launcher-channel closure and same-UID process isolation/pairing expiry/replay/concurrency/attempt-limit/CSRF/session/content-type/CSP/XSS/`sessionStorage` sentinel/port-squatting, saved-identity mismatch, provider-reconnect, and in-flight reconciliation adversarial tests;
- automated accessibility checks plus manual keyboard/screen-reader-oriented proof for pair, lock, expiry, loading, error, retry, focus recovery, and status announcements;
- current full tests, race, vet, security scanners, clean commit, fresh commit-bound pre-live receipt, readback, and exact Product Brain draft/replay proof.

## Exclusions

- hosted site, daemon/service installer, launch-at-login, cloud database, multi-user auth, OAuth marketplace, or OS keychain;
- persistent Product Brain or Slack credentials;
- changing the source-normalization, retrieval, semantic extraction, exact batch approval, or Product Brain writer contracts;
- automatic full-channel processing or delivery;
- treating a private founder run as held-out, generalized, production, or no-human proof;
- a full visual redesign;
- silently migrating immutable activation evidence across incompatible builds.

## Next phase

Run the complete Product, Architecture, Chain, Delivery Quality, and Risk/Safety defect panel on this exact Shape version. After a clean pass, produce a fail-able Spec that freezes session-storage semantics, settings schema/migration, gate validity, API routes, state copy, and lifecycle tests before updating WP-46.
