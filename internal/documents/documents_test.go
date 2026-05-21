package documents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGoldenMarkdownDecomposition(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		minCount   int
		typeCounts map[SemanticType]int
		status     map[ReviewStatus]int
	}{
		{
			name:     "transcript decision action",
			file:     "transcript-decision-action.md",
			minCount: 6,
			typeCounts: map[SemanticType]int{
				SemanticTypeMeetingNote: 1,
				SemanticTypeDecision:    1,
				SemanticTypeAction:      2,
				SemanticTypeTension:     1,
			},
		},
		{
			name:     "mixed thread capture",
			file:     "mixed-thread-capture.md",
			minCount: 7,
			typeCounts: map[SemanticType]int{
				SemanticTypeSourceNote: 1,
				SemanticTypeReference:  1,
				SemanticTypeAction:     1,
				SemanticTypeInsight:    1,
				SemanticTypeUnknown:    1,
			},
			status: map[ReviewStatus]int{
				ReviewStatusNeedsReview: 1,
				ReviewStatusBlocked:     1,
			},
		},
		{
			name:     "strategy capability table",
			file:     "strategy-capability-table.md",
			minCount: 7,
			typeCounts: map[SemanticType]int{
				SemanticTypeStandard:  2,
				SemanticTypeDecision:  1,
				SemanticTypeReference: 2,
				SemanticTypeWorkItem:  1,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			summary, err := DecomposePath(fixturePath(t, "markdown", tc.file), out)
			if err != nil {
				t.Fatalf("decompose: %v", err)
			}
			if summary.SchemaVersion != SegmentSummarySchemaVersion {
				t.Fatalf("unexpected schema: %s", summary.SchemaVersion)
			}
			if summary.SegmentCount < tc.minCount {
				t.Fatalf("expected at least %d segments, got %d", tc.minCount, summary.SegmentCount)
			}
			for semanticType, min := range tc.typeCounts {
				if got := summary.TypeCounts[semanticType]; got < min {
					t.Fatalf("expected at least %d %s segments, got %d in %+v", min, semanticType, got, summary.TypeCounts)
				}
			}
			for status, min := range tc.status {
				if got := countStatus(t, out, summary, status); got < min {
					t.Fatalf("expected at least %d %s segments, got %d", min, status, got)
				}
			}
		})
	}
}

func TestGeneratedOutputMatchesGoldenFixtures(t *testing.T) {
	cases := []string{
		"transcript-decision-action",
		"mixed-thread-capture",
		"strategy-capability-table",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			out := t.TempDir()
			if _, err := DecomposePath(fixturePath(t, "markdown", name+".md"), out); err != nil {
				t.Fatalf("decompose: %v", err)
			}
			assertTreeMatches(t,
				filepath.Join(out, "document-segments"),
				fixturePath(t, "expected", name, "document-segments"),
			)
		})
	}
}

func TestDocumentsDecomposeWritesCompleteArtifactTree(t *testing.T) {
	out := t.TempDir()
	summary, err := DecomposePath(fixturePath(t, "markdown", "mixed-thread-capture.md"), out)
	if err != nil {
		t.Fatalf("decompose: %v", err)
	}
	root := filepath.Join(out, "document-segments")
	if _, err := os.Stat(filepath.Join(root, "segment-summary.json")); err != nil {
		t.Fatalf("missing summary: %v", err)
	}
	referenced := map[string]bool{"segment-summary.json": true}
	for _, item := range summary.Segments {
		for _, rel := range []string{item.SegmentPath, item.PreviewPath} {
			if rel == "" {
				t.Fatalf("missing path in summary item: %+v", item)
			}
			if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
				t.Fatalf("missing referenced artifact %s: %v", rel, err)
			}
			referenced[filepath.ToSlash(rel)] = true
		}
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !referenced[filepath.ToSlash(rel)] {
			t.Fatalf("unreferenced generated file: %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk output: %v", err)
	}
}

func TestWriterRejectsUnexpectedExistingFile(t *testing.T) {
	out := t.TempDir()
	stale := filepath.Join(out, "document-segments", "segments", "stale.json")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatalf("mkdir stale parent: %v", err)
	}
	if err := os.WriteFile(stale, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	if err := Write(out, Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected stale generated file rejection")
	}
}

func TestWriterRedactsUnsafeProvidedSegment(t *testing.T) {
	out := t.TempDir()
	segment := validSegment()
	segment.Title = "PRIVATE_CONTENT ready segment"
	segment.Summary = "secret token body must not persist"
	segment.Evidence.HeadingPath = []string{"SECRET heading"}
	segment.SourceDocumentID = "doc-secret-token"
	if err := Write(out, Summary{RunID: segment.RunID, SourceCount: 1}, []Segment{segment}); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "private_content", "secret", "token")
}

