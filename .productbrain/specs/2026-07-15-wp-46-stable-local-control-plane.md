# Spec: WP-46 Stable Local Control Plane

**Status:** draft for role-panel review  
**Phase:** Spec  
**Version:** draft-7  
**Date:** 2026-07-15  
**Product:** Mindline  
**Materializes Shape:** `.productbrain/specs/2026-07-15-wp-46-stable-local-control-plane-shape.md` draft-7, SHA-256 `e4e1c6ae6ebab7f52efc24bfa1d4368de3d4570fbdf853ff5a39141c045e0f6e`  
**Authority anchors:** WP-46 (paused pending this replacement authority), DEC-418, DEC-419, DEC-414, STD-19, STD-20, STD-22, PRI-1, BR-1

## 1. Outcome

The PoC has one stable, useful local home at `http://127.0.0.1:9876/`. Starting Mindline never opens a browser. Randy can return days or weeks later, start the server with one supported command, click the same URL in Codex, pair that browser through a short non-secret code confirmed over the launcher-owned operator channel, and see the exact non-secret settings previously saved. Immutable proof state remains separate and fail-closed. Slack and Product Brain credentials remain process-memory-only and are requested only when an operation needs them.

Randy is Founder / Product Taste Maker: he owns vision, lived workflow and market truth, steering, taste, boundary changes, irreducible approvals, and the usefulness verdict. The empowered Product Team owns product decisions, architecture, implementation, evidence, and safe delivery inside signed Chain boundaries.

## 2. Scope and ownership

This slice extends the canonical local control-plane and Platform settings capabilities. It does not make Slack, Product Brain, Tolaria, Codex, a private corpus, or one provider part of Mindline core.

| Concern | Owning layer |
|---|---|
| Fixed local listener and launcher channel | CLI / composition root |
| Pairing, browser session, CSRF, locked shell | control UI boundary |
| Editable non-secret preferences | Platform settings repository |
| Frozen strategy and proof evidence | activation aggregate and run journal |
| Slack/Product Brain credentials and verified identities | integration session registry and adapters |
| Build/configuration/gate/source authorization | assurance boundary |
| Exact approved draft delivery | existing Product Brain destination adapter |

## 3. Supported command and lifecycle

The supported production-like PoC command is:

```text
mindline activation serve
```

The command:

1. positively validates that stdin is the read end supplied by the supported anonymous-pipe launcher topology rather than accepting a generic `io.Reader` or FIFO claim;
2. resolves the private data root;
3. computes current clean source, executable-artifact, configuration, and gate-plan identity;
4. binds TCP4 `127.0.0.1:9876` before any settings initialization, migration, recovery, run-selection, or write;
5. loads settings and the exact compatible run/gate projection;
6. concurrently consumes pairing confirmations from stdin and serves HTTP;
7. prints exactly the safe base URL and newline after a successful bind; and
8. runs until signal, context cancellation, or fatal server failure.

There is no `--open`, random port, fallback port, browser invocation, bootstrap fragment, receipt-path argument, or provider credential argument. A bind collision exits nonzero with `port_occupied` before any durable mutation or success URL. Mindline never probes the listener as trusted, kills it, or claims it is an existing Mindline process.

Tests and local development may pass an explicit absolute data-directory override through dependency-injected Go composition options. It is not a public CLI flag or part of Randy's normal start command. Production-like startup rejects relative, symlinked, non-owner, or over-permissive roots.

## 4. Stable data root and run selection

The default root is derived with `os.UserConfigDir()` and the path elements `Mindline/control-plane`. On macOS this normally resolves under `~/Library/Application Support/Mindline/control-plane`. The root is owner-only `0700` and is created without following symlinks.

```text
<root>/
  control/
    settings.json
    settings.backup.json
    settings.lock
    selected-run.json
    selected-run.backup.json
  assurance/
    pre-live-receipt.json
  runs/
    <run-id>/
      activation-state.json
      journal/
      delivery/
```

Run selection uses the separate non-authorizing `selected-run.json`, never “latest”, glob order, modification time, settings, or a user-provided path:

