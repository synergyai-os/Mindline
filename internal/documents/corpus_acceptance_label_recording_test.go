package documents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusAcceptanceLabelRecordingBuildsAnswerKeyFromRecords(t *testing.T) {
	root, _, packet, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.SuiteKind = CorpusAcceptanceSuiteCalibration
	writeDocumentsTestJSON(t, recordsPath, records)

	summary, answerKey, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("build label recording: %v", err)
	}
	if !summary.BenchmarkReady || summary.HeldOutReady || summary.EvalCount != 2 || summary.ExpectedPresentCount != 1 || summary.ExpectedAbsentCount != 1 {
		t.Fatalf("unexpected summary readiness/accounting: %+v", summary)
	}
	if len(summary.Blockers) != 0 {
		t.Fatalf("expected no summary blockers, got %v", summary.Blockers)
	}
	if len(answerKey.Sources) != 1 || len(answerKey.Sources[0].ExpectedOutcomes) != 2 || answerKey.Sources[0].SourceID != packet.Sources[0].SourceID {
		t.Fatalf("unexpected answer key: %+v", answerKey)
	}
	reportData, err := os.ReadFile(filepath.Join(root, "recording", corpusAcceptanceLabelRecordingDirName, "label-recording-report.md"))
	if err != nil {
		t.Fatalf("read recording report: %v", err)
	}
	report := string(reportData)
	if strings.Contains(report, packet.Sources[0].SourceID) || strings.Contains(report, packet.Sources[0].Candidates[0].CandidateID) || strings.Contains(report, root) {
		t.Fatalf("recording report leaked local identifiers: %s", report)
	}
}

func TestCorpusAcceptanceLabelRecordingCountsUncertainAndAbstainOutsideOutcomes(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.Records = append(records.Records,
		CorpusAcceptanceLabelRecordItem{RecordID: "rec-uncertain", CaseID: "case-001", Decision: CorpusAcceptanceLabelUncertain, Notes: "needs second reviewer"},
		CorpusAcceptanceLabelRecordItem{RecordID: "rec-abstain", CaseID: "case-001", Decision: CorpusAcceptanceLabelAbstain, Notes: "insufficient context"},
	)
	writeDocumentsTestJSON(t, recordsPath, records)

	summary, answerKey, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("build label recording: %v", err)
	}
	if summary.UncertainCount != 1 || summary.AbstainCount != 1 || summary.EvalCount != 2 || len(answerKey.Sources[0].ExpectedOutcomes) != 2 {
		t.Fatalf("uncertain/abstain should be counted but not outcomes, summary=%+v answerKey=%+v", summary, answerKey)
	}
}

