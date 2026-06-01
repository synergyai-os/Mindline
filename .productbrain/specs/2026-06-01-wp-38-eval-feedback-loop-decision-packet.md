# WP-38 — Eval Feedback Loop Decision Packet

## Outcome

Mindline can turn existing local eval artifacts into one privacy-safe decision packet that says what to improve next, why that target is product-general, whether the current run improved versus a comparable baseline, and which claims remain blocked.

This closes the local feedback-loop gap left after WP-35, WP-36, and WP-37:

- WP-35 reads eval evidence and blocks unsupported claims.
- WP-36 gives named proof gates.
- WP-37 gives a readable mixed-source value packet.
- WP-38 decides the next generalized improvement target from those artifacts without executing the improvement.

## Chain Authority

- `WP-27` — Privacy-safe trace/eval feedback loop. WP-38 is a continuation of this workstream's feedback-loop closure, not duplicate instrumentation.
- `WP-35` — Eval readback claim gate.
- `WP-36` — Default eval proof gate.
- `WP-37` — Mixed-source local value proof.
- `FLO-1` — Default eval-driven self-improvement loop.
- `DEC-250` — Measurable automation requires eval projection and KR-gated success.
- `DEC-251` — Command success is not outcome success.
- `DEC-252` — Sample-bound proof must declare its generalization limit.
- `DEC-64` — No-human steady state requires held-out >=98% proof and safety guardrails.
- `PRI-1` / `BR-1` — Privacy by design and metadata-only hosted observability.
- `STD-17` — Provider-agnostic LLM behavior must be measured before trust.
- `TEN-23` — Avoid bespoke optimization drift.

## Scope

Add a local, read-only command:

```text
mindline eval loop-decision <current-run-or-readback-or-proof-dir> --out <dir> [--baseline <baseline-run-or-readback-or-proof-dir>]
```

The command consumes existing local artifacts, including supported WP-35 readback summaries, WP-36 proof packets, WP-37 value-proof summaries, and source/corpus trace artifacts already supported by readback.

It writes:

- `eval-loop-decision/decision-packet.json`
- `eval-loop-decision/decision-report.md`
- `eval-loop-decision/chain-capture-draft.md`

## Contract

The packet must:

1. Emit exactly one `top_improvement_target`.
2. Mark the target as product-general or blocked from product-generalization.
3. Compare baseline/current only when readback/proof evidence says the artifacts are comparable.
4. Emit `improved`, `not_improved`, `inconclusive`, or `not_comparable`.
5. Include explicit claim statuses for `safety`, `improvement`, `generalization`, and `dec64`.
6. Include one concrete rerun instruction using existing Mindline commands.
7. Copy KRs and guardrails from artifacts; never invent them.
8. Keep all outputs PR/PB-safe: no raw source text, prompts, completions, private paths, permalinks, secrets, candidate bodies, or provider payloads.
9. Preserve zero prohibited side-effect counters.

## KRs

1. Fixture current-only run produces a decision packet with one top target, `improvement=blocked_missing_baseline`, safety pass, and rerun instruction.
2. Fixture baseline/current comparable run produces `improved` when readback comparison has positive supported deltas.
3. Missing or non-comparable baseline produces `not_comparable` / blocked improvement, not an improvement claim.
4. Generalization and DEC-64 remain blocked unless held-out evidence satisfies their existing proof gates.
5. Side-effect counters are zero for network, hosted telemetry, hosted inference, Slack API, browser, destination writes, Product Brain writes, Tolaria writes, auto-accept, and no-human claims.
6. Output leak scan rejects denied private patterns.
7. Focused tests, `go test ./...`, `git diff --check`, runtime proof on the WP-37 fixture, PB audit, and LOOP review pass.

## Exclusions

- No prompt tuning.
- No classifier changes.
- No automated fix execution.
- No generated answer keys.
- No hosted PostHog query or hosted dependency.
- No Slack API, browser, web fetch, auth, DB, or background job.
- No destination writes.
- No Product Brain auto-write.
- No Tolaria write.
- No no-human, auto-accept, production-ready, generalization, or DEC-64 claim unless the existing proof evidence actually permits it.

## Reviewer Sign-Off

- Chain Steward: SIGN-OFF with scope edits: reconcile WP-27, keep side-effect free, decision support only, comparable baseline required, DEC-64/generalization blocked without held-out proof.
- Domain/User Job: SIGN-OFF: next slice must make proof actionable with one prioritized target and concrete rerun instruction.
- Systems Architect: SIGN-OFF: implement as thin `eval loop-decision` layer over existing artifacts.
- Delivery/Risk: SIGN-OFF: read-only, metadata-only, no hosted/destination behavior, explicit claim statuses and side-effect counters.
