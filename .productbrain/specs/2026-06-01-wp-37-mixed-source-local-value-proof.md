# WP-37 Mixed-Source Local Value Proof

## Status

Spec version: v1
LOOP phase: Spec
Stop mode: Full Delivery

## Outcome

Mindline can run a representative mixed local corpus and produce one inspectable value packet that answers, in plain evidence-backed terms: what was read, what Mindline understood, what candidates/atoms were produced, what evidence supports them, what relations or blockers exist, and what proof gates do or do not allow us to claim.

This moves the product from eval infrastructure toward visible local user value without crossing into destination writes, hosted storage, auth, DB, or no-human autonomy claims.

## Problem

PR #30 gave us a strict eval proof gate, but proof infrastructure is still too technical by itself. Randy cannot judge whether Mindline is creating user value from mixed real-world material unless the system emits a readable packet that connects sources to evidence-backed meaning and then states the proof status honestly.

The next slice must not optimize for one private `/temp` corpus, one Slack sample, one destination, or one provider. It must prove the reusable product behavior: source-neutral intake, evidence-backed semantic output, source accounting, local proof, and explicit claim boundaries.

## Scope

Build the smallest local source/destination/provider-neutral workflow needed to produce a mixed-source value proof packet.

Inputs must support a reusable committed fixture manifest that represents at least:

1. Markdown/source-note style content.
2. Transcript/meeting-turn style content.
3. Notion/process/capability style content.
4. Slack-intake style content, either through existing Slack corpus-intake output or committed Slack-shaped markdown fixture.

Outputs must include:

1. A local private value packet with evidence excerpts and source-relative refs.
2. A PR-safe proof summary that contains no private text, private paths, secrets, raw prompts, raw completions, or hosted trace payloads.
3. A corpus graph artifact produced through the existing corpus graph seam, so duplicate/contradiction/supersession/same-topic relation state is exercised rather than inferred.
4. Existing eval/readback/proof-gate artifacts, or a composed run directory that `mindline eval readback` and `mindline eval proof-gate --claim safety` can inspect.

## Product Model Fit

Layer ownership:

1. Source adapter layer: may normalize Slack-shaped input into source-neutral files/manifests.
2. Document/corpus layer: owns mixed-source processing, source accounting, semantic artifacts, corpus graph relation artifacts, and value packet generation.
3. Eval/readback/proof layer: owns safety/readback/proof-gate evidence.
4. Destination adapters: out of scope.

This is an extension of the existing local corpus/eval system, not a bespoke `/temp` remediation and not a Tolaria/Product Brain adapter.

## Constraints

1. Product Brain remains the source of truth for the work package and claim boundaries.
2. Default run must be deterministic and local-only.
3. No destination writes.
4. No hosted telemetry or hosted inference required by default.
5. LLM usage, if exposed, must remain optional and provider-agnostic behind existing semantic options.
6. Private runtime runs may be used for debugging/proof but must not be committed or used as generalization proof.
7. PR-safe artifacts must be metadata/redacted summaries only.
8. Improvement, generalization, and DEC-64/no-human claims remain blocked unless a comparable baseline and held-out answer-key evidence exist.

## Acceptance Criteria

1. `mindline documents value-proof <markdown-dir-or-manifest> --out <dir>` or an equivalently named local command produces a value-proof artifact bundle from a mixed local corpus.
2. The committed mixed fixture covers markdown/source-note, transcript, Notion/process/capability, and Slack-intake style inputs.
3. The value-proof summary accounts for 100% of manifest sources as processed, blocked, skipped, or failed with a reason.
4. Every surfaced candidate/atom in the local value packet has an inline evidence excerpt and source ref, or an explicit blocker explaining why it is not evidence-ready.
5. The value packet includes relation context sourced from the existing corpus graph seam: possible duplicates, contradictions, supersession, same-topic, or an explicit zero-relations state tied to a corpus graph summary.
6. The command emits a PR-safe proof summary with no private paths, raw source text, raw prompts, raw completions, secrets, or destination-specific write payloads.
7. `mindline eval readback <run> --out <dir>` can inspect the run and reports zero destination writes, Product Brain writes, Tolaria writes, hosted telemetry exports, hosted inference calls, browser calls, Slack API calls, and auto-accepts for the default fixture run.
8. `mindline eval proof-gate <run-or-readback> --claim safety --out <dir>` passes for the default fixture run.
9. Generalization, improvement, and DEC-64/no-human claims are not presented as passing unless the required baseline or held-out evidence is provided; absent that evidence, the packet states they are blocked/not evaluated.
10. Focused tests, full `go test ./...`, `git diff --check`, runtime fixture proof, PR-safe leak scan, Product Brain audit, and LOOP review pass.

## Exclusions

1. No auth/login.
2. No hosted database or permanent user workspace store.
3. No destination writes to Tolaria, Product Brain, Notion, Obsidian, Linear, local vaults, or APIs.
4. No new PostHog dependency or hosted trace requirement.
5. No new classifier tuning aimed at making the committed fixture look good.
6. No no-human approval, DEC-64, or >=98% accuracy claim.
7. No bulk 50-file or live Slack import claim.

## Risks

1. The packet becomes another technical report. Mitigation: include a human-readable value packet with source accounting, key findings, evidence, relations, and blockers in one place.
2. Relation context is faked by summary text. Mitigation: the command must run or consume corpus graph output and cite the graph summary for relation counts.
3. The implementation overfits the fixture. Mitigation: use the existing manifest/corpus contracts and mixed source types; do not special-case fixture names or contents.
4. Private data leaks into PR-safe evidence. Mitigation: separate local private packet from PR-safe proof summary and run a leak scan.
5. Command success is confused with outcome success. Mitigation: require readback and safety proof-gate evidence; state blocked claims explicitly.
6. Scope drifts toward DB/auth/destination writes. Mitigation: keep WP-37 local proof only and capture later roadmap work separately.

## LOOP Sign-Off

Diagnose/Shape panel:

1. Chain Steward: SIGN-OFF. Required wording: representative corpus must be reusable fixtures or explicitly non-generalizable private samples; require redacted PR-safe packet; block improvement/generalization/DEC-64 claims without baseline/held-out proof.
2. Domain/User Job Reviewer: SIGN-OFF. Required wording: packet must answer what was read, what was understood, why it matters, supporting evidence, relations, and failures.
3. Systems Architect: SIGN-OFF. Required wording: implement as thin orchestration over existing corpus-pressure, corpus-graph, meaning-preview, readback, and proof-gate seams.
4. Delivery Quality + Risk/Safety Reviewer: SIGN-OFF. Required wording: define denominators for source/candidate/evidence counters; require safety proof-gate, side-effect counters, and leak scan.
