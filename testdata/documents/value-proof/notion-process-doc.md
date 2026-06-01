# Document processing operating model

## Principles

- Standard: core segments must stay destination-neutral.
- Decision: adapters map segments after decomposition, not during decomposition.

## Capability table

| Capability | Purpose | Status |
| --- | --- | --- |
| Segment contract | Define a reusable document candidate shape | Ready |
| Artifact writer | Persist summaries, segment JSON, and previews under explicit output | Ready |
| Review status | Keep uncertain material out of ready downstream flow | Needs review |
