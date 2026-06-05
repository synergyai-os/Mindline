package documents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestCorpusPressureGuardrailsSerializeCompleteProofGateFloor(t *testing.T) {
	data, err := json.Marshal(CorpusPressureGuardrailCounters{})
	if err != nil {
		t.Fatalf("marshal guardrails: %v", err)
	}
	var guardrails map[string]any
	if err := json.Unmarshal(data, &guardrails); err != nil {
		t.Fatalf("unmarshal guardrails: %v", err)
	}
	for _, key := range []string{
		"network_fetches",
		"hosted_telemetry_exports",
		"hosted_inference_calls",
		"browser_calls",
		"slack_api_calls",
		"destination_writes",
		"product_brain_writes",
		"tolaria_writes",
		"auto_accepts",
		"no_human_claims",
		"committed_private_artifacts",
	} {
		if _, ok := guardrails[key]; !ok {
			t.Fatalf("guardrails missing %q: %#v", key, guardrails)
		}
	}
}

func TestCorpusPressureBlocksReferenceOnlyOneCandidatePerSourceReadiness(t *testing.T) {
	sources := make([]CorpusPressureSourceResult, 0, 50)
	for i := 0; i < 50; i++ {
		sources = append(sources, CorpusPressureSourceResult{
			SourceID:         "source",
			SourceKind:       SourceKindMarkdown,
			State:            CorpusPressureSourceProcessed,
			ReasonCode:       CorpusPressureReasonNone,
			CandidateCount:   1,
			ObservationCount: 1,
			SegmentCount:     9,
			CandidateKindCounts: map[SemanticCandidateKind]int{
				SemanticCandidateKindReference: 1,
			},
		})
	}
	graph := CorpusGraphSummary{
		AtomCount:              50,
		EvidenceReadyAtomCount: 50,
		RelationCount:          1225,
		ReviewBurdenRatio:      0,
		ReplayFingerprint:      "graph-ready",
	}

	summary := buildCorpusPressureSummary("collapse-corpus", sources, graph, "manifest.json", nil)

	if summary.ReadyForFiftyFilePressure {
		t.Fatalf("reference-only one-candidate-per-source collapse must block readiness: %+v", summary)
	}
	if summary.SemanticReadinessStatus != "blocked" {
		t.Fatalf("expected blocked semantic readiness, got %+v", summary)
	}
	for _, reason := range []string{"reference_only_one_candidate_per_source", "low_observation_to_segment_density"} {
		if !containsCorpusPressureString(summary.SemanticReadinessReasonCodes, reason) {
			t.Fatalf("expected semantic readiness reason %q, got %+v", reason, summary.SemanticReadinessReasonCodes)
		}
	}
	if summary.DocumentSegmentCount != 450 || summary.SemanticObservationCount != 50 || summary.ReferenceCandidateCount != 50 {
		t.Fatalf("expected semantic density counters, got %+v", summary)
	}
	evalInput := corpusPressureEvalInput(summary)
	if evalInput.OneCandidateSourceCount != 50 || evalInput.ReferenceOnlySourceCount != 50 {
		t.Fatalf("eval input must project source-level collapse counters, got %+v", evalInput)
	}
	trace := CorpusPressureTraceSummaryFor(summary, CorpusPressureSourceCounters{})
	if trace.OneCandidateSourceCount != 50 || trace.ReferenceOnlySourceCount != 50 {
		t.Fatalf("trace summary must project source-level collapse counters, got %+v", trace)
	}
	if !containsCorpusPressureString(summary.NextImprovementTargets, "semantic_density") {
		t.Fatalf("expected semantic density target, got %+v", summary.NextImprovementTargets)
	}
}

func TestCorpusPressureDoesNotAddLowDensityReasonWithoutSegments(t *testing.T) {
	sources := make([]CorpusPressureSourceResult, 0, 50)
	for i := 0; i < 50; i++ {
		sources = append(sources, CorpusPressureSourceResult{
			SourceID:         "source",
			SourceKind:       SourceKindMarkdown,
			State:            CorpusPressureSourceProcessed,
			ReasonCode:       CorpusPressureReasonNone,
			CandidateCount:   1,
			ObservationCount: 1,
			CandidateKindCounts: map[SemanticCandidateKind]int{
				SemanticCandidateKindReference: 1,
			},
		})
	}
	graph := CorpusGraphSummary{
		AtomCount:              50,
		EvidenceReadyAtomCount: 50,
		RelationCount:          1225,
		ReplayFingerprint:      "graph-ready",
	}

	summary := buildCorpusPressureSummary("collapse-corpus", sources, graph, "manifest.json", nil)

	if summary.SemanticReadinessStatus != "blocked" {
		t.Fatalf("expected reference-only collapse to remain blocked, got %+v", summary)
	}
	if !containsCorpusPressureString(summary.SemanticReadinessReasonCodes, "reference_only_one_candidate_per_source") {
		t.Fatalf("expected reference-only reason, got %+v", summary.SemanticReadinessReasonCodes)
	}
	if containsCorpusPressureString(summary.SemanticReadinessReasonCodes, "low_observation_to_segment_density") {
		t.Fatalf("zero segment denominator must not produce low-density reason: %+v", summary.SemanticReadinessReasonCodes)
	}
}

