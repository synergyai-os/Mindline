package slack

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/routing"
)

type NativeMessage struct {
	NativeMessageID  string `json:"native_message_id"`
	Timestamp        string `json:"timestamp"`
	ThreadParentID   string `json:"thread_parent_id,omitempty"`
	Text             string `json:"text"`
	EditDeleteState  string `json:"edit_delete_state,omitempty"`
	AttachmentCount  int    `json:"attachment_count"`
	PrivateFileCount int    `json:"private_file_count"`
}

type BuildInput struct {
	ConnectorKind    string
	AdapterVersion   string
	WorkspaceID      string
	ChannelID        string
	LowerInclusive   string
	UpperInclusive   string
	Watermark        string
	Messages         []NativeMessage
	ImportedEvidence []acquisition.ImportedEvidence
	DataClass        string
}

// BuildExternalManifest is the Slack source-adapter boundary used by both
// connector exports and fixtures. It preserves every native record and every
// URL occurrence before canonicalization collapses duplicates.
func BuildExternalManifest(input BuildInput) (ExternalManifest, error) {
	return buildExternalManifest(input, false)
}

func BuildAuthorizedExternalManifest(input BuildInput, receipt assurance.Receipt, expectedCommit, expectedConfiguration string, now time.Time, maxAge time.Duration) (ExternalManifest, error) {
	if err := assurance.Validate(receipt, expectedCommit, expectedConfiguration, now, maxAge); err != nil {
		return ExternalManifest{}, errors.New("pre-live authority rejected private Slack build")
	}
	if input.DataClass != DataClassPrivateRuntime {
		return ExternalManifest{}, errors.New("authorized Slack build requires private-runtime data class")
	}
	return buildExternalManifest(input, true)
}

