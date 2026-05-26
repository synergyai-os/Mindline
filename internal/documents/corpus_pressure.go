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
	CorpusPressureManifestSchemaVersion  = "corpus-pressure-manifest/v0.1"
	CorpusPressureSummarySchemaVersion   = "corpus-pressure-summary/v0.1"
	CorpusPressureEvalInputSchemaVersion = "corpus-pressure-eval-input/v0.1"
	CorpusPressureTraceSchemaVersion     = "corpus-pressure-trace-summary/v0.1"
	corpusPressureGeneratedSourceMarker  = ".corpus-pressure-source"
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
	SemanticOptions          SemanticOptions
	CommandConfigFingerprint string
	skipPaths                []string
}

type CorpusPressureSourceState string

const (
	CorpusPressureSourceProcessed CorpusPressureSourceState = "processed"
	CorpusPressureSourceSkipped   CorpusPressureSourceState = "skipped"
	CorpusPressureSourceExcluded  CorpusPressureSourceState = "excluded"
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
	SchemaVersion              string                          `json:"schema_version"`
	CorpusID                   string                          `json:"corpus_id"`
	SourceCount                int                             `json:"source_count"`
	EligibleSourceCount        int                             `json:"eligible_source_count"`
	ProcessedSourceCount       int                             `json:"processed_source_count"`
	SkippedSourceCount         int                             `json:"skipped_source_count"`
	ExcludedSourceCount        int                             `json:"excluded_source_count"`
	BlockedSourceCount         int                             `json:"blocked_source_count"`
	UnexplainedExclusionCount  int                             `json:"unexplained_exclusion_count"`
	ProcessedSourceRatio       float64                         `json:"processed_source_ratio"`
	SemanticCandidateCount     int                             `json:"semantic_candidate_count"`
	GraphAtomCount             int                             `json:"graph_atom_count"`
	GraphRelationCount         int                             `json:"graph_relation_count"`
	RelationTypeCounts         map[CorpusRelationType]int      `json:"relation_type_counts"`
	RelationStatusCounts       map[ReviewStatus]int            `json:"relation_status_counts"`
	EvidenceReadyAtomCount     int                             `json:"evidence_ready_atom_count"`
	EvidenceReadyAtomRatio     float64                         `json:"evidence_ready_atom_ratio"`
	EvidenceReadyRelationCount int                             `json:"evidence_ready_relation_count"`
	ReviewBurdenCount          int                             `json:"review_burden_count"`
	ReviewBurdenRatio          float64                         `json:"review_burden_ratio"`
	ReadyForFiftyFilePressure  bool                            `json:"ready_for_50_file_pressure"`
	ReplayFingerprint          string                          `json:"replay_fingerprint"`
	GraphReplayFingerprint     string                          `json:"graph_replay_fingerprint"`
	CommandConfigFingerprint   string                          `json:"command_config_fingerprint"`
	CorpusFingerprint          string                          `json:"corpus_fingerprint"`
	Guardrails                 CorpusPressureGuardrailCounters `json:"guardrails"`
	GraphManifestPath          string                          `json:"graph_manifest_path"`
	GraphSummaryPath           string                          `json:"graph_summary_path"`
	Blockers                   []string                        `json:"blockers"`
	NextImprovementTargets     []string                        `json:"next_improvement_targets"`
	Sources                    []CorpusPressureSourceResult    `json:"sources"`
}

type CorpusPressureSourceCounters struct {
	Total       int `json:"total"`
	Eligible    int `json:"eligible"`
	Processed   int `json:"processed"`
	Skipped     int `json:"skipped"`
	Excluded    int `json:"excluded"`
	Blocked     int `json:"blocked"`
	Unexplained int `json:"unexplained"`
}

type CorpusPressureGuardrailCounters struct {
	HostedInferenceCalls   int `json:"hosted_inference_calls"`
	HostedTelemetryExports int `json:"hosted_telemetry_exports"`
	DestinationWrites      int `json:"destination_writes"`
}

