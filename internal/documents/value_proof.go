package documents

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ValueProofSchemaVersion = "mindline-value-proof/v0.1"
const ValueProofDirName = "value-proof"

type ValueProofOptions struct {
	PressureOptions CorpusPressureOptions
}

type ValueProofSummary struct {
	SchemaVersion          string                          `json:"schema_version"`
	CorpusID               string                          `json:"corpus_id"`
	SourceCount            int                             `json:"source_count"`
	AccountedSourceCount   int                             `json:"accounted_source_count"`
	SourceAccountingRatio  float64                         `json:"source_accounting_ratio"`
	ProcessedSourceCount   int                             `json:"processed_source_count"`
	SkippedSourceCount     int                             `json:"skipped_source_count"`
	ExcludedSourceCount    int                             `json:"excluded_source_count"`
	BlockedSourceCount     int                             `json:"blocked_source_count"`
	AtomCount              int                             `json:"atom_count"`
	EvidenceReadyAtomCount int                             `json:"evidence_ready_atom_count"`
	EvidenceReadyAtomRatio float64                         `json:"evidence_ready_atom_ratio"`
	EvidenceOrBlockerCount int                             `json:"evidence_or_blocker_atom_count"`
	EvidenceOrBlockerRatio float64                         `json:"evidence_or_blocker_atom_ratio"`
	RelationCount          int                             `json:"relation_count"`
	RelationTypeCounts     map[CorpusRelationType]int      `json:"relation_type_counts"`
	Guardrails             CorpusPressureGuardrailCounters `json:"guardrails"`
	PressureSummaryPath    string                          `json:"pressure_summary_path"`
	GraphSummaryPath       string                          `json:"graph_summary_path"`
	MeaningSummaryPath     string                          `json:"meaning_summary_path"`
	LocalValuePacketPath   string                          `json:"local_value_packet_path"`
	PRSafeSummaryPath      string                          `json:"pr_safe_summary_path"`
	ClaimStatuses          ValueProofClaimStatuses         `json:"claim_statuses"`
	Blockers               []string                        `json:"blockers,omitempty"`
	NextImprovementTargets []string                        `json:"next_improvement_targets,omitempty"`
	Sources                []ValueProofSourceSummary       `json:"sources"`
}

type ValueProofClaimStatuses struct {
	Safety         string `json:"safety"`
	Improvement    string `json:"improvement"`
	Generalization string `json:"generalization"`
	DEC64          string `json:"dec64"`
}

type ValueProofSourceSummary struct {
	SourceID           string                        `json:"source_id"`
	SourceKind         string                        `json:"source_kind"`
	State              CorpusPressureSourceState     `json:"state"`
	ReasonCode         CorpusPressureReason          `json:"reason_code"`
	AtomCount          int                           `json:"atom_count"`
	RelationCount      int                           `json:"relation_count"`
	CandidateKindCount map[SemanticCandidateKind]int `json:"candidate_kind_counts,omitempty"`
	PreviewPath        string                        `json:"preview_path,omitempty"`
}

func BuildValueProof(inputPath, outDir string, options ValueProofOptions) (ValueProofSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return ValueProofSummary{}, fmt.Errorf("missing required --out")
	}
	if options.PressureOptions.SemanticOptions.Classifier == "" {
		options.PressureOptions.SemanticOptions.Classifier = SemanticClassifierDeterministic
	}
	options.PressureOptions.skipPaths = appendValueProofOutputSkipPaths(inputPath, outDir, options.PressureOptions.skipPaths)
	pressure, graph, err := BuildCorpusPressure(inputPath, outDir, options.PressureOptions)
	if err != nil {
		return ValueProofSummary{}, err
	}
	meaning, items, err := BuildSourceMeaningPreview(filepath.Join(outDir, CorpusPressureDirName), outDir)
	if err != nil {
		return ValueProofSummary{}, err
	}
	summary := buildValueProofSummary(pressure, graph, meaning)
	if err := WriteValueProof(outDir, summary, items); err != nil {
		return ValueProofSummary{}, err
	}
	return summary, nil
}

