# Mindline

Mindline is a headless knowledge-processing engine for turning raw captures into structured, safe, useful personal knowledge across any source and interface.

It is not a notes app, vault, or UI. Mindline is the engine layer between capture surfaces and knowledge surfaces:

- source adapters ingest captures from tools such as Slack, web pages, YouTube, PDFs, email, screenshots, or GitHub
- the core normalizes candidates, preserves provenance, applies safety gates, tracks processing state, and decides visibility
- destination adapters publish only useful outputs to surfaces such as Tolaria, Obsidian, Notion, Mem, a local folder, or a custom app

The first implementation slices are intentionally small. They prove the core contract and expose a dry-run developer interface without live source ingestion or live destination writes.

## Current Slice

The current Go core validates normalized JSON candidates and applies deterministic gates:

- required schema, provenance, content, classification, visibility, and idempotency fields
- local candidate store abstraction
- empty and secret-like content skipping
- private provenance and redaction blocking
- enrichment blocking
- clarification, background, attention, and publish routing
- neutral SBOS dry-run artifacts
- method-profile Markdown rendering in the pipeline layer
- destination-neutral dry-run operation planning
- Tolaria dry-run previews without Tolaria vault writes
- PB authority metadata for the build contract

The CLI can run one normalized candidate fixture through that core:

```bash
go run ./cmd/mindline process candidate.json
go run ./cmd/mindline process candidate.json --out ./dry-run
```

It can also normalize local Slack-like dry-run exports into candidate JSON plus checkpoint metadata:

```bash
go run ./cmd/mindline slack normalize examples/slack/reverse-ordered-batch.json
go run ./cmd/mindline slack normalize examples/slack/reverse-ordered-batch.json --out ./dry-run
```

By default, it prints a deterministic JSON result envelope to stdout and writes no files. With `--out`, it writes only emitted dry-run artifacts to the requested directory and reports their paths in stdout.

Slack normalization is local dry-run processing only: no live Slack API calls, no Tolaria writes, and no destination writes.

## Destination Dry-Run

Destination adapters consume a versioned destination input envelope and plan local operations. The contract is destination-neutral: operation ids, write mode, visibility lane, planned locator, blockers, metadata, and authority ids are shared across future destinations.

Tolaria is the first destination adapter, but WP-5 only supports dry-run planning. It never writes to the Tolaria vault, never calls live destination APIs, and never requires Slack, auth, network, PB runtime access, or provider credentials.

```bash
go run ./cmd/mindline destination dry-run examples/destinations/tolaria/publish.json --adapter tolaria --out ./dry-run
go run ./cmd/mindline destination dry-run examples/destinations/tolaria/attention.json --adapter tolaria --out ./dry-run
go run ./cmd/mindline destination dry-run examples/destinations/tolaria/background.json --adapter tolaria --out ./dry-run
```

The command requires `--out` and writes only under that directory:

- `operations/<operation_id>.json` for every planned operation
- `previews/<operation_id>.md` only when a publish or attention preview body is safe to inspect
- `destination-summary.json` with the same deterministic summary printed to stdout

Background, skipped, and blocked operations do not create Markdown previews. Conflict-blocked operations keep their original operation id for traceability, clear their preview body, and report stable blocker metadata.

## Local Pipeline Dry-Run

The local pipeline runner composes the first end-to-end dry-run path:

```bash
go run ./cmd/mindline pipeline dry-run testdata/pipeline/inputs/pipeline-text-only.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp7-output
```

This command is dry-run only. It validates Product Brain authority ids, loads local fixture input, runs SBOS, applies the selected method profile, plans processors, hands safe publish output to the Tolaria destination dry-run adapter, and writes deterministic artifacts under `--out`.

`basb-para-code` is the first method profile, not core architecture. Processor routing is planning-only: YouTube, LinkedIn, web, PDF, unknown, private, and secret-like captures produce local plans and blockers, but the runner does not call live Slack APIs, browsers, LLMs, auth providers, databases, destination APIs, network services, or the Tolaria vault.

Tolaria is the first destination adapter, not the core surface. Future destinations can consume the same pipeline result and processor plan contracts.

Each pipeline dry-run also writes a local run ledger and derived review queue:

- `ledger/run-manifest.json` records the deterministic run id, input fingerprint, state counts, review count, and WP-8 authority ids.
- `ledger/index.json` is the stable item lookup surface.
- `ledger/items/<record_id>.json` records one safe, path-stable outcome per item.
- `review-queue/review-queue.json` lists only items that need enrichment, clarification, or blocker review.
- `review-queue/items/<record_id>.json` gives safe local context and links for each review item.

Text-only publish previews are excluded from the review queue. Private provenance alone is retained as background ledger evidence, not a review item. Secret-like content is skipped without readable body content. Reusing an output directory for the same deterministic run is allowed; reusing it for a different run is refused before new ledger or review queue files are written.

## Product Brain Proposal Dry-Run

Mindline can turn a local run review queue into Product Brain proposal artifacts without writing to Product Brain:

```bash
go run ./cmd/mindline product-brain propose testdata/productbrain/runs/reviewable --profile testdata/productbrain/profiles/default-governance.json --out /tmp/mindline-pb-proposals
```

The profile is a workspace contract, not a hardcoded adapter. It describes the target workspace identity, kernel write affordances, collections, fields, workflow statuses, guidance, quality criteria, and `intent_mappings`. The adapter resolves Mindline semantic intents through that profile so custom workspaces can use renamed collections and fields.

The command writes only under `--out`:

- `productbrain-proposals/proposal-summary.json`
- `productbrain-proposals/proposals/<proposal_id>.json`
- `productbrain-proposals/previews/<proposal_id>.md` for every proposal

WP-9 is proposal-only. It does not call Product Brain runtime services, Convex, `pb`, Slack, Tolaria, network APIs, auth providers, schedulers, LLMs, or browsers. Future live application must treat `externalRef` as source/object identity, `idempotencyKey` as proposal retry/application identity, and preserve actor authority plus provenance for kernel auditability.

## Candidate Contract

Source adapters emit normalized candidate JSON. The public contract is documented in [docs/candidate-contract.md](docs/candidate-contract.md), with runnable examples in [examples/candidates](examples/candidates).

The fixture manifest is the conformance source for examples:

```bash
go test -count=1 ./...
```

## Verify

```bash
go test ./...
```