type CorpusPressureEvalInput struct {
	SchemaVersion             string                          `json:"schema_version"`
	CorpusID                  string                          `json:"corpus_id"`
	CommandConfigFingerprint  string                          `json:"command_config_fingerprint"`
	CorpusFingerprint         string                          `json:"corpus_fingerprint"`
	PressureSummaryPath       string                          `json:"pressure_summary_path"`
	GraphSummaryPath          string                          `json:"graph_summary_path"`
	SourceCounters            CorpusPressureSourceCounters    `json:"source_counters"`
	ProcessedSourceRatio      float64                         `json:"processed_source_ratio"`
	EvidenceReadyAtomRatio    float64                         `json:"evidence_ready_atom_ratio"`
	ReviewBurdenRatio         float64                         `json:"review_burden_ratio"`
	ReadyForFiftyFilePressure bool                            `json:"ready_for_50_file_pressure"`
	Guardrails                CorpusPressureGuardrailCounters `json:"guardrails"`
	NextImprovementTargets    []string                        `json:"next_improvement_targets"`
}

type CorpusPressureTraceSummary struct {
	SchemaVersion            string                          `json:"schema_version"`
	CorpusID                 string                          `json:"corpus_id"`
	Stages                   []CorpusPressureTraceStage      `json:"stages"`
	SourceCounters           CorpusPressureSourceCounters    `json:"source_counters"`
	SourceDeltas             CorpusPressureSourceCounters    `json:"source_deltas"`
	ProcessedSourceRatio     float64                         `json:"processed_source_ratio"`
	EvidenceReadyAtomRatio   float64                         `json:"evidence_ready_atom_ratio"`
	CommandConfigFingerprint string                          `json:"command_config_fingerprint"`
	CorpusFingerprint        string                          `json:"corpus_fingerprint"`
	PressureFingerprint      string                          `json:"pressure_fingerprint"`
	GraphReplayFingerprint   string                          `json:"graph_replay_fingerprint"`
	Guardrails               CorpusPressureGuardrailCounters `json:"guardrails"`
	ArtifactPaths            map[string]string               `json:"artifact_paths"`
}

type CorpusPressureTraceStage struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Count  int    `json:"count,omitempty"`
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
	SourceContentHash   string                        `json:"source_content_hash,omitempty"`
	SourcePath          string                        `json:"source_path"`
	SemanticRunDir      string                        `json:"semantic_run_dir,omitempty"`
}

type corpusPressureSourceInput struct {
	SourceID   string
	SourceKind string
	Path       string
	Label      string
	RunDir     string
}

type corpusPressureCountingLLMProvider struct {
	inner LLMSemanticProvider
	calls *int
}

func (provider corpusPressureCountingLLMProvider) Classify(request LLMSemanticRequest) (llmSemanticResponse, error) {
	if provider.calls != nil {
		(*provider.calls)++
	}
	return provider.inner.Classify(request)
}

func BuildCorpusPressure(inputPath, outDir string, options CorpusPressureOptions) (CorpusPressureSummary, CorpusGraphSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return CorpusPressureSummary{}, CorpusGraphSummary{}, fmt.Errorf("missing required --out")
	}
	if options.SemanticOptions.Classifier == "" {
		options.SemanticOptions.Classifier = SemanticClassifierDeterministic
	}
	options.SemanticOptions.ReferenceFallback = true
	if strings.TrimSpace(options.CommandConfigFingerprint) == "" {
		options.CommandConfigFingerprint = corpusPressureCommandConfigFingerprint(options.SemanticOptions)
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
	skipPaths := append([]string{}, options.skipPaths...)
	skipPaths = append(skipPaths, corpusPressureOutputSkipPaths(inputPath, root)...)
	sources, corpusID, err := loadCorpusPressureSources(inputPath, skipPaths...)
	if err != nil {
		return CorpusPressureSummary{}, CorpusGraphSummary{}, err
	}
	guardrails := CorpusPressureGuardrailCounters{}
	if options.SemanticOptions.Classifier == SemanticClassifierLLM {
		provider, err := semanticLLMProvider(options.SemanticOptions)
		if err != nil {
			return CorpusPressureSummary{}, CorpusGraphSummary{}, err
		}
		options.SemanticOptions.LLMClient = corpusPressureCountingLLMProvider{
			inner: provider,
			calls: &guardrails.HostedInferenceCalls,
		}
	}
	sources = assignCorpusPressureSourceRunDirs(root, sources)
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
	summary.CommandConfigFingerprint = options.CommandConfigFingerprint
	summary.CorpusFingerprint = corpusPressureSourceFingerprint(results)
	summary.Guardrails = guardrails
	summary.ReplayFingerprint = corpusPressureFingerprint(summary)
	if err := WriteCorpusPressure(root, summary, graphSummary); err != nil {
		return CorpusPressureSummary{}, CorpusGraphSummary{}, err
	}
	if graphErr != nil {
		return summary, graphSummary, graphErr
	}
	return summary, graphSummary, nil
}

