package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	CorpusConceptsSchemaVersion = "corpus-concepts/v0.1"
	CorpusConceptsDirName       = "corpus-concepts"
	DefaultCorpusConceptsMax    = 40
	corpusConceptMaxAtoms       = 18
)

type CorpusConceptSection string

const (
	CorpusConceptSectionCrossSource CorpusConceptSection = "cross_source"
	CorpusConceptSectionLocal       CorpusConceptSection = "local"
	CorpusConceptSectionNeedsReview CorpusConceptSection = "needs_review"
	CorpusConceptSectionBlocked     CorpusConceptSection = "blocked"
)

type CorpusConceptSummary struct {
	SchemaVersion                     string                                  `json:"schema_version"`
	CorpusID                          string                                  `json:"corpus_id"`
	SourceCount                       int                                     `json:"source_count"`
	ProcessedSourceCount              int                                     `json:"processed_source_count"`
	AtomCount                         int                                     `json:"atom_count"`
	RelationCount                     int                                     `json:"relation_count"`
	ConceptCount                      int                                     `json:"concept_count"`
	GeneratedConceptCount             int                                     `json:"generated_concept_count,omitempty"`
	CrossSourceConceptCount           int                                     `json:"cross_source_concept_count"`
	LocalConceptCount                 int                                     `json:"local_concept_count"`
	NeedsReviewConceptCount           int                                     `json:"needs_review_concept_count"`
	BlockedConceptCount               int                                     `json:"blocked_concept_count"`
	EvidenceReferenceCount            int                                     `json:"evidence_reference_count"`
	CrossSourceEvidenceReferenceCount int                                     `json:"cross_source_evidence_reference_count"`
	ConceptReviewBurdenCount          int                                     `json:"concept_review_burden_count"`
	ConceptReviewBurdenRatio          float64                                 `json:"concept_review_burden_ratio"`
	RelationReviewCompressionRatio    float64                                 `json:"relation_review_compression_ratio"`
	AtomCoverageRatio                 float64                                 `json:"atom_coverage_ratio"`
	CrossSourceAtomRatio              float64                                 `json:"cross_source_atom_ratio"`
	SourceKindCoverage                map[string]int                          `json:"source_kind_coverage"`
	CrossSourceKindPairCount          int                                     `json:"cross_source_kind_pair_count"`
	MaxConceptCount                   int                                     `json:"max_concept_count"`
	OmittedConceptCount               int                                     `json:"omitted_concept_count,omitempty"`
	OmittedAtomCount                  int                                     `json:"omitted_atom_count,omitempty"`
	ScaleStatus                       string                                  `json:"scale_status"`
	ScaleReasonCodes                  []string                                `json:"scale_reason_codes,omitempty"`
	NonGeneralizableRuntime           bool                                    `json:"non_generalizable_runtime"`
	Comparable                        bool                                    `json:"comparable"`
	Guardrails                        CorpusPressureGuardrailCounters         `json:"guardrails"`
	CorpusFingerprint                 string                                  `json:"corpus_fingerprint"`
	CommandConfigFingerprint          string                                  `json:"command_config_fingerprint"`
	PressureReplayFingerprint         string                                  `json:"pressure_replay_fingerprint"`
	GraphReplayFingerprint            string                                  `json:"graph_replay_fingerprint"`
	ReplayFingerprint                 string                                  `json:"replay_fingerprint"`
	ConceptIndexPath                  string                                  `json:"concept_index_path"`
	ReviewPacketPath                  string                                  `json:"review_packet_path"`
	SectionCounts                     map[CorpusConceptSection]int            `json:"section_counts"`
	CandidateKindCounts               map[SemanticCandidateKind]int           `json:"candidate_kind_counts"`
	RoutingHintCounts                 map[SourceMeaningPreviewRoutingHint]int `json:"routing_hint_counts"`
	Concepts                          []CorpusConceptListItem                 `json:"concepts"`
}

type CorpusConceptIndex struct {
	SchemaVersion string          `json:"schema_version"`
	CorpusID      string          `json:"corpus_id"`
	Concepts      []CorpusConcept `json:"concepts"`
}

