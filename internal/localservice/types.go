package localservice

import (
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

type Envelope struct {
	SchemaVersion string `json:"schema_version"`
	Data          any    `json:"data,omitempty"`
	Error         string `json:"error,omitempty"`
}

type SearchInput struct {
	Query  string `json:"query"`
	LensID string `json:"lens_id,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

const CapabilitiesSchemaVersion = "mindline-agent-capabilities/v0.1"

type Capabilities struct {
	SchemaVersion            string                                 `json:"schema_version"`
	SearchFormats            []string                               `json:"search_formats"`
	CompactSearchEndpoint    string                                 `json:"compact_search_endpoint"`
	CompactAbstentionPolicy  personalmemory.CompactAbstentionPolicy `json:"compact_abstention_policy"`
	ExplicitHydrationCommand string                                 `json:"explicit_hydration_command"`
	FeedbackRetryToken       bool                                   `json:"feedback_retry_token"`
}

type Status struct {
	SchemaVersion string                 `json:"schema_version"`
	ServiceState  string                 `json:"service_state"`
	Memory        personalmemory.Status  `json:"memory"`
	State         PublicAgentStateStatus `json:"state"`
	SemanticIndex SemanticIndexStatus    `json:"semantic_index"`
}

type SemanticIndexStatus struct {
	State              string `json:"state"`
	LibraryFingerprint string `json:"library_fingerprint"`
	IndexedFingerprint string `json:"indexed_fingerprint,omitempty"`
	Completed          int    `json:"completed"`
	Target             int    `json:"target"`
	Reason             string `json:"reason,omitempty"`
}

type PublicAgentStateStatus struct {
	SchemaVersion      string `json:"schema_version"`
	LensCount          int    `json:"lens_count"`
	RetrievalRunCount  int    `json:"retrieval_run_count"`
	JudgmentCount      int    `json:"judgment_count"`
	EmbeddingCount     int    `json:"embedding_count"`
	IndexedFingerprint string `json:"indexed_library_fingerprint,omitempty"`
	RecoveryState      string `json:"recovery_state,omitempty"`
}

type DeleteResult struct {
	Deleted bool `json:"deleted"`
}