func TestCorpusPressureFingerprintIncludesSemanticDensity(t *testing.T) {
	source := CorpusPressureSourceResult{
		SourceID:         "source",
		SourceKind:       SourceKindMarkdown,
		State:            CorpusPressureSourceProcessed,
		ReasonCode:       CorpusPressureReasonNone,
		CandidateCount:   1,
		ObservationCount: 1,
		SegmentCount:     1,
		CandidateKindCounts: map[SemanticCandidateKind]int{
			SemanticCandidateKindReference: 1,
		},
	}
	graph := CorpusGraphSummary{
		AtomCount:              1,
		EvidenceReadyAtomCount: 1,
		ReplayFingerprint:      "graph-ready",
	}

	summaryA := buildCorpusPressureSummary("density-corpus", []CorpusPressureSourceResult{source}, graph, "manifest.json", nil)
	source.SegmentCount = 8
	summaryB := buildCorpusPressureSummary("density-corpus", []CorpusPressureSourceResult{source}, graph, "manifest.json", nil)

	fingerprintA := corpusPressureFingerprint(summaryA)
	fingerprintB := corpusPressureFingerprint(summaryB)
	if fingerprintA == fingerprintB {
		t.Fatalf("semantic density changes must change replay fingerprint: %s", fingerprintA)
	}
}

func TestCorpusPressureDirectoryCorpusIDUsesCanonicalPath(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "source.md"), []byte("# Source\n- capability: keep corpus identity stable across path spellings\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	summaryA, _, err := BuildCorpusPressure(input, t.TempDir(), CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure A: %v", err)
	}
	summaryB, _, err := BuildCorpusPressure(input+string(os.PathSeparator)+".", t.TempDir(), CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure B: %v", err)
	}

	if summaryA.CorpusID != summaryB.CorpusID {
		t.Fatalf("equivalent directory paths must produce the same corpus id: %s != %s", summaryA.CorpusID, summaryB.CorpusID)
	}
	if summaryA.ReplayFingerprint != summaryB.ReplayFingerprint {
		t.Fatalf("equivalent directory paths must produce the same replay fingerprint: %s != %s", summaryA.ReplayFingerprint, summaryB.ReplayFingerprint)
	}
}

