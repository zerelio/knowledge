package storage

import (
	"context"
	"time"

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
	// Cursor-based pagination fields (keyset).  When AfterDiscoveredAt is set,
	// Offset is ignored and the backend returns resources discovered strictly
	// before this timestamp (or at the same timestamp with a lexicographically
	// smaller ID), ordered by (discovered_at DESC, id DESC).
	AfterDiscoveredAt *time.Time
	AfterID           string
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
	GetRun(ctx context.Context, id string) (*types.DiscoveryRun, error)
	ListRuns(ctx context.Context, scopeID string, limit int) ([]*types.DiscoveryRun, error)
	DeleteRun(ctx context.Context, id string) error

	GetUserByEmail(ctx context.Context, email string) (*types.User, error)
	GetUserByID(ctx context.Context, id string) (*types.User, error)
	ListUsers(ctx context.Context, tenantID string) ([]*types.User, error)
	CreateUser(ctx context.Context, user *types.User) error
	UpdateUser(ctx context.Context, user *types.User) error
	DeleteUser(ctx context.Context, id string) error
	CountUsers(ctx context.Context, tenantID string) (int, error)

	StoreRefreshToken(ctx context.Context, jti, userID string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, jti string) (*types.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, jti string) error
	RevokeUserRefreshTokens(ctx context.Context, userID string) error

	GetConfig(ctx context.Context, key string) (string, error)
	SetConfig(ctx context.Context, key, value string) error

	LogAuditEvent(ctx context.Context, event *types.AuditEvent) error
	ListAuditEvents(ctx context.Context, tenantID string, limit int) ([]*types.AuditEvent, error)

	// Generation logs and feedback (AI output auditability)
	LogGeneration(ctx context.Context, log *types.GenerationLog) error
	GetGeneration(ctx context.Context, id string) (*types.GenerationLog, error)
	ListGenerations(ctx context.Context, tenantID, resourceID string, limit int) ([]*types.GenerationLog, error)
	LogGenerationFeedback(ctx context.Context, fb *types.GenerationFeedback) error
	ListGenerationFeedback(ctx context.Context, generationID string) ([]*types.GenerationFeedback, error)
	GetApprovedExamples(ctx context.Context, genType types.GenerationType, hint string, limit int) ([]*types.GenerationLog, error)
	GetQualityMetrics(ctx context.Context, tenantID string) (*types.QualityMetrics, error)

	// Document cache
	UpsertDocument(ctx context.Context, doc *types.CachedDocument) error
	GetDocument(ctx context.Context, resourceID string) (*types.CachedDocument, error)
	UpdateDocumentStatus(ctx context.Context, id string, status types.DocumentStatus, approvedBy string) error

	ExportGraph(ctx context.Context, tenantID string) ([]*types.Resource, []*types.Relationship, error)

	// Change detection
	RecordChange(ctx context.Context, c *types.ResourceChange) error
	ListChanges(ctx context.Context, scopeID, tenantID string, limit int) ([]*types.ResourceChange, error)

	// Webhook subscriptions
	CreateWebhook(ctx context.Context, sub *types.WebhookSubscription) error
	ListWebhooks(ctx context.Context, tenantID string) ([]*types.WebhookSubscription, error)
	DeleteWebhook(ctx context.Context, id string) error
	ListWebhooksForEvent(ctx context.Context, tenantID string, eventType types.EventType) ([]*types.WebhookSubscription, error)

	// Compliance findings
	StoreFinding(ctx context.Context, f *types.ComplianceFinding) error
	ListFindings(ctx context.Context, tenantID, scopeID string, severity types.FindingSeverity, status types.FindingStatus, limit int) ([]*types.ComplianceFinding, error)
	UpdateFindingStatus(ctx context.Context, id string, status types.FindingStatus) error

	// Community-level knowledge summaries
	StoreKnowledgeSummary(ctx context.Context, s *types.KnowledgeSummary) error
	ListKnowledgeSummaries(ctx context.Context, tenantID, scopeID string) ([]*types.KnowledgeSummary, error)
}
