package retrieval

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

var ErrLiveTransportDisabled = errors.New("live retrieval transport disabled before pre-live gate")

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type BrokerOptions struct {
	Resolver               Resolver
	Dialer                 Dialer
	AllowedSyntheticHosts  []string
	MaximumRedirects       int
	MaximumBodyBytes       int64
	MaximumCompressedBytes int64
	RequestTimeout         time.Duration
	ConnectTimeout         time.Duration
	TLSHandshakeTimeout    time.Duration
	ResponseHeaderTimeout  time.Duration
}

type SafeBroker struct {
	resolver               Resolver
	dialer                 Dialer
	allowedHosts           map[string]bool
	maximumRedirects       int
	maximumBodyBytes       int64
	maximumCompressedBytes int64
	requestTimeout         time.Duration
	connectTimeout         time.Duration
	tlsHandshakeTimeout    time.Duration
	responseHeaderTimeout  time.Duration
}

type FetchResponse struct {
	FinalURL    string
	StatusCode  int
	ContentType string
	Body        []byte
}

type BoundaryError struct {
	Category string
	Err      error
}

func (err *BoundaryError) Error() string { return "retrieval blocked: " + err.Category }
func (err *BoundaryError) Unwrap() error { return err.Err }

// NewSyntheticBroker deliberately requires injected DNS/dial boundaries and a
// closed host allowlist. It cannot silently become a live retrieval transport;
// a post-gate composition root must add that authority explicitly.
func NewSyntheticBroker(options BrokerOptions) (*SafeBroker, error) {
	if options.Resolver == nil || options.Dialer == nil || len(options.AllowedSyntheticHosts) == 0 {
		return nil, ErrLiveTransportDisabled
	}
	allowed := map[string]bool{}
	for _, host := range options.AllowedSyntheticHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" || net.ParseIP(host) != nil || !isReservedSyntheticHost(host) {
			return nil, errors.New("invalid synthetic retrieval host")
		}
		allowed[host] = true
	}
	if options.MaximumRedirects <= 0 {
		options.MaximumRedirects = 5
	}
	if options.MaximumBodyBytes <= 0 {
		options.MaximumBodyBytes = 4 << 20
	}
	if options.MaximumCompressedBytes <= 0 {
		options.MaximumCompressedBytes = 2 << 20
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 15 * time.Second
	}
	if options.ConnectTimeout <= 0 {
		options.ConnectTimeout = 3 * time.Second
	}
	if options.TLSHandshakeTimeout <= 0 {
		options.TLSHandshakeTimeout = 3 * time.Second
	}
	if options.ResponseHeaderTimeout <= 0 {
		options.ResponseHeaderTimeout = 5 * time.Second
	}
	return &SafeBroker{
		resolver: options.Resolver, dialer: options.Dialer, allowedHosts: allowed,
		maximumRedirects: options.MaximumRedirects, maximumBodyBytes: options.MaximumBodyBytes, maximumCompressedBytes: options.MaximumCompressedBytes,
		requestTimeout: options.RequestTimeout, connectTimeout: options.ConnectTimeout, tlsHandshakeTimeout: options.TLSHandshakeTimeout, responseHeaderTimeout: options.ResponseHeaderTimeout,
	}, nil
}

func (broker *SafeBroker) Fetch(ctx context.Context, rawURL string) (FetchResponse, error) {
	requestContext, cancel := context.WithTimeout(ctx, broker.requestTimeout)
	defer cancel()
	current, err := parseTarget(rawURL)
	if err != nil {
		return FetchResponse{}, boundaryError("invalid_target", err)
	}
	for redirect := 0; ; redirect++ {
		if !broker.allowedHosts[strings.ToLower(current.Hostname())] {
			return FetchResponse{}, boundaryError("non_synthetic_host", errors.New("host is outside the synthetic allowlist"))
		}
		if redirect > broker.maximumRedirects {
			return FetchResponse{}, boundaryError("redirect_limit", errors.New("redirect limit exceeded"))
		}
		addresses, err := broker.resolvePublic(requestContext, current.Hostname())
		if err != nil {
			return FetchResponse{}, err
		}
		response, err := broker.fetchHop(requestContext, current, addresses[0])
		if err != nil {
			return FetchResponse{}, err
		}
		if response.StatusCode < 300 || response.StatusCode > 399 {
			return broker.readResponse(response, current.String())
		}
		location := response.Header.Get("Location")
		_ = response.Body.Close()
		if location == "" {
			return FetchResponse{}, boundaryError("redirect_location", errors.New("redirect missing location"))
		}
		nextReference, err := url.Parse(location)
		if err != nil {
			return FetchResponse{}, boundaryError("redirect_location", err)
		}
		next, err := parseTarget(current.ResolveReference(nextReference).String())
		if err != nil {
			return FetchResponse{}, boundaryError("redirect_target", err)
		}
		if current.Scheme == "https" && next.Scheme != "https" {
			return FetchResponse{}, boundaryError("https_downgrade", errors.New("HTTPS downgrade blocked"))
		}
		current = next
	}
}

