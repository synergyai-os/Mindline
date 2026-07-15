package productbrain

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type countingSecretProvider struct {
	calls  int
	secret string
	err    error
}

type revocableTestSecretProvider struct {
	secret string
	lease  context.Context
	calls  int
}

func (p *revocableTestSecretProvider) Secret(context.Context) (string, error) {
	return "", errors.New("legacy secret path must not be used")
}

func (p *revocableTestSecretProvider) SecretWithContext(context.Context) (string, context.Context, error) {
	p.calls++
	return p.secret, p.lease, nil
}

func (p *countingSecretProvider) Secret(context.Context) (string, error) {
	p.calls++
	return p.secret, p.err
}

func TestAKIResolvesCredentialForEveryCallAndHonorsRevocation(t *testing.T) {
	profile := testDeliveryProfile()
	provider := &countingSecretProvider{secret: "first-secret"}
	seen := []string{}
	transport, err := NewAKITransport(context.Background(), profile, provider, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		seen = append(seen, request.Header.Get("Authorization"))
		response := `{"ok":true,"data":{"_id":"ws-test","slug":"test","governanceMode":"open","keyScope":"readwrite","keyId":"key-test"}}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response)), Request: request}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.ResolveWorkspace(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.secret = "second-secret"
	if _, err := transport.ResolveWorkspace(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.err = errors.New("revoked")
	if _, err := transport.ResolveWorkspace(context.Background()); err == nil || err.Error() != "credential_missing" {
		t.Fatalf("revoked provider was not rejected: %v", err)
	}
	if provider.calls != 3 || len(seen) != 2 || seen[0] != "Bearer first-secret" || seen[1] != "Bearer second-secret" {
		t.Fatalf("credential was cached or leaked across calls: calls=%d seen=%v", provider.calls, seen)
	}
}

func TestAKIDefaultTransportDisablesAmbientProxy(t *testing.T) {
	transport, err := NewAKITransport(context.Background(), testDeliveryProfile(), &countingSecretProvider{secret: "test-secret"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	httpTransport, ok := transport.client.Transport.(*http.Transport)
	if !ok || httpTransport.Proxy != nil {
		t.Fatalf("default AKI transport did not disable ambient proxy: %#v", transport.client.Transport)
	}
}

func TestAKIRevocationCancelsInFlightRequest(t *testing.T) {
	lease, revoke := context.WithCancel(context.Background())
	provider := &revocableTestSecretProvider{secret: "ephemeral-secret", lease: lease}
	started := make(chan struct{})
	transport, err := NewAKITransport(context.Background(), testDeliveryProfile(), provider, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	}))
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := transport.ResolveWorkspace(context.Background())
		result <- err
	}()
	<-started
	revoke()
	if err := <-result; err == nil || err.Error() != "transient" {
		t.Fatalf("revoked in-flight request did not stop safely: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("revocation-aware provider was not resolved exactly once: %d", provider.calls)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestAKIRejectsUntrustedOriginBeforeRequestingCredential(t *testing.T) {
	profile := testDeliveryProfile()
	profile.Transport.BaseURL = "https://gateway.productbrain.io.evil.example"
	provider := &countingSecretProvider{secret: "must-not-be-requested"}
	_, err := NewAKITransport(context.Background(), profile, provider, nil)
	if err == nil || err.Error() != "untrusted_product_brain_origin" {
		t.Fatalf("expected trusted-origin failure, got %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("credential provider called %d times before origin trust", provider.calls)
	}
}

func TestAKIResolveWorkspaceUsesCanonicalEnvelopeAndHeaders(t *testing.T) {
	profile := testDeliveryProfile()
	provider := &countingSecretProvider{secret: "test-secret"}
	transport, err := NewAKITransport(context.Background(), profile, provider, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != ProductionGatewayOrigin+"/api/aki" {
			return nil, errors.New("unexpected request target")
		}
		if request.Header.Get("Authorization") != "Bearer test-secret" || request.Header.Get("x-pb-source") != "mindline" || request.Header.Get("Content-Type") != "application/json" {
			return nil, errors.New("unexpected request headers")
		}
		var body struct {
			Fn   string         `json:"fn"`
			Args map[string]any `json:"args"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Fn != "resolveWorkspace" || len(body.Args) != 0 {
			return nil, errors.New("unexpected request envelope")
		}
		response := `{"ok":true,"data":{"_id":"ws-test","slug":"test","governanceMode":"open","keyScope":"readwrite","keyId":"key-test"}}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response)), Request: request}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	capability, err := transport.ResolveWorkspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || capability.ID != "ws-test" || capability.Slug != "test" || capability.GovernanceMode != "open" || capability.KeyScope != "readwrite" || capability.KeyID != "key-test" {
		t.Fatalf("unexpected capability: %+v (secret calls %d)", capability, provider.calls)
	}
}

func TestBuildPreflightPerformsOnlyReadOnlyCapabilityAndSchemaCalls(t *testing.T) {
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	provider := &countingSecretProvider{secret: "test-secret"}
	calls := []string{}
	transport, err := NewAKITransport(context.Background(), profile, provider, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body struct {
			Fn   string         `json:"fn"`
			Args map[string]any `json:"args"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			return nil, err
		}
		calls = append(calls, body.Fn)
		response := `{"ok":true,"data":{"_id":"ws-test","slug":"test","governanceMode":"open","keyScope":"readwrite","keyId":"key-test"}}`
		if body.Fn == "chain.getCollectionFields" {
			slug, _ := body.Args["slug"].(string)
			encoded, _ := json.Marshal(map[string]any{"ok": true, "data": testCollectionCapability(slug)})
			response = string(encoded)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response)), Request: request}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := BuildPreflight(context.Background(), outbox, profile, transport)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 4 || calls[0] != "resolveWorkspace" || calls[1] != "chain.getCollectionFields" || calls[2] != "chain.getCollectionFields" || calls[3] != "chain.getCollectionFields" || len(artifact.CollectionContracts) != 3 || artifact.MutationCalls != 0 || artifact.Verdict != "pass" || artifact.Fingerprint != hashValue(artifact) {
		t.Fatalf("preflight was not read-only and sealed: calls=%v artifact=%+v", calls, artifact)
	}
}

