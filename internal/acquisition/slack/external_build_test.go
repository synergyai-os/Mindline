package slack

import (
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/assurance"
)

func TestBuildExternalManifestPreservesDuplicateOccurrencesAndClassifiesFormats(t *testing.T) {
	manifest, err := BuildExternalManifest(BuildInput{
		WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000002.000001", Watermark: "1700000002.000001", DataClass: DataClassSynthetic,
		Messages: []NativeMessage{
			{NativeMessageID: "m-1", Timestamp: "1700000000.000001", Text: "<https://www.linkedin.com/posts/example_activity-123?utm_source=slack|post> https://www.linkedin.com/posts/example_activity-123?utm_source=slack"},
			{NativeMessageID: "m-2", Timestamp: "1700000001.000001", Text: "https://youtu.be/video123 https://github.com/org/repo https://github.com/org/repo/issues/4 https://open.spotify.com/episode/abc https://writer.substack.com/p/example https://example.com/report.pdf"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.SourceRecords) != 2 || len(manifest.URLOccurrences) != 8 || len(manifest.CanonicalItems) != 7 {
		t.Fatalf("denominators changed: records=%d occurrences=%d canonical=%d", len(manifest.SourceRecords), len(manifest.URLOccurrences), len(manifest.CanonicalItems))
	}
	formats := map[string]bool{}
	for _, item := range manifest.CanonicalItems {
		formats[item.Format] = true
		if item.Format == "linkedin_post" && len(item.URLOccurrenceIDs) != 2 {
			t.Fatalf("duplicate LinkedIn occurrences collapsed: %+v", item)
		}
	}
	for _, expected := range []string{"linkedin_post", "youtube_video_short_link", "github_repository", "github_issue", "spotify_episode", "substack_post", "pdf_document"} {
		if !formats[expected] {
			t.Fatalf("missing format %s in %+v", expected, formats)
		}
	}
	if _, err := ValidateExternalManifest(manifest); err != nil {
		t.Fatal(err)
	}
}

func TestExtractURLOccurrencesHandlesSlackLabelsAndPunctuation(t *testing.T) {
	urls := ExtractURLOccurrences("see <https://example.com/a?x=1|label>, then https://example.com/b).")
	if len(urls) != 2 || urls[0] != "https://example.com/a?x=1" || urls[1] != "https://example.com/b" {
		t.Fatalf("unexpected extracted occurrences: %#v", urls)
	}
}

func TestPrivateManifestBuildRequiresCommitBoundReceipt(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	input := BuildInput{WorkspaceID: "T-private", ChannelID: "C-private", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001", DataClass: DataClassPrivateRuntime, Messages: []NativeMessage{{NativeMessageID: "m-1", Timestamp: "1700000000.000001", Text: "https://example.com"}}}
	if _, err := BuildExternalManifest(input); err == nil {
		t.Fatal("private Slack data crossed the ungated builder")
	}
	checks := make([]assurance.Check, 0, len(assurance.RequiredChecks))
	for _, name := range assurance.RequiredChecks {
		checks = append(checks, assurance.Check{Name: name, ToolVersion: "test", Outcome: "pass", EvidenceFingerprint: "sha256:test-" + name})
	}
	receipt, err := assurance.Build("commit-1", "config-1", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now, checks)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildAuthorizedExternalManifest(input, receipt, "commit-1", "config-1", now, time.Minute); err != nil {
		t.Fatal(err)
	}
}