func TestCorpusAcceptanceLabelRecordingRejectsUnknownEvidence(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.Records[0].RequiredEvidence = []string{"node-does-not-exist"}
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "unknown_evidence_ref:rec-present") {
		t.Fatalf("expected unknown evidence blocker, got %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingRejectsNoCandidateUnknownEvidence(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.Records = []CorpusAcceptanceLabelRecordItem{{
		RecordID:               "rec-absent-no-candidate",
		CaseID:                 "case-001",
		Decision:               CorpusAcceptanceLabelExpectedAbsent,
		ExpectedOutcomeID:      "exp-absent-no-candidate",
		ExpectedKind:           SemanticCandidateKindReference,
		SourceDocumentID:       "doc-source",
		RequiredEvidence:       []string{"node-does-not-exist"},
		MinimumConfidenceFloor: ConfidenceLow,
	}}
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "unknown_evidence_ref:rec-absent-no-candidate") {
		t.Fatalf("expected no-candidate unknown evidence blocker, got %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingRejectsUnsupportedDecision(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.Records[0].Decision = CorpusAcceptanceLabelDecision("typo_present")
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "unsupported_decision:rec-present") {
		t.Fatalf("expected unsupported decision blocker, got %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingRedactsUnsafeRecordIDFromFatalBlockerSuffixes(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	unsafeRecordID := "rec-" + unsafeTokenMarker()
	records.Records[0].RecordID = unsafeRecordID
	records.Records[0].Decision = CorpusAcceptanceLabelDecision("typo_present")
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "unsupported_decision:unsafe_record_id") {
		t.Fatalf("expected unsupported decision blocker with safe record fallback, got %v", err)
	}
	if strings.Contains(err.Error(), unsafeRecordID) || strings.Contains(err.Error(), unsafeTokenMarker()) {
		t.Fatalf("fatal blocker leaked unsafe record id: %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingRejectsOutcomeWithoutSourceID(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.Records[0].SourceID = ""
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "missing_source_id:rec-present") {
		t.Fatalf("expected missing source id blocker, got %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingRejectsDuplicateCandidateLabelRef(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	duplicate := records.Records[0]
	duplicate.RecordID = "rec-present-copy"
	duplicate.ExpectedOutcomeID = "exp-present-copy"
	records.Records = append(records.Records, duplicate)
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "duplicate_label_ref:rec-present-copy") {
		t.Fatalf("expected duplicate label ref blocker, got %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingRejectsDuplicateEvidenceLabelRef(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.Records[1].CandidateID = ""
	records.Records[1].ExpectedKind = records.Records[0].ExpectedKind
	records.Records[1].SourceDocumentID = records.Records[0].SourceDocumentID
	records.Records[1].RequiredEvidence = append([]string{}, records.Records[0].RequiredEvidence...)
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "duplicate_label_ref:rec-absent") {
		t.Fatalf("expected duplicate evidence label ref blocker, got %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingAllowsDistinctLabelsToShareEvidence(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	sharedEvidence := records.Records[0].RequiredEvidence[0]
	records.Records[1].RequiredEvidence = []string{sharedEvidence}
	writeDocumentsTestJSON(t, recordsPath, records)

	_, answerKey, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("shared evidence on distinct candidate-scoped labels should be allowed: %v", err)
	}
	if len(answerKey.Sources) != 1 || len(answerKey.Sources[0].ExpectedOutcomes) != 2 {
		t.Fatalf("expected both labels to survive shared evidence, answerKey=%+v", answerKey)
	}
}

func TestCorpusAcceptanceLabelRecordingRejectsContradictoryCandidateLabelRef(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.Records[1].CandidateID = records.Records[0].CandidateID
	records.Records[1].ExpectedKind = records.Records[0].ExpectedKind
	records.Records[1].SourceDocumentID = records.Records[0].SourceDocumentID
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "duplicate_label_ref:rec-absent") {
		t.Fatalf("expected contradictory candidate label ref blocker, got %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingDerivesEvidenceForCandidateScopedAbsent(t *testing.T) {
	root, _, packet, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)

	_, answerKey, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("build label recording: %v", err)
	}
	var absent SemanticExpectedOutcome
	for _, outcome := range answerKey.Sources[0].ExpectedOutcomes {
		if outcome.ExpectedOutcomeID == "exp-absent" {
			absent = outcome
			break
		}
	}
	if absent.ExpectedOutcomeID != "exp-absent" || !stringListContains(absent.RequiredEvidence, packet.Sources[0].Candidates[1].EvidenceNodes[0]) {
		t.Fatalf("candidate-scoped absent outcome should preserve candidate evidence constraints, got %+v", absent)
	}
}

func TestCorpusAcceptanceLabelRecordingIncludesGuardrailBlockersInReadiness(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	packet := readCorpusAcceptanceLabelingPacketForTest(t, filepath.Join(root, "labeling"))
	packet.Guardrails.NetworkFetches = 1
	packet.Guardrails.HostedInferenceCalls = 1
	packet.Guardrails.HostedTelemetryExports = 1
	packet.Guardrails.BrowserCalls = 1
	packet.Guardrails.SlackAPICalls = 1
	packet.Guardrails.DestinationWrites = 1
	packet.Guardrails.ProductBrainWrites = 1
	packet.Guardrails.TolariaWrites = 1
	packet.Guardrails.AutoAccepts = 1
	packet.Guardrails.NoHumanClaims = 1
	packet.Guardrails.CommittedPrivateArtifacts = 1
	writeDocumentsTestJSON(t, filepath.Join(root, "labeling", corpusAcceptanceLabelingDirName, "labeling-packet.json"), packet)

	summary, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("build label recording: %v", err)
	}
	for _, blocker := range []string{
		"network_fetch",
		"hosted_inference_call",
		"hosted_telemetry_export",
		"browser_call",
		"slack_api_call",
		"destination_write",
		"product_brain_write",
		"tolaria_write",
		"auto_accept",
		"no_human_claim",
		"committed_private_artifact",
	} {
		if !stringListContains(summary.Blockers, blocker) {
			t.Fatalf("expected guardrail blocker %q, summary=%+v", blocker, summary)
		}
	}
	if summary.BenchmarkReady {
		t.Fatalf("expected guardrail blockers to block readiness, summary=%+v", summary)
	}
}

func TestCorpusAcceptanceLabelRecordingRedactsInvalidCoverageValues(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	privateCoverageValue := SemanticCandidateKind("https://slack.com/archives/C012345")
	records.CoverageRequirements.CandidateKinds = append(records.CoverageRequirements.CandidateKinds, privateCoverageValue)
	writeDocumentsTestJSON(t, recordsPath, records)

	summary, answerKey, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("build label recording: %v", err)
	}
	if summary.BenchmarkReady || !stringListContains(summary.Blockers, "invalid_candidate_kind_coverage") {
		t.Fatalf("expected invalid coverage blocker to block readiness, summary=%+v", summary)
	}
	joined := strings.Join(summary.Blockers, "\n")
	if strings.Contains(joined, string(privateCoverageValue)) || strings.Contains(joined, "slack.com") {
		t.Fatalf("coverage blocker leaked private value: %v", summary.Blockers)
	}
	for _, kind := range answerKey.CoverageRequirements.CandidateKinds {
		if kind == privateCoverageValue {
			t.Fatalf("answer key retained invalid private coverage value: %+v", answerKey.CoverageRequirements)
		}
	}
	answerKeyData, err := os.ReadFile(filepath.Join(root, "recording", corpusAcceptanceLabelRecordingDirName, "answer-key.json"))
	if err != nil {
		t.Fatalf("read persisted answer key: %v", err)
	}
	if strings.Contains(string(answerKeyData), string(privateCoverageValue)) || strings.Contains(string(answerKeyData), "slack.com") {
		t.Fatalf("persisted answer key leaked invalid private coverage value: %s", string(answerKeyData))
	}
}

func TestCorpusAcceptanceLabelRecordingIncludesCoverageBlockersInReadiness(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.CoverageRequirements.CandidateKinds = append(records.CoverageRequirements.CandidateKinds, SemanticCandidateKindDecision)
	records.CoverageRequirements.RelationTypes = []SemanticRelationshipType{SemanticRelationshipDerivedFrom}
	records.CoverageRequirements.FailureModes = []SemanticAcceptanceReason{SemanticAcceptanceReasonMissingEvidence}
	writeDocumentsTestJSON(t, recordsPath, records)

	summary, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("build label recording: %v", err)
	}
	if summary.BenchmarkReady || !stringListContains(summary.Blockers, "missing_candidate_kind_coverage:decision_candidate") ||
		!stringListContains(summary.Blockers, "missing_relation_coverage:derived_from") ||
		!stringListContains(summary.Blockers, "missing_failure_mode_coverage:missing_evidence") {
		t.Fatalf("expected coverage blockers to block readiness, summary=%+v", summary)
	}
}

func TestCorpusAcceptanceLabelRecordingIncludesDEC64HeldOutMinimumInReadiness(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.MinEvalCount = 1
	writeDocumentsTestJSON(t, recordsPath, records)

	summary, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("build label recording: %v", err)
	}
	if summary.BenchmarkReady || summary.HeldOutReady || !stringListContains(summary.Blockers, "below_dec64_min_eval_count") {
		t.Fatalf("expected DEC-64 held-out minimum blocker to block readiness, summary=%+v", summary)
	}
}

