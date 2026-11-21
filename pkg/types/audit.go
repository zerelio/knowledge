package types

import "time"

// AuditEvent records a user action for compliance and security monitoring.
type AuditEvent struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	TenantID   string    `json:"tenant_id,omitempty"`
	Action     string    `json:"action"`       // e.g. "POST /api/v1/discovery/scopes"
	ResourceID string    `json:"resource_id,omitempty"`
	IP         string    `json:"ip,omitempty"`
	StatusCode int       `json:"status_code"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
