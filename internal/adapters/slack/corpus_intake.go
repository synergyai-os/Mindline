package slack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/synergyai-os/Mindline/internal/documents"
	"github.com/synergyai-os/Mindline/internal/sbos"
)

const CorpusIntakeDirName = "slack-corpus-intake"

type CorpusIntakeFileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	ReadDir(path string) ([]fs.DirEntry, error)
	WriteFile(path string, data []byte) error
	Remove(path string) error
	RemoveAll(path string) error
	IsSymlink(path string) (bool, error)
}

func BuildCorpusIntake(payload Payload, outDir string) (CorpusIntakeSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return CorpusIntakeSummary{}, fmt.Errorf("missing required --out")
	}
	root, err := filepath.Abs(outDir)
	if err != nil {
		return CorpusIntakeSummary{}, err
	}
	return buildCorpusIntake(payload, root, osCorpusIntakeFileSystem{})
}

func BuildCorpusIntakeWithFileSystem(payload Payload, outDir string, fileSystem CorpusIntakeFileSystem) (CorpusIntakeSummary, error) {
	if strings.TrimSpace(outDir) == "" {
		return CorpusIntakeSummary{}, fmt.Errorf("missing required --out")
	}
	return buildCorpusIntake(payload, filepath.Clean(outDir), fileSystem)
}