func TestWriterRebuildsSummaryFromFinalizedSegments(t *testing.T) {
	out := t.TempDir()
	segment := validSegment()
	segment.Title = "PRIVATE_CONTENT ready segment"
	segment.Summary = "secret token body must not persist"
	segment.SourceDocumentID = "doc-secret-token"
	summary := BuildSummary(segment.RunID, 1, []Segment{segment})
	if err := Write(out, summary, []Segment{segment}); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "private_content", "secret", "token", "doc-secret-token")
}

func TestUnsupportedSchema(t *testing.T) {
	segment := Segment{SchemaVersion: "document-segment/v9"}
	if err := ValidateSegment(segment); err == nil {
		t.Fatalf("expected unsupported schema error")
	}
}

func TestReviewStatusConfidence(t *testing.T) {
	unknown := validSegment()
	unknown.SemanticType = SemanticTypeUnknown
	unknown.ReviewStatus = ReviewStatusReady
	if err := ValidateSegment(unknown); err == nil {
		t.Fatalf("expected unknown ready segment to fail validation")
	}
	low := validSegment()
	low.Confidence = ConfidenceLow
	low.ReviewStatus = ReviewStatusReady
	if err := ValidateSegment(low); err == nil {
		t.Fatalf("expected low confidence ready segment to fail validation")
	}
}

func TestUnsafePrivateContentMarkerBlocksSegment(t *testing.T) {
	segment := ClassifyUnsafeMarkers(Segment{
		SchemaVersion:    SegmentSchemaVersion,
		SegmentID:        "seg-private-marker",
		RunID:            "run-doc-private-marker",
		SourceDocumentID: "doc-private-marker",
		SourceKind:       SourceKindMarkdown,
		SemanticType:     SemanticTypeSourceNote,
		ReviewStatus:     ReviewStatusReady,
		Confidence:       ConfidenceHigh,
		Title:            "Synthetic private marker",
		Summary:          "PRIVATE_CONTENT marker should fail closed.",
		Evidence: Evidence{
			Kind:        EvidenceKindLocation,
			HeadingPath: []string{"Private marker sample"},
			LineStart:   1,
			LineEnd:     1,
			ContentHash: "sha256:abc",
		},
	})
	if segment.ReviewStatus != ReviewStatusBlocked {
		t.Fatalf("expected blocked, got %s", segment.ReviewStatus)
	}
	if len(segment.Blockers) == 0 {
		t.Fatalf("expected blocker reason")
	}
	body := strings.ToLower(segment.Title + "\n" + segment.Summary)
	for _, forbidden := range []string{"private_content", "secret", "token"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("expected unsafe marker redaction, found %q in %s", forbidden, body)
		}
	}
}

func TestUnsafeMarkerGeneratedArtifactsAreRedacted(t *testing.T) {
	out := t.TempDir()
	if _, err := DecomposePath(fixturePath(t, "markdown", "mixed-thread-capture.md"), out); err != nil {
		t.Fatalf("decompose: %v", err)
	}
	files := readTree(t, filepath.Join(out, "document-segments"))
	for path, body := range files {
		lower := strings.ToLower(body)
		for _, forbidden := range []string{"private_content", "must block downstream flow"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("generated artifact %s leaked unsafe marker content %q:\n%s", path, forbidden, body)
			}
		}
	}
}

func TestUnsafeMarkerTableArtifactsAreRedacted(t *testing.T) {
	out := t.TempDir()
	segments := decomposeSection("run-doc-unsafe-table", "doc-unsafe-table", section{
		headingPath: []string{"Reference table"},
		lines: []line{
			{number: 1, text: "| Topic | Detail |"},
			{number: 2, text: "| --- | --- |"},
			{number: 3, text: "| PRIVATE_CONTENT row | token value must not persist |"},
		},
	})
	if len(segments) == 0 {
		t.Fatalf("expected table segments")
	}
	if err := Write(out, BuildSummary("run-doc-unsafe-table", 1, segments), segments); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "private_content", "token value must not persist")
}

func TestUnsafeMarkerHeadingArtifactsAreRedacted(t *testing.T) {
	out := t.TempDir()
	segments := []Segment{
		newSegment("run-doc-unsafe-heading", "doc-unsafe-heading", []string{"SECRET roadmap heading"}, 1, 1, SemanticTypeSourceNote, ReviewStatusReady, ConfidenceMedium, "Safe body", "Safe body"),
	}
	if err := Write(out, BuildSummary("run-doc-unsafe-heading", 1, segments), segments); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "secret roadmap heading")
}

