# WP-48 Segment-Level Semantic Atomization Plan

## Sequence

1. Inspect current semantic extraction, segment summaries, candidate consolidation, and PR42 readiness counters.
2. Add a focused fixture/test that reproduces source-level collapse with several meaningful source segments.
3. Add segment-derived observation extraction using existing segment summaries and inspectable structure-node evidence.
4. Expand deterministic observation kind detection with source-neutral explicit-pattern rules.
5. Adjust consolidation so distinct non-reference atoms can become distinct candidates, while keeping reference fallback source-level and fallback-only.
6. Update or add tests for:
   - multiple segment atoms from one source;
   - fallback reference only when no stronger atom exists;
   - destination status remains unresolved;
   - unsafe/private marker blocking still propagates;
   - corpus-pressure semantic readiness no longer sees a rich fixture as reference-only collapse.
7. Run focused tests:
   - `GOCACHE="$PWD/.cache/go-build" go test ./internal/documents ./internal/evalreadback ./internal/evalproof`
8. Run full tests:
   - `GOCACHE="$PWD/.cache/go-build" go test ./...`
9. Confirm comparable runtime paths without reading or printing private source bodies:
   - source input: `/private/tmp/mindline-wp46-real/mixed-corpus/corpus-pressure-manifest.json`
   - PR42 baseline output: `/private/tmp/mindline-wp46-real/mixed-pressure`
   - WP48 output: `/private/tmp/mindline-wp48-segment-atomization`
10. Re-run corpus pressure on the same PR41/PR42 source manifest with WP48 logic into the new private `/private/tmp` output.
11. Run readback/proof over the new output with PR42 output as baseline.
12. Extract and assert the six measurable runtime KRs from the generated readback/proof artifacts with a command that exits non-zero when any threshold fails:
   - `semantic_observation_count >= 125`;
   - `semantic_candidate_count >= 100`;
   - `reference_candidate_ratio < 0.50`;
   - `reference_only_source_count < 25`;
   - `reference_only_one_candidate_per_source` is absent from semantic-readiness reason codes;
   - `review_burden_ratio <= 0.60`.
13. Stage the implementation changes intended for commit, then leak-scan the full generated readback/proof packet directories, staged implementation diff, staged filenames, the exact saved Product Brain capture text, and the exact saved PR body file that will be used to create the PR. Pass only if the scan finds no raw source body excerpts, secret-looking key/value assignments, bearer tokens, session cookies, private email addresses, Slack channel/user tokens, or committed `/private/tmp` artifacts. Policy/spec/plan files may contain guardrail words such as "password" without being treated as leaks. The primary corpus-pressure output may contain private local source copies under `/private/tmp`; treat it as private runtime input/output, not a published proof artifact.
14. Capture delivery evidence and remaining limits on Product Brain using the already scanned capture text file, limited to aggregate metrics, paths, command outcomes, and claim boundaries.
15. Commit, push, and open a draft PR titled `WP48 Segment-level semantic atomization` using the already scanned PR body file via `--body-file /private/tmp/mindline-wp48-pr-body.md` or an equivalent connector call that uses that exact file content verbatim.

## Verification Commands

```sh
GOCACHE="$PWD/.cache/go-build" go test ./internal/documents ./internal/evalreadback ./internal/evalproof
GOCACHE="$PWD/.cache/go-build" go test ./...
test -f /private/tmp/mindline-wp46-real/mixed-corpus/corpus-pressure-manifest.json
test -d /private/tmp/mindline-wp46-real/mixed-pressure
go run ./cmd/mindline corpus pressure /private/tmp/mindline-wp46-real/mixed-corpus/corpus-pressure-manifest.json --out /private/tmp/mindline-wp48-segment-atomization
go run ./cmd/mindline eval readback /private/tmp/mindline-wp48-segment-atomization --baseline /private/tmp/mindline-wp46-real/mixed-pressure --out /private/tmp/mindline-wp48-readback
go run ./cmd/mindline eval proof-gate /private/tmp/mindline-wp48-segment-atomization --claim improvement --baseline /private/tmp/mindline-wp46-real/mixed-pressure --out /private/tmp/mindline-wp48-proof
jq -e '(.semantic_readiness.semantic_observation_count >= 125) and (.semantic_readiness.semantic_candidate_count >= 100) and (.semantic_readiness.reference_candidate_ratio < 0.50) and (.semantic_readiness.reference_only_source_count < 25) and ((.semantic_readiness.reason_codes // []) | index("reference_only_one_candidate_per_source") | not) and ((([.artifacts[] | select(.type == "corpus_pressure_summary") | .metrics.review_burden_ratio][0]) // 999) <= 0.60)' /private/tmp/mindline-wp48-readback/eval-readback/readback-summary.json
git add .productbrain/specs/2026-06-04-wp-48-segment-level-semantic-atomization.md .productbrain/plans/2026-06-04-wp-48-segment-level-semantic-atomization-plan.md internal testdata
git diff --cached > /private/tmp/mindline-wp48-staged.diff
git diff --cached --name-only > /private/tmp/mindline-wp48-staged-files.txt
find /private/tmp/mindline-wp46-real/mixed-corpus/sources -path '*/source.md' -type f -print0 | xargs -0 awk 'length($0) >= 24 && $0 !~ /^#/ && $0 !~ /^(Source kind|Source id|Source label|Captured at|Author|Timestamp|Permalink|Thread|Files|URLs):/ && $0 !~ /^[[:space:][:punct:]]*$/ {print}' | sort -u > /private/tmp/mindline-wp48-private-excerpt-denylist.txt
test -s /private/tmp/mindline-wp48-private-excerpt-denylist.txt
test -s /private/tmp/mindline-wp48-chain-capture.md
test -s /private/tmp/mindline-wp48-pr-body.md
! rg -n "sk-[A-Za-z0-9_-]+|Bearer [A-Za-z0-9._-]+|(password|passwd|api[_-]?key|session[_-]?cookie)\\s*[:=]|xox[baprs]-[A-Za-z0-9-]+|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}" /private/tmp/mindline-wp48-readback/eval-readback /private/tmp/mindline-wp48-proof/eval-proof /private/tmp/mindline-wp48-staged.diff /private/tmp/mindline-wp48-chain-capture.md /private/tmp/mindline-wp48-pr-body.md
! rg -F -n -f /private/tmp/mindline-wp48-private-excerpt-denylist.txt /private/tmp/mindline-wp48-readback/eval-readback /private/tmp/mindline-wp48-proof/eval-proof /private/tmp/mindline-wp48-staged.diff /private/tmp/mindline-wp48-chain-capture.md /private/tmp/mindline-wp48-pr-body.md
! rg -n "/private/tmp|^private/tmp/|^tmp/|mindline-wp48-(readback|proof|segment-atomization)" /private/tmp/mindline-wp48-staged-files.txt
git diff --check
pb capture -c decisions -n "WP48 segment-level semantic atomization delivery proof" -d "$(cat /private/tmp/mindline-wp48-chain-capture.md)"
gh pr create --draft --title "WP48 Segment-level semantic atomization" --body-file /private/tmp/mindline-wp48-pr-body.md
```

## Expected Runtime Outcome

The same private-local source set produces more evidence-backed non-reference observations/candidates than PR42, breaks the exact reference-only one-candidate-per-source failure mode, and remains blocked from any generalization, DEC-64, no-human, or destination-write claim.