func loadCorpusPressureSources(inputPath string, skipPaths ...string) ([]corpusPressureSourceInput, string, error) {
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
	canonicalInput := canonicalCorpusInputPath(absInput)
	paths, err := markdownPaths(inputPath)
	if err != nil {
		return nil, "", err
	}
	sort.Strings(paths)
	seen := map[string]int{}
	out := make([]corpusPressureSourceInput, 0, len(paths))
	for _, path := range paths {
		if shouldSkipCorpusPressurePath(inputPath, path, skipPaths...) {
			continue
		}
		safePath, err := containedDiscoveredSourcePath(absInput, path)
		if err != nil {
			return nil, "", err
		}
		rel, err := filepath.Rel(canonicalInput, safePath)
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
	return out, "corpus-" + shortHash(filepath.ToSlash(canonicalInput)), nil
}

func canonicalCorpusInputPath(absInput string) string {
	realInput, err := filepath.EvalSymlinks(absInput)
	if err != nil {
		return filepath.Clean(absInput)
	}
	return filepath.Clean(realInput)
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

func shouldSkipCorpusPressurePath(root, path string, skipPaths ...string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	if corpusPressureGeneratedSourceCopy(path) {
		return true
	}
	for _, skipPath := range skipPaths {
		if strings.TrimSpace(skipPath) == "" {
			continue
		}
		absSkipPath, err := filepath.Abs(skipPath)
		if err != nil {
			return true
		}
		if isInside(absSkipPath, absPath) {
			return true
		}
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

func corpusPressureOutputSkipPaths(inputPath, outRoot string) []string {
	info, err := os.Stat(inputPath)
	if err != nil || !info.IsDir() {
		return nil
	}
	absInput, err := filepath.Abs(inputPath)
	if err != nil {
		return nil
	}
	if !isInside(absInput, outRoot) {
		return nil
	}
	rel, err := filepath.Rel(absInput, outRoot)
	if err != nil {
		return nil
	}
	if rel == "." {
		return nil
	}
	return []string{outRoot}
}

func corpusPressureGeneratedSourceCopy(path string) bool {
	if filepath.Base(path) != "source.md" {
		return false
	}
	sourceRoot := filepath.Dir(path)
	info, err := os.Stat(filepath.Join(sourceRoot, corpusPressureGeneratedSourceMarker))
	return err == nil && !info.IsDir()
}

func assignCorpusPressureSourceRunDirs(root string, sources []corpusPressureSourceInput) []corpusPressureSourceInput {
	inputPaths := map[string]bool{}
	for _, source := range sources {
		if path, ok := canonicalExistingPath(source.Path); ok {
			inputPaths[path] = true
		}
	}
	usedRunDirs := map[string]bool{}
	for i := range sources {
		baseID := sources[i].SourceID
		for attempt := 0; ; attempt++ {
			runID := corpusPressureRunDirID(baseID, attempt)
			runDir := filepath.ToSlash(filepath.Join("sources", runID))
			if usedRunDirs[runDir] {
				continue
			}
			localSourcePath := filepath.Join(root, filepath.FromSlash(runDir), "source.md")
			if corpusPressureSourceRunDirCollides(localSourcePath, inputPaths) {
				continue
			}
			sources[i].RunDir = runDir
			usedRunDirs[runDir] = true
			break
		}
	}
	return sources
}

func corpusPressureRunDirID(baseID string, attempt int) string {
	if attempt == 0 {
		return baseID
	}
	if attempt == 1 {
		return baseID + "-pressure"
	}
	return fmt.Sprintf("%s-pressure-%d", baseID, attempt)
}

func corpusPressureSourceRunDirCollides(localSourcePath string, inputPaths map[string]bool) bool {
	if path, ok := canonicalExistingPath(localSourcePath); ok {
		if inputPaths[path] {
			return true
		}
	}
	sourceRoot := filepath.Dir(localSourcePath)
	sourceRootInfo, err := os.Stat(sourceRoot)
	if err == nil {
		if !sourceRootInfo.IsDir() {
			return true
		}
		markerInfo, markerErr := os.Stat(filepath.Join(sourceRoot, corpusPressureGeneratedSourceMarker))
		if markerErr != nil || markerInfo.IsDir() {
			return true
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return true
	}
	info, err := os.Stat(localSourcePath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	return !info.IsDir() && !corpusPressureGeneratedSourceCopy(localSourcePath)
}

func canonicalExistingPath(path string) (string, bool) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return filepath.Clean(absPath), false
	}
	return filepath.Clean(realPath), true
}

func runCorpusPressureSource(root string, source corpusPressureSourceInput, options SemanticOptions) (CorpusPressureSourceResult, *CorpusGraphManifestSource) {
	runDir := source.RunDir
	if runDir == "" {
		runDir = filepath.ToSlash(filepath.Join("sources", source.SourceID))
	}
	sourceRoot := filepath.Join(root, filepath.FromSlash(runDir))
	localSourcePath := filepath.Join(sourceRoot, "source.md")
	relSourcePath := filepath.ToSlash(filepath.Join(runDir, "source.md"))
	result := CorpusPressureSourceResult{
		SourceID:    source.SourceID,
		SourceKind:  source.SourceKind,
		SourceLabel: source.Label,
		State:       CorpusPressureSourceBlocked,
		ReasonCode:  CorpusPressureReasonSemanticError,
		SourcePath:  relSourcePath,
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
	if err := os.WriteFile(filepath.Join(sourceRoot, corpusPressureGeneratedSourceMarker), []byte("generated by corpus-pressure\n"), 0o644); err != nil {
		result.Message = err.Error()
		return result, nil
	}
	result.SourceContentHash = "sha256:" + contentHash(string(data))
	summary, err := SemanticPathWithOptions(localSourcePath, sourceRoot, options)
	if err != nil {
		result.Message = err.Error()
		return result, nil
	}
	if summary.SkippedReason == "" {
		summary = promoteCorpusPressureEvidenceReadiness(sourceRoot, summary)
	}
	result.SemanticRunID = summary.RunID
	result.CandidateCount = summary.CandidateCount
	result.CandidateKindCounts = cloneSemanticCandidateKindCounts(summary.CandidateKindCounts)
	result.SemanticRunDir = runDir
	if summary.SkippedReason != "" {
		if strings.Contains(summary.SkippedReason, "all structure nodes are blocked") {
			result.State = CorpusPressureSourceExcluded
		} else {
			result.State = CorpusPressureSourceSkipped
		}
		result.ReasonCode = CorpusPressureReasonSemanticSkipped
		result.Message = summary.SkippedReason
		return result, nil
	}
	if summary.CandidateCount == 0 {
		if corpusPressureAllStructureNodesBlocked(sourceRoot) {
			result.State = CorpusPressureSourceExcluded
			result.ReasonCode = CorpusPressureReasonSemanticSkipped
			result.Message = "all structure nodes are blocked; source excluded from eligible pressure denominator"
			return result, nil
		}
		result.State = CorpusPressureSourceSkipped
		result.ReasonCode = CorpusPressureReasonNoSemanticCandidates
		return result, nil
	}
	result.State = CorpusPressureSourceProcessed
	result.ReasonCode = CorpusPressureReasonNone
	return result, &CorpusGraphManifestSource{
		SourceID:       source.SourceID,
		SourceKind:     source.SourceKind,
		Path:           relSourcePath,
		SemanticRunDir: runDir,
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
		case CorpusPressureSourceExcluded:
			summary.ExcludedSourceCount++
			if source.ReasonCode == CorpusPressureReasonNone {
				summary.UnexplainedExclusionCount++
			}
			summary.Blockers = append(summary.Blockers, fmt.Sprintf("source %s excluded: %s", source.SourceID, source.ReasonCode))
		case CorpusPressureSourceBlocked:
			summary.BlockedSourceCount++
			summary.Blockers = append(summary.Blockers, fmt.Sprintf("source %s blocked: %s", source.SourceID, source.ReasonCode))
		}
	}
	summary.EligibleSourceCount = summary.SourceCount - summary.ExcludedSourceCount
	if summary.EligibleSourceCount > 0 {
		summary.ProcessedSourceRatio = float64(summary.ProcessedSourceCount) / float64(summary.EligibleSourceCount)
	}
	if summary.GraphAtomCount > 0 {
		summary.EvidenceReadyAtomRatio = float64(summary.EvidenceReadyAtomCount) / float64(summary.GraphAtomCount)
	}
	if graphErr != nil {
		summary.Blockers = append(summary.Blockers, "corpus graph failed: "+graphErr.Error())
	}
	summary.ReadyForFiftyFilePressure = graphErr == nil && corpusPressureReady(summary)
	summary.NextImprovementTargets = corpusPressureTargets(summary)
	return summary
}

func corpusPressureReady(summary CorpusPressureSummary) bool {
	if summary.SourceCount == 0 || summary.ProcessedSourceCount == 0 || summary.BlockedSourceCount > 0 || summary.UnexplainedExclusionCount > 0 {
		return false
	}
	if summary.ProcessedSourceRatio < 0.95 {
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
	if summary.ReadyForFiftyFilePressure {
		return nil
	}
	targets := []string{}
	if summary.ProcessedSourceRatio < 0.95 {
		targets = append(targets, "extraction_coverage")
	}
	if summary.SkippedSourceCount > 0 || summary.ExcludedSourceCount > 0 {
		targets = append(targets, "source_state_reduction")
	}
	if summary.BlockedSourceCount > 0 {
		targets = append(targets, "safety_containment")
	}
	if summary.UnexplainedExclusionCount > 0 {
		targets = append(targets, "exclusion_explanation")
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
		"config:" + summary.CommandConfigFingerprint,
		"corpus:" + summary.CorpusFingerprint,
		fmt.Sprintf("graph:%d:%d:%s", summary.GraphAtomCount, summary.GraphRelationCount, summary.GraphReplayFingerprint),
	}
	for _, source := range summary.Sources {
		parts = append(parts, strings.Join([]string{source.SourceID, string(source.State), string(source.ReasonCode), fmt.Sprintf("%d", source.CandidateCount)}, ":"))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return "pressure-" + hex.EncodeToString(sum[:])[:16]
}

func corpusPressureCommandConfigFingerprint(options SemanticOptions) string {
	options = normalizeCorpusPressureSemanticOptions(options)
	parts := []string{
		"classifier:" + string(options.Classifier),
		fmt.Sprintf("reference_fallback:%t", options.ReferenceFallback),
	}
	if options.Classifier == SemanticClassifierLLM {
		parts = append(parts, "provider:"+options.LLMProvider, "model:"+options.LLMModel)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return "config-" + hex.EncodeToString(sum[:])[:16]
}

func normalizeCorpusPressureSemanticOptions(options SemanticOptions) SemanticOptions {
	if options.Classifier == "" {
		options.Classifier = SemanticClassifierDeterministic
	}
	options.ReferenceFallback = true
	if options.Classifier != SemanticClassifierLLM {
		options.LLMProvider = ""
		options.LLMModel = ""
	}
	return options
}

func corpusPressureSourceFingerprint(sources []CorpusPressureSourceResult) string {
	parts := make([]string, 0, len(sources))
	for _, source := range sources {
		parts = append(parts, strings.Join([]string{source.SourceID, source.SourceKind, source.SourceLabel, source.SourcePath, source.SourceContentHash}, ":"))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return "corpus-" + hex.EncodeToString(sum[:])[:16]
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

func promoteCorpusPressureEvidenceReadiness(sourceRoot string, summary SemanticSummary) SemanticSummary {
	root := filepath.Join(sourceRoot, "semantic-candidates")
	changed := false
	candidates := make([]SemanticCandidate, 0, len(summary.Candidates))
	for _, item := range summary.Candidates {
		path := filepath.Join(root, item.CandidatePath)
		data, err := os.ReadFile(path)
		if err != nil {
			return summary
		}
		var candidate SemanticCandidate
		if err := json.Unmarshal(data, &candidate); err != nil {
			return summary
		}
		if candidate.ReviewStatus == ReviewStatusNeedsReview && len(candidate.EvidenceNodes) > 0 && len(candidate.EvidenceRanges) > 0 && !hasBlockerCode(candidate.Blockers, "semantic_review_required") {
			candidate.ReviewStatus = ReviewStatusReady
			candidate.Confidence = ConfidenceMedium
			candidate.Blockers = []Blocker{}
			if err := writeJSON(root, item.CandidatePath, candidate); err != nil {
				return summary
			}
			changed = true
		}
		candidates = append(candidates, candidate)
	}
	if !changed {
		return summary
	}
	for _, candidate := range candidates {
		for _, relationID := range candidate.RelationIDs {
			path := filepath.Join(root, SemanticRelationJSONPath(relationID))
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var relation SemanticRelation
			if err := json.Unmarshal(data, &relation); err != nil {
				continue
			}
			if relation.ReviewStatus == ReviewStatusNeedsReview && len(relation.EvidenceNodes) > 0 && !hasBlockerCode(relation.Blockers, "semantic_review_required") {
				relation.ReviewStatus = ReviewStatusReady
				relation.Confidence = ConfidenceMedium
				relation.Blockers = []Blocker{}
				_ = writeJSON(root, SemanticRelationJSONPath(relationID), relation)
			}
		}
	}
	var observations []SemanticObservation
	for _, item := range summary.Candidates {
		for _, observationID := range candidatesByID(candidates)[item.CandidateID].ObservationIDs {
			path := filepath.Join(root, SemanticObservationJSONPath(observationID))
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var observation SemanticObservation
			if err := json.Unmarshal(data, &observation); err == nil {
				observations = append(observations, observation)
			}
		}
	}
	var relations []SemanticRelation
	for _, candidate := range candidates {
		for _, relationID := range candidate.RelationIDs {
			path := filepath.Join(root, SemanticRelationJSONPath(relationID))
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var relation SemanticRelation
			if err := json.Unmarshal(data, &relation); err == nil {
				relations = append(relations, relation)
			}
		}
	}
	promoted := BuildSemanticSummary(summary.RunID, summary.SourceCount, observations, candidates, relations)
	_ = writeJSON(root, "semantic-summary.json", promoted)
	return promoted
}

func hasBlockerCode(blockers []Blocker, code string) bool {
	for _, blocker := range blockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}

func candidatesByID(candidates []SemanticCandidate) map[string]SemanticCandidate {
	out := map[string]SemanticCandidate{}
	for _, candidate := range candidates {
		out[candidate.CandidateID] = candidate
	}
	return out
}

func corpusPressureAllStructureNodesBlocked(sourceRoot string) bool {
	data, err := os.ReadFile(filepath.Join(sourceRoot, "document-structure", "structure-summary.json"))
	if err != nil {
		return false
	}
	var summary StructureSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return false
	}
	return summary.NodeCount > 0 && summary.BlockedCount == summary.NodeCount
}
