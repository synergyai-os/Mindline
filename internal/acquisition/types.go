package acquisition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/routing"
)

const (
	InventorySnapshotSchema = "mindline-inventory-snapshot/v0.2"
	maximumSourceRecords    = 100_000
	maximumURLOccurrences   = 250_000
	maximumCanonicalItems   = 250_000
)

type SourceScope struct {
	ConnectorKind     string `json:"connector_kind"`
	WorkspaceID       string `json:"workspace_id"`
	ChannelID         string `json:"channel_id"`
	LowerInclusive    string `json:"lower_inclusive"`
	UpperInclusive    string `json:"upper_inclusive"`
	IncludeThreads    bool   `json:"include_threads"`
	IncludeReplies    bool   `json:"include_replies"`
	AttachmentPolicy  string `json:"attachment_policy"`
	PrivateFilePolicy string `json:"private_file_policy"`
	EditDeletePolicy  string `json:"edit_delete_policy"`
	AdapterVersion    string `json:"adapter_version"`
	Fingerprint       string `json:"fingerprint"`
}

type SourceIdentity struct {
	ConnectorKind string `json:"connector_kind"`
	WorkspaceID   string `json:"workspace_id"`
	ChannelID     string `json:"channel_id"`
}

func (identity SourceIdentity) String() string {
	return identity.ConnectorKind + ":" + identity.WorkspaceID + ":" + identity.ChannelID
}

type InventorySnapshot struct {
	SchemaVersion   string          `json:"schema_version"`
	Fingerprint     string          `json:"fingerprint"`
	SourceIdentity  string          `json:"source_identity"`
	Watermark       string          `json:"watermark"`
	OccurrenceCount int             `json:"occurrence_count"`
	CanonicalCount  int             `json:"canonical_count"`
	SourceRecords   []SourceRecord  `json:"source_records"`
	URLOccurrences  []URLOccurrence `json:"url_occurrences"`
	CanonicalItems  []InventoryItem `json:"canonical_items"`
	Strata          []StratumCount  `json:"strata"`
	Completeness    []EvidenceCheck `json:"completeness"`
}

type SourceRecord struct {
	SourceRecordID     string   `json:"source_record_id"`
	NativeMessageID    string   `json:"native_message_id"`
	NativeTimestamp    string   `json:"native_timestamp"`
	RevisionTimestamp  string   `json:"revision_timestamp,omitempty"`
	ContentFingerprint string   `json:"content_fingerprint"`
	URLOccurrenceIDs   []string `json:"url_occurrence_ids"`
	EditDeleteState    string   `json:"edit_delete_state"`
	ThreadParentID     string   `json:"thread_parent_id,omitempty"`
	AttachmentCount    int      `json:"attachment_count,omitempty"`
	PrivateFileCount   int      `json:"private_file_count,omitempty"`
}

type URLOccurrence struct {
	URLOccurrenceID   string `json:"url_occurrence_id"`
	SourceRecordID    string `json:"source_record_id"`
	SourceOrdinal     int    `json:"source_ordinal"`
	ObservedURL       string `json:"observed_url,omitempty"`
	CanonicalItemID   string `json:"canonical_item_id"`
	SanitizationState string `json:"sanitization_state,omitempty"`
}

type InventoryItem struct {
	CanonicalItemID   string   `json:"canonical_item_id"`
	CanonicalURL      string   `json:"canonical_url"`
	Kind              string   `json:"kind"`
	URLOccurrenceIDs  []string `json:"url_occurrence_ids"`
	RetrievalStrategy string   `json:"retrieval_strategy"`
	Format            string   `json:"format"`
	AccessState       string   `json:"access_state,omitempty"`
}

type StratumCount struct {
	RetrievalStrategy string `json:"retrieval_strategy"`
	Format            string `json:"format"`
	Count             int    `json:"count"`
}

