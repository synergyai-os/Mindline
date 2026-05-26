package documents

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusPressureBuildsReadableReportAndReplay(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "documents", "semantic")
	outA := t.TempDir()
	outB := t.TempDir()
	outC := t.TempDir()
	summaryA, _, err := BuildCorpusPressure(input, outA, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure A: %v", err)
	}
	summaryB, _, err := BuildCorpusPressure(input, outB, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure B: %v", err)
	}
	summaryC, _, err := BuildCorpusPressure(input, outC, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure C: %v", err)
	}
	if summaryA.SourceCount != 3 || summaryA.ProcessedSourceCount != 3 || summaryA.SkippedSourceCount != 0 || summaryA.BlockedSourceCount != 0 {
		t.Fatalf("unexpected source accounting: %+v", summaryA)
	}
	if summaryA.ProcessedSourceRatio != 1 {
		t.Fatalf("expected fully processed fixture corpus: %+v", summaryA)
	}
	if summaryA.SemanticCandidateCount == 0 || summaryA.GraphAtomCount == 0 {
		t.Fatalf("expected semantic candidates and graph atoms: %+v", summaryA)
	}
	if summaryA.ReplayFingerprint != summaryB.ReplayFingerprint || summaryA.ReplayFingerprint != summaryC.ReplayFingerprint {
		t.Fatalf("pressure replay changed: %s %s %s", summaryA.ReplayFingerprint, summaryB.ReplayFingerprint, summaryC.ReplayFingerprint)
	}
	reportData, err := os.ReadFile(filepath.Join(outA, CorpusPressureDirName, "pressure-report.md"))
	if err != nil {
		t.Fatalf("read pressure report: %v", err)
	}
	report := string(reportData)
	for _, want := range []string{
		"## Corpus answer",
		"## Source accounting",
		"## Extracted candidates by source",
		"## Connected clusters",
		"## Duplicate candidates",
		"## Contradiction candidates",
		"## Evidence/readiness failures",
		"## Eval/trace artifact pointers",
		"## Next improvement targets",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
	if summaryA.EvidenceReadyAtomCount < summaryA.GraphAtomCount && !strings.Contains(report, "evidence_incomplete_atom") {
		t.Fatalf("report must name evidence-incomplete atoms when readiness fails:\n%s", report)
	}
	var evalInput CorpusPressureEvalInput
	readCorpusPressureJSON(t, filepath.Join(outA, CorpusPressureDirName, "eval-input.json"), &evalInput)
	if evalInput.SchemaVersion != CorpusPressureEvalInputSchemaVersion || evalInput.SourceCounters.Processed != summaryA.ProcessedSourceCount {
		t.Fatalf("unexpected eval input: %+v", evalInput)
	}
	var trace CorpusPressureTraceSummary
	readCorpusPressureJSON(t, filepath.Join(outA, CorpusPressureDirName, "trace-summary.json"), &trace)
	if trace.SchemaVersion != CorpusPressureTraceSchemaVersion {
		t.Fatalf("unexpected trace schema: %+v", trace)
	}
	if trace.SourceCounters.Processed != summaryA.ProcessedSourceCount || trace.SourceCounters.Skipped != summaryA.SkippedSourceCount || trace.SourceCounters.Blocked != summaryA.BlockedSourceCount || trace.SourceCounters.Excluded != summaryA.ExcludedSourceCount {
		t.Fatalf("trace must expose source-state counters: %+v", trace.SourceCounters)
	}
	if trace.Guardrails.HostedInferenceCalls != 0 || trace.Guardrails.HostedTelemetryExports != 0 || trace.Guardrails.DestinationWrites != 0 {
		t.Fatalf("default pressure trace must have zero hosted/destination counters: %+v", trace.Guardrails)
	}
}

