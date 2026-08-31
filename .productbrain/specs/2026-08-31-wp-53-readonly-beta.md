# WP-53 read-only beta authority brief

Status: proposed

## Shape

### Problem

Mindline already lets an agent discover and query personal evidence, but natural-language recall is still too brittle. The current WP-53 branch mixed that user job with an audit-grade proof system. It grew to 136 changed files and 37,734 added lines without reaching `main`, while the user still cannot test the retrieval improvement.

### Product outcome

A fresh agent in any local repository can discover Mindline, use an existing owner-approved project connection, ask its own natural-language question, and receive either:

1. up to three useful, cited personal-evidence records relevant to that project and lens; or
2. an honest abstention when Mindline has no sufficiently relevant evidence.

Mindline remains personal, advisory evidence. It is not Product Brain or organizational truth.

### Direction

Ship a proportional read-only beta from current `origin/main`. Preserve only the retrieval and privacy behavior that directly serves the outcome. Defer the audit-grade private one-shot protocol and its exact-build, process-identity, adoption, namespace, and recovery machinery.

Product Model Fit: **EXTEND** the existing agent-first recall contract governed by DEC-424, DEC-425, WP-53, WP-55, and WP-56. The reusable pattern is project-scoped personal-evidence retrieval; this is not a private-corpus or one-repository workaround.

### Trust boundary

Trust the local owner account and operating system. Defend against accidental concurrency, stale/corrupt state, wrong project binding, invalid citations, cross-project leakage, private-data logging, and partial writes. Do not claim protection against malicious software already running as the same operating-system user, root, or kernel compromise.

## Fail-able specification

### Required behavior

1. Every returned citation independently passes the existing query-only evidence policy after resource-owner expansion.
2. Project, lens, and agent learning may reorder eligible evidence but may never make ineligible evidence returnable.
3. Retrieval combines exact-word and local meaning signals deterministically and remains provider-neutral behind existing ports.
4. A returned citation atomically records its exact qualifying source: the record source or one current resource. Scoped hydration returns only that source; unrelated sibling resources and historical revisions remain hidden.
5. Unanswerable or unauthorized queries abstain instead of producing a weak answer.
6. Existing installed state and project connections remain readable; no migration may delete or silently recreate personal evidence.
7. Search is advisory and destination-neutral. This slice performs no Product Brain, Tolaria, Notion, Linear, or other destination write.
8. For the same library and query, query-only eligible membership is identical across scopes, lenses, and agents. Context and feedback may change ordering only. Feedback and learned state never cross their bound context.
9. “Read-only” means personal evidence and destinations are read-only. Search may write owner-private embedding/index metadata and one atomic retrieval receipt with the exact qualifying-source binding. Feedback writes only when separately requested. Search/get never modify evidence, scopes, lenses, actors, project connections, or destinations.

### Beta scorecard

The frozen public scorecard at `.productbrain/specs/fixtures/wp53-readonly-beta-scorecard-v1.json` contains eight paraphrased answerable cases, four context-bound known-absent cases, four owner-approved contexts, exact shared-membership/order answers, a fully specified multi-resource hydration-isolation case, and one fixed measurement profile. Before retrieval implementation, record the exact `origin/main` result. The exact merge candidate must prove:

