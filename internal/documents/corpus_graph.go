package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	CorpusGraphManifestSchemaVersion  = "corpus-graph-manifest/v0.1"
	CorpusGraphSummarySchemaVersion   = "corpus-graph-summary/v0.1"
	CorpusGraphAtomSchemaVersion      = "corpus-graph-atom/v0.1"
	CorpusGraphRelationSchemaVersion  = "corpus-graph-relation/v0.1"
	CorpusGraphReviewSchemaVersion    = "corpus-graph-review-item/v0.1"
	CorpusGraphAnswerKeySchemaVersion = "corpus-graph-answer-key/v0.1"
)

type CorpusGraphManifest struct {
	SchemaVersion string                      `json:"schema_version"`
	CorpusID      string                      `json:"corpus_id"`
	Sources       []CorpusGraphManifestSource `json:"sources"`
	AnswerKeyPath string                      `json:"answer_key_path,omitempty"`
}

type CorpusGraphManifestSource struct {
	SourceID       string `json:"source_id"`
	SourceKind     string `json:"source_kind"`
	Path           string `json:"path"`
	SemanticRunDir string `json:"semantic_run_dir,omitempty"`
}

type CorpusGraphSummary struct {
	SchemaVersion              string                       `json:"schema_version"`
	CorpusID                   string                       `json:"corpus_id"`
	SourceCount                int                          `json:"source_count"`
	SemanticRunCount           int                          `json:"semantic_run_count"`
	SkippedSourceCount         int                          `json:"skipped_source_count"`
	AtomCount                  int                          `json:"atom_count"`
	RelationCount              int                          `json:"relation_count"`
	RelationTypeCounts         map[CorpusRelationType]int   `json:"relation_type_counts"`
	RelationStatusCounts       map[ReviewStatus]int         `json:"relation_status_counts"`
	EvidenceReadyAtomCount     int                          `json:"evidence_ready_atom_count"`
	EvidenceReadyRelationCount int                          `json:"evidence_ready_relation_count"`
	BlockedCount               int                          `json:"blocked_count"`
	DuplicateClusterCount      int                          `json:"duplicate_cluster_count"`
	ReviewBurdenCount          int                          `json:"review_burden_count"`
	ReviewBurdenRatio          float64                      `json:"review_burden_ratio"`
	ReplayFingerprint          string                       `json:"replay_fingerprint"`
	RelationMetrics            CorpusRelationMetrics        `json:"relation_metrics"`
	ReadyForFiftyFilePressure  bool                         `json:"ready_for_50_file_pressure"`
	Blockers                   []string                     `json:"blockers"`
	Atoms                      []CorpusGraphSummaryAtom     `json:"atoms"`
	Relations                  []CorpusGraphSummaryRelation `json:"relations"`
}

type CorpusGraphSummaryAtom struct {
	AtomID        string                `json:"atom_id"`
	SourceID      string                `json:"source_id"`
	CandidateKind SemanticCandidateKind `json:"candidate_kind"`
	ReviewStatus  ReviewStatus          `json:"review_status"`
	AtomPath      string                `json:"atom_path"`
}

type CorpusGraphSummaryRelation struct {
	RelationID      string             `json:"relation_id"`
	RelationType    CorpusRelationType `json:"relation_type"`
	ReviewStatus    ReviewStatus       `json:"review_status"`
	Confidence      Confidence         `json:"confidence"`
	FromAtomID      string             `json:"from_atom_id"`
	FromSourceID    string             `json:"from_source_id"`
	FromSourceLabel string             `json:"from_source_label"`
	ToAtomID        string             `json:"to_atom_id"`
	ToSourceID      string             `json:"to_source_id"`
	ToSourceLabel   string             `json:"to_source_label"`
	RelationPath    string             `json:"relation_path"`
	ReviewPath      string             `json:"review_path"`
}

type CorpusRelationType string

const (
	CorpusRelationPossibleDuplicate CorpusRelationType = "possible_duplicate"
	CorpusRelationContradicts       CorpusRelationType = "contradicts"
	CorpusRelationSupersedes        CorpusRelationType = "supersedes"
	CorpusRelationSameTopicAs       CorpusRelationType = "same_topic_as"
)

