package personalmemory

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMixedSecretCaptureKeepsSafeLessonSearchable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	repository, err := NewFileRepository(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err := NewCaptureRecord(CaptureRecordInput{
		SourceAdapter: "slack", SourceScopeID: "T-test", SourceContainerID: "D-test",
		ExternalID: "1.000001", OccurredAt: "2026-07-27T12:00:00Z",
		SourceRef: "slack://T-test/D-test/1.000001",
		RawText:   "useful product lesson password=synthetic-private-value keep this explanation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(record.RawText, "useful product lesson") ||
		!strings.Contains(record.RawText, "keep this explanation") ||
		strings.Contains(record.RawText, "synthetic-private-value") {
		t.Fatalf("canonical field-level redaction = %q", record.RawText)
	}
	batch, err := NewCaptureBatch(CaptureBatchInput{
		SourceIdentity: "slack:T-test:D-test",
		LowerInclusive: "1.000001", UpperInclusive: "1.000001", Watermark: "1.000001",
		DeclaredRecords: 1, Records: []CaptureRecord{record},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Import(batch); err != nil {
		t.Fatal(err)
	}
	packet, err := NewLexicalRetriever(repository).SearchCompact(SearchRequest{
		Query: "useful product lesson", Limit: 3,
	})
	if err != nil || packet.AnswerState != "answered" || len(packet.Citations) != 1 ||
		!strings.Contains(packet.Citations[0].Snippet, "useful product lesson") ||
		strings.Contains(packet.Citations[0].Snippet, "synthetic-private-value") {
		t.Fatalf("safe surrounding lesson was not searchable: %#v, %v", packet, err)
	}
}
