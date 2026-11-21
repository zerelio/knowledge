-- Enable pgvector for semantic search and add the embeddings column.
CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE resources ADD COLUMN IF NOT EXISTS embedding vector(1536);

CREATE INDEX IF NOT EXISTS resources_embedding_idx
    ON resources USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
