# WP-34 Slack Link Enrichment Feedback Loop Spec

**Status:** signed-spec candidate V3
**Date:** 2026-05-28
**Product:** Mindline
**Owning workstream:** WS-3 Read-only ingestion pressure
**Strategy:** STR-3 Autonomy-readiness before destination writes
**Governing rules:** PRI-1, BR-1, STD-5, STD-7, STD-11, STD-12, STD-17, STD-20, TEN-12, WP-27
**Input evidence:** WP-31, WP-32, WP-33, INS-18, INS-19, INS-20, DEC-239, ctx7 PostHog docs for `$ai_evaluation`
**Stop mode:** Full delivery

## 1. Diagnosis

WP-31 proved Slack self-DM material can become a local corpus. WP-32 made source meaning preview visible. WP-33 proved local/manual source enrichment can enrich Markdown before corpus-pressure without fetching the network.

The real runtime proof after PR #26 exposed the next product blocker: real Slack link captures are mechanically processed, but every item remains `link_only_source`, `reference_only`, and `missing_link_enrichment`. That is the correct failure mode, but it is not yet useful enough for Randy. The system can say "this link needs enrichment"; it cannot yet produce an operable request pack, accept locally supplied artifacts, rerun the same pipeline, and prove whether source meaning improved.

## 2. Spec/Plan Loop Ledger

### Pass 1

Initial scope: add a next slice for Slack link-capture enrichment after WP-33.

Reviewer pressure:

- Chain Steward: require deterministic request packs and before/after proof, not fixture-only proof.
- Domain/User Job: require human/agent-operable requests, not a raw URL dump.
- Risk/Safety: require local-only, metadata-only, no live fetch, and anti-overfit replay.
- Systems Architect: reuse WP-33 and meaning-preview; do not create a second enrichment engine.

### Pass 2

Tightened scope: WP-34 is an orchestration and measurement loop over existing primitives:

1. generate a local link-artifact request pack from a corpus-pressure manifest or Slack corpus-intake output;
2. consume the existing `local-source-enrichment-artifacts/v0.1` manifest;
3. rerun `enrich-sources -> corpus-pressure -> meaning-preview`;
4. compare baseline and enriched meaning-preview outputs with the same corpus/config fingerprints;
5. report missingness/routing movement, guardrails, and artifact coverage;
6. project the loop result into a metadata-only PostHog `$ai_evaluation` event so the trace/eval system can inspect the KR result after every run.

No live web fetching, browser automation, Slack API client, destination writes, auto-accept, or no-human claim is allowed.

## 3. Outcome

Given a real Slack corpus intake output or a corpus-pressure manifest containing link-only sources, Mindline can answer:

> Which links need evidence, what local artifacts should be supplied, and did those artifacts measurably make the same source corpus more reviewable?

The output is a local feedback-loop bundle with:

- request-pack JSON and Markdown;
- baseline pressure and meaning-preview outputs;
- local enrichment outputs;
- enriched pressure and meaning-preview outputs;
- comparison JSON and Markdown;
- trace/eval counters proving before/after movement and zero prohibited side effects;
- a local PostHog eval projection artifact, plus a live send proof when telemetry is configured.

## 4. Product Model Fit

This is not a Slack-only patch. The canonical product pattern is "source evidence request and enrichment feedback loop." Slack is the first real pressure surface because Randy captures links there, but the command consumes corpus manifests and therefore applies to future source adapters too.

The slice extends existing patterns:

- WP-31 Slack corpus intake;
- WP-33 local source enrichment;
- WP-29 corpus-pressure;
- WP-32 source meaning preview;
- WP-27 trace/eval guardrails and BR-1 metadata-only hosted observability.

It does not create a new destination path.

## 5. Scope

In scope:

1. Add a read-only CLI command:

```bash
mindline documents link-enrichment-loop <corpus-pressure-manifest-or-intake-dir> --artifacts <local-source-enrichment-artifacts.json> --out <dir> [--classifier deterministic|llm --llm-provider openai --llm-model <model>]
```

2. Accept either:
   - a direct `corpus-pressure-manifest.json`; or
   - a Slack corpus-intake output directory containing `corpus-pressure-manifest.json`.
3. Generate a deterministic request pack:
   - `link-enrichment/requests/link-artifact-requests.json`;
   - `link-enrichment/requests/link-artifact-requests.md`.
