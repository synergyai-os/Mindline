# WP-53 read-only beta delivery plan

Status: proposed

Authority:

- DEC-468
- Spec SHA-256 `e1b836f0054da142d0965d0707e27823fd62d6ace9cec02daeb9b3461aaedc88`
- Scorecard SHA-256 `8bea01faad9b94d66eeec9327dba401db20dcbc5b31f28990759e98684247ac5`
- Base `origin/main` commit `bf49078a6c1317b6d383285f52ab6e2a51ee2738`

## Delivery sequence

1. Commit only this signed authority on `codex/wp53-readonly-beta`. Confirm the branch still descends directly from the exact base and contains no audit-candidate code.
2. Add the public scorecard harness without changing retrieval behavior. Run the literal baseline command from the Spec, bind the resolved model digest, and freeze the machine-readable baseline report. If main scores above 6/8 or the digest/profile cannot match, stop and re-shape because the required `+2` improvement is not achievable as signed.
3. Implement the smallest product delta:
   - keep the current WP-55/WP-56 discovery, connection, search, get, and feedback routes;
   - require each citation to pass query-only eligibility independently;
   - store an additive exact qualifying-source binding with the retrieval receipt;
   - hydrate only that current record source or current resource;
   - let scope, lens, and agent feedback reorder only the unchanged eligible set;
   - improve paraphrased recall through the existing exact-word and local-meaning ports, with no new provider, database, evaluator, or audit route.
4. Add focused tests for qualifying-resource hydration, hidden sibling/history, absent queries, fixed eligible membership, lens-specific ordering, context-isolated feedback, downgrade-safe old receipts, privacy sentinel, and the exact main→candidate→main→candidate upgrade lifecycle.
5. Run the candidate scorecard. It must meet every signed accuracy, abstention, source, isolation, digest, and latency gate before broader validation.
6. Freeze one candidate commit and run: full tests, vet, bounded race, branch-wide diff check, exact scanner, upgrade/rollback/roll-forward proof, audit-import/diff guard, and installed binary/source identity checks. No private proof or destination write is permitted.
7. Run the independent five-role review on that exact commit. One in-scope correction attempt is allowed. Every changed commit gets the same clean rereview; any remaining material blocker stops merge.
8. Push the exact reviewed commit, open a bounded PR, resolve only current-head material findings through the repository PR-review process, and merge only if the PR head tree equals the reviewed tree.
9. Pull exact merged `main`, build and install it, and record checksum/source commit. In two unrelated repositories, start two fresh non-Codex agents with no Mindline knowledge; each must discover the installed CLI, use only its distinct owner-approved opaque connection, choose its own question, and honestly record its answer or abstention plus citation/hydration outcome. Do not infer project binding from repository or working directory.
10. Reconcile Product Brain with the exact result, keep WP-53 at `building`/validated-staging, remove owned worktrees and branches, and hand Randy one minimal fresh non-Codex prompt. Only Randy's outside-agent result may mark WP-53 shipped.

## Rollback

If candidate validation, PR review, merge identity, installation, or either smoke test fails, keep or restore the previously released main binary and state. The additive qualifying-source representation may remain, but released main must ignore it and restart with identical evidence IDs, counts, and project connections. Do not delete or recreate personal evidence to recover.

## Claim boundary

Passing this plan proves a bounded local read-only beta on the public fixture and two owner-approved smoke connections. It does not prove broad retrieval quality, autonomous writes, destination readiness, automatic project inference, hostile same-user security, or the deferred audit-grade private proof.
