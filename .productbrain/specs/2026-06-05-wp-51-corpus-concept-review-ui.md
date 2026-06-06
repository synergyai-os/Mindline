# WP-51 Corpus Concept Review UI

## Chain Authority

- DEC-401: WP51 delivery proved a bounded concept index and Light UI on the same 25 Gmail + 25 Slack corpus, but did not prove the review surface was usable.
- INS-28: PR46 review UI must be reviewer-understandable, not machine-readable only.
- INS-29: PR46 concept review needs source-level coherence gates.
- STD-18: reviewer-facing semantic previews need inline evidence excerpts, not only evidence IDs.
- STD-19: loopback review write APIs need Host allowlist, token, same-origin, JSON, write locks, and read-only page endpoints.
- INS-27: next mixed-source proof needs bounded concept review, not relation flood.
- DEC-400: PR 45 merged into main on 2026-06-05.
- DEC-391: WP50 delivered bounded corpus scale gates.
- DEC-386: WP49 delivered source meaning review packets on the same 25 Gmail + 25 Slack runtime corpus.
- DEC-153: Mindline should use local corpus graph/index proof before hosted auth/database work.
- PRI-1, BR-1, STD-17, DEC-64: local/private-safe proof, no destination writes, no hosted leakage, no no-human claims without held-out gates.

This PR46 iteration remediates the gap in DEC-401: bounded concept output existed, but the first review UI did not satisfy INS-28's human-reviewable standard.

This PR46 v3 iteration remediates the gap INS-29 exposed: a readable UI can still fail the human-review job when the concept candidate itself is a noisy atom-neighborhood cluster.

## Problem

WP49 made 208 atoms and 20,926 graph relations reviewable as 19 source meaning groups on the 50-item mixed Gmail/Slack corpus. WP50 made oversized corpus runs bounded and honest. The remaining gap is methodology: pairwise `same_topic_as` relations are useful diagnostic substrate, but they are not the user-facing way Randy should evaluate repeated meaning across real inputs.

If one email or Slack message has many atoms, and many items repeat the same idea, the review surface should show a bounded concept with evidence coverage. It should not force the user to inspect relation floods or infer whether the system combined repeated meaning correctly.

## Outcome

Mindline produces a bounded corpus concept index above the existing corpus graph and source meaning packet:

- concept groups combine related atoms across sources using deterministic, source-neutral signals;
- each concept exposes a non-generic reviewer title, a one-sentence review prompt, grouping rationale, source count, atom count, source-kind coverage, review status, and representative safe evidence excerpts;
- cross-source concepts are clearly separated from local/single-source concepts;
- the concept index writes a compact artifact set and markdown review packet;
- `eval readback` recognizes the concept index as proof evidence;
- the light review UI can serve the concept review packet so Randy can inspect the same 25 Gmail + 25 Slack run in a browser;
- the light review UI lets the reviewer record a decision for each concept: accept, reject as noisy, split needed, merge duplicate, rename needed, or needs more source context.
- evidence is collapsed into source-level review cards before presentation, so repeated atoms from the same source are visible as one source contribution rather than false independent support;
- link-only source evidence is excluded from readable semantic support and flagged as needing enrichment instead of being treated as understood content;
- relation-neighborhood concepts must show readable shared meaning across at least two distinct source-level groups before they can remain normal cross-source review concepts;
- incoherent or under-supported cross-source groups are blocked or downgraded with plain-English reason labels before the human reviewer is asked to judge them.

## Non-Outcome

WP51 does not claim perfect semantic clustering, held-out quality, autonomous acceptance, destination readiness, Product Brain writes, Tolaria writes, or no-human operation. It does not replace future labeled acceptance. It is a private-runtime, sample-bound proof that the next human review surface is bounded corpus-level concepts instead of raw relation volume.

## Key Results

1. The same mixed-source runtime corpus used by WP49/WP50 processes exactly 50 sources: 25 Gmail and 25 Slack.
2. `mindline documents concept-index <corpus-pressure-out-or-parent> --out <dir>` writes `corpus-concepts/concept-summary.json`, `concept-index.json`, `review-packet.md`, and per-concept JSON files.
3. The concept summary reports a bounded concept count within 8-40 groups for the 50-source corpus.
4. At least one concept has evidence from more than one source kind, and cross-source concepts report source coverage explicitly.
5. The user-facing review burden is lower than relation review volume by at least 99% on the same 50-source corpus.
6. `mindline documents concept-serve <concept-index-out-or-parent> --addr <loopback>` serves the review UI and `/api/state` for that concept packet.
7. `eval readback` detects the concept index summary, concept metrics, scale status, guardrails, and private-safe fingerprints.
8. Guardrails stay zero for destination writes, Product Brain writes, Tolaria writes, hosted inference calls, hosted telemetry exports, auto-accepts, and no-human claims.
9. Every emitted concept has a display title that is not the generic relation-neighborhood machine label, plus a grouping rationale that explains why the atoms were grouped.
10. Every emitted concept exposes at least three representative safe evidence previews when at least three evidence refs exist, including both Gmail and Slack previews for cross-source concepts when both are available.
11. The UI shows review progress and can persist one reviewer decision plus an optional note for each concept without editing JSON manually.
12. A reviewer can understand what the selected concept is about, why it exists, what evidence supports it, and what decision is being asked for without opening raw artifacts.
13. The copied bad concept `concept-0bd57f279ab994b2` must no longer present as a normal cross-source concept that asks Randy to decide whether unrelated newsletter, link-only LinkedIn, and duplicate meeting-summary atoms form one coherent concept.
14. For every emitted concept, the UI and copy packet show source-level evidence cards first, including source kind/ref, atom count, source flags, readable excerpts, and traces.
15. Link-only evidence that has not been enriched is labeled as link-only and does not count toward cross-source semantic coherence.
16. Multiple atoms from the same source are counted as one source-level contribution for support, with duplicate-source atom support called out explicitly.
17. Reason codes shown to Randy have plain-English explanations; raw machine codes may remain as trace metadata but cannot be the only explanation.

