package documents

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type line struct {
	number int
	text   string
}

type section struct {
	headingPath []string
	lines       []line
}

func DecomposePath(inputPath, outDir string) (Summary, error) {
	paths, err := markdownPaths(inputPath)
	if err != nil {
		return Summary{}, err
	}
	sourceIDsByPath := sourceDocumentIDs(paths)
	sourceIDs := make([]string, 0, len(paths))
	for _, path := range paths {
		sourceIDs = append(sourceIDs, sourceIDsByPath[path])
	}
	runID := RunID(sourceIDs)
	var segments []Segment
	for _, path := range paths {
		sourceSegments, err := decomposeFile(path, runID, sourceIDsByPath[path])
		if err != nil {
			return Summary{}, err
		}
		segments = append(segments, sourceSegments...)
	}
	summary := BuildSummary(runID, len(paths), segments)
	if err := Write(outDir, summary, segments); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func BuildSummary(runID string, sourceCount int, segments []Segment) Summary {
	summary := Summary{
		SchemaVersion: SegmentSummarySchemaVersion,
		RunID:         runID,
		SourceCount:   sourceCount,
		TypeCounts:    map[SemanticType]int{},
		AuthorityIDs:  append([]string(nil), WP10AuthorityIDs...),
	}
	for _, segment := range segments {
		summary.SegmentCount++
		if segment.ReviewStatus == ReviewStatusNeedsReview {
			summary.NeedsReviewCount++
		}
		summary.TypeCounts[segment.SemanticType]++
		summary.Segments = append(summary.Segments, SummarySegment{
			SegmentID:        segment.SegmentID,
			SourceDocumentID: segment.SourceDocumentID,
			SemanticType:     segment.SemanticType,
			ReviewStatus:     segment.ReviewStatus,
			Confidence:       segment.Confidence,
			SegmentPath:      SegmentJSONPath(segment.SegmentID),
			PreviewPath:      SegmentPreviewPath(segment.SegmentID),
		})
	}
	return summary
}

func markdownPaths(inputPath string) ([]string, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{inputPath}, nil
	}
	var paths []string
	err = filepath.WalkDir(inputPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no markdown files found")
	}
	return paths, nil
}

func sourceDocumentIDs(paths []string) map[string]string {
	counts := map[string]int{}
	for _, path := range paths {
		counts[SourceDocumentID(path)]++
	}
	ids := map[string]string{}
	for _, path := range paths {
		sourceID := SourceDocumentID(path)
		if counts[sourceID] > 1 {
			sourceID = DisambiguatedSourceDocumentID(path)
		}
		ids[path] = sourceID
	}
	return ids
}

func decomposeFile(path, runID, sourceID string) ([]Segment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sections, err := parseSections(string(data))
	if err != nil {
		return nil, err
	}
	var segments []Segment
	for _, section := range sections {
		segments = append(segments, decomposeSection(runID, sourceID, section)...)
	}
	return segments, RejectDuplicateSegmentIDs(segments)
}

func parseSections(body string) ([]section, error) {
	var sections []section
	current := section{}
	var headingPath []string
	scanner := bufio.NewScanner(strings.NewReader(body))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		text := scanner.Text()
		if strings.HasPrefix(text, "#") {
			level := headingLevel(text)
			heading := strings.TrimSpace(strings.TrimLeft(text, "#"))
			if len(current.lines) > 0 || len(current.headingPath) > 0 {
				sections = append(sections, current)
			}
			if level <= len(headingPath) {
				headingPath = headingPath[:level-1]
			}
			headingPath = append(headingPath, heading)
			current = section{headingPath: append([]string(nil), headingPath...)}
			continue
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		current.lines = append(current.lines, line{number: lineNumber, text: text})
	}
	if len(current.lines) > 0 || len(current.headingPath) > 0 {
		sections = append(sections, current)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return sections, nil
}

func headingLevel(text string) int {
	level := 0
	for _, r := range text {
		if r != '#' {
			break
		}
		level++
	}
	if level == 0 {
		return 1
	}
	return level
}

func decomposeSection(runID, sourceID string, section section) []Segment {
	if len(section.lines) == 0 {
		return nil
	}
	var segments []Segment
	if hasMarkdownTable(section.lines) {
		segments = append(segments, decomposeTable(runID, sourceID, section)...)
	}
	for _, line := range section.lines {
		text := strings.TrimSpace(strings.TrimPrefix(line.text, "-"))
		text = strings.TrimSpace(text)
		if text == "" || strings.HasPrefix(text, "|") || isMarkdownTableDelimiter(text) {
			continue
		}
		segments = append(segments, segmentFromText(runID, sourceID, section.headingPath, line.number, line.number, text))
	}
	return segments
}

func hasMarkdownTable(lines []line) bool {
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line.text), "|") {
			return true
		}
	}
	return false
}

