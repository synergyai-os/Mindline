# WP-47 Semantic Density Readiness Plan

## Sequence

1. Add corpus-pressure semantic-density fields and source-level counters.
2. Read segment-summary metadata per processed source and aggregate segment counts without storing source text.
3. Compute semantic-readiness status/reason codes in corpus pressure.
4. Project semantic-density counters into corpus-pressure eval input and trace summaries.
5. Extend eval readback artifact support for document segment summaries and semantic-readiness metrics.
6. Add readback summary `semantic_readiness`, claim gate, report, chain draft, and top-target behavior.
7. Extend proof-gate improvement behavior through the existing readback claim gate.
8. Add tests:
   - corpus pressure blocks reference-only one-candidate-per-source collapse;
   - readback reconstructs collapse from old-style pressure summary plus segment summaries;
   - readback passes richer semantic density;
   - proof-gate improvement blocks semantic-readiness collapse.
9. Run focused package tests.
10. Run full `go test ./...` with local `GOCACHE`.
11. Run runtime readback/proof over `/private/tmp/mindline-wp46-real/mixed-pressure` as the baseline comparison dataset.
12. Leak-scan generated runtime artifacts and staged changes.
13. Capture delivery evidence on Product Brain.
14. Commit, push, and open draft PR.

## Verification Commands

```sh
GOCACHE="$PWD/.cache/go-build" go test ./internal/documents ./internal/evalreadback ./internal/evalproof ./internal/cli
GOCACHE="$PWD/.cache/go-build" go test ./...
go run ./cmd/mindline eval readback /private/tmp/mindline-wp46-real/mixed-pressure --out /private/tmp/mindline-wp47-pr41-readback
go run ./cmd/mindline eval proof-gate /private/tmp/mindline-wp46-real/mixed-pressure --claim improvement --baseline /private/tmp/mindline-wp46-real/mixed-pressure --out /private/tmp/mindline-wp47-pr41-proof
git diff --check
```

Expected runtime outcome: readback and proof outputs are privacy-safe, the improvement proof is blocked, and the reason includes semantic-readiness collapse.

