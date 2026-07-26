package personalmemory

import (
	"regexp"
	"strings"

	"github.com/synergyai-os/Mindline/internal/routing"
)

var personalSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:xox[baprs]-|xapp-|pb_sk_|github_pat_|gh[pousr]_|sk[-_](?:live|proj|svcacct|admin)[-_])\S+`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)\bbearer\s+\S{8,}`),
	regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|client[_-]?secret|private[_-]?key|session[_-]?token)\s*[:=]\s*[^\s,;]{6,}`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
}

func containsSecret(value string) bool {
	for _, pattern := range personalSecretPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func importedEvidenceContainsSecret(inputStrings ...string) bool {
	for _, value := range inputStrings {
		if containsSecret(strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func containsUnsafeURL(value string) bool {
	for _, match := range routing.URLPattern.FindAllString(value, -1) {
		raw := strings.TrimRight(match, ".,;:!?)]}")
		safe, state, err := routing.PrepareURLForStorage(raw)
		if err != nil || state == routing.URLStorageSensitiveRedacted || safe == "" || safe != raw {
			return true
		}
	}
	return false
}
