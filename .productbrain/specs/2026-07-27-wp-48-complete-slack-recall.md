# WP-48 complete Slack recall — signed Spec

Date: 2026-07-27  
Status: signed in DEC-428  
Shape authority: DEC-427  
Depends on: WP-47 at `c5a5617` or a merged equivalent  
Governing authority: DEC-422, DEC-424, DEC-425, DEC-426, DEC-430, STD-20,
STD-24, TEN-37, INS-48

## Outcome

Mindline turns one immutable Slack self-DM scope into a complete, owner-only
personal evidence library that survives restart and can be queried by a fresh
local agent through the installed CLI and skill.

Every user-authored native Slack identity in the scope is retained by default.
Unsafe URL fragments and secret-like text cross the canonical field-level
sanitization/redaction boundary, which preserves a searchable safe shell,
explicit missingness, and the saved identity without persisting the unsafe
value. Unknown or ambiguous author ownership becomes a searchable,
content-free withheld placeholder. Objective non-user/system artifacts may
be structurally excluded. Lenses and feedback affect retrieval only; neither
may change the canonical denominator.

The founder experiences one short journey:

1. Codex freezes and drains the self-DM scope without asking for a Slack token,
   cursor, shard choice, enrichment decision, or destination.
2. Mindline exposes a read-only run state: `incomplete`, `recovering`, `failed`,
   or `complete`, with structural counts and claim limits.
3. After `complete`, a fresh outside agent uses only the installed Mindline
   skill and CLI to search compact citations, explicitly hydrate selected
   records, answer with citations and missingness, and abstain when evidence is
   absent.
4. Randy judges whether the answers are genuinely useful. Only then may
   bounded, retry-safe relevance feedback be recorded.

Founder taste is the final qualitative judgment. It cannot replace executable
denominator, privacy, replay, restart, compatibility, or retrieval gates.

## Product and claim boundary

This work extends WP-47; it does not reopen or replace it.

It proves one private founder Slack self-DM scope on one local Mac and a
reusable synthetic fixture. It does not prove cross-user behavior, production
scale, broad semantic quality, destination safety, autonomous judgment, or
no-human operation.

Out of scope:

- Slack OAuth, onboarding, or a custom UI;
- MCP-first delivery or a custom chat surface;
- Product Brain runtime, API, key, or destination behavior;
- Tolaria or any other destination write;
- organization/product authority or promotion of personal evidence to truth;
- full recovery of every linked page, media body, or comment;
- hostile same-user authentication or multi-user isolation;
- committed private Slack data, URLs, excerpts, identifiers, cursors, or
  runtime artifacts.

## Architecture boundary

The vertical slice is:

`Codex connector → Slack adapter → ingestion controller → FileRepository`

and, independently:

`FileRepository → resource queue/fetch ports → enrichment merge`

and:

`FileRepository → ranking port → compact cited packet → explicit get`

`FileRepository` remains the only canonical authority for captures, resources,
provenance, current values, revisions, and content artifacts. Controller
ledgers, resource queues, embeddings, retrieval traces, lenses, and judgments
are derived or operational state. Product Brain is design authority for this
work, never a Mindline runtime dependency.

Credentials and native Slack pagination remain exclusively inside the
connector. The controller and local service accept no Slack credential or raw
provider cursor field. The Codex connector is an operator adapter for this
proof; future OAuth implements the same source port.

Recommended package ownership:

- `internal/ingestioncontroller`: source-neutral run, structural ledger,
  adoption protocol, restart/replay;
- `internal/adapters/slack`: translation and identity reconciliation for
  strict native Slack frames;
- `internal/resourcequeue`: derived resource work, budgets, attempts, terminal
  reasons;
- `internal/resourcefetch`: public HTTP policy, safe dial/redirect controls,
  provider-neutral extraction;
- `internal/personalmemory`: canonical compact citations and explicit
  hydration;
