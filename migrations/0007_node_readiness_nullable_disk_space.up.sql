-- ---------------------------------------------------------------------------
-- 0007: Allow NULL for sufficient_disk_space in node_readiness
-- ---------------------------------------------------------------------------
-- The readiness evaluator treats disk space as a tri-state:
--   true  = sufficient disk space confirmed
--   false = insufficient disk space confirmed
--   NULL  = unknown (stale node, missing filesystem data, etc.)
--
-- The original schema (0001) defined the column as NOT NULL DEFAULT FALSE,
-- which prevents the evaluator from persisting NULL (unknown) values.
-- ---------------------------------------------------------------------------

ALTER TABLE node_readiness
    ALTER COLUMN sufficient_disk_space DROP NOT NULL,
    ALTER COLUMN sufficient_disk_space DROP DEFAULT;