```json
{
  "schema_version": "mindline.control-run-selection/v1",
  "version": 3,
  "generation": "<256-bit-random-non-authorizing-generation>",
  "selected_run_id": "run-20260715T143000Z-<128-bit-base32>",
  "fingerprint": "sha256:<canonical-selection-hash>"
}
```

Missing selection means a settings-only shell with no active proof. Read-only `RunIntegrity` is separate from `LiveTransportAuthority`: exact state, journal, schema, executable artifact, clean commit, and configuration integrity permits a compatible run projection even when the receipt/gate is missing; receipt, source, and gate readiness control credentials and live operations only. An invalid selection produces explicit `run_blocked` state and never falls back to another run. A new proof requires current live-gate readiness, creates a no-replace owner-only run directory, initializes immutable evidence, and only then compare-and-swap updates selection over exact `{version, generation}`. Pointer failure leaves an inert unselected run that grants no authority. Every selection create, select, clear, or recovery rotates the random generation; its fingerprint covers generation, preventing ABA acceptance when a display version is reconstructed.

Selection mutation uses the same owner-only nonblocking advisory lock and last-valid atomic persistence protocol as settings: strictly validate current, persist and fsync the last valid bytes to `selected-run.backup.json`, atomically replace `selected-run.json`, fsync the directory, and reread the exact result. The backup is never selected automatically and grants no authority.

A corrupt or unsupported `selected-run.json` is never guessed around. `POST /api/runs/recover-selection` requires the exact problem fingerprint, current selection `{version, generation}` CAS pair when readable, and a literal acknowledgement, and can either clear selection or replace it with one explicit fully validated `run_id`. If current selection is unreadable, recovery uses the valid selection backup's version plus a fresh generation, or version 1 plus a fresh generation when no valid backup exists. It atomically changes only the non-authorizing pointer and never modifies, deletes, or migrates run evidence.

Settings survive commit and run changes. Run evidence does not silently migrate across incompatible bindings. Prior runs remain untouched. The UI explains when saved settings are available for a new proof but a selected prior proof is incompatible with the current build. Selecting a run never grants live transport or delivery authority. Legacy disposable runtime roots are not auto-discovered or migrated.

## 5. Settings repository port

### 5.1 Interface

The application depends on a source/destination-neutral repository port with these semantic operations:

```text
Load() -> SettingsSnapshot
Save(expectedRevision {version, generation}, draft) -> SettingsSnapshot
InspectRecovery() -> SettingsRecoveryState
RestoreBackup(problemFingerprint, acknowledgement) -> SettingsSnapshot
ReplaceWithDefaults(problemFingerprint, acknowledgement) -> SettingsSnapshot
```

The JSON/private-file adapter is the only implementation in this slice. No database is added. The port and canonical schema permit a future SQLite, bbolt, hosted, or tenant-aware adapter without changing activation or UI business rules.

### 5.2 Canonical document

```json
{
  "schema_version": "mindline.control-settings/v1",
  "version": 4,
  "generation": "<256-bit-random-non-authorizing-generation>",
  "saved_at": "2026-07-15T14:30:00Z",
  "fingerprint": "sha256:<canonical-document-hash>",
  "draft": {
    "context_lenses": ["Product Brain landscape", "AI-native organization design", "Content and narrative intelligence"],
    "routing_policy": "...",
    "drain_policy": {
      "maximum_network_requests": 5000,
      "maximum_wall_time_seconds": 14400,
      "maximum_cost_microunits": 1000000,
      "maximum_retry_attempts": 2000,
      "manual_support_tolerance": 250
    },
    "adapter_defaults": [
      {
        "slot": "source",
        "adapter_kind": "slack_web_api",
        "schema_version": "mindline.source.slack-web-api-defaults/v1",
        "values": {"channel_id": "C0123456789"}
      }
    ],
    "expected_source_identity": null,
    "expected_destination_identity": null
  }
}
```

`context_lenses` is an ordered, bounded list in storage rather than a fixed field set. This slice may render the list through the current newline editor and may retain the current eight-active-lens processing limit, but the repository must not encode eight columns or otherwise prevent TEN-29's later unlimited first-class lens model.