- `internal/localservice` and `internal/cli`: versioned local-agent transport,
  migration, and rollback;
- the connector implementation remains outside the core and owns Slack access.

No package may copy or depend on LocalDB code.

## Frozen source and acquisition contract

### Scope

At run start the connector freezes one workspace, one self-DM channel, the
oldest inclusive source boundary, and the latest inclusive watermark observed
at that moment. Newer Slack items are outside this run.

The private owner-only run ledger may persist source identity and timestamp
bounds because they are required for restart. Public proof artifacts may not
contain those values.

### Native adoption units

Each adoption unit is an unchanged `mindline_native_slack_batch/v1`:

- one workspace/channel/window;
- threads and replies included;
- channel and thread pagination explicitly exhausted;
- declared count equal to strict message count;
- unique non-empty native message identities;
- every message timestamp inside the unit window;
- watermark equal to the unit upper bound;
- at most 20,000 messages and 50,000 observed URL occurrences.

Overlapping windows are permitted and expected when a parent and reply cross a
shard boundary. The connector must expose every native occurrence needed for
thread closure. It must never present overlapping windows as non-overlapping.

### Run envelope

The source-neutral controller accepts a versioned ephemeral envelope
`mindline_ingestion_run/v0.1` containing:

- random run ID;
- source-adapter kind and private source scope;
- frozen policy and budget configuration fingerprint;
- ordered adoption-unit descriptors;
- one strict native batch per adoption unit;
- connector assertions for channel-history exhaustion, thread exhaustion, and
  frozen watermark;
- one ephemeral identity-bound structural author class for each native message:
  `user`, `non_user`, or `unknown`.

The envelope carries no token. Raw message content and native identities may
exist in memory while processing but are never written to the structural
ledger or proof artifact.

Its closed stream framing is:

- one `begin` frame declaring run/scope/configuration, exact ordered unit count,
  aggregate message-count ceiling, and aggregate byte ceiling;
- exactly one descriptor-bound `unit` frame for each declared ordinal;
- one `end` frame with observed unit/message/byte totals and an ephemeral
  full-envelope commitment.

Each unit frame is capped at 32 MiB; the complete ephemeral envelope is capped
at 128 MiB and 100,000 native messages. The controller buffers and validates
the entire bounded envelope in process memory and computes the exact union
before the first import. Missing/duplicate ordinals, descriptor/total mismatch,
truncation, trailing data, missing end marker, or a cap breach leave the run
incomplete without changing already acknowledged canonical evidence. These are
founder-PoC limits, not production-scale claims; a larger source requires a
separately specified streaming/sharding migration.

### Exact reconciliation

For each attempt the controller recomputes the complete union in memory. It:

1. validates every strict native batch before normalization;
2. constructs the unique key
   `(workspace, channel, native_message_id)`;
3. assigns repeated keys to one final owner using the lexicographically first
   ordered adoption-unit descriptor, and requires every occurrence of a
   repeated key to have the same structural author class and resulting
   disposition;
4. verifies exact unique, overlap, gap, parent, reply, and thread-closure
   counts;
5. creates a sorted aggregate SHA-256 commitment over the unique native keys;
6. assigns each delivered identity one structural disposition: retain,
   withhold, or objectively exclude;
7. passes every non-excluded identity through Slack-to-canonical normalization
   and `FileRepository.Import`, including withheld identities and identities
   repeated by an overlapping batch;
8. creates an adoption receipt satisfying
   `delivered_native = canonical_declared + structural_excluded`, verifies the
   canonical import receipt against `canonical_declared`, and requires repeated
   identities to be unchanged or a truthful edit/delete revision under the
   existing canonical idempotency contract;
9. performs a fresh canonical status/readback;
10. advances the structural ledger atomically only after steps 1–9 pass.

