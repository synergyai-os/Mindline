package activationapp

import (
	"context"
	"io"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/assurance"
	"github.com/synergyai-os/Mindline/internal/controlrun"
	"github.com/synergyai-os/Mindline/internal/controlsettings"
	"github.com/synergyai-os/Mindline/internal/controlui"
	"github.com/synergyai-os/Mindline/internal/integrations"
	"github.com/synergyai-os/Mindline/internal/orchestration"
	"github.com/synergyai-os/Mindline/internal/processing"
	"github.com/synergyai-os/Mindline/internal/productbrain"
	"github.com/synergyai-os/Mindline/internal/retrieval"
	"github.com/synergyai-os/Mindline/internal/routing"
)

const (
	StateSchemaVersion        = "mindline-trusted-activation-state/v0.4"
	FounderReviewSchema       = "mindline-founder-review/v0.1"
	ZeroDeliveryReceiptSchema = "mindline-zero-delivery-receipt/v0.1"
	QueueProjectionSchema     = "mindline-frozen-activation-queue/v0.1"
	DrainPolicySchema         = "mindline-experimental-drain-policy/v0.1"
	RecoveryProofSchema       = "mindline-run-recovery-proof/v0.1"
	DrainConfirmationSchema   = "mindline-human-drain-confirmation/v0.1"
	DeliveryResumeSchema      = "mindline-human-delivery-resume/v0.1"
	SlackDrainWindowSchema    = "mindline-slack-drain-window/v0.1"
	defaultRunID              = "trusted-slack-activation"
)

type Options struct {
	ControlRoot              string
	RuntimeRoot              string
	RunID                    orchestration.RunID
	Commit                   string
	ConfigurationFingerprint string
	PreLiveReceipt           *assurance.Receipt
	SyntheticOnly            bool
	Now                      func() time.Time
	Connector                DestinationConnector
	SourceConnector          SlackSourceConnector
	SettingsRepository       controlsettings.RepositoryPort
	RunRepository            controlrun.RepositoryPort
	RunEntropy               io.Reader
}

type DestinationConnector interface {
	Connect(context.Context, *integrations.Registry, []byte) (*DestinationConnection, error)
}

type DestinationConnection struct {
	SessionRef integrations.SessionRef
	Snapshot   integrations.ConnectionSnapshot
	Capability productbrain.WorkspaceCapability
	Transport  productbrain.ProductBrainTransport
	Disconnect func() error
}

type SlackSourceConnector interface {
	Connect(context.Context, *integrations.Registry, []byte, string, acquisitionslack.SlackHTTPBudgets) (*SlackSourceConnection, error)
}

type SlackSourceConnection struct {
	SessionRef integrations.SessionRef
	Snapshot   integrations.ConnectionSnapshot
	Client     acquisitionslack.WebAPIClient
	Disconnect func() error
}

