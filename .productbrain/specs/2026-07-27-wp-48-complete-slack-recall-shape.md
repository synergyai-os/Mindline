# WP-48 complete Slack recall — Shape

Status: Shape V2 for defect review  
Stop when: full delivery and founder-reviewable end-to-end proof  
Dependency: PR #47 commit `c5a56170381b4a3b4fa81e9c23bcdeba40402c08` or its merged equivalent

## Problem

Mindline now lets a fresh local agent retrieve cited personal evidence, but the
proof starts from only eight adopted Slack records. It does not prove Randy's
actual job: anything intentionally saved in the Slack self-DM must survive as
searchable personal evidence, even when no current lens finds it useful and
even when linked content cannot be retrieved.

The missing product capability is not another destination or review screen. It
is a complete, restart-safe source drain with a truthful denominator, followed
by explicit resource processing and compact agent recall.

## Appetite

Large. This is a private-data migration and operability slice, not a one-off
script. The implementation may be small, but completeness, recovery, privacy,
and live proof are part of the outcome.

## Outcome

For one frozen founder Slack self-DM scope:

1. every native message and reply is reconciled by native identity and
   accounted for exactly once as retained, structurally excluded, or
   structurally withheld; every user-authored self-DM item is retained by
   default;
2. every retained item remains searchable after restart, independent of all
   lenses, ranking feedback, and downstream destinations;
3. every safe extracted resource enters a provider-neutral processing queue
   and reaches an explicit `complete`, `partial`, or `blocked` result, while
   sensitive URLs remain content-free `sensitive_redacted`;
4. a fresh local agent receives compact cited search results, explicitly
   hydrates only selected records, and can record retry-safe relevance feedback;
5. generated proof reconciles the frozen native denominator, canonical
   retention, resource states, replay, restart, privacy, and retrieval quality
   without copying private content into the repository, Product Brain, logs, or
   proof reports.

## Product-model fit

Verdict: `EXTEND`.

This extends existing canonical patterns rather than creating a second memory
system:

- `NativeBatch` remains the connector-to-source-adapter handoff.
- `FileRepository` remains the canonical private evidence authority.
- the local agent service remains a downstream retrieval/feedback consumer.
- source connectors own credentials and native pagination; Mindline owns
  sanitization, semantic denominators, evidence, and recall.
- retrieval indexes and resource queues are derived operational state, never
  evidence or organizational authority.

The reusable product behavior is a source-neutral drain-controller pattern:
credential-owning connectors supply frozen native shards; Mindline validates,
adopts, reconciles, and exposes private evidence. Codex is the first Slack
connector operator, not the product boundary. A future one-click Slack OAuth
adapter must implement the same port and proof contract.

## Direction

### 1. Frozen native acquisition and cross-shard reconciliation

The credential-owning Codex connector freezes workspace, conversation,
inclusive lower/upper bounds, and thread/reply inclusion. It exhausts channel
and thread pagination.

Acquisition windows may overlap when a thread parent and reply cross a time or
capacity boundary. A versioned controller envelope assigns one final owner to
each unique `workspace/channel/native_message_id`, reconciles parents and
replies across all windows, and proves the exact union, overlaps, and gaps
before completion. It carries only an identity-only aggregate commitment and
counts; it never commits raw text, URLs, credentials, or cursors. Owned
adoption units remain strict `mindline_native_slack_batch/v1` frames for
compatibility with the existing Slack adapter and canonical store.

A cap-forced split cannot be called complete unless cross-window thread closure
and unique identity ownership reconcile. A native thread that cannot be
represented safely within the bounded controller contract fails closed rather
than losing replies.

Mindline core and the local agent service receive no Slack credential, provider
cursor, or OAuth behavior.

### 2. Receipt-gated private drain ledger

A source-controller layer commits only allowlisted structural facts:

- source and run identity;
- frozen scope and shard bounds;
- declared native/eligible/retained/excluded/withheld counts;
- pagination and thread-exhaustion states;
- identity-only union, overlap, gap, and cross-shard thread-reconciliation
  counts and aggregate commitment;
- the post-sanitization import batch fingerprint and canonical before/after
  count and fingerprint;
- resource-state counts and run state.

It stores no credential, raw cursor, message body, URL, excerpt, context packet,
or destination policy. A shard advances only after strict frame validation,
canonical import receipt equality, and canonical readback. This is cooperative
local integrity/replay evidence, not cryptographic provenance.

An interrupted run is incomplete, never falsely complete. Completed adoption
units may remain useful canonical evidence; retry reacquires or replays the
exact owned unit and never deletes or rewrites evidence.

### 3. Native identity and revisions

Native identity is `workspace/channel/native_message_id`.

