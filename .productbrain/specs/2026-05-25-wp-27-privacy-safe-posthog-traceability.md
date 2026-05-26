# WP-27 Privacy-Safe Trace/Eval Feedback Loop

## Diagnosis

Mindline is trying to move from human semantic review toward autonomous semantic judgment, but the system cannot improve safely while LLM decisions are opaque. The current loop shows candidate outputs, agent proposals, and human decisions, yet it does not provide one canonical trace/eval spine that connects provider/model, validation result, evaluator verdict, human override, and failure taxonomy.

Without traceability, we optimize by inspecting isolated UI examples and temp artifacts. That is too slow and it encourages more review UI instead of clearer autonomous feedback loops. PostHog should be used as the first hosted observability sink, but the local trace/eval artifact remains canonical and raw private content is non-exportable by default.

## Governing Constraints

- Mindline remains a headless, provider-agnostic semantic ingestion engine. PostHog is an exporter, not the canonical trace model.
- Privacy by design and traceability by design are paired constraints: autonomy may improve only through observable traces, and private source content must be minimized before leaving the local machine.
- EU hosting lowers residency risk but does not permit exporting raw private source material by default.
- This WP proves privacy-safe feedback-loop instrumentation, not autonomy quality, no-human readiness, or destination-write readiness.

## In Scope

1. Capture Chain governance:
   - Principle: privacy-by-design traceability.
   - Business rule: hosted observability exports metadata only by default.
   - Work package: WP-27 as the implementation authority.
2. Add local env scaffolding for PostHog:
   - `MINDLINE_TELEMETRY_ENABLED=false`
   - `MINDLINE_LLM_TRACE_MODE=metadata`
   - `MINDLINE_TELEMETRY_SALT=`
   - `POSTHOG_PROJECT_API_KEY=`
   - `POSTHOG_HOST=https://eu.i.posthog.com`
3. Add a provider-neutral observability config and PostHog exporter boundary.
4. Add a CLI smoke command that sends a harmless metadata-only test event when telemetry is explicitly enabled and configured.
5. Add local trace/eval artifacts for real runs:
   - `documents semantics` writes `trace/trace-summary.json` after semantic runs.
   - `documents judge` writes `trace/trace-summary.json` after judgment runs.
   - trace summaries contain counts, pass/fail labels, failure taxonomy, evidence readiness, provider/model, and recommendations; they never contain prompts, completions, source excerpts, candidate summaries, or private paths.
6. Export real run metadata to PostHog when telemetry is enabled:
   - semantic runs emit metadata-only `$ai_generation` events.
   - judgment runs emit metadata-only `$ai_generation` and `$ai_feedback` events.
   - export failure does not corrupt semantic artifacts, but the CLI must fail visibly so trace export cannot silently lie.
7. Add tests proving:
   - telemetry is disabled by default;
   - enabled telemetry requires host, project key, salt, and metadata trace mode;
   - `raw` / `full` trace modes are rejected;
   - exported events are property-allowlisted;
   - unsafe property names and values such as prompts, completions, source excerpts, private paths, and secrets are rejected;
   - real semantic and judgment runs write local trace summaries;
   - enabled telemetry exports safe metadata for real semantic and judgment runs.

## Out of Scope

- Raw prompt/completion capture.
- Source excerpt, candidate summary, or private file path export.
- PostHog dashboards, feature flags, session replay, or product analytics UI.
- Prompt tuning, automatic self-modification, calibration scoring changes, or no-human claims.
- Destination writes to Tolaria or Product Brain.

## Acceptance Criteria

1. `.env.local.example` documents the PostHog telemetry variables and keeps telemetry disabled by default.
2. Local `.env.local` can be filled by Randy without committing secrets.
3. `mindline observability posthog-test` exits successfully with a disabled envelope when telemetry is disabled.
4. With telemetry enabled and valid config, the smoke command posts exactly one metadata-only event to PostHog's capture endpoint.
5. `documents semantics` writes a local trace/eval summary and exports safe metadata when telemetry is enabled.
6. `documents judge` writes a local trace/eval summary and exports safe generation/feedback metadata when telemetry is enabled.
7. Trace reports explain the next improvement target, such as evidence-readiness failures, no candidates, model errors, human-review burden, or unclear/reject patterns.
8. Tests prove unsafe trace modes and unsafe event properties fail closed before network export.
9. The implementation has no PostHog dependency in semantic extraction or judgment correctness paths.
10. `go test ./...`, `git diff --check`, and Product Brain audit pass or have only explicitly reconciled warn-only findings.

## Reviewer Sign-Off

- Scope reviewer: SIGN-OFF, with PostHog treated as first exporter target, not canonical trace schema.
- Risk/Safety reviewer: BLOCKER resolved by making metadata-only export mandatory and rejecting raw/full modes.
- Delivery reviewer: Phase 0 was insufficient alone; full WP requires real semantic/judgment trace artifacts and PostHog metadata export.
