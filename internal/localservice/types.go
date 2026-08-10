package localservice

import (
	"github.com/synergyai-os/Mindline/internal/agentcontract"
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

type ScopedSearchInput struct {
	Query   string `json:"query"`
	ScopeID string `json:"scope_id"`
	LensID  string `json:"lens_id"`
	AgentID string `json:"agent_id"`
	Limit   int    `json:"limit,omitempty"`
}

type ScopedGetInput struct {
	RunID    string `json:"run_id"`
	ScopeID  string `json:"scope_id"`
	LensID   string `json:"lens_id"`
	AgentID  string `json:"agent_id"`
	RecordID string `json:"record_id"`
}

type AgentRegistrationInput struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
}

const (
	CapabilitiesSchemaVersion   = "mindline-agent-capabilities/v0.1"
	ScopedRecallCapability      = "mindline.scoped-recall.v0.4"
	DiscoveryCapability         = agentcontract.DiscoveryCapability
	AgentRegistrationCapability = agentcontract.AgentRegistrationCapability
	RecommendedAgentRoute       = agentcontract.RecommendedRoute
	OwnerDebugRouteClass        = agentcontract.OwnerDebugRouteClass
	ScopedHydrationEndpoint     = agentcontract.ScopedHydrationEndpoint
)

type Capabilities struct {
	SchemaVersion                string                                 `json:"schema_version"`
	SearchFormats                []string                               `json:"search_formats"`
	CompactSearchEndpoint        string                                 `json:"compact_search_endpoint"`
	CompactAbstentionPolicy      personalmemory.CompactAbstentionPolicy `json:"compact_abstention_policy"`
	ExplicitHydrationCommand     string                                 `json:"explicit_hydration_command"`
	FeedbackRetryToken           bool                                   `json:"feedback_retry_token"`
	Features                     []string                               `json:"features,omitempty"`
	ScopedSearchEndpoint         string                                 `json:"scoped_search_endpoint,omitempty"`
	ScopedFeedbackEndpoint       string                                 `json:"scoped_feedback_endpoint,omitempty"`
	ScopedHydrationEndpoint      string                                 `json:"scoped_hydration_endpoint,omitempty"`
	AgentRegistrationEndpoint    string                                 `json:"agent_registration_endpoint,omitempty"`
	RecommendedAgentRoute        string                                 `json:"recommended_agent_route"`
	OwnerDebugRouteClass         string                                 `json:"owner_debug_route_class"`
	IdentityAssurance            string                                 `json:"identity_assurance"`
	HostileProcessAuthentication bool                                   `json:"hostile_process_authentication"`
	OwnerMutationEnforcement     string                                 `json:"owner_mutation_enforcement"`
	FeedbackTokenCommand         string                                 `json:"feedback_token_command"`
	RegistrationTokenCommand     string                                 `json:"registration_token_command,omitempty"`
}
type Status struct {
	SchemaVersion  string                 `json:"schema_version"`
	ServiceState   string                 `json:"service_state"`
	Memory         personalmemory.Status  `json:"memory"`
	State          PublicAgentStateStatus `json:"state"`
	SemanticIndex  SemanticIndexStatus    `json:"semantic_index"`
	RuntimeBinding RuntimeBinding         `json:"runtime_binding"`
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
	SchemaVersion           string `json:"schema_version"`
	LensCount               int    `json:"lens_count"`
	RetrievalRunCount       int    `json:"retrieval_run_count"`
	JudgmentCount           int    `json:"judgment_count"`
	ScopeCount              int    `json:"scope_count"`
	ScopedLensCount         int    `json:"scoped_lens_count"`
	AgentActorCount         int    `json:"agent_actor_count"`
	ScopedRetrievalRunCount int    `json:"scoped_retrieval_run_count"`
	ScopedJudgmentCount     int    `json:"scoped_judgment_count"`
	EmbeddingCount          int    `json:"embedding_count"`
	IndexedFingerprint      string `json:"indexed_library_fingerprint,omitempty"`
	RecoveryState           string `json:"recovery_state,omitempty"`
}

type DeleteResult struct {
	Deleted bool `json:"deleted"`
}
