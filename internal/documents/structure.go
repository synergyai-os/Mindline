package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	governanceIDPattern   = regexp.MustCompile(`\b(PROD|DOMAIN|WP|DEC|STD|INS)-[0-9]+\b`)
	capabilityCodePattern = regexp.MustCompile(`^(PL-[0-9]+(?:–[0-9]+)?|P-[A-Z][0-9]+)(?: - | — |: )(.+)$`)
	transcriptTurnPattern = regexp.MustCompile(`^([0-9]{1,2}:[0-9]{2}(?::[0-9]{2})?) - (.+)$`)
	readableWordPattern   = regexp.MustCompile(`[A-Za-z][A-Za-z-]*`)
)

type tableBlock struct {
	startLine int
	endLine   int
	rows      []line
	malformed bool
}

func StructurePath(inputPath, outDir string) (StructureSummary, error) {
	paths, err := markdownPaths(inputPath)
	if err != nil {
		return StructureSummary{}, err
	}
	sourceIDsByPath, err := sourceDocumentIDs(inputPath, paths)
	if err != nil {
		return StructureSummary{}, err
	}
	wp10SourceIDsByPath, err := sourceDocumentIDs(inputPath, paths)
	if err != nil {
		return StructureSummary{}, err
	}
	sourceIDs := make([]string, 0, len(paths))
	wp10SourceIDs := make([]string, 0, len(paths))
	contentHashes := make([]string, 0, len(paths))
	fileData := map[string]string{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return StructureSummary{}, err
		}
		body := string(data)
		fileData[path] = body
		sourceIDs = append(sourceIDs, sourceIDsByPath[path])
		wp10SourceIDs = append(wp10SourceIDs, wp10SourceIDsByPath[path])
		contentHashes = append(contentHashes, "sha256:"+contentHash(body))
	}
	runID := StructureRunID(sourceIDs, contentHashes)
	segmentRunID := RunID(wp10SourceIDs)
	var nodes []StructureNode
	for _, path := range paths {
		sourceID := sourceIDsByPath[path]
		segments, err := decomposeFile(path, segmentRunID, wp10SourceIDsByPath[path])
		if err != nil {
			return StructureSummary{}, err
		}
		sourceNodes, err := structureFile(path, fileData[path], runID, sourceID, segments)
		if err != nil {
			return StructureSummary{}, err
		}
		nodes = append(nodes, sourceNodes...)
	}
	nodes = orderStructureNodes(nodes)
	if err := RejectDuplicateStructureNodeIDs(nodes); err != nil {
		return StructureSummary{}, err
	}
	if err := WriteStructure(outDir, runID, len(paths), nodes); err != nil {
		return StructureSummary{}, err
	}
	return BuildStructureSummary(runID, len(paths), nodes), nil
}

func structureFile(path, body, runID, sourceID string, segments []Segment) ([]StructureNode, error) {
	sections, err := parseSections(body)
	if err != nil {
		return nil, err
	}
	maxLine := maxLineNumber(sections)
	title := documentTitle(path, sections)
	root := newStructureNode(runID, sourceID, StructureNodeTypeDocument, "", []string{title}, 1, maxLine, title, "Document structure root.", segmentsForRange(segments, 1, maxLine))
	nodes := []StructureNode{root}
	sectionIDs := map[int]string{}
	latestSectionByPath := map[string]string{}
	hasH1 := documentHasH1(sections)
	for i, section := range sections {
		if hasH1 && len(section.headingPath) <= 1 {
			continue
		}
		if !hasH1 && len(section.headingPath) == 0 {
			continue
		}
		parentID := root.NodeID
		if len(section.headingPath) > 1 {
			parentKey := strings.Join(section.headingPath[:len(section.headingPath)-1], "\x00")
			if existing := latestSectionByPath[parentKey]; existing != "" {
				parentID = existing
			}
		}
		start, end := sectionLineRange(section)
		node := newStructureNode(runID, sourceID, StructureNodeTypeSection, parentID, section.headingPath, start, end, headingTitle(section.headingPath), "Section derived from Markdown heading hierarchy.", segmentsForRange(segments, start, end))
		key := strings.Join(section.headingPath, "\x00")
		sectionIDs[i] = node.NodeID
		latestSectionByPath[key] = node.NodeID
		nodes = append(nodes, node)
	}
	nodes = append(nodes, structureTranscriptTurns(runID, sourceID, root.NodeID, title, sections, segments)...)
	for i, section := range sections {
		parentID := root.NodeID
		if sectionIDs[i] != "" {
			parentID = sectionIDs[i]
		}
		nodes = append(nodes, structureSectionBlocks(runID, sourceID, parentID, section, segments)...)
	}
	nodes = linkStructureChildren(nodes)
	return nodes, nil
}

