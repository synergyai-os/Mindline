# Evidence-sufficient resource deepening successor — signed-Spec candidate

Date: 2026-07-27  
Status: proposed  
Shape authority: DEC-432  
Reserved candidate identity: `MINDLINE-RESOURCE-EVIDENCE-DEEPENING`  
Builds on: WP-48 implementation surfaces for FEAT-27 and FEAT-29  
Unblocks: WP-48 useful-recall evaluation, installation, and founder review

## 1. Outcome and claim boundary

Mindline must preserve every saved capture while distinguishing:

1. whether a public acquisition attempt ended; and
2. whether Mindline has independently validated primary evidence sufficient for
   a named recall capability.

This successor adds capability-scoped evidence deepening for readable public
web bodies, public LinkedIn primary-post bodies, and public media transcripts.
It preserves legacy artifacts, performs all network work asynchronously behind
the existing public-fetch controls, and lets retrieval cite only independently
ready current evidence.

The allowed claim is narrow:

> On Randy's already drained founder-private Slack library and reusable public
> fixtures, bounded evidence deepening improved a frozen selected resource set
> and passed the signed evidence-only held-out recall gate.

It does not prove production, broad-provider, authenticated-source,
cross-user, generalization, autonomy, or no-human behavior.

## 2. Founder journey

1. Randy or an agent searches normally.
2. Search returns cited ready evidence, honest missingness, or a bounded
   `deepening_request` intent. Search performs no fetch and no queue write.
3. Local application orchestration may adopt that intent once under frozen
   budgets.
4. The background worker acquires a bounded public candidate, a separate
   validator evaluates the required capability, and canonical personal memory
   atomically accepts or rejects promotion.
5. The same search can then cite the exact ready artifact, or continue to
   abstain with `insufficient`, `unverified`, or `inaccessible`.
6. Only after executable gates pass does a fresh outside agent use the installed
   Mindline skill/CLI and Randy judge the cited answer as useful or not useful.

Randy never selects provider mechanics, extractor policy, retry counts,
artifact paths, or queue state.

## 3. Architecture and ownership

### 3.1 Canonical authority

`personalmemory.FileRepository` remains the only canonical personal-evidence
authority. The existing
`mindline-personal-evidence-library/v0.4` file remains byte-compatible and is
not migrated in this successor.

FileRepository additionally owns an additive canonical sidecar:

- `resource-evidence.json`;
- `resource-evidence.backup.json`;
- `resource-evidence.recovery.json`;
- `resource-evidence.lock`;
- `evidence-content/`;
- `evidence-validation/`;
- `evidence-staging/`.

The sidecar is ignored safely by prior binaries. Deepened artifacts live in
`evidence-content/`, never the legacy `content/` directory, so a rollback
binary cannot prune or reinterpret them. Rolling back loses access to new
readiness behavior but preserves the v0.4 library, legacy content, new sidecar,
and deepened artifacts unchanged.

`resource-evidence.recovery.json` uses
`mindline-evidence-recovery-journal/v0.1`:

```text
EvidenceRecoveryJournal
  schema_version
  state = acknowledged | replacing
  acknowledged_current_revision
  acknowledged_current_fingerprint
  acknowledged_backup_revision
  acknowledged_backup_fingerprint
  predecessor_revision optional
  predecessor_fingerprint optional
  candidate_revision optional
  candidate_fingerprint optional
  fingerprint
```

In `acknowledged`, current and backup commitments are equal and candidate/
predecessor fields are null. Before replacement, one atomic journal write moves
to `replacing`, preserves the acknowledged predecessor commitments, and records
the exact sealed candidate commitment. After current and mirror readback, one
atomic journal write moves to `acknowledged` for the new equal pair and clears
transition fields. The journal is structural recovery authority, not personal
evidence.

### 3.2 Derived and operational state

`resourcedeepening` owns derived scheduling, leases, workers, recovery
coordination, and structural proof. Canonical refresh allocations and stage
commitments stay in the FileRepository sidecar together with canonical
structural allocations, outcomes, attempt/request ordinals, and consumed-budget
authority. Queue loss
may be rebuilt from open canonical allocations and stage journal; it never
becomes evidence, idempotency, terminality, or budget authority.

### 3.3 Provider mechanics

`resourcefetch` retains the public HTTP trust boundary and exposes a safe,
bounded fetch port. `resourceadapters` owns provider-specific candidate
extraction behind one capability-neutral interface. Core queue, repository,
retrieval, and evaluation packages do not branch on provider names.

Package-neutral `internal/resourcebudget` defines
`RequestBudgetLedgerPort` and structural request/attempt types. `resourcefetch`
depends only on that port, never on `personalmemory` or catalog types.
`deepeningorchestrator` injects a FileRepository-backed implementation for a
canonical attempt context.

### 3.4 Readiness authority

`evidencereadiness` owns immutable capability contracts and validation. An
adapter may emit a candidate but cannot set `ready`. The validator consumes the
candidate envelope and acquisition facts, recomputes the contract, and emits a
`ValidationResult`. FileRepository alone combines that result with immutable
promotion context and creates the attestation.

### 3.5 Retrieval and orchestration

`personalmemory` retrieval remains read-only. It may return one structural
deepening intent. `deepeningorchestrator` is the only application port allowed
to turn that intent into a derived queue mutation. Search, get, feedback, and
evaluation perform zero network work.

## 4. Canonical evidence schemas

### 4.1 Evidence catalog

Schema: `mindline-resource-evidence-catalog/v0.1`

```text
EvidenceCatalog
  schema_version
  revision
  source_library_stable_commitment
  heads[] sorted by resource_id, capability
  revisions[] sorted by revision_id
  refresh_allocations[] sorted by resource_id, capability, policy, retry_event_id
  stage_journal[] sorted by attempt_id
  deepening_runs[] sorted by run_id
  allocations[] sorted by logical_job_id
  attempts[] sorted by attempt_id
  request_executions[] sorted by request_execution_id
  job_outcomes[] sorted by logical_job_id
  promotion_receipts[] sorted by accepted_at, receipt_id
  fingerprint
```

Limits:

- at most the existing `MaximumResources` heads per capability;
- at most 8 capability heads per resource;
- at most 16 retained catalog revisions per resource/capability in this slice;
- serialized catalog at most 64 MiB;
- evidence-content repository at most the existing frozen owner-only storage
  budget extended by an explicit deepening allocation in the run profile.

`source_library_stable_commitment` is recomputed from the v0.4 capture
denominator and stable source facts. It includes current and historical capture
identity/content commitments, resource occurrence IDs, tombstones, and counts.
It excludes mutable resource state, metadata, excerpts, missingness, content
artifact hashes, enrichment receipts, and evidence-catalog values.

### 4.2 Evidence head

