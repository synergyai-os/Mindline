package slack

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

	"github.com/synergyai-os/Mindline/internal/documents"
	"github.com/synergyai-os/Mindline/internal/sbos"
)

const CorpusIntakeDirName = "slack-corpus-intake"

func BuildCorpusIntake(payload Payload, outDir string) (CorpusIntakeSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return CorpusIntakeSummary{}, fmt.Errorf("missing required --out")
	}
	root, err := filepath.Abs(outDir)
	if err != nil {
		return CorpusIntakeSummary{}, err
	}
	if err := rejectSlackCorpusIntakeSymlinkAncestors(root); err != nil {
		return CorpusIntakeSummary{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
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
		ChannelID:          strings.TrimSpace(payload.Source.ChannelID),
		ChannelName:        strings.TrimSpace(payload.Source.ChannelName),
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
	manifest := documents.CorpusPressureManifest{
		SchemaVersion: documents.CorpusPressureManifestSchemaVersion,
		CorpusID:      summary.CorpusID,
	}
	for _, candidate := range result.Candidates {
		item := corpusIntakeItem(candidate, payload.Source)
		switch item.State {
		case CorpusIntakeItemProcessed:
			sourcePath, err := writeCorpusIntakeSource(root, item.SourceID, candidate)
			if err != nil {
				item.State = CorpusIntakeItemBlocked
				item.ReasonCode = CorpusIntakeReasonArtifactWrite
				item.SourcePath = ""
			} else {
				item.SourcePath = sourcePath
				manifest.Sources = append(manifest.Sources, documents.CorpusPressureManifestSource{
					SourceID:   item.SourceID,
					SourceKind: documents.SourceKindMarkdown,
					Path:       sourcePath,
				})
			}
		}
		summary.Items = append(summary.Items, item)
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
		return summary.Items[i].SlackTS < summary.Items[j].SlackTS
	})
	summary.ProcessedCount = summary.StateCounts[CorpusIntakeItemProcessed]
	summary.SkippedCount = summary.StateCounts[CorpusIntakeItemSkipped]
	summary.BlockedCount = summary.StateCounts[CorpusIntakeItemBlocked]
	if len(manifest.Sources) > 0 {
		if err := writeCorpusIntakeManifest(root, manifest); err != nil {
			return CorpusIntakeSummary{}, err
		}
	} else {
		summary.ManifestPath = ""
	}
	if err := writeCorpusIntakeSummary(root, summary); err != nil {
		return CorpusIntakeSummary{}, err
	}
	if err := writeCorpusIntakeReport(root, summary); err != nil {
		return CorpusIntakeSummary{}, err
	}
	return summary, nil
}

func corpusIntakeItem(candidate sbos.Candidate, source Source) CorpusIntakeItem {
	sourceID := corpusIntakeSourceID(source, candidate.Provenance.NativeTimestamp.Value)
	item := CorpusIntakeItem{
		SourceID:     sourceID,
		ExternalID:   candidate.ExternalID,
		SlackTS:      candidate.Provenance.NativeTimestamp.Value,
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

func writeCorpusIntakeSource(root, sourceID string, candidate sbos.Candidate) (string, error) {
	rel := filepath.ToSlash(filepath.Join("sources", sourceID, "source.md"))
	target := filepath.Join(root, filepath.FromSlash(rel))
	if !isInside(root, target) {
		return "", fmt.Errorf("source path escaped output directory")
	}
	if err := rejectSlackCorpusIntakeSymlinkPath(root, filepath.Dir(target)); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := rejectSlackCorpusIntakeSymlinkPath(root, target); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, []byte(corpusIntakeMarkdown(candidate)), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

func writeCorpusIntakeManifest(root string, manifest documents.CorpusPressureManifest) error {
	target := filepath.Join(root, "corpus-pressure-manifest.json")
	if err := rejectSlackCorpusIntakeSymlinkPath(root, target); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(target, data, 0o644)
}

func writeCorpusIntakeSummary(root string, summary CorpusIntakeSummary) error {
	dir := filepath.Join(root, CorpusIntakeDirName)
	if err := rejectSlackCorpusIntakeSymlinkPath(root, dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dir, "intake-summary.json")
	if err := rejectSlackCorpusIntakeSymlinkPath(root, target); err != nil {
		return err
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(target, data, 0o644)
}

func writeCorpusIntakeReport(root string, summary CorpusIntakeSummary) error {
	dir := filepath.Join(root, CorpusIntakeDirName)
	if err := rejectSlackCorpusIntakeSymlinkPath(root, dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dir, "intake-report.md")
	if err := rejectSlackCorpusIntakeSymlinkPath(root, target); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(corpusIntakeReport(summary)), 0o644)
}

func corpusIntakeMarkdown(candidate sbos.Candidate) string {
	var b strings.Builder
	b.WriteString("# Slack capture\n\n")
	b.WriteString("## Source metadata\n\n")
	b.WriteString(fmt.Sprintf("- Candidate ID: `%s`\n", candidate.CandidateID))
	b.WriteString(fmt.Sprintf("- External ID: `%s`\n", candidate.ExternalID))
	b.WriteString(fmt.Sprintf("- Slack timestamp: `%s`\n", candidate.Provenance.NativeTimestamp.Value))
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
	b.WriteString("# Slack corpus intake report\n\n")
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
		b.WriteString(fmt.Sprintf("- `%s` ts=%s state=%s reason=%s source=`%s`\n", item.SourceID, item.SlackTS, item.State, item.ReasonCode, path))
	}
	if len(summary.Items) == 0 {
		b.WriteString("- No Slack messages were present.\n")
	}
	return b.String()
}

func corpusIntakeSourceID(source Source, ts string) string {
	return sanitizeLocalID("slack-" + source.Workspace + "-" + source.ChannelID + "-" + normalizeTS(ts))
}

func corpusID(source Source) string {
	sum := sha256.Sum256([]byte(sourceID(source)))
	return "corpus-slack-" + hex.EncodeToString(sum[:])[:16]
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
		return "slack-source"
	}
	return clean
}

func isInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func rejectSlackCorpusIntakeSymlinkPath(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if rel == "." {
		return rejectSlackCorpusIntakeSymlink(root)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escaped output directory")
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		if err := rejectSlackCorpusIntakeSymlink(current); err != nil {
			return err
		}
	}
	return nil
}

func rejectSlackCorpusIntakeSymlink(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("output path contains symlink: %s", path)
	}
	return nil
}

func rejectSlackCorpusIntakeSymlinkAncestors(path string) error {
	clean := filepath.Clean(path)
	current := string(filepath.Separator)
	rel, err := filepath.Rel(current, clean)
	if err != nil {
		return err
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if isSlackCorpusIntakePlatformTempAlias(current) {
				continue
			}
			return fmt.Errorf("output path contains symlink: %s", current)
		}
	}
	return nil
}

func isSlackCorpusIntakePlatformTempAlias(path string) bool {
	switch filepath.Clean(path) {
	case "/tmp", "/var":
		return true
	default:
		return false
	}
}

func corpusIntakeAuthorityIDs() []string {
	ids := append([]string{}, authorityIDs()...)
	ids = append(ids, "WP-31", "WP-29", "WP-30", "STR-3", "PRI-1", "BR-1")
	return ids
}
