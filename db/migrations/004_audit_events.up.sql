-- Zerelio Knowledge — Audit events table
-- Stores all user-initiated mutations for compliance and intrusion detection.

BEGIN;

CREATE TABLE IF NOT EXISTS audit_events (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL DEFAULT '',
    tenant_id   TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    ip          TEXT NOT NULL DEFAULT '',
    status_code INT  NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_user_idx   ON audit_events (user_id);
CREATE INDEX IF NOT EXISTS audit_tenant_idx ON audit_events (tenant_id);
CREATE INDEX IF NOT EXISTS audit_time_idx   ON audit_events (created_at DESC);

COMMIT;
