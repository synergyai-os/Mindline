package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceMeaningPacketBuildsCompressedReviewPacketWithoutRawExcerpts(t *testing.T) {
	root := t.TempDir()
	pressureOut := filepath.Join(root, "pressure")
	pressure, _, err := BuildCorpusPressure(fixturePath(t, "markdown"), pressureOut, CorpusPressureOptions{})
	if err != nil {
		t.Fatalf("build corpus pressure: %v", err)
	}
	if pressure.GraphAtomCount == 0 {
		t.Fatalf("fixture should produce graph atoms: %+v", pressure)
	}

	packetOut := filepath.Join(root, "packet")
	packet, groups, err := BuildSourceMeaningPacket(pressureOut, packetOut)
	if err != nil {
		t.Fatalf("build source meaning packet: %v", err)
	}

	if packet.SchemaVersion != SourceMeaningPacketSchemaVersion {
		t.Fatalf("unexpected schema version %s", packet.SchemaVersion)
	}
	if packet.ReviewGroupCount == 0 || packet.ReviewGroupCount >= packet.AtomCount {
		t.Fatalf("expected compressed groups, got atoms=%d groups=%d", packet.AtomCount, packet.ReviewGroupCount)
	}
	if packet.EvidenceOrBlockerGroupRatio != 1 {
		t.Fatalf("expected every group to have evidence or blockers: %+v", packet)
	}
	if packet.Guardrails.DestinationWrites != 0 || packet.Guardrails.ProductBrainWrites != 0 || packet.Guardrails.TolariaWrites != 0 {
		t.Fatalf("expected zero write guardrails: %+v", packet.Guardrails)
	}
	if packet.Guardrails.HostedInferenceCalls != 0 || packet.Guardrails.HostedTelemetryExports != 0 {
		t.Fatalf("expected zero hosted side effects: %+v", packet.Guardrails)
	}
	if len(groups) != packet.ReviewGroupCount {
		t.Fatalf("expected group count to match summary")
	}
	for _, group := range groups {
		if group.WriteEligible {
			t.Fatalf("group should be write-ineligible: %+v", group)
		}
		if group.EvidenceReferenceCount == 0 && len(group.BlockerReasons) == 0 {
			t.Fatalf("group lacks evidence and blockers: %+v", group)
		}
	}

	packetText := allPacketText(t, filepath.Join(packetOut, SourceMeaningPacketDirName))
	for _, denied := range sourceFixtureDeniedLines(t, fixturePath(t, "markdown")) {
		if strings.Contains(packetText, denied) {
			t.Fatalf("packet leaked raw source line %q\n%s", denied, packetText)
		}
	}
	for _, expected := range []string{"review-packet.md", "evidence-map.json", "blocked-items.json"} {
		if _, err := os.Stat(filepath.Join(packetOut, SourceMeaningPacketDirName, expected)); err != nil {
			t.Fatalf("missing packet artifact %s: %v", expected, err)
		}
	}
}

func TestSourceMeaningPacketRoutesDuplicatePressureToNeedsReviewWithoutProposal(t *testing.T) {
	pressure := CorpusPressureSummary{
		CorpusID:                 "corpus-test",
		SourceCount:              2,
		ProcessedSourceCount:     2,
		CorpusFingerprint:        "corpus-fingerprint",
		CommandConfigFingerprint: "command-config",
		ReplayFingerprint:        "pressure-replay",
	}
	graph := CorpusGraphSummary{
		AtomCount:          2,
		RelationCount:      1,
		RelationTypeCounts: map[CorpusRelationType]int{CorpusRelationPossibleDuplicate: 1},
		ReplayFingerprint:  "graph-replay",
	}
	atomsBySource := map[string][]CorpusGraphAtom{
		"source-a": {sourceMeaningPacketTestAtom("atom-a", "source-a", SemanticCandidateKindDecision)},
		"source-b": {sourceMeaningPacketTestAtom("atom-b", "source-b", SemanticCandidateKindDecision)},
	}
	relationsBySource := map[string][]CorpusGraphRelation{
		"source-a": {{
			RelationID:   "rel-duplicate",
			RelationType: CorpusRelationPossibleDuplicate,
			FromAtomID:   "atom-a",
			ToAtomID:     "atom-b",
			ReviewStatus: ReviewStatusReady,
		}},
	}

	build := buildSourceMeaningPacket(pressure, graph, atomsBySource, relationsBySource)

	if build.Summary.NeedsReviewGroupCount != 1 || build.Summary.ReadyGroupCount != 0 || build.Summary.ProposalCount != 0 {
		t.Fatalf("expected duplicate pressure to require review without proposals: %+v", build.Summary)
	}
	if len(build.Groups) != 1 || build.Groups[0].Section != SourceMeaningPacketSectionNeedsReview {
		t.Fatalf("expected needs-review group, got %+v", build.Groups)
	}
	if build.Summary.Groups[0].ProposalPath != "" {
		t.Fatalf("needs-review group should not expose a proposal path: %+v", build.Summary.Groups[0])
	}
}

