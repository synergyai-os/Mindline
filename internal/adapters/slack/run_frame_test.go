package slack

import (
	"encoding/json"
	"strings"
	"testing"

	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
)

func TestUserSavedIntentIsRetainedAfterCanonicalFieldLevelSanitization(t *testing.T) {
	messages := []acquisitionslack.NativeMessage{
		{NativeMessageID: "1.000001", Timestamp: "1.000001", AuthorID: "U-user", Text: "read https://example.com/article?utm_source=slack"},
		{NativeMessageID: "1.000002", Timestamp: "1.000002", AuthorID: "U-user", Text: "read https://example.com/private?token=synthetic-private-value"},
		{NativeMessageID: "1.000003", Timestamp: "1.000003", AuthorID: "U-user", Text: "useful product lesson password=synthetic-private-value keep this explanation"},
	}
	frame := RunFrame{
		Descriptor: "saved-intent",
		Batch: acquisitionslack.NativeBatch{
			SchemaVersion: acquisitionslack.NativeBatchSchema,
			WorkspaceID:   "T-test", ChannelID: "D-test",
			LowerInclusive: "1.000001", UpperInclusive: "1.000003", Watermark: "1.000003",
			IncludeThreads: true, IncludeReplies: true, PaginationExhausted: true, ThreadPaginationExhausted: true,
			DeclaredSourceRecords: len(messages), Messages: messages,
		},
		AuthorClasses: map[string]string{"1.000001": "user", "1.000002": "user", "1.000003": "user"},
	}
	capture, dispositions, err := CaptureBatchForAdoption(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.Records) != 3 {
		t.Fatalf("retained records = %d", len(capture.Records))
	}
	for _, disposition := range dispositions {
		if disposition != DispositionRetain {
			t.Fatalf("user-authored saved intent was withheld: %q", disposition)
		}
	}
	encoded, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	for _, forbidden := range []string{"utm_source", "synthetic-private-value", "token="} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("unsafe field survived canonical sanitization: %s", forbidden)
		}
	}
	if !strings.Contains(capture.Records[0].RawText, "https://example.com/article") ||
		!strings.Contains(capture.Records[1].RawText, "[mindline-sensitive-url-redacted]") ||
		capture.Records[2].ContextState != "secret_redacted" ||
		!strings.Contains(capture.Records[2].RawText, "useful product lesson") ||
		!strings.Contains(capture.Records[2].RawText, "keep this explanation") {
		t.Fatalf("canonical redaction projection is incomplete: %#v", capture.Records)
	}
}
