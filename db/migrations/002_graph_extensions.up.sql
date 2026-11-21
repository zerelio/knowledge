-- Zerelio Knowledge — Graph Extension Setup (AGE + pgvector)
-- This migration is applied automatically by docker-entrypoint-initdb.d
-- or manually via golang-migrate.
--
-- Prerequisites: PostgreSQL with Apache AGE extension installed.
-- The apache/age Docker image satisfies this requirement.

BEGIN;

-- Apache AGE: graph database extension for Cypher queries.
-- Silently skipped if the extension is not installed (non-PG environments).
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS age;
EXCEPTION
    WHEN OTHERS THEN
        RAISE NOTICE 'AGE extension not available — graph features disabled: %', SQLERRM;
END;
$$;

-- pgvector: vector similarity search for hybrid search (Milestone 0.3).
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS vector;
EXCEPTION
    WHEN OTHERS THEN
        RAISE NOTICE 'pgvector extension not available — vector search disabled: %', SQLERRM;
END;
$$;

-- Full-text search configuration (BM25 baseline).
CREATE INDEX IF NOT EXISTS resources_fts_idx
    ON resources USING gin (
        to_tsvector('english',
            coalesce(name, '') || ' ' ||
            coalesce(type, '') || ' ' ||
            coalesce(provider, '')
        )
    );

COMMIT;