func TestCorpusAcceptanceLabelRecordingRejectsSourceIDAsSourceDocumentID(t *testing.T) {
	root, _, packet, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.Records = []CorpusAcceptanceLabelRecordItem{{
		RecordID:               "rec-absent-no-candidate",
		CaseID:                 packet.Sources[0].CaseID,
		Decision:               CorpusAcceptanceLabelExpectedAbsent,
		ExpectedOutcomeID:      "exp-absent-no-candidate",
		ExpectedKind:           SemanticCandidateKindReference,
		SourceID:               packet.Sources[0].SourceID,
		SourceDocumentID:       packet.Sources[0].SourceID,
		RequiredEvidence:       []string{packet.Sources[0].Candidates[0].EvidenceNodes[0]},
		MinimumConfidenceFloor: ConfidenceLow,
	}}
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "source_document_id_mismatch:rec-absent-no-candidate") {
		t.Fatalf("expected source document id mismatch blocker, got %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingRedactsUnknownCaseIDFromBlockers(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	unsafeCaseID := "https://slack.example/" + unsafeTokenMarker()
	records.Records[0].CaseID = unsafeCaseID
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "unknown_case_id:rec-present") {
		t.Fatalf("expected unknown case blocker keyed by record id, got %v", err)
	}
	if strings.Contains(err.Error(), unsafeCaseID) || strings.Contains(err.Error(), unsafeTokenMarker()) {
		t.Fatalf("unknown case blocker leaked raw case id: %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingRedactsUnsafeRecordIDFromUnknownCaseBlocker(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	unsafeRecordID := "rec-" + unsafeTokenMarker()
	records.Records[0].RecordID = unsafeRecordID
	records.Records[0].CaseID = "case-does-not-exist"
	writeDocumentsTestJSON(t, recordsPath, records)

	_, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err == nil || !strings.Contains(err.Error(), "unknown_case_id:unsafe_record_id") {
		t.Fatalf("expected unknown case blocker with safe fallback, got %v", err)
	}
	if strings.Contains(err.Error(), unsafeRecordID) || strings.Contains(err.Error(), unsafeTokenMarker()) {
		t.Fatalf("unknown case blocker leaked unsafe record id: %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordingNonIndependentRecordsStayBlocked(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, "generated_from_packet")

	summary, answerKey, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("build label recording: %v", err)
	}
	if summary.BenchmarkReady || summary.HeldOutReady || !stringListContains(summary.Blockers, "label_records_not_independent") {
		t.Fatalf("expected non-independent records to remain blocked, summary=%+v", summary)
	}
	pressureRoot := filepath.Dir(filepath.Join(root, "corpus-pressure", "pressure-summary.json"))
	_ = pressureRoot
	benchmark, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "recording", corpusAcceptanceLabelRecordingDirName, "answer-key.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build benchmark from non-independent answer key: %v", err)
	}
	if benchmark.SuiteValid || !stringListContains(benchmark.SuiteValidityBlockers, "answer_key_not_independent") || answerKey.Provenance.Independence == corpusAcceptanceIndependentProvenance {
		t.Fatalf("benchmark should reject non-independent answer key, benchmark=%+v answerKey=%+v", benchmark, answerKey)
	}
}

func TestWriteCorpusAcceptanceLabelRecordingValidatesReportBeforeWritingArtifacts(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	packet := readCorpusAcceptanceLabelingPacketForTest(t, filepath.Join(root, "labeling"))
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	summary, answerKey, err := buildCorpusAcceptanceLabelRecording(packet, records)
	if err != nil {
		t.Fatalf("build in-memory label recording: %v", err)
	}
	summary.SuiteKind = CorpusAcceptanceSuiteKind("https://slack.com/archives/C012345")
	out := filepath.Join(root, "report-validation")

	err = WriteCorpusAcceptanceLabelRecording(out, summary, answerKey)
	if err == nil || !strings.Contains(err.Error(), "report contains private marker") {
		t.Fatalf("expected report privacy failure, got %v", err)
	}
	recordingRoot := filepath.Join(out, corpusAcceptanceLabelRecordingDirName)
	for _, name := range []string{"answer-key.json", "label-recording-summary.json", "label-recording-report.md"} {
		if _, statErr := os.Stat(filepath.Join(recordingRoot, name)); !os.IsNotExist(statErr) {
			t.Fatalf("report validation failure should not leave %s behind, stat=%v", name, statErr)
		}
	}
}

func writeCorpusAcceptanceLabelRecordingFixture(t *testing.T, independence string) (string, CorpusPressureSummary, CorpusAcceptanceLabelingPacket, string) {
	t.Helper()
	presentCandidate := corpusAcceptanceCandidate(t, SemanticCandidateKindReference, ReviewStatusReady)
	absentCandidate := corpusAcceptanceCandidate(t, SemanticCandidateKindCapability, ReviewStatusReady)
	absentCandidate.CandidateID = "cand-absent"
	root, pressure, _ := writeCorpusAcceptanceFixture(t, []SemanticCandidate{presentCandidate, absentCandidate}, nil)
	packet, _, err := BuildCorpusAcceptanceLabelingPacket(root, filepath.Join(root, "labeling"))
	if err != nil {
		t.Fatalf("build labeling packet: %v", err)
	}
	candidate := packet.Sources[0].Candidates[0]
	absent := packet.Sources[0].Candidates[1]
	records := CorpusAcceptanceLabelRecords{
		SchemaVersion: CorpusAcceptanceLabelRecordsSchemaVersion,
		SuiteID:       "heldout-recording-demo",
		SuiteKind:     CorpusAcceptanceSuiteHeldOut,
		Provenance:    CorpusAcceptanceProvenance{Labeler: "fixture-human", Independence: independence},
		MinEvalCount:  1,
		CoverageRequirements: CorpusAcceptanceCoverage{
			MinSourceCount: 1,
			CandidateKinds: []SemanticCandidateKind{SemanticCandidateKindReference},
			FailureModes:   []SemanticAcceptanceReason{SemanticAcceptanceReasonMissingExpectedOutcome, SemanticAcceptanceReasonUnexpectedCandidate},
		},
		Records: []CorpusAcceptanceLabelRecordItem{
			{
				RecordID:               "rec-present",
				CaseID:                 packet.Sources[0].CaseID,
				Decision:               CorpusAcceptanceLabelExpectedPresent,
				CandidateID:            candidate.CandidateID,
				ExpectedOutcomeID:      "exp-present",
				ExpectedKind:           candidate.CandidateKind,
				SourceID:               packet.Sources[0].SourceID,
				SourceDocumentID:       candidate.SourceDocumentID,
				RequiredEvidence:       []string{candidate.EvidenceNodes[0]},
				TitleSignals:           []string{"checklist"},
				SummarySignals:         []string{"prepare"},
				MinimumConfidenceFloor: ConfidenceLow,
			},
			{
				RecordID:               "rec-absent",
				CaseID:                 packet.Sources[0].CaseID,
				Decision:               CorpusAcceptanceLabelExpectedAbsent,
				CandidateID:            absent.CandidateID,
				ExpectedOutcomeID:      "exp-absent",
				ExpectedKind:           absent.CandidateKind,
				SourceID:               packet.Sources[0].SourceID,
				SourceDocumentID:       absent.SourceDocumentID,
				MinimumConfidenceFloor: ConfidenceLow,
			},
		},
	}
	recordsPath := filepath.Join(root, "label-records.json")
	writeDocumentsTestJSON(t, recordsPath, records)
	return root, pressure, packet, recordsPath
}

func readLabelRecordingFixtureRecords(t *testing.T, path string) CorpusAcceptanceLabelRecords {
	t.Helper()
	records, err := readCorpusAcceptanceLabelRecords(path)
	if err != nil {
		t.Fatalf("read records: %v", err)
	}
	return records
}

func readCorpusAcceptanceLabelingPacketForTest(t *testing.T, path string) CorpusAcceptanceLabelingPacket {
	t.Helper()
	packet, err := readCorpusAcceptanceLabelingPacket(path)
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	return packet
}
