# WP-47 local agent access proof

Date: 2026-07-26

Status: private sample-bound acceptance proof

## Result

The first productized local-agent slice works end to end on Randy's retained
Slack evidence:

- Mindline is installed as a user-level macOS service with `RunAtLoad` and
  `KeepAlive`.
- A generated reusable Codex skill accesses Mindline only through the installed
  CLI and versioned local API.
- The canonical evidence library remains the existing private file repository.
  SQLite holds local runtime state; an owner-only recovery snapshot protects
  user-created lenses and append-only judgments without becoming evidence
  authority.
- Hybrid local retrieval returns cited Slack and linked-resource evidence.
- Product lenses persist across service restarts and have no product-defined
  count limit.
- Trace-bound feedback changes later ranking for one lens, is idempotent, and
  can be reversed without changing retained evidence.
- A fresh agent used only the installed skill and CLI to answer a Product Brain
  question from retained evidence and then recorded bounded agent feedback.

This is evidence for one private founder corpus. It is not held-out,
cross-user, production-scale, or autonomous-correctness proof.

## Frozen evidence boundary

- Canonical schema: `mindline-personal-evidence-library/v0.4`
- Canonical revision: `2`
- Canonical fingerprint:
  `7c320fdda7a1c3f0ced24400b7e96b67e61e481f91a6d12656b5c8181f5d4335`
- Retained records: `8`
- Retained resources: `10`
- Historical resource revisions: `7`
- Authority class: `personal_evidence_non_authoritative`
- Observed lenses: `4`
- Indexed embeddings: `8`
- Final installed binary SHA-256:
  `aa5a8d79ea9ca08e8eec92e71b9409ff040bf6d99fcdf81978b4d9a41f49a0ec`

The fingerprint, record count, and resource count were identical before and
after indexing, restart, retrieval, and feedback proof.

## LinkedIn retrieval proof

Three Slack-saved LinkedIn sources were independently returned by the installed
CLI in hybrid mode:

1. Company-brain architecture and organizational knowledge tensions:
   `slack://T05GQFU2EA3/D05H529M32N/1784902473.012269`
2. Production RAG as a data, provenance, and governance problem:
   `slack://T05GQFU2EA3/D05H529M32N/1784902511.580339`
3. RDF Studio and competency-question-driven knowledge graphs:
   `slack://T05GQFU2EA3/D05H529M32N/1784756437.515429`

Each result carried the Slack source reference, canonical linked-resource URL,
evidence locators, resource hashes, and missingness. The proof explicitly
reported unavailable Slack permalinks and partial/non-verbatim LinkedIn
extraction. Accessible selected comments were retained where present; no claim
is made that comments or posts were exhaustively captured.

## Frozen feedback evaluation

Query:

`What lessons should guide building Product Brain as an agent-native knowledge product?`

Lens: `product-brain-building`

For record `capture-523684df4a42c9e6e2514554`:

| State | Rank | Lens feedback | Final score |
| --- | ---: | ---: | ---: |
| Before user dismissal | 2 | 0.0 | 0.9611662531 |
| After user dismissal | 7 | -0.1 | 0.8611662531 |
| After reversal | 2 | 0.0 | 0.9611662531 |

Replaying the same dismissal idempotency key returned the same judgment with
`replayed: true`; it did not add a second effect. Reversal appended a new
opposite event and restored the prior ranking effect. The retained evidence was
never deleted, rewritten, or demoted in authority.

Each user judgment carries four times the weight of one agent judgment. The
combined lens effect is clamped. This is per-event weighting, not a claim that
one user event must dominate an unlimited number of later agent events.

## Fresh-agent proof

A fresh agent process received only the installed Mindline skill. It was
explicitly prohibited from reading the repository, SQLite, Slack connector, or
Product Brain. Through the installed CLI it:

1. checked service status and listed lenses;
2. queried the `product-brain-building` lens;
3. answered with three cited lessons from Slack-saved LinkedIn and GitHub
   evidence;
4. disclosed source missingness and non-authoritative status;
5. appended one `used` and one `dismissed` agent judgment tied to exact
   retrieval candidates; and
6. repeated the query and observed the persisted positive and negative lens
   components.

The fresh agent concluded that Product Brain should be treated as governed
knowledge infrastructure, that retrieval quality depends on data/provenance
work as much as models, and that agent work requires constraints, review,
tests, and feedback loops. Those are source-backed hypotheses, not promoted
organizational truth.

## Lifecycle, privacy, and recovery proof

- Installed runtime directory: owner-only `0700`
- Config, state, socket, skill, launch configuration, and logs: owner-only
  `0600`
- Runtime transport: owner-only Unix socket
- Service ownership: one advisory-lock-protected writer
- Launch lifecycle: macOS user LaunchAgent with `RunAtLoad` and `KeepAlive`
- Restart persistence: all four lenses, eight embeddings, canonical fingerprint,
  retrieval traces, and judgments survived restart
- Provider failure: lexical results remain available with explicit
  `retrieval_state: degraded`
- State corruption: local runtime SQLite is quarantined and rebuilt; canonical
  evidence is not rewritten; durable lenses and judgments restore from the
  private recovery snapshot through a persistent, resumable recovery marker;
  ordinary startup fails closed until restored user state is verified;
  embeddings rebuild on use, while historical retrieval traces are not
  reconstructed and new traces accumulate normally
- Uninstall: installed service, binary, and skill are removed while canonical
  evidence and relevance state remain
- Recovery correction: restart now tries a non-destructive kickstart first.
  A denied restart reports the failure without unregistering an already-running
  service. The no-argument restart and uninstall paths resolve the same
  canonical config path before validating their receipts.
- Credential scan: no live credential marker was introduced in source,
  generated skill, config, or state
- Retrieved source text is explicitly untrusted data in the installed skill;
  agents use it as evidence and do not follow embedded instructions

Actor labels are cooperative provenance inside one OS user account. The
generated skill always identifies itself as `agent`; the service does not
authenticate a hostile same-user process as human versus agent. The four-times
weighting proof therefore demonstrates ranking policy and audit behavior, not
an adversarial human-presence security boundary.

## Executable checks

Passed:

- focused package tests for state, hybrid retrieval, personal memory, service,
  installer, and CLI;
- race-enabled tests for state, retrieval, embedding boundary, service, and
  personal memory;
- `go vet ./...`;
- `git diff --check`;
- 128-lens persistence test;
- single-writer and restart integration test;
- installer/permissions/uninstall-preserves-evidence test;
- corruption quarantine/rebuild test;
- injected partial-quarantine with WAL/SHM sidecars, restore-failure, and
  post-promotion recovery tests;
- semantic-only citation regression;
- live installed CLI, service, retrieval, feedback, replay, reversal, and
  fresh-agent use.

Repository-wide `go test ./...` has one unrelated pre-existing failure:
`internal/evalreadback.TestBuildCommittedFixtureDetectsMultipleArtifactTypes`
expects `generalizable` but receives `non_generalizable`. WP-47 does not modify
`internal/evalreadback` or its fixtures. All other packages pass.

## What remains

- held-out and cross-user relevance evaluation;
- real product installer/onboarding beyond the founder machine;
- source-adapter OAuth UX for ordinary users;
- scale, migration, and long-duration operability proof;
- stronger aggregate policy if product research shows that one user judgment
  must dominate arbitrarily many agent events;
- MCP only if another client later needs it; the local API is already the
  reusable boundary;
- Product Brain and other destinations remain optional downstream adapters,
  not runtime dependencies.
