# WP-40 Plan - Real Private Held-Out Labeling Packet

## Sequence

1. Preserve the existing corpus acceptance gate.
   - Do not change `not_generated_from_evaluated_run` as the only independent provenance value.
   - Add tests showing generated templates remain blocked for held-out acceptance.

2. Add labeling packet domain types.
   - `corpus-acceptance-labeling-packet/v0.1`.
   - Source summaries from corpus-pressure.
   - Candidate references from semantic artifacts.
   - Relation coverage summary from corpus graph artifacts.
   - Guardrail counters copied from pressure summary.
   - Deterministic redacted case IDs for report surfaces.
   - Explicit `labeling_required`, `held_out_ready=false`, and claim boundary fields.

3. Add the builder/writer.
   - Input: corpus-pressure output or its parent.
   - Output directory: `corpus-acceptance-labeling/`.
   - Artifacts: `labeling-packet.json`, `answer-key-template.json`, `labeling-report.md`.
   - Reject symlink escapes and unexpected existing files using existing artifact safety patterns.
   - Keep report content PR-safe by excluding source IDs, raw URLs, Slack identifiers, and private paths.
   - Treat possible-duplicate relation counts as labeling coverage, not an extractor-quality defect claim.

4. Add the CLI command.
   - Usage: `mindline documents corpus-acceptance-labeling <corpus-pressure-out-or-parent> --out <dir>`.
   - Return JSON summary to stdout.
   - Refuse destination, provider, profile, and classifier flags.

5. Verify with tests and real private data.
   - Focused document tests.
   - Focused CLI tests.
   - Hard negative test showing generated answer-key template fails held-out acceptance with `answer_key_not_independent`.
   - Full `go test ./...`.
   - Real Slack runtime command against the private pressure run.
   - Readback/proof gate as applicable, with claims limited to local labeling readiness and safety.

6. Capture Product Brain truth and open PR.
   - Record the decision, outcome, claim boundaries, real evidence, blockers, and next work.
   - Push branch and open PR.

## Risk Controls

- The answer-key template must use non-independent provenance so it cannot pass held-out gates by accident.
- The local JSON packet may be operator-facing, but the markdown report must not leak private raw content.
- Source IDs and hashes are allowed as local methodology references, but final PR/explanation must avoid raw Slack channel IDs and message URLs.
- Generalization remains blocked until independent held-out labels exist across enough representative sources.

## Implementation Notes

- Prefer reusing `readCorpusAcceptancePressureSummary` and `containedCorpusAcceptancePath`.
- Reuse `readSemanticAcceptanceInput` to inspect candidate references.
- Reuse `readCorpusAcceptanceGraphSummary` for relation counts.
- Keep source ordering stable by source ID.
- Add schema constants near existing corpus acceptance types.