type CorpusConceptListItem struct {
	ConceptID              string                          `json:"concept_id"`
	Title                  string                          `json:"title"`
	ConceptKey             string                          `json:"concept_key"`
	Section                CorpusConceptSection            `json:"section"`
	CandidateKind          SemanticCandidateKind           `json:"candidate_kind"`
	RoutingHint            SourceMeaningPreviewRoutingHint `json:"routing_hint"`
	AtomCount              int                             `json:"atom_count"`
	SourceCount            int                             `json:"source_count"`
	EvidenceReferenceCount int                             `json:"evidence_reference_count"`
	SourceKindCoverage     map[string]int                  `json:"source_kind_coverage"`
	ReviewStatus           ReviewStatus                    `json:"review_status"`
	ReasonCodes            []string                        `json:"reason_codes,omitempty"`
	ConceptPath            string                          `json:"concept_path"`
}

type CorpusConcept struct {
	SchemaVersion          string                           `json:"schema_version"`
	ConceptID              string                           `json:"concept_id"`
	CorpusID               string                           `json:"corpus_id"`
	Title                  string                           `json:"title"`
	ConceptKey             string                           `json:"concept_key"`
	Section                CorpusConceptSection             `json:"section"`
	CandidateKind          SemanticCandidateKind            `json:"candidate_kind"`
	RoutingHint            SourceMeaningPreviewRoutingHint  `json:"routing_hint"`
	WriteEligible          bool                             `json:"write_eligible"`
	ReviewStatus           ReviewStatus                     `json:"review_status"`
	AtomCount              int                              `json:"atom_count"`
	SourceCount            int                              `json:"source_count"`
	EvidenceReferenceCount int                              `json:"evidence_reference_count"`
	SourceKindCoverage     map[string]int                   `json:"source_kind_coverage"`
	ReasonCodes            []string                         `json:"reason_codes,omitempty"`
	EvidenceRefs           []SourceMeaningPacketEvidenceRef `json:"evidence_refs"`
	AtomRefs               []SourceMeaningPacketAtomRef     `json:"atom_refs"`
}

type corpusConceptBuild struct {
	Summary CorpusConceptSummary
	Index   CorpusConceptIndex
}

func BuildCorpusConceptIndex(inputPath, outDir string) (CorpusConceptSummary, CorpusConceptIndex, error) {
	if strings.TrimSpace(outDir) == "" {
		return CorpusConceptSummary{}, CorpusConceptIndex{}, fmt.Errorf("missing required --out")
	}
	root, pressureSummary, err := readSourceMeaningPressureSummary(inputPath)
	if err != nil {
		return CorpusConceptSummary{}, CorpusConceptIndex{}, err
	}
	graphSummary, atomsBySource, relationsBySource, err := readSourceMeaningGraph(root, pressureSummary.GraphSummaryPath)
	if err != nil {
		return CorpusConceptSummary{}, CorpusConceptIndex{}, err
	}
	build := buildCorpusConceptIndex(pressureSummary, graphSummary, flattenSourceMeaningAtoms(atomsBySource), flattenSourceMeaningRelations(relationsBySource), DefaultCorpusConceptsMax)
	if err := WriteCorpusConceptIndex(outDir, build.Summary, build.Index); err != nil {
		return CorpusConceptSummary{}, CorpusConceptIndex{}, err
	}
	return build.Summary, build.Index, nil
}

func ReadCorpusConceptSummary(inputPath string) (CorpusConceptSummary, error) {
	root, err := corpusConceptRoot(inputPath)
	if err != nil {
		return CorpusConceptSummary{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "concept-summary.json"))
	if err != nil {
		return CorpusConceptSummary{}, fmt.Errorf("read corpus concept summary: %w", err)
	}
	var summary CorpusConceptSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return CorpusConceptSummary{}, fmt.Errorf("decode corpus concept summary: %w", err)
	}
	if summary.SchemaVersion != CorpusConceptsSchemaVersion {
		return CorpusConceptSummary{}, fmt.Errorf("unsupported corpus concept summary schema version: %s", summary.SchemaVersion)
	}
	return summary, nil
}

