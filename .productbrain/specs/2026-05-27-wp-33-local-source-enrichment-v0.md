# WP-33 Local Source Enrichment V0 Spec

**Status:** signed-spec candidate  
**Date:** 2026-05-27  
**Product:** Mindline  
**Owning workstream:** WS-3 Read-only ingestion pressure  
**Strategy:** STR-3 Autonomy-readiness before destination writes  
**Governing rules:** PRI-1, STD-2, STD-5, STD-7, STD-11, STD-12, STD-17  
**Input evidence:** WP-32, INS-18, INS-19, TEN-20

## 1. Problem

WP-32 made link-only Slack captures readable and proved the real bottleneck: the system can see URL captures, but it cannot yet understand linked sources. Real private Slack smoke produced source previews for every capture, yet every item remained `needs_enrichment` with `reference_only`, `link_only_source`, and `missing_link_enrichment`.

The next slice must turn local/manual link artifacts into pressure-compatible enriched Markdown so the existing corpus-pressure and meaning-preview loop can judge richer source material. It must not introduce live web fetching yet. Current contracts are no-network by default, and live fetch needs a separately signed policy because it touches privacy, SSRF, credentials, redirects, size limits, and trace sanitization.

## 2. Outcome

Given a corpus-pressure manifest whose Markdown sources contain URLs, Mindline can run a local enrichment pass that:

1. accounts for every URL in every source,
2. appends safe local/manual enrichment evidence where an artifact is available,
3. marks missing, unsupported, unsafe, or private links with explicit reason codes,
4. emits a new corpus-pressure-compatible manifest,
5. produces a human-readable enrichment report and per-source enrichment artifacts,
6. leaves all destination and hosted side effects at zero.

Randy should be able to inspect the generated Markdown/report and understand whether a link capture is now reviewable, still missing context, or blocked.

## 3. Scope

In scope:

- Add a read-only CLI command:

```bash
mindline documents enrich-sources <corpus-pressure-manifest.json> --artifacts <local-enrichment-artifacts.json> --out <dir>
```

- Add a provider-agnostic local source enrichment contract.
- Read only explicit URLs already present in source Markdown.
- Match URLs against a local/manual artifact manifest.
- Produce enriched Markdown sources under `--out/sources/<source_id>/source.md`.
- Produce `--out/corpus-pressure-manifest.json` that points to the enriched Markdown files.
- Produce `--out/source-enrichment/enrichment-summary.json`, `enrichment-report.md`, and per-source JSON artifacts.
- Classify unsupported or unsafe links without fetching.
- Add URL policy classification for future live fetch readiness, while keeping live fetch disabled.
- Preserve original source content and append enrichment evidence; never overwrite provenance.
- Add tests proving the enriched output feeds the existing `documents corpus-pressure` and `documents meaning-preview` path.

Out of scope:

- Live network fetching.
- Browser automation.
- Slack API calls.
- Auth/login.
- Database or persistent review queue.
- Tolaria writes.
- Product Brain writes.
- Destination apply payloads.
- Hosted telemetry exports.
- LLM calls or model tuning.
- No-human, auto-accept, or DEC-64 readiness claims.

## 4. Contracts

### Local Artifact Manifest

`local-source-enrichment-artifacts/v0.1`

```json
{
  "schema_version": "local-source-enrichment-artifacts/v0.1",
  "artifacts": [
    {
      "url": "https://example.com/article",
      "kind": "web_url",
      "title": "Source title",
      "description": "Short metadata summary",
      "excerpt": "Safe local/manual excerpt",
      "source_name": "Example",
      "captured_at": "2026-05-27T00:00:00Z"
    }
  ]
}
```

The manifest is local input, not evidence that the URL was fetched by Mindline. Empty artifacts are allowed; missing artifacts are reported as missingness.

All artifact payload fields are untrusted input. Before any title, description, excerpt, or source name is written to Markdown, summary JSON, report Markdown, or per-source JSON, Mindline must scan it for secret-looking or private-source content. Unsafe payload fields are not copied through as evidence; they become blocked/redacted enrichment with explicit reason codes.

### Enrichment States

