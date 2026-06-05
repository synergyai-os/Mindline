# WP-51 Corpus Concept Review UI

## Chain Authority

- INS-27: next mixed-source proof needs bounded concept review, not relation flood.
- DEC-400: PR 45 merged into main on 2026-06-05.
- DEC-391: WP50 delivered bounded corpus scale gates.
- DEC-386: WP49 delivered source meaning review packets on the same 25 Gmail + 25 Slack runtime corpus.
- DEC-153: Mindline should use local corpus graph/index proof before hosted auth/database work.
- PRI-1, BR-1, STD-17, DEC-64: local/private-safe proof, no destination writes, no hosted leakage, no no-human claims without held-out gates.

## Problem

WP49 made 208 atoms and 20,926 graph relations reviewable as 19 source meaning groups on the 50-item mixed Gmail/Slack corpus. WP50 made oversized corpus runs bounded and honest. The remaining gap is methodology: pairwise `same_topic_as` relations are useful diagnostic substrate, but they are not the user-facing way Randy should evaluate repeated meaning across real inputs.

If one email or Slack message has many atoms, and many items repeat the same idea, the review surface should show a bounded concept with evidence coverage. It should not force the user to inspect relation floods or infer whether the system combined repeated meaning correctly.

## Outcome

Mindline produces a bounded corpus concept index above the existing corpus graph and source meaning packet:

- concept groups combine related atoms across sources using deterministic, source-neutral signals;
- each concept exposes source count, atom count, evidence references, source-kind coverage, and review status;
- cross-source concepts are clearly separated from local/single-source concepts;
- the concept index writes a compact artifact set and markdown review packet;
- `eval readback` recognizes the concept index as proof evidence;
- the light review UI can serve the concept review packet so Randy can inspect the same 25 Gmail + 25 Slack run in a browser.

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

## Measurable Behavior Difference

Before WP51: the system could say 20,926 relations existed and compress them into a packet, but Randy still could not evaluate whether repeated ideas were combined into coherent corpus concepts in the light UI.

After WP51: Randy can open the light UI and review a bounded concept index for the same 25 Gmail + 25 Slack corpus, seeing which ideas repeat across sources, what evidence supports them, and what still needs human review.

## Guardrails

- Concept grouping must be source-agnostic and destination-neutral.
- Concept artifacts must use evidence IDs, source IDs, line spans, hashes, and short content refs, not raw private body excerpts.
- Concept output is review-only and write-ineligible.
- Single-source or weak concepts must be marked as needs review rather than overclaimed as cross-source knowledge.
- The UI must remain loopback-only and preserve the existing review-server host/token safety discipline for writes if writes are added later.
- Do not commit private runtime artifacts.

## Re-Challenge And Reconciliation

First target: show the existing source meaning packet in the light UI.

Rejected as too weak. That would make WP49 easier to browse but would not answer the hard question from the user: one email or Slack message can contain many atoms, and repeated ideas across many items need to be flattened, combined, and evaluated as concepts.

Second target: add a concept artifact without UI.

Rejected as incomplete. The user explicitly needs to see the result in the light UI to evaluate methodology. Artifact-only output would preserve the same gap: the system produces proof that is hard to judge.

Sharper target: build a bounded corpus-level concept index and serve it in the light UI for the same 25 Gmail + 25 Slack dataset. This raises the standard from "we processed 50 sources" to "we can inspect whether repeated meaning is being combined into reviewable, evidence-backed concepts."
