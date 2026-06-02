package documents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusAcceptanceLabelingPacketWritesRedactedReportAndTemplate(t *testing.T) {
	root, pressure, _ := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindReference, ReviewStatusReady)}, func(summary *CorpusPressureSummary) {
		summary.Sources[0].SourceID = "slack-team-d05h529m32n-1780127914236669"
		summary.RelationTypeCounts = map[CorpusRelationType]int{CorpusRelationPossibleDuplicate: 1}
		summary.RelationStatusCounts = map[ReviewStatus]int{ReviewStatusReady: 1}
		summary.GraphRelationCount = 1
	})

	packet, template, err := BuildCorpusAcceptanceLabelingPacket(root, filepath.Join(root, "labeling"))
	if err != nil {
		t.Fatalf("build corpus acceptance labeling packet: %v", err)
	}
	if packet.SchemaVersion != CorpusAcceptanceLabelingPacketSchemaVersion || packet.HeldOutReady || packet.LabelingStatus != corpusAcceptanceLabelingRequired {
		t.Fatalf("expected labeling-required packet, got schema=%s held_out=%t status=%s", packet.SchemaVersion, packet.HeldOutReady, packet.LabelingStatus)
	}
	if packet.SourceCount != 1 || packet.CandidateCount != 1 || packet.Sources[0].CaseID != "case-001" || packet.Sources[0].SourceID != pressure.Sources[0].SourceID {
		t.Fatalf("unexpected packet source accounting: %+v", packet)
	}
	if packet.Sources[0].CandidateKinds[SemanticCandidateKindReference] != 1 {
		t.Fatalf("candidate kind count should come from semantic artifacts without double-counting, got %+v", packet.Sources[0].CandidateKinds)
	}
	if template.Provenance.Independence != corpusAcceptanceGeneratedProvenance || template.Provenance.Labeler != "human_labeler_required" || len(template.Sources[0].ExpectedOutcomes) != 0 {
		t.Fatalf("expected generated labeler-owned template, got %+v", template.Provenance)
	}
	reportData, err := os.ReadFile(filepath.Join(root, "labeling", corpusAcceptanceLabelingDirName, "labeling-report.md"))
	if err != nil {
		t.Fatalf("read labeling report: %v", err)
	}
	report := string(reportData)
	if !strings.Contains(report, "case-001") {
		t.Fatalf("expected redacted case id in report: %s", report)
	}
	if strings.Contains(report, "slack-") || strings.Contains(report, "d05h529m32n") || strings.Contains(report, root) || strings.Contains(report, "slack.com") {
		t.Fatalf("labeling report leaked private identifiers: %s", report)
	}
}

func TestCorpusAcceptanceLabelingTemplateCannotPassHeldOutAcceptance(t *testing.T) {
	root, _, _ := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, nil)
	_, _, err := BuildCorpusAcceptanceLabelingPacket(root, filepath.Join(root, "labeling"))
	if err != nil {
		t.Fatalf("build corpus acceptance labeling packet: %v", err)
	}

	summary, err := BuildCorpusAcceptanceBenchmark(root, filepath.Join(root, "labeling", corpusAcceptanceLabelingDirName, "answer-key-template.json"), filepath.Join(root, "benchmark"), CorpusAcceptanceBenchmarkOptions{Threshold: 0.98, HeldOut: true})
	if err != nil {
		t.Fatalf("build corpus acceptance benchmark: %v", err)
	}
	if summary.SuiteValid || summary.DEC64Eligible || !stringListContains(summary.SuiteValidityBlockers, "answer_key_not_independent") {
		t.Fatalf("generated template must remain blocked for held-out acceptance, valid=%t eligible=%t blockers=%v", summary.SuiteValid, summary.DEC64Eligible, summary.SuiteValidityBlockers)
	}
}
