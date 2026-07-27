# Shape: Trusted Slack-to-Product-Brain Activation and Gated Drain

**Status:** review candidate  
**Phase:** Shape  
**Version:** draft-7  
**Date:** 2026-07-14  
**Product:** Mindline  
**Current foundation:** WP-45  

## Outcome

From a blank state, Randy can configure a session-scoped Slack-to-Product-Brain connection and a versioned strategy, complete a capped proof, inspect a truthful review verdict and, when an exact candidate batch exists and is approved, inspect acknowledged Product Brain draft value without editing JSON or using shell commands.

The slice measures:

- `trusted_activation_completion`: the complete Configure -> Prove -> Review path reaches an evidence-backed verdict;
- `trusted_value_observed`: at least one exact approved draft is acknowledged and Randy judges it useful; this may truthfully remain false when activation succeeds;
- `time_to_trusted_verdict`: elapsed time from blank configuration to the reviewed activation verdict;
- `time_to_trusted_value`: nullable elapsed time from blank configuration to acknowledged useful Product Brain draft value;
- `inventory_accounting`: every source occurrence inside the frozen inventory denominator is reconciled;
- `sample_accounting`: every selected sample is retrieved, classified, queued for help, withheld, or delivered with an explicit reason;
- counter-metrics of zero credential leakage, private-provenance leakage, unexplained loss, duplicate or unacknowledged destination writes, replay mutations, and readiness-gate false positives across adversarial fixtures.

This is a private, sample-bound, operator-assisted proof. It does not establish held-out accuracy, generalization, production readiness, or no-human autonomy.

## User Job

Safely connect a private source to a chosen destination, state what matters and how items should be handled, prove the configuration on a bounded, coverage-declared sample, inspect the resulting value and uncertainty, and decide whether exhaustive processing should begin without manufacturing JSON artifacts or shell orchestration.

The live remainder is not executed in this slice, so real-world drain correctness is not proven.

## Appetite

Large, delivered as one vertical slice with explicit internal seams. The slice may introduce the reusable control-plane, integration, orchestration, queue, and security foundations required for the user job, but it must not become an integration marketplace, hosted multi-user platform, arbitrary policy language, or autonomous destination writer.

## Product Challenge and Reconciliation

The first idea was to let a passing three-per-type sample unlock a full Slack-to-Product-Brain drain. Product, architecture, and risk review rejected that because a small private sample proves mechanics but cannot authorize unseen destination writes.

The reconciled product behavior is:

1. One application service supports both `proof` and `experimental_drain` run policies.
2. Proof retrieves/processes at most three items per declared sampling stratum. The deterministic sample is sealed before retrieval and may not be rerolled, refilled, or replaced because an item fails, is inaccessible, or produces no destination draft.
3. Inventory is not sampled. It is exhaustive to one frozen source watermark so the denominator is known before proof selection.
4. Experimental drain means exhaustive acquisition and processing to that watermark with bounded resources, durable checkpoints, and a complete review/outbox queue. It is disabled by default.
5. A passing sample never grants blanket destination authority. Every Product Brain delivery is an exact immutable batch with fingerprint-bound human approval, a maximum write budget, destination identity, privacy result, and preflight result.
6. Automatic delivery of the unseen remainder remains blocked until the Chain contains the held-out quality and safety authority required by DEC-64.

This preserves the user's desired full-drain capability while keeping acquisition, processing, human judgment, and destination authority distinct.

## Candidate Product Surface

The minimal local control surface has four user concepts:

### Connections

Connections contains two explicit lanes:

- **Sources**: Slack is the first source. The active slice supports a Slack Web API connector and an external-export adapter that share the same source contract.
- **Destinations**: Product Brain is the first destination. The UI accepts a credential only into an ephemeral runtime provider, verifies the fixed Product Brain origin and returned non-secret workspace/key identity, and never persists or reflects the credential.

