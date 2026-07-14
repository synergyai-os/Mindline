# WP-45 Spec: Context-Lens Slack Routing and Acknowledged Product Brain Delivery

Date: 2026-07-14
Phase: Spec
Version: draft-16
Shape: `.productbrain/specs/2026-07-14-wp-45-context-lens-slack-routing-shape.md` draft-5, SHA-256 `650d628d02fad2601ad74dba67aff68c55739b6b1ea9f37906f058070c3c6eeb`

## Authority and claim boundary

This spec implements WP-45 under DEC-412, DEC-413, STD-21, DOMAIN-1, DEC-254, PRI-1, BR-1, STD-12, STD-20, TEN-25, and TEN-26.

The slice proves a private, curated, operator-judged route from a bounded Slack fixture to draft knowledge in one external Product Brain workspace. It does not prove full-drain operation, autonomous judgment, held-out accuracy, generalization, or no-human destination writes.

## Product outcome

Given twelve selected Slack link captures and two workspace-defined context lenses, Mindline must produce a complete review packet showing what each public source means, whether it matters to either lens, what semantic role it plays, what was promoted or held, and why. One promoted public source must become a three-node Product Brain constellation—Landscape entity, Insight, and Tension—with two reconciled relations. Replaying the exact outbox must add no entries or relations.

## Product-model fit

Eligibility: `CREATE` context-lens resolution and semantic-constellation contracts; `EXTEND` normalized sources, source enrichment, Product Brain destination mapping, delivery, and eval/readback.

Ownership boundaries:

- Slack adapter: private Slack schema and complete conversion into the source-neutral graph; the routing package imports no source adapter.
- Routing core: generic source/link graph, lens results, dispositions, semantic nodes, and semantic edges.
- Source enrichment: public URL metadata, bounded excerpts, missingness, and one-hop follow-up evidence.
- Product Brain adapter: collection, field, relation, and public outbound mapping.
- Product Brain transport: workspace probe, create/read APIs, relation APIs, and capability errors.
- Delivery engine: durable state, reconciliation, acknowledgements, replay, and safe diagnostics.
- Eval/readback: evidence aggregation and explicitly bounded delivery proof.

No core routing type, compiler dependency, or generic routing projection contains Slack fields/vocabulary, Product Brain fields/vocabulary, Randy-specific lens identifiers, or Tolaria taxonomy.

## Command surface

### Route a Slack fixture

```sh
mindline slack route <slack-export.json> \
  --links <routing-link-artifacts.json> \
  --lenses <context-lens-profile.json> \
  --judgments <routing-judgments.json> \
  --out <routing-dir>
```

The Slack adapter normalizes the export, then invokes the generic routing compiler. This command performs no destination writes.

### Compile a Product Brain outbox

```sh
mindline product-brain outbox <routing-dir> \
  --profile <productbrain-delivery-profile.json> \
  --out <outbox-dir>
```

This command performs no network activity. It maps only promoted semantic nodes and edges and blocks the entire outbox if any outbound privacy or schema validation fails.

### Deliver or reconcile an outbox

```sh
MINDLINE_PRODUCT_BRAIN_API_KEY=... \
mindline product-brain preflight <outbox-dir> --out <preflight-dir>

MINDLINE_PRODUCT_BRAIN_API_KEY=... \
mindline product-brain deliver <outbox-dir> \
  --preflight <preflight-dir> \
  --out <delivery-dir>
```

The credential environment variable is fixed; no credential flag exists. `preflight` is read-only and writes `productbrain-preflight/v0.1`, bound to the outbox and profile fingerprints, with pass/fail results for trusted origin, outbound runtime-secret scan, workspace ID/slug, governance capability, read/write scope, and expected key ID. It records non-secret identifiers and safe categories only and asserts zero mutation calls. `deliver` requires a matching passing preflight artifact, then repeats every mutable external precondition at invocation start before any mutation so stale preflight state cannot authorize a write. The delivery engine reconciles before every attempted mutation and after every clean or ambiguous response.

The first implementation uses the AKI transport profile. A future REST profile implements the same internal transport interface without changing routing, outbox, or delivery-state schemas.

### Render the integrated review packet

```sh
mindline product-brain review <routing-dir> \
  --outbox <outbox-dir> \
  --delivery <delivery-dir> \
  --out <review-dir>
```

This read-only command verifies the routing, outbox, preflight, and sealed-delivery lineage, then renders one private-local packet. It may augment an older immutable outbox's review projection from fingerprint-matched routing artifacts, but it never changes the authoritative outbox or delivery history.

## Input and artifact contracts

All JSON artifacts use UTF-8, explicit `schema_version`, deterministic ordering, and canonical JSON fingerprints computed with the fingerprint field omitted. Unknown schema versions fail closed. Writers use repository-safe directory validation, reject symlink output paths, stage temporary files in the selected output directory, `fsync`, and atomically rename. A rerun may replace only the command-owned named artifacts; unexpected stale files are reported and never treated as proof. Private runtime roots are exclusively allocated, current-user-owned directories with exact mode `0700`; every existing component beneath them must remain current-user-owned with directories at `0700` and files at `0600`. When `MINDLINE_PRIVATE_RUNTIME_ROOT` is set, routing, outbox, preflight, delivery, integrated-review, readback, proof, and loop-decision inputs and outputs must remain inside that verified root.

### `context-lens-profile/v0.1`

```json
{
  "schema_version": "context-lens-profile/v0.1",
  "profile_id": "randy-research-contexts",
  "profile_version": "2026-07-14.1",
  "lenses": [
    {
      "lens_id": "building-product-brain",
      "name": "Building Product Brain",
      "question": "What does this reveal about tools, competitors, resources, repositories, approaches, or risks relevant to building Product Brain?",
      "include": ["bounded natural-language criteria"],
      "exclude": ["bounded natural-language criteria"]
    }
  ]
}
```

Rules:

- zero to eight lenses; `lens_id` is a stable lowercase slug unique within the profile;
- lens values are workspace/user configuration, not core constants or Product Brain entries;
- profile version changes when lens meaning changes;
- empty lens profiles remain valid and yield no lens results;
- the live pilot contains exactly `building-product-brain` and `ai-native-organizational-design`.

