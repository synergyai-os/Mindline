package slack

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/routing"
)

func TestProjectActivationEvidenceIndependentlyFailsClosed(t *testing.T) {
	base := acquisition.ImportedEvidence{
		CanonicalItemID: "github-acme-tool",
		CanonicalURL:    "https://github.com/acme/tool",
		State:           "complete",
		RetrievedAt:     "2026-07-26T12:00:00Z",
		AccessClass:     "public",
		Metadata: acquisition.ImportedMetadata{
			Title:  "A useful public repository",
			Author: "Acme",
		},
		Excerpts: []acquisition.ImportedExcerpt{{
			ExcerptID: "excerpt-1",
			Text:      "A safe public excerpt",
			Locator:   "README",
		}},
	}
	tests := []struct {
		name   string
		mutate func(*acquisition.ImportedEvidence)
	}{
		{
			name: "caller secret flag",
			mutate: func(item *acquisition.ImportedEvidence) {
				item.SecretLike = true
			},
		},
		{
			name: "unmarked credential",
			mutate: func(item *acquisition.ImportedEvidence) {
				item.Metadata.Title = "credential password=synthetic-private-value"
			},
		},
		{
			name: "unmarked signed URL",
			mutate: func(item *acquisition.ImportedEvidence) {
				item.Excerpts[0].Text = "read https://example.com/private?token=synthetic-private-value"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.Excerpts = append([]acquisition.ImportedExcerpt(nil), base.Excerpts...)
			test.mutate(&input)
			projected := projectActivationEvidence([]acquisition.ImportedEvidence{input})
			if len(projected) != 1 {
				t.Fatalf("unsafe public evidence was not represented by one shell: %+v", projected)
			}
			shell := projected[0]
			if shell.State != "inaccessible" || shell.AccessClass != "unsupported" ||
				len(shell.Missingness) != 1 || shell.Missingness[0] != "activation_secret_like_evidence_redacted" ||
				shell.Metadata != (acquisition.ImportedMetadata{}) || len(shell.Excerpts) != 0 ||
				len(shell.RelatedURLs) != 0 || shell.RetrievedAt != "" || shell.SecretLike {
				t.Fatalf("unsafe public evidence did not become a content-free shell: %+v", shell)
			}
			encoded, err := json.Marshal(shell)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"synthetic-private-value", "password=", "?token="} {
				if strings.Contains(string(encoded), forbidden) {
					t.Fatalf("content-free shell leaked %q: %s", forbidden, encoded)
				}
			}
		})
	}

	safe := projectActivationEvidence([]acquisition.ImportedEvidence{base})
	if len(safe) != 1 || safe[0].Metadata.Title != base.Metadata.Title || len(safe[0].Excerpts) != 1 {
		t.Fatalf("safe public evidence was not preserved: %+v", safe)
	}

	unsafeIdentity := base
	unsafeIdentity.CanonicalItemID = "password=synthetic-private-value"
	if projected := projectActivationEvidence([]acquisition.ImportedEvidence{unsafeIdentity}); len(projected) != 0 {
		t.Fatalf("unsafe evidence identity crossed activation: %+v", projected)
	}
}

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

