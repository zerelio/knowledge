CREATE TABLE IF NOT EXISTS knowledge_summaries (
    id             TEXT PRIMARY KEY,
    scope_id       TEXT        NOT NULL,
    tenant_id      TEXT        NOT NULL,
    type_prefix    TEXT        NOT NULL,  -- "aws:ec2", "all", etc.
    scope_name     TEXT        NOT NULL,
    summary        TEXT        NOT NULL,
    resource_count INT         NOT NULL DEFAULT 0,
    generated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One summary per (scope, type_prefix) combination — upserted on each run.
CREATE UNIQUE INDEX IF NOT EXISTS knowledge_summaries_scope_type
    ON knowledge_summaries (scope_id, type_prefix);

CREATE INDEX IF NOT EXISTS knowledge_summaries_tenant
    ON knowledge_summaries (tenant_id);

-- Full-text search over summaries enables the LLM to retrieve relevant
-- community-level context using the same tsvector pipeline as resources.
ALTER TABLE knowledge_summaries
    ADD COLUMN IF NOT EXISTS tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('english', coalesce(summary, ''))) STORED;

CREATE INDEX IF NOT EXISTS knowledge_summaries_tsv
    ON knowledge_summaries USING gin(tsv);