- the same event and sanitized content replay once;
- an edit under the same identity appends a canonical revision;
- a delete appends a tombstone;
- no event silently disappears or becomes a duplicate;
- every user-authored message/reply is retained by default;
- exclusion is limited to objectively non-user, empty, or platform-system
  artifacts and is reconciled by native identity;
- withholding creates a searchable, content-free owner-only placeholder with
  native identity, timestamp, and structural reason code;
- excluded/withheld reason codes contain no private content.

### 4. Lens-independent retention and resource processing

Lenses and judgments can alter ranking only. They cannot delete, withhold,
rewrite, or decide whether a source survives.

Each safe URL becomes a canonical resource linked to its capture. Processing is
provider-neutral and checkpointed. Reachable sources may become `complete` or
`partial`; inaccessible, unsupported, rate-limited, budget-exhausted, or
human-required sources become `blocked` with explicit missingness. A resource's
state never determines whether its Slack capture remains searchable.

Before the live run, Mindline freezes global and per-provider budgets for
requests, resources, response bytes, extracted bytes, retries, concurrency,
wall time, and owner-only storage. Budget counters and the budget
configuration fingerprint persist across restart. Exhaustion never resets on
restart; remaining resources become `blocked:budget_exhausted` and remain
retryable only through a new explicitly bounded run.

LinkedIn posts may include accessible post text, author, outbound links, and
comments. YouTube may include transcript/description links. GitHub includes
repository metadata and README-level context. Other providers use the same
resource contract rather than shaping the core schema.

### 5. Compact agent contract

Normal agent search returns only bounded citations, scores, evidence snippets,
authority, current missingness, and retrieval diagnostics. Full records,
resources, content, and historical revisions require an explicit `agent get`
for a selected record.

This is an additive, versioned contract change. The successor introduces a new
compact packet version through an explicit endpoint or response-format
negotiation while preserving PR #47's
`mindline-agent-context-packet/v0.2` response for existing clients during the
migration window. The installed Mindline skill must negotiate or request the
new compact format and remain able to consume the legacy packet until the
migration gate passes. Rolling back the successor means restoring the prior
binary and skill without rewriting, deleting, or otherwise migrating the
canonical library or durable agent state.

The successor also:

- removes the absolute SQLite path from normal agent status;
- prevents current citations from inheriting stale missingness from historical
  placeholder revisions;
- generates feedback retry identity from run, lens, record, actor, disposition,
  and an intended-event retry token instead of asking agents for an ambiguous
  global key;
- measures relevance and abstention on held-out answer/no-answer cases before
  making ranking-quality claims.

Before any ranking change, freeze a local answer/no-answer set whose expected
source identities are labelled by a reviewer who did not use evaluated-run
output. Preserve the exact PR #47 lexical/hybrid result as baseline. The Spec
must set fail-able recall, precision/rank, citation-completeness, and no-answer
false-positive thresholds and emit before/after machine-readable results.
Founder usefulness remains the final qualitative verdict; absent independent
labels or a passing gate, Mindline makes no ranking-quality claim.

### 6. Private I/O and resource-fetch policy

Every full in-memory URL is classified under `STD-20` before queueing, hashing,
or fetching. Sensitive or ambiguous occurrences are never fetched.

Allowed fetches:

- use only public HTTP(S), without credentials, cookies, userinfo, or inherited
  browser/session state;
- reject loopback, private, link-local, metadata-service, non-HTTP(S), unsafe
  redirect, and DNS-rebinding destinations before connection and after every
  redirect;
- enforce MIME allowlists plus per-request and aggregate response-byte,
  extracted-byte, redirect, timeout, retry, concurrency, host, provider,
  storage, and wall-time budgets;
- emit only structural reason codes and counters to queue state, errors,
  receipts, scans, and proof.

The drain ledger, queue, receipts, quarantine, and recovery artifacts live
under contained owner-only roots (`0700`) with owner-only files (`0600`).
Symlinks and containment escapes fail closed.

Agent search/get are deliberate, caller-authorized evidence disclosure
surfaces inside the same cooperative OS-user boundary. Search returns bounded
citations; `get` returns only an explicitly selected record. Neither response
is automatically echoed by the drain controller, proof runner, service logs,
repository artifacts, Product Brain, or telemetry. This is data minimization,
not a claim that a hostile same-user process or an authorized Codex task cannot
observe evidence it explicitly requests.

## Use cases and failure states