func decomposeTable(runID, sourceID string, section section) []Segment {
	var rows []line
	for _, line := range section.lines {
		text := strings.TrimSpace(line.text)
		if strings.HasPrefix(text, "|") && !isMarkdownTableDelimiter(text) {
			rows = append(rows, line)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	var segments []Segment
	first := rows[0]
	segments = append(segments, newSegment(runID, sourceID, section.headingPath, first.number, rows[len(rows)-1].number, SemanticTypeReference, ReviewStatusReady, ConfidenceMedium, "Reference table: "+headingTitle(section.headingPath), "Table records structured reference material for this section."))
	for _, row := range rows[1:] {
		cells := tableCells(row.text)
		if len(cells) == 0 {
			continue
		}
		title := cells[0]
		summary := strings.Join(cells, " - ")
		segments = append(segments, newSegment(runID, sourceID, section.headingPath, row.number, row.number, SemanticTypeReference, ReviewStatusReady, ConfidenceMedium, title, summary))
	}
	return segments
}

func tableCells(row string) []string {
	parts := strings.Split(strings.Trim(row, "|"), "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cell := strings.TrimSpace(part)
		if cell != "" {
			cells = append(cells, cell)
		}
	}
	return cells
}

func isMarkdownTableDelimiter(text string) bool {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "|") {
		return strings.Trim(text, "-: ") == ""
	}
	cells := tableCells(text)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if strings.Trim(cell, "-: ") != "" {
			return false
		}
	}
	return true
}

func segmentFromText(runID, sourceID string, headingPath []string, start, end int, text string) Segment {
	lower := strings.ToLower(text)
	semanticType := SemanticTypeSourceNote
	status := ReviewStatusReady
	confidence := ConfidenceMedium
	title := titleFromText(text)
	summary := summaryFromText(text)
	switch {
	case strings.Contains(lower, "private_content"):
		semanticType = SemanticTypeSourceNote
		status = ReviewStatusBlocked
		confidence = ConfidenceLow
	case strings.Contains(lower, "decision:") || strings.HasPrefix(lower, "decision:"):
		semanticType = SemanticTypeDecision
		confidence = ConfidenceHigh
	case strings.Contains(lower, "standard:"):
		semanticType = SemanticTypeStandard
		confidence = ConfidenceHigh
	case strings.Contains(lower, "risk:"):
		semanticType = SemanticTypeTension
	case strings.Contains(lower, "insight:"):
		semanticType = SemanticTypeInsight
	case strings.Contains(lower, "work item:"):
		semanticType = SemanticTypeWorkItem
	case strings.Contains(lower, "maybe") || strings.Contains(lower, "unclear"):
		semanticType = SemanticTypeUnknown
		status = ReviewStatusNeedsReview
		confidence = ConfidenceLow
	case strings.Contains(lower, "action:") || strings.Contains(lower, " to "):
		semanticType = SemanticTypeAction
	case strings.Contains(lower, "reference:") || strings.Contains(lower, "checklist") || strings.Contains(lower, "stage"):
		semanticType = SemanticTypeReference
	case strings.Contains(strings.ToLower(headingTitle(headingPath)), "meeting") || strings.Contains(lower, "speaker"):
		semanticType = SemanticTypeMeetingNote
	default:
		semanticType = SemanticTypeSourceNote
	}
	segment := newSegment(runID, sourceID, headingPath, start, end, semanticType, status, confidence, title, summary)
	return segment
}

func newSegment(runID, sourceID string, headingPath []string, start, end int, semanticType SemanticType, status ReviewStatus, confidence Confidence, title, summary string) Segment {
	hash := contentHash(strings.Join([]string{sourceID, headingTitle(headingPath), title, summary}, "\n"))
	segment := Segment{
		SchemaVersion:    SegmentSchemaVersion,
		RunID:            runID,
		SourceDocumentID: sourceID,
		SourceKind:       SourceKindMarkdown,
		SemanticType:     semanticType,
		ReviewStatus:     status,
		Confidence:       confidence,
		Title:            title,
		Summary:          summary,
		Evidence: Evidence{
			Kind:        EvidenceKindLocation,
			HeadingPath: append([]string(nil), headingPath...),
			LineStart:   start,
			LineEnd:     end,
			ContentHash: "sha256:" + hash,
		},
		Blockers:     []Blocker{},
		AuthorityIDs: append([]string(nil), WP10AuthorityIDs...),
	}
	segment.SegmentID = SegmentID(runID, sourceID, []string{headingTitle(headingPath)}, start, title+"\n"+summary)
	return ClassifyUnsafeMarkers(segment)
}

func titleFromText(text string) string {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"Speaker A:", "Speaker B:", "Speaker C:", "Product Lead:", "Support Lead:"} {
		text = strings.TrimPrefix(text, prefix)
		text = strings.TrimSpace(text)
	}
	for _, prefix := range []string{"Decision:", "Action:", "Insight:", "Risk:", "Reference:", "Standard:", "Work item:"} {
		if strings.HasPrefix(strings.ToLower(text), strings.ToLower(prefix)) {
			text = strings.TrimSpace(text[len(prefix):])
		}
	}
	if len(text) > 72 {
		text = strings.TrimSpace(text[:72])
	}
	return strings.Trim(text, ".")
}

func summaryFromText(text string) string {
	text = strings.TrimSpace(strings.TrimPrefix(text, "-"))
	if strings.HasPrefix(text, "Speaker ") {
		parts := strings.SplitN(text, ":", 2)
		if len(parts) == 2 {
			text = strings.TrimSpace(parts[1])
		}
	}
	return text
}

func headingTitle(path []string) string {
	if len(path) == 0 {
		return "Document"
	}
	return path[len(path)-1]
}

func contentHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
