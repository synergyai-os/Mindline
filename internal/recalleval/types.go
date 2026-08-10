// Package recalleval defines private, fingerprint-only retrieval evaluation
// contracts. The manifest intentionally contains no source text, URLs, or
// native record identifiers, so its structural results can be inspected safely.
package recalleval

const (
	ManifestSchemaVersion = "mindline-retrieval-eval/v0.1"
	ResultSchemaVersion   = "mindline-retrieval-eval-result/v0.1"

	CaseAnswerable = "answerable"
	CaseNoAnswer   = "no_answer"
)

// Manifest is owner-only evaluation state. Query and record values are carried
// only as SHA-256 fingerprints; the source text and native identifiers remain
// outside this contract.
type Manifest struct {
	SchemaVersion         string `json:"schema_version"`
	LibraryFingerprint    string `json:"library_fingerprint"`
	BaselineBuild         string `json:"baseline_build_fingerprint"`
	ReviewerFingerprint   string `json:"reviewer_fingerprint"`
	LabelsFrozenBeforeRun bool   `json:"labels_frozen_before_run"`
	Cases                 []Case `json:"cases"`
	Fingerprint           string `json:"fingerprint,omitempty"`
}

type Case struct {
	CaseID                     string   `json:"case_id"`
	Kind                       string   `json:"kind"`
	QueryFingerprint           string   `json:"query_fingerprint"`
	ExpectedRecordFingerprints []string `json:"expected_record_fingerprints"`
}

// Evaluation is one run over exactly one frozen library and manifest.
type Evaluation struct {
	SchemaVersion             string       `json:"schema_version"`
	LibraryFingerprint        string       `json:"library_fingerprint"`
	ManifestFingerprint       string       `json:"manifest_fingerprint"`
	BuildFingerprint          string       `json:"build_fingerprint"`
	UnselectedHydratedContent bool         `json:"unselected_hydrated_content"`
	Cases                     []CaseResult `json:"cases"`
}

type CaseResult struct {
	CaseID    string     `json:"case_id"`
	Citations []Citation `json:"citations"`
}

// Citation is deliberately structural. Valid is supplied by the local
// evaluator after checking the selected citation's canonical evidence fields.
type Citation struct {
	RecordFingerprint string `json:"record_fingerprint"`
	Valid             bool   `json:"valid"`
}

type Metrics struct {
	AnswerableCases           int     `json:"answerable_cases"`
	NoAnswerCases             int     `json:"no_answer_cases"`
	RecallAt5                 float64 `json:"recall_at_5"`
	PrecisionAt5              float64 `json:"precision_at_5"`
	CitationCompleteness      float64 `json:"citation_completeness"`
	NoAnswerFalsePositiveRate float64 `json:"no_answer_false_positive_rate"`
	ReturnedCitationCount     int     `json:"returned_citation_count"`
	FullyValidCitationCount   int     `json:"fully_valid_citation_count"`
}

type ThresholdResult struct {
	SchemaVersion       string   `json:"schema_version"`
	LibraryFingerprint  string   `json:"library_fingerprint"`
	ManifestFingerprint string   `json:"manifest_fingerprint"`
	Baseline            Metrics  `json:"baseline"`
	Candidate           Metrics  `json:"candidate"`
	Passed              bool     `json:"passed"`
	ReasonCodes         []string `json:"reason_codes"`
	Formulas            []string `json:"formulas"`
}
