package destinations

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

const OperationSchemaVersion = "destination-operation/v0.1"

type OperationType string

const (
	OperationCreateNote       OperationType = "create_note"
	OperationAttentionPreview OperationType = "attention_preview"
	OperationBackgroundRecord OperationType = "background_record"
	OperationSkip             OperationType = "skip"
	OperationBlocked          OperationType = "blocked"
)

type VisibilityLane string

const (
	VisibilityPublish    VisibilityLane = "publish"
	VisibilityAttention  VisibilityLane = "attention"
	VisibilityBackground VisibilityLane = "background"
	VisibilitySkip       VisibilityLane = "skip"
	VisibilityBlocked    VisibilityLane = "blocked"
)

type WriteMode string

const WriteModeDryRun WriteMode = "dry_run"

type Operation struct {
	SchemaVersion        string         `json:"schema_version"`
	OperationID          string         `json:"operation_id"`
	DestinationAdapterID string         `json:"destination_adapter_id"`
	SourceCandidateID    string         `json:"source_candidate_id"`
	SourceRecordID       string         `json:"source_record_id"`
	IdempotencyKey       string         `json:"idempotency_key"`
	OperationType        OperationType  `json:"operation_type"`
	WriteMode            WriteMode      `json:"write_mode"`
	VisibilityLane       VisibilityLane `json:"visibility_lane"`
	PlannedLocator       string         `json:"planned_locator"`
	Title                string         `json:"title"`
	Body                 string         `json:"body"`
	Metadata             map[string]any `json:"metadata"`
	Blockers             []string       `json:"blockers"`
	AuthorityIDs         []string       `json:"authority_ids"`
}

type Conflict struct {
	Field                  string `json:"field"`
	Value                  string `json:"value"`
	ConflictingOperationID string `json:"conflicting_operation_id"`
}

func ValidateOperation(operation Operation) error {
	required := []struct {
		name  string
		value string
	}{
		{"schema_version", operation.SchemaVersion},
		{"operation_id", operation.OperationID},
		{"destination_adapter_id", operation.DestinationAdapterID},
		{"source_candidate_id", operation.SourceCandidateID},
		{"source_record_id", operation.SourceRecordID},
		{"idempotency_key", operation.IdempotencyKey},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	if operation.SchemaVersion != OperationSchemaVersion {
		return fmt.Errorf("schema_version must be %q", OperationSchemaVersion)
	}
	if operation.WriteMode != WriteModeDryRun {
		return fmt.Errorf("write_mode must be %q", WriteModeDryRun)
	}
	if len(operation.AuthorityIDs) == 0 {
		return fmt.Errorf("authority_ids are required")
	}
	switch operation.OperationType {
	case OperationCreateNote, OperationAttentionPreview, OperationBackgroundRecord:
		if err := validateVisibilityLane(operation.OperationType, operation.VisibilityLane); err != nil {
			return err
		}
		if strings.TrimSpace(operation.PlannedLocator) == "" {
			return fmt.Errorf("planned_locator is required for %s", operation.OperationType)
		}
		if err := validatePlannedLocator(operation.PlannedLocator); err != nil {
			return err
		}
		if strings.TrimSpace(operation.Title) == "" {
			return fmt.Errorf("title is required for %s", operation.OperationType)
		}
		if strings.TrimSpace(operation.Body) == "" {
			return fmt.Errorf("body is required for %s", operation.OperationType)
		}
		if operation.OperationType == OperationCreateNote && len(operation.Blockers) > 0 {
			return fmt.Errorf("blockers must be empty for create_note")
		}
	case OperationSkip:
		if err := validateVisibilityLane(operation.OperationType, operation.VisibilityLane); err != nil {
			return err
		}
		if operation.PlannedLocator != "" {
			return fmt.Errorf("planned_locator must be empty for skip")
		}
		if operation.Title != "" {
			return fmt.Errorf("title must be empty for skip")
		}
		if operation.Body != "" {
			return fmt.Errorf("body must be empty for skip")
		}
		if len(operation.Blockers) == 0 {
			return fmt.Errorf("blockers are required for skip")
		}
	case OperationBlocked:
		if err := validateVisibilityLane(operation.OperationType, operation.VisibilityLane); err != nil {
			return err
		}
		if operation.PlannedLocator != "" {
			return fmt.Errorf("planned_locator must be empty for blocked")
		}
		if operation.Body != "" {
			return fmt.Errorf("body must be empty for blocked")
		}
		if len(operation.Blockers) == 0 {
			return fmt.Errorf("blockers are required for blocked")
		}
	default:
		return fmt.Errorf("operation_type is invalid: %q", operation.OperationType)
	}
	return nil
}

func validateVisibilityLane(operationType OperationType, lane VisibilityLane) error {
	want := map[OperationType]VisibilityLane{
		OperationCreateNote:       VisibilityPublish,
		OperationAttentionPreview: VisibilityAttention,
		OperationBackgroundRecord: VisibilityBackground,
		OperationSkip:             VisibilitySkip,
		OperationBlocked:          VisibilityBlocked,
	}[operationType]
	if lane != want {
		return fmt.Errorf("visibility_lane must be %q for %s", want, operationType)
	}
	return nil
}

func validatePlannedLocator(locator string) error {
	if filepath.IsAbs(locator) {
		return fmt.Errorf("planned_locator must be destination-relative")
	}
	normalized := filepath.ToSlash(filepath.Clean(locator))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") || strings.Contains(normalized, "/../") {
		return fmt.Errorf("planned_locator must not traverse parent directories")
	}
	return nil
}

