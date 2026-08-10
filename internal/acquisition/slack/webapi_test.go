package slack

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

type syntheticWebAPIClient struct {
	workspace      string
	history        map[string]WebAPIPage
	replies        map[string]map[string]WebAPIPage
	historyFailure map[string]int
	historyCalls   map[string]int
}

func (*syntheticWebAPIClient) IsSynthetic() bool { return true }

func (client *syntheticWebAPIClient) Probe(context.Context) (string, error) {
	return client.workspace, nil
}
func (client *syntheticWebAPIClient) History(_ context.Context, channel, oldest, latest, cursor string, limit int) (WebAPIPage, error) {
	if channel != "C-proof" || oldest != "100.000001" || latest != "199.000001" || limit != 200 {
		return WebAPIPage{}, errors.New("scope drift")
	}
	if client.historyCalls == nil {
		client.historyCalls = map[string]int{}
	}
	client.historyCalls[cursor]++
	if client.historyFailure[cursor] > 0 {
		client.historyFailure[cursor]--
		return WebAPIPage{}, errors.New("synthetic history interruption")
	}
	return client.history[cursor], nil
}
func (client *syntheticWebAPIClient) Replies(_ context.Context, channel, thread, oldest, latest, cursor string, limit int) (WebAPIPage, error) {
	if channel != "C-proof" || oldest != "100.000001" || latest != "199.000001" || limit != 200 {
		return WebAPIPage{}, errors.New("reply scope drift")
	}
	return client.replies[thread][cursor], nil
}

func TestWebAPIDrainRestartsFrozenWindowWithoutPersistingContentOrRawCursor(t *testing.T) {
	store, err := NewFileWebAPICheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := &syntheticWebAPIClient{
		workspace: "T-proof",
		history: map[string]WebAPIPage{
			"":          {Messages: []WebAPIMessage{{Timestamp: "120.000001", Text: "https://example.com/a"}}, NextCursor: "history-2"},
			"history-2": {Messages: []WebAPIMessage{{Timestamp: "140.000001", Text: "https://example.com/b"}}},
		},
		historyFailure: map[string]int{"history-2": 1},
	}
	drain := WebAPIDrain{Client: first, CheckpointStore: store}
	identity := WebAPIIdentity{WorkspaceID: "T-proof", ChannelID: "C-proof"}
	if _, err := drain.DrainSynthetic(context.Background(), identity, "100.000001", "199.000001"); err == nil {
		t.Fatal("interrupted drain unexpectedly completed")
	}
	scope := WebAPICheckpointScope{WorkspaceID: "T-proof", ChannelID: "C-proof", Oldest: "100.000001", Latest: "199.000001"}
	checkpoint, found, err := store.Load(context.Background(), scope)
	if err != nil || !found || checkpoint.Attempt != 1 || len(checkpoint.CursorFingerprint) != 64 || checkpoint.HistoryRecords != 1 {
		t.Fatalf("durable checkpoint missing after interruption: found=%v checkpoint=%+v err=%v", found, checkpoint, err)
	}
	checkpointBytes, err := os.ReadFile(store.path(scope))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"history-2", "https://example.com/a", `"text"`, `"history_cursor"`, `"reply_cursor"`} {
		if strings.Contains(string(checkpointBytes), forbidden) {
			t.Fatalf("checkpoint persisted private content or raw provider state %q: %s", forbidden, checkpointBytes)
		}
	}
	second := &syntheticWebAPIClient{workspace: "T-proof", history: first.history}
	drain.Client = second
	manifest, err := drain.DrainSynthetic(context.Background(), identity, "100.000001", "199.000001")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.SourceRecords) != 2 || second.historyCalls[""] != 1 || second.historyCalls["history-2"] != 1 {
		t.Fatalf("drain did not safely restart the exact frozen window: records=%d calls=%v", len(manifest.SourceRecords), second.historyCalls)
	}
	if _, found, err := store.Load(context.Background(), scope); err != nil || found {
		t.Fatalf("completed checkpoint was not cleared: found=%v err=%v", found, err)
	}
}

func TestAuthorizedWebAPIDrainRetainsCompletedCheckpointUntilCallerAdopts(t *testing.T) {
	store, err := NewFileWebAPICheckpointStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := &syntheticWebAPIClient{workspace: "T-proof", history: map[string]WebAPIPage{"": {Messages: []WebAPIMessage{{Timestamp: "120.000001", Text: "https://example.com/private"}}}}}
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	identity := WebAPIIdentity{WorkspaceID: "T-proof", ChannelID: "C-proof"}
	if _, err := (WebAPIDrain{Client: client, CheckpointStore: store}).DrainAuthorized(context.Background(), identity, "100.000001", "199.000001", authorizedTestReceipt(t, now), "commit-1", "config-1"); err != nil {
		t.Fatal(err)
	}
	scope := WebAPICheckpointScope{WorkspaceID: "T-proof", ChannelID: "C-proof", Oldest: "100.000001", Latest: "199.000001"}
	checkpoint, found, err := store.Load(context.Background(), scope)
	if err != nil || !found || checkpoint.Phase != "complete" || checkpoint.HistoryRecords != 1 {
		t.Fatalf("completed checkpoint was cleared before adoption: found=%v checkpoint=%+v err=%v", found, checkpoint, err)
	}
}

