# WP-17 Provider-Agnostic LLM Classifier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an optional OpenAI-backed semantic classifier behind a provider-neutral Mindline interface, then measure it against the existing WP-15/WP-16 harness.

**Architecture:** Keep deterministic semantics as the default path. Add a provider-neutral `documents` classifier port and provider registry; OpenAI is the first adapter only. CLI config resolves `.env.local`/environment before reading source content so missing key/provider/model fails closed.

**Tech Stack:** Go standard library only (`net/http`, `encoding/json`, `os`, `strings`), existing Mindline document semantic artifacts, existing CLI runner tests.

---

### Task 1: CLI Contract And Config Fail-Closed

**Files:**
- Modify: `internal/cli/runner.go`
- Modify: `internal/cli/documents_decompose_test.go`
- Modify: `.env.local.example`

- [ ] **Step 1: Write failing tests**

Add tests proving:

```go
func TestDocumentsSemanticsLLMRequiresConfiguredProviderBeforeSourceRead(t *testing.T) {
	fs := newMemoryFS()
	fs.files["temp/private.md"] = []byte("# Private\nsource")
	runner := NewRunner(fs)
	var stdout, stderr bytes.Buffer

	code := runner.Run([]string{"documents", "semantics", "temp/private.md", "--out", "out", "--classifier", "llm", "--llm-provider", "openai"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d", code)
	}
	if !strings.Contains(stderr.String(), "missing OpenAI model") {
		t.Fatalf("expected missing model before source read, got %q", stderr.String())
	}
}
```

```go
func TestDocumentsSemanticsRejectsUnsupportedLLMProviderBeforeSourceRead(t *testing.T) {
	fs := newMemoryFS()
	fs.files["temp/private.md"] = []byte("# Private\nsource")
	runner := NewRunner(fs)
	var stdout, stderr bytes.Buffer

	code := runner.Run([]string{"documents", "semantics", "temp/private.md", "--out", "out", "--classifier", "llm", "--llm-provider", "gemini", "--llm-model", "gemini-test"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("expected usage exit, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unsupported LLM provider: gemini") {
		t.Fatalf("expected unsupported provider, got %q", stderr.String())
	}
}
```

- [ ] **Step 2: Run test to verify RED**

Run:

```sh
go test -count=1 ./internal/cli -run 'TestDocumentsSemanticsLLM'
```

Expected: fails because `--classifier`, `--llm-provider`, and `--llm-model` are unsupported.

- [ ] **Step 3: Implement minimal parser/config**

Add CLI parsing for:

```text
--classifier deterministic|llm
--llm-provider openai
--llm-model <model>
```

Load `.env.local` from current working directory if present and use it only for missing config values. Never print keys.

- [ ] **Step 4: Run GREEN**

Run:

```sh
go test -count=1 ./internal/cli -run 'TestDocumentsSemanticsLLM'
```

Expected: pass.

### Task 2: Provider-Neutral Classifier Port

**Files:**
- Create: `internal/documents/llm_classifier.go`
- Modify: `internal/documents/types.go`
- Modify: `internal/documents/documents_test.go`
- Modify: `internal/documents/semantic.go`

- [ ] **Step 1: Write failing tests**

Add tests proving provider output is rejected when it invents evidence:

```go
func TestLLMClassifierRejectsInventedEvidenceNode(t *testing.T) {
	nodes := []StructureNode{{NodeID: "node-real", SourceDocumentID: "doc-test", Evidence: StructureEvidence{LineStart: 1, LineEnd: 1}}}
	response := llmSemanticResponse{Candidates: []llmSemanticCandidate{{Kind: string(SemanticCandidateKindAction), Title: "Do it", Summary: "Do it", Confidence: string(ConfidenceMedium), EvidenceNodes: []string{"node-fake"}}}}

	_, _, err := buildLLMSemanticArtifacts("run-test", nodes, response)

	if err == nil || !strings.Contains(err.Error(), "unknown evidence node: node-fake") {
		t.Fatalf("expected unknown evidence node rejection, got %v", err)
	}
}
```

