# WP-45 Plan: First Context-Lens Slack-to-Product-Brain Slice

Date: 2026-07-14
Phase: Plan
Version: draft-11
Spec: `.productbrain/specs/2026-07-14-wp-45-context-lens-slack-routing.md` draft-16, SHA-256 `22faacdfc4cc054134f8633cd34078477926dc31d525ae0e3cb1f3d29969d53a`

## Delivery objective

Implement and prove one bounded product-general Mindline route from twelve private Slack link captures through a generic source/link graph, two configurable context lenses, stable source meaning, explicit dispositions, and one three-node destination-neutral constellation into exactly three Product Brain draft entries and two relations. Produce read-only preflight evidence, exact delivery acknowledgements, immutable first-run/replay history, a passing `delivery` proof gate, and one integrated packet for Randy's review.

## Guardrails

- Work only on `codex/slack-strategic-routing`; do not merge or build on WP-51 corpus work.
- Product-OS and product-talk remain read-only references.
- Do not persist, log, echo, commit, or pass the temporary Product Brain key as a CLI argument.
- Do not place the private Slack fixture, Randy lens profile, live judgment manifest, live evidence artifacts, Product Brain key ID/profile, or live run outputs in git. Keep them under one unique owner-verified temporary root created with `0700` directory mode; never reuse a fixed/shared `/tmp` path.
- No Product Brain mutation until Spec/Plan are durable, WP-45 is reconciled, handoff audit has no failure, all tests pass, outbox is exactly five operations, privacy scan is clean, and read-only preflight passes.
- No automatic commit/promotion of Product Brain drafts.
- Stop after the successful replay/proof packet is ready for Randy; key retirement remains a visible manual action.

## Product-model fit

- `CREATE`: `internal/routing` context-lens, stable source-meaning, and semantic-constellation contracts.
- `EXTEND`: Slack normalization, Product Brain adapter, artifact writing, eval readback, and proof claims.
- Adapter boundaries: Slack owns native provenance and source-graph conversion; routing imports no source adapter and owns only generic meaning/relevance/disposition; Product Brain mapping owns collection fields and count/policy-derived review actions; transport owns AKI; delivery owns reconciliation/history; eval owns claims.
- Live fixture URLs and Randy lens values appear only in `/tmp` inputs, never production branch logic.

## Implementation sequence

### 0. Persist the signed handoff and reconcile Product Brain authority

Before any implementation or private-data work:

1. persist the exact signed Spec and Plan under `.productbrain/` and record their SHA-256 hashes in the evidence ledger;
2. update WP-45's stale description/problem/done-when/build-sequence/risk fields to the signed context-lens, semantic-meaning, public-outbox, preflight, truthful-attribution, and reconciliation contracts;
3. link the exact Spec/Plan refs and hashes from WP-45's build contract/evidence fields;
4. set only the lifecycle state justified by the signed handoff;
5. run `pb audit WP-45 --phase handoff --verbose` from the repository root.

Gate: zero audit failures before Step 1. Any warning is explicitly reconciled against the signed inline element scope and recorded in the evidence ledger; a blocking warning or authority mismatch stops implementation. Final PB reconciliation remains required after proof.

### 1. Establish generic routing contracts and safe writers

Add:

- `internal/routing/types.go`
- `internal/routing/url.go`
- `internal/routing/compile.go`
- `internal/routing/validate.go`
- `internal/routing/writer.go`
- `internal/routing/review.go`
- focused `_test.go` files beside each unit

Implement:

- schema-versioned lens profile, link artifact, judgment manifest, source graph, route decisions, and summary types;
- deterministic canonical URL/occurrence/source/edge IDs and tracking-parameter removal;
- generic source kinds and URL kinds with one-hop related-source admission;
- complete accounting for every source record, occurrence, primary URL, and admitted follow-up URL;
- exactly one stable semantic assessment, lens result per configured lens, and disposition per canonical source;
- semantic-node/edge validation independent from destination mapping and disposition filtering;
- deterministic artifact ordering/fingerprints;
- atomic no-symlink writers and private-local review projection;
- private runtime directories at `0700` and private artifacts at `0600`, with current-owner verification and symlink-safe exclusive creation.
- optional `MINDLINE_PRIVATE_RUNTIME_ROOT` containment mode; when set for a private run, every routing/profile/outbox/preflight/delivery/readback/proof input and output must resolve beneath the same verified root or the command fails before reading/writing/network activity.