- `enriched`: local artifact matched and contributed safe title/description/excerpt.
- `needs_manual_processing`: URL is allowed in principle but no local artifact exists.
- `unsupported_source`: source kind is recognized as unsupported for V0 or unknown.
- `blocked_private_or_secret`: URL/source is private, secret-like, unsafe, or redacted.
- `blocked_by_policy`: URL fails future-fetch URL policy.
- `no_url`: source has no URL.

### Retrieval Mode

Every enriched or blocked URL record must include `retrieval_mode`. V0 supports only:

- `local_artifact`: evidence came from a local/manual artifact manifest.
- `none`: no artifact was used.

V0 must not expose a fetch, browser, or network retrieval mode.

### URL Policy

Even though V0 does not fetch, URL policy must classify future fetch eligibility. It must reject:

- non-HTTP(S) schemes,
- malformed hosts,
- localhost,
- loopback,
- RFC1918 private ranges,
- link-local ranges,
- metadata IPs,
- bare private Slack file sentinels,
- obvious secret-bearing URL text.

## 5. Acceptance Criteria

1. 100% of URLs in every input source are accounted for in summary and per-source artifacts.
2. Supported URLs with local artifacts append an `## Enriched Sources` section to output Markdown.
3. Missing local artifacts remain visible as `needs_manual_processing` with reason `missing_local_artifact`.
4. Unsupported kinds remain visible as `unsupported_source`; they are not dropped.
5. Unsafe/private/secret-looking URLs are blocked before enrichment and never written as enriched evidence.
6. Unsafe/private/secret-looking artifact payload fields are blocked or redacted before any Markdown/JSON write, with reason codes preserved.
7. Every URL record reports `retrieval_mode=local_artifact` or `retrieval_mode=none`; no network/browser/fetch mode exists in V0.
8. Output manifest remains compatible with `documents corpus-pressure`.
9. Running `documents corpus-pressure` and `documents meaning-preview` on enriched output shows enriched evidence in source previews.
10. Zero guardrails remain true: destination writes, Product Brain writes, Tolaria writes, hosted inference calls, and hosted telemetry exports are all `0`.
11. Output containment rejects protected roots and symlink escapes.
12. The committed fixture set is synthetic; private runtime proof stays under `/private/tmp`.

## 6. Aggressive KRs

- KR1: URL accounting coverage is `1.0` on synthetic and private runtime runs.
- KR2: Enriched output coverage is `1.0` for URLs with local artifacts.
- KR3: Unsupported/missing/blocked coverage is `1.0` for URLs without usable artifacts.
- KR4: Existing corpus-pressure and meaning-preview consume enriched output without a special path.
- KR5: Leak scan finds no private Slack IDs, private Slack permalinks, raw Slack file URLs, or secret fixture strings in committed artifacts.
- KR6: No-network proof shows the enrichment implementation has no fetch/browser/Slack/LLM execution path or hidden network flag.

## 7. Risks

- A local artifact may be mistaken for live fetch proof. Mitigation: report `retrieval_mode=local_artifact`.
- Enriched Markdown may look destination-ready. Mitigation: all routing/write guardrails remain non-applyable and zero-write.
- URL policy could be too weak for future live fetch. Mitigation: V0 does not fetch and captures a future-fetch policy boundary.
- Source enrichment could fork the pipeline. Mitigation: output is normal Markdown plus a normal corpus-pressure manifest.

## 8. Done Proof

- Focused unit tests for URL extraction, local artifact matching, unsafe URL blocking, output manifest compatibility, and containment.
- Focused unit tests proving unsafe local artifact payload fields are not written to Markdown, summary JSON, report Markdown, or per-source JSON.
- No-network boundary proof: the enrichment path uses only local files and does not call fetch/browser/Slack/LLM/network surfaces.
- CLI tests for `documents enrich-sources`.
- End-to-end synthetic run:
  1. enrich sources,
  2. run corpus-pressure on enriched manifest,
  3. run meaning-preview,
  4. inspect summary and report.
- Full `go test ./...`.
- `git diff --check`.
- Product Brain audit for WP-33 after Chain capture.