```text
EvidenceHead
  resource_id
  capability
  refresh_generation
  extractor_adapter_id
  extractor_policy_fingerprint
  readiness
  fixed_missingness[]
  current_artifact
  attestation
  retrieved_at
  quality
  content_hash
```

Allowed capabilities:

- `readable_body/v1`;
- `primary_post/v1`;
- `transcript/v1`.

Allowed readiness:

- `ready`: independently validated current primary evidence;
- `insufficient`: safe candidate exists but fails the capability contract;
- `unverified`: legacy or candidate evidence has not passed the current
  validator;
- `inaccessible`: policy-safe acquisition cannot obtain a candidate under the
  terminal fixed reason.

State invariants:

| Readiness | Artifact | Validation envelope | Attestation | Authority path |
| --- | --- | --- | --- | --- |
| `unverified` | optional legacy base reference | absent | absent | bootstrap only |
| `inaccessible` | absent | absent | required | trusted mapper over allowlisted fixed acquisition outcome |
| `insufficient` | required | required | required | capability validator |
| `ready` | required | required | required | capability validator |

Every other field matrix is invalid. A trusted inaccessible mapper lives in
`evidencereadiness`, performs no network work, accepts only a fixed allowlisted
safe-fetch outcome, and produces a validation result that the repository binds
into an attestation. An adapter cannot directly write any state.
`unverified|inaccessible` use the all-zero quality vector.

Readiness ordering for promotion is:

`unverified < inaccessible < insufficient < ready`

The v1 quality vector is:

```text
EvidenceQuality
  not_truncated            0 | 1
  structural_source_rank   integer 0..3
  inverse_boilerplate_permille integer 0..1000
  coverage_units           integer >= 0
  normalized_runes         integer >= 0
```

Comparison is lexicographic in the field order above after readiness rank.
`inverse_boilerplate_permille = 1000 - boilerplate_permille`. All values are
recomputed by the validator and bounded by the artifact. Capability-specific
structural ranks and coverage units are frozen in §5.4.

Promotion rules:

1. a higher readiness rank may replace a lower rank;
2. a lower readiness rank never replaces a higher rank;
3. at equal readiness, a lower quality vector never replaces current;
4. identical artifact plus identical attestation is an exact no-op;
5. a different artifact at equal quality is accepted only under a higher
   authorized refresh generation or a different extractor-policy fingerprint;
6. a different artifact at higher quality may be accepted within the same
   logical generation only when it is the result of recovery for the same
   attempt;
7. truncation can never replace non-truncated evidence at equal readiness;
8. the 17th predecessor fails closed with
   `evidence_revision_limit_reached`; no predecessor is compacted or deleted.

### 4.3 Evidence artifact reference

The sidecar reuses the structural fields of `ContentArtifactRef` but freezes a
new storage class:

`owner_only_evidence_content_addressed_file`

Artifact identity is `evidence-content-` plus SHA-256 of the final bounded,
normalized, secret-redacted, STD-20-sanitized text. Artifact files are immutable
UTF-8 text at `0600` beneath a `0700` contained directory. The existing maximum
extracted-content byte/rune limits apply.

### 4.4 Evidence revision

```text
EvidenceRevision
  revision_id
  superseded_at
  prior_head
```

`revision_id` is deterministic from the complete prior-head canonical
commitment, not only artifact content.
Identical promotion replay creates no revision. A predecessor remains loadable
after refresh, extractor-policy upgrade, rollback, failed promotion, and queue
rebuild.

### 4.5 Readiness attestation

Schema: `mindline-evidence-readiness-attestation/v0.1`

```text
ReadinessAttestation
  attestation_id
  resource_id
  capability
  readiness
  fixed_reasons[]
  artifact_id
  artifact_sha256
  candidate_envelope_id
  candidate_envelope_sha256
  candidate_envelope_bytes
  extractor_adapter_id
  extractor_policy_fingerprint
  validator_id
  validator_policy_fingerprint
  quality
  source_response_digest
  base_artifact_id
  predecessor_artifact_id
  refresh_generation
  retrieved_at
  validation_evidence_commitment
```

The attestation ID uses the exact `mindline-readiness-attestation/v1`
projection in §11.1.
`ready|insufficient` require exact artifact and candidate-envelope IDs, SHAs,
and byte lengths. `inaccessible` has neither. `unverified` cannot be emitted by
a v1 validator; it is bootstrap state only.

`source_response_digest` is computed only from the final bounded normalized
candidate after secret redaction and STD-20 sanitization. Every lexical URL is
replaced by an encounter-order ordinal token before hashing. Raw response
bytes, private values, secret values, URL values, hostnames, headers, cookies,
and errors never influence durable digest input. The domain is
`mindline-source-response/v1`.

### 4.6 Promotion receipt

```text
EvidencePromotionReceipt
  schema_version = mindline-evidence-promotion-receipt/v0.1
  receipt_id
  logical_job_id
  attempt_id
  resource_id
  capability
  prior_head_commitment
  accepted_head_commitment
  created_artifact
  created_revision
  source_library_stable_commitment
  accepted_at
```

An exact replay returns the same receipt and adds zero artifacts, heads,
revisions, catalog revisions, or current-state changes.

### 4.7 Canonical refresh allocation

```text
RefreshAllocation
  resource_id
  capability
  extractor_policy_fingerprint
  retry_event_id
  refresh_generation
  actor_class = local_operator
  authorization_surface = cli_resources_deepen
  authorized_at
  allocation_commitment
```

Refresh allocation is canonical idempotency authority in the evidence catalog,
not in the rebuildable queue. Queue rebuild projects every allocated generation
that is not canonically satisfied or terminally accounted for.

### 4.8 Canonical stage journal

```text
EvidenceStage
  attempt_id
  logical_job_id
  resource_id
  capability
  candidate_envelope_id
  candidate_envelope_sha256
  candidate_envelope_bytes
  artifact_id
  artifact_sha256
  artifact_bytes
  storage_state = staged | adopted | rejected
  stage_commitment
```

The journal contains no body, URL, hostname, query, label, excerpt, or raw
error. Before promotion, two files are written content-addressed beneath
owner-only `evidence-staging/`:

1. strict-bounded canonical
   `mindline-evidence-candidate-envelope/v0.1` containing every sanitized
   validator input: ordered blocks, markers, truncation, fixed missingness,
   media type, retrieved time, usage, source-response digest, extractor
   identity/policy, and candidate commitment;
2. the exact final normalized artifact text produced by validation.

The structural stage commits both files and is atomically persisted. Recovery
reloads the complete candidate envelope, reruns the validator, and requires
byte-identical final text, quality, reasons, digest, candidate commitment, and
validation commitment before adoption. Missing/tampered markers, block order,
truncation, usage, digest, envelope, or final artifact fail closed.

