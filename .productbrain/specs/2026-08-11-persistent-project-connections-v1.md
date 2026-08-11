# Persistent project connections v1

Status: candidate Shape / Spec / Plan for review  
Date: 2026-08-11  
Authority: Randy's explicit GO; DEC-418, DEC-438, DEC-442, DEC-444, STD-29, FEAT-30, WP-51, WP-55, KEY-13

## Shape

### User outcome

A new agent chat can resume the same owner-approved project context and the same agent-specific relevance learning through one stable Mindline connection handle. The agent still chooses what it needs for its work. Randy does not need to recreate or paste scope, lens, and actor identifiers into every chat.

### Root cause

Mindline already persists agents, searches, and judgments. The caller does not persist its assigned agent identity. Every fresh chat therefore registers another actor, so its earlier feedback remains durable but invisible. The two latest tests proved this: actors increased from 8 to 10 while six searches and six judgments were recorded. Repository context never reached Mindline, so both tests used the same binding.

### Product boundary

Add a local, owner-created preference mapping:

`stable connection handle -> exact active scope + lens + agent`

This is a convenience binding, not evidence, organizational truth, hostile-process authentication, or a repository scanner. It must not change search membership, ranking rules, canonical evidence, or historical judgments.

### Appetite and exclusions

Small vertical slice. No CWD, Git, remote URL, repository-name, or repository-content inference. No automatic context or actor creation. No UI, MCP, remote authentication, ingestion, enrichment, ranking tuning, generalized learning, destination write, or hosted service. No migration of old judgments between actors.

## Spec

### Public workflow

Owner/operator:

1. Generate an opaque, non-secret connection handle with `agent connection-handle`.
2. Bind it once to an existing active scope, lens, and agent with `agent connection-bind`.
3. Give the handle to the intended local agent integration.
4. Revoke it with `agent connection-archive`; archived handles are permanently reserved.

Agent-only:

1. Run `agent-only discover --connection <handle>`.
2. Receive the existing governed workflow with the resolved tuple.
3. Search, selectively hydrate, and record feedback exactly as today.

Direct `discover --scope --lens --agent` remains compatible. Connection mutation remains unavailable from `agent-only`.

### Storage and lifecycle

- Generate exactly 32 cryptographically random bytes as `mlc1_` plus 43 unpadded base64url characters. Reject rather than trim or normalize every value outside `^mlc1_[A-Za-z0-9_-]{43}$` before hashing.
- Store `SHA-256("mindline-project-connection-v1\\0" + handle)` plus the exact tuple and `active|archived` state in a new additive table inside the existing versioned owner-only agent-state database. Never persist the plaintext handle. The domain-separated digest is the canonical connection identity; the system does not claim it can distinguish two plaintext handles with the same digest.
- Allow at most 256 uniquely digested records. Reject invalid state, tuple fields, timestamps, over-capacity insertion, and unsupported schema before mutation.
- Reuse the existing agent-state guarantees: contained regular non-symlink database path, owner-only `0700` directory and `0600` database/WAL/recovery files, SQLite `FULL` synchronization, foreign keys, one local-service writer, transaction rollback, integrity checks, quarantine, and staged recovery.
- Add a dedicated bounded, versioned project-connection recovery snapshot containing the complete table, including archived tombstones. Old binaries ignore and preserve it. Preflight the exact post-mutation recovery projection before the database transaction; refresh and verify the snapshot before acknowledging success. An idempotent retry repairs a snapshot refresh that failed after the database commit.
- Normal discovery reads only the database and never creates, repairs, or falls back to the recovery snapshot. Extend existing `OpenRecovering` service startup and downgrade/re-upgrade initialization to restore the table from that dedicated snapshot through the staged, marker-bound, quarantined recovery flow, then verify that all connections and tombstones exactly match the last acknowledged snapshot before service readiness.
- An identical digest and tuple is a replay. Reusing the canonical digest for a different tuple fails without mutation.
- `connection-archive` is owner-only and idempotent. It changes only the exact active record to archived, preserves every actor/run/judgment and all other handles, survives restart/rollback/reinstall, and permanently prevents rebinding that handle. Unknown archive fails content-free without mutation.
- Unknown, corrupt, archived, or stale connections fail closed and never infer or recreate a tuple.
- Install, restart, rollback, and uninstall/reinstall preserve the connection state and its recovery snapshot. Old binaries ignore both the additive table and dedicated snapshot.
- Explicit config roots stay isolated. No fallback to the default runtime is allowed.
- Status, logs, errors, public proof, and Chain evidence expose no handle, digest, tuple, repository identity, local path, private query, citation, or source content.
- “Owner-only” means the supported operator CLI boundary. Identity remains cooperative local attribution, not hostile-process or same-user authentication of raw local-service routes.

### Acceptance

1. A second process resolves the same actor without registration; actor count stays unchanged.
2. Feedback recorded before restart affects only that exact actor after restart.
3. Two connection handles can use the same scope/lens with different agents without cross-agent effects.
4. Exact canonical-identity replay succeeds; the same digest with a different tuple fails with unchanged connection rows and recovery projection. No plaintext-collision detection claim is made.
5. Owner archive is idempotent, permanently reserves the handle, changes no other connection or agent history, and archived scope, lens, agent, or connection fails closed.
6. A filesystem spy proves no repository file, Git metadata, or remote config is opened.
7. Malformed, oversized, path-like, URL-like, credential-shaped, unsafe-permission, symlink, corrupt, unsupported, duplicate-digest, and over-capacity cases fail closed. Handle, digest, tuple, path, URL, credential, and secret sentinels are absent from status, logs, errors, and public proof; only content-free counts, hashes, and booleans may appear.
8. Identical handles in two explicit runtime roots never cross-resolve.
9. Corrupt agent state blocks discovery without read-time fallback; existing staged recovery restores the exact last-acknowledged active connections and archived tombstones together with agent history. Interrupted recovery resumes safely, quarantine is preserved, and no archived connection becomes active.
10. Install, restart, rollback, and reinstall preserve connections.
11. Explicit-tuple and connection-resolved retrieval return identical authorized candidates and citations.
12. Existing agent-only forbidden routes and all previous tests remain green.
13. The exact audited candidate is installed; runtime binding is ready and the intentionally absent helper skill remains absent.
14. Two fresh chats in two repositories receive different owner-created connections, choose their own work-relevant questions, use only the agent-only route, and return cited evidence or honest abstention plus feedback. Structural proof expects no new actor for either chat.

## Plan

1. Before code, sign this reviewed artifact, capture a governing decision that extends FEAT-30, create and audit a new work package, and create or revise an active outcome measuring successful connection resumptions with zero actor-count growth.
2. Add focused connection types, grammar, validation, an additive agent-state table, archive, recovery projection, and staged recovery reconciliation.
3. Add local-service bind/get/archive routes with active-tuple revalidation and a single writer.
4. Add owner connection-handle/bind/archive commands and connection-based discovery; keep direct discovery compatible.
5. Extend capability/help output without listing or disclosing stored connections.
6. Add black-box acceptance and live lifecycle tests, full Go test/vet, private scanner, and exact install/rollback proof.
7. Run one unchanged-head five-role implementation review under DEC-442.
8. Install the exact audited candidate and create two private test connections.
9. Stop at the fresh-agent test prompt. External-agent execution requires a new run-bounded DEC-444 disclosure and owner initiation; merge waits for that feedback.

## Claim boundary

Passing proves durable, isolated local project connections and resumption of existing exact-agent relevance. It does not prove hostile-process security, automatic project understanding, generalized preference learning, cross-model quality, production readiness, or broad retrieval accuracy.
