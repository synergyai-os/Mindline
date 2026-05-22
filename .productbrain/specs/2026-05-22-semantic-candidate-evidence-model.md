# MINDLINE-NEXT-SPEC-V2: Semantic Candidate Evidence Model

Date: 2026-05-22
Status: Draft for LOOP Spec review
Stop mode: Plan Ready before final work-package creation
Shape: `.productbrain/specs/2026-05-22-semantic-candidate-evidence-model-shape.md`

## Chain Authority

- Product: `PROD-1` Mindline.
- Product direction: `DOMAIN-1` keeps Product Brain as a future destination/authority consumer, not the Mindline product.
- Prior document layers:
  - `WP-10`: `document-segments/v0.1`.
  - `WP-11`: `document-structure/v0.1`.
  - `WP-12`: real private process/transcript benchmarks proved structure quality, while explicitly excluding transcript topics, summaries, decisions, commitments, and action-owner assignment.
- Prior insight: `INS-9` says raw Markdown examples need decomposition before Product Brain proposal generation.
- Standards: `STD-11`, `STD-12`, `STD-16`.

## Problem

Mindline can now identify structure, but structure is not meaning. The current transcript output proves timestamped speaker turns exist, yet a human still has to read the full source to know whether those turns form an issue, decision, action, unresolved question, or nothing durable.

The obvious next move, "extract semantic items from each structure node," is incomplete. In transcripts, workshops, ADRs, PRDs, specs, and process documents, meaning can be distributed across multiple nodes. Earlier comments may frame a question, middle discussion may explore alternatives, and later recap may resolve or supersede earlier fragments.

Mindline needs a local, destination-neutral artifact layer that can represent semantic candidates and their evidence graph before any workspace policy or destination adapter decides what to do.

## Product Model Fit Proof

Eligibility path: `EXTEND`.

This work extends the canonical Mindline pipeline with one reusable product object family:

- `SemanticObservation`: a local possible meaning extracted from a bounded evidence span.
- `EvidenceRelation`: a typed relationship between observations, structure nodes, or candidates.
- `SemanticCandidate`: a reviewable consolidated candidate that may combine one or more observations.

This is not bespoke because the same pattern is required for transcripts, process documents, PRDs, ADRs, specs, plans, and future source adapters. It is not Product Brain-specific because destination is unresolved in the core artifact contract.

## Impact Pack

### Use Cases

Primary happy path:

- A structured transcript produces local semantic observations for agenda framing, open question, discussion point, proposed action, and recap/action assignment. The layer then emits a consolidated `decision_candidate`, `action_candidate`, `issue_candidate`, or `topic_candidate` only when evidence relationships justify it.

Alternate paths:

- A process/capability document produces `capability_candidate`, `dependency_candidate`, `requirement_candidate`, or `issue_candidate` artifacts linked to section, capability, and table-row structure nodes.
- An ADR-shaped document preserves `context`, `decision`, and `consequence` evidence without inventing a meeting-style flow.
- A PRD-shaped document preserves `problem`, `user`, `requirement`, `constraint`, and `success_signal` evidence without applying Product Brain collection names.
- A transcript without a facilitator or recap still produces local observations but may stop before consolidation.
- Contradictory, weak, or isolated observations become `needs_review`, not ready candidates.
- Unsafe/private markers block candidate generation or preview writing.

### Upstream and Downstream

Upstream:

- Markdown files.
- `document-segments/v0.1`.
- `document-structure/v0.1`.

Downstream:

- Future local review queue.
- Future workspace policy/profile layer.
- Future destination adapters for Product Brain, Tolaria, Notion, Obsidian, and local files.

Out of scope downstream:

- No live writes.
- No destination proposals.
- No workspace profile application.

### Architecture Contract

Add local artifact roots:

```text
<out>/document-structure/
  structure-summary.json
  nodes/<structure_node_id>.json
  previews/<structure_node_id>.md

<out>/semantic-candidates/
  semantic-summary.json
  observations/<observation_id>.json
  candidates/<candidate_id>.json
  relations/<relation_id>.json
  previews/<id>.md
```

The semantic layer reads persisted `document-structure/` artifacts and may read related `document-segments/` artifacts. It must not import Product Brain proposal code or destination packages.

CLI input contract:

- When input is a Markdown file or Markdown directory, the command must first write or reuse `document-structure/` under the same explicit `--out` root, then write `semantic-candidates/`.
- When input is an existing structure run directory, the command must read that persisted `document-structure/` tree and write `semantic-candidates/` under the explicit `--out` root.
- Markdown directory input is supported and must use the same lexical deterministic traversal rules as `mindline documents structure`.
- Semantic artifacts must never reference structure node IDs that are not inspectable in the same output bundle or in the explicitly supplied persisted structure run.

### Candidate Kinds

Allowed candidate kinds for the first slice:

- `topic_candidate`
- `decision_candidate`
- `action_candidate`
- `issue_candidate`
- `question_candidate`
- `requirement_candidate`
- `capability_candidate`
- `dependency_candidate`
- `risk_candidate`
- `reference_candidate`
- `unknown_candidate`

These are semantic candidates, not destination collection names.

### Observation Kinds

Allowed observation kinds:

- `agenda_frame`
- `claim`
- `question`
- `proposal`
- `objection`
- `decision_signal`
- `action_signal`
- `owner_signal`
- `deadline_signal`
- `recap_signal`
- `capability_statement`
- `requirement_statement`
- `dependency_statement`
- `risk_statement`
- `reference_statement`
- `unknown_observation`

### Relationship Types

Allowed relationship types:

- `supports`
- `refines`
- `contradicts`
- `answers`
- `supersedes`
- `summarizes`
- `same_topic_as`
- `depends_on`
- `assigns_action`
- `mentions_owner`
- `mentions_deadline`
- `derived_from`

