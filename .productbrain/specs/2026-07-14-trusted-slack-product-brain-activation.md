# Spec: Trusted Slack-to-Product-Brain Activation and Gated Drain

**Status:** draft for role-panel review  
**Phase:** Spec  
**Version:** draft-4  
**Date:** 2026-07-14  
**Product:** Mindline  
**Signed Shape:** `dbe4c6163374e3cb9540ea692b9bb19281f137b76544a07d5858a76f455b706b`  
**Foundation:** WP-45 commit `449008a`

## Authority and claim boundary

This Spec is governed by DEC-414, DEC-415, STD-21, STD-22, DEC-413, TEN-26, PRI-1, BR-1, STR-3, and DEC-64. CIR-2/CIR-3 and ROL-2 through ROL-7 provide scoped product, architecture, delivery, Chain, quality, and risk accountability. Product Brain global authority-domain cutover and global setup completion remain explicitly incomplete and are not implementation authority.

The first live result is private, sample-bound, operator-assisted founder discovery. It may prove activation mechanics, frozen-inventory accounting, exact sample delivery, readback, and zero-mutation replay. It cannot prove full-remainder quality, continuous operation, arbitrary-site completeness, held-out accuracy, improvement, generalization, production security, or no-human autonomy.

## Product contract

The blank-state flow is:

1. **Configure Connections**: select Slack Web API or External Slack Inventory, enter session-only source credentials when applicable, enter a session-only Product Brain key, verify provider-returned non-secret identities, and pin them.
2. **Import or connect source data**: for External Slack Inventory, use a browser file picker to upload a bounded versioned export, validate it, and inspect provenance/accounting errors before accepting it; for Slack Web API, verify the live scope.
3. **Configure Strategy**: enter context lenses and routing policy as plain text. Save an immutable version and fingerprint.
4. **Freeze Inventory**: exhaustively account for the selected source scope to an immutable upper watermark or import fingerprint.
5. **Prove**: select `min(3, canonical_count)` per observed `(retrieval_strategy_id, format_variant)` using deterministic ordering, seal selection before retrieval, and process every selected item without refill or reroll.
6. **Review**: inspect retrieval evidence/missingness, proposed meaning, relevance, role, disposition, manual-support reason, and destination mapping. The operator confirms or changes each proposed judgment; unreviewed items cannot enter an outbox.
7. **Approve exact batch**: inspect destination identity, ordered operations, payload/outbox/preflight/privacy fingerprints, counts/type distribution, maximum unique writes, maximum mutation attempts, and expiry. Approval binds exactly those values.
8. **Deliver and reconcile**: Product Brain creates drafts only, performs exact readback, seals receipts, and replays without mutation.
9. **Record founder review**: bind usefulness and reason to exact acknowledged draft/receipt IDs, then record session-credential, manual-support, and approval-burden judgments. A zero-draft proof records `trusted_value_observed=false` without rerun.
10. **Decide on drain**: show whether exhaustive processing of the frozen remainder is `READY`, `CONDITIONAL`, or `BLOCKED`. Confirmation authorizes processing/queueing only. Product Brain delivery remains exact-batch only.

`trusted_activation_completion` is true when the Configure through Review path yields a truthful reviewed verdict, including zero drafts. `trusted_value_observed` is separately true only when at least one exact acknowledged Product Brain draft is judged useful by Randy through the founder-review surface. `time_to_trusted_value` is nullable.

## First-slice source and evidence scope

Two source adapters implement one consumer-owned inventory port:

- `external_slack_inventory/v1` imports a bounded manifest through the browser. It contains separate `source_records`, `url_occurrences`, and `canonical_items`: every source-native Slack identity/timestamp appears exactly once; every URL occurrence points to one source record and one canonical item; every canonical item retains all occurrence IDs. It also contains scope/accounting metadata and optional imported retrieval evidence. Canonical-only legacy spike data may be inspected but cannot satisfy exhaustive inventory readiness until every declared occurrence is represented. Imported evidence is labelled replay evidence and never represented as a new live retrieval.
- `slack_web_api/v1` uses fixed-origin read-only Slack methods, cursor pagination, exact workspace/channel identity, lower/upper timestamp bounds, thread/reply policy, edit/delete policy, and attachment/private-file policy. Synthetic multi-page proof is mandatory; a live token is optional for this founder slice.