func buildExternalManifest(input BuildInput, privateAuthorized bool) (ExternalManifest, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.ChannelID) == "" || strings.TrimSpace(input.Watermark) == "" || len(input.Messages) == 0 {
		return ExternalManifest{}, errors.New("incomplete Slack manifest input")
	}
	if input.DataClass != DataClassSynthetic && input.DataClass != DataClassSentinel && !(privateAuthorized && input.DataClass == DataClassPrivateRuntime) {
		return ExternalManifest{}, errors.New("invalid Slack manifest data class")
	}
	messages := append([]NativeMessage(nil), input.Messages...)
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].Timestamp == messages[j].Timestamp {
			return messages[i].NativeMessageID < messages[j].NativeMessageID
		}
		return messages[i].Timestamp < messages[j].Timestamp
	})
	scope := acquisition.SealSourceScope(acquisition.SourceScope{
		ConnectorKind: connectorKind(input), WorkspaceID: input.WorkspaceID, ChannelID: input.ChannelID,
		LowerInclusive: input.LowerInclusive, UpperInclusive: input.UpperInclusive, IncludeThreads: true, IncludeReplies: true,
		AttachmentPolicy: "metadata_only", PrivateFilePolicy: "manual", EditDeletePolicy: "account", AdapterVersion: adapterVersion(input),
	})
	manifest := ExternalManifest{
		DataClass: input.DataClass, SourceIdentity: acquisition.SourceIdentity{ConnectorKind: connectorKind(input), WorkspaceID: input.WorkspaceID, ChannelID: input.ChannelID},
		SourceScope: scope, Watermark: input.Watermark,
	}
	items := map[string]*acquisition.InventoryItem{}
	sensitiveRedactions := 0
	nonSemanticSanitizations := 0
	for _, message := range messages {
		if strings.TrimSpace(message.NativeMessageID) == "" || strings.TrimSpace(message.Timestamp) == "" {
			return ExternalManifest{}, errors.New("invalid Slack native message identity")
		}
		state := message.EditDeleteState
		if state == "" {
			state = "original"
		}
		digest := fingerprintSourceText(message.Text)
		sourceID := stableSourceID("source", message.NativeMessageID, message.Timestamp)
		if message.AttachmentCount < 0 || message.PrivateFileCount < 0 || message.PrivateFileCount > message.AttachmentCount {
			return ExternalManifest{}, errors.New("invalid Slack file accounting")
		}
		record := acquisition.SourceRecord{SourceRecordID: sourceID, NativeMessageID: message.NativeMessageID, NativeTimestamp: message.Timestamp, ContentFingerprint: hex.EncodeToString(digest[:]), EditDeleteState: state, ThreadParentID: message.ThreadParentID, AttachmentCount: message.AttachmentCount, PrivateFileCount: message.PrivateFileCount}
		for index, observed := range ExtractURLOccurrences(message.Text) {
			safeObserved, storageState, err := routing.PrepareURLForStorage(observed)
			if err != nil {
				storageState = routing.URLStorageSensitiveRedacted
			}
			if storageState == routing.URLStorageSensitiveRedacted {
				occurrenceID := stableSourceID("occurrence", sourceID, stableIndex(index))
				canonicalID := stableSourceID("withheld", sourceID, stableIndex(index))
				record.URLOccurrenceIDs = append(record.URLOccurrenceIDs, occurrenceID)
				manifest.URLOccurrences = append(manifest.URLOccurrences, acquisition.URLOccurrence{
					URLOccurrenceID: occurrenceID, SourceRecordID: sourceID, SourceOrdinal: index, CanonicalItemID: canonicalID, SanitizationState: routing.URLStorageSensitiveRedacted,
				})
				items[canonicalID] = &acquisition.InventoryItem{
					CanonicalItemID: canonicalID, Kind: "unknown_sensitive", RetrievalStrategy: "manual_support", Format: "sensitive_redacted",
					AccessState: routing.URLStorageSensitiveRedacted, URLOccurrenceIDs: []string{occurrenceID},
				}
				sensitiveRedactions++
				continue
			}
			canonical, err := routing.CanonicalizeURL(safeObserved)
			if err != nil {
				continue
			}
			canonicalID := routing.CanonicalURLID(canonical)
			occurrenceID := stableSourceID("occurrence", message.NativeMessageID, message.Timestamp, stableIndex(index), safeObserved)
			record.URLOccurrenceIDs = append(record.URLOccurrenceIDs, occurrenceID)
			occurrence := acquisition.URLOccurrence{URLOccurrenceID: occurrenceID, SourceRecordID: sourceID, SourceOrdinal: index, ObservedURL: safeObserved, CanonicalItemID: canonicalID}
			if storageState == routing.URLStorageNonSemanticComponentsRemoved {
				occurrence.SanitizationState = storageState
				nonSemanticSanitizations++
			}
			manifest.URLOccurrences = append(manifest.URLOccurrences, occurrence)
			item := items[canonicalID]
			if item == nil {
				kind, strategy, format := classifyExternalURL(canonical)
				item = &acquisition.InventoryItem{CanonicalItemID: canonicalID, CanonicalURL: canonical, Kind: kind, RetrievalStrategy: strategy, Format: format}
				items[canonicalID] = item
			}
			item.URLOccurrenceIDs = append(item.URLOccurrenceIDs, occurrenceID)
		}
		manifest.SourceRecords = append(manifest.SourceRecords, record)
	}
	itemIDs := make([]string, 0, len(items))
	for id := range items {
		itemIDs = append(itemIDs, id)
	}
	sort.Strings(itemIDs)
	strata := map[string]int{}
	for _, id := range itemIDs {
		item := *items[id]
		sort.Strings(item.URLOccurrenceIDs)
		manifest.CanonicalItems = append(manifest.CanonicalItems, item)
		strata[item.RetrievalStrategy+"\x00"+item.Format]++
	}
	stratumKeys := make([]string, 0, len(strata))
	for key := range strata {
		stratumKeys = append(stratumKeys, key)
	}
	sort.Strings(stratumKeys)
	for _, key := range stratumKeys {
		parts := strings.SplitN(key, "\x00", 2)
		manifest.Strata = append(manifest.Strata, acquisition.StratumCount{RetrievalStrategy: parts[0], Format: parts[1], Count: strata[key]})
	}
	manifest.Completeness = []acquisition.EvidenceCheck{
		{Check: "source_record_denominator", Status: "pass", Count: len(manifest.SourceRecords)},
		{Check: "url_occurrence_denominator", Status: "pass", Count: len(manifest.URLOccurrences)},
		{Check: "canonical_item_denominator", Status: "pass", Count: len(manifest.CanonicalItems)},
		{Check: "bidirectional_occurrence_accounting", Status: "pass", Count: len(manifest.URLOccurrences)},
		{Check: "sensitive_redacted_url_occurrences", Status: "pass", Count: sensitiveRedactions},
		{Check: "non_semantic_url_sanitizations", Status: "pass", Count: nonSemanticSanitizations},
	}
	evidenceByID := map[string]acquisition.ImportedEvidence{}
	for _, evidence := range input.ImportedEvidence {
		if items[evidence.CanonicalItemID] == nil || evidenceByID[evidence.CanonicalItemID].CanonicalItemID != "" {
			return ExternalManifest{}, errors.New("imported evidence is unknown or duplicated")
		}
		evidenceByID[evidence.CanonicalItemID] = evidence
	}
	for _, id := range itemIDs {
		if evidence, ok := evidenceByID[id]; ok {
			manifest.ImportedEvidence = append(manifest.ImportedEvidence, evidence)
		}
	}
	manifest = SealExternalManifest(manifest)
	if _, err := validateExternalManifest(manifest, input.DataClass == DataClassPrivateRuntime); err != nil {
		return ExternalManifest{}, err
	}
	return manifest, nil
}

