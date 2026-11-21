-- Zerelio Knowledge — Generation logs, documents, and human feedback
-- Enables the human-in-the-loop feedback loop for AI-generated content.

BEGIN;

-- Full audit log of every LLM generation (doc, runbook, search rerank).
-- Stores the exact context sent to the model so outputs are fully reproducible.
CREATE TABLE IF NOT EXISTS generation_logs (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL,              -- 'doc' | 'runbook' | 'rerank'
    tenant_id       TEXT NOT NULL DEFAULT '',
    user_id         TEXT NOT NULL DEFAULT '',
    resource_id     TEXT NOT NULL DEFAULT '',   -- primary resource (for docs)
    query           TEXT NOT NULL DEFAULT '',   -- query string (for runbooks/rerank)
    context_text    TEXT NOT NULL DEFAULT '',   -- full context sent to LLM
    retrieved_ids   JSONB NOT NULL DEFAULT '[]', -- resource IDs that were retrieved
    retrieval_signals JSONB NOT NULL DEFAULT '{}', -- signals used: bm25/vector/graph scores
    model           TEXT NOT NULL DEFAULT '',
    provider        TEXT NOT NULL DEFAULT '',
    tokens_prompt   INT  NOT NULL DEFAULT 0,
    tokens_completion INT NOT NULL DEFAULT 0,
    latency_ms      INT  NOT NULL DEFAULT 0,
    output          TEXT NOT NULL DEFAULT '',   -- raw LLM output
    auto_score      FLOAT,                      -- automated quality score (RAGAS-lite)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS gen_resource_idx  ON generation_logs (resource_id);
CREATE INDEX IF NOT EXISTS gen_type_idx      ON generation_logs (type);
CREATE INDEX IF NOT EXISTS gen_tenant_idx    ON generation_logs (tenant_id);
CREATE INDEX IF NOT EXISTS gen_time_idx      ON generation_logs (created_at DESC);

-- Human feedback on a generation.
CREATE TABLE IF NOT EXISTS generation_feedback (
    id              TEXT PRIMARY KEY,
    generation_id   TEXT NOT NULL REFERENCES generation_logs(id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL DEFAULT '',
    tenant_id       TEXT NOT NULL DEFAULT '',
    rating          SMALLINT NOT NULL DEFAULT 0, -- -1 thumbs_down, 0 neutral, 1 thumbs_up
    anomaly_type    TEXT NOT NULL DEFAULT '',    -- 'hallucination' | 'outdated' | 'incorrect' | 'missing_context' | 'irrelevant_retrieval'
    comment         TEXT NOT NULL DEFAULT '',
    corrected_output TEXT NOT NULL DEFAULT '',  -- human-corrected version (empty = no correction)
    missing_resource_ids  JSONB NOT NULL DEFAULT '[]', -- resources that should have been retrieved
    spurious_resource_ids JSONB NOT NULL DEFAULT '[]', -- resources that shouldn't have been retrieved
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS feedback_gen_idx    ON generation_feedback (generation_id);
CREATE INDEX IF NOT EXISTS feedback_user_idx   ON generation_feedback (user_id);
CREATE INDEX IF NOT EXISTS feedback_rating_idx ON generation_feedback (rating);

-- Cached generated documents. Invalidated when the resource changes.
-- Status: 'draft' → human review → 'approved' | 'rejected' | 'flagged'
CREATE TABLE IF NOT EXISTS documents (
    id              TEXT PRIMARY KEY,
    resource_id     TEXT NOT NULL,
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    generation_id   TEXT REFERENCES generation_logs(id),
    status          TEXT NOT NULL DEFAULT 'draft',
    auto_score      FLOAT,
    approved_by     TEXT NOT NULL DEFAULT '',
    approved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS doc_resource_idx ON documents (resource_id);
CREATE INDEX IF NOT EXISTS doc_status_idx   ON documents (status);

-- Cached runbooks.
CREATE TABLE IF NOT EXISTS runbooks (
    id              TEXT PRIMARY KEY,
    title           TEXT NOT NULL,
    scenario        TEXT NOT NULL,
    content         TEXT NOT NULL,
    resource_ids    JSONB NOT NULL DEFAULT '[]',
    generation_id   TEXT REFERENCES generation_logs(id),
    status          TEXT NOT NULL DEFAULT 'draft',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS runbook_status_idx ON runbooks (status);

COMMIT;
