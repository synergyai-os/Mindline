package controlrun

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

var (
	generationPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	runIDPattern      = regexp.MustCompile(`^run-[0-9]{8}T[0-9]{6}Z-[a-z2-7]{26}$`)
)

func randomGeneration(reader io.Reader) (string, error) {
	buffer := make([]byte, 32)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return "", errors.New("control run selection generation unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func NewRunID(now time.Time, entropy io.Reader) (string, error) {
	if entropy == nil || now.IsZero() {
		return "", ErrInvalid
	}
	buffer := make([]byte, 16)
	if _, err := io.ReadFull(entropy, buffer); err != nil {
		return "", errors.New("run identity generation unavailable")
	}
	suffix := encodeBase32Lower(buffer)
	return "run-" + now.UTC().Truncate(time.Second).Format("20060102T150405Z") + "-" + suffix, nil
}

func encodeBase32Lower(data []byte) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	encoded := make([]byte, 0, (len(data)*8+4)/5)
	var accumulator uint64
	var bits uint
	for _, value := range data {
		accumulator = (accumulator << 8) | uint64(value)
		bits += 8
		for bits >= 5 {
			shift := bits - 5
			encoded = append(encoded, alphabet[(accumulator>>shift)&31])
			bits -= 5
			if bits == 0 {
				accumulator = 0
			} else {
				accumulator &= (uint64(1) << bits) - 1
			}
		}
	}
	if bits > 0 {
		encoded = append(encoded, alphabet[(accumulator<<(5-bits))&31])
	}
	return string(encoded)
}

func ValidateRunID(runID string) error {
	if !runIDPattern.MatchString(runID) {
		return ErrInvalid
	}
	if _, err := time.Parse("20060102T150405Z", runID[4:20]); err != nil {
		return ErrInvalid
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	last := strings.IndexByte(alphabet, runID[len(runID)-1])
	if last < 0 || last&3 != 0 {
		return ErrInvalid
	}
	return nil
}

func sealDocument(version uint64, generation, selectedRunID string) Document {
	document := Document{SchemaVersion: SchemaVersion, Version: version, Generation: generation, SelectedRunID: selectedRunID}
	document.Fingerprint = fingerprintDocument(document)
	return document
}

func validateDocument(document Document) (Document, error) {
	if document.SchemaVersion != SchemaVersion {
		return Document{}, ErrUnsupported
	}
	if document.Version == 0 || !generationPattern.MatchString(document.Generation) {
		return Document{}, ErrInvalid
	}
	if document.SelectedRunID != "" {
		if err := ValidateRunID(document.SelectedRunID); err != nil {
			return Document{}, ErrInvalid
		}
	}
	if document.Fingerprint != fingerprintDocument(document) {
		return Document{}, ErrInvalid
	}
	return document, nil
}

func fingerprintDocument(document Document) string {
	projection := struct {
		SchemaVersion string `json:"schema_version"`
		Version       uint64 `json:"version"`
		Generation    string `json:"generation"`
		SelectedRunID string `json:"selected_run_id"`
	}{document.SchemaVersion, document.Version, document.Generation, document.SelectedRunID}
	data, _ := json.Marshal(projection)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func parseDocument(raw []byte) (Document, error) {
	if int64(len(raw)) > MaxDocumentBytes {
		return Document{}, ErrInvalid
	}
	var document Document
	if err := privateio.DecodeJSONStrict(raw, &document); err != nil {
		return Document{}, ErrInvalid
	}
	return validateDocument(document)
}

func canonicalDocumentBytes(document Document) ([]byte, error) {
	data, err := json.Marshal(document)
	if err != nil || int64(len(data)+1) > MaxDocumentBytes {
		return nil, ErrInvalid
	}
	return append(data, '\n'), nil
}

func problemFingerprint(code string, current, backup []byte) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, code+"\x00")
	currentSum := sha256.Sum256(current)
	backupSum := sha256.Sum256(backup)
	_, _ = hash.Write(currentSum[:])
	_, _ = hash.Write(backupSum[:])
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func readableRevision(raw []byte) *Revision {
	var object map[string]json.RawMessage
	if privateio.DecodeJSONStrict(raw, &object) != nil {
		return nil
	}
	var version uint64
	var generation string
	if json.Unmarshal(object["version"], &version) != nil || json.Unmarshal(object["generation"], &generation) != nil || version == 0 || !generationPattern.MatchString(generation) {
		return nil
	}
	revision := Revision{Version: version, Generation: generation}
	return &revision
}