func connectorKind(input BuildInput) string {
	if strings.TrimSpace(input.ConnectorKind) != "" {
		return input.ConnectorKind
	}
	return "external_slack_inventory"
}
func adapterVersion(input BuildInput) string {
	if strings.TrimSpace(input.AdapterVersion) != "" {
		return input.AdapterVersion
	}
	return ExternalInventorySchema
}

func ExtractURLOccurrences(text string) []string {
	matches := routing.URLPattern.FindAllString(text, -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		candidate := strings.TrimRight(match, ".,;:!?)]}")
		if candidate != "" {
			result = append(result, candidate)
		}
	}
	return result
}

func fingerprintSourceText(text string) [sha256.Size]byte {
	matches := routing.URLPattern.FindAllStringIndex(text, -1)
	var redacted strings.Builder
	last := 0
	for index, match := range matches {
		start, end := match[0], match[1]
		candidate := strings.TrimRight(text[start:end], ".,;:!?)]}")
		candidateEnd := start + len(candidate)
		redacted.WriteString(text[last:start])
		redacted.WriteString("[mindline-url-occurrence:")
		redacted.WriteString(stableIndex(index))
		redacted.WriteString("]")
		redacted.WriteString(text[candidateEnd:end])
		last = end
	}
	redacted.WriteString(text[last:])
	return sha256.Sum256([]byte(redacted.String()))
}