The current credential experience is intentionally session-scoped. Restarting Mindline requires re-entry. Persisted connection profiles contain only non-secret identity and an opaque credential reference. OS keychain or managed-secret persistence is a future provider behind the same port.

### Strategy

Strategy is separate from Connections. It contains versioned, fingerprinted text fields for:

- context lenses: what matters and why;
- routing policy: how relevant, uncertain, inaccessible, duplicate, and irrelevant items should be handled;

Manual-review tolerance belongs to the pinned proof/drain run policy. A destination write budget belongs only to an exact immutable batch approval. Neither is Strategy authority.

Strategy text is configuration, not Chain authority. Retrieved content cannot change it.

### Prove

Prove inventories the selected source to a frozen watermark, identifies every observed stratum, selects up to three canonical items per stratum deterministically, processes the sample, and renders:

- retrieval completeness and missingness;
- stable meaning and evidence;
- lens relevance;
- semantic role;
- disposition;
- human-support reason;
- destination mapping and immutable draft-batch preview;
- Product Brain preflight, acknowledgement, and replay result when an exact batch is approved.

### Drain

Drain is an experimental, operator-supervised mode disabled by default. It pages and checkpoints the full frozen inventory through the same application service used by Prove. It can populate the complete processed/review/outbox queue. It cannot use the proof approval as authority for the remainder, and it cannot mutate Product Brain except through separately approved, immutable, budgeted batches.

The surface exposes progress, last safe cursor, counts by state, pause/cancel, retry budget, human queue burden, and the exact reason any readiness stage is blocked or conditional.

The stream-aligned Product Team owns the trusted-activation outcome and may change these candidate components when discovery invalidates them.

## Sampling and Classification Model

The model keeps independent dimensions:

- `source_connector`
- `provider_family`
- `content_kind`
- `format_variant`
- `retrieval_strategy_id`
- `access_state`
- `completeness_status`
- `semantic_role`
- `lens_result`
- `disposition`
- `destination_mapping`
- `delivery_state`

The proof cap applies to `(retrieval_strategy_id, format_variant)`, not to provider host, semantic role, Product Brain collection, or access outcome.

For every observed stratum, selection is `min(3, canonical_count)` under a versioned deterministic ordering. A duplicate consumes one canonical work slot while all source occurrences remain accounted for. Failed, inaccessible, manual, unsupported, irrelevant, and zero-draft results are retained in the sealed sample and are never replaced with more favorable items.

The initial retrieval registry must recognize or support at least:

- LinkedIn post, article, and short/outbound redirect;
- YouTube video, short, and channel metadata;
- Spotify or comparable metadata-only media surface;
- GitHub repository, file, gist, or documentation;
- article, Substack/newsletter, and generic public web;
- PDF or public document export;
- generic or unknown public URL;
- authenticated/private provider surface as recognized-but-unsupported.

`authenticated/private`, policy-skipped, blocked, partial, metadata-only, and link-rot are access/completeness outcomes. In this slice authenticated/private sources deterministically enter `needs_manual_processing`; no browser cookies or ambient authenticated session may be reused.

Proof reports every stratum actually observed in the frozen source scope. An unobserved registry capability is reported as absent coverage, never fabricated proof; an observed but unavailable capability is reported as unsupported or manual.

## Staged Readiness

Readiness is not one boolean.

Orchestration owns generic readiness stages, verdicts, and evidence requirements. Each selected source and destination adapter supplies a versioned readiness contributor with an exact required-check set and evidence fingerprint. Missing required checks fail closed. A check may be `N/A` only when the selected adapter contract explicitly declares why it is inapplicable.

Before inventory, the run pins an exact source-scope fingerprint containing workspace, channel, lower and upper timestamp bounds, thread/reply inclusion, attachment/private-file policy, and edit/delete policy. The upper `latest` watermark alone is not a complete denominator. Proof and experimental-drain runs bind to the same immutable source scope.