type persistedState struct {
	SchemaVersion         string                                `json:"schema_version"`
	Fingerprint           string                                `json:"fingerprint"`
	RunID                 orchestration.RunID                   `json:"run_id"`
	BuildCommit           string                                `json:"build_commit,omitempty"`
	Configuration         string                                `json:"configuration_fingerprint"`
	PreLiveReceipt        string                                `json:"pre_live_receipt_fingerprint,omitempty"`
	PreLiveAuthorizations []string                              `json:"pre_live_authorization_history,omitempty"`
	Inventory             *acquisition.InventorySnapshot        `json:"inventory,omitempty"`
	SourceScope           *acquisition.SourceScope              `json:"source_scope,omitempty"`
	SourceDataClass       string                                `json:"source_data_class,omitempty"`
	ImportAccounting      *ImportAccounting                     `json:"import_accounting,omitempty"`
	Evidence              []acquisition.ImportedEvidence        `json:"imported_evidence,omitempty"`
	Strategy              *processing.StrategySnapshot          `json:"strategy,omitempty"`
	DrainPolicy           *DrainPolicy                          `json:"drain_policy,omitempty"`
	Plan                  *orchestration.RunPlan                `json:"run_plan,omitempty"`
	Sample                *orchestration.SampleManifest         `json:"sample,omitempty"`
	Queue                 *QueueProjection                      `json:"queue,omitempty"`
	ProofInventory        *acquisition.InventorySnapshot        `json:"proof_inventory,omitempty"`
	Retrieval             []retrieval.Artifact                  `json:"retrieval,omitempty"`
	Proposals             []processing.Proposal                 `json:"proposals,omitempty"`
	Reviews               []processing.OperatorReviewRecord     `json:"reviews,omitempty"`
	Route                 *routing.Result                       `json:"route,omitempty"`
	Outbox                *productbrain.Outbox                  `json:"outbox,omitempty"`
	Preflight             *productbrain.PreflightArtifact       `json:"preflight,omitempty"`
	Preview               *controlui.BatchPreview               `json:"preview,omitempty"`
	Approval              *productbrain.DeliveryApproval        `json:"approval,omitempty"`
	HumanEvidence         *productbrain.HumanInitiationEvidence `json:"human_evidence,omitempty"`
	Delivery              *productbrain.ApprovedDeliveryReceipt `json:"delivery,omitempty"`
	Cancellation          *productbrain.CancellationReceipt     `json:"cancellation,omitempty"`
	ZeroDelivery          *ZeroDeliveryReceipt                  `json:"zero_delivery,omitempty"`
	FounderReview         *FounderReview                        `json:"founder_review,omitempty"`
	RecoveryProof         *RecoveryProof                        `json:"recovery_proof,omitempty"`
	DrainConfirmation     *DrainConfirmation                    `json:"drain_confirmation,omitempty"`
	DeliveryResume        *DeliveryResumeConsent                `json:"delivery_resume,omitempty"`
	KnownDestination      *integrations.ConnectionSnapshot      `json:"known_destination,omitempty"`
	KnownSource           *integrations.ConnectionSnapshot      `json:"known_source,omitempty"`
	SlackDrainWindow      *SlackDrainWindow                     `json:"slack_drain_window,omitempty"`
	SettingsVersion       uint64                                `json:"settings_version,omitempty"`
	SettingsGeneration    string                                `json:"settings_generation,omitempty"`
	SettingsFingerprint   string                                `json:"settings_fingerprint,omitempty"`
}

// SlackDrainWindow is the durable, non-secret source scope reserved before
// the first page is requested. Reconnects and retries must use this exact
// denominator so a restart cannot silently widen or roll the proof corpus.
type SlackDrainWindow struct {
	SchemaVersion string `json:"schema_version"`
	Fingerprint   string `json:"fingerprint"`
	WorkspaceID   string `json:"workspace_id"`
	ChannelID     string `json:"channel_id"`
	Oldest        string `json:"oldest"`
	Latest        string `json:"latest"`
	ReservedAt    string `json:"reserved_at"`
}

type ImportAccounting struct {
	FileName       string                      `json:"file_name"`
	FileBytes      int64                       `json:"file_bytes"`
	Declared       acquisition.InventoryCounts `json:"declared"`
	Observed       acquisition.InventoryCounts `json:"observed"`
	OmissionCount  int                         `json:"omission_count"`
	DuplicateCount int                         `json:"duplicate_occurrences"`
}

type DrainPolicy struct {
	SchemaVersion          string `json:"schema_version"`
	Fingerprint            string `json:"fingerprint"`
	MaximumNetworkRequests int    `json:"maximum_network_requests"`
	MaximumWallTimeSeconds int    `json:"maximum_wall_time_seconds"`
	MaximumCostMicrounits  int64  `json:"maximum_cost_microunits"`
	MaximumRetryAttempts   int    `json:"maximum_retry_attempts"`
	ManualSupportTolerance int    `json:"manual_support_tolerance"`
}

type RecoveryProof struct {
	SchemaVersion                string              `json:"schema_version"`
	Fingerprint                  string              `json:"fingerprint"`
	RunID                        orchestration.RunID `json:"run_id"`
	JournalVersion               uint64              `json:"journal_version"`
	RecoveredAuthorityProjection string              `json:"recovered_authority_projection"`
	InventoryFingerprint         string              `json:"inventory_fingerprint"`
	SampleFingerprint            string              `json:"sample_fingerprint"`
	QueueFingerprint             string              `json:"queue_fingerprint"`
	ProvenAt                     string              `json:"proven_at"`
}

