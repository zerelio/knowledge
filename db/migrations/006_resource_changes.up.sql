BEGIN;
CREATE TABLE IF NOT EXISTS resource_changes (
    id              TEXT PRIMARY KEY,
    scope_id        TEXT NOT NULL,
    run_id          TEXT NOT NULL,
    resource_id     TEXT NOT NULL,
    tenant_id       TEXT NOT NULL DEFAULT '',
    change_type     TEXT NOT NULL,
    before_props    JSONB,
    after_props     JSONB,
    detected_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS changes_resource_idx ON resource_changes (resource_id);
CREATE INDEX IF NOT EXISTS changes_run_idx      ON resource_changes (run_id);
CREATE INDEX IF NOT EXISTS changes_scope_idx    ON resource_changes (scope_id);
CREATE INDEX IF NOT EXISTS changes_tenant_idx   ON resource_changes (tenant_id);
COMMIT;