### Normalized source/link graph `strategic-source-graph/v0.1`

```json
{
  "schema_version": "strategic-source-graph/v0.1",
  "adapter": {"kind": "slack", "version": "v0.1"},
  "source_records": [{
    "source_record_id": "local deterministic ID",
    "source_kind": "message",
    "occurred_at": "RFC3339",
    "raw_provenance_ref": "local-only ref",
    "url_occurrence_ids": ["occ-..."]
  }],
  "url_occurrences": [{
    "url_occurrence_id": "occ-...",
    "source_record_id": "src-...",
    "observed_url": "https://...",
    "canonical_url_id": "url-..."
  }],
  "canonical_urls": [{
    "canonical_url_id": "url-...",
    "canonical_url": "https://...",
    "kind": "github_repository",
    "depth": 0,
    "parent_canonical_url_id": null,
    "discovery": "source_occurrence",
    "enrichment_state": "complete",
    "missingness": []
  }],
  "edges": [{
    "edge_id": "edge-...",
    "type": "source_record_contains_url",
    "from": "src-...",
    "to": "url-...",
    "evidence_refs": ["occ-..."]
  }]
}
```

Core enums:

- source kinds: `message`, `document`, `bookmark`, `transcript_segment`, `unknown`;
- URL kinds: `github_repository`, `linkedin_post`, `linkedin_article`, `youtube_video`, `article`, `pdf`, `generic_web`, `unknown`;
- edge types: `source_record_contains_url`, `source_links_to`, `canonical_duplicate_of`;
- enrichment state: `complete`, `partial`, `inaccessible`, `failed`, `not_attempted`.

Slack-native channel ID, message timestamp, author, permalink, raw text, file metadata, and thread context may exist only in adapter-local input or local provenance refs. They must not appear in the public outbox.

URL canonicalization lowercases scheme and host, removes default ports and fragments, normalizes a trailing root slash, sorts retained query parameters, and drops an explicit allowlist of tracking parameters including `utm_*`, `fbclid`, `gclid`, and LinkedIn tracking parameters. It does not resolve redirects without an enrichment artifact. GitHub owner and repository path case is preserved in display metadata but compared case-insensitively for canonical identity. Canonical IDs are `url-` plus the first 20 lowercase hexadecimal characters of SHA-256 over canonical URL.

Each input source record and URL occurrence must be represented exactly once. Multiple occurrences may point to one canonical URL. The June 23 duplicate pair therefore remains two source records and two occurrences but one primary canonical URL.

### `routing-link-artifacts/v0.1`

```json
{
  "schema_version": "routing-link-artifacts/v0.1",
  "items": [{
    "canonical_url": "https://github.com/EXXETA/exxperts",
    "retrieved_at": "RFC3339",
    "state": "complete",
    "public_metadata": {"title": "...", "author": "...", "published_at": null},
    "public_excerpts": [{"excerpt_id": "excerpt-1", "text": "bounded text", "locator": "README section"}],
    "related_urls": [{
      "url": "https://...",
      "relation": "source_links_to",
      "discovery_evidence_ref": "excerpt-1",
      "semantically_relevant": true
    }],
    "missingness": []
  }]
}
```

Rules:

- artifacts contain public evidence only; browser cookies, session data, request headers, local paths, and private source text are invalid;
- every supplied related URL is either admitted as a depth-1 node with a parent edge or rejected with a recorded validation reason;
- only `semantically_relevant=true` related URLs with valid evidence may become children;
- depth greater than one is invalid in this slice;
- inaccessible LinkedIn items have explicit missingness and no invented metadata, excerpts, or children;
- excerpts are individually bounded to 1,000 Unicode code points and 4,000 code points per canonical URL.

### `routing-judgments/v0.1`

The judgment manifest is explicit, inspectable operator/agent input. It is not generated autonomously by this slice.

```json
{
  "schema_version": "routing-judgments/v0.1",
  "judgment_method": "operator_agent_review",
  "judged_at": "RFC3339",
  "profile_id": "randy-research-contexts",
  "profile_version": "2026-07-14.1",
  "sources": [{
    "canonical_url_id": "url-...",
    "lens_results": [{
      "lens_id": "building-product-brain",
      "result": "matched",
      "confidence": 0.91,
      "rationale": "bounded explanation",
      "evidence_refs": ["excerpt-1"],
      "missingness": []
    }],
    "semantic_assessment": {
      "primary_role": "external_entity",
      "summary": "Evidence-backed account of what the source is or means, independent of current relevance or destination action.",
      "confidence": 0.91,
      "evidence_refs": ["excerpt-1"],
      "missingness": []
    },
    "disposition": "promote",
    "disposition_rationale": "bounded explanation",
    "semantic_nodes": [{
      "semantic_node_id": "entity-exxperts",
      "role": "external_entity",
      "name": "Exxperts",
      "description": "bounded evidence-backed statement",
      "confidence": 0.91,
      "lens_refs": ["building-product-brain", "ai-native-organizational-design"],
      "evidence_refs": ["excerpt-1"],
      "attributes": {}
    }],
    "semantic_edges": [{
      "from": "entity-exxperts",
      "type": "related_to",
      "to": "insight-governed-memory",
      "rationale": "bounded evidence-backed explanation",
      "evidence_refs": ["excerpt-1"]
    }]
  }]
}
```

Lens results are `matched`, `not_matched`, or `unknown`; dispositions are `promote`, `hold`, `monitor`, `archive`, or `clarify`; semantic roles are `external_entity`, `evidence_backed_finding`, `unresolved_tension`, `reference_resource`, `action`, or `unknown`. `semantic_assessment` is the stable source-meaning projection: what the source is or means before relevance, disposition, constellation, or destination mapping is applied.

Validation rules:

- exactly one source judgment per canonical URL, including every admitted depth-1 URL, and exactly one result per configured lens;
- exactly one semantic assessment per canonical URL; complete assessments require public evidence, while incomplete assessments use role `unknown`, explicit missingness, and no invented summary;
- every rationale is non-empty and at most 1,000 code points;
- every evidence ref resolves to that source's public enrichment artifact;
- `unknown` requires non-empty missingness;
- each source has exactly one disposition;
- semantic assessment and any evidence-backed semantic nodes are independent of disposition; only `promote` makes nodes eligible for destination mapping;
- incomplete, inaccessible, or unevidenced sources cannot be promoted;
- zero to three semantic nodes per source in the live slice;
- node IDs are unique within the source and stable under judgment replay;
- every node has a public evidence ref and confidence in `[0,1]`; lens refs may be empty, but every supplied lens ref must resolve to the configured profile;
- every edge endpoint resolves within the same source constellation and every edge is evidence-backed;
- duplicate URL occurrences share one canonical judgment; duplicate source records inherit it and emit no additional nodes;
- lens result, semantic role, disposition, and destination mapping remain independent fields.

Changing only the lens profile and corresponding lens results may change relevance, disposition, and therefore destination output, but must not rewrite public source metadata, semantic assessment, or evidence-backed semantic-node roles. The synthetic proof exercises this invariant.

## Routing outputs

`mindline slack route` writes:

- `source-graph.json` (`strategic-source-graph/v0.1`);
- `route-decisions.json` (`strategic-route-decisions/v0.1`), containing validated semantic assessments, lens results, dispositions, nodes, edges, and local evidence refs;
- `route-summary.json` (`mindline-strategic-routing-summary/v0.1`);
- `review-packet.md`, a deterministic human-readable projection.

The summary contains counts for input records, occurrences, canonical primary URLs, admitted depth-1 URLs, all canonical sources, every enrichment state, every lens result, every disposition, node roles, edge types, duplicates, validation failures, local-private handling findings, outbound privacy findings, and `operator_judged=true`. Lens completeness is derived as `canonical source count × configured lens count`; it is never a primary-URL-only constant. The summary also contains the lens profile fingerprint and preserves the fixture projection's exact sample status, including `private_curated_sample`:

- intended user and workspace assumptions;
- source/input and output surfaces;
- provider/judgment assumptions;
- privacy boundary;
- curated/private/sample status;
- held-out/generalization status;
- named thresholds and guardrails.

The generic summary describes normalized sources and selected destination-adapter outputs only. Source- or destination-specific names belong in adapter artifacts and must not appear in a routing result compiled from an unrelated source graph.

## Product Brain destination profile

### `productbrain-delivery-profile/v0.1`

```json
{
  "schema_version": "productbrain-delivery-profile/v0.1",
  "profile_id": "external-pb-delete-later",
  "workspace": {"expected_id": "...", "expected_slug": "delete-later"},
  "transport": {"kind": "aki", "base_url": "https://gateway.productbrain.io", "api_path": "/api/aki"},
  "credential": {"provider": "environment", "name": "MINDLINE_PRODUCT_BRAIN_API_KEY", "expected_key_id": "non-secret key id from resolveWorkspace"},
  "role_mappings": {
    "external_entity": {"collection_slug": "landscape", "id_prefix": "LAND"},
    "evidence_backed_finding": {"collection_slug": "insights", "id_prefix": "INS"},
    "unresolved_tension": {"collection_slug": "tensions", "id_prefix": "TEN"}
  },
  "relation_mappings": {"related_to": "related_to"},
  "draft_only": true,
  "review_policy": {
    "credential_lifecycle": "retire_after_review",
    "private_runtime_lifecycle": "cleanup_after_review"
  }
}
```

The checked-in live profile must not contain a credential value. The production AKI transport accepts only the compile-time trusted origin `https://gateway.productbrain.io` with no userinfo, query, fragment, non-default port, redirect, or host variation; profile data alone cannot extend the credential audience. Future official REST origins require an intentional code/release trust change, while tests may inject a fake trusted origin with a fake secret. Origin validation happens before the secret provider is read or any request is made.

Runtime workspace resolution must match expected workspace ID and slug, governance capability must permit draft creation, key scope must be read/write, and returned `keyId` must exactly match `credential.expected_key_id`. The key ID is non-secret and may appear in the local delivery profile/history, but the key value may not. Any mismatch blocks all mutation. A key for the correct workspace with a different key ID is rejected.

The profile owns destination mapping. This slice accepts exactly the three declared role mappings and the `related_to` relation mapping. `reference_resource`, `action`, `unknown`, additional mappings, and unsupported relation types remain held with explicit `unsupported_destination_mapping`; they never fall through to another collection or extend the live write surface through configuration alone.

The optional fingerprint-bound `review_policy` owns destination-run lifecycle actions. `credential_lifecycle` is exactly `persistent` or `retire_after_review`; `private_runtime_lifecycle` is exactly `retain` or `cleanup_after_review`. A compiled Product Brain review context derives its entry/relation counts from the actual outbox operations, names Product Brain because this is destination-adapter output, and derives credential and runtime actions independently for all four valid policy combinations. It claims temporary-key retirement or runtime cleanup only when the profile explicitly selects those states; cleanup may depend on key retirement only when both are selected.

The shared outbox validator used by runtime, integrated review, readback, and executable proof requires the ordered pending-action list to equal the current operation-count and profile-policy derivation. The only exception is the exact three-string action set already fingerprint-bound into the immutable delivered v0.1 outbox with fingerprint `4dabd8cc6b0c67f3b19173b0a80c425c2ee4ec3ab8b1fe80ea16959baf1f5020`: review the three Product Brain drafts and routing judgments; retire the temporary Product Brain key after review; confirm owner-validated private runtime cleanup after key retirement. Both the exact fingerprint and exact ordered strings must match. No other nil-policy outbox—including a new outbox with the same counts—or altered, partial, reordered, or newly invented legacy action set is valid. The read-only integrated review path preserves that exact immutable set without mutating authority.

For the live schemas:

- `external_entity` maps to Landscape with required `description` plus supported `url`, `category`, `relationshipToPb`, `icpOverlap`, `keyDifferentiator`, and `whatWeLearn` only when present and schema-valid;
- `evidence_backed_finding` maps to Insights with required `description`, public `source`, and `evidenceStrength`;
- `unresolved_tension` maps to Tensions with required `description` and only supported optional type/severity/priority/affected-area/status fields;
- all writes set `forceDraft=true`; a readback status other than `draft` is a mismatch.

## Public-only outbox

