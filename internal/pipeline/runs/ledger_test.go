package runs

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStateFromPipelineItem(t *testing.T) {
	cases := []struct {
		name       string
		input      ItemInput
		wantState  string
		wantReview bool
		wantReason string
	}{
		{
			name: "published preview",
			input: ItemInput{
				RecordID:      "pipeline-text-only",
				PipelineState: "dry_run_published",
				PreviewPaths:  []string{"destinations/pipeline-text-only/previews/op.md"},
			},
			wantState: "published_preview",
		},
		{
			name: "missing enrichment",
			input: ItemInput{
				RecordID: "pipeline-youtube-url",
				Blockers: []string{"missing_local_youtube_transcript"},
			},
			wantState:  "needs_enrichment",
			wantReview: true,
			wantReason: "missing_local_youtube_transcript",
		},
		{
			name: "private provenance",
			input: ItemInput{
				RecordID:          "fingerprint:abc123",
				PrivateProvenance: true,
			},
			wantState:  "background",
			wantReview: false,
			wantReason: "",
		},
		{
			name: "secret skip",
			input: ItemInput{
				RecordID:      "fingerprint:def456",
				PipelineState: "skipped",
				Blockers:      []string{"secret_like_content_detected"},
				SecretLike:    true,
			},
			wantState:  "skipped",
			wantReview: false,
			wantReason: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildLedgerItem("run-abc", tc.input, []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"})
			if got.State != tc.wantState {
				t.Fatalf("state got %q want %q", got.State, tc.wantState)
			}
			if got.ReviewRequired != tc.wantReview {
				t.Fatalf("review got %v want %v", got.ReviewRequired, tc.wantReview)
			}
			if got.ReviewReason != tc.wantReason {
				t.Fatalf("reason got %q want %q", got.ReviewReason, tc.wantReason)
			}
		})
	}
}

func TestPathSafeIDsRedactUnsafeNativeIDs(t *testing.T) {
	cases := []string{
		"https://linkedin.com/private/PRIVATE_DM_SENTINEL_DO_NOT_WRITE",
		"sk-test-secret-do-not-leak",
		"../outside",
		`folder\\outside`,
		"author-name/private-dm-text",
	}
	for _, native := range cases {
		t.Run(native, func(t *testing.T) {
			got := BuildLedgerItem("run-abc", ItemInput{
				RecordID:          native,
				SourceCandidateID: native,
				PrivateProvenance: true,
			}, []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"})
			for _, value := range []string{got.RecordID, got.SourceCandidateID} {
				assertSafeIdentifier(t, value)
			}
			for _, value := range []string{got.PipelineResultPath, got.ProcessorPlanPath, got.DestinationSummaryPath} {
				assertSafeRelativePath(t, value)
			}
		})
	}
}

func assertSafeIdentifier(t *testing.T, value string) {
	t.Helper()
	for _, forbidden := range []string{"PRIVATE_DM_SENTINEL_DO_NOT_WRITE", "sk-test-secret-do-not-leak", "http://", "https://", "..", "/", `\\`} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("unsafe id/path %q contains %q", value, forbidden)
		}
	}
}

func assertSafeRelativePath(t *testing.T, value string) {
	t.Helper()
	for _, forbidden := range []string{"PRIVATE_DM_SENTINEL_DO_NOT_WRITE", "sk-test-secret-do-not-leak", "http://", "https://", "..", `\\`} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("unsafe path %q contains %q", value, forbidden)
		}
	}
	if filepath.IsAbs(value) {
		t.Fatalf("path must be relative: %q", value)
	}
}

func TestRunIDIsDeterministicAndDoesNotUsePrivateContent(t *testing.T) {
	a := RunIdentityInput{
		InputPath:     "/repo/testdata/pipeline/inputs/pipeline-private-provenance.json",
		InputBytes:    []byte(`{"source":"fingerprint-only"}`),
		MethodID:      "basb-para-code",
		DestinationID: "tolaria",
	}
	b := a
	gotA := BuildRunID(a)
	gotB := BuildRunID(b)
	if gotA != gotB {
		t.Fatalf("run id not deterministic: %q %q", gotA, gotB)
	}
	if gotA == "" || len(gotA) != len("run-0123456789abcdef") {
		t.Fatalf("unexpected run id %q", gotA)
	}
}

func TestManifestCountsStates(t *testing.T) {
	items := []LedgerItem{
		{State: "published_preview"},
		{State: "needs_enrichment", ReviewRequired: true},
		{State: "blocked", ReviewRequired: true},
	}
	manifest := BuildManifest(ManifestInput{
		RunID:            "run-abc",
		RunMode:          "dry_run",
		MethodID:         "basb-para-code",
		DestinationID:    "tolaria",
		InputFingerprint: "sha256:test",
		Items:            items,
		AuthorityIDs:     []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"},
	})
	if manifest.ItemCount != 3 || manifest.ReviewQueueCount != 2 {
		t.Fatalf("unexpected counts: %+v", manifest)
	}
	if manifest.States["published_preview"] != 1 || manifest.States["needs_enrichment"] != 1 || manifest.States["blocked"] != 1 {
		t.Fatalf("unexpected state counts: %+v", manifest.States)
	}
}

func TestBuildIndexPreservesItemOrderAndReviewFlags(t *testing.T) {
	items := []LedgerItem{
		{RunID: "run-abc", RecordID: "publish", SourceCandidateID: "publish", State: "published_preview", ReviewRequired: false},
		{RunID: "run-abc", RecordID: "youtube", SourceCandidateID: "youtube", State: "needs_enrichment", ReviewRequired: true},
	}
	index := BuildIndex("run-abc", items, []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"})
	if index.ItemCount != 2 || index.ReviewRequiredCount != 1 {
		t.Fatalf("unexpected counts: %+v", index)
	}
	if index.Items[0].RecordID != "publish" || index.Items[0].LedgerItemPath != "items/publish.json" {
		t.Fatalf("unexpected first item: %+v", index.Items[0])
	}
	if !index.Items[1].ReviewRequired || index.Items[1].State != "needs_enrichment" {
		t.Fatalf("unexpected second item: %+v", index.Items[1])
	}
}

func TestReviewQueueIncludesOnlyReviewRequiredItems(t *testing.T) {
	items := []LedgerItem{
		{RunID: "run-abc", RecordID: "publish", State: "published_preview", ReviewRequired: false},
		{RunID: "run-abc", RecordID: "youtube", State: "needs_enrichment", ReviewRequired: true, ReviewReason: "missing_local_youtube_transcript"},
		{RunID: "run-abc", RecordID: "secret", State: "skipped", ReviewRequired: false, ReviewReason: "secret_like_content_detected"},
	}
	queue := BuildReviewQueue("run-abc", items, []string{"PROD-1", "DEC-17", "DEC-15", "WP-8"})
	if queue.QueueCount != 1 {
		t.Fatalf("queue count got %d want 1", queue.QueueCount)
	}
	if queue.Items[0].RecordID != "youtube" || queue.Items[0].Reason != "missing_local_youtube_transcript" {
		t.Fatalf("unexpected queue item: %+v", queue.Items[0])
	}
}
