package documents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const SourceMeaningPreviewSchemaVersion = "source-meaning-preview/v0.1"

type SourceMeaningPreviewRoutingHint string

const (
	SourceMeaningRoutingTolariaCandidate      SourceMeaningPreviewRoutingHint = "tolaria_candidate"
	SourceMeaningRoutingProductBrainCandidate SourceMeaningPreviewRoutingHint = "product_brain_candidate"
	SourceMeaningRoutingBothCandidate         SourceMeaningPreviewRoutingHint = "both_candidate"
	SourceMeaningRoutingNoOp                  SourceMeaningPreviewRoutingHint = "no_op"
	SourceMeaningRoutingNeedsEnrichment       SourceMeaningPreviewRoutingHint = "needs_enrichment"
	SourceMeaningRoutingBlocked               SourceMeaningPreviewRoutingHint = "blocked"
)

type SourceMeaningPreviewMissingness string

const (
	SourceMeaningMissingnessNone                  SourceMeaningPreviewMissingness = "none"
	SourceMeaningMissingnessNoSemanticCandidates  SourceMeaningPreviewMissingness = "no_semantic_candidates"
	SourceMeaningMissingnessMissingEvidence       SourceMeaningPreviewMissingness = "missing_evidence"
	SourceMeaningMissingnessReferenceOnly         SourceMeaningPreviewMissingness = "reference_only"
	SourceMeaningMissingnessLinkOnlySource        SourceMeaningPreviewMissingness = "link_only_source"
	SourceMeaningMissingnessMissingLinkEnrichment SourceMeaningPreviewMissingness = "missing_link_enrichment"
	SourceMeaningMissingnessBlockedSource         SourceMeaningPreviewMissingness = "blocked_source"
	SourceMeaningMissingnessSkippedSource         SourceMeaningPreviewMissingness = "skipped_source"
)

type SourceMeaningPreviewGuardrails struct {
	HostedInferenceCalls   int `json:"hosted_inference_calls"`
	HostedTelemetryExports int `json:"hosted_telemetry_exports"`
	DestinationWrites      int `json:"destination_writes"`
	ProductBrainWrites     int `json:"product_brain_writes"`
	TolariaWrites          int `json:"tolaria_writes"`
}

type SourceMeaningPreviewSummary struct {
	SchemaVersion         string                                  `json:"schema_version"`
	CorpusID              string                                  `json:"corpus_id"`
	SourceCount           int                                     `json:"source_count"`
	ProcessedSourceCount  int                                     `json:"processed_source_count"`
	SkippedSourceCount    int                                     `json:"skipped_source_count"`
	ExcludedSourceCount   int                                     `json:"excluded_source_count"`
	BlockedSourceCount    int                                     `json:"blocked_source_count"`
	PreviewedSourceCount  int                                     `json:"previewed_source_count"`
	AtomCount             int                                     `json:"atom_count"`
	RelationCount         int                                     `json:"relation_count"`
	MissingnessCounts     map[SourceMeaningPreviewMissingness]int `json:"missingness_counts"`
	RoutingHintCounts     map[SourceMeaningPreviewRoutingHint]int `json:"routing_hint_counts"`
	CandidateKindCounts   map[SemanticCandidateKind]int           `json:"candidate_kind_counts"`
	RelationTypeCounts    map[CorpusRelationType]int              `json:"relation_type_counts"`
	PreviewCoverageRatio  float64                                 `json:"preview_coverage_ratio"`
	EvidenceCoverageRatio float64                                 `json:"evidence_coverage_ratio"`
	RoutingCoverageRatio  float64                                 `json:"routing_coverage_ratio"`
	Guardrails            SourceMeaningPreviewGuardrails          `json:"guardrails"`
	ReportPath            string                                  `json:"report_path"`
	Items                 []SourceMeaningPreviewItemSummary       `json:"items"`
}

type SourceMeaningPreviewItemSummary struct {
	SourceID      string                            `json:"source_id"`
	SourceKind    string                            `json:"source_kind"`
	SourceLabel   string                            `json:"source_label"`
	State         CorpusPressureSourceState         `json:"state"`
	PreviewPath   string                            `json:"preview_path"`
	AtomCount     int                               `json:"atom_count"`
	RelationCount int                               `json:"relation_count"`
	Missingness   []SourceMeaningPreviewMissingness `json:"missingness"`
	RoutingHints  []SourceMeaningPreviewRoutingHint `json:"routing_hints"`
}

