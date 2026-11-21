package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zerelio/knowledge/pkg/types"
)

type MemoryBackend struct {
	mu              sync.RWMutex
	resources       map[string]*types.Resource
	relationships   map[string]*types.Relationship
	scopes          map[string]*types.DiscoveryScope
	runs            map[string]*types.DiscoveryRun
	users           map[string]*types.User
	refreshTokens   map[string]*types.RefreshToken
	auditEvents     []*types.AuditEvent
	generations     map[string]*types.GenerationLog
	genFeedback     map[string][]*types.GenerationFeedback
	documents       map[string]*types.CachedDocument // keyed by resource_id
	config          map[string]string
	changes         []*types.ResourceChange
	webhooks        map[string]*types.WebhookSubscription
	findings        []*types.ComplianceFinding
	knowledgeSummaries map[string]*types.KnowledgeSummary
}

func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		resources:     make(map[string]*types.Resource),
		relationships: make(map[string]*types.Relationship),
		scopes:        make(map[string]*types.DiscoveryScope),
		runs:          make(map[string]*types.DiscoveryRun),
		users:         make(map[string]*types.User),
		refreshTokens: make(map[string]*types.RefreshToken),
		generations:   make(map[string]*types.GenerationLog),
		genFeedback:   make(map[string][]*types.GenerationFeedback),
		documents:     make(map[string]*types.CachedDocument),
		config:        make(map[string]string),
		webhooks:      make(map[string]*types.WebhookSubscription),
	}
}

func (m *MemoryBackend) Connect(_ context.Context) error { return nil }
func (m *MemoryBackend) Close() error                    { return nil }
func (m *MemoryBackend) Ping(_ context.Context) error    { return nil }

func (m *MemoryBackend) StoreResources(_ context.Context, resources []*types.Resource) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range resources {
		m.resources[r.ID] = r
	}
	return nil
}

func (m *MemoryBackend) GetResources(_ context.Context, f ResourceFilter) ([]*types.Resource, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*types.Resource
	for _, r := range m.resources {
		if f.Type != "" && r.Type != f.Type {
			continue
		}
		if f.Provider != "" && r.Provider != f.Provider {
			continue
		}
		if f.Region != "" && r.Region != f.Region {
			continue
		}
		if f.TenantID != "" && r.TenantID != f.TenantID {
			continue
		}
		if f.Query != "" && !strings.Contains(strings.ToLower(r.Name), strings.ToLower(f.Query)) {
			continue
		}
		result = append(result, r)
	}

	// Always sort (discovered_at DESC, id DESC) for deterministic cursor pagination.
	sort.Slice(result, func(i, j int) bool {
		if result[i].DiscoveredAt.Equal(result[j].DiscoveredAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].DiscoveredAt.After(result[j].DiscoveredAt)
	})

	total := len(result)

	if f.AfterDiscoveredAt != nil {
		// Keyset filter: keep only items that come after the cursor position.
		var filtered []*types.Resource
		for _, r := range result {
			if r.DiscoveredAt.Before(*f.AfterDiscoveredAt) ||
				(r.DiscoveredAt.Equal(*f.AfterDiscoveredAt) && r.ID < f.AfterID) {
				filtered = append(filtered, r)
			}
		}
		result = filtered
	} else {
		// Legacy offset-based pagination.
		if f.Offset > 0 && f.Offset < len(result) {
			result = result[f.Offset:]
		} else if f.Offset >= len(result) {
			result = nil
		}
	}
	if f.Limit > 0 && f.Limit < len(result) {
		result = result[:f.Limit]
	}

	return result, total, nil
}

func (m *MemoryBackend) GetResourceByID(_ context.Context, id string) (*types.Resource, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.resources[id]
	if !ok {
		return nil, fmt.Errorf("resource %s not found", id)
	}
	return r, nil
}

func (m *MemoryBackend) DeleteResources(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.resources, id)
		for k, rel := range m.relationships {
			if rel.SourceID == id || rel.TargetID == id {
				delete(m.relationships, k)
			}
		}
	}
	return nil
}

func (m *MemoryBackend) StoreRelationships(_ context.Context, rels []*types.Relationship) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range rels {
		m.relationships[r.Key()] = r
	}
	return nil
}

