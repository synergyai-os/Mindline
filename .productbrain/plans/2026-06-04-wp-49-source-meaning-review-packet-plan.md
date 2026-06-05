# WP-49 Source Meaning Review Packet Plan

## Sequence

1. Capture the PR43 learning on Chain before implementation (`INS-24`).
2. Baseline current meaning preview on the same PR43 private-local output and record aggregate-only metrics: 50 sources, 208 atoms, 20,926 relations.
3. Add source meaning packet types and builder in `internal/documents`, reusing existing corpus pressure and corpus graph readers.
4. Group atoms deterministically by route, candidate kind, relation-connected review context, and bounded batch size.
5. Write packet artifacts under `source-meaning-packet/`:
   - summary JSON;
   - human review Markdown;
   - group JSON files;
   - destination-neutral proposal JSON files;
   - evidence map;
   - blocked items.
6. Add CLI command `documents meaning-packet`.
7. Add readback support for `source-meaning-packet/meaning-summary.json`.
8. Add tests for:
   - compression from many atoms to bounded groups;
   - no raw excerpts in packet summaries, groups, proposals, evidence map, blocked items, or review-packet Markdown;
   - evidence refs or blockers on every group;
   - write guardrails remain zero;
   - CLI command writes packet output;
   - readback detects packet metrics.
9. Run focused tests:
   - `GOCACHE="$PWD/.cache/go-build" go test ./internal/documents ./internal/evalreadback ./internal/cli`
10. Run full tests:
   - `GOCACHE="$PWD/.cache/go-build" go test ./...`
11. Re-run corpus pressure on the same private-local source manifest into `/private/tmp/mindline-wp49-source-meaning-packet`.
12. Run `documents meaning-packet` on the WP49 pressure output into the same private-local output root.
13. Run readback and improvement proof using PR43 output as baseline.
14. Assert runtime KRs from readback and packet summary:
   - `review_group_count` between `5` and `25`;
   - `atom_compression_ratio >= 0.85`;
   - `relation_review_compression_ratio >= 0.95`;
   - `evidence_or_blocker_group_ratio == 1`;
   - `review_burden_ratio <= 0.35`;
   - readback contains `source_meaning_packet_summary`.
15. Stage intended files, then leak-scan generated source-meaning packet artifacts, generated readback/proof directories, staged diff, staged filenames, exact Chain capture text, and exact PR body text.
16. Capture delivery proof on Chain using aggregate metrics and claim boundaries only.
17. Commit the staged work, push `codex/wp49-source-meaning-review-packet`, and open draft PR `WP49 Source meaning review packet`.

## Verification Commands

