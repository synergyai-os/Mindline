package runs

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	LedgerItemSchemaVersion  = "run-ledger-item/v0.1"
	RunLedgerSchemaVersion   = "run-ledger/v0.1"
	IndexSchemaVersion       = "run-ledger-index/v0.1"
	ReviewQueueSchemaVersion = "review-queue/v0.1"
	ReviewItemSchemaVersion  = "review-queue-item/v0.1"
)

type ItemInput struct {
	RecordID               string
	SourceCandidateID      string
	PipelineState          string
	Blockers               []string
	PreviewPaths           []string
	PipelineResultPath     string
	ProcessorPlanPath      string
	DestinationSummaryPath string
	PrivateProvenance      bool
	SecretLike             bool
	RedactionRequired      bool
	SafeTitle              string
}

type LedgerItem struct {
	SchemaVersion          string   `json:"schema_version"`
	RunID                  string   `json:"run_id"`
	RecordID               string   `json:"record_id"`
	SourceCandidateID      string   `json:"source_candidate_id"`
	State                  string   `json:"state"`
	ReviewRequired         bool     `json:"review_required"`
	ReviewReason           string   `json:"-"`
	Blockers               []string `json:"blockers"`
	PipelineResultPath     string   `json:"pipeline_result_path"`
	ProcessorPlanPath      string   `json:"processor_plan_path"`
	DestinationSummaryPath string   `json:"destination_summary_path"`
	PreviewPaths           []string `json:"preview_paths,omitempty"`
	SafeTitle              string   `json:"safe_title"`
	SafeSummary            string   `json:"safe_summary"`
	AuthorityIDs           []string `json:"authority_ids"`
}

type RunIdentityInput struct {
	InputPath     string
	InputBytes    []byte
	MethodID      string
	DestinationID string
}

type ManifestInput struct {
	RunID            string
	RunMode          string
	MethodID         string
	DestinationID    string
	InputFingerprint string
	Items            []LedgerItem
	AuthorityIDs     []string
	Now              string
}

type Manifest struct {
	SchemaVersion       string         `json:"schema_version"`
	RunID               string         `json:"run_id"`
	RunMode             string         `json:"run_mode"`
	PipelineSummaryPath string         `json:"pipeline_summary_path"`
	MethodID            string         `json:"method_id"`
	DestinationID       string         `json:"destination_id"`
	InputFingerprint    string         `json:"input_fingerprint"`
	StartedAt           string         `json:"started_at"`
	CompletedAt         string         `json:"completed_at"`
	ItemCount           int            `json:"item_count"`
	ReviewQueueCount    int            `json:"review_queue_count"`
	States              map[string]int `json:"states"`
	AuthorityIDs        []string       `json:"authority_ids"`
}

type Index struct {
	SchemaVersion       string      `json:"schema_version"`
	RunID               string      `json:"run_id"`
	ItemCount           int         `json:"item_count"`
	ReviewRequiredCount int         `json:"review_required_count"`
	Items               []IndexItem `json:"items"`
	AuthorityIDs        []string    `json:"authority_ids"`
}

type IndexItem struct {
	RecordID          string `json:"record_id"`
	SourceCandidateID string `json:"source_candidate_id"`
	State             string `json:"state"`
	ReviewRequired    bool   `json:"review_required"`
	LedgerItemPath    string `json:"ledger_item_path"`
}

type ReviewQueue struct {
	SchemaVersion string             `json:"schema_version"`
	RunID         string             `json:"run_id"`
	QueueCount    int                `json:"queue_count"`
	Items         []ReviewQueueEntry `json:"items"`
	AuthorityIDs  []string           `json:"authority_ids"`
}

type ReviewQueueEntry struct {
	RecordID       string `json:"record_id"`
	State          string `json:"state"`
	Reason         string `json:"reason"`
	ReviewItemPath string `json:"review_item_path"`
}

type ReviewQueueItem struct {
	SchemaVersion       string            `json:"schema_version"`
	RunID               string            `json:"run_id"`
	RecordID            string            `json:"record_id"`
	State               string            `json:"state"`
	Priority            string            `json:"priority"`
	Reason              string            `json:"reason"`
	SuggestedNextAction string            `json:"suggested_next_action"`
	SafeTitle           string            `json:"safe_title"`
	SafeContext         string            `json:"safe_context"`
	Links               map[string]string `json:"links"`
	AuthorityIDs        []string          `json:"authority_ids"`
}