`mindline product-brain outbox` writes immutable `outbox.json` (`productbrain-outbox/v0.1`), `outbox-summary.json`, and `review-packet.md`.

One structural validator is authoritative anywhere an outbox is loaded, preflighted, delivered, reviewed, or proved. It verifies the top fingerprint, every payload fingerprint, unique operation and destination identities, allowed operation kinds, exactly one kind-matching payload, dependency closure, relation endpoint dependencies, deterministic relation identity/type, draft/actor requirements, embedded profile/review authority, and the complete outbound privacy scan. A self-consistent but malformed outbox cannot reach network code.

Entry operations contain operation ID, collection slug, deterministic entry ID, name, canonical expected data, public `sourceRef`, bounded public `sourceExcerpt`, expected `createdBy="mindline:agent-operator"`, dependency IDs, and payload fingerprint. `createdBy` participates in the fingerprint and exact readback. Relation operations contain operation ID, deterministic relation identity, from/to entry IDs, destination relation type, and explicit allowlisted metadata: public evidence refs, lens refs, sanitized rationale, `initiator_type="agent_operator"`, `judgment_method="operator_agent_review"`, and verified non-secret `credential_key_id`; they also contain `if_missing=true`, dependency IDs, and payload fingerprint. Every relation metadata field participates in both the operation fingerprint and expected readback.

Entry ID algorithm:

1. form identity string `mindline/v0.1|<canonical-public-url>|<semantic-node-id>|<collection-slug>`;
2. SHA-256 the UTF-8 identity string;
3. interpret the first 10 bytes as an unsigned big-endian integer and encode in base 10 without leading zeroes;
4. prefix with the destination collection prefix, yielding e.g. `LAND-123456789...`.

This satisfies Product Brain's `PREFIX-<digits>` convention, is independent of Slack identity and run/output paths, and has an 80-bit collision space. During compile, duplicate derived IDs with different canonical payloads block the entire outbox.

Operation IDs and relation identity use full SHA-256 hex over their canonical public identities. Entry names are semantic names, not identity. A destination name conflict under a different entry ID fails closed during delivery; Mindline does not silently rename knowledge.

Allowlisted outbound values:

- public `https` URLs (or `http` only for explicitly allowed local test servers);
- public title, author, publication date, repository metadata, and bounded excerpt;
- semantic name, description, supported adapter attributes, evidence strength, and sanitized rationale;
- lens IDs, public evidence IDs, schema/profile versions, and SHA-256 fingerprints;
- bounded attribution values `createdBy="mindline:agent-operator"`, `initiator_type="agent_operator"`, `judgment_method="operator_agent_review"`, and the verified non-secret credential key ID.

Always forbidden:

- Slack channel, conversation, user, author, message, thread, timestamp, permalink, raw text, file, and workspace fields or values;
- private URLs, signed URLs, localhost or private-address URLs in a live outbox;
- filesystem paths, environment values, cookies, authorization/header material, API keys, tokens, and session identifiers;
- raw HTTP request/response bodies or errors.

Privacy enforcement is stage-specific. Local routing artifacts may retain authorized private provenance needed for accounting and review, and are scanned against the local-private policy for unexpected secrets or unsafe expansion—not against the public-only field allowlist. Outbox compilation selects a public projection from those artifacts and then applies the strict outbound policy to every selected value, the mapped entry/relation operation structs, and the final serialized outbox. The strict policy uses forbidden field-name checks, secret/token patterns, Slack ID/permalink patterns, local-path patterns, URL host/address validation, and exact known-secret comparison supplied in memory. Any strict-policy finding blocks the whole outbox. Findings report category and JSON path only, never the unsafe value.

## Transport interface

```go
type ProductBrainTransport interface {
    ResolveWorkspace(ctx context.Context) (WorkspaceCapability, error)
    GetCollectionFields(ctx context.Context, collectionSlug string) ([]CollectionField, error)
    GetEntry(ctx context.Context, entryID string) (EntryReadback, error)
    SearchEntries(ctx context.Context, query string, collectionSlug string) ([]EntrySearchResult, error)
    CreateEntry(ctx context.Context, request CreateEntryRequest) (CreateEntryResult, error)
    ListEntryRelations(ctx context.Context, entryID string) ([]RelationReadback, error)
    CreateEntryRelation(ctx context.Context, request CreateRelationRequest) (CreateRelationResult, error)
}

type RuntimeSecretScanner interface {
    RuntimeSecretFindings(value any) []PrivacyFinding
}
```

The AKI implementation sends `POST /api/aki` with `{fn,args}` and bearer authentication. Supported function names for this slice are `resolveWorkspace`, `chain.getCollectionFields`, `chain.getEntry`, `chain.searchEntries`, `chain.createEntry`, `chain.listEntryRelations`, and `chain.createEntryRelation` using the currently verified live argument contracts. `resolveWorkspace` is called with `{}` and its `_id`, `slug`, `governanceMode`, `keyScope`, and `keyId` response is normalized into `WorkspaceCapability`. `chain.getCollectionFields` is called read-only for each mapped collection; its normalized, deterministically sorted field names, types, required flags, and options are fingerprinted into the preflight contract. Unknown live field types, missing required mappings, or option mismatches fail closed. `chain.searchEntries` receives exact intended name text plus collection slug; the adapter locally filters exact case-sensitive name and collection matches. Relation creation sends truthful agent/operator metadata and `ifMissing=true`; it omits Product Brain's `proposedBy="user"` marker because no human has approved these relations before review. Transport construction is selected once at the CLI boundary from the profile's transport kind. Both preflight and delivery depend on the injected transport port rather than reconstructing AKI internally. Origin validation happens before the secret provider is read, and operation code never reads global environment directly.

Entry creation sends `createdBy="mindline:agent-operator"`. Relation metadata and sealed delivery runs record safe `initiator_type="agent_operator"`, `judgment_method="operator_agent_review"`, and the verified non-secret key ID. They never claim a human user or approval. A future `initiator_type="human"`/`proposedBy="user"` path requires explicit, durable human-initiation or approval evidence; it is not part of this slice.

