package slack

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/sbos"
)

var urlPattern = regexp.MustCompile(`https?://[^\s<>"']+`)
var privateSlackFileURLPattern = regexp.MustCompile(`https?://files\.slack\.com/[^\s<>"']*files-pri[^\s<>"']*`)

const privateSlackFileSentinel = "slack-file-private://redacted"

var secretPatterns = []string{
	"password=",
	"api_key=",
	"xoxb-",
	"xoxp-",
	"bearer ",
	"sk_live_",
	"sk-proj-",
	"sk-svcacct-",
	"sk-admin-",
}

func Normalize(payload Payload) (Result, error) {
	adapterID := strings.TrimSpace(payload.Source.AdapterID)
	if adapterID == "" {
		adapterID = "slack"
	}
	if adapterID != "slack" {
		return Result{}, fmt.Errorf("source.adapter_id must be slack for Slack normalization")
	}
	channelID := strings.TrimSpace(payload.Source.ChannelID)
	if channelID == "" {
		return Result{}, fmt.Errorf("missing source.channel_id")
	}

	messages := append([]Message(nil), payload.Messages...)
	for _, message := range messages {
		if strings.TrimSpace(message.TS) == "" {
			return Result{}, fmt.Errorf("missing messages[].ts")
		}
		if strings.TrimSpace(message.User) == "" && strings.TrimSpace(message.AuthorName) == "" {
			return Result{}, fmt.Errorf("missing messages[].user or messages[].author_name")
		}
		if _, err := capturedAt(message); err != nil {
			return Result{}, err
		}
	}

	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].TS < messages[j].TS
	})

	result := Result{
		AdapterID:    adapterID,
		AuthorityIDs: authorityIDs(),
		Checkpoint: Checkpoint{
			AdapterID:             adapterID,
			Source:                sourceID(payload.Source),
			BatchOrder:            "old_to_new",
			InputCount:            len(messages),
			SkippedByAdapterCount: 0,
		},
	}

	for _, message := range messages {
		candidate, err := normalizeMessage(payload.Source, adapterID, channelID, message)
		if err != nil {
			return Result{}, err
		}
		if err := sbos.ValidateCandidate(candidate); err != nil {
			return Result{}, err
		}
		result.Candidates = append(result.Candidates, candidate)
	}
	result.Checkpoint.CandidateCount = len(result.Candidates)
	if len(messages) > 0 {
		result.Checkpoint.FirstTS = messages[0].TS
		result.Checkpoint.LastTS = messages[len(messages)-1].TS
		result.Checkpoint.NextOldestExclusiveTS = messages[0].TS
	}

	return result, nil
}

