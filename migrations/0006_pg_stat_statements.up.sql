-- Migration 0006: Enable pg_stat_statements extension
--
-- pg_stat_statements provides per-query execution statistics (call count,
-- total/mean/min/max execution time, rows returned, buffer usage). It ships
-- as a contrib module with every standard PostgreSQL installation.
--
-- Prerequisite: shared_preload_libraries must include 'pg_stat_statements'
-- in postgresql.conf (requires a PostgreSQL restart). When deploying via
-- Docker or Helm, pass:
--   postgres -c shared_preload_libraries=pg_stat_statements
--
-- If shared_preload_libraries is not configured, CREATE EXTENSION will fail.
-- We wrap it in a DO block that catches the error and raises a notice so
-- that the migration succeeds regardless — the application degrades
-- gracefully by checking for the extension at query time.

DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_stat_statements extension could not be created: %. '
                 'Query-level performance stats will be unavailable. '
                 'To enable, add shared_preload_libraries=pg_stat_statements '
                 'to postgresql.conf and restart PostgreSQL.', SQLERRM;
END
$$;