Recovery deterministically adopts a valid stage or marks it rejected and
reconciles both files into storage budgets. Unjournaled staging files are
orphans: they are never adopted, are counted before bounded cleanup, and are
removed only after contained-path/hash checks. A changed remote response after
crash cannot replace an already valid stage for the same attempt.

On adoption, the candidate envelope moves content-addressed to
`evidence-validation/` and the final text to `evidence-content/`. Every current
and predecessor `ready|insufficient` head remains transitively bound to and
requires both immutable files. Catalog, backup, revision, citation, and
readiness validation fail closed if either is missing or tampered. Cleanup
cannot delete an envelope referenced by a current head or predecessor revision.

### 4.9 Canonical deepening control ledger

Every logical job, including generation zero, mandatory transition/nonregression
work, adopted retrieval intent, and backfill selection, receives a canonical
`DeepeningAllocation` before queue projection:

```text
DeepeningAllocation
  run_id
  logical_job_id
  allocation_source = transition | nonregression | intent | backfill | refresh
  resource_id
  capability
  extractor_policy_fingerprint
  refresh_generation
  base_artifact_id
  state = allocated | terminal
  allocation_commitment
```

Every terminal allocation has exactly one canonical `JobOutcomeReceipt`:

```text
JobOutcomeReceipt
  logical_job_id
  terminal_outcome
  fixed_reason
  accepted_head_commitment optional
  current_head_commitment optional
  outcome_commitment
```

`unchanged_stronger_current` includes same-artifact refresh. It leaves the
current evidence head, attestation, artifact, validation envelope, evidence
revisions, and promotion receipts unchanged. The same one-time transaction
changes only control-ledger allocation state, job outcome, consumed counters,
catalog revision/fingerprint, backup mirror, and recovery acknowledgement.

The catalog also stores:

```text
DeepeningRun
  run_id
  frozen_profile
  counters
    allocated_resources
    attempts
    request_executions
    reserved_wire_bytes
    settled_wire_bytes
    reserved_decoded_bytes
    settled_decoded_bytes
    extracted_bytes
    evidence_storage_bytes
    staging_storage_bytes
    reserved_wall_millis
    settled_wall_millis
  state = open | closed
  run_commitment

JobAttempt
  attempt_id
  logical_job_id
  attempt_ordinal
  state
  attempt_commitment

RequestExecution
  request_execution_id
  attempt_id
  request_ordinal
  reserved_request_count
  reserved_wire_bytes
  reserved_decoded_bytes
  reserved_wall_millis
  settled_wire_bytes optional
  settled_decoded_bytes optional
  settled_wall_millis optional
  state = reserved | settled | ambiguous
  execution_commitment
```

This ledger is structural only and is persisted under the evidence-catalog lock
before queue/network action. Queue deletion or corruption never loses selected
generation-zero work, terminal blocked/no-op outcomes, attempt ordinals,
request ordinals, or consumed budgets. Queue rebuild projects only open
unsatisfied allocations. It never refetches a terminal allocation.

## 5. Candidate and validator contracts

### 5.1 Candidate

Schema: `mindline-evidence-candidate/v0.1`

```text
EvidenceCandidate
  resource_id
  capability
  extractor_adapter_id
  extractor_policy_fingerprint
  media_type
  blocks[]
    kind
    structural_marker
    text
    optional_locator
  response_truncated
  fixed_missingness[]
  retrieved_at
  source_response_digest
  usage
```

Candidate blocks are bounded, ordered structural parse output. They retain only
text needed by the capability validator; scripts, styles, headers, raw HTML,
and network metadata are absent. The adapter candidate remains in memory. The
validator's final normalized text may enter the owner-only staged-artifact
journal before promotion. A failed or inaccessible candidate is not written as
a private body; a rejected stage follows the bounded journal cleanup contract.
Only structural outcome and budget counters may enter the derived queue.

V1 permits at most 2,048 blocks, 64 KiB UTF-8 per block, and 512 KiB UTF-8
across all blocks before the lower run-profile cap is applied.

### 5.2 Adapter port

```text
Acquire(ctx, SafePublicFetcher, TransientResource, Capability, FrozenPolicy)
  -> EvidenceCandidate | FixedInaccessibleOutcome
```

`TransientResource` is resolved from canonical memory immediately before the
attempt. It may contain the canonical URL in process memory but may not be
persisted in the queue, attempt receipt, logs, errors, or proof.

Every adapter-originated secondary request—including a caption track, embed,
structured-data URL, or redirect-like provider reference—is a fresh
`SafePublicFetcher` call. It passes STD-20 classification before durable
identity or request construction and repeats scheme, userinfo, all-answer DNS,
rebinding, redirect, pinned-peer, TLS host, MIME, decompression, byte, timeout,
request, provider, and run-budget enforcement. No header, cookie,
authorization, proxy state, or credential is inherited from the primary
request. A rejected secondary target causes zero network requests and emits
only a fixed structural reason.

### 5.3 Validator port

```text
Validate(EvidenceCandidate, CapabilityContract)
  -> ValidationResult
```

The validator:

- does no network work;
- deterministically selects primary blocks and constructs the final normalized
  artifact text from the bounded candidate;
- hashes the exact final artifact bytes it evaluates;
- uses a frozen validator-policy fingerprint;
- emits fixed reason codes only;
- validates structural evidence and shell/boilerplate exclusions;
- never reads retrieval output, expected held-out sources, or private labels.

`ValidationResult` contains only capability, readiness, fixed reasons, quality,
final normalized artifact text/reference for `insufficient|ready`, candidate
commitment, validator ID, validator-policy fingerprint, and
validation-evidence commitment.

The repository—not the validator—builds the complete attestation from the
validation result plus immutable `PromotionContext`:

```text
PromotionContext
  resource_id
  logical_job_id
  attempt_id
  base_artifact_id
  predecessor_artifact_id
  extractor_adapter_id
  extractor_policy_fingerprint
  refresh_generation
  retrieved_at
```

The trusted inaccessible mapper produces the same `ValidationResult` shape from
an allowlisted `FixedInaccessibleOutcome`; it cannot accept adapter-authored
free text or raw errors.

### 5.4 Capability contracts

#### `readable_body/v1`

Candidate source:

- semantic `article`, `main`, or role-main body; then
- deterministic readability scoring over block density when semantic body is
  absent.

Ready requires:

- valid bounded UTF-8;
- selected primary-body structural marker;
- at least 200 non-whitespace runes;
- at least 3 non-navigation text blocks or one semantic article/main block;
- boilerplate ratio below the frozen fixture-derived maximum;
- no shell reason.

Fixed shell reasons include:

- `login_required`;
- `consent_only`;
- `client_render_shell`;
- `loading_shell`;
- `navigation_only`;
- `body_below_minimum`;
- `body_truncated_before_primary_context`.

