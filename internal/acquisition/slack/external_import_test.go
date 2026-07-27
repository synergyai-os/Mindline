package slack

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/assurance"
)

func TestDecodeExternalInventoryPreservesOccurrenceCompleteAccounting(t *testing.T) {
	manifest := validManifest()
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	result, err := DecodeExternalInventory(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.OccurrenceCount != 2 || result.Snapshot.CanonicalCount != 1 || len(result.Snapshot.SourceRecords) != 2 {
		t.Fatalf("occurrence denominator collapsed: %+v", result.Snapshot)
	}
	if result.Snapshot.CanonicalItems[0].URLOccurrenceIDs[0] != "occ-1" || result.Snapshot.CanonicalItems[0].URLOccurrenceIDs[1] != "occ-2" {
		t.Fatalf("reverse occurrence accounting changed: %+v", result.Snapshot.CanonicalItems[0])
	}
	if result.Snapshot.SchemaVersion != acquisition.InventorySnapshotSchema {
		t.Fatalf("external v2 did not adopt the current normalized inventory schema: %s", result.Snapshot.SchemaVersion)
	}
}

func TestExternalInventoryV1RequiresNativeRebuild(t *testing.T) {
	for _, singleOccurrence := range []bool{true, false} {
		manifest := validManifest()
		if singleOccurrence {
			manifest.SourceRecords = manifest.SourceRecords[:1]
			manifest.URLOccurrences = manifest.URLOccurrences[:1]
			manifest.CanonicalItems[0].URLOccurrenceIDs = manifest.CanonicalItems[0].URLOccurrenceIDs[:1]
		}
		manifest.SchemaVersion = "external_slack_inventory/v1"
		_, err := ValidateExternalManifest(manifest)
		var importErr *ImportError
		if !errors.As(err, &importErr) || importErr.Category != "requires_rebuild_after_STD20" {
			t.Fatalf("pre-STD-20 manifest was not rejected at the schema boundary (single=%t): %v", singleOccurrence, err)
		}
	}
}

func TestDecodeExternalInventoryRejectsUnknownTrailingAndOrphanData(t *testing.T) {
	manifest := validManifest()
	payload, _ := json.Marshal(manifest)
	unknown := append(payload[:len(payload)-1], []byte(`,"credential":"SENTINEL"}`)...)
	if _, err := DecodeExternalInventory(bytes.NewReader(unknown), int64(len(unknown))); err == nil {
		t.Fatal("unknown field was accepted")
	}
	if _, err := DecodeExternalInventory(bytes.NewReader(append(payload, []byte(` {}`)...)), int64(len(payload)+3)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
	manifest = validManifest()
	manifest.SourceRecords[1].URLOccurrenceIDs = nil
	manifest = SealExternalManifest(manifest)
	if _, err := ValidateExternalManifest(manifest); err == nil {
		t.Fatal("orphan occurrence was accepted")
	}
	manifest.SchemaVersion = "canonical_only_legacy/v0.1"
	if _, err := ValidateExternalManifest(manifest); err == nil {
		t.Fatal("canonical-only legacy inventory was accepted")
	} else {
		var importErr *ImportError
		if !errors.As(err, &importErr) || importErr.Category != "canonical_only_legacy" {
			t.Fatalf("wrong legacy failure category: %v", err)
		}
	}
}

func TestExternalInventoryRejectsUnsanitizedObservedURL(t *testing.T) {
	manifest := validManifest()
	manifest.URLOccurrences[0].ObservedURL = "https://example.com/article?token=synthetic-value"
	manifest = SealExternalManifest(manifest)
	_, err := ValidateExternalManifest(manifest)
	var importErr *ImportError
	if !errors.As(err, &importErr) || importErr.Category != "unsafe_observed_url" {
		t.Fatalf("unsafe observed URL was not rejected at import: %v", err)
	}
}

func TestExternalInventoryRejectsUnsanitizedCanonicalURL(t *testing.T) {
	manifest := validManifest()
	unsafe := "https://example.com/article?token=synthetic-value"
	manifest.CanonicalItems[0].CanonicalURL = unsafe
	for index := range manifest.URLOccurrences {
		manifest.URLOccurrences[index].ObservedURL = "https://example.com/article"
	}
	manifest = SealExternalManifest(manifest)
	_, err := ValidateExternalManifest(manifest)
	var importErr *ImportError
	if !errors.As(err, &importErr) || importErr.Category != "unsafe_canonical_url" {
		t.Fatalf("unsafe canonical URL was not rejected at import: %v", err)
	}
}

func TestExternalInventoryRejectsEvidenceOnSensitiveRedactedItem(t *testing.T) {
	manifest, err := BuildExternalManifest(BuildInput{
		WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001", DataClass: DataClassSynthetic,
		Messages: []NativeMessage{{NativeMessageID: "m-1", Timestamp: "1700000000.000001", Text: "https://example.com/shared?token=synthetic-value"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := manifest.CanonicalItems[0]
	manifest.ImportedEvidence = []acquisition.ImportedEvidence{{
		CanonicalItemID: item.CanonicalItemID, State: "not_attempted", AccessClass: "unsupported", Missingness: []string{"free-form source material"},
	}}
	manifest = SealExternalManifest(manifest)
	_, err = ValidateExternalManifest(manifest)
	var importErr *ImportError
	if !errors.As(err, &importErr) || importErr.Category != "imported_evidence" {
		t.Fatalf("sensitive-redacted imported evidence was not rejected: %v", err)
	}
}

func TestExternalInventoryRejectsTamperedSensitiveRedactionIdentity(t *testing.T) {
	manifest, err := BuildExternalManifest(BuildInput{
		WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001", DataClass: DataClassSynthetic,
		Messages: []NativeMessage{{NativeMessageID: "m-1", Timestamp: "1700000000.000001", Text: "https://example.com/shared?token=synthetic-value"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest.URLOccurrences[0].SourceOrdinal++
	manifest = SealExternalManifest(manifest)
	_, err = ValidateExternalManifest(manifest)
	var importErr *ImportError
	if !errors.As(err, &importErr) || importErr.Category != "sensitive_redaction_identity" {
		t.Fatalf("tampered sensitive-redacted ordinal was not rejected: %v", err)
	}
}

func TestExternalInventoryRejectsRedactedItemWithOrdinaryOccurrence(t *testing.T) {
	manifest, err := BuildExternalManifest(BuildInput{
		WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", LowerInclusive: "1700000000.000001", UpperInclusive: "1700000000.000001", Watermark: "1700000000.000001", DataClass: DataClassSynthetic,
		Messages: []NativeMessage{{NativeMessageID: "m-1", Timestamp: "1700000000.000001", Text: "https://example.com/shared?token=synthetic-value"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest.URLOccurrences[0].SanitizationState = ""
	manifest.URLOccurrences[0].ObservedURL = "https://example.com/public"
	manifest = SealExternalManifest(manifest)
	_, err = ValidateExternalManifest(manifest)
	var importErr *ImportError
	if !errors.As(err, &importErr) || importErr.Category != "inventory_invariants" {
		t.Fatalf("ordinary occurrence was allowed to reference a redacted item: %v", err)
	}
}

func TestExternalInventoryRejectsUnsafeRelatedURL(t *testing.T) {
	manifest := validManifest()
	evidence := publicCompleteEvidence(manifest.CanonicalItems[0])
	evidence.RelatedURLs = []acquisition.ImportedRelated{{
		URL: "https://example.com/related/" + "xoxb" + "-synthetic", Relation: "source_links_to", DiscoveryEvidenceRef: "excerpt-1", SemanticallyRelevant: true,
	}}
	manifest.ImportedEvidence = []acquisition.ImportedEvidence{evidence}
	manifest = SealExternalManifest(manifest)
	_, err := ValidateExternalManifest(manifest)
	var importErr *ImportError
	if !errors.As(err, &importErr) || importErr.Category != "imported_evidence" {
		t.Fatalf("unsafe related URL crossed the acquisition boundary: %v", err)
	}
}

func TestExternalInventoryRejectsPrivateInputBeforeGate(t *testing.T) {
	manifest := validManifest()
	manifest.DataClass = "private_founder"
	manifest = SealExternalManifest(manifest)
	_, err := ValidateExternalManifest(manifest)
	var importErr *ImportError
	if !errors.As(err, &importErr) || importErr.Category != "pre_live_private_input" {
		t.Fatalf("private input was not rejected at the pre-live boundary: %v", err)
	}
}

func TestAuthorizedExternalInventoryRequiresExactPreLiveReceipt(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	manifest := validManifest()
	manifest.DataClass = DataClassPrivateRuntime
	manifest = SealExternalManifest(manifest)
	payload, _ := json.Marshal(manifest)
	checks := make([]assurance.Check, 0, len(assurance.RequiredChecks))
	for _, name := range assurance.RequiredChecks {
		checks = append(checks, assurance.Check{Name: name, ToolVersion: "test", Outcome: "pass", EvidenceFingerprint: "sha256:test-" + name})
	}
	receipt, err := assurance.Build("commit-1", "config-1", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now, checks)
	if err != nil {
		t.Fatal(err)
	}
	result, err := DecodeAuthorizedExternalInventory(bytes.NewReader(payload), int64(len(payload)), receipt, "commit-1", "config-1")
	if err != nil || result.DataClass != DataClassPrivateRuntime {
		t.Fatalf("authorized private inventory failed: result=%+v err=%v", result, err)
	}
	if _, err := DecodeAuthorizedExternalInventory(bytes.NewReader(payload), int64(len(payload)), receipt, "commit-drift", "config-1"); err == nil {
		t.Fatal("commit-drifted private authority was accepted")
	}
}

func TestAuthorizedExternalInventoryRejectsCallerControlledPrivateClassification(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	receipt := authorizedTestReceipt(t, now)
	for _, target := range []string{
		"https://workspace.notion.so/private-page",
		"https://acme.slack.com/archives/C-private/p123",
		"https://docs.google.com/document/d/private-id/edit",
		"https://drive.google.com/file/d/private-id/view",
	} {
		t.Run(target, func(t *testing.T) {
			manifest := oneItemManifest(target, "generic_web", "generic_web", "web_page")
			manifest.ImportedEvidence = []acquisition.ImportedEvidence{publicCompleteEvidence(manifest.CanonicalItems[0])}
			manifest.DataClass = DataClassPrivateRuntime
			manifest = SealExternalManifest(manifest)
			payload, _ := json.Marshal(manifest)
			_, err := DecodeAuthorizedExternalInventory(bytes.NewReader(payload), int64(len(payload)), receipt, "commit-1", "config-1")
			var importErr *ImportError
			if !errors.As(err, &importErr) || importErr.Category != "source_classification" {
				t.Fatalf("caller-controlled private classification was accepted: %v", err)
			}
		})
	}
}

func TestAuthorizedExternalInventoryRequiresManualEvidenceForAuthenticatedAndUnknownProviders(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	receipt := authorizedTestReceipt(t, now)
	for _, target := range []string{"https://workspace.notion.so/private-page", "https://private-like.example/internal/page"} {
		t.Run(target, func(t *testing.T) {
			kind, strategy, format, _ := classifyExternalURLPolicy(target)
			manifest := oneItemManifest(target, kind, strategy, format)
			manifest.ImportedEvidence = []acquisition.ImportedEvidence{publicCompleteEvidence(manifest.CanonicalItems[0])}
			manifest.DataClass = DataClassPrivateRuntime
			manifest = SealExternalManifest(manifest)
			payload, _ := json.Marshal(manifest)
			_, err := DecodeAuthorizedExternalInventory(bytes.NewReader(payload), int64(len(payload)), receipt, "commit-1", "config-1")
			var importErr *ImportError
			if !errors.As(err, &importErr) || importErr.Category != "manual_evidence_required" {
				t.Fatalf("public complete evidence bypassed manual policy: %v", err)
			}
		})
	}
}

func TestAuthorizedExternalInventoryRejectsPrivateObservedURLMappedToPublicCanonicalItem(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	receipt := authorizedTestReceipt(t, now)
	manifest := validManifest()
	manifest.URLOccurrences[0].ObservedURL = "https://workspace.notion.so/private-page"
	manifest.ImportedEvidence = []acquisition.ImportedEvidence{publicCompleteEvidence(manifest.CanonicalItems[0])}
	manifest.DataClass = DataClassPrivateRuntime
	manifest = SealExternalManifest(manifest)
	payload, _ := json.Marshal(manifest)
	_, err := DecodeAuthorizedExternalInventory(bytes.NewReader(payload), int64(len(payload)), receipt, "commit-1", "config-1")
	var importErr *ImportError
	if !errors.As(err, &importErr) || importErr.Category != "source_classification" {
		t.Fatalf("private observed URL was laundered through a public canonical item: %v", err)
	}
}

func TestAuthorizedExternalInventoryAllowsDerivedPublicAndManualEvidence(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	receipt := authorizedTestReceipt(t, now)
	tests := []struct {
		target   string
		evidence func(acquisition.InventoryItem) acquisition.ImportedEvidence
	}{
		{target: "https://github.com/acme/tool", evidence: publicCompleteEvidence},
		{target: "https://workspace.notion.so/private-page", evidence: manualImportedEvidence},
		{target: "https://unknown.example/page", evidence: manualImportedEvidence},
	}
	for _, test := range tests {
		kind, strategy, format, _ := classifyExternalURLPolicy(test.target)
		manifest := oneItemManifest(test.target, kind, strategy, format)
		manifest.ImportedEvidence = []acquisition.ImportedEvidence{test.evidence(manifest.CanonicalItems[0])}
		manifest.DataClass = DataClassPrivateRuntime
		manifest = SealExternalManifest(manifest)
		payload, _ := json.Marshal(manifest)
		if _, err := DecodeAuthorizedExternalInventory(bytes.NewReader(payload), int64(len(payload)), receipt, "commit-1", "config-1"); err != nil {
			t.Fatalf("derived provider policy rejected safe evidence for %s: %v", test.target, err)
		}
	}
}

func authorizedTestReceipt(t *testing.T, now time.Time) assurance.Receipt {
	t.Helper()
	checks := make([]assurance.Check, 0, len(assurance.RequiredChecks))
	for _, name := range assurance.RequiredChecks {
		checks = append(checks, assurance.Check{Name: name, ToolVersion: "test", Outcome: "pass", EvidenceFingerprint: "sha256:test-" + name})
	}
	receipt, err := assurance.Build("commit-1", "config-1", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", now, checks)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func oneItemManifest(target, kind, strategy, format string) ExternalManifest {
	manifest := validManifest()
	item := &manifest.CanonicalItems[0]
	item.CanonicalURL, item.Kind, item.RetrievalStrategy, item.Format = target, kind, strategy, format
	manifest.URLOccurrences[0].ObservedURL = target
	manifest.URLOccurrences[1].ObservedURL = target
	manifest.Strata = acquisition.BuildStrata(manifest.CanonicalItems)
	return manifest
}

func publicCompleteEvidence(item acquisition.InventoryItem) acquisition.ImportedEvidence {
	return acquisition.ImportedEvidence{
		CanonicalItemID: item.CanonicalItemID, CanonicalURL: item.CanonicalURL, State: "complete", RetrievedAt: "2026-07-14T11:00:00Z", AccessClass: "public",
		Metadata: acquisition.ImportedMetadata{Title: "Private-looking source", Author: "External author"},
		Excerpts: []acquisition.ImportedExcerpt{{ExcerptID: "excerpt-1", Text: "Imported evidence must not choose its own access policy.", Locator: "page"}},
	}
}

func manualImportedEvidence(item acquisition.InventoryItem) acquisition.ImportedEvidence {
	return acquisition.ImportedEvidence{
		CanonicalItemID: item.CanonicalItemID, CanonicalURL: item.CanonicalURL, State: "inaccessible", AccessClass: "authenticated", Missingness: []string{"authentication required"},
	}
}

func validManifest() ExternalManifest {
	canonical := "https://example.com/article"
	digest := sha256.Sum256([]byte("synthetic-source"))
	scope := acquisition.SealSourceScope(acquisition.SourceScope{
		ConnectorKind: "external_slack_inventory", WorkspaceID: "T-synthetic", ChannelID: "C-synthetic",
		LowerInclusive: "1700000000.000001", UpperInclusive: "1700000001.000001", IncludeThreads: true, IncludeReplies: true,
		AttachmentPolicy: "metadata_only", PrivateFilePolicy: "manual", EditDeletePolicy: "account", AdapterVersion: ExternalInventorySchema,
	})
	manifest := ExternalManifest{
		DataClass:      DataClassSynthetic,
		SourceIdentity: acquisition.SourceIdentity{ConnectorKind: "external_slack_inventory", WorkspaceID: "T-synthetic", ChannelID: "C-synthetic"},
		SourceScope:    scope, Watermark: "1700000001.000001",
		SourceRecords: []acquisition.SourceRecord{
			{SourceRecordID: "source-1", NativeMessageID: "message-1", NativeTimestamp: "1700000000.000001", ContentFingerprint: hex.EncodeToString(digest[:]), URLOccurrenceIDs: []string{"occ-1"}, EditDeleteState: "original"},
			{SourceRecordID: "source-2", NativeMessageID: "message-2", NativeTimestamp: "1700000001.000001", ContentFingerprint: hex.EncodeToString(digest[:]), URLOccurrenceIDs: []string{"occ-2"}, EditDeleteState: "original"},
		},
		URLOccurrences: []acquisition.URLOccurrence{
			{URLOccurrenceID: "occ-1", SourceRecordID: "source-1", ObservedURL: canonical, CanonicalItemID: "canonical-1"},
			{URLOccurrenceID: "occ-2", SourceRecordID: "source-2", ObservedURL: canonical, CanonicalItemID: "canonical-1"},
		},
		CanonicalItems: []acquisition.InventoryItem{{CanonicalItemID: "canonical-1", CanonicalURL: canonical, Kind: "generic_web", URLOccurrenceIDs: []string{"occ-1", "occ-2"}, RetrievalStrategy: "generic_web", Format: "web_page"}},
		Strata:         []acquisition.StratumCount{{RetrievalStrategy: "generic_web", Format: "web_page", Count: 1}},
		Completeness:   []acquisition.EvidenceCheck{{Check: "occurrence_accounting", Status: "pass", Count: 2}},
	}
	return SealExternalManifest(manifest)
}