type CorpusGraphAtom struct {
	SchemaVersion    string                    `json:"schema_version"`
	AtomID           string                    `json:"atom_id"`
	CorpusID         string                    `json:"corpus_id"`
	SourceID         string                    `json:"source_id"`
	SourceKind       string                    `json:"source_kind"`
	SourceLabel      string                    `json:"source_label"`
	SourceDocumentID string                    `json:"source_document_id"`
	CandidateKind    SemanticCandidateKind     `json:"candidate_kind"`
	ReviewStatus     ReviewStatus              `json:"review_status"`
	Confidence       Confidence                `json:"confidence"`
	Title            string                    `json:"title"`
	Summary          string                    `json:"summary"`
	LineStart        int                       `json:"line_start"`
	LineEnd          int                       `json:"line_end"`
	Excerpt          string                    `json:"excerpt"`
	ContentHash      string                    `json:"content_hash"`
	Provenance       CorpusGraphAtomProvenance `json:"provenance"`
	Blockers         []Blocker                 `json:"blockers"`
}

type CorpusGraphAtomProvenance struct {
	SemanticRunID          string   `json:"semantic_run_id,omitempty"`
	SemanticCandidateID    string   `json:"semantic_candidate_id,omitempty"`
	SemanticObservationIDs []string `json:"semantic_observation_ids,omitempty"`
	StructureNodeIDs       []string `json:"structure_node_ids,omitempty"`
	SemanticRelationIDs    []string `json:"semantic_relation_ids,omitempty"`
}

type CorpusGraphRelation struct {
	SchemaVersion string                        `json:"schema_version"`
	RelationID    string                        `json:"relation_id"`
	CorpusID      string                        `json:"corpus_id"`
	RelationType  CorpusRelationType            `json:"relation_type"`
	FromAtomID    string                        `json:"from_atom_id"`
	ToAtomID      string                        `json:"to_atom_id"`
	Confidence    Confidence                    `json:"confidence"`
	ReviewStatus  ReviewStatus                  `json:"review_status"`
	ReasonCode    string                        `json:"reason_code"`
	Evidence      []CorpusGraphRelationEvidence `json:"evidence"`
	Blockers      []Blocker                     `json:"blockers"`
}

type CorpusGraphRelationEvidence struct {
	AtomID           string `json:"atom_id"`
	SourceID         string `json:"source_id"`
	SourceLabel      string `json:"source_label"`
	SourceDocumentID string `json:"source_document_id"`
	LineStart        int    `json:"line_start"`
	LineEnd          int    `json:"line_end"`
	Excerpt          string `json:"excerpt"`
	ContentHash      string `json:"content_hash"`
}

type CorpusGraphReviewItem struct {
	SchemaVersion string             `json:"schema_version"`
	RelationID    string             `json:"relation_id"`
	RelationType  CorpusRelationType `json:"relation_type"`
	ReviewStatus  ReviewStatus       `json:"review_status"`
	ReasonCode    string             `json:"reason_code"`
	From          CorpusGraphAtom    `json:"from"`
	To            CorpusGraphAtom    `json:"to"`
}

type CorpusGraphAnswerKey struct {
	SchemaVersion string                      `json:"schema_version"`
	Relations     []CorpusGraphAnswerRelation `json:"relations"`
}

type CorpusGraphAnswerRelation struct {
	RelationType CorpusRelationType `json:"relation_type"`
	FromTitle    string             `json:"from_title"`
	ToTitle      string             `json:"to_title"`
}

type CorpusRelationMetrics struct {
	EvalCountedRelationCount int     `json:"eval_counted_relation_count"`
	TruePositiveCount        int     `json:"true_positive_count"`
	FalsePositiveCount       int     `json:"false_positive_count"`
	FalseNegativeCount       int     `json:"false_negative_count"`
	Precision                float64 `json:"precision"`
	Recall                   float64 `json:"recall"`
}

type corpusGraphBuild struct {
	Summary   CorpusGraphSummary
	Atoms     []CorpusGraphAtom
	Relations []CorpusGraphRelation
	Reviews   []CorpusGraphReviewItem
}

func BuildCorpusGraph(manifestPath string) (CorpusGraphSummary, []CorpusGraphAtom, []CorpusGraphRelation, []CorpusGraphReviewItem, error) {
	build, err := buildCorpusGraph(manifestPath)
	if err != nil {
		return CorpusGraphSummary{}, nil, nil, nil, err
	}
	return build.Summary, build.Atoms, build.Relations, build.Reviews, nil
}

