package orchestration

import (
	"context"
	"encoding/json"
	"time"
)

type EventStore interface {
	Load(context.Context, RunID) ([]Event, error)
	Append(context.Context, RunID, ExpectedVersion, ...Event) error
}

func (service *ActivationService) RecordAuthority(ctx context.Context, runID RunID, domain, artifactFingerprint, projectionFingerprint string) (Aggregate, error) {
	if service == nil || service.store == nil {
		return Aggregate{}, ErrMissingEventStore
	}
	events, err := service.store.Load(ctx, runID)
	if err != nil {
		return Aggregate{}, err
	}
	aggregate, err := Rebuild(runID, events)
	if err != nil {
		return Aggregate{}, err
	}
	payload, err := json.Marshal(AuthorityReferencePayload{Domain: domain, ArtifactFingerprint: artifactFingerprint, ProjectionFingerprint: projectionFingerprint})
	if err != nil {
		return Aggregate{}, err
	}
	event := Event{SchemaVersion: EventSchemaVersion, RunID: runID, Sequence: aggregate.Version + 1, Type: EventAuthorityReference, OccurredAt: service.now().UTC().Format(time.RFC3339Nano), Payload: payload}
	if _, err := DecodeAuthorityReferencePayload(event); err != nil {
		return Aggregate{}, err
	}
	if err := service.store.Append(ctx, runID, ExpectedVersion(aggregate.Version), event); err != nil {
		return Aggregate{}, err
	}
	if err := aggregate.Apply(event); err != nil {
		return Aggregate{}, err
	}
	return aggregate, nil
}

type ActivationService struct {
	store EventStore
	now   func() time.Time
}

func NewActivationService(store EventStore, now func() time.Time) *ActivationService {
	if now == nil {
		now = time.Now
	}
	return &ActivationService{store: store, now: now}
}

func (service *ActivationService) Execute(ctx context.Context, runID RunID, command Command) (Aggregate, error) {
	if service == nil || service.store == nil {
		return Aggregate{}, ErrMissingEventStore
	}
	events, err := service.store.Load(ctx, runID)
	if err != nil {
		return Aggregate{}, err
	}
	aggregate, err := Rebuild(runID, events)
	if err != nil {
		return Aggregate{}, err
	}
	if command.Now.IsZero() {
		command.Now = service.now()
	}
	event, err := HandleCommand(aggregate, command)
	if err != nil {
		return Aggregate{}, err
	}
	if err := service.store.Append(ctx, runID, ExpectedVersion(aggregate.Version), event); err != nil {
		return Aggregate{}, err
	}
	if err := aggregate.Apply(event); err != nil {
		return Aggregate{}, err
	}
	return aggregate, nil
}

func (service *ActivationService) Get(ctx context.Context, runID RunID) (Aggregate, error) {
	if service == nil || service.store == nil {
		return Aggregate{}, ErrMissingEventStore
	}
	events, err := service.store.Load(ctx, runID)
	if err != nil {
		return Aggregate{}, err
	}
	return Rebuild(runID, events)
}
