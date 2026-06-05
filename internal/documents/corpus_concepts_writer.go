package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func WriteCorpusConceptIndex(outDir string, summary CorpusConceptSummary, index CorpusConceptIndex) error {
	if strings.TrimSpace(outDir) == "" {
		return ArtifactWriteError{Err: fmt.Errorf("missing required --out")}
	}
	outRoot, err := filepath.Abs(outDir)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectSymlinkAncestors(outRoot); err != nil {
		return ArtifactWriteError{Err: err}
	}
	root, err := filepath.Abs(filepath.Join(outDir, CorpusConceptsDirName))
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := rejectIfSymlink(root); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ArtifactWriteError{Err: err}
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return ArtifactWriteError{Err: err}
	}
	expected := map[string]bool{
		"concept-summary.json": true,
		"concept-index.json":   true,
		"review-packet.md":     true,
		"review-records.json":  true,
	}
	for _, concept := range index.Concepts {
		expected[CorpusConceptPath(concept.ConceptID)] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "concept-summary.json", summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "concept-index.json", index); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "review-packet.md", []byte(corpusConceptReviewMarkdown(summary))); err != nil {
		return ArtifactWriteError{Err: err}
	}
	for _, concept := range index.Concepts {
		if err := writeJSON(realRoot, CorpusConceptPath(concept.ConceptID), concept); err != nil {
			return ArtifactWriteError{Err: err}
		}
	}
	return nil
}

func CorpusConceptPath(conceptID string) string {
	return filepath.ToSlash(filepath.Join("concepts", sanitizeID(conceptID)+".json"))
}

func corpusConceptReviewMarkdown(summary CorpusConceptSummary) string {
	var b strings.Builder
	b.WriteString("# Corpus concept review packet\n\n")
	b.WriteString("> Review packet only. Concepts are destination-neutral, write-ineligible, and not accepted output.\n\n")
	b.WriteString("## Outcome\n\n")
	b.WriteString(fmt.Sprintf("- Corpus: `%s`\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Sources processed: %d of %d\n", summary.ProcessedSourceCount, summary.SourceCount))
	b.WriteString(fmt.Sprintf("- Atoms covered: %d with %.2f coverage\n", summary.AtomCount, summary.AtomCoverageRatio))
	b.WriteString(fmt.Sprintf("- Concepts: %d emitted", summary.ConceptCount))
	if summary.GeneratedConceptCount > 0 {
		b.WriteString(fmt.Sprintf(" of %d generated", summary.GeneratedConceptCount))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("- Cross-source concepts: %d\n", summary.CrossSourceConceptCount))
	b.WriteString(fmt.Sprintf("- Evidence refs: %d\n", summary.EvidenceReferenceCount))
	b.WriteString(fmt.Sprintf("- Relation review compression ratio: %.4f\n", summary.RelationReviewCompressionRatio))
	b.WriteString(fmt.Sprintf("- Concept review burden ratio: %.2f\n", summary.ConceptReviewBurdenRatio))
	b.WriteString(fmt.Sprintf("- Scale status: %s", summary.ScaleStatus))
	if len(summary.ScaleReasonCodes) > 0 {
		b.WriteString(fmt.Sprintf(" (%s)", strings.Join(summary.ScaleReasonCodes, ", ")))
	}
	b.WriteString(fmt.Sprintf(", omitted concepts=%d, omitted atoms=%d\n\n", summary.OmittedConceptCount, summary.OmittedAtomCount))
	b.WriteString("## Guardrails\n\n")
	b.WriteString(fmt.Sprintf("- Destination writes: %d\n", summary.Guardrails.DestinationWrites))
	b.WriteString(fmt.Sprintf("- Product Brain writes: %d\n", summary.Guardrails.ProductBrainWrites))
	b.WriteString(fmt.Sprintf("- Tolaria writes: %d\n", summary.Guardrails.TolariaWrites))
	b.WriteString(fmt.Sprintf("- Hosted inference calls: %d\n", summary.Guardrails.HostedInferenceCalls))
	b.WriteString(fmt.Sprintf("- Hosted telemetry exports: %d\n", summary.Guardrails.HostedTelemetryExports))
	b.WriteString("- Generalization claim: blocked\n")
	b.WriteString("- DEC-64/no-human claim: blocked\n")
	b.WriteString("- Destination-readiness claim: blocked\n\n")
	writeCorpusConceptSection(&b, "Cross Source", summary.Concepts, CorpusConceptSectionCrossSource)
	writeCorpusConceptSection(&b, "Needs Review", summary.Concepts, CorpusConceptSectionNeedsReview)
	writeCorpusConceptSection(&b, "Local", summary.Concepts, CorpusConceptSectionLocal)
	writeCorpusConceptSection(&b, "Blocked", summary.Concepts, CorpusConceptSectionBlocked)
	return b.String()
}

func writeCorpusConceptSection(b *strings.Builder, heading string, concepts []CorpusConceptListItem, section CorpusConceptSection) {
	b.WriteString("## " + heading + "\n\n")
	filtered := []CorpusConceptListItem{}
	for _, concept := range concepts {
		if concept.Section == section {
			filtered = append(filtered, concept)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ConceptID < filtered[j].ConceptID })
	if len(filtered) == 0 {
		b.WriteString("- None.\n\n")
		return
	}
	for _, concept := range filtered {
		b.WriteString(fmt.Sprintf("- `%s`: %s; atoms=%d sources=%d evidence_refs=%d evidence_previews=%d coverage=%s concept=`%s`\n", concept.ConceptID, concept.Title, concept.AtomCount, concept.SourceCount, concept.EvidenceReferenceCount, concept.RepresentativeEvidence, corpusConceptCoverageString(concept.SourceKindCoverage), concept.ConceptPath))
		if strings.TrimSpace(concept.GroupingRationale) != "" {
			b.WriteString(fmt.Sprintf("  - rationale=%s\n", concept.GroupingRationale))
		}
		if strings.TrimSpace(concept.ReviewPrompt) != "" {
			b.WriteString(fmt.Sprintf("  - prompt=%s\n", concept.ReviewPrompt))
		}
		if len(concept.ReasonCodes) > 0 {
			b.WriteString(fmt.Sprintf("  - reasons=%s\n", strings.Join(concept.ReasonCodes, ",")))
		}
	}
	b.WriteString("\n")
}

func corpusConceptCoverageString(coverage map[string]int) string {
	keys := make([]string, 0, len(coverage))
	for key := range coverage {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", key, coverage[key]))
	}
	return strings.Join(parts, ",")
}
