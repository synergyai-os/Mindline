package gmail

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/sbos"
)

var urlPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

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
		adapterID = "gmail"
	}
	if adapterID != "gmail" {
		return Result{}, fmt.Errorf("source.adapter_id must be gmail for Gmail normalization")
	}
	messages := payloadMessages(payload)
	for _, message := range messages {
		if strings.TrimSpace(message.ID) == "" {
			return Result{}, fmt.Errorf("missing messages[].id")
		}
		if strings.TrimSpace(message.EmailTS) == "" {
			return Result{}, fmt.Errorf("missing messages[].email_ts")
		}
		if _, err := capturedAt(message); err != nil {
			return Result{}, err
		}
	}

	sort.SliceStable(messages, func(i, j int) bool {
		left, _ := parsedEmailTime(messages[i])
		right, _ := parsedEmailTime(messages[j])
		if left.Equal(right) {
			return messages[i].ID < messages[j].ID
		}
		return left.Before(right)
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
		candidate, err := normalizeMessage(payload.Source, adapterID, message)
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
		result.Checkpoint.FirstEmailTS = messages[0].EmailTS
		result.Checkpoint.LastEmailTS = messages[len(messages)-1].EmailTS
		result.Checkpoint.NextOldestExclusiveTS = messages[0].EmailTS
	}
	return result, nil
}

func payloadMessages(payload Payload) []Message {
	if len(payload.Messages) > 0 {
		return append([]Message(nil), payload.Messages...)
	}
	return append([]Message(nil), payload.Responses...)
}

func normalizeMessage(source Source, adapterID string, message Message) (sbos.Candidate, error) {
	captured, err := capturedAt(message)
	if err != nil {
		return sbos.Candidate{}, err
	}
	author := strings.TrimSpace(message.From)
	if author == "" {
		author = strings.TrimSpace(message.FromAlt)
	}
	if author == "" {
		author = "unknown Gmail sender"
	} else {
		author = "gmail-sender-" + fingerprint(author)
	}
	secretLike := isSecretLike(message)
	emptyContent := isEmpty(message)
	text := messageText(message)
	if secretLike {
		text = "[REDACTED SECRET-LIKE CONTENT]"
	} else if emptyContent {
		text = "[empty Gmail message]"
	}
	title := strings.TrimSpace(message.Subject)
	if title == "" {
		title = strings.TrimSpace(message.DisplayTitle)
	}
	if title == "" || secretLike {
		title = "Gmail message"
	}
	permalink := gmailPermalink(message)
	attachments := attachmentLabels(message, secretLike)
	candidateID := "gmail-" + fingerprint(sourceID(source)+"/"+message.ID)
	return sbos.Candidate{
		SchemaVersion: "v0.1",
		CandidateID:   candidateID,
		AdapterID:     adapterID,
		ExternalID:    "gmail-message-" + fingerprint(message.ID),
		CapturedAt:    captured,
		Provenance: sbos.Provenance{
			Permalink:       sbos.VisibilityValue{Value: permalink, Visibility: "private"},
			NativeTimestamp: sbos.VisibilityValue{Value: message.EmailTS, Visibility: "private"},
			Author:          sbos.VisibilityValue{Value: author, Visibility: "private"},
			RawLocator:      sbos.VisibilityValue{Value: "gmail://message/" + fingerprint(sourceID(source)+"/"+message.ID), Visibility: "private"},
		},
		Content: sbos.Content{
			Text:        text,
			URLs:        collectURLs(text),
			Attachments: attachments,
			SourceTitle: title,
		},
		EnrichmentStatus: enrichmentStatus(text),
		Classification: sbos.Classification{
			Type:       "Source",
			Domain:     "Inbox / Unknown",
			Topics:     []string{"gmail-capture"},
			Confidence: "low",
		},
		Safety: sbos.Safety{
			RedactionRequired: secretLike,
			SecretLike:        secretLike,
			EmptyContent:      emptyContent,
			PrivateProvenance: true,
		},
		DesiredVisibility: "background",
		IdempotencyKey:    "gmail:" + fingerprint(sourceID(source)+"/"+message.ID),
	}, nil
}

