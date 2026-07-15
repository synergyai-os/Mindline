package activationapp

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/integrations"
	"github.com/synergyai-os/Mindline/internal/privateio"
)

func (app *App) ConnectSlackSource(ctx context.Context, credential []byte, channelID string) (any, error) {
	if len(credential) == 0 || !slackChannelIDPattern.MatchString(channelID) {
		return nil, errors.New("Slack source connection blocked")
	}
	app.mu.Lock()
	if !app.transportAuthorizedLocked() || app.state.DrainPolicy == nil {
		app.mu.Unlock()
		return nil, errors.New("Slack source connection blocked")
	}
	budgets := slackBudgetsFromPolicy(*app.state.DrainPolicy)
	generation := app.sourceGeneration
	var expectedWorkspace, expectedChannel string
	if app.state.KnownSource != nil {
		expectedWorkspace = app.state.KnownSource.Identity.WorkspaceID
		expectedChannel = app.state.KnownSource.Identity.ChannelID
	}
	app.mu.Unlock()

	connection, err := app.sourceConnector.Connect(ctx, app.registry, credential, channelID, budgets)
	if err != nil {
		return nil, err
	}
	closeCandidate := func() {
		if connection != nil && connection.Disconnect != nil {
			_ = connection.Disconnect()
		}
	}
	if connection == nil || connection.Client == nil || connection.Snapshot.Identity.Provider != "slack" || connection.Snapshot.Identity.ChannelID != channelID || connection.Snapshot.Identity.CapabilityVersion != acquisitionslack.WebAPIAdapterVersion {
		closeCandidate()
		return nil, errors.New("Slack source capability missing")
	}
	if expectedWorkspace != "" && expectedWorkspace != connection.Snapshot.Identity.WorkspaceID {
		closeCandidate()
		return nil, errors.New("Slack source workspace changed; start a new activation run")
	}
	if expectedChannel != "" && expectedChannel != connection.Snapshot.Identity.ChannelID {
		closeCandidate()
		return nil, errors.New("Slack source channel changed; start a new activation run")
	}

	app.mu.Lock()
	defer app.mu.Unlock()
	if app.sourceGeneration != generation || !app.transportAuthorizedLocked() || app.state.Sample != nil {
		closeCandidate()
		return nil, errors.New("Slack source connection state changed during verification")
	}
	if app.sourceConnection != nil && app.sourceConnection.Disconnect != nil {
		_ = app.sourceConnection.Disconnect()
	}
	app.sourceConnection = connection
	app.sourceGeneration++
	app.state.KnownSource = &connection.Snapshot
	if err := app.commitAuthorityLocked(ctx, "source_connection", acquisition.Fingerprint(connection.Snapshot)); err != nil {
		return nil, err
	}
	return map[string]any{"connected": true, "identity": connection.Snapshot.Identity, "session_only": true}, nil
}

func (app *App) DrainSlackSource(ctx context.Context, oldest, latest string) (any, error) {
	app.mu.Lock()
	connection := app.sourceConnection
	if connection == nil || app.receipt == nil || !app.transportAuthorizedLocked() || app.state.Sample != nil {
		app.mu.Unlock()
		return nil, errors.New("Slack source drain blocked")
	}
	identity := connection.Snapshot.Identity
	window := app.state.SlackDrainWindow
	if window == nil {
		reserved, err := newSlackDrainWindow(identity, oldest, latest, app.now().UTC())
		if err != nil {
			app.mu.Unlock()
			return nil, err
		}
		app.state.SlackDrainWindow = &reserved
		if err := app.commitAuthorityLocked(ctx, "source_window", reserved.Fingerprint); err != nil {
			app.mu.Unlock()
			return nil, err
		}
		window = &reserved
	} else if err := matchSlackDrainWindow(*window, identity, oldest, latest); err != nil {
		app.mu.Unlock()
		return nil, err
	}
	receipt := *app.receipt
	policy := *app.state.DrainPolicy
	commit, configuration, now, maxAge := app.commit, app.configuration, app.now(), app.receiptMaxAge
	app.mu.Unlock()

	checkpointStore, err := acquisitionslack.NewFileWebAPICheckpointStore(filepath.Join(app.runtimeRoot, "slack-web-api-checkpoints"))
	if err != nil {
		return nil, err
	}
	scope := acquisitionslack.WebAPICheckpointScope{WorkspaceID: identity.WorkspaceID, ChannelID: identity.ChannelID, Oldest: window.Oldest, Latest: window.Latest}
	client := connection.Client
	if connection.SessionRef != "" {
		budgets := slackBudgetsFromPolicy(policy)
		budgetStore, budgetErr := acquisitionslack.NewFileSlackBudgetStore(
			filepath.Join(app.runtimeRoot, "slack-web-api-budgets"),
			acquisitionslack.SlackBudgetScope{WorkspaceID: identity.WorkspaceID, ChannelID: identity.ChannelID, Oldest: window.Oldest, Latest: window.Latest},
			budgets, now,
		)
		if budgetErr != nil {
			return nil, budgetErr
		}
		durableClient, clientErr := acquisitionslack.NewDurablyBudgetedLeasedSlackHTTPClient(app.registry, connection.SessionRef, identity, budgets, budgetStore)
		if clientErr != nil {
			return nil, clientErr
		}
		defer durableClient.Close()
		client = durableClient
	}
	manifest, err := (acquisitionslack.WebAPIDrain{Client: client, CheckpointStore: checkpointStore}).DrainAuthorized(
		ctx, acquisitionslack.WebAPIIdentity{WorkspaceID: identity.WorkspaceID, ChannelID: identity.ChannelID}, window.Oldest, window.Latest,
		receipt, commit, configuration, now, maxAge,
	)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(app.runtimeRoot, "slack-web-api-import.json")
	if err := privateio.WriteJSON(path, manifest); err != nil {
		return nil, err
	}
	result, err := app.ImportExternalInventory(ctx, path, "slack-web-api-import.json")
	if err != nil {
		return nil, err
	}
	// Adoption is the authority boundary for clearing restart progress. A crash
	// before this point leaves the completed, content-free checkpoint intact.
	if err := checkpointStore.Clear(ctx, scope); err != nil {
		return nil, err
	}
	return result, nil
}

