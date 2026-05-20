# Mindline Candidate Contract v0.1

Mindline source adapters emit normalized candidate JSON. A candidate is a source-side processing object: it preserves what was captured, where it came from, what the adapter knows, and what visibility the item is allowed to request.

Candidates are not destination instructions. They must not contain Tolaria paths, Obsidian folders, Notion page IDs, Mem metadata, final Markdown frontmatter, or write commands.

## Required Fields

- `schema_version`: currently `v0.1`.
- `candidate_id`: stable adapter-local candidate id.
- `adapter_id`: source adapter id such as `slack`, `browser`, `youtube`, or `github`.
- `external_id`: native source id.
- `captured_at`: source capture timestamp.
- `idempotency_key`: stable key used to avoid duplicate records.
- `desired_visibility`: one of `background`, `attention`, `publish`, `clarify`.
- `enrichment_status`: one of `not_required`, `complete`, `incomplete`, `failed`.

## Provenance

`provenance` is required and each field wraps a `value` plus `visibility`.

Required fields:

- `permalink`
- `native_timestamp`
- `author`
- `raw_locator`

Visibility is either `public` or `private`. Private provenance is authoritative: if any provenance field is private, publish output is blocked unless the item is routed to an attention preview with redaction.

## Content

`content` is required:

- `text`: captured or enriched source text.
- `urls`: related URLs preserved by the adapter.
- `attachments`: source attachment identifiers or URLs.
- `source_title`: human-readable title.

Adapters should preserve save intent. If a captured post points to another page, keep both the captured source context and the outbound URL in the candidate. Do not fetch the network as part of this contract.

## Classification

`classification` is required:

- `type`
- `domain`
- `topics`
- `confidence`
- `needs_clarification`
- `clarification_reason`

Classification helps the core route a candidate. It does not choose a destination folder or write location.

## Safety

`safety` is required:

- `redaction_required`
- `secret_like`
- `empty_content`
- `private_provenance`

Empty or secret-like captures are skipped. Redacted or private-provenance publish attempts are blocked from publish output.

## Expected Routes

- Publish-ready candidates produce `dry_run_published`.
- Attention candidates produce `attention_ready`.
- Clarify candidates produce `attention_ready`.
- Background candidates produce `background_ready`.
- Incomplete or failed enrichment produces `needs_enrichment`, unless clarify intent creates an attention preview.
- Empty or secret-like candidates produce `skipped`.
- Private provenance publish attempts produce `background_ready`.
- Invalid schema candidates fail with CLI exit code `2`.

## Conformance

Run:

```bash
go test -count=1 ./...
```

The fixture manifest at `examples/candidates/manifest.json` is the executable expectation source for the public examples.