type DrainConfirmation struct {
	SchemaVersion      string `json:"schema_version"`
	Fingerprint        string `json:"fingerprint"`
	RunPlanFingerprint string `json:"run_plan_fingerprint"`
	QueueFingerprint   string `json:"queue_fingerprint"`
	SessionFingerprint string `json:"session_fingerprint"`
	NonceFingerprint   string `json:"nonce_fingerprint"`
	ConfirmedAt        string `json:"confirmed_at"`
}

type DeliveryResumeConsent struct {
	SchemaVersion       string `json:"schema_version"`
	Fingerprint         string `json:"fingerprint"`
	BatchFingerprint    string `json:"batch_fingerprint"`
	ApprovalFingerprint string `json:"approval_fingerprint"`
	SessionFingerprint  string `json:"session_fingerprint"`
	NonceFingerprint    string `json:"nonce_fingerprint"`
	ResumedAt           string `json:"resumed_at"`
}

type QueueProjection struct {
	SchemaVersion        string      `json:"schema_version"`
	Fingerprint          string      `json:"fingerprint"`
	InventoryFingerprint string      `json:"inventory_fingerprint"`
	SampleFingerprint    string      `json:"sample_fingerprint"`
	Items                []QueueItem `json:"items"`
	SelectedCount        int         `json:"selected_count"`
	UnselectedCount      int         `json:"unselected_count"`
}

type QueueItem struct {
	CanonicalItemID string `json:"canonical_item_id"`
	Stratum         string `json:"stratum"`
	State           string `json:"state"`
}

type FounderReview struct {
	SchemaVersion       string                     `json:"schema_version"`
	Fingerprint         string                     `json:"fingerprint"`
	ReceiptFingerprint  string                     `json:"receipt_fingerprint,omitempty"`
	UsefulDraftIDs      []string                   `json:"useful_draft_ids,omitempty"`
	ValueVerdict        string                     `json:"value_verdict"`
	UsefulnessReason    string                     `json:"usefulness_reason,omitempty"`
	CredentialBurden    string                     `json:"credential_burden"`
	ManualSupportBurden string                     `json:"manual_support_burden,omitempty"`
	ApprovalBurden      string                     `json:"approval_burden"`
	ZeroDraft           bool                       `json:"zero_draft"`
	ReviewedAt          string                     `json:"reviewed_at"`
	DiscoveryMetrics    controlui.DiscoveryMetrics `json:"discovery_metrics"`
}

type ZeroDeliveryReceipt struct {
	SchemaVersion    string `json:"schema_version"`
	Fingerprint      string `json:"fingerprint"`
	BatchFingerprint string `json:"batch_fingerprint"`
	Status           string `json:"status"`
	RecordedAt       string `json:"recorded_at"`
}

type View struct {
	SchemaVersion    string                   `json:"schema_version"`
	Mode             string                   `json:"mode"`
	PreLiveReady     bool                     `json:"pre_live_ready"`
	Run              RunView                  `json:"run"`
	Connections      ConnectionView           `json:"connections"`
	Strategy         StrategyView             `json:"strategy"`
	Inventory        InventoryView            `json:"inventory"`
	Proof            ProofView                `json:"proof"`
	Destination      DestinationView          `json:"destination"`
	Founder          FounderView              `json:"founder"`
	Drain            DrainView                `json:"drain"`
	DeliveryInFlight bool                     `json:"delivery_in_flight"`
	Settings         controlsettings.Snapshot `json:"settings"`
	RunSelection     RunSelectionView         `json:"run_selection"`
	ActiveStrategy   ActiveStrategyView       `json:"active_strategy"`
}

type RunSelectionView struct {
	State              string `json:"state"`
	Version            uint64 `json:"version"`
	Generation         string `json:"generation"`
	SelectedRunID      string `json:"selected_run_id,omitempty"`
	SafePriorRun       string `json:"safe_prior_run_reference,omitempty"`
	ReasonCode         string `json:"reason_code,omitempty"`
	ProblemFingerprint string `json:"problem_fingerprint,omitempty"`
	ReadableVersion    uint64 `json:"readable_version,omitempty"`
	ReadableGeneration string `json:"readable_generation,omitempty"`
	BackupAvailable    bool   `json:"backup_available,omitempty"`
}

