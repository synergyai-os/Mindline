package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	SourceMeaningPacketSchemaVersion = "source-meaning-packet/v0.1"
	sourceMeaningPacketMaxGroupAtoms = 12
)

type SourceMeaningPacketSection string

const (
	SourceMeaningPacketSectionReady       SourceMeaningPacketSection = "ready"
	SourceMeaningPacketSectionNeedsReview SourceMeaningPacketSection = "needs_review"
	SourceMeaningPacketSectionBlocked     SourceMeaningPacketSection = "blocked"
)

type SourceMeaningPacketSummary struct {
	SchemaVersion                  string                                  `json:"schema_version"`
	CorpusID                       string                                  `json:"corpus_id"`
	SourceCount                    int                                     `json:"source_count"`
	ProcessedSourceCount           int                                     `json:"processed_source_count"`
	AtomCount                      int                                     `json:"atom_count"`
	RelationCount                  int                                     `json:"relation_count"`
	ReviewGroupCount               int                                     `json:"review_group_count"`
	ReadyGroupCount                int                                     `json:"ready_group_count"`
	NeedsReviewGroupCount          int                                     `json:"needs_review_group_count"`
	BlockedGroupCount              int                                     `json:"blocked_group_count"`
	ProposalCount                  int                                     `json:"proposal_count"`
	EvidenceReferenceCount         int                                     `json:"evidence_reference_count"`
	EvidenceOrBlockerGroupCount    int                                     `json:"evidence_or_blocker_group_count"`
	ReviewBurdenCount              int                                     `json:"review_burden_count"`
	AtomCompressionRatio           float64                                 `json:"atom_compression_ratio"`
	RelationReviewCompressionRatio float64                                 `json:"relation_review_compression_ratio"`
	EvidenceOrBlockerGroupRatio    float64                                 `json:"evidence_or_blocker_group_ratio"`
	ReviewBurdenRatio              float64                                 `json:"review_burden_ratio"`
	NonGeneralizableRuntime        bool                                    `json:"non_generalizable_runtime"`
	Comparable                     bool                                    `json:"comparable"`
	Guardrails                     SourceMeaningPreviewGuardrails          `json:"guardrails"`
	SectionCounts                  map[SourceMeaningPacketSection]int      `json:"section_counts"`
	CandidateKindCounts            map[SemanticCandidateKind]int           `json:"candidate_kind_counts"`
	RoutingHintCounts              map[SourceMeaningPreviewRoutingHint]int `json:"routing_hint_counts"`
	RelationTypeCounts             map[CorpusRelationType]int              `json:"relation_type_counts"`
	CorpusFingerprint              string                                  `json:"corpus_fingerprint"`
	CommandConfigFingerprint       string                                  `json:"command_config_fingerprint"`
	ReplayFingerprint              string                                  `json:"replay_fingerprint"`
	PressureReplayFingerprint      string                                  `json:"pressure_replay_fingerprint"`
	GraphReplayFingerprint         string                                  `json:"graph_replay_fingerprint"`
	ReviewPacketPath               string                                  `json:"review_packet_path"`
	EvidenceMapPath                string                                  `json:"evidence_map_path"`
	BlockedItemsPath               string                                  `json:"blocked_items_path"`
	Groups                         []SourceMeaningPacketGroupSummary       `json:"groups"`
}

type SourceMeaningPacketGroupSummary struct {
	GroupID                string                          `json:"group_id"`
	Title                  string                          `json:"title"`
	Section                SourceMeaningPacketSection      `json:"section"`
	CandidateKind          SemanticCandidateKind           `json:"candidate_kind"`
	RoutingHint            SourceMeaningPreviewRoutingHint `json:"routing_hint"`
	AtomCount              int                             `json:"atom_count"`
	SourceCount            int                             `json:"source_count"`
	EvidenceReferenceCount int                             `json:"evidence_reference_count"`
	RelationCount          int                             `json:"relation_count"`
	DuplicatePressureCount int                             `json:"duplicate_pressure_count"`
	BlockerReasons         []string                        `json:"blocker_reasons,omitempty"`
	GroupPath              string                          `json:"group_path"`
	ProposalPath           string                          `json:"proposal_path,omitempty"`
}

