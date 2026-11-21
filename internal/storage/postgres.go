package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerelio/zerelio/pkg/types"
)

// PGBackend is the production PostgreSQL storage backend.
// It requires PostgreSQL with the Apache AGE and pgvector extensions.
// Schema migration is handled externally (e.g., via golang-migrate).
type PGBackend struct {
	pool *pgxpool.Pool
	dsn  string
}

func NewPGBackend(dsn string) *PGBackend {
	return &PGBackend{dsn: dsn}
}

func (p *PGBackend) Connect(ctx context.Context) error {
	cfg, err := pgxpool.ParseConfig(p.dsn)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}

	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("ping: %w", err)
	}

	p.pool = pool
	return nil
}

func (p *PGBackend) Close() error {
	if p.pool != nil {
		p.pool.Close()
	}
	return nil
}

func (p *PGBackend) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// ---- Resources ----

func (p *PGBackend) StoreResources(ctx context.Context, resources []*types.Resource) error {
	if len(resources) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, r := range resources {
		propsJSON, _ := json.Marshal(r.Properties)
		rawData := r.RawData
		if rawData == nil {
			rawData = json.RawMessage("{}")
		}
		batch.Queue(`
			INSERT INTO resources (
				id, tenant_id, name, type, provider, region, account_id,
				tags, properties, raw_data, discovered_at, valid_from, valid_to
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (id) DO UPDATE SET
				name=EXCLUDED.name, type=EXCLUDED.type, provider=EXCLUDED.provider,
				region=EXCLUDED.region, account_id=EXCLUDED.account_id,
				tags=EXCLUDED.tags, properties=EXCLUDED.properties, raw_data=EXCLUDED.raw_data,
				discovered_at=EXCLUDED.discovered_at,
				valid_from=EXCLUDED.valid_from, valid_to=EXCLUDED.valid_to,
				updated_at=now()`,
			r.ID, r.TenantID, r.Name, r.Type, r.Provider, r.Region, r.AccountID,
			r.Tags, propsJSON, rawData, r.DiscoveredAt, r.ValidFrom, r.ValidTo,
		)
	}

	br := p.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range resources {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("store resource: %w", err)
		}
	}
	return br.Close()
}

func (p *PGBackend) GetResources(ctx context.Context, f ResourceFilter) ([]*types.Resource, int, error) {
	var conditions []string
	var args []any
	i := 1

	addCond := func(cond string, val any) {
		conditions = append(conditions, fmt.Sprintf(cond, i))
		args = append(args, val)
		i++
	}

	if f.TenantID != "" {
		addCond("tenant_id = $%d", f.TenantID)
	}
	if f.Type != "" {
		addCond("type = $%d", f.Type)
	}
	if f.Provider != "" {
		addCond("provider = $%d", f.Provider)
	}
	if f.Region != "" {
		addCond("region = $%d", f.Region)
	}
	if f.Query != "" {
		addCond("name ILIKE $%d", "%"+f.Query+"%")
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM resources " + where
	var total int
	if err := p.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count resources: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit, f.Offset)

	query := fmt.Sprintf(`
		SELECT id, tenant_id, name, type, provider, region, account_id,
		       tags, properties, raw_data, discovered_at, valid_from, valid_to
		FROM resources %s
		ORDER BY discovered_at DESC
		LIMIT $%d OFFSET $%d`, where, i, i+1)

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query resources: %w", err)
	}
	defer rows.Close()

	var results []*types.Resource
	for rows.Next() {
		r := &types.Resource{}
		var propsJSON []byte
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Provider, &r.Region, &r.AccountID,
			&r.Tags, &propsJSON, &r.RawData, &r.DiscoveredAt, &r.ValidFrom, &r.ValidTo,
		); err != nil {
			return nil, 0, fmt.Errorf("scan resource: %w", err)
		}
		if propsJSON != nil {
			json.Unmarshal(propsJSON, &r.Properties) //nolint:errcheck
		}
		results = append(results, r)
	}
	return results, total, rows.Err()
}