HTTP defaults: TLS only for live endpoints, 15-second request timeout, bounded response body, no automatic mutation retry, no redirect across hosts, and a redacting client/logger. Only read calls may use bounded retry on transient network failure. Create calls never retry until readback reconciliation establishes absence.

Transport errors are normalized before leaving the adapter. The single closed safe-delivery category set, shared by transport errors, delivery operation diagnostics, sealed-run validation, and proof validation, is: `credential_missing`, `untrusted_product_brain_origin`, `unauthorized`, `forbidden`, `workspace_mismatch`, `capability_missing`, `collection_contract_mismatch`, `not_found`, `already_exists`, `validation_failed`, `rate_limited`, `transient`, `remote_failure`, `ambiguous_outcome`, `destination_name_conflict`, `readback_mismatch`, `dependency_not_acknowledged`, `outbox_state_mismatch`, `unsafe_outbound_value`, and `local_state_failure`. Any network, response, or remote-status failure on a mutation call that may have committed normalizes to `ambiguous_outcome`; malformed or oversized read responses normalize to `remote_failure`; unknown adapter categories normalize to `remote_failure`; arbitrary non-transport error text normalizes to `local_state_failure`. The former ad hoc values `transport_failure` and `invalid_response` are invalid. Safe diagnostics contain operation ID, function name, category, HTTP status when safe, and retryability—not headers, bodies, URLs with query strings, credentials, filesystem paths, unsafe values, or remote stack traces.

### Read-only preflight

Preflight validates the production trusted origin before reading the secret, rescans the exact outbox through the transport's runtime-secret scanner, calls only `resolveWorkspace` and `chain.getCollectionFields`, and compares expected origin, workspace ID/slug, governance mode, read/write scope, key ID, and live collection-field contracts. Every live field descriptor must have a unique non-empty key, a recognized type (`select`, `string`, `text`, `date`, `number`, `boolean`, or destination person ID), and deterministic type-valid options even when the outbox does not emit that optional field; an actually unknown type fails closed. The artifact contains its own schema version and fingerprint, outbox/profile fingerprints, safe checked identifiers, deterministic per-collection schema fingerprints, one exact unique base gate plus one gate per exact collection contract, `mutation_calls=0`, and overall verdict. Missing, duplicate, fabricated, or extra gates fail closed. It contains no secret, request/response body, header, or private Slack value.

Delivery rejects missing, failed, malformed, fingerprint-mismatched, or foreign-workspace preflight evidence. After accepting it and acquiring the exclusive lock, delivery copies the exact verified public-safe artifact into `preflight-snapshots/<preflight-fingerprint>.json` using no-replace semantics; an existing exact snapshot is reused and any content mismatch blocks. Every active journal and sealed delivery run records the preflight fingerprint, snapshot ref, verdict, and `mutation_calls=0`. History, summary, integrated packet, and eval readback expose that lineage and proof revalidates the referenced snapshot's schema, fingerprint, outbox/profile binding, gate results, and zero-mutation counter.

Delivery then performs the same live origin/secret scan/workspace/scope/key checks again inside the exclusive invocation lock. Preflight is evidence, not a lease or cached authority.

## Delivery state and reconciliation

`delivery-state.json` uses `productbrain-delivery-state/v0.1` but is a rebuildable projection, not an independent authority. The current `.delivery-active.json` journal is authoritative while an invocation is open; once sealed, the ordered immutable run records are authoritative. `delivery-summary.json`, `delivery-history.json`, state, and the integrated packet are regenerated from those authorities. Every authority and projection is bound to the immutable outbox fingerprint and expected workspace/profile identities. A different outbox or profile cannot reuse a delivery directory. Completed, interrupted, and failed runs remain explicit. A failed external-precondition run is acceptable lineage only when its valid read-only preflight remains sealed and it records zero mutation counters and zero per-operation mutation observations; it never disappears from review or proof.

Every invocation also has a durable journal and immutable history record:

- before reading or mutating delivery state, journals, history, or sequence numbers, the command acquires `.delivery.lock` with exclusive-create semantics and holds it for the entire invocation; a second live invocation fails `delivery_locked` before network activity;
- the lock records only safe host, PID, invocation ID, and start time. Automatic stale-lock recovery is allowed only on the same host when an OS liveness probe proves the recorded process does not exist; an unreadable, foreign-host, permission-denied, or inconclusive lock fails closed and requires explicit operator remediation;
- `.delivery-active.json` is the atomically updated journal for the current invocation, bound to outbox, workspace, and profile fingerprints and a monotonic sequence number;
- on clean exit it is atomically sealed as `delivery-runs/<six-digit-sequence>-<invocation-id>.json` using `productbrain-delivery-run/v0.1` and is never overwritten; sealing uses a no-replace primitive and any path/sequence collision fails closed;
- after safely recovering a stale lock, the next invocation seals the surviving journal first with outcome `interrupted`, preserving its recorded transitions and observed mutations before allocating the next sequence;
- `delivery-history.json` is a deterministic projection over all sealed run records and can be rebuilt by scanning them; missing, duplicate, out-of-order, or fingerprint-mismatched records block proof;
- each sealed record contains its immutable preflight snapshot ref/fingerprint, safe start/end times, start/end operation states, attempted operations, mutations observed during that invocation, readback acknowledgements, safe failure categories, and completion outcome—never raw remote responses or credentials.

Sequence allocation occurs only while the exclusive lock is held and uses `max(sealed sequence, active sequence)+1`. Lock release happens only after state, sealed-run, history, summary, and packet writes complete. A clean failure still seals its run before releasing the lock.

Authoritative write order is journal-first:

1. before a network action, persist the intended operation transition and attempt number to the active journal;
2. perform at most the one authorized network action;
3. persist the normalized result, ambiguity, or observed mutation/readback to the active journal before updating any projection;
4. rebuild `delivery-state.json` from sealed history plus the active journal;
5. on invocation end, journal the final outcome, seal the journal with no-replace semantics, then rebuild history/state/summary/packet from sealed records.