func (broker *SafeBroker) resolvePublic(ctx context.Context, host string) ([]netip.Addr, error) {
	resolved, err := broker.resolver.LookupIPAddr(ctx, host)
	if err != nil || len(resolved) == 0 {
		return nil, boundaryError("dns_failure", errors.New("host resolution failed"))
	}
	addresses := make([]netip.Addr, 0, len(resolved))
	seen := map[netip.Addr]bool{}
	publicCount := 0
	blockedCount := 0
	for _, candidate := range resolved {
		address, ok := netip.AddrFromSlice(candidate.IP)
		if !ok {
			blockedCount++
			continue
		}
		address = address.Unmap()
		if !isPublicAddress(address) {
			blockedCount++
			continue
		}
		publicCount++
		if !seen[address] {
			seen[address] = true
			addresses = append(addresses, address)
		}
	}
	if publicCount == 0 || blockedCount != 0 {
		return nil, boundaryError("mixed_or_private_dns", errors.New("DNS answer was not uniformly public"))
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].String() < addresses[j].String() })
	return addresses, nil
}

func (broker *SafeBroker) fetchHop(ctx context.Context, target *url.URL, pinned netip.Addr) (*http.Response, error) {
	port := target.Port()
	if port == "" {
		if target.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dialContext, cancel := context.WithTimeout(ctx, broker.connectTimeout)
			defer cancel()
			connection, err := broker.dialer.DialContext(dialContext, network, net.JoinHostPort(pinned.String(), port))
			if err != nil {
				return nil, err
			}
			remote, ok := connection.RemoteAddr().(*net.TCPAddr)
			if !ok {
				_ = connection.Close()
				return nil, boundaryError("peer_identity", errors.New("unexpected peer address type"))
			}
			peer, ok := netip.AddrFromSlice(remote.IP)
			if !ok || peer.Unmap() != pinned {
				_ = connection.Close()
				return nil, boundaryError("peer_identity", errors.New("connected peer does not match pinned address"))
			}
			return connection, nil
		},
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   broker.tlsHandshakeTimeout,
		ResponseHeaderTimeout: broker.responseHeaderTimeout,
		DisableCompression:    true,
		ForceAttemptHTTP2:     false,
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, boundaryError("request", err)
	}
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", "Mindline-Synthetic-Retrieval/0.1")
	response, err := client.Do(request)
	if err != nil {
		return nil, boundaryError("transport", errors.New("retrieval transport failed"))
	}
	return response, nil
}

func (broker *SafeBroker) readResponse(response *http.Response, finalURL string) (FetchResponse, error) {
	defer response.Body.Close()
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	allowedContent := map[string]bool{"text/html": true, "text/plain": true, "application/json": true, "application/pdf": true}
	if !allowedContent[contentType] {
		return FetchResponse{}, boundaryError("content_type", errors.New("unsupported response content type"))
	}
	bodyReader := io.Reader(io.LimitReader(response.Body, broker.maximumCompressedBytes+1))
	if encoding := strings.ToLower(strings.TrimSpace(response.Header.Get("Content-Encoding"))); encoding != "" && encoding != "identity" {
		if encoding != "gzip" {
			return FetchResponse{}, boundaryError("content_encoding", errors.New("unsupported content encoding"))
		}
		compressed, err := io.ReadAll(bodyReader)
		if err != nil || int64(len(compressed)) > broker.maximumCompressedBytes {
			return FetchResponse{}, boundaryError("compressed_size", errors.New("compressed body limit exceeded"))
		}
		gzipReader, err := gzip.NewReader(strings.NewReader(string(compressed)))
		if err != nil {
			return FetchResponse{}, boundaryError("content_encoding", errors.New("invalid gzip response"))
		}
		defer gzipReader.Close()
		bodyReader = gzipReader
	}
	body, err := io.ReadAll(io.LimitReader(bodyReader, broker.maximumBodyBytes+1))
	if err != nil {
		return FetchResponse{}, boundaryError("body_read", errors.New("response body read failed"))
	}
	if int64(len(body)) > broker.maximumBodyBytes {
		return FetchResponse{}, boundaryError("body_size", errors.New("response body limit exceeded"))
	}
	return FetchResponse{FinalURL: finalURL, StatusCode: response.StatusCode, ContentType: contentType, Body: body}, nil
}

func parseTarget(raw string) (*url.URL, error) {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || target.Hostname() == "" || target.User != nil || (target.Scheme != "http" && target.Scheme != "https") || net.ParseIP(target.Hostname()) != nil {
		return nil, errors.New("target must be an HTTP(S) hostname without userinfo or IP literal")
	}
	host := strings.ToLower(target.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || !isASCII(host) {
		return nil, errors.New("local or ambiguous hostname blocked")
	}
	port := target.Port()
	if (target.Scheme == "http" && port != "" && port != "80") || (target.Scheme == "https" && port != "" && port != "443") {
		return nil, errors.New("target port blocked")
	}
	for key := range target.Query() {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") || strings.Contains(lower, "signature") {
			return nil, errors.New("secret-bearing query blocked")
		}
	}
	query := strings.ToLower(target.RawQuery)
	for _, marker := range []string{"xoxb-", "xoxp-", "bearer%20", "sk_live_", "sk-proj-", "password%3d", "api_key%3d"} {
		if strings.Contains(query, marker) {
			return nil, errors.New("secret-looking query value blocked")
		}
	}
	return target, nil
}

func isPublicAddress(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2001:10::/28"), netip.MustParsePrefix("fc00::/7"),
}

func isASCII(value string) bool {
	for _, character := range value {
		if character > 127 {
			return false
		}
	}
	return true
}

func isReservedSyntheticHost(host string) bool {
	return host == "test" || host == "invalid" || host == "example" || strings.HasSuffix(host, ".test") || strings.HasSuffix(host, ".invalid") || strings.HasSuffix(host, ".example")
}

func boundaryError(category string, err error) error {
	return &BoundaryError{Category: category, Err: fmt.Errorf("%s", category)}
}
