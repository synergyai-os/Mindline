package documents

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	corpusAcceptanceLabelingDirName           = "corpus-acceptance-labeling"
	corpusAcceptanceLabelingRequired          = "labeling_required"
	corpusAcceptanceGeneratedProvenance       = "generated_template_requires_human_labeling"
	corpusAcceptanceLabelSeedSelectionVersion = "corpus-acceptance-label-seed-selection/v0.1"
)

func BuildCorpusAcceptanceLabelingPacket(pressurePath, outDir string) (CorpusAcceptanceLabelingPacket, CorpusAcceptanceAnswerKey, error) {
	packet, template, _, err := BuildCorpusAcceptanceLabelingPacketWithOptions(pressurePath, outDir, CorpusAcceptanceLabelingOptions{})
	return packet, template, err
}

func BuildCorpusAcceptanceLabelingPacketWithOptions(pressurePath, outDir string, options CorpusAcceptanceLabelingOptions) (CorpusAcceptanceLabelingPacket, CorpusAcceptanceAnswerKey, *CorpusAcceptanceLabelSeedSummary, error) {
	pressureRoot, summary, err := readCorpusAcceptancePressureSummary(pressurePath)
	if err != nil {
		return CorpusAcceptanceLabelingPacket{}, CorpusAcceptanceAnswerKey{}, nil, err
	}
	packet, template, err := buildCorpusAcceptanceLabelingPacket(pressureRoot, summary)
	if err != nil {
		return CorpusAcceptanceLabelingPacket{}, CorpusAcceptanceAnswerKey{}, nil, err
	}
	var seedSummary *CorpusAcceptanceLabelSeedSummary
	var privateMap *CorpusAcceptanceLabelSeedPrivateMap
	if options.Seed {
		packet, template, seedSummary, privateMap = buildCorpusAcceptanceLabelSeed(packet, template, options)
	}
	if err := WriteCorpusAcceptanceLabelingPacketWithSeed(outDir, packet, template, seedSummary, privateMap); err != nil {
		return CorpusAcceptanceLabelingPacket{}, CorpusAcceptanceAnswerKey{}, nil, err
	}
	return packet, template, seedSummary, nil
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
		LabelingPacketPath:    filepath.ToSlash(filepath.Join(corpusAcceptanceLabelingDirName, "labeling-packet.json")),
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

func CorpusAcceptanceLabelingOutputFor(packet CorpusAcceptanceLabelingPacket) CorpusAcceptanceLabelingOutputSummary {
	return CorpusAcceptanceLabelingOutputForSeed(packet, nil)
}

func CorpusAcceptanceLabelingOutputForSeed(packet CorpusAcceptanceLabelingPacket, seed *CorpusAcceptanceLabelSeedSummary) CorpusAcceptanceLabelingOutputSummary {
	output := CorpusAcceptanceLabelingOutputSummary{
		SchemaVersion:         packet.SchemaVersion,
		LabelingStatus:        packet.LabelingStatus,
		HeldOutReady:          packet.HeldOutReady,
		SourceCount:           packet.SourceCount,
		CandidateCount:        packet.CandidateCount,
		RelationCoverageCount: packet.RelationCoverage.RelationCount,
		LabelingPacketPath:    packet.LabelingPacketPath,
		AnswerKeyTemplatePath: packet.AnswerKeyTemplatePath,
		ReportPath:            packet.ReportPath,
		ClaimBoundaries:       append([]string{}, packet.ClaimBoundaries...),
	}
	if seed != nil {
		output.SeedMode = true
		output.SeedSelectedCount = seed.SelectedCaseCount
		output.SeedSummaryPath = filepath.ToSlash(filepath.Join(corpusAcceptanceLabelingDirName, "seed-summary.json"))
		output.SeedReportPath = seed.ReportPath
	}
	return CorpusAcceptanceLabelingOutputSummary{
		SchemaVersion:         output.SchemaVersion,
		LabelingStatus:        output.LabelingStatus,
		HeldOutReady:          output.HeldOutReady,
		SourceCount:           output.SourceCount,
		CandidateCount:        output.CandidateCount,
		RelationCoverageCount: output.RelationCoverageCount,
		LabelingPacketPath:    output.LabelingPacketPath,
		AnswerKeyTemplatePath: output.AnswerKeyTemplatePath,
		ReportPath:            output.ReportPath,
		SeedMode:              output.SeedMode,
		SeedSelectedCount:     output.SeedSelectedCount,
		SeedSummaryPath:       output.SeedSummaryPath,
		SeedReportPath:        output.SeedReportPath,
		ClaimBoundaries:       output.ClaimBoundaries,
	}
}

type corpusAcceptanceLabelSeedCandidate struct {
	sourceIndex      int
	candidateIndex   int
	sourceGroup      string
	candidateKind    SemanticCandidateKind
	confidence       Confidence
	reviewStatus     ReviewStatus
	sourceState      CorpusPressureSourceState
	zeroCandidate    bool
	rationaleBuckets []string
}

func buildCorpusAcceptanceLabelSeed(packet CorpusAcceptanceLabelingPacket, template CorpusAcceptanceAnswerKey, options CorpusAcceptanceLabelingOptions) (CorpusAcceptanceLabelingPacket, CorpusAcceptanceAnswerKey, *CorpusAcceptanceLabelSeedSummary, *CorpusAcceptanceLabelSeedPrivateMap) {
	maxCases := options.MaxCases
	if maxCases <= 0 {
		maxCases = 50
	}
	candidates := corpusAcceptanceLabelSeedCandidates(packet)
	selected := selectCorpusAcceptanceLabelSeedCandidates(candidates, maxCases)
	groupRefs := corpusAcceptanceLabelSeedGroupRefs(candidates)
	filtered := packet
	filtered.PacketID = packet.PacketID + "-seed"
	filtered.SourceCount = 0
	filtered.CandidateCount = 0
	filtered.Sources = []CorpusAcceptanceLabelingSource{}
	filtered.ClaimBoundaries = append(filtered.ClaimBoundaries, "seed_mode_private_safe_label_queue_only")
	filtered.ReportPath = filepath.ToSlash(filepath.Join(corpusAcceptanceLabelingDirName, "labeling-report.md"))

	filteredTemplate := template
	filteredTemplate.SuiteID = filtered.PacketID + "-template"
	filteredTemplate.Sources = []CorpusAcceptanceAnswerKeySource{}
	filteredTemplate.CoverageRequirements.MinSourceCount = 0
	filteredTemplate.CoverageRequirements.CandidateKinds = nil

	privateMap := &CorpusAcceptanceLabelSeedPrivateMap{
		SchemaVersion:            CorpusAcceptanceLabelSeedPrivateMapSchemaVersion,
		PacketID:                 filtered.PacketID,
		CorpusFingerprint:        packet.CorpusFingerprint,
		CommandConfigFingerprint: packet.CommandConfigFingerprint,
	}
	summary := &CorpusAcceptanceLabelSeedSummary{
		SchemaVersion:             CorpusAcceptanceLabelSeedSummarySchemaVersion,
		SelectionVersion:          corpusAcceptanceLabelSeedSelectionVersion,
		SeedMode:                  true,
		MaxCases:                  maxCases,
		EligibleCaseCount:         len(candidates),
		CorpusFingerprint:         packet.CorpusFingerprint,
		CommandConfigFingerprint:  packet.CommandConfigFingerprint,
		PressureReplayFingerprint: packet.PressureReplayFingerprint,
		Guardrails:                packet.Guardrails,
		LabelingPacketPath:        filtered.LabelingPacketPath,
		PrivateMapPath:            filepath.ToSlash(filepath.Join(corpusAcceptanceLabelingDirName, "seed-private-map.json")),
		ReportPath:                filepath.ToSlash(filepath.Join(corpusAcceptanceLabelingDirName, "seed-report.md")),
		ClaimBoundaries: []string{
			"label_seed_readiness_only",
			"answer_key_not_generated",
			"held_out_accuracy_not_claimed",
			"generalization_not_claimed",
			"dec64_not_claimed",
			"destination_write_readiness_not_claimed",
			"no_human_operation_not_claimed",
		},
		Coverage: CorpusAcceptanceLabelSeedCoverage{
			SourceGroupCounts:   map[string]int{},
			CandidateKindCounts: map[SemanticCandidateKind]int{},
			ConfidenceCounts:    map[string]int{},
			ReviewStatusCounts:  map[ReviewStatus]int{},
			SourceStateCounts:   map[CorpusPressureSourceState]int{},
			RationaleCounts:     map[string]int{},
		},
	}

	selectedBySource := map[int][]corpusAcceptanceLabelSeedCandidate{}
	for _, candidate := range selected {
		selectedBySource[candidate.sourceIndex] = append(selectedBySource[candidate.sourceIndex], candidate)
	}
	sourceIndexes := make([]int, 0, len(selectedBySource))
	for sourceIndex := range selectedBySource {
		sourceIndexes = append(sourceIndexes, sourceIndex)
	}
	sort.Ints(sourceIndexes)
	for _, sourceIndex := range sourceIndexes {
		sourceCandidates := selectedBySource[sourceIndex]
		sort.SliceStable(sourceCandidates, func(i, j int) bool {
			return sourceCandidates[i].candidateIndex < sourceCandidates[j].candidateIndex
		})
		source := packet.Sources[sourceIndex]
		caseRef := fmt.Sprintf("case-%03d", len(filtered.Sources)+1)
		sourceRef := fmt.Sprintf("source-%03d", len(filtered.Sources)+1)
		filteredSource := CorpusAcceptanceLabelingSource{
			CaseID:            caseRef,
			SourceID:          sourceRef,
			SourceContentHash: fmt.Sprintf("sha256:redacted-%03d", len(filtered.Sources)+1),
			SourceState:       source.SourceState,
			ReasonCode:        source.ReasonCode,
			CandidateKinds:    map[SemanticCandidateKind]int{},
		}
		mapCase := CorpusAcceptanceLabelSeedPrivateMapCase{
			CaseRef:                caseRef,
			OriginalCaseID:         source.CaseID,
			OriginalSourceID:       source.SourceID,
			OriginalSourcePath:     source.SourcePath,
			OriginalSemanticRunDir: source.SemanticRunDir,
		}
		for _, selectedCandidate := range sourceCandidates {
			seedCase := CorpusAcceptanceLabelSeedCase{
				CaseRef:          caseRef,
				SourceGroupRef:   groupRefs[selectedCandidate.sourceGroup],
				CandidateKind:    selectedCandidate.candidateKind,
				ConfidenceBucket: string(selectedCandidate.confidence),
				ReviewStatus:     selectedCandidate.reviewStatus,
				SourceState:      selectedCandidate.sourceState,
				RationaleBuckets: append([]string{}, selectedCandidate.rationaleBuckets...),
			}
			if selectedCandidate.zeroCandidate {
				summary.SelectedCases = append(summary.SelectedCases, seedCase)
				continue
			}
			originalCandidate := source.Candidates[selectedCandidate.candidateIndex]
			candidateRef := fmt.Sprintf("candidate-%03d", len(filteredSource.Candidates)+1)
			seedCase.CandidateRef = candidateRef
			evidenceNodes := make([]string, 0, len(originalCandidate.EvidenceNodes))
			for evidenceIndex := range originalCandidate.EvidenceNodes {
				evidenceNodes = append(evidenceNodes, fmt.Sprintf("evidence-node-%03d", evidenceIndex+1))
			}
			filteredCandidate := CorpusAcceptanceLabelingCandidateReference{
				CandidateID:      candidateRef,
				SourceDocumentID: fmt.Sprintf("doc-%03d", len(filtered.Sources)+1),
				CandidateKind:    originalCandidate.CandidateKind,
				ReviewStatus:     originalCandidate.ReviewStatus,
				Confidence:       originalCandidate.Confidence,
				EvidenceNodes:    evidenceNodes,
			}
			filteredSource.Candidates = append(filteredSource.Candidates, filteredCandidate)
			filteredSource.CandidateKinds[filteredCandidate.CandidateKind]++
			mapCase.Candidates = append(mapCase.Candidates, CorpusAcceptanceLabelSeedPrivateMapCandidate{
				CandidateRef:             candidateRef,
				OriginalCandidateID:      originalCandidate.CandidateID,
				OriginalSourceDocumentID: originalCandidate.SourceDocumentID,
				OriginalEvidenceNodes:    append([]string{}, originalCandidate.EvidenceNodes...),
			})
			summary.SelectedCases = append(summary.SelectedCases, seedCase)
			summary.SelectedCandidateCount++
			if !semanticKindListContains(filteredTemplate.CoverageRequirements.CandidateKinds, filteredCandidate.CandidateKind) {
				filteredTemplate.CoverageRequirements.CandidateKinds = append(filteredTemplate.CoverageRequirements.CandidateKinds, filteredCandidate.CandidateKind)
			}
		}
		filteredSource.CandidateCount = len(filteredSource.Candidates)
		filtered.CandidateCount += filteredSource.CandidateCount
		filtered.Sources = append(filtered.Sources, filteredSource)
		filteredTemplate.Sources = append(filteredTemplate.Sources, CorpusAcceptanceAnswerKeySource{
			SourceID:         filteredSource.SourceID,
			SourceDocumentID: fmt.Sprintf("doc-%03d", len(filtered.Sources)),
			ExpectedOutcomes: []SemanticExpectedOutcome{},
		})
		privateMap.Cases = append(privateMap.Cases, mapCase)
	}
	filtered.SourceCount = len(filtered.Sources)
	filteredTemplate.CoverageRequirements.MinSourceCount = filtered.SourceCount
	summary.SelectedCaseCount = len(summary.SelectedCases)
	summary.UnselectedCaseCount = len(candidates) - len(selected)
	for _, selectedCase := range summary.SelectedCases {
		summary.Coverage.SourceGroupCounts[selectedCase.SourceGroupRef]++
		if selectedCase.CandidateKind != "" {
			summary.Coverage.CandidateKindCounts[selectedCase.CandidateKind]++
		}
		if selectedCase.ConfidenceBucket != "" {
			summary.Coverage.ConfidenceCounts[selectedCase.ConfidenceBucket]++
		}
		if selectedCase.ReviewStatus != "" {
			summary.Coverage.ReviewStatusCounts[selectedCase.ReviewStatus]++
		}
		summary.Coverage.SourceStateCounts[selectedCase.SourceState]++
		for _, bucket := range selectedCase.RationaleBuckets {
			summary.Coverage.RationaleCounts[bucket]++
		}
	}
	summary.UnmetCoverage = corpusAcceptanceLabelSeedUnmetCoverage(candidates, selected)
	sort.SliceStable(filteredTemplate.CoverageRequirements.CandidateKinds, func(i, j int) bool {
		return filteredTemplate.CoverageRequirements.CandidateKinds[i] < filteredTemplate.CoverageRequirements.CandidateKinds[j]
	})
	return filtered, filteredTemplate, summary, privateMap
}

func corpusAcceptanceLabelSeedCandidates(packet CorpusAcceptanceLabelingPacket) []corpusAcceptanceLabelSeedCandidate {
	out := []corpusAcceptanceLabelSeedCandidate{}
	for sourceIndex, source := range packet.Sources {
		sourceGroup := corpusAcceptanceLabelSeedSourceGroup(source)
		if len(source.Candidates) == 0 {
			out = append(out, corpusAcceptanceLabelSeedCandidate{
				sourceIndex:      sourceIndex,
				candidateIndex:   -1,
				sourceGroup:      sourceGroup,
				sourceState:      source.SourceState,
				zeroCandidate:    true,
				rationaleBuckets: []string{"zero_candidate_source_review", "source_group", "source_state:" + string(source.SourceState)},
			})
			continue
		}
		for candidateIndex, candidate := range source.Candidates {
			buckets := []string{
				"source_group",
				"kind:" + string(candidate.CandidateKind),
				"confidence:" + string(candidate.Confidence),
				"review:" + string(candidate.ReviewStatus),
			}
			if candidate.ReviewStatus == ReviewStatusNeedsReview || candidate.Confidence == ConfidenceLow {
				buckets = append(buckets, "fallback_or_needs_review")
			}
			out = append(out, corpusAcceptanceLabelSeedCandidate{
				sourceIndex:      sourceIndex,
				candidateIndex:   candidateIndex,
				sourceGroup:      sourceGroup,
				candidateKind:    candidate.CandidateKind,
				confidence:       candidate.Confidence,
				reviewStatus:     candidate.ReviewStatus,
				sourceState:      source.SourceState,
				rationaleBuckets: buckets,
			})
		}
	}
	return out
}

func selectCorpusAcceptanceLabelSeedCandidates(candidates []corpusAcceptanceLabelSeedCandidate, maxCases int) []corpusAcceptanceLabelSeedCandidate {
	if maxCases <= 0 || len(candidates) <= maxCases {
		return append([]corpusAcceptanceLabelSeedCandidate{}, candidates...)
	}
	selected := map[int]bool{}
	var out []corpusAcceptanceLabelSeedCandidate
	addFirstBy := func(key func(corpusAcceptanceLabelSeedCandidate) string) {
		seen := map[string]bool{}
		for idx, candidate := range candidates {
			if len(out) >= maxCases {
				return
			}
			value := key(candidate)
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			if selected[idx] {
				continue
			}
			selected[idx] = true
			out = append(out, candidate)
		}
	}
	addFirstBy(func(candidate corpusAcceptanceLabelSeedCandidate) string { return string(candidate.candidateKind) })
	addFirstBy(func(candidate corpusAcceptanceLabelSeedCandidate) string { return string(candidate.confidence) })
	addFirstBy(func(candidate corpusAcceptanceLabelSeedCandidate) string { return string(candidate.reviewStatus) })
	addFirstBy(func(candidate corpusAcceptanceLabelSeedCandidate) string {
		if candidate.zeroCandidate {
			return "zero_candidate_source_review"
		}
		return ""
	})
	addFirstBy(func(candidate corpusAcceptanceLabelSeedCandidate) string {
		if candidate.reviewStatus == ReviewStatusNeedsReview || candidate.confidence == ConfidenceLow {
			return "fallback_or_needs_review"
		}
		return ""
	})
	addFirstBy(func(candidate corpusAcceptanceLabelSeedCandidate) string { return candidate.sourceGroup })
	for idx, candidate := range candidates {
		if len(out) >= maxCases {
			break
		}
		if selected[idx] {
			continue
		}
		selected[idx] = true
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].sourceIndex != out[j].sourceIndex {
			return out[i].sourceIndex < out[j].sourceIndex
		}
		return out[i].candidateIndex < out[j].candidateIndex
	})
	return out
}