### READY_TO_INVENTORY

Requires the selected source contributor to prove identity, scope/integrity, acquisition mechanics, budgets, and cancellation. For the Slack Web API adapter this includes:

- source token/profile bound to the expected workspace and selected channel;
- least-privilege read scopes;
- successful read-only connector probe;
- synthetic pagination, cursor restart, dedupe, and rate-limit mechanics;
- declared page, item, byte, wall-time, retry, and cost budgets;
- owner-only private runtime root and an active cancel path.

For the external-export adapter the pre-inventory contributor binds bounded input selection, import manifest/schema version, file/content fingerprints, source identity, declared source scope, file integrity, owner-only path/permissions, and restart capability. A declared record count is only an expected denominator. Exhaustive record accounting is proved and reconciled during inventory for `READY_TO_PROCESS`. The adapter does not fabricate API token/scope/rate evidence.

### READY_TO_PROCESS

Requires a completed inventory to the captured `latest` watermark with:

- every page and source-native identity journaled;
- zero pagination loops or unexplained gaps;
- a frozen metadata inventory fingerprint;
- every occurrence reconciled as selected or not selected;
- every discovered stratum represented in readiness counts, even if unsampled;
- configured processor versions and manual-support capacity.

### READY_TO_EXPERIMENTAL_DRAIN

Orchestration proves:

- the capped proof to be complete;
- 100% sample accounting;
- every observed stratum classified as supported, manual, or blocked;
- zero critical privacy or security findings;
- passing queue reconstruction and recovery proof;
- projected rate, wall-time, cost, and retry demand inside the pinned run-policy budgets;
- projected manual-support burden inside the operator's pinned tolerance;
- explicit human confirmation bound to the run plan.

When the run intends to build destination-ready proposals, the selected destination contributor supplies only versioned mapping/schema/capacity capability evidence at this stage. Product Brain contributes collection mapping/schema/capacity evidence. No outbox approval or mutation authority is required or implied.

This gate authorizes exhaustive processing to the frozen watermark only. It never authorizes Product Brain delivery.

### READY_TO_DELIVER

Orchestration requires generic pinned-result, privacy, recovery, immutable-batch, and approval evidence. The selected destination contributor supplies its versioned identity, mapping/schema/capacity, and exact preflight evidence. For Product Brain this requires:

- pinned processed results and strategy/configuration versions;
- zero privacy findings in the exact outbound projection;
- exact destination workspace/key identity and collection-contract preflight;
- healthy journal reconstruction and queue recovery;
- an immutable outbox fingerprint and explicit maximum write budget;
- human approval bound to that exact outbox fingerprint, destination identity, counts, type distribution, and gate results.

Any configuration, content, outbox, destination, identity, or fingerprint change invalidates approval. Unknown or unaccounted state is `CONDITIONAL` or `BLOCKED`, never `READY`.

## Accounting Invariant

The frozen metadata inventory is the denominator. Every discovered occurrence must reconcile through:

```text
inventoried
  -> selected | not_selected
selected
  -> retrieved | manual | unsupported
  -> disposition
  -> outbox | withheld
  -> acknowledged | not_attempted
```

Duplicate Slack occurrences remain traceable to their source records while canonical work is deduplicated. No occurrence disappears when a canonical source is selected, withheld, queued, or delivered.

## Architecture

The implementation adds application and infrastructure seams without rewriting WP-45 artifacts or the legacy CLI.

### Modules