func TestWebAPIDrainProvesPaginationRepliesAndOccurrenceCompleteness(t *testing.T) {
	client := &syntheticWebAPIClient{
		workspace: "T-proof",
		history: map[string]WebAPIPage{
			"":          {Messages: []WebAPIMessage{{Timestamp: "120.000001", Text: "one https://example.com/a", ReplyCount: 1}}, NextCursor: "history-2"},
			"history-2": {Messages: []WebAPIMessage{{Timestamp: "140.000001", Text: "duplicate https://example.com/a and https://github.com/acme/tool"}}},
		},
		replies: map[string]map[string]WebAPIPage{
			"120.000001": {
				"":        {Messages: []WebAPIMessage{{Timestamp: "120.000001", Text: "parent"}}, NextCursor: "reply-2"},
				"reply-2": {Messages: []WebAPIMessage{{Timestamp: "125.000001", ThreadTimestamp: "120.000001", Text: "reply https://youtu.be/video"}}},
			},
		},
	}
	manifest, err := (WebAPIDrain{Client: client}).DrainSynthetic(context.Background(), WebAPIIdentity{WorkspaceID: "T-proof", ChannelID: "C-proof"}, "100.000001", "199.000001")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.SourceRecords) != 3 || len(manifest.URLOccurrences) != 4 || len(manifest.CanonicalItems) != 3 {
		t.Fatalf("occurrence-complete synthetic drain mismatch: records=%d occurrences=%d canonical=%d", len(manifest.SourceRecords), len(manifest.URLOccurrences), len(manifest.CanonicalItems))
	}
	if manifest.DataClass != DataClassSynthetic || manifest.SourceRecords[1].ThreadParentID != "slack:120.000001" {
		t.Fatalf("thread or data-class authority drifted: %+v", manifest.SourceRecords)
	}
}

func TestWebAPIDrainRejectsIdentityAndCursorCycles(t *testing.T) {
	client := &syntheticWebAPIClient{workspace: "wrong", history: map[string]WebAPIPage{}}
	if _, err := (WebAPIDrain{Client: client}).DrainSynthetic(context.Background(), WebAPIIdentity{WorkspaceID: "T-proof", ChannelID: "C-proof"}, "100.000001", "199.000001"); err == nil {
		t.Fatal("workspace identity drift was accepted")
	}
	client.workspace = "T-proof"
	client.history = map[string]WebAPIPage{"": {NextCursor: "cycle"}, "cycle": {NextCursor: "cycle"}}
	if _, err := (WebAPIDrain{Client: client}).DrainSynthetic(context.Background(), WebAPIIdentity{WorkspaceID: "T-proof", ChannelID: "C-proof"}, "100.000001", "199.000001"); err == nil {
		t.Fatal("cursor cycle was accepted")
	}
}

func TestWebAPIDrainRejectsDuplicatePagesOmittedRepliesAndAccountsEditsDeletesFiles(t *testing.T) {
	client := &syntheticWebAPIClient{workspace: "T-proof", history: map[string]WebAPIPage{
		"":     {Messages: []WebAPIMessage{{Timestamp: "120.000001", ReplyCount: 1, Subtype: "message_changed", FileCount: 1, PrivateFileCount: 1}}, NextCursor: "next"},
		"next": {Messages: []WebAPIMessage{{Timestamp: "120.000001"}}},
	}, replies: map[string]map[string]WebAPIPage{"120.000001": {"": {Messages: []WebAPIMessage{{Timestamp: "120.000001"}}}}}}
	if _, err := (WebAPIDrain{Client: client}).DrainSynthetic(context.Background(), WebAPIIdentity{WorkspaceID: "T-proof", ChannelID: "C-proof"}, "100.000001", "199.000001"); err == nil {
		t.Fatal("duplicate history page was accepted")
	}
	client.history = map[string]WebAPIPage{"": {Messages: []WebAPIMessage{{Timestamp: "120.000001", ReplyCount: 1, Subtype: "message_deleted", FileCount: 1, PrivateFileCount: 1}}}}
	if _, err := (WebAPIDrain{Client: client}).DrainSynthetic(context.Background(), WebAPIIdentity{WorkspaceID: "T-proof", ChannelID: "C-proof"}, "100.000001", "199.000001"); err == nil {
		t.Fatal("omitted declared reply was accepted")
	}
	client.history = map[string]WebAPIPage{"": {Messages: []WebAPIMessage{{Timestamp: "120.000001", RevisionTimestamp: "121.000001"}}}}
	manifest, err := (WebAPIDrain{Client: client}).DrainSynthetic(context.Background(), WebAPIIdentity{WorkspaceID: "T-proof", ChannelID: "C-proof"}, "100.000001", "199.000001")
	if err != nil || manifest.SourceRecords[0].EditDeleteState != "edited" {
		t.Fatalf("provider edit chronology was not classified as edited: %+v err=%v", manifest.SourceRecords, err)
	}
	client.history = map[string]WebAPIPage{"": {Messages: []WebAPIMessage{{Timestamp: "120.000001", Subtype: "message_deleted", FileCount: 1, PrivateFileCount: 1}}}}
	manifest, err = (WebAPIDrain{Client: client}).DrainSynthetic(context.Background(), WebAPIIdentity{WorkspaceID: "T-proof", ChannelID: "C-proof"}, "100.000001", "199.000001")
	if err != nil || manifest.SourceRecords[0].EditDeleteState != "deleted" || manifest.SourceRecords[0].AttachmentCount != 1 || manifest.SourceRecords[0].PrivateFileCount != 1 {
		t.Fatalf("edit/delete/file accounting drifted: %+v err=%v", manifest.SourceRecords, err)
	}
}