func ReadCorpusConceptIndex(inputPath string) (CorpusConceptIndex, error) {
	root, err := corpusConceptRoot(inputPath)
	if err != nil {
		return CorpusConceptIndex{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "concept-index.json"))
	if err != nil {
		return CorpusConceptIndex{}, fmt.Errorf("read corpus concept index: %w", err)
	}
	var index CorpusConceptIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return CorpusConceptIndex{}, fmt.Errorf("decode corpus concept index: %w", err)
	}
	if index.SchemaVersion != CorpusConceptsSchemaVersion {
		return CorpusConceptIndex{}, fmt.Errorf("unsupported corpus concept index schema version: %s", index.SchemaVersion)
	}
	return index, nil
}

func corpusConceptRoot(inputPath string) (string, error) {
	if strings.TrimSpace(inputPath) == "" {
		return "", fmt.Errorf("missing corpus concept path")
	}
	root, err := filepath.Abs(inputPath)
	if err != nil {
		return "", err
	}
	if filepath.Base(root) != CorpusConceptsDirName {
		root = filepath.Join(root, CorpusConceptsDirName)
	}
	return root, nil
}

func buildCorpusConceptIndex(pressure CorpusPressureSummary, graph CorpusGraphSummary, atoms []CorpusGraphAtom, relations []CorpusGraphRelation, maxConcepts int) corpusConceptBuild {
	if maxConcepts <= 0 {
		maxConcepts = DefaultCorpusConceptsMax
	}
	generated := buildCorpusConcepts(pressure.CorpusID, atoms, relations)
	concepts := append([]CorpusConcept{}, generated...)
	omittedConceptCount := 0
	omittedAtomCount := 0
	if len(concepts) > maxConcepts {
		omittedConceptCount = len(concepts) - maxConcepts
		for _, concept := range concepts[maxConcepts:] {
			omittedAtomCount += concept.AtomCount
		}
		concepts = append([]CorpusConcept{}, concepts[:maxConcepts]...)
	}
	index := CorpusConceptIndex{
		SchemaVersion: CorpusConceptsSchemaVersion,
		CorpusID:      pressure.CorpusID,
		Concepts:      concepts,
	}
	summary := buildCorpusConceptSummary(pressure, graph, concepts, len(generated), maxConcepts, omittedConceptCount, omittedAtomCount)
	return corpusConceptBuild{Summary: summary, Index: index}
}

func buildCorpusConcepts(corpusID string, atoms []CorpusGraphAtom, relations []CorpusGraphRelation) []CorpusConcept {
	assigned := map[string]bool{}
	concepts := buildCorpusRelationConcepts(corpusID, atoms, relations, assigned)
	termBuckets := corpusConceptTermBuckets(atoms)
	for _, bucket := range termBuckets {
		selected := []CorpusGraphAtom{}
		for _, atom := range bucket.Atoms {
			if assigned[atom.AtomID] {
				continue
			}
			selected = append(selected, atom)
			if len(selected) >= corpusConceptMaxAtoms {
				break
			}
		}
		if len(selected) < 2 {
			continue
		}
		for _, atom := range selected {
			assigned[atom.AtomID] = true
		}
		concepts = append(concepts, buildCorpusConcept(corpusID, bucket.Key, selected))
	}
	localBuckets := map[string][]CorpusGraphAtom{}
	for _, atom := range atoms {
		if assigned[atom.AtomID] {
			continue
		}
		key := strings.Join([]string{"local", string(atom.CandidateKind), sourceKindForConcept(atom)}, "\x00")
		localBuckets[key] = append(localBuckets[key], atom)
	}
	keys := make([]string, 0, len(localBuckets))
	for key := range localBuckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		bucket := localBuckets[key]
		sortCorpusConceptAtoms(bucket)
		for start := 0; start < len(bucket); start += corpusConceptMaxAtoms {
			end := start + corpusConceptMaxAtoms
			if end > len(bucket) {
				end = len(bucket)
			}
			concepts = append(concepts, buildCorpusConcept(corpusID, key, bucket[start:end]))
		}
	}
	sort.Slice(concepts, func(i, j int) bool {
		return corpusConceptSortKey(concepts[i]) < corpusConceptSortKey(concepts[j])
	})
	return concepts
}