- `internal/integrations`: non-secret connection definitions, source/destination instances, credential references, source/destination ports, and connection verifiers.
- `internal/integrations` also defines versioned source/destination readiness-contributor ports. Adapter packages own provider-specific checks; orchestration owns only generic stage policy and consumes contributor evidence.
- `internal/acquisition/slack`: fixed-origin Slack Web API client plus external-export adapter; both produce a source-neutral inventory contract.
- `internal/retrieval`: retrieval-strategy registry, typed fetch/result contracts, access/completeness states, provider implementations, and the SSRF-safe network transport. Semantic processors consume retrieval artifacts and receive no network credential or fetch authority.
- `internal/orchestration`: proof/drain run aggregate, immutable versioned/fingerprinted strategy snapshots, staged readiness, deterministic sampling, state transitions, batch approval, and application services. A separate strategy package is deferred until a second strategy implementation requires it.
- `internal/runjournal`: append-only hash-bound run events plus rebuildable materialized queue projections, leases, retry budgets, idempotency, and recovery.
- `internal/processing`: provider-neutral worker request/result contracts and evidence/missingness validation.
- `internal/controlui`: hardened loopback HTTP surface that calls application services through typed commands and read models.
- existing `internal/routing`: source- and destination-neutral meaning/relevance/disposition compilation.
- existing `internal/productbrain`: destination mapping, fixed-origin transport, preflight, draft delivery, exact readback, immutable delivery history, and replay.
- existing eval packages: artifact readback, proof, and claim boundaries.

CLI and web are delivery surfaces over the same application services. The UI must not shell out and must not add orchestration to the existing monolithic `runner.go`.

### Run Authority

The append-only journal is authoritative; the visible queue is a rebuildable projection. Events and sealed run plans are fingerprint-bound. State transitions are monotonic and validated. Work uses leases/CAS or an equivalent single-writer rule, stable idempotency keys, poison-item isolation, bounded attempts/backoff, and ambiguous-mutation reconciliation before retry.

The orchestration run journal is authoritative only for acquisition, retrieval, processing, review, approval, and queue state. Product Brain's existing sealed delivery-run/history journal remains the sole mutation authority. The orchestration journal records only immutable delivery intent plus referenced Product Brain outbox, preflight, run, receipt, and readback fingerprints.

Destination handoff follows a crash-safe protocol:

1. seal immutable batch intent and approval in the orchestration journal;
2. invoke Product Brain delivery with the exact outbox/preflight fingerprints;
3. let Product Brain journal each mutation attempt and reconcile ambiguous outcomes;
4. validate exact Product Brain readback and sealed receipt;
5. append the receipt/reference to orchestration only after validation;
6. on restart, reconcile Product Brain authority before any retry.

No orchestration state may infer a destination mutation from an HTTP result alone.

Every run pins:

- non-secret source and destination connection versions;
- strategy version and fingerprint;
- source adapter, processor, routing, and destination adapter versions;
- retrieval-registry schema/version, every selected retrieval-strategy implementation/provider version, canonicalization version, and SSRF/network-policy/fetcher version;
- selected source/destination readiness-contributor versions and evidence fingerprints;
- run policy plus acquisition/retrieval/processing resource budgets and manual-burden tolerance;
- frozen source watermark;
- eval projection;
- idempotency namespace.

Configuration drift rejects resume or forks a new run. It never silently changes an in-flight run.

Retriever, canonicalizer, network-policy, or readiness-contributor drift invalidates derived readiness, sample, and outbox artifacts. It can never advance the old run. The idempotency namespace includes those pinned versions.

`maximum_destination_writes` exists only inside the fingerprint-bound immutable Product Brain batch-approval record alongside destination identity, outbox/preflight fingerprints, operation counts, type distribution, and privacy results. It limits unique destination operations. A separate `maximum_mutation_attempts` limits sends and retries. A unique-write slot is reserved before first send; an ambiguous result retains that slot, and any retry requires exact reconciliation plus remaining attempt budget. A generic run plan or experimental-drain confirmation cannot independently authorize destination mutation; it may only reference a validated batch-approval fingerprint.

### Migration and Rollback

