package types

import "time"

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Roles        []string  `json:"roles"`
	TenantID     string    `json:"tenant_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateUserRequest struct {
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Roles    []string `json:"roles,omitempty"`
}

type UpdateUserRequest struct {
	Email *string  `json:"email,omitempty"`
	Roles []string `json:"roles,omitempty"`
}

const (
	RoleViewer = "viewer"
	RoleEditor = "editor"
	RoleAdmin  = "admin"
)

const (
	PermResourcesRead  = "resources:read"
	PermResourcesWrite = "resources:write"
	PermResourcesDel   = "resources:delete"
	PermGraphRead      = "graph:read"
	PermGraphWrite     = "graph:write"
	PermDiscoveryRead  = "discovery:read"
	PermDiscoveryRun   = "discovery:run"
	PermDocsRead       = "docs:read"
	PermDocsWrite      = "docs:write"
	PermAdminRead      = "admin:read"
	PermAdminWrite     = "admin:write"
	PermAuditRead      = "audit:read"
	PermDataExport     = "data:export"
	PermDataImport     = "data:import"
)

var DefaultRolePermissions = map[string][]string{
	RoleViewer: {
		PermResourcesRead, PermGraphRead, PermDiscoveryRead, PermDocsRead,
	},
	RoleEditor: {
		PermResourcesRead, PermResourcesWrite, PermGraphRead,
		PermDiscoveryRead, PermDiscoveryRun, PermDocsRead, PermDocsWrite,
		PermDataExport,
	},
	RoleAdmin: {
		PermResourcesRead, PermResourcesWrite, PermResourcesDel,
		PermGraphRead, PermGraphWrite,
		PermDiscoveryRead, PermDiscoveryRun,
		PermDocsRead, PermDocsWrite,
		PermAdminRead, PermAdminWrite,
		PermAuditRead, PermDataExport, PermDataImport,
	},
}