type SourceMeaningPreviewItem struct {
	SourceID      string                            `json:"source_id"`
	SourceKind    string                            `json:"source_kind"`
	SourceLabel   string                            `json:"source_label"`
	State         CorpusPressureSourceState         `json:"state"`
	ReasonCode    CorpusPressureReason              `json:"reason_code"`
	Message       string                            `json:"message,omitempty"`
	SourcePath    string                            `json:"source_path"`
	AtomCount     int                               `json:"atom_count"`
	RelationCount int                               `json:"relation_count"`
	Atoms         []SourceMeaningPreviewAtom        `json:"atoms"`
	Relations     []SourceMeaningPreviewRelation    `json:"relations"`
	Missingness   []SourceMeaningPreviewMissingness `json:"missingness"`
	RoutingHints  []SourceMeaningPreviewRoute       `json:"routing_hints"`
	PreviewPath   string                            `json:"preview_path"`
}

type SourceMeaningPreviewAtom struct {
	AtomID        string                            `json:"atom_id"`
	CandidateKind SemanticCandidateKind             `json:"candidate_kind"`
	ReviewStatus  ReviewStatus                      `json:"review_status"`
	Confidence    Confidence                        `json:"confidence"`
	Title         string                            `json:"title"`
	Summary       string                            `json:"summary"`
	LineStart     int                               `json:"line_start"`
	LineEnd       int                               `json:"line_end"`
	Excerpt       string                            `json:"excerpt"`
	RoutingHints  []SourceMeaningPreviewRoute       `json:"routing_hints"`
	Missingness   []SourceMeaningPreviewMissingness `json:"missingness"`
}

type SourceMeaningPreviewRelation struct {
	RelationID   string             `json:"relation_id"`
	RelationType CorpusRelationType `json:"relation_type"`
	Confidence   Confidence         `json:"confidence"`
	ReviewStatus ReviewStatus       `json:"review_status"`
	OtherAtomID  string             `json:"other_atom_id"`
	OtherSource  string             `json:"other_source"`
	ReasonCode   string             `json:"reason_code"`
	Evidence     []string           `json:"evidence"`
}

type SourceMeaningPreviewRoute struct {
	Hint          SourceMeaningPreviewRoutingHint `json:"hint"`
	ReasonCodes   []string                        `json:"reason_codes"`
	WriteEligible bool                            `json:"write_eligible"`
}

func BuildSourceMeaningPreview(inputPath, outDir string) (SourceMeaningPreviewSummary, []SourceMeaningPreviewItem, error) {
	if strings.TrimSpace(outDir) == "" {
		return SourceMeaningPreviewSummary{}, nil, fmt.Errorf("missing required --out")
	}
	root, pressureSummary, err := readSourceMeaningPressureSummary(inputPath)
	if err != nil {
		return SourceMeaningPreviewSummary{}, nil, err
	}
	graphSummary, atomsBySource, relationsBySource, err := readSourceMeaningGraph(root, pressureSummary.GraphSummaryPath)
	if err != nil {
		return SourceMeaningPreviewSummary{}, nil, err
	}
	items := buildSourceMeaningItems(root, pressureSummary, atomsBySource, relationsBySource)
	summary := buildSourceMeaningSummary(pressureSummary, graphSummary, items)
	for i := range items {
		items[i].PreviewPath = filepath.ToSlash(filepath.Join(SourceMeaningPreviewDirName, "sources", sanitizeID(items[i].SourceID)+".md"))
		summary.Items[i].PreviewPath = items[i].PreviewPath
	}
	if err := WriteSourceMeaningPreview(outDir, summary, items); err != nil {
		return SourceMeaningPreviewSummary{}, nil, err
	}
	return summary, items, nil
}

