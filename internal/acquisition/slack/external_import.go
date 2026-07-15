package slack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/routing"
)

const (
	ExternalInventorySchema = "external_slack_inventory/v2"
	DefaultMaximumBytes     = int64(64 << 20)
	DataClassSynthetic      = "synthetic_fixture"
	DataClassSentinel       = "sentinel_fixture"
	DataClassPrivateRuntime = "private_runtime"
)

type ExternalManifest struct {
	SchemaVersion      string                         `json:"schema_version"`
	ContentFingerprint string                         `json:"content_fingerprint"`
	DataClass          string                         `json:"data_class"`
	SourceIdentity     acquisition.SourceIdentity     `json:"source_identity"`
	SourceScope        acquisition.SourceScope        `json:"source_scope"`
	Watermark          string                         `json:"watermark"`
	DeclaredCounts     acquisition.InventoryCounts    `json:"declared_counts"`
	SourceRecords      []acquisition.SourceRecord     `json:"source_records"`
	URLOccurrences     []acquisition.URLOccurrence    `json:"url_occurrences"`
	CanonicalItems     []acquisition.InventoryItem    `json:"canonical_items"`
	Strata             []acquisition.StratumCount     `json:"strata"`
	Completeness       []acquisition.EvidenceCheck    `json:"completeness"`
	ImportedEvidence   []acquisition.ImportedEvidence `json:"imported_evidence,omitempty"`
}

type ImportResult struct {
	Snapshot         acquisition.InventorySnapshot
	ImportedEvidence []acquisition.ImportedEvidence
	DeclaredCounts   acquisition.InventoryCounts
	ObservedCounts   acquisition.InventoryCounts
	SourceScope      acquisition.SourceScope
	DataClass        string
}

type ImportError struct {
	Category string
	Err      error
}

func (err *ImportError) Error() string { return "external Slack inventory rejected: " + err.Category }
func (err *ImportError) Unwrap() error { return err.Err }

func SealExternalManifest(manifest ExternalManifest) ExternalManifest {
	manifest.SchemaVersion = ExternalInventorySchema
	manifest.DeclaredCounts = acquisition.InventoryCounts{
		SourceRecords: len(manifest.SourceRecords), URLOccurrences: len(manifest.URLOccurrences), CanonicalItems: len(manifest.CanonicalItems),
	}
	manifest.ContentFingerprint = ""
	manifest.ContentFingerprint = manifestFingerprint(manifest)
	return manifest
}

func DecodeExternalInventory(reader io.Reader, maximumBytes int64) (ImportResult, error) {
	manifest, err := decodeExternalManifest(reader, maximumBytes)
	if err != nil {
		return ImportResult{}, err
	}
	return ValidateExternalManifest(manifest)
}

// DecodeAuthorizedExternalInventory is the sole private-input extension. It
// validates the exact commit/configuration-bound pre-live receipt before the
// private data class can cross the source-adapter boundary.
func DecodeAuthorizedExternalInventory(reader io.Reader, maximumBytes int64, receipt assurance.Receipt, expectedCommit, expectedConfiguration string, now time.Time, maxAge time.Duration) (ImportResult, error) {
	if err := assurance.Validate(receipt, expectedCommit, expectedConfiguration, now, maxAge); err != nil {
		return ImportResult{}, importError("pre_live_authority", err)
	}
	manifest, err := decodeExternalManifest(reader, maximumBytes)
	if err != nil {
		return ImportResult{}, err
	}
	return validateExternalManifest(manifest, true)
}

