package personalmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/routing"
)

const (
	LibrarySchemaVersion          = "mindline-personal-evidence-library/v0.4"
	CaptureBatchSchemaVersion     = "mindline-personal-capture-batch/v0.1"
	AuthorityClass                = "personal_evidence_non_authoritative"
	MaximumLibraryBytes           = 64 << 20
	MaximumExtractedContentBytes  = 4 << 20
	MaximumRepositoryContentBytes = int64(1 << 30)
	MaximumRecords                = 100_000
	MaximumResources              = 250_000
	EnrichmentBatchSchemaVersion  = "mindline-personal-enrichment-batch/v0.1"
	LensBatchSchemaVersion        = "mindline-personal-lens-batch/v0.1"
	LensReviewSchemaVersion       = "mindline-personal-lens-review/v0.1"
)

type Library struct {
	SchemaVersion     string              `json:"schema_version"`
	Revision          uint64              `json:"revision"`
	Fingerprint       string              `json:"fingerprint"`
	Records           []CaptureRecord     `json:"records"`
	Revisions         []CaptureRevision   `json:"revisions"`
	Resources         []ResourceContext   `json:"resources"`
	ResourceRevisions []ResourceRevision  `json:"resource_revisions"`
	Imports           []ImportReceipt     `json:"imports"`
	EnrichmentImports []EnrichmentReceipt `json:"enrichment_imports"`
}

// CaptureBatch is the source-neutral handoff from any source adapter into the
// canonical personal evidence repository.
type CaptureBatch struct {
	SchemaVersion   string          `json:"schema_version"`
	SourceIdentity  string          `json:"source_identity"`
	LowerInclusive  string          `json:"lower_inclusive"`
	UpperInclusive  string          `json:"upper_inclusive"`
	Watermark       string          `json:"watermark"`
	DeclaredRecords int             `json:"declared_records"`
	Records         []CaptureRecord `json:"records"`
}

type CaptureRecord struct {
	RecordID                 string   `json:"record_id"`
	IdempotencyKey           string   `json:"idempotency_key"`
	SourceAdapter            string   `json:"source_adapter"`
	SourceScopeID            string   `json:"source_scope_id"`
	SourceContainerID        string   `json:"source_container_id"`
	ExternalID               string   `json:"external_id"`
	OccurredAt               string   `json:"occurred_at"`
	AuthorID                 string   `json:"author_id,omitempty"`
	AuthorName               string   `json:"author_name,omitempty"`
	SourceRef                string   `json:"source_ref"`
	RawText                  string   `json:"raw_text"`
	URLs                     []string `json:"urls"`
	ResourceIDs              []string `json:"resource_ids"`
	ThreadParentID           string   `json:"thread_parent_id,omitempty"`
	AttachmentCount          int      `json:"attachment_count"`
	PrivateFileCount         int      `json:"private_file_count"`
	EditDeleteState          string   `json:"edit_delete_state"`
	RevisionAt               string   `json:"revision_at,omitempty"`
	ContextState             string   `json:"context_state"`
	Missingness              []string `json:"missingness"`
	AuthorityClass           string   `json:"authority_class"`
	SourceContentFingerprint string   `json:"source_content_fingerprint"`
	ContentHash              string   `json:"content_hash"`
}

type CaptureRevision struct {
	RevisionID   string        `json:"revision_id"`
	SupersededAt string        `json:"superseded_at"`
	Record       CaptureRecord `json:"record"`
}

type ResourceRevision struct {
	RevisionID   string          `json:"revision_id"`
	SupersededAt string          `json:"superseded_at"`
	Resource     ResourceContext `json:"resource"`
}

