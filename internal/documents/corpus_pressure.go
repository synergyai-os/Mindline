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
)

const (
	CorpusPressureManifestSchemaVersion = "corpus-pressure-manifest/v0.1"
	CorpusPressureSummarySchemaVersion  = "corpus-pressure-summary/v0.1"
)

type CorpusPressureManifest struct {
	SchemaVersion string                         `json:"schema_version"`
	CorpusID      string                         `json:"corpus_id"`
	Sources       []CorpusPressureManifestSource `json:"sources"`
}

type CorpusPressureManifestSource struct {
	SourceID   string `json:"source_id"`
	SourceKind string `json:"source_kind"`
	Path       string `json:"path"`
}

type CorpusPressureOptions struct {
	SemanticOptions SemanticOptions
}

type CorpusPressureSourceState string

const (
	CorpusPressureSourceProcessed CorpusPressureSourceState = "processed"
	CorpusPressureSourceSkipped   CorpusPressureSourceState = "skipped"
	CorpusPressureSourceBlocked   CorpusPressureSourceState = "blocked"
)

type CorpusPressureReason string

const (
	CorpusPressureReasonNone                  CorpusPressureReason = "none"
	CorpusPressureReasonNoSemanticCandidates  CorpusPressureReason = "no_semantic_candidates"
	CorpusPressureReasonSemanticSkipped       CorpusPressureReason = "semantic_skipped"
	CorpusPressureReasonSemanticError         CorpusPressureReason = "semantic_error"
	CorpusPressureReasonInputContainmentError CorpusPressureReason = "input_containment_error"
)

type CorpusPressureSummary struct {
	SchemaVersion              string                       `json:"schema_version"`
	CorpusID                   string                       `json:"corpus_id"`
	SourceCount                int                          `json:"source_count"`
	ProcessedSourceCount       int                          `json:"processed_source_count"`
	SkippedSourceCount         int                          `json:"skipped_source_count"`
	BlockedSourceCount         int                          `json:"blocked_source_count"`
	SemanticCandidateCount     int                          `json:"semantic_candidate_count"`
	GraphAtomCount             int                          `json:"graph_atom_count"`
	GraphRelationCount         int                          `json:"graph_relation_count"`
	RelationTypeCounts         map[CorpusRelationType]int   `json:"relation_type_counts"`
	RelationStatusCounts       map[ReviewStatus]int         `json:"relation_status_counts"`
	EvidenceReadyAtomCount     int                          `json:"evidence_ready_atom_count"`
	EvidenceReadyRelationCount int                          `json:"evidence_ready_relation_count"`
	ReviewBurdenCount          int                          `json:"review_burden_count"`
	ReviewBurdenRatio          float64                      `json:"review_burden_ratio"`
	ReadyForFiftyFilePressure  bool                         `json:"ready_for_50_file_pressure"`
	ReplayFingerprint          string                       `json:"replay_fingerprint"`
	GraphReplayFingerprint     string                       `json:"graph_replay_fingerprint"`
	GraphManifestPath          string                       `json:"graph_manifest_path"`
	GraphSummaryPath           string                       `json:"graph_summary_path"`
	Blockers                   []string                     `json:"blockers"`
	NextImprovementTargets     []string                     `json:"next_improvement_targets"`
	Sources                    []CorpusPressureSourceResult `json:"sources"`
}

type CorpusPressureSourceResult struct {
	SourceID            string                        `json:"source_id"`
	SourceKind          string                        `json:"source_kind"`
	SourceLabel         string                        `json:"source_label"`
	State               CorpusPressureSourceState     `json:"state"`
	ReasonCode          CorpusPressureReason          `json:"reason_code"`
	Message             string                        `json:"message,omitempty"`
	CandidateCount      int                           `json:"candidate_count"`
	CandidateKindCounts map[SemanticCandidateKind]int `json:"candidate_kind_counts,omitempty"`
	SemanticRunID       string                        `json:"semantic_run_id,omitempty"`
	SourcePath          string                        `json:"source_path"`
	SemanticRunDir      string                        `json:"semantic_run_dir,omitempty"`
}