func BuildLedgerItem(runID string, input ItemInput, authorityIDs []string) LedgerItem {
	recordID := BuildSafeID(input.RecordID)
	sourceCandidateID := BuildSafeID(input.SourceCandidateID)
	if sourceCandidateID == "" {
		sourceCandidateID = recordID
	}
	state, reviewRequired, reviewReason := itemState(input)
	title := input.SafeTitle
	if title == "" || input.PrivateProvenance || input.RedactionRequired || input.SecretLike {
		title = "Processed source"
	}
	summary := "Processed in local dry-run."
	if input.PrivateProvenance || input.RedactionRequired {
		summary = "Private provenance retained as background processing evidence."
	}
	if input.SecretLike {
		summary = "Secret-like content skipped without human-readable body content."
	}
	return LedgerItem{
		SchemaVersion:          LedgerItemSchemaVersion,
		RunID:                  runID,
		RecordID:               recordID,
		SourceCandidateID:      sourceCandidateID,
		State:                  state,
		ReviewRequired:         reviewRequired,
		ReviewReason:           reviewReason,
		Blockers:               safeBlockers(input.Blockers, input.SecretLike),
		PipelineResultPath:     safeOutputPath(input.PipelineResultPath, cleanOutPath("results", recordID+".json")),
		ProcessorPlanPath:      safeOutputPath(input.ProcessorPlanPath, cleanOutPath("processors", recordID+".json")),
		DestinationSummaryPath: safeOutputPath(input.DestinationSummaryPath, cleanOutPath("destinations", recordID, "destination-summary.json")),
		PreviewPaths:           safePreviewPaths(input.PreviewPaths),
		SafeTitle:              title,
		SafeSummary:            summary,
		AuthorityIDs:           append([]string(nil), authorityIDs...),
	}
}

func BuildSafeID(native string) string {
	value := strings.TrimSpace(native)
	if isSafeNativeID(value) {
		return value
	}
	sum := sha256.Sum256([]byte(value))
	return "item-" + hex.EncodeToString(sum[:])[:16]
}

func BuildRunID(input RunIdentityInput) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		RunLedgerSchemaVersion,
		canonicalInputPath(input.InputPath),
		string(input.InputBytes),
		input.MethodID,
		input.DestinationID,
	}, "\x00")))
	return "run-" + hex.EncodeToString(sum[:])[:16]
}

func InputFingerprint(inputBytes []byte) string {
	sum := sha256.Sum256(inputBytes)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func BuildManifest(input ManifestInput) Manifest {
	now := input.Now
	if strings.TrimSpace(now) == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}
	states := map[string]int{
		"published_preview":   0,
		"background":          0,
		"needs_enrichment":    0,
		"needs_clarification": 0,
		"blocked":             0,
		"skipped":             0,
	}
	reviewQueueCount := 0
	for _, item := range input.Items {
		states[item.State]++
		if item.ReviewRequired {
			reviewQueueCount++
		}
	}
	return Manifest{
		SchemaVersion:       RunLedgerSchemaVersion,
		RunID:               input.RunID,
		RunMode:             input.RunMode,
		PipelineSummaryPath: "pipeline-summary.json",
		MethodID:            input.MethodID,
		DestinationID:       input.DestinationID,
		InputFingerprint:    input.InputFingerprint,
		StartedAt:           now,
		CompletedAt:         now,
		ItemCount:           len(input.Items),
		ReviewQueueCount:    reviewQueueCount,
		States:              states,
		AuthorityIDs:        append([]string(nil), input.AuthorityIDs...),
	}
}

func BuildIndex(runID string, items []LedgerItem, authorityIDs []string) Index {
	index := Index{
		SchemaVersion: IndexSchemaVersion,
		RunID:         runID,
		ItemCount:     len(items),
		Items:         make([]IndexItem, 0, len(items)),
		AuthorityIDs:  append([]string(nil), authorityIDs...),
	}
	pathIDs := BuildUniquePathIDs(ledgerRecordIDs(items))
	for i, item := range items {
		recordID := BuildSafeID(item.RecordID)
		sourceCandidateID := BuildSafeID(item.SourceCandidateID)
		if sourceCandidateID == "" {
			sourceCandidateID = recordID
		}
		if item.ReviewRequired {
			index.ReviewRequiredCount++
		}
		index.Items = append(index.Items, IndexItem{
			RecordID:          recordID,
			SourceCandidateID: sourceCandidateID,
			State:             item.State,
			ReviewRequired:    item.ReviewRequired,
			LedgerItemPath:    cleanOutPath("items", pathIDs[i]+".json"),
		})
	}
	return index
}

func BuildReviewQueue(runID string, items []LedgerItem, authorityIDs []string) ReviewQueue {
	queue := ReviewQueue{
		SchemaVersion: ReviewQueueSchemaVersion,
		RunID:         runID,
		Items:         []ReviewQueueEntry{},
		AuthorityIDs:  append([]string(nil), authorityIDs...),
	}
	reviewItems := make([]LedgerItem, 0, len(items))
	for _, item := range items {
		if !item.ReviewRequired {
			continue
		}
		reviewItems = append(reviewItems, item)
	}
	pathIDs := BuildUniquePathIDs(ledgerRecordIDs(reviewItems))
	for i, item := range reviewItems {
		recordID := BuildSafeID(item.RecordID)
		entry := ReviewQueueEntry{
			RecordID:       recordID,
			State:          item.State,
			Reason:         safeReason(item.ReviewReason),
			ReviewItemPath: cleanOutPath("items", pathIDs[i]+".json"),
		}
		queue.Items = append(queue.Items, entry)
	}
	queue.QueueCount = len(queue.Items)
	return queue
}

