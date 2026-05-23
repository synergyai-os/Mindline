# MINDLINE-WP17-PLAN-V1 — Provider-Agnostic LLM Semantic Classifier

## Status

Plan draft for WP-17. Delivery starts only after LOOP sign-off and Chain update.

## Delivery Sequence

### 1. Preflight

- Confirm branch is based on current `main`.
- Confirm `.env.local` is ignored and `.env.local.example` contains only placeholders.
- Confirm deterministic `documents semantics` still runs without any provider key.
- Freeze the evaluation corpus, answer keys, and metric definitions before classifier changes.
- Choose one named failure class from WP-15/WP-16 outputs as the first improvement target.

### 2. Provider Port

- Add a provider-neutral LLM classifier interface.
- Keep provider-specific request/response types out of core semantic logic.
- Define provider-neutral request fields: task, candidate kinds, source excerpts, structure nodes, evidence requirements, max output items, and schema version.
- Define provider-neutral response fields: candidates, evidence references, confidence, blockers, raw-provider metadata redacted or omitted from persisted artifacts.

### 3. OpenAI Adapter

- Implement OpenAI as the first provider adapter.
- Read provider config from explicit CLI flags and/or local environment.
- Require `MINDLINE_LLM_PROVIDER=openai`, `OPENAI_API_KEY`, and model config for LLM mode.
- Fail before source processing if LLM mode is requested and config is missing.
- Keep deterministic mode independent from OpenAI.

### 4. Provider Registry

- Add registry entries for provider IDs: `openai`, `gemini`, `claude`, `openrouter`.
- Mark only OpenAI implemented in WP-17.
- Ensure unsupported providers fail with a clear message and do not read/send source content.

### 5. Prompt And Schema Validation

- Build prompt/task construction in Mindline core, not in the OpenAI adapter.
- Require source-grounded evidence IDs/ranges in every candidate.
- Reject malformed JSON, unsupported candidate kinds, missing evidence, invented evidence IDs, private markers, and destination-specific fields.
- Persist only normalized Mindline semantic candidate artifacts, not raw provider payloads.

### 6. Evaluation

- Run baseline deterministic classifier on the frozen corpus.
- Run LLM classifier on the same corpus.
- Run WP-15 acceptance on both outputs.
- Run WP-16 calibration on both outputs.
- Compare before/after using the same answer keys and metrics.
- Report only measured quality on this harness.

### 7. Tests

- Unit-test provider config resolution.
- Unit-test missing-key and unsupported-provider fail-closed behavior.
- Unit-test provider adapter translation with fake client fixtures.
- Unit-test schema/evidence validation of LLM output.
- Regression-test the selected failure class.
- Verify deterministic mode without keys.
- Verify `.env.local` is ignored and no key-like values are committed.
- Run full Go test suite.

### 8. Chain And PR Closeout

- Update `WP-17` with signed scope, acceptance, exclusions, and validation status.
- Link/cite `ARCH-1`, `DEC-74`, and `STD-17`.
- Capture delivery decision only after tests and before/after reports pass.
- PR summary must state provider-agnostic architecture, OpenAI-first implementation, and measured-quality limits.

## Stop Conditions

Stop and capture a blocker if:

- provider-agnostic port cannot be kept separate from provider SDK types
- LLM output cannot be evidence-validated
- OpenAI mode requires committing private source/output artifacts
- deterministic mode breaks without API keys
- metrics are not comparable before/after
- the implementation starts pulling in destination mapping or Product Brain apply behavior