The live founder proof uses an occurrence-complete external inventory derived from the full Slack conversation so it can prove the complete denominator without requiring a persistent Slack token. The UI accepts the export via bounded multipart upload, validates schema/content fingerprint/count invariants before adoption, and shows file name, byte count, source identity, declared versus observed counts, omissions, duplicates, and safe error categories. No project file path or pre-positioned JSON is required. Slack Web API support must pass synthetic connector proof before the slice is closed.

## Strategy and bounded processor

`StrategySnapshot` contains:

- `strategy_id`, `version`, `fingerprint`;
- context-lens text;
- routing-policy text;
- normalized significant terms and explicit include/exclude terms;
- operator identity and creation time.

The first processor is `evidence_matcher/v0.1`. It is deliberately conservative and deterministic:

- retrieval evidence and source metadata are tokenized with a fixed stopword/version set;
- include/exclude matches and evidence refs are recorded;
- inaccessible, partial, private/authenticated, unsupported, and secret-like sources are manual/hold outcomes;
- a relevant accessible source may propose `external_entity`, `evidence_backed_finding`, or `unresolved_tension` only when required role attributes and public evidence are complete;
- otherwise it proposes `reference_resource`/hold for review, but the Product Brain v0.1 adapter cannot map that role and therefore creates no destination operation;
- proposed judgments are not destination authority. An operator review record is required before `routing.CompileGraph` and Product Brain outbox compilation.

This processor provides a usable deterministic baseline and an explicit future processor port. No hosted inference is used by default. A later hosted processor requires per-run opt-in, fixed provider/model, minimized/redacted projection, disclosed retention/data-use assumptions, and new eval evidence.

## Core domain types

```go
type SessionRef string
type RunID string

type SourceScope struct {
    ConnectorKind, WorkspaceID, ChannelID string
    LowerInclusive, UpperInclusive string
    IncludeThreads, IncludeReplies bool
    AttachmentPolicy, PrivateFilePolicy string
    EditDeletePolicy, AdapterVersion, Fingerprint string
}

type InventorySnapshot struct {
    SchemaVersion, Fingerprint, SourceIdentity, Watermark string
    OccurrenceCount, CanonicalCount int
    SourceRecords []SourceRecord
    URLOccurrences []URLOccurrence
    CanonicalItems []InventoryItem
    Strata []StratumCount
    Completeness []EvidenceCheck
}

type SourceRecord struct {
    SourceRecordID, NativeMessageID, NativeTimestamp, ContentFingerprint string
    URLOccurrenceIDs []string
    EditDeleteState, ThreadParentID string
}

type URLOccurrence struct {
    URLOccurrenceID, SourceRecordID, ObservedURL, CanonicalItemID string
}

type StrategySnapshot struct {
    SchemaVersion, StrategyID, Version, Fingerprint string
    ContextLenses, RoutingPolicy string
    SignificantTerms, IncludeTerms, ExcludeTerms []string
}

type RunPlan struct {
    SchemaVersion, Fingerprint string
    SourceScopeFingerprint, InventoryFingerprint, StrategyFingerprint string
    ComponentVersions map[string]string
    PrivacyPolicy, Mode, IdempotencyNamespace string
    Budgets RunBudgets
}

type BatchApproval struct {
    SchemaVersion, Fingerprint, BatchFingerprint string
    OutboxFingerprint, PreflightFingerprint, PrivacyFingerprint string
    DestinationWorkspaceID, DestinationKeyID string
    OrderedOperationFingerprints []string
    MaximumDestinationWrites, MaximumMutationAttempts int
    HumanInitiationEvidenceFingerprint, ApprovedAt, ExpiresAt string
}
```