func (m *MemoryBackend) GetRelationships(_ context.Context, f RelationshipFilter) ([]*types.Relationship, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*types.Relationship
	for _, r := range m.relationships {
		if f.SourceID != "" && r.SourceID != f.SourceID {
			continue
		}
		if f.TargetID != "" && r.TargetID != f.TargetID {
			continue
		}
		if f.Type != "" && r.Type != f.Type {
			continue
		}
		result = append(result, r)
	}

	if f.Limit > 0 && f.Limit < len(result) {
		result = result[:f.Limit]
	}

	return result, nil
}

func (m *MemoryBackend) CreateScope(_ context.Context, scope *types.DiscoveryScope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scopes[scope.ID] = scope
	return nil
}

func (m *MemoryBackend) ListScopes(_ context.Context, tenantID string) ([]*types.DiscoveryScope, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*types.DiscoveryScope
	for _, s := range m.scopes {
		if tenantID != "" && s.TenantID != tenantID {
			continue
		}
		result = append(result, s)
	}
	return result, nil
}

func (m *MemoryBackend) GetScope(_ context.Context, id string) (*types.DiscoveryScope, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.scopes[id]
	if !ok {
		return nil, fmt.Errorf("scope %s not found", id)
	}
	return s, nil
}

func (m *MemoryBackend) UpdateScope(_ context.Context, scope *types.DiscoveryScope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scopes[scope.ID] = scope
	return nil
}

func (m *MemoryBackend) DeleteScope(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.scopes, id)
	return nil
}

func (m *MemoryBackend) StoreRun(_ context.Context, run *types.DiscoveryRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := *run
	m.runs[run.ID] = &stored
	return nil
}

func (m *MemoryBackend) GetRun(_ context.Context, id string) (*types.DiscoveryRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.runs[id]
	if !ok {
		return nil, fmt.Errorf("run %q not found", id)
	}
	cp := *r
	return &cp, nil
}

func (m *MemoryBackend) ListRuns(_ context.Context, scopeID string, limit int) ([]*types.DiscoveryRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*types.DiscoveryRun
	for _, r := range m.runs {
		if scopeID != "" && r.ScopeID != scopeID {
			continue
		}
		result = append(result, r)
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *MemoryBackend) DeleteRun(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.runs, id)
	return nil
}

func (m *MemoryBackend) GetUserByEmail(_ context.Context, email string) (*types.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, fmt.Errorf("user %s not found", email)
}

func (m *MemoryBackend) GetUserByID(_ context.Context, id string) (*types.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return nil, fmt.Errorf("user %s not found", id)
	}
	return u, nil
}

func (m *MemoryBackend) ListUsers(_ context.Context, tenantID string) ([]*types.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*types.User
	for _, u := range m.users {
		if tenantID != "" && u.TenantID != tenantID {
			continue
		}
		result = append(result, u)
	}
	return result, nil
}

func (m *MemoryBackend) CreateUser(_ context.Context, user *types.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == user.Email {
			return fmt.Errorf("user %s already exists", user.Email)
		}
	}
	m.users[user.ID] = user
	return nil
}

func (m *MemoryBackend) UpdateUser(_ context.Context, user *types.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[user.ID] = user
	return nil
}

func (m *MemoryBackend) DeleteUser(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.users, id)
	return nil
}

func (m *MemoryBackend) CountUsers(_ context.Context, tenantID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, u := range m.users {
		if tenantID == "" || u.TenantID == tenantID {
			count++
		}
	}
	return count, nil
}

func (m *MemoryBackend) GetConfig(_ context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.config[key]
	if !ok {
		return "", fmt.Errorf("config key %s not found", key)
	}
	return v, nil
}

func (m *MemoryBackend) SetConfig(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config[key] = value
	return nil
}

func (m *MemoryBackend) ExportGraph(_ context.Context, tenantID string) ([]*types.Resource, []*types.Relationship, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var resources []*types.Resource
	for _, r := range m.resources {
		if tenantID != "" && r.TenantID != tenantID {
			continue
		}
		resources = append(resources, r)
	}

	var rels []*types.Relationship
	for _, r := range m.relationships {
		rels = append(rels, r)
	}

	return resources, rels, nil
}

// ---- Refresh tokens ----

func (m *MemoryBackend) StoreRefreshToken(_ context.Context, jti, userID string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshTokens[jti] = &types.RefreshToken{
		JTI:       jti,
		UserID:    userID,
		IssuedAt:  time.Now(),
		ExpiresAt: expiresAt,
		Revoked:   false,
	}
	return nil
}

