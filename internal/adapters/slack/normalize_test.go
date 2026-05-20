package slack_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/synergyai-os/Mindline/internal/adapters/slack"
	"github.com/synergyai-os/Mindline/internal/sbos"
)

func TestNormalizeSortsMessagesOldToNewAndBuildsCheckpoint(t *testing.T) {
	payload := slack.Payload{
		Source: slack.Source{Workspace: "synergyai-os", ChannelID: "DSELF", ChannelName: "self-dm"},
		Messages: []slack.Message{
			{TS: "1710000002.000001", User: "U1", AuthorName: "Randy", Text: "third", Permalink: "https://workspace.slack.com/archives/DSELF/p1710000002000001"},
			{TS: "1710000000.000001", User: "U1", AuthorName: "Randy", Text: "first", Permalink: "https://workspace.slack.com/archives/DSELF/p1710000000000001"},
			{TS: "1710000001.000001", User: "U1", AuthorName: "Randy", Text: "second", Permalink: "https://workspace.slack.com/archives/DSELF/p1710000001000001"},
		},
	}

	result, err := slack.Normalize(payload)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	got := []string{result.Candidates[0].ExternalID, result.Candidates[1].ExternalID, result.Candidates[2].ExternalID}
	want := []string{"DSELF:1710000000.000001", "DSELF:1710000001.000001", "DSELF:1710000002.000001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order got %#v want %#v", got, want)
	}
	if result.Checkpoint.BatchOrder != "old_to_new" || result.Checkpoint.FirstTS != "1710000000.000001" || result.Checkpoint.LastTS != "1710000002.000001" {
		t.Fatalf("bad checkpoint: %#v", result.Checkpoint)
	}
	if result.Checkpoint.NextOldestExclusiveTS != "1710000000.000001" {
		t.Fatalf("next oldest cursor got %q want oldest processed ts", result.Checkpoint.NextOldestExclusiveTS)
	}
}

func TestNormalizeRejectsMismatchedAdapterID(t *testing.T) {
	payload := slack.Payload{
		Source: slack.Source{Workspace: "synergyai-os", ChannelID: "DSELF", AdapterID: "not-slack"},
		Messages: []slack.Message{
			{TS: "1710000000.000001", User: "U1", AuthorName: "Randy", Text: "hello"},
		},
	}

	_, err := slack.Normalize(payload)
	if err == nil || !strings.Contains(err.Error(), "source.adapter_id") {
		t.Fatalf("expected adapter id validation error, got %v", err)
	}
}

func TestNormalizeMapsSafetyAndPrivateProvenance(t *testing.T) {
	payload := slack.Payload{
		Source: slack.Source{Workspace: "synergyai-os", ChannelID: "DSELF"},
		Messages: []slack.Message{
			{TS: "1710000000.000001", User: "U1", AuthorName: "Randy", Text: "   "},
			{TS: "1710000001.000001", User: "U1", AuthorName: "Randy", Text: "password=" + secretValue() + " " + botToken() + " api_key=" + liveKey()},
			{
				TS:         "1710000002.000001",
				User:       "U1",
				AuthorName: "Randy",
				Text:       "Review https://example.com/page",
				Permalink:  "https://workspace.slack.com/archives/DSELF/p1710000002000001",
				Files: []slack.File{{
					ID:         "F123",
					Title:      "Design PDF",
					URLPrivate: privateFileURL(),
					URLPublic:  publicFileURL(),
				}},
				Attachments: []slack.Attachment{{
					Title:     "Article",
					TitleLink: "https://article.example/post",
					FromURL:   "https://source.example/root",
				}},
			},
		},
	}

	result, err := slack.Normalize(payload)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	empty := result.Candidates[0]
	if !empty.Safety.EmptyContent || route(t, empty) != sbos.StateSkipped {
		t.Fatalf("empty candidate did not skip: %#v", empty.Safety)
	}

	secret := result.Candidates[1]
	secretJSON := marshalCandidate(t, secret)
	forbidden := []string{secretValue(), botToken(), liveKey()}
	for _, value := range forbidden {
		if strings.Contains(secretJSON, value) {
			t.Fatalf("secret candidate leaked %q in %s", value, secretJSON)
		}
	}
	if !secret.Safety.SecretLike || !secret.Safety.RedactionRequired || route(t, secret) != sbos.StateSkipped {
		t.Fatalf("secret candidate safety/route mismatch: %#v route=%s", secret.Safety, route(t, secret))
	}

	privateFile := result.Candidates[2]
	privateFileJSON := marshalCandidate(t, privateFile)
	if strings.Contains(privateFileJSON, privateFileMarker()) || strings.Contains(privateFileJSON, publicFileURL()) {
		t.Fatalf("private file candidate leaked private or non-asserted public file URL: %s", privateFileJSON)
	}
	if !contains(privateFile.Content.URLs, "https://example.com/page") || !contains(privateFile.Content.URLs, "https://article.example/post") || !contains(privateFile.Content.URLs, "https://source.example/root") {
		t.Fatalf("eligible URLs missing: %#v", privateFile.Content.URLs)
	}
	if !contains(privateFile.Content.Attachments, "slack-file-private://F123") || !privateFile.Safety.PrivateProvenance {
		t.Fatalf("private file sentinel/provenance missing: attachments=%#v safety=%#v", privateFile.Content.Attachments, privateFile.Safety)
	}
}