func TestCorpusPressureLoopStopsHonestlyWhenUnchanged(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "blocked.md"), []byte("# Secret\nAPI key sk-test-secret-token should stay blocked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	summary, err := BuildCorpusPressureLoop(input, out, CorpusPressureLoopOptions{MaxRuns: 20, BuildFingerprint: "test-build"})
	if err != nil {
		t.Fatalf("build corpus pressure loop: %v", err)
	}
	if summary.KRPassed {
		t.Fatalf("fixture should not claim raised KRs pass unless evidence ratio is high enough: %+v", summary)
	}
	if summary.StopReason != "same_binary_same_inputs" {
		t.Fatalf("expected honest no-change stop, got %+v", summary)
	}
	if summary.RunCount != 2 {
		t.Fatalf("expected baseline plus no-change confirmation, got %d", summary.RunCount)
	}
	if summary.Iterations[1].SourceDeltas.Processed != 0 || summary.Iterations[1].SourceDeltas.Skipped != 0 || summary.Iterations[1].SourceDeltas.Excluded != 0 || summary.Iterations[1].SourceDeltas.Blocked != 0 {
		t.Fatalf("expected zero source-state deltas for unchanged run: %+v", summary.Iterations[1].SourceDeltas)
	}
	if _, err := os.Stat(filepath.Join(out, CorpusPressureLoopDirName, "loop-summary.json")); err != nil {
		t.Fatalf("missing loop summary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, CorpusPressureLoopDirName, "loop-report.md")); err != nil {
		t.Fatalf("missing loop report: %v", err)
	}
}

func TestCorpusPressureLoopIgnoresNestedOutputSources(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "blocked.md"), []byte("# Secret\nAPI key sk-test-secret-token should stay blocked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(input, "out")
	summary, err := BuildCorpusPressureLoop(input, out, CorpusPressureLoopOptions{MaxRuns: 3, BuildFingerprint: "test-build"})
	if err != nil {
		t.Fatalf("build corpus pressure loop: %v", err)
	}
	if summary.StopReason != "same_binary_same_inputs" {
		t.Fatalf("nested loop output should not be rediscovered as corpus input: %+v", summary)
	}
	if summary.RunCount != 2 {
		t.Fatalf("expected baseline plus stable replay, got %d", summary.RunCount)
	}
	if summary.Iterations[0].SourceCounters.Total != 1 || summary.Iterations[1].SourceCounters.Total != 1 {
		t.Fatalf("generated nested outputs must not inflate source counts: %+v", summary.Iterations)
	}
	if summary.Iterations[1].SourceDeltas.Blocked != 0 || summary.Iterations[1].SourceDeltas.Processed != 0 || summary.Iterations[1].SourceDeltas.Skipped != 0 || summary.Iterations[1].SourceDeltas.Excluded != 0 {
		t.Fatalf("expected zero source-state deltas for nested output replay: %+v", summary.Iterations[1].SourceDeltas)
	}
}

func TestCorpusPressureLoopInPlaceOutputKeepsInputSources(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "blocked.md"), []byte("# Secret\nAPI key sk-test-secret-token should stay blocked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := BuildCorpusPressureLoop(input, input, CorpusPressureLoopOptions{MaxRuns: 3, BuildFingerprint: "test-build"})
	if err != nil {
		t.Fatalf("build corpus pressure loop in place: %v", err)
	}
	if summary.StopReason != "same_binary_same_inputs" {
		t.Fatalf("in-place loop output should not hide inputs or rediscover generated output: %+v", summary)
	}
	if summary.RunCount != 2 {
		t.Fatalf("expected baseline plus stable replay, got %d", summary.RunCount)
	}
	if summary.Iterations[0].SourceCounters.Total != 1 || summary.Iterations[1].SourceCounters.Total != 1 {
		t.Fatalf("in-place loop output must preserve source counts: %+v", summary.Iterations)
	}
}

func TestCorpusPressureInPlaceOutputKeepsInputSources(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "blocked.md"), []byte("# Secret\nAPI key sk-test-secret-token should stay blocked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, _, err := BuildCorpusPressure(input, input, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure in place: %v", err)
	}
	if summary.SourceCount != 1 {
		t.Fatalf("in-place output must not exclude the input corpus: %+v", summary)
	}
	replayed, _, err := BuildCorpusPressure(input, input, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("replay corpus pressure in place: %v", err)
	}
	if replayed.SourceCount != 1 {
		t.Fatalf("in-place replay must not rediscover generated source copies: %+v", replayed)
	}
}

