# WP-44 Plan

## 1. Schema and Types

- Add seed summary, seed case, coverage, and private-map schemas.
- Add a seed options struct with `Enabled` and `MaxCases`.
- Keep the existing labeling packet schema compatible with `label-next`, `label-record`, and `label-apply`.

## 2. Seed Builder

- Extend `BuildCorpusAcceptanceLabelingPacket` with options or add an optioned variant.
- Preserve the existing unfiltered path when seed mode is not enabled.
- In seed mode, build the full internal packet, select deterministic seed cases, then write:
  - filtered `labeling-packet.json`;
  - non-independent `answer-key-template.json`;
  - redacted `labeling-report.md`;
  - `seed-summary.json`;
  - `seed-report.md`;
  - `seed-private-map.json`.
- Use stable opaque case refs and bucket rationale labels in durable artifacts.
- Keep original operational refs only in the private map and workflow-compatible packet fields where required for downstream command resolution.

## 3. Selection Algorithm

- Generate candidate review cases from packet candidates.
- Generate zero-candidate source review cases from sources with no candidates.
- Derive source groups from source paths internally but only persist redacted group refs/counts.
- Prioritize coverage buckets before filling remaining slots:
  - source group;
  - candidate kind;
  - confidence band;
  - review status;
  - fallback/needs-review;
  - zero-candidate source review.
- Use deterministic tie-breaking from packet order and stable safe IDs.
- Emit unmet coverage reasons for unavailable buckets.

## 4. CLI

- Extend `documents corpus-acceptance-labeling` with `--seed` and `--max-cases <n>`.
- Reject unrelated flags as before.
- Stdout remains a redacted output summary; include seed summary path and selected count only when seed mode is enabled.

## 5. Tests

- Add synthetic document tests for deterministic selection, bucket coverage, unmet coverage, private marker redaction, and private-map separation.
- Add CLI tests for `--seed --max-cases`, stdout redaction, usage validation, and unfiltered private-marker behavior remaining blocked.
- Add a round-trip test proving the filtered seed packet works through `label-next -> label-record -> label-apply`.

## 6. Real Private Proof

Run:

```bash
go run ./cmd/mindline documents corpus-pressure temp/pb-docs --out /private/tmp/mindline-wp44-pb-docs-pressure
go run ./cmd/mindline documents corpus-acceptance-labeling /private/tmp/mindline-wp44-pb-docs-pressure --out /private/tmp/mindline-wp44-pb-docs-labeling
go run ./cmd/mindline documents corpus-acceptance-labeling /private/tmp/mindline-wp44-pb-docs-pressure --out /private/tmp/mindline-wp44-pb-docs-seed --seed --max-cases 50
```

Expected:

- pressure succeeds with zero side-effect guardrails;
- unfiltered labeling may still fail closed on private markers;
- seed mode succeeds;
- seed selects at most 50 cases;
- seed report/summary are marker-clean;
- private map is local-only;
- no destination writes, Product Brain writes, Tolaria writes, hosted inference, telemetry, auto-accepts, or no-human claims.

## 7. PB Capture and Closeout

- Capture the WP, plan, final proof, blocked claims, and real-data limitation in PB.
- Capture any durable learning about private-safe labeling artifacts and local-only operational maps.
- Request LOOP delivery review before PR.

