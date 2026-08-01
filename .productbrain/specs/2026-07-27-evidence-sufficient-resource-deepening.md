# Evidence-sufficient resource deepening successor — signed-Spec candidate

Date: 2026-07-27  
Amended: 2026-08-01  
Status: proposed  
Shape authority: DEC-434  
Supersedes stale Spec authority: DEC-433 at `a17b0e7`  
Reserved candidate identity: `MINDLINE-RESOURCE-EVIDENCE-DEEPENING`  
Builds on: WP-48 implementation surfaces for FEAT-27 and FEAT-29  
Unblocks: WP-48 useful-recall evaluation, installation, and founder review

DEC-434 follows and replaces the product direction previously captured by
DEC-432. DEC-432 and deprecated/invalidated DEC-433 remain historical evidence,
not current delivery authority. The signed decision for this exact amended Spec
must implement DEC-434 and replace DEC-433 before WP-49 can be reconciled.

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
6. Only after executable gates pass does a fresh outside agent use an exact
   contained candidate skill/CLI under a bounded disclosure grant, without
   replacing the default install, and Randy judges its cited answer or
   evidence-bound honest abstention as useful or not useful.
7. Only a bound `useful` verdict authorizes installing that exact proven
   candidate; post-install smoke must reproduce its behavior or restore the
   previous install.

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
- `evidence-staging/`;
- `evidence-reconciliation-receipts/`.

The sidecar is ignored safely by prior binaries. Deepened artifacts live in
`evidence-content/`, never the legacy `content/` directory, so a rollback
binary cannot prune or reinterpret them. Rolling back loses access to new
readiness behavior but preserves the v0.4 library, legacy content, new sidecar,
and deepened artifacts unchanged.

`resource-evidence.recovery.json` uses
`mindline-evidence-recovery-journal/v0.1`:

```text
EvidenceRecoveryJournal
  schema_version = mindline-evidence-recovery-journal/v0.1
  state = acknowledged | replacing
  transition_kind = catalog_mutation | stable_source_reconciliation | null
  acknowledged_current_revision
  acknowledged_current_fingerprint
  acknowledged_current_stable_source_commitment
  acknowledged_backup_revision
  acknowledged_backup_fingerprint
  acknowledged_backup_stable_source_commitment
  predecessor_revision optional
  predecessor_fingerprint optional
  predecessor_stable_source_commitment optional
  candidate_revision optional
  candidate_fingerprint optional
  candidate_stable_source_commitment optional
  fingerprint
```

Optional predecessor/candidate fields are encoded as explicit JSON null when
inapplicable. The fingerprint is SHA-256 over canonical JSON domain
`mindline-evidence-recovery/v1` containing every field above in declaration
order with `fingerprint` blank. No field is excluded. Mutation of any state,
revision, current/backup, predecessor, or candidate field changes the
fingerprint and fails recovery readback.

In `acknowledged`, current and backup revision, fingerprint, and stable-source
commitment are equal; `transition_kind` and every candidate/predecessor field
are null. Before replacement, one atomic journal write moves to `replacing`,
sets one transition kind, preserves the complete acknowledged predecessor
authority, and records every non-null candidate authority field. After current
and mirror readback, one atomic journal write moves to `acknowledged` for the
new equal triple and clears transition fields. The journal is structural
recovery authority, not personal evidence.

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

Canonical personal memory exposes only this read boundary to retrieval,
evaluation, and staged proof:

```text
OpenEvidenceSnapshot(ctx) -> EvidenceReadSnapshot

EvidenceReadBudget
  maximum_library_bytes = 67108864
  maximum_catalog_bytes = 67108864
  maximum_single_artifact_bytes = 4194304
  maximum_validated_artifact_bytes = 33554432
  maximum_working_memory_bytes = 268435456
  maximum_index_build_wall_millis = 30000
  maximum_snapshot_bind_wall_millis = 1000
  maximum_snapshot_retries = 2

EvidenceReadSnapshot
  schema_version = mindline-evidence-read-snapshot/v0.1
  library_revision
  library_fingerprint
  source_library_stable_commitment
  evidence_catalog_revision
  evidence_catalog_fingerprint
  source_documents[] sorted by record_id
  validated_current_entries[]
  snapshot_fingerprint

SourceDocumentReadModel
  record_id
  logical_record_id
  version_state
  source_ref
  occurred_at
  author
  safe_text
  content_hash
  resource_ids[] sorted
  stable_source_commitment

ValidatedEvidenceEntry
  resource_id
  capability
  refresh_generation
  extractor_policy_fingerprint
  artifact_text
  artifact_id
  artifact_sha256
  artifact_bytes
  candidate_envelope_id
  candidate_envelope_sha256
  candidate_envelope_bytes
  candidate_commitment
  attestation_id
  attestation_commitment
  evidence_head_commitment
  citation_commitment
```

`validated_current_entries` contains only immutable record/resource identities,
validated current artifact text/references, capability/readiness/attestation
and citation commitments needed by the existing document/ranking boundary. It
contains no mutation method, queue/lease/provider type, filesystem path, or
sidecar layout type. `source_documents` is the immutable bounded read model
needed for Slack-source retrieval so no consumer also receives the broad
repository. Frozen limits are repository-owned and callers cannot raise them.

Snapshot publication is a two-phase coherent read: capture bounded
library/catalog bytes and fingerprints under ordered locks; release locks and
validate immutable artifacts/envelopes; reacquire locks and require both
fingerprints unchanged. It retries at most twice, then fails closed. The
published snapshot is immutable. Staged proof consumes this same port and does
not copy canonical evidence into staged state.

The retrieval index is keyed by the complete snapshot fingerprint. A matching
fingerprint reuses it; any library/catalog change invalidates it before search.
Cap or deadline exhaustion yields fixed
`evidence_snapshot_budget_exceeded` and no resource-body authorization. Index
build and snapshot binding use injected counters/clock for deterministic cap
tests; ordinary search never replays the complete catalog or all envelopes.

`personalmemory` retrieval remains read-only. It may return one structural
deepening intent. `deepeningorchestrator` is the only application port allowed
to turn that intent into a derived queue mutation. Search, get, and evaluation
perform zero network work. Feedback is network-free and appends only derived
local-agent state; it cannot mutate canonical evidence, retention, or queue.

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
  source_reconciliations[] sorted by reconciled_at, reconciliation_id
  fingerprint
```

Limits:

- at most the existing `MaximumResources` heads per capability;
- at most 8 capability heads per resource;
- at most 16 retained catalog revisions per resource/capability in this slice;
- at most 256 stable-source reconciliation records and matching external
  receipts;
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
  schema_version = mindline-evidence-revision/v0.1
  revision_id
  superseded_at
  prior_head
```

`prior_head` is the complete canonical `EvidenceHead`, not a reference. The
revision ID is `"evidence-revision-" + SHA-256` over canonical JSON domain
`mindline-evidence-revision-identity/v1` containing only the exact
`prior_head_commitment`. `superseded_at` is included in the full revision
projection `mindline-evidence-revision/v1`, which contains every field above in
declaration order with the populated `revision_id`; no full-projection field is
blanked. `superseded_at` is deliberately excluded only from the separate
identity projection so replay at another clock instant cannot create another
revision.
The catalog fingerprint commits the full projection and therefore detects
timestamp mutation. `prior_head_commitment` is the exact
`mindline-evidence-head/v1` commitment from §11.1, not only artifact content.
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
  candidate_commitment
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
byte lengths, and candidate commitment. `inaccessible` has none of them.
`unverified` cannot be emitted by
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
  candidate_commitment
  artifact_id
  artifact_sha256
  artifact_bytes
  storage_state = staged | adopted | rejected
  stage_commitment
```

The journal contains no body, URL, hostname, query, label, excerpt, or raw
error. Before promotion, two files are written content-addressed beneath
owner-only `evidence-staging/`:

1. this strict-bounded canonical envelope:

```text
EvidenceCandidateEnvelope
  schema_version = mindline-evidence-candidate-envelope/v0.1
  candidate_envelope_id
  resource_id
  capability
  extractor_adapter_id
  extractor_policy_fingerprint
  media_type
  blocks[] in source order
    ordinal
    kind
    structural_marker
    text
    locator string | null
  response_truncated
  fixed_missingness[] sorted unique
  retrieved_at
  source_response_digest
  usage
    request_executions
    wire_bytes
    decoded_bytes
    extracted_bytes
    wall_millis
  candidate_commitment
```

   `candidate_commitment` is SHA-256 over canonical JSON domain
   `mindline-evidence-candidate/v1` containing every envelope field in
   declaration order with `candidate_envelope_id` and `candidate_commitment`
   blank. `candidate_envelope_id` is `evidence-candidate-` plus that
   commitment. `candidate_envelope_sha256` elsewhere is SHA-256 of the exact
   immutable serialized envelope bytes including both populated fields; it is
   not interchangeable with `candidate_commitment`. Empty arrays remain `[]`,
   absent locators are explicit null, and integers are decimal JSON integers.

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
FrozenRunProfile
  schema_version = mindline-deepening-run-profile/v0.1
  maximum_distinct_resources = 48
  maximum_attempts = 64
  maximum_request_executions = 64
  maximum_global_concurrency = 4
  maximum_per_host_concurrency = 1
  maximum_wire_bytes = 67108864
  maximum_decoded_bytes = 100663296
  maximum_extracted_bytes = 25165824
  maximum_new_evidence_and_staging_bytes = 33554432
  maximum_wall_millis = 1800000
  maximum_automatic_refresh_generations = 0
  maximum_candidate_blocks = 2048
  maximum_candidate_block_bytes = 65536
  maximum_candidate_total_text_bytes = 524288
  maximum_candidate_envelope_bytes = 1048576
  capability_contract_fingerprints[] sorted by capability
    capability
    contract_fingerprint
  extractor_policy_fingerprints[] sorted by adapter_id, capability
    adapter_id
    capability
    policy_fingerprint
  fetch_policy_fingerprint
  validator_policy_fingerprint
  profile_fingerprint

DeepeningRun
  run_id
  frozen_profile = complete FrozenRunProfile
  opened_at
  deadline_at
  closed_at string | null
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

`profile_fingerprint` is SHA-256 over canonical JSON domain
`mindline-deepening-run-profile/v1` containing every `FrozenRunProfile` field
in declaration order with `profile_fingerprint` blank. `run_commitment` uses
domain `mindline-deepening-run/v1` and contains every `DeepeningRun` field,
including the complete populated frozen profile and counters, with
`run_commitment` blank. The numeric constants above are the sole machine
authority for the §6.6 caps; prose and queue configuration cannot override
them. `deadline_at` is exactly `opened_at + 1,800,000ms`; restart, queue rebuild,
or worker replacement cannot reset it. Evidence and staging share the single
33,554,432-byte cap. Any profile, timestamp, or counter mutation changes the
commitment.

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

`EvidenceCandidate` is transient only. The strict-decoded, read-back
`EvidenceCandidateEnvelope` in §4.8 is the only durable validator input; a
separately retained in-memory candidate cannot authorize validation or
promotion. V1 permits at most 2,048 blocks, 64 KiB UTF-8 per block, 512 KiB
UTF-8 across all block text, and 1,048,576 exact serialized envelope bytes
before any lower canonical run-profile remainder is applied.

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
Validate(EvidenceCandidateEnvelope, CapabilityContract)
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

Candidate-envelope ID is `"evidence-candidate-" + candidate_commitment` as
defined in §4.8. The raw exact-envelope SHA-256 is a separate file-integrity
value and cannot substitute for the domain commitment. Deepened artifact ID remains
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

For a live v0.4 stable-source mismatch after rollback, an old stable commitment
is eligible only when current, backup, and the acknowledged recovery journal
agree on that exact old authority; it may then enter
`ReconcileStableSourceCommitment` immediately and may not authorize an evidence
read first. An unacknowledged old commitment is rejected.

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
v0.4 library.

A prior binary may legitimately add/edit/tombstone captures while the candidate
is rolled back. The next candidate never ignores that source-library change and
never rewrites the sidecar ad hoc. Before evidence can authorize a citation it
runs one `ReconcileStableSourceCommitment` transaction under the normal
legacy-library then evidence-catalog locks:

1. strict-decode and fingerprint-validate the current v0.4 library, current
   catalog, mirror, and acknowledged recovery journal;
2. require the catalog/mirror pair still matches its recorded old stable-source
   commitment and validate every head, revision, stage, allocation, outcome,
   receipt, artifact, and envelope under that old authority;
3. recompute the new stable-source commitment and require every catalog
   resource remains reachable from a retained current or historical capture;
4. construct this typed canonical catalog record:

```text
StableSourceReconciliationRecord
  schema_version = mindline-stable-source-reconciliation-record/v0.1
  reconciliation_id
  prior_library_fingerprint
  accepted_library_fingerprint
  prior_stable_source_commitment
  accepted_stable_source_commitment
  prior_catalog_revision
  prior_catalog_fingerprint
  unchanged_evidence_control_projection_commitment
  reconciled_at
  record_commitment
```

Both library fingerprints are the same exact v0.4 bytes captured at transaction
open and revalidated before commit; therefore
`prior_library_fingerprint == accepted_library_fingerprint`. They do not claim
to fingerprint the historical pre-rollback library. The old
`prior_stable_source_commitment` in the acknowledged catalog is the sole prior
source-projection authority, while the recomputed accepted commitment describes
those same locked current v0.4 bytes under the stable-source projection.

   `reconciliation_id` is `stable-source-reconciliation-` plus the record
   commitment computed with ID and commitment blank. It deliberately cannot
   contain the not-yet-computed accepted catalog fingerprint;
5. build one next catalog changing only `revision`,
   `source_library_stable_commitment`, `source_reconciliations`, and
   `fingerprint`; the complete heads, revisions, refresh allocations, stages,
   runs, allocations, attempts, request executions, outcomes, and promotion
   receipts must have byte-identical canonical projections;
6. persist it through the normal recovery-journal/mirror protocol with
   `transition_kind=stable_source_reconciliation` and read back
   both current and backup before evidence reads resume;
7. after accepted catalog and acknowledged recovery readback, store this
   owner-only audit receipt outside the catalog:

```text
StableSourceReconciliationReceipt
  schema_version = mindline-stable-source-reconciliation-receipt/v0.1
  receipt_id
  reconciliation_record_commitment
  accepted_catalog_revision
  accepted_catalog_fingerprint
  acknowledged_recovery_journal_fingerprint
  receipt_fingerprint
