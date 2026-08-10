# Project-scoped adaptive recall v1 — signed-spec candidate

Date: 2026-08-08  
Stop when: full delivery, local installation, and blind-agent proof  
Authority: Randy's explicit GO; DEC-424; FEAT-30; FEAT-28; WP-50; DEC-437; STD-28

## Outcome

An agent on Randy's computer can use Mindline as a private, cited equivalent of
ctx7: it searches one retained evidence library through a selected project
scope, lens, and stable agent identity; uses only the evidence needed; and
records feedback that improves later retrieval inside that exact context
without affecting another agent or context.

This slice proves contextual retrieval and item-level learning. It does not
claim that feedback generalizes to unseen items.

## Product model fit

Verdict: **EXTEND** the existing local-agent and lens patterns defined by
FEAT-28 and FEAT-30. Canonical personal evidence remains stored once in
`personalmemory.FileRepository`. Owner-only SQLite agent state owns scopes,
lenses, actors, retrieval traces, and reversible relevance judgments. Context
changes derived ordering, never evidence identity, retention, or authority.

## User and agent contract

- A project scope has an ID, name, purpose, and `active|archived` status.
- A lens belongs to exactly one scope and has an ID, name, query, and
  `active|archived` status. There is no product count cap.
- A stable local agent actor has an ID, name, and `active|archived` status.
  Identity provides attribution and relevance isolation, not hostile same-user
  authentication.
- Owner CLI commands create, list, and archive scopes, lenses, and actors.
  Installed agent instructions allow agents to list/select existing objects,
  but not invent or mutate them.
- Contextual search requires one active scope, lens in that scope, and active
  agent actor. Legacy search remains available without those flags.
- The original query alone authorizes the bounded candidate set. Scope purpose,
  lens query, and feedback may only reorder that set. They cannot introduce a
  citation the query did not authorize.
- Owner feedback is shared only inside its exact scope/lens. Agent feedback is
  applied only inside its exact scope/lens/agent identity. A judgment must bind
  to a cited candidate from the matching retrieval run.
- Existing lenses are projected into `owner_root_scope`. Historical owner
  judgments are projected into that exact root scope/lens; historical generic
  agent judgments are projected only under reserved `legacy_agent_actor` and
  never into another actor. The unchanged legacy tables remain authoritative
  compatibility ingress. Existing IDs, timestamps, reversals, effects,
  embeddings, and canonical evidence remain intact.

## CLI contract

Owner configuration:

- `mindline agent scope-put <id> --name <name> --purpose <text>`
- `mindline agent scope-list`
- `mindline agent scope-archive <id>`
- `mindline agent lens-put <id> --scope <scope> --name <name> --query <text>`
- `mindline agent lens-list [--scope <scope>]`
- `mindline agent lens-archive <id> --scope <scope>`
- `mindline agent actor-put <id> --name <name>`
- `mindline agent actor-list`
- `mindline agent actor-archive <id>`

Agent use:

- `mindline agent search <query> --scope <scope> --lens <lens> --agent <actor> --format compact-scoped-v0.4`
- `mindline agent feedback --run <run> --scope <scope> --lens <lens> --agent <actor> --record <record> --actor agent --disposition used|dismissed --retry-token <token>`
- `mindline agent feedback --run <run> --scope <scope> --lens <lens> --record <record> --actor owner --disposition used|dismissed --retry-token <token>`

The service advertises `mindline.scoped-recall.v0.4`. Scoped search and feedback
use distinct v0.4 routes and require a complete active tuple. Partial,
mismatched, archived, or unsupported tuples fail closed. The unchanged v0.2/v0.3
legacy routes never infer or discard context. Owner feedback has no agent ID;
agent feedback requires one. Retry identity, reversal, and run validation bind
the complete context and feedback-source tuple.

## Compatibility, safety, and recovery

- Scoped runs, candidates, judgments, scopes, actors, and scoped lenses live in
  additive tables that prior binaries cannot aggregate as generic lens state.
  Versioned recovery preserves both old and scoped state; a migration marker
  that old binaries cannot erase makes re-upgrade deterministic.
