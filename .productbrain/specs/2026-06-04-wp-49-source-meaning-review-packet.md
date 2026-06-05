# WP-49 Source Meaning Review Packet

## Outcome

Mindline must turn PR43's evidence-backed atoms into a useful source-neutral review packet. The product win is not raw atom volume; it is a bounded set of grouped candidate outputs that a reviewer can inspect, route, block, or defer without reading 208 atom cards and 20,926 relation candidates.

## Chain Authority

- `DEC-384`: PR43 / WP48 is merged into `main`.
- `DEC-381`: WP48 proved segment-level atomization on the same real mixed Gmail/Slack dataset.
- `INS-24`: atoms must become meaning review packets.
- `INS-23`: semantic-density gates must force segment-level atomization.
- `PRI-1`, `BR-1`, `STD-17`, `DEC-64`: semantic autonomy remains privacy-safe, provider-agnostic, measured, and human-gated until held-out proof exists.

## Product Model Fit

Eligibility: `EXTEND`.

WP49 extends the source-neutral review/evaluation layer above semantic atoms and corpus graph relations. It does not add Gmail logic, Slack logic, destination-specific writes, hosted inference, hosted telemetry, or no-human acceptance. Source adapters still only provide local source items with provenance. Destination adapters may later consume approved packet groups, but this PR only writes local review artifacts.

## Spec Challenge

Initial target rejected: "show the extracted atoms in a nicer report" is too weak because it preserves the PR43 problem: one human still has to review raw atom volume and relation noise.

Sharper target accepted: create a rebuildable `source-meaning-packet` artifact that compresses many atoms and graph relations into bounded review groups, each with evidence references, route/status, blocker reasons when relevant, and proposal stubs that remain destination-neutral and write-ineligible.

## In Scope

1. A deterministic `mindline documents meaning-packet <corpus-pressure-out-or-parent> --out <dir>` command.
2. A local artifact directory:
   - `source-meaning-packet/meaning-summary.json`
   - `source-meaning-packet/review-packet.md`
   - `source-meaning-packet/evidence-map.json`
   - `source-meaning-packet/blocked-items.json`
   - `source-meaning-packet/groups/*.json`
   - `source-meaning-packet/proposals/*.json`
3. Grouping over existing source-neutral atoms and graph relations, with deterministic IDs and bounded group sizes.
4. Evidence references that use source IDs, atom IDs, line spans, content hashes, and artifact references without raw private excerpts in packet summaries, group files, proposals, evidence maps, blocked-item files, review-packet Markdown, or readback proof.
5. Ready / Needs Review / Blocked sections in the packet.
6. Aggregate packet metrics exposed through eval readback.
7. Runtime proof against the same private-local PR42/PR43 mixed source manifest at `/private/tmp/mindline-wp46-real/mixed-corpus/corpus-pressure-manifest.json`, comparing against the PR43 output at `/private/tmp/mindline-wp48-segment-atomization-v2`.

## Out Of Scope

- New extraction or classifier behavior.
- Gmail- or Slack-specific grouping heuristics.
- LLM calls, network fetching, or link enrichment.
- Destination writes to Product Brain, Tolaria, Notion, Obsidian, Linear, APIs, or local destination folders.
- Auto-acceptance, auto-merge, or no-human claims.
- A full interactive review UI.
- Generalization or DEC-64 claims.

## Behavioral Contract

- Every grouped atom must keep inspectable provenance back to corpus graph atoms and source evidence coordinates.
- Every group must have exactly one review section: `ready`, `needs_review`, or `blocked`.
- Groups with blocked atoms, missing evidence, or unsafe/private markers are blocked.
- Groups with possible duplicate relation pressure are reviewable, not auto-merged.
- Ready groups may emit proposal stubs, but all proposals must be destination-neutral and `write_eligible=false`.
- Packet artifacts and readback evidence must be aggregate/reference/hash-based; raw private body text must not be required for Chain or PR proof and must not appear in generated packet artifacts.
- The packet must preserve PR43 semantic-readiness evidence and add review-compression evidence.

## Key Results

1. Same-dataset packet compresses `208` PR43 atoms into `5-25` review groups.
2. Same-dataset atom-to-group compression ratio is at least `0.85`.
3. Same-dataset relation-review compression ratio is at least `0.95` versus the `20,926` PR43 graph relations.
4. `100%` of groups have evidence references or explicit blocker reasons.
5. Review burden ratio for packet groups is `<= 0.35`.
6. Readback detects `source_meaning_packet_summary` and exposes review group, evidence, compression, and guardrail metrics.
7. Guardrail counters remain zero for destination writes, Product Brain writes, Tolaria writes, hosted inference calls, and hosted telemetry exports.
8. Generated packet artifacts, published proof/readback artifacts, staged changes, Chain/PR text, and committed files contain no raw private Gmail/Slack body text; private runtime packet output stays uncommitted under `/private/tmp`.
9. Focused and full Go tests pass.

## Guardrails

- Do not hardcode Randy, Gmail, Slack, `/private/tmp`, or PR43 corpus identities in product logic.
- Do not tune grouping to private source strings.
- Do not treat same-topic graph density as truth; use it as review grouping context only.
- Do not hide blocked or duplicate-pressure cases inside ready proposals.
- Do not weaken semantic-readiness, proof-gate, or privacy readback behavior.
- Do not claim destination readiness, no-human readiness, or held-out generalization.