func structureSectionBlocks(runID, sourceID, parentID string, section section, segments []Segment) []StructureNode {
	var nodes []StructureNode
	for _, block := range tableBlocks(section.lines) {
		title := "Table: " + headingTitle(section.headingPath)
		tablePath := append(append([]string(nil), section.headingPath...), title)
		tableNode := newStructureNode(runID, sourceID, StructureNodeTypeTable, parentID, tablePath, block.startLine, block.endLine, title, "Markdown table block.", segmentsForRange(segments, block.startLine, block.endLine))
		if block.malformed {
			tableNode = markStructureNeedsReview(tableNode, "Malformed Markdown table block requires review.")
		}
		nodes = append(nodes, tableNode)
		for _, row := range dataRows(block.rows) {
			cells := tableCells(row.text)
			if len(cells) == 0 {
				continue
			}
			rowTitle := cells[0]
			rowPath := append(append([]string(nil), tablePath...), rowTitle)
			rowNode := newStructureNode(runID, sourceID, StructureNodeTypeTableRow, tableNode.NodeID, rowPath, row.number, row.number, rowTitle, strings.Join(cells, " - "), segmentsForRange(segments, row.number, row.number))
			if block.malformed {
				rowNode = markStructureNeedsReview(rowNode, "Row belongs to a malformed Markdown table block.")
			}
			nodes = append(nodes, rowNode)
			if typedNodeType, typedTitle, ok := typedNodeFromTitle(rowTitle, rowTitle+" "+strings.Join(cells[1:], " ")); ok {
				typedPath := append(rowPath, string(typedNodeType))
				typedNode := newStructureNode(runID, sourceID, typedNodeType, rowNode.NodeID, typedPath, row.number, row.number, typedTitle, strings.Join(cells, " - "), rowNode.RelatedSegmentIDs)
				nodes = append(nodes, typedNode)
			}
		}
	}
	for _, item := range listItems(section.lines) {
		nodeType, title, ok := typedNodeFromTitle(item.text, item.text)
		if !ok {
			continue
		}
		nodePath := append(append([]string(nil), section.headingPath...), title)
		nodes = append(nodes, newStructureNode(runID, sourceID, nodeType, parentID, nodePath, item.number, item.number, title, strings.TrimSpace(strings.TrimPrefix(item.text, "-")), segmentsForRange(segments, item.number, item.number)))
	}
	return nodes
}

type transcriptTurn struct {
	timestamp string
	speaker   string
	start     int
	end       int
	hasText   bool
}