func TestNormalizeHandlesClarifyAndPublicProvenance(t *testing.T) {
	payload := slack.Payload{
		Source: slack.Source{Workspace: "synergyai-os", ChannelID: "DSELF"},
		Messages: []slack.Message{
			{
				TS:         "1710000000.000001",
				User:       "U1",
				AuthorName: "Randy",
				Text:       "Not sure whether I meant the post or the link",
				CaptureMetadata: slack.CaptureMetadata{
					SaveIntentStatus:    "ambiguous",
					ClarificationReason: "Need to know whether Randy wanted the post or linked page",
				},
			},
			{
				TS:         "1710000001.000001",
				User:       "U1",
				AuthorName: "Randy",
				Text:       "Read https://public.example/source",
				Permalink:  "https://public.example/slack-source",
				CaptureMetadata: slack.CaptureMetadata{
					DesiredVisibilityHint:     "publish",
					ProvenanceVisibilityHint:  "public",
					PublicProvenanceAssertion: "fixture-approved-public-provenance",
				},
			},
		},
	}

	result, err := slack.Normalize(payload)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	clarify := result.Candidates[0]
	if clarify.DesiredVisibility != "clarify" || !clarify.Classification.NeedsClarification || strings.TrimSpace(clarify.Classification.ClarificationReason) == "" {
		t.Fatalf("clarify mapping mismatch: %#v visibility=%s", clarify.Classification, clarify.DesiredVisibility)
	}
	if route(t, clarify) != sbos.StateAttentionReady {
		t.Fatalf("expected attention route, got %s", route(t, clarify))
	}

	public := result.Candidates[1]
	if public.Provenance.Permalink.Visibility != "public" || public.Provenance.Author.Visibility != "public" || public.Provenance.NativeTimestamp.Visibility != "public" || public.Provenance.RawLocator.Visibility != "public" {
		t.Fatalf("expected public provenance, got %#v", public.Provenance)
	}
	if public.EnrichmentStatus != "incomplete" || route(t, public) != sbos.StateNeedsEnrichment {
		t.Fatalf("expected needs_enrichment for public URL capture, status=%s route=%s", public.EnrichmentStatus, route(t, public))
	}
}

