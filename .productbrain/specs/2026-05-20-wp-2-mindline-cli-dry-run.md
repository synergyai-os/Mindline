# WP-2 Spec: Mindline CLI Dry-Run Runner

Output version: `MINDLINE-WP2-SPEC-V3`

## Authority

- Product: `PROD-1` Mindline
- Prior delivery: `WP-1` validation-only Second Brain OS core
- Architecture decisions: `DEC-2`, `DEC-3`, `DEC-4`, `DEC-5`
- Standards: `STD-11`, `STD-12`, plus WP-1 safety/visibility standards
- Published repo: `INS-8`

## Product Model Fit Proof

Verdict: PASS.

Mindline is `PROD-1`: an infrastructure product whose current validated slice is a headless Go core. A CLI dry-run runner is the next smallest product-shaped capability because it turns the core into an executable developer surface without changing the product model. It strengthens the adapter architecture by giving future source and destination adapters a shared validation path before live integrations exist.

This is not bespoke local automation: it becomes the first public developer interface for Mindline and remains useful for any future adapter/runtime.

## Impact Pack

Users affected:

- Randy and agents building Mindline now.
- Future contributors who need to validate normalized candidate contracts.
- Future adapter authors who need a safe dry-run before live integration.

Upstream effects:

- Source adapters can target a fixture/contract runner before live ingestion.
- Existing `internal/sbos` must stay CLI-agnostic.

Downstream effects:

- Destination adapters can later consume dry-run artifacts or result envelopes.
- README/docs can show a concrete first command.

Governance:

- PB remains SSOT before/during/after work (`STD-11`).
- Private provenance publish-blocking remains authoritative and must be visible through CLI outcomes (`STD-12`).

Regression map:

- Core safety gates must not regress.
- CLI must not introduce live Slack/Tolaria/network/auth/db dependencies.
- Default command must not write files.
- `--out` must not write outside the requested directory.

Outcome:

- A developer can run one candidate fixture through Mindline and inspect a deterministic result.

Guardrails:

- No live adapters.
- No hidden writes.
- No provider choices.
- No path escape from `--out`.

## Problem

Mindline has a tested core package, but there is no executable path for a developer or agent to run a normalized candidate fixture through the engine. Without a CLI dry-run runner, future adapter work has no simple, repeatable way to prove what the core will do before live source or destination integrations exist.

## Direction

Build a minimal Go CLI entrypoint named `mindline` with a `process` command. It reads one normalized candidate JSON file, processes it through the existing core, emits deterministic dry-run output, and exits with explicit status codes. The default output is stdout-only. File writes happen only when the user explicitly passes `--out <dir>`.

## In Scope

- `cmd/mindline` executable entrypoint.
- A small CLI package that is testable without spawning an external process.
- Command shape: `mindline process <candidate.json> [--out <dir>]`.
- Read candidate JSON from the provided file path.
- Process through the existing `internal/sbos` engine.
- Emit a deterministic JSON result envelope to stdout.
- Emit errors to stderr.
- Support optional `--out <dir>` for writing emitted artifacts as deterministic files.
- Return clear exit codes for core outcomes.
- Strict TDD: failing tests before production code for each behavior group.

## Out of Scope

- Live Slack ingestion.
- Live Tolaria, Obsidian, Notion, Mem, or other destination writes.
- Network access.
- Authentication.
- Database/provider selection.
- Long-running daemon mode.
- Interactive prompts.
- LLM enrichment or automatic classification.
- Multiple candidate batch processing.

## CLI Contract

Default:

```bash
mindline process candidate.json
```

Behavior:

- reads `candidate.json`
- processes it through `sbos.Engine`
- writes a JSON result envelope to stdout
- writes no files

Explicit artifact output:

```bash
mindline process candidate.json --out ./dry-run
```

Behavior:

- writes the same JSON result envelope to stdout
- writes artifact files only if the engine emits artifacts
- creates `--out` directory if missing
- never writes outside the requested output directory

## Result Envelope

The stdout envelope must be deterministic JSON with stable field names:

```json
{
  "state": "dry_run_published",
  "record_id": "candidate-publish",
  "artifact_count": 1,
  "artifacts": [
    {
      "kind": "dry_run_publish",
      "path": "",
      "body": "..."
    }
  ],
  "authority_ids": ["DEC-4", "DEC-3", "DEC-2", "DEC-1", "FEAT-1", "STD-1", "STD-7", "STD-10", "STD-11", "STD-12", "FEAT-4", "WP-1"]
}
```

JSON formatting:

- pretty printed with two-space indentation
- field order follows the Go struct order used by `internal/cli`
- exactly one trailing newline
- `authority_ids` order is stable and includes `STD-12`

When no `--out` is used:

- `path` is empty
- `body` contains the full artifact body

When `--out` is used:

- `path` is a clean relative path from the current working directory to the written file when possible
- `body` is an empty string in stdout to avoid duplicating artifact content
- the written file contains the full artifact body

## Artifact File Contract

When `--out` is provided:

- publish artifacts use `<candidate_id>-publish.md`
- attention artifacts use `<candidate_id>-attention.md`
- files are written only for emitted artifacts
- file contents exactly match the artifact body
- stdout still reports the file path
- artifact filenames are sanitized to use only letters, numbers, dash, underscore, and dot
- path separators, absolute paths, `..`, and empty candidate IDs are never allowed to influence output paths
- all artifact paths must remain inside the requested output directory after path cleaning

## Exit Codes

- `0`: candidate processed into `dry_run_published`, `attention_ready`, `background_ready`, `skipped`, or `needs_enrichment`
- `1`: command usage error, missing file path, unknown command, unreadable input file, invalid `--out`
- `2`: candidate schema/validation/process error that results in core `error` state
- `3`: artifact write failure after core processing

## Error Message Contract

Stderr must include these stable substrings:

- usage/unknown command/missing path: `usage: mindline process <candidate.json> [--out <dir>]`
- unreadable input file: `read candidate:`
- invalid candidate/core error: `process candidate:`
- invalid `--out`: `invalid --out:`
- artifact write failure: `write artifact:`

Invalid `--out` includes:

- empty value
- path exists and is not a directory
- directory cannot be created
- directory cannot be written

Artifact write failure behavior:

- return exit code `3`
- write `write artifact:` to stderr
- do not print a success envelope to stdout
- keep already-written files as-is; rollback is out of scope for this slice

## Acceptance Criteria

1. `mindline process <candidate.json>` prints deterministic JSON to stdout and writes no files.
2. `mindline process <candidate.json> --out <dir>` writes deterministic artifact files only inside `<dir>` and prints paths in the JSON envelope.
3. Missing command, missing file path, unknown flags, and unreadable files return exit code `1` and write useful stderr.
4. Invalid candidate JSON or schema returns exit code `2`, writes useful stderr, and emits no artifact files.
5. Artifact write failures return exit code `3`.
6. Publish, attention, background, skipped, and needs-enrichment states all produce deterministic envelopes.
7. Background, skipped, and needs-enrichment states do not write artifact files.
8. CLI code has no live Slack/Tolaria/network/auth/database dependencies.
9. Tests prove stdout, stderr, exit codes, no-write default, explicit `--out`, and deterministic output.
10. Tests prove `STD-12`: private provenance field visibility blocks publish through the CLI, emits no publish artifact, and does not leak private permalink/author/body values in stdout or files.
11. Tests prove `--out` path containment, including candidate IDs with separators, absolute-looking values, and `..`.
12. `go test -count=1 ./...` passes.

## Implementation Shape

- `cmd/mindline/main.go` should only wire OS stdin/stdout/stderr/args to the CLI package and call `os.Exit`.
- `internal/cli` should own command parsing, file IO boundary, result envelope, and exit code mapping.
- `internal/sbos` remains the domain core and should not gain CLI concerns.

## Review Requirements

LOOP reviewers must sign off on this spec before `WP-2` is created or implementation starts:

- Chain Steward
- Domain/User Job Reviewer
- Systems Architect
- Delivery Quality Reviewer
- Risk/Safety Reviewer
