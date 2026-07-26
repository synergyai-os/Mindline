package slack

import (
	"strings"
	"testing"

	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func TestCaptureBatchFromNativeMapsSlackIntoSourceNeutralContract(t *testing.T) {
	native := acquisitionslack.NativeBatch{
		SchemaVersion: acquisitionslack.NativeBatchSchema,
		WorkspaceID:   "T-source", ChannelID: "D-capture",
		LowerInclusive: "1784902473.012269", UpperInclusive: "1784902473.012269",
		Watermark: "1784902473.012269", IncludeThreads: true, IncludeReplies: true,
		PaginationExhausted: true, ThreadPaginationExhausted: true, DeclaredSourceRecords: 1,
		Messages: []acquisitionslack.NativeMessage{{
			NativeMessageID: "1784902473.012269", Timestamp: "1784902473.012269",
			AuthorName: "Randy", Text: "saved https://example.com/context?utm_source=slack",
		}},
	}
	batch, err := CaptureBatchFromNative(native)
	if err != nil {
		t.Fatal(err)
	}
	if batch.SchemaVersion != personalmemory.CaptureBatchSchemaVersion ||
		batch.SourceIdentity != "slack:T-source:D-capture" ||
		len(batch.Records) != 1 {
		t.Fatalf("unexpected source-neutral batch: %+v", batch)
	}
	record := batch.Records[0]
	if record.SourceAdapter != "slack" || record.SourceScopeID != "T-source" ||
		record.SourceContainerID != "D-capture" ||
		!strings.HasPrefix(record.SourceRef, "slack://") ||
		len(record.URLs) != 1 || strings.Contains(record.URLs[0], "utm_source") {
		t.Fatalf("Slack facts were not safely normalized: %+v", record)
	}
}
