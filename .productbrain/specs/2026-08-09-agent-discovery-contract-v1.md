# Agent discovery contract v1 — signed-spec candidate

Date: 2026-08-09  
Spec version: `agent-discovery-contract-v1`  
Shape SHA-256: `a343329441b24df62f5618adb243470b78cd08382ab40281b9a03b3aeda72eb1`  
Authority: DEC-424, DEC-425, DEC-426, DEC-437, DEC-438, FEAT-28, FEAT-30, WP-51, STD-28, PRI-1, BR-1

## Outcome and claim

A fresh cooperative local agent, given one CLI command containing an
owner-selected scope, lens, and stable actor ID, discovers and completes cited
recall, scoped hydration, and retry-safe feedback without a skill, source or
config access, prior reports, or bespoke instructions.

Passing proves CLI-only discovery on one private local library. It does not
prove authenticated identity, general recall quality, generalized learning,
full Slack-drain quality, production readiness, or cross-user behavior.

## Product fit and trust

**EXTEND** FEAT-28 and FEAT-30. Canonical evidence and existing scoped state
remain authoritative. Add only CLI/API contracts and response fields; no data
migration or evidence mutation.

The private local socket is the access boundary. `agent_id` is an owner-assigned
cooperative audit/relevance namespace, not a credential. A hostile process
running as the owner can impersonate actors or call owner routes. Mindline never
infers, creates, or selects a binding. Scope/lens/actor mutation remains
owner-operated by contract, not server-enforced authorization.

## Required contract

### Help and discovery

`mindline agent --help` and `mindline agent help` exit `0` and show only:
discovery; scoped-v0.4 search; scoped get; token creation; feedback/retry; and
reversal. They state that binding is owner-provided, identity is declared,
abstention is terminal, hydration is selective, and `memory search|get` plus
unscoped `agent get` are unapproved owner/debug routes. They omit mutation
commands and the global usage wall. Invalid commands still fail.

Discovery is read-only:

```text
mindline agent discover --scope <scope> --lens <lens> --agent <actor> [--config <path>]
```

All binding fields are required. Output schema
`mindline-agent-discovery/v0.1` returns `discovery_state: ready`, the resolved
binding names/IDs, trust limits, approved route `scoped_v0.4`, authority
`personal_evidence_non_authoritative`, and exact templates for search, scoped
get, token, feedback, and reversal. Policy states: terminal abstention,
selective hydration, no memory fallback. Feedback states: caller-owned token,
at least 128 random bits, identical retry replay, conflict rejection, new token
per event, and exact-scope/lens/agent effect.

Partial, cross-scope, unknown, or archived bindings fail with stable categorical
codes and no state change. Discovery never chooses among contexts. If explicit
`--config` is supplied, output reports `mode: explicit` and requires reusing
that same argument for every service-backed template without echoing the path.
Default mode omits it. A two-runtime test proves no silent default fallback.

Discovery, scoped-get, and scoped-feedback failures use closed schema
`mindline-agent-error/v0.1`; allowed fields are only `schema_version`,
`error_code`, `operation`, `retryable`, and enum `repair_action`. They never echo
raw errors, submitted values, paths, credentials, queries, source content,
records, or judgments.

Capabilities add: recommended route; owner/debug route class;
`identity_assurance: declared_local_actor`;
`hostile_process_authentication: false`;
`owner_mutation_enforcement: cooperative`; feedback-token command; and scoped
hydration endpoint. Help, capabilities, and installed skill derive from shared
constants and must agree. Blind proof cannot read the skill.

### Search, diagnostics, and route labels

Existing scoped search remains canonical. Every v0.4 packet adds
`route_class: agent_scoped_governed` and `agent_recall_approved: true`.

Abstention adds `abstention_diagnostics` with classification, bounded
`ranked_candidate_count`, and `authorized_candidate_count: 0`:

- meaningless terms: `query_has_no_meaningful_terms`, ranked `0`;
- no valid ranked hit: `no_ranked_hits`, ranked `0`;
- ranked hits rejected by current gates: `below_evidence_threshold`, ranked
  greater than `0`.

Diagnostics expose no rejected IDs, scores, terms, snippets, sources, or
prompts. Answered packets omit diagnostics. Ranking, thresholds, authorization,
and citation membership do not change. Existing `answer_state`,
`abstention_reason`, policy fingerprint, `retrieval_state`, and
`degraded_reason` remain. v0.2/v0.3 remain compatible.

`memory search|get` and unscoped `agent get` output
`route_class: owner_debug_ungated`, `agent_recall_approved: false`. Legacy
unscoped agent search uses `legacy_agent_unscoped`, false. Existing content is
not removed.

### Scoped hydration and feedback

Approved hydration requires:

```text
mindline agent get <record> --run <run> --scope <scope> --lens <lens> --agent <actor>
```

`POST /v1/scoped/get` resolves the active tuple, requires that run to belong to
it, and requires the record in that run's cited candidates before canonical
hydration. Output binds the tuple and reports governed/approved. Wrong run,
record, tuple, archive state, or uncited record fails before hydration and
writes nothing.