```sh
GOCACHE="$PWD/.cache/go-build" go test ./internal/documents ./internal/evalreadback ./internal/cli
GOCACHE="$PWD/.cache/go-build" go test ./...
test -f /private/tmp/mindline-wp46-real/mixed-corpus/corpus-pressure-manifest.json
test -d /private/tmp/mindline-wp48-segment-atomization-v2
go run ./cmd/mindline documents corpus-pressure /private/tmp/mindline-wp46-real/mixed-corpus/corpus-pressure-manifest.json --out /private/tmp/mindline-wp49-source-meaning-packet
go run ./cmd/mindline documents meaning-packet /private/tmp/mindline-wp49-source-meaning-packet --out /private/tmp/mindline-wp49-source-meaning-packet
go run ./cmd/mindline eval readback /private/tmp/mindline-wp49-source-meaning-packet --baseline /private/tmp/mindline-wp48-segment-atomization-v2 --out /private/tmp/mindline-wp49-readback
go run ./cmd/mindline eval proof-gate /private/tmp/mindline-wp49-source-meaning-packet --claim improvement --baseline /private/tmp/mindline-wp48-segment-atomization-v2 --out /private/tmp/mindline-wp49-proof
jq -e '(.artifact_type_counts.source_meaning_packet_summary == 1) and (([.artifacts[] | select(.type == "source_meaning_packet_summary") | .metrics.review_group_count][0]) >= 5) and (([.artifacts[] | select(.type == "source_meaning_packet_summary") | .metrics.review_group_count][0]) <= 25) and (([.artifacts[] | select(.type == "source_meaning_packet_summary") | .metrics.atom_compression_ratio][0]) >= 0.85) and (([.artifacts[] | select(.type == "source_meaning_packet_summary") | .metrics.relation_review_compression_ratio][0]) >= 0.95) and (([.artifacts[] | select(.type == "source_meaning_packet_summary") | .metrics.evidence_or_blocker_group_ratio][0]) == 1) and (([.artifacts[] | select(.type == "source_meaning_packet_summary") | .metrics.review_burden_ratio][0]) <= 0.35)' /private/tmp/mindline-wp49-readback/eval-readback/readback-summary.json
git add .productbrain/specs/2026-06-04-wp-49-source-meaning-review-packet.md .productbrain/plans/2026-06-04-wp-49-source-meaning-review-packet-plan.md internal testdata
git diff --cached > /private/tmp/mindline-wp49-staged.diff
git diff --cached --name-only > /private/tmp/mindline-wp49-staged-files.txt
find /private/tmp/mindline-wp46-real/mixed-corpus/sources -path '*/source.md' -type f -print0 | xargs -0 awk 'length($0) >= 24 && $0 !~ /^#/ && $0 !~ /^(Source kind|Source id|Source label|Captured at|Author|Timestamp|Permalink|Thread|Files|URLs):/ && $0 !~ /^[[:space:][:punct:]]*$/ {print}' | sort -u > /private/tmp/mindline-wp49-private-excerpt-denylist.txt
test -s /private/tmp/mindline-wp49-private-excerpt-denylist.txt
test -s /private/tmp/mindline-wp49-chain-capture.md
test -s /private/tmp/mindline-wp49-pr-body.md
! rg -n "sk-[A-Za-z0-9_-]+|Bearer [A-Za-z0-9._-]+|(password|passwd|api[_-]?key|session[_-]?cookie)\\s*[:=]|xox[baprs]-[A-Za-z0-9-]+|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}" /private/tmp/mindline-wp49-readback/eval-readback /private/tmp/mindline-wp49-proof/eval-proof /private/tmp/mindline-wp49-staged.diff /private/tmp/mindline-wp49-chain-capture.md /private/tmp/mindline-wp49-pr-body.md
! rg -n "sk-[A-Za-z0-9_-]+|Bearer [A-Za-z0-9._-]+|(password|passwd|api[_-]?key|session[_-]?cookie)\\s*[:=]|xox[baprs]-[A-Za-z0-9-]+|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}" /private/tmp/mindline-wp49-source-meaning-packet/source-meaning-packet /private/tmp/mindline-wp49-readback/eval-readback /private/tmp/mindline-wp49-proof/eval-proof /private/tmp/mindline-wp49-staged.diff /private/tmp/mindline-wp49-chain-capture.md /private/tmp/mindline-wp49-pr-body.md
! rg -F -n -f /private/tmp/mindline-wp49-private-excerpt-denylist.txt /private/tmp/mindline-wp49-source-meaning-packet/source-meaning-packet /private/tmp/mindline-wp49-readback/eval-readback /private/tmp/mindline-wp49-proof/eval-proof /private/tmp/mindline-wp49-staged.diff /private/tmp/mindline-wp49-chain-capture.md /private/tmp/mindline-wp49-pr-body.md
! rg -n "/private/tmp|^private/tmp/|^tmp/|mindline-wp49-(readback|proof|source-meaning-packet)" /private/tmp/mindline-wp49-staged-files.txt
git diff --check
pb capture -c decisions -n "WP49 source meaning review packet delivery proof" -d "$(cat /private/tmp/mindline-wp49-chain-capture.md)"
git commit -m "Add source meaning review packet"
git push origin codex/wp49-source-meaning-review-packet
gh pr create --draft --title "WP49 Source meaning review packet" --body-file /private/tmp/mindline-wp49-pr-body.md
```

## Expected Runtime Outcome

The same 50-item private-local corpus still proves semantic atomization from PR43, then adds a reviewable output layer: roughly 5-25 source meaning groups with complete evidence references, zero write side effects, measurable review compression, and explicit blocks on generalization, DEC-64, no-human, and destination-write claims.
