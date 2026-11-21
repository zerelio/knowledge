-- Generation logs: full audit trail for LLM calls (RAG context, outputs, scores).
CREATE TABLE IF NOT EXISTS generation_logs (
    id                TEXT PRIMARY KEY,
    type              TEXT        NOT NULL,
    tenant_id         TEXT,
    user_id           TEXT,
    resource_id       TEXT,
    query             TEXT,
    context_text      TEXT,
    retrieved_ids     TEXT[],
    retrieval_signals JSONB,
    model             TEXT        NOT NULL DEFAULT '',
    provider          TEXT        NOT NULL DEFAULT '',
    tokens_prompt     INT         NOT NULL DEFAULT 0,
    tokens_completion INT         NOT NULL DEFAULT 0,
    latency_ms        INT         NOT NULL DEFAULT 0,
    output            TEXT        NOT NULL DEFAULT '',
    auto_score        FLOAT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS generation_feedback (
    id                   TEXT PRIMARY KEY,
    generation_id        TEXT        NOT NULL REFERENCES generation_logs(id) ON DELETE CASCADE,
    user_id              TEXT        NOT NULL,
    tenant_id            TEXT,
    rating               INT         NOT NULL CHECK (rating IN (-1, 0, 1)),
    anomaly_type         TEXT,
    comment              TEXT,
    corrected_output     TEXT,
    missing_resource_ids TEXT[],
    spurious_resource_ids TEXT[],
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cached_documents (
    id            TEXT PRIMARY KEY,
    resource_id   TEXT        NOT NULL,
    title         TEXT        NOT NULL DEFAULT '',
    content       TEXT        NOT NULL DEFAULT '',
    generation_id TEXT        REFERENCES generation_logs(id) ON DELETE SET NULL,
    status        TEXT        NOT NULL DEFAULT 'draft',
    auto_score    FLOAT,
    approved_by   TEXT,
    approved_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS generation_logs_tenant     ON generation_logs (tenant_id);
CREATE INDEX IF NOT EXISTS generation_logs_resource   ON generation_logs (resource_id);
CREATE INDEX IF NOT EXISTS generation_feedback_gen    ON generation_feedback (generation_id);
CREATE INDEX IF NOT EXISTS cached_documents_resource  ON cached_documents (resource_id);