The strict v1 frame is always validated whole. Structural exclusion is the only
pre-canonical filter; overlap ownership never filters delivery. Final ownership
is a denominator/reconciliation fact only. The ledger never stores individual
native identities. If a process stops before final reconciliation, restart
reacquires and replays the frozen scope from its beginning. Already imported
canonical evidence remains useful and idempotent; the run remains `recovering`
or `incomplete`, never `complete`.

Completion requires:

- every channel-history page and discovered thread exhausted;
- exact unique-key union and aggregate commitment;
- one final owner per unique key;
- no unresolved gap, overlap, parent, reply, or thread-closure count;
- no conflicting author class or disposition for an overlapping native key;
- adoption equation, canonical import receipt equality, and fresh canonical
  readback for every adoption unit;
- `unique_native = retained + excluded + withheld`;
- `user_authored_excluded = 0`.

## Disposition and revision semantics

Disposition is independent from enrichment:

- `retained`: the current user-authored capture is canonical and searchable;
  unsafe URL fragments and secret-like text are sanitized or redacted inside
  the canonical constructor with explicit missingness, never by withholding
  the complete user-authored item;
- `excluded`: only objectively empty non-user/system transport artifacts;
- `withheld`: unknown or ambiguous author ownership becomes a content-free, owner-only,
  searchable placeholder with native identity, occurrence time, provenance,
  authority, and a fixed structural reason;
- `unknown_author`: fail closed to `withheld`, never `excluded`.

Capacity exhaustion makes the run incomplete; it may not turn a user-authored
capture into `excluded` or silently drop it.

Duplicate delivery reuses the same logical record. A changed native message
creates one immutable superseded revision and one current edited record. A
delete creates a current tombstone while preserving prior revisions. Replay of
unchanged content adds no record, resource, revision, or feedback effect.

Source references remain stable local `slack://` references unless the
connector truthfully supplies a real permalink. Mindline never fabricates one.

## Structural run ledger

The owner-only, atomically replaced ledger uses
`mindline_ingestion_ledger/v0.1`. It contains only:

- schema/build version, random run ID, lifecycle state, and claim-limit enums;
- private source scope and shard timestamp bounds;
- fixed policy/budget configuration and fingerprint;
- per-unit strict-validation, pagination/thread, adoption, receipt, and
  canonical-readback states;
- delivered/canonical-declared/owned/retained/excluded/withheld/overlap/gap/
  thread counts and the adoption equation result;
- aggregate native commitment;
- post-sanitization batch, canonical-before, and canonical-after counts and
  fingerprints;
- resource queue counts, fixed reason-code counts, and consumed budgets.

It contains no message text, URL, hostname, content-derived error, author name,
credential, header, cookie, raw provider cursor, individual native identity,
excerpt, or destination instruction.

Ledger root mode is `0700`; files are `0600`. Existing unsafe ownership,
symlinks, containment escapes, or wrong modes fail closed. The ledger sits
outside the canonical evidence root and agent SQLite state so a PR #47 binary
can safely ignore it.

## Resource queue and processing contract

Every safe canonical resource discovered by the Slack import receives exactly
one run state:

- `queued`;
- `processing`;
- terminal `complete`;
- terminal `partial`;
- terminal `blocked:<fixed_reason>`.

`complete` means the provider's required public context profile was collected
within policy. `partial` means non-empty useful public context was collected
but named required fields remain missing. `blocked` means no useful remote
context was safely retrievable; the Slack capture and explicit missingness
remain searchable.

Fixed blocked reasons include:

- `sensitive_or_ambiguous`;
- `unsupported_scheme`;
- `unsupported_mime`;
- `access_denied`;
- `unreachable`;
- `rate_limited`;
- `unsafe_network_target`;
- `budget_exhausted`;
- `extractor_unsupported`;
- `manual_processing_required`.

The queue persists only safe resource ID, lifecycle state, reason code,
attempt/budget counters, and configuration fingerprint. It resolves the
canonical URL from `FileRepository` in memory only when executing a fetch.
Queue state is derived and may be rebuilt; enrichment is committed only through
the existing canonical merge path.