func buildCorpusIntake(payload Payload, root string, fileSystem CorpusIntakeFileSystem) (CorpusIntakeSummary, error) {
	if err := rejectSlackCorpusIntakeSymlinkAncestors(fileSystem, root); err != nil {
		return CorpusIntakeSummary{}, err
	}
	if err := fileSystem.MkdirAll(root, 0o755); err != nil {
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
	currentSourceIDs := corpusIntakeProcessedSourceIDs(result.Candidates, payload.Source)
	if err := pruneCorpusIntakeSources(fileSystem, root, currentSourceIDs); err != nil {
		return CorpusIntakeSummary{}, err
	}
	processedSourceIDs := map[string]bool{}
	for _, candidate := range result.Candidates {
		item := corpusIntakeItem(candidate, payload.Source)
		switch item.State {
		case CorpusIntakeItemProcessed:
			if processedSourceIDs[item.SourceID] {
				item.State = CorpusIntakeItemSkipped
				item.ReasonCode = CorpusIntakeReasonDuplicateMessage
			} else {
				processedSourceIDs[item.SourceID] = true
				sourcePath, err := writeCorpusIntakeSource(fileSystem, root, item.SourceID, candidate)
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
	if err := pruneCorpusIntakeSources(fileSystem, root, corpusIntakeManifestSourceIDs(manifest)); err != nil {
		return CorpusIntakeSummary{}, err
	}
	if len(manifest.Sources) > 0 {
		if err := writeCorpusIntakeManifest(fileSystem, root, manifest); err != nil {
			return CorpusIntakeSummary{}, err
		}
	} else {
		if err := removeCorpusIntakeManifest(fileSystem, root); err != nil {
			return CorpusIntakeSummary{}, err
		}
		if err := removeCorpusIntakeSources(fileSystem, root); err != nil {
			return CorpusIntakeSummary{}, err
		}
		summary.ManifestPath = ""
	}
	if err := writeCorpusIntakeSummary(fileSystem, root, summary); err != nil {
		return CorpusIntakeSummary{}, err
	}
	if err := writeCorpusIntakeReport(fileSystem, root, summary); err != nil {
		return CorpusIntakeSummary{}, err
	}
	return summary, nil
}

func corpusIntakeManifestSourceIDs(manifest documents.CorpusPressureManifest) map[string]bool {
	sourceIDs := map[string]bool{}
	for _, source := range manifest.Sources {
		sourceIDs[source.SourceID] = true
	}
	return sourceIDs
}

func corpusIntakeProcessedSourceIDs(candidates []sbos.Candidate, source Source) map[string]bool {
	sourceIDs := map[string]bool{}
	for _, candidate := range candidates {
		item := corpusIntakeItem(candidate, source)
		if item.State == CorpusIntakeItemProcessed {
			sourceIDs[item.SourceID] = true
		}
	}
	return sourceIDs
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

func writeCorpusIntakeSource(fileSystem CorpusIntakeFileSystem, root, sourceID string, candidate sbos.Candidate) (string, error) {
	rel := filepath.ToSlash(filepath.Join("sources", sourceID, "source.md"))
	target := filepath.Join(root, filepath.FromSlash(rel))
	if !isInside(root, target) {
		return "", fmt.Errorf("source path escaped output directory")
	}
	if err := rejectSlackCorpusIntakeSymlinkPath(fileSystem, root, filepath.Dir(target)); err != nil {
		return "", err
	}
	if err := fileSystem.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := rejectSlackCorpusIntakeSymlinkPath(fileSystem, root, target); err != nil {
		return "", err
	}
	if err := fileSystem.WriteFile(target, []byte(corpusIntakeMarkdown(candidate))); err != nil {
		return "", err
	}
	return rel, nil
}

func writeCorpusIntakeManifest(fileSystem CorpusIntakeFileSystem, root string, manifest documents.CorpusPressureManifest) error {
	target := filepath.Join(root, "corpus-pressure-manifest.json")
	if err := rejectSlackCorpusIntakeSymlinkPath(fileSystem, root, target); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fileSystem.WriteFile(target, data)
}

func removeCorpusIntakeManifest(fileSystem CorpusIntakeFileSystem, root string) error {
	target := filepath.Join(root, "corpus-pressure-manifest.json")
	if !isInside(root, target) {
		return fmt.Errorf("manifest path escaped output directory")
	}
	return fileSystem.Remove(target)
}

func removeCorpusIntakeSources(fileSystem CorpusIntakeFileSystem, root string) error {
	target := filepath.Join(root, "sources")
	if !isInside(root, target) {
		return fmt.Errorf("sources path escaped output directory")
	}
	return fileSystem.RemoveAll(target)
}

func pruneCorpusIntakeSources(fileSystem CorpusIntakeFileSystem, root string, keepSourceIDs map[string]bool) error {
	sourcesDir := filepath.Join(root, "sources")
	if !isInside(root, sourcesDir) {
		return fmt.Errorf("sources path escaped output directory")
	}
	isSymlink, err := fileSystem.IsSymlink(sourcesDir)
	if err != nil {
		return err
	}
	if isSymlink {
		return nil
	}
	entries, err := fileSystem.ReadDir(sourcesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if keepSourceIDs[entry.Name()] {
			continue
		}
		target := filepath.Join(sourcesDir, entry.Name())
		if !isInside(root, target) {
			return fmt.Errorf("source path escaped output directory")
		}
		if err := fileSystem.RemoveAll(target); err != nil {
			return err
		}
	}
	return nil
}

func writeCorpusIntakeSummary(fileSystem CorpusIntakeFileSystem, root string, summary CorpusIntakeSummary) error {
	dir := filepath.Join(root, CorpusIntakeDirName)
	if err := rejectSlackCorpusIntakeSymlinkPath(fileSystem, root, dir); err != nil {
		return err
	}
	if err := fileSystem.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dir, "intake-summary.json")
	if err := rejectSlackCorpusIntakeSymlinkPath(fileSystem, root, target); err != nil {
		return err
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fileSystem.WriteFile(target, data)
}

func writeCorpusIntakeReport(fileSystem CorpusIntakeFileSystem, root string, summary CorpusIntakeSummary) error {
	dir := filepath.Join(root, CorpusIntakeDirName)
	if err := rejectSlackCorpusIntakeSymlinkPath(fileSystem, root, dir); err != nil {
		return err
	}
	if err := fileSystem.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dir, "intake-report.md")
	if err := rejectSlackCorpusIntakeSymlinkPath(fileSystem, root, target); err != nil {
		return err
	}
	return fileSystem.WriteFile(target, []byte(corpusIntakeReport(summary)))
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

func rejectSlackCorpusIntakeSymlinkPath(fileSystem CorpusIntakeFileSystem, root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if rel == "." {
		return rejectSlackCorpusIntakeSymlink(fileSystem, root)
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
		if err := rejectSlackCorpusIntakeSymlink(fileSystem, current); err != nil {
			return err
		}
	}
	return nil
}

func rejectSlackCorpusIntakeSymlink(fileSystem CorpusIntakeFileSystem, path string) error {
	isSymlink, err := fileSystem.IsSymlink(path)
	if err != nil {
		return err
	}
	if isSymlink {
		return fmt.Errorf("output path contains symlink: %s", path)
	}
	return nil
}

func rejectSlackCorpusIntakeSymlinkAncestors(fileSystem CorpusIntakeFileSystem, path string) error {
	clean := filepath.Clean(path)
	current := ""
	rel := clean
	if filepath.IsAbs(clean) {
		current = string(filepath.Separator)
		var err error
		rel, err = filepath.Rel(current, clean)
		if err != nil {
			return err
		}
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		if current == "" {
			current = part
		} else {
			current = filepath.Join(current, part)
		}
		isSymlink, err := fileSystem.IsSymlink(current)
		if err != nil {
			return err
		}
		if isSymlink {
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

type osCorpusIntakeFileSystem struct{}

func (osCorpusIntakeFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osCorpusIntakeFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return entries, err
}

func (osCorpusIntakeFileSystem) WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func (osCorpusIntakeFileSystem) Remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (osCorpusIntakeFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (osCorpusIntakeFileSystem) IsSymlink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}