func GenerateOperationID(destinationAdapterID, sourceCandidateID string, initialType OperationType) string {
	tuple := destinationAdapterID + "-" + sourceCandidateID + "-" + string(initialType)
	fingerprint := stableFingerprint(tuple)
	if isSensitive(tuple) {
		return sanitizeOperationIDBase(destinationAdapterID) + "-operation-" + fingerprint
	}
	return sanitizeOperationIDBase(tuple) + "-" + fingerprint
}

func GeneratePrivateOperationID(destinationAdapterID, sourceCandidateID string, initialType OperationType) string {
	tuple := destinationAdapterID + "-" + sourceCandidateID + "-" + string(initialType)
	return sanitizeOperationIDBase(destinationAdapterID) + "-operation-" + stableFingerprint(tuple)
}

func ResolveConflicts(operations []Operation) []Operation {
	resolved := make([]Operation, len(operations))
	copy(resolved, operations)

	locators := map[string]string{}
	idempotencyKeys := map[string]string{}

	for index := range resolved {
		operation := &resolved[index]
		var conflicts []Conflict
		if operation.PlannedLocator != "" {
			if conflictingID, ok := locators[operation.PlannedLocator]; ok {
				conflicts = append(conflicts, Conflict{
					Field:                  "planned_locator",
					Value:                  safeDiagnosticValue(operation.PlannedLocator),
					ConflictingOperationID: conflictingID,
				})
			} else {
				locators[operation.PlannedLocator] = operation.OperationID
			}
		}
		if operation.IdempotencyKey != "" {
			if conflictingID, ok := idempotencyKeys[operation.IdempotencyKey]; ok {
				conflicts = append(conflicts, Conflict{
					Field:                  "idempotency_key",
					Value:                  safeDiagnosticValue(operation.IdempotencyKey),
					ConflictingOperationID: conflictingID,
				})
			} else {
				idempotencyKeys[operation.IdempotencyKey] = operation.OperationID
			}
		}
		if len(conflicts) == 0 {
			continue
		}
		operation.OperationType = OperationBlocked
		operation.VisibilityLane = VisibilityBlocked
		operation.PlannedLocator = ""
		operation.Body = ""
		operation.IdempotencyKey = safeDiagnosticValue(operation.IdempotencyKey)
		operation.Blockers = addUnique(operation.Blockers, blockersForConflicts(conflicts)...)
		if operation.Metadata == nil {
			operation.Metadata = map[string]any{}
		}
		operation.Metadata["conflicts"] = conflicts
	}
	return resolved
}

func blockersForConflicts(conflicts []Conflict) []string {
	blockers := make([]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		blockers = append(blockers, "conflict:"+conflict.Field)
	}
	return blockers
}

func addUnique(values []string, additions ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, addition := range additions {
		if !seen[addition] {
			values = append(values, addition)
			seen[addition] = true
		}
	}
	return values
}

func safeDiagnosticValue(value string) string {
	if isSensitive(value) {
		return "fingerprint:" + stableFingerprint(value)
	}
	return value
}

func SafeDiagnosticValue(value string) string {
	return safeDiagnosticValue(value)
}

func IsSensitive(value string) bool {
	return isSensitive(value)
}

func StableFingerprint(value string) string {
	return stableFingerprint(value)
}

func isSensitive(value string) bool {
	lower := strings.ToLower(value)
	sentinels := []string{
		"xoxb-",
		"sk_live_",
		"password=",
		"api_key=",
		"bearer ",
		"private.example",
		"super-secret",
		"files-pri",
		"slack-file-private",
	}
	for _, sentinel := range sentinels {
		if strings.Contains(lower, sentinel) {
			return true
		}
	}
	return false
}

func stableFingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func sanitizeOperationIDBase(value string) string {
	var b strings.Builder
	lastWasDash := false
	for _, r := range value {
		allowed := unicode.IsLetter(r) || unicode.IsDigit(r)
		if allowed {
			b.WriteRune(r)
			lastWasDash = false
			continue
		}
		if !lastWasDash {
			b.WriteRune('-')
			lastWasDash = true
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		return "operation"
	}
	return cleaned
}
