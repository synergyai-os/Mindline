# WP-10 Markdown Document Decomposition and Destination-Neutral Proposal Evaluation Shape

Shape version: `MINDLINE-WP10-SHAPE-V2`
Date: 2026-05-21
Status: Draft for LOOP Shape sign-off.

## Product Brain Authority

- Product: `PROD-1` - Mindline.
- Architecture boundary: `DOMAIN-1` - Product Brain is a destination/authority consumer, not the Mindline product.
- Prior delivery: `WP-8` - local run ledger and review queue.
- Prior delivery: `WP-9` - Product Brain workspace profile and proposal adapter.
- Evaluation evidence: `INS-9` - raw Markdown examples need decomposition before Product Brain proposal generation.
- Method boundary: `DEC-15` - Mindline core is method-neutral; BASB/PARA/CODE is a profile, not core architecture.
- Destination boundary: `WP-5`, `DEC-12`, `DEC-13` - destinations are adapters with dry-run/proposal contracts.
- Safety and containment standards: `STD-12`, `STD-13`, `STD-14`, `STD-15`, `STD-16`.

## Problem

Mindline can safely generate Product Brain proposal artifacts from already-curated review queue items, but it cannot yet understand raw Markdown documents.

The six real examples in `temp/` are not single knowledge objects. They are mixed documents: meeting transcripts, Slack-like threads, Notion strategy pages, capability lists, will/won't-do tables, project risks, actions, decisions, and open questions. When each file is wrapped as one review item, the current WP-9 path either skips the whole file or creates one broad proposal. That is mechanically safe but semantically wrong.

The architecture risk is larger than Product Brain quality. If the next slice fixes this only for Product Brain collections, Mindline will drift into a Product Brain-specific parser. That would violate `DOMAIN-1`, `DEC-15`, and the destination adapter model. Product Brain, Notion, Obsidian, Tolaria, local folders, and future APIs must all consume a shared intermediate representation rather than each owning its own document understanding.

## User Job

A user can drop a messy Markdown export or transcript into Mindline and get a reviewable set of smaller, well-typed knowledge candidates with provenance, confidence, and destination-neutral meaning before any destination-specific proposal is generated.

## Selected Direction

Create a destination-neutral Markdown/document decomposition layer that runs before destination proposal adapters.

The layer reads Markdown source files and emits `DocumentCandidate` / `DocumentSegment` review artifacts. Segments preserve source provenance back to file, heading, table row, transcript timestamp, or excerpt range. Each segment receives a normalized semantic type and confidence. Destination adapters, including Product Brain, consume these typed segments later through their own profile or adapter rules.

Product Brain remains one adapter surface. WP-10 may include a Product Brain evaluation fixture because WP-9 made that surface available, but Product Brain must not define the core segment schema, type set, confidence model, or decomposition rules.

## Product Model Fit

Eligibility verdict: `EXTEND`.

WP-10 extends the canonical Mindline pipeline model:

1. source adapters normalize captures with provenance;
2. processors and method profiles enrich and classify without live actions;
3. run ledger and review queue preserve inspectable local state;
4. destination/proposal adapters consume safe review artifacts.

The reusable product object is a destination-neutral `DocumentSegment` produced by a Markdown document processor. This is not bespoke to the current six files because the same object applies to transcripts, Notion pages, Slack exports, Obsidian notes, Google Docs exports, and future source adapters.

## Scope

In scope:

- local Markdown example evaluation using the six files in `temp/`, with `temp/` ignored by Git and treated as private local input;
- a versioned destination-neutral document segment contract;
- deterministic Markdown decomposition for headings, tables, transcript blocks, bullets, and mixed prose sections;
- normalized semantic segment types such as source_note, decision, tension, action, commitment, standard, insight, work_item, reference, meeting_note, and unknown;
- confidence and review status for each segment;
- provenance back to source document ID and stable location metadata, such as heading path, table row index, transcript timestamp, line range, or content hash;
- generated local evaluation artifacts outside tracked paths by default, showing proposed segments and why they were typed that way;
- Product Brain proposal evaluation only as a downstream adapter check, not as the source of truth for segmentation;
- sanitized expected-output fixtures for at least three representative example patterns: one transcript, one mixed Slack/thread document, and one Notion/strategy document;
- documentation of where Product Brain, Notion, Obsidian, Tolaria, and future destinations plug in.

