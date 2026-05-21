# Document processing strategy

## Principles

- Standard: core segments must stay destination-neutral.
- Decision: adapters map segments after decomposition, not during decomposition.
- Standard: generated previews summarize metadata instead of copying private source text.

## Capability table

| Capability | Purpose | Status |
| --- | --- | --- |
| Segment contract | Define a reusable document candidate shape | Ready |
| Artifact writer | Persist summaries, segment JSON, and previews under explicit output | Ready |
| Review status | Keep uncertain material out of ready downstream flow | Needs review |

## Work package candidate

Work item: build a local command that decomposes Markdown files into document segment artifacts.

