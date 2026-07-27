package resourcefetch

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Fetcher has no ambient HTTP state: each request has a fresh transport, no
// proxy, no cookie jar, no inherited headers, and no automatic redirects.
// Dependencies are intentionally limited to DNS, dialing, and time so tests
// can exercise SSRF/rebinding behavior without any external network access.
type Fetcher struct {
	dependencies Dependencies
	policy       FrozenPolicy
}

func New(dependencies Dependencies) Fetcher {
	fetcher, err := NewWithPolicy(dependencies, DefaultPolicy())
	if err != nil {
		panic("default resource fetch policy is invalid")
	}
	return fetcher
}

// NewWithPolicy accepts the run's already frozen policy and fingerprint. Test
// profiles may use lower limits but must supply an explicit fingerprint.
func NewWithPolicy(dependencies Dependencies, policy FrozenPolicy) (Fetcher, error) {
	if err := policy.validate(); err != nil {
		return Fetcher{}, err
	}
	if dependencies.Now == nil {
		dependencies.Now = time.Now
	}
	return Fetcher{dependencies: dependencies, policy: policy}, nil
}

// Fetch returns a deterministic complete, partial, or blocked result. A
// blocked result holds a fixed reason only; it never contains a request URL,
// hostname, header, provider response, or raw network error.
func (fetcher Fetcher) Fetch(ctx context.Context, rawURL string) Result {
	started := fetcher.dependencies.Now()
	target, err := Prepare(rawURL)
	if err != nil {
		return fetcher.stamp(withElapsed(blockedError(err), started, fetcher.dependencies.Now()))
	}
	return fetcher.stamp(withElapsed(fetcher.fetchPrepared(ctx, target), started, fetcher.dependencies.Now()))
}

func withElapsed(result Result, started, finished time.Time) Result {
	if result.RequestCount == 0 {
		return result
	}
	elapsed := finished.Sub(started)
	seconds := int64(math.Ceil(elapsed.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	result.WallSeconds = seconds
	return result
}

func (fetcher Fetcher) stamp(result Result) Result {
	now := fetcher.dependencies.Now
	if now == nil {
		now = time.Now
	}
	result.RetrievedAt = now().UTC()
	result.PolicyFingerprint = fetcher.policy.Fingerprint
	return result
}

func (fetcher Fetcher) fetchPrepared(ctx context.Context, target PreparedURL) Result {
	requestCount := 0
	for redirects := 0; redirects <= fetcher.policy.MaximumRedirects; redirects++ {
		result, redirect, attempted, err := fetcher.fetchOnce(ctx, target)
		if attempted {
			requestCount++
		}
		if err != nil {
			return withRequestCount(blockedError(err), requestCount)
		}
		if redirect == "" {
			return withRequestCount(result, requestCount)
		}
		if redirects == fetcher.policy.MaximumRedirects {
			return withRequestCount(blocked(ReasonUnreachable), requestCount)
		}
		next, err := redirectTarget(target, redirect)
		if err != nil {
			return withRequestCount(blockedError(err), requestCount)
		}
		target = next
	}
	return withRequestCount(blocked(ReasonUnreachable), requestCount)
}

func withRequestCount(result Result, requestCount int) Result {
	result.RequestCount = requestCount
	return result
}

func (fetcher Fetcher) fetchOnce(parent context.Context, target PreparedURL) (Result, string, bool, error) {
	ctx, cancel := context.WithTimeout(parent, fetcher.policy.RequestTimeout)
	defer cancel()
	answers, err := resolvePublic(ctx, fetcher.dependencies.resolver(), target.Host)
	if err != nil {
		return Result{}, "", false, err
	}
	// A fresh transport pins this request to one already validated answer. A
	// redirect always constructs another transport after re-resolving its host.
	transport := secureTransport(target, answers[0], fetcher.dependencies.dial(), fetcher.policy.RequestTimeout)
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   fetcher.policy.RequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := requestFor(target)
	if err != nil {
		return Result{}, "", false, err
	}
	request = request.WithContext(ctx)
	response, err := client.Do(request)
	if err != nil {
		return Result{}, "", true, classifyRequestError(err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		location := strings.TrimSpace(response.Header.Get("Location"))
		if location == "" {
			return Result{}, "", true, reasonError(ReasonUnreachable)
		}
		return Result{}, location, true, nil
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Result{}, "", true, statusError(response.StatusCode, response.Header.Get("Retry-After"), fetcher.dependencies.Now(), fetcher.policy.MaximumRetryAfterSeconds)
	}
	mediaType, err := validateMIME(response.Header.Get("Content-Type"), target.Host)
	if err != nil {
		return Result{}, "", true, err
	}
	wire, err := boundedRead(response.Body, fetcher.policy.MaximumWireBytes)
	if err != nil {
		return Result{}, "", true, err
	}
	decoded, err := decodeBody(wire, response.Header.Get("Content-Encoding"), fetcher.policy.MaximumDecodedBytes)
	if err != nil {
		return Result{}, "", true, err
	}
	result, err := extract(decoded, mediaType, target, fetcher.policy)
	if err != nil {
		return Result{}, "", true, err
	}
	result.WireBytes = int64(len(wire))
	result.DecodedBytes = int64(len(decoded))
	return result, "", true, nil
}

func blocked(reason string) Result { return Result{State: "blocked", Reason: reason} }

func blockedError(err error) Result {
	retryable, retryAfterSeconds := retryOf(err)
	return Result{State: "blocked", Reason: ReasonOf(err), Retryable: retryable, RetryAfterSeconds: retryAfterSeconds}
}

func statusError(status int, retryAfter string, now time.Time, maximumRetryAfterSeconds int) error {
	switch {
	case status == http.StatusTooManyRequests:
		return retryableError(ReasonRateLimited, boundedRetryAfter(retryAfter, now, maximumRetryAfterSeconds))
	case status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusProxyAuthRequired:
		return reasonError(ReasonAccessDenied)
	case status >= http.StatusBadRequest && status < http.StatusInternalServerError:
		return reasonError(ReasonAccessDenied)
	default:
		return retryableError(ReasonUnreachable, 0)
	}
}

func classifyRequestError(err error) error {
	var structural *Error
	if errors.As(err, &structural) {
		return structural
	}
	if errors.Is(err, context.Canceled) {
		return reasonError(ReasonUnreachable)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return retryableError(ReasonUnreachable, 0)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return retryableError(ReasonUnreachable, 0)
	}
	return reasonError(ReasonOf(err))
}

func boundedRetryAfter(value string, now time.Time, maximum int) int {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil && seconds >= 0 {
		if seconds > maximum {
			return maximum
		}
		return seconds
	}
	parsed, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	seconds = int(math.Ceil(parsed.Sub(now).Seconds()))
	if seconds < 0 {
		return 0
	}
	if seconds > maximum {
		return maximum
	}
	return seconds
}
