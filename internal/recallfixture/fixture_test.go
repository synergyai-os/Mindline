// Package recallfixture assembles a deterministic closed Slack recall run using
// only the public vertical seams. It is intentionally a fixture, not another
// ingestion implementation.
package recallfixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	slackadapter "github.com/synergyai-os/Mindline/internal/adapters/slack"
	"github.com/synergyai-os/Mindline/internal/ingestioncontroller"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
	"github.com/synergyai-os/Mindline/internal/resourcequeue"
)

func TestClosedSlackRecallFixture(t *testing.T) {
	root := t.TempDir()
	envelope := decodeFixtureEnvelope(t)
	repository, err := personalmemory.NewFileRepository(filepath.Join(root, "library"), fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := ingestioncontroller.NewLedgerStore(filepath.Join(root, "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	controller := ingestioncontroller.Controller{Repository: repository, Ledger: ledgerStore}
	ledger, err := controller.Apply(envelope)
	if err != nil {
		t.Fatalf("initial closed import: %v", err)
	}
	assertClosedAccounting(t, ledger)

	library, err := repository.Load()
	if err != nil {
		t.Fatal(err)
	}
	assertCanonicalFixture(t, library)

	// A fresh process must be able to read the same durable canonical and
	// structural state before attempting an exact envelope replay.
	restartedRepository, err := personalmemory.NewFileRepository(filepath.Join(root, "library"), fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	restartedLedger, err := ingestioncontroller.NewLedgerStore(filepath.Join(root, "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	if status, err := restartedRepository.Status(); err != nil || status.Fingerprint != library.Fingerprint {
		t.Fatalf("restart canonical readback = %#v, %v", status, err)
	}
	if persisted, err := restartedLedger.Load(); err != nil || persisted.State != "complete" || persisted.AggregateCommitment != ledger.AggregateCommitment {
		t.Fatalf("restart ledger readback = %#v, %v", persisted, err)
	}

	// Exact replay is a closed-envelope contract: it may not alter the
	// canonical fingerprint or turn a previously completed run incomplete.
	t.Run("exact replay", func(t *testing.T) {
		before := library.Fingerprint
		replayed, err := (ingestioncontroller.Controller{Repository: restartedRepository, Ledger: restartedLedger}).Apply(envelope)
		if err != nil {
			t.Errorf("exact replay rejected: %v", err)
			return
		}
		assertClosedAccounting(t, replayed)
		if replayed.State != "complete" || replayed.CanonicalAfterFingerprint != before {
			t.Errorf("exact replay receipt = %#v", replayed)
		}
		after, loadErr := restartedRepository.Load()
		if loadErr != nil || after.Fingerprint != before {
			t.Errorf("exact replay changed canonical state = %#v, %v", after, loadErr)
		}
	})

	queueLibrary, err := restartedRepository.Load()
	if err != nil {
		t.Fatal(err)
	}
	profile := resourcequeue.FixtureProfile()
	profile.Name = "fixture-closed-slack-recall"
	profile.MaxResources = 4
	profile = resourcequeue.SealProfile(profile)
	store, err := resourcequeue.NewStore(filepath.Join(root, "queue"), profile)
	if err != nil {
		t.Fatal(err)
	}
	resourceIDs := fixtureResourceIDs(queueLibrary)
	if len(resourceIDs) != 4 {
		t.Fatalf("safe resource set = %#v", queueLibrary.Resources)
	}
	if _, err := store.Enqueue(resourceIDs); err != nil {
		t.Fatal(err)
	}
	fetcher := &fixtureFetcher{}
	runner := resourcequeue.Runner{Store: store, Repository: restartedRepository, Fetcher: fetcher}
	if err := runner.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	queue, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	canonicalAfterDrain, err := restartedRepository.Load()
	if err != nil {
		t.Fatal(err)
	}
	unreachableID := assertQueueTerminalStates(t, queue, canonicalAfterDrain)

	// Queue state is derived and can be discarded/rebuilt without changing
	// canonical readback when the same terminal outcome is replayed.
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := resourcequeue.NewStore(filepath.Join(root, "queue"), profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rebuilt.Enqueue([]string{unreachableID}); err != nil {
		t.Fatal(err)
	}
	if err := (resourcequeue.Runner{Store: rebuilt, Repository: restartedRepository, Fetcher: unreachableFetcher{}}).Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	canonicalAfterRebuild, err := restartedRepository.Load()
	if err != nil {
		t.Fatal(err)
	}
	if canonicalAfterRebuild.Fingerprint != canonicalAfterDrain.Fingerprint {
		t.Fatalf("queue rebuild changed canonical fingerprint: %s != %s", canonicalAfterRebuild.Fingerprint, canonicalAfterDrain.Fingerprint)
	}

	retriever := personalmemory.NewLexicalRetriever(restartedRepository)
	compact, err := retriever.SearchCompact(personalmemory.SearchRequest{Query: "fixture retained parent", Limit: 3, RunID: "fixture-run"})
	if err != nil || compact.AnswerState != "answered" || len(compact.Citations) == 0 {
		t.Fatalf("compact search = %#v, %v", compact, err)
	}
	hydrated, err := retriever.Get(compact.Citations[0].RecordID)
	if err != nil || hydrated.Record.RecordID != compact.Citations[0].RecordID {
		t.Fatalf("explicit get = %#v, %v", hydrated, err)
	}
}

func fixtureNow() time.Time { return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC) }

func decodeFixtureEnvelope(t *testing.T) ingestioncontroller.Envelope {
	t.Helper()
	units := fixtureUnits()
	begin := ingestioncontroller.BeginFrame{
		Type: "begin", SchemaVersion: ingestioncontroller.RunSchemaVersion, RunID: "closed-slack-recall-fixture",
		SourceAdapter: "slack", SourceScope: "slack:fixture-workspace:fixture-channel",
		ConfigurationFingerprint: strings.Repeat("a", 64), UnitCount: len(units), MessageCeiling: 14, ByteCeiling: 1 << 20,
	}
	var stream bytes.Buffer
	writeFrame(t, &stream, begin)
	var unitBytes int64
	for _, unit := range units {
		encoded, err := json.Marshal(unit)
		if err != nil {
			t.Fatal(err)
		}
		unitBytes += int64(len(encoded) + 1)
		stream.Write(encoded)
		stream.WriteByte('\n')
	}
	writeFrame(t, &stream, ingestioncontroller.EndFrame{
		Type: "end", UnitCount: len(units), MessageCount: 14, ByteCount: unitBytes,
		EnvelopeCommitment: ingestioncontroller.EnvelopeCommitment(units),
	})
	envelope, err := ingestioncontroller.DecodeEnvelope(bytes.NewReader(stream.Bytes()))
	if err != nil {
		t.Fatalf("decode fixture envelope: %v", err)
	}
	return envelope
}

func writeFrame(t *testing.T, stream *bytes.Buffer, frame any) {
	t.Helper()
	encoded, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	stream.Write(encoded)
	stream.WriteByte('\n')
}

func fixtureUnits() []ingestioncontroller.UnitFrame {
	parent := fixtureMessage("1.000001", "fixture retained parent")
	original := fixtureMessage("1.000005", "fixture retained editable original")
	tombstoneOriginal := fixtureMessage("1.000006", "fixture retained soon deleted")
	return []ingestioncontroller.UnitFrame{
		fixtureUnit(0, "000", []acquisitionslack.NativeMessage{
			parent,
			fixtureMessage("1.000002", "fixture retained complete https://complete.example/fixture"),
			fixtureMessage("1.000003", ""), // objectively empty non-user transport artifact
			fixtureMessage("1.000004", "unknown author credential=https://secret.example/private"),
			original,
			tombstoneOriginal,
		}, map[string]string{
			"1.000001": "user", "1.000002": "user", "1.000003": "non_user", "1.000004": "unknown", "1.000005": "user", "1.000006": "user",
		}),
		fixtureUnit(1, "001", []acquisitionslack.NativeMessage{
			parent, // overlapping duplicate
			func() acquisitionslack.NativeMessage {
				m := fixtureMessage("1.000007", "fixture retained reply")
				m.ThreadParentID = "1.000001"
				return m
			}(),
			func() acquisitionslack.NativeMessage {
				m := original
				m.Text = "fixture retained editable revised"
				m.EditDeleteState = "edited"
				return m
			}(),
			func() acquisitionslack.NativeMessage {
				m := tombstoneOriginal
				m.EditDeleteState = "tombstone"
				return m
			}(),
			fixtureMessage("1.000008", "fixture retained partial https://partial.example/fixture"),
			fixtureMessage("1.000009", "fixture retained public unreachable https://unreachable.example/fixture"),
			fixtureMessage("1.000010", "fixture retained budget https://budget.example/fixture"),
			fixtureMessage("1.000011", "fixture retained ordinary evidence"),
		}, map[string]string{
			"1.000001": "user", "1.000005": "user", "1.000006": "user", "1.000007": "user", "1.000008": "user", "1.000009": "user", "1.000010": "user", "1.000011": "user",
		}),
	}
}

func fixtureUnit(ordinal int, descriptor string, messages []acquisitionslack.NativeMessage, classes map[string]string) ingestioncontroller.UnitFrame {
	return ingestioncontroller.UnitFrame{Type: "unit", Ordinal: ordinal, Descriptor: descriptor, AuthorClasses: classes,
		Batch: acquisitionslack.NativeBatch{SchemaVersion: acquisitionslack.NativeBatchSchema, WorkspaceID: "fixture-workspace", ChannelID: "fixture-channel",
			LowerInclusive: "1.000000", UpperInclusive: "2.000000", Watermark: "2.000000", IncludeThreads: true, IncludeReplies: true,
			PaginationExhausted: true, ThreadPaginationExhausted: true, DeclaredSourceRecords: len(messages), Messages: messages}}
}

func fixtureMessage(id, text string) acquisitionslack.NativeMessage {
	return acquisitionslack.NativeMessage{NativeMessageID: id, Timestamp: id, Text: text, EditDeleteState: "original"}
}

func assertClosedAccounting(t *testing.T, ledger ingestioncontroller.Ledger) {
	t.Helper()
	if ledger.State != "complete" || ledger.DeliveredCount != 14 || ledger.CanonicalDeclaredCount != 13 || ledger.StructuralExcludedCount != 1 ||
		ledger.OwnedCount != 11 || ledger.RetainedCount != 9 || ledger.WithheldCount != 1 || ledger.OverlapCount != 3 || ledger.ThreadCount != 1 || ledger.GapCount != 0 ||
		ledger.DeliveredCount != ledger.CanonicalDeclaredCount+ledger.StructuralExcludedCount || ledger.OwnedCount != ledger.RetainedCount+ledger.WithheldCount+ledger.StructuralExcludedCount {
		t.Fatalf("closed accounting = %#v", ledger)
	}
	userExclusions := 0
	for _, unit := range fixtureUnits() {
		for _, message := range unit.Batch.Messages {
			disposition, err := slackadapter.DispositionFor(message, unit.AuthorClasses[message.NativeMessageID])
			if err != nil {
				t.Fatal(err)
			}
			if disposition == slackadapter.DispositionExclude && unit.AuthorClasses[message.NativeMessageID] == "user" {
				userExclusions++
			}
		}
	}
	if userExclusions != 0 {
		t.Fatalf("user-authored structural exclusions = %d", userExclusions)
	}
}

func assertCanonicalFixture(t *testing.T, library personalmemory.Library) {
	t.Helper()
	if len(library.Records) != 10 || len(library.Revisions) != 2 || len(library.Resources) != 4 {
		t.Fatalf("canonical fixture cardinality = records:%d revisions:%d resources:%d", len(library.Records), len(library.Revisions), len(library.Resources))
	}
	encoded, err := json.Marshal(library)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret.example") || strings.Contains(string(encoded), "credential=") {
		t.Fatal("withheld secret URL persisted in canonical evidence")
	}
	byExternalID := map[string]personalmemory.CaptureRecord{}
	for _, record := range library.Records {
		byExternalID[record.ExternalID] = record
	}
	if _, exists := byExternalID["1.000003"]; exists {
		t.Fatal("objective non-user empty artifact entered canonical evidence")
	}
	withheld, exists := byExternalID["1.000004"]
	if !exists || withheld.RawText != "[Capture has no text]" || !contains(withheld.Missingness, "withheld_unknown_author") {
		t.Fatalf("withheld record = %#v", withheld)
	}
	if reply := byExternalID["1.000007"]; reply.ThreadParentID != "1.000001" {
		t.Fatalf("cross-unit reply closure = %#v", reply)
	}
	if edited := byExternalID["1.000005"]; edited.EditDeleteState != "edited" || edited.RawText != "fixture retained editable revised" {
		t.Fatalf("truthful edit current record = %#v", edited)
	}
	if tombstone := byExternalID["1.000006"]; tombstone.EditDeleteState != "tombstone" || tombstone.ContextState != "deleted_tombstone" {
		t.Fatalf("tombstone current record = %#v", tombstone)
	}
	urls := map[string]bool{}
	for _, resource := range library.Resources {
		urls[resource.CanonicalURL] = true
	}
	for _, want := range fixtureURLs {
		if !urls[want] {
			t.Fatalf("safe resource %q absent from canonical evidence: %#v", want, library.Resources)
		}
	}
}

func fixtureResourceIDs(library personalmemory.Library) []string {
	ids := make([]string, 0, len(library.Resources))
	for _, resource := range library.Resources {
		ids = append(ids, resource.ResourceID)
	}
	return ids
}

func assertQueueTerminalStates(t *testing.T, queue resourcequeue.Queue, library personalmemory.Library) string {
	t.Helper()
	resources := map[string]personalmemory.ResourceContext{}
	for _, resource := range library.Resources {
		resources[resource.ResourceID] = resource
	}
	statesByURL := map[string]string{}
	var unreachableID string
	for _, item := range queue.Items {
		key := item.State
		if item.Reason != "" {
			key += ":" + item.Reason
		}
		resource, exists := resources[item.ResourceID]
		if !exists {
			t.Fatalf("queue resource %q is absent from canonical evidence", item.ResourceID)
		}
		statesByURL[resource.CanonicalURL] = key
		if resource.CanonicalURL == fixtureUnreachableURL && key == "blocked:unreachable" {
			unreachableID = item.ResourceID
		}
	}
	want := map[string]string{
		fixtureCompleteURL:    "complete",
		fixturePartialURL:     "partial",
		fixtureUnreachableURL: "blocked:unreachable",
		fixtureBudgetURL:      "blocked:budget_exhausted",
	}
	for url, expected := range want {
		if statesByURL[url] != expected {
			t.Fatalf("resource queue terminal for %s = %q, want %q; all=%#v", url, statesByURL[url], expected, statesByURL)
		}
	}
	if unreachableID == "" {
		t.Fatalf("unreachable resource did not retain its expected terminal state: %#v", queue.Items)
	}
	canonical, err := libraryByID(library)
	if err != nil {
		t.Fatal(err)
	}
	for url, expected := range map[string]string{fixtureCompleteURL: "complete", fixturePartialURL: "partial", fixtureUnreachableURL: "failed", fixtureBudgetURL: "failed"} {
		resource := canonical[url]
		if resource.State != expected {
			t.Fatalf("canonical resource state for %s = %q, want %q", url, resource.State, expected)
		}
	}
	if !contains(canonical[fixtureUnreachableURL].Missingness, "resource_blocked:unreachable") || !contains(canonical[fixtureBudgetURL].Missingness, "resource_blocked:budget_exhausted") {
		t.Fatalf("canonical blocked reasons = unreachable:%#v budget:%#v", canonical[fixtureUnreachableURL], canonical[fixtureBudgetURL])
	}
	return unreachableID
}

func libraryByID(library personalmemory.Library) (map[string]personalmemory.ResourceContext, error) {
	byURL := make(map[string]personalmemory.ResourceContext, len(library.Resources))
	for _, resource := range library.Resources {
		if _, exists := byURL[resource.CanonicalURL]; exists {
			return nil, errors.New("fixture has duplicate canonical resource URL")
		}
		byURL[resource.CanonicalURL] = resource
	}
	return byURL, nil
}

type fixtureFetcher struct{}

func (fetcher *fixtureFetcher) Fetch(_ context.Context, target resourcequeue.Target) (resourcequeue.FetchResult, error) {
	switch target.CanonicalURL {
	case fixtureCompleteURL:
		return resourcequeue.FetchResult{State: resourcequeue.StateComplete, Usage: resourcequeue.Usage{Requests: 1},
			Evidence: acquisition.ImportedEvidence{State: "complete", Metadata: acquisition.ImportedMetadata{Title: "fixture complete"}},
			Content:  &personalmemory.ExtractedContent{CanonicalURL: target.CanonicalURL, MediaType: "text/plain", Completeness: "full", Text: "fixture retained complete resource", AccessClass: "public"}}, nil
	case fixturePartialURL:
		return resourcequeue.FetchResult{State: resourcequeue.StatePartial, Usage: resourcequeue.Usage{Requests: 1},
			Evidence: acquisition.ImportedEvidence{State: "partial", Metadata: acquisition.ImportedMetadata{Title: "fixture partial"}}}, nil
	case fixtureUnreachableURL:
		return resourcequeue.FetchResult{BlockedReason: "unreachable", Usage: resourcequeue.Usage{Requests: 1}}, errors.New("fixture unreachable public URL")
	case fixtureBudgetURL:
		// Full per-response exhaustion is a permanent resource outcome. Run-wide
		// aggregate exhaustion is covered separately by continuation fixtures.
		return resourcequeue.FetchResult{BlockedReason: resourcequeue.ReasonBudgetExhausted, Usage: resourcequeue.Usage{Requests: 1}}, nil
	default:
		return resourcequeue.FetchResult{}, errors.New("fixture fetcher received an unknown safe resource")
	}
}

type unreachableFetcher struct{}

func (unreachableFetcher) Fetch(context.Context, resourcequeue.Target) (resourcequeue.FetchResult, error) {
	return resourcequeue.FetchResult{BlockedReason: "unreachable", Usage: resourcequeue.Usage{Requests: 1}}, errors.New("fixture unreachable public URL")
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

const (
	fixtureCompleteURL    = "https://complete.example/fixture"
	fixturePartialURL     = "https://partial.example/fixture"
	fixtureUnreachableURL = "https://unreachable.example/fixture"
	fixtureBudgetURL      = "https://budget.example/fixture"
)

var fixtureURLs = []string{fixtureCompleteURL, fixturePartialURL, fixtureUnreachableURL, fixtureBudgetURL}