- Preserve all v0.1 routing, outbox, preflight, delivery, readback, and proof artifacts and their sealed histories.
- Add compatibility readers into the control-plane read model; do not dual-write legacy and new authority formats.
- Route new CLI and UI entry points through shared application services incrementally. Existing commands keep their behavior until deliberately migrated and covered by compatibility tests.
- Legacy runs may appear in the UI only as read-only imported history with explicit legacy capability limits; they cannot be resumed as new orchestration runs.
- Retain the current CLI path as rollback while the new control plane is experimental.
- Regression tests must prove unchanged legacy commands, exact artifact decoding, and WP-45 delivery/readback/replay.

### Processor Authority

Source content is untrusted data. A processing worker has no access to Slack or Product Brain credentials, destination mutation, shell execution, Product Brain Chain tools, approval state, strategy mutation, connection mutation, or ambient authenticated browser sessions.

Inputs and outputs are bounded typed schemas. Outputs require source provenance, evidence references, missingness, allowlisted enums, and privacy/policy validation before the queue may advance. Prompt-injection fixtures must prove that source text cannot change configuration, strategy, approval, run policy, destination, Chain state, or execute tools.

## Security and Privacy Contract

- Product Brain and Slack API origins are fixed by adapter kind, never user-entered.
- Credentials enter only bounded password fields, are immediately removed from the DOM, remain behind random opaque in-memory leases, and become logically inaccessible on expiry, disconnect, revoke, or shutdown. The product does not claim Go heap zeroization.
- Credentials never enter files, environment files, query strings, CLI arguments, logs, telemetry, HTML/JSON responses, browser storage, artifacts, or git.
- Sentinel-secret tests scan success, failure, disconnect, and restart surfaces.
- First connection is explicit trust-on-first-use of provider-returned non-secret identity. Resume requires re-entry and exact match to the pinned Slack workspace/channel and Product Brain workspace/key identity before inventory or mutation.
- Product Brain validates its fixed HTTPS origin before requesting its credential, uses a revocable ephemeral secret provider, disables ambient proxying, and remains draft-only.
- Slack uses an exact read-only scope/method set, fixed HTTPS origin/path, no ambient proxy/cookies, binds returned workspace/channel identity and membership, honors bounded `Retry-After`, detects cursor loops, limits pages/items/bytes/retries/time/cost, and never forwards Slack authorization to files or external URLs.
- External retrieval passes through one central broker. It permits HTTP(S) on ports 80/443 only; rejects userinfo, secret-bearing URLs, local/special IPs and mixed public/private answers; normalizes IPv4-mapped IPv6; resolves and pins a validated public peer; revalidates every redirect; blocks HTTPS-to-HTTP downgrade; disables ambient proxy/cookies/auth; and enforces redirect, header/body/decompression/time/content-type limits. Processors receive inert artifacts, never network capability.
- Hosted inference is disabled by default for private Slack content. A later run may use a fixed provider/model only through explicit per-run opt-in with a minimized outbound projection, disclosed retention/data-use assumptions, and no ambient credential. Hosted telemetry remains metadata-only under BR-1.
- The loopback server binds one explicit loopback listener, requires exact Host and loopback peer on every route, disables CORS, and uses an unguessable per-process session capability plus an independent CSRF token. Every state change requires a non-GET method, exact Origin, strict JSON schema/content type/body/trailing-data validation, and constant-time session/CSRF checks.
- GET/static routes are side-effect-free and use security/no-store headers. The server sets explicit read-header/read/write/idle timeouts and header limits.
- Server-rendered templates escape all values; source content is text only; no third-party assets, inline/eval scripts, `innerHTML`, localStorage/sessionStorage, dynamic templates, or trusted raw HTML.
- The runtime root, journals, projections, and artifacts are owner-only, size-bounded, reject symlinks/path traversal/permission widening/unknown files, and fail closed on corruption or tamper. A hash chain proves consistency, not authenticity against a malicious same-user rewrite.
- The Product Brain mutation entry point validates the immutable approval, unique-write budget, attempt budget, exact outbox/preflight/privacy fingerprints, destination identity, expiry, and readback/reconciliation state inside the destination authority. An orchestration-only approval check is insufficient.