func TestCorpusPressureLoopStopReasonUsesEffectiveMaxRuns(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "blocked.md"), []byte("# Secret\nAPI key sk-test-secret-token should stay blocked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := BuildCorpusPressureLoop(input, t.TempDir(), CorpusPressureLoopOptions{MaxRuns: 1, BuildFingerprint: "test-build"})
	if err != nil {
		t.Fatalf("build corpus pressure loop: %v", err)
	}
	if summary.RunCount != 1 {
		t.Fatalf("expected one configured run, got %d", summary.RunCount)
	}
	if summary.StopReason != "stopped_after_1" {
		t.Fatalf("stop reason should reflect configured max runs, got %+v", summary)
	}
}

func TestCorpusPressureDeterministicFingerprintIgnoresUnusedLLMSettings(t *testing.T) {
	base := SemanticOptions{
		Classifier:        SemanticClassifierDeterministic,
		LLMProvider:       "openai",
		LLMModel:          "gpt-a",
		ReferenceFallback: true,
	}
	changedLLM := SemanticOptions{
		Classifier:        SemanticClassifierDeterministic,
		LLMProvider:       "other-provider",
		LLMModel:          "gpt-b",
		ReferenceFallback: true,
	}
	if corpusPressureCommandConfigFingerprint(base) != corpusPressureCommandConfigFingerprint(changedLLM) {
		t.Fatalf("deterministic pressure fingerprints must ignore unused LLM settings")
	}

	llmA := base
	llmA.Classifier = SemanticClassifierLLM
	llmB := changedLLM
	llmB.Classifier = SemanticClassifierLLM
	if corpusPressureCommandConfigFingerprint(llmA) == corpusPressureCommandConfigFingerprint(llmB) {
		t.Fatalf("LLM pressure fingerprints must include provider and model")
	}
}

func TestCorpusPressureLoopFingerprintUsesEffectiveConfig(t *testing.T) {
	base := CorpusPressureLoopOptions{
		MaxRuns: 0,
		PressureOptions: CorpusPressureOptions{SemanticOptions: SemanticOptions{
			Classifier:  SemanticClassifierDeterministic,
			LLMProvider: "openai",
			LLMModel:    "gpt-a",
		}},
	}
	sameEffective := CorpusPressureLoopOptions{
		MaxRuns: 20,
		PressureOptions: CorpusPressureOptions{SemanticOptions: SemanticOptions{
			Classifier:  SemanticClassifierDeterministic,
			LLMProvider: "other-provider",
			LLMModel:    "gpt-b",
		}},
	}
	if corpusPressureLoopConfigFingerprint(base) != corpusPressureLoopConfigFingerprint(sameEffective) {
		t.Fatalf("loop fingerprints must normalize max-runs and ignore unused deterministic LLM settings")
	}

	capped := base
	capped.MaxRuns = 100
	if corpusPressureLoopConfigFingerprint(base) != corpusPressureLoopConfigFingerprint(capped) {
		t.Fatalf("loop fingerprints must hash the capped effective max-runs value")
	}
}