Terminal queue state maps into the existing canonical `ResourceContext`
contract so rebuilding the queue cannot erase searchable missingness:

- queue `complete` → canonical `complete`;
- queue `partial` → canonical `partial`;
- queue `blocked:sensitive_or_ambiguous`, `blocked:access_denied`, or
  `blocked:manual_processing_required` → canonical `inaccessible`;
- every other queue `blocked:<fixed_reason>` → canonical `failed`;
- every blocked mapping adds current missingness
  `resource_blocked:<fixed_reason>` and no fabricated excerpt/content.

Compact citations and explicit get read the canonical current resource state
and missingness, never derived queue state. The queue remains operational
evidence for attempts and budgets only.

A restart converts stale `processing` work to retryable `queued` without
resetting attempts or budgets. Terminal items remain terminal for this run. A
later retry requires a new explicitly bounded run.

Provider-neutral required context:

- HTML/article/Substack: title when available, author/date when available,
  main text or a named missingness reason, and discovered semantically relevant
  outbound links;
- GitHub: repository identity, description/metadata, and README-level context;
- YouTube: title/channel/description and transcript when publicly accessible,
  otherwise explicit transcript missingness;
- Spotify/media: public title/creator/description where accessible, with
  explicit body/transcript missingness;
- LinkedIn: public post text/author/outbound links and relevant comments only
  when accessible without credentials; otherwise `partial` or `blocked`;
- unknown public HTTP(S): safe metadata and extracted readable text, otherwise
  a fixed terminal reason.

The provider profile does not change the canonical resource schema.

## Frozen PoC budgets

Run start requires at least 4 GiB free. Before each import, a repository-owned
lock-held admission operation computes the exact next canonical library,
serializes it, and rejects the transition before persistence if it would exceed
48 MiB. The controller may not duplicate repository mutation logic or perform
an unlocked estimate/then-import sequence. This preserves headroom beneath the
current 64 MiB `MaximumLibraryBytes` invariant. Reaching the cap leaves the run
incomplete and retains all evidence already acknowledged by canonical
readback; it never converts a user item to excluded/withheld. Direct and
concurrent imports must pass the same repository admission path. A future
larger corpus requires a separately specified canonical sharding migration.

Resource run caps:

- 500 resources;
- 1,000 requests;
- 256 MiB aggregate downloaded bytes;
- 64 MiB aggregate extracted bytes;
- 512 MiB derived-runtime storage;
- global concurrency 4 and per-host concurrency 1;
- 45-minute run wall time;
- three total attempts per resource;
- three redirects per attempt.

Per response:

- 20-second end-to-end deadline;
- 5 MiB compressed/wire bytes;
- 2 MiB decoded bytes;
- 512 KiB extracted text.

Retries are limited to transient network errors, HTTP 429, and HTTP 5xx.
Backoff is 1 second then 3 seconds; `Retry-After` is honored up to 60 seconds.
All numeric caps and consumed counters survive restart. Exhaustion marks the
affected resource `blocked:budget_exhausted`; it never drops its Slack capture.

Budget configuration schema is `mindline-resource-budget/v0.1`. Each run
persists the exact numeric configuration and its fingerprint. The live profile
uses the caps above. Deterministic tests use separately named, versioned,
fingerprinted fixture profiles; test-only lower caps may never become live
defaults.

## Private fetch policy

The complete in-memory URL token passes `STD-20` before queue identity,
fingerprint, fetch, or log. Sensitive or ambiguous tokens are never fetched and
must produce identical durable fingerprints when only their secret value
changes.

Allowed requests:

- HTTP(S) only, ports 80/443;
- no userinfo, IP literal, single-label/local hostname, proxy environment,
  inherited browser/session state, cookies, credentials, `Authorization`, or
  `Proxy-Authorization`;
