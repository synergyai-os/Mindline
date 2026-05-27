package documents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusAcceptanceTinyHeldOutSuiteCannotPassEligibility(t *testing.T) {
	root, pressure, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, nil)
	answerKey.MinEvalCount = 1
	answerKey.CoverageRequirements.MinSourceCount = 1
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.SuiteValid || summary.DEC64Eligible || summary.Accuracy != 1 {
		t.Fatalf("expected tiny held-out suite to be blocked despite perfect accuracy, got valid=%t eligible=%t accuracy=%.2f blockers=%v suite=%v pressure=%s", summary.SuiteValid, summary.DEC64Eligible, summary.Accuracy, summary.EligibilityBlockers, summary.SuiteValidityBlockers, pressure.ReplayFingerprint)
	}
	if !stringListContains(summary.SuiteValidityBlockers, "below_dec64_min_eval_count") || !stringListContains(summary.EligibilityBlockers, "below_dec64_min_eval_count") {
		t.Fatalf("expected DEC-64 min eval blockers, got suite=%v eligibility=%v", summary.SuiteValidityBlockers, summary.EligibilityBlockers)
	}
	reportData, err := os.ReadFile(filepath.Join(root, "benchmark", corpusAcceptanceDirName, "benchmark-report.md"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	report := string(reportData)
	if strings.Contains(report, "Lead will prepare") || strings.Contains(report, root) {
		t.Fatalf("benchmark report leaked private content or absolute path: %s", report)
	}
}

func TestCorpusAcceptanceExpectedAbsentCountsAsCorrect(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, nil, nil)
	answerKey.MinEvalCount = 1
	answerKey.Sources[0].ExpectedOutcomes[0].ExpectedState = ExpectedOutcomeAbsent
	answerKey.Sources[0].ExpectedOutcomes[0].RequiredEvidence = nil
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.MatchedExpectedCount != 1 || summary.Accuracy != 1 || summary.Sources[0].Accuracy != 1 || summary.FalsePositiveCount != 0 || summary.FalseNegativeCount != 0 {
		t.Fatalf("expected absent outcome to count as correct negative control, got matched=%d accuracy=%.2f source=%.2f fp=%d fn=%d", summary.MatchedExpectedCount, summary.Accuracy, summary.Sources[0].Accuracy, summary.FalsePositiveCount, summary.FalseNegativeCount)
	}
}

func TestCorpusAcceptanceDerivesMissingSourceDocumentIDFromSemanticArtifacts(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, nil)
	answerKey.MinEvalCount = 1
	answerKey.Sources[0].SourceDocumentID = ""
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.MatchedExpectedCount != 1 || summary.FalseNegativeCount != 0 || summary.Sources[0].Accuracy != 1 {
		t.Fatalf("expected omitted source_document_id to derive from semantic artifacts, got matched=%d fn=%d source_accuracy=%.2f", summary.MatchedExpectedCount, summary.FalseNegativeCount, summary.Sources[0].Accuracy)
	}
}

func TestCorpusAcceptanceMissingSourceDocumentIDDoesNotHideUnexpectedCandidates(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, nil)
	answerKey.MinEvalCount = 1
	answerKey.Sources[0].SourceDocumentID = ""
	answerKey.Sources[0].ExpectedOutcomes[0].ExpectedState = ExpectedOutcomeAbsent
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.FalsePositiveCount != 1 || summary.MatchedExpectedCount != 0 || summary.Sources[0].Accuracy != 0 {
		t.Fatalf("expected omitted source_document_id to expose unexpected artifact candidates, got fp=%d matched=%d source_accuracy=%.2f", summary.FalsePositiveCount, summary.MatchedExpectedCount, summary.Sources[0].Accuracy)
	}
}

