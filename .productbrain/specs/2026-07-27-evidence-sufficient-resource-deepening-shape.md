# Evidence-sufficient resource deepening successor — Shape

Status: proposed amendment to signed Shape `DEC-432` after Spec review  
Reserved candidate identity: `MINDLINE-RESOURCE-EVIDENCE-DEEPENING`  
Builds on: delivered WP-48 resource/library/evaluation outputs, especially the
FEAT-27 and FEAT-29 implementation surfaces; it does not depend on WP-48
completion  
Unblocks: WP-48 useful-recall evaluation, installation, and founder review

## Problem

The founder-private WP-48 run observed a complete frozen Slack self-DM drain:
1,112 canonical records, every non-excluded native item retained, 970 safe
resources at an operational terminal state, and a local agent interface over
the resulting personal library. WP-48 remains `building` and `unvalidated`;
these local observations are not a shipped lifecycle or generalization claim.

That is not yet useful recall. A finished fetch attempt is currently allowed to
stand in for usable primary evidence. A login page, client-rendered shell,
metadata-only page, transcript-less media page, or boilerplate-heavy HTML
response can be non-empty and therefore terminal even when it does not contain
the lesson Randy saved. Retrieval already indexes the full bounded normalized
text that Mindline persisted. Further rank tuning cannot recover evidence that
is absent from that artifact.

The founder job remains:

> If I intentionally saved something, preserve it; when an agent later works on
> a related topic, give the agent the useful primary context when Mindline can
> safely acquire it, and say what is missing when it cannot.

Operational terminality and semantic evidence readiness are different facts.
Mindline needs to model and prove both.

The founder also clarified that relevance is contextual. One saved source may
matter for Product Brain governance, for AI-native organization design, or for
content creation at different strengths. Multiple agents must be able to work
over the same retained evidence without copying or mutating it, while each
project, lens, and agent keeps a bounded, inspectable relevance history. The
same query may therefore authorize the same evidence but order it differently
under two contexts. Contextual relevance is not canonical truth.

## Evidence and claim boundary

### Decided authority

- WP-48 and DEC-427 through DEC-429 define complete Slack recall, canonical
  retention, and privacy. DEC-429 requires a material boundary change to return
  to Spec.
- Signed Diagnose D-ES-2 found that this change also reopens product direction
  and therefore requires a new successor Shape before Spec.
- STD-24 keeps native completeness at the credential-owning connector while
  Mindline owns semantic extraction, completeness, and sealed inventories.
- STD-5 requires inaccessible context to stay explicit rather than being
  presented as complete.
- STD-20, PRI-1, and BR-1 govern private values, public fetching, owner-only
  evidence, and emitted surfaces.
- STD-21 keeps evidence, relevance, disposition, and destination separate;
  contextual ranking cannot become canonical truth.
- DEC-413 and DEC-424 make user-selected lenses/scopes contextual and preserve
  personal-evidence authority. This amendment extends those contexts with
  stable local agent attribution and actor-isolated feedback.
- DEC-426 keeps the local CLI/skill as the primary agent interface and makes
  feedback derived, reversible, and non-authoritative. This amendment newly
  requires contained proof of the exact candidate before default installation.

DEC-433 at `a17b0e7` and the current WP-49 materialization are stale for this
amended scope and are not delivery authority. After this Shape amendment is
captured, the amended Spec must be reviewed and captured, then existing WP-49
must be reconciled to those exact authorities before any Plan, build, staged
proof, or installation proceeds.

### Observed local evidence, not general authority

- The full frozen Slack scope contains 1,112 canonical records and 970
  processable resources.
- All 970 resources reached an operational terminal state, but the latest
  sealed retrieval cycle still reached only 0.50 recall.
- In the owner-only diagnosis, none of six unmatched expected resources had
  sufficient stored answer evidence; five were partial-but-insufficient and one
  had none. Only two of six matched resources contained sufficient evidence.
- These observations support the next hypothesis. They do not prove broad
  provider coverage, general semantic quality, or production readiness.

## Appetite

Medium.

Build the smallest reusable evidence-deepening vertical slice needed to test
the root-cause hypothesis. Do not redesign the whole web ingestion stack, store
raw web responses, recover every resource, add authenticated browsing, or build
a UI.

