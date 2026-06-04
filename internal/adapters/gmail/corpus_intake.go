package gmail

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/synergyai-os/Mindline/internal/corpusintake"
	"github.com/synergyai-os/Mindline/internal/sbos"
)

const CorpusIntakeDirName = "gmail-corpus-intake"

type CorpusIntakeFileSystem = corpusintake.FileSystem

func BuildCorpusIntake(payload Payload, outDir string) (CorpusIntakeSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return CorpusIntakeSummary{}, fmt.Errorf("missing required --out")
	}
	root, err := filepath.Abs(outDir)
	if err != nil {
		return CorpusIntakeSummary{}, err
	}
	return buildCorpusIntake(payload, root, corpusintake.OSFileSystem{})
}

func BuildCorpusIntakeWithFileSystem(payload Payload, outDir string, fileSystem CorpusIntakeFileSystem) (CorpusIntakeSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return CorpusIntakeSummary{}, fmt.Errorf("missing required --out")
	}
	return buildCorpusIntake(payload, filepath.Clean(outDir), fileSystem)
}

func buildCorpusIntake(payload Payload, root string, fileSystem CorpusIntakeFileSystem) (CorpusIntakeSummary, error) {
	if err := corpusintake.PrepareRoot(fileSystem, root); err != nil {
		return CorpusIntakeSummary{}, err
	}
	result, err := Normalize(payload)
	if err != nil {
		return CorpusIntakeSummary{}, err
	}
	summary := CorpusIntakeSummary{
		SchemaVersion:      CorpusIntakeSummarySchemaVersion,
		AdapterID:          result.AdapterID,
		CorpusID:           corpusID(payload.Source),
		Source:             sourceID(payload.Source),
		Mailbox:            mailbox(payload.Source),
		BatchOrder:         "old_to_new",
		InputCount:         result.Checkpoint.InputCount,
		ManifestPath:       "corpus-pressure-manifest.json",
		ReportPath:         filepath.ToSlash(filepath.Join(CorpusIntakeDirName, "intake-report.md")),
		AuthorityIDs:       corpusIntakeAuthorityIDs(),
		ReasonCounts:       map[CorpusIntakeReason]int{},
		StateCounts:        map[CorpusIntakeItemState]int{},
		DestinationWrites:  0,
		ProductBrainWrites: 0,
		TolariaWrites:      0,
	}
	sourceInputs := []corpusintake.SourceInput{}
	processedSourceIDs := map[string]bool{}
	for _, candidate := range result.Candidates {
		item := corpusIntakeItem(candidate)
		switch item.State {
		case CorpusIntakeItemProcessed:
			if processedSourceIDs[item.SourceID] {
				item.State = CorpusIntakeItemSkipped
				item.ReasonCode = CorpusIntakeReasonDuplicateMessage
			} else {
				processedSourceIDs[item.SourceID] = true
				sourceInputs = append(sourceInputs, corpusintake.SourceInput{SourceID: item.SourceID, Candidate: candidate})
			}
		}
		summary.Items = append(summary.Items, item)
	}
	outputs, manifestPath, err := corpusintake.WriteSourcesAndManifest(fileSystem, root, summary.CorpusID, sourceInputs, corpusIntakeMarkdown)
	if err != nil {
		return CorpusIntakeSummary{}, err
	}
	summary.ManifestPath = manifestPath
	for i := range summary.Items {
		item := &summary.Items[i]
		if output, ok := outputs[item.SourceID]; ok {
			if output.Error != nil {
				item.State = CorpusIntakeItemBlocked
				item.ReasonCode = CorpusIntakeReasonArtifactWrite
			} else {
				item.SourcePath = output.Path
			}
		}
		summary.StateCounts[item.State]++
		summary.ReasonCounts[item.ReasonCode]++
		if item.Private {
			summary.PrivateProvenance++
		}
		if item.SecretLike {
			summary.SecretLikeCount++
		}
	}
	sort.SliceStable(summary.Items, func(i, j int) bool {
		left, leftErr := parsedEmailTimestamp(summary.Items[i].EmailTS)
		right, rightErr := parsedEmailTimestamp(summary.Items[j].EmailTS)
		if leftErr == nil && rightErr == nil {
			if left.Equal(right) {
				return summary.Items[i].SourceID < summary.Items[j].SourceID
			}
			return left.Before(right)
		}
		if summary.Items[i].EmailTS == summary.Items[j].EmailTS {
			return summary.Items[i].SourceID < summary.Items[j].SourceID
		}
		return summary.Items[i].EmailTS < summary.Items[j].EmailTS
	})
	summary.ProcessedCount = summary.StateCounts[CorpusIntakeItemProcessed]
	summary.SkippedCount = summary.StateCounts[CorpusIntakeItemSkipped]
	summary.BlockedCount = summary.StateCounts[CorpusIntakeItemBlocked]
	if err := writeCorpusIntakeSummary(fileSystem, root, summary); err != nil {
		return CorpusIntakeSummary{}, err
	}
	if err := writeCorpusIntakeReport(fileSystem, root, summary); err != nil {
		return CorpusIntakeSummary{}, err
	}
	return summary, nil
}