func TestCorpusAcceptanceEvaluatesNoCandidateNegativeControls(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, nil, func(summary *CorpusPressureSummary) {
		summary.Sources[0].State = CorpusPressureSourceSkipped
		summary.Sources[0].ReasonCode = CorpusPressureReasonNoSemanticCandidates
	})
	answerKey.MinEvalCount = 1
	answerKey.Sources[0].ExpectedOutcomes[0].ExpectedState = ExpectedOutcomeAbsent
	answerKey.Sources[0].ExpectedOutcomes[0].RequiredEvidence = nil
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.UnjudgedCount != 0 || summary.MatchedExpectedCount != 1 || summary.FalseNegativeCount != 0 || summary.Sources[0].Accuracy != 1 {
		t.Fatalf("no-candidate negative control should be judged correct, got unjudged=%d matched=%d fn=%d source_accuracy=%.2f blockers=%v", summary.UnjudgedCount, summary.MatchedExpectedCount, summary.FalseNegativeCount, summary.Sources[0].Accuracy, summary.Sources[0].Blockers)
	}
}

func TestCorpusAcceptanceCalibrationAndTinySuitesCannotPassEligibility(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, nil)
	answerKey.SuiteKind = CorpusAcceptanceSuiteCalibration
	answerKey.MinEvalCount = 5
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.DEC64Eligible {
		t.Fatalf("calibration suite should not pass eligibility")
	}
	if !stringListContains(summary.SuiteValidityBlockers, "below_min_eval_count") || !stringListContains(summary.SuiteValidityBlockers, "held_out_option_requires_held_out_suite") {
		t.Fatalf("expected tiny calibration blockers, got %v", summary.SuiteValidityBlockers)
	}
}

func TestCorpusAcceptanceBlocksWrongKindFalseTrust(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindCapability, ReviewStatusReady)}, nil)
	answerKey.MinEvalCount = 1
	answerKey.CoverageRequirements.FailureModes = append(answerKey.CoverageRequirements.FailureModes, SemanticAcceptanceReasonWrongKind)
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.WrongKindCount != 1 || !stringListContains(summary.EligibilityBlockers, "wrong_kind") {
		t.Fatalf("expected wrong-kind blocker, got wrong=%d blockers=%v", summary.WrongKindCount, summary.EligibilityBlockers)
	}
	if stringListContains(summary.SuiteValidityBlockers, "missing_failure_mode_coverage:wrong_kind") {
		t.Fatalf("wrong-kind coverage should be satisfiable for expected-present outcomes, got %v", summary.SuiteValidityBlockers)
	}
	if summary.FalseNegativeCount == 0 || summary.FalsePositiveCount == 0 {
		t.Fatalf("wrong-kind candidate should also expose missed expected and unexpected candidate, got fp=%d fn=%d", summary.FalsePositiveCount, summary.FalseNegativeCount)
	}
}

func TestCorpusAcceptanceRelationMissesDoNotInvalidateSuite(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, nil)
	answerKey.MinEvalCount = 1
	answerKey.CoverageRequirements.RelationTypes = []SemanticRelationshipType{SemanticRelationshipDerivedFrom}
	graphDir := filepath.Join(root, CorpusGraphDirName)
	if err := os.MkdirAll(graphDir, 0o755); err != nil {
		t.Fatalf("mkdir graph: %v", err)
	}
	writeDocumentsTestJSON(t, filepath.Join(graphDir, "graph-summary.json"), CorpusGraphSummary{
		SchemaVersion: CorpusGraphSummarySchemaVersion,
		RelationMetrics: CorpusRelationMetrics{
			EvalCountedRelationCount: 0,
			FalseNegativeCount:       1,
		},
	})
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if stringListContains(summary.SuiteValidityBlockers, "relation_answer_key_missing") {
		t.Fatalf("relation misses should not be treated as a missing answer key, got blockers=%v", summary.SuiteValidityBlockers)
	}
	if summary.FalseNegativeCount != 1 || !stringListContains(summary.EligibilityBlockers, "false_negative") {
		t.Fatalf("expected relation miss to count as false-negative eligibility blocker, got fn=%d blockers=%v", summary.FalseNegativeCount, summary.EligibilityBlockers)
	}
}