If a crash occurs between any steps, the next lock holder trusts the surviving active journal, not a newer-looking projection. It validates fingerprints and sequence, seals that journal as interrupted, rebuilds projections, and starts recovery in a new invocation. A journal in `sending` means remote mutation outcome is ambiguous; recovery must reconcile remote readback before any retry. If a projection disagrees with its authoritative journal/history, it is deterministically rebuilt; if an authority record disagrees with another authority record or its bound fingerprints, delivery and proof block. Tests inject a crash after every journal, network, projection, and sealing boundary and prove idempotent recovery without evidence loss or duplicate mutation.

Per-operation states are `pending`, `reconciling`, `sending`, `acknowledged`, or `blocked`. Each transition is atomically persisted before the next network action. State stores safe attempts, last safe category, readback fingerprint, acknowledgement time, and whether a mutation was observed during this invocation. It stores no raw remote response.

Entry algorithm:

1. resolve and verify workspace capability before processing any operation;
2. set `reconciling`; call `GetEntry` by deterministic entry ID;
3. if found, compare collection, entry ID, name, draft status, canonical data subset, public source ref, public excerpt, and exact `createdBy="mindline:agent-operator"`; acknowledge on exact match, otherwise block `readback_mismatch` and report actor attribution as unproven;
4. if absent, search the intended name within the intended collection and locally require exact case-sensitive comparison; any exact-name result with a different entry ID blocks `destination_name_conflict`;
5. if ID and name are absent, persist `sending`, call `CreateEntry` once. Product Brain's verified live `chain.createEntry` also enforces exact same-collection name uniqueness server-side; a duplicate-name rejection is normalized without echoing the remote body, followed by search/readback, and blocks rather than renaming or retrying;
6. after a clean response, duplicate response, timeout, disconnect, or any outcome that could have committed remotely, persist `reconciling` and call `GetEntry`;
7. acknowledge only after exact readback; if still absent after bounded read reconciliation, block with `ambiguous_outcome` and require rerun; rerun starts at step 2.

System-managed timestamps, document IDs, warnings, and server normalization metadata are ignored unless they alter an expected canonical field. Unexpected extra user-data fields cause mismatch; explicitly enumerated server-owned fields do not.

Relation algorithm:

1. wait until both endpoint entries are acknowledged;
2. persist `reconciling`; list relations for the source endpoint and match exact `(from entry ID, mapped type, to entry ID)` plus the expected public evidence refs, lens refs, and sanitized rationale metadata;
3. scan the complete relation result; if exactly one identity-and-metadata match exists and no conflicting identity exists, acknowledge; duplicate exact matches, exact-plus-conflicting matches, missing/additional/different user metadata, or other repeated unexpected matches block;
4. if absent, persist `sending`, call create with `ifMissing=true` once;
5. after every clean or ambiguous result, list and reconcile again;
6. acknowledge only on exact match.

The summary reports the ordered immutable run refs, completed/interrupted/failed run counts, cumulative mutation/acknowledgement history across recovery, latest-run state, `entries_created_this_run`, `relations_created_this_run`, `entries_acknowledged`, `relations_acknowledged`, `mismatches`, `blocked`, and replay status. A replay run passes only when all operations reconcile before mutation and both created-this-run counters are zero. Regenerating the summary or packet never deletes or rewrites a sealed run.

### Integrated final review packet

Every delivery invocation regenerates `mindline-review-packet.md` as the single private-local review surface. The standalone review command uses a strictly read-only loader: it never repairs projections, and it validates the exact sealed run directory, exact referenced preflight snapshot set, every schema/fingerprint/binding/gate, and zero-mutation preflight before rendering. It joins the immutable routing and outbox fingerprints to the full ordered delivery-run history and current state and must not be treated as PR-safe or Chain-safe evidence. Its first section has exactly one row for each of the 12 original Slack captures in original order, with:

- local capture reference and public canonical URL;
- duplicate/canonical relationship;
- enrichment state, public evidence links, and missingness;
- stable semantic assessment role and summary;
- both lens results, rationales, and confidence;
- disposition and rationale;
- promoted semantic nodes/edges, or explicit reason none were materialized;
- Product Brain draft entry IDs, relation IDs/types, and acknowledgement state when applicable;
- verified entry/relation initiator attribution and non-secret credential key ID, or an explicit unproven/mismatch state;
- replay outcome and any pending manual action.

A separate section covers admitted depth-1 sources so follow-up context is visible without duplicating the original-capture ledger. A delivery-history section lists each invocation in order, its immutable read-only preflight fingerprint/ref and zero-mutation verdict, interrupted or partial runs, mutations observed, acknowledgements, and the later zero-mutation replay. The packet ends with totals, privacy posture, claim limits, and a checklist compiled from the exact Product Brain operation counts plus the profile's explicit credential/runtime lifecycle policy. A golden-packet test asserts every original capture is represented exactly once and can be traced through meaning, relevance, disposition, destination outcome, and replay without consulting another artifact.

## First live fixture and expected decisions

The private input fixture is an explicit curated set, oldest to newest:

1. Harrison Chase LinkedIn self-improving harness, 2026-06-23 23:36:06;
2. exact same canonical URL duplicate, 2026-06-23 23:38:00;
3. Insify business-insurance page, 2026-06-26 00:15:16;
4. ontology-imperative Substack, 2026-07-08 19:03:13;
5. Latent Force code-change graphs, 2026-07-09 18:32:36;
6. `braedonsaunders/codeflow`, 2026-07-10 12:37:15;
7. LinkedIn ontology-guided AI-native knowledge-graph article, 2026-07-12 15:42:36;
8. LinkedIn graph article, 2026-07-12 22:05:30;
9. J. Gothelf AI-roadmap Substack, 2026-07-12 22:09:01;
10. accountability Substack, 2026-07-13 10:28:06;
11. `glare.zurb.com/skills`, 2026-07-14 09:53:02;
12. `EXXETA/exxperts`, 2026-07-14 09:56:25.

This selection is not a contiguous backlog watermark. The insurance page is an evidenced negative control. Inaccessible LinkedIn sources remain incomplete and unpromoted. All decisions are explicit in the judgment manifest and reviewable; the implementation does not hard-code expected decisions by URL.

The single live promotion is `https://github.com/EXXETA/exxperts` with exactly:

- Landscape: `Exxperts`, an external entity/tool;
- Insight: governed persistent agent memory is becoming a user-facing product capability;
- Tension: useful persistent agent context conflicts with silent memory writes and approval burden;
- relation: Exxperts `related_to` the Insight;
- relation: Exxperts `related_to` the Tension.