type EvidenceCheck struct {
	Check  string `json:"check"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type InventoryCounts struct {
	SourceRecords  int `json:"source_records"`
	URLOccurrences int `json:"url_occurrences"`
	CanonicalItems int `json:"canonical_items"`
}

// ImportedEvidence is an inert, optional evidence envelope in an external
// inventory. Retrieval converts it to an explicitly replay-labelled artifact.
type ImportedEvidence struct {
	CanonicalItemID string            `json:"canonical_item_id"`
	CanonicalURL    string            `json:"canonical_url"`
	State           string            `json:"state"`
	RetrievedAt     string            `json:"retrieved_at"`
	Metadata        ImportedMetadata  `json:"public_metadata"`
	Excerpts        []ImportedExcerpt `json:"public_excerpts"`
	RelatedURLs     []ImportedRelated `json:"related_urls"`
	Missingness     []string          `json:"missingness"`
	AccessClass     string            `json:"access_class,omitempty"`
	SecretLike      bool              `json:"secret_like,omitempty"`
}

type ImportedMetadata struct {
	Title       string `json:"title,omitempty"`
	Author      string `json:"author,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

type ImportedExcerpt struct {
	ExcerptID string `json:"excerpt_id"`
	Text      string `json:"text"`
	Locator   string `json:"locator"`
}

type ImportedRelated struct {
	URL                  string `json:"url"`
	Relation             string `json:"relation"`
	DiscoveryEvidenceRef string `json:"discovery_evidence_ref"`
	SemanticallyRelevant bool   `json:"semantically_relevant"`
}

func SealSourceScope(scope SourceScope) SourceScope {
	scope.Fingerprint = ""
	scope.Fingerprint = Fingerprint(scope)
	return scope
}

func SealInventory(snapshot InventorySnapshot) InventorySnapshot {
	snapshot.SchemaVersion = InventorySnapshotSchema
	snapshot.OccurrenceCount = len(snapshot.URLOccurrences)
	snapshot.CanonicalCount = len(snapshot.CanonicalItems)
	snapshot.Fingerprint = ""
	snapshot.Fingerprint = Fingerprint(snapshot)
	return snapshot
}

func ValidateInventory(snapshot InventorySnapshot) error {
	if snapshot.SchemaVersion != InventorySnapshotSchema {
		return errors.New("unsupported inventory schema")
	}
	if strings.TrimSpace(snapshot.SourceIdentity) == "" || strings.TrimSpace(snapshot.Watermark) == "" {
		return errors.New("missing inventory identity or watermark")
	}
	if snapshot.OccurrenceCount != len(snapshot.URLOccurrences) || snapshot.CanonicalCount != len(snapshot.CanonicalItems) {
		return errors.New("declared inventory counts do not match observed counts")
	}
	if len(snapshot.SourceRecords) > maximumSourceRecords || len(snapshot.URLOccurrences) > maximumURLOccurrences || len(snapshot.CanonicalItems) > maximumCanonicalItems {
		return errors.New("inventory count limit exceeded")
	}
	if snapshot.Fingerprint == "" || snapshot.Fingerprint != Fingerprint(snapshot) {
		return errors.New("inventory fingerprint mismatch")
	}

	records := make(map[string]SourceRecord, len(snapshot.SourceRecords))
	for _, record := range snapshot.SourceRecords {
		if !validID(record.SourceRecordID) || !validID(record.NativeMessageID) || !validSHA256(record.ContentFingerprint) || records[record.SourceRecordID].SourceRecordID != "" {
			return errors.New("invalid or duplicate source record")
		}
		if _, err := NativeTimestampToRFC3339(record.NativeTimestamp); err != nil {
			return errors.New("invalid source record timestamp")
		}
		if record.RevisionTimestamp != "" {
			revision, revisionErr := NativeRevisionTimestampToRFC3339(record.RevisionTimestamp)
			occurred, occurredErr := NativeRevisionTimestampToRFC3339(record.NativeTimestamp)
			revisionTime, revisionParseErr := time.Parse(time.RFC3339Nano, revision)
			occurredTime, occurredParseErr := time.Parse(time.RFC3339Nano, occurred)
			if record.EditDeleteState != "edited" || revisionErr != nil || occurredErr != nil ||
				revisionParseErr != nil || occurredParseErr != nil || !revisionTime.After(occurredTime) {
				return errors.New("invalid source record revision timestamp")
			}
		}
		if record.AttachmentCount < 0 || record.PrivateFileCount < 0 || record.PrivateFileCount > record.AttachmentCount {
			return errors.New("invalid source file accounting")
		}
		switch record.EditDeleteState {
		case "original", "edited", "deleted", "tombstone":
		default:
			return errors.New("invalid edit/delete state")
		}
		if hasEmptyOrDuplicate(record.URLOccurrenceIDs) {
			return errors.New("invalid source occurrence references")
		}
		records[record.SourceRecordID] = record
	}

	items := make(map[string]InventoryItem, len(snapshot.CanonicalItems))
	canonicalURLSeen := map[string]bool{}
	for _, item := range snapshot.CanonicalItems {
		if !validID(item.CanonicalItemID) || items[item.CanonicalItemID].CanonicalItemID != "" || strings.TrimSpace(item.Kind) == "" || strings.TrimSpace(item.RetrievalStrategy) == "" || strings.TrimSpace(item.Format) == "" {
			return errors.New("invalid or duplicate canonical item")
		}
		if item.AccessState == "sensitive_redacted" {
			if item.CanonicalURL != "" || item.Kind != "unknown_sensitive" || item.RetrievalStrategy != "manual_support" || item.Format != "sensitive_redacted" {
				return errors.New("invalid sensitive-redacted canonical item")
			}
		} else {
			if item.AccessState != "" {
				return errors.New("invalid canonical item access state")
			}
			canonical, err := validateWebURL(item.CanonicalURL)
			if err != nil || canonicalURLSeen[canonical] {
				return errors.New("invalid or duplicate canonical URL")
			}
			canonicalURLSeen[canonical] = true
		}
		if hasEmptyOrDuplicate(item.URLOccurrenceIDs) {
			return errors.New("invalid canonical occurrence references")
		}
		items[item.CanonicalItemID] = item
	}

	occurrences := make(map[string]URLOccurrence, len(snapshot.URLOccurrences))
	sourceOrdinals := make(map[string]bool, len(snapshot.URLOccurrences))
	for _, occurrence := range snapshot.URLOccurrences {
		ordinalKey := occurrence.SourceRecordID + "\x00" + strconv.Itoa(occurrence.SourceOrdinal)
		if occurrence.SourceOrdinal < 0 || sourceOrdinals[ordinalKey] || !validID(occurrence.URLOccurrenceID) || occurrences[occurrence.URLOccurrenceID].URLOccurrenceID != "" || records[occurrence.SourceRecordID].SourceRecordID == "" || items[occurrence.CanonicalItemID].CanonicalItemID == "" {
			return errors.New("invalid or duplicate URL occurrence")
		}
		if occurrence.SanitizationState != "" && occurrence.SanitizationState != "sensitive_redacted" && occurrence.SanitizationState != "non_semantic_components_removed" {
			return errors.New("invalid URL occurrence sanitization state")
		}
		item := items[occurrence.CanonicalItemID]
		sensitiveOccurrence := occurrence.SanitizationState == "sensitive_redacted"
		sensitiveItem := item.AccessState == "sensitive_redacted"
		if sensitiveOccurrence != sensitiveItem {
			return errors.New("sensitive-redacted occurrence and item state mismatch")
		}
		if sensitiveOccurrence {
			if occurrence.ObservedURL != "" {
				return errors.New("invalid sensitive-redacted URL occurrence")
			}
		} else if _, err := validateWebURL(occurrence.ObservedURL); err != nil {
			return errors.New("invalid observed URL")
		}
		sourceOrdinals[ordinalKey] = true
		occurrences[occurrence.URLOccurrenceID] = occurrence
	}

	seenFromRecords := map[string]bool{}
	for _, record := range snapshot.SourceRecords {
		for _, occurrenceID := range record.URLOccurrenceIDs {
			occurrence, ok := occurrences[occurrenceID]
			if !ok || occurrence.SourceRecordID != record.SourceRecordID || seenFromRecords[occurrenceID] {
				return errors.New("source-to-occurrence accounting mismatch")
			}
			seenFromRecords[occurrenceID] = true
		}
	}
	seenFromItems := map[string]bool{}
	for _, item := range snapshot.CanonicalItems {
		for _, occurrenceID := range item.URLOccurrenceIDs {
			occurrence, ok := occurrences[occurrenceID]
			if !ok || occurrence.CanonicalItemID != item.CanonicalItemID || seenFromItems[occurrenceID] {
				return errors.New("canonical-to-occurrence accounting mismatch")
			}
			seenFromItems[occurrenceID] = true
		}
	}
	if len(seenFromRecords) != len(occurrences) || len(seenFromItems) != len(occurrences) {
		return errors.New("unreferenced URL occurrence")
	}
	if err := validateStrata(snapshot.CanonicalItems, snapshot.Strata); err != nil {
		return err
	}
	for _, check := range snapshot.Completeness {
		if strings.TrimSpace(check.Check) == "" || check.Count < 0 {
			return errors.New("invalid completeness check")
		}
		switch check.Status {
		case "pass", "fail", "not_applicable":
		default:
			return errors.New("invalid completeness status")
		}
	}
	return nil
}

func ValidateImportedEvidence(evidence []ImportedEvidence, snapshot InventorySnapshot) error {
	items := make(map[string]InventoryItem, len(snapshot.CanonicalItems))
	for _, item := range snapshot.CanonicalItems {
		items[item.CanonicalItemID] = item
	}
	seen := map[string]bool{}
	for _, artifact := range evidence {
		item, ok := items[artifact.CanonicalItemID]
		if !ok || seen[artifact.CanonicalItemID] || artifact.CanonicalURL != item.CanonicalURL {
			return errors.New("imported evidence identity mismatch")
		}
		if item.AccessState == "sensitive_redacted" {
			return errors.New("sensitive-redacted item cannot carry imported evidence")
		}
		seen[artifact.CanonicalItemID] = true
		switch artifact.AccessClass {
		case "", "public", "private", "authenticated", "unsupported":
		default:
			return errors.New("invalid imported evidence access class")
		}
		switch artifact.State {
		case "complete", "partial", "inaccessible", "failed", "not_attempted":
		default:
			return errors.New("invalid imported evidence state")
		}
		if artifact.RetrievedAt != "" {
			if _, err := time.Parse(time.RFC3339, artifact.RetrievedAt); err != nil {
				return errors.New("invalid imported evidence timestamp")
			}
		}
		excerptIDs := map[string]bool{}
		totalRunes := 0
		for _, excerpt := range artifact.Excerpts {
			if !validID(excerpt.ExcerptID) || excerptIDs[excerpt.ExcerptID] || strings.TrimSpace(excerpt.Text) == "" || len([]rune(excerpt.Text)) > 1000 || strings.TrimSpace(excerpt.Locator) == "" {
				return errors.New("invalid imported evidence excerpt")
			}
			totalRunes += len([]rune(excerpt.Text))
			excerptIDs[excerpt.ExcerptID] = true
		}
		if totalRunes > 4000 {
			return errors.New("imported evidence excerpt budget exceeded")
		}
		if artifact.State == "inaccessible" && (len(artifact.Excerpts) != 0 || len(artifact.Missingness) == 0) {
			return errors.New("inaccessible imported evidence must be explicit and unevidenced")
		}
		for _, related := range artifact.RelatedURLs {
			safeURL, storageState, err := routing.PrepareURLForStorage(related.URL)
			if err != nil || storageState == routing.URLStorageSensitiveRedacted || safeURL != related.URL || related.Relation != "source_links_to" || !excerptIDs[related.DiscoveryEvidenceRef] {
				return errors.New("invalid related imported evidence")
			}
		}
	}
	return nil
}

func NativeTimestampToRFC3339(value string) (string, error) {
	parsed, err := parseNativeTimestamp(value)
	if err != nil {
		return "", err
	}
	// Preserve the established canonical representation for ordinary source
	// timestamps. Changing this format would rewrite every existing capture on
	// an exact replay.
	return parsed.UTC().Format(time.RFC3339), nil
}

// NativeRevisionTimestampToRFC3339 preserves sub-second provider chronology
// without changing the established canonical OccurredAt representation.
func NativeRevisionTimestampToRFC3339(value string) (string, error) {
	parsed, err := parseNativeTimestamp(value)
	if err != nil {
		return "", err
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func parseNativeTimestamp(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, nil
	}
	parts := strings.SplitN(trimmed, ".", 2)
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || seconds < 0 {
		return time.Time{}, errors.New("invalid native timestamp")
	}
	nanos := int64(0)
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) > 9 || fraction == "" {
			return time.Time{}, errors.New("invalid native timestamp")
		}
		fraction += strings.Repeat("0", 9-len(fraction))
		nanos, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return time.Time{}, errors.New("invalid native timestamp")
		}
	}
	return time.Unix(seconds, nanos).UTC(), nil
}