func TestCorpusAcceptanceRelationFailuresDoNotDistortCandidateRates(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, nil)
	answerKey.MinEvalCount = 1
	graphDir := filepath.Join(root, CorpusGraphDirName)
	if err := os.MkdirAll(graphDir, 0o755); err != nil {
		t.Fatalf("mkdir graph: %v", err)
	}
	writeDocumentsTestJSON(t, filepath.Join(graphDir, "graph-summary.json"), CorpusGraphSummary{
		SchemaVersion: CorpusGraphSummarySchemaVersion,
		RelationMetrics: CorpusRelationMetrics{
			EvalCountedRelationCount: 3,
			FalsePositiveCount:       2,
			FalseNegativeCount:       1,
		},
	})
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.FalsePositiveCount != 2 || summary.FalseNegativeCount != 1 {
		t.Fatalf("relation failures should remain visible in counts, got fp=%d fn=%d", summary.FalsePositiveCount, summary.FalseNegativeCount)
	}
	if summary.FalsePositiveRate != 0 || summary.FalseNegativeRate != 0 {
		t.Fatalf("relation failures should not distort candidate/outcome rates, got fp_rate=%.2f fn_rate=%.2f", summary.FalsePositiveRate, summary.FalseNegativeRate)
	}
}

func TestCorpusAcceptanceRejectsGeneratedRunProvenance(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, nil)
	answerKey.Provenance.Independence = "generated_from_evaluated_run"
	answerKey.MinEvalCount = 1
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.SuiteValid || !stringListContains(summary.SuiteValidityBlockers, "answer_key_not_independent") {
		t.Fatalf("expected independent-provenance blocker, got valid=%t blockers=%v", summary.SuiteValid, summary.SuiteValidityBlockers)
	}
}

func TestCorpusAcceptanceRejectsInvalidExpectedOutcome(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, nil)
	answerKey.MinEvalCount = 1
	answerKey.Sources[0].ExpectedOutcomes[0].RequiredEvidence = nil
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.SuiteValid || !stringListContains(summary.SuiteValidityBlockers, "invalid_expected_outcomes:source-demo") {
		t.Fatalf("expected invalid expected-outcome blocker, got valid=%t blockers=%v", summary.SuiteValid, summary.SuiteValidityBlockers)
	}
}

func TestCorpusAcceptanceCountsSemanticErrorAsModelError(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, func(summary *CorpusPressureSummary) {
		summary.Sources[0].State = CorpusPressureSourceBlocked
		summary.Sources[0].ReasonCode = CorpusPressureReasonSemanticError
	})
	answerKey.MinEvalCount = 1
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.ModelErrorCount != 1 || !stringListContains(summary.EligibilityBlockers, "model_error") || !stringListContains(summary.EligibilityBlockers, "unjudged") {
		t.Fatalf("expected model-error/unjudged blockers, got model=%d blockers=%v", summary.ModelErrorCount, summary.EligibilityBlockers)
	}
}

func TestCorpusAcceptanceBlockedSourceIncludesArtifactPath(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, func(summary *CorpusPressureSummary) {
		summary.Sources[0].State = CorpusPressureSourceBlocked
		summary.Sources[0].ReasonCode = CorpusPressureReasonSemanticError
	})
	answerKey.MinEvalCount = 1
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.Sources[0].AcceptanceSummaryPath != filepath.ToSlash(filepath.Join(corpusAcceptanceDirName, "sources", "source-demo", "acceptance-summary.json")) {
		t.Fatalf("blocked source should include discoverable artifact path, got %q", summary.Sources[0].AcceptanceSummaryPath)
	}
	if _, err := os.Stat(filepath.Join(root, "benchmark", summary.Sources[0].AcceptanceSummaryPath)); err != nil {
		t.Fatalf("expected per-source artifact at summary path: %v", err)
	}
}

func TestCorpusAcceptanceRejectsEscapingSemanticRunDir(t *testing.T) {
	root, _, answerKey := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, func(summary *CorpusPressureSummary) {
		summary.Sources[0].SemanticRunDir = "../outside"
	})
	answerKey.MinEvalCount = 1
	writeDocumentsTestJSON(t, filepath.Join(root, "answer-key.json"), answerKey)

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.UnjudgedCount != 1 || !stringListContains(summary.EligibilityBlockers, "unjudged") {
		t.Fatalf("expected escaping semantic run dir to be unjudged, got unjudged=%d blockers=%v", summary.UnjudgedCount, summary.EligibilityBlockers)
	}
}