Every consolidated candidate must have at least one `derived_from` relation to observations and at least one evidence node reference.

### Summary Schema

`semantic-summary.json` must include:

- `schema_version`: `semantic-candidate-summary/v0.1`
- `run_id`
- `source_count`
- `observation_count`
- `candidate_count`
- `relation_count`
- `needs_review_count`
- `blocked_count`
- `candidate_kind_counts`
- `observation_kind_counts`
- `relationship_type_counts`
- `candidates[]` with `candidate_id`, `candidate_kind`, `review_status`, `confidence`, `candidate_path`, `preview_path`

### Observation Schema

Each observation JSON must include:

- `schema_version`: `semantic-observation/v0.1`
- `observation_id`
- `run_id`
- `source_document_id`
- `observation_kind`
- `review_status`
- `confidence`
- `title`
- `summary`
- `evidence_nodes[]`
- `evidence_ranges[]`
- `content_hash`
- `blockers[]`

### Candidate Schema

Each candidate JSON must include:

- `schema_version`: `semantic-candidate/v0.1`
- `candidate_id`
- `run_id`
- `candidate_kind`
- `review_status`
- `confidence`
- `title`
- `summary`
- `evidence_nodes[]`
- `evidence_ranges[]`
- `observation_ids[]`
- `relation_ids[]`
- `destination_status`: always `unresolved` in core artifacts
- `blockers[]`

### Relation Schema

Each relation JSON must include:

- `schema_version`: `semantic-relation/v0.1`
- `relation_id`
- `run_id`
- `relationship_type`
- `from_id`
- `from_type`: `structure_node`, `observation`, or `candidate`
- `to_id`
- `to_type`: `structure_node`, `observation`, or `candidate`
- `evidence_nodes[]`
- `confidence`
- `review_status`
- `blockers[]`

## Extraction And Consolidation Rules

The first implementation must provide a deterministic baseline and a contract that can later accept an AI-backed processor.

Deterministic baseline:

- Transcript turns can emit observations only from explicit lexical markers such as question marks, action phrases, owner/deadline phrases, recap phrases, and decision phrases.
- Process/capability nodes can emit observations from capability nodes, requirement-like headings, dependency phrases, and risk phrases.
- Consolidation may group adjacent or same-topic observations when they share normalized keywords, source document, nearby line ranges, or explicit recap/action markers.
- Weak or broad consolidation must be `needs_review`.
- Ready candidates require clear evidence, non-empty title/summary, at least one relation, and no blockers.

AI/LLM boundary:

- No live LLM/provider integration is required or allowed in the first implementation slice.
- If high-quality extraction later requires an AI processor, it must write the same artifact schemas, preserve evidence, and remain local/dry-run unless explicitly authorized by a later signed spec.

## Safety Rules

- Raw private source content must not be committed.
- `temp/` remains ignored.
- Generated committed fixtures must be synthetic.
- Core artifacts must not contain Product Brain, Tolaria, Notion, Obsidian, profile, collection, or destination proposal hints.
- Core artifacts must not contain Chain governance IDs.
- Unsafe/private markers in titles, summaries, evidence node IDs, source document IDs, relation endpoints, or previews must block or redact before writing.
- Candidate previews must show evidence references and short summaries, not full raw transcript dumps.

## Acceptance Criteria

1. A local command shape is defined and implemented:
   `mindline documents semantics <structure-run-dir-or-markdown-path-or-markdown-dir> --out <dir>`.
2. The command writes `semantic-candidates/` artifacts and prints deterministic summary JSON.
3. Markdown file and Markdown directory input persist inspectable `document-structure/` artifacts under the same `--out` root before semantic artifacts reference structure node IDs.
4. Synthetic transcript fixture proves separated agenda, discussion, and recap/action turns can consolidate into one candidate with relationship evidence.
5. Synthetic transcript negative fixture proves isolated or contradicted observations remain `needs_review` and do not become ready final candidates.
6. Synthetic process/capability fixture proves a capability candidate can link parent area, capability node, and related table-row evidence.
7. Schemas include observations, candidates, relations, review status, confidence, evidence nodes, evidence ranges, blockers, and unresolved destination status.
8. Generated artifacts are deterministic across repeated runs.
9. Generated artifacts contain no destination hints, live integration hooks, Chain governance IDs, private markers, or `null` relation arrays.
10. Private benchmark comparison docs under ignored `temp/` show the same process and transcript benchmark files before/after semantic candidate generation.
11. `go test -count=1 ./...` passes.

## Required Verification

- `go test -count=1 ./...`
- `go run ./cmd/mindline documents semantics testdata/documents/semantic --out /tmp/mindline-semantic-verify`
- verify `/tmp/mindline-semantic-verify/document-structure/structure-summary.json` exists when Markdown directory input is used
- deterministic rerun/diff of generated semantic artifacts
- scans over generated semantic artifacts for destination hints, live integration terms, Chain governance IDs, private markers, unsafe marker leakage, and `: null`
- `git status --short --ignored temp`

## Exclusions

- No live writes.
- No Product Brain proposal generation.
- No Product Brain, Tolaria, Notion, Obsidian, Slack, browser, network, auth, provider, daemon, UI, or LLM integration.
- No workspace profile application.
- No final work-package creation before signed spec/plan and user sign-off.
- No committed private source content or recognizable private labels.
- No claim that deterministic heuristics are final high-quality semantic understanding.

## Open Follow-Up

This spec intentionally creates the semantic evidence contract before model choice. A later work package may introduce an AI-backed semantic processor if deterministic output proves insufficient, but it must preserve the same destination-neutral evidence model and review gates.