All persisted types use strict versioned JSON, reject unknown fields and trailing data, and have bounded size. Every source record, occurrence, and canonical item ID is unique; every listed relation resolves both ways exactly once; declared and observed counts match; and no orphan or unreferenced occurrence is allowed. Credentials and usable credential references never appear in them.

## Application ports

The consumer-owned interfaces live in `internal/orchestration`:

```go
type SourceInventory interface {
    Probe(context.Context, SessionRef, SourceScope) (SourceCapability, error)
    Freeze(context.Context, SessionRef, SourceScope) (InventorySnapshot, error)
}

type Retriever interface {
    Retrieve(context.Context, RetrievalRequest) (RetrievalResult, error)
}

type Processor interface {
    Process(context.Context, ProcessingRequest) (ProcessingResult, error)
}

type DestinationDelivery interface {
    Preflight(context.Context, BatchCandidate) (PreflightReceipt, error)
    DeliverApproved(context.Context, ApprovedBatch) (DeliveryReceiptRef, error)
    Reconcile(context.Context, DeliveryIntentRef) (DeliveryReceiptRef, error)
    CancelApproved(context.Context, ApprovalRef) (CancellationReceipt, error)
}

type EventStore interface {
    Load(context.Context, RunID) ([]Event, error)
    Append(context.Context, RunID, ExpectedVersion, ...Event) error
}
```

`routing.CompileGraph` remains a deterministic package function. UI and CLI use one `ActivationService`; neither receives filesystem, raw credential, network, or destination mutation access.

## Package ownership

- `internal/orchestration`: pure aggregate, commands/queries, immutable plan, readiness composition, sampling, review, approval, and application service.
- `internal/runjournal`: owner-only append/fsync/hash-chain event store, CAS versioning, lease, and rebuildable projections.
- `internal/integrations`: process-local session connection registry, opaque credential leases, verified non-secret identity/capability snapshots, expiry/revoke.
- `internal/acquisition` and `internal/acquisition/slack`: source-neutral inventory plus external-import and Slack Web API implementations.
- `internal/retrieval`: registry, imported-evidence adapter, fixed network broker, access/completeness evidence.
- `internal/processing`: closed processor input/output and deterministic evidence matcher.
- `internal/processing/routingcompat`: the sole compatibility adapter from activation artifacts into the four validated v0.1 routing inputs; it calls `routing.CompileGraph` and cannot construct `routing.Result` directly.
- `internal/controlui`: hardened loopback HTTP adapter and embedded static assets only.
- existing `internal/routing`: source/destination-neutral semantic compiler.
- existing `internal/productbrain`: sole Product Brain mutation journal, approval validation, preflight, delivery, reconciliation, and readback.
- existing `internal/privateio`: containment/nofollow/atomic storage extended with bounded reads.

A separate strategy package is deferred until a second strategy implementation exists. Activation orchestration is not added to `runner.go`.

## Commands, queries, and UI API

`ActivationService` commands:

- `ConnectSource`, `ConnectDestination`, `Disconnect`, `Reconnect`;
- `UploadExternalInventory`, `ValidateExternalInventory`, `AcceptExternalInventory`;
- `SaveStrategy`, `FreezeInventory`, `StartProof`;
- `RecordItemReview`, `RecordManualSupportOutcome`;
- `PreviewBatch`, `ApproveExactBatch`, `DeliverApprovedBatch`, `ReconcileBatch`;
- `ConfirmExperimentalDrain`, `StartDrainProcessing`, `Pause`, `Resume`, `Cancel`, `RetryItem`.
- `RecordFounderReview` bound to exact delivery receipt and acknowledged draft identities, or to an explicit zero-draft proof result.

`ApproveExactBatch` is available only through the authenticated browser review ceremony. The server first renders and journals the exact batch preview, then issues a short-lived one-time random review nonce bound to the session, batch fingerprint, destination identity, budgets, and preview evidence. Approval accepts that nonce and a human gesture only; it accepts no client-supplied actor. The server derives and seals human-initiation evidence. CLI, processors, agents, replay, and generic service callers have no approval command/capability.