type corpusPressureSourceInput struct {
	SourceID   string
	SourceKind string
	Path       string
	Label      string
}

func BuildCorpusPressure(inputPath, outDir string, options CorpusPressureOptions) (CorpusPressureSummary, CorpusGraphSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return CorpusPressureSummary{}, CorpusGraphSummary{}, fmt.Errorf("missing required --out")
	}
	if options.SemanticOptions.Classifier == "" {
		options.SemanticOptions.Classifier = SemanticClassifierDeterministic
	}
	sources, corpusID, err := loadCorpusPressureSources(inputPath)
	if err != nil {
		return CorpusPressureSummary{}, CorpusGraphSummary{}, err
	}
	root, err := filepath.Abs(outDir)
	if err != nil {
		return CorpusPressureSummary{}, CorpusGraphSummary{}, err
	}
	if err := rejectSymlinkAncestors(root); err != nil {
		return CorpusPressureSummary{}, CorpusGraphSummary{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return CorpusPressureSummary{}, CorpusGraphSummary{}, err
	}
	results := make([]CorpusPressureSourceResult, 0, len(sources))
	graphSources := make([]CorpusGraphManifestSource, 0, len(sources))
	for _, source := range sources {
		result, graphSource := runCorpusPressureSource(root, source, options.SemanticOptions)
		results = append(results, result)
		if graphSource != nil {
			graphSources = append(graphSources, *graphSource)
		}
	}
	graphManifest := CorpusGraphManifest{
		SchemaVersion: CorpusGraphManifestSchemaVersion,
		CorpusID:      corpusID,
		Sources:       graphSources,
	}
	graphManifestPath := filepath.Join(root, "corpus-graph-manifest.json")
	if err := writeJSON(root, "corpus-graph-manifest.json", graphManifest); err != nil {
		return CorpusPressureSummary{}, CorpusGraphSummary{}, err
	}
	graphSummary := CorpusGraphSummary{
		SchemaVersion:             CorpusGraphSummarySchemaVersion,
		CorpusID:                  corpusID,
		RelationTypeCounts:        map[CorpusRelationType]int{},
		RelationStatusCounts:      map[ReviewStatus]int{},
		ReadyForFiftyFilePressure: false,
	}
	var graphErr error
	if len(graphSources) > 0 {
		var atoms []CorpusGraphAtom
		var relations []CorpusGraphRelation
		var reviews []CorpusGraphReviewItem
		graphSummary, atoms, relations, reviews, graphErr = BuildCorpusGraph(graphManifestPath)
		if graphErr == nil {
			graphErr = WriteCorpusGraph(root, graphSummary, atoms, relations, reviews)
		}
	}
	summary := buildCorpusPressureSummary(corpusID, results, graphSummary, graphManifestPath, graphErr)
	if err := WriteCorpusPressure(root, summary, graphSummary); err != nil {
		return CorpusPressureSummary{}, CorpusGraphSummary{}, err
	}
	if graphErr != nil {
		return summary, graphSummary, graphErr
	}
	return summary, graphSummary, nil
}