func TestUnsafeMarkerFilenameArtifactsAreRedacted(t *testing.T) {
	input := filepath.Join(t.TempDir(), "secret-token.md")
	if err := os.WriteFile(input, []byte("# Safe heading\n\nSafe body.\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	out := t.TempDir()
	if _, err := DecomposePath(input, out); err != nil {
		t.Fatalf("decompose: %v", err)
	}
	assertGeneratedTreeExcludes(t, filepath.Join(out, "document-segments"), "secret", "token")
}

func TestDirectoryInputsDisambiguateDuplicateBasenames(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"alpha", "beta"} {
		path := filepath.Join(root, dir, "notes.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir input parent: %v", err)
		}
		if err := os.WriteFile(path, []byte("# Decisions\n\nDecision: Ship the staged release.\n"), 0o644); err != nil {
			t.Fatalf("write input: %v", err)
		}
	}
	out := t.TempDir()
	summary, err := DecomposePath(root, out)
	if err != nil {
		t.Fatalf("decompose duplicate basenames: %v", err)
	}
	if summary.SourceCount != 2 || summary.SegmentCount != 2 {
		t.Fatalf("unexpected summary counts: %+v", summary)
	}
	sourceIDs := map[string]bool{}
	for _, item := range summary.Segments {
		sourceIDs[item.SourceDocumentID] = true
	}
	if len(sourceIDs) != 2 {
		t.Fatalf("expected distinct source document ids, got %+v", sourceIDs)
	}
}

func TestDecomposePathRejectsMarkdownScannerErrors(t *testing.T) {
	input := filepath.Join(t.TempDir(), "long-line.md")
	longLine := strings.Repeat("a", 1024*1024)
	if err := os.WriteFile(input, []byte("# Oversized\n\n"+longLine+"\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if _, err := DecomposePath(input, t.TempDir()); err == nil {
		t.Fatalf("expected scanner error for oversized markdown line")
	}
}

func TestUncertaintyMarkersTakePrecedenceOverActionHeuristics(t *testing.T) {
	segment := segmentFromText("run-doc-demo", "doc-demo", []string{"Open questions"}, 3, 3, "Maybe we need to revisit this")
	if segment.SemanticType != SemanticTypeUnknown {
		t.Fatalf("expected unknown semantic type, got %s", segment.SemanticType)
	}
	if segment.ReviewStatus != ReviewStatusNeedsReview {
		t.Fatalf("expected needs_review status, got %s", segment.ReviewStatus)
	}
}

func TestParseSectionsPreservesHeadingHierarchy(t *testing.T) {
	sections, err := parseSections("# Strategy\n\nIntro note.\n\n## Risks\n\nRisk: ambiguous provenance.\n\n### Follow up\n\nAction: map nested headings.\n")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d: %+v", len(sections), sections)
	}
	assertHeadingPath(t, sections[0].headingPath, []string{"Strategy"})
	assertHeadingPath(t, sections[1].headingPath, []string{"Strategy", "Risks"})
	assertHeadingPath(t, sections[2].headingPath, []string{"Strategy", "Risks", "Follow up"})
}

func TestMixedTableSectionKeepsNonTableSegments(t *testing.T) {
	segments := decomposeSection("run-doc-demo", "doc-demo", section{
		headingPath: []string{"Capability review"},
		lines: []line{
			{number: 3, text: "Decision: keep local segment artifacts destination-neutral."},
			{number: 5, text: "| Capability | Status |"},
			{number: 6, text: "| --- | --- |"},
			{number: 7, text: "| Segment writer | Ready |"},
			{number: 9, text: "Action: validate downstream proposal adapters separately."},
		},
	})
	if len(segments) < 4 {
		t.Fatalf("expected table and non-table segments, got %d: %+v", len(segments), segments)
	}
	assertHasSemanticType(t, segments, SemanticTypeDecision)
	assertHasSemanticType(t, segments, SemanticTypeAction)
	assertHasSemanticType(t, segments, SemanticTypeReference)
}

func TestTableRowsMayContainDashesAsData(t *testing.T) {
	segments := decomposeTable("run-doc-demo", "doc-demo", section{
		headingPath: []string{"Version table"},
		lines: []line{
			{number: 1, text: "| Name | Range |"},
			{number: 2, text: "| --- | --- |"},
			{number: 3, text: "| Parser | v1---v2 |"},
		},
	})
	if len(segments) != 2 {
		t.Fatalf("expected table-level plus dashed data row segments, got %d: %+v", len(segments), segments)
	}
	if segments[1].Title != "Parser" || !strings.Contains(segments[1].Summary, "v1---v2") {
		t.Fatalf("expected dashed data row to be preserved, got %+v", segments[1])
	}
}