func structureTranscriptTurns(runID, sourceID, rootID, documentTitle string, sections []section, segments []Segment) []StructureNode {
	turns := transcriptTurns(sections)
	nodes := make([]StructureNode, 0, len(turns))
	for _, turn := range turns {
		title := turn.timestamp + " - " + turn.speaker
		node := newStructureNode(
			runID,
			sourceID,
			StructureNodeTypeTranscriptTurn,
			rootID,
			[]string{documentTitle, title},
			turn.start,
			turn.end,
			title,
			"Transcript turn by "+turn.speaker+" at "+turn.timestamp+".",
			segmentsForRange(segments, turn.start, turn.end),
		)
		if !turn.hasText {
			node = markStructureNeedsReview(node, "Transcript turn has a timestamp and speaker but no utterance text.")
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func transcriptTurns(sections []section) []transcriptTurn {
	lines := allSectionLines(sections)
	var turns []transcriptTurn
	for _, item := range lines {
		matches := transcriptTurnPattern.FindStringSubmatch(strings.TrimSpace(item.text))
		if matches == nil {
			if len(turns) > 0 && strings.TrimSpace(item.text) != "" {
				turns[len(turns)-1].end = item.number
				turns[len(turns)-1].hasText = true
			}
			continue
		}
		if len(turns) > 0 && turns[len(turns)-1].end < turns[len(turns)-1].start {
			turns[len(turns)-1].end = turns[len(turns)-1].start
		}
		turns = append(turns, transcriptTurn{
			timestamp: matches[1],
			speaker:   strings.TrimSpace(matches[2]),
			start:     item.number,
			end:       item.number,
		})
	}
	return turns
}

func allSectionLines(sections []section) []line {
	var lines []line
	for _, section := range sections {
		lines = append(lines, section.lines...)
	}
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].number < lines[j].number
	})
	return lines
}

func markStructureNeedsReview(node StructureNode, message string) StructureNode {
	node.ReviewStatus = ReviewStatusNeedsReview
	node.Confidence = ConfidenceLow
	node.Blockers = append(node.Blockers, Blocker{Code: "malformed_structure", Message: message})
	return node
}

func newStructureNode(runID, sourceID string, nodeType StructureNodeType, parentID string, structuralPath []string, start, end int, title, summary string, relatedSegmentIDs []string) StructureNode {
	if end < start {
		end = start
	}
	hash := "sha256:" + structureHash(sourceID, nodeType, structuralPath, start, end, title, summary)
	node := StructureNode{
		SchemaVersion:     StructureNodeSchemaVersion,
		RunID:             runID,
		SourceDocumentID:  sourceID,
		NodeType:          nodeType,
		ReviewStatus:      ReviewStatusReady,
		Confidence:        ConfidenceMedium,
		Title:             title,
		Summary:           summary,
		ParentNodeID:      parentID,
		ChildNodeIDs:      []string{},
		RelatedSegmentIDs: append([]string(nil), relatedSegmentIDs...),
		Evidence: StructureEvidence{
			SourceKind:        SourceKindMarkdown,
			SourceDocumentID:  sourceID,
			HeadingPath:       append([]string(nil), structuralPath...),
			LineStart:         start,
			LineEnd:           end,
			ContentHash:       hash,
			RelatedSegmentIDs: append([]string(nil), relatedSegmentIDs...),
		},
		Blockers: []Blocker{},
	}
	switch nodeType {
	case StructureNodeTypeUnknown:
		node.ReviewStatus = ReviewStatusNeedsReview
		node.Confidence = ConfidenceLow
	case StructureNodeTypeDocument, StructureNodeTypeSection, StructureNodeTypeTable, StructureNodeTypeTableRow, StructureNodeTypeTranscriptTurn:
		node.Confidence = ConfidenceMedium
	default:
		node.Confidence = ConfidenceHigh
	}
	node.NodeID = StructureNodeID(runID, sourceID, nodeType, structuralPath, start, end, hash)
	return ClassifyUnsafeStructureMarkers(node)
}

