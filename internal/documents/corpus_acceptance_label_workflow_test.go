package documents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusAcceptanceLabelNextBootstrapsEmptyRecordsAndRedactedMap(t *testing.T) {
	root, _, packet, _ := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	recordsPath := filepath.Join(root, "guided-records.json")

	summary, records, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "next"))
	if err != nil {
		t.Fatalf("build label next: %v", err)
	}
	if summary.SchemaVersion != CorpusAcceptanceLabelNextSummarySchemaVersion || summary.QueueState != corpusAcceptanceLabelQueueReady {
		t.Fatalf("unexpected summary state: %+v", summary)
	}
	if summary.NextItem == nil || summary.NextItem.CaseRef != "case-001" || summary.NextItem.CandidateRef != "candidate-001" {
		t.Fatalf("unexpected next item: %+v", summary.NextItem)
	}
	if summary.NextItem.CandidateID != "" || summary.NextItem.SourceID != "" || len(summary.NextItem.EvidenceNodeIDs) > 0 {
		t.Fatalf("next item leaked raw identifiers: %+v", summary.NextItem)
	}
	if records.Provenance.Independence != corpusAcceptanceGeneratedProvenance || len(records.Records) != 0 {
		t.Fatalf("expected empty non-independent records envelope, got %+v", records)
	}

	mapPath := filepath.Join(root, "next", corpusAcceptanceLabelNextDirName, "label-next-map.json")
	mapData, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatalf("read label-next map: %v", err)
	}
	if !strings.Contains(string(mapData), packet.Sources[0].SourceID) || !strings.Contains(string(mapData), packet.Sources[0].Candidates[0].CandidateID) {
		t.Fatalf("local map did not contain raw refs needed for record resolution: %s", string(mapData))
	}
	reportData, err := os.ReadFile(filepath.Join(root, "next", corpusAcceptanceLabelNextDirName, "label-next-report.md"))
	if err != nil {
		t.Fatalf("read label-next report: %v", err)
	}
	report := string(reportData)
	if strings.Contains(report, packet.Sources[0].SourceID) || strings.Contains(report, packet.Sources[0].Candidates[0].CandidateID) || strings.Contains(report, packet.Sources[0].Candidates[0].EvidenceNodes[0]) {
		t.Fatalf("report leaked raw refs: %s", report)
	}
}

func TestCorpusAcceptanceLabelRecordRoundTripsThroughApply(t *testing.T) {
	root, _, packet, _ := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	recordsPath := filepath.Join(root, "guided-records.json")
	nextOut := filepath.Join(root, "next")
	_, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, nextOut)
	if err != nil {
		t.Fatalf("build label next: %v", err)
	}

	mapPath := filepath.Join(nextOut, corpusAcceptanceLabelNextDirName, "label-next-map.json")
	recorded, err := RecordCorpusAcceptanceLabel(filepath.Join(root, "labeling"), recordsPath, mapPath, CorpusAcceptanceLabelRecordInput{
		CaseRef:                 "case-001",
		CandidateRef:            "candidate-001",
		Decision:                CorpusAcceptanceLabelExpectedPresent,
		ExpectedKind:            packet.Sources[0].Candidates[0].CandidateKind,
		RequiredEvidenceRefs:    []string{"evidence-001"},
		IndependenceAttestation: corpusAcceptanceIndependentProvenance,
	})
	if err != nil {
		t.Fatalf("record label: %v", err)
	}
	if recorded.Provenance.Independence != corpusAcceptanceIndependentProvenance || len(recorded.Records) != 1 {
		t.Fatalf("unexpected recorded labels: %+v", recorded)
	}
	record := recorded.Records[0]
	if record.RecordID != "rec-case-001-candidate-001-expected-present" || record.ExpectedOutcomeID != "case-001-candidate-001-expected-present" || record.SourceID != packet.Sources[0].SourceID || record.CandidateID != packet.Sources[0].Candidates[0].CandidateID {
		t.Fatalf("record did not resolve raw refs correctly: %+v", record)
	}

	summary, answerKey, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("apply guided records: %v", err)
	}
	if summary.EvalCount != 1 || summary.ExpectedPresentCount != 1 || len(answerKey.Sources) != 1 {
		t.Fatalf("unexpected apply output: summary=%+v answerKey=%+v", summary, answerKey)
	}
}

