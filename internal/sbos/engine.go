package sbos

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type State string

const (
	StateIngested        State = "ingested"
	StateValidated       State = "validated"
	StateSkipped         State = "skipped"
	StateNeedsEnrichment State = "needs_enrichment"
	StateNeedsClarify    State = "needs_clarification"
	StateBackgroundReady State = "background_ready"
	StateAttentionReady  State = "attention_ready"
	StatePublishReady    State = "publish_ready"
	StateDryRunPublished State = "dry_run_published"
	StateError           State = "error"
)

type ArtifactKind string

const (
	ArtifactAttentionPreview ArtifactKind = "attention_preview"
	ArtifactDryRunPublish    ArtifactKind = "dry_run_publish"
)

var requiredAuthorityIDs = []string{
	"DEC-4",
	"DEC-3",
	"DEC-2",
	"DEC-1",
	"FEAT-1",
	"STD-1",
	"STD-7",
	"STD-10",
	"STD-11",
	"STD-12",
	"FEAT-4",
	"WP-1",
}

type Engine struct {
	store CandidateStore
}

type CandidateStore interface {
	GetByIdempotencyKey(key string) (StoredRecord, bool)
	Save(idempotencyKey string, record StoredRecord)
	Count() int
}

type MemoryCandidateStore struct {
	recordsByIdempotencyKey map[string]StoredRecord
}

type ProcessResult struct {
	State        State
	RecordID     string
	StoredRecord StoredRecord
	Artifacts    []Artifact
	AuthorityIDs []string
}

type StoredRecord struct {
	ID         string
	RawContent string
}

type Artifact struct {
	Kind ArtifactKind
	Body string
}

type Candidate struct {
	SchemaVersion     string         `json:"schema_version"`
	CandidateID       string         `json:"candidate_id"`
	AdapterID         string         `json:"adapter_id"`
	ExternalID        string         `json:"external_id"`
	CapturedAt        string         `json:"captured_at"`
	Provenance        Provenance     `json:"provenance"`
	Content           Content        `json:"content"`
	EnrichmentStatus  string         `json:"enrichment_status"`
	Classification    Classification `json:"classification"`
	Safety            Safety         `json:"safety"`
	DesiredVisibility string         `json:"desired_visibility"`
	IdempotencyKey    string         `json:"idempotency_key"`
}

type Provenance struct {
	Permalink       VisibilityValue `json:"permalink"`
	NativeTimestamp VisibilityValue `json:"native_timestamp"`
	Author          VisibilityValue `json:"author"`
	RawLocator      VisibilityValue `json:"raw_locator"`
}

type VisibilityValue struct {
	Value      string `json:"value"`
	Visibility string `json:"visibility"`
}

type Content struct {
	Text        string   `json:"text"`
	URLs        []string `json:"urls"`
	Attachments []string `json:"attachments"`
	SourceTitle string   `json:"source_title"`
}

type Classification struct {
	Type                string   `json:"type"`
	Domain              string   `json:"domain"`
	Topics              []string `json:"topics"`
	Confidence          string   `json:"confidence"`
	NeedsClarification  bool     `json:"needs_clarification"`
	ClarificationReason string   `json:"clarification_reason"`
}

type Safety struct {
	RedactionRequired bool `json:"redaction_required"`
	SecretLike        bool `json:"secret_like"`
	EmptyContent      bool `json:"empty_content"`
	PrivateProvenance bool `json:"private_provenance"`
}

func NewEngine() *Engine {
	return NewEngineWithStore(NewMemoryCandidateStore())
}

func NewEngineWithStore(store CandidateStore) *Engine {
	return &Engine{store: store}
}

func NewMemoryCandidateStore() *MemoryCandidateStore {
	return &MemoryCandidateStore{recordsByIdempotencyKey: map[string]StoredRecord{}}
}

func (s *MemoryCandidateStore) GetByIdempotencyKey(key string) (StoredRecord, bool) {
	record, ok := s.recordsByIdempotencyKey[key]
	return record, ok
}

func (s *MemoryCandidateStore) Save(idempotencyKey string, record StoredRecord) {
	s.recordsByIdempotencyKey[idempotencyKey] = record
}

func (s *MemoryCandidateStore) Count() int {
	return len(s.recordsByIdempotencyKey)
}

func (e *Engine) RecordCount() int {
	return e.store.Count()
}

