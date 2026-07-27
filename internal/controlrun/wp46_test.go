package controlrun

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

type incrementingEntropy struct {
	mu   sync.Mutex
	next byte
}

func (entropy *incrementingEntropy) Read(buffer []byte) (int, error) {
	entropy.mu.Lock()
	defer entropy.mu.Unlock()
	for index := range buffer {
		entropy.next++
		buffer[index] = entropy.next
	}
	return len(buffer), nil
}

func TestWP46_ControlRunIDsAreExplicitReservedAndNeverLatest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stable-control")
	entropy := &incrementingEntropy{}
	firstID, err := NewRunID(time.Date(2026, 7, 15, 14, 30, 0, 0, time.UTC), entropy)
	if err != nil || ValidateRunID(firstID) != nil {
		t.Fatalf("first run id = %q, %v", firstID, err)
	}
	secondID, err := NewRunID(time.Date(2026, 7, 16, 14, 30, 0, 0, time.UTC), entropy)
	if err != nil {
		t.Fatal(err)
	}
	firstPath, err := ReserveRun(root, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReserveRun(root, firstID); !errors.Is(err, os.ErrExist) {
		t.Fatalf("duplicate run reservation = %v", err)
	}
	if _, err := ReserveRun(root, secondID); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != privateio.DirMode {
		t.Fatalf("unsafe run directory: %v", info)
	}
	repository, err := NewRepository(root, Options{Entropy: entropy})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := repository.Load(context.Background())
	if err != nil || empty.State != StateNone || empty.Document.Version != 0 {
		t.Fatalf("missing selection = %+v, %v", empty, err)
	}
	selected, err := repository.CompareAndSwap(context.Background(), empty.Document.Revision(), firstID)
	if err != nil || selected.State != StateSelected || selected.Document.SelectedRunID != firstID {
		t.Fatalf("explicit selection = %+v, %v", selected, err)
	}
	reloadedRepository, _ := NewRepository(root, Options{Entropy: &incrementingEntropy{next: 200}})
	reloaded, err := reloadedRepository.Load(context.Background())
	if err != nil || reloaded.Document.SelectedRunID != firstID || reloaded.Document.SelectedRunID == secondID {
		t.Fatalf("selection inferred latest: %+v, %v", reloaded, err)
	}
	cleared, err := repository.CompareAndSwap(context.Background(), selected.Document.Revision(), "")
	if err != nil || cleared.State != StateNone || cleared.Document.Version != selected.Document.Version+1 || cleared.Document.Generation == selected.Document.Generation {
		t.Fatalf("explicit clear = %+v, %v", cleared, err)
	}
}

func TestWP46_ControlRunRecoveryIsExplicitPointerOnlyAndABASafe(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stable-control")
	entropy := &incrementingEntropy{}
	firstID, _ := NewRunID(time.Date(2026, 7, 15, 14, 30, 0, 0, time.UTC), entropy)
	secondID, _ := NewRunID(time.Date(2026, 7, 16, 14, 30, 0, 0, time.UTC), entropy)
	firstPath, _ := ReserveRun(root, firstID)
	secondPath, _ := ReserveRun(root, secondID)
	marker := filepath.Join(firstPath, "immutable-marker.json")
	if err := os.WriteFile(marker, []byte("{}\n"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	repository, _ := NewRepository(root, Options{Entropy: entropy})
	empty, _ := repository.Load(context.Background())
	first, err := repository.CompareAndSwap(context.Background(), empty.Document.Revision(), firstID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.CompareAndSwap(context.Background(), first.Document.Revision(), secondID)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := second.Document
	corrupt.Fingerprint = "sha256:invalid"
	raw, _ := json.Marshal(corrupt)
	if err := os.WriteFile(filepath.Join(root, "control", "selected-run.json"), append(raw, '\n'), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	blocked, err := repository.Load(context.Background())
	if err != nil || blocked.State != StateRecoveryRequired || blocked.Problem == nil || blocked.Problem.ReadableRevision == nil || !blocked.Problem.BackupAvailable {
		t.Fatalf("selection corruption was not explicit: %+v, %v", blocked, err)
	}
	wrong := first.Document.Revision()
	if _, err := repository.Recover(context.Background(), blocked.Problem.Fingerprint, &wrong, RecoveryAcknowledgement, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("wrong readable revision = %v", err)
	}
	expected := second.Document.Revision()
	recovered, err := repository.Recover(context.Background(), blocked.Problem.Fingerprint, &expected, RecoveryAcknowledgement, "")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != StateNone || recovered.Document.Version != second.Document.Version || recovered.Document.Generation == second.Document.Generation {
		t.Fatalf("ABA-safe clear recovery = %+v", recovered)
	}
	if _, err := repository.CompareAndSwap(context.Background(), second.Document.Revision(), firstID); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale selection CAS = %v", err)
	}
	if _, err := os.Lstat(marker); err != nil {
		t.Fatal("selection recovery modified prior run evidence")
	}
	if info, err := os.Lstat(secondPath); err != nil || !info.IsDir() {
		t.Fatal("selection recovery modified alternate run evidence")
	}
}

func TestWP46_ControlRunStrictValidationRejectsDuplicatesAndPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stable-control")
	repository, _ := NewRepository(root, Options{Entropy: &incrementingEntropy{}})
	empty, _ := repository.Load(context.Background())
	if _, err := repository.CompareAndSwap(context.Background(), empty.Document.Revision(), "../../latest"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("path-like run id = %v", err)
	}
	if _, err := repository.CompareAndSwap(context.Background(), empty.Document.Revision(), "run-20260715T143000Z-aaaaaaaaaaaaaaaaaaaaaaaaaa"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("unreserved run id = %v", err)
	}
	if _, err := repository.CompareAndSwap(context.Background(), empty.Document.Revision(), ""); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "control", "selected-run.json")
	raw, _ := os.ReadFile(path)
	var document Document
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	duplicate := `{"schema_version":"` + document.SchemaVersion + `","version":1,"version":1,"generation":"` + document.Generation + `","selected_run_id":"","fingerprint":"` + document.Fingerprint + `"}`
	if err := os.WriteFile(path, []byte(duplicate), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	blocked, err := repository.Load(context.Background())
	if err != nil || blocked.State != StateRecoveryRequired {
		t.Fatalf("duplicate selection accepted: %+v, %v", blocked, err)
	}
}
