package retrieval

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/acquisition"
)

func TestImportedEvidenceIsAlwaysReplayLabelled(t *testing.T) {
	adapter, err := NewImportedEvidenceAdapter([]acquisition.ImportedEvidence{{
		CanonicalItemID: "item-1", CanonicalURL: "https://example.com/article", State: "complete", RetrievedAt: "2026-07-14T10:00:00Z",
		Excerpts: []acquisition.ImportedExcerpt{{ExcerptID: "evidence-1", Text: "Synthetic public evidence.", Locator: "page"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := adapter.Retrieve(context.Background(), Request{CanonicalItemID: "item-1", CanonicalURL: "https://example.com/article", Strategy: "generic", Format: "html"})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Origin != OriginImportedReplay || artifact.State != StateComplete {
		t.Fatalf("imported evidence lost replay label: %+v", artifact)
	}
}

func TestSyntheticBrokerRejectsLiveConstructionAndUnsafeTargets(t *testing.T) {
	if _, err := NewSyntheticBroker(BrokerOptions{}); !errors.Is(err, ErrLiveTransportDisabled) {
		t.Fatalf("missing boundaries must keep live transport disabled, got %v", err)
	}
	resolver := fixedResolver{addresses: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}}
	broker, err := NewSyntheticBroker(BrokerOptions{Resolver: resolver, Dialer: rejectingDialer{}, AllowedSyntheticHosts: []string{"fixture.test"}})
	if err != nil {
		t.Fatal(err)
	}
	unsafe := []string{
		"http://127.0.0.1/", "http://[::ffff:127.0.0.1]/", "http://fixture.test:8080/", "https://user:pass@fixture.test/", "https://fixture.test/?token=secret", "https://fixture.test/?q=xoxb-secret", "https://example.org/",
	}
	for _, target := range unsafe {
		if _, err := broker.Fetch(context.Background(), target); err == nil {
			t.Fatalf("unsafe target accepted: %s", target)
		}
	}
}

func TestSyntheticBrokerRejectsMixedDNSBeforeDial(t *testing.T) {
	resolver := fixedResolver{addresses: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("127.0.0.1")}}}
	dialer := &countingDialer{}
	broker, err := NewSyntheticBroker(BrokerOptions{Resolver: resolver, Dialer: dialer, AllowedSyntheticHosts: []string{"fixture.test"}, RequestTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Fetch(context.Background(), "https://fixture.test/"); err == nil {
		t.Fatal("mixed DNS response accepted")
	}
	if dialer.calls != 0 {
		t.Fatalf("dial occurred before mixed DNS rejection: %d", dialer.calls)
	}
}

type fixedResolver struct{ addresses []net.IPAddr }

func (resolver fixedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return append([]net.IPAddr(nil), resolver.addresses...), nil
}

type rejectingDialer struct{}

func (rejectingDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("synthetic dial rejected")
}

type countingDialer struct{ calls int }

func (dialer *countingDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	dialer.calls++
	return nil, errors.New("synthetic dial rejected")
}
