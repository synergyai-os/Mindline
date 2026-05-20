# WP-7 Local Pipeline Runner and Method Routing Implementation Plan

Plan version: `MINDLINE-WP7-PLAN-V5`
Date: 2026-05-20
Status: Revised draft for LOOP Plan review. Not delivery authority until signed in Product Brain.

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:test-driven-development` before implementation, then `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Mindline's first local-only `pipeline dry-run` runner with method-profile policy, processor-routing plans, deterministic safe artifacts, and Tolaria destination dry-run handoff.

**Architecture:** Add `internal/pipeline` as the composition layer between local source/candidate input, existing SBOS processing, method policy, processor routing, WP-5 destination dry-run operations, and artifact writing. Keep BASB/PARA/CODE inside `internal/pipeline/methods`, keep processor routing planning-only inside `internal/pipeline/processors`, and isolate all file creation in `internal/pipeline/artifacts`. The CLI only wires these packages together after the contracts pass red/green tests.

**Tech Stack:** Go, standard library JSON/path/file APIs, existing `internal/sbos`, existing `internal/adapters/slack`, existing `internal/destinations`, and existing CLI runner patterns.

**Product Brain Authority:** `WP-7` is the governed Product Brain work item for the human-readable WP-6 slice. The plan is aligned to `MINDLINE-WP6-SPEC-V12`; Product Brain `DEC-17` must be updated to sign V12 and this plan before delivery authority exists. The dropped accidental `WP-6` capture is not authority and must be rejected by implementation fixtures.

**Delivery Preconditions:** Do not implement until WP-5 PR #2 is merged into `main`, or Product Brain captures an explicit branch-base decision to build on `codex/wp-5-destination-dry-run`. At execution time, verify the branch contains `internal/destinations`, `internal/destinations/tolaria`, and `destination dry-run` CLI support before writing pipeline code.

---

## Files and Responsibilities

- Create `internal/pipeline/input.go`: parse and validate `pipeline-input/v0.1`, authority ids, method/destination ids, run mode, source kind, and bundle-root path resolution.
- Create `internal/pipeline/input_test.go`: CLI-independent input validation, authority validation, path-root/symlink behavior, method/destination/run-mode mismatches, and no-output-before-validation tests.
- Create `internal/pipeline/methods/profile.go`: versioned method profile types and the single `basb-para-code` profile.
- Create `internal/pipeline/methods/profile_test.go`: profile loading, unsupported method rejection, policy fields, and method-boundary tests.
- Create `internal/pipeline/processors/route.go`: content-unit detection and planning-only processor routing.
- Create `internal/pipeline/processors/route_test.go`: text, YouTube, LinkedIn+web, PDF, mixed links, unknown, private provenance, secret-like, blocker ordering, and no-live-action tests.
- Create `internal/pipeline/artifacts/writer.go`: the only WP-7 package allowed to create files; writes summaries, results, processor plans, destination summaries, operation JSON, and previews under `--out`.
- Create `internal/pipeline/artifacts/writer_test.go`: slugging, collision behavior, relative paths, protected output roots, symlink escapes, golden output bodies, and private/secret no-leak scans.
- Create `internal/pipeline/runner.go`: orchestration from parsed input through source loading, SBOS processing, method policy, processor routing, destination planning, artifact writing, and summary JSON.
- Create `internal/pipeline/runner_test.go`: fixture-level golden tests for every spec fixture and invalid-case no-output assertions.
- Modify `internal/sbos/engine.go`: remove method-shaped Markdown rendering from production SBOS output so method/profile note sections live in `internal/pipeline/methods`.
- Modify `internal/sbos/engine_test.go` and `internal/cli/cli_test.go`: update legacy expectations to neutral SBOS artifacts and keep method-shaped output assertions inside pipeline tests.
- Modify `internal/cli/runner.go`: add `mindline pipeline dry-run <pipeline-input.json> --method basb-para-code --destination tolaria --out <dir>`.
- Create `internal/cli/pipeline_dry_run_test.go`: CLI usage, required flags, mismatches, stdout/file parity, exit codes, protected paths, and no-live-action checks.
- Create `testdata/pipeline/inputs/*.json`: exact valid and invalid pipeline input fixtures from the signed spec.
- Create `testdata/pipeline/candidates/*.json`: exact candidate fixtures from the signed spec.
- Create `testdata/pipeline/slack/*.json`: exact two-message Slack export batch fixture.
- Create `testdata/pipeline/expected/**`: golden summary, result, processor plan, destination summary, operation, and preview files.
- Modify `README.md`: document local pipeline dry-run, run modes, method profile boundary, processor routing boundary, authority validation, and no-live-action guardrails.