1. At least 7/8 answerable cases place the exact expected governed record in the first three, improve by at least two hits over `origin/main`, and include at least one predeclared paraphrase that main misses and the candidate finds.
2. All 4 absent cases abstain; zero wrong evidence is returned.
3. Every returned citation hydrates to its exact qualifying source and supports the result. The multi-resource case exposes neither its sibling source nor historical revision.
4. Eligible membership for the shared query is identical across both lenses while ordering differs; zero cross-context feedback/state leakage occurs.
5. Existing agent discovery, project connection, feedback, recall, and hydration tests remain green.
6. Warm fixture p95 is at most 10 seconds. Baseline and candidate must resolve, record, and exactly match the manifest's semantic-model content digest before any case is scored; a mismatch fails before comparison. This is a bounded local beta gate, not a broad production claim.
7. A sentinel test proves queries, source text, local paths, handles, and exact resource bindings never enter stdout/stderr service logs or hosted telemetry.
8. Upgrade proof starts from the exact released `origin/main` binary with pre-existing evidence and one project connection, upgrades to the candidate, restarts, recalls and hydrates, preserves IDs/counts/connection, rolls back to main, restarts/resolves, then rolls forward without evidence or connection loss. Old retrieval receipts remain readable but cannot authorize hydration without an explicit qualifying-source binding; never infer that binding from scores.
9. Run baseline with `MINDLINE_WP53_SCORECARD_MODE=baseline MINDLINE_WP53_SCORECARD_REPORT=.productbrain/proof/wp53-main-scorecard.json go test -count=1 -run '^TestWP53ReadonlyBetaScorecard$' ./internal/localservice` and candidate with `MINDLINE_WP53_SCORECARD_MODE=candidate MINDLINE_WP53_SCORECARD_REPORT=.productbrain/proof/wp53-candidate-scorecard.json go test -count=1 -run '^TestWP53ReadonlyBetaScorecard$' ./internal/localservice`. Each emits a JSON report containing source commit, manifest hash, resolved model digest, per-case ranked IDs, absent decisions, qualifying-source checks, measurement-profile identity, latency samples/p95, and pass/fail. Upgrade proof is `go test -count=1 -run '^TestWP53ReadonlyBetaUpgradeRollback$' ./internal/localservice`.
10. Full repository tests, vet, race tests for changed concurrent paths, diff checks, and the existing secret/private-data scanner pass. Required commands: `go test -count=1 ./...`; `go vet ./...`; `go test -race -count=1 ./internal/personalmemory ./internal/agentretrieval ./internal/agentstate ./internal/localservice ./internal/cli`; `git diff --check origin/main...HEAD`; `go run ./cmd/mindline-secret-check --root . --self-test --proof-dir .productbrain/proof`. The scanner must report `state=clean`, `self_test=passed`, and zero findings.
11. The reviewed PR head tree equals merged `main`; record the installed binary checksum and source commit. The same installed binary passes smoke tests from two unrelated repositories using two distinct owner-approved opaque project connections. Each fresh agent chooses its own question; repository/CWD inference and automatic binding are forbidden.
12. The final handoff gives Randy one minimal prompt for a fresh non-Codex agent. WP-53 remains `building`/validated-staging until Randy reports that outside-agent result; only then may it become shipped.

### Explicit exclusions

- No private one-shot scored proof before merge.
- No exact baseline/candidate binary pairing or external builder receipts.
- No process tracing, executable attestation, private-context adoption, closed proof namespace, immutable proof sentinel, or audit-only recovery schemas.
- No claim of broad retrieval quality, generalization, autonomous writes, or hostile same-user tamper resistance.
- No new database, hosted retrieval, graph search, MCP server, UI, ingestion expansion, or destination integration.
- No hostile same-user, root, or kernel tamper-resistance claim.

### Delivery constraints

- Start from current `origin/main`; do not merge or cherry-pick the audit branch wholesale.
- Reuse the smallest product changes from the prior candidate only after checking them against current main.
- Audit-only code and historical proof fixtures stay out of the beta branch.
- Preserve the current WP-55/WP-56 agent search/get route and project resolver. The beta may not import or add routes from `internal/recalleval`, `internal/exactpreflight`, `internal/fileidentity`, `internal/evidenceidentity`, `internal/projectresolution`, exact-preflight commands/tests, recovery-proof packages, or historical proof fixtures.
- Use an additive downgrade-safe qualifying-source table or representation that current main can ignore. Rollback must not reject the existing recovery sidecar. Missing bindings make old runs untrusted for hydration and require a fresh search.
- The founder's direct 2026-08-31 instruction to update the goal and finish from `main` authorizes this final mechanical manifest-closure correction and one unchanged-artifact full-panel rerun. No further pre-build correction is allowed: a remaining material blocker stops. Implementation has one correction attempt and a mandatory clean rereview. A new concern outside this threat model becomes follow-up Chain work, not silent scope expansion.
- Merge only the exact independently reviewed commit through a PR. Install and smoke-test the exact merged `main` build, then clean owned delivery branches/worktrees.

## Implementation sequence

1. Identify the minimal runtime retrieval delta and its direct tests from the prior candidate.
2. Port/refactor that delta onto current `origin/main`, preserving current WP-55/WP-56 discovery and connection behavior.
3. Add the bounded two-project/two-lens scorecard and privacy/regression checks.
4. Run full validation, then independent Product, Architecture, Delivery Quality, Chain, and Risk review on one frozen commit.
5. Fix only in-scope blockers once; rerun the same panel on the new frozen commit.
6. Push, open the PR, resolve bounded current-head review, merge the identical tree, install, smoke-test from two repositories, reconcile Product Brain, and clean owned work.

## Supersession

When ratified through a governing decision, this brief supersedes WP-53's current audit-grade build contract and plan as the release authority for the read-only beta while preserving DEC-446's per-citation safety rule. Ratification must materialize WP-53 from this brief and create one deferred audit-grade work package owning the historical private proof contract and TEN-58. Replace the current `WP-53 validates KEY-15` relation with `deferred audit work package validates KEY-15`; move TEN-58's blocking relation to that deferred item without marking TEN-58 or KEY-15 achieved. Prior proof code and artifacts remain historical evidence only. Beta merge/install may reach validated-staging; only Randy's fresh-agent result may mark WP-53 shipped.
