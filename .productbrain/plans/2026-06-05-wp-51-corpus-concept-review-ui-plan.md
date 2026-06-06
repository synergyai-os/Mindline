# WP-51 Plan: Corpus Concept Review UI

## Scope

Add a deterministic corpus concept index above corpus pressure/graph output, make it human-reviewable in the light review UI, and prove it against the same 25 Gmail + 25 Slack runtime corpus used by WP49/WP50. This iteration keeps the PR46 concept-index scope but raises the review-surface standard: bounded concepts are not enough unless a reviewer can understand and decide them.

PR46 v3 keeps the same PR and dataset, but raises the methodology standard again: a concept is not reviewable merely because its atom snippets are readable. The builder must first prove source-level reviewability, link-only handling, duplicate-source accounting, and readable cross-source coherence.

PR46 v4 raises the cross-source standard again: raw source-kind coverage is not semantic support. A normal cross-source concept requires readable, non-link evidence from at least two source kinds, and readable source groups that do not share core cluster terms must block/split the relation neighborhood.

PR46 v5 raises the same-kind term-bucket standard: generic generated action/title terms such as `prepare` and `checklist` cannot by themselves create a normal review concept, and evidence cards must put clean excerpts before noisy generated summaries.

## Authority Anchors

- DEC-401: WP51 bounded concept index proof exists, but the first Light UI did not prove review usability.
- DEC-403: PR46 v2 delivered a human-reviewable UI iteration whose output exposed the deeper concept-quality failure now addressed by v3.
- INS-28: PR46 review UI must be reviewer-understandable, not machine-readable only.
- INS-29: PR46 concept review needs source-level coherence gates.
- INS-30: PR46 cross-source concepts require readable support across source kinds and no readable outliers.
- INS-31: PR46 generic action term buckets are not reviewable concepts.
- STD-18: inline evidence excerpts are required for reviewer-facing semantic previews.
- STD-19: loopback review writes require Host allowlist, token, same-origin, JSON, write locks, and read-only page endpoints.

This plan is a PR46 remediation of DEC-401's proof gap, not a new autonomy, destination-write, or generalization claim.

## Implementation Plan

1. Add corpus concept model and builder:
   - read the existing corpus pressure summary and graph atoms;
   - group atoms into bounded concepts using candidate kind, route, normalized title/summary terms, and evidence readiness;
   - separate cross-source concepts from single-source/local concepts;
   - preserve evidence refs, atom refs, source-kind counts, content hashes, and short content refs;
   - add a reviewer display model: non-generic title, review prompt, grouping rationale, representative safe evidence previews, and source mix.
   - add source-level evidence groups that collapse multiple atoms from one source into one review card;
   - flag link-only evidence and exclude it from readable semantic support until enriched;
   - require readable term overlap across at least two distinct source-level groups before relation-neighborhood concepts remain normal cross-source review items;
   - block or downgrade incoherent relation-neighborhood clusters with reason codes for weak cross-source coherence, insufficient readable source support, duplicate-source atom support, and link-only evidence requiring enrichment.
   - require readable source-kind support across at least two source kinds before a relation-neighborhood concept can remain normal `cross_source`;
   - detect readable outlier source groups that share no core terms with the coherent source cluster and block/split the concept with a plain reason.
   - block or mark noisy term-based same-kind concepts when their only shared terms are generic generated action/title words and their source cards do not share a meaningful object;
   - make evidence previews expose clean excerpts before generated summaries so the first visible text is reviewable.
2. Add writer:
   - `corpus-concepts/concept-summary.json`;
   - `corpus-concepts/concept-index.json`;
   - `corpus-concepts/review-packet.md`;
   - `corpus-concepts/concepts/<concept-id>.json`;
   - allow a local `review-records.json` file beside the concept index without stale-file rejection;
   - stale-file rejection and symlink/out-dir protections consistent with existing artifact writers.
3. Add CLI:
   - `mindline documents concept-index <corpus-pressure-out-or-parent> --out <dir>`;
   - `mindline documents concept-serve <concept-index-out-or-parent> [--addr 127.0.0.1:8788]`.
4. Add light UI support:
   - load concept summary/index;
   - serve a reviewable HTML view with concept list, progress, filters, source coverage, rationale, representative evidence, and decision controls;
   - show source-level evidence cards before atom-level traces;
   - translate reason codes into plain-English reviewer explanations;
   - keep copy packets source-level so Randy can paste one concept back for review without needing raw artifacts.
   - expose `/api/state` for review state inspection;
   - expose a loopback-only POST endpoint for persisting concept review decisions;
   - keep host allowlist, token, same-origin, JSON body, and serialized write safety consistent with `judge-serve`.
5. Extend eval readback:
   - detect `corpus-concepts/concept-summary.json`;
   - extract concept count, cross-source concept count, concept evidence counts, review burden ratio, compression ratio, source coverage, scale status, guardrails, and fingerprints.
