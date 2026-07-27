# WP-45 Shape: Context-Lens Slack Routing to Product Brain

Date: 2026-07-14
Phase: Shape
Version: draft-5

## Authority

- WP-45 and DEC-412 require complete Slack source accounting with selective, evidence-backed promotion into the external Product Brain product.
- DEC-413 defines context lenses as the relevance authority, independent from semantic role, disposition, and destination collection.
- STD-21 requires evidence, relevance, disposition, and destination mapping to remain separate projections.
- DOMAIN-1 and DEC-254 keep Mindline source- and destination-neutral; Product Brain is one destination adapter.
- PRI-1, BR-1, STD-12, STD-20, TEN-25, and TEN-26 require private-safe proof, honest capability reporting, acknowledged writes, and no unsupported autonomy claim.

## Sharp Problem

Randy's private Slack self-DM is a queue of links without usable context. Today Mindline can normalize, enrich, and produce review artifacts, but it cannot answer the user-level questions that make the capture valuable:

1. Is this relevant to something I currently care about?
2. What does the source actually say or represent?
3. Is it an external entity, insight, tension, or unresolved item?
4. What should be captured, how should related meanings be connected, and what must remain held?
5. Did the selected knowledge actually arrive in Product Brain once—and only once—without leaking private Slack provenance?

The missing product boundary is not Slack parsing or Tolaria output. It is the evidence-backed transition from private source capture to contextual meaning, semantic constellation, and acknowledged destination delivery.

## First Pilot Context

The pilot accepts a versioned context-lens profile containing two Randy-specific lenses:

- `building-product-brain`: external tools, competitors, repositories, methods, resources, architecture approaches, what they mean for Product Brain, and how they might be used.
- `ai-native-organizational-design`: team structures, roles, governance, coordination patterns, and tensions in an AI-dominant, transformational environment.

The product contract is not these two lens values. The product contract is that a workspace can supply zero or more versioned lenses and every source is resolved independently against each lens with evidence, rationale, confidence, and missingness.

## Selected Direction

Build one bounded, reviewable route-and-deliver path:

1. ingest the explicit 12-message representative Slack fixture oldest-to-newest;
2. account for every message and URL occurrence, including one exact duplicate pair, in a destination-neutral source/link graph;
3. type primary URLs and enrichment-supplied follow-up URLs, preserve parent/child edges and canonical identities, join allowlisted local enrichment artifacts, and preserve inaccessible LinkedIn items as incomplete;
4. validate an operator/agent judgment manifest that resolves every item against both lenses and emits exactly one disposition;
5. allow one source to yield a bounded semantic constellation of distinct external-entity, insight, and tension nodes plus evidence-backed edges;
6. compile only `promote` nodes through a Product Brain destination profile; hold anything without a semantically valid live collection;
7. produce a public-source-only outbox that contains no Slack provenance or private capture data;
8. deliver drafts and relations through a transport interface, initially backed by `/api/aki`, with future REST hidden behind the same interface;
9. reconcile every operation by exact readback and replay the same outbox with no additional entries or relations;
10. run the same routing contracts against a synthetic alternate-user profile, different lens values, and a non-Slack normalized source fixture before the private pilot;
11. stop with the live Product Brain drafts and local review/eval packet ready for Randy.

The first classifier is an explicit operator/agent judgment manifest, not a claimed autonomous semantic router. The manifest is a replaceable provider seam and must satisfy the same evidence contract a future model-backed classifier will satisfy.

## Challenge and Reconciliation

### Rejected: full Slack drain first

A full drain would magnify unknown routing, privacy, enrichment, and replay behavior before the destination path is proven. It would also make user review unbounded.

### Rejected: newest 12 and one Mindline strategy anchor

The newest 12 are overwhelmingly inaccessible LinkedIn links and do not contain the observed duplicate. STR-2 describes Mindline's product vision, not the two research contexts Randy wants the source queue resolved against.

### Rejected: route all relevant material to Landscape

Landscape accepts external entities. It does not accept arbitrary findings, tensions, or resources. Forcing all useful material into one collection would make Product Brain's schema contaminate core routing.

### Reconciled shape

Use a deterministic representative fixture, configurable context lenses, distinct semantic nodes, adapter-owned collection mapping, a public outbound projection, and a replay-safe transport. This is the smallest slice that proves actual destination value without baking Randy, Slack, or Product Brain into Mindline's core model.

## Core Contracts

### Source/link graph

Every adapter maps its native item into a generic normalized `source_record`; every discovered URL mention becomes a generic `url_occurrence` linked by `source_record_contains_url`. URL nodes have a canonical public identity, media/source kind, parent identity, discovery method, depth, enrichment state, and missingness. Edges preserve `source_record_contains_url`, `source_links_to`, and duplicate/canonical relationships without using destination concepts. A Slack message is an adapter-native input, not a core graph node type.

The first slice recognizes GitHub repository, LinkedIn post/article, YouTube video, article, PDF, and generic-web kinds. A source enrichment artifact may supply semantically relevant outbound or follow-up URLs together with the parent URL and discovery evidence; those URLs become distinct graph nodes and must be typed and accounted for. The pilot permits one follow-up hop beyond a primary Slack URL. It does not indiscriminately crawl every hyperlink on a page or recurse without a bound. An inaccessible or incomplete parent never fabricates children.

