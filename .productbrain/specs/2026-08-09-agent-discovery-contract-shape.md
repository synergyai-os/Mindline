# Agent discovery contract — Shape

Date: 2026-08-09  
Shape version: `agent-discovery-shape-v1`  
Status: review candidate; not delivery authority  
Authority anchors: DEC-424, DEC-425, DEC-426, DEC-437, DEC-438, FEAT-28, FEAT-30, WP-51, STD-28, PRI-1, BR-1

## Problem

WP-51 proved that scoped recall and feedback isolation work when a caller is
already told the complete scope, lens, and actor tuple. A blind non-Codex agent
then exposed the missing product contract:

- `mindline agent --help` exits with failure and dumps the complete operator CLI;
- the caller cannot safely discover how to use its owner-assigned actor;
- owner/debug `memory search` looks like a legitimate fallback after governed recall abstains;
- abstention does not distinguish no ranked evidence from evidence rejected by the gates;
- retry-token creation, exact replay, conflicting reuse, and reversal are not discoverable;
- `agent get <record-id>` can hydrate a known record without binding it to the exact scoped retrieval run.

The result is a working engine that an unprimed agent cannot use safely or
correctly from the CLI alone.

## User job

Given the Mindline binary, an owner-selected project scope/lens and stable agent
ID, and a question, a fresh local agent can discover the approved workflow,
validate that complete binding, retrieve cited personal evidence, hydrate only
evidence returned by that exact search, and record retry-safe isolated feedback
without reading private storage, guessing context or identity, or bypassing
governed recall.

## Product Model Fit

Verdict: **EXTEND** FEAT-28 and FEAT-30.

This is not a new retrieval engine, source, destination, identity system, or
knowledge store. It makes the existing scoped local-agent contract
self-describing and closes the scoped hydration gap. Canonical evidence remains
stored once. Existing scope/lens/actor and relevance state remain authoritative.

## Chosen shape

1. Add bounded `agent --help` and `agent help` output. It exits `0`, describes
   only the approved agent workflow, and never recommends owner mutations or
   `memory search|get`.
2. Add read-only, machine-readable
   `agent discover --scope <owner-selected-scope> --lens <owner-selected-lens> --agent <owner-assigned-id>`.
   It validates that exact active tuple, reports ready capabilities and
   identity-assurance limits, returns the selected context plus command
   templates and feedback semantics. Partial, mismatched, unknown, or archived
   bindings fail with typed codes; discovery never selects among contexts.
3. Keep actor, scope, and lens creation owner-operated. Do not auto-create,
   infer, or choose an actor. Actor IDs remain cooperative attribution on an
   owner-only local socket, not hostile-process authentication.
4. Add scoped hydration requiring `run + scope + lens + agent + record`. The
   service returns a record only when that exact active tuple produced it as a
   cited candidate. Existing unscoped `agent get` remains compatible but is
   classified in its output as `owner_debug_ungated` with
   `agent_recall_approved: false` and omitted from agent help.
5. Add v0.4-only, content-free abstention diagnostics that distinguish a
   meaningless query, no ranked hits, and ranked hits rejected by evidence
   gates. Do not expose rejected IDs, snippets, or scores.
6. Label owner/debug memory output as ungated and not approved for agent recall.
   Keep the routes for operator use and rollback compatibility.
7. Expose a machine-readable feedback contract and a local
   `agent feedback-token` generator. The caller owns the generated token,
   preserves it for an identical retry, uses a new token for a new event, and
   gets a hard failure for conflicting reuse. The service never issues a token
   with search results.
8. Keep help, capabilities, and installed instruction projection derived from
   one discovery contract so they cannot silently disagree.

## Trust and privacy boundary

- The local Unix socket and private runtime files remain owner-only (`0600` or
  stricter). Actor IDs are declared audit labels, not authentication.
- An honest local client receives an owner-selected scope/lens and assigned
  actor ID. A hostile process running as the same OS user can still call owner
  routes or impersonate an actor; this slice makes no claim otherwise.
- Discovery may return the assigned actor and active scope/lens metadata to the
  local caller. It returns no query, record, source content, judgment, retry
  token, credential, or local storage path.
- Public proof contains only synthetic fixtures or content-free counts, hashes,
  timings, and categorical outcomes. Real queries, IDs, names, citations, and
  source material stay private.

## Fail-able acceptance

1. A held-out non-Codex agent receives only one CLI command containing the
   binary path and its owner-selected scope/lens and assigned actor ID; no
   skill, source, config file, prior report, or bespoke workflow instructions.
2. `agent --help` exits `0` and shows a bounded agent-only quickstart.
3. `agent discover --scope <scope> --lens <lens> --agent <id>` is read-only,
   validates and returns only that complete binding plus the approved workflow,
   and reports the cooperative identity boundary. Partial, cross-scope,
   unknown, and archived bindings fail with stable typed codes. A fixture with
   two plausible contexts proves discovery never chooses or guesses one.
4. The held-out agent uses only scoped v0.4 retrieval, never `memory search` as
   fallback, and either produces cited personal evidence with missingness or
   stops on an honest abstention.
5. Scoped hydration succeeds only for a citation in the matching run and exact
   scope/lens/agent tuple. Wrong run, record, scope, lens, agent, archived tuple,
   and uncited records fail closed. The operation creates no judgment or new
   context.
6. v0.4 abstention diagnostics distinguish meaningless query, no ranked hits,
   and below-gate candidates without disclosing rejected identities or content.
   v0.2/v0.3 packet shapes remain compatible.
7. Owner/debug `memory search|get` and legacy unscoped `agent get` output
   declare `route_class: owner_debug_ungated` and
   `agent_recall_approved: false`. Governed scoped search and hydration declare
   the approved route. Contract tests prove both the positive and negative
   classifications.
8. A caller generates a token with at least 128 random bits, submits feedback,
   retries the exact event to the same judgment, receives rejection for changed
   intent with the same token, and can reverse with a fresh idempotency key.
9. Existing ranking membership, scoped relevance isolation, recovery,
   rollback/re-upgrade, canonical evidence, and the installed local service
   remain unchanged except for the additive contract.
10. One unchanged candidate tree passes tests, vet, race tests, secret scan,
    install failure injection, exact installation, private content-free smoke,
    and two clean independent review rounds.

## Exclusions

- No actor self-registration, inference, arbitrary actor selection, remote
  identity, hostile same-user authentication, or cross-agent aggregation.
- No ingestion, Slack drain, enrichment, ranking/threshold tuning, query
  coaching, generalized preference learning, UI, MCP, remote API, destination
  write, or Product Brain runtime transport.
- No deletion of owner/debug memory routes and no broad CLI redesign.
- No broad recall-quality, autonomy, production, cross-user, or generalization
  claim.

## Rejected shortcuts

- Updating only a Codex skill or README: test-specific coaching, not a product
  contract.
- Auto-provisioning a fresh actor: destroys stable attribution and conflicts
  with the owner-assigned WP-51 model.
- Choosing an existing `codex-*` actor: contaminates another agent's feedback.
- Falling back to `memory search` after abstention: bypasses the scope, lens,
  agent, and evidence gates.
- Relaxing abstention thresholds: manufactures visible results without proving
  better relevance.

## Success claim

If the acceptance proof passes, Mindline may claim that one fresh cooperative
local agent can discover and safely use the existing project-scoped recall and
feedback workflow from the CLI alone. It may not claim authenticated identity,
generalized learning, full Slack-drain quality, or production readiness.