4. Reuse WP-33 URL extraction, normalization, policy, unsafe-token scanning, and artifact matching.
5. Use the existing `local-source-enrichment-artifacts/v0.1` artifact manifest. Do not invent a second artifact format unless implementation proves a hard gap.
6. Run baseline `corpus-pressure` and `meaning-preview`.
7. Run `enrich-sources`, then enriched `corpus-pressure` and enriched `meaning-preview`.
8. Produce comparison artifacts:
   - `link-enrichment/comparison/comparison-summary.json`;
   - `link-enrichment/comparison/comparison-report.md`.
9. Produce a PostHog eval projection artifact:
   - `link-enrichment/posthog/eval-projection.json`.
10. When `MINDLINE_TELEMETRY_ENABLED=true`, export one metadata-only PostHog `$ai_evaluation` event named `mindline.link_enrichment.covered_missingness_reduction`.
11. Report deltas for:
   - `missing_link_enrichment`;
   - `link_only_source`;
   - `reference_only`;
   - `needs_enrichment`;
   - `tolaria_candidate`;
   - `product_brain_candidate`;
   - enriched URL coverage;
   - artifact matched/rejected/stale counts;
   - evidence coverage;
   - review burden;
   - guardrails.
10. Include real private Slack runtime proof under `/private/tmp` only.

Out of scope:

- live web fetching;
- browser automation;
- Slack API calls in committed code;
- auth/login;
- database or persistent queue;
- Tolaria writes;
- Product Brain runtime writes;
- destination apply payloads;
- answer-key generation from the evaluated run;
- DEC-64/no-human/autonomy readiness claims;
- optimizing for only `/temp` or Randy's private Slack sample.

## 6. Contracts

### Request Manifest

`local-link-artifact-requests/v0.1`

Each request has:

- stable `request_id`;
- `source_id`;
- `source_kind`;
- safe relative source path or source label;
- raw URL only when safe;
- normalized URL;
- URL kind;
- request state: `requestable`, `already_artifacted`, `unsupported`, `blocked_private_or_secret`, `blocked_by_policy`;
- reason codes;
- requested artifact fields: `title`, `source_name`, `description`, `excerpt`, `captured_at`;
- `safe_for_top_level_report` boolean.

Request packs must not include raw Slack message text, raw private Slack permalinks, absolute private paths, prompts, completions, or source excerpts in committed/top-level metadata artifacts.

### Comparison Summary

`link-enrichment-comparison/v0.1`

The comparison must include:

- input manifest path;
- artifact manifest path;
- corpus ID;
- baseline and enriched corpus/config fingerprints;
- source counts;
- URL request accounting;
- artifact consumption;
- before/after missingness counts;
- before/after routing counts;
- before/after evidence/review counters;
- guardrails;
- verdict: `improved`, `unchanged`, or `blocked`;
- non-generalization note for private runtime proof.

Before/after comparison is valid only when the source corpus fingerprint and command config fingerprint are equal or explicitly comparable under the same classifier settings. If not, verdict must be `blocked`.

### PostHog Evaluation Projection

`mindline-link-enrichment-eval-projection/v0.1`

The projection must:

- write locally on every `link-enrichment-loop` run, even when telemetry is disabled;
- export to PostHog only when telemetry is explicitly enabled and configured;
- emit event `$ai_evaluation`, not a generic analytics event;
- set `$ai_evaluation_name` to `mindline.link_enrichment.covered_missingness_reduction`;
- set `$ai_evaluation_result` from the KR gate, not from command success;
- set `$ai_evaluation_reasoning` to stable reason codes only;
- include only allowlisted metadata: counts, ratios, guardrails, verdict, salted run/trace identifiers, and boolean KR fields;
- include `non_generalizable_runtime=true` for private `/tmp` runtime proofs so private sample wins cannot be mistaken for broad corpus accuracy;
- exclude raw URLs, source text, prompts, completions, source excerpts, file paths, Slack permalinks, private IDs, and notes;
- fail closed when `MINDLINE_LLM_TRACE_MODE` is not `metadata`;
- mark the eval failed when missingness/routing KRs, comparability, artifact coverage, or guardrails fail.