func TestCollectionContractRejectsUnknownLiveFieldType(t *testing.T) {
	profile := testDeliveryProfile()
	outbox, _, err := CompileOutbox(promotedRouteFixture(t), profile)
	if err != nil {
		t.Fatal(err)
	}
	contract := testCollectionCapability("insights")
	contract.Fields = append(contract.Fields, CollectionFieldCapability{Key: "unusedFutureField", Type: "future-unknown-type"})
	if err := validateOutboxCollectionContract(outbox, "insights", contract); err == nil {
		t.Fatal("unknown optional live field type was accepted")
	}
	contract = testCollectionCapability("insights")
	contract.Fields = append(contract.Fields, contract.Fields[0])
	if err := validateOutboxCollectionContract(outbox, "insights", contract); err == nil {
		t.Fatal("duplicate live field key was accepted")
	}
}

func TestSafeDeliveryCategoryContractIsClosedAndNormalized(t *testing.T) {
	expected := []string{
		"credential_missing", "untrusted_product_brain_origin", "unauthorized", "forbidden", "workspace_mismatch",
		"capability_missing", "collection_contract_mismatch", "not_found", "already_exists", "validation_failed",
		"rate_limited", "transient", "remote_failure", "ambiguous_outcome", "destination_name_conflict",
		"readback_mismatch", "dependency_not_acknowledged", "outbox_state_mismatch", "unsafe_outbound_value", "local_state_failure",
	}
	actual := SafeDeliveryCategoryValues()
	if len(actual) != len(expected) {
		t.Fatalf("safe category contract changed: %v", actual)
	}
	for index, category := range expected {
		if actual[index] != category || !ValidSafeDeliveryCategory(category) {
			t.Fatalf("safe category contract changed at %d: %v", index, actual)
		}
	}
	if ValidSafeDeliveryCategory("invalid_response") || ValidSafeDeliveryCategory("transport_failure") || ValidSafeDeliveryCategory("remote body text") {
		t.Fatal("unsigned transport categories were accepted")
	}
	if got := (&TransportError{Category: "transport_failure"}).Error(); got != "remote_failure" {
		t.Fatalf("unknown transport category was not normalized: %s", got)
	}
	if got := (&TransportError{Category: "transient", MayHaveCommitted: true}).Error(); got != "ambiguous_outcome" {
		t.Fatalf("possibly committed mutation was not normalized: %s", got)
	}
	if got := safeCategory(errors.New("remote body text")); got != "local_state_failure" {
		t.Fatalf("arbitrary error text escaped into a safe category: %s", got)
	}
}

func TestAKIFailuresUseOnlySignedSafeCategories(t *testing.T) {
	profile := testDeliveryProfile()
	provider := &countingSecretProvider{secret: "test-secret"}
	for _, test := range []struct {
		name     string
		response *http.Response
		err      error
		mutation bool
		expected string
	}{
		{name: "read network failure", err: errors.New("private network detail"), expected: "transient"},
		{name: "mutation network failure", err: errors.New("private network detail"), mutation: true, expected: "ambiguous_outcome"},
		{name: "malformed response", response: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("not-json"))}, expected: "remote_failure"},
		{name: "rate limited", response: &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":false}`))}, expected: "rate_limited"},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport, err := NewAKITransport(context.Background(), profile, provider, roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if test.response != nil {
					test.response.Request = request
				}
				return test.response, test.err
			}))
			if err != nil {
				t.Fatal(err)
			}
			err = transport.call(context.Background(), "test.fn", map[string]any{}, nil, test.mutation)
			if err == nil || err.Error() != test.expected || !ValidSafeDeliveryCategory(err.Error()) {
				t.Fatalf("expected signed category %q, got %v", test.expected, err)
			}
		})
	}
}