func readSourceMeaningPressureSummary(inputPath string) (string, CorpusPressureSummary, error) {
	if strings.TrimSpace(inputPath) == "" {
		return "", CorpusPressureSummary{}, fmt.Errorf("missing corpus pressure path")
	}
	root, err := filepath.Abs(inputPath)
	if err != nil {
		return "", CorpusPressureSummary{}, err
	}
	summaryPath := filepath.Join(root, CorpusPressureDirName, "pressure-summary.json")
	if filepath.Base(root) == CorpusPressureDirName {
		summaryPath = filepath.Join(root, "pressure-summary.json")
		root = filepath.Dir(root)
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return "", CorpusPressureSummary{}, fmt.Errorf("read corpus pressure summary: %w", err)
	}
	var summary CorpusPressureSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return "", CorpusPressureSummary{}, fmt.Errorf("decode corpus pressure summary: %w", err)
	}
	if summary.SchemaVersion != CorpusPressureSummarySchemaVersion {
		return "", CorpusPressureSummary{}, fmt.Errorf("unsupported corpus pressure summary schema version: %s", summary.SchemaVersion)
	}
	return root, summary, nil
}

func readSourceMeaningGraph(root, graphSummaryPath string) (CorpusGraphSummary, map[string][]CorpusGraphAtom, map[string][]CorpusGraphRelation, error) {
	atomsBySource := map[string][]CorpusGraphAtom{}
	relationsBySource := map[string][]CorpusGraphRelation{}
	if strings.TrimSpace(graphSummaryPath) == "" {
		return CorpusGraphSummary{RelationTypeCounts: map[CorpusRelationType]int{}}, atomsBySource, relationsBySource, nil
	}
	summaryPath, err := containedManifestPath(root, graphSummaryPath)
	if err != nil {
		return CorpusGraphSummary{}, nil, nil, err
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return CorpusGraphSummary{}, nil, nil, fmt.Errorf("read corpus graph summary: %w", err)
	}
	var summary CorpusGraphSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return CorpusGraphSummary{}, nil, nil, fmt.Errorf("decode corpus graph summary: %w", err)
	}
	if summary.SchemaVersion != CorpusGraphSummarySchemaVersion {
		return CorpusGraphSummary{}, nil, nil, fmt.Errorf("unsupported corpus graph summary schema version: %s", summary.SchemaVersion)
	}
	graphRoot := filepath.Dir(summaryPath)
	for _, atomSummary := range summary.Atoms {
		atomPath, err := containedManifestPath(graphRoot, atomSummary.AtomPath)
		if err != nil {
			return CorpusGraphSummary{}, nil, nil, err
		}
		var atom CorpusGraphAtom
		if err := readSourceMeaningJSONFile(atomPath, &atom); err != nil {
			return CorpusGraphSummary{}, nil, nil, err
		}
		atomsBySource[atom.SourceID] = append(atomsBySource[atom.SourceID], atom)
	}
	for _, relationSummary := range summary.Relations {
		relationPath, err := containedManifestPath(graphRoot, relationSummary.RelationPath)
		if err != nil {
			return CorpusGraphSummary{}, nil, nil, err
		}
		var relation CorpusGraphRelation
		if err := readSourceMeaningJSONFile(relationPath, &relation); err != nil {
			return CorpusGraphSummary{}, nil, nil, err
		}
		fromSource := sourceForRelationEndpoint(summary, relation.FromAtomID)
		if fromSource != "" {
			relationsBySource[fromSource] = append(relationsBySource[fromSource], relation)
		}
		if toSource := sourceForRelationEndpoint(summary, relation.ToAtomID); toSource != "" && toSource != fromSource {
			relationsBySource[toSource] = append(relationsBySource[toSource], relation)
		}
	}
	return summary, atomsBySource, relationsBySource, nil
}

func readSourceMeaningJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func sourceForRelationEndpoint(summary CorpusGraphSummary, atomID string) string {
	for _, atom := range summary.Atoms {
		if atom.AtomID == atomID {
			return atom.SourceID
		}
	}
	return ""
}