func buildCorpusRelationConcepts(corpusID string, atoms []CorpusGraphAtom, relations []CorpusGraphRelation, assigned map[string]bool) []CorpusConcept {
	atomsByID := map[string]CorpusGraphAtom{}
	for _, atom := range atoms {
		atomsByID[atom.AtomID] = atom
	}
	adjacency := map[string]map[string]bool{}
	for _, relation := range relations {
		if relation.ReviewStatus == ReviewStatusBlocked {
			continue
		}
		if relation.RelationType != CorpusRelationSameTopicAs && relation.RelationType != CorpusRelationPossibleDuplicate {
			continue
		}
		from, fromOK := atomsByID[relation.FromAtomID]
		to, toOK := atomsByID[relation.ToAtomID]
		if !fromOK || !toOK || sourceKindForConcept(from) == sourceKindForConcept(to) {
			continue
		}
		if adjacency[from.AtomID] == nil {
			adjacency[from.AtomID] = map[string]bool{}
		}
		if adjacency[to.AtomID] == nil {
			adjacency[to.AtomID] = map[string]bool{}
		}
		adjacency[from.AtomID][to.AtomID] = true
		adjacency[to.AtomID][from.AtomID] = true
	}
	visited := map[string]bool{}
	components := [][]CorpusGraphAtom{}
	for atomID := range adjacency {
		if visited[atomID] {
			continue
		}
		stack := []string{atomID}
		visited[atomID] = true
		component := []CorpusGraphAtom{}
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			component = append(component, atomsByID[current])
			for next := range adjacency[current] {
				if visited[next] {
					continue
				}
				visited[next] = true
				stack = append(stack, next)
			}
		}
		if corpusConceptSourceKindCount(component) > 1 {
			components = append(components, component)
		}
	}
	sort.Slice(components, func(i, j int) bool {
		if len(components[i]) != len(components[j]) {
			return len(components[i]) > len(components[j])
		}
		sortCorpusConceptAtoms(components[i])
		sortCorpusConceptAtoms(components[j])
		return components[i][0].AtomID < components[j][0].AtomID
	})
	concepts := []CorpusConcept{}
	for componentIndex, component := range components {
		for chunkIndex, chunk := range corpusConceptMixedChunks(component, corpusConceptMaxAtoms) {
			if len(chunk) < 2 || corpusConceptSourceKindCount(chunk) < 2 {
				continue
			}
			for _, atom := range chunk {
				assigned[atom.AtomID] = true
			}
			key := fmt.Sprintf("relation\x00cross_source\x00%04d\x00%04d", componentIndex+1, chunkIndex+1)
			concepts = append(concepts, buildCorpusConcept(corpusID, key, chunk))
		}
	}
	return concepts
}

func corpusConceptMixedChunks(atoms []CorpusGraphAtom, chunkSize int) [][]CorpusGraphAtom {
	byKind := map[string][]CorpusGraphAtom{}
	kinds := []string{}
	for _, atom := range atoms {
		kind := sourceKindForConcept(atom)
		if _, ok := byKind[kind]; !ok {
			kinds = append(kinds, kind)
		}
		byKind[kind] = append(byKind[kind], atom)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		sortCorpusConceptAtoms(byKind[kind])
	}
	ordered := []CorpusGraphAtom{}
	for {
		added := false
		for _, kind := range kinds {
			if len(byKind[kind]) == 0 {
				continue
			}
			ordered = append(ordered, byKind[kind][0])
			byKind[kind] = byKind[kind][1:]
			added = true
		}
		if !added {
			break
		}
	}
	chunks := [][]CorpusGraphAtom{}
	for start := 0; start < len(ordered); start += chunkSize {
		end := start + chunkSize
		if end > len(ordered) {
			end = len(ordered)
		}
		chunks = append(chunks, ordered[start:end])
	}
	return chunks
}

func corpusConceptSourceKindCount(atoms []CorpusGraphAtom) int {
	kinds := map[string]bool{}
	for _, atom := range atoms {
		kinds[sourceKindForConcept(atom)] = true
	}
	return len(kinds)
}

