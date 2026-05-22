package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

func RunID(sourceIDs []string) string {
	joined := strings.Join(sourceIDs, "\x00")
	sum := sha256.Sum256([]byte(joined))
	return "run-doc-" + hex.EncodeToString(sum[:])[:16]
}

func StructureRunID(sourceIDs []string, contentHashes []string) string {
	parts := append([]string(nil), sourceIDs...)
	parts = append(parts, contentHashes...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "run-struct-" + hex.EncodeToString(sum[:])[:16]
}

func SemanticRunID(structureRunID string, nodeIDs []string) string {
	parts := append([]string{structureRunID}, sortedStrings(nodeIDs)...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "run-sem-" + hex.EncodeToString(sum[:])[:16]
}

func SourceDocumentID(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if containsUnsafeMarker(base) {
		return redactedDocumentID(base)
	}
	return "doc-" + sanitizeID(base)
}

func DisambiguatedSourceDocumentID(path string) string {
	return disambiguatedSourceDocumentID(path, filepath.Clean(path))
}

func disambiguatedSourceDocumentID(path, disambiguator string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if containsUnsafeMarker(base) {
		return redactedDocumentID(disambiguator)
	}
	sum := sha256.Sum256([]byte(disambiguator))
	return "doc-" + sanitizeID(base) + "-" + hex.EncodeToString(sum[:])[:8]
}

func redactedDocumentID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "doc-redacted-" + hex.EncodeToString(sum[:])[:16]
}

func SegmentID(runID, sourceDocumentID string, headingPath []string, lineStart int, text string) string {
	seed := runID + "\x00" + sourceDocumentID + "\x00" + strings.Join(headingPath, "/") + "\x00" + strconv.Itoa(lineStart) + "\x00" + strings.TrimSpace(text)
	sum := sha256.Sum256([]byte(seed))
	return "seg-" + hex.EncodeToString(sum[:])[:16]
}

func SegmentJSONPath(segmentID string) string {
	return filepath.ToSlash(filepath.Join("segments", sanitizeID(segmentID)+".json"))
}

func SegmentPreviewPath(segmentID string) string {
	return filepath.ToSlash(filepath.Join("previews", sanitizeID(segmentID)+".md"))
}

func StructureNodeID(runID, sourceDocumentID string, nodeType StructureNodeType, structuralPath []string, lineStart, lineEnd int, contentHash string) string {
	seed := runID + "\x00" + sourceDocumentID + "\x00" + string(nodeType) + "\x00" + strings.Join(structuralPath, "/") + "\x00" + strconv.Itoa(lineStart) + "\x00" + strconv.Itoa(lineEnd) + "\x00" + contentHash
	sum := sha256.Sum256([]byte(seed))
	return "node-" + hex.EncodeToString(sum[:])[:16]
}

func StructureNodeJSONPath(nodeID string) string {
	return filepath.ToSlash(filepath.Join("nodes", sanitizeID(nodeID)+".json"))
}

func StructureNodePreviewPath(nodeID string) string {
	return filepath.ToSlash(filepath.Join("previews", sanitizeID(nodeID)+".md"))
}

func SemanticObservationID(runID, nodeID string, kind SemanticObservationKind, title string) string {
	seed := strings.Join([]string{runID, nodeID, string(kind), strings.TrimSpace(title)}, "\x00")
	sum := sha256.Sum256([]byte(seed))
	return "obs-" + hex.EncodeToString(sum[:])[:16]
}

func SemanticCandidateID(runID string, kind SemanticCandidateKind, sourceDocumentID, title string, evidenceNodes []string) string {
	parts := []string{runID, string(kind), sourceDocumentID, strings.TrimSpace(title)}
	parts = append(parts, sortedStrings(evidenceNodes)...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "cand-" + hex.EncodeToString(sum[:])[:16]
}

func SemanticRelationID(runID string, relationshipType SemanticRelationshipType, fromID, toID string) string {
	seed := strings.Join([]string{runID, string(relationshipType), fromID, toID}, "\x00")
	sum := sha256.Sum256([]byte(seed))
	return "rel-" + hex.EncodeToString(sum[:])[:16]
}

func SemanticObservationJSONPath(observationID string) string {
	return filepath.ToSlash(filepath.Join("observations", sanitizeID(observationID)+".json"))
}

func SemanticCandidateJSONPath(candidateID string) string {
	return filepath.ToSlash(filepath.Join("candidates", sanitizeID(candidateID)+".json"))
}

func SemanticRelationJSONPath(relationID string) string {
	return filepath.ToSlash(filepath.Join("relations", sanitizeID(relationID)+".json"))
}

func SemanticPreviewPath(id string) string {
	return filepath.ToSlash(filepath.Join("previews", sanitizeID(id)+".md"))
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
