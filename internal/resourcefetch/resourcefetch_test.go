package resourcefetch

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

type staticResolver map[string][]netip.Addr

func (resolver staticResolver) LookupNetIP(_ context.Context, host string) ([]netip.Addr, error) {
	return resolver[host], nil
}

type recordingResolver struct {
	answers staticResolver
	mu      sync.Mutex
	calls   []string
}

func (resolver *recordingResolver) LookupNetIP(ctx context.Context, host string) ([]netip.Addr, error) {
	resolver.mu.Lock()
	resolver.calls = append(resolver.calls, host)
	resolver.mu.Unlock()
	return resolver.answers.LookupNetIP(ctx, host)
}

type peerConn struct {
	net.Conn
	peer net.Addr
}

func (connection peerConn) RemoteAddr() net.Addr { return connection.peer }

type scriptedDial struct {
	mu        sync.Mutex
	responses []string
	headers   []http.Header
	peer      net.Addr
}

func (dial *scriptedDial) dial(_ context.Context, _, _ string) (net.Conn, error) {
	dial.mu.Lock()
	if len(dial.responses) == 0 {
		dial.mu.Unlock()
		return nil, io.EOF
	}
	response := dial.responses[0]
	dial.responses = dial.responses[1:]
	dial.mu.Unlock()
	client, server := net.Pipe()
	go func() {
		request, err := http.ReadRequest(bufio.NewReader(server))
		if err == nil {
			dial.mu.Lock()
			dial.headers = append(dial.headers, request.Header.Clone())
			dial.mu.Unlock()
		}
		_, _ = io.WriteString(server, response)
		_ = server.Close()
	}()
	return peerConn{Conn: client, peer: dial.peer}, nil
}

func TestPrepareAppliesSTD20BeforeFetchIdentity(t *testing.T) {
	for _, raw := range []string{
		"https://public.example/path?token=one-secret",
		"https://public.example/path?token=two-secret",
		"https://user:password@public.example/path",
	} {
		_, err := Prepare(raw)
		if ReasonOf(err) != ReasonSensitiveOrAmbiguous {
			t.Fatalf("Prepare(%q) reason = %q", raw, ReasonOf(err))
		}
		if strings.Contains(err.Error(), "one-secret") || strings.Contains(err.Error(), "two-secret") || strings.Contains(err.Error(), "password") {
			t.Fatal("structural error leaked a sensitive URL component")
		}
	}
	for _, raw := range []string{"file:///tmp/private", "https://127.0.0.1/x", "https://localhost/x", "https://public.example:8080/x"} {
		if _, err := Prepare(raw); err == nil {
			t.Fatalf("Prepare(%q) unexpectedly allowed target", raw)
		}
	}
}

func TestResolvePublicRejectsAnyUnsafeDNSAnswer(t *testing.T) {
	public := netip.MustParseAddr("8.8.8.8")
	for _, unsafe := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254", "192.0.2.1", "2001:db8::1", "fc00::1"} {
		resolver := staticResolver{"public.example": {public, netip.MustParseAddr(unsafe)}}
		if _, err := resolvePublic(context.Background(), resolver, "public.example"); ReasonOf(err) != ReasonUnsafeNetworkTarget {
			t.Fatalf("unsafe DNS answer %s reason = %q", unsafe, ReasonOf(err))
		}
	}
}

func TestFetchPinsPeerAndSendsNoAmbientCredentials(t *testing.T) {
	peer := &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 80}
	dial := &scriptedDial{peer: peer, responses: []string{
		"HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nConnection: close\r\n\r\n<html><title>Public title</title><body>public body</body></html>",
	}}
	fixedNow := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	fetcher := New(Dependencies{Resolver: staticResolver{"public.example": {netip.MustParseAddr("8.8.8.8")}}, DialContext: dial.dial, Now: func() time.Time { return fixedNow }})
	result := fetcher.Fetch(context.Background(), "http://public.example/article")
	if result.State != "complete" || result.Title != "Public title" || result.Text == "" || result.Reason != "" {
		t.Fatalf("unexpected fetch result: %#v", result)
	}
	if !result.RetrievedAt.Equal(fixedNow) {
		t.Fatalf("clock was not injected: %v", result.RetrievedAt)
	}
	dial.mu.Lock()
	defer dial.mu.Unlock()
	if len(dial.headers) != 1 {
		t.Fatalf("request count = %d", len(dial.headers))
	}
	for _, header := range []string{"Authorization", "Proxy-Authorization", "Cookie"} {
		if dial.headers[0].Get(header) != "" {
			t.Fatalf("forbidden header %s was sent", header)
		}
	}
}

