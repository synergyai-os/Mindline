package slack

import (
	"errors"
	"strings"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/contentguard"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

// RunFrame is the in-memory adapter handoff used by the ingestion controller.
// Author classes are connector facts only; neither credentials nor cursors are
// admitted to this contract.
type RunFrame struct {
	Descriptor    string                       `json:"descriptor"`
	Batch         acquisitionslack.NativeBatch `json:"batch"`
	AuthorClasses map[string]string            `json:"author_classes"`
}

type Disposition string

const (
	DispositionRetain   Disposition = "retain"
	DispositionWithhold Disposition = "withhold"
	DispositionExclude  Disposition = "exclude"
)

func ValidateRunFrame(frame RunFrame) error {
	if strings.TrimSpace(frame.Descriptor) == "" {
		return errors.New("Slack run-frame descriptor is missing")
	}
	if err := acquisitionslack.ValidateNativeBatch(frame.Batch); err != nil {
		return err
	}
	if len(frame.AuthorClasses) != len(frame.Batch.Messages) {
		return errors.New("Slack run-frame author classes are incomplete")
	}
	for _, message := range frame.Batch.Messages {
		class := strings.TrimSpace(frame.AuthorClasses[message.NativeMessageID])
		if class != "user" && class != "non_user" && class != "unknown" {
			return errors.New("invalid Slack structural author class")
		}
	}
	for identity := range frame.AuthorClasses {
		found := false
		for _, message := range frame.Batch.Messages {
			if message.NativeMessageID == identity {
				found = true
				break
			}
		}
		if !found {
			return errors.New("Slack run-frame author class identity is unknown")
		}
	}
	return nil
}

func DispositionFor(message acquisitionslack.NativeMessage, authorClass string) (Disposition, error) {
	authorClass = strings.TrimSpace(authorClass)
	if authorClass != "user" && authorClass != "non_user" && authorClass != "unknown" {
		return "", errors.New("invalid Slack structural author class")
	}
	if authorClass == "non_user" && strings.TrimSpace(message.Text) == "" && message.AttachmentCount == 0 && message.PrivateFileCount == 0 {
		return DispositionExclude, nil
	}
	if authorClass == "unknown" || contentguard.ContainsNonPersistableContent(message.Text) {
		return DispositionWithhold, nil
	}
	return DispositionRetain, nil
}

// CaptureBatchForAdoption applies the only permitted pre-canonical filter:
// objective non-user empty transport artifacts. Withheld values become a fixed
// content-free placeholder while retaining their native identity.
func CaptureBatchForAdoption(frame RunFrame) (personalmemory.CaptureBatch, map[string]Disposition, error) {
	if err := ValidateRunFrame(frame); err != nil {
		return personalmemory.CaptureBatch{}, nil, err
	}
	records := make([]personalmemory.CaptureRecord, 0, len(frame.Batch.Messages))
	dispositions := make(map[string]Disposition, len(frame.Batch.Messages))
	for _, message := range frame.Batch.Messages {
		class := frame.AuthorClasses[message.NativeMessageID]
		if class == "" {
			class = "unknown"
		}
		disposition, err := DispositionFor(message, class)
		if err != nil {
			return personalmemory.CaptureBatch{}, nil, err
		}
		dispositions[message.NativeMessageID] = disposition
		if disposition == DispositionExclude {
			continue
		}
		missingness := []string{"permalink_unavailable"}
		sourceRef := "slack://" + frame.Batch.WorkspaceID + "/" + frame.Batch.ChannelID + "/" + message.NativeMessageID
		if message.Permalink != "" {
			sourceRef, missingness = message.Permalink, nil
		}
		if disposition == DispositionWithhold {
			message.Text = ""
			if class == "unknown" {
				missingness = append(missingness, "withheld_unknown_author")
			} else {
				missingness = append(missingness, "withheld_unsafe_content")
			}
		}
		occurredAt, err := acquisition.NativeTimestampToRFC3339(message.Timestamp)
		if err != nil {
			return personalmemory.CaptureBatch{}, nil, errors.New("invalid personal evidence timestamp")
		}
		record, err := personalmemory.NewCaptureRecord(personalmemory.CaptureRecordInput{
			SourceAdapter: "slack", SourceScopeID: frame.Batch.WorkspaceID, SourceContainerID: frame.Batch.ChannelID,
			ExternalID: message.NativeMessageID, OccurredAt: occurredAt, AuthorID: message.AuthorID, AuthorName: message.AuthorName,
			SourceRef: sourceRef, RawText: message.Text, ThreadParentID: message.ThreadParentID, AttachmentCount: message.AttachmentCount,
			PrivateFileCount: message.PrivateFileCount, EditDeleteState: message.EditDeleteState, Missingness: missingness,
		})
		if err != nil {
			return personalmemory.CaptureBatch{}, nil, err
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		// FileRepository correctly refuses an empty canonical batch. The
		// controller records this as a structural-only adoption receipt.
		return personalmemory.CaptureBatch{}, dispositions, nil
	}
	capture, err := personalmemory.NewCaptureBatch(personalmemory.CaptureBatchInput{
		SourceIdentity: "slack:" + frame.Batch.WorkspaceID + ":" + frame.Batch.ChannelID,
		LowerInclusive: frame.Batch.LowerInclusive, UpperInclusive: frame.Batch.UpperInclusive,
		Watermark: frame.Batch.Watermark, DeclaredRecords: len(records), Records: records,
	})
	return capture, dispositions, err
}

// CaptureBatchFromRunFrame deliberately preserves the strict native batch.
// Structural author classes are consumed by reconciliation accounting; content
// normalization remains the established source-neutral adapter path.
func CaptureBatchFromRunFrame(frame RunFrame) (personalmemory.CaptureBatch, error) {
	if err := ValidateRunFrame(frame); err != nil {
		return personalmemory.CaptureBatch{}, err
	}
	return CaptureBatchFromNative(frame.Batch)
}