```

`unchanged_evidence_control_projection_commitment` uses canonical domain
`mindline-unchanged-evidence-control/v1` over these complete catalog arrays,
with every nested row populated and the §4.1 sort order preserved:
`heads`, `revisions`, `refresh_allocations`, `stage_journal`,
`deepening_runs`, `allocations`, `attempts`, `request_executions`,
`job_outcomes`, and `promotion_receipts`. It excludes only catalog schema,
revision, source-library stable commitment, `source_reconciliations`, and catalog
fingerprint. The value is computed over both prior and candidate catalogs and
must be byte-identical before the candidate catalog may persist.

FileRepository exclusively owns
`evidence-reconciliation-receipts/` under the same `0700` sidecar boundary.
`receipt_id` is `"stable-source-reconciliation-receipt-" +
reconciliation_record_commitment`; its sole path is
`evidence-reconciliation-receipts/<receipt_id>.json`. Each strict canonical JSON
file is `0600`, at most 16 KiB, written by contained sibling-temp atomic
replacement plus file/directory fsync and strict readback. Exact replay returns
the same receipt. At most 256 record/receipt pairs and 4 MiB total are retained;
the cap fails closed as `stable_source_reconciliation_limit_reached` before
catalog mutation, and no receipt is compacted or deleted in this slice.

   If receipt persistence is interrupted, recovery recreates it
   deterministically from the stored record plus acknowledged catalog/journal;
8. read back the receipt and every bound commitment.

Exact replay is a no-op. Missing prior authority, an unreachable referenced
resource, any non-allowlisted catalog difference, concurrent legacy mutation,
or readback ambiguity leaves the old catalog untouched and all resource-body
authorization blocked. Public fixtures cover added capture, edit/history,
tombstone/history, concurrent mutation, crash at every mirror step, and
rollback/reinstall. Reinstalling the candidate therefore either reads the exact
unchanged catalog when the v0.4 stable commitment is unchanged or completes
this single additive rebase before resuming.

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

### 9.4 Shared evidence, scoped relevance, and many agents

Evidence is stored once and never copied, deleted, or reclassified because one
project, lens, or agent finds it useful or irrelevant. Relevance is a derived,
reversible projection over that shared evidence.

```text
RelevanceContextRef
  schema = mindline-relevance-context-ref/v0.1
  scope_id
  lens_id
  agent_actor_id
  task_run_id
  fingerprint
```

`scope_id` identifies the user's project or working context. `lens_id`
identifies one perspective inside that scope. `agent_actor_id` attributes a
judgment to a stable local agent identity; it does not grant access. The local
agent-state authority owns additive context objects:

```text
ContextScope
  schema = mindline-context-scope/v0.1
  scope_id
  name
  purpose
  owner_actor = user
  status = active | archived
  created_at
  updated_at
  fingerprint

ContextLens
  schema = mindline-context-lens/v0.1
  lens_id
  scope_id
  name
  query
  owner_actor = user
  status = active | archived
  created_at
  updated_at
  fingerprint

ContextAgentActor
  schema = mindline-context-agent-actor/v0.1
  agent_actor_id
  name
  owner_actor = user
  status = active | archived
  created_at
  updated_at
  fingerprint
```

Only the owner surface creates, renames, or archives a scope, lens, or stable
agent actor in this slice. Agents select an existing active scope/lens and their
configured active actor but cannot create or mutate one. Identity is attribution
and isolation, not hostile same-user authentication. Lens/actor count is not a
product constant; storage, pagination, field, and per-request execution budgets
are bounded instead.

The owner-local CLI exposes bounded `scope-put|scope-list|scope-archive`,
`lens-put|lens-list|lens-archive --scope`, and
`actor-put|actor-list|actor-archive` operations. They use the canonical
agent-state transaction/readback path. Agent-mode CLI/skill exposes list/select
only. No disclosure grant includes a context-mutation operation.

IDs and names are at most 256 UTF-8 bytes; scope purpose and lens query are at
most 16,384 UTF-8 bytes; list pages contain at most 100 rows; each retrieval
names exactly one scope/lens/actor; and the owner-only agent-state database has
a 512-MiB limit. There is no product-level count cap on scopes, lenses, or
actors within that storage budget.

The original query alone determines whether a result is authorized and whether
Mindline must abstain. The query-only stage enumerates the complete bounded
authorized base candidate identities and citation commitments before any
scope/lens/actor text or feedback is applied. Scope purpose, lens query, and
prior judgments rerank only that frozen base set; top-k truncation happens
after reranking. Therefore the same query, library/evidence snapshot, output
version, and base budget have identical authorized base candidates and
citations under every context, while order and bounded relevance components
may differ.

For v0.1, the query-only hybrid stage produces at most 100 authorized base
candidates with deterministic candidate-ID tie-breaking; context reranking may
return at most the requested limit and never more than the existing 100-result
maximum. The contained proof grant separately caps output at five. A lens cannot
introduce a candidate outside that base set. The 100-candidate base budget,
query-only scorer fingerprint, and output limit are committed in every
`ContextualRetrievalRun`.

Effective feedback for `(scope_id, lens_id, agent_actor_id)` is exactly:

1. every non-reversed owner judgment for that exact scope/lens, weighted by the
   existing fixed user event weight `1.0`; plus
2. every non-reversed agent judgment for that exact scope/lens and exact
   `agent_actor_id`, weighted by the existing fixed agent event weight `0.25`.

For record `r` in active context `(s,l,a)`, the frozen scorer projection is:

```text
raw_feedback(s,l,a,r) =
  SUM(effect(j) WHERE j.scope_id=s AND j.lens_id=l AND j.record_id=r
                      AND j.actor=user)
  +
  SUM(effect(j) WHERE j.scope_id=s AND j.lens_id=l AND j.record_id=r
                      AND j.actor=agent AND j.agent_actor_id=a)

effect(user, used)       = +1.0
effect(user, dismissed)  = -1.0
effect(agent, used)      = +0.25
effect(agent, dismissed) = -0.25
lens_feedback = clamp(raw_feedback * 0.1, -0.3, +0.3)
```

A reversal inherits the original judgment's scope/lens/actor partition and is
the exact negative of its effect. The actor initiating the reversal may be
audited separately but cannot move it between partitions. Additive agent-state
migration adds `scope_id`, `lens_id`, and `agent_actor_id` to retrieval runs and
adds `scope_id`, `lens_id`, `actor`, and nullable `agent_actor_id` to judgments.
Owner rows require null actor ID; new agent rows require a non-reserved stable
actor ID.

```text
ContextualRetrievalRun
  schema = mindline-contextual-retrieval-run/v0.1
  run_id
  query_commitment
  library_fingerprint
  evidence_catalog_fingerprint
  query_only_scorer_fingerprint
  maximum_base_candidates = 100
  requested_output_limit = 1..100
  base_candidate_set_commitment
  scope_id
  lens_id
  agent_actor_id
  created_at
  run_fingerprint

ContextualJudgment
  schema = mindline-contextual-judgment/v0.1
  judgment_id
  idempotency_key
  run_id
  scope_id
  lens_id
  record_id
  actor = user | agent
  agent_actor_id string | null
  disposition = used | dismissed
  reverses_judgment_id string | null
  effect
  created_at
  judgment_fingerprint
```

The run binds the exact query-authorized base set before reranking. A judgment
must reference a result in that run and exactly match its scope/lens/actor
partition. `effect` is recomputed from
actor/disposition or the reversed row and
is never trusted as input. `query_commitment` uses owner-private canonical
domain `mindline-context-query/v1` over the exact normalized query; the query or
commitment never enters public proof. `base_candidate_set_commitment` uses
domain `mindline-base-candidate-set/v1` over snapshot fingerprint, query
commitment, output version, base budget, and candidate identity/citation
commitments sorted by candidate identity before contextual order.

No judgment from another scope, lens, or stable agent enters the projection.
Every judgment is append-only and bound to its context, retrieval run, cited
item, and actor. Conflicts remain observable events, not truth mutations. This
is the full actor handling for this slice; there is no cross-agent aggregate.

This successor adds the context identity and isolation contract but does not
tune relevance weights or activate a new cross-agent aggregation formula. The
frozen single-lens scorer remains the evaluation control. Any later shared-lens
or actor-personal aggregation requires its own held-out comparison and signed
authority. Existing lenses migrate deterministically to the owner root scope.
Historical `actor=agent` rows have no stable identity, so they map to reserved
`legacy_agent_actor` and affect only the legacy-compatible root context; no new
actor inherits them. The legacy-compatible query path reproduces prior output
bytes and prior binaries remain able to read their original rows. The reserved
root scope ID is `owner_root_scope`; existing lens IDs, names, queries, and
timestamps do not change, nor do reversal links, idempotency identities, event
counts, or effects. Only the reserved legacy actor may request the legacy output
version and prior single-stage lens behavior. New actors, including those using
an existing lens under the root scope, use the two-stage contextual contract;
context-isolation comparisons never mix legacy and contextual output versions.

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
owner-only. They cannot enter telemetry, Product Brain, git, public fixtures,
logs, or reports. They cannot enter hosted inference without a separately
governed, bounded, explicit agent-disclosure grant.

### 10.1 Owner authorization and staged-agent identity

Mindline is built for agent consumption, so deliberate agent recall cannot be
modeled as accidental leakage. For the contained fresh-agent proof, default is
deny: staged search, get, and feedback return no private response unless the
staged localservice disclosure authorizer atomically accepts a matching grant.
This one-shot proof grant does not replace the existing owner-local default
access boundary after installation. The installed CLI remains available to
processes the owner runs through the explicitly installed skill/integration,
which is the owner's existing local-agent disclosure opt-in under DEC-426. It
permits no background export, remote listener, or unrelated task access beyond
that local integration. Stable agent IDs attribute
context/feedback and do not authenticate a hostile same-OS-user process. Any
future remote, multi-user, or hosted access model requires separate authority.

After the fresh Codex task exists and before it receives private results, the
owner invokes the operator-only authorization command. `agentstate.Store` is
the sole authority for authorization, grant, and use state: one SQLite
transaction creates and readbacks this immutable receipt.

```text
AgentDisclosureAuthorizationReceipt
  schema = mindline-agent-disclosure-authorization/v0.1
  authorization_id
  owner_authorization_event_id
  active_goal_fingerprint
  purpose
  surface_class = local_agent | codex_task
  codex_thread_id
  codex_host_instance_id
  issued_at
  expires_at
  receipt_fingerprint
```

A hand-authored or copied file is never authority. The receipt authorizes one
immutable staged manifest:

```text
StagedAgentRunManifest
  schema = mindline-staged-agent-run-manifest/v0.1
  staged_run_id
  authorization_receipt_fingerprint
  surface_class = local_agent | codex_task
  codex_thread_id
  codex_host_instance_id
  pre_minted_agent_run_id
  maximum_task_wait_millis = 3600000
  task_output_deadline_at
  relevance_scope_id
  relevance_lens_id
  agent_actor_id
  candidate_tree_sha256
  candidate_binary_sha256
  candidate_skill_template_sha256
  staged_rendered_skill_sha256
  agent_skill_behavior_fingerprint
  behavior_policy_fingerprint
  staged_config_sha256
  staged_deployment_fingerprint
  staged_runtime_identity
  staged_socket_identity
  canonical_agent_state_source_fingerprint
  staged_agent_state_seed_fingerprint
  stable_source_commitment
  evidence_catalog_fingerprint
  created_at
  expires_at
  manifest_fingerprint
```

Task, host, and run identifiers are opaque structural identifiers, not bearer
secrets. The staged client supplies the manifest fingerprint, exact task
identity, and pre-minted run ID on every operation. The staged service hashes
its own binary, rendered skill, config, and seeded agent state at startup and
before each response-bearing operation. Any task, host, run, tree, binary,
skill-template, rendered-skill, behavior, config, deployment, runtime, socket,
agent-state seed, source-library, evidence-catalog, expiry, or manifest mismatch
fails closed before private bytes are read. This is a structurally enforced
product boundary, not a defense against a malicious process running as the same
OS user.

### 10.2 Immutable grant and consumable use state

The matching authorization and staged manifest may issue one immutable grant:

```text
AgentDisclosureGrantTerms
  schema = mindline-agent-disclosure-grant/v0.1
  grant_id
  authorization_receipt_fingerprint
  staged_manifest_fingerprint
  codex_thread_id
  codex_host_instance_id
  pre_minted_agent_run_id
  relevance_scope_id
  relevance_lens_id
  agent_actor_id
  purpose
  allowed_query_hmac
  allowed_operations[] = compact_search | selected_get | feedback
  maximum_compact_search_calls
  maximum_search_results
  maximum_snippet_bytes_per_result
  maximum_total_snippet_bytes
  maximum_selected_gets
  maximum_selected_get_bytes
  maximum_compact_search_response_bytes
  maximum_selected_get_response_bytes
  maximum_feedback_events
  maximum_total_private_disclosed_bytes
  issued_at
  expires_at
  grant_terms_fingerprint
```

One authorization cannot multiply the disclosure budget. After the staged
candidate has been assembled but before it starts, one SQLite transaction
creates and readbacks the manifest, immutable grant terms, initial use state,
and this mapping:

```text
AgentDisclosureIssuance
  schema = mindline-agent-disclosure-issuance/v0.1
  authorization_receipt_fingerprint
  staged_manifest_fingerprint
  grant_terms_fingerprint
  initial_use_state_fingerprint
  issued_at
  issuance_fingerprint
