package types

import "time"

// ChangeType classifies how a resource changed between discovery runs.
type ChangeType string

const (
	ChangeAdded    ChangeType = "added"
	ChangeRemoved  ChangeType = "removed"
	ChangeModified ChangeType = "modified"
)

// ResourceChange records a single diff detected between two discovery runs.
type ResourceChange struct {
	ID          string            `json:"id"`
	ScopeID     string            `json:"scope_id"`
	RunID       string            `json:"run_id"`
	ResourceID  string            `json:"resource_id"`
	TenantID    string            `json:"tenant_id"`
	ChangeType  ChangeType        `json:"change_type"`
	BeforeProps map[string]string `json:"before_props,omitempty"`
	AfterProps  map[string]string `json:"after_props,omitempty"`
	DetectedAt  time.Time         `json:"detected_at"`
}