func normalizeMessage(source Source, adapterID, channelID string, message Message) (sbos.Candidate, error) {
	captured, err := capturedAt(message)
	if err != nil {
		return sbos.Candidate{}, err
	}
	normalizedTS := normalizeTS(message.TS)
	meta := normalizeMetadata(message.CaptureMetadata)
	author := strings.TrimSpace(message.AuthorName)
	if author == "" {
		author = strings.TrimSpace(message.User)
	}

	secretLike := isSecretLike(message)
	emptyContent := isEmpty(message)
	text := strings.TrimSpace(message.Text)
	text, textHadPrivateFileURL := replacePrivateSlackFileURLs(text)
	if secretLike {
		text = "[REDACTED SECRET-LIKE CONTENT]"
	} else if emptyContent {
		text = "[empty Slack capture]"
	}

	visibility := provenanceVisibility(meta)
	permalink := strings.TrimSpace(message.Permalink)
	if permalink == "" {
		permalink = "slack://missing-permalink/" + channelID + "/" + normalizedTS
		visibility = "private"
	}

	urls := collectURLs(text)
	attachments := []string{}
	privateProvenance := visibility == "private" || textHadPrivateFileURL
	for _, file := range message.Files {
		if strings.TrimSpace(file.ID) != "" {
			attachments = appendUnique(attachments, file.ID)
		}
		if strings.TrimSpace(file.Name) != "" {
			attachments = appendUnique(attachments, sanitizeSecret(file.Name, secretLike))
		}
		if strings.TrimSpace(file.Title) != "" {
			attachments = appendUnique(attachments, sanitizeSecret(file.Title, secretLike))
		}
		if strings.TrimSpace(file.URLPrivate) != "" {
			attachments = appendUnique(attachments, "slack-file-private://"+file.ID)
			privateProvenance = true
		}
		if strings.TrimSpace(file.URLPublic) != "" && visibility == "public" && !secretLike {
			urls = appendUnique(urls, strings.TrimSpace(file.URLPublic))
		}
	}
	for _, attachment := range message.Attachments {
		if strings.TrimSpace(attachment.ID) != "" {
			attachments = appendUnique(attachments, attachment.ID)
		}
		if strings.TrimSpace(attachment.Title) != "" {
			attachments = appendUnique(attachments, sanitizeSecret(attachment.Title, secretLike))
		}
		if strings.TrimSpace(attachment.Text) != "" && !secretLike {
			attachmentText, hadPrivateFileURL := replacePrivateSlackFileURLs(attachment.Text)
			if hadPrivateFileURL {
				privateProvenance = true
				attachments = appendUnique(attachments, privateSlackFileSentinel)
			}
			attachments = appendUnique(attachments, attachmentText)
			urls = appendURLs(urls, attachmentText)
		}
		if strings.TrimSpace(attachment.TitleLink) != "" && !secretLike {
			titleLink, hadPrivateFileURL := replacePrivateSlackFileURLs(attachment.TitleLink)
			if hadPrivateFileURL {
				privateProvenance = true
				attachments = appendUnique(attachments, privateSlackFileSentinel)
			} else {
				urls = appendUnique(urls, strings.TrimSpace(titleLink))
			}
		}
		if strings.TrimSpace(attachment.FromURL) != "" && !secretLike {
			fromURL, hadPrivateFileURL := replacePrivateSlackFileURLs(attachment.FromURL)
			if hadPrivateFileURL {
				privateProvenance = true
				attachments = appendUnique(attachments, privateSlackFileSentinel)
			} else {
				urls = appendUnique(urls, strings.TrimSpace(fromURL))
			}
		}
	}

	needsClarification := meta.SaveIntentStatus == "ambiguous" || meta.ClassificationStatus == "ambiguous"
	desiredVisibility := meta.DesiredVisibilityHint
	if needsClarification {
		desiredVisibility = "clarify"
	}
	clarificationReason := ""
	if needsClarification {
		clarificationReason = strings.TrimSpace(meta.ClarificationReason)
		if clarificationReason == "" {
			clarificationReason = "Ambiguous Slack capture metadata"
		}
	}
	enrichment := "not_required"
	if len(urls) > 0 {
		enrichment = "incomplete"
	}

	candidateID := "slack-" + channelID + "-" + normalizedTS
	return sbos.Candidate{
		SchemaVersion: "v0.1",
		CandidateID:   candidateID,
		AdapterID:     adapterID,
		ExternalID:    channelID + ":" + message.TS,
		CapturedAt:    captured,
		Provenance: sbos.Provenance{
			Permalink:       sbos.VisibilityValue{Value: permalink, Visibility: visibility},
			NativeTimestamp: sbos.VisibilityValue{Value: message.TS, Visibility: visibility},
			Author:          sbos.VisibilityValue{Value: author, Visibility: visibility},
			RawLocator:      sbos.VisibilityValue{Value: sourceID(source) + "/" + message.TS, Visibility: visibility},
		},
		Content: sbos.Content{
			Text:        text,
			URLs:        urls,
			Attachments: attachments,
			SourceTitle: sourceTitle(message, secretLike),
		},
		EnrichmentStatus: enrichment,
		Classification: sbos.Classification{
			Type:                "Source",
			Domain:              meta.DomainHint,
			Topics:              meta.TopicHints,
			Confidence:          "low",
			NeedsClarification:  needsClarification,
			ClarificationReason: clarificationReason,
		},
		Safety: sbos.Safety{
			RedactionRequired: secretLike,
			SecretLike:        secretLike,
			EmptyContent:      emptyContent,
			PrivateProvenance: privateProvenance || visibility == "private",
		},
		DesiredVisibility: desiredVisibility,
		IdempotencyKey:    "slack:" + channelID + ":" + message.TS,
	}, nil
}