Queries return connection readiness, upload/import validation and provenance, immutable source scope, strategy, inventory accounting, proof selection, item review, queue, batch, delivery receipt/history, founder-review/value state, and staged-readiness projections.

HTTP routes use strict JSON command envelopes except `POST /api/import/external-slack`, the single authenticated multipart exception. That route requires the same memory-held session and CSRF headers plus exact Origin/Host/loopback peer, accepts exactly one named file part and no form values, caps headers/parts/file/total bytes, streams into a fresh owner-only quarantine file, strictly decodes the versioned manifest with unknown/trailing-data rejection, validates fingerprints/count/referential invariants, and atomically adopts only a valid import. Any error deletes quarantine state and returns a fixed safe category/correlation ID without reflecting file names or content. `GET /`, static assets, and authenticated `GET /api/state` are read-only. Other state changes are POST routes under `/api/commands/*`. Errors expose only fixed categories and correlation IDs, never input reflection.

The four UI sections are Connections, Strategy, Prove, and Drain. Readiness labels include the exact authorization sentence:

- `READY_TO_INVENTORY`: inventory may start; processing and destination writes are unauthorized.
- `READY_TO_PROCESS`: the frozen capped proof may start; destination writes are unauthorized.
- `READY_TO_EXPERIMENTAL_DRAIN`: exhaustive processing to this frozen watermark may start after confirmation; Product Brain delivery is unauthorized.
- `READY_TO_DELIVER`: only the displayed batch fingerprint, destination, and budgets are authorized.
- `CONDITIONAL`: unauthorized until every named condition passes.

## Run and batch state machines

Run states:

```text
configured -> inventorying -> inventory_frozen
inventory_frozen -> proof_selected -> proof_processing -> proof_complete
proof_complete -> drain_confirmed -> drain_processing -> queue_sealed
```

Batch states:

```text
proposed -> preflighted -> approved -> delivery_requested -> receipt_linked
```

Pause/cancel never delete history. Configuration or component drift requires a new run. After process restart, any run with a credential dependency becomes `credential_required`; reconnect must prove the same pinned identity.

The activation journal is authoritative for configuration, inventory, processing, review, queue, approval, and delivery intent. Product Brain delivery history is the sole destination-mutation authority. If a crash occurs after `delivery_requested`, Product Brain reconciliation runs before retry and the activation journal links only a validated sealed receipt.

## Product Brain compatibility and approval enforcement

The v0.1 delivery profile and legacy CLI remain unchanged. Activation builds a compatibility projection from its verified non-secret destination identity and injects a session lease through `SecretProvider`. `AKITransport` must resolve the provider for every call rather than retain a copied secret, reject expired/revoked leases, and use a fixed no-proxy transport. This isolates runtime credential behavior without weakening legacy profile validation and allows a future REST transport behind `ProductBrainTransport`.

Add `productbrain.DeliveryApproval/v0.1` plus approved-delivery `Run/State/History` schemas at v0.2 and `DeliverApproved`. The v0.2 run binds the immutable approval fingerprint, human-initiation evidence fingerprint, cancellation state, unique-write reservations, cumulative attempt reservations, and every operation attempt. Dual readers expose legacy v0.1 history as read-only and v0.2 approved history as resumable; writers never mix them. Legacy `Deliver` writes only isolated v0.1 state and is unreachable from activation. The Product Brain package, not orchestration alone, validates:

- approval/batch/outbox/preflight/privacy fingerprints;
- exact ordered operation fingerprints;
- workspace/key identity and draft-only profile;
- expiry and server-derived one-time human-initiation evidence;
- `maximum_destination_writes` against unique operations;
- `maximum_mutation_attempts` against journaled attempts;
- configuration/preflight drift and prior ambiguous outcomes.

The approval snapshot and fingerprint are sealed into Product Brain v0.2 delivery state before any mutation. A unique-write slot is reserved before first send. Before every network mutation, Product Brain atomically validates approval/expiry/cancel, reserves one cumulative attempt, writes and fsyncs the reservation, and only then sends. The attempt remains consumed after crash, timeout, or missing response. An ambiguous result retains its unique-write slot. Retry requires exact reconciliation plus remaining attempt budget.