## Task 0: Delivery Base Preflight

- [ ] **Step 1: Verify Product Brain and Git base**

Run:

```bash
pb profile list
git branch --show-current
gh pr view 2 --json state,mergeStateStatus,headRefName,baseRefName,url
```

Expected:

- `pb profile list` reports `activeSource: local` and `active: randy-s-pkm`.
- PR #2 is merged into `main`, or Product Brain contains an explicit branch-base decision allowing work on `codex/wp-5-destination-dry-run`.
- If neither is true, stop before implementation.

- [ ] **Step 2: Verify WP-5 code is present**

Run:

```bash
test -f internal/destinations/operation.go
test -f internal/destinations/tolaria/dry_run.go
rg -n 'destination dry-run' internal/cli
```

Expected: all commands succeed. If any fail, stop and reconcile the WP-5 base before continuing.

- [ ] **Step 3: Create implementation branch**

Run only after Steps 1 and 2 pass:

```bash
git switch main
git pull --ff-only
git switch -c codex/wp-7-local-pipeline-runner
```

Expected: new implementation branch from the verified base.

## Task 1: Pipeline Input and Authority Contract

- [ ] **Step 1: Write failing input validation tests**

Create `internal/pipeline/input_test.go` with tests:

```go
func TestParseInputAcceptsValidCandidateInput(t *testing.T) {
	inputPath := writeFixture(t, "inputs/pipeline-text-only.json", `{
	  "schema_version": "pipeline-input/v0.1",
	  "run_mode": "dry_run",
	  "source": {"kind": "candidate", "path": "../candidates/pipeline-text-only.json"},
	  "method": {"id": "basb-para-code"},
	  "destination": {"id": "tolaria"},
	  "authority_ids": ["DEC-15", "DEC-6", "DEC-12", "DEC-13"]
	}`)
	parsed, err := pipeline.ParseInputFile(os.DirFS(filepath.Dir(inputPath)), filepath.Base(inputPath), pipeline.ParseOptions{})
	require.NoError(t, err)
	assert.Equal(t, "dry_run", parsed.RunMode)
	assert.Equal(t, "candidate", parsed.Source.Kind)
	assert.Equal(t, []string{"DEC-15", "DEC-6", "DEC-12", "DEC-13"}, parsed.AuthorityIDs)
}

func TestParseInputRejectsInvalidAuthorityIDsBeforeOutput(t *testing.T) {
	cases := []struct {
		name string
		ids  string
		want string
	}{
		{"missing", ``, "authority_ids are required"},
		{"emptyList", `"authority_ids": []`, "authority_ids are required"},
		{"emptyID", `"authority_ids": [""]`, "authority_id must not be empty"},
		{"droppedWP6", `"authority_ids": ["WP-6", "DEC-15"]`, "dropped or unauthorized authority_id: WP-6"},
		{"unknown", `"authority_ids": ["DEC-999"]`, "unknown authority_id: DEC-999"},
		{"duplicate", `"authority_ids": ["DEC-15", "DEC-15"]`, "duplicate authority_id: DEC-15"},
		{"malformed", `"authority_ids": ["not an id"]`, "malformed authority_id: not an id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pipeline.ParseInputBytes([]byte(validInputWithAuthorityFragment(tc.ids)), "/repo/testdata/pipeline/inputs/pipeline-invalid.json")
			require.ErrorContains(t, err, tc.want)
		})
	}
}
```