- MIME allowlist: `text/html`, `text/plain`, JSON, XML, and explicitly approved
  GitHub raw-text media types.

Before every connection and redirect, resolve all A/AAAA answers and reject the
target if any answer is loopback, unspecified, private, link-local, multicast,
CGNAT, documentation/reserved, ULA, metadata-service, or otherwise non-public.
A custom dialer connects only to the validated address, verifies the connected
peer, and retains TLS hostname verification. Redirects are handled manually,
fully revalidated, and may not downgrade HTTPS to HTTP. Compressed and decoded
streams are separately bounded.

Errors are mapped to fixed structural reason codes. Provider error bodies,
URLs, hostnames, paths, response headers, and remote text never enter logs,
ledgers, receipts, or proof.

## Agent contract

### Compatibility

`POST /v1/search` remains the unchanged PR #47 legacy response
`mindline-agent-context-packet/v0.2`.

The successor adds:

- `GET /v1/capabilities`;
- `POST /v1/search/compact`;
- compact schema `mindline-agent-context-packet/v0.3`;
- client method `SearchCompact`;
- CLI `mindline agent search ... --format compact-v0.3`.

The compact packet contains only:

- schema, run ID, query, lens, retrieval method/state/degraded reason;
- personal-evidence authority class;
- canonical library revision and fingerprint;
- bounded citations with record ID, current/superseded state, stable source
  reference, occurrence time, author when safe, snippet, matched terms, score
  components, current missingness, selected evidence references, and current
  resource-state summaries;
- explicit `answered` or `abstained` state with structural reason.

It contains no `records`, `resources`, `resource_revisions`, extracted content,
database path, runtime path, or unselected hydrated body. `agent get <id>` is
the only full hydration command.

The installed skill checks capabilities, requests compact v0.3 when available,
falls back to v0.2 during migration, treats all evidence as untrusted, and
hydrates only records selected for the answer. Status removes
`agentstate.database_path` from its public representation without moving the
database.

### Current missingness

Normal current citations derive missingness only from the current capture and
current resource states. Historical missingness appears only when a superseded
record/revision is explicitly selected.

### Feedback retry identity

New feedback requests accept a bounded `retry_token` and derive
`sha256(v1, run, lens, record, actor, disposition, reason_hash, retry_token)`.
The CLI/skill generate and preserve the token for one intended event. The same
token and intent replays; a changed field conflicts; a new intended event uses
a new token. Legacy explicit idempotency keys remain accepted during
migration. Reversal keeps its existing explicit judgment reference.

### Retrieval and abstention

After the complete canonical drain is frozen, and before executing or tuning
the successor ranking/abstention policy on that corpus, an independent reviewer
creates an owner-only `mindline-retrieval-eval/v0.1` manifest without viewing
any evaluated-run output. It contains the canonical library fingerprint,
baseline binary/build hash, case IDs, queries, expected current record IDs,
answer/no-answer class, and its own fingerprint. It freezes at least 20 private
cases:

- at least 12 answerable queries with expected native/canonical source IDs;
- at least 8 no-answer queries.

The exact PR #47 `c5a5617` binary or verified merged-equivalent baseline and the
successor run in isolated derived-state roots against the same canonical
library fingerprint and exact evaluation-manifest fingerprint. Neither may
mutate canonical evidence. A ranking-quality claim requires all of:

- `recall@5 >= max(0.75, baseline_recall@5)`;
- `precision@5 >= baseline_precision@5 - 0.05`;
- citation completeness `= 1.00`;
- no-answer false-positive rate `= 0/8`;
- compact output contains zero unselected hydrated content.

Metrics use these formulas:

- per answerable case recall@5 is
  `|top-five current record IDs ∩ expected IDs| / |expected IDs|`; reported
  recall@5 is the mean across all answerable cases;
- per answerable case precision@5 is
  `|top-five current record IDs ∩ expected IDs| / |returned current IDs up to five|`;
  an empty answerable result scores zero; reported precision@5 is the mean;
