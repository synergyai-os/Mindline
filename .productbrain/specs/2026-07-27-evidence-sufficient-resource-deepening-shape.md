# Evidence-sufficient resource deepening successor — Shape

Status: proposed after signed Diagnose `D-ES-2`  
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
7. A fresh outside agent uses the unchanged installed workflow to return useful
   cited context, honest missingness, and abstention before Randy judges the
   result.

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
application orchestration port owns the at-most-once queue mutation. Search,
get, and feedback remain read-only and network-free. The same intent is a no-op
when the current attestation already satisfies the logical identity. Promotion
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

Slack source text remains independently searchable and is not downgraded by
resource readiness. Metadata, generic related links, shells, and unverified
legacy artifacts may help describe missingness but cannot authorize a
resource-body claim.

The ranking algorithm, thresholds, model/provider assumptions, and query
network behavior remain unchanged during the intervention.

### 6. Preserve the private-fetch boundary

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

### Live and founder proof

The live run remains founder-private and sample-bound. Structural proof reports
counts, states, fingerprints, and blocked claims only. It emits no private
content, URLs, hostnames, queries, labels, excerpts, or resource identities.

Only after every executable gate passes may the candidate binary and skill be
installed. A fresh outside agent must then use only the installed Mindline
skill and CLI for cited recall, hydration, abstention, and retry-safe feedback.
WP-48 and its evidence-deepening successor remain open until Randy records the
cited recall as useful.

## Exclusions

- no Slack OAuth, onboarding, UI, or destination writes;
- no Product Brain runtime/API/key integration;
- no authenticated browser/session/cookie acquisition;
- no blanket full-library, full-page, media, or comment recovery;
- no raw HTTP response archive;
- no synchronous fetch from search, get, or feedback;
- no replacement of canonical personal memory or copying LocalDB;
- no ranking-threshold tuning in the evidence intervention;
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
6. the next Spec may define exact schemas and tests without reopening product
   direction;
7. Product Brain contains a signed-Shape decision with reserved unique candidate
   identity `MINDLINE-RESOURCE-EVIDENCE-DEEPENING`, a relation to WP-48, and the
   exact artifact/evidence reference. An untracked file or chat review is not
   delivery authority. The work package is materialized only after Spec
   Authority passes.
