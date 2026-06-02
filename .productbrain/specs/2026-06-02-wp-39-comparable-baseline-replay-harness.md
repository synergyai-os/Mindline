# WP-39 - Comparable Baseline Replay Harness

## Outcome

Mindline can turn a local readback run into a reusable comparable baseline packet for the next improvement PR. The packet must prove that the run is safe to reuse as a baseline, expose the corpus and command-configuration replay contract, and let `eval proof-gate` and `eval loop-decision` compare a later current run against that baseline without reinterpreting command success as improvement.

This is the first PR in `DEC-317` after PR 33. It raises the bar from "baseline supplied" to "baseline is replay-ready and proof-consumable."

## Chain Authority

- `DEC-317` - signed next PR sequence after PR 33 merge.
- `WP-38` - eval feedback loop decision packet; top target was `establish_comparable_baseline`.
- `WP-35` - eval readback claim gate.
- `WP-36` - default eval proof gate.
- `WP-37` - mixed-source local value proof.
- `FLO-1` - default eval-driven self-improvement loop.
- `STR-3` - autonomy-readiness before destination writes.
- `DEC-250` - measurable automation requires eval projection and KR-gated success.
- `DEC-251` - command success is not outcome success.
- `DEC-252` - sample-bound proof must declare its generalization limit.
- `DEC-64` / `KEY-3` - no-human readiness requires held-out >=98% threshold proof and safety guardrails.
- `PRI-1` / `BR-1` - privacy by design and metadata-only observability.
- `TEN-23` - avoid bespoke optimization drift.

## Product Model Fit

Eligibility: `EXTEND`.

WP-39 extends the existing local eval/readback/proof contract rather than creating a new evaluator. The product object is the local eval evidence packet: value-proof and other eval artifacts feed readback; readback produces claim gates and a reusable replay contract; proof-gate and loop-decision consume that contract for pass/block decisions.

This is not bespoke to one fixture, Slack sample, destination, provider, or workspace. It strengthens the core review-learning layer so future source and destination adapters inherit the same comparability rule.

## Scope

In scope:

1. Add a first-class `replay_baseline` contract to eval readback summaries and reports.
2. Mark replay baselines `ready` only when they have exactly one corpus fingerprint, exactly one command-configuration fingerprint, supported schemas, privacy-safe artifacts, zero prohibited side-effect counters, and at least one supported artifact.
3. Mark replay baselines `blocked` with reason codes when any readiness condition is missing or unsafe.
4. Let `eval proof-gate --baseline <readback-output-or-summary>` consume an existing readback summary as the baseline evidence.
5. Preserve `eval loop-decision --baseline <readback-output-or-summary>` behavior and include replay-baseline state in its persisted readback output through the refreshed summary.
6. Keep all emitted refs PR/PB-safe: no raw source text, prompts, completions, private paths, permalinks, secrets, candidate bodies, or provider payloads.

Out of scope:

- No prompt tuning.
- No classifier changes.
- No answer-key generation.
- No hosted PostHog query or hosted dependency.
- No Slack API, browser, web fetch, auth, DB, or background job.
- No destination writes, Product Brain auto-write, Tolaria write, auto-accept, no-human claim, generalization claim, or DEC-64 claim.
- No PR 2 improvement implementation; WP-39 only makes the baseline comparable enough for PR 2 to select one target.

## Higher KRs

1. Current-only readback over a supported value/eval run emits `replay_baseline.status=ready` only when both `corpus_fingerprint` and `command_config_fingerprint` are present and non-conflicting.
2. Missing corpus fingerprint, missing command-config fingerprint, conflicting fingerprints, unsafe artifacts, unsupported schemas, or nonzero prohibited side-effect counters emit `replay_baseline.status=blocked` with concrete reason codes.
3. `eval proof-gate CURRENT --baseline BASELINE_READBACK --claim improvement` can compare against a prior readback output and pass only when current improves against that comparable baseline.
4. `eval proof-gate` blocks improvement when the supplied baseline readback is replay-blocked or non-comparable.
5. `eval loop-decision` with a readback baseline keeps improvement/generalization/DEC-64 honest and never treats a replay-blocked baseline as improvement proof.
6. Readback report and Chain draft name the replay-baseline state and rerun instruction in plain English.
7. Guardrails remain zero for network, hosted telemetry, hosted inference, Slack API, browser, destination writes, Product Brain writes, Tolaria writes, auto-accept, no-human claims, and committed private artifacts.
8. Focused tests, full `go test ./...`, `git diff --check`, runtime value-proof -> readback baseline -> proof-gate -> loop-decision proof, generated-output leak scan, PB audit, and LOOP review pass.

## Behavior Impact

Before WP-39, agents could provide a baseline path but still fail to prove that the next run was replay-compatible. After WP-39, the first artifact in the improvement loop answers:

- can this run serve as a baseline;
- what exact corpus/config identity must be replayed;
- why a baseline is blocked if it cannot be reused;
- whether proof-gate and loop-decision can trust the comparison.

This lets PR 2 improve one target from evidence instead of preselecting a fix.

## Architecture

Ownership stays in the review-learning layer:

- `internal/evalreadback` owns the replay baseline contract and readback summary/report projection.
- `internal/evalproof` owns claim-specific pass/block behavior and must accept readback summaries as baseline evidence.
- `internal/evalloopdecision` remains decision support over readback/proof evidence.
- `internal/documents` value-proof remains an upstream artifact producer only.
- Destination adapters remain out of scope.

No new provider, source, destination, auth, DB, network, or hosted telemetry layer is introduced.

## Acceptance

The PR is complete only when:

1. A signed spec and plan are captured or linked on Chain.
2. A durable `WP-39` work package materializes this spec without weakening KRs or exclusions.
3. PB audit for WP-39 has no blocking failures.
4. TDD red/green evidence exists for readback replay-baseline readiness and proof-gate baseline summary consumption.
5. Runtime proof demonstrates a WP-37-style value-proof run can become a replay-ready baseline and feed a later proof-gate/loop-decision comparison.
6. LOOP reviewers sign off on final delivery evidence.
