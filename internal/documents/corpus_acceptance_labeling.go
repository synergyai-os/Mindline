package documents

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	corpusAcceptanceLabelingDirName     = "corpus-acceptance-labeling"
	corpusAcceptanceLabelingRequired    = "labeling_required"
	corpusAcceptanceGeneratedProvenance = "generated_template_requires_human_labeling"
)

func BuildCorpusAcceptanceLabelingPacket(pressurePath, outDir string) (CorpusAcceptanceLabelingPacket, CorpusAcceptanceAnswerKey, error) {
	pressureRoot, summary, err := readCorpusAcceptancePressureSummary(pressurePath)
	if err != nil {
		return CorpusAcceptanceLabelingPacket{}, CorpusAcceptanceAnswerKey{}, err
	}
	packet, template, err := buildCorpusAcceptanceLabelingPacket(pressureRoot, summary)
	if err != nil {
		return CorpusAcceptanceLabelingPacket{}, CorpusAcceptanceAnswerKey{}, err
	}
	if err := WriteCorpusAcceptanceLabelingPacket(outDir, packet, template); err != nil {
		return CorpusAcceptanceLabelingPacket{}, CorpusAcceptanceAnswerKey{}, err
	}
	return packet, template, nil
}

func buildCorpusAcceptanceLabelingPacket(root string, summary CorpusPressureSummary) (CorpusAcceptanceLabelingPacket, CorpusAcceptanceAnswerKey, error) {
	packet := CorpusAcceptanceLabelingPacket{
		SchemaVersion:             CorpusAcceptanceLabelingPacketSchemaVersion,
		PacketID:                  "labeling-" + summary.ReplayFingerprint,
		CorpusID:                  summary.CorpusID,
		CorpusFingerprint:         summary.CorpusFingerprint,
		CommandConfigFingerprint:  summary.CommandConfigFingerprint,
		PressureReplayFingerprint: summary.ReplayFingerprint,
		LabelingStatus:            corpusAcceptanceLabelingRequired,
		HeldOutReady:              false,
		SourceCount:               len(summary.Sources),
		RelationCoverage: CorpusAcceptanceLabelingRelationCoverage{
			RelationTypeCounts:   copyCorpusRelationTypeCounts(summary.RelationTypeCounts),
			RelationStatusCounts: copyReviewStatusCounts(summary.RelationStatusCounts),
		},
		Guardrails: summary.Guardrails,
		ClaimBoundaries: []string{
			"generated_templates_are_not_independent_answer_keys",
			"held_out_accuracy_not_claimed",
			"generalization_not_claimed",
			"dec64_not_claimed",
			"destination_write_readiness_not_claimed",
		},
		Instructions: []string{
			"Use each case_id to label the corresponding local source and candidates.",
			"Edit the answer-key template only after independent human review.",
			"Set provenance.independence to not_generated_from_evaluated_run only when the labels were created independently from this evaluated run.",
			"Add expected_present outcomes for required candidates and expected_absent outcomes for negative controls.",
			"Use notes for uncertain or abstained cases; do not convert uncertainty into a pass claim.",
		},
		AnswerKeyTemplatePath: filepath.ToSlash(filepath.Join(corpusAcceptanceLabelingDirName, "answer-key-template.json")),
		ReportPath:            filepath.ToSlash(filepath.Join(corpusAcceptanceLabelingDirName, "labeling-report.md")),
	}
	graph, ok := readCorpusAcceptanceGraphSummary(root, summary.GraphSummaryPath)
	if ok {
		packet.RelationCoverage.RelationCount = graph.RelationCount
		packet.RelationCoverage.RelationTypeCounts = copyCorpusRelationTypeCounts(graph.RelationTypeCounts)
		packet.RelationCoverage.RelationStatusCounts = copyReviewStatusCounts(graph.RelationStatusCounts)
	} else {
		packet.RelationCoverage.RelationCount = summary.GraphRelationCount
	}

	template := CorpusAcceptanceAnswerKey{
		SchemaVersion:            CorpusAcceptanceAnswerKeySchemaVersion,
		SuiteID:                  packet.PacketID + "-template",
		SuiteKind:                CorpusAcceptanceSuiteHeldOut,
		Provenance:               CorpusAcceptanceProvenance{Labeler: "human_labeler_required", Independence: corpusAcceptanceGeneratedProvenance},
		CorpusID:                 summary.CorpusID,
		CorpusFingerprint:        summary.CorpusFingerprint,
		CommandConfigFingerprint: summary.CommandConfigFingerprint,
		MinEvalCount:             CorpusAcceptanceDEC64MinEvalCount,
		CoverageRequirements: CorpusAcceptanceCoverage{
			MinSourceCount: len(summary.Sources),
		},
	}

	sources := append([]CorpusPressureSourceResult{}, summary.Sources...)
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].SourceID < sources[j].SourceID
	})
	for idx, source := range sources {
		labelingSource, sourceDocumentID, err := buildCorpusAcceptanceLabelingSource(root, source, idx+1)
		if err != nil {
			return CorpusAcceptanceLabelingPacket{}, CorpusAcceptanceAnswerKey{}, err
		}
		packet.CandidateCount += labelingSource.CandidateCount
		packet.Sources = append(packet.Sources, labelingSource)
		template.Sources = append(template.Sources, CorpusAcceptanceAnswerKeySource{
			SourceID:         source.SourceID,
			SourceDocumentID: sourceDocumentID,
			ExpectedOutcomes: []SemanticExpectedOutcome{},
		})
		mergeLabelingCoverage(&template.CoverageRequirements, labelingSource)
	}
	return packet, template, nil
}

