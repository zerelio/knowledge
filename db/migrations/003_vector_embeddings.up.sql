-- Enable pgvector and add embeddings column.
-- Fully defensive: each step guarded so the migration succeeds even without pgvector.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS vector;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pgvector not available — vector search disabled at runtime';
END;
$$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        ALTER TABLE resources ADD COLUMN IF NOT EXISTS embedding vector(1536);
        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes WHERE tablename = 'resources' AND indexname = 'resources_embedding_idx'
        ) THEN
            CREATE INDEX resources_embedding_idx
                ON resources USING ivfflat (embedding vector_cosine_ops)
                WITH (lists = 100);
        END IF;
    END IF;
END;
$$;