Out of scope:

- live Product Brain writes;
- live Notion, Obsidian, Tolaria, Slack, or browser integrations;
- LLM-based extraction in the first implementation slice;
- perfect semantic understanding of every sentence;
- applying proposals to any destination;
- changing Product Brain kernel behavior or workspace collection definitions;
- hardcoding the current workspace's Product Brain collections as core types;
- turning `temp/` into committed product fixtures unless sanitized and explicitly selected.
- committing raw private Markdown examples, raw meeting transcript excerpts, full local filesystem paths, Slack/thread content, personal identifiers, or raw-derived output artifacts.

## Architecture Boundary

Core owns:

- source document identity;
- document segmentation;
- provenance ranges;
- semantic segment type;
- confidence and review status;
- safety/redaction state;
- destination-neutral review artifacts.

Destination adapters own:

- workspace/profile-specific mapping;
- destination-specific proposal shape;
- destination-specific field names and workflow states;
- preview format for that destination;
- apply/write authorization.

Product Brain adapter owns Product Brain proposal mapping only after document segments exist. It cannot be used as the classifier for Notion, Obsidian, Tolaria, or local-folder behavior.

## Outcome

After WP-10, Mindline can show a local evaluation report that answers:

1. What did this Markdown document contain?
2. Which smaller candidate objects were extracted?
3. Which candidates are confident enough for downstream proposal generation?
4. Which candidates need human review?
5. Which candidates should not become durable knowledge?
6. How would a Product Brain adapter interpret the candidates without owning the decomposition model?

## Guardrails

- Destination neutrality is a hard guardrail. Any spec or implementation that names Product Brain collections as core segment types fails.
- Preserve provenance for every extracted segment.
- Prefer `needs_review` over confident misclassification.
- Keep raw examples local unless a sanitized fixture is intentionally committed.
- No live destination writes.
- No LLM dependency in this slice unless explicitly approved in a later WP.
- Product Brain chain awareness is evaluated as a follow-up concern, not implemented as live duplicate detection in WP-10.

## Raw Example Safety Boundary

Raw examples in `temp/` are private local inputs. The repository must ignore `temp/`; no implementation plan may stage or commit those files.

Generated evaluation output must default to a temporary or explicit `--out` directory outside tracked source paths. If the implementation needs committed fixtures, the fixtures must be sanitized and intentionally authored under `testdata/` with:

- fake names or role labels instead of personal identifiers;
- no raw transcript text longer than short synthetic examples;
- no full local filesystem paths;
- no private Slack/thread content;
- no raw URLs unless they are synthetic;
- provenance expressed as stable document ID plus location/range metadata, not private source text.

Acceptance must include a no-leak scan over committed fixtures and generated outputs, plus a no-live scan proving no destination writes occur.

## Evidence From Temp Evaluation

Manual smoke test on 2026-05-21:

- `meeting-transcript-1.md`, `meeting-transcript-2.md`, and `mixed-doc.md` were skipped as whole-file `no_product_brain_write`.
- `notion-doc-1.md` became one broad `insights` proposal.
- `notion-doc-2.md` and `notion-doc-3.md` became one broad `decisions` proposal each.

Verdict: WP-9 works mechanically but should not be used directly on raw Markdown. WP-10 must introduce decomposition before destination proposal generation.

## Stop Point

This LOOP run stops at Plan Ready:

- signed spec persisted in `.productbrain/specs/`;
- durable WP materialized in Product Brain after spec sign-off;
- signed implementation plan persisted in `.productbrain/plans/`;
- no implementation starts in this turn.