func BuildStructureSummary(runID string, sourceCount int, nodes []StructureNode) StructureSummary {
	finalized := buildStructureNodePaths(linkStructureChildren(finalizeStructureNodes(nodes)))
	summary := StructureSummary{
		SchemaVersion:  StructureSummarySchemaVersion,
		RunID:          runID,
		SourceCount:    sourceCount,
		NodeTypeCounts: map[StructureNodeType]int{},
	}
	for _, node := range finalized {
		summary.NodeCount++
		if node.ParentNodeID == "" {
			summary.RootNodeIDs = append(summary.RootNodeIDs, node.NodeID)
		}
		if node.ReviewStatus == ReviewStatusNeedsReview {
			summary.NeedsReviewCount++
		}
		if node.ReviewStatus == ReviewStatusBlocked {
			summary.BlockedCount++
		}
		summary.NodeTypeCounts[node.NodeType]++
		summary.Nodes = append(summary.Nodes, StructureSummaryNode{
			NodeID:           node.NodeID,
			SourceDocumentID: node.SourceDocumentID,
			NodeType:         node.NodeType,
			ReviewStatus:     node.ReviewStatus,
			Confidence:       node.Confidence,
			NodePath:         node.NodePath,
			PreviewPath:      StructureNodePreviewPath(node.NodeID),
		})
	}
	sort.Slice(summary.Nodes, func(i, j int) bool {
		left, right := summary.Nodes[i], summary.Nodes[j]
		return strings.Join([]string{left.SourceDocumentID, left.NodePath, string(left.NodeType), left.NodeID}, "\x00") < strings.Join([]string{right.SourceDocumentID, right.NodePath, string(right.NodeType), right.NodeID}, "\x00")
	})
	return summary
}

func WriteStructure(outDir, runID string, sourceCount int, nodes []StructureNode) error {
	if err := writeStructure(outDir, runID, sourceCount, nodes); err != nil {
		return ArtifactWriteError{Err: err}
	}
	return nil
}

func writeStructure(outDir, runID string, sourceCount int, nodes []StructureNode) error {
	if strings.TrimSpace(outDir) == "" {
		return fmt.Errorf("missing required --out")
	}
	nodes = buildStructureNodePaths(linkStructureChildren(finalizeStructureNodes(nodes)))
	if err := RejectDuplicateStructureNodeIDs(nodes); err != nil {
		return err
	}
	root, err := filepath.Abs(filepath.Join(outDir, "document-structure"))
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
	for _, node := range nodes {
		if err := ValidateStructureNode(node); err != nil {
			return err
		}
	}
	expectedFiles := map[string]bool{"structure-summary.json": true}
	for _, node := range nodes {
		expectedFiles[StructureNodeJSONPath(node.NodeID)] = true
		expectedFiles[StructureNodePreviewPath(node.NodeID)] = true
	}
	if err := rejectUnexpectedExistingFiles(realRoot, expectedFiles); err != nil {
		return err
	}
	summary := BuildStructureSummary(runID, sourceCount, nodes)
	if err := writeJSON(realRoot, "structure-summary.json", summary); err != nil {
		return err
	}
	for _, node := range nodes {
		if err := writeJSON(realRoot, StructureNodeJSONPath(node.NodeID), node); err != nil {
			return err
		}
		if err := writeFile(realRoot, StructureNodePreviewPath(node.NodeID), []byte(structurePreviewMarkdown(node))); err != nil {
			return err
		}
	}
	return nil
}

func finalizeStructureNodes(nodes []StructureNode) []StructureNode {
	out := make([]StructureNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, ClassifyUnsafeStructureMarkers(node))
	}
	return out
}

func ClassifyUnsafeStructureMarkers(node StructureNode) StructureNode {
	body := node.Title + "\n" + node.Summary + "\n" + node.NodePath + "\n" + strings.Join(node.Evidence.HeadingPath, "\n") + "\n" + node.SourceDocumentID + "\n" + strings.Join(node.RelatedSegmentIDs, "\n")
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		node.ReviewStatus = ReviewStatusBlocked
		node.Confidence = ConfidenceLow
		node.Title = "Unsafe content redacted"
		node.Summary = "Structure node content was redacted because it contains an unsafe marker."
		node.SourceDocumentID = redactedDocumentID(node.SourceDocumentID)
		node.Evidence.SourceDocumentID = node.SourceDocumentID
		for i := range node.Evidence.HeadingPath {
			node.Evidence.HeadingPath[i] = "Unsafe heading redacted"
		}
		node.RelatedSegmentIDs = []string{}
		node.Evidence.RelatedSegmentIDs = []string{}
		node.Blockers = append(node.Blockers, Blocker{Code: "unsafe_private_marker", Message: "Structure node contains an unsafe or private marker."})
	}
	return node
}