func (m *MemoryBackend) GetRefreshToken(_ context.Context, jti string) (*types.RefreshToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rt, ok := m.refreshTokens[jti]
	if !ok {
		return nil, fmt.Errorf("refresh token %s not found", jti)
	}
	return rt, nil
}

func (m *MemoryBackend) RevokeRefreshToken(_ context.Context, jti string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rt, ok := m.refreshTokens[jti]
	if !ok {
		return fmt.Errorf("refresh token %s not found", jti)
	}
	rt.Revoked = true
	return nil
}

func (m *MemoryBackend) RevokeUserRefreshTokens(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rt := range m.refreshTokens {
		if rt.UserID == userID {
			rt.Revoked = true
		}
	}
	return nil
}

// ---- Audit events ----

func (m *MemoryBackend) LogAuditEvent(_ context.Context, event *types.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.auditEvents == nil {
		m.auditEvents = make([]*types.AuditEvent, 0)
	}
	m.auditEvents = append(m.auditEvents, event)
	return nil
}

func (m *MemoryBackend) ListAuditEvents(_ context.Context, tenantID string, limit int) ([]*types.AuditEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	var result []*types.AuditEvent
	for i := len(m.auditEvents) - 1; i >= 0 && len(result) < limit; i-- {
		e := m.auditEvents[i]
		if tenantID == "" || e.TenantID == tenantID {
			result = append(result, e)
		}
	}
	return result, nil
}

// ---- Generation logs & feedback ----

func (m *MemoryBackend) LogGeneration(_ context.Context, log *types.GenerationLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.generations[log.ID] = log
	return nil
}

func (m *MemoryBackend) GetGeneration(_ context.Context, id string) (*types.GenerationLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.generations[id]
	if !ok {
		return nil, fmt.Errorf("generation %s not found", id)
	}
	return g, nil
}

func (m *MemoryBackend) ListGenerations(_ context.Context, tenantID, resourceID string, limit int) ([]*types.GenerationLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	var result []*types.GenerationLog
	for _, g := range m.generations {
		if tenantID != "" && g.TenantID != tenantID {
			continue
		}
		if resourceID != "" && g.ResourceID != resourceID {
			continue
		}
		result = append(result, g)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *MemoryBackend) LogGenerationFeedback(_ context.Context, fb *types.GenerationFeedback) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.genFeedback[fb.GenerationID] = append(m.genFeedback[fb.GenerationID], fb)
	return nil
}

func (m *MemoryBackend) ListGenerationFeedback(_ context.Context, generationID string) ([]*types.GenerationFeedback, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.genFeedback[generationID], nil
}

func (m *MemoryBackend) GetApprovedExamples(_ context.Context, genType types.GenerationType, _ string, limit int) ([]*types.GenerationLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 3
	}
	var result []*types.GenerationLog
	for id, fbs := range m.genFeedback {
		for _, fb := range fbs {
			if fb.Rating >= 1 {
				if g, ok := m.generations[id]; ok && g.Type == genType {
					result = append(result, g)
					break
				}
			}
		}
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *MemoryBackend) GetQualityMetrics(_ context.Context, tenantID string) (*types.QualityMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	metrics := &types.QualityMetrics{
		ByType:    make(map[string]int),
		ByAnomaly: make(map[string]int),
	}
	for _, g := range m.generations {
		if tenantID != "" && g.TenantID != tenantID {
			continue
		}
		metrics.TotalGenerations++
		metrics.ByType[string(g.Type)]++
	}
	for _, fbs := range m.genFeedback {
		for _, fb := range fbs {
			metrics.WithFeedback++
			if fb.Rating > 0 {
				metrics.ApprovalRate++
			} else if fb.Rating < 0 {
				metrics.RejectionRate++
			}
			if fb.AnomalyType != "" {
				metrics.ByAnomaly[string(fb.AnomalyType)]++
				if fb.AnomalyType == types.AnomalyHallucination {
					metrics.HallucinationRate++
				}
			}
		}
	}
	if metrics.WithFeedback > 0 {
		f := float64(metrics.WithFeedback)
		metrics.ApprovalRate /= f
		metrics.RejectionRate /= f
		metrics.HallucinationRate /= f
	}
	return metrics, nil
}

// ---- Document cache ----

func (m *MemoryBackend) UpsertDocument(_ context.Context, doc *types.CachedDocument) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.documents[doc.ResourceID] = doc
	return nil
}