func (p *PGBackend) GetResourceByID(ctx context.Context, id string) (*types.Resource, error) {
	r := &types.Resource{}
	var propsJSON []byte
	err := p.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, type, provider, region, account_id,
		       tags, properties, raw_data, discovered_at, valid_from, valid_to
		FROM resources WHERE id = $1`, id).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.Type, &r.Provider, &r.Region, &r.AccountID,
		&r.Tags, &propsJSON, &r.RawData, &r.DiscoveredAt, &r.ValidFrom, &r.ValidTo,
	)
	if err != nil {
		return nil, fmt.Errorf("get resource %s: %w", id, err)
	}
	if propsJSON != nil {
		json.Unmarshal(propsJSON, &r.Properties) //nolint:errcheck
	}
	return r, nil
}

func (p *PGBackend) DeleteResources(ctx context.Context, ids []string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM resources WHERE id = ANY($1)`, ids)
	return err
}

// ---- Relationships ----

func (p *PGBackend) StoreRelationships(ctx context.Context, rels []*types.Relationship) error {
	if len(rels) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, r := range rels {
		propsJSON, _ := json.Marshal(r.Properties)
		batch.Queue(`
			INSERT INTO relationships (source_id, target_id, type, properties, valid_from, valid_to)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (source_id, target_id, type) DO UPDATE SET
				properties=EXCLUDED.properties,
				valid_from=EXCLUDED.valid_from,
				valid_to=EXCLUDED.valid_to,
				updated_at=now()`,
			r.SourceID, r.TargetID, r.Type, propsJSON, r.ValidFrom, r.ValidTo,
		)
	}

	br := p.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range rels {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("store relationship: %w", err)
		}
	}
	return br.Close()
}