// ResourceContext is durable, source-neutral context retrieved for a resource
// referenced by one or more captures. It is evidence, not an authoritative
// organizational claim, and can be re-enriched without changing save intent.
type ResourceContext struct {
	ResourceID     string              `json:"resource_id"`
	CanonicalURL   string              `json:"canonical_url"`
	State          string              `json:"state"`
	AccessClass    string              `json:"access_class"`
	RetrievedAt    string              `json:"retrieved_at,omitempty"`
	Metadata       ResourceMetadata    `json:"public_metadata"`
	Excerpts       []ResourceExcerpt   `json:"public_excerpts"`
	RelatedURLs    []RelatedResource   `json:"related_urls"`
	Missingness    []string            `json:"missingness"`
	Content        *ContentArtifactRef `json:"content,omitempty"`
	AuthorityClass string              `json:"authority_class"`
	ContentHash    string              `json:"content_hash"`
}

type ContentArtifactRef struct {
	ArtifactID   string `json:"artifact_id"`
	SHA256       string `json:"sha256"`
	ByteLength   int    `json:"byte_length"`
	RuneCount    int    `json:"rune_count"`
	MediaType    string `json:"media_type"`
	Completeness string `json:"completeness"`
	StorageClass string `json:"storage_class"`
}

type ExtractedContent struct {
	CanonicalURL string   `json:"canonical_url"`
	MediaType    string   `json:"media_type"`
	Completeness string   `json:"completeness"`
	Text         string   `json:"text"`
	Missingness  []string `json:"missingness"`
	AccessClass  string   `json:"access_class,omitempty"`
}

type ExtractedContentArtifact struct {
	Reference ContentArtifactRef `json:"reference"`
	Text      string             `json:"text"`
}

type ResourceMetadata struct {
	Title       string `json:"title,omitempty"`
	Author      string `json:"author,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

type ResourceExcerpt struct {
	ExcerptID string `json:"excerpt_id"`
	Text      string `json:"text"`
	Locator   string `json:"locator"`
}

type RelatedResource struct {
	URL                  string `json:"url"`
	Relation             string `json:"relation"`
	DiscoveryEvidenceRef string `json:"discovery_evidence_ref"`
	SemanticallyRelevant bool   `json:"semantically_relevant"`
}

type ImportReceipt struct {
	BatchFingerprint string `json:"batch_fingerprint"`
	SourceIdentity   string `json:"source_identity"`
	LowerInclusive   string `json:"lower_inclusive"`
	UpperInclusive   string `json:"upper_inclusive"`
	Watermark        string `json:"watermark"`
	DeclaredRecords  int    `json:"declared_records"`
	InsertedRecords  int    `json:"inserted_records"`
	UpdatedRecords   int    `json:"updated_records"`
	UnchangedRecords int    `json:"unchanged_records"`
	TotalRecords     int    `json:"total_records"`
	ImportedAt       string `json:"imported_at"`
}

type EnrichmentReceipt struct {
	InputFingerprint   string `json:"input_fingerprint"`
	DeclaredResources  int    `json:"declared_resources"`
	InsertedResources  int    `json:"inserted_resources"`
	UpdatedResources   int    `json:"updated_resources"`
	UnchangedResources int    `json:"unchanged_resources"`
	TotalResources     int    `json:"total_resources"`
	ImportedAt         string `json:"imported_at"`
}

type EnrichmentBatch struct {
	SchemaVersion string                         `json:"schema_version"`
	Resources     []acquisition.ImportedEvidence `json:"resources"`
	Contents      []ExtractedContent             `json:"contents,omitempty"`
}

type Status struct {
	SchemaVersion           string `json:"schema_version"`
	Revision                uint64 `json:"revision"`
	Fingerprint             string `json:"fingerprint"`
	RecordCount             int    `json:"record_count"`
	ResourceCount           int    `json:"resource_count"`
	HistoricalRevisionCount int    `json:"historical_revision_count"`
	HistoricalResourceCount int    `json:"historical_resource_revision_count"`
	ImportCount             int    `json:"import_count"`
	EnrichmentImportCount   int    `json:"enrichment_import_count"`
	AuthorityClass          string `json:"authority_class"`
}

type SearchRequest struct {
	Query        string `json:"query"`
	LexicalQuery string `json:"-"`
	Limit        int    `json:"limit"`
	RunID        string `json:"run_id,omitempty"`
	LensID       string `json:"lens_id,omitempty"`
	ScopeID      string `json:"scope_id,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	ScopePurpose string `json:"-"`
	LensQuery    string `json:"-"`
	// QueryAuthorizedLimit preserves the caller-visible bound while compact
	// retrieval asks the ranking backend for its larger internal candidate pool.
	QueryAuthorizedLimit int `json:"-"`
}