func (m *MemoryBackend) GetDocument(_ context.Context, resourceID string) (*types.CachedDocument, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	doc, ok := m.documents[resourceID]
	if !ok {
		return nil, fmt.Errorf("document for resource %s not found", resourceID)
	}
	return doc, nil
}

func (m *MemoryBackend) UpdateDocumentStatus(_ context.Context, id string, status types.DocumentStatus, approvedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, doc := range m.documents {
		if doc.ID == id {
			doc.Status = status
			doc.ApprovedBy = approvedBy
			if status == types.DocumentApproved {
				now := time.Now()
				doc.ApprovedAt = &now
			}
			return nil
		}
	}
	return fmt.Errorf("document %s not found", id)
}

// ---- Change detection ----

func (m *MemoryBackend) RecordChange(_ context.Context, c *types.ResourceChange) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changes = append(m.changes, c)
	return nil
}

func (m *MemoryBackend) ListChanges(_ context.Context, scopeID, tenantID string, limit int) ([]*types.ResourceChange, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	var result []*types.ResourceChange
	for i := len(m.changes) - 1; i >= 0 && len(result) < limit; i-- {
		c := m.changes[i]
		if scopeID != "" && c.ScopeID != scopeID {
			continue
		}
		if tenantID != "" && c.TenantID != tenantID {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}

// ---- Webhook subscriptions ----

func (m *MemoryBackend) CreateWebhook(_ context.Context, sub *types.WebhookSubscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhooks[sub.ID] = sub
	return nil
}

func (m *MemoryBackend) ListWebhooks(_ context.Context, tenantID string) ([]*types.WebhookSubscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*types.WebhookSubscription
	for _, w := range m.webhooks {
		if tenantID != "" && w.TenantID != tenantID {
			continue
		}
		result = append(result, w)
	}
	return result, nil
}

func (m *MemoryBackend) DeleteWebhook(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.webhooks, id)
	return nil
}

func (m *MemoryBackend) ListWebhooksForEvent(_ context.Context, tenantID string, eventType types.EventType) ([]*types.WebhookSubscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*types.WebhookSubscription
	for _, w := range m.webhooks {
		if !w.Active {
			continue
		}
		if tenantID != "" && w.TenantID != tenantID {
			continue
		}
		for _, e := range w.Events {
			if e == string(eventType) || e == "*" {
				result = append(result, w)
				break
			}
		}
	}
	return result, nil
}
func (m *MemoryBackend) StoreFinding(_ context.Context, f *types.ComplianceFinding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Replace existing finding with same ID.
	for i, existing := range m.findings {
		if existing.ID == f.ID {
			cp := *f
			m.findings[i] = &cp
			return nil
		}
	}
	cp := *f
	m.findings = append(m.findings, &cp)
	return nil
}

func (m *MemoryBackend) ListFindings(_ context.Context, tenantID, scopeID string, severity types.FindingSeverity, status types.FindingStatus, limit int) ([]*types.ComplianceFinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*types.ComplianceFinding
	for _, f := range m.findings {
		if tenantID != "" && f.TenantID != tenantID {
			continue
		}
		if scopeID != "" && f.ScopeID != scopeID {
			continue
		}
		if severity != "" && f.Severity != severity {
			continue
		}
		if status != "" && f.Status != status {
			continue
		}
		cp := *f
		result = append(result, &cp)
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *MemoryBackend) UpdateFindingStatus(_ context.Context, id string, status types.FindingStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.findings {
		if f.ID == id {
			f.Status = status
			return nil
		}
	}
	return fmt.Errorf("finding %q not found", id)
}

func (m *MemoryBackend) StoreKnowledgeSummary(_ context.Context, s *types.KnowledgeSummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// summaries field initialised lazily
	if m.knowledgeSummaries == nil {
		m.knowledgeSummaries = make(map[string]*types.KnowledgeSummary)
	}
	cp := *s
	m.knowledgeSummaries[s.ID] = &cp
	return nil
}

func (m *MemoryBackend) ListKnowledgeSummaries(_ context.Context, tenantID, scopeID string) ([]*types.KnowledgeSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*types.KnowledgeSummary
	for _, s := range m.knowledgeSummaries {
		if tenantID != "" && s.TenantID != tenantID {
			continue
		}
		if scopeID != "" && s.ScopeID != scopeID {
			continue
		}
		cp := *s
		result = append(result, &cp)
	}
	return result, nil
}