type corpusConceptTermBucket struct {
	Key         string
	Atoms       []CorpusGraphAtom
	SourceCount int
	KindCount   int
}

func corpusConceptTermBuckets(atoms []CorpusGraphAtom) []corpusConceptTermBucket {
	buckets := map[string][]CorpusGraphAtom{}
	for _, atom := range atoms {
		for _, term := range corpusConceptTerms(atom) {
			key := strings.Join([]string{"term", string(atom.CandidateKind), term}, "\x00")
			buckets[key] = append(buckets[key], atom)
		}
	}
	out := []corpusConceptTermBucket{}
	for key, bucketAtoms := range buckets {
		if len(bucketAtoms) < 2 {
			continue
		}
		sortCorpusConceptAtoms(bucketAtoms)
		sourceSet := map[string]bool{}
		kindSet := map[string]bool{}
		deduped := []CorpusGraphAtom{}
		seenAtoms := map[string]bool{}
		for _, atom := range bucketAtoms {
			if seenAtoms[atom.AtomID] {
				continue
			}
			seenAtoms[atom.AtomID] = true
			sourceSet[atom.SourceID] = true
			kindSet[sourceKindForConcept(atom)] = true
			deduped = append(deduped, atom)
		}
		if len(deduped) < 2 {
			continue
		}
		out = append(out, corpusConceptTermBucket{Key: key, Atoms: deduped, SourceCount: len(sourceSet), KindCount: len(kindSet)})
	}
	sort.Slice(out, func(i, j int) bool {
		left := fmt.Sprintf("%03d:%03d:%03d:%s", out[i].KindCount, out[i].SourceCount, len(out[i].Atoms), out[i].Key)
		right := fmt.Sprintf("%03d:%03d:%03d:%s", out[j].KindCount, out[j].SourceCount, len(out[j].Atoms), out[j].Key)
		return left > right
	})
	return out
}

func buildCorpusConcept(corpusID, key string, atoms []CorpusGraphAtom) CorpusConcept {
	sortCorpusConceptAtoms(atoms)
	kind := atoms[0].CandidateKind
	route := packetRouteForAtom(atoms[0])
	concept := CorpusConcept{
		SchemaVersion:      CorpusConceptsSchemaVersion,
		CorpusID:           corpusID,
		ConceptKey:         key,
		CandidateKind:      kind,
		RoutingHint:        route,
		WriteEligible:      false,
		ReviewStatus:       ReviewStatusReady,
		SourceKindCoverage: map[string]int{},
	}
	sourceIDs := map[string]bool{}
	sourceKindSourceIDs := map[string]map[string]bool{}
	for _, atom := range atoms {
		sourceIDs[atom.SourceID] = true
		sourceKind := sourceKindForConcept(atom)
		if sourceKindSourceIDs[sourceKind] == nil {
			sourceKindSourceIDs[sourceKind] = map[string]bool{}
		}
		sourceKindSourceIDs[sourceKind][atom.SourceID] = true
		concept.AtomRefs = append(concept.AtomRefs, sourceMeaningAtomRef(atom))
		evidence := sourceMeaningEvidenceRef(atom)
		if evidence.ContentHash == "" || evidence.LineStart <= 0 || evidence.LineEnd <= 0 {
			concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "missing_evidence_reference")
		} else {
			concept.EvidenceRefs = append(concept.EvidenceRefs, evidence)
		}
		if atom.ReviewStatus == ReviewStatusBlocked {
			concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "blocked_atom")
		}
		if atom.ReviewStatus == ReviewStatusNeedsReview {
			concept.ReviewStatus = ReviewStatusNeedsReview
		}
		for _, blocker := range atom.Blockers {
			concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, blocker.Code)
		}
	}
	concept.AtomCount = len(concept.AtomRefs)
	concept.SourceCount = len(sourceIDs)
	concept.EvidenceReferenceCount = len(concept.EvidenceRefs)
	for sourceKind, sourceKindIDs := range sourceKindSourceIDs {
		concept.SourceKindCoverage[sourceKind] = len(sourceKindIDs)
	}
	if concept.SourceCount < 2 {
		concept.Section = CorpusConceptSectionLocal
		concept.ReviewStatus = ReviewStatusNeedsReview
		concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "single_source_concept")
	} else if len(concept.SourceKindCoverage) < 2 {
		concept.Section = CorpusConceptSectionNeedsReview
		concept.ReviewStatus = ReviewStatusNeedsReview
		concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "single_source_kind_concept")
	} else {
		concept.Section = CorpusConceptSectionCrossSource
	}
	if strings.HasPrefix(key, "relation\x00cross_source\x00") {
		concept.ReviewStatus = ReviewStatusNeedsReview
		concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "relation_neighborhood_requires_review")
	}
	if len(concept.ReasonCodes) > 0 {
		if containsCorpusConceptString(concept.ReasonCodes, "blocked_atom") || containsCorpusConceptString(concept.ReasonCodes, "missing_evidence_reference") {
			concept.Section = CorpusConceptSectionBlocked
			concept.ReviewStatus = ReviewStatusBlocked
		}
	}
	concept.Title = corpusConceptTitle(key, atoms)
	concept.ConceptID = corpusConceptID(corpusID, key, atoms)
	sort.Slice(concept.EvidenceRefs, func(i, j int) bool {
		return concept.EvidenceRefs[i].EvidenceRefID < concept.EvidenceRefs[j].EvidenceRefID
	})
	return concept
}