## Outcome

For a bounded selected subset of the already drained personal library:

1. Mindline reports fetch-attempt terminality separately from
   capability-specific semantic readiness.
2. Mindline can safely deepen a resource for one named primary-evidence
   capability and atomically promote an improved owner-only artifact without
   deleting its predecessor.
3. Duplicate, interrupted, restarted, and extractor-upgrade work is replay-safe:
   public acquisition is bounded at-least-once across ambiguous crash windows,
   while accepted attestations, current artifacts, promotions, and revisions
   have exactly-once durable effects.
4. Retrieval authorizes answers only from ready primary evidence, never from a
   shell, metadata-only artifact, or unverified legacy artifact.
5. Queries perform no network work or queue mutation. Retrieval may return a
   bounded `deepening_request` intent and report selected evidence as pending;
   an application orchestration port may enqueue that intent at most once.
6. A sealed evidence-only intervention turns at least three of the same six
   precommitted insufficient resources into sufficient evidence, then passes a
   fresh held-out retrieval gate without regressing the six already matched
   cases.
7. Evidence is stored once. An owner-defined project scope and lens rerank only
   the query-authorized candidate set. User judgment is shared within that
   context; an agent sees its own agent feedback, not another agent's private
   ranking history. No relevance event changes retention or evidence.
8. Before the default installation changes, a fresh outside agent uses the
   exact candidate CLI and skill in a contained staged runtime under one
   bounded owner-authorized disclosure grant. Its final cited answer or honest
   abstention is captured as the proof output Randy judges.
9. Only Randy's bound `useful` verdict permits crash-safe installation of the
   exact proven candidate. A non-useful verdict leaves the current installation
   plus canonical evidence and canonical relevance state unchanged; only
   owner-private proof/audit receipts may remain after staged feedback is
   destroyed.

## Product-model fit

Verdict: `EXTEND`.

This extends Mindline's existing resource-enrichment and personal-memory
patterns. It does not introduce a new memory store or destination model.

- source adapters acquire and normalize provider-specific public facts behind a
  source-neutral port;
- canonical personal memory owns current evidence artifacts, immutable
  predecessors, readiness attestations, and provenance;
- the derived queue owns leases, retry budgets, progressive scheduling, and
  restart;
- retrieval consumes canonical ready evidence and never becomes a fetcher;
- canonical personal memory owns a narrow read-only evidence port that returns
  one coherent immutable library/catalog snapshot with validated current
  evidence and citation commitments, never mutation, queue, provider, path, or
  file-layout types;
- local agent state owns project scopes, lenses, and append-only user/agent
  relevance events as derived, reversible projections over shared evidence;
- a contained candidate deployment proves the consumer experience before the
  default installation changes;
- Product Brain remains delivery authority only and is not a runtime
  dependency.

This is a successor candidate, not an in-place expansion of WP-48. It changes
canonical evidence semantics and introduces progressive deepening while
preserving WP-48's signed exclusion against blanket recovery. It builds on
named WP-48 outputs and unblocks WP-48's remaining useful-recall evaluation,
installation, and founder-review gates; WP-48 completion is not its
prerequisite.

## Direction

### 1. Separate the two state axes

Keep transport/attempt lifecycle compatible:

- queued;
- processing;
- terminal complete, partial, or blocked with fixed reasons.

Add a separate versioned semantic evidence contract:

- capability, initially `readable_body/v1`, `primary_post/v1`, or
  `transcript/v1`;
- readiness: `ready`, `insufficient`, `unverified`, or `inaccessible`;
- fixed capability-specific missingness;
- extractor adapter and policy fingerprint;
- retrieved time and source-response digest;
- produced artifact identity and predecessor artifact identity.

`complete` never implies `ready`. Legacy content starts `unverified`; migration
does not reinterpret it as answer-bearing evidence.

### 2. Preserve primary evidence and history

The full bounded normalized primary text remains the canonical owner-only
artifact. Summaries, snippets, embeddings, search projections, readiness
evidence, and proof excerpts are owner-only derivatives under the same
no-export boundary. They cannot enter hosted inference, telemetry, Product
Brain, git, public fixtures, logs, or reports without a separately governed
opt-in. This slice does not require retaining raw HTML or network responses.