And a test proving valid provider output becomes normal Mindline artifacts with unresolved destination status.

- [ ] **Step 2: Run RED**

Run:

```sh
go test -count=1 ./internal/documents -run 'TestLLMClassifier'
```

Expected: fails because LLM classifier types/functions do not exist.

- [ ] **Step 3: Implement minimal port and artifact builder**

Create provider-neutral request/response structs and validation. Build candidates and `derived_from` relations using existing `newSemanticCandidate` and `newSemanticRelation` patterns. Do not persist provider raw payloads.

- [ ] **Step 4: Run GREEN**

Run:

```sh
go test -count=1 ./internal/documents -run 'TestLLMClassifier'
```

Expected: pass.

### Task 3: OpenAI Adapter

**Files:**
- Create: `internal/documents/openai_provider.go`
- Modify: `internal/documents/documents_test.go`

- [ ] **Step 1: Write failing fake-client tests**

Add a fake HTTP client/round tripper test proving:

- request goes to `/v1/responses`
- authorization header is set but never persisted
- response output text JSON is parsed into provider-neutral candidates
- malformed provider output fails closed

- [ ] **Step 2: Run RED**

Run:

```sh
go test -count=1 ./internal/documents -run 'TestOpenAI'
```

Expected: fails because OpenAI provider does not exist.

- [ ] **Step 3: Implement minimal OpenAI Responses adapter**

Use `net/http`. POST a Responses API request with model and input. Parse `output[].content[].text` as JSON. Return provider-neutral candidate data only.

- [ ] **Step 4: Run GREEN**

Run:

```sh
go test -count=1 ./internal/documents -run 'TestOpenAI'
```

Expected: pass.

### Task 4: End-to-End LLM Semantics

**Files:**
- Modify: `internal/documents/semantic.go`
- Modify: `internal/cli/runner.go`
- Modify: `internal/cli/documents_decompose_test.go`

- [ ] **Step 1: Write failing integration test**

Use a fake provider in documents package to prove `SemanticPathWithOptions(... Classifier=llm ...)` writes normal `semantic-candidates` artifacts and keeps deterministic mode unchanged.

- [ ] **Step 2: Run RED**

Run:

```sh
go test -count=1 ./internal/documents ./internal/cli -run 'LLM|Semantics'
```

Expected: fails until options are wired through.

- [ ] **Step 3: Wire options through**

Add `SemanticPathWithOptions(inputPath, outDir, options)` and keep `SemanticPath` as deterministic wrapper.

- [ ] **Step 4: Run GREEN**

Run:

```sh
go test -count=1 ./internal/documents ./internal/cli -run 'LLM|Semantics'
```

Expected: pass.

### Task 5: Temp Corpus Verification And Iteration

**Files:**
- Create: `scripts/wp17-temp-corpus-verify.mjs` or use a temporary verifier under `/private/tmp`
- Do not commit private temp outputs.

- [ ] **Step 1: Run deterministic baseline**

Run `mindline documents semantics` for every direct `temp/*.md`.

- [ ] **Step 2: Run LLM mode**

Run the same files with:

```sh
--classifier llm --llm-provider openai --llm-model gpt-5.2
```

- [ ] **Step 3: Compare**

Compare candidate count, candidate kinds, needs-review count, blocked count, source/evidence validity, and whether transcript outputs are more interpretable than deterministic baseline.

- [ ] **Step 4: Iterate**

Improve prompts/schema validation until no meaningful gain is visible on the corpus without weakening evidence requirements.

### Task 6: Final Review And PR

**Files:**
- Update Chain and PR metadata only after verification.

- [ ] **Step 1: Full verification**

Run:

```sh
go test -count=1 ./...
git diff --check
```

- [ ] **Step 2: Multi-judge review**

Run at least three independent LLM judge reviews over final temp corpus evidence and require `SIGN-OFF`.

- [ ] **Step 3: Commit and PR**

Commit the code/spec/plan/template changes, push the branch, and open a ready-for-review PR.

