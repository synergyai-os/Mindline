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

func TestApplyRejectsStructurallyExcludedThreadParentBeforeImport(t *testing.T) {
	repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	parent := message("1.000001", "")
	reply := message("1.000002", "retained reply")
	reply.ThreadParentID = parent.NativeMessageID
	frame := unit(0, "000", []acquisitionslack.NativeMessage{parent, reply}, map[string]string{
		parent.NativeMessageID: "non_user", reply.NativeMessageID: "user",
	})
	if _, err := (Controller{Repository: repository, Ledger: ledgerStore}).Apply(testEnvelope([]UnitFrame{frame})); err == nil {
		t.Fatal("reply with a structurally excluded parent was accepted")
	}
	status, err := repository.Status()
	if err != nil || status.RecordCount != 0 {
		t.Fatalf("excluded-parent thread gap mutated canonical evidence: %+v err=%v", status, err)
	}
	ledger, err := ledgerStore.Load()
	if err != nil || ledger.State != "incomplete" || ledger.GapCount != 1 {
		t.Fatalf("excluded-parent gap was not recorded truthfully: %+v err=%v", ledger, err)
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
	ledger, err := ledgerStore.Load()
	if err != nil || ledger.State != "incomplete" || !validLedger(ledger) ||
		ledger.OwnedCount != ledger.RetainedCount+ledger.WithheldCount+ledger.StructuralExcludedCount ||
		ledger.DeliveredCount != ledger.CanonicalDeclaredCount+ledger.StructuralExcludedCount {
		t.Fatalf("conflict did not persist a truthful incomplete ledger: %+v err=%v", ledger, err)
	}
}

func TestApplyRejectsConcurrentRunBeforeCanonicalMutation(t *testing.T) {
	repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := ledgerStore.AcquireApplyLock()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	envelope := testEnvelope([]UnitFrame{unit(0, "000", []acquisitionslack.NativeMessage{message("1.000001", "saved")}, map[string]string{"1.000001": "user"})})
	if _, err := (Controller{Repository: repository, Ledger: ledgerStore}).Apply(envelope); err == nil {
		t.Fatal("concurrent ingestion apply was accepted")
	}
	status, err := repository.Status()
	if err != nil || status.RecordCount != 0 {
		t.Fatalf("rejected concurrent run mutated canonical state: %+v err=%v", status, err)
	}
}

func TestApplyRejectsOriginalAfterEditedOverlapBeforeImport(t *testing.T) {
	repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	original := message("1.000001", "original")
	edited := message("1.000001", "edited")
	edited.EditDeleteState = "edited"
	reverted := message("1.000001", "original again")
	units := []UnitFrame{
		unit(0, "000", []acquisitionslack.NativeMessage{original}, map[string]string{"1.000001": "user"}),
		unit(1, "001", []acquisitionslack.NativeMessage{edited}, map[string]string{"1.000001": "user"}),
		unit(2, "002", []acquisitionslack.NativeMessage{reverted}, map[string]string{"1.000001": "user"}),
	}
	if _, err := (Controller{Repository: repository, Ledger: ledgerStore}).Apply(testEnvelope(units)); err == nil {
		t.Fatal("backward original-after-edit transition was accepted")
	}
	status, err := repository.Status()
	if err != nil || status.RecordCount != 0 {
		t.Fatalf("rejected transition mutated canonical state: %+v err=%v", status, err)
	}
}

func TestApplyRejectsEditedContentAfterDeletedOverlapBeforeImport(t *testing.T) {
	for _, state := range []string{"deleted", "tombstone"} {
		t.Run(state, func(t *testing.T) {
			repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
			if err != nil {
				t.Fatal(err)
			}
			ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
			if err != nil {
				t.Fatal(err)
			}
			deleted := message("1.000001", state)
			deleted.EditDeleteState = state
			staleEdit := message("1.000001", "stale edited content")
			staleEdit.EditDeleteState = "edited"
			units := []UnitFrame{
				unit(0, "000", []acquisitionslack.NativeMessage{deleted}, map[string]string{"1.000001": "user"}),
				unit(1, "001", []acquisitionslack.NativeMessage{staleEdit}, map[string]string{"1.000001": "user"}),
			}
			if _, err := (Controller{Repository: repository, Ledger: ledgerStore}).Apply(testEnvelope(units)); err == nil {
				t.Fatal("edited content after a deleted state was accepted")
			}
			status, err := repository.Status()
			if err != nil || status.RecordCount != 0 {
				t.Fatalf("rejected resurrection mutated canonical state: %+v err=%v", status, err)
			}
		})
	}
}

func TestApplyKeepsLatestAcceptedEditCurrent(t *testing.T) {
	repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	original := message("1.000001", "original")
	edited := message("1.000001", "latest edit")
	edited.EditDeleteState = "edited"
	units := []UnitFrame{
		unit(0, "000", []acquisitionslack.NativeMessage{original}, map[string]string{"1.000001": "user"}),
		unit(1, "001", []acquisitionslack.NativeMessage{edited}, map[string]string{"1.000001": "user"}),
	}
	if _, err := (Controller{Repository: repository, Ledger: ledgerStore}).Apply(testEnvelope(units)); err != nil {
		t.Fatal(err)
	}
	library, err := repository.Load()
	if err != nil || len(library.Records) != 1 || library.Records[0].RawText != "latest edit" || len(library.Revisions) != 1 {
		t.Fatalf("latest accepted edit was not canonical: records=%+v revisions=%+v err=%v", library.Records, library.Revisions, err)
	}
}

func TestApplyReassertsExistingFinalEditAfterNewEarlierOccurrence(t *testing.T) {
	repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	controller := Controller{Repository: repository, Ledger: ledgerStore}
	edited := message("1.000001", "latest edit")
	edited.EditDeleteState = "edited"
	editedUnit := unit(0, "000", []acquisitionslack.NativeMessage{edited}, map[string]string{"1.000001": "user"})
	if _, err := controller.Apply(testEnvelope([]UnitFrame{editedUnit})); err != nil {
		t.Fatal(err)
	}
	original := message("1.000001", "original")
	orderedEdited := unit(1, "001", []acquisitionslack.NativeMessage{edited}, map[string]string{"1.000001": "user"})
	envelope := testEnvelope([]UnitFrame{
		unit(0, "000", []acquisitionslack.NativeMessage{original}, map[string]string{"1.000001": "user"}),
		orderedEdited,
	})
	if _, err := controller.Apply(envelope); err != nil {
		t.Fatal(err)
	}
	library, err := repository.Load()
	if err != nil || len(library.Records) != 1 || library.Records[0].RawText != "latest edit" {
		t.Fatalf("existing final edit was not reasserted: records=%+v err=%v", library.Records, err)
	}
	for _, revision := range library.Revisions {
		if revision.Record.ContentHash == library.Records[0].ContentHash {
			t.Fatalf("current edit was duplicated as historical revision: %+v", revision)
		}
	}
	before, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Apply(envelope); err != nil {
		t.Fatal(err)
	}
	after, err := repository.Status()
	if err != nil || before != after {
		t.Fatalf("exact envelope replay changed canonical state: before=%+v after=%+v err=%v", before, after, err)
	}
}

func TestApplyRejectsImportedFinalEditWhenNewerCurrentEditExists(t *testing.T) {
	repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	controller := Controller{Repository: repository, Ledger: ledgerStore}
	editB := message("1.000001", "edit B")
	editB.EditDeleteState = "edited"
	editB.RevisionTimestamp = "1.000002"
	if _, err := controller.Apply(testEnvelope([]UnitFrame{unit(0, "000", []acquisitionslack.NativeMessage{editB}, map[string]string{"1.000001": "user"})})); err != nil {
		ledger, _ := ledgerStore.Load()
		t.Fatalf("initial edited capture failed: %v ledger=%+v", err, ledger)
	}
	editC := message("1.000001", "newer edit C")
	editC.EditDeleteState = "edited"
	editC.RevisionTimestamp = "1.000003"
	if _, err := controller.Apply(testEnvelope([]UnitFrame{unit(0, "000", []acquisitionslack.NativeMessage{editC}, map[string]string{"1.000001": "user"})})); err != nil {
		t.Fatal(err)
	}
	before, err := repository.Status()
	if err != nil {
		t.Fatal(err)
	}
	original := message("1.000001", "newly observed original A")
	envelope := testEnvelope([]UnitFrame{
		unit(0, "000", []acquisitionslack.NativeMessage{original}, map[string]string{"1.000001": "user"}),
		unit(1, "001", []acquisitionslack.NativeMessage{editB}, map[string]string{"1.000001": "user"}),
	})
	if _, err := controller.Apply(envelope); err == nil {
		t.Fatal("stale imported final edit was accepted over a newer current edit")
	}
	after, err := repository.Status()
	library, loadErr := repository.Load()
	if err != nil || loadErr != nil || before != after || len(library.Records) != 1 || library.Records[0].RawText != "newer edit C" {
		t.Fatalf("rejected stale envelope changed current evidence: before=%+v after=%+v records=%+v err=%v load=%v", before, after, library.Records, err, loadErr)
	}
}

func TestApplyRejectsOlderEditLaterInSameEnvelope(t *testing.T) {
	repository, err := personalmemory.NewFileRepository(filepath.Join(t.TempDir(), "library"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := NewLedgerStore(filepath.Join(t.TempDir(), "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	newer := message("1.000001", "newer edit")
	newer.EditDeleteState = "edited"
	newer.RevisionTimestamp = "1.000003"
	older := message("1.000001", "unseen older edit")
	older.EditDeleteState = "edited"
	older.RevisionTimestamp = "1.000002"
	envelope := testEnvelope([]UnitFrame{
		unit(0, "000", []acquisitionslack.NativeMessage{newer}, map[string]string{"1.000001": "user"}),
		unit(1, "001", []acquisitionslack.NativeMessage{older}, map[string]string{"1.000001": "user"}),
	})
	if _, err := (Controller{Repository: repository, Ledger: ledgerStore}).Apply(envelope); err == nil {
		t.Fatal("an older overlapping edit was accepted as the envelope final state")
	}
	status, err := repository.Status()
	if err != nil || status.RecordCount != 0 {
		t.Fatalf("rejected overlapping edit changed canonical evidence: %+v err=%v", status, err)
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
