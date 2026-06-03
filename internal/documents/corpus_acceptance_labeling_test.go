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
	if packet.Sources[0].SourcePath != pressure.Sources[0].SourcePath || packet.Sources[0].SemanticRunDir != pressure.Sources[0].SemanticRunDir {
		t.Fatalf("labeling case should preserve artifact paths, got source=%q semantic=%q", packet.Sources[0].SourcePath, packet.Sources[0].SemanticRunDir)
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

func TestCorpusAcceptanceLabelingPreservesSuffixedRunDirPaths(t *testing.T) {
	root, _, _ := writeCorpusAcceptanceFixture(t, []SemanticCandidate{}, func(summary *CorpusPressureSummary) {
		summary.Sources[0].SourceID = "source-demo"
		summary.Sources[0].SourcePath = "sources/source-demo-pressure/source.md"
		summary.Sources[0].SemanticRunDir = "sources/source-demo-pressure"
	})

	packet, _, err := BuildCorpusAcceptanceLabelingPacket(root, filepath.Join(root, "labeling"))
	if err != nil {
		t.Fatalf("build corpus acceptance labeling packet: %v", err)
	}
	if packet.Sources[0].SourcePath != "sources/source-demo-pressure/source.md" || packet.Sources[0].SemanticRunDir != "sources/source-demo-pressure" {
		t.Fatalf("expected suffixed artifact paths, got source=%q semantic=%q", packet.Sources[0].SourcePath, packet.Sources[0].SemanticRunDir)
	}
}

func TestCorpusAcceptanceLabelingSeedRedactsPrivateMarkersAndKeepsPrivateMap(t *testing.T) {
	root, _, _ := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, func(summary *CorpusPressureSummary) {
		summary.Sources[0].SourcePath = "sources/source-demo/WP-353-source.md"
	})

	_, _, err := BuildCorpusAcceptanceLabelingPacket(root, filepath.Join(root, "labeling-unfiltered"))
	if err == nil || !strings.Contains(err.Error(), "private marker") {
		t.Fatalf("expected unfiltered labeling to fail closed on private marker, got %v", err)
	}

	packet, _, seed, err := BuildCorpusAcceptanceLabelingPacketWithOptions(root, filepath.Join(root, "labeling-seed"), CorpusAcceptanceLabelingOptions{Seed: true, MaxCases: 1})
	if err != nil {
		t.Fatalf("build seed labeling packet: %v", err)
	}
	if seed == nil || !seed.SeedMode || seed.SelectedCaseCount != 1 || seed.SelectedCandidateCount != 1 {
		t.Fatalf("unexpected seed summary: %+v", seed)
	}
	if packet.SourceCount != 1 || packet.CandidateCount != 1 || packet.Sources[0].SourceID != "source-001" || packet.Sources[0].SourcePath != "" {
		t.Fatalf("seed packet should use safe aliases, got %+v", packet.Sources[0])
	}
	if packet.Sources[0].Candidates[0].CandidateID != "candidate-001" || packet.Sources[0].Candidates[0].SourceDocumentID != "doc-001" {
		t.Fatalf("seed candidate should use safe aliases, got %+v", packet.Sources[0].Candidates[0])
	}
	reportData, err := os.ReadFile(filepath.Join(root, "labeling-seed", corpusAcceptanceLabelingDirName, "seed-report.md"))
	if err != nil {
		t.Fatalf("read seed report: %v", err)
	}
	summaryData, err := os.ReadFile(filepath.Join(root, "labeling-seed", corpusAcceptanceLabelingDirName, "seed-summary.json"))
	if err != nil {
		t.Fatalf("read seed summary: %v", err)
	}
	durable := string(reportData) + "\n" + string(summaryData)
	if strings.Contains(durable, "WP-353") || strings.Contains(durable, "source-demo/WP-353") || strings.Contains(durable, "candidate-demo") || strings.Contains(durable, "node-demo") {
		t.Fatalf("seed durable artifacts leaked private refs:\n%s", durable)
	}
	mapData, err := os.ReadFile(filepath.Join(root, "labeling-seed", corpusAcceptanceLabelingDirName, "seed-private-map.json"))
	if err != nil {
		t.Fatalf("read seed private map: %v", err)
	}
	if !strings.Contains(string(mapData), "WP-353") || !strings.Contains(string(mapData), "source-demo") {
		t.Fatalf("private map should preserve operational refs, got %s", string(mapData))
	}
}

