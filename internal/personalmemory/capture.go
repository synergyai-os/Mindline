package personalmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/routing"
)

// CaptureRecordInput is the source-neutral constructor contract adapters use.
// Adapters own native acquisition and timestamp conversion; Mindline owns
// identity, sanitization, canonical resource references, and sealing.
type CaptureRecordInput struct {
	SourceAdapter     string
	SourceScopeID     string
	SourceContainerID string
	ExternalID        string
	OccurredAt        string
	AuthorID          string
	AuthorName        string
	SourceRef         string
	RawText           string
	ThreadParentID    string
	AttachmentCount   int
	PrivateFileCount  int
	EditDeleteState   string
	Missingness       []string
}

type CaptureBatchInput struct {
	SourceIdentity  string
	LowerInclusive  string
	UpperInclusive  string
	Watermark       string
	DeclaredRecords int
	Records         []CaptureRecord
}

func NewCaptureRecord(input CaptureRecordInput) (CaptureRecord, error) {
	input.SourceAdapter = strings.TrimSpace(input.SourceAdapter)
	input.SourceScopeID = strings.TrimSpace(input.SourceScopeID)
	input.SourceContainerID = strings.TrimSpace(input.SourceContainerID)
	input.ExternalID = strings.TrimSpace(input.ExternalID)
	input.OccurredAt = strings.TrimSpace(input.OccurredAt)
	if input.SourceAdapter == "" || input.SourceScopeID == "" ||
		input.SourceContainerID == "" || input.ExternalID == "" {
		return CaptureRecord{}, errors.New("incomplete personal evidence source identity")
	}
	if _, err := time.Parse(time.RFC3339, input.OccurredAt); err != nil {
		return CaptureRecord{}, errors.New("invalid personal evidence timestamp")
	}
	missingness := append([]string(nil), input.Missingness...)
	sourceRef := strings.TrimSpace(input.SourceRef)
	if sourceRef == "" || containsSecret(sourceRef) || containsUnsafeURL(sourceRef) {
		return CaptureRecord{}, errors.New("invalid personal evidence source reference")
	}
	if strings.HasPrefix(sourceRef, "http://") || strings.HasPrefix(sourceRef, "https://") {
		safe, state, err := routing.PrepareURLForStorage(sourceRef)
		if err != nil || state == routing.URLStorageSensitiveRedacted || safe == "" {
			return CaptureRecord{}, errors.New("invalid personal evidence source reference")
		}
		sourceRef = safe
	}

	rawInput := strings.TrimSpace(input.RawText)
	rawText := rawInput
	contextState := "source_complete"
	if containsSecret(rawText) {
		rawText = "[REDACTED SECRET-LIKE CONTENT]"
		contextState = "secret_redacted"
		missingness = append(missingness, "secret_like_content_redacted")
	} else if rawText == "" {
		rawText = "[Capture has no text]"
		contextState = "empty_source"
		missingness = append(missingness, "source_text_empty")
	} else {
		var redacted bool
		rawText, redacted = sanitizeTextURLs(rawText)
		if redacted {
			missingness = append(missingness, "sensitive_url_redacted")
		}
	}
	if input.EditDeleteState == "deleted" || input.EditDeleteState == "tombstone" {
		rawText = "[Source item deleted; tombstone retained]"
		contextState = "deleted_tombstone"
		missingness = append(missingness, "source_content_deleted")
	}
	if input.AttachmentCount > 0 {
		missingness = append(missingness, "attachment_content_not_captured")
	}
	if input.PrivateFileCount > 0 {
		missingness = append(missingness, "private_file_content_not_captured")
	}
	authorID := strings.TrimSpace(input.AuthorID)
	authorName := strings.TrimSpace(input.AuthorName)
	if importedEvidenceContainsSecret(authorID, authorName) ||
		containsUnsafeURL(authorID+"\n"+authorName) {
		authorID = ""
		authorName = ""
		missingness = append(missingness, "author_metadata_redacted")
	} else {
		authorID, _ = sanitizeTextURLs(authorID)
		authorName, _ = sanitizeTextURLs(authorName)
	}
	threadParentID := strings.TrimSpace(input.ThreadParentID)
	if importedEvidenceContainsSecret(threadParentID) || containsUnsafeURL(threadParentID) {
		threadParentID = ""
		missingness = append(missingness, "thread_parent_redacted")
	}
	urls := safeURLsFromText(rawInput)
	resourceIDs := make([]string, 0, len(urls))
	for _, canonicalURL := range urls {
		resourceIDs = append(resourceIDs, stableResourceID(canonicalURL))
	}
	editDeleteState := strings.TrimSpace(input.EditDeleteState)
	if editDeleteState == "" {
		editDeleteState = "original"
	}
	sourceDigest := routing.URLFreeSourceFingerprint(rawText)
	idempotencyKey := strings.Join([]string{
		input.SourceAdapter, input.SourceScopeID, input.SourceContainerID, input.ExternalID,
	}, ":")
	record := CaptureRecord{
		RecordID: stableRecordID(idempotencyKey), IdempotencyKey: idempotencyKey,
		SourceAdapter: input.SourceAdapter, SourceScopeID: input.SourceScopeID,
		SourceContainerID: input.SourceContainerID, ExternalID: input.ExternalID,
		OccurredAt: input.OccurredAt, AuthorID: authorID, AuthorName: authorName,
		SourceRef: sourceRef, RawText: rawText, URLs: urls, ResourceIDs: resourceIDs,
		ThreadParentID: threadParentID, AttachmentCount: input.AttachmentCount,
		PrivateFileCount: input.PrivateFileCount, EditDeleteState: editDeleteState,
		ContextState: contextState, Missingness: uniqueSorted(missingness),
		AuthorityClass: AuthorityClass, SourceContentFingerprint: hex.EncodeToString(sourceDigest[:]),
	}
	record.ContentHash = fingerprintRecord(record)
	if err := validateRecord(record); err != nil {
		return CaptureRecord{}, err
	}
	return record, nil
}

