# MINDLINE-NEXT-PLAN-V2: Semantic Candidate Evidence Model

Date: 2026-05-22
Status: Draft for LOOP Plan review
Spec: `.productbrain/specs/2026-05-22-semantic-candidate-evidence-model.md`
Stop mode: Plan Ready before final work-package creation

## Delivery Sequence

1. Reconcile chain lifecycle before implementation.
   - Confirm PR #7 merge state is represented by `DEC-47`.
   - Confirm whether `WP-11`/`WP-12` should be marked shipped or superseded before creating the final next work package.
   - Do not create the final next work package until this spec and plan are signed off by the LOOP panel and the human owner.

2. Add synthetic semantic fixtures.
   - `testdata/documents/semantic/transcript-consolidated-action.md`
   - `testdata/documents/semantic/transcript-contradicted-observation.md`
   - `testdata/documents/semantic/process-capability-evidence.md`
   - Fixtures must use synthetic roles and organizations only.
   - Fixtures must include structure patterns already supported by WP-11/WP-12.

3. Add failing schema and writer tests.
   - Define summary, observation, candidate, and relation schemas.
   - Assert deterministic IDs and paths.
   - Assert candidate summaries rebuild from finalized artifacts.
   - Assert duplicate IDs, stale unexpected generated files, traversal, and symlink parent escapes are rejected.
   - Assert unsafe/private markers are blocked or redacted before summary and previews are written.

4. Add semantic types and artifact model.
   - Add constants and structs for:
     - semantic candidate summary
     - semantic observation
     - semantic candidate
     - semantic relation
   - Keep this model inside `internal/documents` or a destination-neutral package.
   - Do not import `internal/productbrain` or destination packages.

5. Add deterministic observation extraction.
   - Read `document-structure/` artifacts or produce persisted structure under the same `--out` root when given a Markdown file or Markdown directory.
   - Extract transcript observations from explicit question, action, owner, deadline, recap, and decision lexical signals.
   - Extract process/capability observations from capability nodes, requirement-like phrases, dependency phrases, and risk phrases.
   - Mark weak observations `needs_review`.

6. Add relationship extraction.
   - Generate `derived_from` relations from candidates to observations.
   - Generate same-topic/refines/answers/supersedes/contradicts relationships only from explicit lexical or proximity evidence.
   - Preserve evidence node IDs and line ranges.
   - Keep all ambiguous relationships `needs_review`.

7. Add deterministic consolidation.
   - Consolidate observations into candidates only when enough evidence links exist.
   - Require all consolidated candidates to include observation IDs, relation IDs, evidence nodes, and evidence ranges.
   - Default inferred transcript candidates to `needs_review` unless the fixture proves a narrow ready case.
   - Always set `destination_status` to `unresolved`.

8. Add CLI command.
   - Add `mindline documents semantics <structure-run-dir-or-markdown-path-or-markdown-dir> --out <dir>`.
   - If input is a Markdown file or Markdown directory, persist or reuse `document-structure/` under the same `--out` root before writing semantic artifacts.
   - If input is an existing structure run directory, read it without rerunning Markdown parsing and write semantic artifacts under the explicit `--out` root.
   - Support Markdown directory input with lexical deterministic traversal matching `mindline documents structure`.
   - Ensure semantic artifacts never reference structure node IDs that are not inspectable in the same output bundle or explicitly supplied persisted structure run.
   - Print `semantic-summary.json` to stdout.
   - Reject destination/profile/proposal flags.

9. Add golden outputs and deterministic diff verification.
   - Generate expected semantic artifacts under `testdata/documents/expected/semantic/`.
   - Add tests comparing summary, observations, candidates, relations, and previews.
   - Add repeated-run tests proving deterministic output.

10. Update ignored private benchmark comparisons.
   - Generate semantic output for the ignored private process-document benchmark.
   - Generate semantic output for the ignored private transcript benchmark.
   - Update ignored comparison docs to show:
     - structure-only output;
     - semantic observations;
     - consolidated candidates;
     - evidence relationships;
     - remaining `needs_review` items.
   - Keep all private benchmark artifacts ignored.

11. Verify.
   - `go test -count=1 ./...`
   - semantic CLI generation against synthetic fixtures.
   - explicit check that Markdown directory input writes inspectable `document-structure/` alongside `semantic-candidates/`.
   - deterministic rerun/diff.
   - leakage scans for destination hints, live integration hooks, Chain governance IDs, private markers, unsafe marker leakage, and `: null`.
   - `git status --short --ignored temp`.

12. LOOP review.
   - Chain Steward.
   - Domain/User Job Reviewer.
   - Systems Architect.
   - Delivery Quality Reviewer.
   - Risk/Safety Reviewer.
   - Rerun full panel if any blocker changes shape, spec, plan, acceptance, exclusions, or authority.

13. After human-owner sign-off only.
   - Capture signed spec/plan proof on Chain.
   - Materialize the final next work package from the signed spec only.
   - Run Product Brain audit on the materialized work package.
   - Reconcile audit without changing signed meaning; if meaning changes, rerun LOOP sign-off.

## Expected Implementation Files

- `internal/documents/types.go`
- `internal/documents/semantic.go`
- `internal/documents/semantic_writer.go`
- `internal/documents/documents_test.go`
- `internal/cli/runner.go`
- `internal/cli/documents_decompose_test.go`
- `testdata/documents/semantic/*`
- `testdata/documents/expected/semantic/*`

Spec and plan files may change only during pre-signoff iteration. Any post-signoff change to meaning, scope, acceptance, exclusions, or authority requires a new LOOP review.

Ignored evidence, not committed:

- `temp/*semantic*`
- `/tmp/mindline-semantic-*`

## Exclusions

- No Product Brain proposal generation.
- No live writes.
- No workspace profile application.
- No destination adapter changes.
- No LLM/provider integration in this slice.
- No committed private benchmark source material or raw-derived private output.
- No final next work package creation before LOOP and human-owner sign-off.
