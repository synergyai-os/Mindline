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
- deterministic dry-run Markdown artifact rendering
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
