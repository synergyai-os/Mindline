package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	CorpusConceptsSchemaVersion             = "corpus-concepts/v0.2"
	CorpusConceptReviewRecordsSchemaVersion = "corpus-concept-review-records/v0.2"
	CorpusConceptsDirName                   = "corpus-concepts"
	DefaultCorpusConceptsMax                = 40
	corpusConceptMaxAtoms                   = 18
	corpusConceptPreviewMax                 = 8
	corpusConceptSourcePreviewMax           = 3
)

type CorpusConceptSection string

const (
	CorpusConceptSectionCrossSource CorpusConceptSection = "cross_source"
	CorpusConceptSectionLocal       CorpusConceptSection = "local"
	CorpusConceptSectionNeedsReview CorpusConceptSection = "needs_review"
	CorpusConceptSectionBlocked     CorpusConceptSection = "blocked"
)

type CorpusConceptSummary struct {
	SchemaVersion                     string                              `json:"schema_version"`
	CorpusID                          string                              `json:"corpus_id"`
	SourceCount                       int                                 `json:"source_count"`
	ProcessedSourceCount              int                                 `json:"processed_source_count"`
	AtomCount                         int                                 `json:"atom_count"`
	RelationCount                     int                                 `json:"relation_count"`
	ConceptCount                      int                                 `json:"concept_count"`
	GeneratedConceptCount             int                                 `json:"generated_concept_count,omitempty"`
	CrossSourceConceptCount           int                                 `json:"cross_source_concept_count"`
	LocalConceptCount                 int                                 `json:"local_concept_count"`
	NeedsReviewConceptCount           int                                 `json:"needs_review_concept_count"`
	BlockedConceptCount               int                                 `json:"blocked_concept_count"`
	ConceptReviewCount                int                                 `json:"concept_review_count"`
	CleanupTriageCount                int                                 `json:"cleanup_triage_count"`
	EnrichmentBacklogCount            int                                 `json:"enrichment_backlog_count"`
	BlockedDiagnosticCount            int                                 `json:"blocked_diagnostic_count"`
	EvidenceReferenceCount            int                                 `json:"evidence_reference_count"`
	CrossSourceEvidenceReferenceCount int                                 `json:"cross_source_evidence_reference_count"`
	ConceptReviewBurdenCount          int                                 `json:"concept_review_burden_count"`
	ConceptReviewBurdenRatio          float64                             `json:"concept_review_burden_ratio"`
	RelationReviewCompressionRatio    float64                             `json:"relation_review_compression_ratio"`
	AtomCoverageRatio                 float64                             `json:"atom_coverage_ratio"`
	CrossSourceAtomRatio              float64                             `json:"cross_source_atom_ratio"`
	SourceKindCoverage                map[string]int                      `json:"source_kind_coverage"`
	CrossSourceKindPairCount          int                                 `json:"cross_source_kind_pair_count"`
	MaxConceptCount                   int                                 `json:"max_concept_count"`
	OmittedConceptCount               int                                 `json:"omitted_concept_count,omitempty"`
	OmittedAtomCount                  int                                 `json:"omitted_atom_count,omitempty"`
	ScaleStatus                       string                              `json:"scale_status"`
	ScaleReasonCodes                  []string                            `json:"scale_reason_codes,omitempty"`
	NonGeneralizableRuntime           bool                                `json:"non_generalizable_runtime"`
	Comparable                        bool                                `json:"comparable"`
	Guardrails                        CorpusPressureGuardrailCounters     `json:"guardrails"`
	CorpusFingerprint                 string                              `json:"corpus_fingerprint"`
	CommandConfigFingerprint          string                              `json:"command_config_fingerprint"`
	PressureReplayFingerprint         string                              `json:"pressure_replay_fingerprint"`
	GraphReplayFingerprint            string                              `json:"graph_replay_fingerprint"`
	ReplayFingerprint                 string                              `json:"replay_fingerprint"`
	ConceptIndexPath                  string                              `json:"concept_index_path"`
	ReviewPacketPath                  string                              `json:"review_packet_path"`
	SectionCounts                     map[CorpusConceptSection]int        `json:"section_counts"`
	CandidateKindCounts               map[SemanticCandidateKind]int       `json:"candidate_kind_counts"`
	ReviewWorkKindCounts              map[CorpusConceptReviewWorkKind]int `json:"review_work_kind_counts"`
	Concepts                          []CorpusConceptListItem             `json:"concepts"`
}

type CorpusConceptIndex struct {
	SchemaVersion string          `json:"schema_version"`
	CorpusID      string          `json:"corpus_id"`
	Concepts      []CorpusConcept `json:"concepts"`
}

type CorpusConceptListItem struct {
	ConceptID              string                           `json:"concept_id"`
	Title                  string                           `json:"title"`
	ReviewPrompt           string                           `json:"review_prompt"`
	GroupingRationale      string                           `json:"grouping_rationale"`
	CandidateMeaning       string                           `json:"candidate_meaning"`
	AcceptMeaning          string                           `json:"accept_meaning"`
	DecisionRubric         []CorpusConceptDecisionCriterion `json:"decision_rubric,omitempty"`
	SourceContributions    []string                         `json:"source_contributions,omitempty"`
	ConceptKey             string                           `json:"concept_key"`
	Section                CorpusConceptSection             `json:"section"`
	CandidateKind          SemanticCandidateKind            `json:"candidate_kind"`
	AtomCount              int                              `json:"atom_count"`
	SourceCount            int                              `json:"source_count"`
	EvidenceReferenceCount int                              `json:"evidence_reference_count"`
	SourceKindCoverage     map[string]int                   `json:"source_kind_coverage"`
	ReviewStatus           ReviewStatus                     `json:"review_status"`
	ReviewWorkKind         CorpusConceptReviewWorkKind      `json:"review_work_kind"`
	ReasonCodes            []string                         `json:"reason_codes,omitempty"`
	ConceptPath            string                           `json:"concept_path"`
	RepresentativeEvidence int                              `json:"representative_evidence_count"`
	SourceEvidence         int                              `json:"source_evidence_count"`
}

type CorpusConcept struct {
	SchemaVersion          string                           `json:"schema_version"`
	ConceptID              string                           `json:"concept_id"`
	CorpusID               string                           `json:"corpus_id"`
	Title                  string                           `json:"title"`
	ReviewPrompt           string                           `json:"review_prompt"`
	GroupingRationale      string                           `json:"grouping_rationale"`
	CandidateMeaning       string                           `json:"candidate_meaning"`
	AcceptMeaning          string                           `json:"accept_meaning"`
	DecisionRubric         []CorpusConceptDecisionCriterion `json:"decision_rubric,omitempty"`
	ConceptKey             string                           `json:"concept_key"`
	Section                CorpusConceptSection             `json:"section"`
	CandidateKind          SemanticCandidateKind            `json:"candidate_kind"`
	WriteEligible          bool                             `json:"write_eligible"`
	ReviewStatus           ReviewStatus                     `json:"review_status"`
	ReviewWorkKind         CorpusConceptReviewWorkKind      `json:"review_work_kind"`
	AtomCount              int                              `json:"atom_count"`
	SourceCount            int                              `json:"source_count"`
	EvidenceReferenceCount int                              `json:"evidence_reference_count"`
	SourceKindCoverage     map[string]int                   `json:"source_kind_coverage"`
	ReasonCodes            []string                         `json:"reason_codes,omitempty"`
	EvidenceRefs           []SourceMeaningPacketEvidenceRef `json:"evidence_refs"`
	SourceEvidence         []CorpusConceptSourceEvidence    `json:"source_evidence"`
	RepresentativeEvidence []CorpusConceptEvidencePreview   `json:"representative_evidence"`
	AtomRefs               []SourceMeaningPacketAtomRef     `json:"atom_refs"`
}

type CorpusConceptDecisionCriterion struct {
	Choice    CorpusConceptReviewChoice `json:"choice,omitempty"`
	Label     string                    `json:"label"`
	Criterion string                    `json:"criterion"`
}

type CorpusConceptSourceEvidence struct {
	SourceID            string                         `json:"source_id"`
	SourceKind          string                         `json:"source_kind"`
	SourceRef           string                         `json:"source_ref"`
	AtomCount           int                            `json:"atom_count"`
	ReviewableAtomCount int                            `json:"reviewable_atom_count"`
	DuplicateAtomCount  int                            `json:"duplicate_atom_count,omitempty"`
	LinkOnly            bool                           `json:"link_only"`
	Contribution        string                         `json:"contribution,omitempty"`
	Flags               []string                       `json:"flags,omitempty"`
	SharedTerms         []string                       `json:"shared_terms,omitempty"`
	Evidence            []CorpusConceptEvidencePreview `json:"evidence"`
}

