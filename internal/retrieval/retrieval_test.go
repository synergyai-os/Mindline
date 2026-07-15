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
	unsafeRelated := artifact
	unsafeRelated.RelatedURLs = []RelatedURL{{URL: "https://example.com/related?token=synthetic-value", Relation: "source_links_to", DiscoveryEvidenceRef: "evidence-1", SemanticallyRelevant: true}}
	if err := ValidateArtifact(unsafeRelated); err == nil {
		t.Fatal("retrieval artifact accepted an unsafe related URL")
	}
}

func TestSensitiveRedactedArtifactIsContentFreeAndNotAttempted(t *testing.T) {
	request := Request{CanonicalItemID: "withheld-1", Strategy: "manual_support", Format: "sensitive_redacted"}
	artifact := MissingArtifact(request, StateNotAttempted, AccessUnsupported, OriginSourcePolicy, SensitiveRedactedMissingnessReason)
	artifact.SecretLike = true
	if err := ValidateArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	withURL := artifact
	withURL.CanonicalURL = "https://example.com/"
	if err := ValidateArtifact(withURL); err == nil {
		t.Fatal("source-policy evidence accepted a URL")
	}
	withoutMarker := artifact
	withoutMarker.SecretLike = false
	if err := ValidateArtifact(withoutMarker); err == nil {
		t.Fatal("content-free artifact without sensitive-redacted authority was accepted")
	}
	withMetadata := artifact
	withMetadata.Metadata.Title = "must not persist"
	if err := ValidateArtifact(withMetadata); err == nil {
		t.Fatal("sensitive-redacted artifact accepted source metadata")
	}
	withFreeFormReason := artifact
	withFreeFormReason.Missingness = []string{"operator supplied text"}
	if err := ValidateArtifact(withFreeFormReason); err == nil {
		t.Fatal("sensitive-redacted artifact accepted free-form missingness")
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
