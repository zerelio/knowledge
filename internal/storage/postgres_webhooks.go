package storage

import (
	"context"
	"fmt"

	"github.com/zerelio/zerelio/pkg/types"
)

// ---- Webhook subscriptions ----

func (p *PGBackend) CreateWebhook(ctx context.Context, sub *types.WebhookSubscription) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO webhook_subscriptions (id, tenant_id, url, secret, events, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO NOTHING`,
		sub.ID, sub.TenantID, sub.URL, sub.Secret, sub.Events, sub.Active, sub.CreatedAt, sub.CreatedAt,
	)
	return err
}

func (p *PGBackend) ListWebhooks(ctx context.Context, tenantID string) ([]*types.WebhookSubscription, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, tenant_id, url, secret, events, active, created_at
		FROM webhook_subscriptions
		WHERE tenant_id = $1
		ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*types.WebhookSubscription
	for rows.Next() {
		w := &types.WebhookSubscription{}
		if err := rows.Scan(&w.ID, &w.TenantID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func (p *PGBackend) DeleteWebhook(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM webhook_subscriptions WHERE id = $1`, id)
	return err
}

func (p *PGBackend) ListWebhooksForEvent(ctx context.Context, tenantID string, eventType types.EventType) ([]*types.WebhookSubscription, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, tenant_id, url, secret, events, active, created_at
		FROM webhook_subscriptions
		WHERE tenant_id = $1
		  AND active = TRUE
		  AND (events @> ARRAY[$2]::TEXT[] OR events @> ARRAY['*']::TEXT[])
		ORDER BY created_at`, tenantID, string(eventType))
	if err != nil {
		return nil, fmt.Errorf("list webhooks for event: %w", err)
	}
	defer rows.Close()
	var result []*types.WebhookSubscription
	for rows.Next() {
		w := &types.WebhookSubscription{}
		if err := rows.Scan(&w.ID, &w.TenantID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}
