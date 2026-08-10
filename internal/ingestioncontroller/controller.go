package ingestioncontroller

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"

	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	slackadapter "github.com/synergyai-os/Mindline/internal/adapters/slack"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

type Controller struct {
	Repository *personalmemory.FileRepository
	Ledger     *LedgerStore
}

func DecodeEnvelope(reader io.Reader) (Envelope, error) {
	if reader == nil {
		return Envelope{}, errors.New("ingestion envelope is missing")
	}
	buffer := bufio.NewReader(reader)
	var envelope Envelope
	var streamBytes, unitBytes int64
	phase := "begin"
	for {
		line, err := readBoundedFrame(buffer, MaximumUnitBytes)
		if len(line) > 0 {
			rawBytes := int64(len(line))
			streamBytes += rawBytes
			if streamBytes > MaximumEnvelopeBytes {
				return Envelope{}, errors.New("ingestion envelope exceeds bounded framing")
			}
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				return Envelope{}, errors.New("ingestion envelope contains empty frame")
			}
			var kind struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(line, &kind) != nil {
				return Envelope{}, errors.New("ingestion envelope frame is invalid")
			}
			switch kind.Type {
			case "begin":
				if phase != "begin" || privateio.DecodeJSONStrict(line, &envelope.Begin) != nil {
					return Envelope{}, errors.New("ingestion begin frame is invalid")
				}
				phase = "unit"
			case "unit":
				var unit UnitFrame
				if phase != "unit" || privateio.DecodeJSONStrict(line, &unit) != nil {
					return Envelope{}, errors.New("ingestion unit frame is invalid")
				}
				envelope.Units = append(envelope.Units, unit)
				if rawBytes > MaximumUnitBytes {
					return Envelope{}, errors.New("ingestion unit exceeds bounded framing")
				}
				unitBytes += rawBytes
			case "end":
				if phase != "unit" || privateio.DecodeJSONStrict(line, &envelope.End) != nil {
					return Envelope{}, errors.New("ingestion end frame is invalid")
				}
				phase = "end"
			default:
				return Envelope{}, errors.New("ingestion envelope frame type is invalid")
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Envelope{}, errors.New("ingestion envelope cannot be read")
		}
	}
	if phase != "end" {
		return Envelope{}, errors.New("ingestion envelope is truncated")
	}
	envelope.observedUnitBytes = unitBytes
	if err := validateEnvelope(envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// readBoundedFrame never allows ReadBytes-style unbounded allocation before
// the framing cap is checked. All envelope frame kinds fit within the unit
// ceiling; the aggregate ceiling is enforced separately by DecodeEnvelope.
func readBoundedFrame(reader *bufio.Reader, maximum int64) ([]byte, error) {
	var frame bytes.Buffer
	for {
		chunk, err := reader.ReadSlice('\n')
		if int64(frame.Len())+int64(len(chunk)) > maximum {
			return nil, errors.New("ingestion frame exceeds bounded framing")
		}
		_, _ = frame.Write(chunk)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return frame.Bytes(), err
		}
	}
}

func validateEnvelope(envelope Envelope) error {
	begin, end := envelope.Begin, envelope.End
	if begin.Type != "begin" || begin.SchemaVersion != RunSchemaVersion || strings.TrimSpace(begin.RunID) == "" ||
		strings.TrimSpace(begin.SourceAdapter) != "slack" || strings.TrimSpace(begin.SourceScope) == "" ||
		strings.TrimSpace(begin.ConfigurationFingerprint) == "" || begin.UnitCount <= 0 || begin.UnitCount > MaximumEnvelopeRecords ||
		begin.MessageCeiling <= 0 || begin.MessageCeiling > MaximumEnvelopeRecords || begin.ByteCeiling <= 0 || begin.ByteCeiling > MaximumEnvelopeBytes ||
		end.Type != "end" || end.UnitCount != begin.UnitCount || end.ByteCount != envelope.observedUnitBytes || envelope.observedUnitBytes > begin.ByteCeiling {
		return errors.New("ingestion envelope totals are invalid")
	}
	if len(envelope.Units) != begin.UnitCount {
		return errors.New("ingestion envelope unit count is incomplete")
	}
	seenOrdinals := make(map[int]bool, len(envelope.Units))
	messageCount := 0
	priorDescriptor := ""
	for index, unit := range envelope.Units {
		if unit.Type != "unit" || unit.Ordinal != index || unit.Ordinal < 0 || unit.Ordinal >= begin.UnitCount || seenOrdinals[unit.Ordinal] || strings.TrimSpace(unit.Descriptor) == "" {
			return errors.New("ingestion unit descriptor is invalid")
		}
		seenOrdinals[unit.Ordinal] = true
		if priorDescriptor != "" && unit.Descriptor <= priorDescriptor {
			return errors.New("ingestion descriptors are not uniquely ordered")
		}
		priorDescriptor = unit.Descriptor
		frame := slackadapter.RunFrame{Descriptor: unit.Descriptor, Batch: unit.Batch, AuthorClasses: unit.AuthorClasses}
		if err := slackadapter.ValidateRunFrame(frame); err != nil {
			return errors.New("ingestion strict Slack frame is invalid")
		}
		if "slack:"+unit.Batch.WorkspaceID+":"+unit.Batch.ChannelID != begin.SourceScope {
			return errors.New("ingestion source scope is inconsistent")
		}
		messageCount += len(unit.Batch.Messages)
	}
	if messageCount != end.MessageCount || messageCount > begin.MessageCeiling || messageCount > MaximumEnvelopeRecords || end.EnvelopeCommitment != EnvelopeCommitment(envelope.Units) {
		return errors.New("ingestion envelope commitment is invalid")
	}
	return nil
}

func (controller Controller) Apply(envelope Envelope) (Ledger, error) {
	if controller.Repository == nil || controller.Ledger == nil {
		return Ledger{}, errors.New("ingestion controller is incomplete")
	}
	// Re-validate a decoded envelope before any canonical mutation.
	if err := validateEnvelope(envelope); err != nil {
		return Ledger{}, err
	}
	applyLock, err := controller.Ledger.AcquireApplyLock()
	if err != nil {
		return Ledger{}, err
	}
	defer applyLock.Close()
	before, err := controller.Repository.Status()
	if err != nil {
		return Ledger{}, errors.New("canonical readback unavailable")
	}
	ledger := Ledger{SchemaVersion: LedgerSchemaVersion, RunID: envelope.Begin.RunID, State: "recovering", SourceAdapter: envelope.Begin.SourceAdapter,
		SourceScope: envelope.Begin.SourceScope, ConfigurationFingerprint: envelope.Begin.ConfigurationFingerprint,
		CanonicalBeforeFingerprint: before.Fingerprint, CanonicalBeforeCount: before.RecordCount,
		CanonicalAfterFingerprint: before.Fingerprint, CanonicalAfterCount: before.RecordCount,
		AggregateCommitment: commitment(nil)}
	if err := controller.Ledger.Save(ledger); err != nil {
		return Ledger{}, err
	}

	type identityFact struct {
		class       string
		disposition slackadapter.Disposition
		message     acquisitionslack.NativeMessage
	}
	facts := map[string]identityFact{}
	parents := map[string]bool{}
	keys := []string{}
	for _, unit := range envelope.Units {
		for _, message := range unit.Batch.Messages {
			key := unit.Batch.WorkspaceID + "\x00" + unit.Batch.ChannelID + "\x00" + message.NativeMessageID
			class := unit.AuthorClasses[message.NativeMessageID]
			disposition, err := slackadapter.DispositionFor(message, class)
			if err != nil {
				return controller.fail(ledger)
			}
			if prior, exists := facts[key]; exists {
				if prior.class != class || prior.disposition != disposition {
					return controller.fail(ledger)
				}
				if !samePayload(prior.message, message) && !truthfulTransition(prior.message, message) {
					return controller.fail(ledger)
				}
				ledger.OverlapCount++
				facts[key] = identityFact{class: class, disposition: disposition, message: message}
			} else {
				facts[key] = identityFact{class: class, disposition: disposition, message: message}
				keys = append(keys, key)
				if disposition == slackadapter.DispositionExclude {
					ledger.StructuralExcludedCount++
				} else if disposition == slackadapter.DispositionWithhold {
					ledger.WithheldCount++
				} else {
					ledger.RetainedCount++
				}
			}
			if strings.TrimSpace(message.ThreadParentID) != "" {
				parents[unit.Batch.WorkspaceID+"\x00"+unit.Batch.ChannelID+"\x00"+message.ThreadParentID] = true
				ledger.ThreadCount++
			}
		}
	}
	for parent := range parents {
		if _, exists := facts[parent]; !exists {
			ledger.GapCount++
			return controller.fail(ledger)
		}
	}
	sort.Strings(keys)
	ledger.OwnedCount, ledger.AggregateCommitment = len(keys), commitment(keys)
	captures := make([]personalmemory.CaptureBatch, 0, len(envelope.Units))
	for _, unit := range envelope.Units {
		frame := slackadapter.RunFrame{Descriptor: unit.Descriptor, Batch: unit.Batch, AuthorClasses: unit.AuthorClasses}
		capture, dispositions, err := slackadapter.CaptureBatchForAdoption(frame)
		if err != nil {
			return controller.fail(ledger)
		}
		receipt := AdoptionReceipt{DeliveredNative: len(unit.Batch.Messages)}
		for _, disposition := range dispositions {
			if disposition == slackadapter.DispositionExclude {
				receipt.StructuralExcluded++
			}
		}
		if capture.DeclaredRecords > 0 {
			captures = append(captures, capture)
			receipt.CanonicalDeclared = capture.DeclaredRecords
		}
		if !receipt.Valid() {
			return controller.fail(ledger)
		}
		ledger.DeliveredCount += receipt.DeliveredNative
		ledger.CanonicalDeclaredCount += receipt.CanonicalDeclared
	}
	if len(captures) > 0 {
		memoryReceipts, importErr := controller.Repository.ImportManyWithinBudget(captures, personalmemory.MaximumCaptureLibraryBytes)
		if importErr != nil || len(memoryReceipts) != len(captures) {
			return controller.fail(ledger)
		}
		for index, memoryReceipt := range memoryReceipts {
			capture := captures[index]
			if memoryReceipt.DeclaredRecords != capture.DeclaredRecords ||
				memoryReceipt.InsertedRecords+memoryReceipt.UpdatedRecords+memoryReceipt.UnchangedRecords != capture.DeclaredRecords {
				return controller.fail(ledger)
			}
		}
		library, readErr := controller.Repository.Load()
		if readErr != nil {
			return controller.fail(ledger)
		}
		for _, capture := range captures {
			if !capturesCurrent(library, capture) {
				return controller.fail(ledger)
			}
		}
	}
	after, err := controller.Repository.Status()
	if err != nil {
		return controller.fail(ledger)
	}
	ledger.CanonicalAfterFingerprint, ledger.CanonicalAfterCount = after.Fingerprint, after.RecordCount
	ledger.State = "complete"
	if err := controller.Ledger.Save(ledger); err != nil {
		return Ledger{}, err
	}
	return ledger, nil
}

func (controller Controller) fail(ledger Ledger) (Ledger, error) {
	ledger.State = "incomplete"
	_ = controller.Ledger.Save(ledger)
	return Ledger{}, errors.New("ingestion reconciliation failed")
}

func samePayload(left, right acquisitionslack.NativeMessage) bool { return left == right }

func truthfulTransition(before, after acquisitionslack.NativeMessage) bool {
	if before.NativeMessageID != after.NativeMessageID || before.Timestamp != after.Timestamp || before.ThreadParentID != after.ThreadParentID {
		return false
	}
	if before.EditDeleteState == "deleted" || before.EditDeleteState == "tombstone" {
		return false
	}
	return after.EditDeleteState == "edited" || after.EditDeleteState == "deleted" || after.EditDeleteState == "tombstone"
}

func capturesCurrent(library personalmemory.Library, batch personalmemory.CaptureBatch) bool {
	byKey := make(map[string]personalmemory.CaptureRecord, len(library.Records))
	for _, record := range library.Records {
		byKey[record.IdempotencyKey] = record
	}
	historical := make(map[string]map[string]bool, len(library.Revisions))
	for _, revision := range library.Revisions {
		if historical[revision.Record.IdempotencyKey] == nil {
			historical[revision.Record.IdempotencyKey] = map[string]bool{}
		}
		historical[revision.Record.IdempotencyKey][revision.Record.ContentHash] = true
	}
	for _, record := range batch.Records {
		current, exists := byKey[record.IdempotencyKey]
		if !exists || (current.ContentHash != record.ContentHash &&
			!historical[record.IdempotencyKey][record.ContentHash]) {
			return false
		}
	}
	return true
}