A successful deepening atomically promotes a new current artifact and
attestation while preserving prior artifacts and record revisions. An
unavailable, insufficient, failed, or poorer result cannot replace stronger
current evidence.

Adapters emit bounded evidence candidates, not readiness authority. A separate
capability validator evaluates the candidate against the frozen contract and
emits the readiness attestation. Private live sufficiency labels and commitments
are frozen by an independent owner-only reviewer before the evaluated run; the
candidate adapter and retrieval output cannot create or change them.

### 3. Use one source-neutral deepening port

Core queue, canonical memory, and retrieval do not branch on provider names.
Adapters satisfy capability contracts:

- generic public HTML: extract a readable primary body and reject
  login/consent/loading/client shells;
- public LinkedIn: acquire a primary post body independently from comments;
  inaccessible comments cannot downgrade a ready primary post;
- public media: acquire a transcript or captions independently from page
  metadata; a media page alone cannot satisfy transcript readiness.

The first implementation may prove one adapter at a time, but the Spec must
freeze the shared port and all three fixture contracts before delivery begins.

### 4. Deepen progressively and replay safely

Logical satisfaction identity binds:

`resource + capability + extractor-policy fingerprint + refresh generation`

Attempt identity additionally binds:

`logical satisfaction identity + base artifact + attempt ordinal`

Selected retrieval may return one bounded `deepening_request` intent and
`evidence_pending`; it never fetches or mutates the queue synchronously. An
application orchestration port owns the at-most-once queue mutation. Search and
get are read-only over canonical evidence and queue state. Feedback is
network-free and may append only to derived local-agent state; it never mutates
canonical evidence, retention, or queue state. The same intent is a no-op when
the current attestation already satisfies the logical identity. Promotion
cannot recursively schedule another attempt under the same policy and refresh
generation.

Default refresh generation is zero. An explicit same-policy refresh carries a
stable caller retry token; an atomic allocation maps that token to exactly one
bounded next generation. Replay of the same token is a no-op, while a distinct
authorized token may allocate one later generation within frozen refresh,
attempt, and request budgets. A changed extractor policy starts at generation
zero under its new policy identity.

HTTP acquisition is bounded at-least-once. A crash after response receipt but
before durable acknowledgement may cause a duplicate GET for that ambiguous
attempt. Repeated ambiguous crashes can repeat acquisition only up to persisted
maximum attempt and request budgets; every execution is charged. Crash points
before the request, after the response, after staged artifact creation, and
after promotion must be frozen and tested. Regardless of refetch, accepted
attestation, current-artifact promotion, resource revision, and canonical
readback have exactly-once durable effects.

The live sequence is:

1. deepen the six precommitted failing resources;
2. if the evidence transition gate passes, run a capped deterministic backfill
   over the highest-priority remaining unverified resources;
3. stop at the frozen request, byte, storage, attempt, and wall-time budgets.

This is neither blanket recovery nor demand-only fetching.

### 5. Fail closed at retrieval

Resource evidence may authorize an answer only when the required capability is
independently validated as `ready` and the citation resolves through canonical
readback. Each resource-derived citation commits to the resource, exact current
artifact, capability, and readiness attestation. Readback and evaluation must
recompute or validate readiness from the artifact and frozen capability
contract rather than trust a stored status field.

Retrieval, evaluation, and the staged runtime consume the same narrow
`EvidenceReadPort`. One call returns an immutable coherent snapshot keyed by
library and evidence-catalog fingerprints. It exposes validated current
evidence and citation commitments only and copies no canonical evidence into
staged state. The Spec must freeze fingerprint-keyed cache invalidation and
per-index/per-read byte, memory, and latency caps so validation cannot become
unbounded at search time.

Slack source text remains independently searchable and is not downgraded by
resource readiness. Metadata, generic related links, shells, and unverified
legacy artifacts may help describe missingness but cannot authorize a
resource-body claim.

The ranking algorithm, thresholds, model/provider assumptions, and query
network behavior remain unchanged during the intervention.