All raw adapter-native occurrences remain local. Public canonical URL identity drives deduplication, route evidence, and any outbound destination identity. In the private pilot, the exact duplicate Slack pair becomes two generic source records and two URL occurrences pointing to one canonical URL node. The synthetic non-Slack fixture must exercise the same generic node and edge vocabulary.

### Context-lens resolution

Every source records a result for every configured lens: `matched`, `not_matched`, or `unknown`, with evidence refs, a bounded explanation, confidence, and missingness. Lens results do not select a destination collection.

### Disposition

Every source receives exactly one source-level disposition: `promote`, `hold`, `monitor`, `archive`, or `clarify`. Only `promote` may reach destination mapping. Duplicate source items remain accounted for but point to one canonical source identity.

### Semantic constellation

A promoted source may emit multiple distinct semantic nodes. Each node has a destination-neutral role, claim/description, evidence refs, confidence, and lens refs. Edges have stable endpoint identities, an evidence-backed semantic relation, and rationale. The first live constellation is capped at three nodes.

### Product Brain adapter

Mapping follows the live destination profile:

- external entity -> `landscape` when its required fields are satisfied;
- evidence-backed finding -> `insights` when its evidence contract is satisfied;
- unresolved contradiction, risk, or pressure -> `tensions` when its live schema is satisfied;
- anything else -> held with an explicit mapping reason.

### Outbound privacy projection

The outbox may contain allowlisted public URLs, public source metadata, bounded public excerpts, sanitized semantic rationale, lens IDs, and non-sensitive fingerprints. It excludes Slack permalinks, conversation/user IDs, timestamps, raw messages, private files, local paths, credentials, and authorization material. The projection is rescanned after enrichment and mapping; one unsafe value blocks the whole operation.

### Replay-safe delivery

Entry IDs derive from public canonical source identity plus semantic-node identity, not Slack metadata. Expected canonical payload is persisted before send. Every clean or ambiguous create outcome is followed by exact `getEntry` reconciliation. Relations use deterministic `(from, type, to)` identity, `ifMissing:true`, and exact `listEntryRelations` reconciliation. Mismatch fails closed.

The Product Brain credential is accepted only from a process environment variable or secret-provider interface. It is never accepted as a CLI argument, persisted in config/outbox/artifacts, included in request or response diagnostics, or exposed through header logging. The supplied temporary key is retired after Randy reviews the pilot.

## First-Slice Outcome

Randy can review:

- all 12 source dispositions and both lens explanations;
- which sources were incomplete, duplicates, irrelevant, or promoted;
- one small semantic constellation with public evidence;
- the resulting draft entries and relations inside the external Product Brain product;
- operation acknowledgements and a replay report proving no additional writes.

This is user value because the selected Slack material becomes contextual, connected Product Brain knowledge rather than another local artifact queue.

## Proof Expectations

- exact 12/12 source accounting and 100% URL accounting;
- every primary and enrichment-supplied follow-up URL is typed and linked to its parent, with at least one bounded follow-up edge exercised;
- explicit result for both lenses on every canonical source;
- duplicate pair collapses to one canonical source identity without losing message accounting;
- inaccessible LinkedIn items remain held/clarify and never fabricate context;
- zero Slack/private/secret fields in every outbound operation;
- credential scanners and negative tests prove that the Product Brain key cannot enter CLI arguments, persisted artifacts, logs, errors, or diagnostics; the pilot handoff includes explicit temporary-key retirement;
- all promoted entry and relation operations acknowledged by exact readback;
- second execution creates zero additional entries and zero additional relations;
- destination drafts remain reviewable and uncommitted;
- the same normalized router passes a synthetic alternate-user fixture with different lens IDs/content and a non-Slack source; changing only the lens profile changes relevance without changing semantic role or introducing Product Brain fields;
- eval readback states that the proof is private, curated, sample-bound, operator-judged, and non-generalizable.

## Non-Outcomes

- no full Slack backlog drain or production watermark;
- no autonomous routing, no-human, held-out, or generalization claim;
- no LinkedIn browser automation or fabricated source context;
- no Product Brain REST implementation before the API exists;
- no hard-coded lens, Slack, Product Brain collection, or Randy taxonomy in core logic;
- no Tolaria output;
- no automatic commit of Product Brain drafts.

## Product-Model Fit

- source adapter owns Slack-native ingestion and private provenance;
- source enrichment owns URL typing, canonical link identity, bounded follow-up discovery, parent/child edges, and enrichment missingness;
- normalized routing owns context lenses, semantic nodes, semantic edges, evidence, missingness, and disposition;
- destination profile owns Product Brain collection/field/relation mapping;
- transport owns gateway capability, outbox state, reconciliation, and acknowledgements;
- eval/readback owns proof and claim limits.

This extends the existing candidate/enrichment/proposal patterns but introduces a new canonical context-lens and constellation contract because no existing core contract represents them without destination coupling.

## Stop Point

Shape authorizes a written Spec and Plan only after reviewer sign-off. It does not authorize destination writes until the signed Spec and Plan are captured on Chain and delivery authority is clean.
