package documents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ArtifactWriteError struct {
	Err error
}

func (e ArtifactWriteError) Error() string {
	return e.Err.Error()
}

func (e ArtifactWriteError) Unwrap() error {
	return e.Err
}

func IsArtifactWriteError(err error) bool {
	var writeErr ArtifactWriteError
	return errors.As(err, &writeErr)
}

func Write(outDir string, summary Summary, segments []Segment) error {
	if err := write(outDir, summary, segments); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func write(outDir string, summary Summary, segments []Segment) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("missing required --out")
	}
	if err := RejectDuplicateSegmentIDs(segments); err != nil {
		return err
	}
	segments = finalizeSegments(segments)
	root, err := filepath.Abs(filepath.Join(outDir, "document-segments"))
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
	for _, segment := range segments {
		if err := ValidateSegment(segment); err != nil {
			return err
		}
	}
	summary = BuildSummary(summary.RunID, summary.SourceCount, segments)
	if summary.SchemaVersion != SegmentSummarySchemaVersion {
		return fmt.Errorf("unsupported summary schema version: %s", summary.SchemaVersion)
	}
	expectedFiles := map[string]bool{"segment-summary.json": true}
	for _, segment := range segments {
		expectedFiles[SegmentJSONPath(segment.SegmentID)] = true
		expectedFiles[SegmentPreviewPath(segment.SegmentID)] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expectedFiles); err != nil {
		return err
	}
	if err := writeJSON(realRoot, "segment-summary.json", summary); err != nil {
		return err
	}
	for _, segment := range segments {
		if err := writeJSON(realRoot, SegmentJSONPath(segment.SegmentID), segment); err != nil {
			return err
		}
		if err := writeFile(realRoot, SegmentPreviewPath(segment.SegmentID), []byte(previewMarkdown(segment))); err != nil {
			return err
		}
	}
	return nil
}

func finalizeSegments(segments []Segment) []Segment {
	finalized := make([]Segment, 0, len(segments))
	for _, segment := range segments {
		finalized = append(finalized, ClassifyUnsafeMarkers(segment))
	}
	return finalized
}

func rejectIfSymlink(path string) error {
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("output path escaped output directory")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func rejectSymlinkAncestors(path string) error {
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
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				if isPlatformTempAlias(current) {
					continue
				}
				return fmt.Errorf("output path escaped output directory")
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func isPlatformTempAlias(path string) bool {
	switch filepath.Clean(path) {
	case "/tmp", "/var":
		return true
	default:
		return false
	}
}

func rejectUnexpectedExistingFiles(root string, expected map[string]bool) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !expected[rel] {
			return fmt.Errorf("unexpected existing generated file: %s", rel)
		}
		return nil
	})
}

func writeJSON(root, relative string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(root, relative, data)
}

func writeFile(root, relative string, data []byte) error {
	if filepath.IsAbs(relative) || strings.Contains(relative, "..") {
		return fmt.Errorf("output path escaped output directory")
	}
	target := filepath.Join(root, relative)
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if !isInside(cleanRoot, cleanTarget) || cleanRoot == cleanTarget {
		return fmt.Errorf("output path escaped output directory")
	}
	if err := ensureParentDir(cleanRoot, relative); err != nil {
		return err
	}
	if info, err := os.Lstat(cleanTarget); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("output path escaped output directory")
	}
	return os.WriteFile(cleanTarget, data, 0o644)
}

func ensureParentDir(root, relative string) error {
	dir := filepath.Dir(filepath.Clean(relative))
	if dir == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(dir, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("output path escaped output directory")
			}
			if !info.IsDir() {
				return fmt.Errorf("output path parent is not a directory")
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.Mkdir(current, 0o755); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}

func isInside(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}

func previewMarkdown(segment Segment) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(segment.Title)
	b.WriteString("\n\n")
	b.WriteString("- Segment: ")
	b.WriteString(segment.SegmentID)
	b.WriteString("\n")
	b.WriteString("- Type: ")
	b.WriteString(string(segment.SemanticType))
	b.WriteString("\n")
	b.WriteString("- Review status: ")
	b.WriteString(string(segment.ReviewStatus))
	b.WriteString("\n")
	b.WriteString("- Confidence: ")
	b.WriteString(string(segment.Confidence))
	b.WriteString("\n")
	b.WriteString("- Source document: ")
	b.WriteString(segment.SourceDocumentID)
	b.WriteString("\n")
	b.WriteString("- Lines: ")
	b.WriteString(fmt.Sprintf("%d-%d", segment.Evidence.LineStart, segment.Evidence.LineEnd))
	b.WriteString("\n\n")
	b.WriteString(segment.Summary)
	b.WriteString("\n")
	return b.String()
}
