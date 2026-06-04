package gmail

import "github.com/synergyai-os/Mindline/internal/sbos"

type Payload struct {
	Source    Source    `json:"source"`
	Messages  []Message `json:"messages"`
	Responses []Message `json:"responses"`
}

type Source struct {
	Account   string `json:"account"`
	Mailbox   string `json:"mailbox"`
	AdapterID string `json:"adapter_id"`
}

type Message struct {
	ID            string       `json:"id"`
	ThreadID      string       `json:"thread_id"`
	From          string       `json:"from"`
	FromAlt       string       `json:"from_"`
	To            []string     `json:"to"`
	CC            []string     `json:"cc"`
	BCC           []string     `json:"bcc"`
	Subject       string       `json:"subject"`
	Snippet       string       `json:"snippet"`
	Body          string       `json:"body"`
	Labels        []string     `json:"labels"`
	EmailTS       string       `json:"email_ts"`
	DisplayURL    string       `json:"display_url"`
	DisplayTitle  string       `json:"display_title"`
	HasAttachment bool         `json:"has_attachment"`
	Attachments   []Attachment `json:"attachments"`
}

type Attachment struct {
	Filename     string `json:"filename"`
	MimeType     string `json:"mime_type"`
	AttachmentID string `json:"attachment_id"`
	Size         int64  `json:"size"`
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
	FirstEmailTS          string `json:"first_email_ts"`
	LastEmailTS           string `json:"last_email_ts"`
	NextOldestExclusiveTS string `json:"next_oldest_exclusive_ts"`
}

const CorpusIntakeSummarySchemaVersion = "gmail-corpus-intake-summary/v0.1"

type CorpusIntakeItemState string

const (
	CorpusIntakeItemProcessed CorpusIntakeItemState = "processed"
	CorpusIntakeItemSkipped   CorpusIntakeItemState = "skipped"
	CorpusIntakeItemBlocked   CorpusIntakeItemState = "blocked"
)

type CorpusIntakeReason string

const (
	CorpusIntakeReasonNone             CorpusIntakeReason = "none"
	CorpusIntakeReasonEmptyMessage     CorpusIntakeReason = "empty_message"
	CorpusIntakeReasonSecretLike       CorpusIntakeReason = "secret_like"
	CorpusIntakeReasonDuplicateMessage CorpusIntakeReason = "duplicate_message"
	CorpusIntakeReasonArtifactWrite    CorpusIntakeReason = "artifact_write"
)

type CorpusIntakeSummary struct {
	SchemaVersion      string                        `json:"schema_version"`
	AdapterID          string                        `json:"adapter_id"`
	CorpusID           string                        `json:"corpus_id"`
	Source             string                        `json:"source"`
	Mailbox            string                        `json:"mailbox"`
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
	EmailTS      string                `json:"email_ts"`
	State        CorpusIntakeItemState `json:"state"`
	ReasonCode   CorpusIntakeReason    `json:"reason_code"`
	SourcePath   string                `json:"source_path,omitempty"`
	Private      bool                  `json:"private"`
	SecretLike   bool                  `json:"secret_like"`
	EmptyContent bool                  `json:"empty_content"`
}