For v1, `boilerplate_permille` must be at most `600`. Structural source rank is
3 for an explicit semantic `article`, 2 for explicit `main`/role-main, 1 for a
readability-selected block set, and 0 otherwise. Coverage units are retained
non-navigation primary text blocks.

For every capability,
`boilerplate_permille = floor(1000 * boilerplate_non_whitespace_runes /
max(1, all_candidate_non_whitespace_runes))`. The validator classifies blocks
by the frozen capability rules; adapter-proposed kind names alone do not decide
the classification.

#### `primary_post/v1`

Candidate source:

- public structured post/article body;
- embedded public structured data tied to the requested post;
- public server-rendered primary post container.

Ready requires:

- explicit primary-post structural marker;
- valid bounded UTF-8;
- at least 40 non-whitespace runes;
- body is not only author/date/engagement metadata;
- no login/consent/client-shell reason.

Comments use a future separate capability. Missing comments cannot lower
`primary_post/v1` readiness.

Structural source rank is 3 for a public structured post object tied to the
requested post, 2 for an explicit server-rendered primary-post container, 1 for
an explicit semantic article body tied to the post, and 0 otherwise. Coverage
units are retained primary-post text blocks. Boilerplate permille must be at
most `300`.

#### `transcript/v1`

Candidate source:

- public caption/transcript track explicitly associated with the requested
  media resource;
- public server-provided timed transcript segments.

Ready requires:

- transcript/caption structural marker;
- valid bounded UTF-8;
- at least 120 non-whitespace runes;
- at least 3 timed segments or an explicit complete transcript body;
- page title, description, or player shell alone is rejected.

Fixed reasons include `transcript_unavailable`,
`transcript_language_unsupported`, `transcript_empty`, and
`transcript_truncated_before_context`.

Structural source rank is 3 for timed caption/transcript segments explicitly
tied to the requested media, 2 for an explicit complete transcript body, and 0
otherwise. Coverage units are timed segments, or transcript paragraphs when an
explicit complete body is used. Boilerplate permille must be at most `100`.

## 6. Queue, identity, replay, and refresh

### 6.1 Queue schema

Schema: `mindline-resource-deepening-queue/v0.1`

```text
DeepeningQueue
  schema_version
  profile
  projected_counters
  items[] sorted by logical_job_id
  fingerprint
```

Queue profile/counters must equal the open canonical run projection. A mismatch
fails closed and rebuilds without network continuation until canonical readback
succeeds.

An item contains structural identifiers only:

```text
DeepeningItem
  logical_job_id
  resource_id
  capability
  extractor_policy_fingerprint
  refresh_generation
  base_artifact_id
  state
  terminal_outcome
  fixed_reason
  next_attempt_ordinal
  attempts
  reserved_requests
```

No URL, hostname, title, author, query, excerpt, body, label, header, cursor,
credential, cookie, raw error, or source identity beyond the opaque resource ID
may enter the queue.

### 6.2 States and outcomes

Work state:

- `queued`;
- `processing`;
- `terminal`.

Terminal outcome:

- `promoted_ready`;
- `promoted_insufficient`;
- `unchanged_stronger_current`;
- `inaccessible`;
- `blocked`.

Attempt terminality never derives readiness. Only the canonical current
attestation does.

### 6.3 Identities

Logical job ID:

```text
"deepening-job-" +
hex(SHA256("mindline-logical-job-id/v1" + 0x00 +
  canonical_json({
    resource_id,
    capability,
    extractor_policy_fingerprint,
    refresh_generation
  })))
```

Attempt ID:

```text
"deepening-attempt-" +
hex(SHA256("mindline-attempt-id/v1" + 0x00 +
  canonical_json({logical_job_id, base_artifact_id, attempt_ordinal})))
```

Request execution ID:

```text
"deepening-request-" +
hex(SHA256("mindline-request-execution-id/v1" + 0x00 +
  canonical_json({attempt_id, request_ordinal})))
```

Candidate-envelope ID is
`"evidence-candidate-" + candidate_envelope_sha256`, where the SHA uses
`mindline-evidence-candidate/v1`. Deepened artifact ID remains
`"evidence-content-" + artifact_sha256`.

Run ID and refresh retry event ID are independently minted RFC 4122 UUIDv4
values. They are never derived from private data. Identity projections exclude
the resulting ID itself; later commitments may include the already derived ID
without circularity.

Default refresh generation is zero. The current satisfying attestation makes
enqueue for the same logical job a no-op.

### 6.4 Refresh retry identity

Explicit refresh is a sensitive local-operator action exposed only by
`mindline resources deepen --refresh --retry-id <uuid>`. Retrieval, search, get,
feedback, adapters, and background workers cannot authorize it.

The actor class is fixed to `local_operator` inside the cooperative OS-user
boundary and is recorded in the canonical allocation. No broader authentication
claim is made.

`retry-id` is a non-secret UUIDv4 event identity used only for exact replay. It
does not authorize refresh; successful invocation of the local-operator CLI
surface is the authorization boundary. It may appear in argv and structural
audit output because it grants no capability by itself. The CLI generates a
UUIDv4 when first requested and returns the non-secret allocation receipt and
retry ID. An operator that needs crash-safe replay supplies the same ID. Invalid
UUID/version/variant or oversized input fails before allocation.

Under legacy-library then catalog lock:

1. replay of a known retry event ID returns its existing generation;
2. a new locally authorized retry event ID allocates current maximum generation
   plus one;
3. allocation and its actor/surface audit evidence persist canonically before
   queue projection;
4. allocation fails when refresh/run/request budgets are exhausted;
5. one retry event ID can never map to two generations, including
   concurrency/restart;
6. promotion under generation N does not enqueue generation N or N+1;
7. queue loss rebuilds the same unsatisfied allocated generation from catalog.

### 6.5 At-least-once acquisition and exact effects

Before one adapter `Acquire`, orchestration atomically allocates one
`JobAttempt` and debits the attempt cap. The attempt may perform multiple public
request executions for primary, redirect, caption, embed, or other secondary
requests.

The injected port is:

```text
BeginAttempt(run_id, logical_job_id) -> AttemptContext
ReserveRequest(attempt_context, request_ordinal, maximum_envelope)
  -> RequestReservation
SettleRequest(request_reservation, bounded_actual_usage)
  -> SettledRequest
MarkAmbiguous(request_reservation)
```

Every method returns only after canonical sidecar readback. Inputs and outputs
are structural IDs/counters; no URL, hostname, header, body, provider, or
personal-memory type crosses the port. Reservation/readback failure prevents
the send. `resourcefetch` assigns a fresh request ordinal for primary,
redirect, and every secondary request through the injected attempt context.

Before each request that may reach the network, `SafePublicFetcher` atomically
allocates a new request ordinal and conservatively reserves:

- one request count;
- the maximum permitted wire bytes for that request bounded by aggregate
  remainder;
- the maximum permitted decoded bytes bounded by aggregate remainder;
- the maximum permitted timeout/wall milliseconds bounded by aggregate
  remainder.

No send occurs unless canonical reservation readback succeeds. An ambiguous
reservation is never refunded. Extracted/staged bytes are separately reserved
before owner-only stage creation.

- crash after request reservation but before send: the reservation remains
  consumed; recovery starts a new job attempt and request ordinal;
- crash after response but before staged acknowledgement: recovery may repeat
  acquisition only through a new job attempt and freshly reserved request
  ordinals;
- repeated ambiguous crashes may repeat acquisition only while persisted
  attempt/request budgets remain;
- crash after staged artifact: recovery validates/reuses the stage;
- crash after promotion: canonical readback returns the existing receipt.

Observed outbound request count can never exceed canonical request reservations
or frozen caps. Redirects and secondary requests consume separate ordinals.
Successful executions durably settle unused wire/decoded/time reservation only
after bounded response accounting; ambiguous executions retain their full
conservative reservation. Candidate extraction cannot exceed the remaining
extracted/staging reservation. Canonical accepted artifact, attestation, head,
revision, catalog revision, and receipt effects are exactly once.

### 6.6 Founder-private progressive sequence and caps

The live sequence is fixed:

1. deepen the six precommitted insufficient resources;
2. stop and run the evidence-transition gate;
3. if fewer than three become independently ready, do not backfill or evaluate;
4. if the gate passes, deepen every precommitted currently matched case whose
   match depends on resource evidence;
5. run a deterministic capped backfill;
6. stop and seal before held-out evaluation.

Backfill priority is:

1. valid orchestration-adopted pending intents, oldest first;
2. remaining unverified resources by retained current occurrence count
   descending;
3. latest retained occurrence time descending;
4. opaque resource ID ascending.

Private labels, expected held-out sources, candidate retrieval output, provider
name, and destination/lens judgments never affect priority.

Founder-PoC caps across steps 1, 4, and 5:

- at most 48 distinct resources;
- at most 64 charged public request executions;
- at most 64 attempts;
- at most 4 global and 1 per-host concurrent executions;
- 64 MiB wire, 96 MiB decoded, 24 MiB extracted, and 32 MiB new
  evidence/staging storage;
- at most 30 minutes wall time;
- no explicit refresh generations during automatic backfill.

Exhaustion terminalizes the remaining selected work structurally and blocks the
useful-recall claim; restart does not reset any cap.

## 7. Canonical promotion transaction

`PromoteEvidence` acquires locks in fixed order:

1. legacy library lock;
2. evidence-catalog lock.

It then:

1. loads and validates the v0.4 library and catalog;
2. recomputes stable source commitment;
3. verifies the resource remains reachable from a current or historical
   retained capture;
4. verifies logical/attempt identity and current base artifact;
5. reloads the exact staged candidate bytes and validates its journal
   commitment;
6. reruns the validator, constructs the repository-owned attestation from
   immutable promotion context, and compares all commitments;
7. applies stronger-only/current-policy/current-generation rules;
8. makes both the content-addressed validation envelope and adopted artifact
   valid at their final owner-only paths while the prior catalog still records
   the stage as `staged`;
9. adds at most one predecessor revision or fails closed at the revision cap;
10. builds one sealed next catalog containing the accepted head, predecessor
    revision, promotion receipt, terminal job outcome, and the same stage marked
    `adopted`;
11. atomically persists and reads back that catalog once, then returns the
    deterministic receipt;
12. for rejection, builds and persists one next catalog containing the terminal
    rejection/no-op outcome and stage state `rejected`, with no head mutation;
13. performs bounded orphan cleanup only after canonical readback.

Lock order is used by every catalog mutation. Legacy v0.4 enrichment never
takes the catalog lock and never visits `evidence-content/`.

If the process crashes after the adopted artifact file is valid but before the
one catalog commit, the prior catalog still has a `staged` journal entry.
Recovery validates the candidate envelope and final artifact, reruns validation,
and either completes the same one-catalog transition or rejects it. The physical
file alone never becomes current authority.

### 7.1 Artifact-free inaccessible transaction

`RecordInaccessible` uses the same lock order without a stage:

1. loads and validates library, catalog, allocation, attempt, and fixed
   acquisition outcome;
2. passes only an allowlisted structural outcome to the trusted
   `evidencereadiness` inaccessible mapper;
3. FileRepository constructs an artifact-free attestation from the mapper
   result and immutable promotion context;
4. applies the same readiness/quality stronger-current rules;
5. atomically writes one next catalog containing either:
   - an `inaccessible` head/attestation, predecessor revision when allowed, and
     terminal outcome; or
   - only `unchanged_stronger_current` when current evidence is stronger;
6. reads back once and returns the deterministic outcome receipt.

An inaccessible outcome never replaces current `insufficient|ready` evidence,
contains no body/artifact/envelope, and never accepts adapter free text or raw
errors.

### 7.2 Catalog persistence and backup recovery

Bootstrap, refresh allocation, stage journaling, promotion, rejection, and
recovery all use the same legacy-library then catalog lock order, strict bounded
decode, sealed fingerprint validation, atomic replacement, fsync/readback, and
reference reachability checks.

`resource-evidence.backup.json` is a verified mirror, not a normal predecessor.
For transition N:

1. recovery journal is `acknowledged` for equal current/backup N-1;
2. journal atomically moves to `replacing`, preserving predecessor N-1 and
   recording exact candidate N commitment;
3. atomic replace writes candidate current N while retaining N-1 backup;
4. current N is strict-decoded and read back;
5. an exact verified copy of current N atomically replaces backup;
6. backup N is strict-decoded and read back;
7. recovery journal atomically moves to `acknowledged` for equal current/backup
   N and clears predecessor/candidate transition fields.

If a crash occurs before step 7, a valid current N is used and the mirror/journal
sequence finishes. If current N is invalid before acknowledgement, only the
journaled N-1 backup may be restored because N was never acknowledged. After
step 7, current or its equal recorded backup N may recover the other without
rollback.

Backup may become recovery input only when:

1. current catalog is missing or fails strict decode/fingerprint validation;
2. backup passes strict schema/fingerprint validation;
3. backup stable-source commitment matches the current v0.4 library;
4. every referenced legacy/current/adopted artifact and every journal entry in
   `staged` state exists and validates; rejected stages may be absent;
5. backup exactly matches either the recovery journal's last acknowledged
   current/backup commitment, or the journaled predecessor during an
   unacknowledged in-progress replacement;
6. one locked recovery transaction atomically restores, reads back, and seals
   current before returning it.