func buildCorpusGraph(manifestPath string) (corpusGraphBuild, error) {
	manifest, manifestDir, err := loadCorpusGraphManifest(manifestPath)
	if err != nil {
		return corpusGraphBuild{}, err
	}
	atoms := []CorpusGraphAtom{}
	blockers := []string{}
	semanticRunCount := 0
	skipped := 0
	for _, source := range manifest.Sources {
		if strings.TrimSpace(source.SemanticRunDir) == "" {
			skipped++
			blockers = append(blockers, "source "+source.SourceID+" skipped: missing_semantic_run_dir")
			continue
		}
		sourcePath, err := containedManifestPath(manifestDir, source.Path)
		if err != nil {
			return corpusGraphBuild{}, fmt.Errorf("source path %s: %w", source.SourceID, err)
		}
		semanticRoot, err := containedManifestPath(manifestDir, source.SemanticRunDir)
		if err != nil {
			return corpusGraphBuild{}, fmt.Errorf("semantic_run_dir %s: %w", source.SourceID, err)
		}
		runAtoms, err := atomsFromSemanticRun(manifest.CorpusID, source, sourcePath, semanticRoot)
		if err != nil {
			if isMissingSemanticArtifactError(err) {
				skipped++
				blockers = append(blockers, "source "+source.SourceID+" skipped: missing_semantic_artifacts")
				continue
			}
			return corpusGraphBuild{}, err
		}
		if len(runAtoms) == 0 {
			skipped++
			blockers = append(blockers, "source "+source.SourceID+" skipped: no_semantic_candidates")
			continue
		}
		semanticRunCount++
		atoms = append(atoms, runAtoms...)
	}
	atoms = orderCorpusAtoms(atoms)
	relations := generateCorpusRelations(manifest.CorpusID, atoms)
	answerKey, err := loadCorpusGraphAnswerKey(manifestDir, manifest.AnswerKeyPath)
	if err != nil {
		return corpusGraphBuild{}, err
	}
	summary := buildCorpusGraphSummary(manifest, semanticRunCount, skipped, atoms, relations, answerKey, blockers)
	reviews := buildCorpusReviewItems(atoms, relations)
	return corpusGraphBuild{Summary: summary, Atoms: atoms, Relations: relations, Reviews: reviews}, nil
}

func loadCorpusGraphManifest(path string) (CorpusGraphManifest, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CorpusGraphManifest{}, "", err
	}
	var manifest CorpusGraphManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return CorpusGraphManifest{}, "", err
	}
	if manifest.SchemaVersion != CorpusGraphManifestSchemaVersion {
		return CorpusGraphManifest{}, "", fmt.Errorf("unsupported corpus graph manifest schema version: %s", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.CorpusID) == "" || sanitizeID(manifest.CorpusID) != manifest.CorpusID {
		return CorpusGraphManifest{}, "", fmt.Errorf("unsafe corpus id: %s", manifest.CorpusID)
	}
	if len(manifest.Sources) == 0 {
		return CorpusGraphManifest{}, "", fmt.Errorf("corpus graph manifest requires sources")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return CorpusGraphManifest{}, "", err
	}
	dir := filepath.Dir(abs)
	if err := rejectSymlinkAncestors(abs); err != nil {
		return CorpusGraphManifest{}, "", err
	}
	seen := map[string]bool{}
	for _, source := range manifest.Sources {
		if strings.TrimSpace(source.SourceID) == "" || sanitizeID(source.SourceID) != source.SourceID {
			return CorpusGraphManifest{}, "", fmt.Errorf("unsafe source id: %s", source.SourceID)
		}
		if seen[source.SourceID] {
			return CorpusGraphManifest{}, "", fmt.Errorf("duplicate source id: %s", source.SourceID)
		}
		seen[source.SourceID] = true
		if source.SourceKind != SourceKindMarkdown {
			return CorpusGraphManifest{}, "", fmt.Errorf("unsupported source kind: %s", source.SourceKind)
		}
		if _, err := containedManifestPath(dir, source.Path); err != nil {
			return CorpusGraphManifest{}, "", fmt.Errorf("source path %s: %w", source.SourceID, err)
		}
	}
	if manifest.AnswerKeyPath != "" {
		if _, err := containedManifestPath(dir, manifest.AnswerKeyPath); err != nil {
			return CorpusGraphManifest{}, "", fmt.Errorf("answer key path: %w", err)
		}
	}
	return manifest, dir, nil
}

func containedManifestPath(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path must be relative")
	}
	if strings.Contains(filepath.ToSlash(relative), "../") || strings.HasPrefix(filepath.ToSlash(relative), "..") {
		return "", fmt.Errorf("path escaped manifest directory")
	}
	target := filepath.Join(root, relative)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if !isInside(absRoot, absTarget) {
		return "", fmt.Errorf("path escaped manifest directory")
	}
	if err := rejectSymlinkAncestors(absTarget); err != nil {
		return "", err
	}
	return absTarget, nil
}

