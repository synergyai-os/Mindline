// Package resourcefetch retrieves public resource context through a deliberately
// narrow, bounded HTTP(S) policy. It never logs request or response material.
package resourcefetch

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/routing"
)

const (
	ReasonSensitiveOrAmbiguous = "sensitive_or_ambiguous"
	ReasonUnsupportedScheme    = "unsupported_scheme"
	ReasonUnsupportedMIME      = "unsupported_mime"
	ReasonAccessDenied         = "access_denied"
	ReasonUnreachable          = "unreachable"
	ReasonRateLimited          = "rate_limited"
	ReasonUnsafeNetworkTarget  = "unsafe_network_target"
	ReasonBudgetExhausted      = "budget_exhausted"

	BudgetDimensionWire      = "wire"
	BudgetDimensionDecoded   = "decoded"
	BudgetDimensionExtracted = "extracted"
)

// FrozenPolicy is supplied by the run's persisted, fingerprinted resource
// budget. It makes every per-response limit visible to the queue/ledger rather
// than leaving a hidden live value inside the HTTP client.
type FrozenPolicy struct {
	Fingerprint              string
	RequestTimeout           time.Duration
	MaximumRedirects         int
	MaximumWireBytes         int64
	MaximumDecodedBytes      int64
	MaximumExtractedBytes    int
	MaximumRetryAfterSeconds int
}

func DefaultPolicy() FrozenPolicy {
	return FrozenPolicy{
		Fingerprint:              "mindline-resource-fetch-policy/v0.1:live-default",
		RequestTimeout:           20 * time.Second,
		MaximumRedirects:         3,
		MaximumWireBytes:         5 << 20,
		MaximumDecodedBytes:      2 << 20,
		MaximumExtractedBytes:    512 << 10,
		MaximumRetryAfterSeconds: 60,
	}
}

func (policy FrozenPolicy) validate() error {
	if strings.TrimSpace(policy.Fingerprint) == "" || policy.RequestTimeout <= 0 || policy.MaximumRedirects < 0 || policy.MaximumWireBytes <= 0 || policy.MaximumDecodedBytes <= 0 || policy.MaximumExtractedBytes <= 0 || policy.MaximumRetryAfterSeconds <= 0 {
		return errors.New("resource fetch policy is invalid")
	}
	return nil
}

// Error intentionally contains a fixed structural reason only. In particular,
// it never embeds a URL, hostname, header, provider error, or response text.
type Error struct {
	Reason            string
	Retryable         bool
	RetryAfterSeconds int
	BudgetDimension   string
}

func (err *Error) Error() string { return "resource fetch blocked: " + err.Reason }

func reasonError(reason string) error { return &Error{Reason: reason} }

func budgetError(dimension string) error {
	return &Error{Reason: ReasonBudgetExhausted, BudgetDimension: dimension}
}

func retryableError(reason string, retryAfterSeconds int) error {
	return &Error{Reason: reason, Retryable: true, RetryAfterSeconds: retryAfterSeconds}
}

func ReasonOf(err error) string {
	var fetchError *Error
	if errors.As(err, &fetchError) {
		return fetchError.Reason
	}
	return ReasonUnreachable
}

func BudgetDimensionOf(err error) string {
	var fetchError *Error
	if errors.As(err, &fetchError) && fetchError.Reason == ReasonBudgetExhausted {
		return fetchError.BudgetDimension
	}
	return ""
}

func retryOf(err error) (bool, int) {
	var fetchError *Error
	if errors.As(err, &fetchError) {
		return fetchError.Retryable, fetchError.RetryAfterSeconds
	}
	return false, 0
}

// Resolver is injected for deterministic DNS and rebinding tests. Every
// returned answer is checked before a connection is made.
type Resolver interface {
	LookupNetIP(context.Context, string) ([]netip.Addr, error)
}

// DialContextFunc is injected only at the transport boundary. The fetcher
// supplies the already validated, pinned address and verifies the returned
// connection peer before sending an HTTP request.
type DialContextFunc func(context.Context, string, string) (net.Conn, error)

// Dependencies make the network boundary deterministic in tests. Nil fields
// select the safe standard-library defaults.
type Dependencies struct {
	Resolver    Resolver
	DialContext DialContextFunc
	Now         func() time.Time
}

// PreparedURL is in-memory-only. Callers must not use CanonicalURL in logs,
// queue ledgers, receipts, or proof artifacts.
type PreparedURL struct {
	CanonicalURL string
	URL          *url.URL
	Host         string
	Port         string
}

// Prepare performs STD-20 before an identity/fingerprint can be made or a
// request can be sent. Secret variants are rejected before their values leave
// this in-memory call.
func Prepare(raw string) (PreparedURL, error) {
	safe, storageState, err := routing.PrepareURLForStorage(raw)
	if err != nil {
		return PreparedURL{}, reasonError(ReasonUnsupportedScheme)
	}
	if storageState == routing.URLStorageSensitiveRedacted || safe == "" {
		return PreparedURL{}, reasonError(ReasonSensitiveOrAmbiguous)
	}
	parsed, err := url.Parse(safe)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" {
		return PreparedURL{}, reasonError(ReasonSensitiveOrAmbiguous)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return PreparedURL{}, reasonError(ReasonUnsupportedScheme)
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	parsedAddress, parseAddressErr := netip.ParseAddr(host)
	if host == "" || isLocalHostname(host) || (parseAddressErr == nil && parsedAddress.IsValid()) {
		return PreparedURL{}, reasonError(ReasonUnsafeNetworkTarget)
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if port != "80" && port != "443" {
		return PreparedURL{}, reasonError(ReasonUnsafeNetworkTarget)
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return PreparedURL{}, reasonError(ReasonUnsafeNetworkTarget)
	}
	return PreparedURL{CanonicalURL: safe, URL: parsed, Host: host, Port: port}, nil
}

func isLocalHostname(host string) bool {
	return !strings.Contains(host, ".") || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local")
}

func validateMIME(contentType, host string) (string, error) {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch mediaType {
	case "text/html", "text/plain", "application/json", "application/xml", "text/xml":
		return mediaType, nil
	case "text/markdown", "text/x-markdown", "application/vnd.github.raw":
		if host == "github.com" || strings.HasSuffix(host, ".github.com") || host == "raw.githubusercontent.com" {
			return mediaType, nil
		}
		return "", reasonError(ReasonUnsupportedMIME)
	}
	if strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml") {
		return mediaType, nil
	}
	return "", reasonError(ReasonUnsupportedMIME)
}