func TestCorpusPressureDirectorySourceIdentityUsesCanonicalPath(t *testing.T) {
	input, err := os.MkdirTemp("/tmp", "corpus-pressure-alias-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(input)
	realInput, err := filepath.EvalSymlinks(input)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(realInput) == filepath.Clean(input) {
		t.Skip("platform temp path has no alias to canonicalize")
	}
	if err := os.WriteFile(filepath.Join(input, "source.md"), []byte("# Source\n- capability: keep source identity stable across path aliases\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	summaryAlias, _, err := BuildCorpusPressure(input, t.TempDir(), CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure through alias path: %v", err)
	}
	summaryCanonical, _, err := BuildCorpusPressure(realInput, t.TempDir(), CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure through canonical path: %v", err)
	}

	if summaryAlias.CorpusID != summaryCanonical.CorpusID {
		t.Fatalf("alias and canonical paths must produce same corpus id: %s != %s", summaryAlias.CorpusID, summaryCanonical.CorpusID)
	}
	if summaryAlias.CorpusFingerprint != summaryCanonical.CorpusFingerprint {
		t.Fatalf("alias and canonical paths must produce same corpus fingerprint: %s != %s", summaryAlias.CorpusFingerprint, summaryCanonical.CorpusFingerprint)
	}
	if summaryAlias.ReplayFingerprint != summaryCanonical.ReplayFingerprint {
		t.Fatalf("alias and canonical paths must produce same replay fingerprint: %s != %s", summaryAlias.ReplayFingerprint, summaryCanonical.ReplayFingerprint)
	}
	if len(summaryAlias.Sources) != 1 || len(summaryCanonical.Sources) != 1 {
		t.Fatalf("expected one source each: alias=%+v canonical=%+v", summaryAlias.Sources, summaryCanonical.Sources)
	}
	if summaryAlias.Sources[0].SourceID != summaryCanonical.Sources[0].SourceID || summaryAlias.Sources[0].SourceLabel != summaryCanonical.Sources[0].SourceLabel {
		t.Fatalf("alias and canonical paths must produce same source identity: alias=%+v canonical=%+v", summaryAlias.Sources[0], summaryCanonical.Sources[0])
	}
}

func TestCorpusPressureLLMClassifierCountsHostedInferenceGuardrail(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "source.md"), []byte("# Source\n- capability: count hosted inference calls when LLM classification runs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	provider := &fakeLLMSemanticProvider{}

	summary, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{
		SemanticOptions: SemanticOptions{
			Classifier:  SemanticClassifierLLM,
			LLMProvider: "openai",
			LLMModel:    "fake-model",
			LLMAPIKey:   "fake-key",
			LLMClient:   provider,
		},
	})
	if err != nil {
		t.Fatalf("build corpus pressure with LLM classifier: %v", err)
	}

	if provider.calls == 0 {
		t.Fatalf("test expected hosted provider to be called")
	}
	if summary.Guardrails.HostedInferenceCalls != provider.calls {
		t.Fatalf("summary must count hosted inference calls: got %+v calls=%d", summary.Guardrails, provider.calls)
	}
	var persisted CorpusPressureSummary
	readCorpusPressureJSON(t, filepath.Join(out, CorpusPressureDirName, "pressure-summary.json"), &persisted)
	if persisted.Guardrails.HostedInferenceCalls != provider.calls {
		t.Fatalf("persisted summary must count hosted inference calls: got %+v calls=%d", persisted.Guardrails, provider.calls)
	}
	var evalInput CorpusPressureEvalInput
	readCorpusPressureJSON(t, filepath.Join(out, CorpusPressureDirName, "eval-input.json"), &evalInput)
	if evalInput.Guardrails.HostedInferenceCalls != provider.calls {
		t.Fatalf("eval input must count hosted inference calls: got %+v calls=%d", evalInput.Guardrails, provider.calls)
	}
	var trace CorpusPressureTraceSummary
	readCorpusPressureJSON(t, filepath.Join(out, CorpusPressureDirName, "trace-summary.json"), &trace)
	if trace.Guardrails.HostedInferenceCalls != provider.calls {
		t.Fatalf("trace summary must count hosted inference calls: got %+v calls=%d", trace.Guardrails, provider.calls)
	}
}

func TestCorpusPressureLLMClassifierUsesRawSourceTextAfterSegmentBudgetScreening(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "source.md"), []byte("# Source\n- capability: raw LLM evidence survives prebuilt structure reuse\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeLLMSemanticProvider{}

	if _, _, err := BuildCorpusPressure(input, t.TempDir(), CorpusPressureOptions{
		SemanticOptions: SemanticOptions{
			Classifier:  SemanticClassifierLLM,
			LLMProvider: "openai",
			LLMModel:    "fake-model",
			LLMAPIKey:   "fake-key",
			LLMClient:   provider,
		},
		ScaleBudget: CorpusPressureScaleBudget{
			MaxProcessedSources:     10,
			MaxSourceBytes:          1024 * 1024,
			MaxSourceSegments:       200,
			MaxGraphPairComparisons: 250000,
			MaxGraphRelations:       50000,
			MaxPacketReviewGroups:   50,
		},
	}); err != nil {
		t.Fatalf("build corpus pressure with LLM classifier: %v", err)
	}
	var requestText string
	for _, node := range provider.request.Nodes {
		requestText += " " + node.Text
	}
	if !strings.Contains(requestText, "raw LLM evidence survives prebuilt structure reuse") {
		t.Fatalf("expected LLM request to include raw source text, got %q", requestText)
	}
	if strings.Contains(requestText, "Document structure root") {
		t.Fatalf("LLM request used generated structure summary instead of raw source text: %q", requestText)
	}
}

type multiCandidateLLMSemanticProvider struct {
	request LLMSemanticRequest
}

func (provider *multiCandidateLLMSemanticProvider) Classify(request LLMSemanticRequest) (llmSemanticResponse, error) {
	provider.request = request
	evidenceNode := ""
	if len(request.Nodes) > 0 {
		evidenceNode = request.Nodes[0].NodeID
	}
	return llmSemanticResponse{Candidates: []llmSemanticCandidate{
		{
			Kind:          string(SemanticCandidateKindAction),
			Title:         "Prepare bounded evidence",
			Summary:       "Prepare bounded evidence from the cited source.",
			Confidence:    string(ConfidenceMedium),
			EvidenceNodes: []string{evidenceNode},
		},
		{
			Kind:          string(SemanticCandidateKindRequirement),
			Title:         "Require bounded evidence",
			Summary:       "Require bounded evidence from the cited source.",
			Confidence:    string(ConfidenceMedium),
			EvidenceNodes: []string{evidenceNode},
		},
	}}, nil
}

