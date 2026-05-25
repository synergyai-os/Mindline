# WP-27 Privacy-Safe Trace/Eval Feedback Loop Plan

## Build Sequence

1. Chain governance
   - Capture the privacy-by-design traceability principle.
   - Capture the metadata-only hosted observability business rule.
   - Capture WP-27 and link it to the autonomy readiness/evaluation foundation context.

2. Env scaffolding
   - Extend `.env.local.example` with disabled-by-default PostHog variables.
   - Add the same placeholders to local `.env.local` for Randy to fill, without committing it.

3. Observability core
   - Add `internal/observability`.
   - Keep canonical trace configuration and safe event validation provider-neutral.
   - Require `MINDLINE_LLM_TRACE_MODE=metadata` for Phase 0.
   - Validate an explicit allowlist of event properties.

4. PostHog exporter
   - Add a small HTTP exporter that posts to `<POSTHOG_HOST>/capture/`.
   - Export only the safe event payload.
   - Fail closed on missing config, unsafe mode, unsafe property names, unsafe values, or non-2xx response.

5. CLI smoke command
   - Add `mindline observability posthog-test`.
   - When disabled, return a JSON envelope showing no network export happened.
   - When enabled, send one harmless event with provider-neutral metadata and return a JSON envelope without secrets.

6. Real run trace/eval artifacts
   - Add a trace summary type in `internal/observability`.
   - Add builders for semantic summaries and judgment summaries.
   - Write `trace/trace-summary.json` under the command output directory.
   - Include counts, failure labels, readiness labels, provider/model metadata, and recommended next improvement.

7. Real run PostHog export
   - After `documents semantics`, emit a metadata-only `$ai_generation` event when telemetry is enabled.
   - After `documents judge`, emit metadata-only `$ai_generation` and `$ai_feedback` events when telemetry is enabled.
   - Keep export outside semantic correctness logic; fail visibly if enabled export cannot complete.

8. Tests and review
   - Unit-test config defaults and validation.
   - Unit-test property allowlist / unsafe value rejection.
   - CLI-test disabled and enabled smoke paths with an `httptest` PostHog endpoint.
   - Unit-test trace summary builders.
   - CLI-test semantics and judgment trace summary artifacts.
   - CLI-test enabled export paths with an `httptest` PostHog endpoint.
   - Run `go test ./...`, `git diff --check`, temp-file smoke runs, and `pb audit WP-27`.

## Validation Commands

```sh
go test ./...
git diff --check
pb audit WP-27
```

## Phase 0 Manual Test

After Randy fills local `.env.local`:

```sh
go run ./cmd/mindline observability posthog-test
```

Expected:

- disabled config: `{"state":"telemetry_disabled",...}`
- enabled config: `{"state":"posthog_test_sent",...}`

## Full WP Manual Test

Run at least one real semantic or judgment command with telemetry enabled and confirm:

- `trace/trace-summary.json` exists locally.
- PostHog receives metadata-only `$ai_generation` / `$ai_feedback` events.
- No raw source excerpt, prompt, completion, candidate summary, local path, or secret is exported.