func writeCorpusAcceptanceFixture(t *testing.T, candidates []SemanticCandidate, mutatePressure func(*CorpusPressureSummary)) (string, CorpusPressureSummary, CorpusAcceptanceAnswerKey) {
	t.Helper()
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "sources", "source-demo")
	if err := os.MkdirAll(filepath.Dir(sourceRoot), 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}
	semanticRun := writeSemanticAcceptanceRun(t, candidates)
	if err := os.Rename(semanticRun, sourceRoot); err != nil {
		t.Fatalf("move semantic run: %v", err)
	}
	pressure := CorpusPressureSummary{
		SchemaVersion:            CorpusPressureSummarySchemaVersion,
		CorpusID:                 "corpus-demo",
		SourceCount:              1,
		EligibleSourceCount:      1,
		ProcessedSourceCount:     1,
		SemanticCandidateCount:   len(candidates),
		CommandConfigFingerprint: "config-demo",
		CorpusFingerprint:        "corpus-fingerprint-demo",
		ReplayFingerprint:        "pressure-fingerprint-demo",
		GraphReplayFingerprint:   "graph-fingerprint-demo",
		GraphSummaryPath:         "corpus-graph/graph-summary.json",
		Guardrails:               CorpusPressureGuardrailCounters{},
		NextImprovementTargets:   []string{},
		Sources: []CorpusPressureSourceResult{{
			SourceID:          "source-demo",
			SourceKind:        SourceKindMarkdown,
			State:             CorpusPressureSourceProcessed,
			ReasonCode:        CorpusPressureReasonNone,
			CandidateCount:    len(candidates),
			SemanticRunID:     "run-sem-demo",
			SourceContentHash: "sha256:fixture",
			SourcePath:        "sources/source-demo/source.md",
			SemanticRunDir:    "sources/source-demo",
		}},
	}
	if mutatePressure != nil {
		mutatePressure(&pressure)
	}
	if err := os.MkdirAll(filepath.Join(root, CorpusPressureDirName), 0o755); err != nil {
		t.Fatalf("mkdir pressure: %v", err)
	}
	writeDocumentsTestJSON(t, filepath.Join(root, CorpusPressureDirName, "pressure-summary.json"), pressure)
	answerKey := CorpusAcceptanceAnswerKey{
		SchemaVersion:     CorpusAcceptanceAnswerKeySchemaVersion,
		SuiteID:           "heldout-demo",
		SuiteKind:         CorpusAcceptanceSuiteHeldOut,
		Provenance:        CorpusAcceptanceProvenance{Labeler: "fixture-human", Independence: corpusAcceptanceIndependentProvenance},
		CorpusID:          pressure.CorpusID,
		CorpusFingerprint: pressure.CorpusFingerprint,
		MinEvalCount:      1,
		CoverageRequirements: CorpusAcceptanceCoverage{
			MinSourceCount: 1,
			CandidateKinds: []SemanticCandidateKind{SemanticCandidateKindAction},
			FailureModes:   []SemanticAcceptanceReason{SemanticAcceptanceReasonMissingExpectedOutcome},
		},
		Sources: []CorpusAcceptanceAnswerKeySource{{
			SourceID:         "source-demo",
			SourceDocumentID: "doc-demo",
			ExpectedOutcomes: []SemanticExpectedOutcome{{
				ExpectedOutcomeID:      "exp-action",
				ExpectedState:          ExpectedOutcomePresent,
				ExpectedKind:           SemanticCandidateKindAction,
				RequiredEvidence:       []string{"node-demo"},
				TitleSignals:           []string{"checklist"},
				SummarySignals:         []string{"prepare"},
				RelationRequirements:   []SemanticRelationshipType{SemanticRelationshipDerivedFrom},
				MinimumConfidenceFloor: ConfidenceLow,
			}},
		}},
	}
	return root, pressure, answerKey
}

func corpusAcceptanceCandidate(t *testing.T, kind SemanticCandidateKind, status ReviewStatus) SemanticCandidate {
	t.Helper()
	node := validStructureNode()
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	candidate.CandidateKind = kind
	candidate.ReviewStatus = status
	return candidate
}

func stringListContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
