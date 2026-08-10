package recallproof

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/ingestioncontroller"
	"github.com/synergyai-os/Mindline/internal/localservice"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/resourcequeue"
)

const liveConfigurationSchema = "mindline-recall-live-configuration/v0.1"

type liveConfiguration struct {
	SchemaVersion string `json:"schema_version"`

	IngestionRunSchema     string `json:"ingestion_run_schema"`
	IngestionLedgerSchema  string `json:"ingestion_ledger_schema"`
	MaximumUnitBytes       int64  `json:"maximum_unit_bytes"`
	MaximumEnvelopeBytes   int64  `json:"maximum_envelope_bytes"`
	MaximumEnvelopeRecords int    `json:"maximum_envelope_records"`

	LibrarySchema                 string `json:"library_schema"`
	MaximumCaptureLibraryBytes    int64  `json:"maximum_capture_library_bytes"`
	MaximumLibraryBytes           int64  `json:"maximum_library_bytes"`
	MaximumExtractedContentBytes  int    `json:"maximum_extracted_content_bytes"`
	MaximumRepositoryContentBytes int64  `json:"maximum_repository_content_bytes"`
	MaximumRecords                int    `json:"maximum_records"`
	MaximumResources              int    `json:"maximum_resources"`
	MaximumQueueItems             int    `json:"maximum_queue_items"`
	MaximumQueueBytes             int64  `json:"maximum_queue_bytes"`

	MaximumLensRequestRunes      int `json:"maximum_lens_request_runes"`
	MaximumRetrievalContentBytes int `json:"maximum_retrieval_content_bytes"`
	MaximumCitationEvidenceRefs  int `json:"maximum_citation_evidence_refs"`
	MaximumCompactResourceStates int `json:"maximum_compact_resource_states"`
	MaximumCompactSnippetRunes   int `json:"maximum_compact_snippet_runes"`
	DefaultSearchLimit           int `json:"default_search_limit"`
	MaximumSearchLimit           int `json:"maximum_search_limit"`

	ResourceProfile resourcequeue.BudgetProfile            `json:"resource_profile"`
	CompactPolicy   personalmemory.CompactAbstentionPolicy `json:"compact_policy"`
	AgentState      string                                 `json:"agent_state_schema"`
	LocalConfig     string                                 `json:"local_config_schema"`
	LocalAPI        string                                 `json:"local_api_schema"`
	Capabilities    string                                 `json:"capabilities_schema"`
}

func currentLiveConfiguration() liveConfiguration {
	return liveConfiguration{
		SchemaVersion: liveConfigurationSchema,

		IngestionRunSchema:     ingestioncontroller.RunSchemaVersion,
		IngestionLedgerSchema:  ingestioncontroller.LedgerSchemaVersion,
		MaximumUnitBytes:       ingestioncontroller.MaximumUnitBytes,
		MaximumEnvelopeBytes:   ingestioncontroller.MaximumEnvelopeBytes,
		MaximumEnvelopeRecords: ingestioncontroller.MaximumEnvelopeRecords,

		LibrarySchema:                 personalmemory.LibrarySchemaVersion,
		MaximumCaptureLibraryBytes:    personalmemory.MaximumCaptureLibraryBytes,
		MaximumLibraryBytes:           personalmemory.MaximumLibraryBytes,
		MaximumExtractedContentBytes:  personalmemory.MaximumExtractedContentBytes,
		MaximumRepositoryContentBytes: personalmemory.MaximumRepositoryContentBytes,
		MaximumRecords:                personalmemory.MaximumRecords,
		MaximumResources:              personalmemory.MaximumResources,
		MaximumQueueItems:             resourcequeue.MaximumQueueItems,
		MaximumQueueBytes:             resourcequeue.MaximumQueueBytes,

		MaximumLensRequestRunes:      personalmemory.MaximumLensRequestRunes,
		MaximumRetrievalContentBytes: personalmemory.MaximumRetrievalContentBytes,
		MaximumCitationEvidenceRefs:  personalmemory.MaximumCitationEvidenceRefs,
		MaximumCompactResourceStates: personalmemory.MaximumCompactResourceStates,
		MaximumCompactSnippetRunes:   personalmemory.MaximumCompactSnippetRunes,
		DefaultSearchLimit:           personalmemory.DefaultSearchLimit,
		MaximumSearchLimit:           personalmemory.MaximumSearchLimit,

		ResourceProfile: resourcequeue.LiveProfile(),
		CompactPolicy:   personalmemory.DefaultCompactAbstentionPolicy(),
		AgentState:      agentstate.SchemaVersion,
		LocalConfig:     localservice.ConfigSchemaVersion,
		LocalAPI:        localservice.APISchemaVersion,
		Capabilities:    localservice.CapabilitiesSchemaVersion,
	}
}

// LiveConfigurationFingerprint binds every numeric ingestion, persistence,
// resource-processing, retrieval, and local-agent contract used by the live
// WP-48 run. It contains no path, source identity, credential, or evidence.
func LiveConfigurationFingerprint() string {
	payload, err := json.Marshal(currentLiveConfiguration())
	if err != nil {
		panic("marshal fixed live recall configuration: " + err.Error())
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