Run:

```bash
go test ./internal/pipeline -run 'TestParseInput' -count=1
```

Expected: FAIL because `internal/pipeline` does not exist.

- [ ] **Step 2: Implement the minimal input parser**

Create `internal/pipeline/input.go` with:

```go
package pipeline

type SourceKind string

const (
	SchemaPipelineInput = "pipeline-input/v0.1"
	RunModeDryRun       = "dry_run"
	SourceSlackExport   SourceKind = "slack_export"
	SourceCandidate     SourceKind = "candidate"
	SourceCandidateBatch SourceKind = "candidate_batch"
)

type Input struct {
	SchemaVersion string      `json:"schema_version"`
	RunMode       string      `json:"run_mode"`
	Source        SourceInput `json:"source"`
	Method        IDRef       `json:"method"`
	Destination   IDRef       `json:"destination"`
	AuthorityIDs  []string    `json:"authority_ids"`
	BundleRoot    string      `json:"-"`
}

type SourceInput struct {
	Kind  SourceKind `json:"kind"`
	Path  string     `json:"path,omitempty"`
	Paths []string   `json:"paths,omitempty"`
}

type IDRef struct {
	ID string `json:"id"`
}

type ParseOptions struct {
	AllowedFutureWorkPackageID string
}
```

Implement `ParseInputBytes`, `ParseInputFile`, `ValidateAuthorityIDs`, and `ResolveBundlePath`. Rules:

- schema must equal `pipeline-input/v0.1`;
- run mode must equal `dry_run`;
- supported source kinds are only `slack_export`, `candidate`, and `candidate_batch`;
- method must equal `basb-para-code`;
- destination must equal `tolaria`;
- authority ids must be non-empty, ordered, allowlisted, unique, not malformed, and must reject `WP-6`;
- bundle root is the parent of `inputs/` when input path is under a directory named `inputs`, else the input file directory;
- source paths and later local artifact paths must resolve under the bundle root after symlink evaluation.

- [ ] **Step 3: Run input tests**

Run:

```bash
go test ./internal/pipeline -run 'TestParseInput|TestResolveBundlePath|TestValidateAuthorityIDs' -count=1
```

Expected: PASS.

## Task 2: Method Profile Boundary

- [ ] **Step 1: Write failing method profile tests**

Create `internal/pipeline/methods/profile_test.go`:

```go
func TestLoadBASBPARACODEProfile(t *testing.T) {
	profile, err := methods.Load("basb-para-code")
	require.NoError(t, err)
	assert.Equal(t, "method-profile/v0.1", profile.SchemaVersion)
	assert.Equal(t, "basb-para-code", profile.MethodID)
	assert.Equal(t, "dry_run", profile.RunMode)
	assert.Equal(t, "PARA", profile.Organize.DefaultModel)
	assert.Equal(t, "youtube_transcript", profile.ProcessorPolicy["youtube_url"].RequiredProcessor)
	assert.Equal(t, "missing_local_youtube_transcript", profile.ProcessorPolicy["youtube_url"].MissingArtifactReason)
}

func TestLoadUnsupportedMethodFails(t *testing.T) {
	_, err := methods.Load("zettelkasten")
	require.ErrorContains(t, err, "unsupported method: zettelkasten")
}
```

Run:

```bash
go test ./internal/pipeline/methods -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 2: Implement method profile loader**

Create `internal/pipeline/methods/profile.go` with versioned structs and a pure `Load(methodID string) (Profile, error)`. Do not import `internal/sbos`, source adapters, destinations, filesystem, network, or CLI packages.

- [ ] **Step 3: Add boundary grep test**

Add a test in `internal/pipeline/methods/profile_test.go` or `internal/pipeline/runner_test.go` that scans `internal/sbos` and `internal/adapters` for `PARA|CODE|BASB|Snapshot|Source Content|Key Details|Relevance|Signals|Related Sources|Next Action`.

Run:

```bash
go test ./internal/pipeline/methods -count=1
```

Expected: PASS.

## Task 2.5: Move Method-Shaped Rendering Out of SBOS

- [ ] **Step 1: Write failing boundary test for production SBOS code**

Add `internal/sbos/method_boundary_test.go`:

```go
func TestSBOSProductionCodeDoesNotOwnMethodSections(t *testing.T) {
	files := []string{"engine.go"}
	for _, file := range files {
		body, err := os.ReadFile(filepath.Join("..", "sbos", file))
		require.NoError(t, err)
		forbidden := []string{"PARA", "CODE", "BASB", "Snapshot", "Source Content", "Key Details", "Relevance", "Signals", "Related Sources", "Next Action"}
		for _, term := range forbidden {
			assert.NotContains(t, string(body), term)
		}
	}
}
```

Run:

```bash
go test ./internal/sbos -run TestSBOSProductionCodeDoesNotOwnMethodSections -count=1
```

Expected: FAIL against the current repo because `internal/sbos/engine.go` still renders method-shaped Markdown.

- [ ] **Step 2: Refactor SBOS artifacts to neutral bodies**

Modify `internal/sbos/engine.go` so production SBOS emits neutral artifact bodies instead of BASB/PARA/CODE or Tolaria-style note sections.

Required behavior:

- keep state transitions, candidate validation, storage, safety handling, and artifact kinds unchanged;
- replace method-shaped section rendering with neutral machine-readable text such as:

```text
source_candidate_id: <candidate-id>
state: <state>
text: <candidate content text>
urls:
- <url>
```

- do not include `Snapshot`, `Source Content`, `Key Details`, `Relevance`, `Signals`, `Related Sources`, `Next Action`, `PARA`, `CODE`, or `BASB` in production SBOS code;
- let pipeline method profile rendering create human-facing sections later.

- [ ] **Step 3: Update legacy tests to neutral SBOS expectations**

Modify `internal/sbos/engine_test.go` and existing CLI process tests so they assert neutral SBOS artifact behavior instead of method-shaped Markdown. Keep old safety assertions for redaction/private/secret behavior.

Run:

```bash
go test ./internal/sbos ./internal/cli -count=1
```

Expected: PASS.

## Task 3: Processor Routing Boundary

- [ ] **Step 1: Write failing processor route tests**

Create `internal/pipeline/processors/route_test.go` with table cases:

```go
func TestPlanProcessorsForFixtureCandidates(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		urls     []processors.URLUnit
		private  bool
		secret   bool
		steps    []string
		blockers []string
	}{
		{"text", "Mindline should keep raw capture, method policy, and destination preview separate.", nil, false, false, []string{"text_capture_review:required:planned"}, nil},
		{"youtube", "Watch https://www.youtube.com/watch?v=wp6example", []processors.URLUnit{{URL: "https://www.youtube.com/watch?v=wp6example", Kind: "youtube_url"}}, false, false, []string{"youtube_transcript:required:planned", "manual_processing_required:blocked:blocked"}, []string{"missing_local_youtube_transcript"}},
		{"linkedinWeb", "https://www.linkedin.com/posts/example-mindline https://example.com/mindline-routing", []processors.URLUnit{{URL: "https://www.linkedin.com/posts/example-mindline", Kind: "linkedin_url"}, {URL: "https://example.com/mindline-routing", Kind: "web_url"}}, false, false, []string{"linkedin_post_context:required:planned", "web_page_metadata:required:planned", "manual_processing_required:blocked:blocked"}, []string{"missing_local_linkedin_context", "missing_local_web_metadata"}},
		{"unknown", "unclassified://mindline/local-capture", []processors.URLUnit{{URL: "unclassified://mindline/local-capture", Kind: "unknown"}}, false, false, []string{"manual_processing_required:blocked:blocked"}, []string{"unknown_content_type"}},
		{"private", "PRIVATE_DM_SENTINEL_DO_NOT_WRITE", nil, true, false, []string{"private_provenance_block:blocked:blocked"}, []string{"private_provenance_requires_review"}},
		{"secret", "sk-test-secret-do-not-leak", nil, false, true, []string{"secret_skip:blocked:blocked"}, []string{"secret_like_content_detected"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := processors.Plan(processors.Input{Text: tc.text, URLs: tc.urls, PrivateProvenance: tc.private, SecretLike: tc.secret}, basbProfile(t), []string{"DEC-15", "DEC-6", "DEC-12", "DEC-13"})
			assert.Equal(t, tc.steps, compactSteps(plan.Steps))
			assert.Equal(t, tc.blockers, plan.Blockers)
		})
	}
}
```

Run:

```bash
go test ./internal/pipeline/processors -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 2: Implement planning-only router**