```

The database enforces a unique primary mapping on each of authorization,
manifest, and grant fingerprints. Exact replay returns the same four objects;
any competing manifest/grant, concurrent issuer, partial insert, or restart
conflict fails closed before the staged service can read private evidence. The
issuance fingerprint uses canonical domain
`mindline-agent-disclosure-issuance/v1` over every field with its own
fingerprint blank. Canonical issuance authority lives in the owner-only
proof/control root outside the deletable staged runtime and remains immutable
through expiry, teardown, and proof retention. The staged store receives issued
objects but cannot issue replacements.

Mutable lifecycle and counters never alter the immutable terms fingerprint:

```text
AgentDisclosureUseState
  schema = mindline-agent-disclosure-use-state/v0.1
  state_id
  grant_terms_fingerprint
  revision
  previous_state_fingerprint
  status = active | exhausted | expired | revoked
  terminal_reason = null | limits_exhausted | expired_at_boundary |
    owner_revoked | recovery_revoked | task_output_captured |
    task_output_invalid | task_output_oversize | task_output_timeout
  reserved_searches
  settled_searches
  ambiguous_searches
  reserved_gets
  settled_gets
  ambiguous_gets
  reserved_feedback_events
  settled_feedback_events
  ambiguous_feedback_events
  reserved_private_bytes
  settled_private_bytes
  ambiguous_private_bytes
  last_use_event_fingerprint
  updated_at
  state_fingerprint
```

Each hash-chained append-only event is exact:

```text
AgentDisclosureUseEvent
  schema = mindline-agent-disclosure-use-event/v0.1
  event_id
  reservation_ordinal
  phase_ordinal = 0 | 1
  previous_event_fingerprint
  reservation_event_fingerprint
  request_id
  grant_terms_fingerprint
  state_before_fingerprint
  operation = compact_search | selected_get | feedback
  target_record_id
  search_response_commitment
  citation_set_commitment
  feedback_judgment_commitment
  result_count
  reserved_private_bytes
  actual_private_bytes
  response_commitment
  phase = reserved | settled | consumed_no_disclosure | ambiguous_consumed
  created_at
  event_fingerprint
```

The event is the acyclic event core. Its fingerprint is computed first over
domain `mindline-agent-disclosure-use-event/v1`, containing every event field
above with `event_fingerprint` blank. It never contains an after-state
fingerprint. The next use state is computed with
`previous_state_fingerprint = state_before_fingerprint` and
`last_use_event_fingerprint = event_fingerprint`. The transaction finally
appends:

```text
AgentDisclosureUseTransition
  schema = mindline-agent-disclosure-use-transition/v0.1
  grant_terms_fingerprint
  reservation_ordinal
  phase_ordinal
  event_fingerprint
  state_before_fingerprint
  state_after_fingerprint
  transition_fingerprint
```

`transition_fingerprint` uses domain
`mindline-agent-disclosure-use-transition/v1` over every field with its own
fingerprint blank. Commitment order is mandatory: event → after-state →
transition. One SQLite transaction inserts all three and readbacks all three.
The event never commits the after state; the state never commits the
transition. Unique constraints reject duplicate
`(grant_terms_fingerprint, reservation_ordinal, phase_ordinal)` and enforce
`UNIQUE(grant_terms_fingerprint, request_id)` only for `phase_ordinal=0`. A
completion repeats its reservation request ID and exact
`reservation_event_fingerprint`.

Terminal task-output capture uses the same SQLite writer lock but is not a
grant operation and cannot be represented as agent feedback. One transaction
first resolves every unmatched reservation as ambiguous, then changes a still
active or exhausted use state to `revoked`, commits the exact terminal reason,
and inserts/readbacks:

```text
AgentDisclosureTerminalizationReceipt
  schema = mindline-agent-disclosure-terminalization/v0.1
  grant_terms_fingerprint
  pre_minted_agent_run_id
  exact_output_commitment string | null
  exact_output_byte_length integer | null
  terminal_reason = task_output_captured | task_output_invalid |
    task_output_oversize | task_output_timeout
  state_before_fingerprint
  state_after_fingerprint
  final_use_ledger_fingerprint
  terminalized_at
  receipt_fingerprint
```

`final_use_ledger_fingerprint` covers immutable grant terms, every use
event/transition, and the terminal after-state, but excludes this receipt and
therefore has no commitment cycle. `task_output_captured` requires a non-null
output commitment and length; invalid/oversize/timeout require both null. A transaction
conflict retries against the new current state; once terminalization commits,
every later search, get, or feedback request is denied before private read.

Every reservation has `phase_ordinal=0` and an explicit null
`reservation_event_fingerprint`. Its one optional completion has
`phase_ordinal=1` and references that reservation. Inapplicable fields are
explicit null. A private ledger fingerprint commits the immutable grant terms,
every event and transition sorted by `(reservation_ordinal, phase_ordinal)`,
and the exact final use-state revision. Every transition must resolve its exact
event, before state, and after state. Gaps, duplicate ordinals/phases,
completion without reservation, multiple completions, or event/state mismatch
fail closed.

The only feedback payload is:

```text
AgentFeedbackJudgment
  schema = mindline-agent-feedback-judgment/v0.1
  feedback_event_id
  pre_minted_agent_run_id
  search_response_commitment
  scope_id
  lens_id
  agent_actor_id
  record_id
  actor = agent
  disposition = used | dismissed
  judgment_commitment
```

The feedback completion event commits this exact judgment. Its record must be
in the sole search citation set; its run, scope, lens, actor, and search
commitments must match the grant. No free text, score, URL, body, or arbitrary
metadata is accepted.

The normalized query is committed with a per-grant random HMAC key retained
only in the staged `agentstate.Store`. Neither query nor HMAC enters structural
proof. The founder grant allows exactly one compact search with at most five
results, 1,024 UTF-8 bytes per snippet, 5,120 snippet bytes total, and a 32,768
UTF-8 byte complete serialized response cap; one citation-bound selected get
with at most 16,384 body bytes, cut at a deterministic paragraph boundary with
an explicit truncation marker, and a 24,576 byte complete serialized response
cap; one feedback event; and 57,344 total private disclosed response bytes.
Feedback returns only a fixed structural acknowledgement and consumes zero
private-disclosure bytes.

Private disclosed bytes equal the UTF-8 length of the exact final serialized
JSON response body released to the agent, including record IDs, source/citation
references, metadata, missingness, truncation markers, and JSON syntax, and
excluding HTTP framing/headers. The byte oracle serializes exactly once before
settlement. Snippet/body sublimits and full-response limits must all pass; a
one-byte excess releases nothing.

`response_commitment` is:

```text
SHA256("mindline-agent-response-bytes/v1" || 0x00 ||
       uint64_be(byte_length) || exact_body_bytes)
```

`citation_set_commitment` is canonical JSON domain
`mindline-agent-citation-set/v1` over the pre-minted run ID, search-response
commitment, and citations in emitted rank order. Each citation includes rank,
record ID, stable-source commitment, and explicit nullable
resource/artifact/capability/attestation/head commitments; duplicates fail.
Responses use canonical JSON with no trailing newline. The exact immutable byte
slice counted and committed before settlement is written verbatim after
settlement readback and is never re-encoded. Allowed headers are only fixed
`Content-Type: application/json`, `Cache-Control: no-store`, and exact
structural `Content-Length`; no private value enters a header. Exact-cap
succeeds, one-byte-over records `consumed_no_disclosure` and releases no private
body.

Every field named `search_response_commitment` or
`selected_get_response_commitment` is the exact `response_commitment` of the
corresponding settled operation under this byte domain; no second semantic hash
or reserialization is permitted.

Limit checks debit cumulative reservation counters only. Settled and ambiguous
counters are diagnostic subsets, not additional debits. Search reserves 32,768
bytes, selected get 24,576, and feedback zero; prior reserved private bytes plus
the new reservation may never exceed 57,344.

Every operation uses two committed SQLite transactions:

1. the reservation transaction verifies authorization, manifest,
   task/run/candidate/query bindings, expiry, current hash-chained state, and
   remaining limits; appends a `reserved` event, next state revision, and
   transition with
   the full search/get response allowance or zero for feedback; commits; and
   readbacks all three before private bytes are read;
2. after exact response serialization, the completion transaction appends
   `settled` with actual bytes/commitments/cited records, or
   `consumed_no_disclosure` when any final check fails; appends the next state
   revision and transition; commits; and readbacks all three before any
   response is released.

A reservation is never refunded. Crash, repository ambiguity, failed readback,
or restart after a committed reservation without completion causes recovery to
append/readback `ambiguous_consumed` and its next state before serving; capacity
remains consumed and no response is replayed. A crash after settled commit but
before/during socket write is also consumed; duplicate request IDs are denied.
Concurrency serializes in SQLite. `exhausted` means no remaining allowed
operation can execute; reaching the private-byte cap does not block the
remaining zero-private-byte feedback event. Expiry and owner revocation are
terminal hash-chained state revisions. Startup/recovery first completes every
unmatched reservation as ambiguous, then revokes every still-active grant.
Corrupt or non-readbackable state denies every agent response without rewriting
prior events.

The localservice disclosure authorizer is a narrow port in front of every
response-bearing path. Direct `GET /captures/{recordID}`, ordinary config, or a
different socket cannot bypass it in staged-agent mode.

### 10.3 Contained staged topology and behavior equivalence

The fresh-agent proof uses an owner-only staged runtime root with distinct
config, socket, local state, staged `agentstate.Store`, lock, and service
identity. It has no launchd registration and never replaces, stops, rewrites,
or reconfigures the default binary, skill, config, socket, database, or service.
The staged agent state is seeded through a consistent SQLite backup of the
exact canonical agent state so lenses and embeddings are available, then is
isolated. It shares only the selected canonical memory/evidence root; search
and get are read-only there. Proof feedback is written only to staged agent
state until the founder verdict.

Candidate logic is separated from deployment paths. The exact candidate binary
is byte-identical across staging and installation. One environment-neutral
skill template is hashed before rendering. Staged and default rendered skills
differ only in a signed deployment-binding block for binary/config/socket
paths; the `mindline-agent-skill-behavior/v0.1` projection excludes only that
block and must equal the template behavior fingerprint. Likewise,
`mindline-runtime-behavior-policy/v0.1` includes provider/model, retrieval,
output-version, privacy, and safety settings while excluding only allowlisted
absolute runtime/database/socket paths. Staged and default configs have
different deployment hashes but the same behavior-policy fingerprint. Mutation
tests prove no behavior or safety field can hide in either excluded projection.

The outside task reads the exact staged rendered skill by verified absolute
path. It invokes the manifest-bound staged binary/config/socket only. The
service refuses escaped paths, default socket selection, hash/fingerprint
mismatch, or changed memory/evidence commitments.

Lifecycle authority lives in an owner-only proof/control root outside the
staged runtime. Before staging it stores this immutable baseline receipt:

```text
StagedAgentBaselineReceipt
  schema = mindline-staged-agent-baseline-receipt/v0.1
  staged_manifest_fingerprint
  default_binary_before_sha256
  default_rendered_skill_before_sha256
  default_config_before_sha256
  default_deployment_before_fingerprint
  default_service_before_fingerprint
  default_agent_state_before_fingerprint
  receipt_fingerprint
```

Before teardown, the proof controller captures the fresh task's exact terminal
output from the bound task/host surface. The only valid output bytes are strict
canonical JSON for this closed envelope:

```text
FreshAgentTerminalOutputEnvelope
  schema = mindline-fresh-agent-terminal-output/v0.1
  outcome = answered | abstained
  claims[] in answer order
    text
    citation_commitments[] in first-use order
  fixed_missingness[] sorted unique
```

Search/get responses may expose only this closed typed missingness row:

```text
AgentMissingnessObservation
  operation = compact_search | selected_get
  reason = query_no_authorized_match | selected_evidence_pending |
    selected_evidence_insufficient | selected_evidence_unverified |
    selected_evidence_inaccessible | selected_evidence_stale |
    evidence_snapshot_budget_exceeded
  record_id string | null
  resource_id string | null
  capability string | null
  evidence_head_commitment string | null
  observation_commitment
```

One additional terminal-only semantic reason is closed and distinct from those
repository observations:

```text
AgentSemanticAbstention
  schema = mindline-agent-semantic-abstention/v0.1
  reason = disclosed_evidence_does_not_support_answer
  pre_minted_agent_run_id
  relevance_scope_id
  relevance_lens_id
  agent_actor_id
  search_response_commitment
  selected_get_response_commitment
  selected_record_id
  selected_citation_commitment
  exact_output_commitment
  commitment
