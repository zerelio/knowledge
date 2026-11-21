-- compliance_findings stores policy evaluation results per resource.
CREATE TABLE IF NOT EXISTS compliance_findings (
    id              TEXT PRIMARY KEY,
    resource_id     TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    scope_id        TEXT        NOT NULL,
    rule_id         TEXT        NOT NULL,
    rule_name       TEXT        NOT NULL,
    severity        TEXT        NOT NULL,  -- critical | high | medium | low | info
    message         TEXT        NOT NULL,
    remediation     TEXT,
    status          TEXT        NOT NULL DEFAULT 'open',  -- open | suppressed | resolved
    evaluated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS compliance_findings_tenant ON compliance_findings (tenant_id);
CREATE INDEX IF NOT EXISTS compliance_findings_scope  ON compliance_findings (tenant_id, scope_id);
CREATE INDEX IF NOT EXISTS compliance_findings_res    ON compliance_findings (resource_id);
CREATE INDEX IF NOT EXISTS compliance_findings_sev    ON compliance_findings (tenant_id, severity);
