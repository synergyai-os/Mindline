# WP-51 Plan: Corpus Concept Review UI

## Scope

Add a deterministic corpus concept index above corpus pressure/graph output, make it visible in the light review UI, and prove it against the same 25 Gmail + 25 Slack runtime corpus used by WP49/WP50.

## Implementation Plan

1. Add corpus concept model and builder:
   - read the existing corpus pressure summary and graph atoms;
   - group atoms into bounded concepts using candidate kind, route, normalized title/summary terms, and evidence readiness;
   - separate cross-source concepts from single-source/local concepts;
   - preserve evidence refs, atom refs, source-kind counts, content hashes, and short content refs.
2. Add writer:
   - `corpus-concepts/concept-summary.json`;
   - `corpus-concepts/concept-index.json`;
   - `corpus-concepts/review-packet.md`;
   - `corpus-concepts/concepts/<concept-id>.json`;
   - stale-file rejection and symlink/out-dir protections consistent with existing artifact writers.
3. Add CLI:
   - `mindline documents concept-index <corpus-pressure-out-or-parent> --out <dir>`;
   - `mindline documents concept-serve <concept-index-out-or-parent> [--addr 127.0.0.1:8788]`.
4. Add light UI support:
   - load concept summary/index;
   - serve a compact HTML view with concept list, source coverage, evidence counts, status, and detail panel;
   - expose `/api/state` for review state inspection;
   - keep loopback host validation consistent with `judge-serve`.
5. Extend eval readback:
   - detect `corpus-concepts/concept-summary.json`;
   - extract concept count, cross-source concept count, concept evidence counts, review burden ratio, compression ratio, source coverage, scale status, guardrails, and fingerprints.
6. Add focused tests:
   - concept grouping combines repeated meaning across source kinds;
   - weak/single-source groups stay needs-review or local, not overclaimed;
   - writer rejects stale unexpected files and produces expected paths;
   - CLI builds concept artifacts;
   - UI handler serves HTML and `/api/state`;
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
- Light UI serves the final concept packet over loopback and renders non-empty concept state.
- Product Brain captures spec/plan sign-off and delivery proof.

## Risks

- Deterministic concept grouping may be coarse. This is acceptable for WP51 if the summary exposes needs-review status and does not claim held-out accuracy.
- If the 50-source private runtime corpus has sparse repeated wording, cross-source concept count may be lower than desired. The proof should report the actual result and block overclaiming.
- Adding UI without write recording could feel incomplete. For WP51 the target is inspectability; human accept/reject recording can be the next PR if the review surface is useful.

## Reviewer Sign-Off Targets

- Product/Domain reviewer: confirms the UI-visible behavior answers the user's methodology question.
- Systems reviewer: confirms the concept layer sits above graph/packet output and remains source/destination agnostic.
- Eval/Safety reviewer: confirms metrics, readback, privacy, and claim limits block overclaiming.