Cancellation is a durable v0.2 Product Brain authority record bound to the batch and approval fingerprints. `DestinationDelivery.CancelApproved` seals it idempotently in the local Product Brain delivery authority without requiring a destination mutation credential. `ActivationService.Cancel` must obtain this cancellation receipt before it reports an approved/delivering batch cancelled or appends its activation cancellation event. Product Brain checks cancellation atomically before each attempt reservation. If an attempt reservation is durably first, that attempt may complete and must reconcile; if cancellation is durably first, no new send may occur. Cancellation never erases an ambiguous attempt or receipt. Existing `Deliver` remains legacy-only; activation cannot call it.

## Routing compatibility adapter

`internal/processing/routingcompat.CompileReviewed` owns the exact v0.1 transformation:

1. `InventorySnapshot.SourceRecords`, `URLOccurrences`, and `CanonicalItems` map one-for-one into `routing.SourceGraph`; occurrence and source IDs/fingerprints are preserved, graph edges are deterministic, and imported/live retrieval states become canonical missingness.
2. retrieval results map into `routing.LinkArtifacts`; public metadata/excerpts and related-link discovery evidence are copied only from validated retrieval artifacts.
3. `StrategySnapshot` maps into `routing.LensProfile`; strategy/lens IDs and versions are preserved and include/exclude text is normalized by the pinned strategy tokenizer.
4. immutable operator-review records map into `routing.Judgments`; each selected canonical item has exactly one reviewed judgment, evidence refs must resolve, and unreviewed proposals are rejected.
5. the adapter calls `routing.CompileGraph` and returns its validated `routing.Result`. It cannot bypass validation or introduce destination fields.

Product Brain outbox compilation consumes only this returned v0.1 result. Compatibility golden tests compare the transformation to WP-45 fixtures.

## Credential contract

- Password fields are bounded; submitted values are removed from the DOM immediately and never returned.
- `SessionRegistry` stores secrets only in memory behind 256-bit random opaque handles, with idle and absolute TTL, revoke, disconnect, and shutdown invalidation.
- The lease is rechecked for each external call and cancelling/revoking it cancels in-flight contexts. The claim is logical inaccessibility, not Go heap zeroization.
- Persisted connection data includes only provider-returned workspace/channel/key identity, capability version, and a non-authorizing connection ID.
- First connection is explicit trust-on-first-use. Resume/reconnect requires exact pinned identity before inventory or mutation.
- No secret enters files, env files, args, URLs, browser storage, logs, traces, journals, artifacts, telemetry, git, or error envelopes.

## Slack Web API contract

- Fixed `https://slack.com/api/` origin; no user URL, cookies, ambient proxy, or redirects.
- Versioned read-only method/scope allowlist; workspace identity, selected channel, membership/access, and source bounds are verified.
- Cursor loop detection, deterministic dedupe, edit/delete/thread/file accounting, page/item/byte/time/cost/retry budgets, cancellation, bounded `Retry-After`, and revoked-token terminal state.
- Authorization is never forwarded to private files or external URLs. Private/authenticated files are manual in this slice.
- Checkpoints contain non-secret cursor fingerprints and counts; raw API responses are not persisted.

## External retrieval and processor isolation

One fetch broker permits HTTP/HTTPS ports 80/443 only. It rejects userinfo, secret-bearing query material, localhost and special ranges, IP literals, mixed public/private DNS answers, and IPv4-mapped bypasses. It resolves and pins a validated public address in `DialContext`, verifies the connected peer, validates every redirect, blocks HTTPS-to-HTTP downgrade, disables environment proxying, and sends no auth/cookie/referer. It enforces connect/TLS/header/body/decompressed-byte/content-type/redirect/time limits and cancellation. Active content is never executed or rendered.

