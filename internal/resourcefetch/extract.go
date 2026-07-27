package resourcefetch

import (
	"bytes"
	"compress/gzip"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Result contains bounded public context only. It is intentionally separate
// from the canonical schema; resourcequeue performs the later explicit merge.
type Result struct {
	State             string
	Reason            string
	Retryable         bool
	RetryAfterSeconds int
	RequestCount      int
	PolicyFingerprint string
	RetrievedAt       time.Time
	Profile           Profile
	MediaType         string
	Title             string
	Author            string
	PublishedAt       string
	Text              string
	RelatedURLs       []string
	Missingness       []string
	WireBytes         int64
	DecodedBytes      int64
	ExtractedBytes    int
	WallSeconds       int64
}

func boundedRead(reader io.Reader, maximum int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, reasonError(ReasonUnreachable)
	}
	if int64(len(payload)) > maximum {
		return nil, reasonError(ReasonBudgetExhausted)
	}
	return payload, nil
}

func decodeBody(wire []byte, encoding string, maximumDecodedBytes int64) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity":
		return append([]byte(nil), wire...), nil
	case "gzip", "x-gzip":
		reader, err := gzip.NewReader(bytes.NewReader(wire))
		if err != nil {
			return nil, reasonError(ReasonUnreachable)
		}
		defer reader.Close()
		return boundedRead(reader, maximumDecodedBytes)
	default:
		return nil, reasonError(ReasonUnsupportedMIME)
	}
}

var (
	tagPattern        = regexp.MustCompile(`(?is)<(?:script|style|noscript)[^>]*>.*?</(?:script|style|noscript)>|<[^>]+>`)
	titlePattern      = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	metaPattern       = regexp.MustCompile(`(?is)<meta\s+[^>]*(?:name|property)\s*=\s*["']?([^"'\s>]+)["']?[^>]*content\s*=\s*["']([^"']*)["'][^>]*>`)
	hrefPattern       = regexp.MustCompile(`(?is)\bhref\s*=\s*["']([^"']+)["']`)
	whitespacePattern = regexp.MustCompile(`\s+`)
)

func extract(decoded []byte, mediaType string, target PreparedURL, policy FrozenPolicy) (Result, error) {
	if len(decoded) > int(policy.MaximumDecodedBytes) {
		return Result{}, reasonError(ReasonBudgetExhausted)
	}
	profile := ProfileForHost(target.Host)
	text := string(decoded)
	result := Result{State: "complete", Profile: profile, MediaType: mediaType, DecodedBytes: int64(len(decoded))}
	if mediaType == "text/html" {
		result.Title, result.Author, result.PublishedAt = htmlMetadata(text)
		result.RelatedURLs = publicRelatedURLs(text, target)
		text = strings.TrimSpace(whitespacePattern.ReplaceAllString(tagPattern.ReplaceAllString(text, " "), " "))
	}
	if !utf8.ValidString(text) {
		return Result{}, reasonError(ReasonUnreachable)
	}
	if len([]byte(text)) > policy.MaximumExtractedBytes {
		return Result{}, reasonError(ReasonBudgetExhausted)
	}
	text = truncateUTF8(text, policy.MaximumExtractedBytes)
	result.Text, result.ExtractedBytes = text, len([]byte(text))
	if result.Text == "" {
		result.State = "partial"
		result.Missingness = append(result.Missingness, "readable_text_unavailable")
	}
	result.Missingness = append(result.Missingness, profileMissingness(profile, result)...)
	if len(result.Missingness) > 0 && result.State == "complete" {
		result.State = "partial"
	}
	return result, nil
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func htmlMetadata(source string) (title, author, published string) {
	if match := titlePattern.FindStringSubmatch(source); len(match) == 2 {
		title = strings.TrimSpace(whitespacePattern.ReplaceAllString(match[1], " "))
	}
	for _, match := range metaPattern.FindAllStringSubmatch(source, -1) {
		if len(match) != 3 {
			continue
		}
		key, value := strings.ToLower(strings.TrimSpace(match[1])), strings.TrimSpace(match[2])
		switch key {
		case "og:title", "twitter:title":
			if title == "" {
				title = value
			}
		case "author", "article:author":
			if author == "" {
				author = value
			}
		case "article:published_time", "date", "publishdate":
			if published == "" {
				published = value
			}
		}
	}
	return truncateUTF8(title, 4096), truncateUTF8(author, 1024), truncateUTF8(published, 1024)
}

func publicRelatedURLs(source string, target PreparedURL) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, match := range hrefPattern.FindAllStringSubmatch(source, -1) {
		if len(match) != 2 {
			continue
		}
		candidate := strings.TrimSpace(match[1])
		prepared, err := redirectTarget(target, candidate)
		if err != nil || sameHost(prepared.Host, target.Host) || seen[prepared.CanonicalURL] {
			continue
		}
		seen[prepared.CanonicalURL] = true
		result = append(result, prepared.CanonicalURL)
		if len(result) == 100 {
			break
		}
	}
	return result
}