func containsGovernanceID(value string) bool {
	return governanceIDPattern.MatchString(value)
}

func buildStructureNodePaths(nodes []StructureNode) []StructureNode {
	index := map[string]int{}
	for i, node := range nodes {
		index[node.NodeID] = i
		nodes[i].NodePath = ""
	}
	var build func(int) string
	build = func(i int) string {
		slug := sanitizeID(nodes[i].Title)
		if slug == "segment" {
			slug = sanitizeID(string(nodes[i].NodeType))
		}
		if parentIndex, ok := index[nodes[i].ParentNodeID]; ok {
			nodes[i].NodePath = strings.Trim(build(parentIndex)+"/"+slug, "/")
		} else {
			nodes[i].NodePath = slug
		}
		return nodes[i].NodePath
	}
	for i := range nodes {
		build(i)
	}
	return nodes
}

func linkStructureChildren(nodes []StructureNode) []StructureNode {
	for i := range nodes {
		nodes[i].ChildNodeIDs = []string{}
	}
	index := map[string]int{}
	for i, node := range nodes {
		index[node.NodeID] = i
	}
	for _, node := range nodes {
		if node.ParentNodeID == "" {
			continue
		}
		if parentIndex, ok := index[node.ParentNodeID]; ok {
			nodes[parentIndex].ChildNodeIDs = append(nodes[parentIndex].ChildNodeIDs, node.NodeID)
		}
	}
	for i := range nodes {
		nodes[i].RelatedSegmentIDs = cloneStringList(nodes[i].RelatedSegmentIDs)
		nodes[i].Evidence.RelatedSegmentIDs = cloneStringList(nodes[i].Evidence.RelatedSegmentIDs)
	}
	return nodes
}

func cloneStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func ValidateStructureNode(node StructureNode) error {
	if node.SchemaVersion != StructureNodeSchemaVersion {
		return fmt.Errorf("unsupported structure node schema version: %s", node.SchemaVersion)
	}
	if strings.TrimSpace(node.NodeID) == "" {
		return fmt.Errorf("missing structure node id")
	}
	if sanitizeID(node.NodeID) != node.NodeID {
		return fmt.Errorf("unsafe structure node id: %s", node.NodeID)
	}
	if strings.TrimSpace(node.RunID) == "" {
		return fmt.Errorf("missing run id")
	}
	if strings.TrimSpace(node.SourceDocumentID) == "" {
		return fmt.Errorf("missing source document id")
	}
	if !validStructureNodeType(node.NodeType) {
		return fmt.Errorf("unsupported structure node type: %s", node.NodeType)
	}
	if !validReviewStatus(node.ReviewStatus) {
		return fmt.Errorf("unsupported review status: %s", node.ReviewStatus)
	}
	if !validConfidence(node.Confidence) {
		return fmt.Errorf("unsupported confidence: %s", node.Confidence)
	}
	if node.NodeType == StructureNodeTypeUnknown && node.ReviewStatus != ReviewStatusNeedsReview {
		return fmt.Errorf("unknown structure nodes must need review")
	}
	if node.Confidence == ConfidenceLow && node.ReviewStatus == ReviewStatusReady {
		return fmt.Errorf("low confidence structure nodes cannot be ready")
	}
	if node.ReviewStatus == ReviewStatusReady && (strings.TrimSpace(node.Title) == "" || strings.TrimSpace(node.Summary) == "") {
		return fmt.Errorf("ready structure nodes require title and summary")
	}
	if node.ReviewStatus == ReviewStatusReady && node.Evidence.LineStart <= 0 {
		return fmt.Errorf("ready structure nodes require provenance")
	}
	return nil
}

