package slack

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/assurance"
)

func TestNativeBatchOwnsOccurrenceDerivationNotConnector(t *testing.T) {
	batch := NativeBatch{
		SchemaVersion: NativeBatchSchema, WorkspaceID: "T1", ChannelID: "D1",
		LowerInclusive: "1.000001", UpperInclusive: "2.000001", Watermark: "2.000001",
		IncludeThreads: true, IncludeReplies: true, PaginationExhausted: true, ThreadPaginationExhausted: true,
		DeclaredSourceRecords: 2,
		Messages: []NativeMessage{
			{NativeMessageID: "1.000001", Timestamp: "1.000001", Text: "https://example.com/post"},
			{NativeMessageID: "2.000001", Timestamp: "2.000001", Text: "duplicate https://example.com/post"},
		},
	}
	if err := validateNativeBatch(batch); err != nil {
		t.Fatal(err)
	}
	batch.DeclaredSourceRecords = 1
	if err := validateNativeBatch(batch); err == nil {
		t.Fatal("connector source-record denominator mismatch was accepted")
	}
}

func TestNativeBatchRejectsAuthorityBeforePrivateValidation(t *testing.T) {
	_, err := BuildAuthorizedExternalManifestFromNativeBatch(
		NativeBatch{Messages: []NativeMessage{{Text: "private-sentinel"}}},
		assurance.Receipt{}, "commit", "configuration",
	)
	if err == nil || !strings.Contains(err.Error(), "authority") || strings.Contains(err.Error(), "private-sentinel") {
		t.Fatalf("private batch crossed authority boundary: %v", err)
	}
}

func TestNativeBatchUsesNumericSlackTimestampWindow(t *testing.T) {
	batch := NativeBatch{
		SchemaVersion: NativeBatchSchema, WorkspaceID: "T1", ChannelID: "D1",
		LowerInclusive: "10.000001", UpperInclusive: "20.000001", Watermark: "20.000001",
		IncludeThreads: true, IncludeReplies: true, PaginationExhausted: true, ThreadPaginationExhausted: true,
		DeclaredSourceRecords: 3,
		Messages: []NativeMessage{
			{NativeMessageID: "10", Timestamp: "10.000001", Text: "https://example.com/one"},
			{NativeMessageID: "100", Timestamp: "100.000001", Text: "https://example.com/outside"},
			{NativeMessageID: "20", Timestamp: "20.000001", Text: "https://example.com/two"},
		},
	}
	if err := validateNativeBatch(batch); err == nil {
		t.Fatal("numeric timestamp outside the declared window was accepted")
	}
}

func TestPublishedNativeBatchExampleMatchesContract(t *testing.T) {
	document, err := os.ReadFile("../../../docs/native-slack-batch-v1.md")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(document), "```json\n")
	if start < 0 {
		t.Fatal("published native-batch JSON example is missing")
	}
	start += len("```json\n")
	end := strings.Index(string(document)[start:], "\n```")
	if end < 0 {
		t.Fatal("published native-batch JSON example is unterminated")
	}
	var batch NativeBatch
	if err := json.Unmarshal(document[start:start+end], &batch); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeBatch(batch); err != nil {
		t.Fatalf("published native-batch example violates its contract: %v", err)
	}
}
