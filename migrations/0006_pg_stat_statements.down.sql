-- Migration 0006 (down): Remove pg_stat_statements extension
--
-- This reverses the up migration by dropping the extension. The DO block
-- ensures the migration succeeds even if the extension was never created
-- (e.g. because shared_preload_libraries was not configured).

DO $$
BEGIN
    DROP EXTENSION IF EXISTS pg_stat_statements;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_stat_statements extension could not be dropped: %', SQLERRM;
END
$$;
