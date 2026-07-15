package activationapp

import (
	"context"
	"errors"
	"regexp"

	acquisitionslack "github.com/synergyai-os/Mindline/internal/acquisition/slack"
	"github.com/synergyai-os/Mindline/internal/integrations"
)

var (
	slackChannelIDPattern = regexp.MustCompile(`^[A-Z0-9]{2,64}$`)
	slackTimestampPattern = regexp.MustCompile(`^[0-9]{1,16}\.[0-9]{1,9}$`)
)

type productionSlackSourceConnector struct{}

func (productionSlackSourceConnector) Connect(ctx context.Context, registry *integrations.Registry, credential []byte, channelID string, budgets acquisitionslack.SlackHTTPBudgets) (*SlackSourceConnection, error) {
	if registry == nil || !slackChannelIDPattern.MatchString(channelID) {
		return nil, errors.New("invalid Slack source connection")
	}
	probe, err := acquisitionslack.NewSlackHTTPClient(credential, budgets)
	if err != nil {
		return nil, err
	}
	workspaceID, err := probe.Probe(ctx)
	probe.Close()
	if err != nil {
		return nil, err
	}
	identity := integrations.VerifiedIdentity{
		Provider: "slack", WorkspaceID: workspaceID, ChannelID: channelID,
		CapabilityVersion: acquisitionslack.WebAPIAdapterVersion,
	}
	ref, snapshot, err := registry.Register(integrations.LeaseOptions{
		Kind: integrations.ConnectionSlackWebAPI, Secret: credential, Identity: identity,
	})
	if err != nil {
		return nil, err
	}
	client, err := acquisitionslack.NewLeasedSlackHTTPClient(registry, ref, identity, budgets)
	if err != nil {
		_ = registry.Revoke(ref)
		return nil, err
	}
	return &SlackSourceConnection{
		SessionRef: ref, Snapshot: snapshot, Client: client,
		Disconnect: func() error {
			client.Close()
			return registry.Disconnect(ref)
		},
	}, nil
}
