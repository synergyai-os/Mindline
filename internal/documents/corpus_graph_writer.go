package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const CorpusGraphDirName = "corpus-graph"

func WriteCorpusGraph(outDir string, summary CorpusGraphSummary, atoms []CorpusGraphAtom, relations []CorpusGraphRelation, reviews []CorpusGraphReviewItem) error {
	if err := writeCorpusGraph(outDir, summary, atoms, relations, reviews); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func writeCorpusGraph(outDir string, summary CorpusGraphSummary, atoms []CorpusGraphAtom, relations []CorpusGraphRelation, reviews []CorpusGraphReviewItem) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("missing required --out")
	}
	root, err := filepath.Abs(filepath.Join(outDir, CorpusGraphDirName))
	if err != nil {
		return err
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return err
	}
	if err := rejectIfSymlink(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	expected := map[string]bool{"graph-summary.json": true, "graph-report.md": true}
	for _, atom := range atoms {
		expected[CorpusAtomJSONPath(atom.AtomID)] = true
	}
	for _, relation := range relations {
		expected[CorpusRelationJSONPath(relation.RelationID)] = true
	}
	for _, review := range reviews {
		expected[CorpusReviewJSONPath(review.RelationID)] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return err
	}
	if err := writeJSON(realRoot, "graph-summary.json", summary); err != nil {
		return err
	}
	for _, atom := range atoms {
		if err := writeJSON(realRoot, CorpusAtomJSONPath(atom.AtomID), atom); err != nil {
			return err
		}
	}
	for _, relation := range relations {
		if err := writeJSON(realRoot, CorpusRelationJSONPath(relation.RelationID), relation); err != nil {
			return err
		}
	}
	for _, review := range reviews {
		if err := writeJSON(realRoot, CorpusReviewJSONPath(review.RelationID), review); err != nil {
			return err
		}
	}
	return writeFile(realRoot, "graph-report.md", []byte(corpusGraphMarkdown(summary)))
}

func CorpusAtomJSONPath(atomID string) string {
	return filepath.ToSlash(filepath.Join("atoms", sanitizeID(atomID)+".json"))
}

func CorpusRelationJSONPath(relationID string) string {
	return filepath.ToSlash(filepath.Join("relations", sanitizeID(relationID)+".json"))
}

func CorpusReviewJSONPath(relationID string) string {
	return filepath.ToSlash(filepath.Join("review-items", sanitizeID(relationID)+".json"))
}

func corpusGraphMarkdown(summary CorpusGraphSummary) string {
	var b strings.Builder
	b.WriteString("# Corpus graph report\n\n")
	b.WriteString(fmt.Sprintf("- Corpus: %s\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Sources: %d\n", summary.SourceCount))
	b.WriteString(fmt.Sprintf("- Semantic runs: %d\n", summary.SemanticRunCount))
	b.WriteString(fmt.Sprintf("- Skipped sources: %d\n", summary.SkippedSourceCount))
	b.WriteString(fmt.Sprintf("- Atoms: %d\n", summary.AtomCount))
	b.WriteString(fmt.Sprintf("- Relations: %d\n", summary.RelationCount))
	b.WriteString(fmt.Sprintf("- Evidence-ready atoms: %d\n", summary.EvidenceReadyAtomCount))
	b.WriteString(fmt.Sprintf("- Evidence-ready relations: %d\n", summary.EvidenceReadyRelationCount))
	b.WriteString(fmt.Sprintf("- Review burden: %d (%.2f)\n", summary.ReviewBurdenCount, summary.ReviewBurdenRatio))
	b.WriteString(fmt.Sprintf("- Replay fingerprint: %s\n", summary.ReplayFingerprint))
	b.WriteString(fmt.Sprintf("- Ready for 50-file pressure: %t\n\n", summary.ReadyForFiftyFilePressure))
	b.WriteString("## Relation metrics\n\n")
	b.WriteString(fmt.Sprintf("- Eval-counted: %d\n", summary.RelationMetrics.EvalCountedRelationCount))
	b.WriteString(fmt.Sprintf("- True positives: %d\n", summary.RelationMetrics.TruePositiveCount))
	b.WriteString(fmt.Sprintf("- False positives: %d\n", summary.RelationMetrics.FalsePositiveCount))
	b.WriteString(fmt.Sprintf("- False negatives: %d\n", summary.RelationMetrics.FalseNegativeCount))
	b.WriteString(fmt.Sprintf("- Precision: %.4f\n", summary.RelationMetrics.Precision))
	b.WriteString(fmt.Sprintf("- Recall: %.4f\n\n", summary.RelationMetrics.Recall))
	if len(summary.Blockers) > 0 {
		b.WriteString("## Blockers\n\n")
		for _, blocker := range summary.Blockers {
			b.WriteString("- " + blocker + "\n")
		}
	}
	return b.String()
}