func capturedAt(message Message) (string, error) {
	parsed, err := parsedEmailTime(message)
	if err != nil {
		return "", fmt.Errorf("invalid Gmail email_ts %q", message.EmailTS)
	}
	return parsed.UTC().Format(time.RFC3339), nil
}

func parsedEmailTime(message Message) (time.Time, error) {
	return parsedEmailTimestamp(message.EmailTS)
}

func parsedEmailTimestamp(value string) (time.Time, error) {
	return time.Parse(time.RFC3339, strings.TrimSpace(value))
}

func messageText(message Message) string {
	parts := []string{}
	if strings.TrimSpace(message.Subject) != "" {
		parts = append(parts, "Subject: "+strings.TrimSpace(message.Subject))
	}
	if strings.TrimSpace(message.Snippet) != "" {
		parts = append(parts, "Snippet: "+strings.TrimSpace(message.Snippet))
	}
	if strings.TrimSpace(message.Body) != "" {
		parts = append(parts, strings.TrimSpace(message.Body))
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func isEmpty(message Message) bool {
	return messageText(message) == "" && !message.HasAttachment && len(message.Attachments) == 0
}

func isSecretLike(message Message) bool {
	text := strings.ToLower(message.Subject + " " + message.Snippet + " " + message.Body + " " + message.From + " " + message.FromAlt)
	for _, attachment := range message.Attachments {
		text += " " + strings.ToLower(attachment.Filename+" "+attachment.AttachmentID)
	}
	for _, pattern := range secretPatterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}
	return false
}

func attachmentLabels(message Message, secretLike bool) []string {
	if secretLike {
		if message.HasAttachment || len(message.Attachments) > 0 {
			return []string{"[REDACTED SECRET-LIKE ATTACHMENT METADATA]"}
		}
		return nil
	}
	labels := []string{}
	for _, attachment := range message.Attachments {
		label := strings.TrimSpace(attachment.Filename)
		if label == "" {
			label = strings.TrimSpace(attachment.MimeType)
		}
		if label == "" {
			label = "gmail-attachment"
		}
		labels = appendUnique(labels, label)
	}
	if message.HasAttachment && len(labels) == 0 {
		labels = append(labels, "gmail-attachment")
	}
	return labels
}

func enrichmentStatus(text string) string {
	if len(collectURLs(text)) > 0 {
		return "incomplete"
	}
	return "not_required"
}

func collectURLs(text string) []string {
	urls := []string{}
	for _, match := range urlPattern.FindAllString(text, -1) {
		urls = appendUnique(urls, strings.TrimRight(match, ".,);]"))
	}
	return urls
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

func sourceID(source Source) string {
	account := strings.TrimSpace(source.Account)
	if account == "" {
		account = "unknown-account"
	}
	return "gmail-account-" + fingerprint(account) + "/" + mailboxID(source)
}

func gmailPermalink(message Message) string {
	displayURL := strings.TrimSpace(message.DisplayURL)
	if displayURL == "" {
		return "gmail://missing-display-url/" + fingerprint(message.ID)
	}
	return "gmail://display-url/" + fingerprint(displayURL)
}

func mailboxID(source Source) string {
	mailbox := strings.TrimSpace(source.Mailbox)
	if mailbox == "" {
		return "all-mail"
	}
	switch strings.ToLower(mailbox) {
	case "all-mail", "inbox":
		return strings.ToLower(mailbox)
	default:
		return "gmail-mailbox-" + fingerprint(mailbox)
	}
}

func fingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func authorityIDs() []string {
	return []string{"WP-46", "WP-31", "WP-4", "WP-3", "FEAT-1", "STD-2", "STD-7", "STD-11", "STD-12", "PRI-1", "BR-1"}
}