func TestSourceMeaningPacketPreservesNeedsReviewStatusWithoutProposal(t *testing.T) {
	pressure := CorpusPressureSummary{CorpusID: "corpus-test", SourceCount: 1, ProcessedSourceCount: 1}
	graph := CorpusGraphSummary{AtomCount: 1, RelationCount: 1}
	atom := sourceMeaningPacketTestAtom("atom-a", "source-a", SemanticCandidateKindIssue)
	atom.ReviewStatus = ReviewStatusNeedsReview
	relationsBySource := map[string][]CorpusGraphRelation{
		"source-a": {{
			RelationID:   "rel-review",
			RelationType: CorpusRelationSameTopicAs,
			FromAtomID:   "atom-a",
			ToAtomID:     "atom-a",
			ReviewStatus: ReviewStatusNeedsReview,
		}},
	}

	build := buildSourceMeaningPacket(pressure, graph, map[string][]CorpusGraphAtom{"source-a": {atom}}, relationsBySource)

	if build.Summary.NeedsReviewGroupCount != 1 || build.Summary.ProposalCount != 0 || build.Summary.ReviewBurdenCount != 1 {
		t.Fatalf("expected needs-review group without proposal: %+v", build.Summary)
	}
	if len(build.Groups) != 1 || build.Groups[0].Section != SourceMeaningPacketSectionNeedsReview {
		t.Fatalf("expected needs-review section, got %+v", build.Groups)
	}
}

func TestSourceMeaningPacketClustersReviewPressureBeforeChunking(t *testing.T) {
	atoms := make([]CorpusGraphAtom, 0, 25)
	for i := 1; i <= 25; i++ {
		atomID := fmt.Sprintf("atom-%02d", i)
		sourceID := fmt.Sprintf("source-%02d", i)
		atoms = append(atoms, sourceMeaningPacketTestAtom(atomID, sourceID, SemanticCandidateKindTopic))
	}
	relationsBySource := map[string][]CorpusGraphRelation{
		"source-01": {{
			RelationID:   "rel-duplicate-spread",
			RelationType: CorpusRelationPossibleDuplicate,
			FromAtomID:   "atom-01",
			ToAtomID:     "atom-25",
			ReviewStatus: ReviewStatusReady,
		}},
	}

	build := buildSourceMeaningPacket(
		CorpusPressureSummary{CorpusID: "corpus-test", SourceCount: 25, ProcessedSourceCount: 25},
		CorpusGraphSummary{AtomCount: 25, RelationCount: 1},
		map[string][]CorpusGraphAtom{"sources": atoms},
		relationsBySource,
	)

	if build.Summary.ReviewGroupCount != 3 || build.Summary.NeedsReviewGroupCount != 1 || build.Summary.ReadyGroupCount != 2 {
		t.Fatalf("expected duplicate-pressure atoms clustered into one review group: %+v", build.Summary)
	}
	if build.Summary.ReviewBurdenRatio > 0.35 {
		t.Fatalf("expected bounded review burden, got %+v", build.Summary)
	}
}

func TestSourceMeaningPacketReportsScalePartialWhenGroupBudgetIsReached(t *testing.T) {
	atoms := make([]CorpusGraphAtom, 0, 25)
	for i := 1; i <= 25; i++ {
		atomID := fmt.Sprintf("atom-%02d", i)
		sourceID := fmt.Sprintf("source-%02d", i)
		atoms = append(atoms, sourceMeaningPacketTestAtom(atomID, sourceID, SemanticCandidateKindTopic))
	}

	build := buildSourceMeaningPacket(
		CorpusPressureSummary{
			CorpusID:             "corpus-test",
			SourceCount:          25,
			ProcessedSourceCount: 25,
			ScaleBudget:          CorpusPressureScaleBudget{MaxPacketReviewGroups: 1},
		},
		CorpusGraphSummary{AtomCount: 25},
		map[string][]CorpusGraphAtom{"sources": atoms},
		nil,
	)

	if build.Summary.ReviewGroupCount != 3 || len(build.Groups) != 1 {
		t.Fatalf("expected group budget to cap packet groups: %+v", build.Summary)
	}
	if build.Summary.ScaleStatus != "scale_partial" || !containsCorpusPressureString(build.Summary.ScaleReasonCodes, "scale_packet_group_limit") {
		t.Fatalf("expected packet scale partial, got %+v", build.Summary)
	}
	if build.Summary.OmittedAtomCount != 13 {
		t.Fatalf("expected 13 omitted atoms after first 12-atom group, got %+v", build.Summary)
	}
	if build.Summary.AtomCompressionRatio != 0.88 {
		t.Fatalf("summary metrics should account for all generated groups before cap, got %+v", build.Summary)
	}
}

func allPacketText(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("read packet files: %v", err)
	}
	return b.String()
}

func sourceFixtureDeniedLines(t *testing.T, root string) []string {
	t.Helper()
	lines := []string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if len(line) < 32 || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "-") {
				continue
			}
			lines = append(lines, line)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read source fixture lines: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("fixture denied line set is empty")
	}
	return lines
}

func sourceMeaningPacketTestAtom(atomID, sourceID string, kind SemanticCandidateKind) CorpusGraphAtom {
	return CorpusGraphAtom{
		AtomID:           atomID,
		CorpusID:         "corpus-test",
		SourceID:         sourceID,
		SourceKind:       "fixture",
		SourceDocumentID: sourceID + "-doc",
		CandidateKind:    kind,
		ReviewStatus:     ReviewStatusReady,
		Confidence:       ConfidenceHigh,
		Title:            atomID + " title",
		Summary:          atomID + " summary",
		LineStart:        1,
		LineEnd:          2,
		ContentHash:      atomID + "-hash",
	}
}