func TestParseSectionsIgnoresFencedCodeBlocks(t *testing.T) {
	sections, err := parseSections("# Notes\n\nDecision: keep semantic prose.\n\n```go\n// Decision: code sample must not become a segment.\nfmt.Println(\"Action: skip code\")\n```\n\nAction: process real prose.\n")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d: %+v", len(sections), sections)
	}
	segments := decomposeSection("run-doc-demo", "doc-demo", sections[0])
	if len(segments) != 2 {
		t.Fatalf("expected only prose segments, got %d: %+v", len(segments), segments)
	}
	for _, segment := range segments {
		if strings.Contains(strings.ToLower(segment.Summary), "code sample") || strings.Contains(strings.ToLower(segment.Summary), "fmt.println") {
			t.Fatalf("code block content became a segment: %+v", segment)
		}
	}
}

func TestParseSectionsRequiresValidATXHeading(t *testing.T) {
	sections, err := parseSections("# Notes\n\n#123 should remain prose.\n#include should remain prose too.\n## Follow up\n\nAction: keep valid headings.\n")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("expected 2 valid heading sections, got %d: %+v", len(sections), sections)
	}
	if len(sections[0].lines) != 2 {
		t.Fatalf("expected invalid heading markers to remain prose, got %+v", sections[0].lines)
	}
	assertHeadingPath(t, sections[1].headingPath, []string{"Notes", "Follow up"})
}

func TestDocumentSegmentHasNoDestinationHints(t *testing.T) {
	data, err := json.Marshal(validSegment())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{
		"destination_hints",
		"surface",
		"product" + "brain",
		"no" + "tion",
		"ob" + "sidian",
		"to" + "laria",
	} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Fatalf("segment JSON contains destination-specific value %q: %s", forbidden, string(data))
		}
	}
}

func TestDocumentsPackageDoesNotImportProductBrain(t *testing.T) {
	root := repoRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "internal", "documents"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		forbidden := "internal/" + "productbrain"
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("documents package imports Product Brain code in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk documents package: %v", err)
	}
}

func TestSegmentID(t *testing.T) {
	got := SegmentID("run-doc-demo", "doc-demo", []string{"Actions"}, 12, "Product Lead to prepare checklist.")
	if got != "seg-57061a0612ea70f1" {
		t.Fatalf("unexpected segment id: %s", got)
	}
}

func TestSourceDocumentIDRedactsUnsafeFilename(t *testing.T) {
	got := SourceDocumentID("/tmp/secret-token.md")
	if strings.Contains(got, "secret") || strings.Contains(got, "token") {
		t.Fatalf("expected redacted source document id, got %s", got)
	}
	if !strings.HasPrefix(got, "doc-redacted-") {
		t.Fatalf("expected redacted source document id prefix, got %s", got)
	}
}

func TestSegmentPath(t *testing.T) {
	if got := SegmentJSONPath("seg-demo"); got != "segments/seg-demo.json" {
		t.Fatalf("unexpected segment path: %s", got)
	}
	if got := SegmentPreviewPath("seg-demo"); got != "previews/seg-demo.md" {
		t.Fatalf("unexpected preview path: %s", got)
	}
}

func TestDuplicateSegmentID(t *testing.T) {
	segments := []Segment{validSegment(), validSegment()}
	if err := RejectDuplicateSegmentIDs(segments); err == nil {
		t.Fatalf("expected duplicate segment id error")
	}
}

func TestWriterRejectsTraversal(t *testing.T) {
	out := t.TempDir()
	segment := validSegment()
	segment.SegmentID = "../escape"
	if err := Write(out, Summary{RunID: segment.RunID}, []Segment{segment}); err == nil {
		t.Fatalf("expected traversal write error")
	}
}

func TestWriterRejectsSymlink(t *testing.T) {
	out := t.TempDir()
	root := filepath.Join(out, "document-segments")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "segments")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := Write(out, Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected symlink write error")
	}
}

func TestWriterRejectsSymlinkedOutRoot(t *testing.T) {
	base := t.TempDir()
	realOut := filepath.Join(base, "real")
	linkOut := filepath.Join(base, "link")
	if err := os.Mkdir(realOut, 0o755); err != nil {
		t.Fatalf("mkdir real out: %v", err)
	}
	if err := os.Symlink(realOut, linkOut); err != nil {
		t.Fatalf("symlink out: %v", err)
	}
	if err := Write(linkOut, Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected symlinked --out rejection")
	}
}