func buildCorpusConceptSummary(pressure CorpusPressureSummary, graph CorpusGraphSummary, concepts []CorpusConcept, generatedConceptCount, maxConcepts, omittedConceptCount, omittedAtomCount int) CorpusConceptSummary {
	summary := CorpusConceptSummary{
		SchemaVersion:             CorpusConceptsSchemaVersion,
		CorpusID:                  pressure.CorpusID,
		SourceCount:               pressure.SourceCount,
		ProcessedSourceCount:      pressure.ProcessedSourceCount,
		AtomCount:                 graph.AtomCount,
		RelationCount:             graph.RelationCount,
		ConceptCount:              len(concepts),
		GeneratedConceptCount:     generatedConceptCount,
		MaxConceptCount:           maxConcepts,
		OmittedConceptCount:       omittedConceptCount,
		OmittedAtomCount:          omittedAtomCount,
		ScaleStatus:               "scale_complete",
		NonGeneralizableRuntime:   true,
		Comparable:                true,
		Guardrails:                pressure.Guardrails,
		CorpusFingerprint:         pressure.CorpusFingerprint,
		CommandConfigFingerprint:  pressure.CommandConfigFingerprint,
		PressureReplayFingerprint: pressure.ReplayFingerprint,
		GraphReplayFingerprint:    graph.ReplayFingerprint,
		ConceptIndexPath:          filepath.ToSlash(filepath.Join(CorpusConceptsDirName, "concept-index.json")),
		ReviewPacketPath:          filepath.ToSlash(filepath.Join(CorpusConceptsDirName, "review-packet.md")),
		SourceKindCoverage:        map[string]int{},
		SectionCounts:             map[CorpusConceptSection]int{},
		CandidateKindCounts:       map[SemanticCandidateKind]int{},
		RoutingHintCounts:         map[SourceMeaningPreviewRoutingHint]int{},
	}
	if pressure.ScaleStatus == "scale_partial" {
		summary.ScaleStatus = "scale_partial"
		for _, reason := range pressure.ScaleReasonCodes {
			summary.ScaleReasonCodes = appendUniqueString(summary.ScaleReasonCodes, reason)
		}
	}
	if graph.ScaleStatus == "scale_partial" {
		summary.ScaleStatus = "scale_partial"
		for _, reason := range graph.ScaleReasonCodes {
			summary.ScaleReasonCodes = appendUniqueString(summary.ScaleReasonCodes, reason)
		}
	}
	if omittedConceptCount > 0 {
		summary.ScaleStatus = "scale_partial"
		summary.ScaleReasonCodes = appendUniqueString(summary.ScaleReasonCodes, "scale_concept_limit")
	}
	coveredAtoms := 0
	crossSourceAtoms := 0
	for _, concept := range concepts {
		coveredAtoms += concept.AtomCount
		summary.EvidenceReferenceCount += concept.EvidenceReferenceCount
		summary.SectionCounts[concept.Section]++
		summary.CandidateKindCounts[concept.CandidateKind] += concept.AtomCount
		summary.RoutingHintCounts[concept.RoutingHint]++
		for kind, count := range concept.SourceKindCoverage {
			summary.SourceKindCoverage[kind] += count
		}
		if len(concept.SourceKindCoverage) > 1 {
			summary.CrossSourceKindPairCount++
			summary.CrossSourceEvidenceReferenceCount += concept.EvidenceReferenceCount
		}
		if concept.ReviewStatus != ReviewStatusReady {
			summary.ConceptReviewBurdenCount++
		}
		switch concept.Section {
		case CorpusConceptSectionCrossSource:
			summary.CrossSourceConceptCount++
			crossSourceAtoms += concept.AtomCount
		case CorpusConceptSectionLocal:
			summary.LocalConceptCount++
		case CorpusConceptSectionNeedsReview:
			summary.NeedsReviewConceptCount++
		case CorpusConceptSectionBlocked:
			summary.BlockedConceptCount++
		}
		summary.Concepts = append(summary.Concepts, CorpusConceptListItem{
			ConceptID:              concept.ConceptID,
			Title:                  concept.Title,
			ConceptKey:             concept.ConceptKey,
			Section:                concept.Section,
			CandidateKind:          concept.CandidateKind,
			RoutingHint:            concept.RoutingHint,
			AtomCount:              concept.AtomCount,
			SourceCount:            concept.SourceCount,
			EvidenceReferenceCount: concept.EvidenceReferenceCount,
			SourceKindCoverage:     cloneStringIntMap(concept.SourceKindCoverage),
			ReviewStatus:           concept.ReviewStatus,
			ReasonCodes:            append([]string{}, concept.ReasonCodes...),
			ConceptPath:            filepath.ToSlash(filepath.Join(CorpusConceptsDirName, CorpusConceptPath(concept.ConceptID))),
		})
	}
	if generatedConceptCount == len(concepts) {
		summary.GeneratedConceptCount = 0
	}
	if summary.ConceptCount > 0 {
		summary.ConceptReviewBurdenRatio = float64(summary.ConceptReviewBurdenCount) / float64(summary.ConceptCount)
	}
	if graph.RelationCount > 0 {
		summary.RelationReviewCompressionRatio = 1 - float64(summary.ConceptCount)/float64(graph.RelationCount)
	}
	if graph.AtomCount > 0 {
		summary.AtomCoverageRatio = float64(coveredAtoms) / float64(graph.AtomCount)
		summary.CrossSourceAtomRatio = float64(crossSourceAtoms) / float64(graph.AtomCount)
	}
	summary.ReplayFingerprint = corpusConceptReplayFingerprint(summary)
	return summary
}