Processors receive only inert bounded artifacts. They have no credentials, fetch capability, shell, arbitrary filesystem, browser session, Chain tool, strategy mutation, approval mutation, or destination authority. Any subprocess added later must use fixed binary/args, no shell, scrubbed environment, private cwd, bounded IO/time, process-group cancellation, and no inherited descriptors.

## Loopback control surface

- Bind exactly one unpredictable `127.0.0.1` or `::1` listener, never wildcard.
- Require exact listener Host and loopback `RemoteAddr` on every request; CORS is disabled.
- Bootstrap with a one-time random URL fragment exchanged in a custom header and immediately removed via `history.replaceState`. Return a bounded session capability and independent CSRF token to the application, keep both only in JS memory, and send them only in custom headers. No cookie is used, so the browser cannot automatically disclose a capability to another loopback port.
- Private reads and all writes require the memory-held session capability. Every state change requires non-GET, exact Origin, CSRF/session constant-time checks, JSON media type, strict schema/unknown-field rejection, trailing-data rejection, and body cap.
- Use a dedicated mux and `http.Server` read-header/read/write/idle timeouts plus header limit. No DefaultServeMux or debug endpoints.
- CSP is `default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'`; also no-store, nosniff, no-referrer, and restrictive Permissions-Policy.
- No third-party assets, inline/eval script, raw HTML, `innerHTML`, or active source rendering.

## Journal and queue contract

Use a freshly allocated exclusive 0700 root and regular 0600 files. Reads are size-capped and reject symlinks, widened modes, unknown files, unexpected owners, invalid schemas, gaps, duplicates, reordered events, bad prior/event hashes, and illegal state transitions. Append uses compare-and-append plus fsync. The journal is authority; queue/read models are rebuildable projections. One writer/CAS lease, bounded attempts/backoff, poison isolation, pause/cancel, and deterministic recovery are required.

The hash chain proves internal consistency, not authenticity against a malicious same-user rewrite. Secret-like ingress is quarantined/redacted before persistence while source identity and accounting remain visible.

## Staged readiness contributors

Every readiness verdict records stage, contributor/version, required checks, evidence fingerprint, verdict, and next action. Missing checks fail closed; `N/A` is allowed only by the selected adapter contract.

- Inventory contributor: connection identity, immutable scope/import manifest, acquisition budgets, cancellation, private root.
- Process contributor: exhaustive reconciled inventory, frozen fingerprint, selection/accounting, processor versions, manual capacity.
- Experimental-drain contributor: proof accounting, observed-strata outcomes, privacy/security, queue recovery, projected resource/manual burden, explicit confirmation.
- Product Brain delivery contributor: exact identity, mapping/schema/capacity, live preflight, privacy, approval and budgets.

No global `READY` is rendered.

## Eval projection and proof data

Intended user: Randy as founder/operator. Source: one private Slack link inventory plus synthetic Slack API fixtures. Destination: one disposable Product Brain workspace, drafts only. Workspace/provider assumptions are pinned in run artifacts. Privacy boundary is local except explicit fixed Product Brain draft payloads. Sample is deterministic and not held out. No baseline or generalization claim exists.

Proof outputs:

- exhaustive inventory accounting and observed-strata table;
- sealed sample manifest and selection algorithm/version;
- item retrieval/processing/review packets;
- run journal/readiness report and queue projection;
- exact batch approval, outbox, preflight, Product Brain history/readback/replay;
- sentinel privacy/security report;
- founder-discovery record with errors, retries, backtracks, help, elapsed time, usefulness, and burden judgments;
- `mindline eval readback` plus claim-specific proof gates that reject safety/generalization/improvement/DEC-64 claims.

## Adversarial acceptance matrix

