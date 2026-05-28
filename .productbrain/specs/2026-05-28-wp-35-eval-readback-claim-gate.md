# WP-35 Eval Readback Claim Gate Spec

**Status:** signed-spec V2
**Date:** 2026-05-28
**Product:** Mindline
**Owning workstream:** WS-2 Evidence and evaluation foundation
**Strategy:** STR-3 Autonomy-readiness before destination writes
**Governing rules:** PRI-1, BR-1, STD-11, STD-17, DEC-250, DEC-251, DEC-252, FLO-1, TEN-23
**Input evidence:** WP-23, WP-27, WP-29, WP-30, WP-34, DEC-241
**Stop mode:** Full delivery

## 1. Diagnosis

Mindline now emits many local trace/eval artifacts: semantic trace summaries, corpus-pressure summaries, autonomy-readiness reports, corpus acceptance benchmarks, and WP-34 link-enrichment eval projections. That is progress, but the feedback loop still depends on an agent manually remembering which artifact matters and how to interpret it.

This creates the failure mode DEC-251 names: command success can be mistaken for product outcome. It also keeps TEN-23 alive: a private `/temp` or Slack run can look like proof even when it is sample-bound, non-held-out, or not comparable to a baseline.

The next product slice is therefore not destination writes, auth, DB, UI, or a PostHog dashboard. The next slice is a local readback command that turns existing trace/eval artifacts into one claim-gating packet:

> What evidence exists, what changed versus baseline, can this result be generalized, what claims are blocked, and what is the next product-general improvement target?

## 2. Spec/Plan Loop Ledger

### Pass 1

Initial scope: "use PostHog/evals more so the system can self-improve."

Reviewer pressure:

- Chain/Product Strategy: PostHog must not become the canonical eval store; local truth remains source of record.
- Systems/Eval Architecture: do not add another evaluator; first make existing evaluators auditable and comparable.
- Risk/Safety: raw private source text, prompts, completions, paths, permalinks, and reconstructable excerpts must not be exported or copied into top-level summaries.
- Product Model Fit: the slice must support any future source/destination/provider, not just `/temp`, Randy Slack, OpenAI, or Tolaria.

### Pass 2

Tightened scope: add a local `eval readback` command that consumes existing local artifacts, emits a canonical readback packet, blocks unsupported claims, and gives the next generalized improvement target. PostHog remains optional downstream projection/readback later; it is not needed for this PR's product truth.

## 3. Outcome

After WP-35, Codex or a future agent can run one command against a run directory and get a truthful answer:

- which trace/eval artifacts were found;
- which KRs passed or failed;
- whether the run is private/sample-bound/non-generalizable;
- whether an improvement claim is valid against a comparable baseline;
- whether a no-human / DEC-64 style claim remains blocked;
- what generalized product behavior should improve next;
- exactly which local artifacts should be rerun or inspected next.

This is the short-term self-improvement foundation: Codex is still the active operator, but it now has a product-level readback surface that prevents overclaiming and makes learning repeatable.

## 4. Product Model Fit

WP-35 is source-neutral and destination-neutral. It consumes artifact shapes, not Slack messages, Notion docs, Tolaria notes, Product Brain entries, or provider-specific traces. It applies to:

- semantic extraction/classification runs;
- corpus-pressure runs;
- link-enrichment loops;
- autonomy-readiness reports;
- held-out corpus acceptance benchmarks;
- future provider/model comparisons.

It does not decide destination writes or autonomous acceptance. It tells the system whether those claims are currently justified.

## 5. Scope

In scope:

1. Add a read-only CLI command:

```bash
mindline eval readback <run-or-artifact-dir> --out <dir> [--baseline <run-or-artifact-dir>]
```

2. Recursively detect supported local artifact files under the current run root:
   - `trace/trace-summary.json`;
   - `corpus-pressure/pressure-summary.json`;
   - `corpus-pressure/eval-input.json`;
   - `corpus-pressure/trace-summary.json`;
   - `corpus-pressure-loop/loop-summary.json`;
   - `corpus-acceptance/benchmark-summary.json`;
   - `autonomy-readiness/readiness-report.json`;
   - `link-enrichment/loop-summary.json`;
   - `link-enrichment/comparison/comparison-summary.json`;
   - `link-enrichment/requests/link-artifact-requests.json`;
   - `link-enrichment/posthog/eval-projection.json`.
3. Emit:
   - `eval-readback/readback-summary.json`;
   - `eval-readback/readback-report.md`;
   - `eval-readback/chain-capture-draft.md`;
   - `eval-readback/comparison-summary.json` when `--baseline` is supplied.
4. Normalize evidence into closed-vocabulary gates:
   - `artifact_detected`;
   - `kr_passed`;
   - `kr_failed`;
   - `claim_blocked`;
   - `non_generalizable`;
   - `not_comparable`;
   - `unsafe_or_leaky`;
   - `needs_held_out_labels`;
   - `needs_source_enrichment`;
   - `needs_evidence_readiness`;
   - `needs_failure_taxonomy_review`;
   - `needs_provider_comparison`;
   - `ready_for_next_pressure_run`.
5. Gate improvement claims:
   - improvement requires current and baseline artifacts;
   - baseline/current must be comparable by corpus/config fingerprint when those fields exist;
   - at least one supported metric delta must improve;
   - no safety/guardrail counter may regress;
   - private runtime or non-held-out proof must remain marked non-generalizable.