6. Add focused tests:
   - concept grouping combines repeated meaning across source kinds;
   - concept display model produces non-generic titles, rationales, prompts, and representative evidence previews;
   - review records persist and read back without manual JSON editing;
   - weak/single-source groups stay needs-review or local, not overclaimed;
   - the copied PR46 bad concept pattern is blocked/downgraded rather than emitted as a normal cross-source review item;
   - the Modelo 190 packet is blocked/downgraded rather than emitted as a normal cross-source review item when Slack support is link-only and the funding-rounds email is an outlier;
   - the `prepare` packet is blocked/downgraded rather than emitted as a normal same-kind needs-review item when unrelated Gmail snippets only share generic action/title terms;
   - link-only evidence does not count as readable support;
   - link-only source kinds do not count toward readable cross-source-kind support;
   - readable source outliers are flagged as split-needed/blocking evidence;
   - duplicate atoms from the same source collapse into one source-level contribution;
   - writer rejects stale unexpected files and produces expected paths;
   - CLI builds concept artifacts;
   - UI handler serves HTML, `/api/state`, and a token-protected decision POST;
   - readback recognizes concept summary evidence.
7. Runtime proof:
   - verify the mixed corpus manifest contains 25 Gmail and 25 Slack sources;
   - run corpus pressure on the same mixed-source dataset;
   - run concept index;
   - run eval readback and safety/improvement proof gates where applicable;
   - start the light UI server and verify it responds.

## Acceptance Criteria

- `go test ./...` passes.
- The same runtime dataset contains exactly 25 Gmail + 25 Slack sources.
- The 50-source run completes as scale-complete or honestly reports scale status without destination writes.
- Concept index writes all expected artifacts and remains bounded at 8-40 emitted concepts on the 50-source proof.
- Cross-source concept coverage is visible in summary and UI state.
- Concept review burden is at least 99% lower than raw relation count on the same corpus.
- Light UI serves the final concept packet over loopback and renders non-empty concept state with non-generic concept titles, grouping rationale, and representative safe evidence previews.
- Light UI persists at least one reviewer decision through the token-protected POST path, and `/api/state` reflects review progress without mutating read paths.
- The exact copied failure mode is judged by the shipped code after implementation: newsletter notification + link-only LinkedIn Slack save + duplicate Gmail meeting-summary atoms must be marked blocked or under-supported, with source-level evidence cards and plain-English reasons.
- The exact Modelo 190 failure mode is judged by the shipped code after implementation: Gmail tax/admin sources + Gmail promo/funding digest + unread Slack LinkedIn URLs must not remain a normal `cross_source` item.
- The exact `prepare` failure mode is judged by the shipped code after implementation: founder work time + June invoice timing + proof-of-payment snippets must not remain a normal `needs_review` concept just because generated action text repeats `prepare`.
- Source-level evidence cards are present in JSON, UI state, and copied packets for review.
- Source evidence cards and copied packets present clean excerpts before generated summaries, and do not lead with duplicated/truncated generated summary text.
- Raw reason codes are accompanied by plain-English reason labels in the UI/copy packet.
- Product Brain captures spec/plan sign-off and delivery proof.

## Risks

- Deterministic concept grouping may be coarse. This is acceptable for WP51 if the summary exposes needs-review status and does not claim held-out accuracy.
- If the 50-source private runtime corpus has sparse repeated wording, cross-source concept count may be lower than desired. The proof should report the actual result and block overclaiming.
- Adding excerpts creates private-data handling risk. The mitigation is to reuse existing unsafe marker redaction, keep artifacts local/private and uncommitted, preserve metadata-only hosted observability, and keep review server loopback-only.
- Persisted reviewer decisions can become misleading if concept labels remain generic. The mitigation is to block acceptance on a non-generic display model plus representative evidence, not just buttons.
- Source-level coherence gates can reduce the apparent cross-source concept count. That is acceptable; a lower number of reviewable concepts is better than asking Randy to judge obvious junk.
- Deterministic term-overlap coherence is still not semantic truth. The mitigation is to use it only as a fail-closed sanity gate and avoid any held-out quality, destination-write, or no-human claim.
- Readable source-kind and outlier gates may block nearly all current relation-neighborhood concepts. That is acceptable for this PR: honest blocked review work is better than false cross-source confidence.
- Generic-term blocking may hide some weak but potentially useful action clusters. That is acceptable until Mindline has stronger labeled action-object extraction; false reviewability is worse than fewer normal concepts.

## Reviewer Sign-Off Targets

- Product/Domain reviewer: confirms the UI-visible behavior answers the user's methodology question.
- Systems reviewer: confirms the concept layer sits above graph/packet output and remains source/destination agnostic.
- Eval/Safety reviewer: confirms metrics, readback, privacy, and claim limits block overclaiming.
- Delivery Quality reviewer: confirms the UI and API are usable enough to support actual concept review, not only artifact inspection.