Create `internal/pipeline/processors/route.go` with `Plan(input Input, profile methods.Profile, authorityIDs []string) Plan`. Implementation rules:

- preserve URL order;
- assign `text-1`, `url-1`, `url-2`, `url-3`;
- emit one required planned processor per external content unit;
- emit one `manual_processing_required` blocked step when required local artifacts are missing;
- if multiple artifacts are missing, `manual_processing_required.reason` is `missing_required_local_artifacts`;
- safety blockers run first and suppress content processors;
- copy authority IDs unchanged;
- never import or call network, browser, LLM, auth, database, Slack API, or destination code.

- [ ] **Step 3: Run processor tests**

Run:

```bash
go test ./internal/pipeline/processors -count=1
```

Expected: PASS.

## Task 4: Signed Fixture Tree

- [ ] **Step 1: Create signed fixtures before golden tests**

Create:

- `testdata/pipeline/inputs/*.json`;
- `testdata/pipeline/candidates/*.json`;
- `testdata/pipeline/slack/pipeline-slack-export-batch.json`;
- `testdata/pipeline/invalid/*.json`;
- `testdata/pipeline/expected/pipeline-text-only/**`.

Use the exact bodies and expected outputs from `MINDLINE-WP6-SPEC-V12`. Preserve authority list order as `["DEC-15", "DEC-6", "DEC-12", "DEC-13"]`.

Run:

```bash
test -f testdata/pipeline/inputs/pipeline-text-only.json
test -f testdata/pipeline/expected/pipeline-text-only/pipeline-summary.json
```

Expected: PASS.

## Task 5: Artifact Writer Boundary

- [ ] **Step 1: Write failing artifact writer tests**

Create `internal/pipeline/artifacts/writer_test.go` with tests:

```go
func TestWriterWritesTextOnlyGoldenOutput(t *testing.T) {
	out := t.TempDir()
	summary := goldenTextOnlyPipelineOutput()
	err := artifacts.Write(out, summary)
	require.NoError(t, err)
	repoRoot := findRepoRoot(t)
	assertJSONFile(t, filepath.Join(out, "pipeline-summary.json"), filepath.Join(repoRoot, "testdata/pipeline/expected/pipeline-text-only/pipeline-summary.json"))
	assertJSONFile(t, filepath.Join(out, "results/pipeline-text-only.json"), filepath.Join(repoRoot, "testdata/pipeline/expected/pipeline-text-only/results/pipeline-text-only.json"))
	assertJSONFile(t, filepath.Join(out, "destinations/pipeline-text-only/destination-summary.json"), filepath.Join(repoRoot, "testdata/pipeline/expected/pipeline-text-only/destinations/pipeline-text-only/destination-summary.json"))
	assertFile(t, filepath.Join(out, "destinations/pipeline-text-only/previews/pipeline-text-only.md"), "# Processed source pipeline-text-only\n\n## Snapshot\n\nMindline should keep raw capture, method policy, and destination preview separate.\n")
}

func TestWriterRejectsProtectedTolariaOutputAndSymlinkEscapes(t *testing.T) {
	fs := newMemoryOrOSFixtureFS(t)
	writer := artifacts.NewWriter(fs, []string{"/Users/randyhereman/Young Human Club Dropbox/02. Areas/PKM - Tolaria"})
	err := writer.Write("/Users/randyhereman/Young Human Club Dropbox/02. Areas/PKM - Tolaria", goldenTextOnlyPipelineOutput())
	require.ErrorContains(t, err, "refusing to write pipeline output inside protected Tolaria vault")
}
```

