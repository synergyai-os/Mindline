package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	CorpusPressureLoopDirName              = "corpus-pressure-loop"
	CorpusPressureLoopSummarySchemaVersion = "corpus-pressure-loop-summary/v0.1"
	DefaultCorpusPressureLoopMaxRuns       = 20
)

type CorpusPressureLoopOptions struct {
	PressureOptions  CorpusPressureOptions
	MaxRuns          int
	BuildFingerprint string
}

type CorpusPressureLoopSummary struct {
	SchemaVersion            string                        `json:"schema_version"`
	CorpusID                 string                        `json:"corpus_id"`
	MaxRuns                  int                           `json:"max_runs"`
	RunCount                 int                           `json:"run_count"`
	StopReason               string                        `json:"stop_reason"`
	KRPassed                 bool                          `json:"kr_passed"`
	BuildFingerprint         string                        `json:"build_fingerprint"`
	CommandConfigFingerprint string                        `json:"command_config_fingerprint"`
	CorpusFingerprint        string                        `json:"corpus_fingerprint"`
	Iterations               []CorpusPressureLoopIteration `json:"iterations"`
}

type CorpusPressureLoopIteration struct {
	Iteration                 int                          `json:"iteration"`
	OutDir                    string                       `json:"out_dir"`
	BuildFingerprint          string                       `json:"build_fingerprint"`
	CommandConfigFingerprint  string                       `json:"command_config_fingerprint"`
	CorpusFingerprint         string                       `json:"corpus_fingerprint"`
	PressureSummaryPath       string                       `json:"pressure_summary_path"`
	TraceSummaryPath          string                       `json:"trace_summary_path"`
	EvalInputPath             string                       `json:"eval_input_path"`
	PressureFingerprint       string                       `json:"pressure_fingerprint"`
	SourceCounters            CorpusPressureSourceCounters `json:"source_counters"`
	SourceDeltas              CorpusPressureSourceCounters `json:"source_deltas"`
	ProcessedSourceRatio      float64                      `json:"processed_source_ratio"`
	EvidenceReadyAtomRatio    float64                      `json:"evidence_ready_atom_ratio"`
	ReviewBurdenRatio         float64                      `json:"review_burden_ratio"`
	ReadyForFiftyFilePressure bool                         `json:"ready_for_50_file_pressure"`
	KRPassed                  bool                         `json:"kr_passed"`
}

func BuildCorpusPressureLoop(inputPath, outDir string, options CorpusPressureLoopOptions) (CorpusPressureLoopSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return CorpusPressureLoopSummary{}, fmt.Errorf("missing required --out")
	}
	maxRuns := options.MaxRuns
	if maxRuns <= 0 {
		maxRuns = DefaultCorpusPressureLoopMaxRuns
	}
	if maxRuns > DefaultCorpusPressureLoopMaxRuns {
		maxRuns = DefaultCorpusPressureLoopMaxRuns
	}
	if strings.TrimSpace(options.BuildFingerprint) == "" {
		options.BuildFingerprint = "build-unknown"
	}
	root, err := filepath.Abs(outDir)
	if err != nil {
		return CorpusPressureLoopSummary{}, err
	}
	if err := rejectSymlinkAncestors(root); err != nil {
		return CorpusPressureLoopSummary{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return CorpusPressureLoopSummary{}, err
	}
	configFingerprint := corpusPressureLoopConfigFingerprint(options)
	options.PressureOptions.CommandConfigFingerprint = configFingerprint
	loopSummary := CorpusPressureLoopSummary{
		SchemaVersion:            CorpusPressureLoopSummarySchemaVersion,
		MaxRuns:                  maxRuns,
		BuildFingerprint:         options.BuildFingerprint,
		CommandConfigFingerprint: configFingerprint,
	}
	var previous *CorpusPressureLoopIteration
	for i := 1; i <= maxRuns; i++ {
		iterationOut := filepath.Join(root, "iterations", fmt.Sprintf("%02d", i))
		summary, _, err := BuildCorpusPressure(inputPath, iterationOut, options.PressureOptions)
		if err != nil {
			return loopSummary, err
		}
		if loopSummary.CorpusID == "" {
			loopSummary.CorpusID = summary.CorpusID
			loopSummary.CorpusFingerprint = summary.CorpusFingerprint
		}
		iteration := corpusPressureLoopIteration(i, root, iterationOut, options.BuildFingerprint, configFingerprint, summary, previous)
		if err := writeJSON(filepath.Join(iterationOut, CorpusPressureDirName), "trace-summary.json", CorpusPressureTraceSummaryFor(summary, iteration.SourceDeltas)); err != nil {
			return loopSummary, err
		}
		loopSummary.Iterations = append(loopSummary.Iterations, iteration)
		loopSummary.RunCount = len(loopSummary.Iterations)
		replayProven := previous != nil && corpusPressureLoopStableReplay(*previous, iteration)
		loopSummary.KRPassed = iteration.KRPassed && replayProven
		if loopSummary.KRPassed {
			loopSummary.StopReason = "krs_passed"
			break
		}
		if previous != nil && corpusPressureLoopNoChange(*previous, iteration) {
			loopSummary.StopReason = "same_binary_same_inputs"
			break
		}
		previous = &loopSummary.Iterations[len(loopSummary.Iterations)-1]
	}
	if loopSummary.StopReason == "" {
		loopSummary.StopReason = fmt.Sprintf("stopped_after_%d", maxRuns)
	}
	if err := WriteCorpusPressureLoop(root, loopSummary); err != nil {
		return loopSummary, err
	}
	return loopSummary, nil
}