- Prior binaries can read/write unchanged legacy rows after rollback. A later
  new binary leaves those rows quarantined from active scoped recall.
- Archived contexts cannot be selected for new retrieval or feedback. Archive
  preserves traces and judgments.
- Installation is transactional. Before its first mutation it inventories,
  hashes, and backs up every changed existing binary, skill, config, receipt,
  launcher, rollback manifest, and rollback payload artifact. Any later failure
  restores that identical artifact set in reverse mutation order plus the exact
  prior running service; a failed first install is removed cleanly.
- The live install changes only derived agent state and that named artifact
  set. Canonical personal evidence is read but never rewritten by this slice.

## Acceptance proof matrix

All automated evidence is produced from one committed candidate tree. Install
is forbidden until the complete matrix passes and an unchanged-tree Chain,
Product/user-job, Architecture, Engineering/Delivery, Chain Steward, Delivery
Quality, and Risk/Safety review completes two clean rounds.

1. **Context lifecycle:** a deterministic fixture creates two scopes, two lenses
   per scope, and two agents; list/read/archive and restart preserve them.
   Missing, partial, mismatched, cross-scope, archived, and unsupported tuples
   fail. Legacy v0.2/v0.3 requests and capability negotiation remain unchanged.
2. **Authorization and lenses:** holding query, library, agent, and feedback
   constant, two lenses in one scope return exactly the same query-authorized
   record/citation set but different fixture-pinned ordering. Holding query,
   lens text, agent, and feedback constant, two scope purposes likewise change
   ordering without changing membership.
3. **Learning direction and isolation:** on a pinned candidate, `used` raises and
   `dismissed` lowers later rank/score. Owner feedback affects both agents only
   in one exact scope/lens; Agent A and B affect only themselves there. No effect
   crosses any scope or lens. Results persist after restart. Reusing one retry
   token has one durable effect; conflicting reuse fails.
4. **Binding negatives:** feedback with the wrong run, record, scope, lens,
   agent, source type, archived object, or reversal target fails before and
   after restart.
5. **Migration/recovery/downgrade:** preserve the four lenses and twelve legacy
   judgments, then prove `old state -> new migration -> scoped writes -> prior
   binary rollback -> legacy write/recovery -> new re-upgrade`; scoped state is
   preserved and legacy feedback remains quarantined. Versioned snapshot restore
   proves the same invariants.
6. **Exact commands:** `go test ./...`, `go vet ./...`, and `go test -race ./...`
   pass; migration, recovery, API/CLI contract, install-failure injection, and
   isolation suites pass inside those commands. `git diff --check` passes and
   `go run ./cmd/mindline-secret-check --root . --self-test` using scanner
   contract `mindline-secret-check/v0.1` scans tracked source plus generated
   proof. Its in-memory credential-shaped positive sentinel must be detected so
   a no-op scanner fails.
7. **Transactional install:** an injected failure after every mutation point
   restores the byte-identical named artifact set and service state. Before
   commit, v0.4 capability negotiation, status, and a fail-closed scoped request
   smoke pass; injected failure at every smoke stage restores the prior install
   or removes a failed first install. Exact tree/binary/skill hashes and rollback
   receipt bind the candidate; local status then reports the real private
   library and honest semantic-index progress.
8. **Fresh-agent outcome:** a new process/session receives only the installed
   skill, one question, and existing scope/lens/agent IDs. Its owner-only
   transcript must contain only selected `get` calls, supported material claims
   with resolvable Slack citations or an evidence-bound abstention, one durable
   judgment after an identical retry, and rejection of conflicting-token reuse.
   Real context metadata, record IDs, Slack references, queries, paths, evidence,
   and transcript stay private; durable/public proof contains only synthetic
   fixtures or content-free counts and hashes.

## Exclusions

No generalized preference learning for unseen items; no automatic agent-created
contexts; no cross-agent feedback aggregation; no source ingestion or full
Slack-drain claim; no enrichment change; no destination or Product Brain write;
no UI; no remote/multi-user authentication; no ranking-weight tuning; no
production, cross-user, autonomy, or broad quality claim.