func atomsFromSemanticRun(corpusID string, source CorpusGraphManifestSource, sourcePath string, semanticRoot string) ([]CorpusGraphAtom, error) {
	root := semanticRoot
	if _, err := os.Stat(filepath.Join(root, "semantic-summary.json")); err != nil {
		root = filepath.Join(semanticRoot, "semantic-candidates")
	}
	data, err := os.ReadFile(filepath.Join(root, "semantic-summary.json"))
	if err != nil {
		return nil, fmt.Errorf("read semantic summary for %s: %w", source.SourceID, err)
	}
	var summary SemanticSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}
	out := []CorpusGraphAtom{}
	for _, item := range summary.Candidates {
		candidatePath, err := containedArtifactPath(root, item.CandidatePath)
		if err != nil {
			return nil, err
		}
		candidateData, err := os.ReadFile(candidatePath)
		if err != nil {
			return nil, err
		}
		var candidate SemanticCandidate
		if err := json.Unmarshal(candidateData, &candidate); err != nil {
			return nil, err
		}
		atom := atomFromSemanticCandidate(corpusID, source, sourcePath, summary.RunID, candidate)
		out = append(out, atom)
	}
	return out, nil
}

func isMissingSemanticArtifactError(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, os.ErrNotExist)
}

func containedArtifactPath(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("artifact path escaped semantic run directory")
	}
	cleanSlash := filepath.ToSlash(relative)
	if strings.HasPrefix(cleanSlash, "../") || strings.Contains(cleanSlash, "/../") || cleanSlash == ".." {
		return "", fmt.Errorf("artifact path escaped semantic run directory")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(absRoot, relative)
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if !isInside(absRoot, absTarget) || absRoot == absTarget {
		return "", fmt.Errorf("artifact path escaped semantic run directory")
	}
	if err := rejectSymlinkAncestors(absTarget); err != nil {
		return "", err
	}
	return absTarget, nil
}

func atomFromSemanticCandidate(corpusID string, source CorpusGraphManifestSource, sourcePath string, runID string, candidate SemanticCandidate) CorpusGraphAtom {
	lineStart, lineEnd := 0, 0
	if len(candidate.EvidenceRanges) > 0 {
		lineStart = candidate.EvidenceRanges[0].LineStart
		lineEnd = candidate.EvidenceRanges[0].LineEnd
	}
	excerpt := ""
	if len(candidate.EvidenceExcerpts) > 0 {
		excerpt = candidate.EvidenceExcerpts[0].Text
	}
	if strings.TrimSpace(excerpt) == "" && lineStart > 0 && lineEnd >= lineStart {
		excerpt = excerptFromFile(sourcePath, lineStart, lineEnd)
	}
	hash := corpusContentHash(excerpt)
	atom := CorpusGraphAtom{
		SchemaVersion:    CorpusGraphAtomSchemaVersion,
		CorpusID:         corpusID,
		SourceID:         source.SourceID,
		SourceKind:       source.SourceKind,
		SourceLabel:      filepath.ToSlash(source.Path),
		SourceDocumentID: candidate.SourceDocumentID,
		CandidateKind:    candidate.CandidateKind,
		ReviewStatus:     candidate.ReviewStatus,
		Confidence:       candidate.Confidence,
		Title:            candidate.Title,
		Summary:          candidate.Summary,
		LineStart:        lineStart,
		LineEnd:          lineEnd,
		Excerpt:          excerpt,
		ContentHash:      hash,
		Provenance: CorpusGraphAtomProvenance{
			SemanticRunID:          runID,
			SemanticCandidateID:    candidate.CandidateID,
			SemanticObservationIDs: cloneStringList(candidate.ObservationIDs),
			SemanticRelationIDs:    cloneStringList(candidate.RelationIDs),
		},
		Blockers: cloneBlockerList(candidate.Blockers),
	}
	for _, evidence := range candidate.EvidenceRanges {
		atom.Provenance.StructureNodeIDs = append(atom.Provenance.StructureNodeIDs, evidence.StructureNodeID)
	}
	if atom.SourceDocumentID == "" || atom.LineStart <= 0 || atom.LineEnd <= 0 || strings.TrimSpace(atom.Excerpt) == "" {
		atom.ReviewStatus = ReviewStatusBlocked
		atom.Confidence = ConfidenceLow
		atom.Blockers = append(atom.Blockers, Blocker{Code: "missing_evidence", Message: "Corpus graph atom is missing source document, line span, or excerpt."})
	}
	atom.AtomID = CorpusAtomID(atom)
	return atom
}