func RejectDuplicateStructureNodeIDs(nodes []StructureNode) error {
	seen := map[string]bool{}
	for _, node := range nodes {
		if strings.TrimSpace(node.NodeID) == "" {
			return fmt.Errorf("missing structure node id")
		}
		if seen[node.NodeID] {
			return fmt.Errorf("duplicate structure node id: %s", node.NodeID)
		}
		seen[node.NodeID] = true
	}
	return nil
}

func validStructureNodeType(value StructureNodeType) bool {
	switch value {
	case StructureNodeTypeDocument, StructureNodeTypeSection, StructureNodeTypeTable, StructureNodeTypeTableRow, StructureNodeTypeCapability, StructureNodeTypeTranscriptTurn, StructureNodeTypeAudience, StructureNodeTypeWorkflow, StructureNodeTypeRequirement, StructureNodeTypeUnknown:
		return true
	default:
		return false
	}
}

func structurePreviewMarkdown(node StructureNode) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(node.Title)
	b.WriteString("\n\n")
	b.WriteString("- Node: ")
	b.WriteString(node.NodeID)
	b.WriteString("\n")
	b.WriteString("- Type: ")
	b.WriteString(string(node.NodeType))
	b.WriteString("\n")
	b.WriteString("- Review status: ")
	b.WriteString(string(node.ReviewStatus))
	b.WriteString("\n")
	b.WriteString("- Confidence: ")
	b.WriteString(string(node.Confidence))
	b.WriteString("\n")
	b.WriteString("- Source document: ")
	b.WriteString(node.SourceDocumentID)
	b.WriteString("\n")
	b.WriteString("- Lines: ")
	b.WriteString(fmt.Sprintf("%d-%d", node.Evidence.LineStart, node.Evidence.LineEnd))
	b.WriteString("\n\n")
	b.WriteString(node.Summary)
	b.WriteString("\n")
	return b.String()
}

func tableBlocks(lines []line) []tableBlock {
	var blocks []tableBlock
	var current []line
	flush := func() {
		if len(current) == 0 {
			return
		}
		blocks = append(blocks, tableBlock{startLine: current[0].number, endLine: current[len(current)-1].number, rows: append([]line(nil), current...), malformed: !hasTableDelimiter(current)})
		current = nil
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line.text), "|") {
			current = append(current, line)
			continue
		}
		flush()
	}
	flush()
	return blocks
}

func hasTableDelimiter(rows []line) bool {
	for _, row := range rows {
		if isMarkdownTableDelimiter(strings.TrimSpace(row.text)) {
			return true
		}
	}
	return false
}

func dataRows(rows []line) []line {
	var out []line
	for i, row := range rows {
		text := strings.TrimSpace(row.text)
		if i == 0 || isMarkdownTableDelimiter(text) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func listItems(lines []line) []line {
	var out []line
	for _, item := range lines {
		text := strings.TrimSpace(item.text)
		if strings.HasPrefix(text, "- ") || strings.HasPrefix(text, "* ") {
			out = append(out, line{number: item.number, text: strings.TrimSpace(text[2:])})
		}
	}
	return out
}

func typedNodeFromText(text string) (StructureNodeType, bool) {
	if _, ok := capabilityCodeTitle(text); ok {
		return StructureNodeTypeCapability, true
	}
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "maybe") || strings.Contains(lower, "unclear") || strings.Contains(lower, "ambiguous"):
		return StructureNodeTypeUnknown, true
	case strings.HasPrefix(lower, "cap-") || strings.Contains(lower, "capability:") || strings.Contains(lower, "**capability"):
		return StructureNodeTypeCapability, true
	case strings.HasPrefix(lower, "req-") || strings.Contains(lower, "requirement:"):
		return StructureNodeTypeRequirement, true
	case strings.HasPrefix(lower, "wf-") || strings.Contains(lower, "workflow:"):
		return StructureNodeTypeWorkflow, true
	case strings.Contains(lower, "audience:") || strings.Contains(lower, "user group:"):
		return StructureNodeTypeAudience, true
	default:
		return "", false
	}
}

