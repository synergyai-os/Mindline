package runjournal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/synergyai-os/Mindline/internal/orchestration"
)

const (
	JournalSchemaVersion    = "mindline-activation-run-journal/v0.1"
	ProjectionSchemaVersion = "mindline-activation-run-projection/v0.1"
	LeaseSchemaVersion      = "mindline-activation-run-lease/v0.1"
	journalFilename         = "journal.json"
	projectionFilename      = "projection.json"
	leaseFilename           = "lease.json"
	runLockFilename         = ".run.lock"
	genesisHash             = "genesis"
)

var (
	ErrJournalCorrupt  = errors.New("run journal is corrupt")
	ErrJournalTooLarge = errors.New("run journal exceeds configured limit")
	ErrEventTooLarge   = errors.New("run event exceeds configured limit")
	ErrUnknownRunFile  = errors.New("unknown file in run journal directory")
	ErrInvalidRunID    = errors.New("invalid run id")
	ErrLeaseHeld       = errors.New("run lease is held")
	ErrLeaseExpired    = errors.New("run lease expired")
	ErrLeaseMismatch   = errors.New("run lease does not match")
	ErrLeaseNotFound   = errors.New("run lease not found")
)

type journal struct {
	SchemaVersion string                `json:"schema_version"`
	Fingerprint   string                `json:"fingerprint"`
	RunID         orchestration.RunID   `json:"run_id"`
	Events        []orchestration.Event `json:"events"`
}

func sealEvent(event orchestration.Event, sequence uint64, previousHash string) (orchestration.Event, error) {
	if event.Sequence != sequence || event.EventID != "" || event.PayloadHash != "" || event.EventHash != "" || event.PreviousHash != "" {
		return orchestration.Event{}, fmt.Errorf("%w: caller supplied sealed event metadata", ErrJournalCorrupt)
	}
	if err := validateEventPayload(event); err != nil {
		return orchestration.Event{}, fmt.Errorf("%w: %v", ErrJournalCorrupt, err)
	}
	var compact json.RawMessage
	if err := json.Unmarshal(event.Payload, &compact); err != nil {
		return orchestration.Event{}, fmt.Errorf("%w: invalid payload", ErrJournalCorrupt)
	}
	canonical, err := json.Marshal(compact)
	if err != nil {
		return orchestration.Event{}, err
	}
	event.Payload = canonical
	event.Sequence = sequence
	event.PreviousHash = previousHash
	event.PayloadHash = hashBytes(event.Payload)
	event.EventID = hashBytes([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s", event.RunID, event.Sequence, event.Type, event.PayloadHash)))
	event.EventHash = ""
	event.EventHash = orchestration.Fingerprint(event)
	return event, nil
}

func validateEvent(event orchestration.Event, runID orchestration.RunID, sequence uint64, previousHash string, maximumEventBytes int64) error {
	if event.RunID != runID || event.Sequence != sequence || event.PreviousHash != previousHash || int64(len(event.Payload)) > maximumEventBytes {
		return fmt.Errorf("%w: event binding mismatch", ErrJournalCorrupt)
	}
	canonicalPayload, err := canonicalJSON(event.Payload)
	if err != nil {
		return fmt.Errorf("%w: invalid payload JSON", ErrJournalCorrupt)
	}
	event.Payload = canonicalPayload
	if err := validateEventPayload(event); err != nil {
		return fmt.Errorf("%w: invalid transition payload", ErrJournalCorrupt)
	}
	if event.PayloadHash != hashBytes(event.Payload) {
		return fmt.Errorf("%w: payload hash mismatch", ErrJournalCorrupt)
	}
	wantID := hashBytes([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s", event.RunID, event.Sequence, event.Type, event.PayloadHash)))
	if event.EventID != wantID {
		return fmt.Errorf("%w: event id mismatch", ErrJournalCorrupt)
	}
	fingerprint := event.EventHash
	event.EventHash = ""
	if fingerprint == "" || fingerprint != orchestration.Fingerprint(event) {
		return fmt.Errorf("%w: event hash mismatch", ErrJournalCorrupt)
	}
	return nil
}

func validateEventPayload(event orchestration.Event) error {
	switch event.Type {
	case orchestration.EventRunTransition:
		_, err := orchestration.DecodeTransitionPayload(event)
		return err
	case orchestration.EventAuthorityReference:
		_, err := orchestration.DecodeAuthorityReferencePayload(event)
		return err
	default:
		return orchestration.ErrInvalidEvent
	}
}

func canonicalJSON(value []byte) ([]byte, error) {
	var compact json.RawMessage
	if err := json.Unmarshal(value, &compact); err != nil {
		return nil, err
	}
	return json.Marshal(compact)
}

func sealJournal(value *journal) {
	value.Fingerprint = ""
	value.Fingerprint = orchestration.Fingerprint(*value)
}

func validateJournal(value journal, runID orchestration.RunID, maximumEventBytes int64) error {
	if value.SchemaVersion != JournalSchemaVersion || value.RunID != runID || value.Fingerprint == "" {
		return fmt.Errorf("%w: invalid journal header", ErrJournalCorrupt)
	}
	fingerprint := value.Fingerprint
	value.Fingerprint = ""
	if fingerprint != orchestration.Fingerprint(value) {
		return fmt.Errorf("%w: journal fingerprint mismatch", ErrJournalCorrupt)
	}
	previous := genesisHash
	for index, event := range value.Events {
		if err := validateEvent(event, runID, uint64(index+1), previous, maximumEventBytes); err != nil {
			return fmt.Errorf("%w: event %d: %v", ErrJournalCorrupt, index+1, err)
		}
		previous = event.EventHash
	}
	return nil
}

func hashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}
