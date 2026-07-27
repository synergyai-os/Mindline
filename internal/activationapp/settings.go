package activationapp

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
	"github.com/synergyai-os/Mindline/internal/controlsettings"
	"github.com/synergyai-os/Mindline/internal/privateio"
	"github.com/synergyai-os/Mindline/internal/processing"
)

var settingsSlackChannelIDPattern = regexp.MustCompile(`^[A-Z0-9]{2,64}$`)

func controlSettingsOptions(now func() time.Time) controlsettings.Options {
	return controlsettings.Options{
		Now: now,
		Validators: map[string]controlsettings.AdapterValidator{
			"slack_web_api": controlsettings.AdapterValidatorFunc(validateSlackDefaults),
		},
	}
}

func validateSlackDefaults(schemaVersion string, raw json.RawMessage) (json.RawMessage, error) {
	if schemaVersion != "mindline.source.slack-web-api-defaults/v1" {
		return nil, controlsettings.ErrInvalid
	}
	var value struct {
		ChannelID string `json:"channel_id"`
	}
	if err := privateio.DecodeJSONStrict(raw, &value); err != nil || !settingsSlackChannelIDPattern.MatchString(value.ChannelID) {
		return nil, controlsettings.ErrInvalid
	}
	return json.Marshal(value)
}

func (app *App) SaveSettings(ctx context.Context, expected controlsettings.Revision, draft controlsettings.Draft) (controlsettings.Snapshot, error) {
	if app == nil || app.settings == nil {
		return controlsettings.Snapshot{}, errors.New("settings repository unavailable")
	}
	return app.settings.Save(ctx, expected, draft)
}

func (app *App) applySettingsLocked(ctx context.Context, payload useSettingsPayload) (any, error) {
	if app.settings == nil {
		return nil, errors.New("settings repository unavailable")
	}
	snapshot, err := app.settings.Load(ctx)
	if err != nil {
		return nil, err
	}
	document := snapshot.Document
	if snapshot.State != controlsettings.StateSaved || document.Version != payload.SettingsVersion || document.Generation != payload.SettingsGeneration || document.Fingerprint != payload.SettingsFingerprint {
		return nil, controlsettings.ErrConflict
	}
	if app.state.Sample != nil {
		return nil, errors.New("sealed strategy cannot be changed")
	}
	policy := DrainPolicy{
		SchemaVersion:          DrainPolicySchema,
		MaximumNetworkRequests: document.Draft.DrainPolicy.MaximumNetworkRequests,
		MaximumWallTimeSeconds: document.Draft.DrainPolicy.MaximumWallTimeSeconds,
		MaximumCostMicrounits:  document.Draft.DrainPolicy.MaximumCostMicrounits,
		MaximumRetryAttempts:   document.Draft.DrainPolicy.MaximumRetryAttempts,
		ManualSupportTolerance: document.Draft.DrainPolicy.ManualSupportTolerance,
	}
	policy.Fingerprint = acquisition.Fingerprint(policy)
	if err := validateDrainPolicy(policy); err != nil {
		return nil, err
	}
	strategy := processing.SealStrategy(processing.StrategySnapshot{
		StrategyID:       "founder-activation",
		Version:          strings.TrimPrefix(document.Fingerprint, "sha256:")[:16],
		ContextLenses:    strings.Join(document.Draft.ContextLenses, "\n"),
		RoutingPolicy:    document.Draft.RoutingPolicy,
		OperatorIdentity: "founder-browser-session",
		CreatedAt:        app.now().UTC().Format(time.RFC3339),
	})
	if err := processing.ValidateStrategy(strategy); err != nil {
		return nil, err
	}
	app.state.Strategy = &strategy
	app.state.DrainPolicy = &policy
	app.state.SettingsVersion = document.Version
	app.state.SettingsGeneration = document.Generation
	app.state.SettingsFingerprint = document.Fingerprint
	app.resetAfterStrategyLocked()
	if err := app.commitAuthorityLocked(ctx, "strategy", strategy.Fingerprint); err != nil {
		return nil, err
	}
	return map[string]any{
		"applied":              true,
		"settings_version":     document.Version,
		"settings_generation":  document.Generation,
		"settings_fingerprint": document.Fingerprint,
		"strategy_fingerprint": strategy.Fingerprint,
	}, nil
}