func typedNodeFromTitle(title, text string) (StructureNodeType, string, bool) {
	if capabilityTitle, ok := capabilityCodeTitle(title); ok {
		return StructureNodeTypeCapability, capabilityTitle, true
	}
	nodeType, ok := typedNodeFromText(text)
	if !ok {
		return "", "", false
	}
	return nodeType, listTitle(title), true
}

func listTitle(text string) string {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"Capability:", "Requirement:", "Workflow:", "Audience:", "User group:"} {
		if strings.HasPrefix(strings.ToLower(text), strings.ToLower(prefix)) {
			text = strings.TrimSpace(text[len(prefix):])
		}
	}
	text = strings.Trim(text, "* ")
	if len(text) > 72 {
		text = strings.TrimSpace(text[:72])
	}
	return strings.Trim(text, ".")
}

func capabilityCodeTitle(text string) (string, bool) {
	text = stripMarkdownEmphasis(strings.TrimSpace(text))
	matches := capabilityCodePattern.FindStringSubmatch(text)
	if matches == nil {
		return "", false
	}
	if !readableWordPattern.MatchString(matches[2]) {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func stripMarkdownEmphasis(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "__", "")
	text = strings.Trim(text, "*_ ")
	return strings.TrimSpace(text)
}

func segmentsForRange(segments []Segment, start, end int) []string {
	var ids []string
	for _, segment := range segments {
		if segment.Evidence.LineEnd < start || segment.Evidence.LineStart > end {
			continue
		}
		ids = append(ids, segment.SegmentID)
	}
	return ids
}

func orderStructureNodes(nodes []StructureNode) []StructureNode {
	sort.SliceStable(nodes, func(i, j int) bool {
		left, right := nodes[i], nodes[j]
		return strings.Join([]string{left.SourceDocumentID, fmt.Sprintf("%06d", left.Evidence.LineStart), left.NodePath, string(left.NodeType), left.NodeID}, "\x00") < strings.Join([]string{right.SourceDocumentID, fmt.Sprintf("%06d", right.Evidence.LineStart), right.NodePath, string(right.NodeType), right.NodeID}, "\x00")
	})
	return nodes
}

func documentTitle(path string, sections []section) string {
	for _, section := range sections {
		if section.sourceHeadingLevel == 1 && len(section.headingPath) > 0 {
			return section.headingPath[0]
		}
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func documentHasH1(sections []section) bool {
	for _, section := range sections {
		if section.sourceHeadingLevel == 1 {
			return true
		}
	}
	return false
}

func maxLineNumber(sections []section) int {
	max := 1
	for _, section := range sections {
		_, end := sectionLineRange(section)
		if end > max {
			max = end
		}
	}
	return max
}

func sectionLineRange(section section) (int, int) {
	if len(section.lines) == 0 {
		if section.headingLine > 0 {
			return section.headingLine, section.headingLine
		}
		return 1, 1
	}
	start := section.lines[0].number
	if section.headingLine > 0 && section.headingLine < start {
		start = section.headingLine
	}
	return start, section.lines[len(section.lines)-1].number
}

func structureHash(sourceID string, nodeType StructureNodeType, path []string, start, end int, title, summary string) string {
	value := strings.Join([]string{sourceID, string(nodeType), strings.Join(path, "/"), fmt.Sprintf("%d-%d", start, end), title, summary}, "\n")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
