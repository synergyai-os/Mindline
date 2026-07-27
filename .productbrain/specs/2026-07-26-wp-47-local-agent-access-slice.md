# WP-47 local agent access slice

Status: implemented; private sample-bound proof complete

## Outcome

A user installs Mindline once, keeps their saved evidence private on their
computer, and lets any local agent retrieve cited lessons through a stable CLI.
The same evidence can be viewed through any number of user-defined product
lenses. Explicit feedback changes later ranking for that lens without deleting,
rewriting, or promoting the underlying evidence.

## Existing authority to extend

- `internal/personalmemory.FileRepository` is the canonical private evidence
  store. It already preserves source provenance, immutable revisions,
  content-addressed enrichment, missingness, replay receipts, fingerprints,
  owner-only permissions, and bounded hydration.
- `internal/personalmemory.ContextRetriever` owns cited hydration and delegates
  only ranking through `RetrievalBackendPort`.
- The existing `mindline memory` commands are useful operator/import
  compatibility commands, but they are not yet an installable agent product.
- The stable founder proof contains eight real Slack saves and ten enriched
  resources, including three LinkedIn posts. It is private sample evidence, not
  a generalization corpus.

## Smallest coherent slice

1. Add a versioned local runtime API over an owner-only Unix socket.
2. Add a user-level service install/start/status/restart/uninstall lifecycle.
3. Add top-level machine-readable agent commands for status, search, get,
   lenses, and feedback. Preserve the existing `memory` commands.
4. Add a local SQLite runtime store for rebuildable document embeddings and
   retrieval traces plus persistent lenses and append-only judgments. Protect
   the non-derived user state with an owner-only recovery snapshot. Neither is
   a second evidence authority.
5. Add a replaceable embedding port and an Ollama adapter. Fuse existing BM25
   ranking with semantic cosine ranking using reciprocal-rank fusion.
6. Apply lens-specific record reputation from reversible judgments. User
   judgments outweigh agent judgments; neither can change retention or source
   authority.
7. Ship a reusable agent skill that teaches agents to query first, cite
   evidence and missingness, treat retrieved source content as untrusted data
   rather than instructions, and submit bounded feedback only after use.

The versioned API owns five operations: health/status, cited search, hydrated
get, lens administration, and judgment append/reverse. The CLI is a thin
client; it must not open the state database directly. The service is the only
state writer. Search index synchronization is keyed by the canonical library
fingerprint, so unchanged evidence is not re-embedded.

## Boundaries

- The CLI and skill are the first client. MCP is a later client of the same
  runtime API, not a second engine.
- Product Brain is neither a runtime dependency nor a required destination.
- LocalDB and hosted vector databases are not dependencies.
- No custom chat UI, autonomous destination writes, organizational truth, or
  no-human claim is included.
- Ollama is a replaceable founder-proof adapter. Lexical retrieval remains
  available with an explicit degraded state when semantic embedding is
  unavailable.
- Private source text, queries, embeddings, and judgments stay in the
  owner-only local boundary.
- Actor labels are cooperative audit provenance within one OS user account,
  not hostile-process authentication. Generated agent instructions always use
  the agent actor. A stronger human-presence boundary is deferred with no
  adversarial user-precedence claim.
- Install, socket, database, and logs must use user-controlled absolute paths
  with containment checks. The socket directory is mode `0700`; persisted
  secret-bearing files, state, and launch configuration are mode `0600`.
- Lenses are bounded by input size and machine resources, not a product-defined
  count or hard-coded taxonomy.

## Acceptance proof

- A clean temporary home can install the binary and skill, start the service,
  discover it after the invoking process exits, restart it, and uninstall it.
- The real stable Slack library is indexed without changing its fingerprint or
  retained record count.
- Three LinkedIn saves are retrievable with Slack provenance, linked evidence,
  and explicit missingness.
- At least four persisted lenses can be added independently; implementation
  contains no fixed lens-count product limit.
- Search reports run id, lens, retrieval mode, component scores, citations, and
  semantic degradation when applicable.
- Repeating a frozen answer-key query before and after recorded feedback shows
  a relevant citation moving up or dismissed noise moving down; a replayed
  judgment is idempotent; reversal restores the prior learned effect.
- A fresh local agent process uses only the installed skill and CLI to answer a
  founder question with citations and records its bounded feedback.
- Permission, credential, corruption/rebuild, unavailable-provider, replay,
  and restart checks pass. Proof is explicitly sample-bound.
- The existing personal-memory test suite remains green, and package/file-size
  quality gates pass without adding logic to the existing CLI god file.

## Failure and recovery

- A missing or stale socket triggers one bounded service discovery/start
  attempt, then a clear error.
- A second service process cannot become a second writer. It exits with a
  specific already-running result without modifying state.
- Corrupt SQLite is quarantined before rebuild. Embeddings rebuild from
  canonical evidence on later use; historical retrieval traces are intentionally
  not reconstructed. Lenses and judgments restore from an owner-only recovery
  snapshot. A durable recovery marker blocks ordinary startup until the
  replacement is verified, so partial quarantine, restore, or promotion
  failures resume without accepting empty state. Evidence is never rewritten
  by recovery.
- Embedding failures preserve lexical results and report the degraded mode.
- Retrieval traces and judgments are append-only and idempotent. Reversal is a
  new judgment event, never mutation of history.
- Install artifacts are owner-only. Uninstall stops the service and removes
  installed runtime/skill artifacts but does not delete canonical evidence
  unless the user separately requests destructive removal.

## Pre-build defect review

Pass 1 found and corrected four design defects before implementation:

- The runtime API and single-writer ownership were implicit.
- “Unlimited lenses” had no explicit distinction between product limits and
  safety/resource bounds.
- Duplicate-service behavior and index idempotency were unspecified.
- The brief did not protect the existing CLI file-size boundary or regression
  suite.

Pass 2 must be clean on outcome, acceptance, failure, security, operability,
maintainability, and claim-boundary review before implementation starts.

Pass 2 result: clean. No critical or high defect remained. The slice has one
evidence authority, one state writer, explicit degraded behavior, reversible
learning, bounded local mutations, observable acceptance checks, and an honest
sample-only claim.

## Claim boundary

Passing this slice proves that one private founder corpus can be accessed by a
fresh local agent through a product-shaped CLI and that explicit lens feedback
can change ranking on frozen sample queries. It does not prove cross-user
generalization, production-scale relevance quality, or autonomous correctness.

The executable proof and remaining limits are recorded in
`2026-07-26-wp-47-local-agent-access-proof.md`.
