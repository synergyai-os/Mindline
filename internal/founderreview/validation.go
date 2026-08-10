package founderreview

import (
	"bytes"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

func validateRecord(record Record) error {
	if record.SchemaVersion != SchemaVersion || !runIDPattern.MatchString(record.RunID) || !runIDPattern.MatchString(record.ProofRunID) || !proofHashPattern.MatchString(record.StructuralProofFingerprint) || !proofHashPattern.MatchString(record.CitedRecordsFingerprint) || !proofHashPattern.MatchString(record.EventID) || !proofHashPattern.MatchString(record.RetryTokenHash) {
		return ErrInvalid
	}
	switch record.Verdict {
	case VerdictUseful, VerdictNotUseful, VerdictDeclined:
	default:
		return ErrInvalid
	}
	parsed, err := time.Parse(time.RFC3339, record.RecordedAt)
	if err != nil || parsed.UTC().Format(time.RFC3339) != record.RecordedAt {
		return ErrInvalid
	}
	return nil
}

func validateRequest(request Request) error {
	if !runIDPattern.MatchString(request.ProofRunID) || !proofHashPattern.MatchString(request.StructuralProofFingerprint) || !proofHashPattern.MatchString(request.CitedRecordsFingerprint) || !retryTokenPattern.MatchString(request.RetryToken) {
		return ErrInvalid
	}
	switch request.Verdict {
	case VerdictUseful, VerdictNotUseful, VerdictDeclined:
	default:
		return ErrInvalid
	}
	return nil
}

func parseRecord(data []byte) (Record, error) {
	if int64(len(data)) > MaxRecordBytes {
		return Record{}, ErrInvalid
	}
	var record Record
	if err := privateio.DecodeJSONStrict(bytes.TrimSpace(data), &record); err != nil {
		return Record{}, ErrInvalid
	}
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	return record, nil
}