A stale, unrecorded predecessor, source-mismatched, reference-missing, corrupt,
or weaker backup is rejected. If current and backup are both invalid, evidence deepening fails
closed without changing the v0.4 library or any artifact.

## 8. Bootstrap and rollback

### 8.1 Provider-neutral bootstrap

FileRepository never infers provider or capability from hostname. Application
orchestration builds a bounded
`mindline-evidence-bootstrap-batch/v0.1` through the source-neutral adapter
capability resolver:

```text
EvidenceBootstrapBatch
  schema_version
  source_library_stable_commitment
  items[] sorted by resource_id, capability
  fingerprint

EvidenceBootstrapItem
  resource_id
  capability
  legacy_base_artifact_id optional
```

The resolver receives a transient canonical resource, applies STD-20 before
classification, and returns only an allowlisted capability or an
unsafe/ambiguous no-item outcome. It emits no hostname/provider field to
FileRepository.

Under the normal legacy-library then catalog lock order, FileRepository:

1. strict-validates the complete bounded batch;
2. recomputes stable source commitment;
3. verifies every resource is reachable from retained current/history evidence;
4. validates each optional legacy base artifact;
5. creates one `unverified` head without a readiness attestation;
6. atomically persists and reads back the complete catalog old-or-new.

No partial bootstrap is visible. Exact replay changes nothing. Conflicting
capability/base projection fails before persistence. Unsafe or ambiguous
classification creates no head and no durable classification fact. A later
bounded resolver pass may add a previously absent head through a new complete
bootstrap batch.

### 8.2 Rollback

Rollback restores the prior binary and skill only. It does not delete or rewrite
the v0.4 library, legacy content, evidence catalog, evidence content, queue, or
validation envelopes, staging, or agent state. The prior binary must pass
status/search/get over the unchanged
v0.4 library. Reinstalling the candidate reads back the unchanged evidence
catalog and resumes exactly.

## 9. Retrieval contract

### 9.1 Ready evidence

Resource-derived search documents are indexed only from the current evidence
head when:

- readiness is `ready`;
- attestation hashes the exact current artifact;
- current resource/capability/policy/generation match;
- canonical readback succeeds;
- artifact, validation envelope, and attestation validate;
- validator rerun over the retained envelope reproduces the cited final
  artifact, quality, reasons, and validation commitment.

Legacy resource content, metadata-only context, generic related URLs, and
unverified/insufficient/inaccessible heads cannot authorize a resource-body
answer. Slack capture text remains independently searchable.

### 9.2 Citation commitment

Every resource-derived citation adds:

```text
resource_id
artifact_id
artifact_sha256
candidate_envelope_id
candidate_envelope_sha256
capability
attestation_id
evidence_head_commitment
```

Normal compact output remains bounded and contains no full body, path, URL,
hostname, or private label. Explicit get hydrates only the selected current
ready artifact and repeats the same commitments.

### 9.3 Deepening request intent

When a selected relevant resource lacks ready required evidence, retrieval may
return at most one:

```text
DeepeningRequest
  schema_version = mindline-deepening-request/v0.1
  resource_id
  capability
  extractor_policy_fingerprint
  refresh_generation = 0
  base_artifact_id
  request_fingerprint
```

Retrieval does not persist the request. Orchestration validates canonical
readback, budgets, existing satisfaction, and duplicate queue identity before
atomically creating the canonical generation-zero allocation; only then does it
project/enqueue derived work. Feedback cannot create a refresh generation.

## 10. Privacy and safety

All existing WP-48 fetch controls remain mandatory:

- STD-20 classification before identity, hashing, logging, or fetch;
- public HTTP(S) only;
- no userinfo, ambient authorization, cookies, proxy state, browser session, or
  inherited headers;
- all-answer DNS validation, redirect revalidation, DNS-rebinding defense,
  pinned public peer, TLS-host preservation, and peer verification;
- MIME, wire, decoded, extracted, redirect, timeout, retry, concurrency,
  provider, storage, refresh, attempt, run, and wall-time budgets;
- owner-only roots/files, containment, symlink rejection, strict bounded JSON,
  atomic replacement, and protected-path checks;
- structural fixed reasons and counts only on emitted surfaces.

Canonical primary evidence and every derivative—summaries, snippets, embeddings,
search projections, readiness evidence, labels, and proof excerpts—remain
owner-only. They cannot enter hosted inference, telemetry, Product Brain, git,
public fixtures, logs, or reports without separately governed opt-in.

## 11. Evaluation authority

### 11.1 Canonical commitment primitives

Every v1 commitment is:

```text
lowercase_hex(SHA256(utf8(domain_prefix) + 0x00 + canonical_json))
```

Canonical JSON uses UTF-8, lexicographically sorted object keys, no insignificant
whitespace, decimal integers, JSON `true|false|null`, minimal JSON string
escaping, no HTML escaping, and arrays sorted by the key stated below. Strings
are hashed as stored without Unicode normalization. Every optional schema field
is present as explicit JSON `null` in commitment projections; absent and null
are never two encodings. Fixed reasons and missingness arrays are sorted
lexicographically and deduplicated. Candidate blocks preserve source order.
Catalog collections use the sort keys in §4.1. Each projection has public
mutation tests for every included and excluded field.

Domain prefixes and projections:

- `mindline-stable-source/v1`
  - library schema;
  - current capture rows sorted by idempotency key:
    `record_id, idempotency_key, content_hash, edit_delete_state,
    sorted(resource_ids)`;
  - capture revision rows sorted by revision ID with the same fields;
  - tombstone/current/history counts;
  - excludes resource metadata/state/content/missingness, enrichment imports,
    resource revisions, evidence catalog, and queue;
- `mindline-source-occurrence/v1`
  - `record_id, resource_id`;
  - one occurrence identity per unique resource reference on a capture;
- `mindline-evidence-artifact/v1`
  - all evidence artifact reference fields;
- `mindline-source-response/v1`
  - final bounded normalized, secret-redacted, STD-20-sanitized candidate text
    with lexical URLs replaced by encounter-order ordinal tokens;
- `mindline-evidence-head/v1`
  - every EvidenceHead field, with the attestation represented by its complete
    attestation commitment and `content_hash` blanked; resulting commitment is
    stored as `content_hash` and is the head commitment;
- `mindline-evidence-candidate/v1`
  - every bounded candidate-envelope field, with blocks in source order and
    commitment blanked;
- `mindline-validation-result/v1`
  - capability, readiness, fixed reasons, quality, final-artifact commitment,
    candidate commitment, validator identity/policy, and commitment blanked;
- `mindline-readiness-attestation/v1`
  - every attestation field with `attestation_id` blanked;
- `mindline-validation-evidence/v1`
  - capability contract/policy, candidate commitment, selected block ordinals,
    quality inputs/result, reasons, and final-artifact commitment;