func loadCorpusPressureSources(inputPath string) ([]corpusPressureSourceInput, string, error) {
	if strings.TrimSpace(inputPath) == "" {
		return nil, "", fmt.Errorf("missing input path")
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() && strings.HasSuffix(strings.ToLower(inputPath), ".json") {
		return loadCorpusPressureManifest(inputPath)
	}
	absInput, err := filepath.Abs(inputPath)
	if err != nil {
		return nil, "", err
	}
	paths, err := markdownPaths(inputPath)
	if err != nil {
		return nil, "", err
	}
	sort.Strings(paths)
	seen := map[string]int{}
	out := make([]corpusPressureSourceInput, 0, len(paths))
	for _, path := range paths {
		if shouldSkipCorpusPressurePath(inputPath, path) {
			continue
		}
		safePath, err := containedDiscoveredSourcePath(absInput, path)
		if err != nil {
			return nil, "", err
		}
		rel, err := filepath.Rel(absInput, safePath)
		if err != nil {
			return nil, "", err
		}
		id := sanitizeID(strings.TrimSuffix(filepath.ToSlash(rel), filepath.Ext(rel)))
		if id == "" {
			id = "source"
		}
		seen[id]++
		if seen[id] > 1 {
			id = fmt.Sprintf("%s-%d", id, seen[id])
		}
		out = append(out, corpusPressureSourceInput{
			SourceID:   id,
			SourceKind: SourceKindMarkdown,
			Path:       safePath,
			Label:      filepath.ToSlash(rel),
		})
	}
	if len(out) == 0 {
		return nil, "", fmt.Errorf("corpus pressure requires markdown sources")
	}
	return out, "corpus-" + shortHash(filepath.ToSlash(inputPath)), nil
}

func loadCorpusPressureManifest(path string) ([]corpusPressureSourceInput, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var manifest CorpusPressureManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, "", err
	}
	if manifest.SchemaVersion != CorpusPressureManifestSchemaVersion {
		return nil, "", fmt.Errorf("unsupported corpus pressure manifest schema version: %s", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.CorpusID) == "" || sanitizeID(manifest.CorpusID) != manifest.CorpusID {
		return nil, "", fmt.Errorf("unsafe corpus id: %s", manifest.CorpusID)
	}
	if len(manifest.Sources) == 0 {
		return nil, "", fmt.Errorf("corpus pressure manifest requires sources")
	}
	dir := filepath.Dir(path)
	seen := map[string]bool{}
	out := make([]corpusPressureSourceInput, 0, len(manifest.Sources))
	for _, source := range manifest.Sources {
		if strings.TrimSpace(source.SourceID) == "" || sanitizeID(source.SourceID) != source.SourceID {
			return nil, "", fmt.Errorf("unsafe source id: %s", source.SourceID)
		}
		if seen[source.SourceID] {
			return nil, "", fmt.Errorf("duplicate source id: %s", source.SourceID)
		}
		seen[source.SourceID] = true
		if source.SourceKind != SourceKindMarkdown {
			return nil, "", fmt.Errorf("unsupported source kind: %s", source.SourceKind)
		}
		resolved, err := containedManifestPath(dir, source.Path)
		if err != nil {
			return nil, "", fmt.Errorf("source path %s: %w", source.SourceID, err)
		}
		safePath, err := containedDiscoveredSourcePath(dir, resolved)
		if err != nil {
			return nil, "", fmt.Errorf("source path %s: %w", source.SourceID, err)
		}
		out = append(out, corpusPressureSourceInput{
			SourceID:   source.SourceID,
			SourceKind: source.SourceKind,
			Path:       safePath,
			Label:      filepath.ToSlash(source.Path),
		})
	}
	return out, manifest.CorpusID, nil
}

func containedDiscoveredSourcePath(root, path string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := rejectSymlinkAncestors(absPath); err != nil {
		return "", err
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("source path escaped corpus directory")
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", err
	}
	if !isInside(realRoot, realPath) {
		return "", fmt.Errorf("source path escaped corpus directory")
	}
	return realPath, nil
}

func shouldSkipCorpusPressurePath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, part := range parts {
		switch part {
		case ".git", ".productbrain", "node_modules", "corpus-pressure", "corpus-graph", "semantic-candidates", "document-structure", "document-segments":
			return true
		}
	}
	return false
}