```

Every non-null identity/commitment resolves to the exact response and current
snapshot that produced the fixed reason. `query_no_authorized_match` requires a
zero-result settled search and all target fields null. Snapshot budget
exhaustion requires the repository's fixed cap failure and all target fields
null. Selected-evidence reasons require the selected record/resource and its
exact readiness or staleness state; an adapter or agent cannot supply them.
Observations are part of the exact serialized search/get response body and are
therefore bound by its response commitment.

The complete exact UTF-8 envelope is at most 32,768 bytes, contains at most
eight claims, at most 4,096 UTF-8 bytes per claim, at most 24,576 claim-text
bytes total, and at most five unique citations. Unknown/duplicate fields,
non-canonical JSON, duplicate citations within a claim, an empty claim, or any
cap plus one is invalid. For `answered`, there are one to eight claims, every
claim has at least one citation, every citation resolves to the sole settled
search or selected get, at least one unique citation is used, fixed missingness
is empty, and exactly one selected get is settled. All free text is inside a
cited claim, so no uncited resource-body assertion exists. For `abstained`,
claims is exactly `[]`, fixed missingness contains one to eight closed reasons,
and zero or one selected get is allowed; there is no free-text field in
which a resource-body claim can be hidden.

For repository-observed abstention, `fixed_missingness` must equal—not merely
overlap—the non-empty sorted unique union of reason codes in every bound settled
search/get missingness observation and semantic-abstention commitment is null.
The controller recomputes
`observed_missingness_set_commitment` with domain
`mindline-agent-observed-missingness-set/v1` over the search response
commitment, nullable selected-get response commitment, and complete observation
rows sorted by operation, reason, record ID, resource ID, and capability. An
unsupported, unobserved, omitted, duplicated, or extra reason invalidates the
task output.

When that observed union is empty but the settled search returned at least one
current ready citation, the agent may instead return exactly the singleton
`["disclosed_evidence_does_not_support_answer"]`. This semantic path requires
claims `[]`, exactly one settled selected get for one of those ready citations,
and an exact `AgentSemanticAbstention` commitment binding task/run/context/actor,
both response commitments, selected record/citation, and output bytes. It is an
agent judgment over disclosed evidence, not repository missingness, and Randy's
bound usefulness verdict is its only acceptance authority. It cannot combine
with a repository-observed reason. For `answered`, the observed set may exist
but terminal `fixed_missingness` is exactly `[]` and semantic commitment is
null.

The controller reads at most 32,769 bytes from the task surface. It waits only
until `task_output_deadline_at`, exactly
`min(expires_at, created_at + 3,600,000ms)`; restart cannot extend it. Deadline
expiry uses `task_output_timeout`, revokes the grant, and enters teardown. Under the same
agent-state writer lock the controller reconciles outstanding reservations, strict-parses
and validates the bounded bytes against the current ledger, and atomically
terminalizes the grant. Valid bytes use `task_output_captured`; malformed or
semantically inconsistent bounded bytes use `task_output_invalid`; 32,769
observed bytes use `task_output_oversize`. Only the first creates a task-output
receipt. The latter two still force teardown and permanently block verdict and
installation. After terminalization readback, valid exact bytes enter an
owner-only encrypted envelope and this receipt:

```text
FreshAgentTaskOutputReceipt
  schema = mindline-fresh-agent-task-output/v0.1
  output_receipt_id
  authorization_receipt_fingerprint
  staged_manifest_fingerprint
  grant_terms_fingerprint
  final_use_state_fingerprint
  final_use_ledger_fingerprint
  terminalization_receipt_fingerprint
  codex_thread_id
  codex_host_instance_id
  pre_minted_agent_run_id
  relevance_scope_id
  relevance_lens_id
  agent_actor_id
  candidate_tree_sha256
  candidate_binary_sha256
  candidate_skill_template_sha256
  agent_skill_behavior_fingerprint
  behavior_policy_fingerprint
  stable_source_commitment
  evidence_catalog_fingerprint
  outcome = answered | abstained
  exact_output_byte_length
  exact_output_commitment
  search_response_commitment
  selected_get_response_commitment string | null
  answer_citation_set_commitment
  answer_used_citation_commitments[] in first-use order
  citation_validation_commitment
  observed_missingness_set_commitment
  semantic_abstention_commitment string | null
  fixed_missingness[] sorted unique
  encrypted_output_envelope_sha256
  encrypted_output_key_reference_fingerprint
  output_crypto_policy_fingerprint
  recorded_at
  receipt_fingerprint
```

The output commitment is SHA-256 over
`mindline-fresh-agent-output-bytes/v1 || 0x00 || uint64_be(length) || exact
UTF-8 bytes`. For `answered`, at least one used citation is required and every
used citation must resolve to the exact settled search citation set or selected
get, pass current evidence readback, and appear in the answer; exactly one
selected get must be settled. For `abstained`,
the used-citation array may be empty, at least one fixed missingness reason is
required, zero or one selected get is allowed, and the output may not assert a
resource-body claim. The citation
validation commitment uses domain `mindline-agent-output-citation-validation/v1`
over outcome, output commitment, response commitments, emitted citation set,
used subset, per-citation validation results, observed missingness-set
commitment, nullable semantic-abstention commitment, and terminal missingness. Substitution,
unknown citation, invalid current head, unbound task/run/context, malformed
output, or a second terminal output blocks verdict and installation.

The strict parser derives `outcome`, claim text, fixed missingness, and
`answer_used_citation_commitments` directly from the exact envelope bytes; no
controller-supplied semantic field may override them. The used array is the
first occurrence of each citation while walking claims in order.
`answer_citation_set_commitment` uses canonical domain
`mindline-agent-output-citation-set/v1` over exact output commitment, settled
search citation-set commitment, selected-get response commitment as explicit
null or string, and that derived used-citation array. The receipt is valid only
when its fields equal those derived values and the terminalization receipt
binds the same output commitment, byte length, terminal state, and final use
ledger.

The encrypted output remains available only for Randy's verdict, then its key
and ciphertext are destroyed/read back; the structural receipt and commitment
remain. No raw answer, query, citation text, or private identity enters public
proof, Product Brain, telemetry, git, logs, or reports.

After terminal grant state, the controller selects exactly one feedback outcome:

- `settled_feedback`: the terminal readable ledger proves one settled feedback
  event after one settled search and the valid operation-shape get count, so
  the controller seals that exact judgment;
- `no_feedback_terminal`: a readable final ledger proves no settled feedback
  and binds one fixed reason: `expired_before_feedback`,
  `revoked_before_feedback`, `ambiguous_feedback_consumed`, or
  `exhausted_without_feedback`, or `task_completed_without_feedback`;
- `unreadable_state_destroyed`: corrupt/non-readbackable staged agent state
  prevents a ledger claim, permanently blocking founder verdict and install.

For `settled_feedback`, the controller stores:

```text
StagedAgentFeedbackSeal
  schema = mindline-staged-agent-feedback-seal/v0.1
  staged_manifest_fingerprint
  grant_terms_fingerprint
  feedback_use_event_fingerprint
  feedback_judgment_commitment
  encrypted_envelope_reference
  envelope_ciphertext_sha256
  envelope_key_reference_fingerprint
  envelope_crypto_policy_fingerprint
  sealed_at
  seal_fingerprint
```

The encrypted envelope contains only the bounded judgment. Its key and
ciphertext remain owner-private. Seal readback must reproduce the use-event
commitment. No-feedback outcomes have no envelope.

The controller owns this lifecycle-and-teardown journal outside the runtime;
its first state is armed before staging and it reaches `prepared` only after
output/feedback disposition is known:

```text
StagedAgentTeardownJournal
  schema = mindline-staged-agent-teardown-journal/v0.1
  teardown_id
  baseline_receipt_fingerprint
  staged_manifest_fingerprint
  grant_terms_fingerprint string | null
  task_output_receipt_fingerprint string | null
  task_output_state = pending | valid | missing_or_invalid
  final_use_ledger_fingerprint string | null
  feedback_seal_fingerprint string | null
  feedback_terminal_outcome = pending | settled_feedback |
    no_feedback_terminal | unreadable_state_destroyed
  no_feedback_reason string | null
  pending_output_envelope_identity
  pending_feedback_envelope_identity
  task_output_deadline_at
  staged_runtime_identity
  staged_socket_identity
  sequence
  previous_journal_fingerprint
  state = armed | staging | staged |
    service_starting | service_started | task_waiting |
    output_capturing | output_terminalized | feedback_sealing |
    prepared |
    service_stopping | service_stopped |
    socket_absent |
    runtime_removing | runtime_absent |
    defaults_verified | complete | quarantined
  observed_default_deployment_fingerprint string | null
  observed_default_service_fingerprint string | null
  observed_default_agent_state_fingerprint string | null
  fixed_fault_code string | null
  created_at
  journal_fingerprint
```

`teardown_id` is `"staged-teardown-" + staged_manifest_fingerprint`;
envelope identities are deterministic opaque structural IDs under the
owner-only proof/control root, never paths. For `settled_feedback`, seal and
readable ledger fingerprints are required. For
`no_feedback_terminal`, the seal is null and the readable ledger must prove no
settled feedback. For `unreadable_state_destroyed`, seal and ledger are null and
reason is `corrupt_or_unreadable_state`. Pending states require null
output/ledger/seal fields and pending feedback. The first record has sequence
zero and null previous fingerprint.

A missing or invalid task output is recorded as `missing_or_invalid` with a
null receipt fingerprint. Teardown still runs to completion and destroys staged
state; that state can never enter founder verdict or installation.

The proof controller owns one teardown lock outside the staged root.
Immediately after the immutable baseline receipt and before creating the staged
directory, copying agent state, issuing a grant, binding a socket, or starting a
process, it writes/readbacks the `armed` record with pending output/feedback and
null grant/ledger/seal fields. Each later lifecycle state commits before its
named side effect and advances only after readback. Issuance remains one SQLite
transaction in its sole authority; after issuance readback the next journal
record binds its exact grant fingerprint. A crash between those stores is
reconciled from the unique issuance mapping before any service start, so the
grant is absent, bound once, or revoked and cleaned—never inferred.

Startup, a new staged run, founder verdict, feedback promotion, and installation
scan baseline receipts, issuance mappings, deterministic envelope identities,
and lifecycle journals before work. A baseline/manifest without a journal
creates its deterministic `armed` recovery record; any nonterminal journal
resumes. Recovery revokes an issued active grant, resolves unmatched
reservations, stops the manifest-bound process, proves socket absence, removes
the contained runtime and any partial output/feedback envelope or key, verifies
unchanged defaults, and completes cleanup. No pending output or feedback state
can authorize a receipt or verdict.

After output terminalization and feedback seal/no-feedback selection, the
journal reaches `prepared`. Cleanup then resumes idempotently through:

```text
prepared → service_stopping → service_stopped → socket_absent →
runtime_removing → runtime_absent → defaults_verified → complete
```

Each before-state commits/readbacks before its side effect; each after-state
commits only after contained removal, directory fsync, socket/process absence,
and default-fingerprint readback. Missing/corrupt control authority, escaped
paths, an unkillable service, persistent socket owner, changed default, or more
than 64 records transitions to `quarantined`, blocks verdict/installation, and
makes no completion claim. Each record is at most 32 KiB; the journal is at
most 2 MiB. If the completion receipt write is interrupted after `complete`,
recovery recreates it deterministically without recreating private output or
feedback.

Pre-service staging, service start, task wait/deadline, output read/cap/parse,
terminalization, encrypted-envelope creation, feedback sealing, issuance
absence/presence, and every transition through `prepared` are in the same
crash/restart matrix as teardown. Repeated recovery cannot accumulate a
runtime, socket, agent-state copy, key, ciphertext, grant, or envelope.

Only after the `complete` journal record exists does the controller store:

```text
StagedAgentCompletionReceipt
  schema = mindline-staged-agent-completion-receipt/v0.1
  baseline_receipt_fingerprint
  staged_manifest_fingerprint
  task_output_receipt_fingerprint string | null
  task_output_state = valid | missing_or_invalid
  final_grant_terms_fingerprint string | null
  final_use_ledger_fingerprint string | null
  feedback_seal_fingerprint string | null
  feedback_terminal_outcome = settled_feedback | no_feedback_terminal |
    unreadable_state_destroyed
  no_feedback_reason string | null
  complete_teardown_journal_fingerprint
  teardown_state = complete
  default_deployment_after_fingerprint
  default_service_after_fingerprint
  default_agent_state_after_fingerprint
  receipt_fingerprint
```

Every default fingerprint must equal baseline. Every outcome destroys and
readbacks absence of staged private state. A valid task-output receipt, terminal
readable use ledger, and `complete` teardown may enter founder verdict whether
feedback is settled or absent. `unreadable_state_destroyed` and `quarantined`
permanently block verdict and installation.

### 10.4 Founder verdict and feedback promotion

The founder records `useful` or `not_useful` against the fresh task's cited
answer or evidence-bound honest abstention:

```text
FounderUsefulnessVerdict
  schema = mindline-founder-usefulness-verdict/v0.1
  verdict_id
  authorization_receipt_fingerprint
  staged_manifest_fingerprint
  grant_terms_fingerprint
  final_use_state_fingerprint
  use_ledger_fingerprint
  staged_completion_receipt_fingerprint
  task_output_receipt_fingerprint
  codex_thread_id
  codex_host_instance_id
  pre_minted_agent_run_id
  relevance_scope_id
  relevance_lens_id
  agent_actor_id
  candidate_tree_sha256
  candidate_binary_sha256
  candidate_skill_template_sha256
  agent_skill_behavior_fingerprint
  behavior_policy_fingerprint
  stable_source_commitment
  evidence_catalog_fingerprint
  search_response_commitment
  selected_get_response_commitment string | null
  feedback_use_event_fingerprint string | null
  feedback_judgment_commitment string | null
  citation_set_commitment
  task_output_outcome = answered | abstained
  exact_task_output_commitment
  answer_citation_set_commitment
  citation_validation_commitment
  observed_missingness_set_commitment
  semantic_abstention_commitment string | null
  verdict = useful | not_useful
  owner_authorization_event_id
  recorded_at
  verdict_fingerprint
```

A missing, unattested, stale, unrelated, `not_useful`, changed-candidate,
invalid task output/citations, unreadable ledger, non-complete teardown, or
incomplete verdict cannot authorize install. Feedback is optional: the terminal
ledger must prove exactly one settled search, zero or one settled selected get,
and zero or one settled feedback event. An honest `abstained` output follows
the same verdict path without fabricating a record target or feedback.
Selected-get commitment is null exactly when no get settled. Feedback event and
judgment commitments are both null exactly when no feedback settled and both
non-null otherwise. Teardown/seal reason fields follow the same paired
nullability stated in §10.3; explicit null is the only inapplicable encoding.
Semantic-abstention commitment is non-null only for the exact singleton
semantic reason and must equal the task-output receipt; it is null for answered
and repository-missingness paths.

If and only if the verdict is `useful` and settled feedback exists, that exact
staged feedback is promoted once:

```text
AgentFeedbackPromotionReceipt
  schema = mindline-agent-feedback-promotion/v0.1
  verdict_fingerprint
  grant_terms_fingerprint
  feedback_use_event_fingerprint
  feedback_seal_fingerprint
  feedback_judgment_commitment
  deterministic_idempotency_key
  canonical_agent_state_before_fingerprint
  judgment_commitment
  canonical_agent_state_after_fingerprint
  status = adopted | replayed | failed
  recorded_at
  receipt_fingerprint
