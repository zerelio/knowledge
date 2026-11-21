-- Zerelio Knowledge — Vector embeddings for hybrid search
-- Requires pgvector extension (applied by migration 002 if available).

BEGIN;

-- Add embedding column to resources for vector similarity search.
-- NULL until an embedding job populates it; BM25 is the fallback.
DO $$
BEGIN
    ALTER TABLE resources ADD COLUMN IF NOT EXISTS embedding vector(1536);
EXCEPTION
    WHEN OTHERS THEN
        RAISE NOTICE 'pgvector not available — vector search disabled: %', SQLERRM;
END;
$$;

-- Index for fast approximate nearest-neighbour search (IVFFlat).
-- Only created if the column exists.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'resources' AND column_name = 'embedding'
    ) THEN
        EXECUTE $q$
            CREATE INDEX IF NOT EXISTS resources_embedding_idx
            ON resources USING ivfflat (embedding vector_cosine_ops)
            WITH (lists = 100)
        $q$;
    END IF;
EXCEPTION
    WHEN OTHERS THEN
        RAISE NOTICE 'Could not create vector index: %', SQLERRM;
END;
$$;

COMMIT;