Tests:

- duplicate occurrences collapse without losing records;
- every depth-1 source is routed and contributes to derived lens coverage;
- incomplete sources require missingness and cannot promote;
- changing only lenses/disposition preserves source meaning and node roles;
- zero-lens and alternate-user profiles pass without Randy/Product Brain vocabulary;
- symlink, traversal, stale file, schema, fingerprint, and evidence-ref failures block.
- wrong owner, group/world-readable mode, reused private root, or permission-hardening failure blocks before private data is written.
- containment tests reject sibling, parent, symlink, `..`, alternate-realpath, and mixed-root inputs/outputs when private runtime mode is active.

Gate: `go test -count=1 ./internal/routing`.

### 2. Add the Slack route adapter and neutralize bespoke defaults

Add/modify:

- `internal/adapters/slack/routing.go`
- `internal/adapters/slack/routing_test.go`
- `internal/adapters/slack/normalize.go`
- `internal/adapters/slack/normalize_test.go`
- `internal/cli/strategic_routing.go`
- `internal/cli/runner.go`
- `internal/cli/cli_test.go`

Implement:

- conversion from existing Slack `Payload` to generic routing source records and URL occurrences;
- complete removal of Slack compilation/imports from `internal/routing`, including source-neutral generic summary vocabulary;
- preservation of private Slack provenance only in local graph fields;
- removal of implicit `Research Landscape` and `slack-capture` defaults when no explicit source metadata supplies them;
- `mindline slack route ...` argument parsing and route artifact writing;
- private-local integrated identifiers without leaking native values into public projection.

Tests:

- Slack adapter emits the same generic vocabulary used by a synthetic bookmark/document adapter fixture;
- the complete unrelated-source routing result rejects Slack, Product Brain, and Tolaria vocabulary;
- order remains oldest-to-newest;
- private permalink/channel/user/timestamp remain local and are absent from public candidates;
- old normalize/corpus-intake behavior remains compatible except for removed bespoke classification defaults.

Gate: `go test -count=1 ./internal/adapters/slack ./internal/cli`.

### 3. Build Product Brain destination profile, outbox, and privacy projection

Add:

- `internal/productbrain/delivery_profile.go`
- `internal/productbrain/outbox.go`
- `internal/productbrain/outbox_writer.go`
- `internal/productbrain/privacy.go`
- `internal/productbrain/outbox_test.go`
- `internal/productbrain/privacy_test.go`

Implement:

- `productbrain-delivery-profile/v0.1` with expected workspace ID/slug, expected non-secret key ID, exact three supported role mappings, one supported relation mapping, transport selector, draft-only posture, and optional signed credential/private-runtime review lifecycle;
- deterministic 80-bit numeric digest IDs with collection prefixes;
- three role mappings and fail-closed unsupported roles/relations;
- exactly fingerprinted entry/relation operations including public evidence and truthful agent/operator attribution;
- public-safe `review_context` inside `outbox.json`, with `capture-001` through `capture-012` ordinals, original order, canonical/duplicate relationships, public evidence and missingness, stable meaning, lens results, dispositions, nodes/edges, and Product Brain actions derived from actual operation counts plus the explicit signed review lifecycle—never Slack-native identifiers;
- an exact non-secret `delivery_profile_snapshot` inside `outbox.json`, containing the destination mapping, trusted-origin expectation, workspace identity, and expected key ID;
- one canonical outbox fingerprint over operations, review context, and profile snapshot so delivery/proof can reconstruct from the signed Spec's existing `productbrain-outbox/v0.1` artifact without parsing Markdown or following an untrusted filesystem path;
- one shared structural validator used by load, preflight, delivery, integrated review, and proof boundaries for top/payload fingerprints, unique identities, kind/payload shapes, dependency closure, embedded authority, and privacy;
- central exact pending-action validation against current operation counts plus independently derived credential/runtime policy; allow the delivered-v0.1 three-action set only when both its exact immutable outbox fingerprint and ordered strings match;
- strict public allowlist and three-stage outbound scan;
- `mindline product-brain outbox ...` with no network activity.