func excerptFromFile(path string, lineStart, lineEnd int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if lineStart < 1 || lineStart > len(lines) {
		return ""
	}
	if lineEnd < lineStart {
		lineEnd = lineStart
	}
	if lineEnd > len(lines) {
		lineEnd = len(lines)
	}
	return strings.TrimSpace(strings.Join(lines[lineStart-1:lineEnd], "\n"))
}

func CorpusAtomID(atom CorpusGraphAtom) string {
	seed := strings.Join([]string{
		atom.CorpusID,
		atom.SourceID,
		atom.SourceLabel,
		string(atom.CandidateKind),
		normalizeGraphText(atom.Title),
		normalizeGraphText(atom.Summary),
		strconv.Itoa(atom.LineStart),
		strconv.Itoa(atom.LineEnd),
		atom.ContentHash,
	}, "\x00")
	return "atom-" + shortHash(seed)
}

func CorpusRelationID(corpusID string, relationType CorpusRelationType, fromAtomID, toAtomID, reasonCode string) string {
	a, b := relationKeyPair(relationType, fromAtomID, toAtomID)
	return "crel-" + shortHash(strings.Join([]string{corpusID, string(relationType), a, b, reasonCode}, "\x00"))
}

func generateCorpusRelations(corpusID string, atoms []CorpusGraphAtom) []CorpusGraphRelation {
	out := []CorpusGraphRelation{}
	seen := map[string]bool{}
	for i := 0; i < len(atoms); i++ {
		for j := i + 1; j < len(atoms); j++ {
			for _, relation := range relationsForAtomPair(corpusID, atoms[i], atoms[j]) {
				if !seen[relation.RelationID] {
					seen[relation.RelationID] = true
					out = append(out, relation)
				}
			}
		}
	}
	return orderCorpusRelations(out)
}

func relationsForAtomPair(corpusID string, a, b CorpusGraphAtom) []CorpusGraphRelation {
	out := []CorpusGraphRelation{}
	titleA := normalizeGraphText(a.Title)
	titleB := normalizeGraphText(b.Title)
	summaryA := normalizeGraphText(a.Summary)
	summaryB := normalizeGraphText(b.Summary)
	if titleA != "" && titleA == titleB {
		out = append(out, corpusRelation(corpusID, CorpusRelationPossibleDuplicate, a, b, ConfidenceHigh, ReviewStatusReady, "same_normalized_title"))
	}
	if markerMatch(a, b, "contradicts") {
		out = append(out, corpusRelation(corpusID, CorpusRelationContradicts, a, b, ConfidenceHigh, ReviewStatusReady, "seeded_contradiction_marker"))
	}
	if target := markerTarget(a, "supersedes"); markerTargetMatchesTitle(target, normalizeGraphText(b.Title)) {
		out = append(out, corpusRelation(corpusID, CorpusRelationSupersedes, a, b, ConfidenceHigh, ReviewStatusReady, "seeded_supersession_marker"))
	} else if target := markerTarget(b, "supersedes"); markerTargetMatchesTitle(target, normalizeGraphText(a.Title)) {
		out = append(out, corpusRelation(corpusID, CorpusRelationSupersedes, b, a, ConfidenceHigh, ReviewStatusReady, "seeded_supersession_marker"))
	}
	if len(out) == 0 && sharedTopicScore(titleA+" "+summaryA, titleB+" "+summaryB) >= 2 {
		out = append(out, corpusRelation(corpusID, CorpusRelationSameTopicAs, a, b, ConfidenceMedium, ReviewStatusReady, "shared_topic_terms"))
	}
	return out
}

func corpusRelation(corpusID string, relationType CorpusRelationType, from, to CorpusGraphAtom, confidence Confidence, status ReviewStatus, reason string) CorpusGraphRelation {
	relation := CorpusGraphRelation{
		SchemaVersion: CorpusGraphRelationSchemaVersion,
		CorpusID:      corpusID,
		RelationType:  relationType,
		FromAtomID:    from.AtomID,
		ToAtomID:      to.AtomID,
		Confidence:    confidence,
		ReviewStatus:  status,
		ReasonCode:    reason,
		Evidence:      []CorpusGraphRelationEvidence{relationEvidence(from), relationEvidence(to)},
	}
	if from.ReviewStatus == ReviewStatusBlocked || to.ReviewStatus == ReviewStatusBlocked {
		relation.ReviewStatus = ReviewStatusBlocked
		relation.Confidence = ConfidenceLow
		relation.Blockers = append(relation.Blockers, Blocker{Code: "blocked_atom", Message: "Relation includes a blocked atom."})
	}
	relation.RelationID = CorpusRelationID(corpusID, relationType, from.AtomID, to.AtomID, reason)
	return relation
}

