-- Zerelio Knowledge — Initial Schema
-- Requires PostgreSQL with extensions: uuid-ossp, pgcrypto
-- Apache AGE and pgvector are optional extensions loaded after.

BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ---- Core tables ----

CREATE TABLE IF NOT EXISTS resources (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT '',
    name            TEXT NOT NULL,
    type            TEXT NOT NULL,
    provider        TEXT NOT NULL,
    region          TEXT NOT NULL DEFAULT '',
    account_id      TEXT NOT NULL DEFAULT '',
    tags            TEXT[] NOT NULL DEFAULT '{}',
    properties      JSONB NOT NULL DEFAULT '{}',
    raw_data        JSONB NOT NULL DEFAULT '{}',
    discovered_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to        TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS resources_tenant_idx    ON resources (tenant_id);
CREATE INDEX IF NOT EXISTS resources_type_idx      ON resources (type);
CREATE INDEX IF NOT EXISTS resources_provider_idx  ON resources (provider);
CREATE INDEX IF NOT EXISTS resources_region_idx    ON resources (region);
CREATE INDEX IF NOT EXISTS resources_name_idx      ON resources USING gin (to_tsvector('english', name));
CREATE INDEX IF NOT EXISTS resources_temporal_idx  ON resources (valid_from, valid_to);
CREATE INDEX IF NOT EXISTS resources_props_idx     ON resources USING gin (properties);

CREATE TABLE IF NOT EXISTS relationships (
    source_id   TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    type        TEXT NOT NULL,
    properties  JSONB NOT NULL DEFAULT '{}',
    valid_from  TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to    TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source_id, target_id, type)
);

CREATE INDEX IF NOT EXISTS rels_source_idx ON relationships (source_id);
CREATE INDEX IF NOT EXISTS rels_target_idx ON relationships (target_id);
CREATE INDEX IF NOT EXISTS rels_type_idx   ON relationships (type);

CREATE TABLE IF NOT EXISTS discovery_scopes (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL DEFAULT '',
    name              TEXT NOT NULL,
    plugin_name       TEXT NOT NULL,
    config            JSONB NOT NULL DEFAULT '{}',
    credential_source JSONB,
    schedule          TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'active',
    last_run          TIMESTAMPTZ,
    resource_count    INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS scopes_tenant_idx ON discovery_scopes (tenant_id);

CREATE TABLE IF NOT EXISTS discovery_runs (
    id                  TEXT PRIMARY KEY,
    scope_id            TEXT NOT NULL REFERENCES discovery_scopes(id) ON DELETE CASCADE,
    status              TEXT NOT NULL DEFAULT 'pending',
    started_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at        TIMESTAMPTZ,
    resources_found     INT NOT NULL DEFAULT 0,
    relationships_found INT NOT NULL DEFAULT 0,
    errors              JSONB NOT NULL DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS runs_scope_idx  ON discovery_runs (scope_id);
CREATE INDEX IF NOT EXISTS runs_status_idx ON discovery_runs (status);
CREATE INDEX IF NOT EXISTS runs_start_idx  ON discovery_runs (started_at DESC);

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    roles         TEXT[] NOT NULL DEFAULT '{"viewer"}',
    tenant_id     TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS users_email_idx  ON users (email);
CREATE INDEX IF NOT EXISTS users_tenant_idx ON users (tenant_id);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    jti         TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    issued_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked     BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS rt_user_idx    ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS rt_expires_idx ON refresh_tokens (expires_at);

CREATE TABLE IF NOT EXISTS config (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Row-Level Security for multi-tenancy
ALTER TABLE resources         ENABLE ROW LEVEL SECURITY;
ALTER TABLE relationships     ENABLE ROW LEVEL SECURITY;
ALTER TABLE discovery_scopes  ENABLE ROW LEVEL SECURITY;
ALTER TABLE discovery_runs    ENABLE ROW LEVEL SECURITY;

-- Policies: app role bypasses RLS (runs as privileged user), tenant isolation is enforced at app layer.
-- These policies ensure defense in depth if the DB is accessed directly.
CREATE POLICY resources_tenant_policy ON resources
    USING (tenant_id = current_setting('app.tenant_id', TRUE) OR current_setting('app.tenant_id', TRUE) IS NULL);

CREATE POLICY scopes_tenant_policy ON discovery_scopes
    USING (tenant_id = current_setting('app.tenant_id', TRUE) OR current_setting('app.tenant_id', TRUE) IS NULL);

COMMIT;