Tests:

- exact live collection field contracts for Landscape, Insights, and Tensions;
- same source/node identity gives the same ID; changed identity differs; forced collision blocks;
- unsupported mapping and duplicate ID/payload mismatch block the whole outbox;
- Slack IDs/permalinks, private/signed URLs, paths, tokens, runtime secret, raw errors, and non-allowlisted actor fields block without echoing values;
- attribution fields and non-secret key ID survive fingerprinting and strict scan;
- public-only Exxperts fixture compiles to exactly three entry and two relation operations.
- alternate-user compilation produces a one-entry count-aware action without inventing a temporary credential; the explicit live-like review policy produces exact three-entry/two-relation, temporary-key-retirement, and owner-validated cleanup actions in the signed snapshot.
- all four explicit credential/runtime policy combinations derive non-contradictory actions; a refingerprinted unauthorized action and a one-entry/new outbox reusing the legacy strings fail the shared outbox/proof boundary; only the actual immutable delivered fingerprint/action set passes read-only integrated review unchanged.
- embedded review context reconstructs all twelve ordered capture rows and depth-1 context from public-safe structured data; tamper, missing embedded profile snapshot, fingerprint mismatch, and private-native field injection block `outbox.json`.

Gate: `go test -count=1 ./internal/productbrain` with no network.

### 4. Implement trusted AKI transport and read-only preflight

Add:

- `internal/productbrain/transport.go`
- `internal/productbrain/aki.go`
- `internal/productbrain/preflight.go`
- `internal/productbrain/preflight_writer.go`
- `internal/productbrain/aki_test.go`
- `internal/productbrain/preflight_test.go`
- `internal/cli/productbrain_delivery.go`

Modify Runner construction only enough to inject an HTTP round tripper and secret provider in tests.

Implement:

- production trust pinned to exact `https://gateway.productbrain.io` before secret-provider access;
- no redirects, bounded bodies, TLS, timeouts, mutation retry prohibition, and the Spec's single exact closed safe-delivery category set shared by adapter emission, sealed-run validation, and proof; normalize possibly committed mutation failures to `ambiguous_outcome`, malformed responses and unknown transport categories to `remote_failure`, arbitrary non-transport text to `local_state_failure`, and reject `transport_failure`, `invalid_response`, or any other unsigned category;
- a replaceable `ProductBrainTransport` port for workspace resolution, collection-field discovery, entry/relation reads, and mutations, plus a companion runtime-secret scanner; transport construction occurs only at the CLI boundary;
- verified AKI calls for `resolveWorkspace`, `chain.getCollectionFields`, `chain.getEntry`, `chain.searchEntries`, `chain.createEntry`, `chain.listEntryRelations`, and `chain.createEntryRelation`;
- fixed environment secret provider `MINDLINE_PRODUCT_BRAIN_API_KEY` with no key flag;
- `mindline product-brain preflight ...` using the injected transport port, `resolveWorkspace`, canonical sorted `chain.getCollectionFields` contracts, runtime-secret rescan, zero mutation counter, exact workspace/scope/key/schema checks, exact unique base/collection gate sets, and fingerprinted artifact; unknown or duplicate live field descriptors fail closed while the known destination person-ID type remains schema-valid.

Tests:

- exact `{fn,args}` request/response contracts;
- exact safe-category membership and normalization plus runtime/proof rejection of arbitrary blocked-operation categories;
- origin variations, redirects, wrong host/port/userinfo/query, wrong workspace/scope/key ID, missing key, and oversized/malformed responses fail closed;
- secret provider is not accessed for an untrusted origin;
- preflight makes zero create calls and contains no secret/private value;
- fake trusted test origin works only with explicitly injected fake trust and fake secret.

Gate: `go test -count=1 ./internal/productbrain ./internal/cli`.

### 5. Implement single-writer delivery, reconciliation, immutable history, and final packet

Add:

- `internal/productbrain/delivery.go`
- `internal/productbrain/delivery_store.go`
- `internal/productbrain/delivery_review.go`
- `internal/productbrain/delivery_test.go`
- `internal/productbrain/delivery_store_test.go`
- `internal/productbrain/delivery_review_test.go`

