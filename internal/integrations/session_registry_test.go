package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSessionRegistryPinsIdentityAndRevokesInFlightUse(t *testing.T) {
	randomByte := byte(1)
	registry := NewSessionRegistry(RegistryOptions{
		Random: func(buffer []byte) (int, error) {
			for index := range buffer {
				buffer[index] = randomByte
			}
			randomByte++
			return len(buffer), nil
		},
	})
	secret := []byte("SENTINEL_SESSION_SECRET")
	identity := slackIdentity("T-synthetic")
	ref, snapshot, err := registry.Register(LeaseOptions{Kind: ConnectionSlackWebAPI, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	if len(ref) < 40 || snapshot.Kind != ConnectionSlackWebAPI {
		t.Fatalf("opaque reference or safe snapshot contract failed: ref=%q snapshot=%+v", ref, snapshot)
	}
	if _, err := registry.PinIdentity(ref, identity); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- registry.Use(context.Background(), ref, identity, func(ctx context.Context, leased []byte) error {
			if string(leased) != string(secret) {
				return errors.New("leased secret mismatch")
			}
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-started
	if err := registry.Revoke(ref); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected in-flight cancellation, got %v", err)
	}
	assertRevoked(t, registry, ref, identity)
}

func TestWP46_ProcessLeaseSurvivesElapsedTimeAndRevokesOnEvents(t *testing.T) {
	// The lease and its safe projection expose no duration, timestamp, or
	// expiry field. This makes elapsed wall time incapable of authorizing or
	// revoking a credential; only process-lifetime events below can do so.
	for _, target := range []any{LeaseOptions{}, ConnectionSnapshot{}, RegistryOptions{}, Registry{}} {
		typeOf := reflect.TypeOf(target)
		for _, forbidden := range []string{"Now", "IdleTTL", "AbsoluteTTL", "CreatedAt", "LastUsedAt", "IdleExpiresAt", "AbsoluteExpiresAt"} {
			if _, exists := typeOf.FieldByName(forbidden); exists {
				t.Fatalf("%s must not expose wall-clock authority field %s", typeOf.Name(), forbidden)
			}
		}
	}

	identity := productBrainIdentity("workspace-synthetic")
	registry := NewSessionRegistry(RegistryOptions{})
	ref, snapshot, err := registry.Register(LeaseOptions{Kind: ConnectionProductBrain, Secret: []byte("SESSION_SECRET_SENTINEL"), Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	for use := 0; use < 3; use++ {
		if err := registry.Use(context.Background(), ref, identity, func(context.Context, []byte) error { return nil }); err != nil {
			t.Fatalf("unchanged process lease use %d failed: %v", use, err)
		}
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	for _, forbidden := range []string{"SESSION_SECRET_SENTINEL", "connection_id", "session_ref", "created", "used", "expires"} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("safe snapshot persisted forbidden live lease state %q: %s", forbidden, jsonText)
		}
	}

	t.Run("explicit disconnect", func(t *testing.T) {
		registry, ref := registeredRegistry(t, identity)
		if err := registry.Disconnect(ref); err != nil {
			t.Fatal(err)
		}
		assertRevoked(t, registry, ref, identity)
	})

	t.Run("identity drift", func(t *testing.T) {
		registry, ref := registeredRegistry(t, identity)
		drifted := productBrainIdentity("different-workspace")
		if _, err := registry.PinIdentity(ref, drifted); !errors.Is(err, ErrIdentityMismatch) {
			t.Fatalf("expected identity mismatch, got %v", err)
		}
		assertRevoked(t, registry, ref, identity)
	})

	t.Run("expected identity drift", func(t *testing.T) {
		registry, ref := registeredRegistry(t, identity)
		drifted := productBrainIdentity("different-workspace")
		if err := registry.Use(context.Background(), ref, drifted, func(context.Context, []byte) error { return nil }); !errors.Is(err, ErrIdentityMismatch) {
			t.Fatalf("expected identity mismatch, got %v", err)
		}
		assertRevoked(t, registry, ref, identity)
	})

	t.Run("provider credential rejection", func(t *testing.T) {
		registry, ref := registeredRegistry(t, identity)
		err := registry.Use(context.Background(), ref, identity, func(context.Context, []byte) error {
			return errors.Join(errors.New("provider response"), ErrCredentialRejected)
		})
		if !errors.Is(err, ErrCredentialRejected) {
			t.Fatalf("expected credential rejection, got %v", err)
		}
		assertRevoked(t, registry, ref, identity)
	})

	t.Run("transient provider error is not revocation", func(t *testing.T) {
		registry, ref := registeredRegistry(t, identity)
		transient := errors.New("temporary provider failure")
		if err := registry.Use(context.Background(), ref, identity, func(context.Context, []byte) error { return transient }); !errors.Is(err, transient) {
			t.Fatalf("expected transient error, got %v", err)
		}
		if err := registry.Use(context.Background(), ref, identity, func(context.Context, []byte) error { return nil }); err != nil {
			t.Fatalf("transient provider failure revoked lease: %v", err)
		}
	})

	t.Run("shutdown", func(t *testing.T) {
		registry, ref := registeredRegistry(t, identity)
		registry.Shutdown()
		if err := registry.Use(context.Background(), ref, identity, func(context.Context, []byte) error { return nil }); !errors.Is(err, ErrRegistryClosed) {
			t.Fatalf("expected closed registry, got %v", err)
		}
	})
}

func registeredRegistry(t *testing.T, identity VerifiedIdentity) (*Registry, SessionRef) {
	t.Helper()
	registry := NewSessionRegistry(RegistryOptions{})
	ref, _, err := registry.Register(LeaseOptions{Kind: ConnectionProductBrain, Secret: []byte("synthetic"), Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	return registry, ref
}

func assertRevoked(t *testing.T, registry *Registry, ref SessionRef, identity VerifiedIdentity) {
	t.Helper()
	if err := registry.Use(context.Background(), ref, identity, func(context.Context, []byte) error { return nil }); !errors.Is(err, ErrLeaseRevoked) {
		t.Fatalf("expected revoked lease, got %v", err)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	entry := registry.entries[ref]
	if entry == nil || !entry.revoked || entry.secret != nil {
		t.Fatalf("revocation did not cancel and zero stored credential: %+v", entry)
	}
}

func slackIdentity(workspace string) VerifiedIdentity {
	return VerifiedIdentity{Provider: "slack", WorkspaceID: workspace, ChannelID: "C-synthetic", CapabilityVersion: "slack_web_api/v1"}
}

func productBrainIdentity(workspace string) VerifiedIdentity {
	return VerifiedIdentity{Provider: "product_brain", WorkspaceID: workspace, KeyID: "key-synthetic", CapabilityVersion: "aki/v0.2"}
}
