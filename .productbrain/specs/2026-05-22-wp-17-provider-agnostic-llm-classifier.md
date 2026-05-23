# MINDLINE-WP17-SPEC-V1 — Provider-Agnostic LLM Semantic Classifier

## Status

Spec draft for WP-17. Delivery is not authorized until LOOP sign-off and Chain update complete.

## Chain Authority

- `WP-17` — Measured classifier improvement slice.
- `DEC-64` — Mindline semantic automation target: no human in steady state, `>=98%` measured held-out accuracy.
- `WP-15` — Semantic candidate acceptance loop.
- `WP-16` — Autonomy calibration from acceptance batches.
- `ARCH-1` — Model providers are interchangeable behind a Mindline LLM provider port.
- `DEC-74` — WP-17 starts with OpenAI but requires model-provider agnostic LLM architecture.
- `STD-17` — LLM-backed semantic classifiers must be provider-agnostic and measured before trust.
- `DOMAIN-1` — Mindline is the ingestion engine; Product Brain is a future destination/authority consumer, not the product.

## Plain-English Goal

WP-15 and WP-16 built the scoreboard. WP-17 improves the semantic engine.

The first improvement may use OpenAI because Randy can provide an OpenAI key now, but Mindline must not become OpenAI-shaped. Gemini, Claude, OpenRouter, and future providers must be able to plug in behind the same provider-neutral interface.

## Problem

The current deterministic semantic classifier can create inspectable artifacts, but real temp evidence shows weak transcript candidates and overly coarse process-doc candidates. A human can now review the output, but the product target is not a permanent review queue. Mindline needs a measured way to try stronger semantic classification while preserving local/open architecture, destination neutrality, evidence grounding, and the `>=98%` held-out trust gate.

## In Scope

1. Add a provider-neutral LLM classifier architecture for semantic candidate generation.
2. Implement one provider adapter first: OpenAI.
3. Configure provider/model/key through local environment and CLI options, not hardcoded constants.
4. Keep deterministic classification available without any API key.
5. Require LLM output to validate against Mindline semantic candidate/evidence schemas.
6. Require cited source evidence nodes/ranges; unsupported claims fail closed.
7. Run WP-15 acceptance and WP-16 calibration against the same frozen corpus, answer keys, and metrics before and after the classifier change.
8. Report measured quality on the harness only; no autonomy claim unless held-out accuracy reaches `>=98%`.
9. Keep real private `temp/` source and generated private outputs uncommitted.
10. Provide `.env.local.example` for local setup while ignoring real `.env.local`.

## Out of Scope

- No Product Brain writes.
- No Tolaria writes.
- No destination policy mapping.
- No Product Brain proposal or apply-time client changes.
- No provider lock-in.
- No committed private source content or private generated outputs.
- No claim of calibrated semantic quality from self-derived or mechanical labels.
- No requirement that users have an OpenAI key for deterministic/local classifier mode.

## Provider-Agnostic Contract

Core Mindline code owns these concepts:

- semantic candidate schema
- evidence node/range schema
- validation and rejection rules
- prompt/task contract
- token/input budget policy
- acceptance/calibration metrics
- privacy and source-containment checks

Provider adapters own only these concepts:

- provider name
- credential lookup
- endpoint/client call
- model name
- request/response translation
- provider-specific retry/error mapping

The core classifier must depend on a provider-neutral interface, not provider SDK types.

Initial providers to design for:

- `openai` — first implemented provider for WP-17.
- `gemini` — future adapter.
- `claude` — future adapter.
- `openrouter` — future adapter.

## Runtime Configuration

Local key file:

```sh
.env.local
```

Committed template:

```sh
.env.local.example
```

Expected first local values:

```sh
MINDLINE_LLM_PROVIDER=openai
OPENAI_API_KEY=...
OPENAI_MODEL=...
```

Rules:

- `.env.local` must be ignored by git.
- Missing key must not break deterministic classifier mode.
- LLM mode with missing key must fail clearly before reading or sending source content.
- Provider choice must be explicit through config/CLI, not inferred from whichever key exists.

## CLI Shape

Target shape:

```sh
mindline documents semantics <input> --classifier deterministic --out <dir>
mindline documents semantics <input> --classifier llm --llm-provider openai --out <dir>
```

Equivalent environment defaults are allowed, but CLI flags must be able to override them.

## Acceptance

WP-17 is complete when:

1. A provider-neutral LLM classifier port exists.
2. OpenAI is implemented as the first adapter behind that port.
3. Gemini, Claude, and OpenRouter are represented as provider IDs/config slots without being required for delivery.
4. `.env.local.example` exists and `.env.local` is ignored.
5. Deterministic classifier mode still works without API keys.
6. LLM mode fails closed when provider/key/model config is missing.
7. LLM classifier output is schema-validated and evidence-grounded.
8. The same frozen corpus, answer keys, and metric definitions are used for before/after reports.
9. One named WP-15/WP-16 failure class is targeted for improvement before implementation starts.
10. Before/after WP-15/WP-16 reports show the measured effect without overclaiming trust.
11. Full tests pass.
12. No private temp content, keys, provider raw responses, or destination-specific fields are committed.

## Guardrails

- The LLM is not authority. The acceptance/calibration harness is authority.
- The provider is not architecture. The provider adapter is replaceable infrastructure.
- No-human eligibility is blocked unless held-out accuracy is `>=98%`.
- Destination neutrality remains intact: semantic outputs may not contain PB collection names, proposal intents, Tolaria paths, destination locators, or apply/client transport fields.

