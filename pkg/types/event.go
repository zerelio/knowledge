package types

import "time"

// EventType names the category of an event emitted by the system.
type EventType string

const (
	EventResourceCreated    EventType = "resource.created"
	EventResourceUpdated    EventType = "resource.updated"
	EventResourceDeleted    EventType = "resource.deleted"
	EventDiscoveryCompleted EventType = "discovery.completed"
	EventDiscoveryFailed    EventType = "discovery.failed"
	EventChangeDetected     EventType = "change.detected"
)

// Event is the envelope delivered to webhook subscribers.
type Event struct {
	ID         string    `json:"id"`
	Type       EventType `json:"type"`
	TenantID   string    `json:"tenant_id"`
	ResourceID string    `json:"resource_id,omitempty"`
	ScopeID    string    `json:"scope_id,omitempty"`
	RunID      string    `json:"run_id,omitempty"`
	Payload    any       `json:"payload,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// WebhookSubscription represents a registered webhook endpoint.
type WebhookSubscription struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret,omitempty"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}
