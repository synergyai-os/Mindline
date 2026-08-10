package founderreview

import (
	"bytes"
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

const (
	testProofRunID              = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testProofFingerprint        = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testCitedRecordsFingerprint = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testRetryToken              = "founder-review-retry-token-0001"
)

func testRequest(verdict Verdict) Request {
	return Request{
		ProofRunID:                 testProofRunID,
		StructuralProofFingerprint: testProofFingerprint,
		CitedRecordsFingerprint:    testCitedRecordsFingerprint,
		Verdict:                    verdict,
		RetryToken:                 testRetryToken,
	}
}

func TestCreateOwnerOnlyStructuralRecord(t *testing.T) {
	repository := testRepository(t, t.TempDir())
	record, err := repository.Create(context.Background(), testRequest(VerdictUseful))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if record.SchemaVersion != SchemaVersion || record.RunID == "" || record.ProofRunID != testProofRunID || record.StructuralProofFingerprint != testProofFingerprint || record.CitedRecordsFingerprint != testCitedRecordsFingerprint || record.EventID != eventID(testRequest(VerdictUseful)) || record.RecordedAt != "2026-07-27T10:11:12Z" {
		t.Fatalf("unexpected record: %+v", record)
	}
	if !record.ClosesUserValue() || record.Resolution() != ResolutionClosed {
		t.Fatalf("useful review must close user value: %+v", record)
	}
	path := filepath.Join(repository.Root(), "founder-review", "review.json")
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != privateio.FileMode {
		t.Fatalf("review must be owner-only regular file, info=%v err=%v", info, err)
	}
	loaded, err := repository.Load(context.Background())
	if err != nil || loaded != record {
		t.Fatalf("Load() = %+v, %v; want %+v, nil", loaded, err, record)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(testRetryToken)) {
		t.Fatalf("record persisted raw retry token: %s", raw)
	}
	receiptBytes, err := json.Marshal(record.Receipt())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(receiptBytes, []byte(record.RecordedAt)) || bytes.Contains(receiptBytes, []byte(record.RetryTokenHash)) || bytes.Contains(receiptBytes, []byte(record.RunID)) {
		t.Fatalf("receipt exposed private review state: %s", receiptBytes)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"schema_version": true, "run_id": true, "proof_run_id": true,
		"structural_proof_fingerprint": true, "cited_records_fingerprint": true,
		"verdict": true, "event_id": true, "retry_token_hash": true, "recorded_at": true,
	}
	if len(fields) != len(want) {
		t.Fatalf("record must contain only structural fields, got %s", raw)
	}
	for field := range fields {
		if !want[field] {
			t.Fatalf("record leaked disallowed field %q: %s", field, raw)
		}
	}
}

func TestResolutionNeverClosesForNegativeOrDeclined(t *testing.T) {
	tests := []struct {
		verdict    Verdict
		resolution Resolution
	}{
		{VerdictUseful, ResolutionClosed},
		{VerdictNotUseful, ResolutionReturnToDiagnoseShape},
		{VerdictDeclined, ResolutionUnverified},
	}
	for _, test := range tests {
		t.Run(string(test.verdict), func(t *testing.T) {
			if got := test.verdict.Resolution(); got != test.resolution {
				t.Fatalf("Resolution() = %q, want %q", got, test.resolution)
			}
			if got := test.verdict.ClosesUserValue(); got != (test.verdict == VerdictUseful) {
				t.Fatalf("ClosesUserValue() = %t", got)
			}
		})
	}
}

func TestCreateReplaysSameIntentAndRejectsChangedIntent(t *testing.T) {
	repository := testRepository(t, t.TempDir())
	request := testRequest(VerdictNotUseful)
	first, err := repository.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.Create(context.Background(), request)
	if err != nil || second != first {
		t.Fatalf("same intent Create() = %+v, %v; want %+v, nil", second, err, first)
	}
	changed := request
	changed.Verdict = VerdictUseful
	if _, err := repository.Create(context.Background(), changed); !errors.Is(err, ErrRetryConflict) {
		t.Fatalf("changed same-token intent error = %v, want ErrRetryConflict", err)
	}
	newToken := request
	newToken.RetryToken = "founder-review-retry-token-0002"
	if _, err := repository.Create(context.Background(), newToken); !errors.Is(err, ErrAlreadyRecorded) {
		t.Fatalf("new-token second review error = %v, want ErrAlreadyRecorded", err)
	}
}

func TestCreateRejectsInvalidRequest(t *testing.T) {
	repository := testRepository(t, t.TempDir())
	for _, request := range []Request{
		{ProofRunID: "run", StructuralProofFingerprint: testProofFingerprint, CitedRecordsFingerprint: testCitedRecordsFingerprint, Verdict: VerdictUseful, RetryToken: testRetryToken},
		{ProofRunID: testProofRunID, StructuralProofFingerprint: "proof", CitedRecordsFingerprint: testCitedRecordsFingerprint, Verdict: VerdictUseful, RetryToken: testRetryToken},
		{ProofRunID: testProofRunID, StructuralProofFingerprint: testProofFingerprint, CitedRecordsFingerprint: "citation", Verdict: VerdictUseful, RetryToken: testRetryToken},
		{ProofRunID: testProofRunID, StructuralProofFingerprint: testProofFingerprint, CitedRecordsFingerprint: testCitedRecordsFingerprint, Verdict: Verdict("maybe"), RetryToken: testRetryToken},
		{ProofRunID: testProofRunID, StructuralProofFingerprint: testProofFingerprint, CitedRecordsFingerprint: testCitedRecordsFingerprint, Verdict: VerdictUseful, RetryToken: "short"},
	} {
		if _, err := repository.Create(context.Background(), request); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Create(%+v) error = %v, want ErrInvalid", request, err)
		}
	}
}