func relationEvidence(atom CorpusGraphAtom) CorpusGraphRelationEvidence {
	return CorpusGraphRelationEvidence{
		AtomID: atom.AtomID, SourceID: atom.SourceID, SourceLabel: atom.SourceLabel,
		SourceDocumentID: atom.SourceDocumentID, LineStart: atom.LineStart, LineEnd: atom.LineEnd,
		Excerpt: atom.Excerpt, ContentHash: atom.ContentHash,
	}
}

func markerMatch(a, b CorpusGraphAtom, prefix string) bool {
	targetA := markerTarget(a, prefix)
	targetB := markerTarget(b, prefix)
	titleA := normalizeGraphText(a.Title)
	titleB := normalizeGraphText(b.Title)
	return markerTargetMatchesTitle(targetA, titleB) || markerTargetMatchesTitle(targetB, titleA)
}

func markerTargetMatchesTitle(target, title string) bool {
	return target != "" && title != "" && (target == title || strings.HasPrefix(target, title))
}

func markerTarget(atom CorpusGraphAtom, prefix string) string {
	text := normalizeGraphText(atom.Summary)
	idx := strings.Index(text, prefix)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(text[idx+len(prefix):])
	if rest == "" {
		return ""
	}
	if stop := strings.Index(rest, ";"); stop >= 0 {
		rest = rest[:stop]
	}
	return strings.TrimSpace(rest)
}

func sharedTopicScore(a, b string) int {
	termsA := graphTerms(a)
	termsB := graphTerms(b)
	score := 0
	for term := range termsA {
		if termsB[term] {
			score++
		}
	}
	return score
}

func graphTerms(value string) map[string]bool {
	out := map[string]bool{}
	for _, term := range strings.Fields(value) {
		term = strings.Trim(term, "-_")
		if len(term) >= 5 {
			out[term] = true
		}
	}
	return out
}

func buildCorpusGraphSummary(manifest CorpusGraphManifest, semanticRunCount, skipped int, atoms []CorpusGraphAtom, relations []CorpusGraphRelation, answerKey *CorpusGraphAnswerKey, blockers []string) CorpusGraphSummary {
	summary := CorpusGraphSummary{
		SchemaVersion: CorpusGraphSummarySchemaVersion,
		CorpusID:      manifest.CorpusID, SourceCount: len(manifest.Sources), SemanticRunCount: semanticRunCount, SkippedSourceCount: skipped,
		AtomCount: len(atoms), RelationCount: len(relations), RelationTypeCounts: map[CorpusRelationType]int{}, RelationStatusCounts: map[ReviewStatus]int{},
		Blockers: append([]string(nil), blockers...),
	}
	for _, atom := range atoms {
		if atom.ReviewStatus != ReviewStatusBlocked && len(atom.Blockers) == 0 {
			summary.EvidenceReadyAtomCount++
		} else {
			summary.BlockedCount++
		}
		summary.Atoms = append(summary.Atoms, CorpusGraphSummaryAtom{AtomID: atom.AtomID, SourceID: atom.SourceID, CandidateKind: atom.CandidateKind, ReviewStatus: atom.ReviewStatus, AtomPath: CorpusAtomJSONPath(atom.AtomID)})
	}
	atomsByID := map[string]CorpusGraphAtom{}
	for _, atom := range atoms {
		atomsByID[atom.AtomID] = atom
	}
	for _, relation := range relations {
		summary.RelationTypeCounts[relation.RelationType]++
		summary.RelationStatusCounts[relation.ReviewStatus]++
		if relation.ReviewStatus == ReviewStatusNeedsReview || relation.ReviewStatus == ReviewStatusBlocked {
			summary.ReviewBurdenCount++
		}
		if relation.ReviewStatus == ReviewStatusBlocked || len(relation.Blockers) > 0 {
			summary.BlockedCount++
		} else if relationHasEvidence(relation) {
			summary.EvidenceReadyRelationCount++
		}
		from := atomsByID[relation.FromAtomID]
		to := atomsByID[relation.ToAtomID]
		summary.Relations = append(summary.Relations, CorpusGraphSummaryRelation{
			RelationID:      relation.RelationID,
			RelationType:    relation.RelationType,
			ReviewStatus:    relation.ReviewStatus,
			Confidence:      relation.Confidence,
			FromAtomID:      relation.FromAtomID,
			FromSourceID:    from.SourceID,
			FromSourceLabel: from.SourceLabel,
			ToAtomID:        relation.ToAtomID,
			ToSourceID:      to.SourceID,
			ToSourceLabel:   to.SourceLabel,
			RelationPath:    CorpusRelationJSONPath(relation.RelationID),
			ReviewPath:      CorpusReviewJSONPath(relation.RelationID),
		})
	}
	summary.DuplicateClusterCount = duplicateClusterCount(relations)
	if len(relations) > 0 {
		summary.ReviewBurdenRatio = float64(summary.ReviewBurdenCount) / float64(len(relations))
	}
	if answerKey != nil {
		summary.RelationMetrics = evaluateCorpusRelations(relations, *answerKey, atoms)
	}
	if summary.EvidenceReadyAtomCount == len(atoms) && summary.EvidenceReadyRelationCount == len(relations) && summary.BlockedCount == 0 && skipped == 0 {
		summary.ReadyForFiftyFilePressure = true
	}
	summary.ReplayFingerprint = corpusReplayFingerprint(summary, atoms, relations)
	sort.Slice(summary.Atoms, func(i, j int) bool { return summary.Atoms[i].AtomID < summary.Atoms[j].AtomID })
	sort.Slice(summary.Relations, func(i, j int) bool { return summary.Relations[i].RelationID < summary.Relations[j].RelationID })
	return summary
}