func (e *Engine) ProcessCandidate(input []byte) (ProcessResult, error) {
	var candidate Candidate
	if err := json.Unmarshal(input, &candidate); err != nil {
		return ProcessResult{State: StateError, AuthorityIDs: authorityIDs()}, err
	}

	if err := candidate.validate(); err != nil {
		return ProcessResult{State: StateError, AuthorityIDs: authorityIDs()}, err
	}

	if existing, ok := e.store.GetByIdempotencyKey(candidate.IdempotencyKey); ok {
		return ProcessResult{
			State:        StateValidated,
			RecordID:     existing.ID,
			StoredRecord: existing,
			AuthorityIDs: authorityIDs(),
		}, nil
	}

	record := StoredRecord{
		ID:         candidate.CandidateID,
		RawContent: candidate.recordContent(),
	}
	e.store.Save(candidate.IdempotencyKey, record)

	result := ProcessResult{
		State:        StateValidated,
		RecordID:     record.ID,
		StoredRecord: record,
		AuthorityIDs: authorityIDs(),
	}

	if candidate.Safety.EmptyContent || candidate.Safety.SecretLike {
		result.State = StateSkipped
		return result, nil
	}

	if candidate.Safety.RedactionRequired || candidate.Safety.PrivateProvenance || candidate.hasPrivatePublishProvenance() {
		if candidate.DesiredVisibility == "attention" || candidate.DesiredVisibility == "clarify" {
			result.State = StateAttentionReady
			result.Artifacts = []Artifact{{Kind: ArtifactAttentionPreview, Body: renderAttention(candidate, true, "Redaction required")}}
			return result, nil
		}
		result.State = StateBackgroundReady
		return result, nil
	}

	if candidate.EnrichmentStatus == "incomplete" || candidate.EnrichmentStatus == "failed" {
		if candidate.DesiredVisibility == "clarify" {
			result.State = StateAttentionReady
			result.Artifacts = []Artifact{{Kind: ArtifactAttentionPreview, Body: renderAttention(candidate, false, "Enrichment blocker")}}
			return result, nil
		}
		result.State = StateNeedsEnrichment
		return result, nil
	}

	if candidate.DesiredVisibility == "clarify" || candidate.Classification.NeedsClarification {
		result.State = StateAttentionReady
		result.Artifacts = []Artifact{{Kind: ArtifactAttentionPreview, Body: renderAttention(candidate, false, "Clarification needed")}}
		return result, nil
	}

	if candidate.DesiredVisibility == "background" {
		result.State = StateBackgroundReady
		return result, nil
	}

	if candidate.DesiredVisibility == "attention" {
		result.State = StateAttentionReady
		result.Artifacts = []Artifact{{Kind: ArtifactAttentionPreview, Body: renderAttention(candidate, false, "Attention needed")}}
		return result, nil
	}

	result.State = StateDryRunPublished
	result.Artifacts = []Artifact{{Kind: ArtifactDryRunPublish, Body: renderPublish(candidate)}}
	return result, nil
}

func (c Candidate) validate() error {
	if c.SchemaVersion != "v0.1" {
		return fmt.Errorf("unsupported schema_version %q", c.SchemaVersion)
	}
	required := map[string]string{
		"candidate_id":       c.CandidateID,
		"adapter_id":         c.AdapterID,
		"external_id":        c.ExternalID,
		"captured_at":        c.CapturedAt,
		"idempotency_key":    c.IdempotencyKey,
		"enrichment_status":  c.EnrichmentStatus,
		"desired_visibility": c.DesiredVisibility,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("missing required field %s", field)
		}
	}
	if !oneOf(c.EnrichmentStatus, "not_required", "complete", "incomplete", "failed") {
		return fmt.Errorf("invalid enrichment_status %q", c.EnrichmentStatus)
	}
	if !oneOf(c.DesiredVisibility, "background", "attention", "publish", "clarify") {
		return fmt.Errorf("invalid desired_visibility %q", c.DesiredVisibility)
	}
	if err := c.Provenance.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Content.Text) == "" && !c.Safety.EmptyContent {
		return errors.New("missing required field content.text")
	}
	if strings.TrimSpace(c.Classification.Type) == "" {
		return errors.New("missing required field classification.type")
	}
	if strings.TrimSpace(c.Classification.Domain) == "" {
		return errors.New("missing required field classification.domain")
	}
	if strings.TrimSpace(c.Classification.Confidence) == "" {
		return errors.New("missing required field classification.confidence")
	}
	if len(c.Classification.Topics) == 0 {
		return errors.New("missing required field classification.topics")
	}
	return nil
}

func (p Provenance) validate() error {
	fields := map[string]VisibilityValue{
		"provenance.permalink":        p.Permalink,
		"provenance.native_timestamp": p.NativeTimestamp,
		"provenance.author":           p.Author,
		"provenance.raw_locator":      p.RawLocator,
	}
	for field, value := range fields {
		if strings.TrimSpace(value.Value) == "" {
			return fmt.Errorf("missing required field %s.value", field)
		}
		if !oneOf(value.Visibility, "public", "private") {
			return fmt.Errorf("invalid %s.visibility %q", field, value.Visibility)
		}
	}
	return nil
}