func TestCorpusPressureLoopKRRequiresFullPressureReadiness(t *testing.T) {
	summary := CorpusPressureSummary{
		SourceCount:               10,
		EligibleSourceCount:       10,
		ProcessedSourceCount:      10,
		SkippedSourceCount:        0,
		BlockedSourceCount:        0,
		UnexplainedExclusionCount: 0,
		ProcessedSourceRatio:      1,
		GraphAtomCount:            10,
		EvidenceReadyAtomCount:    10,
		EvidenceReadyAtomRatio:    1,
		GraphReplayFingerprint:    "graph-ready",
		ReviewBurdenRatio:         0.21,
		ReadyForFiftyFilePressure: false,
	}
	if corpusPressureLoopKRPassed(summary) {
		t.Fatalf("loop KRs must not pass when pressure readiness fails review burden threshold")
	}
	summary.ReviewBurdenRatio = 0.20
	if corpusPressureLoopKRPassed(summary) {
		t.Fatalf("loop KRs must not pass when persisted pressure readiness is false")
	}
	summary.ReadyForFiftyFilePressure = true
	if !corpusPressureLoopKRPassed(summary) {
		t.Fatalf("loop KRs should pass when pressure readiness and source accounting pass")
	}
	summary.GraphReplayFingerprint = ""
	summary.ReadyForFiftyFilePressure = false
	if corpusPressureLoopKRPassed(summary) {
		t.Fatalf("loop KRs must not pass without graph replay proof")
	}
}

func TestCorpusPressureTraceMarksGraphFailure(t *testing.T) {
	stages := corpusPressureTraceStages(CorpusPressureSummary{
		SourceCount:               1,
		SemanticCandidateCount:    1,
		GraphAtomCount:            1,
		Blockers:                  []string{"corpus graph failed: write corpus graph"},
		ReadyForFiftyFilePressure: false,
	})

	for _, stage := range stages {
		if stage.Name == "corpus_graph" {
			if stage.Status != "failed" {
				t.Fatalf("corpus_graph stage should fail when graph build/write fails: %+v", stage)
			}
			return
		}
	}
	t.Fatalf("missing corpus_graph stage: %+v", stages)
}

func TestCorpusPressureRaisedKRsDoNotCountSkippedAsProcessed(t *testing.T) {
	summary := CorpusPressureSummary{
		SourceCount:               10,
		EligibleSourceCount:       10,
		ProcessedSourceCount:      9,
		SkippedSourceCount:        1,
		BlockedSourceCount:        0,
		UnexplainedExclusionCount: 0,
		ProcessedSourceRatio:      0.90,
		GraphAtomCount:            10,
		EvidenceReadyAtomCount:    10,
		EvidenceReadyAtomRatio:    1,
		GraphReplayFingerprint:    "graph-ready",
	}
	if corpusPressureLoopKRPassed(summary) {
		t.Fatalf("skipped sources must not count as processed or improvement")
	}
	summary.ProcessedSourceCount = 10
	summary.SkippedSourceCount = 0
	summary.EligibleSourceCount = 10
	summary.ProcessedSourceRatio = 1
	summary.ReadyForFiftyFilePressure = true
	if !corpusPressureLoopKRPassed(summary) {
		t.Fatalf("expected raised KRs to pass when all counted sources are processed and evidence-ready")
	}
}

func TestCorpusPressureClosedExclusionsLeaveEligibleRatioHonest(t *testing.T) {
	summary := CorpusPressureSummary{
		SourceCount:               10,
		EligibleSourceCount:       9,
		ProcessedSourceCount:      9,
		ExcludedSourceCount:       1,
		BlockedSourceCount:        0,
		UnexplainedExclusionCount: 0,
		ProcessedSourceRatio:      1,
		GraphAtomCount:            9,
		EvidenceReadyAtomCount:    9,
		EvidenceReadyAtomRatio:    1,
		GraphReplayFingerprint:    "graph-ready",
		ReadyForFiftyFilePressure: true,
	}
	if !corpusPressureLoopKRPassed(summary) {
		t.Fatalf("closed exclusions should stay visible without lowering eligible processed ratio")
	}
	summary.UnexplainedExclusionCount = 1
	summary.ReadyForFiftyFilePressure = false
	if corpusPressureLoopKRPassed(summary) {
		t.Fatalf("unexplained exclusions must block raised KRs")
	}
}