func TestFetchRevalidatesEveryRedirectAndRejectsRebindingPeer(t *testing.T) {
	peer := &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 80}
	dial := &scriptedDial{peer: peer, responses: []string{
		"HTTP/1.1 302 Found\r\nLocation: http://second.example/final\r\nConnection: close\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\npublic result",
	}}
	resolver := &recordingResolver{answers: staticResolver{
		"first.example":  {netip.MustParseAddr("8.8.8.8")},
		"second.example": {netip.MustParseAddr("8.8.8.8")},
	}}
	result := New(Dependencies{Resolver: resolver, DialContext: dial.dial}).Fetch(context.Background(), "http://first.example/start")
	if result.State != "complete" || result.Text != "public result" {
		t.Fatalf("redirect result = %#v", result)
	}
	if result.RequestCount != 2 {
		t.Fatalf("redirect request count = %d", result.RequestCount)
	}
	resolver.mu.Lock()
	calls := append([]string(nil), resolver.calls...)
	resolver.mu.Unlock()
	if strings.Join(calls, ",") != "first.example,second.example" {
		t.Fatalf("redirect DNS calls = %v", calls)
	}

	rebinding := &scriptedDial{
		peer:      &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 80},
		responses: []string{"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\nnever reached"},
	}
	result = New(Dependencies{Resolver: staticResolver{"public.example": {netip.MustParseAddr("8.8.8.8")}}, DialContext: rebinding.dial}).Fetch(context.Background(), "http://public.example/rebound")
	if result.State != "blocked" || result.Reason != ReasonUnsafeNetworkTarget {
		t.Fatalf("rebound result = %#v", result)
	}
}

func TestRedirectAndBodyLimitsFailClosed(t *testing.T) {
	policy := DefaultPolicy()
	current, err := Prepare("https://public.example/start")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := redirectTarget(current, "http://other.example/down"); ReasonOf(err) != ReasonUnsafeNetworkTarget {
		t.Fatalf("downgrade reason = %q", ReasonOf(err))
	}
	missingLocation := &scriptedDial{peer: &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 80}, responses: []string{"HTTP/1.1 302 Found\r\nConnection: close\r\n\r\n"}}
	result := New(Dependencies{Resolver: staticResolver{"public.example": {netip.MustParseAddr("8.8.8.8")}}, DialContext: missingLocation.dial}).Fetch(context.Background(), "http://public.example/no-location")
	if result.State != "blocked" || result.Reason != ReasonUnreachable {
		t.Fatalf("missing redirect location result = %#v", result)
	}
	if _, err := validateMIME("application/octet-stream", "public.example"); ReasonOf(err) != ReasonUnsupportedMIME {
		t.Fatalf("mime reason = %q", ReasonOf(err))
	}
	if _, err := validateMIME("application/vnd.github.raw", "public.example"); ReasonOf(err) != ReasonUnsupportedMIME {
		t.Fatalf("non-GitHub raw MIME reason = %q", ReasonOf(err))
	}
	if _, err := validateMIME("application/vnd.github.raw", "raw.githubusercontent.com"); err != nil {
		t.Fatalf("GitHub raw MIME rejected: %v", err)
	}
	wire, err := boundedRead(
		strings.NewReader(strings.Repeat("x", int(policy.MaximumWireBytes)+1)),
		policy.MaximumWireBytes, BudgetDimensionWire,
	)
	if ReasonOf(err) != ReasonBudgetExhausted || len(wire) != int(policy.MaximumWireBytes)+1 ||
		BudgetDimensionOf(err) != BudgetDimensionWire {
		t.Fatalf("wire cap reason = %q dimension=%q", ReasonOf(err), BudgetDimensionOf(err))
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write([]byte(strings.Repeat("x", int(policy.MaximumDecodedBytes)+1)))
	_ = writer.Close()
	decoded, err := decodeBody(compressed.Bytes(), "gzip", policy.MaximumDecodedBytes)
	if ReasonOf(err) != ReasonBudgetExhausted || len(decoded) != int(policy.MaximumDecodedBytes)+1 ||
		BudgetDimensionOf(err) != BudgetDimensionDecoded {
		t.Fatalf("decoded cap reason = %q dimension=%q", ReasonOf(err), BudgetDimensionOf(err))
	}
}

func TestProviderProfilesAndExtractedCap(t *testing.T) {
	policy := DefaultPolicy()
	target, err := Prepare("https://www.youtube.com/watch?v=publicvid01")
	if err != nil {
		t.Fatal(err)
	}
	result, err := extract([]byte(strings.Repeat("b", policy.MaximumExtractedBytes)), "text/plain", target, policy)
	if err != nil {
		t.Fatal(err)
	}
	if result.Profile != ProfileYouTube || result.State != "partial" || result.ExtractedBytes != policy.MaximumExtractedBytes {
		t.Fatalf("profile result = %#v", result)
	}
	if !strings.Contains(strings.Join(result.Missingness, ","), "transcript_not_publicly_accessible") {
		t.Fatalf("missing transcript status: %#v", result.Missingness)
	}
	blocked, err := extract([]byte("a"+strings.Repeat("b", policy.MaximumExtractedBytes)), "text/plain", target, policy)
	if ReasonOf(err) != ReasonBudgetExhausted || blocked.DecodedBytes != int64(policy.MaximumExtractedBytes+1) ||
		blocked.ExtractedBytes != policy.MaximumExtractedBytes+1 ||
		BudgetDimensionOf(err) != BudgetDimensionExtracted {
		t.Fatalf("extracted cap reason = %q dimension=%q", ReasonOf(err), BudgetDimensionOf(err))
	}
}