Implement:

- required matching preflight load, no-replace snapshot, and per-run lineage;
- authoritative consumption of fingerprint-bound `review_context` and `delivery_profile_snapshot` fields inside `outbox.json`; no Markdown parsing, secondary proof-critical artifact, or ambient routing-directory lookup;
- exclusive delivery lock with same-host proven-dead-process stale recovery only;
- journal-first authoritative state, rebuildable projections, no-replace sealed run records, deterministic ordered history;
- entry preflight by deterministic ID plus exact-name collection search and verified server duplicate-name handling;
- exact entry comparison including collection, ID, name, draft, data, public evidence, and `createdBy`;
- exact full-result relation identity and metadata comparison; duplicate exact and exact-plus-conflicting matches block; omit unproved Product Brain `proposedBy=user`;
- reconcile before mutation and after every clean/ambiguous create result;
- integrated `mindline-review-packet.md` with exactly twelve original-capture rows, a separate depth-1 section, public evidence/missingness, meaning, complete lens rationale/confidence, semantic nodes/edges, expected/actual destination identities and types, attribution, every sealed preflight/run lineage, replay, and manual actions;
- a read-only `mindline product-brain review` command that verifies routing/outbox/delivery fingerprint lineage and can augment a legacy immutable outbox's review projection without mutating its authority.
- explicit completed/interrupted/failed run counts; a failed external-precondition run remains valid only with sealed passing preflight evidence and zero mutation counters/observations, and cumulative bounded mutations plus a later zero-mutation replay remain exact.
- private review/history artifacts remain `0600` beneath the owner-only runtime root even when public-safe embedded outbox fields could otherwise use broader modes.

Tests:

- existing-match, absent/create/readback, duplicate response, ambiguous commit/no-commit, mismatches, name conflict, and attribution omission/rewriting;
- relation `ifMissing`, metadata mismatch, and ambiguous outcomes;
- concurrent invocation refusal, stale-lock proof, duplicate sequence/path prevention, no-overwrite sealing;
- crash injection after every journal/network/projection/seal boundary and idempotent recovery;
- multiple reruns preserve evidence; summary/packet regenerate without deleting sealed runs;
- preflight snapshot mismatch/absence blocks and exact snapshot is retained in every sealed run;
- golden packet accounts for every original capture once and exposes all required review fields.
- embedded review/profile fields survive delivery-directory reconstruction through the immutable outbox snapshot, while tamper, missing field, outbox-fingerprint mismatch, or private-field injection blocks before network activity.
- current policy/count actions and the one exact immutable delivered legacy set survive integrated review, while changed/reordered/partial lifecycle actions fail before rendering or proof.
- private writer permission tests assert `0700` directories, `0600` artifacts, current ownership, symlink/existing-path refusal, and no mode regression after atomic replacement.

Gate: `go test -count=1 ./internal/productbrain ./internal/cli`.

### 6. Extend eval readback and add the delivery proof claim

Modify:

- `internal/evalreadback/types.go`
- `internal/evalreadback/readback.go`
- `internal/evalreadback/readback_test.go`
- `internal/evalproof/types.go`
- `internal/evalproof/proof.go`
- `internal/evalproof/proof_test.go`
- `internal/cli/runner.go`
- relevant CLI tests

Implement:

- artifact recognition for routing, the full immutable outbox, outbox summary, preflight snapshot, delivery history, every referenced sealed run/preflight snapshot, and latest summary;
- safe metrics/flags for complete accounting, lens coverage, privacy, draft status, actor attribution, first-run mutations, replay zero mutations, and immutable lineage;
- `ClaimDelivery = "delivery"` with dedicated mandatory gates;
- derive delivery authority by opening every canonical run/snapshot ref, rejecting cached-summary-only proof, missing/tampered/extra/symlinked/permission-widened files and unsupported run schemas, validating exact embedded/sealed equality, exact preflight gate sets, every route-to-outbox-to-preflight-to-run/summary binding, the complete source-by-lens matrix, and exact operation/destination/readback identities rather than aggregate maxima;
- retain existing `safety` zero-write semantics unchanged while truthfully surfacing the bounded delivery's five destination/Product Brain writes; permit them only under the dedicated `delivery` claim;
- readback limits: private, curated, operator/agent-judged, non-held-out, non-generalizable, no autonomy claim.
- preserve exact sample status such as `private_curated_sample`, keep generated Chain drafts work-package-neutral, and apply private-root containment/owner/`0700`/`0600` rules to eval readback, proof, and loop-decision inputs/outputs.

