package storage

import (
	"context"

	"github.com/zerelio/knowledge/pkg/types"
)

type ResourceFilter struct {
	Type     string
	Provider string
	Region   string
	TenantID string
	Tags     []string
	Query    string
	Limit    int
	Offset   int
}

type RelationshipFilter struct {
	SourceID string
	TargetID string
	Type     string
	TenantID string
	Limit    int
}

type Backend interface {
	Connect(ctx context.Context) error
	Close() error
	Ping(ctx context.Context) error

	StoreResources(ctx context.Context, resources []*types.Resource) error
	GetResources(ctx context.Context, filter ResourceFilter) ([]*types.Resource, int, error)
	GetResourceByID(ctx context.Context, id string) (*types.Resource, error)
	DeleteResources(ctx context.Context, ids []string) error

	StoreRelationships(ctx context.Context, rels []*types.Relationship) error
	GetRelationships(ctx context.Context, filter RelationshipFilter) ([]*types.Relationship, error)

	CreateScope(ctx context.Context, scope *types.DiscoveryScope) error
	ListScopes(ctx context.Context, tenantID string) ([]*types.DiscoveryScope, error)
	GetScope(ctx context.Context, id string) (*types.DiscoveryScope, error)
	UpdateScope(ctx context.Context, scope *types.DiscoveryScope) error
	DeleteScope(ctx context.Context, id string) error

	StoreRun(ctx context.Context, run *types.DiscoveryRun) error
	ListRuns(ctx context.Context, scopeID string, limit int) ([]*types.DiscoveryRun, error)
	DeleteRun(ctx context.Context, id string) error

	GetUserByEmail(ctx context.Context, email string) (*types.User, error)
	ListUsers(ctx context.Context, tenantID string) ([]*types.User, error)
	CreateUser(ctx context.Context, user *types.User) error
	UpdateUser(ctx context.Context, user *types.User) error
	DeleteUser(ctx context.Context, id string) error
	CountUsers(ctx context.Context, tenantID string) (int, error)

	GetConfig(ctx context.Context, key string) (string, error)
	SetConfig(ctx context.Context, key, value string) error

	ExportGraph(ctx context.Context, tenantID string) ([]*types.Resource, []*types.Relationship, error)
}