type Citation struct {
	RecordID        string              `json:"record_id"`
	LogicalRecordID string              `json:"logical_record_id"`
	VersionState    string              `json:"version_state"`
	SourceRef       string              `json:"source_ref"`
	OccurredAt      string              `json:"occurred_at"`
	Author          string              `json:"author,omitempty"`
	Snippet         string              `json:"snippet"`
	MatchedTerms    []string            `json:"matched_terms"`
	Score           float64             `json:"score"`
	ComponentScores map[string]float64  `json:"component_scores,omitempty"`
	ContentHash     string              `json:"content_hash"`
	ContextState    string              `json:"context_state"`
	Missingness     []string            `json:"missingness"`
	ResourceIDs     []string            `json:"resource_ids"`
	EvidenceRefs    []EvidenceReference `json:"evidence_refs"`
	AuthorityClass  string              `json:"authority_class"`
}

type EvidenceReference struct {
	ResourceID           string `json:"resource_id"`
	CanonicalURL         string `json:"canonical_url"`
	ResourceHash         string `json:"resource_hash"`
	ResourceVersionState string `json:"resource_version_state"`
	ResourceRevisionID   string `json:"resource_revision_id,omitempty"`
	ExcerptID            string `json:"excerpt_id,omitempty"`
	ArtifactID           string `json:"artifact_id,omitempty"`
	Locator              string `json:"locator"`
	MatchedSnippet       string `json:"matched_snippet"`
}

type ContextPacket struct {
	SchemaVersion       string             `json:"schema_version"`
	RunID               string             `json:"run_id,omitempty"`
	Query               string             `json:"query"`
	LensID              string             `json:"lens_id,omitempty"`
	RetrievalMethod     string             `json:"retrieval_method"`
	RetrievalState      string             `json:"retrieval_state,omitempty"`
	DegradedReason      string             `json:"degraded_reason,omitempty"`
	AuthorityClass      string             `json:"authority_class"`
	LibraryRevision     uint64             `json:"library_revision"`
	LibraryFingerprint  string             `json:"library_fingerprint"`
	RouteClass          string             `json:"route_class"`
	AgentRecallApproved bool               `json:"agent_recall_approved"`
	Citations           []Citation         `json:"citations"`
	Records             []CaptureRecord    `json:"records"`
	Resources           []ResourceContext  `json:"resources"`
	ResourceRevisions   []ResourceRevision `json:"resource_revisions"`
}

type CompactContextPacket struct {
	SchemaVersion               string                 `json:"schema_version"`
	RunID                       string                 `json:"run_id,omitempty"`
	Query                       string                 `json:"query"`
	LensID                      string                 `json:"lens_id,omitempty"`
	ScopeID                     string                 `json:"scope_id,omitempty"`
	AgentID                     string                 `json:"agent_id,omitempty"`
	RetrievalMethod             string                 `json:"retrieval_method"`
	RetrievalState              string                 `json:"retrieval_state,omitempty"`
	DegradedReason              string                 `json:"degraded_reason,omitempty"`
	AbstentionPolicyFingerprint string                 `json:"abstention_policy_fingerprint"`
	AuthorityClass              string                 `json:"authority_class"`
	LibraryRevision             uint64                 `json:"library_revision"`
	LibraryFingerprint          string                 `json:"library_fingerprint"`
	RouteClass                  string                 `json:"route_class"`
	AgentRecallApproved         bool                   `json:"agent_recall_approved"`
	AuditState                  string                 `json:"audit_state,omitempty"`
	AnswerState                 string                 `json:"answer_state"`
	AbstentionReason            string                 `json:"abstention_reason,omitempty"`
	AbstentionDiagnostics       *AbstentionDiagnostics `json:"abstention_diagnostics,omitempty"`
	Citations                   []CompactCitation      `json:"citations"`
	NextActions                 *AgentNextActions      `json:"next_actions,omitempty"`
}

