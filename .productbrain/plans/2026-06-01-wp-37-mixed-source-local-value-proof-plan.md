# WP-37 Mixed-Source Local Value Proof Plan

## Build Strategy

Implement WP-37 as a thin local orchestration over existing corpus and eval systems. Do not add a destination adapter, hosted dependency, auth, DB, or no-human claim.

## Workstreams

1. Fixture and contract
   - Add a reusable mixed-source fixture manifest under `testdata/documents/value-proof/`.
   - Cover markdown/source-note, transcript, Notion/process/capability, and Slack-intake style source shapes.
   - Expected validation: command can process the manifest without fixture-name special casing.

2. Value proof model and writer
   - Add a `documents` value proof package/file that builds a summary from existing corpus-pressure, corpus-graph, and meaning-preview outputs.
   - Emit local private packet with evidence excerpts and source-relative refs.
   - Emit PR-safe summary with counters, hashed/redacted refs where needed, no raw private text, and claim status.
   - Expected validation: focused unit test asserts source accounting, evidence coverage, corpus-graph-backed relation state, and PR-safe fields.

3. CLI integration
   - Add `mindline documents value-proof <markdown-dir-or-manifest> --out <dir> [--classifier deterministic|llm --llm-provider openai --llm-model <model>]`.
   - Default must be deterministic/local-only with zero hosted or destination side effects.
   - Expected validation: focused CLI test covers usage, output files, and summary JSON.

4. Eval/readback/proof integration
   - Make the value-proof run directory contain or reference supported artifacts so existing `eval readback` and `eval proof-gate --claim safety` can inspect it.
   - Ensure corpus-graph artifacts are written under the run directory and referenced from the value-proof summary.
   - Expected validation: runtime readback and safety proof-gate pass on the committed fixture.

5. Verification and review
   - Run focused tests, `go test ./...`, `git diff --check`, fixture runtime proof, PR-safe leak scan, PB audit, and LOOP reviewer sign-off.
   - Use private `/temp` or Slack runtime data only as optional local debugging evidence; do not commit private artifacts or generalize from them.

## Acceptance Proof Commands

Use a temporary output root for runtime proof:

```sh
go run ./cmd/mindline documents value-proof testdata/documents/value-proof/corpus-pressure-manifest.json --out /private/tmp/mindline-wp37-value-proof
go run ./cmd/mindline eval readback /private/tmp/mindline-wp37-value-proof --out /private/tmp/mindline-wp37-readback
go run ./cmd/mindline eval proof-gate /private/tmp/mindline-wp37-value-proof --claim safety --out /private/tmp/mindline-wp37-proof
go test ./...
git diff --check
```

## Done When

1. WP-37 Chain entry is captured, linked to governing entries, and PB audit is pass or only explicitly reconciled warnings.
2. The value-proof command produces the local packet and PR-safe proof summary.
3. Source accounting is 100% for the committed mixed fixture.
4. Candidate/atom evidence coverage is 100% evidence excerpt or explicit blocker.
5. Relation context is backed by a corpus graph summary, not inferred text.
6. Default readback side-effect counters are zero for hosted calls, Slack API calls, browser calls, destination writes, Product Brain writes, Tolaria writes, and auto-accepts.
7. Safety proof-gate passes for the fixture run.
8. Improvement/generalization/DEC-64 claims remain blocked/not evaluated unless valid baseline or held-out evidence is present.
9. Tests, runtime proof, leak scan, PB audit, and LOOP review pass.
