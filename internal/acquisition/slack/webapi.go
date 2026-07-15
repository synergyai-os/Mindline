package slack

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/assurance"
)

const WebAPIAdapterVersion = "slack_web_api/v1"

var webAPITimestampPattern = regexp.MustCompile(`^[0-9]{1,16}\.[0-9]{1,9}$`)

type WebAPIIdentity struct {
	WorkspaceID string
	ChannelID   string
}

type WebAPIMessage struct {
	Timestamp        string
	ThreadTimestamp  string
	Text             string
	ReplyCount       int
	Subtype          string
	FileCount        int
	PrivateFileCount int
}

type WebAPIPage struct {
	Messages   []WebAPIMessage
	NextCursor string
}

// WebAPIClient is the read-only provider port. Production clients must pin the
// Slack origin and expose only auth.test, conversations.history, and
// conversations.replies through this boundary.
type WebAPIClient interface {
	Probe(context.Context) (string, error)
	History(context.Context, string, string, string, string, int) (WebAPIPage, error)
	Replies(context.Context, string, string, string, string, string, int) (WebAPIPage, error)
}

type SyntheticWebAPIClient interface {
	WebAPIClient
	IsSynthetic() bool
}

type WebAPIDrain struct {
	Client          WebAPIClient
	CheckpointStore WebAPICheckpointStore
}

func (drain WebAPIDrain) DrainSynthetic(ctx context.Context, identity WebAPIIdentity, oldest, latest string) (ExternalManifest, error) {
	client, ok := drain.Client.(SyntheticWebAPIClient)
	if !ok || !client.IsSynthetic() {
		return ExternalManifest{}, errors.New("synthetic Slack drain requires an explicit synthetic client")
	}
	if drain.CheckpointStore == nil {
		drain.CheckpointStore = newMemoryWebAPICheckpointStore()
	}
	input, scope, err := drain.collect(ctx, identity, oldest, latest)
	if err != nil {
		return ExternalManifest{}, err
	}
	input.DataClass = DataClassSynthetic
	manifest, err := BuildExternalManifest(input)
	if err == nil {
		err = drain.CheckpointStore.Clear(ctx, scope)
	}
	return manifest, err
}

func (drain WebAPIDrain) DrainAuthorized(ctx context.Context, identity WebAPIIdentity, oldest, latest string, receipt assurance.Receipt, commit, configuration string) (ExternalManifest, error) {
	if err := assurance.Validate(receipt, commit, configuration); err != nil {
		return ExternalManifest{}, err
	}
	if drain.CheckpointStore == nil {
		return ExternalManifest{}, errors.New("authorized Slack drain requires a durable checkpoint store")
	}
	input, _, err := drain.collect(ctx, identity, oldest, latest)
	if err != nil {
		return ExternalManifest{}, err
	}
	input.DataClass = DataClassPrivateRuntime
	// The completed checkpoint remains durable until the caller has durably
	// adopted the manifest. Building a manifest is not itself adoption.
	return BuildAuthorizedExternalManifest(input, receipt, commit, configuration)
}