Per PostHog's AI eval docs surfaced through ctx7, `$ai_evaluation` events carry properties such as `$ai_evaluation_name`, `$ai_evaluation_result`, `$ai_evaluation_reasoning`, and `$ai_trace_id`; WP-34 uses that event shape with Mindline's metadata-only allowlist.

## 7. Aggressive KRs

1. **Request coverage:** 100% of eligible URLs in synthetic and private runtime Slack samples are accounted for as requestable, already artifacted, unsupported, blocked private/secret, or policy blocked.
2. **Artifact consumption:** 100% of supplied artifacts are consumed or rejected with explicit reason codes; stale artifacts are counted.
3. **Missingness movement:** For artifact-covered real Slack links, `missing_link_enrichment` decreases by at least 98% in the enriched meaning-preview compared with baseline.
4. **Routing movement:** For artifact-covered real Slack links, `needs_enrichment` decreases by at least 98% without increasing blocked/private/unsafe counters.
5. **Reviewability proof:** The enriched preview/report must include enough title/description/excerpt evidence for Randy to judge at least three real Slack link captures without reopening Slack.
6. **Replay stability:** An unchanged synthetic replay produces the same aggregate request and comparison counts.
7. **No-live-fetch proof:** request generation and loop execution show zero network/browser/Slack API/destination writes and no hidden fetch mode; the only allowed network call is the explicit metadata-only PostHog eval export when telemetry is enabled.
8. **Leak proof:** generated-output leak scan finds zero private Slack permalinks, raw Slack file URLs, secret-looking strings, prompt/completion text, absolute private paths, or private runtime source excerpts in committed/top-level artifacts.
9. **Trace/eval proof:** comparison artifacts and PostHog eval projection expose before/after missingness, routing, enrichment coverage, evidence coverage, review burden, and guardrails; not just candidate volume.
10. **Hosted eval proof:** with telemetry enabled, a live PostHog send records one `$ai_evaluation` projection with status `sent`, metadata-only fields, `$ai_evaluation_result=true`, and no unsafe payload.
11. **Self-optimization proof:** at least one finding from trace/eval inspection must cause a product/generalizable fix or KR upgrade before PR readiness. If no finding is found, the PR must show two independent real/synthetic replays and an explicit no-change rationale.

## 8. Risks

1. **False confidence from local artifacts.** Mitigation: label retrieval as local/manual artifact coverage, not source completeness or fetch proof.
2. **Privacy leakage in request packs.** Mitigation: top-level request packs are metadata-only; private runtime artifacts stay under `/private/tmp`.
3. **Duplicate URL coverage hiding per-source missingness.** Mitigation: report both per-source URL accounting and deduped URL accounting.
4. **Bespoke private sample optimization.** Mitigation: committed fixtures are synthetic, replay-stable, and include duplicate, missing, unsupported, blocked, unsafe, and stale artifact cases.
5. **Pipeline fork.** Mitigation: the loop calls existing source enrichment, corpus-pressure, and meaning-preview builders.

## 9. Done Proof

WP-34 is done only when:

1. `documents link-enrichment-loop` exists and accepts a manifest or Slack intake output directory.
2. Request-pack JSON and Markdown are generated deterministically.
3. Existing WP-33 artifact manifests are consumed without a new artifact format.
4. Baseline and enriched pressure/meaning outputs are produced through existing builders.
5. Comparison JSON/Markdown proves before/after movement and blocks invalid comparisons.
6. Tests cover synthetic Slack link-only fixtures, partial artifact coverage, duplicate URL across sources, unsupported/private/secret URLs, stale artifacts, unsafe artifact payloads, deterministic replay, and containment.
7. Runtime proof over real Slack sample under `/private/tmp` reduces `missing_link_enrichment` and `needs_enrichment` for artifact-covered links by at least 98%.
8. Runtime proof writes `link-enrichment/posthog/eval-projection.json` and live PostHog send proof has `status=sent`, event `$ai_evaluation`, eval name `mindline.link_enrichment.covered_missingness_reduction`, and eval result `true`.
9. `go test ./internal/documents ./internal/cli ./internal/observability` passes.
10. `go test ./...` passes.
11. `git diff --check` passes.
12. Generated-output leak scan passes.
13. Product Brain audit for WP-34 handoff is pass or warn-only with explicit reconciliation.
