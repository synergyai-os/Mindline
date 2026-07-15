package routing

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var URLPattern = regexp.MustCompile(`(?i)https?://[^\s<>"'|]+`)

const (
	URLStorageSensitiveRedacted            = "sensitive_redacted"
	URLStorageNonSemanticComponentsRemoved = "non_semantic_components_removed"
)

// PrepareURLForStorage applies a deny-by-default durable URL policy. Unknown
// query parameters, userinfo, and ambiguous query serialization are withheld
// completely. Only provider-specific public identity parameters survive.
// Known provider-scoped marketing parameters are removed as non-semantic;
// fragments are withheld because their semantics cannot be safely inferred.
func PrepareURLForStorage(raw string) (string, string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), ".,;:!?)]}")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Hostname() == "" {
		return "", "", errors.New("invalid URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", errors.New("invalid URL")
	}
	if parsed.User != nil {
		return "", URLStorageSensitiveRedacted, nil
	}
	if parsed.Fragment != "" {
		return "", URLStorageSensitiveRedacted, nil
	}
	if secretLookingURLComponent(parsed.Hostname()) || secretLookingURLComponent(parsed.Path) {
		return "", URLStorageSensitiveRedacted, nil
	}
	if !rawQueryKeysAreExact(parsed.RawQuery) {
		return "", URLStorageSensitiveRedacted, nil
	}
	state := ""
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", URLStorageSensitiveRedacted, nil
	}
	host := strings.ToLower(parsed.Hostname())
	for key, candidates := range values {
		if sensitiveQueryKey(key) || secretLookingQueryValues(candidates) {
			return "", URLStorageSensitiveRedacted, nil
		}
		if trackingQueryKey(host, key) {
			values.Del(key)
			state = URLStorageNonSemanticComponentsRemoved
			continue
		}
		if !durableQueryParameterAllowed(host, parsed.EscapedPath(), key, candidates) {
			return "", URLStorageSensitiveRedacted, nil
		}
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), state, nil
}

func rawQueryKeysAreExact(rawQuery string) bool {
	if rawQuery == "" {
		return true
	}
	for _, field := range strings.Split(rawQuery, "&") {
		rawKey, _, _ := strings.Cut(field, "=")
		decodedKey, err := url.QueryUnescape(rawKey)
		if err != nil || rawKey == "" || rawKey != decodedKey || decodedKey != strings.ToLower(decodedKey) || decodedKey != strings.TrimSpace(decodedKey) {
			return false
		}
	}
	return true
}

func sensitiveQueryKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"token", "secret", "password", "passwd", "api_key", "apikey", "signature", "credential", "authorization", "access_key", "private_key", "session", "jwt", "signed", "expires"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return lower == "sig" || lower == "auth"
}

func secretLookingQueryValues(values []string) bool {
	for _, value := range values {
		if secretLookingURLComponent(value) {
			return true
		}
	}
	return false
}

func secretLookingURLComponent(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, marker := range []string{"xoxb-", "xoxp-", "xoxa-", "xoxr-", "sk-", "sk_", "ghp_", "github_pat_", "pb_sk_", "bearer ", "eyj"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func trackingQueryKey(host, key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if key != lower {
		return false
	}
	if strings.HasPrefix(lower, "utm_") || lower == "fbclid" || lower == "gclid" {
		return true
	}
	switch {
	case host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com"):
		return lower == "trk" || lower == "rcm" || strings.HasPrefix(lower, "lipi")
	case host == "youtube.com" || strings.HasSuffix(host, ".youtube.com") || host == "youtu.be":
		return lower == "feature" || lower == "si"
	case host == "open.spotify.com" || strings.HasSuffix(host, ".spotify.com"):
		return lower == "si"
	default:
		return false
	}
}

func durableQueryParameterAllowed(host, escapedPath, key string, values []string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	if key != lower || len(values) != 1 {
		return false
	}
	allowedShape := func(value string, maximum int) bool {
		if value == "" || len(value) > maximum {
			return false
		}
		for _, character := range value {
			if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_') {
				return false
			}
		}
		return true
	}
	allowed := false
	switch host {
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com":
		if lower == "v" {
			return escapedPath == "/watch" && allowedShape(values[0], 11) && len(values[0]) == 11
		}
		if lower == "list" {
			if escapedPath != "/playlist" && escapedPath != "/watch" || !allowedShape(values[0], 64) || len(values[0]) < 10 {
				return false
			}
			for _, prefix := range []string{"PL", "UU", "LL", "RD", "FL", "OLAK5uy"} {
				if strings.HasPrefix(values[0], prefix) {
					return true
				}
			}
			return false
		}
	case "news.ycombinator.com":
		allowed = escapedPath == "/item" && lower == "id"
		if allowed {
			for _, value := range values {
				if _, err := strconv.ParseUint(value, 10, 64); err != nil {
					return false
				}
			}
			return true
		}
	}
	if !allowed || len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !allowedShape(value, 64) {
			return false
		}
	}
	return true
}

func CanonicalizeURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), ".,;:!?)]}")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("invalid URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("unsupported URL scheme")
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	parsed.Host = host
	parsed.Fragment = ""
	parsed.User = nil
	values := parsed.Query()
	for key := range values {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || lower == "fbclid" || lower == "gclid" || lower == "trk" || lower == "rcm" || strings.HasPrefix(lower, "lipi") || strings.HasPrefix(lower, "tracking") {
			values.Del(key)
		}
	}
	for key := range values {
		sort.Strings(values[key])
	}
	parsed.RawQuery = values.Encode()
	escapedPath := path.Clean("/" + strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if escapedPath != "/" {
		escapedPath = strings.TrimSuffix(escapedPath, "/")
	}
	decodedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return "", errors.New("invalid URL path encoding")
	}
	parsed.Path = decodedPath
	parsed.RawPath = escapedPath
	return parsed.String(), nil
}

func CanonicalURLID(canonical string) string {
	digest := sha256.Sum256([]byte(canonicalIdentity(canonical)))
	return "url-" + hex.EncodeToString(digest[:])[:20]
}

func URLKind(canonical string) string {
	parsed, err := url.Parse(canonical)
	if err != nil {
		return "unknown"
	}
	host := strings.ToLower(parsed.Hostname())
	p := strings.ToLower(parsed.Path)
	switch {
	case host == "github.com" && len(strings.Split(strings.Trim(p, "/"), "/")) >= 2:
		return "github_repository"
	case strings.Contains(host, "linkedin.com") && strings.Contains(p, "/posts/"):
		return "linkedin_post"
	case strings.Contains(host, "linkedin.com"):
		return "linkedin_article"
	case host == "youtube.com" || host == "www.youtube.com" || host == "youtu.be":
		return "youtube_video"
	case strings.HasSuffix(p, ".pdf"):
		return "pdf"
	case strings.Contains(host, "substack.com") || strings.Contains(p, "/article") || strings.Contains(p, "/blog"):
		return "article"
	default:
		return "generic_web"
	}
}

func canonicalIdentity(canonical string) string {
	parsed, err := url.Parse(canonical)
	if err == nil && strings.EqualFold(parsed.Hostname(), "github.com") {
		parsed.Path = strings.ToLower(parsed.Path)
		return parsed.String()
	}
	return canonical
}

func stableID(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return prefix + hex.EncodeToString(digest[:])[:20]
}