func (p *PGBackend) GetRelationships(ctx context.Context, f RelationshipFilter) ([]*types.Relationship, error) {
	var conditions []string
	var args []any
	i := 1

	addCond := func(cond string, val any) {
		conditions = append(conditions, fmt.Sprintf(cond, i))
		args = append(args, val)
		i++
	}

	if f.SourceID != "" {
		addCond("source_id = $%d", f.SourceID)
	}
	if f.TargetID != "" {
		addCond("target_id = $%d", f.TargetID)
	}
	if f.Type != "" {
		addCond("type = $%d", f.Type)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 1000
	}
	args = append(args, limit)

	rows, err := p.pool.Query(ctx, fmt.Sprintf(`
		SELECT source_id, target_id, type, properties, valid_from, valid_to
		FROM relationships %s
		ORDER BY source_id, target_id
		LIMIT $%d`, where, i), args...)
	if err != nil {
		return nil, fmt.Errorf("query relationships: %w", err)
	}
	defer rows.Close()

	var results []*types.Relationship
	for rows.Next() {
		r := &types.Relationship{}
		var propsJSON []byte
		if err := rows.Scan(&r.SourceID, &r.TargetID, &r.Type, &propsJSON, &r.ValidFrom, &r.ValidTo); err != nil {
			return nil, fmt.Errorf("scan relationship: %w", err)
		}
		if propsJSON != nil {
			json.Unmarshal(propsJSON, &r.Properties) //nolint:errcheck
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ---- Scopes ----

func (p *PGBackend) CreateScope(ctx context.Context, scope *types.DiscoveryScope) error {
	configJSON, _ := json.Marshal(scope.Config)
	credJSON, _ := json.Marshal(scope.CredentialSource)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO discovery_scopes (id, tenant_id, name, plugin_name, config, credential_source, schedule, status, resource_count)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		scope.ID, scope.TenantID, scope.Name, scope.PluginName,
		configJSON, credJSON, scope.Schedule, scope.Status, scope.ResourceCount,
	)
	return err
}

func (p *PGBackend) ListScopes(ctx context.Context, tenantID string) ([]*types.DiscoveryScope, error) {
	var args []any
	where := ""
	if tenantID != "" {
		where = "WHERE tenant_id = $1"
		args = append(args, tenantID)
	}

	rows, err := p.pool.Query(ctx, `
		SELECT id, tenant_id, name, plugin_name, config, credential_source, schedule, status, last_run, resource_count
		FROM discovery_scopes `+where+` ORDER BY name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*types.DiscoveryScope
	for rows.Next() {
		s := &types.DiscoveryScope{}
		var configJSON, credJSON []byte
		if err := rows.Scan(&s.ID, &s.TenantID, &s.Name, &s.PluginName,
			&configJSON, &credJSON, &s.Schedule, &s.Status, &s.LastRun, &s.ResourceCount); err != nil {
			return nil, err
		}
		if configJSON != nil {
			json.Unmarshal(configJSON, &s.Config) //nolint:errcheck
		}
		if credJSON != nil {
			json.Unmarshal(credJSON, &s.CredentialSource) //nolint:errcheck
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (p *PGBackend) GetScope(ctx context.Context, id string) (*types.DiscoveryScope, error) {
	s := &types.DiscoveryScope{}
	var configJSON, credJSON []byte
	err := p.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, plugin_name, config, credential_source, schedule, status, last_run, resource_count
		FROM discovery_scopes WHERE id = $1`, id).Scan(
		&s.ID, &s.TenantID, &s.Name, &s.PluginName,
		&configJSON, &credJSON, &s.Schedule, &s.Status, &s.LastRun, &s.ResourceCount,
	)
	if err != nil {
		return nil, fmt.Errorf("get scope %s: %w", id, err)
	}
	if configJSON != nil {
		json.Unmarshal(configJSON, &s.Config) //nolint:errcheck
	}
	if credJSON != nil {
		json.Unmarshal(credJSON, &s.CredentialSource) //nolint:errcheck
	}
	return s, nil
}

func (p *PGBackend) UpdateScope(ctx context.Context, scope *types.DiscoveryScope) error {
	configJSON, _ := json.Marshal(scope.Config)
	credJSON, _ := json.Marshal(scope.CredentialSource)
	_, err := p.pool.Exec(ctx, `
		UPDATE discovery_scopes
		SET name=$2, plugin_name=$3, config=$4, credential_source=$5,
		    schedule=$6, status=$7, resource_count=$8, last_run=$9, updated_at=now()
		WHERE id=$1`,
		scope.ID, scope.Name, scope.PluginName, configJSON, credJSON,
		scope.Schedule, scope.Status, scope.ResourceCount, scope.LastRun,
	)
	return err
}

func (p *PGBackend) DeleteScope(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM discovery_scopes WHERE id = $1`, id)
	return err
}

// ---- Runs ----

func (p *PGBackend) StoreRun(ctx context.Context, run *types.DiscoveryRun) error {
	errorsJSON, _ := json.Marshal(run.Errors)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO discovery_runs (id, scope_id, status, started_at, completed_at, resources_found, relationships_found, errors)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET
			status=EXCLUDED.status, completed_at=EXCLUDED.completed_at,
			resources_found=EXCLUDED.resources_found, relationships_found=EXCLUDED.relationships_found,
			errors=EXCLUDED.errors`,
		run.ID, run.ScopeID, run.Status,
		run.StartedAt, run.CompletedAt, run.ResourcesFound, run.RelationshipsFound, errorsJSON,
	)
	return err
}

func (p *PGBackend) ListRuns(ctx context.Context, scopeID string, limit int) ([]*types.DiscoveryRun, error) {
	if limit <= 0 {
		limit = 50
	}
	var args []any
	where := ""
	if scopeID != "" {
		where = "WHERE scope_id = $1"
		args = append(args, scopeID)
	}
	args = append(args, limit)

	rows, err := p.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, scope_id, status, started_at, completed_at, resources_found, relationships_found, errors
		FROM discovery_runs %s ORDER BY started_at DESC LIMIT $%d`, where, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*types.DiscoveryRun
	for rows.Next() {
		r := &types.DiscoveryRun{}
		var errorsJSON []byte
		if err := rows.Scan(&r.ID, &r.ScopeID, &r.Status,
			&r.StartedAt, &r.CompletedAt, &r.ResourcesFound, &r.RelationshipsFound, &errorsJSON); err != nil {
			return nil, err
		}
		if errorsJSON != nil {
			json.Unmarshal(errorsJSON, &r.Errors) //nolint:errcheck
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (p *PGBackend) DeleteRun(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM discovery_runs WHERE id = $1`, id)
	return err
}

// ---- Users ----

