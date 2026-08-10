package slack

import (
	"errors"
	"strings"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/assurance"
)

const (
	NativeBatchSchema           = "mindline_native_slack_batch/v1"
	MaximumNativeBatchMessages  = 20_000
	MaximumNativeURLOccurrences = 50_000
)

// NativeBatch is the versioned handoff contract between a credential-owning
// Slack connector and Mindline's Slack source adapter. The connector declares
// only facts it owns: native scope, exhausted pagination, and source-record
// count. Mindline owns URL extraction, canonicalization, strata, and the sealed
// occurrence denominator.
type NativeBatch struct {
	SchemaVersion             string          `json:"schema_version"`
	WorkspaceID               string          `json:"workspace_id"`
	ChannelID                 string          `json:"channel_id"`
	LowerInclusive            string          `json:"lower_inclusive"`
	UpperInclusive            string          `json:"upper_inclusive"`
	Watermark                 string          `json:"watermark"`
	IncludeThreads            bool            `json:"include_threads"`
	IncludeReplies            bool            `json:"include_replies"`
	PaginationExhausted       bool            `json:"pagination_exhausted"`
	ThreadPaginationExhausted bool            `json:"thread_pagination_exhausted"`
	DeclaredSourceRecords     int             `json:"declared_source_records"`
	Messages                  []NativeMessage `json:"messages"`
}

func BuildAuthorizedExternalManifestFromNativeBatch(batch NativeBatch, receipt assurance.Receipt, commit, configuration string) (ExternalManifest, error) {
	return BuildAuthorizedExternalManifestFromNativeBatchWithEvidence(batch, nil, receipt, commit, configuration)
}

func BuildAuthorizedExternalManifestFromNativeBatchWithEvidence(batch NativeBatch, evidence []acquisition.ImportedEvidence, receipt assurance.Receipt, commit, configuration string) (ExternalManifest, error) {
	if err := assurance.Validate(receipt, commit, configuration); err != nil {
		return ExternalManifest{}, errors.New("pre-live authority rejected private Slack batch")
	}
	if err := validateNativeBatch(batch); err != nil {
		return ExternalManifest{}, err
	}
	return BuildAuthorizedExternalManifest(BuildInput{
		ConnectorKind:    "external_slack_inventory",
		AdapterVersion:   ExternalInventorySchema,
		WorkspaceID:      batch.WorkspaceID,
		ChannelID:        batch.ChannelID,
		LowerInclusive:   batch.LowerInclusive,
		UpperInclusive:   batch.UpperInclusive,
		Watermark:        batch.Watermark,
		Messages:         batch.Messages,
		ImportedEvidence: evidence,
		DataClass:        DataClassPrivateRuntime,
	}, receipt, commit, configuration)
}

func validateNativeBatch(batch NativeBatch) error {
	if batch.SchemaVersion != NativeBatchSchema || strings.TrimSpace(batch.WorkspaceID) == "" || strings.TrimSpace(batch.ChannelID) == "" ||
		strings.TrimSpace(batch.LowerInclusive) == "" || strings.TrimSpace(batch.UpperInclusive) == "" || strings.TrimSpace(batch.Watermark) == "" ||
		!batch.IncludeThreads || !batch.IncludeReplies || !batch.PaginationExhausted || !batch.ThreadPaginationExhausted || len(batch.Messages) == 0 ||
		batch.DeclaredSourceRecords != len(batch.Messages) || len(batch.Messages) > MaximumNativeBatchMessages ||
		!validWebAPIWindow(batch.LowerInclusive, batch.UpperInclusive) || batch.Watermark != batch.UpperInclusive {
		return errors.New("incomplete native Slack occurrence scope")
	}
	seen := make(map[string]bool, len(batch.Messages))
	occurrences := 0
	for _, message := range batch.Messages {
		if strings.TrimSpace(message.NativeMessageID) == "" || !webAPITimestampInWindow(message.Timestamp, batch.LowerInclusive, batch.UpperInclusive) || seen[message.NativeMessageID] {
			return errors.New("invalid native Slack source-record denominator")
		}
		if message.RevisionTimestamp != "" &&
			(!webAPITimestampPattern.MatchString(message.RevisionTimestamp) ||
				compareWebAPITimestamp(message.RevisionTimestamp, message.Timestamp) <= 0) {
			return errors.New("invalid native Slack revision chronology")
		}
		seen[message.NativeMessageID] = true
		occurrences += len(ExtractURLOccurrences(message.Text))
		if occurrences > MaximumNativeURLOccurrences {
			return errors.New("native Slack URL occurrence budget exceeded")
		}
	}
	return nil
}

// ValidateNativeBatch exposes the closed, occurrence-complete connector
// handoff contract to source-neutral consumers such as the personal evidence
// library. Authorization to acquire private Slack data remains at the
// connector boundary; this function validates only the handed-off facts.
func ValidateNativeBatch(batch NativeBatch) error {
	return validateNativeBatch(batch)
}