func duplicateClusterCount(relations []CorpusGraphRelation) int {
	parent := map[string]string{}
	var find func(string) string
	find = func(atomID string) string {
		if parent[atomID] == "" {
			parent[atomID] = atomID
			return atomID
		}
		if parent[atomID] != atomID {
			parent[atomID] = find(parent[atomID])
		}
		return parent[atomID]
	}
	union := func(a, b string) {
		rootA := find(a)
		rootB := find(b)
		if rootA != rootB {
			parent[rootB] = rootA
		}
	}
	for _, relation := range relations {
		if relation.RelationType != CorpusRelationPossibleDuplicate {
			continue
		}
		union(relation.FromAtomID, relation.ToAtomID)
	}
	clusters := map[string]bool{}
	for atomID := range parent {
		clusters[find(atomID)] = true
	}
	return len(clusters)
}

func evaluateCorpusRelations(relations []CorpusGraphRelation, answerKey CorpusGraphAnswerKey, atoms []CorpusGraphAtom) CorpusRelationMetrics {
	atomTitles := map[string]string{}
	for _, atom := range atoms {
		atomTitles[atom.AtomID] = normalizeGraphText(atom.Title)
	}
	expected := []corpusExpectedRelation{}
	for _, relation := range answerKey.Relations {
		expected = append(expected, corpusExpectedRelation{
			RelationType: relation.RelationType,
			FromTitle:    normalizeGraphText(relation.FromTitle),
			ToTitle:      normalizeGraphText(relation.ToTitle),
		})
	}
	m := CorpusRelationMetrics{}
	for _, relation := range relations {
		if relation.ReviewStatus != ReviewStatusReady || !relationHasEvidence(relation) {
			continue
		}
		if !answerKeyTypePresent(answerKey, relation.RelationType) {
			continue
		}
		m.EvalCountedRelationCount++
		if match := matchingExpectedRelationIndex(relation, expected, atomTitles); match >= 0 {
			expected[match].Matched = true
			m.TruePositiveCount++
		} else {
			m.FalsePositiveCount++
		}
	}
	for _, relation := range expected {
		if !relation.Matched {
			m.FalseNegativeCount++
		}
	}
	if m.TruePositiveCount+m.FalsePositiveCount > 0 {
		m.Precision = float64(m.TruePositiveCount) / float64(m.TruePositiveCount+m.FalsePositiveCount)
	}
	if m.TruePositiveCount+m.FalseNegativeCount > 0 {
		m.Recall = float64(m.TruePositiveCount) / float64(m.TruePositiveCount+m.FalseNegativeCount)
	}
	return m
}

type corpusExpectedRelation struct {
	RelationType CorpusRelationType
	FromTitle    string
	ToTitle      string
	Matched      bool
}

func matchingExpectedRelationIndex(relation CorpusGraphRelation, expected []corpusExpectedRelation, atomTitles map[string]string) int {
	for i, candidate := range expected {
		if candidate.Matched || !relationMatchesExpected(relation, candidate, atomTitles) {
			continue
		}
		return i
	}
	return -1
}