// AgentNextActions makes the permitted continuation explicit without exposing
// owner/debug routes or any unauthorized candidate identity.
type AgentNextActions struct {
	State                  string   `json:"state"`
	AbstentionTerminal     bool     `json:"abstention_terminal"`
	NewQueryRule           string   `json:"new_query_rule"`
	HydrateSelectedCommand string   `json:"hydrate_selected_command,omitempty"`
	FeedbackTokenCommand   string   `json:"feedback_token_command,omitempty"`
	FeedbackCommand        string   `json:"feedback_command,omitempty"`
	ForbiddenFallbacks     []string `json:"forbidden_fallbacks"`
}

type AbstentionDiagnostics struct {
	Classification           string `json:"classification"`
	RankedCandidateCount     int    `json:"ranked_candidate_count"`
	AuthorizedCandidateCount int    `json:"authorized_candidate_count"`
}

type CompactAbstentionPolicy struct {
	SchemaVersion                  string  `json:"schema_version"`
	MinimumSemanticCosine          float64 `json:"minimum_semantic_cosine"`
	MinimumSemanticMargin          float64 `json:"minimum_semantic_margin"`
	MinimumSemanticOnlyCosine      float64 `json:"minimum_semantic_only_cosine"`
	MinimumSemanticOnlyMargin      float64 `json:"minimum_semantic_only_margin"`
	MinimumSemanticLexicalCoverage float64 `json:"minimum_semantic_lexical_coverage"`
	MinimumLexicalIDFCoverage      float64 `json:"minimum_lexical_idf_coverage"`
	MaximumLexicalDocumentRatio    float64 `json:"maximum_lexical_document_ratio"`
	MinimumLexicalWinnerMargin     float64 `json:"minimum_lexical_winner_margin"`
	MinimumLexicalMatchedTerms     int     `json:"minimum_lexical_matched_terms"`
	MinimumOrderedPhraseTerms      int     `json:"minimum_ordered_phrase_terms"`
	MinimumFullCoverageTerms       int     `json:"minimum_full_coverage_terms"`
	MinimumBroadQueryTerms         int     `json:"minimum_broad_query_terms"`
	MinimumBroadQueryMatches       int     `json:"minimum_broad_query_matches"`
	MinimumBroadQueryIDFCoverage   float64 `json:"minimum_broad_query_idf_coverage"`
	MaximumBroadQueryRank          int     `json:"maximum_broad_query_rank"`
	MinimumBroadSemanticCosine     float64 `json:"minimum_broad_semantic_cosine"`
	LexicalEvidenceRule            string  `json:"lexical_evidence_rule"`
	StopwordPolicy                 string  `json:"stopword_policy"`
	SemanticCalibrationIdentity    string  `json:"semantic_calibration_identity"`
	RankingIdentity                string  `json:"ranking_identity"`
	ChunkingIdentity               string  `json:"chunking_identity"`
	Fingerprint                    string  `json:"fingerprint"`
}

type CompactCitation struct {
	RecordID                string                     `json:"record_id"`
	LogicalRecordID         string                     `json:"logical_record_id"`
	VersionState            string                     `json:"version_state"`
	SourceRef               string                     `json:"source_ref"`
	OccurredAt              string                     `json:"occurred_at"`
	Author                  string                     `json:"author,omitempty"`
	Snippet                 string                     `json:"snippet"`
	MatchedTerms            []string                   `json:"matched_terms"`
	Score                   float64                    `json:"score"`
	ComponentScores         map[string]float64         `json:"component_scores,omitempty"`
	ContentHash             string                     `json:"content_hash"`
	ContextState            string                     `json:"context_state"`
	Missingness             []string                   `json:"missingness"`
	EvidenceRefs            []CompactEvidenceReference `json:"evidence_refs"`
	ResourceStates          []ResourceStateSummary     `json:"resource_states"`
	ResourceStatesTruncated bool                       `json:"resource_states_truncated,omitempty"`
	AuthorityClass          string                     `json:"authority_class"`
}

