package integrations

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSessionRegistryPinsIdentityAndRevokesInFlightUse(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	randomByte := byte(1)
	registry := NewSessionRegistry(RegistryOptions{
		Now: func() time.Time { return now },
		Random: func(buffer []byte) (int, error) {
			for index := range buffer {
				buffer[index] = randomByte
			}
			randomByte++
			return len(buffer), nil
		},
	})
	secret := []byte("SENTINEL_SESSION_SECRET")
	identity := VerifiedIdentity{Provider: "slack", WorkspaceID: "T-synthetic", ChannelID: "C-synthetic", CapabilityVersion: "slack_web_api/v1"}
	ref, snapshot, err := registry.Register(LeaseOptions{Kind: ConnectionSlackWebAPI, Secret: secret, IdleTTL: time.Minute, AbsoluteTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(ref) < 40 || strings.Contains(snapshot.ConnectionID, string(secret)) {
		t.Fatalf("opaque reference or safe snapshot contract failed: ref=%q snapshot=%+v", ref, snapshot)
	}
	if _, err := registry.PinIdentity(ref, identity); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.PinIdentity(ref, VerifiedIdentity{Provider: "slack", WorkspaceID: "wrong", CapabilityVersion: "slack_web_api/v1"}); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("expected identity mismatch, got %v", err)
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
	if err := registry.Use(context.Background(), ref, identity, func(context.Context, []byte) error { return nil }); !errors.Is(err, ErrLeaseRevoked) {
		t.Fatalf("expected revoked lease, got %v", err)
	}
}

func TestSessionRegistryEnforcesIdleAndAbsoluteTTL(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	registry := NewSessionRegistry(RegistryOptions{Now: func() time.Time { return now }})
	identity := VerifiedIdentity{Provider: "product_brain", WorkspaceID: "workspace-synthetic", KeyID: "key-synthetic", CapabilityVersion: "aki/v0.2"}
	ref, _, err := registry.Register(LeaseOptions{Kind: ConnectionProductBrain, Secret: []byte("synthetic"), IdleTTL: time.Minute, AbsoluteTTL: 2 * time.Minute, Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if err := registry.Use(context.Background(), ref, identity, func(context.Context, []byte) error { return nil }); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("idle boundary must expire fail-closed, got %v", err)
	}
}
