package types

import "time"

// GenerationType identifies what produced a generation.
type GenerationType string

const (
	GenerationTypeDoc     GenerationType = "doc"
	GenerationTypeRunbook GenerationType = "runbook"
	GenerationTypeRerank  GenerationType = "rerank"
)

// AnomalyType classifies problems found in a generation.
type AnomalyType string

const (
	AnomalyHallucination      AnomalyType = "hallucination"
	AnomalyOutdated           AnomalyType = "outdated"
	AnomalyIncorrect          AnomalyType = "incorrect"
	AnomalyMissingContext     AnomalyType = "missing_context"
	AnomalyIrrelevantRetrieval AnomalyType = "irrelevant_retrieval"
)

// DocumentStatus tracks human review state.
type DocumentStatus string

const (
	DocumentDraft    DocumentStatus = "draft"
	DocumentApproved DocumentStatus = "approved"
	DocumentRejected DocumentStatus = "rejected"
	DocumentFlagged  DocumentStatus = "flagged"
)

// GenerationLog records a single LLM call with its full context.
// This is the primary auditability record — it contains everything needed
// to reproduce and evaluate the output.
type GenerationLog struct {
	ID               string            `json:"id"`
	Type             GenerationType    `json:"type"`
	TenantID         string            `json:"tenant_id,omitempty"`
	UserID           string            `json:"user_id,omitempty"`
	ResourceID       string            `json:"resource_id,omitempty"`
	Query            string            `json:"query,omitempty"`
	ContextText      string            `json:"context_text,omitempty"` // full prompt context
	RetrievedIDs     []string          `json:"retrieved_ids,omitempty"`
	RetrievalSignals map[string]any    `json:"retrieval_signals,omitempty"`
	Model            string            `json:"model"`
	Provider         string            `json:"provider"`
	TokensPrompt     int               `json:"tokens_prompt"`
	TokensCompletion int               `json:"tokens_completion"`
	LatencyMS        int               `json:"latency_ms"`
	Output           string            `json:"output"`
	AutoScore        *float64          `json:"auto_score,omitempty"` // RAGAS-lite score
	CreatedAt        time.Time         `json:"created_at"`
}

// GenerationFeedback is a human annotation on a GenerationLog.
type GenerationFeedback struct {
	ID                 string      `json:"id"`
	GenerationID       string      `json:"generation_id"`
	UserID             string      `json:"user_id"`
	TenantID           string      `json:"tenant_id,omitempty"`
	Rating             int         `json:"rating"` // -1, 0, 1
	AnomalyType        AnomalyType `json:"anomaly_type,omitempty"`
	Comment            string      `json:"comment,omitempty"`
	CorrectedOutput    string      `json:"corrected_output,omitempty"`
	MissingResourceIDs []string    `json:"missing_resource_ids,omitempty"`
	SpuriousResourceIDs []string   `json:"spurious_resource_ids,omitempty"`
	CreatedAt          time.Time   `json:"created_at"`
}

// CachedDocument is a generated and optionally human-reviewed resource dossier.
type CachedDocument struct {
	ID           string         `json:"id"`
	ResourceID   string         `json:"resource_id"`
	Title        string         `json:"title"`
	Content      string         `json:"content"`
	GenerationID string         `json:"generation_id,omitempty"`
	Status       DocumentStatus `json:"status"`
	AutoScore    *float64       `json:"auto_score,omitempty"`
	ApprovedBy   string         `json:"approved_by,omitempty"`
	ApprovedAt   *time.Time     `json:"approved_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// QualityMetrics aggregates feedback for a reporting period.
type QualityMetrics struct {
	TotalGenerations  int     `json:"total_generations"`
	WithFeedback      int     `json:"with_feedback"`
	ApprovalRate      float64 `json:"approval_rate"`      // % rated +1
	RejectionRate     float64 `json:"rejection_rate"`     // % rated -1
	HallucinationRate float64 `json:"hallucination_rate"` // % flagged as hallucination
	AvgAutoScore      float64 `json:"avg_auto_score"`
	ByType            map[string]int `json:"by_type"`
	ByAnomaly         map[string]int `json:"by_anomaly"`
}