func runCorpusPressureSource(root string, source corpusPressureSourceInput, options SemanticOptions) (CorpusPressureSourceResult, *CorpusGraphManifestSource) {
	sourceRoot := filepath.Join(root, "sources", source.SourceID)
	localSourcePath := filepath.Join(sourceRoot, "source.md")
	result := CorpusPressureSourceResult{
		SourceID:    source.SourceID,
		SourceKind:  source.SourceKind,
		SourceLabel: source.Label,
		State:       CorpusPressureSourceBlocked,
		ReasonCode:  CorpusPressureReasonSemanticError,
		SourcePath:  filepath.ToSlash(filepath.Join("sources", source.SourceID, "source.md")),
	}
	if err := rejectSymlinkAncestors(sourceRoot); err != nil {
		result.Message = err.Error()
		return result, nil
	}
	if err := rejectIfSymlink(sourceRoot); err != nil {
		result.Message = err.Error()
		return result, nil
	}
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		result.Message = err.Error()
		return result, nil
	}
	if err := rejectSymlinkAncestors(localSourcePath); err != nil {
		result.Message = err.Error()
		return result, nil
	}
	if err := rejectIfSymlink(localSourcePath); err != nil {
		result.Message = err.Error()
		return result, nil
	}
	data, err := os.ReadFile(source.Path)
	if err != nil {
		result.ReasonCode = CorpusPressureReasonInputContainmentError
		result.Message = err.Error()
		return result, nil
	}
	if err := os.WriteFile(localSourcePath, data, 0o644); err != nil {
		result.Message = err.Error()
		return result, nil
	}
	summary, err := SemanticPathWithOptions(localSourcePath, sourceRoot, options)
	if err != nil {
		result.Message = err.Error()
		return result, nil
	}
	result.SemanticRunID = summary.RunID
	result.CandidateCount = summary.CandidateCount
	result.CandidateKindCounts = cloneSemanticCandidateKindCounts(summary.CandidateKindCounts)
	result.SemanticRunDir = filepath.ToSlash(filepath.Join("sources", source.SourceID))
	if summary.SkippedReason != "" {
		result.State = CorpusPressureSourceSkipped
		result.ReasonCode = CorpusPressureReasonSemanticSkipped
		result.Message = summary.SkippedReason
		return result, nil
	}
	if summary.CandidateCount == 0 {
		result.State = CorpusPressureSourceSkipped
		result.ReasonCode = CorpusPressureReasonNoSemanticCandidates
		return result, nil
	}
	result.State = CorpusPressureSourceProcessed
	result.ReasonCode = CorpusPressureReasonNone
	return result, &CorpusGraphManifestSource{
		SourceID:       source.SourceID,
		SourceKind:     source.SourceKind,
		Path:           filepath.ToSlash(filepath.Join("sources", source.SourceID, "source.md")),
		SemanticRunDir: filepath.ToSlash(filepath.Join("sources", source.SourceID)),
	}
}

func buildCorpusPressureSummary(corpusID string, sources []CorpusPressureSourceResult, graph CorpusGraphSummary, manifestPath string, graphErr error) CorpusPressureSummary {
	summary := CorpusPressureSummary{
		SchemaVersion:              CorpusPressureSummarySchemaVersion,
		CorpusID:                   corpusID,
		SourceCount:                len(sources),
		GraphAtomCount:             graph.AtomCount,
		GraphRelationCount:         graph.RelationCount,
		RelationTypeCounts:         cloneCorpusRelationTypeCounts(graph.RelationTypeCounts),
		RelationStatusCounts:       cloneReviewStatusCounts(graph.RelationStatusCounts),
		EvidenceReadyAtomCount:     graph.EvidenceReadyAtomCount,
		EvidenceReadyRelationCount: graph.EvidenceReadyRelationCount,
		ReviewBurdenCount:          graph.ReviewBurdenCount,
		ReviewBurdenRatio:          graph.ReviewBurdenRatio,
		GraphReplayFingerprint:     graph.ReplayFingerprint,
		GraphManifestPath:          filepath.ToSlash(filepath.Base(manifestPath)),
		GraphSummaryPath:           filepath.ToSlash(filepath.Join(CorpusGraphDirName, "graph-summary.json")),
		Sources:                    append([]CorpusPressureSourceResult{}, sources...),
	}
	for _, source := range sources {
		summary.SemanticCandidateCount += source.CandidateCount
		switch source.State {
		case CorpusPressureSourceProcessed:
			summary.ProcessedSourceCount++
		case CorpusPressureSourceSkipped:
			summary.SkippedSourceCount++
			summary.Blockers = append(summary.Blockers, fmt.Sprintf("source %s skipped: %s", source.SourceID, source.ReasonCode))
		case CorpusPressureSourceBlocked:
			summary.BlockedSourceCount++
			summary.Blockers = append(summary.Blockers, fmt.Sprintf("source %s blocked: %s", source.SourceID, source.ReasonCode))
		}
	}
	if graphErr != nil {
		summary.Blockers = append(summary.Blockers, "corpus graph failed: "+graphErr.Error())
	}
	summary.NextImprovementTargets = corpusPressureTargets(summary)
	summary.ReadyForFiftyFilePressure = corpusPressureReady(summary)
	summary.ReplayFingerprint = corpusPressureFingerprint(summary)
	return summary
}

