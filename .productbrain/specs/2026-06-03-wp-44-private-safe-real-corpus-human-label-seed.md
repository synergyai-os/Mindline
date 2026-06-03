# WP-44 Private-safe real corpus human label seed

## Outcome

Mindline must turn real private corpus-pressure artifacts into a bounded, representative, private-safe human labeling seed without weakening privacy guards or claiming semantic correctness.

PR 38 proved the real `temp/pb-docs` corpus can process without blockers. The next real blocker is sharper: the current unfiltered `corpus-acceptance-labeling` handoff can fail on that same corpus with `corpus acceptance labeling packet contains private marker`, because real Product Brain-style Markdown can contain governance identifiers and private operational refs. This WP adds an opt-in seed mode that succeeds by redacting durable outputs and separating local-only operational refs.

## Product Layer

Evaluation / corpus acceptance labeling.

This is not document decomposition, semantic extraction, a new review UI, a destination adapter, a Slack/Gmail adapter, or an answer-key generator.

## Scope

Add an explicit seed mode to the existing corpus acceptance labeling path:

```bash
mindline documents corpus-acceptance-labeling <corpus-pressure-out-or-parent> --out <dir> --seed --max-cases 50
```

Seed mode consumes existing corpus-pressure artifacts and emits a bounded filtered labeling packet plus seed artifacts. It must remain compatible with the existing `label-next -> label-record -> label-apply` workflow.

## Non-Negotiables

1. Private marker detection is not relaxed, bypassed, or disabled.
2. Existing unfiltered labeling behavior remains unchanged unless `--seed` is explicitly selected.
3. Durable seed outputs contain no raw source text, source labels, private paths, governance IDs, external message IDs, emails, permalinks, candidate IDs, or evidence node IDs.
4. Operational refs needed to map human labels back to original artifacts live only in local-only map artifacts under the chosen output directory and are never printed to stdout or Markdown reports.
5. Seed selection is deterministic: same input artifacts and same seed config produce stable selected cases, bucket assignments, and summary counts.
6. Every selected case has a redacted rationale bucket explaining why it was chosen.
7. The output states that no auto-labeling occurred and no answer key, held-out accuracy, generalization, DEC-64, no-human autonomy, or destination-write readiness claim is made.

## Seed Selection Rules

Seed mode must select up to `--max-cases` cases from available labeling candidates and zero-candidate source review cases. Default `--max-cases` is 50.

Selection must prioritize representative coverage before filling remaining slots. Available buckets include:

- source group;
- candidate kind;
- confidence band;
- review status;
- fallback or needs-review cases;
- zero-candidate source review cases;
- excluded source review cases when upstream artifacts expose enough local provenance to review them safely.

When a requested bucket is not available in the input corpus, the seed summary must report it as unavailable instead of implying full coverage.

## Artifacts

Seed mode writes under `corpus-acceptance-labeling/`:

- `labeling-packet.json`: filtered, workflow-compatible labeling packet for selected cases.
- `answer-key-template.json`: generated template with non-independent provenance, still not held-out ready.
- `labeling-report.md`: redacted human-readable report.
- `seed-summary.json`: metadata-only seed summary with input fingerprints, schema version, selection version, max case count, selected/unselected counts, coverage counts, unmet coverage reasons, and claim boundaries.
- `seed-report.md`: metadata-only rationale report with redacted case refs and bucket reasons.
- `seed-private-map.json`: local-only operational map containing original source/candidate/evidence refs needed for later label workflow resolution.

`seed-private-map.json` may contain private refs because it is a local runtime artifact. It must not be printed to stdout, copied into PB, or committed as real private evidence.

## Key Results

- KR1: `--seed --max-cases 50` succeeds on the real `temp/pb-docs` pressure artifacts that currently cause unfiltered labeling to fail closed.
- KR2: Existing unfiltered `corpus-acceptance-labeling` still rejects private-marker-bearing real artifacts unless seed mode is explicitly selected.
- KR3: Seed selection emits at most the requested max cases, deterministically, with redacted rationale buckets for every selected case.
- KR4: Seed summary reports coverage counts and unavailable bucket reasons for source group, candidate kind, confidence band, review status, fallback/needs-review, and zero-candidate source review classes.
- KR5: The filtered seed packet can feed the existing `label-next -> label-record -> label-apply` workflow without a parallel review path.
- KR6: Automated synthetic tests prove stdout and Markdown/summary seed artifacts leak zero raw private refs, while local-only maps preserve the operational refs needed for workflow resolution.
- KR7: Real private runtime proof records only metadata counts and keeps all private artifacts under `/private/tmp`.

## Claim Boundaries

Allowed claim:

- Mindline can create a private-safe, bounded, representative human labeling queue from real private corpus artifacts.

Blocked claims:

- semantic correctness;
- held-out accuracy;
- generalization;
- DEC-64 readiness;
- no-human autonomy;
- destination-write readiness;
- Product Brain, Tolaria, Slack, Gmail, or other destination write readiness.

## Verification Projection

Users: operators preparing private corpus material for human semantic evaluation.

Inputs: current Markdown corpus-pressure artifacts, with real private proof from `temp/pb-docs`.

Outputs: local seed artifacts and a filtered labeling packet, not an answer key or destination proposal.

Workspace assumptions: Product Brain remains SSOT; real private artifacts stay under `/private/tmp`.

Provider/model assumptions: deterministic local classifier path only; no hosted inference requirement.

Privacy boundary: PB and PR receive metadata-only counts. Local-only map may contain operational refs and is not durable proof.

Held-out/generalization status: blocked until independent human labels are recorded and accepted by corpus acceptance gates.