func TestNormalizeRedactsSecretLikeURLFields(t *testing.T) {
	payload := slack.Payload{
		Source: slack.Source{Workspace: "synergyai-os", ChannelID: "DSELF"},
		Messages: []slack.Message{
			{
				TS:         "1710000000.000001",
				User:       "U1",
				AuthorName: "Randy",
				Text:       "public proof",
				Files: []slack.File{{
					ID:        "FSECRET",
					URLPublic: "https://files.example/" + botToken(),
				}},
				Attachments: []slack.Attachment{{
					TitleLink: "https://article.example/password=" + secretValue(),
					FromURL:   "https://source.example/api_key=" + liveKey(),
				}},
				CaptureMetadata: slack.CaptureMetadata{
					ProvenanceVisibilityHint:  "public",
					PublicProvenanceAssertion: "fixture-approved-public-provenance",
				},
			},
		},
	}

	result, err := slack.Normalize(payload)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	candidate := result.Candidates[0]
	body := marshalCandidate(t, candidate)
	for _, forbidden := range []string{botToken(), secretValue(), liveKey(), "password=", "api_key="} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("secret-like URL field leaked %q in %s", forbidden, body)
		}
	}
	if !candidate.Safety.SecretLike || !candidate.Safety.RedactionRequired {
		t.Fatalf("expected secret safety flags: %#v", candidate.Safety)
	}
	if route(t, candidate) != sbos.StateSkipped {
		t.Fatalf("expected skipped route, got %s", route(t, candidate))
	}
}

func TestNormalizeReplacesPrivateSlackFileURLsEverywhere(t *testing.T) {
	payload := slack.Payload{
		Source: slack.Source{Workspace: "synergyai-os", ChannelID: "DSELF"},
		Messages: []slack.Message{
			{
				TS:         "1710000000.000001",
				User:       "U1",
				AuthorName: "Randy",
				Text:       "Review " + privateFileURL(),
				Attachments: []slack.Attachment{{
					ID:        "A123",
					Title:     "Private file mention",
					TitleLink: privateFileURL(),
					FromURL:   privateFileURL(),
					Text:      "Attachment body " + privateFileURL(),
				}},
			},
		},
	}

	result, err := slack.Normalize(payload)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	candidate := result.Candidates[0]
	body := marshalCandidate(t, candidate)
	if strings.Contains(body, privateFileMarker()) || strings.Contains(body, privateFileURL()) {
		t.Fatalf("private Slack file URL leaked in candidate: %s", body)
	}
	if !candidate.Safety.PrivateProvenance {
		t.Fatalf("expected private provenance for private file URL sentinel: %#v", candidate.Safety)
	}
	if !strings.Contains(body, "slack-file-private://redacted") {
		t.Fatalf("expected redacted private file sentinel in candidate: %s", body)
	}
}