## Team Topologies and Product Operating Model

- The stream-aligned Product Team owns trusted activation, the control UI, application flow, and time-to-trusted-value.
- Platform capability owns the runtime, append-only journal/materialized queue, secret boundary, and local control host.
- Retrieval and evaluation are complicated-subsystem capabilities with evidence-backed versioned contracts.
- Source and destination adapter capabilities expose versioned contracts X-as-a-Service.
- Collaboration is limited to first integration or deliberate contract evolution; normal operation consumes the published contract.
- Architecture, Delivery Quality, and Risk/Safety reviewers gate material phase transitions.
- The empowered product trio may change UI flow, secret lifecycle, sampling policy, strategy representation, and connector boundary when discovery evidence invalidates the current candidate solution.

## Chain-Bounded Autonomy

The user authorizes the delivery team to execute signed work end to end, including Product Brain `promote` and `verify` operations for product-development Chain entries when evidence and configured authority allow them.

That autonomy does not authorize:

- bypassing Chain or reviewer gates;
- fabricating proof or human approval;
- making Product Brain destination writes outside an exact approved batch;
- broadening source, destination, privacy, or secret scope without a signed change;
- claiming held-out quality, generalization, production readiness, or no-human autonomy without the required evidence;
- using product runtime code to call development-governance `pb promote` or `pb verify`.

This rule must be captured as Chain authority before it is projected into repository instruction surfaces.

### Chain Authority Prerequisites

Shape consensus does not itself confer Spec or Delivery Authority.

Before the Spec may become implementation authority:

1. Scoped accountable ownership must be explicit in Chain for Product, Architecture, Engineering/Delivery, Chain Steward, Delivery Quality, and Risk/Safety. The current Product Brain authority-domain cutover remains `activeSource: legacy`; it is neither repaired nor used as delivery authority for this slice. CIR-2/CIR-3, ROL-2 through ROL-7, DEC-414, STD-22, exact role verdicts, and a signed successor work package provide the bounded authority instead.
2. `DEC-413 -> governs -> WP-45` must remain accepted and current.
3. `TEN-26` must continue to distinguish the bounded, draft-only, sample-bound Product Brain transport proven by WP-45 from the still-unproven production/full-drain transport claim.
4. WP-45's implementation commit, signed artifact hashes, final reviewer evidence, and Product Brain reconciliation must be durable and versioned. The successor may consume only that pinned foundation. WP-45 remains separately `building` until Randy reviews the three drafts, the temporary key is retired, and its private runtime root is cleaned; the successor cannot close or broaden those obligations.
5. While TEN-27 is open, DEC-415 is the only permitted projection exception: exact authoring-body/storage-body equality, exact canonical-envelope/stored-hash equality, reviewed generated-surface diff, and committed Chain-referenced instruction projection must all pass without force or lenient mode. Product Brain's global setup phase and defective clean-audit signal remain explicitly incomplete.

WP-45 remains a separate bounded predecessor. This shape does not rewrite its acceptance, claim boundary, temporary-key lifecycle, private-root cleanup obligation, or Randy-review requirement. The successor work package is created only after its own signed full Spec is captured, and it links to WP-45 as an informed-by/versioned foundation rather than silently expanding WP-45 scope.

### Instruction Surface Projection Gate

The required Chain authority consists of:

- a decision named **Chain-bounded autonomous Mindline delivery** defining permitted execution, `pb promote`/`pb verify` governance use, approval boundaries, privacy, rollback, audit, and fail-closed behavior;
- a standard named **Mindline team/role orchestration and instruction projection** defining role selection, phase signoff, authoritative setup source, generated surfaces, propagation, validation, and drift handling.