```

Promotion decrypts the seal and requires semantic and byte equality among the
judgment, feedback event, verdict, and promotion. It uses the existing
idempotent feedback transaction, copies no search body/snippet, and changes only
the exact feedback-derived projection. Substitution, wrong target/run/search,
tamper, or readback mismatch fails closed.

After successful promotion, useful-with-no-feedback, after `not_useful`, or
after failed promotion has
rolled canonical agent state back and readback its pre-promotion fingerprint,
the encrypted envelope and key are destroyed. An immutable
receipt is exact:

```text
AgentFeedbackEnvelopeDispositionReceipt
  schema = mindline-agent-feedback-envelope-disposition/v0.1
  disposition_id
  task_output_receipt_fingerprint
  verdict_fingerprint
  feedback_terminal_outcome
  feedback_seal_fingerprint string | null
  feedback_promotion_receipt_fingerprint string | null
  action = promoted_deleted | useful_no_feedback |
    not_useful_deleted | failed_rolled_back_deleted
  sequence
  previous_disposition_fingerprint string | null
  state = prepared | canonical_state_verified |
    feedback_key_destroyed | feedback_ciphertext_removed |
    output_key_destroyed | output_ciphertext_removed | complete
  canonical_agent_state_before_fingerprint
  canonical_agent_state_after_fingerprint
  feedback_key_absence_readback_fingerprint
  feedback_ciphertext_absence_readback_fingerprint
  output_key_absence_readback_fingerprint
  output_ciphertext_absence_readback_fingerprint
  recorded_at
  receipt_fingerprint
```

Failed promotion blocks install and requires a new fresh-agent proof.
`not_useful` and `useful_no_feedback` leave canonical feedback unchanged. Every
verdict path destroys/readbacks both private feedback and task-output envelopes;
only commitments and structural audit receipts remain. Disposition is an
append-only, crash-resumable journal under the proof/control lock. It progresses
through the states above, skipping no absence check even when feedback never
existed. `promoted_deleted` requires adopted/replayed promotion;
`useful_no_feedback` and `not_useful_deleted` require identical canonical
before/after fingerprints; failed promotion requires rollback readback. Only a
terminal `complete` disposition for a `useful` verdict may authorize install.
"Destroyed" claims only cryptographic key absence plus ciphertext absence, not
physical-media erasure.

### 10.5 Crash-safe exact installation

Recovery authority is outside every candidate/prior replacement set:

```text
BootstrapHandoffAuthority
  schema = mindline-bootstrap-handoff-authority/v0.1
  handoff_id
  prior_autostart_descriptor_blob_sha256
  prior_target_binary_sha256
  prior_target_rendered_skill_sha256
  prior_target_config_sha256
  prior_target_service_descriptor_sha256
  prior_default_deployment_fingerprint
  prior_default_service_fingerprint
  prior_default_agent_state_fingerprint
  bootstrap_binary_sha256
  bootstrap_autostart_descriptor_sha256
  default_listener_identity
  prepared_at
  authority_fingerprint

InstallationRecoveryBootstrapManifest
  schema = mindline-installation-recovery-bootstrap/v0.1
  bootstrap_handoff_authority_fingerprint
  bootstrap_binary_sha256
  autostart_descriptor_sha256
  bootstrap_protocol_fingerprint
  control_root_identity
  install_lock_identity
  default_listener_identity
  created_at
  manifest_fingerprint

BootstrapHandoffJournal
  schema = mindline-bootstrap-handoff-journal/v0.1
  bootstrap_handoff_authority_fingerprint
  bootstrap_manifest_fingerprint
  sequence
  previous_journal_fingerprint string | null
  state = prepared | bootstrap_bits_installed |
    descriptor_replacing | descriptor_replaced |
    bootstrap_loading | bootstrap_loaded |
    prior_quiescing | prior_quiesced | default_socket_absent | ready |
    prior_preserved | prior_restoring | prior_restored |
    quarantined_no_service
  observed_autostart_descriptor_sha256
  observed_default_service_fingerprint string | null
  observed_default_socket_absent boolean
  fixed_reason string | null
  created_at
  journal_fingerprint

InstallBootstrapReadyReceipt
  schema = mindline-install-bootstrap-ready/v0.1
  bootstrap_handoff_authority_fingerprint
  bootstrap_manifest_fingerprint
  prior_autostart_descriptor_snapshot_sha256
  target_service_descriptor_sha256
  direct_autostart_quarantined = true
  prior_target_binary_sha256
  prior_target_behavior_fingerprint
  default_socket_absent = true
  ready_handoff_journal_fingerprint
  ready_at
  receipt_fingerprint

BootstrapHandoffRecoveryReceipt
  schema = mindline-bootstrap-handoff-recovery/v0.1
  bootstrap_handoff_authority_fingerprint
  bootstrap_manifest_fingerprint
  final_handoff_journal_fingerprint
  observed_autostart_descriptor_sha256
  outcome = exact_prior_descriptor_preserved |
    exact_prior_descriptor_restored | exact_prior_started_sealed |
    bootstrap_ready |
    quarantined_no_service
  prior_service_readback_fingerprint string | null
  bootstrap_ready_receipt_fingerprint string | null
  default_socket_absent boolean
  default_forwarding_target = none | prior
  fixed_reason string | null
  recovered_at
  receipt_fingerprint

DefaultResponseSealPolicy
  schema = mindline-default-response-seal/v0.1
  bootstrap_manifest_fingerprint
  default_listener_identity
  allowed_terminal_statuses[] = handoff_prior | installed | rolled_back
  allowed_terminal_receipt_kinds[] = bootstrap_handoff_recovery |
    exact_installation
  require_terminal_receipt = true
  nonterminal_behavior = fixed_structural_denial_zero_private_bytes
  policy_fingerprint
```

The permanent post-handoff OS autostart descriptor invokes only this immutable
bootstrap, never a candidate or rollback binary. Candidate and prior binaries
run only as children on sealed private sockets; the bootstrap is the sole
default-listener owner. Every `response_seal_fingerprint` is this exact policy
fingerprint.

Bootstrap handoff is itself old-or-new and completes before candidate mutation.
The installer first stores/readbacks the exact prior descriptor and target
authority, bootstrap bits, and fully rendered replacement descriptor in
`BootstrapHandoffAuthority`. It installs/readbacks the bootstrap binary, writes
the complete replacement descriptor to a sibling temporary file, fsyncs it,
and uses one same-directory atomic replacement plus directory fsync. At every
crash boundary the configured autostart path therefore contains either the
exact prior descriptor or the complete bootstrap descriptor; a missing
autostart path is never an allowed intermediate or recovery state.

Every journal state is appended/read back before its named side effect and the
following state only after file, directory, process, service, and socket
readback. `descriptor_replacing` may recover either exact descriptor;
`descriptor_replaced` requires the exact bootstrap descriptor. The only normal
path is `prepared → bootstrap_bits_installed → descriptor_replacing →
descriptor_replaced → bootstrap_loading → bootstrap_loaded → prior_quiescing →
prior_quiesced → default_socket_absent → ready`. The three terminal recovery
paths are `prior_preserved`, `prior_restored`, and
`quarantined_no_service`. A journal contains at most 32 records and 1 MiB;
missing, duplicate, out-of-order, or cap-exceeding authority fails to the
bootstrap-owned no-service quarantine.

If the prior descriptor remains, recovery validates and preserves it and emits
`exact_prior_descriptor_preserved`. If the bootstrap descriptor is durable,
bootstrap is the reboot recovery owner. Before a ready receipt exists it may do
only one of three things: finish quiescence and create/readback the ready
receipt; atomically restore/readback the exact prior descriptor and restart the
exact prior service; or, when neither action is provable, retain its own
descriptor, serve zero private bytes, and commit
`quarantined_no_service`. Every result is captured by the handoff recovery
receipt. Candidate mutation cannot start until `bootstrap_ready` and the exact
`InstallBootstrapReadyReceipt` are durable. A missing/mismatched ready receipt
therefore permits no candidate mutation and never strands the machine without
an autostart recovery owner.

After a valid ready receipt but before any installation snapshot exists, a
restarted bootstrap validates and starts the exact prior target on its sealed
socket, commits/readbacks `exact_prior_started_sealed`, and only then forwards
the default listener under `handoff_prior`. It never invents an installation
receipt. A new installation attempt must quiesce that prior child and prove
socket absence again. Thus the post-ready/pre-snapshot crash gap is an explicit
exact-prior service state, not quarantine or implicit authority.

Before mutation, the installer renders the exact skill template with default
bindings, acquires a dedicated global install/rollback lock, quiesces and stops
the default service, and proves no socket owner or writable request remains. It
then writes/fsyncs/readbacks this owner-private snapshot manifest:

```text
ExactInstallationSnapshotManifest
  schema = mindline-exact-installation-snapshot/v0.1
  installation_id
  verdict_fingerprint
  task_output_receipt_fingerprint
  staged_completion_receipt_fingerprint
  feedback_adoption_state = none | promoted
  feedback_promotion_receipt_fingerprint string | null
  feedback_envelope_disposition_fingerprint
  bootstrap_ready_receipt_fingerprint
  bootstrap_manifest_fingerprint
  response_seal_fingerprint
  candidate_tree_sha256
  candidate_binary_sha256
  candidate_skill_template_sha256
  agent_skill_behavior_fingerprint
  behavior_policy_fingerprint
  prior_binary_blob_sha256
  prior_rendered_skill_blob_sha256
  prior_config_blob_sha256
  prior_target_service_descriptor_blob_sha256
  prior_agent_state_backup_sha256
  prior_default_deployment_fingerprint
  prior_default_service_fingerprint
  prior_default_agent_state_fingerprint
  snapshot_created_at
  manifest_fingerprint
```

The rollback package contains exact owner-only prior bytes plus a
quiesced/checkpointed SQLite backup. Every blob is durable before the manifest
commits. Replacement and recovery use this append-only journal:

```text
ExactInstallationJournal
  schema = mindline-exact-installation-journal/v0.1
  installation_id
  snapshot_manifest_fingerprint
  bootstrap_ready_receipt_fingerprint
  response_seal_fingerprint
  sequence
  previous_journal_fingerprint
  state = prepared |
    replacing_binary | binary_replaced |
    replacing_skill | skill_replaced |
    replacing_config | config_replaced |
    replacing_target_service_descriptor | target_service_descriptor_replaced |
    swapped |
    starting_candidate_sealed | candidate_started_sealed |
    smoke_passed | installed |
    rollback_required |
    restoring_target_service_descriptor | target_service_descriptor_restored |
    restoring_config | config_restored |
    restoring_skill | skill_restored |
    restoring_binary | binary_restored |
    restoring_agent_state | agent_state_restored |
    restored | prior_started_sealed | rolled_back |
    quarantined_no_service
  observed_deployment_fingerprint
  observed_service_fingerprint
  observed_agent_state_fingerprint
  created_at
  journal_fingerprint
```

The installer commits/readbacks `replacing_*` before each atomic file
replacement and `*_replaced` only after fsync and hash readback. After
`swapped`, the bootstrap starts the candidate sealed on its private socket. A
no-private-output smoke runs while writes and default forwarding remain
blocked. The journal commits `installed` only after `smoke_passed`; the
installation receipt is then fsynced/read back. Only after both validate may
the bootstrap forward the default listener to that exact child. A target child
cannot bind the default listener or answer a default request directly.

On every installer/system start after handoff, the immutable bootstrap acquires the same lock
before any target starts. It validates itself, the ready receipt, snapshot,
journal chain, response seal, prior package, and smoked candidate. A valid
nonterminal journal enters `rollback_required` and restores every prior target
component and agent-state backup in reverse order. The bootstrap starts the
prior target sealed, commits `prior_started_sealed` then `rolled_back`, writes
and readbacks the receipt, and only then permits default forwarding. If neither
the exact prior target nor an exact candidate with a durable `smoke_passed`
journal can be proven—or authority is missing, corrupt, or contradictory—it
commits `quarantined_no_service`, proves
the default socket has no target owner/forwarding route, starts no child, and
requires explicit local-operator repair. No mixed or nonterminal deployment can
serve.

Recovery preference is exact and non-vacuous. A valid nonterminal journal plus
a valid prior package always rolls back to prior. If that prior package is
unprovable but the exact candidate bits, behavior, `smoke_passed` journal, and
response seal all validate, bootstrap appends `installed`, recreates/readbacks
the installation receipt, and serves that candidate. A terminal `installed`
journal with an interrupted receipt likewise recreates and serves candidate.
Quarantine is permitted only for the fixed unprovable-authority cases in the
fault oracle of §11.2; it is never an acceptable substitute for a provable
prior or provable smoke-passed candidate.

Final readback derives this receipt; if the journal is `installed` but the
receipt write was interrupted, recovery deterministically recreates it before
ordinary service access:

```text
ExactInstallationReceipt
  schema = mindline-exact-installation-receipt/v0.1
  verdict_fingerprint
  task_output_receipt_fingerprint
  staged_completion_receipt_fingerprint
  feedback_adoption_state = none | promoted
  feedback_promotion_receipt_fingerprint string | null
  feedback_envelope_disposition_fingerprint
  bootstrap_manifest_fingerprint
  bootstrap_ready_receipt_fingerprint
  response_seal_fingerprint
  snapshot_manifest_fingerprint
  final_journal_fingerprint
  candidate_tree_sha256
  candidate_binary_sha256
  candidate_skill_template_sha256
  agent_skill_behavior_fingerprint
  behavior_policy_fingerprint
  prior_default_deployment_fingerprint
  prior_default_service_fingerprint
  prior_default_agent_state_fingerprint
  serving_binary_sha256
  serving_rendered_skill_sha256
  serving_config_sha256
  serving_deployment_fingerprint
  serving_service_fingerprint
  serving_agent_state_fingerprint
  smoke_behavior_fingerprint
  serving_target = candidate | prior
  status = installed | rolled_back
  rollback_readback_fingerprint string | null
  recorded_at
  receipt_fingerprint