type CompactEvidenceReference struct {
	ResourceID           string `json:"resource_id"`
	ResourceHash         string `json:"resource_hash"`
	ResourceVersionState string `json:"resource_version_state"`
	ResourceRevisionID   string `json:"resource_revision_id,omitempty"`
	ExcerptID            string `json:"excerpt_id,omitempty"`
	ArtifactID           string `json:"artifact_id,omitempty"`
	Locator              string `json:"locator"`
	MatchedSnippet       string `json:"matched_snippet"`
}

type ResourceStateSummary struct {
	ResourceID     string   `json:"resource_id"`
	State          string   `json:"state"`
	AccessClass    string   `json:"access_class"`
	ContentHash    string   `json:"content_hash"`
	Missingness    []string `json:"missingness"`
	AuthorityClass string   `json:"authority_class"`
}

type HydratedCapture struct {
	SchemaVersion       string                     `json:"schema_version"`
	RecordID            string                     `json:"record_id"`
	VersionState        string                     `json:"version_state"`
	Record              CaptureRecord              `json:"record"`
	Resources           []ResourceContext          `json:"resources"`
	ResourceRevisions   []ResourceRevision         `json:"resource_revisions"`
	Contents            []ExtractedContentArtifact `json:"contents"`
	RunID               string                     `json:"run_id,omitempty"`
	ScopeID             string                     `json:"scope_id,omitempty"`
	LensID              string                     `json:"lens_id,omitempty"`
	AgentID             string                     `json:"agent_id,omitempty"`
	RouteClass          string                     `json:"route_class"`
	AgentRecallApproved bool                       `json:"agent_recall_approved"`
}

type Lens struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Query string `json:"query"`
}

type LensBatch struct {
	SchemaVersion string `json:"schema_version"`
	Lenses        []Lens `json:"lenses"`
}

type LensProjection struct {
	Lens               Lens       `json:"lens"`
	LibraryFingerprint string     `json:"library_fingerprint"`
	RetainedCount      int        `json:"retained_count"`
	Matches            []Citation `json:"matches"`
}

type LensReviewPacket struct {
	SchemaVersion      string           `json:"schema_version"`
	AuthorityClass     string           `json:"authority_class"`
	RetainedBefore     int              `json:"retained_before"`
	RetainedAfter      int              `json:"retained_after"`
	FingerprintBefore  string           `json:"fingerprint_before"`
	FingerprintAfter   string           `json:"fingerprint_after"`
	RetentionUnchanged bool             `json:"retention_unchanged"`
	LensCount          int              `json:"lens_count"`
	Projections        []LensProjection `json:"projections"`
}

func EmptyLibrary() Library {
	library := Library{
		SchemaVersion: LibrarySchemaVersion,
		Records:       []CaptureRecord{}, Revisions: []CaptureRevision{}, Resources: []ResourceContext{},
		ResourceRevisions: []ResourceRevision{},
		Imports:           []ImportReceipt{}, EnrichmentImports: []EnrichmentReceipt{},
	}
	library.Fingerprint = fingerprintLibrary(library)
	return library
}