The authoritative instruction source must be mapped as a Product Brain setup asset or other explicit Chain-owned setup record before projection. `pb handshake` is the propagation mechanism for generated surfaces actually present in the repository. Under DEC-415 only, an explicit Chain-referenced repository instruction section may supplement the incomplete generated Codex body when its exact source, diff, and commit are evidenced.

No `AGENTS.md` or other instruction-surface edit is authorized until the Instruction Surface Projection Gate records:

- the authoritative Chain entries/setup asset and exact versions;
- intended generated surfaces;
- propagation command and output metadata;
- validation that generated instructions preserve the decision/standard boundaries;
- drift detection and repair behavior;
- no bypass of Chain, reviewers, source/destination authority, or DEC-64.

After propagation, direct inspection plus Product Brain reconciliation must prove every present surface matches the authoritative version or the exact DEC-415 scoped supplement. Any unexplained drift blocks delivery until repaired. This does not make Product Brain's global setup audit or authority-domain cutover ready.

## Acceptance

1. Blank-state browser flow configures a session-scoped Slack source, Product Brain destination, and versioned strategy without JSON editing or shell commands.
2. Connection tests expose only provider-returned non-secret identity and fail closed on wrong origin, workspace, channel, scope, governance, key ID, or collection contract.
3. Full inventory to a frozen watermark is complete, restartable, deduplicated, bounded, and reconciled with zero unexplained omissions in synthetic multi-page proof.
4. Proof selection is deterministic and processes `min(3, canonical_count)` per `(retrieval_strategy_id, format_variant)` while reporting every observed stratum and unselected count. The sample is sealed before retrieval and cannot be rerolled, refilled, or replaced after failure, inaccessible/manual outcome, or zero-draft disposition.
5. Multiple/no-link Slack messages, duplicates, edits/deletes, threads, attachments, private files, secrets, pagination loops, 429s, revoked tokens, and crash/restart have explicit tested states.
6. Recognized inaccessible/authenticated/private content enters a safe manual-support queue with evidence/missingness and never fabricates or promotes content.
7. The processor boundary rejects prompt injection, unsupported enums, missing evidence, authority changes, and secret/tool access.
8. The sample review packet cleanly separates retrieval completeness, stable meaning, relevance, semantic role, disposition, human support, destination mapping, and delivery acknowledgement.
9. `trusted_activation_completion` can pass with a truthful zero-draft review verdict. `trusted_value_observed` passes only when an exact approved Product Brain sample batch creates drafts, receives exact acknowledgements/readback, preserves attribution, replays with zero entry/relation mutations, and Randy judges at least one draft useful.
10. Sample success does not authorize the remainder. Every later batch requires exact fingerprint-bound approval enforced inside Product Brain delivery and respects separate maximum unique-write and mutation-attempt budgets, expiry, reconciliation, and cancel semantics.
11. The hardened loopback surface passes Host/origin/CSRF/content-type/body/header/timeouts/CSP/no-store/XSS/DNS-rebinding tests and browser smoke verification.
12. Sentinel Slack and Product Brain credentials are absent from every response, error, log, file, artifact, browser-storage surface, argument, telemetry payload, and repository scan after success, failure, disconnect, and restart.
13. Append-only journal reconstruction, queue projection, configuration drift, lease expiry, poison item, bounded retry, ambiguous mutation, and crash points fail or recover deterministically without duplicate side effects. Crash tests cover sealed intent, Product Brain send, Product Brain journal, reconciliation/readback, sealed receipt, and orchestration receipt-reference boundaries.
14. Eval readback and proof report the exact private/sample-bound/operator-assisted claim and block generalization, improvement, broad safety, and DEC-64/no-human claims.
15. `go test -count=1 ./...`, targeted `go test -race`, `go vet ./...`, pinned `govulncheck`, `gosec`, and `gitleaks` scans with zero unresolved high/critical or verified-secret findings, sentinel surface scans, `git diff --check`, hardened browser smoke, and private live proof pass. Tool unavailability is a blocked gate, not a silent skip.
16. The final report gives `READY`, `CONDITIONAL`, or `BLOCKED` separately for inventory, capped proof, experimental drain processing, and each exact destination batch, with evidence and next action. `READY_TO_EXPERIMENTAL_DRAIN` requires the complete capped-proof, accounting, stratum, privacy/security, recovery, budget, manual-burden, and human-confirmation gates and never authorizes delivery.
17. Founder discovery: from blank state, Randy completes Configure -> Prove -> Review without assistance, correctly explains the staged verdict and blocked reasons, judges any acknowledged drafts for usefulness, and evaluates whether session-only credential re-entry, manual-support burden, and per-batch approval burden are acceptable. Errors, retries, backtracks, help use, elapsed time, and friction are recorded. A zero-draft outcome is retained, not rerun to manufacture value, and the Product Team may revise the candidate surface from that evidence.
18. v0.1 artifacts and legacy CLI behavior remain readable and unchanged; the control plane imports legacy history read-only, avoids dual writes, and preserves the current CLI as rollback during the experiment.
19. Changing a retrieval registry/implementation, canonicalization version, SSRF/network policy, or readiness contributor after checkpoint rejects resume or forks a new run and invalidates derived readiness/sample/outbox authority.
20. Slack Web API, external Slack export, and Product Brain readiness contributors prove their adapter-specific required checks through versioned evidence fingerprints; missing checks fail, and `N/A` is allowed only by the selected adapter contract.
21. A run-plan resource budget or experimental-drain confirmation cannot satisfy Product Brain delivery authorization; only an immutable batch approval containing `maximum_destination_writes`, `maximum_mutation_attempts`, and the exact destination/outbox/preflight/privacy identity can authorize delivery.
22. Scoped Product/Architecture/Engineering/Chain Steward/Delivery Quality/Risk ownership, DEC-413, TEN-26 transport truth, and versioned WP-45 foundation evidence are reconciled in Product Brain before Spec/Delivery Authority is granted. Global authority-domain cutover remains legacy and is explicitly not claimed or used.
23. DEC-414, STD-22, and DEC-415 are active and verified; the selected setup body and canonical envelope are independently hash-verified; `pb handshake` output and the exact Chain-referenced repository supplement are diff-verified and committed; global Product Brain setup/audit incompleteness remains visible.
24. Private Slack content has no hosted inference egress by default. Any enabled hosted inference is a pinned, explicit per-run opt-in with disclosed provider/model, minimized/redacted outbound projection, retention/data-use assumptions, and sentinel proof.