func corpusPressureReady(summary CorpusPressureSummary) bool {
	if summary.SourceCount == 0 || summary.ProcessedSourceCount == 0 || summary.BlockedSourceCount > 0 {
		return false
	}
	if summary.GraphReplayFingerprint == "" || summary.GraphAtomCount == 0 {
		return false
	}
	if summary.GraphAtomCount > 0 && float64(summary.EvidenceReadyAtomCount)/float64(summary.GraphAtomCount) < 0.90 {
		return false
	}
	if summary.ReviewBurdenRatio > 0.20 {
		return false
	}
	return true
}

func corpusPressureTargets(summary CorpusPressureSummary) []string {
	targets := []string{}
	if summary.SkippedSourceCount > 0 || summary.ProcessedSourceCount < summary.SourceCount {
		targets = append(targets, "extraction_coverage")
	}
	if summary.BlockedSourceCount > 0 {
		targets = append(targets, "safety_containment")
	}
	if summary.GraphAtomCount > 0 && summary.EvidenceReadyAtomCount < summary.GraphAtomCount {
		targets = append(targets, "evidence_completeness")
	}
	if summary.ReviewBurdenRatio > 0.20 {
		targets = append(targets, "review_burden")
	}
	if summary.GraphRelationCount == 0 {
		targets = append(targets, "relation_coverage")
	}
	if len(targets) == 0 && !summary.ReadyForFiftyFilePressure {
		targets = append(targets, "readiness_thresholds")
	}
	return targets
}

func corpusPressureFingerprint(summary CorpusPressureSummary) string {
	parts := []string{
		summary.CorpusID,
		fmt.Sprintf("sources:%d:%d:%d:%d", summary.SourceCount, summary.ProcessedSourceCount, summary.SkippedSourceCount, summary.BlockedSourceCount),
		fmt.Sprintf("semantic:%d", summary.SemanticCandidateCount),
		fmt.Sprintf("graph:%d:%d:%s", summary.GraphAtomCount, summary.GraphRelationCount, summary.GraphReplayFingerprint),
	}
	for _, source := range summary.Sources {
		parts = append(parts, strings.Join([]string{source.SourceID, string(source.State), string(source.ReasonCode), fmt.Sprintf("%d", source.CandidateCount)}, ":"))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return "pressure-" + hex.EncodeToString(sum[:])[:16]
}

func cloneCorpusRelationTypeCounts(in map[CorpusRelationType]int) map[CorpusRelationType]int {
	out := map[CorpusRelationType]int{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneReviewStatusCounts(in map[ReviewStatus]int) map[ReviewStatus]int {
	out := map[ReviewStatus]int{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneSemanticCandidateKindCounts(in map[SemanticCandidateKind]int) map[SemanticCandidateKind]int {
	out := map[SemanticCandidateKind]int{}
	for key, value := range in {
		out[key] = value
	}
	return out
}