func (p *PGBackend) GetUserByEmail(ctx context.Context, email string) (*types.User, error) {
	u := &types.User{}
	err := p.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, roles, tenant_id, created_at, updated_at
		FROM users WHERE email = $1`, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Roles, &u.TenantID, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

func (p *PGBackend) GetUserByID(ctx context.Context, id string) (*types.User, error) {
	u := &types.User{}
	err := p.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, roles, tenant_id, created_at, updated_at
		FROM users WHERE id = $1`, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Roles, &u.TenantID, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

func (p *PGBackend) ListUsers(ctx context.Context, tenantID string) ([]*types.User, error) {
	var args []any
	where := ""
	if tenantID != "" {
		where = "WHERE tenant_id = $1"
		args = append(args, tenantID)
	}

	rows, err := p.pool.Query(ctx, `
		SELECT id, email, password_hash, roles, tenant_id, created_at, updated_at
		FROM users `+where+` ORDER BY email`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*types.User
	for rows.Next() {
		u := &types.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Roles,
			&u.TenantID, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, u)
	}
	return results, rows.Err()
}

func (p *PGBackend) CreateUser(ctx context.Context, user *types.User) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, roles, tenant_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		user.ID, user.Email, user.PasswordHash, user.Roles,
		user.TenantID, user.CreatedAt, user.UpdatedAt,
	)
	return err
}

func (p *PGBackend) UpdateUser(ctx context.Context, user *types.User) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE users SET email=$2, password_hash=$3, roles=$4, tenant_id=$5, updated_at=now()
		WHERE id=$1`,
		user.ID, user.Email, user.PasswordHash, user.Roles, user.TenantID,
	)
	return err
}

func (p *PGBackend) DeleteUser(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

func (p *PGBackend) CountUsers(ctx context.Context, tenantID string) (int, error) {
	var count int
	var err error
	if tenantID != "" {
		err = p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE tenant_id = $1`, tenantID).Scan(&count)
	} else {
		err = p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	}
	return count, err
}

// ---- Config ----

func (p *PGBackend) GetConfig(ctx context.Context, key string) (string, error) {
	var value string
	err := p.pool.QueryRow(ctx, `SELECT value FROM config WHERE key = $1`, key).Scan(&value)
	if err != nil {
		return "", fmt.Errorf("get config %s: %w", key, err)
	}
	return value, nil
}

func (p *PGBackend) SetConfig(ctx context.Context, key, value string) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO config (key, value) VALUES ($1,$2)
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`,
		key, value,
	)
	return err
}

// ---- Graph export ----

func (p *PGBackend) ExportGraph(ctx context.Context, tenantID string) ([]*types.Resource, []*types.Relationship, error) {
	resourceFilter := ResourceFilter{TenantID: tenantID, Limit: 10000}
	resources, _, err := p.GetResources(ctx, resourceFilter)
	if err != nil {
		return nil, nil, err
	}

	rels, err := p.GetRelationships(ctx, RelationshipFilter{Limit: 100000})
	if err != nil {
		return nil, nil, err
	}

	return resources, rels, nil
}

// ---- Refresh tokens ----

func (p *PGBackend) StoreRefreshToken(ctx context.Context, jti, userID string, expiresAt time.Time) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (jti, user_id, expires_at)
		VALUES ($1,$2,$3)
		ON CONFLICT (jti) DO NOTHING`,
		jti, userID, expiresAt,
	)
	return err
}

func (p *PGBackend) GetRefreshToken(ctx context.Context, jti string) (*types.RefreshToken, error) {
	rt := &types.RefreshToken{}
	err := p.pool.QueryRow(ctx, `
		SELECT jti, user_id, issued_at, expires_at, revoked
		FROM refresh_tokens WHERE jti = $1`, jti).Scan(
		&rt.JTI, &rt.UserID, &rt.IssuedAt, &rt.ExpiresAt, &rt.Revoked,
	)
	if err != nil {
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	return rt, nil
}

func (p *PGBackend) RevokeRefreshToken(ctx context.Context, jti string) error {
	_, err := p.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked=TRUE WHERE jti=$1`, jti)
	return err
}

func (p *PGBackend) RevokeUserRefreshTokens(ctx context.Context, userID string) error {
	_, err := p.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked=TRUE WHERE user_id=$1`, userID)
	return err
}