func TestCorpusAcceptanceLabelNextGeneratedEmptyRecordsStayProofBlocked(t *testing.T) {
	root, _, _, _ := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	recordsPath := filepath.Join(root, "guided-records.json")
	_, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "next"))
	if err != nil {
		t.Fatalf("build label next: %v", err)
	}

	summary, _, err := BuildCorpusAcceptanceLabelRecording(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("apply generated empty records: %v", err)
	}
	if summary.BenchmarkReady || summary.HeldOutReady {
		t.Fatalf("generated empty records should not be ready: %+v", summary)
	}
	if !stringListContains(summary.Blockers, "label_records_not_independent") || !stringListContains(summary.Blockers, "below_min_eval_count") {
		t.Fatalf("expected proof blockers, got %+v", summary.Blockers)
	}
}

func TestCorpusAcceptanceLabelRecordUpdatesDeterministicRecord(t *testing.T) {
	root, _, _, _ := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	recordsPath := filepath.Join(root, "guided-records.json")
	nextOut := filepath.Join(root, "next")
	_, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, nextOut)
	if err != nil {
		t.Fatalf("build label next: %v", err)
	}
	input := CorpusAcceptanceLabelRecordInput{
		CaseRef:                 "case-001",
		CandidateRef:            "candidate-001",
		Decision:                CorpusAcceptanceLabelUncertain,
		Labeler:                 "fixture-human",
		IndependenceAttestation: corpusAcceptanceIndependentProvenance,
		Notes:                   "first pass",
	}
	mapPath := filepath.Join(nextOut, corpusAcceptanceLabelNextDirName, "label-next-map.json")
	if _, err := RecordCorpusAcceptanceLabel(filepath.Join(root, "labeling"), recordsPath, mapPath, input); err != nil {
		t.Fatalf("record first label: %v", err)
	}
	input.Notes = "updated note"
	records, err := RecordCorpusAcceptanceLabel(filepath.Join(root, "labeling"), recordsPath, mapPath, input)
	if err != nil {
		t.Fatalf("record updated label: %v", err)
	}
	if len(records.Records) != 1 || records.Records[0].Notes != "updated note" {
		t.Fatalf("expected deterministic update, got %+v", records.Records)
	}
}

func TestCorpusAcceptanceLabelRecordDerivesExpectedAbsentOutcomeID(t *testing.T) {
	root, _, packet, _ := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	recordsPath := filepath.Join(root, "guided-records.json")
	nextOut := filepath.Join(root, "next")
	_, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, nextOut)
	if err != nil {
		t.Fatalf("build label next: %v", err)
	}
	records, err := RecordCorpusAcceptanceLabel(filepath.Join(root, "labeling"), recordsPath, filepath.Join(nextOut, corpusAcceptanceLabelNextDirName, "label-next-map.json"), CorpusAcceptanceLabelRecordInput{
		CaseRef:                 "case-001",
		CandidateRef:            "candidate-002",
		Decision:                CorpusAcceptanceLabelExpectedAbsent,
		ExpectedKind:            packet.Sources[0].Candidates[1].CandidateKind,
		IndependenceAttestation: corpusAcceptanceIndependentProvenance,
	})
	if err != nil {
		t.Fatalf("record expected absent: %v", err)
	}
	if got := records.Records[0].ExpectedOutcomeID; got != "case-001-candidate-002-expected-absent" {
		t.Fatalf("expected derived absent outcome id, got %s", got)
	}
}

func TestCorpusAcceptanceLabelRecordRejectsUnknownRedactedRefs(t *testing.T) {
	root, _, _, _ := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	recordsPath := filepath.Join(root, "guided-records.json")
	nextOut := filepath.Join(root, "next")
	_, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, nextOut)
	if err != nil {
		t.Fatalf("build label next: %v", err)
	}
	_, err = RecordCorpusAcceptanceLabel(filepath.Join(root, "labeling"), recordsPath, filepath.Join(nextOut, corpusAcceptanceLabelNextDirName, "label-next-map.json"), CorpusAcceptanceLabelRecordInput{
		CaseRef:                 "case-999",
		Decision:                CorpusAcceptanceLabelUncertain,
		Labeler:                 "fixture-human",
		IndependenceAttestation: corpusAcceptanceIndependentProvenance,
	})
	if err == nil || !strings.Contains(err.Error(), "unknown case ref") {
		t.Fatalf("expected unknown case ref error, got %v", err)
	}
}