func sealLibrary(library Library) Library {
	library.SchemaVersion = LibrarySchemaVersion
	sort.Slice(library.Records, func(i, j int) bool {
		return library.Records[i].IdempotencyKey < library.Records[j].IdempotencyKey
	})
	sort.Slice(library.Resources, func(i, j int) bool {
		return library.Resources[i].CanonicalURL < library.Resources[j].CanonicalURL
	})
	sort.Slice(library.Revisions, func(i, j int) bool {
		return library.Revisions[i].RevisionID < library.Revisions[j].RevisionID
	})
	sort.Slice(library.ResourceRevisions, func(i, j int) bool {
		return library.ResourceRevisions[i].RevisionID < library.ResourceRevisions[j].RevisionID
	})
	sort.Slice(library.Imports, func(i, j int) bool {
		if library.Imports[i].ImportedAt == library.Imports[j].ImportedAt {
			return library.Imports[i].BatchFingerprint < library.Imports[j].BatchFingerprint
		}
		return library.Imports[i].ImportedAt < library.Imports[j].ImportedAt
	})
	sort.Slice(library.EnrichmentImports, func(i, j int) bool {
		if library.EnrichmentImports[i].ImportedAt == library.EnrichmentImports[j].ImportedAt {
			return library.EnrichmentImports[i].InputFingerprint < library.EnrichmentImports[j].InputFingerprint
		}
		return library.EnrichmentImports[i].ImportedAt < library.EnrichmentImports[j].ImportedAt
	})
	library.Fingerprint = ""
	library.Fingerprint = fingerprintLibrary(library)
	return library
}

func validateLibrary(library Library) error {
	if library.SchemaVersion != LibrarySchemaVersion ||
		len(library.Records)+len(library.Revisions) > MaximumRecords ||
		len(library.Resources)+len(library.ResourceRevisions) > MaximumResources {
		return errors.New("unsupported personal evidence library")
	}
	if library.Fingerprint == "" || library.Fingerprint != fingerprintLibrary(library) {
		return errors.New("personal evidence library fingerprint mismatch")
	}
	ids := make(map[string]bool, len(library.Records))
	keys := make(map[string]bool, len(library.Records))
	resourceIDs := make(map[string]bool, len(library.Resources))
	resourceURLs := make(map[string]bool, len(library.Resources))
	for _, resource := range library.Resources {
		if err := validateResource(resource); err != nil {
			return err
		}
		if resourceIDs[resource.ResourceID] || resourceURLs[resource.CanonicalURL] {
			return errors.New("duplicate personal evidence resource")
		}
		resourceIDs[resource.ResourceID] = true
		resourceURLs[resource.CanonicalURL] = true
	}
	for _, record := range library.Records {
		if err := validateRecord(record); err != nil {
			return err
		}
		if ids[record.RecordID] || keys[record.IdempotencyKey] {
			return errors.New("duplicate personal evidence identity")
		}
		ids[record.RecordID] = true
		keys[record.IdempotencyKey] = true
		for _, resourceID := range record.ResourceIDs {
			if !resourceIDs[resourceID] {
				return errors.New("personal evidence resource reference is unresolved")
			}
		}
	}
	revisions := map[string]bool{}
	for _, revision := range library.Revisions {
		if strings.TrimSpace(revision.RevisionID) == "" || revisions[revision.RevisionID] {
			return errors.New("invalid or duplicate personal evidence revision")
		}
		if _, err := time.Parse(time.RFC3339Nano, revision.SupersededAt); err != nil {
			return errors.New("invalid personal evidence revision timestamp")
		}
		if err := validateRecord(revision.Record); err != nil {
			return err
		}
		if revision.RevisionID != stableRevisionID(revision.Record) {
			return errors.New("personal evidence revision identity mismatch")
		}
		for _, resourceID := range revision.Record.ResourceIDs {
			if !resourceIDs[resourceID] {
				return errors.New("historical personal evidence resource reference is unresolved")
			}
		}
		revisions[revision.RevisionID] = true
	}
	resourceRevisions := map[string]bool{}
	for _, revision := range library.ResourceRevisions {
		if strings.TrimSpace(revision.RevisionID) == "" || resourceRevisions[revision.RevisionID] {
			return errors.New("invalid or duplicate personal evidence resource revision")
		}
		if _, err := time.Parse(time.RFC3339Nano, revision.SupersededAt); err != nil {
			return errors.New("invalid personal evidence resource revision timestamp")
		}
		if err := validateResource(revision.Resource); err != nil {
			return err
		}
		if revision.RevisionID != stableResourceRevisionID(revision.Resource) {
			return errors.New("personal evidence resource revision identity mismatch")
		}
		if !resourceIDs[revision.Resource.ResourceID] {
			return errors.New("historical resource has no current logical resource")
		}
		resourceRevisions[revision.RevisionID] = true
	}
	return nil
}