```

```text
InstallationQuarantineReceipt
  schema = mindline-installation-quarantine/v0.1
  installation_id string | null
  bootstrap_manifest_fingerprint
  bootstrap_ready_receipt_fingerprint string | null
  snapshot_manifest_fingerprint string | null
  final_journal_fingerprint string | null
  response_seal_fingerprint
  fixed_reason = missing_authority | corrupt_authority |
    candidate_unprovable | prior_unprovable | terminal_receipt_unprovable |
    socket_owner_mismatch
  prior_validation_result
  candidate_validation_result
  default_socket_absent = true
  state = quarantined_no_service
  recorded_at
  receipt_fingerprint
```

`feedback_adoption_state=none` requires no settled feedback and a null promotion
receipt. `promoted` requires settled feedback plus an exact adopted/replayed
promotion. Both require the same valid useful verdict, task-output receipt,
complete staged teardown, and complete feedback/output-envelope disposition.
An `installed` receipt has null `rollback_readback_fingerprint`; a `rolled_back`
receipt requires a non-null fingerprint that validates the complete exact
prior deployment/service/agent-state readback.
Installation, rollback, and quarantine records are each at most 64 KiB; the
journal has at most 128 records and 8 MiB total before startup fails to
quarantine.

Success deletes/readbacks the private agent-state backup after the installed
receipt is durable; deployment rollback bytes follow bounded rollback
retention. Failure restores/readbacks the snapshot before prior-service restart.
Concurrent-write and crash tests cover every prepare, journal, replacement,
fsync, smoke, service start, restore, readback, cleanup, and unlock boundary.

For founder proof, the active goal authorizes one bounded `codex_task`
disclosure following §§10.1–10.5. It permits no bulk access, background export,
telemetry, logs, Product Brain, git, public proof, another task, or use after
expiry/revocation. Structural facts appear only in owner-private proof; no
query/body/snippet may. Every allowed operation scans every non-agent emitted
surface. This is not a zero-hosted-call or hostile-same-user security claim.

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

Load-bearing structural fingerprints use these closed projections (all
identifiers are structural and contain no path or private content):

```text
ActiveGoalBinding
  schema_version = mindline-active-goal-binding/v0.1
  codex_thread_id
  objective_sha256
  status = active
  captured_at
  fingerprint

AgentStateSnapshot
  schema_version = mindline-agent-state-snapshot/v0.1
  sqlite_schema_version
  scope_count
  lens_count
  retrieval_run_count
  judgment_count
  embedding_count
  canonical_tables_commitment
  fingerprint

DeploymentState
  schema_version = mindline-deployment-state/v0.1
  binary_sha256
  rendered_skill_sha256
  config_sha256
  target_service_descriptor_sha256
  skill_behavior_fingerprint
  behavior_policy_fingerprint
  fingerprint

ServiceState
  schema_version = mindline-service-state/v0.1
  bootstrap_manifest_fingerprint string | null
  deployment_fingerprint
  service_identity
  target_state = stopped | direct_legacy | sealed_child | forwarding
  child_socket_identity string | null
  default_listener_identity
  response_seal_fingerprint string | null
  fingerprint

SmokeBehavior
  schema_version = mindline-smoke-behavior/v0.1
  candidate_binary_sha256
  skill_behavior_fingerprint
  behavior_policy_fingerprint
  probes[] sorted by probe_id
    probe_id
    expected_structural_result
    observed_structural_result
  private_response_bytes = 0
  fingerprint

RollbackReadback
  schema_version = mindline-rollback-readback/v0.1
  snapshot_manifest_fingerprint
  final_journal_fingerprint
  deployment_fingerprint
  service_fingerprint
  agent_state_fingerprint
  default_socket_forwarding_state
  fingerprint
```

`canonical_tables_commitment` covers every owner scope, lens, retrieval-run,
judgment/reversal, embedding/index row, and migration marker sorted by table
name then declared primary key, excluding SQLite page/layout metadata. Service
and deployment identities are opaque IDs, never filesystem paths.

Domain prefixes and projections:

- `mindline-active-goal-binding/v1`
  - every ActiveGoalBinding field with fingerprint blanked;
- `mindline-agent-state-snapshot/v1`
  - every AgentStateSnapshot field with fingerprint blanked;
- `mindline-deployment-state/v1`
  - every DeploymentState field with fingerprint blanked;
- `mindline-service-state/v1`
  - every ServiceState field with fingerprint blanked;
- `mindline-smoke-behavior/v1`
  - every SmokeBehavior field with probes sorted by ID and fingerprint blanked;
- `mindline-rollback-readback/v1`
  - every RollbackReadback field with fingerprint blanked;
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
- `mindline-evidence-revision-identity/v1`
  - exact prior-head commitment only;
- `mindline-evidence-revision/v1`
  - schema version, populated revision ID, superseded timestamp, and complete
    prior head; no field is blanked by this nested catalog projection;
- `mindline-evidence-candidate/v1`
  - every bounded candidate-envelope field, with blocks in source order and
    candidate envelope ID and candidate commitment blanked;
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
- `mindline-deepening-run-profile/v1`
  - every FrozenRunProfile field with profile fingerprint blanked;
- `mindline-job-attempt/v1`
  - every JobAttempt field with attempt commitment blanked;
- `mindline-request-execution/v1`
  - every RequestExecution field with execution commitment blanked;
- `mindline-evidence-catalog/v1`
  - every catalog field and collection in §4.1, with catalog fingerprint
    blanked;
- `mindline-evidence-recovery/v1`
  - every EvidenceRecoveryJournal field with journal fingerprint blanked;
- `mindline-evidence-read-snapshot/v1`
  - every EvidenceReadSnapshot field, source documents sorted by record ID,
    validated entries sorted by resource ID/capability, with snapshot
    fingerprint blanked;
- `mindline-stable-source-reconciliation-record/v1`
  - every StableSourceReconciliationRecord field with reconciliation ID and
    record commitment blanked;
- `mindline-unchanged-evidence-control/v1`
  - the ten complete catalog evidence/control arrays named in §8.2 with their
    §4.1 sort order preserved and no other catalog field;
- `mindline-stable-source-reconciliation-receipt/v1`
  - every StableSourceReconciliationReceipt field with receipt fingerprint
    blanked;
- `mindline-prebuild-authority/v1`
  - every pre-build receipt field with receipt fingerprint blanked;
- `mindline-relevance-context-ref/v1`
  - every RelevanceContextRef field with fingerprint blanked;
- `mindline-context-scope/v1`
  - every ContextScope field with fingerprint blanked;
- `mindline-context-lens/v1`
  - every ContextLens field with fingerprint blanked;
- `mindline-context-agent-actor/v1`
  - every ContextAgentActor field with fingerprint blanked;
- `mindline-contextual-retrieval-run/v1`
  - every ContextualRetrievalRun field with run fingerprint blanked;
- `mindline-context-query/v1`
  - exact normalized owner-private query string;
- `mindline-base-candidate-set/v1`
  - snapshot/query/output-version/base-budget commitments and candidate
    identity/citation commitments sorted by candidate identity;
- `mindline-contextual-judgment/v1`
  - every ContextualJudgment field with judgment fingerprint blanked;
- `mindline-agent-disclosure-authorization/v1`
  - every AgentDisclosureAuthorizationReceipt field with receipt fingerprint
    blanked;
- `mindline-staged-agent-run/v1`
  - every StagedAgentRunManifest field with manifest fingerprint blanked;
- `mindline-agent-disclosure-grant/v1`
  - every immutable AgentDisclosureGrantTerms field with terms fingerprint
    blanked;
- `mindline-agent-disclosure-issuance/v1`
  - every AgentDisclosureIssuance field with issuance fingerprint blanked;
- `mindline-agent-disclosure-use-state/v1`
  - every AgentDisclosureUseState field with state fingerprint blanked;
- `mindline-agent-disclosure-use-event/v1`
  - every AgentDisclosureUseEvent field with event fingerprint blanked;
- `mindline-agent-disclosure-use-transition/v1`
  - every AgentDisclosureUseTransition field with transition fingerprint
    blanked;
- `mindline-agent-disclosure-terminalization/v1`
  - every AgentDisclosureTerminalizationReceipt field with receipt fingerprint
    blanked;
- `mindline-agent-disclosure-ledger/v1`
  - grant-terms fingerprint, event and transition fingerprints sorted by
    `(reservation_ordinal, phase_ordinal)`, and final use-state fingerprint;
- `mindline-agent-response-bytes/v1`
  - exact byte construction in §10.2; no canonical JSON projection;
- `mindline-agent-citation-set/v1`
  - pre-minted run ID, search-response commitment, and complete citations in
    emitted rank order with explicit nullable evidence commitments;
- `mindline-agent-missingness-observation/v1`
  - every AgentMissingnessObservation field with observation commitment
    blanked;
- `mindline-agent-observed-missingness-set/v1`
  - search response commitment, nullable selected-get response commitment, and
    complete observation rows in the declared deterministic order;
- `mindline-agent-semantic-abstention/v1`
  - every AgentSemanticAbstention field with commitment blanked;
- `mindline-agent-feedback-judgment/v1`
  - every AgentFeedbackJudgment field with judgment commitment blanked;
- `mindline-agent-skill-behavior/v1`
  - complete skill template content with only the signed deployment-binding
    block replaced by fixed placeholders;
- `mindline-runtime-behavior-policy/v1`
  - every provider/model, retrieval, output-version, privacy, safety, and
    functional config field; excludes only allowlisted absolute
    runtime/database/socket paths;
- `mindline-staged-agent-baseline-receipt/v1`
  - every StagedAgentBaselineReceipt field with receipt fingerprint blanked;
- `mindline-fresh-agent-task-output/v1`
  - every FreshAgentTaskOutputReceipt field with receipt fingerprint blanked;
- `mindline-fresh-agent-output-bytes/v1`
  - exact byte construction in §10.3; no canonical JSON projection;
- `mindline-agent-output-citation-set/v1`
  - exact output commitment, settled search citation-set commitment, nullable
    selected-get response commitment, and derived citation commitments in
    first-use order;
- `mindline-agent-output-citation-validation/v1`
  - outcome, exact output commitment, response commitments, emitted citation
    set, used subset in first-use order, per-citation validation,
    observed-missingness-set commitment, nullable semantic-abstention
    commitment, and fixed terminal missingness;
- `mindline-staged-agent-feedback-seal/v1`
  - every StagedAgentFeedbackSeal field with seal fingerprint blanked;
- `mindline-staged-agent-teardown-journal/v1`
  - every StagedAgentTeardownJournal field with journal fingerprint blanked;
- `mindline-staged-agent-completion-receipt/v1`
  - every StagedAgentCompletionReceipt field with receipt fingerprint blanked;
- `mindline-founder-usefulness-verdict/v1`
  - every FounderUsefulnessVerdict field with verdict fingerprint blanked;
- `mindline-agent-feedback-promotion/v1`
  - every AgentFeedbackPromotionReceipt field with receipt fingerprint blanked;
- `mindline-agent-feedback-envelope-disposition/v1`
  - every AgentFeedbackEnvelopeDispositionReceipt field with receipt
    fingerprint blanked;
- `mindline-bootstrap-handoff-authority/v1`
  - every BootstrapHandoffAuthority field with authority fingerprint blanked;
- `mindline-bootstrap-handoff-journal/v1`
  - every BootstrapHandoffJournal field with journal fingerprint blanked;
- `mindline-bootstrap-handoff-recovery/v1`
  - every BootstrapHandoffRecoveryReceipt field with receipt fingerprint
    blanked;
- `mindline-installation-recovery-bootstrap/v1`
  - every InstallationRecoveryBootstrapManifest field with manifest
    fingerprint blanked;
- `mindline-install-bootstrap-ready/v1`
  - every InstallBootstrapReadyReceipt field with receipt fingerprint blanked;
- `mindline-default-response-seal/v1`
  - every DefaultResponseSealPolicy field with policy fingerprint blanked;
- `mindline-exact-installation-snapshot/v1`
  - every ExactInstallationSnapshotManifest field with manifest fingerprint
    blanked;
- `mindline-exact-installation-journal/v1`
  - every ExactInstallationJournal field with journal fingerprint blanked;
- `mindline-exact-installation-receipt/v1`
  - every ExactInstallationReceipt field with receipt fingerprint blanked;
- `mindline-installation-quarantine/v1`
  - every InstallationQuarantineReceipt field with receipt fingerprint blanked;
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
- `mindline-context-isolation-manifest/v1`
  - every ContextIsolationManifest field and complete case/nested row, with
    fingerprint and nested row commitments blanked and all declared sorts
    applied;
- `mindline-context-isolation-feedback-event/v1`
  - every ContextIsolationFeedbackEvent field with event commitment blanked;
- `mindline-context-isolation-observation-oracle/v1`
  - every ContextIsolationObservationOracle field with oracle commitment
    blanked;
- `mindline-context-base-citation-set/v1`
  - case query commitment plus base citation commitments sorted
    lexicographically;
- `mindline-context-observed-order/v1`
  - case/checkpoint/observer identity plus candidate commitments in emitted
    rank order;
- `mindline-context-visible-feedback/v1`
  - checkpoint, observer context/actor, visible event ordinals, and per-candidate
    quarter-unit totals in declared order;
- `mindline-evidence-transition-manifest/v1`
  - schema, independent reviewer binding, rows sorted by opaque transition case
    ID, each containing resource ID, capability, base-artifact commitment,
    initial readiness, initial sufficiency label, and fixed target predicate;
- `mindline-installation-fault-oracle/v1`
  - every InstallationFaultOracleManifest field and sorted row, with manifest,
    row, and nested commitments blanked;
- `mindline-installation-fault-oracle-row/v1`
  - every InstallationFaultOracleRow field with oracle commitment blanked;
- `mindline-slack-source-behavior/v1`
  - rows sorted by opaque case ID for cases answered from Slack source text:
    `case_id, answered, expected_source_commitment, matched_source_commitment,
    rank, citation_valid`;
- `mindline-private-evidence-intervention-authority/v1`
  - every PrivateEvidenceInterventionAuthority field with authority fingerprint
    blanked;
- `mindline-private-evidence-change-rule/v1`
  - every global or scoped catalog-change rule with rule commitment blanked;
- `mindline-private-evidence-change-scope/v1`
  - every change-scope field and sorted rule commitments with scope commitment
    blanked;
- `mindline-private-catalog-diff-row/v1`
  - every catalog-diff row field with row result identity retained;
- `mindline-private-resource-diff-result/v1`
  - every per-resource result field with result commitment blanked;
- `mindline-private-retrieval-case-result/v1`
  - every per-case result field with result commitment blanked;
- `mindline-private-context-isolation-result/v1`
  - every context-isolation result field with result commitment blanked;
- `mindline-retrieval-metric-inputs/v1`
  - every RetrievalMetricInputs field with input commitment blanked;
- `mindline-baseline-retrieval-metric-projection/v1`
  - every BaselineRetrievalMetricProjection field with projection commitment
    blanked;
- `mindline-candidate-retrieval-metric-projection/v1`
  - every CandidateRetrievalMetricProjection field with projection commitment
    blanked;
- `mindline-retrieval-threshold-results/v1`
  - every RetrievalThresholdResults field with result commitment blanked;
- `mindline-private-evidence-intervention-result/v1`
  - every PrivateEvidenceInterventionResult field with result fingerprint
    blanked;
- `mindline-public-evidence-intervention-result/v1`
  - every PublicEvidenceInterventionResult field with result fingerprint
    blanked.

Prior/accepted head commitments use `mindline-evidence-head/v1`. Semantic
manifest fingerprints use `mindline-private-manifest/v1`. Catalog fingerprints
commit the complete catalog projection with its fingerprint blanked.
Candidate, validation, attestation, receipt, allocation, stage, outcome,
run/attempt/request, recovery, pre-build, relevance-context, disclosure,
staged-agent, skill, runtime-policy, verdict, feedback-promotion, and install
IDs use their matching domain projection above. Refresh retry event IDs are
non-secret UUIDv4 values governed by §6.4 and are included in the
refresh-allocation projection. Evidence revision IDs are the explicit exception:
they use `mindline-evidence-revision-identity/v1`; the populated full revision
projection is committed only through the catalog.

Exact per-case non-regression means: for every one of the six precommitted
currently matched answerable cases, candidate must return the same expected
stable source at a rank numerically less than or equal to baseline rank, with
all artifact/capability/attestation citations valid. A missing result, worse
rank, different expected source, or invalid citation fails.

Unchanged Slack-source behavior means the before/after
`mindline-slack-source-behavior/v1` fingerprints are identical.

### 11.2 Pre-build owner-only manifests

Before implementation begins, four manifests are frozen and semantically
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
3. context-isolation manifest:

```text
ContextIsolationManifest
  schema = mindline-context-isolation-manifest/v0.1
  stable_source_commitment
  evidence_catalog_fingerprint
  scorer_fingerprint
  owner_weight = 1.0
  agent_weight = 0.25
  agent_count
  context_count
  cases[] sorted by opaque case_id
  reviewer_commitment
  fingerprint