func relationMatchesExpected(relation CorpusGraphRelation, expected corpusExpectedRelation, atomTitles map[string]string) bool {
	if relation.RelationType != expected.RelationType {
		return false
	}
	fromTitle := atomTitles[relation.FromAtomID]
	toTitle := atomTitles[relation.ToAtomID]
	return answerKeyRelationKey(relation.RelationType, fromTitle, toTitle) == answerKeyRelationKey(expected.RelationType, expected.FromTitle, expected.ToTitle)
}

func answerKeyTypePresent(answerKey CorpusGraphAnswerKey, relationType CorpusRelationType) bool {
	for _, relation := range answerKey.Relations {
		if relation.RelationType == relationType {
			return true
		}
	}
	return false
}

func answerKeyRelationKey(relationType CorpusRelationType, a, b string) string {
	x, y := relationKeyPair(relationType, a, b)
	return string(relationType) + "\x00" + x + "\x00" + y
}

func buildCorpusReviewItems(atoms []CorpusGraphAtom, relations []CorpusGraphRelation) []CorpusGraphReviewItem {
	byID := map[string]CorpusGraphAtom{}
	for _, atom := range atoms {
		byID[atom.AtomID] = atom
	}
	out := []CorpusGraphReviewItem{}
	for _, relation := range relations {
		out = append(out, CorpusGraphReviewItem{SchemaVersion: CorpusGraphReviewSchemaVersion, RelationID: relation.RelationID, RelationType: relation.RelationType, ReviewStatus: relation.ReviewStatus, ReasonCode: relation.ReasonCode, From: byID[relation.FromAtomID], To: byID[relation.ToAtomID]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelationID < out[j].RelationID })
	return out
}

func loadCorpusGraphAnswerKey(root, relative string) (*CorpusGraphAnswerKey, error) {
	if strings.TrimSpace(relative) == "" {
		return nil, nil
	}
	path, err := containedManifestPath(root, relative)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var answerKey CorpusGraphAnswerKey
	if err := json.Unmarshal(data, &answerKey); err != nil {
		return nil, err
	}
	if answerKey.SchemaVersion != CorpusGraphAnswerKeySchemaVersion {
		return nil, fmt.Errorf("unsupported corpus graph answer key schema version: %s", answerKey.SchemaVersion)
	}
	return &answerKey, nil
}

func relationHasEvidence(relation CorpusGraphRelation) bool {
	if len(relation.Evidence) < 2 {
		return false
	}
	for _, evidence := range relation.Evidence {
		if evidence.SourceID == "" || evidence.SourceDocumentID == "" || evidence.SourceLabel == "" || evidence.LineStart <= 0 || evidence.LineEnd <= 0 || strings.TrimSpace(evidence.Excerpt) == "" {
			return false
		}
	}
	return true
}

func corpusReplayFingerprint(summary CorpusGraphSummary, atoms []CorpusGraphAtom, relations []CorpusGraphRelation) string {
	parts := []string{summary.CorpusID, strconv.Itoa(summary.AtomCount), strconv.Itoa(summary.RelationCount)}
	for _, atom := range orderCorpusAtoms(atoms) {
		parts = append(parts, atom.AtomID, atom.SourceID, atom.SourceDocumentID, strconv.Itoa(atom.LineStart), strconv.Itoa(atom.LineEnd), atom.ContentHash)
	}
	for _, relation := range orderCorpusRelations(relations) {
		parts = append(parts, relation.RelationID, string(relation.RelationType), string(relation.ReviewStatus), relation.ReasonCode)
	}
	return "cgfp-" + shortHash(strings.Join(parts, "\x00"))
}

func corpusContentHash(value string) string {
	return "sha256:" + shortHash(value)
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func normalizeGraphText(value string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		case !lastSpace:
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func orderedPair(a, b string) (string, string) {
	if a <= b {
		return a, b
	}
	return b, a
}

func relationKeyPair(relationType CorpusRelationType, a, b string) (string, string) {
	switch relationType {
	case CorpusRelationPossibleDuplicate, CorpusRelationSameTopicAs, CorpusRelationContradicts:
		return orderedPair(a, b)
	default:
		return a, b
	}
}

func orderCorpusAtoms(atoms []CorpusGraphAtom) []CorpusGraphAtom {
	out := append([]CorpusGraphAtom(nil), atoms...)
	sort.Slice(out, func(i, j int) bool { return out[i].AtomID < out[j].AtomID })
	return out
}

func orderCorpusRelations(relations []CorpusGraphRelation) []CorpusGraphRelation {
	out := append([]CorpusGraphRelation(nil), relations...)
	sort.Slice(out, func(i, j int) bool { return out[i].RelationID < out[j].RelationID })
	return out
}