6. Gate generalization claims:
   - block if `non_generalizable_runtime=true`;
   - block if held-out proof is absent for DEC-64/no-human readiness;
   - block if sample status is private, `/temp`, one workspace, one provider, or one customer without reusable fixture/held-out evidence.
7. Produce a next-improvement target with:
   - closed-vocabulary code;
   - plain-English rationale;
   - evidence artifact references relative to input root;
   - rerun instruction.
8. Preserve privacy:
   - top-level readback artifacts contain metadata, counts, reason codes, relative artifact references, and redacted labels only;
   - `input_root`, `baseline_root`, output root references, and proof locations are represented as safe root labels and relative artifact refs, never private absolute paths;
   - no source excerpts, prompt bodies, completions, raw Slack text, raw URLs containing secrets, private permalinks, absolute private paths, destination note bodies, or Chain governance content copied from private sources.

Out of scope:

- live PostHog querying or dashboards;
- hosted LLM-as-judge over private traces;
- live web fetching, Slack API calls, browser automation, auth/login, DB/storage;
- Tolaria writes, Product Brain writes, destination apply payloads, auto-accept, or no-human approval;
- tuning classifier prompts or extraction behavior to pass `/temp`;
- generating answer keys from the evaluated run;
- changing existing artifact schemas except for strictly additive metadata needed for readback.

## 6. Contracts

### Readback Summary

`mindline-eval-readback-summary/v0.1`

Required fields:

- `schema_version`;
- `run_id`;
- `input_root_label`;
- `baseline_root_label` when supplied;
- `artifact_count`;
- `artifact_type_counts`;
- `sample_status`: `fixture`, `held_out`, `private_runtime`, `temp_runtime`, `unknown`;
- `generalization_status`: `generalizable`, `non_generalizable`, `blocked`;
- `improvement_status`: `improved`, `unchanged`, `regressed`, `not_comparable`, `not_evaluated`;
- `claim_gates[]`;
- `guardrails`;
- `top_improvement_target`;
- `rerun_instructions[]`;
- `safe_artifact_refs[]`.

### Claim Gate

Each gate has:

- `gate`;
- `status`: `pass`, `fail`, `blocked`, `not_applicable`;
- `reason_codes[]`;
- `evidence_refs[]`;
- `claim_impact`: what claim this permits or blocks.

### Chain Capture Draft

The draft is local only. It must be safe to paste into Product Brain and must include:

- the truthful result;
- the blocked claims;
- the next improvement target;
- the generalization limit;
- local proof references as safe labels and relative artifact paths.

It must not auto-write to Product Brain.
It must not include `/private/tmp`, Dropbox paths, home-directory paths, Slack permalinks, secret-bearing URLs, raw private URLs, or absolute runtime paths.

## 7. Aggressive KRs

1. **Artifact coverage:** readback detects at least five supported artifact types across committed fixtures and at least one real runtime output under `/private/tmp`.
2. **Claim blocking:** private runtime and `/temp` proofs are marked non-generalizable unless held-out/reusable fixture evidence is present.
3. **Improvement gating:** a current run cannot claim improvement without a comparable baseline and positive delta; fixture tests cover improved, unchanged, regressed, and not-comparable cases.
4. **Next target quality:** every failed or blocked readback emits exactly one top generalized improvement target and at least one rerun instruction.
5. **Privacy:** generated readback artifacts contain zero raw source excerpts, prompts, completions, private absolute paths, Dropbox paths, home-directory paths, Slack permalinks, secret-looking strings, or raw private URLs.
6. **Provider/destination neutrality:** fixtures prove readback works without OpenAI, Gemini, Claude, OpenRouter, PostHog, Slack, Tolaria, or Product Brain availability.
7. **No side effects:** the command performs no network, hosted telemetry, Slack API, browser, Product Brain, Tolaria, or destination writes.
8. **Runtime proof:** readback runs successfully over at least one WP-34 synthetic output and one real/private runtime output, with private output marked non-generalizable.
9. **Auditability:** `readback-report.md` explains the verdict well enough that a reviewer can understand why a claim passed or failed without opening JSON first.
10. **Regression safety:** focused tests, `go test ./...`, `git diff --check`, generated-output leak scan, PB audit warn-only-or-better, and LOOP reviewer sign-off pass.

## 8. Risks

1. **Reporting without learning.** Mitigation: every failed/blocked run must emit one next-improvement target and rerun instruction.
2. **False comparability.** Mitigation: require matching fingerprints when available; otherwise mark comparison `not_comparable`.
3. **Privacy leakage through summaries.** Mitigation: allowlisted fields only, relative artifact refs only, leak-scan proof.
4. **PostHog distraction.** Mitigation: hosted readback is out of scope; local readback is canonical.
5. **Overclaiming autonomy.** Mitigation: DEC-64/no-human claims remain blocked without held-out labels and threshold proof.

## 9. Done Proof

WP-35 is done only when:

1. `mindline eval readback` exists and writes the three required local artifacts.
2. The reader detects all in-scope artifact types it encounters and records unknown supported-looking artifacts safely.
3. Claim gates block unsupported improvement/generalization/no-human claims.
4. Baseline/current comparison works only for comparable runs and reports deltas from artifact data, not command success.
5. Top improvement target and rerun instructions are generated for failed/blocked runs.
6. Tests cover artifact detection, claim gates, comparison states, privacy filtering, no-side-effect behavior, and CLI errors.
7. Runtime proof covers committed fixtures and private `/private/tmp` artifacts without committing private outputs.
8. Chain is updated with the WP, spec/plan authority, and final proof.
9. LOOP reviewers sign off on spec, plan, delivery, and review phases.