ContextIsolationCase
  case_id
  kind = order_divergence | agent_isolation |
    context_isolation | legacy_migration | no_change_control
  query
  contexts[] sorted by context_key
    context_key
    scope_id
    lens_id
    lens_query
  agents[] distinct stable actor IDs sorted lexicographically
  requested_output_limit
  base_candidate_commitments[] sorted lexicographically
  base_citation_commitments[] sorted lexicographically
  base_candidate_set_commitment
  base_citation_set_commitment
  feedback_events[] sorted by event_ordinal
  observation_oracles[] sorted by checkpoint_event_ordinal,
    observation_phase, observer_context_key, observer_agent_actor_id

ContextIsolationFeedbackEvent
  event_ordinal = 1..16 contiguous
  actor = user | agent
  agent_actor_id string | null
  context_key
  target_candidate_commitment
  disposition = used | dismissed
  reverses_event_ordinal integer | null
  expected_effect_quarter_units
  event_commitment

ContextIsolationObservationOracle
  checkpoint_event_ordinal = 0..16
  observation_phase = live | after_restart
  observer_context_key
  observer_agent_actor_id
  expected_visible_feedback_event_ordinals[] sorted ascending
  expected_raw_feedback_quarter_units_by_candidate[] sorted by candidate commitment
    candidate_commitment
    quarter_units
  expected_ordered_candidate_commitments[] in exact rank order
  expected_base_candidate_set_commitment
  expected_base_citation_set_commitment
  oracle_commitment
```

For sorting, `live < after_restart`; all remaining keys use bytewise lexical
order over their canonical UTF-8 strings.

   It freezes 2–8 agents, 2–8 contexts, 4–32 cases, at most five candidates per
   case, at most 16 total feedback events per case, and at most 4,096 UTF-8
   bytes per query or lens text. `expected_effect_quarter_units` is exactly
   `+4|-4` for owner used/dismissed and `+1|-1` for agent used/dismissed; a
   reversal is the exact negative of its target. Every event target is in the
   frozen base set. Owner rows require null actor ID; agent rows require one
   listed actor. At least one agent-isolation case contains one event from every
   listed agent, and at least one order-divergence case precommits different
   exact post-authorization orders for two contexts.

   Each case contains exactly
   `(feedback_event_count + 1) × context_count × agent_count × 2` observation
   rows: baseline checkpoint zero and every post-event checkpoint, each for all
   context/agent observers both live and after a store close/process restart.
   Thus every source-agent to distinct observer-agent cell and every
   cross-context cell is explicit, not inferred from an aggregate. For an agent
   event, only the exact same context and actor may add its ordinal/effect; for
   an owner event, all agents in only the exact same context see it. All other
   visible-event lists, quarter-unit totals, and ordered outputs must be
   byte-identical to the prior checkpoint. Each order contains exactly the
   requested number of unique members from the frozen base set. Both base
   commitments are identical in every oracle row. The manifest is at most
   8 MiB; any missing, duplicate, extra, mis-sorted, or cardinality-invalid row
   fails before candidate execution.
4. public installation fault-oracle manifest:

```text
InstallationFaultOracleManifest
  schema = mindline-installation-fault-oracle/v0.1
  bootstrap_handoff_state_enum_commitment
  installation_journal_state_enum_commitment
  fault_point_ids[] sorted lexicographically
  rows[] sorted by fault_point_id, recovery_authority_state,
    prior_validation, candidate_validation
  fingerprint

InstallationFaultOracleRow
  fault_point_id
  recovery_authority_state = valid | missing | corrupt | contradictory
  prior_validation = valid | unprovable
  candidate_validation = absent | present_unproved |
    smoke_passed | installed_terminal
  expected_handoff_outcome = not_applicable |
    exact_prior_descriptor_preserved | exact_prior_descriptor_restored |
    exact_prior_started_sealed | bootstrap_ready | quarantined_no_service
  expected_serving_target = exact_prior | exact_candidate |
    quarantined_no_service
  expected_installation_terminal = not_started | rolled_back |
    installed | quarantined_no_service
  expected_fixed_reason string | null
  oracle_commitment
```

   `fault_point_ids` contains `no_fault` plus `before_<state>` and
   `after_<state>` for every declared bootstrap-handoff and installation-journal
   state/side effect; the generator rejects an omitted or extra point. The
   matrix contains: a no-fault exact-candidate install; every valid
   pre-`smoke_passed` fault with valid prior resolving to exact prior; every
   durable `smoke_passed`/`installed` candidate with unprovable prior resolving
   to exact candidate; every terminal-installed missing-receipt case recreating
   candidate authority; every invalid candidate with valid prior resolving to
   prior; both bootstrap-descriptor sides of every handoff fault; the
   post-ready/pre-snapshot gap serving only an exact prior sealed child; and
   quarantine only for the enumerated missing/corrupt/contradictory authority
   rows where neither prior nor candidate is provable. There are at most 2,048
   rows and the manifest is at most 2 MiB. The evaluator executes every row;
   an always-quarantine, always-rollback, or always-candidate implementation
   necessarily fails.

The candidate implementer cannot read private cases, labels, queries, expected
sources, resource identities, actor identities, context identities, expected
orders, or feedback rows. Only their semantic fingerprints, counts, schema,
proof rules, and the complete non-private installation fault-oracle manifest
are public to the build.

### 11.3 Pre-build authority receipt

Before the first code change, an owner-only
`mindline-prebuild-authority-receipt/v0.1` binds:

- exact clean baseline commit and tree;
- transition, held-out, and context-isolation semantic manifest fingerprints;
- installation fault-oracle manifest fingerprint;
- case counts, required answerable/no-answer split, context count, and agent
  count;
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

Private identities and public proof are separate types:

```text
PrivateEvidenceInterventionAuthority
  schema = mindline-private-evidence-intervention-authority/v0.1
  prebuild_authority_receipt_fingerprint
  transition_manifest_fingerprint
  held_out_manifest_fingerprint
  context_isolation_manifest_fingerprint
  candidate_tree_sha256
  candidate_binary_sha256
  evaluator_sha256
  retrieval_ranker_fingerprint
  validator_fingerprint
  runtime_configuration_fingerprint
  pre_minted_intervention_run_id
  frozen_run_profile_fingerprint
  stable_source_commitment
  before_catalog_fingerprint
  global_allowed_catalog_changes[] sorted by collection
    collection = deepening_runs
    maximum_added_rows = 1
    maximum_replaced_rows = 0
    rule_commitment
  allowed_change_scopes[] sorted by change_scope_id
    change_scope_id
    resource_id
    capability
    extractor_policy_fingerprint
    refresh_generation = 0
    base_artifact_id string | null
    logical_job_id
    prior_head_commitment
    target_predicate = independently_validated_ready
    catalog_change_rules[] sorted by collection
      collection = heads | revisions | refresh_allocations |
        stage_journal | allocations | attempts | request_executions |
        job_outcomes | promotion_receipts
      allowed_change_kinds[] sorted = added | replaced
      maximum_added_rows
      maximum_replaced_rows
      rule_commitment
    change_scope_commitment
  authority_fingerprint

PrivateEvidenceInterventionResult
  schema = mindline-private-evidence-intervention-result/v0.1
  authority_fingerprint
  before_catalog_fingerprint
  after_catalog_fingerprint
  stable_source_before_commitment
  stable_source_after_commitment
  catalog_diff_rows[] sorted by collection, row_identity_commitment
    collection
    change_kind = added | replaced | removed
    row_identity_commitment
    prior_row_commitment string | null
    accepted_row_commitment string | null
    owning_change_scope_id string | null
    matched_rule_commitment string | null
    allowed
  per_resource_diff_results[] sorted by change_scope_id
    change_scope_id
    prior_head_commitment
    accepted_head_commitment string | null
    accepted_artifact_commitment string | null
    accepted_attestation_commitment string | null
    predecessor_commitment string | null
    final_readiness
    target_predicate_passed
    unexpected_diff_count
    result_commitment
  per_case_results[] sorted by opaque case_id
    case_id
    label = answerable | no_answer
    baseline_returned_count
    baseline_correct_count
    baseline_expected_found
    baseline_expected_rank integer | null
    candidate_returned_count
    candidate_correct_count
    candidate_expected_found
    candidate_expected_rank integer | null
    candidate_false_positive
    candidate_required_citation_count
    candidate_valid_citation_count
    nonregression_required
    nonregression_passed
    result_commitment
  context_isolation_results[] sorted by case_id, oracle_commitment
    case_id
    oracle_commitment
    observed_base_candidate_set_commitment
    observed_base_citation_set_commitment
    observed_visible_feedback_commitment
    observed_order_commitment
    passed_live
    passed_after_restart
    result_commitment
  metric_inputs = complete RetrievalMetricInputs
  threshold_results = complete RetrievalThresholdResults
  leakage_scan_fingerprint
  passed
  fixed_reason_codes[]
  result_fingerprint

PublicEvidenceInterventionResult
  schema = mindline-public-evidence-intervention-result/v0.1
  private_authority_fingerprint
  private_result_fingerprint
  candidate_tree_sha256
  candidate_binary_sha256
  evaluator_sha256
  retrieval_ranker_fingerprint
  validator_fingerprint
  runtime_configuration_fingerprint
  stable_source_before_commitment
  stable_source_after_commitment
  before_catalog_fingerprint
  after_catalog_fingerprint
  transition_case_count
  transitioned_ready_count
  answerable_case_count
  no_answer_case_count
  context_case_count
  agent_count
  baseline_metrics = complete BaselineRetrievalMetricProjection
  candidate_metrics = complete CandidateRetrievalMetricProjection
  thresholds = complete RetrievalThresholdResults
  changed_resource_count
  passed
  fixed_reason_codes[]
  claim_limits[]
  result_fingerprint
```

```text
RetrievalMetricInputs
  answerable_case_count
  no_answer_case_count
  baseline_answerable_expected_found_count
  candidate_answerable_expected_found_count
  baseline_returned_count
  baseline_correct_count
  candidate_returned_count
  candidate_correct_count
  candidate_no_answer_false_positive_count
  candidate_required_citation_count
  candidate_valid_citation_count
  nonregression_required_count
  nonregression_passed_count
  input_commitment

BaselineRetrievalMetricProjection
  recall_ppm
  precision_ppm
  projection_commitment

CandidateRetrievalMetricProjection
  recall_ppm
  precision_ppm
  citation_completeness_ppm
  no_answer_false_positive_ppm
  projection_commitment

RetrievalThresholdResults
  baseline_recall_ppm
  candidate_recall_ppm
  required_recall_ppm
  baseline_precision_ppm
  candidate_precision_ppm
  required_precision_ppm
  citation_completeness_ppm
  no_answer_false_positive_ppm
  nonregression_passed
  context_isolation_passed
  stable_source_unchanged
  slack_source_behavior_unchanged
  all_thresholds_passed
  result_commitment
