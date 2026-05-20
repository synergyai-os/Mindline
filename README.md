# Mindline

Mindline is a headless knowledge-processing engine for turning raw captures into structured, safe, useful personal knowledge across any source and interface.

It is not a notes app, vault, or UI. Mindline is the engine layer between capture surfaces and knowledge surfaces:

- source adapters ingest captures from tools such as Slack, web pages, YouTube, PDFs, email, screenshots, or GitHub
- the core normalizes candidates, preserves provenance, applies safety gates, tracks processing state, and decides visibility
- destination adapters publish only useful outputs to surfaces such as Tolaria, Obsidian, Notion, Mem, a local folder, or a custom app

The first implementation slice is intentionally small and validation-only. It proves the core contract without live source ingestion or live destination writes.

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

No live Slack, Tolaria, network, or filesystem publishing is wired into this slice.

## Verify

```bash
go test ./...
```
