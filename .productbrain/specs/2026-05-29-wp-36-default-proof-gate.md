# WP-36 Default Eval Proof Gate Spec

**Status:** signed-spec-candidate V2
**Date:** 2026-05-29
**Product:** Mindline
**Owning workstream:** WS-2 Evidence and evaluation foundation
**Strategy:** STR-3 Autonomy-readiness before destination writes
**Governing rules:** PRI-1, BR-1, STD-17, DEC-250, DEC-251, DEC-252, FLO-1, TEN-23
**Input evidence:** WP-35, DEC-296
**Stop mode:** Full delivery

## 1. Diagnosis

WP-35 made eval readback truthful, but it is still a report. A report can say "improvement blocked" and still exit successfully because the reporting command did its job. That is correct for inspection, but it is not strong enough as the default proof standard for future work.

The failure mode is now sharper:

- agents can run readback, see blocked claims, and still present the PR as product progress;
- Chain capture can mention a readback artifact without making the pass/fail policy explicit;
- a private or sample-bound run can be useful evidence, but it must not silently become a broad product claim;
- future work can avoid the feedback loop unless there is a simple local command that makes the outcome executable.

WP-36 turns WP-35's readback into a strict local proof gate. The gate does not replace readback. It consumes readback evidence, applies a named claim profile, emits a machine-readable proof packet, writes a reviewer-facing report, and exits nonzero when mandatory gates for the selected claim are failed or blocked.

The gate is claim-specific. A safety proof should not fail just because an improvement baseline is absent. An improvement proof must fail without comparable baseline/current evidence. A generalization proof must fail for private or sample-bound evidence. A DEC-64/no-human proof must fail unless held-out threshold and safety evidence pass.

## 2. Outcome

After WP-36, every relevant Mindline PR/work package can run one local command and get a hard answer for the claim it wants to make:

> Does this run have enough safe local evidence to support this specific claim, and what claims remain blocked?

The intended user outcome is not "another artifact." It is a higher accountability floor:

- Codex can no longer treat command success as outcome success for eval-sensitive work.
- Randy/reviewers can see the exact pass/fail policy and blocked claims without interpreting raw JSON.
- Product Brain capture gets a safe proof summary, not an overclaimed success note.
- Future source/destination/provider work has the same gate, whether the data comes from Slack, Markdown, Notion exports, transcripts, Tolaria, Product Brain, Linear, or another adapter.

## 3. Product Model Fit

WP-36 is source-neutral, destination-neutral, provider-neutral, and local-first. It operates on eval/readback artifacts and claim policies, not on one corpus, one `/temp` run, one Slack export, one OpenAI model, one Tolaria folder, or one Product Brain proposal shape.

The proof gate is in the evaluation/readback layer. It must not move rules into source adapters or destination adapters.

## 4. Scope

In scope:

1. Add a strict CLI command:

```bash
mindline eval proof-gate <run-or-readback-dir> --out <dir> --claim safety|improvement|generalization|dec64 [--baseline <run-or-artifact-dir>]
```

2. Build on WP-35 readback:
   - run or reuse readback summary logic for current and baseline;
   - consume an existing `eval-readback/readback-summary.json` when supplied;
   - require comparable baseline evidence only for improvement claims;
   - preserve readback's safe root labels and relative refs.
3. Emit:
   - `eval-proof/proof-packet.json`;
   - `eval-proof/proof-report.md`;
   - `eval-proof/chain-capture-draft.md`;
   - nested `eval-proof/readback/` artifacts when the command builds readback from artifact dirs.
4. Apply closed claim profiles:
   - `safety`: requires artifact presence, supported schemas, privacy-safe readback, and side-effect safety.
   - `improvement`: all `safety` requirements plus `improvement_claim=pass`.
   - `generalization`: all `safety` requirements plus `generalization_claim=pass`.
   - `dec64`: all `safety` requirements plus `dec64_no_human_claim=pass`.
5. Exit behavior:
   - exit `0` only when the selected claim profile verdict is `pass`;
   - exit nonzero when mandatory gates fail or are blocked;
   - valid proof packets always include deterministic `claim`, `verdict`, `exit_code`, and stable failure codes;
   - usage errors remain distinct from process/gate failures.
6. Privacy and safety:
   - proof outputs are metadata-only and safe for PR/Product Brain capture;
   - no raw private source text, prompts, completions, permalinks, private absolute paths, Dropbox paths, home paths, or secret-looking strings;
   - no network, Slack API, browser, hosted telemetry, hosted inference, Product Brain write, Tolaria write, or destination write.

Out of scope:

- CI integration or GitHub required checks;
- hosted PostHog readback or dashboards;
- changing semantic extraction/classifier quality;
- destination writes or autonomous approval;
- database/auth/workspace storage;
- generating answer keys;
- tuning to `/temp`, Randy Slack, Tolaria, Product Brain, or OpenAI.

## 5. Proof Summary Contract

`mindline-eval-proof-packet/v0.1`

Required fields:

