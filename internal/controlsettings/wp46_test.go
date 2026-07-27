package controlsettings

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func wp46Validators() map[string]AdapterValidator {
	return map[string]AdapterValidator{
		"slack_web_api": AdapterValidatorFunc(func(schema string, raw json.RawMessage) (json.RawMessage, error) {
			if schema != "mindline.source.slack-web-api-defaults/v1" {
				return nil, ErrInvalid
			}
			var values struct {
				ChannelID string `json:"channel_id"`
			}
			if err := privateio.DecodeJSONStrict(raw, &values); err != nil || len(values.ChannelID) < 2 || len(values.ChannelID) > 64 {
				return nil, ErrInvalid
			}
			return json.Marshal(values)
		}),
	}
}

func TestWP46_ControlSettingsDefaultsPersistAndReloadAfterThirtyDays(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stable-control")
	now := time.Date(2026, 7, 15, 14, 30, 0, 0, time.UTC)
	repository, err := NewRepository(root, Options{Now: func() time.Time { return now }, Entropy: &incrementingEntropy{}, Validators: wp46Validators()})
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := repository.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if defaults.State != StateDefaults || defaults.Document.Version != 0 || len(defaults.Document.Draft.ContextLenses) != 3 {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}
	if defaults.Document.Draft.ContextLenses[2] != DefaultContentNarrativeLens || defaults.Document.Draft.RoutingPolicy != DefaultRoutingPolicy {
		t.Fatal("server defaults do not reproduce the signed strategy")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("loading missing defaults wrote durable state")
	}
	saved, err := repository.Save(context.Background(), defaults.Document.Revision(), defaults.Document.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if saved.State != StateSaved || saved.Document.Version != 1 || saved.Document.SavedAt != "2026-07-15T14:30:00Z" || saved.Document.Generation == defaults.Document.Generation {
		t.Fatalf("unexpected first save: %+v", saved)
	}
	for path, mode := range map[string]os.FileMode{
		root:                           privateio.DirMode,
		filepath.Join(root, "control"): privateio.DirMode,
		filepath.Join(root, "control", "settings.json"): privateio.FileMode,
		filepath.Join(root, "control", "settings.lock"): privateio.FileMode,
	} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("mode %s = %v", path, info.Mode())
		}
	}
	now = now.Add(30 * 24 * time.Hour)
	reloadedRepository, err := NewRepository(root, Options{Now: func() time.Time { return now }, Entropy: &incrementingEntropy{next: 99}, Validators: wp46Validators()})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedRepository.Load(context.Background())
	if err != nil || !reflect.DeepEqual(reloaded.Document, saved.Document) {
		t.Fatalf("thirty-day reload = %+v, %v", reloaded, err)
	}
}

func TestWP46_ControlSettingsRecoveryRotatesGenerationAndPreventsABA(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stable-control")
	repository, err := NewRepository(root, Options{Now: func() time.Time { return time.Date(2026, 7, 15, 14, 30, 0, 0, time.UTC) }, Entropy: &incrementingEntropy{}, Validators: wp46Validators()})
	if err != nil {
		t.Fatal(err)
	}
	defaults, _ := repository.Load(context.Background())
	first, err := repository.Save(context.Background(), defaults.Document.Revision(), defaults.Document.Draft)
	if err != nil {
		t.Fatal(err)
	}
	secondDraft := first.Document.Draft
	secondDraft.RoutingPolicy += " Additional reviewed rule."
	second, err := repository.Save(context.Background(), first.Document.Revision(), secondDraft)
	if err != nil {
		t.Fatal(err)
	}
	secretSentinel := "pb" + "_sk_this_must_never_be_reflected"
	if err := os.WriteFile(filepath.Join(root, "control", "settings.json"), []byte(`{"schema_version":"mindline.control-settings/v1","version":2,"generation":"`+secretSentinel+`"}`), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	blocked, err := repository.Load(context.Background())
	if err != nil || blocked.State != StateRecoveryRequired || blocked.Problem == nil || !blocked.Problem.BackupAvailable {
		t.Fatalf("corruption was not explicit: %+v, %v", blocked, err)
	}
	projection, _ := json.Marshal(blocked)
	if strings.Contains(string(projection), secretSentinel) {
		t.Fatal("recovery projection reflected secret-shaped input")
	}
	if _, err := repository.Recover(context.Background(), blocked.Problem.Fingerprint, "wrong", RecoveryRestoreBackup); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong acknowledgement = %v", err)
	}
	recovered, err := repository.Recover(context.Background(), blocked.Problem.Fingerprint, RecoveryAcknowledgement, RecoveryRestoreBackup)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Document.Version != second.Document.Version || recovered.Document.Generation == second.Document.Generation || recovered.Document.Draft.RoutingPolicy != first.Document.Draft.RoutingPolicy {
		t.Fatalf("ABA-safe recovery failed: recovered=%+v prior=%+v", recovered.Document, second.Document)
	}
	if _, err := repository.Save(context.Background(), second.Document.Revision(), secondDraft); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale pre-recovery CAS = %v", err)
	}
}