func (drain WebAPIDrain) collect(ctx context.Context, identity WebAPIIdentity, oldest, latest string) (BuildInput, WebAPICheckpointScope, error) {
	scope := WebAPICheckpointScope{WorkspaceID: identity.WorkspaceID, ChannelID: identity.ChannelID, Oldest: oldest, Latest: latest}
	if drain.Client == nil || strings.TrimSpace(identity.WorkspaceID) == "" || strings.TrimSpace(identity.ChannelID) == "" || !validWebAPIWindow(oldest, latest) {
		return BuildInput{}, scope, errors.New("invalid Slack Web API drain scope")
	}
	workspaceID, err := drain.Client.Probe(ctx)
	if err != nil || workspaceID != identity.WorkspaceID {
		return BuildInput{}, scope, errors.New("Slack Web API workspace identity mismatch")
	}
	previous, found, err := drain.CheckpointStore.Load(ctx, scope)
	if err != nil {
		return BuildInput{}, scope, err
	}
	attempt, completedPages := 1, 0
	if found {
		attempt = previous.Attempt + 1
		completedPages = previous.CompletedPages
	}
	checkpoint := sealWebAPICheckpoint(WebAPICheckpoint{
		Scope: scope, Phase: "history", Attempt: attempt,
		CursorFingerprint: checkpointCursorFingerprint("history", ""), CompletedPages: completedPages,
	})
	if err := drain.CheckpointStore.Save(ctx, checkpoint); err != nil {
		return BuildInput{}, scope, err
	}
	parents, err := drain.collectHistory(ctx, &checkpoint)
	if err != nil {
		return BuildInput{}, scope, err
	}
	replies, err := drain.collectReplies(ctx, &checkpoint, parents)
	if err != nil {
		return BuildInput{}, scope, err
	}
	sort.Slice(parents, func(left, right int) bool {
		return compareWebAPITimestamp(parents[left].Timestamp, parents[right].Timestamp) < 0
	})
	byTimestamp := map[string]NativeMessage{}
	for _, parent := range parents {
		if _, duplicate := byTimestamp[parent.Timestamp]; duplicate {
			return BuildInput{}, scope, errors.New("Slack Web API returned duplicate history message identity")
		}
		byTimestamp[parent.Timestamp] = nativeWebAPIMessage(parent, "")
		if parent.ReplyCount <= 0 {
			continue
		}
		threadReplies := replies[parent.Timestamp]
		replyCount := 0
		for _, reply := range threadReplies {
			if reply.Timestamp == parent.Timestamp {
				continue
			}
			if !webAPITimestampInWindow(reply.Timestamp, oldest, latest) {
				continue
			}
			replyCount++
			if _, duplicate := byTimestamp[reply.Timestamp]; duplicate {
				return BuildInput{}, scope, errors.New("Slack Web API returned duplicate message identity")
			}
			byTimestamp[reply.Timestamp] = nativeWebAPIMessage(reply, "slack:"+parent.Timestamp)
		}
		if replyCount != parent.ReplyCount {
			return BuildInput{}, scope, errors.New("Slack Web API thread reply accounting mismatch")
		}
	}
	timestamps := make([]string, 0, len(byTimestamp))
	for timestamp := range byTimestamp {
		timestamps = append(timestamps, timestamp)
	}
	sort.Slice(timestamps, func(left, right int) bool { return compareWebAPITimestamp(timestamps[left], timestamps[right]) < 0 })
	input := BuildInput{ConnectorKind: "slack_web_api", AdapterVersion: WebAPIAdapterVersion, WorkspaceID: identity.WorkspaceID, ChannelID: identity.ChannelID, LowerInclusive: oldest, UpperInclusive: latest, Watermark: latest}
	for _, timestamp := range timestamps {
		input.Messages = append(input.Messages, byTimestamp[timestamp])
	}
	if len(input.Messages) == 0 {
		return BuildInput{}, scope, errors.New("Slack Web API drain returned no source records")
	}
	return input, scope, nil
}

func checkpointCursorFingerprint(phase, cursor string) string {
	return acquisition.Fingerprint(struct {
		Phase  string `json:"phase"`
		Cursor string `json:"cursor"`
	}{Phase: phase, Cursor: cursor})
}