1. Credential sentinel across success, error, 429, revoke, disconnect, crash, and restart leaves zero copies outside the in-memory provider; revoke prevents further calls.
2. Wrong Product Brain workspace/key/schema/governance or Slack workspace/channel/scope fails before inventory/mutation with safe category only.
3. Slack cursor cycle, duplicate page, omitted thread, 429 storm, revoke mid-page, edit/delete, and private file produce bounded explicit accounting and never false readiness.
4. Disallowed schemes, userinfo, private/special IPv4/IPv6, mixed DNS, rebinding, public-to-private redirect, HTTPS downgrade, env proxy, slow headers, oversized body, and decompression bomb make no prohibited connection and end bounded/manual.
5. Host rebinding, non-loopback peer, missing/wrong Origin, guessed session/CSRF, form/simple request, oversized/trailing JSON, slowloris, CORS read, and GET mutation produce 4xx/timeout and zero event.
6. Source HTML/script/prompt injection renders as text and cannot change plan, strategy, approval, destination, or invoke tools.
7. Journal symlink/mode widening, event gap/reorder/tamper, unknown file, stale lease, concurrent writer, and poison item block or rebuild deterministically with no side effect.
8. Approval after item/config/destination/privacy/preflight change, over-budget send, expiry, or replay on another batch is rejected inside Product Brain delivery.
9. Crash at intent, durably reserved attempt, socket write before response, Product Brain journal, response, readback, sealed receipt, and activation receipt-link reconciles first with no duplicate mutation and visible ambiguity; every reserved attempt remains consumed.
10. A private/sample run submitted to safety, generalization, improvement, or DEC-64 proof is blocked.
11. A hostile listener on another loopback port receives no session/CSRF capability. JSON actor injection, automatic CLI/processor/agent approval, review-nonce replay, and nonce use for another batch/session all fail without a delivery event.
12. Cancellation before attempt reservation forbids the send; cancellation after reservation permits only that reserved attempt to reconcile; ambiguous cancellation and restart never create a new send without exact reconciliation and remaining budget.

## DevSecOps gates

The pre-merge and live-proof gate pins and runs:

- `go test -count=1 ./...`;
- targeted `go test -race` for orchestration, runjournal, integrations, acquisition/slack, retrieval, processing, controlui, productbrain;
- `go vet ./...`;
- pinned `govulncheck ./...` with no reachable known vulnerability;
- pinned `gosec ./...` with no unresolved high/critical finding;
- pinned `gitleaks detect --no-git` plus repository-history scan with no verified secret;
- sentinel scan of responses/errors/logs/files/journals/queue/artifacts/telemetry/browser storage/git;
- `git diff --check`;
- hardened browser smoke and crash/replay proof.

Tool unavailability, incomplete coverage, or a verified high/critical/secret finding blocks the gate. Findings store category/path only, never matching secret content.

## Compatibility, migration, and rollback

- Preserve v0.1 routing, outbox, preflight, delivery, history, readback, proof artifacts, and legacy CLI behavior.
- Add separately versioned activation plan/event/projection/approval schemas and Product Brain approved-delivery v0.2 run/state/history schemas with dual read-only v0.1 readers.
- Transform inventory, retrieval, strategy, and reviewed judgments into the four strict v0.1 routing inputs through `internal/processing/routingcompat`, call `routing.CompileGraph`, then reuse Product Brain outbox/preflight mapping.
- Do not add a second Product Brain writer or infer mutation from HTTP response.
- Legacy activation artifacts are read-only and never silently resumed.
- Rollback disables the activation entry point and preserves private journals/artifacts for readback; it does not reverse destination mutations.

## Acceptance

The slice passes only when all Shape acceptance items are traceable to tests or live evidence, the adversarial matrix and DevSecOps gates pass, the exact sample receives a truthful activation verdict, and any delivered sample batch is draft-only, exact-approved, acknowledged/read back, and zero-mutation on replay. The complete frozen remainder may be represented in a durable queue and processed only after the staged drain confirmation; it is not automatically delivered or used for broad claims.

## Exclusions

OAuth/keychain/persistent credentials; hosted multi-user/RBAC/TLS/database/scheduler/HA; arbitrary authenticated-browser extraction; integration marketplace; routing DSL/schema builder; automatic full-remainder delivery; non-draft writes; blanket approvals; production/generalization/autonomy claims; and OS container/seccomp guarantees are excluded.
