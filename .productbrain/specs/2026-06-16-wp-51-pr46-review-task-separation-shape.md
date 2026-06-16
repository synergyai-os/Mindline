# WP-51 PR46 v6 Shape: Review Task Separation

## Authority

- User evidence, 2026-06-16: Randy still finds the PR46 Mindline Concept Review UI hard or impossible to review.
- INS-32: local single-source concepts are not reviewable corpus concepts.
- INS-28 through INS-31: prior PR46 iterations made artifacts readable, added source-level coherence gates, rejected false cross-source support, and blocked generic action buckets.
- DEC-401 through DEC-406: prior proof reduced relation volume and added multiple gates, but later user review still found the queue not reviewable.
- STD-18: reviewer-facing semantic previews need inline evidence.
- STD-19, PRI-1, BR-1: review UI and proof must remain local/private-safe, loopback-only, token-protected for writes, and metadata-only externally.

## Sharp Problem

PR46 is still mixing different human jobs into one queue. The UI presents local single-source term buckets, blocked or under-supported groups, enrichment backlog, duplicate-source trace detail, and actual corpus concept candidates with the same review prompt and the same Accept/Split/Merge/Rename/Need-context controls.

That makes the review task incoherent. A reviewer cannot usefully "accept" a local single-source bucket as a corpus concept when the system already says only one source supports it. The right action is cleanup routing, enrichment, ignore/discard, or diagnostic inspection, not concept acceptance.

## Product Frame

The core product behavior is source-neutral review workflow classification above the corpus graph. Destination adapters are out of scope. Tolaria and Product Brain writes are out of scope. The review UI is local proof tooling for evaluating whether Mindline can present evidence-backed candidates, not an accepted-knowledge destination.

## Selected Direction

Split the artifact/UI model by review work kind:

1. `concept_review`: a normal corpus concept candidate that passed basic source-level gates and can be judged for coherence.
2. `cleanup_triage`: local, single-source-kind, generic, duplicate-heavy, or otherwise weak groups that can help improve extraction but are not corpus concept decisions.
3. `enrichment_backlog`: link-only or unread source support that may become reviewable only after enrichment.
4. `blocked_diagnostic`: unsafe, missing-evidence, incoherent, or unsupported groups retained for traceability and debugging.

The main review queue should default to `concept_review`. Other work kinds remain visible, countable, copyable, and inspectable, but they must have their own prompts, labels, counts, and decision controls.

## Outcome

A reviewer can open the PR46 review surface and immediately understand what work is being asked:

- corpus concept validation asks whether evidence from distinct readable sources supports one concept;
- cleanup triage asks whether to discard, split, rename, or use the group as extraction feedback;
- enrichment backlog asks whether to enrich/read sources before concept review;
- blocked diagnostics explain why the group is not review work.

The screenshot failure is resolved only when the local single-source `workspace`-style item cannot appear as a normal concept-review item and cannot offer a misleading Accept action.

## Proof Expectations

- Focused unit tests for review work kind classification.
- UI/API tests proving filters, prompts, counts, and controls differ by work kind.
- A regression fixture matching the screenshot: one local source, two atoms, duplicate-source support, `single_source_concept`; expected `cleanup_triage` or `blocked_diagnostic`, not `concept_review`.
- Fresh runtime proof on the same 25 Gmail + 25 Slack corpus.
- Fresh `/api/state` proof from a live server pointing at an existing artifact path.
- Updated PR body reflecting DEC-403 through DEC-406 and the new v6 proof, not stale DEC-401-only claims.

## Stop Point

This shape authorizes a spec and plan update only. It does not authorize implementation until the updated spec and plan are captured on Chain and pass LOOP review.
