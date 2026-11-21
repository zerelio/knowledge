package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zerelio/knowledge/pkg/types"
)

// RecordChange inserts a resource change record into the resource_changes table.
func (p *PGBackend) RecordChange(ctx context.Context, c *types.ResourceChange) error {
	beforeJSON, _ := json.Marshal(c.BeforeProps)
	afterJSON, _ := json.Marshal(c.AfterProps)

	_, err := p.pool.Exec(ctx, `
		INSERT INTO resource_changes
			(id, scope_id, run_id, resource_id, tenant_id, change_type, before_props, after_props, detected_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO NOTHING`,
		c.ID, c.ScopeID, c.RunID, c.ResourceID, c.TenantID,
		string(c.ChangeType), beforeJSON, afterJSON, c.DetectedAt,
	)
	return err
}

// ListChanges returns resource changes for a scope (and optionally tenant), newest first.
func (p *PGBackend) ListChanges(ctx context.Context, scopeID, tenantID string, limit int) ([]*types.ResourceChange, error) {
	if limit <= 0 {
		limit = 50
	}

	var args []any
	i := 1

	addArg := func(val any) string {
		args = append(args, val)
		s := fmt.Sprintf("$%d", i)
		i++
		return s
	}

	where := "WHERE scope_id = " + addArg(scopeID)
	if tenantID != "" {
		where += " AND tenant_id = " + addArg(tenantID)
	}
	limitPlaceholder := addArg(limit)

	query := fmt.Sprintf(`
		SELECT id, scope_id, run_id, resource_id, tenant_id, change_type,
		       before_props, after_props, detected_at
		FROM resource_changes
		%s
		ORDER BY detected_at DESC
		LIMIT %s`, where, limitPlaceholder)

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list changes: %w", err)
	}
	defer rows.Close()

	var results []*types.ResourceChange
	for rows.Next() {
		c := &types.ResourceChange{}
		var beforeJSON, afterJSON []byte
		var detectedAt time.Time
		if err := rows.Scan(
			&c.ID, &c.ScopeID, &c.RunID, &c.ResourceID, &c.TenantID, &c.ChangeType,
			&beforeJSON, &afterJSON, &detectedAt,
		); err != nil {
			return nil, fmt.Errorf("scan change: %w", err)
		}
		c.DetectedAt = detectedAt
		if beforeJSON != nil {
			json.Unmarshal(beforeJSON, &c.BeforeProps) //nolint:errcheck
		}
		if afterJSON != nil {
			json.Unmarshal(afterJSON, &c.AfterProps) //nolint:errcheck
		}
		results = append(results, c)
	}
	return results, rows.Err()
}