func buildSourceMeaningItems(root string, pressure CorpusPressureSummary, atomsBySource map[string][]CorpusGraphAtom, relationsBySource map[string][]CorpusGraphRelation) []SourceMeaningPreviewItem {
	items := make([]SourceMeaningPreviewItem, 0, len(pressure.Sources))
	for _, source := range pressure.Sources {
		sourceContent := readSourceMeaningSourceContent(root, source.SourcePath)
		sourceAtoms := atomsBySource[source.SourceID]
		sort.Slice(sourceAtoms, func(i, j int) bool { return sourceAtoms[i].AtomID < sourceAtoms[j].AtomID })
		item := SourceMeaningPreviewItem{
			SourceID:    source.SourceID,
			SourceKind:  source.SourceKind,
			SourceLabel: source.SourceLabel,
			State:       source.State,
			ReasonCode:  source.ReasonCode,
			Message:     source.Message,
			SourcePath:  sourceMeaningDisplayPath(source.SourcePath),
		}
		for _, atom := range sourceAtoms {
			atomPreview := sourceMeaningAtomPreview(atom, sourceContent)
			item.Atoms = append(item.Atoms, atomPreview)
			item.Missingness = appendMeaningMissingness(item.Missingness, atomPreview.Missingness...)
			item.RoutingHints = appendMeaningRoutes(item.RoutingHints, atomPreview.RoutingHints...)
		}
		item.Relations = sourceMeaningRelations(source.SourceID, relationsBySource[source.SourceID])
		item.RelationCount = len(item.Relations)
		item.AtomCount = len(item.Atoms)
		item.Missingness = appendMeaningMissingness(item.Missingness, sourceMissingness(source, item, sourceContent)...)
		if len(item.RoutingHints) == 0 {
			item.RoutingHints = append(item.RoutingHints, sourceLevelRoute(source, item))
		}
		items = append(items, item)
	}
	return items
}

func readSourceMeaningSourceContent(root, sourcePath string) string {
	if strings.TrimSpace(sourcePath) == "" {
		return ""
	}
	path := sourcePath
	if filepath.IsAbs(path) {
		return ""
	}
	var err error
	path, err = containedManifestPath(root, path)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func sourceMeaningDisplayPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Base(path)
	}
	return filepath.ToSlash(path)
}

func sourceMeaningAtomPreview(atom CorpusGraphAtom, sourceContent string) SourceMeaningPreviewAtom {
	routes, missing := routesForAtom(atom, sourceContent)
	excerpt := safeSourceMeaningExcerpt(atom.Excerpt)
	if strings.TrimSpace(excerpt) == "" {
		missing = appendMeaningMissingness(missing, SourceMeaningMissingnessMissingEvidence)
	}
	if excerpt != atom.Excerpt {
		routes = []SourceMeaningPreviewRoute{{Hint: SourceMeaningRoutingBlocked, ReasonCodes: []string{"unsafe_private_marker"}, WriteEligible: false}}
		missing = appendMeaningMissingness(missing, SourceMeaningMissingnessBlockedSource)
	}
	return SourceMeaningPreviewAtom{
		AtomID:        atom.AtomID,
		CandidateKind: atom.CandidateKind,
		ReviewStatus:  atom.ReviewStatus,
		Confidence:    atom.Confidence,
		Title:         atom.Title,
		Summary:       atom.Summary,
		LineStart:     atom.LineStart,
		LineEnd:       atom.LineEnd,
		Excerpt:       excerpt,
		RoutingHints:  routes,
		Missingness:   missing,
	}
}

func safeSourceMeaningExcerpt(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if containsUnsafeMarker(value) || containsGovernanceID(value) {
		return "[redacted unsafe/private evidence excerpt]"
	}
	return value
}

