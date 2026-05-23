# MINDLINE-WP18-PLAN-V3 - LLM Semantic Candidate Review And Judging Harness

## Status

PR #13 remediation plan for LOOP sign-off against `MINDLINE-WP18-SPEC-V3`.

## Implementation Sequence

1. Add failing tests for the judgment model.
   - Initialize from a semantic-candidate fixture.
   - Assert summary, cursor, candidate page, and report paths.
   - Assert page includes candidate title, kind, confidence, summary, progress, evidence ranges, excerpts or unavailable reasons, and allowed choices.

2. Add failing tests for judgment recording.
   - Record `accept`, `reject`, `unclear`, `duplicate`, and `wrong-kind`.
   - Assert per-candidate judgment JSON, updated counts, precision estimate, review burden, and resume cursor behavior.
   - Assert duplicate judgment without overwrite fails closed.

3. Add failing tests for safety and containment.
   - Reject unknown candidate id.
   - Reject unsupported choice.
   - Reject path traversal and symlinked input/output parents.
   - Reject private/governance markers in committed fixture output.

4. Implement `internal/documents/semantic_judgment.go`.
   - Read and validate existing semantic run bundles.
   - Build ordered review candidates.
   - Build pages with source excerpts using the WP-16 excerpt policy.
   - Load existing judgments.
   - Apply one judgment and rebuild summary/report state.

5. Implement `internal/documents/semantic_judgment_writer.go`.
   - Write `semantic-judgment/` bundle.
   - Keep expected-file checks.
   - Update summary, cursor, per-candidate JSON, page Markdown, judgment JSON, and report Markdown.

6. Wire CLI commands in `internal/cli/runner.go`.
   - `documents judge`
   - `documents judge-next`
   - `documents judge-record`
   - Parse `--source-root`, `--source`, `--candidate`, `--choice`, `--note`, and `--reviewer`.

7. Add failing tests for the local review UI.
   - Start from a judgment bundle and call the UI handler/API.
   - Assert API state includes aggregate context: total, judged, remaining, accepted/rejected/unclear/duplicate/wrong-kind counts, run id, and completion state.
   - Assert API state includes exactly one current candidate body.
   - Assert UI HTML contains the operational shell: progress, remaining work, current candidate area, evidence area, note field, and decision controls.
   - Assert posting a valid choice persists the same judgment JSON/report/cursor model as `judge-record`.
   - Assert invalid choices and unknown candidates fail closed.
   - Assert default loopback serving is accepted and non-loopback bind attempts fail closed, including `0.0.0.0` and `::`.

8. Implement `documents judge-serve`.
   - Add a local-only HTTP server command in the CLI.
   - Reject non-loopback bind hosts before listening.
   - Serve a self-contained HTML/CSS/JS review UI with no external assets.
   - Serve API state from the existing semantic-judgment bundle.
   - Persist posted judgments through `documents.RecordSemanticJudgment`.
   - Advance to the next unjudged candidate after submit and show final completion state at exhaustion.

9. Run focused tests.
   - `go test ./internal/documents -run 'SemanticJudgment|SemanticAcceptance|SemanticCalibration' -count=1`
   - `go test ./internal/cli -run 'DocumentsJudge|DocumentsCalibrate|DocumentsAccept' -count=1`

10. Run full verification.
   - `go test -count=1 ./...`

11. Run real temp corpus verification.
   - For every direct `temp/*.md`, run `documents semantics` with LLM mode when the local OpenAI config is available.
   - Initialize `documents judge` for each candidate-producing semantic run.
   - Page at least the first candidate with `documents judge-next`.
   - Smoke the local UI/API for candidate-producing runs to prove one current item plus aggregate context.
   - Record one synthetic local verification judgment in `/private/tmp` only, then verify report updates.
   - Count blocked/skipped/no-candidate runs separately.
   - Do not commit generated temp outputs or judgments.

12. Browser/runtime verification.
   - Start `documents judge-serve` against a committed fixture-derived judgment bundle.
   - Open the local UI and verify the screen shows one candidate, remaining work, progress, and batch context.
   - Submit a smoke judgment and verify the UI advances or completes.
   - Stop the local server before final response.

13. Review and judge.
   - Run spec/authority implementation review.
   - Run quality/integration implementation review.
   - Run three independent zero-context LLM judge reviews over the final diff/evidence packet.
   - Judge packets must exclude real temp source text, temp-derived excerpts, raw provider responses, and temp-derived judgment artifacts; use sanitized fixtures plus aggregate temp counts/results only.
   - Fix blockers and rerun affected reviews.

14. Close Chain and PR.
   - Update `WP-18` with delivered proof or blocker truth.
   - Capture a decision with verification evidence, temp aggregate result, and judge sign-off.
   - Commit and push branch.
   - Update the existing ready-for-review PR #13 only after tests, temp verification, UI verification, and three judge reviews pass.

## Expected Files

- `.productbrain/specs/2026-05-23-wp-18-llm-semantic-candidate-review-judging-harness.md`
- `.productbrain/plans/2026-05-23-wp-18-llm-semantic-candidate-review-judging-harness-plan.md`
- `internal/documents/types.go`
- `internal/documents/semantic_judgment.go`
- `internal/documents/semantic_judgment_writer.go`
- `internal/documents/documents_test.go`
- `internal/cli/runner.go`
- `internal/cli/semantic_judgment_ui.go`
- `internal/cli/documents_decompose_test.go`

## Stop Conditions

Stop and capture a blocker if:

- the semantic run format cannot provide enough candidate evidence for a self-contained page;
- the judgment artifact cannot be made deterministic and bundle-contained;
- the local UI requires separate persistence or external assets;
- the UI cannot preserve one visible candidate while still showing batch context;
- temp verification exposes private/governance marker leakage in committed surfaces;
- any final judge returns a blocker that cannot be fixed within WP-18 scope.
