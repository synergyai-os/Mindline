package resourcefetch

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type defaultResolver struct{}

func (defaultResolver) LookupNetIP(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func defaultDial(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: -1}).DialContext(ctx, network, address)
}

func (dependencies Dependencies) resolver() Resolver {
	if dependencies.Resolver != nil {
		return dependencies.Resolver
	}
	return defaultResolver{}
}

func (dependencies Dependencies) dial() DialContextFunc {
	if dependencies.DialContext != nil {
		return dependencies.DialContext
	}
	return defaultDial
}

func resolvePublic(ctx context.Context, resolver Resolver, host string) ([]netip.Addr, error) {
	answers, err := resolver.LookupNetIP(ctx, host)
	if err != nil || len(answers) == 0 {
		return nil, reasonError(ReasonUnreachable)
	}
	for _, address := range answers {
		if !isPublicAddress(address) {
			return nil, reasonError(ReasonUnsafeNetworkTarget)
		}
	}
	return answers, nil
}

// isPublicAddress is intentionally stricter than IsGlobalUnicast: RFC
// special-use/documentation, carrier-grade NAT, ULA, and metadata ranges are
// all unsuitable fetch targets.
func isPublicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	if address.Is4() {
		for _, prefix := range []netip.Prefix{
			netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
			netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
			netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"),
		} {
			if prefix.Contains(address) {
				return false
			}
		}
		return true
	}
	for _, prefix := range []netip.Prefix{
		netip.MustParsePrefix("::/128"), netip.MustParsePrefix("::1/128"), netip.MustParsePrefix("::ffff:0:0/96"),
		netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("2001:2::/48"),
		netip.MustParsePrefix("2001:10::/28"), netip.MustParsePrefix("2001:20::/28"), netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("fec0::/10"),
	} {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func peerAddress(connection net.Conn) (netip.Addr, bool) {
	if tcp, ok := connection.RemoteAddr().(*net.TCPAddr); ok {
		address, ok := netip.AddrFromSlice(tcp.IP)
		return address.Unmap(), ok
	}
	value := connection.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(value); err == nil {
		address, err := netip.ParseAddr(host)
		return address.Unmap(), err == nil
	}
	address, err := netip.ParseAddr(value)
	return address.Unmap(), err == nil
}

func secureTransport(target PreparedURL, address netip.Addr, dial DialContextFunc, requestTimeout time.Duration) *http.Transport {
	transport := &http.Transport{
		Proxy: nil, DisableCompression: true, ForceAttemptHTTP2: false,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ServerName: target.Host},
		TLSHandshakeTimeout: requestTimeout, ResponseHeaderTimeout: requestTimeout,
		IdleConnTimeout: time.Second, MaxIdleConns: 0,
	}
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		connection, err := dial(ctx, network, net.JoinHostPort(address.String(), target.Port))
		if err != nil {
			return nil, err
		}
		peer, ok := peerAddress(connection)
		if !ok || peer != address.Unmap() {
			_ = connection.Close()
			return nil, reasonError(ReasonUnsafeNetworkTarget)
		}
		return connection, nil
	}
	return transport
}

func redirectTarget(current PreparedURL, location string) (PreparedURL, error) {
	next, err := url.Parse(location)
	if err != nil {
		return PreparedURL{}, reasonError(ReasonUnreachable)
	}
	if !next.IsAbs() {
		next = current.URL.ResolveReference(next)
	}
	prepared, err := Prepare(next.String())
	if err != nil {
		return PreparedURL{}, err
	}
	if current.URL.Scheme == "https" && prepared.URL.Scheme != "https" {
		return PreparedURL{}, reasonError(ReasonUnsafeNetworkTarget)
	}
	return prepared, nil
}

func requestFor(target PreparedURL) (*http.Request, error) {
	request, err := http.NewRequest(http.MethodGet, target.CanonicalURL, nil)
	if err != nil {
		return nil, reasonError(ReasonUnreachable)
	}
	// Explicitly clear potentially ambient credentials; the transport also has no
	// proxy or cookie jar and never follows redirects automatically.
	request.Header = make(http.Header)
	request.Header.Set("Accept", "text/html, text/plain, application/json, application/xml, text/xml")
	request.Header.Set("User-Agent", "Mindline-resourcefetch/1")
	return request, nil
}

func sameHost(a, b string) bool { return strings.EqualFold(a, b) }
