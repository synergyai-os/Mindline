package productbrain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/routing"
)

type memoryPBTransport struct {
	capability      WorkspaceCapability
	entries         map[string]EntryReadback
	relations       []RelationReadback
	entryCreates    int
	relationCreates int
}

type ambiguousMutationTransport struct {
	*memoryPBTransport
	entryReads    int
	relationReads int
}

type successfulEntryReadFailureTransport struct {
	*memoryPBTransport
	entryReads int
}

func (f *successfulEntryReadFailureTransport) GetEntry(ctx context.Context, id string) (EntryReadback, error) {
	f.entryReads++
	if f.entryReads > 1 {
		return EntryReadback{}, &TransportError{Category: "transient"}
	}
	return f.memoryPBTransport.GetEntry(ctx, id)
}

func (f *successfulEntryReadFailureTransport) CreateEntry(context.Context, CreateEntryRequest) (CreateEntryResult, error) {
	f.entryCreates++
	return CreateEntryResult{EntryID: "accepted", Status: "draft"}, nil
}

type successfulRelationReadFailureTransport struct {
	*memoryPBTransport
	relationCreateResponded bool
}

type entryMismatchTransport struct {
	*memoryPBTransport
	mode string
}

func (f *entryMismatchTransport) CreateEntry(ctx context.Context, request CreateEntryRequest) (CreateEntryResult, error) {
	result, err := f.memoryPBTransport.CreateEntry(ctx, request)
	if err != nil {
		return result, err
	}
	readback := f.entries[request.EntryID]
	switch f.mode {
	case "status":
		readback.Status = "active"
	case "actor":
		readback.CreatedBy = "unexpected:actor"
	case "data":
		readback.Data = map[string]any{"unexpected": true}
	}
	f.entries[request.EntryID] = readback
	return result, nil
}

type relationMismatchTransport struct {
	*memoryPBTransport
	mode string
}

func (f *relationMismatchTransport) CreateEntryRelation(ctx context.Context, request CreateRelationRequest) (CreateRelationResult, error) {
	result, err := f.memoryPBTransport.CreateEntryRelation(ctx, request)
	if err != nil {
		return result, err
	}
	relation := f.relations[len(f.relations)-1]
	switch f.mode {
	case "endpoint":
		relation.ToDocID = "wrong-endpoint"
	case "type":
		relation.Type = "wrong_type"
	case "metadata":
		relation.Metadata = map[string]any{"unexpected": true}
	}
	f.relations[len(f.relations)-1] = relation
	return result, nil
}

func (f *successfulRelationReadFailureTransport) ListEntryRelations(ctx context.Context, from string) ([]RelationReadback, error) {
	if f.relationCreateResponded {
		return nil, &TransportError{Category: "transient"}
	}
	return f.memoryPBTransport.ListEntryRelations(ctx, from)
}

func (f *successfulRelationReadFailureTransport) CreateEntryRelation(context.Context, CreateRelationRequest) (CreateRelationResult, error) {
	f.relationCreates++
	f.relationCreateResponded = true
	return CreateRelationResult{RelationID: "accepted"}, nil
}

type networkCountingTransport struct {
	*memoryPBTransport
	networkCalls int
}

func (f *networkCountingTransport) ResolveWorkspace(ctx context.Context) (WorkspaceCapability, error) {
	f.networkCalls++
	return f.memoryPBTransport.ResolveWorkspace(ctx)
}

func (f *ambiguousMutationTransport) GetEntry(ctx context.Context, id string) (EntryReadback, error) {
	f.entryReads++
	if f.entryReads > 1 {
		return EntryReadback{}, &TransportError{Category: "transient"}
	}
	return f.memoryPBTransport.GetEntry(ctx, id)
}

func (f *ambiguousMutationTransport) CreateEntry(context.Context, CreateEntryRequest) (CreateEntryResult, error) {
	return CreateEntryResult{}, &TransportError{Category: "transient", MayHaveCommitted: true}
}

func (f *ambiguousMutationTransport) ListEntryRelations(ctx context.Context, from string) ([]RelationReadback, error) {
	f.relationReads++
	if f.relationReads > 1 {
		return nil, &TransportError{Category: "transient"}
	}
	return f.memoryPBTransport.ListEntryRelations(ctx, from)
}

func (f *ambiguousMutationTransport) CreateEntryRelation(context.Context, CreateRelationRequest) (CreateRelationResult, error) {
	return CreateRelationResult{}, &TransportError{Category: "transient", MayHaveCommitted: true}
}

func newMemoryPBTransport(profile DeliveryProfile) *memoryPBTransport {
	return &memoryPBTransport{capability: WorkspaceCapability{ID: profile.Workspace.ExpectedID, Slug: profile.Workspace.ExpectedSlug, GovernanceMode: "open", KeyScope: "readwrite", KeyID: profile.Credential.ExpectedKeyID}, entries: map[string]EntryReadback{}}
}
func (f *memoryPBTransport) ResolveWorkspace(context.Context) (WorkspaceCapability, error) {
	return f.capability, nil
}
func (f *memoryPBTransport) GetCollectionFields(_ context.Context, slug string) (CollectionCapability, error) {
	return testCollectionCapability(slug), nil
}
func (f *memoryPBTransport) GetEntry(_ context.Context, id string) (EntryReadback, error) {
	value, ok := f.entries[id]
	if !ok {
		return EntryReadback{Found: false}, nil
	}
	return value, nil
}
func (f *memoryPBTransport) SearchEntries(_ context.Context, query, collection string) ([]EntrySearchResult, error) {
	out := []EntrySearchResult{}
	for _, entry := range f.entries {
		if entry.Name == query && entry.CollectionSlug == collection {
			out = append(out, EntrySearchResult{DocID: entry.DocID, EntryID: entry.EntryID, CollectionSlug: entry.CollectionSlug, Name: entry.Name, Status: entry.Status})
		}
	}
	return out, nil
}
func (f *memoryPBTransport) CreateEntry(_ context.Context, r CreateEntryRequest) (CreateEntryResult, error) {
	if _, ok := f.entries[r.EntryID]; ok {
		return CreateEntryResult{}, &TransportError{Category: "already_exists"}
	}
	f.entryCreates++
	f.entries[r.EntryID] = EntryReadback{Found: true, DocID: fmt.Sprintf("doc-%d", f.entryCreates), EntryID: r.EntryID, CollectionSlug: r.CollectionSlug, Name: r.Name, Status: "draft", Data: r.Data, SourceRef: r.SourceRef, SourceExcerpt: r.SourceExcerpt, CreatedBy: r.CreatedBy}
	return CreateEntryResult{EntryID: r.EntryID, Status: "draft"}, nil
}
func (f *memoryPBTransport) ListEntryRelations(_ context.Context, _ string) ([]RelationReadback, error) {
	return append([]RelationReadback{}, f.relations...), nil
}
func (f *memoryPBTransport) CreateEntryRelation(_ context.Context, r CreateRelationRequest) (CreateRelationResult, error) {
	from := f.entries[r.FromEntryID]
	to := f.entries[r.ToEntryID]
	for _, relation := range f.relations {
		if relation.FromDocID == from.DocID && relation.ToDocID == to.DocID && relation.Type == r.Type {
			return CreateRelationResult{RelationID: relation.RelationID, AlreadyExists: true}, nil
		}
	}
	f.relationCreates++
	relation := RelationReadback{RelationID: fmt.Sprintf("rel-%d", f.relationCreates), FromDocID: from.DocID, ToDocID: to.DocID, Type: r.Type, Metadata: r.Metadata}
	f.relations = append(f.relations, relation)
	return CreateRelationResult{RelationID: relation.RelationID}, nil
}
func (f *memoryPBTransport) RuntimeSecretFindings(any) []PrivacyFinding { return nil }

