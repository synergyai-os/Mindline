package slack

import (
	"errors"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/routing"
)

// CaptureBatchFromNative is the Slack adapter boundary into Mindline's
// source-neutral canonical personal-evidence contract.
func CaptureBatchFromNative(batch acquisitionslack.NativeBatch) (personalmemory.CaptureBatch, error) {
	if err := acquisitionslack.ValidateNativeBatch(batch); err != nil {
		return personalmemory.CaptureBatch{}, err
	}
	records := make([]personalmemory.CaptureRecord, 0, len(batch.Messages))
	for _, message := range batch.Messages {
		occurredAt, err := acquisition.NativeTimestampToRFC3339(message.Timestamp)
		if err != nil {
			return personalmemory.CaptureBatch{}, errors.New("invalid personal evidence timestamp")
		}
		sourceRef := "slack://" + batch.WorkspaceID + "/" + batch.ChannelID + "/" + message.NativeMessageID
		missingness := []string{}
		permalink := message.Permalink
		if permalink == "" {
			permalink = sourceRef
			missingness = append(missingness, "permalink_unavailable")
		} else {
			safe, state, err := routing.PrepareURLForStorage(permalink)
			if err != nil || state == routing.URLStorageSensitiveRedacted || safe == "" {
				permalink = sourceRef
				missingness = append(missingness, "permalink_redacted")
			} else {
				permalink = safe
			}
		}
		record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
			SourceAdapter: "slack", SourceScopeID: batch.WorkspaceID,
			SourceContainerID: batch.ChannelID, ExternalID: message.NativeMessageID,
			OccurredAt: occurredAt, AuthorID: message.AuthorID, AuthorName: message.AuthorName,
			SourceRef: permalink, RawText: message.Text, ThreadParentID: message.ThreadParentID,
			AttachmentCount: message.AttachmentCount, PrivateFileCount: message.PrivateFileCount,
			EditDeleteState: message.EditDeleteState, Missingness: missingness,
		})
		if err != nil {
			return personalmemory.CaptureBatch{}, err
		}
		records = append(records, record)
	}
	return personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "slack:" + batch.WorkspaceID + ":" + batch.ChannelID,
		LowerInclusive: batch.LowerInclusive, UpperInclusive: batch.UpperInclusive,
		Watermark: batch.Watermark, DeclaredRecords: batch.DeclaredSourceRecords,
		Records: records,
	})
}