func TestCorpusAcceptanceLabelRecordRejectsUnsupportedRecordsSchemaBeforeWrite(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.SchemaVersion = "corpus-acceptance-label-records/v0.0"
	writeDocumentsTestJSON(t, recordsPath, records)
	before, err := os.ReadFile(recordsPath)
	if err != nil {
		t.Fatalf("read records before mutation: %v", err)
	}
	nextOut := filepath.Join(root, "next")
	if _, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, nextOut); err != nil {
		t.Fatalf("build label next: %v", err)
	}

	_, err = RecordCorpusAcceptanceLabel(filepath.Join(root, "labeling"), recordsPath, filepath.Join(nextOut, corpusAcceptanceLabelNextDirName, "label-next-map.json"), CorpusAcceptanceLabelRecordInput{
		CaseRef:                 "case-001",
		CandidateRef:            "candidate-001",
		Decision:                CorpusAcceptanceLabelUncertain,
		Labeler:                 "fixture-human",
		IndependenceAttestation: corpusAcceptanceIndependentProvenance,
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported_records_schema") {
		t.Fatalf("expected unsupported schema error, got %v", err)
	}
	after, err := os.ReadFile(recordsPath)
	if err != nil {
		t.Fatalf("read records after mutation: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("unsupported schema records were mutated:\nbefore=%s\nafter=%s", string(before), string(after))
	}
}

func TestCorpusAcceptanceLabelRecordRejectsIndependenceUpgradeForExistingGeneratedRecords(t *testing.T) {
	root, _, _, _ := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	recordsPath := filepath.Join(root, "guided-records.json")
	nextOut := filepath.Join(root, "next")
	if _, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, nextOut); err != nil {
		t.Fatalf("build label next: %v", err)
	}
	mapPath := filepath.Join(nextOut, corpusAcceptanceLabelNextDirName, "label-next-map.json")
	if _, err := RecordCorpusAcceptanceLabel(filepath.Join(root, "labeling"), recordsPath, mapPath, CorpusAcceptanceLabelRecordInput{
		CaseRef:      "case-001",
		CandidateRef: "candidate-001",
		Decision:     CorpusAcceptanceLabelUncertain,
		Labeler:      "fixture-human",
	}); err != nil {
		t.Fatalf("record generated label: %v", err)
	}
	before, err := os.ReadFile(recordsPath)
	if err != nil {
		t.Fatalf("read records before upgrade: %v", err)
	}

	_, err = RecordCorpusAcceptanceLabel(filepath.Join(root, "labeling"), recordsPath, mapPath, CorpusAcceptanceLabelRecordInput{
		CaseRef:                 "case-001",
		CandidateRef:            "candidate-001",
		Decision:                CorpusAcceptanceLabelExpectedPresent,
		ExpectedKind:            SemanticCandidateKindReference,
		RequiredEvidenceRefs:    []string{"evidence-001"},
		Labeler:                 "fixture-human",
		IndependenceAttestation: corpusAcceptanceIndependentProvenance,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot upgrade generated label records") {
		t.Fatalf("expected generated records upgrade rejection, got %v", err)
	}
	after, err := os.ReadFile(recordsPath)
	if err != nil {
		t.Fatalf("read records after upgrade: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("generated records were mutated by rejected upgrade:\nbefore=%s\nafter=%s", string(before), string(after))
	}
}

func TestCorpusAcceptanceLabelRecordRejectsCorruptedCandidateRefMap(t *testing.T) {
	root, _, packet, _ := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	packet.Sources[0].Candidates[1].SourceDocumentID = packet.Sources[0].Candidates[0].SourceDocumentID
	writeDocumentsTestJSON(t, filepath.Join(root, "labeling", "corpus-acceptance-labeling", "labeling-packet.json"), packet)
	recordsPath := filepath.Join(root, "guided-records.json")
	nextOut := filepath.Join(root, "next")
	if _, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, nextOut); err != nil {
		t.Fatalf("build label next: %v", err)
	}
	mapPath := filepath.Join(nextOut, corpusAcceptanceLabelNextDirName, "label-next-map.json")
	var labelMap CorpusAcceptanceLabelNextMap
	mapData, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatalf("read label map: %v", err)
	}
	if err := json.Unmarshal(mapData, &labelMap); err != nil {
		t.Fatalf("decode label map: %v", err)
	}
	labelMap.Cases[0].Candidates[0].CandidateID = packet.Sources[0].Candidates[1].CandidateID
	labelMap.Cases[0].Candidates[0].SourceDocumentID = packet.Sources[0].Candidates[1].SourceDocumentID
	writeDocumentsTestJSON(t, mapPath, labelMap)

	_, err = RecordCorpusAcceptanceLabel(filepath.Join(root, "labeling"), recordsPath, mapPath, CorpusAcceptanceLabelRecordInput{
		CaseRef:                 "case-001",
		CandidateRef:            "candidate-001",
		Decision:                CorpusAcceptanceLabelUncertain,
		Labeler:                 "fixture-human",
		IndependenceAttestation: corpusAcceptanceIndependentProvenance,
	})
	if err == nil || !strings.Contains(err.Error(), "label next map candidate does not match current packet") {
		t.Fatalf("expected corrupted candidate map error, got %v", err)
	}
}