func routesForAtom(atom CorpusGraphAtom, sourceContent string) ([]SourceMeaningPreviewRoute, []SourceMeaningPreviewMissingness) {
	if atom.ReviewStatus == ReviewStatusBlocked {
		return []SourceMeaningPreviewRoute{{Hint: SourceMeaningRoutingBlocked, ReasonCodes: []string{"blocked_atom"}, WriteEligible: false}}, []SourceMeaningPreviewMissingness{SourceMeaningMissingnessBlockedSource}
	}
	if atom.CandidateKind == SemanticCandidateKindReference && containsURL(sourceContent+"\n"+atom.Excerpt+"\n"+atom.Summary) {
		return []SourceMeaningPreviewRoute{{Hint: SourceMeaningRoutingNeedsEnrichment, ReasonCodes: []string{"reference_only", "link_only_source", "missing_link_enrichment"}, WriteEligible: false}}, []SourceMeaningPreviewMissingness{SourceMeaningMissingnessReferenceOnly, SourceMeaningMissingnessLinkOnlySource, SourceMeaningMissingnessMissingLinkEnrichment}
	}
	switch atom.CandidateKind {
	case SemanticCandidateKindDecision, SemanticCandidateKindIssue, SemanticCandidateKindRequirement, SemanticCandidateKindDependency, SemanticCandidateKindRisk, SemanticCandidateKindCapability:
		return []SourceMeaningPreviewRoute{{Hint: SourceMeaningRoutingProductBrainCandidate, ReasonCodes: []string{string(atom.CandidateKind)}, WriteEligible: false}}, nil
	case SemanticCandidateKindAction:
		return []SourceMeaningPreviewRoute{{Hint: SourceMeaningRoutingBothCandidate, ReasonCodes: []string{"action_candidate"}, WriteEligible: false}}, nil
	case SemanticCandidateKindTopic, SemanticCandidateKindQuestion, SemanticCandidateKindReference:
		return []SourceMeaningPreviewRoute{{Hint: SourceMeaningRoutingTolariaCandidate, ReasonCodes: []string{string(atom.CandidateKind)}, WriteEligible: false}}, nil
	default:
		return []SourceMeaningPreviewRoute{{Hint: SourceMeaningRoutingNoOp, ReasonCodes: []string{"unknown_candidate"}, WriteEligible: false}}, nil
	}
}

func containsURL(value string) bool {
	return strings.Contains(value, "http://") || strings.Contains(value, "https://")
}

func sourceMissingness(source CorpusPressureSourceResult, item SourceMeaningPreviewItem, sourceContent string) []SourceMeaningPreviewMissingness {
	switch source.State {
	case CorpusPressureSourceBlocked:
		return []SourceMeaningPreviewMissingness{SourceMeaningMissingnessBlockedSource}
	case CorpusPressureSourceSkipped, CorpusPressureSourceExcluded:
		return []SourceMeaningPreviewMissingness{SourceMeaningMissingnessSkippedSource}
	}
	if item.AtomCount == 0 {
		return []SourceMeaningPreviewMissingness{SourceMeaningMissingnessNoSemanticCandidates}
	}
	if len(item.Missingness) == 0 && strings.TrimSpace(sourceContent) != "" {
		return []SourceMeaningPreviewMissingness{SourceMeaningMissingnessNone}
	}
	return nil
}

func sourceLevelRoute(source CorpusPressureSourceResult, item SourceMeaningPreviewItem) SourceMeaningPreviewRoute {
	if source.State == CorpusPressureSourceBlocked {
		return SourceMeaningPreviewRoute{Hint: SourceMeaningRoutingBlocked, ReasonCodes: []string{"blocked_source"}, WriteEligible: false}
	}
	if item.AtomCount == 0 {
		return SourceMeaningPreviewRoute{Hint: SourceMeaningRoutingNoOp, ReasonCodes: []string{string(source.ReasonCode)}, WriteEligible: false}
	}
	return SourceMeaningPreviewRoute{Hint: SourceMeaningRoutingNoOp, ReasonCodes: []string{"no_route"}, WriteEligible: false}
}

func sourceMeaningRelations(sourceID string, relations []CorpusGraphRelation) []SourceMeaningPreviewRelation {
	out := []SourceMeaningPreviewRelation{}
	for _, relation := range relations {
		preview := SourceMeaningPreviewRelation{
			RelationID:   relation.RelationID,
			RelationType: relation.RelationType,
			Confidence:   relation.Confidence,
			ReviewStatus: relation.ReviewStatus,
			ReasonCode:   relation.ReasonCode,
		}
		for _, evidence := range relation.Evidence {
			if evidence.SourceID != sourceID {
				preview.OtherSource = evidence.SourceID
			}
			if strings.TrimSpace(evidence.Excerpt) != "" {
				preview.Evidence = append(preview.Evidence, safeSourceMeaningExcerpt(evidence.Excerpt))
			}
		}
		out = append(out, preview)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelationID < out[j].RelationID })
	return out
}