type SourceMeaningPacketGroup struct {
	SchemaVersion          string                           `json:"schema_version"`
	GroupID                string                           `json:"group_id"`
	Title                  string                           `json:"title"`
	Section                SourceMeaningPacketSection       `json:"section"`
	CandidateKind          SemanticCandidateKind            `json:"candidate_kind"`
	RoutingHint            SourceMeaningPreviewRoutingHint  `json:"routing_hint"`
	WriteEligible          bool                             `json:"write_eligible"`
	AtomCount              int                              `json:"atom_count"`
	SourceCount            int                              `json:"source_count"`
	EvidenceReferenceCount int                              `json:"evidence_reference_count"`
	RelationCount          int                              `json:"relation_count"`
	DuplicatePressureCount int                              `json:"duplicate_pressure_count"`
	BlockerReasons         []string                         `json:"blocker_reasons,omitempty"`
	EvidenceRefs           []SourceMeaningPacketEvidenceRef `json:"evidence_refs"`
	AtomRefs               []SourceMeaningPacketAtomRef     `json:"atom_refs"`
	RelatedRelationRefs    []SourceMeaningPacketRelationRef `json:"related_relation_refs,omitempty"`
}

type SourceMeaningPacketAtomRef struct {
	AtomID           string                `json:"atom_id"`
	SourceID         string                `json:"source_id"`
	SourceKind       string                `json:"source_kind"`
	SourceDocumentID string                `json:"source_document_id"`
	CandidateKind    SemanticCandidateKind `json:"candidate_kind"`
	ReviewStatus     ReviewStatus          `json:"review_status"`
	Confidence       Confidence            `json:"confidence"`
	TitleRef         string                `json:"title_ref"`
	SummaryRef       string                `json:"summary_ref"`
}

type SourceMeaningPacketEvidenceRef struct {
	EvidenceRefID    string `json:"evidence_ref_id"`
	AtomID           string `json:"atom_id"`
	SourceID         string `json:"source_id"`
	SourceDocumentID string `json:"source_document_id"`
	LineStart        int    `json:"line_start"`
	LineEnd          int    `json:"line_end"`
	ContentHash      string `json:"content_hash"`
}

type SourceMeaningPacketRelationRef struct {
	RelationID   string             `json:"relation_id"`
	RelationType CorpusRelationType `json:"relation_type"`
	ReviewStatus ReviewStatus       `json:"review_status"`
	FromAtomID   string             `json:"from_atom_id"`
	ToAtomID     string             `json:"to_atom_id"`
}

type SourceMeaningPacketProposal struct {
	SchemaVersion string                          `json:"schema_version"`
	ProposalID    string                          `json:"proposal_id"`
	GroupID       string                          `json:"group_id"`
	Title         string                          `json:"title"`
	CandidateKind SemanticCandidateKind           `json:"candidate_kind"`
	RoutingHint   SourceMeaningPreviewRoutingHint `json:"routing_hint"`
	WriteEligible bool                            `json:"write_eligible"`
	Destination   string                          `json:"destination"`
	EvidenceRefs  []string                        `json:"evidence_refs"`
	ReasonCodes   []string                        `json:"reason_codes"`
}

type SourceMeaningPacketEvidenceMap struct {
	SchemaVersion string                           `json:"schema_version"`
	CorpusID      string                           `json:"corpus_id"`
	EvidenceRefs  []SourceMeaningPacketEvidenceRef `json:"evidence_refs"`
}

type SourceMeaningPacketBlockedItems struct {
	SchemaVersion string                     `json:"schema_version"`
	CorpusID      string                     `json:"corpus_id"`
	Items         []SourceMeaningPacketGroup `json:"items"`
}

