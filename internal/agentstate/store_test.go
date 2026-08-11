package agentstate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

func TestStorePersistsUnlimitedLensesAndReversibleWeightedFeedback(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	store, err := Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for index := 0; index < 128; index++ {
		id := "lens-" + time.Unix(int64(index), 0).UTC().Format("150405")
		if _, err := store.PutLens(ctx, Lens{ID: id, Name: id, Query: "product evidence"}); err != nil {
			t.Fatal(err)
		}
	}
	lenses, err := store.ListLenses(ctx)
	if err != nil || len(lenses) != 128 {
		t.Fatalf("lenses=%d err=%v", len(lenses), err)
	}
	if err := store.SaveRetrieval(ctx, RetrievalTrace{
		RunID: "run-1", Query: "product", LensID: lenses[0].ID,
		RetrievalMethod: "test", LibraryFingerprint: "fingerprint",
		CreatedAt: now.Format(time.RFC3339),
		Candidates: []CandidateTrace{{
			RecordID: "record-1", Rank: 1, FinalScore: 1,
			ComponentScore: map[string]float64{"final": 1},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	agent, err := store.ApplyJudgment(ctx, JudgmentRequest{
		IdempotencyKey: "agent-used", RunID: "run-1", LensID: lenses[0].ID,
		RecordID: "record-1", Actor: "agent", Disposition: "used",
	})
	if err != nil || agent.Effect != 0.25 {
		t.Fatalf("agent judgment=%+v err=%v", agent, err)
	}
	user, err := store.ApplyJudgment(ctx, JudgmentRequest{
		IdempotencyKey: "user-dismissed", RunID: "run-1", LensID: lenses[0].ID,
		RecordID: "record-1", Actor: "user", Disposition: "dismissed",
	})
	if err != nil || user.Effect != -1 {
		t.Fatalf("user judgment=%+v err=%v", user, err)
	}
	relevance, err := store.Relevance(ctx, lenses[0].ID, []string{"record-1"})
	if err != nil || math.Abs(relevance["record-1"]-(-0.075)) > 1e-12 {
		t.Fatalf("relevance=%v err=%v", relevance, err)
	}
	replay, err := store.ApplyJudgment(ctx, JudgmentRequest{
		IdempotencyKey: "user-dismissed", RunID: "run-1", LensID: lenses[0].ID,
		RecordID: "record-1", Actor: "user", Disposition: "dismissed",
	})
	if err != nil || !replay.Replayed || replay.JudgmentID != user.JudgmentID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	if _, err := store.ApplyJudgment(ctx, JudgmentRequest{
		IdempotencyKey: "user-dismissed", RunID: "run-1", LensID: lenses[0].ID,
		RecordID: "record-1", Actor: "agent", Disposition: "used",
	}); err == nil {
		t.Fatal("conflicting idempotency payload was accepted")
	}
	reversal, err := store.ApplyJudgment(ctx, JudgmentRequest{
		IdempotencyKey: "reverse-user", Actor: "user", ReversesID: user.JudgmentID,
	})
	if err != nil || reversal.Effect != 1 {
		t.Fatalf("reversal=%+v err=%v", reversal, err)
	}
	relevance, err = store.Relevance(ctx, lenses[0].ID, []string{"record-1"})
	if err != nil || math.Abs(relevance["record-1"]-0.025) > 1e-12 {
		t.Fatalf("reversed relevance=%v err=%v", relevance, err)
	}
}

func TestLegacyAgentStateRejectsCredentialShapedDurableText(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "state", "agent.sqlite"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	secret := "Bearer synthetic-private-token"
	if _, err := store.PutLens(ctx, Lens{ID: "secret-lens", Name: "Secret lens", Query: secret}); err == nil {
		t.Fatal("credential-shaped lens text was accepted")
	}
	if err := store.SaveRetrieval(ctx, RetrievalTrace{
		RunID: "secret-run", Query: secret, LensID: "missing",
		RetrievalMethod: "test", LibraryFingerprint: "fingerprint", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}); err == nil {
		t.Fatal("credential-shaped retrieval query was accepted")
	}
	credentialID := "pb_sk_synthetic-private-value"
	if _, err := store.PutLens(ctx, Lens{ID: credentialID, Name: "Secret ID", Query: "query"}); err == nil {
		t.Fatal("credential-shaped lens ID was accepted")
	}
	if err := store.SaveRetrieval(ctx, RetrievalTrace{
		RunID: credentialID, Query: "query", LensID: "missing",
		RetrievalMethod: "test", LibraryFingerprint: "fingerprint", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}); err == nil {
		t.Fatal("credential-shaped retrieval ID was accepted")
	}
	var lensCount, runCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lenses WHERE id='secret-lens'`).Scan(&lensCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM retrieval_runs WHERE run_id='secret-run'`).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if lensCount != 0 || runCount != 0 {
		t.Fatalf("rejected credential text reached durable state: lenses=%d runs=%d", lensCount, runCount)
	}
	if snapshot, present, err := readRecoverySnapshot(store.path); err != nil || !present {
		t.Fatalf("read recovery snapshot: present=%v err=%v", present, err)
	} else if data, err := json.Marshal(snapshot); err != nil || bytes.Contains(data, []byte(credentialID)) {
		t.Fatalf("credential-shaped ID reached recovery snapshot: %v", err)
	}
}

func TestOpenRecoveringQuarantinesCorruptDatabase(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "state")
	if err := privateio.PrepareDir(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "agent.sqlite")
	if err := os.WriteFile(path, []byte("not sqlite"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	if err := privateio.WriteJSON(recoveryPath(path), recoverySnapshot{
		SchemaVersion: recoverySchemaVersion,
		Lenses:        []Lens{},
		Judgments:     []Judgment{},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 13, 0, 0, 0, time.UTC)
	store, quarantine, err := OpenRecovering(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if quarantine == "" {
		t.Fatal("expected quarantine path")
	}
	if _, err := os.Stat(quarantine); err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != privateio.FileMode {
		t.Fatalf("rebuilt state mode=%v err=%v", info.Mode().Perm(), err)
	}
	if adopted, err := readProjectConnectionAdoptionMarker(path); err != nil || !adopted {
		t.Fatalf("legacy recovery did not adopt project connections: adopted=%v err=%v", adopted, err)
	}
}

func TestOpenRecoveringRestoresLensesAndJudgmentsFromPrivateSnapshot(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 26, 14, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	lens, err := store.PutLens(ctx, Lens{ID: "product", Name: "Product", Query: "product evidence"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRetrieval(ctx, RetrievalTrace{
		RunID: "run", Query: "product", LensID: lens.ID,
		RetrievalMethod: "test", LibraryFingerprint: "fingerprint",
		CreatedAt: now.Format(time.RFC3339),
		Candidates: []CandidateTrace{{
			RecordID: "record", Rank: 1, FinalScore: 1,
			ComponentScore: map[string]float64{"final": 1},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	request := JudgmentRequest{
		IdempotencyKey: "restore-judgment", RunID: "run", LensID: lens.ID,
		RecordID: "record", Actor: "agent", Disposition: "used",
	}
	original, err := store.ApplyJudgment(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	recovered, quarantine, err := OpenRecovering(path, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if quarantine == "" {
		t.Fatal("expected corrupt database quarantine")
	}
	lenses, err := recovered.ListLenses(ctx)
	if err != nil || len(lenses) != 1 || lenses[0].ID != lens.ID {
		t.Fatalf("recovered lenses=%+v err=%v", lenses, err)
	}
	relevance, err := recovered.Relevance(ctx, lens.ID, []string{"record"})
	if err != nil || relevance["record"] != 0.025 {
		t.Fatalf("recovered relevance=%v err=%v", relevance, err)
	}
	replay, err := recovered.ApplyJudgment(ctx, request)
	if err != nil || !replay.Replayed || replay.JudgmentID != original.JudgmentID {
		t.Fatalf("recovered replay=%+v err=%v", replay, err)
	}
	if _, err := recovered.ApplyJudgment(ctx, JudgmentRequest{
		IdempotencyKey: "restore-reversal", Actor: "user", ReversesID: original.JudgmentID,
	}); err != nil {
		t.Fatalf("reverse recovered judgment: %v", err)
	}
}

func TestOpenRecoveringResumesAfterPartialQuarantine(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 26, 15, 0, 0, 0, time.UTC)
	path, lens, original := seedRecoverableState(t, now)
	renameCount := 0
	_, _, err := openRecovering(path, func() time.Time { return now.Add(time.Second) }, recoveryHooks{
		rename: func(source, destination string) error {
			renameCount++
			if err := os.Rename(source, destination); err != nil {
				return err
			}
			if renameCount == 1 {
				return errors.New("injected failure after quarantine rename")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("partial quarantine unexpectedly succeeded")
	}
	if _, err := Open(path, func() time.Time { return now }); !errors.Is(err, ErrRecoveryInProgress) {
		t.Fatalf("ordinary open during recovery error=%v", err)
	}
	recovered, quarantine, err := OpenRecovering(path, func() time.Time { return now.Add(2 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if renameCount != 1 || quarantine == "" {
		t.Fatalf("rename_count=%d quarantine=%q", renameCount, quarantine)
	}
	assertRecoveredUserState(t, recovered, lens, original)
}

func TestOpenRecoveringQuarantinesSidecarsAfterInterruptedMainRename(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 26, 15, 30, 0, 0, time.UTC)
	path, lens, original := seedRecoverableState(t, now)
	renameCount := 0
	_, quarantine, err := openRecovering(path, func() time.Time { return now.Add(time.Second) }, recoveryHooks{
		rename: func(source, destination string) error {
			renameCount++
			if err := os.Rename(source, destination); err != nil {
				return err
			}
			if renameCount == 1 {
				for _, suffix := range []string{"-wal", "-shm"} {
					if err := os.WriteFile(path+suffix, []byte("interrupted sidecar"), privateio.FileMode); err != nil {
						return err
					}
				}
				return errors.New("injected failure after main quarantine rename")
			}
			return nil
		},
	})
	if err == nil || quarantine != "" {
		t.Fatalf("interrupted quarantine result=%q err=%v", quarantine, err)
	}
	recovered, quarantine, err := OpenRecovering(path, func() time.Time { return now.Add(2 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	for _, suffix := range []string{"-wal", "-shm"} {
		if content, err := os.ReadFile(path + suffix); err == nil &&
			string(content) == "interrupted sidecar" {
			t.Fatalf("stale canonical sidecar %s remained after recovery", suffix)
		} else if err != nil && !os.IsNotExist(err) {
			t.Fatalf("inspect canonical sidecar %s: %v", suffix, err)
		}
		if info, err := os.Stat(quarantine + suffix); err != nil ||
			!info.Mode().IsRegular() || info.Mode().Perm() != privateio.FileMode {
			t.Fatalf("quarantined sidecar %s info=%v err=%v", suffix, info, err)
		}
	}
	assertRecoveredUserState(t, recovered, lens, original)
}

func TestOpenRecoveringResumesAfterRestoreFailure(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 26, 16, 0, 0, 0, time.UTC)
	path, lens, original := seedRecoverableState(t, now)
	_, _, err := openRecovering(path, func() time.Time { return now.Add(time.Second) }, recoveryHooks{
		rename: os.Rename,
		beforeRestore: func() error {
			return errors.New("injected restore failure")
		},
	})
	if err == nil {
		t.Fatal("injected restore failure unexpectedly succeeded")
	}
	if _, err := Open(path, func() time.Time { return now }); !errors.Is(err, ErrRecoveryInProgress) {
		t.Fatalf("ordinary open during recovery error=%v", err)
	}
	recovered, quarantine, err := OpenRecovering(path, func() time.Time { return now.Add(2 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if quarantine == "" {
		t.Fatal("expected corrupt database quarantine")
	}
	assertRecoveredUserState(t, recovered, lens, original)
}

func TestOpenRecoveringFinalizesAnAlreadyPromotedRecovery(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 26, 17, 0, 0, 0, time.UTC)
	path, lens, original := seedRecoverableState(t, now)
	renameCount := 0
	_, _, err := openRecovering(path, func() time.Time { return now.Add(time.Second) }, recoveryHooks{
		rename: func(source, destination string) error {
			renameCount++
			if err := os.Rename(source, destination); err != nil {
				return err
			}
			if renameCount == 2 {
				return errors.New("injected failure after stage promotion")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("interrupted promotion unexpectedly succeeded")
	}
	if _, err := Open(path, func() time.Time { return now }); !errors.Is(err, ErrRecoveryInProgress) {
		t.Fatalf("ordinary open during recovery error=%v", err)
	}
	recovered, quarantine, err := OpenRecovering(path, func() time.Time { return now.Add(2 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if renameCount != 2 || quarantine == "" {
		t.Fatalf("rename_count=%d quarantine=%q", renameCount, quarantine)
	}
	assertRecoveredUserState(t, recovered, lens, original)
}

func seedRecoverableState(t *testing.T, now time.Time) (string, Lens, Judgment) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state", "agent.sqlite")
	store, err := Open(path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	lens, err := store.PutLens(ctx, Lens{ID: "product", Name: "Product", Query: "product evidence"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRetrieval(ctx, RetrievalTrace{
		RunID: "run", Query: "product", LensID: lens.ID,
		RetrievalMethod: "test", LibraryFingerprint: "fingerprint",
		CreatedAt: now.Format(time.RFC3339),
		Candidates: []CandidateTrace{{
			RecordID: "record", Rank: 1, FinalScore: 1,
			ComponentScore: map[string]float64{"final": 1},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	original, err := store.ApplyJudgment(ctx, JudgmentRequest{
		IdempotencyKey: "restore-judgment", RunID: "run", LensID: lens.ID,
		RecordID: "record", Actor: "agent", Disposition: "used",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	return path, lens, original
}

func assertRecoveredUserState(t *testing.T, store *Store, lens Lens, original Judgment) {
	t.Helper()
	ctx := context.Background()
	lenses, err := store.ListLenses(ctx)
	if err != nil || len(lenses) != 1 || lenses[0].ID != lens.ID {
		t.Fatalf("recovered lenses=%+v err=%v", lenses, err)
	}
	relevance, err := store.Relevance(ctx, lens.ID, []string{"record"})
	if err != nil || relevance["record"] != 0.025 {
		t.Fatalf("recovered relevance=%v err=%v", relevance, err)
	}
	replay, err := store.ApplyJudgment(ctx, JudgmentRequest{
		IdempotencyKey: "restore-judgment", RunID: "run", LensID: lens.ID,
		RecordID: "record", Actor: "agent", Disposition: "used",
	})
	if err != nil || !replay.Replayed || replay.JudgmentID != original.JudgmentID {
		t.Fatalf("recovered replay=%+v err=%v", replay, err)
	}
}