func Fingerprint(value any) string {
	data, _ := json.Marshal(value)
	var generic any
	_ = json.Unmarshal(data, &generic)
	if object, ok := generic.(map[string]any); ok {
		delete(object, "fingerprint")
	}
	canonical, _ := json.Marshal(generic)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func validateStrata(items []InventoryItem, strata []StratumCount) error {
	expected := map[string]int{}
	for _, item := range items {
		expected[item.RetrievalStrategy+"\x00"+item.Format]++
	}
	observed := map[string]int{}
	for _, stratum := range strata {
		key := stratum.RetrievalStrategy + "\x00" + stratum.Format
		if strings.TrimSpace(stratum.RetrievalStrategy) == "" || strings.TrimSpace(stratum.Format) == "" || stratum.Count <= 0 || observed[key] != 0 {
			return errors.New("invalid inventory stratum")
		}
		observed[key] = stratum.Count
	}
	if len(observed) != len(expected) {
		return errors.New("inventory stratum coverage mismatch")
	}
	for key, count := range expected {
		if observed[key] != count {
			return errors.New("inventory stratum count mismatch")
		}
	}
	return nil
}

func BuildStrata(items []InventoryItem) []StratumCount {
	counts := map[string]StratumCount{}
	for _, item := range items {
		key := item.RetrievalStrategy + "\x00" + item.Format
		stratum := counts[key]
		stratum.RetrievalStrategy = item.RetrievalStrategy
		stratum.Format = item.Format
		stratum.Count++
		counts[key] = stratum
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]StratumCount, 0, len(keys))
	for _, key := range keys {
		result = append(result, counts[key])
	}
	return result
}

func validateWebURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("invalid web URL")
	}
	return parsed.String(), nil
}

func validID(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && len(trimmed) <= 256 && trimmed == value
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func hasEmptyOrDuplicate(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if !validID(value) || seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}

func ValidateSourceScope(scope SourceScope) error {
	if strings.TrimSpace(scope.ConnectorKind) == "" || strings.TrimSpace(scope.WorkspaceID) == "" || strings.TrimSpace(scope.ChannelID) == "" || strings.TrimSpace(scope.AdapterVersion) == "" {
		return errors.New("incomplete source scope")
	}
	for _, bound := range []string{scope.LowerInclusive, scope.UpperInclusive} {
		if bound != "" {
			if _, err := NativeTimestampToRFC3339(bound); err != nil {
				return errors.New("invalid source scope bound")
			}
		}
	}
	if scope.LowerInclusive != "" && scope.UpperInclusive != "" {
		lower, _ := NativeTimestampToRFC3339(scope.LowerInclusive)
		upper, _ := NativeTimestampToRFC3339(scope.UpperInclusive)
		if lower > upper {
			return errors.New("invalid source scope range")
		}
	}
	if scope.Fingerprint == "" || scope.Fingerprint != Fingerprint(scope) {
		return fmt.Errorf("source scope fingerprint mismatch")
	}
	return nil
}