type sourceMeaningPacketBuild struct {
	Summary      SourceMeaningPacketSummary
	Groups       []SourceMeaningPacketGroup
	Proposals    []SourceMeaningPacketProposal
	EvidenceMap  SourceMeaningPacketEvidenceMap
	BlockedItems SourceMeaningPacketBlockedItems
}

func BuildSourceMeaningPacket(inputPath, outDir string) (SourceMeaningPacketSummary, []SourceMeaningPacketGroup, error) {
	if strings.TrimSpace(outDir) == "" {
		return SourceMeaningPacketSummary{}, nil, fmt.Errorf("missing required --out")
	}
	root, pressureSummary, err := readSourceMeaningPressureSummary(inputPath)
	if err != nil {
		return SourceMeaningPacketSummary{}, nil, err
	}
	graphSummary, atomsBySource, relationsBySource, err := readSourceMeaningGraph(root, pressureSummary.GraphSummaryPath)
	if err != nil {
		return SourceMeaningPacketSummary{}, nil, err
	}
	build := buildSourceMeaningPacket(pressureSummary, graphSummary, atomsBySource, relationsBySource)
	if err := WriteSourceMeaningPacket(outDir, build.Summary, build.Groups, build.Proposals, build.EvidenceMap, build.BlockedItems); err != nil {
		return SourceMeaningPacketSummary{}, nil, err
	}
	return build.Summary, build.Groups, nil
}

func buildSourceMeaningPacket(pressure CorpusPressureSummary, graph CorpusGraphSummary, atomsBySource map[string][]CorpusGraphAtom, relationsBySource map[string][]CorpusGraphRelation) sourceMeaningPacketBuild {
	atoms := flattenSourceMeaningAtoms(atomsBySource)
	relations := flattenSourceMeaningRelations(relationsBySource)
	relationIndex := sourceMeaningRelationIndex(relations)
	groups := buildSourceMeaningGroups(pressure.CorpusID, atoms, relationIndex)
	proposals := buildSourceMeaningProposals(groups)
	evidenceMap := buildSourceMeaningEvidenceMap(pressure.CorpusID, groups)
	blockedItems := SourceMeaningPacketBlockedItems{SchemaVersion: SourceMeaningPacketSchemaVersion, CorpusID: pressure.CorpusID}
	for _, group := range groups {
		if group.Section == SourceMeaningPacketSectionBlocked {
			blockedItems.Items = append(blockedItems.Items, group)
		}
	}
	summary := buildSourceMeaningPacketSummary(pressure, graph, groups, proposals, evidenceMap)
	return sourceMeaningPacketBuild{Summary: summary, Groups: groups, Proposals: proposals, EvidenceMap: evidenceMap, BlockedItems: blockedItems}
}

func flattenSourceMeaningAtoms(atomsBySource map[string][]CorpusGraphAtom) []CorpusGraphAtom {
	atoms := []CorpusGraphAtom{}
	for _, sourceAtoms := range atomsBySource {
		atoms = append(atoms, sourceAtoms...)
	}
	sort.Slice(atoms, func(i, j int) bool {
		return strings.Join([]string{string(atoms[i].CandidateKind), atoms[i].SourceID, fmt.Sprintf("%08d", atoms[i].LineStart), atoms[i].AtomID}, "\x00") <
			strings.Join([]string{string(atoms[j].CandidateKind), atoms[j].SourceID, fmt.Sprintf("%08d", atoms[j].LineStart), atoms[j].AtomID}, "\x00")
	})
	return atoms
}

func flattenSourceMeaningRelations(relationsBySource map[string][]CorpusGraphRelation) []CorpusGraphRelation {
	seen := map[string]bool{}
	relations := []CorpusGraphRelation{}
	for _, sourceRelations := range relationsBySource {
		for _, relation := range sourceRelations {
			if seen[relation.RelationID] {
				continue
			}
			seen[relation.RelationID] = true
			relations = append(relations, relation)
		}
	}
	sort.Slice(relations, func(i, j int) bool { return relations[i].RelationID < relations[j].RelationID })
	return relations
}

