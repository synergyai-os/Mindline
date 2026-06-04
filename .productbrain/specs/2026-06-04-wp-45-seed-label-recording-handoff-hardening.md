# WP-45 Seed-label recording handoff hardening

## Outcome

Mindline must make the private-safe seed label handoff auditable and fail-closed after PR 39.

PR 39 already lets labels recorded against seed aliases apply back to original corpus-compatible answer-key artifacts through `seed-private-map.json`. The next gap is not rehydration itself. The gap is that `label-apply` does not yet expose safe aggregate proof that seed mode was detected, the private map was used, aliases were translated, and the resulting output is private-local rather than benchmark-ready durable proof.

This WP hardens the boundary from private-safe seed labeling records to local corpus-acceptance answer-key artifacts.

## Product Layer

Evaluation/readback and corpus acceptance labeling workflow.

This is not a new source adapter, destination adapter, review UI, semantic classifier, Slack/Gmail ingestion path, or no-human autonomy step.

## Scope

Update:

```bash
mindline documents corpus-acceptance-label-apply <corpus-acceptance-labeling-out-or-parent> --records <label-records.json> --out <dir>
```

for seed-mode labeling packets so that apply output is explicit, fail-closed, and safely consumable by the corpus-acceptance benchmark path.

## Contracts

1. Seed-mode apply without a valid matching `seed-private-map.json` fails before writing `answer-key.json`.
2. Seed-map-backed apply output is classified as `local_private_rehydrated`.
3. Seed-map-backed apply output is rejected after symlink resolution under durable repository, Product Brain, or git-visible roots, including current workspace descendants, `.productbrain`, `testdata`, and durable report roots.
4. Stdout, Markdown reports, PB captures, PR descriptions, and durable discussion surfaces may contain only safe aggregate proof fields, not original private refs.
5. Non-seed label-apply behavior remains backward compatible.
6. `original_corpus_compatible=true` means compatible with local corpus-acceptance artifact shape and original corpus refs after private-map translation. It does not mean benchmark-ready, held-out, generalizable, DEC-64 eligible, destination-write-ready, or no-human-ready.

## Artifact Confidentiality

Use enum-like values:

- `local_private_rehydrated`: seed-map-backed apply output tree that may contain original refs, including answer-key JSON, recording summary JSON, Markdown report sidecar, and any generated apply artifact under that output tree.
- `private_safe_redacted`: safe aggregate stdout/report/PB/PR evidence with no original refs or private-map contents.
- `non_seed_local`: non-seed local recording output.
- `blocked`: failed apply before private artifacts are written.

## Summary And Report Fields

Add safe aggregate fields to recording summary, CLI output, and report:

- `seed_mode`
- `seed_private_map_status`
- `original_corpus_compatible`
- `translated_source_count`
- `translated_expected_outcome_count`
- `translated_evidence_ref_count`
- `artifact_confidentiality`

These fields must not include source IDs, candidate IDs, evidence node IDs, source paths, local temp paths, source excerpts, private-map entries, PB entry IDs, Slack/Gmail IDs, or destination locators.

## Key Results

- KR1: Seed-mode label apply with a valid private map emits aggregate rehydration proof fields and marks the apply output `local_private_rehydrated`.
- KR2: Seed-mode label apply without a valid matching private map fails closed before writing answer-key artifacts.
- KR3: Seed-map-backed apply output under workspace, `.productbrain`, `testdata`, or other durable/git-visible roots is rejected after symlink resolution.
- KR4: A CLI end-to-end test proves `labeling --seed -> label-next -> label-record -> label-apply -> corpus-acceptance` works without hand editing and that the benchmark consumer counts the applied label.
- KR5: The same end-to-end proof keeps `suite_valid=false` and `dec64_eligible=false` for expected reasons when labels are non-independent or below minimum count/source gates.
- KR6: Stdout, stderr, Markdown report, durable JSON summaries, and failure/error messages leak no original refs, source paths, private-map contents, or private markers.
- KR7: Real private runtime proof uses `temp/pb-docs` and keeps private artifacts under `/private/tmp`; PB/PR receives metadata-only proof.

## Claim Boundaries

Allowed claim:

- Mindline can make seed-label recording handoff private-local, auditable, fail-closed, and consumable by the corpus-acceptance benchmark path.

Blocked claims:

- semantic correctness;
- benchmark readiness;
- held-out accuracy;
- generalization;
- DEC-64 readiness;
- no-human autonomy;
- destination-write readiness;
- durable reusable benchmark fixture readiness from Randy's private corpus.

## Verification Projection

Users: operators preparing private corpus labels for local benchmark-format evaluation.

Inputs: private-safe seed labeling artifacts from real `temp/pb-docs` corpus-pressure output and synthetic test fixtures.

Outputs: private-local rehydrated answer-key/recording artifacts plus redacted aggregate status/proof.

Workspace assumptions: Product Brain remains SSOT; private rehydrated artifacts stay outside repo/PB/git-visible paths.

Provider/model assumptions: deterministic local path only; no hosted inference requirement.

Privacy boundary: original refs may exist only in private-local artifacts. PB, PRs, stdout intended for durable reading, and Markdown reports receive safe aggregates only.

Held-out/generalization status: blocked unless independent provenance, min eval count, min source count, held-out, and DEC-64 gates all pass.