func decodeExternalManifest(reader io.Reader, maximumBytes int64) (ExternalManifest, error) {
	if reader == nil {
		return ExternalManifest{}, importError("invalid_input", errors.New("missing reader"))
	}
	if maximumBytes <= 0 {
		maximumBytes = DefaultMaximumBytes
	}
	bounded := io.LimitReader(reader, maximumBytes+1)
	payload, err := io.ReadAll(bounded)
	if err != nil {
		return ExternalManifest{}, importError("read_failed", err)
	}
	if int64(len(payload)) > maximumBytes {
		return ExternalManifest{}, importError("size_limit", errors.New("payload exceeds configured limit"))
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var manifest ExternalManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ExternalManifest{}, importError("invalid_json", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ExternalManifest{}, importError("trailing_data", errors.New("multiple JSON values"))
	}
	return manifest, nil
}

func ValidateExternalManifest(manifest ExternalManifest) (ImportResult, error) {
	return validateExternalManifest(manifest, false)
}

func validateExternalManifest(manifest ExternalManifest, privateAuthorized bool) (ImportResult, error) {
	if manifest.SchemaVersion != ExternalInventorySchema {
		if manifest.SchemaVersion == "external_slack_inventory/v1" {
			return ImportResult{}, importError("requires_rebuild_after_STD20", errors.New("pre-STD-20 inventory must be rebuilt from its native source"))
		}
		category := "unsupported_schema"
		if strings.Contains(strings.ToLower(manifest.SchemaVersion), "legacy") || strings.Contains(strings.ToLower(manifest.SchemaVersion), "canonical") {
			category = "canonical_only_legacy"
		}
		return ImportResult{}, importError(category, errors.New("schema does not prove occurrence completeness"))
	}
	if manifest.DataClass != DataClassSynthetic && manifest.DataClass != DataClassSentinel && !(privateAuthorized && manifest.DataClass == DataClassPrivateRuntime) {
		return ImportResult{}, importError("pre_live_private_input", errors.New("only synthetic or sentinel fixtures are accepted before the pre-live gate"))
	}
	if manifest.SourceIdentity.ConnectorKind != "external_slack_inventory" && manifest.SourceIdentity.ConnectorKind != "slack_web_api" || strings.TrimSpace(manifest.SourceIdentity.WorkspaceID) == "" || strings.TrimSpace(manifest.SourceIdentity.ChannelID) == "" {
		return ImportResult{}, importError("source_identity", errors.New("invalid source identity"))
	}
	expectedAdapter := ExternalInventorySchema
	if manifest.SourceIdentity.ConnectorKind == "slack_web_api" {
		expectedAdapter = WebAPIAdapterVersion
	}
	if manifest.SourceScope.ConnectorKind != manifest.SourceIdentity.ConnectorKind || manifest.SourceScope.WorkspaceID != manifest.SourceIdentity.WorkspaceID || manifest.SourceScope.ChannelID != manifest.SourceIdentity.ChannelID || manifest.SourceScope.AdapterVersion != expectedAdapter {
		return ImportResult{}, importError("scope_identity", errors.New("scope identity mismatch"))
	}
	if err := acquisition.ValidateSourceScope(manifest.SourceScope); err != nil {
		return ImportResult{}, importError("source_scope", err)
	}
	if manifest.ContentFingerprint == "" || manifest.ContentFingerprint != manifestFingerprint(manifest) {
		return ImportResult{}, importError("content_fingerprint", errors.New("content fingerprint mismatch"))
	}
	observed := acquisition.InventoryCounts{SourceRecords: len(manifest.SourceRecords), URLOccurrences: len(manifest.URLOccurrences), CanonicalItems: len(manifest.CanonicalItems)}
	if observed != manifest.DeclaredCounts {
		return ImportResult{}, importError("declared_counts", errors.New("declared and observed counts differ"))
	}
	for _, occurrence := range manifest.URLOccurrences {
		if occurrence.SanitizationState == routing.URLStorageSensitiveRedacted {
			expectedOccurrenceID := stableSourceID("occurrence", occurrence.SourceRecordID, stableIndex(occurrence.SourceOrdinal))
			expectedCanonicalID := stableSourceID("withheld", occurrence.SourceRecordID, stableIndex(occurrence.SourceOrdinal))
			if occurrence.SourceOrdinal < 0 || occurrence.ObservedURL != "" || occurrence.URLOccurrenceID != expectedOccurrenceID || occurrence.CanonicalItemID != expectedCanonicalID {
				return ImportResult{}, importError("sensitive_redaction_identity", errors.New("sensitive-redacted identity must derive only from source record and URL ordinal"))
			}
			continue
		}
		safeObserved, storageState, err := routing.PrepareURLForStorage(occurrence.ObservedURL)
		if err != nil || storageState == routing.URLStorageSensitiveRedacted || safeObserved != occurrence.ObservedURL {
			return ImportResult{}, importError("unsafe_observed_url", errors.New("observed URL contains unsafe durable components"))
		}
	}
	for _, item := range manifest.CanonicalItems {
		if item.AccessState == routing.URLStorageSensitiveRedacted && item.CanonicalURL == "" {
			continue
		}
		safeCanonical, storageState, err := routing.PrepareURLForStorage(item.CanonicalURL)
		if err != nil || storageState == routing.URLStorageSensitiveRedacted || safeCanonical != item.CanonicalURL {
			return ImportResult{}, importError("unsafe_canonical_url", errors.New("canonical URL contains unsafe durable components"))
		}
	}
	snapshot := acquisition.SealInventory(acquisition.InventorySnapshot{
		SourceIdentity: manifest.SourceIdentity.String(),
		Watermark:      manifest.Watermark,
		SourceRecords:  append([]acquisition.SourceRecord(nil), manifest.SourceRecords...),
		URLOccurrences: append([]acquisition.URLOccurrence(nil), manifest.URLOccurrences...),
		CanonicalItems: append([]acquisition.InventoryItem(nil), manifest.CanonicalItems...),
		Strata:         append([]acquisition.StratumCount(nil), manifest.Strata...),
		Completeness:   append([]acquisition.EvidenceCheck(nil), manifest.Completeness...),
	})
	if err := acquisition.ValidateInventory(snapshot); err != nil {
		return ImportResult{}, importError("inventory_invariants", err)
	}
	if err := acquisition.ValidateImportedEvidence(manifest.ImportedEvidence, snapshot); err != nil {
		return ImportResult{}, importError("imported_evidence", err)
	}
	if manifest.DataClass == DataClassPrivateRuntime {
		policies, err := validatePrivateSourceClassifications(manifest)
		if err != nil {
			return ImportResult{}, err
		}
		for _, evidence := range manifest.ImportedEvidence {
			if strings.TrimSpace(evidence.AccessClass) == "" || evidence.SecretLike {
				return ImportResult{}, importError("evidence_quarantine", errors.New("private-runtime evidence requires explicit safe access classification"))
			}
			if policies[evidence.CanonicalItemID] == importedEvidenceManualOnly || evidence.AccessClass != "public" {
				if !manualEvidenceEnvelope(evidence) {
					return ImportResult{}, importError("manual_evidence_required", errors.New("external private, authenticated, or unknown evidence must enter manual processing"))
				}
			}
		}
	}
	return ImportResult{
		Snapshot: snapshot, ImportedEvidence: append([]acquisition.ImportedEvidence(nil), manifest.ImportedEvidence...), DeclaredCounts: manifest.DeclaredCounts, ObservedCounts: observed,
		SourceScope: manifest.SourceScope, DataClass: manifest.DataClass,
	}, nil
}

func validatePrivateSourceClassifications(manifest ExternalManifest) (map[string]importedEvidenceAccessPolicy, error) {
	policies := make(map[string]importedEvidenceAccessPolicy, len(manifest.CanonicalItems))
	canonicalURLs := make(map[string]string, len(manifest.CanonicalItems))
	for _, item := range manifest.CanonicalItems {
		if item.AccessState == routing.URLStorageSensitiveRedacted {
			if item.CanonicalURL != "" || item.Kind != "unknown_sensitive" || item.RetrievalStrategy != "manual_support" || item.Format != "sensitive_redacted" {
				return nil, importError("source_classification", errors.New("invalid sensitive-redacted source classification"))
			}
			policies[item.CanonicalItemID] = importedEvidenceManualOnly
			continue
		}
		kind, strategy, format, policy := classifyExternalURLPolicy(item.CanonicalURL)
		if item.Kind != kind || item.RetrievalStrategy != strategy || item.Format != format {
			return nil, importError("source_classification", errors.New("canonical item classification does not match source-adapter policy"))
		}
		canonical, err := routing.CanonicalizeURL(item.CanonicalURL)
		if err != nil {
			return nil, importError("source_classification", errors.New("canonical item URL cannot be re-derived"))
		}
		policies[item.CanonicalItemID] = policy
		canonicalURLs[item.CanonicalItemID] = canonical
	}
	for _, occurrence := range manifest.URLOccurrences {
		if occurrence.SanitizationState == routing.URLStorageSensitiveRedacted {
			if policies[occurrence.CanonicalItemID] != importedEvidenceManualOnly || occurrence.ObservedURL != "" {
				return nil, importError("source_classification", errors.New("sensitive-redacted occurrence policy mismatch"))
			}
			continue
		}
		observed, err := routing.CanonicalizeURL(occurrence.ObservedURL)
		if err != nil || canonicalURLs[occurrence.CanonicalItemID] == "" || observed != canonicalURLs[occurrence.CanonicalItemID] {
			return nil, importError("source_classification", errors.New("observed URL does not match its canonical provider policy"))
		}
	}
	return policies, nil
}

func manualEvidenceEnvelope(evidence acquisition.ImportedEvidence) bool {
	if evidence.AccessClass == "public" || evidence.State == "complete" || evidence.State == "partial" || len(evidence.Excerpts) != 0 || len(evidence.RelatedURLs) != 0 || evidence.Metadata != (acquisition.ImportedMetadata{}) {
		return false
	}
	return len(evidence.Missingness) > 0
}

func manifestFingerprint(manifest ExternalManifest) string {
	manifest.ContentFingerprint = ""
	return acquisition.Fingerprint(manifest)
}

func importError(category string, err error) error {
	return &ImportError{Category: category, Err: fmt.Errorf("%s", category)}
}