func validateRecord(record CaptureRecord) error {
	required := []string{
		record.RecordID, record.IdempotencyKey, record.SourceAdapter,
		record.SourceScopeID, record.SourceContainerID, record.ExternalID,
		record.OccurredAt, record.SourceRef, record.RawText,
		record.ContextState, record.AuthorityClass,
		record.SourceContentFingerprint, record.ContentHash,
	}
	for _, value := range required {
		if strings.TrimSpace(value) == "" {
			return errors.New("incomplete personal evidence record")
		}
	}
	if record.AuthorityClass != AuthorityClass || !validSHA256(record.SourceContentFingerprint) || !validSHA256(record.ContentHash) {
		return errors.New("invalid personal evidence authority or fingerprint")
	}
	recordText := []string{
		record.IdempotencyKey, record.SourceAdapter, record.SourceScopeID,
		record.SourceContainerID, record.ExternalID, record.AuthorID,
		record.AuthorName, record.SourceRef, record.RawText, record.ThreadParentID,
		record.EditDeleteState, record.RevisionAt,
		strings.Join(record.Missingness, " "),
	}
	if importedEvidenceContainsSecret(recordText...) ||
		containsUnsafeURL(strings.Join(recordText, "\n")) {
		return errors.New("personal evidence record contains unsafe material")
	}
	if record.ContentHash != fingerprintRecord(record) || len(record.URLs) != len(record.ResourceIDs) {
		return errors.New("personal evidence record content mismatch")
	}
	for index, canonicalURL := range record.URLs {
		safe, state, err := routing.PrepareURLForStorage(canonicalURL)
		if err != nil || state == routing.URLStorageSensitiveRedacted || safe != canonicalURL {
			return errors.New("personal evidence record URL is unsafe")
		}
		if record.ResourceIDs[index] != stableResourceID(canonicalURL) {
			return errors.New("personal evidence record resource identity mismatch")
		}
	}
	switch record.ContextState {
	case "source_complete", "secret_redacted", "empty_source", "deleted_tombstone":
	default:
		return errors.New("invalid personal evidence context state")
	}
	switch record.EditDeleteState {
	case "original", "edited", "deleted", "tombstone":
	default:
		return errors.New("invalid personal evidence edit/delete state")
	}
	if record.RevisionAt != "" {
		revisionAt, err := time.Parse(time.RFC3339Nano, record.RevisionAt)
		occurredAt, occurredErr := time.Parse(time.RFC3339Nano, record.OccurredAt)
		if err != nil || occurredErr != nil || !revisionAt.After(occurredAt) {
			return errors.New("invalid personal evidence revision chronology")
		}
	}
	if record.AttachmentCount < 0 || record.PrivateFileCount < 0 || record.PrivateFileCount > record.AttachmentCount {
		return errors.New("invalid personal evidence attachment accounting")
	}
	return nil
}