- citation completeness is
  `fully valid returned top-five citations / all returned top-five citations`
  across answerable cases. A valid citation has record/source identity,
  authority, current/superseded state, content hash, current missingness,
  retrieval score, and every referenced current resource state;
- no-answer false-positive rate is
  `no-answer cases returning one or more citations / all no-answer cases`.

The abstention policy and threshold are frozen from the labelled set before
the successor evaluation and may not be tuned after viewing its held-out
results. If labels, matching fingerprints, baseline, or any threshold are
absent or failing, Mindline may claim retention and operability only—not
ranking quality or generalization.

## Upgrade and rollback

Before replacing an installed binary or skill, the installer atomically saves
the prior artifacts and hashes. Upgrade changes no canonical evidence or
durable agent-state schema.

Rollback:

1. stops the successor service;
2. restores only the prior binary and skill after hash verification;
3. restarts the prior service;
4. verifies canonical-library and durable-agent-state fingerprints equal their
   immediately pre-rollback values;
5. proves the legacy CLI can status, search, and get.

Rollback may not rewrite, delete, migrate, or relocate the canonical library,
content artifacts, SQLite, recovery snapshots, controller ledger, or resource
queue. Successor-only ledger and queue state remain safely ignored.

## Proof contract

### Deterministic fixture

The reusable, network-free fixture has 11 unique native identities across two
overlapping shards:

- 9 retained;
- 1 objective non-user/system exclusion;
- 1 content-free withholding;
- parent/reply crossing a shard boundary;
- duplicate delivery, edit, delete/tombstone, empty/system item, secret URL,
  public unreachable URL, and four safe resources.

Expected final resources:

- `complete = 1`;
- `partial = 1`;
- `blocked:unreachable = 1`;
- `blocked:budget_exhausted = 1`;
- the sensitive occurrence is redacted and never fetched.

The fixture covers every terminal queue-to-canonical mapping and asserts the
exact canonical state, `resource_blocked:<fixed_reason>` current missingness,
and absence of fabricated excerpt/content. It then deletes and rebuilds the
derived queue and proves canonical fingerprint, compact citation resource
summary, and explicit-get `ResourceContext` state/missingness are unchanged
without consulting queue state.

The primary fixture uses
`mindline-resource-budget/v0.1:fixture-primary`, with three processable resource
slots in deterministic queue order, so the fourth safe resource terminates as
`blocked:budget_exhausted`. Separate table-driven profiles lower exactly one
cap at a time:

- `fixture-resource-count`;
- `fixture-request-count`;
- `fixture-download-bytes`;
- `fixture-decoded-bytes`;
- `fixture-extracted-bytes`;
- `fixture-runtime-storage`;
- `fixture-attempt-count`;
- `fixture-wall-time`.

Each profile fixes fake response sizes, fake time, and queue order so the named
cap is crossed deterministically. Restart before and after crossing must retain
the exact configuration fingerprint and consumed counters and produce the same
terminal `blocked:budget_exhausted` result as the uninterrupted oracle.
Concurrency is a throttle, not an exhaustion state: its test asserts observed
global and per-host concurrency never exceed the frozen cap.

Inject failures immediately before and after:

`acquire → validate → import → canonical readback → resource process → ledger advance`

Every resumed run must equal the uninterrupted oracle for canonical and ledger
fingerprints, counts, revisions, resource states, and final `complete` state.
Every owned-shard and full-run replay must add zero records, revisions,
resources, or feedback effects.

The fixture must also prove:

- `unique = retained + excluded + withheld = 11`;
- `user_authored_excluded = 0`;
- every unit satisfies
  `delivered_native = canonical_declared + structural_excluded`, and its
  canonical import receipt matches `canonical_declared`;
- exact ownership, overlap, gap, parent/reply, and thread closure;
- conflicting author class or disposition for one overlapping native key fails
  incomplete before import and preserves the prior canonical fingerprint;
