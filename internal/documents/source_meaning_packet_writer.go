package documents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const SourceMeaningPacketDirName = "source-meaning-packet"

func WriteSourceMeaningPacket(outDir string, summary SourceMeaningPacketSummary, groups []SourceMeaningPacketGroup, proposals []SourceMeaningPacketProposal, evidenceMap SourceMeaningPacketEvidenceMap, blockedItems SourceMeaningPacketBlockedItems) error {
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
	root, err := filepath.Abs(filepath.Join(outDir, SourceMeaningPacketDirName))
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
		"meaning-summary.json": true,
		"review-packet.md":     true,
		"evidence-map.json":    true,
		"blocked-items.json":   true,
	}
	for _, group := range groups {
		expected[SourceMeaningPacketGroupPath(group.GroupID)] = true
	}
	for _, proposal := range proposals {
		expected[SourceMeaningPacketProposalPath(proposal.ProposalID)] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expected); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "meaning-summary.json", summary); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "evidence-map.json", evidenceMap); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeJSON(realRoot, "blocked-items.json", blockedItems); err != nil {
		return ArtifactWriteError{Err: err}
	}
	if err := writeFile(realRoot, "review-packet.md", []byte(sourceMeaningPacketMarkdown(summary))); err != nil {
		return ArtifactWriteError{Err: err}
	}
	for _, group := range groups {
		if err := writeJSON(realRoot, SourceMeaningPacketGroupPath(group.GroupID), group); err != nil {
			return ArtifactWriteError{Err: err}
		}
	}
	for _, proposal := range proposals {
		if err := writeJSON(realRoot, SourceMeaningPacketProposalPath(proposal.ProposalID), proposal); err != nil {
			return ArtifactWriteError{Err: err}
		}
	}
	return nil
}

func SourceMeaningPacketGroupPath(groupID string) string {
	return filepath.ToSlash(filepath.Join("groups", sanitizeID(groupID)+".json"))
}

func SourceMeaningPacketProposalPath(proposalID string) string {
	return filepath.ToSlash(filepath.Join("proposals", sanitizeID(proposalID)+".json"))
}

func sourceMeaningPacketMarkdown(summary SourceMeaningPacketSummary) string {
	var b strings.Builder
	b.WriteString("# Source meaning review packet\n\n")
	b.WriteString("> Review packet only. Groups are destination-neutral, write-ineligible, and not accepted output.\n\n")
	b.WriteString("## Outcome\n\n")
	b.WriteString(fmt.Sprintf("- Corpus: `%s`\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Sources processed: %d of %d\n", summary.ProcessedSourceCount, summary.SourceCount))
	b.WriteString(fmt.Sprintf("- Atoms grouped: %d into %d review groups\n", summary.AtomCount, summary.ReviewGroupCount))
	b.WriteString(fmt.Sprintf("- Relations compressed: %d graph relations into %d review groups\n", summary.RelationCount, summary.ReviewGroupCount))
	b.WriteString(fmt.Sprintf("- Atom compression ratio: %.2f\n", summary.AtomCompressionRatio))
	b.WriteString(fmt.Sprintf("- Relation review compression ratio: %.2f\n", summary.RelationReviewCompressionRatio))
	b.WriteString(fmt.Sprintf("- Evidence/blocker group ratio: %.2f\n", summary.EvidenceOrBlockerGroupRatio))
	b.WriteString(fmt.Sprintf("- Review burden ratio: %.2f\n\n", summary.ReviewBurdenRatio))

	b.WriteString("## Guardrails\n\n")
	b.WriteString(fmt.Sprintf("- Destination writes: %d\n", summary.Guardrails.DestinationWrites))
	b.WriteString(fmt.Sprintf("- Product Brain writes: %d\n", summary.Guardrails.ProductBrainWrites))
	b.WriteString(fmt.Sprintf("- Tolaria writes: %d\n", summary.Guardrails.TolariaWrites))
	b.WriteString(fmt.Sprintf("- Hosted inference calls: %d\n", summary.Guardrails.HostedInferenceCalls))
	b.WriteString(fmt.Sprintf("- Hosted telemetry exports: %d\n", summary.Guardrails.HostedTelemetryExports))
	b.WriteString("- Generalization claim: blocked\n")
	b.WriteString("- DEC-64/no-human claim: blocked\n")
	b.WriteString("- Destination-readiness claim: blocked\n\n")

	writeSourceMeaningPacketSection(&b, "Ready", summary.Groups, SourceMeaningPacketSectionReady)
	writeSourceMeaningPacketSection(&b, "Needs Review", summary.Groups, SourceMeaningPacketSectionNeedsReview)
	writeSourceMeaningPacketSection(&b, "Blocked", summary.Groups, SourceMeaningPacketSectionBlocked)
	return b.String()
}

func writeSourceMeaningPacketSection(b *strings.Builder, heading string, groups []SourceMeaningPacketGroupSummary, section SourceMeaningPacketSection) {
	b.WriteString("## " + heading + "\n\n")
	filtered := []SourceMeaningPacketGroupSummary{}
	for _, group := range groups {
		if group.Section == section {
			filtered = append(filtered, group)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].GroupID < filtered[j].GroupID })
	if len(filtered) == 0 {
		b.WriteString("- None.\n\n")
		return
	}
	for _, group := range filtered {
		b.WriteString(fmt.Sprintf("- `%s`: atoms=%d sources=%d evidence_refs=%d duplicate_pressure=%d group=`%s` proposal=`%s`\n", group.GroupID, group.AtomCount, group.SourceCount, group.EvidenceReferenceCount, group.DuplicatePressureCount, group.GroupPath, group.ProposalPath))
		if len(group.BlockerReasons) > 0 {
			b.WriteString(fmt.Sprintf("  - blockers=%s\n", strings.Join(group.BlockerReasons, ",")))
		}
	}
	b.WriteString("\n")
}