The server-owned v1 defaults reproduce the current three complete editable lens texts, routing policy, and all five drain-policy ceilings exactly, including DEC-417 content and narrative intelligence. The HTML contains no authoritative strategy or ceiling values.

Adapter-specific defaults live in a versioned discriminated envelope `{slot, adapter_kind, schema_version, values}`. Core settings validates the envelope and delegates the strictly closed, non-secret `values` schema to the registered adapter; adding a future source or destination adapter does not change the core settings document. The Slack adapter alone owns `channel_id`. Unknown adapter kinds, schemas, fields, duplicates, or slots fail closed.

Expected identities contain only adapter kind plus stable verified non-secret workspace/key/channel identifiers already returned by `integrations.VerifiedIdentity`. Provider credentials, lease/session/connection identifiers or times, browser authority, pairing material, approval nonces, source content, arbitrary URLs or file paths, Slack drain-window timestamps or frozen denominator, destination write/attempt budgets, receipts, run fingerprints, and delivery state are forbidden. Processing resource ceilings remain editable defaults; mutation authority never does.

### 5.3 Validation and persistence

- Maximum encoded document: 64 KiB.
- Strict closed JSON: exact schema, no unknown keys, no duplicate keys, no trailing data.
- UTF-8 strings and list/count/numeric limits are validated before canonicalization.
- Secret-shaped field names and values are rejected without reflecting the value in errors.
- The canonical fingerprint excludes the fingerprint field itself and covers every other semantic field, including an opaque random 256-bit non-authorizing `generation`.
- `version` is `0` for unsaved built-in defaults, begins at `1` on first save, and increases by exactly one per ordinary successful save. Recovery uses `valid_backup.version + 1`, or `1` when no valid backup exists; a display version may therefore repeat one seen before corruption, while the rotated generation preserves CAS uniqueness.
- `Save` uses compare-and-swap over both `expected_version` and `expected_generation`. A mismatch changes nothing. Every successful save or recovery rotates generation, so restoring a backup cannot collide with a stale client even if the reconstructed display version equals a formerly observed version.
- The root, files, and lock must be regular, owner-owned, non-symlink objects with modes `0700` and `0600` as applicable.
- All operations are serialized by an in-process mutex plus a nonblocking OS advisory lock held on an owner-only regular `0600` descriptor. Lock contention fails closed; the kernel releases the lock after crash/process exit. Multi-process writers are unsupported.
- Save sequence: validate current; preserve the last valid canonical bytes as backup; fsync backup; atomically replace current; fsync current and directory; reread and validate the exact persisted document before success.
- Crash-stage outcomes are deterministic: before current replacement, the previously valid current remains authoritative; after atomic replacement, startup accepts only the fully valid new current; a missing/torn/invalid current enters explicit recovery and never auto-adopts backup. Fault-injection tests cover every boundary before/after backup fsync, current temp fsync, rename, directory fsync, and reread.
- Missing settings returns server-owned editable defaults with state `defaults`; it does not write until the user saves.
- If settings are missing and an exactly compatible selected run contains a strategy snapshot, state exposes `adoption_available` and its non-secret exact values. The explicit **Save current proof settings as defaults** action copies those values into a new settings document; it never rewrites or treats the run as migrated.
- Corrupt, unsafe, or unsupported settings returns an explicit recovery state and never auto-overwrites, auto-falls back, or changes run evidence.
- Restore and replace require the exact current problem fingerprint and a literal acknowledgement. A changed problem fails with a version conflict.

## 6. Operator confirmation channel

The supported launcher creates an anonymous unidirectional pipe before starting `mindline activation serve`. Only the launcher retains the write end; the server receives only the read end as stdin. Unused ends are closed and every pipe descriptor is close-on-exec before any provider subprocess or child can exist.

Production `activationcli` accepts an `*os.File`, not a generic `io.Reader`, and passes it through an `OperatorChannelVerifier`. On Darwin the verifier requires a read-only `S_IFIFO` descriptor with `Stat_t.Nlink == 0`. TTY, character device, regular file, `/dev/null`, named FIFO, socket/path input, and an unverifiable platform fail startup. An injected verifier is allowed only in tests. Anonymity and single-writer ownership are additionally proved by executable launcher construction and descriptor-leak tests, not overclaimed from `fstat` alone.