func capturedAt(message Message) (string, error) {
	if strings.TrimSpace(message.CapturedAt) != "" {
		return strings.TrimSpace(message.CapturedAt), nil
	}
	parts := strings.Split(message.TS, ".")
	seconds, err := time.ParseDuration(parts[0] + "s")
	if err != nil {
		return "", fmt.Errorf("invalid Slack timestamp %q", message.TS)
	}
	return time.Unix(int64(seconds.Seconds()), 0).UTC().Format(time.RFC3339), nil
}

func normalizeMetadata(meta CaptureMetadata) CaptureMetadata {
	meta.SaveIntentStatus = defaultString(meta.SaveIntentStatus, "clear")
	meta.ClassificationStatus = defaultString(meta.ClassificationStatus, "clear")
	meta.DesiredVisibilityHint = defaultString(meta.DesiredVisibilityHint, "background")
	meta.ProvenanceVisibilityHint = defaultString(meta.ProvenanceVisibilityHint, "private")
	meta.DomainHint = defaultString(meta.DomainHint, "Research Landscape")
	if len(meta.TopicHints) == 0 {
		meta.TopicHints = []string{"slack-capture"}
	}
	return meta
}

func provenanceVisibility(meta CaptureMetadata) string {
	if meta.ProvenanceVisibilityHint == "public" && strings.TrimSpace(meta.PublicProvenanceAssertion) != "" {
		return "public"
	}
	return "private"
}

func isEmpty(message Message) bool {
	return strings.TrimSpace(message.Text) == "" && len(message.Files) == 0 && len(message.Attachments) == 0
}

func isSecretLike(message Message) bool {
	text := strings.ToLower(message.Text)
	for _, file := range message.Files {
		text += " " + strings.ToLower(file.Name+" "+file.Title+" "+file.URLPrivate+" "+file.URLPublic)
	}
	for _, attachment := range message.Attachments {
		text += " " + strings.ToLower(attachment.Title+" "+attachment.TitleLink+" "+attachment.FromURL+" "+attachment.Text)
	}
	for _, pattern := range secretPatterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func collectURLs(text string) []string {
	return appendURLs(nil, text)
}

func appendURLs(urls []string, text string) []string {
	for _, match := range urlPattern.FindAllString(text, -1) {
		url := strings.TrimRight(match, ".,);]")
		if privateSlackFileURLPattern.MatchString(url) {
			continue
		}
		urls = appendUnique(urls, url)
	}
	return urls
}

func replacePrivateSlackFileURLs(value string) (string, bool) {
	if !privateSlackFileURLPattern.MatchString(value) {
		return value, false
	}
	return privateSlackFileURLPattern.ReplaceAllString(value, privateSlackFileSentinel), true
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sanitizeSecret(value string, secretLike bool) string {
	if secretLike {
		return "[REDACTED SECRET-LIKE CONTENT]"
	}
	return strings.TrimSpace(value)
}

func sourceTitle(message Message, secretLike bool) string {
	if secretLike {
		return "Slack self-DM capture"
	}
	for _, file := range message.Files {
		if strings.TrimSpace(file.Title) != "" {
			return strings.TrimSpace(file.Title)
		}
	}
	for _, attachment := range message.Attachments {
		if strings.TrimSpace(attachment.Title) != "" {
			return strings.TrimSpace(attachment.Title)
		}
	}
	return "Slack self-DM capture"
}

func normalizeTS(ts string) string {
	return strings.ReplaceAll(ts, ".", "")
}

func sourceID(source Source) string {
	workspace := strings.TrimSpace(source.Workspace)
	if workspace == "" {
		workspace = "unknown-workspace"
	}
	return workspace + "/" + strings.TrimSpace(source.ChannelID)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func authorityIDs() []string {
	return []string{"WP-4", "WP-3", "WP-2", "FEAT-1", "STD-5", "STD-6", "STD-7", "STD-11", "STD-12", "DEC-6"}
}