`mindline agent feedback-token` performs no service call, persists nothing,
and returns schema `mindline-feedback-token/v0.1` plus a URL-safe token with at
least 128 random bits, owner `caller`, reuse `identical_retry_only`. Exact
feedback retry returns the original judgment with `replayed: true`; changed
intent with that token fails; reversal appends one event using the original
judgment ID and a fresh idempotency key. Existing isolation is unchanged.

## Compatibility, privacy, and recovery

Changes are additive. Prior binaries and re-upgrades preserve scopes, lenses,
actors, runs, candidates, judgments, reversals, and evidence. Transactional
install restores the exact prior binary, skill, config, receipt, launcher,
rollback artifacts, and service state after any failed mutation or smoke.

Real queries, names, IDs, paths, citations, and transcripts stay owner-local.
Public SHA-256 is allowed only for non-private trees/binaries/schemas. Private
values use domain-separated HMAC-SHA-256 with a fresh owner-private 256-bit key:
`mindline-proof-private-v1\0<kind>\0<value>`. Key and value never enter the repo
or receipt; raw deterministic private-value hashes are rejected.

## Evaluation projection

User: one owner with cooperative local agents. Inputs: an owner-selected binding
and natural-language query. Outputs: CLI discovery, v0.4 cited packet or
abstention, scoped hydration, and judgment lifecycle; no destination write.
Runtime: one owner-local macOS library using the existing local hybrid provider,
model, calibration, and ranking unchanged. Evidence: reusable synthetic
fixtures plus one real-private staging library; only the consumer is held out.
No content-quality, cross-library, cross-user, or model generalization claim.
Privacy and public proof follow the rules above. Thresholds are the unchanged
authorization gates and stated latency ceilings. Guardrails: no guessed binding,
debug fallback, uncited hydration, canonical mutation, feedback leakage, or
stronger identity claim.

## Product Brain authority before build

After clean Spec review, compute its final SHA and create:

1. a content-free blind-discovery tension/insight;
2. a ratified decision, **Adopt self-describing scoped agent discovery v1**,
   bound to this path/SHA and rejecting actor inference/provision and debug
   fallback;
3. a small qualitative-with-evidence work package, **CLI-only scoped agent
   discovery v1**, bound to this path/SHA and `status: shaped`.

Relate the WP: `governed_by` new DEC and DEC-437; `implements` FEAT-28/30;
`depends_on` WP-51; `commits_to` STR-3; `informed_by` the new finding. Sequence:
contract/types; scoped validation/hydration; diagnostics/route labels; token;
compatibility/install/privacy/latency proof; reviews; exact install; blind
positive/absent proof; Chain close. Reconcile shaping audit, link signed Plan,
then handoff audit. Only Delivery Authority moves it to `building`.

## Acceptance

One frozen tree must pass:

1. Help/discovery positive and all binding negatives; discovery changes no
   context/run/judgment fingerprints; two plausible contexts prove no guessing.
2. Governed and owner/debug route-label positives/negatives, including a case
   where debug browse finds hits after governed abstention.
3. All abstention classes and degraded retrieval without leakage; unchanged
   v0.2/v0.3 golden behavior.
4. Scoped-get success plus wrong run/record/scope/lens/agent/archive/uncited
   failures and zero mutation.
5. Token entropy, exact replay, conflict, reversal, second/cross-context reversal,
   restart persistence, and exact-agent isolation.
6. Recovery, rollback/re-upgrade, install failure injection, evidence
   fingerprints, `go test ./...`, vet, race, diff-check, and self-testing secret
   scan.
7. Committed latency manifest: binary/config/library/machine bindings, private
   keyed question commitments, one cold plus 20 warm process-spawn samples,
   nearest-rank p50/p95 and max. Ceilings: help/token 250ms; discovery/status/
   typed errors 3s; get/feedback/reversal 5s; search 25s. Comparable old paths
   bind baseline commit `723b7b3`; >20% p95 regression requires Delivery
   Quality, Risk, and Founder disposition.
8. Blind non-Codex agent receives one discovery command and answerable question;
   it must return a citation, selectively hydrate, answer with personal evidence,
   and prove feedback replay/conflict. Separate absent query must abstain with
   zero get, feedback, or memory calls. Scope/lens predate and are hashed before
   questions; only actor is fresh; private harness attests no query coaching.
9. Public receipt strict schema allows only enums, booleans, counts, timings,
   non-private SHA, and private HMAC commitments. It rejects raw private hashes,
   values, keys, arbitrary fields, and evidence.
10. Failure-schema fixtures for invalid discovery, hydration, and feedback prove
    only the closed repair-safe fields can appear.
11. Product/User Job, Architecture, Chain, Quality, and Risk sign the unchanged
    Spec, Plan, tree, and Close with required second clean passes.

## Exclusions

No actor creation/inference, authentication, cross-agent aggregation, ingestion,
Slack drain, enrichment, ranking tuning, query coaching, generalized learning,
MCP, UI, remote API, destination/PB runtime write, debug-route removal, or broad
quality/autonomy/production/generalization claim. Follow-ups: authenticated
bindings, owner assignment UX, extra no-hit optimization, MCP, and full-drain
quality.