Descriptions and optional fields must be grounded only in the captured public README evidence. All three entries are drafts.

Expected live counts:

- 12 source records and 12 primary URL occurrences;
- 11 primary canonical URLs because of one exact duplicate pair;
- exactly one accepted depth-1 related URL and therefore exactly 12 total canonical source nodes in this fixture;
- 24 lens results, two for each canonical source including the depth-1 source;
- exactly 3 promoted semantic nodes and 2 semantic edges;
- exactly 5 outbox operations;
- first delivery: 5/5 exact acknowledgements;
- replay: 5/5 reconciled acknowledgements, zero entry creates, zero relation creates;
- zero outbound privacy/secret findings.

## Synthetic product-fit fixture

Before private delivery, tests run the generic compiler with:

- a non-Slack `bookmark` or `document` source adapter fixture;
- an alternate-user lens profile with different IDs and content;
- the same public source evidence and semantic role judgment;
- a second profile variant that changes only lens relevance.

Acceptance:

- the entire generic routing result contains no Slack, Product Brain, or Tolaria vocabulary, and the routing package has no source-adapter import;
- alternate lens IDs flow through without code changes;
- changing only lenses changes lens results/disposition and destination operation count as declared, while public enrichment, semantic assessment, and destination-neutral semantic-node roles remain byte-equivalent;
- routing artifacts contain no Product Brain fields;
- Product Brain mapping appears only after the destination adapter runs.
- newly compiled Product Brain actions use exact operation counts; credential/runtime actions are independently correct for all four valid policy combinations; temporary-key retirement and private-root cleanup appear only from an explicit signed review policy;
- a recomputed outbox/proof lineage with an unauthorized pending-action change or a new outbox reusing the delivered legacy strings fails, while only the exact fingerprint-bound immutable delivered v0.1 action set remains reviewable without mutation.

This is a contract portability test, not generalization evidence.

## Eval and proof contract

`mindline eval readback` recognizes the routing summary, full immutable outbox, outbox summary, read-only preflight artifact, delivery summary/history, every referenced sealed delivery run, and every referenced preflight snapshot and reports:

- evidence present and artifact fingerprints;
- exact source/URL/lens/disposition coverage;
- privacy and secret findings;
- expected versus acknowledged operations;
- first-run and replay mutation counts;
- draft-only status;
- operator-judged, private, curated, sample-bound status;
- blocked claims and next product-general improvement target.

Add proof claim `delivery` without weakening the existing global `safety` claim:

```sh
mindline eval proof-gate <delivery-dir> --claim delivery
```

`delivery` treats the sealed run records and referenced preflight snapshots as authority rather than trusting embedded delivery projections. It rejects cached readback-summary-only input, missing or extra authority files, symlinked or permission-widened authority, unsupported run schemas, invalid preflight gate sets, and fingerprint mismatches. It requires exact equality between embedded and sealed run records and binds routing fingerprint to full outbox, outbox/profile fingerprints to every preflight/run/summary, and the unique outbox operation ID/kind set to exact entry destination IDs, relation identities/types, remote IDs, and canonical readback fingerprints. It verifies `required_lens_result_count == canonical_source_count × lens_count`, preserves the exact `private_curated_sample` boundary, and does not take maximum values across unrelated artifacts. Interrupted or fail-closed zero-mutation runs may remain in authority when cumulative mutations still equal the bounded outbox exactly and a later completed replay is zero-mutation. `delivery` passes only when:

- source and URL accounting are complete;
- every canonical source, including each admitted depth-1 source, has a lens result for every configured lens;
- no incomplete or unevidenced source was promoted;
- matching read-only preflight passed every gate with zero mutation calls and delivery repeated the external preconditions;
- outbound privacy and credential findings are zero;
- every outbox operation is acknowledged by exact readback;
- all destination entries are draft;
- all three destination entries read back `createdBy="mindline:agent-operator"` and both relation metadata records read back the expected agent/operator attribution values and verified non-secret key ID;
- readback mismatches and blocked operations are zero;
- an intact ordered immutable run sequence for the same outbox/workspace/profile fingerprints records any first-run or recovery mutations and a later completed run with zero new entries and relations after all operations were acknowledged;
- the artifact declares `operator_judged=true`, `held_out=false`, `generalizable=false`, and no autonomy claim.

The global `safety` guardrails remain truthful: a successful bounded delivery reports five destination/Product Brain writes and therefore does not pass the zero-write safety claim. The dedicated `delivery` claim permits only the exactly bounded, draft-only, acknowledged write set above. Generated readback/Chain-draft language is work-package-neutral and must not name another work package.

The proof gate validates artifacts and named thresholds. Command exit `0` outside this gate is not outcome proof.

## Failure behavior

All validation and privacy failures occur before mutation and return non-zero. Delivery stops at the first blocked operation; dependent operations remain pending. A later rerun reconciles acknowledged remote state before continuing. Partial remote success is never reported as full success.

Stable safe codes include:

- routing: `missing_source_accounting`, `missing_url_accounting`, `invalid_link_depth`, `unresolved_evidence_ref`, `missing_lens_result`, `invalid_lens_result`, `invalid_disposition`, `incomplete_source_promoted`, `constellation_limit_exceeded`, `invalid_semantic_edge`;
- mapping/privacy: `unsupported_destination_mapping`, `destination_schema_mismatch`, `unsafe_outbound_field`, `unsafe_outbound_value`, `entry_identity_collision`;
- delivery operation diagnostics: only members of the single closed safe-delivery category set defined in the Transport interface section. A blocked operation with a missing or non-member category invalidates runtime history and executable proof. Validation/storage failures that occur before any operation attempt return a stable validation code but are never copied into `safe_category`.

Errors never echo source content, unsafe values, the credential, authorization headers, or raw remote errors.

## Test matrix

Unit and contract tests must cover:

- URL canonicalization, tracking removal, duplicates, occurrence preservation, one-hop links, and depth rejection;
- zero, one, and multiple configurable lenses; completeness and independence validation;
- missingness, evidence resolution, disposition rules, constellation bounds, and edge endpoints;
- semantic assessment preservation across `promote`, `hold`, `monitor`, `archive`, and `clarify` dispositions;
- non-Slack alternate-user fixture with no source/destination vocabulary in the complete generic routing result and no source-adapter dependency in routing;
- each supported Product Brain role/field mapping and fail-closed unsupported roles;
- structural outbox validation at load, preflight, delivery, review, and proof boundaries, including tampered payload fingerprints, duplicate IDs, wrong kinds/payload shapes, and dependency mismatch;
- deterministic numeric digest entry IDs, same-input stability, changed-node distinction, and forced collision handling;
- public allowlist plus Slack IDs/permalinks, private URLs, signed URLs, local paths, token patterns, exact runtime-secret scanning, and redacted errors;
- credential accepted from secret provider/environment only and absent from CLI flags, artifacts, logs, and diagnostics;
- the exact closed safe-delivery category set, network/mutation ambiguity normalization, malformed-response normalization, unknown-category normalization, and runtime/proof rejection of arbitrary sealed categories;
- trusted-origin rejection before secret-provider access; redirects and host/port/userinfo/query variations rejected; fake test origin injectable only with fake secret;
- workspace, key-scope, and expected-key-ID mismatch before mutation;
- read-only preflight uses the injected transport port, resolves workspace plus canonical live collection-field contracts with zero mutation calls, fails closed on unknown or duplicate live field descriptors and incomplete/duplicate gate sets, and never constructs AKI internally; delivery rejects missing/failed/stale-fingerprint evidence, preserves an immutable no-replace snapshot and per-run lineage, and repeats all external preconditions before mutation;
- truthful agent/operator entry/relation/run attribution, entry `createdBy` omission/rewriting mismatches, relation metadata omission/rewriting mismatches, and rejection of unproved human/user attribution;
- entry preflight match, exact-name/different-ID search conflict, server-side duplicate-name rejection, absence/create/readback, duplicate response, timeout-after-commit, timeout-before-commit, and mismatch;
- relation preflight full-result identity-and-metadata match, duplicate exact matches, exact-plus-conflicting matches, absence/create/readback, timeout-after-commit, `ifMissing`, metadata mismatch, and endpoint/type mismatch;
- journal-first authority, projection divergence rebuild, crash injection at every state/network/seal boundary, outbox binding, exclusive concurrent-invocation refusal, same-host proven-stale lock recovery, interrupted `sending` recovery through reconciliation, active-journal sealing, duplicate-sequence prevention, no-overwrite immutable ordered run retention, history reconstruction, multiple reruns, summary/packet regeneration without evidence loss, symlink rejection, and stale artifact handling;
- integrated final review packet completeness, original-order stability, exactly-once capture rows, depth-1 separation, and pending-action visibility;
- Product Brain review-action counts derived from current operations, credential/private-runtime actions independently derived for every valid signed-policy combination, unauthorized refingerprinted action rejection at runtime/proof, rejection of a one-entry/new outbox reusing the legacy strings, and exact fingerprint-bound immutable delivered-v0.1 review compatibility;
- readback and `delivery` proof pass/fail thresholds, including cached-summary refusal, missing/tampered/extra/symlinked sealed authority files, unsupported run schemas, routing-to-outbox-to-preflight-to-run binding, exact operation/destination/readback identity, complete source-by-lens matrix, preserved private curated sample status, recovery/failed-run lineage, and truthful non-zero global write guardrails;
- existing Mindline test suite.

The fake transport records attempted mutations and supports injected ambiguous outcomes. Tests assert that no create is retried without an intervening successful absence reconciliation.

## Acceptance gates

1. Signed Spec and Plan are durable and linked from WP-45 before live write.
2. `go test -count=1 ./...` passes.
3. Synthetic alternate-user/non-Slack contract proof passes.
4. Private route produces exact 12/12 source and URL occurrence accounting, 11 primary canonical URLs, exactly one admitted bounded follow-up source, and 24/24 required lens results across all 12 canonical sources.
5. Duplicate occurrences collapse without losing accounting.
6. LinkedIn incomplete sources do not fabricate or promote meaning.
7. Insurance negative control is `not_matched` for both lenses and not promoted.
8. Exxperts yields exactly three nodes and two relations from public evidence.
9. The public-only outbox contains exactly five operations and zero private/secret findings.
10. Read-only Product Brain preflight passes with zero mutation calls and exact trusted origin, workspace identity, read/write capability, expected non-secret key ID, outbox/profile fingerprints, and runtime-secret absence.
11. Delivery accepts and immutably snapshots the matching preflight artifact, binds its fingerprint/ref into every sealed run, repeats those checks before mutation, and preserves `agent_operator` attribution with no human-approval claim.
12. First delivery acknowledges all five operations by exact readback and leaves three draft entries; its immutable run record preserves observed mutations and acknowledgements.
13. A later immutable replay run for the exact outbox creates zero entries and zero relations and acknowledges all five operations; the ordered history retains both runs and any interrupted recovery records.
14. One integrated private-local review packet represents all 12 original captures exactly once and exposes their evidence, missingness, stable meaning/role, two lens results, disposition, promoted drafts/relations, acknowledgements, replay, and manual next actions.
15. Eval readback is inspectable and `proof-gate --claim delivery` passes.
16. Evidence and handoff explicitly block full-drain, autonomy, held-out, and generalization claims.
17. The temporary Product Brain key is retired by Randy after reviewing the pilot; the handoff marks retirement pending until confirmed.

## Non-outcomes

- no full Slack drain, daemon, watermark, or production source credential;
- no autonomous model router or accuracy claim;
- no LinkedIn browser/session automation;
- no destination other than the selected Product Brain draft workspace;
- no Product Brain REST assumption beyond the replaceable transport interface;
- no Tolaria output;
- no automatic Product Brain draft commit;
- no hard-coded first-user lens values or live fixture URLs in production routing logic.

## Review stop

Implementation may begin only after this exact Spec and its implementation Plan each receive two clean five-role reviewer passes, are persisted under `.productbrain/`, are linked from WP-45, and the handoff audit has no blocking failure. External delivery remains forbidden until tests and preflight proof pass.