func corpusPressureLoopIteration(iteration int, root, iterationOut string, buildFingerprint, configFingerprint string, summary CorpusPressureSummary, previous *CorpusPressureLoopIteration) CorpusPressureLoopIteration {
	relOut, _ := filepath.Rel(root, iterationOut)
	item := CorpusPressureLoopIteration{
		Iteration:                 iteration,
		OutDir:                    filepath.ToSlash(relOut),
		BuildFingerprint:          buildFingerprint,
		CommandConfigFingerprint:  configFingerprint,
		CorpusFingerprint:         summary.CorpusFingerprint,
		PressureSummaryPath:       filepath.ToSlash(filepath.Join(relOut, CorpusPressureDirName, "pressure-summary.json")),
		TraceSummaryPath:          filepath.ToSlash(filepath.Join(relOut, CorpusPressureDirName, "trace-summary.json")),
		EvalInputPath:             filepath.ToSlash(filepath.Join(relOut, CorpusPressureDirName, "eval-input.json")),
		PressureFingerprint:       summary.ReplayFingerprint,
		SourceCounters:            corpusPressureSourceCounters(summary),
		ProcessedSourceRatio:      summary.ProcessedSourceRatio,
		EvidenceReadyAtomRatio:    summary.EvidenceReadyAtomRatio,
		ReviewBurdenRatio:         summary.ReviewBurdenRatio,
		ReadyForFiftyFilePressure: summary.ReadyForFiftyFilePressure,
		KRPassed:                  corpusPressureLoopKRPassed(summary),
	}
	if previous != nil {
		item.SourceDeltas = corpusPressureCounterDeltas(previous.SourceCounters, item.SourceCounters)
	}
	return item
}

func corpusPressureLoopKRPassed(summary CorpusPressureSummary) bool {
	return summary.SourceCount > 0 &&
		summary.ProcessedSourceCount+summary.SkippedSourceCount+summary.ExcludedSourceCount+summary.BlockedSourceCount == summary.SourceCount &&
		summary.EligibleSourceCount > 0 &&
		summary.ProcessedSourceRatio >= 0.95 &&
		summary.BlockedSourceCount == 0 &&
		summary.UnexplainedExclusionCount == 0 &&
		summary.GraphAtomCount > 0 &&
		summary.EvidenceReadyAtomRatio >= 0.90
}

func corpusPressureLoopNoChange(previous, current CorpusPressureLoopIteration) bool {
	return corpusPressureLoopStableReplay(previous, current)
}

func corpusPressureLoopStableReplay(previous, current CorpusPressureLoopIteration) bool {
	return corpusPressureLoopReplayMatches(previous, current) &&
		previous.ProcessedSourceRatio == current.ProcessedSourceRatio &&
		previous.EvidenceReadyAtomRatio == current.EvidenceReadyAtomRatio &&
		previous.ReviewBurdenRatio == current.ReviewBurdenRatio &&
		previous.SourceCounters == current.SourceCounters
}

func corpusPressureLoopReplayMatches(previous, current CorpusPressureLoopIteration) bool {
	return previous.BuildFingerprint == current.BuildFingerprint &&
		previous.CommandConfigFingerprint == current.CommandConfigFingerprint &&
		previous.CorpusFingerprint == current.CorpusFingerprint &&
		previous.PressureFingerprint == current.PressureFingerprint
}

func corpusPressureCounterDeltas(previous, current CorpusPressureSourceCounters) CorpusPressureSourceCounters {
	return CorpusPressureSourceCounters{
		Total:       current.Total - previous.Total,
		Eligible:    current.Eligible - previous.Eligible,
		Processed:   current.Processed - previous.Processed,
		Skipped:     current.Skipped - previous.Skipped,
		Excluded:    current.Excluded - previous.Excluded,
		Blocked:     current.Blocked - previous.Blocked,
		Unexplained: current.Unexplained - previous.Unexplained,
	}
}

func corpusPressureLoopConfigFingerprint(options CorpusPressureLoopOptions) string {
	parts := []string{
		fmt.Sprintf("max:%d", options.MaxRuns),
		"classifier:" + string(options.PressureOptions.SemanticOptions.Classifier),
		"provider:" + options.PressureOptions.SemanticOptions.LLMProvider,
		"model:" + options.PressureOptions.SemanticOptions.LLMModel,
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return "config-" + hex.EncodeToString(sum[:])[:16]
}
