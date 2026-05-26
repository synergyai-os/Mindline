package documents

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAutonomyReadinessEligibleOnlyWithHeldOutThresholdEvidenceAndSafety(t *testing.T) {
	out := t.TempDir()
	summary := autonomyReadinessTestSummary(t, SemanticJudgmentChoiceAccept, "")
	if err := WriteSemanticJudgmentRoot(filepath.Join(out, "semantic-judgment"), summary); err != nil {
		t.Fatalf("write judgment: %v", err)
	}

	report, err := BuildAutonomyReadinessReport(out, AutonomyReadinessOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if report.ThresholdStatus != AutonomyReadinessEligible {
		t.Fatalf("expected eligible report, got %+v", report)
	}
	if !report.KRs["KEY-3"].Passed || !report.KRs["KEY-7"].Passed {
		t.Fatalf("expected threshold and evidence KRs to pass: %+v", report.KRs)
	}

	report, err = BuildAutonomyReadinessReport(out, AutonomyReadinessOptions{Threshold: 0.98})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if report.ThresholdStatus != AutonomyReadinessNotEligible || !containsString(report.Blockers, "not_held_out") {
		t.Fatalf("expected not-held-out blocker, got %+v", report.Blockers)
	}
}

func TestAutonomyReadinessSafetyCountersComeFromArtifactsBeforeEligibility(t *testing.T) {
	out := t.TempDir()
	summary := autonomyReadinessTestSummary(t, SemanticJudgmentChoiceAccept, "")
	if err := WriteSemanticJudgmentRoot(filepath.Join(out, "semantic-judgment"), summary); err != nil {
		t.Fatalf("write judgment: %v", err)
	}
	writeTestArtifact(t, out, "destinations/candidate-1/operations/001-create-note.json", `{
  "schema_version": "destination-operation/v0.1",
  "operation_id": "op-1",
  "destination_adapter_id": "tolaria",
  "source_candidate_id": "candidate-1",
  "source_record_id": "record-1",
  "idempotency_key": "key-1",
  "operation_type": "create_note",
  "write_mode": "dry_run",
  "visibility_lane": "publish",
  "planned_locator": "30-resources/source.md",
  "title": "Processed source",
  "body": "body",
  "metadata": {"state": "dry_run_published"},
  "authority_ids": ["DEC-64"]
}`)
	writeTestArtifact(t, out, "semantic-calibration/calibration-summary.json", `{
  "schema_version": "semantic-calibration-summary/v0.2",
  "no_human_eligible": true
}`)
	writeTestArtifact(t, out, "review/auto-accept-summary.json", `{
  "schema_version": "semantic-auto-accept-summary/v0.1",
  "auto_accept_count": 1
}`)
	writeTestArtifact(t, out, "results/private-result.json", `{
  "schema_version": "pipeline-result/v0.1",
  "safety": {"private_provenance": true, "redaction_required": false, "secret_like": false}
}`)

	report, err := BuildAutonomyReadinessReport(out, AutonomyReadinessOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if report.SafetyCounters.DestinationWrites != 1 ||
		report.SafetyCounters.AutoAccepts != 1 ||
		report.SafetyCounters.NoHumanClaims != 1 ||
		report.SafetyCounters.CommittedPrivateArtifacts != 1 {
		t.Fatalf("expected safety counters from artifacts, got %+v", report.SafetyCounters)
	}
	if report.ThresholdStatus != AutonomyReadinessNotEligible || !containsString(report.Blockers, "safety_counter_nonzero") {
		t.Fatalf("expected safety blocker before eligibility, got status=%s blockers=%+v", report.ThresholdStatus, report.Blockers)
	}
	if report.KRs["KEY-6"].Passed {
		t.Fatalf("expected KEY-6 to fail when safety counters are nonzero: %+v", report.KRs["KEY-6"])
	}
}

func TestAutonomyReadinessIncludesRequiredSlicesAndImprovementTargets(t *testing.T) {
	out := t.TempDir()
	summary := autonomyReadinessTestSummary(t, SemanticJudgmentChoiceReject, SemanticFailureUnexpectedCandidate)
	if err := WriteSemanticJudgmentRoot(filepath.Join(out, "semantic-judgment"), summary); err != nil {
		t.Fatalf("write judgment: %v", err)
	}
	traceDir := filepath.Join(out, "trace")
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		t.Fatalf("mkdir trace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(traceDir, "trace-summary.json"), []byte(`{"run_id":"run-demo","provider":"openai","model":"gpt-5.2","status":"needs_improvement"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write trace: %v", err)
	}

	report, err := BuildAutonomyReadinessReport(out, AutonomyReadinessOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if report.ThresholdStatus != AutonomyReadinessNotEligible || !containsString(report.Blockers, "below_threshold") {
		t.Fatalf("expected below-threshold report, got %+v", report)
	}
	if report.Counts.FalsePositiveCount != 1 || report.Counts.FalseNegativeCount != 0 {
		t.Fatalf("expected explicit FP/FN counts, got %+v", report.Counts)
	}
	if report.Slices.BySourceType["transcript"][string(SemanticJudgmentChoiceReject)] != 1 {
		t.Fatalf("expected transcript source type slice: %+v", report.Slices.BySourceType)
	}
	if report.Slices.ByProviderModel["openai/gpt-5.2"] != 1 || report.Slices.ByRunStatus["needs_improvement"] != 1 {
		t.Fatalf("expected trace slices, got provider=%+v status=%+v", report.Slices.ByProviderModel, report.Slices.ByRunStatus)
	}
	if !report.KRs["KEY-5"].Passed || !report.KRs["KEY-4"].Passed {
		t.Fatalf("expected slice and taxonomy KRs to pass: %+v", report.KRs)
	}
	if len(report.Improvement) == 0 || report.Improvement[0].Code != "below_threshold" {
		t.Fatalf("expected below-threshold improvement target, got %+v", report.Improvement)
	}
}

func TestAutonomyReadinessKeepsBelowThresholdTargetForTinyAccuracyGap(t *testing.T) {
	report := AutonomyReadinessReport{
		HeldOut:   true,
		Threshold: 0.98,
		Accuracy:  0.97995,
		Counts: AutonomyReadinessCounts{
			EvalCountedAcceptedCount:      97995,
			EvalCountedFalsePositiveCount: 2005,
		},
	}

	targets := autonomyImprovementTargets(report, SemanticJudgmentSummary{})
	for _, target := range targets {
		if target.Code == "below_threshold" {
			if target.Count != 1 {
				t.Fatalf("expected minimum below-threshold count of 1, got %+v", target)
			}
			return
		}
	}
	t.Fatalf("expected below-threshold target for tiny accuracy gap, got %+v", targets)
}

func TestAutonomyReadinessKeepsBelowThresholdTargetWhenTopTargetsAreCapped(t *testing.T) {
	report := AutonomyReadinessReport{
		HeldOut:   true,
		Threshold: 0.98,
		Accuracy:  0.97995,
		Counts: AutonomyReadinessCounts{
			CandidateCount:                1,
			EvalCountedAcceptedCount:      97995,
			EvalCountedFalsePositiveCount: 2005,
			EvidenceExcludedCount:         10,
			EvalCountedHumanReviewCount:   9,
			EvalCountedModelErrorCount:    8,
			ReviewBurdenCount:             7,
			WrongKindCount:                6,
			DuplicateCount:                5,
			FalsePositiveCount:            4,
			FalseNegativeCount:            3,
			EvalCountedRemainingCount:     2,
		},
	}
	summary := SemanticJudgmentSummary{
		WrongKindCount: 6,
		DuplicateCount: 5,
		FailureReasonCounts: map[SemanticFailureReason]int{
			SemanticFailureWrongKind: 6,
			SemanticFailureDuplicate: 5,
		},
	}

	targets := autonomyImprovementTargets(report, summary)
	if len(targets) > 5 {
		t.Fatalf("expected capped improvement targets, got %d: %+v", len(targets), targets)
	}
	for _, target := range targets {
		if target.Code == "below_threshold" {
			return
		}
	}
	t.Fatalf("expected capped targets to retain below_threshold, got %+v", targets)
}

func TestAutonomyReadinessMapsMissingExpectedOutcomeToFalseNegative(t *testing.T) {
	summary := SemanticJudgmentSummary{
		FailureReasonCounts: map[SemanticFailureReason]int{
			SemanticFailureMissingExpectedOutcome: 1,
		},
	}
	if autonomyFalseNegativeCount(summary) != 1 {
		t.Fatalf("expected false negative count from missing expected outcome")
	}
}

func TestAutonomyReadinessCountsSecondaryMissingExpectedOutcomeAsFalseNegative(t *testing.T) {
	out := t.TempDir()
	summary := autonomyReadinessTestSummary(t, SemanticJudgmentChoiceReject, SemanticFailureUnexpectedCandidate)
	judgment := summary.Judgments[0]
	judgment.SecondaryReasons = []SemanticFailureReason{SemanticFailureMissingExpectedOutcome}
	summary = BuildSemanticJudgmentSummary("run-demo", 1, summary.Items, []SemanticJudgmentRecord{judgment})
	if err := WriteSemanticJudgmentRoot(filepath.Join(out, "semantic-judgment"), summary); err != nil {
		t.Fatalf("write judgment: %v", err)
	}

	report, err := BuildAutonomyReadinessReport(out, AutonomyReadinessOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if report.Counts.FalseNegativeCount != 1 || report.Counts.EvalCountedFalseNegativeCount != 1 {
		t.Fatalf("expected secondary missing outcome to count as false negative, got %+v", report.Counts)
	}
}

func TestAutonomyReadinessDoesNotDoubleCountSecondaryMissingOutcomeAsFalsePositive(t *testing.T) {
	var items []SemanticJudgmentCandidate
	var judgments []SemanticJudgmentRecord
	for i := 0; i < 49; i++ {
		candidateID := "candidate-accepted-" + strconv.Itoa(i)
		items = append(items, SemanticJudgmentCandidate{
			CandidateID: candidateID,
			EvidenceReadiness: SemanticEvidenceReadiness{
				Status:      SemanticEvidenceReadinessPass,
				EvalCounted: true,
			},
		})
		judgments = append(judgments, SemanticJudgmentRecord{
			SchemaVersion: SemanticJudgmentRecordSchemaVersion,
			RunID:         "run-demo",
			CandidateID:   candidateID,
			Choice:        SemanticJudgmentChoiceAccept,
			ReviewerID:    "test",
			RecordedAt:    time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		})
	}
	missed := SemanticJudgmentCandidate{
		CandidateID: "candidate-secondary-missing",
		EvidenceReadiness: SemanticEvidenceReadiness{
			Status:      SemanticEvidenceReadinessPass,
			EvalCounted: true,
		},
	}
	items = append(items, missed)
	judgments = append(judgments, SemanticJudgmentRecord{
		SchemaVersion:    SemanticJudgmentRecordSchemaVersion,
		RunID:            "run-demo",
		CandidateID:      missed.CandidateID,
		Choice:           SemanticJudgmentChoiceReject,
		FailureReason:    SemanticFailureUnexpectedCandidate,
		SecondaryReasons: []SemanticFailureReason{SemanticFailureMissingExpectedOutcome},
		ReviewerID:       "test",
		RecordedAt:       time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	})

	counts := autonomyEvalCountedOutcomeCounts(items, judgments)
	if counts.accepted != 49 || counts.falsePositive != 0 || counts.falseNegative != 1 {
		t.Fatalf("expected secondary missing outcome to occupy one eval error bucket, got %+v", counts)
	}
	reportCounts := AutonomyReadinessCounts{
		EvalCountedAcceptedCount:      counts.accepted,
		EvalCountedFalsePositiveCount: counts.falsePositive,
		EvalCountedFalseNegativeCount: counts.falseNegative,
	}
	if got := autonomyAccuracy(reportCounts); got != 0.98 {
		t.Fatalf("expected single-bucket denominator accuracy 0.98, got %f", got)
	}
}

func TestAutonomyReadinessAccuracyUsesTrueFalsePositiveFalseNegativeDenominator(t *testing.T) {
	counts := AutonomyReadinessCounts{
		EvalCountedAcceptedCount:      98,
		EvalCountedFalsePositiveCount: 1,
		EvalCountedFalseNegativeCount: 1,
	}
	if got := autonomyAccuracy(counts); got != 0.98 {
		t.Fatalf("expected accuracy 0.98, got %f", got)
	}
	counts = AutonomyReadinessCounts{EvalCountedAcceptedCount: 98, EvalCountedFalsePositiveCount: 0, EvalCountedFalseNegativeCount: 2}
	if got := autonomyAccuracy(counts); got != 0.98 {
		t.Fatalf("expected false negatives to affect denominator, got %f", got)
	}
}

func TestAutonomyReadinessAccuracyIgnoresEvidenceExcludedJudgments(t *testing.T) {
	out := t.TempDir()
	summary := autonomyReadinessTestSummary(t, SemanticJudgmentChoiceReject, SemanticFailureUnexpectedCandidate)
	item := summary.Items[0]
	item.RelationContext = nil
	item.EvidenceReadiness = semanticEvidenceReadiness(item)
	summary = BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{item}, summary.Judgments)
	if err := WriteSemanticJudgmentRoot(filepath.Join(out, "semantic-judgment"), summary); err != nil {
		t.Fatalf("write judgment: %v", err)
	}

	report, err := BuildAutonomyReadinessReport(out, AutonomyReadinessOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if report.Counts.FalsePositiveCount != 1 || report.Counts.EvalCountedFalsePositiveCount != 0 || report.Accuracy != 0 {
		t.Fatalf("expected excluded judgment to stay out of DEC-64 denominator, got %+v accuracy=%f", report.Counts, report.Accuracy)
	}
}

func TestAutonomyReadinessEligibilityIgnoresExcludedRemainingReviewAndModelBlockers(t *testing.T) {
	out := t.TempDir()
	summary := autonomyReadinessTestSummary(t, SemanticJudgmentChoiceAccept, "")
	accepted := summary.Items[0]
	excluded := accepted
	excluded.CandidateID = "candidate-excluded"
	excluded.RelationContext = nil
	excluded.AgentReview = &SemanticAgentReviewProposal{
		SchemaVersion:       SemanticAgentReviewProposalSchemaVersion,
		Provider:            "openai",
		Model:               "gpt-5.2",
		Choice:              SemanticJudgmentChoiceUnclear,
		FailureReason:       SemanticFailureOther,
		Confidence:          ConfidenceLow,
		HumanReviewRequired: true,
		ReviewReasonCodes:   []SemanticAgentReviewReasonCode{SemanticAgentReviewReasonModelError},
		Rationale:           "model call failed",
	}
	excluded.EvidenceReadiness = semanticEvidenceReadiness(excluded)
	summary = BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{accepted, excluded}, []SemanticJudgmentRecord{summary.Judgments[0]})
	if err := WriteSemanticJudgmentRoot(filepath.Join(out, "semantic-judgment"), summary); err != nil {
		t.Fatalf("write judgment: %v", err)
	}

	report, err := BuildAutonomyReadinessReport(out, AutonomyReadinessOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if report.ThresholdStatus != AutonomyReadinessEligible {
		t.Fatalf("expected excluded unjudged review/model item not to block eligibility, got blockers=%+v counts=%+v", report.Blockers, report.Counts)
	}
	for _, blocker := range []string{"remaining_judgments", "human_review_required", "model_errors"} {
		if containsString(report.Blockers, blocker) {
			t.Fatalf("expected blocker %q to ignore evidence-excluded candidates, got %+v", blocker, report.Blockers)
		}
	}
}

func TestAutonomyReadinessJudgedEvalHumanReviewRequiredBlocksEligibility(t *testing.T) {
	out := t.TempDir()
	summary := autonomyReadinessTestSummary(t, SemanticJudgmentChoiceAccept, "")
	item := summary.Items[0]
	item.AgentReview = &SemanticAgentReviewProposal{
		SchemaVersion:       SemanticAgentReviewProposalSchemaVersion,
		Provider:            "openai",
		Model:               "gpt-5.2",
		Choice:              SemanticJudgmentChoiceUnclear,
		FailureReason:       SemanticFailureOther,
		Confidence:          ConfidenceLow,
		HumanReviewRequired: true,
		ReviewReasonCodes:   []SemanticAgentReviewReasonCode{SemanticAgentReviewReasonLowConfidence},
		Rationale:           "needs human judgment",
	}
	summary = BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{item}, summary.Judgments)
	if err := WriteSemanticJudgmentRoot(filepath.Join(out, "semantic-judgment"), summary); err != nil {
		t.Fatalf("write judgment: %v", err)
	}

	report, err := BuildAutonomyReadinessReport(out, AutonomyReadinessOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if report.Counts.EvalCountedHumanReviewCount != 1 {
		t.Fatalf("expected judged eval-counted human review requirement to count, got %+v", report.Counts)
	}
	if report.ThresholdStatus != AutonomyReadinessNotEligible || !containsString(report.Blockers, "human_review_required") {
		t.Fatalf("expected judged human-review requirement to block eligibility, got status=%s blockers=%+v", report.ThresholdStatus, report.Blockers)
	}
}

func TestAutonomyReadinessReportsNoCandidateRunsAsSpecificImprovementTarget(t *testing.T) {
	out := t.TempDir()
	summary := BuildSemanticJudgmentSummary("run-empty", 1, nil, nil)
	if err := WriteSemanticJudgmentRoot(filepath.Join(out, "semantic-judgment"), summary); err != nil {
		t.Fatalf("write judgment: %v", err)
	}

	report, err := BuildAutonomyReadinessReport(out, AutonomyReadinessOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	found := false
	for _, target := range report.Improvement {
		if target.Code == "no_candidates" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected no_candidates target, got %+v", report.Improvement)
	}
}

func TestAutonomyReadinessDoesNotTreatSkippedNoCandidateRunsAsExtractionFailure(t *testing.T) {
	out := t.TempDir()
	summary := BuildSemanticJudgmentSummaryWithSkippedReason("run-skipped", 1, nil, nil, "all structure nodes are blocked or empty; no semantic candidates expected")
	if err := WriteSemanticJudgmentRoot(filepath.Join(out, "semantic-judgment"), summary); err != nil {
		t.Fatalf("write judgment: %v", err)
	}

	report, err := BuildAutonomyReadinessReport(out, AutonomyReadinessOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	if !containsString(report.Blockers, "no_judged_eval_outcomes") || containsString(report.Blockers, "below_threshold") {
		t.Fatalf("expected skipped empty run to block on missing eval evidence, not below threshold: %+v", report.Blockers)
	}
	for _, target := range report.Improvement {
		if target.Code == "no_candidates" || target.Code == "below_threshold" || target.Code == "no_judged_eval_outcomes" {
			t.Fatalf("expected skipped empty run not to emit extraction-quality target, got %+v", report.Improvement)
		}
	}
	if report.Improvement == nil {
		t.Fatalf("expected empty improvement list to serialize as []")
	}
}

func TestWriteAutonomyReadinessReportRejectsUnexpectedFiles(t *testing.T) {
	out := t.TempDir()
	root := filepath.Join(out, AutonomyReadinessDirName)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "private.md"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("write poison: %v", err)
	}
	report := AutonomyReadinessReport{
		SchemaVersion:    AutonomyReadinessReportSchemaVersion,
		EvaluatorVersion: AutonomyReadinessEvaluatorVersion,
		SuiteID:          "suite-demo",
		ThresholdStatus:  AutonomyReadinessNotEligible,
		KRs:              map[string]AutonomyReadinessKR{},
		Projection:       AutonomyReadinessProjectionReport{Status: AutonomyReadinessProjectionDisabled},
	}
	err := WriteAutonomyReadinessReport(out, report)
	if err == nil || !strings.Contains(err.Error(), "unexpected existing generated file") {
		t.Fatalf("expected unexpected file rejection, got %v", err)
	}
}

func autonomyReadinessTestSummary(t *testing.T, choice SemanticJudgmentChoice, reason SemanticFailureReason) SemanticJudgmentSummary {
	t.Helper()
	node := validStructureNode()
	node.SourceDocumentID = "doc-meeting-transcript-1"
	observation := validSemanticObservation(node)
	candidate := validSemanticCandidate(observation, node)
	candidate.SourceDocumentID = "doc-meeting-transcript-1"
	item := semanticJudgmentCandidates([]SemanticCandidate{candidate}, []SemanticRelation{validSemanticRelation(candidate, observation, node)}, []SemanticObservation{observation}, semanticCalibrationSourceContext{
		Label: "meeting-transcript-1.md",
		Lines: []string{"one", "two"},
	}, nil)[0]
	record := SemanticJudgmentRecord{
		SchemaVersion:    SemanticJudgmentRecordSchemaVersion,
		RunID:            "run-demo",
		CandidateID:      item.CandidateID,
		SourceDocumentID: item.SourceDocumentID,
		CandidateKind:    item.CandidateKind,
		Confidence:       item.Confidence,
		Choice:           choice,
		FailureReason:    reason,
		ReviewerID:       "test",
		RecordedAt:       time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
	return BuildSemanticJudgmentSummary("run-demo", 1, []SemanticJudgmentCandidate{item}, []SemanticJudgmentRecord{record})
}

func writeTestArtifact(t *testing.T, root, path, body string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir artifact: %v", err)
	}
	if err := os.WriteFile(target, []byte(body+"\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
