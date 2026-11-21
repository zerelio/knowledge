package types

import "time"

// FindingSeverity classifies the urgency of a compliance finding.
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "critical"
	SeverityHigh     FindingSeverity = "high"
	SeverityMedium   FindingSeverity = "medium"
	SeverityLow      FindingSeverity = "low"
	SeverityInfo     FindingSeverity = "info"
)

// FindingStatus tracks the lifecycle of a compliance finding.
type FindingStatus string

const (
	FindingStatusOpen       FindingStatus = "open"
	FindingStatusSuppressed FindingStatus = "suppressed"
	FindingStatusResolved   FindingStatus = "resolved"
)

// ComplianceFinding records a single policy violation on a resource.
type ComplianceFinding struct {
	ID          string          `json:"id"`
	ResourceID  string          `json:"resource_id"`
	TenantID    string          `json:"tenant_id"`
	ScopeID     string          `json:"scope_id"`
	RuleID      string          `json:"rule_id"`
	RuleName    string          `json:"rule_name"`
	Severity    FindingSeverity `json:"severity"`
	Message     string          `json:"message"`
	Remediation string          `json:"remediation,omitempty"`
	Status      FindingStatus   `json:"status"`
	EvaluatedAt time.Time       `json:"evaluated_at"`
}
