# WP-3 Plan: Mindline Candidate Contract and Fixture Pack

Output version: `MINDLINE-WP3-PLAN-V1`

Authority: `MINDLINE-WP3-SPEC-V2`, `WP-3`, `DEC-6`, `STD-5`, `STD-6`, `STD-7`, `STD-11`, `STD-12`.

## Delivery Sequence

1. Add failing conformance tests first.
   - Create tests that expect `docs/candidate-contract.md`, `examples/candidates/manifest.json`, and every required fixture file.
   - Tests must load the manifest, verify every fixture is valid JSON, run CLI processing through the existing runner, and assert exact manifest expectations.
   - Verify red before creating docs/fixtures.

2. Add the fixture manifest and fixtures.
   - Create `examples/candidates/manifest.json` with the exact V2 expectation table.
   - Create all fixture JSON files required by the spec.
   - Keep fixtures source-candidate-only: no Tolaria paths, Notion IDs, Obsidian folders, destination frontmatter, or write instructions.

3. Add public contract documentation.
   - Create `docs/candidate-contract.md`.
   - Document schema, enums, adapter responsibilities, safety/provenance rules, idempotency, source/destination boundary, and conformance usage.

4. Wire conformance tests to the current CLI/core.
   - Use `internal/cli.Runner` with `MemoryFS` or a test filesystem to avoid shelling out.
   - Validate valid fixture states, artifact counts, deterministic stdout, no-leak assertions, invalid fixture exit code/stderr, and path containment.
   - Keep the harness inside tests unless a command becomes necessary.

5. Update README.
   - Add a short section pointing to the candidate contract, fixtures, and `go test -count=1 ./...`.

6. Verify and review.
   - Run `gofmt`.
   - Run `env GOCACHE="$PWD/.cache/go-build" go test -count=1 ./...`.
   - Run `env GOCACHE="$PWD/.cache/go-build" go test -json ./...`.
   - Search for forbidden live dependencies or destination-specific schema leakage.
   - Run LOOP delivery/review sign-off before PB close-out.

## TDD Checkpoints

- RED 1: conformance test fails because docs/manifest/fixtures are missing.
- GREEN 1: docs/manifest/fixtures exist and parse.
- RED 2: expected-state tests fail until fixtures encode correct candidate behavior.
- GREEN 2: valid and invalid fixture expectations pass through CLI runner.
- RED 3: privacy/path assertions fail until manifest/test assertions are complete.
- GREEN 3: no-leak and path containment assertions pass.

## File Ownership

- Add: `docs/candidate-contract.md`
- Add: `examples/candidates/*.json`
- Add/update: conformance tests under `internal/cli` or a dedicated internal test package
- Update: `README.md`

## Guardrails

- No live Slack access.
- No Tolaria writes or destination-specific schema assumptions.
- No network, auth, database, provider, or LLM dependencies.
- No change to `mindline process` command shape unless the spec is revised and re-signed.
- Do not commit local-only `AGENTS.md`, `.productbrain/`, or `scripts/`.