| State | Required behavior |
| --- | --- |
| First complete drain | Freeze scope, exhaust all pages/threads, assign unique identity ownership, adopt old-to-new, reconcile exact union/overlaps/gaps |
| Connector/page failure | Keep run incomplete; reacquire the frozen shard without cursor-only advancement |
| Import/readback failure | Do not advance ledger; exact retry is safe |
| Duplicate delivery | Replay without adding a record or resource effect |
| Message edit/delete | Preserve prior revision and current edit/tombstone |
| Empty/system item | Retain user-authored items; exclude only objective non-user/system artifacts by native identity |
| Sensitive URL | Persist no URL or URL-derived identity; retain content-free redacted occurrence |
| Unreachable content | Retain Slack context and mark linked resource `blocked` |
| Resource budget exhausted | Preserve queue accounting and mark remainder `blocked:budget_exhausted` |
| Process restart | Resume from structural ledger and canonical receipts |
| Lens/feedback changes | Ranking may change; retention count and fingerprint do not |
| Agent query | Compact cited search, explicit hydration, bounded feedback |
| No relevant evidence | Return an honest empty/abstained result rather than fill the limit with noise |

## Proof expectations

### Reusable fixture proof

- deterministic multi-page Slack fixture with parents, replies, duplicates,
  edits, deletes, empty/system items, unsafe URLs, unreachable resources, and
  a shard boundary;
- parent/reply crossing a shard boundary plus overlapping acquisition windows,
  unique native identity ownership, and exact union/overlap/gap reconciliation;
- injected failure before and after acquisition, import, readback, resource
  processing, and ledger advancement;
- interrupted/resumed output equals uninterrupted output;
- exact replay adds zero records, revisions, resources, or feedback effects;
- only the connector port can acquire native Slack facts; core/service schemas
  expose no credential or raw provider cursor;
- normal status/search output contains no database path or unselected hydrated
  content;
- a legacy v0.2 client receives the unchanged PR #47 packet during migration,
  while the installed skill negotiates and consumes the compact packet;
- upgrade then rollback to the PR #47 binary and skill preserves the canonical
  evidence and durable agent-state fingerprints and leaves both usable;
- current missingness, retry identity, relevance, and no-answer regressions;
- request/resource/byte/retry/concurrency/wall-time/storage budget exhaustion
  before and after restart;
- owner-only modes, containment, symlink rejection, redirect/SSRF/DNS-rebinding
  rejection, and structural-only error/proof surfaces.

### Live founder proof

- one immutable frozen self-DM scope with exhausted channel and thread
  pagination;
- unique native identities = retained + structurally excluded + structurally
  withheld, with identity-level reconciliation and no silently excluded
  user-authored item;
- every retained record searchable after service restart;
- every safe resource has an explicit terminal processing state;
- lens creation/removal and relevance feedback preserve the canonical
  denominator;
- exact replay produces no duplicate effect;
- owner-only permission and secret/sensitive-URL scans pass;
- a fresh outside agent answers held-out founder questions using only the
  installed skill/CLI, cites evidence, hydrates selected records, abstains when
  appropriate, and records retry-safe bounded feedback;
- the installed skill consumes the compact contract while a legacy v0.2 client
  still receives its unchanged packet; one upgrade/rollback exercise preserves
  canonical evidence and durable agent-state fingerprints and leaves the
  rolled-back CLI usable;
- independently labelled before/after answer/no-answer results pass the frozen
  Spec thresholds; otherwise ranking-quality claims remain blocked.

Proof artifacts contain counters, fingerprints, states, reason-code counts,
test names, and claim limits only. Private content and URLs remain in the
owner-only runtime.

## Exclusions

- Product Brain, Tolaria, Notion, or any other destination write;
- Product Brain API/key/runtime dependency;
- Slack OAuth or real-user onboarding UI;
- custom chat UI or MCP-first implementation;
- blanket claim that every linked page or comment is fully retrievable;
- organizational authority graph or promotion of personal evidence to truth;
- hostile same-user authentication claim;
- cross-user, production-scale, broad relevance/generalization, destination
  safety, or no-human claim;
- committing any private Slack fixture, URL, excerpt, cursor, or runtime
  artifact.

## Circuit breakers

- fail closed when the native window, denominator, pagination exhaustion,
  strict schema, shard coverage, or import/readback equality is ambiguous;
- stop resource fetching on credentials, sensitive URLs, permission drift,
  repeated provider rate limits, or storage budgets;
- do not claim full drain from search counts, connector assignment, page
  success, or an incomplete ledger;
- do not claim a non-overlapping acquisition when thread-complete windows
  overlap; claim only unique native identity ownership after exact union
  reconciliation;
- do not start implementation from the dirty `5216fb3` worktree;
- do not merge the successor before PR #47 or an equivalent prerequisite is
  present in its base;
- do not replace the PR #47 agent packet in place: version or negotiate the
  compact response, prove the installed-skill migration, and preserve a
  non-destructive rollback to the prior binary and skill.

## Founder decisions already resolved

- Randy is steering founder and Product Taste Maker; the empowered Product Team
  owns architecture and delivery inside this boundary.
- Codex is the temporary Slack connector operator.
- Mindline, not Product Brain or LocalDB, owns personal evidence and first
  retrieval.
- full source survival is mandatory; enrichment and relevance remain separate.

No additional founder decision is required for Shape.
