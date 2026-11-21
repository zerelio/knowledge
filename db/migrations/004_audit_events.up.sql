CREATE TABLE IF NOT EXISTS audit_events (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT        NOT NULL,
    user_id     TEXT,
    action      TEXT        NOT NULL,
    resource_id TEXT,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS audit_events_tenant     ON audit_events (tenant_id);
CREATE INDEX IF NOT EXISTS audit_events_created_at ON audit_events (tenant_id, created_at DESC);
