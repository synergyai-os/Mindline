# WP-43 Implementation Plan

Date: 2026-06-03
Status: signed for implementation
Spec: `.productbrain/specs/2026-06-03-wp-43-real-corpus-semantic-fallback-normalization.md`

## Sequence

1. Persist signed spec and plan.
2. Capture or link signed authority in Product Brain.
3. Create branch `codex/wp43-real-corpus-fallback-normalization`.
4. Add RED tests for segment finalization:
   - unknown-ready segment becomes `needs_review`;
   - low-confidence-ready segment becomes `needs_review`;
   - ready segment missing title becomes `needs_review`;
   - ready segment missing summary becomes `needs_review`;
   - unsafe marker precedence remains blocked/redacted.
5. Add RED tests for structure finalization:
   - unknown-ready node becomes `needs_review`;
   - low-confidence-ready node becomes `needs_review`;
   - ready node missing title becomes `needs_review`;
   - ready node missing summary becomes `needs_review`;
   - unsafe marker precedence remains blocked/redacted;
   - child/path rebuild stays valid.
6. Add RED writer/summary tests proving finalized `needs_review` artifacts are counted.
7. Add RED synthetic corpus-pressure regression proving fallback classes no longer become source-level semantic write blockers and review burden remains visible.
8. Implement the smallest finalization change in `internal/documents`.
9. Run targeted tests until green.
10. Run full verification:
    - `go test ./internal/documents`
    - `go test ./...`
    - `git diff --check`
11. Rerun private runtime proof:
    - `go run ./cmd/mindline documents corpus-pressure temp/pb-docs --out /private/tmp/mindline-wp43-pb-docs-current`
    - compare `pressure-summary.json` against baseline counts;
    - scan output for ready unknown/empty fallback states.
12. Run readback and gates:
    - `go run ./cmd/mindline eval readback /private/tmp/mindline-wp43-pb-docs-current --out /private/tmp/mindline-wp43-pb-docs-current-readback --baseline /private/tmp/mindline-wp43-pb-docs-readback`
    - `go run ./cmd/mindline eval proof-gate /private/tmp/mindline-wp43-pb-docs-current-readback --out /private/tmp/mindline-wp43-pb-docs-current-safety-gate --claim safety`
    - `go run ./cmd/mindline eval proof-gate /private/tmp/mindline-wp43-pb-docs-current-readback --out /private/tmp/mindline-wp43-pb-docs-current-improvement-gate --claim improvement --baseline /private/tmp/mindline-wp43-pb-docs-readback`
13. Run privacy checks:
    - `git status -sb`
    - inspect staged diff;
    - confirm no private runtime artifacts or source bodies are committed.
14. Run LOOP delivery review panel against the final branch output.
15. Capture delivery truth in Product Brain.
16. Commit, push, and open PR.

## Implementation Notes

The preferred implementation point is finalization before validation:

- `internal/documents/writer.go` for segments;
- `internal/documents/structure.go` for structure nodes.

Do not weaken validators. The validators remain the authority for rejecting invalid ready artifacts.

Do not create placeholder ready titles or summaries. If the artifact lacks required ready fields, demote it to `needs_review` and attach an explicit blocker. Preserve evidence and provenance instead of inventing it.

Unsafe marker redaction remains higher priority than fallback demotion.

## Acceptance Evidence

Delivery cannot close until:

- tests prove the observed fallback classes;
- private rerun shows `blocked_source_count == 0`;
- private rerun keeps all named side-effect guardrails at zero;
- ready unknown/empty scans return zero;
- readback compares against baseline and remains `private_runtime` / `non_generalizable`;
- no private runtime artifacts or source bodies are staged or committed.