## Exclusions

- Hosted multi-user authentication, database, scheduler, or cloud control plane.
- OAuth installation UX beyond the minimal source token/provider port.
- Persistent credential storage; OS/managed secret providers are future implementations.
- Authenticated-browser extraction or reuse of LinkedIn/Notion/Google/other sessions.
- Bypassing paywalls, DRM, login, bot protections, provider terms, or private-source consent.
- Support for every observed host or format in one slice.
- Arbitrary routing DSL, destination schema builder, or integration marketplace.
- Non-draft Product Brain writes, automatic promotion/commit, or blanket approval of a drain.
- Executing the complete live Slack remainder in this proof run.
- Claims of held-out quality, generalization, production readiness, or no-human autonomy.

## Diagnose Review Record

`DIAGNOSE-SYNTHESIS-v3` reached clean phase consensus:

- Domain/User Job: PASS after outcome, accounting, batch-approval, readiness, taxonomy, and empowerment corrections.
- Systems Architecture/Team Topologies: PASS after staged readiness, exact denominator, request-origin scope, run binding, journal/projection, and ownership corrections.
- Risk/Safety/DevSecOps: PASS after exact batch authority, prompt-injection isolation, manual authenticated/private lane, Slack security, and sentinel-secret corrections.

The Shape phase must review this exact artifact before its hash is used as downstream authority.