func TestExternalManifestCommitsProviderEditChronology(t *testing.T) {
	build := func(revision string) ExternalManifest {
		manifest, err := BuildExternalManifest(BuildInput{
			WorkspaceID: "T-synthetic", ChannelID: "C-synthetic",
			LowerInclusive: "1700000000.000001", UpperInclusive: "1700000010.000001",
			Watermark: "1700000010.000001", DataClass: DataClassSynthetic,
			Messages: []NativeMessage{{
				NativeMessageID: "m-edit", Timestamp: "1700000000.000001",
				RevisionTimestamp: revision, EditDeleteState: "edited",
				Text: "same edited content https://example.com/evidence",
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	first := build("1700000001.000001")
	second := build("1700000002.000001")
	if first.SourceRecords[0].RevisionTimestamp != "1700000001.000001" ||
		second.SourceRecords[0].RevisionTimestamp != "1700000002.000001" ||
		first.ContentFingerprint == second.ContentFingerprint {
		t.Fatalf("provider edit chronology was not committed: first=%+v second=%+v", first.SourceRecords[0], second.SourceRecords[0])
	}
	if _, err := ValidateExternalManifest(first); err != nil {
		t.Fatal(err)
	}
}

func TestExtractURLOccurrencesHandlesSlackLabelsAndPunctuation(t *testing.T) {
	urls := ExtractURLOccurrences("see <https://example.com/a?x=1|label>, then HTTPS://example.com/b).")
	if len(urls) != 2 || urls[0] != "https://example.com/a?x=1" || urls[1] != "HTTPS://example.com/b" {
		t.Fatalf("unexpected extracted occurrences: %#v", urls)
	}
}

func TestBuildExternalManifestCountsMalformedLexicalURLAsSensitive(t *testing.T) {
	manifest, err := BuildExternalManifest(BuildInput{
		WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001", DataClass: DataClassSynthetic,
		Messages: []NativeMessage{{NativeMessageID: "m-1", Timestamp: "1700000000.000001", Text: "HTTPS://example.com/public https://example.com/%zz?token=synthetic"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.URLOccurrences) != 2 || manifest.URLOccurrences[0].SourceOrdinal != 0 || manifest.URLOccurrences[1].SourceOrdinal != 1 {
		t.Fatalf("lexical source denominator was reduced: %+v", manifest.URLOccurrences)
	}
	if manifest.URLOccurrences[0].ObservedURL != "https://example.com/public" || manifest.URLOccurrences[1].ObservedURL != "" || manifest.URLOccurrences[1].SanitizationState != "sensitive_redacted" {
		t.Fatalf("lexical URLs were not normalized fail-closed: %+v", manifest.URLOccurrences)
	}
}

func TestBuildExternalManifestCountsEncodedQueryKeyAliasesAsSensitive(t *testing.T) {
	manifest, err := BuildExternalManifest(BuildInput{
		WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001", DataClass: DataClassSynthetic,
		Messages: []NativeMessage{{NativeMessageID: "m-1", Timestamp: "1700000000.000001", Text: "https://example.com/path?%75tm_source=x https://www.youtube.com/watch?%73i=x https://www.youtube.com/watch?%76=publicvid01"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.URLOccurrences) != 3 || len(manifest.CanonicalItems) != 3 {
		t.Fatalf("encoded query aliases changed the denominator: occurrences=%d items=%d", len(manifest.URLOccurrences), len(manifest.CanonicalItems))
	}
	for index, occurrence := range manifest.URLOccurrences {
		if occurrence.SourceOrdinal != index || occurrence.ObservedURL != "" || occurrence.SanitizationState != "sensitive_redacted" {
			t.Fatalf("encoded query alias was not withheld at ordinal %d: %+v", index, occurrence)
		}
	}
}

func TestBuildExternalManifestSanitizesSecretBearingURLsWithoutChangingDenominators(t *testing.T) {
	manifest, err := BuildExternalManifest(BuildInput{
		WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001", DataClass: DataClassSynthetic,
		Messages: []NativeMessage{{NativeMessageID: "m-1", Timestamp: "1700000000.000001", Text: "https://example.com/shared?amp;token=synthetic-value&keep=ok"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.SourceRecords) != 1 || len(manifest.URLOccurrences) != 1 || len(manifest.CanonicalItems) != 1 {
		t.Fatalf("sanitization changed denominators: records=%d occurrences=%d canonical=%d", len(manifest.SourceRecords), len(manifest.URLOccurrences), len(manifest.CanonicalItems))
	}
	occurrence := manifest.URLOccurrences[0]
	item := manifest.CanonicalItems[0]
	if occurrence.ObservedURL != "" || occurrence.SanitizationState != "sensitive_redacted" || item.CanonicalURL != "" || item.AccessState != "sensitive_redacted" || item.RetrievalStrategy != "manual_support" {
		t.Fatalf("unsafe URL was not withheld transparently: %+v item=%+v", occurrence, item)
	}
	if manifest.Completeness[len(manifest.Completeness)-2].Check != "sensitive_redacted_url_occurrences" || manifest.Completeness[len(manifest.Completeness)-2].Count != 1 {
		t.Fatalf("sanitization evidence missing: %+v", manifest.Completeness)
	}
}

func TestBuildExternalManifestDoesNotCollapseProviderForeignQueryIdentity(t *testing.T) {
	manifest, err := BuildExternalManifest(BuildInput{
		WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001", DataClass: DataClassSynthetic,
		Messages: []NativeMessage{{NativeMessageID: "m-1", Timestamp: "1700000000.000001", Text: "https://unrelated.example/resource?si=one https://unrelated.example/resource?si=two"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.URLOccurrences) != 2 || len(manifest.CanonicalItems) != 2 || manifest.Completeness[len(manifest.Completeness)-2].Count != 2 {
		t.Fatalf("unknown provider identities collapsed or disappeared: occurrences=%d items=%d evidence=%+v", len(manifest.URLOccurrences), len(manifest.CanonicalItems), manifest.Completeness)
	}
	for index, occurrence := range manifest.URLOccurrences {
		if occurrence.ObservedURL != "" || occurrence.SanitizationState != "sensitive_redacted" {
			t.Fatalf("provider-foreign query was persisted: %+v", occurrence)
		}
		if occurrence.SourceOrdinal != index {
			t.Fatalf("source URL ordinal was not preserved: %+v", occurrence)
		}
	}
}

func TestSensitiveRedactedIdentityDoesNotDependOnSecretValue(t *testing.T) {
	build := func(value string) ExternalManifest {
		manifest, err := BuildExternalManifest(BuildInput{
			WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001", DataClass: DataClassSynthetic,
			Messages: []NativeMessage{{NativeMessageID: "m-stable", Timestamp: "1700000000.000001", Text: "https://example.com/shared?token=" + value}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	first := build("synthetic-value-one")
	second := build("synthetic-value-two")
	if first.URLOccurrences[0].URLOccurrenceID != second.URLOccurrences[0].URLOccurrenceID || first.CanonicalItems[0].CanonicalItemID != second.CanonicalItems[0].CanonicalItemID {
		t.Fatalf("withheld identity changed with secret value: first=%+v second=%+v", first.URLOccurrences[0], second.URLOccurrences[0])
	}
	if first.SourceRecords[0].ContentFingerprint != second.SourceRecords[0].ContentFingerprint {
		t.Fatal("source fingerprint retained URL-derived content")
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
	if _, err := BuildAuthorizedExternalManifest(input, receipt, "commit-1", "config-1"); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizedManifestConstructorAlwaysProjectsUnsafeEvidence(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	checks := make([]assurance.Check, 0, len(assurance.RequiredChecks))
	for _, name := range assurance.RequiredChecks {
		checks = append(checks, assurance.Check{Name: name, ToolVersion: "test", Outcome: "pass", EvidenceFingerprint: "sha256:test-" + name})
	}
	receipt, err := assurance.Build("commit-1", "config-1", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now, checks)
	if err != nil {
		t.Fatal(err)
	}
	target := "https://github.com/acme/tool"
	input := BuildInput{
		WorkspaceID: "T-private", ChannelID: "C-private",
		LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001",
		DataClass: DataClassPrivateRuntime,
		Messages:  []NativeMessage{{NativeMessageID: "m-1", Timestamp: "1700000000.000001", Text: target}},
		ImportedEvidence: []acquisition.ImportedEvidence{{
			CanonicalItemID: routing.CanonicalURLID(target),
			CanonicalURL:    target,
			State:           "complete",
			RetrievedAt:     "2026-07-26T12:00:00Z",
			AccessClass:     "public",
			Metadata:        acquisition.ImportedMetadata{Title: "password=synthetic-private-value"},
			Excerpts:        []acquisition.ImportedExcerpt{{ExcerptID: "excerpt-1", Text: "unsafe detail", Locator: "README"}},
		}},
	}
	manifest, err := BuildAuthorizedExternalManifest(input, receipt, "commit-1", "config-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.ImportedEvidence) != 1 {
		t.Fatalf("authorized constructor lost explicit evidence state: %+v", manifest.ImportedEvidence)
	}
	shell := manifest.ImportedEvidence[0]
	if shell.State != "inaccessible" || len(shell.Missingness) != 1 ||
		shell.Missingness[0] != "activation_secret_like_evidence_redacted" ||
		shell.Metadata != (acquisition.ImportedMetadata{}) || len(shell.Excerpts) != 0 {
		t.Fatalf("authorized constructor did not enforce content-free projection: %+v", shell)
	}
}