func buildSourceMeaningSummary(pressure CorpusPressureSummary, graph CorpusGraphSummary, items []SourceMeaningPreviewItem) SourceMeaningPreviewSummary {
	summary := SourceMeaningPreviewSummary{
		SchemaVersion:        SourceMeaningPreviewSchemaVersion,
		CorpusID:             pressure.CorpusID,
		SourceCount:          pressure.SourceCount,
		ProcessedSourceCount: pressure.ProcessedSourceCount,
		SkippedSourceCount:   pressure.SkippedSourceCount,
		ExcludedSourceCount:  pressure.ExcludedSourceCount,
		BlockedSourceCount:   pressure.BlockedSourceCount,
		AtomCount:            graph.AtomCount,
		RelationCount:        graph.RelationCount,
		MissingnessCounts:    map[SourceMeaningPreviewMissingness]int{},
		RoutingHintCounts:    map[SourceMeaningPreviewRoutingHint]int{},
		CandidateKindCounts:  map[SemanticCandidateKind]int{},
		RelationTypeCounts:   cloneSourceMeaningRelationTypeCounts(graph.RelationTypeCounts),
		ReportPath:           filepath.ToSlash(filepath.Join(SourceMeaningPreviewDirName, "meaning-report.md")),
		Guardrails: SourceMeaningPreviewGuardrails{
			HostedInferenceCalls:   0,
			HostedTelemetryExports: 0,
			DestinationWrites:      0,
			ProductBrainWrites:     0,
			TolariaWrites:          0,
		},
	}
	evidenceReady := 0
	routedAtoms := 0
	for _, item := range items {
		if item.State == CorpusPressureSourceProcessed {
			summary.PreviewedSourceCount++
		}
		itemHints := []SourceMeaningPreviewRoutingHint{}
		for _, route := range item.RoutingHints {
			summary.RoutingHintCounts[route.Hint]++
			itemHints = appendUniqueRouteHint(itemHints, route.Hint)
		}
		for _, missing := range item.Missingness {
			summary.MissingnessCounts[missing]++
		}
		for _, atom := range item.Atoms {
			summary.CandidateKindCounts[atom.CandidateKind]++
			if strings.TrimSpace(atom.Excerpt) != "" || len(atom.Missingness) > 0 {
				evidenceReady++
			}
			if len(atom.RoutingHints) > 0 {
				routedAtoms++
			}
		}
		summary.Items = append(summary.Items, SourceMeaningPreviewItemSummary{
			SourceID:      item.SourceID,
			SourceKind:    item.SourceKind,
			SourceLabel:   item.SourceLabel,
			State:         item.State,
			AtomCount:     item.AtomCount,
			RelationCount: item.RelationCount,
			Missingness:   item.Missingness,
			RoutingHints:  itemHints,
		})
	}
	if pressure.ProcessedSourceCount > 0 {
		summary.PreviewCoverageRatio = float64(summary.PreviewedSourceCount) / float64(pressure.ProcessedSourceCount)
	}
	if graph.AtomCount > 0 {
		summary.EvidenceCoverageRatio = float64(evidenceReady) / float64(graph.AtomCount)
		summary.RoutingCoverageRatio = float64(routedAtoms) / float64(graph.AtomCount)
	}
	return summary
}

func cloneSourceMeaningRelationTypeCounts(counts map[CorpusRelationType]int) map[CorpusRelationType]int {
	out := map[CorpusRelationType]int{}
	for key, value := range counts {
		out[key] = value
	}
	return out
}

func appendUniqueRouteHint(values []SourceMeaningPreviewRoutingHint, value SourceMeaningPreviewRoutingHint) []SourceMeaningPreviewRoutingHint {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendMeaningMissingness(values []SourceMeaningPreviewMissingness, additions ...SourceMeaningPreviewMissingness) []SourceMeaningPreviewMissingness {
	seen := map[SourceMeaningPreviewMissingness]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if value == "" || seen[value] {
			continue
		}
		values = append(values, value)
		seen[value] = true
	}
	return values
}

func appendMeaningRoutes(values []SourceMeaningPreviewRoute, additions ...SourceMeaningPreviewRoute) []SourceMeaningPreviewRoute {
	seen := map[SourceMeaningPreviewRoutingHint]bool{}
	for _, value := range values {
		seen[value.Hint] = true
	}
	for _, value := range additions {
		if value.Hint == "" || seen[value.Hint] {
			continue
		}
		values = append(values, value)
		seen[value.Hint] = true
	}
	return values
}