func TestFindRelationRejectsDuplicateAndConflictingIdentity(t *testing.T) {
	expected := RelationOperation{Type: "related_to", Metadata: map[string]any{"rationale": "evidence"}}
	exact := RelationReadback{RelationID: "rel-1", FromDocID: "from", ToDocID: "to", Type: "related_to", Metadata: map[string]any{"rationale": "evidence"}}
	if matched, _, conflict := findRelation([]RelationReadback{exact}, "from", "to", expected); !matched || conflict {
		t.Fatal("single exact relation was not acknowledged")
	}
	duplicate := exact
	duplicate.RelationID = "rel-2"
	if matched, _, conflict := findRelation([]RelationReadback{exact, duplicate}, "from", "to", expected); matched || !conflict {
		t.Fatal("duplicate exact relations were not rejected")
	}
	conflicting := duplicate
	conflicting.Metadata = map[string]any{"rationale": "different"}
	if matched, _, conflict := findRelation([]RelationReadback{exact, conflicting}, "from", "to", expected); matched || !conflict {
		t.Fatal("exact plus conflicting relation was not rejected")
	}
}

func TestMutationAmbiguitySurvivesFailedImmediateReconciliation(t *testing.T) {
	profile := testDeliveryProfile()
	t.Run("entry", func(t *testing.T) {
		transport := &ambiguousMutationTransport{memoryPBTransport: newMemoryPBTransport(profile)}
		run := DeliveryRun{Operations: []DeliveryOperationResult{{OperationID: "op-entry", Kind: "entry"}}}
		expected := EntryOperation{EntryID: "ENTRY-1", CollectionSlug: "entries", Name: "Entry", Data: map[string]any{}, ForceDraft: true, CreatedBy: ExpectedCreatedBy}
		if _, _, err := deliverEntry(context.Background(), transport, expected, &run, 0, t.TempDir(), writeActiveJournal); safeCategory(err) != "ambiguous_outcome" {
			t.Fatalf("may-have-committed entry became %q", safeCategory(err))
		}
	})
	t.Run("relation", func(t *testing.T) {
		transport := &ambiguousMutationTransport{memoryPBTransport: newMemoryPBTransport(profile)}
		run := DeliveryRun{Operations: []DeliveryOperationResult{{OperationID: "op-relation", Kind: "relation"}}}
		expected := RelationOperation{FromEntryID: "FROM", ToEntryID: "TO", Type: "related_to", IfMissing: true, Metadata: map[string]any{}}
		entries := map[string]EntryReadback{"FROM": {Found: true, DocID: "doc-from"}, "TO": {Found: true, DocID: "doc-to"}}
		if _, err := deliverRelation(context.Background(), transport, expected, profile.Credential.ExpectedKeyID, entries, &run, 0, t.TempDir(), writeActiveJournal); safeCategory(err) != "ambiguous_outcome" {
			t.Fatalf("may-have-committed relation became %q", safeCategory(err))
		}
	})
}

func TestSuccessfulMutationResponseWithFailedReadbackIsSealedAsAmbiguousWrite(t *testing.T) {
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	preflight := testPreflight(outbox, profile)
	tests := []struct {
		name      string
		transport ProductBrainTransport
		wantKind  string
	}{
		{"entry", &successfulEntryReadFailureTransport{memoryPBTransport: newMemoryPBTransport(profile)}, "entry"},
		{"relation", &successfulRelationReadFailureTransport{memoryPBTransport: newMemoryPBTransport(profile)}, "relation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := Deliver(context.Background(), outbox, profile, preflight, test.transport, dir, DeliveryOptions{})
			if safeCategory(err) != "ambiguous_outcome" {
				t.Fatalf("successful mutation plus failed readback became %q", safeCategory(err))
			}
			var history DeliveryHistory
			if err := privateio.ReadJSONStrict(dir, filepath.Join(dir, "delivery-history.json"), &history); err != nil {
				t.Fatal(err)
			}
			if len(history.Runs) != 1 {
				t.Fatalf("unexpected run count: %d", len(history.Runs))
			}
			run := history.Runs[0]
			if test.wantKind == "entry" && run.EntriesCreated != 1 {
				t.Fatalf("successful entry response was not counted: %+v", run)
			}
			if test.wantKind == "relation" && run.RelationsCreated != 1 {
				t.Fatalf("successful relation response was not counted: %+v", run)
			}
			found := false
			for _, operation := range run.Operations {
				if operation.Kind == test.wantKind && operation.State == "blocked" {
					found = operation.MutationResponseReceived && !operation.MutationObserved && operation.SafeCategory == "ambiguous_outcome"
					break
				}
			}
			if !found {
				t.Fatal("sealed operation did not distinguish mutation response from readback observation")
			}
		})
	}
}