func TestCorpusPressureSummaryDoesNotEmitTargetsWhenReady(t *testing.T) {
	source := CorpusPressureSourceResult{
		SourceID:       "ready-source",
		SourceKind:     "markdown",
		State:          CorpusPressureSourceProcessed,
		ReasonCode:     CorpusPressureReasonNone,
		CandidateCount: 1,
	}
	graph := CorpusGraphSummary{
		AtomCount:              1,
		RelationCount:          1,
		EvidenceReadyAtomCount: 1,
		ReviewBurdenRatio:      0,
		ReplayFingerprint:      "graph-ready",
	}

	summary := buildCorpusPressureSummary("ready-corpus", []CorpusPressureSourceResult{source}, graph, "manifest.json", nil)

	if !summary.ReadyForFiftyFilePressure {
		t.Fatalf("healthy pressure summary should be ready: %+v", summary)
	}
	if len(summary.NextImprovementTargets) != 0 {
		t.Fatalf("ready pressure summary must not emit improvement targets: %+v", summary.NextImprovementTargets)
	}
}

func TestCorpusPressureTargetsSuppressRelationCoverageWhenReady(t *testing.T) {
	summary := CorpusPressureSummary{
		SourceCount:               1,
		EligibleSourceCount:       1,
		ProcessedSourceCount:      1,
		ProcessedSourceRatio:      1,
		GraphAtomCount:            1,
		GraphRelationCount:        0,
		EvidenceReadyAtomCount:    1,
		EvidenceReadyAtomRatio:    1,
		GraphReplayFingerprint:    "graph-ready",
		ReviewBurdenRatio:         0,
		ReadyForFiftyFilePressure: true,
	}

	targets := corpusPressureTargets(summary)

	if len(targets) != 0 {
		t.Fatalf("ready pressure summary must not emit relation coverage target: %+v", targets)
	}
}

func TestCorpusPressureSummaryTreatsGraphFailureAsNotReady(t *testing.T) {
	source := CorpusPressureSourceResult{
		SourceID:       "ready-source",
		SourceKind:     "markdown",
		State:          CorpusPressureSourceProcessed,
		ReasonCode:     CorpusPressureReasonNone,
		CandidateCount: 1,
	}
	graph := CorpusGraphSummary{
		AtomCount:              1,
		RelationCount:          1,
		EvidenceReadyAtomCount: 1,
		ReviewBurdenRatio:      0,
		ReplayFingerprint:      "graph-ready",
	}

	summary := buildCorpusPressureSummary("ready-corpus", []CorpusPressureSourceResult{source}, graph, "manifest.json", errors.New("write graph"))

	if summary.ReadyForFiftyFilePressure {
		t.Fatalf("graph failure must block pressure readiness: %+v", summary)
	}
	if len(summary.NextImprovementTargets) == 0 {
		t.Fatalf("graph failure should leave an improvement target: %+v", summary)
	}
}

