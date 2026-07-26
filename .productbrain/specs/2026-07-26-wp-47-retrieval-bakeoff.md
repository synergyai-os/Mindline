# WP-47 retrieval substrate bake-off

Date: 2026-07-26  
Status: owned-first direction accepted; no external dependency adopted  
Authority: DEC-424, DEC-425, INS-46, WP-47

## Decision question

Should Mindline build and own its first retrieval implementation, adopt LocalDB as
a derived retrieval backend, or combine both?

This spike does not authorize adoption. It compares a released LocalDB binary
against a minimal Mindline-owned baseline under the same product contract.

## Product contract

The canonical system must:

1. Preserve every eligible Slack capture in the declared fixed window.
2. Preserve useful source context and provenance without persisting credentials
   or unsafe URL material.
3. Store canonical personal evidence owner-only.
4. Survive process restart.
5. Replay without loss or duplicates.
6. Return record-level citations and the complete selected source record.
7. Keep personal-evidence authority distinct from Product Brain authority.
8. Keep retention independent from any number of lenses.
9. Expose a headless agent-facing contract.
10. Leave derived indexes rebuildable and replaceable.

## Shared evidence

The bounded comparison used eight real link-only messages from Randy's Slack
self-capture DM in the inclusive window:

- lower: `1784756437.515429`
- upper/watermark: `1785049859.590279`
- declared records: `8`

Private source material and exact URLs remain outside source control. The local
spike roots are owner-contained under `/tmp`.

The final owned proof was regenerated from the native Slack batch after the
source-neutral record change received its own durable schema version
(`mindline-personal-evidence-library/v0.4`). Its final state was:

- library revision `2`;
- eight retained captures;
- ten current resources;
- seven immutable historical resource revisions;
- one capture import and one enrichment import;
- owner-only `0700` directories and `0600` files; and
- authority `personal_evidence_non_authoritative`.

Exact replay retained the same eight records with zero inserts and zero
updates. Four lenses retained the same eight-record denominator and unchanged
library fingerprint. Separate-process searches returned cited results for
“company brain” and “context engineering coding agents”; full-context get
rehydrated the selected record, its current and historical resource context,
and its extracted body. A durable-root secret-pattern scan returned no match.

## LocalDB spike

Version:

- repository commit: `8dcc029523398928f233dd3c5148cac4b3cc1554`
- released binary: `v0.1.0pre5`, reporting `localdb 0.1.0`
- release archive SHA-256:
  `a7dfdfdee88195ebff906a4194efaf7f8141946e94c56e6b0a2941fa46f3923d`

Observed strengths:

- Indexed all eight materialized Markdown documents into 32 chunks.
- Re-index replay wrote zero chunks, indexed zero documents, and skipped all
  eight unchanged documents.
- CLI search returned structured citations with content hashes and source URIs.
- The MCP server completed protocol initialization and exposed `search`,
  `get_document`, `get_chunks`, and `list_stores`.
- The architecture already includes embedded libSQL, FTS5, vector search,
  content-addressed documents/chunks, multiple stores, context-aware chunking,
  local embedding providers, CLI, MCP, and HTTP surfaces.

Observed gaps or unproven areas:

- LocalDB is an index, not Mindline's canonical save-intent/evidence ledger.
  Mindline still needs its own complete capture authority and a projection into
  LocalDB.
- The released binary created its data directory and database with
  group/world-readable modes in this environment. The spike had to correct
  permissions explicitly before placing private evidence there.
- LocalDB has no Slack connector today. Message connectors are roadmap work.
- The HTTP daemon's ingestion job is documented as a no-op.
- The Markdown projection did not surface the supplied Slack author, date, and
  source fields as structured citation metadata without additional adapter
  work. Source identity remained visible only inside indexed frontmatter text
  and the generated file URI.
- Search returns chunk citations; the bounded queries returned multiple chunks
  from the same capture. Mindline needs record/resource-level grouping for
  agent context packets.
- A real local ONNX embedder could not be started from the released binary in
  this environment (`model cache I/O error: Operation not permitted`). The
  executable smoke proof therefore used LocalDB's fake embedder. Exact BM25
  queries worked; semantic relevance quality remains unproven.
- `tools/list` worked through a minimal MCP client. Subsequent raw
  `tools/call` requests did not return in that hand-built client. This is
  unproven client/runtime compatibility, not yet an attributed LocalDB defect.
- The repository `LICENSE`, README, GitHub metadata, and contribution guide say
  AGPL-3.0-or-later, while the workspace `Cargo.toml` says `license = "MIT"`.
  Distribution or embedding requires explicit license clarification; no code
  may be copied into Mindline.
