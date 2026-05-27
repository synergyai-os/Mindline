# WP-31 Read-Only Slack Corpus Intake

## Status

Signed target for the next PR after WP-30. This slice connects Slack source intake to the existing corpus-pressure and corpus-acceptance path. It does not write to Tolaria, Product Brain, or any destination.

## Authority

- `PROD-1`: Mindline is the headless knowledge-processing engine between capture sources and downstream destinations.
- `DOMAIN-1`: Product Brain and Tolaria are downstream destinations, not the product.
- `STR-3`: autonomy-readiness before destination writes.
- `INI-1`: Semantic autonomy readiness cycle.
- `PRI-1` and `BR-1`: privacy by design and traceability by design; hosted observability remains metadata-only.
- `STD-6`: Slack backlog processing runs old-to-new in small checkpointed batches.
- `STD-7`: Slack ingestion skips empty artifacts and never persists secret-looking snippets.
- `STD-12`: private provenance visibility is authoritative for publish blocking.
- `WP-29`: corpus-pressure can process Markdown directories or manifests into eval-ready local artifacts.
- `WP-30`: corpus-acceptance can evaluate corpus-pressure output against held-out expectations.

## Diagnosis

Mindline can now measure document-corpus extraction quality, but the measurement path is not yet attached to the real source surface Randy cares about: Slack self-DM/channel captures. The existing Slack adapter can normalize a prepared Slack payload into SBOS candidates, but it does not produce a corpus-pressure-compatible source bundle. That leaves a gap between "I captured items in Slack" and "Mindline can evaluate whether those captures became useful semantic candidates."

The next PR should close that read-only gap without adding a Slack API client to the repo. Slack access remains an external adapter/runtime concern. Mindline accepts a Slack export payload and turns it into sanitized local Markdown source artifacts plus a corpus manifest, so the existing document pressure and acceptance commands can evaluate it.

## Outcome

Given a Slack self-DM or channel export payload, Mindline can produce a local intake bundle that answers:

> Did Mindline account for this Slack batch and convert it into benchmarkable source material without leaking private content or writing to a destination?

The output is a corpus-pressure-compatible manifest when at least one Slack message is eligible for processing, one Markdown source per eligible Slack message or thread unit, a machine-readable intake summary, and a human-readable intake report. If all messages are skipped or blocked, Mindline must emit the summary/report only and state that no pressure manifest was produced because there are no eligible sources.

## Scope

Implement:

1. `mindline slack corpus-intake <slack-export.json> --out <dir>`.
2. A Slack corpus intake model that:
   - reuses the existing Slack payload contract;
   - preserves workspace, channel ID/name, message timestamp, author, permalink/raw locator, and old-to-new ordering in metadata;
   - produces stable source IDs from source-native Slack identity;
   - accounts for every input message as processed, skipped, or blocked with closed reason codes;
   - writes sanitized Markdown source files under the output directory;
   - writes a `corpus-pressure-manifest/v0.1` manifest consumable by `documents corpus-pressure` when at least one source is processed;
   - suppresses the manifest and reports `manifest_path=""` when there are no processed sources.
3. A summary/report with:
   - input count, processed/skipped/blocked counts;
   - source identity and checkpoint metadata;
   - per-item state and reason code;
   - corpus manifest path;
   - guardrails showing zero destination writes, zero Product Brain writes, and zero Tolaria writes.
4. Tests with synthetic Slack fixtures covering links, empty messages, secret-looking content, missing permalinks, reverse ordering, and output write failure.
5. Private runtime validation using Randy's Slack self-DM/channel connector data, written only to `/private/tmp`.

## Input Contract

```text
mindline slack corpus-intake /private/tmp/randy-self-dm.json --out /private/tmp/mindline-wp31-slack-intake
mindline documents corpus-pressure /private/tmp/mindline-wp31-slack-intake/corpus-pressure-manifest.json --out /private/tmp/mindline-wp31-slack-pressure
```

`slack-export.json` uses the existing adapter payload shape:

```json
{
  "source": {
    "workspace": "example",
    "channel_id": "D123",
    "channel_name": "self-dm",
    "adapter_id": "slack"
  },
  "messages": []
}
```

## Privacy Rules

Committed fixtures must be synthetic. Real Slack connector data may be used only as local runtime validation. Do not commit raw private Slack message text, private names, private permalinks, or private derived excerpts.

Top-level intake summaries and reports must not include raw private message text, private file URLs, secret-looking strings, prompts, model completions, absolute source paths, destination payloads, or Chain-private labels. Raw Slack message text may appear only inside the generated local source Markdown files that are explicitly the private source corpus for local processing.

## KRs

1. **Slack source accounting:** 100% of input messages are accounted for as processed, skipped, or blocked.
2. **Stable source identity:** processed source IDs are deterministic functions of Slack workspace, channel, and message timestamp, not run time or output path.
3. **Old-to-new processing:** output manifest order follows Slack old-to-new order.
4. **Corpus-pressure compatibility:** the generated manifest runs through `documents corpus-pressure` without custom temp logic.
5. **Privacy guardrails:** secret-looking content and private Slack file URLs are redacted from committed outputs and top-level reports.
6. **No destination writes:** intake and pressure summaries show zero destination/Product Brain/Tolaria writes.
7. **Runtime proof:** a private run over Randy's Slack self-DM/channel data produces intake artifacts and a corpus-pressure result under `/private/tmp`.
8. **Audit-ready Chain:** WP-31 spec, plan, and sign-off decision are captured in Product Brain.

## Anti-Goals

- No Slack API client in this PR.
- No Slack connector dependency in committed code.
- No Tolaria writes.
- No Product Brain destination writes.
- No Product Brain proposal apply path.
- No UI.
- No auth/login or hosted DB.
- No model/provider tuning.
- No hosted telemetry export by default.
- No no-human autonomy claim.
- No answer-key generation from the evaluated run.
- No optimizing specifically for Randy's private Slack data.

## Verification

- Unit tests for intake accounting, source IDs, manifest compatibility, old-to-new order, privacy redaction, missing permalinks, empty-message skips, secret-message blocks, no-eligible-source behavior, nested symlink containment, and write failures.
- CLI regression for `slack corpus-intake`.
- Focused integration run: `slack corpus-intake` fixture output into `documents corpus-pressure`.
- Private runtime smoke over Randy's Slack self-DM/channel data into `/private/tmp`.
- `go test ./...`
- `git diff --check`
- Product Brain capture/link for WP-31 evidence.
