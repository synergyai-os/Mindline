package activationapp

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/integrations"
	"github.com/synergyai-os/Mindline/internal/productbrain"
)

type productionConnector struct{}

type transientSecretProvider struct{ secret []byte }

func (provider transientSecretProvider) Secret(context.Context) (string, error) {
	if len(provider.secret) == 0 {
		return "", errors.New("credential_missing")
	}
	return string(provider.secret), nil
}

func (productionConnector) Connect(ctx context.Context, registry *integrations.Registry, credential []byte) (*DestinationConnection, error) {
	credential = bytes.TrimSpace(credential)
	if registry == nil || len(credential) < 16 {
		return nil, errors.New("credential_missing")
	}
	probe, err := productbrain.NewAKITransport(ctx, provisionalDeliveryProfile(), transientSecretProvider{secret: credential}, nil)
	if err != nil {
		return nil, err
	}
	capability, err := probe.ResolveWorkspace(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(capability.ID) == "" || strings.TrimSpace(capability.Slug) == "" || strings.TrimSpace(capability.KeyID) == "" || capability.KeyScope != "readwrite" {
		return nil, errors.New("capability_missing")
	}
	identity := integrations.VerifiedIdentity{
		Provider: "product_brain", WorkspaceID: capability.ID, KeyID: capability.KeyID, CapabilityVersion: "aki/v0.2",
	}
	ref, snapshot, err := registry.Register(integrations.LeaseOptions{
		Kind: integrations.ConnectionProductBrain, Secret: credential, Identity: identity,
		IdleTTL: 20 * time.Minute, AbsoluteTTL: 2 * time.Hour,
	})
	if err != nil {
		return nil, err
	}
	transport := &sessionTransport{registry: registry, ref: ref, identity: identity}
	return &DestinationConnection{
		SessionRef: ref, Snapshot: snapshot, Capability: capability, Transport: transport,
		Disconnect: func() error { return registry.Disconnect(ref) },
	}, nil
}

func provisionalDeliveryProfile() productbrain.DeliveryProfile {
	return productbrain.DeliveryProfile{
		SchemaVersion: productbrain.DeliveryProfileSchema, ProfileID: "mindline-activation-probe",
		Workspace:  productbrain.DeliveryWorkspace{ExpectedID: "probe", ExpectedSlug: "probe"},
		Transport:  productbrain.DeliveryTransportProfile{Kind: "aki", BaseURL: productbrain.ProductionGatewayOrigin, APIPath: "/api/aki"},
		Credential: productbrain.DeliveryCredentialProfile{Provider: "environment", Name: "MINDLINE_PRODUCT_BRAIN_API_KEY", ExpectedKeyID: "probe"},
		RoleMappings: map[string]productbrain.RoleMapping{
			"external_entity":         {CollectionSlug: "landscape", IDPrefix: "LAND"},
			"evidence_backed_finding": {CollectionSlug: "insights", IDPrefix: "INS"},
			"unresolved_tension":      {CollectionSlug: "tensions", IDPrefix: "TEN"},
		},
		RelationMappings: map[string]string{"related_to": "related_to"}, DraftOnly: true,
	}
}

func deliveryProfile(capability productbrain.WorkspaceCapability) productbrain.DeliveryProfile {
	profile := provisionalDeliveryProfile()
	profile.ProfileID = "mindline-trusted-activation"
	profile.Workspace = productbrain.DeliveryWorkspace{ExpectedID: capability.ID, ExpectedSlug: capability.Slug}
	profile.Credential.ExpectedKeyID = capability.KeyID
	profile.ReviewPolicy = &productbrain.DeliveryReviewPolicy{CredentialLifecycle: "retire_after_review", PrivateRuntimeLifecycle: "cleanup_after_review"}
	return profile
}

type sessionTransport struct {
	registry *integrations.Registry
	ref      integrations.SessionRef
	identity integrations.VerifiedIdentity
}

func withAKI[T any](ctx context.Context, transport *sessionTransport, call func(context.Context, *productbrain.AKITransport) (T, error)) (T, error) {
	var result T
	if transport == nil || transport.registry == nil {
		return result, errors.New("credential_missing")
	}
	err := transport.registry.Use(ctx, transport.ref, transport.identity, func(callContext context.Context, secret []byte) error {
		aki, err := productbrain.NewAKITransport(callContext, provisionalDeliveryProfile(), transientSecretProvider{secret: secret}, nil)
		if err != nil {
			return err
		}
		result, err = call(callContext, aki)
		return err
	})
	return result, err
}

func (transport *sessionTransport) ResolveWorkspace(ctx context.Context) (productbrain.WorkspaceCapability, error) {
	return withAKI(ctx, transport, func(callContext context.Context, aki *productbrain.AKITransport) (productbrain.WorkspaceCapability, error) {
		return aki.ResolveWorkspace(callContext)
	})
}

func (transport *sessionTransport) GetCollectionFields(ctx context.Context, slug string) (productbrain.CollectionCapability, error) {
	return withAKI(ctx, transport, func(callContext context.Context, aki *productbrain.AKITransport) (productbrain.CollectionCapability, error) {
		return aki.GetCollectionFields(callContext, slug)
	})
}

func (transport *sessionTransport) GetEntry(ctx context.Context, id string) (productbrain.EntryReadback, error) {
	return withAKI(ctx, transport, func(callContext context.Context, aki *productbrain.AKITransport) (productbrain.EntryReadback, error) {
		return aki.GetEntry(callContext, id)
	})
}

func (transport *sessionTransport) SearchEntries(ctx context.Context, query, collection string) ([]productbrain.EntrySearchResult, error) {
	return withAKI(ctx, transport, func(callContext context.Context, aki *productbrain.AKITransport) ([]productbrain.EntrySearchResult, error) {
		return aki.SearchEntries(callContext, query, collection)
	})
}

func (transport *sessionTransport) CreateEntry(ctx context.Context, request productbrain.CreateEntryRequest) (productbrain.CreateEntryResult, error) {
	return withAKI(ctx, transport, func(callContext context.Context, aki *productbrain.AKITransport) (productbrain.CreateEntryResult, error) {
		return aki.CreateEntry(callContext, request)
	})
}

func (transport *sessionTransport) ListEntryRelations(ctx context.Context, id string) ([]productbrain.RelationReadback, error) {
	return withAKI(ctx, transport, func(callContext context.Context, aki *productbrain.AKITransport) ([]productbrain.RelationReadback, error) {
		return aki.ListEntryRelations(callContext, id)
	})
}

func (transport *sessionTransport) CreateEntryRelation(ctx context.Context, request productbrain.CreateRelationRequest) (productbrain.CreateRelationResult, error) {
	return withAKI(ctx, transport, func(callContext context.Context, aki *productbrain.AKITransport) (productbrain.CreateRelationResult, error) {
		return aki.CreateEntryRelation(callContext, request)
	})
}

func (transport *sessionTransport) RuntimeSecretFindings(value any) []productbrain.PrivacyFinding {
	var findings []productbrain.PrivacyFinding
	err := transport.registry.Use(context.Background(), transport.ref, transport.identity, func(_ context.Context, secret []byte) error {
		findings = productbrain.ScanPublicArtifact(value, string(secret))
		return nil
	})
	if err != nil {
		return []productbrain.PrivacyFinding{{Category: "runtime_secret_unavailable", JSONPath: "$"}}
	}
	return findings
}

var _ productbrain.ProductBrainTransport = (*sessionTransport)(nil)
var _ productbrain.RuntimeSecretScanner = (*sessionTransport)(nil)