func TestCorpusAcceptanceLabelNextDoesNotSkipInvalidExistingRecords(t *testing.T) {
	root, _, _, _ := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	recordsPath := filepath.Join(root, "guided-records.json")
	writeDocumentsTestJSON(t, recordsPath, CorpusAcceptanceLabelRecords{
		SchemaVersion: CorpusAcceptanceLabelRecordsSchemaVersion,
		SuiteID:       "guided-heldout",
		SuiteKind:     CorpusAcceptanceSuiteHeldOut,
		Provenance:    CorpusAcceptanceProvenance{Labeler: "fixture-human", Independence: corpusAcceptanceIndependentProvenance},
		MinEvalCount:  1,
		Records: []CorpusAcceptanceLabelRecordItem{{
			RecordID: "rec-invalid",
			CaseID:   "case-001",
			Decision: CorpusAcceptanceLabelExpectedPresent,
		}},
	})
	summary, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "next"))
	if err != nil {
		t.Fatalf("build label next: %v", err)
	}
	if summary.NextItem == nil || summary.NextItem.CaseRef != "case-001" || len(summary.Blockers) == 0 {
		t.Fatalf("invalid record should not hide queue item: %+v", summary)
	}
}

func TestCorpusAcceptanceLabelNextDoesNotSkipWrongSchemaRecords(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	records := readLabelRecordingFixtureRecords(t, recordsPath)
	records.SchemaVersion = "corpus-acceptance-label-records/v0.0"
	writeDocumentsTestJSON(t, recordsPath, records)
	summary, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "next"))
	if err != nil {
		t.Fatalf("build label next: %v", err)
	}
	if summary.NextItem == nil || !stringListContains(summary.Blockers, "unsupported_records_schema") {
		t.Fatalf("wrong schema should block skip and expose next item: %+v", summary)
	}
}

func TestCorpusAcceptanceLabelNextSkipsValidExistingRecordsWithRawCandidateIDs(t *testing.T) {
	root, _, _, recordsPath := writeCorpusAcceptanceLabelRecordingFixture(t, corpusAcceptanceIndependentProvenance)
	summary, _, err := BuildCorpusAcceptanceLabelNext(filepath.Join(root, "labeling"), recordsPath, filepath.Join(root, "next"))
	if err != nil {
		t.Fatalf("build label next: %v", err)
	}
	if summary.QueueState != corpusAcceptanceLabelQueueEmpty || summary.NextItem != nil || summary.RemainingCount != 0 {
		t.Fatalf("valid existing records should drain queue: %+v", summary)
	}
}

func readLabelNextSummary(t *testing.T, path string) CorpusAcceptanceLabelNextSummary {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var summary CorpusAcceptanceLabelNextSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	return summary
}
