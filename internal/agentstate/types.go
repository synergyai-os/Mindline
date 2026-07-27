package agentstate

import "time"

const SchemaVersion = "mindline-agent-state/v0.1"

type Lens struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Query     string `json:"query"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Embedding struct {
	DocumentID          string
	DocumentFingerprint string
	Model               string
	Vector              []float64
}

type CandidateTrace struct {
	RecordID       string             `json:"record_id"`
	Rank           int                `json:"rank"`
	FinalScore     float64            `json:"final_score"`
	ComponentScore map[string]float64 `json:"component_scores"`
}

type RetrievalTrace struct {
	RunID              string           `json:"run_id"`
	Query              string           `json:"query"`
	LensID             string           `json:"lens_id,omitempty"`
	RetrievalMethod    string           `json:"retrieval_method"`
	LibraryFingerprint string           `json:"library_fingerprint"`
	CreatedAt          string           `json:"created_at"`
	Candidates         []CandidateTrace `json:"candidates"`
}

type JudgmentRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	RetryToken     string `json:"retry_token,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	LensID         string `json:"lens_id"`
	RecordID       string `json:"record_id,omitempty"`
	Actor          string `json:"actor"`
	Disposition    string `json:"disposition,omitempty"`
	Reason         string `json:"reason,omitempty"`
	ReversesID     string `json:"reverses_judgment_id,omitempty"`
}

type Judgment struct {
	JudgmentID     string  `json:"judgment_id"`
	IdempotencyKey string  `json:"idempotency_key"`
	RunID          string  `json:"run_id,omitempty"`
	LensID         string  `json:"lens_id"`
	RecordID       string  `json:"record_id"`
	Actor          string  `json:"actor"`
	Disposition    string  `json:"disposition"`
	Reason         string  `json:"reason,omitempty"`
	ReversesID     string  `json:"reverses_judgment_id,omitempty"`
	Effect         float64 `json:"effect"`
	CreatedAt      string  `json:"created_at"`
	Replayed       bool    `json:"replayed"`
}

type Status struct {
	SchemaVersion      string `json:"schema_version"`
	LensCount          int    `json:"lens_count"`
	RetrievalRunCount  int    `json:"retrieval_run_count"`
	JudgmentCount      int    `json:"judgment_count"`
	EmbeddingCount     int    `json:"embedding_count"`
	IndexedFingerprint string `json:"indexed_library_fingerprint,omitempty"`
	DatabasePath       string `json:"database_path"`
	RecoveryState      string `json:"recovery_state,omitempty"`
}

type Clock func() time.Time