func TestCorpusPressureDoesNotPromoteReviewRequiredCandidatesWithEvidence(t *testing.T) {
	sourceRoot := t.TempDir()
	semanticRoot := filepath.Join(sourceRoot, "semantic-candidates")
	if err := os.MkdirAll(semanticRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	candidate := SemanticCandidate{
		SchemaVersion:  SemanticCandidateSchemaVersion,
		CandidateID:    "candidate-review-required",
		RunID:          "run-test",
		CandidateKind:  SemanticCandidateKindDecision,
		ReviewStatus:   ReviewStatusNeedsReview,
		Confidence:     ConfidenceLow,
		Title:          "Ambiguous decision",
		Summary:        "Needs human review",
		EvidenceNodes:  []string{"node-1"},
		EvidenceRanges: []SemanticEvidenceRange{{StructureNodeID: "node-1", LineStart: 1, LineEnd: 2}},
		Blockers:       []Blocker{{Code: "semantic_review_required", Message: "Candidate requires review because evidence is weak, contradicted, or ambiguous."}},
	}
	if err := writeJSON(semanticRoot, SemanticCandidateJSONPath(candidate.CandidateID), candidate); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	summary := BuildSemanticSummary("run-test", 1, nil, []SemanticCandidate{candidate}, nil)

	promoted := promoteCorpusPressureEvidenceReadiness(sourceRoot, summary)

	if promoted.NeedsReviewCount != 1 {
		t.Fatalf("review-required candidate must remain review burden: %+v", promoted)
	}
	var loaded SemanticCandidate
	readCorpusPressureJSON(t, filepath.Join(semanticRoot, SemanticCandidateJSONPath(candidate.CandidateID)), &loaded)
	if loaded.ReviewStatus != ReviewStatusNeedsReview || loaded.Confidence != ConfidenceLow || len(loaded.Blockers) != 1 {
		t.Fatalf("review-required candidate should not be rewritten as ready: %+v", loaded)
	}
}

func TestCorpusPressureCorpusFingerprintIncludesSourceContent(t *testing.T) {
	input := t.TempDir()
	source := filepath.Join(input, "source.md")
	if err := os.WriteFile(source, []byte("# Source\n- capability: first version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, _, err := BuildCorpusPressure(input, t.TempDir(), CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build first pressure: %v", err)
	}
	if err := os.WriteFile(source, []byte("# Source\n- capability: second version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, _, err := BuildCorpusPressure(input, t.TempDir(), CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build second pressure: %v", err)
	}
	if first.CorpusFingerprint == "" || second.CorpusFingerprint == "" {
		t.Fatalf("expected corpus fingerprints: first=%q second=%q", first.CorpusFingerprint, second.CorpusFingerprint)
	}
	if first.CorpusFingerprint == second.CorpusFingerprint {
		t.Fatalf("corpus fingerprint must change when source content changes: %s", first.CorpusFingerprint)
	}
}

func TestCorpusPressureManifestRejectsEscapingSource(t *testing.T) {
	root := t.TempDir()
	manifest := `{
  "schema_version": "corpus-pressure-manifest/v0.1",
  "corpus_id": "bad-corpus",
  "sources": [{"source_id":"bad","source_kind":"markdown","path":"../outside.md"}]
}`
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := BuildCorpusPressure(manifestPath, t.TempDir(), CorpusPressureOptions{}); err == nil || !strings.Contains(err.Error(), "escaped") {
		t.Fatalf("expected containment error, got %v", err)
	}
}

func TestCorpusPressureDirectoryRejectsSymlinkSourceEscape(t *testing.T) {
	root := t.TempDir()
	in := filepath.Join(root, "in")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(in, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("# Outside\nsecret outside root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(in, "leak.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, _, err := BuildCorpusPressure(in, t.TempDir(), CorpusPressureOptions{}); err == nil || !strings.Contains(err.Error(), "escaped") {
		t.Fatalf("expected symlink containment error, got %v", err)
	}
}

func TestCorpusPressureRejectsOutputSourceSymlinkEscape(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "documents", "semantic")
	out := t.TempDir()
	escaped := filepath.Join(t.TempDir(), "escaped")
	if err := os.MkdirAll(filepath.Join(out, "sources"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(escaped, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escaped, filepath.Join(out, "sources", "process-capability-evidence")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{}); err != nil {
		t.Fatalf("build corpus pressure should account for blocked source and continue: %v", err)
	}
	if _, err := os.Stat(filepath.Join(escaped, "source.md")); err == nil {
		t.Fatalf("source copy escaped output root through symlink")
	}
}

func TestCorpusPressureRejectsPressureReportSymlinkEscape(t *testing.T) {
	input := filepath.Join("..", "..", "testdata", "documents", "semantic")
	out := t.TempDir()
	escaped := filepath.Join(t.TempDir(), "escaped-pressure")
	if err := os.MkdirAll(escaped, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(escaped, filepath.Join(out, CorpusPressureDirName)); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{}); err == nil || !strings.Contains(err.Error(), "escaped") {
		t.Fatalf("expected pressure report symlink escape error, got %v", err)
	}
	for _, file := range []string{"pressure-summary.json", "pressure-report.md"} {
		if _, err := os.Stat(filepath.Join(escaped, file)); err == nil {
			t.Fatalf("pressure artifact escaped output root through symlink: %s", file)
		}
	}
}

func readCorpusPressureJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