### 6. Keep relevance contextual and reversible

Canonical evidence is shared. Retention, readiness, provenance, and citation
identity never vary by project, lens, or agent.

Every contextual retrieval binds one owner-defined active project scope, one
owner-defined active lens within that scope, and one stable local agent actor.
The original query establishes the authorized base candidate set. Scope and
lens text may rerank only that set. Effective feedback is the combination of
owner feedback for that exact scope/lens and feedback previously recorded by
the current agent for that exact scope/lens. Feedback from a different scope,
lens, or agent cannot affect the order. Conflicts remain attributed events,
not evidence changes. Owner and agent events retain the existing fixed `1.0`
and `0.25` contribution weights; "owner precedence" means that existing
weighting, not an absolute override or new scorer rule.

Existing lenses migrate into an owner root scope without changing prior search
output or losing judgments. Historical generic-agent events map to one reserved
`legacy_agent_actor` and affect only the legacy-compatible context; no new
stable agent inherits them. Scope, lens, and actor identifiers are additive
local-agent-state metadata, never access grants. This slice freezes the
existing relevance weights and adds identity/isolation only. It does not add a
cross-agent aggregation formula, learned personalization, agent-created lens
operation, or a claim that different contexts must always produce different
orders.

Users may create any practical number of scopes and lenses through the owner
surface. Storage, pagination, field sizes, and per-request work are bounded;
the product does not impose a fixed global lens count.

### 7. Prove the exact candidate before installing it

The current installed workflow is the rollback control and must remain
unchanged while usefulness is unknown. A fresh outside-agent task is bound to
one owner authorization, exact candidate tree/binary/skill behavior, staged
runtime, project/lens/actor context, fixed query, and short-lived disclosure
budget. Search, one selected hydration, and optional feedback use only the
staged copy. The staged runtime has distinct config, socket, lock, and agent
state and cannot serve through default paths.

Disclosure is deny-by-default, one authorization can issue only one manifest
and one grant, private-response allowance is reserved durably before evidence
is read, and ambiguous crashes consume rather than replay the allowance. The
agent's terminal output is a bounded `answered` or `abstained` receipt that
binds the exact task/run, output commitment, cited subset, citation validation,
and missingness. Randy's verdict binds that receipt. A valid abstention does
not require fabricated record feedback.

Teardown is crash-resumable and destroys/readbacks all staged private state.
Only a settled useful verdict for the exact candidate permits installation.
Installation uses an immutable recovery bootstrap or equivalent autostart
quarantine outside the replaceable deployment, so no default service can serve
a mixed or nonterminal install. Every fault returns to either the exact prior
installation or the exact smoked candidate. If missing or corrupt authority
prevents proving either state, the only third outcome is
`quarantined_no_service`; the bootstrap keeps the service stopped and reports
the structural recovery fault. The immutable bootstrap—not a replaceable
candidate or rollback binary—owns autostart, journal validation, recovery, and
quarantine before any default component can run.

### 8. Preserve the private-fetch boundary

Every fetch remains subject to the existing:

- STD-20 classification before durable identity, hashing, logging, or fetch;
- public HTTP(S)-only policy with no userinfo, ambient credentials, cookies,
  proxy state, or browser session;
- all-answer DNS validation, rebinding defense, pinned public peer, and
  per-redirect revalidation;
- MIME, wire, decoded, extracted, redirect, timeout, retry, concurrency,
  provider, storage, run, and wall-time budgets;
- owner-only `0700` roots, `0600` files, containment, symlink, and atomic-write
  rules;
- secret redaction and structural-only logs, receipts, status, and proof.

Network work stays outside the query path. Private full evidence and
every reconstructive derivative—including summaries, snippets, embeddings,
search projections, readiness evidence, and proof excerpts—remain owner-only.
They never enter hosted inference, telemetry, Product Brain, git, public
fixtures, logs, or reports without a separately governed opt-in.

The durable source-response digest is never computed from raw or pre-redaction
response bytes. It is derived only after response bounds, normalization, secret
redaction, and STD-20 sanitization. All lexical URLs in the digest input are
replaced with encounter-order ordinal tokens so a withheld URL cannot influence
durable identity.

