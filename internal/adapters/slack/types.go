package slack

import "github.com/synergyai-os/Mindline/internal/sbos"

type Payload struct {
	Source   Source    `json:"source"`
	Messages []Message `json:"messages"`
}

type Source struct {
	Workspace   string `json:"workspace"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	AdapterID   string `json:"adapter_id"`
}

type Message struct {
	TS              string          `json:"ts"`
	User            string          `json:"user"`
	AuthorName      string          `json:"author_name"`
	Text            string          `json:"text"`
	Permalink       string          `json:"permalink"`
	Files           []File          `json:"files"`
	Attachments     []Attachment    `json:"attachments"`
	CapturedAt      string          `json:"captured_at"`
	CaptureMetadata CaptureMetadata `json:"capture_metadata"`
}

type File struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URLPrivate string `json:"url_private"`
	URLPublic  string `json:"url_public"`
	Title      string `json:"title"`
}

type Attachment struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	TitleLink string `json:"title_link"`
	FromURL   string `json:"from_url"`
	Text      string `json:"text"`
}

type CaptureMetadata struct {
	SaveIntentStatus          string   `json:"save_intent_status"`
	ClassificationStatus      string   `json:"classification_status"`
	DesiredVisibilityHint     string   `json:"desired_visibility_hint"`
	ProvenanceVisibilityHint  string   `json:"provenance_visibility_hint"`
	PublicProvenanceAssertion string   `json:"public_provenance_assertion"`
	DomainHint                string   `json:"domain_hint"`
	TopicHints                []string `json:"topic_hints"`
	ClarificationReason       string   `json:"clarification_reason"`
}

type Result struct {
	AdapterID    string           `json:"adapter_id"`
	Candidates   []sbos.Candidate `json:"candidates"`
	Checkpoint   Checkpoint       `json:"checkpoint"`
	AuthorityIDs []string         `json:"authority_ids"`
}

type Checkpoint struct {
	AdapterID             string `json:"adapter_id"`
	Source                string `json:"source"`
	BatchOrder            string `json:"batch_order"`
	InputCount            int    `json:"input_count"`
	CandidateCount        int    `json:"candidate_count"`
	SkippedByAdapterCount int    `json:"skipped_by_adapter_count"`
	FirstTS               string `json:"first_ts"`
	LastTS                string `json:"last_ts"`
	NextOldestExclusiveTS string `json:"next_oldest_exclusive_ts"`
}

const CorpusIntakeSummarySchemaVersion = "slack-corpus-intake-summary/v0.1"

type CorpusIntakeItemState string

const (
	CorpusIntakeItemProcessed CorpusIntakeItemState = "processed"
	CorpusIntakeItemSkipped   CorpusIntakeItemState = "skipped"
	CorpusIntakeItemBlocked   CorpusIntakeItemState = "blocked"
)

type CorpusIntakeReason string

const (
	CorpusIntakeReasonNone          CorpusIntakeReason = "none"
	CorpusIntakeReasonEmptyMessage  CorpusIntakeReason = "empty_message"
	CorpusIntakeReasonSecretLike    CorpusIntakeReason = "secret_like"
	CorpusIntakeReasonArtifactWrite CorpusIntakeReason = "artifact_write"
)

type CorpusIntakeSummary struct {
	SchemaVersion      string                        `json:"schema_version"`
	AdapterID          string                        `json:"adapter_id"`
	CorpusID           string                        `json:"corpus_id"`
	Source             string                        `json:"source"`
	ChannelID          string                        `json:"channel_id"`
	ChannelName        string                        `json:"channel_name"`
	BatchOrder         string                        `json:"batch_order"`
	InputCount         int                           `json:"input_count"`
	ProcessedCount     int                           `json:"processed_count"`
	SkippedCount       int                           `json:"skipped_count"`
	BlockedCount       int                           `json:"blocked_count"`
	PrivateProvenance  int                           `json:"private_provenance_count"`
	SecretLikeCount    int                           `json:"secret_like_count"`
	ManifestPath       string                        `json:"manifest_path"`
	ReportPath         string                        `json:"report_path"`
	DestinationWrites  int                           `json:"destination_writes"`
	ProductBrainWrites int                           `json:"product_brain_writes"`
	TolariaWrites      int                           `json:"tolaria_writes"`
	AuthorityIDs       []string                      `json:"authority_ids"`
	Items              []CorpusIntakeItem            `json:"items"`
	ReasonCounts       map[CorpusIntakeReason]int    `json:"reason_counts"`
	StateCounts        map[CorpusIntakeItemState]int `json:"state_counts"`
}

type CorpusIntakeItem struct {
	SourceID     string                `json:"source_id"`
	ExternalID   string                `json:"external_id"`
	SlackTS      string                `json:"slack_ts"`
	State        CorpusIntakeItemState `json:"state"`
	ReasonCode   CorpusIntakeReason    `json:"reason_code"`
	SourcePath   string                `json:"source_path,omitempty"`
	Private      bool                  `json:"private"`
	SecretLike   bool                  `json:"secret_like"`
	EmptyContent bool                  `json:"empty_content"`
}