Tests:

- pass only with exact source/URL/lens coverage, clean privacy, a complete route/full-outbox/preflight/sealed-run/summary lineage, exact operation acknowledgements and destination identities, draft/actor readback, intact immutable history, and later zero-mutation replay;
- fail on missing/mutated preflight snapshot, mismatch, partial acknowledgement, active entry, actor rewrite, blocked operation, replay mutation, or claim inflation;
- existing safety/improvement/generalization/DEC-64 behavior remains unchanged.

Gate: `go test -count=1 ./internal/evalreadback ./internal/evalproof ./internal/cli`.

### 7. Run the product-fit and full local gates before private data

Create only synthetic checked-in test fixtures under `testdata/routing/`:

- alternate-user lens profile with different lens IDs/content;
- non-Slack bookmark/document normalized source whose complete result is free of source/destination-specific vocabulary;
- public synthetic evidence, judgments, and expected summaries.

Run:

```sh
gofmt -w <changed-go-files>
go test -count=1 ./...
git diff --check
```

Inspect for bespoke coupling:

```sh
rg -n "Research Landscape|slack-capture|building-product-brain|ai-native-organizational-design|delete-later|EXXETA|Tolaria" internal testdata/routing
```

Expected: Randy/live-fixture values do not appear in production code; synthetic fixture values appear only where intentionally asserted.

Gate: all tests pass and the non-Slack alternate-user invariant is proven before reading or writing live runtime data.

### 8. Prepare and route the bounded private Slack fixture

Under one newly allocated `os.MkdirTemp`-style root such as `/tmp/mindline-wp45-live-<random>` only:

- create the root exclusively with `0700`, verify it is owned by the current user and is not a symlink, and refuse reuse or permission widening;
- write every fixture, lens, judgment, enrichment, profile, packet, state, history, and proof artifact with `0600` beneath `0700` directories;
- set `MINDLINE_PRIVATE_RUNTIME_ROOT` to that exact verified root for every live CLI invocation so mixed-root or escaped paths fail before access;
- record only the root path for handoff; do not copy private contents into PB, git, terminal output, or world-readable locations.

1. use the connected Slack read surface to materialize the exact twelve selected self-DM messages oldest-to-newest;
2. write the two Randy context lenses as versioned input config;
3. create bounded public enrichment artifacts with explicit missingness, exactly one admitted depth-1 source, and no private browser/session data;
4. create the explicit operator/agent judgment manifest covering all twelve canonical sources and twenty-four lens results;
5. route with the new CLI and inspect the integrated routing packet.

Required route assertions:

- 12 source records, 12 primary occurrences, 11 primary canonical URLs, exactly one admitted depth-1 source, 24 lens results;
- duplicate pair preserved as two records/one canonical source;
- inaccessible LinkedIn sources incomplete and unpromoted;
- insurance negative control `not_matched` for both lenses and unpromoted;
- Exxperts only live promotion, exactly three nodes/two `related_to` edges;
- no code or artifact invents inaccessible context.

If any assertion fails, stop before outbox compilation, correct the generalized contract or judgment input, rerun tests, and capture the learning in PB.

### 9. Compile, preflight, deliver, replay, and prove

1. Securely obtain the temporary key's non-secret `keyId` from an allowlisted `resolveWorkspace` probe without echoing the key; write the live local delivery profile only at `<runtime-root>/inputs/productbrain-delivery-profile.json`.
2. Compile outbox and require exactly five operations, zero strict privacy findings, and no private Slack value.
3. Run read-only preflight using an interactive non-echoing environment injection; require exact workspace `delete-later`, read/write scope, expected key ID, and zero mutation calls.
4. Run first delivery with the matching preflight; require three draft entries and two relations acknowledged by exact readback.
5. Run delivery again with the same outbox/preflight; require zero new entries and zero new relations with all five acknowledged.
6. Render the integrated review packet from the verified routing/outbox/delivery lineage.
7. Assemble one proof input containing the routing summary, full outbox and summary, latest preflight, delivery history/summary, every sealed run, and every referenced preflight snapshot; run eval readback and the dedicated delivery proof gate.

