# MINDLINE-WP11-SHAPE-V3: Document Structure and Capability Model Extraction

Date: 2026-05-21
Status: Shape for LOOP review
Stop mode: Plan Ready

## Problem

WP-10 proved that Mindline can decompose Markdown into safe, destination-neutral `document-segments/v0.1` artifacts. The real process/capability document test exposed the next blocker: flat line-level segments are not enough for structured process documents.

The representative real document produced 89 segments from one Markdown file, with most meaningful capability rows classified as generic `source_note` and no durable hierarchy linking parent capability rows, child capability rows, audience sections, and audience-specific capability rows. That is safe, but not useful enough for Product Brain, Tolaria, Notion, Obsidian, or any destination adapter to make high-quality decisions.

## Shape Direction

Create the next Mindline work package: **Document Structure and Capability Model Extraction**.

The work should extend WP-10 by adding a destination-neutral structure layer above raw `DocumentSegment` artifacts:

```text
Markdown source
  -> DocumentSegment artifacts
  -> DocumentStructure artifact
  -> typed structural nodes and relationships
  -> later review/apply/destination evaluation
```

The target is not Product Brain proposal generation. The target is a reusable local structure artifact that can represent process/capability documents well enough for downstream adapters to evaluate later.

## Product Model Fit

Eligibility path: **EXTEND**.

WP-11 extends the WP-10 `DocumentSegment` contract with a higher-level, destination-neutral `DocumentStructure` artifact. It does not replace WP-10 and does not make Product Brain the schema owner.

Product object:
- `DocumentStructure`: a local artifact that groups segments into typed nodes, section hierarchy, capability-like records, and relationships.

Adjacent canonical patterns:
- WP-10: `document-segments/v0.1` local decomposition contract.
- DOMAIN-1: Product Brain is a downstream consumer, not Mindline core.
- DEC-28: WP-10 delivery sign-off and safety boundary.
- INS-9: raw Markdown examples need decomposition before proposal generation.
- TYPE-capability and TYPE-requirement exist in the Chain as semantic concepts, but WP-11 must not hard-code Product Brain collections.

Why this is not bespoke:
- The representative process document is one example of a broader class: process docs, capability maps, strategy docs, operating models, and requirements matrices.
- The output contract should be reusable across multiple destinations and future source adapters.

## In Scope

- Define a `document-structure/v0.1` summary artifact and typed node artifact contract.
- Derive section hierarchy from Markdown headings and tables.
- Preserve parent-child relationships between headings, table rows, bullets, and derived capability-like nodes.
- Recognize stable structural node kinds without destination coupling, such as:
  - `section`
  - `table`
  - `table_row`
  - `capability`
  - `audience`
  - `workflow`
  - `requirement`
  - `unknown`
- Preserve source provenance by linking structure nodes back to one or more `DocumentSegment` IDs and source line ranges.
- Add review status and confidence at the structure-node level.
- Emit local artifacts only under explicit `--out`.
- Use sanitized fixtures derived from the observed process/capability document shape, not raw private content.
- Keep writer-boundary safety and output containment at least as strict as WP-10.

## Out of Scope

- No Product Brain proposals or live Product Brain writes.
- No Tolaria, Notion, Obsidian, Slack, browser, network, auth, provider, daemon, UI, or LLM integration.
- No raw private document content committed to the repo.
- No semantic certainty claims beyond deterministic structural extraction.
- No review/apply workflow yet. Review/apply remains a follow-up package.
- No destination-specific mapping from `DocumentStructure` to Product Brain capabilities, requirements, or work packages.

## Expected Outcome

After WP-11, a process/capability document should produce a structure artifact that is recognizable as a capability/process model instead of a flat list of line fragments.

Expected shape for the representative process document class:

```text
DocumentStructure
- Platform capability map
  - Parent capability A
    - capability nodes
  - Parent capability B
    - capability nodes
    - audience-specific process nodes
- Product capability map
  - Audience group A
    - audience capability nodes
  - Audience group B
    - audience capability nodes
  - Audience group C
    - audience capability nodes
```

## Proof Expectations

- Sanitized fixtures cover:
  - process/capability table with parent capability rows
  - audience-specific capability list
  - mixed headings, bullets, and prose
  - ambiguous rows that should remain `unknown` or `needs_review`
- Golden expected outputs compare structure summary, structure nodes, and previews.
- Structure output links back to WP-10 segment IDs and source provenance.
- Output contains no destination hints such as Product Brain, Notion, Obsidian, or Tolaria.
- Unsafe marker and private provenance tests remain fail-closed.
- `go test -count=1 ./...` passes.

## Follow-Up Work

WP-12 should likely become the review/apply contract for `DocumentStructure` and `DocumentSegment` artifacts. Destination adapters should come after that, not before it.