func TestSuccessfulMutationResponseWithMismatchingReadbackCountsWrite(t *testing.T) {
	profile := testDeliveryProfile()
	tests := []struct {
		name      string
		kind      string
		transport func() ProductBrainTransport
	}{
		{"entry status", "entry", func() ProductBrainTransport {
			return &entryMismatchTransport{memoryPBTransport: newMemoryPBTransport(profile), mode: "status"}
		}},
		{"entry actor", "entry", func() ProductBrainTransport {
			return &entryMismatchTransport{memoryPBTransport: newMemoryPBTransport(profile), mode: "actor"}
		}},
		{"entry data", "entry", func() ProductBrainTransport {
			return &entryMismatchTransport{memoryPBTransport: newMemoryPBTransport(profile), mode: "data"}
		}},
		{"relation endpoint", "relation", func() ProductBrainTransport {
			return &relationMismatchTransport{memoryPBTransport: newMemoryPBTransport(profile), mode: "endpoint"}
		}},
		{"relation type", "relation", func() ProductBrainTransport {
			return &relationMismatchTransport{memoryPBTransport: newMemoryPBTransport(profile), mode: "type"}
		}},
		{"relation metadata", "relation", func() ProductBrainTransport {
			return &relationMismatchTransport{memoryPBTransport: newMemoryPBTransport(profile), mode: "metadata"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			_, err = Deliver(context.Background(), outbox, profile, testPreflight(outbox, profile), test.transport(), dir, DeliveryOptions{})
			if safeCategory(err) != "readback_mismatch" {
				t.Fatalf("mismatching readback became %q", safeCategory(err))
			}
			var history DeliveryHistory
			if err := privateio.ReadJSONStrict(dir, filepath.Join(dir, "delivery-history.json"), &history); err != nil {
				t.Fatal(err)
			}
			run := history.Runs[0]
			wantEntries, wantRelations := 1, 0
			if test.kind == "relation" {
				wantEntries, wantRelations = 3, 1
			}
			if run.EntriesCreated != wantEntries || run.RelationsCreated != wantRelations {
				t.Fatalf("successful mutation response was undercounted: %+v", run)
			}
			found := false
			for _, operation := range run.Operations {
				if operation.Kind == test.kind && operation.State == "blocked" {
					found = operation.MutationResponseReceived && operation.MutationObserved && operation.SafeCategory == "readback_mismatch"
					break
				}
			}
			if !found {
				t.Fatal("mismatching response/readback authority was not preserved")
			}
		})
	}
}

func TestPostMutationJournalFailureLeavesDurableSendingAuthority(t *testing.T) {
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	preflight := testPreflight(outbox, profile)
	transport := newMemoryPBTransport(profile)
	dir := t.TempDir()
	injected := false
	writer := func(outDir string, run DeliveryRun) error {
		for _, operation := range run.Operations {
			if operation.MutationResponseReceived && !injected {
				injected = true
				return errors.New("injected journal failure")
			}
		}
		return writeActiveJournal(outDir, run)
	}
	if _, err := Deliver(context.Background(), outbox, profile, preflight, transport, dir, DeliveryOptions{journalWriter: writer}); safeCategory(err) != "local_state_failure" {
		t.Fatalf("journal failure became %q", safeCategory(err))
	}
	if transport.entryCreates != 1 {
		t.Fatalf("expected one remote mutation before injected storage failure, got %d", transport.entryCreates)
	}
	var active DeliveryRun
	if err := privateio.ReadJSONStrict(dir, filepath.Join(dir, ".delivery-active.json"), &active); err != nil {
		t.Fatal(err)
	}
	if active.Outcome != "running" || active.Operations[0].State != "sending" || active.Operations[0].MutationResponseReceived {
		t.Fatalf("last durable authority was not the pre-mutation sending state: %+v", active.Operations[0])
	}
	if _, err := os.Stat(filepath.Join(dir, "delivery-history.json")); !os.IsNotExist(err) {
		t.Fatal("unjournaled in-memory transition was sealed into delivery history")
	}
	summary, err := Deliver(context.Background(), outbox, profile, preflight, transport, dir, DeliveryOptions{})
	if err != nil {
		t.Fatalf("restart failed to reconcile durable sending authority: %v", err)
	}
	if summary.InterruptedRunCount != 1 || transport.entryCreates != 3 || transport.relationCreates != 2 {
		t.Fatalf("restart duplicated or lost recovery lineage: %+v", summary)
	}
}

func TestLocalAuthorityViolationsFailBeforeNetwork(t *testing.T) {
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	preflight := testPreflight(outbox, profile)
	tests := []struct {
		name    string
		prepare func(*testing.T, string)
	}{
		{"symlink lock", func(t *testing.T, dir string) { createAuthoritySymlink(t, filepath.Join(dir, ".delivery.lock")) }},
		{"wrong-mode lock", func(t *testing.T, dir string) {
			body, _ := json.Marshal(deliveryLock{PID: os.Getpid(), Hostname: "host"})
			path := filepath.Join(dir, ".delivery.lock")
			if err := os.WriteFile(path, body, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink binding", func(t *testing.T, dir string) { createAuthoritySymlink(t, filepath.Join(dir, "delivery-binding.json")) }},
		{"symlink projected history", func(t *testing.T, dir string) { createAuthoritySymlink(t, filepath.Join(dir, "delivery-history.json")) }},
		{"symlink active journal", func(t *testing.T, dir string) { createAuthoritySymlink(t, filepath.Join(dir, ".delivery-active.json")) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			test.prepare(t, dir)
			transport := &networkCountingTransport{memoryPBTransport: newMemoryPBTransport(profile)}
			if _, err := Deliver(context.Background(), outbox, profile, preflight, transport, dir, DeliveryOptions{}); err == nil {
				t.Fatal("invalid local authority was accepted")
			}
			if transport.networkCalls != 0 || transport.entryCreates != 0 || transport.relationCreates != 0 {
				t.Fatalf("network or mutation occurred before rejecting local authority: %+v", transport)
			}
		})
	}
}

func createAuthoritySymlink(t *testing.T, path string) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "authority.json")
	if err := privateio.WriteJSON(target, map[string]any{"sentinel": true}); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func TestDeliveryRejectsSymlinkedPreflightSnapshotBeforeMutation(t *testing.T) {
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	preflight := testPreflight(outbox, profile)
	dir := t.TempDir()
	snapshotDir := filepath.Join(dir, "preflight-snapshots")
	if err := privateio.PrepareDir(snapshotDir); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside-preflight.json")
	if err := privateio.WriteJSON(target, preflight); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(snapshotDir, preflight.Fingerprint+".json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	transport := newMemoryPBTransport(profile)
	if _, err := Deliver(context.Background(), outbox, profile, preflight, transport, dir, DeliveryOptions{}); err == nil {
		t.Fatal("delivery accepted symlinked preflight authority")
	}
	if transport.entryCreates != 0 || transport.relationCreates != 0 {
		t.Fatal("delivery mutated destination before rejecting symlinked preflight authority")
	}
}

func TestCompileDeliverAndReplayFiveOperationConstellation(t *testing.T) {
	route := promotedRouteFixture(t)
	profile := testDeliveryProfile()
	outbox, summary, err := CompileOutbox(route, profile)
	if err != nil {
		t.Fatalf("CompileOutbox: %v", err)
	}
	if summary.OperationCount != 5 || summary.EntryOperationCount != 3 || summary.RelationOperationCount != 2 || len(outbox.PrivacyFindings) != 0 {
		t.Fatalf("unexpected outbox summary: %+v", summary)
	}
	preflight := testPreflight(outbox, profile)
	fake := newMemoryPBTransport(profile)
	dir := t.TempDir()
	clock := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	first, err := Deliver(context.Background(), outbox, profile, preflight, fake, dir, DeliveryOptions{Now: func() time.Time { clock = clock.Add(time.Second); return clock }})
	if err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if first.EntriesAcknowledged != 3 || first.RelationsAcknowledged != 2 || first.FirstRunEntryMutations != 3 || first.FirstRunRelationMutations != 2 {
		t.Fatalf("unexpected first summary: %+v", first)
	}
	second, err := Deliver(context.Background(), outbox, profile, preflight, fake, dir, DeliveryOptions{Now: func() time.Time { clock = clock.Add(time.Second); return clock }})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !second.ReplayZeroMutation || second.LatestRunEntryMutations != 0 || second.LatestRunRelationMutations != 0 || fake.entryCreates != 3 || fake.relationCreates != 2 {
		t.Fatalf("replay was not idempotent: %+v", second)
	}
	historyPath := filepath.Join(dir, "delivery-history.json")
	before, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteIntegratedReview(filepath.Join(t.TempDir(), "review"), dir, route, outbox, profile); err != nil {
		t.Fatalf("strict integrated review: %v", err)
	}
	after, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("integrated review mutated delivery authority")
	}
	tamperedRoute := cloneRoutingResult(t, route)
	tamperedRoute.Decisions.Sources[0].LensResults[0].Rationale = "tampered rationale"
	if _, err := WriteIntegratedReview(filepath.Join(t.TempDir(), "review-tampered-routing"), dir, tamperedRoute, outbox, profile); err == nil {
		t.Fatal("integrated review accepted routing evidence that did not match the embedded ordered review matrix")
	}
	snapshots, err := os.ReadDir(filepath.Join(dir, "preflight-snapshots"))
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("unexpected preflight snapshots: %v %v", snapshots, err)
	}
	if err := os.Remove(filepath.Join(dir, "preflight-snapshots", snapshots[0].Name())); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteIntegratedReview(filepath.Join(t.TempDir(), "review-missing-snapshot"), dir, route, outbox, profile); err == nil {
		t.Fatal("integrated review accepted missing preflight authority")
	}
}

func TestValidateOutboxRejectsMalformedOperationAuthority(t *testing.T) {
	profile := testDeliveryProfile()
	original, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Outbox)
	}{
		{"payload tamper", func(outbox *Outbox) { outbox.Operations[0].Entry.Name = "tampered" }},
		{"unknown kind", func(outbox *Outbox) {
			outbox.Operations[0].Kind = "future"
			outbox.Operations[0].PayloadFingerprint = outboxOperationFingerprint(outbox.Operations[0])
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"nil entry payload", func(outbox *Outbox) {
			outbox.Operations[0].Entry = nil
			outbox.Operations[0].PayloadFingerprint = outboxOperationFingerprint(outbox.Operations[0])
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"wrong relation dependency", func(outbox *Outbox) {
			index := len(outbox.Operations) - 1
			outbox.Operations[index].Dependencies = []string{"op-entry-does-not-exist", outbox.Operations[0].OperationID}
			outbox.Operations[index].PayloadFingerprint = outboxOperationFingerprint(outbox.Operations[index])
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"human entry actor", func(outbox *Outbox) {
			outbox.Operations[0].Entry.CreatedBy = "human:user"
			outbox.Operations[0].PayloadFingerprint = outboxOperationFingerprint(outbox.Operations[0])
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"wrong relation key id", func(outbox *Outbox) {
			index := len(outbox.Operations) - 1
			outbox.Operations[index].Relation.Metadata["credential_key_id"] = "wrong-key"
			outbox.Operations[index].PayloadFingerprint = outboxOperationFingerprint(outbox.Operations[index])
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"extra proposedBy attribution", func(outbox *Outbox) {
			index := len(outbox.Operations) - 1
			outbox.Operations[index].Relation.Metadata["proposedBy"] = "user"
			outbox.Operations[index].PayloadFingerprint = outboxOperationFingerprint(outbox.Operations[index])
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"unsupported snapshot mapping", func(outbox *Outbox) {
			outbox.ProfileSnapshot.RoleMappings["reference_resource"] = RoleMapping{CollectionSlug: "resources", IDPrefix: "RES"}
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"new outbox omits transport authority", func(outbox *Outbox) {
			outbox.ProfileSnapshot.TransportKind = ""
			outbox.ProfileSnapshot.TransportAPIPath = ""
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"missing embedded review authority", func(outbox *Outbox) { outbox.ReviewContext.Captures = nil; outbox.Fingerprint = hashValue(*outbox) }},
		{"duplicate capture ordinal", func(outbox *Outbox) {
			outbox.ReviewContext.Captures[0].CaptureRef = "capture-002"
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"missing destination operation closure", func(outbox *Outbox) {
			outbox.ReviewContext.Captures[0].DestinationOperationIDs = outbox.ReviewContext.Captures[0].DestinationOperationIDs[:1]
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"unsupported lens result", func(outbox *Outbox) {
			outbox.ReviewContext.Captures[0].LensResults[0].Result = "teleport"
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"unsupported semantic assessment role", func(outbox *Outbox) {
			outbox.ReviewContext.Captures[0].SemanticAssessment.PrimaryRole = "reference"
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"unsupported disposition", func(outbox *Outbox) {
			outbox.ReviewContext.Captures[0].Disposition = "teleport"
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"refingerprinted entry diverges from semantic node", func(outbox *Outbox) {
			outbox.Operations[0].Entry.Name = "Refingerprinted but semantically unbound"
			outbox.Operations[0].PayloadFingerprint = outboxOperationFingerprint(outbox.Operations[0])
			outbox.Fingerprint = hashValue(*outbox)
		}},
		{"refingerprinted relation diverges from semantic edge", func(outbox *Outbox) {
			index := len(outbox.Operations) - 1
			outbox.Operations[index].Relation.Metadata["rationale"] = "Different evidence relation."
			outbox.Operations[index].PayloadFingerprint = outboxOperationFingerprint(outbox.Operations[index])
			outbox.Fingerprint = hashValue(*outbox)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outbox := cloneOutbox(t, original)
			test.mutate(&outbox)
			if err := ValidateOutbox(outbox); err == nil {
				t.Fatal("malformed outbox was accepted")
			}
		})
	}
}

func TestOmittedTransportCompatibilityIsBoundToExactDeliveredOutbox(t *testing.T) {
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), testDeliveryProfile())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := outbox.ProfileSnapshot
	snapshot.TransportKind = ""
	snapshot.TransportAPIPath = ""
	if err := validateProfileSnapshot(snapshot, legacyDeliveredOutboxFingerprint); err != nil {
		t.Fatalf("exact legacy transport identity was rejected: %v", err)
	}
	if err := validateProfileSnapshot(snapshot, "refingerprinted-new-outbox"); err == nil {
		t.Fatal("omitted transport authority was accepted for a new outbox")
	}
}

func TestAlternateUserLensVariantChangesOnlyRoutingAndAdapterOperationCount(t *testing.T) {
	canonicalURLs := []string{"https://example.com/public-tool", "https://example.org/public-service"}
	graph := routing.SourceGraph{SchemaVersion: routing.SourceGraphSchema, Adapter: routing.AdapterRef{Kind: "bookmark", Version: "v1"}}
	artifacts := routing.LinkArtifacts{SchemaVersion: routing.LinkArtifactsSchema}
	for index, canonicalURL := range canonicalURLs {
		id := routing.CanonicalURLID(canonicalURL)
		sourceID := fmt.Sprintf("source-%d", index+1)
		occurrenceID := fmt.Sprintf("occurrence-%d", index+1)
		graph.SourceRecords = append(graph.SourceRecords, routing.SourceRecord{SourceRecordID: sourceID, SourceKind: "bookmark", OccurredAt: "2026-07-14T10:00:00Z", RawProvenanceRef: fmt.Sprintf("adapter-local://bookmark/%d", index+1), URLOccurrenceIDs: []string{occurrenceID}})
		graph.URLOccurrences = append(graph.URLOccurrences, routing.URLOccurrence{URLOccurrenceID: occurrenceID, SourceRecordID: sourceID, ObservedURL: canonicalURL, CanonicalURLID: id})
		graph.CanonicalURLs = append(graph.CanonicalURLs, routing.CanonicalURL{CanonicalURLID: id, CanonicalURL: canonicalURL, Kind: "generic_web", Discovery: "source_occurrence", EnrichmentState: "complete"})
		graph.Edges = append(graph.Edges, routing.GraphEdge{EdgeID: fmt.Sprintf("edge-%d", index+1), Type: "source_record_contains_url", From: sourceID, To: id, EvidenceRefs: []string{occurrenceID}})
		artifacts.Items = append(artifacts.Items, routing.LinkArtifact{CanonicalURL: canonicalURL, State: "complete", PublicMetadata: routing.PublicMetadata{Title: fmt.Sprintf("Public source %d", index+1)}, PublicExcerpts: []routing.PublicExcerpt{{ExcerptID: fmt.Sprintf("evidence-%d", index+1), Text: fmt.Sprintf("Public evidence %d.", index+1), Locator: "page"}}})
	}
	profileA := routing.LensProfile{SchemaVersion: routing.LensProfileSchemaVersion, ProfileID: "work-user", ProfileVersion: "1", Lenses: []routing.Lens{{LensID: "product-work", Name: "Product work", Question: "Does this inform product work?"}}}
	profileB := routing.LensProfile{SchemaVersion: routing.LensProfileSchemaVersion, ProfileID: "garden-user", ProfileVersion: "1", Lenses: []routing.Lens{{LensID: "garden-design", Name: "Garden design", Question: "Does this inform garden design?"}}}
	judgments := func(profile routing.LensProfile, archiveSecond bool) routing.Judgments {
		manifest := routing.Judgments{SchemaVersion: routing.JudgmentsSchema, JudgmentMethod: "operator_agent_review", JudgedAt: "2026-07-14T10:00:00Z", ProfileID: profile.ProfileID, ProfileVersion: profile.ProfileVersion}
		for index, canonicalURL := range canonicalURLs {
			evidenceID := fmt.Sprintf("evidence-%d", index+1)
			result, disposition := "matched", "promote"
			if archiveSecond && index == 1 {
				result, disposition = "not_matched", "archive"
			}
			manifest.Sources = append(manifest.Sources, routing.SourceJudgment{
				CanonicalURLID:     routing.CanonicalURLID(canonicalURL),
				LensResults:        []routing.LensResult{{LensID: profile.Lenses[0].LensID, Result: result, Confidence: .9, Rationale: "The public evidence determines relevance for this user lens.", EvidenceRefs: []string{evidenceID}}},
				SemanticAssessment: routing.SemanticAssessment{PrimaryRole: "external_entity", Summary: fmt.Sprintf("Stable public meaning %d.", index+1), Confidence: .9, EvidenceRefs: []string{evidenceID}},
				Disposition:        disposition, DispositionRationale: "The explicit operator judgment selects the route for this lens.",
				SemanticNodes: []routing.SemanticNode{{SemanticNodeID: fmt.Sprintf("entity-%d", index+1), Role: "external_entity", Name: fmt.Sprintf("Public entity %d", index+1), Description: fmt.Sprintf("Stable public description %d.", index+1), Confidence: .9, LensRefs: []string{profile.Lenses[0].LensID}, EvidenceRefs: []string{evidenceID}, Attributes: map[string]any{}}},
			})
		}
		return manifest
	}
	routeA, err := routing.CompileGraph(graph, artifacts, profileA, judgments(profileA, false))
	if err != nil {
		t.Fatal(err)
	}
	routeB, err := routing.CompileGraph(graph, artifacts, profileB, judgments(profileB, true))
	if err != nil {
		t.Fatal(err)
	}
	for index := range routeA.Decisions.Sources {
		leftMeaning, _ := json.Marshal(routeA.Decisions.Sources[index].SemanticAssessment)
		rightMeaning, _ := json.Marshal(routeB.Decisions.Sources[index].SemanticAssessment)
		if !bytes.Equal(leftMeaning, rightMeaning) || routeA.Decisions.Sources[index].SemanticNodes[0].Role != routeB.Decisions.Sources[index].SemanticNodes[0].Role {
			t.Fatalf("stable meaning or semantic role changed at source %d", index)
		}
	}
	if routeA.Decisions.Sources[1].Disposition != "promote" || routeB.Decisions.Sources[1].Disposition != "archive" || routeA.Decisions.Sources[1].LensResults[0].Result == routeB.Decisions.Sources[1].LensResults[0].Result {
		t.Fatal("lens-only variant did not change relevance and disposition")
	}
	routingJSON, _ := json.Marshal(routeB)
	for _, field := range []string{"collection_slug", "entry_id", "force_draft", "created_by"} {
		if bytes.Contains(routingJSON, []byte(field)) {
			t.Fatalf("routing authority leaked destination field %q", field)
		}
	}
	neutralRoutingJSON := strings.ToLower(string(routingJSON))
	for _, vocabulary := range []string{"slack", "product brain", "tolaria"} {
		if strings.Contains(neutralRoutingJSON, vocabulary) {
			t.Fatalf("generic routing result leaked adapter vocabulary %q", vocabulary)
		}
	}
	outboxA, summaryA, err := CompileOutbox(routeA, testDeliveryProfile())
	if err != nil {
		t.Fatal(err)
	}
	outboxB, summaryB, err := CompileOutbox(routeB, testDeliveryProfile())
	if err != nil {
		t.Fatal(err)
	}
	if summaryA.OperationCount != 2 || summaryB.OperationCount != 1 || len(outboxA.Operations) != 2 || len(outboxB.Operations) != 1 {
		t.Fatalf("destination operation count did not change after adapter mapping: A=%d B=%d", summaryA.OperationCount, summaryB.OperationCount)
	}
	if got := outboxB.ReviewContext.PendingActions[0]; got != "Review 1 Product Brain draft entry and 0 proposed relations; accept or reject the routing judgments" {
		t.Fatalf("adapter did not derive the current operation count: %q", got)
	}
	for _, action := range outboxB.ReviewContext.PendingActions {
		if strings.Contains(action, "temporary Product Brain key") {
			t.Fatalf("adapter invented a temporary credential lifecycle: %q", action)
		}
	}
}

func TestCompileOutboxRetainsCountAwareReviewAndExplicitTemporaryLifecycle(t *testing.T) {
	profile := testDeliveryProfile()
	profile.ReviewPolicy = &DeliveryReviewPolicy{CredentialLifecycle: "retire_after_review", PrivateRuntimeLifecycle: "cleanup_after_review"}
	outbox, summary, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	if summary.EntryOperationCount != 3 || summary.RelationOperationCount != 2 {
		t.Fatalf("unexpected compiled operation counts: %+v", summary)
	}
	want := []string{
		"Review 3 Product Brain draft entries and 2 proposed relations; accept or reject the routing judgments",
		"Retire the temporary Product Brain key after review",
		"Confirm owner-validated private runtime cleanup after key retirement",
	}
	if !canonicalEqual(outbox.ReviewContext.PendingActions, want) {
		t.Fatalf("review policy was not retained in signed outbox context: %#v", outbox.ReviewContext.PendingActions)
	}
	if outbox.ProfileSnapshot.ReviewPolicy == nil || outbox.ProfileSnapshot.ReviewPolicy.CredentialLifecycle != "retire_after_review" || outbox.ProfileSnapshot.ReviewPolicy.PrivateRuntimeLifecycle != "cleanup_after_review" {
		t.Fatalf("review policy was not snapshot-bound: %+v", outbox.ProfileSnapshot.ReviewPolicy)
	}
}

func TestCompileOutboxDerivesAllExplicitReviewPolicyCombinations(t *testing.T) {
	tests := []struct {
		name       string
		policy     DeliveryReviewPolicy
		credential string
		runtime    string
	}{
		{name: "persistent and retain", policy: DeliveryReviewPolicy{CredentialLifecycle: "persistent", PrivateRuntimeLifecycle: "retain"}, credential: "Keep the Product Brain credential active under the selected delivery profile", runtime: "Retain the owner-validated private runtime evidence after review"},
		{name: "persistent and cleanup", policy: DeliveryReviewPolicy{CredentialLifecycle: "persistent", PrivateRuntimeLifecycle: "cleanup_after_review"}, credential: "Keep the Product Brain credential active under the selected delivery profile", runtime: "Confirm owner-validated private runtime cleanup after review"},
		{name: "retire and retain", policy: DeliveryReviewPolicy{CredentialLifecycle: "retire_after_review", PrivateRuntimeLifecycle: "retain"}, credential: "Retire the temporary Product Brain key after review", runtime: "Retain the owner-validated private runtime evidence after review"},
		{name: "retire and cleanup", policy: DeliveryReviewPolicy{CredentialLifecycle: "retire_after_review", PrivateRuntimeLifecycle: "cleanup_after_review"}, credential: "Retire the temporary Product Brain key after review", runtime: "Confirm owner-validated private runtime cleanup after key retirement"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := testDeliveryProfile()
			profile.ReviewPolicy = &test.policy
			outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
			if err != nil {
				t.Fatal(err)
			}
			if outbox.ReviewContext.PendingActions[1] != test.credential || outbox.ReviewContext.PendingActions[2] != test.runtime {
				t.Fatalf("review policy was not derived independently: %#v", outbox.ReviewContext.PendingActions)
			}
		})
	}
}

func TestOutboxRejectsRefingerprintedUnauthorizedReviewAction(t *testing.T) {
	profile := testDeliveryProfile()
	profile.ReviewPolicy = &DeliveryReviewPolicy{CredentialLifecycle: "retire_after_review", PrivateRuntimeLifecycle: "cleanup_after_review"}
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ReviewContext.PendingActions[1] = "Retire the temporary Product Brain key immediately without review"
	outbox.Fingerprint = hashValue(outbox)
	if err := ValidateOutbox(outbox); err == nil {
		t.Fatal("refingerprinted policy-unauthorized review action was accepted")
	}
}

func TestLegacyReviewActionSetIsBoundToExactDeliveredOutboxFingerprint(t *testing.T) {
	legacy := Outbox{Fingerprint: legacyDeliveredOutboxFingerprint, ReviewContext: ReviewContext{PendingActions: legacyProductBrainPendingActions()}}
	if err := validateProductBrainPendingActions(legacy); err != nil {
		t.Fatalf("exact delivered legacy authority was rejected: %v", err)
	}
	legacy.Fingerprint = "different-outbox"
	if err := validateProductBrainPendingActions(legacy); err == nil {
		t.Fatal("legacy action set was accepted for a different outbox fingerprint")
	}
}

func TestCurrentOneEntryOutboxCannotAdoptLegacyThreeDraftActions(t *testing.T) {
	route := promotedRouteFixture(t)
	route.Decisions.Sources[0].SemanticNodes = route.Decisions.Sources[0].SemanticNodes[:1]
	route.Decisions.Sources[0].SemanticEdges = nil
	outbox, summary, err := CompileOutbox(route, testDeliveryProfile())
	if err != nil {
		t.Fatal(err)
	}
	if summary.EntryOperationCount != 1 || summary.RelationOperationCount != 0 {
		t.Fatalf("fixture did not produce one entry: %+v", summary)
	}
	outbox.ReviewContext.PendingActions = legacyProductBrainPendingActions()
	outbox.Fingerprint = hashValue(outbox)
	if err := ValidateOutbox(outbox); err == nil {
		t.Fatal("current one-entry outbox adopted the delivered three-draft legacy actions")
	}
}

func TestSummarizeDeliveryRetainsInterruptedAndFailedRecoveryLineage(t *testing.T) {
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	acknowledged := []DeliveryOperationResult{}
	for _, operation := range outbox.Operations {
		result := DeliveryOperationResult{OperationID: operation.OperationID, Kind: operation.Kind, State: "acknowledged", Acknowledged: true, DraftVerified: true, ActorVerified: true, AttributionVerified: true}
		acknowledged = append(acknowledged, result)
	}
	history := DeliveryHistory{OutboxFingerprint: outbox.Fingerprint, ProfileFingerprint: hashValue(profile), Runs: []DeliveryRun{
		{Outcome: "interrupted", EntriesCreated: 1, Operations: acknowledged},
		{Outcome: "completed", EntriesCreated: 2, RelationsCreated: 2, Operations: acknowledged},
		{Outcome: "failed", Operations: acknowledged},
		{Outcome: "completed", Operations: acknowledged},
	}}
	summary := summarizeDelivery(outbox, history, acknowledged)
	if summary.RunCount != 4 || summary.CompletedRunCount != 2 || summary.InterruptedRunCount != 1 || summary.FailedRunCount != 1 || summary.FirstRunEntryMutations != 3 || summary.FirstRunRelationMutations != 2 || !summary.ReplayZeroMutation || summary.DestinationWrites != 5 {
		t.Fatalf("recovery lineage was not retained truthfully: %+v", summary)
	}
}

func TestDeliveryRunAuthorityRejectsUnsignedSafeCategories(t *testing.T) {
	run := DeliveryRun{Operations: []DeliveryOperationResult{{OperationID: "op-1", Kind: "entry", State: "blocked", Attempts: 1, SafeCategory: "transient"}}}
	if err := validateDeliveryRunOperations(run); err != nil {
		t.Fatalf("signed safe category was rejected: %v", err)
	}
	run.Operations[0].SafeCategory = "remote response body"
	if err := validateDeliveryRunOperations(run); err == nil {
		t.Fatal("arbitrary safe category was accepted into sealed authority")
	}
}

func TestDeliveryRunAuthorityCountsMutationResponseOrObservationExactlyOnce(t *testing.T) {
	run := DeliveryRun{EntriesCreated: 1, Operations: []DeliveryOperationResult{{OperationID: "op-1", Kind: "entry", State: "blocked", Attempts: 1, MutationResponseReceived: true, MutationObserved: true, SafeCategory: "readback_mismatch"}}}
	if err := validateDeliveryRunOperations(run); err != nil {
		t.Fatalf("coherent mutation response/observation was rejected: %v", err)
	}
	run.EntriesCreated = 0
	if err := validateDeliveryRunOperations(run); err == nil {
		t.Fatal("mutation response/observation without a truthful run counter was accepted")
	}
	run.EntriesCreated = 1
	run.Operations[0].MutationResponseReceived = false
	run.Operations[0].MutationObserved = false
	if err := validateDeliveryRunOperations(run); err == nil {
		t.Fatal("run counter without mutation response or observation was accepted")
	}
	run.EntriesCreated = 0
	if err := validateDeliveryRunOperations(run); err == nil {
		t.Fatal("readback mismatch without a readback observation was accepted")
	}
}

func cloneOutbox(t *testing.T, value Outbox) Outbox {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone Outbox
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func cloneRoutingResult(t *testing.T, value routing.Result) routing.Result {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone routing.Result
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func TestPreflightUsesReplaceableProductBrainTransportPort(t *testing.T) {
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	fake := newMemoryPBTransport(profile)
	preflight, err := BuildPreflight(context.Background(), outbox, profile, fake)
	if err != nil {
		t.Fatalf("preflight rejected non-AKI transport implementation: %v", err)
	}
	if preflight.Verdict != "pass" || preflight.MutationCalls != 0 {
		t.Fatalf("unexpected port-based preflight: %+v", preflight)
	}
}

func TestPreflightRequiresExactGateSet(t *testing.T) {
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	preflight := testPreflight(outbox, profile)
	preflight.Gates = preflight.Gates[1:]
	preflight.Fingerprint = hashValue(preflight)
	if err := ValidatePreflight(preflight, outbox, profile); err == nil {
		t.Fatal("preflight with a missing base gate was accepted")
	}
	preflight = testPreflight(outbox, profile)
	preflight.Gates = append(preflight.Gates, preflight.Gates[0])
	preflight.Fingerprint = hashValue(preflight)
	if err := ValidatePreflight(preflight, outbox, profile); err == nil {
		t.Fatal("preflight with a duplicate gate was accepted")
	}
	for _, test := range []struct {
		name   string
		mutate func(*PreflightArtifact)
	}{
		{"wrong workspace gate actual", func(value *PreflightArtifact) { value.Gates[2].Actual = "wrong-workspace" }},
		{"read-only key scope", func(value *PreflightArtifact) {
			value.Workspace.KeyScope = "read"
			value.Gates[5].Actual = "read"
		}},
		{"unsupported governance mode", func(value *PreflightArtifact) {
			value.Workspace.GovernanceMode = "future"
			value.Gates[4].Actual = "future"
		}},
		{"wrong collection gate actual", func(value *PreflightArtifact) { value.Gates[len(value.Gates)-1].Actual = "wrong-contract" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := testPreflight(outbox, profile)
			test.mutate(&value)
			value.Fingerprint = hashValue(value)
			if err := ValidatePreflight(value, outbox, profile); err == nil {
				t.Fatal("preflight with false semantic gate values was accepted")
			}
		})
	}
}

func TestDeliveryReviewPacketIsCompleteForTwelveCaptures(t *testing.T) {
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	template := outbox.ReviewContext.Captures[0]
	outbox.ReviewContext.Captures = nil
	for index := 1; index <= 11; index++ {
		canonicalIndex := index
		if index == 2 {
			canonicalIndex = 1
		}
		capture := template
		capture.CaptureRef = fmt.Sprintf("capture-%03d", index)
		capture.CanonicalURL = fmt.Sprintf("https://example.com/source-%02d", canonicalIndex)
		capture.CanonicalURLID = fmt.Sprintf("source-%02d", canonicalIndex)
		capture.PublicMetadata = &routing.PublicMetadata{Title: fmt.Sprintf("Distinct source %02d", canonicalIndex)}
		capture.PublicExcerpts = []routing.PublicExcerpt{{ExcerptID: fmt.Sprintf("evidence-%02d", canonicalIndex), Text: fmt.Sprintf("Public evidence for source %02d.", canonicalIndex), Locator: "page"}}
		capture.Disposition = "hold"
		capture.DispositionRationale = fmt.Sprintf("Hold source %02d for review.", canonicalIndex)
		capture.SemanticNodes = nil
		capture.SemanticEdges = nil
		capture.DestinationOperationIDs = nil
		capture.DuplicateOf = ""
		if index == 2 {
			capture.DuplicateOf = "capture-001"
		}
		outbox.ReviewContext.Captures = append(outbox.ReviewContext.Captures, capture)
	}
	template.CaptureRef = "capture-012"
	template.DuplicateOf = ""
	outbox.ReviewContext.Captures = append(outbox.ReviewContext.Captures, template)
	outbox.Fingerprint = hashValue(outbox)
	fake := newMemoryPBTransport(profile)
	dir := t.TempDir()
	if _, err := Deliver(context.Background(), outbox, profile, testPreflight(outbox, profile), fake, dir, DeliveryOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Deliver(context.Background(), outbox, profile, testPreflight(outbox, profile), fake, dir, DeliveryOptions{}); err != nil {
		t.Fatal(err)
	}
	var history DeliveryHistory
	var summary DeliverySummary
	if err := privateio.ReadJSON(dir+"/delivery-history.json", &history); err != nil {
		t.Fatal(err)
	}
	if err := privateio.ReadJSON(dir+"/delivery-summary.json", &summary); err != nil {
		t.Fatal(err)
	}
	packet := deliveryReviewPacket(outbox, history, summary)
	for index := 1; index <= 12; index++ {
		if strings.Count(packet, fmt.Sprintf("### capture-%03d\n", index)) != 1 {
			t.Fatalf("capture %03d missing or duplicated in details", index)
		}
	}
	for index := 1; index <= 11; index++ {
		canonicalIndex := index
		if index == 2 {
			canonicalIndex = 1
		}
		if !strings.Contains(packet, fmt.Sprintf("Distinct source %02d", canonicalIndex)) || !strings.Contains(packet, fmt.Sprintf("Public evidence for source %02d", canonicalIndex)) {
			t.Fatalf("capture %03d was not traced to its own public evidence", index)
		}
	}
	for _, expected := range []string{"Public evidence", "at 0.95", "Lens judgments", "Semantic nodes", "Semantic edges", "force draft true", ExpectedCreatedBy, "--`related_to`-->", "actual relation `rel-", profile.Credential.ExpectedKeyID, "preflight-snapshots/", "acknowledgements 5/5", "Replay zero mutation: true"} {
		if !strings.Contains(packet, expected) {
			t.Fatalf("review packet missing %q:\n%s", expected, packet)
		}
	}
}

func TestPublicOutboxRejectsPrivateSlackIdentity(t *testing.T) {
	route := promotedRouteFixture(t)
	route.Decisions.Sources[0].SemanticNodes[0].Description = "Leaked C123456789 private source"
	_, _, err := CompileOutbox(route, testDeliveryProfile())
	if err == nil {
		t.Fatal("expected unsafe outbound value rejection")
	}
}

func TestDeliveryReconstructsSealedAuthorityAndStaleProjection(t *testing.T) {
	route := promotedRouteFixture(t)
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(route, profile)
	if err != nil {
		t.Fatal(err)
	}
	preflight := testPreflight(outbox, profile)
	fake := newMemoryPBTransport(profile)
	dir := t.TempDir()
	clock := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { clock = clock.Add(time.Second); return clock }
	if _, err := Deliver(context.Background(), outbox, profile, preflight, fake, dir, DeliveryOptions{Now: now}); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	var firstHistory DeliveryHistory
	if err := privateio.ReadJSON(dir+"/delivery-history.json", &firstHistory); err != nil {
		t.Fatal(err)
	}
	// Model a crash after the immutable run was sealed but before the mutable
	// history and active-journal projections were finalized.
	if err := writeActiveJournal(dir, firstHistory.Runs[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir + "/delivery-history.json"); err != nil {
		t.Fatal(err)
	}
	second, err := Deliver(context.Background(), outbox, profile, preflight, fake, dir, DeliveryOptions{Now: now})
	if err != nil {
		t.Fatalf("reconstructed replay: %v", err)
	}
	if second.RunCount != 2 || !second.ReplayZeroMutation || fake.entryCreates != 3 || fake.relationCreates != 2 {
		t.Fatalf("sealed authority was not reconstructed safely: %+v", second)
	}
	// A valid-but-stale projection is a cache, not authority. The immutable run
	// directory must win and permit another zero-mutation replay.
	if err := privateio.WriteJSON(dir+"/delivery-history.json", firstHistory); err != nil {
		t.Fatal(err)
	}
	third, err := Deliver(context.Background(), outbox, profile, preflight, fake, dir, DeliveryOptions{Now: now})
	if err != nil {
		t.Fatalf("stale projection replay: %v", err)
	}
	if third.RunCount != 3 || !third.ReplayZeroMutation || fake.entryCreates != 3 || fake.relationCreates != 2 {
		t.Fatalf("stale projection overrode sealed authority: %+v", third)
	}
}

func TestFailedDeliveryPreservesBoundOutboxAndProfileProjection(t *testing.T) {
	route := promotedRouteFixture(t)
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(route, profile)
	if err != nil {
		t.Fatal(err)
	}
	fake := newMemoryPBTransport(profile)
	fake.capability.ID = "wrong-workspace"
	summary, err := Deliver(context.Background(), outbox, profile, testPreflight(outbox, profile), fake, t.TempDir(), DeliveryOptions{})
	if err == nil {
		t.Fatal("expected delivery failure")
	}
	if summary.OutboxFingerprint != outbox.Fingerprint || summary.ProfileFingerprint != hashValue(profile) || summary.ExpectedOperationCount != 5 || summary.PrivacyFindingCount != 0 {
		t.Fatalf("failure projection lost its bound inputs: %+v", summary)
	}
}

func testPreflight(outbox Outbox, profile DeliveryProfile) PreflightArtifact {
	preflight := PreflightArtifact{SchemaVersion: PreflightSchema, OutboxFingerprint: outbox.Fingerprint, ProfileFingerprint: hashValue(profile), ExpectedOrigin: profile.Transport.BaseURL, Workspace: WorkspaceCapability{ID: profile.Workspace.ExpectedID, Slug: profile.Workspace.ExpectedSlug, GovernanceMode: "open", KeyScope: "readwrite", KeyID: profile.Credential.ExpectedKeyID}, MutationCalls: 0, Verdict: "pass"}
	actuals := map[string]string{"trusted_origin": profile.Transport.BaseURL, "runtime_secret_scan": "zero findings", "workspace_id": profile.Workspace.ExpectedID, "workspace_slug": profile.Workspace.ExpectedSlug, "governance_mode": "open", "key_scope": "readwrite", "key_id": profile.Credential.ExpectedKeyID}
	for _, name := range []string{"trusted_origin", "runtime_secret_scan", "workspace_id", "workspace_slug", "governance_mode", "key_scope", "key_id"} {
		preflight.Gates = append(preflight.Gates, PreflightGate{Name: name, Verdict: "pass", Actual: actuals[name]})
	}
	for _, slug := range outboxCollectionSlugs(outbox) {
		fingerprint := hashValue(testCollectionCapability(slug))
		preflight.CollectionContracts = append(preflight.CollectionContracts, CollectionContractRef{Slug: slug, Fingerprint: fingerprint})
		preflight.Gates = append(preflight.Gates, PreflightGate{Name: "collection_contract:" + slug, Verdict: "pass", Actual: fingerprint})
	}
	preflight.Fingerprint = hashValue(preflight)
	return preflight
}

func testCollectionCapability(slug string) CollectionCapability {
	fields := map[string][]CollectionFieldCapability{
		"landscape": {
			{Key: "category", Type: "select", Options: []string{"competitor", "complementary", "platform", "ecosystem"}},
			{Key: "relationshipToPb", Type: "select", Options: []string{"direct_competitor", "indirect_competitor", "complementary", "neutral"}},
			{Key: "icpOverlap", Type: "select", Options: []string{"high", "medium", "low", "none"}},
			{Key: "description", Type: "string", Required: true}, {Key: "url", Type: "string"}, {Key: "keyDifferentiator", Type: "text"}, {Key: "whatWeLearn", Type: "text"},
		},
		"insights": {{Key: "source", Type: "string"}, {Key: "description", Type: "string", Required: true}, {Key: "evidenceStrength", Type: "select", Options: []string{"unvalidated", "anecdotal", "first-hand", "strong", "validated"}}},
		"tensions": {{Key: "type", Type: "select", Options: []string{"bug", "improvement", "tech-debt", "process", "ux", "other"}}, {Key: "severity", Type: "select", Options: []string{"low", "medium", "high", "critical"}}, {Key: "priority", Type: "select", Options: []string{"low", "medium", "high", "critical"}}, {Key: "affectedArea", Type: "string"}, {Key: "status", Type: "select", Options: []string{"draft", "active", "resolved"}}, {Key: "description", Type: "string", Required: true}},
	}
	value, ok := fields[slug]
	for index := range value {
		sort.Strings(value[index].Options)
	}
	sort.Slice(value, func(i, j int) bool { return value[i].Key < value[j].Key })
	return CollectionCapability{Found: ok, Slug: slug, Fields: value}
}

func promotedRouteFixture(t *testing.T) routing.Result {
	t.Helper()
	canonical := "https://github.com/EXXETA/exxperts"
	id := routing.CanonicalURLID(canonical)
	profile := routing.LensProfile{SchemaVersion: routing.LensProfileSchemaVersion, ProfileID: "contexts", ProfileVersion: "1", Lenses: []routing.Lens{{LensID: "building-product", Name: "Building", Question: "Relevant?"}, {LensID: "team-design", Name: "Teams", Question: "Relevant?"}}}
	graph := routing.SourceGraph{SchemaVersion: routing.SourceGraphSchema, Adapter: routing.AdapterRef{Kind: "bookmark", Version: "1"}, SourceRecords: []routing.SourceRecord{{SourceRecordID: "src-1", SourceKind: "bookmark", OccurredAt: "2026-07-14T10:00:00Z", RawProvenanceRef: "local-1", URLOccurrenceIDs: []string{"occ-1"}}}, URLOccurrences: []routing.URLOccurrence{{URLOccurrenceID: "occ-1", SourceRecordID: "src-1", ObservedURL: canonical, CanonicalURLID: id}}, CanonicalURLs: []routing.CanonicalURL{{CanonicalURLID: id, CanonicalURL: canonical, Kind: "github_repository", Depth: 0, Discovery: "source_occurrence", EnrichmentState: "complete"}}, Edges: []routing.GraphEdge{{EdgeID: "edge-1", Type: "source_record_contains_url", From: "src-1", To: id, EvidenceRefs: []string{"occ-1"}}}}
	artifacts := routing.LinkArtifacts{SchemaVersion: routing.LinkArtifactsSchema, Items: []routing.LinkArtifact{{CanonicalURL: canonical, State: "complete", PublicExcerpts: []routing.PublicExcerpt{{ExcerptID: "readme", Text: "Persistent AI rooms use approval-gated memory and local files.", Locator: "README"}}}}}
	lenses := []routing.LensResult{{LensID: "building-product", Result: "matched", Confidence: .95, Rationale: "A relevant external implementation.", EvidenceRefs: []string{"readme"}}, {LensID: "team-design", Result: "matched", Confidence: .8, Rationale: "Approval gates shape agent-team governance.", EvidenceRefs: []string{"readme"}}}
	nodes := []routing.SemanticNode{{SemanticNodeID: "entity", Role: "external_entity", Name: "Exxperts", Description: "An external persistent-agent workspace with approval-gated memory.", Confidence: .95, LensRefs: []string{"building-product"}, EvidenceRefs: []string{"readme"}, Attributes: map[string]any{"entity_category": "adjacent_product", "market_relationship": "adjacent", "audience_overlap": "medium", "key_differentiator": "Approval-gated local memory.", "learning": "Governed memory is a visible product capability."}}, {SemanticNodeID: "insight", Role: "evidence_backed_finding", Name: "Governed persistent agent memory is becoming a product capability", Description: "Persistent memory is exposed as a governed user-facing capability.", Confidence: .9, LensRefs: []string{"building-product", "team-design"}, EvidenceRefs: []string{"readme"}, Attributes: map[string]any{}}, {SemanticNodeID: "tension", Role: "unresolved_tension", Name: "Persistent context versus approval burden", Description: "Useful persistent agent context conflicts with silent writes and approval burden.", Confidence: .9, LensRefs: []string{"building-product", "team-design"}, EvidenceRefs: []string{"readme"}, Attributes: map[string]any{"tension_kind": "governance_tradeoff", "severity": "medium", "priority": "investigate", "affected_area": "memory governance", "resolution_state": "unresolved"}}}
	edges := []routing.SemanticEdge{{From: "entity", Type: "related_to", To: "insight", Rationale: "The implementation evidences the product capability.", EvidenceRefs: []string{"readme"}}, {From: "entity", Type: "related_to", To: "tension", Rationale: "The implementation exposes the governance tradeoff.", EvidenceRefs: []string{"readme"}}}
	manifest := routing.Judgments{SchemaVersion: routing.JudgmentsSchema, JudgmentMethod: "operator_agent_review", JudgedAt: "2026-07-14T10:00:00Z", ProfileID: profile.ProfileID, ProfileVersion: profile.ProfileVersion, Sources: []routing.SourceJudgment{{CanonicalURLID: id, LensResults: lenses, SemanticAssessment: routing.SemanticAssessment{PrimaryRole: "external_entity", Summary: "A persistent-agent workspace with governed memory.", Confidence: .95, EvidenceRefs: []string{"readme"}}, Disposition: "promote", DispositionRationale: "Promote one bounded evidence-backed constellation.", SemanticNodes: nodes, SemanticEdges: edges}}}
	result, err := routing.CompileGraph(graph, artifacts, profile, manifest)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func testDeliveryProfile() DeliveryProfile {
	return DeliveryProfile{SchemaVersion: DeliveryProfileSchema, ProfileID: "test", Workspace: DeliveryWorkspace{ExpectedID: "ws-test", ExpectedSlug: "test"}, Transport: DeliveryTransportProfile{Kind: "aki", BaseURL: ProductionGatewayOrigin, APIPath: "/api/aki"}, Credential: DeliveryCredentialProfile{Provider: "environment", Name: "MINDLINE_PRODUCT_BRAIN_API_KEY", ExpectedKeyID: "key-test"}, RoleMappings: map[string]RoleMapping{"external_entity": {CollectionSlug: "landscape", IDPrefix: "LAND"}, "evidence_backed_finding": {CollectionSlug: "insights", IDPrefix: "INS"}, "unresolved_tension": {CollectionSlug: "tensions", IDPrefix: "TEN"}}, RelationMappings: map[string]string{"related_to": "related_to"}, DraftOnly: true}
}

func TestDeliveryProfileRejectsUnsupportedSemanticRoleMapping(t *testing.T) {
	profile := testDeliveryProfile()
	profile.RoleMappings["reference_resource"] = RoleMapping{CollectionSlug: "resources", IDPrefix: "RES"}
	if err := ValidateDeliveryProfile(profile); err == nil {
		t.Fatal("unsupported semantic role mapping was accepted")
	}
}