Commands, with secret injection performed interactively and never written into shell history:

```sh
go run ./cmd/mindline product-brain outbox <runtime-root>/routing --profile <runtime-root>/inputs/productbrain-delivery-profile.json --out <runtime-root>/outbox
go run ./cmd/mindline product-brain preflight <runtime-root>/outbox --out <runtime-root>/preflight
go run ./cmd/mindline product-brain deliver <runtime-root>/outbox --preflight <runtime-root>/preflight --out <runtime-root>/delivery
go run ./cmd/mindline product-brain deliver <runtime-root>/outbox --preflight <runtime-root>/preflight --out <runtime-root>/delivery
go run ./cmd/mindline product-brain review <runtime-root>/routing --outbox <runtime-root>/outbox --delivery <runtime-root>/delivery --out <runtime-root>/review
go run ./cmd/mindline eval readback <runtime-root>/proof-input --out <runtime-root>/readback
go run ./cmd/mindline eval proof-gate <runtime-root>/proof-input --out <runtime-root>/proof --claim delivery
```

Gate: delivery proof verdict `pass`, integrated packet complete, all destination objects remain drafts, and no mutation is attempted after any failed check.

### 10. Review, PB reconciliation, and stop

Run the five-role LOOP implementation review and a second clean pass on the exact code/artifact version. Fix blockers and rerun the full tests/proof after every material correction.

Update PB with:

- final WP-45 scope/status, exact Spec/Plan refs and hashes;
- implementation/test/proof results and immutable artifact fingerprints;
- live destination draft IDs and relations, without private Slack contents or the key;
- durable learnings, tensions, tradeoffs, claim limits, and next product-general target;
- explicit reminder that the temporary key must be retired after Randy's review.
- private runtime retention status: retain the owner-only root only until Randy completes review; after explicit confirmation and key retirement, validate owner/path/symlink boundaries and remove the whole root. Do not auto-delete before review or perform cleanup without confirmation.

Run `pb audit WP-45 --phase handoff --verbose` again after final reconciliation. Stop with the integrated packet path and Product Brain draft IDs ready for Randy. Do not merge, commit Product Brain drafts, drain more Slack, or claim completion beyond the first slice.

At handoff, mark both key retirement and private-root cleanup as pending manual actions. If Randy confirms review in a later turn, retire the key first, then perform owner/symlink-validated cleanup; absence of that later confirmation does not justify leaving the retention policy undocumented.

## Verification matrix

| Gate | Required evidence | Blocks |
|---|---|---|
| Generic routing | alternate-user/non-Slack tests; complete canonical-source lens coverage | live private route |
| Privacy | strict outbox scan; runtime secret scan; redacted negative tests | outbox/preflight/delivery |
| Credential audience | trusted origin before secret; workspace/scope/key ID match | any mutation |
| Destination safety | exact schema mapping; draft-only; truthful actor; name conflict handling | any mutation |
| Reconciliation | exact entry/relation readback and mismatch fail-close | acknowledgement |
| History | exclusive lock; journal authority; immutable no-replace runs/preflight lineage | replay/proof |
| Replay | later run has zero entry/relation creates and five acknowledgements | delivery proof |
| User value | one packet accounts for 12 captures and three connected drafts | handoff |
| Claims | private/curated/operator-judged/no-generalization readback | closure language |

## Stop conditions

Stop and ask Randy only if:

- a source's meaning or relevance requires founder judgment that cannot be responsibly encoded in the explicit manifest;
- the live Product Brain schema/API differs from the verified contract in a way that changes user-visible mapping or safety;
- the temporary credential resolves to a different workspace/key ID or lacks read/write scope;
- an existing Product Brain entry/name/relationship conflicts with the intended constellation;
- a privacy scanner finds a value whose safe treatment is a product decision rather than a mechanical fix.

Otherwise continue through the successful first delivery, replay, proof, PB reconciliation, and review handoff.