func corpusAcceptanceLabelSeedSourceGroup(source CorpusAcceptanceLabelingSource) string {
	sourceID := strings.TrimSpace(source.SourceID)
	value := strings.TrimSpace(source.SourcePath)
	if value == "" {
		value = strings.TrimSpace(source.SemanticRunDir)
	}
	if value == "" {
		value = strings.TrimSpace(source.SourceID)
	}
	value = filepath.ToSlash(value)
	parts := strings.Split(value, "/")
	if len(parts) > 0 && parts[0] == "sources" && sourceID != "" {
		if idx := strings.Index(sourceID, "-"); idx > 0 {
			return sourceID[:idx]
		}
		return sourceID
	}
	if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" && parts[0] != "." {
		return parts[0]
	}
	return "root"
}

func corpusAcceptanceLabelSeedGroupRefs(candidates []corpusAcceptanceLabelSeedCandidate) map[string]string {
	groups := []string{}
	for _, candidate := range candidates {
		if !corpusAcceptanceLabelSeedStringListContains(groups, candidate.sourceGroup) {
			groups = append(groups, candidate.sourceGroup)
		}
	}
	sort.Strings(groups)
	refs := map[string]string{}
	for idx, group := range groups {
		refs[group] = fmt.Sprintf("group-%03d", idx+1)
	}
	return refs
}

func corpusAcceptanceLabelSeedStringListContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func corpusAcceptanceLabelSeedUnmetCoverage(candidates, selected []corpusAcceptanceLabelSeedCandidate) []string {
	selectedKinds := map[SemanticCandidateKind]bool{}
	availableKinds := map[SemanticCandidateKind]bool{}
	selectedConfidence := map[Confidence]bool{}
	availableConfidence := map[Confidence]bool{}
	selectedReview := map[ReviewStatus]bool{}
	availableReview := map[ReviewStatus]bool{}
	selectedZero := false
	availableZero := false
	for _, candidate := range candidates {
		if candidate.candidateKind != "" {
			availableKinds[candidate.candidateKind] = true
		}
		if candidate.confidence != "" {
			availableConfidence[candidate.confidence] = true
		}
		if candidate.reviewStatus != "" {
			availableReview[candidate.reviewStatus] = true
		}
		if candidate.zeroCandidate {
			availableZero = true
		}
	}
	for _, candidate := range selected {
		if candidate.candidateKind != "" {
			selectedKinds[candidate.candidateKind] = true
		}
		if candidate.confidence != "" {
			selectedConfidence[candidate.confidence] = true
		}
		if candidate.reviewStatus != "" {
			selectedReview[candidate.reviewStatus] = true
		}
		if candidate.zeroCandidate {
			selectedZero = true
		}
	}
	var unmet []string
	if len(availableKinds) > len(selectedKinds) {
		unmet = append(unmet, "candidate_kind_capacity_limited")
	}
	if len(availableConfidence) > len(selectedConfidence) {
		unmet = append(unmet, "confidence_capacity_limited")
	}
	if len(availableReview) > len(selectedReview) {
		unmet = append(unmet, "review_status_capacity_limited")
	}
	if availableZero && !selectedZero {
		unmet = append(unmet, "zero_candidate_source_review_capacity_limited")
	}
	if !availableZero {
		unmet = append(unmet, "zero_candidate_source_review_not_available")
	}
	return unmet
}

func buildCorpusAcceptanceLabelingSource(root string, source CorpusPressureSourceResult, ordinal int) (CorpusAcceptanceLabelingSource, string, error) {
	sourcePath, err := cleanCorpusAcceptanceLabelingArtifactPath(root, source.SourcePath)
	if err != nil {
		return CorpusAcceptanceLabelingSource{}, "", err
	}
	semanticRunDir, err := cleanCorpusAcceptanceLabelingArtifactPath(root, source.SemanticRunDir)
	if err != nil {
		return CorpusAcceptanceLabelingSource{}, "", err
	}
	labelingSource := CorpusAcceptanceLabelingSource{
		CaseID:            fmt.Sprintf("case-%03d", ordinal),
		SourceID:          source.SourceID,
		SourceContentHash: source.SourceContentHash,
		SourcePath:        sourcePath,
		SemanticRunDir:    semanticRunDir,
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

func cleanCorpusAcceptanceLabelingArtifactPath(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" {
		return "", nil
	}
	if _, err := containedCorpusAcceptancePath(root, relative); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative))), nil
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
