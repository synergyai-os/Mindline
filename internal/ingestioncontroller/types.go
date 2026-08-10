package ingestioncontroller

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
)

const (
	RunSchemaVersion             = "mindline_ingestion_run/v0.1"
	LedgerSchemaVersion          = "mindline_ingestion_ledger/v0.1"
	MaximumUnitBytes       int64 = 32 << 20
	MaximumEnvelopeBytes         = int64(128 << 20)
	MaximumEnvelopeRecords       = 100_000
)

type BeginFrame struct {
	Type                     string `json:"type"`
	SchemaVersion            string `json:"schema_version"`
	RunID                    string `json:"run_id"`
	SourceAdapter            string `json:"source_adapter"`
	SourceScope              string `json:"source_scope"`
	ConfigurationFingerprint string `json:"configuration_fingerprint"`
	UnitCount                int    `json:"unit_count"`
	MessageCeiling           int    `json:"message_ceiling"`
	ByteCeiling              int64  `json:"byte_ceiling"`
}

type UnitFrame struct {
	Type          string                       `json:"type"`
	Ordinal       int                          `json:"ordinal"`
	Descriptor    string                       `json:"descriptor"`
	Batch         acquisitionslack.NativeBatch `json:"batch"`
	AuthorClasses map[string]string            `json:"author_classes"`
}

type Envelope struct {
	Begin             BeginFrame
	Units             []UnitFrame
	End               EndFrame
	observedUnitBytes int64
}

type AdoptionReceipt struct {
	DeliveredNative    int `json:"delivered_native"`
	CanonicalDeclared  int `json:"canonical_declared"`
	StructuralExcluded int `json:"structural_excluded"`
}

type EndFrame struct {
	Type               string `json:"type"`
	UnitCount          int    `json:"unit_count"`
	MessageCount       int    `json:"message_count"`
	ByteCount          int64  `json:"byte_count"`
	EnvelopeCommitment string `json:"envelope_commitment"`
}

// Ledger is deliberately structural. It never carries raw frames, URLs,
// individual native identities, or provider cursors.
type Ledger struct {
	SchemaVersion              string `json:"schema_version"`
	RunID                      string `json:"run_id"`
	State                      string `json:"state"`
	SourceAdapter              string `json:"source_adapter"`
	SourceScope                string `json:"source_scope"`
	ConfigurationFingerprint   string `json:"configuration_fingerprint"`
	DeliveredCount             int    `json:"delivered_count"`
	CanonicalDeclaredCount     int    `json:"canonical_declared_count"`
	StructuralExcludedCount    int    `json:"structural_excluded_count"`
	OwnedCount                 int    `json:"owned_count"`
	RetainedCount              int    `json:"retained_count"`
	WithheldCount              int    `json:"withheld_count"`
	OverlapCount               int    `json:"overlap_count"`
	GapCount                   int    `json:"gap_count"`
	ThreadCount                int    `json:"thread_count"`
	AggregateCommitment        string `json:"aggregate_commitment"`
	CanonicalBeforeFingerprint string `json:"canonical_before_fingerprint"`
	CanonicalAfterFingerprint  string `json:"canonical_after_fingerprint"`
	CanonicalBeforeCount       int    `json:"canonical_before_count"`
	CanonicalAfterCount        int    `json:"canonical_after_count"`
}

func (receipt AdoptionReceipt) Valid() bool {
	return receipt.DeliveredNative >= 0 && receipt.CanonicalDeclared >= 0 && receipt.StructuralExcluded >= 0 &&
		receipt.DeliveredNative == receipt.CanonicalDeclared+receipt.StructuralExcluded
}

func commitment(values []string) string {
	hash := sha256.New()
	for _, value := range values {
		hash.Write([]byte(value))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// EnvelopeCommitment binds the exact canonical unit payload, including native
// messages and ephemeral author classes. It is checked in memory only.
func EnvelopeCommitment(units []UnitFrame) string {
	hash := sha256.New()
	for _, unit := range units {
		payload, _ := json.Marshal(unit)
		hash.Write(payload)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
