# WP-48 Segment-Level Semantic Atomization

## Outcome

Mindline must turn meaningful source segments into evidence-backed semantic atoms instead of collapsing each source into one generic reference candidate. PR42 made the failure visible; WP48 must make the first source-neutral extraction improvement that moves the same real mixed Gmail/Slack dataset toward useful outputs.

## Chain Authority

- `INS-22`: mixed-source intake can hide semantic-density collapse.
- `INS-23`: semantic-density gates must force segment-level atomization.
- `DEC-379`: PR42 is merged into `main`.
- `DEC-374`: WP47 delivered semantic readiness gates that block reference-only one-candidate-per-source collapse.
- `PRI-1`, `BR-1`, `STD-17`, `DEC-64`: semantic autonomy must remain privacy-safe, provider-agnostic, measured, and human-gated until held-out proof exists.

## Product Model Fit

Eligibility: `EXTEND`.

WP48 extends deterministic semantic extraction and candidate consolidation inside the source-neutral semantic layer. It does not add Gmail logic, Slack logic, destination-specific behavior, provider-specific behavior, or autonomous writes. Source adapters still only feed Markdown-like source items with provenance; destination adapters still receive unresolved candidates only after safety and evidence gates.

## Spec Challenge

Initial target rejected: "produce more than 50 candidates" is too weak because a noisy splitter could inflate counts without increasing value.

Sharper target accepted: produce more non-reference, evidence-backed observations and candidates from existing segment evidence, while preserving review status, provenance, and no-destination-write guardrails. The same PR41/PR42 dataset is used only as a comparable private-local pressure case, not as a generalization claim.

## In Scope

1. Deterministic segment-level semantic atomization for existing document segments.
2. Source-neutral extraction rules for explicit questions, decisions, actions, requirements, dependencies, risks, objections, proposals, recaps, claims, deadlines, and owner/deadline signals when present in segment text.
3. Candidate consolidation that can preserve multiple meaningful atoms per source instead of collapsing all observations into one source-level candidate.
4. Evidence ranges and node references remain inspectable through existing structure nodes.
5. Reference fallback remains available only when no stronger semantic atom exists for that source.
6. Runtime comparison against the same private-local PR41/PR42 mixed source manifest at `/private/tmp/mindline-wp46-real/mixed-corpus/corpus-pressure-manifest.json`, using PR42 output `/private/tmp/mindline-wp46-real/mixed-pressure` as the baseline.
7. Readback/proof interpretation remains honest: this PR may prove a private-local improvement over the baseline dataset, not DEC-64, no-human autonomy, or broad generalization.

## Out Of Scope

- LLM classifier changes.
- Gmail- or Slack-specific extraction heuristics.
- Link enrichment, media enrichment, or network fetching.
- Destination writes to Product Brain, Tolaria, Notion, Obsidian, Linear, APIs, or local destination folders.
- Human-review UI changes.
- Generalization, no-human, or autonomous action claims.

## Behavioral Contract

For deterministic semantic extraction:

- blocked structure nodes still produce no usable semantic atom;
- reference observations are fallback-only and must not be emitted when a source has stronger non-reference atoms;
- a source may produce multiple candidates when its evidence contains multiple distinct semantic atoms;
- candidate kinds should match the atom, not the source container;
- all candidates keep `destination_status: unresolved`;
- unsafe/private markers still block or redact through the existing safety classifiers.

For the PR41/PR42 comparison dataset:

- the run must no longer be exactly 50 candidates for 50 sources with 50 reference candidates;
- semantic observation density must improve over the PR42 baseline of 50 observations over 425 segments;
- the proof packet must say what improved and what remains blocked.

## Key Results

1. Same-dataset semantic observation count increases from PR42 baseline `50` to at least `125`.
2. Same-dataset semantic candidate count increases from PR42 baseline `50` to at least `100`.
3. Same-dataset reference-candidate ratio drops below `0.50`.
4. Same-dataset reference-only source count drops below `25`.
5. Same-dataset readback no longer reports `reference_only_one_candidate_per_source`.
6. Review burden remains controlled: `review_burden_ratio` does not exceed `0.60` on the same private-local run.
7. Published proof/readback artifacts, staged changes, Chain/PR text, and committed files contain no raw private Gmail/Slack body text; primary private-local corpus output may contain local source copies only under `/private/tmp` and must not be committed or quoted.
8. Focused and full Go tests pass.

## Guardrails

- Do not hardcode Randy, Gmail, Slack, `/private/tmp`, or PR41 corpus identities in product logic.
- Do not commit private runtime artifacts.
- Do not let count inflation become a success claim; every new candidate must derive from evidence and carry a concrete semantic kind.
- Do not weaken PR42 readiness gates.
- Do not claim destination readiness, no-human readiness, or held-out generalization.