func TestRequiredFixturesNormalizeAndRoute(t *testing.T) {
	cases := []struct {
		name  string
		route sbos.State
		check func(t *testing.T, candidate sbos.Candidate, result slack.Result)
	}{
		{
			name: "reverse-ordered-batch.json",
			check: func(t *testing.T, _ sbos.Candidate, result slack.Result) {
				got := []string{result.Candidates[0].ExternalID, result.Candidates[1].ExternalID, result.Candidates[2].ExternalID}
				want := []string{"DSELF:1710000000.000001", "DSELF:1710000001.000001", "DSELF:1710000002.000001"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("order got %#v want %#v", got, want)
				}
			},
		},
		{
			name: "url-file-attachment.json",
			check: func(t *testing.T, candidate sbos.Candidate, _ slack.Result) {
				body := marshalCandidate(t, candidate)
				for _, forbidden := range []string{privateFileMarker(), publicFileURL()} {
					if strings.Contains(body, forbidden) {
						t.Fatalf("forbidden URL leaked: %q in %s", forbidden, body)
					}
				}
				for _, want := range []string{"https://example.com/page", "https://article.example/post", "https://source.example/root"} {
					if !contains(candidate.Content.URLs, want) {
						t.Fatalf("missing URL %q in %#v", want, candidate.Content.URLs)
					}
				}
				if !contains(candidate.Content.Attachments, "slack-file-private://F123") || !candidate.Safety.PrivateProvenance {
					t.Fatalf("missing private file proof: attachments=%#v safety=%#v", candidate.Content.Attachments, candidate.Safety)
				}
			},
		},
		{name: "empty-content.json", route: sbos.StateSkipped},
		{
			name:  "secret-redaction.json",
			route: sbos.StateSkipped,
			check: func(t *testing.T, candidate sbos.Candidate, _ slack.Result) {
				body := marshalCandidate(t, candidate)
				for _, forbidden := range []string{secretValue(), botToken(), liveKey()} {
					if strings.Contains(body, forbidden) {
						t.Fatalf("secret leaked: %q in %s", forbidden, body)
					}
				}
			},
		},
		{
			name:  "ambiguous-metadata.json",
			route: sbos.StateAttentionReady,
			check: func(t *testing.T, candidate sbos.Candidate, _ slack.Result) {
				if candidate.DesiredVisibility != "clarify" || !candidate.Classification.NeedsClarification || strings.TrimSpace(candidate.Classification.ClarificationReason) == "" {
					t.Fatalf("ambiguous metadata mismatch: visibility=%s classification=%#v", candidate.DesiredVisibility, candidate.Classification)
				}
			},
		},
		{
			name:  "missing-permalink-publish.json",
			route: sbos.StateBackgroundReady,
			check: func(t *testing.T, candidate sbos.Candidate, _ slack.Result) {
				if candidate.Provenance.Permalink.Value != "slack://missing-permalink/DSELF/1710000000000001" || candidate.Provenance.Permalink.Visibility != "private" {
					t.Fatalf("missing permalink sentinel mismatch: %#v", candidate.Provenance.Permalink)
				}
			},
		},
		{
			name:  "private-default-publish.json",
			route: sbos.StateBackgroundReady,
			check: func(t *testing.T, candidate sbos.Candidate, _ slack.Result) {
				if candidate.Provenance.Permalink.Visibility != "private" || candidate.Provenance.NativeTimestamp.Visibility != "private" || candidate.Provenance.Author.Visibility != "private" || candidate.Provenance.RawLocator.Visibility != "private" {
					t.Fatalf("expected all private provenance: %#v", candidate.Provenance)
				}
			},
		},
		{
			name:  "public-provenance-enrichment.json",
			route: sbos.StateNeedsEnrichment,
			check: func(t *testing.T, candidate sbos.Candidate, _ slack.Result) {
				if candidate.Provenance.Permalink.Visibility != "public" || candidate.EnrichmentStatus != "incomplete" {
					t.Fatalf("public enrichment mismatch: provenance=%#v enrichment=%s", candidate.Provenance, candidate.EnrichmentStatus)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var payload slack.Payload
			data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "slack", tc.name))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			result, err := slack.Normalize(payload)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			if len(result.Candidates) == 0 {
				t.Fatalf("expected candidates")
			}
			candidate := result.Candidates[0]
			if tc.route != "" && route(t, candidate) != tc.route {
				t.Fatalf("route got %s want %s", route(t, candidate), tc.route)
			}
			if tc.check != nil {
				tc.check(t, candidate, result)
			}
		})
	}
}

func route(t *testing.T, candidate sbos.Candidate) sbos.State {
	t.Helper()
	data, err := json.Marshal(candidate)
	if err != nil {
		t.Fatalf("marshal candidate: %v", err)
	}
	result, err := sbos.NewEngine().ProcessCandidate(data)
	if err != nil {
		t.Fatalf("process candidate: %v\n%s", err, string(data))
	}
	return result.State
}

func marshalCandidate(t *testing.T, candidate sbos.Candidate) string {
	t.Helper()
	data, err := json.Marshal(candidate)
	if err != nil {
		t.Fatalf("marshal candidate: %v", err)
	}
	return string(data)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func secretValue() string {
	return "super-" + "secret-value"
}

func botToken() string {
	return "xoxb-" + "1234567890-abcdef"
}

func liveKey() string {
	return "sk_live" + "_secret"
}

func privateFileURL() string {
	return "https://files.slack.com/files-" + "pri/T/F123/design.pdf"
}

func privateFileMarker() string {
	return "files-" + "pri"
}

func publicFileURL() string {
	return "https://files.example/public/" + "design.pdf"
}
