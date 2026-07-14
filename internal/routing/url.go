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
	"strings"
)

var URLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

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