func TestLoadRejectsUnknownOrUnsafeRecord(t *testing.T) {
	root := t.TempDir()
	repository := testRepository(t, root)
	if _, err := repository.Create(context.Background(), testRequest(VerdictDeclined)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "founder-review", "review.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"mindline-founder-review/v1","run_id":"`+testProofRunID+`","proof_run_id":"`+testProofRunID+`","structural_proof_fingerprint":"`+testProofFingerprint+`","cited_records_fingerprint":"`+testCitedRecordsFingerprint+`","verdict":"useful","event_id":"`+eventID(testRequest(VerdictUseful))+`","retry_token_hash":"`+retryTokenHash(testRetryToken)+`","recorded_at":"2026-07-27T10:11:12Z","query":"must-not-persist"}`), privateio.FileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Load(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Load() error = %v, want unavailable for unknown/source-adjacent field", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("elsewhere", path); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Load(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Load() error = %v, want unavailable for symlink", err)
	}
}

func TestConcurrentRepositoriesRecordExactlyOne(t *testing.T) {
	root := t.TempDir()
	first := testRepository(t, root)
	second := testRepository(t, root)
	var start sync.WaitGroup
	start.Add(1)
	results := make(chan error, 2)
	for _, repository := range []*Repository{first, second} {
		go func(repository *Repository) {
			start.Wait()
			_, err := repository.Create(context.Background(), testRequest(VerdictUseful))
			results <- err
		}(repository)
	}
	start.Done()
	var successes int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrAlreadyRecorded) && !errors.Is(err, ErrLockBusy) {
			t.Fatalf("concurrent Create error = %v", err)
		}
	}
	if successes < 1 || successes > 2 {
		t.Fatalf("successful review/replays = %d, want 1 or 2", successes)
	}
	if record, err := first.Load(context.Background()); err != nil || record.Verdict != VerdictUseful {
		t.Fatalf("Load() = %+v, %v; want one useful record", record, err)
	}
}

func testRepository(t *testing.T, root string) *Repository {
	t.Helper()
	repository, err := NewRepository(root, Options{
		Now:     func() time.Time { return time.Date(2026, time.July, 27, 10, 11, 12, 0, time.UTC) },
		Entropy: bytes.NewReader(bytes.Repeat([]byte{7}, 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