type ActiveStrategyView struct {
	State              string   `json:"state"`
	SettingsVersion    uint64   `json:"settings_version,omitempty"`
	SettingsGeneration string   `json:"settings_generation,omitempty"`
	Fingerprint        string   `json:"fingerprint,omitempty"`
	ExactLenses        []string `json:"context_lenses,omitempty"`
	RoutingPolicy      string   `json:"routing_policy,omitempty"`
}

type RunView struct {
	RunID   orchestration.RunID    `json:"run_id"`
	State   orchestration.RunState `json:"state,omitempty"`
	Version uint64                 `json:"version"`
}

type ConnectionView struct {
	SourceImported       bool                           `json:"source_imported"`
	SourceConnected      bool                           `json:"source_connected"`
	SourceIdentity       *integrations.VerifiedIdentity `json:"source_identity,omitempty"`
	DestinationConnected bool                           `json:"destination_connected"`
	DestinationIdentity  *integrations.VerifiedIdentity `json:"destination_identity,omitempty"`
	SessionOnly          bool                           `json:"credential_session_only"`
}

type StrategyView struct {
	Configured    bool         `json:"configured"`
	Fingerprint   string       `json:"fingerprint,omitempty"`
	ContextLenses string       `json:"context_lenses,omitempty"`
	RoutingPolicy string       `json:"routing_policy,omitempty"`
	DrainPolicy   *DrainPolicy `json:"drain_policy,omitempty"`
}

type InventoryView struct {
	FileName         string                        `json:"file_name,omitempty"`
	Frozen           bool                          `json:"frozen"`
	SourceRecords    int                           `json:"source_records"`
	URLOccurrences   int                           `json:"url_occurrences"`
	CanonicalItems   int                           `json:"canonical_items"`
	SelectedItems    int                           `json:"selected_items"`
	UnselectedItems  int                           `json:"unselected_items"`
	Strata           []orchestration.StratumSample `json:"strata,omitempty"`
	SourceIdentity   string                        `json:"source_identity,omitempty"`
	Watermark        string                        `json:"watermark,omitempty"`
	DataClass        string                        `json:"data_class,omitempty"`
	DuplicateCount   int                           `json:"duplicate_occurrences"`
	Completeness     []acquisition.EvidenceCheck   `json:"completeness,omitempty"`
	QueueFingerprint string                        `json:"queue_fingerprint,omitempty"`
	FileBytes        int64                         `json:"file_bytes"`
	DeclaredCounts   acquisition.InventoryCounts   `json:"declared_counts"`
	ObservedCounts   acquisition.InventoryCounts   `json:"observed_counts"`
	OmissionCount    int                           `json:"omission_count"`
}

type EvidenceExcerptView struct {
	Text    string `json:"text"`
	Locator string `json:"locator"`
}

type SourceReferenceView struct {
	NativeMessageID string `json:"native_message_id"`
	NativeTimestamp string `json:"native_timestamp"`
	URLOrdinal      int    `json:"url_ordinal"`
}

type ProofItemView struct {
	CanonicalItemID      string                  `json:"canonical_item_id"`
	CanonicalURL         string                  `json:"canonical_url"`
	Kind                 string                  `json:"kind"`
	RetrievalStrategy    string                  `json:"retrieval_strategy"`
	Format               string                  `json:"format"`
	RetrievalState       string                  `json:"retrieval_state,omitempty"`
	EvidenceOrigin       string                  `json:"evidence_origin,omitempty"`
	RequiresManualReview bool                    `json:"requires_manual_review"`
	Role                 string                  `json:"role,omitempty"`
	Disposition          string                  `json:"disposition,omitempty"`
	ReviewFingerprint    string                  `json:"review_fingerprint,omitempty"`
	Title                string                  `json:"title,omitempty"`
	Author               string                  `json:"author,omitempty"`
	Excerpts             []EvidenceExcerptView   `json:"excerpts,omitempty"`
	Missingness          []string                `json:"missingness,omitempty"`
	LensResults          []processing.LensResult `json:"lens_results,omitempty"`
	ProposedRole         string                  `json:"proposed_role,omitempty"`
	ProposedDisposition  string                  `json:"proposed_disposition,omitempty"`
	ProposedRationale    string                  `json:"proposed_rationale,omitempty"`
	ProposedSummary      string                  `json:"proposed_summary,omitempty"`
	ReasonCodes          []string                `json:"reason_codes,omitempty"`
	DestinationMapping   string                  `json:"destination_mapping,omitempty"`
	ReviewStatus         string                  `json:"review_status"`
	SourceReferences     []SourceReferenceView   `json:"source_references,omitempty"`
}