func classifyExternalURL(raw string) (kind, strategy, format string) {
	parsed, _ := url.Parse(raw)
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
	path := strings.Trim(parsed.Path, "/")
	switch {
	case host == "lnkd.in":
		kind, strategy, format = "linkedin_post", "redirect", "linkedin_short_link"
	case host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com"):
		kind, strategy, format = "linkedin_post", "linkedin", "linkedin_post"
		if strings.Contains(parsed.Path, "/pulse/") {
			kind, format = "linkedin_article", "linkedin_article"
		} else if !strings.Contains(parsed.Path, "/posts/") && !strings.Contains(parsed.Path, "/feed/update/") {
			format = "linkedin_other"
		}
	case host == "youtube.com" || host == "youtu.be" || strings.HasSuffix(host, ".youtube.com"):
		kind, strategy, format = "youtube_video", "youtube", "youtube_video"
		if host == "youtu.be" {
			format = "youtube_video_short_link"
		} else if strings.Contains(parsed.Path, "/shorts/") {
			format = "youtube_short"
		} else if strings.Contains(parsed.Path, "/channel/") || strings.Contains(parsed.Path, "/@") {
			format = "youtube_channel"
		}
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		kind, strategy, format = "github_repository", "github", "github_repository"
		segments := strings.Split(path, "/")
		if host == "gist.github.com" {
			format = "github_gist"
		} else if len(segments) >= 4 && segments[2] == "issues" {
			format = "github_issue"
		} else if len(segments) >= 4 && segments[2] == "pull" {
			format = "github_pull_request"
		} else if len(segments) >= 4 && (segments[2] == "blob" || segments[2] == "tree") {
			format = "github_file"
		} else if strings.Contains(strings.ToLower(path), "readme") || strings.Contains(strings.ToLower(path), "wiki") {
			format = "github_documentation"
		}
	case host == "open.spotify.com":
		kind, strategy, format = "generic_web", "spotify", "spotify_resource"
		if strings.HasPrefix(path, "episode/") {
			format = "spotify_episode"
		} else if strings.HasPrefix(path, "show/") {
			format = "spotify_show"
		}
	case strings.HasSuffix(host, ".substack.com") || host == "substack.com":
		kind, strategy, format = "article", "substack", "substack_post"
		if !strings.Contains(path, "/") && !strings.HasPrefix(path, "p/") {
			format = "substack_publication"
		}
	case strings.Contains(host, "amazon.") || host == "amzn.to":
		kind, strategy, format = "generic_web", "commerce", "amazon_product"
	case host == "instagram.com" && strings.Contains(parsed.Path, "/reel/"):
		kind, strategy, format = "generic_web", "social_media", "instagram_reel"
	case host == "tiktok.com" || strings.HasSuffix(host, ".tiktok.com"):
		kind, strategy, format = "generic_web", "social_media", "tiktok_video"
	case host == "x.com" || host == "twitter.com" || strings.HasSuffix(host, ".twitter.com"):
		kind, strategy, format = "generic_web", "social_media", "x_twitter_post"
	case host == "threads.net" || strings.HasSuffix(host, ".threads.net"):
		kind, strategy, format = "generic_web", "social_media", "threads_post"
	case strings.Contains(host, "notion."):
		kind, strategy, format = "generic_web", "authenticated_document", "notion_page"
	case host == "docs.google.com" && strings.HasPrefix(path, "spreadsheets/"):
		kind, strategy, format = "generic_web", "authenticated_document", "google_spreadsheet"
	case host == "docs.google.com" && strings.HasPrefix(path, "document/"):
		kind, strategy, format = "generic_web", "authenticated_document", "google_document"
	case host == "drive.google.com":
		kind, strategy, format = "generic_web", "authenticated_document", "google_drive"
	case host == "mail.google.com":
		kind, strategy, format = "generic_web", "authenticated_document", "google_mail"
	case host == "keep.google.com":
		kind, strategy, format = "generic_web", "authenticated_document", "google_keep"
	case strings.Contains(host, "calendar.google."):
		kind, strategy, format = "generic_web", "authenticated_document", "authenticated_calendar"
	case strings.Contains(host, "slack.com"):
		kind, strategy, format = "generic_web", "authenticated_document", "authenticated_slack"
	case host == "medium.com" || strings.HasSuffix(host, ".medium.com"):
		kind, strategy, format = "article", "article", "medium_article"
	case host == "observablehq.com":
		kind, strategy, format = "generic_web", "notebook", "observable_notebook"
	case strings.Contains(host, "airtable.com"):
		kind, strategy, format = "generic_web", "authenticated_document", "airtable_shared_view"
	case strings.Contains(host, "figma.com"):
		kind, strategy, format = "generic_web", "authenticated_document", "figma_design"
	case host == "dev.to":
		kind, strategy, format = "article", "article", "devto_article"
	case host == "npmjs.com" || strings.HasSuffix(host, ".npmjs.com"):
		kind, strategy, format = "generic_web", "package_registry", "npm_package"
	case strings.Contains(host, "producthunt.com"):
		kind, strategy, format = "generic_web", "product_directory", "product_hunt_page"
	case host == "reddit.com" || strings.HasSuffix(host, ".reddit.com"):
		kind, strategy, format = "generic_web", "community", "reddit_post"
	case strings.Contains(host, "linear.app"):
		kind, strategy, format = "generic_web", "linear", "linear_product"
		if strings.Contains(path, "docs") || strings.Contains(path, "method") {
			format = "linear_article"
		}
	case host == "vimeo.com" || strings.HasSuffix(host, ".vimeo.com"):
		kind, strategy, format = "generic_web", "video", "vimeo_video"
		if strings.Contains(path, "ondemand") {
			format = "vimeo_on_demand"
		} else if strings.Contains(path, "showcase") || strings.Contains(path, "user") {
			format = "vimeo_folder_or_profile"
		}
	case host == "chat.whatsapp.com":
		kind, strategy, format = "generic_web", "authenticated_social", "whatsapp_invite"
	case strings.HasSuffix(strings.ToLower(parsed.Path), ".pdf"):
		kind, strategy, format = "pdf", "pdf", "pdf_document"
	default:
		kind, strategy, format = "generic_web", "generic_web", "web_page"
	}
	return kind, strategy, format
}

type importedEvidenceAccessPolicy string

const (
	importedEvidencePublicEligible importedEvidenceAccessPolicy = "public_eligible"
	importedEvidenceManualOnly     importedEvidenceAccessPolicy = "manual_only"
)

// classifyExternalURLPolicy is the source adapter's independent provider
// policy. Unknown providers deliberately remain manual-only until a retrieval
// implementation proves public access; a manifest cannot promote itself by
// declaring a generic/public strategy.
func classifyExternalURLPolicy(raw string) (kind, strategy, format string, policy importedEvidenceAccessPolicy) {
	kind, strategy, format = classifyExternalURL(raw)
	policy = importedEvidencePublicEligible
	switch strategy {
	case "authenticated_document", "authenticated_social", "generic_web":
		policy = importedEvidenceManualOnly
	}
	return kind, strategy, format, policy
}

func stableSourceID(prefix string, values ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return prefix + "-" + hex.EncodeToString(digest[:])[:24]
}

func stableIndex(index int) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if index == 0 {
		return "0"
	}
	var result []byte
	for index > 0 {
		result = append(result, digits[index%len(digits)])
		index /= len(digits)
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return string(result)
}