- `mindline-promotion-receipt/v1`
  - every promotion-receipt field with `receipt_id` blanked;
- `mindline-refresh-allocation/v1`
  - every canonical refresh-allocation field with allocation commitment
    blanked;
- `mindline-evidence-stage/v1`
  - every stage-journal field with stage commitment blanked;
- `mindline-deepening-allocation/v1`
  - every DeepeningAllocation field with allocation commitment blanked;
- `mindline-job-outcome/v1`
  - every JobOutcomeReceipt field with outcome commitment blanked;
- `mindline-deepening-run/v1`
  - every DeepeningRun field with run commitment blanked;
- `mindline-job-attempt/v1`
  - every JobAttempt field with attempt commitment blanked;
- `mindline-request-execution/v1`
  - every RequestExecution field with execution commitment blanked;
- `mindline-evidence-catalog/v1`
  - every catalog field and collection in §4.1, with catalog fingerprint
    blanked;
- `mindline-evidence-recovery/v1`
  - current and backup revision/fingerprint commitments plus transition state,
    with journal fingerprint blanked;
- `mindline-prebuild-authority/v1`
  - every pre-build receipt field with receipt fingerprint blanked;
- `mindline-logical-job-id/v1`
  - resource ID, capability, extractor-policy fingerprint, refresh generation;
- `mindline-attempt-id/v1`
  - logical job ID, base artifact ID as explicit null or string, attempt ordinal;
- `mindline-request-execution-id/v1`
  - attempt ID and request ordinal;
- `mindline-deepening-request/v1`
  - every DeepeningRequest field except `request_fingerprint`;
- `mindline-private-manifest/v1`
  - schema, reviewer binding, frozen case rows sorted by opaque case ID,
    queries, labels, expected stable-source commitments, and case status;
- `mindline-evidence-transition-manifest/v1`
  - schema, independent reviewer binding, rows sorted by opaque transition case
    ID, each containing resource ID, capability, base-artifact commitment,
    initial readiness, initial sufficiency label, and fixed target predicate;
- `mindline-slack-source-behavior/v1`
  - rows sorted by opaque case ID for cases answered from Slack source text:
    `case_id, answered, expected_source_commitment, matched_source_commitment,
    rank, citation_valid`;
- `mindline-evidence-intervention/v1`
  - every structural run-binding field except its own fingerprint.

Prior/accepted head commitments use `mindline-evidence-head/v1`. Semantic
manifest fingerprints use `mindline-private-manifest/v1`. Catalog fingerprints
commit the complete catalog projection with its fingerprint blanked.
Candidate, validation, attestation, receipt, allocation, stage, outcome,
run/attempt/request, recovery, and pre-build IDs use their matching domain
projection above. Refresh retry event IDs are non-secret UUIDv4 values governed
by §6.4 and are included in the refresh-allocation projection.

Exact per-case non-regression means: for every one of the six precommitted
currently matched answerable cases, candidate must return the same expected
stable source at a rank numerically less than or equal to baseline rank, with
all artifact/capability/attestation citations valid. A missing result, worse
rank, different expected source, or invalid citation fails.

Unchanged Slack-source behavior means the before/after
`mindline-slack-source-behavior/v1` fingerprints are identical.

### 11.2 Pre-build owner-only manifests

Before implementation begins, two manifests are frozen and semantically
fingerprinted:

1. evidence-transition manifest:
   - schema `mindline-evidence-transition-manifest/v0.1`;
   - the same six diagnosed insufficient resources;
   - required capability;
   - current base artifact;
   - independent initial sufficiency label and commitment;
   - fixed target predicate `independently_validated_ready`;
   - rows sorted by opaque transition case ID;
   - no candidate-run output;
2. held-out retrieval manifest:
   - at least 20 cases;
   - at least 12 answerable and 8 no-answer;
   - query, label, expected stable source commitments;
   - six current matched per-case non-regression commitments.

The candidate implementer cannot read private cases, labels, queries, expected
sources, or resource identities. Only semantic manifest fingerprints, counts,
schema, and proof rules are public to the build.

### 11.3 Pre-build authority receipt

Before the first code change, an owner-only
`mindline-prebuild-authority-receipt/v0.1` binds:

- exact clean baseline commit and tree;
- transition and held-out semantic manifest fingerprints;
- case counts and required answerable/no-answer split;
- independent reviewer identity/role commitments;
- creation timestamp;
- candidate boundary state `not_started`;
- receipt fingerprint.

Product Brain records only the receipt fingerprint, counts, baseline
commit/tree, and claim limits. It receives no path, resource identity, query,
label, source, or case content. Build work receives only the receipt
fingerprint. The evaluator rejects a missing receipt, a manifest fingerprint not
bound by it, a receipt whose baseline is not an ancestor of the exact candidate
tree, or any manifest created/changed after the receipt boundary.

### 11.4 Intervention run binding

Schema: `mindline-evidence-intervention/v0.1`

The post-build binding contains:

- exact candidate git tree and binary SHA-256;
- exact evaluator SHA-256;
- retrieval/ranker and validator fingerprints;
- runtime/model/provider configuration fingerprints;
- stable source-library commitment;
- pre-build manifest fingerprints;
- before/after evidence-catalog fingerprints;
- allowlisted resource/capability evidence revisions;
- no private content.

The same candidate binary evaluates before and after catalog snapshots.
Candidate retrieval/ranker behavior and source library are unchanged.

### 11.5 Evaluator changes

`recalleval` adds a v0.2 evidence-intervention mode that:

- compares stable source commitments instead of complete library fingerprints;
- requires exact before/after catalog fingerprints;
- rejects any evidence revision outside the precommitted allowlist;
- validates resource/artifact/capability/attestation citation commitments;
- reruns readiness validation on every cited current artifact;
- rejects a stored `ready` value without validator agreement;
- reports per-case baseline/candidate outcomes;
- preserves existing label independence, result isolation, and production seal.

### 11.6 Passing gates

Evidence transition:

- at least 3 of the exact committed 6 move from
  `unverified|insufficient` to independently validated `ready`;
- substitution or post-hoc relabeling fails.

Held-out retrieval:

- at least 12 answerable and 8 no-answer cases;
- recall at least `max(0.75, baseline recall)`;
- precision at least `max(0.15, baseline precision - 0.05)`;
- citation completeness exactly `1.00`;
- no-answer false-positive rate exactly `0`;
- exact non-regression on the six precommitted current matches;
- unchanged stable source denominator and Slack-source behavior.

Before evaluation, every current matched case that depends on resource evidence
is a mandatory deepening target.

## 12. Required public tests

### 12.1 Capability fixtures

- article/main body survives navigation, cookie, footer, and related-link
  decoys;
- login, consent, loading, client shell, and navigation-only HTML may end an
  attempt but cannot become ready;