func buildValueProofSummary(pressure CorpusPressureSummary, graph CorpusGraphSummary, meaning SourceMeaningPreviewSummary) ValueProofSummary {
	accounted := pressure.ProcessedSourceCount + pressure.SkippedSourceCount + pressure.ExcludedSourceCount + pressure.BlockedSourceCount
	summary := ValueProofSummary{
		SchemaVersion:          ValueProofSchemaVersion,
		CorpusID:               pressure.CorpusID,
		SourceCount:            pressure.SourceCount,
		AccountedSourceCount:   accounted,
		SourceAccountingRatio:  valueProofRatio(accounted, pressure.SourceCount),
		ProcessedSourceCount:   pressure.ProcessedSourceCount,
		SkippedSourceCount:     pressure.SkippedSourceCount,
		ExcludedSourceCount:    pressure.ExcludedSourceCount,
		BlockedSourceCount:     pressure.BlockedSourceCount,
		AtomCount:              graph.AtomCount,
		EvidenceReadyAtomCount: graph.EvidenceReadyAtomCount,
		EvidenceReadyAtomRatio: valueProofRatio(graph.EvidenceReadyAtomCount, graph.AtomCount),
		EvidenceOrBlockerCount: valueProofEvidenceOrBlockerCount(meaning, graph.AtomCount),
		EvidenceOrBlockerRatio: meaning.EvidenceCoverageRatio,
		RelationCount:          graph.RelationCount,
		RelationTypeCounts:     cloneSourceMeaningRelationTypeCounts(graph.RelationTypeCounts),
		Guardrails:             pressure.Guardrails,
		PressureSummaryPath:    filepath.ToSlash(filepath.Join(CorpusPressureDirName, "pressure-summary.json")),
		GraphSummaryPath:       pressure.GraphSummaryPath,
		MeaningSummaryPath:     filepath.ToSlash(filepath.Join(SourceMeaningPreviewDirName, "meaning-summary.json")),
		LocalValuePacketPath:   filepath.ToSlash(filepath.Join(ValueProofDirName, "local-value-packet.md")),
		PRSafeSummaryPath:      filepath.ToSlash(filepath.Join(ValueProofDirName, "pr-safe-summary.md")),
		ClaimStatuses: ValueProofClaimStatuses{
			Safety:         "ready_for_proof_gate",
			Improvement:    "blocked_missing_comparable_baseline",
			Generalization: "blocked_missing_held_out_evidence",
			DEC64:          "blocked_missing_held_out_98_percent_no_human_proof",
		},
		Blockers:               append([]string{}, pressure.Blockers...),
		NextImprovementTargets: append([]string{}, pressure.NextImprovementTargets...),
	}
	meaningBySource := map[string]SourceMeaningPreviewItemSummary{}
	for _, item := range meaning.Items {
		meaningBySource[item.SourceID] = item
	}
	for _, source := range pressure.Sources {
		item := meaningBySource[source.SourceID]
		summary.Sources = append(summary.Sources, ValueProofSourceSummary{
			SourceID:           source.SourceID,
			SourceKind:         source.SourceKind,
			State:              source.State,
			ReasonCode:         source.ReasonCode,
			AtomCount:          item.AtomCount,
			RelationCount:      item.RelationCount,
			CandidateKindCount: cloneCandidateKindCounts(source.CandidateKindCounts),
			PreviewPath:        item.PreviewPath,
		})
	}
	return summary
}

func appendValueProofOutputSkipPaths(inputPath, outDir string, skipPaths []string) []string {
	info, err := os.Stat(inputPath)
	if err != nil || !info.IsDir() {
		return skipPaths
	}
	absInput, err := filepath.Abs(inputPath)
	if err != nil {
		return skipPaths
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return skipPaths
	}
	if !isInside(absInput, absOut) {
		return skipPaths
	}
	return append(skipPaths,
		filepath.Join(absOut, SourceMeaningPreviewDirName),
		filepath.Join(absOut, ValueProofDirName),
	)
}

func valueProofEvidenceOrBlockerCount(meaning SourceMeaningPreviewSummary, atomCount int) int {
	if meaning.EvidenceOrBlockerAtomCount > 0 || atomCount == 0 {
		return meaning.EvidenceOrBlockerAtomCount
	}
	return int(math.Round(meaning.EvidenceCoverageRatio * float64(atomCount)))
}

func valueProofRatio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func cloneCandidateKindCounts(counts map[SemanticCandidateKind]int) map[SemanticCandidateKind]int {
	if len(counts) == 0 {
		return nil
	}
	out := map[SemanticCandidateKind]int{}
	for key, value := range counts {
		out[key] = value
	}
	return out
}

func sortedValueProofSources(sources []ValueProofSourceSummary) []ValueProofSourceSummary {
	out := append([]ValueProofSourceSummary{}, sources...)
	sort.Slice(out, func(i, j int) bool { return out[i].SourceID < out[j].SourceID })
	return out
}