func validateResource(resource ResourceContext) error {
	if strings.TrimSpace(resource.ResourceID) == "" ||
		strings.TrimSpace(resource.CanonicalURL) == "" ||
		resource.AuthorityClass != AuthorityClass ||
		!validSHA256(resource.ContentHash) {
		return errors.New("invalid personal evidence resource")
	}
	if resource.ResourceID != stableResourceID(resource.CanonicalURL) || resource.ContentHash != fingerprintResource(resource) {
		return errors.New("personal evidence resource content mismatch")
	}
	safeURL, storageState, err := routing.PrepareURLForStorage(resource.CanonicalURL)
	if err != nil || storageState == routing.URLStorageSensitiveRedacted || safeURL != resource.CanonicalURL {
		return errors.New("invalid personal evidence resource URL")
	}
	if resource.RetrievedAt != "" {
		if _, err := time.Parse(time.RFC3339, resource.RetrievedAt); err != nil {
			return errors.New("invalid personal evidence resource timestamp")
		}
	}
	resourceText := []string{
		resource.Metadata.Title, resource.Metadata.Author,
		resource.Metadata.PublishedAt, strings.Join(resource.Missingness, " "),
	}
	for _, excerpt := range resource.Excerpts {
		resourceText = append(resourceText, excerpt.ExcerptID, excerpt.Text, excerpt.Locator)
	}
	for _, related := range resource.RelatedURLs {
		resourceText = append(resourceText, related.Relation, related.DiscoveryEvidenceRef)
	}
	if importedEvidenceContainsSecret(resourceText...) ||
		containsUnsafeURL(strings.Join(resourceText, "\n")) {
		return errors.New("personal evidence resource contains unsafe material")
	}
	switch resource.State {
	case "complete", "partial", "inaccessible", "failed", "not_attempted":
	default:
		return errors.New("invalid personal evidence resource state")
	}
	switch resource.AccessClass {
	case "public", "private", "authenticated", "unsupported":
	default:
		return errors.New("invalid personal evidence resource access")
	}
	if resource.State == "complete" && resource.Content == nil {
		return errors.New("complete personal evidence resource requires full extracted content")
	}
	if resource.State == "inaccessible" && (len(resource.Excerpts) != 0 || len(resource.Missingness) == 0) {
		return errors.New("inaccessible personal evidence resource must be explicit")
	}
	excerpts := map[string]bool{}
	totalRunes := 0
	for _, excerpt := range resource.Excerpts {
		if strings.TrimSpace(excerpt.ExcerptID) == "" ||
			excerpts[excerpt.ExcerptID] ||
			strings.TrimSpace(excerpt.Text) == "" ||
			strings.TrimSpace(excerpt.Locator) == "" ||
			len([]rune(excerpt.Text)) > 1000 {
			return errors.New("invalid personal evidence resource excerpt")
		}
		excerpts[excerpt.ExcerptID] = true
		totalRunes += len([]rune(excerpt.Text))
	}
	if totalRunes > 4000 {
		return errors.New("personal evidence resource excerpt budget exceeded")
	}
	for _, related := range resource.RelatedURLs {
		if strings.TrimSpace(related.URL) == "" ||
			related.Relation != "source_links_to" ||
			!excerpts[related.DiscoveryEvidenceRef] {
			return errors.New("invalid personal evidence related resource")
		}
		safeRelated, state, err := routing.PrepareURLForStorage(related.URL)
		if err != nil || state == routing.URLStorageSensitiveRedacted || safeRelated != related.URL {
			return errors.New("invalid personal evidence related resource URL")
		}
	}
	if resource.Content != nil {
		if err := validateContentReference(*resource.Content); err != nil {
			return err
		}
	}
	return nil
}

func validateContentReference(reference ContentArtifactRef) error {
	if strings.TrimSpace(reference.ArtifactID) == "" || reference.ArtifactID != "content-"+reference.SHA256 ||
		!validSHA256(reference.SHA256) || reference.ByteLength < 1 ||
		reference.ByteLength > MaximumExtractedContentBytes || reference.RuneCount < 1 ||
		strings.TrimSpace(reference.MediaType) == "" || len([]rune(reference.MediaType)) > 256 ||
		importedEvidenceContainsSecret(reference.MediaType) || containsUnsafeURL(reference.MediaType) ||
		reference.StorageClass != "owner_only_content_addressed_file" {
		return errors.New("invalid personal evidence content reference")
	}
	switch reference.Completeness {
	case "full", "partial":
	default:
		return errors.New("invalid personal evidence content completeness")
	}
	return nil
}

func fingerprintLibrary(library Library) string {
	copy := library
	copy.Fingerprint = ""
	data, _ := json.Marshal(copy)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fingerprintValue(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