- `schema_version`;
- `run_id`;
- `claim`;
- `verdict`: `pass`, `fail`, or `blocked`;
- `exit_code`;
- `input_root_label`;
- `baseline_root_label`;
- `readback_summary_ref`;
- `eval_projection`;
- `mandatory_gates[]`;
- `blocked_claims[]`;
- `failed_claims[]`;
- `permitted_claims[]`;
- `generalization_limit`;
- `top_improvement_target`;
- `rerun_instructions[]`;
- `safe_artifact_refs[]`.

Exit semantics:

- `0`: selected claim verdict is `pass`.
- nonzero process/gate failure: selected claim verdict is `fail` or `blocked`.
- usage errors remain CLI usage failures and may not masquerade as proof packets.

Each mandatory gate has:

- `gate`;
- `required_statuses[]`;
- `actual_status`;
- `verdict`;
- `reason_codes[]`;
- `claim_impact`.

The `eval_projection` must state:

- intended users;
- input/source types;
- output/destination surfaces;
- workspace assumptions;
- provider/model assumptions;
- privacy boundary;
- sample status;
- held-out/generalization claim;
- KR thresholds;
- guardrails.

Claim-specific hard blockers:

- `improvement` without `--baseline` is `blocked` with `missing_baseline`; a current run or prior readback alone cannot imply improvement.
- `dec64` requires a passing `dec64_no_human_claim` gate whose supporting readback evidence includes held-out labels, threshold proof, no-human eligibility, non-generalizable blocking, and side-effect safety.

## 6. Aggressive KRs

1. **Executable proof:** `eval proof-gate` exits nonzero when mandatory gates for the selected claim fail or are blocked.
2. **Claim-specific correctness:** `safety` can pass without a baseline, while `improvement` fails for missing baseline, not-comparable baseline, unchanged runs, or regressed runs.
3. **Improvement standard:** `improvement` cannot pass unless `improvement_claim=pass`; readback-only success is insufficient.
4. **Generalization honesty:** `generalization` and `dec64` fail for private runtime, `/temp`, one-workspace, one-provider, or non-held-out evidence unless converted into reusable fixture/held-out proof.
5. **DEC-64 floor:** `dec64` fails unless the readback evidence proves held-out labels, threshold >=98%, no-human eligibility, absence of non-generalizable evidence, and zero destination-write/autonomy side-effect counters.
6. **Reviewer clarity:** proof report states in plain English what passed, what is blocked, what cannot be claimed, and the next product-general improvement target.
7. **Privacy:** proof outputs and nested readback outputs contain zero private absolute paths, Dropbox paths, home-directory paths, Slack permalinks, raw private URLs, prompts, completions, source excerpts, candidate summaries, provider payloads, or secret-looking strings.
8. **Product generality:** fixtures prove the gate works from artifact contracts only and does not require OpenAI, Gemini, Claude, OpenRouter, PostHog, Slack, Tolaria, Product Brain, Notion, Linear, or Obsidian.
9. **Side-effect floor:** gate proof includes zero network, hosted telemetry, hosted inference, browser, Slack API, Product Brain, Tolaria, destination write, auto-accept, and committed-private-artifact counters for safety claims.
10. **Stable failure codes:** proof packets use stable reason codes including `missing_proof`, `missing_baseline`, `not_comparable`, `kr_failed`, `guardrail_failed`, `unsafe_output`, `unsupported_artifact`, and `non_generalizable`.
11. **Chain readiness:** the generated Chain draft is safe to paste and says the claim verdict plus blocked claims; it does not auto-write to PB.
12. **Review standard:** focused tests, full tests, diff check, leak scan, PB audit, runtime proof on committed fixtures, and LOOP reviewer sign-off pass before PR.

## 7. Risks

1. **Over-strict gate blocks useful private learning.** Mitigation: `safety` and `improvement` claims can pass when their mandatory evidence passes while broad/no-human claims remain blocked.
2. **Gate duplicates readback logic.** Mitigation: proof gate consumes the same `evalreadback.Summary` and only adds policy evaluation/output.
3. **Claim naming creates fake certainty.** Mitigation: proof packet lists permitted, blocked, and failed claims separately.
4. **Future agents bypass the gate.** Mitigation: update AGENTS.md and Chain so relevant PRs treat proof gate as the default when readback artifacts exist.
5. **Privacy leakage through nested artifacts.** Mitigation: reuse WP-35 sanitization and run leak scan across the entire proof output tree.

## 8. Done Proof

WP-36 is done only when:

1. `mindline eval proof-gate` exists with the specified claim-profile contract.
2. `safety`, `improvement`, `generalization`, and `dec64` claims are tested.
3. Missing baseline, not-comparable, unchanged, regressed, unsafe, unsupported, and side-effect failure cases fail or block correctly.
4. Passing fixture proof emits JSON, Markdown, Chain draft, and nested readback artifacts.
5. Proof artifacts are metadata-only and leak-scan clean.
6. Chain captures WP-36 and links it to the governing strategy/rules and WP-35.
7. Product Brain audit is pass or warn-only with reconciliation.
8. LOOP spec/plan/delivery/review sign-off is recorded.
