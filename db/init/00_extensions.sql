-- Enable pgvector for semantic search.
CREATE EXTENSION IF NOT EXISTS vector;

-- Apache AGE is not bundled in pgvector/pgvector:pg16.
-- Zerelio detects its absence and disables graph features gracefully.
-- To enable graph queries, use a postgres image that includes AGE.