## Measurable Behavior Difference

Before WP51: the system could say 20,926 relations existed and compress them into a packet, but Randy still could not evaluate whether repeated ideas were combined into coherent corpus concepts in the light UI.

After WP51: Randy can open the light UI and review a bounded concept index for the same 25 Gmail + 25 Slack corpus, seeing which ideas repeat across sources, why the system grouped them, readable representative evidence, and a concrete decision surface for accepting, rejecting, splitting, merging, renaming, or requesting more context.

After PR46 v3: Randy should not have to reject obvious junk clusters by hand. If Mindline only has a newsletter notification, a bare LinkedIn URL, and duplicate atoms from one meeting-summary email, the UI should say the concept is blocked or under-supported, explain why, and show the source-level evidence that led to that judgment.

## Guardrails

- Concept grouping must be source-agnostic and destination-neutral.
- Concept artifacts must keep traceability through evidence IDs, source IDs, line spans, hashes, and short content refs.
- The reviewer-facing surface must include representative safe excerpts because evidence IDs alone are not reviewable evidence. Excerpts must pass the existing unsafe marker redaction and remain local/private runtime artifacts, not hosted telemetry or committed fixtures.
- Concept output is review-only and write-ineligible.
- Single-source or weak concepts must be marked as needs review rather than overclaimed as cross-source knowledge.
- Weak cross-source relation neighborhoods must be blocked or downgraded when readable source-level evidence does not share meaning across distinct source groups.
- Link-only sources are provenance, not semantic understanding, until enriched; they must be flagged and excluded from coherence support.
- Duplicate atoms from one source must not inflate independent source support.
- The UI must remain loopback-only and preserve the existing review-server host/token/same-origin/JSON/write-lock safety discipline for persisted review decisions.
- Do not commit private runtime artifacts.

## Re-Challenge And Reconciliation

First target: show the existing source meaning packet in the light UI.

Rejected as too weak. That would make WP49 easier to browse but would not answer the hard question from the user: one email or Slack message can contain many atoms, and repeated ideas across many items need to be flattened, combined, and evaluated as concepts.

Second target: add a concept artifact without UI.

Rejected as incomplete. The user explicitly needs to see the result in the light UI to evaluate methodology. Artifact-only output would preserve the same gap: the system produces proof that is hard to judge.

Sharper target: build a bounded corpus-level concept index and serve it in the light UI for the same 25 Gmail + 25 Slack dataset. This raises the standard from "we processed 50 sources" to "we can inspect whether repeated meaning is being combined into reviewable, evidence-backed concepts."

## PR46 Iteration Diagnosis

Randy rejected the first PR46 draft because the Light UI was a machine-readable artifact browser, not a human review service. It repeated generic "Cross-source topic candidate relation neighborhood" labels, showed raw evidence refs, source IDs, line spans, and hashes as the primary content, and had no plain-English concept distinction, no grouping explanation, no representative excerpts, no decision controls, and no persisted reviewer outcome.

That failed the actual job. A human reviewer must be able to understand the proposed concept, judge whether the evidence belongs together, and record a decision directly in the review surface. A compact concept count is necessary but not sufficient.

## PR46 Iteration Re-Challenge And Reconciliation

First iteration target: add readable evidence excerpts to the existing detail panel.

Rejected as too weak. Excerpts would make the page less opaque, but Randy would still lack the decision workflow and the system would still not know what the reviewer approved, rejected, split, merged, or renamed.

Second iteration target: add decision buttons and notes to the existing generic concept page.

Rejected as too weak. Recording decisions against indistinguishable generic labels would create review data, but the decisions would be low-quality because the reviewer would still be judging machine buckets rather than named, explained concepts.

Sharpened iteration target: promote a reviewer display model inside the concept artifact and build a complete local review loop around it. Each concept needs a non-generic title, review prompt, grouping rationale, representative safe evidence previews, source mix, quality flags, review controls, and persisted review records. This raises the bar from "bounded concepts exist" to "a human can actually review them."

## PR46 v3 Iteration Diagnosis

Randy's copied review packet proved the v2 UI still let an incoherent concept through as normal review work. The item combined a SaaS newsletter notification, a Slack message containing only a LinkedIn URL, another near-duplicate link-only Slack atom, and duplicate atoms from the same Gmail meeting-summary source. The UI was readable enough to expose the issue, but the concept methodology still treated atom-neighborhood membership as enough to ask a human whether the group was coherent.

That fails the higher review-service standard. A human reviewer should review borderline meaning, not do first-pass cleanup of obvious source-level incoherence.

## PR46 v3 Re-Challenge And Reconciliation

First v3 target: only make the copied packet easier to read.

Rejected as too weak. The copied packet is readable enough to know it is bad; improving labels alone would still push junk to the reviewer.

Second v3 target: add a "Noisy" default decision for suspicious concepts.

Rejected as too weak. That records the failure but does not improve the methodology. The system already has enough signals to see link-only evidence, duplicate-source support, and lack of shared readable source-level meaning.

Sharpened v3 target: add source-level coherence gates before presentation. Concepts must be built and displayed around distinct source contributions. Link-only items become enrichment blockers, duplicate atoms remain trace detail rather than independent support, and relation-neighborhood clusters without shared readable meaning are blocked or downgraded with plain-English reasons. This raises the bar from "a human can read the candidate" to "the system only asks the human to review candidates that passed basic source-level sanity."