func TestWriterRejectsSymlinkedOutParent(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	linkParent := filepath.Join(base, "link-parent")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, linkParent); err != nil {
		t.Fatalf("symlink parent: %v", err)
	}
	if err := Write(filepath.Join(linkParent, "generated"), Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected symlinked --out parent rejection")
	}
}

func TestWriterRejectsExistingOutUnderSymlinkedParent(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	linkParent := filepath.Join(base, "link-parent")
	if err := os.MkdirAll(filepath.Join(outside, "generated"), 0o755); err != nil {
		t.Fatalf("mkdir existing out: %v", err)
	}
	if err := os.Symlink(outside, linkParent); err != nil {
		t.Fatalf("symlink parent: %v", err)
	}
	if err := Write(filepath.Join(linkParent, "generated"), Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected existing --out under symlinked parent rejection")
	}
}

func TestWriterRejectsSymlinkedDocumentSegmentsRoot(t *testing.T) {
	out := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(out, "document-segments")); err != nil {
		t.Fatalf("symlink document-segments root: %v", err)
	}
	if err := Write(out, Summary{RunID: "run-doc-demo"}, []Segment{validSegment()}); err == nil {
		t.Fatalf("expected symlinked document-segments root rejection")
	}
}

func validSegment() Segment {
	return Segment{
		SchemaVersion:    SegmentSchemaVersion,
		SegmentID:        "seg-demo",
		RunID:            "run-doc-demo",
		SourceDocumentID: "doc-demo",
		SourceKind:       SourceKindMarkdown,
		SemanticType:     SemanticTypeAction,
		ReviewStatus:     ReviewStatusReady,
		Confidence:       ConfidenceMedium,
		Title:            "Prepare checklist",
		Summary:          "Product Lead should prepare the release checklist.",
		Evidence: Evidence{
			Kind:        EvidenceKindLocation,
			HeadingPath: []string{"Actions"},
			LineStart:   12,
			LineEnd:     12,
			ContentHash: "sha256:abc",
		},
		Blockers: []Blocker{},
	}
}

func countStatus(t *testing.T, out string, summary Summary, status ReviewStatus) int {
	t.Helper()
	count := 0
	for _, item := range summary.Segments {
		data, err := os.ReadFile(filepath.Join(out, "document-segments", item.SegmentPath))
		if err != nil {
			t.Fatalf("read segment %s: %v", item.SegmentPath, err)
		}
		var segment Segment
		if err := json.Unmarshal(data, &segment); err != nil {
			t.Fatalf("decode segment %s: %v", item.SegmentPath, err)
		}
		if segment.ReviewStatus == status {
			count++
		}
	}
	return count
}

func assertTreeMatches(t *testing.T, actualRoot, expectedRoot string) {
	t.Helper()
	actual := readTree(t, actualRoot)
	expected := readTree(t, expectedRoot)
	if len(actual) != len(expected) {
		t.Fatalf("file count mismatch actual=%d expected=%d\nactual=%v\nexpected=%v", len(actual), len(expected), keys(actual), keys(expected))
	}
	for path, expectedBody := range expected {
		actualBody, ok := actual[path]
		if !ok {
			t.Fatalf("missing generated file %s", path)
		}
		if actualBody != expectedBody {
			t.Fatalf("golden mismatch for %s\nactual:\n%s\nexpected:\n%s", path, actualBody, expectedBody)
		}
	}
}

func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("read tree %s: %v", root, err)
	}
	return files
}

func assertGeneratedTreeExcludes(t *testing.T, root string, forbidden ...string) {
	t.Helper()
	files := readTree(t, root)
	for path, body := range files {
		lower := strings.ToLower(body)
		for _, value := range forbidden {
			if strings.Contains(lower, strings.ToLower(value)) {
				t.Fatalf("generated artifact %s leaked %q:\n%s", path, value, body)
			}
		}
	}
}

func assertHeadingPath(t *testing.T, got []string, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected heading path got=%+v want=%+v", got, want)
	}
}

func assertHasSemanticType(t *testing.T, segments []Segment, semanticType SemanticType) {
	t.Helper()
	for _, segment := range segments {
		if segment.SemanticType == semanticType {
			return
		}
	}
	t.Fatalf("expected semantic type %s in %+v", semanticType, segments)
}

func keys(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}

func fixturePath(t *testing.T, parts ...string) string {
	t.Helper()
	all := append([]string{repoRoot(t), "testdata", "documents"}, parts...)
	return filepath.Join(all...)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("cannot resolve caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
