package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"unicode"
)

func RunID(sourceIDs []string) string {
	joined := strings.Join(sourceIDs, "\x00")
	sum := sha256.Sum256([]byte(joined))
	return "run-doc-" + hex.EncodeToString(sum[:])[:16]
}

func SourceDocumentID(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if containsUnsafeMarker(base) {
		return redactedDocumentID(filepath.Clean(path))
	}
	return "doc-" + sanitizeID(base)
}

func redactedDocumentID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "doc-redacted-" + hex.EncodeToString(sum[:])[:16]
}

func SegmentID(runID, sourceDocumentID string, headingPath []string, lineStart int, text string) string {
	seed := runID + "\x00" + sourceDocumentID + "\x00" + strings.Join(headingPath, "/") + "\x00" + string(rune(lineStart)) + "\x00" + strings.TrimSpace(text)
	sum := sha256.Sum256([]byte(seed))
	return "seg-" + hex.EncodeToString(sum[:])[:16]
}

func SegmentJSONPath(segmentID string) string {
	return filepath.ToSlash(filepath.Join("segments", sanitizeID(segmentID)+".json"))
}

func SegmentPreviewPath(segmentID string) string {
	return filepath.ToSlash(filepath.Join("previews", sanitizeID(segmentID)+".md"))
}

func sanitizeID(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	clean := strings.Trim(b.String(), "-_")
	for strings.Contains(clean, "--") {
		clean = strings.ReplaceAll(clean, "--", "-")
	}
	if clean == "" {
		return "segment"
	}
	return clean
}
