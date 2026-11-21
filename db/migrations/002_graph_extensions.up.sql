-- Enable Apache AGE for graph queries.
-- If AGE is not available (pgvector-only image), this is a no-op;
-- the application detects AGE absence and disables graph features gracefully.
CREATE EXTENSION IF NOT EXISTS age;