type ProofView struct {
	Started             bool            `json:"started"`
	Completed           bool            `json:"completed"`
	ItemCount           int             `json:"item_count"`
	ManualSupportCount  int             `json:"manual_support_count"`
	PromoteCount        int             `json:"promote_count"`
	ReviewedCount       int             `json:"reviewed_count"`
	AwaitingReviewCount int             `json:"awaiting_review_count"`
	Items               []ProofItemView `json:"items,omitempty"`
}

type DestinationView struct {
	OperationCount          int                     `json:"operation_count"`
	BatchPreview            *controlui.BatchPreview `json:"batch_preview,omitempty"`
	DeliveryStatus          string                  `json:"delivery_status,omitempty"`
	ReceiptFingerprint      string                  `json:"receipt_fingerprint,omitempty"`
	DraftIDs                []string                `json:"draft_ids,omitempty"`
	ApprovalFingerprint     string                  `json:"approval_fingerprint,omitempty"`
	CancellationFingerprint string                  `json:"cancellation_fingerprint,omitempty"`
}

type FounderView struct {
	ReviewRecorded              bool `json:"review_recorded"`
	TrustedActivationCompletion bool `json:"trusted_activation_completion"`
	TrustedValueObserved        bool `json:"trusted_value_observed"`
}

type DrainView struct {
	ExperimentalDrainAuthorized  bool                                    `json:"experimental_drain_authorized"`
	FullInventoryQueued          bool                                    `json:"full_inventory_queued"`
	RequiresExplicitConfirmation bool                                    `json:"requires_explicit_confirmation"`
	ProcessedRemainder           bool                                    `json:"processed_remainder"`
	Verdict                      orchestration.ReadinessVerdict          `json:"readiness"`
	Stages                       []orchestration.ReadinessVerdict        `json:"stages"`
	AuthorizationSentences       map[orchestration.ReadinessStage]string `json:"authorization_sentences"`
}

type saveStrategyPayload struct {
	ContextLenses          string `json:"context_lenses"`
	RoutingPolicy          string `json:"routing_policy"`
	MaximumNetworkRequests int    `json:"maximum_network_requests"`
	MaximumWallTimeSeconds int    `json:"maximum_wall_time_seconds"`
	MaximumCostMicrounits  int64  `json:"maximum_cost_microunits"`
	MaximumRetryAttempts   int    `json:"maximum_retry_attempts"`
	ManualSupportTolerance int    `json:"manual_support_tolerance"`
}

type useSettingsPayload struct {
	SettingsVersion     uint64 `json:"settings_version"`
	SettingsGeneration  string `json:"settings_generation"`
	SettingsFingerprint string `json:"settings_fingerprint"`
}

type reviewItemPayload struct {
	ItemID               string `json:"item_id"`
	Decision             string `json:"decision"`
	Role                 string `json:"role"`
	Disposition          string `json:"disposition"`
	Rationale            string `json:"rationale"`
	ManualSupportOutcome string `json:"manual_support_outcome"`
}

type founderReviewPayload struct {
	ReceiptFingerprint  string                     `json:"receipt_fingerprint"`
	UsefulDraftIDs      []string                   `json:"useful_draft_ids"`
	ValueVerdict        string                     `json:"value_verdict"`
	UsefulnessReason    string                     `json:"usefulness_reason"`
	CredentialBurden    string                     `json:"credential_burden"`
	ManualSupportBurden string                     `json:"manual_support_burden"`
	ApprovalBurden      string                     `json:"approval_burden"`
	ZeroDraft           bool                       `json:"zero_draft"`
	DiscoveryMetrics    controlui.DiscoveryMetrics `json:"discovery_metrics"`
}

type cancelPayload struct {
	ApprovalFingerprint string `json:"approval_fingerprint"`
}