The only accepted frame is:

```text
MINDLINE_PAIR_V1 <22-character-unpadded-base64url-challenge>\n
```

The challenge encodes 128 random bits. A frame is at most 64 ASCII bytes. CR, NUL, extra whitespace, extra fields or bytes, unknown version, invalid alphabet/length, oversized or incomplete frames, repeats, and confirmations without the exact pending challenge are rejected. The frame and challenge value are never logged.

Malformed or oversized input permanently disables new pairing until restart. EOF cancels a pending challenge and disables new pairing; an already paired browser session may continue. The operator pipe accepts no credentials, settings, delivery authority, or generic commands.

## 7. Browser pairing and session

### 7.1 Pairing route

`POST /api/session/pair` requires:

- exact request Host `127.0.0.1:9876`;
- a TCP4 loopback peer;
- exact Origin `http://127.0.0.1:9876`;
- `Content-Type: application/json`;
- exact body `{}` within the global size and read deadline; and
- no cookies, authorization header, query, or trailing body.

The response is `application/x-ndjson`, `Cache-Control: no-store`, and remains on the same originating request:

```json
{"type":"challenge","challenge":"q1w2e3r4t5y6u7i8o9p0_A","expires_in_seconds":300}
{"type":"paired","session":"<256-bit capability>","csrf":"<independent 256-bit capability>","server_instance":"<random instance id>"}
```

The page displays and offers a copy control for the exact 22-character canonical challenge; no shorter alias can approve the wrong request. It groups the code visually without changing copied bytes, gives it an accessible label that announces each character unambiguously, and provides a one-action **Create a new code** recovery after expiry. Ordinary expiry does not count as malformed operator input or trigger the abuse cooldown by itself. The second frame is emitted only after the operator pipe atomically confirms that exact challenge while the same response is still connected and pending. There is no polling, redeem, bootstrap, or second retrieval route. A challenge alone cannot create or retrieve a session.

Process-wide rules:

- at most one pending pairing request;
- five-minute monotonic expiry, long enough for a normal Codex round trip but short relative to the durable server/session lifecycle;
- one confirmation attempt per challenge;
- at most three incomplete challenge creations in a rolling minute, then a 60-second cooldown;
- disconnection, cancellation, expiry, replacement, malformed confirmation, or use invalidates the challenge;
- one active browser session; successful new pairing atomically revokes the old one; and
- failure responses are value-free and do not reveal which check failed.

### 7.2 Session and CSRF

Session and CSRF are independent 256-bit random capabilities. The client stores only these two versioned keys in exact-origin `sessionStorage`. Cookies, localStorage, IndexedDB, Cache API, service workers, URLs, DOM text, logs, and files are forbidden for capabilities.

All paired reads require the session. Every mutation requires exact Host, peer, Origin, session, CSRF, method, content type, bounded strict body, and constant-time capability comparison. Restart, successful replacement pairing, explicit lock, or rejected/stale session invalidates browser authority.

On startup the client clears any URL hash with `history.replaceState` before enabling pairing; fragments never reach the server and URL state never carries authority. On an unauthorized response the client first clears the two storage keys and private DOM, then renders the locked shell. Refresh may restore a valid session in the same tab. Crash-restored stale capabilities are rejected by the new server instance. CSP forbids third-party, inline, eval, worker, object, frame, and external connection paths. Dynamic/private values use inert `textContent`; `innerHTML` and raw source HTML are forbidden.

`POST /api/session/lock` clears the client storage before transmission using local copies, then atomically revokes the server session, CSRF, pending challenge, and browser-review nonces. It does not disconnect provider leases or delete settings/run/delivery evidence. If acknowledgement is lost, the client says that lock is unconfirmed and stopping Mindline guarantees revocation.

## 8. Assurance and credential lifecycle

The composition root discovers exactly `<root>/assurance/pre-live-receipt.json`. There is no fallback, glob, newest-file selection, or user path. `mindline activation gate-receipt` writes only that discovered path under the default root; test composition may inject a private root. The receipt is strictly validated for schema, fingerprint, full check set, tool identities, runner version, gate-plan fingerprint, commit, configuration, authorization flags, and source-binding fingerprint. `GeneratedAt` remains well-formed informational evidence and is not an expiry authority.

