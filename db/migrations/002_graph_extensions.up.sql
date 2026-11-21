-- Enable Apache AGE for graph queries (optional — app handles absence gracefully).
-- Using a DO block so this migration succeeds even when AGE is not installed.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS age;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Apache AGE extension not available — graph features will be disabled at runtime';
END;
$$;