func (drain WebAPIDrain) collectHistory(ctx context.Context, checkpoint *WebAPICheckpoint) ([]WebAPIMessage, error) {
	var history []WebAPIMessage
	cursor := ""
	seen := map[string]bool{"": true}
	for checkpoint.CompletedPages < 10_000 {
		page, err := drain.Client.History(ctx, checkpoint.Scope.ChannelID, checkpoint.Scope.Oldest, checkpoint.Scope.Latest, cursor, 200)
		if err != nil {
			return nil, err
		}
		for _, message := range page.Messages {
			if err := validateWebAPIMessage(message); err != nil {
				return nil, err
			}
			if webAPITimestampInWindow(message.Timestamp, checkpoint.Scope.Oldest, checkpoint.Scope.Latest) {
				history = append(history, message)
			}
		}
		next := strings.TrimSpace(page.NextCursor)
		checkpoint.HistoryRecords = len(history)
		checkpoint.AttemptPages++
		checkpoint.CompletedPages++
		if next == "" {
			checkpoint.Phase = "replies"
			checkpoint.CursorFingerprint = checkpointCursorFingerprint("replies", "")
			*checkpoint = sealWebAPICheckpoint(*checkpoint)
			return history, drain.CheckpointStore.Save(ctx, *checkpoint)
		}
		if seen[next] {
			return nil, errors.New("Slack Web API cursor cycle")
		}
		seen[next], cursor = true, next
		checkpoint.CursorFingerprint = checkpointCursorFingerprint("history", cursor)
		*checkpoint = sealWebAPICheckpoint(*checkpoint)
		if err := drain.CheckpointStore.Save(ctx, *checkpoint); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("Slack Web API page budget exceeded")
}

func (drain WebAPIDrain) collectReplies(ctx context.Context, checkpoint *WebAPICheckpoint, parents []WebAPIMessage) (map[string][]WebAPIMessage, error) {
	replies := map[string][]WebAPIMessage{}
	parents = append([]WebAPIMessage(nil), parents...)
	sort.Slice(parents, func(left, right int) bool {
		return compareWebAPITimestamp(parents[left].Timestamp, parents[right].Timestamp) < 0
	})
	for index, parent := range parents {
		checkpoint.CompletedThreads = index
		if parent.ReplyCount <= 0 {
			checkpoint.CompletedThreads++
			*checkpoint = sealWebAPICheckpoint(*checkpoint)
			if err := drain.CheckpointStore.Save(ctx, *checkpoint); err != nil {
				return nil, err
			}
			continue
		}
		cursor := ""
		seen := map[string]bool{"": true}
		checkpoint.ThreadFingerprint = checkpointCursorFingerprint("thread", parent.Timestamp)
		for {
			page, err := drain.Client.Replies(ctx, checkpoint.Scope.ChannelID, parent.Timestamp, checkpoint.Scope.Oldest, checkpoint.Scope.Latest, cursor, 200)
			if err != nil {
				return nil, err
			}
			for _, message := range page.Messages {
				if err := validateWebAPIMessage(message); err != nil {
					return nil, err
				}
				replies[parent.Timestamp] = append(replies[parent.Timestamp], message)
			}
			next := strings.TrimSpace(page.NextCursor)
			checkpoint.ReplyRecords = 0
			for _, values := range replies {
				checkpoint.ReplyRecords += len(values)
			}
			checkpoint.AttemptPages++
			checkpoint.CompletedPages++
			if checkpoint.CompletedPages >= 10_000 && next != "" {
				return nil, errors.New("Slack Web API reply page budget exceeded")
			}
			if next == "" {
				checkpoint.CompletedThreads++
				checkpoint.ThreadFingerprint = ""
				checkpoint.CursorFingerprint = checkpointCursorFingerprint("replies", "")
				*checkpoint = sealWebAPICheckpoint(*checkpoint)
				if err := drain.CheckpointStore.Save(ctx, *checkpoint); err != nil {
					return nil, err
				}
				break
			}
			if seen[next] {
				return nil, errors.New("Slack Web API reply cursor cycle")
			}
			seen[next], cursor = true, next
			checkpoint.CursorFingerprint = checkpointCursorFingerprint("replies", cursor)
			*checkpoint = sealWebAPICheckpoint(*checkpoint)
			if err := drain.CheckpointStore.Save(ctx, *checkpoint); err != nil {
				return nil, err
			}
		}
	}
	checkpoint.Phase = "complete"
	checkpoint.ThreadFingerprint = ""
	checkpoint.CursorFingerprint = checkpointCursorFingerprint("complete", "")
	*checkpoint = sealWebAPICheckpoint(*checkpoint)
	return replies, drain.CheckpointStore.Save(ctx, *checkpoint)
}

func validateWebAPIMessage(message WebAPIMessage) error {
	if !webAPITimestampPattern.MatchString(message.Timestamp) || message.ReplyCount < 0 || message.FileCount < 0 || message.PrivateFileCount < 0 || message.PrivateFileCount > message.FileCount {
		return errors.New("invalid Slack Web API message")
	}
	return nil
}

func validWebAPIWindow(oldest, latest string) bool {
	if !webAPITimestampPattern.MatchString(oldest) || !webAPITimestampPattern.MatchString(latest) {
		return false
	}
	return compareWebAPITimestamp(oldest, latest) <= 0
}

func webAPITimestampInWindow(value, oldest, latest string) bool {
	return webAPITimestampPattern.MatchString(value) && compareWebAPITimestamp(value, oldest) >= 0 && compareWebAPITimestamp(value, latest) <= 0
}

func compareWebAPITimestamp(left, right string) int {
	leftValue, leftOK := new(big.Rat).SetString(left)
	rightValue, rightOK := new(big.Rat).SetString(right)
	if !leftOK || !rightOK {
		return strings.Compare(left, right)
	}
	return leftValue.Cmp(rightValue)
}

func nativeWebAPIMessage(message WebAPIMessage, parent string) NativeMessage {
	state := "original"
	if message.Subtype == "message_deleted" {
		state = "deleted"
	}
	if message.Subtype == "message_changed" {
		state = "edited"
	}
	return NativeMessage{NativeMessageID: fmt.Sprintf("slack:%s", message.Timestamp), Timestamp: message.Timestamp, ThreadParentID: parent, Text: message.Text, EditDeleteState: state, AttachmentCount: message.FileCount, PrivateFileCount: message.PrivateFileCount}
}