```

Parts per million use integer floor division of numerator × 1,000,000 by its
non-zero denominator. Precision with zero returned results is zero. Baseline
projection contains only baseline answerable-found/answerable-total recall and
baseline correct/returned precision because the frozen baseline does not claim
evidence citation or no-answer safety metrics. Candidate projection uses the
matching candidate recall/precision plus candidate-valid/candidate-required
citation completeness and candidate no-answer false-positive/no-answer count.
Required recall is `max(750000, baseline_recall_ppm)`; required precision is
`max(150000, baseline_precision_ppm - 50000)`. Per-case sums must reproduce
every prefixed metric input exactly. Candidate citation completeness must be
1,000,000 and candidate no-answer false-positive rate zero. Aggregate booleans
cannot override failed row arithmetic, and no baseline-only value may populate
a candidate field or vice versa.

The authority and private result are owner-only and at most 8 MiB each. The
public result is at most 64 KiB and derives through a type that cannot hold a
private row. It contains only aggregate counts/metrics, structural
fingerprints, fixed reasons, and claim limits. Resource or case IDs, private
capability rows, artifact/attestation identities, queries, labels, expected
sources/orders, actor/context IDs, per-case outcomes, paths, and excerpts are
forbidden publicly. At most 48 private resources, 512 catalog-diff rows, and
64 held-out cases are accepted. The final public bytes pass an in-memory
lexical leakage scan against every private value, allowing only explicitly
typed opaque fingerprint fields.

The authority is signed and read back before any queue, network, stage, or
catalog mutation. It contains no after-catalog value or accepted result
commitment. Only catalog revision/fingerprint may change outside the listed
collections. `source_reconciliations` and row removal are forbidden. Every
added/replaced row must resolve by its canonical identity/ownership fields to
exactly one precommitted global rule or resource/capability/logical-job change
scope and remain within that rule's count. Heads may only be replaced;
revisions, stages, allocations, attempts, request executions, outcomes, and
promotion receipts may only be added unless the authority explicitly lists
the collection's `replaced` kind. The sole run row is owned by the pre-minted
run ID. Duplicate rule matches fail.

The same candidate binary evaluates before and after catalog snapshots. The
private evaluator computes the complete collection diff, rejects every removed,
unmatched, duplicate-owned, excess, or wrong-kind row, validates every result
and metric projection above, writes/readbacks the owner-only result, and only
then derives the public result. Candidate retrieval/ranker behavior and source
library are unchanged.

### 11.5 Evaluator changes

`recalleval` adds a v0.2 evidence-intervention mode that:

- compares stable source commitments instead of complete library fingerprints;
- requires exact before/after catalog fingerprints;
- rejects any evidence revision outside the precommitted allowlist;
- validates resource/artifact/capability/attestation citation commitments;
- reruns readiness validation on every cited current artifact;
- rejects a stored `ready` value without validator agreement;
- reports per-case baseline/candidate outcomes;
- executes the complete pairwise context/agent matrix before and after store
  reopen/process restart without exposing its rows;
- rejects any non-allowlisted catalog difference before public projection;
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

Context/agent isolation:

- every context/actor receives byte-identical authorized base candidate and
  citation commitments for identical query/library inputs;
- at least one precommitted context pair produces its two different exact
  post-authorization order arrays;
- after each agent's feedback, only that actor's same-context effective history
  and order may change; every other actor and context is byte-identical;
- the complete `N × (N-1)` noninterference matrix passes after store close/open
  and process restart;
- owner feedback is visible only in its exact scope/lens, with fixed formula
  `1.0`, `0.25`, multiplier `0.1`, and clamp `[-0.3,+0.3]`;
- `legacy_agent_actor` alone sees historical generic-agent events;
- stable source, evidence catalog, retention, and readiness remain unchanged.

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
- every recovery-journal field/state matrix and stable-source reconciliation
  transition has exact mutation and crash oracles; no unjournaled catalog is
  selectable;
- evidence-revision identity changes with nested prior-head mutation, not replay
  clock; populated-ID mutation fails catalog readback;
- candidate envelope raw-byte SHA and domain commitment cannot substitute;
  unknown/missing field, block reordering, locator/null mutation, ID/commitment
  tamper, and 1-MiB-plus-one fail closed;
- repository-owned evidence snapshot rejects mutation-bearing dependencies,
  mixed fingerprints, every byte/memory/deadline cap plus one, and invalidates
  its index on every library/catalog change;
- stable-source reconciliation no-op, additive capture, edit/history,
  tombstone/history, concurrent mutation, unreachable reference, stale
  unacknowledged authority, exact replay, every mirror crash, unchanged-control
  projection mutation, interrupted deterministic receipt recreation, receipt
  path/permission/readback failure, exact 256/4-MiB caps, and cap plus one;
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
- staged runtime uses distinct config/socket/state/agent-state/lock identities,
  refuses default paths or binding mismatch, and changes no default deployment
  or agent-state fingerprint before, during, or after teardown;
- staged and default rendered skill/config bytes differ only through allowlisted
  deployment bindings; their skill/runtime behavior fingerprints match, and
  mutation of any non-path behavior/safety field fails equivalence;
- concurrent/crash/restart/exact/conflicting issuance proves one authorization
  permanently maps to one manifest/grant outside staged-root deletion;
- maximum-size search plus maximum-size selected get still permits the
  zero-private-byte feedback event, then transitions immutable-grant-bound state
  to exhausted without rewriting a prior event;
- two committed reservation/completion transactions preserve one-shot
  consumption across crashes after reservation commit, private read, response
  serialization, completion commit, and partial socket write; unmatched
  reservations recover as ambiguous before service;
- active→exhausted, active→expired, active→revoked, and
  active→ambiguous-consumed/revoked recovery retain verifiable immutable terms,
  hash-chained states, events, and final ledger;
- event→state→transition commitments are acyclic and reject wrong before/after
  state, missing transition, duplicate ordinal, substituted event, or cycle;
- answered output binds exact task bytes, one selected get, and validated cited
  subset; abstained output binds explicit missingness and succeeds with zero
  feedback; task/run/context/candidate/snapshot/output/citation substitution
  fails;
- terminal-output strict parsing accepts canonical envelopes only at the exact
  32,768-byte cap, rejects cap-plus-one/unknown/duplicate/mis-sorted/malformed
  fields, derives outcome/claims/citations/missingness from bytes, and permits no
  uncited answered claim or free-text abstention;
- repository-missingness abstention accepts only the closed codes emitted by
  the exact settled search/get responses and requires exact observed-set equality;
  unsupported, agent-invented, unobserved, omitted, duplicated, extra,
  wrong-record, wrong-head, response-substituted, observation-row-mutated, or
  observed-set-commitment-mutated reasons fail citation validation;
- a ready-but-non-answering fixture permits only the singleton semantic
  abstention after one bound selected get; response/output/context/citation or
  semantic-commitment substitution fails, mixing it with repository missingness
  fails, and Randy's bound verdict remains required;
- task-output capture resolves outstanding reservations and atomically revokes
  the grant with the exact output commitment before receipt creation; concurrent
  or post-capture search/get/feedback, duplicate capture, crash before/after
  terminalization, invalid output, and oversized output cannot stale the final
  state/ledger or authorize verdict;
- useful output with settled feedback promotes that exact feedback once; useful
  output without feedback records adoption `none`; replay is idempotent,
  unrelated state is unchanged, and `not_useful` or failed promotion blocks
  install;
- feedback payload accepts only the fixed judgment schema and binds exact
  scope/lens/actor/cited target/run/search in the event, verdict, seal, and
  promotion; substitution or tamper fails;
- useful, not-useful, no-feedback, and failed-promotion paths crash-resume every
  feedback/output key/ciphertext disposition state and read back absence; failed
  promotion rolls canonical state back first;
- expired-before-search, revoked-after-search, ambiguous-feedback, exhausted
  without feedback, and corrupt/unreadable staged-state paths remove and verify
  the staged runtime without fabricating a feedback seal; a valid task-output
  receipt, readable operation-shape ledger, and complete teardown are required;
- lifecycle recovery is armed before the first staged side effect and crashes at
  baseline/journal creation, staging, issuance, service start, task wait and
  one-hour deadline, output capture/parse/terminalization/encryption, feedback
  sealing, and transition into `prepared` revoke and remove every bounded
  partial artifact without accumulation;
- crash before/after every prepared→service-stop→socket-absence→runtime-remove→
  runtime-absence→default-readback→complete transition resumes idempotently;
  quarantine cannot claim completion;
- installer refuses missing, `not_useful`, stale, unrelated, incomplete-use,
  tree/binary/skill-template/behavior-policy, task-output, conditional feedback
  adoption/disposition, bootstrap, response-seal, or deployment binding
  mismatch;
- immutable bootstrap is the sole autostart/default-listener owner; target bits
  cannot replace or bypass it, and every preterminal request releases zero
  private bytes;
- atomic bootstrap handoff leaves the autostart path equal to exact prior or
  complete bootstrap at every fault point and recovers exact prior, bootstrap
  readiness, or an explicit quarantine receipt without an unowned gap;
- the complete installation fault-oracle matrix proves no-fault candidate
  installation, every valid-prior nonterminal rollback, every provable
  smoke-passed/installed candidate recovery, terminal receipt recreation, and
  quarantine only for enumerated unprovable-authority rows; all rows also prove
  quiescence and no mixed/default service;
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
- retrieval/ranker fingerprint remains unchanged;
- same query/library under every precommitted scope/lens/actor retains
  byte-identical base authorization, candidates, and citation commitments; at
  least one precommitted pair must produce its two different exact final orders;
- owner feedback affects only the exact scope/lens; Agent A sees owner plus A
  feedback and Agent B sees owner plus B feedback, with the exact frozen formula
  and full pairwise no-leakage before/after restart;
- existing lenses migrate to the owner root scope without rank/output change,
  prior binary read failure, or lost/reinterpreted judgments;
- owner scope/lens/actor create/rename/archive succeeds within bounds; every
  agent create, propose, activate, rename, merge, or archive attempt is
  rejected.

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
- disclosure defaults deny; hand-authored authority/grant files and expired,
  revoked, exhausted, wrong-task/host/run/scope/lens/actor/query/operation/tree/
  binary/skill/config/runtime/socket/source/evidence bindings fail before read;
- founder grant permits exactly one search, at most five results, one
  citation-bound selected get, and one same-context feedback event within every
  per-response and cumulative cap; second/unrelated operations are denied;
- canonical byte-oracle tests count exact serialized JSON including identifiers,
  citations, metadata, markers, and syntax at exact and one-byte-over bounds;
- socket-captured body is the exact committed immutable slice; re-encoding,
  citation-order mutation, non-allowlisted/private header, compression, or
  trailing newline fails;
- full-size search plus full-size get plus feedback succeeds once; feedback at
  the private-byte cap remains possible because it discloses zero private bytes,
  and every later operation is denied;
- concurrent use, duplicate request, crash/readback failure, restart,
  revocation, corrupt state, and recovery consume or revoke fail-closed without
  replay; every allowed operation is followed by a non-agent-surface scan;
- issuance, task-output, teardown, disposition, install, and quarantine
  artifacts contain structural commitments only; feedback/output key and
  ciphertext absence is read back and no receipt can reconstruct either;
- private intervention authority enforces every allowed resource change; the
  separately typed public result contains aggregates/fingerprints only and
  passes lexical leakage mutation scans for every forbidden private field;
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

1. this exact amended Spec passes two unchanged five-role reviews;
2. the amended Spec is committed and Product Brain contains a new signed
   superseding/amending decision with that exact artifact reference; DEC-433's
   `a17b0e7` reference is never authority for amended bytes;
3. WP-49 is reconciled to that decision and orders contained fresh-agent proof
   and founder verdict before default installation;
4. WP-49 replaces its stale `governed_by DEC-433` relation and stale material
   fields, remains the unique successor related to WP-48, and is governed by
   DEC-434 plus the current Spec decision;
5. work-package shaping and handoff audits pass;
6. only after steps 1–5, a Plan referencing current Spec authority passes two unchanged five-role
   reviews, is committed, and is captured with an evidence ledger;
7. Plan change policy requires renewed Chain authority for changes to scope,
   acceptance, proof, exclusions, phase/install order, work package, privacy,
   ranking, or claim boundaries;
8. independently reviewed authority tooling exists before it freezes private
   transition, held-out, and context-isolation manifests plus the public
   installation fault-oracle manifest;
9. manifests and pre-build receipt are frozen before the first candidate
   product-code change;
10. the exact clean candidate baseline is recorded.

No default installation or useful-recall claim is permitted until every public,
private structural, intervention, rollback, contained fresh-agent, and founder
gate passes. A contained candidate may run only through §§10.1–10.3. After a
bound `useful` verdict over the exact task-output receipt, plus exact feedback
promotion when and only when settled feedback exists, only the proven
tree/binary/skill template and behavior policy may enter §10.5 installation.
Changed product bits return to proof; failed smoke restores prior default.

Any reviewed-artifact byte change reruns its full panel. A material change to
scope, acceptance/proof, schema/commitment, state machine, ownership/trust/
privacy, phase/install/recovery order, ranking, claim, or exclusion requires new
Spec Chain authority, WP-49 reconciliation/audit, and renewed Plan authority.
The Plan may refine sequencing only; it cannot decide these boundaries.

## 14. Exclusions

- no Slack OAuth, onboarding, UI, or destination;
- no Product Brain runtime/API/key behavior;
- no authenticated browser, cookies, session, or login automation;
- no blanket full-library/full-page/media/comment recovery;
- no raw HTTP archive;
- no synchronous or query-path fetch;
- no ranking or threshold tuning during the evidence intervention;
- no new cross-agent relevance aggregation formula or learned-weight claim;
- no agent-created or agent-mutated project scope, lens, or stable actor in this
  slice;
- no legacy library schema migration;
- no LocalDB code or second canonical memory system;
- no private fixture, query, label, URL, body, identity, or runtime artifact in
  repository/PB/log/proof;
- no production, broad-provider, cross-user, generalization, autonomy, or
  no-human claim.
