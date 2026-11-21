package types

import (
	"encoding/json"
	"fmt"
	"time"
)

type Resource struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	Provider     string            `json:"provider"`
	Region       string            `json:"region,omitempty"`
	AccountID    string            `json:"account_id,omitempty"`
	TenantID     string            `json:"tenant_id,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	RawData      json.RawMessage   `json:"raw_data,omitempty"`
	DiscoveredAt time.Time         `json:"discovered_at"`
	ValidFrom    time.Time         `json:"valid_from,omitempty"`
	ValidTo      *time.Time        `json:"valid_to,omitempty"`
}

func (r *Resource) Key() string {
	return fmt.Sprintf("%s:%s:%s", r.Provider, r.Type, r.ID)
}

type Relationship struct {
	SourceID   string            `json:"source_id"`
	TargetID   string            `json:"target_id"`
	Type       string            `json:"type"`
	Properties map[string]string `json:"properties,omitempty"`
	ValidFrom  time.Time         `json:"valid_from,omitempty"`
	ValidTo    *time.Time        `json:"valid_to,omitempty"`
}

func (r *Relationship) Key() string {
	return fmt.Sprintf("%s-%s-%s", r.SourceID, r.Type, r.TargetID)
}

const (
	RelContains    = "CONTAINS"
	RelConnectsTo  = "CONNECTS_TO"
	RelDependsOn   = "DEPENDS_ON"
	RelHasRole     = "HAS_ROLE"
	RelHasPolicy   = "HAS_POLICY"
	RelExposes     = "EXPOSES"
	RelRoutesTo    = "ROUTES_TO"
	RelPeeredWith  = "PEERED_WITH"
	RelRunsOn      = "RUNS_ON"
	RelBackedBy    = "BACKED_BY"
	RelCalls       = "CALLS"
	RelImports     = "IMPORTS"
	RelExtends     = "EXTENDS"
	RelImplements  = "IMPLEMENTS"
	RelManagedBy   = "MANAGED_BY"
)
