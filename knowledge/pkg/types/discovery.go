package types

import "time"

type DiscoveryStatus string

const (
	StatusPending   DiscoveryStatus = "pending"
	StatusRunning   DiscoveryStatus = "running"
	StatusCompleted DiscoveryStatus = "completed"
	StatusFailed    DiscoveryStatus = "failed"
)

type DiscoveryScope struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	PluginName       string            `json:"plugin_name"`
	Config           map[string]string `json:"config,omitempty"`
	CredentialSource *CredentialSource `json:"credential_source,omitempty"`
	Schedule         string            `json:"schedule,omitempty"`
	Status           string            `json:"status"`
	TenantID         string            `json:"tenant_id,omitempty"`
	LastRun          *time.Time        `json:"last_run,omitempty"`
	ResourceCount    int               `json:"resource_count"`
}

type CredentialSource struct {
	Type   string            `json:"type"`
	Path   string            `json:"path,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

type DiscoveryRun struct {
	ID                 string          `json:"id"`
	ScopeID            string          `json:"scope_id"`
	Status             DiscoveryStatus `json:"status"`
	StartedAt          time.Time       `json:"started_at"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
	ResourcesFound     int             `json:"resources_found"`
	RelationshipsFound int             `json:"relationships_found"`
	Errors             []string        `json:"errors,omitempty"`
}

func (r *DiscoveryRun) Duration() time.Duration {
	if r.CompletedAt != nil {
		return r.CompletedAt.Sub(r.StartedAt)
	}
	return time.Since(r.StartedAt)
}