func sourceMeaningRelationIndex(relations []CorpusGraphRelation) map[string][]CorpusGraphRelation {
	index := map[string][]CorpusGraphRelation{}
	for _, relation := range relations {
		index[relation.FromAtomID] = append(index[relation.FromAtomID], relation)
		index[relation.ToAtomID] = append(index[relation.ToAtomID], relation)
	}
	return index
}

func buildSourceMeaningGroups(corpusID string, atoms []CorpusGraphAtom, relationIndex map[string][]CorpusGraphRelation) []SourceMeaningPacketGroup {
	buckets := map[string][]CorpusGraphAtom{}
	for _, atom := range atoms {
		route := packetRouteForAtom(atom)
		key := strings.Join([]string{string(route), string(atom.CandidateKind)}, "\x00")
		buckets[key] = append(buckets[key], atom)
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	groups := []SourceMeaningPacketGroup{}
	for _, key := range keys {
		bucket := buckets[key]
		sort.Slice(bucket, func(i, j int) bool {
			return strings.Join([]string{bucket[i].SourceID, fmt.Sprintf("%08d", bucket[i].LineStart), bucket[i].AtomID}, "\x00") <
				strings.Join([]string{bucket[j].SourceID, fmt.Sprintf("%08d", bucket[j].LineStart), bucket[j].AtomID}, "\x00")
		})
		for start := 0; start < len(bucket); start += sourceMeaningPacketMaxGroupAtoms {
			end := start + sourceMeaningPacketMaxGroupAtoms
			if end > len(bucket) {
				end = len(bucket)
			}
			groups = append(groups, buildSourceMeaningGroup(corpusID, len(groups)+1, bucket[start:end], relationIndex))
		}
	}
	return groups
}

func buildSourceMeaningGroup(corpusID string, ordinal int, atoms []CorpusGraphAtom, relationIndex map[string][]CorpusGraphRelation) SourceMeaningPacketGroup {
	route := packetRouteForAtom(atoms[0])
	kind := atoms[0].CandidateKind
	groupID := sourceMeaningPacketGroupID(corpusID, ordinal, route, kind, atoms)
	group := SourceMeaningPacketGroup{
		SchemaVersion: SourceMeaningPacketSchemaVersion,
		GroupID:       groupID,
		Title:         sourceMeaningPacketGroupTitle(kind, route, ordinal),
		Section:       SourceMeaningPacketSectionReady,
		CandidateKind: kind,
		RoutingHint:   route,
		WriteEligible: false,
	}
	sourceIDs := map[string]bool{}
	relationSeen := map[string]bool{}
	for _, atom := range atoms {
		sourceIDs[atom.SourceID] = true
		group.AtomRefs = append(group.AtomRefs, sourceMeaningAtomRef(atom))
		evidence := sourceMeaningEvidenceRef(atom)
		if evidence.ContentHash == "" || evidence.LineStart <= 0 || evidence.LineEnd <= 0 {
			group.BlockerReasons = appendUniqueString(group.BlockerReasons, "missing_evidence_reference")
		} else {
			group.EvidenceRefs = append(group.EvidenceRefs, evidence)
		}
		if atom.ReviewStatus == ReviewStatusBlocked {
			group.BlockerReasons = appendUniqueString(group.BlockerReasons, "blocked_atom")
		}
		for _, blocker := range atom.Blockers {
			group.BlockerReasons = appendUniqueString(group.BlockerReasons, blocker.Code)
		}
		for _, relation := range relationIndex[atom.AtomID] {
			if relationSeen[relation.RelationID] {
				continue
			}
			relationSeen[relation.RelationID] = true
			ref := SourceMeaningPacketRelationRef{
				RelationID:   relation.RelationID,
				RelationType: relation.RelationType,
				ReviewStatus: relation.ReviewStatus,
				FromAtomID:   relation.FromAtomID,
				ToAtomID:     relation.ToAtomID,
			}
			group.RelatedRelationRefs = append(group.RelatedRelationRefs, ref)
			if relation.RelationType == CorpusRelationPossibleDuplicate {
				group.DuplicatePressureCount++
			}
			if relation.ReviewStatus == ReviewStatusBlocked {
				group.BlockerReasons = appendUniqueString(group.BlockerReasons, "blocked_relation")
			}
		}
	}
	group.AtomCount = len(group.AtomRefs)
	group.SourceCount = len(sourceIDs)
	group.EvidenceReferenceCount = len(group.EvidenceRefs)
	group.RelationCount = len(group.RelatedRelationRefs)
	sort.Slice(group.EvidenceRefs, func(i, j int) bool { return group.EvidenceRefs[i].EvidenceRefID < group.EvidenceRefs[j].EvidenceRefID })
	sort.Slice(group.RelatedRelationRefs, func(i, j int) bool {
		return group.RelatedRelationRefs[i].RelationID < group.RelatedRelationRefs[j].RelationID
	})
	if len(group.BlockerReasons) > 0 {
		group.Section = SourceMeaningPacketSectionBlocked
	}
	return group
}

func packetRouteForAtom(atom CorpusGraphAtom) SourceMeaningPreviewRoutingHint {
	switch atom.CandidateKind {
	case SemanticCandidateKindDecision, SemanticCandidateKindIssue, SemanticCandidateKindRequirement, SemanticCandidateKindDependency, SemanticCandidateKindRisk, SemanticCandidateKindCapability:
		return SourceMeaningRoutingProductBrainCandidate
	case SemanticCandidateKindAction:
		return SourceMeaningRoutingBothCandidate
	case SemanticCandidateKindTopic, SemanticCandidateKindQuestion, SemanticCandidateKindReference:
		return SourceMeaningRoutingTolariaCandidate
	default:
		return SourceMeaningRoutingNoOp
	}
}

func sourceMeaningPacketGroupID(corpusID string, ordinal int, route SourceMeaningPreviewRoutingHint, kind SemanticCandidateKind, atoms []CorpusGraphAtom) string {
	parts := []string{corpusID, fmt.Sprintf("%04d", ordinal), string(route), string(kind)}
	for _, atom := range atoms {
		parts = append(parts, atom.AtomID)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "meaning-group-" + hex.EncodeToString(sum[:])[:16]
}

func sourceMeaningPacketGroupTitle(kind SemanticCandidateKind, route SourceMeaningPreviewRoutingHint, ordinal int) string {
	return fmt.Sprintf("%s %s group %02d", strings.ReplaceAll(string(route), "_", " "), strings.ReplaceAll(string(kind), "_", " "), ordinal)
}

func sourceMeaningAtomRef(atom CorpusGraphAtom) SourceMeaningPacketAtomRef {
	return SourceMeaningPacketAtomRef{
		AtomID:           atom.AtomID,
		SourceID:         atom.SourceID,
		SourceKind:       atom.SourceKind,
		SourceDocumentID: atom.SourceDocumentID,
		CandidateKind:    atom.CandidateKind,
		ReviewStatus:     atom.ReviewStatus,
		Confidence:       atom.Confidence,
		TitleRef:         shortContentRef(atom.Title),
		SummaryRef:       shortContentRef(atom.Summary),
	}
}

func sourceMeaningEvidenceRef(atom CorpusGraphAtom) SourceMeaningPacketEvidenceRef {
	refSeed := strings.Join([]string{atom.AtomID, atom.SourceID, atom.SourceDocumentID, fmt.Sprintf("%d", atom.LineStart), fmt.Sprintf("%d", atom.LineEnd), atom.ContentHash}, "\x00")
	sum := sha256.Sum256([]byte(refSeed))
	return SourceMeaningPacketEvidenceRef{
		EvidenceRefID:    "evref-" + hex.EncodeToString(sum[:])[:16],
		AtomID:           atom.AtomID,
		SourceID:         atom.SourceID,
		SourceDocumentID: atom.SourceDocumentID,
		LineStart:        atom.LineStart,
		LineEnd:          atom.LineEnd,
		ContentHash:      atom.ContentHash,
	}
}

func shortContentRef(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

func buildSourceMeaningProposals(groups []SourceMeaningPacketGroup) []SourceMeaningPacketProposal {
	proposals := []SourceMeaningPacketProposal{}
	for _, group := range groups {
		if group.Section == SourceMeaningPacketSectionBlocked {
			continue
		}
		evidenceRefs := make([]string, 0, len(group.EvidenceRefs))
		for _, ref := range group.EvidenceRefs {
			evidenceRefs = append(evidenceRefs, ref.EvidenceRefID)
		}
		proposal := SourceMeaningPacketProposal{
			SchemaVersion: SourceMeaningPacketSchemaVersion,
			ProposalID:    strings.Replace(group.GroupID, "meaning-group-", "meaning-proposal-", 1),
			GroupID:       group.GroupID,
			Title:         group.Title,
			CandidateKind: group.CandidateKind,
			RoutingHint:   group.RoutingHint,
			WriteEligible: false,
			Destination:   "destination_neutral",
			EvidenceRefs:  evidenceRefs,
			ReasonCodes:   []string{"review_packet_only", "destination_write_blocked"},
		}
		proposals = append(proposals, proposal)
	}
	return proposals
}

func buildSourceMeaningEvidenceMap(corpusID string, groups []SourceMeaningPacketGroup) SourceMeaningPacketEvidenceMap {
	seen := map[string]bool{}
	refs := []SourceMeaningPacketEvidenceRef{}
	for _, group := range groups {
		for _, ref := range group.EvidenceRefs {
			if seen[ref.EvidenceRefID] {
				continue
			}
			seen[ref.EvidenceRefID] = true
			refs = append(refs, ref)
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].EvidenceRefID < refs[j].EvidenceRefID })
	return SourceMeaningPacketEvidenceMap{SchemaVersion: SourceMeaningPacketSchemaVersion, CorpusID: corpusID, EvidenceRefs: refs}
}

func buildSourceMeaningPacketSummary(pressure CorpusPressureSummary, graph CorpusGraphSummary, groups []SourceMeaningPacketGroup, proposals []SourceMeaningPacketProposal, evidenceMap SourceMeaningPacketEvidenceMap) SourceMeaningPacketSummary {
	summary := SourceMeaningPacketSummary{
		SchemaVersion:             SourceMeaningPacketSchemaVersion,
		CorpusID:                  pressure.CorpusID,
		SourceCount:               pressure.SourceCount,
		ProcessedSourceCount:      pressure.ProcessedSourceCount,
		AtomCount:                 graph.AtomCount,
		RelationCount:             graph.RelationCount,
		ReviewGroupCount:          len(groups),
		ProposalCount:             len(proposals),
		EvidenceReferenceCount:    len(evidenceMap.EvidenceRefs),
		NonGeneralizableRuntime:   true,
		Comparable:                true,
		CorpusFingerprint:         pressure.CorpusFingerprint,
		CommandConfigFingerprint:  pressure.CommandConfigFingerprint,
		PressureReplayFingerprint: pressure.ReplayFingerprint,
		GraphReplayFingerprint:    graph.ReplayFingerprint,
		ReviewPacketPath:          filepath.ToSlash(filepath.Join(SourceMeaningPacketDirName, "review-packet.md")),
		EvidenceMapPath:           filepath.ToSlash(filepath.Join(SourceMeaningPacketDirName, "evidence-map.json")),
		BlockedItemsPath:          filepath.ToSlash(filepath.Join(SourceMeaningPacketDirName, "blocked-items.json")),
		SectionCounts:             map[SourceMeaningPacketSection]int{},
		CandidateKindCounts:       map[SemanticCandidateKind]int{},
		RoutingHintCounts:         map[SourceMeaningPreviewRoutingHint]int{},
		RelationTypeCounts:        cloneSourceMeaningRelationTypeCounts(graph.RelationTypeCounts),
		Guardrails: SourceMeaningPreviewGuardrails{
			HostedInferenceCalls:   0,
			HostedTelemetryExports: 0,
			DestinationWrites:      0,
			ProductBrainWrites:     0,
			TolariaWrites:          0,
		},
	}
	for _, group := range groups {
		summary.SectionCounts[group.Section]++
		summary.CandidateKindCounts[group.CandidateKind] += group.AtomCount
		summary.RoutingHintCounts[group.RoutingHint]++
		if group.Section == SourceMeaningPacketSectionReady {
			summary.ReadyGroupCount++
		}
		if group.Section == SourceMeaningPacketSectionNeedsReview {
			summary.NeedsReviewGroupCount++
			summary.ReviewBurdenCount++
		}
		if group.Section == SourceMeaningPacketSectionBlocked {
			summary.BlockedGroupCount++
			summary.ReviewBurdenCount++
		}
		if group.EvidenceReferenceCount > 0 || len(group.BlockerReasons) > 0 {
			summary.EvidenceOrBlockerGroupCount++
		}
		groupPath := SourceMeaningPacketGroupPath(group.GroupID)
		proposalPath := ""
		if group.Section != SourceMeaningPacketSectionBlocked {
			proposalPath = SourceMeaningPacketProposalPath(strings.Replace(group.GroupID, "meaning-group-", "meaning-proposal-", 1))
		}
		summary.Groups = append(summary.Groups, SourceMeaningPacketGroupSummary{
			GroupID:                group.GroupID,
			Title:                  group.Title,
			Section:                group.Section,
			CandidateKind:          group.CandidateKind,
			RoutingHint:            group.RoutingHint,
			AtomCount:              group.AtomCount,
			SourceCount:            group.SourceCount,
			EvidenceReferenceCount: group.EvidenceReferenceCount,
			RelationCount:          group.RelationCount,
			DuplicatePressureCount: group.DuplicatePressureCount,
			BlockerReasons:         append([]string{}, group.BlockerReasons...),
			GroupPath:              filepath.ToSlash(filepath.Join(SourceMeaningPacketDirName, groupPath)),
			ProposalPath:           filepath.ToSlash(filepath.Join(SourceMeaningPacketDirName, proposalPath)),
		})
	}
	if graph.AtomCount > 0 {
		summary.AtomCompressionRatio = 1 - float64(summary.ReviewGroupCount)/float64(graph.AtomCount)
	}
	if graph.RelationCount > 0 {
		summary.RelationReviewCompressionRatio = 1 - float64(summary.ReviewGroupCount)/float64(graph.RelationCount)
	}
	if summary.ReviewGroupCount > 0 {
		summary.EvidenceOrBlockerGroupRatio = float64(summary.EvidenceOrBlockerGroupCount) / float64(summary.ReviewGroupCount)
		summary.ReviewBurdenRatio = float64(summary.ReviewBurdenCount) / float64(summary.ReviewGroupCount)
	}
	summary.ReplayFingerprint = sourceMeaningPacketReplayFingerprint(summary)
	return summary
}

func sourceMeaningPacketReplayFingerprint(summary SourceMeaningPacketSummary) string {
	parts := []string{
		summary.CorpusID,
		summary.CorpusFingerprint,
		summary.CommandConfigFingerprint,
		fmt.Sprintf("%d:%d:%d", summary.AtomCount, summary.RelationCount, summary.ReviewGroupCount),
	}
	for _, group := range summary.Groups {
		parts = append(parts, strings.Join([]string{group.GroupID, string(group.Section), string(group.CandidateKind), string(group.RoutingHint), fmt.Sprintf("%d", group.AtomCount)}, ":"))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "source-meaning-packet-" + hex.EncodeToString(sum[:])[:16]
}

func appendUniqueString(values []string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
