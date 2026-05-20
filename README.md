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
- PB authority metadata for the build contract

The CLI can run one normalized candidate fixture through that core:

```bash
go run ./cmd/mindline process candidate.json
go run ./cmd/mindline process candidate.json --out ./dry-run
```

By default, it prints a deterministic JSON result envelope to stdout and writes no files. With `--out`, it writes only emitted dry-run artifacts to the requested directory and reports their paths in stdout.

No live Slack, Tolaria, network, auth, database, or provider integration is wired into this slice.

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