The receipt schema and its own fingerprint include `build_artifact_fingerprint`, a canonical SHA-256 over the exact executable bytes used to run the gate. The supported flow gates and serves with the same built executable; `go run` invocations that produce different temporary artifacts cannot share live authority. At server startup Mindline recomputes its own executable fingerprint and constant-time compares it with the receipt.

At startup and immediately before credential acceptance or any live Slack, retrieval, Product Brain preflight, reservation, or socket attempt, Mindline recomputes the executable and current clean source bindings and constant-time compares them with the receipt. Drift blocks new transport, revokes provider leases, and cancels queued work. Ambiguous destination I/O already in flight enters the existing reconciliation path and is never blindly retried.

The locked UI may run without a valid receipt. A paired user may review and edit settings and read existing compatible proof evidence while the gate is missing. Provider credential fields and all live network actions remain unavailable until the exact gate is ready. A receipt created or changed after startup does not hot-upgrade authority; restart is required so one server instance has one frozen assurance projection.

Integration leases become process-lifetime leases with the exact registration shape `LeaseOptions{Kind, Secret, Identity}` and no TTL fields. Live `ConnectionSnapshot` removes idle/absolute expiry fields. v0.4 persistence stores only adapter kind plus `VerifiedIdentity`, never a live snapshot, `ConnectionID`, or session reference. Leases are valid only while the unchanged server instance, verified provider identity, exact gate, and source binding remain valid. A provider-neutral terminal `ErrCredentialRejected` atomically revokes the lease, zeroes secret bytes, cancels its context, and prevents further use. The same occurs on process stop, explicit disconnect, provider revocation, identity mismatch/drift, or gate/source drift. Locking the browser does not implicitly revoke a provider lease, but a new paired session is required to access any action that could use it.

Credentials are bounded byte buffers, removed from the input DOM immediately after submission, and never reflected. They never enter settings, journals, receipts, responses, errors, telemetry, URLs, browser storage, command arguments, or environment variables.

## 9. HTTP surface

Before routing any request, the server requires exact Host `127.0.0.1:9876`, a TCP4 loopback peer, no query, and no cookies or `Authorization` header. Every `/api/*` request requires an exact `X-Mindline-Origin: http://127.0.0.1:9876` header; a cross-origin browser cannot add it without a rejected preflight. Pairing and every mutation additionally require the browser-supplied Origin to equal that exact origin. Any supplied Origin or `Referer` on a read must also match. `OPTIONS` and CORS preflight are rejected. Paired requests use only `X-Mindline-Session`; mutations additionally use `X-Mindline-CSRF`. Every dynamic/API response, including success, error, unauthorized, pairing, and private state, inherits `Cache-Control: no-store`; security headers and bounded request/response timeouts apply route-independently.

| Method and route | Authentication | Contract |
|---|---|---|
| `GET /` | locked/public | Static useful locked shell only; contains no settings, identities, evidence, or capabilities |
| `POST /api/session/pair` | exact local boundary | Single streaming challenge/pair response defined above |
| `POST /api/session/lock` | session + CSRF | Revokes browser authority only |
| `GET /api/state` | session | Authoritative paired view |
| `PUT /api/settings` | session + CSRF | Saves non-secret settings with `expected_version` and `expected_generation` |
| `POST /api/settings/adopt-active-strategy` | session + CSRF | Creates settings from an exactly compatible run snapshot only after explicit confirmation |
| `POST /api/settings/recover` | session + CSRF | Explicit backup/default recovery with problem fingerprint and acknowledgement |
| `POST /api/commands/use-settings-for-proof` | session + CSRF | Snapshots exact settings version, generation, fingerprint, and values into an open activation run |
| `POST /api/runs` | session + CSRF | Creates and selects a new proof only when no delivery is in flight and any approved batch is terminal |
| `POST /api/runs/select` | session + CSRF | Selects one explicit existing `run_id` only after exact validation; never infers latest |
| `POST /api/runs/recover-selection` | session + CSRF | Explicitly clears or replaces only a corrupt selection pointer; never changes run evidence |
| Existing import/source/destination/proof/review/delivery routes | session + CSRF where mutating | Preserve existing exact approval, budgets, readback, replay, and recovery semantics |