func corpusIntakeItem(candidate sbos.Candidate) CorpusIntakeItem {
	item := CorpusIntakeItem{
		SourceID:     corpusIntakeSourceID(candidate),
		EmailTS:      candidate.Provenance.NativeTimestamp.Value,
		State:        CorpusIntakeItemProcessed,
		ReasonCode:   CorpusIntakeReasonNone,
		Private:      candidate.Safety.PrivateProvenance,
		SecretLike:   candidate.Safety.SecretLike,
		EmptyContent: candidate.Safety.EmptyContent,
	}
	if candidate.Safety.EmptyContent {
		item.State = CorpusIntakeItemSkipped
		item.ReasonCode = CorpusIntakeReasonEmptyMessage
	}
	if candidate.Safety.SecretLike {
		item.State = CorpusIntakeItemBlocked
		item.ReasonCode = CorpusIntakeReasonSecretLike
	}
	return item
}

func writeCorpusIntakeSummary(fileSystem CorpusIntakeFileSystem, root string, summary CorpusIntakeSummary) error {
	return corpusintake.WriteJSON(fileSystem, root, filepath.ToSlash(filepath.Join(CorpusIntakeDirName, "intake-summary.json")), summary)
}

func writeCorpusIntakeReport(fileSystem CorpusIntakeFileSystem, root string, summary CorpusIntakeSummary) error {
	return corpusintake.WriteMarkdown(fileSystem, root, filepath.ToSlash(filepath.Join(CorpusIntakeDirName, "intake-report.md")), corpusIntakeReport(summary))
}

func corpusIntakeMarkdown(candidate sbos.Candidate) string {
	var b strings.Builder
	b.WriteString("# Gmail message\n\n")
	b.WriteString("## Source metadata\n\n")
	b.WriteString(fmt.Sprintf("- Candidate ID: `%s`\n", candidate.CandidateID))
	b.WriteString(fmt.Sprintf("- External ID: `%s`\n", candidate.ExternalID))
	b.WriteString(fmt.Sprintf("- Gmail timestamp: `%s`\n", candidate.Provenance.NativeTimestamp.Value))
	b.WriteString(fmt.Sprintf("- Author: %s\n", candidate.Provenance.Author.Value))
	b.WriteString(fmt.Sprintf("- Permalink: %s\n", candidate.Provenance.Permalink.Value))
	b.WriteString(fmt.Sprintf("- Raw locator: `%s`\n\n", candidate.Provenance.RawLocator.Value))
	b.WriteString("## Content\n\n")
	b.WriteString(strings.TrimSpace(candidate.Content.Text))
	b.WriteString("\n")
	if len(candidate.Content.URLs) > 0 {
		b.WriteString("\n## URLs\n\n")
		for _, url := range candidate.Content.URLs {
			b.WriteString("- " + url + "\n")
		}
	}
	if len(candidate.Content.Attachments) > 0 {
		b.WriteString("\n## Attachments\n\n")
		for _, attachment := range candidate.Content.Attachments {
			b.WriteString("- " + attachment + "\n")
		}
	}
	return b.String()
}

func corpusIntakeReport(summary CorpusIntakeSummary) string {
	var b strings.Builder
	b.WriteString("# Gmail corpus intake report\n\n")
	b.WriteString("## Intake answer\n\n")
	b.WriteString(fmt.Sprintf("- Corpus: `%s`\n", summary.CorpusID))
	b.WriteString(fmt.Sprintf("- Source: `%s`\n", summary.Source))
	b.WriteString(fmt.Sprintf("- Batch order: %s\n", summary.BatchOrder))
	b.WriteString(fmt.Sprintf("- Messages: %d input, %d processed, %d skipped, %d blocked\n", summary.InputCount, summary.ProcessedCount, summary.SkippedCount, summary.BlockedCount))
	b.WriteString(fmt.Sprintf("- Private provenance count: %d\n", summary.PrivateProvenance))
	b.WriteString(fmt.Sprintf("- Secret-like count: %d\n", summary.SecretLikeCount))
	if summary.ManifestPath == "" {
		b.WriteString("- Corpus manifest: not emitted because there are no processed sources\n")
	} else {
		b.WriteString(fmt.Sprintf("- Corpus manifest: `%s`\n", summary.ManifestPath))
	}
	b.WriteString("- Destination writes: 0\n")
	b.WriteString("- Product Brain writes: 0\n")
	b.WriteString("- Tolaria writes: 0\n\n")
	b.WriteString("## Source accounting\n\n")
	for _, item := range summary.Items {
		path := item.SourcePath
		if path == "" {
			path = "-"
		}
		b.WriteString(fmt.Sprintf("- `%s` ts=%s state=%s reason=%s source=`%s`\n", item.SourceID, item.EmailTS, item.State, item.ReasonCode, path))
	}
	if len(summary.Items) == 0 {
		b.WriteString("- No Gmail messages were present.\n")
	}
	return b.String()
}

func corpusIntakeSourceID(candidate sbos.Candidate) string {
	return sanitizeLocalID(candidate.CandidateID)
}

func corpusID(source Source) string {
	sum := sha256.Sum256([]byte(sourceID(source)))
	return "corpus-gmail-" + hex.EncodeToString(sum[:])[:16]
}

func mailbox(source Source) string {
	return mailboxID(source)
}

func sanitizeLocalID(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	clean := strings.Trim(b.String(), "-_")
	for strings.Contains(clean, "--") {
		clean = strings.ReplaceAll(clean, "--", "-")
	}
	if clean == "" {
		return "gmail-source"
	}
	return clean
}

func corpusIntakeAuthorityIDs() []string {
	ids := append([]string{}, authorityIDs()...)
	ids = append(ids, "WP-46", "WP-31", "WP-30", "STR-3", "PRI-1", "BR-1")
	return ids
}
