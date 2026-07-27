package contentguard

import (
	"regexp"
	"strings"

	"github.com/synergyai-os/Mindline/internal/routing"
)

var secretLikePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:xox[baprs]-|xapp-|pb_sk_|github_pat_|gh[pousr]_|sk[-_](?:live|proj|svcacct|admin)[-_])\S+`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)\bbearer\s+\S{8,}`),
	regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|client[_-]?secret|private[_-]?key|session[_-]?token)\s*[:=]\s*[^\s,;]{6,}`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
}

// ContainsSecretLike reports whether a value contains credential-shaped
// material that must not cross a durable boundary.
func ContainsSecretLike(value string) bool {
	for _, pattern := range secretLikePatterns {
		if pattern.MatchString(strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

// ContainsNonPersistableURL reports whether any lexical HTTP(S) occurrence
// would be rejected, redacted, or changed by the durable URL policy.
func ContainsNonPersistableURL(value string) bool {
	for _, raw := range routing.ExtractLexicalURLOccurrences(value) {
		safe, state, err := routing.PrepareURLForStorage(raw)
		if err != nil || state == routing.URLStorageSensitiveRedacted || safe == "" || safe != raw {
			return true
		}
	}
	return false
}

// ContainsNonPersistableContent applies both credential and URL policy to all
// supplied text. Callers must still decide whether to reject, redact, or emit a
// content-free shell at their own trust boundary.
func ContainsNonPersistableContent(values ...string) bool {
	for _, value := range values {
		if ContainsSecretLike(value) || ContainsNonPersistableURL(value) {
			return true
		}
	}
	return false
}