Run:

```bash
go test ./internal/pipeline/artifacts -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 2: Implement artifact writer**

Create `internal/pipeline/artifacts/writer.go`. It is the only WP-7 package allowed to call file creation APIs. It must:

- reject protected output roots and symlink escapes;
- create only under `--out`;
- write JSON with deterministic indentation;
- write all summary paths relative to `--out`;
- use exact slug/collision rules from the spec;
- suppress previews for blocked items;
- reject writes if generated content contains `PRIVATE_DM_SENTINEL_DO_NOT_WRITE` or `sk-test-secret-do-not-leak`.

- [ ] **Step 3: Add static write-boundary test**

Create a test that scans production files under `internal/pipeline`, excluding `internal/pipeline/artifacts/**` and `*_test.go`, for:

```text
os.WriteFile\(|os.Create\(|os.CreateTemp\(|os.OpenFile\(|os.Mkdir\(|os.MkdirAll\(|ioutil.WriteFile\(|io.Copy\(
```

Run:

```bash
go test ./internal/pipeline/artifacts -run TestOnlyArtifactPackageCreatesFiles -count=1
```

Expected: PASS.

## Task 6: Pipeline Runner Composition

- [ ] **Step 1: Write failing runner golden tests**

Create `internal/pipeline/runner_test.go` using the signed fixture matrix. Include one test per valid fixture:

- `pipeline-text-only.json`;
- `pipeline-youtube-url.json`;
- `pipeline-linkedin-with-website.json`;
- `pipeline-pdf-url.json`;
- `pipeline-mixed-links.json`;
- `pipeline-unknown-source.json`;
- `pipeline-private-provenance.json`;
- `pipeline-secret-like.json`;
- `pipeline-slack-export-batch.json`.

Each test must run `pipeline.Run(inputPath, outDir)` and compare:

- exact summary counts;
- exact item ordering;
- exact relative paths;
- exact processor ids, requirement values, statuses, reason strings;
- exact blockers;
- exact authority propagation;
- preview presence or absence;
- sentinel absence in every generated file.

Run:

```bash
go test ./internal/pipeline -run TestRunnerGoldenFixtures -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 1.5: Write failing WP-5 prerequisite test**

Add a runner test proving delivery behavior refuses to run when destination support is unavailable:

```go
func TestRunnerRefusesWhenDestinationPrerequisiteMissing(t *testing.T) {
	runner := pipeline.Runner{
		DestinationAvailable: func(adapterID string) bool { return false },
	}
	_, err := runner.Run("testdata/pipeline/inputs/pipeline-text-only.json", t.TempDir())
	require.ErrorContains(t, err, "WP-5 destination dry-run support is required before pipeline delivery")
}
```

Expected: FAIL before runner supports prerequisite checking.

- [ ] **Step 2: Implement runner orchestration**

Create `internal/pipeline/runner.go`:

```go
func Run(inputPath, outDir string, opts RunOptions) (Summary, error)
```

Implementation sequence:

1. parse input and validate authority before output;
2. load method profile;
3. load source candidates from `candidate`, `candidate_batch`, or `slack_export`;
4. normalize Slack export through `internal/adapters/slack`;
5. process candidates through `internal/sbos`;
6. map SBOS result into `pipeline-result/v0.1`;
7. plan processors with method policy;
8. build destination dry-run input and call `tolaria.Plan` for unblocked items only;
9. build blocked destination summaries for blocked items;
10. call artifact writer;
11. return the same summary written to `pipeline-summary.json`.

Add an injectable prerequisite check so tests can force missing WP-5 behavior. The default check must verify destination adapter id `tolaria` is available before any output is written.

- [ ] **Step 3: Run runner tests**

Run:

```bash
go test ./internal/pipeline -count=1
```

Expected: PASS.

## Task 7: CLI Command

- [ ] **Step 1: Write failing CLI tests**

Create `internal/cli/pipeline_dry_run_test.go` with tests:

```go
func TestPipelineDryRunRequiresFlags(t *testing.T) {
	runner := cli.NewRunner(newMemoryFS())
	exit := runner.Run([]string{"pipeline", "dry-run", "testdata/pipeline/inputs/pipeline-text-only.json"}, io.Discard, &stderr)
	assert.Equal(t, cli.ExitProcess, exit)
	assert.Contains(t, stderr.String(), "missing required --out")
}

func TestPipelineDryRunStdoutMatchesSummaryFile(t *testing.T) {
	out := t.TempDir()
	exit, stdout, stderr := runCLI(t, "pipeline", "dry-run", "testdata/pipeline/inputs/pipeline-text-only.json", "--method", "basb-para-code", "--destination", "tolaria", "--out", out)
	require.Equal(t, cli.ExitOK, exit, stderr)
	require.JSONEq(t, readFile(t, filepath.Join(out, "pipeline-summary.json")), stdout)
}

func TestPipelineDryRunRouteDoesNotCreateFilesDirectly(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(repoRoot(t), "internal/cli/runner.go"))
	require.NoError(t, err)
	pipelineRoute := extractFunctionBody(t, string(body), "runPipeline")
	forbidden := []string{"os.WriteFile(", "os.Create(", "os.CreateTemp(", "os.OpenFile(", "os.Mkdir(", "os.MkdirAll(", "ioutil.WriteFile(", "io.Copy("}
	for _, call := range forbidden {
		assert.NotContains(t, pipelineRoute, call)
	}
}
```

Run:

```bash
go test ./internal/cli -run TestPipelineDryRun -count=1
```

Expected: FAIL before implementation.

- [ ] **Step 2: Implement CLI routing**

Modify `internal/cli/runner.go`:

- add usage line `usage: mindline pipeline dry-run <pipeline-input.json> --method basb-para-code --destination tolaria --out <dir>`;
- route `pipeline dry-run`;
- require `--method`, `--destination`, and `--out`;
- reject unsupported method/destination/run mode with spec error strings;
- reject CLI method/destination mismatches against input envelope;
- print deterministic summary JSON to stdout;
- delegate artifact creation to the pipeline runner/artifact writer and add no direct file creation in the pipeline CLI route;
- return `ExitProcess` for validation/processing failures and `ExitArtifactWrite` for artifact writer failures.

- [ ] **Step 3: Run CLI tests**

Run:

```bash
go test ./internal/cli -run TestPipelineDryRun -count=1
```

Expected: PASS.

## Task 8: README and Static Verification

- [ ] **Step 1: Complete remaining golden expected outputs**

Extend `testdata/pipeline/expected/**` for every valid fixture from the signed matrix, not only `pipeline-text-only`. Preserve authority list order as `["DEC-15", "DEC-6", "DEC-12", "DEC-13"]`.

- [ ] **Step 2: Update README**

Add a section `Local pipeline dry-run` to `README.md` with:

```bash
mindline pipeline dry-run testdata/pipeline/inputs/pipeline-text-only.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp7-output
```

Document:

- dry-run only;
- `basb-para-code` is the first method profile, not core architecture;
- processor routing is planning-only;
- Tolaria is the first destination adapter, not core architecture;
- authority ids are validated and propagated;
- no live Slack/network/browser/LLM/auth/database/Tolaria writes.

- [ ] **Step 3: Run exact verification commands**

Run:

```bash
go test -count=1 ./...
go test -json ./... > /tmp/mindline-wp7-go-test.json
rm -rf /tmp/mindline-wp7-output /tmp/mindline-wp7-private-output /tmp/mindline-wp7-secret-output /tmp/mindline-wp7-stdout.txt /tmp/mindline-wp7-stderr.txt /tmp/mindline-wp7-private-stdout.txt /tmp/mindline-wp7-private-stderr.txt /tmp/mindline-wp7-secret-stdout.txt /tmp/mindline-wp7-secret-stderr.txt
go run ./cmd/mindline pipeline dry-run testdata/pipeline/inputs/pipeline-text-only.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp7-output > /tmp/mindline-wp7-stdout.txt 2> /tmp/mindline-wp7-stderr.txt
go run ./cmd/mindline pipeline dry-run testdata/pipeline/inputs/pipeline-private-provenance.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp7-private-output > /tmp/mindline-wp7-private-stdout.txt 2> /tmp/mindline-wp7-private-stderr.txt
go run ./cmd/mindline pipeline dry-run testdata/pipeline/inputs/pipeline-secret-like.json --method basb-para-code --destination tolaria --out /tmp/mindline-wp7-secret-output > /tmp/mindline-wp7-secret-stdout.txt 2> /tmp/mindline-wp7-secret-stderr.txt
rg -n 'slack\.com/api|net/http|http\.Client|chromedp|playwright|puppeteer|openai|anthropic|claude|convex|supabase|mongodb|mongo\.Connect|clerk|workos|descope|oauth2' internal/pipeline internal/adapters internal/cli internal/destinations internal/sbos -g '!**/*_test.go'
rg -n 'os\.WriteFile\(|os\.Create\(|os\.CreateTemp\(|os\.OpenFile\(|os\.Mkdir\(|os\.MkdirAll\(|ioutil\.WriteFile\(|io\.Copy\(' internal/pipeline -g '!internal/pipeline/artifacts/**' -g '!**/*_test.go'
rg -n 'os\.WriteFile\(|os\.Create\(|os\.CreateTemp\(|os\.OpenFile\(|os\.Mkdir\(|os\.MkdirAll\(|ioutil\.WriteFile\(|io\.Copy\(' internal/cli/runner.go
rg -n 'PARA|CODE|BASB|Snapshot|Source Content|Key Details|Relevance|Signals|Related Sources|Next Action' internal/sbos internal/adapters -g '!**/*_test.go'
rg -n 'PRIVATE_DM_SENTINEL_DO_NOT_WRITE|sk-test-secret-do-not-leak' /tmp/mindline-wp7-output /tmp/mindline-wp7-private-output /tmp/mindline-wp7-secret-output /tmp/mindline-wp7-stdout.txt /tmp/mindline-wp7-stderr.txt /tmp/mindline-wp7-private-stdout.txt /tmp/mindline-wp7-private-stderr.txt /tmp/mindline-wp7-secret-stdout.txt /tmp/mindline-wp7-secret-stderr.txt
```

Expected:

- both `go test` commands pass;
- CLI command exits `0`;
- all five `rg` commands return no matches and exit `1`;
- sentinel scan returns no matches and exits `1`;
- if implementation places pipeline-invoked code outside listed directories, expand the static scan targets before review.

## Review and Close Criteria

- [ ] All tasks above are implemented with TDD red/green evidence.
- [ ] `go test -count=1 ./...` passes.
- [ ] Golden fixture outputs match the signed spec.
- [ ] Static no-live/no-method-leak/no-write-boundary scans pass.
- [ ] Private/secret sentinel scans pass across generated artifacts and stdout/stderr.
- [ ] LOOP delivery review signs off with Chain, Domain/User Job, Systems Architect, Delivery Quality, and Risk/Safety reviewers.
- [ ] Product Brain `WP-7` is updated from `shaping` only after delivery evidence exists.