func BuildReviewQueueItem(item LedgerItem, authorityIDs []string) ReviewQueueItem {
	recordID := BuildSafeID(item.RecordID)
	reason := safeReason(item.ReviewReason)
	return ReviewQueueItem{
		SchemaVersion:       ReviewItemSchemaVersion,
		RunID:               item.RunID,
		RecordID:            recordID,
		State:               item.State,
		Priority:            priorityForState(item.State),
		Reason:              reason,
		SuggestedNextAction: suggestedNextAction(item.State),
		SafeTitle:           item.SafeTitle,
		SafeContext:         item.SafeSummary,
		Links: map[string]string{
			"ledger_item":         cleanOutPath("ledger", "items", recordID+".json"),
			"pipeline_result":     item.PipelineResultPath,
			"processor_plan":      item.ProcessorPlanPath,
			"destination_summary": item.DestinationSummaryPath,
		},
		AuthorityIDs: append([]string(nil), authorityIDs...),
	}
}

func BuildUniquePathIDs(ids []string) []string {
	out := make([]string, len(ids))
	seen := map[string]int{}
	for i, id := range ids {
		base := BuildSafeID(id)
		seen[base]++
		if seen[base] == 1 {
			out[i] = base
			continue
		}
		out[i] = base + "-" + strconv.Itoa(seen[base])
	}
	return out
}

func ledgerRecordIDs(items []LedgerItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.RecordID)
	}
	return ids
}

func safeReason(reason string) string {
	if strings.Contains(reason, "PRIVATE_DM_SENTINEL_DO_NOT_WRITE") || strings.Contains(reason, "sk-test-secret-do-not-leak") {
		return "redacted"
	}
	return reason
}

func priorityForState(state string) string {
	switch state {
	case "blocked":
		return "high"
	case "needs_enrichment", "needs_clarification":
		return "normal"
	default:
		return "low"
	}
}

func suggestedNextAction(state string) string {
	switch state {
	case "needs_enrichment":
		return "Provide or run the missing local processor artifact before destination write."
	case "needs_clarification":
		return "Clarify save intent, classification, or destination visibility."
	case "blocked":
		return "Review blocker before retrying this item."
	default:
		return "Review item before retrying."
	}
}

func itemState(input ItemInput) (string, bool, string) {
	if input.SecretLike || hasBlocker(input.Blockers, "secret_like") {
		return "skipped", false, ""
	}
	for _, blocker := range input.Blockers {
		switch {
		case strings.Contains(blocker, "missing_local_"):
			return "needs_enrichment", true, blocker
		case strings.Contains(blocker, "clarification") || strings.Contains(blocker, "ambiguous"):
			return "needs_clarification", true, blocker
		}
	}
	if input.PrivateProvenance && len(input.Blockers) == 0 {
		return "background", false, ""
	}
	if len(input.Blockers) > 0 {
		return "blocked", true, input.Blockers[0]
	}
	if len(input.PreviewPaths) > 0 && input.PipelineState == "dry_run_published" {
		return "published_preview", false, ""
	}
	return "background", false, ""
}

func isSafeNativeID(value string) bool {
	if value == "" || strings.Contains(value, "..") || strings.ContainsAny(value, "/\\") {
		return false
	}
	if strings.Contains(value, "PRIVATE_DM_SENTINEL_DO_NOT_WRITE") || strings.Contains(value, "sk-test-secret-do-not-leak") {
		return false
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "private-dm") || strings.Contains(lower, "secret") {
		return false
	}
	spaceCount := 0
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		case r == ' ' || r == '\t' || r == '\n':
			spaceCount++
		default:
			return false
		}
	}
	return spaceCount == 0
}

func cleanOutPath(parts ...string) string {
	return filepath.ToSlash(filepath.Join(parts...))
}

func canonicalInputPath(path string) string {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return filepath.Clean(path)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs
	}
	return real
}

func safeOutputPath(path string, fallback string) string {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "..") || strings.Contains(path, `\`) {
		return fallback
	}
	if strings.Contains(path, "PRIVATE_DM_SENTINEL_DO_NOT_WRITE") || strings.Contains(path, "sk-test-secret-do-not-leak") {
		return fallback
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func safePreviewPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" || filepath.IsAbs(path) || strings.Contains(path, "..") || strings.Contains(path, `\`) {
			continue
		}
		if strings.Contains(path, "PRIVATE_DM_SENTINEL_DO_NOT_WRITE") || strings.Contains(path, "sk-test-secret-do-not-leak") {
			continue
		}
		out = append(out, filepath.ToSlash(filepath.Clean(path)))
	}
	return out
}

func safeBlockers(blockers []string, secretLike bool) []string {
	if secretLike {
		return []string{"secret_like_content_detected"}
	}
	out := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		if strings.Contains(blocker, "PRIVATE_DM_SENTINEL_DO_NOT_WRITE") || strings.Contains(blocker, "sk-test-secret-do-not-leak") {
			continue
		}
		out = append(out, blocker)
	}
	return out
}

func hasBlocker(blockers []string, needle string) bool {
	for _, blocker := range blockers {
		if strings.Contains(blocker, needle) {
			return true
		}
	}
	return false
}