## Key use cases and failure states

| State | Required behavior |
| --- | --- |
| Useful article inside boilerplate | Primary body becomes ready; boilerplate does not become answer evidence |
| Login, consent, loading, or JS shell | Attempt may be terminal; readiness is insufficient or inaccessible; no answer authorization |
| LinkedIn post without comments | Primary post may be ready; comments remain separately inaccessible |
| Media page without transcript | Transcript remains insufficient or inaccessible; page metadata cannot substitute |
| Useful bounded prefix only | Preserve partial evidence and fixed truncation missingness; do not claim full readiness |
| Duplicate enqueue | One logical job and exactly one accepted durable effect; acquisition remains bounded at-least-once |
| Interrupted before request | Restart starts the same attempt identity within persisted budgets |
| Interrupted after response but before acknowledgement | A duplicate GET is permitted per ambiguous attempt; repeated crashes remain bounded by persisted attempt/request budgets; durable effect remains exact |
| Interrupted after staged artifact or promotion | Restart reuses or reads back the staged/promoted state without another durable effect |
| New extractor policy | One new versioned job; predecessor remains readable |
| Failed or poorer result | Current stronger evidence remains unchanged |
| Query before deepening | No network; honest pending or abstention |
| Query after promotion | Same ranking policy can cite the newly ready canonical evidence |
| Sensitive or unsafe URL | No fetch and no URL-derived durable identity |

## Proof strategy

### Frozen evidence-transition gate

Before implementation, an owner-only manifest commits the same six diagnosed
resources, their required capability, current evidence artifact, independent
sufficiency commitment, and initial readiness. Passing requires an independent
validator to recompute that at least three of those exact six transitioned from
insufficient/unverified to ready. A stored `ready` flag alone is not evidence.
Substitution and post-hoc relabeling fail the gate.

### Pre-build held-out authority

Before implementation, a separate owner-only manifest freezes at least 20
held-out cases—at least 12 answerable and 8 no-answer—with their queries,
labels, expected stable source commitments, and the six current matched
non-regression commitments. Its semantic fingerprint is recorded without
revealing private case content. Candidate implementation may not read this
manifest. A manifest created or semantically changed after the candidate build
boundary is invalid.

### Controlled evidence-only retrieval intervention

After the candidate is frozen, a sealed run binding adds:

- the complete capture/record denominator and stable source commitments that do
  not include mutable resource state or content hashes;
- the immutable pre-build held-out manifest fingerprint;
- exact candidate tree, binary, evaluator, retrieval/ranker fingerprint, and
  runtime configuration;
- old and deepened canonical snapshots;
- the allowlist of named resource evidence revisions that may differ, with
  separate old/new evidence commitments.

The same candidate binary evaluates both snapshots. Any other canonical,
configuration, pre-build manifest, label, query, ranking, or model difference
invalidates the comparison. Expected source records are checked through stable
source commitments; allowed evidence changes are checked through resource,
exact artifact, capability, readiness-attestation, and predecessor
commitments.

Passing requires:

- evidence transition: at least 3 of the committed 6 become ready;
- recall at least `max(0.75, baseline recall)`;
- precision at least `max(0.15, baseline precision - 0.05)`;
- citation completeness `1.00`;
- no-answer false-positive rate `0`;
- exact per-case non-regression for the six previously matched answerable
  cases;
- unchanged source denominator and Slack-source recall behavior.

Before evaluation, every previously matched case that depends on resource
evidence is a mandatory deepening target. Optional backfill cannot be the only
route to the exact non-regression gate after legacy evidence becomes
`unverified`.

### Reusable lifecycle and safety gates

Public fixtures prove:

- readable article body retained while navigation/cookie/footer decoys do not
  authorize;
- nonempty shell terminalizes without becoming ready;
- LinkedIn primary post and comments are independent capabilities;
- transcript and page metadata are independent capabilities;
- duplicate enqueue, bounded at-least-once acquisition, exactly-once durable
  effects, crash before request, one and repeated ambiguous crashes after
  response, crash after staging, crash after promotion, restart, and extractor
  upgrade; repeated ambiguity never exceeds persisted attempt/request budgets;