func newSlackDrainWindow(identity integrations.VerifiedIdentity, oldest, latest string, now time.Time) (SlackDrainWindow, error) {
	oldest = strings.TrimSpace(oldest)
	latest = strings.TrimSpace(latest)
	if oldest == "" {
		oldest = "0.000001"
	}
	if latest == "" {
		latest = fmt.Sprintf("%d.%06d", now.Unix(), now.Nanosecond()/1_000)
	}
	window := SlackDrainWindow{
		SchemaVersion: SlackDrainWindowSchema, WorkspaceID: identity.WorkspaceID, ChannelID: identity.ChannelID,
		Oldest: oldest, Latest: latest, ReservedAt: now.Format(time.RFC3339Nano),
	}
	window.Fingerprint = acquisition.Fingerprint(window)
	if err := validateSlackDrainWindow(window); err != nil {
		return SlackDrainWindow{}, err
	}
	return window, nil
}

func matchSlackDrainWindow(window SlackDrainWindow, identity integrations.VerifiedIdentity, oldest, latest string) error {
	if err := validateSlackDrainWindow(window); err != nil {
		return err
	}
	if window.WorkspaceID != identity.WorkspaceID || window.ChannelID != identity.ChannelID {
		return errors.New("Slack source identity differs from the reserved drain window")
	}
	if strings.TrimSpace(oldest) != "" && strings.TrimSpace(oldest) != window.Oldest || strings.TrimSpace(latest) != "" && strings.TrimSpace(latest) != window.Latest {
		return errors.New("Slack drain window is already reserved; retry the exact window")
	}
	return nil
}

func validateSlackDrainWindow(window SlackDrainWindow) error {
	fingerprint := window.Fingerprint
	window.Fingerprint = ""
	oldest, oldestOK := new(big.Rat).SetString(window.Oldest)
	latest, latestOK := new(big.Rat).SetString(window.Latest)
	reservedAt, timeErr := time.Parse(time.RFC3339Nano, window.ReservedAt)
	if window.SchemaVersion != SlackDrainWindowSchema || fingerprint == "" || fingerprint != acquisition.Fingerprint(window) ||
		!slackTimestampPattern.MatchString(window.Oldest) || !slackTimestampPattern.MatchString(window.Latest) ||
		!oldestOK || !latestOK || oldest.Sign() < 0 || latest.Cmp(oldest) < 0 || timeErr != nil || reservedAt.IsZero() ||
		strings.TrimSpace(window.WorkspaceID) == "" || !slackChannelIDPattern.MatchString(window.ChannelID) {
		return errors.New("Slack drain window is invalid")
	}
	return nil
}

func slackBudgetsFromPolicy(policy DrainPolicy) acquisitionslack.SlackHTTPBudgets {
	budgets := acquisitionslack.DefaultSlackHTTPBudgets()
	if policy.MaximumNetworkRequests < budgets.MaximumRequests {
		budgets.MaximumRequests = policy.MaximumNetworkRequests
	}
	if policy.MaximumRetryAttempts < budgets.MaximumRetries {
		budgets.MaximumRetries = policy.MaximumRetryAttempts
	}
	wall := time.Duration(policy.MaximumWallTimeSeconds) * time.Second
	if wall < budgets.MaximumWallTime {
		budgets.MaximumWallTime = wall
	}
	if policy.MaximumCostMicrounits < budgets.MaximumCostMicrounits {
		budgets.MaximumCostMicrounits = policy.MaximumCostMicrounits
	}
	return budgets
}

func (app *App) DisconnectSlackSource() (any, error) {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.sourceConnection != nil && app.sourceConnection.Disconnect != nil {
		_ = app.sourceConnection.Disconnect()
	}
	app.sourceConnection = nil
	app.sourceGeneration++
	return map[string]any{"disconnected": true}, nil
}