func TestCorpusAcceptanceLabelingSeedWorksWithLabelWorkflow(t *testing.T) {
	root, _, _ := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, func(summary *CorpusPressureSummary) {
		summary.Sources[0].SourcePath = "sources/source-demo/WP-353-source.md"
	})
	labelingOut := filepath.Join(root, "labeling-seed")
	_, _, _, err := BuildCorpusAcceptanceLabelingPacketWithOptions(root, labelingOut, CorpusAcceptanceLabelingOptions{Seed: true, MaxCases: 1})
	if err != nil {
		t.Fatalf("build seed labeling packet: %v", err)
	}
	recordsPath := filepath.Join(root, "records.json")
	nextOut := filepath.Join(root, "next")
	nextSummary, _, err := BuildCorpusAcceptanceLabelNext(labelingOut, recordsPath, nextOut)
	if err != nil {
		t.Fatalf("build label next: %v", err)
	}
	if nextSummary.NextItem == nil || nextSummary.NextItem.CaseRef != "case-001" || nextSummary.NextItem.CandidateRef != "candidate-001" {
		t.Fatalf("unexpected next item: %+v", nextSummary.NextItem)
	}
	_, err = RecordCorpusAcceptanceLabel(labelingOut, recordsPath, filepath.Join(nextOut, corpusAcceptanceLabelNextDirName, "label-next-map.json"), CorpusAcceptanceLabelRecordInput{
		CaseRef:                 "case-001",
		CandidateRef:            "candidate-001",
		Decision:                CorpusAcceptanceLabelExpectedPresent,
		ExpectedKind:            SemanticCandidateKindAction,
		RequiredEvidenceRefs:    []string{"evidence-001"},
		Labeler:                 "fixture-human",
		IndependenceAttestation: corpusAcceptanceIndependentProvenance,
	})
	if err != nil {
		t.Fatalf("record seed label: %v", err)
	}
	recording, _, err := BuildCorpusAcceptanceLabelRecording(labelingOut, recordsPath, filepath.Join(root, "recording"))
	if err != nil {
		t.Fatalf("apply seed label records: %v", err)
	}
	if recording.RecordCount != 1 || recording.EvalCount != 1 {
		t.Fatalf("unexpected recording summary: %+v", recording)
	}
}

func TestCorpusAcceptanceLabelingSeedRejectsCurrentWorkspaceOutput(t *testing.T) {
	root, _, _ := writeCorpusAcceptanceFixture(t, []SemanticCandidate{corpusAcceptanceCandidate(t, SemanticCandidateKindAction, ReviewStatusReady)}, nil)
	workspace := t.TempDir()
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	defer func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}()

	_, _, _, err = BuildCorpusAcceptanceLabelingPacketWithOptions(root, filepath.Join(workspace, "seed-out"), CorpusAcceptanceLabelingOptions{Seed: true, MaxCases: 1})
	if err == nil || !strings.Contains(err.Error(), "outside the current workspace") {
		t.Fatalf("expected seed output under workspace to fail closed, got %v", err)
	}
}

func TestCorpusAcceptanceLabelSeedPrioritizesEdgeBucketsBeforeSourceGroupFill(t *testing.T) {
	var candidates []corpusAcceptanceLabelSeedCandidate
	for i := 0; i < 20; i++ {
		candidates = append(candidates, corpusAcceptanceLabelSeedCandidate{
			sourceIndex:      i,
			candidateIndex:   0,
			sourceGroup:      "group-" + string(rune('a'+i)),
			candidateKind:    SemanticCandidateKindReference,
			confidence:       ConfidenceMedium,
			reviewStatus:     ReviewStatusReady,
			sourceState:      CorpusPressureSourceProcessed,
			rationaleBuckets: []string{"source_group", "kind:reference_candidate"},
		})
	}
	candidates = append(candidates, corpusAcceptanceLabelSeedCandidate{
		sourceIndex:      21,
		candidateIndex:   0,
		sourceGroup:      "group-edge",
		candidateKind:    SemanticCandidateKindIssue,
		confidence:       ConfidenceLow,
		reviewStatus:     ReviewStatusNeedsReview,
		sourceState:      CorpusPressureSourceProcessed,
		rationaleBuckets: []string{"kind:issue_candidate", "fallback_or_needs_review"},
	})
	candidates = append(candidates, corpusAcceptanceLabelSeedCandidate{
		sourceIndex:      22,
		candidateIndex:   -1,
		sourceGroup:      "group-zero",
		sourceState:      CorpusPressureSourceExcluded,
		zeroCandidate:    true,
		rationaleBuckets: []string{"zero_candidate_source_review"},
	})

	selected := selectCorpusAcceptanceLabelSeedCandidates(candidates, 5)
	hasIssue := false
	hasZero := false
	for _, candidate := range selected {
		if candidate.candidateKind == SemanticCandidateKindIssue {
			hasIssue = true
		}
		if candidate.zeroCandidate {
			hasZero = true
		}
	}
	if !hasIssue || !hasZero {
		t.Fatalf("expected issue and zero-candidate edge buckets before source group fill, got %+v", selected)
	}
}