- unavailable terminal state and bounded retry;
- failed/poorer promotion no-op and predecessor retention;
- logical satisfaction prevents post-promotion self-requeue; exact replay of a
  refresh token is a no-op; one authorized new token allocates one new bounded
  refresh generation;
- retrieval emits at most one bounded intent, performs zero queue mutations and
  network calls, and abstains while pending; orchestration enqueues the intent
  at most once;
- unchanged retrieval/ranker fingerprint;
- adapter candidate cannot self-authorize readiness; validator recomputes
  capability status from the cited current artifact;
- resource citations bind resource, artifact, capability, and attestation;
- stable source commitments remain unchanged while allowlisted evidence
  commitments change exactly;
- all evidence derivatives remain owner-only and absent from hosted inference,
  telemetry, Product Brain, git, public fixtures, logs, and reports;
- source-response digest changes neither with secret values nor with lexical URL
  values that sanitize to the same ordinal structure;
- canonical readback, rollback, privacy, SSRF, credential, secret, and
  owner-only containment gates.

Public contextual-relevance fixtures also prove:

- identical query/library inputs yield the same authorized base candidates and
  citations across two project/lens contexts;
- at least one precommitted fixture yields a different post-authorization
  order under those contexts;
- owner feedback applies only to its exact scope/lens and has precedence;
- each of at least two agents sees only owner feedback plus its own feedback in
  that scope/lens, before and after restart;
- feedback never changes evidence, retention, readiness, or another context;
- existing lenses migrate into the owner root scope with byte-equivalent legacy
  output and no lost or reinterpreted judgments; historical generic-agent
  events remain isolated to the reserved legacy actor.

### Live and founder proof

The live run remains founder-private and sample-bound. Structural proof reports
counts, states, fingerprints, and blocked claims only. It emits no private
content, URLs, hostnames, queries, labels, excerpts, or resource identities.

Only after every non-founder executable gate passes may a fresh outside agent
use the exact contained candidate skill and CLI for cited recall, hydration,
honest abstention, and bounded retry-safe feedback. Its terminal answer receipt
and staged-state teardown must pass before Randy can record `useful` or
`not_useful`. Only `useful` authorizes exact candidate installation, followed by
post-install smoke or complete rollback. WP-48 and its evidence-deepening
successor remain open until Randy records the cited recall as useful and the
installed proof reproduces it.

## Exclusions

- no Slack OAuth, onboarding, UI, or destination writes;
- no Product Brain runtime/API/key integration;
- no authenticated browser/session/cookie acquisition;
- no blanket full-library, full-page, media, or comment recovery;
- no raw HTTP response archive;
- no synchronous fetch from search, get, or feedback;
- no replacement of canonical personal memory or copying LocalDB;
- no ranking-threshold tuning in the evidence intervention;
- no agent-created scope/lens operation, shared cross-agent aggregation,
  learned actor weighting, or global personalization in this slice;
- no cross-user, production-scale, broad-provider, generalization, autonomy, or
  no-human claim;
- no committed private source fixture or runtime artifact.

## Shape completion gate

Shape is complete when two unchanged five-role reviews agree that:

1. the user job and product-general boundary are correct;
2. successor ownership avoids silently mutating WP-48 or creating a circular
   completion dependency;
3. semantic readiness, deepening, promotion, replay, and migration are coherent;
4. privacy and query/network boundaries fail closed;
5. the evidence-only intervention is executable and falsifiable;
6. shared evidence plus project/lens/agent-scoped derived relevance is coherent,
   isolated, reversible, and does not change query authorization;
7. contained fresh-agent proof before default installation is fail-closed,
   bounded, recoverable, and binds Randy's verdict to the agent's actual cited
   answer or abstention;
8. the next Spec may define exact schemas and tests without reopening product
   direction;
9. Product Brain contains a signed-Shape decision with reserved unique candidate
   identity `MINDLINE-RESOURCE-EVIDENCE-DEEPENING`, a relation to WP-48, and the
   exact artifact/evidence reference. An untracked file or chat review is not
   delivery authority. Existing WP-49 is reconciled only after amended Spec
   Authority passes.