func corpusConceptTerms(atom CorpusGraphAtom) []string {
	text := strings.Join([]string{atom.Title, atom.Summary}, " ")
	text = removeCorpusConceptURLs(text)
	parts := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	seen := map[string]bool{}
	terms := []string{}
	for _, part := range parts {
		term := strings.TrimSpace(part)
		if !corpusConceptUsefulTerm(term) || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
		if len(terms) >= 8 {
			break
		}
	}
	return terms
}

func removeCorpusConceptURLs(value string) string {
	fields := strings.Fields(value)
	for i, field := range fields {
		lower := strings.ToLower(field)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "www.") {
			fields[i] = " "
		}
	}
	return strings.Join(fields, " ")
}

func corpusConceptUsefulTerm(term string) bool {
	if len(term) < 4 || len(term) > 32 {
		return false
	}
	if corpusConceptStopWords[term] {
		return false
	}
	digits := 0
	for _, r := range term {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	if digits > len(term)/2 {
		return false
	}
	if digits > 0 && len(term) >= 8 {
		return false
	}
	return true
}

var corpusConceptStopWords = map[string]bool{
	"about": true, "after": true, "also": true, "and": true, "candidate": true, "changed": true,
	"confirmation": true, "correct": true, "from": true, "gmail": true, "have": true, "https": true,
	"http": true, "into": true, "linkedin": true, "locator": true, "message": true, "needs": true,
	"post": true, "posts": true, "private": true, "review": true, "reviewed": true, "runtime": true,
	"slack": true, "snippet": true, "snippe": true, "source": true, "that": true, "this": true,
	"timestamp": true, "topic": true, "updates": true, "what": true, "with": true, "your": true,
}

func corpusConceptTitle(key string, atoms []CorpusGraphAtom) string {
	parts := strings.Split(key, "\x00")
	if len(parts) >= 3 && parts[0] == "term" {
		return fmt.Sprintf("%s concept: %s", strings.ReplaceAll(string(atoms[0].CandidateKind), "_", " "), parts[2])
	}
	if len(parts) >= 3 && parts[0] == "local" {
		return fmt.Sprintf("Local %s concept: %s", strings.ReplaceAll(string(atoms[0].CandidateKind), "_", " "), parts[2])
	}
	if len(parts) >= 2 && parts[0] == "relation" {
		return fmt.Sprintf("Cross-source %s relation neighborhood", strings.ReplaceAll(string(atoms[0].CandidateKind), "_", " "))
	}
	return fmt.Sprintf("%s concept", strings.ReplaceAll(string(atoms[0].CandidateKind), "_", " "))
}

func corpusConceptID(corpusID, key string, atoms []CorpusGraphAtom) string {
	parts := []string{corpusID, key}
	for _, atom := range atoms {
		parts = append(parts, atom.AtomID)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "concept-" + hex.EncodeToString(sum[:])[:16]
}

func corpusConceptReplayFingerprint(summary CorpusConceptSummary) string {
	parts := []string{
		summary.CorpusID,
		summary.CorpusFingerprint,
		summary.CommandConfigFingerprint,
		fmt.Sprintf("counts:%d:%d:%d:%d:%d:%d", summary.AtomCount, summary.RelationCount, summary.ConceptCount, summary.CrossSourceConceptCount, summary.EvidenceReferenceCount, summary.ConceptReviewBurdenCount),
	}
	for _, concept := range summary.Concepts {
		parts = append(parts, strings.Join([]string{concept.ConceptID, string(concept.Section), string(concept.CandidateKind), fmt.Sprintf("%d", concept.AtomCount)}, ":"))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "corpus-concepts-" + hex.EncodeToString(sum[:])[:16]
}

func corpusConceptSortKey(concept CorpusConcept) string {
	sectionRank := map[CorpusConceptSection]string{
		CorpusConceptSectionCrossSource: "0",
		CorpusConceptSectionNeedsReview: "1",
		CorpusConceptSectionLocal:       "2",
		CorpusConceptSectionBlocked:     "3",
	}
	rank := sectionRank[concept.Section]
	return fmt.Sprintf("%s:%03d:%03d:%s", rank, 999-concept.SourceCount, 999-concept.AtomCount, concept.ConceptID)
}

func sortCorpusConceptAtoms(atoms []CorpusGraphAtom) {
	sort.Slice(atoms, func(i, j int) bool {
		return strings.Join([]string{atoms[i].SourceID, fmt.Sprintf("%08d", atoms[i].LineStart), atoms[i].AtomID}, "\x00") <
			strings.Join([]string{atoms[j].SourceID, fmt.Sprintf("%08d", atoms[j].LineStart), atoms[j].AtomID}, "\x00")
	})
}

func sourceKindForConcept(atom CorpusGraphAtom) string {
	sourceID := strings.ToLower(strings.TrimSpace(atom.SourceID))
	switch {
	case strings.HasPrefix(sourceID, "gmail-"):
		return "gmail"
	case strings.HasPrefix(sourceID, "slack-"):
		return "slack"
	}
	if strings.TrimSpace(atom.SourceKind) != "" {
		return strings.ToLower(strings.TrimSpace(atom.SourceKind))
	}
	return "unknown"
}

func cloneStringIntMap(input map[string]int) map[string]int {
	out := map[string]int{}
	for key, value := range input {
		out[key] = value
	}
	return out
}

func containsCorpusConceptString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
