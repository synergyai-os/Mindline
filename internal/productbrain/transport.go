package productbrain

import "context"

const ProductionGatewayOrigin = "https://gateway.productbrain.io"

// safeDeliveryCategories is the single closed diagnostic vocabulary allowed to
// cross the Product Brain transport boundary or be sealed on delivery
// operations. Validation and storage errors that occur before an operation is
// attempted are returned directly and never become operation diagnostics.
var safeDeliveryCategories = []string{
	"credential_missing",
	"untrusted_product_brain_origin",
	"unauthorized",
	"forbidden",
	"workspace_mismatch",
	"capability_missing",
	"collection_contract_mismatch",
	"not_found",
	"already_exists",
	"validation_failed",
	"rate_limited",
	"transient",
	"remote_failure",
	"ambiguous_outcome",
	"destination_name_conflict",
	"readback_mismatch",
	"dependency_not_acknowledged",
	"outbox_state_mismatch",
	"unsafe_outbound_value",
	"local_state_failure",
}

var safeDeliveryCategorySet = func() map[string]bool {
	values := make(map[string]bool, len(safeDeliveryCategories))
	for _, value := range safeDeliveryCategories {
		values[value] = true
	}
	return values
}()

func SafeDeliveryCategoryValues() []string {
	return append([]string{}, safeDeliveryCategories...)
}

func ValidSafeDeliveryCategory(category string) bool {
	return safeDeliveryCategorySet[category]
}

func normalizeTransportCategory(category string, mayHaveCommitted bool) string {
	if mayHaveCommitted {
		return "ambiguous_outcome"
	}
	if ValidSafeDeliveryCategory(category) {
		return category
	}
	return "remote_failure"
}

type WorkspaceCapability struct {
	ID             string `json:"id"`
	Slug           string `json:"slug"`
	GovernanceMode string `json:"governance_mode"`
	KeyScope       string `json:"key_scope"`
	KeyID          string `json:"key_id"`
}

type CollectionFieldCapability struct {
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

type CollectionCapability struct {
	Found  bool                        `json:"found"`
	Slug   string                      `json:"slug,omitempty"`
	Fields []CollectionFieldCapability `json:"fields,omitempty"`
}

type EntryReadback struct {
	Found          bool           `json:"found"`
	DocID          string         `json:"doc_id,omitempty"`
	EntryID        string         `json:"entry_id,omitempty"`
	CollectionSlug string         `json:"collection_slug,omitempty"`
	Name           string         `json:"name,omitempty"`
	Status         string         `json:"status,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
	SourceRef      string         `json:"source_ref,omitempty"`
	SourceExcerpt  string         `json:"source_excerpt,omitempty"`
	CreatedBy      string         `json:"created_by,omitempty"`
}

type EntrySearchResult struct {
	DocID          string `json:"doc_id"`
	EntryID        string `json:"entry_id"`
	CollectionSlug string `json:"collection_slug"`
	Name           string `json:"name"`
	Status         string `json:"status"`
}
type RelationReadback struct {
	RelationID string         `json:"relation_id"`
	FromDocID  string         `json:"from_doc_id"`
	ToDocID    string         `json:"to_doc_id"`
	Type       string         `json:"type"`
	Metadata   map[string]any `json:"metadata"`
}
type CreateEntryRequest struct {
	CollectionSlug string
	EntryID        string
	Name           string
	Data           map[string]any
	SourceRef      string
	SourceExcerpt  string
	CreatedBy      string
	ForceDraft     bool
}
type CreateEntryResult struct {
	EntryID string
	Status  string
}
type CreateRelationRequest struct {
	FromEntryID string
	ToEntryID   string
	Type        string
	Metadata    map[string]any
	IfMissing   bool
}
type CreateRelationResult struct {
	RelationID    string
	AlreadyExists bool
}

type ProductBrainTransport interface {
	ResolveWorkspace(context.Context) (WorkspaceCapability, error)
	GetCollectionFields(context.Context, string) (CollectionCapability, error)
	GetEntry(context.Context, string) (EntryReadback, error)
	SearchEntries(context.Context, string, string) ([]EntrySearchResult, error)
	CreateEntry(context.Context, CreateEntryRequest) (CreateEntryResult, error)
	ListEntryRelations(context.Context, string) ([]RelationReadback, error)
	CreateEntryRelation(context.Context, CreateRelationRequest) (CreateRelationResult, error)
}

// RuntimeSecretScanner keeps credential knowledge inside the transport while
// allowing preflight and delivery to prove that the exact outbound artifact
// does not contain the in-memory credential. Every writable transport must
// implement this companion port.
type RuntimeSecretScanner interface {
	RuntimeSecretFindings(any) []PrivacyFinding
}

type SecretProvider interface {
	Secret(context.Context) (string, error)
}

// RevocationAwareSecretProvider is the activation-time extension. The returned
// context is cancelled by the provider when the lease expires or is revoked;
// AKITransport links it to the individual HTTP request. Legacy providers remain
// supported through SecretProvider and are still resolved before every call.
type RevocationAwareSecretProvider interface {
	SecretProvider
	SecretWithContext(context.Context) (string, context.Context, error)
}

type EnvironmentSecretProvider struct{ Name string }

func (p EnvironmentSecretProvider) Secret(_ context.Context) (string, error) {
	return environmentSecret(p.Name)
}

type TransportError struct {
	Category         string
	MayHaveCommitted bool
}

func (e *TransportError) Error() string {
	return normalizeTransportCategory(e.Category, e.MayHaveCommitted)
}