- closed-envelope truncation, missing/duplicate unit, descriptor/total/end
  mismatch, trailing data, 32-MiB frame, 128-MiB aggregate, and 100,000-message
  breaches all fail incomplete before the first import and preserve the prior
  canonical fingerprint;
- repository lock-held admission accepts an under-48-MiB transition, rejects
  an over-cap transition before persistence, gives direct/controller and
  concurrent imports the same atomic result, and preserves the prior canonical
  fingerprint on rejection;
- each fixture budget profile exhausts its named cap consistently before and
  after restart, while concurrency never exceeds its cap;
- owner-only modes, atomic replacement, containment and symlink rejection;
- SSRF, redirect, DNS-rebinding, proxy, userinfo, header, MIME, decompression,
  retry, rate-limit, and storage cases fail closed;
- a legacy client receives the unchanged v0.2 packet;
- the installed skill negotiates and consumes compact v0.3;
- compact output has no hydrated collections or path;
- explicit get hydrates only the selected record;
- upgrade then rollback preserves canonical and durable-state fingerprints and
  leaves the legacy CLI usable.

### Live founder proof

The private runtime must prove:

- one immutable self-DM scope with exhausted history and threads;
- the live closed envelope remains within its declared and absolute founder-PoC
  frame, aggregate-byte, and message-count caps;
- `unique_native = retained + excluded + withheld`;
- no user-authored item excluded and no unresolved reconciliation count;
- every acknowledged unit has strict validation, a passing adoption equation,
  a canonical import receipt matching `canonical_declared`, and fresh canonical
  readback;
- every import passed the repository-owned 48-MiB admission operation while
  holding the canonical lock;
- every safe resource is terminal `complete`, `partial`, or `blocked`;
- canonical readback contains the corresponding current resource state and
  fixed missingness; rebuilding the derived queue leaves canonical fingerprint,
  compact citation summaries, and explicit-get state/missingness unchanged;
- restart and replay preserve fingerprints and add zero duplicate effects;
- lens creation/removal and feedback preserve the canonical denominator;
- owner-only and secret/sensitive-URL scans pass;
- compact/legacy compatibility and one upgrade/rollback exercise pass;
- a fresh outside agent with no Slack, database, or corpus access uses only the
  installed skill/CLI, answers the frozen cases with citations and explicit
  hydration, abstains for no-answer cases, and records retry-safe bounded
  feedback;
- the PR #47 baseline and successor use the same canonical-library and
  evaluation-manifest fingerprints, and the independently labelled retrieval
  gate passes before any ranking-quality claim.

### Structural proof allowlist

Exported proof contains only:

- schema/build version, random run ID, lifecycle state;
- fixed reason-code and native/resource-state counts;
- post-sanitization SHA-256 fingerprints;
- numeric budget configuration/fingerprint and consumed counters;
- permission/scan booleans;
- test names/results;
- claim-limit enums and aggregate retrieval metrics/case fingerprints.

It contains no timestamps/bounds, source/record/native identities, text, URLs,
hostnames, paths, cursors, headers, credentials, excerpts, raw errors, or
destination policy.

## Required implementation gates

Before live private acquisition:

- the spec and plan are signed and captured on Chain;
- WP-48 exactly materializes this spec and passes Product Brain audit;
- the base contains clean PR #47 behavior;
- reusable fixture, privacy/fetch, compatibility, rollback, and failure
  injection gates pass;
- a pre-live authority receipt is bound to the exact commit and configuration.

Before completion:

- live structural proof passes;
- the outside-agent proof passes;
- two clean unchanged-tree review passes from Product, Architecture, Delivery
  Quality, Risk/Safety, and Chain Steward;
- Product Brain states the exact result, limitations, and remaining work;
- Randy can review the cited usefulness outcome without seeing private proof
  content exported from the owner-only runtime.
