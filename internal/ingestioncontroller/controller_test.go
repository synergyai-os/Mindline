package ingestioncontroller

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func TestApplySealsAdoptionEquationWithStructuralExclusion(t *testing.T) {
	repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	units := []UnitFrame{
		unit(0, "000", []acquisitionslack.NativeMessage{message("1.000001", "saved evidence"), message("1.000002", "")}, map[string]string{"1.000001": "user", "1.000002": "non_user"}),
		unit(1, "001", []acquisitionslack.NativeMessage{message("1.000001", "saved evidence"), message("1.000003", "not retained verbatim")}, map[string]string{"1.000001": "user", "1.000003": "unknown"}),
	}
	envelope := testEnvelope(units)
	result, err := (Controller{Repository: repository, Ledger: ledgerStore}).Apply(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "complete" || result.DeliveredCount != 4 || result.CanonicalDeclaredCount != 3 || result.StructuralExcludedCount != 1 || result.OwnedCount != 3 || result.OverlapCount != 1 || result.RetainedCount != 1 || result.WithheldCount != 1 {
		t.Fatalf("unexpected structural adoption result: %+v", result)
	}
	status, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.RecordCount != 2 {
		t.Fatalf("expected retained plus withheld canonical records, got %+v", status)
	}
	library, err := repository.Load()
	if err != nil || len(library.Records) != 2 || library.Records[1].RawText != "[Capture has no text]" || !contains(library.Records[1].Missingness, "withheld_unknown_author") {
		t.Fatalf("unknown author was not a content-free searchable withholding: %#v err=%v", library.Records, err)
	}
	persisted, err := ledgerStore.Load()
	if err != nil || persisted != result {
		t.Fatalf("ledger was not atomically advanced: %#v err=%v", persisted, err)
	}
}

func TestDecodeEnvelopeBindsFullUnitPayloadAndUnitBytes(t *testing.T) {
	units := []UnitFrame{unit(0, "000", []acquisitionslack.NativeMessage{message("1.000001", "saved")}, map[string]string{"1.000001": "user"})}
	begin := BeginFrame{Type: "begin", SchemaVersion: RunSchemaVersion, RunID: "run-wire", SourceAdapter: "slack", SourceScope: "slack:T:D", ConfigurationFingerprint: "configuration-test", UnitCount: 1, MessageCeiling: 10, ByteCeiling: 4096}
	unitBytes, _ := json.Marshal(units[0])
	end := EndFrame{Type: "end", UnitCount: 1, MessageCount: 1, ByteCount: int64(len(unitBytes) + 1), EnvelopeCommitment: EnvelopeCommitment(units)}
	beginBytes, _ := json.Marshal(begin)
	endBytes, _ := json.Marshal(end)
	wire := bytes.Join([][]byte{beginBytes, unitBytes, endBytes}, []byte("\n"))
	wire = append(wire, '\n')
	decoded, err := DecodeEnvelope(bytes.NewReader(wire))
	if err != nil || decoded.observedUnitBytes != int64(len(unitBytes)+1) {
		t.Fatalf("valid closed envelope rejected: %#v err=%v", decoded, err)
	}
	units[0].Batch.Messages[0].Text = "mutated"
	mutatedUnit, _ := json.Marshal(units[0])
	mutated := bytes.Join([][]byte{beginBytes, mutatedUnit, endBytes}, []byte("\n"))
	if _, err := DecodeEnvelope(bytes.NewReader(mutated)); err == nil {
		t.Fatal("content mutation was not bound by envelope commitment")
	}
}

func TestApplyRejectsMissingThreadParentBeforeImport(t *testing.T) {
	repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	reply := message("1.000001", "reply")
	reply.ThreadParentID = "1.000099"
	if _, err := (Controller{Repository: repository, Ledger: ledgerStore}).Apply(testEnvelope([]UnitFrame{unit(0, "000", []acquisitionslack.NativeMessage{reply}, map[string]string{"1.000001": "user"})})); err == nil {
		t.Fatal("missing thread parent was accepted")
	}
	status, _ := repository.Status()
	if status.RecordCount != 0 {
		t.Fatalf("thread gap mutated canonical evidence: %+v", status)
	}
}

func TestApplyRejectsOverlappingDispositionConflictBeforeImport(t *testing.T) {
	repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	units := []UnitFrame{
		unit(0, "000", []acquisitionslack.NativeMessage{message("1.000001", "saved")}, map[string]string{"1.000001": "user"}),
		unit(1, "001", []acquisitionslack.NativeMessage{message("1.000001", "saved")}, map[string]string{"1.000001": "unknown"}),
	}
	if _, err := (Controller{Repository: repository, Ledger: ledgerStore}).Apply(testEnvelope(units)); err == nil {
		t.Fatal("expected overlapping author/disposition conflict")
	}
	status, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.RecordCount != 0 {
		t.Fatalf("conflict reached canonical repository: %+v", status)
	}
}

func testEnvelope(units []UnitFrame) Envelope {
	return Envelope{Begin: BeginFrame{Type: "begin", SchemaVersion: RunSchemaVersion, RunID: "run-test", SourceAdapter: "slack", SourceScope: "slack:T:D", ConfigurationFingerprint: "configuration-test", UnitCount: len(units), MessageCeiling: 100, ByteCeiling: 1}, Units: units, End: EndFrame{Type: "end", UnitCount: len(units), MessageCount: messageCount(units), ByteCount: 1, EnvelopeCommitment: EnvelopeCommitment(units)}, observedUnitBytes: 1}
}

func unit(ordinal int, descriptor string, messages []acquisitionslack.NativeMessage, classes map[string]string) UnitFrame {
	return UnitFrame{Type: "unit", Ordinal: ordinal, Descriptor: descriptor, AuthorClasses: classes, Batch: acquisitionslack.NativeBatch{SchemaVersion: acquisitionslack.NativeBatchSchema, WorkspaceID: "T", ChannelID: "D", LowerInclusive: "1.000000", UpperInclusive: "2.000000", Watermark: "2.000000", IncludeThreads: true, IncludeReplies: true, PaginationExhausted: true, ThreadPaginationExhausted: true, DeclaredSourceRecords: len(messages), Messages: messages}}
}

func message(id, text string) acquisitionslack.NativeMessage {
	return acquisitionslack.NativeMessage{NativeMessageID: id, Timestamp: id, Text: text}
}
func messageCount(units []UnitFrame) int {
	total := 0
	for _, unit := range units {
		total += len(unit.Batch.Messages)
	}
	return total
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