The existing `POST /api/bootstrap` and bootstrap fragment are removed. `save-strategy` is replaced by the explicit settings save plus proof snapshot routes. Reconnecting a provider never auto-repeats preview, resume, approval, send, or another mutation; Randy takes a fresh explicit action.

## 10. State projection

`GET /api/state` returns explicit enums rather than forcing the browser to infer workflow from booleans:

```text
session:
  state: paired
  server_instance
gate:
  state: ready | missing | drifted
  reason_code
  operator_action
settings:
  state: defaults | saved | adoption_available | corrupt | unsupported
  schema_version
  version
  generation
  fingerprint
  saved_at
  editable
  edit_blocker
  draft:
    context_lenses
    routing_policy
    drain_policy
    adapter_defaults
active_strategy:
  state: absent | open | sealed
  settings_version
  settings_generation
  fingerprint
  sealed_at
  exact_values
run_selection:
  state: none | compatible_selected | incompatible_preserved | blocked
  version
  generation
  selected_run_id
  safe_prior_run_reference
  reason_code
connections.source / connections.destination:
  state: not_configured | connected | credential_required | identity_mismatch
  known_identity
  credential_required_for[]
  reason_code
recovery:
  state: ready | recovering_delivery | action_blocked | fatal
  code
  user_message
  changed
  retryable
  correlation_id
  field_id
  focus_target
```

Saved editable settings and the active proof strategy are distinct objects. Changing settings never changes a sealed proof. When they differ, the UI says the saved values apply to the next proof and displays the sealed values read-only.

## 11. UI behavior

The primary workflow copy is: “Choose what matters → review a small proof → send only the drafts you approve.” Technical terms such as denominator, strata, fingerprints, and projection remain inside optional evidence details.

The browser fetches authoritative state before enabling settings controls. `saved` hydrates exact server values and shows saved time and version. `defaults` uses defaults returned by the server and says “Not saved yet.” HTML literals never override server values.

The client keeps a baseline and dirty state. Background refresh never overwrites unsaved edits. Save sends `expected_version` and `expected_generation`; success replaces the baseline with the exact returned document. A `409 settings_version_conflict` preserves local edits and returns the current safe settings snapshot/version/generation plus the fields changed from the submitted baseline. The UI shows the field-level conflict and offers:

- **Reload saved version** — explicitly discards local edits and hydrates the returned current version.
- **Review and save my edits** — keeps local edits, changes their acknowledged base to the returned current version/generation only after Randy reviews the conflicting fields, and retries with that exact new CAS pair. Another concurrent change conflicts again; there is no unconditional overwrite.

The UI exposes separate primary actions:

- **Save settings** — persists future defaults.
- **Use these settings for this proof** — snapshots the exact saved version into an open proof.

If the proof is sealed or the selected prior proof is incompatible, the second action is disabled with an explanation and **Start a new proof** backed by `POST /api/runs`. The request includes the exact run-selection version/generation, exact saved settings version/generation/fingerprint, and literal confirmation that the selected prior run will remain untouched. Starting or selecting a proof fails while delivery is in flight or an approved batch is non-terminal; a selection/settings conflict changes nothing. Every settings-derived run snapshot is bound to the exact settings `{version, generation}` plus fingerprint and values.

Missing Slack never blocks settings or existing review. Missing Product Brain never blocks settings, proof review, or founder review. The UI requests reconnection only when a chosen action needs that adapter, names the saved expected identity, and changes nothing on mismatch.

Lock copy states that locking hides the browser session but does not disconnect Slack or Product Brain; explicit Disconnect revokes those provider connections.

## 12. Error contract

Safe JSON errors use:

```text
error_code
user_message
changed: none | <exact durable change>
retryable
recovery_action
correlation_id
field_id
focus_target
```