func TestCorpusPressureFailsWhenOutputSourcesPathIsNotDirectory(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "source.md"), []byte("# Source\n- capability: fail instead of hanging on invalid output sources path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "sources"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected invalid output sources path to fail")
		}
		if !strings.Contains(err.Error(), "sources") {
			t.Fatalf("expected actionable sources-path error, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("build corpus pressure hung while assigning source run directories")
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

func TestCorpusPressureInPlaceOutputKeepsRealSourcesDirectory(t *testing.T) {
	input := t.TempDir()
	sourceDir := filepath.Join(input, "sources")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "real.md"), []byte("# Real source\n- capability: preserve corpus inputs stored under sources\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, _, err := BuildCorpusPressure(input, input, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure in place with real sources dir: %v", err)
	}
	if summary.SourceCount != 1 {
		t.Fatalf("in-place output must not skip real input documents under sources/: %+v", summary)
	}
}

func TestCorpusPressureInPlaceOutputKeepsRealSourceMarkdownWithAnalysisSiblings(t *testing.T) {
	input := t.TempDir()
	sourceDir := filepath.Join(input, "sources", "real")
	if err := os.MkdirAll(filepath.Join(sourceDir, "semantic-candidates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "source.md"), []byte("# Real source\n- capability: preserve source.md layouts under sources\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, _, err := BuildCorpusPressure(input, input, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure in place with real source.md layout: %v", err)
	}
	if summary.SourceCount != 1 {
		t.Fatalf("in-place output must not skip real source.md documents with analysis siblings: %+v", summary)
	}
}

func TestCorpusPressureInPlaceOutputDoesNotOverwriteRealSourcePathCollision(t *testing.T) {
	input := t.TempDir()
	topLevelSource := []byte("# Top level\n- capability: process top-level source without overwriting nested source\n")
	if err := os.WriteFile(filepath.Join(input, "foo.md"), topLevelSource, 0o644); err != nil {
		t.Fatal(err)
	}
	realSourceDir := filepath.Join(input, "sources", "foo")
	if err := os.MkdirAll(realSourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realSource := []byte("# Real nested source\n- capability: preserve legitimate sources/foo/source.md input\n")
	realSourcePath := filepath.Join(realSourceDir, "source.md")
	if err := os.WriteFile(realSourcePath, realSource, 0o644); err != nil {
		t.Fatal(err)
	}

	realInput, err := filepath.EvalSymlinks(input)
	if err != nil {
		t.Fatal(err)
	}
	summary, _, err := BuildCorpusPressure(realInput, realInput, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure in place with colliding real source path: %v", err)
	}
	if summary.SourceCount != 2 {
		t.Fatalf("in-place output must keep both colliding input sources: %+v", summary)
	}
	data, err := os.ReadFile(realSourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(realSource) {
		t.Fatalf("in-place output overwrote real source path: got %q want %q", string(data), string(realSource))
	}
	if _, err := os.Stat(filepath.Join(realSourceDir, corpusPressureGeneratedSourceMarker)); err == nil {
		t.Fatalf("in-place output marked real source directory as generated")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestCorpusPressureInPlaceOutputDoesNotWriteIntoRealSourceDirectoryCollision(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "foo.md"), []byte("# Top level\n- capability: process top-level source outside real source directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	realSourceDir := filepath.Join(input, "sources", "foo")
	if err := os.MkdirAll(realSourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realSourceDir, "notes.md"), []byte("# Nested source\n- capability: preserve real source directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	realInput, err := filepath.EvalSymlinks(input)
	if err != nil {
		t.Fatal(err)
	}
	summary, _, err := BuildCorpusPressure(realInput, realInput, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure in place with real source directory collision: %v", err)
	}
	if summary.SourceCount != 2 {
		t.Fatalf("in-place output must keep both input sources: %+v", summary)
	}
	if _, err := os.Stat(filepath.Join(realSourceDir, "source.md")); err == nil {
		t.Fatalf("in-place output wrote generated source into real source directory")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(realSourceDir, corpusPressureGeneratedSourceMarker)); err == nil {
		t.Fatalf("in-place output marked real source directory as generated")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(input, "sources", "foo-pressure", "source.md")); err != nil {
		t.Fatalf("generated source should be disambiguated into foo-pressure: %v", err)
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

	changedBudget := base
	changedBudget.PressureOptions.ScaleBudget = CorpusPressureScaleBudget{
		MaxProcessedSources:     25,
		MaxSourceBytes:          DefaultCorpusPressureMaxSourceBytes,
		MaxSourceSegments:       DefaultCorpusPressureMaxSourceSegments,
		MaxSourceCandidates:     DefaultCorpusPressureMaxSourceCandidates,
		MaxGraphPairComparisons: DefaultCorpusPressureMaxGraphPairComparisons,
		MaxGraphRelations:       DefaultCorpusPressureMaxGraphRelations,
		MaxPacketReviewGroups:   DefaultCorpusPressureMaxPacketReviewGroups,
	}
	if corpusPressureLoopConfigFingerprint(base) == corpusPressureLoopConfigFingerprint(changedBudget) {
		t.Fatalf("loop fingerprints must include normalized scale budgets")
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

func TestCorpusPressureFallbackReviewDoesNotBecomeSourceBlocker(t *testing.T) {
	input := t.TempDir()
	body := "# Fallback\n\nDecision:\n- Keep the rollout local until review evidence is ready.\n"
	if err := os.WriteFile(filepath.Join(input, "fallback.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()

	summary, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}

	if summary.BlockedSourceCount != 0 {
		t.Fatalf("fallback review artifact must not block source processing: %+v", summary)
	}
	if len(summary.Sources) != 1 || summary.Sources[0].State == CorpusPressureSourceBlocked {
		t.Fatalf("fallback source must be accounted without source-level block: %+v", summary.Sources)
	}
	var segmentSummary Summary
	readCorpusPressureJSON(t, filepath.Join(out, filepath.FromSlash(summary.Sources[0].SemanticRunDir), "document-segments", "segment-summary.json"), &segmentSummary)
	if segmentSummary.NeedsReviewCount == 0 {
		t.Fatalf("fallback source must preserve visible segment review burden: %+v", segmentSummary)
	}
}

func TestCorpusPressureEvalInputPropagatesGuardrails(t *testing.T) {
	summary := CorpusPressureSummary{
		CorpusID: "corpus-test",
		Guardrails: CorpusPressureGuardrailCounters{
			HostedInferenceCalls:   2,
			HostedTelemetryExports: 1,
			DestinationWrites:      3,
		},
	}

	evalInput := corpusPressureEvalInput(summary)

	if evalInput.Guardrails != summary.Guardrails {
		t.Fatalf("eval input must preserve guardrail counters: %+v", evalInput.Guardrails)
	}
	trace := CorpusPressureTraceSummaryFor(summary, CorpusPressureSourceCounters{})
	if trace.Guardrails != summary.Guardrails {
		t.Fatalf("trace summary must preserve guardrail counters: %+v", trace.Guardrails)
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

func TestCorpusPressureWritesScalePartialSummaryWhenSourceLimitReached(t *testing.T) {
	input := t.TempDir()
	for i := 1; i <= 3; i++ {
		path := filepath.Join(input, fmt.Sprintf("source-%02d.md", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("# Source %02d\n- capability: bounded scale proof %02d\n", i, i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	summary, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{ScaleBudget: CorpusPressureScaleBudget{
		MaxProcessedSources:     2,
		MaxSourceBytes:          1024 * 1024,
		MaxSourceSegments:       200,
		MaxGraphPairComparisons: 250000,
		MaxGraphRelations:       50000,
		MaxPacketReviewGroups:   50,
	}})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	if summary.ScaleStatus != "scale_partial" || !containsCorpusPressureString(summary.ScaleReasonCodes, "scale_source_limit") {
		t.Fatalf("expected scale source limit, got %+v", summary)
	}
	if summary.ProcessedSourceCount != 2 || summary.SkippedSourceCount != 1 || summary.ScaleSkippedSourceCount != 1 {
		t.Fatalf("unexpected source accounting: %+v", summary)
	}
	if summary.ReadyForFiftyFilePressure {
		t.Fatalf("scale partial run must not be ready: %+v", summary)
	}
	if !containsCorpusPressureString(summary.NextImprovementTargets, "scale_capacity") {
		t.Fatalf("expected scale capacity target, got %+v", summary.NextImprovementTargets)
	}
	if _, err := os.Stat(filepath.Join(out, CorpusPressureDirName, "pressure-summary.json")); err != nil {
		t.Fatalf("expected final pressure summary: %v", err)
	}
}

func TestCorpusPressureSkipsSourcesBeforeSemanticExplosionWhenScaleBudgetsHit(t *testing.T) {
	input := t.TempDir()
	large := strings.Repeat("large source body\n", 20)
	if err := os.WriteFile(filepath.Join(input, "large.md"), []byte("# Large\n"+large), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, "many-segments.md"), []byte("# Many\n\n## A\nalpha\n\n## B\nbeta\n\n## C\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, "small.md"), []byte("# Small\n- capability: bounded source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, _, err := BuildCorpusPressure(input, t.TempDir(), CorpusPressureOptions{ScaleBudget: CorpusPressureScaleBudget{
		MaxProcessedSources:     10,
		MaxSourceBytes:          80,
		MaxSourceSegments:       2,
		MaxGraphPairComparisons: 250000,
		MaxGraphRelations:       50000,
		MaxPacketReviewGroups:   50,
	}})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	if summary.ScaleStatus != "scale_partial" {
		t.Fatalf("expected scale partial, got %+v", summary)
	}
	for _, reason := range []string{"scale_source_size_limit", "scale_segment_limit"} {
		if !containsCorpusPressureString(summary.ScaleReasonCodes, reason) {
			t.Fatalf("expected scale reason %q, got %+v", reason, summary.ScaleReasonCodes)
		}
	}
	if summary.ProcessedSourceCount != 1 || summary.ScaleSkippedSourceCount != 2 {
		t.Fatalf("expected one processed and two scale skipped sources, got %+v", summary)
	}
}

func TestCorpusPressureCountsOnlyProcessedSourcesAgainstSourceLimit(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "source-01-large.md"), []byte("# Large\n"+strings.Repeat("oversized source body\n", 20)), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= 3; i++ {
		if err := os.WriteFile(filepath.Join(input, fmt.Sprintf("source-%02d-small.md", i)), []byte(fmt.Sprintf("# Small %02d\n- capability: process after skipped source\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	summary, _, err := BuildCorpusPressure(input, t.TempDir(), CorpusPressureOptions{ScaleBudget: CorpusPressureScaleBudget{
		MaxProcessedSources:     1,
		MaxSourceBytes:          80,
		MaxSourceSegments:       200,
		MaxSourceCandidates:     500,
		MaxGraphPairComparisons: 250000,
		MaxGraphRelations:       50000,
		MaxPacketReviewGroups:   50,
	}})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	if summary.ProcessedSourceCount != 1 {
		t.Fatalf("source cap should count processed sources only, got %+v", summary)
	}
	for _, reason := range []string{"scale_source_size_limit", "scale_source_limit"} {
		if !containsCorpusPressureString(summary.ScaleReasonCodes, reason) {
			t.Fatalf("expected scale reason %q, got %+v", reason, summary.ScaleReasonCodes)
		}
	}
}

func TestCorpusPressureStopsBeforeWritingSemanticArtifactsWhenCandidateBudgetIsHit(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "source.md"), []byte("# Source\n- capability: candidate budget should stop semantic artifact writes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	provider := &multiCandidateLLMSemanticProvider{}
	summary, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{
		SemanticOptions: SemanticOptions{
			Classifier:  SemanticClassifierLLM,
			LLMProvider: "openai",
			LLMModel:    "fake-model",
			LLMAPIKey:   "fake-key",
			LLMClient:   provider,
		},
		ScaleBudget: CorpusPressureScaleBudget{
			MaxProcessedSources:     10,
			MaxSourceBytes:          1024 * 1024,
			MaxSourceSegments:       200,
			MaxSourceCandidates:     1,
			MaxGraphPairComparisons: 250000,
			MaxGraphRelations:       50000,
			MaxPacketReviewGroups:   50,
		},
	})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	if summary.ScaleStatus != "scale_partial" || !containsCorpusPressureString(summary.ScaleReasonCodes, "scale_candidate_limit") {
		t.Fatalf("expected scale candidate limit, got %+v", summary)
	}
	if summary.ProcessedSourceCount != 0 || summary.ScaleSkippedSourceCount != 1 {
		t.Fatalf("candidate-limited source should be scale skipped, got %+v", summary)
	}
	source := summary.Sources[0]
	if source.ReasonCode != CorpusPressureReasonScaleCandidateLimit {
		t.Fatalf("expected source reason scale_candidate_limit, got %+v", source)
	}
	semanticRoot := filepath.Join(out, source.SemanticRunDir, "semantic-candidates")
	if candidates, err := filepath.Glob(filepath.Join(semanticRoot, "candidates", "*.json")); err != nil || len(candidates) != 0 {
		t.Fatalf("candidate budget should not write candidate artifacts: paths=%v err=%v", candidates, err)
	}
}

func TestCorpusPressureOversizedSkippedSourceContentChangesCorpusFingerprint(t *testing.T) {
	build := func(body string) CorpusPressureSummary {
		t.Helper()
		input := t.TempDir()
		if err := os.WriteFile(filepath.Join(input, "oversized.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		summary, _, err := BuildCorpusPressure(input, t.TempDir(), CorpusPressureOptions{ScaleBudget: CorpusPressureScaleBudget{
			MaxProcessedSources:     10,
			MaxSourceBytes:          1,
			MaxSourceSegments:       200,
			MaxSourceCandidates:     500,
			MaxGraphPairComparisons: 250000,
			MaxGraphRelations:       50000,
			MaxPacketReviewGroups:   50,
		}})
		if err != nil {
			t.Fatalf("build corpus pressure: %v", err)
		}
		return summary
	}
	left := build("# Oversized\nleft skipped content\n")
	right := build("# Oversized\nright skipped content\n")
	if left.Sources[0].SourceContentHash == "" || right.Sources[0].SourceContentHash == "" {
		t.Fatalf("oversized skipped sources must still carry content hashes: left=%+v right=%+v", left.Sources[0], right.Sources[0])
	}
	if left.CorpusFingerprint == right.CorpusFingerprint {
		t.Fatalf("different oversized skipped source content must change corpus fingerprint: %s", left.CorpusFingerprint)
	}
}

func TestCorpusPressureSourceLimitSkippedTailChangesCorpusFingerprint(t *testing.T) {
	build := func(tail string) CorpusPressureSummary {
		t.Helper()
		input := t.TempDir()
		if err := os.WriteFile(filepath.Join(input, "source-01.md"), []byte("# Source 01\n- capability: processed source\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(input, "source-02.md"), []byte(tail), 0o644); err != nil {
			t.Fatal(err)
		}
		summary, _, err := BuildCorpusPressure(input, t.TempDir(), CorpusPressureOptions{ScaleBudget: CorpusPressureScaleBudget{
			MaxProcessedSources:     1,
			MaxSourceBytes:          1024 * 1024,
			MaxSourceSegments:       200,
			MaxSourceCandidates:     500,
			MaxGraphPairComparisons: 250000,
			MaxGraphRelations:       50000,
			MaxPacketReviewGroups:   50,
		}})
		if err != nil {
			t.Fatalf("build corpus pressure: %v", err)
		}
		return summary
	}
	left := build("# Source 02\nleft skipped tail\n")
	right := build("# Source 02\nright skipped tail\n")
	if left.Sources[1].ReasonCode != CorpusPressureReasonScaleSourceLimit || left.Sources[1].SourceContentHash == "" {
		t.Fatalf("expected source-limit tail hash, got %+v", left.Sources[1])
	}
	if left.CorpusFingerprint == right.CorpusFingerprint {
		t.Fatalf("different source-limit tail content must change corpus fingerprint: %s", left.CorpusFingerprint)
	}
}

func TestCorpusPressureTracePreservesGraphFailureWhenScalePartial(t *testing.T) {
	stages := corpusPressureTraceStages(CorpusPressureSummary{
		SourceCount:               2,
		ScaleStatus:               "scale_partial",
		SemanticCandidateCount:    1,
		ReadyForFiftyFilePressure: false,
		Blockers:                  []string{"corpus graph failed: write failed"},
	})
	for _, stage := range stages {
		if stage.Name == "corpus_graph" && stage.Status != "failed" {
			t.Fatalf("graph failure must not be downgraded to scale_partial: %+v", stages)
		}
	}
}

func TestCorpusPressureOversizedSkippedSourceMarksGeneratedDirForReruns(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "oversized.md"), []byte("# Oversized\n"+strings.Repeat("body\n", 20)), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	options := CorpusPressureOptions{ScaleBudget: CorpusPressureScaleBudget{
		MaxProcessedSources:     10,
		MaxSourceBytes:          1,
		MaxSourceSegments:       200,
		MaxSourceCandidates:     500,
		MaxGraphPairComparisons: 250000,
		MaxGraphRelations:       50000,
		MaxPacketReviewGroups:   50,
	}}
	for i := 0; i < 2; i++ {
		if _, _, err := BuildCorpusPressure(input, out, options); err != nil {
			t.Fatalf("build corpus pressure rerun %d: %v", i+1, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(out, "sources"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "oversized" {
		t.Fatalf("oversized rerun should reuse marked generated dir, got %+v", entries)
	}
}

func TestCorpusPressureScaleSkippedSourcesDoNotAdvertiseMissingSourceArtifacts(t *testing.T) {
	input := t.TempDir()
	for i := 1; i <= 2; i++ {
		if err := os.WriteFile(filepath.Join(input, fmt.Sprintf("source-%02d.md", i)), []byte(fmt.Sprintf("# Source %02d\n- capability: source path contract\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	summary, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{ScaleBudget: CorpusPressureScaleBudget{
		MaxProcessedSources:     1,
		MaxSourceBytes:          1024 * 1024,
		MaxSourceSegments:       200,
		MaxGraphPairComparisons: 250000,
		MaxGraphRelations:       50000,
		MaxPacketReviewGroups:   50,
	}})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	for _, source := range summary.Sources {
		if source.State != CorpusPressureSourceSkipped || !corpusPressureReasonIsScale(source.ReasonCode) {
			continue
		}
		if source.SourcePath != "" {
			if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(source.SourcePath))); os.IsNotExist(err) {
				t.Fatalf("scale-skipped source advertised missing source artifact: %+v", source)
			}
		}
	}
}

func TestCorpusPressureAllScaleSkippedSourcesDoNotAdvertiseMissingGraphArtifacts(t *testing.T) {
	input := t.TempDir()
	for i := 1; i <= 2; i++ {
		if err := os.WriteFile(filepath.Join(input, fmt.Sprintf("source-%02d.md", i)), []byte(fmt.Sprintf("# Source %02d\n- capability: graph path contract\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	summary, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{ScaleBudget: CorpusPressureScaleBudget{
		MaxProcessedSources:     10,
		MaxSourceBytes:          1,
		MaxSourceSegments:       200,
		MaxGraphPairComparisons: 250000,
		MaxGraphRelations:       50000,
		MaxPacketReviewGroups:   50,
	}})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	if summary.ProcessedSourceCount != 0 || summary.ScaleSkippedSourceCount != 2 {
		t.Fatalf("expected all sources scale skipped, got %+v", summary)
	}
	if summary.GraphSummaryPath != "" {
		t.Fatalf("all-skipped run must not advertise missing graph artifacts: %+v", summary)
	}
	if summary.GraphManifestPath != "" {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(summary.GraphManifestPath))); err != nil {
			t.Fatalf("advertised graph manifest must exist: %v", err)
		}
	}
	if _, _, err := BuildSourceMeaningPreview(out, t.TempDir()); err != nil {
		t.Fatalf("source meaning preview should tolerate all-skipped pressure runs: %v", err)
	}
	if _, _, err := BuildSourceMeaningPacket(out, t.TempDir()); err != nil {
		t.Fatalf("source meaning packet should tolerate all-skipped pressure runs: %v", err)
	}
}

func TestCorpusPressureUsesRawSourceTextAfterSegmentBudgetScreening(t *testing.T) {
	input := t.TempDir()
	if err := os.WriteFile(filepath.Join(input, "raw.md"), []byte("# Raw\n- capability: raw source evidence survives prebuilt structure reuse\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	summary, _, err := BuildCorpusPressure(input, out, CorpusPressureOptions{ScaleBudget: CorpusPressureScaleBudget{
		MaxProcessedSources:     10,
		MaxSourceBytes:          1024 * 1024,
		MaxSourceSegments:       200,
		MaxGraphPairComparisons: 250000,
		MaxGraphRelations:       50000,
		MaxPacketReviewGroups:   50,
	}})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	if summary.ProcessedSourceCount != 1 {
		t.Fatalf("expected one processed source, got %+v", summary)
	}
	var observationFound bool
	for _, source := range summary.Sources {
		if source.State != CorpusPressureSourceProcessed {
			continue
		}
		var semanticSummary SemanticSummary
		semanticRoot := filepath.Join(out, source.SemanticRunDir, "semantic-candidates")
		readCorpusPressureJSON(t, filepath.Join(semanticRoot, "semantic-summary.json"), &semanticSummary)
		observationPaths, err := filepath.Glob(filepath.Join(semanticRoot, "observations", "*.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, observationPath := range observationPaths {
			var observation SemanticObservation
			readCorpusPressureJSON(t, observationPath, &observation)
			if strings.Contains(observation.Summary, "raw source evidence survives prebuilt structure reuse") {
				observationFound = true
			}
			if strings.Contains(observation.Summary, "Document structure root") {
				t.Fatalf("semantic extraction used generated structure summary instead of raw source text: %+v", observation)
			}
		}
	}
	if !observationFound {
		t.Fatalf("expected semantic observation to preserve raw source text")
	}
}

func containsCorpusPressureString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
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
