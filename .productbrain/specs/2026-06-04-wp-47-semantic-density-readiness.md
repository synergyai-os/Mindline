# WP-47 Semantic Density Readiness

## Outcome

Mindline must distinguish source processing coverage from semantic extraction value. A corpus run can be privacy-safe and fully processed while still failing to extract meaningful atoms; readback and proof surfaces must make that failure explicit.

## Chain Authority

- `INS-22`: PR 41 processed 50/50 real Gmail/Slack sources, but 425 ready document segments collapsed into 50 observations and 50 candidates: exactly one `reference_candidate` per source.
- `WP-43` / PR 41: mixed Gmail/Slack corpus intake proof is merged and provides the comparison baseline.
- `STD-17`, `DEC-64`, `PRI-1`, `BR-1`: semantic autonomy claims require measured, privacy-safe, provider-agnostic evidence.
- WP-35 / WP-36 specs: `eval readback` and `eval proof-gate` are the claim-gating surfaces that prevent command success from becoming outcome success.

## Product Model Fit

Eligibility: `EXTEND`.

WP-47 extends the existing corpus-pressure and eval-readback/proof contract. It does not add source-specific Gmail or Slack logic, destination behavior, or autonomous writes. The product object is the eval evidence packet that sits between source ingestion and any destination proposal.

## In Scope

1. Corpus pressure emits additive semantic-density counters:
   - processed source count;
   - document segment count;
   - semantic observation count;
   - semantic candidate count;
   - reference candidate count;
   - one-candidate source count;
   - reference-only source count;
   - candidate/source, observation/segment, and reference-candidate ratios.
2. Corpus pressure emits a `semantic_readiness_status` and reason codes.
3. `ready_for_50_file_pressure` is blocked for larger runs that collapse into reference-only one-candidate-per-source output.
4. Eval readback consumes the new counters and can also reconstruct the collapse from existing PR 41 artifacts.
5. Readback adds a `semantic_readiness` claim gate and makes semantic collapse the top improvement target ahead of generic held-out-label advice.
6. `eval proof-gate --claim improvement` and claim classification block improvement when semantic readiness is blocked.
7. Generated readback/proof artifacts remain private-safe and contain only metadata, counts, reason codes, and safe refs.

## Out Of Scope

- Full semantic atom extraction improvement.
- LLM provider changes.
- Gmail- or Slack-specific extraction rules.
- Destination writes to Product Brain, Tolaria, Notion, Obsidian, Linear, or APIs.
- No-human, DEC-64, or generalization claims.

## Behavioral Contract

For larger corpus pressure runs, a run is semantically blocked when all of these are true:

- processed source count is at least 10;
- semantic candidate count equals processed source count;
- reference candidate count equals semantic candidate count;
- every processed source produced exactly one candidate.

The run is also semantically blocked when observation/segment density is very low:

- processed source count is at least 10;
- document segment count is at least twice semantic observation count;
- observation-per-segment ratio is below `0.25`.

Blocked semantic readiness does not mean intake failed. It means Mindline has not proven meaningful extraction value for the run.

## Key Results

1. The existing PR 41 real mixed-source run is read back as semantic-readiness blocked with reason `reference_only_one_candidate_per_source`.
2. A committed fixture with 50 processed sources, 425 segments, and 50 reference candidates is blocked by readback and proof-gate improvement.
3. A richer fixture with more observations/candidates than sources is not blocked by semantic readiness.
4. Existing safety proof for corpus-pressure fixtures still passes when semantic readiness is not part of the requested claim.
5. Full Go tests pass.
6. Runtime proof over the same PR 41 dataset produces privacy-safe readback/proof artifacts and no private content is committed.

## Guardrails

- No raw Gmail or Slack body text in committed fixtures, reports, Chain captures, or PR text.
- No source-specific hardcoding of Gmail, Slack, Randy, `/private/tmp`, or the PR 41 corpus identity.
- Additive schema only; existing artifact consumers should continue to load old artifacts.
- Command success alone remains insufficient for outcome claims.