- useful body below a naive global length heuristic is handled according to its
  capability-specific contract;
- LinkedIn primary post is independent from comments;
- missing LinkedIn comments do not downgrade a ready post;
- media page metadata cannot satisfy transcript;
- timed transcript with late answer-bearing context is fully retained and
  retrievable;
- missing transcript is explicitly inaccessible/insufficient.

### 12.2 Lifecycle

- state-invariant matrix accepts only the four valid
  readiness/artifact/attestation rows;
- all readiness and quality-vector transition pairs, equal-quality tie,
  same-artifact no-op, truncation downgrade, policy change, refresh change, and
  revision-cap failure;
- provider-neutral bootstrap exact replay, conflict, unsafe/ambiguous no-item,
  every atomic fault point, and concurrent bootstrap/promotion;
- corrupt/missing current catalog, valid/stale/corrupt/source-mismatched/
  reference-missing backup, dual corruption, and exact backup recovery;
- every crash point before/after recovery-journal `replacing`, current write,
  current readback, mirror write, mirror readback, and acknowledged journal;
- exact duplicate intent and enqueue;
- post-promotion self-requeue no-op;
- refresh retry-ID replay no-op;
- one new locally authorized retry ID creates one bounded generation;
- same-artifact refresh terminalizes as `unchanged_stronger_current`, changes no
  evidence head/attestation/artifact/envelope/revision/promotion receipt,
  changes only the exact control-ledger/catalog fields, and is not fetched after
  queue rebuild;
- refresh allocation survives queue deletion before promotion and rebuilds the
  same generation;
- concurrent same-retry-ID allocation, allocation crash/restart, and allocation
  budget exhaustion;
- extractor-policy upgrade creates one generation-zero job under the new
  policy;
- charged crash before send;
- one ambiguous crash after response;
- repeated ambiguous crashes never produce more observed GETs than charged
  executions or persisted request/attempt caps;
- crash after stage with same and changed remote response, orphan-stage
  reconciliation, stage storage-budget settlement, and crash after promotion;
- unavailable terminal and bounded retry;
- failed/poorer promotion preserves current head;
- predecessor remains loadable;
- current and predecessor validation envelopes remain retained; missing/tampered
  adopted envelope fails catalog/backup/revision/citation readback and can never
  authorize;
- 17th predecessor fails closed and deletes nothing;
- queue deletion/rebuild preserves canonical evidence;
- queue deletion while generation-zero transition/intent/backfill work is
  pending reconstructs the exact open allocation/ordinals/counters; deletion
  after blocked or no-op terminality reconstructs no fetchable work;
- candidate rollback/reinstall preserves all fingerprints and usability;
- rollback binary neither edits nor prunes catalog, evidence content, staging,
  validation envelopes, refresh allocations, or recovery journal;
- concurrent legacy enrichment and evidence promotion obey lock ordering and
  preserve both authorities.
- package-neutral budget-ledger reserve/settle/ambiguous readback failures
  prevent sends and expose no personalmemory/provider/private fields.

### 12.3 Retrieval and evaluation

- search/get/feedback perform zero network calls and zero queue mutations;
- retrieval emits at most one bounded request intent;
- orchestration enqueues once;
- before completion search abstains/pending; after promotion unchanged ranking
  cites ready evidence;
- unverified/metadata/shell/transcript-missing content cannot authorize;
- citation commitments fail on wrong artifact, capability, attestation, stale
  head, or non-current revision;
- readiness is recomputed, not trusted;
- stable source commitment remains unchanged across allowed evidence revisions;
- canonical commitment mutation tests cover every included/excluded field and
  array-order rule;
- intervention rejects post-build label changes and unexpected evidence changes;
- pre-build authority receipt rejects missing, late, changed, non-ancestor, or
  fingerprint-mismatched manifests;
- retrieval/ranker fingerprint remains unchanged.

### 12.4 Privacy and trust

- refresh rejects unauthorized surfaces, malformed/oversized/non-v4 retry IDs,
  and exhausted budget; concurrent/restart replay returns one generation;
- argv/process/history/terminal/agent-tool tests confirm refresh carries only a
  non-secret retry ID and allocation receipt, never an authorization secret;
- raw response and pre-redaction digest are impossible through the API;
- candidate-envelope tampering of marker, block order, truncation, usage,
  digest, or candidate commitment; missing envelope; envelope/final-artifact
  mismatch; crash/restart revalidation; and candidate-envelope no-export;
- secret variants produce no durable secret-dependent digest;
- lexical URL variants with the same ordinal structure produce the same digest;
- evidence bodies and every derivative are absent from logs, telemetry,
  Product Brain-shaped output, git/public fixtures, structural proof, and
  errors;
- URL, hostname, query, label, excerpt, and resource identity are absent from
  public/structural proof;
- existing SSRF, rebinding, redirect, peer-pin, no-ambient-credential, MIME,
  decompression, byte/time/run-budget, private-I/O, protected-path, and secret
  suites remain green;
- malicious adapter secondary targets prove fresh STD-20 and SafePublicFetcher
  enforcement for loopback/private/link-local/all-answer DNS, rebinding,
  redirect, userinfo, query secret, ambient credentials, MIME, decompression,
  byte, timeout, and request/run budgets, with zero rejected-target request and
  zero durable/log/proof leakage.
- multi-request primary/redirect/secondary adapters and repeated crashes prove
  distinct attempt/request ordinals, conservative reservations, and that
  request, wire, decoded, extracted, storage, and wall-time caps are never
  exceeded.

## 13. Delivery gates

Implementation remains blocked until:

1. this Spec passes two unchanged five-role reviews;
2. Product Brain contains a signed Spec decision implementing DEC-432 with the
   exact committed artifact reference;
3. the reserved candidate is materialized as one unique successor work package
   related to WP-48, DEC-432, and the signed Spec;
4. work-package shaping and handoff audits pass;
5. a delivery Plan passes two unchanged five-role reviews and is captured;
6. pre-build transition and held-out manifests are frozen before the first code
   change;
7. the exact clean worktree and baseline are recorded.

No candidate installation or useful-recall claim is permitted until every
public, private structural, intervention, rollback, fresh-agent, and founder
gate passes.

## 14. Exclusions

- no Slack OAuth, onboarding, UI, or destination;
- no Product Brain runtime/API/key behavior;
- no authenticated browser, cookies, session, or login automation;
- no blanket full-library/full-page/media/comment recovery;
- no raw HTTP archive;
- no synchronous or query-path fetch;
- no ranking or threshold tuning during the evidence intervention;
- no legacy library schema migration;
- no LocalDB code or second canonical memory system;
- no private fixture, query, label, URL, body, identity, or runtime artifact in
  repository/PB/log/proof;
- no production, broad-provider, cross-user, generalization, autonomy, or
  no-human claim.
