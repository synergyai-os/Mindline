# WP-31 Read-Only Slack Corpus Intake Plan

## Status

Implementation plan for the signed WP-31 spec. Keep the slice read-only and corpus-pressure compatible. Do not add destination writes, a Slack API client, UI, auth, DB, or provider tuning.

## Delivery Shape

Add a Slack corpus intake layer beside the existing Slack normalizer.

Primary Go API:

```go
func BuildCorpusIntake(payload Payload, outDir string) (CorpusIntakeSummary, error)
```

Primary CLI:

```text
mindline slack corpus-intake <slack-export.json> --out <dir>
```

Output shape:

```text
<out>/
  slack-corpus-intake/
    intake-summary.json
    intake-report.md
  sources/
    <source-id>/source.md
  corpus-pressure-manifest.json   # only when processed sources > 0
```

## Steps

1. Add Slack intake types in `internal/adapters/slack/types.go`:
   - summary schema;
   - item state/reason codes;
   - guardrail counters;
   - artifact paths.

2. Add intake builder/writer in `internal/adapters/slack/corpus_intake.go`:
   - normalize and sort payload old-to-new;
   - reuse existing candidate normalization for redaction/safety decisions;
   - skip empty messages;
   - block secret-like messages from source Markdown;
   - write one sanitized `source.md` per processed message;
   - write corpus-pressure manifest with `source_kind: markdown` when processed sources exist;
   - suppress the manifest and report an empty manifest path when every message is skipped or blocked;
   - write metadata-only summary and report.

3. Add CLI routing in `internal/cli/runner.go`:
   - usage line;
   - `slack corpus-intake`;
   - argument parsing;
   - JSON stdout envelope.

4. Add tests:
   - adapter-level summary and artifact contract;
   - CLI regression;
   - privacy leak checks;
   - reverse-order input produces old-to-new manifest;
   - intake manifest can feed `documents corpus-pressure`;
   - all-skipped/all-blocked batches produce a clear no-pressure-run report;
   - nested symlinks under the output directory are rejected before private source writes.

5. Run private runtime validation:
   - read Randy self-DM/channel through Slack connector;
   - transform connector output into the existing Slack payload shape under `/private/tmp`;
   - run `slack corpus-intake`;
   - run `documents corpus-pressure` on the generated manifest;
   - report counts only, not raw private message content.

6. Capture Chain truth:
   - WP-31 work package;
   - sign-off decision;
   - implementation decision after verification;
   - relations or text links to `PROD-1`, `DOMAIN-1`, `STR-3`, `INI-1`, `PRI-1`, `BR-1`, `STD-6`, `STD-7`, `STD-12`, `WP-29`, and `WP-30`.

## Review Gates

- Chain Steward: no authority overclaim, no destination write scope, Chain captures present.
- Systems Architect: intake feeds existing corpus-pressure manifest path and does not fork the semantic pipeline.
- Domain/User Job: user can test Slack self-DM/channel intake and bulk Markdown pressure as one product path.
- Risk/Safety: private Slack data is runtime-only; committed fixtures are synthetic; top-level reports are metadata-only.
- Delivery Quality: tests and private smoke prove the slice works end-to-end.

## Expected PR Value

After this PR, Mindline can take Slack capture material, convert it into a local benchmarkable corpus, and run the same pressure/eval machinery already built for Markdown. This is the first practical bridge from Randy's real Slack capture workflow to the measured semantic ingestion loop.
