# WP-33 Local Source Enrichment V0 Plan

**Spec:** `.productbrain/specs/2026-05-27-wp-33-local-source-enrichment-v0.md`  
**Stop mode:** Full Delivery  
**Implementation rule:** enrich existing corpus-pressure input before semantic extraction; do not create a parallel semantic path.

## 1. Product Brain Authority

1. Capture WP-33 after spec sign-off.
2. Link WP-33 to STR-3, WS-3, PRI-1, STD-5, STD-17, WP-32, INS-19, and a new tension for live-fetch policy.
3. Run `pb audit WP-33 --phase handoff --verbose`.
4. Reconcile warnings that affect delivery authority before implementation.

## 2. Code Shape

Add a local enrichment layer in `internal/documents`:

- `source_enrichment.go`
- `source_enrichment_writer.go`
- `source_enrichment_test.go`

Primary API:

```go
func BuildSourceEnrichment(manifestPath, artifactManifestPath, outDir string) (SourceEnrichmentSummary, error)
```

The builder:

1. loads an existing corpus-pressure manifest,
2. reads each source Markdown file through the same containment model,
3. extracts stable ordered URLs,
4. classifies each URL with a no-fetch policy,
5. matches local artifacts by normalized URL,
6. writes enriched Markdown sources under `--out/sources/<source_id>/source.md`,
7. writes a pressure-compatible `corpus-pressure-manifest.json`,
8. writes source-enrichment summary/report/per-source JSON artifacts.

## 3. CLI Shape

Add:

```bash
mindline documents enrich-sources <corpus-pressure-manifest.json> --artifacts <local-enrichment-artifacts.json> --out <dir>
```

CLI must:

- validate `--out` with existing protected-root output rules,
- return usage errors for missing arguments,
- return artifact-write errors for containment/write failures,
- print the summary JSON to stdout.

## 4. Tests

Focused tests:

1. local artifact happy path appends an enriched section and emits enriched counters.
2. every URL is accounted for even when local artifact is missing.
3. unsupported/unknown URLs are visible and do not disappear.
4. unsafe/private/secret-looking URLs are blocked before enrichment.
5. unsafe local artifact payload fields are blocked/redacted before any Markdown, summary JSON, report Markdown, or per-source JSON write.
6. URL records include `retrieval_mode=local_artifact` or `retrieval_mode=none`, plus blocked/redacted reason codes.
7. policy rejects localhost, loopback, RFC1918, link-local, metadata IPs, malformed hosts, non-HTTP(S), and Slack private file sentinels.
8. no-network boundary proof: enrichment has no fetch/browser/Slack/LLM execution path or hidden flag.
9. output manifest feeds `BuildCorpusPressure`.
10. meaning-preview over enriched pressure output includes enriched evidence.
11. writer rejects symlink/protected-root style escapes via existing output validation.
12. CLI test covers successful command and usage errors.

## 5. Verification Commands

```bash
go test ./internal/documents ./internal/cli
go test ./...
git diff --check
pb audit WP-33 --phase handoff --verbose
```

After the runtime proof commands below, scan generated output bundles only:

```bash
rg "xoxb-|sk_live_|sk-proj-|files-pri|workspace\\.slack\\.com/archives|PRIVATE_CONTENT" /private/tmp/mindline-wp33-enriched /private/tmp/mindline-wp33-pressure /private/tmp/mindline-wp33-meaning
```

The generated-output leak scan is expected to return no matches. It intentionally does not scan source code, tests, specs, or plans because those surfaces may contain detector strings or synthetic unsafe fixtures used to prove blocking behavior.

Runtime proof:

```bash
go run ./cmd/mindline documents enrich-sources <manifest> --artifacts <artifact-manifest> --out /private/tmp/mindline-wp33-enriched
go run ./cmd/mindline documents corpus-pressure /private/tmp/mindline-wp33-enriched/corpus-pressure-manifest.json --out /private/tmp/mindline-wp33-pressure
go run ./cmd/mindline documents meaning-preview /private/tmp/mindline-wp33-pressure --out /private/tmp/mindline-wp33-meaning
```

## 6. Review Gates

Before push:

- Chain Steward signs off that WP-33 is captured and audit-reconciled.
- Systems Architect signs off that enrichment feeds the existing corpus path.
- Domain/User Job signs off that Markdown/report output is reviewable by a human.
- Risk/Safety signs off that V0 does not fetch and fails closed.
- Delivery Quality signs off on tests, containment, and maintainability.

## 7. Non-Goals To Protect

- Do not add live network fetching under hidden flags.
- Do not use LLMs for enrichment.
- Do not add destination routes or apply payloads.
- Do not optimize for only `/temp` or the private Slack smoke set.
- Do not remove URL missingness from meaning-preview unless evidence is actually enriched.