func (c Candidate) recordContent() string {
	if c.Safety.EmptyContent || c.Safety.SecretLike || c.Safety.RedactionRequired || c.Safety.PrivateProvenance || c.hasPrivatePublishProvenance() {
		return "[redacted]"
	}
	return c.Content.Text
}

func (c Candidate) hasPrivatePublishProvenance() bool {
	return c.Provenance.Permalink.Visibility == "private" ||
		c.Provenance.NativeTimestamp.Visibility == "private" ||
		c.Provenance.Author.Visibility == "private" ||
		c.Provenance.RawLocator.Visibility == "private"
}

func renderAttention(candidate Candidate, redacted bool, reason string) string {
	source := safeValue(candidate.Provenance.Permalink, redacted)
	author := safeValue(candidate.Provenance.Author, redacted)
	body := safeText(candidate.Content.Text, redacted)

	return fmt.Sprintf(`# %s

%s.

- Candidate: %s
- Source: %s
- Author: %s
- Detail: %s
`, titleOrFallback(candidate), reason, candidate.CandidateID, source, author, body)
}

func renderPublish(candidate Candidate) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: " + candidate.Classification.Type + "\n")
	b.WriteString("status: dry_run\n")
	b.WriteString("domain: " + candidate.Classification.Domain + "\n")
	b.WriteString("topics:\n")
	for _, topic := range candidate.Classification.Topics {
		b.WriteString("  - " + topic + "\n")
	}
	b.WriteString("source_adapter: " + candidate.AdapterID + "\n")
	if candidate.Provenance.Permalink.Visibility == "public" && candidate.Provenance.Permalink.Value != "" {
		b.WriteString("source_url: " + candidate.Provenance.Permalink.Value + "\n")
	}
	b.WriteString("confidence: " + candidate.Classification.Confidence + "\n")
	b.WriteString("processing_status: dry_run_published\n")
	b.WriteString("visibility: publish\n")
	b.WriteString("schema_version: " + candidate.SchemaVersion + "\n")
	b.WriteString("candidate_id: " + candidate.CandidateID + "\n")
	b.WriteString("---\n\n")

	b.WriteString("# " + titleOrFallback(candidate) + "\n\n")
	b.WriteString("## Snapshot\n")
	b.WriteString(candidate.Content.Text + "\n\n")
	b.WriteString("## Source Content\n")
	b.WriteString("- Source: " + candidate.Provenance.Permalink.Value + "\n")
	b.WriteString("- Captured from: " + candidate.AdapterID + "\n")
	b.WriteString("- Author: " + candidate.Provenance.Author.Value + "\n\n")
	b.WriteString("## Key Details\n")
	b.WriteString("- " + candidate.Content.Text + "\n\n")
	b.WriteString("## Relevance\n")
	b.WriteString("Classified under " + candidate.Classification.Domain + " with " + candidate.Classification.Confidence + " confidence.\n\n")
	b.WriteString("## Signals\n")
	for _, topic := range candidate.Classification.Topics {
		b.WriteString("- " + topic + "\n")
	}
	b.WriteString("\n## Related Sources\n")
	for _, url := range candidate.Content.URLs {
		b.WriteString("- " + url + "\n")
	}
	b.WriteString("\n## Next Action\n")
	b.WriteString("No immediate action. Keep as processed source reference.\n")
	return b.String()
}

func titleOrFallback(candidate Candidate) string {
	if candidate.Content.SourceTitle != "" {
		return candidate.Content.SourceTitle
	}
	return candidate.CandidateID
}

func safeValue(value VisibilityValue, redacted bool) string {
	if redacted || value.Visibility == "private" {
		return "[redacted]"
	}
	if value.Value == "" {
		return "unknown"
	}
	return value.Value
}

func safeText(value string, redacted bool) string {
	if redacted {
		return "[redacted]"
	}
	return value
}

func authorityIDs() []string {
	ids := make([]string, len(requiredAuthorityIDs))
	copy(ids, requiredAuthorityIDs)
	return ids
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

type StateMachine struct{}

func NewStateMachine() StateMachine {
	return StateMachine{}
}

func (StateMachine) Transition(from, to State) error {
	allowed := map[State][]State{
		StateIngested:        {StateValidated},
		StateValidated:       {StateSkipped, StateNeedsEnrichment, StateNeedsClarify, StateBackgroundReady, StateAttentionReady, StatePublishReady, StateError},
		StateNeedsClarify:    {StateAttentionReady},
		StatePublishReady:    {StateDryRunPublished},
		StateNeedsEnrichment: {StateError},
	}
	for _, allowedTo := range allowed[from] {
		if allowedTo == to {
			return nil
		}
	}
	return fmt.Errorf("invalid transition from %q to %q", from, to)
}