type CorpusConceptEvidencePreview struct {
	EvidenceRefID string `json:"evidence_ref_id"`
	AtomID        string `json:"atom_id"`
	SourceID      string `json:"source_id"`
	SourceKind    string `json:"source_kind"`
	SourceRef     string `json:"source_ref"`
	LineStart     int    `json:"line_start"`
	LineEnd       int    `json:"line_end"`
	ContentHash   string `json:"content_hash"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
	Excerpt       string `json:"excerpt"`
}

type CorpusConceptReviewChoice string

const (
	CorpusConceptReviewAccept             CorpusConceptReviewChoice = "accept"
	CorpusConceptReviewRejectNoisy        CorpusConceptReviewChoice = "reject_noisy"
	CorpusConceptReviewSplitNeeded        CorpusConceptReviewChoice = "split_needed"
	CorpusConceptReviewMergeDuplicate     CorpusConceptReviewChoice = "merge_duplicate"
	CorpusConceptReviewRenameNeeded       CorpusConceptReviewChoice = "rename_needed"
	CorpusConceptReviewNeedsSourceContext CorpusConceptReviewChoice = "needs_source_context"
)

type CorpusConceptReviewWorkKind string

const (
	CorpusConceptReviewWorkConceptReview     CorpusConceptReviewWorkKind = "concept_review"
	CorpusConceptReviewWorkCleanupTriage     CorpusConceptReviewWorkKind = "cleanup_triage"
	CorpusConceptReviewWorkEnrichmentBacklog CorpusConceptReviewWorkKind = "enrichment_backlog"
	CorpusConceptReviewWorkBlockedDiagnostic CorpusConceptReviewWorkKind = "blocked_diagnostic"
)

type CorpusConceptReviewRecords struct {
	SchemaVersion             string                                                          `json:"schema_version"`
	CorpusID                  string                                                          `json:"corpus_id"`
	ReviewContractFingerprint string                                                          `json:"review_contract_fingerprint"`
	ReviewWorkKindProgress    map[CorpusConceptReviewWorkKind]CorpusConceptReviewWorkProgress `json:"review_work_kind_progress,omitempty"`
	Records                   []CorpusConceptReviewRecord                                     `json:"records"`
}

type CorpusConceptReviewRecord struct {
	SchemaVersion  string                      `json:"schema_version"`
	CorpusID       string                      `json:"corpus_id"`
	ConceptID      string                      `json:"concept_id"`
	ConceptTitle   string                      `json:"concept_title"`
	ReviewWorkKind CorpusConceptReviewWorkKind `json:"review_work_kind"`
	Choice         CorpusConceptReviewChoice   `json:"choice"`
	Note           string                      `json:"note,omitempty"`
	ReviewerID     string                      `json:"reviewer_id,omitempty"`
	RecordedAt     string                      `json:"recorded_at"`
}

type CorpusConceptReviewRecordInput struct {
	ConceptID      string
	ReviewWorkKind CorpusConceptReviewWorkKind
	Choice         CorpusConceptReviewChoice
	Note           string
	ReviewerID     string
	RecordedAt     time.Time
}

type CorpusConceptReviewProgress struct {
	TotalConceptCount     int                                                             `json:"total_concept_count"`
	ReviewedConceptCount  int                                                             `json:"reviewed_concept_count"`
	RemainingConceptCount int                                                             `json:"remaining_concept_count"`
	ChoiceCounts          map[CorpusConceptReviewChoice]int                               `json:"choice_counts"`
	WorkKindCounts        map[CorpusConceptReviewWorkKind]CorpusConceptReviewWorkProgress `json:"work_kind_counts"`
}

type CorpusConceptReviewWorkProgress struct {
	TotalCount     int                               `json:"total_count"`
	ReviewedCount  int                               `json:"reviewed_count"`
	RemainingCount int                               `json:"remaining_count"`
	ChoiceCounts   map[CorpusConceptReviewChoice]int `json:"choice_counts"`
}

type corpusConceptBuild struct {
	Summary CorpusConceptSummary
	Index   CorpusConceptIndex
}

func BuildCorpusConceptIndex(inputPath, outDir string) (CorpusConceptSummary, CorpusConceptIndex, error) {
	if strings.TrimSpace(outDir) == "" {
		return CorpusConceptSummary{}, CorpusConceptIndex{}, fmt.Errorf("missing required --out")
	}
	root, pressureSummary, err := readSourceMeaningPressureSummary(inputPath)
	if err != nil {
		return CorpusConceptSummary{}, CorpusConceptIndex{}, err
	}
	graphSummary, atomsBySource, relationsBySource, err := readSourceMeaningGraph(root, pressureSummary.GraphSummaryPath)
	if err != nil {
		return CorpusConceptSummary{}, CorpusConceptIndex{}, err
	}
	build := buildCorpusConceptIndex(pressureSummary, graphSummary, flattenSourceMeaningAtoms(atomsBySource), flattenSourceMeaningRelations(relationsBySource), DefaultCorpusConceptsMax)
	if err := WriteCorpusConceptIndex(outDir, build.Summary, build.Index); err != nil {
		return CorpusConceptSummary{}, CorpusConceptIndex{}, err
	}
	return build.Summary, build.Index, nil
}

func ReadCorpusConceptSummary(inputPath string) (CorpusConceptSummary, error) {
	root, err := corpusConceptRoot(inputPath)
	if err != nil {
		return CorpusConceptSummary{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "concept-summary.json"))
	if err != nil {
		return CorpusConceptSummary{}, fmt.Errorf("read corpus concept summary: %w", err)
	}
	var summary CorpusConceptSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return CorpusConceptSummary{}, fmt.Errorf("decode corpus concept summary: %w", err)
	}
	if summary.SchemaVersion != CorpusConceptsSchemaVersion {
		return CorpusConceptSummary{}, fmt.Errorf("unsupported corpus concept summary schema version: %s", summary.SchemaVersion)
	}
	return summary, nil
}

func ReadCorpusConceptIndex(inputPath string) (CorpusConceptIndex, error) {
	root, err := corpusConceptRoot(inputPath)
	if err != nil {
		return CorpusConceptIndex{}, err
	}
	data, err := os.ReadFile(filepath.Join(root, "concept-index.json"))
	if err != nil {
		return CorpusConceptIndex{}, fmt.Errorf("read corpus concept index: %w", err)
	}
	var index CorpusConceptIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return CorpusConceptIndex{}, fmt.Errorf("decode corpus concept index: %w", err)
	}
	if index.SchemaVersion != CorpusConceptsSchemaVersion {
		return CorpusConceptIndex{}, fmt.Errorf("unsupported corpus concept index schema version: %s", index.SchemaVersion)
	}
	return index, nil
}

func ReadCorpusConceptReviewRecords(inputPath string) (CorpusConceptReviewRecords, error) {
	root, err := corpusConceptRoot(inputPath)
	if err != nil {
		return CorpusConceptReviewRecords{}, err
	}
	index, err := ReadCorpusConceptIndex(root)
	if err != nil {
		return CorpusConceptReviewRecords{}, err
	}
	path := filepath.Join(root, "review-records.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return CorpusConceptReviewRecords{
			SchemaVersion:             CorpusConceptReviewRecordsSchemaVersion,
			CorpusID:                  index.CorpusID,
			ReviewContractFingerprint: CorpusConceptReviewContractFingerprint(index),
			Records:                   []CorpusConceptReviewRecord{},
		}, nil
	}
	if err != nil {
		return CorpusConceptReviewRecords{}, fmt.Errorf("read corpus concept review records: %w", err)
	}
	var records CorpusConceptReviewRecords
	if err := json.Unmarshal(data, &records); err != nil {
		return CorpusConceptReviewRecords{}, fmt.Errorf("decode corpus concept review records: %w", err)
	}
	if records.SchemaVersion != CorpusConceptReviewRecordsSchemaVersion {
		return CorpusConceptReviewRecords{}, fmt.Errorf("unsupported corpus concept review records schema version: %s", records.SchemaVersion)
	}
	if records.CorpusID != index.CorpusID {
		return CorpusConceptReviewRecords{}, fmt.Errorf("corpus concept review records corpus mismatch")
	}
	expectedFingerprint := CorpusConceptReviewContractFingerprint(index)
	if records.ReviewContractFingerprint != expectedFingerprint {
		return CorpusConceptReviewRecords{}, fmt.Errorf("corpus concept review records contract fingerprint mismatch")
	}
	conceptsByID := map[string]CorpusConcept{}
	for _, concept := range index.Concepts {
		conceptsByID[concept.ConceptID] = concept
	}
	for _, record := range records.Records {
		if err := ValidateCorpusConceptReviewRecord(record); err != nil {
			return CorpusConceptReviewRecords{}, err
		}
		concept, ok := conceptsByID[record.ConceptID]
		if !ok {
			return CorpusConceptReviewRecords{}, fmt.Errorf("unknown corpus concept review record concept: %s", record.ConceptID)
		}
		if normalizeCorpusConceptReviewWorkKind(record.ReviewWorkKind) != normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind) {
			return CorpusConceptReviewRecords{}, fmt.Errorf("corpus concept review record work kind mismatch for %s", record.ConceptID)
		}
		if !allowedCorpusConceptReviewChoice(concept.ReviewWorkKind, record.Choice) {
			return CorpusConceptReviewRecords{}, fmt.Errorf("unsupported %s corpus concept review choice: %s", concept.ReviewWorkKind, record.Choice)
		}
	}
	expectedProgress := BuildCorpusConceptReviewProgress(index, records).WorkKindCounts
	if !corpusConceptReviewWorkProgressEqual(records.ReviewWorkKindProgress, expectedProgress) {
		return CorpusConceptReviewRecords{}, fmt.Errorf("corpus concept review records progress mismatch")
	}
	return records, nil
}

func RecordCorpusConceptReview(inputPath string, input CorpusConceptReviewRecordInput) (CorpusConceptReviewRecords, error) {
	root, err := corpusConceptRoot(inputPath)
	if err != nil {
		return CorpusConceptReviewRecords{}, err
	}
	if !validCorpusConceptReviewChoice(input.Choice) {
		return CorpusConceptReviewRecords{}, fmt.Errorf("unsupported corpus concept review choice: %s", input.Choice)
	}
	var updated CorpusConceptReviewRecords
	err = withSemanticJudgmentBundleLock(root, func() error {
		index, err := ReadCorpusConceptIndex(root)
		if err != nil {
			return err
		}
		conceptsByID := map[string]CorpusConcept{}
		for _, concept := range index.Concepts {
			conceptsByID[concept.ConceptID] = concept
		}
		concept, ok := conceptsByID[input.ConceptID]
		if !ok {
			return fmt.Errorf("unknown corpus concept: %s", input.ConceptID)
		}
		concept.ReviewWorkKind = normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind)
		inputWorkKind := input.ReviewWorkKind
		if inputWorkKind == "" {
			inputWorkKind = concept.ReviewWorkKind
		}
		if inputWorkKind != concept.ReviewWorkKind {
			return fmt.Errorf("corpus concept review work kind mismatch: got %s want %s", inputWorkKind, concept.ReviewWorkKind)
		}
		if !allowedCorpusConceptReviewChoice(concept.ReviewWorkKind, input.Choice) {
			return fmt.Errorf("unsupported %s corpus concept review choice: %s", concept.ReviewWorkKind, input.Choice)
		}
		records, err := ReadCorpusConceptReviewRecords(root)
		if err != nil {
			return err
		}
		recordedAt := input.RecordedAt
		if recordedAt.IsZero() {
			recordedAt = time.Now().UTC()
		}
		record := CorpusConceptReviewRecord{
			SchemaVersion:  CorpusConceptReviewRecordsSchemaVersion,
			CorpusID:       index.CorpusID,
			ConceptID:      concept.ConceptID,
			ConceptTitle:   concept.Title,
			ReviewWorkKind: concept.ReviewWorkKind,
			Choice:         input.Choice,
			Note:           strings.TrimSpace(input.Note),
			ReviewerID:     strings.TrimSpace(input.ReviewerID),
			RecordedAt:     recordedAt.UTC().Format(time.RFC3339),
		}
		if err := ValidateCorpusConceptReviewRecord(record); err != nil {
			return err
		}
		replaced := false
		for i := range records.Records {
			if records.Records[i].ConceptID == record.ConceptID {
				records.Records[i] = record
				replaced = true
				break
			}
		}
		if !replaced {
			records.Records = append(records.Records, record)
		}
		sort.Slice(records.Records, func(i, j int) bool { return records.Records[i].ConceptID < records.Records[j].ConceptID })
		records.ReviewContractFingerprint = CorpusConceptReviewContractFingerprint(index)
		records.ReviewWorkKindProgress = BuildCorpusConceptReviewProgress(index, records).WorkKindCounts
		updated = records
		if err := writeJSON(root, "review-records.json", updated); err != nil {
			return ArtifactWriteError{Err: err}
		}
		return nil
	})
	if err != nil {
		return CorpusConceptReviewRecords{}, err
	}
	return updated, nil
}

func BuildCorpusConceptReviewProgress(index CorpusConceptIndex, records CorpusConceptReviewRecords) CorpusConceptReviewProgress {
	conceptsByID := map[string]CorpusConcept{}
	for _, concept := range index.Concepts {
		concept.ReviewWorkKind = normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind)
		conceptsByID[concept.ConceptID] = concept
	}
	progress := CorpusConceptReviewProgress{
		ChoiceCounts:   map[CorpusConceptReviewChoice]int{},
		WorkKindCounts: initialCorpusConceptReviewWorkProgress(),
	}
	for _, concept := range index.Concepts {
		workKind := normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind)
		bucket := progress.WorkKindCounts[workKind]
		bucket.TotalCount++
		progress.WorkKindCounts[workKind] = bucket
		if workKind == CorpusConceptReviewWorkConceptReview {
			progress.TotalConceptCount++
		}
	}
	seen := map[string]bool{}
	for _, record := range records.Records {
		concept, ok := conceptsByID[record.ConceptID]
		if !ok || seen[record.ConceptID] {
			continue
		}
		workKind := normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind)
		if normalizeCorpusConceptReviewWorkKind(record.ReviewWorkKind) != workKind || !allowedCorpusConceptReviewChoice(workKind, record.Choice) {
			continue
		}
		seen[record.ConceptID] = true
		bucket := progress.WorkKindCounts[workKind]
		bucket.ReviewedCount++
		bucket.ChoiceCounts[record.Choice]++
		progress.WorkKindCounts[workKind] = bucket
		if workKind == CorpusConceptReviewWorkConceptReview {
			progress.ReviewedConceptCount++
			progress.ChoiceCounts[record.Choice]++
		}
	}
	progress.RemainingConceptCount = progress.TotalConceptCount - progress.ReviewedConceptCount
	if progress.RemainingConceptCount < 0 {
		progress.RemainingConceptCount = 0
	}
	for kind, bucket := range progress.WorkKindCounts {
		bucket.RemainingCount = bucket.TotalCount - bucket.ReviewedCount
		if bucket.RemainingCount < 0 {
			bucket.RemainingCount = 0
		}
		progress.WorkKindCounts[kind] = bucket
	}
	return progress
}

func ValidateCorpusConceptReviewRecord(record CorpusConceptReviewRecord) error {
	if record.SchemaVersion != CorpusConceptReviewRecordsSchemaVersion {
		return fmt.Errorf("unsupported corpus concept review record schema version: %s", record.SchemaVersion)
	}
	if strings.TrimSpace(record.CorpusID) == "" {
		return fmt.Errorf("missing corpus concept review record corpus id")
	}
	if strings.TrimSpace(record.ConceptID) == "" || sanitizeID(record.ConceptID) != record.ConceptID {
		return fmt.Errorf("unsafe corpus concept review record concept id: %s", record.ConceptID)
	}
	if record.ReviewWorkKind == "" || !validCorpusConceptReviewWorkKind(record.ReviewWorkKind) {
		return fmt.Errorf("unsupported corpus concept review work kind: %s", record.ReviewWorkKind)
	}
	if !validCorpusConceptReviewChoice(record.Choice) {
		return fmt.Errorf("unsupported corpus concept review choice: %s", record.Choice)
	}
	if strings.TrimSpace(record.RecordedAt) == "" {
		return fmt.Errorf("missing corpus concept review record recorded_at")
	}
	body := strings.Join([]string{record.ConceptTitle, record.Note, record.ReviewerID}, "\n")
	if containsUnsafeMarker(body) || containsGovernanceID(body) {
		return fmt.Errorf("corpus concept review record contains private marker")
	}
	return nil
}

func initialCorpusConceptReviewWorkProgress() map[CorpusConceptReviewWorkKind]CorpusConceptReviewWorkProgress {
	out := map[CorpusConceptReviewWorkKind]CorpusConceptReviewWorkProgress{}
	for _, kind := range []CorpusConceptReviewWorkKind{
		CorpusConceptReviewWorkConceptReview,
		CorpusConceptReviewWorkCleanupTriage,
		CorpusConceptReviewWorkEnrichmentBacklog,
		CorpusConceptReviewWorkBlockedDiagnostic,
	} {
		out[kind] = CorpusConceptReviewWorkProgress{ChoiceCounts: map[CorpusConceptReviewChoice]int{}}
	}
	return out
}

func corpusConceptReviewWorkProgressEqual(left, right map[CorpusConceptReviewWorkKind]CorpusConceptReviewWorkProgress) bool {
	for _, kind := range []CorpusConceptReviewWorkKind{
		CorpusConceptReviewWorkConceptReview,
		CorpusConceptReviewWorkCleanupTriage,
		CorpusConceptReviewWorkEnrichmentBacklog,
		CorpusConceptReviewWorkBlockedDiagnostic,
	} {
		leftBucket, leftOK := left[kind]
		rightBucket, rightOK := right[kind]
		if !leftOK || !rightOK || leftBucket.TotalCount != rightBucket.TotalCount || leftBucket.ReviewedCount != rightBucket.ReviewedCount || leftBucket.RemainingCount != rightBucket.RemainingCount {
			return false
		}
		for _, choice := range []CorpusConceptReviewChoice{
			CorpusConceptReviewAccept,
			CorpusConceptReviewRejectNoisy,
			CorpusConceptReviewSplitNeeded,
			CorpusConceptReviewMergeDuplicate,
			CorpusConceptReviewRenameNeeded,
			CorpusConceptReviewNeedsSourceContext,
		} {
			if leftBucket.ChoiceCounts[choice] != rightBucket.ChoiceCounts[choice] {
				return false
			}
		}
	}
	return len(left) == len(right)
}

func normalizeCorpusConceptReviewWorkKind(kind CorpusConceptReviewWorkKind) CorpusConceptReviewWorkKind {
	if kind == "" {
		return CorpusConceptReviewWorkConceptReview
	}
	return kind
}

func validCorpusConceptReviewWorkKind(kind CorpusConceptReviewWorkKind) bool {
	switch normalizeCorpusConceptReviewWorkKind(kind) {
	case CorpusConceptReviewWorkConceptReview, CorpusConceptReviewWorkCleanupTriage, CorpusConceptReviewWorkEnrichmentBacklog, CorpusConceptReviewWorkBlockedDiagnostic:
		return true
	default:
		return false
	}
}

func allowedCorpusConceptReviewChoice(kind CorpusConceptReviewWorkKind, choice CorpusConceptReviewChoice) bool {
	if !validCorpusConceptReviewChoice(choice) {
		return false
	}
	switch normalizeCorpusConceptReviewWorkKind(kind) {
	case CorpusConceptReviewWorkConceptReview:
		return true
	case CorpusConceptReviewWorkCleanupTriage:
		switch choice {
		case CorpusConceptReviewRejectNoisy, CorpusConceptReviewSplitNeeded, CorpusConceptReviewMergeDuplicate, CorpusConceptReviewRenameNeeded:
			return true
		}
	case CorpusConceptReviewWorkEnrichmentBacklog:
		switch choice {
		case CorpusConceptReviewNeedsSourceContext, CorpusConceptReviewRejectNoisy:
			return true
		}
	case CorpusConceptReviewWorkBlockedDiagnostic:
		switch choice {
		case CorpusConceptReviewRejectNoisy, CorpusConceptReviewSplitNeeded, CorpusConceptReviewNeedsSourceContext:
			return true
		}
	}
	return false
}

func validCorpusConceptReviewChoice(choice CorpusConceptReviewChoice) bool {
	switch choice {
	case CorpusConceptReviewAccept, CorpusConceptReviewRejectNoisy, CorpusConceptReviewSplitNeeded, CorpusConceptReviewMergeDuplicate, CorpusConceptReviewRenameNeeded, CorpusConceptReviewNeedsSourceContext:
		return true
	default:
		return false
	}
}

func corpusConceptRoot(inputPath string) (string, error) {
	return resolveNamedArtifactRoot(inputPath, CorpusConceptsDirName)
}

func buildCorpusConceptIndex(pressure CorpusPressureSummary, graph CorpusGraphSummary, atoms []CorpusGraphAtom, relations []CorpusGraphRelation, maxConcepts int) corpusConceptBuild {
	if maxConcepts <= 0 {
		maxConcepts = DefaultCorpusConceptsMax
	}
	generated := buildCorpusConcepts(pressure.CorpusID, atoms, relations)
	concepts := append([]CorpusConcept{}, generated...)
	omittedConceptCount := 0
	omittedAtomCount := 0
	if len(concepts) > maxConcepts {
		omittedConceptCount = len(concepts) - maxConcepts
		for _, concept := range concepts[maxConcepts:] {
			omittedAtomCount += concept.AtomCount
		}
		concepts = append([]CorpusConcept{}, concepts[:maxConcepts]...)
	}
	index := CorpusConceptIndex{
		SchemaVersion: CorpusConceptsSchemaVersion,
		CorpusID:      pressure.CorpusID,
		Concepts:      concepts,
	}
	summary := buildCorpusConceptSummary(pressure, graph, concepts, len(generated), maxConcepts, omittedConceptCount, omittedAtomCount)
	return corpusConceptBuild{Summary: summary, Index: index}
}

func buildCorpusConcepts(corpusID string, atoms []CorpusGraphAtom, relations []CorpusGraphRelation) []CorpusConcept {
	assigned := map[string]bool{}
	concepts := buildCorpusRelationConcepts(corpusID, atoms, relations, assigned)
	termBuckets := corpusConceptTermBuckets(atoms)
	for _, bucket := range termBuckets {
		selected := []CorpusGraphAtom{}
		for _, atom := range bucket.Atoms {
			if assigned[atom.AtomID] {
				continue
			}
			selected = append(selected, atom)
			if len(selected) >= corpusConceptMaxAtoms {
				break
			}
		}
		if len(selected) < 2 {
			continue
		}
		for _, atom := range selected {
			assigned[atom.AtomID] = true
		}
		concepts = append(concepts, buildCorpusConcept(corpusID, bucket.Key, selected))
	}
	localBuckets := map[string][]CorpusGraphAtom{}
	for _, atom := range atoms {
		if assigned[atom.AtomID] {
			continue
		}
		key := strings.Join([]string{"local", string(atom.CandidateKind), sourceKindForConcept(atom)}, "\x00")
		localBuckets[key] = append(localBuckets[key], atom)
	}
	keys := make([]string, 0, len(localBuckets))
	for key := range localBuckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		bucket := localBuckets[key]
		sortCorpusConceptAtoms(bucket)
		for start := 0; start < len(bucket); start += corpusConceptMaxAtoms {
			end := start + corpusConceptMaxAtoms
			if end > len(bucket) {
				end = len(bucket)
			}
			concepts = append(concepts, buildCorpusConcept(corpusID, key, bucket[start:end]))
		}
	}
	sort.Slice(concepts, func(i, j int) bool {
		return corpusConceptSortKey(concepts[i]) < corpusConceptSortKey(concepts[j])
	})
	return concepts
}

func buildCorpusRelationConcepts(corpusID string, atoms []CorpusGraphAtom, relations []CorpusGraphRelation, assigned map[string]bool) []CorpusConcept {
	atomsByID := map[string]CorpusGraphAtom{}
	for _, atom := range atoms {
		atomsByID[atom.AtomID] = atom
	}
	adjacency := map[string]map[string]bool{}
	for _, relation := range relations {
		if relation.ReviewStatus == ReviewStatusBlocked {
			continue
		}
		if relation.RelationType != CorpusRelationSameTopicAs && relation.RelationType != CorpusRelationPossibleDuplicate {
			continue
		}
		from, fromOK := atomsByID[relation.FromAtomID]
		to, toOK := atomsByID[relation.ToAtomID]
		if !fromOK || !toOK || sourceKindForConcept(from) == sourceKindForConcept(to) {
			continue
		}
		if adjacency[from.AtomID] == nil {
			adjacency[from.AtomID] = map[string]bool{}
		}
		if adjacency[to.AtomID] == nil {
			adjacency[to.AtomID] = map[string]bool{}
		}
		adjacency[from.AtomID][to.AtomID] = true
		adjacency[to.AtomID][from.AtomID] = true
	}
	visited := map[string]bool{}
	components := [][]CorpusGraphAtom{}
	for atomID := range adjacency {
		if visited[atomID] {
			continue
		}
		stack := []string{atomID}
		visited[atomID] = true
		component := []CorpusGraphAtom{}
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			component = append(component, atomsByID[current])
			for next := range adjacency[current] {
				if visited[next] {
					continue
				}
				visited[next] = true
				stack = append(stack, next)
			}
		}
		if corpusConceptSourceKindCount(component) > 1 {
			components = append(components, component)
		}
	}
	sort.Slice(components, func(i, j int) bool {
		if len(components[i]) != len(components[j]) {
			return len(components[i]) > len(components[j])
		}
		sortCorpusConceptAtoms(components[i])
		sortCorpusConceptAtoms(components[j])
		return components[i][0].AtomID < components[j][0].AtomID
	})
	concepts := []CorpusConcept{}
	for componentIndex, component := range components {
		for chunkIndex, chunk := range corpusConceptMixedChunks(component, corpusConceptMaxAtoms) {
			if len(chunk) < 2 || corpusConceptSourceKindCount(chunk) < 2 {
				continue
			}
			for _, atom := range chunk {
				assigned[atom.AtomID] = true
			}
			key := fmt.Sprintf("relation\x00cross_source\x00%04d\x00%04d", componentIndex+1, chunkIndex+1)
			concepts = append(concepts, buildCorpusConcept(corpusID, key, chunk))
		}
	}
	return concepts
}

func corpusConceptMixedChunks(atoms []CorpusGraphAtom, chunkSize int) [][]CorpusGraphAtom {
	byKind := map[string][]CorpusGraphAtom{}
	kinds := []string{}
	for _, atom := range atoms {
		kind := sourceKindForConcept(atom)
		if _, ok := byKind[kind]; !ok {
			kinds = append(kinds, kind)
		}
		byKind[kind] = append(byKind[kind], atom)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		sortCorpusConceptAtoms(byKind[kind])
	}
	ordered := []CorpusGraphAtom{}
	for {
		added := false
		for _, kind := range kinds {
			if len(byKind[kind]) == 0 {
				continue
			}
			ordered = append(ordered, byKind[kind][0])
			byKind[kind] = byKind[kind][1:]
			added = true
		}
		if !added {
			break
		}
	}
	chunks := [][]CorpusGraphAtom{}
	for start := 0; start < len(ordered); start += chunkSize {
		end := start + chunkSize
		if end > len(ordered) {
			end = len(ordered)
		}
		chunks = append(chunks, ordered[start:end])
	}
	return chunks
}

func corpusConceptSourceKindCount(atoms []CorpusGraphAtom) int {
	kinds := map[string]bool{}
	for _, atom := range atoms {
		kinds[sourceKindForConcept(atom)] = true
	}
	return len(kinds)
}

type corpusConceptTermBucket struct {
	Key         string
	Atoms       []CorpusGraphAtom
	SourceCount int
	KindCount   int
}

func corpusConceptTermBuckets(atoms []CorpusGraphAtom) []corpusConceptTermBucket {
	buckets := map[string][]CorpusGraphAtom{}
	for _, atom := range atoms {
		for _, term := range corpusConceptTerms(atom) {
			key := strings.Join([]string{"term", string(atom.CandidateKind), term}, "\x00")
			buckets[key] = append(buckets[key], atom)
		}
	}
	out := []corpusConceptTermBucket{}
	for key, bucketAtoms := range buckets {
		if len(bucketAtoms) < 2 {
			continue
		}
		sortCorpusConceptAtoms(bucketAtoms)
		sourceSet := map[string]bool{}
		kindSet := map[string]bool{}
		deduped := []CorpusGraphAtom{}
		seenAtoms := map[string]bool{}
		for _, atom := range bucketAtoms {
			if seenAtoms[atom.AtomID] {
				continue
			}
			seenAtoms[atom.AtomID] = true
			sourceSet[atom.SourceID] = true
			kindSet[sourceKindForConcept(atom)] = true
			deduped = append(deduped, atom)
		}
		if len(deduped) < 2 {
			continue
		}
		out = append(out, corpusConceptTermBucket{Key: key, Atoms: deduped, SourceCount: len(sourceSet), KindCount: len(kindSet)})
	}
	sort.Slice(out, func(i, j int) bool {
		left := fmt.Sprintf("%03d:%03d:%03d:%s", out[i].KindCount, out[i].SourceCount, len(out[i].Atoms), out[i].Key)
		right := fmt.Sprintf("%03d:%03d:%03d:%s", out[j].KindCount, out[j].SourceCount, len(out[j].Atoms), out[j].Key)
		return left > right
	})
	return out
}

func buildCorpusConcept(corpusID, key string, atoms []CorpusGraphAtom) CorpusConcept {
	sortCorpusConceptAtoms(atoms)
	kind := atoms[0].CandidateKind
	concept := CorpusConcept{
		SchemaVersion:      CorpusConceptsSchemaVersion,
		CorpusID:           corpusID,
		ConceptKey:         key,
		CandidateKind:      kind,
		WriteEligible:      false,
		ReviewStatus:       ReviewStatusReady,
		SourceKindCoverage: map[string]int{},
	}
	sourceIDs := map[string]bool{}
	sourceKindSourceIDs := map[string]map[string]bool{}
	for _, atom := range atoms {
		sourceIDs[atom.SourceID] = true
		sourceKind := sourceKindForConcept(atom)
		if sourceKindSourceIDs[sourceKind] == nil {
			sourceKindSourceIDs[sourceKind] = map[string]bool{}
		}
		sourceKindSourceIDs[sourceKind][atom.SourceID] = true
		concept.AtomRefs = append(concept.AtomRefs, sourceMeaningAtomRef(atom))
		evidence := sourceMeaningEvidenceRef(atom)
		if evidence.ContentHash == "" || evidence.LineStart <= 0 || evidence.LineEnd <= 0 {
			concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "missing_evidence_reference")
		} else {
			concept.EvidenceRefs = append(concept.EvidenceRefs, evidence)
		}
		if atom.ReviewStatus == ReviewStatusBlocked {
			concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "blocked_atom")
		}
		if atom.ReviewStatus == ReviewStatusNeedsReview {
			concept.ReviewStatus = ReviewStatusNeedsReview
		}
		for _, blocker := range atom.Blockers {
			concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, blocker.Code)
		}
	}
	concept.AtomCount = len(concept.AtomRefs)
	concept.SourceCount = len(sourceIDs)
	concept.EvidenceReferenceCount = len(concept.EvidenceRefs)
	for sourceKind, sourceKindIDs := range sourceKindSourceIDs {
		concept.SourceKindCoverage[sourceKind] = len(sourceKindIDs)
	}
	if concept.SourceCount < 2 {
		concept.Section = CorpusConceptSectionLocal
		concept.ReviewStatus = ReviewStatusNeedsReview
		concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "single_source_concept")
	} else if len(concept.SourceKindCoverage) < 2 {
		concept.Section = CorpusConceptSectionNeedsReview
		concept.ReviewStatus = ReviewStatusNeedsReview
		concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "single_source_kind_concept")
	} else {
		concept.Section = CorpusConceptSectionCrossSource
	}
	if strings.HasPrefix(key, "relation\x00cross_source\x00") {
		concept.ReviewStatus = ReviewStatusNeedsReview
		concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "relation_neighborhood_requires_review")
	}
	concept.SourceEvidence = corpusConceptSourceEvidence(atoms)
	applyCorpusConceptSourceQualityGates(&concept, key)
	if len(concept.ReasonCodes) > 0 {
		if containsCorpusConceptString(concept.ReasonCodes, "blocked_atom") || containsCorpusConceptString(concept.ReasonCodes, "missing_evidence_reference") {
			concept.Section = CorpusConceptSectionBlocked
			concept.ReviewStatus = ReviewStatusBlocked
		}
	}
	concept.ReviewWorkKind = corpusConceptReviewWorkKind(concept)
	concept.Title = corpusConceptTitle(key, atoms)
	concept.GroupingRationale = corpusConceptGroupingRationale(key, concept)
	concept.CandidateMeaning = corpusConceptCandidateMeaning(concept)
	concept.AcceptMeaning = corpusConceptAcceptMeaning(concept)
	concept.DecisionRubric = corpusConceptDecisionRubric(concept)
	concept.ReviewPrompt = corpusConceptReviewPrompt(concept)
	concept.RepresentativeEvidence = corpusConceptRepresentativeEvidence(atoms)
	concept.ConceptID = corpusConceptID(corpusID, key, atoms)
	sort.Slice(concept.EvidenceRefs, func(i, j int) bool {
		return concept.EvidenceRefs[i].EvidenceRefID < concept.EvidenceRefs[j].EvidenceRefID
	})
	return concept
}

func buildCorpusConceptSummary(pressure CorpusPressureSummary, graph CorpusGraphSummary, concepts []CorpusConcept, generatedConceptCount, maxConcepts, omittedConceptCount, omittedAtomCount int) CorpusConceptSummary {
	summary := CorpusConceptSummary{
		SchemaVersion:             CorpusConceptsSchemaVersion,
		CorpusID:                  pressure.CorpusID,
		SourceCount:               pressure.SourceCount,
		ProcessedSourceCount:      pressure.ProcessedSourceCount,
		AtomCount:                 graph.AtomCount,
		RelationCount:             graph.RelationCount,
		ConceptCount:              len(concepts),
		GeneratedConceptCount:     generatedConceptCount,
		MaxConceptCount:           maxConcepts,
		OmittedConceptCount:       omittedConceptCount,
		OmittedAtomCount:          omittedAtomCount,
		ScaleStatus:               "scale_complete",
		NonGeneralizableRuntime:   true,
		Comparable:                true,
		Guardrails:                pressure.Guardrails,
		CorpusFingerprint:         pressure.CorpusFingerprint,
		CommandConfigFingerprint:  pressure.CommandConfigFingerprint,
		PressureReplayFingerprint: pressure.ReplayFingerprint,
		GraphReplayFingerprint:    graph.ReplayFingerprint,
		ConceptIndexPath:          filepath.ToSlash(filepath.Join(CorpusConceptsDirName, "concept-index.json")),
		ReviewPacketPath:          filepath.ToSlash(filepath.Join(CorpusConceptsDirName, "review-packet.md")),
		SourceKindCoverage:        map[string]int{},
		SectionCounts:             map[CorpusConceptSection]int{},
		CandidateKindCounts:       map[SemanticCandidateKind]int{},
		ReviewWorkKindCounts:      map[CorpusConceptReviewWorkKind]int{},
	}
	if pressure.ScaleStatus == "scale_partial" {
		summary.ScaleStatus = "scale_partial"
		for _, reason := range pressure.ScaleReasonCodes {
			summary.ScaleReasonCodes = appendUniqueString(summary.ScaleReasonCodes, reason)
		}
	}
	if graph.ScaleStatus == "scale_partial" {
		summary.ScaleStatus = "scale_partial"
		for _, reason := range graph.ScaleReasonCodes {
			summary.ScaleReasonCodes = appendUniqueString(summary.ScaleReasonCodes, reason)
		}
	}
	if omittedConceptCount > 0 {
		summary.ScaleStatus = "scale_partial"
		summary.ScaleReasonCodes = appendUniqueString(summary.ScaleReasonCodes, "scale_concept_limit")
	}
	coveredAtoms := 0
	crossSourceAtoms := 0
	for _, concept := range concepts {
		coveredAtoms += concept.AtomCount
		summary.EvidenceReferenceCount += concept.EvidenceReferenceCount
		summary.SectionCounts[concept.Section]++
		summary.CandidateKindCounts[concept.CandidateKind] += concept.AtomCount
		workKind := normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind)
		summary.ReviewWorkKindCounts[workKind]++
		switch workKind {
		case CorpusConceptReviewWorkConceptReview:
			summary.ConceptReviewCount++
			summary.ConceptReviewBurdenCount++
		case CorpusConceptReviewWorkCleanupTriage:
			summary.CleanupTriageCount++
		case CorpusConceptReviewWorkEnrichmentBacklog:
			summary.EnrichmentBacklogCount++
		case CorpusConceptReviewWorkBlockedDiagnostic:
			summary.BlockedDiagnosticCount++
		}
		for kind, count := range concept.SourceKindCoverage {
			summary.SourceKindCoverage[kind] += count
		}
		if len(concept.SourceKindCoverage) > 1 {
			summary.CrossSourceKindPairCount++
			summary.CrossSourceEvidenceReferenceCount += concept.EvidenceReferenceCount
		}
		switch concept.Section {
		case CorpusConceptSectionCrossSource:
			summary.CrossSourceConceptCount++
			crossSourceAtoms += concept.AtomCount
		case CorpusConceptSectionLocal:
			summary.LocalConceptCount++
		case CorpusConceptSectionNeedsReview:
			summary.NeedsReviewConceptCount++
		case CorpusConceptSectionBlocked:
			summary.BlockedConceptCount++
		}
		summary.Concepts = append(summary.Concepts, CorpusConceptListItem{
			ConceptID:              concept.ConceptID,
			Title:                  concept.Title,
			ReviewPrompt:           concept.ReviewPrompt,
			GroupingRationale:      concept.GroupingRationale,
			CandidateMeaning:       concept.CandidateMeaning,
			AcceptMeaning:          concept.AcceptMeaning,
			DecisionRubric:         append([]CorpusConceptDecisionCriterion{}, concept.DecisionRubric...),
			SourceContributions:    corpusConceptSourceContributionList(concept.SourceEvidence),
			ConceptKey:             concept.ConceptKey,
			Section:                concept.Section,
			CandidateKind:          concept.CandidateKind,
			AtomCount:              concept.AtomCount,
			SourceCount:            concept.SourceCount,
			EvidenceReferenceCount: concept.EvidenceReferenceCount,
			SourceKindCoverage:     cloneStringIntMap(concept.SourceKindCoverage),
			ReviewStatus:           concept.ReviewStatus,
			ReviewWorkKind:         workKind,
			ReasonCodes:            append([]string{}, concept.ReasonCodes...),
			ConceptPath:            filepath.ToSlash(filepath.Join(CorpusConceptsDirName, CorpusConceptPath(concept.ConceptID))),
			RepresentativeEvidence: len(concept.RepresentativeEvidence),
			SourceEvidence:         len(concept.SourceEvidence),
		})
	}
	if generatedConceptCount == len(concepts) {
		summary.GeneratedConceptCount = 0
	}
	if summary.ConceptCount > 0 {
		summary.ConceptReviewBurdenRatio = float64(summary.ConceptReviewBurdenCount) / float64(summary.ConceptCount)
	}
	if graph.RelationCount > 0 {
		summary.RelationReviewCompressionRatio = 1 - float64(summary.ConceptCount)/float64(graph.RelationCount)
	}
	if graph.AtomCount > 0 {
		summary.AtomCoverageRatio = float64(coveredAtoms) / float64(graph.AtomCount)
		summary.CrossSourceAtomRatio = float64(crossSourceAtoms) / float64(graph.AtomCount)
	}
	summary.ReplayFingerprint = corpusConceptReplayFingerprint(summary)
	return summary
}

func corpusConceptTerms(atom CorpusGraphAtom) []string {
	text := strings.Join([]string{atom.Title, atom.Summary}, " ")
	return corpusConceptTermsFromText(text)
}

func corpusConceptTermsFromText(text string) []string {
	text = removeCorpusConceptURLs(text)
	parts := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	seen := map[string]bool{}
	terms := []string{}
	for _, part := range parts {
		term := strings.TrimSpace(part)
		if !corpusConceptUsefulTerm(term) || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
		if len(terms) >= 8 {
			break
		}
	}
	return terms
}

func corpusConceptPreviewLinkOnly(preview CorpusConceptEvidencePreview) bool {
	text := strings.TrimSpace(strings.Join([]string{preview.Title, preview.Summary, preview.Excerpt}, " "))
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "http://") && !strings.Contains(lower, "https://") && !strings.Contains(lower, "www.") {
		return false
	}
	withoutURLs := removeCorpusConceptURLs(text)
	parts := strings.FieldsFunc(strings.ToLower(withoutURLs), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	useful := 0
	for _, part := range parts {
		if corpusConceptUsefulTerm(strings.TrimSpace(part)) {
			useful++
		}
	}
	return useful < 3
}

func removeCorpusConceptURLs(value string) string {
	fields := strings.Fields(value)
	for i, field := range fields {
		lower := strings.ToLower(field)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "www.") {
			fields[i] = " "
		}
	}
	return strings.Join(fields, " ")
}

func corpusConceptUsefulTerm(term string) bool {
	if len(term) < 4 || len(term) > 32 {
		return false
	}
	if corpusConceptStopWords[term] {
		return false
	}
	digits := 0
	for _, r := range term {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	if digits > len(term)/2 {
		return false
	}
	if digits > 0 && len(term) >= 8 {
		return false
	}
	return true
}

func corpusConceptWeakGroupingTerm(term string) bool {
	return corpusConceptWeakGroupingTerms[strings.ToLower(strings.TrimSpace(term))]
}

var corpusConceptStopWords = map[string]bool{
	"about": true, "after": true, "also": true, "and": true, "candidate": true, "changed": true,
	"confirmation": true, "correct": true, "excerpt": true, "from": true, "gmail": true, "have": true, "https": true,
	"http": true, "into": true, "linkedin": true, "locator": true, "message": true, "needs": true,
	"post": true, "posts": true, "private": true, "review": true, "reviewed": true, "runtime": true,
	"slack": true, "snippet": true, "snippe": true, "source": true, "that": true, "this": true,
	"timestamp": true, "topic": true, "updates": true, "what": true, "with": true, "your": true,
}

var corpusConceptWeakGroupingTerms = map[string]bool{
	"action": true, "asking": true, "checklist": true, "confirm": true, "create": true,
	"draft": true, "follow": true, "message": true, "needed": true, "prepare": true,
	"prepared": true, "preparing": true, "reply": true, "request": true, "send": true,
	"should": true, "task": true, "tasks": true, "update": true, "updated": true,
	"whether": true,
}

func corpusConceptTitle(key string, atoms []CorpusGraphAtom) string {
	parts := strings.Split(key, "\x00")
	if len(parts) >= 3 && parts[0] == "term" {
		return fmt.Sprintf("%s concept: %s", strings.ReplaceAll(string(atoms[0].CandidateKind), "_", " "), parts[2])
	}
	if len(parts) >= 3 && parts[0] == "local" {
		terms := corpusConceptTopTerms(atoms, 3)
		if len(terms) == 0 {
			return fmt.Sprintf("Local %s concept needing review", strings.ReplaceAll(string(atoms[0].CandidateKind), "_", " "))
		}
		return fmt.Sprintf("Local %s concept: %s", strings.ReplaceAll(string(atoms[0].CandidateKind), "_", " "), strings.Join(terms, ", "))
	}
	if len(parts) >= 2 && parts[0] == "relation" {
		terms := corpusConceptTopTerms(atoms, 3)
		if len(terms) == 0 {
			return fmt.Sprintf("Cross-source %s cluster needing review", strings.ReplaceAll(string(atoms[0].CandidateKind), "_", " "))
		}
		return fmt.Sprintf("Cross-source %s concept: %s", strings.ReplaceAll(string(atoms[0].CandidateKind), "_", " "), strings.Join(terms, ", "))
	}
	return fmt.Sprintf("%s concept", strings.ReplaceAll(string(atoms[0].CandidateKind), "_", " "))
}

func corpusConceptReviewWorkKind(concept CorpusConcept) CorpusConceptReviewWorkKind {
	if containsCorpusConceptString(concept.ReasonCodes, "blocked_atom") ||
		containsCorpusConceptString(concept.ReasonCodes, "missing_evidence_reference") ||
		containsCorpusConceptString(concept.ReasonCodes, "insufficient_readable_source_kind_support") ||
		containsCorpusConceptString(concept.ReasonCodes, "weak_cross_source_coherence") ||
		containsCorpusConceptString(concept.ReasonCodes, "readable_source_outlier") {
		return CorpusConceptReviewWorkBlockedDiagnostic
	}
	if containsCorpusConceptString(concept.ReasonCodes, "link_only_evidence_requires_enrichment") || containsCorpusConceptString(concept.ReasonCodes, "no_readable_source_evidence") || containsCorpusConceptString(concept.ReasonCodes, "insufficient_reviewable_source_support") {
		return CorpusConceptReviewWorkEnrichmentBacklog
	}
	if concept.Section == CorpusConceptSectionLocal ||
		containsCorpusConceptString(concept.ReasonCodes, "single_source_concept") ||
		containsCorpusConceptString(concept.ReasonCodes, "single_source_kind_concept") ||
		containsCorpusConceptString(concept.ReasonCodes, "generic_term_bucket_requires_cleanup") {
		return CorpusConceptReviewWorkCleanupTriage
	}
	if concept.ReviewStatus == ReviewStatusBlocked || concept.Section == CorpusConceptSectionBlocked {
		return CorpusConceptReviewWorkBlockedDiagnostic
	}
	return CorpusConceptReviewWorkConceptReview
}

func corpusConceptCandidateMeaning(concept CorpusConcept) string {
	kind := strings.ReplaceAll(string(concept.CandidateKind), "_", " ")
	terms := corpusConceptMeaningTerms(concept.SourceEvidence)
	termPhrase := corpusConceptHumanList(terms)
	switch normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind) {
	case CorpusConceptReviewWorkConceptReview:
		if termPhrase != "" {
			return fmt.Sprintf("Possible corpus concept: the %s evidence appears to describe one %s around %s.", corpusConceptSourceKindList(concept.SourceKindCoverage), kind, termPhrase)
		}
		return fmt.Sprintf("Possible corpus concept: readable evidence from %d source(s) may describe one %s.", concept.SourceCount, kind)
	case CorpusConceptReviewWorkCleanupTriage:
		if termPhrase != "" {
			return fmt.Sprintf("Cleanup item: extracted atoms look related around %s, but they are not eligible to become accepted corpus knowledge yet.", termPhrase)
		}
		return "Cleanup item: extracted atoms need triage before they can be considered for concept review."
	case CorpusConceptReviewWorkEnrichmentBacklog:
		return "Enrichment item: the system needs more readable source context before this can be judged as a concept."
	case CorpusConceptReviewWorkBlockedDiagnostic:
		return "Blocked diagnostic: quality gates found a problem that must be fixed before concept review."
	default:
		if termPhrase != "" {
			return fmt.Sprintf("Possible review item around %s.", termPhrase)
		}
		return "Possible review item needing human judgment."
	}
}

func corpusConceptAcceptMeaning(concept CorpusConcept) string {
	switch normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind) {
	case CorpusConceptReviewWorkConceptReview:
		return "Accept means the readable source contributions independently support this candidate as one accepted corpus concept. It is still reviewer feedback, not a destination write or autonomous knowledge claim."
	case CorpusConceptReviewWorkCleanupTriage:
		return "Accept is not available for cleanup triage; choose the cleanup action that best improves or discards the extraction."
	case CorpusConceptReviewWorkEnrichmentBacklog:
		return "Accept is not available until source enrichment provides readable support."
	case CorpusConceptReviewWorkBlockedDiagnostic:
		return "Accept is not available while the item is blocked by quality diagnostics."
	default:
		return "Accept only when the source evidence supports one coherent corpus concept."
	}
}

func corpusConceptDecisionRubric(concept CorpusConcept) []CorpusConceptDecisionCriterion {
	switch normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind) {
	case CorpusConceptReviewWorkConceptReview:
		return []CorpusConceptDecisionCriterion{
			{Choice: CorpusConceptReviewAccept, Label: "Accept", Criterion: "Use when every readable source contribution supports the candidate meaning as one coherent corpus concept."},
			{Choice: CorpusConceptReviewRejectNoisy, Label: "Noisy", Criterion: "Use when the grouping is accidental, too weak, or not useful as a concept."},
			{Choice: CorpusConceptReviewSplitNeeded, Label: "Split", Criterion: "Use when the evidence contains two or more distinct concepts that should be reviewed separately."},
			{Choice: CorpusConceptReviewMergeDuplicate, Label: "Merge", Criterion: "Use when this candidate duplicates another concept and should not stand alone."},
			{Choice: CorpusConceptReviewRenameNeeded, Label: "Rename", Criterion: "Use when the concept is coherent but the title or meaning label needs clearer wording."},
			{Choice: CorpusConceptReviewNeedsSourceContext, Label: "Need context", Criterion: "Use when excerpts are promising but insufficient to judge without the full source card."},
		}
	case CorpusConceptReviewWorkCleanupTriage:
		return []CorpusConceptDecisionCriterion{
			{Choice: CorpusConceptReviewRejectNoisy, Label: "Noisy", Criterion: "Use when the extracted item should be discarded as noise."},
			{Choice: CorpusConceptReviewSplitNeeded, Label: "Split", Criterion: "Use when one cleanup item contains unrelated extracted meanings."},
			{Choice: CorpusConceptReviewMergeDuplicate, Label: "Merge", Criterion: "Use when this cleanup item duplicates another cleanup item."},
			{Choice: CorpusConceptReviewRenameNeeded, Label: "Rename", Criterion: "Use when the extraction label is misleading or too vague."},
		}
	case CorpusConceptReviewWorkEnrichmentBacklog:
		return []CorpusConceptDecisionCriterion{
			{Choice: CorpusConceptReviewNeedsSourceContext, Label: "Need context", Criterion: "Use when the source must be enriched before judging the concept."},
			{Choice: CorpusConceptReviewRejectNoisy, Label: "Noisy", Criterion: "Use when enrichment would not make this item reviewable."},
		}
	case CorpusConceptReviewWorkBlockedDiagnostic:
		return []CorpusConceptDecisionCriterion{
			{Choice: CorpusConceptReviewNeedsSourceContext, Label: "Need context", Criterion: "Use when a source or evidence reference must be repaired before review."},
			{Choice: CorpusConceptReviewRejectNoisy, Label: "Noisy", Criterion: "Use when the blocked item should be discarded."},
			{Choice: CorpusConceptReviewSplitNeeded, Label: "Split", Criterion: "Use when the diagnostic points to an over-broad grouping."},
		}
	default:
		return nil
	}
}

func corpusConceptReviewPrompt(concept CorpusConcept) string {
	switch normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind) {
	case CorpusConceptReviewWorkBlockedDiagnostic:
		return "This item is blocked diagnostic evidence: decide whether the system should enrich sources, split the group, or discard it before normal review."
	case CorpusConceptReviewWorkEnrichmentBacklog:
		return "This item needs source enrichment before concept review: decide whether to request source context or discard it as noisy."
	case CorpusConceptReviewWorkCleanupTriage:
		return "Use this as extraction cleanup feedback, not as accepted knowledge."
	case CorpusConceptReviewWorkConceptReview:
		if concept.Section == CorpusConceptSectionCrossSource || len(concept.SourceKindCoverage) > 1 {
			return fmt.Sprintf("Decide whether the candidate meaning is supported by the %s source contributions.", corpusConceptSourceKindList(concept.SourceKindCoverage))
		}
	}
	return "Decide whether these evidence snippets describe one coherent review concept or need cleanup."
}

func corpusConceptSourceKindList(coverage map[string]int) string {
	keys := make([]string, 0, len(coverage))
	for key := range coverage {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "source"
	}
	if len(keys) == 1 {
		return corpusConceptSourceKindLabel(keys[0])
	}
	if len(keys) == 2 {
		return corpusConceptSourceKindLabel(keys[0]) + " and " + corpusConceptSourceKindLabel(keys[1])
	}
	labels := make([]string, 0, len(keys))
	for _, key := range keys {
		labels = append(labels, corpusConceptSourceKindLabel(key))
	}
	return strings.Join(labels[:len(labels)-1], ", ") + ", and " + labels[len(labels)-1]
}

func corpusConceptSourceKindLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "gmail":
		return "Gmail"
	case "slack":
		return "Slack"
	default:
		return strings.TrimSpace(kind)
	}
}

func corpusConceptGroupingRationale(key string, concept CorpusConcept) string {
	coverage := corpusConceptCoverageString(concept.SourceKindCoverage)
	if strings.HasPrefix(key, "relation\x00cross_source\x00") {
		return fmt.Sprintf("Grouped from cross-source same-topic or duplicate graph relations across %d source(s) with %s coverage. This is reviewer feedback only, not accepted knowledge.", concept.SourceCount, coverage)
	}
	parts := strings.Split(key, "\x00")
	if len(parts) >= 3 && parts[0] == "term" {
		return fmt.Sprintf("Grouped because atoms share the useful term %q across %d source(s) with %s coverage.", parts[2], concept.SourceCount, coverage)
	}
	if len(parts) >= 3 && parts[0] == "local" {
		return fmt.Sprintf("Grouped as a local source-kind review bucket for %s atoms with %s coverage.", strings.ReplaceAll(string(concept.CandidateKind), "_", " "), coverage)
	}
	return fmt.Sprintf("Grouped as a bounded %s review bucket across %d source(s).", strings.ReplaceAll(string(concept.CandidateKind), "_", " "), concept.SourceCount)
}

func corpusConceptRepresentativeEvidence(atoms []CorpusGraphAtom) []CorpusConceptEvidencePreview {
	byKind := map[string][]CorpusConceptEvidencePreview{}
	fallbackByKind := map[string][]CorpusConceptEvidencePreview{}
	kinds := []string{}
	for _, atom := range atoms {
		kind := sourceKindForConcept(atom)
		if _, ok := byKind[kind]; !ok {
			kinds = append(kinds, kind)
		}
		preview := corpusConceptEvidencePreview(atom)
		if corpusConceptReviewableEvidencePreview(preview) {
			byKind[kind] = append(byKind[kind], preview)
		} else {
			fallbackByKind[kind] = append(fallbackByKind[kind], preview)
		}
	}
	sort.Strings(kinds)
	out := corpusConceptRoundRobinEvidencePreviews(kinds, byKind, corpusConceptPreviewMax)
	if len(out) == 0 {
		out = append(out, corpusConceptRoundRobinEvidencePreviews(kinds, fallbackByKind, corpusConceptPreviewMax)...)
	}
	return out
}

func corpusConceptSourceEvidence(atoms []CorpusGraphAtom) []CorpusConceptSourceEvidence {
	bySource := map[string][]CorpusGraphAtom{}
	sourceIDs := []string{}
	for _, atom := range atoms {
		if _, ok := bySource[atom.SourceID]; !ok {
			sourceIDs = append(sourceIDs, atom.SourceID)
		}
		bySource[atom.SourceID] = append(bySource[atom.SourceID], atom)
	}
	sort.Strings(sourceIDs)
	out := []CorpusConceptSourceEvidence{}
	for _, sourceID := range sourceIDs {
		sourceAtoms := bySource[sourceID]
		sortCorpusConceptAtoms(sourceAtoms)
		group := CorpusConceptSourceEvidence{
			SourceID:   sourceID,
			SourceKind: sourceKindForConcept(sourceAtoms[0]),
			SourceRef:  corpusConceptSourceRef(sourceID, sourceKindForConcept(sourceAtoms[0])),
			AtomCount:  len(sourceAtoms),
		}
		if group.AtomCount > 1 {
			group.DuplicateAtomCount = group.AtomCount - 1
			group.Flags = appendUniqueString(group.Flags, "duplicate_source_atom_support")
		}
		fallback := []CorpusConceptEvidencePreview{}
		for _, atom := range sourceAtoms {
			preview := corpusConceptEvidencePreview(atom)
			if corpusConceptReviewableEvidencePreview(preview) {
				group.ReviewableAtomCount++
				if len(group.Evidence) < corpusConceptSourcePreviewMax {
					group.Evidence = append(group.Evidence, preview)
				}
			} else if len(fallback) < corpusConceptSourcePreviewMax {
				fallback = append(fallback, preview)
			}
		}
		if len(group.Evidence) == 0 {
			group.Evidence = fallback
		}
		group.LinkOnly = corpusConceptSourceLinkOnly(group)
		if group.LinkOnly {
			group.Flags = appendUniqueString(group.Flags, "link_only_evidence_requires_enrichment")
			group.ReviewableAtomCount = 0
		} else if group.ReviewableAtomCount == 0 {
			group.Flags = appendUniqueString(group.Flags, "no_readable_source_evidence")
		}
		group.SharedTerms = corpusConceptSharedSourceTerms(group)
		group.Contribution = corpusConceptSourceContribution(group)
		out = append(out, group)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReviewableAtomCount != out[j].ReviewableAtomCount {
			return out[i].ReviewableAtomCount > out[j].ReviewableAtomCount
		}
		if out[i].SourceKind != out[j].SourceKind {
			return out[i].SourceKind < out[j].SourceKind
		}
		return out[i].SourceID < out[j].SourceID
	})
	return out
}

func corpusConceptSourceContribution(source CorpusConceptSourceEvidence) string {
	sourceLabel := corpusConceptSourceKindLabel(source.SourceKind)
	if strings.TrimSpace(sourceLabel) == "" {
		sourceLabel = "This source"
	}
	if source.LinkOnly {
		return fmt.Sprintf("%s contributes only link-style evidence; enrich it before using it as independent support.", sourceLabel)
	}
	if source.ReviewableAtomCount == 0 {
		return fmt.Sprintf("%s does not yet contribute readable support for a concept decision.", sourceLabel)
	}
	terms := corpusConceptSourceMeaningTerms(source)
	termPhrase := corpusConceptHumanList(terms)
	evidencePhrase := corpusConceptSourceEvidencePhrase(source)
	if termPhrase != "" && evidencePhrase != "" {
		return fmt.Sprintf("%s contributes readable evidence around %s: %s.", sourceLabel, termPhrase, evidencePhrase)
	}
	if termPhrase != "" {
		return fmt.Sprintf("%s contributes readable evidence around %s.", sourceLabel, termPhrase)
	}
	if evidencePhrase != "" {
		return fmt.Sprintf("%s contributes readable evidence: %s.", sourceLabel, evidencePhrase)
	}
	return fmt.Sprintf("%s contributes readable evidence for this review item.", sourceLabel)
}

func corpusConceptSourceContributionList(sources []CorpusConceptSourceEvidence) []string {
	out := []string{}
	for _, source := range sources {
		if strings.TrimSpace(source.Contribution) != "" {
			out = append(out, source.Contribution)
		}
	}
	return out
}

func applyCorpusConceptSourceQualityGates(concept *CorpusConcept, key string) {
	claimsCrossSource := concept.Section == CorpusConceptSectionCrossSource
	readableSources := []CorpusConceptSourceEvidence{}
	for _, source := range concept.SourceEvidence {
		for _, flag := range source.Flags {
			concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, flag)
		}
		if source.ReviewableAtomCount > 0 && !source.LinkOnly {
			readableSources = append(readableSources, source)
		}
	}
	if strings.HasPrefix(key, "relation\x00cross_source\x00") {
		applyCorpusConceptReadableOutlierGate(concept, readableSources)
	}
	if strings.HasPrefix(key, "term\x00") {
		applyCorpusConceptGenericTermGate(concept, readableSources)
	}
	if concept.SourceCount > 1 && len(readableSources) < 2 {
		concept.Section = CorpusConceptSectionBlocked
		concept.ReviewStatus = ReviewStatusBlocked
		concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "insufficient_reviewable_source_support")
		return
	}
	if claimsCrossSource && corpusConceptReadableSourceKindCount(readableSources) < 2 {
		concept.Section = CorpusConceptSectionBlocked
		concept.ReviewStatus = ReviewStatusBlocked
		concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "insufficient_readable_source_kind_support")
	}
	if strings.HasPrefix(key, "relation\x00cross_source\x00") && !corpusConceptHasSourceLevelOverlap(readableSources) {
		concept.Section = CorpusConceptSectionBlocked
		concept.ReviewStatus = ReviewStatusBlocked
		concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "weak_cross_source_coherence")
	}
}

func applyCorpusConceptGenericTermGate(concept *CorpusConcept, readableSources []CorpusConceptSourceEvidence) {
	if len(readableSources) < 2 || corpusConceptReadableSourceKindCount(readableSources) > 1 {
		return
	}
	coreTerms := corpusConceptCoreTerms(readableSources)
	if len(coreTerms) == 0 {
		return
	}
	for term := range coreTerms {
		if !corpusConceptWeakGroupingTerm(term) {
			return
		}
	}
	concept.Section = CorpusConceptSectionBlocked
	concept.ReviewStatus = ReviewStatusBlocked
	concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "generic_term_bucket_requires_cleanup")
}

func applyCorpusConceptReadableOutlierGate(concept *CorpusConcept, readableSources []CorpusConceptSourceEvidence) {
	coreTerms := corpusConceptCoreTerms(readableSources)
	if len(coreTerms) == 0 {
		return
	}
	outlierFound := false
	for index := range concept.SourceEvidence {
		source := &concept.SourceEvidence[index]
		if source.ReviewableAtomCount == 0 || source.LinkOnly {
			continue
		}
		if corpusConceptSourceSharesCoreTerm(*source, coreTerms) {
			continue
		}
		source.Flags = appendUniqueString(source.Flags, "readable_source_outlier")
		outlierFound = true
	}
	if outlierFound {
		concept.Section = CorpusConceptSectionBlocked
		concept.ReviewStatus = ReviewStatusBlocked
		concept.ReasonCodes = appendUniqueString(concept.ReasonCodes, "readable_source_outlier")
	}
}

func corpusConceptReadableSourceKindCount(sources []CorpusConceptSourceEvidence) int {
	kinds := map[string]bool{}
	for _, source := range sources {
		if source.ReviewableAtomCount > 0 && !source.LinkOnly {
			kinds[source.SourceKind] = true
		}
	}
	return len(kinds)
}

func corpusConceptMeaningTerms(sources []CorpusConceptSourceEvidence) []string {
	readableSources := []CorpusConceptSourceEvidence{}
	for _, source := range sources {
		if source.ReviewableAtomCount > 0 && !source.LinkOnly {
			readableSources = append(readableSources, source)
		}
	}
	core := corpusConceptCoreTerms(readableSources)
	terms := make([]string, 0, len(core))
	for term := range core {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	if len(terms) == 0 {
		counts := map[string]int{}
		for _, source := range readableSources {
			for _, term := range corpusConceptSourceMeaningTerms(source) {
				counts[term]++
			}
		}
		for term := range counts {
			terms = append(terms, term)
		}
		sort.Slice(terms, func(i, j int) bool {
			if counts[terms[i]] != counts[terms[j]] {
				return counts[terms[i]] > counts[terms[j]]
			}
			return terms[i] < terms[j]
		})
	}
	if len(terms) > 4 {
		terms = terms[:4]
	}
	return terms
}

func corpusConceptSourceMeaningTerms(source CorpusConceptSourceEvidence) []string {
	terms := append([]string{}, source.SharedTerms...)
	if len(terms) > 4 {
		terms = terms[:4]
	}
	return terms
}

func corpusConceptHumanList(values []string) string {
	clean := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			clean = append(clean, value)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	if len(clean) == 1 {
		return clean[0]
	}
	if len(clean) == 2 {
		return clean[0] + " and " + clean[1]
	}
	return strings.Join(clean[:len(clean)-1], ", ") + ", and " + clean[len(clean)-1]
}

func corpusConceptSourceEvidencePhrase(source CorpusConceptSourceEvidence) string {
	for _, preview := range source.Evidence {
		text := strings.TrimSpace(preview.Title)
		if text == "" {
			text = strings.TrimSpace(preview.Summary)
		}
		if text == "" {
			text = strings.TrimSpace(preview.Excerpt)
		}
		if text == "" {
			continue
		}
		text = strings.Join(strings.Fields(text), " ")
		runes := []rune(text)
		if len(runes) > 150 {
			text = strings.TrimSpace(string(runes[:150])) + "..."
		}
		return strings.TrimRight(text, ".;:!? ")
	}
	return ""
}

func corpusConceptCoreTerms(sources []CorpusConceptSourceEvidence) map[string]bool {
	termSourceCount := map[string]int{}
	for _, source := range sources {
		seen := map[string]bool{}
		for _, term := range source.SharedTerms {
			if seen[term] {
				continue
			}
			seen[term] = true
			termSourceCount[term]++
		}
	}
	core := map[string]bool{}
	for term, count := range termSourceCount {
		if count >= 2 {
			core[term] = true
		}
	}
	return core
}

func corpusConceptSourceSharesCoreTerm(source CorpusConceptSourceEvidence, coreTerms map[string]bool) bool {
	for _, term := range source.SharedTerms {
		if coreTerms[term] {
			return true
		}
	}
	return false
}

func corpusConceptHasSourceLevelOverlap(sources []CorpusConceptSourceEvidence) bool {
	termSourceCount := map[string]int{}
	for _, source := range sources {
		for _, term := range source.SharedTerms {
			termSourceCount[term]++
			if termSourceCount[term] >= 2 {
				return true
			}
		}
	}
	return false
}

func corpusConceptSharedSourceTerms(source CorpusConceptSourceEvidence) []string {
	counts := map[string]int{}
	for _, preview := range source.Evidence {
		for _, term := range corpusConceptTermsFromText(strings.Join([]string{preview.Title, preview.Summary, preview.Excerpt}, " ")) {
			counts[term]++
		}
	}
	terms := make([]string, 0, len(counts))
	for term := range counts {
		terms = append(terms, term)
	}
	sort.Slice(terms, func(i, j int) bool {
		if counts[terms[i]] != counts[terms[j]] {
			return counts[terms[i]] > counts[terms[j]]
		}
		return terms[i] < terms[j]
	})
	if len(terms) > 8 {
		terms = terms[:8]
	}
	return terms
}

func corpusConceptSourceLinkOnly(source CorpusConceptSourceEvidence) bool {
	if len(source.Evidence) == 0 {
		return false
	}
	for _, preview := range source.Evidence {
		if !corpusConceptPreviewLinkOnly(preview) {
			return false
		}
	}
	return true
}

func corpusConceptRoundRobinEvidencePreviews(kinds []string, byKind map[string][]CorpusConceptEvidencePreview, maxItems int) []CorpusConceptEvidencePreview {
	if maxItems <= 0 {
		return nil
	}
	out := []CorpusConceptEvidencePreview{}
	seen := map[string]bool{}
	for len(out) < maxItems {
		added := false
		for _, kind := range kinds {
			if len(byKind[kind]) == 0 {
				continue
			}
			preview := byKind[kind][0]
			byKind[kind] = byKind[kind][1:]
			key := corpusConceptEvidencePreviewKey(preview)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, preview)
			added = true
			if len(out) >= maxItems {
				break
			}
		}
		if !added {
			break
		}
	}
	return out
}

func corpusConceptEvidencePreviewKey(preview CorpusConceptEvidencePreview) string {
	excerpt := strings.ToLower(strings.TrimSpace(preview.Excerpt))
	if excerpt != "" {
		return preview.SourceKind + "\x00" + excerpt
	}
	return preview.AtomID
}

func corpusConceptEvidencePreview(atom CorpusGraphAtom) CorpusConceptEvidencePreview {
	evidence := sourceMeaningEvidenceRef(atom)
	return CorpusConceptEvidencePreview{
		EvidenceRefID: evidence.EvidenceRefID,
		AtomID:        atom.AtomID,
		SourceID:      atom.SourceID,
		SourceKind:    sourceKindForConcept(atom),
		SourceRef:     corpusConceptSourceRef(atom.SourceID, sourceKindForConcept(atom)),
		LineStart:     atom.LineStart,
		LineEnd:       atom.LineEnd,
		ContentHash:   atom.ContentHash,
		Title:         corpusConceptSafeEvidenceTitle(atom.Title),
		Summary:       corpusConceptSafeDisplayText(atom.Summary, 260),
		Excerpt:       corpusConceptSafeDisplayText(atom.Excerpt, 520),
	}
}

func corpusConceptReviewableEvidencePreview(preview CorpusConceptEvidencePreview) bool {
	text := strings.ToLower(strings.Join([]string{preview.Title, preview.Summary, preview.Excerpt}, "\n"))
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	metadataMarkers := []string{
		"raw locator:",
		"candidate id:",
		"source id:",
		"content hash:",
		"gmail timestamp:",
		"timestamp:",
		"sha256:",
		"yhc-private-runtime/",
	}
	for _, marker := range metadataMarkers {
		if strings.Contains(text, marker) {
			return false
		}
	}
	return true
}

func corpusConceptTopTerms(atoms []CorpusGraphAtom, maxTerms int) []string {
	counts := map[string]int{}
	firstSeen := map[string]int{}
	order := 0
	for _, atom := range atoms {
		for _, term := range corpusConceptTerms(atom) {
			counts[term]++
			if _, ok := firstSeen[term]; !ok {
				firstSeen[term] = order
				order++
			}
		}
	}
	terms := make([]string, 0, len(counts))
	for term := range counts {
		terms = append(terms, term)
	}
	sort.Slice(terms, func(i, j int) bool {
		if counts[terms[i]] != counts[terms[j]] {
			return counts[terms[i]] > counts[terms[j]]
		}
		if firstSeen[terms[i]] != firstSeen[terms[j]] {
			return firstSeen[terms[i]] < firstSeen[terms[j]]
		}
		return terms[i] < terms[j]
	})
	if len(terms) > maxTerms {
		terms = terms[:maxTerms]
	}
	return terms
}

func corpusConceptSafeDisplayText(value string, maxLen int) string {
	value = strings.TrimSpace(safeSourceMeaningExcerpt(value))
	value = strings.Join(strings.Fields(value), " ")
	if maxLen > 0 && len(value) > maxLen {
		value = strings.TrimSpace(value[:maxLen])
		value += "..."
	}
	return value
}

func corpusConceptSafeEvidenceTitle(value string) string {
	value = corpusConceptSafeDisplayText(value, 140)
	for {
		lower := strings.ToLower(value)
		changed := false
		for _, prefix := range []string{"topic candidate:", "action candidate:", "decision candidate:", "issue candidate:", "reference candidate:", "snippet:"} {
			if strings.HasPrefix(lower, prefix) {
				value = strings.TrimSpace(value[len(prefix):])
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	return value
}

func corpusConceptSourceRef(sourceID, sourceKind string) string {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return "unknown"
	}
	kind := strings.ToLower(strings.TrimSpace(sourceKind))
	if kind == "" || kind == "unknown" {
		kind = "source"
	}
	if len(sourceID) <= 12 {
		return kind + ":" + sourceID
	}
	return kind + ":" + sourceID[len(sourceID)-12:]
}

func corpusConceptID(corpusID, key string, atoms []CorpusGraphAtom) string {
	parts := []string{corpusID, key}
	for _, atom := range atoms {
		parts = append(parts, atom.AtomID)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "concept-" + hex.EncodeToString(sum[:])[:16]
}

func corpusConceptReplayFingerprint(summary CorpusConceptSummary) string {
	parts := []string{
		summary.CorpusID,
		summary.CorpusFingerprint,
		summary.CommandConfigFingerprint,
		fmt.Sprintf("counts:%d:%d:%d:%d:%d:%d", summary.AtomCount, summary.RelationCount, summary.ConceptCount, summary.CrossSourceConceptCount, summary.EvidenceReferenceCount, summary.ConceptReviewBurdenCount),
	}
	for _, concept := range summary.Concepts {
		parts = append(parts, strings.Join([]string{
			concept.ConceptID,
			string(concept.Section),
			string(concept.CandidateKind),
			string(concept.ReviewWorkKind),
			string(concept.ReviewStatus),
			fmt.Sprintf("%d", concept.AtomCount),
			concept.ReviewPrompt,
			concept.CandidateMeaning,
			concept.AcceptMeaning,
		}, "\x00"))
		for _, criterion := range concept.DecisionRubric {
			parts = append(parts, strings.Join([]string{string(criterion.Choice), criterion.Label, criterion.Criterion}, "\x00"))
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "corpus-concepts-" + hex.EncodeToString(sum[:])[:16]
}

func corpusConceptSortKey(concept CorpusConcept) string {
	sectionRank := map[CorpusConceptSection]string{
		CorpusConceptSectionCrossSource: "0",
		CorpusConceptSectionNeedsReview: "1",
		CorpusConceptSectionLocal:       "2",
		CorpusConceptSectionBlocked:     "3",
	}
	rank := sectionRank[concept.Section]
	return fmt.Sprintf("%s:%03d:%03d:%s", rank, 999-concept.SourceCount, 999-concept.AtomCount, concept.ConceptID)
}

func sortCorpusConceptAtoms(atoms []CorpusGraphAtom) {
	sort.Slice(atoms, func(i, j int) bool {
		return strings.Join([]string{atoms[i].SourceID, fmt.Sprintf("%08d", atoms[i].LineStart), atoms[i].AtomID}, "\x00") <
			strings.Join([]string{atoms[j].SourceID, fmt.Sprintf("%08d", atoms[j].LineStart), atoms[j].AtomID}, "\x00")
	})
}

func sourceKindForConcept(atom CorpusGraphAtom) string {
	if strings.TrimSpace(atom.SourceKind) != "" {
		return strings.ToLower(strings.TrimSpace(atom.SourceKind))
	}
	return "unknown"
}

func CorpusConceptReviewContractFingerprint(index CorpusConceptIndex) string {
	concepts := append([]CorpusConcept{}, index.Concepts...)
	sort.Slice(concepts, func(i, j int) bool { return concepts[i].ConceptID < concepts[j].ConceptID })
	parts := []string{index.SchemaVersion, index.CorpusID}
	for _, concept := range concepts {
		parts = append(parts, strings.Join([]string{
			concept.ConceptID,
			string(normalizeCorpusConceptReviewWorkKind(concept.ReviewWorkKind)),
			string(concept.Section),
			string(concept.CandidateKind),
			string(concept.ReviewStatus),
			concept.Title,
			concept.ReviewPrompt,
			concept.CandidateMeaning,
			concept.AcceptMeaning,
			strings.Join(concept.ReasonCodes, ","),
		}, "\x00"))
		for _, criterion := range concept.DecisionRubric {
			parts = append(parts, strings.Join([]string{string(criterion.Choice), criterion.Label, criterion.Criterion}, "\x00"))
		}
		for _, evidence := range concept.EvidenceRefs {
			parts = append(parts, strings.Join([]string{evidence.EvidenceRefID, evidence.AtomID, evidence.SourceID, evidence.ContentHash}, "\x00"))
		}
		sources := append([]CorpusConceptSourceEvidence{}, concept.SourceEvidence...)
		sort.Slice(sources, func(i, j int) bool { return sources[i].SourceID < sources[j].SourceID })
		for _, source := range sources {
			flags := append([]string{}, source.Flags...)
			sharedTerms := append([]string{}, source.SharedTerms...)
			sort.Strings(flags)
			sort.Strings(sharedTerms)
			parts = append(parts, strings.Join([]string{
				source.SourceID,
				source.SourceKind,
				source.SourceRef,
				fmt.Sprintf("%d", source.AtomCount),
				fmt.Sprintf("%d", source.ReviewableAtomCount),
				fmt.Sprintf("%d", source.DuplicateAtomCount),
				fmt.Sprintf("%t", source.LinkOnly),
				source.Contribution,
				strings.Join(flags, ","),
				strings.Join(sharedTerms, ","),
			}, "\x00"))
			sourceEvidence := append([]CorpusConceptEvidencePreview{}, source.Evidence...)
			sort.Slice(sourceEvidence, func(i, j int) bool {
				return sourceEvidence[i].EvidenceRefID+"\x00"+sourceEvidence[i].AtomID < sourceEvidence[j].EvidenceRefID+"\x00"+sourceEvidence[j].AtomID
			})
			for _, evidence := range sourceEvidence {
				parts = append(parts, corpusConceptEvidenceContractFingerprintPart(evidence))
			}
		}
		representativeEvidence := append([]CorpusConceptEvidencePreview{}, concept.RepresentativeEvidence...)
		sort.Slice(representativeEvidence, func(i, j int) bool {
			return representativeEvidence[i].EvidenceRefID+"\x00"+representativeEvidence[i].AtomID < representativeEvidence[j].EvidenceRefID+"\x00"+representativeEvidence[j].AtomID
		})
		for _, evidence := range representativeEvidence {
			parts = append(parts, corpusConceptEvidenceContractFingerprintPart(evidence))
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "corpus-concept-review-contract-" + hex.EncodeToString(sum[:])[:16]
}

func corpusConceptEvidenceContractFingerprintPart(evidence CorpusConceptEvidencePreview) string {
	return strings.Join([]string{
		evidence.EvidenceRefID,
		evidence.AtomID,
		evidence.SourceID,
		evidence.SourceKind,
		evidence.SourceRef,
		fmt.Sprintf("%d", evidence.LineStart),
		fmt.Sprintf("%d", evidence.LineEnd),
		evidence.ContentHash,
		evidence.Title,
		evidence.Summary,
		evidence.Excerpt,
	}, "\x00")
}

func cloneStringIntMap(input map[string]int) map[string]int {
	out := map[string]int{}
	for key, value := range input {
		out[key] = value
	}
	return out
}

func containsCorpusConceptString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
