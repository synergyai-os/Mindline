# WP-34 Slack Link Enrichment Feedback Loop Plan

**Spec:** `.productbrain/specs/2026-05-28-wp-34-slack-link-enrichment-feedback-loop.md`
**Stop mode:** Full Delivery
**Plan status:** signed-plan candidate V2
**Implementation rule:** orchestrate existing primitives; do not create a live fetcher or second enrichment engine.

## 1. Chain Authority

1. Reconcile WP-33 merged state:
   - update WP-33 to shipped/validated evidence state;
   - relate WP-33 to an applicable KR or explicitly reconcile the warn-only missing-KR audit.
2. Capture WP-34 as a work package.
3. Link WP-34 to:
   - STR-3;
   - WS-3;
   - PRI-1;
   - BR-1;
   - STD-5;
   - STD-7;
   - STD-12;
   - STD-20;
   - WP-31;
   - WP-32;
   - WP-33;
   - INS-18;
   - INS-19;
   - KEY-6 and KEY-7 if audit requires KR validation.
4. Run `pb audit WP-34 --phase handoff --verbose` before delivery authority.

## 2. Code Shape

Add an orchestration layer in `internal/documents`:

- `link_enrichment_loop.go`
- `link_enrichment_loop_writer.go`
- `link_enrichment_loop_test.go`

Primary API:

```go
type LinkEnrichmentLoopOptions struct {
    SemanticOptions SemanticOptions
    CommandConfigFingerprint string
}

func BuildLinkEnrichmentLoop(inputPath, artifactsPath, outDir string, options LinkEnrichmentLoopOptions) (LinkEnrichmentLoopSummary, error)
```

Builder responsibilities:

1. Resolve input path:
   - direct manifest file; or
   - directory containing `corpus-pressure-manifest.json`.
2. Build request pack from the resolved manifest.
3. Run baseline `BuildCorpusPressure`.
4. Run baseline `BuildSourceMeaningPreview`.
5. Run `BuildSourceEnrichment`.
6. Run enriched `BuildCorpusPressure`.
7. Run enriched `BuildSourceMeaningPreview`.
8. Build comparison summary/report.
9. Write all artifacts under `--out`.

Directory layout:

```text
link-enrichment/
  requests/
    link-artifact-requests.json
    link-artifact-requests.md
  comparison/
    comparison-summary.json
    comparison-report.md
baseline-pressure/
baseline-meaning/
enrichment/
enriched-pressure/
enriched-meaning/
```

## 3. URL Request Pack

Reuse WP-33 URL functions by making narrowly scoped helpers package-visible as needed:

- URL extraction;
- URL normalization;
- kind detection;
- policy block detection;
- unsafe source-token scanning;
- local artifact matching.

Do not duplicate URL policy.

Request pack fields:

- `request_id`;
- `source_id`;
- `source_kind`;
- `source_label`;
- `normalized_url`;
- `raw_url` when safe;
- `kind`;
- `state`;
- `reason_codes`;
- `requested_fields`;
- `safe_for_top_level_report`.

Coverage counters:

- total URL mentions;
- unique normalized URLs;
- requestable;
- already artifacted;
- unsupported;
- blocked private/secret;
- blocked by policy;
- missing artifacts;
- supplied artifacts;
- matched artifacts;
- stale artifacts.

## 4. Comparison

Comparison must read generated baseline/enriched summaries, not infer from command success.

Compare:

- source count;
- processed/excluded/blocked count;
- corpus fingerprint;
- command config fingerprint;
- guardrails;
- missingness counts;
- routing hint counts;
- candidate kind counts;
- evidence coverage ratio;
- review burden ratio;
- enriched URL coverage;
- artifact rejection/stale counts.

Verdict:

- `improved`: comparable fingerprints/config and missingness/routing KRs move in the right direction.
- `unchanged`: comparable, safe, but no meaningful missingness movement.
- `blocked`: invalid comparison, unsafe output, missing artifacts, or guardrail violation.

## 5. CLI Shape

Add:

```bash
mindline documents link-enrichment-loop <corpus-pressure-manifest-or-intake-dir> --artifacts <local-source-enrichment-artifacts.json> --out <dir> [--classifier deterministic|llm --llm-provider openai --llm-model <model>]
```

CLI requirements:

- use existing protected-root output validation;
- print summary JSON to stdout;
- return usage on missing args;
- return artifact-write on containment/write failures;
- return process errors for invalid comparisons or missing manifest.

## 6. Tests

Focused tests:

1. link-only synthetic Slack-style corpus creates request pack and baseline missingness.
2. local artifacts reduce enriched missingness and `needs_enrichment`.
3. partial artifact coverage leaves uncovered links visible.
4. duplicate URL across sources reports per-source and deduped coverage.
5. unsupported/private/secret URLs are blocked or unsupported with reason codes.
6. unsafe artifact payload is rejected before Markdown/JSON/report write.
7. stale artifact URL is counted.
8. invalid before/after comparability blocks verdict.
9. deterministic replay has stable request/comparison counters.
10. output containment rejects protected roots/symlink escapes.
11. CLI success and usage errors.

## 7. Runtime Proof

Synthetic proof:

```bash
go run ./cmd/mindline documents link-enrichment-loop testdata/documents/link-enrichment-loop/corpus-pressure-manifest.json --artifacts testdata/documents/link-enrichment-loop/local-artifacts.json --out /private/tmp/mindline-wp34-synthetic
```

Real Slack proof:

1. Use Slack connector at runtime to create `/private/tmp/mindline-wp34-real-slack/slack-dm-sample.json`.
2. Run `slack corpus-intake` into `/private/tmp/mindline-wp34-real-slack/intake`.
3. Create local artifact manifest for at least five real safe links under `/private/tmp`.
4. Run `documents link-enrichment-loop` with LLM classifier if OpenAI is available.
5. Verify at least 80% reduction for artifact-covered links in `missing_link_enrichment` and `needs_enrichment`.
6. Inspect at least three enriched previews for human-review usefulness.

## 8. Verification Commands

```bash
go test ./internal/documents ./internal/cli
go test ./...
git diff --check
pb audit WP-34 --phase handoff --verbose
```

Generated-output leak scan:

```bash
rg "xoxb-|sk_live_|sk-proj-|files-pri|workspace\\.slack\\.com/archives|PRIVATE_CONTENT|/Users/randyhereman|api.openai.com/v1/responses" /private/tmp/mindline-wp34-synthetic /private/tmp/mindline-wp34-real-slack
```

The leak scan is expected to return no matches in generated top-level reports and committed fixtures. Private runtime source Markdown may contain user-approved source URLs but must not be committed.

## 9. Review Gates

Before push:

- Chain Steward: WP-34 captured, linked, audited, and WP-33 state reconciled.
- Systems Architect: orchestration reuses WP-33/WP-29/WP-32 and does not fork semantics.
- Domain/User Job: real Slack output is inspectable and materially more useful than link-only preview.
- Risk/Safety: no fetch, no leakage, no hidden hosted telemetry, no destination writes.
- Delivery Quality: tests, runtime proof, replay stability, and maintainability sign off.

## 10. Non-Goals

- Do not fetch live URLs.
- Do not add browser automation.
- Do not add Slack API client code.
- Do not write Tolaria or Product Brain.
- Do not create permanent UI or DB.
- Do not claim DEC-64 readiness.
- Do not tune to only `/temp` or the private Slack smoke sample.