func TestFetchRetrySignalsAndTerminalClientErrors(t *testing.T) {
	peer := &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 80}
	resolver := staticResolver{"public.example": {netip.MustParseAddr("8.8.8.8")}}
	for _, test := range []struct {
		name       string
		response   string
		reason     string
		retryable  bool
		retryAfter int
	}{
		{"rate limited", "HTTP/1.1 429 Too Many Requests\r\nRetry-After: 999\r\nConnection: close\r\n\r\n", ReasonRateLimited, true, 60},
		{"server error", "HTTP/1.1 503 Service Unavailable\r\nConnection: close\r\n\r\n", ReasonUnreachable, true, 0},
		{"client error", "HTTP/1.1 404 Not Found\r\nConnection: close\r\n\r\n", ReasonAccessDenied, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			dial := &scriptedDial{peer: peer, responses: []string{test.response}}
			result := New(Dependencies{Resolver: resolver, DialContext: dial.dial}).Fetch(context.Background(), "http://public.example/status")
			if result.State != "blocked" || result.Reason != test.reason || result.Retryable != test.retryable || result.RetryAfterSeconds != test.retryAfter || result.RequestCount != 1 {
				t.Fatalf("status result = %#v", result)
			}
		})
	}

	transientDial := func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.DNSError{IsTimeout: true}
	}
	result := New(Dependencies{Resolver: resolver, DialContext: transientDial}).Fetch(context.Background(), "http://public.example/transient")
	if result.State != "blocked" || result.Reason != ReasonUnreachable || !result.Retryable || result.RequestCount != 1 {
		t.Fatalf("transient result = %#v", result)
	}
}

func TestFrozenPolicyControlsCapsAndIsReturnedStructurally(t *testing.T) {
	policy := DefaultPolicy()
	policy.Fingerprint = "mindline-resource-fetch-policy/v0.1:fixture-wire-cap"
	policy.MaximumWireBytes = 3
	peer := &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 80}
	dial := &scriptedDial{peer: peer, responses: []string{"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\nfour"}}
	fetcher, err := NewWithPolicy(Dependencies{Resolver: staticResolver{"public.example": {netip.MustParseAddr("8.8.8.8")}}, DialContext: dial.dial}, policy)
	if err != nil {
		t.Fatal(err)
	}
	result := fetcher.Fetch(context.Background(), "http://public.example/capped")
	if result.State != "blocked" || result.Reason != ReasonBudgetExhausted ||
		result.ExhaustedBudgetDimension != BudgetDimensionWire ||
		result.RequestCount != 1 || result.WireBytes != policy.MaximumWireBytes+1 ||
		result.PolicyFingerprint != policy.Fingerprint {
		t.Fatalf("capped result = %#v", result)
	}
}

func TestFailedDecodeAndExtractionReturnConsumedUsage(t *testing.T) {
	peer := &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 80}
	resolver := staticResolver{"public.example": {netip.MustParseAddr("8.8.8.8")}}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write([]byte("four"))
	_ = writer.Close()
	gzipResponse := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", compressed.Len(), compressed.String())

	decodedPolicy := DefaultPolicy()
	decodedPolicy.Fingerprint = "mindline-resource-fetch-policy/v0.1:fixture-decoded-cap"
	decodedPolicy.MaximumDecodedBytes = 3
	decodedDial := &scriptedDial{peer: peer, responses: []string{gzipResponse}}
	decodedFetcher, err := NewWithPolicy(Dependencies{Resolver: resolver, DialContext: decodedDial.dial}, decodedPolicy)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodedFetcher.Fetch(context.Background(), "http://public.example/decoded-cap")
	if decoded.State != "blocked" || decoded.ExhaustedBudgetDimension != BudgetDimensionDecoded ||
		decoded.WireBytes != int64(compressed.Len()) || decoded.DecodedBytes != 4 {
		t.Fatalf("decoded failure lost consumed usage: %#v", decoded)
	}

	extractedPolicy := DefaultPolicy()
	extractedPolicy.Fingerprint = "mindline-resource-fetch-policy/v0.1:fixture-extracted-cap"
	extractedPolicy.MaximumExtractedBytes = 3
	extractedDial := &scriptedDial{peer: peer, responses: []string{"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 4\r\nConnection: close\r\n\r\nfour"}}
	extractedFetcher, err := NewWithPolicy(Dependencies{Resolver: resolver, DialContext: extractedDial.dial}, extractedPolicy)
	if err != nil {
		t.Fatal(err)
	}
	extracted := extractedFetcher.Fetch(context.Background(), "http://public.example/extracted-cap")
	if extracted.State != "blocked" || extracted.ExhaustedBudgetDimension != BudgetDimensionExtracted ||
		extracted.WireBytes != 4 || extracted.DecodedBytes != 4 || extracted.ExtractedBytes != 4 {
		t.Fatalf("extracted failure lost consumed usage: %#v", extracted)
	}
}

func ExampleFetcher_Fetch() {
	result := New(Dependencies{}).Fetch(context.Background(), "https://127.0.0.1/private")
	fmt.Println(result.State, result.Reason)
	// Output: blocked unsafe_network_target
}