func NewCaptureBatch(input CaptureBatchInput) (CaptureBatch, error) {
	records := append([]CaptureRecord(nil), input.Records...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].OccurredAt == records[j].OccurredAt {
			return records[i].ExternalID < records[j].ExternalID
		}
		return records[i].OccurredAt < records[j].OccurredAt
	})
	batch := CaptureBatch{
		SchemaVersion: CaptureBatchSchemaVersion, SourceIdentity: strings.TrimSpace(input.SourceIdentity),
		LowerInclusive: strings.TrimSpace(input.LowerInclusive), UpperInclusive: strings.TrimSpace(input.UpperInclusive),
		Watermark: strings.TrimSpace(input.Watermark), DeclaredRecords: input.DeclaredRecords, Records: records,
	}
	if err := validateCaptureBatch(batch); err != nil {
		return CaptureBatch{}, err
	}
	return batch, nil
}

func sanitizeTextURLs(value string) (string, bool) {
	matches := routing.URLPattern.FindAllStringIndex(value, -1)
	if len(matches) == 0 {
		return value, false
	}
	var output strings.Builder
	last := 0
	redacted := false
	for _, match := range matches {
		output.WriteString(value[last:match[0]])
		raw := strings.TrimRight(value[match[0]:match[1]], ".,;:!?)]}")
		suffix := value[match[0]+len(raw) : match[1]]
		safe, state, err := routing.PrepareURLForStorage(raw)
		if err != nil || state == routing.URLStorageSensitiveRedacted || safe == "" {
			output.WriteString("[mindline-sensitive-url-redacted]")
			redacted = true
		} else {
			output.WriteString(safe)
		}
		output.WriteString(suffix)
		last = match[1]
	}
	output.WriteString(value[last:])
	return output.String(), redacted
}

func safeURLsFromText(value string) []string {
	urls := []string{}
	for _, raw := range routing.ExtractLexicalURLOccurrences(value) {
		safe, state, err := routing.PrepareURLForStorage(raw)
		if err != nil || state == routing.URLStorageSensitiveRedacted || safe == "" {
			continue
		}
		canonical, err := routing.CanonicalizeURL(safe)
		if err != nil {
			continue
		}
		urls = append(urls, canonical)
	}
	return uniqueSorted(urls)
}

func stableRecordID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "capture-" + hex.EncodeToString(sum[:])[:24]
}

func stableRevisionID(record CaptureRecord) string {
	sum := sha256.Sum256([]byte(record.IdempotencyKey + "\x00" + record.ContentHash))
	return "revision-" + hex.EncodeToString(sum[:])[:24]
}

func stableResourceID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "resource-" + hex.EncodeToString(sum[:])[:24]
}

func stableResourceRevisionID(resource ResourceContext) string {
	sum := sha256.Sum256([]byte(resource.ResourceID + "\x00" + resource.ContentHash))
	return "resource-revision-" + hex.EncodeToString(sum[:])[:24]
}

func fingerprintRecord(record CaptureRecord) string {
	copy := record
	copy.ContentHash = ""
	return fingerprintValue(copy)
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