func TestWP46_ControlSettingsStrictSecretAndDuplicateRejection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stable-control")
	repository, err := NewRepository(root, Options{Entropy: &incrementingEntropy{}, Validators: wp46Validators()})
	if err != nil {
		t.Fatal(err)
	}
	defaults, _ := repository.Load(context.Background())
	secretDraft := defaults.Document.Draft
	secretSentinel := "pb" + "_sk_value_that_must_not_escape"
	secretDraft.RoutingPolicy = secretSentinel
	if _, err := repository.Save(context.Background(), defaults.Document.Revision(), secretDraft); !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), secretSentinel) {
		t.Fatalf("secret validation error = %v", err)
	}
	saved, err := repository.Save(context.Background(), defaults.Document.Revision(), defaults.Document.Draft)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "control", "settings.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1)
	if err := os.WriteFile(path, []byte(duplicate), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	blocked, err := repository.Load(context.Background())
	if err != nil || blocked.State != StateRecoveryRequired {
		t.Fatalf("duplicate-key document accepted after revision %+v: %+v, %v", saved.Document.Revision(), blocked, err)
	}
}

func TestWP46_ControlSettingsLensStorageIsOrderedAndNotEightColumns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stable-control")
	repository, _ := NewRepository(root, Options{Entropy: &incrementingEntropy{}, Validators: wp46Validators()})
	defaults, _ := repository.Load(context.Background())
	draft := defaults.Document.Draft
	draft.ContextLenses = []string{
		"Lens 01", "Lens 02", "Lens 03", "Lens 04", "Lens 05", "Lens 06",
		"Lens 07", "Lens 08", "Lens 09", "Lens 10", "Lens 11", "Lens 12",
	}
	saved, err := repository.Save(context.Background(), defaults.Document.Revision(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Document.Draft.ContextLenses) != 12 || saved.Document.Draft.ContextLenses[8] != "Lens 09" || saved.Document.Draft.ContextLenses[11] != "Lens 12" {
		t.Fatalf("ordered lens list was constrained or reordered: %#v", saved.Document.Draft.ContextLenses)
	}
}

func TestWP46_ControlSettingsConcurrentCASHasOneWinner(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stable-control")
	firstRepository, _ := NewRepository(root, Options{Entropy: &incrementingEntropy{}, Validators: wp46Validators()})
	defaults, _ := firstRepository.Load(context.Background())
	current, err := firstRepository.Save(context.Background(), defaults.Document.Revision(), defaults.Document.Draft)
	if err != nil {
		t.Fatal(err)
	}
	secondRepository, _ := NewRepository(root, Options{Entropy: &incrementingEntropy{next: 101}, Validators: wp46Validators()})
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, repository := range []*Repository{firstRepository, secondRepository} {
		go func(repository *Repository) {
			<-start
			_, err := repository.Save(context.Background(), current.Document.Revision(), current.Document.Draft)
			results <- err
		}(repository)
	}
	close(start)
	successes := 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrConflict) && !errors.Is(err, ErrLockBusy) {
			t.Fatalf("unexpected loser error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("CAS successes = %d, want 1", successes)
	}
}