- The project is young: repository creation 2026-06-10, current release line
  `0.1.0pre5`, and a small public adoption footprint at spike time.

## Mindline-owned baseline

Implemented behavior after independent architecture, product, delivery, and
risk review:

- Extended the existing occurrence-complete Slack native handoff with optional
  author and permalink facts.
- Added a source-neutral personal evidence library with a repository port and
  source-generic scope/container provenance fields.
- Versioned that source-neutral durable representation as library schema
  `v0.4`; the earlier in-progress Slack-shaped `v0.3` proof is not treated as
  compatible evidence.
- Persisted all eight declared records exactly once.
- Stored personal evidence and content-addressed extracted bodies at `0700`
  directory / `0600` file modes; existing non-private directories are rejected
  rather than silently adopted or chmodded.
- Removed known tracking parameters and withheld unsafe URL material before
  persistence.
- Retained redacted shells and explicit missingness for secret, empty, deleted,
  attachment, private-file, and missing-permalink cases.
- Preserved edited capture versions as immutable searchable revisions.
- Made semantically relevant follow-up links first-class resources that can be
  enriched and retrieved through their parent capture.
- Returned one citation per record, the complete selected record, durable
  extracted context, exact excerpt/artifact evidence references, content
  hashes, missingness, and authority class
  `personal_evidence_non_authoritative`.
- A fresh process retrieved the correct "company brain" and "context
  engineering coding agents" captures.
- Exact replay reported eight unchanged records, zero inserts, zero updates,
  and the unchanged total of eight.
- Four derived lenses left the retained count and library fingerprint unchanged
  in the real bounded proof; tests cover larger user-defined lens sets without
  a product lens-count cap.
- Split the canonical agent context assembler from the ranking backend:
  retrieval implementations return ranked IDs only and cannot own authority,
  hydration, or citation semantics.
- Exposed executable JSON CLI commands for import, enrichment, status, search,
  full-context get, and lens review.

Known limits:

- Retrieval is a lexical BM25 baseline, not yet a semantic-quality solution.
- It does not yet parse PDFs, Office files, EPUB, or arbitrary web pages.
- It does not yet expose MCP; the CLI is the first headless agent contract.
- It has not yet run over the complete Slack-channel denominator.
- It has produced bounded lens-review proof but not yet the final
  human-approved Product Brain destination readback.

## Comparison

| Contract | LocalDB spike | Mindline baseline |
|---|---|---|
| Complete canonical Slack retention | Needs Mindline projection and ledger | Pass for bounded window |
| Owner-only storage by construction | Failed until manually corrected | Pass |
| Source-level provenance | Adapter work required | Pass |
| Record-level citations | Chunk duplicates require grouping | Pass |
| Full selected source context | Tool exists; raw MCP call unproven | Pass |
| Restart retrieval | Pass through separate CLI processes | Pass |
| Idempotent replay | Pass: 8 skipped | Pass: 8 unchanged |
| Agent surface | CLI and MCP available | CLI available |
| Semantic retrieval | Product capability; runtime unproven here | Not implemented |
| Parser breadth | Strong | Not implemented |
| Slack/save-intent semantics | Not implemented | Owned by Mindline |
| Lens/review/routing workflow | Not implemented | Mindline responsibility |
| License/control | AGPL and metadata inconsistency | Mindline-owned code |

## Recommendation

Do not adopt LocalDB as a first-slice product dependency, without rejecting it
permanently. This is an owned-first direction, not permission to copy LocalDB
code or a permanent commitment to rebuild commodity infrastructure.

Continue with the Mindline-owned canonical library, record-level citation and
hydration contract, lexical baseline, and replaceable ranking port. Mindline
owns what makes the product trustworthy: save intent, canonical evidence,
provenance, revisions, authority labels, lenses, context packets, review, and
routing. Commodity ranking, embedding, parsing, and indexing remain replaceable
providers rather than product authority.

Add semantic retrieval only after an answer-key evaluation defines the required
quality. Keep LocalDB as a future optional derived-index adapter candidate once:

1. Randy explicitly approves the dependency;
2. licensing is clarified;
3. owner-only storage is guaranteed without wrapper repair;
4. Slack conversation metadata maps losslessly;
5. real local embeddings and MCP tool calls pass on the supported runtime; and
6. retrieval quality beats the owned baseline on held-out queries enough to
   justify the operational and exit cost.

This recommendation owns the product-critical evidence and context contract
without committing Mindline to rebuilding every parser or embedding runtime.
The retrieval port preserves the option to integrate LocalDB or another engine
later.

## Claim boundary

This is an eight-record private founder spike. It proves bounded mechanics only.
It does not prove full-channel completeness, semantic quality, production
readiness, generalization, autonomous routing, or destination value.
