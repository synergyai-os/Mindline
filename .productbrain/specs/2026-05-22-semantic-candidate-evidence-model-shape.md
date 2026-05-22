# MINDLINE-NEXT-SHAPE-V2: Semantic Candidate Evidence Model

Date: 2026-05-22
Status: Draft for LOOP Shape review
Version: V2
Stop mode: Plan Ready before final work-package creation

## Chain Authority

- Product: `PROD-1` Mindline.
- Product direction: `DOMAIN-1` says Product Brain is a future destination/authority consumer, not the product; Mindline remains the ingestion engine with source, method, processor, ledger, and destination boundaries.
- Prior evidence: `INS-9` says raw Markdown examples need document decomposition before Product Brain proposal generation.
- Prior document layers:
  - `WP-10` created destination-neutral `document-segments/v0.1`.
  - `WP-11` created destination-neutral `document-structure/v0.1`.
  - `WP-12` proved real process documents and transcripts can become recognizable structure without semantic interpretation.
- Safety and adapter standards:
  - `STD-11` Product Brain SSOT must be current before and after work.
  - `STD-12` private provenance visibility is authoritative for publish blocking.
  - `STD-16` proposal adapters must fail closed and remain bundle-contained.

## Problem

WP-12 gives Mindline a reliable structural substrate, but the output is still not useful enough for human review or downstream routing. A transcript turn such as `synthetic-transcript-1/turn-023-speaker-a` proves where a speaker turn exists, but it does not explain the possible decision, issue, action, or relationship to earlier and later turns.

The next risk is overcorrecting by extracting every interesting sentence as an independent truth. In transcripts and long documents, the real reusable knowledge often emerges across spans:

- an opening agenda frames the topic;
- check-in comments or objections create context;
- middle discussion contains partial claims and alternatives;
- a recap or action assignment resolves, narrows, or supersedes earlier statements.

ADRs, PRDs, specs, plans, and process documents have different shapes, but the same architectural rule holds: meaning is often between nodes, not only inside one node.

If Mindline jumps directly from structure nodes to Product Brain proposals, Tolaria notes, Notion pages, or Obsidian files, one destination will become the hidden semantic model and Mindline will lose portability.

## Direction

Create a destination-neutral semantic candidate and evidence relationship model between `document-structure/v0.1` and destination adapters.

The model should support two levels:

1. Local semantic observations from bounded evidence spans.
2. Consolidated semantic candidates that can combine, refine, supersede, or answer related observations before any destination decides what to do.

This layer should produce reviewable local artifacts only. It should not write to Product Brain, Tolaria, Notion, Obsidian, or any live service.

## Scope

In scope:

- Define a versioned artifact contract for semantic observations, evidence relationships, and consolidated candidates.
- Support transcripts and process/capability documents as the first benchmark shapes.
- Preserve source provenance through `document-structure` node IDs, source document IDs, line ranges, and content hashes.
- Represent relationships such as `supports`, `refines`, `contradicts`, `answers`, `summarizes`, `supersedes`, `assigns_action`, and `same_topic_as`.
- Mark candidate status and confidence separately from source structure confidence.
- Require `needs_review` for any inferred semantic candidate unless deterministic evidence proves a narrow ready state.
- Keep destination unresolved in core artifacts.
- Add synthetic fixtures that prove consolidation from separated evidence spans.
- Update ignored private comparison artifacts to show what the semantic layer would produce for the same Notion/process and transcript files.

Out of scope:

- No live writes.
- No Product Brain proposal generation.
- No destination-specific fields, collection names, folder paths, database properties, or write instructions.
- No workspace-profile application.
- No final work-package materialization before user sign-off.
- No ontology solving across all document genres.
- No claim that every transcript has a facilitator, agenda, check-in, recap, or action section.
- No committing raw private `temp/` examples or raw-derived outputs.

## Product Model Fit Pre-Verdict

Verdict: `EXTEND`.

This extends the existing Mindline document pipeline:

```text
source candidate
-> document segments
-> document structure
-> semantic candidates and evidence relationships
-> workspace policy / destination proposal adapters
```

The reusable product object is `SemanticCandidate`, backed by `SemanticObservation` and `EvidenceRelation`. Product Brain, Tolaria, Notion, Obsidian, and local review queues can consume the same candidate model later, but none of them owns it.

## Outcome

After this work lands, a reviewer should be able to inspect a generated local artifact and understand:

- what possible knowledge item Mindline detected;
- which structure nodes and line ranges support it;
- whether the candidate is a local observation or a consolidation;
- which earlier/later observations it refines, answers, contradicts, or supersedes;
- why the candidate is `ready`, `needs_review`, or `blocked`;
- that no destination decision has been made yet.

## Proof Expectations

- Synthetic transcript fixture proves one final candidate can consolidate evidence from agenda, discussion, and recap/action turns.
- Synthetic process/capability fixture proves a platform capability candidate can link parent area, capability line, and related table row evidence.
- Negative fixture proves isolated comments do not become confident final candidates when later evidence contradicts or narrows them.
- Generated semantic artifacts contain no Product Brain/Tolaria/Notion/Obsidian destination hints.
- Generated artifacts contain no Chain governance IDs.
- Private benchmark comparisons remain ignored under `temp/`.
- `go test -count=1 ./...` remains passing.

## Open Risk

High-quality semantic consolidation may eventually require an AI/LLM-backed processor. The first work should still define the artifact contract, status model, deterministic baseline, fixtures, and safety boundaries before choosing or integrating any model provider. If an LLM is introduced later, it must be a processor behind the same local artifact contract, not a destination adapter shortcut.