func buildCorpusAcceptanceLabelingSource(root string, source CorpusPressureSourceResult, ordinal int) (CorpusAcceptanceLabelingSource, string, error) {
	labelingSource := CorpusAcceptanceLabelingSource{
		CaseID:            fmt.Sprintf("case-%03d", ordinal),
		SourceID:          source.SourceID,
		SourceContentHash: source.SourceContentHash,
		SourceState:       source.State,
		ReasonCode:        source.ReasonCode,
		CandidateKinds:    map[SemanticCandidateKind]int{},
	}
	if strings.TrimSpace(source.SemanticRunDir) == "" || source.CandidateCount == 0 {
		labelingSource.CandidateKinds = copySemanticCandidateKindCounts(source.CandidateKindCounts)
		labelingSource.CandidateCount = source.CandidateCount
		return labelingSource, "", nil
	}
	semanticRoot, err := containedCorpusAcceptancePath(root, source.SemanticRunDir)
	if err != nil {
		return CorpusAcceptanceLabelingSource{}, "", err
	}
	_, candidates, _, err := readSemanticAcceptanceInput(semanticRoot)
	if err != nil {
		return CorpusAcceptanceLabelingSource{}, "", err
	}
	sourceDocumentID := corpusAcceptanceArtifactSourceDocumentID(candidates, source.SourceID)
	for _, candidate := range candidates {
		ref := CorpusAcceptanceLabelingCandidateReference{
			CandidateID:      candidate.CandidateID,
			SourceDocumentID: candidateSourceDocumentID(candidate),
			CandidateKind:    candidate.CandidateKind,
			ReviewStatus:     candidate.ReviewStatus,
			Confidence:       candidate.Confidence,
			EvidenceNodes:    append([]string{}, candidate.EvidenceNodes...),
			EvidenceRanges:   append([]SemanticEvidenceRange{}, candidate.EvidenceRanges...),
		}
		labelingSource.Candidates = append(labelingSource.Candidates, ref)
		labelingSource.CandidateKinds[candidate.CandidateKind]++
	}
	labelingSource.CandidateCount = len(labelingSource.Candidates)
	return labelingSource, sourceDocumentID, nil
}

func mergeLabelingCoverage(coverage *CorpusAcceptanceCoverage, source CorpusAcceptanceLabelingSource) {
	for kind := range source.CandidateKinds {
		if !semanticKindListContains(coverage.CandidateKinds, kind) {
			coverage.CandidateKinds = append(coverage.CandidateKinds, kind)
		}
	}
	sort.SliceStable(coverage.CandidateKinds, func(i, j int) bool {
		return coverage.CandidateKinds[i] < coverage.CandidateKinds[j]
	})
}

func semanticKindListContains(values []SemanticCandidateKind, want SemanticCandidateKind) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func copySemanticCandidateKindCounts(values map[SemanticCandidateKind]int) map[SemanticCandidateKind]int {
	out := map[SemanticCandidateKind]int{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func copyCorpusRelationTypeCounts(values map[CorpusRelationType]int) map[CorpusRelationType]int {
	out := map[CorpusRelationType]int{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func copyReviewStatusCounts(values map[ReviewStatus]int) map[ReviewStatus]int {
	out := map[ReviewStatus]int{}
	for key, value := range values {
		out[key] = value
	}
	return out
}