Required codes include `session_stale`, `pairing_expired`, `pairing_channel_unavailable`, `gate_missing`, `gate_drifted`, `build_artifact_mismatch`, `source_credential_required`, `destination_credential_required`, `identity_mismatch`, `settings_version_conflict`, `settings_corrupt`, `settings_unsupported`, `run_selection_corrupt`, `run_incompatible`, `run_sealed`, `delivery_reconciling`, and `port_occupied`. Errors never echo credentials, private source content, challenges, capabilities, or internal paths. `changed` reports the exact durable change or `none`; it never implies a rollback that did not occur.

## 13. Accessibility contract

- Pair, lock, reconnect, save, conflict resolution, retry, review, approval, and cancellation are fully keyboard operable without focus traps.
- Pairing, expiry, save, error, and recovery are announced through appropriate live regions without countdown spam.
- Focus moves to the next actionable control after pair, lock, expiry, validation failure, retry, and fatal recovery.
- Field errors use `aria-invalid` and `aria-describedby`; dynamic controls retain programmatic names, state, and loading/disabled semantics.
- No state relies on color alone; visible focus, 200% zoom/reflow, and reduced-motion behavior are verified.

## 14. Compatibility and rollback

- Existing activation aggregates, journals, exact approvals, approved-delivery history, readback, replay, and adapter contracts remain authoritative and are not rewritten by settings work.
- Stable-control runs cut over to `mindline-trusted-activation-state/v0.4`, which persists stable verified identities but no live lease deadlines. Existing v0.3 runs remain immutable/read-only and are never silently upgraded.
- Old random-port/bootstrap launch is removed rather than kept as a second security authority.
- An older binary ignores the retained stable settings document; it does not delete or downgrade it.
- A newer unsupported settings schema fails closed and offers only explicit acknowledged recovery.
- The implementation remains behind the CLI/composition boundary until proof passes. Rollback restores the previous binary and leaves settings and old run evidence untouched; provider credentials are never stranded.

## 15. Fail-able acceptance projection

**Intended user:** founder/operator now; later any single-user local Mindline operator.  
**Inputs/sources:** saved non-secret configuration plus existing Slack source adapter; future adapters must fit without changing core settings/session contracts.  
**Outputs/destinations:** existing review surface and Product Brain draft-only adapter; destination-neutral settings and proof semantics.  
**Workspace:** one owner-operated local machine and one server process; multi-user and daemon service excluded.  
**Provider/model:** provider-neutral control plane; no semantic-quality improvement claim.  
**Privacy:** private source content remains in authorized run evidence and bounded authenticated review only; no provider credential persistence.  
**Sample status:** Randy's run is usability/private-runtime proof, not held-out or generalization proof.  
**Claim:** local lifecycle and persistence safety for this bounded PoC only; no production, no-human, full-channel, or generalized extraction claim.

All automated lifecycle and boundary tests use injected wall and monotonic clocks, entropy, filesystem fault points, process/signal/anonymous-pipe hooks, listener factory, executable/source/configuration binding providers, and provider transports. They contain no wall-clock sleeps or ambient network dependency. A separate real-browser lifecycle is pinned to the exact browser product/version and built executable and records the controlled local environment.

The implementation Plan must freeze a canonical proof manifest on the exact commit. For every acceptance item it records command and arguments, tool and version, environment/configuration fingerprint, expected artifact, and machine-checkable pass predicate. The final evidence ledger and pre-live receipt reference the proof-manifest hash. A defect or tree change invalidates prior evidence: rebuild the executable, regenerate the gate, and rerun the entire manifest on the corrected frozen commit before either clean reviewer pass counts.

The slice fails if any of these proofs fail:

1. Fixed-port/no-open: only TCP4 `127.0.0.1:9876`; collision is nonzero before mutation or success output; no browser subprocess, fallback, probe-as-trust, or kill.
2. Pair topology: TTY/file/device/socket/path injection rejected; child/descendants hold no writer; close-on-exec; unrelated same-UID process cannot confirm.
3. Frame/parser: partial, oversized, CRLF, NUL, extra token/data, wrong version/challenge, replay, EOF, and post-EOF cases fail closed.
4. Origin binding: `localhost`, IPv6, trailing dot, alternate port, DNS-rebinding Host, non-loopback peer, CORS/preflight, cross-request redemption, expired/disconnected/concurrent browser fail.
5. Limits: five-minute monotonic expiry, creation ceiling, cooldown, one attempt, blocked reader, and restart cases are deterministic and value-free.
6. Browser authority: refresh succeeds with exactly two `sessionStorage` keys; lock/restart/stale session clears; no cookie/localStorage/IndexedDB/cache/service-worker/URL/DOM/log/file capability hits.
7. XSS/privacy: strict CSP; no inline/eval/third party/worker/`innerHTML`; private evidence is inert, uncached, unpersisted, and cleared on lock.
8. Settings and selection: exact save/reload/hydration/version/time; dirty refresh and conflict preserve edits; wrong mode/owner, symlink, replacement race, oversize, unknown/duplicate/trailing fields, corruption, unsupported schema, current/backup corruption, and crash at every write stage fail safely. Selection backup is never auto-adopted and recovery changes only the pointer.
9. Upgrade separation: settings survive a build change; incompatible immutable run evidence is not migrated or selected; sealed active strategy never changes when settings change.
10. Gate: wrong/missing/duplicate receipt, executable-artifact/commit/config/source/gate-plan/check/tool/fingerprint drift, startup drift, and drift immediately before live mutation block transport and revoke leases.
11. Credentials: elapsed-time simulation preserves a valid process-lifetime lease; restart, disconnect, provider rejection, identity mismatch, or gate drift revoke it; sentinel absent from every forbidden surface on success and failure.
12. Authority: pairing never creates batch approval; reconnect never replays mutation; lock never deletes durable evidence; write locks, exact approval nonce, budgets, cancellation, readback, replay, and delivery reconciliation remain enforced.
13. UI/accessibility: defaults/saved/corrupt/unsupported/open/sealed/divergent/recovering states; actionable error/correlation/focus behavior; keyboard, live-region, field association, 200% reflow, visible focus, and reduced-motion proof.
14. Lifecycle: real browser start → click stable link → pair → save exact settings → refresh → lock/re-pair → restart → re-pair → see exact settings and compatible proof progress, with blank credentials after restart.
15. Days/weeks and rollback: save settings; advance injected wall and monotonic clocks by at least 30 days; verify exact values, fingerprint, version, and generation after graceful and crash restart; run the pre-change binary against the same owner data environment and prove the stable root plus legacy v0.3 evidence are byte-for-byte unchanged; roll forward to the exact frozen new binary and prove the same settings return while provider credentials and browser capabilities are absent.
16. Repository quality: every focused, integration, browser, adversarial, full-test, race, vet, security, readback, and exact Product Brain draft/replay command in the closed proof manifest passes on the same frozen tree and built executable.

## 16. Exclusions

- hosted site, daemon/service installer, launch-at-login, multi-user authentication, OAuth marketplace, OS keychain, database, sync, or cloud settings;
- persistent Slack/Product Brain credentials or browser authority beyond exact-origin `sessionStorage` for the current server instance;
- full lens-management UX, full visual redesign, or resolution of TEN-29;
- source-normalization, retrieval, semantic extraction, destination writer, approval, or Product Brain REST transport redesign;
- automatic full-channel processing/delivery, non-draft writes, generalized/production proof, or no-human autonomy;
- silent run migration or automatic recovery that overwrites settings/evidence.

## 17. Authority and delivery gate

This draft grants no implementation authority. The complete Product, Architecture, Chain, Delivery Quality, and Risk/Safety role panel must review the exact Spec hash twice cleanly. After consensus, capture and verify the signed Spec, then make WP-46 exactly materialize its outcome, architecture, acceptance/proof contract, residual risks, exclusions, delivery sequence, and exact Shape/Spec hashes without weakening or inventing scope. Explicitly supersede the prior random-port/auto-open/memory-only hashes and pass or reconcile `pb audit WP-46 --phase handoff --verbose`. Then create the fail-able implementation Plan, review it twice cleanly, capture it, add its exact hash and proof-manifest obligation to WP-46, and rerun `pb audit WP-46 --phase handoff --verbose`. Implementation begins only after that final handoff audit passes.
